package ai

import (
	"fmt"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// ToolService 业务服务接口
// 由 openvpnweb 包实现并注入，避免 ai 包与 openvpnweb 包循环依赖。
// operator 参数为当前登录用户名（来自 ADK session 的 userID），用于权限校验。
type ToolService interface {
	// CreateUser 创建用户（要求当前操作者具备 user:create 权限）
	CreateUser(ctx agent.ToolContext, operator string, req CreateUserRequest) (CreateUserResult, error)
	// ListUsers 列出用户（要求 user:view 权限），支持分页以避免大用户量部署 token 爆炸
	ListUsers(ctx agent.ToolContext, operator string, req ListUsersRequest) (ListUsersResult, error)
	// BindRole 给用户绑定角色（要求 role:assign 权限）
	BindRole(ctx agent.ToolContext, operator string, req BindRoleRequest) (BindRoleResult, error)
	// GetSystemCounts 查询系统用户、客户端配置和在线客户端数量（需要相应查看权限）
	GetSystemCounts(ctx agent.ToolContext, operator string) (SystemCountsResult, error)
	// ResetPassword 重置指定用户的登录密码（要求 user:reset_password 权限）。
	// 若 newPassword 为空，服务端自动生成强密码；否则使用调用方提供的密码。
	// notifyEmail=true 时通过邮件下发新密码（异步，不阻塞返回）。
	ResetPassword(ctx agent.ToolContext, operator string, req ResetPasswordRequest) (ResetPasswordResult, error)
	// ResetMFA 重置指定用户的多因素认证（要求 user:reset_mfa 权限）。
	// 同时重建该用户关联的 .ovpn 配置（移除 MFA challenge）并异步邮件通知。
	ResetMFA(ctx agent.ToolContext, operator string, req ResetMFARequest) (ResetMFAResult, error)
	// GenerateClient 为指定用户生成 .ovpn 客户端配置（要求 client:create 权限）。
	// 若同名客户端已存在则返回错误，不会覆盖现有配置。
	GenerateClient(ctx agent.ToolContext, operator string, req GenerateClientRequest) (GenerateClientResult, error)
	// UpdateUser 更新用户信息（启用/禁用、有效期、固定IP、姓名、邮箱等）。要求 user:update 权限。
	UpdateUser(ctx agent.ToolContext, operator string, req UpdateUserRequest) (UpdateUserResult, error)
	// DeleteUser 删除指定用户。要求 user:delete 权限。
	DeleteUser(ctx agent.ToolContext, operator string, req DeleteUserRequest) (DeleteUserResult, error)
	// ListOnlineClients 列出当前在线的 VPN 客户端。要求 client:view_online 权限。
	ListOnlineClients(ctx agent.ToolContext, operator string) (ListOnlineClientsResult, error)
	// KillConnection 断开指定在线 VPN 连接。要求 client:kill 权限。
	KillConnection(ctx agent.ToolContext, operator string, req KillConnectionRequest) (KillConnectionResult, error)
	// ListClients 列出所有已生成的 .ovpn 客户端配置文件。要求 client:view 权限。
	ListClients(ctx agent.ToolContext, operator string) (ListClientsResult, error)
	// DeleteClient 删除客户端配置并吊销证书。要求 client:delete 权限。
	DeleteClient(ctx agent.ToolContext, operator string, req DeleteClientRequest) (DeleteClientResult, error)
	// UpdateCCD 更新指定客户端的 CCD 推送配置。要求 client:create 权限。
	UpdateCCD(ctx agent.ToolContext, operator string, req UpdateCCDRequest) (UpdateCCDResult, error)
	// RegenerateClient 重新生成指定客户端的 .ovpn 配置文件。要求 client:regenerate 权限。
	RegenerateClient(ctx agent.ToolContext, operator string, req RegenerateClientRequest) (RegenerateClientResult, error)
	// ListFirewallRules 列出防火墙规则。要求 firewall:view 权限。
	ListFirewallRules(ctx agent.ToolContext, operator string) (ListFirewallRulesResult, error)
	// ManageFirewall 管理防火墙规则（添加/删除黑名单、设置/移除限速）。要求相应 firewall 权限。
	ManageFirewall(ctx agent.ToolContext, operator string, req ManageFirewallRequest) (ManageFirewallResult, error)
	// ListCerts 列出 PKI 证书信息（CA、CRL、已签发证书）。要求 cert:view 权限。
	ListCerts(ctx agent.ToolContext, operator string) (ListCertsResult, error)
	// ListChannels 列出通知渠道配置。要求 channel:view 权限。
	ListChannels(ctx agent.ToolContext, operator string) (ListChannelsResult, error)
	// ManageChannel 管理通知渠道（创建/更新/删除/测试）。要求相应 channel 权限。
	ManageChannel(ctx agent.ToolContext, operator string, req ManageChannelRequest) (ManageChannelResult, error)
	// QueryAuditLogs 查询操作审计日志，支持按操作者/模块/时间筛选。要求 audit:view 权限。
	QueryAuditLogs(ctx agent.ToolContext, operator string, req QueryAuditLogsRequest) (QueryAuditLogsResult, error)
	// GetDashboard 获取系统仪表盘摘要（服务器状态、在线数、风险项、趋势）。需要 menu:overview 权限。
	GetDashboard(ctx agent.ToolContext, operator string) (GetDashboardResult, error)
	// GetServerResources returns the latest CPU, memory, disk, network, and load snapshot.
	GetServerResources(ctx agent.ToolContext, operator string) (ServerResourcesResult, error)
}

// CreateUserRequest 创建用户工具入参
// 与页面"新增用户"流程对齐：创建用户 → 自动生成 .ovpn → 发送开通邮件
type CreateUserRequest struct {
	Username         string `json:"username" jsonschema:"登录用户名，唯一"`
	Name             string `json:"name" jsonschema:"用户姓名"`
	Email            string `json:"email" jsonschema:"邮箱地址（必填）"`
	ExpireDate       string `json:"expireDate,omitempty" jsonschema:"有效期，格式 YYYY-MM-DD 或 YYYY-MM-DD HH:MM:SS，可选"`
	IpAddr           string `json:"ipAddr,omitempty" jsonschema:"固定 IP 地址，可选"`
	AutoCreateClient *bool  `json:"autoCreateClient,omitempty" jsonschema:"是否自动生成 OpenVPN 客户端配置，默认 true"`
	SendNotifyEmail  *bool  `json:"sendNotifyEmail,omitempty" jsonschema:"是否发送开通通知邮件（含密码和客户端配置），默认 true"`
}

// CreateUserResult 创建用户工具返回
type CreateUserResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	UserID  uint   `json:"userId,omitempty"`
}

