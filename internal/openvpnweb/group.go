package openvpnweb

import (
	"context"
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// GroupRole 用户组-角色多对多关联表
// 一个用户组可绑定多个角色，组内用户的权限 = 用户直接角色权限 ∪ 组角色权限（并集）
type GroupRole struct {
	GroupID   uint      `gorm:"column:group_id;primaryKey" json:"groupId" form:"groupId"`
	RoleID    uint      `gorm:"column:role_id;primaryKey" json:"roleId" form:"roleId"`
	CreatedAt time.Time `gorm:"column:created_at" json:"createdAt"`
}

func (GroupRole) TableName() string { return "group_role" }

type Group struct {
	ID       uint    `gorm:"primarykey" json:"id" form:"id"`
	Name     string  `json:"name" form:"name"`
	ParentID *uint   `json:"parent_id" form:"parent_id"`
	Config   *string `json:"config" form:"config"`
	// RoleID 已废弃：保留字段用于数据迁移兼容，实际多角色关联存储在 group_role 表
	// 新代码应使用 group_role 表查询组绑定的角色
	RoleID    *uint     `gorm:"column:role_id;default:NULL" json:"roleId" form:"roleId"`
	Users     []User    `gorm:"foreignKey:Gid;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	CreatedAt time.Time `json:"createdAt" form:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" form:"updatedAt"`
}

func (g *Group) BeforeCreate(tx *gorm.DB) (err error) {
	if g.ParentID == nil && g.Name != "Default" {
		return errors.New("必须指定父节点")
	}

	if g.ParentID != nil {
		var parent Group
		if err := tx.First(&parent, *g.ParentID).Error; err != nil {
			return errors.New("父节点不存在")
		}
	}

	return nil
}

func (g *Group) Create() error {
	result := db.Create(&g)
	return result.Error
}

func (g *Group) Get(id string) Group {
	result := db.First(&g, id)
	if result.Error != nil {
		logger.Error(context.Background(), result.Error.Error())
		return Group{}
	}

	return *g
}

func (g *Group) All() []Group {
	var groups []Group

	result := db.Model(&Group{}).WithContext(context.Background()).Find(&groups)
	if result.Error != nil {
		logger.Error(context.Background(), result.Error.Error())
		return []Group{}
	}

	return groups
}

func (g *Group) Update() error {
	if g.ID == 0 {
		return errors.New("分组 ID 不能为空")
	}

	updates := map[string]interface{}{
		"name": g.Name,
	}
	if g.Config != nil {
		updates["config"] = g.Config
	}

	// role_id 已废弃：组的多角色关联通过 group_role 表管理（roleAssignGroupsHandler）
	// Group.Update 不再处理 role_id 字段

	if g.ID == 1 {
		updates["parent_id"] = nil
	} else if g.ParentID != nil {
		if *g.ParentID == g.ID {
			return errors.New("上级分组不能选择自己")
		}

		var parent Group
		if err := db.First(&parent, *g.ParentID).Error; err != nil {
			return errors.New("父节点不存在")
		}

		var groups []Group
		if err := db.Find(&groups).Error; err != nil {
			return err
		}
		if hasDescendantGroup(groups, g.ID, *g.ParentID) {
			return errors.New("上级分组不能选择自己的子分组")
		}

		updates["parent_id"] = *g.ParentID
	} else {
		return errors.New("必须指定父节点")
	}

	result := db.Model(&Group{}).Where("id = ?", g.ID).Updates(updates)
	return result.Error
}

func hasDescendantGroup(groups []Group, groupID uint, targetID uint) bool {
	for _, group := range groups {
		if group.ParentID == nil || *group.ParentID != groupID {
			continue
		}
		if group.ID == targetID || hasDescendantGroup(groups, group.ID, targetID) {
			return true
		}
	}
	return false
}

func (g *Group) Delete(id string) error {
	groupID, err := strconv.ParseUint(id, 10, 64)
	if err != nil || groupID == 0 {
		return errors.New("分组ID不正确")
	}

	if groupID == 1 {
		return errors.New("默认分组不能删除")
	}

	var group Group
	if err := db.First(&group, groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("分组不存在")
		}
		return err
	}

	var childCount int64
	if err := db.Model(&Group{}).Where("parent_id = ?", groupID).Count(&childCount).Error; err != nil {
		return err
	}
	if childCount > 0 {
		return errors.New("存在子分组，不能删除")
	}

	var userCount int64
	if err := db.Model(&User{}).Where("gid = ?", groupID).Count(&userCount).Error; err != nil {
		return err
	}
	if userCount > 0 {
		return errors.New("分组已绑定用户，不能删除")
	}

	result := db.Unscoped().Delete(&Group{}, groupID)
	return result.Error
}

func (g *Group) GetUsers(id string) []User {
	var users []User

	result := db.WithContext(context.Background()).
		Where(`
			gid IN (
				WITH RECURSIVE group_tree AS (
					SELECT id, parent_id
					FROM "group"
					WHERE id = ?
		
					UNION ALL
		
					SELECT g.id, g.parent_id
					FROM "group" g
					JOIN group_tree gt ON g.parent_id = gt.id
				)
				SELECT id FROM group_tree
			)
		`, id).
		Find(&users)

	if result.Error != nil {
		logger.Error(context.Background(), result.Error.Error())
		return []User{}
	}

	// 标记内置 admin 用户，前端用于隐藏删除按钮
	for i := range users {
		if users[i].Username == adminUsername {
			users[i].IsBuiltin = true
		}
	}

	return users
}

func (Group) TableName() string {
	return "group"
}

// GetSubtreeIDs 获取分组及其所有子节点的 ID 列表（包含自身）
// 使用递归 CTE 查询分组树
func GetSubtreeIDs(groupID uint) []uint {
	var ids []uint
	result := db.Raw(`
		WITH RECURSIVE group_tree AS (
			SELECT id, parent_id
			FROM "group"
			WHERE id = ?

			UNION ALL

			SELECT g.id, g.parent_id
			FROM "group" g
			JOIN group_tree gt ON g.parent_id = gt.id
		)
		SELECT id FROM group_tree
	`, groupID).Scan(&ids)
	if result.Error != nil {
		logger.Error(context.Background(), result.Error.Error())
		return []uint{}
	}
	return ids
}
