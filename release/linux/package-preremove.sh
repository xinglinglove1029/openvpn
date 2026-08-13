#!/usr/bin/env bash
set -e
if command -v systemctl >/dev/null 2>&1 && [[ "$(ps -p 1 -o comm= 2>/dev/null || true)" == systemd ]]; then
  systemctl disable --now openvpn-web.target openvpn-server.service openvpn-web.service 2>/dev/null || true
fi
