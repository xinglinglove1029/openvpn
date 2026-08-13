FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend-builder

WORKDIR /src

# Use pnpm to match the local development dependency layout.
# This avoids React duplicate-instance errors from different package-manager layouts.
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
# 默认使用国内 Go 模块代理（goproxy.cn），可通过 --build-arg GOPROXY=... 覆盖；
# 否则直连 proxy.golang.org 在国内网络下容易 stream INTERNAL_ERROR 中断。
ARG GOPROXY=https://goproxy.cn,direct
RUN GOPROXY=${GOPROXY} go mod download

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

# Alpine runtime image for OpenVPN and the web service.
FROM alpine:3.23.4

RUN apk add --no-cache \
    bash \
    ca-certificates \
    coreutils \
    curl \
    easy-rsa \
    grep \
    iproute2 \
    iptables \
    iptables-legacy \
    jq \
    mailcap \
    nftables \
    openssl \
    openvpn \
    sqlite \
    supervisor \
    tzdata \
  && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
  && echo "Asia/Shanghai" > /etc/timezone

COPY build/docker-entrypoint.sh /usr/bin/docker-entrypoint.sh
COPY build/openvpn-auth /usr/lib/openvpn/plugins/openvpn-auth
COPY build/supervisord.conf /etc/supervisor/conf.d/supervisord.conf
COPY --from=backend-builder /out/openvpn-web /usr/local/bin/openvpn-web

# Windows 下编辑保存的 shell 脚本常为 CRLF 行尾，Linux 内核无法直接执行
# （容器启动报 "no such file or directory"），统一转换为 LF 后再 chmod。
RUN sed -i 's/\r$//' /usr/bin/docker-entrypoint.sh \
 && sed -i 's/\r$//' /usr/lib/openvpn/plugins/openvpn-auth \
 && sed -i 's/\r$//' /etc/supervisor/conf.d/supervisord.conf \
 && chmod +x /usr/local/bin/openvpn-web /usr/bin/docker-entrypoint.sh /usr/lib/openvpn/plugins/openvpn-auth

VOLUME ["/data"]

ENV GIN_MODE=release
ENV OVPN_DATA=/data
ENV TZ=Asia/Shanghai

EXPOSE 1194/udp 8888

USER root
WORKDIR /data

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["/usr/bin/supervisord"]
