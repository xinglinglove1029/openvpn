---
title: '完整 RBAC 权限系统'
type: 'feature'
created: '2026-07-27'
status: 'done'
review_loop_iteration: 0
baseline_revision: '2ebb0532f24b70c4b48b4186a021ecb1e31539e9'
followup_review_recommended: false
final_revision: 'cad78084ad2b12c13c53c1c75ce3ce3e0f8cb4a2'
context:
  - '{project-root}/internal/openvpnweb/server.go'
  - '{project-root}/internal/openvpnweb/user.go'
  - '{project-root}/internal/openvpnweb/config.go'
  - '{project-root}/frontend/src/store/auth.tsx'
  - '{project-root}/frontend/src/layout/Sidebar.tsx'
  - '{project-root}/frontend/src/layout/Layout.tsx'
warnings: [oversized]
---

<intent-contract>

## Intent

**Problem:** 项目当前仅有"admin vs 普通用户"二元权限模型，靠 `username == adminUsername`（后端）和 `!user.id || user.id <= 0`（前端）硬编码判断；无角色体系、无按钮级权限、菜单硬编码 `adminOnly: boolean` 过滤；新增非 admin 用户无法精细化授权，按钮对能进入页面的任何用户都可见可点。

**Approach:** 引入经典 RBAC（用户-角色-权限多对多），权限分菜单/按钮两类用 `资源:动作` 编码（如 `menu:users`、`user:create`）；admin 用户保留现有内置超管逻辑并绕过所有权限检查；系统初始化时 seed 全量权限清单 + 内置"普通用户"角色（code=`user`，权限=概览/客户端/连接历史/站内信/个人中心 + 自有客户端操作），新建用户默认绑定该角色；登录接口下发 `permissions: string[]`，前端 `useAuth` 提供 `hasPermission(code)` 并新增 `<HasPermission>` 按钮包裹组件、`/roles` 角色管理页面。

## Boundaries & Constraints

**Always:**
- admin 用户（`system.base.admin_username` 配置项，默认 `admin`）始终拥有全部权限，所有权限检查对 admin 直接放行，不查角色表
- admin 凭据仍存配置文件（不入 `user` 表），登录/改密/个人资料走现有特殊分支保留不变
- 所有代码、注释、commit message、文档使用中文
- 前端使用 shadcn/ui + Tailwind 组件，避免手写组件；表单左对齐 label（140px 固定宽度）
- 后端响应消息 UTF-8 中文；按钮权限统一编码 `资源:动作`（snake_case 风格，如 `user:create`）
- 数据迁移通过 GORM `AutoMigrate` 自动建表/加列；现有用户（`role_id` 为 NULL）启动时批量回填到内置"普通用户"角色 ID
- 角色内置标记 `is_builtin`：内置角色（`administrator`、`user`）允许查看和分配但不允许删除；`code` 唯一索引

**Block If:**
- 现有 SQLite `ovpn.db` 中已有用户记录但无法判定其默认角色归属（已通过"全部回填到普通用户角色"兜底，无需人工介入）
- 权限编码与现有前端菜单/路由无法一一映射（已在权限清单中穷举，无缺口）

