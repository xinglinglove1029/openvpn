package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DingTalkNotifier 钉钉群机器人
// config 字段：
//   webhook      必填，钉钉机器人 Webhook 地址
//   secret       可选，加签密钥（启用加签安全设置时必填）
//   mention_all 可选，@所有人
type DingTalkNotifier struct{}

func (DingTalkNotifier) Type() string { return ChannelDingTalk }

type dingTalkConfig struct {
	Webhook    string `json:"webhook"`
	Secret     string `json:"secret"`
	MentionAll bool   `json:"mention_all"`
}

func (DingTalkNotifier) TestConfig(raw json.RawMessage) error {
	var c dingTalkConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.Webhook) == "" {
		return fmt.Errorf("webhook 不能为空")
	}
	return nil
}

func (DingTalkNotifier) Send(ctx context.Context, msg Message, raw json.RawMessage) error {
	var c dingTalkConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.Webhook) == "" {
		return fmt.Errorf("钉钉 webhook 不能为空")
	}

	endpoint := c.Webhook
	if secret := strings.TrimSpace(c.Secret); secret != "" {
		timestamp := time.Now().UnixMilli()
		stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(stringToSign))
		sign := url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
		sep := "?"
		if strings.Contains(endpoint, "?") {
			sep = "&"
		}
		endpoint = fmt.Sprintf("%s%stimestamp=%d&sign=%s", endpoint, sep, timestamp, sign)
	}

	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": msg.Title,
			"text":  msg.Content,
		},
		"at": map[string]any{
			"isAtAll": c.MentionAll,
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := doHTTP(req); err != nil {
		return err
	}
	return nil
}
