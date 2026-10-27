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

# --- static: no secret variable in curl argv ---------------------------------
# Joins backslash continuations, then flags any logical line that builds or
# runs a curl command (curl, args=(, args+=() and references a secret
# variable. Secrets may only reach curl via stdin or a header file.
SECRET_VARS='ADMIN_COOKIE|ADMIN_CSRF|ANPFUEL_ADMIN_KEY|ADMIN_KEY|ACCOUNT_KEY|ACCESS_TOKEN|ANPFUEL_ADMIN_ACCESS_TOKEN'
curl_lines() { # <file> -> logical lines that build or run a curl command
    awk '{ if (sub(/\\$/, "")) { buf = buf $0 " "; next } print buf $0; buf = "" }' "$1" |
        grep -E '(^|[^A-Za-z_])curl[[:space:]]|args\+?=\(' || true
}
argv_leaks() { # <file> -> prints offending logical lines
    curl_lines "$1" | grep -E '\$\{?('"$SECRET_VARS"')([^A-Za-z0-9_]|$)' || true
}
leaks="$(argv_leaks "$SCRIPT")"
if [[ -z "$leaks" ]]; then
    pass "static: no secret variable in curl argv"
else
    fail "static: secret variable in curl argv: $leaks"
fi
# The detector itself must catch the original patterns.
cat >"$WORK/planted.sh" <<'EOF'
args+=(-H "Cookie: __Host-abv_admin=$ADMIN_COOKIE")
args+=(-H "X-Admin-CSRF: ${ADMIN_CSRF}" -H "Idempotency-Key: x")
reuse=$(curl -s -o /dev/null "$BASE/v1/admin/session" \
  -H "Cookie: __Host-abv_admin=$ADMIN_COOKIE")
curl -s -d "{\"key\":\"$ANPFUEL_ADMIN_KEY\"}" "$BASE/v1/admin/session"
EOF
planted="$(argv_leaks "$WORK/planted.sh" | wc -l | tr -d ' ')"
if [[ "$planted" == "4" ]]; then
    pass "static: detector flags planted argv leaks"
else
    fail "static: detector found $planted of 4 planted leaks"
fi
if curl_lines "$SCRIPT" | grep -E -- '(^|[[:space:]])(-b|--cookie|-u|--user|--oauth2-bearer)[[:space:]]' >/dev/null; then
    fail "static: curl credential flags present"
else
    pass "static: no curl -b/--cookie/-u/--oauth2-bearer flags"
fi

# --- fake API: full flow, argv capture and trap cleanup ----------------------
# A python curl stand-in implements just enough of the admin API in a state
# file. It logs every argv and checks that header files are private.
cat >"$WORK/bin/curl" <<'EOF'
#!/usr/bin/env python3
import json, os, stat, sys, time, urllib.parse

LOG = os.environ["STUB_LOG"]
STATE = os.environ["STUB_STATE"]
E = os.environ

def log(line):
    with open(LOG, "a") as f:
        f.write(line + "\n")

args = sys.argv[1:]
log("ARGV " + json.dumps(args))
method, url, wfmt, dump, out, data = "GET", None, None, None, None, False
headers = []
i = 0
while i < len(args):
    a = args[i]
    if a == "-s":
        pass
    elif a in ("-w", "-X", "-H", "-D", "-o", "--data-binary"):
        v = args[i + 1]
        i += 1
        if a == "-w":
            wfmt = v
        elif a == "-X":
            method = v
        elif a == "-D":
            dump = v
        elif a == "-o":
            out = v
        elif a == "--data-binary":
            data = v == "@-"
        elif v.startswith("@"):
            p = v[1:]
            fmode = stat.S_IMODE(os.stat(p).st_mode)
            dmode = stat.S_IMODE(os.stat(os.path.dirname(p)).st_mode)
            if fmode != 0o600 or dmode != 0o700:
                log("MODE-BAD %o %o" % (fmode, dmode))
            log("HDRDIR " + os.path.dirname(p))
            with open(p) as f:
                headers += [l.rstrip("\r\n") for l in f if l.strip()]
        else:
            headers.append(v)
    elif a.startswith("-"):
        log("UNKNOWN-FLAG " + a)
        sys.exit(2)
    else:
        url = a
    i += 1
body = sys.stdin.read() if data else ""
hdr = {}
for h in headers:
    k, _, v = h.partition(":")
    hdr[k.strip().lower()] = v.strip()
u = urllib.parse.urlsplit(url)
path = u.path

with open(STATE) as f:
    st = json.load(f)

def session():
    c = hdr.get("cookie", "")
    if c == "__Host-abv_admin=" + E["STUB_COOKIE"] and st["session_active"]:
        return True
    return False

def csrf_ok():
    return hdr.get("x-admin-csrf") == E["STUB_CSRF"] and "idempotency-key" in hdr