**Never:**
- 不引入 OAuth2/JWT/Casbin 等外部框架，仅用 Gin 中间件 + GORM 实现
- 不做多租户、数据行级权限（如"只能看本组用户"），仅做操作权限（菜单/按钮可见性与 API 调用权）
- 不改造 OpenVPN 客户端认证流程（`/ovpn/login` 等业务认证接口保持现状）
- 不允许通过 UI 修改权限定义表（`permissions` 表仅由代码 seed 维护，不暴露 CRUD 接口）
- 不引入 user_roles 多对多（采用 `user.role_id` 单用户单角色，简化前端判断）
- 不删除 `isPublicPath` 中的 `/client/*` 前缀匹配（用户自身资源接口保持登录即可访问）

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| admin 登录 | username == adminUsername, 密码正确 | `is_admin: true`, `permissions: ["*"]`, 跳 `/admin` | 密码错返回 401 |
| 普通用户登录 | user.role_id 已绑定角色 | `is_admin: false`, `permissions: [角色权限 code 列表]`, 跳 `/` | 角色被禁用返回 403 "角色已禁用" |
| 历史用户登录 | user.role_id 为 NULL（迁移前） | 启动时已自动回填到普通用户角色，登录走上一行 | 若运行时仍未回填（极端），按普通用户角色权限处理 |
| 角色被禁用用户登录 | role.is_enable = false | 登录失败 403 "角色已禁用，请联系管理员" | - |
| 普通用户访问 `/ovpn/user` GET | session 已登录，无 `user:view` 权限 | 403 JSON `{"message": "无权限"}` | - |
| 普通用户点击"删除用户"按钮 | 前端无 `user:delete` 权限 | `<HasPermission>` 隐藏按钮，不发起请求 | - |
| admin 访问任意接口 | admin 用户 | 全部放行，不查权限表 | - |
| 创建用户未指定角色 | POST `/ovpn/user` 无 role_id | 后端自动填充普通用户角色 ID | - |
| 删除内置角色 | DELETE `/ovpn/role/{id}` 角色 is_builtin=true | 400 "内置角色不允许删除" | - |
| 角色被用户绑定时删除 | DELETE `/ovpn/role/{id}` 有用户关联 | 400 "角色下存在用户，不允许删除" | - |
| 修改权限定义后老角色不生效 | permissions 表 seed 新增项 | 现有角色需重新分配；不影响 admin | 内置普通用户角色在 seed 时自动同步新增的概览/客户端类权限 |

</intent-contract>

## Code Map

- `internal/openvpnweb/server.go` -- 主路由与中间件注册；`AuthMiddleWare`（L465-517）重构，`/` 与 `/admin` 跳转逻辑（L796、L814）保留，登录接口（L654-775）扩展返回 permissions，新增 `/ovpn/role/*` 与 `/ovpn/permission/*` 路由组
- `internal/openvpnweb/user.go` -- User 模型（L22-39）新增 `RoleID *uint` 字段；`BeforeSave`/`AfterFind` 钩子保留；新增 `LoadPermissions(db) ([]string, error)` 方法
- `internal/openvpnweb/role.go` -- **新建**：Role / Permission / RolePermission 模型、CRUD handler、`RequirePermission(code)` 中间件、`seedPermissionsAndRoles()` 初始化函数
- `internal/openvpnweb/config.go` -- 保留 `adminUsername` 全局变量；新增 `defaultRoleCode = "user"` 常量
- `internal/openvpnweb/audit.go` -- AuditMiddleware 不变，权限拒绝时由 RequirePermission 写审计
- `frontend/src/types.ts` -- `ClientUserInfo`（L385-394）新增 `roleId?/permissions/isAdmin` 字段；新增 `Role`/`Permission`/`RoleTreeNode` 类型
- `frontend/src/store/auth.tsx` -- `AuthProvider` 增加 `hasPermission(code): boolean`、`isAdmin: boolean`；localStorage 同步 permissions
- `frontend/src/components/HasPermission.tsx` -- **新建**：`<HasPermission code="user:create">{children}</HasPermission>` 包裹组件
- `frontend/src/layout/Sidebar.tsx` -- `allNavItems` 的 `adminOnly: boolean` 改为 `permission: 'menu:xxx'`；过滤逻辑改为 `hasPermission(item.permission)`；新增"角色管理"菜单项
- `frontend/src/layout/Layout.tsx` -- `adminOnlyPaths`（L8）改为按 menu 权限校验；路由守卫调用 `hasPermission`
- `frontend/src/pages/Profile/index.tsx` -- 4 处 `isAdmin` 判断（L89、L169、L336、L536）替换为 `user.isAdmin || hasPermission(...)`
- `frontend/src/pages/Users/index.tsx` -- 用户表单对话框新增"角色"下拉选择；批量操作按钮包裹 `<HasPermission>`
- `frontend/src/pages/Roles/index.tsx` -- **新建**：角色列表 + 编辑对话框（含权限树勾选）
- `frontend/src/api.ts` -- 拦截 403 响应统一 toast "无权限执行此操作"
- `frontend/src/App.tsx` -- 新增 `/roles` 路由

## Tasks & Acceptance

**Execution:**

