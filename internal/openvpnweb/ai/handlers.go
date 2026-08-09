package ai

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// RegisterAIRoutes 注册 AI 模块路由到 ovpn 路由组下
// chatMgr: 会话管理器（始终非 nil，确保路由可注册）
// llmClient: LLM 客户端原子引用（可为 nil，表示 AI 未配置）
// healthChecker: 后台自检器（可为 nil，此时 Health 接口实时检查）
func RegisterAIRoutes(rg *gin.RouterGroup,
	chatMgr *ChatSessionManager,
	llmClient *AtomicClient,
	healthChecker *HealthChecker) {

	handler := &AIHandler{
		chatMgr:       chatMgr,
		llmClient:     llmClient,
		healthChecker: healthChecker,
	}

	rg.GET("/health", handler.Health)
	rg.POST("/chat", handler.Chat)
	rg.GET("/history", handler.History)
}

// AIHandler AI 模块 HTTP handler
type AIHandler struct {
	chatMgr       *ChatSessionManager
	llmClient     *AtomicClient
	healthChecker *HealthChecker
}

// HealthResponse 健康检查响应体
type HealthResponse struct {
	Available bool      `json:"available"`
	Model     string    `json:"model"`
	Provider  string    `json:"provider,omitempty"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checkedAt,omitempty"`
}

// HistoryMessage 历史消息
type HistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// HistoryResponse 会话历史响应体
type HistoryResponse struct {
	SessionID string           `json:"session_id"`
	Messages  []HistoryMessage `json:"messages"`
}

// ChatRequest 聊天请求体
type ChatRequest struct {
	Message   string `json:"message" binding:"required"`
	SessionID string `json:"session_id"`
}

// Chat SSE 流式聊天（接入 ADK Runner）
// POST /ovpn/ai/chat
// 请求体: { "message": "...", "session_id": "" }
// 响应: text/event-stream
//   - event: session     data: <session_id>
//   - event: token       data: <token_text>      （流式文本片段）
//   - event: tool_call   data: <tool_name>       （工具调用开始，可选）
//   - event: tool_result data: <result_summary>  （工具调用结果，可选）
//   - event: done        data: [DONE]
//   - event: error       data: <error_message>
func (h *AIHandler) Chat(c *gin.Context) {
	username, _ := c.Get("user")
	usernameStr, ok := username.(string)
	if !ok || usernameStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}

	// 获取 AgentRunner（通过 chatMgr 间接获取，支持热切换）
	if h.chatMgr == nil {
		c.JSON(http.StatusOK, gin.H{"message": "AI 会话管理器未初始化"})
		return
	}
	agentRunner := h.chatMgr.GetAgentRunner()
	if agentRunner == nil {
		c.JSON(http.StatusOK, gin.H{"message": "AI 服务尚未就绪，请在系统设置中配置 AI 助手"})
		return
	}

	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数无效", "detail": err.Error()})
		return
	}

	// 确保会话存在
	sessionID, err := h.chatMgr.EnsureSession(c.Request.Context(), usernameStr, req.SessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建会话失败: " + err.Error()})
		return
	}

	// 先检查 Flusher 是否可用，再设置 SSE 响应头（避免错误响应 Content-Type 错误）
	flusher, canFlush := c.Writer.(http.Flusher)
	if !canFlush {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "不支持流式响应"})
		return
	}

	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 禁用 nginx 缓冲

	// 发送 session_id 事件
	fmt.Fprintf(c.Writer, "event: session\ndata: %s\n\n", sessionID)
	flusher.Flush()

	// 创建超时 context（Chat 120s）
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()

	// panic 防护：避免 ADK Runner 内部 panic 导致进程崩溃
	defer func() {
		if r := recover(); r != nil {
			log.Printf("⚠ Chat handler panic: %v", r)
			fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", escapeSSEData("内部错误，请重试"))
			flusher.Flush()
		}
	}()

	// 调用 ADK Runner，遍历事件流
	var fullResponse strings.Builder
	var hasToolCall bool
	var hasPartialToken bool // 标记是否已通过 Partial 发送过 token

	for event, err := range agentRunner.Run(ctx, usernameStr, sessionID, req.Message) {
		if err != nil {
			errMsg := err.Error()
			if ctx.Err() == context.DeadlineExceeded {
				errMsg = "请求超时，请重试"
			} else if ctx.Err() == context.Canceled {
				errMsg = "请求已取消"
			}
			fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", escapeSSEData(errMsg))
			flusher.Flush()
			return
		}

		// 处理工具调用事件（通知前端正在执行操作）
		if HasToolCall(event) {
			hasToolCall = true
			for _, part := range event.Content.Parts {
				if part.FunctionCall != nil {
					fmt.Fprintf(c.Writer, "event: tool_call\ndata: %s\n\n", escapeSSEData(part.FunctionCall.Name))
					flusher.Flush()
				}
			}
			continue
		}

		// 处理工具响应事件
		if HasToolResponse(event) {
			for _, part := range event.Content.Parts {
				if part.FunctionResponse != nil && part.FunctionResponse.Response != nil {
					summary := summarizeToolResponse(part.FunctionResponse.Response)
					fmt.Fprintf(c.Writer, "event: tool_result\ndata: %s\n\n", escapeSSEData(summary))
					flusher.Flush()
				}
			}
			continue
		}

		// 处理文本事件
		text := ExtractEventText(event)
		if text == "" {
			continue
		}

		if event.Partial {
			// 流式 token：直接推送
			fmt.Fprintf(c.Writer, "event: token\ndata: %s\n\n", escapeSSEData(text))
			flusher.Flush()
			hasPartialToken = true
		} else if event.TurnComplete && !hasPartialToken {
			// 非流式模型（非 Partial）的完整响应：补发一次 token 事件
			// 避免前端 fullText 为空导致消息丢失
			fmt.Fprintf(c.Writer, "event: token\ndata: %s\n\n", escapeSSEData(text))
			flusher.Flush()
		}

		// 累积完整响应（无论是否 Partial）
		fullResponse.WriteString(text)
	}

	// 发送完成事件
	finalText := fullResponse.String()
	if finalText == "" && !hasToolCall {
		// 无响应且无工具调用
		fmt.Fprintf(c.Writer, "event: error\ndata: AI 未返回有效内容\n\n")
		flusher.Flush()
		return
	}

	fmt.Fprintf(c.Writer, "event: done\ndata: [DONE]\n\n")
	flusher.Flush()
}