// ListUsersRequest 列出用户工具入参（支持分页，避免大用户量部署 token 爆炸）
type ListUsersRequest struct {
	Limit  int `json:"limit,omitempty" jsonschema:"返回数量上限，默认50，最大200"`
	Offset int `json:"offset,omitempty" jsonschema:"偏移量，默认0"`
}

// ListUsersResult 列出用户工具返回
// Total 为全量用户总数（让 LLM 知道是否还有更多），Users 为当前分页的子集
type ListUsersResult struct {
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
	Users  []UserInfo `json:"users"`
}

// UserInfo 用户简要信息
type UserInfo struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Enabled  bool   `json:"enabled"`
}

// SystemCountsResult 供 AI 回答“当前多少用户、多少客户端”的精简统计结果。
// ClientConfigs 是已生成的 .ovpn 配置数量，OnlineClients 是管理接口当前在线数。
type SystemCountsResult struct {
	TotalUsers    int64 `json:"totalUsers"`
	EnabledUsers  int64 `json:"enabledUsers"`
	ClientConfigs int   `json:"clientConfigs"`
	OnlineClients int   `json:"onlineClients"`
	ManagementOK  bool  `json:"managementOk"`
}

// BindRoleRequest 绑定角色工具入参
type BindRoleRequest struct {
	UserID uint `json:"userId" jsonschema:"用户 ID"`
	RoleID uint `json:"roleId" jsonschema:"角色 ID"`
}

