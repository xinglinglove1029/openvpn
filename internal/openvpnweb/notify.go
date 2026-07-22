package openvpnweb

import (
	"bytes"
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

	"github.com/gavintan/gopkg/tools"
	"github.com/spf13/viper"
)

type NotifyEvent struct {
	Event         string
	Vip           string
	Vip6          string
	Rip           string
	Rip6          string
	CommonName    string
	Username      string
	BytesReceived float64
	BytesSent     float64
	TimeUnix      int64
	TimeDuration  int64
}

type NotifyLog struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Event     string    `json:"event"`
	Provider  string    `json:"provider"`
	Username  string    `json:"username"`
	Success   bool      `json:"success"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

func NotifyClientEvent(event NotifyEvent) error {
	if !viper.GetBool("system.notify.enabled") {
		return nil
	}

	webhook := strings.TrimSpace(viper.GetString("system.notify.webhook"))
	provider := notifyProvider()
	if webhook == "" {
		err := fmt.Errorf("notify webhook is empty")
		recordNotifyLog(event, provider, false, err.Error())
		return err
	}

	title := notifyTitle(event.Event)
	content := buildNotifyMarkdown(title, event)
	var err error

	switch provider {
	case "wecom", "workwechat", "wechat", "qywechat", "企业微信":
		err = sendWecomNotify(webhook, content)
	case "dingtalk", "dingding", "閽夐拤":
		err = sendDingTalkNotify(webhook, title, content)
	default:
		err = fmt.Errorf("unsupported notify provider: %s", provider)
	}

	if err != nil {
		recordNotifyLog(event, provider, false, err.Error())
		return err
	}

	recordNotifyLog(event, provider, true, "notify sent")
	return nil
}

func notifyProvider() string {
	provider := strings.ToLower(strings.TrimSpace(viper.GetString("system.notify.provider")))
	if provider == "" {
		return "dingtalk"
	}
	return provider
}

func notifyTitle(event string) string {
	switch strings.ToLower(event) {
	case "connect", "online":
		return "OpenVPN 用户上线"
	case "disconnect", "offline":
		return "OpenVPN 用户下线"
	default:
		return "OpenVPN 用户事件"
	}
}

func buildNotifyMarkdown(title string, event NotifyEvent) string {
	username := event.Username
	if username == "" || username == "UNDEF" {
		username = event.CommonName
	}

	lines := []string{
		"### " + title,
		"- 用户：" + emptyValue(username),
		"- 客户端：" + emptyValue(event.CommonName),
		"- VPN IP：" + emptyValue(joinNonEmpty(event.Vip, event.Vip6)),
		"- 来源 IP：" + emptyValue(joinNonEmpty(event.Rip, event.Rip6)),
	}

	if event.TimeUnix > 0 {
		lines = append(lines, "- 上线时间："+time.Unix(event.TimeUnix, 0).Format("2006-01-02 15:04:05"))
	}
	if event.TimeDuration > 0 {
		lines = append(lines, "- 在线时长："+(time.Duration(event.TimeDuration)*time.Second).String())
	}
	if event.BytesReceived > 0 || event.BytesSent > 0 {
		lines = append(lines, "- 下行流量："+tools.FormatBytes(event.BytesReceived))
		lines = append(lines, "- 上行流量："+tools.FormatBytes(event.BytesSent))
	}
	lines = append(lines, "- 通知时间："+time.Now().Format("2006-01-02 15:04:05"))

	return strings.Join(lines, "\n")
}

func emptyValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func joinNonEmpty(values ...string) string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			items = append(items, value)
		}
	}
	return strings.Join(items, " / ")
}

func sendDingTalkNotify(webhook, title, content string) error {
	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  content,
		},
		"at": map[string]any{
			"isAtAll": viper.GetBool("system.notify.mention_all"),
		},
	}

	return postRobotMessage(dingTalkSignedWebhook(webhook), payload)
}

func dingTalkSignedWebhook(webhook string) string {
	secret := strings.TrimSpace(viper.GetString("system.notify.secret"))
	if secret == "" {
		return webhook
	}

	timestamp := time.Now().UnixMilli()
	message := fmt.Sprintf("%d\n%s", timestamp, secret)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	sign := url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	separator := "?"
	if strings.Contains(webhook, "?") {
		separator = "&"
	}

	return fmt.Sprintf("%s%stimestamp=%d&sign=%s", webhook, separator, timestamp, sign)
}

func sendWecomNotify(webhook, content string) error {
	if viper.GetBool("system.notify.mention_all") {
		content += "\n<@all>"
	}

	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": content,
		},
	}

	return postRobotMessage(webhook, payload)
}

func postRobotMessage(webhook string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(webhook, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("robot webhook returned status %d", resp.StatusCode)
	}

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.ErrCode != 0 {
		return fmt.Errorf("robot webhook error %d: %s", result.ErrCode, result.ErrMsg)
	}

	return nil
}

func LogNotifyError(event NotifyEvent, err error) {
	if err != nil {
		logger.Error(context.Background(), "send %s notify failed: %s", event.Event, err)
	}
}

func recordNotifyLog(event NotifyEvent, provider string, success bool, message string) {
	if db == nil {
		return
	}

	username := strings.TrimSpace(event.Username)
	if username == "" || username == "UNDEF" {
		username = strings.TrimSpace(event.CommonName)
	}
	if username == "" {
		username = "unknown"
	}

	logItem := NotifyLog{
		Event:    event.Event,
		Provider: provider,
		Username: username,
		Success:  success,
		Message:  message,
	}
	if err := db.WithContext(context.Background()).Create(&logItem).Error; err != nil {
		logger.Error(context.Background(), "record notify log failed: %s", err)
	}
}

func queryNotifyLogs(limit int) []NotifyLog {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	logs := make([]NotifyLog, 0)
	if db == nil {
		return logs
	}

	if err := db.WithContext(context.Background()).Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		logger.Error(context.Background(), "query notify logs failed: %s", err)
		return []NotifyLog{}
	}
	return logs
}
