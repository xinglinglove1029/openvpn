# Deferred Work

收集审查中发现的非本次故事引入、但值得后续关注的问题。

- source_spec: `_bmad-output/implementation-artifacts/spec-role-binding-users-groups.md`
  summary: roleAssignGroupsHandler 未拒绝把组绑定到内置 administrator 角色，存在理论上的权限提升路径（需 role:assign_groups + user:create 权限组合）
  evidence: spec 第 91 行明确设计"分配用户组按钮正常可用"，Blind Hunter 指出拥有 role:assign_groups + user:create 权限的管理员可创建组绑定到 administrator 角色，再创建用户到该组继承超管角色。当前 spec 设计允许此行为，属于设计权衡而非实现 bug。如未来需要收紧，可在 roleAssignGroupsHandler 增加 administrator 角色拒绝检查。

- source_spec: `_bmad-output/implementation-artifacts/spec-role-binding-users-groups.md`
  summary: roleUsersHandler 返回已禁用用户但穿梭框无禁用状态标识
  evidence: roleUsersHandler 查询未过滤 is_enable=false，穿梭框中包含已禁用用户但前端条目只显示用户名+组名，管理员无法区分启用/禁用状态。属于 UX 增强，spec 未要求。

- source_spec: `_bmad-output/implementation-artifacts/spec-role-binding-users-groups.md`
  summary: roleAssignGroupsHandler 无法清理 Default 组（ID=1）的历史脏数据 role_id
  evidence: step 1 和 step 2 都加了 `AND id != 1` 保护 Default 组。如果 Default 组的 role_id 已被外部 SQL 写入非空值，本接口不会清理它。Group.Update() 也拒绝修改 Default 组 role_id，无任何清理路径。建议启动时或迁移脚本中检查并清理。

- source_spec: `_bmad-output/implementation-artifacts/spec-role-binding-users-groups.md`
  summary: 前端 collectLeafIds/collectSubtreeIds 为 O(n²) 性能，大树可能卡顿
  evidence: collectSubtreeIds 每次调用都 allGroups.filter(...)，递归调用；renderGroupNode 内部又 filter 子节点。对 N 个节点的树整体约 O(N²)。用户组通常几十个，实际影响不大，但大树会有性能问题。建议预计算 parentToChildren Map。

- source_spec: `_bmad-output/implementation-artifacts/spec-role-binding-users-groups.md`
  summary: GroupAssignDialog 中 collectSubtreeIds/collectLeafIds 无循环 parentId 兜底
  evidence: 递归遍历子节点无 visited set 保护。Group.BeforeCreate 阻止了"父节点是自己"，但未阻止多层循环（数据被直接写库或迁移历史数据可能触发）。低风险但建议加 visited set 防止栈溢出。

- source_spec: `_bmad-output/implementation-artifacts/spec-role-binding-users-groups.md`
  summary: roleAssignUsersHandler/roleAssignGroupsHandler 并发同时分配无显式锁
  evidence: 事务内两次 UPDATE 之间无 SELECT ... FOR UPDATE 锁定。SQLite 默认串行化通常无问题，MySQL/PostgreSQL 多实例场景有 last-write-wins 风险。已通过事务内重新查询角色缩小 TOCTOU 窗口，但未完全消除。

- source_spec: `_bmad-output/implementation-artifacts/spec-role-binding-users-groups.md`
  summary: 角色用户/用户组变更未通过 WebSocket 推送，其他管理员需手动刷新
  evidence: 项目已有 EventBus + WsHub 机制（Bus().Publish(topic, payload)），但本次 4 个新 handler 与 roleDeleteHandler 均未调用。spec 未明确要求实时推送，属于增强项。

- source_spec: `_bmad-output/implementation-artifacts/spec-role-binding-users-groups.md`
  summary: PATCH /ovpn/user 接口可绕过 role:assign_users 权限修改用户角色，存在权限提升路径
  evidence: PATCH /ovpn/user 仅要求 user:update 权限，但通过 ShouldBind 绑定整个 User 结构（含 RoleID *uint），持有 user:update 但无 role:assign_users 权限的操作员可修改任意用户（含自己）的 role_id。validateRoleID 仅校验角色存在且启用，不校验调用者是否有权分配。pre-existing 问题，非本次变更引入，但与角色绑定主题高度相关。

