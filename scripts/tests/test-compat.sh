#!/usr/bin/env bash
# P09-T02 harness: compat gate happy path plus mutant refusals.
# Docs-only; no backend/Android suites here. Usage: make test-compat.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

echo "== compat happy path =="
bash scripts/check-compat.sh
echo "PASS: compat happy path"

echo "== mutant: broken wire enum refused =="
cp contracts/testdata/compat/legacy-deltas.json /tmp/deltas-backup.json
python3 -c "import json; d=json.load(open('contracts/testdata/compat/legacy-deltas.json')); d['fuel_vocabulary']['wire_enum']=[]; json.dump(d, open('contracts/testdata/compat/legacy-deltas.json','w'))"
if bash scripts/check-compat.sh >/dev/null 2>&1; then
    cp /tmp/deltas-backup.json contracts/testdata/compat/legacy-deltas.json
    echo "FAIL: broken enum accepted" >&2
    exit 1
fi
cp /tmp/deltas-backup.json contracts/testdata/compat/legacy-deltas.json
echo "PASS: broken enum refused"

echo "== mutant: missing evidence refused =="
mv docs/release-evidence/p09-t02-compatibility.md /tmp/p09-t02-backup.md
if bash scripts/check-compat.sh >/dev/null 2>&1; then
    mv /tmp/p09-t02-backup.md docs/release-evidence/p09-t02-compatibility.md
    echo "FAIL: missing evidence accepted" >&2
    exit 1
fi
mv /tmp/p09-t02-backup.md docs/release-evidence/p09-t02-compatibility.md
echo "PASS: missing evidence refused"

echo "compat campaign ok (docs-only; Android baseline recorded separately)"
