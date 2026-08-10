package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// LLMConfig LLM 客户端初始化配置
type LLMConfig struct {
	Provider    string  // ollama | openai | deepseek | customize
	BaseURL     string  // 服务地址
	APIKey      string  // API 密钥
	Model       string  // 模型名称
	MaxTokens   int     // 最大生成 token 数
	Temperature float64 // 温度参数
}

// OpenAIModel 实现 adk model.LLM 接口，适配 OpenAI 兼容 API（DeepSeek/OpenAI/Ollama v1 API）
type OpenAIModel struct {
	mu          sync.RWMutex
	provider    string
	httpClient  *http.Client
	baseURL     string
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
}

// NewLLMClient 根据配置创建 LLM 客户端（返回实现 model.LLM 接口的实例）
func NewLLMClient(cfg LLMConfig) (*OpenAIModel, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("BaseURL 不能为空")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("模型名称不能为空")
	}
	// 校验 MaxTokens 范围（允许 0 表示不限制，负数拒绝，超出 int32 拒绝）
	if cfg.MaxTokens < 0 {
		return nil, fmt.Errorf("MaxTokens 不能为负数: %d", cfg.MaxTokens)
	}
	if cfg.MaxTokens > math.MaxInt32 {
		return nil, fmt.Errorf("MaxTokens 超出 int32 范围: %d", cfg.MaxTokens)
	}
	// 校验 Temperature 范围（OpenAI 规范 0~2）
	if cfg.Temperature < 0 || cfg.Temperature > 2 {
		return nil, fmt.Errorf("Temperature 超出范围 [0,2]: %f", cfg.Temperature)
	}

	baseURL, err := NormalizeBaseURL(cfg.Provider, cfg.BaseURL)
	if err != nil {
		return nil, err
	}

	c := &OpenAIModel{
		provider: cfg.Provider,
		// httpClient 不设置整体 Timeout，依赖调用方 context 控制总时长
		// （流式响应可能持续较长时间，整体 Timeout 会截断流）
		httpClient:  &http.Client{},
		baseURL:     baseURL,
		apiKey:      cfg.APIKey,
		model:       cfg.Model,
		maxTokens:   cfg.MaxTokens,
		temperature: cfg.Temperature,
	}
	return c, nil
}

// NormalizeBaseURL keeps each provider on its OpenAI-compatible API root.
// Ollama serves that compatibility API at /v1, while older configurations often omit it.
func NormalizeBaseURL(provider, rawURL string) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("Base URL is required")
	}
	if !strings.EqualFold(strings.TrimSpace(provider), "ollama") {
		return baseURL, nil
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("Invalid Base URL: %q", rawURL)
	}
	// Containers commonly bind Ollama to IPv4 only; avoid localhost resolving to ::1.
	if parsed.Hostname() == "localhost" && parsed.Port() == "11434" {
		parsed.Host = "127.0.0.1:11434"
	}
	// Preserve an explicit prefix, but turn a bare Ollama host into /v1.
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/v1"
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// Provider 返回当前 provider 类型
func (c *OpenAIModel) Provider() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.provider
}

// Model 返回当前模型名称
func (c *OpenAIModel) Model() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.model
}

// Name 实现 model.LLM 接口
func (c *OpenAIModel) Name() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.model
}

