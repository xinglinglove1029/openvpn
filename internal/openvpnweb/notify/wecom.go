package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// WeComNotifier 企业微信群机器人
// config 字段：
//   webhook      必填，企业微信机器人 Webhook 地址
//   mention_all 可选，@所有人
type WeComNotifier struct{}

func (WeComNotifier) Type() string { return ChannelWeCom }

type wecomConfig struct {
	Webhook    string `json:"webhook"`
	MentionAll bool   `json:"mention_all"`
}

func (WeComNotifier) TestConfig(raw json.RawMessage) error {
	var c wecomConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.Webhook) == "" {
		return fmt.Errorf("webhook 不能为空")
	}
	return nil
}

func (WeComNotifier) Send(ctx context.Context, msg Message, raw json.RawMessage) error {
	var c wecomConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.Webhook) == "" {
		return fmt.Errorf("企业微信 webhook 不能为空")
	}

	content := msg.Content
	if c.MentionAll {
		content += "\n<@all>"
	}

	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": content,
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Webhook, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doHTTP(req)
}
