#!/usr/bin/env bash
# P08-T02 harness: deploy/rollback failure behavior with stubbed
# docker and curl. Proves gate failures return to the previous
# compatible release, refusals mutate nothing, and receipts report
# honestly. No containers, network or registry involved.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1" >&2; FAIL=$((FAIL + 1)); }

STUBBIN="$(mktemp -d)"
STUBLOG="$(mktemp)"
trap 'rm -rf "$STUBBIN" "$STUBLOG"' EXIT

# Stub docker: logs calls, fails when args contain STUB_DOCKER_FAIL.
cat >"$STUBBIN/docker" <<'EOF'
#!/usr/bin/env bash
echo "docker $* ANPFUEL_RELEASE=${ANPFUEL_RELEASE:-}" >>"$STUB_LOG"
if [[ -n "${STUB_DOCKER_FAIL:-}" && "$*" == *"$STUB_DOCKER_FAIL"* ]]; then
    exit 1
fi
exit 0
EOF
# Stub curl: prints STUB_CURL_CODE, fails for STUB_CURL_FAIL_URL or
# always, or for exactly the first STUB_CURL_FAIL_FIRST_N calls.
cat >"$STUBBIN/curl" <<'EOF'
#!/usr/bin/env bash
echo "curl $*" >>"$STUB_LOG"
if [[ -n "${STUB_CURL_FAIL_FIRST_N:-}" && -n "${STUB_COUNT_FILE:-}" ]]; then
    n=0
    [[ -f "$STUB_COUNT_FILE" ]] && n=$(cat "$STUB_COUNT_FILE")
    n=$((n + 1))
    echo "$n" >"$STUB_COUNT_FILE"
    if [[ "$n" -le "$STUB_CURL_FAIL_FIRST_N" ]]; then
        exit 1
    fi
fi
if [[ -n "${STUB_CURL_FAIL_URL:-}" && "$*" == *"$STUB_CURL_FAIL_URL"* ]]; then
    exit 1
fi
if [[ "${STUB_CURL_FAIL_ALL:-}" == "1" ]]; then
    exit 1
fi
printf '%s' "${STUB_CURL_CODE:-200}"
exit 0
EOF
chmod +x "$STUBBIN/docker" "$STUBBIN/curl"

ENVFILE="$(mktemp)"
trap 'rm -rf "$STUBBIN" "$STUBLOG" "$ENVFILE"' EXIT
touch "$ENVFILE"

# run_deploy sets a clean baseline env, then applies overrides.
# Overrides arrive as VAR=value prefixes and survive the env -i reset
# through explicit forwarding below (empty stays empty).
run_deploy() {
    : >"$STUBLOG"
    : >"$STUBLOG.count"
    env -i PATH="$STUBBIN:/usr/bin:/bin" HOME="$HOME" \
        ANPFUEL_RELEASE="${ANPFUEL_RELEASE-t2026.09.30}" \
        ANPFUEL_PREVIOUS_RELEASE=t2026.09.29 \
        ANPFUEL_ENV_FILE="$ENVFILE" \
        ANPFUEL_COMPOSE_FILE=infra/compose.staging.yml \
        ANPFUEL_REVISION=test \
        ANPFUEL_NO_BUILD="${ANPFUEL_NO_BUILD-1}" \
        ANPFUEL_ALLOW_NO_BACKUP="${ANPFUEL_ALLOW_NO_BACKUP-1}" \
        ANPFUEL_READINESS_TIMEOUT_SECONDS="${ANPFUEL_READINESS_TIMEOUT_SECONDS-60}" \
        ANPFUEL_SMOKE_BASE=http://127.0.0.1 \
        STUB_LOG="$STUBLOG" \
        STUB_COUNT_FILE="$STUBLOG.count" \
        STUB_DOCKER_FAIL="${STUB_DOCKER_FAIL-}" \
        STUB_CURL_FAIL_URL="${STUB_CURL_FAIL_URL-}" \
        STUB_CURL_FAIL_ALL="${STUB_CURL_FAIL_ALL-}" \
        STUB_CURL_FAIL_FIRST_N="${STUB_CURL_FAIL_FIRST_N-}" \
        STUB_CURL_CODE="${STUB_CURL_CODE-200}" \
        bash infra/scripts/deploy.sh
}

