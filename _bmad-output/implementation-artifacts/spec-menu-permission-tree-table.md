---
title: '菜单与按钮权限管理（Tree Table）'
type: 'feature'
created: '2026-07-28'
status: 'in-review'
review_loop_iteration: 0
followup_review_recommended: false
context: []
warnings: []
baseline_revision: '69d119d'
---

<intent-contract>

## Intent

**Problem:** 当前菜单和按钮权限仅通过代码 seed 初始化，无法在运行时调整菜单顺序、名称或路径。侧边栏菜单项硬编码在前端，调整顺序需要改代码重新部署。

**Approach:** 新增一个权限管理页面，以 Tree Table 展示全部菜单和按钮权限，支持增删改查和排序调整。后端暴露 Permission CRUD API，前端侧边栏改为从 `/ovpn/permission/tree` 动态渲染。

## Boundaries & Constraints

**Always:**
- 权限树的 ParentID/Sort 字段驱动树结构和显示顺序
- 删除菜单节点时级联删除其子节点（按钮）及关联的 RolePermission 记录
- 侧边栏仅展示 `type=menu` 且用户拥有对应权限码的节点，按 sort 排序
- 内置权限（code 以 `menu:` 或已 seed 的按钮权限）的 code 字段不可修改，不可删除
- 新增权限节点时 code 必须唯一且符合格式校验（字母开头，仅字母数字下划线冒号）
- 保留 `SeedPermissionsAndRoles` 的 seed 逻辑，启动时同步元数据但不覆盖运行时 sort/parentID 修改

**Block If:**
- 无

**Never:**
- 不改动角色管理页面的权限分配树组件（复用现有 PermissionTree 组件）
- 不引入拖拽排序库（使用上移/下移按钮调整顺序）
- 不修改 `RequirePermission` 中间件逻辑

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| 新增菜单节点 | name/code/path/icon/sort/parentID=0 | 创建成功，返回新节点 | code 重复时返回 409 |
| 新增按钮节点 | name/code/sort/parentID=<menuID> | 创建成功，挂到指定菜单下 | parentID 不存在时返回 400 |
| 调整排序 | PUT /permission/sort [{id, sort}, ...] | 批量更新 sort 字段 | 部分失败时整体回滚 |
| 删除菜单节点 | DELETE /permission/:id | 删除该节点及其所有子节点和 RolePermission 关联 | 无权限返回 403 |
| 修改内置权限 | PUT /permission/:id 且 code 以 menu: 开头 | 允许修改 name/path/icon/sort，不允许修改 code 和 type | 尝试改 code 时返回 400 |
| 侧边栏动态渲染 | 用户登录，permissions 含 menu:* | 按 sort 顺序展示有权限的菜单 | API 失败时 fallback 到空菜单 |

</intent-contract>

## Code Map

- `internal/openvpnweb/role.go` -- Permission 模型定义、seed 数据、permissionTreeHandler、现有 CRUD handler
- `internal/openvpnweb/server.go` -- 路由注册，新增 permission CRUD 路由
- `internal/openvpnweb/audit.go` -- 审计日志 permission seed 数据
- `frontend/src/layout/Sidebar.tsx` -- 硬编码菜单列表，改为动态渲染
- `frontend/src/layout/Layout.tsx` -- 路由权限映射，改为动态
- `frontend/src/App.tsx` -- 路由注册，新增 /permissions 路由
- `frontend/src/types.ts` -- PermissionTreeNode 类型定义
- `frontend/src/pages/Permissions/index.tsx` -- 新增权限管理页面（Tree Table）

## Tasks & Acceptance

**Execution:**
- [x] `internal/openvpnweb/role.go` -- 新增 `permissionCreateHandler`、`permissionUpdateHandler`、`permissionDeleteHandler`、`permissionSortHandler` 四个 handler；新增 `Permission` 模型的 `Delete` 方法（级联删除子节点和 RolePermission）；在 seed 数据中添加 `menu:permissions` 菜单和 `permission:manage` 按钮 -- 支持权限 CRUD 和排序
- [x] `internal/openvpnweb/server.go` -- 注册 `POST /permission`、`PUT /permission/:id`、`DELETE /permission/:id`、`PUT /permission/sort` 四条路由，均加 `RequirePermission("permission:manage")` -- 暴露 CRUD API
- [x] `frontend/src/pages/Permissions/index.tsx` -- 新建 Tree Table 页面：展示权限树（Name/Code/Type/Path/Icon/Sort/Actions），支持新增子节点、编辑、删除、上移/下移排序 -- 核心功能页面
- [x] `frontend/src/layout/Sidebar.tsx` -- 移除硬编码 `allNavItems`，改为从 `/ovpn/permission/tree` 拉取菜单节点，按 sort 排序，用 icon 名称动态映射 lucide 图标组件 -- 动态侧边栏
- [x] `frontend/src/layout/Layout.tsx` -- `pathPermissionMap` 改为动态：从 permission tree 中 `type=menu` 的节点构建 path→code 映射 -- 动态路由守卫
- [x] `frontend/src/App.tsx` -- 新增 `PermissionsPage` lazy import 和 `/permissions` 路由 -- 页面注册
- [x] `frontend/src/types.ts` -- `PermissionTreeNode` 已有定义，无需修改

**Acceptance Criteria:**
- Given 管理员登录，when 访问 /permissions，then 看到 Tree Table 展示所有菜单和按钮权限节点
- Given 权限管理页面，when 点击上移/下移按钮调整排序后保存，then 侧边栏菜单顺序随之变化
- Given 权限管理页面，when 新增一个菜单节点并分配给角色，then 该角色用户侧边栏出现新菜单项
- Given 权限管理页面，when 删除一个菜单节点，then 其下所有按钮节点和 RolePermission 关联也被删除
- Given 已登录用户，when 侧边栏加载，then 菜单项按 DB 中的 sort 字段排序展示

## Design Notes

侧边栏动态渲染方案：前端启动时（或用户登录后）调用 `GET /ovpn/permission/tree` 获取完整权限树，缓存到 auth store 中。Sidebar 组件从 store 读取 `type=menu` 的根节点，按 sort 排序后渲染。图标通过 `icon` 字段（字符串）映射到 lucide-react 组件，使用动态 import 或预建映射表。

Tree Table 实现：使用 shadcn Table 组件构建，通过递归渲染子行，缩进表示层级。每行有上移/下移按钮（调整 sort）、新增子节点、编辑、删除操作。排序变更后调用 `PUT /permission/sort` 批量保存。

内置权限保护：`Permission` 模型添加 `IsBuiltin bool` 字段（seed 时设为 true）。内置权限不可删除、不可改 code 和 type，但可以改 name/path/icon/sort。

## Verification

**Commands:**
- `cd f:\develop\openvpn && go build ./...` -- expected: 编译成功无错误
- `cd f:\develop\openvpn\frontend && npx tsc --noEmit` -- expected: TypeScript 类型检查通过

**Manual checks:**
- 访问 /permissions 页面，确认 Tree Table 正确展示所有权限节点
- 调整菜单排序后刷新页面，确认顺序持久化
- 侧边栏菜单顺序与权限树中的 sort 一致
