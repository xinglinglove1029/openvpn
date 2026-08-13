#!/usr/bin/env bash
set -Eeuo pipefail
DATA_DIR=/var/lib/openvpn-web
ETC_DIR=/etc/openvpn-web
find_easyrsa() {
  local candidate
  candidate=$(command -v easyrsa 2>/dev/null || true)
  if [[ -n "$candidate" && -x "$candidate" ]]; then printf '%s\n' "$candidate"; return 0; fi
  for candidate in /usr/share/easy-rsa/*/easyrsa /usr/share/easy-rsa/easyrsa; do
    if [[ -x "$candidate" ]]; then printf '%s\n' "$candidate"; return 0; fi
  done
  return 1
}
INITIAL_ADMIN_PASSWORD_FILE=$ETC_DIR/initial-admin-password
install -d -m 0700 "$DATA_DIR" "$ETC_DIR"
install -d -m 0755 /usr/lib/openvpn-web
chmod 0700 "$DATA_DIR" "$ETC_DIR"
INITIAL_ADMIN_PASSWORD_CREATED=0
if [[ ! -e "$DATA_DIR/config.json" && ! -e "$INITIAL_ADMIN_PASSWORD_FILE" ]]; then
  password=$(openssl rand -base64 48 | tr -d '\n')
  [[ ${#password} -ge 16 ]] || { echo 'failed to generate a strong initial admin password' >&2; exit 1; }
  (umask 077; printf '%s\n' "$password" >"$INITIAL_ADMIN_PASSWORD_FILE")
  chmod 0600 "$INITIAL_ADMIN_PASSWORD_FILE"
  INITIAL_ADMIN_PASSWORD_CREATED=1
fi
ensure_env_setting() {
  local key=$1 value=$2 env="$ETC_DIR/openvpn-web.env"
  if grep -q "^${key}=" "$env"; then
    sed -i "s|^${key}=.*|${key}=${value}|" "$env"
  else
    printf '%s=%s\n' "$key" "$value" >>"$env"
  fi
}

openvpn_bin=$(command -v openvpn || true)
easyrsa_bin=$(find_easyrsa || true)
[[ -x "$openvpn_bin" ]] || { echo 'OpenVPN executable not found after package installation' >&2; exit 1; }
[[ -n "$easyrsa_bin" && -x "$easyrsa_bin" ]] || { echo 'EasyRSA executable not found after package installation' >&2; exit 1; }
easyrsa_bin=$(readlink -f "$easyrsa_bin")

if [[ ! -e "$ETC_DIR/openvpn-web.env" ]]; then
  : >"$ETC_DIR/openvpn-web.env"
fi
ensure_env_setting OVPN_DATA "$DATA_DIR"
ensure_env_setting GIN_MODE release
ensure_env_setting TZ Asia/Shanghai
ensure_env_setting OPENVPN_BIN "$openvpn_bin"
ensure_env_setting EASYRSA_BIN "$easyrsa_bin"
ensure_env_setting EASYRSA_HOME "$(dirname "$easyrsa_bin")"
ensure_env_setting OPENVPN_WEB_ENTRYPOINT /usr/lib/openvpn-web/openvpn-web-entrypoint.sh
ensure_env_setting OPENVPN_AUTH_PLUGIN /usr/lib/openvpn-web/openvpn-auth
ensure_env_setting OPENVPN_WEB_INITIAL_ADMIN_PASSWORD_FILE "$INITIAL_ADMIN_PASSWORD_FILE"
ensure_env_setting OPENVPN_WEB_SECURE_DATA_PERMISSIONS true
chmod 0600 "$ETC_DIR/openvpn-web.env"

if command -v systemctl >/dev/null 2>&1 && [[ "$(ps -p 1 -o comm= 2>/dev/null || true)" == systemd ]]; then
  systemctl daemon-reload
  systemctl enable openvpn-web.target >/dev/null 2>&1 || true
  systemctl start openvpn-web.target >/dev/null 2>&1 || echo 'warning: services were not started; run systemctl status openvpn-web.target' >&2
fi
if (( INITIAL_ADMIN_PASSWORD_CREATED )); then
  echo "A random initial admin password was stored in $INITIAL_ADMIN_PASSWORD_FILE (root-readable only)." >&2
  echo "Retrieve it with: sudo cat $INITIAL_ADMIN_PASSWORD_FILE" >&2
  echo "Log in as admin, change it immediately, then remove the file." >&2
fi
