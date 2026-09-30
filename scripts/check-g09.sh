#!/usr/bin/env bash
# P09-T03 G09 readiness gate: refuse certification while any release
# blocker is open. Docs-only (no product change): checks the sign-off
# file records G09 BLOCKED with explicit blockers, all required evidence
# files present, historical source provenance retained, and no
# COMPLETE claim without proof. Never prints secrets.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

log() { echo "g09: $*" >&2; }
refuse() { echo "REFUSED: $*" >&2; exit 2; }

SIGNOFF=docs/release-evidence/G09.md
[[ -f "$SIGNOFF" ]] || refuse "missing $SIGNOFF"

grep -q "^Status: \*\*G09 BLOCKED\*\*" "$SIGNOFF" || refuse "sign-off must record G09 BLOCKED until blockers clear"
grep -q "P10 MUST NOT start" "$SIGNOFF" || refuse "sign-off must keep P10 blocked"
grep -q "4ce5aaf79aa3bf8d8cf31c05635dfa2b30714556" "$SIGNOFF" || refuse "sign-off must pin candidate 4ce5aaf"
for b in "Fresh certification" "Provisioned staging" "Legal review" "Wiki mirror"; do
    grep -q "$b" "$SIGNOFF" || refuse "sign-off missing blocker: $b"
done
log "blockers recorded"

for f in docs/release-evidence/p09-t01-rehearsal.md docs/release-evidence/p09-t02-compatibility.md docs/release-evidence/p08-t04-restore-drill.md docs/release-evidence/p08-t07-load.md docs/release-evidence/p08-t08-security.md contracts/testdata/compat/legacy-deltas.json; do
    [[ -f "$f" ]] || refuse "missing evidence $f"
done
log "evidence files present"

if grep -q "^Status: \*\*G09 COMPLETE\*\*" "$SIGNOFF"; then
    refuse "COMPLETE claim requires provisioned-staging proof; not present"
fi

log "historical candidate evidence cannot certify current local changes"

echo "g09 ok: BLOCKED sign-off recorded (P10 stays blocked)"
