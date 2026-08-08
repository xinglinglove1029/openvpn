package ai

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tmc/langchaingo/llms"
)

// RegisterAIRoutes 注册 AI 模块路由到 ovpn 路由组下
func RegisterAIRoutes(rg *gin.RouterGroup,
	chatMgr *ChatSessionManager,
	llmClient *AtomicClient) {

	handler := &AIHandler{
		chatMgr:   chatMgr,
		llmClient: llmClient,
	}

	rg.GET("/health", handler.Health)
	rg.POST("/chat", handler.Chat)
	rg.GET("/history", handler.History)
}

// AIHandler AI 模块 HTTP handler
type AIHandler struct {
	chatMgr   *ChatSessionManager
	llmClient *AtomicClient
}

// HealthResponse 健康检查响应体
type HealthResponse struct {
	Available bool   `json:"available"`
	Model     string `json:"model"`
	Error     string `json:"error,omitempty"`
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

// SSEEventType SSE 事件类型
const (
	SSEEventToken = "token"
	SSEEventDone  = "done"
	SSEEventError = "error"
)

// Chat SSE 流式聊天
// POST /ovpn/ai/chat
// 请求体: { "message": "...", "session_id": "" }
// 响应: text/event-stream，逐 token SSE 推送
func (h *AIHandler) Chat(c *gin.Context) {
	username, _ := c.Get("user")
	usernameStr, ok := username.(string)
	if !ok || usernameStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}

	var req struct {
		Message   string `json:"message" binding:"required"`
		SessionID string `json:"session_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数无效", "detail": err.Error()})
		return
	}

	// 获取或创建会话
	session, sessionID := h.chatMgr.GetOrCreate(usernameStr, req.SessionID)

	// 添加用户消息
	session.AddMessage("user", req.Message)

	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 禁用 nginx 缓冲

	flusher, canFlush := c.Writer.(http.Flusher)
	if !canFlush {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "不支持流式响应"})
		return
	}

	// 先发送 session_id 事件
	fmt.Fprintf(c.Writer, "event: session\ndata: %s\n\n", sessionID)
	flusher.Flush()

	// 获取上下文消息
	messages := session.GetContext()

	// 创建超时 context
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()

	// 收集完整响应用于存入历史
	var fullResponse string

	// 调用 LLM 流式生成
	_, err := h.llmClient.Get().GenerateStream(ctx, messages, func(token string) error {
		fullResponse += token
		// 发送 SSE token 事件
		fmt.Fprintf(c.Writer, "event: token\ndata: %s\n\n", token)
		flusher.Flush()
		return nil
	})

	if err != nil {
		// 如果已经有部分 token 发出了，先保存再发送错误
		if fullResponse != "" {
			session.AddMessage("assistant", fullResponse)
		}

		if ctx.Err() == context.DeadlineExceeded {
			fmt.Fprintf(c.Writer, "event: error\ndata: 请求超时，请重试\n\n")
		} else if ctx.Err() == context.Canceled {
			fmt.Fprintf(c.Writer, "event: error\ndata: 请求已取消\n\n")
		} else {
			fmt.Fprintf(c.Writer, "event: error\ndata: AI 服务异常: %s\n\n", err.Error())
		}
		flusher.Flush()
		return
	}

	// 保存完整响应
	if fullResponse != "" {
		session.AddMessage("assistant", fullResponse)
	}

	// 发送完成事件
	fmt.Fprintf(c.Writer, "event: done\ndata: [DONE]\n\n")
	flusher.Flush()
}

// Health AI 服务健康检查
// GET /ovpn/ai/health
func (h *AIHandler) Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// 发送一条简单消息验证 LLM 连通性
	client := h.llmClient.Get()
	if client == nil {
		c.JSON(http.StatusOK, HealthResponse{
			Available: false,
			Model:     "",
			Error:     "LLM 客户端未初始化",
		})
		return
	}

	messages := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, "ping"),
	}
	_, err := client.Generate(ctx, messages)
	if err != nil {
		c.JSON(http.StatusOK, HealthResponse{
			Available: false,
			Model:     client.Model(),
			Error:     fmt.Sprintf("连接失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, HealthResponse{
		Available: true,
		Model:     client.Model(),
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

	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "缺少 session_id 参数"})
		return
	}

	session, _ := h.chatMgr.GetOrCreate(usernameStr, sessionID)
	msgs := session.GetMessages()

	historyMsgs := make([]HistoryMessage, 0, len(msgs))
	for _, m := range msgs {
		historyMsgs = append(historyMsgs, HistoryMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	c.JSON(http.StatusOK, HistoryResponse{
		SessionID: sessionID,
		Messages:  historyMsgs,
	})
}
