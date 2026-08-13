#!/bin/bash
# 配置 Docker 镜像加速器并重启 Docker
set -e

cat > /etc/docker/daemon.json <<'EOF'
{
  "registry-mirrors": [
    "https://docker.m.daocloud.io",
    "https://docker.1ms.run",
    "https://docker.xuanyuan.me"
  ]
}
EOF

echo "=== daemon.json ==="
cat /etc/docker/daemon.json

systemctl restart docker
sleep 4
docker info --format 'Mirrors: {{json .RegistryConfig.Mirrors}}'
