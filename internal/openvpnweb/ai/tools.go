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
}

// CreateUserRequest 创建用户工具入参
// 不含 Password 字段：由服务端生成随机密码，避免硬编码和明文泄露
type CreateUserRequest struct {
	Username string `json:"username" jsonschema:"登录用户名，唯一"`
	Name     string `json:"name" jsonschema:"用户姓名"`
	Email    string `json:"email" jsonschema:"邮箱地址（必填）"`
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
			RequireConfirmation: true, // 敏感操作需人工确认
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

	bindRoleTool, err := functiontool.New(
		functiontool.Config{
			Name:                "bind_role",
			Description:         "给指定用户绑定一个角色。需要当前操作者具备 role:assign 权限。当用户要求'给某某用户绑定角色'、'分配角色'等场景时调用。",
			RequireConfirmation: true, // 敏感操作需人工确认
		},
		func(ctx agent.ToolContext, args BindRoleRequest) (BindRoleResult, error) {
			return svc.BindRole(ctx, ctx.UserID(), args)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 bind_role 工具失败: %w", err)
	}

	return []tool.Tool{createUserTool, listUsersTool, bindRoleTool}, nil
}
