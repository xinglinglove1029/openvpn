#!/bin/bash
set -u

SYSTEM_CONFIG="${OVPN_DATA:-/data}/config.json"
SURICATA_CONFIG="/etc/suricata/suricata.yaml"
SURICATA_LOG_DIR="/data/suricata"
SURICATA_PID=""
SURICATA_TUN_INDEX=""
SURICATA_EXITED=false
SURICATA_FAILURES=0
SURICATA_NEXT_START_AT=0

suricata_running() {
	local state=""
	[ -n "$SURICATA_PID" ] || return 1
	if ! kill -0 "$SURICATA_PID" 2>/dev/null; then
		wait "$SURICATA_PID" 2>/dev/null || true
		SURICATA_PID=""
		SURICATA_TUN_INDEX=""
		SURICATA_EXITED=true
		return 1
	fi
	# kill -0 also succeeds for an exited but unreaped child. Read its Linux
	# process state so a Suricata crash is not mistaken for a live sensor.
	if [ -r "/proc/$SURICATA_PID/stat" ]; then
		read -r _ _ state _ < "/proc/$SURICATA_PID/stat" || true
		if [ "$state" = "Z" ]; then
			wait "$SURICATA_PID" 2>/dev/null || true
			SURICATA_PID=""
			SURICATA_TUN_INDEX=""
			SURICATA_EXITED=true
			return 1
		fi
	fi
	return 0
}

stop_suricata() {
	local attempt=0
	if suricata_running; then
		echo "[suricata-audit] stopping Suricata"
		kill -TERM "$SURICATA_PID" 2>/dev/null || true
		while suricata_running && [ "$attempt" -lt 10 ]; do
			sleep 1
			attempt=$((attempt + 1))
		done
		if suricata_running; then
			kill -KILL "$SURICATA_PID" 2>/dev/null || true
			wait "$SURICATA_PID" 2>/dev/null || true
		fi
	fi
	SURICATA_PID=""
	SURICATA_TUN_INDEX=""
	SURICATA_FAILURES=0
	SURICATA_NEXT_START_AT=0
}

shutdown() {
	stop_suricata
	exit 0
}

trap shutdown INT TERM

uses_builtin_eve_path() {
	jq -e '(.system.base.suricata_eve_path // "/data/suricata/eve.json") == "/data/suricata/eve.json"' "$SYSTEM_CONFIG" >/dev/null 2>&1
}

capture_enabled() {
	jq -e '(.system.base.suricata_eve_enabled // false) == true and (.system.base.suricata_eve_path // "/data/suricata/eve.json") == "/data/suricata/eve.json"' "$SYSTEM_CONFIG" >/dev/null 2>&1
}

tun_index() {
	ip -o link show tun0 2>/dev/null | cut -d: -f1 | tr -d ' '
}

schedule_restart() {
	local now delay
	now=$(date +%s)
	SURICATA_FAILURES=$((SURICATA_FAILURES + 1))
	if [ "$SURICATA_FAILURES" -gt 5 ]; then
		SURICATA_FAILURES=5
	fi
	delay=$((2 ** SURICATA_FAILURES))
	SURICATA_NEXT_START_AT=$((now + delay))
	echo "[suricata-audit] Suricata exited; retrying in ${delay}s"
}

ensure_eve_file() {
	install -d -m 0700 "$SURICATA_LOG_DIR"
	if [ -L "$SURICATA_LOG_DIR/eve.json" ]; then
		echo "[suricata-audit] refusing symbolic-link EVE path" >&2
		return 1
	fi
	if [ -e "$SURICATA_LOG_DIR/eve.json" ] && [ ! -f "$SURICATA_LOG_DIR/eve.json" ]; then
		echo "[suricata-audit] EVE path is not a regular file" >&2
		return 1
	fi
	if [ ! -e "$SURICATA_LOG_DIR/eve.json" ]; then
		(umask 077; : >"$SURICATA_LOG_DIR/eve.json")
	fi
	chmod 0600 "$SURICATA_LOG_DIR/eve.json"
}

# Create the default file before openvpn-web starts. Its importer validates the
# configured file synchronously, so delaying this until config.json exists can
# leave an enabled importer in an error state until the next settings save.
ensure_eve_file || exit 1

while :; do
	if [ ! -s "$SYSTEM_CONFIG" ] || ! jq -e '.system.base | type == "object"' "$SYSTEM_CONFIG" >/dev/null 2>&1; then
		# Capture is opt-in. A missing or malformed configuration is never a
		# valid authorization to keep collecting network metadata.
		stop_suricata
		sleep 2
		continue
	fi

	if uses_builtin_eve_path && ! ensure_eve_file; then
		stop_suricata
		sleep 2
		continue
	fi

	if capture_enabled; then
		current_tun_index=$(tun_index)
		if [ -z "$current_tun_index" ]; then
			stop_suricata
			sleep 2
			continue
		fi
		if suricata_running && [ "$SURICATA_TUN_INDEX" != "$current_tun_index" ]; then
			echo "[suricata-audit] tun0 was recreated; restarting Suricata"
			stop_suricata
		fi
		if ! suricata_running; then
			if [ "$SURICATA_EXITED" = true ]; then
				schedule_restart
				SURICATA_EXITED=false
			fi
			now=$(date +%s)
			if [ "$now" -ge "$SURICATA_NEXT_START_AT" ]; then
				echo "[suricata-audit] starting Suricata on tun0"
				suricata -c "$SURICATA_CONFIG" -i tun0 &
				SURICATA_PID=$!
				SURICATA_TUN_INDEX="$current_tun_index"
			fi
		fi
	else
		stop_suricata
	fi
	sleep 2
done
