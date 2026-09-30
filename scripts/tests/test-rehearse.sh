#!/usr/bin/env bash
# P09-T01 harness: local rehearsal happy path plus mutant refusals on the
# disposable dev topology. Bounded by design (single API boot, 2 synthetic
# stations, public reads only); signed-write BUC flows run in the
# real-PostGIS integration suites. Usage: make test-rehearse.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

CANDIDATE="$(git rev-parse origin/main)"
export REHEARSE_COMPOSE_FILE=infra/compose.dev.yml
export REHEARSE_DB_USER=anpfuel
export REHEARSE_DB_NAME=anpfuel
export REHEARSE_DB_PASSWORD=anpfuel
export REHEARSE_CANDIDATE="$CANDIDATE"
export REHEARSE_ALLOW_DEV=1
export REHEARSE_API_ADDR=127.0.0.1:18093

BIN="$ROOT/backend/.tmp-rehearse-api"
(
    cd backend
    GOTOOLCHAIN=go1.27.1 go build -o "$BIN" ./cmd/api
)
export REHEARSE_API_BIN="$BIN"
trap 'rm -f "$BIN" /tmp/rehearse-body.json' EXIT

echo "== rehearsal happy path =="
bash infra/scripts/rehearse.sh
echo "PASS: rehearsal happy path"

echo "== mutant: missing binary refused =="
if REHEARSE_API_BIN=/tmp/missing-api-bin bash infra/scripts/rehearse.sh >/dev/null 2>&1; then
    echo "FAIL: missing binary accepted" >&2
    exit 1
fi
echo "PASS: missing binary refused"

echo "== mutant: wrong candidate refused =="
if REHEARSE_CANDIDATE=0000000000000000000000000000000000000000 bash infra/scripts/rehearse.sh >/dev/null 2>&1; then
    echo "FAIL: wrong candidate accepted" >&2
    exit 1
fi
echo "PASS: wrong candidate refused"

echo "== mutant: dev guard refused =="
if REHEARSE_ALLOW_DEV=0 bash infra/scripts/rehearse.sh >/dev/null 2>&1; then
    echo "FAIL: dev guard bypassed" >&2
    exit 1
fi
echo "PASS: dev guard refused"

echo "rehearsal campaign ok (bounded local profile; deployed matrix stays pending)"
