package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// FeishuNotifier 飞书群机器人
// config 字段：
//   webhook      必填，飞书机器人 Webhook 地址
//   secret       可选，加签密钥（启用签名校验时必填）
type FeishuNotifier struct{}

func (FeishuNotifier) Type() string { return ChannelFeishu }

type feishuConfig struct {
	Webhook string `json:"webhook"`
	Secret  string `json:"secret"`
}

func (FeishuNotifier) TestConfig(raw json.RawMessage) error {
	var c feishuConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.Webhook) == "" {
		return fmt.Errorf("webhook 不能为空")
	}
	return nil
}

func (FeishuNotifier) Send(ctx context.Context, msg Message, raw json.RawMessage) error {
	var c feishuConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.Webhook) == "" {
		return fmt.Errorf("飞书 webhook 不能为空")
	}

	// 飞书签名校验：timestamp + key 做 HMAC-SHA256 后 base64
	timestamp := time.Now().Unix()
	body := map[string]any{
		"timestamp": timestamp,
		"msg_type":  "interactive",
		"card": map[string]any{
			"header": map[string]any{
				"title": map[string]string{
					"tag":     "plain_text",
					"content": msg.Title,
				},
				"template": "blue",
			},
			"elements": []any{
				map[string]any{
					"tag": "markdown",
					"content": map[string]string{
						"tag":     "lark_md",
						"content": msg.Content,
					},
				},
			},
		},
	}
	if secret := strings.TrimSpace(c.Secret); secret != "" {
		stringToSign := fmt.Sprintf("%v\n%s", timestamp, secret)
		mac := hmac.New(sha256.New, []byte(stringToSign))
		mac.Write([]byte(""))
		sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		body["sign"] = sign
	}

	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Webhook, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := doHTTP(req); err != nil {
		return err
	}

	// 飞书会返回 {"StatusCode":0,"StatusMessage":"success"}，此处 doHTTP 仅判断状态码
	return nil
}
