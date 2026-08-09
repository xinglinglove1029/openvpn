// AI provider settings are stored in SQLite so that every provider retains its own profile.
package openvpnweb

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gavintan/gopkg/aes"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

const (
	AIProviderOllama    = "ollama"
	AIProviderDeepSeek  = "deepseek"
	AIProviderOpenAI    = "openai"
	AIProviderCustomize = "customize"
)

var aiProviders = map[string]struct{}{
	AIProviderOllama: {}, AIProviderDeepSeek: {}, AIProviderOpenAI: {}, AIProviderCustomize: {},
}

// AISettings is the singleton containing settings shared by all provider profiles.
type AISettings struct {
	ID             uint   `gorm:"primaryKey"`
	Enabled        bool   `gorm:"not null;default:false"`
	ActiveProvider string `gorm:"size:32;not null"`
}

// AIProviderProfile stores all provider-specific settings. API keys are always AES-encrypted.
type AIProviderProfile struct {
	ID              uint    `gorm:"primaryKey"`
	Provider        string  `gorm:"size:32;not null;uniqueIndex"`
	BaseURL         string  `gorm:"not null"`
	APIKeyEncrypted string  `gorm:"column:api_key_encrypted"`
	Model           string  `gorm:"not null"`
	SystemPrompt    string  `gorm:"type:text"`
	MaxTokens       int     `gorm:"not null"`
	Temperature     float64 `gorm:"not null"`
}

func (AISettings) TableName() string { return "ai_settings" }

func (AIProviderProfile) TableName() string { return "ai_provider_profiles" }

// AIProviderProfileInput is accepted by the settings endpoint. api_key is write-only.
type AIProviderProfileInput struct {
	Provider     string  `json:"provider"`
	BaseURL      string  `json:"base_url"`
	APIKey       string  `json:"api_key,omitempty"`
	ClearAPIKey  bool    `json:"clear_api_key,omitempty"`
	Model        string  `json:"model"`
	SystemPrompt string  `json:"system_prompt"`
	MaxTokens    int     `json:"max_tokens"`
	Temperature  float64 `json:"temperature"`
}

// AIProviderProfileResponse deliberately omits the API key value.
type AIProviderProfileResponse struct {
	Provider     string  `json:"provider"`
	BaseURL      string  `json:"base_url"`
	Model        string  `json:"model"`
	SystemPrompt string  `json:"system_prompt"`
	MaxTokens    int     `json:"max_tokens"`
	Temperature  float64 `json:"temperature"`
	HasAPIKey    bool    `json:"has_api_key"`
}

// AISettingsRequest keeps the previous flat payload compatible while accepting profiles.
type AISettingsRequest struct {
	Enabled        bool                     `json:"enabled"`
	Provider       string                   `json:"provider"`
	ActiveProvider string                   `json:"active_provider"`
	BaseURL        string                   `json:"base_url"`
	APIKey         string                   `json:"api_key,omitempty"`
	ClearAPIKey    bool                     `json:"clear_api_key,omitempty"`
	Model          string                   `json:"model"`
	SystemPrompt   string                   `json:"system_prompt"`
	MaxTokens      int                      `json:"max_tokens"`
	Temperature    float64                  `json:"temperature"`
	Profiles       []AIProviderProfileInput `json:"profiles"`
}

type AISettingsResponse struct {
	Config         AIConfig                    `json:"config"`
	Profiles       []AIProviderProfileResponse `json:"profiles"`
	ActiveProvider string                      `json:"active_provider"`
	Provider       string                      `json:"provider"`
	Model          string                      `json:"model"`
}

func normalizeAIProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if _, ok := aiProviders[provider]; ok {
		return provider
	}
	return AIProviderOllama
}

func defaultAIProfile(provider string) AIProviderProfile {
	provider = normalizeAIProvider(provider)
	profile := AIProviderProfile{
		Provider: provider, MaxTokens: 4096, Temperature: 0.7,
		SystemPrompt: viper.GetString("ai.system_prompt"),
	}
	switch provider {
	case AIProviderOllama:
		profile.BaseURL, profile.Model = "http://127.0.0.1:11434/v1", "qwen2.5:7b"
	case AIProviderDeepSeek:
		profile.BaseURL, profile.Model = "https://api.deepseek.com/v1", "deepseek-v4-flash"
	case AIProviderOpenAI:
		profile.BaseURL, profile.Model = "https://api.openai.com/v1", "gpt-5.4-mini"
	case AIProviderCustomize:
		profile.BaseURL, profile.Model = "https://your-api.com/v1", ""
	}
	return profile
}

