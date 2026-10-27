#!/usr/bin/env bash
# Baseline subcommand: record the real staging facts as current.json without
# deploying, migrating or switching landing content.
# shellcheck disable=SC2154
BASELINE_STAGE="preflight"
BASELINE_DB_MAX=""
BASELINE_FLOOR=""
BASELINE_ANNOTATION=""
BASELINE_API_IMAGE=""
BASELINE_WORKER_IMAGE=""
BASELINE_MIGRATE_IMAGE=""
BASELINE_LANDING_SHA=""
BASELINE_LANDING_SOURCE=""
BASELINE_SCHEMA_JOB=""
BASELINE_CHECKSUM=""

baseline_log() {
    log "baseline[$BASELINE_STAGE]: $*"
}

baseline_write_receipt() {
    local result="$1" stage="$2" message="$3"
    local created_at name tmp
    created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    if [[ "$result" == "baseline" ]]; then
        name="baseline-${created_at//[:]/}-$$.json"
    else
        name="baseline-failure-${created_at//[:]/}-$$.json"
    fi
    install -d -m 0700 "$ABASTEVO_RECEIPT_DIR"
    tmp="$(mktemp "$ABASTEVO_RECEIPT_DIR/.baseline.XXXXXX")"
    if jq -n \
        --arg schema "abastevo-deploy-baseline-v1" \
        --arg result "$result" \
        --arg stage "$stage" \
        --arg message "$message" \
        --arg created_at "$created_at" \
        --arg sha "$BASELINE_SHA" \
        --arg database_max "$BASELINE_DB_MAX" \
        --arg schema_floor "$BASELINE_FLOOR" \
        --arg schema_annotation "$BASELINE_ANNOTATION" \
        --arg api_image "$BASELINE_API_IMAGE" \
        --arg worker_image "$BASELINE_WORKER_IMAGE" \
        --arg migrate_image "$BASELINE_MIGRATE_IMAGE" \
        --arg landing_sha256 "$BASELINE_LANDING_SHA" \
        --arg landing_source "$BASELINE_LANDING_SOURCE" \
        --arg schema_job "$BASELINE_SCHEMA_JOB" \
        --arg checksum "$BASELINE_CHECKSUM" \
        '{
            schema: $schema,
            result: $result,
            stage: $stage,
            message: $message,
            created_at: $created_at,
            sha: $sha,
            database_max: $database_max,
            schema_min_supported: $schema_floor,
            schema_min_source: "database_max",
            schema_version_annotation: $schema_annotation,
            images: {
                api: $api_image,
                worker: $worker_image,
                migrate: $migrate_image
            },
            migrate_source: "release-manifest",
            landing_sha256: $landing_sha256,
            landing_sha256_source: $landing_source,
            checksum: $checksum,
            checksum_source: "health-ready",
            jobs: {
                schema: $schema_job
            }
        }' > "$tmp"; then
        chmod 0600 "$tmp"
        mv "$tmp" "$ABASTEVO_RECEIPT_DIR/$name"
        baseline_log "receipt $name"
    else
        rm -f "$tmp"
        log "could not write the baseline receipt"
    fi
}

baseline_fail() {
    local stage="$1" message="$2"
    set +e
    trap - ERR
    baseline_log "failed: $message"
    baseline_write_receipt "failed" "$stage" "$message"
    exit 1
}

baseline_trap() {
    local status="$1"
    trap - ERR
    baseline_fail "$BASELINE_STAGE" "command failed with status $status"
}

baseline_acquire_lock() {
    install -d -m 0700 "$(dirname "$ABASTEVO_LOCK_FILE")"
    exec 8>"$ABASTEVO_LOCK_FILE"
    chmod 0600 "$ABASTEVO_LOCK_FILE"
    if ! flock -n 8; then
        baseline_fail "lock" "another deploy operation is running"
    fi
}

