package openvpnweb

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"openvpn-web/internal/openvpnweb/ai"
)

// AIChatMessage is an independently persisted AI chat message. It intentionally does not reuse the VPN history table.
type AIChatMessage struct {
	ID        uint      `gorm:"primaryKey"`
	Username  string    `gorm:"size:128;not null;index:idx_ai_chat_user_session_created,priority:1"`
	SessionID string    `gorm:"size:128;not null;index:idx_ai_chat_user_session_created,priority:2"`
	Role      string    `gorm:"size:16;not null"`
	Content   string    `gorm:"type:text;not null"`
	CreatedAt time.Time `gorm:"not null;index:idx_ai_chat_user_session_created,priority:3"`
}

func (AIChatMessage) TableName() string { return "ai_chat_messages" }

// MigrateAIChatHistory creates the chat-history table without touching existing AI settings or VPN history.
func MigrateAIChatHistory(database *gorm.DB) error {
	if err := database.AutoMigrate(&AIChatMessage{}); err != nil {
		return err
	}
	// MySQL 不支持 CREATE INDEX IF NOT EXISTS，需先检查索引是否存在
	exists, err := indexExists(database, "ai_chat_messages", "idx_ai_chat_user_created")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return database.Exec(`CREATE INDEX idx_ai_chat_user_created ON ai_chat_messages(username, created_at DESC, id DESC)`).Error
}

type sqliteAIChatHistoryStore struct {
	db *gorm.DB
}

func NewSQLiteAIChatHistoryStore(database *gorm.DB) ai.ChatHistoryStore {
	return &sqliteAIChatHistoryStore{db: database}
}

func (s *sqliteAIChatHistoryStore) Append(ctx context.Context, username, sessionID string, message ai.HistoryMessage) error {
	return s.db.WithContext(ctx).Create(&AIChatMessage{
		Username:  username,
		SessionID: sessionID,
		Role:      message.Role,
		Content:   message.Content,
	}).Error
}

func (s *sqliteAIChatHistoryStore) SaveExchange(ctx context.Context, username, sessionID string, userMessage, assistantMessage ai.HistoryMessage) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, message := range []ai.HistoryMessage{userMessage, assistantMessage} {
			if err := tx.Create(&AIChatMessage{
				Username: username, SessionID: sessionID, Role: message.Role, Content: message.Content,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *sqliteAIChatHistoryStore) List(ctx context.Context, username, sessionID string) ([]ai.HistoryMessage, error) {
	var records []AIChatMessage
	if err := s.db.WithContext(ctx).
		Where("username = ? AND session_id = ?", username, sessionID).
		Order("created_at ASC").
		Order("id ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	messages := make([]ai.HistoryMessage, 0, len(records))
	for _, record := range records {
		messages = append(messages, ai.HistoryMessage{Role: record.Role, Content: record.Content})
	}
	return messages, nil
}

func (s *sqliteAIChatHistoryStore) LatestSession(ctx context.Context, username string) (string, error) {
	var record AIChatMessage
	err := s.db.WithContext(ctx).
		Where("username = ?", username).
		Order("created_at DESC").
		Order("id DESC").
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return record.SessionID, nil
}

func (s *sqliteAIChatHistoryStore) Delete(ctx context.Context, username, sessionID string) error {
	return s.db.WithContext(ctx).
		Where("username = ? AND session_id = ?", username, sessionID).
		Delete(&AIChatMessage{}).Error
}