func normalizeAIProfileInput(in AIProviderProfileInput) (AIProviderProfileInput, error) {
	in.Provider = normalizeAIProvider(in.Provider)
	defaults := defaultAIProfile(in.Provider)
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if in.BaseURL == "" {
		in.BaseURL = defaults.BaseURL
	}
	in.Model = strings.TrimSpace(in.Model)
	if in.Model == "" {
		return in, errors.New("模型名称不能为空")
	}
	if in.MaxTokens <= 0 {
		in.MaxTokens = defaults.MaxTokens
	}
	if in.Temperature < 0 {
		in.Temperature = defaults.Temperature
	}
	if in.SystemPrompt == "" {
		in.SystemPrompt = defaults.SystemPrompt
	}
	return in, nil
}

func encryptAIKey(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if secretKey == "" {
		return "", errors.New("AI 密钥加密失败：系统密钥未初始化")
	}
	return aes.AesEncrypt(plain, secretKey)
}

func decryptAIKey(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if secretKey == "" {
		return "", errors.New("AI 密钥解密失败：系统密钥未初始化")
	}
	return aes.AesDecrypt(ciphertext, secretKey)
}

// clearLegacyAIAPIKey removes the plaintext key from the legacy Viper source.
// It is intentionally safe to retry after a previous migration completed in SQLite.
func clearLegacyAIAPIKey() error {
	if viper.GetString("ai.api_key") == "" {
		return nil
	}
	viper.Set("ai.api_key", "")
	if viper.ConfigFileUsed() == "" {
		return nil
	}
	if err := viper.WriteConfig(); err != nil {
		return fmt.Errorf("??? AI ??????: %w", err)
	}
	return nil
}

