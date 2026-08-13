#!/usr/bin/env bash
set -Eeuo pipefail
PURGE=0
YES=0
while (($#)); do
  case "$1" in
    --purge) PURGE=1; shift ;;
    --yes) YES=1; shift ;;
    -h|--help) echo 'Usage: sudo ./uninstall.sh [--purge --yes]'; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 1 ;;
  esac
done
(( EUID == 0 )) || { echo 'run as root' >&2; exit 1; }
if (( PURGE && ! YES )); then echo '--purge requires --yes; data was not removed' >&2; exit 1; fi
if command -v systemctl >/dev/null 2>&1 && [[ "$(ps -p 1 -o comm= 2>/dev/null || true)" == systemd ]]; then
  systemctl disable --now openvpn-web.target openvpn-server.service openvpn-web.service 2>/dev/null || true
  systemctl daemon-reload || true
fi
rm -f /etc/systemd/system/openvpn-web.target /etc/systemd/system/openvpn-server.service /etc/systemd/system/openvpn-web.service
rm -rf /usr/local/bin/openvpn-web /usr/lib/openvpn-web /etc/openvpn-web /etc/sysctl.d/99-openvpn-web.conf
if (( PURGE )); then rm -rf /var/lib/openvpn-web; fi
printf 'openvpn-web removed; persistent data %s\n' "$( (( PURGE )) && echo 'was purged' || echo 'was preserved in /var/lib/openvpn-web' )"
