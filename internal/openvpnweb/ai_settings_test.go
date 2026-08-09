package openvpnweb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/spf13/viper"
	"gorm.io/gorm"

	"openvpn-web/internal/openvpnweb/ai"
)

func newTestAIDatabase(t *testing.T, filename string) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filename), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("get test database handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return database
}

func configureLegacyAI(t *testing.T, provider, baseURL, apiKey, model string) {
	t.Helper()
	viper.Reset()
	viper.Set("ai.enabled", true)
	viper.Set("ai.provider", provider)
	viper.Set("ai.base_url", baseURL)
	viper.Set("ai.api_key", apiKey)
	viper.Set("ai.model", model)
	viper.Set("ai.system_prompt", "legacy prompt")
	viper.Set("ai.max_tokens", 2048)
	viper.Set("ai.temperature", 0.4)
}

func TestMigrateAISettingsMigratesOnceAndProtectsKey(t *testing.T) {
	previousSecret := secretKey
	secretKey = "test-secret-key"
	t.Cleanup(func() { secretKey = previousSecret; viper.Reset() })
	configureLegacyAI(t, AIProviderDeepSeek, "https://api.deepseek.com/v1", "legacy-secret", "deepseek-v4-flash")

	database := newTestAIDatabase(t, ":memory:")
	if err := MigrateAISettings(database); err != nil {
		t.Fatalf("MigrateAISettings() error: %v", err)
	}

	config, err := activeAIConfig(database)
	if err != nil {
		t.Fatalf("activeAIConfig() error: %v", err)
	}
	if config.Provider != AIProviderDeepSeek || config.APIKey != "legacy-secret" {
		t.Fatalf("migrated config = %#v, want DeepSeek legacy profile", config)
	}
	var profile AIProviderProfile
	if err := database.Where("provider = ?", AIProviderDeepSeek).First(&profile).Error; err != nil {
		t.Fatalf("load migrated profile: %v", err)
	}
	if profile.APIKeyEncrypted == "legacy-secret" || profile.APIKeyEncrypted == "" {
		t.Fatalf("API key was not encrypted in SQLite: %q", profile.APIKeyEncrypted)
	}
	if viper.GetString("ai.api_key") != "" {
		t.Fatalf("legacy Viper API key was not cleared")
	}

	viper.Set("ai.model", "should-not-overwrite")
	viper.Set("ai.api_key", "must-not-remain-in-viper")
	if err := MigrateAISettings(database); err != nil {
		t.Fatalf("idempotent MigrateAISettings() error: %v", err)
	}
	if viper.GetString("ai.api_key") != "" {
		t.Fatal("idempotent migration did not retry legacy plaintext API key cleanup")
	}
	config, err = activeAIConfig(database)
	if err != nil || config.Model != "deepseek-v4-flash" {
		t.Fatalf("migration overwrote stored profile: config=%#v err=%v", config, err)
	}
}

func TestSaveAISettingsKeepsProfilesIndependentAndNeverReturnsKey(t *testing.T) {
	previousSecret := secretKey
	secretKey = "test-secret-key"
	t.Cleanup(func() { secretKey = previousSecret; viper.Reset() })
	configureLegacyAI(t, AIProviderOllama, "http://127.0.0.1:11434", "", "qwen2.5:7b")
	path := filepath.Join(t.TempDir(), "ai.db")
	database := newTestAIDatabase(t, path)
	if err := MigrateAISettings(database); err != nil {
		t.Fatal(err)
	}

	_, err := saveAISettings(database, AISettingsRequest{
		Enabled: true, ActiveProvider: AIProviderDeepSeek,
		Profiles: []AIProviderProfileInput{
			{Provider: AIProviderOllama, BaseURL: "http://127.0.0.1:11434", Model: "qwen2.5:7b", SystemPrompt: "ollama", MaxTokens: 4096, Temperature: 0.7},
			{Provider: AIProviderDeepSeek, BaseURL: "https://api.deepseek.com/v1", APIKey: "deepseek-secret", Model: "deepseek-v4-pro", SystemPrompt: "deepseek", MaxTokens: 8192, Temperature: 0.3},
		},
	})
	if err != nil {
		t.Fatalf("save profiles: %v", err)
	}
	response, err := aiSettingsAPIResponse(database, AIProviderDeepSeek, "deepseek-v4-pro")
	if err != nil {
		t.Fatal(err)
	}
	if response.Config.APIKey != "" {
		t.Fatalf("GET response leaked API key: %q", response.Config.APIKey)
	}
	var deepSeekProfile AIProviderProfile
	if err := database.Where("provider = ?", AIProviderDeepSeek).First(&deepSeekProfile).Error; err != nil {
		t.Fatal(err)
	}
	if deepSeekProfile.APIKeyEncrypted == "deepseek-secret" || deepSeekProfile.APIKeyEncrypted == "" {
		t.Fatalf("database API key is not encrypted: %q", deepSeekProfile.APIKeyEncrypted)
	}

	// Reopen the database to simulate a service restart and switch the active profile.
	reopened := newTestAIDatabase(t, path)
	if err := MigrateAISettings(reopened); err != nil {
		t.Fatal(err)
	}
	config, err := saveAISettings(reopened, AISettingsRequest{
		Enabled: true, ActiveProvider: AIProviderOllama,
		Profiles: []AIProviderProfileInput{
			{Provider: AIProviderOllama, BaseURL: "http://127.0.0.1:11434", Model: "qwen2.5:7b", SystemPrompt: "ollama", MaxTokens: 4096, Temperature: 0.7},
			{Provider: AIProviderDeepSeek, BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-v4-pro", SystemPrompt: "deepseek", MaxTokens: 8192, Temperature: 0.3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.Provider != AIProviderOllama || config.BaseURL != "http://127.0.0.1:11434" {
		t.Fatalf("active Ollama profile not restored: %#v", config)
	}
	if _, err := saveAISettings(reopened, AISettingsRequest{Enabled: true, ActiveProvider: AIProviderDeepSeek, Profiles: []AIProviderProfileInput{{Provider: AIProviderDeepSeek, BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-v4-pro", SystemPrompt: "deepseek", MaxTokens: 8192, Temperature: 0.3}}}); err != nil {
		t.Fatal(err)
	}
	config, err = activeAIConfig(reopened)
	if err != nil || config.APIKey != "deepseek-secret" || config.Model != "deepseek-v4-pro" {
		t.Fatalf("DeepSeek profile was not preserved after restart: %#v err=%v", config, err)
	}
}

func TestOllamaBareURLUsesV1ChatCompletions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := ai.NewLLMClient(ai.LLMConfig{Provider: AIProviderOllama, BaseURL: server.URL, Model: "qwen2.5:7b", Temperature: 0.7})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error: %v", err)
	}
}
