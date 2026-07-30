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
// Provider 存放渠道类型（如 email / dingtalk / webhook）便于按类型筛选
// ChannelName 存放渠道名称（用户在 UI 中给渠道起的名称）便于人眼识别
type NotifyLog struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	Event       string    `json:"event"`
	Provider    string    `json:"provider"`    // 渠道类型：email / dingtalk / webhook ...
	ChannelName string    `json:"channelName"` // 渠道名称：用户自定义的名称
	UserID      uint      `gorm:"index;default:0" json:"userId"`
	Username    string    `json:"username"`
	Success     bool      `json:"success"`
	Message     string    `json:"message"`
	CreatedAt   time.Time `json:"createdAt"`
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

func recordNotifyLog(event NotifyEvent, provider, channelName string, success bool, message string) {
	if db == nil {
		return
	}
	username := displayUsername(event)
	userID := GetUserIDByUsername(username)

	logItem := NotifyLog{
		Event:       event.Event,
		Provider:    provider,
		ChannelName: channelName,
		UserID:      userID,
		Username:    username,
		Success:     success,
		Message:     message,
	}
	if err := db.WithContext(context.Background()).Create(&logItem).Error; err != nil {
		logger.Error(context.Background(), "record notify log failed: %s", err)
		return
	}

	// 站内信实时广播：通过全局事件总线解耦推送
	// 任意监听 notify:new 主题的模块都会收到；当前由 WsHub 转发到 WebSocket 客户端
	Bus().Publish("notify:new", map[string]any{
		"id":          logItem.ID,
		"event":       logItem.Event,
		"provider":    logItem.Provider,
		"channelName": logItem.ChannelName,
		"username":    logItem.Username,
		"success":     logItem.Success,
		"message":     logItem.Message,
		"createdAt":   logItem.CreatedAt,
	})
}

// migrateNotifyLogChannelName 在启动时把旧 channel_type 列数据复制到新 channel_name 列
// 仅在 channel_name 为空但旧 channel_type 有值时执行，避免覆盖新数据
func migrateNotifyLogChannelName() {
	if db == nil {
		return
	}
	// AutoMigrate 已经把新列加上了，这里做一次性数据迁移
	// GORM 用 Column 名是 snake_case，所以 SQL 里用 channel_name / channel_type
	if err := db.WithContext(context.Background()).Exec(
		"UPDATE notify_logs SET channel_name = channel_type WHERE channel_name IS NULL OR channel_name = ''",
	).Error; err != nil {
		// 旧表可能没有 channel_type 列，忽略错误
		logger.Error(context.Background(), "migrate notify_logs channel_name: %s", err)
	}
}

func queryNotifyLogs(limit int, currentUsername string, isAdmin bool) []NotifyLog {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	logs := make([]NotifyLog, 0)
	if db == nil {
		return logs
	}

	query := db.WithContext(context.Background()).Model(&NotifyLog{})

	// 数据权限过滤：普通用户只能看到自己所在分组及下级分组用户的站内信
	if !isAdmin && currentUsername != "" {
		accessibleUserIDs, skipFilter := GetAccessibleUserIDs(currentUsername)
		if !skipFilter {
			query = query.Where("user_id IN ?", accessibleUserIDs)
		}
	}

	if err := query.Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		logger.Error(context.Background(), "query notify logs failed: %s", err)
		return []NotifyLog{}
	}
	return logs
}
