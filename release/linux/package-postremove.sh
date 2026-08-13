#!/usr/bin/env bash
set -e
systemctl daemon-reload 2>/dev/null || true
# /var/lib/openvpn-web and /etc/openvpn-web intentionally survive package removal.
