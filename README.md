# OpenVPN 运维控制台

一个面向个人、团队和小型企业的 OpenVPN 统一运维管理台，内置 **AI 智能运维助手**，支持自然语言完成账号、客户端、防火墙、证书、通知等所有日常管理操作。

![Overview](doc/screenshots/overview.png)

---

## 目录

- [核心亮点](#核心亮点)
- [功能矩阵](#功能矩阵)
- [AI 智能运维助手](#ai-智能运维助手)
- [页面速览](#页面速览)
- [系统架构](#系统架构)
- [技术栈](#技术栈)
- [快速启动](#快速启动)
- [本地开发](#本地开发)
- [构建打包](#构建打包)
- [GitHub 自动发布](#github-自动发布)
- [项目结构](#项目结构)
- [文档](#文档)

---

## 核心亮点

- **一站式管理**：账号、客户端配置、CCD 路由、证书、防火墙、通知渠道、审计、统计全部在一个控制台内完成
- **AI 运维助手**：基于 Google ADK + 大模型的对话式运维，22 个内置工具覆盖所有功能模块，支持流式响应与多轮会话
- **RBAC 权限模型**：管理员/审计员/普通用户三级角色，权限码细粒度到按钮级（如 `client:kill`、`firewall:create`）
- **MFA + 多主题**：TOTP 动态口令登录、4 套主题切换、深色玻璃拟态界面
- **多平台客户端分发**：Windows / macOS / Linux / iOS / Android 客户端安装包统一管理，用户自助下载
- **多架构 Docker 镜像**：amd64 / arm64 / armv7 一键覆盖，树莓派/家庭服务器/NAS 通用
- **零依赖单体部署**：Go 1.25 + go:embed，前端构建产物嵌入二进制，单文件即可运行完整 Web 管理台

---

## 功能矩阵

| 模块 | 能力 | 截图 |
|------|------|------|
| **账号管理** | 创建/修改/删除账号、批量导入导出、有效期、固定 IP、MFA、角色绑定、密码重置 | [Users](doc/screenshots/users.png) |
| **用户分组** | 多级用户组隔离、按组推送 CCD、独立防火墙策略 | [Roles](doc/screenshots/roles.png) |
| **客户端配置** | 一键生成 .ovpn、CCD 路由推送、客户端证书生命周期、客户端下载页 | [Clients](doc/screenshots/clients.png) |
| **连接历史** | 上线/下线历史、IP 归属地查询（ip2region）、流量统计 | [History](doc/screenshots/history.png) |
| **证书管理** | CA / 服务端 / 客户端证书、CRL 撤销列表、续签、过期预警 | [Certs](doc/screenshots/certs.png) |
| **防火墙** | nftables 黑名单、上下行带宽限速（Qos）、按 IP / 用户 / 用户组维度 | [Firewall](doc/screenshots/firewall.png) |
| **操作审计** | 全量操作日志、按操作人/模块/动作/时间筛选、CSV 导出 | [Audit](doc/screenshots/audit.png) |
| **通知渠道** | 钉钉/企微/Webhook/Email/Discord/Slack/Telegram/Mattermost 上线下线告警 | [Channels](doc/screenshots/channels.png) |
| **通知中心** | 系统通知收件箱、未读计数、WebSocket 实时推送 | [Notifications](doc/screenshots/notifications.png) |
| **系统设置** | 基础配置、LDAP、OpenVPN 参数、客户端安装包、AI 配置 | [Settings](doc/screenshots/settings.png) |
| **运维总览** | 实时 CPU/内存/磁盘/网络、在线连接、服务器负载 | [Overview](doc/screenshots/overview.png) |
| **AI 助手** | 自然语言运维对话、内置 22 个工具、流式响应 | [AI Assistant](doc/screenshots/ai-assistant.png) |

---

## AI 智能运维助手

内置 Google ADK Agent + 大模型的对话式 AI 运维助手，**22 个内置工具** 覆盖所有日常管理操作。

![AI Assistant](doc/screenshots/ai-assistant.png)

### 支持的 LLM Provider

- **DeepSeek**（推荐，国内访问快，中文能力强）
- **Ollama**（本地部署，Docker 镜像内置 `qwen2.5:1.5b`）
- **OpenAI** 兼容（任何 OpenAI API 格式的服务）
- **Customize**（自定义 OpenAI 兼容端点）

### 22 个 AI 工具一览

| 模块 | 工具 | 说明 |
|------|------|------|
| **账号管理** | `create_user` | 创建用户（自动生成 .ovpn + 发送开通邮件） |
| | `list_users` | 列出所有用户 |
| | `update_user` | 修改用户信息 |
| | `delete_user` | 删除用户 |
| | `bind_role` | 绑定/解绑角色 |
| | `reset_password` | 重置密码并邮件下发 |
| | `reset_mfa` | 重置 MFA 并重建 .ovpn |
| | `get_system_counts` | 用户/客户端统计 |
| **VPN 客户端** | `generate_client` | 生成客户端配置 |
| | `regenerate_client` | 重新生成配置 |
| | `list_clients` | 列出所有客户端 |
| | `list_online_clients` | 查看当前在线连接 |
| | `kill_connection` | 断开指定连接 |
| | `delete_client` | 删除客户端（吊销证书） |
| | `update_ccd` | 配置 CCD 路由/固定 IP |
| **防火墙** | `list_firewall_rules` | 查看黑名单 + 限速规则 |
| | `manage_firewall` | 黑名单 / 限速增删 |
| **证书管理** | `list_certs` | 查看所有证书 |
| **通知渠道** | `list_channels` | 列出通知渠道 |
| | `manage_channel` | 渠道 CRUD |
| **审计日志** | `query_audit_logs` | 按模块/操作者/时间筛选 |
| **仪表盘** | `get_dashboard` | 全局汇总数据 |

### 使用示例

```
👤 你：帮我创建一个测试用户 test001，邮箱 test@example.com
🤖 AI：好的，正在创建用户...
       ✅ 已创建用户 test001，自动生成 .ovpn 客户端配置
       ✅ 开通邮件已发送至 test@example.com

👤 你：查看当前在线连接有哪些？
🤖 AI：当前在线连接 0 个：
       （无活跃连接）

👤 你：把所有超过 30 天没上线的用户禁用
🤖 AI：查询到 5 个用户超过 30 天未登录，是否全部禁用？
       [即将执行：update_user * 5]
```

### AI 工具与页面操作的完全对齐

`create_user` 工具复用了页面 "新增用户" 的完整流程：
1. 创建用户记录（含姓名、邮箱、有效期、固定 IP）
2. 自动生成 OpenVPN 客户端 .ovpn 配置文件
3. 异步发送开通通知邮件（包含初始密码 + .ovpn 附件 + 安装包下载链接）

所有工具均通过 `agent.ToolContext.UserID()` 获取当前操作者身份并执行 RBAC 权限校验。

---

## 页面速览

### 登录页

![Login](doc/screenshots/login.png)

- 支持账号密码 + MFA 双因素认证
- 首次登录强制修改密码
- 7 天免登录（受控选项）

### 运维总览

![Overview](doc/screenshots/overview.png)

- 实时系统监控（CPU/内存/磁盘/网络）通过 WebSocket 推送，采集间隔 5 秒
- 服务器基本信息（主机名、内核、运行时间、负载）
- 每核 CPU 使用率、物理分区、网络接口流量 Top N

### 账号管理

![Users](doc/screenshots/users.png)

- 用户组 + 账号两级管理
- 搜索/排序/分页/批量操作
- 单个用户操作：编辑、配置、删除、修改密码、生成客户端、下载 .ovpn、CCD 设置、修改 MFA

### 客户端配置

![Clients](doc/screenshots/clients.png)

- 按客户端证书文件管理
- 多平台客户端下载（WINDOWS / LINUX / MACOS / IOS / ANDROID）
- 单个 .ovpn 操作：下载、复制内容、配置、CCD、删除

### 证书管理

![Certs](doc/screenshots/certs.png)

- CA / 服务端 / 客户端证书分类
- 状态显示（即将过期/正常）
- 一键续签

### 操作审计

![Audit](doc/screenshots/audit.png)

- 全量操作日志（创建/更新/删除/登录/登出 等）
- 多维度筛选（操作人/模块/动作/时间范围）
- CSV 导出

### 角色管理

![Roles](doc/screenshots/roles.png)

- RBAC 角色定义
- 角色权限分配（按钮级权限码）
- 角色用户绑定

### 防火墙

![Firewall](doc/screenshots/firewall.png)

- 基于 nftables 的黑名单 + 限速规则
- 支持按 IP / 用户 / 用户组维度
- 上下行带宽独立限速

### 通知中心

![Notifications](doc/screenshots/notifications.png)

- 系统通知收件箱
- 未读计数实时刷新
- WebSocket 推送

### 通知渠道

![Channels](doc/screenshots/channels.png)

- 9 种渠道支持：钉钉/企业微信/Webhook/Email/Discord/Slack/Telegram/Mattermost/通用
- 每种渠道独立测试按钮
- 失败重试记录

### 系统设置

![Settings](doc/screenshots/settings.png)

- 基础设置（站点名、Logo、注册开关）
- LDAP 集成
- OpenVPN 服务端参数（子网、DNS、协议、加密算法）
- 客户端安装包管理（上传/启用/下载）
- AI 助手配置（Provider/Model/API Key/System Prompt）

### 连接历史

![History](doc/screenshots/history.png)

- 上线/下线历史
- IP 归属地查询
- 流量统计

---

## 系统架构

```
┌─────────────────────────────────────────────────────────────────┐
│                      Browser (Chrome/Edge)                       │
└──────────────────────────────┬──────────────────────────────────┘
                               │ HTTP/SSE/WebSocket
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                  openvpn-web (Single Binary)                     │
│  ┌──────────────────────┐    ┌──────────────────────────────┐    │
│  │  Gin HTTP Router     │───▶│  React SPA (embedded via     │    │
│  │  /login /admin /ovpn │    │  go:embed index.html)        │    │
│  └──────────┬───────────┘    └──────────────────────────────┘    │
│             │                                                    │
│  ┌──────────▼─────────────────────────────────────────────────┐  │
│  │              GORM + SQLite (data/openvpn.db)              │  │
│  └──────────┬─────────────────────────────────────────────────┘  │
│             │                                                    │
│  ┌──────────▼──────────┐    ┌──────────────────────────────┐    │
│  │  OpenVPN PKI        │    │  nftables / supervisor        │    │
│  │  (Easy-RSA)         │    │  (firewall + hooks)          │    │
│  └─────────────────────┘    └──────────────────────────────┘    │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │   AI Agent Layer (Google ADK v1.5.1)                     │    │
│  │   ┌─────────────┐  ┌─────────────┐  ┌────────────────┐   │    │
│  │   │ ChatSession │  │ AgentRunner │  │ ToolService    │   │    │
│  │   │ Manager     │──│ (functiontool│──│ (22 tools)     │   │    │
│  │   │             │  │  loop)     │  │                │   │    │
│  │   └─────────────┘  └──────┬──────┘  └────────────────┘   │    │
│  └────────────────────────────┼─────────────────────────────┘    │
│                               │                                  │
└───────────────────────────────┼──────────────────────────────────┘
                                ▼
                ┌──────────────────────────────┐
                │  LLM Backend                 │
                │  (DeepSeek / Ollama /        │
                │   OpenAI compatible)         │
                └──────────────────────────────┘
```

### 后端分层

```
cmd/openvpn-web/                 # 启动入口（main.go）
└── internal/openvpnweb/
    ├── server.go                # Gin 路由 + RBAC 中间件
    ├── config.go                # Viper 配置
    ├── user.go                  # User 模型 + AES 加密/解密
    ├── client.go                # 客户端 .ovpn 生成
    ├── ccd.go                   # CCD 路由配置
    ├── pki.go                   # Easy-RSA 证书管理
    ├── firewall.go              # nftables 封装
    ├── audit.go                 # 操作审计
    ├── dashboard.go             # 仪表盘汇总
    ├── channel.go               # 通知渠道（9 种实现）
    ├── ai/
    │   ├── agent.go             # ADK Agent 初始化
    │   ├── tools.go             # 22 个 functiontool 注册
    │   ├── sse.go               # SSE 流式响应
    │   └── llm.go               # DeepSeek/Ollama/OpenAI 客户端
    ├── ai_tool_service.go       # 22 个工具的业务实现
    └── templates/               # Go embed 模板
        └── index.html           # React 宿主页
```

### 前端分层

```
frontend/src/
├── api.ts                       # Axios 封装 + 拦截器
├── App.tsx                      # 路由
├── layout/
│   ├── Layout.tsx               # 整体布局（含 AIWidget 挂载）
│   └── Sidebar.tsx              # RBAC 动态菜单
├── pages/
│   ├── Overview/                # 仪表盘
│   ├── Users/                   # 账号管理
│   ├── Clients/                 # 客户端配置
│   ├── Firewall/                # 防火墙
│   ├── History/                 # 连接历史
│   ├── Certs/                   # 证书管理
│   ├── Audit/                   # 操作审计
│   ├── Settings/                # 系统设置（含 AI 配置）
│   ├── Notifications/           # 通知中心
│   ├── ChannelProviders/        # 通知渠道
│   ├── Roles/                   # 角色管理
│   ├── Permissions/             # 权限管理
│   ├── Profile/                 # 个人中心
│   ├── Login/                   # 登录页
│   ├── Download/                # 客户端下载页（公开）
│   └── AIAssistant/             # AI 助手独立页面
├── components/
│   ├── AIWidget/                # 全局悬浮 AI 入口
│   ├── HeroOrbitScene/          # 登录页 3D 动效
│   ├── MarkdownContent/         # Markdown 渲染（AI 回复用）
│   └── ... UI 组件库
├── store/
│   ├── auth.tsx                 # 登录态 + 用户信息
│   ├── theme.tsx                # 4 套主题切换
│   └── systemStatus.tsx         # 系统实时状态
└── lib/
    ├── sse.ts                   # 统一 SSE 解析器
    └── notificationHub.ts       # WebSocket 客户端
```

---

## 技术栈

### 后端

| 类别 | 选型 |
|------|------|
| 语言 | Go 1.25.4 |
| Web 框架 | Gin v1.11.0 |
| ORM | GORM v1.31.1 |
| 数据库 | SQLite (glebarez/sqlite 驱动) |
| AI 框架 | Google ADK v1.5.1 |
| LLM 客户端 | DeepSeek / Ollama / OpenAI 兼容 |
| 证书 | Easy-RSA + crypto/x509 |
| 防火墙 | nftables |
| 配置 | Viper v1.21.0 |
| 会话 | gin-contrib/sessions + gorilla/sessions |
| WebSocket | gorilla/websocket |
| MFA | pquerna/otp（TOTP） |
| LDAP | go-ldap/ldap/v3 |
| 邮件 | wneessen/go-mail |
| 加密 | AES (自实现敏感字段加密) + bcrypt (密码) |
| 定时任务 | robfig/cron/v3 |
| 监控 | gopsutil/v3 |
| IP 归属地 | lionsoul2014/ip2region |

### 前端

| 类别 | 选型 |
|------|------|
| 框架 | React 18 + TypeScript 5 |
| 构建 | Vite 5 |
| 路由 | React Router v6 |
| 状态 | Zustand |
| 样式 | Tailwind CSS 3 + CSS Variables |
| 组件库 | 自研（基于 shadcn/ui 设计语言） |
| 图标 | lucide-react |
| 表格 | TanStack Table |
| 图表 | Recharts |
| Markdown 渲染 | react-markdown |
| 通知 | sonner |
| HTTP | Axios |
| 包管理 | pnpm |

### 部署

| 类别 | 选型 |
|------|------|
| 镜像 | Debian bookworm-slim（glibc 兼容官方 Ollama） |
| 进程管理 | supervisor |
| VPN | OpenVPN 2.6 |
| 防火墙 | nftables |
| 多架构 | linux/amd64, linux/arm64, linux/arm/v7 |

---

## 快速启动

### Docker Compose（推荐）

```bash
git clone https://github.com/xinglinglove1029/openvpn-web.git
cd openvpn-web
docker compose up -d --build
docker compose logs -f
```

启动后访问：

```text
http://127.0.0.1:8888/login       # 管理员登录
http://127.0.0.1:8888/download    # 客户端下载（公开页）
```

默认管理员账号：

```text
用户名：admin
密码：admin
```

> **生产环境首次登录后请立即修改默认管理员密码。**

停止服务：

```bash
docker compose down
```

### Docker Run

```bash
docker run -d \
  --name openvpn \
  --cap-add=NET_ADMIN \
  -p 1194:1194/udp \
  -p 8888:8888 \
  -e ADMIN_USERNAME=admin \
  -e ADMIN_PASSWORD=admin \
  -e OLLAMA_AUTO_PULL=true \
  -e OLLAMA_DEFAULT_MODEL=qwen2.5:1.5b \
  -v $(pwd)/data:/data \
  xinglinglove1029/openvpn:latest
```

---

## 本地开发

### 1. 前端构建

```bash
cd frontend
npm install          # 或 pnpm install
npm run build        # 输出到 internal/openvpnweb/templates/static/admin/
```

### 2. 启动后端

```bash
# Linux/macOS
mkdir -p ./data
export OVPN_DATA=$(pwd)/data
go run ./cmd/openvpn-web

# Windows PowerShell
New-Item -ItemType Directory -Force .\data
$env:OVPN_DATA = (Resolve-Path .\data).Path
go run .\cmd\openvpn-web
```

### 3. 前端热更新（开发模式）

```bash
cd frontend
npm run dev          # 启动 Vite dev server (默认 http://127.0.0.1:5173)
```

修改 `frontend` 后必须重新执行 `npm run build`，并重启 Go 服务或重新打包二进制。
Go 使用 `go:embed` 嵌入静态资源，仅刷新浏览器不会更新已嵌入的前端资源。

---

## 构建打包

### 多平台服务端压缩包

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-openvpn-web-local.ps1
```

产物输出到 `dist/`：

```text
dist/openvpn-web-linux-x86_64.tar.gz
dist/openvpn-web-linux-aarch64.tar.gz
dist/openvpn-web-linux-armv6l.tar.gz
dist/openvpn-web-linux-armv7l.tar.gz
dist/openvpn-web-windows-x86_64.zip
dist/openvpn-web-darwin-x86_64.tar.gz
dist/openvpn-web-darwin-aarch64.tar.gz
dist/openvpn-web_sha256_checksums.txt
```

### 单架构 Docker 镜像

```bash
docker build -f build/Dockerfile -t xinglinglove1029/openvpn:latest .
```

### 多架构 Docker 镜像

```bash
docker buildx create --name openvpn-builder --use
docker buildx inspect --bootstrap
docker buildx build \
  --platform linux/amd64,linux/arm64,linux/arm/v7 \
  -f build/Dockerfile \
  -t xinglinglove1029/openvpn:latest \
  --push \
  .
```

---

## GitHub 自动发布

项目内置 `.github/workflows/build.yml`。推送 tag 后自动构建多平台二进制 + 多架构 Docker 镜像。

```bash
git tag v1.0.0
git push origin v1.0.0
```

需要在 GitHub 仓库配置 Actions Secrets：

```text
DOCKERHUB_USERNAME=xinglinglove1029
DOCKERHUB_TOKEN=<Docker Hub Access Token>
```

发布完成后生成：

- GitHub Release 压缩包：Linux、Windows、macOS 多平台版本
- GitHub Release 校验文件：`openvpn-web_sha256_checksums.txt`
- Docker Hub 镜像：`xinglinglove1029/openvpn:latest`
- Docker Hub 镜像：`xinglinglove1029/openvpn:<tag>`

---

## 项目结构

```text
openvpn/
├── cmd/openvpn-web/                         # Go 程序入口
├── internal/openvpnweb/                     # Go Web 服务和业务代码
│   ├── ai/                                  # AI Agent 模块
│   ├── templates/                           # Go embed 模板和静态资源
│   └── *.go                                 # 业务模块
├── frontend/                                # React 源码
│   ├── src/pages/                           # 16 个页面
│   ├── src/components/                      # 通用组件
│   └── src/layout/                          # 布局
├── build/                                   # Docker 镜像文件
├── scripts/                                 # 构建/截图脚本
├── doc/
│   └── screenshots/                         # README 截图
├── docker-compose.yml                       # Docker Compose 编排
├── .dockerignore
├── .goreleaser.yml
├── go.mod
└── README.md
```

---

## 文档

- [本地调试、Docker 构建、GitHub 发布](doc/local-dev-build.md)
- [OpenVPN 安装参考脚本](scripts/openvpn-install.sh)

---

## 验证命令

```bash
cd frontend
npm run build

cd ../..
go test ./...
docker compose config
```

---

## 注意事项

- 生产环境首次登录后请立即修改默认管理员密码。
- 只执行 `go run ./cmd/openvpn-web` 不会启动真实 OpenVPN 进程；上线/下线 hook、防火墙、在线连接等能力需要 Docker 完整环境验证。
- 本地数据目录为 `data/`，构建产物目录为 `dist/`，均已在 `.gitignore` 中排除。
- AI 助手需要配置有效的 LLM（DeepSeek API Key 或 Ollama 本地模型）才能启用；未启用时所有其他功能正常工作。

---

## License

详见 [LICENSE](LICENSE) 文件。
