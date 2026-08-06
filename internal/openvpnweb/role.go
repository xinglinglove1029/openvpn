package openvpnweb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/microcosm-cc/bluemonday"
	"gorm.io/gorm"
)

// roleCodeRegex 角色代码格式正则：以字母开头，仅包含字母、数字、下划线
var roleCodeRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)

// Role 角色模型
type Role struct {
	ID          uint   `gorm:"primarykey" json:"id"`
	Name        string `gorm:"column:name;size:64;not null" json:"name" form:"name"`
	Code        string `gorm:"column:code;size:64;uniqueIndex;not null" json:"code" form:"code"`
	Description string `gorm:"column:description;size:255" json:"description" form:"description"`
	IsBuiltin   bool   `gorm:"column:is_builtin;default:false" json:"isBuiltin" form:"isBuiltin"`
	// IsEnable 使用 *bool 指针：避免 GORM default:true 与 bool 零值冲突
	// 创建时若前端未传 isEnable（nil），GORM 用 default:true 兜底；前端显式传 false 时保留 false
	IsEnable  *bool     `gorm:"column:is_enable;default:true" json:"isEnable" form:"isEnable"`
	Sort      int       `gorm:"column:sort;default:0" json:"sort" form:"sort"`
	CreatedAt time.Time `json:"createdAt,omitempty" form:"createdAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty" form:"updatedAt,omitempty"`
}

// BeforeSave 角色保存前钩子：使用 bluemonday 净化 name 与 description 字段，防止 XSS
func (r *Role) BeforeSave(tx *gorm.DB) (err error) {
	p := bluemonday.UGCPolicy()
	r.Name = p.Sanitize(r.Name)
	r.Description = p.Sanitize(r.Description)
	return nil
}

// Permission 权限定义模型
// 内置权限（IsBuiltin=true）由 seed 维护，不可删除，code/type 不可修改；
// 非内置权限支持完整 CRUD，用于运行时调整菜单结构
type Permission struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	ParentID  uint      `gorm:"column:parent_id;default:0" json:"parentId"`
	Name      string    `gorm:"column:name;size:64;not null" json:"name" form:"name"`
	Code      string    `gorm:"column:code;size:64;uniqueIndex;not null" json:"code" form:"code"`
	Type      string    `gorm:"column:type;size:16;default:'button'" json:"type" form:"type"` // menu | button
	Path      string    `gorm:"column:path;size:255" json:"path" form:"path"`
	Icon      string    `gorm:"column:icon;size:64" json:"icon" form:"icon"`
	Sort      int       `gorm:"column:sort;default:0" json:"sort" form:"sort"`
	IsBuiltin bool      `gorm:"column:is_builtin;default:false" json:"isBuiltin" form:"isBuiltin"`
	CreatedAt time.Time `json:"createdAt,omitempty" form:"createdAt,omitempty"`
}

// RolePermission 角色-权限关联（多对多）
type RolePermission struct {
	RoleID       uint `gorm:"column:role_id;primaryKey" json:"roleId"`
	PermissionID uint `gorm:"column:permission_id;primaryKey" json:"permissionId"`
}

func (Role) TableName() string {
	return "role"
}

func (Permission) TableName() string {
	return "permission"
}

func (RolePermission) TableName() string {
	return "role_permission"
}

// 内置角色 code 常量
const (
	BuiltinRoleAdministrator = "administrator"
	BuiltinRoleUser          = "user"
)

// permissionSeed 权限清单（菜单 12 项 + 按钮约 40 项）
// 字段顺序：parentCode, code, name, type, path, icon, sort
type permissionSeedItem struct {
	ParentCode string
	Code       string
	Name       string
	Type       string
	Path       string
	Icon       string
	Sort       int
}

// 菜单权限
var menuPermissions = []permissionSeedItem{
	{"", "menu:overview", "概览", "menu", "/overview", "LayoutDashboard", 1},
	{"", "menu:users", "账号管理", "menu", "/users", "Users", 2},
	{"", "menu:roles", "角色管理", "menu", "/roles", "ShieldCheck", 3},
	{"", "menu:permissions", "权限管理", "menu", "/permissions", "KeyRound", 4},
	{"", "menu:clients", "客户端", "menu", "/clients", "Smartphone", 5},
	{"", "menu:firewall", "防火墙", "menu", "/firewall", "Shield", 6},
	{"", "menu:history", "连接历史", "menu", "/history", "History", 7},
	{"", "menu:certs", "证书", "menu", "/certs", "FileKey", 8},
	{"", "menu:audit", "操作审计", "menu", "/audit", "FileText", 9},
	{"", "menu:channels", "通知渠道", "menu", "/channels", "BellRing", 10},
	{"", "menu:notifications", "站内信", "menu", "/notifications", "Bell", 11},
	{"", "menu:settings", "系统设置", "menu", "/settings", "Settings", 12},
	{"", "menu:profile", "个人中心", "menu", "/profile", "User", 13},
}

// 按钮权限（按资源分组）
var buttonPermissions = []permissionSeedItem{
	// 用户管理（10）
	{"menu:users", "user:view", "查看用户", "button", "", "", 1},
	{"menu:users", "user:create", "创建用户", "button", "", "", 2},
	{"menu:users", "user:update", "更新用户", "button", "", "", 3},
	{"menu:users", "user:delete", "删除用户", "button", "", "", 4},
	{"menu:users", "user:enable", "启用用户", "button", "", "", 5},
	{"menu:users", "user:disable", "禁用用户", "button", "", "", 6},
	{"menu:users", "user:reset_password", "重置密码", "button", "", "", 7},
	{"menu:users", "user:reset_mfa", "重置 MFA", "button", "", "", 8},
	{"menu:users", "user:import", "导入用户", "button", "", "", 9},
	{"menu:users", "user:export", "导出用户", "button", "", "", 10},
	// 分组（5）
	{"menu:users", "group:view", "查看分组", "button", "", "", 11},
	{"menu:users", "group:create", "创建分组", "button", "", "", 12},
	{"menu:users", "group:update", "更新分组", "button", "", "", 13},
	{"menu:users", "group:delete", "删除分组", "button", "", "", 14},
	{"menu:users", "group:config", "分组配置", "button", "", "", 15},
	// 客户端（5）
	{"menu:clients", "client:view", "查看客户端", "button", "", "", 1},
	{"menu:clients", "client:create", "创建客户端", "button", "", "", 2},
	{"menu:clients", "client:download", "下载客户端", "button", "", "", 3},
	{"menu:clients", "client:delete", "删除客户端", "button", "", "", 4},
	{"menu:clients", "client:regenerate", "重新生成客户端", "button", "", "", 5},
	// 客户端包（1）
	{"menu:clients", "client:manage_all", "管理客户端安装包", "button", "", "", 5},
	// 在线客户端查看（1）
	{"menu:overview", "client:view_online", "查看在线客户端", "button", "", "", 3},
	// 防火墙（5）
	{"menu:firewall", "firewall:view", "查看防火墙", "button", "", "", 1},
	{"menu:firewall", "firewall:create", "创建规则", "button", "", "", 2},
	{"menu:firewall", "firewall:update", "更新规则", "button", "", "", 3},
	{"menu:firewall", "firewall:delete", "删除规则", "button", "", "", 4},
	{"menu:firewall", "firewall:clear", "清空规则", "button", "", "", 5},
	// 证书（2）
	{"menu:certs", "cert:view", "查看证书", "button", "", "", 1},
	{"menu:certs", "cert:renew", "续签证书", "button", "", "", 2},
	// 审计（1）
	{"menu:audit", "audit:view", "查看审计", "button", "", "", 1},
	// 系统设置（7）
	{"menu:settings", "settings:view", "查看设置", "button", "", "", 1},
	{"menu:settings", "settings:base", "基础控制Tab", "button", "", "", 2},
	{"menu:settings", "settings:ldap", "LDAP认证Tab", "button", "", "", 3},
	{"menu:settings", "settings:openvpn", "OpenVPN参数Tab", "button", "", "", 4},
	{"menu:settings", "settings:service", "服务管理Tab", "button", "", "", 5},
	{"menu:settings", "settings:packages", "客户端安装包Tab", "button", "", "", 6},
	// 系统设置按钮级权限（8）：各Tab操作按钮的独立权限码
	{"settings:base", "settings:base:update", "保存基础控制", "button", "", "", 1},
	{"settings:ldap", "settings:ldap:update", "保存LDAP认证", "button", "", "", 2},
	{"settings:openvpn", "settings:openvpn:update", "保存OpenVPN参数", "button", "", "", 3},
	{"settings:service", "settings:service:restart", "重启OpenVPN服务", "button", "", "", 1},
	{"settings:service", "settings:service:config", "编辑server.conf", "button", "", "", 2},
	{"settings:packages", "settings:packages:upload", "上传安装包", "button", "", "", 1},
	{"settings:packages", "settings:packages:delete", "删除安装包", "button", "", "", 2},
	{"settings:packages", "settings:packages:enable", "启用安装包", "button", "", "", 3},
	// 通知渠道（5）
	{"menu:channels", "channel:view", "查看渠道", "button", "", "", 1},
	{"menu:channels", "channel:create", "创建渠道", "button", "", "", 2},
	{"menu:channels", "channel:update", "更新渠道", "button", "", "", 3},
	{"menu:channels", "channel:delete", "删除渠道", "button", "", "", 4},
	{"menu:channels", "channel:test", "测试渠道", "button", "", "", 5},
	// 服务器（2）
	{"menu:overview", "server:manage", "服务器操作", "button", "", "", 1},
	{"menu:overview", "client:kill", "断开连接", "button", "", "", 2},
	// 角色管理（5）
	{"menu:roles", "role:view", "查看角色", "button", "", "", 1},
	{"menu:roles", "role:create", "创建角色", "button", "", "", 2},
	{"menu:roles", "role:update", "更新角色", "button", "", "", 3},
	{"menu:roles", "role:delete", "删除角色", "button", "", "", 4},
	{"menu:roles", "role:assign_permissions", "分配权限", "button", "", "", 5},
	// 权限查询（1）
	{"menu:roles", "permission:view", "查看权限树", "button", "", "", 6},
	// 角色分配用户与用户组（2）
	{"menu:roles", "role:assign_users", "分配用户", "button", "", "", 7},
	{"menu:roles", "role:assign_groups", "分配用户组", "button", "", "", 8},
	// 历史记录（1）
	{"menu:history", "history:view", "查看连接历史", "button", "", "", 1},
	// 权限管理（1）：CRUD + 排序权限
	{"menu:permissions", "permission:manage", "管理权限", "button", "", "", 1},
}

