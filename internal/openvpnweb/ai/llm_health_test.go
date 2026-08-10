package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaPingUsesTagsAndValidatesConfiguredModel(t *testing.T) {
	var requestedPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodGet || r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen2.5:1.5b"}]}`))
	}))
	defer server.Close()

	client, err := NewLLMClient(LLMConfig{
		Provider: "ollama", BaseURL: server.URL, Model: "qwen2.5:1.5b", Temperature: 0.7,
	})
	if err != nil {
		t.Fatalf("NewLLMClient() error: %v", err)
	}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error: %v", err)
	}
	if len(requestedPaths) != 1 || requestedPaths[0] != "GET /api/tags" {
		t.Fatalf("health requests = %v, want exactly GET /api/tags", requestedPaths)
	}
}

func TestOllamaPingReportsMissingConfiguredModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen2.5:7b"}]}`))
	}))
	defer server.Close()

	client, err := NewLLMClient(LLMConfig{
		Provider: "ollama", BaseURL: server.URL, Model: "qwen2.5:1.5b", Temperature: 0.7,
	})
	if err != nil {
		t.Fatalf("NewLLMClient() error: %v", err)
	}
	err = client.Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), `qwen2.5:1.5b`) || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("Ping() error = %v, want missing configured model error", err)
	}
}

func TestOllamaPingAcceptsImplicitLatestTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen2.5:latest"}]}`))
	}))
	defer server.Close()

	client, err := NewLLMClient(LLMConfig{Provider: "ollama", BaseURL: server.URL, Model: "qwen2.5"})
	if err != nil {
		t.Fatalf("NewLLMClient() error: %v", err)
	}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error = %v, want implicit latest tag to match", err)
	}
}
