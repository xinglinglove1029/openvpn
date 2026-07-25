package openvpnweb

import (
	"context"
	"strings"
	"time"

	"github.com/gavintan/gopkg/tools"
)

// NotifyEvent 用户上下线事件
type NotifyEvent struct {
	Event         string
	Vip           string
	Vip6          string
	Rip           string
	Rip6          string
	CommonName    string
	Username      string
	BytesReceived float64
	BytesSent     float64
	TimeUnix      int64
	TimeDuration  int64
}

// NotifyLog 通知发送记录
type NotifyLog struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Event     string    `json:"event"`
	Provider  string    `json:"provider"`
	Username  string    `json:"username"`
	Success   bool      `json:"success"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

func notifyTitle(event string) string {
	switch strings.ToLower(event) {
	case "connect", "online":
		return "OpenVPN 用户上线"
	case "disconnect", "offline":
		return "OpenVPN 用户下线"
	default:
		return "OpenVPN 用户事件"
	}
}

func buildNotifyMarkdown(title string, event NotifyEvent) string {
	username := event.Username
	if username == "" || username == "UNDEF" {
		username = event.CommonName
	}

	lines := []string{
		"### " + title,
		"- 用户：" + emptyValue(username),
		"- 客户端：" + emptyValue(event.CommonName),
		"- VPN IP：" + emptyValue(joinNonEmpty(event.Vip, event.Vip6)),
		"- 来源 IP：" + emptyValue(joinNonEmpty(event.Rip, event.Rip6)),
	}

	if event.TimeUnix > 0 {
		lines = append(lines, "- 上线时间："+time.Unix(event.TimeUnix, 0).Format("2006-01-02 15:04:05"))
	}
	if event.TimeDuration > 0 {
		lines = append(lines, "- 在线时长："+(time.Duration(event.TimeDuration)*time.Second).String())
	}
	if event.BytesReceived > 0 || event.BytesSent > 0 {
		lines = append(lines, "- 下行流量："+tools.FormatBytes(event.BytesReceived))
		lines = append(lines, "- 上行流量："+tools.FormatBytes(event.BytesSent))
	}
	lines = append(lines, "- 通知时间："+time.Now().Format("2006-01-02 15:04:05"))

	return strings.Join(lines, "\n")
}

func emptyValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func joinNonEmpty(values ...string) string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			items = append(items, value)
		}
	}
	return strings.Join(items, " / ")
}

func LogNotifyError(event NotifyEvent, err error) {
	if err != nil {
		logger.Error(context.Background(), "send %s notify failed: %s", event.Event, err)
	}
}

func recordNotifyLog(event NotifyEvent, provider string, success bool, message string) {
	if db == nil {
		return
	}
	username := displayUsername(event)

	logItem := NotifyLog{
		Event:    event.Event,
		Provider: provider,
		Username: username,
		Success:  success,
		Message:  message,
	}
	if err := db.WithContext(context.Background()).Create(&logItem).Error; err != nil {
		logger.Error(context.Background(), "record notify log failed: %s", err)
		return
	}

	// 站内信实时广播：通过全局事件总线解耦推送
	// 任意监听 notify:new 主题的模块都会收到；当前由 WsHub 转发到 WebSocket 客户端
	Bus().Publish("notify:new", map[string]any{
		"id":        logItem.ID,
		"event":     logItem.Event,
		"provider":  logItem.Provider,
		"username":  logItem.Username,
		"success":   logItem.Success,
		"message":   logItem.Message,
		"createdAt": logItem.CreatedAt,
	})
}

func queryNotifyLogs(limit int) []NotifyLog {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	logs := make([]NotifyLog, 0)
	if db == nil {
		return logs
	}

	if err := db.WithContext(context.Background()).Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		logger.Error(context.Background(), "query notify logs failed: %s", err)
		return []NotifyLog{}
	}
	return logs
}
