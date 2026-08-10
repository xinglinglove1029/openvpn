package openvpnweb

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/viper"
	"google.golang.org/adk/agent"

	"openvpn-web/internal/openvpnweb/ai"
)

// AIToolService 实现 ai.ToolService 接口。
// 由 openvpnweb 包实现，注入到 ai 包的业务工具中，避免循环依赖。
// 所有方法都会校验 operator 的权限，确保 AI Agent 不会越权操作。
type AIToolService struct {
	ov *ovpn
}

// NewAIToolService 创建业务工具服务实例。ov 用于读取 OpenVPN 管理接口的在线客户端统计。
func NewAIToolService(ov *ovpn) *AIToolService {
	return &AIToolService{ov: ov}
}

// hasPermission 检查指定用户是否拥有某权限 code
// admin 用户自动放行；其他用户通过 LoadPermissionCodes 加载权限列表匹配
func (s *AIToolService) hasPermission(ctx agent.ToolContext, username, code string) bool {
	if username == "" {
		return false
	}
	// admin 用户直接放行
	if adminUsername != "" && username == adminUsername {
		return true
	}

	// 查询用户
	var u User
	if err := db.WithContext(ctx).Where("username = ?", username).First(&u).Error; err != nil {
		return false
	}

	// 加载权限码
	codes, err := u.LoadPermissionCodes(db)
	if err != nil {
		return false
	}
	for _, c := range codes {
		if c == "*" || c == code {
			return true
		}
	}
	return false
}

// CreateUser 创建用户（实现 ai.ToolService 接口）
func (s *AIToolService) CreateUser(ctx agent.ToolContext, operator string, req ai.CreateUserRequest) (ai.CreateUserResult, error) {
	// 权限校验
	if !s.hasPermission(ctx, operator, "user:create") {
		return ai.CreateUserResult{
			Success: false,
			Message: "当前用户无 user:create 权限，无法创建用户",
		}, fmt.Errorf("权限不足: 需要 user:create 权限")
	}

	// 参数校验
	if strings.TrimSpace(req.Username) == "" {
		return ai.CreateUserResult{Success: false, Message: "用户名不能为空"}, fmt.Errorf("用户名不能为空")
	}
	if strings.TrimSpace(req.Name) == "" {
		return ai.CreateUserResult{Success: false, Message: "姓名不能为空"}, fmt.Errorf("姓名不能为空")
	}
	if strings.TrimSpace(req.Email) == "" {
		return ai.CreateUserResult{Success: false, Message: "邮箱不能为空"}, fmt.Errorf("邮箱不能为空")
	}

	// 生成随机初始密码（12 位，含大小写字母+数字），避免硬编码和明文泄露
	password, err := generateRandomPassword(12)
	if err != nil {
		return ai.CreateUserResult{Success: false, Message: "生成随机密码失败"}, fmt.Errorf("生成随机密码失败: %w", err)
	}

	enabled := true
	u := User{
		Username: strings.TrimSpace(req.Username),
		Password: password,
		Name:     strings.TrimSpace(req.Name),
		Email:    strings.TrimSpace(req.Email),
		IsEnable: &enabled,
		Gid:      1, // 默认分组
	}
	// 与网页“新增用户”保持一致：未指定角色时绑定普通用户默认角色。
	if defaultRoleID := GetDefaultRoleID(db); defaultRoleID > 0 {
		u.RoleIDs = []uint{defaultRoleID}
	}

	if err := u.Create(); err != nil {
		return ai.CreateUserResult{
			Success: false,
			Message: fmt.Sprintf("创建用户失败: %v", err),
		}, err
	}

	// 与页面"新增用户"流程对齐：自动创建客户端配置（默认 true）
	autoCreateClient := true
	if req.AutoCreateClient != nil {
		autoCreateClient = *req.AutoCreateClient
	}
	sendNotifyEmail := true
	if req.SendNotifyEmail != nil {
		sendNotifyEmail = *req.SendNotifyEmail
	}

	createdClientName := ""
	clientGenErr := ""
	if autoCreateClient {
		clientName := u.Username
		if u.OvpnConfig == "" {
			u.OvpnConfig = clientName
			_ = db.Model(&User{}).Where("id = ?", u.ID).Update("ovpn_config", clientName).Error
		} else {
			clientName = u.OvpnConfig
		}
		if err := generateClientConfig(clientName, u.IsMFAEnabled()); err != nil {
			clientGenErr = err.Error()
			logger.Error(context.Background(), "auto create client for %s failed: %s", u.Username, err)
		} else {
			createdClientName = clientName
		}
	}

	// 与页面流程对齐：发送开通通知邮件（含密码和客户端配置附件）
	emailErr := ""
	if sendNotifyEmail && strings.TrimSpace(u.Email) != "" {
		go func(email, name, uname, pwd, clientName string) {
			var localPackages []LocalPackageInfo
			activePackages := GetActivePackagesByPlatform()
			for platform, pkg := range activePackages {
				localPackages = append(localPackages, LocalPackageInfo{
					Platform:      platform,
					PlatformLabel: PlatformLabel(platform),
					Version:       pkg.Version,
					DownloadURL:   pkg.PublicDownloadURL(),
				})
			}
			html, err := renderAccountEmailWithPackages("addUser", name, uname, pwd, localPackages)
			if err != nil {
				logger.Error(context.Background(), "渲染开通邮件模板失败: %s", err.Error())
				return
			}
			var attachments []string
			if clientName != "" {
				attachments = append(attachments, filepath.Join(ovData, "clients", clientName+".ovpn"))
			}
			if err := sendUserEmail(email, "用户开通通知", html, attachments, uname, "user_register"); err != nil {
				logger.Error(context.Background(), "发送开通邮件失败: %s", err.Error())
			}
		}(u.Email, u.Name, u.Username, password, createdClientName)
	} else if sendNotifyEmail && strings.TrimSpace(u.Email) == "" {
		emailErr = "用户未填写邮箱，跳过邮件发送"
	}

	// 构造诚实的 message：明确告知客户端生成/邮件发送的实际状态，
	// 避免 LLM 在最终回复中误以为全部成功
	var message string
	switch {
	case createdClientName != "" && emailErr == "":
		message = fmt.Sprintf("用户 %s 创建成功，已自动生成客户端配置 %s.ovpn，开通邮件已发送", req.Name, createdClientName)
	case createdClientName != "" && emailErr != "":
		message = fmt.Sprintf("用户 %s 创建成功，已自动生成客户端配置 %s.ovpn；但 %s", req.Name, createdClientName, emailErr)
	case createdClientName == "" && clientGenErr != "" && emailErr == "":
		message = fmt.Sprintf("用户 %s 创建成功，但生成客户端配置失败（%s）；邮件已发送", req.Name, clientGenErr)
	case createdClientName == "" && clientGenErr != "" && emailErr != "":
		message = fmt.Sprintf("用户 %s 创建成功，但生成客户端配置失败（%s）；%s", req.Name, clientGenErr, emailErr)
	default:
		message = fmt.Sprintf("用户 %s 创建成功，初始密码已随机生成", req.Name)
	}
	return ai.CreateUserResult{
		Success: true,
		Message: message,
		UserID:  u.ID,
	}, nil
}

