---
title: '用户多角色绑定'
type: 'refactor'
created: '2026-08-06'
status: 'done'
review_loop_iteration: 0
followup_review_recommended: true
baseline_revision: '4af1a1918311b98309001a04ad628fc1686b1446'
final_revision: 'de2ff6a9e8d792d3620f96ee53ade0510bed1200'
context:
  - '{project-root}/internal/openvpnweb/user.go'
  - '{project-root}/internal/openvpnweb/role.go'
  - '{project-root}/internal/openvpnweb/group.go'
  - '{project-root}/internal/openvpnweb/server.go'
  - '{project-root}/frontend/src/pages/Roles/index.tsx'
  - '{project-root}/frontend/src/pages/Users/index.tsx'
  - '{project-root}/frontend/src/types.ts'
  - '{project-root}/frontend/src/store/auth.tsx'
warnings: [oversized]
---

<intent-contract>

## Intent

**Problem:** 当前系统采用单用户-单角色模型（`user.role_id` 指向唯一角色），无法满足"一个用户同时拥有多个角色"的需求。例如一个用户需要同时具备"审计员"和"网络管理员"的权限，当前只能新建一个合并权限的角色，管理成本高且不灵活。

**Approach:** 引入 `user_role` 多对多关联表替代 `user.role_id` 单字段；`LoadPermissionCodes` 改为从用户的所有角色并集加载权限码；用户编辑表单新增多角色选择器；角色页穿梭框语义从"转移"改为"增删本角色"（不影响用户的其他角色）；启动时自动迁移历史 `role_id` 数据到 `user_role` 表。

## Boundaries & Constraints

**Always:**
- 所有代码、注释、commit message、文档使用中文
- `user_role` 表复合主键 `(user_id, role_id)`，与 `role_permission` 表设计一致
- 权限码并集去重：用户多角色的权限码取并集后去重，`LoadRolePermissionCodes` 逐角色加载后合并
- 禁用的角色跳过（不报错、不断登），仅当用户所有角色均禁用时返回空权限集
- Group.RoleID 保持单角色 `*uint` 不变（本次仅改造用户侧；组默认角色仍为单角色）
- 用户创建时角色填充优先级：显式传入 roleIds > 所在组的 RoleID（单条） > 默认普通用户角色
- `user.role_id` 列在 DB 中保留（SQLite AutoMigrate 不删列），但 User 结构体移除该字段，GORM 不再读写
- admin 用户（username == adminUsername）仍返回 `["*"]`，不进入 user_role 表
- Default 组（ID=1）保护逻辑不变
- 前端穿梭框保存时仍使用事务全量替换（DELETE + INSERT），但仅影响当前角色的绑定

**Block If:**
- 无（设计决策已明确，无需人工介入）

