#!/bin/sh

set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
COMPOSE_FILE="$REPO_ROOT/deploy/compose/live-test/compose.yaml"
ADGUARD_URL="http://127.0.0.1:13000"
ADGUARD_DNS_HOST="127.0.0.1"
ADGUARD_DNS_PORT="5353"
ADGUARD_USER="admin"
ADGUARD_PASSWORD="adguard-test-password"
MANAGED_DOMAIN="whoami.test"
MANAGED_ORIGINAL_ANSWER="127.0.0.1"
MANAGED_UPDATED_ANSWER="127.0.0.2"
MANUAL_DOMAIN="manual-live.test"
MANUAL_ANSWER="127.0.0.77"
KEEP_RUNNING=${KEEP_RUNNING:-0}

if [ "${1:-}" = "--keep-running" ]; then
	KEEP_RUNNING=1
	shift
fi

if [ "$#" -ne 0 ]; then
	printf '%s\n' "verify: unsupported arguments" >&2
	exit 2
fi

cd "$REPO_ROOT"

TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/docker-dns-sync-live-test.XXXXXX")
RUNTIME_DIR="$TMP_DIR/runtime"
OVERRIDE_FILE="$TMP_DIR/compose.override.yaml"
COMPOSE_LOG="$TMP_DIR/compose.log"

ADGUARD_CONF_DIR="$RUNTIME_DIR/adguard/conf"
ADGUARD_WORK_DIR="$RUNTIME_DIR/adguard/work"
STATE_DIR="$RUNTIME_DIR/state"

mkdir -p "$ADGUARD_CONF_DIR" "$ADGUARD_WORK_DIR" "$STATE_DIR"
cp -a "$REPO_ROOT/deploy/compose/live-test/adguard/conf/." "$ADGUARD_CONF_DIR/"
cp -a "$REPO_ROOT/deploy/compose/live-test/adguard/work/." "$ADGUARD_WORK_DIR/"

export LIVE_TEST_ADGUARD_CONF_DIR="$ADGUARD_CONF_DIR"
export LIVE_TEST_ADGUARD_WORK_DIR="$ADGUARD_WORK_DIR"
export LIVE_TEST_STATE_DIR="$STATE_DIR"

cleanup() {
	status=$?
	trap - EXIT INT TERM HUP
	if [ "$KEEP_RUNNING" -ne 1 ]; then
		rm -f "$OVERRIDE_FILE" "$COMPOSE_LOG"
		docker compose -f "$COMPOSE_FILE" down -v >/dev/null 2>&1 || true
		rm -rf "$TMP_DIR"
	else
		rm -f "$OVERRIDE_FILE" "$COMPOSE_LOG"
		if [ "$status" -eq 0 ]; then
			log "verify: keep-running requested; stack remains up"
		else
			log "verify: keep-running requested; runtime directories preserved for inspection"
		fi
		log "verify: cleanup when finished: docker compose -f $COMPOSE_FILE down -v"
		log "verify: cleanup when finished: rm -rf $TMP_DIR"
	fi
	exit "$status"
}

trap cleanup EXIT INT TERM HUP

log() {
	printf '%s\n' "$1"
}

fail() {
	printf '%s\n' "verify: $1" >&2
	exit 1
}

run_compose() {
	if ! docker compose -f "$COMPOSE_FILE" "$@" >"$COMPOSE_LOG" 2>&1; then
		fail "docker compose $* failed"
	fi
}

run_compose_with_override() {
	if ! docker compose -f "$COMPOSE_FILE" -f "$OVERRIDE_FILE" "$@" >"$COMPOSE_LOG" 2>&1; then
		fail "docker compose override $* failed"
	fi
}

adguard_request() {
	curl -fsS -u "$ADGUARD_USER:$ADGUARD_PASSWORD" \
		-H 'Content-Type: application/json' \
		"$@" 2>/dev/null
}

list_rewrites() {
	adguard_request "$ADGUARD_URL/control/rewrite/list"
}

rewrite_present() {
	domain=$1
	answer=$2
	json=$(list_rewrites | tr -d '[:space:]') || return 1
	case "$json" in
		*"\"domain\":\"$domain\",\"answer\":\"$answer\""*|*"\"answer\":\"$answer\",\"domain\":\"$domain\""*) return 0 ;;
		*) return 1 ;;
	esac
}