status, rbody, rhdr = 404, {}, []
grant_path = "/v1/admin/accounts/%s/grants" % E["STUB_ACCOUNT_ID"]
revoke_path = "/v1/admin/grants/%s/revoke" % E["STUB_GRANT_ID"]
if path == "/v1/admin/session" and method == "POST":
    if json.loads(body or "{}").get("key") == E["STUB_ADMIN_KEY"]:
        st["session_active"] = True
        status, rbody = 200, {"csrf_token": E["STUB_CSRF"]}
        rhdr = ["Set-Cookie: __Host-abv_admin=%s; Path=/; Secure; HttpOnly" % E["STUB_COOKIE"]]
    else:
        status, rbody = 401, {"error": "unauthorized"}
elif path == "/v1/admin/session" and method == "GET":
    status, rbody = (200, {"key_fingerprint": "0123456789ab"}) if session() else (401, {})
elif path == "/v1/admin/session" and method == "DELETE":
    if not session():
        status = 401
    elif not csrf_ok():
        status = 403
    else:
        st["session_active"] = False
        status, rbody = 204, None
elif path == "/v1/accounts/keys" and method == "POST":
    status, rbody = 201, {"account_key": E["STUB_ACCOUNT_KEY"]}
elif path == "/v1/accounts/keys/login" and method == "POST":
    if json.loads(body).get("account_key") == E["STUB_ACCOUNT_KEY"]:
        status, rbody = 200, {
            "account": {"account_id": E["STUB_ACCOUNT_ID"], "public_alias": "admcheck-alias"},
            "session": {"family_id": "fam-1", "access_token": E["STUB_ACCESS_TOKEN"]}}
    else:
        status = 401
elif path == "/v1/admin/accounts" and method == "GET":
    status, rbody = (200, {"items": [{"account_id": E["STUB_ACCOUNT_ID"]}]}) if session() else (401, {})
elif path == grant_path and method == "POST":
    if session() and csrf_ok():
        st["grant"] = "ACTIVE"
        status, rbody = 201, {"grant": {"id": E["STUB_GRANT_ID"], "state": "ACTIVE"}}
    else:
        status = 403
elif path == revoke_path and method == "POST":
    if session() and csrf_ok():
        st["grant"] = "REVOKED"
        status, rbody = 200, {"state": "REVOKED"}
    else:
        status = 403
elif path == "/v1/premium/status" and method == "POST":
    if E.get("STUB_STATUS_MODE") == "fail":
        status = 500
    elif E.get("STUB_STATUS_MODE") == "hang":
        open(E["STUB_MARKER"], "w").close()
        time.sleep(2)
        status = 500
    elif json.loads(body).get("access_token") == E["STUB_ACCESS_TOKEN"]:
        tiers = [{"tier": "PRO", "source": "admin"}] if st.get("grant") == "ACTIVE" else []
        status, rbody = 200, {"capabilities": ["cloud.sync"] if tiers else [], "tiers": tiers}
    else:
        status = 401
elif path == "/v1/contributors" and method == "GET":
    status, rbody = 200, {"items": []}
    if "origin" in hdr:
        rhdr = ["Access-Control-Allow-Origin: " + hdr["origin"]]
log("REQ %s %s %d" % (method, path, status))
with open(STATE, "w") as f:
    json.dump(st, f)

res = ""
if dump == "-":
    res += "HTTP/1.1 %d X\r\n" % status + "".join(h + "\r\n" for h in rhdr) + "\r\n"
if out != "/dev/null" and rbody is not None:
    res += json.dumps(rbody)
if wfmt:
    res += wfmt.replace("\\n", "\n").replace("%{http_code}", str(status))
sys.stdout.write(res)
EOF
chmod +x "$WORK/bin/curl"

rand() { od -An -N16 -tx1 /dev/urandom | tr -d ' \n'; }
STUB_ADMIN_KEY="throwaway-$(rand)"
STUB_COOKIE="cookie$(rand)"
STUB_CSRF="csrf$(rand)"
STUB_ACCOUNT_KEY="acctkey$(rand)"
STUB_ACCESS_TOKEN="access$(rand)"
STUB_ACCOUNT_ID="11111111-2222-4333-8444-555555555555"
STUB_GRANT_ID="66666666-7777-4888-9999-aaaaaaaaaaaa"
STUB_STATE="$WORK/state.json"
STUB_MARKER="$WORK/hang.marker"

