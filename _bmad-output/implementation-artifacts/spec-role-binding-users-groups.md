---
title: '角色绑定用户与用户组'
type: 'feature'
created: '2026-08-05'
status: 'done'
review_loop_iteration: 0
followup_review_recommended: true
baseline_revision: 'b81a93de51afdb1b1c4819dd6e6266c5b3990865'
final_revision: '3511be8d99d752e2448be52aab147b7ca699a729'
context:
  - '{project-root}/internal/openvpnweb/role.go'
  - '{project-root}/internal/openvpnweb/user.go'
  - '{project-root}/internal/openvpnweb/group.go'
  - '{project-root}/internal/openvpnweb/server.go'
  - '{project-root}/frontend/src/pages/Roles/index.tsx'
  - '{project-root}/frontend/src/types.ts'
warnings: [oversized]
---

<intent-contract>

## Intent

**Problem:** 角色管理页面只能编辑角色信息与分配权限，看不到也无法管理"哪些用户/用户组在使用这个角色"，管理员无法从角色视角批量分配用户或设置组的默认角色，只能逐个编辑用户来设置 role_id。

**Approach:** 在角色列表操作列新增"分配用户"（穿梭框，管理 user.role_id）与"分配用户组"（树形勾选，管理 group.role_id）两个入口；Group 模型增加 RoleID 字段实现"组的默认角色"，新建用户未指定角色时继承所在组的角色；新增 role:assign_users / role:assign_groups 两个权限码并 seed 到角色管理权限组。

## Boundaries & Constraints

**Always:**
- 所有代码、注释、commit message、文档使用中文
- 前端使用 shadcn/ui + Tailwind 组件，表单左对齐 label（140px 固定宽度），placeholder 12px/28% 透明度
- 后端响应消息 UTF-8 中文；穿梭框/树形勾选保存时使用事务全量替换
- admin 用户（username == adminUsername）不出现在穿梭框中（admin 绕过权限检查，不需要角色）
- 内置 administrator 角色支持分配用户与用户组（系统管理员可手动为用户/组绑定超管角色，实现灵活的多超管配置）
- Group.RoleID 为 *uint 指针，nil 表示未绑定角色；AutoMigrate 自动加列，历史组 role_id 为 NULL 不影响
- 新建用户 role_id 为空时的填充优先级：显式指定 > 所在组的 RoleID > 普通用户默认角色 ID
- 一个用户组只能绑定一个角色（与单用户单角色架构一致）；在角色 A 页面勾选已绑定角色 B 的组，会自动把该组的 RoleID 从 B 改为 A
- "分配用户组"对话框中需展示每个组当前绑定的角色名（若已绑定其他角色，勾选时会提示"将从原角色转移"）

**Block If:**
- 无（设计决策已明确，无需人工介入）

**Never:**
- 不引入 user_roles 多对多关联表（保持 user.role_id 单用户单角色）
- 不做权限叠加（用户登录权限仅来自 user.role_id，不合并 group.role_id 的权限）
- 不修改 LoadPermissionCodes 现有逻辑（组角色仅作为新建用户的默认值来源，不影响已存在用户）
- 不在用户组页面增加角色管理入口（本次仅在角色页面做分配入口；用户组页面的角色展示为可选 follow-up）
- 不允许通过"分配用户组"接口修改 Default 组（ID=1）的 RoleID（Default 组保持无角色绑定）

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| 查看角色已绑定用户 | GET /ovpn/role/:id/users | 返回 {allUsers:[{id,username,name,gid,groupName,roleId,roleName}], assignedUserIds:[1,2,3]} | 角色不存在返回 404 |
| 穿梭框保存 | PUT /ovpn/role/:id/users body:{userIds:[1,2,3]} | 事务内：把 userIds 中的用户 role_id 设为该角色；把原来在该角色但不在 userIds 中的用户 role_id 设为 NULL（回填默认角色） | 用户不存在跳过并记日志；角色不存在返回 404 |
| 查看角色已绑定用户组 | GET /ovpn/role/:id/groups | 返回 {allGroups:[{id,name,parentId,roleId,roleName}], assignedGroupIds:[1,2]} | 角色不存在返回 404 |
| 树形勾选保存 | PUT /ovpn/role/:id/groups body:{groupIds:[1,2]} | 事务内：把 groupIds 中的组 role_id 设为该角色；把原来在该角色但不在 groupIds 中的组 role_id 设为 NULL | Default 组(ID=1)拒绝修改其 role_id，返回 400 |
| 新建用户未指定角色且所在组有 RoleID | POST /ovpn/user 无 roleId，gid 对应组有 RoleID | 用户 role_id 自动填充为组的 RoleID | 组不存在时回退到默认角色 |
| 内置 administrator 角色分配用户 | PUT /ovpn/role/{adminRoleId}/users | 正常执行，把指定用户的 role_id 设为 administrator 角色（多超管配置支持） | - |
| 删除角色时存在已绑定用户组 | DELETE /ovpn/role/:id 有组 role_id 指向该角色 | 返回 400 "角色下存在用户或用户组，不允许删除" | - |

