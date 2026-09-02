#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
fail() { echo "smoke check failed: $*" >&2; exit 1; }

for f in \
  build/docker-entrypoint.sh \
  build/openvpn-auth \
  release/linux/install.sh \
  release/linux/uninstall.sh \
  release/linux/openvpn-web-entrypoint.sh \
  release/linux/openvpn-auth \
  release/linux/openvpn-web.service \
  release/linux/openvpn-server.service \
  release/linux/openvpn-web.target \
  release/linux/package-postinstall.sh \
  release/linux/package-preremove.sh \
  release/linux/package-postremove.sh; do
  test -s "$ROOT/$f" || fail "$f missing"
done

if grep -R -nE '/data|docker-entrypoint\.sh' \
  "$ROOT/release/linux/openvpn-web.service" \
  "$ROOT/release/linux/openvpn-server.service" >/dev/null; then
  fail 'native unit contains Docker-only path'
fi
for unit in "$ROOT/release/linux/openvpn-web.service" "$ROOT/release/linux/openvpn-server.service"; do
  grep -qx 'UMask=0077' "$unit" || fail "$unit must set UMask=0077"
done

# Every tag release must retain the original cross-platform Web-only archives.
# The native Linux build is deliberately separate so the full server package
# cannot replace the Web-only Linux/Windows/macOS artifacts.
for required in \
  'id: openvpn-web' \
  '      - linux' \
  '      - windows' \
  '      - darwin' \
  'id: web' \
  '      - openvpn-web' \
  'id: full-linux' \
  '      - openvpn-web-native' \
  'dst: openvpn-web-entrypoint.sh' \
  'dst: openvpn-auth'; do
  grep -Fq "$required" "$ROOT/.goreleaser.yml" || fail ".goreleaser.yml missing $required"
done

release_args=$(sed -n '/args: release --clean --skip=validate/p' "$ROOT/.github/workflows/build.yml")
[[ -n "$release_args" ]] || fail 'release workflow does not run the unified GoReleaser release command'

grep -Fq 'ENTRYPOINT_SOURCE="$BUNDLE_DIR/openvpn-web-entrypoint.sh"' "$ROOT/release/linux/install.sh" || fail 'installer does not use archive entrypoint name'
grep -Fq 'AUTH_SOURCE="$BUNDLE_DIR/openvpn-auth"' "$ROOT/release/linux/install.sh" || fail 'installer does not use archive auth name'
grep -Fq 'openssl rand -base64 48' "$ROOT/release/linux/install.sh" || fail 'installer does not generate bootstrap password'
grep -Fq 'OPENVPN_WEB_INITIAL_ADMIN_PASSWORD_FILE' "$ROOT/release/linux/install.sh" || fail 'installer does not configure bootstrap password file'
grep -Fq 'OPENVPN_WEB_SECURE_DATA_PERMISSIONS=true' "$ROOT/release/linux/install.sh" || fail 'installer does not enable native permission hardening'
grep -Fq 'chmod 0700 "$DATA_DIR" "$ETC_DIR"' "$ROOT/release/linux/install.sh" || fail 'installer does not harden state directories'
grep -Fq 'chmod 0600 "$ETC_DIR/openvpn-web.env"' "$ROOT/release/linux/install.sh" || fail 'installer does not harden environment file'
grep -Fq 'openssl rand -base64 48' "$ROOT/release/linux/package-postinstall.sh" || fail 'package postinstall does not generate bootstrap password'
grep -Fq 'OPENVPN_WEB_INITIAL_ADMIN_PASSWORD_FILE' "$ROOT/release/linux/package-postinstall.sh" || fail 'package postinstall does not configure bootstrap password file'
grep -Fq 'ensure_env_setting OPENVPN_WEB_SECURE_DATA_PERMISSIONS true' "$ROOT/release/linux/package-postinstall.sh" || fail 'package postinstall does not enable native permission hardening'
grep -Fq '[ "${OPENVPN_WEB_SECURE_DATA_PERMISSIONS:-false}" = "true" ] || return 0' "$ROOT/build/docker-entrypoint.sh" || fail 'runtime secure-permission mode is not opt-in'
grep -Fq 'chmod 0600 "$profile"' "$ROOT/build/docker-entrypoint.sh" || fail 'client profiles are not restricted'
grep -Fq 'find "$OVPN_DATA/pki/private" -type f -exec chmod 0600 {} +' "$ROOT/build/docker-entrypoint.sh" || fail 'PKI private keys are not restricted'
for runtime in "$ROOT/build/docker-entrypoint.sh" "$ROOT/release/linux/openvpn-web-entrypoint.sh"; do
  grep -Fq 'cleanup_retired_web_audit_rules' "$runtime" || fail "$runtime does not clean retired web-audit rules"
  grep -Fq 'iptables-legacy iptables-nft iptables' "$runtime" || fail "$runtime does not scan every IPv4 iptables backend"
  grep -Fq 'ip6tables-legacy ip6tables-nft ip6tables' "$runtime" || fail "$runtime does not scan every IPv6 iptables backend"
