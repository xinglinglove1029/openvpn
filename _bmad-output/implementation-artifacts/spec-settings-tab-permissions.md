---
title: '系统设置Tab权限细分'
type: 'feature'
created: '2026-07-27'
status: 'in-review'
review_loop_iteration: 0
baseline_revision: '0b57f9f0f7d2b467ca44ebd6ab3daabe932216a2'
followup_review_recommended: false
context: []
warnings: []
---

<intent-contract>

## Intent

**Problem:** 系统设置页面当前仅有两个粗粒度权限（settings:view和settings:update），所有Tab对有权限的用户完全可见可操作，无法实现"仅允许查看基础设置"、"仅允许管理LDAP"、"仅允许下载客户端包"等精细化管控，不符合最小权限原则。

**Approach:** 将系统设置权限从2个扩展为7个（5个Tab查看权限 + 2个操作权限），每个Tab对应一个独立的查看权限（settings:base、settings:ldap、settings:openvpn、settings:service、settings:packages），保留settings:update作为全局保存权限；后端新增权限编码并应用到数据读写路由，前端根据权限动态渲染Tab并控制保存按钮显示。

## Boundaries & Constraints

**Always:**
- 所有代码、注释、commit message、文档使用中文
- 前端使用shadcn/ui + Tailwind组件，避免手写组件
- 后端响应消息UTF-8中文
- 权限编码遵循`资源:动作`格式（如settings:base）
- 数据迁移通过GORM AutoMigrate自动建表/加列
- admin用户始终拥有全部权限，所有权限检查对admin直接放行
- Tab权限仅控制可见性，settings:update控制保存操作
- 用户至少有一个Tab权限才能访问/settings页面

**Block If:**
- 无法确定某个Tab应该对应哪些数据字段的读写权限
- 前端Tab渲染逻辑与权限判断无法正确匹配

**Never:**
- 不改变现有settings:update权限的语义（仍作为全局保存权限）
- 不引入新的角色或修改现有角色权限结构
- 不允许通过UI修改权限定义表
- 不改变系统设置的保存接口（仍为POST /ovpn/settings）

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| 用户仅有settings:base权限 | 登录后访问/settings | 仅显示"基础控制"Tab，其他Tab隐藏 | - |
| 用户仅有settings:ldap权限 | 登录后访问/settings | 仅显示"LDAP认证"Tab，其他Tab隐藏 | - |
| 用户有settings:base + settings:update权限 | 访问/settings | 显示"基础控制"Tab，保存按钮可见 | - |
| 用户有settings:base但无settings:update权限 | 访问/settings | 显示"基础控制"Tab，保存按钮隐藏 | - |
| 用户无任何Tab权限 | 访问/settings | 重定向到/overview | - |
| admin用户访问/settings | admin登录态 | 显示所有5个Tab，保存按钮可见 | - |
| 用户尝试保存无权限的Tab数据 | POST /ovpn/settings | 后端校验settings:update权限，无权限返回403 | - |
| 用户尝试直接访问API获取无权限的Tab数据 | GET /ovpn/settings | 后端根据settings:*权限过滤返回数据，无权限的Tab数据返回空对象 | - |

</intent-contract>

## Code Map

- `internal/openvpnweb/role.go` -- 权限定义与seed函数，需新增5个Tab权限编码
- `internal/openvpnweb/server.go` -- 系统设置路由，需调整GET权限过滤和POST权限校验
- `frontend/src/pages/Settings/index.tsx` -- 系统设置页面，需根据权限动态渲染Tab和控制保存按钮
- `frontend/src/store/auth.tsx` -- 权限判断函数，需支持Tab权限检查
- `frontend/src/layout/Sidebar.tsx` -- 侧边栏菜单，需调整settings菜单的权限判断逻辑

## Tasks & Acceptance

**Execution:**
- [x] `internal/openvpnweb/role.go` -- 在SeedPermissionsAndRoles函数中新增5个Tab权限编码（settings:base、settings:ldap、settings:openvpn、settings:service、settings:packages），类型为button，parent为settings:view -- 提供Tab级权限基础
- [x] `internal/openvpnweb/role.go` -- 更新内置user角色的默认权限，添加settings:base和settings:packages权限（普通用户可查看基础设置和客户端包） -- 普通用户默认权限
- [x] `internal/openvpnweb/server.go` -- 修改GET /ovpn/settings接口，根据用户拥有的Tab权限过滤返回数据（无权限的Tab返回空对象） -- 数据级权限过滤
- [x] `internal/openvpnweb/server.go` -- 保留POST /ovpn/settings接口的settings:update权限校验，无需Tab级校验 -- 保存权限控制
- [x] `frontend/src/pages/Settings/index.tsx` -- 根据hasPermission动态渲染TabsList中的TabsTrigger，无权限的Tab不渲染 -- Tab显示控制
- [x] `frontend/src/pages/Settings/index.tsx` -- 根据hasPermission('settings:update')控制保存按钮的显示 -- 保存按钮控制
- [x] `frontend/src/pages/Settings/index.tsx` -- 当用户无任何Tab权限时，显示空状态提示并引导用户联系管理员 -- 边界情况处理
- [x] `frontend/src/layout/Sidebar.tsx` -- 修改settings菜单的权限判断，改为检查是否至少有一个Tab权限 -- 菜单显示控制