// BindRoleResult 绑定角色工具返回
type BindRoleResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ResetPasswordRequest 重置密码工具入参
// newPassword 为空时由服务端随机生成 14 位强密码（含大小写、数字、特殊字符，满足 isValidPassword 规则）。
// notifyEmail=true 时调用邮件模板下发新密码到用户邮箱（异步）。
type ResetPasswordRequest struct {
	Username    string `json:"username" jsonschema:"目标账号用户名"`
	NewPassword string `json:"newPassword,omitempty" jsonschema:"新密码，可选；留空则服务端生成强密码并通过邮件下发"`
	NotifyEmail bool   `json:"notifyEmail" jsonschema:"是否通过邮件通知用户新密码，默认 true"`
}

// ResetPasswordResult 重置密码工具返回
// 当服务端自动生成密码时，GeneratedPassword 字段会带新密码明文（仅供本次对话回复用户）；
// 调用方提供的密码则不返回，避免泄露。
type ResetPasswordResult struct {
	Success           bool   `json:"success"`
	Message           string `json:"message"`
	UserID            uint   `json:"userId,omitempty"`
	GeneratedPassword string `json:"generatedPassword,omitempty"`
}

// ResetMFARequest 重置 MFA 工具入参
type ResetMFARequest struct {
	Username string `json:"username" jsonschema:"目标账号用户名"`
}

// ResetMFAResult 重置 MFA 工具返回
type ResetMFAResult struct {
	Success      bool     `json:"success"`
	Message      string   `json:"message"`
	UserID       uint     `json:"userId,omitempty"`
	UpdatedFiles []string `json:"updatedFiles,omitempty"`
}

// GenerateClientRequest 生成客户端配置工具入参
// 默认使用系统配置中的服务器地址 / 端口（与"添加用户时自动创建客户端"保持一致）。
type GenerateClientRequest struct {
	Username   string `json:"username" jsonschema:"目标账号用户名"`
	ServerAddr string `json:"serverAddr,omitempty" jsonschema:"服务器地址，可选；留空则从系统配置/站点URL推断"`
	ServerPort string `json:"serverPort,omitempty" jsonschema:"服务器端口，可选；留空则使用 openvpn.ovpn_port（默认 1194）"`
	CCDConfig  string `json:"ccdConfig,omitempty" jsonschema:"CCD 推送配置，可选"`
}

// GenerateClientResult 生成客户端配置工具返回
type GenerateClientResult struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	ClientName string `json:"clientName,omitempty"`
	UserID     uint   `json:"userId,omitempty"`
	OvpnConfig string `json:"ovpnConfig,omitempty"`
}

// UpdateUserRequest 更新用户工具入参
// 只需要传 username + 要修改的字段，未传的字段不会被修改
type UpdateUserRequest struct {
	Username   string `json:"username" jsonschema:"目标账号用户名"`
	Name       string `json:"name,omitempty" jsonschema:"姓名，可选"`
	Email      string `json:"email,omitempty" jsonschema:"邮箱，可选"`
	IsEnable   *bool  `json:"isEnable,omitempty" jsonschema:"启用状态，true 表示启用，false 表示禁用，可选"`
	ExpireDate string `json:"expireDate,omitempty" jsonschema:"有效期，格式 YYYY-MM-DD，传空字符串清除有效期"`
	IpAddr     string `json:"ipAddr,omitempty" jsonschema:"固定 IP，可选"`
}

type UpdateUserResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	UserID  uint   `json:"userId,omitempty"`
}

// DeleteUserRequest 删除用户工具入参
type DeleteUserRequest struct {
	Username string `json:"username" jsonschema:"目标账号用户名"`
}

type DeleteUserResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// OnlineClientInfo 在线客户端信息
type OnlineClientInfo struct {
	CID         string  `json:"cid"`
	RealIP      string  `json:"realIp"`
	VirtualIP   string  `json:"virtualIp"`
	Username    string  `json:"username"`
	CommonName  string  `json:"commonName"`
	RecvBytes   float64 `json:"recvBytes"`
	SendBytes   float64 `json:"sendBytes"`
	ConnDate    string  `json:"connDate"`
	OnlineTime  string  `json:"onlineTime"`
	IsBlacklist bool    `json:"isBlacklist"`
}

type ListOnlineClientsResult struct {
	OnlineClients []OnlineClientInfo `json:"onlineClients"`
	ServerStatus  string             `json:"serverStatus"`
	ManagementOK  bool               `json:"managementOk"`
}

// KillConnectionRequest 断开连接工具入参
type KillConnectionRequest struct {
	CID string `json:"cid" jsonschema:"要断开的连接 ID（可从 list_online_clients 获取）"`
}

type KillConnectionResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ClientConfigInfo 客户端配置文件信息
type ClientConfigInfo struct {
	Name     string `json:"name"`
	FullName string `json:"fullName"`
	Date     string `json:"date"`
}

type ListClientsResult struct {
	Clients []ClientConfigInfo `json:"clients"`
	Total   int                `json:"total"`
}

// DeleteClientRequest 删除客户端工具入参
type DeleteClientRequest struct {
	Name string `json:"name" jsonschema:"客户端名称（即 .ovpn 文件名，不含扩展名）"`
}

type DeleteClientResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// UpdateCCDRequest 更新 CCD 配置工具入参
type UpdateCCDRequest struct {
	Name    string `json:"name" jsonschema:"客户端名称"`
	Content string `json:"content" jsonschema:"CCD 配置内容（OpenVPN client-config-dir 指令格式）"`
}

type UpdateCCDResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// RegenerateClientRequest 重新生成客户端配置工具入参
type RegenerateClientRequest struct {
	Name string `json:"name" jsonschema:"客户端名称"`
}

type RegenerateClientResult struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	ClientName string `json:"clientName,omitempty"`
}

// FirewallRuleInfo 防火墙规则信息
type FirewallRuleInfo struct {
	ID      uint   `json:"id"`
	SIP     string `json:"sip"`
	DIP     string `json:"dip"`
	Policy  string `json:"policy"`
	Status  bool   `json:"status"`
	Comment string `json:"comment"`
}

type ListFirewallRulesResult struct {
	Rules []FirewallRuleInfo `json:"rules"`
	Total int                `json:"total"`
}

// ManageFirewallRequest 防火墙管理工具入参
// action: add_blacklist / remove_blacklist / set_rateLimit / remove_rateLimit
type ManageFirewallRequest struct {
	Action       string `json:"action" jsonschema:"操作类型：add_blacklist(拉黑IP) / remove_blacklist(解除拉黑) / set_rateLimit(设限速) / remove_rateLimit(移除限速)"`
	IP           string `json:"ip" jsonschema:"目标 IP 地址"`
	Upload       string `json:"upload,omitempty" jsonschema:"上传速率，仅 set_rateLimit 需要"`
	UploadUnit   string `json:"uploadUnit,omitempty" jsonschema:"上传单位(mbit/kbit)，仅 set_rateLimit 需要"`
	Download     string `json:"download,omitempty" jsonschema:"下载速率，仅 set_rateLimit 需要"`
	DownloadUnit string `json:"downloadUnit,omitempty" jsonschema:"下载单位(mbit/kbit)，仅 set_rateLimit 需要"`
}

type ManageFirewallResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// CertInfo 证书信息
type CertInfo struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	NotBefore string `json:"notBefore"`
	NotAfter  string `json:"notAfter"`
	Status    string `json:"status"`
	Issuer    string `json:"issuer"`
	Subject   string `json:"subject"`
	Serial    string `json:"serial"`
}

type ListCertsResult struct {
	Certs []CertInfo `json:"certs"`
	Total int        `json:"total"`
}

// ChannelInfo 通知渠道信息
type ChannelInfo struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
}

type ListChannelsResult struct {
	Channels []ChannelInfo `json:"channels"`
	Total    int           `json:"total"`
}

// ManageChannelRequest 通知渠道管理工具入参
// action: create / update / delete / test
type ManageChannelRequest struct {
	Action  string `json:"action" jsonschema:"操作类型：create(创建) / update(更新) / delete(删除) / test(测试发送)"`
	ID      uint   `json:"id,omitempty" jsonschema:"渠道 ID，update/delete/test 时必填"`
	Name    string `json:"name,omitempty" jsonschema:"渠道名称，create/update 时必填"`
	Type    string `json:"type,omitempty" jsonschema:"渠道类型(email/webhook 等)，create 时必填"`
	Enabled *bool  `json:"enabled,omitempty" jsonschema:"是否启用，可选"`
	Config  string `json:"config,omitempty" jsonschema:"渠道配置(JSON 字符串)，create/update 时必填"`
}

type ManageChannelResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	ID      uint   `json:"id,omitempty"`
}

