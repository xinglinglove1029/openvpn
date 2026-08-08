package ai

import (
	"sync"
	"time"

	"github.com/tmc/langchaingo/llms"
)

const (
	// MaxContextMessages 对话上下文最大消息数（不含 system prompt）
	MaxContextMessages = 20
	// SessionIdleTimeout 会话空闲超时时间
	SessionIdleTimeout = 30 * time.Minute
)

// ChatMessage 对话消息
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatSession 单个用户的对话会话，包含上下文窗口管理
type ChatSession struct {
	mu           sync.Mutex
	Messages     []ChatMessage
	SystemPrompt string
	CreatedAt    time.Time
	LastActiveAt time.Time
}

// AddMessage 添加一条消息到上下文窗口，超过上限时自动裁剪
func (s *ChatSession) AddMessage(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.LastActiveAt = time.Now()
	s.Messages = append(s.Messages, ChatMessage{
		Role:    role,
		Content: content,
	})

	// 环形缓冲区：保留最近 N 条消息
	if len(s.Messages) > MaxContextMessages {
		s.Messages = s.Messages[len(s.Messages)-MaxContextMessages:]
	}
}

// GetContext 获取当前上下文，转为 langchaingo MessageContent 格式
func (s *ChatSession) GetContext() []llms.MessageContent {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := make([]llms.MessageContent, 0, len(s.Messages)+1)

	// 系统提示词放在最前面
	if s.SystemPrompt != "" {
		ctx = append(ctx, llms.TextParts(llms.ChatMessageTypeSystem, s.SystemPrompt))
	}

	for _, m := range s.Messages {
		switch m.Role {
		case "user":
			ctx = append(ctx, llms.TextParts(llms.ChatMessageTypeHuman, m.Content))
		case "assistant":
			ctx = append(ctx, llms.TextParts(llms.ChatMessageTypeAI, m.Content))
		}
	}

	return ctx
}

// GetMessages 返回原始消息列表（供 API 返回）
func (s *ChatSession) GetMessages() []ChatMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	msgs := make([]ChatMessage, len(s.Messages))
	copy(msgs, s.Messages)
	return msgs
}

// IsIdle 检查会话是否空闲超时
func (s *ChatSession) IsIdle(timeout time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.LastActiveAt) > timeout
}

// ChatSessionManager 会话管理器，以内存 map 存储所有活跃会话
type ChatSessionManager struct {
	mu           sync.RWMutex
	sessions     map[string]*ChatSession
	systemPrompt string
}

// NewChatSessionManager 创建会话管理器
func NewChatSessionManager(systemPrompt string) *ChatSessionManager {
	return &ChatSessionManager{
		sessions:     make(map[string]*ChatSession),
		systemPrompt: systemPrompt,
	}
}

// GetOrCreate 获取或创建用户会话
func (m *ChatSessionManager) GetOrCreate(username, sessionID string) (*ChatSession, string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sessionID != "" {
		key := username + ":" + sessionID
		if session, ok := m.sessions[key]; ok {
			session.mu.Lock()
			session.LastActiveAt = time.Now()
			session.mu.Unlock()
			return session, sessionID
		}
	}

	newID := generateSessionID(username)
	key := username + ":" + newID
	session := &ChatSession{
		SystemPrompt: m.systemPrompt,
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}
	m.sessions[key] = session
	return session, newID
}

// Cleanup 清理空闲会话，返回清理数量
func (m *ChatSessionManager) Cleanup(maxIdle time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleaned := 0
	for key, session := range m.sessions {
		if session.IsIdle(maxIdle) {
			delete(m.sessions, key)
			cleaned++
		}
	}
	return cleaned
}

func generateSessionID(username string) string {
	return username + "_" + time.Now().Format("20060102_150405")
}