**Never:**
- 不引入角色层级或权限子集校验（pre-existing 设计局限，不在本次范围）
- 不修改 `role_permission` 表结构（角色-权限多对多不变）
- 不修改 Group.RoleID 为多角色（组默认角色仍为单角色）
- 不删除 DB 中的 `user.role_id` 列（SQLite 无法 DROP COLUMN，保留为历史遗留列）
- 不修改 `LoadRolePermissionCodes` 函数签名（保持单角色查询，由上层循环调用）

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| 用户登录（多角色） | POST /login 用户有 roleIds=[1,2] | 返回 permissions=角色1∪角色2的权限码并集，roleIds=[1,2]，roleNames=["系统超管","审计员"] | 某角色禁用时跳过，仅合并启用角色权限 |
| 用户登录（所有角色禁用） | POST /login 用户 roleIds=[1,2] 均禁用 | 返回 permissions=[]，可登录但无任何菜单权限 | 不返回 403，不强制登出 |
| 创建用户指定多角色 | POST /ovpn/user body含 roleIds=[2,3] | 用户创建成功，user_role 表插入 (uid,2)(uid,3) 两条记录 | 角色不存在或禁用返回 400 |
| 创建用户未指定角色且组有角色 | POST /ovpn/user 无 roleIds，gid 对应组 RoleID=3 | user_role 表插入 (uid,3) | 组不存在时回退到默认角色 |
| 更新用户角色 | PATCH /ovpn/user body含 roleIds=[2,3] | 事务内全量替换 user_role：删除旧绑定，插入新绑定 | 角色不存在或禁用返回 400 |
| 角色穿梭框保存 | PUT /ovpn/role/1/users body:{userIds:[1,2,3]} | 事务内：DELETE FROM user_role WHERE role_id=1 AND user_id NOT IN [1,2,3]；INSERT IGNORE (1,1)(2,1)(3,1) | 用户不存在跳过并记日志 |
| 查看角色已绑定用户 | GET /ovpn/role/1/users | 返回 allUsers（每个用户含 roleIds/roleNames 数组），assignedUserIds=[在该角色下的用户ID] | 角色不存在返回 404 |
| 删除角色时存在用户绑定 | DELETE /ovpn/role/:id user_role 表有记录 | 返回 400 "角色下存在用户或用户组，不允许删除" | - |
| 角色列表 userCount | GET /ovpn/role | 每个角色的 userCount 从 user_role 表 COUNT（排除 admin 用户） | - |
| AuthMiddleWare 权限加载 | 每次请求 | 从 user_role 表查所有 role_id，逐角色加载权限码并集去重 | 用户无任何角色绑定时回退到默认角色 |

</intent-contract>

## Code Map

- `internal/openvpnweb/user.go` -- 新增 `UserRole` 结构体（join table）；User 结构体移除 `RoleID` 字段，新增 `gorm:"-"` 临时字段 `RoleIDs []uint`；重写 `LoadPermissionCodes` 为多角色并集加载；`Create()` / `Update()` 方法增加 user_role 同步逻辑；`Delete()` 增加清理 user_role 记录
- `internal/openvpnweb/role.go` -- `roleUsersHandler` 改为查 user_role 表；`roleAssignUsersHandler` 改为 user_role 表的 INSERT/DELETE；`roleListHandler` userCount 改为查 user_role 表；`roleDeleteHandler` 检查 user_role 表；新增 `validateRoleIDs` 批量校验
- `internal/openvpnweb/server.go` -- AutoMigrate 增加 `UserRole`；启动迁移把 `user.role_id` 复制到 `user_role`；POST/PATCH /ovpn/user 处理 roleIds；AuthMiddleWare 移除 `c.Set("roleId", ...)`；登录响应返回 roleIds/roleNames 数组
- `internal/openvpnweb/group.go` -- 无结构性改动（Group.RoleID 保持不变）
- `frontend/src/types.ts` -- `UserRecord` 改 `roleId/roleName` 为 `roleIds?:number[]` / `roleNames?:string[]`；`ClientUserInfo` 同理
- `frontend/src/pages/Roles/index.tsx` -- `UserAssignDialog` 中 `RoleAssignUser` 接口改为 `roleIds/roleNames` 数组；移除"将从「xxx」转移"警告，改为显示用户已有角色列表
- `frontend/src/pages/Users/index.tsx` -- `UserFormDialog` 新增多角色选择器（Checkbox Group 或 Multi-Select）；用户列表角色列改为展示多个角色 Badge
- `frontend/src/store/auth.tsx` -- `roleId` → `roleIds` 字段适配

## Tasks & Acceptance

