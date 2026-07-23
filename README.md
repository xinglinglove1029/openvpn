# OpenVPN 运维控制台

OpenVPN 运维控制台是一个面向个人、团队和小型企业运维场景的 Web 管理台。后端使用 Go，前端使用 React + Vite，提供账号、客户端、用户组、防火墙、系统设置、通知告警、操作审计和连接态势总览等能力。

本项目采用现代化的前端架构，使用 React + Vite 构建。所有用户交互界面（包括登录页、用户自助服务页和管理员后台）均通过 `internal/openvpnweb/templates/react-admin.html` 这一单一入口文件承载。该文件作为应用的宿主页，配合 Go 的 `embed` 机制，将编译后的前端资源无缝集成进后端二进制文件，实现了真正的单体应用部署。旧版前端页面和冗余静态资源已全部移除，确保项目结构的清晰与高效。

## 功能特性

- 账号、用户组、客户端配置、CCD、证书和防火墙规则管理
- 管理员登录、用户自助登录、MFA、LDAP、邮件和系统参数配置
- 钉钉 / 企业微信上线下线通知、通知测试和失败记录查询
- 首页运维总览、在线连接、连接历史、操作审计和列表分页
- 多主题科技风界面，统一表单校验、确认弹窗、Toast、加载和错误状态

## 项目结构

```text
openvpn/
|-- cmd/openvpn-web/                         # Go 程序入口
|-- internal/openvpnweb/                     # Go Web 服务和业务代码
|   `-- templates/                           # Go embed 模板和静态资源
|       |-- react-admin.html                 # /login、/、/admin 的 React 宿主页
|       `-- static/admin/                    # React 构建产物
|-- frontend/admin/                          # React 源码
|-- build/                                   # Docker 镜像文件和 OpenVPN 辅助脚本
|-- scripts/                                 # 本地构建、清理和安装脚本
|-- doc/                                     # 项目文档
|-- dist/                                    # 本地多平台构建产物，不提交仓库
|-- docker-compose.yml                       # 唯一保留的 Docker Compose 文件
|-- .dockerignore
|-- .goreleaser.yml
|-- go.mod
`-- README.md
```

Go 包结构采用常见的 `cmd/` + `internal/` 布局：`cmd/openvpn-web` 只保留启动入口，核心 Web 服务和业务逻辑集中在 `internal/openvpnweb`。

## 快速启动

### Docker Compose

根目录只保留一个 Compose 文件，镜像名称统一为 `xinglinglove1029/openvpn:latest`。

```bash
docker compose up -d --build
docker compose logs -f
```

访问地址：

```text
http://127.0.0.1:8833/login
http://127.0.0.1:8833/admin
```

默认管理员账号：

```text
用户名：admin
密码：admin
```

停止服务：

```bash
docker compose down
```

### Docker Run

如果已经从 Docker Hub 拉取或本地构建了镜像：

```bash
docker run -d \
  --name openvpn \
  --cap-add=NET_ADMIN \
  -p 1194:1194/udp \
  -p 8833:8833 \
  -e ADMIN_USERNAME=admin \
  -e ADMIN_PASSWORD=admin \
  -e OVPN_GATEWAY=false \
  -v $(pwd)/data:/data \
  xinglinglove1029/openvpn:latest
```

## 本地开发

前端源码在 `frontend/admin`，构建后输出到 Go embed 目录：

```bash
cd frontend/admin
npm install
npm run build

cd ../..
mkdir -p ./data
export OVPN_DATA=$(pwd)/data
go run ./cmd/openvpn-web
```

Windows PowerShell：

```powershell
cd frontend\admin
npm.cmd install
npm.cmd run build

