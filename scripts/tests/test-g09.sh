#!/usr/bin/env bash
# P09-T03 harness: G09 gate records BLOCKED plus mutant refusals.
# Docs-only. Usage: make test-g09.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

echo "== g09 happy path (BLOCKED recorded) =="
bash scripts/check-g09.sh
echo "PASS: g09 BLOCKED sign-off"

echo "== mutant: COMPLETE claim refused =="
cp docs/release-evidence/G09.md /tmp/g09-backup.md
sed -i 's/^Status: \*\*G09 BLOCKED\*\*.*/Status: **G09 COMPLETE** — test mutant/' docs/release-evidence/G09.md
if bash scripts/check-g09.sh >/dev/null 2>&1; then
    cp /tmp/g09-backup.md docs/release-evidence/G09.md
    echo "FAIL: COMPLETE claim accepted" >&2
    exit 1
fi
cp /tmp/g09-backup.md docs/release-evidence/G09.md
echo "PASS: COMPLETE claim refused"

echo "== mutant: missing blocker refused =="
python3 -c "t=open('docs/release-evidence/G09.md').read(); open('docs/release-evidence/G09.md','w').write(t.replace('Legal review','Removed-blocker'))"
if bash scripts/check-g09.sh >/dev/null 2>&1; then
    cp /tmp/g09-backup.md docs/release-evidence/G09.md
    echo "FAIL: missing blocker accepted" >&2
    exit 1
fi
cp /tmp/g09-backup.md docs/release-evidence/G09.md
echo "PASS: missing blocker refused"

echo "g09 campaign ok (BLOCKED honest; P10 stays blocked)"
