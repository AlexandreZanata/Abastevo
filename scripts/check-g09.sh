#!/usr/bin/env bash
# Deferred release record validator. Never certifies runtime/production.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
refuse() { echo "REFUSED: $*" >&2; exit 2; }
SIGNOFF="${1:-docs/release-evidence/G09.md}"
[[ $# -le 1 ]] || refuse 'expected at most one record path'
[[ -f "$SIGNOFF" ]] || refuse "missing $SIGNOFF"
grep -Fqx 'Classification: **RELEASE**' "$SIGNOFF" || refuse 'G09 must be classified RELEASE'
grep -Fqx 'Status: **DEFERRED_UNTIL_APP_FUNCTIONAL**' "$SIGNOFF" || refuse 'unsupported certification/status claim'
grep -Fqx 'Entry for mobile: **G09-LOCAL integration**, not production certification.' "$SIGNOFF" || refuse 'missing protected local integration prerequisite'
grep -Fqx 'Production prerequisite: **G18 functional Android/iOS acceptance**.' "$SIGNOFF" || refuse 'missing functional app prerequisite'
grep -Fq '4ce5aaf79aa3bf8d8cf31c05635dfa2b30714556' "$SIGNOFF" || refuse 'historical provenance missing'
for condition in 'Fresh certification' 'Provisioned staging' 'Legal review' 'Wiki mirror'; do
    grep -Fq "$condition" "$SIGNOFF" || refuse "missing production condition: $condition"
done
for file in docs/release-evidence/p09-t01-rehearsal.md docs/release-evidence/p09-t02-compatibility.md docs/release-evidence/p08-t04-restore-drill.md docs/release-evidence/p08-t07-load.md docs/release-evidence/p08-t08-security.md contracts/testdata/compat/legacy-deltas.json docs/release-evidence/p09-local-runtime-validation.md; do
    [[ -f "$file" ]] || refuse "missing evidence $file"
done
echo 'g09 record ok: RELEASE deferred until G18; mobile requires G09-LOCAL integration'
