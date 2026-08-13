#!/usr/bin/env bash
set -Eeuo pipefail

PROGRAM=openvpn-web
DATA_DIR=/var/lib/openvpn-web
ETC_DIR=/etc/openvpn-web
RUNTIME_DIR=/usr/lib/openvpn-web
BIN_DIR=/usr/local/bin
INITIAL_ADMIN_PASSWORD_FILE=$ETC_DIR/initial-admin-password
INITIAL_ADMIN_PASSWORD_CREATED=0
NO_START=0
DRY_RUN=0

log() { printf '[openvpn-web] %s\n' "$*"; }
die() { printf '[openvpn-web] ERROR: %s\n' "$*" >&2; exit 1; }
run() { log "+ $*"; if (( ! DRY_RUN )); then "$@"; fi; }

usage() {
  cat <<EOF
Usage: sudo ./install.sh [options]

Options:
  --data-dir DIR   Persistent data directory (default: /var/lib/openvpn-web)
  --web-port PORT  Initial Web port when creating config.json (default: 8888)
  --ovpn-port PORT Initial OpenVPN port when creating config.json (default: 1194)
  --no-start       Install files but do not start systemd services

On a new installation, a random initial admin password is stored in
/etc/openvpn-web/initial-admin-password (root-readable only). Retrieve it with
sudo cat /etc/openvpn-web/initial-admin-password, log in as admin, change it
immediately, then delete that file.
  --dry-run        Print actions without changing the host
  -h, --help       Show this help
EOF
}

WEB_PORT=8888
OVPN_PORT=1194
while (($#)); do
  case "$1" in
    --data-dir) DATA_DIR=${2:?missing value for --data-dir}; shift 2 ;;
    --web-port) WEB_PORT=${2:?missing value for --web-port}; shift 2 ;;
    --ovpn-port) OVPN_PORT=${2:?missing value for --ovpn-port}; shift 2 ;;
    --no-start) NO_START=1; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

validate_port() {
  local name=$1 value=$2
  [[ "$value" =~ ^[1-9][0-9]{0,4}$ ]] || die "$name must be an integer between 1 and 65535"
  (( 10#$value <= 65535 )) || die "$name must be an integer between 1 and 65535"
}
validate_port "--web-port" "$WEB_PORT"
validate_port "--ovpn-port" "$OVPN_PORT"

BUNDLE_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
if [[ -f "$BUNDLE_DIR/openvpn-web-entrypoint.sh" && -f "$BUNDLE_DIR/openvpn-auth" ]]; then
  # Full release bundle.
  ENTRYPOINT_SOURCE="$BUNDLE_DIR/openvpn-web-entrypoint.sh"
  AUTH_SOURCE="$BUNDLE_DIR/openvpn-auth"
elif [[ -f "$BUNDLE_DIR/runtime/openvpn-web-entrypoint.sh" && -f "$BUNDLE_DIR/runtime/openvpn-auth" ]]; then
  # Compatibility with early bundle layouts.
  ENTRYPOINT_SOURCE="$BUNDLE_DIR/runtime/openvpn-web-entrypoint.sh"
  AUTH_SOURCE="$BUNDLE_DIR/runtime/openvpn-auth"
elif [[ -f "$BUNDLE_DIR/../../build/docker-entrypoint.sh" && -f "$BUNDLE_DIR/../../build/openvpn-auth" ]]; then
  # Repository checkout developer workflow.
  ENTRYPOINT_SOURCE="$BUNDLE_DIR/../../build/docker-entrypoint.sh"
  AUTH_SOURCE="$BUNDLE_DIR/../../build/openvpn-auth"
else
  die "runtime scripts are missing from this bundle"
fi

if (( ! DRY_RUN )) && (( EUID != 0 )); then die "run as root (for example: sudo ./install.sh)"; fi

ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) EXPECTED_ARCH=x86_64 ;;
  aarch64|arm64) EXPECTED_ARCH=aarch64 ;;
  armv7l|armv7*) EXPECTED_ARCH=armv7l ;;
  armv6l) EXPECTED_ARCH=armv6l ;;
  *) die "unsupported Linux architecture: $ARCH" ;;
