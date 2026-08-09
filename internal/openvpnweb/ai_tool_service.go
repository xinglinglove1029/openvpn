package openvpnweb

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

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

	// 响应不返回明文密码，仅提示创建成功（密码由管理员通过重置密码功能下发）
	return ai.CreateUserResult{
		Success: true,
		Message: fmt.Sprintf("用户 %s（%s）创建成功，初始密码已随机生成，请通过重置密码功能下发", req.Name, req.Username),
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

// 编译期断言：确保 AIToolService 实现 ai.ToolService 接口
var _ ai.ToolService = (*AIToolService)(nil)
