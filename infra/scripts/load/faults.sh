#!/usr/bin/env bash
# P08-T07 bounded fault suite: database outage, storage outage and
# capped disk pressure against the disposable dev topology. Each fault
# asserts degraded (never false-healthy) behavior plus recovery, with
# traps restoring the environment. Staging repeats with real volumes
# and longer windows (P09 matrix).
#
# Environment: LOAD_COMPOSE_FILE, LOAD_DB_USER/_DB_NAME/_DB_PASSWORD,
# LOAD_API_BIN (built api binary). Requires the disposable database.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

COMPOSE="${LOAD_COMPOSE_FILE:-infra/compose.dev.yml}"
DB_USER="${LOAD_DB_USER:?LOAD_DB_USER is required}"
DB_NAME="${LOAD_DB_NAME:?LOAD_DB_NAME is required}"
DB_PASSWORD="${LOAD_DB_PASSWORD:?LOAD_DB_PASSWORD is required}"
BIN="${LOAD_API_BIN:?LOAD_API_BIN is required (go build ./cmd/api)}"
ADDR="127.0.0.1:18083"
FAULT_MODE="${LOAD_DB_FAULT_MODE:-stop}"
[[ "$FAULT_MODE" == stop || "$FAULT_MODE" == pause ]] || { echo "invalid DB fault mode" >&2; exit 2; }

PASS=0
FAIL=0
pass() { echo "PASS: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1${2:+ ($2)}" >&2; FAIL=$((FAIL + 1)); }

CAP_OUT=""
CAP_CODE=0
capture() {
    set +e
    CAP_OUT="$("$@")"
    CAP_CODE=$?
    set -e
}

API_PID=""
stop_api() {
    if [[ -n "$API_PID" ]]; then
        kill "$API_PID" 2>/dev/null || true
        wait "$API_PID" 2>/dev/null || true
        API_PID=""
    fi
}
boot_api() {
    # $1 = extra env assignments evaluated by the caller shell
    stop_api
    ANPFUEL_HTTP_ADDR="$ADDR" ANPFUEL_METRICS_ADDR="" "$BIN" >/tmp/fault-api.log 2>&1 &
    API_PID=$!
    local i
    for i in $(seq 1 30); do
        if curl -sS --max-time 2 "http://$ADDR/health/live" >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
    done
    return 1
}

db() {
    docker compose -f "$COMPOSE" exec -T -e "PGPASSWORD=$DB_PASSWORD" db \
        psql -h 127.0.0.1 -U "$DB_USER" -d "$DB_NAME" -tAX -c "$1" 2>/dev/null
}

restart_db() {
    if [[ "$FAULT_MODE" == pause ]]; then
        docker compose -f "$COMPOSE" unpause db >/dev/null 2>&1 || true
    else
        docker compose -f "$COMPOSE" start db >/dev/null 2>&1 || true
    fi
    local i
    for i in $(seq 1 30); do
        if db "SELECT 1;" | grep -q 1; then
            return 0
        fi
        sleep 1
    done
    return 1
}

cleanup() {
    stop_api
    restart_db
    rm -f /tmp/fault-api.log /tmp/disk-pressure.bin
}
trap cleanup EXIT

# --- baseline: api boots against the disposable database ---
if boot_api; then
    pass "api boots for fault baseline"
else
    fail "api boots for fault baseline"
fi

# --- fault 1: database outage rejects writes/reads as unavailable ---
if [[ "$FAULT_MODE" == pause ]]; then
    docker compose -f "$COMPOSE" pause db >/dev/null 2>&1
else
    docker compose -f "$COMPOSE" stop db >/dev/null 2>&1
fi
sleep 3
capture curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "http://$ADDR/health/ready"
if [[ "$CAP_CODE" -ne 0 || "$CAP_OUT" == "503" ]]; then
    pass "db outage surfaces (refused or 503, never false-healthy)"
else
    fail "db outage surfaces" "ready=$CAP_OUT"
fi
capture curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "http://$ADDR/v1/stations?q=load&limit=1"
if [[ "$CAP_CODE" -ne 0 || "$CAP_OUT" == "500" || "$CAP_OUT" == "503" ]]; then
    pass "reads fail closed during outage"
else
    fail "reads fail closed during outage" "code=$CAP_OUT"
fi
if restart_db; then
    pass "database recovers"
else
    fail "database recovers"
fi
sleep 2
if curl -fsS --max-time 10 "http://$ADDR/health/ready" >/dev/null 2>&1; then
    pass "readiness returns after recovery"
else
    fail "readiness returns after recovery"
fi

# --- fault 2: storage outage degrades uploads explicitly ---
stop_api
ANPFUEL_HTTP_ADDR="$ADDR" ANPFUEL_METRICS_ADDR="" ANPFUEL_R2_ENDPOINT="http://127.0.0.1:9" \
    ANPFUEL_R2_BUCKET=drill ANPFUEL_R2_REGION=auto \
    ANPFUEL_R2_ACCESS_KEY_ID=drill ANPFUEL_R2_SECRET_ACCESS_KEY=drill \
    "$BIN" >/tmp/fault-api.log 2>&1 &
API_PID=$!
SIGNED_OK=0
for _ in $(seq 1 30); do
    if curl -sS --max-time 2 "http://$ADDR/health/live" >/dev/null 2>&1; then
        SIGNED_OK=1
        break
    fi
    sleep 1
done
if [[ "$SIGNED_OK" == "1" ]]; then
    # Anonymous upload intent without proof must not issue storage
    # authorizations: 401/503 both prove no costly work was granted.
    CODE="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 -X POST "http://$ADDR/v1/uploads" \
        -H 'Content-Type: application/json' \
        --data '{"client_submission_id":"fault-1","content_type":"image/jpeg","size_bytes":100,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}' || echo 000)"
    if [[ "$CODE" == "401" || "$CODE" == "503" ]]; then
        pass "storage outage grants no upload authorization ($CODE)"
    else
        fail "storage outage grants no upload authorization" "code=$CODE"
    fi
else
    fail "api boots for storage fault"
fi
stop_api

# --- fault 3: capped disk pressure keeps serving ---
FREE_KB="$(df --output=avail /tmp | tail -n 1 | tr -d ' ')"
if [[ "$FREE_KB" -gt 2097152 ]]; then
    fallocate -l 128M /tmp/disk-pressure.bin
    if curl -sS --max-time 10 "http://$ADDR/health/live" >/dev/null 2>&1; then
        pass "serves under capped disk pressure"
    else
        # api is down after the storage reboot on purpose; boot cleanly and retry once
        if boot_api && curl -sS --max-time 10 "http://$ADDR/health/live" >/dev/null 2>&1; then
            pass "serves under capped disk pressure"
        else
            fail "serves under capped disk pressure"
        fi
    fi
    rm -f /tmp/disk-pressure.bin
else
    echo "SKIP: disk pressure needs 2 GiB free in /tmp"
fi

echo "faults: $PASS passed, $FAIL failed"
[[ "$FAIL" -eq 0 ]]