// 普通用户角色默认权限（菜单 + 按钮）
// 说明：系统设置（menu:settings / settings:*）默认不授予普通用户
//
//	管理员可在"角色管理"页面按需为普通用户分配设置类权限
var defaultUserRoleCodes = []string{
	"menu:overview",
	"menu:clients",
	"menu:history",
	"menu:notifications",
	"menu:profile",
	"client:view",
	"client:download",
	"client:delete",
	"client:regenerate",
	"client:view_online",
	"history:view",
}

// SeedPermissionsAndRoles 初始化权限与内置角色
// - 写入全量权限（菜单 + 按钮）
// - 创建内置 administrator（全权限）与 user（普通用户权限）角色
// - 仅首次初始化：若内置角色已存在 role_permission 记录，则跳过权限写入，保留管理员运行期修改
// - 整个写入操作包在事务中
func SeedPermissionsAndRoles(db *gorm.DB) error {
	// 1. 写入权限定义
	codeToID := make(map[string]uint, 0)
	allPerms := append([]permissionSeedItem{}, menuPermissions...)
	allPerms = append(allPerms, buttonPermissions...)

	// 先写菜单（parentID=0）
	for _, p := range menuPermissions {
		var perm Permission
		if err := db.Where("code = ?", p.Code).First(&perm).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				perm = Permission{
					Name: p.Name, Code: p.Code, Type: p.Type, Path: p.Path, Icon: p.Icon, Sort: p.Sort,
					IsBuiltin: true,
				}
				if err := db.Create(&perm).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		}
		// 更新元数据（已有记录时仅同步 is_builtin，不覆盖运行时管理员对 name/path/icon/sort 的修改）
		db.Model(&perm).Where("id = ?", perm.ID).Updates(map[string]interface{}{
			"is_builtin": true,
		})
		codeToID[p.Code] = perm.ID
	}
	// 再写按钮（按 parentCode 关联）
	for _, p := range buttonPermissions {
		var perm Permission
		if err := db.Where("code = ?", p.Code).First(&perm).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				perm = Permission{
					Name: p.Name, Code: p.Code, Type: p.Type, Sort: p.Sort,
					IsBuiltin: true,
				}
				if err := db.Create(&perm).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		}
		parentID := uint(0)
		if p.ParentCode != "" {
			if pid, ok := codeToID[p.ParentCode]; ok {
				parentID = pid
			} else {
				var pp Permission
				if err := db.Where("code = ?", p.ParentCode).First(&pp).Error; err == nil {
					parentID = pp.ID
					codeToID[p.ParentCode] = pp.ID
				}
			}
		}
		// 更新元数据（已有记录时仅同步 is_builtin 和 parent_id，不覆盖运行时 sort/name 修改）
		db.Model(&perm).Where("id = ?", perm.ID).Updates(map[string]interface{}{
			"type": p.Type, "parent_id": parentID, "is_builtin": true,
		})
		codeToID[p.Code] = perm.ID
	}

	// 2. 创建内置 administrator 角色（全权限）
	adminEnable := true
	var adminRole Role
	// 不使用 FirstOrCreate：SQLite 下并发/钩子场景下可能误触发 INSERT 导致 UNIQUE 冲突
	// 改为显式先查，不存在再创建，存在则同步元数据
	if err := db.WithContext(context.Background()).Where("code = ?", BuiltinRoleAdministrator).First(&adminRole).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		adminRole = Role{
			Name: "系统超管", Code: BuiltinRoleAdministrator, Description: "拥有全部权限的内置超级管理员",
			IsBuiltin: true, IsEnable: &adminEnable, Sort: 0,
		}
		if err := db.WithContext(context.Background()).Create(&adminRole).Error; err != nil {
			return err
		}
	}
	// 同步元数据（即使已存在也确保内置字段正确）
	db.Model(&adminRole).Where("id = ?", adminRole.ID).Updates(map[string]interface{}{
		"name": "系统超管", "description": "拥有全部权限的内置超级管理员", "is_builtin": true, "is_enable": true, "sort": 0,
	})

	// 3. 创建内置 user 角色（普通用户权限）
	userEnable := true
	var userRole Role
	if err := db.WithContext(context.Background()).Where("code = ?", BuiltinRoleUser).First(&userRole).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		userRole = Role{
			Name: "普通用户", Code: BuiltinRoleUser, Description: "仅可访问概览/客户端/历史/站内信/个人中心",
			IsBuiltin: true, IsEnable: &userEnable, Sort: 1,
		}
		if err := db.WithContext(context.Background()).Create(&userRole).Error; err != nil {
			return err
		}
	}
	db.Model(&userRole).Where("id = ?", userRole.ID).Updates(map[string]interface{}{
		"name": "普通用户", "description": "仅可访问概览/客户端/历史/站内信/个人中心", "is_builtin": true, "is_enable": true, "sort": 1,
	})

	// 4. 内置角色权限同步策略：
	//    administrator 与 user 均采用"首次初始化"策略：
	//    - 仅当角色尚未有任何 role_permission 记录时才写入默认权限
	//    - 之后保留管理员运行期修改，不再自动同步，避免管理员移除的权限被回填
	//    - 新增的权限项需管理员在"角色管理"页面手动分配
	return db.Transaction(func(tx *gorm.DB) error {
		// administrator 角色：仅首次初始化时写入全权限
		var adminCount int64
		if err := tx.Model(&RolePermission{}).Where("role_id = ?", adminRole.ID).Count(&adminCount).Error; err != nil {
			return err
		}
		if adminCount == 0 {
			for _, pid := range codeToID {
				if err := tx.FirstOrCreate(&RolePermission{}, RolePermission{RoleID: adminRole.ID, PermissionID: pid}).Error; err != nil {
					return err
				}
			}
		}

		// user 角色：仅首次初始化时写入 defaultUserRoleCodes
		// 管理员后续在"角色管理"页面移除/新增的权限不会被自动覆盖
		var userCount int64
		if err := tx.Model(&RolePermission{}).Where("role_id = ?", userRole.ID).Count(&userCount).Error; err != nil {
			return err
		}
		if userCount == 0 {
			for _, code := range defaultUserRoleCodes {
				pid, ok := codeToID[code]
				if !ok {
					// 权限清单中不存在该 code（可能是配置错误），跳过并记日志
					logger.Error(context.Background(), "SeedPermissionsAndRoles: defaultUserRoleCodes 引用未定义的权限 code: %s", code)
					continue
				}
				if err := tx.FirstOrCreate(&RolePermission{}, RolePermission{RoleID: userRole.ID, PermissionID: pid}).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})
}

// GetDefaultRoleID 获取普通用户角色 ID（用于回填与创建用户默认值）
func GetDefaultRoleID(db *gorm.DB) uint {
	var role Role
	if err := db.Where("code = ?", BuiltinRoleUser).First(&role).Error; err != nil {
		return 0
	}
	return role.ID
}

// LoadRolePermissionCodes 查询角色对应的权限 code 列表
// - role_id 不存在返回 ErrRoleNotFound
// - 角色被禁用返回 ErrRoleDisabled
func LoadRolePermissionCodes(db *gorm.DB, roleID uint) ([]string, error) {
	if roleID == 0 {
		return nil, ErrRoleNotFound
	}
	var role Role
	if err := db.Where("id = ?", roleID).First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}
	// IsEnable 为 *bool，nil 视为启用（DB default:true）
	if role.IsEnable != nil && !*role.IsEnable {
		return nil, ErrRoleDisabled
	}

	var codes []string
	// 使用 INNER JOIN 避免孤儿 role_permission 把 NULL code 塞进结果
	err := db.Table("role_permission").
		Select("permission.code").
		Joins("INNER JOIN permission ON permission.id = role_permission.permission_id").
		Where("role_permission.role_id = ? AND permission.code IS NOT NULL AND permission.code != ''", roleID).
		Pluck("permission.code", &codes).Error
	if err != nil {
		return nil, err
	}
	if codes == nil {
		codes = []string{}
	}
	return codes, nil
}

