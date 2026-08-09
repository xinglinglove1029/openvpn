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

# 运行时必须使用 glibc 发行版：官方 Ollama Linux 二进制依赖 fcntl64 等 glibc 符号，
# 在 Alpine/musl（即使安装 gcompat）中会启动失败。
FROM debian:bookworm-slim

# Ollama 版本，可通过 --build-arg 覆盖
ARG OLLAMA_VERSION=0.32.5
# buildx 自动注入的平台变量，需显式 ARG 声明才能在 RUN 中使用
ARG TARGETPLATFORM
ARG TARGETARCH
ARG TARGETVARIANT
ARG BUILDPLATFORM

# 安装系统依赖 + Ollama。
# Debian 提供 glibc，避免官方 Ollama 在 Alpine/musl 中出现
# “fcntl64: symbol not found”并导致 supervisor 反复重启。
# arm/v7 无官方 Ollama 二进制，创建明确失败的 stub。
RUN set -eux \
  && apt-get update \
  && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    coreutils \
    curl \
    easy-rsa \
    grep \
    iproute2 \
    iptables \
    jq \
    libgomp1 \
    mailcap \
    nftables \
    openssl \
    openvpn \
    sqlite3 \
    supervisor \
    tzdata \
    zstd \
  && rm -rf /var/lib/apt/lists/* \
  && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
  && echo "Asia/Shanghai" > /etc/timezone \
  && if [ "${TARGETARCH}" = "amd64" ] || [ "${TARGETARCH}" = "arm64" ]; then \
       mkdir -p /usr/local/lib/ollama \
       && echo "Downloading Ollama ${OLLAMA_VERSION} for ${TARGETPLATFORM}" \
       && curl -fsSL \
            --retry 3 \
            --retry-delay 5 \
            --connect-timeout 30 \
            --max-time 1200 \
            "https://ollama.com/download/ollama-linux-${TARGETARCH}.tar.zst?version=${OLLAMA_VERSION}" \
            -o /tmp/ollama.tar.zst \
       && zstd -d /tmp/ollama.tar.zst -o /tmp/ollama.tar \
       && tar -xf /tmp/ollama.tar -C /usr/local \
       && rm -f /tmp/ollama.tar.zst /tmp/ollama.tar \
       && chmod +x /usr/local/bin/ollama \
       && echo "Ollama installed: $(du -sh /usr/local/lib/ollama | cut -f1)"; \
     else \
       echo "Ollama not available for ${TARGETPLATFORM}, creating stub" \
       && printf '#!/bin/sh\necho "ollama not supported on this architecture" >&2\nexit 1\n' > /usr/local/bin/ollama \
       && chmod +x /usr/local/bin/ollama; \
     fi

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
