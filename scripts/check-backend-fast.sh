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

require_tool go "toolchain go1.27.2 via go.mod"
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
# The pinned staticcheck predates the go1.27.2 compiler export format, so it
# type-checks under go1.27.1 (same language version, older export data) while
# the build, tests and vulnerability scan run on the declared toolchain.
(cd backend && GOTOOLCHAIN=go1.27.1 staticcheck ./...)

echo "== sqlc vet + drift =="
(cd backend && sqlc vet)
(cd backend && sqlc diff)

echo "== openapi lint =="
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check

echo "== secret scan (tracked) =="
bash scripts/scan-secrets.sh

echo "== secret scan (changed files) =="
# Scanner scripts carry detection pattern literals; never scan themselves.
CHANGED="$(git status --porcelain -- backend/ contracts/ infra/ scripts/ .github/ 2>/dev/null | awk '{print $2}' | grep -v -e '^scripts/check-backend-fast.sh$' -e '^scripts/scan-secrets.sh$' -e '^scripts/quick-verify.sh$' -e '^scripts/check-security.sh$' -e '^scripts/verify-release.sh$' -e '^scripts/wiki.sh$' -e '^scripts/tests/test-gate-selection.sh$' || true)"
if [ -n "$CHANGED" ]; then
    FILES=""
    # shellcheck disable=SC2086
    for f in $CHANGED; do
        if [ -f "$f" ]; then
            FILES="$FILES $f"
        fi
    done
    if [ -n "$FILES" ]; then
        # shellcheck disable=SC2086
        set +e
        SCAN_OUT="$(grep -nE 'ghp_[A-Za-z0-9]{20,}|github_pat_|BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY|sk_live_|AKIA[0-9A-Z]{16}|xox[bap]-' $FILES 2>&1)"
        SCAN_EXIT=$?
        set -e
        if [ "$SCAN_EXIT" -eq 0 ]; then
            echo "$SCAN_OUT"
            echo "ERROR: potential secret pattern in changed files"
            exit 1
        elif [ "$SCAN_EXIT" -ne 1 ]; then
            echo "$SCAN_OUT" >&2
            echo "ERROR: changed-file secret scanner failed (exit $SCAN_EXIT)"
            exit 1
        fi
    fi
fi

echo "Backend fast gate passed."