**Execution:**
- [x] `internal/openvpnweb/user.go` -- 新增 `UserRole` 结构体（`UserID uint`/`RoleID uint`/`CreatedAt time.Time`，复合主键，TableName="user_role"）；User 结构体移除 `RoleID *uint` 字段，新增 `RoleIDs []uint \`gorm:"-" json:"roleIds" form:"roleIds"\`` 临时字段 -- 数据基础
- [x] `internal/openvpnweb/user.go` -- 重写 `LoadPermissionCodes`：从 `user_role` 表查所有 role_id，逐角色调用 `LoadRolePermissionCodes`，并集去重；无绑定时回退 `GetDefaultRoleID`；禁用角色跳过不报错 -- 权限加载核心
- [x] `internal/openvpnweb/user.go` -- `Create()` 方法在 `db.Create(&u)` 后，若 `u.RoleIDs` 非空则批量 INSERT user_role 记录；若为空则执行组角色继承逻辑（查 Group.RoleID，插入 user_role）或回退默认角色 -- 创建同步
- [x] `internal/openvpnweb/user.go` -- `Update()` 方法在 `db.Model(&u).Updates(&u)` 后，若 `u.RoleIDs` 非 nil（区分 nil 与空切片：nil=不修改，空切片=清空），事务内全量替换 user_role 记录 -- 更新同步
- [x] `internal/openvpnweb/user.go` -- `Delete()` 方法增加 `db.Where("user_id = ?", id).Delete(&UserRole{})` 清理关联记录 -- 删除清理
- [x] `internal/openvpnweb/role.go` -- 新增 `validateRoleIDs(db, roleIDs []uint) error` 批量校验角色存在且启用，供 POST/PATCH /user 复用 -- 数据校验
- [x] `internal/openvpnweb/role.go` -- `roleUsersHandler` 改为查 user_role 表获取 assignedUserIds；allUsers 中每个用户的 roleId/roleName 改为 roleIds/roleNames 数组（批量查 user_role JOIN role） -- 角色用户查询
- [x] `internal/openvpnweb/role.go` -- `roleAssignUsersHandler` 改为事务内操作 user_role 表：DELETE WHERE role_id=:id AND user_id NOT IN ?；INSERT IGNORE 新增记录；排除 admin 用户；审计日志记录 added/removed ID 列表 -- 角色用户分配
- [x] `internal/openvpnweb/role.go` -- `roleListHandler` userCount 改为 `SELECT COUNT(*) FROM user_role WHERE role_id = ?`（排除 admin 用户需 JOIN user 表） -- 列表展示
- [x] `internal/openvpnweb/role.go` -- `roleDeleteHandler` 检查改为 `SELECT COUNT(*) FROM user_role WHERE role_id = ?` -- 删除保护
- [x] `internal/openvpnweb/server.go` -- AutoMigrate 增加 `db.AutoMigrate(&UserRole{})`；启动迁移逻辑：`INSERT INTO user_role (user_id, role_id) SELECT id, role_id FROM user WHERE role_id IS NOT NULL AND role_id > 0`（SQLite 用 INSERT OR IGNORE） -- 数据库迁移
- [x] `internal/openvpnweb/server.go` -- POST /ovpn/user handler：解析 body 中 roleIds；若非空则 `validateRoleIDs`；若空则执行组角色继承（查 Group.RoleID 加入 roleIds）或回退默认角色；调用 `u.Create()` 后同步 user_role -- 用户创建
- [x] `internal/openvpnweb/server.go` -- PATCH /ovpn/user handler：解析 body 中 roleIds；`validateRoleIDs`；调用 `u.Update()` 后同步 user_role -- 用户更新
- [x] `internal/openvpnweb/server.go` -- CSV 导入用户：roleIds 继承逻辑与单用户一致（组角色 > 默认角色） -- 批量导入
- [x] `internal/openvpnweb/server.go` -- AuthMiddleWare：移除 `c.Set("roleId", u.RoleID)`；登录响应中 `roleId` → `roleIds`（数组），`roleName` → `roleNames`（数组）；`/me` 接口同理 -- 会话与响应
- [x] `frontend/src/types.ts` -- `UserRecord`：`roleId?:number|null` → `roleIds?:number[]`，`roleName?:string` → `roleNames?:string[]`；`ClientUserInfo`：`roleId?:number|null` → `roleIds?:number[]`，`roleName?:string` → `roleNames?:string[]` -- 类型基础
- [x] `frontend/src/pages/Roles/index.tsx` -- `RoleAssignUser` 接口改 `roleId/roleName` 为 `roleIds/roleNames` 数组；移除穿梭框中"将从「xxx」转移"警告；用户条目改为显示已有角色列表（多个 Badge） -- 穿梭框适配
- [x] `frontend/src/pages/Users/index.tsx` -- `UserFormDialog` 新增多角色选择器（Checkbox Group，显示角色名称列表）；提交 payload 增加 `roleIds: number[]`；用户列表角色列改为展示多个角色 Badge -- 用户表单增强
- [x] `frontend/src/store/auth.tsx` -- `roleId` → `roleIds` 字段适配；`hasPermission` 逻辑不变（仍检查 permissions 数组） -- 状态适配