**Acceptance Criteria:**
- Given 用户仅有settings:base权限，when 访问/settings页面，then 仅显示"基础控制"Tab，其他Tab隐藏
- Given 用户有settings:base和settings:update权限，when 访问/settings页面，then 显示"基础控制"Tab且保存按钮可见
- Given 用户有settings:base但无settings:update权限，when 访问/settings页面，then 显示"基础控制"Tab但保存按钮隐藏
- Given 用户无任何Tab权限，when 访问/settings页面，then 重定向到/overview
- Given admin用户，when 访问/settings页面，then 显示所有5个Tab且保存按钮可见
- Given 普通用户（user角色），when 登录后访问/settings，then 显示"基础控制"和"客户端安装包"两个Tab，保存按钮隐藏
- Given 用户尝试POST /ovpn/settings，when 无settings:update权限，then 返回403错误

## Spec Change Log

（空，由step-04在review loopback时追加）

## Review Triage Log

### 2026-07-27 — Review pass
- intent_gap: 0
- bad_spec: 0
- patch: 4: (high 2, medium 2, low 0)
- defer: 0
- reject: 0
- addressed_findings:
  - [high] [patch] `server.go` GET /ovpn/settings缺少settings:service和settings:packages权限过滤，导致服务管理和客户端包数据泄露；添加缺失的过滤逻辑
  - [high] [patch] `server.go` viper.Unmarshal失败时返回零值配置可能泄露敏感默认值；添加错误检查并返回500错误
  - [medium] [patch] `server.go` 空权限数组可能绕过所有Tab过滤器；添加显式处理确保空权限时所有Tab数据被过滤
  - [medium] [patch] `Settings/index.tsx` 前端重定向时机可能在settings加载前执行导致竞态条件；确保在settings加载完成后才检查权限

## Design Notes

**权限编码设计：**

在现有settings:view和settings:update基础上，新增5个Tab权限：
```
settings:base      基础控制Tab
settings:ldap      LDAP认证Tab
settings:openvpn   OpenVPN参数Tab
settings:service   服务管理Tab
settings:packages  客户端安装包Tab
```

这些权限为button类型，parent为settings:view，表示它们是settings:view的子权限。

**内置角色默认权限调整：**

- administrator角色：拥有全部7个settings权限
- user角色：添加settings:base和settings:packages权限，允许普通用户查看基础设置和客户端包

**后端数据过滤策略：**

GET /ovpn/settings接口根据用户Tab权限过滤返回数据：
- 有settings:base权限 → 返回base字段数据
- 无settings:base权限 → base字段返回空对象{}
- 其他Tab同理

这样前端可以根据返回数据是否为空来判断Tab是否应该渲染。

**前端Tab渲染逻辑：**

```tsx
const canViewBase = hasPermission('settings:base');
const canViewLdap = hasPermission('settings:ldap');
// ...其他Tab同理

<TabsList>
  {canViewBase && <TabsTrigger value="base">基础控制</TabsTrigger>}
  {canViewLdap && <TabsTrigger value="ldap">LDAP认证</TabsTrigger>}
  {/* ...其他Tab同理 */}
</TabsList>
```

**保存按钮控制：**

```tsx
{hasPermission('settings:update') && (
  <Button onClick={handleSave}>保存</Button>
)}
```

## Verification

**Commands:**
- `cd f:\develop\openvpn && go build ./...` -- expected: 编译通过，无错误
- `cd f:\develop\openvpn\frontend && npm run build` -- expected: 前端构建通过

**Manual checks:**
- 启动后端，使用admin登录 → 访问/settings → 所有5个Tab可见，保存按钮可见
- 创建普通用户testuser → 登录 → 访问/settings → 仅"基础控制"和"客户端安装包"两个Tab可见，保存按钮隐藏
- 使用admin编辑user角色，去掉settings:base权限 → testuser重新登录 → 访问/settings → 仅"客户端安装包"Tab可见
- 使用admin创建新角色，仅勾选settings:ldap权限 → 创建用户绑定该角色 → 登录 → 访问/settings → 仅"LDAP认证"Tab可见