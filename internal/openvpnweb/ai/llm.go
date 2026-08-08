package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
)

// LLMConfig LLM 客户端初始化配置
type LLMConfig struct {
	Provider    string  // ollama | openai | deepseek | customize
	BaseURL     string  // 服务地址
	APIKey      string  // API 密钥（OpenAI 兼容接口使用）
	Model       string  // 模型名称
	MaxTokens   int     // 最大生成 token 数
	Temperature float64 // 温度参数
}

// LLMClient 多 Provider LLM 客户端，支持 Ollama 和 OpenAI 兼容接口
// 可动态切换 provider，线程安全
type LLMClient struct {
	mu          sync.RWMutex
	provider    string
	ollamaLLM   llms.Model
	httpClient  *http.Client
	baseURL     string
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
}

// NewLLMClient 根据配置创建 LLM 客户端
func NewLLMClient(cfg LLMConfig) (*LLMClient, error) {
	c := &LLMClient{
		provider:    cfg.Provider,
		httpClient:  &http.Client{Timeout: 120 * time.Second},
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:      cfg.APIKey,
		model:       cfg.Model,
		maxTokens:   cfg.MaxTokens,
		temperature: cfg.Temperature,
	}

	if err := c.initProvider(); err != nil {
		return nil, err
	}
	return c, nil
}

// initProvider 初始化底层 provider
func (c *LLMClient) initProvider() error {
	switch c.provider {
	case "ollama":
		llm, err := ollama.New(
			ollama.WithModel(c.model),
			ollama.WithServerURL(c.baseURL),
		)
		if err != nil {
			return fmt.Errorf("初始化 Ollama 客户端失败: %w", err)
		}
		c.ollamaLLM = llm
	case "openai", "deepseek", "customize":
		// OpenAI 兼容接口通过 HTTP 调用，无需预初始化
		if c.baseURL == "" {
			return fmt.Errorf("BaseURL 不能为空")
		}
		if c.model == "" {
			return fmt.Errorf("模型名称不能为空")
		}
	default:
		return fmt.Errorf("不支持的 provider: %s", c.provider)
	}
	return nil
}

// Reconfigure 动态切换 provider 配置（线程安全）
func (c *LLMClient) Reconfigure(cfg LLMConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.provider = cfg.Provider
	c.baseURL = strings.TrimRight(cfg.BaseURL, "/")
	c.apiKey = cfg.APIKey
	c.model = cfg.Model
	c.maxTokens = cfg.MaxTokens
	c.temperature = cfg.Temperature
	c.ollamaLLM = nil // 清空旧客户端，触发重新初始化

	return c.initProvider()
}

// Provider 返回当前 provider 类型
func (c *LLMClient) Provider() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.provider
}

// Model 返回当前模型名称
func (c *LLMClient) Model() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.model
}

// IsOpenAICompatible 判断是否为 OpenAI 兼容 provider
func (c *LLMClient) isOpenAICompatible() bool {
	return c.provider == "openai" || c.provider == "deepseek" || c.provider == "customize"
}

// Generate 非流式对话生成
func (c *LLMClient) Generate(ctx context.Context,
	messages []llms.MessageContent) (*llms.ContentResponse, error) {

	c.mu.RLock()
	provider := c.provider
	c.mu.RUnlock()

	if provider == "ollama" {
		c.mu.RLock()
		llm := c.ollamaLLM
		c.mu.RUnlock()
		resp, err := llm.GenerateContent(ctx, messages)
		if err != nil {
			return nil, fmt.Errorf("LLM 生成失败: %w", err)
		}
		return resp, nil
	}

	return c.openAICompatibleGenerate(ctx, messages, false, nil)
}

// GenerateStream 流式对话生成
func (c *LLMClient) GenerateStream(ctx context.Context,
	messages []llms.MessageContent,
	onToken func(token string) error) (*llms.ContentResponse, error) {

	c.mu.RLock()
	provider := c.provider
	c.mu.RUnlock()

	if provider == "ollama" {
		c.mu.RLock()
		llm := c.ollamaLLM
		c.mu.RUnlock()
		return llm.GenerateContent(ctx, messages,
			llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
				return onToken(string(chunk))
			}),
		)
	}

	return c.openAICompatibleGenerate(ctx, messages, true, onToken)
}

