// AI provider settings are stored in SQLite so that every provider retains its own profile.
package openvpnweb

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gavintan/gopkg/aes"
	"gorm.io/gorm"
)

const (
	AIProviderOllama    = "ollama"
	AIProviderDeepSeek  = "deepseek"
	AIProviderOpenAI    = "openai"
	AIProviderCustomize = "customize"
)

const defaultAISystemPrompt = "你是 OpenVPN 运维控制台的智能助手，具备全面的运维管理能力。\n\n你可以调用工具直接执行以下操作：\n\n## 用户管理\n- 创建用户（自动生成 .ovpn 客户端配置 + 发送开通邮件，与页面流程完全一致）\n- 列出用户、更新用户（启用/禁用、设有效期、固定IP）、删除用户\n- 重置密码、重置 MFA、绑定角色\n\n## VPN 客户端管理\n- 列出所有客户端配置、删除客户端（吊销证书）\n- 更新 CCD 配置（设置固定IP、推送路由）\n- 重新生成客户端配置、生成新客户端\n- 查看在线客户端、断开连接\n\n## 防火墙管理\n- 列出防火墙规则、拉黑/解黑 IP、设置/移除限速\n\n## 证书管理\n- 查看 CA 证书、CRL 吊销列表、已签发客户端证书的状态和有效期\n\n## 通知渠道管理\n- 列出/创建/更新/删除通知渠道（邮件、Webhook 等）\n\n## 审计与监控\n- 查询操作审计日志（按模块、操作类型筛选）\n- 获取系统仪表盘摘要（服务器状态、用户统计、在线数、风险项）\n\n## 绝对重要的使用原则（违反会导致误报）\n1. **禁止幻觉**：不要在工具未实际调用、或工具调用失败时告诉用户\"已执行完成\"。如果不确定工具是否真的执行了，必须先调用工具确认。\n2. **等待工具返回**：每次需要执行操作时，必须真正发出 function_call 并等待工具返回结果，再基于工具的实际返回内容（success 字段、message 字段）回答用户。\n3. **失败必须明示**：工具返回 success=false 或返回 error 时，必须明确告知用户失败原因，不得掩饰为\"已成功\"。\n4. **复合任务必须分别调用**：当用户要求\"先删除再创建\"或\"同时做多件事\"时，对每个动作都要单独调用对应工具；不要因为意图里同时含多个动作就只调用一部分。\n5. **查询用工具**：用户问\"系统有多少用户\"\"当前谁在线\"等问题，必须先调用工具获取实时数据，不要凭印象回答。\n6. **执行敏感操作前简要说明**：删除用户、断开连接等动作前先一句话告知用户。\n7. **权限不足直接告知**：工具返回权限不足时，直接告诉用户需要相应权限，不要反复重试。\n8. **用简洁专业的中文回答**"

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
		SystemPrompt: defaultAISystemPrompt,
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

// MigrateAISettings initializes the SQLite-backed AI settings schema.
// AI settings are database-only; legacy configuration values are deliberately ignored.
func MigrateAISettings(database *gorm.DB) error {
	if err := database.AutoMigrate(&AISettings{}, &AIProviderProfile{}); err != nil {
		return err
	}

	return database.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&AISettings{}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			// Existing SQLite settings are authoritative and must never be overwritten.
			return nil
		}

		profile := defaultAIProfile(AIProviderOllama)
		if err := tx.Create(&profile).Error; err != nil {
			return err
		}
		return tx.Create(&AISettings{
			ID:             1,
			Enabled:        false,
			ActiveProvider: AIProviderOllama,
		}).Error
	})
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
