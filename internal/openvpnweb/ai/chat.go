package ai

import (
	"context"
	"sync"
	"time"

	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// ChatSessionManager 轻量会话管理器
// 负责生成 sessionID、追踪活跃会话、从 ADK session.Service 提取历史消息。
// 实际会话存储与上下文管理由 ADK 的 session.Service（在 AgentRunner 内部）负责。
type ChatSessionManager struct {
	mu          sync.RWMutex
	activeUsers map[string]time.Time // username → 最后活跃时间（用于清理统计）
	agentRunner *AgentRunner         // 用于读取 session events
}

// NewChatSessionManager 创建会话管理器
// agentRunner 可为 nil（此时 History 接口返回空列表）；启动后通过 SetAgentRunner 注入。
func NewChatSessionManager(agentRunner *AgentRunner) *ChatSessionManager {
	return &ChatSessionManager{
		activeUsers: make(map[string]time.Time),
		agentRunner: agentRunner,
	}
}

// SetAgentRunner 注入或更新 AgentRunner（用于热切换后重新绑定）
func (m *ChatSessionManager) SetAgentRunner(r *AgentRunner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agentRunner = r
}

// GetAgentRunner 获取当前 AgentRunner（线程安全）
func (m *ChatSessionManager) GetAgentRunner() *AgentRunner {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.agentRunner
}

// EnsureSession 确保会话存在并标记用户活跃
func (m *ChatSessionManager) EnsureSession(ctx context.Context, username, sessionID string) (string, error) {
	m.mu.Lock()
	m.activeUsers[username] = time.Now()
	runner := m.agentRunner
	m.mu.Unlock()

	if runner == nil {
		// AgentRunner 未就绪时仅生成 sessionID，不实际创建 ADK session
		if sessionID == "" {
			sessionID = generateSessionID(username)
		}
		return sessionID, nil
	}
	return runner.EnsureSession(ctx, username, sessionID)
}

// GetHistory 从 ADK session 提取历史消息
// 返回 user/assistant 角色的文本消息列表（过滤工具调用中间事件）
func (m *ChatSessionManager) GetHistory(ctx context.Context, username, sessionID string) ([]HistoryMessage, error) {
	m.mu.RLock()
	runner := m.agentRunner
	m.mu.RUnlock()

	if runner == nil || sessionID == "" {
		return []HistoryMessage{}, nil
	}

	resp, err := runner.sessionService.Get(ctx, &session.GetRequest{
		AppName: AgentAppName,
		UserID:  username,
		SessionID: sessionID,
	})
	if err != nil {
		return []HistoryMessage{}, nil // 会话不存在时返回空列表
	}

	events := resp.Session.Events()
	if events == nil {
		return []HistoryMessage{}, nil
	}
	msgs := make([]HistoryMessage, 0, events.Len())
	for event := range events.All() {
		// 跳过工具调用/响应事件、部分事件
		if HasToolCall(event) || HasToolResponse(event) {
			continue
		}
		if event.Partial {
			continue
		}
		text := ExtractEventText(event)
		if text == "" {
			continue
		}
		role := "assistant"
		if event.Author == "user" {
			role = "user"
		}
		msgs = append(msgs, HistoryMessage{
			Role:    role,
			Content: text,
		})
	}
	return msgs, nil
}

// DeleteSession 删除会话（用于"新会话"功能）
func (m *ChatSessionManager) DeleteSession(ctx context.Context, username, sessionID string) error {
	m.mu.RLock()
	runner := m.agentRunner
	m.mu.RUnlock()
	if runner == nil || sessionID == "" {
		return nil
	}
	return runner.DeleteSession(ctx, username, sessionID)
}

// CleanupIdle 清理空闲用户记录并同步删除其 ADK session，避免内存泄漏
// 返回清理的用户数
func (m *ChatSessionManager) CleanupIdle(ctx context.Context, maxIdle time.Duration) int {
	m.mu.Lock()
	expiredUsers := make([]string, 0)
	now := time.Now()
	for user, lastActive := range m.activeUsers {
		if now.Sub(lastActive) > maxIdle {
			expiredUsers = append(expiredUsers, user)
			delete(m.activeUsers, user)
		}
	}
	runner := m.agentRunner
	m.mu.Unlock()

	cleaned := len(expiredUsers)
	// 同步删除 ADK session（在锁外执行，避免阻塞其他操作）
	if runner != nil && cleaned > 0 {
		for _, user := range expiredUsers {
			sessions, err := runner.ListSessions(ctx, user)
			if err != nil {
				continue
			}
			for _, s := range sessions {
				_ = runner.DeleteSession(ctx, user, s.ID())
			}
		}
	}
	return cleaned
}

// BuildUserContent 构造用户消息的 genai.Content
func BuildUserContent(text string) *genai.Content {
	return &genai.Content{
		Role:  "user",
		Parts: []*genai.Part{{Text: text}},
	}
}
