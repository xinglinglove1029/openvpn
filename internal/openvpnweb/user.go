package openvpnweb

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path"
	"reflect"
	"strings"
	"time"

	"github.com/gavintan/gopkg/aes"
	"github.com/microcosm-cc/bluemonday"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

// UserRole 用户-角色多对多关联表
type UserRole struct {
	UserID    uint      `gorm:"column:user_id;primaryKey" json:"userId" form:"userId"`
	RoleID    uint      `gorm:"column:role_id;primaryKey" json:"roleId" form:"roleId"`
	CreatedAt time.Time `gorm:"column:created_at" json:"createdAt"`
}

func (UserRole) TableName() string { return "user_role" }

type User struct {
	ID uint `gorm:"primarykey" json:"id" form:"id" uri:"id"`
	// size:191：MySQL 不允许对 TEXT 列建唯一索引（Error 1170），
	// 需限定为 varchar(191)（utf8mb4 下 191 字符 = 764 字节，唯一索引上限）。
	Username     string     `gorm:"uniqueIndex;column:username;size:191" json:"username" form:"username"`
	Password     string     `form:"password" json:"password"`
	IsEnable     *bool      `gorm:"default:true" form:"isEnable" json:"isEnable"`
	Name         string     `json:"name" form:"name"`
	Email        string     `json:"email" form:"email"`
	Gid          uint       `gorm:"default:1" json:"gid" form:"gid"`
	ExpireDate   string     `gorm:"default:NULL" json:"expireDate" form:"expireDate"`
	IpAddr       string     `gorm:"uniqueIndex;default:NULL" json:"ipAddr" form:"ipAddr"`
	IpRegion     string     `gorm:"-" json:"ipRegion"` // 非数据库字段，运行时计算
	OvpnConfig   string     `json:"ovpnConfig" form:"ovpnConfig"`
	MfaSecret    string     `json:"mfaSecret" form:"mfaSecret"`
	MfaEnabled   bool       `gorm:"default:false" json:"mfaEnabled" form:"mfaEnabled"`
	IsFirstLogin *bool      `gorm:"default:true" form:"isFirstLogin" json:"isFirstLogin"`
	RoleIDs      []uint     `gorm:"-" json:"roleIds" form:"roleIds"`
	LastLoginAt  *time.Time `json:"lastLoginAt,omitempty" form:"lastLoginAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt,omitempty" form:"createdAt,omitempty"`
	UpdatedAt    time.Time  `json:"updatedAt,omitempty" form:"updatedAt,omitempty"`
	// 非数据库字段：内置 admin 用户标记，运行时计算，前端用于隐藏删除按钮
	IsBuiltin bool `gorm:"-" json:"isBuiltin,omitempty"`
}

// ErrRoleDisabled 角色已禁用错误
var ErrRoleDisabled = fmt.Errorf("角色已禁用，请联系管理员")

// ErrRoleNotFound 角色不存在或已删除错误
var ErrRoleNotFound = errors.New("角色不存在或已删除")

// LoadPermissionCodes 加载用户权限 code 列表
// 权限合并逻辑（全部取并集去重）：
//  1. 用户直接绑定的角色权限（user_role 表）
//  2. 用户所属组（Gid）绑定的角色权限（group_role 表）
//  3. 若以上均为空，回退默认角色权限
//
// admin 用户已纳入 user 表并绑定 administrator 角色，走标准 RBAC 加载
func (u *User) LoadPermissionCodes(d *gorm.DB) ([]string, error) {
	codeSet := make(map[string]bool)

	// 1. 用户直接绑定的角色权限
	var userRoleIDs []uint
	if err := d.Table("user_role").Where("user_id = ?", u.ID).Pluck("role_id", &userRoleIDs).Error; err != nil {
		return nil, err
	}
	for _, rid := range userRoleIDs {
		codes, err := LoadRolePermissionCodes(d, rid)
		if err != nil {
			// 禁用/不存在的角色跳过
			continue
		}
		for _, c := range codes {
			codeSet[c] = true
		}
	}

	// 2. 用户所属组绑定的角色权限（并集）
	if u.Gid > 0 {
		var groupRoleIDs []uint
		if err := d.Table("group_role").Where("group_id = ?", u.Gid).Pluck("role_id", &groupRoleIDs).Error; err != nil {
			logger.Error(context.Background(), "LoadPermissionCodes 查询 group_role 失败: %s", err.Error())
		} else {
			for _, rid := range groupRoleIDs {
				codes, err := LoadRolePermissionCodes(d, rid)
				if err != nil {
					continue
				}
				for _, c := range codes {
					codeSet[c] = true
				}
			}
		}
	}

	// 3. 若用户和组均无角色绑定，回退默认角色
	if len(userRoleIDs) == 0 && len(codeSet) == 0 {
		defaultRoleID := GetDefaultRoleID(d)
		if defaultRoleID == 0 {
			return []string{}, nil
		}
		codes, err := LoadRolePermissionCodes(d, defaultRoleID)
		if err != nil {
			return []string{}, nil
		}
		for _, c := range codes {
			codeSet[c] = true
		}
	}

	result := make([]string, 0, len(codeSet))
	for code := range codeSet {
		result = append(result, code)
	}
	return result, nil
}

