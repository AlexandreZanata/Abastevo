#!/usr/bin/env bash
# P08-T07 bounded load run: deterministic seed, origin read scenarios
# and budget assertions. Small-scale smoke by design: the 30-minute
# acceptance matrix (100k stations, 1M rows) runs on provisioned
# staging infrastructure (P09), never on a shared workstation.
#
# Environment:
#   LOAD_COMPOSE_FILE (default infra/compose.dev.yml)
#   LOAD_DB_USER/_DB_NAME/_DB_PASSWORD (required)
#   LOAD_SEED_STATIONS (default 2000)
#   LOAD_API_ADDR (default 127.0.0.1:18082), LOAD_API_BIN (built api binary)
#   LOAD_DURATION_SECONDS (default 30), LOAD_CONCURRENCY (default 8)
#   LOAD_P95_BUDGET_MS (default 300), LOAD_MAX_5XX_RATE (default 0.01)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

COMPOSE="${LOAD_COMPOSE_FILE:-infra/compose.dev.yml}"
DB_USER="${LOAD_DB_USER:?LOAD_DB_USER is required}"
DB_NAME="${LOAD_DB_NAME:?LOAD_DB_NAME is required}"
DB_PASSWORD="${LOAD_DB_PASSWORD:?LOAD_DB_PASSWORD is required}"
STATIONS="${LOAD_SEED_STATIONS:-2000}"
ADDR="${LOAD_API_ADDR:-127.0.0.1:18082}"
BIN="${LOAD_API_BIN:?LOAD_API_BIN is required (go build ./cmd/api)}"
DURATION="${LOAD_DURATION_SECONDS:-30}"
CONCURRENCY="${LOAD_CONCURRENCY:-8}"
P95_BUDGET_MS="${LOAD_P95_BUDGET_MS:-300}"
MAX_5XX="${LOAD_MAX_5XX_RATE:-0.01}"

log() { echo "load: $*" >&2; }
refuse() { echo "REFUSED: $*" >&2; exit 2; }

API_PID=""
stop_api() {
    if [[ -n "$API_PID" ]]; then
        kill "$API_PID" 2>/dev/null || true
        API_PID=""
    fi
}
cleanup() { stop_api; }
# Pre-declared for the EXIT trap under set -u.
LAT=""; CODES=""; SEEDFILE=""

[[ -x "$BIN" ]] || refuse "api binary missing: $BIN"

db() {
    docker compose -f "$COMPOSE" exec -T -e "PGPASSWORD=$DB_PASSWORD" db \
        psql -h 127.0.0.1 -U "$DB_USER" -d "$DB_NAME" -tAX -c "$1" 2>/dev/null
}

# --- seed (idempotent) ---
log "seeding $STATIONS stations"
db "DELETE FROM directory_stations WHERE id::text LIKE 'b0000000-0000-4000-8000-%';" >/dev/null
SEEDFILE="$(mktemp)"
trap 'cleanup; rm -f "$LAT" "$CODES" "$SEEDFILE"' EXIT
sed -e "s/:STATIONS/$STATIONS/" -e '/^--/d' backend/testdata/load/seed.sql >"$SEEDFILE"
if ! docker compose -f "$COMPOSE" exec -T -e "PGPASSWORD=$DB_PASSWORD" db \
    psql -h 127.0.0.1 -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 -f - <"$SEEDFILE" >/dev/null 2>&1; then
    refuse "seed failed"
fi
HAVE="$(db "SELECT COUNT(*) FROM directory_stations WHERE id::text LIKE 'b0000000-0000-4000-8000-%';")"
[[ "$HAVE" == "$STATIONS" ]] || refuse "seeded $HAVE, want $STATIONS"
db "SELECT id FROM directory_stations WHERE id::text LIKE 'b0000000-0000-4000-8000-%' ORDER BY id LIMIT 200;" > /tmp/load-ids.txt
log "seed ok: $HAVE rows"

# --- boot api ---
export ANPFUEL_HTTP_ADDR="$ADDR"
export ANPFUEL_METRICS_ADDR=""
"$BIN" >/tmp/load-api.log 2>&1 &
API_PID=$!
for _ in $(seq 1 30); do
    if curl -sS --max-time 2 "http://$ADDR/health/live" >/dev/null 2>&1; then
        break
    fi
    sleep 1
done
curl -sS --max-time 5 "http://$ADDR/health/live" >/dev/null || refuse "api did not boot"

# --- scenarios: origin reads (search/detail/nearby rotation) ---
log "running ${DURATION}s at concurrency ${CONCURRENCY}"
END=$((SECONDS + DURATION))
LAT="$(mktemp)"; CODES="$(mktemp)"
trap 'cleanup; rm -f "$LAT" "$CODES" "$SEEDFILE"' EXIT
seq 1 $((CONCURRENCY * 4)) | xargs -P "$CONCURRENCY" -I{} bash -c '
    end='"$END"'
    while ((SECONDS < end)); do
        id=$(shuf -n 1 /tmp/load-ids.txt | tr -d "[:space:]")
        case $((RANDOM % 3)) in
            0) url="http://'"$ADDR"'/v1/stations?q=load&limit=20" ;;
            1) url="http://'"$ADDR"'/v1/stations/$id" ;;
            *) url="http://'"$ADDR"'/v1/stations/nearby?lat=-15.8&lon=-47.9&radius_m=3000&limit=10" ;;
        esac
        out=$(curl -sS -o /dev/null -w "%{time_total} %{http_code}" --max-time 10 "$url" 2>/dev/null || echo "10.000 000")
        echo "${out%% *}" >>"'"$LAT"'"
        echo "${out##* }" >>"'"$CODES"'"
    done
'
COUNT="$(wc -l <"$LAT" | tr -d ' ')"
[[ "$COUNT" -gt 0 ]] || refuse "no samples collected"
STATS="$(python3 infra/scripts/load/stats.py "$LAT" "$CODES")"
echo "$STATS"
P95_MS="$(printf '%s' "$STATS" | sed -n 's/.*p95=\([0-9.]*\).*/\1/p' | awk '{print $1*1000}')"
RATE="$(printf '%s' "$STATS" | sed -n 's/.*rate=\([0-9.]*\).*/\1/p')"
log "requests=$COUNT p95=${P95_MS}ms 5xx_rate=$RATE (budgets: ${P95_BUDGET_MS}ms, $MAX_5XX)"
awk "BEGIN {exit !( $P95_MS <= $P95_BUDGET_MS )}" || refuse "p95 budget breached: ${P95_MS}ms > ${P95_BUDGET_MS}ms"
awk "BEGIN {exit !( $RATE <= $MAX_5XX )}" || refuse "5xx budget breached: $RATE > $MAX_5XX"

# --- scrub seed (keep the disposable database clean for others) ---
db "DELETE FROM directory_stations WHERE id::text LIKE 'b0000000-0000-4000-8000-%';" >/dev/null
rm -f /tmp/load-ids.txt /tmp/load-api.log
echo "load ok: $COUNT requests within budgets"
