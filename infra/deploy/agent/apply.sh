#!/usr/bin/env bash
# Apply orchestration for the staging deploy agent.
# shellcheck disable=SC2154
#
# Sourced only when ABASTEVO_APPLY=1. The runtime adapter supplies all cluster
# operations; this file owns the release order and the failure rollback.

APPLY_STAGE="preflight"
APPLY_ROLLOUT_STARTED=0
APPLY_LANDING_SWITCHED=0
APPLY_STRATEGY_PATCHED=0
APPLY_PREVIOUS_LANDING=""
APPLY_PREVIOUS_STRATEGY_JSON=""
APPLY_SURGE=false
APPLY_MAX_SURGE=0
APPLY_MAX_UNAVAILABLE=1
APPLY_GAP=true
APPLY_QUOTA_HEADROOM_MILLI=0
APPLY_BACKUP_NAME=""
APPLY_BACKUP_JOB=""
APPLY_MIGRATE_JOB=""
APPLY_MIGRATE_OUTPUT=""
APPLY_SCHEMA_CURRENT=""
APPLY_SCHEMA_DB=""
APPLY_SCHEMA_TARGET=""
RUNTIME_IMAGE_SET=0

apply_log() {
    log "apply[$APPLY_STAGE]: $*"
}

apply_restore_landing() {
    local landing_root="$ABASTEVO_LANDING_ROOT"
    local target="$APPLY_PREVIOUS_LANDING"
    if [[ -n "$target" ]]; then
        ln -sfn "$target" "$landing_root/.current.rollback.$$"
        mv -Tf "$landing_root/.current.rollback.$$" "$landing_root/current"
    else
        rm -f "$landing_root/current"
    fi
}

apply_restore_strategy() {
    local max_surge max_unavailable
    max_surge="$(printf '%s' "$APPLY_PREVIOUS_STRATEGY_JSON" | jq -r '.maxSurge // 0 | tostring')"
    max_unavailable="$(printf '%s' "$APPLY_PREVIOUS_STRATEGY_JSON" | jq -r '.maxUnavailable // 1 | tostring')"
    [[ "$max_surge" =~ ^[0-9]+$ ]] || max_surge=0
    [[ "$max_unavailable" =~ ^[0-9]+$ ]] || max_unavailable=1
    runtime_patch_strategy "$ABASTEVO_NAMESPACE" "$ABASTEVO_API_DEPLOYMENT" \
        "$max_surge" "$max_unavailable"
}