</intent-contract>

## Code Map

- `internal/openvpnweb/group.go` -- Group 模型新增 `RoleID *uint` 字段；Update 方法支持更新 role_id
- `internal/openvpnweb/role.go` -- 新增 roleUsersHandler/roleAssignUsersHandler/roleGroupsHandler/roleAssignGroupsHandler；roleDeleteHandler 增加用户组检查；新增 role:assign_users / role:assign_groups 权限码 seed
- `internal/openvpnweb/server.go` -- 注册 4 个新路由；POST /ovpn/user 创建用户时 role_id 填充优先级增加"所在组 RoleID"；AutoMigrate 已包含 Group（新增字段自动迁移）
- `internal/openvpnweb/user.go` -- 无结构性改动（User.RoleID 已存在）
- `frontend/src/types.ts` -- Role 接口增加 userCount/groupCount 可选字段；Group 接口增加 roleId/roleName 可选字段
- `frontend/src/pages/Roles/index.tsx` -- 操作列新增"分配用户""分配用户组"按钮；新增 UserAssignDialog（穿梭框）与 GroupAssignDialog（树形勾选）组件；列表增加用户数/用户组数列
- `frontend/src/api.ts` -- 无需改动（复用现有 get/postJson/putJson 方法）

## Tasks & Acceptance