// GenerateContent 实现 model.LLM 接口
// 将 genai.Content 转为 OpenAI 格式，调用 API，返回 iter.Seq2 流
func (c *OpenAIModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		// iter.Seq 的消费者提前退出时 yield 会返回 false。必须立即停止生产，
		// 否则 Go runtime 会触发 “range function continued iteration” panic。
		emit := func(response *model.LLMResponse, err error) bool {
			return yield(response, err)
		}

		c.mu.RLock()
		provider := c.provider
		baseURL := c.baseURL
		apiKey := c.apiKey
		modelName := c.model
		maxTokens := c.maxTokens
		temperature := c.temperature
		httpClient := c.httpClient
		c.mu.RUnlock()

		// 构建 OpenAI 请求
		messages := contentsToOpenAIMessages(req.Contents, req.Config)
		payload := openAIChatReq{
			Model:       modelName,
			Messages:    messages,
			Stream:      stream,
			MaxTokens:   maxTokens,
			Temperature: temperature,
		}

		// 从 Config.Tools 提取 function declarations
		if req.Config != nil && len(req.Config.Tools) > 0 {
			payload.Tools = extractOpenAITools(req.Config.Tools)
			payload.ToolChoice = "auto"
		}

		// Some small Ollama models served through OpenAI-compatible endpoints emit a
		// textual JSON tool call instead of the standard tool_calls field. Keep an
		// allow-list from this request and normalize that format below.
		allowedTextToolNames := make(map[string]struct{}, len(payload.Tools))
		for _, toolDef := range payload.Tools {
			allowedTextToolNames[toolDef.Function.Name] = struct{}{}
		}
		allowTextToolFallback := strings.EqualFold(provider, "ollama") && len(allowedTextToolNames) > 0

		// system instruction
		if req.Config != nil && req.Config.SystemInstruction != nil {
			sysText := contentToText(req.Config.SystemInstruction)
			if sysText != "" {
				payload.Messages = append([]openAIMessage{
					{Role: "system", Content: sysText},
				}, payload.Messages...)
			}
		}

		body, err := json.Marshal(payload)
		if err != nil {
			emit(nil, fmt.Errorf("序列化请求失败: %w", err))
			return
		}

		url := baseURL + "/chat/completions"
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			emit(nil, fmt.Errorf("创建请求失败: %w", err))
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		}

		resp, err := httpClient.Do(httpReq)
		if err != nil {
			emit(nil, fmt.Errorf("请求 API 失败: %w", err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBytes, _ := io.ReadAll(resp.Body)
			emit(nil, fmt.Errorf("API 返回错误 (status=%d): %s", resp.StatusCode, sanitizeAPIErrorBody(string(respBytes))))
			return
		}

		if !stream {
			// 非流式：解析完整响应
			llmResp, err := parseNonStreamOpenAIResponse(resp.Body, allowTextToolFallback, allowedTextToolNames)
			if err != nil {
				emit(nil, err)
				return
			}
			emit(llmResp, nil)
			return
		}

		// 流式：逐块解析并 yield
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 支持长行
		var fullText strings.Builder
		// tool_calls 跨 chunk 累积器：按 index 累积 id/name/arguments 片段
		toolCallAccum := make(map[int]*toolCallAccumulator)
		hasNativeToolCalls := false

		flushToolCalls := func() bool {
			// 按 index 顺序组装完整的 FunctionCall 并 yield
			if len(toolCallAccum) == 0 {
				return true
			}
			// 收集并排序 index
			indices := make([]int, 0, len(toolCallAccum))
			for idx := range toolCallAccum {
				indices = append(indices, idx)
			}
			sort.Ints(indices)

			content := &genai.Content{Role: "model"}
			for _, idx := range indices {
				acc := toolCallAccum[idx]
				if acc.name == "" {
					continue // 未收到 name，跳过残缺调用
				}
				var args map[string]any
				if acc.arguments != "" {
					if err := json.Unmarshal([]byte(acc.arguments), &args); err != nil {
						log.Printf("⚠ 流式 tool_call arguments 解析失败 (tool=%s): %v, raw=%s", acc.name, err, acc.arguments)
						// 保留原始字符串作为 args，让工具层处理
						args = map[string]any{"_raw": acc.arguments}
					}
				}
				content.Parts = append(content.Parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						ID:   acc.id,
						Name: acc.name,
						Args: args,
					},
				})
			}
			// 清空累积器
			toolCallAccum = make(map[int]*toolCallAccumulator)
			if len(content.Parts) > 0 && !emit(&model.LLMResponse{
				Content:      content,
				TurnComplete: true,
			}, nil) {
				return false
			}
			return true
		}

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				// Flush remaining native tool_calls; stop if the consumer has exited.
				if !flushToolCalls() {
					return
				}
				// Some Ollama models emit function calls as ordinary JSON text.
				// Convert only complete allow-listed JSON so it never reaches the chat UI.
				if allowTextToolFallback && !hasNativeToolCalls {
					if toolCallResp, ok := textToolCallResponse(fullText.String(), allowedTextToolNames); ok {
						emit(toolCallResp, nil)
						return
					}
				}
				// Final event.
				emit(&model.LLMResponse{
					Content: &genai.Content{
						Role:  "model",
						Parts: []*genai.Part{{Text: fullText.String()}},
					},
					TurnComplete: true,
				}, nil)
				return
			}

			var chunk openAIStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				log.Printf("⚠ 解析 SSE chunk 失败: %v, data=%s", err, data)
				continue
			}

			for _, choice := range chunk.Choices {
				// 处理文本 token
				if choice.Delta.Content != "" {
					fullText.WriteString(choice.Delta.Content)
					// Buffer Ollama textual fallback candidates so their raw JSON never
					// reaches the chat UI before it can be converted into a FunctionCall.
					if !allowTextToolFallback && !emit(&model.LLMResponse{
						Content: &genai.Content{
							Role:  "model",
							Parts: []*genai.Part{{Text: choice.Delta.Content}},
						},
						Partial: true,
					}, nil) {
						return
					}
				}

				// 处理 tool_calls：按 index 累积
				for _, tc := range choice.Delta.ToolCalls {
					hasNativeToolCalls = true
					acc, ok := toolCallAccum[tc.Index]
					if !ok {
						acc = &toolCallAccumulator{}
						toolCallAccum[tc.Index] = acc
					}
					if tc.ID != "" {
						acc.id = tc.ID
					}
					if tc.Function.Name != "" {
						acc.name = tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						acc.arguments += tc.Function.Arguments
					}
				}

				// finish_reason=tool_calls 时 flush 累积的 tool_calls
				if choice.FinishReason == "tool_calls" && !flushToolCalls() {
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			emit(nil, fmt.Errorf("读取流式响应失败: %w", err))
			return
		}

		// 如果没有收到 [DONE] 但有内容，也发送完成事件。
		if !flushToolCalls() {
			return
		}
		if fullText.Len() > 0 {
			if allowTextToolFallback && !hasNativeToolCalls {
				if toolCallResp, ok := textToolCallResponse(fullText.String(), allowedTextToolNames); ok {
					emit(toolCallResp, nil)
					return
				}
			}
			emit(&model.LLMResponse{
				Content: &genai.Content{
					Role:  "model",
					Parts: []*genai.Part{{Text: fullText.String()}},
				},
				TurnComplete: true,
			}, nil)
		}
	}
}

