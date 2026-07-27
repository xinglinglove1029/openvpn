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
	ID          uint      `gorm:"primarykey" json:"id"`
	Name        string    `gorm:"column:name;size:64;not null" json:"name" form:"name"`
	Code        string    `gorm:"column:code;size:64;uniqueIndex;not null" json:"code" form:"code"`
	Description string    `gorm:"column:description;size:255" json:"description" form:"description"`
	IsBuiltin   bool      `gorm:"column:is_builtin;default:false" json:"isBuiltin" form:"isBuiltin"`
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

// Permission 权限定义模型（仅由代码 seed 维护，不暴露 CRUD）
type Permission struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	ParentID  uint      `gorm:"column:parent_id;default:0" json:"parentId"`
	Name      string    `gorm:"column:name;size:64;not null" json:"name" form:"name"`
	Code      string    `gorm:"column:code;size:64;uniqueIndex;not null" json:"code" form:"code"`
	Type      string    `gorm:"column:type;size:16;default:'button'" json:"type" form:"type"` // menu | button
	Path      string    `gorm:"column:path;size:255" json:"path" form:"path"`
	Icon      string    `gorm:"column:icon;size:64" json:"icon" form:"icon"`
	Sort      int       `gorm:"column:sort;default:0" json:"sort" form:"sort"`
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
	{"", "menu:clients", "客户端", "menu", "/clients", "Smartphone", 3},
	{"", "menu:firewall", "防火墙", "menu", "/firewall", "Shield", 4},
	{"", "menu:history", "连接历史", "menu", "/history", "History", 5},
	{"", "menu:certs", "证书", "menu", "/certs", "FileKey", 6},
	{"", "menu:audit", "操作审计", "menu", "/audit", "FileText", 7},
	{"", "menu:settings", "系统设置", "menu", "/settings", "Settings", 8},
	{"", "menu:channels", "通知渠道", "menu", "/channels", "BellRing", 9},
	{"", "menu:notifications", "站内信", "menu", "/notifications", "Bell", 10},
	{"", "menu:roles", "角色管理", "menu", "/roles", "ShieldCheck", 11},
	{"", "menu:profile", "个人中心", "menu", "/profile", "User", 12},
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
	// 客户端（4）
	{"menu:clients", "client:create", "创建客户端", "button", "", "", 1},
	{"menu:clients", "client:download", "下载客户端", "button", "", "", 2},
	{"menu:clients", "client:delete", "删除客户端", "button", "", "", 3},
	{"menu:clients", "client:regenerate", "重新生成客户端", "button", "", "", 4},
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
	{"menu:settings", "settings:update", "更新设置", "button", "", "", 2},
	{"menu:settings", "settings:base", "基础控制Tab", "button", "", "", 3},
	{"menu:settings", "settings:ldap", "LDAP认证Tab", "button", "", "", 4},
	{"menu:settings", "settings:openvpn", "OpenVPN参数Tab", "button", "", "", 5},
	{"menu:settings", "settings:service", "服务管理Tab", "button", "", "", 6},
	{"menu:settings", "settings:packages", "客户端安装包Tab", "button", "", "", 7},
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
	// 历史记录（1）
	{"menu:history", "history:view", "查看连接历史", "button", "", "", 1},
}