// Health AI 服务健康检查。
// 默认读取后台缓存；传入 ?refresh=true 时强制执行一次真实探测并更新缓存，
// 供设置页“测试连接”和配置热切换后的即时状态确认使用。
// GET /ovpn/ai/health?refresh=true
func (h *AIHandler) Health(c *gin.Context) {
	if h.healthChecker != nil {
		if c.Query("refresh") == "true" {
			status := h.healthChecker.CheckOnce(c.Request.Context())
			c.JSON(http.StatusOK, HealthResponse{
				Available: status.Available,
				Model:     status.Model,
				Provider:  status.Provider,
				Error:     status.Error,
				CheckedAt: status.CheckedAt,
			})
			return
		}

		// 默认优先读缓存
		if status, ok := h.healthChecker.GetCachedStatus(); ok {
			c.JSON(http.StatusOK, HealthResponse{
				Available: status.Available,
				Model:     status.Model,
				Provider:  status.Provider,
				Error:     status.Error,
				CheckedAt: status.CheckedAt,
			})
			return
		}
		// 无缓存时触发一次即时检查
		status := h.healthChecker.CheckOnce(c.Request.Context())
		c.JSON(http.StatusOK, HealthResponse{
			Available: status.Available,
			Model:     status.Model,
			Provider:  status.Provider,
			Error:     status.Error,
			CheckedAt: status.CheckedAt,
		})
		return
	}

	// 回退：无 healthChecker 时直接检查 LLM 客户端
	client := h.llmClient.Get()
	if client == nil {
		c.JSON(http.StatusOK, HealthResponse{
			Available: false,
			Error:     "LLM 客户端未初始化",
		})
		return
	}

	timeout := 30 * time.Second
	if client.Provider() == "ollama" {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		c.JSON(http.StatusOK, HealthResponse{
			Available: false,
			Model:     client.Model(),
			Provider:  client.Provider(),
			Error:     sanitizeErrorMessage(fmt.Sprintf("连接失败: %v", err)),
		})
		return
	}

	c.JSON(http.StatusOK, HealthResponse{
		Available: true,
		Model:     client.Model(),
		Provider:  client.Provider(),
	})
}

// History 获取会话历史
// GET /ovpn/ai/history?session_id=xxx
func (h *AIHandler) History(c *gin.Context) {
	username, _ := c.Get("user")
	usernameStr, ok := username.(string)
	if !ok || usernameStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}

	if h.chatMgr == nil {
		c.JSON(http.StatusOK, HistoryResponse{
			SessionID: "",
			Messages:  []HistoryMessage{},
		})
		return
	}

	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "缺少 session_id 参数"})
		return
	}

	msgs, _ := h.chatMgr.GetHistory(c.Request.Context(), usernameStr, sessionID)
	c.JSON(http.StatusOK, HistoryResponse{
		SessionID: sessionID,
		Messages:  msgs,
	})
}

// escapeSSEData 按 SSE 规范处理 data 字段中的换行
// SSE 规范要求 data 字段不能包含裸换行，需用多行 data: 表示以保留换行格式
// 注意：调用方负责写第一行的 "data: " 前缀，本函数仅处理中间换行处的 "data: " 追加
// 例如：escapeSSEData("a\nb") 返回 "a\ndata: b"，外层拼为 "data: a\ndata: b\n\n"
func escapeSSEData(s string) string {
	// 如果不包含换行，直接原样返回（调用方已写 data: 前缀）
	if !strings.Contains(s, "\n") {
		return s
	}
	var b strings.Builder
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i == 0 {
			// 首行：调用方已写 data: 前缀，这里直接写内容
			b.WriteString(line)
		} else {
			// 后续行：换行后补上 data: 前缀（SSE 多行 data 字段格式）
			b.WriteString("\ndata: ")
			b.WriteString(line)
		}
	}
	return b.String()
}

// summarizeToolResponse 将工具响应 map 摘要为简短文本
func summarizeToolResponse(response map[string]any) string {
	if response == nil {
		return "工具执行完成"
	}
	if msg, ok := response["message"].(string); ok && msg != "" {
		return msg
	}
	if success, ok := response["success"].(bool); ok {
		if success {
			return "操作成功"
		}
		return "操作失败"
	}
	return "工具执行完成"
}
