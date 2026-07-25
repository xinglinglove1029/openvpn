package openvpnweb

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// NotificationChannel 通知渠道配置（多渠道维护）
// config 字段为渠道特定的 JSON，结构由 notify 包内各 Notifier 自行定义
type NotificationChannel struct {
	ID        uint            `gorm:"primarykey" json:"id"`
	Name      string          `gorm:"size:64;not null" json:"name"`
	Type      string          `gorm:"size:32;not null;index" json:"type"`
	Enabled   bool            `gorm:"default:true" json:"enabled"`
	Config    json.RawMessage `gorm:"type:text" json:"config"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// TableName 指定表名
func (NotificationChannel) TableName() string { return "notification_channels" }

// BeforeSave 字段清洗
func (c *NotificationChannel) BeforeSave(tx *gorm.DB) error {
	c.Name = strings.TrimSpace(c.Name)
	c.Type = strings.TrimSpace(c.Type)
	return nil
}

// AllChannels 取出所有渠道（按 ID 升序）
func (c *NotificationChannel) All() []NotificationChannel {
	out := make([]NotificationChannel, 0)
	if db == nil {
		return out
	}
	if err := db.WithContext(context.Background()).Order("id ASC").Find(&out).Error; err != nil {
		logger.Error(context.Background(), "query notification channels failed: "+err.Error())
		return []NotificationChannel{}
	}
	return out
}

// EnabledChannels 取出所有启用的渠道
func (c *NotificationChannel) EnabledChannels() []NotificationChannel {
	out := make([]NotificationChannel, 0)
	if db == nil {
		return out
	}
	if err := db.WithContext(context.Background()).Where("enabled = ?", true).Order("id ASC").Find(&out).Error; err != nil {
		logger.Error(context.Background(), "query enabled notification channels failed: "+err.Error())
		return []NotificationChannel{}
	}
	return out
}

// Get 取单条
func (c *NotificationChannel) Get(id uint) (NotificationChannel, error) {
	var ch NotificationChannel
	if db == nil {
		return ch, errors.New("数据库未初始化")
	}
	if err := db.WithContext(context.Background()).First(&ch, id).Error; err != nil {
		return ch, err
	}
	return ch, nil
}

// Create 新建（name 在渠道内唯一）
func (c *NotificationChannel) Create() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("渠道名称不能为空")
	}
	if strings.TrimSpace(c.Type) == "" {
		return errors.New("渠道类型不能为空")
	}
	if len(c.Config) == 0 {
		c.Config = json.RawMessage("{}")
	}
	return db.WithContext(context.Background()).Create(c).Error
}

// Update 更新（不修改 createdAt）
func (c *NotificationChannel) Update() error {
	if c.ID == 0 {
		return errors.New("缺少渠道 ID")
	}
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("渠道名称不能为空")
	}
	if strings.TrimSpace(c.Type) == "" {
		return errors.New("渠道类型不能为空")
	}
	if len(c.Config) == 0 {
		c.Config = json.RawMessage("{}")
	}
	return db.WithContext(context.Background()).Model(c).Updates(map[string]any{
		"name":      c.Name,
		"type":      c.Type,
		"enabled":   c.Enabled,
		"config":    c.Config,
		"updatedAt": time.Now(),
	}).Error
}

// Delete 删除单条
func (c *NotificationChannel) Delete() error {
	if c.ID == 0 {
		return errors.New("缺少渠道 ID")
	}
	return db.WithContext(context.Background()).Delete(c).Error
}
