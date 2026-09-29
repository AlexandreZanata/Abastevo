#!/usr/bin/env bash
# P08-T04 isolated restore drill.
# Restores one verified backup artifact into a fresh database, verifies
# schema/counts/integrity, and leaves deletion-ledger replay to the
# operator (traffic stays closed until `ops privacy replay` runs per
# the recovery runbook). Reviewable locally against the disposable dev
# database; staging drills use the staging topology file.
#
# Commands:
#   restore --artifact <file> --target-db <name>
#     Verify the artifact (decrypt + checksums + catalog list), create
#     the target database, restore into it, and verify schema, counts
#     and referential integrity. Prints a receipt to stdout.
#   drop --target-db <name>
#     Remove a drill database (never anything else: names outside the
#     drill pattern are refused).
#
# Environment:
#   ANPFUEL_RESTORE_COMPOSE_FILE topology (default infra/compose.staging.yml)
#   ANPFUEL_RESTORE_DB_USER / _DB_NAME(admin db, default anpfuel) /
#     _DB_PASSWORD (required) for server-level CREATE/DROP DATABASE
#   ANPFUEL_RESTORE_DB       target name, required, must match
#                            ^anpfuel_drill_[0-9TZ]+$ (anything else refused)
#   ANPFUEL_BACKUP_PASSPHRASE  passed through to backup.sh verify
#   ANPFUEL_RESTORE_KEEP=1     keep the drill database for inspection
#
# Exit codes: 0 restored+verified; 1 verification failed (target kept
# for forensics unless dropped); 2 refused before any mutation.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

COMPOSE="${ANPFUEL_RESTORE_COMPOSE_FILE:-infra/compose.staging.yml}"
ADMIN_DB="${ANPFUEL_RESTORE_DB_NAME:-anpfuel}"
ADMIN_USER="${ANPFUEL_RESTORE_DB_USER:-}"
ADMIN_PASSWORD="${ANPFUEL_RESTORE_DB_PASSWORD:-}"
TARGET="${ANPFUEL_RESTORE_DB:-}"
PASSPHRASE="${ANPFUEL_BACKUP_PASSPHRASE:-}"

log() { echo "restore: $*" >&2; }
refuse() { echo "REFUSED: $*" >&2; exit 2; }

CMD="${1:-}"
shift || true

[[ -n "$ADMIN_USER" && -n "$ADMIN_PASSWORD" ]] || refuse "restore database credentials are required"

db_exec() {
    docker compose -f "$COMPOSE" exec -T -e "PGPASSWORD=$ADMIN_PASSWORD" db "$@"
}

# cleanup_target drops the drill database unless kept for forensics or
# inspection. Production names can never reach here (target pattern).
cleanup_target() {
    if [[ "${CREATED:-}" == "1" && "${ANPFUEL_RESTORE_KEEP:-}" != "1" ]]; then
        db_exec psql -h 127.0.0.1 -U "$ADMIN_USER" -d postgres -c "DROP DATABASE \"$TARGET\";" >/dev/null 2>&1 || true
    fi
}

case "$CMD" in
drop)
    while [[ $# -gt 0 ]]; do
        case "$1" in
        --target-db) TARGET="$2"; shift 2 ;;
        *) refuse "unknown flag $1" ;;
        esac
    done
    [[ "$TARGET" =~ ^anpfuel_drill_[0-9TZ]+$ ]] || refuse "refusing to drop non-drill database '$TARGET'"
    db_exec psql -h 127.0.0.1 -U "$ADMIN_USER" -d postgres -tAX -c "SELECT 1" >/dev/null 2>&1 \
        || refuse "database unreachable"
    db_exec psql -h 127.0.0.1 -U "$ADMIN_USER" -d postgres -c "DROP DATABASE \"$TARGET\";" >/dev/null 2>&1
    log "dropped $TARGET"
    ;;