// toolCallAccumulator 流式 tool_call 跨 chunk 累积器
type toolCallAccumulator struct {
	id        string
	name      string
	arguments string
}

// sanitizeAPIErrorBody 对 API 错误响应体进行脱敏和截断
func sanitizeAPIErrorBody(body string) string {
	if len(body) > 200 {
		body = body[:200] + "..."
	}
	// 脱敏 sk- 开头的 API Key
	for _, prefix := range []string{"sk-", "Bearer "} {
		for {
			idx := strings.Index(strings.ToLower(body), strings.ToLower(prefix))
			if idx < 0 {
				break
			}
			end := idx + len(prefix) + 20
			if end > len(body) {
				end = len(body)
			}
			body = body[:idx] + prefix + "***" + body[end:]
		}
	}
	return body
}

// --- OpenAI 请求/响应结构体 ---

type openAIChatReq struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Stream      bool            `json:"stream"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature"`
	Tools       []openAITool    `json:"tools,omitempty"`
	ToolChoice  any             `json:"tool_choice,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type openAIToolCall struct {
	Index    int              `json:"index"`
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function openAIToolCallFn `json:"function"`
}

type openAIToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatResp struct {
	Choices []struct {
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string           `json:"content"`
			Role      string           `json:"role"`
			ToolCalls []openAIToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// --- 转换辅助函数 ---

// contentsToOpenAIMessages 将 genai.Content 列表转为 OpenAI 消息格式
// 注意：FunctionResponse 单独作为一条 tool 消息，避免覆盖同 Content 中的 Text/FunctionCall
func contentsToOpenAIMessages(contents []*genai.Content, cfg *genai.GenerateContentConfig) []openAIMessage {
	result := make([]openAIMessage, 0, len(contents))
	for _, content := range contents {
		msg := openAIMessage{Role: normalizeRole(content.Role)}
		var textParts []string
		var toolCalls []openAIToolCall
		var funcResponses []openAIToolCall // 仅用于判断是否需要拆分

		for _, part := range content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
			if part.FunctionCall != nil {
				args, _ := json.Marshal(part.FunctionCall.Args)
				toolCalls = append(toolCalls, openAIToolCall{
					ID:   part.FunctionCall.ID,
					Type: "function",
					Function: openAIToolCallFn{
						Name:      part.FunctionCall.Name,
						Arguments: string(args),
					},
				})
			}
			if part.FunctionResponse != nil {
				// function response 单独作为 tool 消息，避免覆盖 Text/FunctionCall
				respData, _ := json.Marshal(part.FunctionResponse.Response)
				result = append(result, openAIMessage{
					Role:       "tool",
					ToolCallID: part.FunctionResponse.ID,
					Content:    string(respData),
				})
				funcResponses = append(funcResponses, openAIToolCall{}) // 标记已处理
			}
		}

		// 仅当该 content 中无 FunctionResponse 时才追加 Text/FunctionCall 消息
		// （FunctionResponse 已单独作为 tool 消息追加）
		if len(funcResponses) == 0 {
			msg.Content = strings.Join(textParts, "\n")
			if len(toolCalls) > 0 {
				msg.ToolCalls = toolCalls
			}
			result = append(result, msg)
		} else if len(textParts) > 0 || len(toolCalls) > 0 {
			// 同时含 FunctionResponse 和 Text/FunctionCall 的混合情况
			// 将 Text/FunctionCall 作为单独的 assistant 消息追加
			msg.Content = strings.Join(textParts, "\n")
			if len(toolCalls) > 0 {
				msg.ToolCalls = toolCalls
			}
			result = append(result, msg)
		}
	}
	return result
}

func normalizeRole(role string) string {
	switch strings.ToLower(role) {
	case "model", "ai", "assistant":
		return "assistant"
	case "human", "user":
		return "user"
	case "system":
		return "system"
	default:
		if role == "" {
			return "user"
		}
		return role
	}
}

func contentToText(content *genai.Content) string {
	if content == nil {
		return ""
	}
	var parts []string
	for _, p := range content.Parts {
		if p.Text != "" {
			parts = append(parts, p.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// extractOpenAITools 从 genai.Tools 提取 OpenAI 格式的工具定义
func extractOpenAITools(tools []*genai.Tool) []openAITool {
	result := make([]openAITool, 0)
	for _, t := range tools {
		for _, fd := range t.FunctionDeclarations {
			params := fd.ParametersJsonSchema
			if params == nil {
				// 默认空 schema
				params = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			result = append(result, openAITool{
				Type: "function",
				Function: openAIToolFunction{
					Name:        fd.Name,
					Description: fd.Description,
					Parameters:  params,
				},
			})
		}
	}
	return result
}

// parseNonStreamOpenAIResponse parses a non-streaming response.
// Textual tool-call fallback is enabled only for Ollama.
func parseNonStreamOpenAIResponse(body io.Reader, allowTextToolFallback bool, allowedTextToolNames map[string]struct{}) (*model.LLMResponse, error) {
	var resp openAIChatResp
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return &model.LLMResponse{
			Content:      &genai.Content{Role: "model"},
			TurnComplete: true,
		}, nil
	}

	choice := resp.Choices[0]
	// Native tool_calls take priority for every provider. The text fallback is
	// deliberately limited to Ollama and only runs when no native call exists.
	if len(choice.Message.ToolCalls) == 0 && allowTextToolFallback {
		if toolCallResp, ok := textToolCallResponse(choice.Message.Content, allowedTextToolNames); ok {
			return toolCallResp, nil
		}
	}

	content := &genai.Content{Role: "model"}
	if choice.Message.Content != "" {
		content.Parts = append(content.Parts, &genai.Part{Text: choice.Message.Content})
	}

	// Preserve native tool_calls from all OpenAI-compatible providers.
	for _, tc := range choice.Message.ToolCalls {
		var args map[string]any
		if tc.Function.Arguments != "" {
			json.Unmarshal([]byte(tc.Function.Arguments), &args)
		}
		content.Parts = append(content.Parts, &genai.Part{
			FunctionCall: &genai.FunctionCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: args,
			},
		})
	}

	return &model.LLMResponse{
		Content:      content,
		TurnComplete: true,
	}, nil
}

// textToolCallResponse converts the JSON shape emitted as ordinary text by some
// small Ollama models into an ADK FunctionCall. It only accepts a complete,
// single JSON object (optionally wrapped by a pure Markdown code fence) whose
// tool name was advertised in this request.
func textToolCallResponse(text string, allowedToolNames map[string]struct{}) (*model.LLMResponse, bool) {
	if len(allowedToolNames) == 0 {
		return nil, false
	}

	payload, ok := unwrapTextToolCallPayload(text)
	if !ok || !strings.HasPrefix(payload, "{") {
		return nil, false
	}

	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.UseNumber()
	var raw struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := decoder.Decode(&raw); err != nil || raw.Name == "" {
		return nil, false
	}
	if _, allowed := allowedToolNames[raw.Name]; !allowed {
		return nil, false
	}
	// InputOffset is relative to payload and verifies that a second JSON object,
	// natural-language suffix, or any other trailing content cannot be ignored.
	if strings.TrimSpace(payload[decoder.InputOffset():]) != "" {
		return nil, false
	}

	args := map[string]any{}
	if len(raw.Arguments) > 0 && string(raw.Arguments) != "null" {
		if err := json.Unmarshal(raw.Arguments, &args); err != nil {
			var encoded string
			if json.Unmarshal(raw.Arguments, &encoded) != nil || json.Unmarshal([]byte(encoded), &args) != nil {
				return nil, false
			}
		}
	}

	return &model.LLMResponse{
		Content: &genai.Content{Role: "model", Parts: []*genai.Part{{
			FunctionCall: &genai.FunctionCall{ID: "text-tool-call-0", Name: raw.Name, Args: args},
		}}},
		TurnComplete: true,
	}, true
}

// unwrapTextToolCallPayload permits whitespace around raw JSON, or a code fence
// containing only the JSON. Any explanatory text outside/inside the JSON remains
// model text rather than becoming an executable tool call.
func unwrapTextToolCallPayload(text string) (string, bool) {
	payload := strings.TrimSpace(text)
	if !strings.HasPrefix(payload, "```") {
		return payload, true
	}

	newline := strings.IndexByte(payload, '\n')
	if newline < 0 {
		return "", false
	}
	openingFence := strings.TrimSpace(payload[:newline])
	if !strings.HasPrefix(openingFence, "```") || strings.Contains(openingFence[3:], "`") {
		return "", false
	}

	remaining := strings.TrimSpace(payload[newline+1:])
	if !strings.HasSuffix(remaining, "```") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimSuffix(remaining, "```")), true
}

