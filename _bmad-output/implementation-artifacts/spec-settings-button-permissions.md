---
title: '系统设置Tab按钮权限控制'
type: 'feature'
created: '2026-07-27'
status: 'done'
review_loop_iteration: 0
baseline_revision: '60d947ce199ddf01c91d40c41ce38ada988cc012'
final_revision: '080160fe94a89df6f9dc3a7c700a44f29a40e7a7'
followup_review_recommended: false
context: []
warnings: []
---

<intent-contract>

## Intent

**Problem:** 系统设置页面各Tab下的操作按钮（如"保存全部"、"上传安装包"、"删除安装包"、"重启OpenVPN"、"编辑server.conf"等）仅依赖Tab查看权限，没有独立的按钮级权限控制，导致有Tab查看权限的用户可以执行所有操作，不符合最小权限原则。

**Approach:** 为每个Tab的操作按钮新增独立的权限编码，将保存权限从全局`settings:update`拆分为各Tab独立保存权限（`settings:base:update`、`settings:ldap:update`、`settings:openvpn:update`），并为服务管理和客户端包Tab的操作按钮新增独立权限（`settings:service:restart`、`settings:service:config`、`settings:packages:upload`、`settings:packages:delete`、`settings:packages:enable`）；后端对对应API路由追加权限校验，前端根据权限控制按钮显示。

## Boundaries & Constraints

**Always:**
- 所有代码、注释、commit message、文档使用中文
- 前端使用shadcn/ui + Tailwind组件，避免手写组件
- 后端响应消息UTF-8中文
- 权限编码遵循`资源:动作`格式
- admin用户始终拥有全部权限，所有权限检查对admin直接放行
- `settings:update`保留为兼容权限，拥有该权限的用户等价于拥有所有Tab的保存权限
- 保存按钮（SaveBar）仅当用户拥有至少一个Tab的保存权限时才显示
- 保存时仅提交用户有保存权限的Tab数据

**Block If:**
- 无法确定某个按钮应该映射到哪个权限编码

**Never:**
- 不改变现有Tab查看权限（settings:base等）的语义
- 不修改系统设置的保存接口路径（仍为POST /ovpn/settings）
- 不引入新的角色或修改现有角色权限结构（仅更新seed权限清单）
- 不改变客户端包管理API路径

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| 用户有settings:base:update但无settings:ldap:update | 修改base和ldap后点击保存 | 仅保存base数据，ldap修改被忽略 | - |
| 用户有settings:update但无任何Tab:update权限 | 点击保存 | 等价于拥有所有Tab:update权限，全部保存 | - |
| 用户无任何Tab:update权限 | 查看设置页面 | 不显示保存按钮 | - |
| 用户有settings:packages:upload权限 | 点击上传安装包 | 上传按钮可见，可上传 | - |
| 用户无settings:packages:upload权限 | 查看客户端包Tab | 上传按钮隐藏 | - |
| 用户无settings:service:restart权限 | 查看服务管理Tab | 重启按钮隐藏 | - |
| 用户直接POST /ovpn/client-packages | 无settings:packages:upload权限 | 返回403 | - |
| 用户直接POST /ovpn/server(action=restartSrv) | 无settings:service:restart权限 | 返回403 | - |
| admin用户 | 访问任意操作 | 全部放行 | - |

</intent-contract>

## Code Map

- `internal/openvpnweb/role.go` -- 权限定义与seed函数，新增8个按钮权限编码，更新user角色默认权限
- `internal/openvpnweb/server.go` -- 系统设置相关路由，POST /ovpn/settings按Tab保存权限校验，/ovpn/server和/ovpn/client-packages追加按钮权限
- `frontend/src/pages/Settings/index.tsx` -- 系统设置页面，SaveBar按Tab保存权限控制显示，各操作按钮按权限控制显示
- `frontend/src/store/auth.tsx` -- hasPermission函数已支持任意权限编码，无需修改
- `frontend/src/components/HasPermission.tsx` -- 按钮级权限包裹组件已存在，直接使用

## Tasks & Acceptance

