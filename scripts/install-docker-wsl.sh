#!/bin/bash
# 在 WSL Ubuntu 26.04 中安装 Docker CE（官方仓库方式）
set -e

echo "=== [1/6] apt update & install deps ==="
apt-get update -y
apt-get install -y ca-certificates curl gnupg

echo "=== [2/6] add Docker GPG key ==="
install -m 0755 -d /etc/apt/keyrings
if [ ! -f /etc/apt/keyrings/docker.gpg ]; then
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor --yes -o /etc/apt/keyrings/docker.gpg
  chmod a+r /etc/apt/keyrings/docker.gpg
fi

echo "=== [3/6] add Docker apt repo (resolute) ==="
cat > /etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: resolute
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.gpg
EOF

echo "=== [4/6] apt update (docker repo) ==="
apt-get update -y

echo "=== [5/6] install docker-ce + buildx + compose ==="
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

echo "=== [6/6] verify ==="
docker --version
docker compose version
docker buildx version
