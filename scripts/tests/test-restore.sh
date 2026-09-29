#!/usr/bin/env bash
# P08-T04 harness: isolated restore drill on the disposable dev database.
# Backs up the live dev content, restores into a fresh drill database,
# verifies counts/integrity, replays the deletion ledger with the real
# ops binary, and proves wrong artifacts are refused. Nothing
# provisioned; drill databases always drop at the end.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1" >&2; FAIL=$((FAIL + 1)); }

CAP_OUT=""
CAP_CODE=0
capture() {
    set +e
    CAP_OUT="$("$@")"
    CAP_CODE=$?
    set -e
}

OUTBOX="$(mktemp -d)"
WORK="$(mktemp -d)"
# shellcheck disable=SC2064
trap "rm -rf '$OUTBOX' '$WORK'" EXIT

COMPOSE=infra/compose.dev.yml
DRILL="anpfuel_drill_$(date -u +%Y%m%dT%H%M%SZ)"
PASSPHRASE="drill-passphrase-value"

drill_env() {
    env -i PATH="/usr/bin:/bin" HOME="$HOME" \
        ANPFUEL_BACKUP_COMPOSE_FILE="$COMPOSE" \
        ANPFUEL_BACKUP_DB_USER=anpfuel \
        ANPFUEL_BACKUP_DB_NAME=anpfuel \
        ANPFUEL_BACKUP_DB_PASSWORD=anpfuel \
        ANPFUEL_BACKUP_OUTBOX="$OUTBOX" \
        ANPFUEL_BACKUP_PASSPHRASE="$PASSPHRASE" \
        ANPFUEL_BACKUP_ALLOW_DEV=1 \
        ANPFUEL_RESTORE_COMPOSE_FILE="$COMPOSE" \
        ANPFUEL_RESTORE_DB_USER=anpfuel \
        ANPFUEL_RESTORE_DB_NAME=anpfuel \
        ANPFUEL_RESTORE_DB_PASSWORD=anpfuel \
        ANPFUEL_RESTORE_KEEP=1 \
        "$@"
}

psql_dev() {
    docker compose -f "$COMPOSE" exec -T -e PGPASSWORD=anpfuel db \
        psql -h 127.0.0.1 -U anpfuel -d "$1" -tAX -c "$2" 2>/dev/null
}

# Drill fixture IDs are namespaced (9999999/tok-drill); scrub first so
# reruns after an aborted drill converge instead of colliding.
scrub_fixtures() {
    psql_dev anpfuel "DELETE FROM community_observations WHERE id = 'b9999999-0000-4000-8000-000000000001'" >/dev/null || true
    psql_dev anpfuel "DELETE FROM trust_current WHERE contributor_ref = 'tok-drill'" >/dev/null || true
    psql_dev anpfuel "DELETE FROM privacy_deletion_ledger WHERE contributor_id LIKE 'c999999%'" >/dev/null || true
    psql_dev anpfuel "DELETE FROM identity_contributors WHERE id::text LIKE 'c999999%'" >/dev/null || true
    psql_dev anpfuel "DELETE FROM directory_stations WHERE id = 'd9999999-0000-4000-8000-000000000001'" >/dev/null || true
}

scrub_fixtures

# 1. Fixtures modeling post-stale-restore state: live rows behind an
# owner reference with a pending deletion ledger, plus one cleanly
# erased contributor that must survive untouched.
psql_dev anpfuel "INSERT INTO identity_contributors (id, status, attribution_token) VALUES ('c9999999-0000-4000-8000-000000000001', 'active', 'tok-drill')" >/dev/null
psql_dev anpfuel "INSERT INTO directory_stations (id, display_name) VALUES ('d9999999-0000-4000-8000-000000000001', 'Drill Station')" >/dev/null
psql_dev anpfuel "INSERT INTO community_observations (id, contributor_ref, client_submission_id, station_id, fuel_product, unit, amount_milli_brl, condition_kind, qualifier_key, policy_version) VALUES ('b9999999-0000-4000-8000-000000000001', 'tok-drill', 'drill-obs', 'd9999999-0000-4000-8000-000000000001', 'GASOLINE_REGULAR', 'L', 6000, 'STANDARD', 'STANDARD', 'community-v1')" >/dev/null
psql_dev anpfuel "INSERT INTO trust_current (contributor_ref, tier) VALUES ('tok-drill', 'ESTABLISHED')" >/dev/null
for scope in IDENTITY COMMUNITY EVIDENCE TRUST EXPORTS; do
    psql_dev anpfuel "INSERT INTO privacy_deletion_ledger (id, contributor_id, contributor_ref, scope, reason, policy_version) VALUES (gen_random_uuid(), 'c9999999-0000-4000-8000-000000000001', 'tok-drill', '$scope', 'drill', 'privacy-v1')" >/dev/null
done
psql_dev anpfuel "INSERT INTO identity_contributors (id, status) VALUES ('c9999998-0000-4000-8000-000000000001', 'deleted')" >/dev/null

