# 本地调试启动与编译指南

这份文档按当前代码结构编写，适合第一次接触项目的新手，也适合二次开发者快速了解如何本地启动、构建二进制、构建 Docker 镜像，以及如何通过 GitHub Actions 自动发布多平台产物。

## 1. 当前项目结构

```text
openvpn/
|-- cmd/openvpn-web/                         # Go 程序入口
|-- internal/openvpnweb/                     # Go Web 服务和业务代码
|   `-- templates/                           # Go embed 模板和静态资源
|       `-- index.html                     # /login、/、/admin 的 React 宿主页
|       `-- static/
|           |-- cropped-openvpn-32x32.png    # favicon
|           `-- admin/                       # React 构建产物
|-- frontend/                          # React 源码
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

后端采用常见 Go 布局：

- `cmd/openvpn-web` 是命令入口，只调用 `internal/openvpnweb.Run(...)`。
- `internal/openvpnweb` 是项目私有业务包，用于承载 Web 服务、路由、模板、模型和 OpenVPN 管理逻辑。
- `frontend` 是唯一保留的前端源码目录，登录页、管理员后台和用户自助页均从这里构建。
- `build/Dockerfile` 是唯一保留的 Dockerfile，不再维护 `Dockerfile.dev`。
- `dist/` 是本地编译产物目录，不提交 Git。

## 2. 环境准备

### 2.1 必装工具

- Git
- Go：使用 `go.mod` 声明的版本，当前为 `1.25.4`
- Node.js 与 npm：用于构建 React 管理后台
- Docker Desktop 或 Linux Docker Engine：用于完整 OpenVPN 流程调试和镜像构建
- Docker Buildx：用于构建多架构 Docker 镜像

### 2.2 Windows 注意事项

- PowerShell 中优先使用 `npm.cmd`，避免执行策略拦截 `npm.ps1`。
- 只调试 Web 页面和接口时，可以直接在 Windows PowerShell 运行 Go 服务。
- 调试 OpenVPN、iptables、nftables、上线/下线 hook 时，建议使用 WSL2 + Docker。
- 本机路径 `F:\develop\openvpn` 在 WSL 中通常对应 `/mnt/f/develop/openvpn`。
- 不要把 `build/docker-entrypoint.sh` 转成 CRLF，Shell 脚本必须保持 LF 换行。

### 2.3 检查 Docker / WSL

WSL 中执行：

```bash
cd /mnt/f/develop/openvpn
docker version
docker compose version
docker buildx version
```

如果找不到 `docker`，打开 Docker Desktop，进入 `Settings -> Resources -> WSL Integration`，开启当前 Ubuntu 发行版集成后重试。

## 3. 首次准备代码

```bash
git clone <你的仓库地址> openvpn
cd openvpn
```

如果已经有项目目录，直接进入项目根目录：

```bash
cd /mnt/f/develop/openvpn
```

Windows PowerShell：

```powershell
cd F:\develop\openvpn
```

## 4. 构建 React 管理后台

新版前端源码在 `frontend`。

Linux / macOS / WSL：

```bash
cd frontend
npm install
npm run build
```

Windows PowerShell：

```powershell
cd frontend
npm.cmd install
npm.cmd run build
```

构建完成后会生成：

```text
internal/openvpnweb/templates/static/admin/assets/app.js
internal/openvpnweb/templates/static/admin/assets/app.css
```

Go 程序通过 `go:embed` 把 `internal/openvpnweb/templates` 打进二进制。所以每次修改 React 后，都要执行 `npm run build`，然后重启 `go run ./cmd/openvpn-web` 或重新打包二进制。否则浏览器可能继续看到旧页面或旧样式。

## 5. 本地启动 Go Web 服务

本地直接运行适合调试后台页面、接口、配置保存、通知测试等功能。它不会启动真正的 OpenVPN 进程。

### 5.1 创建数据目录

Linux / macOS / WSL：

```bash
mkdir -p ./data
export OVPN_DATA=$(pwd)/data
```

Windows PowerShell：

```powershell
New-Item -ItemType Directory -Force .\data
$env:OVPN_DATA = (Resolve-Path .\data).Path
```

### 5.2 启动服务

项目根目录执行：

```bash
go mod download
go run ./cmd/openvpn-web
```

Windows PowerShell：

```powershell
go mod download
go run .\cmd\openvpn-web
```

启动后访问：

```text
http://127.0.0.1:8888/login
http://127.0.0.1:8888/admin
```

默认管理员账号：

```text
用户名：admin
密码：admin
```

### 5.3 本地调试限制

只运行 Go Web 服务时，以下功能可能不可用或返回错误，因为它们依赖完整 OpenVPN 环境：

- OpenVPN management 在线状态读取
- 在线客户端真实列表
- 重启 OpenVPN 服务
- 生成真实客户端证书
- 防火墙、iptables、nftables 规则下发
- 用户上线/下线 hook 自动通知

如需验证完整 VPN 流程，请使用 Docker 调试方式。

## 6. Docker 完整流程调试

Docker 调试会运行 OpenVPN、`openvpn-web`、supervisor 等完整环境，适合验证客户端生成、上线/下线通知、OpenVPN hook、防火墙等功能。

项目根目录只保留一个 `docker-compose.yml`，镜像名称统一为 `xinglinglove1029/openvpn:latest`。

### 6.1 启动完整环境

```bash
docker compose up -d --build
docker compose logs -f
```

访问：

```text
http://127.0.0.1:8888/login
```

停止环境：

```bash
docker compose down
```

如需清空本地测试数据，停止容器后删除 `data/` 目录再启动。

### 6.2 单独构建 Docker 镜像

```bash
docker build -f build/Dockerfile -t xinglinglove1029/openvpn:latest .
```

这个命令会执行完整多阶段构建：

1. 使用 Node 镜像安装 `frontend` 依赖并执行 `npm run build`。
2. 使用 Go 镜像编译 `cmd/openvpn-web`。
3. 使用 Alpine 镜像安装 OpenVPN、EasyRSA、supervisor、iptables/nftables 等运行依赖。
4. 把最终二进制和 OpenVPN 辅助脚本打进运行镜像。

构建完成后查看镜像：

```bash
docker images xinglinglove1029/openvpn
```

本地运行镜像：

```bash
docker run --rm -it \
  --cap-add=NET_ADMIN \
  -p 1194:1194/udp \
  -p 8888:8888 \
  -e ADMIN_USERNAME=admin \
  -e ADMIN_PASSWORD=admin \
  -e OVPN_GATEWAY=false \
  -v $(pwd)/data:/data \
  xinglinglove1029/openvpn:latest