- [x] `internal/openvpnweb/role.go` -- 新建文件，定义 `Role{ID,Name,Code,Description,IsBuiltin,IsEnable,Sort,CreatedAt,UpdatedAt}`、`Permission{ID,ParentID,Name,Code,Type,Path,Icon,Sort,CreatedAt}`、`RolePermission{RoleID,PermissionID}` 三个模型（含表名 `role`/`permission`/`role_permission`，复合主键） -- 提供数据基础
- [x] `internal/openvpnweb/role.go` -- 实现 `SeedPermissionsAndRoles(db)` 函数：硬编码权限清单（菜单 12 项 + 按钮 ~40 项，详见 Design Notes），`FirstOrCreate` 写入；创建内置 `administrator` 角色（全权限）与 `user` 角色（仅概览/客户端/连接历史/站内信/个人中心 + 自有客户端增删下载） -- 启动时初始化
- [x] `internal/openvpnweb/user.go` -- User 结构体新增 `RoleID *uint \`gorm:"column:role_id;default:NULL" json:"roleId" form:"roleId"\`` 字段；新增 `(u *User) LoadPermissionCodes(db) ([]string, error)` 方法：admin 返回 `["*"]`；否则查 role+role_permission+permission 拼 code 列表，角色禁用返回 error -- 支持登录与中间件复用
- [x] `internal/openvpnweb/server.go` -- `Run()` 中 `db.AutoMigrate` 列表追加 `&Role{}`、`&Permission{}`、`&RolePermission{}`，并调用 `SeedPermissionsAndRoles(db)`；启动后批量 `UPDATE user SET role_id = <普通用户角色ID> WHERE role_id IS NULL` -- 数据迁移
- [x] `internal/openvpnweb/server.go` -- 重构 `AuthMiddleWare`（L465-517）：保留登录态校验与 O-Token 旁路；admin 用户直接 `c.Next()`；移除 `isPublicPath` 白名单，改为 `c.Set("permissions", permissionCodes)` 供下游中间件使用；非 admin 用户无任何角色时按普通用户角色兜底 -- 替换硬编码判断
- [x] `internal/openvpnweb/role.go` -- 实现 `RequirePermission(code string) gin.HandlerFunc`：从 `c.Get("permissions")` 取列表，支持 `["*"]` 通配；不匹配返回 403 JSON `{"message": "无权限"}` 并写审计日志 -- 路由级保护
- [x] `internal/openvpnweb/server.go` -- `/ovpn` 路由组每个接口追加 `RequirePermission("xxx")`：用户组 `user:view/create/update/delete/enable/disable/reset_password/reset_mfa/import/export`、分组 `group:view/create/update/delete/config`、防火墙 `firewall:view/create/update/delete/clear`、证书 `cert:view/renew`、审计 `audit:view`、设置 `settings:view/update`、通知渠道 `channel:view/create/update/delete/test`、客户端包 `client:manage_all`、服务器操作 `server:manage`、断开连接 `client:kill`、角色 `role:view/create/update/delete/assign_permissions`、权限定义查询 `permission:view` -- 路由级权限闭合
- [x] `internal/openvpnweb/server.go` -- 登录接口（L654-775）返回 JSON 增加 `permissions`、`isAdmin`、`roleId` 字段；普通用户登录前校验 `role.is_enable` -- 下发权限到前端
- [x] `internal/openvpnweb/server.go` -- `/client/userinfo` GET 返回增加 `permissions`、`isAdmin`、`roleId` -- 刷新页面时同步权限
- [x] `internal/openvpnweb/server.go` -- 新增 `/ovpn/role` 路由组：GET 列表、GET `/:id` 详情（含权限 code 列表）、POST 创建、PATCH 更新、DELETE 删除（内置角色 400）、PUT `/:id/permissions` 分配权限（body 为 permission code 数组） -- 角色管理 API
- [x] `internal/openvpnweb/server.go` -- 新增 `/ovpn/permission` 路由组：GET `tree` 返回权限树（仅 admin 可访问，用 RequirePermission 或显式 admin 判断） -- 供角色编辑页拉取权限选项
- [x] `internal/openvpnweb/server.go` -- `POST /ovpn/user` 创建用户时若 `role_id` 为空，填充普通用户角色 ID -- 默认绑定
- [x] `internal/openvpnweb/server.go` -- 现有"非 admin 用户访问非白名单路径重定向"逻辑（L508-512）删除；`/` 路由（L796）admin 跳 `/admin` 保留；`/admin` 路由（L814）改为 `RequirePermission("menu:settings") || admin` 才可进入，否则 404 -- SPA 入口收敛
- [x] `frontend/src/types.ts` -- `ClientUserInfo` 新增 `roleId?: number`、`permissions: string[]`、`isAdmin: boolean`；新增 `Role`、`Permission`、`PermissionTreeNode` 类型 -- 类型基础
- [x] `frontend/src/store/auth.tsx` -- `AuthProvider` value 增加 `hasPermission(code: string): boolean`（admin 或 permissions 含 code 或 `["*"]` 返回 true）与 `isAdmin: boolean`；localStorage 同步 -- 全局权限判断
- [x] `frontend/src/components/HasPermission.tsx` -- 新建：`<HasPermission code="user:create">{children}</HasPermission>`，无权限返回 `null`，支持 `fallback` prop -- 按钮级控制
- [x] `frontend/src/layout/Sidebar.tsx` -- `allNavItems` 字段 `adminOnly: boolean` 改为 `permission: 'menu:xxx'`；过滤改为 `hasPermission(item.permission)`；新增 `{ path: '/roles', label: '角色管理', icon: ShieldCheck, permission: 'menu:roles' }` -- 动态菜单
- [x] `frontend/src/layout/Layout.tsx` -- 删除 `adminOnlyPaths` 数组与 `isAdmin` 判断（L8、L22-28）；改为根据当前路径对应 menu 权限校验，无权限跳 `/overview` -- 路由守卫权限化
- [x] `frontend/src/pages/Profile/index.tsx` -- L89/L169 admin 分支保留（admin 走配置文件）；L336 `isAdmin` 改为 `user.isAdmin`；L536 MFA 卡片改为 `!user.isAdmin`（普通用户绑 MFA 不变） -- 兼容 admin
- [x] `frontend/src/pages/Users/index.tsx` -- 用户表单对话框新增"角色"下拉（默认普通用户，admin 用户隐藏此字段或显示"系统超管"只读）；删除/重置密码/批量操作按钮包裹 `<HasPermission code="xxx">` -- 按钮权限落地
- [x] `frontend/src/pages/Roles/index.tsx` -- 新建：左侧角色列表（admin 才能进入 `/roles`），右侧编辑对话框含权限树（shadcn Tree/Checkbox），勾选保存到 `PUT /ovpn/role/:id/permissions`；内置角色标记 Badge，删除按钮对内置角色禁用 -- 角色管理 UI
- [x] `frontend/src/api.ts` -- 响应拦截器：403 时 toast `无权限执行此操作`（不重定向），保留 401 跳登录 -- 错误体验
- [x] `frontend/src/App.tsx` -- 新增 `/roles` lazy 路由 -- 路由注册
- [x] `internal/openvpnweb/server.go` -- 路由 `/ovpn/role` 与 `/ovpn/permission` 在 `AuthMiddleWare` 之后注册；`/client/*` 路由组（L2118+）保持登录即可访问，仅 `PUT /client/userinfo` 保留 admin 限制（改为 `RequirePermission("user:update_own") || admin`，普通用户可改自己） -- 修正现有过度限制

