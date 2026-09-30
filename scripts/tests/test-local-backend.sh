#!/usr/bin/env bash
# Full local backend matrix. Creates and removes only its unique tmpfs Compose
# project. Requires Docker, Go and the pinned static tools from backend/README.
# No production DSN, public TLS authority, source discovery or real photos.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
for tool in docker go sqlc staticcheck govulncheck vacuum python3; do
    command -v "$tool" >/dev/null || { echo "missing tool: $tool" >&2; exit 1; }
done
WORK="$(mktemp -d /tmp/anpfuel-local-suite.XXXXXXXX)"
REPORT="${ANPFUEL_LOCAL_REPORT_DIR:-$WORK/report}"
mkdir -p "$REPORT"
export ANPFUEL_VALIDATION_PROJECT="anpfuel-validation-$(basename "$WORK" | tr '[:upper:].' '[:lower:]-')"
COMPOSE="$ROOT/infra/compose.validation.yml"
cleanup() {
    docker compose -f "$COMPOSE" down --volumes --remove-orphans >/dev/null 2>&1 || true
    rm -f "$WORK/api"
}
trap cleanup EXIT INT TERM
export GOCACHE="${GOCACHE:-$WORK/go-cache}"
echo "Local reports: $REPORT"
docker compose -f "$COMPOSE" up -d --wait db storage >"$REPORT/startup.log" 2>&1
docker compose -f "$COMPOSE" run --rm storage-init >"$REPORT/storage-init.log" 2>&1
DB_ADDR="$(docker compose -f "$COMPOSE" port db 5432)"
STORAGE_ADDR="$(docker compose -f "$COMPOSE" port storage 9000)"
# Clear inherited runtime credentials/settings; this matrix must not select an
# existing deployment via a developer's shell environment.
while IFS= read -r name; do unset "$name"; done < <(compgen -v | rg '^ANPFUEL_(R2_|DATABASE_URL$|ENV$|HTTP_ADDR$|METRICS_ADDR$|CANONICAL_HOST$|CURSOR_SECRET$)')
export ANPFUEL_TEST_DATABASE_URL="postgres://anpfuel:anpfuel@$DB_ADDR/anpfuel?sslmode=disable"
export ANPFUEL_DATABASE_URL="$ANPFUEL_TEST_DATABASE_URL"
export ANPFUEL_ENV=development ANPFUEL_METRICS_ADDR="" ANPFUEL_ANP_DISCOVERY_ENABLED=false
export ANPFUEL_CANONICAL_HOST=local.example.invalid
export ANPFUEL_CURSOR_SECRET=local-validation-cursor-key-only-20260930
export ANPFUEL_TEST_STORAGE_ENDPOINT="http://$STORAGE_ADDR"
export ANPFUEL_TEST_STORAGE_BUCKET=validation-private ANPFUEL_TEST_STORAGE_REGION=us-east-1
export ANPFUEL_TEST_STORAGE_ACCESS_KEY_ID=localvalidation
export ANPFUEL_TEST_STORAGE_SECRET_ACCESS_KEY=local-validation-only-20260930
export ANPFUEL_TEST_COMPOSE_FILE="$COMPOSE"
# Pausing preserves tmpfs data while forcing real PostgreSQL timeouts.
export LOAD_DB_FAULT_MODE=pause
export ANPFUEL_TEST_DRILL_DSN_PREFIX="postgres://anpfuel:anpfuel@$DB_ADDR"
(
    cd backend
    go run ./cmd/migrate >"$REPORT/migrations.log" 2>&1
    go test -json -race -count=1 ./... >"$REPORT/unit.json" 2>"$REPORT/unit.stderr"
    go test -json -race -count=1 -p 2 -timeout 10m -tags=integration ./... >"$REPORT/integration.json" 2>"$REPORT/integration.stderr"
    # Exercise the deliberately unavailable storage path too.
    ANPFUEL_TEST_STORAGE_ENDPOINT= go test -race -tags=integration -run '^TestPublicProcessIdentityAndSignedWrites$' -count=1 ./cmd/api >"$REPORT/no-storage.log" 2>&1
    go vet ./... >"$REPORT/vet.log" 2>&1
    staticcheck ./... >"$REPORT/staticcheck.log" 2>&1
    sqlc vet >"$REPORT/sqlc.log" 2>&1
    govulncheck -json ./... >"$REPORT/vulnerability.json" 2>"$REPORT/vulnerability.stderr"
)
bash scripts/tests/test-infra-config.sh >"$REPORT/infra.log" 2>&1
bash scripts/tests/test-deploy.sh >"$REPORT/deploy.log" 2>&1
bash scripts/tests/test-backup.sh >"$REPORT/backup.log" 2>&1
bash scripts/tests/test-restore.sh >"$REPORT/restore.log" 2>&1
bash scripts/tests/test-load.sh >"$REPORT/load.log" 2>&1
bash scripts/check-security.sh --static-only >"$REPORT/security.log" 2>&1
python3 - "$REPORT" <<'PY'
import json, pathlib, sys
root=pathlib.Path(sys.argv[1]); report={"scope":"local-only", "release_certified":False}
for name in ("unit","integration"):
    events=[json.loads(line) for line in (root/(name+".json")).read_text().splitlines()]
    passed=sum(e.get("Action")=="pass" and "Test" in e for e in events)
    failed=sum(e.get("Action")=="fail" for e in events)
    skipped=sum(e.get("Action")=="skip" and "Test" in e for e in events)
    report[name]={"passed_tests":passed,"failed_events":failed,"skipped_tests":skipped}
    if failed: raise SystemExit("test failures detected")
(root/"summary.json").write_text(json.dumps(report,indent=2)+"\n")
print(json.dumps(report,indent=2))
PY
printf 'Local backend matrix passed. Reports: %s\n' "$REPORT"
