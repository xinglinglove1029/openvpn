package openvpnweb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
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

func configureConflictingLegacyAI(t *testing.T, provider, baseURL, apiKey, model string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("ai.enabled", true)
	viper.Set("ai.provider", provider)
	viper.Set("ai.base_url", baseURL)
	viper.Set("ai.api_key", apiKey)
	viper.Set("ai.model", model)
	viper.Set("ai.system_prompt", "legacy prompt")
	viper.Set("ai.max_tokens", 2048)
	viper.Set("ai.temperature", 0.4)
}

func TestMigrateAISettingsCreatesDisabledOllamaDefaultWithoutViper(t *testing.T) {
	configureConflictingLegacyAI(t, AIProviderDeepSeek, "https://legacy.example/v1", "legacy-secret", "legacy-model")
	database := newTestAIDatabase(t, ":memory:")

	if err := MigrateAISettings(database); err != nil {
		t.Fatalf("MigrateAISettings() error: %v", err)
	}

	var settings AISettings
	if err := database.First(&settings, 1).Error; err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if settings.Enabled || settings.ActiveProvider != AIProviderOllama {
		t.Fatalf("settings = %#v, want disabled Ollama defaults", settings)
	}

	var profile AIProviderProfile
	if err := database.Where("provider = ?", AIProviderOllama).First(&profile).Error; err != nil {
		t.Fatalf("load default Ollama profile: %v", err)
	}
	if profile.BaseURL != "http://127.0.0.1:11434/v1" || profile.Model != "qwen2.5:7b" {
		t.Fatalf("default profile = %#v, want Ollama database defaults", profile)
	}
	if profile.APIKeyEncrypted != "" {
		t.Fatalf("default profile unexpectedly contains an API key: %q", profile.APIKeyEncrypted)
	}
	if profile.SystemPrompt != defaultAISystemPrompt {
		t.Fatal("default profile did not preserve the built-in system prompt")
	}
	if viper.GetString("ai.api_key") != "legacy-secret" {
		t.Fatalf("AI initialization changed legacy Viper state: %q", viper.GetString("ai.api_key"))
	}
}

func TestMigrateAISettingsPreservesExistingDatabaseConfiguration(t *testing.T) {
	previousSecret := secretKey
	secretKey = "test-secret-key"
	t.Cleanup(func() { secretKey = previousSecret })
	configureConflictingLegacyAI(t, AIProviderOllama, "http://legacy.invalid/v1", "legacy-secret", "legacy-model")

	database := newTestAIDatabase(t, ":memory:")
	if err := database.AutoMigrate(&AISettings{}, &AIProviderProfile{}); err != nil {
		t.Fatalf("prepare tables: %v", err)
	}
	encrypted, err := encryptAIKey("database-secret")
	if err != nil {
		t.Fatalf("encrypt test key: %v", err)
	}
	if err := database.Create(&AIProviderProfile{
		Provider: AIProviderDeepSeek, BaseURL: "https://api.deepseek.com/v1", APIKeyEncrypted: encrypted,
		Model: "deepseek-v4-pro", SystemPrompt: "database prompt", MaxTokens: 8192, Temperature: 0.3,
	}).Error; err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := database.Create(&AISettings{ID: 1, Enabled: true, ActiveProvider: AIProviderDeepSeek}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}

	if err := MigrateAISettings(database); err != nil {
		t.Fatalf("MigrateAISettings() error: %v", err)
	}

	config, err := activeAIConfig(database)
	if err != nil {
		t.Fatalf("activeAIConfig() error: %v", err)
	}
	if config.Provider != AIProviderDeepSeek || config.Model != "deepseek-v4-pro" || config.APIKey != "database-secret" {
		t.Fatalf("existing database configuration was overwritten: %#v", config)
	}
	var profileCount int64
	if err := database.Model(&AIProviderProfile{}).Count(&profileCount).Error; err != nil {
		t.Fatalf("count profiles: %v", err)
	}
	if profileCount != 1 {
		t.Fatalf("existing database initialization added profiles: got %d, want 1", profileCount)
	}
	if viper.GetString("ai.model") != "legacy-model" {
		t.Fatalf("AI initialization changed legacy Viper state: %q", viper.GetString("ai.model"))
	}
}

func TestSaveAISettingsDoesNotDependOnLegacyViper(t *testing.T) {
	previousSecret := secretKey
	secretKey = "test-secret-key"
	t.Cleanup(func() { secretKey = previousSecret; viper.Reset() })

	legacyPath := filepath.Join(t.TempDir(), "config.json")
	legacyJSON := []byte(`{"ai":{"enabled":true,"provider":"ollama","base_url":"http://legacy.invalid/v1","api_key":"legacy-secret","model":"legacy-model"}}`)
	if err := os.WriteFile(legacyPath, legacyJSON, 0o400); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	viper.Reset()
	viper.SetConfigFile(legacyPath)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("read legacy config: %v", err)
	}

	database := newTestAIDatabase(t, ":memory:")
	if err := MigrateAISettings(database); err != nil {
		t.Fatalf("MigrateAISettings() error: %v", err)
	}
	if _, err := saveAISettings(database, AISettingsRequest{
		Enabled: true, ActiveProvider: AIProviderDeepSeek,
		Profiles: []AIProviderProfileInput{{
			Provider: AIProviderDeepSeek, BaseURL: "https://api.deepseek.com/v1", APIKey: "database-secret",
			Model: "deepseek-v4-flash", SystemPrompt: "database prompt", MaxTokens: 4096, Temperature: 0.7,
		}},
	}); err != nil {
		t.Fatalf("saveAISettings() error: %v", err)
	}

	contents, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read legacy config after save: %v", err)
	}
	if string(contents) != string(legacyJSON) {
		t.Fatalf("saving AI settings wrote legacy config: got %q, want %q", contents, legacyJSON)
	}
	if viper.GetString("ai.api_key") != "legacy-secret" {
		t.Fatalf("saving AI settings changed legacy Viper state: %q", viper.GetString("ai.api_key"))
	}

	config, err := activeAIConfig(database)
	if err != nil {
		t.Fatalf("activeAIConfig() error: %v", err)
	}
	if config.Provider != AIProviderDeepSeek || config.APIKey != "database-secret" {
		t.Fatalf("saved database configuration = %#v", config)
	}
}

func TestSaveAISettingsKeepsProfilesIndependentAndNeverReturnsKey(t *testing.T) {
	previousSecret := secretKey
	secretKey = "test-secret-key"
	t.Cleanup(func() { secretKey = previousSecret })
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
