#!/usr/bin/env bash
# P09-T01 local release rehearsal on the disposable dev topology.
# Exercises the MVP cross-module reads on the immutable candidate with
# synthetic stations: directory search/detail, official history and the
# source-separated community projection (B-BR-001), plus denied paths.
# Signed-write BUC-002…007 flows run in the real-PostGIS integration
# suites; this script proves the modules serve together without source
# mixing, on candidate-equivalent product code, loopback only.
#
# Environment:
#   REHEARSE_COMPOSE_FILE (default infra/compose.dev.yml)
#   REHEARSE_DB_USER/_DB_NAME/_DB_PASSWORD (required)
#   REHEARSE_API_BIN (required, built api binary)
#   REHEARSE_API_ADDR (default 127.0.0.1:18093)
#   REHEARSE_CANDIDATE (required, immutable commit, e.g. 4ce5aaf…)
#   REHEARSE_ALLOW_DEV=1 (required to target compose.dev.yml)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

COMPOSE="${REHEARSE_COMPOSE_FILE:-infra/compose.dev.yml}"
DB_USER="${REHEARSE_DB_USER:?REHEARSE_DB_USER is required}"
DB_NAME="${REHEARSE_DB_NAME:?REHEARSE_DB_NAME is required}"
DB_PASSWORD="${REHEARSE_DB_PASSWORD:?REHEARSE_DB_PASSWORD is required}"
BIN="${REHEARSE_API_BIN:?REHEARSE_API_BIN is required (go build ./cmd/api)}"
ADDR="${REHEARSE_API_ADDR:-127.0.0.1:18093}"
CANDIDATE="${REHEARSE_CANDIDATE:?REHEARSE_CANDIDATE is required}"

log() { echo "rehearse: $*" >&2; }
refuse() { echo "REFUSED: $*" >&2; exit 2; }

[[ -x "$BIN" ]] || refuse "api binary missing: $BIN"
if [[ "$COMPOSE" == *"compose.dev.yml" && "${REHEARSE_ALLOW_DEV:-}" != "1" ]]; then
    refuse "dev topology needs REHEARSE_ALLOW_DEV=1 (never in production)"
fi
# Tracked modifications block the rehearsal (candidate-equivalence); new
# untracked harness files are allowed pre-commit and are covered by the
# core product-tree check below.
[[ -n "$(git status --porcelain --untracked-files=no)" ]] && refuse "dirty tracked tree; commit owned changes first"
MAIN_SHA="$(git rev-parse origin/main)"
[[ "$MAIN_SHA" == "$CANDIDATE" ]] || refuse "origin/main $MAIN_SHA != candidate $CANDIDATE"
# Core product behavior must match the candidate; the rehearsal harness
# itself (testdata/e2e, rehearse scripts, docs, Makefile entry) may differ.
if [[ -n "$(git diff "$CANDIDATE"...HEAD -- backend/cmd backend/internal backend/db backend/go.mod backend/go.sum contracts/openapi)" ]]; then
    refuse "core product tree differs from candidate $CANDIDATE (harness/docs delta allowed)"
fi

db() {
    docker compose -f "$COMPOSE" exec -T -e "PGPASSWORD=$DB_PASSWORD" db \
        psql -h 127.0.0.1 -U "$DB_USER" -d "$DB_NAME" -tAX -c "$1" 2>/dev/null
}

POSTGIS="$(db "SELECT PostGIS_Version();" || true)"
[[ -n "$POSTGIS" ]] || refuse "PostGIS unreachable"
log "postgis: $POSTGIS"

API_PID=""
LOGFILE="$(mktemp)"
SEED_IDS=()
cleanup() {
    if [[ -n "${API_PID:-}" ]]; then
        kill "$API_PID" 2>/dev/null || true
    fi
    db "DELETE FROM directory_stations WHERE id::text LIKE 'c0000000-0000-4000-8000-%';" >/dev/null 2>&1 || true
    rm -f "$LOGFILE"
}
trap cleanup EXIT

