#!/usr/bin/env bash
# Run inside an isolated network namespace/container with NET_ADMIN.
set -Eeuo pipefail
runtime=${1:?entrypoint path required}
export OVPN_DATA
OVPN_DATA=$(mktemp -d)
trap 'rm -rf "$OVPN_DATA"' EXIT
printf '%s\n' '{"system":{"base":{"nft_table_name":"openvpn-nft"}}}' >"$OVPN_DATA/config.json"
set -- test-noop
script_type=test-noop
exec() { :; }
source "$runtime"
unset -f exec
nft add table ip foreign_rules
load_nftconfig
nft add element inet openvpn-nft blacklist_v4 '{ 10.8.0.99 }'
nft add rule inet openvpn-nft forward ip saddr @blacklist_v4 drop
nft add rule inet openvpn-nft forward ip saddr @blacklist_v4 drop
nft list table inet openvpn-nft >"$OVPN_DATA/openvpn.nft"
for attempt in 1 2 3; do
  load_nftconfig
  rules=$(nft list chain inet openvpn-nft forward | grep -c 'ip saddr @blacklist_v4 drop')
  [[ "$rules" == 1 ]] || { echo "FAIL: reload $attempt left $rules duplicate IPv4 rules" >&2; exit 1; }
  nft get element inet openvpn-nft blacklist_v4 '{ 10.8.0.99 }' >/dev/null
  nft list table ip foreign_rules >/dev/null
  nft list table inet openvpn-nft >"$OVPN_DATA/openvpn.nft"
done
before=$(nft list table inet openvpn-nft)
printf '\ninvalid nft configuration\n' >>"$OVPN_DATA/openvpn.nft"
if load_nftconfig; then echo 'FAIL: invalid configuration was accepted' >&2; exit 1; fi
[[ "$(nft list table inet openvpn-nft)" == "$before" ]] || { echo 'FAIL: failed reload changed the active firewall' >&2; exit 1; }
echo 'PASS: repeated reloads deduplicate rules, preserve sets/foreign tables, and roll back invalid batches'