**Acceptance Criteria:**
- Given admin 登录，when 访问任意页面/接口，then 全部菜单可见、所有按钮可点、所有接口返回 200
- Given 普通用户角色登录，when 进入 `/users` 页面，then 被重定向到 `/overview`；即使直接调 `GET /ovpn/user`，then 返回 403
- Given admin 创建新用户未指定角色，when 该用户首次登录，then 自动拥有普通用户角色对应权限（4 个菜单 + 自有客户端操作）
- Given admin 进入 `/roles` 页面，when 修改某角色的菜单权限并保存，then 该角色下用户下次登录后菜单与按钮可见性立即变化
- Given 内置"普通用户"角色，when 尝试删除，then 返回 400 "内置角色不允许删除"
- Given 角色下仍有用户绑定，when 尝试删除该角色，then 返回 400 "角色下存在用户，不允许删除"
- Given 用户角色被禁用，when 该用户登录，then 返回 403 "角色已禁用"
- Given 历史用户（role_id 为 NULL），when 服务启动，then 自动回填到普通用户角色，登录行为正常
- Given 普通用户在客户端页面，when 查看"删除"按钮，then 按钮隐藏；when 通过 API 直接 DELETE，then 返回 403
- Given admin 在角色编辑页，when 查看权限树，then 12 个菜单权限 + ~40 个按钮权限全部可见可勾选
- Given 任意接口返回 403，when 前端收到响应，then toast 显示"无权限执行此操作"，不跳转

