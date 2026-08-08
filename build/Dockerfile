FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend-builder

WORKDIR /src

# 使用 pnpm（与本地开发环境一致），避免 npm/pnpm 的 node_modules 结构差异
# 导致 Rolldown manualChunks 行为不同，从而引发 React 双实例冲突
RUN npm install -g pnpm@11.17.0

COPY frontend/package.json frontend/pnpm-lock.yaml ./frontend/
RUN cd frontend && pnpm install --frozen-lockfile

COPY frontend ./frontend
COPY internal/openvpnweb/templates/index.html ./internal/openvpnweb/templates/index.html
COPY internal/openvpnweb/templates/static/cropped-openvpn-32x32.png ./internal/openvpnweb/templates/static/cropped-openvpn-32x32.png
RUN cd frontend && pnpm run build

FROM --platform=$BUILDPLATFORM golang:1.25.4-alpine AS backend-builder

WORKDIR /src


COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY --from=frontend-builder /src/internal/openvpnweb/templates/static/admin ./internal/openvpnweb/templates/static/admin

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG TARGETVARIANT=
RUN case "${TARGETVARIANT}" in \
      v6) export GOARM=6 ;; \
      v7) export GOARM=7 ;; \
    esac \
  && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE} -X main.builtBy=docker" \
    -o /out/openvpn-web ./cmd/openvpn-web

FROM alpine:3.23.4

# Ollama 版本，可通过 --build-arg 覆盖
ARG OLLAMA_VERSION=0.32.5
# TARGETPLATFORM / BUILDPLATFORM 由 buildx 自动注入，用于跨架构构建
ARG TARGETPLATFORM
ARG BUILDPLATFORM

RUN set -euxo pipefail \
  && apk add --no-cache \
    bash \
    coreutils \
    curl \
    gcompat \
    grep \
    iptables \
    iptables-legacy \
    jq \
    libgomp \
    mailcap \
    nftables \
    openssl \
    openvpn \
    sqlite \
    supervisor \
    tzdata \
  && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
  && echo "Asia/Shanghai" > /etc/timezone \
  # 下载 Ollama 二进制（按 TARGETARCH 选择 amd64 / arm64）
  # 注意：不直接执行 ollama --version 做验证，
  # 因为在跨架构构建（BUILDPLATFORM != TARGETPLATFORM）下，
  # 通过 QEMU user + gcompat 模拟执行 glibc 二进制偶现失败，
  # 改为只校验文件下载成功且非空即可
  && OLLAMA_ARCH="${TARGETARCH}" \
  && echo "Downloading Ollama ${OLLAMA_VERSION} for ${TARGETPLATFORM} (arch=${OLLAMA_ARCH})" \
  && curl -fsSL \
       --retry 3 \
       --retry-delay 5 \
       --connect-timeout 30 \
       --max-time 600 \
       "https://ollama.com/download/ollama-linux-${OLLAMA_ARCH}?version=v${OLLAMA_VERSION}" \
       -o /usr/local/bin/ollama \
  && chmod +x /usr/local/bin/ollama \
  && test -s /usr/local/bin/ollama \
  && echo "Ollama binary downloaded: $(du -h /usr/local/bin/ollama | cut -f1)"

COPY build/docker-entrypoint.sh /usr/bin/docker-entrypoint.sh
COPY build/openvpn-auth /usr/lib/openvpn/plugins/openvpn-auth
COPY build/ollama-bootstrap.sh /usr/local/bin/ollama-bootstrap.sh
COPY build/supervisord.conf /etc/supervisor/conf.d/supervisord.conf
COPY --from=backend-builder /out/openvpn-web /usr/local/bin/openvpn-web

RUN chmod +x /usr/local/bin/openvpn-web /usr/bin/docker-entrypoint.sh /usr/lib/openvpn/plugins/openvpn-auth /usr/local/bin/ollama /usr/local/bin/ollama-bootstrap.sh

VOLUME ["/data"]

ENV GIN_MODE=release
ENV OVPN_DATA=/data
ENV TZ=Asia/Shanghai
# Ollama 配置：容器内监听本地端口，模型持久化到 /data/ollama/models
ENV OLLAMA_HOST=127.0.0.1:11434
ENV OLLAMA_MODELS=/data/ollama/models
# 首次启动自动拉取默认模型（设为 false 可禁用）
ENV OLLAMA_AUTO_PULL=true
ENV OLLAMA_DEFAULT_MODEL=qwen2.5:7b

EXPOSE 1194/udp 8888

USER root
WORKDIR /data

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["/usr/bin/supervisord"]
