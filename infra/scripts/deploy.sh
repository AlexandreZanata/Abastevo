#!/usr/bin/env bash
# P08-T02 release deploy and rollback orchestrator.
# One versioned monolith release across migrate -> api/worker/caddy with
# readiness and smoke gates; any gate failure returns to the previous
# compatible release (no destructive down migration, ever). Review and
# staging-drill artifact: production use additionally requires G09.
#
# Required environment:
#   ANPFUEL_RELEASE          immutable tag for this rollout (never latest)
#   ANPFUEL_PREVIOUS_RELEASE last known-good tag, or "none" for a fresh env
#   ANPFUEL_ENV_FILE         operator-owned env file (never committed)
#   ANPFUEL_COMPOSE_FILE     compose topology (default infra/compose.staging.yml)
# Optional:
#   ANPFUEL_REVISION         source revision label (default: clean HEAD SHA)
#   ANPFUEL_SMOKE_BASE       smoke origin, e.g. http://127.0.0.1 (default)
#   ANPFUEL_BACKUP_MANIFEST  fresh backup manifest for the precheck; when
#                            unset the precheck refuses unless ANPFUEL_ALLOW_NO_BACKUP=1
#   ANPFUEL_RECEIPT_FILE     path to append the release receipt (no secrets)
#   ANPFUEL_READINESS_TIMEOUT_SECONDS (default 120), ANPFUEL_NO_BUILD=1 to
#                            skip image builds (prebuilt/digest-pinned hosts)
#
# Exit codes: 0 deployed+verified; 1 rolled back after a gate failure;
# 2 refused before any mutation. stdout carries the receipt; stderr the log.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

COMPOSE="${ANPFUEL_COMPOSE_FILE:-infra/compose.staging.yml}"
RELEASE="${ANPFUEL_RELEASE:-}"
PREVIOUS="${ANPFUEL_PREVIOUS_RELEASE:-}"
if [[ -z "$RELEASE" ]]; then echo "REFUSED: ANPFUEL_RELEASE is required (never latest)" >&2; exit 2; fi
if [[ -z "$PREVIOUS" ]]; then echo "REFUSED: ANPFUEL_PREVIOUS_RELEASE is required (or \"none\")" >&2; exit 2; fi
[[ "$RELEASE" != "latest" ]] || { echo "REFUSED: floating release tag" >&2; exit 2; }
[[ -n "${ANPFUEL_ENV_FILE:-}" ]] || { echo "REFUSED: ANPFUEL_ENV_FILE is required" >&2; exit 2; }
export ANPFUEL_ENV_FILE ANPFUEL_RELEASE

REVISION="${ANPFUEL_REVISION:-}"
if [[ -z "$REVISION" ]]; then
    if ! git diff --quiet || [[ -n "$(git status --short --untracked-files=no)" ]]; then
        echo "REFUSED: dirty tree cannot produce a reproducible release (set ANPFUEL_REVISION explicitly to override)" >&2
        exit 2
    fi
    REVISION="$(git rev-parse --short HEAD)"
fi
SMOKE="${ANPFUEL_SMOKE_BASE:-http://127.0.0.1}"
READY_TIMEOUT="${ANPFUEL_READINESS_TIMEOUT_SECONDS:-120}"
RECEIPT="${ANPFUEL_RECEIPT_FILE:-}"

log() { echo "deploy: $*" >&2; }
receipt() {
    local line="$1"
    echo "$line"
    [[ -z "$RECEIPT" ]] || echo "$line" >>"$RECEIPT"
}

rolled_back=0
rollback_to_previous() {
    local reason="$1"
    log "gate failed ($reason); rolling back"
    if [[ "$PREVIOUS" == "none" ]]; then
        log "no previous release: stopping rollout without rollback target"
        return 1
    fi
    ANPFUEL_RELEASE="$PREVIOUS" \
        docker compose -f "$COMPOSE" up -d api worker caddy >/dev/null 2>&1
    if ! wait_ready "$PREVIOUS"; then
        log "rollback release $PREVIOUS also unhealthy; manual recovery per runbook"
        return 1
    fi
    rolled_back=1
    receipt "RESULT=rolled-back REASON=$reason RELEASE=$RELEASE PREVIOUS=$PREVIOUS REVISION=$REVISION"
    return 0
}

# fail_gate rolls back when possible, reports honestly when rollback
# itself fails, and always exits 1 (a gate failed either way).
fail_gate() {
    local reason="$1"
    if rollback_to_previous "$reason"; then
        log "rolled back to $PREVIOUS"
    else
        receipt "RESULT=rollback-failed REASON=$reason RELEASE=$RELEASE PREVIOUS=$PREVIOUS REVISION=$REVISION"
    fi
    exit 1
}

wait_ready() {
    local tag="$1"
    local deadline=$((SECONDS + READY_TIMEOUT))
    while ((SECONDS < deadline)); do
        if curl -sS -o /dev/null --max-time 10 "$SMOKE/health/ready" >/dev/null 2>&1; then
            log "readiness ok ($tag)"
            return 0
        fi
        sleep 5
    done
    return 1
}

smoke() {
    local code
    code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 15 "$SMOKE/health/live")"
    [[ "$code" == "200" ]] || return 1
    code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 15 "$SMOKE/v1/stations?limit=1")"
    [[ "$code" == "200" ]] || return 1
    return 0
}

# --- prechecks (no mutations below this line may precede them) ---
[[ -f "$COMPOSE" ]] || { echo "REFUSED: compose file missing" >&2; exit 2; }
if ! docker compose -f "$COMPOSE" config --quiet 2>/dev/null; then
    echo "REFUSED: compose config does not parse" >&2
    exit 2
fi
if [[ -n "${ANPFUEL_BACKUP_MANIFEST:-}" ]]; then
    [[ -f "$ANPFUEL_BACKUP_MANIFEST" ]] || { echo "REFUSED: backup manifest missing" >&2; exit 2; }
elif [[ "${ANPFUEL_ALLOW_NO_BACKUP:-}" != "1" ]]; then
    echo "REFUSED: no backup manifest (set ANPFUEL_BACKUP_MANIFEST or ANPFUEL_ALLOW_NO_BACKUP=1 with a recorded reason)" >&2
    exit 2
fi

log "release=$RELEASE previous=$PREVIOUS revision=$REVISION compose=$COMPOSE"

# --- build (skippable on digest-pinned hosts) ---
if [[ "${ANPFUEL_NO_BUILD:-}" != "1" ]]; then
    for target in api worker migrate; do
        log "building $target"
        if ! docker build -f infra/docker/Dockerfile --target "$target" \
            --build-arg REVISION="$REVISION" -t "anpfuel-$target:$RELEASE" . >/dev/null 2>&1; then
            echo "REFUSED: image build failed for $target" >&2
            exit 2
        fi
    done
fi

# --- migrate (lock/checksum ledger inside the migrator) ---
log "running one-shot migration"
if ! docker compose -f "$COMPOSE" run --rm migrate >/dev/null 2>&1; then
    fail_gate "migration"
fi

# --- rollout ---
log "rolling out $RELEASE"
if ! docker compose -f "$COMPOSE" up -d api worker caddy >/dev/null 2>&1; then
    fail_gate "rollout"
fi

# --- readiness gate ---
if ! wait_ready "$RELEASE"; then
    fail_gate "readiness"
fi

# --- smoke gate ---
if ! smoke; then
    fail_gate "smoke"
fi

receipt "RESULT=deployed RELEASE=$RELEASE PREVIOUS=$PREVIOUS REVISION=$REVISION"
log "deployed and verified: $RELEASE"
