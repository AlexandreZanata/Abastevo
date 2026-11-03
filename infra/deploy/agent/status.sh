#!/usr/bin/env bash
# Read-only, namespace-scoped status. Never print host configuration/raw errors.
set -euo pipefail
STATE="${ABASTEVO_STATE_DIR:-/var/lib/abastevo-deploy}"
RECEIPTS="${ABASTEVO_RECEIPT_DIR:-$STATE/receipts}"

service_status() {
    local value
    value="$(systemctl show "$1" --property=ActiveState,SubState,Result,UnitFileState,LastTriggerUSec 2>/dev/null)" || value=""
    printf '%s' "$value" | jq -Rsc 'split("\n") | map(select(test("^(ActiveState|SubState|Result|UnitFileState|LastTriggerUSec)=")) | split("=") | {(.[0]):.[1]}) | add // {}'
}

state_summary() {
    if [[ ! -f "$1" ]]; then printf 'null'; return; fi
    jq '{sha:((.sha // .target.sha // "") | if test("^[0-9a-f]{40}$") then . else null end),
         migrations_max:(.migrations_max | if type == "string" and test("^[0-9]{6}$") then . else null end),
         schema_min_supported:(.schema_min_supported | if type == "string" and test("^[0-9]{6}$") then . else null end),
         at:((.created_at // .applied_at) | if type == "string" and test("^[0-9T:Z.+-]{16,40}$") then . else null end),
         result:((.result // .status // .action // "") | if test("^(deployed|failed|rejected|refused|no-change|plan-only|baseline)$") then . else null end),
         stage:(.stage | if type == "string" and test("^[a-z-]{1,30}$") then . else null end)}' "$1" 2>/dev/null || printf '{"unreadable":true}'
}

last=""
if [[ -d "$RECEIPTS" ]]; then
    last="$(find "$RECEIPTS" -maxdepth 1 -type f -name '*.json' -printf '%T@ %p\n' | sort -nr | sed -n '1p' | cut -d' ' -f2-)"
fi
cluster="$("${ABASTEVO_KUBECTL:-kubectl}" -n abastevo-temp get pods,replicasets,resourcequotas -o json 2>/dev/null \
    | jq '[.items[] | if .kind == "Pod" and (.metadata.labels.app // "" | startswith("abastevo-temp-")) then
         {kind:.kind,name:.metadata.name,phase:.status.phase,
          containers:[.status.containerStatuses[]? | {name,ready,restarts:.restartCount}]}
       elif .kind == "ReplicaSet" and (.spec.replicas // 0) > 0 then
         {kind:.kind,name:.metadata.name,desired:.spec.replicas,ready:.status.readyReplicas}
       elif .kind == "ResourceQuota" then {kind:.kind,hard:.status.hard,used:.status.used}
       else empty end]')" || cluster='null'

jq -n --argjson timer "$(service_status abastevo-deploy-agent.timer)" \
    --argjson service "$(service_status abastevo-deploy-agent.service)" \
    --argjson current "$(state_summary "$STATE/current.json")" \
    --argjson last_receipt "$(state_summary "$last")" \
    --argjson checked "$(state_summary "$STATE/checked.json")" \
    --argjson held "$([[ -e "$STATE/operator-hold.json" ]] && printf true || printf false)" \
    --argjson inflight "$([[ -e "$STATE/inflight.json" ]] && printf true || printf false)" \
    --argjson cluster "$cluster" \
    '{timer:$timer,service:$service,current:$current,last_receipt:$last_receipt,checked:$checked,
      operator_hold:$held,inflight:$inflight,cluster:$cluster}'