- source_spec: `_bmad-output/implementation-artifacts/spec-role-binding-users-groups.md`
  summary: roleAssignUsersHandler/roleAssignGroupsHandler 缺乏角色层级约束，可分配到权限超过自身的角色
  evidence: 任意持有 role:assign_users 的用户可把任意用户分配到任意已存在且启用的角色（包括权限超过自身的角色）。后端无"目标角色权限必须为调用者权限子集"的约束。这是 RBAC 系统的整体设计问题，role:assign_permissions 也存在同样的模式。pre-existing 设计局限。

- source_spec: `_bmad-output/implementation-artifacts/spec-role-binding-users-groups.md`
  summary: roleDeleteHandler 在 MySQL/PostgreSQL 下存在 TOCTOU 竞态，可产生孤儿 role_id
  evidence: 代码注释已承认 Count 不加写锁，在 READ COMMITTED 隔离级别下，并发 roleAssignUsersHandler 事务可在 Count 与 Delete 之间提交 UPDATE，造成用户/组 role_id 指向已删除角色。roleAssignUsers/GroupsHandler 也有类似的 last-write-wins 风险。需统一引入 SELECT ... FOR UPDATE 或乐观锁方案。

- source_spec: `_bmad-output/implementation-artifacts/spec-role-binding-users-groups.md`
  summary: roleDeleteHandler 与 roleAssignPermissionsHandler 未使用 parseRoleIDParam，输入校验不一致
  evidence: 这两个旧 handler 直接 c.Param("id") 后 db.Where("id = ?", id).First(&role)，未先 parseRoleIDParam 转为 uint。若 :id 为非数字字符串，不同 DB 驱动行为不一。本次新增的 4 个 handler 均使用了 parseRoleIDParam。pre-existing 不一致。

- source_spec: `_bmad-output/implementation-artifacts/spec-role-binding-users-groups.md`
  summary: 审计日志缺乏"谁被分配到哪个角色"的细节，仅记录目标角色名
  evidence: recordAudit(c, "role", "assign_users", role.Name, true, "分配角色用户: "+role.Name) 未记录被加入/移出的用户 ID 列表。assign_groups 同理。对于 RBAC 敏感操作，审计日志无法回答"操作员 X 在时间 T 把用户 Y 从角色 A 移到角色 B"。建议在 message 中加入 diff（added/removed ID 列表）。增强型改进。

- source_spec: `_bmad-output/implementation-artifacts/spec-role-binding-users-groups.md`
  summary: roleUsersHandler / roleGroupsHandler 无分页，全量返回用户/组清单
  evidence: db.Find(&users) / db.Find(&groups) 无 LIMIT，返回全部非 admin 用户/全部组。在 10k+ 用户的部署中，单次请求可产生数 MB JSON，前端穿梭框渲染卡顿。性能优化项，当前部署规模下影响不大。

- source_spec: `_bmad-output/implementation-artifacts/spec-role-binding-users-groups.md`
  summary: 5xx 错误直接回显 GORM/DB 原始错误信息，可能泄露 schema
  evidence: c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()}) 将底层 DB 错误原文返回。全代码库通病，非本 diff 独有，但新代码延续了该模式。建议对 5xx 错误统一返回"服务器内部错误"，原文写日志。

- source_spec: `_bmad-output/implementation-artifacts/spec-multi-role-per-user.md`
  summary: user_role 表迁移后 user.role_id 列仍保留为死列，SQLite 无法 DROP COLUMN
  evidence: SQLite 不支持 DROP COLUMN（除非重建表）。User 结构体已移除 RoleID 字段，GORM 不再读写该列，但历史升级部署的 user 表中 role_id 列仍保留为死列。启动迁移通过 pragma_table_info 检查列存在性后复制数据到 user_role 表，但不会清理原列。属于已知 SQLite 限制，对功能无影响，仅 schema 不整洁。如未来切换到 MySQL/PostgreSQL 可考虑清理。

- source_spec: `_bmad-output/implementation-artifacts/spec-multi-role-per-user.md`
  summary: AuthMiddleWare 每次请求都查 user_role 表加载权限码，无缓存
  evidence: LoadPermissionCodes 每次请求执行 `SELECT role_id FROM user_role WHERE user_id = ?` 然后逐角色 `LoadRolePermissionCodes`，N+1 查询模式。当前部署规模下影响不大，但高并发场景下可考虑加内存缓存（如用户角色变更时失效）。pre-existing 性能模式，本次改造未恶化。
