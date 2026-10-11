#!/usr/bin/env bash
# CS05B staging-only adversarial pass over the helpfulness command set.
# Runs stateless probes (no account, no storage, no other-user data):
# anonymous writes, malformed/oversized bodies, unknown targets and keys,
# tampered proof and wrong methods. Authenticated adversarial coverage
# (replay, self-vote, stale revision, quota) lives in the Go e2e suite:
#   go test -race -count=1 -tags=integration \
#     ./internal/modules/community/adapters/http/ -run 'TestHelpfulness'
#
# Usage:
#   scripts/helpfulness-adversarial.sh --target local   # 127.0.0.1:PORT only
#   scripts/helpfulness-adversarial.sh --target staging --base-url https://staging.example.invalid [--yes]
#
# Staging execution requires the authorized staging window and an
# operator-provided base URL; production hosts are never probed.
set -euo pipefail

TARGET=""
BASE_URL=""
ASSUME_YES=0

while [ $# -gt 0 ]; do
  case "$1" in
    --target) TARGET="${2:-}"; shift 2 ;;
    --base-url) BASE_URL="${2:-}"; shift 2 ;;
    --yes) ASSUME_YES=1; shift ;;
    --help|-h)
      sed -n '2,16p' "$0"
      exit 0
      ;;
    *) echo "unknown flag: $1 (see --help)" >&2; exit 2 ;;
  esac
done

if [ "$TARGET" != "local" ] && [ "$TARGET" != "staging" ]; then
  echo "refusing: pass --target local|staging" >&2
  exit 2
fi

if [ "$TARGET" = "local" ]; then
  BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
  case "$BASE_URL" in
    http://127.0.0.1:*|http://localhost:*|http://\[::1\]:*) ;;
    *) echo "refusing: local target must be loopback (got $BASE_URL)" >&2; exit 2 ;;
  esac
else
  if [ -z "$BASE_URL" ]; then
    echo "refusing: staging needs --base-url https://<staging-host>" >&2
    exit 2
  fi
  case "$BASE_URL" in
    https://*) ;;
    *) echo "refusing: staging needs https (got $BASE_URL)" >&2; exit 2 ;;
  esac
  host="$(printf '%s' "$BASE_URL" | sed -e 's#^https://##' -e 's#/.*##' -e 's#:.*##')"
  case "$host" in
    127.*|localhost|::1|"") echo "refusing: staging host must not be loopback ($host)" >&2; exit 2 ;;
  esac
  if [ "$ASSUME_YES" != "1" ]; then
    printf 'probe staging %s with stateless adversarial cases? [y/N] ' "$BASE_URL"
    read -r answer
    [ "$answer" = "y" ] || { echo "aborted"; exit 1; }
  fi
fi

OBS="00000000-0000-0000-0000-000000000000"
PASS=0
FAIL=0

# probe <name> <expected-status> <curl-args...>: stateless, no credentials.
probe() {
  name="$1"; want="$2"; shift 2
  code="$(curl -sS -o /tmp/helpadv.body -w '%{http_code}' --max-time 20 "$@" || echo CURLFAIL)"
  if [ "$code" = "$want" ]; then
    PASS=$((PASS + 1))
    echo "PASS $name -> $code"
  else
    FAIL=$((FAIL + 1))
    echo "FAIL $name -> $code (want $want)"
    head -c 300 /tmp/helpadv.body 2>/dev/null; echo
  fi
}

BIG="$(python3 -c 'print("y"*70000)')"
BAD_UTF8="$(printf '{"choice":"HELPFUL\xff","decision_sequence":2,"client_submission_id":"s"}')"

probe "anon-cast-401"            401 -X POST "$BASE_URL/v1/observations/$OBS/helpfulness" \
  -H 'Content-Type: application/json' -d '{"choice":"HELPFUL","decision_sequence":2,"client_submission_id":"s"}'
probe "anon-remove-401"          401 -X POST "$BASE_URL/v1/observations/$OBS/helpfulness/remove" \
  -H 'Content-Type: application/json' -d '{"decision_sequence":2}'
probe "unsigned-proof-401"       401 -X POST "$BASE_URL/v1/observations/$OBS/helpfulness" \
  -H 'Content-Type: application/json' \
  -H 'Signature: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' \
  -H 'Signature-Keyid: fp:unknown' -H 'Signature-Created: 1' -H 'Signature-Expires: 2' -H 'Signature-Nonce: n' \
  -d '{"choice":"HELPFUL","decision_sequence":2,"client_submission_id":"s"}'
probe "malformed-body-400"       400 -X POST "$BASE_URL/v1/observations/$OBS/helpfulness" \
  -H 'Content-Type: application/json' -d '{"choice":'
probe "unknown-field-400"        400 -X POST "$BASE_URL/v1/observations/$OBS/helpfulness" \
  -H 'Content-Type: application/json' -d '{"choice":"HELPFUL","decision_sequence":2,"client_submission_id":"s","x":1}'
probe "bad-choice-400"           400 -X POST "$BASE_URL/v1/observations/$OBS/helpfulness" \
  -H 'Content-Type: application/json' -d '{"choice":"VALID","decision_sequence":2,"client_submission_id":"s"}'
probe "zero-sequence-400"        400 -X POST "$BASE_URL/v1/observations/$OBS/helpfulness" \
  -H 'Content-Type: application/json' -d '{"choice":"HELPFUL","decision_sequence":0,"client_submission_id":"s"}'
probe "oversized-400"            400 -X POST "$BASE_URL/v1/observations/$OBS/helpfulness" \
  -H 'Content-Type: application/json' -d "{\"choice\":\"HELPFUL\",\"decision_sequence\":2,\"client_submission_id\":\"$BIG\"}"
probe "bad-utf8-400"             400 -X POST "$BASE_URL/v1/observations/$OBS/helpfulness" \
  -H 'Content-Type: application/json' --data-binary "$BAD_UTF8"
probe "tally-bad-sequence-400"   400 "$BASE_URL/v1/observations/$OBS/helpfulness?decision_sequence=nope"
probe "remove-wrong-method-405"    405 -X DELETE "$BASE_URL/v1/observations/$OBS/helpfulness/remove"

echo "pass=$PASS fail=$FAIL"
[ "$FAIL" = "0" ]