```

推送到 Docker Hub：

```bash
docker login
docker push xinglinglove1029/openvpn:latest
```

### 6.3 本地构建多架构 Docker 镜像

多架构镜像需要 Docker Buildx，适合在 Linux、WSL2 或 GitHub Actions 中执行。

创建并启用 builder：

```bash
docker buildx create --name openvpn-builder --use
docker buildx inspect --bootstrap
```

只构建并推送 `latest`：

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64,linux/arm/v7 \
  -f build/Dockerfile \
  -t xinglinglove1029/openvpn:latest \
  --push \
  .
```

同时推送版本号和 `latest`：

```bash
VERSION=v1.0.0
docker buildx build \
  --platform linux/amd64,linux/arm64,linux/arm/v7 \
  --build-arg VERSION=$VERSION \
  --build-arg COMMIT=$(git rev-parse HEAD) \
  --build-arg DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -f build/Dockerfile \
  -t xinglinglove1029/openvpn:latest \
  -t xinglinglove1029/openvpn:$VERSION \
  --push \
  .
```

如果只想在本机测试，不推送远端，只能加载当前机器架构的镜像：

```bash
docker buildx build \
  --platform linux/amd64 \
  -f build/Dockerfile \
  -t xinglinglove1029/openvpn:local \
  --load \
  .
```

查看远端镜像架构：

```bash
docker buildx imagetools inspect xinglinglove1029/openvpn:latest
```

### 6.4 Dockerfile 维护说明