**Acceptance Criteria:**
- Given 用户 A 绑定角色 [审计员, 网络管理员]，when A 登录，then 返回的 permissions 为两个角色权限码的并集去重
- Given 用户 A 绑定角色 [审计员]，when 在"网络管理员"角色页穿梭框把 A 移到右侧并保存，then A 的角色变为 [审计员, 网络管理员]（不丢失审计员角色）
- Given 用户 A 绑定角色 [审计员, 网络管理员]，when 在"审计员"角色页穿梭框把 A 移回左侧并保存，then A 的角色变为 [网络管理员]（仅移除审计员，不影响网络管理员）
- Given 管理员创建用户时选择角色 [审计员, 网络管理员]，when 创建成功，then user_role 表有两条记录
- Given 管理员编辑用户时清空所有角色，when 保存成功，then user_role 表该用户记录被清空，用户登录后无任何菜单权限
- Given 用户仅有 1 个角色且该角色被禁用，when 登录，then 可登录但 permissions 为空
- Given 系统启动且有历史 user.role_id 数据，when 迁移完成，then user_role 表包含所有历史 role_id 的映射记录
- Given 删除一个仍有用户绑定的角色，when 调用 DELETE /ovpn/role/:id，then 返回 400
- Given 用户列表页，when 查看角色列，then 显示该用户所有角色名称的 Badge 列表

## Design Notes

**user_role 表设计与迁移策略：**
```sql
CREATE TABLE user_role (
    user_id  INTEGER NOT NULL,
    role_id  INTEGER NOT NULL,
    created_at DATETIME,
    PRIMARY KEY (user_id, role_id)
);
-- 迁移：INSERT OR IGNORE INTO user_role (user_id, role_id, created_at)
--   SELECT id, role_id, CURRENT_TIMESTAMP FROM user WHERE role_id IS NOT NULL AND role_id > 0;
```
- `User.RoleID` 列保留在 DB 中但 GORM 不再读写（User 结构体移除该字段）
- AutoMigrate 创建 `user_role` 表后，启动迁移脚本把历史 `role_id` 复制过来
- 迁移是幂等的（INSERT OR IGNORE / ON CONFLICT DO NOTHING）

**LoadPermissionCodes 多角色并集加载：**
```go
func (u *User) LoadPermissionCodes(d *gorm.DB) ([]string, error) {
    if adminUsername != "" && u.Username == adminUsername {
        return []string{"*"}, nil
    }
    var roleIDs []uint
    d.Table("user_role").Where("user_id = ?", u.ID).Pluck("role_id", &roleIDs)
    if len(roleIDs) == 0 {
        roleIDs = []uint{GetDefaultRoleID(d)} // 回退默认角色
    }
    codeSet := map[string]bool{}
    for _, rid := range roleIDs {
        if codes, err := LoadRolePermissionCodes(d, rid); err == nil {
            for _, c := range codes { codeSet[c] = true }
        } // 禁用/不存在的角色跳过
    }
    return keys(codeSet), nil
}
```

**穿梭框语义变更：**
- 旧：用户在角色 A 右侧 = user.role_id = A（唯一角色，从 B 转移到 A 会丢失 B）
- 新：用户在角色 A 右侧 = user_role 表有 (uid, A) 记录（不影响用户的其他角色绑定）
- 移除"将从「xxx」转移"橙色警告，改为显示用户当前已有角色列表（灰色 Badge）