// LoadRoleIDsAndNames 加载用户绑定的所有角色 ID 与名称（用于登录响应、/me 接口等）
// admin 用户已纳入 user 表并绑定 administrator 角色，走标准 user_role 加载
// 出错或无绑定时返回两个等长空切片，确保调用方解构后长度始终一致
func (u *User) LoadRoleIDsAndNames(d *gorm.DB) ([]uint, []string) {
	var roleIDs []uint
	if err := d.Table("user_role").Where("user_id = ?", u.ID).Pluck("role_id", &roleIDs).Error; err != nil {
		return []uint{}, []string{}
	}
	if len(roleIDs) == 0 {
		return []uint{}, []string{}
	}
	var roles []Role
	if err := d.Where("id IN ?", roleIDs).Find(&roles).Error; err != nil {
		// 查询角色失败时返回等长空切片，避免 roleIDs/roleNames 长度不一致
		return []uint{}, []string{}
	}
	roleNameMap := make(map[uint]string, len(roles))
	for _, r := range roles {
		roleNameMap[r.ID] = r.Name
	}
	roleNames := make([]string, 0, len(roleIDs))
	for _, rid := range roleIDs {
		roleNames = append(roleNames, roleNameMap[rid])
	}
	return roleIDs, roleNames
}

func (u *User) IsMFAEnabled() bool {
	return u.MfaEnabled || u.MfaSecret != ""
}

func (u *User) BeforeSave(tx *gorm.DB) (err error) {
	p := bluemonday.UGCPolicy()

	val := reflect.ValueOf(u).Elem()
	for i := 0; i < val.NumField(); i++ {
		fieldVal := val.Field(i)
		if fieldVal.Kind() == reflect.String && fieldVal.CanSet() {
			rawStr := val.Field(i).String()
			val.Field(i).SetString(p.Sanitize(rawStr))
		}
	}

	if u.Password != "" {
		ep, _ := aes.AesEncrypt(u.Password, secretKey)
		tx.Statement.SetColumn("Password", ep)
	}

	return nil
}

func (u *User) AfterFind(tx *gorm.DB) error {
	// Password 为空时（如 Select("id") 只查 id 字段）跳过解密，避免 ciphertext too short 错误
	if u.Password == "" {
		return nil
	}
	if dp, err := aes.AesDecrypt(u.Password, secretKey); err == nil {
		u.Password = dp
	}
	// 解密失败时保持原值，不阻断查询
	return nil
}

func (u *User) All() []User {
	var users []User

	result := db.WithContext(context.Background()).Find(&users)
	if result.Error != nil {
		logger.Error(context.Background(), result.Error.Error())
		return []User{}
	}

	// 解析 IP 归属地
	for i := range users {
		users[i].IpRegion = GetIPRegion(users[i].IpAddr)
	}

	return users
}

func (u *User) Get(id string) User {
	result := db.First(&u, id)
	if result.Error != nil {
		logger.Error(context.Background(), result.Error.Error())
		return User{}
	}

	return *u
}