apply_write_receipt() {
    local result="$1" stage="$2" message="$3"
    local rollout_undone="$4" landing_rolled_back="$5" strategy_restored="$6"
    local created_at receipt_name receipt_tmp target current
    created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    target="$(jq '{sha, images, landing_sha256, migrations_max, schema_min_supported}' "$manifest_file")"
    current="$(jq 'if . == null then null else {sha, images, landing_sha256, migrations_max, schema_min_supported} end' "$workdir/current.json")"
    if [[ "$result" == "deployed" ]]; then
        receipt_name="deploy-${created_at//[:]/}-$$.json"
    else
        receipt_name="failure-${created_at//[:]/}-$$.json"
    fi
    install -d -m 0700 "$ABASTEVO_RECEIPT_DIR"
    receipt_tmp="$(mktemp "$ABASTEVO_RECEIPT_DIR/.receipt.XXXXXX")"
    if jq -n \
        --arg schema "abastevo-deploy-receipt-v1" \
        --arg result "$result" \
        --arg stage "$stage" \
        --arg message "$message" \
        --arg created_at "$created_at" \
        --arg namespace "$ABASTEVO_NAMESPACE" \
        --arg api_deployment "$ABASTEVO_API_DEPLOYMENT" \
        --arg worker_deployment "$ABASTEVO_WORKER_DEPLOYMENT" \
        --arg api_container "$ABASTEVO_API_CONTAINER" \
        --arg worker_container "$ABASTEVO_WORKER_CONTAINER" \
        --arg backup "$APPLY_BACKUP_NAME" \
        --arg backup_job "$APPLY_BACKUP_JOB" \
        --arg migrate_job "$APPLY_MIGRATE_JOB" \
        --arg migrate_output "$APPLY_MIGRATE_OUTPUT" \
        --arg schema_current "$APPLY_SCHEMA_CURRENT" \
        --arg schema_db "$APPLY_SCHEMA_DB" \
        --arg schema_target "$APPLY_SCHEMA_TARGET" \
        --argjson surge "$APPLY_SURGE" \
        --argjson max_surge "$APPLY_MAX_SURGE" \
        --argjson max_unavailable "$APPLY_MAX_UNAVAILABLE" \
        --argjson gap "$APPLY_GAP" \
        --argjson quota_headroom_milli "$APPLY_QUOTA_HEADROOM_MILLI" \
        --argjson rollout_started "$([[ "$APPLY_ROLLOUT_STARTED" == "1" ]] && printf true || printf false)" \
        --argjson landing_switched "$([[ "$APPLY_LANDING_SWITCHED" == "1" ]] && printf true || printf false)" \
        --argjson rollout_undone "$rollout_undone" \
        --argjson landing_rolled_back "$landing_rolled_back" \
        --argjson strategy_restored "$strategy_restored" \
        --argjson target "$target" \
        --argjson current "$current" \
        '{
            schema: $schema,
            result: $result,
            stage: $stage,
            message: $message,
            created_at: $created_at,
            host: {
                namespace: $namespace,
                api_deployment: $api_deployment,
                worker_deployment: $worker_deployment,
                api_container: $api_container,
                worker_container: $worker_container
            },
            current: $current,
            target: $target,
            schema_gate: {
                current: $schema_current,
                database: $schema_db,
                target: $schema_target
            },
            surge: $surge,
            strategy: {
                maxSurge: $max_surge,
                maxUnavailable: $max_unavailable,
                gap: $gap
            },
            quota_headroom_milli: $quota_headroom_milli,
            backup: $backup,
            backup_job: $backup_job,
            migrate_job: $migrate_job,
            migrate_output: $migrate_output,
            rollout_started: $rollout_started,
            landing_switched: $landing_switched,
            rollout_undone: $rollout_undone,
            landing_rolled_back: $landing_rolled_back,
            strategy_restored: $strategy_restored
        }' > "$receipt_tmp"; then
        chmod 0600 "$receipt_tmp"
        mv "$receipt_tmp" "$ABASTEVO_RECEIPT_DIR/$receipt_name"
        apply_log "receipt $receipt_name"
    else
        rm -f "$receipt_tmp"
        log "could not write the $result receipt"
    fi
}

apply_fail() {
    local stage="$1" message="$2"
    set +e
    trap - ERR
    local rollout_undone=false landing_rolled_back=false strategy_restored=false
    if [[ "$APPLY_LANDING_SWITCHED" == "1" ]]; then
        if apply_restore_landing; then landing_rolled_back=true; else log "landing rollback failed"; fi
    fi
    if [[ "$APPLY_ROLLOUT_STARTED" == "1" ]]; then
        if runtime_rollout_undo "$ABASTEVO_NAMESPACE" "$ABASTEVO_API_DEPLOYMENT"; then
            rollout_undone=true
        else
            log "rollout undo failed for the api deployment"
        fi
        if [[ "$ABASTEVO_WORKER_DEPLOYMENT" != "$ABASTEVO_API_DEPLOYMENT" ]]; then
            runtime_rollout_undo "$ABASTEVO_NAMESPACE" "$ABASTEVO_WORKER_DEPLOYMENT" \
                || log "rollout undo failed for the worker deployment"
        fi
    elif [[ "$APPLY_STRATEGY_PATCHED" == "1" ]]; then
        if apply_restore_strategy; then strategy_restored=true; else log "strategy restore failed"; fi
    fi
    log "apply failed at $stage: $message"
    apply_write_receipt "failed" "$stage" "$message" "$rollout_undone" \
        "$landing_rolled_back" "$strategy_restored"
    exit 1
}

apply_trap() {
    local status="$1"
    trap - ERR
    apply_fail "$APPLY_STAGE" "command failed with status $status"
}

apply_acquire_lock() {
    install -d -m 0700 "$(dirname "$ABASTEVO_LOCK_FILE")"
    exec 9>"$ABASTEVO_LOCK_FILE"
    chmod 0600 "$ABASTEVO_LOCK_FILE"
    if ! flock -n 9; then
        apply_fail "lock" "another apply is already running"
    fi
}