done

if command -v bash >/dev/null 2>&1; then
  bash -n \
    "$ROOT/build/docker-entrypoint.sh" \
    "$ROOT/release/linux/install.sh" \
    "$ROOT/release/linux/uninstall.sh" \
    "$ROOT/release/linux/package-postinstall.sh" \
    "$ROOT/release/linux/package-preremove.sh" \
    "$ROOT/release/linux/package-postremove.sh"
fi

# An upgrade can already have converted `server` to an explicit address pool.
# Ensure the startup migration repairs both a stale net30 directive and a
# missing topology directive, and remains idempotent on Docker and native
# Linux runtimes.
if command -v bash >/dev/null 2>&1; then
  topology_tmp=$(mktemp -d)
  cleanup_smoke_tmp() { rm -rf "${tmp:-}" "${topology_tmp:-}"; }
  trap cleanup_smoke_tmp EXIT
  for runtime in "$ROOT/build/docker-entrypoint.sh" "$ROOT/release/linux/openvpn-web-entrypoint.sh"; do
    for topology_case in net30 missing; do
      case_dir="$topology_tmp/$(basename "$runtime")-$topology_case"
      mkdir -p "$case_dir"
      if [[ "$topology_case" == net30 ]]; then
        topology_line='topology net30'
      else
        topology_line=''
      fi
      cat >"$case_dir/server.conf" <<EOF
$topology_line
mode server
ifconfig 10.8.0.1 255.255.255.0
ifconfig-pool 10.8.0.128 10.8.0.253 255.255.255.0
EOF
      (
        export OVPN_DATA="$case_dir"
        set -- smoke-noop
        script_type=smoke-noop
        # Prevent the entrypoint fallback from replacing this test shell.
        exec() { :; }
        # shellcheck disable=SC1090
        source "$runtime"
        ensure_subnet_topology_for_explicit_pool
      )
      grep -qx 'topology subnet' "$case_dir/server.conf" || fail "$runtime did not enforce subnet topology ($topology_case)"
      [[ "$(grep -Ec '^[[:space:]]*topology[[:space:]]+' "$case_dir/server.conf")" == 1 ]] || fail "$runtime left duplicate topology directives ($topology_case)"
      [[ "$(grep -Ec '^[[:space:]]*push[[:space:]]+\"topology[[:space:]]+subnet\"' "$case_dir/server.conf")" == 1 ]] || fail "$runtime did not push subnet topology ($topology_case)"
      cp "$case_dir/server.conf" "$case_dir/server.conf.before"
      (
        export OVPN_DATA="$case_dir"
        set -- smoke-noop
        script_type=smoke-noop
        # Prevent the entrypoint fallback from replacing this test shell.
        exec() { :; }
        # shellcheck disable=SC1090
        source "$runtime"
        ensure_subnet_topology_for_explicit_pool
      )
      cmp -s "$case_dir/server.conf.before" "$case_dir/server.conf" || fail "$runtime topology migration is not idempotent ($topology_case)"
    done
  done
fi

if ! grep -Fq 'openvpn-web-linux-*' "$ROOT/release/linux/README.md" || \
   ! grep -Fq 'openvpn-web-full-linux-*' "$ROOT/release/linux/README.md"; then
  fail 'README does not document both archive types'
fi

# Reproduce the full archive's top-level layout and make sure install.sh finds
# the renamed runtime files before touching the host.
tmp=$(mktemp -d)
trap 'rm -rf "$tmp" "${topology_tmp:-}"' EXIT
bundle="$tmp/openvpn-web-full-linux-smoke"
mkdir -p "$bundle"
touch "$bundle/openvpn-web"
chmod 0755 "$bundle/openvpn-web"
for f in \
  install.sh uninstall.sh README.md openvpn-web.service openvpn-server.service \
  openvpn-web.target package-postinstall.sh package-preremove.sh package-postremove.sh; do
  cp "$ROOT/release/linux/$f" "$bundle/$f"
done
cp "$ROOT/build/docker-entrypoint.sh" "$bundle/openvpn-web-entrypoint.sh"
cp "$ROOT/build/openvpn-auth" "$bundle/openvpn-auth"
chmod 0755 "$bundle/install.sh" "$bundle/uninstall.sh" "$bundle/openvpn-web-entrypoint.sh" "$bundle/openvpn-auth"
(
  cd "$bundle"
  ./install.sh --dry-run --no-start --data-dir "$tmp/data" >/dev/null
) || fail 'full archive layout does not pass installer dry run'

echo 'native packaging smoke checks passed'