func (u *User) Create() error {
	if u.Username == "" || u.Password == "" || strings.TrimSpace(u.Name) == "" {
		return fmt.Errorf("非法请求")
	}

	// admin 用户由 initBuiltinUsers 在启动时创建，此处不再手动拦截同名用户；
	// 若运行期尝试创建与 admin 同名的用户，由 user 表 username UNIQUE 约束拒绝
	if strings.TrimSpace(u.Email) == "" {
		return fmt.Errorf("邮箱为必填项")
	}

	// 使用事务确保用户与角色绑定数据一致性
	return db.Transaction(func(tx *gorm.DB) error {
		result := tx.Create(&u)
		if result.Error != nil {
			if strings.Contains(result.Error.Error(), "UNIQUE constraint failed") {
				return fmt.Errorf("用户名 \"%s\" 已存在", u.Username)
			}
			return result.Error
		}

		// 同步用户角色绑定
		if len(u.RoleIDs) > 0 {
			userRoles := make([]UserRole, 0, len(u.RoleIDs))
			for _, rid := range u.RoleIDs {
				userRoles = append(userRoles, UserRole{UserID: u.ID, RoleID: rid})
			}
			if err := tx.Create(&userRoles).Error; err != nil {
				return fmt.Errorf("绑定用户角色失败: %w", err)
			}
		}
		return nil
	})
}

func (u *User) Update() error {
	// 使用事务确保角色绑定更新原子性（先删后插，失败回滚）
	return db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&u).Updates(&u)
		if result.Error != nil {
			return result.Error
		}
		// RoleIDs 非 nil 时同步用户角色绑定（nil=不修改，空切片=清空）
		if u.RoleIDs != nil {
			if err := tx.Where("user_id = ?", u.ID).Delete(&UserRole{}).Error; err != nil {
				return fmt.Errorf("清理旧角色绑定失败: %w", err)
			}
			if len(u.RoleIDs) > 0 {
				userRoles := make([]UserRole, 0, len(u.RoleIDs))
				for _, rid := range u.RoleIDs {
					userRoles = append(userRoles, UserRole{UserID: u.ID, RoleID: rid})
				}
				if err := tx.Create(&userRoles).Error; err != nil {
					return fmt.Errorf("写入新角色绑定失败: %w", err)
				}
			}
		}
		return nil
	})
}

func (u *User) Delete(id string) error {
	// 使用事务确保清理关联和删除用户原子性
	return db.Transaction(func(tx *gorm.DB) error {
		// 先清理 user_role 关联
		if err := tx.Where("user_id = ?", id).Delete(&UserRole{}).Error; err != nil {
			return fmt.Errorf("清理用户角色绑定失败: %w", err)
		}
		result := tx.Unscoped().Delete(&User{}, id)
		return result.Error
	})
}

func (u *User) UpdatePassword() error {
	result := db.Model(&u).Updates(User{Password: u.Password})
	return result.Error
}

