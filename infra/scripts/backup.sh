#!/usr/bin/env bash
# P08-T03 encrypted off-host backup pipeline.
# Daily encrypted logical dump with roles/extensions manifest and
# SHA-256 verification; 7 daily + 4 weekly copies; stale-age alarm.
# Reviewable locally against the disposable dev database; production
# use additionally requires G09 plus provisioned transport.
#
# Commands:
#   run                 create backup + manifest, prune old copies
#   verify <artifact> [manifest]  decrypt and checksum-verify (plus catalog list)
#   check-age           fail when the newest manifest exceeds the max age
#
# Environment:
#   ANPFUEL_BACKUP_COMPOSE_FILE  topology (default infra/compose.staging.yml)
#   ANPFUEL_BACKUP_DB_USER / _DB_NAME / _DB_PASSWORD (required)
#   ANPFUEL_BACKUP_OUTBOX        destination dir (required, synced off-host
#                                by operator transport: rsync/rclone/S3)
#   ANPFUEL_BACKUP_PASSPHRASE    encryption secret from env (never a CLI arg,
#                                never committed, kept outside the VPS for recovery)
#   ANPFUEL_BACKUP_KEEP_DAILY=7 ANPFUEL_BACKUP_KEEP_WEEKLY=4
#   ANPFUEL_BACKUP_MAX_AGE_HOURS=26 (stale alarm for monitoring)
#   ANPFUEL_BACKUP_ALLOW_DEV=1   required to target compose.dev.yml
#
# The manifest carries names and hashes only: no secrets, safe to store
# next to the artifact. Recovery needs only the passphrase (outside the
# VPS) plus any PostgreSQL 18 + PostGIS host.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

COMPOSE="${ANPFUEL_BACKUP_COMPOSE_FILE:-infra/compose.staging.yml}"
OUTBOX="${ANPFUEL_BACKUP_OUTBOX:-}"
DB_USER="${ANPFUEL_BACKUP_DB_USER:-}"
DB_NAME="${ANPFUEL_BACKUP_DB_NAME:-}"
DB_PASSWORD="${ANPFUEL_BACKUP_DB_PASSWORD:-}"
PASSPHRASE="${ANPFUEL_BACKUP_PASSPHRASE:-}"
KEEP_DAILY="${ANPFUEL_BACKUP_KEEP_DAILY:-7}"
KEEP_WEEKLY="${ANPFUEL_BACKUP_KEEP_WEEKLY:-4}"
MAX_AGE_HOURS="${ANPFUEL_BACKUP_MAX_AGE_HOURS:-26}"

log() { echo "backup: $*" >&2; }
refuse() { echo "REFUSED: $*" >&2; exit 2; }

CMD="${1:-}"
ARTIFACT="${2:-}"
MANIFEST_ARG="${3:-}"

[[ -n "$OUTBOX" ]] || refuse "ANPFUEL_BACKUP_OUTBOX is required"
[[ -n "$DB_USER" && -n "$DB_NAME" && -n "$DB_PASSWORD" ]] || refuse "backup database credentials are required"
if [[ "$COMPOSE" == *"compose.dev.yml" && "${ANPFUEL_BACKUP_ALLOW_DEV:-}" != "1" ]]; then
    refuse "dev topology needs ANPFUEL_BACKUP_ALLOW_DEV=1 (never in production)"
fi

db_exec() {
    # Password travels as container environment for TCP auth; it never
    # lands in committed files (operator env only).
    docker compose -f "$COMPOSE" exec -T -e "PGPASSWORD=$DB_PASSWORD" db "$@"
}

db_container() {
    docker compose -f "$COMPOSE" ps -q db | head -n 1
}

cmd_run() {
    [[ -n "$PASSPHRASE" ]] || refuse "ANPFUEL_BACKUP_PASSPHRASE is required"
    mkdir -p "$OUTBOX"
    local ts base dump enc manifest roles exts
    ts="$(date -u +%Y%m%dT%H%M%SZ)"
    base="anpfuel-$ts"
    dump="$OUTBOX/$base.dump"
    enc="$dump.enc"
    manifest="$OUTBOX/$base.sha256.json"
    log "dumping $DB_NAME"
    if ! db_exec pg_dump -h 127.0.0.1 -U "$DB_USER" -d "$DB_NAME" -Fc >"$dump" 2>/dev/null; then
        rm -f "$dump"
        echo "REFUSED: pg_dump failed (check credentials and database)" >&2
        exit 1
    fi
    roles="$(db_exec pg_dumpall -h 127.0.0.1 -r -U "$DB_USER" 2>/dev/null | sha256sum | cut -d' ' -f1)" || {
        rm -f "$dump"
        echo "REFUSED: roles dump failed" >&2
        exit 1
    }
    exts="$(db_exec psql -h 127.0.0.1 -U "$DB_USER" -d "$DB_NAME" -tAX -c "SELECT extname || ' ' || extversion FROM pg_extension ORDER BY 1;" 2>/dev/null | tr '\n' ';')" || {
        rm -f "$dump"
        echo "REFUSED: extensions inventory failed" >&2
        exit 1
    }
    local dump_sha
    dump_sha="$(sha256sum "$dump" | cut -d' ' -f1)"
    if ! printf '%s' "$PASSPHRASE" | openssl enc -aes-256-cbc -pbkdf2 -pass stdin -in "$dump" -out "$enc" 2>/dev/null; then
        rm -f "$dump" "$enc"
        echo "REFUSED: encryption failed" >&2
        exit 1
    fi
    rm -f "$dump"
    local enc_sha
    enc_sha="$(sha256sum "$enc" | cut -d' ' -f1)"
    printf '{"created_at":"%s","db":"%s","artifact":"%s","dump_sha256":"%s","roles_sha256":"%s","extensions":"%s","encrypted_sha256":"%s"}\n' \
        "$ts" "$DB_NAME" "$(basename "$enc")" "$dump_sha" "$roles" "$exts" "$enc_sha" >"$manifest"
    log "wrote $enc"
    cmd_prune
    echo "$manifest"
}