TABLES="community_observations community_confirmations community_disputes evidence_sessions evidence_objects trust_decisions trust_current privacy_requests privacy_deletion_ledger moderation_cases moderation_actions identity_contributors"
SOURCE_COUNTS=""
for t in $TABLES; do
    SOURCE_COUNTS="$SOURCE_COUNTS $t=$(psql_dev anpfuel "SELECT COUNT(*) FROM $t;")"
done

# 2. Backup the source.
MANIFEST="$(drill_env bash infra/scripts/backup.sh run)"
if [[ -f "$MANIFEST" ]]; then
    pass "drill backup created"
else
    fail "drill backup created"
fi
ARTIFACT="${MANIFEST%.sha256.json}.dump.enc"

# 3. Wrong artifact refuses before creating anything.
cp "$ARTIFACT" "$WORK/corrupt.dump.enc"
cp "$MANIFEST" "$WORK/corrupt.sha256.json"
printf 'X' | dd of="$WORK/corrupt.dump.enc" bs=1 seek=100 conv=notrunc status=none
capture drill_env bash infra/scripts/restore.sh restore --artifact "$WORK/corrupt.dump.enc" --target-db "$DRILL"
if [[ "$CAP_CODE" -ne 0 ]] \
    && [[ "$(psql_dev anpfuel "SELECT COUNT(*) FROM pg_database WHERE datname = '$DRILL';")" == "0" ]]; then
    pass "corrupt artifact refused without creating"
else
    fail "corrupt artifact refused without creating (code=$CAP_CODE)"
fi

# 4. Restore into the isolated drill database.
capture drill_env bash infra/scripts/restore.sh restore --artifact "$ARTIFACT" --target-db "$DRILL"
if [[ "$CAP_CODE" -eq 0 && "$CAP_OUT" == RESULT=restored-verified* ]] \
    && [[ "$CAP_OUT" == *"ORPHANS=0"* ]]; then
    pass "restore verified with integrity"
else
    fail "restore verified with integrity (code=$CAP_CODE out=$CAP_OUT)"
fi
MISMATCH=0
for t in $TABLES; do
    want=" $t=$(psql_dev anpfuel "SELECT COUNT(*) FROM $t;")"
    got=" $t=$(psql_dev "$DRILL" "SELECT COUNT(*) FROM $t;" 2>/dev/null || echo MISSING)"
    if [[ "$want" != "$got" ]]; then
        MISMATCH=1
    fi
done
if [[ "$MISMATCH" -eq 0 ]]; then
    pass "restored counts match source"
else
    fail "restored counts match source"
fi

# 5. Replay the ledger on the restored snapshot with the real binary.
(
    cd backend
    GOTOOLCHAIN=go1.27.1 go build -o "$WORK/ops" ./cmd/ops
)
capture env -i PATH="/usr/bin:/bin" HOME="$HOME" \
    ANPFUEL_DATABASE_URL="postgres://anpfuel:anpfuel@127.0.0.1:5434/$DRILL?sslmode=disable" \
    ANPFUEL_OPERATOR_ID=drill-op \
    "$WORK/ops" privacy replay --contributor c9999999-0000-4000-8000-000000000001
if [[ "$CAP_CODE" -eq 0 ]]; then
    pass "ledger replayed on restored snapshot"
else
    fail "ledger replayed on restored snapshot (code=$CAP_CODE out=$CAP_OUT)"
fi
if [[ "$(psql_dev "$DRILL" "SELECT COUNT(*) FROM community_observations WHERE contributor_ref = 'tok-drill';")" == "0" ]] \
    && [[ "$(psql_dev "$DRILL" "SELECT COUNT(*) FROM trust_current WHERE contributor_ref = 'tok-drill';")" == "0" ]] \
    && [[ "$(psql_dev "$DRILL" "SELECT COUNT(*) FROM privacy_deletion_ledger WHERE contributor_id = 'c9999999-0000-4000-8000-000000000001' AND replayed_at IS NULL;")" == "0" ]]; then
    pass "deleted data stays removed after replay"
else
    fail "deleted data stays removed after replay"
fi
# The cleanly erased contributor survived the whole drill untouched.
if [[ "$(psql_dev "$DRILL" "SELECT status FROM identity_contributors WHERE id = 'c9999998-0000-4000-8000-000000000001';")" == "deleted" ]]; then
    pass "erased state preserved through drill"
else
    fail "erased state preserved through drill"
fi

# 6. Drop the drill database (never anything else: pattern-guarded).
capture drill_env bash infra/scripts/restore.sh drop --target-db "$DRILL"
if [[ "$CAP_CODE" -eq 0 ]] \
    && [[ "$(psql_dev anpfuel "SELECT COUNT(*) FROM pg_database WHERE datname = '$DRILL';")" == "0" ]]; then
    pass "drill database dropped"
else
    fail "drill database dropped (code=$CAP_CODE)"
fi
capture drill_env bash infra/scripts/restore.sh drop --target-db anpfuel
if [[ "$CAP_CODE" -eq 2 ]]; then
    pass "non-drill drop refused"
else
    fail "non-drill drop refused (code=$CAP_CODE)"
fi

# 7. Scrub drill fixtures from the disposable source.
scrub_fixtures

echo "restore-drill: $PASS passed, $FAIL failed"
[[ "$FAIL" -eq 0 ]]