// RequirePermission 路由级权限校验中间件
// - admin 用户（c.Get("isAdmin") == true）直接放行
// - 否则从 c.Get("permissions") 取权限 code 列表，匹配 code 或 "*" 通过
// - 不匹配返回 403 并写审计日志
func RequirePermission(code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 优先使用 AuthMiddleWare 已设置的 isAdmin 标志，单一真相源
		if isAdmin, ok := c.Get("isAdmin"); ok {
			if b, _ := isAdmin.(bool); b {
				c.Next()
				return
			}
		} else {
			// 兜底：AuthMiddleWare 未设置（理论上不会发生），按 session 判 admin
			if username, ok := sessions.Default(c).Get("user").(string); ok && adminUsername != "" && username == adminUsername {
				c.Next()
				return
			}
		}

		perms, _ := c.Get("permissions")
		codes, _ := perms.([]string)
		for _, p := range codes {
			if p == "*" || p == code {
				c.Next()
				return
			}
		}

		// 写审计日志
		recordAudit(c, "rbac", "deny", code, false, "无权限")

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "无权限"})
	}
}

// hasPermissionCode 检查当前请求用户是否拥有指定权限 code
// - admin 用户（c.Get("isAdmin") == true）直接放行
// - 否则从 c.Get("permissions") 取权限 code 列表，匹配 code 或 "*" 通过
// 用于 handler 内部的细粒度权限判断（如按 action 区分权限的场景）
func hasPermissionCode(c *gin.Context, code string) bool {
	if isAdmin, ok := c.Get("isAdmin"); ok {
		if b, _ := isAdmin.(bool); b {
			return true
		}
	} else {
		// 兜底：AuthMiddleWare 未设置（理论上不会发生），按 session 判 admin
		if username, ok := sessions.Default(c).Get("user").(string); ok && adminUsername != "" && username == adminUsername {
			return true
		}
	}
	perms, _ := c.Get("permissions")
	codes, _ := perms.([]string)
	for _, p := range codes {
		if p == "*" || p == code {
			return true
		}
	}
	return false
}

// requirePermissionCode 在 handler 内部检查权限，无权限时写审计日志并返回 403
// 返回 true 表示有权限可继续执行，false 表示已被拒绝（handler 应直接 return）
func requirePermissionCode(c *gin.Context, code string) bool {
	if hasPermissionCode(c, code) {
		return true
	}
	recordAudit(c, "rbac", "deny", code, false, "无权限")
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "无权限"})
	return false
}