wait_until() {
	timeout_seconds=$1
	interval_seconds=$2
	description=$3
	shift 3
	deadline=$(($(date +%s) + timeout_seconds))
	while :; do
		if "$@"; then
			return 0
		fi
		if [ "$(date +%s)" -ge "$deadline" ]; then
			fail "$description"
		fi
		sleep "$interval_seconds"
	done
}

extract_nslookup_answer() {
	awk '
		BEGIN { capture = 0 }
		$1 == "Name:" { capture = 1; next }
		capture && /^Address/ {
			for (i = 1; i <= NF; i++) {
				if ($i ~ /^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$/) {
					print $i
					exit
				}
			}
		}
	'
}

query_dns() {
	hostname=$1
	if command -v dig >/dev/null 2>&1; then
		dig +short @"$ADGUARD_DNS_HOST" -p "$ADGUARD_DNS_PORT" "$hostname" 2>/dev/null | awk 'NF { print; exit }'
		return 0
	fi
	docker run --rm --network docker-dns-sync-live-test_default busybox:1.36 nslookup "$hostname" adguardhome 2>/dev/null | extract_nslookup_answer
}

dns_matches() {
	hostname=$1
	answer=$2
	resolved=$(query_dns "$hostname" | tr -d '[:space:]') || return 1
	[ "$resolved" = "$answer" ]
}

wait_for_dns() {
	hostname=$1
	answer=$2
	wait_until 90 2 "DNS did not resolve $hostname to the expected answer" dns_matches "$hostname" "$answer"
}

wait_for_rewrite() {
	domain=$1
	answer=$2
	wait_until 90 2 "AdGuard did not report the expected rewrite for $domain" rewrite_present "$domain" "$answer"
}

adguard_ready() {
	adguard_request "$ADGUARD_URL/control/rewrite/list" >/dev/null
}

create_manual_rewrite() {
	payload=$(printf '{"domain":"%s","answer":"%s"}' "$MANUAL_DOMAIN" "$MANUAL_ANSWER")
	adguard_request -X POST --data "$payload" "$ADGUARD_URL/control/rewrite/add" >/dev/null
}

cat >"$OVERRIDE_FILE" <<EOF
services:
  whoami:
    labels:
      proxy.aliases: $MANAGED_DOMAIN
      proxy.whoami.test.port: "80"
      proxy.whoami.test.host: $MANAGED_UPDATED_ANSWER
EOF

log "verify: starting live-test stack"
run_compose up -d --build

log "verify: waiting for AdGuard API readiness"
wait_until 120 2 "AdGuard API did not become ready" adguard_ready

log "verify: waiting for initial managed rewrite"
wait_for_rewrite "$MANAGED_DOMAIN" "$MANAGED_ORIGINAL_ANSWER"
wait_for_dns "$MANAGED_DOMAIN" "$MANAGED_ORIGINAL_ANSWER"

log "verify: applying managed rewrite update"
run_compose_with_override up -d --force-recreate --no-deps whoami
wait_for_rewrite "$MANAGED_DOMAIN" "$MANAGED_UPDATED_ANSWER"

log "verify: restoring original managed workload"
run_compose up -d --force-recreate --no-deps whoami
wait_for_rewrite "$MANAGED_DOMAIN" "$MANAGED_ORIGINAL_ANSWER"
wait_for_dns "$MANAGED_DOMAIN" "$MANAGED_ORIGINAL_ANSWER"

log "verify: creating manual rewrite"
create_manual_rewrite || fail "failed to create manual rewrite"
wait_for_rewrite "$MANUAL_DOMAIN" "$MANUAL_ANSWER"

log "verify: restarting docker-dns-sync"
run_compose restart docker-dns-sync

log "verify: verifying restart recovery preserves rewrites"
wait_for_rewrite "$MANAGED_DOMAIN" "$MANAGED_ORIGINAL_ANSWER"
wait_for_dns "$MANAGED_DOMAIN" "$MANAGED_ORIGINAL_ANSWER"
wait_for_rewrite "$MANUAL_DOMAIN" "$MANUAL_ANSWER"

log "verify: live-test smoke passed"