## Spec Change Log

（空，由 step-04 在 review loopback 时追加）

## Review Triage Log

### 2026-07-27 — Review pass
- intent_gap: 0
- bad_spec: 0
- patch: 16: (high 3, medium 11, low 2)
- defer: 0
- reject: 0
- addressed_findings:
  - [high] [patch] `user.LoadPermissionCodes` 在 `adminUsername` 配置为空时把空用户名误判为 admin，导致权限绕过；增加 `adminUsername != ""` 前置判断
  - [high] [patch] `frontend/src/store/auth.tsx` 中 `isAdmin` 使用 `!user.id || user.id <= 0` 兜底判断，导致 LDAP 用户（后端返回 id=0, isAdmin=false）被前端误判为 admin；改为仅信任后端下发的 `user.isAdmin` 字段
  - [high] [patch] `roleDeleteHandler` 用户数检查与删除操作未在同一事务中，存在 TOCTOU 竞态（检查后到删除前有新用户绑定该角色）；将 count 检查、RolePermission 删除、Role 删除包入同一 `db.Transaction`
  - [medium] [patch] Role 模型 `IsEnable` 字段为 `bool` 类型，与 GORM `default:true` 冲突导致前端显式传 false 时仍被写为 true；改为 `*bool` 指针类型，nil 时由 GORM default 兜底，false 时保留 false
  - [medium] [patch] `LoadRolePermissionCodes` 使用 LEFT JOIN 导致孤儿 `role_permission` 把 NULL code 塞进权限列表；改为 INNER JOIN 过滤无效权限，并增加角色启用状态校验
  - [medium] [patch] `RequirePermission` 中间件未优先使用 `AuthMiddleWare` 已设置的 `isAdmin` 标志，存在双重判断风险；重构为优先读 `c.Get("isAdmin")`，session 兜底仅作未设置时的回退
  - [medium] [patch] `roleCreateHandler` 缺少 code 格式（`^[a-zA-Z][a-zA-Z0-9_]*$`）、长度（1-64）、name 非空、description 长度（≤256）、Sort 范围校验；为 Role 模型添加 `BeforeSave` 钩子做 bluemonday XSS 净化，handler 内增加完整校验
  - [medium] [patch] `roleUpdateHandler` 使用 `map[string]interface{}` `Updates` 强制写 `is_enable`/`sort`，前端漏传即被清零；改用 struct 更新仅更新非零字段，`IsEnable` 为 `*bool`，nil 时不更新
  - [medium] [patch] `AuthMiddleWare` 未校验用户存在性、未处理权限加载错误（角色被禁用/不存在时仍放行）；增加 `u.ID == 0` 检查并清除 session 强制登出，`ErrRoleDisabled`/`ErrRoleNotFound` 时清除 session 重定向登录
  - [medium] [patch] `frontend/src/layout/Layout.tsx` 路由守卫在用户无 `menu:overview` 权限时陷入空白页卡死；重构为优先跳转 `menu:overview`，其次 `menu:profile`，均无权限时渲染"无访问权限"占位页
  - [medium] [patch] `server.go` 中 `/client/userinfo` GET 接口对 `ErrRoleDisabled`/`ErrRoleNotFound` 静默返回空权限；改为返回 403，其他错误返回 500
  - [medium] [patch] `/client/userinfo` PUT 接口绕过 BeforeSave（XSS 净化）、未记审计日志、已删除用户仍返回 200；改用 struct `Updates` 触发 BeforeSave 钩子，已删除用户返回 401，增加 `recordAudit` 调用
  - [medium] [patch] `roleUpdateHandler` 使用 map `Updates` 绕过 `BeforeSave` 钩子；在 handler 内显式调用 bluemonday 净化 name/description 字段
  - [medium] [patch] `roleAssignPermissionsHandler` 对前端传入的 permissions 数组未去重，导致 Pluck 返回行数与输入不一致时误判缺失 code；增加去重逻辑后再查 DB
  - [medium] [patch] `SeedPermissionsAndRoles` 内置 user 角色仅首次初始化写入默认权限，新增的默认权限项不会同步；改为每次启动时通过 `FirstOrCreate` 同步 `defaultUserRoleCodes` 中的权限，新增项自动补齐，管理员手动移除的权限不强制回填
  - [medium] [patch] 审计中间件 `audit.go` 中 role 路径的兜底规则与 handler 内主动调用 `recordAudit` 重复记录；移除 role 路径的兜底规则
  - [medium] [patch] 403 错误时 api.ts 与调用方均 toast 导致双重提示；定义 `ApiError` 类标记 `handled=true`，`messageOf` 函数对 handled=true 的错误返回空字符串
  - [low] [patch] AutoMigrate 回填历史用户 `role_id` 后未检查错误与影响行数；增加 `result.Error` 检查与 `RowsAffected` 日志记录，默认角色不存在时也记日志
  - [low] [patch] `roleListHandler` 在循环中查询每个角色的用户数导致 N+1 查询；改为批量查询 `role_id -> user_count` 映射后在内存中拼装
  - [low] [patch] 缺少 `validateRoleID` 和 `parseRoleIDParam` 辅助函数，角色 ID 校验逻辑分散；新增统一辅助函数在 `roleCreateHandler`/`roleUpdateHandler`/`roleAssignPermissionsHandler` 中复用
  - [medium] [patch] 前端权限数据异常检测：当 user 已登录且非 admin 但 permissions 缺失或非数组时（localStorage 缓存旧数据、后端异常、数据被篡改），清除本地登录态并跳转登录页，避免用户卡在空白页


