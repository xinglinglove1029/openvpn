package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// TelegramNotifier Telegram Bot
// config 字段：
//   bot_token  必填，从 @BotFather 获取
//   chat_id    必填，目标 chat id（个人/群组/频道均可）
//   parse_mode 可选，"Markdown" | "MarkdownV2" | "HTML"（默认 MarkdownV2）
//   disable_web_page_preview 可选
type TelegramNotifier struct{}

func (TelegramNotifier) Type() string { return ChannelTelegram }

type telegramConfig struct {
	BotToken             string `json:"bot_token"`
	ChatID               string `json:"chat_id"`
	ParseMode            string `json:"parse_mode"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
}

func (TelegramNotifier) TestConfig(raw json.RawMessage) error {
	var c telegramConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.BotToken) == "" {
		return fmt.Errorf("bot_token 不能为空")
	}
	if strings.TrimSpace(c.ChatID) == "" {
		return fmt.Errorf("chat_id 不能为空")
	}
	return nil
}

func (TelegramNotifier) Send(ctx context.Context, msg Message, raw json.RawMessage) error {
	var c telegramConfig
	if err := unmarshalConfig(raw, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.BotToken) == "" || strings.TrimSpace(c.ChatID) == "" {
		return fmt.Errorf("Telegram bot_token/chat_id 不能为空")
	}

	parseMode := strings.TrimSpace(c.ParseMode)
	if parseMode == "" {
		parseMode = "Markdown"
	}
	// Telegram MarkdownV2 对很多字符需要转义；这里默认用 Markdown（更宽松）
	text := fmt.Sprintf("*%s*\n%s", msg.Title, msg.Content)
	body := map[string]any{
		"chat_id":    c.ChatID,
		"text":       text,
		"parse_mode": parseMode,
	}
	if c.DisableWebPagePreview {
		body["disable_web_page_preview"] = true
	}
	payload, _ := json.Marshal(body)
	endpoint := "https://api.telegram.org/bot" + c.BotToken + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doHTTP(req)
}
