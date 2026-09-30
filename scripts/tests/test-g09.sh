#!/usr/bin/env bash
# Release-record policy tests; fixtures never mutate canonical evidence.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/deferred.md" <<'RECORD'
Classification: **RELEASE**
Status: **DEFERRED_UNTIL_APP_FUNCTIONAL**
Entry for mobile: **G09-LOCAL integration**, not production certification.
Production prerequisite: **G18 functional Android/iOS acceptance**.
Historical candidate: 4ce5aaf79aa3bf8d8cf31c05635dfa2b30714556
Fresh certification
Provisioned staging
Legal review
Wiki mirror
RECORD
bash scripts/check-g09.sh "$tmp/deferred.md"
for mutation in certified missing-app missing-legal mobile-without-integration; do
    cp "$tmp/deferred.md" "$tmp/mutant.md"
    case "$mutation" in
        certified) sed -i 's/DEFERRED_UNTIL_APP_FUNCTIONAL/RELEASE_CERTIFIED/' "$tmp/mutant.md" ;;
        missing-app) sed -i '/Production prerequisite:/d' "$tmp/mutant.md" ;;
        missing-legal) sed -i '/Legal review/d' "$tmp/mutant.md" ;;
        mobile-without-integration) sed -i '/Entry for mobile:/d' "$tmp/mutant.md" ;;
    esac
    if bash scripts/check-g09.sh "$tmp/mutant.md" > "$tmp/result" 2>&1; then
        echo "FAIL: accepted $mutation" >&2; exit 1
    fi
    grep -q 'REFUSED:' "$tmp/result"
    echo "PASS: refused $mutation"
done
if bash scripts/check-g09.sh "$tmp/absent.md" >/dev/null 2>&1; then
    echo 'FAIL: missing record accepted' >&2; exit 1
fi
bash scripts/check-g09.sh
echo 'g09 record campaign ok: deferred release; no certification inferred'
