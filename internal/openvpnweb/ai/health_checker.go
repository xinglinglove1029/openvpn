package ai

import (
	"context"
	"log"
	"strings"
	"sync"
	"sync/atomic"
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

// HealthChecker 后台静默自检器
// 周期性检测 LLM 可达性，缓存结果，状态变更时通过回调通知。
// 设计目标：用户打开 AI 助手时无需等待 HTTP health 请求，直接读缓存；
// 服务异常时主动通过 WebSocket 推送，前端弹出提示。
type HealthChecker struct {
	mu        sync.RWMutex
	client    *AtomicClient
	interval  time.Duration
	onChange  HealthStatusChangeHandler
	cached    HealthStatus
	hasCached bool
	stopCh    chan struct{}
	stoppedCh chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	started   atomic.Bool // 标记是否已启动，用于 Stop 时区分未启动场景
}

// HealthCheckerOption 自检器配置选项
type HealthCheckerOption func(*HealthChecker)

// WithHealthCheckInterval 设置检查间隔（默认 60s）
func WithHealthCheckInterval(d time.Duration) HealthCheckerOption {
	return func(h *HealthChecker) {
		if d > 0 {
			h.interval = d
		}
	}
}

// WithHealthChangeHandler 设置状态变更回调
func WithHealthChangeHandler(handler HealthStatusChangeHandler) HealthCheckerOption {
	return func(h *HealthChecker) {
		h.onChange = handler
	}
}

// NewHealthChecker 创建后台自检器
// client: LLM 客户端原子引用（支持热切换，每次检查都读最新值）
func NewHealthChecker(client *AtomicClient, opts ...HealthCheckerOption) *HealthChecker {
	h := &HealthChecker{
		client:    client,
		interval:  60 * time.Second,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Start 启动后台自检 goroutine（幂等，重复调用安全）
// 用 sync.Once 确保即使多次调用也只启动一个 loop goroutine
func (h *HealthChecker) Start() {
	h.startOnce.Do(func() {
		h.started.Store(true)
		go h.loop()
	})
}

// Stop 停止后台自检（阻塞等待 goroutine 退出）
// 用 sync.Once 防止 double close channel panic；
// 若从未启动则直接返回，避免永久阻塞
func (h *HealthChecker) Stop() {
	if !h.started.Load() {
		return // 从未启动，直接返回
	}
	h.stopOnce.Do(func() { close(h.stopCh) })
	select {
	case <-h.stoppedCh:
	case <-time.After(35 * time.Second): // 兜底超时，避免 CheckOnce 阻塞导致 Stop 卡死
	}
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

// loop 后台周期检查主循环
func (h *HealthChecker) loop() {
	defer close(h.stoppedCh)
	// panic 防护：避免 CheckOnce 内部 panic 导致整个进程崩溃
	defer func() {
		if r := recover(); r != nil {
			log.Printf("⚠ HealthChecker loop panic: %v", r)
		}
	}()

	// 启动时立即检查一次
	h.CheckOnce(context.Background())

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopCh:
			return
		case <-ticker.C:
			// 使用可被 stopCh 取消的 context，避免 Stop 时等待完整超时
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer cancel()
				h.CheckOnce(ctx)
			}()
			select {
			case <-h.stopCh:
				cancel() // 通知 doCheck 取消
				return
			case <-done:
			}
		}
	}
}

// doCheck 执行一次真实健康检查
// 超时根据 provider 动态调整：外部 API 30s，Ollama 15s
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

// updateCache 更新缓存并在状态变更时触发回调
// onChange 异步执行，避免阻塞健康检查循环
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
		// 异步执行回调，避免阻塞 loop
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