apply_preflight() {
    local name
    for name in ABASTEVO_API_CONTAINER ABASTEVO_WORKER_CONTAINER \
        ABASTEVO_DB_IMAGE ABASTEVO_DB_SECRET ABASTEVO_SMOKE_BASE_URL \
        ABASTEVO_LOCK_FILE KUBECONFIG; do
        require_var "$name"
    done
    for tool in flock tar sha256sum; do
        require_tool "$tool"
    done
    [[ -x "$ABASTEVO_KUBECTL" ]] || command -v "$ABASTEVO_KUBECTL" >/dev/null 2>&1 \
        || fail "ABASTEVO_KUBECTL is not executable"
    [[ "$ABASTEVO_SMOKE_BASE_URL" =~ ^https?:// ]] \
        || fail "ABASTEVO_SMOKE_BASE_URL must be an http(s) URL"
    [[ "$ABASTEVO_SURGE_CPU_MILLI" =~ ^[0-9]+$ ]] \
        || fail "ABASTEVO_SURGE_CPU_MILLI must be an integer"
    [[ "$ABASTEVO_ALLOW_SURGE" =~ ^[01]$ ]] \
        || fail "ABASTEVO_ALLOW_SURGE must be 0 or 1"
    [[ "$ABASTEVO_ROLLOUT_TIMEOUT" =~ ^[0-9]+$ ]] \
        || fail "ABASTEVO_ROLLOUT_TIMEOUT must be an integer"
    [[ "$ABASTEVO_JOB_TIMEOUT" =~ ^[0-9]+$ ]] \
        || fail "ABASTEVO_JOB_TIMEOUT must be an integer"
    [[ "$ABASTEVO_BACKUP_KEEP" =~ ^[0-9]+$ ]] \
        || fail "ABASTEVO_BACKUP_KEEP must be an integer"
    [[ "$ABASTEVO_LANDING_KEEP" =~ ^[0-9]+$ ]] \
        || fail "ABASTEVO_LANDING_KEEP must be an integer"
    local landing_digest manifest_landing
    landing_digest="$(jq -r '[.layers[] | select(.mediaType == "application/vnd.abastevo.landing.tar+gzip")] | if length == 1 then .[0].digest else empty end' "$workdir/oci-manifest.json")"
    [[ "$landing_digest" =~ ^sha256:[0-9a-f]{64}$ ]] \
        || fail "signed landing layer is missing or ambiguous"
    [[ -f "$workdir/landing-dist.tar.gz" ]] || fail "landing archive is missing"
    manifest_landing="sha256:$(sha256sum "$workdir/landing-dist.tar.gz" | cut -d' ' -f1)"
    [[ "$manifest_landing" == "$landing_digest" ]] \
        || fail "landing archive digest does not match the OCI layer"
    [[ "${manifest_landing#sha256:}" == "$(jq -r '.landing_sha256' "$manifest_file")" ]] \
        || fail "landing archive digest does not match the signed manifest"
    apply_acquire_lock
}

apply_schema_gate() {
    local current_floor db_max target_floor job
    if [[ ! -f "$current_file" ]]; then
        apply_fail "schema" "baseline required"
    fi
    current_floor="$(jq -r '.schema_min_supported // empty' "$current_file" 2>/dev/null || true)"
    [[ "$current_floor" =~ ^[0-9]{6}$ ]] \
        || apply_fail "schema" "current state has no schema floor; baseline required"
    target_floor="$(jq -r '.schema_min_supported // empty' "$manifest_file")"
    [[ "$target_floor" =~ ^[0-9]{6}$ ]] \
        || apply_fail "schema" "manifest has no schema floor"
    job="schema-${release_sha:0:12}-$(date -u +%H%M%S)"
    db_max="$(runtime_schema_version "$ABASTEVO_NAMESPACE" "$job" \
        "$ABASTEVO_DB_IMAGE" "$ABASTEVO_DB_SECRET" "$release_sha" "$ABASTEVO_JOB_TIMEOUT")" \
        || apply_fail "schema" "schema version job failed"
    [[ "$db_max" =~ ^[0-9]{6}$ ]] \
        || apply_fail "schema" "database schema version is unavailable"
    local migrations_max
    migrations_max="$(jq -r '.migrations_max' "$manifest_file")"
    [[ "$migrations_max" =~ ^[0-9]{6}$ ]] \
        || apply_fail "schema" "manifest has no migration chain maximum"
    # Versions keep leading zeros; compare in base 10, never as octal.
    (( 10#$current_floor <= 10#$db_max )) \
        || apply_fail "schema" "running floor is above the database schema"
    (( 10#$target_floor <= 10#$current_floor )) \
        || apply_fail "schema" "contract schema floor refused"
    (( 10#$target_floor <= 10#$migrations_max )) \
        || apply_fail "schema" "target floor is above the migration chain"
    APPLY_SCHEMA_CURRENT="$current_floor"
    APPLY_SCHEMA_DB="$db_max"
    APPLY_SCHEMA_TARGET="$target_floor"
    apply_log "schema window $current_floor..$db_max, target floor $target_floor"
}

apply_prune_backups() {
    local dump file
    while IFS= read -r dump; do
        [[ -n "$dump" ]] || continue
        file="$(basename "$dump")"
        [[ "$file" == "$APPLY_BACKUP_NAME.dump" ]] && continue
        rm -f "$dump" "$dump.sha256"
    done < <(find "$ABASTEVO_BACKUP_DIR" -maxdepth 1 -name 'pre-*.dump' -printf '%T@ %p\n' \
        | sort -nr | tail -n +$((ABASTEVO_BACKUP_KEEP + 1)) | cut -d' ' -f2-)
}

apply_backup() {
    local job dump hashfile expected actual
    install -d -m 0700 "$ABASTEVO_BACKUP_DIR"
    APPLY_BACKUP_NAME="pre-${release_sha:0:12}-$(date -u +%Y%m%dT%H%M%SZ)"
    job="backup-${release_sha:0:12}-$(date -u +%H%M%S)"
    runtime_backup "$ABASTEVO_NAMESPACE" "$job" "$ABASTEVO_DB_IMAGE" \
        "$ABASTEVO_DB_SECRET" "$ABASTEVO_BACKUP_DIR" "$APPLY_BACKUP_NAME" \
        "$release_sha" "$ABASTEVO_JOB_TIMEOUT" \
        || apply_fail "backup" "backup job failed"
    APPLY_BACKUP_JOB="$job"
    dump="$ABASTEVO_BACKUP_DIR/$APPLY_BACKUP_NAME.dump"
    hashfile="$dump.sha256"
    [[ -s "$dump" ]] || apply_fail "backup" "backup archive is missing"
    [[ -s "$hashfile" ]] || apply_fail "backup" "backup checksum is missing"
    expected="$(tr -d '[:space:]' < "$hashfile")"
    actual="$(sha256sum "$dump" | cut -d' ' -f1)"
    [[ "$expected" =~ ^[0-9a-f]{64}$ && "$expected" == "$actual" ]] \
        || apply_fail "backup" "backup checksum mismatch"
    apply_prune_backups
    apply_log "backup verified: $APPLY_BACKUP_NAME.dump"
}

apply_migrate() {
    local job output
    job="migrate-${release_sha:0:12}-$(date -u +%H%M%S)"
    output="$(runtime_migrate "$ABASTEVO_NAMESPACE" "$job" \
        "$(jq -r '.images.migrate' "$manifest_file")" "$ABASTEVO_DB_SECRET" \
        "$release_sha" "$ABASTEVO_JOB_TIMEOUT")" \
        || apply_fail "migrate" "migration job failed"
    APPLY_MIGRATE_JOB="$job"
    APPLY_MIGRATE_OUTPUT="$output"
    apply_log "migration job completed: $job"
}

apply_rollout() {
    local headroom max_surge max_unavailable prev_surge prev_unavailable
    headroom="$(runtime_quota_cpu_headroom_milli "$ABASTEVO_NAMESPACE" \
        "$ABASTEVO_SURGE_RESOURCE_QUOTA")" || headroom=0
    [[ "$headroom" =~ ^[0-9]+$ ]] || headroom=0
    APPLY_QUOTA_HEADROOM_MILLI="$headroom"
    APPLY_PREVIOUS_STRATEGY_JSON="$(runtime_deployment_strategy \
        "$ABASTEVO_NAMESPACE" "$ABASTEVO_API_DEPLOYMENT")" \
        || apply_fail "rollout" "cannot read the deployment strategy"
    prev_surge="$(printf '%s' "$APPLY_PREVIOUS_STRATEGY_JSON" | jq -r '.maxSurge // 0 | tostring')"
    prev_unavailable="$(printf '%s' "$APPLY_PREVIOUS_STRATEGY_JSON" | jq -r '.maxUnavailable // 1 | tostring')"
    if [[ "$ABASTEVO_ALLOW_SURGE" == "1" ]] \
        && (( headroom >= ABASTEVO_SURGE_CPU_MILLI )); then
        APPLY_SURGE=true
        max_surge=1
        max_unavailable=0
    else
        APPLY_SURGE=false
        max_surge=0
        max_unavailable=1
    fi
    APPLY_MAX_SURGE="$max_surge"
    APPLY_MAX_UNAVAILABLE="$max_unavailable"
    if (( max_unavailable >= 1 )); then APPLY_GAP=true; else APPLY_GAP=false; fi
    if [[ "$prev_surge" != "$max_surge" || "$prev_unavailable" != "$max_unavailable" ]]; then
        runtime_patch_strategy "$ABASTEVO_NAMESPACE" "$ABASTEVO_API_DEPLOYMENT" \
            "$max_surge" "$max_unavailable" \
            || apply_fail "rollout" "strategy patch failed"
        APPLY_STRATEGY_PATCHED=1
    fi
    RUNTIME_IMAGE_SET=0
    if [[ "$ABASTEVO_WORKER_DEPLOYMENT" == "$ABASTEVO_API_DEPLOYMENT" ]]; then
        if ! runtime_set_images "$ABASTEVO_NAMESPACE" "$ABASTEVO_API_DEPLOYMENT" \
            "$ABASTEVO_API_CONTAINER" "$(jq -r '.images.api' "$manifest_file")" \
            "$ABASTEVO_WORKER_CONTAINER" "$(jq -r '.images.worker' "$manifest_file")"; then
            [[ "$RUNTIME_IMAGE_SET" == "1" ]] && APPLY_ROLLOUT_STARTED=1
            apply_fail "rollout" "image update failed"
        fi
        APPLY_ROLLOUT_STARTED=1
        runtime_rollout_status "$ABASTEVO_NAMESPACE" "$ABASTEVO_API_DEPLOYMENT" \
            "$ABASTEVO_ROLLOUT_TIMEOUT" \
            || apply_fail "rollout" "api rollout failed"
    else
        if ! runtime_set_image "$ABASTEVO_NAMESPACE" "$ABASTEVO_API_DEPLOYMENT" \
            "$ABASTEVO_API_CONTAINER" "$(jq -r '.images.api' "$manifest_file")"; then
            [[ "$RUNTIME_IMAGE_SET" == "1" ]] && APPLY_ROLLOUT_STARTED=1
            apply_fail "rollout" "api image update failed"
        fi
        APPLY_ROLLOUT_STARTED=1
        if ! runtime_set_image "$ABASTEVO_NAMESPACE" "$ABASTEVO_WORKER_DEPLOYMENT" \
            "$ABASTEVO_WORKER_CONTAINER" "$(jq -r '.images.worker' "$manifest_file")"; then
            apply_fail "rollout" "worker image update failed"
        fi
        runtime_rollout_status "$ABASTEVO_NAMESPACE" "$ABASTEVO_API_DEPLOYMENT" \
            "$ABASTEVO_ROLLOUT_TIMEOUT" \
            || apply_fail "rollout" "api rollout failed"
        runtime_rollout_status "$ABASTEVO_NAMESPACE" "$ABASTEVO_WORKER_DEPLOYMENT" \
            "$ABASTEVO_ROLLOUT_TIMEOUT" \
            || apply_fail "rollout" "worker rollout failed"
    fi
    apply_log "rollout completed (surge=$APPLY_SURGE, headroom=${headroom}m)"
}

apply_smoke() {
    local base="$ABASTEVO_SMOKE_BASE_URL" timeout="$ABASTEVO_SMOKE_TIMEOUT" shard
    base="${base%/}"
    curl -fsS --max-time "$timeout" -o /dev/null "$base/health/ready" || return 1
    curl -fsS --max-time "$timeout" -o /dev/null "$base/" || return 1
    curl -fsS --max-time "$timeout" -o /dev/null "$base/consultar/" || return 1
    curl -fsS --max-time "$timeout" -o /dev/null "$base/v1/stations?limit=1" || return 1
    curl -fsS --max-time "$timeout" -o "$workdir/portal-manifest.json" \
        "$base/portal-manifest.json" || return 1
    shard="$(jq -r '[.files[]?.path | select(startswith("dados/busca/") and endswith(".json"))][0] // empty' "$workdir/portal-manifest.json")"
    [[ -n "$shard" ]] || return 1
    curl -fsS --max-time "$timeout" -o /dev/null "$base/$shard" || return 1
}

apply_landing() {
    local landing_root="$ABASTEVO_LANDING_ROOT" releases staged release_dir
    releases="$landing_root/releases"
    release_dir="$releases/$release_sha"
    install -d -m 0755 "$landing_root" "$releases"
    staged="$releases/.$release_sha.$$"
    rm -rf "$staged"
    mkdir -p "$staged"
    tar -xzf "$workdir/landing-dist.tar.gz" -C "$staged" \
        || apply_fail "landing" "landing archive extraction failed"
    chmod -R a+rX "$staged"
    APPLY_PREVIOUS_LANDING="$(readlink "$landing_root/current" 2>/dev/null || true)"
    rm -rf "$release_dir"
    mv "$staged" "$release_dir"
    ln -sfn "releases/$release_sha" "$landing_root/.current.$$"
    mv -Tf "$landing_root/.current.$$" "$landing_root/current" \
        || apply_fail "landing" "landing switch failed"
    APPLY_LANDING_SWITCHED=1
    local old
    while IFS= read -r old; do
        [[ "$old" =~ ^[0-9a-f]{40}$ ]] || continue
        [[ "$old" == "$release_sha" ]] && continue
        rm -rf "${releases:?}/$old"
    done < <(find "$releases" -mindepth 1 -maxdepth 1 -type d -printf '%T@ %f\n' \
        | sort -nr | tail -n +$((ABASTEVO_LANDING_KEEP + 1)) | cut -d' ' -f2-)
    apply_log "landing switched to releases/$release_sha"
}

apply_write_current_state() {
    local tmp state_dir applied_at
    state_dir="$ABASTEVO_STATE_DIR"
    install -d -m 0700 "$state_dir"
    applied_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    tmp="$(mktemp "$state_dir/.current.XXXXXX")"
    jq -n \
        --arg schema "abastevo-deploy-current-v1" \
        --arg sha "$release_sha" \
        --arg landing_sha256 "$(jq -r '.landing_sha256' "$manifest_file")" \
        --arg migrations_max "$(jq -r '.migrations_max' "$manifest_file")" \
        --arg schema_min_supported "$(jq -r '.schema_min_supported' "$manifest_file")" \
        --arg applied_at "$applied_at" \
        --argjson images "$(jq '.images' "$manifest_file")" \
        '{
            schema: $schema,
            sha: $sha,
            images: $images,
            landing_sha256: $landing_sha256,
            migrations_max: $migrations_max,
            schema_min_supported: $schema_min_supported,
            applied_at: $applied_at
        }' > "$tmp" || {
        rm -f "$tmp"
        apply_fail "state" "current state write failed"
    }
    chmod 0600 "$tmp"
    mv "$tmp" "$current_file" || apply_fail "state" "current state replace failed"
}

apply_release() {
    if [[ "$action" != "plan-only" ]]; then
        log "no changes to apply"
        return 0
    fi
    trap 'apply_trap $?' ERR
    APPLY_STAGE="preflight"
    apply_preflight
    APPLY_STAGE="schema"
    apply_schema_gate
    APPLY_STAGE="backup"
    apply_backup
    APPLY_STAGE="migrate"
    apply_migrate
    APPLY_STAGE="rollout"
    apply_rollout
    APPLY_STAGE="smoke"
    apply_smoke || apply_fail "smoke" "smoke check failed"
    APPLY_STAGE="landing"
    apply_landing
    APPLY_STAGE="state"
    apply_write_current_state
    trap - ERR
    APPLY_STAGE="done"
    apply_write_receipt "deployed" "done" "" false false false
    runtime_annotate_source "$ABASTEVO_NAMESPACE" "$ABASTEVO_API_DEPLOYMENT" \
        "$release_sha" "$(jq -r '.schema_min_supported' "$manifest_file")" \
        || log "source annotations were not updated"
    log "release $release_sha deployed"
}
