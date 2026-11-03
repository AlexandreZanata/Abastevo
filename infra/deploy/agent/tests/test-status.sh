#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/bin" "$WORK/state" "$WORK/receipts"
printf '%s\n' '{"sha":"1111111111111111111111111111111111111111","host":"PRIVATE-CANARY","images":{"api":"PRIVATE-CANARY"}}' > "$WORK/state/current.json"
printf '%s\n' '{"result":"failed","stage":"rollout","message":"PRIVATE-CANARY"}' > "$WORK/receipts/failure.json"
cat > "$WORK/bin/systemctl" <<'STUB'
#!/usr/bin/env bash
printf 'ActiveState=active\nResult=success\n'
STUB
cat > "$WORK/bin/kubectl" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' '{"items":[{"kind":"Pod","metadata":{"name":"owned-fixture","labels":{"app":"abastevo-temp-api"}},"status":{"phase":"Running","podIP":"PRIVATE-CANARY","containerStatuses":[{"name":"api","ready":true,"restartCount":0}]}},{"kind":"ResourceQuota","status":{"hard":{"limits.cpu":"6"},"used":{"limits.cpu":"3500m"}}}]}'
STUB
chmod +x "$WORK/bin"/*
PATH="$WORK/bin:$PATH" ABASTEVO_STATE_DIR="$WORK/state" ABASTEVO_RECEIPT_DIR="$WORK/receipts" bash "$ROOT/infra/deploy/agent/status.sh" > "$WORK/status.json"
if grep -q 'PRIVATE-CANARY' "$WORK/status.json"; then exit 1; fi
jq -e '.current.sha == "1111111111111111111111111111111111111111" and .last_receipt.result == "failed" and .cluster[0].containers[0].ready == true' "$WORK/status.json" >/dev/null
printf 'Status privacy/scope test passed.\n'