// generateRandomPassword 生成指定长度的随机密码（含大小写字母+数字）
func generateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}
	return string(b), nil
}

// generateStrongPassword 生成满足 isValidPassword 规则的强密码：
// 长度 14，包含大小写字母、数字、特殊字符。后端 isValidPassword 要求同时满足 4 类。
// 用于 AI Agent 重置密码场景，比 create_user 的 generateRandomPassword 更安全。
func generateStrongPassword(length int) (string, error) {
	if length < 12 {
		length = 14
	}
	const (
		lower   = "abcdefghijklmnopqrstuvwxyz"
		upper   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		digit   = "0123456789"
		special = "!@#$%^&*"
		all     = lower + upper + digit + special
	)
	out := make([]byte, 0, length)
	// 先保证四类字符各出现一次，再补足剩余位
	required := []string{lower, upper, digit, special}
	for _, set := range required {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
		if err != nil {
			return "", err
		}
		out = append(out, set[n.Int64()])
	}
	for len(out) < length {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(all))))
		if err != nil {
			return "", err
		}
		out = append(out, all[n.Int64()])
	}
	// Fisher-Yates 洗牌，避免前 4 位固定是 lower/upper/digit/special
	for i := len(out) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", err
		}
		j := int(n.Int64())
		out[i], out[j] = out[j], out[i]
	}
	return string(out), nil
}

// renderAccountEmail 渲染 accountEmailTemplate；type 取值 resetPass / resetMfa / userRegister。
// 返回渲染后的 HTML 字符串与错误。
func renderAccountEmail(tplType, name, username, password string) (string, error) {
	return renderAccountEmailWithPackages(tplType, name, username, password, nil)
}

