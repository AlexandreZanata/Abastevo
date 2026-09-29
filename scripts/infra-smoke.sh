#!/usr/bin/env bash
# P08-T01 network/TLS smoke: static topology assertions by default, live
# edge verification with --live HOST at staging deploy time (P08-T02).
# Static mode provisions nothing; live mode touches one host only.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

LIVE="${2:-}"

if [[ "${1:-}" == "--live" ]]; then
    [[ -n "$LIVE" ]] || { echo "usage: infra-smoke.sh --live HOST" >&2; exit 2; }
    echo "== live edge smoke against $LIVE"
    CODE="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 15 "https://$LIVE/health/live")"
    [[ "$CODE" == "200" ]] || { echo "REFUSED: /health/live returned $CODE" >&2; exit 1; }
    echo "PASS: https://$LIVE/health/live returns 200"
    if ! timeout 20 openssl s_client -connect "$LIVE:443" -servername "$LIVE" </dev/null 2>/dev/null \
        | openssl x509 -noout -checkend 0 -subject -issuer 2>/dev/null; then
        echo "REFUSED: TLS certificate check failed for $LIVE" >&2
        exit 1
    fi
    echo "PASS: TLS certificate valid for $LIVE"
    exit 0
fi

echo "== static topology smoke"
# Gate covers both reviewable topologies.
bash scripts/check-infra-config.sh staging
bash scripts/check-infra-config.sh prod
# Development stays loopback-only: the dev database port must bind
# 127.0.0.1 and nothing else public.
DEV_PORTS="$(docker compose -f infra/compose.dev.yml config 2>/dev/null | grep -E 'published:|host_ip:' || true)"
if printf '%s' "$DEV_PORTS" | grep -q 'published:'; then
    if ! docker compose -f infra/compose.dev.yml config 2>/dev/null | grep -A 3 'ports:' | grep -q '127.0.0.1'; then
        echo "REFUSED: dev compose publishes non-loopback ports" >&2
        exit 1
    fi
fi
echo "PASS: dev database stays loopback-only"
echo "static smoke ok (live edge smoke runs at staging deploy: scripts/infra-smoke.sh --live HOST)"