// --- AtomicClient 原子引用包装 ---

// AtomicClient model.LLM 的原子引用包装，支持运行时热替换
type AtomicClient struct {
	value atomic.Value // 存储 *OpenAIModel
}

// NewAtomicClient 创建原子引用
func NewAtomicClient(client *OpenAIModel) *AtomicClient {
	ac := &AtomicClient{}
	if client != nil {
		ac.value.Store(client)
	}
	return ac
}

// Get 获取当前 model.LLM（线程安全）
func (ac *AtomicClient) Get() *OpenAIModel {
	v := ac.value.Load()
	if v == nil {
		return nil
	}
	return v.(*OpenAIModel)
}

// Set 替换 model.LLM（线程安全，热切换）
func (ac *AtomicClient) Set(client *OpenAIModel) {
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

// Ping checks provider reachability. Ollama uses its native model-list endpoint so health checks never load or infer a model.
func (c *OpenAIModel) Ping(ctx context.Context) error {
	c.mu.RLock()
	provider := c.provider
	baseURL := c.baseURL
	apiKey := c.apiKey
	modelName := c.model
	httpClient := c.httpClient
	c.mu.RUnlock()

	if strings.EqualFold(provider, "ollama") {
		return pingOllamaTags(ctx, httpClient, baseURL, modelName)
	}

	payload := openAIChatReq{
		Model: modelName,
		Messages: []openAIMessage{
			{Role: "user", Content: "ping"},
		},
		MaxTokens: 1,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal ping request: %w", err)
	}

	endpoint := baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create ping request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned an error (status=%d): %s", resp.StatusCode, sanitizeAPIErrorBody(string(respBytes)))
	}
	return nil
}

