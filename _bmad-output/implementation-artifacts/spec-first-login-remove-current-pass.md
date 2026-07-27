---
title: '首次登录修改密码移除当前密码输入'
type: 'bugfix'
created: '2026-07-27'
status: 'completed'
review_loop_iteration: 0
followup_review_recommended: false
context: []
warnings: []
---

<intent-contract>

## Intent

**Problem:** 新用户首次登录重置密码流程中，第一步登录已经验证过当前密码，成功后才会跳转到改密码流程。如果当前密码不对，压根不会到这个流程。但改密码表单中仍然要求输入"当前密码"，这一步是多余的，增加了用户操作负担。

**Approach:** 移除首次登录修改密码表单中的"当前密码"输入框，保留新密码和确认新密码字段；后端 modifyPass 接口在首次登录场景下（currentPass未传或为空）跳过当前密码验证，直接更新密码。

## Boundaries & Constraints

**Always:**
- 所有代码、注释使用中文
- 保留个人中心（Profile）修改密码功能的当前密码验证（该场景用户已登录一段时间，需要再次验证身份）
- 保留管理员通过"重置密码"功能为其他用户重置密码的逻辑
- 首次登录修改密码成功后仍需调用 login() 函数刷新 session

**Block If:**
- 无法确定哪些场景需要 currentPass 验证

**Never:**
- 不影响 Profile 页面修改密码功能
- 不影响管理员重置用户密码功能

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| 首次登录用户提交新密码 | currentPass为空，newPassword和confirmPassword正确 | 密码更新成功，进入门户 | - |
| 首次登录用户提交新密码 | currentPass为空，newPassword和confirmPassword不匹配 | 验证失败，显示错误提示 | 前端验证 |
| Profile页面修改密码 | currentPass正确，newPassword正确 | 密码更新成功 | - |
| Profile页面修改密码 | currentPass错误 | 返回401 "当前密码错误" | - |
| 直接调用modifyPass | currentPass为空，非首次登录session | 跳过currentPass验证，直接更新密码 | - |

</intent-contract>

## Code Map

- `frontend/src/pages/Login/index.tsx` -- 移除 first-password 模式中的 currentPass 输入框、state 变量、验证逻辑
- `internal/openvpnweb/server.go` -- modifyPass 接口：当 currentPass 为空时跳过验证，直接更新密码

## Tasks & Acceptance

**Execution:**
- [ ] `frontend/src/pages/Login/index.tsx` -- 移除 currentPass 和 showCurrentPass state 变量 -- 状态清理
- [ ] `frontend/src/pages/Login/index.tsx` -- 移除 validateFields 中对 currentPass 的验证 -- 验证清理
- [ ] `frontend/src/pages/Login/index.tsx` -- 移除 first-password 表单中的"当前密码"输入框组件 -- UI清理
- [ ] `frontend/src/pages/Login/index.tsx` -- 修改 submitFirstPassword 函数，不再发送 currentPass 参数 -- API调用修改
- [ ] `internal/openvpnweb/server.go` -- modifyPass 接口：当 currentPass 未传或为空时，跳过当前密码验证，直接更新密码 -- 后端逻辑修改

**Acceptance Criteria:**
- Given 首次登录用户，when 进入改密码页面，then 只显示新密码和确认新密码两个输入框
- Given 首次登录用户，when 输入新密码并确认，then 密码修改成功，进入门户
- Given Profile页面用户，when 修改密码，then 仍需输入当前密码验证
- Given 管理员重置用户密码，when 在Users页面操作，then 功能不受影响

## Design Notes

**前端修改要点：**
1. 删除 `const [currentPass, setCurrentPass] = useState('');` 和 `const [showCurrentPass, setShowCurrentPass] = useState(false);`
2. 删除 validateFields 中对 currentPass 的检查
3. 删除 first-password 表单中第384-415行的"当前密码"输入框
4. 修改 submitFirstPassword 中 body 不再包含 currentPass

**后端修改要点：**
1. modifyPass 接口中，当 currentPass 未传或为空时，跳过密码验证逻辑（第2499-2525行），直接进入密码更新流程（第2527行开始）

## Verification

**Commands:**
- `cd f:\develop\openvpn && go build ./...` -- expected: 编译通过
- `cd f:\develop\openvpn\frontend && npx tsc --noEmit` -- expected: 类型检查通过

**Manual checks:**
- 新建普通用户并勾选"首次登录修改密码" -- 登录时输入当前密码 -- 成功后进入改密码页面 -- 只显示新密码和确认新密码 -- 提交成功
- 登录admin账户 -- 进入个人中心 -- 修改密码 -- 仍需输入当前密码