// renderAccountEmailWithPackages 渲染邮件模板，附带客户端安装包下载链接
func renderAccountEmailWithPackages(tplType, name, username, password string, packages []LocalPackageInfo) (string, error) {
	tpl, err := template.New("account-email").Parse(accountEmailTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, struct {
		Type          string
		Name          string
		Username      string
		Password      string
		SiteUrl       string
		LocalPackages []LocalPackageInfo
	}{
		Type:          tplType,
		Name:          name,
		Username:      username,
		Password:      password,
		SiteUrl:       siteDownloadLandingURL(),
		LocalPackages: packages,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ListUsers 列出用户（实现 ai.ToolService 接口），支持分页
// limit≤0 或 >200 时 clamp 到 50（默认）或 200（上限），避免大用户量部署 token 爆炸
func (s *AIToolService) ListUsers(ctx agent.ToolContext, operator string, req ai.ListUsersRequest) (ai.ListUsersResult, error) {
	// 权限校验
	if !s.hasPermission(ctx, operator, "user:view") {
		return ai.ListUsersResult{}, fmt.Errorf("权限不足: 需要 user:view 权限")
	}

	// 参数 clamp
	limit := req.Limit
	if limit <= 0 {
		limit = 50 // 默认 50
	}
	if limit > 200 {
		limit = 200 // 上限 200
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	u := User{}
	users := u.All()
	total := len(users)

	// 构建 UserInfo 切片（仅当前分页）
	end := offset + limit
	if end > total {
		end = total
	}
	if offset > total {
		offset = total
	}
	paged := users[offset:end]
	infos := make([]ai.UserInfo, 0, len(paged))
	for _, user := range paged {
		enabled := false
		if user.IsEnable != nil {
			enabled = *user.IsEnable
		}
		infos = append(infos, ai.UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Name:     user.Name,
			Email:    user.Email,
			Enabled:  enabled,
		})
	}

	return ai.ListUsersResult{
		Total:  total, // 全量总数，让 LLM 知道是否还有更多
		Limit:  limit,
		Offset: offset,
		Users:  infos,
	}, nil
}

// BindRole 给用户绑定角色（实现 ai.ToolService 接口）
// GetSystemCounts 返回 AI 运维所需的最小统计集。
// 用户统计与客户端配置分别受 user:view、client:view 控制；在线数额外要求 client:view_online。
// 管理接口不可达不是工具失败，而是通过 ManagementOK 告知模型真实状态。
func (s *AIToolService) GetSystemCounts(ctx agent.ToolContext, operator string) (ai.SystemCountsResult, error) {
	if !s.hasPermission(ctx, operator, "user:view") {
		return ai.SystemCountsResult{}, fmt.Errorf("权限不足: 需要 user:view 权限")
	}
	if !s.hasPermission(ctx, operator, "client:view") {
		return ai.SystemCountsResult{}, fmt.Errorf("权限不足: 需要 client:view 权限")
	}

	result := ai.SystemCountsResult{ClientConfigs: countClientConfigs()}
	if err := db.WithContext(ctx).Model(&User{}).Count(&result.TotalUsers).Error; err != nil {
		return ai.SystemCountsResult{}, fmt.Errorf("查询用户总数失败: %w", err)
	}
	if err := db.WithContext(ctx).Model(&User{}).Where("is_enable = ?", true).Count(&result.EnabledUsers).Error; err != nil {
		return ai.SystemCountsResult{}, fmt.Errorf("查询启用用户数失败: %w", err)
	}

	if s.hasPermission(ctx, operator, "client:view_online") {
		if s.ov == nil {
			return ai.SystemCountsResult{}, fmt.Errorf("OpenVPN 运行时未初始化")
		}
		clients, managementOK := s.ov.safeOnlineClients()
		result.OnlineClients = len(clients)
		result.ManagementOK = managementOK
	}
	return result, nil
}

func (s *AIToolService) BindRole(ctx agent.ToolContext, operator string, req ai.BindRoleRequest) (ai.BindRoleResult, error) {
	// 权限校验
	if !s.hasPermission(ctx, operator, "role:assign") {
		return ai.BindRoleResult{
			Success: false,
			Message: "当前用户无 role:assign 权限，无法绑定角色",
		}, fmt.Errorf("权限不足: 需要 role:assign 权限")
	}

	if req.UserID == 0 || req.RoleID == 0 {
		return ai.BindRoleResult{Success: false, Message: "用户 ID 和角色 ID 不能为空"}, fmt.Errorf("参数无效")
	}

	// 校验用户存在
	var user User
	if err := db.WithContext(ctx).First(&user, req.UserID).Error; err != nil {
		return ai.BindRoleResult{Success: false, Message: "用户不存在"}, fmt.Errorf("用户不存在: %w", err)
	}

	// 校验角色存在且启用
	var role Role
	if err := db.WithContext(ctx).Where("id = ?", req.RoleID).First(&role).Error; err != nil {
		return ai.BindRoleResult{Success: false, Message: "角色不存在"}, fmt.Errorf("角色不存在: %w", err)
	}
	if role.IsEnable != nil && !*role.IsEnable {
		return ai.BindRoleResult{Success: false, Message: "角色已被禁用，无法绑定"}, fmt.Errorf("角色已禁用")
	}

	// 绑定角色（INSERT OR IGNORE 语义：FirstOrCreate 避免重复主键冲突）
	if err := db.WithContext(ctx).Where("user_id = ? AND role_id = ?", req.UserID, req.RoleID).
		FirstOrCreate(&UserRole{}, UserRole{UserID: req.UserID, RoleID: req.RoleID}).Error; err != nil {
		return ai.BindRoleResult{
			Success: false,
			Message: fmt.Sprintf("绑定角色失败: %v", err),
		}, err
	}

	return ai.BindRoleResult{
		Success: true,
		Message: fmt.Sprintf("已为用户 %s 绑定角色 %s", user.Username, role.Name),
	}, nil
}

// ResetPassword 重置指定用户的登录密码（实现 ai.ToolService 接口）
//
// 行为：
//  1. 校验 operator 具有 user:reset_password 权限
//  2. 通过 username 查用户；admin 用户名仅允许 admin 操作者自身重置
//  3. 若调用方传入 newPassword 则校验强度；否则服务端生成 14 位强密码
//  4. 写入密码 + 标记 isFirstLogin=true（密码被改后下次必须重设）
//  5. notifyEmail=true 时异步发送密码重置邮件（不阻塞返回）
//
// 返回 ResetPasswordResult；服务端生成的密码会通过 GeneratedPassword 字段带出，
// 仅用于本轮会话回复用户，不会持久化明文。
func (s *AIToolService) ResetPassword(ctx agent.ToolContext, operator string, req ai.ResetPasswordRequest) (ai.ResetPasswordResult, error) {
	if !s.hasPermission(ctx, operator, "user:reset_password") {
		return ai.ResetPasswordResult{
			Success: false,
			Message: "当前用户无 user:reset_password 权限，无法重置密码",
		}, fmt.Errorf("权限不足: 需要 user:reset_password 权限")
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		return ai.ResetPasswordResult{Success: false, Message: "username 不能为空"}, fmt.Errorf("username 不能为空")
	}

	var target User
	if err := db.WithContext(ctx).Where("username = ?", username).First(&target).Error; err != nil {
		return ai.ResetPasswordResult{
			Success: false,
			Message: fmt.Sprintf("用户 %s 不存在", username),
		}, fmt.Errorf("用户不存在: %w", err)
	}

	// 保护 admin：只有 admin 才能改 admin 自己，避免普通用户被 Lockout
	if target.Username == adminUsername && operator != adminUsername {
		return ai.ResetPasswordResult{
			Success: false,
			Message: "内置 admin 用户的密码仅允许 admin 重置",
		}, fmt.Errorf("禁止越权重置 admin 密码")
	}

	newPassword := req.NewPassword
	if strings.TrimSpace(newPassword) == "" {
		generated, err := generateStrongPassword(14)
		if err != nil {
			return ai.ResetPasswordResult{Success: false, Message: "生成强密码失败"}, fmt.Errorf("生成强密码失败: %w", err)
		}
		newPassword = generated
	} else {
		// 校验调用方提供的新密码满足 isValidPassword 规则
		if !isValidPassword(newPassword) {
			return ai.ResetPasswordResult{
				Success: false,
				Message: "新密码不满足强度要求：长度 ≥ 12 且同时包含大小写字母、数字、特殊字符",
			}, fmt.Errorf("密码强度不足")
		}
	}

	isFirstLogin := true
	// 通过 struct Updates 触发 User.BeforeSave（AES 加密 + XSS 过滤）
	if err := db.WithContext(ctx).Model(&target).Updates(User{
		Password:     newPassword,
		IsFirstLogin: &isFirstLogin,
	}).Error; err != nil {
		return ai.ResetPasswordResult{
			Success: false,
			Message: fmt.Sprintf("写入新密码失败: %v", err),
		}, err
	}

	// 异步邮件通知，避免阻塞 AI 流式响应
	if req.NotifyEmail && target.Email != "" {
		go func(email, name, uname, pwd string) {
			html, err := renderAccountEmail("resetPass", name, uname, pwd)
			if err != nil {
				logger.Error(context.Background(), "渲染密码重置邮件模板失败: %s", err.Error())
				return
			}
			if sendErr := sendUserEmail(email, "用户密码配置通知", html, nil, uname, "password_reset"); sendErr != nil {
				logger.Error(context.Background(), "发送密码重置邮件失败: %s", sendErr.Error())
			}
		}(target.Email, target.Name, target.Username, newPassword)
	}

	result := ai.ResetPasswordResult{
		Success: true,
		Message: fmt.Sprintf("用户 %s 的密码已重置", target.Username),
		UserID:  target.ID,
	}
	// 仅当服务端自动生成时返回明文密码（供本轮对话回复用户）
	if strings.TrimSpace(req.NewPassword) == "" {
		result.GeneratedPassword = newPassword
		result.Message = fmt.Sprintf("已为用户 %s 生成新密码：%s（请通过安全渠道下发，下次登录需修改）", target.Username, newPassword)
	}
	return result, nil
}

// ResetMFA 重置指定用户的多因素认证（实现 ai.ToolService 接口）
//
// 行为：
//  1. 校验 operator 具有 user:reset_mfa 权限
//  2. 清空用户 mfa_secret 并将 mfa_enabled 置为 false
//  3. 调用 RegenerateUserClientConfigs 移除 .ovpn 中的 static-challenge
//  4. 用户邮箱非空时异步发送 MFA 重置通知邮件
func (s *AIToolService) ResetMFA(ctx agent.ToolContext, operator string, req ai.ResetMFARequest) (ai.ResetMFAResult, error) {
	if !s.hasPermission(ctx, operator, "user:reset_mfa") {
		return ai.ResetMFAResult{
			Success: false,
			Message: "当前用户无 user:reset_mfa 权限，无法重置 MFA",
		}, fmt.Errorf("权限不足: 需要 user:reset_mfa 权限")
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		return ai.ResetMFAResult{Success: false, Message: "username 不能为空"}, fmt.Errorf("username 不能为空")
	}

	var target User
	if err := db.WithContext(ctx).Where("username = ?", username).First(&target).Error; err != nil {
		return ai.ResetMFAResult{
			Success: false,
			Message: fmt.Sprintf("用户 %s 不存在", username),
		}, fmt.Errorf("用户不存在: %w", err)
	}

	if err := db.WithContext(ctx).Model(&target).Updates(map[string]interface{}{
		"mfa_secret":  nil,
		"mfa_enabled": false,
	}).Error; err != nil {
		return ai.ResetMFAResult{
			Success: false,
			Message: fmt.Sprintf("清空 MFA 失败: %v", err),
		}, err
	}

	// 重建关联的 .ovpn 移除 static-challenge（即使未启用 MFA 也要保证配置一致）
	updated, regenErr := RegenerateUserClientConfigs(target.Username, false)
	if regenErr != nil {
		// 不阻断主流程，但记录警告
		logger.Warn(context.Background(),
			fmt.Sprintf("重置 MFA 后重建客户端配置失败: %s", regenErr.Error()))
	}

	// 异步发送 MFA 重置通知邮件
	if target.Email != "" {
		go func(email, name, uname string, attachments []string) {
			html, err := renderAccountEmail("resetMfa", name, uname, "")
			if err != nil {
				logger.Error(context.Background(), "渲染 MFA 重置邮件模板失败: %s", err.Error())
				return
			}
			// 附件：重新生成的客户端配置（如有）
			var paths []string
			for _, configName := range attachments {
				fp := filepath.Join(ovData, "clients", configName+".ovpn")
				if fileExists(fp) {
					paths = append(paths, fp)
				}
			}
			if sendErr := sendUserEmail(email, "用户 MFA 重置通知", html, paths, uname, "mfa_reset"); sendErr != nil {
				logger.Error(context.Background(), "发送 MFA 重置邮件失败: %s", sendErr.Error())
			}
		}(target.Email, target.Name, target.Username, updated)
	}

	stripExt := func(s string) string { return strings.TrimSuffix(s, ".ovpn") }
	updatedNames := make([]string, 0, len(updated))
	for _, n := range updated {
		updatedNames = append(updatedNames, stripExt(n))
	}

	return ai.ResetMFAResult{
		Success:      true,
		Message:      fmt.Sprintf("用户 %s 的 MFA 已重置，下次登录需要重新绑定", target.Username),
		UserID:       target.ID,
		UpdatedFiles: updatedNames,
	}, nil
}

// GenerateClient 为指定用户生成 .ovpn 客户端配置（实现 ai.ToolService 接口）
//
// 行为：
//  1. 校验 operator 具有 client:create 权限
//  2. 校验目标用户存在；如已绑定 OvpnConfig 且同名 .ovpn 已存在，则拒绝覆盖
//  3. 调用 generateClientConfig (viper 包装的便捷函数) 生成证书 + .ovpn
//  4. 同步把客户端配置名写入 User.OvpnConfig
//  5. mfaEnabled=true 时用户旧 MFA 状态可被忽略（Ovpn 由用户表当前 mfa 决定）
//
// 设计取舍：同名客户端已存在不覆盖，避免误删证书；如需重签先删除再调本工具。
func (s *AIToolService) GenerateClient(ctx agent.ToolContext, operator string, req ai.GenerateClientRequest) (ai.GenerateClientResult, error) {
	if !s.hasPermission(ctx, operator, "client:create") {
		return ai.GenerateClientResult{
			Success: false,
			Message: "当前用户无 client:create 权限，无法生成客户端配置",
		}, fmt.Errorf("权限不足: 需要 client:create 权限")
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		return ai.GenerateClientResult{Success: false, Message: "username 不能为空"}, fmt.Errorf("username 不能为空")
	}

	var target User
	if err := db.WithContext(ctx).Where("username = ?", username).First(&target).Error; err != nil {
		return ai.GenerateClientResult{
			Success: false,
			Message: fmt.Sprintf("用户 %s 不存在", username),
		}, fmt.Errorf("用户不存在: %w", err)
	}

	// 检查是否已存在同名 .ovpn，若存在则拒绝（避免覆盖用户已有的客户端）
	clientName := username
	clientsDir := filepath.Join(ovData, "clients")
	ovpnFile := filepath.Join(clientsDir, clientName+".ovpn")
	if fileExists(ovpnFile) {
		return ai.GenerateClientResult{
			Success:    false,
			Message:    fmt.Sprintf("客户端 %s 已存在；如需重新签发，请先通过客户端管理删除旧配置后再调此工具", clientName),
			ClientName: clientName,
			UserID:     target.ID,
		}, fmt.Errorf("客户端已存在")
	}

	// CCD：传入则写到 ccd/<name>，否则不写
	if strings.TrimSpace(req.CCDConfig) != "" {
		ccdDir := filepath.Join(ovData, "ccd")
		if err := os.MkdirAll(ccdDir, 0o755); err != nil {
			return ai.GenerateClientResult{Success: false, Message: "写入 CCD 目录失败"}, fmt.Errorf("创建 ccd 目录失败: %w", err)
		}
		if err := os.WriteFile(filepath.Join(ccdDir, clientName), []byte(req.CCDConfig), 0o644); err != nil {
			return ai.GenerateClientResult{Success: false, Message: "写入 CCD 文件失败"}, fmt.Errorf("写入 CCD 失败: %w", err)
		}
	}

	// 决定 server addr / port：调用方传入优先；否则沿用后端 viper 配置和 POST /client 路由保持完全一致
	serverAddr := strings.TrimSpace(req.ServerAddr)
	serverPort := strings.TrimSpace(req.ServerPort)
	mfaEnabled := target.IsMFAEnabled()

	if serverAddr != "" && serverPort != "" {
		// 调用方两条都给齐，调用底层生成函数（保留与 POST /client 完全一致的语义）
		proto := strings.TrimSpace(viper.GetString("openvpn.ovpn_proto"))
		if proto == "" {
			proto = "udp"
		}
		ipv6 := viper.GetBool("openvpn.ovpn_ipv6")
		if err := generateClientConfigGo(clientName, serverAddr, serverPort, proto, ipv6, "", mfaEnabled); err != nil {
			return ai.GenerateClientResult{Success: false, Message: fmt.Sprintf("生成客户端配置失败: %v", err)}, err
		}
	} else {
		// 默认走 viper 配置（与"添加用户自动创建客户端"流程完全一致）
		if err := generateClientConfig(clientName, mfaEnabled); err != nil {
			return ai.GenerateClientResult{Success: false, Message: fmt.Sprintf("生成客户端配置失败: %v", err)}, err
		}
	}

	// 把生成的 .ovpn 文件名写回 User 表（便于前端用户表单显示和后续下载）
	if err := db.WithContext(ctx).Model(&target).Update("ovpn_config", clientName+".ovpn").Error; err != nil {
		logger.Warn(context.Background(),
			fmt.Sprintf("客户端生成成功但写入 User.OvpnConfig 失败: %s", err.Error()))
	}

	return ai.GenerateClientResult{
		Success:    true,
		Message:    fmt.Sprintf("已为用户 %s 生成客户端 %s.ovpn", target.Username, clientName),
		ClientName: clientName,
		UserID:     target.ID,
		OvpnConfig: clientName + ".ovpn",
	}, nil
}

// UpdateUser 更新用户信息（实现 ai.ToolService 接口）
func (s *AIToolService) UpdateUser(ctx agent.ToolContext, operator string, req ai.UpdateUserRequest) (ai.UpdateUserResult, error) {
	if !s.hasPermission(ctx, operator, "user:update") {
		return ai.UpdateUserResult{Success: false, Message: "无 user:update 权限"}, fmt.Errorf("权限不足: 需要 user:update 权限")
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		return ai.UpdateUserResult{Success: false, Message: "username 不能为空"}, fmt.Errorf("username 不能为空")
	}
	var target User
	if err := db.WithContext(ctx).Where("username = ?", username).First(&target).Error; err != nil {
		return ai.UpdateUserResult{Success: false, Message: fmt.Sprintf("用户 %s 不存在", username)}, fmt.Errorf("用户不存在: %w", err)
	}
	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Email != "" {
		updates["email"] = req.Email
	}
	if req.IsEnable != nil {
		updates["is_enable"] = *req.IsEnable
	}
	if req.ExpireDate != "" {
		updates["expire_date"] = req.ExpireDate
	}
	if req.IpAddr != "" {
		updates["ip_addr"] = req.IpAddr
	}
	if len(updates) == 0 {
		return ai.UpdateUserResult{Success: false, Message: "没有需要更新的字段"}, fmt.Errorf("没有需要更新的字段")
	}
	if err := db.WithContext(ctx).Model(&target).Updates(updates).Error; err != nil {
		return ai.UpdateUserResult{Success: false, Message: fmt.Sprintf("更新失败: %v", err)}, err
	}
	return ai.UpdateUserResult{Success: true, Message: fmt.Sprintf("用户 %s 已更新", username), UserID: target.ID}, nil
}

// DeleteUser 删除用户（实现 ai.ToolService 接口）
func (s *AIToolService) DeleteUser(ctx agent.ToolContext, operator string, req ai.DeleteUserRequest) (ai.DeleteUserResult, error) {
	if !s.hasPermission(ctx, operator, "user:delete") {
		return ai.DeleteUserResult{Success: false, Message: "无 user:delete 权限"}, fmt.Errorf("权限不足: 需要 user:delete 权限")
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		return ai.DeleteUserResult{Success: false, Message: "username 不能为空"}, fmt.Errorf("username 不能为空")
	}
	if username == adminUsername {
		return ai.DeleteUserResult{Success: false, Message: "内置 admin 用户不可删除"}, fmt.Errorf("禁止删除 admin")
	}
	var target User
	if err := db.WithContext(ctx).Where("username = ?", username).First(&target).Error; err != nil {
		return ai.DeleteUserResult{Success: false, Message: fmt.Sprintf("用户 %s 不存在", username)}, fmt.Errorf("用户不存在: %w", err)
	}

	// 与页面删除用户流程对齐：先清理客户端配置和证书，再删数据库记录
	// 客户端配置名：优先使用 ovpn_config 字段，兜底用 username
	clientName := target.OvpnConfig
	if clientName == "" {
		clientName = username
	}

	// 1. 撤销客户端证书（证书可能不存在，只记录警告不阻断）
	certRevoked := false
	if revErr := RevokeByName(clientName); revErr != nil {
		logger.Warn(context.Background(), fmt.Sprintf("DeleteUser: 撤销证书 %s 失败（可能不存在）: %s", clientName, revErr.Error()))
	} else {
		certRevoked = true
		if s.ov != nil {
			s.ov.sendCommand("signal SIGHUP")
		}
	}

	// 2. 删除客户端配置文件和 CCD 目录
	ovpnFile := filepath.Join(ovData, "clients", clientName+".ovpn")
	ccdDir := filepath.Join(ovData, "ccd", clientName)
	ovpnRemoved := false
	if os.Remove(ovpnFile) == nil {
		ovpnRemoved = true
	}
	os.Remove(ccdDir)

	// 3. 删除用户数据库记录
	if err := target.Delete(fmt.Sprintf("%d", target.ID)); err != nil {
		return ai.DeleteUserResult{Success: false, Message: fmt.Sprintf("删除失败: %v", err)}, err
	}

	// 构造结果消息
	parts := []string{fmt.Sprintf("用户 %s 已删除", username)}
	if ovpnRemoved {
		parts = append(parts, fmt.Sprintf("客户端配置 %s.ovpn 已删除", clientName))
	}
	if certRevoked {
		parts = append(parts, "客户端证书已吊销")
	}
	return ai.DeleteUserResult{Success: true, Message: strings.Join(parts, "，")}, nil
}

// ListOnlineClients 列出在线 VPN 客户端
func (s *AIToolService) ListOnlineClients(ctx agent.ToolContext, operator string) (ai.ListOnlineClientsResult, error) {
	if !s.hasPermission(ctx, operator, "client:view_online") {
		return ai.ListOnlineClientsResult{}, fmt.Errorf("权限不足: 需要 client:view_online 权限")
	}
	if s.ov == nil {
		return ai.ListOnlineClientsResult{}, fmt.Errorf("OpenVPN 运行时未初始化")
	}
	clients, managementOK := s.ov.safeOnlineClients()
	server, _ := s.ov.safeServerData()
	infos := make([]ai.OnlineClientInfo, 0, len(clients))
	for _, c := range clients {
		infos = append(infos, ai.OnlineClientInfo{
			CID:         c.ID,
			RealIP:      c.Rip,
			VirtualIP:   c.Vip,
			Username:    c.Username,
			CommonName:  c.CommonName,
			RecvBytes:   c.RecvBytes,
			SendBytes:   c.SendBytes,
			ConnDate:    c.ConnDate,
			OnlineTime:  c.OnlineTime,
			IsBlacklist: c.IsNftBlacklist,
		})
	}
	return ai.ListOnlineClientsResult{
		OnlineClients: infos,
		ServerStatus:  server.Status,
		ManagementOK:  managementOK,
	}, nil
}

// KillConnection 断开在线 VPN 连接
func (s *AIToolService) KillConnection(ctx agent.ToolContext, operator string, req ai.KillConnectionRequest) (ai.KillConnectionResult, error) {
	if !s.hasPermission(ctx, operator, "client:kill") {
		return ai.KillConnectionResult{Success: false, Message: "无 client:kill 权限"}, fmt.Errorf("权限不足: 需要 client:kill 权限")
	}
	if req.CID == "" {
		return ai.KillConnectionResult{Success: false, Message: "cid 不能为空"}, fmt.Errorf("cid 不能为空")
	}
	if s.ov == nil {
		return ai.KillConnectionResult{Success: false, Message: "OpenVPN 运行时未初始化"}, fmt.Errorf("OpenVPN 运行时未初始化")
	}
	s.ov.killClient(req.CID)
	return ai.KillConnectionResult{Success: true, Message: fmt.Sprintf("已断开连接 %s", req.CID)}, nil
}

// ListClients 列出所有 .ovpn 客户端配置文件
func (s *AIToolService) ListClients(ctx agent.ToolContext, operator string) (ai.ListClientsResult, error) {
	if !s.hasPermission(ctx, operator, "client:view") {
		return ai.ListClientsResult{}, fmt.Errorf("权限不足: 需要 client:view 权限")
	}
	clientsDir := filepath.Join(ovData, "clients")
	files, err := os.ReadDir(clientsDir)
	if err != nil {
		return ai.ListClientsResult{}, fmt.Errorf("读取客户端目录失败: %w", err)
	}
	infos := make([]ai.ClientConfigInfo, 0, len(files))
	for _, file := range files {
		if file.IsDir() || !strings.EqualFold(filepath.Ext(file.Name()), ".ovpn") {
			continue
		}
		finfo, _ := file.Info()
		name := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
		infos = append(infos, ai.ClientConfigInfo{
			Name:     name,
			FullName: file.Name(),
			Date:     finfo.ModTime().Local().Format("2006-01-02 15:04:05"),
		})
	}
	return ai.ListClientsResult{Clients: infos, Total: len(infos)}, nil
}

// DeleteClient 删除客户端配置并吊销证书
func (s *AIToolService) DeleteClient(ctx agent.ToolContext, operator string, req ai.DeleteClientRequest) (ai.DeleteClientResult, error) {
	if !s.hasPermission(ctx, operator, "client:delete") {
		return ai.DeleteClientResult{Success: false, Message: "无 client:delete 权限"}, fmt.Errorf("权限不足: 需要 client:delete 权限")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return ai.DeleteClientResult{Success: false, Message: "name 不能为空"}, fmt.Errorf("name 不能为空")
	}
	ovpnFile := filepath.Join(ovData, "clients", name+".ovpn")
	if !fileExists(ovpnFile) {
		return ai.DeleteClientResult{Success: false, Message: fmt.Sprintf("客户端 %s 不存在", name)}, fmt.Errorf("客户端不存在")
	}
	if revErr := RevokeByName(name); revErr != nil {
		if fileExists(ovpnFile) || fileExists(clientCertPath(name)) {
			return ai.DeleteClientResult{Success: false, Message: fmt.Sprintf("吊销证书失败: %v", revErr)}, revErr
		}
		logger.Warn(context.Background(), fmt.Sprintf("吊销证书失败，继续清理: %s", revErr.Error()))
	} else if s.ov != nil {
		s.ov.sendCommand("signal SIGHUP")
	}
	os.Remove(ovpnFile)
	os.Remove(filepath.Join(ovData, "ccd", name))
	return ai.DeleteClientResult{Success: true, Message: fmt.Sprintf("客户端 %s 已删除", name)}, nil
}

// UpdateCCD 更新客户端 CCD 配置
func (s *AIToolService) UpdateCCD(ctx agent.ToolContext, operator string, req ai.UpdateCCDRequest) (ai.UpdateCCDResult, error) {
	if !s.hasPermission(ctx, operator, "client:create") {
		return ai.UpdateCCDResult{Success: false, Message: "无 client:create 权限"}, fmt.Errorf("权限不足: 需要 client:create 权限")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return ai.UpdateCCDResult{Success: false, Message: "name 不能为空"}, fmt.Errorf("name 不能为空")
	}
	ccdDir := filepath.Join(ovData, "ccd")
	if err := os.MkdirAll(ccdDir, 0o755); err != nil {
		return ai.UpdateCCDResult{Success: false, Message: "创建 CCD 目录失败"}, fmt.Errorf("创建 CCD 目录失败: %w", err)
	}
	// 确保OpenVPN配置中启用了 client-config-dir
	cfg, err := initOvpnConfig()
	if err == nil && cfg.Get("client-config-dir") == "" {
		cfg.Set("client-config-dir", ccdDir)
		cfg.Save()
	}
	if err := os.WriteFile(filepath.Join(ccdDir, name), []byte(req.Content), 0o644); err != nil {
		return ai.UpdateCCDResult{Success: false, Message: "写入 CCD 失败"}, fmt.Errorf("写入 CCD 失败: %w", err)
	}
	return ai.UpdateCCDResult{Success: true, Message: fmt.Sprintf("客户端 %s 的 CCD 配置已更新", name)}, nil
}

// RegenerateClient 重新生成客户端 .ovpn 配置
func (s *AIToolService) RegenerateClient(ctx agent.ToolContext, operator string, req ai.RegenerateClientRequest) (ai.RegenerateClientResult, error) {
	if !s.hasPermission(ctx, operator, "client:regenerate") {
		return ai.RegenerateClientResult{Success: false, Message: "无 client:regenerate 权限"}, fmt.Errorf("权限不足: 需要 client:regenerate 权限")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return ai.RegenerateClientResult{Success: false, Message: "name 不能为空"}, fmt.Errorf("name 不能为空")
	}
	// 查询关联用户的 MFA 状态
	mfaEnabled := false
	var queryUser User
	if err := db.WithContext(ctx).Where("username = ? OR ovpn_config = ?", name, name+".ovpn").First(&queryUser).Error; err == nil {
		mfaEnabled = queryUser.IsMFAEnabled()
	}
	if err := generateClientConfig(name, mfaEnabled); err != nil {
		return ai.RegenerateClientResult{Success: false, Message: fmt.Sprintf("重新生成配置失败: %v", err)}, err
	}
	return ai.RegenerateClientResult{Success: true, Message: fmt.Sprintf("客户端 %s 的配置已重新生成", name), ClientName: name}, nil
}

// ListFirewallRules 列出防火墙规则
func (s *AIToolService) ListFirewallRules(ctx agent.ToolContext, operator string) (ai.ListFirewallRulesResult, error) {
	if !s.hasPermission(ctx, operator, "firewall:view") {
		return ai.ListFirewallRulesResult{}, fmt.Errorf("权限不足: 需要 firewall:view 权限")
	}
	var f Firewall
	rules := f.All()
	infos := make([]ai.FirewallRuleInfo, 0, len(rules))
	for _, r := range rules {
		status := false
		if r.Status != nil {
			status = *r.Status
		}
		infos = append(infos, ai.FirewallRuleInfo{
			ID:      r.ID,
			SIP:     r.SIP,
			DIP:     r.DIP,
			Policy:  r.Policy,
			Status:  status,
			Comment: r.Comment,
		})
	}
	return ai.ListFirewallRulesResult{Rules: infos, Total: len(infos)}, nil
}

// ManageFirewall 管理防火墙规则（黑名单/限速）
func (s *AIToolService) ManageFirewall(ctx agent.ToolContext, operator string, req ai.ManageFirewallRequest) (ai.ManageFirewallResult, error) {
	ip := strings.TrimSpace(req.IP)
	if ip == "" {
		return ai.ManageFirewallResult{Success: false, Message: "IP 不能为空"}, fmt.Errorf("IP 不能为空")
	}
	switch req.Action {
	case "add_blacklist":
		if !s.hasPermission(ctx, operator, "firewall:create") {
			return ai.ManageFirewallResult{Success: false, Message: "无 firewall:create 权限"}, fmt.Errorf("权限不足")
		}
		if err := setNftBlackList(ip, "add"); err != nil {
			return ai.ManageFirewallResult{Success: false, Message: fmt.Sprintf("拉黑失败: %v", err)}, err
		}
		return ai.ManageFirewallResult{Success: true, Message: fmt.Sprintf("已将 %s 加入黑名单", ip)}, nil
	case "remove_blacklist":
		if !s.hasPermission(ctx, operator, "firewall:delete") {
			return ai.ManageFirewallResult{Success: false, Message: "无 firewall:delete 权限"}, fmt.Errorf("权限不足")
		}
		if err := setNftBlackList(ip, "delete"); err != nil {
			return ai.ManageFirewallResult{Success: false, Message: fmt.Sprintf("解除拉黑失败: %v", err)}, err
		}
		return ai.ManageFirewallResult{Success: true, Message: fmt.Sprintf("已将 %s 移出黑名单", ip)}, nil
	case "set_rateLimit":
		if !s.hasPermission(ctx, operator, "firewall:create") {
			return ai.ManageFirewallResult{Success: false, Message: "无 firewall:create 权限"}, fmt.Errorf("权限不足")
		}
		if err := setNftQosChain("upload", ip, req.Upload, req.UploadUnit); err != nil {
			return ai.ManageFirewallResult{Success: false, Message: fmt.Sprintf("设置上传限速失败: %v", err)}, err
		}
		if err := setNftQosChain("download", ip, req.Download, req.DownloadUnit); err != nil {
			return ai.ManageFirewallResult{Success: false, Message: fmt.Sprintf("设置下载限速失败: %v", err)}, err
		}
		return ai.ManageFirewallResult{Success: true, Message: fmt.Sprintf("已为 %s 设置限速（↑%s%s ↓%s%s）", ip, req.Upload, req.UploadUnit, req.Download, req.DownloadUnit)}, nil
	case "remove_rateLimit":
		if !s.hasPermission(ctx, operator, "firewall:delete") {
			return ai.ManageFirewallResult{Success: false, Message: "无 firewall:delete 权限"}, fmt.Errorf("权限不足")
		}
		// setNftQosChain 传 rate="0" 会删除已有的限速规则
		_ = setNftQosChain("upload", ip, "0", "")
		_ = setNftQosChain("download", ip, "0", "")
		return ai.ManageFirewallResult{Success: true, Message: fmt.Sprintf("已移除 %s 的限速规则", ip)}, nil
	default:
		return ai.ManageFirewallResult{Success: false, Message: fmt.Sprintf("未知操作: %s", req.Action)}, fmt.Errorf("未知操作: %s", req.Action)
	}
}

// ListCerts 列出 PKI 证书
func (s *AIToolService) ListCerts(ctx agent.ToolContext, operator string) (ai.ListCertsResult, error) {
	if !s.hasPermission(ctx, operator, "cert:view") {
		return ai.ListCertsResult{}, fmt.Errorf("权限不足: 需要 cert:view 权限")
	}
	certs := getCerts(ovData)
	infos := make([]ai.CertInfo, 0, len(certs))
	for _, c := range certs {
		infos = append(infos, ai.CertInfo{
			Name:      c.Name,
			Type:      c.Type,
			NotBefore: c.NotBefore,
			NotAfter:  c.NotAfter,
			Status:    c.Status,
			Issuer:    c.Issuer,
			Subject:   c.Subject,
			Serial:    c.SerialNo,
		})
	}
	return ai.ListCertsResult{Certs: infos, Total: len(infos)}, nil
}

// ListChannels 列出通知渠道
func (s *AIToolService) ListChannels(ctx agent.ToolContext, operator string) (ai.ListChannelsResult, error) {
	if !s.hasPermission(ctx, operator, "channel:view") {
		return ai.ListChannelsResult{}, fmt.Errorf("权限不足: 需要 channel:view 权限")
	}
	channels := (&NotificationChannel{}).All()
	infos := make([]ai.ChannelInfo, 0, len(channels))
	for _, ch := range channels {
		infos = append(infos, ai.ChannelInfo{
			ID:      ch.ID,
			Name:    ch.Name,
			Type:    ch.Type,
			Enabled: ch.Enabled,
		})
	}
	return ai.ListChannelsResult{Channels: infos, Total: len(infos)}, nil
}

// ManageChannel 管理通知渠道（创建/更新/删除/测试）
func (s *AIToolService) ManageChannel(ctx agent.ToolContext, operator string, req ai.ManageChannelRequest) (ai.ManageChannelResult, error) {
	switch req.Action {
	case "create":
		if !s.hasPermission(ctx, operator, "channel:create") {
			return ai.ManageChannelResult{Success: false, Message: "无 channel:create 权限"}, fmt.Errorf("权限不足")
		}
		if req.Name == "" || req.Type == "" {
			return ai.ManageChannelResult{Success: false, Message: "name 和 type 必填"}, fmt.Errorf("参数不完整")
		}
		ch := NotificationChannel{Name: req.Name, Type: req.Type, Enabled: true}
		if req.Config != "" {
			ch.Config = json.RawMessage(req.Config)
		}
		if err := ch.Create(); err != nil {
			return ai.ManageChannelResult{Success: false, Message: fmt.Sprintf("创建失败: %v", err)}, err
		}
		return ai.ManageChannelResult{Success: true, Message: fmt.Sprintf("渠道 %s 已创建", req.Name), ID: ch.ID}, nil
	case "update":
		if !s.hasPermission(ctx, operator, "channel:update") {
			return ai.ManageChannelResult{Success: false, Message: "无 channel:update 权限"}, fmt.Errorf("权限不足")
		}
		if req.ID == 0 {
			return ai.ManageChannelResult{Success: false, Message: "id 必填"}, fmt.Errorf("参数不完整")
		}
		ch, err := (&NotificationChannel{}).Get(req.ID)
		if err != nil {
			return ai.ManageChannelResult{Success: false, Message: fmt.Sprintf("渠道不存在: %v", err)}, err
		}
		if req.Name != "" {
			ch.Name = req.Name
		}
		if req.Type != "" {
			ch.Type = req.Type
		}
		if req.Enabled != nil {
			ch.Enabled = *req.Enabled
		}
		if req.Config != "" {
			ch.Config = json.RawMessage(req.Config)
		}
		if err := ch.Update(); err != nil {
			return ai.ManageChannelResult{Success: false, Message: fmt.Sprintf("更新失败: %v", err)}, err
		}
		return ai.ManageChannelResult{Success: true, Message: fmt.Sprintf("渠道 %s 已更新", ch.Name), ID: ch.ID}, nil
	case "delete":
		if !s.hasPermission(ctx, operator, "channel:delete") {
			return ai.ManageChannelResult{Success: false, Message: "无 channel:delete 权限"}, fmt.Errorf("权限不足")
		}
		if req.ID == 0 {
			return ai.ManageChannelResult{Success: false, Message: "id 必填"}, fmt.Errorf("参数不完整")
		}
		ch := NotificationChannel{ID: req.ID}
		if err := ch.Delete(); err != nil {
			return ai.ManageChannelResult{Success: false, Message: fmt.Sprintf("删除失败: %v", err)}, err
		}
		return ai.ManageChannelResult{Success: true, Message: "渠道已删除"}, nil
	default:
		return ai.ManageChannelResult{Success: false, Message: fmt.Sprintf("未知操作: %s", req.Action)}, fmt.Errorf("未知操作: %s", req.Action)
	}
}

// QueryAuditLogs 查询操作审计日志
func (s *AIToolService) QueryAuditLogs(ctx agent.ToolContext, operator string, req ai.QueryAuditLogsRequest) (ai.QueryAuditLogsResult, error) {
	if !s.hasPermission(ctx, operator, "audit:view") {
		return ai.QueryAuditLogsResult{}, fmt.Errorf("权限不足: 需要 audit:view 权限")
	}
	limit := req.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := db.WithContext(ctx).Model(&AuditLog{})
	if req.Module != "" {
		query = query.Where("module = ?", req.Module)
	}
	if req.Action != "" {
		query = query.Where("action = ?", req.Action)
	}
	var total int64
	query.Count(&total)
	logs := make([]AuditLog, 0)
	if err := query.Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		return ai.QueryAuditLogsResult{}, fmt.Errorf("查询审计日志失败: %w", err)
	}
	infos := make([]ai.AuditLogInfo, 0, len(logs))
	for _, l := range logs {
		infos = append(infos, ai.AuditLogInfo{
			ID:        l.ID,
			Operator:  l.Operator,
			Module:    l.Module,
			Action:    l.Action,
			Target:    l.Target,
			Success:   l.Success,
			Message:   l.Message,
			IP:        l.IP,
			CreatedAt: l.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return ai.QueryAuditLogsResult{Logs: infos, Total: total}, nil
}

// GetServerResources returns the latest collector snapshot instead of making a second
// expensive host-stat probe for every AI question.
func (s *AIToolService) GetServerResources(ctx agent.ToolContext, operator string) (ai.ServerResourcesResult, error) {
	if !s.hasPermission(ctx, operator, "menu:overview") {
		return ai.ServerResourcesResult{}, fmt.Errorf("权限不足: 需要 menu:overview 权限")
	}

	_, latest := GetSystemStatsHistory()
	if latest == nil {
		return ai.ServerResourcesResult{}, fmt.Errorf("服务器资源采集尚未就绪，请稍后重试")
	}

	if latest.IntervalMs <= 0 || time.Since(time.UnixMilli(latest.Timestamp)) > 3*time.Duration(latest.IntervalMs)*time.Millisecond {
		return ai.ServerResourcesResult{}, fmt.Errorf("\u670d\u52a1\u5668\u8d44\u6e90\u91c7\u96c6\u6570\u636e\u5df2\u8fc7\u671f\uff0c\u8bf7\u7a0d\u540e\u91cd\u8bd5")
	}

	result := ai.ServerResourcesResult{
		CollectedAt:       latest.Timestamp,
		Hostname:          latest.Host.Hostname,
		CPUPercent:        latest.CpuPercent,
		CPUCores:          latest.Host.CpuCores,
		LoadAvg1:          latest.Host.LoadAvg1,
		LoadAvg5:          latest.Host.LoadAvg5,
		LoadAvg15:         latest.Host.LoadAvg15,
		MemoryTotalBytes:  latest.Memory.TotalBytes,
		MemoryUsedBytes:   latest.Memory.UsedBytes,
		MemoryAvailable:   latest.Memory.AvailableBytes,
		MemoryUsedPercent: latest.Memory.UsedPercent,
		SwapUsedPercent:   latest.Memory.SwapPercent,
		NetRxBps:          latest.NetTotalRxBps,
		NetTxBps:          latest.NetTotalTxBps,
		Disks:             make([]ai.ServerResourceDisk, 0, len(latest.Disks)),
	}
	for _, disk := range latest.Disks {
		result.Disks = append(result.Disks, ai.ServerResourceDisk{
			Mountpoint: disk.Mountpoint, TotalBytes: disk.TotalBytes, UsedBytes: disk.UsedBytes,
			FreeBytes: disk.FreeBytes, UsedPercent: disk.UsedPercent,
		})
	}
	return result, nil
}

// GetDashboard 获取系统仪表盘摘要
func (s *AIToolService) GetDashboard(ctx agent.ToolContext, operator string) (ai.GetDashboardResult, error) {
	if !s.hasPermission(ctx, operator, "menu:overview") {
		return ai.GetDashboardResult{}, fmt.Errorf("权限不足: 需要 menu:overview 权限")
	}
	result := ai.GetDashboardResult{
		ClientConfigs: countClientConfigs(),
	}
	db.WithContext(ctx).Model(&User{}).Count(&result.TotalUsers)
	db.WithContext(ctx).Model(&User{}).Where("is_enable = ?", true).Count(&result.EnabledUsers)
	now := time.Now()
	db.WithContext(ctx).Model(&User{}).Where("expire_date <> '' AND expire_date < ?", now.Format("2006-01-02")).Count(&result.ExpiredUsers)
	db.WithContext(ctx).Model(&User{}).Where("expire_date <> '' AND expire_date >= ? AND expire_date <= ?", now.Format("2006-01-02"), now.AddDate(0, 0, 7).Format("2006-01-02")).Count(&result.ExpiringUsers)
	db.WithContext(ctx).Model(&Firewall{}).Count(&result.FirewallRules)

	if s.ov != nil {
		clients, managementOK := s.ov.safeOnlineClients()
		server, _ := s.ov.safeServerData()
		result.OnlineClients = len(clients)
		result.ManagementOK = managementOK
		result.ServerStatus = strings.TrimSpace(server.Status)
		if result.ServerStatus == "" {
			result.ServerStatus = "UNKNOWN"
		}
		if !managementOK {
			result.Risks = append(result.Risks, ai.DashboardRisk{
				Level: "danger", Title: "OpenVPN Management 不可用",
				Message: "无法连接 OpenVPN management 端口，请检查服务状态",
			})
		}
	}
	if result.ExpiredUsers > 0 {
		result.Risks = append(result.Risks, ai.DashboardRisk{
			Level: "warning", Title: "存在已过期用户",
			Message: fmt.Sprintf("有 %d 个用户已过期", result.ExpiredUsers),
		})
	}
	if result.ExpiringUsers > 0 {
		result.Risks = append(result.Risks, ai.DashboardRisk{
			Level: "warning", Title: "用户即将过期",
			Message: fmt.Sprintf("有 %d 个用户将在 7 天内过期", result.ExpiringUsers),
		})
	}
	return result, nil
}

// 编译期断言：确保 AIToolService 实现 ai.ToolService 接口
var _ ai.ToolService = (*AIToolService)(nil)
