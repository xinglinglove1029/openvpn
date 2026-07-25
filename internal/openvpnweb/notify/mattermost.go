package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// MattermostNotifier Mattermost Incoming Webhook
// config 字段：
//   webhook  必填，Main Menu -> Integrations -> Incoming Webhook URL
//   channel  可选，覆盖默认频道（如 "town-square"）
//   username 可选，自定义发送者
//   icon_url 可选，自定义头像
type MattermostNotifier struct{}

func (MattermostNotifier) Type() string { return ChannelMattermost }

type mattermostConfig struct {
	Webhook  string `json:"webhook"`
	Channel  string `json:"channel"`
	Username string `json:"username"`
	IconURL  string `json:"icon_url"`
}

func (MattermostNotifier) TestConfig(raw json.RawMessage) error {
	var c mattermostConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.Webhook) == "" {
		return fmt.Errorf("webhook 不能为空")
	}
	return nil
}

func (MattermostNotifier) Send(ctx context.Context, msg Message, raw json.RawMessage) error {
	var c mattermostConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.Webhook) == "" {
		return fmt.Errorf("Mattermost webhook 不能为空")
	}

	body := map[string]any{
		"text": fmt.Sprintf("#### %s\n%s", msg.Title, msg.Content),
	}
	if strings.TrimSpace(c.Channel) != "" {
		body["channel"] = c.Channel
	}
	if strings.TrimSpace(c.Username) != "" {
		body["username"] = c.Username
	}
	if strings.TrimSpace(c.IconURL) != "" {
		body["icon_url"] = c.IconURL
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Webhook, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doHTTP(req)
}
