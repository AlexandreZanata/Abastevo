#!/usr/bin/env bash
# P08-T01 infrastructure gate: staging/production topology review.
# Validates Compose render plus edge rules without provisioning anything.
# Fails loudly on dev defaults, public database/API ports, unpinned
# images, missing TLS/trust rules or secret-shaped examples. Never
# prints rendered values: failure output carries counts and rule names
# only, so operator secrets cannot leak through this gate.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ENV="${1:-}"
COMPOSE_OVERRIDE="${2:-}"
CADDY_OVERRIDE="${3:-}"
EXAMPLE_OVERRIDE="${4:-}"

if [[ "$ENV" != "staging" && "$ENV" != "prod" ]]; then
    echo "usage: check-infra-config.sh <staging|prod> [compose-file] [caddy-file] [env-example]" >&2
    exit 2
fi

COMPOSE="${COMPOSE_OVERRIDE:-infra/compose.${ENV}.yml}"
CADDYFILE="${CADDY_OVERRIDE:-infra/caddy/Caddyfile}"
EXAMPLE="${EXAMPLE_OVERRIDE:-infra/env.${ENV}.example}"
VOLUME="anpfuel-pgdata-${ENV}"

FAIL=0
refuse() {
    echo "REFUSED [$ENV]: $1" >&2
    FAIL=1
}

[[ -f "$COMPOSE" ]] || { refuse "compose file missing: $COMPOSE"; }
[[ -f "$CADDYFILE" ]] || { refuse "Caddyfile missing: $CADDYFILE"; }
[[ -f "$EXAMPLE" ]] || { refuse "env example missing: $EXAMPLE"; }

# Dummy non-secret interpolation values for render validation. Real
# operator secrets never enter this script.
TMP_ENV="$(mktemp)"
trap 'rm -f "$TMP_ENV"' EXIT
touch "$TMP_ENV"
export ANPFUEL_RELEASE=gate-check
export ANPFUEL_ENV_FILE="$TMP_ENV"
export ANPFUEL_DB_USER=gatecheck_user
export ANPFUEL_DB_NAME=gatecheck_db
export ANPFUEL_DB_PASSWORD=gatecheck_password_value
export CADDY_DOMAIN="gatecheck.example.invalid"
export CADDY_TLS_EMAIL="gatecheck@example.invalid"

RENDERED=""
if [[ "$FAIL" -eq 0 ]]; then
    if ! RENDERED="$(docker compose -f "$COMPOSE" config 2>/dev/null)"; then
        refuse "compose config does not parse"
    fi
fi

if [[ -n "$RENDERED" ]]; then
    # Only Caddy publishes ports (80+443): any other published port,
    # especially the database, fails.
    PUBLISHED="$(printf '%s' "$RENDERED" | grep -c 'published:' || true)"
    [[ "$PUBLISHED" -eq 2 ]] || refuse "published port count is $PUBLISHED, want exactly 80+443 from caddy"
    # Dev defaults must not survive outside development.
    for marker in 'anpfuel/anpfuel' '127.0.0.1:5434' 'anpfuel-pgdata-dev'; do
        if printf '%s' "$RENDERED" | grep -qF "$marker"; then
            refuse "dev default leaked: ${marker%%:*}"
        fi
    done
    # No floating tags: digests for third-party images, immutable
    # release tags for built roles.
    if printf '%s' "$RENDERED" | grep -q 'latest'; then
        refuse "floating :latest image reference"
    fi
    while IFS= read -r line; do
        if ! printf '%s' "$line" | grep -qE '@sha256:[0-9a-f]{64}|anpfuel-(api|worker|migrate):[^ ]+'; then
            refuse "unpinned image reference"
            break
        fi
    done < <(printf '%s' "$RENDERED" | grep -E '^\s*image: ' || true)
    # Persistent per-environment volume, never the dev one.
    printf '%s' "$RENDERED" | grep -qF "$VOLUME" || refuse "expected volume $VOLUME absent"
fi

# Edge rules on the Caddyfile source (placeholders intact).
grep -qE '^[[:space:]]*admin off' "$CADDYFILE" || refuse "caddy admin not off"
grep -qE '^[[:space:]]*tls \{' "$CADDYFILE" || refuse "caddy TLS block missing"
grep -q 'trusted_proxies static private_ranges' "$CADDYFILE" || refuse "caddy trusted_proxies not restricted to private_ranges"
if grep -q '0\.0\.0\.0/0' "$CADDYFILE"; then refuse "caddy trusts the open internet"; fi
    grep -qE '^[[:space:]]*request delete$' "$CADDYFILE" || refuse "caddy request privacy filter missing"
    grep -qE '^[[:space:]]*resp_headers delete$' "$CADDYFILE" || refuse "caddy response-header privacy filter missing"
# Metrics check skips `#` comments: only a routed path counts.
if grep -vE '^[[:space:]]*#' "$CADDYFILE" | grep -qi 'metrics'; then refuse "caddy exposes a metrics path"; fi
if grep -q ':2019' "$CADDYFILE"; then refuse "caddy admin port exposed"; fi
grep -q '{$CADDY_DOMAIN}' "$CADDYFILE" || refuse "caddy domain not parameterized"
# Shared-cacheable paths must strip identity so edge caches key on
# method+URL only (invariance proven in the http cache tests).
grep -q 'request_header @shared_cacheable -Cookie' "$CADDYFILE" || refuse "caddy keeps Cookie on shared-cacheable paths"
grep -q 'request_header @shared_cacheable -Authorization' "$CADDYFILE" || refuse "caddy keeps Authorization on shared-cacheable paths"

# Examples carry placeholders, never secret-shaped values.
for marker in 'anpfuel/anpfuel' '127.0.0.1' 'changeme' 'password123' 'secret123'; do
    if grep -qiF "$marker" "$EXAMPLE"; then refuse "dev/weak secret in $EXAMPLE"; fi
done
# A bare well-known password on a credential variable is a dev
# secret even without the user pair.
if grep -qiE '^(ANPFUEL_DB_PASSWORD|POSTGRES_PASSWORD)=(anpfuel|postgres|password|changeme|secret)[[:space:]]*$' "$EXAMPLE"; then
    refuse "weak database password in $EXAMPLE"
fi
grep -q 'REPLACE_ME' "$EXAMPLE" || refuse "$EXAMPLE has no REPLACE_ME placeholders"

if [[ "$FAIL" -ne 0 ]]; then
    exit 1
fi
echo "infra config ok [$ENV]"