restore)
    ARTIFACT=""
    while [[ $# -gt 0 ]]; do
        case "$1" in
        --artifact) ARTIFACT="$2"; shift 2 ;;
        --target-db) TARGET="$2"; shift 2 ;;
        *) refuse "unknown flag $1" ;;
        esac
    done
    [[ -n "$ARTIFACT" ]] || refuse "restore needs --artifact"
    [[ "$TARGET" =~ ^anpfuel_drill_[0-9TZ]+$ ]] || refuse "target must match ^anpfuel_drill_[0-9TZ]+ (got '$TARGET')"
    [[ "$COMPOSE" == *"compose.dev.yml" && "${ANPFUEL_BACKUP_ALLOW_DEV:-}" != "1" ]] \
        && refuse "dev topology needs ANPFUEL_BACKUP_ALLOW_DEV=1 (never in production)"

    # 1. Artifact verification first: wrong/corrupt artifacts never
    # create anything.
    log "verifying artifact"
    if ! ANPFUEL_BACKUP_PASSPHRASE="$PASSPHRASE" bash infra/scripts/backup.sh verify "$ARTIFACT" >/dev/null 2>&1; then
        echo "REFUSED: artifact verification failed; nothing created" >&2
        exit 1
    fi

    # 2. Fresh target database (fails closed when it already exists:
    # restores never merge into existing data).
    log "creating target $TARGET"
    if ! db_exec psql -h 127.0.0.1 -U "$ADMIN_USER" -d postgres -c "CREATE DATABASE \"$TARGET\";" >/dev/null 2>&1; then
        echo "REFUSED: cannot create $TARGET (exists?)" >&2
        exit 1
    fi
    CREATED=1
    trap cleanup_target EXIT

    # 3. Restore through the container (pg_restore reads a container path).
    container="$(docker compose -f "$COMPOSE" ps -q db | head -n 1)"
    [[ -n "$container" ]] || { echo "REFUSED: database container not running" >&2; exit 1; }
    plain="$(mktemp)"
    trap 'rm -f "$plain"; cleanup_target' EXIT
    printf '%s' "$PASSPHRASE" | openssl enc -d -aes-256-cbc -pbkdf2 -pass stdin -in "$ARTIFACT" -out "$plain" 2>/dev/null
    docker cp "$plain" "$container:/tmp/restore.dump" >/dev/null 2>&1
    rm -f "$plain"
    if ! db_exec pg_restore -h 127.0.0.1 -U "$ADMIN_USER" -d "$TARGET" --no-owner /tmp/restore.dump >/dev/null 2>&1; then
        echo "REFUSED: pg_restore failed; target kept for forensics (drop with restore.sh drop)" >&2
        CREATED=""
        exit 1
    fi
    db_exec rm -f /tmp/restore.dump >/dev/null 2>&1 || true

    # 4. Structural verification: migrations ledger, extensions, and
    # per-table counts plus evidence join integrity.
    q() { db_exec psql -h 127.0.0.1 -U "$ADMIN_USER" -d "$TARGET" -tAX -c "$1" 2>/dev/null; }
    applied="$(q "SELECT COUNT(*) FROM schema_migrations;" || true)"
    if [[ -z "$applied" || "$applied" -le 0 ]]; then echo "REFUSED: migration ledger unreadable" >&2; CREATED=""; exit 1; fi
    postgis="$(q "SELECT extversion FROM pg_extension WHERE extname = 'postgis';" || true)"
    if [[ -z "$postgis" ]]; then echo "REFUSED: postgis missing after restore" >&2; CREATED=""; exit 1; fi
    counts=""
    for t in community_observations community_confirmations community_disputes evidence_sessions evidence_objects trust_decisions trust_current privacy_requests privacy_deletion_ledger moderation_cases moderation_actions identity_contributors; do
        n="$(q "SELECT COUNT(*) FROM $t;" || true)"
        if [[ -z "$n" ]]; then echo "REFUSED: table $t unreadable" >&2; CREATED=""; exit 1; fi
        counts="$counts $t=$n"
    done
    orphans="$(q "SELECT COUNT(*) FROM evidence_objects o LEFT JOIN evidence_sessions s ON s.id = o.session_id WHERE s.id IS NULL;" || true)"
    if [[ "$orphans" != "0" ]]; then
        echo "REFUSED: $orphans orphaned evidence objects" >&2
        CREATED=""
        exit 1
    fi
    ledgers="$(q "SELECT COUNT(*) FROM privacy_deletion_ledger WHERE replayed_at IS NULL;" || true)"
    CREATED=""
    if [[ "${ANPFUEL_RESTORE_KEEP:-}" != "1" ]]; then
        log "checks passed; dropping drill database (set ANPFUEL_RESTORE_KEEP=1 to inspect)"
        db_exec psql -h 127.0.0.1 -U "$ADMIN_USER" -d postgres -c "DROP DATABASE \"$TARGET\";" >/dev/null 2>&1
        echo "RESULT=restored-verified TARGET=$TARGET MIGRATIONS=$applied POSTGIS=$postgis COUNTS:$counts ORPHANS=0 PENDING_REPLAY=$ledgers DROPPED=yes"
    else
        echo "RESULT=restored-verified TARGET=$TARGET MIGRATIONS=$applied POSTGIS=$postgis COUNTS:$counts ORPHANS=0 PENDING_REPLAY=$ledgers DROPPED=no"
    fi
    ;;
*)
    echo "usage: restore.sh <restore --artifact <file> --target-db anpfuel_drill_<ts>|drop --target-db <name>>" >&2
    exit 2
    ;;
esac
