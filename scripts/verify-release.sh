#!/usr/bin/env bash
# P01-T13 release verification entry point (foundation subset).
# Runs quick verification, then reports the complete release matrix with
# explicit outstanding work. During P01 only foundation checks exist, so this
# command always ends NOT CERTIFIED; only the P09 expected-result manifest on
# an immutable candidate can certify G09. Never claim a release from this
# foundation subset alone.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

bash scripts/quick-verify.sh

echo "== release matrix (P01 foundation subset) =="
echo "Present in this tree:"
echo "  - quick-verify (gofmt, build, vet, unit, staticcheck, sqlc, openapi, secrets, govulncheck)"
echo "  - backend fast gate script and existing backend/integration CI jobs"
echo "  - disposable PostGIS integration via compose + -tags=integration (selected, not always-on)"
echo "Outstanding, required before any release certification:"
while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%%#*}"
    line="$(echo "$line" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
    if [[ -z "$line" || "$line" == "["* ]]; then
        continue
    fi
    case "$line" in
        backend/*|gofmt|go-*|unit-*|staticcheck|sqlc-*|openapi-*|secret-*|govulncheck|docs-refs)
            ;;
        *)
            echo "  - FUTURE: $line"
            ;;
    esac
done < scripts/check-manifest.txt

# Guard against compile-only masquerading as behavioral evidence.
# Only non-comment invocations count; comments documenting the rule are fine.
if grep -rn "^[^#]*go test -run" scripts/quick-verify.sh scripts/verify-release.sh Makefile 2>/dev/null | grep -q "test"; then
    echo "ERROR: unlabeled compile-only test invocation; label it compile-only explicitly" >&2
    exit 1
fi

HEAD_SHA="$(git rev-parse HEAD)"
echo "verify-release: foundation subset only, NOT CERTIFIED (head=$HEAD_SHA)"
echo "Release certification requires the P09 immutable candidate and its complete expected-result manifest."
