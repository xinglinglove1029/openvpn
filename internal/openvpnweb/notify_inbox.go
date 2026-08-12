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

// resolveNotifyUserID 根据 username 解析 user_notify_read 表中使用的 user_id
// admin/system 已纳入 user 表，统一查 users 表获取真实 ID
func resolveNotifyUserID(username string) uint {
	if username == "" {
		return 0
	}
	return GetUserIDByUsername(username)
}

// getUserNotifyRead 获取用户已读进度；不存在时返回零值
func getUserNotifyRead(username string) UserNotifyRead {
	if username == "" {
		return UserNotifyRead{}
	}
	userID := resolveNotifyUserID(username)
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
// 兼容内置账号（admin/system）通过保留 ID 写入
func markUserNotifyRead(username string, lastID uint) (UserNotifyRead, error) {
	if username == "" {
		return UserNotifyRead{}, errors.New("username is empty")
	}
	userID := resolveNotifyUserID(username)
	if userID == 0 {
		return UserNotifyRead{}, errors.New("user not found")
	}
	now := time.Now().Unix()

	// 先查询是否存在记录
	var existing UserNotifyRead
	findErr := db.WithContext(context.Background()).
		Where("user_id = ? AND scope = ?", userID, "default").
		First(&existing).Error

	if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return UserNotifyRead{}, findErr
	}

	if errors.Is(findErr, gorm.ErrRecordNotFound) {
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

// RepairNotifyReadUserIDs 修复历史 user_notify_read 表中 user_id=0 或使用保留 ID 的记录
// admin 已纳入 user 表，统一迁移到真实 user.id；system 账号已移除，历史 system 记录迁移到 admin
func RepairNotifyReadUserIDs() {
	if db == nil {
		return
	}

	// 修复 admin 记录：user_id=0 或保留 ID 且 username=admin → admin 真实 ID
	adminID := uint(0)
	if adminUsername != "" {
		adminID = GetUserIDByUsername(adminUsername)
		if adminID > 0 {
			result := db.WithContext(context.Background()).
				Model(&UserNotifyRead{}).
				Where("username = ? AND (user_id = ? OR user_id = ?)", adminUsername, 0, AdminAuditOperatorID).
				Update("user_id", adminID)
			if result.Error != nil {
				logger.Error(context.Background(), "修复 admin user_notify_read user_id 失败: %s", result.Error.Error())
			} else if result.RowsAffected > 0 {
				logger.Error(context.Background(), "已修复 %d 条 admin user_notify_read 记录的 user_id → %d", result.RowsAffected, adminID)
			}
		}
	}

	// 历史 system 记录：username=system 或保留 ID=SystemAuditOperatorID → 迁移到 adminID（若存在）
	// 策略：逐条迁移，对同一 scope 下 admin 已存在的记录（UNIQUE 冲突）直接删除 system 那条（admin 已读优先级更高）
	if adminID > 0 {
		// 1. 先尝试把不会冲突的记录迁过去（WHERE admin 同 scope 下不存在）
		// 注意：子查询不能直接引用目标表 user_notify_read（MySQL Error 1093），
		// 需用派生表包裹一层，SQLite/PostgreSQL/MySQL 均兼容。
		result := db.WithContext(context.Background()).Exec(`
			UPDATE user_notify_read
			SET user_id = ?, username = ?
			WHERE (username = ? OR user_id = ? OR user_id = ?)
			  AND username != ?
			  AND NOT EXISTS (
				SELECT 1 FROM (
					SELECT scope FROM user_notify_read AS t2
					WHERE t2.username = ?
				) AS admin_scopes
				WHERE admin_scopes.scope = user_notify_read.scope
			  )
		`, adminID, adminUsername, "system", 0, SystemAuditOperatorID, adminUsername, adminUsername)
		if result.Error != nil {
			logger.Error(context.Background(), "修复 system user_notify_read (无冲突迁移) 失败: %s", result.Error.Error())
		} else if result.RowsAffected > 0 {
			logger.Error(context.Background(), "已修复 %d 条 system user_notify_read 记录 → admin (id=%d, 无冲突)", result.RowsAffected, adminID)
		}

		// 2. 剩余冲突记录：system 同 scope 下 admin 已存在 → 直接删除 system 那条（admin 已读状态保留）
		result = db.WithContext(context.Background()).Exec(`
			DELETE FROM user_notify_read
			WHERE (username = ? OR user_id = ? OR user_id = ?)
			  AND username != ?
		`, "system", 0, SystemAuditOperatorID, adminUsername)
		if result.Error != nil {
			logger.Error(context.Background(), "修复 system user_notify_read (删除冲突剩余) 失败: %s", result.Error.Error())
		} else if result.RowsAffected > 0 {
			logger.Error(context.Background(), "已删除 %d 条 system user_notify_read 冲突记录 (admin 同 scope 已存在)", result.RowsAffected)
		}
	}
}