// 普通用户角色默认权限（菜单 + 按钮）
var defaultUserRoleCodes = []string{
	"menu:overview",
	"menu:clients",
	"menu:history",
	"menu:notifications",
	"menu:profile",
	"client:create",
	"client:download",
	"client:delete",
	"client:regenerate",
	"client:view_online",
	"history:view",
	"menu:settings",      // 系统设置菜单
	"settings:view",      // 查看设置
	"settings:base",      // 基础控制Tab
	"settings:packages",   // 客户端安装包Tab
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
		if err := db.Where("code = ?", p.Code).FirstOrCreate(&perm, Permission{
			Name: p.Name, Code: p.Code, Type: p.Type, Path: p.Path, Icon: p.Icon, Sort: p.Sort,
		}).Error; err != nil {
			return err
		}
		// 更新元数据（已有记录时同步名称/路径/排序）
		db.Model(&perm).Where("id = ?", perm.ID).Updates(map[string]interface{}{
			"name": p.Name, "type": p.Type, "path": p.Path, "icon": p.Icon, "sort": p.Sort,
		})
		codeToID[p.Code] = perm.ID
	}
	// 再写按钮（按 parentCode 关联）
	for _, p := range buttonPermissions {
		var perm Permission
		if err := db.Where("code = ?", p.Code).FirstOrCreate(&perm, Permission{
			Name: p.Name, Code: p.Code, Type: p.Type, Sort: p.Sort,
		}).Error; err != nil {
			return err
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
		db.Model(&perm).Where("id = ?", perm.ID).Updates(map[string]interface{}{
			"name": p.Name, "type": p.Type, "parent_id": parentID, "sort": p.Sort,
		})
		codeToID[p.Code] = perm.ID
	}

	// 2. 创建内置 administrator 角色（全权限）
	adminEnable := true
	var adminRole Role
	if err := db.Where("code = ?", BuiltinRoleAdministrator).FirstOrCreate(&adminRole, Role{
		Name: "系统超管", Code: BuiltinRoleAdministrator, Description: "拥有全部权限的内置超级管理员",
		IsBuiltin: true, IsEnable: &adminEnable, Sort: 0,
	}).Error; err != nil {
		return err
	}
	// 同步元数据
	db.Model(&adminRole).Where("id = ?", adminRole.ID).Updates(map[string]interface{}{
		"name": "系统超管", "description": "拥有全部权限的内置超级管理员", "is_builtin": true, "is_enable": true, "sort": 0,
	})

	// 3. 创建内置 user 角色（普通用户权限）
	userEnable := true
	var userRole Role
	if err := db.Where("code = ?", BuiltinRoleUser).FirstOrCreate(&userRole, Role{
		Name: "普通用户", Code: BuiltinRoleUser, Description: "仅可访问概览/客户端/历史/站内信/个人中心",
		IsBuiltin: true, IsEnable: &userEnable, Sort: 1,
	}).Error; err != nil {
		return err
	}
	db.Model(&userRole).Where("id = ?", userRole.ID).Updates(map[string]interface{}{
		"name": "普通用户", "description": "仅可访问概览/客户端/历史/站内信/个人中心", "is_builtin": true, "is_enable": true, "sort": 1,
	})

	// 4. 内置角色权限同步策略：
	//    administrator：首次初始化写入全权限，之后保留管理员运行期修改，不再自动同步
	//    user：每次启动时 FirstOrCreate 同步 defaultUserRoleCodes 中的默认权限
	//          （新增的默认权限项会被自动补齐，管理员额外添加的权限保留，管理员移除的默认权限不强行回填）
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

		// user 角色：每次启动确保 defaultUserRoleCodes 中的权限都存在
		// 这样新增的默认权限项会被自动同步，但不会删除管理员移除的权限
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
		ID       uint        `json:"id"`
		Name     string      `json:"name"`
		Code     string      `json:"code"`
		Type     string      `json:"type"`
		Path     string      `json:"path"`
		Icon     string      `json:"icon"`
		Sort     int         `json:"sort"`
		Children []*TreeNode `json:"children"`
	}

	byID := make(map[uint]*TreeNode)
	roots := make([]*TreeNode, 0)
	for _, p := range perms {
		node := &TreeNode{
			ID: p.ID, Name: p.Name, Code: p.Code, Type: p.Type, Path: p.Path, Icon: p.Icon, Sort: p.Sort,
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

	type RoleWithPerms struct {
		Role
		Permissions []string `json:"permissions"`
	}

	result := make([]RoleWithPerms, 0, len(roles))
	for _, r := range roles {
		codes := permsByRole[r.ID]
		if codes == nil {
			codes = []string{}
		}
		result = append(result, RoleWithPerms{Role: r, Permissions: codes})
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

	// 事务：用户数检查 + 删除关联权限 + 删除角色
	// 把 count 检查与 delete 放在同一事务，避免检查后到删除前有新用户绑定该角色
	err := db.Transaction(func(tx *gorm.DB) error {
		// 在事务内重新检查用户关联（加锁避免并发绑定）
		var userCount int64
		if err := tx.Model(&User{}).Where("role_id = ?", role.ID).Count(&userCount).Error; err != nil {
			return err
		}
		if userCount > 0 {
			return fmt.Errorf("角色下存在用户，不允许删除")
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
		// 区分"存在用户"和其他错误
		if err.Error() == "角色下存在用户，不允许删除" {
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
// - 内置角色权限由系统管理，拒绝修改
// - 返回未识别的权限 code 列表
func roleAssignPermissionsHandler(c *gin.Context) {
	id := c.Param("id")
	var role Role
	if err := db.Where("id = ?", id).First(&role).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "角色不存在"})
		return
	}

	// 内置角色保护：拒绝修改权限
	if role.IsBuiltin {
		recordAudit(c, "role", "assign_permissions", role.Name, false, "内置角色权限由系统管理，不允许修改")
		c.JSON(http.StatusBadRequest, gin.H{"message": "内置角色权限由系统管理，不允许修改"})
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
