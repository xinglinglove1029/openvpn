package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DiscordNotifier Discord Incoming Webhook
// config 字段：
//   webhook      必填，Server Settings -> Integrations -> Webhooks 创建的 URL
//   username     可选，自定义发送者名称
//   avatar_url   可选，自定义头像 URL
type DiscordNotifier struct{}

func (DiscordNotifier) Type() string { return ChannelDiscord }

type discordConfig struct {
	Webhook   string `json:"webhook"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}

func (DiscordNotifier) TestConfig(raw json.RawMessage) error {
	var c discordConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.Webhook) == "" {
		return fmt.Errorf("webhook 不能为空")
	}
	return nil
}

func (DiscordNotifier) Send(ctx context.Context, msg Message, raw json.RawMessage) error {
	var c discordConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.Webhook) == "" {
		return fmt.Errorf("Discord webhook 不能为空")
	}
	// Discord Markdown 与 GitHub Flavored Markdown 兼容
	body := map[string]any{
		"content": fmt.Sprintf("### %s\n%s", msg.Title, msg.Content),
	}
	if strings.TrimSpace(c.Username) != "" {
		body["username"] = c.Username
	}
	if strings.TrimSpace(c.AvatarURL) != "" {
		body["avatar_url"] = c.AvatarURL
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Webhook, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doHTTP(req)
}
