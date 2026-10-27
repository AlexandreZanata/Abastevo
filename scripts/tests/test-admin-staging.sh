#!/usr/bin/env bash
# A5a admin staging validation (ADMIN_CONTRIBUTOR_PLAN.md sections A5 and 1a).
#
# Runs the admin-surface checks against a live API: login, who-am-I, wrong
# key, missing CSRF, synthetic account search, PRO grant, premium status
# tier, revoke, status cleared, logout, cookie reuse and the public
# contributors CORS check. The S1-style disabled check is intentionally
# skipped: admin routes are mounted wherever a key hash is configured.
#
# Environment (operator shell only, never committed):
#   ANPFUEL_ADMIN_KEY      raw admin key (required, never printed)
#   ANPFUEL_STAGING_URL    API origin, e.g. https://staging.example (required)
#   ANPFUEL_ADMIN_ORIGIN   exact Origin header (default: ANPFUEL_STAGING_URL)
#   ANPFUEL_ADMIN_ACCOUNT  existing synthetic account id (optional; otherwise
#                          a throwaway key account is created; when set, also
#                          set ANPFUEL_ADMIN_FAMILY_ID and
#                          ANPFUEL_ADMIN_ACCESS_TOKEN for the status check)
#   ANPFUEL_PUBLIC_ORIGIN  Origin for the contributors CORS check
#                          (default: https://abastevo.com.br)
#
# Target rule (checked before any network call): ANPFUEL_STAGING_URL must be
# a bare origin (scheme://host[:port], optional trailing slash; no userinfo,
# path, query, fragment or percent-encoding). Loopback hosts (127.0.0.1, ::1,
# localhost) may use http or https; the only remote host accepted is the
# documented staging origin https://teste.abastevo.com.br (ADR-019,
# RST_VPS_HARDENING), on the default port. abastevo.com.br and every other
# *.abastevo.com.br name are refused as production. Plain http to a remote
# host is refused because the raw admin key travels in the login body.
#
# Usage:
#   test-admin-staging.sh                 run the validation
#   test-admin-staging.sh --check-target  validate ANPFUEL_STAGING_URL only,
#                                         no key required, no network
#
# Secrets (admin key, session cookie, CSRF token, account key and access
# token) never appear in curl's argv, where `ps` could show them: request
# bodies are piped to `--data-binary @-` and the cookie/CSRF headers are
# read by curl from 0600 files (`-H @file`) inside a private `mktemp -d`
# directory. An EXIT/INT/TERM trap revokes a grant this run created and
# logs the admin session out when they are still active, then removes the
# directory.
#
# Prints step results only. Exits non-zero on the first failure.
set +x
set -uo pipefail

say() { printf 'PASS: %s\n' "$1"; }
die() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

STAGING_HOST="teste.abastevo.com.br"
PRODUCTION_DOMAIN="abastevo.com.br"