**Execution:**
- [x] `internal/openvpnweb/group.go` -- Group 结构体新增 `RoleID *uint \`gorm:"column:role_id;default:NULL" json:"roleId" form:"roleId"\`` 字段；Update 方法在 updates map 中支持 role_id 更新（Default 组 ID=1 拒绝修改 role_id） -- 数据基础
- [x] `internal/openvpnweb/role.go` -- 在 buttonPermissions 中新增 `role:assign_users`（"分配用户"）与 `role:assign_groups`（"分配用户组"）两个权限码，parentCode 为 menu:roles -- 权限 seed
- [x] `internal/openvpnweb/role.go` -- 新增 `roleUsersHandler(c)` GET /ovpn/role/:id/users：查询所有非 admin 用户（id/username/name/gid/roleId），批量查组名与角色名拼装，返回 allUsers + assignedUserIds（role_id == :id 的用户 ID 列表） -- 角色用户查询
- [x] `internal/openvpnweb/role.go` -- 新增 `roleAssignUsersHandler(c)` PUT /ovpn/role/:id/users：接收 {userIds:[]}；事务内先把该角色下不在 userIds 中的用户 role_id 设为 NULL，再把 userIds 中的用户 role_id 设为 :id；排除 admin 用户（username == adminUsername）；写审计日志 -- 角色用户分配（支持内置 administrator 角色，实现灵活多超管配置）
- [x] `internal/openvpnweb/role.go` -- 新增 `roleGroupsHandler(c)` GET /ovpn/role/:id/groups：查询所有组（id/name/parentId/roleId），批量查角色名拼装，返回 allGroups + assignedGroupIds（role_id == :id 的组 ID 列表） -- 角色用户组查询
- [x] `internal/openvpnweb/role.go` -- 新增 `roleAssignGroupsHandler(c)` PUT /ovpn/role/:id/groups：接收 {groupIds:[]}；事务内先把该角色下不在 groupIds 中的组 role_id 设为 NULL，再把 groupIds 中的组 role_id 设为 :id；Default 组(ID=1)拒绝修改 role_id（400）；写审计日志 -- 角色用户组分配（支持内置 administrator 角色，实现组默认继承超管角色）
- [x] `internal/openvpnweb/role.go` -- `roleDeleteHandler` 事务内增加用户组数量检查：`tx.Model(&Group{}).Where("role_id = ?", role.ID).Count(&groupCount)`，groupCount > 0 时返回 400 "角色下存在用户或用户组，不允许删除" -- 删除保护
- [x] `internal/openvpnweb/role.go` -- `roleListHandler` 批量查询每个角色的用户数与用户组数，返回 JSON 增加 userCount/groupCount 字段 -- 列表展示
- [x] `internal/openvpnweb/server.go` -- 注册 4 个新路由：GET/PUT /ovpn/role/:id/users（RequirePermission role:assign_users）、GET/PUT /ovpn/role/:id/groups（RequirePermission role:assign_groups） -- 路由注册
- [x] `internal/openvpnweb/server.go` -- POST /ovpn/user 创建用户时，role_id 为空的填充优先级改为：先查所在组的 RoleID，若非空则用组的 RoleID，否则用 GetDefaultRoleID -- 组角色继承
- [x] `frontend/src/types.ts` -- Role 接口增加 `userCount?: number` / `groupCount?: number`；Group 类型增加 `roleId?: number|null` / `roleName?: string` -- 类型基础
- [x] `frontend/src/pages/Roles/index.tsx` -- 列表新增"用户数""用户组数"两列（居中，Badge 展示）；操作列在"分配权限"后新增"分配用户"（Users 图标）与"分配用户组"（Network 图标）按钮，包裹 HasPermission -- 列表增强
- [x] `frontend/src/pages/Roles/index.tsx` -- 新增 UserAssignDialog 组件：穿梭框布局（左侧"可选用户"右侧"已选用户"，中间穿梭按钮），支持搜索过滤，左侧显示用户名+所在组，右侧显示用户名+所在组；底部显示"已选 X/Y 人"；保存调用 PUT /ovpn/role/:id/users -- 穿梭框实现
- [x] `frontend/src/pages/Roles/index.tsx` -- 新增 GroupAssignDialog 组件：树形勾选对话框（复用权限树的树形渲染模式），展示用户组树，每个组节点显示名称+当前绑定角色名（若已绑定其他角色显示橙色 Badge 提示"将从原角色转移"）；Default 组节点禁用勾选并提示"默认组不支持绑定角色"；保存调用 PUT /ovpn/role/:id/groups -- 树形勾选实现
- [x] `frontend/src/pages/Roles/index.tsx` -- 内置 administrator 角色："分配用户"与"分配用户组"按钮均正常可用（支持多超管配置与组默认继承超管角色）-- 内置角色支持分配

**Acceptance Criteria:**
- Given admin 进入角色管理页面，when 查看角色列表，then 每个角色显示用户数与用户组数两列
- Given admin 点击某自定义角色的"分配用户"，when 穿梭框弹出，then 左侧显示所有非 admin 用户（含所在组名），右侧显示该角色已绑定用户；搜索框可按用户名过滤
- Given 穿梭框中把用户从左侧移到右侧并保存，when 保存成功，then 该用户的 role_id 被设为该角色；把用户从右侧移除并保存，then 该用户的 role_id 被设为 NULL（回填默认角色）
- Given admin 点击某角色的"分配用户组"，when 树形勾选对话框弹出，then 展示用户组树，已绑定该角色的组显示选中态，绑定其他角色的组显示橙色"将从原角色转移"提示
- Given 勾选一个已绑定角色 B 的组并保存（当前在角色 A 页面），when 保存成功，then 该组的 role_id 从 B 改为 A
- Given 尝试勾选 Default 组（ID=1），when 点击勾选框，then 勾选框禁用，提示"默认组不支持绑定角色"
- Given 新建用户未指定角色且所在组已绑定角色 R，when 创建用户成功，then 该用户的 role_id 自动设为 R
- Given 删除一个仍有用户组绑定的角色，when 调用 DELETE /ovpn/role/:id，then 返回 400 "角色下存在用户或用户组，不允许删除"
- Given 内置 administrator 角色，when 点击"分配用户"，then 穿梭框正常弹出，可将用户绑定为系统超管（多超管配置）
- Given 内置 administrator 角色，when 点击"分配用户组"，then 树形勾选正常弹出，勾选用户组后该组新建用户将自动继承超管角色

