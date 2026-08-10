package ai

import (
	"context"
	"log"
	"sync"
	"time"

	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// ChatHistoryStore persists completed chat messages independently of the ADK in-memory session.
type ChatHistoryStore interface {
	Append(ctx context.Context, username, sessionID string, message HistoryMessage) error
	SaveExchange(ctx context.Context, username, sessionID string, userMessage, assistantMessage HistoryMessage) error
	List(ctx context.Context, username, sessionID string) ([]HistoryMessage, error)
	LatestSession(ctx context.Context, username string) (string, error)
	Delete(ctx context.Context, username, sessionID string) error
}

// ChatSessionManager manages active ADK sessions while SQLite remains the source of truth for chat history.
type ChatSessionManager struct {
	mu           sync.RWMutex
	activeUsers  map[string]time.Time // username to last activity time
	agentRunner  *AgentRunner         // immediate model context and tool execution
	historyStore ChatHistoryStore     // completed message persistence
}

// NewChatSessionManager creates a session manager.
// agentRunner may be nil; persisted history is still available and a runner may be injected later.
func NewChatSessionManager(agentRunner *AgentRunner, historyStore ...ChatHistoryStore) *ChatSessionManager {
	var store ChatHistoryStore
	if len(historyStore) > 0 {
		store = historyStore[0]
	}
	return &ChatSessionManager{
		activeUsers:  make(map[string]time.Time),
		agentRunner:  agentRunner,
		historyStore: store,
	}
}

// SetAgentRunner injects or replaces the AgentRunner after a provider switch.
func (m *ChatSessionManager) SetAgentRunner(r *AgentRunner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agentRunner = r
}

// GetAgentRunner returns the current AgentRunner safely.
func (m *ChatSessionManager) GetAgentRunner() *AgentRunner {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.agentRunner
}

// EnsureSession makes sure the session exists and updates the user's activity time.
func (m *ChatSessionManager) EnsureSession(ctx context.Context, username, sessionID string) (string, error) {
	m.mu.Lock()
	m.activeUsers[username] = time.Now()
	runner := m.agentRunner
	m.mu.Unlock()

	if runner == nil {
		// Before a runner is ready, generate an ID but do not create an ADK session.
		if sessionID == "" {
			sessionID = generateSessionID(username)
		}
		return sessionID, nil
	}

	// ADK sessions are intentionally in memory. When a process restart, idle cleanup, or
	// provider switch removed the ADK session, rebuild its model context from SQLite before
	// accepting the follow-up prompt.
	var history []HistoryMessage
	if sessionID != "" {
		m.mu.RLock()
		store := m.historyStore
		m.mu.RUnlock()
		if store != nil {
			var err error
			history, err = store.List(ctx, username, sessionID)
			if err != nil {
				return "", err
			}
		}
	}
	return runner.EnsureSessionWithHistory(ctx, username, sessionID, history)
}

// SaveExchange atomically persists a completed user/assistant turn. Persistence errors are returned to
// callers so they can be logged without changing an otherwise successful chat response.
func (m *ChatSessionManager) SaveExchange(ctx context.Context, username, sessionID string, userMessage, assistantMessage HistoryMessage) error {
	m.mu.RLock()
	store := m.historyStore
	m.mu.RUnlock()
	if store == nil || sessionID == "" {
		return nil
	}
	return store.SaveExchange(ctx, username, sessionID, userMessage, assistantMessage)
}

// GetHistory reads the SQLite history authority. It only falls back to ADK when no history store was injected.
func (m *ChatSessionManager) GetHistory(ctx context.Context, username, sessionID string) ([]HistoryMessage, error) {
	if sessionID == "" {
		return []HistoryMessage{}, nil
	}

	m.mu.RLock()
	store := m.historyStore
	runner := m.agentRunner
	m.mu.RUnlock()
	if store != nil {
		return store.List(ctx, username, sessionID)
	}
	if runner == nil {
		return []HistoryMessage{}, nil
	}

	resp, err := runner.sessionService.Get(ctx, &session.GetRequest{
		AppName:   AgentAppName,
		UserID:    username,
		SessionID: sessionID,
	})
	if err != nil {
		return []HistoryMessage{}, nil
	}

	events := resp.Session.Events()
	if events == nil {
		return []HistoryMessage{}, nil
	}
	msgs := make([]HistoryMessage, 0, events.Len())
	for event := range events.All() {
		if HasToolCall(event) || HasToolResponse(event) || event.Partial {
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
		msgs = append(msgs, HistoryMessage{Role: role, Content: text})
	}
	return msgs, nil
}

// LatestSession returns the most recently active persisted session for a user.
func (m *ChatSessionManager) LatestSession(ctx context.Context, username string) (string, error) {
	m.mu.RLock()
	store := m.historyStore
	m.mu.RUnlock()
	if store == nil {
		return "", nil
	}
	return store.LatestSession(ctx, username)
}

// DeleteSession removes only the specified user's persisted messages and its ADK session.
func (m *ChatSessionManager) DeleteSession(ctx context.Context, username, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	m.mu.RLock()
	store := m.historyStore
	runner := m.agentRunner
	m.mu.RUnlock()
	if store != nil {
		if err := store.Delete(ctx, username, sessionID); err != nil {
			return err
		}
	}
	if runner == nil {
		return nil
	}
	// SQLite is authoritative. A stale in-memory session must not make a successful
	// durable-history deletion look like a failed clear operation to the client.
	if err := runner.DeleteSession(ctx, username, sessionID); err != nil {
		log.Printf("delete AI in-memory session failed (user=%s, session=%s): %v", username, sessionID, err)
	}
	return nil
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
