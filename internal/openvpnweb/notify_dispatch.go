package openvpnweb

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"

	"openvpn-web/internal/openvpnweb/notify"
)

// 启动时把所有渠道注册到全局 Manager
func registerNotifiers() {
	m := notify.Global()
	m.Register(notify.WebhookNotifier{})
	m.Register(notify.EmailNotifier{})
	m.Register(notify.DingTalkNotifier{})
	m.Register(notify.FeishuNotifier{})
	m.Register(notify.WeComNotifier{})
	m.Register(notify.DiscordNotifier{})
	m.Register(notify.SlackNotifier{})
	m.Register(notify.TelegramNotifier{})
	m.Register(notify.MattermostNotifier{})
}

// 把 DB 里的 NotificationChannel 列表转为 notify.Channel 列表
func toNotifyChannels(items []NotificationChannel) []notify.Channel {
	out := make([]notify.Channel, 0, len(items))
	for _, c := range items {
		out = append(out, notify.Channel{
			ID:      c.ID,
			Name:    c.Name,
			Type:    c.Type,
			Enabled: c.Enabled,
			Config:  c.Config,
		})
	}
	return out
}

// dispatchNotification 把消息派发到所有启用的渠道，并把结果写入 NotifyLog。
// 即便没有任何渠道被启用，也会写一条"系统通知（无渠道）"的记录，
// 保证站内信能呈现每一次事件，便于审计和后续补发。
func dispatchNotification(event NotifyEvent, title, content string) {
	if db == nil {
		return
	}
	enabled := (&NotificationChannel{}).EnabledChannels()
	if len(enabled) == 0 {
		// 无启用渠道时，操作人使用 admin（system 账号已移除）
		systemOperator := "admin"
		if adminUsername != "" {
			systemOperator = adminUsername
		}
		systemName := "超级管理员"
		if adminName := viper.GetString("system.base.admin_name"); adminName != "" {
			systemName = adminName
		}
		recordNotifyLog(event, systemOperator, systemName, true, content)
		return
	}

	msg := notify.Message{
		Title:    title,
		Content:  content,
		Event:    event.Event,
		Username: displayUsername(event),
	}

	results := notify.Global().Dispatch(context.Background(), toNotifyChannels(enabled), msg)
	for _, r := range results {
		logMsg := ""
		if r.Success {
			logMsg = content
		} else {
			logMsg = r.Error
		}
		recordNotifyLog(event, r.ChannelType, r.ChannelName, r.Success, logMsg)
	}
}

// displayUsername 解析通知展示用的用户名（沿用旧逻辑）
func displayUsername(event NotifyEvent) string {
	u := strings.TrimSpace(event.Username)
	if u == "" || u == "UNDEF" {
		u = strings.TrimSpace(event.CommonName)
	}
	if u == "" {
		u = "unknown"
	}
	return u
}

// Lifecycle notifications must never delay OpenVPN hooks. Dispatch can include
// SMTP/webhook I/O, so it is handled by one bounded background worker. History
// records and firewall updates remain synchronous at their own API layer; only
// the optional notification fan-out is lossy under sustained overload.
const lifecycleNotificationQueueSize = 128

var lifecycleNotificationQueue = make(chan NotifyEvent, lifecycleNotificationQueueSize)

func init() {
	go func() {
		for event := range lifecycleNotificationQueue {
			LogNotifyError(event, NotifyClientEvent(event))
		}
	}()
}

func enqueueLifecycleNotification(event NotifyEvent) {
	select {
	case lifecycleNotificationQueue <- event:
	default:
		logger.Warn(context.Background(), "lifecycle notification queue full; dropping %s event for %s", event.Event, displayUsername(event))
	}
}

// NotifyClientEvent is kept synchronous for explicit administrative actions
// such as channel tests. OpenVPN lifecycle routes use enqueueLifecycleNotification.
func NotifyClientEvent(event NotifyEvent) error {
	title := notifyTitle(event.Event)
	content := buildNotifyMarkdown(title, event)
	dispatchNotification(event, title, content)
	return nil
}

// 测试发送：单独给某条渠道发一条测试消息
func sendChannelTestMessage(channelID uint) error {
	var ch NotificationChannel
	if err := db.WithContext(context.Background()).First(&ch, channelID).Error; err != nil {
		return err
	}
	if !ch.Enabled {
		return fmt.Errorf("渠道未启用，请先启用后再测试")
	}
	n, ok := notify.Global().Get(ch.Type)
	if !ok {
		return fmt.Errorf("未注册的渠道类型：%s", ch.Type)
	}
	// 校验 config 必填项
	if err := n.TestConfig(ch.Config); err != nil {
		return err
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	title := "OpenVPN 通知测试"
	content := fmt.Sprintf(
		"这是一条来自 OpenVPN 管理后台的测试通知。\n\n- 渠道：%s\n- 类型：%s\n- 时间：%s",
		ch.Name, notify.ChannelTypeLabel(ch.Type), now,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	event := NotifyEvent{
		Event:    "test",
		Username: "admin",
	}
	if err := n.Send(ctx, notify.Message{
		Title: title, Content: content, Event: "test", Username: "admin",
	}, ch.Config); err != nil {
		recordNotifyLog(event, ch.Type, ch.Name, false, fmt.Sprintf("测试发送失败：%s", err.Error()))
		return err
	}
	recordNotifyLog(event, ch.Type, ch.Name, true, fmt.Sprintf("测试发送成功（渠道：%s）", ch.Name))
	return nil
}

// sendUserEmail 通过通知渠道系统发送用户邮件
// 找到第一个启用的邮件渠道，用它发送到指定邮箱，支持附件和 HTML 内容
// 同时会把发送结果记录到站内信
func sendUserEmail(toEmail, subject, htmlContent string, attachments []string, username string, eventType string) error {
	if strings.TrimSpace(toEmail) == "" {
		return fmt.Errorf("收件人邮箱为空")
	}

	enabled := (&NotificationChannel{}).EnabledChannels()
	var emailCh *NotificationChannel
	for i := range enabled {
		if enabled[i].Type == notify.ChannelEmail {
			emailCh = &enabled[i]
			break
		}
	}

	event := NotifyEvent{
		Event:    eventType,
		Username: username,
	}

	if emailCh == nil {
		recordNotifyLog(event, "email", "", false, "未配置启用的邮件通知渠道，无法发送用户注册邮件")
		return fmt.Errorf("未配置启用的邮件通知渠道")
	}

	n, ok := notify.Global().Get(emailCh.Type)
	if !ok {
		recordNotifyLog(event, emailCh.Type, emailCh.Name, false, "未注册的邮件渠道类型")
		return fmt.Errorf("未注册的邮件渠道类型: %s", emailCh.Type)
	}

	msg := notify.Message{
		Title:       subject,
		Content:     htmlContent,
		Event:       eventType,
		Username:    username,
		To:          []string{toEmail},
		Attachments: attachments,
		Extra: map[string]string{
			"raw_html": "true",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := n.Send(ctx, msg, emailCh.Config); err != nil {
		recordNotifyLog(event, emailCh.Type, emailCh.Name, false, fmt.Sprintf("用户注册邮件发送失败：%s", err.Error()))
		return err
	}

	recordNotifyLog(event, emailCh.Type, emailCh.Name, true, fmt.Sprintf("用户注册邮件发送成功（收件人：%s）", toEmail))
	return nil
}

// 留一个用于确保 json 包被引用（未直接使用，但保留备用）
var _ = json.Marshal
