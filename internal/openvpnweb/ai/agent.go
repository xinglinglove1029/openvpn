package ai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"iter"
	"strings"
	"time"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

const (
	// AgentAppName ADK 应用名（用于 session 隔离）
	AgentAppName = "openvpn-ai-assistant"
	// AgentName 根 Agent 名称
	AgentName = "ai_assistant"
	// MaxContextMessages 上下文最大消息数（保留最近 N 条，避免 token 爆炸）
	MaxContextMessages = 20
	// SessionIdleTimeout 会话空闲超时
	SessionIdleTimeout = 30 * time.Minute
)

// AgentConfig Agent 初始化配置
type AgentConfig struct {
	SystemPrompt string  // 系统提示词
	MaxTokens    int     // 最大生成 token 数
	Temperature  float64 // 温度参数
}

// AgentRunner ADK Agent + Runner 封装
// 持有 ADK 的 Runner 和会话存储，对外暴露 Run 方法接收用户消息并返回事件流。
type AgentRunner struct {
	runner         *runner.Runner
	sessionService session.Service
	systemPrompt   string
}

// NewAgentRunner 创建 ADK Agent + Runner
// llm: 实现 model.LLM 接口的客户端（*OpenAIModel）
// tools: 业务工具集合（可为空，表示纯对话模式）
// cfg: Agent 配置（system prompt、温度等）
func NewAgentRunner(llm *OpenAIModel, tools []tool.Tool, cfg AgentConfig) (*AgentRunner, error) {
	if llm == nil {
		return nil, fmt.Errorf("LLM 客户端不能为空")
	}

	// 构造 GenerateContentConfig
	// 注意：genai.GenerateContentConfig.MaxOutputTokens 为 int32，Temperature 为 *float32
	maxTokens := int32(cfg.MaxTokens)
	temperature := float32(cfg.Temperature)
	genCfg := &genai.GenerateContentConfig{
		MaxOutputTokens: maxTokens,
		Temperature:     &temperature,
	}
	if strings.TrimSpace(cfg.SystemPrompt) != "" {
		genCfg.SystemInstruction = &genai.Content{
			Role:  "system",
			Parts: []*genai.Part{{Text: cfg.SystemPrompt}},
		}
	}

	// 创建 LLMAgent
	agentInst, err := llmagent.New(llmagent.Config{
		Name:                   AgentName,
		Description:            "OpenVPN 智能运维助手，可协助配置、排障、分析，并能调用业务工具完成用户管理任务",
		Model:                  llm,
		Instruction:            cfg.SystemPrompt,
		GenerateContentConfig:  genCfg,
		Tools:                  tools,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 LLMAgent 失败: %w", err)
	}

	// 创建会话存储（内存实现，进程重启后清空）
	sessionSvc := session.InMemoryService()

	// 创建 Runner
	r, err := runner.New(runner.Config{
		AppName:           AgentAppName,
		Agent:             agentInst,
		SessionService:    sessionSvc,
		AutoCreateSession: true, // 首次调用时自动创建会话
	})
	if err != nil {
		return nil, fmt.Errorf("创建 Runner 失败: %w", err)
	}

	return &AgentRunner{
		runner:         r,
		sessionService: sessionSvc,
		systemPrompt:   cfg.SystemPrompt,
	}, nil
}

// Run 执行一次对话推理
// userID: 当前登录用户名（用于会话隔离）
// sessionID: 会话 ID（空则自动生成，通过 EnsureSession 创建）
// message: 用户消息文本
// 返回事件流，调用方需遍历 iter.Seq2 提取文本/工具调用结果
func (a *AgentRunner) Run(ctx context.Context, userID, sessionID, message string) iter.Seq2[*session.Event, error] {
	return a.runner.Run(ctx, userID, sessionID, &genai.Content{
		Role:  "user",
		Parts: []*genai.Part{{Text: message}},
	}, agent.RunConfig{
		StreamingMode: agent.StreamingModeSSE,
	})
}

// EnsureSession 确保会话存在（不存在则创建），返回可用的 sessionID
// 区分 NotFound（正常，创建新会话）与其他错误（返回，避免误创建）
func (a *AgentRunner) EnsureSession(ctx context.Context, userID, sessionID string) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("userID 不能为空")
	}
	if sessionID == "" {
		sessionID = generateSessionID(userID)
	}

	// 尝试获取，不存在则创建（AutoCreateSession=true 时 Runner.Run 会自动创建，
	// 但提前创建可确保 session 事件先返回 session_id 给前端）
	_, err := a.sessionService.Get(ctx, &session.GetRequest{
		AppName:   AgentAppName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err == nil {
		return sessionID, nil
	}

	// 区分 NotFound 与其他错误：NotFound 是预期行为（创建新会话），
	// 其他错误（如网络/存储异常）不应继续创建，直接返回避免掩盖真实问题
	if !isSessionNotFoundError(err) {
		return "", fmt.Errorf("获取会话失败: %w", err)
	}

	_, err = a.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   AgentAppName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return "", fmt.Errorf("创建会话失败: %w", err)
	}
	return sessionID, nil
}

// isSessionNotFoundError 判断错误是否为 session 不存在
// ADK session.Service 的 Get 在 session 不存在时返回 error，
// 通过字符串匹配兼容不同实现（ADK 未导出统一的 NotFound 错误）
func isSessionNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "不存在")
}

// DeleteSession 删除指定会话（用于新会话/清理）
func (a *AgentRunner) DeleteSession(ctx context.Context, userID, sessionID string) error {
	if userID == "" || sessionID == "" {
		return nil
	}
	return a.sessionService.Delete(ctx, &session.DeleteRequest{
		AppName:   AgentAppName,
		UserID:    userID,
		SessionID: sessionID,
	})
}

// ListSessions 列出指定用户的所有会话
func (a *AgentRunner) ListSessions(ctx context.Context, userID string) ([]session.Session, error) {
	resp, err := a.sessionService.List(ctx, &session.ListRequest{
		AppName: AgentAppName,
		UserID:  userID,
	})
	if err != nil {
		return nil, err
	}
	return resp.Sessions, nil
}

// SystemPrompt 返回当前系统提示词
func (a *AgentRunner) SystemPrompt() string {
	return a.systemPrompt
}

// ExtractEventText 从事件中提取纯文本内容（合并所有 Text parts）
func ExtractEventText(event *session.Event) string {
	if event == nil || event.Content == nil {
		return ""
	}
	var sb strings.Builder
	for _, part := range event.Content.Parts {
		if part.Text != "" {
			sb.WriteString(part.Text)
		}
	}
	return sb.String()
}

// HasToolCall 判断事件是否包含工具调用（用于前端展示"正在执行操作..."）
func HasToolCall(event *session.Event) bool {
	if event == nil || event.Content == nil {
		return false
	}
	for _, part := range event.Content.Parts {
		if part.FunctionCall != nil {
			return true
		}
	}
	return false
}

// HasToolResponse 判断事件是否包含工具响应
func HasToolResponse(event *session.Event) bool {
	if event == nil || event.Content == nil {
		return false
	}
	for _, part := range event.Content.Parts {
		if part.FunctionResponse != nil {
			return true
		}
	}
	return false
}

// generateSessionID 生成带随机后缀的会话 ID，避免秒级冲突导致"新会话"复用旧会话
// 格式: username_20060102_150405_<8位hex随机>
func generateSessionID(username string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b) // crypto/rand.Read 极少失败，忽略错误降级为零字节
	return username + "_" + time.Now().Format("20060102_150405") + "_" + hex.EncodeToString(b)
}