baseline_preflight() {
    require_var KUBECONFIG
    require_var ABASTEVO_KUBECTL
    require_var ABASTEVO_DB_IMAGE
    require_var ABASTEVO_DB_SECRET
    require_var ABASTEVO_SMOKE_BASE_URL
    require_tool flock
    [[ -x "$ABASTEVO_KUBECTL" ]] || command -v "$ABASTEVO_KUBECTL" >/dev/null 2>&1 \
        || fail "ABASTEVO_KUBECTL is not executable"
    [[ "$ABASTEVO_SMOKE_BASE_URL" =~ ^https?:// ]] \
        || fail "ABASTEVO_SMOKE_BASE_URL must be an http(s) URL"
    [[ "$ABASTEVO_JOB_TIMEOUT" =~ ^[0-9]+$ ]] \
        || fail "ABASTEVO_JOB_TIMEOUT must be an integer"
    baseline_acquire_lock
}

baseline_read_images() {
    local images
    images="$(runtime_deployment_images "$ABASTEVO_NAMESPACE" "$ABASTEVO_API_DEPLOYMENT")" \
        || baseline_fail "images" "cannot read the running deployment images"
    BASELINE_API_IMAGE="$(printf '%s' "$images" \
        | jq -r --arg name "$ABASTEVO_API_CONTAINER" '.[] | select(.name == $name) | .image' \
        | head -n 1)"
    BASELINE_WORKER_IMAGE="$(printf '%s' "$images" \
        | jq -r --arg name "$ABASTEVO_WORKER_CONTAINER" '.[] | select(.name == $name) | .image' \
        | head -n 1)"
    [[ -n "$BASELINE_API_IMAGE" && -n "$BASELINE_WORKER_IMAGE" ]] \
        || baseline_fail "images" "api or worker container is missing"
    BASELINE_MIGRATE_IMAGE="$(jq -r '.images.migrate' "$manifest_file")"
    [[ "$BASELINE_MIGRATE_IMAGE" =~ ^ghcr\.io/[a-z0-9._/-]+/abastevo-migrate@sha256:[0-9a-f]{64}$ ]] \
        || baseline_fail "images" "release manifest migrate image is invalid"
    BASELINE_ANNOTATION="$(runtime_schema_annotation "$ABASTEVO_NAMESPACE" \
        "$ABASTEVO_API_DEPLOYMENT" 2>/dev/null || true)"
    baseline_log "running images captured"
}

baseline_read_schema() {
    BASELINE_SCHEMA_JOB="baseline-schema-${BASELINE_SHA:0:12}-$(date -u +%H%M%S)"
    BASELINE_DB_MAX="$(runtime_schema_version "$ABASTEVO_NAMESPACE" \
        "$BASELINE_SCHEMA_JOB" "$ABASTEVO_DB_IMAGE" "$ABASTEVO_DB_SECRET" \
        "$BASELINE_SHA" "$ABASTEVO_JOB_TIMEOUT")" \
        || baseline_fail "schema" "schema version job failed"
    [[ "$BASELINE_DB_MAX" =~ ^[0-9]{6}$ ]] \
        || baseline_fail "schema" "database schema version is unavailable"
    BASELINE_FLOOR="$BASELINE_DB_MAX"
    baseline_log "database schema $BASELINE_DB_MAX"
}

baseline_check_checksums() {
    local base="${ABASTEVO_SMOKE_BASE_URL%/}"
    curl -fsS --max-time "$ABASTEVO_SMOKE_TIMEOUT" -o /dev/null "$base/health/ready" \
        || baseline_fail "checksum" "running schema compatibility check failed"
    BASELINE_CHECKSUM="running-readiness"
    baseline_log "running schema compatibility verified"
}

baseline_read_landing() {
    if [[ -e "$ABASTEVO_LANDING_ROOT/current" ]]; then
        BASELINE_LANDING_SHA="$(cd "$ABASTEVO_LANDING_ROOT/current" \
            && find . -type f -print0 | sort -z | xargs -0 -r sha256sum \
            | sha256sum | cut -d' ' -f1)"
        BASELINE_LANDING_SOURCE="tree"
    else
        BASELINE_LANDING_SHA="$(printf '%064d' 0)"
        BASELINE_LANDING_SOURCE="absent"
    fi
    [[ "$BASELINE_LANDING_SHA" =~ ^[0-9a-f]{64}$ ]] \
        || baseline_fail "landing" "landing digest is unavailable"
    baseline_log "landing digest captured ($BASELINE_LANDING_SOURCE)"
}

baseline_write_current_state() {
    local state_dir tmp
    state_dir="$ABASTEVO_STATE_DIR"
    install -d -m 0700 "$state_dir"
    tmp="$(mktemp "$state_dir/.baseline.XXXXXX")"
    jq -n \
        --arg schema "abastevo-deploy-current-v1" \
        --arg sha "$BASELINE_SHA" \
        --arg api "$BASELINE_API_IMAGE" \
        --arg worker "$BASELINE_WORKER_IMAGE" \
        --arg migrate "$BASELINE_MIGRATE_IMAGE" \
        --arg landing_sha256 "$BASELINE_LANDING_SHA" \
        --arg migrations_max "$BASELINE_DB_MAX" \
        --arg schema_min_supported "$BASELINE_FLOOR" \
        --arg applied_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        '{
            schema: $schema,
            sha: $sha,
            images: {api: $api, worker: $worker, migrate: $migrate},
            landing_sha256: $landing_sha256,
            migrations_max: $migrations_max,
            schema_min_supported: $schema_min_supported,
            applied_at: $applied_at,
            baseline: true
        }' > "$tmp" || {
        rm -f "$tmp"
        return 1
    }
    chmod 0600 "$tmp"
    mv "$tmp" "$ABASTEVO_STATE_DIR/current.json"
}

baseline_release() {
    trap 'baseline_trap $?' ERR
    BASELINE_STAGE="preflight"
    baseline_preflight
    BASELINE_STAGE="images"
    baseline_read_images
    BASELINE_STAGE="schema"
    baseline_read_schema
    BASELINE_STAGE="checksum"
    baseline_check_checksums
    BASELINE_STAGE="landing"
    baseline_read_landing
    BASELINE_STAGE="state"
    baseline_write_current_state || baseline_fail "state" "current state write failed"
    trap - ERR
    BASELINE_STAGE="done"
    baseline_write_receipt "baseline" "done" ""
    baseline_log "current.json written"
}
