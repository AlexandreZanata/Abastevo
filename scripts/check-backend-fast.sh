#!/usr/bin/env bash
# P01-T12 fast gate: compile, unit tests, formatting, vet, staticcheck,
# sqlc drift, OpenAPI lint and changed-file secret scan. No database needed.
# Fails loudly; never skips. Integration (real PostGIS) runs separately.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

require_tool() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "ERROR: missing tool $1 ($2)"
        exit 1
    fi
}

require_tool go "toolchain go1.27.1 via go.mod"
require_tool sqlc "go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1"
require_tool staticcheck "GOTOOLCHAIN=go1.27.1 go install honnef.co/go/tools/cmd/staticcheck@v0.8.1"
require_tool vacuum "go install github.com/daveshanley/vacuum@v0.30.6"

echo "== gofmt =="
GOFMT_OUT="$(gofmt -l backend/ contracts/ 2>/dev/null || true)"
# gofmt only lists; an explicit fail keeps the gate honest.
if [ -n "$GOFMT_OUT" ]; then
    echo "ERROR: unformatted files:"
    echo "$GOFMT_OUT"
    exit 1
fi

echo "== go build + vet + unit tests =="
cd backend
go build ./...
go vet ./...
go test -count=1 ./...
cd "$ROOT"

echo "== staticcheck =="
(cd backend && staticcheck ./...)

echo "== sqlc vet + drift =="
(cd backend && sqlc vet)
(cd backend && sqlc diff)

echo "== openapi lint =="
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check

echo "== secret scan (tracked) =="
bash scripts/scan-secrets.sh

echo "== secret scan (changed files) =="
# The gate script carries its own patterns, so it is never scanned.
CHANGED="$(git status --porcelain -- backend/ contracts/ infra/ scripts/ .github/ 2>/dev/null | awk '{print $2}' | grep -v '^scripts/check-backend-fast.sh$' || true)"
if [ -n "$CHANGED" ]; then
    # shellcheck disable=SC2086
    if grep -nE 'ghp_[A-Za-z0-9]{20,}|github_pat_|BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY|sk_live_|AKIA[0-9A-Z]{16}|xox[bap]-' $CHANGED 2>/dev/null; then
        echo "ERROR: potential secret pattern in changed files"
        exit 1
    fi
fi

echo "Backend fast gate passed."
