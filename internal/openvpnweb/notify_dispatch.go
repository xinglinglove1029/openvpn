package openvpnweb

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
		recordNotifyLog(event, "system", true, "系统通知（当前未配置任何启用的通知渠道，仅记录在站内信）")
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
		recordNotifyLog(event, r.ChannelName, r.Success, r.Error)
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

// 旧 API 兼容：直接派发到所有启用的渠道（取代原 NotifyClientEvent 单渠道逻辑）
// 无论是否配置了渠道，都会走 dispatchNotification；后者会保证站内信至少有一条记录
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
	if err := n.Send(ctx, notify.Message{
		Title: title, Content: content, Event: "test", Username: "admin",
	}, ch.Config); err != nil {
		return err
	}
	return nil
}

// 留一个用于确保 json 包被引用（未直接使用，但保留备用）
var _ = json.Marshal