**User.RoleIDs 临时字段的 nil vs 空切片语义：**
- `RoleIDs == nil`：不修改用户角色绑定（用于 PATCH 时仅修改其他字段）
- `RoleIDs == []`（空切片）：清空用户所有角色绑定
- `RoleIDs == [1,2]`：设置为这两个角色

## Verification

**Commands:**
- `cd f:\develop\openvpn && go build ./cmd/openvpn-web` -- expected: 编译通过，无错误
- `cd f:\develop\openvpn && go vet ./...` -- expected: 无 vet 警告
- `cd f:\develop\openvpn\frontend && npm run build` -- expected: 前端构建通过

**Manual checks:**
- 启动后端，确认 user_role 表自动创建；历史 user.role_id 数据已迁移到 user_role
- 创建用户选择多个角色 → user_role 表有多条记录 → 登录后权限为并集
- 角色页穿梭框：把已有其他角色的用户移到右侧 → 保存 → 用户同时拥有两个角色
- 用户列表页角色列显示多个 Badge
- 编辑用户清空角色 → 保存 → 用户登录后无菜单权限

## Review Triage Log

### 2026-08-06 — Review pass 1
- intent_gap: 0
- bad_spec: 0
- patch: 6: (high 2, medium 3, low 1)
- defer: 2: (medium 2)
- reject: 0
- addressed_findings:
  - `[high]` `[patch]` server.go 迁移 SQL 兼容全新部署：User 结构体已移除 RoleID 字段，但启动迁移代码仍直接引用 `user.role_id` 列，全新部署时该列不存在导致 SQL 报错。修复为通过 `pragma_table_info('user')` 检查列存在性，仅在列存在时执行回填与迁移。
  - `[high]` `[patch]` user.go Create/Update/Delete 缺少事务保护：原实现用 `db.Create` / `db.Where().Delete` / `db.Unscoped().Delete` 串行调用且忽略 user_role 写入错误，失败时 user 表与 user_role 表数据不一致。修复为统一用 `db.Transaction` 包裹，并显式返回所有错误（绑定失败、清理失败、写入失败均回滚）。
  - `[medium]` `[patch]` server.go /ovpn/user/me 接口未填充 roleIds/roleNames：`u.RoleIDs` 为 `gorm:"-"` 临时字段，`Info()` 不会加载，原实现直接 `c.JSON(http.StatusOK, u)` 导致 `roleIds` 为 `null`。修复为显式调用 `u.LoadRoleIDsAndNames(db)` 填充并以 gin.H 返回完整字段。
  - `[medium]` `[patch]` user.go LoadRoleIDsAndNames 错误时长度不一致：角色查询失败时原返回 `(roleIDs, []string{})`，roleIDs 有值而 roleNames 为空，前端解构后长度不匹配。修复为查询失败时统一返回 `([]uint{}, []string{})` 等长空切片。
  - `[medium]` `[patch]` server.go PATCH /ovpn/user 中 `hasRoleIDs` 死代码：变量声明后仅 `_ = hasRoleIDs` 抑制未使用警告，无实际业务用途。修复为直接移除该变量及其赋值。
  - `[low]` `[patch]` role.go validateRoleIDs 静默跳过 rid=0：原实现 `if rid == 0 { continue }` 允许零值角色 ID 静默通过，可能导致无效数据写入 user_role 表。修复为 `rid == 0` 时返回 `"角色 ID 不能为 0"` 错误，拒绝无效角色 ID。

## Auto Run Result

**Status:** done

### 实现变更摘要