cmd_verify() {
    [[ -n "$ARTIFACT" ]] || refuse "verify needs an artifact path"
    [[ -f "$ARTIFACT" ]] || refuse "artifact missing: $ARTIFACT"
    [[ -n "$PASSPHRASE" ]] || refuse "ANPFUEL_BACKUP_PASSPHRASE is required"
    local manifest="$MANIFEST_ARG"
    if [[ -z "$manifest" ]]; then
        manifest="${ARTIFACT%.dump.enc}.sha256.json"
    fi
    [[ -f "$manifest" ]] || refuse "manifest missing for $ARTIFACT"
    local want_enc want_dump
    want_enc="$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['encrypted_sha256'])" "$manifest")"
    want_dump="$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['dump_sha256'])" "$manifest")"
    local got_enc
    got_enc="$(sha256sum "$ARTIFACT" | cut -d' ' -f1)"
    if [[ "$got_enc" != "$want_enc" ]]; then
        echo "REFUSED: artifact checksum mismatch (transport corruption?)" >&2
        exit 1
    fi
    local tmp
    tmp="$(mktemp)"
    # Expand now: function-locals vanish before EXIT fires (set -u).
    trap "rm -f '$tmp'" EXIT
    if ! printf '%s' "$PASSPHRASE" | openssl enc -d -aes-256-cbc -pbkdf2 -pass stdin -in "$ARTIFACT" -out "$tmp" 2>/dev/null; then
        echo "REFUSED: decryption failed (wrong passphrase?)" >&2
        exit 1
    fi
    local got_dump
    got_dump="$(sha256sum "$tmp" | cut -d' ' -f1)"
    if [[ "$got_dump" != "$want_dump" ]]; then
        echo "REFUSED: decrypted checksum mismatch" >&2
        exit 1
    fi
    # Catalog proof, not just bytes: the archive must list restorable
    # objects (snapshot-only dumps are insufficient assurance).
    local container
    container="$(db_container)"
    [[ -n "$container" ]] || { echo "REFUSED: database container not running" >&2; exit 1; }
    docker cp "$tmp" "$container:/tmp/verify.dump" >/dev/null 2>&1
    if ! docker exec "$container" pg_restore --list /tmp/verify.dump >/dev/null 2>&1; then
        docker exec "$container" rm -f /tmp/verify.dump >/dev/null 2>&1 || true
        echo "REFUSED: archive catalog unreadable" >&2
        exit 1
    fi
    docker exec "$container" rm -f /tmp/verify.dump >/dev/null 2>&1 || true
    echo "verify ok: $ARTIFACT"
}

cmd_prune() {
    # Keep newest KEEP_DAILY daily copies plus KEEP_WEEKLY Sunday copies;
    # drop anything older than 35 days regardless (backup horizon).
    local f kept_daily=0 kept_weekly=0
    while IFS= read -r f; do
        [[ -n "$f" ]] || continue
        local name age_days dow
        name="$(basename "$f")"
        if ! age_days="$(file_age_days "$OUTBOX/$f")" || ! dow="$(file_dow "$OUTBOX/$f")"; then
            continue
        fi
        if ((age_days > 35)); then
            rm -f "$OUTBOX/$f" "$OUTBOX/${f%.dump.enc}.sha256.json"
            continue
        fi
        if ((kept_daily < KEEP_DAILY)); then
            kept_daily=$((kept_daily + 1))
            continue
        fi
        if [[ "$dow" == "Sun" && "$kept_weekly" -lt "$KEEP_WEEKLY" ]]; then
            kept_weekly=$((kept_weekly + 1))
            continue
        fi
        rm -f "$OUTBOX/$f" "$OUTBOX/${f%.dump.enc}.sha256.json"
    done < <(cd "$OUTBOX" && ls -1 anpfuel-*.dump.enc 2>/dev/null | sort -r)
}

file_age_days() {
    local mtime now
    mtime="$(stat -c %Y "$1" 2>/dev/null)" || return 1
    now="$(date +%s)"
    echo $(((now - mtime) / 86400))
}

file_dow() {
    date -d "@$(stat -c %Y "$1" 2>/dev/null)" +%a 2>/dev/null || return 1
}

cmd_check_age() {
    local newest
    newest="$(ls -1t "$OUTBOX"/anpfuel-*.sha256.json 2>/dev/null | head -n 1 || true)"
    [[ -n "$newest" ]] || { echo "REFUSED: no backup manifest in $OUTBOX" >&2; exit 1; }
    local age_hours
    age_hours=$((($(date +%s) - $(stat -c %Y "$newest")) / 3600))
    if ((age_hours > MAX_AGE_HOURS)); then
        echo "REFUSED: newest backup is ${age_hours}h old (max ${MAX_AGE_HOURS}h)" >&2
        exit 1
    fi
    echo "backup age ok: ${age_hours}h <= ${MAX_AGE_HOURS}h ($newest)"
}

case "$CMD" in
run) cmd_run ;;
verify) cmd_verify ;;
prune) mkdir -p "$OUTBOX"; cmd_prune ;;
check-age) cmd_check_age ;;
*) echo "usage: backup.sh <run|verify <artifact> [manifest]|prune|check-age>" >&2; exit 2 ;;
esac
