FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend-builder

WORKDIR /src

COPY frontend/admin/package*.json ./frontend/admin/
RUN cd frontend/admin && npm ci

COPY frontend/admin ./frontend/admin
COPY internal/openvpnweb/templates/index.html ./internal/openvpnweb/templates/index.html
COPY internal/openvpnweb/templates/static/cropped-openvpn-32x32.png ./internal/openvpnweb/templates/static/cropped-openvpn-32x32.png
RUN cd frontend/admin && npm run build

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

RUN apk add --no-cache \
    bash \
    coreutils \
    curl \
    grep \
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
  && echo "Asia/Shanghai" > /etc/timezone \
  && wget https://github.com/OpenVPN/easy-rsa/releases/download/v3.2.6/EasyRSA-3.2.6.tgz \
  && tar -ozxf EasyRSA-3.2.6.tgz \
  && mv EasyRSA-3.2.6 /usr/share/easy-rsa \
  && rm -rf EasyRSA-3.2.6.tgz \
  && ln -s /usr/share/easy-rsa/easyrsa /usr/local/bin

COPY build/docker-entrypoint.sh /usr/bin/docker-entrypoint.sh
COPY build/openvpn-auth /usr/lib/openvpn/plugins/openvpn-auth
COPY build/supervisord.conf /etc/supervisor/conf.d/supervisord.conf
COPY --from=backend-builder /out/openvpn-web /usr/local/bin/openvpn-web

RUN chmod +x /usr/local/bin/openvpn-web /usr/bin/docker-entrypoint.sh /usr/lib/openvpn/plugins/openvpn-auth

VOLUME ["/data"]

ENV GIN_MODE=release
ENV OVPN_DATA=/data
ENV TZ=Asia/Shanghai

EXPOSE 1194/udp 8888

USER root
WORKDIR /data

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["/usr/bin/supervisord"]