log "seeding synthetic rehearsal stations"
db "INSERT INTO directory_stations (id, display_name, municipality_code, state) VALUES ('c0000000-0000-4000-8000-000000000001','Rehearsal Station 1','3550308','SP'),('c0000000-0000-4000-8000-000000000002','Rehearsal Station 2','3304557','RJ') ON CONFLICT (id) DO NOTHING;" >/dev/null
HAVE="$(db "SELECT COUNT(*) FROM directory_stations WHERE id::text LIKE 'c0000000-0000-4000-8000-%';")"
[[ "$HAVE" == "2" ]] || refuse "seeded $HAVE, want 2"
STATION="c0000000-0000-4000-8000-000000000001"

export ANPFUEL_ENV=development
export ANPFUEL_HTTP_ADDR="$ADDR"
export ANPFUEL_METRICS_ADDR=""
export ANPFUEL_DATABASE_URL="postgres://$DB_USER:$DB_PASSWORD@127.0.0.1:5434/$DB_NAME?sslmode=disable"
"$BIN" >"$LOGFILE" 2>&1 &
API_PID=$!
for _ in $(seq 1 30); do
    if curl -sS --max-time 2 "http://$ADDR/health/live" >/dev/null 2>&1; then
        break
    fi
    sleep 1
done
curl -sS --max-time 5 "http://$ADDR/health/live" >/dev/null || refuse "api did not boot"
curl -sS --max-time 5 "http://$ADDR/health/ready" >/dev/null || refuse "api not ready (DB down?)"
log "api boot ok"

check() {
    local desc="$1" want="$2"
    shift 2
    local code
    code="$(curl -sS -o /tmp/rehearse-body.json -w "%{http_code}" --max-time 10 "$@" 2>/dev/null || echo "000")"
    [[ "$code" == "$want" ]] || refuse "$desc: got $code want $want ($(cat /tmp/rehearse-body.json 2>/dev/null))"
    log "$desc: $code ok"
}

check "search" 200 "http://$ADDR/v1/stations?q=Rehearsal&limit=20"
python3 -c "import json; d=json.load(open('/tmp/rehearse-body.json')); ids=[i.get('station_id','') for i in d.get('items',[])]; assert 'c0000000-0000-4000-8000-000000000001' in ids, ids"
check "detail" 200 "http://$ADDR/v1/stations/$STATION"
check "prices" 200 "http://$ADDR/v1/stations/$STATION/prices?fuel_product=GASOLINE_REGULAR"
python3 - <<'PY'
import json
d = json.load(open('/tmp/rehearse-body.json'))
for g in d.get('items', []):
    off = g.get('official')
    com = g.get('community')
    if off is not None:
        assert off.get('source') == 'ANP', off
    if com is not None:
        assert com.get('source') == 'COMMUNITY', com
        assert 'amount_milli_brl' not in com or isinstance(com.get('amount_milli_brl'), (int, type(None))), com
print('source separation ok (B-BR-001)')
PY
check "official history" 200 "http://$ADDR/v1/stations/$STATION/official-prices?limit=5"
check "bad uuid refused" 400 "http://$ADDR/v1/stations/not-a-uuid"
check "unknown station 404" 404 "http://$ADDR/v1/stations/00000000-0000-4000-8000-000000000000"
check "unauthenticated write 401" 401 -X POST "http://$ADDR/v1/observations" -H 'Content-Type: application/json' -d '{"client_submission_id":"x"}'
check "nearby without coords 400" 400 "http://$ADDR/v1/stations/nearby?limit=10"

if grep -Ei "postgres://[^ ]*:[^ ]*@|BEGIN PRIVATE|PRIVATE KEY" "$LOGFILE" >/dev/null 2>&1; then
    refuse "secret material in api log"
fi
log "no secret material in api log (B-BR-011)"

echo "rehearse ok: candidate $CANDIDATE, 6 reads + 4 denied paths, source separation intact"