将系统从单用户-单角色模型（`user.role_id`）改造为多用户-多角色模型（`user_role` 多对多关联表）：
- 后端：新增 `UserRole` 结构体与 `user_role` 表；`User` 移除 `RoleID` 字段，新增 `RoleIDs []uint` 临时字段；重写 `LoadPermissionCodes` 为多角色权限码并集去重加载；用户 CRUD 用事务保护 `user` 与 `user_role` 表一致性；启动迁移兼容全新部署与历史升级（检查 `role_id` 列存在性）；`/me` 接口填充 `roleIds`/`roleNames`；登录响应返回多角色数组；`validateRoleIDs` 拒绝零值与禁用角色。
- 前端：`UserRecord`/`ClientUserInfo` 类型从 `roleId/roleName` 改为 `roleIds/roleNames` 数组；用户表单新增多角色 Checkbox Group；用户列表角色列显示多个角色 Badge；角色穿梭框语义从"转移"改为"增删本角色"（不影响用户的其他角色）；`auth.tsx` 适配 `roleIds` 字段。

### 修改的文件

- `internal/openvpnweb/user.go` -- 新增 `UserRole` 结构体；`User` 移除 `RoleID` 改用 `RoleIDs []uint`；重写 `LoadPermissionCodes`（多角色并集）；`LoadRoleIDsAndNames` 错误时返回等长空切片；`Create/Update/Delete` 加事务保护与错误处理
- `internal/openvpnweb/role.go` -- 新增 `validateRoleIDs` 批量校验（拒绝 0/不存在/禁用）；`roleUsersHandler`/`roleAssignUsersHandler`/`roleListHandler`/`roleDeleteHandler` 适配 user_role 表
- `internal/openvpnweb/server.go` -- AutoMigrate 增加 `UserRole`；启动迁移检查 `role_id` 列存在性后再执行回填与迁移；POST/PATCH `/ovpn/user` 处理 `roleIds`；`/ovpn/user/me` 显式加载 `roleIds/roleNames`；登录响应返回多角色数组；移除 `hasRoleIDs` 死代码
- `frontend/src/types.ts` -- `UserRecord`/`ClientUserInfo` 改为 `roleIds?:number[]` / `roleNames?:string[]`
- `frontend/src/pages/Users/index.tsx` -- `UserFormDialog` 新增多角色 Checkbox Group；列表角色列显示多 Badge；提交 payload 含 `roleIds`
- `frontend/src/pages/Roles/index.tsx` -- `RoleAssignUser` 接口改数组；穿梭框移除转移警告，显示用户已有角色列表
- `frontend/src/store/auth.tsx` -- 适配 `roleIds` 字段，`hasPermission` 逻辑不变

### 审查发现分类

- patch: 6 项（high 2, medium 3, low 1）全部修复
- defer: 2 项（schema 死列 + 权限加载无缓存）已记录到 `deferred-work.md`
- reject: 0
- intent_gap: 0
- bad_spec: 0

### 验证结果

- `go build ./...` -- 通过（exit 0）
- `go vet ./...` -- 通过（exit 0）
- `npm run build`（frontend） -- 通过（exit 0，2921 模块转译，19.84s）
- 手动检查项：见 spec `Verification > Manual checks`（建议运行后端启动 + 手动验证多角色场景）

### 残留风险

1. **SQLite 死列**：历史升级部署的 `user.role_id` 列在迁移后仍保留（SQLite 无法 DROP COLUMN），但不影响功能。
2. **权限加载性能**：`LoadPermissionCodes` 每次请求执行 N+1 查询，当前规模无影响，高并发场景可加缓存。
3. **并发竞态**：`roleAssignUsersHandler` 与 `PATCH /ovpn/user` 并发修改同一用户的角色绑定时，事务隔离级别依赖底层 DB（SQLite 串行化，MySQL/PostgreSQL 需评估）。
4. **手动验证未覆盖**：本次仅验证编译与 vet，未启动后端实测迁移与多角色业务流程。建议手动验证 spec 的 9 项 Acceptance Criteria。

### Follow-up Review 推荐

**推荐**：本次审查修复了 2 个 high 严重度问题（迁移 SQL 兼容性 + 事务保护），涉及数据一致性与全新部署兼容性，影响面较大。事务保护改动影响 `User.Create/Update/Delete` 三个核心方法，建议独立 follow-up review 验证事务边界与错误处理正确性。
