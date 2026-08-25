#!/bin/bash
set -e

SYSTEM_CONFIG="$OVPN_DATA/config.json"
export EASYRSA_PKI="$OVPN_DATA/pki"

# Docker keeps the historical paths; native Linux packages override these via systemd.
OPENVPN_BIN="${OPENVPN_BIN:-/usr/sbin/openvpn}"
EASYRSA_HOME="${EASYRSA_HOME:-/usr/share/easy-rsa}"
EASYRSA_BIN="${EASYRSA_BIN:-$EASYRSA_HOME/easyrsa}"
OPENVPN_WEB_ENTRYPOINT="${OPENVPN_WEB_ENTRYPOINT:-$0}"
OPENVPN_AUTH_PLUGIN="${OPENVPN_AUTH_PLUGIN:-/usr/lib/openvpn/plugins/openvpn-auth}"

# Native systemd units opt in to normalizing sensitive state. Docker keeps its
# historical directory behavior unless explicitly opted in.
secure_data_permissions() {
	[ "${OPENVPN_WEB_SECURE_DATA_PERMISSIONS:-false}" = "true" ] || return 0
	install -d -m 0700 "$OVPN_DATA"
	chmod 0700 "$OVPN_DATA"
	for directory in "$OVPN_DATA/pki" "$OVPN_DATA/pki/private" "$OVPN_DATA/clients" "$OVPN_DATA/ccd"; do
		[ -d "$directory" ] || continue
		chmod 0700 "$directory"
	done
	if [ -d "$OVPN_DATA/pki/private" ]; then
		find "$OVPN_DATA/pki/private" -type f -exec chmod 0600 {} +
	fi
	if [ -d "$OVPN_DATA/clients" ]; then
		find "$OVPN_DATA/clients" -type f -name '*.ovpn' -exec chmod 0600 {} +
	fi
}