## Design Notes

**穿梭框设计（UserAssignDialog）：**
shadcn/ui 无原生 Transfer 组件，用双栏 Card + 中间穿梭按钮实现：
- 左栏标题"可选用户"，右栏标题"已选用户"
- 每栏顶部有搜索框（按用户名过滤）
- 用户条目：用户名 + 所在组名（muted 小字）
- 中间两个按钮：→（选中到右侧）、←（移回左侧）
- 底部统计"已选 X / 共 Y 人"
- 保存时右侧所有用户 ID 即为分配列表

**树形勾选设计（GroupAssignDialog）：**
复用 Permissions 页面的树形渲染模式（ChevronRight/Down 展开 + Checkbox 勾选）：
- 组节点显示：组名 + 当前绑定角色名（若有，显示为 Badge）
- 已绑定其他角色的组：勾选时显示橙色提示"将从 {原角色名} 转移"
- Default 组（ID=1）：Checkbox 禁用，显示"默认组"Badge + 提示文字
- 支持父节点全选/取消全选子节点（与权限树行为一致）

**Group.RoleID 与单用户单角色的一致性：**
Group.RoleID 是"组的默认角色"，语义与 User.RoleID 平行：
- 用户角色（User.RoleID）= 用户直接绑定的角色，决定登录权限
- 组角色（Group.RoleID）= 该组新建用户的默认角色来源
- 不做权限叠加：已存在用户的权限仅来自其 User.RoleID，不受组角色影响
- "同步到组内用户"为可选 follow-up（本次不实现），管理员可通过"分配用户"穿梭框手动同步

## Verification

**Commands:**
- `cd f:\develop\openvpn && go build ./...` -- expected: 编译通过，无错误
- `cd f:\develop\openvpn && go vet ./...` -- expected: 无 vet 警告
- `cd f:\develop\openvpn\frontend && npm run build` -- expected: 前端构建通过

**Manual checks:**
- 启动后端，确认 Group 表新增 role_id 列；新增权限码 role:assign_users / role:assign_groups 已 seed
- admin 进入 /roles → 列表显示用户数/用户组数列 → 点击"分配用户"穿梭框正常工作 → 点击"分配用户组"树形勾选正常工作
- 新建用户指定所在组（组已绑定角色 R）但不指定角色 → 用户 role_id 自动为 R
- 删除有用户组绑定的角色 → 返回 400

## Review Triage Log

### 2026-08-05 — Review pass 1

- intent_gap: 0
- bad_spec: 0
- patch: 7: (high 1, medium 5, low 1)
- defer: 8: (high 3, medium 4, low 1)
- reject: 3: (medium 1, low 2)
- addressed_findings:
  - `[high]` `[patch]` roleAssignGroupsHandler 未拒绝内置 administrator 角色 + 前端"分配用户组"按钮未禁用 — 添加与 assign_users 一致的 administrator 拒绝检查；前端按钮添加 disabled 逻辑
  - `[medium]` `[patch]` Group.Update() 不校验 RoleID 有效性 — 添加 validateRoleID 调用与 *g.RoleID > 0 守卫，避免产生孤儿 group.role_id
  - `[medium]` `[patch]` CSV 批量导入用户不继承用户组角色 — 复用单用户创建的继承逻辑（组角色 > 默认角色）
  - `[medium]` `[patch]` roleListHandler 的 userCount 未排除 admin 用户 — 添加 adminUsername 排除条件，与 roleUsersHandler 保持一致
  - `[medium]` `[patch]` roleAssignGroupsHandler 使用 tx.Table("group") 直接拼接 — 改为 tx.Model(&Group{}) 让 GORM 处理保留字引号
  - `[medium]` `[patch]` 可向已禁用角色分配用户/用户组 — 在 assign_users 与 assign_groups handler 中添加 role.IsEnable 检查
  - `[low]` `[patch]` 对话框在保存过程中可通过遮罩点击/Escape 关闭 — 为 UserAssignDialog 与 GroupAssignDialog 的 DialogContent 添加 onPointerDownOutside/onEscapeKeyDown 守卫

### 2026-08-06 — Follow-up: 移除 administrator 分配限制（需求变更）

