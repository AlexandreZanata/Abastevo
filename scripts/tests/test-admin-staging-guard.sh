#!/usr/bin/env bash
# A5a review harness for scripts/tests/test-admin-staging.sh. No network:
# curl is replaced by a logging stub on PATH. Proves the target rule runs
# before any request and never echoes the rejected URL.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT="$ROOT/scripts/tests/test-admin-staging.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1" >&2; FAIL=$((FAIL + 1)); }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/bin"
STUB_LOG="$WORK/curl.log"
: >"$STUB_LOG"
cat >"$WORK/bin/curl" <<'EOF'
#!/usr/bin/env bash
echo "curl $*" >>"$STUB_LOG"
exit 7
EOF
chmod +x "$WORK/bin/curl"
export STUB_LOG

run_check() { # <url> -> rc; stderr kept in $WORK/err
    set +e
    env -i PATH="$WORK/bin:$PATH" HOME="$WORK" STUB_LOG="$STUB_LOG" \
        ANPFUEL_STAGING_URL="$1" bash "$SCRIPT" --check-target \
        >"$WORK/out" 2>"$WORK/err"
    local rc=$?
    set -e
    return "$rc"
}

# --- target rule -----------------------------------------------------------
accept=(
    "https://teste.abastevo.com.br"
    "https://teste.abastevo.com.br/"
    "https://teste.abastevo.com.br:443"
    "https://TESTE.abastevo.com.br"
    "http://127.0.0.1:8081"
    "https://127.0.0.1:8443"
    "http://[::1]:8081"
    "http://localhost:8081"
    "http://localhost"
)
for url in "${accept[@]}"; do
    if run_check "$url"; then pass "accept $url"; else fail "accept $url ($(cat "$WORK/err"))"; fi
done

reject=(
    "plain http to staging|http://teste.abastevo.com.br"
    "plain http to remote|http://evil.example"
    "production api|https://api.abastevo.com.br"
    "production apex|https://abastevo.com.br"
    "production www|https://www.abastevo.com.br"
    "production app|https://app.abastevo.com.br"
    "production portal|https://portal.abastevo.com.br"
    "production trailing dot|https://api.abastevo.com.br."
    "production uppercase|https://API.Abastevo.com.br"
    "staging trailing dot|https://teste.abastevo.com.br."
    "unlisted https host|https://evil.example"
    "staging lookalike|https://teste.abastevo.com.br.evil.example"
    "userinfo password|https://user:s3cr3t-pass@teste.abastevo.com.br"
    "userinfo host swap|https://teste.abastevo.com.br@evil.example"
    "backslash userinfo|https://teste.abastevo.com.br\\@evil.example"
    "path|https://teste.abastevo.com.br/v1"
    "dot path|https://teste.abastevo.com.br/../x"
    "double slash|https://teste.abastevo.com.br//"
    "query|https://teste.abastevo.com.br?x=1"
    "fragment|https://teste.abastevo.com.br#x"
    "percent encoding|https://teste%2eabastevo.com.br"
    "non-default staging port|https://teste.abastevo.com.br:8443"
    "empty port|http://127.0.0.1:"
    "port out of range|http://127.0.0.1:99999"
    "non-numeric port|http://127.0.0.1:80x"
    "two ports|http://127.0.0.1:80:81"
    "short loopback form|http://127.1"
    "decimal loopback form|http://2130706433"
    "mapped ipv6|http://[::ffff:127.0.0.1]"
    "unterminated ipv6|http://[::1"
    "ipv6 garbage suffix|http://[::1]x"
    "loopback trailing dot|http://localhost."
    "localhost lookalike|https://localhost.evil.example"
    "uppercase scheme|HTTPS://teste.abastevo.com.br"
    "other scheme|ftp://teste.abastevo.com.br"
    "no scheme|teste.abastevo.com.br"
    "missing host|https://"
    "space in host|https://teste.abastevo.com.br x"
    "unicode host|https://teste.abastevó.com.br"
)
for entry in "${reject[@]}"; do
    name="${entry%%|*}"
    url="${entry#*|}"
    if run_check "$url"; then
        fail "reject $name: accepted"
    elif grep -q 'rejected before any network call' "$WORK/err"; then
        pass "reject $name"
    else
        fail "reject $name: unexpected error ($(cat "$WORK/err"))"
    fi
done

run_check "https://user:s3cr3t-pass@teste.abastevo.com.br" || true
if grep -q 's3cr3t-pass' "$WORK/err" "$WORK/out"; then
    fail "refusal echoes userinfo"
else
    pass "refusal never echoes the URL"
fi

# Full mode with a key set still refuses before any request.
set +e
env -i PATH="$WORK/bin:$PATH" HOME="$WORK" STUB_LOG="$STUB_LOG" \
    ANPFUEL_STAGING_URL="https://api.abastevo.com.br" \
    ANPFUEL_ADMIN_KEY="throwaway-test-key-not-real" \
    bash "$SCRIPT" >"$WORK/out" 2>"$WORK/err"
rc=$?
set -e
if [[ "$rc" -ne 0 ]] && grep -q 'production host' "$WORK/err"; then
    pass "run mode refuses production"
else
    fail "run mode refuses production (rc=$rc)"
fi
if [[ -s "$STUB_LOG" ]]; then
    fail "curl invoked during target checks"
else
    pass "no curl invocation for any target check"
fi

echo "admin-staging guard: $PASS passed, $FAIL failed"
[[ "$FAIL" -eq 0 ]]
