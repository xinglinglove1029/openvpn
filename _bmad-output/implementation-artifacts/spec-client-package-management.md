---
title: 'OpenVPN 客户端安装包维护功能'
type: 'feature'
created: '2026-07-26'
status: 'ready-for-dev'
review_loop_iteration: 0
followup_review_recommended: false
context: []
warnings: []
baseline_revision: 'feature/client-package-management'
---

<intent-contract>

## Intent

**Problem:** OpenVPN 官方客户端安装包需要翻墙下载，国内用户无法直接获取。管理员需要手动下载各平台安装包并托管在自有服务器上，方便国内用户直接下载。同时，新建用户时需要自动发送本地托管的客户端下载链接。

**Approach:** 新增客户端安装包管理模块，支持各平台（Windows/macOS/Linux/Android/iOS）安装包的上传、维护和下载。在系统设置页面增加"客户端管理"标签页，管理员可上传、启用/禁用、删除各平台安装包。创建用户时，邮件模板自动包含本地托管的安装包下载链接。

## Boundaries & Constraints

**Always:**
- 安装包文件存储在服务器本地磁盘（data/client-packages/ 目录）
- 每个平台只能有一个启用的安装包版本
- 下载链接需鉴权保护（管理员或登录用户）
- 文件上传限制：单文件最大 500MB
- 安装包元数据存储在 SQLite 数据库

**Block If:**
- 磁盘空间不足 1GB 时禁止上传
- 上传的文件类型与所选平台不匹配时需警告

**Never:**
- 不支持在线直接从 GitHub/OpenVPN 下载安装包
- 不支持安装包的自动更新或版本检查
- 不修改现有的外部下载 URL 配置（client.client_url），两种模式并存

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| HAPPY_PATH_UPLOAD | 管理员上传 Windows 安装包 | 创建数据库记录，保存文件到磁盘，返回成功 | 文件名重复时覆盖旧版本并记录日志 |
| HAPPY_PATH_LIST | 请求安装包列表 | 返回所有平台的安装包信息，按平台分组 | 无数据时返回空数组 |
| HAPPY_PATH_DOWNLOAD | 已登录用户请求下载启用的安装包 | 返回文件流，支持断点续传 | 文件不存在时返回 404 |
| HAPPY_PATH_DELETE | 管理员删除安装包记录 | 删除数据库记录和磁盘文件 | 文件已被删除时仅清理数据库记录 |
| HAPPY_PATH_ENABLE | 管理员启用某平台安装包 | 禁用同平台其他版本，启用选定版本 | 同时只能有一个启用状态 |
| ERROR_FILE_TOO_LARGE | 上传 600MB 文件 | 返回 413 错误 | 提示文件超过 500MB 限制 |
| ERROR_DISK_FULL | 磁盘剩余 < 1GB | 返回 507 错误 | 提示磁盘空间不足 |
| ERROR_UNAUTHORIZED | 未登录用户请求下载 | 返回 401 错误 | 重定向到登录页 |
| ERROR_PLATFORM_MISMATCH | iOS 平台上传 .msi 文件 | 返回 400 警告 | 提示文件格式与平台不匹配 |

</intent-contract>

## Code Map

- internal/openvpnweb/package.go -- 新增：ClientPackage 数据模型、CRUD 方法、文件操作
- internal/openvpnweb/server.go -- 修改：新增安装包管理路由
- internal/openvpnweb/email_template.go -- 修改：邮件模板增加本地安装包下载链接
- frontend/src/pages/Settings/index.tsx -- 修改：新增"客户端管理"标签页
- frontend/src/types.ts -- 修改：新增 ClientPackage 类型定义

## Tasks & Acceptance

**Execution:**

- [ ] internal/openvpnweb/package.go -- 创建 ClientPackage 模型，实现数据库 CRUD 和文件存储/删除方法
- [ ] internal/openvpnweb/server.go -- 注册安装包路由，修改用户创建 API 发送本地下载链接
- [ ] internal/openvpnweb/email_template.go -- 修改邮件模板，增加本地托管安装包下载链接区域
- [ ] frontend/src/types.ts -- 新增 ClientPackage 接口类型
- [ ] frontend/src/pages/Settings/index.tsx -- 新增"客户端管理"Tab，实现上传、列表、启用/禁用、删除操作
- [ ] frontend/src/api.ts -- 新增安装包 API 封装
- [ ] internal/openvpnweb/server.go -- 注册 ClientPackage 模型到 AutoMigrate，启动时创建存储目录

**Acceptance Criteria:**

- Given 管理员已登录，当访问系统设置的"客户端管理"标签页时，应显示所有平台的安装包列表
- Given 管理员上传 Windows 平台安装包并填写版本号，当提交后，应创建数据库记录、保存文件到磁盘、该平台旧版本自动禁用
- Given 管理员已启用某个平台的安装包，当新建用户并勾选"发送邮件"时，用户邮件中应包含本地托管的安装包下载链接
- Given 已登录用户，当通过下载链接请求安装包时，应能正常下载文件
- Given 管理员，当删除某个安装包时，数据库记录和磁盘文件都应被清理
- Given 磁盘空间不足 1GB，当尝试上传安装包时，应返回错误并提示
- Given 未登录用户，当请求安装包下载时，应返回 401 错误

## Design Notes

### 数据库表结构

CREATE TABLE client_packages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    platform TEXT NOT NULL,
    version TEXT NOT NULL,
    filename TEXT NOT NULL,
    stored_name TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    download_url TEXT,
    is_active INTEGER DEFAULT 0,
    created_at DATETIME,
    updated_at DATETIME
);

CREATE INDEX idx_client_packages_platform ON client_packages(platform, is_active);

### 存储目录结构

data/client-packages/{platform}/{uuid}.{ext}

### 平台标识映射

| Platform | 存储子目录 | 典型扩展名 |
|----------|-----------|------------|
| windows  | windows/  | .msi, .exe |
| macos    | macos/    | .dmg, .pkg |
| linux    | linux/    | .deb, .rpm, .AppImage |
| android  | android/  | .apk |
| ios      | ios/      | .ipa |

### API 设计

| Method | Path | 描述 | 权限 |
|--------|------|------|------|
| GET | /ovpn/client-packages | 获取所有安装包列表 | 管理员 |
| POST | /ovpn/client-packages | 上传新安装包 | 管理员 |
| DELETE | /ovpn/client-packages/:id | 删除安装包 | 管理员 |
| POST | /ovpn/client-packages/:id/enable | 启用安装包 | 管理员 |
| GET | /ovpn/client-packages/:id/download | 下载安装包 | 登录用户 |

## Verification

**Commands:**
- go build ./... -- 编译通过
- cd frontend && npm run build -- 前端构建通过

**Manual checks:**
- 启动服务后访问系统设置 -> 客户端管理标签页
- 上传各平台安装包
- 启用某个平台版本
- 创建新用户并勾选发送邮件
- 检查邮件中是否包含安装包下载链接
- 点击下载链接确认文件可正常下载
- 删除安装包确认数据库和文件清理