// permissionTreeHandler GET /ovpn/permission/tree 返回权限树
// 权限校验由路由层 RequirePermission("permission:view") 完成，handler 不再重复校验
func permissionTreeHandler(c *gin.Context) {
	var perms []Permission
	if err := db.WithContext(context.Background()).Order("sort ASC, id ASC").Find(&perms).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// 构建树
	type TreeNode struct {
		ID        uint        `json:"id"`
		ParentID  uint        `json:"parentId"`
		Name      string      `json:"name"`
		Code      string      `json:"code"`
		Type      string      `json:"type"`
		Path      string      `json:"path"`
		Icon      string      `json:"icon"`
		Sort      int         `json:"sort"`
		IsBuiltin bool        `json:"isBuiltin"`
		Children  []*TreeNode `json:"children"`
	}

	byID := make(map[uint]*TreeNode)
	roots := make([]*TreeNode, 0)
	for _, p := range perms {
		node := &TreeNode{
			ID: p.ID, ParentID: p.ParentID, Name: p.Name, Code: p.Code, Type: p.Type,
			Path: p.Path, Icon: p.Icon, Sort: p.Sort, IsBuiltin: p.IsBuiltin,
		}
		byID[p.ID] = node
	}
	for _, p := range perms {
		node := byID[p.ID]
		if p.ParentID == 0 {
			roots = append(roots, node)
		} else {
			if parent, ok := byID[p.ParentID]; ok {
				parent.Children = append(parent.Children, node)
			} else {
				roots = append(roots, node)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": roots})
}

// permissionCodeRegex 权限 code 格式正则：以字母开头，仅包含字母、数字、下划线、冒号
var permissionCodeRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_:]*$`)

// parsePermissionIDParam 解析 :id 路径参数为 uint，非法时返回 0 + false
func parsePermissionIDParam(c *gin.Context) (uint, bool) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的权限 ID"})
		return 0, false
	}
	return uint(id), true
}

// permissionCreateHandler POST /ovpn/permission 创建权限节点
// 接收 JSON body: name, code, type(menu|button), path, icon, sort, parentID
// 校验：code 唯一且符合格式，parentID 存在时必须是 menu 类型
// 创建时 IsBuiltin=false
func permissionCreateHandler(c *gin.Context) {
	var body struct {
		Name     string `json:"name"`
		Code     string `json:"code"`
		Type     string `json:"type"`
		Path     string `json:"path"`
		Icon     string `json:"icon"`
		Sort     int    `json:"sort"`
		ParentID uint   `json:"parentId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	// 净化输入
	policy := bluemonday.UGCPolicy()
	body.Name = strings.TrimSpace(policy.Sanitize(body.Name))
	body.Code = strings.TrimSpace(body.Code)
	body.Type = strings.TrimSpace(body.Type)
	body.Path = strings.TrimSpace(body.Path)
	body.Icon = strings.TrimSpace(body.Icon)

	// 校验 name 非空
	if body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "权限名称不能为空"})
		return
	}
	if len(body.Name) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "权限名称长度不能超过 64"})
		return
	}
	// 校验 code 格式
	if body.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "权限编码不能为空"})
		return
	}
	if !permissionCodeRegex.MatchString(body.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "权限编码只能以字母开头，包含字母数字下划线冒号"})
		return
	}
	// 校验 type
	if body.Type != "menu" && body.Type != "button" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "权限类型必须为 menu 或 button"})
		return
	}
	// 校验 sort 范围
	if body.Sort < 0 || body.Sort > 9999 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "排序值需在 0-9999 之间"})
		return
	}
	// 校验 code 唯一
	var exists int64
	db.Model(&Permission{}).Where("code = ?", body.Code).Count(&exists)
	if exists > 0 {
		c.JSON(http.StatusConflict, gin.H{"message": "权限编码已存在"})
		return
	}
	// 校验 parentID：非 0 时必须存在且为 menu 类型
	if body.ParentID != 0 {
		var parent Permission
		if err := db.Where("id = ?", body.ParentID).First(&parent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusBadRequest, gin.H{"message": "父权限不存在"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}
		if parent.Type != "menu" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "父权限必须为 menu 类型"})
			return
		}
	}

	perm := Permission{
		Name: body.Name, Code: body.Code, Type: body.Type, Path: body.Path, Icon: body.Icon,
		Sort: body.Sort, ParentID: body.ParentID, IsBuiltin: false,
	}
	if err := db.Create(&perm).Error; err != nil {
		recordAudit(c, "permission", "create", body.Name, false, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	recordAudit(c, "permission", "create", body.Name, true, fmt.Sprintf("创建权限: %s (code=%s)", body.Name, body.Code))
	c.JSON(http.StatusOK, gin.H{"message": "创建成功", "id": perm.ID})
}

// permissionUpdateHandler PUT /ovpn/permission/:id 更新权限节点
// 允许修改：name, path, icon, sort, parentID
// 内置权限（IsBuiltin=true）：不允许修改 code 和 type（返回 400）
// 非内置权限：允许修改 code（需校验唯一性）和 type
func permissionUpdateHandler(c *gin.Context) {
	id, ok := parsePermissionIDParam(c)
	if !ok {
		return
	}
	var perm Permission
	if err := db.Where("id = ?", id).First(&perm).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": "权限不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	var body struct {
		Name     string `json:"name"`
		Code     string `json:"code"`
		Type     string `json:"type"`
		Path     string `json:"path"`
		Icon     string `json:"icon"`
		Sort     int    `json:"sort"`
		ParentID uint   `json:"parentId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	policy := bluemonday.UGCPolicy()
	body.Name = strings.TrimSpace(policy.Sanitize(body.Name))
	body.Code = strings.TrimSpace(body.Code)
	body.Type = strings.TrimSpace(body.Type)
	body.Path = strings.TrimSpace(policy.Sanitize(body.Path))
	body.Icon = strings.TrimSpace(body.Icon)

	// 校验 name 非空
	if body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "权限名称不能为空"})
		return
	}
	if len(body.Name) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "权限名称长度不能超过 64"})
		return
	}
	// 校验 sort 范围
	if body.Sort < 0 || body.Sort > 9999 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "排序值需在 0-9999 之间"})
		return
	}

	// 内置权限保护：禁止修改 code 与 type
	if perm.IsBuiltin {
		if body.Code != "" && body.Code != perm.Code {
			recordAudit(c, "permission", "update", perm.Name, false, "内置权限代码不允许修改")
			c.JSON(http.StatusBadRequest, gin.H{"message": "内置权限代码不允许修改"})
			return
		}
		if body.Type != "" && body.Type != perm.Type {
			recordAudit(c, "permission", "update", perm.Name, false, "内置权限类型不允许修改")
			c.JSON(http.StatusBadRequest, gin.H{"message": "内置权限类型不允许修改"})
			return
		}
	}

	// 非内置权限允许改 code：校验格式与唯一性
	if !perm.IsBuiltin && body.Code != "" && body.Code != perm.Code {
		if !permissionCodeRegex.MatchString(body.Code) {
			c.JSON(http.StatusBadRequest, gin.H{"message": "权限编码只能以字母开头，包含字母数字下划线冒号"})
			return
		}
		var exists int64
		db.Model(&Permission{}).Where("code = ? AND id != ?", body.Code, perm.ID).Count(&exists)
		if exists > 0 {
			c.JSON(http.StatusConflict, gin.H{"message": "权限编码已存在"})
			return
		}
	}

	// 非内置权限允许改 type：校验取值
	if !perm.IsBuiltin && body.Type != "" && body.Type != perm.Type {
		if body.Type != "menu" && body.Type != "button" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "权限类型必须为 menu 或 button"})
			return
		}
	}

	// 校验 parentID：非 0 时必须存在且为 menu 类型；不能将自己设为父节点
	if body.ParentID != 0 && body.ParentID != perm.ParentID {
		if body.ParentID == perm.ID {
			c.JSON(http.StatusBadRequest, gin.H{"message": "不能将自己设为父节点"})
			return
		}
		var parent Permission
		if err := db.Where("id = ?", body.ParentID).First(&parent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusBadRequest, gin.H{"message": "父权限不存在"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}
		if parent.Type != "menu" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "父权限必须为 menu 类型"})
			return
		}
		// 防止循环：检查 parentID 是否在当前节点的子树中
		descendants := collectPermissionDescendants(perm.ID)
		for _, did := range descendants {
			if did == body.ParentID {
				c.JSON(http.StatusBadRequest, gin.H{"message": "不能将子节点设为父节点（循环引用）"})
				return
			}
		}
	}

	updates := map[string]interface{}{
		"name": body.Name,
		"sort": body.Sort,
	}
	// path/icon 允许置空（前端传空字符串时同步覆盖）
	updates["path"] = body.Path
	updates["icon"] = body.Icon
	// parentID 仅在前端显式传值（非 0 或显式 0）时更新；这里 JSON 解析无指针，统一更新
	updates["parent_id"] = body.ParentID
	if !perm.IsBuiltin && body.Code != "" {
		updates["code"] = body.Code
	}
	if !perm.IsBuiltin && body.Type != "" {
		updates["type"] = body.Type
	}

	if err := db.Model(&perm).Updates(updates).Error; err != nil {
		recordAudit(c, "permission", "update", perm.Name, false, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	recordAudit(c, "permission", "update", perm.Name, true, fmt.Sprintf("更新权限: %s (code=%s)", body.Name, perm.Code))
	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// collectPermissionDescendants 递归收集指定权限节点的所有后代 ID（不含自身）
func collectPermissionDescendants(id uint) []uint {
	var ids []uint
	var children []Permission
	db.Where("parent_id = ?", id).Find(&children)
	for _, c := range children {
		ids = append(ids, c.ID)
		ids = append(ids, collectPermissionDescendants(c.ID)...)
	}
	return ids
}

// permissionDeleteHandler DELETE /ovpn/permission/:id 删除权限节点
// - 内置权限（IsBuiltin=true）不可删除（返回 400）
// - 级联删除所有子节点（递归删除 children）
// - 删除关联的 RolePermission 记录
// - 使用事务
func permissionDeleteHandler(c *gin.Context) {
	id, ok := parsePermissionIDParam(c)
	if !ok {
		return
	}
	var perm Permission
	if err := db.Where("id = ?", id).First(&perm).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": "权限不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	if perm.IsBuiltin {
		recordAudit(c, "permission", "delete", perm.Name, false, "内置权限不允许删除")
		c.JSON(http.StatusBadRequest, gin.H{"message": "内置权限不允许删除"})
		return
	}

	// 收集所有需要删除的节点 ID（自身 + 所有后代）
	toDelete := []uint{perm.ID}
	toDelete = append(toDelete, collectPermissionDescendants(perm.ID)...)

	// 检查后代中是否有内置权限，如有则拒绝删除
	for _, did := range toDelete {
		var d Permission
		if err := db.Where("id = ?", did).First(&d).Error; err == nil && d.IsBuiltin {
			recordAudit(c, "permission", "delete", perm.Name, false, fmt.Sprintf("子节点 %s (code=%s) 为内置权限，不允许删除", d.Name, d.Code))
			c.JSON(http.StatusBadRequest, gin.H{"message": fmt.Sprintf("子节点 %q 为内置权限，不允许删除", d.Name)})
			return
		}
	}

	// 事务：删除 RolePermission + Permission
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("permission_id IN ?", toDelete).Delete(&RolePermission{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id IN ?", toDelete).Delete(&Permission{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		recordAudit(c, "permission", "delete", perm.Name, false, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	recordAudit(c, "permission", "delete", perm.Name, true, fmt.Sprintf("删除权限: %s (code=%s), 含 %d 个子节点", perm.Name, perm.Code, len(toDelete)-1))
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// permissionSortHandler PUT /ovpn/permission/sort 批量更新排序
// 接收 JSON body: [{id, sort}, ...]
// 使用事务批量更新 sort 字段
func permissionSortHandler(c *gin.Context) {
	var body []struct {
		ID   uint `json:"id"`
		Sort int  `json:"sort"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	if len(body) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "排序数据不能为空"})
		return
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		for _, item := range body {
			if item.ID == 0 {
				continue
			}
			if item.Sort < 0 || item.Sort > 9999 {
				return fmt.Errorf("排序值需在 0-9999 之间")
			}
			if err := tx.Model(&Permission{}).Where("id = ?", item.ID).Update("sort", item.Sort).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		recordAudit(c, "permission", "sort", "", false, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	recordAudit(c, "permission", "sort", "", true, fmt.Sprintf("批量更新 %d 个权限排序", len(body)))
	c.JSON(http.StatusOK, gin.H{"message": "排序已更新"})
}

// validateRoleID 校验 role_id 有效性：非空且非 0 时必须存在且启用
// 供 POST /user 与 PATCH /user 复用，避免孤儿 user.role_id
func validateRoleID(d *gorm.DB, roleID *uint) error {
	if roleID == nil || *roleID == 0 {
		return nil
	}
	var role Role
	if err := d.First(&role, *roleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("角色不存在")
		}
		return err
	}
	if role.IsEnable != nil && !*role.IsEnable {
		return fmt.Errorf("角色已被禁用，无法分配给用户")
	}
	return nil
}

// validateRoleIDs 批量校验角色 ID 列表有效性：每个 ID 必须非零、存在且启用
func validateRoleIDs(d *gorm.DB, roleIDs []uint) error {
	for _, rid := range roleIDs {
		if rid == 0 {
			return fmt.Errorf("角色 ID 不能为 0")
		}
		var role Role
		if err := d.First(&role, rid).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("角色 %d 不存在", rid)
			}
			return err
		}
		if role.IsEnable != nil && !*role.IsEnable {
			return fmt.Errorf("角色「%s」已被禁用，无法分配给用户", role.Name)
		}
	}
	return nil
}

