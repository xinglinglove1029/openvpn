package openvpnweb

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserNotifyRead 记录每个用户对站内信（NotifyLog）的已读进度
// Key: (Username, Scope)；Scope 预留用于将来按业务线隔离
type UserNotifyRead struct {
	ID            uint   `gorm:"primarykey" json:"id"`
	Username      string `gorm:"uniqueIndex:idx_user_scope;size:64" json:"username"`
	Scope         string `gorm:"uniqueIndex:idx_user_scope;size:32;default:default" json:"scope"`
	LastReadID    uint   `gorm:"default:0" json:"lastReadId"`
	UpdatedAtUnix int64  `json:"updatedAt"`
}

func (UserNotifyRead) TableName() string {
	return "user_notify_read"
}

// getUserNotifyRead 获取用户已读进度；不存在时返回零值
func getUserNotifyRead(username string) UserNotifyRead {
	if username == "" {
		return UserNotifyRead{}
	}
	var rec UserNotifyRead
	if err := db.WithContext(context.Background()).
		Where("username = ? AND scope = ?", username, "default").
		First(&rec).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logger.Error(context.Background(), "query user notify read failed: %s", err.Error())
		}
		return UserNotifyRead{}
	}
	return rec
}

// markUserNotifyRead 将用户已读进度推进到指定 id（取较大值）
func markUserNotifyRead(username string, lastID uint) (UserNotifyRead, error) {
	if username == "" {
		return UserNotifyRead{}, errors.New("username is empty")
	}
	now := time.Now().Unix()
	rec := UserNotifyRead{
		Username:      username,
		Scope:         "default",
		LastReadID:    lastID,
		UpdatedAtUnix: now,
	}
	err := db.WithContext(context.Background()).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "username"}, {Name: "scope"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"last_read_id":    gorm.Expr("MAX(last_read_id, ?)", lastID),
				"updated_at_unix": now,
			}),
		}).
		Create(&rec).Error
	if err != nil {
		return UserNotifyRead{}, err
	}
	return getUserNotifyRead(username), nil
}

// countUnreadNotifyLogs 计算全局 NotifyLog 中 id > lastReadID 的数量
// 数据权限：普通用户只统计自己所在分组及下级分组用户的未读数
func countUnreadNotifyLogs(lastReadID uint, currentUsername string, isAdmin bool) int64 {
	if db == nil {
		return 0
	}
	query := db.WithContext(context.Background()).Model(&NotifyLog{}).Where("id > ?", lastReadID)

	// 数据权限过滤
	if !isAdmin && currentUsername != "" {
		accessibleUsers, skipFilter := GetAccessibleUsernames(currentUsername)
		if !skipFilter {
			query = query.Where("username IN ?", accessibleUsers)
		}
	}

	var n int64
	if err := query.Count(&n).Error; err != nil {
		logger.Error(context.Background(), "count unread notify logs failed: %s", err.Error())
		return 0
	}
	return n
}

// maxNotifyLogID 获取当前最大 NotifyLog id；表为空时返回 0
// 数据权限：普通用户只统计自己所在分组及下级分组用户的最大 id
func maxNotifyLogID(currentUsername string, isAdmin bool) uint {
	if db == nil {
		return 0
	}
	query := db.WithContext(context.Background()).Model(&NotifyLog{})

	// 数据权限过滤
	if !isAdmin && currentUsername != "" {
		accessibleUsers, skipFilter := GetAccessibleUsernames(currentUsername)
		if !skipFilter {
			query = query.Where("username IN ?", accessibleUsers)
		}
	}

	var maxID *uint
	if err := query.Select("MAX(id)").Scan(&maxID).Error; err != nil {
		logger.Error(context.Background(), "max notify log id failed: %s", err.Error())
		return 0
	}
	if maxID == nil {
		return 0
	}
	return *maxID
}