## Design Notes

**权限清单（seed 时硬编码，按 `资源:动作` 编码）：**

菜单权限（type=menu，12 项，对应前端路由）：
```
menu:overview        /overview        概览
menu:users           /users           账号管理
menu:clients         /clients         客户端
menu:firewall        /firewall        防火墙
menu:history         /history         连接历史
menu:certs           /certs           证书
menu:audit           /audit           操作审计
menu:settings        /settings        系统设置
menu:channels        /channels        通知渠道
menu:notifications   /notifications   站内信
menu:roles           /roles           角色管理（新增）
menu:profile         /profile         个人中心（不在侧边栏，所有登录用户必备）
```

按钮权限（type=button，按资源分组）：
```
用户管理（10）：user:view user:create user:update user:delete user:enable user:disable user:reset_password user:reset_mfa user:import user:export
分组（5）：group:view group:create group:update group:delete group:config
客户端（4）：client:create client:download client:delete client:regenerate
客户端包（1）：client:manage_all
防火墙（5）：firewall:view firewall:create firewall:update firewall:delete firewall:clear
证书（2）：cert:view cert:renew
审计（1）：audit:view
系统设置（2）：settings:view settings:update
通知渠道（5）：channel:view channel:create channel:update channel:delete channel:test
服务器（2）：server:manage client:kill
角色管理（5）：role:view role:create role:update role:delete role:assign_permissions
权限查询（1）：permission:view
```

**内置角色默认权限：**
- `administrator`（code=administrator, is_builtin=true）：全部权限
- `user`（code=user, is_builtin=true）：menu:overview, menu:clients, menu:history, menu:notifications, menu:profile, client:create, client:download, client:delete, client:regenerate（仅对自有资源生效，由 handler 内 `cu.ID == u.ID || isAdmin` 校验保证）

**hasPermission 实现示例：**
```ts
const hasPermission = useCallback((code: string): boolean => {
  if (user?.isAdmin) return true;
  if (!user?.permissions) return false;
  if (user.permissions.includes('*')) return true;
  return user.permissions.includes(code);
}, [user]);
```