// parseRoleIDParam 解析 :id 路径参数为 uint，非法时返回 0 + false
func parseRoleIDParam(c *gin.Context) (uint, bool) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的角色 ID"})
		return 0, false
	}
	return uint(id), true
}

// roleListHandler GET /ovpn/role 角色列表（含权限 code 数组）
func roleListHandler(c *gin.Context) {
	var roles []Role
	if err := db.WithContext(context.Background()).Order("sort ASC, id ASC").Find(&roles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// 一次性拉所有 role_permission → permission.code 映射，避免 N+1
	type rolePerm struct {
		RoleID uint
		Code   string
	}
	var allPerms []rolePerm
	db.Table("role_permission").
		Select("role_permission.role_id AS role_id, permission.code AS code").
		Joins("INNER JOIN permission ON permission.id = role_permission.permission_id").
		Where("permission.code IS NOT NULL AND permission.code != ''").
		Scan(&allPerms)

	permsByRole := make(map[uint][]string)
	for _, p := range allPerms {
		permsByRole[p.RoleID] = append(permsByRole[p.RoleID], p.Code)
	}

	// 批量查询每个角色的用户数与用户组数，避免 N+1
	type roleCount struct {
		RoleID uint
		Count  int64
	}
	var userCounts []roleCount
	userCountQuery := db.Table("user_role").
		Select("user_role.role_id AS role_id, COUNT(*) as count").
		Joins("INNER JOIN user ON user.id = user_role.user_id")
	if adminUsername != "" {
		userCountQuery = userCountQuery.Where("user.username != ?", adminUsername)
	}
	userCountQuery.Group("user_role.role_id").Scan(&userCounts)
	userCountMap := make(map[uint]int64, len(userCounts))
	for _, uc := range userCounts {
		userCountMap[uc.RoleID] = uc.Count
	}

	var groupCounts []roleCount
	db.Model(&Group{}).
		Select("role_id, COUNT(*) as count").
		Where("role_id IS NOT NULL").
		Group("role_id").
		Scan(&groupCounts)
	groupCountMap := make(map[uint]int64, len(groupCounts))
	for _, gc := range groupCounts {
		groupCountMap[gc.RoleID] = gc.Count
	}

	type RoleWithPerms struct {
		Role
		Permissions []string `json:"permissions"`
		UserCount   int64    `json:"userCount"`
		GroupCount  int64    `json:"groupCount"`
	}

	result := make([]RoleWithPerms, 0, len(roles))
	for _, r := range roles {
		codes := permsByRole[r.ID]
		if codes == nil {
			codes = []string{}
		}
		result = append(result, RoleWithPerms{
			Role:        r,
			Permissions: codes,
			UserCount:   userCountMap[r.ID],
			GroupCount:  groupCountMap[r.ID],
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// roleDetailHandler GET /ovpn/role/:id 角色详情（含权限 code 数组）
func roleDetailHandler(c *gin.Context) {
	id, ok := parseRoleIDParam(c)
	if !ok {
		return
	}
	var role Role
	if err := db.Where("id = ?", id).First(&role).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "角色不存在"})
		return
	}

	var codes []string
	db.Table("role_permission").
		Select("permission.code").
		Joins("INNER JOIN permission ON permission.id = role_permission.permission_id").
		Where("role_permission.role_id = ? AND permission.code IS NOT NULL AND permission.code != ''", role.ID).
		Pluck("permission.code", &codes)
	if codes == nil {
		codes = []string{}
	}

	c.JSON(http.StatusOK, gin.H{
		"id":          role.ID,
		"name":        role.Name,
		"code":        role.Code,
		"description": role.Description,
		"isBuiltin":   role.IsBuiltin,
		"isEnable":    role.IsEnable,
		"sort":        role.Sort,
		"createdAt":   role.CreatedAt,
		"updatedAt":   role.UpdatedAt,
		"permissions": codes,
	})
}

// roleCreateHandler POST /ovpn/role 创建角色（is_builtin 强制为 false）
func roleCreateHandler(c *gin.Context) {
	var role Role
	if err := c.ShouldBind(&role); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	// code 规范化：强制小写，避免 Admin/ADMIN/admin 重复语义
	role.Code = strings.ToLower(strings.TrimSpace(role.Code))

	// 校验 code：非空、长度 1-64、匹配 ^[a-zA-Z][a-zA-Z0-9_]*$
	if len(role.Code) == 0 || len(role.Code) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "角色代码长度需在 1-64 之间"})
		return
	}
	if !roleCodeRegex.MatchString(role.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "角色代码只能以字母开头，包含字母数字下划线"})
		return
	}
	// 校验 name：非空、长度 1-64
	if len(role.Name) == 0 || len(role.Name) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "角色名称长度需在 1-64 之间"})
		return
	}
	// 校验 description 长度 ≤ 255
	if len(role.Description) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "角色描述长度不能超过 255"})
		return
	}
	// 校验 sort 范围
	if role.Sort < 0 || role.Sort > 9999 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "排序值需在 0-9999 之间"})
		return
	}

	// 检查 code 唯一
	var exists int64
	db.Model(&Role{}).Where("code = ?", role.Code).Count(&exists)
	if exists > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "角色编码已存在"})
		return
	}

	role.IsBuiltin = false
	// IsEnable 为 nil 时由 DB default:true 兜底；前端显式传 false 时保留 false
	if err := db.Create(&role).Error; err != nil {
		recordAudit(c, "role", "create", role.Name, false, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	recordAudit(c, "role", "create", role.Name, true, fmt.Sprintf("创建角色: %s (code=%s)", role.Name, role.Code))
	c.JSON(http.StatusOK, gin.H{"message": "创建成功", "id": role.ID})
}

// roleUpdateHandler PATCH /ovpn/role/:id 更新角色
// - 内置角色允许改 name/description/is_enable/sort 但不允许改 code
// - 内置角色不允许禁用（is_enable=false）
// - 非内置角色允许改所有字段
// - 非内置角色改 code 时校验唯一性
// - 使用 struct Updates 仅更新非零字段，避免 map Updates 清零未传字段
func roleUpdateHandler(c *gin.Context) {
	id, ok := parseRoleIDParam(c)
	if !ok {
		return
	}
	var role Role
	if err := db.Where("id = ?", id).First(&role).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "角色不存在"})
		return
	}

	var input Role
	if err := c.ShouldBind(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	// 内置角色保护：禁止修改 code
	if role.IsBuiltin && input.Code != "" && input.Code != role.Code {
		recordAudit(c, "role", "update", role.Name, false, "内置角色代码不允许修改")
		c.JSON(http.StatusBadRequest, gin.H{"message": "内置角色代码不允许修改"})
		return
	}
	// 内置角色保护：禁止禁用（input.IsEnable 为 *bool，nil 表示未传，false 表示显式禁用）
	if role.IsBuiltin && input.IsEnable != nil && !*input.IsEnable {
		recordAudit(c, "role", "update", role.Name, false, "内置角色不允许禁用")
		c.JSON(http.StatusBadRequest, gin.H{"message": "内置角色不允许禁用"})
		return
	}
	// 校验 name 非空
	if input.Name == "" {
		recordAudit(c, "role", "update", role.Name, false, "角色名称不能为空")
		c.JSON(http.StatusBadRequest, gin.H{"message": "角色名称不能为空"})
		return
	}
	// 校验 name 长度
	if len(input.Name) > 64 {
		recordAudit(c, "role", "update", role.Name, false, "角色名称长度不能超过 64")
		c.JSON(http.StatusBadRequest, gin.H{"message": "角色名称长度不能超过 64"})
		return
	}
	// 校验 description 长度
	if len(input.Description) > 255 {
		recordAudit(c, "role", "update", role.Name, false, "角色描述长度不能超过 255")
		c.JSON(http.StatusBadRequest, gin.H{"message": "角色描述长度不能超过 255"})
		return
	}
	// 校验 sort 范围
	if input.Sort < 0 || input.Sort > 9999 {
		recordAudit(c, "role", "update", role.Name, false, "排序值需在 0-9999 之间")
		c.JSON(http.StatusBadRequest, gin.H{"message": "排序值需在 0-9999 之间"})
		return
	}

	// 非内置角色改 code 时校验唯一性与格式
	newCode := strings.ToLower(strings.TrimSpace(input.Code))
	if !role.IsBuiltin && newCode != "" && newCode != role.Code {
		if !roleCodeRegex.MatchString(newCode) {
			recordAudit(c, "role", "update", role.Name, false, "角色代码格式不合法")
			c.JSON(http.StatusBadRequest, gin.H{"message": "角色代码只能以字母开头，包含字母数字下划线"})
			return
		}
		var exists int64
		db.Model(&Role{}).Where("code = ? AND id != ?", newCode, role.ID).Count(&exists)
		if exists > 0 {
			recordAudit(c, "role", "update", role.Name, false, "角色代码已存在")
			c.JSON(http.StatusBadRequest, gin.H{"message": "角色代码已存在"})
			return
		}
	}

	// 使用 map Updates 灵活处理 *bool；先调用 bluemonday 净化 name/description
	// （map Updates 不触发 BeforeSave 钩子，需在此显式净化）
	policy := bluemonday.UGCPolicy()
	safeName := policy.Sanitize(input.Name)
	safeDesc := policy.Sanitize(input.Description)
	updates := map[string]interface{}{
		"name":        safeName,
		"description": safeDesc,
		"sort":        input.Sort,
	}
	// IsEnable 仅在前端显式传值时更新（*bool 非 nil）
	if input.IsEnable != nil {
		updates["is_enable"] = *input.IsEnable
	}
	// 非内置角色允许改 code
	if !role.IsBuiltin && newCode != "" {
		updates["code"] = newCode
	}
	// 内置角色强制保留 is_builtin=true
	if role.IsBuiltin {
		updates["is_builtin"] = true
	}

	if err := db.Model(&role).Updates(updates).Error; err != nil {
		recordAudit(c, "role", "update", role.Name, false, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	recordAudit(c, "role", "update", role.Name, true, fmt.Sprintf("更新角色: %s (code=%s)", role.Name, role.Code))
	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// roleDeleteHandler DELETE /ovpn/role/:id
// - 内置角色不允许删除
// - 角色下存在用户不允许删除（用户数检查与删除在同一事务中，避免 TOCTOU 竞态）
// - 删除关联权限与角色记录包在事务中
func roleDeleteHandler(c *gin.Context) {
	id := c.Param("id")
	var role Role
	if err := db.Where("id = ?", id).First(&role).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "角色不存在"})
		return
	}

	if role.IsBuiltin {
		recordAudit(c, "role", "delete", role.Name, false, "内置角色不允许删除")
		c.JSON(http.StatusBadRequest, gin.H{"message": "内置角色不允许删除"})
		return
	}

	// 事务：用户数/用户组数检查 + 删除关联权限 + 删除角色
	// 把 count 检查与 delete 放在同一事务，避免检查后到删除前有新用户绑定该角色
	// 注意：SQLite 默认串行化事务可防止 TOCTOU；MySQL/PostgreSQL 下 Count 不加写锁，
	// 极端并发时序下仍可能产生孤儿 role_id，如需严格防护可改用 SELECT ... FOR UPDATE。
	err := db.Transaction(func(tx *gorm.DB) error {
		// 在事务内重新检查用户关联
		var userCount int64
		if err := tx.Table("user_role").Where("role_id = ?", role.ID).Count(&userCount).Error; err != nil {
			return err
		}
		if userCount > 0 {
			return fmt.Errorf("角色下存在用户或用户组，不允许删除")
		}
		// 检查用户组关联
		var groupCount int64
		if err := tx.Model(&Group{}).Where("role_id = ?", role.ID).Count(&groupCount).Error; err != nil {
			return err
		}
		if groupCount > 0 {
			return fmt.Errorf("角色下存在用户或用户组，不允许删除")
		}
		if err := tx.Where("role_id = ?", role.ID).Delete(&RolePermission{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&role).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		// 区分"存在用户/用户组"和其他错误
		if err.Error() == "角色下存在用户或用户组，不允许删除" {
			recordAudit(c, "role", "delete", role.Name, false, err.Error())
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		recordAudit(c, "role", "delete", role.Name, false, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	recordAudit(c, "role", "delete", role.Name, true, "删除角色: "+role.Name)
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// roleAssignPermissionsHandler PUT /ovpn/role/:id/permissions
// 全量替换 role_permission
// - 返回未识别的权限 code 列表
func roleAssignPermissionsHandler(c *gin.Context) {
	id := c.Param("id")
	var role Role
	if err := db.Where("id = ?", id).First(&role).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "角色不存在"})
		return
	}

	var body struct {
		Permissions []string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	// 查询权限 ID，同时记录未识别的 code
	// 去重前端传入的权限 code，避免重复 code 导致 Pluck 返回行数与输入不一致而误判
	seen := make(map[string]bool, len(body.Permissions))
	dedupedPerms := make([]string, 0, len(body.Permissions))
	for _, code := range body.Permissions {
		if !seen[code] {
			seen[code] = true
			dedupedPerms = append(dedupedPerms, code)
		}
	}

	var permIDs []uint
	if len(dedupedPerms) > 0 {
		db.Model(&Permission{}).Where("code IN ?", dedupedPerms).Pluck("id", &permIDs)
	}
	// 校验权限 code 是否全部存在（基于去重后的列表比较）
	if len(permIDs) != len(dedupedPerms) {
		// 找出未识别的 code
		var foundCodes []string
		db.Model(&Permission{}).Where("code IN ?", dedupedPerms).Pluck("code", &foundCodes)
		foundSet := make(map[string]bool, len(foundCodes))
		for _, c := range foundCodes {
			foundSet[c] = true
		}
		var unknown []string
		for _, c := range dedupedPerms {
			if !foundSet[c] {
				unknown = append(unknown, c)
			}
		}
		recordAudit(c, "role", "assign_permissions", role.Name, false, "权限代码不存在: "+strings.Join(unknown, ", "))
		c.JSON(http.StatusBadRequest, gin.H{"message": "权限代码不存在: [" + strings.Join(unknown, ", ") + "]"})
		return
	}

	// 事务：先删除旧关联，再写入新关联
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", role.ID).Delete(&RolePermission{}).Error; err != nil {
			return err
		}
		for _, pid := range permIDs {
			if err := tx.Create(&RolePermission{RoleID: role.ID, PermissionID: pid}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		recordAudit(c, "role", "assign_permissions", role.Name, false, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	recordAudit(c, "role", "assign_permissions", role.Name, true, "分配角色权限: "+role.Name)
	c.JSON(http.StatusOK, gin.H{"message": "权限已更新"})
}

// roleUserInfo 角色分配用户对话框中展示的单个用户信息
type roleUserInfo struct {
	ID        uint     `json:"id"`
	Username  string   `json:"username"`
	Name      string   `json:"name"`
	Gid       uint     `json:"gid"`
	GroupName string   `json:"groupName"`
	RoleIDs   []uint   `json:"roleIds"`
	RoleNames []string `json:"roleNames"`
}

// roleUsersHandler GET /ovpn/role/:id/users
// 返回所有非 admin 用户 + 当前角色已绑定用户 ID 列表
func roleUsersHandler(c *gin.Context) {
	id, ok := parseRoleIDParam(c)
	if !ok {
		return
	}
	var role Role
	if err := db.Where("id = ?", id).First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": "角色不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// 查询所有非 admin 用户（admin 用户绕过权限检查，不需要角色绑定）
	query := db.Model(&User{}).Select("id, username, name, gid")
	if adminUsername != "" {
		query = query.Where("username != ?", adminUsername)
	}
	var users []User
	if err := query.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// 批量查组名（按 gid 查 Group.Name）
	gidSet := make(map[uint]bool)
	for _, u := range users {
		if u.Gid != 0 {
			gidSet[u.Gid] = true
		}
	}
	groupNameMap := make(map[uint]string)
	if len(gidSet) > 0 {
		gids := make([]uint, 0, len(gidSet))
		for gid := range gidSet {
			gids = append(gids, gid)
		}
		var groups []Group
		if err := db.Where("id IN ?", gids).Find(&groups).Error; err != nil {
			logger.Error(context.Background(), "roleUsersHandler 批量查询组名失败: %s", err.Error())
		} else {
			for _, g := range groups {
				groupNameMap[g.ID] = g.Name
			}
		}
	}

	// 批量查 user_role 关联：一次性拉所有 (user_id, role_id) 对，避免 N+1
	type userRoleRow struct {
		UserID uint
		RoleID uint
	}
	var userRoleRows []userRoleRow
	if err := db.Table("user_role").
		Select("user_id, role_id").
		Scan(&userRoleRows).Error; err != nil {
		logger.Error(context.Background(), "roleUsersHandler 批量查询 user_role 失败: %s", err.Error())
	}

	// 收集所有出现过的 role_id，批量查角色名
	roleIDSet := make(map[uint]bool)
	userRoleMap := make(map[uint][]uint) // user_id -> []role_id
	for _, r := range userRoleRows {
		userRoleMap[r.UserID] = append(userRoleMap[r.UserID], r.RoleID)
		roleIDSet[r.RoleID] = true
	}
	roleNameMap := make(map[uint]string)
	if len(roleIDSet) > 0 {
		roleIDs := make([]uint, 0, len(roleIDSet))
		for rid := range roleIDSet {
			roleIDs = append(roleIDs, rid)
		}
		var roles []Role
		if err := db.Where("id IN ?", roleIDs).Find(&roles).Error; err != nil {
			logger.Error(context.Background(), "roleUsersHandler 批量查询角色名失败: %s", err.Error())
		} else {
			for _, r := range roles {
				roleNameMap[r.ID] = r.Name
			}
		}
	}

	// 拼装返回结果
	allUsers := make([]roleUserInfo, 0, len(users))
	assignedUserIDs := make([]uint, 0)
	for _, u := range users {
		rids := userRoleMap[u.ID]
		if rids == nil {
			rids = []uint{}
		}
		rnames := make([]string, 0, len(rids))
		for _, rid := range rids {
			rnames = append(rnames, roleNameMap[rid])
			if rid == role.ID {
				assignedUserIDs = append(assignedUserIDs, u.ID)
			}
		}
		info := roleUserInfo{
			ID:        u.ID,
			Username:  u.Username,
			Name:      u.Name,
			Gid:       u.Gid,
			GroupName: groupNameMap[u.Gid],
			RoleIDs:   rids,
			RoleNames: rnames,
		}
		allUsers = append(allUsers, info)
	}

	c.JSON(http.StatusOK, gin.H{
		"allUsers":        allUsers,
		"assignedUserIds": assignedUserIDs,
	})
}

// roleAssignUsersHandler PUT /ovpn/role/:id/users
// 全量替换该角色下的用户：把 userIds 中的用户绑定到该角色（user_role 表 INSERT），
// 把原来在该角色但不在 userIds 中的用户解绑（user_role 表 DELETE）
// 排除 admin 用户（admin 绕过权限检查，不参与角色绑定）
func roleAssignUsersHandler(c *gin.Context) {
	id, ok := parseRoleIDParam(c)
	if !ok {
		return
	}
	var role Role
	if err := db.Where("id = ?", id).First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": "角色不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// 已禁用角色拒绝分配（避免"分配成功但用户无法登录"的困惑）
	if role.IsEnable != nil && !*role.IsEnable {
		c.JSON(http.StatusBadRequest, gin.H{"message": "角色已被禁用，无法分配"})
		return
	}

	var body struct {
		UserIDs []uint `json:"userIds"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	// 去重前端传入的 userIds
	seen := make(map[uint]bool, len(body.UserIDs))
	dedupedIDs := make([]uint, 0, len(body.UserIDs))
	for _, uid := range body.UserIDs {
		if uid != 0 && !seen[uid] {
			seen[uid] = true
			dedupedIDs = append(dedupedIDs, uid)
		}
	}

	// 事务：操作 user_role 表（DELETE 旧绑定 + INSERT 新绑定）
	// 排除 admin 用户（admin 绕过权限检查，不参与角色绑定）
	err := db.Transaction(func(tx *gorm.DB) error {
		// 事务内重新检查角色是否存在，避免 TOCTOU（角色在事务外查询后被并发删除）
		var txRole Role
		if err := tx.Where("id = ?", role.ID).First(&txRole).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("角色不存在")
			}
			return err
		}

		// 校验 userIds 中实际存在的用户，记录不存在的 ID（spec 要求"用户不存在跳过并记日志"）
		validIDs := make([]uint, 0, len(dedupedIDs))
		if len(dedupedIDs) > 0 {
			var existingIDs []uint
			existQuery := tx.Model(&User{}).Where("id IN ?", dedupedIDs)
			if adminUsername != "" {
				existQuery = existQuery.Where("username != ?", adminUsername)
			}
			if err := existQuery.Pluck("id", &existingIDs).Error; err != nil {
				return err
			}
			existingSet := make(map[uint]bool, len(existingIDs))
			for _, eid := range existingIDs {
				existingSet[eid] = true
			}
			for _, uid := range dedupedIDs {
				if existingSet[uid] {
					validIDs = append(validIDs, uid)
				} else {
					logger.Error(context.Background(), "roleAssignUsersHandler: 用户 ID %d 不存在或为 admin 用户，已跳过", uid)
				}
			}
		}

		// 1. 删除该角色下不在 userIds 中的用户绑定（排除 admin 用户）
		//    通过子查询排除 admin 用户，避免误删 admin 的（理论上的）绑定
		delQuery := tx.Table("user_role").Where("role_id = ?", role.ID)
		if adminUsername != "" {
			delQuery = delQuery.Where("user_id NOT IN (?)",
				tx.Model(&User{}).Select("id").Where("username = ?", adminUsername))
		}
		if len(validIDs) > 0 {
			delQuery = delQuery.Where("user_id NOT IN ?", validIDs)
		}
		if err := delQuery.Delete(&UserRole{}).Error; err != nil {
			return err
		}

		// 2. 为 userIds 中的用户绑定该角色（INSERT OR IGNORE 避免重复主键冲突）
		for _, uid := range validIDs {
			if err := tx.Where("user_id = ? AND role_id = ?", uid, role.ID).
				FirstOrCreate(&UserRole{}, UserRole{UserID: uid, RoleID: role.ID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		recordAudit(c, "role", "assign_users", role.Name, false, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	recordAudit(c, "role", "assign_users", role.Name, true, "分配角色用户: "+role.Name)
	c.JSON(http.StatusOK, gin.H{"message": "用户已分配"})
}

// roleGroupInfo 角色分配用户组对话框中展示的单个组信息
type roleGroupInfo struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	ParentID *uint  `json:"parentId"`
	RoleID   *uint  `json:"roleId"`
	RoleName string `json:"roleName"`
}

// roleGroupsHandler GET /ovpn/role/:id/groups
// 返回所有组 + 当前角色已绑定组 ID 列表
func roleGroupsHandler(c *gin.Context) {
	id, ok := parseRoleIDParam(c)
	if !ok {
		return
	}
	var role Role
	if err := db.Where("id = ?", id).First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": "角色不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	var groups []Group
	if err := db.Select("id, name, parent_id, role_id").Find(&groups).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// 批量查角色名
	roleIDSet := make(map[uint]bool)
	for _, g := range groups {
		if g.RoleID != nil && *g.RoleID != 0 {
			roleIDSet[*g.RoleID] = true
		}
	}
	roleNameMap := make(map[uint]string)
	if len(roleIDSet) > 0 {
		roleIDs := make([]uint, 0, len(roleIDSet))
		for rid := range roleIDSet {
			roleIDs = append(roleIDs, rid)
		}
		var roles []Role
		if err := db.Where("id IN ?", roleIDs).Find(&roles).Error; err != nil {
			logger.Error(context.Background(), "roleGroupsHandler 批量查询角色名失败: %s", err.Error())
		} else {
			for _, r := range roles {
				roleNameMap[r.ID] = r.Name
			}
		}
	}

	// 拼装返回结果
	allGroups := make([]roleGroupInfo, 0, len(groups))
	assignedGroupIDs := make([]uint, 0)
	for _, g := range groups {
		info := roleGroupInfo{
			ID:       g.ID,
			Name:     g.Name,
			ParentID: g.ParentID,
			RoleID:   g.RoleID,
		}
		if g.RoleID != nil && *g.RoleID != 0 {
			info.RoleName = roleNameMap[*g.RoleID]
			if *g.RoleID == role.ID {
				assignedGroupIDs = append(assignedGroupIDs, g.ID)
			}
		}
		allGroups = append(allGroups, info)
	}

	c.JSON(http.StatusOK, gin.H{
		"allGroups":        allGroups,
		"assignedGroupIds": assignedGroupIDs,
	})
}

// roleAssignGroupsHandler PUT /ovpn/role/:id/groups
// 全量替换该角色下的用户组：把 groupIds 中的组 role_id 设为该角色，
// 把原来在该角色但不在 groupIds 中的组 role_id 设为 NULL
// 内置 administrator 角色拒绝（400，与 assign_users 一致，避免新建用户继承超管角色）
// Default 组（ID=1）拒绝修改 role_id（400）
func roleAssignGroupsHandler(c *gin.Context) {
	id, ok := parseRoleIDParam(c)
	if !ok {
		return
	}
	var role Role
	if err := db.Where("id = ?", id).First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": "角色不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// 已禁用角色拒绝分配（避免"分配成功但用户无法登录"的困惑）
	if role.IsEnable != nil && !*role.IsEnable {
		c.JSON(http.StatusBadRequest, gin.H{"message": "角色已被禁用，无法分配"})
		return
	}

	var body struct {
		GroupIDs []uint `json:"groupIds"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	// 检查 groupIds 是否包含 Default 组（ID=1），若包含返回 400
	for _, gid := range body.GroupIDs {
		if gid == 1 {
			recordAudit(c, "role", "assign_groups", role.Name, false, "默认组不支持绑定角色")
			c.JSON(http.StatusBadRequest, gin.H{"message": "默认组不支持绑定角色"})
			return
		}
	}

	// 去重前端传入的 groupIds
	seen := make(map[uint]bool, len(body.GroupIDs))
	dedupedIDs := make([]uint, 0, len(body.GroupIDs))
	for _, gid := range body.GroupIDs {
		if gid != 0 && !seen[gid] {
			seen[gid] = true
			dedupedIDs = append(dedupedIDs, gid)
		}
	}

	// 事务：先移除不在 groupIds 中的组，再分配 groupIds 中的组
	err := db.Transaction(func(tx *gorm.DB) error {
		// 事务内重新检查角色是否存在，避免 TOCTOU
		var txRole Role
		if err := tx.Where("id = ?", role.ID).First(&txRole).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("角色不存在")
			}
			return err
		}

		// 校验 groupIds 中实际存在的组，记录不存在的 ID（与用户分配逻辑对齐）
		if len(dedupedIDs) > 0 {
			var existingIDs []uint
			if err := tx.Model(&Group{}).Where("id IN ? AND id != 1", dedupedIDs).Pluck("id", &existingIDs).Error; err != nil {
				return err
			}
			existingSet := make(map[uint]bool, len(existingIDs))
			for _, eid := range existingIDs {
				existingSet[eid] = true
			}
			for _, gid := range dedupedIDs {
				if !existingSet[gid] {
					logger.Error(context.Background(), "roleAssignGroupsHandler: 用户组 ID %d 不存在或为 Default 组，已跳过", gid)
				}
			}
		}

		// 1. 把该角色下不在 groupIds 中的组 role_id 设为 NULL（排除 Default 组 ID=1）
		if len(dedupedIDs) > 0 {
			if err := tx.Model(&Group{}).
				Where("role_id = ? AND id NOT IN ? AND id != 1", role.ID, dedupedIDs).
				Update("role_id", nil).Error; err != nil {
				return err
			}
		} else {
			// 全部移除：把该角色下所有组 role_id 设为 NULL（排除 Default 组）
			if err := tx.Model(&Group{}).
				Where("role_id = ? AND id != 1", role.ID).
				Update("role_id", nil).Error; err != nil {
				return err
			}
		}
		// 2. 把 groupIds 中的组 role_id 设为该角色
		if len(dedupedIDs) > 0 {
			if err := tx.Model(&Group{}).
				Where("id IN ? AND id != 1", dedupedIDs).
				Update("role_id", role.ID).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		recordAudit(c, "role", "assign_groups", role.Name, false, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	recordAudit(c, "role", "assign_groups", role.Name, true, "分配角色用户组: "+role.Name)
	c.JSON(http.StatusOK, gin.H{"message": "用户组已分配"})
}