**Execution:**
- [x] `internal/openvpnweb/role.go` -- 在SeedPermissionsAndRoles中新增8个按钮权限编码：settings:base:update、settings:ldap:update、settings:openvpn:update（parent为settings:base/ldap/openvpn）；settings:service:restart、settings:service:config（parent为settings:service）；settings:packages:upload、settings:packages:delete、settings:packages:enable（parent为settings:packages） -- 权限基础
- [x] `internal/openvpnweb/role.go` -- 更新user角色默认权限：不添加任何Tab:update权限和操作按钮权限（普通用户仅查看不操作） -- 普通用户默认只读
- [x] `internal/openvpnweb/server.go` -- POST /ovpn/settings接口：对非admin用户，仅接受用户拥有对应Tab:update权限的字段（如无settings:base:update则忽略system.base.*字段）；保留settings:update兼容（有该权限等价于所有Tab:update） -- 后端保存权限过滤
- [x] `internal/openvpnweb/server.go` -- POST /ovpn/server(action=restartSrv)追加RequirePermission("settings:service:restart")；POST /ovpn/server(action=getConfig/updateConfig)追加RequirePermission("settings:service:config") -- 服务管理按钮权限
- [x] `internal/openvpnweb/server.go` -- POST /ovpn/client-packages追加RequirePermission("settings:packages:upload")；DELETE /ovpn/client-packages/:id追加RequirePermission("settings:packages:delete")；POST /ovpn/client-packages/:id/enable追加RequirePermission("settings:packages:enable") -- 客户端包按钮权限
- [x] `frontend/src/pages/Settings/index.tsx` -- SaveBar显示条件改为：拥有settings:update或至少一个Tab:update权限；保存时仅提交有权限的Tab数据（通过检查hasPermission过滤dirtyKeys） -- 保存按钮控制
- [x] `frontend/src/pages/Settings/index.tsx` -- ServiceTab中"重启OpenVPN"按钮用HasPermission包裹（code="settings:service:restart"）；"编辑server.conf"按钮用HasPermission包裹（code="settings:service:config"） -- 服务管理按钮控制
- [x] `frontend/src/pages/Settings/index.tsx` -- ClientPackagesTab中"上传安装包"按钮用HasPermission包裹（code="settings:packages:upload"）；"删除"按钮用HasPermission包裹（code="settings:packages:delete"）；"启用"按钮用HasPermission包裹（code="settings:packages:enable"） -- 客户端包按钮控制

**Acceptance Criteria:**
- Given 用户有settings:base:update权限，when 修改base和ldap字段后点击保存，then 仅base字段被提交保存，ldap修改被忽略
- Given 用户仅有settings:base:view权限无任何:update权限，when 访问设置页面，then 保存按钮不显示
- Given 用户有settings:update权限，when 点击保存，then 所有Tab数据被保存（兼容模式）
- Given 用户无settings:packages:upload权限，when 查看客户端包Tab，then 上传按钮隐藏
- Given 用户无settings:packages:delete权限，when 查看安装包列表，then 删除按钮隐藏
- Given 用户无settings:service:restart权限，when 查看服务管理Tab，then 重启按钮隐藏
- Given 用户直接POST /ovpn/client-packages，when 无settings:packages:upload权限，then 返回403
- Given admin用户，when 访问任意Tab，then 所有按钮可见可操作

## Spec Change Log

（空，由step-04在review loopback时追加）

## Review Triage Log

### 2026-07-27 — Review pass
- intent_gap: 0
- bad_spec: 0
- patch: 6: (high 1, medium 4, low 1)
- defer: 8
- reject: 0
- addressed_findings:
  - [high] [patch] GET /client-packages权限与settings:packages不一致：默认user角色有settings:packages但API需要client:manage_all，改为settings:packages权限
  - [medium] [patch] 默认user角色缺少menu:settings和settings:view，Tab权限形同虚设：添加这两个权限到defaultUserRoleCodes
  - [medium] [patch] POST /settings在所有字段被权限过滤后仍返回200"更新成功"：添加savedCount检查，无字段通过时返回403
  - [medium] [patch] saveableDirtyKeys未过滤service.auth_user：添加service.auth_user的权限检查（需要server:manage或settings:update）
  - [medium] [patch] canShowSaveBar未考虑service tab：用户有service tab权限但无base/ldap/openvpn保存权限时SaveBar不显示
  - [low] [patch] defaultTab逻辑：无Tab权限时fallback到'packages'而非'base'

## Design Notes

**新增权限编码清单（8项，均为button类型）：**

```
settings:base:update      保存基础控制          parent: settings:base
settings:ldap:update      保存LDAP认证          parent: settings:ldap
settings:openvpn:update   保存OpenVPN参数       parent: settings:openvpn
settings:service:restart   重启OpenVPN服务      parent: settings:service
settings:service:config    编辑server.conf       parent: settings:service
settings:packages:upload   上传安装包            parent: settings:packages
settings:packages:delete   删除安装包            parent: settings:packages
settings:packages:enable   启用安装包            parent: settings:packages
```

**`settings:update`兼容逻辑：**
- 后端：POST /ovpn/settings中，若用户有settings:update权限，等价于拥有所有Tab:update权限
- 前端：SaveBar显示条件为 `hasPermission('settings:update') || hasPermission('settings:base:update') || hasPermission('settings:ldap:update') || hasPermission('settings:openvpn:update')`

**后端保存过滤逻辑示例：**
```go
canSaveBase := hasPerm("settings:base:update") || hasPerm("settings:update")
canSaveLdap := hasPerm("settings:ldap:update") || hasPerm("settings:update")
canSaveOvpn := hasPerm("settings:openvpn:update") || hasPerm("settings:update")
// 遍历PostForm，仅保留用户有保存权限的Tab对应字段的key
```

