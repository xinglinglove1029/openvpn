package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/wneessen/go-mail"
)

// EmailNotifier 邮件渠道：基于 SMTP 发送
// config 字段：
//   host           必填，SMTP 主机
//   port           必填，端口（1-65535）
//   username       可选，SMTP 用户名
//   password       可选，SMTP 密码（明文存储在 DB；若为空则尝试无认证）
//   from           必填，发件人邮箱
//   to             必填，收件人列表（[]string 或 字符串，多个用逗号分隔）
//   subject_prefix 可选，主题前缀
//   security       可选，"" | "tls" | "ssl"
type EmailNotifier struct{}

func (EmailNotifier) Type() string { return ChannelEmail }

type emailConfig struct {
	Host          string   `json:"host"`
	Port          int      `json:"port"`
	Username      string   `json:"username"`
	Password      string   `json:"password"`
	From          string   `json:"from"`
	To            []string `json:"to"`
	SubjectPrefix string   `json:"subject_prefix"`
	Security      string   `json:"security"`
}

func (EmailNotifier) TestConfig(raw json.RawMessage) error {
	var c emailConfig
	if err := unmarshalEmailConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("SMTP 主机不能为空")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("SMTP 端口必须是 1-65535")
	}
	if strings.TrimSpace(c.From) == "" {
		return fmt.Errorf("发件人邮箱不能为空")
	}
	if len(c.To) == 0 {
		return fmt.Errorf("收件人不能为空")
	}
	return nil
}

func (EmailNotifier) Send(ctx context.Context, msg Message, raw json.RawMessage) error {
	var c emailConfig
	if err := unmarshalEmailConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.Host) == "" || c.Port == 0 || strings.TrimSpace(c.From) == "" {
		return fmt.Errorf("邮件渠道配置不完整（host/port/from 必填）")
	}

	// 新版通知渠道：密码明文存储在 DB，不再做 AES 解密兼容
	password := c.Password

	m := mail.NewMsg()
	if err := m.From(c.From); err != nil {
		return fmt.Errorf("发件人地址非法：%w", err)
	}
	// 优先用 msg.To（一次性测试场景），否则用 config.To
	recipients := msg.To
	if len(recipients) == 0 {
		recipients = c.To
	}
	if len(recipients) == 0 {
		return fmt.Errorf("没有收件人")
	}
	if err := m.To(recipients...); err != nil {
		return fmt.Errorf("收件人地址非法：%w", err)
	}

	subject := c.SubjectPrefix + msg.Title
	if msg.Title == "" {
		subject = c.SubjectPrefix + "OpenVPN 通知"
	}
	m.Subject(subject)

	// 支持 raw_html：当 Extra["raw_html"]=="true" 时，Content 已是完整 HTML，直接使用
	// 否则按默认规则：标题当 H3，正文当 HTML 段落（转义后放入 <pre>）
	var htmlBody string
	if msg.Extra != nil && msg.Extra["raw_html"] == "true" {
		htmlBody = msg.Content
	} else {
		htmlBody = "<h3>" + escapeHTML(msg.Title) + "</h3>" +
			"<pre style=\"font-family:inherit;white-space:pre-wrap;\">" + escapeHTML(msg.Content) + "</pre>"
	}
	m.SetBodyString(mail.TypeTextHTML, htmlBody)

	// 处理附件（本地文件路径列表）
	for _, filePath := range msg.Attachments {
		if strings.TrimSpace(filePath) == "" {
			continue
		}
		// 先校验文件是否存在，避免 AttachFile 静默忽略后 DialAndSend 时报错难以定位
		if _, statErr := os.Stat(filePath); statErr != nil {
			return fmt.Errorf("附件不存在 [%s]：%w", filePath, statErr)
		}
		m.AttachFile(filePath)
	}

	opts := []mail.Option{mail.WithPort(c.Port)}
	if c.Username != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover))
		opts = append(opts, mail.WithUsername(c.Username))
		if password != "" {
			opts = append(opts, mail.WithPassword(password))
		}
	}
	client, err := mail.NewClient(c.Host, opts...)
	if err != nil {
		log.Printf("[email] 创建 SMTP 客户端失败: %v (host=%s, port=%d, security=%s)", err, c.Host, c.Port, c.Security)
		return err
	}
	switch c.Security {
	case "tls":
		client.SetTLSPolicy(mail.TLSMandatory)
	case "ssl":
		client.SetSSL(true)
	default:
		client.SetTLSPolicy(mail.TLSOpportunistic)
	}

	log.Printf("[email] 准备发送邮件: from=%s, to=%v, subject=%q", c.From, recipients, c.SubjectPrefix+msg.Title)

	// 分步发送，便于排查问题
	done := make(chan error, 1)
	go func() {
		if dialErr := client.DialWithContext(ctx); dialErr != nil {
			done <- fmt.Errorf("SMTP 连接失败: %w", dialErr)
			return
		}
		defer client.Close()
		if sendErr := client.Send(m); sendErr != nil {
			done <- fmt.Errorf("SMTP 发送失败: %w", sendErr)
			return
		}
		done <- nil
	}()
	select {
	case err := <-done:
		if err != nil {
			log.Printf("[email] 邮件发送失败: %v", err)
		} else {
			log.Printf("[email] 邮件发送成功: to=%v, subject=%q", recipients, c.SubjectPrefix+msg.Title)
		}
		return err
	case <-ctx.Done():
		log.Printf("[email] 邮件发送超时: %v", ctx.Err())
		return ctx.Err()
	}
}

// unmarshalEmailConfig 解析邮件配置，兼容 to 字段为 string 或 []string 的情况
func unmarshalEmailConfig(raw json.RawMessage, c *emailConfig) error {
	var rawMap map[string]any
	if err := json.Unmarshal(raw, &rawMap); err != nil {
		return fmt.Errorf("配置 JSON 解析失败：%w", err)
	}

	if host, ok := rawMap["host"].(string); ok {
		c.Host = host
	}
	if port, ok := rawMap["port"].(float64); ok {
		c.Port = int(port)
	}
	if username, ok := rawMap["username"].(string); ok {
		c.Username = username
	}
	if password, ok := rawMap["password"].(string); ok {
		c.Password = password
	}
	if from, ok := rawMap["from"].(string); ok {
		c.From = from
	}
	if subjectPrefix, ok := rawMap["subject_prefix"].(string); ok {
		c.SubjectPrefix = subjectPrefix
	}
	if security, ok := rawMap["security"].(string); ok {
		c.Security = security
	}

	// 兼容 to 字段为 string 或 []string
	if toVal, ok := rawMap["to"]; ok {
		switch v := toVal.(type) {
		case string:
			parts := strings.FieldsFunc(v, func(r rune) bool {
				return r == ',' || r == ';' || r == '\n' || r == ' '
			})
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					c.To = append(c.To, p)
				}
			}
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					s = strings.TrimSpace(s)
					if s != "" {
						c.To = append(c.To, s)
					}
				}
			}
		}
	}

	return nil
}

func escapeHTML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}