// AuditLogInfo 审计日志信息
type AuditLogInfo struct {
	ID        uint   `json:"id"`
	Operator  string `json:"operator"`
	Module    string `json:"module"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	IP        string `json:"ip"`
	CreatedAt string `json:"createdAt"`
}

type QueryAuditLogsRequest struct {
	Module string `json:"module,omitempty" jsonschema:"按模块筛选(如 user/client/firewall)，可选"`
	Action string `json:"action,omitempty" jsonschema:"按操作类型筛选，可选"`
	Limit  int    `json:"limit,omitempty" jsonschema:"返回数量上限，默认50，最大200"`
}

type QueryAuditLogsResult struct {
	Logs  []AuditLogInfo `json:"logs"`
	Total int64          `json:"total"`
}

// DashboardRisk 仪表盘风险项
type DashboardRisk struct {
	Level   string `json:"level"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

type ServerResourceDisk struct {
	Mountpoint  string  `json:"mountpoint"`
	TotalBytes  uint64  `json:"totalBytes"`
	UsedBytes   uint64  `json:"usedBytes"`
	FreeBytes   uint64  `json:"freeBytes"`
	UsedPercent float64 `json:"usedPercent"`
}

type ServerResourcesResult struct {
	CollectedAt       int64                `json:"collectedAt"`
	Hostname          string               `json:"hostname"`
	CPUPercent        float64              `json:"cpuPercent"`
	CPUCores          int                  `json:"cpuCores"`
	LoadAvg1          float64              `json:"loadAvg1"`
	LoadAvg5          float64              `json:"loadAvg5"`
	LoadAvg15         float64              `json:"loadAvg15"`
	MemoryTotalBytes  uint64               `json:"memoryTotalBytes"`
	MemoryUsedBytes   uint64               `json:"memoryUsedBytes"`
	MemoryAvailable   uint64               `json:"memoryAvailableBytes"`
	MemoryUsedPercent float64              `json:"memoryUsedPercent"`
	SwapUsedPercent   float64              `json:"swapUsedPercent"`
	Disks             []ServerResourceDisk `json:"disks"`
	NetRxBps          float64              `json:"netRxBps"`
	NetTxBps          float64              `json:"netTxBps"`
}

type GetDashboardResult struct {
	TotalUsers    int64           `json:"totalUsers"`
	EnabledUsers  int64           `json:"enabledUsers"`
	OnlineClients int             `json:"onlineClients"`
	ClientConfigs int             `json:"clientConfigs"`
	ExpiredUsers  int64           `json:"expiredUsers"`
	ExpiringUsers int64           `json:"expiringUsers"`
	FirewallRules int64           `json:"firewallRules"`
	ServerStatus  string          `json:"serverStatus"`
	ManagementOK  bool            `json:"managementOk"`
	Risks         []DashboardRisk `json:"risks"`
}

// BuildBusinessTools 构造业务工具集合（create_user / list_users / bind_role）
// svc 由 openvpnweb 包注入；当前操作者用户名通过 ADK ToolContext.UserID() 获取，
// 该值来自 AgentRunner.Run(ctx, userID, ...) 调用时传入的 userID 参数。
func BuildBusinessTools(svc ToolService) ([]tool.Tool, error) {
	if svc == nil {
		return nil, fmt.Errorf("ToolService 不能为空")
	}

	createUserTool, err := functiontool.New(
		functiontool.Config{
			Name:                "create_user",
			Description:         "创建一个新的系统用户。需要当前操作者具备 user:create 权限。当用户要求'新建用户'、'创建账号'等场景时调用。",
			RequireConfirmation: false, // 关闭：DeepSeek-flash 模型对 adk_request_confirmation 协议处理不可靠，会导致工具调用静默失败；当前 AI 在聊天中告知"正在执行XX..."本身就是软确认，用户可随时取消
		},
		func(ctx agent.ToolContext, args CreateUserRequest) (CreateUserResult, error) {
			// operator 来自 ADK session 的 userID（即 AgentRunner.Run 调用时传入的 usernameStr）
			return svc.CreateUser(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 create_user 工具失败: %w", err)
	}

	listUsersTool, err := functiontool.New(
		functiontool.Config{
			Name:        "list_users",
			Description: "列出系统中的用户（支持分页）。需要当前操作者具备 user:view 权限。当用户询问'有哪些用户'、'用户列表'等场景时调用。默认返回前50条，如需更多请指定 offset 翻页。",
		},
		func(ctx agent.ToolContext, args ListUsersRequest) (ListUsersResult, error) {
			return svc.ListUsers(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 list_users 工具失败: %w", err)
	}

	getSystemCountsTool, err := functiontool.New(
		functiontool.Config{
			Name:        "get_system_counts",
			Description: "查询系统中的用户总数、启用用户数、已生成客户端配置数量和当前在线客户端数。当用户询问“多少用户”“多少客户端”“系统概况”时调用。需要 user:view 和 client:view 权限；若无查看在线客户端权限则只返回可安全读取的配置数量。",
		},
		func(ctx agent.ToolContext, _ struct{}) (SystemCountsResult, error) {
			return svc.GetSystemCounts(ctx, ctx.UserID())
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 get_system_counts 工具失败: %w", err)
	}

	bindRoleTool, err := functiontool.New(
		functiontool.Config{
			Name:                "bind_role",
			Description:         "给指定用户绑定一个角色。需要当前操作者具备 role:assign 权限。当用户要求'给某某用户绑定角色'、'分配角色'等场景时调用。",
			RequireConfirmation: false, // 关闭：DeepSeek-flash 模型对 adk_request_confirmation 协议处理不可靠，会导致工具调用静默失败；当前 AI 在聊天中告知"正在执行XX..."本身就是软确认，用户可随时取消
		},
		func(ctx agent.ToolContext, args BindRoleRequest) (BindRoleResult, error) {
			return svc.BindRole(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 bind_role 工具失败: %w", err)
	}

	resetPasswordTool, err := functiontool.New(
		functiontool.Config{
			Name:                "reset_password",
			Description:         "重置指定账号的登录密码。需要 user:reset_password 权限；若未指定 newPassword 则由服务端生成强密码并通过邮件下发。典型场景：用户忘记密码、密码泄露、初次开通。",
			RequireConfirmation: false, // 关闭：DeepSeek-flash 模型对 adk_request_confirmation 协议处理不可靠，会导致工具调用静默失败；当前 AI 在聊天中告知"正在执行XX..."本身就是软确认，用户可随时取消
		},
		func(ctx agent.ToolContext, args ResetPasswordRequest) (ResetPasswordResult, error) {
			return svc.ResetPassword(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 reset_password 工具失败: %w", err)
	}

	resetMfaTool, err := functiontool.New(
		functiontool.Config{
			Name:                "reset_mfa",
			Description:         "重置指定账号的多因素认证（清空 MFA 密钥、关闭 MFA、重新生成关联 .ovpn 配置文件）。需要 user:reset_mfa 权限。典型场景：用户丢失 MFA 设备、需要紧急解除 MFA 验证。",
			RequireConfirmation: false, // 关闭：DeepSeek-flash 模型对 adk_request_confirmation 协议处理不可靠，会导致工具调用静默失败；当前 AI 在聊天中告知"正在执行XX..."本身就是软确认，用户可随时取消
		},
		func(ctx agent.ToolContext, args ResetMFARequest) (ResetMFAResult, error) {
			return svc.ResetMFA(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 reset_mfa 工具失败: %w", err)
	}

	generateClientTool, err := functiontool.New(
		functiontool.Config{
			Name:                "generate_client",
			Description:         "为指定账号生成 OpenVPN 客户端配置（.ovpn 文件并签发客户端证书）。需要 client:create 权限。典型场景：用户已有账号但还没有可用的 VPN 客户端配置、客户端证书遗失需要重签。同名客户端已存在时拒绝覆盖。",
			RequireConfirmation: false, // 关闭：DeepSeek-flash 模型对 adk_request_confirmation 协议处理不可靠，会导致工具调用静默失败；当前 AI 在聊天中告知"正在执行XX..."本身就是软确认，用户可随时取消
		},
		func(ctx agent.ToolContext, args GenerateClientRequest) (GenerateClientResult, error) {
			return svc.GenerateClient(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 generate_client 工具失败: %w", err)
	}

	updateUserTool, err := functiontool.New(
		functiontool.Config{
			Name:        "update_user",
			Description: "更新用户信息（启用/禁用、有效期、固定IP、姓名、邮箱）。需要 user:update 权限。典型场景：禁用用户、设置有效期、修改固定IP。",
		},
		func(ctx agent.ToolContext, args UpdateUserRequest) (UpdateUserResult, error) {
			return svc.UpdateUser(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 update_user 工具失败: %w", err)
	}

	deleteUserTool, err := functiontool.New(
		functiontool.Config{
			Name:        "delete_user",
			Description: "删除指定用户。需要 user:delete 权限。典型场景：离职用户清理、误建账号删除。",
		},
		func(ctx agent.ToolContext, args DeleteUserRequest) (DeleteUserResult, error) {
			return svc.DeleteUser(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 delete_user 工具失败: %w", err)
	}

	listOnlineClientsTool, err := functiontool.New(
		functiontool.Config{
			Name:        "list_online_clients",
			Description: "列出当前在线的 VPN 客户端（含真实IP、虚拟IP、流量、在线时长）。需要 client:view_online 权限。当用户询问'谁在线'、'在线客户端'时调用。",
		},
		func(ctx agent.ToolContext, _ struct{}) (ListOnlineClientsResult, error) {
			return svc.ListOnlineClients(ctx, ctx.UserID())
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 list_online_clients 工具失败: %w", err)
	}

	killConnectionTool, err := functiontool.New(
		functiontool.Config{
			Name:        "kill_connection",
			Description: "断开指定在线 VPN 连接。需要 client:kill 权限。典型场景：强制用户下线、处理异常连接。",
		},
		func(ctx agent.ToolContext, args KillConnectionRequest) (KillConnectionResult, error) {
			return svc.KillConnection(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 kill_connection 工具失败: %w", err)
	}

	listClientsTool, err := functiontool.New(
		functiontool.Config{
			Name:        "list_clients",
			Description: "列出所有已生成的 .ovpn 客户端配置文件。需要 client:view 权限。当用户询问'有哪些客户端配置'、'客户端列表'时调用。",
		},
		func(ctx agent.ToolContext, _ struct{}) (ListClientsResult, error) {
			return svc.ListClients(ctx, ctx.UserID())
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 list_clients 工具失败: %w", err)
	}

	deleteClientTool, err := functiontool.New(
		functiontool.Config{
			Name:        "delete_client",
			Description: "删除客户端配置并吊销对应证书。需要 client:delete 权限。典型场景：用户离职清理、证书过期重签前删除旧配置。",
		},
		func(ctx agent.ToolContext, args DeleteClientRequest) (DeleteClientResult, error) {
			return svc.DeleteClient(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 delete_client 工具失败: %w", err)
	}

	updateCCDTool, err := functiontool.New(
		functiontool.Config{
			Name:        "update_ccd",
			Description: "更新指定客户端的 CCD（Client Config Dir）推送配置。需要 client:create 权限。典型场景：设置固定IP(ifconfig-push)、推送路由(push-route)。",
		},
		func(ctx agent.ToolContext, args UpdateCCDRequest) (UpdateCCDResult, error) {
			return svc.UpdateCCD(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 update_ccd 工具失败: %w", err)
	}

	regenerateClientTool, err := functiontool.New(
		functiontool.Config{
			Name:        "regenerate_client",
			Description: "重新生成指定客户端的 .ovpn 配置文件（覆盖现有配置）。需要 client:regenerate 权限。典型场景：MFA状态变更后重签、服务器地址变更后更新配置。",
		},
		func(ctx agent.ToolContext, args RegenerateClientRequest) (RegenerateClientResult, error) {
			return svc.RegenerateClient(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 regenerate_client 工具失败: %w", err)
	}

	listFirewallTool, err := functiontool.New(
		functiontool.Config{
			Name:        "list_firewall_rules",
			Description: "列出防火墙规则。需要 firewall:view 权限。当用户询问'防火墙规则'、'有哪些限制'时调用。",
		},
		func(ctx agent.ToolContext, _ struct{}) (ListFirewallRulesResult, error) {
			return svc.ListFirewallRules(ctx, ctx.UserID())
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 list_firewall_rules 工具失败: %w", err)
	}

	manageFirewallTool, err := functiontool.New(
		functiontool.Config{
			Name:        "manage_firewall",
			Description: "管理防火墙规则（拉黑/解黑IP、设置/移除限速）。需要 firewall:create 或 firewall:delete 权限。action=add_blacklist 拉黑IP，remove_blacklist 解除，set_rateLimit 设限速，remove_rateLimit 移除限速。",
		},
		func(ctx agent.ToolContext, args ManageFirewallRequest) (ManageFirewallResult, error) {
			return svc.ManageFirewall(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 manage_firewall 工具失败: %w", err)
	}

	listCertsTool, err := functiontool.New(
		functiontool.Config{
			Name:        "list_certs",
			Description: "列出 PKI 证书信息（CA证书、CRL吊销列表、已签发的客户端证书）。需要 cert:view 权限。当用户询问'证书状态'、'证书过期'时调用。",
		},
		func(ctx agent.ToolContext, _ struct{}) (ListCertsResult, error) {
			return svc.ListCerts(ctx, ctx.UserID())
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 list_certs 工具失败: %w", err)
	}

	listChannelsTool, err := functiontool.New(
		functiontool.Config{
			Name:        "list_channels",
			Description: "列出通知渠道配置（邮件、Webhook等）。需要 channel:view 权限。当用户询问'通知渠道'、'邮件配置'时调用。",
		},
		func(ctx agent.ToolContext, _ struct{}) (ListChannelsResult, error) {
			return svc.ListChannels(ctx, ctx.UserID())
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 list_channels 工具失败: %w", err)
	}

	manageChannelTool, err := functiontool.New(
		functiontool.Config{
			Name:        "manage_channel",
			Description: "管理通知渠道（创建/更新/删除/测试发送）。需要 channel:create/update/delete/test 权限。action=create 创建，update 更新，delete 删除，test 测试发送。",
		},
		func(ctx agent.ToolContext, args ManageChannelRequest) (ManageChannelResult, error) {
			return svc.ManageChannel(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 manage_channel 工具失败: %w", err)
	}

	queryAuditLogsTool, err := functiontool.New(
		functiontool.Config{
			Name:        "query_audit_logs",
			Description: "查询操作审计日志，支持按模块和操作类型筛选。需要 audit:view 权限。当用户询问'操作记录'、'审计日志'、'谁做了什么'时调用。返回最近的操作记录。",
		},
		func(ctx agent.ToolContext, args QueryAuditLogsRequest) (QueryAuditLogsResult, error) {
			return svc.QueryAuditLogs(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 query_audit_logs 工具失败: %w", err)
	}

	getDashboardTool, err := functiontool.New(
		functiontool.Config{
			Name:        "get_dashboard",
			Description: "获取系统仪表盘摘要（服务器状态、用户统计、在线数、过期/即将过期用户、防火墙规则数、风险项）。需要 menu:overview 权限。当用户询问'系统概况'、'有什么风险'、'服务器状态'时调用。",
		},
		func(ctx agent.ToolContext, _ struct{}) (GetDashboardResult, error) {
			return svc.GetDashboard(ctx, ctx.UserID())
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 get_dashboard 工具失败: %w", err)
	}

	getServerResourcesTool, err := functiontool.New(
		functiontool.Config{
			Name:        "get_server_resources",
			Description: "获取服务器实时资源状态，包括 CPU、内存、磁盘、网络速率和系统负载。需要 menu:overview 权限。当用户询问‘服务器资源’、‘CPU/内存/磁盘使用率’或‘机器负载’时调用。",
		},
		func(ctx agent.ToolContext, _ struct{}) (ServerResourcesResult, error) {
			return svc.GetServerResources(ctx, ctx.UserID())
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 get_server_resources 工具失败: %w", err)
	}

	return []tool.Tool{
		createUserTool,
		listUsersTool,
		getSystemCountsTool,
		bindRoleTool,
		resetPasswordTool,
		resetMfaTool,
		generateClientTool,
		updateUserTool,
		deleteUserTool,
		listOnlineClientsTool,
		killConnectionTool,
		listClientsTool,
		deleteClientTool,
		updateCCDTool,
		regenerateClientTool,
		listFirewallTool,
		manageFirewallTool,
		listCertsTool,
		listChannelsTool,
		manageChannelTool,
		queryAuditLogsTool,
		getDashboardTool,
		getServerResourcesTool,
	}, nil
}
