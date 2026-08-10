package ai

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetConfiguredStatusDoesNotProbeProvider(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "a provider probe was not expected", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewLLMClient(LLMConfig{
		Provider: "deepseek",
		BaseURL:  server.URL + "/v1",
		APIKey:   "test-key",
		Model:    "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatalf("NewLLMClient() error: %v", err)
	}

	checker := NewHealthChecker(NewAtomicClient(client))
	status := checker.SetConfiguredStatus()
	if !status.Available {
		t.Fatalf("SetConfiguredStatus().Available = false, want true (error=%q)", status.Error)
	}
	if status.Provider != "deepseek" || status.Model != "deepseek-v4-flash" {
		t.Fatalf("SetConfiguredStatus() = provider=%q model=%q, want configured client", status.Provider, status.Model)
	}
	if requests.Load() != 0 {
		t.Fatalf("provider requests = %d, want 0", requests.Load())
	}

	cached, ok := checker.GetCachedStatus()
	if !ok || !cached.Available {
		t.Fatalf("GetCachedStatus() = %#v, %v; want cached available status", cached, ok)
	}
}

func TestSetConfiguredStatusReportsUnconfiguredClient(t *testing.T) {
	checker := NewHealthChecker(NewAtomicClient(nil))
	status := checker.SetConfiguredStatus()
	if status.Available {
		t.Fatal("SetConfiguredStatus().Available = true, want false without a client")
	}
	if status.Error == "" {
		t.Fatal("SetConfiguredStatus().Error is empty, want an unconfigured error")
	}
}

func TestHealthProbesProviderOnlyWhenExplicitlyRefreshed(t *testing.T) {
	var requests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer provider.Close()

	client, err := NewLLMClient(LLMConfig{
		Provider: "deepseek",
		BaseURL:  provider.URL + "/v1",
		APIKey:   "test-key",
		Model:    "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatalf("NewLLMClient() error: %v", err)
	}

	handler := &AIHandler{
		llmClient:     NewAtomicClient(client),
		healthChecker: NewHealthChecker(NewAtomicClient(client)),
	}
	gin.SetMode(gin.TestMode)

	normalRecorder := httptest.NewRecorder()
	normalCtx, _ := gin.CreateTestContext(normalRecorder)
	normalCtx.Request = httptest.NewRequest(http.MethodGet, "/ovpn/ai/health", nil)
	handler.Health(normalCtx)
	if normalRecorder.Code != http.StatusOK {
		t.Fatalf("GET /health status = %d, want %d", normalRecorder.Code, http.StatusOK)
	}
	if requests.Load() != 0 {
		t.Fatalf("normal GET /health provider requests = %d, want 0", requests.Load())
	}

	refreshRecorder := httptest.NewRecorder()
	refreshCtx, _ := gin.CreateTestContext(refreshRecorder)
	refreshCtx.Request = httptest.NewRequest(http.MethodGet, "/ovpn/ai/health?refresh=true", nil)
	handler.Health(refreshCtx)
	if refreshRecorder.Code != http.StatusOK {
		t.Fatalf("GET /health?refresh=true status = %d, want %d", refreshRecorder.Code, http.StatusOK)
	}
	if requests.Load() != 1 {
		t.Fatalf("refresh GET /health provider requests = %d, want 1", requests.Load())
	}
}
