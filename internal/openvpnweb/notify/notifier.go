// Package notify 实现了多渠道消息通知的统一抽象。
// 所有渠道都实现 Notifier 接口，Manager 负责按 type 注册和派发。
package notify

import (
	"context"
	"encoding/json"
	"time"
)

// ChannelType 通知渠道类型常量
const (
	ChannelWebhook   = "webhook"   // 通用 Webhook（自定义 HTTP POST）
	ChannelEmail     = "email"     // 邮件（SMTP）
	ChannelDingTalk  = "dingtalk"  // 钉钉群机器人
	ChannelFeishu    = "feishu"    // 飞书群机器人
	ChannelWeCom     = "wecom"     // 企业微信群机器人
	ChannelDiscord   = "discord"   // Discord Incoming Webhook
	ChannelSlack     = "slack"     // Slack Incoming Webhook
	ChannelTelegram  = "telegram"  // Telegram Bot API
	ChannelMattermost = "mattermost" // Mattermost Incoming Webhook
)

// AllChannelTypes 列出所有支持的渠道类型
var AllChannelTypes = []string{
	ChannelWebhook,
	ChannelEmail,
	ChannelDingTalk,
	ChannelFeishu,
	ChannelWeCom,
	ChannelDiscord,
	ChannelSlack,
	ChannelTelegram,
	ChannelMattermost,
}

// ChannelTypeLabel 渠道的中文显示名
func ChannelTypeLabel(t string) string {
	switch t {
	case ChannelWebhook:
		return "通用 Webhook"
	case ChannelEmail:
		return "邮件"
	case ChannelDingTalk:
		return "钉钉"
	case ChannelFeishu:
		return "飞书"
	case ChannelWeCom:
		return "企业微信"
	case ChannelDiscord:
		return "Discord"
	case ChannelSlack:
		return "Slack"
	case ChannelTelegram:
		return "Telegram"
	case ChannelMattermost:
		return "Mattermost"
	default:
		return t
	}
}

// ChannelTypeIcon 渠道对应的 lucide-react 图标名（前端使用）
func ChannelTypeIcon(t string) string {
	switch t {
	case ChannelWebhook:
		return "Webhook"
	case ChannelEmail:
		return "Mail"
	case ChannelDingTalk:
		return "BellRing"
	case ChannelFeishu:
		return "MessageSquare"
	case ChannelWeCom:
		return "MessageCircle"
	case ChannelDiscord:
		return "MessageSquare"
	case ChannelSlack:
		return "Hash"
	case ChannelTelegram:
		return "Send"
	case ChannelMattermost:
		return "Radio"
	default:
		return "Bell"
	}
}

// Message 统一的通知消息体
type Message struct {
	// Title 消息标题（Markdown 的标题/邮件 Subject/Slack 标题 等）
	Title string
	// Content 消息正文（Markdown 格式）
	Content string
	// Event 原始事件名（connect / disconnect / test 等）
	Event string
	// Username 关联的用户名（仅作日志参考）
	Username string
	// 收件人（仅邮件渠道使用；其他渠道一般从 config 中读取）
	To []string
	// 扩展元数据：渠道可读取 Extra 中的字段
	Extra map[string]string
}

// Notifier 渠道实现统一接口
type Notifier interface {
	// Type 返回渠道类型（ChannelWebhook / ChannelEmail 等）
	Type() string
	// Send 发送消息，返回错误即视为发送失败（会被记录到日志）
	Send(ctx context.Context, msg Message, config json.RawMessage) error
	// TestConfig 校验 config JSON 是否合法（必填项是否齐全），返回错误信息
	TestConfig(config json.RawMessage) error
}

// 渠道 HTTP 请求统一超时
const httpTimeout = 6 * time.Second
