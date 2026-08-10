package ai

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
)

// HealthStatus AI 服务健康状态
type HealthStatus struct {
	Available bool      `json:"available"`
	Model     string    `json:"model"`
	Provider  string    `json:"provider"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
}

// HealthStatusChangeHandler 状态变更回调
// 当健康状态发生变化（可用→不可用 或 不可用→可用）时触发。
// 由 server.go 注入，用于通过 WebSocket 推送 ai:health 事件。
type HealthStatusChangeHandler func(status HealthStatus)

// HealthChecker caches AI service state. It never performs scheduled provider probes.
// Only an administrator's explicit connection test calls the model provider, preventing background token usage.
type HealthChecker struct {
	mu        sync.RWMutex
	client    *AtomicClient
	onChange  HealthStatusChangeHandler
	cached    HealthStatus
	hasCached bool
}

// HealthCheckerOption configures a health checker.
type HealthCheckerOption func(*HealthChecker)

// WithHealthChangeHandler registers a callback that receives status updates.
func WithHealthChangeHandler(handler HealthStatusChangeHandler) HealthCheckerOption {
	return func(h *HealthChecker) {
		h.onChange = handler
	}
}

// NewHealthChecker creates a cache-backed status checker. CheckOnce is reserved for an explicit connection test.
func NewHealthChecker(client *AtomicClient, opts ...HealthCheckerOption) *HealthChecker {
	h := &HealthChecker{client: client}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// SetConfiguredStatus updates cached status from configuration only; it does not call the model provider.
// A configured client is marked ready until an administrator explicitly tests connectivity.
func (h *HealthChecker) SetConfiguredStatus() HealthStatus {
	status := HealthStatus{CheckedAt: time.Now()}
	if client := h.client.Get(); client != nil {
		status.Available = true
		status.Provider = client.Provider()
		status.Model = client.Model()
	} else {
		status.Error = "AI service is not configured"
	}
	h.updateCache(status)
	return status
}

// GetCachedStatus 获取缓存的健康状态（无缓存时返回零值）
func (h *HealthChecker) GetCachedStatus() (HealthStatus, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cached, h.hasCached
}

// CheckOnce 立即执行一次检查并更新缓存（供 Health 接口按需触发）
func (h *HealthChecker) CheckOnce(ctx context.Context) HealthStatus {
	status := h.doCheck(ctx)
	h.updateCache(status)
	return status
}

func (h *HealthChecker) doCheck(ctx context.Context) HealthStatus {
	client := h.client.Get()
	if client == nil {
		return HealthStatus{
			Available: false,
			Error:     "LLM 客户端未初始化",
			CheckedAt: time.Now(),
		}
	}

	timeout := 30 * time.Second
	if strings.EqualFold(client.Provider(), "ollama") {
		// Ollama only checks /api/tags, so it never loads a model; a short timeout detects an unreachable service quickly.
		timeout = 5 * time.Second
	}

	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	status := HealthStatus{
		Provider:  client.Provider(),
		Model:     client.Model(),
		CheckedAt: time.Now(),
	}

	if err := client.Ping(checkCtx); err != nil {
		status.Available = false
		status.Error = sanitizeErrorMessage(err.Error())
		log.Printf("⚠ AI 健康检查失败（Provider: %s, 模型: %s）: %v", status.Provider, status.Model, err)
	} else {
		status.Available = true
	}

	return status
}

// updateCache writes cached state and calls the change handler only when availability or the error changes.
// The handler runs asynchronously so status updates never block the current request.
func (h *HealthChecker) updateCache(newStatus HealthStatus) {
	h.mu.Lock()
	oldStatus := h.cached
	// 状态变更判定：Available 变化 或 Error 变化（覆盖 Set(nil) 后 error 文案变化但 available 都为 false 的场景）
	changed := !h.hasCached || oldStatus.Available != newStatus.Available || oldStatus.Error != newStatus.Error
	h.cached = newStatus
	h.hasCached = true
	onChange := h.onChange
	h.mu.Unlock()

	if changed && onChange != nil {
		log.Printf("ℹ AI 健康状态变更: available=%v → %v（model=%s, error=%s）",
			oldStatus.Available, newStatus.Available, newStatus.Model, newStatus.Error)
		// Run the callback asynchronously to avoid blocking the current request.
		go onChange(newStatus)
	}
}

// sanitizeErrorMessage 对错误信息进行脱敏和截断，避免泄露 API Key 等敏感信息
func sanitizeErrorMessage(msg string) string {
	// 截断过长错误信息
	if len(msg) > 200 {
		msg = msg[:200] + "..."
	}
	// 脱敏 sk- 开头的 API Key 片段
	for _, prefix := range []string{"sk-", "Bearer "} {
		for {
			idx := indexFold(msg, prefix)
			if idx < 0 {
				break
			}
			// 替换 key 片段（取后续 20 字符）
			end := idx + len(prefix) + 20
			if end > len(msg) {
				end = len(msg)
			}
			msg = msg[:idx] + prefix + "***" + msg[end:]
		}
	}
	return msg
}

// indexFold 大小写不敏感查找子串
func indexFold(s, substr string) int {
	sLower := strings.ToLower(s)
	subLower := strings.ToLower(substr)
	return strings.Index(sLower, subLower)
}