# Captures a possibly-failing command without tripping set -e.
CAP_OUT=""
CAP_CODE=0
capture() {
    set +e
    CAP_OUT="$("$@")"
    CAP_CODE=$?
    set -e
}

# 1. Happy path deploys and verifies.
capture run_deploy
if [[ "$CAP_CODE" -eq 0 && "$CAP_OUT" == "RESULT=deployed RELEASE=t2026.09.30"* ]]; then
    pass "happy path deployed"
else
    fail "happy path deployed (code=$CAP_CODE out=$CAP_OUT)"
fi

# 2. Failed migration returns to the previous release.
STUB_DOCKER_FAIL="run --rm migrate" capture run_deploy
if [[ "$CAP_CODE" -eq 1 && "$CAP_OUT" == "RESULT=rolled-back REASON=migration"* ]] \
    && grep -q 'docker compose -f infra/compose.staging.yml up -d api worker caddy ANPFUEL_RELEASE=t2026.09.29' "$STUBLOG"; then
    pass "failed migration rolls back"
else
    fail "failed migration rolls back (code=$CAP_CODE out=$CAP_OUT)"
fi

# 3. Readiness timeout returns to the previous release: the first two
# curl calls fail (forward gate exhausts its window) while the
# rollback probe succeeds.
STUB_CURL_FAIL_FIRST_N=2 ANPFUEL_READINESS_TIMEOUT_SECONDS=6 capture run_deploy
if [[ "$CAP_CODE" -eq 1 && "$CAP_OUT" == "RESULT=rolled-back REASON=readiness"* ]]; then
    pass "readiness timeout rolls back"
else
    fail "readiness timeout rolls back (code=$CAP_CODE out=$CAP_OUT)"
fi

# 4. Smoke failure returns to the previous release.
STUB_CURL_FAIL_URL="stations" ANPFUEL_READINESS_TIMEOUT_SECONDS=60 capture run_deploy
if [[ "$CAP_CODE" -eq 1 && "$CAP_OUT" == "RESULT=rolled-back REASON=smoke"* ]]; then
    pass "smoke failure rolls back"
else
    fail "smoke failure rolls back (code=$CAP_CODE out=$CAP_OUT)"
fi

# 5. Missing release refuses before any mutation.
ANPFUEL_RELEASE= capture run_deploy
if [[ "$CAP_CODE" -eq 2 && ! -s "$STUBLOG" ]]; then
    pass "missing release refused without mutations"
else
    fail "missing release refused without mutations (code=$CAP_CODE)"
fi

# 6. Floating tag refuses.
ANPFUEL_RELEASE=latest capture run_deploy
if [[ "$CAP_CODE" -eq 2 ]]; then
    pass "floating tag refused"
else
    fail "floating tag refused (code=$CAP_CODE)"
fi

# 7. Build failure refuses without rollout.
ANPFUEL_NO_BUILD= STUB_DOCKER_FAIL="build -f" capture run_deploy
if [[ "$CAP_CODE" -eq 2 ]] && ! grep -q "up -d" "$STUBLOG"; then
    pass "build failure refuses without rollout"
else
    fail "build failure refuses without rollout (code=$CAP_CODE)"
fi

# 8. Failed rollback reports honestly.
STUB_DOCKER_FAIL="run --rm migrate" STUB_CURL_FAIL_ALL=1 ANPFUEL_READINESS_TIMEOUT_SECONDS=0 capture run_deploy
if [[ "$CAP_CODE" -eq 1 && "$CAP_OUT" == "RESULT=rollback-failed REASON=migration"* ]]; then
    pass "failed rollback reported"
else
    fail "failed rollback reported (code=$CAP_CODE out=$CAP_OUT)"
fi

# 9. Missing backup manifest refuses. Compose config is a read-only
# precheck, so the assertion targets mutations only (no run/up/build).
ANPFUEL_ALLOW_NO_BACKUP= capture run_deploy
if [[ "$CAP_CODE" -eq 2 ]] && ! grep -qE "run --rm migrate|up -d|build -f" "$STUBLOG"; then
    pass "missing backup refused without mutations"
else
    fail "missing backup refused without mutations (code=$CAP_CODE)"
fi

echo "deploy: $PASS passed, $FAIL failed"
[[ "$FAIL" -eq 0 ]]
