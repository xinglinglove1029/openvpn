package ai

import (
	"context"
	"testing"

	"google.golang.org/adk/session"
)

func TestEnsureSessionWithHistoryHydratesMissingInMemorySession(t *testing.T) {
	service := session.InMemoryService()
	runner := &AgentRunner{sessionService: service}
	history := []HistoryMessage{
		{Role: "user", Content: "first prompt"},
		{Role: "assistant", Content: "first answer"},
	}

	sessionID, err := runner.EnsureSessionWithHistory(context.Background(), "alice", "persisted-session", history)
	if err != nil {
		t.Fatalf("EnsureSessionWithHistory() error: %v", err)
	}
	if sessionID != "persisted-session" {
		t.Fatalf("session ID = %q, want persisted-session", sessionID)
	}

	resp, err := service.Get(context.Background(), &session.GetRequest{
		AppName: AgentAppName, UserID: "alice", SessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	events := resp.Session.Events()
	if events.Len() != 2 {
		t.Fatalf("hydrated event count = %d, want 2", events.Len())
	}
	var got []HistoryMessage
	for event := range events.All() {
		got = append(got, HistoryMessage{Role: event.Author, Content: ExtractEventText(event)})
	}
	if got[0].Role != "user" || got[0].Content != "first prompt" || got[1].Role != AgentName || got[1].Content != "first answer" {
		t.Fatalf("hydrated events = %#v, want ordered user and assistant context", got)
	}
}