# check_target <url>: prints a non-secret refusal reason on stderr and
# returns 1 when the URL is not an acceptable target. Never echoes the URL
# (it could carry userinfo). Pure string checks: no network, no DNS.
check_target() {
  local LC_ALL=C url="$1" scheme rest authority host port lower
  case "$url" in
    http://*) scheme=http; rest="${url#http://}" ;;
    https://*) scheme=https; rest="${url#https://}" ;;
    *) printf 'target: scheme must be lowercase http:// or https://\n' >&2; return 1 ;;
  esac
  authority="${rest%/}"
  if [ -z "$authority" ]; then
    printf 'target: missing host\n' >&2; return 1
  fi
  case "$authority" in
    *@*) printf 'target: userinfo (@) is not allowed\n' >&2; return 1 ;;
    */*|*\?*|*#*|*\\*) printf 'target: path, query or fragment is not allowed\n' >&2; return 1 ;;
    *%*) printf 'target: percent-encoding is not allowed\n' >&2; return 1 ;;
  esac
  if [ "${authority#\[}" != "$authority" ]; then
    host="${authority#\[}"; host="${host%%]*}"
    port="${authority#*]}"
    case "$port" in
      '') ;;
      :*) port="${port#:}" ;;
      *) printf 'target: malformed bracketed host\n' >&2; return 1 ;;
    esac
    if [ "$host" != "::1" ]; then
      printf 'target: only the ::1 IPv6 literal is allowed\n' >&2; return 1
    fi
  else
    host="${authority%%:*}"
    port=""
    if [ "$host" != "$authority" ]; then
      port="${authority#*:}"
    fi
    case "$host" in
      ''|*[!A-Za-z0-9.-]*) printf 'target: host has characters outside [A-Za-z0-9.-]\n' >&2; return 1 ;;
    esac
  fi
  if [ -n "$port" ] || [ "${authority%:}" != "$authority" ]; then
    case "$port" in
      ''|*[!0-9]*) printf 'target: port must be numeric\n' >&2; return 1 ;;
    esac
    if [ "${#port}" -gt 5 ] || [ "$((10#$port))" -lt 1 ] || [ "$((10#$port))" -gt 65535 ]; then
      printf 'target: port out of range\n' >&2; return 1
    fi
  fi
  lower=$(printf '%s' "$host" | tr '[:upper:]' '[:lower:]')
  # Trailing dots would dodge exact-name comparisons; normalize for the
  # production check and refuse them for the allowlist.
  local bare="$lower"
  while [ "${bare%.}" != "$bare" ]; do bare="${bare%.}"; done
  case "$bare" in
    127.0.0.1|::1|localhost)
      if [ "$bare" != "$lower" ]; then
        printf 'target: trailing dot is not allowed\n' >&2; return 1
      fi
      return 0 ;;
  esac
  if [ "$bare" != "$STAGING_HOST" ]; then
    case "$bare" in
      "$PRODUCTION_DOMAIN"|*."$PRODUCTION_DOMAIN")
        printf 'target: refusing production host; this script is staging-only\n' >&2; return 1 ;;
    esac
  fi
  if [ "$scheme" != https ]; then
    printf 'target: plain http is only allowed for loopback hosts (the admin key is sent in the login body)\n' >&2
    return 1
  fi
  if [ "$lower" != "$STAGING_HOST" ]; then
    printf 'target: host is not the allowlisted staging host %s or loopback\n' "$STAGING_HOST" >&2
    return 1
  fi
  if [ -n "$port" ] && [ "$((10#$port))" -ne 443 ]; then
    printf 'target: the staging host must use the default https port\n' >&2; return 1
  fi
  return 0
}

MODE=run
case "${1:-}" in
  '') ;;
  --check-target) MODE=check-target ;;
  *) die "usage: test-admin-staging.sh [--check-target]" ;;
esac

: "${ANPFUEL_STAGING_URL:?ANPFUEL_STAGING_URL is required}"
check_target "$ANPFUEL_STAGING_URL" || die "ANPFUEL_STAGING_URL rejected before any network call"
if [ "$MODE" = check-target ]; then
  say "target"
  exit 0
fi

for tool in curl python3 date; do
  command -v "$tool" >/dev/null 2>&1 || die "missing required tool: $tool"
done

: "${ANPFUEL_ADMIN_KEY:?ANPFUEL_ADMIN_KEY is required}"
BASE="${ANPFUEL_STAGING_URL%/}"
ADMIN_ORIGIN="${ANPFUEL_ADMIN_ORIGIN:-$BASE}"
PUBLIC_ORIGIN="${ANPFUEL_PUBLIC_ORIGIN:-https://abastevo.com.br}"

# Private work directory for the secret header files (mode 0700/0600).
umask 077
SESSION_ACTIVE=0
GRANT_ACTIVE=0
GRANT_ID=""
ADMIN_COOKIE=""
ADMIN_CSRF=""
WORKDIR=$(mktemp -d) || die "cannot create a private temp directory"
COOKIE_HDR="$WORKDIR/cookie.hdr"
CSRF_HDR="$WORKDIR/csrf.hdr"

note() { printf 'CLEANUP: %s\n' "$1" >&2; }

# cleanup: best effort, never prints a secret. Runs once on any exit.
cleanup() {
  local rc=$?
  trap - EXIT INT TERM
  if [ "$SESSION_ACTIVE" = 1 ] && [ -s "$COOKIE_HDR" ] && [ -s "$CSRF_HDR" ]; then
    if [ "$GRANT_ACTIVE" = 1 ] && [ -n "$GRANT_ID" ]; then
      api POST "/v1/admin/grants/$GRANT_ID/revoke" '{"reason":"staging validation cleanup revoke"}'
      if [ "$RESP_STATUS" = 200 ]; then
        note "grant $GRANT_ID revoked"
      else
        note "grant $GRANT_ID revoke failed (HTTP ${RESP_STATUS:-none}); revoke it manually"
      fi
    fi
    api DELETE /v1/admin/session '{"reason":"staging validation cleanup logout"}'
    if [ "$RESP_STATUS" = 204 ]; then
      note "admin session logged out"
    else
      note "admin session logout failed (HTTP ${RESP_STATUS:-none}); it expires on its own"
    fi
  elif [ "$GRANT_ACTIVE" = 1 ] && [ -n "$GRANT_ID" ]; then
    note "grant $GRANT_ID may still be active; revoke it manually"
  fi
  rm -rf "$WORKDIR"
  exit "$rc"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# write_secret_headers: printf is a builtin, so the values never reach an
# exec argv; the files inherit umask 077.
write_secret_headers() {
  printf 'Cookie: __Host-abv_admin=%s\n' "$ADMIN_COOKIE" >"$COOKIE_HDR"
  printf 'X-Admin-CSRF: %s\n' "$ADMIN_CSRF" >"$CSRF_HDR"
}

new_uuid() {
  if [ -r /proc/sys/kernel/random/uuid ]; then
    cat /proc/sys/kernel/random/uuid
  else
    python3 -c 'import uuid; print(uuid.uuid4())'
  fi
}

# jget <python-expr>: evaluate against $RESP_BODY, print the result.
# Callers must capture the output into a variable, never bare.
jget() {
  printf '%s' "$RESP_BODY" | python3 -c "import json,sys; d=json.load(sys.stdin); print($1)"
}

# api <method> <path> [body]: performs the request with the session cookie
# and CSRF headers when set. Sets RESP_STATUS and RESP_BODY. Bodies travel
# through stdin and secret headers through 0600 files, never argv.
api() {
  local method="$1" path="$2" body="${3:-}"
  local args=(-s -w '\n%{http_code}' -X "$method" "$BASE$path"
    -H 'Content-Type: application/json' -H "Origin: $ADMIN_ORIGIN")
  if [ -s "$COOKIE_HDR" ]; then
    args+=(-H "@$COOKIE_HDR")
  fi
  if [ -s "$CSRF_HDR" ]; then
    args+=(-H "@$CSRF_HDR" -H "Idempotency-Key: $(new_uuid)")
  fi
  local raw
  if [ -n "$body" ]; then
    raw=$(printf '%s' "$body" | curl "${args[@]}" --data-binary @-)
  else
    raw=$(curl "${args[@]}")
  fi
  RESP_STATUS=$(printf '%s' "$raw" | tail -n 1)
  RESP_BODY=$(printf '%s' "$raw" | sed '$d')
}

expect() { # <step> <want-status> <got-status>
  if [ "$3" != "$2" ]; then
    die "$1: want HTTP $2, got $3"
  fi
  say "$1"
}

login_body() {
  ADMIN_KEY="$ANPFUEL_ADMIN_KEY" python3 -c 'import json,os; print(json.dumps({"key": os.environ["ADMIN_KEY"]}))'
}

say "config (disabled-check skipped: admin routes are mounted)"
[ -n "$ANPFUEL_ADMIN_KEY" ] || die "config: empty admin key"

# 1. Login. One request: headers (cookie) and JSON body stay in memory.
exchange=$(login_body | curl -s -D - -o - -X POST "$BASE/v1/admin/session" \
  -H 'Content-Type: application/json' -H "Origin: $ADMIN_ORIGIN" \
  -H "Idempotency-Key: $(new_uuid)" --data-binary @- -w '\n%{http_code}')
RESP_STATUS=$(printf '%s' "$exchange" | tail -n 1)
RESP_BODY=$(printf '%s' "$exchange" | grep '^{' | head -n 1)
expect "login" "200" "$RESP_STATUS"
SESSION_ACTIVE=1
ADMIN_COOKIE=$(printf '%s' "$exchange" | awk 'tolower($1)=="set-cookie:"{split($2,a,";");split(a[1],b,"=");print b[2];exit}')
[ "${#ADMIN_COOKIE}" -ge 20 ] || die "login: missing session cookie"
ADMIN_CSRF=$(jget 'd["csrf_token"]')
[ "${#ADMIN_CSRF}" -ge 20 ] || die "login: missing csrf token"
write_secret_headers

# 2. Who-am-I.
api GET /v1/admin/session
expect "whoami" "200" "$RESP_STATUS"
[ "$(jget 'len(d.get("key_fingerprint",""))')" = "12" ] || die "whoami: bad fingerprint"

# 3. Wrong key shares one 401.
wrong=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/v1/admin/session" \
  -H 'Content-Type: application/json' -H "Origin: $ADMIN_ORIGIN" \
  --data-binary @- <<< '{"key":"not-a-real-key"}')
expect "wrong-key" "401" "$wrong"

# 4. Mutation without CSRF is 403.
no_csrf=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$BASE/v1/admin/session" \
  -H 'Content-Type: application/json' -H "Origin: $ADMIN_ORIGIN" \
  -H "@$COOKIE_HDR" \
  -H "Idempotency-Key: $(new_uuid)" --data-binary @- <<< '{"reason":"staging validation logout"}')
expect "missing-csrf" "403" "$no_csrf"

# 5. Synthetic account: reuse or create a throwaway key account.
if [ -n "${ANPFUEL_ADMIN_ACCOUNT:-}" ]; then
  case "$ANPFUEL_ADMIN_ACCOUNT" in
    ????????-????-????-????-????????????) ACCOUNT_ID="$ANPFUEL_ADMIN_ACCOUNT" ;;
    *) die "account: ANPFUEL_ADMIN_ACCOUNT is not a UUID" ;;
  esac
  : "${ANPFUEL_ADMIN_FAMILY_ID:?ANPFUEL_ADMIN_ACCOUNT needs ANPFUEL_ADMIN_FAMILY_ID}"
  : "${ANPFUEL_ADMIN_ACCESS_TOKEN:?ANPFUEL_ADMIN_ACCOUNT needs ANPFUEL_ADMIN_ACCESS_TOKEN}"
  FAMILY_ID="$ANPFUEL_ADMIN_FAMILY_ID"
  ACCESS_TOKEN="$ANPFUEL_ADMIN_ACCESS_TOKEN"
  ACCOUNT_ALIAS="$ACCOUNT_ID"
  say "account (provided)"
else
  USERNAME="admcheck$(date +%s | tail -c 7)"
  created=$(curl -s -w '\n%{http_code}' -X POST "$BASE/v1/accounts/keys" \
    -H 'Content-Type: application/json' --data-binary @- <<<"{\"username\":\"$USERNAME\"}")
  RESP_STATUS=$(printf '%s' "$created" | tail -n 1)
  RESP_BODY=$(printf '%s' "$created" | sed '$d')
  expect "account-create" "201" "$RESP_STATUS"
  ACCOUNT_KEY=$(jget 'd["account_key"]')
  [ "${#ACCOUNT_KEY}" -ge 10 ] || die "account-create: missing account key"
  key_login_body=$(ACCOUNT_KEY="$ACCOUNT_KEY" python3 -c 'import json,os; print(json.dumps({"account_key": os.environ["ACCOUNT_KEY"]}))')
  logged=$(curl -s -w '\n%{http_code}' -X POST "$BASE/v1/accounts/keys/login" \
    -H 'Content-Type: application/json' --data-binary @- <<<"$key_login_body")
  RESP_STATUS=$(printf '%s' "$logged" | tail -n 1)
  RESP_BODY=$(printf '%s' "$logged" | sed '$d')
  expect "account-login" "200" "$RESP_STATUS"
  ACCOUNT_ID=$(jget 'd["account"]["account_id"]')
  ACCOUNT_ALIAS=$(jget 'd["account"]["public_alias"]')
  FAMILY_ID=$(jget 'd["session"]["family_id"]')
  ACCESS_TOKEN=$(jget 'd["session"]["access_token"]')
  [ "${#ACCOUNT_ID}" -eq 36 ] || die "account-login: bad account id"
fi

# 6. Search finds the synthetic account.
api GET "/v1/admin/accounts?q=$ACCOUNT_ALIAS"
expect "search" "200" "$RESP_STATUS"
[ "$(jget 'len(d["items"])')" -ge 1 ] || die "search: account not listed"

# 7. Grant PRO for 1 day.
END_AT=$(date -u -d '+24 hours' '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || python3 -c 'import datetime; print((datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(hours=24)).strftime("%Y-%m-%dT%H:%M:%SZ"))')
api POST "/v1/admin/accounts/$ACCOUNT_ID/grants" "$(END_AT="$END_AT" python3 -c 'import json,os; print(json.dumps({"tier":"PRO","end_at":os.environ["END_AT"],"reason":"staging validation grant","replace":False}))')"
expect "grant" "201" "$RESP_STATUS"
GRANT_ID=$(jget 'd["grant"]["id"]')
case "$GRANT_ID" in
  ????????-????-????-????-????????????) GRANT_ACTIVE=1 ;;
  *) GRANT_ID=""; die "grant: response has no grant id" ;;
esac
[ "$(jget 'd["grant"]["state"]')" = "ACTIVE" ] || die "grant: not active"

# 8. Premium status shows the admin tier.
status_body=$(FAMILY_ID="$FAMILY_ID" ACCESS_TOKEN="$ACCESS_TOKEN" python3 -c 'import json,os; print(json.dumps({"family_id":os.environ["FAMILY_ID"],"access_token":os.environ["ACCESS_TOKEN"]}))')
status=$(curl -s -w '\n%{http_code}' -X POST "$BASE/v1/premium/status" \
  -H 'Content-Type: application/json' --data-binary @- <<<"$status_body")
RESP_STATUS=$(printf '%s' "$status" | tail -n 1)
RESP_BODY=$(printf '%s' "$status" | sed '$d')
expect "status-tier" "200" "$RESP_STATUS"
[ "$(jget '("cloud.sync" in d["capabilities"])')" = "True" ] || die "status-tier: capability missing"
[ "$(jget 'any(t["tier"]=="PRO" and t["source"]=="admin" for t in d["tiers"])')" = "True" ] || die "status-tier: admin tier missing"

# 9. Revoke, then status no longer shows the admin tier.
api POST "/v1/admin/grants/$GRANT_ID/revoke" '{"reason":"staging validation revoke"}'
expect "revoke" "200" "$RESP_STATUS"
[ "$(jget 'd["state"]')" = "REVOKED" ] || die "revoke: not revoked"
GRANT_ACTIVE=0
status=$(curl -s -w '\n%{http_code}' -X POST "$BASE/v1/premium/status" \
  -H 'Content-Type: application/json' --data-binary @- <<<"$status_body")
RESP_STATUS=$(printf '%s' "$status" | tail -n 1)
RESP_BODY=$(printf '%s' "$status" | sed '$d')
expect "status-cleared" "200" "$RESP_STATUS"
[ "$(jget 'any(t.get("source")=="admin" for t in d["tiers"])')" = "False" ] || die "status-cleared: admin tier still present"

# 10. Logout, then cookie reuse is 401.
api DELETE /v1/admin/session '{"reason":"staging validation logout"}'
expect "logout" "204" "$RESP_STATUS"
SESSION_ACTIVE=0
reuse=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/v1/admin/session" -H "@$COOKIE_HDR")
expect "reuse-cookie" "401" "$reuse"

# 11. Public contributors list with CORS for the landing origin.
cors=$(curl -s -D - -o /dev/null "$BASE/v1/contributors" -H "Origin: $PUBLIC_ORIGIN")
[ "$(printf '%s' "$cors" | head -n 1 | awk '{print $2}')" = "200" ] || die "contributors-cors: bad status"
[ "$(printf '%s' "$cors" | awk -F': ' 'tolower($1)=="access-control-allow-origin"{print $2}' | tr -d '\r')" = "$PUBLIC_ORIGIN" ] || die "contributors-cors: missing ACAO header"
say "contributors-cors"

say "admin staging validation complete"