`build/Dockerfile` 是唯一保留的镜像构建文件，不再维护 `Dockerfile.dev`。

- `frontend-builder` 阶段负责编译 React 静态资源。
- `backend-builder` 阶段根据 Buildx 传入的 `TARGETOS`、`TARGETARCH`、`TARGETVARIANT` 编译对应架构的 Go 二进制。
- 最终运行阶段只保留 OpenVPN 运行依赖、`openvpn-web` 二进制和必要脚本，避免把源码、Node 依赖和 Go 编译缓存打入镜像。
- `.dockerignore` 会排除 `data/`、`dist/`、`node_modules`、本地临时二进制和旧构建产物，减少镜像构建上下文。

二次开发时，如果修改了前端或 Go 代码，直接重新执行 `docker compose up -d --build` 即可；Dockerfile 会自动完成前端构建和后端编译。

## 7. GitHub Actions 自动发布

项目内置 `.github/workflows/build.yml`，用于在 GitHub 上完成多平台二进制和多架构 Docker 镜像发布。

### 7.1 发布触发方式

当前工作流在推送 tag 时触发：

```bash
git tag v1.0.0
git push origin v1.0.0
```

触发后会执行：

1. 检出代码。
2. 安装 Go 和 Node.js。
3. 构建 React 管理后台。
4. 使用 GoReleaser 构建 Linux、Windows、macOS 多平台二进制，并生成校验和。
5. 登录 Docker Hub。
6. 使用 QEMU + Buildx 构建并推送多架构镜像。

### 7.2 GitHub Secrets 配置

进入 GitHub 仓库：`Settings -> Secrets and variables -> Actions -> New repository secret`。

需要配置：

```text
DOCKERHUB_USERNAME=xinglinglove1029
DOCKERHUB_TOKEN=<Docker Hub Access Token>
```

`GITHUB_TOKEN` 由 GitHub Actions 自动提供，不需要手动创建。

Docker Hub Token 创建路径：`Docker Hub -> Account Settings -> Security -> New Access Token`。

### 7.3 GitHub Actions 输出产物

推送 tag 后会生成：

- GitHub Release 附件：`openvpn-web-linux-x86_64.tar.gz`、`openvpn-web-linux-aarch64.tar.gz`、`openvpn-web-windows-x86_64.zip` 等服务端压缩包。
- GitHub Release 附件：`openvpn-web_sha256_checksums.txt`。
- Docker Hub 镜像：`xinglinglove1029/openvpn:latest`。
- Docker Hub 镜像：`xinglinglove1029/openvpn:<tag>`，例如 `xinglinglove1029/openvpn:v1.0.0`。

### 7.4 支持的平台架构

Docker 多架构镜像当前支持：

```text
linux/amd64
linux/arm64
linux/arm/v7
```

`debian:bookworm-slim` 未提供 `linux/arm/v6` 清单，因此 Docker 镜像不再构建 armv6；GoReleaser 仍会发布 armv6 独立二进制压缩包。

GoReleaser 服务端压缩包当前支持：

```text
linux/amd64
linux/arm64
linux/arm/v6
linux/arm/v7
windows/amd64
darwin/amd64
darwin/arm64
```

### 7.5 常见发布问题

如果 Docker Hub 推送失败，优先检查：

- `DOCKERHUB_USERNAME` 是否为 `xinglinglove`。
- `DOCKERHUB_TOKEN` 是否是 Access Token，不是网页登录密码。
- Docker Hub 中是否已经创建或允许自动创建 `xinglinglove1029/openvpn` 仓库。
- GitHub Actions 是否允许读取仓库代码并写入 Release。

如果多架构构建失败，优先检查：

- `build/Dockerfile` 是否仍在仓库内。
- `.dockerignore` 是否误排除了必须文件。
- `frontend/package-lock.json` 是否和 `package.json` 同步。
- Go 依赖是否可以正常下载。

## 8. 多平台二进制打包

本项目打包出来的是 `openvpn-web` 管理服务端，不是 Windows / macOS 桌面 OpenVPN 客户端。Windows 包只能启动 Web 管理服务相关逻辑，不能提供 Linux OpenVPN 网关、防火墙、上线/下线 hook 等完整能力。