// MigrateAISettings creates the tables and imports the old Viper ai.* settings only once.
func MigrateAISettings(database *gorm.DB) error {
	if err := database.AutoMigrate(&AISettings{}, &AIProviderProfile{}); err != nil {
		return err
	}
	var count int64
	if err := database.Model(&AISettings{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		// If a previous run committed the database transaction but could not write the
		// legacy file, finish the plaintext-key cleanup without overwriting profiles.
		return clearLegacyAIAPIKey()
	}

	provider := normalizeAIProvider(viper.GetString("ai.provider"))
	legacy := AIProviderProfileInput{
		Provider: provider, BaseURL: viper.GetString("ai.base_url"), APIKey: viper.GetString("ai.api_key"),
		Model: viper.GetString("ai.model"), SystemPrompt: viper.GetString("ai.system_prompt"),
		MaxTokens: viper.GetInt("ai.max_tokens"), Temperature: viper.GetFloat64("ai.temperature"),
	}
	if legacy.Model == "" {
		legacy.Model = defaultAIProfile(provider).Model
	}
	legacy, err := normalizeAIProfileInput(legacy)
	if err != nil {
		return fmt.Errorf("???? AI ????: %w", err)
	}
	encrypted, err := encryptAIKey(legacy.APIKey)
	if err != nil {
		return err
	}

	err = database.Transaction(func(tx *gorm.DB) error {
		profile := AIProviderProfile{Provider: legacy.Provider, BaseURL: legacy.BaseURL, APIKeyEncrypted: encrypted, Model: legacy.Model, SystemPrompt: legacy.SystemPrompt, MaxTokens: legacy.MaxTokens, Temperature: legacy.Temperature}
		if err := tx.Create(&profile).Error; err != nil {
			return err
		}
		return tx.Create(&AISettings{ID: 1, Enabled: viper.GetBool("ai.enabled"), ActiveProvider: provider}).Error
	})
	if err != nil {
		return err
	}

	// The legacy JSON is only a migration source. Never leave a newly-migrated API key there.
	return clearLegacyAIAPIKey()
}

func loadAISettings(database *gorm.DB) (AISettings, []AIProviderProfile, error) {
	var settings AISettings
	if err := database.First(&settings, 1).Error; err != nil {
		return settings, nil, err
	}
	var profiles []AIProviderProfile
	if err := database.Order("provider ASC").Find(&profiles).Error; err != nil {
		return settings, nil, err
	}
	return settings, profiles, nil
}

func activeAIConfig(database *gorm.DB) (AIConfig, error) {
	settings, profiles, err := loadAISettings(database)
	if err != nil {
		return AIConfig{}, err
	}
	for _, profile := range profiles {
		if profile.Provider != settings.ActiveProvider {
			continue
		}
		key, err := decryptAIKey(profile.APIKeyEncrypted)
		if err != nil {
			return AIConfig{}, err
		}
		return AIConfig{
			Enabled: settings.Enabled, Provider: profile.Provider, BaseURL: profile.BaseURL,
			APIKey: key, Model: profile.Model, SystemPrompt: profile.SystemPrompt,
			MaxTokens: profile.MaxTokens, Temperature: profile.Temperature,
		}, nil
	}
	return AIConfig{}, fmt.Errorf("active AI provider profile %q was not found", settings.ActiveProvider)
}

func aiSettingsAPIResponse(database *gorm.DB, runtimeProvider, runtimeModel string) (AISettingsResponse, error) {
	settings, profiles, err := loadAISettings(database)
	if err != nil {
		return AISettingsResponse{}, err
	}
	response := AISettingsResponse{ActiveProvider: settings.ActiveProvider, Provider: runtimeProvider, Model: runtimeModel}
	for _, profile := range profiles {
		response.Profiles = append(response.Profiles, AIProviderProfileResponse{
			Provider: profile.Provider, BaseURL: profile.BaseURL, Model: profile.Model,
			SystemPrompt: profile.SystemPrompt, MaxTokens: profile.MaxTokens,
			Temperature: profile.Temperature, HasAPIKey: profile.APIKeyEncrypted != "",
		})
		if profile.Provider == settings.ActiveProvider {
			// api_key is intentionally left empty; GET never returns plaintext or a usable key.
			response.Config = AIConfig{Enabled: settings.Enabled, Provider: profile.Provider, BaseURL: profile.BaseURL, Model: profile.Model, SystemPrompt: profile.SystemPrompt, MaxTokens: profile.MaxTokens, Temperature: profile.Temperature}
		}
	}
	return response, nil
}

// saveAISettings saves all supplied profiles atomically. Empty or masked API keys keep a saved key.
func saveAISettings(database *gorm.DB, req AISettingsRequest) (AIConfig, error) {
	active := req.ActiveProvider
	if active == "" {
		active = req.Provider
	}
	if _, ok := aiProviders[strings.ToLower(strings.TrimSpace(active))]; !ok {
		return AIConfig{}, errors.New("unsupported provider")
	}
	active = normalizeAIProvider(active)
	profiles := req.Profiles
	if len(profiles) == 0 {
		profiles = []AIProviderProfileInput{{
			Provider: active, BaseURL: req.BaseURL, APIKey: req.APIKey, ClearAPIKey: req.ClearAPIKey,
			Model: req.Model, SystemPrompt: req.SystemPrompt, MaxTokens: req.MaxTokens, Temperature: req.Temperature,
		}}
	}

	normalized := make(map[string]AIProviderProfileInput, len(profiles))
	for _, profile := range profiles {
		if _, ok := aiProviders[strings.ToLower(strings.TrimSpace(profile.Provider))]; !ok {
			return AIConfig{}, fmt.Errorf("unsupported provider %q", profile.Provider)
		}
		provider := normalizeAIProvider(profile.Provider)
		// The untouched custom profile is allowed to remain blank while another profile is active.
		if provider != active && strings.TrimSpace(profile.Model) == "" {
			continue
		}
		p, err := normalizeAIProfileInput(profile)
		if err != nil {
			return AIConfig{}, fmt.Errorf("invalid %s profile: %w", profile.Provider, err)
		}
		normalized[p.Provider] = p
	}
	if _, ok := normalized[active]; !ok {
		return AIConfig{}, errors.New("the active provider requires a model")
	}

	err := database.Transaction(func(tx *gorm.DB) error {
		settings := AISettings{ID: 1}
		if err := tx.FirstOrCreate(&settings, AISettings{ID: 1}).Error; err != nil {
			return err
		}
		settings.Enabled, settings.ActiveProvider = req.Enabled, active
		if err := tx.Save(&settings).Error; err != nil {
			return err
		}
		for provider, input := range normalized {
			profile := defaultAIProfile(provider)
			err := tx.Where("provider = ?", provider).First(&profile).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			profile.Provider, profile.BaseURL, profile.Model = provider, input.BaseURL, input.Model
			profile.SystemPrompt, profile.MaxTokens, profile.Temperature = input.SystemPrompt, input.MaxTokens, input.Temperature
			if input.ClearAPIKey {
				profile.APIKeyEncrypted = ""
			} else if key := strings.TrimSpace(input.APIKey); key != "" && key != "****" {
				encrypted, err := encryptAIKey(key)
				if err != nil {
					return err
				}
				profile.APIKeyEncrypted = encrypted
			}
			if profile.ID == 0 {
				if err := tx.Create(&profile).Error; err != nil {
					return err
				}
			} else if err := tx.Save(&profile).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return AIConfig{}, err
	}
	return activeAIConfig(database)
}

func isAIEnabled(database *gorm.DB) bool {
	var settings AISettings
	return database.First(&settings, 1).Error == nil && settings.Enabled
}