**前端SaveBar保存逻辑：**
```tsx
const canSaveBase = hasPermission('settings:update') || hasPermission('settings:base:update');
const canSaveLdap = hasPermission('settings:update') || hasPermission('settings:ldap:update');
const canSaveOvpn = hasPermission('settings:update') || hasPermission('settings:openvpn:update');

// 过滤dirtyKeys：仅保留有保存权限的Tab字段
const filteredDirtyKeys = dirtyKeys.filter(key => {
  if (key.startsWith('system.base.') && !canSaveBase) return false;
  if (key.startsWith('system.ldap.') && !canSaveLdap) return false;
  if (key.startsWith('openvpn.') && !canSaveOvpn) return false;
  return true;
});
```

## Verification

**Commands:**
- `cd f:\develop\openvpn && go build ./...` -- expected: 编译通过，无错误
- `cd f:\develop\openvpn\frontend && npm run build` -- expected: 前端构建通过

**Manual checks:**
- admin登录 → 设置页面 → 所有Tab可见，所有按钮可见可操作
- 创建普通用户（user角色）→ 登录 → 访问设置 → 仅"基础控制"和"客户端包"Tab可见，保存按钮隐藏，上传/删除/启用按钮隐藏
- admin编辑user角色，添加settings:base:update → 普通用户重新登录 → 保存按钮出现，仅可保存base Tab数据
- admin创建角色仅勾选settings:packages:upload → 创建用户绑定 → 登录 → 客户端包Tab可见，上传按钮可见，删除/启用按钮隐藏
- 直接curl POST /ovpn/client-packages无权限 → 返回403
- 直接curl POST /ovpn/server(action=restartSrv)无权限 → 返回403

## Auto Run Result

**Status:** done

### Summary of Implemented Change

实现了系统设置页面各Tab下操作按钮的权限控制：

**新增8个按钮权限编码：**
- 3个Tab保存权限：`settings:base:update`、`settings:ldap:update`、`settings:openvpn:update`
- 2个服务管理按钮权限：`settings:service:restart`、`settings:service:config`
- 3个客户端包按钮权限：`settings:packages:upload`、`settings:packages:delete`、`settings:packages:enable`

**后端：**
- POST /ovpn/settings 按Tab保存权限过滤字段，`settings:update`作为兼容通配
- POST /ovpn/server 按action分派权限（restart→`settings:service:restart`，config→`settings:service:config`）
- 客户端包API（上传/删除/启用）分别追加对应权限校验
- GET /client-packages权限从`client:manage_all`改为`settings:packages`
- 所有字段被权限过滤后返回403而非200
- CIDR/IP校验错误码从500改为400

**前端：**
- SaveBar根据Tab保存权限控制显示
- dirtyKeys按保存权限过滤后提交
- 服务管理Tab的"重启"和"编辑server.conf"按钮用HasPermission包裹
- 客户端包Tab的"上传/删除/启用"按钮用HasPermission包裹
- service.auth_user保存需要server:manage或settings:update权限
- defaultTab逻辑修复

### Files Changed

- `internal/openvpnweb/role.go` — 新增8个按钮权限编码，添加hasPermissionCode/requirePermissionCode辅助函数，更新user角色默认权限（添加menu:settings和settings:view）
- `internal/openvpnweb/server.go` — POST /settings按Tab权限过滤字段，/ovpn/server和/ovpn/client-packages追加按钮权限，GET /client-packages权限改为settings:packages，savedCount检查，CIDR/IP错误码修复
- `frontend/src/pages/Settings/index.tsx` — SaveBar显示逻辑、saveableDirtyKeys过滤、HasPermission包裹各操作按钮、defaultTab修复

### Review Findings Breakdown

- patch: 6项已修复（high 1, medium 4, low 1）
- defer: 8项（预存在的设计问题，非本次引入）
- reject: 0项
- intent_gap: 0项
- bad_spec: 0项

### Follow-up Review Recommendation

**false** — 6项patch均为权限一致性修复和边界条件处理。defer项为预存在的设计问题（如service.auth_user的权限模型、审计日志缺失等），非本次变更引入，可后续单独处理。

### Verification Performed

- `go build ./...` — 编译通过（EXIT_CODE=0）
- `npx tsc --noEmit` — 前端类型检查通过（EXIT_CODE=0）
- 手动代码审查确认权限过滤逻辑完整

### Residual Risks

- service.auth_user的保存仍走POST /ovpn/server(action=settings)，需要`server:manage`权限而非`settings:service:*`权限（预存在设计，已在saveableDirtyKeys中做兼容过滤）
- system.email.*字段仍需要`settings:update`权限，未拆分为细粒度权限（超出本次范围）
- 审计日志在成功路径上仍缺失（预存在问题）