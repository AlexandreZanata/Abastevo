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
# Prints step results only. Exits non-zero on the first failure.
set +x
set -uo pipefail

say() { printf 'PASS: %s\n' "$1"; }
die() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

for tool in curl python3 date; do
  command -v "$tool" >/dev/null 2>&1 || die "missing required tool: $tool"
done

: "${ANPFUEL_ADMIN_KEY:?ANPFUEL_ADMIN_KEY is required}"
: "${ANPFUEL_STAGING_URL:?ANPFUEL_STAGING_URL is required}"
ADMIN_ORIGIN="${ANPFUEL_ADMIN_ORIGIN:-$ANPFUEL_STAGING_URL}"
PUBLIC_ORIGIN="${ANPFUEL_PUBLIC_ORIGIN:-https://abastevo.com.br}"
BASE="${ANPFUEL_STAGING_URL%/}"

case "$BASE" in
  http://*|https://*) ;;
  *) die "ANPFUEL_STAGING_URL must be an http(s) origin" ;;
esac

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

# api <method> <path>: performs the request with the session cookie when set.
# Sets RESP_STATUS and RESP_BODY. Bodies travel through stdin, never argv.
api() {
  local method="$1" path="$2" body="${3:-}"
  local args=(-s -w '\n%{http_code}' -X "$method" "$BASE$path"
    -H 'Content-Type: application/json' -H "Origin: $ADMIN_ORIGIN")
  if [ -n "${ADMIN_COOKIE:-}" ]; then
    args+=(-H "Cookie: __Host-abv_admin=$ADMIN_COOKIE")
  fi
  if [ -n "${ADMIN_CSRF:-}" ]; then
    args+=(-H "X-Admin-CSRF: $ADMIN_CSRF" -H "Idempotency-Key: $(new_uuid)")
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
ADMIN_COOKIE=$(printf '%s' "$exchange" | awk 'tolower($1)=="set-cookie:"{split($2,a,";");split(a[1],b,"=");print b[2];exit}')
[ "${#ADMIN_COOKIE}" -ge 20 ] || die "login: missing session cookie"
ADMIN_CSRF=$(jget 'd["csrf_token"]')
[ "${#ADMIN_CSRF}" -ge 20 ] || die "login: missing csrf token"

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
  -H "Cookie: __Host-abv_admin=$ADMIN_COOKIE" \
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
  logged=$(curl -s -w '\n%{http_code}' -X POST "$BASE/v1/accounts/keys/login" \
    -H 'Content-Type: application/json' --data-binary @- <<<"$(ACCOUNT_KEY="$ACCOUNT_KEY" python3 -c 'import json,os; print(json.dumps({"account_key": os.environ["ACCOUNT_KEY"]}))')")
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
[ "$(jget 'd["grant"]["state"]')" = "ACTIVE" ] || die "grant: not active"
GRANT_ID=$(jget 'd["grant"]["id"]')

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
status=$(curl -s -w '\n%{http_code}' -X POST "$BASE/v1/premium/status" \
  -H 'Content-Type: application/json' --data-binary @- <<<"$status_body")
RESP_STATUS=$(printf '%s' "$status" | tail -n 1)
RESP_BODY=$(printf '%s' "$status" | sed '$d')
expect "status-cleared" "200" "$RESP_STATUS"
[ "$(jget 'any(t.get("source")=="admin" for t in d["tiers"])')" = "False" ] || die "status-cleared: admin tier still present"

# 10. Logout, then cookie reuse is 401.
api DELETE /v1/admin/session '{"reason":"staging validation logout"}'
expect "logout" "204" "$RESP_STATUS"
reuse=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/v1/admin/session" -H "Cookie: __Host-abv_admin=$ADMIN_COOKIE")
expect "reuse-cookie" "401" "$reuse"

# 11. Public contributors list with CORS for the landing origin.
cors=$(curl -s -D - -o /dev/null "$BASE/v1/contributors" -H "Origin: $PUBLIC_ORIGIN")
[ "$(printf '%s' "$cors" | head -n 1 | awk '{print $2}')" = "200" ] || die "contributors-cors: bad status"
[ "$(printf '%s' "$cors" | awk -F': ' 'tolower($1)=="access-control-allow-origin"{print $2}' | tr -d '\r')" = "$PUBLIC_ORIGIN" ] || die "contributors-cors: missing ACAO header"
say "contributors-cors"

say "admin staging validation complete"