type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func pingOllamaTags(ctx context.Context, httpClient *http.Client, baseURL, modelName string) error {
	endpoint, err := ollamaTagsEndpoint(baseURL)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create Ollama tags request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request Ollama tags: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Ollama tags returned an error (status=%d): %s", resp.StatusCode, sanitizeAPIErrorBody(string(respBytes)))
	}

	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return fmt.Errorf("decode Ollama tags: %w", err)
	}
	for _, model := range tags.Models {
		if normalizeOllamaModelName(model.Name) == normalizeOllamaModelName(modelName) {
			return nil
		}
	}
	return fmt.Errorf("Ollama model %q is not installed; run: ollama pull %s", modelName, modelName)
}

// normalizeOllamaModelName aligns Ollama's implicit latest tag with the explicit
// name returned by /api/tags. Explicit tags and digest references remain unchanged.
func normalizeOllamaModelName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "@") {
		return name
	}
	lastSlash := strings.LastIndex(name, "/")
	if strings.LastIndex(name, ":") <= lastSlash {
		return name + ":latest"
	}
	return name
}

func ollamaTagsEndpoint(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid Ollama base URL: %q", baseURL)
	}
	path := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(path, "/v1") {
		path = strings.TrimSuffix(path, "/v1")
	}
	parsed.Path = strings.TrimRight(path, "/") + "/api/tags"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