func (u *User) Login(clogin bool) error {
	user := u.Username
	pass := u.Password
	commonName := u.OvpnConfig

	if clogin {
		if viper.GetInt("system.base.max_duplicate_login") > 0 {
			data, err := os.ReadFile(path.Join(ovData, "openvpn-status.log"))
			if err != nil {
				logger.Error(context.Background(), err.Error())
			}

			loginCount := 0
			for _, v := range strings.Split(string(data), "\n") {
				cdSlice := strings.Split(v, "\t")

				if cdSlice[0] == "CLIENT_LIST" {
					if cdSlice[9] == user {
						loginCount++
					}
				}
			}

			if loginCount >= viper.GetInt("system.base.max_duplicate_login") {
				return fmt.Errorf("用户已禁用，当前客户端不允许登录")
			}
		}
	}

	if ldapAuth {
		l, err := InitLdap()
		if err != nil {
			return err
		}

		return l.Auth(clogin, user, pass, commonName)
	} else {
		result := db.First(&u, "username = ?", user)

		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return fmt.Errorf("用户不存在")
		}

		if !*u.IsEnable {
			return fmt.Errorf("账号已禁用")
		}

		if u.ExpireDate != "" {
			ed, _ := time.Parse("2006-01-02", u.ExpireDate)
			if ed.Before(time.Now()) {
				return fmt.Errorf("账号已过期")
			}
		}

		if clogin {
			if u.MfaSecret != "" && !strings.HasPrefix(pass, "SCRV1:") {
				return fmt.Errorf("未获取到 MFA 验证码")
			}

			var passcode string
			if strings.HasPrefix(pass, "SCRV1:") {
				parts := strings.Split(pass, ":")
				if len(parts) == 3 {
					p, err := base64.StdEncoding.DecodeString(parts[1])
					if err != nil {
						return fmt.Errorf("passwd 解码错误：%w", err)
					}

					pass = string(p)

					k, err := base64.StdEncoding.DecodeString(parts[2])
					if err != nil {
						return fmt.Errorf("key 解码错误：%w", err)
					}

					passcode = string(k)
				}
			}

			if u.MfaSecret != "" {
				vaild := ValidateMfa(passcode, u.MfaSecret)
				if !vaild {
					return fmt.Errorf("MFA 验证失败")
				}
			}
		}

		if subtle.ConstantTimeCompare([]byte(u.Password), []byte(pass)) != 1 {
			return fmt.Errorf("密码错误")
		}

		if clogin {
			if viper.GetBool("system.base.validate_client_config") {
				if commonName != strings.TrimSuffix(u.OvpnConfig, ".ovpn") {
					return fmt.Errorf("使用非法配置文件登录")
				}
			}

			var ovconfig sql.NullString
			// 跨方言：不使用 GROUP_CONCAT（MySQL 语法不同、PostgreSQL 无此函数），
			// 改为取出分组链上的全部 config，在 Go 侧按行替换 \n 并拼接
			var groupConfigs []struct {
				Config string
			}
			err := db.Raw(`
				WITH RECURSIVE group_up AS (
					SELECT
						id,
						parent_id,
						config,
						0 AS level
					FROM `+groupIdent(db)+`
					WHERE id = ?
		
					UNION ALL
		
					SELECT
						g.id,
						g.parent_id,
						g.config,
						gu.level + 1
					FROM `+groupIdent(db)+` g
					JOIN group_up gu ON g.id = gu.parent_id
				)
				SELECT config FROM group_up WHERE config IS NOT NULL
			`, u.Gid).Scan(&groupConfigs).Error
			if err == nil && len(groupConfigs) > 0 {
				parts := make([]string, 0, len(groupConfigs))
				for _, gc := range groupConfigs {
					parts = append(parts, strings.ReplaceAll(gc.Config, "\\n", "\n"))
				}
				ovconfig = sql.NullString{String: strings.Join(parts, "\n"), Valid: true}
			}

			options := ""
			if ovconfig.Valid {
				options = ovconfig.String
			}
			if err := writeClientConnectOptions(u.Username, commonName, u.IpAddr, options); err != nil {
				return fmt.Errorf("prepare client connection options: %w", err)
			}
		}

		db.Model(&u).Update("last_login_at", time.Now())

		return nil
	}
}

func (u User) Info() User {
	if u.Username != "" {
		db.First(&u, "username = ?", u.Username)
	} else {
		db.First(&u)
	}

	return u
}

func (u *User) GetGroups() []Group {
	var groups []Group

	db.Raw(`
		WITH RECURSIVE group_tree AS (
			SELECT g.*
			FROM `+groupIdent(db)+` g
			INNER JOIN `+userIdent(db)+` u ON u.gid = g.id
			WHERE u.username = ?

			UNION ALL

			SELECT g.*
			FROM `+groupIdent(db)+` g
			INNER JOIN group_tree gt ON g.id = gt.parent_id
		)
		SELECT DISTINCT id FROM group_tree
	`, u.Username).Scan(&groups)

	return groups
}

func (User) TableName() string {
	return "user"
}

// GetAccessibleUsernames 获取当前登录用户可见的所有用户名列表
// 基于分组数据权限：用户只能看到自己所在分组及其下级分组中的用户
// admin 用户返回空切片和 true（表示不做过滤）
func GetAccessibleUsernames(currentUsername string) ([]string, bool) {
	// admin 不做数据权限过滤
	if adminUsername != "" && currentUsername == adminUsername {
		return nil, true
	}

	currentUser := User{Username: currentUsername}.Info()
	if currentUser.ID == 0 || currentUser.Gid == 0 {
		return []string{currentUsername}, false
	}

	// 获取当前用户所在分组及其所有下级分组的 ID
	groupIDs := GetSubtreeIDs(currentUser.Gid)
	if len(groupIDs) == 0 {
		return []string{currentUsername}, false
	}

	// 查询这些分组下的所有用户名
	var usernames []string
	if err := db.WithContext(context.Background()).
		Model(&User{}).
		Where("gid IN ?", groupIDs).
		Pluck("username", &usernames).Error; err != nil {
		return []string{currentUsername}, false
	}

	if len(usernames) == 0 {
		return []string{currentUsername}, false
	}
	return usernames, false
}