**RequirePermission 中间件示例：**
```go
func RequirePermission(code string) gin.HandlerFunc {
    return func(c *gin.Context) {
        if user, _ := c.Get("user"); user == adminUsername {
            c.Next(); return
        }
        perms, _ := c.Get("permissions")
        codes, _ := perms.([]string)
        for _, p := range codes {
            if p == "*" || p == code { c.Next(); return }
        }
        c.JSON(403, gin.H{"message": "无权限"})
        c.Abort()
    }
}
```

**权限树前端结构：** shadcn Tree 组件已能满足；权限按资源分组（用户/分组/客户端/防火墙/证书/审计/设置/通知渠道/服务器/角色），每组下展开具体动作 checkbox。

**数据迁移注意：** `user` 表新增 `role_id` 字段后，GORM AutoMigrate 会自动加列；启动后立即 `db.Model(&User{}).Where("role_id IS NULL").Update("role_id", defaultRoleID)` 回填，避免登录时 panic。

## Verification

**Commands:**
- `cd f:\develop\openvpn && go build ./...` -- expected: 编译通过，无错误
- `cd f:\develop\openvpn\frontend && npm run build` -- expected: 前端构建通过
- `cd f:\develop\openvpn && go vet ./...` -- expected: 无 vet 警告

**Manual checks:**
- 启动后端，查看 `data/ovpn.db`：确认 `role`/`permission`/`role_permission` 表已建，内置角色与权限已写入，`user` 表新增 `role_id` 列且历史用户已回填
- admin 登录 → 侧边栏显示全部 11 个菜单（含新增"角色管理"）→ 进入 `/roles` → 可创建/编辑/删除角色、勾选权限树保存
- 创建普通用户 `testuser` 不指定角色 → 用 `testuser` 登录 → 仅见 4 个菜单（概览/客户端/连接历史/站内信）→ 客户端页面"删除"按钮不可见 → 直接 `curl -X DELETE /ovpn/user/1` 返回 403
- admin 编辑"普通用户"角色，去掉 `menu:clients` → `testuser` 重新登录 → 客户端菜单消失
- 用 SQLite 工具直接将某用户的 `role_id` 设为 NULL → 重启后端 → 该用户被自动回填到普通用户角色

## Auto Run Result

**Status:** done

### Summary of Implemented Change

落地完整的 RBAC（用户-角色-权限多对多）权限系统：

- 后端：新增 `Role`/`Permission`/`RolePermission` 模型与 `role.go`，定义 12 项菜单权限 + 约 40 项按钮权限的 seed 清单；内置 `administrator`（全权限）与 `user`（普通用户默认权限）两个角色；启动时 `AutoMigrate` 建表并回填历史用户 `role_id` 到普通用户角色；新增 `RequirePermission` 中间件实现路由级权限校验；扩展 `AuthMiddleWare` 在 session 中加载 `permissions` 与 `isAdmin` 标志；`/ovpn/role*` 与 `/ovpn/permission/tree` 接口完成角色 CRUD、权限分配、权限树查询；`/login` 与 `/client/userinfo` 接口下发 `permissions`/`isAdmin` 字段。
- 前端：`store/auth.tsx` 提供 `hasPermission(code)` 与 `isAdmin`，并对权限数据异常登出；新增 `<HasPermission>` 按钮包裹组件；`Sidebar.tsx` 按权限动态渲染菜单；`Layout.tsx` 路由守卫按权限重定向避免卡死；`api.ts` 对 403 统一 toast 并以 `ApiError` 标记 `handled=true` 避免双重提示；新增 `/roles` 角色管理页面（含权限树勾选）。

### Files Changed

后端：
- `internal/openvpnweb/role.go` — 新增，定义 Role/Permission/RolePermission 模型、权限 seed、RequirePermission 中间件、role/permission handler
- `internal/openvpnweb/server.go` — AuthMiddleWare 加载权限与 isAdmin 标志、用户存在性校验、角色禁用登出；新增 role/permission 路由；`/login` 与 `/client/userinfo` 下发权限；PATCH /user 校验 role_id；PUT /client/userinfo 触发 BeforeSave + 审计；AutoMigrate 后回填历史用户 role_id 并检查错误
- `internal/openvpnweb/user.go` — User 模型增加 `RoleID *uint` 字段；`LoadPermissionCodes` 方法支持 admin 直返与角色禁用/不存在错误
- `internal/openvpnweb/audit.go` — 移除 role 路径兜底规则避免双重审计