# Debian, RPM and Alpine packages use different EasyRSA locations. Resolve it
# before executing PKI operations, while preserving the historical Docker path.
ensure_easyrsa() {
	if [ ! -x "$EASYRSA_BIN" ]; then
		local candidate=""
		candidate=$(command -v easyrsa 2>/dev/null || true)
		if [ -n "$candidate" ] && [ -x "$candidate" ]; then
			EASYRSA_BIN="$candidate"
		fi
	fi
	if [ ! -x "$EASYRSA_BIN" ]; then
		for candidate in /usr/share/easy-rsa/*/easyrsa /usr/share/easy-rsa/easyrsa; do
			if [ -x "$candidate" ]; then
				EASYRSA_BIN="$candidate"
				break
			fi
		done
	fi
	if [ ! -x "$EASYRSA_BIN" ]; then
		echo "EasyRSA executable not found (looked for $EASYRSA_BIN)" >&2
		return 1
	fi
	if command -v readlink >/dev/null 2>&1; then
		local resolved
		resolved=$(readlink -f "$EASYRSA_BIN" 2>/dev/null || printf '%s' "$EASYRSA_BIN")
		EASYRSA_BIN="$resolved"
		EASYRSA_HOME=$(dirname "$resolved")
	fi
}

wait_for_runtime_config() {
	local attempt=0
	while [ "$attempt" -lt 60 ]; do
		if [ -s "$SYSTEM_CONFIG" ] && jq -e -r '.system.base.server_name // empty' "$SYSTEM_CONFIG" >/dev/null 2>&1; then
			return 0
		fi
		attempt=$((attempt + 1))
		sleep 1
	done
	echo "Timed out waiting for openvpn-web to initialize $SYSTEM_CONFIG" >&2
	return 1
}

init_pki() {
	ensure_easyrsa
	SERVER_NAME=$(jq -r '.system.base.server_name // ""' $SYSTEM_CONFIG)
	SERVER_CN=$(jq -r '.system.base.server_cn // ""' $SYSTEM_CONFIG)
	cd $OVPN_DATA && $EASYRSA_BIN init-pki

	cat <<EOF >$EASYRSA_PKI/vars
set_var EASYRSA "$EASYRSA_HOME"
set_var EASYRSA_CA_EXPIRE 365
set_var EASYRSA_CERT_EXPIRE 365
set_var EASYRSA_CRL_DAYS 365
set_var EASYRSA_ALGO ec
set_var EASYRSA_CURVE prime256v1
EOF

	$EASYRSA_BIN --batch --req-cn="$SERVER_CN" build-ca nopass
	$EASYRSA_BIN --batch build-server-full "$SERVER_NAME" nopass
	$EASYRSA_BIN gen-crl
	$OPENVPN_BIN --genkey secret $EASYRSA_PKI/tc.key
}

init_config() {
	SERVER_NAME=$(jq -r '.system.base.server_name // ""' $SYSTEM_CONFIG)
	OVPN_PORT=$(jq -r '.openvpn.ovpn_port // "1194"' $SYSTEM_CONFIG)
	OVPN_PROTO=$(jq -r '.openvpn.ovpn_proto // "udp"' $SYSTEM_CONFIG)
	OVPN_MAXCLIENTS=$(jq -r '.openvpn.ovpn_maxclients // "200"' $SYSTEM_CONFIG)
	OVPN_MANAGEMENT=$(jq -r '.openvpn.ovpn_management // "127.0.0.1:7505"' $SYSTEM_CONFIG)
	OVPN_IPV6=$(jq -r '.openvpn.ovpn_ipv6 // "false"' $SYSTEM_CONFIG)
	OVPN_GATEWAY=$(jq -r '.openvpn.ovpn_gateway // "true"' $SYSTEM_CONFIG)
	OVPN_SUBNET=$(jq -r '.openvpn.ovpn_subnet // "10.8.0.0/24"' $SYSTEM_CONFIG)
	OVPN_SUBNET6=$(jq -r '.openvpn.ovpn_subnet6 // "fdaf:f178:e916:6dd0::/64"' $SYSTEM_CONFIG)
	OVPN_DNS1=$(jq -r '.openvpn.ovpn_push_dns1 // "8.8.8.8"' $SYSTEM_CONFIG)
	OVPN_DNS2=$(jq -r '.openvpn.ovpn_push_dns2 // "2001:4860:4860::8888"' $SYSTEM_CONFIG)
	# Avoid malformed settings injecting additional server.conf directives. DNS
	# audit only supports literal IP upstreams, so retain safe defaults otherwise.
	[[ "$OVPN_DNS1" =~ ^[0-9A-Fa-f:.]+$ ]] || OVPN_DNS1="8.8.8.8"
	[[ "$OVPN_DNS2" =~ ^[0-9A-Fa-f:.]+$ ]] || OVPN_DNS2="2001:4860:4860::8888"
	WEB_PORT=$(jq -r '.system.base.web_port // "8888"' $SYSTEM_CONFIG)

	cat <<EOF >$OVPN_DATA/server.conf
port $OVPN_PORT
proto $OVPN_PROTO
dev tun
persist-key
persist-tun
keepalive 10 60
topology subnet
$([[ "$OVPN_IPV6" == "true" ]] && echo -e "server $(getsubnet $OVPN_SUBNET)\nserver-ipv6 $OVPN_SUBNET6" || echo "server $(getsubnet $OVPN_SUBNET)")
$([[ "$OVPN_GATEWAY" == "true" ]] && printf 'push "dhcp-option DNS %s"\npush "dhcp-option DNS %s"\npush "redirect-gateway def1 ipv6 bypass-dhcp"' "$OVPN_DNS1" "$OVPN_DNS2" || printf '#push "dhcp-option DNS %s"\n#push "dhcp-option DNS %s"\n#push "redirect-gateway def1 ipv6 bypass-dhcp"' "$OVPN_DNS1" "$OVPN_DNS2")
dh none
tls-groups prime256v1
tls-crypt $EASYRSA_PKI/tc.key
crl-verify $EASYRSA_PKI/crl.pem
ca $EASYRSA_PKI/ca.crt
cert $EASYRSA_PKI/issued/$SERVER_NAME.crt
key $EASYRSA_PKI/private/$SERVER_NAME.key
auth SHA256
cipher AES-128-GCM
data-ciphers AES-128-GCM
tls-server
tls-version-min 1.2
tls-cipher TLS-ECDHE-ECDSA-WITH-AES-128-GCM-SHA256
auth-user-pass-verify $OPENVPN_AUTH_PLUGIN via-env
client-disconnect $OPENVPN_WEB_ENTRYPOINT
client-connect $OPENVPN_WEB_ENTRYPOINT
script-security 3
status $OVPN_DATA/openvpn-status.log
client-config-dir $OVPN_DATA/ccd
duplicate-cn
client-to-client
max-clients $OVPN_MAXCLIENTS
management ${OVPN_MANAGEMENT/:/ }
verb 2
$([[ "$OVPN_PROTO" =~ "udp" ]] && echo "explicit-exit-notify 1")
setenv ovpn_data ${OVPN_DATA:-/data}
setenv auth_api http://127.0.0.1:$WEB_PORT/login
setenv ovpn_auth_api http://127.0.0.1:$WEB_PORT/ovpn/login
setenv ovpn_history_api http://127.0.0.1:$WEB_PORT/ovpn/history
setenv ovpn_notify_api http://127.0.0.1:$WEB_PORT/ovpn/notify
setenv ovpn_web_audit_client_map_api http://127.0.0.1:$WEB_PORT/ovpn/web-audit/client-map
EOF
}


# Website-audit firewall rules used to be managed by this entrypoint. Older
# releases may therefore have rules in either xtables backend (legacy or nft),
# while the current Go service owns only its comment-tagged rules. Do not select
# only the active backend here: mixed upgrades leave the inactive table intact.
cleanup_legacy_dns_audit_redirect() {
	local ipt proto
	for ipt in iptables-legacy iptables-nft iptables; do
		command -v "$ipt" >/dev/null 2>&1 || continue
		for proto in udp tcp; do
			# The old entrypoint installed this exact broad redirect without an
			# owner comment. Remove every duplicate so disabled web audit cannot
			# black-hole VPN DNS at the unbound local 5353 port.
			while "$ipt" -t nat -C PREROUTING -i tun0 -p "$proto" --dport 53 -j REDIRECT --to-ports 5353 >/dev/null 2>&1; do
				"$ipt" -t nat -D PREROUTING -i tun0 -p "$proto" --dport 53 -j REDIRECT --to-ports 5353 >/dev/null 2>&1 || break
				echo "[OPENVPN-WEB] removed legacy DNS audit redirect (${ipt}, ${proto}/53)" >&2
			done
		done
	done
}

# UDP/443 blocking was retired because it breaks QUIC-first services such as
# Google and YouTube. It may still exist in a backend the Go process did not
# select, so purge only the old comment-owned rule from every available table.
cleanup_retired_web_audit_quic_block() {
	local ipt reject
	for ipt in iptables-legacy iptables-nft iptables; do
		command -v "$ipt" >/dev/null 2>&1 || continue
		reject="icmp-port-unreachable"
		while "$ipt" -t filter -C FORWARD -i tun0 -p udp -m comment --comment "openvpn-web:web-audit:quic-block" --dport 443 -j REJECT --reject-with "$reject" >/dev/null 2>&1; do
			"$ipt" -t filter -D FORWARD -i tun0 -p udp -m comment --comment "openvpn-web:web-audit:quic-block" --dport 443 -j REJECT --reject-with "$reject" >/dev/null 2>&1 || break
			echo "[OPENVPN-WEB] removed retired QUIC block (${ipt}, udp/443)" >&2
		done
	done
	for ipt in ip6tables-legacy ip6tables-nft ip6tables; do
		command -v "$ipt" >/dev/null 2>&1 || continue
		reject="icmp6-port-unreachable"
		while "$ipt" -t filter -C FORWARD -i tun0 -p udp -m comment --comment "openvpn-web:web-audit:quic-block" --dport 443 -j REJECT --reject-with "$reject" >/dev/null 2>&1; do
			"$ipt" -t filter -D FORWARD -i tun0 -p udp -m comment --comment "openvpn-web:web-audit:quic-block" --dport 443 -j REJECT --reject-with "$reject" >/dev/null 2>&1 || break
			echo "[OPENVPN-WEB] removed retired IPv6 QUIC block (${ipt}, udp/443)" >&2
		done
	done
}

cleanup_retired_web_audit_rules() {
	cleanup_legacy_dns_audit_redirect
	cleanup_retired_web_audit_quic_block
}

run_server() {
	mkdir -p /dev/net
	if [ ! -c /dev/net/tun ]; then
		mknod /dev/net/tun c 10 200
	fi

	ipt="iptables-nft"
	if iptables-legacy -L -n -t nat >/dev/null 2>&1; then
		ipt="iptables-legacy"
	fi

	config=$OVPN_DATA/server.conf

	ovpn_subnet=$(awk '$1=="server"{print $2, $3}' $config)
	$ipt -t nat -C POSTROUTING -s ${ovpn_subnet/ /\/} -j MASQUERADE >/dev/null 2>&1 || {
		$ipt -t nat -A POSTROUTING -s ${ovpn_subnet/ /\/} -j MASQUERADE
	}

	if [ "$(jq -r '.openvpn.ovpn_ipv6 // false' "$SYSTEM_CONFIG")" = "true" ]; then
		ovpn_subnet6=$(awk '$1=="server-ipv6"{print $2, $3}' $config)
		${ipt/iptables/ip6tables} -t nat -C POSTROUTING -s $ovpn_subnet6 -j MASQUERADE >/dev/null 2>&1 || {
			${ipt/iptables/ip6tables} -t nat -A POSTROUTING -s $ovpn_subnet6 -j MASQUERADE
		}
	fi

	# Clear retired website-audit rules from every xtables backend before OpenVPN
	# starts. This covers legacy/nft mixed upgrades even if the selected backend
	# below remains responsible only for current NAT rules.
	cleanup_retired_web_audit_rules

	$OPENVPN_BIN $OVPN_DATA/server.conf
}

renew_cert() {
	ensure_easyrsa
	DAYS="${1:-1095}"
	case "$DAYS" in
		''|*[!0-9]*) DAYS=1095 ;;
	esac

	SERVER_NAME=$(jq -r '.system.base.server_name // ""' $SYSTEM_CONFIG)

	#cd $EASYRSA_PKI
	#openssl x509 -in ca.crt -days $DAYS -out ca.crt -signkey private/ca.key
	$EASYRSA_BIN --batch "--days=$DAYS" renew-ca
	$EASYRSA_BIN --batch "--days=$DAYS" renew "$SERVER_NAME"
	$EASYRSA_BIN --batch revoke-renewed "$SERVER_NAME"
	$EASYRSA_BIN --batch "--days=$DAYS" gen-crl
}

auth() {
	if [ "$1" = "true" ]; then
		sed -i 's/^#auth-user-pass-verify/auth-user-pass-verify/' $OVPN_DATA/server.conf
	else
		sed -i 's/^auth-user-pass-verify/#&/' $OVPN_DATA/server.conf
	fi
}

getsubnet() {
	ip=$(echo $1 | cut -d'/' -f1)
	prefix=$(echo $1 | cut -d'/' -f2)

	mask=""
	for i in {1..4}; do
		if [ $prefix -ge 8 ]; then
			mask+="255"
			prefix=$((prefix - 8))
		else
			mask+=$((256 - 2 ** (8 - prefix)))
			prefix=0
		fi

		if [ $i -lt 4 ]; then
			mask+="."
		fi
	done
	echo $ip $mask
}

genclient() {
	ensure_easyrsa
	SERVER_NAME=$(jq -r '.system.base.server_name // ""' $SYSTEM_CONFIG)
	OVPN_PROTO=$(jq -r '.openvpn.ovpn_proto // "udp"' $SYSTEM_CONFIG)
	OVPN_PORT=$(jq -r '.openvpn.ovpn_port // "1194"' $SYSTEM_CONFIG)
	OVPN_IPV6=$(jq -r '.openvpn.ovpn_ipv6 // "false"' $SYSTEM_CONFIG)

	CLIENT_CERT="$EASYRSA_PKI/issued/$1.crt"
	CLIENT_KEY="$EASYRSA_PKI/private/$1.key"

	# A deleted client certificate remains revoked in crl.pem. Reusing its key and
	# certificate would produce a new .ovpn that OpenVPN rejects during TLS verification.
	if [ -f "$CLIENT_CERT" ] && [ -f "$CLIENT_KEY" ]; then
		if ! VERIFY_OUTPUT=$(openssl verify -crl_check -CAfile "$EASYRSA_PKI/ca.crt" -CRLfile "$EASYRSA_PKI/crl.pem" "$CLIENT_CERT" 2>&1); then
			if printf '%s\n' "$VERIFY_OUTPUT" | grep -qi 'certificate revoked'; then
				# The CRL is the revocation record of authority. Removing only the local
				# artifacts permits EasyRSA to issue a fresh certificate with a new serial.
				rm -f "$CLIENT_CERT" "$CLIENT_KEY"
			else
				echo "Refusing to reuse client certificate for $1: $VERIFY_OUTPUT" >&2
				return 1
			fi
		fi
	elif [ -e "$CLIENT_CERT" ] || [ -e "$CLIENT_KEY" ]; then
		# A partial credential pair cannot generate a usable client profile.
		rm -f "$CLIENT_CERT" "$CLIENT_KEY"
	fi

	if [ ! -f "$CLIENT_CERT" ] || [ ! -f "$CLIENT_KEY" ]; then
		$EASYRSA_BIN --batch build-client-full "$1" nopass >/dev/null
	fi
	install -d -m 0700 "$OVPN_DATA/clients"
	local profile="$OVPN_DATA/clients/$1.ovpn"
	( umask 077; cat <<EOF >"$profile"
client
proto $([[ "$OVPN_IPV6" == "true" ]] && [[ ! "$OVPN_PROTO" =~ 6 ]] && echo "${OVPN_PROTO}6" || echo $OVPN_PROTO)
remote ${2:-$([[ "$OVPN_IPV6" == "true" ]] && ip -6 route get 2001:4860:4860::8888 | grep -oP 'src \K\S+' || ip -4 route get 8.8.8.8 | grep -oP 'src \K\S+')} ${3:-$OVPN_PORT}
dev tun
resolv-retry infinite
nobind
persist-key
persist-tun
remote-cert-tls server
verify-x509-name $SERVER_NAME name
auth SHA256
$(grep -q '^auth-user-pass-verify' $OVPN_DATA/server.conf && echo 'auth-user-pass' || echo '#auth-user-pass')
cipher AES-128-GCM
tls-client
tls-version-min 1.2
tls-cipher TLS-ECDHE-ECDSA-WITH-AES-128-GCM-SHA256
verb 3
$([[ "$OVPN_IPV6" == "true" ]] && echo -e "tun-mtu 1400\nmssfix 1360")
$([[ "$OVPN_PROTO" =~ "udp" ]] && echo "explicit-exit-notify")
$([[ "$5" == "true" ]] && echo 'static-challenge "Enter MFA code" 1')

## Custom configuration ##
$(echo -e $4)
## end ##

<ca>
$(cat $EASYRSA_PKI/ca.crt)
</ca>
<cert>
$(openssl x509 -in $EASYRSA_PKI/issued/$1.crt)
</cert>
<key>
$(cat $EASYRSA_PKI/private/$1.key)
</key>
<tls-crypt>
$(cat $EASYRSA_PKI/tc.key)
</tls-crypt>
EOF
	)
	chmod 0600 "$profile"
	secure_data_permissions
}

check_config() {
	config=$OVPN_DATA/server.conf
	grep -q "^client-connect" $config || echo "client-connect $OPENVPN_WEB_ENTRYPOINT" >>$config
	grep -q "^client-disconnect" $config || echo "client-disconnect $OPENVPN_WEB_ENTRYPOINT" >>$config
	grep -q "^learn-address" $config || echo "learn-address $OPENVPN_WEB_ENTRYPOINT" >>$config
}

add_history() {
	#https://build.openvpn.net/man/openvpn-2.6/openvpn.8.html#environmental-variables
	set +e
	TOKEN=$(jq -r '.system.base.token // ""' $ovpn_data/config.json)
	data="vip=$ifconfig_pool_remote_ip&vip6=$ifconfig_pool_remote_ip6&rip=$trusted_ip&rip6=$trusted_ip6&common_name=$common_name&connection_id=$connection_id&username=$username&bytes_received=$bytes_received&bytes_sent=$bytes_sent&time_unix=$time_unix&time_duration=$time_duration"
	status=$(curl -w "%{http_code}" --connect-timeout 5 -s -X POST -o /dev/null -d $data $ovpn_history_api -H "O-Token: $TOKEN")
	if [[ $? -ne 0 || $status -ne 200 ]]; then
		echo "[CLIENT-DISCONNECT] $0:$LINENO 保存历史记录出错，请检查！"
	fi
	set -e
}

send_notify() {
	set +e
	TOKEN=$(jq -r '.system.base.token // ""' $ovpn_data/config.json)
	event="$1"
	api="${ovpn_notify_api:-http://127.0.0.1:$(jq -r '.system.base.web_port // "8888"' $ovpn_data/config.json)/ovpn/notify}"
	data="event=$event&vip=$ifconfig_pool_remote_ip&vip6=$ifconfig_pool_remote_ip6&rip=$trusted_ip&rip6=$trusted_ip6&common_name=$common_name&connection_id=$connection_id&username=$username&bytes_received=$bytes_received&bytes_sent=$bytes_sent&time_unix=$time_unix&time_duration=$time_duration"
	status=$(curl -w "%{http_code}" --connect-timeout 5 -s -X POST -o /dev/null -d $data $api -H "O-Token: $TOKEN")
	if [[ $? -ne 0 || $status -ne 200 ]]; then
		echo "[CLIENT-$event] $0:$LINENO send notify failed"
	fi
	set -e
}


set_ovip() {
	cc_file="$1"
	ip_file="$ovpn_data/.ovip"

	if [ -f "$ip_file" ]; then
		ipaddr=$(cat $ip_file)
		if [ -n "$ipaddr" ]; then
			echo "ifconfig-push $ipaddr $ifconfig_netmask" >$cc_file
			rm -rf $ip_file
		fi
	fi
}

set_ovconfig() {
	cc_file="$1"
	ovc_file="$ovpn_data/.ovc"

	if [ -f "$ovc_file" ]; then
		ovconfig=$(cat $ovc_file)
		if [ -n "$ovconfig" ]; then
			echo "$ovconfig" >>$cc_file
			rm -rf $ovc_file
		fi
	fi
}

load_nftconfig() {
	NFT_CONFIG="$OVPN_DATA/openvpn.nft"
	TABLE=$(jq -r '.system.base.nft_table_name // "openvpn-nft"' $SYSTEM_CONFIG)

	[ ! -f $NFT_CONFIG ] && cat <<EOF >$NFT_CONFIG
table inet $TABLE {
	set blacklist_v4 {
		type ipv4_addr
	}

	set blacklist_v6 {
		type ipv6_addr
	}

	chain forward {
		type filter hook forward priority filter; policy accept;
		ip saddr @blacklist_v4 drop
		ip6 saddr @blacklist_v6 drop
	}

	chain upload {
		type filter hook postrouting priority filter; policy accept;
	}

	chain download {
		type filter hook prerouting priority filter; policy accept;
	}
}
EOF

	nft -f $NFT_CONFIG
}

set_firewall() {
	set +e
	WEB_PORT=$(jq -r '.system.base.web_port // "8888"' $ovpn_data/config.json)
	TOKEN=$(jq -r '.system.base.token // ""' $ovpn_data/config.json)
	ovpn_firewall_api="http://127.0.0.1:$WEB_PORT/ovpn/firewall?a=add_ovips"
	data="vip=$ifconfig_pool_remote_ip&vip6=$ifconfig_pool_remote_ip6&username=$username"
	status=$(curl -w "%{http_code}" --connect-timeout 5 -s -X POST -o /dev/null -d $data $ovpn_firewall_api -H "O-Token: $TOKEN")
	if [[ $? -ne 0 || $status -ne 200 ]]; then
		echo "[CLIENT-CONNECT] $0:$LINENO 设置防火墙出错，请检查！"
	fi
	set -e
}

delete_firewall() {
	set +e
	WEB_PORT=$(jq -r '.system.base.web_port // "8888"' $ovpn_data/config.json)
	TOKEN=$(jq -r '.system.base.token // ""' $ovpn_data/config.json)
	ovpn_firewall_api="http://127.0.0.1:$WEB_PORT/ovpn/firewall?a=delete_ovips"
	data="vip=$ifconfig_pool_remote_ip&vip6=$ifconfig_pool_remote_ip6&username=$username"
	status=$(curl -w "%{http_code}" --connect-timeout 5 -s -X POST -o /dev/null -d $data $ovpn_firewall_api -H "O-Token: $TOKEN")
	if [[ $? -ne 0 || $status -ne 200 ]]; then
		echo "[CLIENT-DISCONNECT] $0:$LINENO 移除防火墙策略出错，请检查！"
	fi
	set -e
}

sync_web_audit_client_map() {
	# DNS ownership is updated from OpenVPN lifecycle events so a recycled VPN IP
	# is never attributed to its former user while the management cache refreshes.
	set +e
	local action="$1"
	WEB_PORT=$(jq -r '.system.base.web_port // "8888"' "$ovpn_data/config.json")
	TOKEN=$(jq -r '.system.base.token // ""' "$ovpn_data/config.json")
	api="${ovpn_web_audit_client_map_api:-http://127.0.0.1:$WEB_PORT/ovpn/web-audit/client-map}"
	status=$(curl -w "%{http_code}" --connect-timeout 2 --max-time 3 -s -X POST -o /dev/null \
		--data-urlencode "action=$action" --data-urlencode "vip=$ifconfig_pool_remote_ip" \
		--data-urlencode "vip6=$ifconfig_pool_remote_ip6" --data-urlencode "common_name=$common_name" \
		--data-urlencode "connection_id=$connection_id" --data-urlencode "username=$username" \
		--data-urlencode "event_time_ns=$(date +%s%N)" \
		"$api" -H "O-Token: $TOKEN")
	if [[ $? -ne 0 || ( $status -ne 200 && $status -ne 204 ) ]]; then
		echo "[DNS-AUDIT] $0:$LINENO 更新客户端 DNS 审计归属失败" >&2
	fi
	set -e
}

client_disconnect() {
	sync_web_audit_client_map delete
	delete_firewall
	add_history
}

client_connect() {
	set_ovip "$1"
	set_ovconfig "$1"
	sync_web_audit_client_map upsert
	send_notify connect
}

################################################################################################

if [ "${1#-}" != "$1" ]; then
	$OPENVPN_BIN "$@"
fi

case $1 in
"genclient")
	if [ -z $2 ]; then
		echo "请输入生成客户端名称！"
		exit 1
	fi

	if [ -n "$6" ]; then
		echo -e "$6" >$OVPN_DATA/ccd/$2
	fi

	genclient "$2" "$3" "$4" "$5" "$7"
	exit 0
	;;
"auth")
	auth $2
	exit 0
	;;
"renewcert")
	renew_cert $2
	exit 0
	;;
"$OPENVPN_BIN")
	secure_data_permissions
	wait_for_runtime_config
	if [ ! -f "$OVPN_DATA/server.conf" ]; then
		install -d -m 0700 "$OVPN_DATA/ccd"

		init_pki
		secure_data_permissions
		init_config
	fi

	load_nftconfig
	check_config
	run_server
	;;
"/usr/bin/supervisord")
	/usr/bin/supervisord -c /etc/supervisor/conf.d/supervisord.conf
	;;
"$OPENVPN_WEB_ENTRYPOINT")
	# OpenVPN invokes this script for client hooks.
	:
esac

case "$script_type" in
client-connect)
	client_connect "$@"
	exit 0
	;;
client-disconnect)
	client_disconnect "$@"
	exit 0
	;;
learn-address)
	case "$1" in
	add|update)
		set_firewall "$@"
		sync_web_audit_client_map upsert
		;;
	delete)
		sync_web_audit_client_map delete
		;;
	esac
	exit 0
	;;
esac

exec "$@"
