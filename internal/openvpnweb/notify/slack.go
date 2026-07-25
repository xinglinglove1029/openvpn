package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// SlackNotifier Slack Incoming Webhook
// config 字段：
//   webhook      必填，Slack App -> Incoming Webhooks URL
//   channel      可选，覆盖默认频道（"#ops" 或 "@user"）
//   username     可选，自定义发送者
//   icon_emoji   可选，自定义 emoji 如 ":robot_face:"
type SlackNotifier struct{}

func (SlackNotifier) Type() string { return ChannelSlack }

type slackConfig struct {
	Webhook   string `json:"webhook"`
	Channel   string `json:"channel"`
	Username  string `json:"username"`
	IconEmoji string `json:"icon_emoji"`
}

func (SlackNotifier) TestConfig(raw json.RawMessage) error {
	var c slackConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.Webhook) == "" {
		return fmt.Errorf("webhook 不能为空")
	}
	return nil
}

func (SlackNotifier) Send(ctx context.Context, msg Message, raw json.RawMessage) error {
	var c slackConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.Webhook) == "" {
		return fmt.Errorf("Slack webhook 不能为空")
	}

	text := fmt.Sprintf("*%s*\n%s", msg.Title, msg.Content)
	body := map[string]any{
		"text":      text,
		"mrkdwn":    true,
	}
	if strings.TrimSpace(c.Channel) != "" {
		body["channel"] = c.Channel
	}
	if strings.TrimSpace(c.Username) != "" {
		body["username"] = c.Username
	}
	if strings.TrimSpace(c.IconEmoji) != "" {
		body["icon_emoji"] = c.IconEmoji
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Webhook, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doHTTP(req)
}