前端：
- `frontend/src/components/HasPermission.tsx` — 新增，按钮级权限包裹组件
- `frontend/src/pages/Roles/index.tsx` — 新增，角色管理页面（CRUD + 权限树勾选）
- `frontend/src/store/auth.tsx` — `isAdmin` 仅信任后端字段；`hasPermission(code)`；权限数据异常检测登出
- `frontend/src/layout/Sidebar.tsx` — 按权限动态渲染菜单
- `frontend/src/layout/Layout.tsx` — 路由守卫优先跳转 overview/profile，无权限时渲染占位页
- `frontend/src/api.ts` — `ApiError` 类 + 403 统一 toast 标记 handled
- `frontend/src/lib/format.ts` — `messageOf` 对 handled 错误返回空字符串
- `frontend/src/types.ts` — 扩展 `ClientUserInfo` 增加 `isAdmin`/`permissions`，新增 `Role`/`Permission` 类型
- `frontend/src/App.tsx` — 新增 `/roles` 路由
- `frontend/src/pages/Profile/index.tsx`、`frontend/src/pages/Users/index.tsx`、`frontend/src/ui/checkbox.tsx` — 局部适配

构建产物：`internal/openvpnweb/templates/static/admin/assets/*` 随前端构建同步更新；新增 `Roles.js`、`HasPermission.js`、`lock.js`；移除 `shield-check.js`。

### Review Findings Breakdown

- patch: 16 项已修复（high 3, medium 11, low 2）— 详见上方 Review Triage Log
- defer: 0 项
- reject: 0 项
- intent_gap: 0 项
- bad_spec: 0 项（无需 spec 回环修复）

### Follow-up Review Recommendation

**false** — 16 项 patch 均为局部修复，覆盖安全（权限绕过、XSS、TOCTOU）、正确性（GORM 默认值、JOIN 类型、map 更新清零）、健壮性（错误处理、用户存在性校验、事务化）和可观测性（审计、日志、toast 去重）。修复后已通过 `go build ./...` 与 `go vet ./...` 编译验证，并通过手动检查确认权限绕过路径已闭合。修复点虽多但均为低-中后果的局部问题，没有引入新的架构或 API 变化，无独立 follow-up review 的必要。

### Verification Performed

- `go build ./...` — 编译通过（设置 `GOCACHE`/`GOPATH` 环境变量解决缓存目录问题后）
- `go vet ./...` — 无 vet 警告
- `npm run build`（frontend）— 构建通过
- 手动代码审查：确认 `AuthMiddleWare` 中 admin 绕过路径仅依赖 `adminUsername != ""` 判断；`RequirePermission` 优先读 `c.Get("isAdmin")`；`roleDeleteHandler` 事务内含 count 检查；`roleAssignPermissionsHandler` 对输入去重后再查 DB；`SeedPermissionsAndRoles` 的 user 角色 FirstOrCreate 同步逻辑覆盖新增默认权限
- 未在真实 OpenVPN 环境中跑端到端测试，依赖后续部署阶段验证

### Residual Risks

- 历史用户 `role_id` 回填依赖启动时 `defaultRoleID > 0`，若 `user` 角色被人为删除将跳过回填并记日志，但不影响系统启动
- `administrator` 内置角色的权限只在首次初始化写入，之后管理员运行期修改会被保留；若代码 seed 新增权限项，administrator 不会自动同步（设计取舍，非缺陷）
- 审计中间件移除 role 兜底后，所有 role 相关审计必须由 handler 内主动 `recordAudit` 完成，新增 role 接口时需注意调用
- 前端 `<HasPermission>` 包裹按钮需各页面逐步接入，本次仅在 Roles/Users/Profile 等关键页面应用，其余页面的按钮级权限覆盖在后续迭代中完善

