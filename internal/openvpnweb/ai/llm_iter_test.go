package ai

import (
	"context"
	"encoding/json"
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
	if client.baseURL != "http://127.0.0.1:11434/v1" {
		t.Fatalf("baseURL = %q, want IPv4 loopback with /v1", client.baseURL)
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

func TestGenerateContentOllamaTextToolCallOnDone(t *testing.T) {
	const toolJSON = `{"name":"get_system_counts","arguments":{"limit":50,"offset":0}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSETextChunk(t, w, toolJSON[:53])
		writeSSETextChunk(t, w, toolJSON[53:])
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := newToolTestClient(t, "ollama", server.URL)
	responses := collectLLMResponses(t, client, newTextToolRequest(), true)
	if len(responses) != 1 {
		t.Fatalf("response count = %d, want 1", len(responses))
	}
	call := responseFunctionCall(t, responses[0])
	if call.Name != "get_system_counts" {
		t.Fatalf("FunctionCall name = %q, want get_system_counts", call.Name)
	}
	if got := call.Args["limit"]; got != float64(50) {
		t.Fatalf("FunctionCall limit = %#v, want 50", got)
	}
	if got := call.Args["offset"]; got != float64(0) {
		t.Fatalf("FunctionCall offset = %#v, want 0", got)
	}
}

func TestGenerateContentOllamaNonStreamTextToolCall(t *testing.T) {
	const toolJSON = `{"name":"get_system_counts","arguments":{"limit":50}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%s}}]}`, mustJSON(t, toolJSON))
	}))
	defer server.Close()

	client := newToolTestClient(t, "ollama", server.URL)
	responses := collectLLMResponses(t, client, newTextToolRequest(), false)
	if len(responses) != 1 {
		t.Fatalf("response count = %d, want 1", len(responses))
	}
	call := responseFunctionCall(t, responses[0])
	if call.Name != "get_system_counts" || call.Args["limit"] != float64(50) {
		t.Fatalf("unexpected FunctionCall: %#v", call)
	}
}

func TestGenerateContentOllamaTextToolCallRejectsUnknownTool(t *testing.T) {
	const text = `{"name":"delete_everything","arguments":{}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSETextChunk(t, w, text)
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	responses := collectLLMResponses(t, newToolTestClient(t, "ollama", server.URL), newTextToolRequest(), true)
	if len(responses) != 1 {
		t.Fatalf("response count = %d, want 1", len(responses))
	}
	if len(responses[0].Content.Parts) != 1 || responses[0].Content.Parts[0].FunctionCall != nil {
		t.Fatalf("unknown tool was converted into a FunctionCall: %#v", responses[0].Content)
	}
	if got := responses[0].Content.Parts[0].Text; got != text {
		t.Fatalf("text = %q, want %q", got, text)
	}
}

func TestTextToolCallResponseRejectsEmbeddedOrTrailingText(t *testing.T) {
	allowed := map[string]struct{}{"get_system_counts": {}}
	valid := `{"name":"get_system_counts","arguments":{}}`
	for _, text := range []string{
		"Example only:\n" + valid,
		valid + " followed by an explanation",
		valid + "\n" + valid,
		"```json\n" + valid + "\n```\nExplanation",
	} {
		if _, ok := textToolCallResponse(text, allowed); ok {
			t.Fatalf("textToolCallResponse(%q) converted non-standalone text", text)
		}
	}

	response, ok := textToolCallResponse("```json\n"+valid+"\n```", allowed)
	if !ok {
		t.Fatal("pure fenced JSON was not converted")
	}
	if call := responseFunctionCall(t, response); call.Name != "get_system_counts" {
		t.Fatalf("FunctionCall name = %q, want get_system_counts", call.Name)
	}
}

func TestGenerateContentNonOllamaTextToolCallRemainsText(t *testing.T) {
	const toolJSON = `{"name":"get_system_counts","arguments":{}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%s}}]}`, mustJSON(t, toolJSON))
	}))
	defer server.Close()

	responses := collectLLMResponses(t, newToolTestClient(t, "customize", server.URL), newTextToolRequest(), false)
	if len(responses) != 1 || len(responses[0].Content.Parts) != 1 {
		t.Fatalf("responses = %#v", responses)
	}
	part := responses[0].Content.Parts[0]
	if part.FunctionCall != nil || part.Text != toolJSON {
		t.Fatalf("non-Ollama text was unexpectedly converted: %#v", part)
	}
}

func newToolTestClient(t *testing.T, provider, baseURL string) *OpenAIModel {
	t.Helper()
	client, err := NewLLMClient(LLMConfig{Provider: provider, BaseURL: baseURL, Model: "test", Temperature: 0.7})
	if err != nil {
		t.Fatalf("NewLLMClient() error: %v", err)
	}
	return client
}

func newTextToolRequest() *model.LLMRequest {
	return &model.LLMRequest{
		Contents: []*genai.Content{genai.NewContentFromText("show system status", genai.RoleUser)},
		Config: &genai.GenerateContentConfig{Tools: []*genai.Tool{{
			FunctionDeclarations: []*genai.FunctionDeclaration{{Name: "get_system_counts"}},
		}}},
	}
}

func collectLLMResponses(t *testing.T, client *OpenAIModel, req *model.LLMRequest, stream bool) []*model.LLMResponse {
	t.Helper()
	var responses []*model.LLMResponse
	for response, err := range client.GenerateContent(context.Background(), req, stream) {
		if err != nil {
			t.Fatalf("GenerateContent() error: %v", err)
		}
		responses = append(responses, response)
	}
	return responses
}

func responseFunctionCall(t *testing.T, response *model.LLMResponse) *genai.FunctionCall {
	t.Helper()
	if response == nil || response.Content == nil || len(response.Content.Parts) != 1 || response.Content.Parts[0].FunctionCall == nil {
		t.Fatalf("response does not contain exactly one FunctionCall: %#v", response)
	}
	return response.Content.Parts[0].FunctionCall
}

func writeSSETextChunk(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	chunk, err := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"delta": map[string]any{"content": content}}},
	})
	if err != nil {
		t.Fatalf("json.Marshal(SSE chunk) error: %v", err)
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
}

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	return string(encoded)
}