// openAIChatReq OpenAI 兼容接口请求体
type openAIChatReq struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Stream      bool            `json:"stream"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatResp 非流式响应
type openAIChatResp struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
}

// openAIStreamChunk 流式响应分块
type openAIStreamChunk struct {
	Choices []struct {
		Delta openAIMessage `json:"delta"`
	} `json:"choices"`
}

// messageContentToOpenAI 将 langchaingo MessageContent 转为 OpenAI 格式
func messageContentToOpenAI(messages []llms.MessageContent) []openAIMessage {
	result := make([]openAIMessage, 0, len(messages))
	for _, msg := range messages {
		role := string(msg.Role)
		content := serializeContent(msg.Parts)
		if role == "" {
			role = "user"
		}
		// 标准化角色名
		switch strings.ToLower(role) {
		case "human":
			role = "user"
		case "ai", "assistant":
			role = "assistant"
		}
		result = append(result, openAIMessage{Role: role, Content: content})
	}
	return result
}

// serializeContent 将 llms.ContentPart 序列化为纯文本
func serializeContent(parts []llms.ContentPart) string {
	var buf strings.Builder
	for _, p := range parts {
		switch v := p.(type) {
		case llms.TextContent:
			buf.WriteString(v.Text)
		case llms.BinaryContent:
			buf.WriteString("[Binary Content]")
		default:
			buf.WriteString(fmt.Sprint(v))
		}
	}
	return buf.String()
}

// openAICompatibleGenerate OpenAI 兼容接口调用
func (c *LLMClient) openAICompatibleGenerate(ctx context.Context,
	messages []llms.MessageContent,
	stream bool,
	onToken func(token string) error) (*llms.ContentResponse, error) {

	c.mu.RLock()
	baseURL := c.baseURL
	apiKey := c.apiKey
	model := c.model
	maxTokens := c.maxTokens
	temperature := c.temperature
	c.mu.RUnlock()

	payload := openAIChatReq{
		Model:       model,
		Messages:    messageContentToOpenAI(messages),
		Stream:      stream,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	url := baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	c.mu.RLock()
	httpClient := c.httpClient
	c.mu.RUnlock()

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API 返回错误 (status=%d): %s", resp.StatusCode, string(respBytes))
	}

	if !stream {
		return parseNonStreamResponse(resp.Body)
	}
	return parseStreamResponse(resp.Body, onToken)
}

// parseNonStreamResponse 解析非流式响应
func parseNonStreamResponse(body io.Reader) (*llms.ContentResponse, error) {
	var resp openAIChatResp
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if len(resp.Choices) == 0 {
		return &llms.ContentResponse{}, nil
	}
	return &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{
				Content: resp.Choices[0].Message.Content,
			},
		},
	}, nil
}

// parseStreamResponse 解析 SSE 流式响应
func parseStreamResponse(body io.Reader, onToken func(token string) error) (*llms.ContentResponse, error) {
	scanner := bufio.NewScanner(body)
	var fullContent strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// SSE 格式: "data: {...}"
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

		// 流结束标记
		if data == "[DONE]" {
			break
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // 忽略解析错误的分块
		}

		for _, choice := range chunk.Choices {
			token := choice.Delta.Content
			if token == "" {
				continue
			}
			fullContent.WriteString(token)
			if onToken != nil {
				if err := onToken(token); err != nil {
					return nil, err
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取流式响应失败: %w", err)
	}

	return &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{Content: fullContent.String()},
		},
	}, nil
}

// AtomicClient LLMClient 的原子引用包装，支持运行时热替换底层 provider
type AtomicClient struct {
	value atomic.Value // 存储 *LLMClient
}

// NewAtomicClient 创建原子引用
func NewAtomicClient(client *LLMClient) *AtomicClient {
	ac := &AtomicClient{}
	if client != nil {
		ac.value.Store(client)
	}
	return ac
}

// Get 获取当前 LLMClient（线程安全）
func (ac *AtomicClient) Get() *LLMClient {
	v := ac.value.Load()
	if v == nil {
		return nil
	}
	return v.(*LLMClient)
}

// Set 替换 LLMClient（线程安全，热切换）
func (ac *AtomicClient) Set(client *LLMClient) {
	ac.value.Store(client)
}

// Provider 返回当前 provider 名称
func (ac *AtomicClient) Provider() string {
	client := ac.Get()
	if client == nil {
		return ""
	}
	return client.Provider()
}

// Model 返回当前模型名称
func (ac *AtomicClient) Model() string {
	client := ac.Get()
	if client == nil {
		return ""
	}
	return client.Model()
}
