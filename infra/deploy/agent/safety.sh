#!/usr/bin/env bash
# Durable safety guards; all callers hold the agent lock (fd 9).
# shellcheck disable=SC2154,SC2034
safety_write() {
    local name="$1" payload="$2" tmp
    tmp="$(mktemp "$ABASTEVO_STATE_DIR/.safety.XXXXXX")"
    printf '%s\n' "$payload" > "$tmp"
    chmod 0600 "$tmp"
    mv "$tmp" "$ABASTEVO_STATE_DIR/$name"
}

safety_open() {
    if [[ "$COMMAND" == "release-hold" ]]; then
        rm -f "$ABASTEVO_STATE_DIR/operator-hold.json"
        log "operator hold released; rejected SHA still requires explicit recheck"
        exit 0
    fi
    [[ ! -e "$ABASTEVO_STATE_DIR/operator-hold.json" ]] \
        || fail "operator hold active; inspect recovery before release-hold"
    if [[ -e "$ABASTEVO_STATE_DIR/inflight.json" ]]; then
        jq -e '.schema == "abastevo-deploy-inflight-v1"
            and (.sha | test("^[0-9a-f]{40}$"))
            and (.previous_revision | test("^[0-9]+$"))
            and (.previous_strategy | type == "object")
            and (.previous_landing | type == "string")' \
            "$ABASTEVO_STATE_DIR/inflight.json" >/dev/null \
            || fail "invalid inflight journal; operator recovery required"
        [[ "$COMMAND" != "baseline" && -z "$RECHECK_SHA" ]] \
            || fail "recover inflight deployment before operator override"
        ABASTEVO_MANIFEST_TAG="sha-$(jq -r .sha "$ABASTEVO_STATE_DIR/inflight.json")"
        log "resuming the pinned interrupted candidate"
    fi
}

safety_refuse() {
    local reason="$1" tmp
    install -d -m 0700 "$ABASTEVO_RECEIPT_DIR"
    tmp="$(mktemp "$ABASTEVO_RECEIPT_DIR/.refusal.XXXXXX")"
    jq -n --arg sha "$release_sha" --arg reason "$reason" \
        --arg created_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        '{schema:"abastevo-deploy-refusal-v1",result:"refused",sha:$sha,reason:$reason,created_at:$created_at}' > "$tmp"
    mv "$tmp" "$ABASTEVO_RECEIPT_DIR/refusal-$(date -u +%Y%m%dT%H%M%SZ)-$$.json"
    fail "$reason"
}

safety_candidate() {
    local floor http_code status
    floor="$(jq -r '.sha // empty' "$workdir/current.json")"
    if [[ -n "$floor" && "$floor" != "$release_sha" ]]; then
        http_code="$(curl -sS -o "$workdir/floor.json" -w '%{http_code}' \
            --connect-timeout 10 --max-time 30 \
            -H 'Accept: application/vnd.github+json' \
            -H 'X-GitHub-Api-Version: 2022-11-28' \
            -H 'User-Agent: abastevo-deploy-agent' \
            "$ABASTEVO_GITHUB_API/repos/$ABASTEVO_GITHUB_REPOSITORY/compare/$floor...$release_sha")" \
            || safety_refuse "deployment floor comparison unavailable"
        [[ "$http_code" == "200" ]] || safety_refuse "deployment floor comparison HTTP refusal"
        status="$(jq -r '.status // empty' "$workdir/floor.json")" \
            || safety_refuse "invalid deployment floor response"
        [[ "$status" == "ahead" || "$status" == "identical" ]] \
            || safety_refuse "candidate does not descend from deployed SHA"
    fi
    if [[ -n "$RECHECK_SHA" && "$release_sha" != "$RECHECK_SHA" ]]; then
        safety_refuse "candidate differs from explicit recheck SHA"
    fi
    if [[ -e "$ABASTEVO_STATE_DIR/checked.json" ]]; then
        jq -e '.schema == "abastevo-deploy-checked-v1"
            and .status == "rejected" and (.sha | test("^[0-9a-f]{40}$"))' \
            "$ABASTEVO_STATE_DIR/checked.json" >/dev/null \
            || fail "invalid rejection latch; operator review required"
        if [[ "$(jq -r .sha "$ABASTEVO_STATE_DIR/checked.json")" == "$release_sha" && -z "$RECHECK_SHA" ]]; then
            log "rejected candidate skipped; use plan --recheck <sha> after review"
            exit 0
        fi
    fi
}

safety_reject() {
    safety_write checked.json "$(jq -n --arg sha "$release_sha" --arg stage "$APPLY_STAGE" \
        '{schema:"abastevo-deploy-checked-v1",status:"rejected",sha:$sha,stage:$stage}')"
}

safety_hold() {
    safety_write operator-hold.json "$(jq -n --arg sha "$release_sha" \
        --arg revision "$APPLY_PREVIOUS_REVISION" --arg landing "$APPLY_PREVIOUS_LANDING" \
        '{schema:"abastevo-deploy-hold-v1",sha:$sha,previous_revision:$revision,previous_landing:$landing,reason:"rollback incomplete; operator review required"}')"
}

safety_journal() {
    if [[ -f "$ABASTEVO_STATE_DIR/inflight.json" ]]; then
        APPLY_PREVIOUS_REVISION="$(jq -r .previous_revision "$ABASTEVO_STATE_DIR/inflight.json")"
        APPLY_PREVIOUS_STRATEGY_JSON="$(jq -c .previous_strategy "$ABASTEVO_STATE_DIR/inflight.json")"
        APPLY_PREVIOUS_LANDING="$(jq -r .previous_landing "$ABASTEVO_STATE_DIR/inflight.json")"
        return
    fi
    local deployment
    deployment="$(runtime_kubectl -n "$ABASTEVO_NAMESPACE" get deployment "$ABASTEVO_API_DEPLOYMENT" -o json)"
    APPLY_PREVIOUS_REVISION="$(printf '%s' "$deployment" | jq -r '.metadata.annotations["deployment.kubernetes.io/revision"] // empty')"
    [[ "$APPLY_PREVIOUS_REVISION" =~ ^[0-9]+$ ]] || apply_fail journal "deployment revision unavailable"
    APPLY_PREVIOUS_STRATEGY_JSON="$(printf '%s' "$deployment" | jq -c '.spec.strategy.rollingUpdate')"
    APPLY_PREVIOUS_LANDING="$(readlink "$ABASTEVO_LANDING_ROOT/current" 2>/dev/null || true)"
    safety_write inflight.json "$(jq -n --arg sha "$release_sha" \
        --arg revision "$APPLY_PREVIOUS_REVISION" --arg landing "$APPLY_PREVIOUS_LANDING" \
        --argjson strategy "$APPLY_PREVIOUS_STRATEGY_JSON" \
        '{schema:"abastevo-deploy-inflight-v1",sha:$sha,previous_revision:$revision,previous_landing:$landing,previous_strategy:$strategy}')"
}
