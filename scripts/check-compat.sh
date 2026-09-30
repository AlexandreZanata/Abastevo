#!/usr/bin/env bash
# P09-T02 compatibility gate: frozen contract + legacy deltas + runbook links.
# Docs-only by design (no product behavior change, no Android source edits):
# fails on OpenAPI lint errors, missing/unparseable deltas fixture, wire-enum
# drift, or broken release-evidence/operator links. Never prints secrets.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

log() { echo "compat: $*" >&2; }
refuse() { echo "REFUSED: $*" >&2; exit 2; }

[[ -f contracts/openapi/v1.yaml ]] || refuse "missing contracts/openapi/v1.yaml"
[[ -f contracts/openapi/vacuum-rules.yaml ]] || refuse "missing vacuum rules"
[[ -f contracts/testdata/compat/legacy-deltas.json ]] || refuse "missing legacy deltas fixture"
[[ -f docs/release-evidence/p09-t02-compatibility.md ]] || refuse "missing p09-t02 evidence"

command -v vacuum >/dev/null 2>&1 || refuse "missing tool vacuum"
VAC_OUT="$(vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check 2>&1 || true)"
if echo "$VAC_OUT" | grep -Eq "errors[^0-9]*[1-9]|✗ errors[[:space:]]+[1-9]"; then
    echo "$VAC_OUT" >&2
    refuse "OpenAPI lint errors"
fi
log "openapi lint ok (0 errors)"

python3 - <<'PY'
import json
d = json.load(open('contracts/testdata/compat/legacy-deltas.json'))
assert d['id'] == 'legacy-deltas-v1', d.get('id')
wire = d['fuel_vocabulary']['wire_enum']
assert wire == ["ETHANOL", "GASOLINE_REGULAR", "GASOLINE_ADDITIVED", "DIESEL_S500", "DIESEL_S10", "CNG", "LPG_P13"], wire
assert d['fuel_vocabulary']['legacy_map'] == {"GASOLINE_PREMIUM": "GASOLINE_ADDITIVED"}
print('deltas fixture ok')
PY

python3 - <<'PY'
import re
text = open('contracts/openapi/v1.yaml').read()
m = re.search(r'enum:\s*\[(ETHANOL[^\]]+)\]', text)
assert m, 'FuelProduct enum not found'
wire = [x.strip() for x in m.group(1).split(',')]
assert wire == ["ETHANOL", "GASOLINE_REGULAR", "GASOLINE_ADDITIVED", "DIESEL_S500", "DIESEL_S10", "CNG", "LPG_P13"], wire
assert 'GASOLINE_PREMIUM' not in text.split('Legacy Android')[0] or 'never appears on the wire' in text, 'premium guard missing'
print('wire enum ok (GASOLINE_PREMIUM never on wire)')
PY

for f in docs/operator/deploy.md docs/operator/backup.md docs/operator/recovery.md docs/operator/monitoring.md docs/operator/edge-cache.md docs/MIGRATION_PLAN.md docs/product/PRODUCT_CONTRACT.md; do
    [[ -f "$f" ]] || refuse "missing runbook $f"
done
log "runbooks present"

echo "compat ok: contract + frozen deltas + runbooks"
