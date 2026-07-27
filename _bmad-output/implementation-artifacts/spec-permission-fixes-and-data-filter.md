---
title: '权限修复与数据权限过滤'
type: 'bugfix'
created: '2026-07-27'
status: 'completed'
review_loop_iteration: 0
followup_review_recommended: false
context: []
warnings: ['multiple-goals']
---

<intent-contract>

## Intent

**Problem:** 用户报告了多个权限相关的问题：
1. 系统设置页面：普通用户未分配基础控制权限，但仍能看到该 Tab
2. 客户端模块：缺少查询按钮权限（`client:view`），GET /client 接口使用 `client:create` 权限不合理
3. 客户端模块：添加客户端按钮未做权限控制
4. 数据权限：查询数据时未按分组过滤，存在越权访问问题

**Approach:** 
1. 修复 Settings 页面 Tab 权限控制逻辑
2. 添加 `client:view` 权限并修正客户端相关接口的权限要求
3. 在客户端页面添加按钮权限控制
4. 实现基于分组的数据权限过滤，父节点只能查看自己及其下级的数据

## Boundaries & Constraints

**Always:**
- 所有代码、注释使用中文
- admin 用户拥有全部权限，不受数据权限限制
- 保留现有菜单权限和按钮权限的结构
- 数据权限基于用户所在分组（Gid）实现层级过滤

**Block If:**
- 无法确定分组层级结构
- 数据权限过滤影响核心功能

**Never:**
- 不影响 admin 用户的操作
- 不改变现有权限编码体系

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| 普通用户无 settings:base 权限 | 访问系统设置页面 | 不显示基础控制 Tab | - |
| 普通用户有 client:view 但无 client:create | 访问客户端页面 | 显示客户端列表，不显示添加按钮 | - |
| 普通用户无 client:view 权限 | 访问客户端页面 | 403 禁止访问 | - |
| 父节点用户查询用户列表 | 用户属于该父节点或子节点 | 只返回该分组及下级分组的用户 | - |
| 父节点用户查询客户端 | 客户端属于该父节点或子节点 | 只返回该分组及下级分组的客户端 | - |
| admin 用户查询 | 任意分组 | 返回所有数据 | - |

</intent-contract>

## Code Map

- `internal/openvpnweb/role.go` -- 添加 `client:view` 权限编码
- `internal/openvpnweb/server.go` -- 修改客户端相关接口权限要求，添加数据权限过滤逻辑
- `internal/openvpnweb/group.go` -- 添加分组树查询函数（获取节点及其所有子节点）
- `frontend/src/pages/Clients/index.tsx` -- 添加按钮权限控制（添加客户端、删除、编辑等）
- `frontend/src/pages/Settings/index.tsx` -- 验证 Tab 权限控制逻辑

## Tasks & Acceptance

**Execution:**
- [ ] `internal/openvpnweb/role.go` -- 添加 `client:view` 权限编码到按钮权限列表 -- 权限定义
- [ ] `internal/openvpnweb/server.go` -- 修改 GET /client 接口权限从 `client:create` 改为 `client:view` -- API权限修正
- [ ] `internal/openvpnweb/server.go` -- 修改 GET /client/:name/ccd 接口权限从 `client:create` 改为 `client:view` -- API权限修正
- [ ] `internal/openvpnweb/group.go` -- 添加 `GetSubtreeIDs` 函数获取分组及其所有子节点 ID -- 分组树查询
- [ ] `internal/openvpnweb/server.go` -- 在用户列表、客户端列表等查询接口中添加数据权限过滤 -- 数据权限实现
- [ ] `frontend/src/pages/Clients/index.tsx` -- 使用 HasPermission 包裹添加客户端按钮 -- 按钮权限控制
- [ ] `frontend/src/pages/Clients/index.tsx` -- 使用 HasPermission 包裹其他操作按钮（删除、编辑等） -- 按钮权限控制
- [ ] `frontend/src/pages/Settings/index.tsx` -- 验证 Tab 权限控制逻辑（已实现，需确认） -- 权限验证

**Acceptance Criteria:**
- Given 普通用户无 settings:base 权限，when 进入系统设置页面，then 不显示基础控制 Tab
- Given 普通用户有 client:view 但无 client:create，when 进入客户端页面，then 显示列表但不显示添加按钮
- Given 普通用户无 client:view 权限，when 访问客户端页面，then 返回 403
- Given 父节点用户，when 查询用户列表，then 只返回该分组及下级分组的用户
- Given 父节点用户，when 查询客户端列表，then 只返回该分组及下级分组的客户端
- Given admin 用户，when 查询任意数据，then 返回全部数据

## Design Notes

**权限编码修改：**
1. 在 `buttonPermissions` 中添加 `{"menu:clients", "client:view", "查看客户端", "button", "", "", 1}`
2. 修改 defaultUserRoleCodes 添加 `client:view` 权限

**分组树查询：**
1. 添加递归函数 `GetSubtreeIDs(id uint) []uint` 获取分组及其所有子节点 ID
2. 使用闭包递归遍历子节点

**数据权限过滤：**
1. 在查询接口中获取当前用户所在分组
2. 调用 `GetSubtreeIDs` 获取所有可访问的分组 ID
3. 查询时添加 `WHERE gid IN (...)` 条件
4. admin 用户跳过过滤

**前端按钮权限控制：**
1. 使用 `hasPermission` 检查权限
2. 使用 `HasPermission` 组件包裹按钮
3. 添加 `client:view`、`client:create`、`client:delete`、`client:regenerate` 权限控制

## Verification

**Commands:**
- `cd f:\develop\openvpn && go build ./...` -- expected: 编译通过
- `cd f:\develop\openvpn\frontend && npx tsc --noEmit` -- expected: 类型检查通过

**Manual checks:**
- 创建普通用户，仅分配 menu:clients 和 client:view 权限 -- 进入客户端页面 -- 显示列表 -- 添加按钮不可见
- 创建普通用户，分配 menu:clients 和 client:create 权限 -- 进入客户端页面 -- 添加按钮可见 -- 但无 client:view 权限无法访问列表（验证 API 权限）
- 创建分组层级（父分组A -> 子分组B -> 子分组C）-- 将用户分配到子分组B -- 登录该用户 -- 查询用户列表 -- 只看到子分组B和C的用户