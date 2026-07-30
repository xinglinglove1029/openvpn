package openvpnweb

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// UserNotifyRead 记录每个用户对站内信（NotifyLog）的已读进度
// Key: (UserID, Scope)；Scope 预留用于将来按业务线隔离
type UserNotifyRead struct {
	ID            uint   `gorm:"primarykey" json:"id"`
	UserID        uint   `gorm:"uniqueIndex:idx_user_scope;default:0;index" json:"userId"`
	Username      string `gorm:"size:64" json:"username"`
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
	userID := GetUserIDByUsername(username)
	if userID == 0 {
		return UserNotifyRead{}
	}
	var rec UserNotifyRead
	if err := db.WithContext(context.Background()).
		Where("user_id = ? AND scope = ?", userID, "default").
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
	userID := GetUserIDByUsername(username)
	if userID == 0 {
		return UserNotifyRead{}, errors.New("user not found")
	}
	now := time.Now().Unix()

	// 先查询是否存在记录，避免依赖 ON CONFLICT（旧表可能缺唯一索引）
	// 注意：旧表唯一约束是 (username, scope)，用 username 查询以匹配约束
	var existing UserNotifyRead
	err := db.WithContext(context.Background()).
		Where("username = ? AND scope = ?", username, "default").
		First(&existing).Error

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return UserNotifyRead{}, err
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 插入新记录
		rec := UserNotifyRead{
			UserID:        userID,
			Username:      username,
			Scope:         "default",
			LastReadID:    lastID,
			UpdatedAtUnix: now,
		}
		if err := db.WithContext(context.Background()).Create(&rec).Error; err != nil {
			return UserNotifyRead{}, err
		}
	} else {
		// 更新已有记录，取较大的 lastReadID
		newLastReadID := lastID
		if existing.LastReadID > newLastReadID {
			newLastReadID = existing.LastReadID
		}
		if err := db.WithContext(context.Background()).
			Model(&existing).
			Updates(map[string]interface{}{
				"last_read_id":    newLastReadID,
				"updated_at_unix": now,
				"username":        username,
			}).Error; err != nil {
			return UserNotifyRead{}, err
		}
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
		accessibleUserIDs, skipFilter := GetAccessibleUserIDs(currentUsername)
		if !skipFilter {
			query = query.Where("user_id IN ?", accessibleUserIDs)
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
		accessibleUserIDs, skipFilter := GetAccessibleUserIDs(currentUsername)
		if !skipFilter {
			query = query.Where("user_id IN ?", accessibleUserIDs)
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
