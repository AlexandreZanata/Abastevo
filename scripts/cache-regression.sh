#!/usr/bin/env bash
# P08-T06 cache/header/signature regression suite.
# Read-only client asserting the edge allowlist matrix against a live
# origin or staging edge. No writes, no containers, no provisioning.
# Usage: BASE_URL=https://staging.example.invalid bash scripts/cache-regression.sh
set -euo pipefail

BASE="${BASE_URL:-http://127.0.0.1:8080}"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1${2:+ ($2)}" >&2; FAIL=$((FAIL + 1)); }

STATION="d6c74c23-63db-4c24-a2e5-408cb23bad26"

get() { curl -sS -D - -o /tmp/cache-reg-body "$@" 2>/dev/null; }
header() { grep -i "^$1:" /tmp/cache-reg-headers | tr -d '\r' | sed "s/^[^:]*:[[:space:]]*//I" | head -n 1; }

check_cache() {
    local name="$1" url="$2" want="$3"
    shift 3
    get "$@" "$url" > /tmp/cache-reg-headers
    local cc
    cc="$(header Cache-Control)"
    if [[ "$cc" == "$want" ]]; then
        pass "$name cache = $want"
    else
        fail "$name cache" "got [$cc] want [$want]"
    fi
}

# Public allowlist matrix.
check_cache "search" "$BASE/v1/stations?q=al&limit=1" "public, max-age=60"
check_cache "detail" "$BASE/v1/stations/$STATION" "public, max-age=30, s-maxage=60"
check_cache "prices" "$BASE/v1/stations/$STATION/prices" "public, max-age=60"
check_cache "official-prices" "$BASE/v1/stations/$STATION/official-prices?limit=1" "public, max-age=60"

# ETag round-trip on a cacheable read.
get "$BASE/v1/stations/$STATION" > /tmp/cache-reg-headers
ETAG="$(header ETag)"
if [[ -n "$ETAG" ]]; then
    pass "detail carries ETag"
    CODE="$(curl -sS -o /dev/null -w '%{http_code}' -H "If-None-Match: $ETAG" "$BASE/v1/stations/$STATION")"
    if [[ "$CODE" == "304" ]]; then
        pass "etag replays 304"
    else
        fail "etag replays 304" "got $CODE"
    fi
else
    fail "detail carries ETag" "missing"
fi

# Private surface never shared-cached.
check_cache "nearby" "$BASE/v1/stations/nearby?lat=-15.8&lon=-47.9&radius_m=3000&limit=1" "no-store"

# Identity invariance on a public read (shared-cache safety).
plain="$(curl -sS "$BASE/v1/stations/$STATION")"
authed="$(curl -sS -H "Authorization: Signature sig1=:fictitious:" -H "Cookie: session=fictitious" "$BASE/v1/stations/$STATION")"
if [[ "$plain" == "$authed" ]]; then
    pass "public body invariant to identity headers"
else
    fail "public body invariant to identity headers"
fi
vary="$(curl -sS -D - -o /dev/null "$BASE/v1/stations/$STATION" | tr -d '\r')"
if printf '%s' "$vary" | grep -qiE '^vary:.*(authorization|cookie)'; then
    fail "no Vary on identity"
else
    pass "no Vary on identity"
fi

# Error envelopes never cached.
curl -sS -D /tmp/cache-reg-err-headers -o /tmp/cache-reg-body -w '%{http_code}' "$BASE/v1/stations?q=al&limit=999" > /tmp/cache-reg-code
CODE="$(cat /tmp/cache-reg-code)"
CC="$(grep -i '^cache-control:' /tmp/cache-reg-err-headers | tr -d '\r' | sed 's/^[^:]*:[[:space:]]*//I')"
if [[ "$CODE" == "400" && "$CC" == "no-store" ]]; then
    pass "errors rejected with no-store"
else
    fail "errors rejected with no-store" "code=$CODE cache=[$CC]"
fi

echo "cache-regression: $PASS passed, $FAIL failed against $BASE"
rm -f /tmp/cache-reg-body /tmp/cache-reg-headers /tmp/cache-reg-err-headers /tmp/cache-reg-code
[[ "$FAIL" -eq 0 ]]
