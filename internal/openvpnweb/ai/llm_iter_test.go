package ai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

func TestGenerateContentStopsWhenConsumerBreaks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"second\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client, err := NewLLMClient(LLMConfig{Provider: "customize", BaseURL: server.URL, Model: "test", Temperature: 0.7})
	if err != nil {
		t.Fatalf("NewLLMClient() error: %v", err)
	}

	got := ""
	for response, err := range client.GenerateContent(context.Background(), &model.LLMRequest{
		Contents: []*genai.Content{genai.NewContentFromText("hello", genai.RoleUser)},
	}, true) {
		if err != nil {
			t.Fatalf("GenerateContent() error: %v", err)
		}
		got = response.Content.Parts[0].Text
		break // Simulates an HTTP client disconnecting while the provider is still streaming.
	}
	if got != "first" {
		t.Fatalf("first streamed token = %q, want %q", got, "first")
	}
}

func TestNewLLMClientNormalizesOllamaLocalhost(t *testing.T) {
	client, err := NewLLMClient(LLMConfig{
		Provider:    "ollama",
		BaseURL:     "http://localhost:11434/",
		Model:       "qwen2.5:7b",
		Temperature: 0.7,
	})
	if err != nil {
		t.Fatalf("NewLLMClient() error: %v", err)
	}
	if client.baseURL != "http://127.0.0.1:11434" {
		t.Fatalf("baseURL = %q, want IPv4 loopback", client.baseURL)
	}

	custom, err := NewLLMClient(LLMConfig{
		Provider:    "customize",
		BaseURL:     "http://localhost:11434/",
		Model:       "test",
		Temperature: 0.7,
	})
	if err != nil {
		t.Fatalf("NewLLMClient(customize) error: %v", err)
	}
	if !strings.Contains(custom.baseURL, "localhost") {
		t.Fatalf("custom provider baseURL unexpectedly changed: %q", custom.baseURL)
	}
}