esac

if [[ -r /etc/os-release ]]; then . /etc/os-release; else die '/etc/os-release is required'; fi
if [[ "${ID_LIKE:-} ${ID:-}" =~ (debian|ubuntu) ]]; then FAMILY=debian
elif [[ "${ID_LIKE:-} ${ID:-}" =~ (rhel|fedora|centos|rocky|almalinux) ]]; then FAMILY=rpm
else die "unsupported Linux distribution: ${ID:-unknown}"; fi

find_easyrsa() {
  local candidate
  candidate=$(command -v easyrsa 2>/dev/null || true)
  if [[ -n "$candidate" && -x "$candidate" ]]; then printf '%s\n' "$candidate"; return 0; fi
  for candidate in /usr/share/easy-rsa/*/easyrsa /usr/share/easy-rsa/easyrsa; do
    if [[ -x "$candidate" ]]; then printf '%s\n' "$candidate"; return 0; fi
  done
  return 1
}

install_dependencies() {
  (( DRY_RUN )) && { log "would install ${FAMILY} runtime dependencies"; return; }
  if [[ "$FAMILY" == debian ]]; then
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y \
      bash ca-certificates curl jq openssl openvpn iproute2 iptables nftables sqlite3 easy-rsa
  else
    dnf install -y bash ca-certificates curl jq openssl openvpn iproute iptables nftables sqlite easy-rsa
  fi
  if ! find_easyrsa >/dev/null; then
    die 'EasyRSA is not installed correctly; install the easy-rsa package and retry'
  fi
}

create_initial_admin_password() {
  local config="$DATA_DIR/config.json"
  [[ -e "$config" ]] && return
  if (( DRY_RUN )); then
    log "would generate a root-only initial admin password at $INITIAL_ADMIN_PASSWORD_FILE"
    return
  fi

  install -d -m 0700 "$DATA_DIR" "$ETC_DIR"
  chmod 0700 "$DATA_DIR" "$ETC_DIR"
  if [[ -e "$INITIAL_ADMIN_PASSWORD_FILE" ]]; then
    [[ -f "$INITIAL_ADMIN_PASSWORD_FILE" ]] || die "$INITIAL_ADMIN_PASSWORD_FILE must be a regular file"
    chmod 0600 "$INITIAL_ADMIN_PASSWORD_FILE"
    return
  fi

  local password
  password=$(openssl rand -base64 48 | tr -d '\n')
  [[ ${#password} -ge 16 ]] || die 'failed to generate a strong initial admin password'
  (umask 077; printf '%s\n' "$password" >"$INITIAL_ADMIN_PASSWORD_FILE")
  chmod 0600 "$INITIAL_ADMIN_PASSWORD_FILE"
  INITIAL_ADMIN_PASSWORD_CREATED=1
}

write_initial_config() {
  local config="$DATA_DIR/config.json"
  if [[ -e "$config" ]]; then return; fi
  create_initial_admin_password
  run install -d -m 0700 "$DATA_DIR"
  if (( ! DRY_RUN )); then
    cat >"$config" <<EOF
{"system":{"base":{"web_port":"$WEB_PORT"}},"openvpn":{"ovpn_port":$OVPN_PORT}}
EOF
    chmod 0600 "$config"
  else
    log "would create $config with Web port $WEB_PORT and OpenVPN port $OVPN_PORT"
  fi
}

install_files() {
  run install -d -m 0700 "$DATA_DIR" "$ETC_DIR"
  run install -d -m 0755 "$RUNTIME_DIR" "$BIN_DIR"
  if (( ! DRY_RUN )); then chmod 0700 "$DATA_DIR" "$ETC_DIR"; fi
  local binary="$BUNDLE_DIR/openvpn-web"
  [[ -f "$binary" ]] || binary="$BUNDLE_DIR/openvpn-web-linux"
  [[ -f "$binary" ]] || die "openvpn-web binary is missing from this bundle"
  run install -m 0755 "$binary" "$BIN_DIR/openvpn-web"
  run install -m 0755 "$ENTRYPOINT_SOURCE" "$RUNTIME_DIR/openvpn-web-entrypoint.sh"
  run install -m 0755 "$AUTH_SOURCE" "$RUNTIME_DIR/openvpn-auth"
  local web_unit="$BUNDLE_DIR/openvpn-web.service"
  local server_unit="$BUNDLE_DIR/openvpn-server.service"
  local target_unit="$BUNDLE_DIR/openvpn-web.target"
  [[ -f "$web_unit" ]] || web_unit="$BUNDLE_DIR/systemd/openvpn-web.service"
  [[ -f "$server_unit" ]] || server_unit="$BUNDLE_DIR/systemd/openvpn-server.service"
  [[ -f "$target_unit" ]] || target_unit="$BUNDLE_DIR/systemd/openvpn-web.target"
  run install -m 0644 "$web_unit" /etc/systemd/system/openvpn-web.service
  run install -m 0644 "$server_unit" /etc/systemd/system/openvpn-server.service
  run install -m 0644 "$target_unit" /etc/systemd/system/openvpn-web.target
  if (( ! DRY_RUN )); then
    cat >"$ETC_DIR/openvpn-web.env" <<EOF
OVPN_DATA=$DATA_DIR
GIN_MODE=release
TZ=Asia/Shanghai
OPENVPN_BIN=$(command -v openvpn)
EASYRSA_BIN=$(find_easyrsa)
EASYRSA_HOME=$(dirname "$(readlink -f "$(find_easyrsa)")")
OPENVPN_WEB_ENTRYPOINT=$RUNTIME_DIR/openvpn-web-entrypoint.sh
OPENVPN_AUTH_PLUGIN=$RUNTIME_DIR/openvpn-auth
OPENVPN_WEB_INITIAL_ADMIN_PASSWORD_FILE=$INITIAL_ADMIN_PASSWORD_FILE
OPENVPN_WEB_SECURE_DATA_PERMISSIONS=true
EOF
    chmod 0600 "$ETC_DIR/openvpn-web.env"
  else
    log "would write $ETC_DIR/openvpn-web.env"
  fi
}

configure_host() {
  if (( DRY_RUN )); then return; fi
  install -d -m 0755 /etc/sysctl.d
  cat >/etc/sysctl.d/99-openvpn-web.conf <<EOF
net.ipv4.ip_forward=1
net.ipv6.conf.all.forwarding=1
EOF
  sysctl --system >/dev/null || log 'warning: could not apply forwarding sysctl; apply it manually before enabling VPN gateway mode'
}

start_services() {
  if (( NO_START || DRY_RUN )); then
    log 'services were not started (--no-start/--dry-run)'
    return
  fi
  if ! command -v systemctl >/dev/null 2>&1 || [[ "$(ps -p 1 -o comm= 2>/dev/null || true)" != systemd ]]; then
    log 'systemd is unavailable in this environment; files were installed, start openvpn-web.target on a systemd host'
    return
  fi
  systemctl daemon-reload
  systemctl enable openvpn-web.target
  systemctl restart openvpn-web.target
}

log "installing complete Linux server bundle for $EXPECTED_ARCH (${ID:-unknown})"
install_dependencies
write_initial_config
install_files
configure_host
start_services
if (( INITIAL_ADMIN_PASSWORD_CREATED )); then
  log "a random initial admin password was stored in $INITIAL_ADMIN_PASSWORD_FILE (root-readable only)"
  log "retrieve it with: sudo cat $INITIAL_ADMIN_PASSWORD_FILE"
  log "log in as admin, change the password immediately, then remove the file: sudo rm -f $INITIAL_ADMIN_PASSWORD_FILE"
fi
log "installation complete; persistent data: $DATA_DIR; Web UI: http://<server-ip>:$WEB_PORT"