Linux 包按 CPU 架构区分，不按 Ubuntu、Debian、CentOS、Alpine 等发行版分别拆包：

- `openvpn-web` 是 `CGO_ENABLED=0` 静态编译的 Go 程序，通常只需要匹配操作系统和 CPU 架构。
- 发行版差异主要是 OpenVPN、iptables/nftables、supervisor、EasyRSA 等系统依赖不同。
- 推荐生产使用 Docker 镜像 `xinglinglove1029/openvpn:latest`，它已经把完整运行环境一起打包。
- 如果不用 Docker，可使用 `scripts/openvpn-install.sh`，脚本会按发行版安装系统依赖，再下载匹配架构的 Linux 压缩包。

### 8.1 推荐：本地脚本打包

项目根目录执行：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-openvpn-web-local.ps1
```

输出产物：

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

查看产物：

```powershell
Get-ChildItem .\dist | Select-Object Name,Length
Get-Content .\dist\openvpn-web_sha256_checksums.txt
```

### 8.2 使用 GoReleaser

项目根目录执行：

```bash
goreleaser release --snapshot --clean
```

配置文件是根目录 `.goreleaser.yml`，输出目录同样是 `dist/`。

## 9. 编译和测试

### 9.1 前端构建检查

```powershell
cd frontend
npm.cmd run build
```

### 9.2 Go 编译检查和测试

项目根目录执行：

```bash
go test ./...
```

如果没有测试文件，`go test ./...` 也会执行编译检查。

### 9.3 Docker Compose 配置检查

```bash
docker compose config
```

### 9.4 单平台快速构建

Windows PowerShell：

```powershell
go build -trimpath -ldflags "-s -w" -o .\dist\openvpn-web-windows-x86_64.exe .\cmd\openvpn-web
```

如需发布给别人使用，建议使用 `scripts/build-openvpn-web-local.ps1` 生成 `.zip`，不要直接分发裸 `.exe`。

Linux / WSL：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/openvpn-web-linux-x86_64 ./cmd/openvpn-web
```

如需发布给别人使用，建议使用 `scripts/build-openvpn-web-local.ps1` 或 GoReleaser 生成 `.tar.gz`，并配套校验文件。

## 10. 常用接口自测

服务启动后可以先验证这些地址：

```bash
curl http://127.0.0.1:8888/login
curl http://127.0.0.1:8888/admin
curl http://127.0.0.1:8888/ovpn/dashboard/summary
```

后台核心流程建议手动检查：

- 登录页主题和错误提示
- 首页仪表盘、风险提示、趋势卡片
- 用户、分组、客户端、防火墙、系统设置列表分页
- 新增/编辑表单校验
- 删除和危险操作确认弹窗
- 通知配置保存和模拟上线/下线测试

## 11. 静态资源缓存说明

React 构建后的 `app.js`、`app.css` 会被 Go embed 进二进制。`index.html` 会追加资源版本参数，降低浏览器缓存旧资源的概率。

开发时建议按下面顺序操作：

1. 修改 `frontend` 源码。
2. 执行 `npm run build` 或 `npm.cmd run build`。
3. 停止当前 Go 服务。
4. 回到项目根目录重新执行 `go run ./cmd/openvpn-web`。
5. 浏览器强制刷新 `/admin`。

## 12. 常见问题

### 页面还是旧样式？

先确认是否已经执行 React 构建：

```powershell
Test-Path .\internal\openvpnweb\templates\static\admin\assets\app.js
```

然后重启 Go 服务。只刷新浏览器不够，因为 Go embed 需要重新编译进程。

### Docker 镜像构建很慢？

首次构建需要下载 Node、Go、npm 依赖、Go 依赖和 EasyRSA。后续 Docker 会复用缓存，速度会快很多。

### Windows 中 npm 被禁止执行？

使用：

```powershell
npm.cmd install
npm.cmd run build
```

### OpenVPN 状态接口报错？

这是本地只启动 Go 服务时的正常限制。需要完整验证时，请使用 `docker compose up -d --build`。
