package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// 通用 Webhook 渠道：把消息以 JSON POST 到指定 URL
// config 字段：
//   url        必填，POST 目标地址
//   method     可选，默认 POST
//   headers    可选，自定义请求头（map[string]string）
//   body_template 可选，自定义 JSON 模板（占位符 {{title}} {{content}} {{event}} {{username}}）
//   secret     可选，启用 HMAC-SHA256 签名后放在 X-Signature 头（与 GitHub Webhook 风格一致）
//   mention_all 可选，bool，是否 @所有人
type WebhookNotifier struct{}

func (WebhookNotifier) Type() string { return ChannelWebhook }

type webhookConfig struct {
	URL          string            `json:"url"`
	Method       string            `json:"method"`
	Headers      map[string]string `json:"headers"`
	BodyTemplate string            `json:"body_template"`
	Secret       string            `json:"secret"`
	MentionAll   bool              `json:"mention_all"`
}

func (WebhookNotifier) TestConfig(raw json.RawMessage) error {
	var c webhookConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.URL) == "" {
		return fmt.Errorf("url 不能为空")
	}
	if c.Method == "" {
		c.Method = http.MethodPost
	}
	return nil
}

func (w WebhookNotifier) Send(ctx context.Context, msg Message, raw json.RawMessage) error {
	var c webhookConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.URL) == "" {
		return fmt.Errorf("webhook url 不能为空")
	}
	method := strings.ToUpper(strings.TrimSpace(c.Method))
	if method == "" {
		method = http.MethodPost
	}

	var body []byte
	if strings.TrimSpace(c.BodyTemplate) != "" {
		rendered := c.BodyTemplate
		rendered = strings.ReplaceAll(rendered, "{{title}}", msg.Title)
		rendered = strings.ReplaceAll(rendered, "{{content}}", msg.Content)
		rendered = strings.ReplaceAll(rendered, "{{event}}", msg.Event)
		rendered = strings.ReplaceAll(rendered, "{{username}}", msg.Username)
		body = []byte(rendered)
	} else {
		payload := map[string]any{
			"title":       msg.Title,
			"content":     msg.Content,
			"event":       msg.Event,
			"username":    msg.Username,
			"mention_all": c.MentionAll,
		}
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	if strings.TrimSpace(c.Secret) != "" {
		sig := hmacSHA256Hex(c.Secret, body)
		req.Header.Set("X-Signature", "sha256="+sig)
	}

	return doHTTP(req)
}

// 通用：把 json.RawMessage 解到 v
func unmarshalConfig(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// 通用：执行 HTTP 请求并检查状态码
func doHTTP(req *http.Request) error {
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 读前 512 字节错误体便于诊断
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(buf))
	}
	return nil
}
