package openvpnweb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"openvpn-web/internal/openvpnweb/ai"
)

func TestSQLiteAIChatHistoryStoreKeepsUsersAndSessionsIsolated(t *testing.T) {
	database := newTestAIDatabase(t, ":memory:")
	if err := MigrateAIChatHistory(database); err != nil {
		t.Fatalf("MigrateAIChatHistory() error: %v", err)
	}
	store := NewSQLiteAIChatHistoryStore(database)
	ctx := context.Background()

	for _, message := range []ai.HistoryMessage{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "second"},
	} {
		if err := store.Append(ctx, "alice", "session-a", message); err != nil {
			t.Fatalf("Append(alice) error: %v", err)
		}
	}
	if err := store.Append(ctx, "bob", "session-a", ai.HistoryMessage{Role: "user", Content: "private"}); err != nil {
		t.Fatalf("Append(bob) error: %v", err)
	}
	if err := store.Append(ctx, "alice", "session-old", ai.HistoryMessage{Role: "user", Content: "old"}); err != nil {
		t.Fatalf("Append(old session) error: %v", err)
	}

	messages, err := store.List(ctx, "alice", "session-a")
	if err != nil {
		t.Fatalf("List(alice) error: %v", err)
	}
	if len(messages) != 2 || messages[0].Content != "first" || messages[1].Content != "second" {
		t.Fatalf("List(alice) = %#v, want ordered alice messages only", messages)
	}
	latest, err := store.LatestSession(ctx, "alice")
	if err != nil {
		t.Fatalf("LatestSession() error: %v", err)
	}
	if latest != "session-old" {
		t.Fatalf("LatestSession() = %q, want session-old", latest)
	}
	if err := store.Delete(ctx, "alice", "session-a"); err != nil {
		t.Fatalf("Delete(alice) error: %v", err)
	}
	messages, err = store.List(ctx, "alice", "session-a")
	if err != nil || len(messages) != 0 {
		t.Fatalf("List after delete = %#v, %v; want empty", messages, err)
	}
	messages, err = store.List(ctx, "bob", "session-a")
	if err != nil || len(messages) != 1 || messages[0].Content != "private" {
		t.Fatalf("bob history after alice delete = %#v, %v; want unchanged", messages, err)
	}
}

func TestSQLiteAIChatHistoryStoreSavesCompletedExchangeAtomically(t *testing.T) {
	database := newTestAIDatabase(t, ":memory:")
	if err := MigrateAIChatHistory(database); err != nil {
		t.Fatalf("MigrateAIChatHistory() error: %v", err)
	}
	store := NewSQLiteAIChatHistoryStore(database)
	if err := store.SaveExchange(context.Background(), "alice", "session-a",
		ai.HistoryMessage{Role: "user", Content: "question"},
		ai.HistoryMessage{Role: "assistant", Content: "answer"},
	); err != nil {
		t.Fatalf("SaveExchange() error: %v", err)
	}
	messages, err := store.List(context.Background(), "alice", "session-a")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("saved exchange = %#v, want ordered completed pair", messages)
	}
}

func TestAIHistoryEndpointsRestoreLatestAndProtectOtherUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database := newTestAIDatabase(t, ":memory:")
	if err := MigrateAIChatHistory(database); err != nil {
		t.Fatalf("MigrateAIChatHistory() error: %v", err)
	}
	store := NewSQLiteAIChatHistoryStore(database)
	ctx := context.Background()
	if err := store.Append(ctx, "alice", "alice-session", ai.HistoryMessage{Role: "user", Content: "hello"}); err != nil {
		t.Fatalf("Append() error: %v", err)
	}

	manager := ai.NewChatSessionManager(nil, store)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user", "alice") })
	ai.RegisterAIRoutes(router.Group("/ovpn/ai"), manager, ai.NewAtomicClient(nil), nil)

	request := httptest.NewRequest(http.MethodGet, "/ovpn/ai/history", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET latest history status = %d, body=%s", response.Code, response.Body.String())
	}
	var history struct {
		SessionID string              `json:"session_id"`
		Messages  []ai.HistoryMessage `json:"messages"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
		t.Fatalf("decode history response: %v", err)
	}
	if history.SessionID != "alice-session" || len(history.Messages) != 1 || history.Messages[0].Content != "hello" {
		t.Fatalf("GET latest history = %#v, want alice's latest persisted session", history)
	}

	// Re-register the same history store as Bob: querying/deleting Alice's session must expose nothing and retain Alice's data.
	bobRouter := gin.New()
	bobRouter.Use(func(c *gin.Context) { c.Set("user", "bob") })
	ai.RegisterAIRoutes(bobRouter.Group("/ovpn/ai"), manager, ai.NewAtomicClient(nil), nil)
	request = httptest.NewRequest(http.MethodGet, "/ovpn/ai/history?session_id=alice-session", nil)
	response = httptest.NewRecorder()
	bobRouter.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET foreign history status = %d, body=%s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
		t.Fatalf("decode foreign history response: %v", err)
	}
	if len(history.Messages) != 0 {
		t.Fatalf("foreign history = %#v, must be empty", history)
	}

	request = httptest.NewRequest(http.MethodDelete, "/ovpn/ai/history?session_id=alice-session", nil)
	response = httptest.NewRecorder()
	bobRouter.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("DELETE foreign history status = %d, body=%s", response.Code, response.Body.String())
	}
	messages, err := store.List(ctx, "alice", "alice-session")
	if err != nil || len(messages) != 1 {
		t.Fatalf("alice messages after bob delete = %#v, %v; want retained", messages, err)
	}
}
