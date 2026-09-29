#!/usr/bin/env bash
# P08-T03 harness: backup pipeline against the disposable dev database.
# Proves encrypted backup + manifest creation, checksum verification,
# corruption/passphrase/credential refusals, stale-age alarm and
# retention pruning with real pg_dump/openssl. Nothing provisioned.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1" >&2; FAIL=$((FAIL + 1)); }

# Captures a possibly-failing command without tripping set -e.
CAP_OUT=""
CAP_CODE=0
capture() {
    set +e
    CAP_OUT="$("$@")"
    CAP_CODE=$?
    set -e
}

OUTBOX="$(mktemp -d)"
trap 'rm -rf "$OUTBOX"' EXIT

backup_env() {
    env -i PATH="/usr/bin:/bin" HOME="$HOME" \
        ANPFUEL_BACKUP_COMPOSE_FILE=infra/compose.dev.yml \
        ANPFUEL_BACKUP_DB_USER="${BACKUP_DB_USER-anpfuel}" \
        ANPFUEL_BACKUP_DB_NAME=anpfuel \
        ANPFUEL_BACKUP_DB_PASSWORD="${BACKUP_DB_PASSWORD-anpfuel}" \
        ANPFUEL_BACKUP_OUTBOX="$OUTBOX" \
        ANPFUEL_BACKUP_PASSPHRASE="${BACKUP_PASSPHRASE-harness-passphrase-value}" \
        ANPFUEL_BACKUP_ALLOW_DEV=1 \
        "$@"
}

# 1. Run creates an encrypted artifact plus manifest.
MANIFEST="$(backup_env bash infra/scripts/backup.sh run)"
if [[ -f "$MANIFEST" ]]; then
    pass "backup run writes manifest"
else
    fail "backup run writes manifest (out=$MANIFEST)"
fi
ARTIFACT="${MANIFEST%.sha256.json}.dump.enc"
if [[ -f "$ARTIFACT" ]]; then
    pass "backup run writes encrypted artifact"
else
    fail "backup run writes encrypted artifact"
fi

# 2. Manifest carries hashes, never the passphrase.
if python3 -c "import json,sys; m=json.load(open(sys.argv[1])); assert m['dump_sha256'] and m['encrypted_sha256'] and m['extensions']" "$MANIFEST" \
    && ! grep -q "harness-passphrase-value" "$MANIFEST"; then
    pass "manifest verifiable and secret-free"
else
    fail "manifest verifiable and secret-free"
fi

# 3. Verify accepts the artifact (checksums plus catalog list).
capture backup_env bash infra/scripts/backup.sh verify "$ARTIFACT"
if [[ "$CAP_CODE" -eq 0 && "$CAP_OUT" == "verify ok:"* ]]; then
    pass "verify accepts artifact"
else
    fail "verify accepts artifact (code=$CAP_CODE out=$CAP_OUT)"
fi

# 4. Corrupted bytes refuse (RED: transport corruption detected).
cp "$ARTIFACT" "$OUTBOX/corrupt.dump.enc"
cp "$MANIFEST" "$OUTBOX/corrupt.sha256.json"
printf 'X' | dd of="$OUTBOX/corrupt.dump.enc" bs=1 seek=100 conv=notrunc status=none
capture backup_env bash infra/scripts/backup.sh verify "$OUTBOX/corrupt.dump.enc" "$OUTBOX/corrupt.sha256.json"
if [[ "$CAP_CODE" -ne 0 ]]; then
    pass "corrupted artifact refused"
else
    fail "corrupted artifact refused"
fi
rm -f "$OUTBOX/corrupt.dump.enc" "$OUTBOX/corrupt.sha256.json"

# 5. Wrong passphrase refuses.
BACKUP_PASSPHRASE=wrong-passphrase capture backup_env bash infra/scripts/backup.sh verify "$ARTIFACT"
if [[ "$CAP_CODE" -ne 0 ]]; then
    pass "wrong passphrase refused"
else
    fail "wrong passphrase refused"
fi

# 6. Wrong database credential refuses the run (expired-credential class).
BACKUP_DB_USER=ghost-role capture backup_env bash infra/scripts/backup.sh run
if [[ "$CAP_CODE" -ne 0 ]]; then
    pass "bad credential refused"
else
    fail "bad credential refused"
fi

# 7. Stale alarm: fresh manifest passes, 30 h-old manifest fails.
capture backup_env bash infra/scripts/backup.sh check-age
if [[ "$CAP_CODE" -eq 0 ]]; then
    pass "fresh backup passes age check"
else
    fail "fresh backup passes age check"
fi
touch -d '30 hours ago' "$OUTBOX"/*.sha256.json
capture backup_env bash infra/scripts/backup.sh check-age
if [[ "$CAP_CODE" -ne 0 ]]; then
    pass "stale backup alarms"
else
    fail "stale backup alarms"
fi

# 8. Retention pruning: 40-day-old copies drop, recent keep.
for d in 20200101 20200108 20200115 20200122 20200129; do
    touch "$OUTBOX/anpfuel-${d}T000000Z.dump.enc"
    touch "$OUTBOX/anpfuel-${d}T000000Z.sha256.json"
done
touch -d '40 days ago' "$OUTBOX"/anpfuel-202001*.dump.enc "$OUTBOX"/anpfuel-202001*.sha256.json
capture backup_env bash infra/scripts/backup.sh prune
if [[ "$CAP_CODE" -eq 0 ]] \
    && [[ ! -e "$OUTBOX/anpfuel-20200101T000000Z.dump.enc" ]] \
    && [[ -f "$ARTIFACT" ]]; then
    pass "old copies pruned, fresh kept"
else
    fail "old copies pruned, fresh kept"
fi

echo "backup: $PASS passed, $FAIL failed"
[[ "$FAIL" -eq 0 ]]