func GetAccessibleUserIDs(currentUsername string) ([]uint, bool) {
	if adminUsername != "" && currentUsername == adminUsername {
		return nil, true
	}

	currentUser := User{Username: currentUsername}.Info()
	// A missing user must never widen a scoped query to user_id = 0. DNS audit
	// entries are normally created only for resolved users, but treating a
	// deleted or failed-to-load operator as an empty scope is fail-closed.
	if currentUser.ID == 0 {
		return []uint{}, false
	}
	if currentUser.Gid == 0 {
		return []uint{currentUser.ID}, false
	}

	groupIDs := GetSubtreeIDs(currentUser.Gid)
	if len(groupIDs) == 0 {
		return []uint{currentUser.ID}, false
	}

	var userIDs []uint
	if err := db.WithContext(context.Background()).
		Model(&User{}).
		Where("gid IN ?", groupIDs).
		Pluck("id", &userIDs).Error; err != nil {
		return []uint{currentUser.ID}, false
	}

	if len(userIDs) == 0 {
		return []uint{currentUser.ID}, false
	}
	return userIDs, false
}

func GetUserIDByUsername(username string) uint {
	if username == "" {
		return 0
	}
	var user User
	err := db.WithContext(context.Background()).
		Where("username = ?", username).
		Select("id").
		First(&user).Error
	if err != nil {
		logger.Error(context.Background(), "[GetUserIDByUsername] username=%q err=%v", username, err)
		return 0
	}
	return user.ID
}

func GetAccessibleClientConfigs(currentUsername string) ([]string, bool) {
	if adminUsername != "" && currentUsername == adminUsername {
		return nil, true
	}

	currentUser := User{Username: currentUsername}.Info()
	if currentUser.ID == 0 || currentUser.Gid == 0 {
		configs := make([]string, 0)
		if currentUser.OvpnConfig != "" {
			configs = append(configs, currentUser.OvpnConfig)
		}
		if currentUser.Username != "" {
			configs = append(configs, currentUser.Username)
		}
		return configs, false
	}

	groupIDs := GetSubtreeIDs(currentUser.Gid)
	if len(groupIDs) == 0 {
		configs := make([]string, 0)
		if currentUser.OvpnConfig != "" {
			configs = append(configs, currentUser.OvpnConfig)
		}
		if currentUser.Username != "" {
			configs = append(configs, currentUser.Username)
		}
		return configs, false
	}

	type userConfig struct {
		Username   string
		OvpnConfig string
	}
	var rows []userConfig
	if err := db.WithContext(context.Background()).
		Model(&User{}).
		Select("username, ovpn_config").
		Where("gid IN ?", groupIDs).
		Find(&rows).Error; err != nil {
		configs := make([]string, 0)
		if currentUser.OvpnConfig != "" {
			configs = append(configs, currentUser.OvpnConfig)
		}
		if currentUser.Username != "" {
			configs = append(configs, currentUser.Username)
		}
		return configs, false
	}

	configSet := make(map[string]bool)
	for _, r := range rows {
		if r.OvpnConfig != "" {
			configSet[r.OvpnConfig] = true
		}
		if r.Username != "" {
			configSet[r.Username] = true
		}
	}

	configs := make([]string, 0, len(configSet))
	for k := range configSet {
		configs = append(configs, k)
	}
	return configs, false
}

func CanAccessClientConfig(currentUsername, clientName string) bool {
	if adminUsername != "" && currentUsername == adminUsername {
		return true
	}
	configs, skip := GetAccessibleClientConfigs(currentUsername)
	if skip {
		return true
	}
	for _, name := range configs {
		if name == clientName {
			return true
		}
	}
	return false
}