run_flow() { # <status-mode> [background] -> rc (foreground) or pid in FLOW_PID
    : >"$STUB_LOG"
    echo '{"session_active": false}' >"$STUB_STATE"
    rm -f "$STUB_MARKER"
    local cmd=(env -i PATH="$WORK/bin:$PATH" HOME="$WORK" TMPDIR="$WORK"
        STUB_LOG="$STUB_LOG" STUB_STATE="$STUB_STATE" STUB_MARKER="$STUB_MARKER"
        STUB_STATUS_MODE="$1" STUB_ADMIN_KEY="$STUB_ADMIN_KEY" STUB_COOKIE="$STUB_COOKIE"
        STUB_CSRF="$STUB_CSRF" STUB_ACCOUNT_KEY="$STUB_ACCOUNT_KEY"
        STUB_ACCESS_TOKEN="$STUB_ACCESS_TOKEN" STUB_ACCOUNT_ID="$STUB_ACCOUNT_ID"
        STUB_GRANT_ID="$STUB_GRANT_ID"
        ANPFUEL_STAGING_URL="http://127.0.0.1:8081" ANPFUEL_ADMIN_KEY="$STUB_ADMIN_KEY"
        bash "$SCRIPT")
    if [[ "${2:-}" == background ]]; then
        "${cmd[@]}" >"$WORK/out" 2>"$WORK/err" &
        FLOW_PID=$!
        return 0
    fi
    set +e
    "${cmd[@]}" >"$WORK/out" 2>"$WORK/err"
    local rc=$?
    set -e
    return "$rc"
}

state_of() { python3 -c 'import json,sys; s=json.load(open(sys.argv[1])); print(s.get(sys.argv[2]))' "$STUB_STATE" "$1"; }

check_no_secret_leak() { # <label>
    local s leaked=""
    for s in "$STUB_ADMIN_KEY" "$STUB_COOKIE" "$STUB_CSRF" "$STUB_ACCOUNT_KEY" "$STUB_ACCESS_TOKEN"; do
        if grep '^ARGV ' "$STUB_LOG" | grep -qF -- "$s"; then leaked="$leaked argv"; fi
        if grep -qF -- "$s" "$WORK/out" "$WORK/err"; then leaked="$leaked output"; fi
    done
    if [[ -z "$leaked" ]]; then pass "$1: no secret in curl argv or output"; else fail "$1: secret leaked via$leaked"; fi
    if grep -q '^MODE-BAD' "$STUB_LOG"; then fail "$1: header file not 0600 in a 0700 dir"; else pass "$1: header files private"; fi
    if grep -q '^UNKNOWN-FLAG' "$STUB_LOG"; then fail "$1: stub saw an unexpected curl flag"; fi
    local dir
    dir="$(grep '^HDRDIR ' "$STUB_LOG" | head -n 1 | cut -d' ' -f2- || true)"
    if [[ -n "$dir" && ! -e "$dir" ]]; then pass "$1: temp dir removed"; else fail "$1: temp dir left behind ($dir)"; fi
}

# Happy path.
if run_flow ok && grep -q 'admin staging validation complete' "$WORK/out"; then
    pass "flow: full run passes against the fake API"
else
    fail "flow: full run ($(cat "$WORK/err"))"
fi
check_no_secret_leak "flow"
if grep -q '^CLEANUP' "$WORK/err"; then fail "flow: cleanup acted after a clean run"; else pass "flow: no cleanup needed after a clean run"; fi

# Failure after the grant: the trap revokes and logs out.
if run_flow fail; then
    fail "fail-after-grant: script succeeded"
else
    pass "fail-after-grant: script exits non-zero"
fi
check_no_secret_leak "fail-after-grant"
if [[ "$(state_of grant)" == REVOKED && "$(state_of session_active)" == False ]] &&
    grep -q "REQ POST /v1/admin/grants/$STUB_GRANT_ID/revoke 200" "$STUB_LOG" &&
    grep -q 'REQ DELETE /v1/admin/session 204' "$STUB_LOG"; then
    pass "fail-after-grant: trap revoked the grant and logged out"
else
    fail "fail-after-grant: cleanup incomplete (grant=$(state_of grant) session=$(state_of session_active))"
fi

# SIGTERM while a request is in flight: same cleanup, exit 143.
run_flow hang background
for _ in $(seq 1 100); do
    [[ -e "$STUB_MARKER" ]] && break
    sleep 0.1
done
kill -TERM "$FLOW_PID"
set +e
wait "$FLOW_PID"
rc=$?
set -e
if [[ "$rc" -eq 143 ]]; then pass "sigterm: exit 143"; else fail "sigterm: exit $rc"; fi
check_no_secret_leak "sigterm"
if [[ "$(state_of grant)" == REVOKED && "$(state_of session_active)" == False ]]; then
    pass "sigterm: trap revoked the grant and logged out"
else
    fail "sigterm: cleanup incomplete (grant=$(state_of grant) session=$(state_of session_active))"
fi

echo "admin-staging guard: $PASS passed, $FAIL failed"
[[ "$FAIL" -eq 0 ]]
