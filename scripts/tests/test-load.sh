#!/usr/bin/env bash
# P08-T07 harness: bounded load smoke plus fault suite on the
# disposable dev topology. Small deterministic profile by design
# (minutes, not the 30-minute staging campaign): seed assertion,
# origin reads within budgets, outage/degraded behavior, cleanup.
# Usage: make test-load (or this script directly with LOAD_* overrides).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

export LOAD_COMPOSE_FILE="${ANPFUEL_TEST_COMPOSE_FILE:-infra/compose.dev.yml}"
export LOAD_DB_USER=anpfuel
export LOAD_DB_NAME=anpfuel
export LOAD_DB_PASSWORD=anpfuel
export LOAD_SEED_STATIONS=500
export LOAD_DURATION_SECONDS=20
export LOAD_CONCURRENCY=8

BIN="$ROOT/backend/.tmp-load-api"
(
    cd backend
    GOTOOLCHAIN=go1.27.2 go build -o "$BIN" ./cmd/api
)
export LOAD_API_BIN="$BIN"
trap 'rm -f "$BIN"' EXIT

python3 infra/scripts/load/stats.py --self-test
bash infra/scripts/load/run.sh
echo "PASS: load smoke within budgets"
bash infra/scripts/load/faults.sh
echo "load campaign ok (bounded smoke profile; full matrix runs on staging)"