- 业务需求：系统管理员角色（administrator）也要支持分配用户和用户组，实现灵活的多超管配置与组默认继承超管角色
- patch: 4
- addressed_findings:
  - `[patch]` roleAssignUsersHandler 移除 administrator 拒绝检查（400 → 正常执行）
  - `[patch]` roleAssignGroupsHandler 移除 administrator 拒绝检查（400 → 正常执行）
  - `[patch]` 前端"分配用户"按钮移除 administrator 禁用逻辑（disabled + title 提示 → 正常可用）
  - `[patch]` 前端"分配用户组"按钮移除 administrator 禁用逻辑（disabled + title 提示 → 正常可用）

## Auto Run Result

Status: done

### Summary of Implemented Change

为角色管理页面新增"分配用户"（穿梭框）与"分配用户组"（树形勾选）功能，实现从角色视角批量管理用户与用户组的角色绑定。Group 模型新增 RoleID 字段作为"组的默认角色"，新建用户未指定角色时优先继承所在组的角色。

Follow-up 更新（2026-08-06）：根据需求移除对内置 administrator 角色的分配限制，支持：
1. **多超管配置**：可通过 administrator 角色的"分配用户"穿梭框，将任意非 admin 账号提升为系统超管（拥有全部权限）
2. **组默认继承超管**：可通过 administrator 角色的"分配用户组"树形勾选，设置某组的默认角色为超管，该组新建用户将自动获得超管权限

### Files Changed

- `internal/openvpnweb/group.go` — Group 结构体新增 RoleID 字段；Update 方法支持 role_id 更新并校验角色有效性
- `internal/openvpnweb/role.go` — 新增 role:assign_users / role:assign_groups 权限码；实现 4 个新 handler；roleDeleteHandler 增加用户组检查；roleListHandler 返回 userCount/groupCount；assign handler 添加禁用角色检查；**follow-up 移除 administrator 拒绝检查**
- `internal/openvpnweb/server.go` — 注册 4 个新路由；POST /ovpn/user 与 CSV 导入均支持组角色继承
- `frontend/src/types.ts` — Role 接口增加 userCount/groupCount；Group 接口增加 roleId/roleName
- `frontend/src/pages/Roles/index.tsx` — 列表增加用户数/用户组数列；新增 UserAssignDialog（穿梭框）与 GroupAssignDialog（树形勾选）；**follow-up 移除 administrator 按钮禁用逻辑**；对话框保存时禁止关闭
- `_bmad-output/implementation-artifacts/spec-role-binding-users-groups.md` — 更新 Boundaries & Constraints、I/O Matrix、Tasks & Acceptance、Review Triage Log 以反映需求变更

### Review Findings Breakdown

- **Patches applied (pass 1)**: 7 项（1 高危 + 5 中危 + 1 低危）
- **Patches applied (follow-up)**: 4 项（需求变更，移除 administrator 分配限制）
- **Items deferred**: 8 项（3 高危 pre-existing + 4 中危 + 1 低危，记录到 deferred-work.md）
- **Items rejected**: 3 项（spec 明确要求返回全量字段、设计决定、误报）

### Follow-up Review Recommendation

true — 本次 follow-up 移除了 administrator 角色的分配限制，涉及权限模型的核心决策变更（从"拒绝超管角色分配"改为"支持多超管配置"），建议独立 follow-up review 验证以下风险：
1. 是否有用户被误分配到 administrator 角色导致权限泄漏
2. Default 组（ID=1）的保护逻辑是否仍然有效
3. 禁用角色的分配保护是否仍然生效

### Verification Performed

- `go build ./cmd/openvpn-web` — 编译通过，exit code 0
- `cd frontend && npm run build` — 前端构建通过（2921 modules, 1.75s），无错误

### Residual Risks

1. PATCH /ovpn/user 仍可通过 user:update 权限修改 role_id（pre-existing，已记录到 deferred-work）
2. RBAC 缺乏角色层级约束，assign_users 可分配到任意已启用角色（**本次有意放开**，用于多超管配置）
3. MySQL/PostgreSQL 下 roleDelete 与 assign 操作存在 TOCTOU 竞态（pre-existing，需统一加锁方案）
4. administrator 角色分配放开后，需注意 admin 账号本身（username == adminUsername）仍被排除在分配逻辑之外，避免产生孤儿绑定