cd ..\..
New-Item -ItemType Directory -Force .\data
$env:OVPN_DATA = (Resolve-Path .\data).Path
go run .\cmd\openvpn-web
```

修改 `frontend/admin` 后必须重新执行 `npm run build`，并重启 Go 服务或重新打包二进制。Go 使用 `go:embed` 嵌入静态资源，仅刷新浏览器不会更新已嵌入的前端资源。

## 构建打包

### 多平台服务端压缩包

这里打包的是 `openvpn-web` 管理服务端程序，不是 Windows / macOS 桌面 OpenVPN 客户端。完整 OpenVPN 网关运行依赖 Linux 网络栈、OpenVPN、iptables/nftables 和 supervisor，生产部署优先使用 Docker 镜像 `xinglinglove1029/openvpn:latest`。

Linux 产物按 CPU 架构区分，不按 Ubuntu、Debian、CentOS、Alpine 等发行版分别命名。原因是 Go 程序使用 `CGO_ENABLED=0` 静态编译，`openvpn-web` 自身通常只需要匹配 `linux + CPU 架构`；不同发行版差异主要体现在 OpenVPN、iptables、supervisor 等系统依赖，由 Docker 镜像或 `scripts/openvpn-install.sh` 处理。

根目录执行：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-openvpn-web-local.ps1
```

产物统一输出到仓库根目录的 `dist/`：

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

多架构镜像需要 Docker Buildx，支持 `linux/amd64`、`linux/arm64`、`linux/arm/v6`、`linux/arm/v7`。

```bash
docker buildx create --name openvpn-builder --use
docker buildx inspect --bootstrap
docker buildx build \
  --platform linux/amd64,linux/arm64,linux/arm/v6,linux/arm/v7 \
  -f build/Dockerfile \
  -t xinglinglove1029/openvpn:latest \
  --push \
  .
```

带版本号发布：

```bash
VERSION=v1.0.0
docker buildx build \
  --platform linux/amd64,linux/arm64,linux/arm/v6,linux/arm/v7 \
  --build-arg VERSION=$VERSION \
  --build-arg COMMIT=$(git rev-parse HEAD) \
  --build-arg DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -f build/Dockerfile \
  -t xinglinglove1029/openvpn:latest \
  -t xinglinglove1029/openvpn:$VERSION \
  --push \
  .
```

## GitHub 自动发布

项目内置 `.github/workflows/build.yml`。推送 tag 后，GitHub Actions 会自动构建多平台二进制，并发布多架构 Docker 镜像到 `xinglinglove1029/openvpn`。

```bash
git tag v1.0.0
git push origin v1.0.0
```

需要在 GitHub 仓库配置 Actions Secrets：

```text
DOCKERHUB_USERNAME=xinglinglove1029
DOCKERHUB_TOKEN=<Docker Hub Access Token>
```

发布完成后会生成：

- GitHub Release 压缩包：Linux、Windows、macOS 多平台服务端版本
- GitHub Release 校验文件：`openvpn-web_sha256_checksums.txt`
- Docker Hub 镜像：`xinglinglove1029/openvpn:latest`
- Docker Hub 镜像：`xinglinglove1029/openvpn:<tag>`，例如 `xinglinglove1029/openvpn:v1.0.0`

详细二次开发、Docker 镜像构建和 GitHub 发布说明见 `doc/local-dev-build.md`。

## 验证命令

```bash
cd frontend/admin
npm run build

cd ../..
go test ./...
docker compose config
```

Windows PowerShell 前端构建建议使用：

```powershell
cd frontend\admin
npm.cmd run build
```

## 文档

- 本地调试、Docker 完整流程、多架构镜像和 GitHub 自动发布：`doc/local-dev-build.md`
- OpenVPN 安装参考脚本：`scripts/openvpn-install.sh`

## 注意事项

- 生产环境首次登录后请立即修改默认管理员密码。
- 只执行 `go run ./cmd/openvpn-web` 不会启动真实 OpenVPN 进程；上线/下线 hook、防火墙、在线连接等能力需要 Docker 完整环境验证。
- 本地数据目录为 `data/`，构建产物目录为 `dist/`，均已在 `.gitignore` 中排除。
