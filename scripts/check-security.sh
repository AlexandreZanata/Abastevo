#!/usr/bin/env bash
# P08-T08 security and dependency release review gate.
#
# Static review (no provisioning, no secret output) plus the pinned
# vulnerability scan and the focused adversarial suites that already
# prove replay/IDOR/SSRF/oversize/role/secret behavior in their owning
# modules. This gate composes them; it never redefines product behavior.
#
# Usage:
#   bash scripts/check-security.sh [--static-only]
#
# Mutant testing overrides (environment only, never committed):
#   SECURITY_COMPOSE_STAGING / SECURITY_COMPOSE_PROD / SECURITY_CADDY /
#   SECURITY_DOCKERFILE / SECURITY_ENV_STAGING / SECURITY_ENV_PROD /
#   SECURITY_BACKEND_ENV_EXAMPLE / SECURITY_GOMOD / SECURITY_GOSUM
#
# Failure output carries rule names and counts only, never values, so
# operator secrets cannot leak through this gate.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

MODE="full"
if [[ "${1:-}" == "--static-only" ]]; then
    MODE="static-only"
elif [[ -n "${1:-}" ]]; then
    echo "usage: check-security.sh [--static-only]" >&2
    exit 2
fi

COMPOSE_STAGING="${SECURITY_COMPOSE_STAGING:-infra/compose.staging.yml}"
COMPOSE_PROD="${SECURITY_COMPOSE_PROD:-infra/compose.prod.yml}"
CADDYFILE="${SECURITY_CADDY:-infra/caddy/Caddyfile}"
DOCKERFILE="${SECURITY_DOCKERFILE:-infra/docker/Dockerfile}"
ENV_STAGING="${SECURITY_ENV_STAGING:-infra/env.staging.example}"
ENV_PROD="${SECURITY_ENV_PROD:-infra/env.prod.example}"
BACKEND_ENV_EXAMPLE="${SECURITY_BACKEND_ENV_EXAMPLE:-backend/.env.example}"
GOMOD="${SECURITY_GOMOD:-backend/go.mod}"
GOSUM="${SECURITY_GOSUM:-backend/go.sum}"

FAIL=0
refuse() {
    echo "REFUSED [security]: $1" >&2
    FAIL=1
}

for f in "$COMPOSE_STAGING" "$COMPOSE_PROD" "$CADDYFILE" "$DOCKERFILE" \
    "$ENV_STAGING" "$ENV_PROD" "$BACKEND_ENV_EXAMPLE" "$GOMOD" "$GOSUM"; do
    [[ -f "$f" ]] || refuse "file missing: $f"
done

# --- Go dependency pins (reviewed set; additions need license review) ---
if [[ -f "$GOMOD" ]]; then
    grep -q '^go 1\.27$' "$GOMOD" || refuse "go.mod must pin go 1.27"
    grep -q '^toolchain go1\.27\.1$' "$GOMOD" || refuse "go.mod must pin toolchain go1.27.1"
    grep -q 'github.com/go-chi/chi/v5 v5\.3\.2' "$GOMOD" || refuse "go.mod must pin chi v5.3.2"
    grep -q 'github.com/jackc/pgx/v5 v5\.11\.0' "$GOMOD" || refuse "go.mod must pin pgx v5.11.0"
    grep -q 'github.com/getkin/kin-openapi v0\.149\.0' "$GOMOD" || refuse "go.mod must pin kin-openapi v0.149.0"
    if grep -qE '^replace ' "$GOMOD"; then refuse "go.mod must carry no replace directives"; fi
fi
if [[ -f "$GOSUM" ]]; then
    [[ -s "$GOSUM" ]] || refuse "go.sum is empty"
fi

# --- Image provenance: no floating tags, digests pinned, non-root ---
if [[ -f "$DOCKERFILE" ]]; then
    grep -qE '^ARG GOLANG_IMAGE=.*@sha256:[0-9a-f]{64}' "$DOCKERFILE" \
        || refuse "dockerfile builder image not pinned by digest"
    grep -qE '^ARG RUNTIME_IMAGE=.*@sha256:[0-9a-f]{64}' "$DOCKERFILE" \
        || refuse "dockerfile runtime image not pinned by digest"
    if grep -q ':latest' "$DOCKERFILE"; then refuse "dockerfile uses floating :latest"; fi
    NONROOT_COUNT="$(grep -c '^USER 65532:65532' "$DOCKERFILE" || true)"
    [[ "$NONROOT_COUNT" -ge 3 ]] || refuse "dockerfile must run all 3 roles as 65532:65532"
    if grep -qiE '^[[:space:]]*(ENV|ARG).*(PASSWORD|SECRET_ACCESS|PRIVATE_KEY|SK_LIVE)' "$DOCKERFILE"; then
        refuse "dockerfile carries credential build args"
    fi
    if grep -q '\.env' "$DOCKERFILE"; then refuse "dockerfile references .env files"; fi
fi

# --- Staging/prod topology (static; docker render belongs to check-infra) ---
for compose in "$COMPOSE_STAGING" "$COMPOSE_PROD"; do
    [[ -f "$compose" ]] || continue
    if grep -q ':latest' "$compose"; then refuse "$(basename "$compose") uses floating :latest"; fi
    grep -q 'postgis/postgis:18-3\.6@sha256:[0-9a-f]\{64\}' "$compose" \
        || refuse "$(basename "$compose") postgis image not pinned by digest"
    grep -q 'caddy:2\.10\.2-alpine@sha256:[0-9a-f]\{64\}' "$compose" \
        || refuse "$(basename "$compose") caddy image not pinned by digest"
    if grep -qE 'anpfuel-(api|worker|migrate):latest' "$compose"; then
        refuse "$(basename "$compose") role image uses latest"
    fi
    for port in '5432:' '8080:' '9090:' '5434'; do
        if grep -qF "$port" "$compose"; then refuse "$(basename "$compose") exposes origin port ${port%:}"; fi
    done
    for marker in 'anpfuel-pgdata-dev' 'anpfuel/anpfuel@127.0.0.1'; do
        if grep -qF "$marker" "$compose"; then refuse "$(basename "$compose") leaks dev default"; fi
    done
done
grep -qF 'anpfuel-pgdata-staging' "$COMPOSE_STAGING" 2>/dev/null \
    || refuse "staging volume name absent"
grep -qF 'anpfuel-pgdata-prod' "$COMPOSE_PROD" 2>/dev/null \
    || refuse "prod volume name absent"

# --- Edge rules (mirror of the infra gate; security-relevant subset) ---
if [[ -f "$CADDYFILE" ]]; then
    grep -qE '^[[:space:]]*admin off' "$CADDYFILE" || refuse "caddy admin not off"
    grep -qE '^[[:space:]]*tls \{' "$CADDYFILE" || refuse "caddy TLS block missing"
    grep -q 'trusted_proxies static private_ranges' "$CADDYFILE" \
        || refuse "caddy trusted_proxies not restricted to private_ranges"
    if grep -q '0\.0\.0\.0/0' "$CADDYFILE"; then refuse "caddy trusts the open internet"; fi
    grep -qE '^[[:space:]]*request delete$' "$CADDYFILE" || refuse "caddy request privacy filter missing"
    grep -qE '^[[:space:]]*resp_headers delete$' "$CADDYFILE" || refuse "caddy response-header privacy filter missing"
    if grep -vE '^[[:space:]]*#' "$CADDYFILE" | grep -qi 'metrics'; then
        refuse "caddy exposes a metrics path"
    fi
    if grep -q ':2019' "$CADDYFILE"; then refuse "caddy admin port exposed"; fi
    grep -q 'request_header @shared_cacheable -Cookie' "$CADDYFILE" \
        || refuse "caddy keeps Cookie on shared-cacheable paths"
    grep -q 'request_header @shared_cacheable -Authorization' "$CADDYFILE" \
        || refuse "caddy keeps Authorization on shared-cacheable paths"
fi

# --- Environment templates: placeholders only, never secrets ---
for example in "$ENV_STAGING" "$ENV_PROD"; do
    [[ -f "$example" ]] || continue
    grep -q 'REPLACE_ME' "$example" || refuse "$(basename "$example") has no REPLACE_ME placeholders"
    for marker in 'anpfuel/anpfuel' '127.0.0.1' 'changeme' 'password123' 'secret123'; do
        if grep -qiF "$marker" "$example"; then refuse "dev/weak secret in $(basename "$example")"; fi
    done
    if grep -qiE '^(ANPFUEL_DB_PASSWORD|POSTGRES_PASSWORD)=(anpfuel|postgres|password|changeme|secret)[[:space:]]*$' "$example"; then
        refuse "weak database password in $(basename "$example")"
    fi
done
if [[ -f "$BACKEND_ENV_EXAMPLE" ]]; then
    grep -qi 'never commit real' "$BACKEND_ENV_EXAMPLE" \
        || refuse "backend .env.example lost its no-secrets notice"
fi

# --- High-confidence secret patterns in security-relevant files ---
SECRET_PATTERN='ghp_[A-Za-z0-9]{20,}|github_pat_|BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY|sk_live_|AKIA[0-9A-Z]{16}|xox[bap]-'
for f in "$COMPOSE_STAGING" "$COMPOSE_PROD" "$CADDYFILE" "$DOCKERFILE" \
    "$ENV_STAGING" "$ENV_PROD" "$BACKEND_ENV_EXAMPLE"; do
    [[ -f "$f" ]] || continue
    if grep -nE "$SECRET_PATTERN" "$f" >/dev/null 2>&1; then
        refuse "secret-shaped pattern in $(basename "$f")"
    fi
done

# --- Tracked secret scan (same scanner as the fast gate) ---
bash scripts/scan-secrets.sh >/dev/null || { refuse "tracked secret scan failed"; }

if [[ "$FAIL" -ne 0 ]]; then
    exit 1
fi
echo "security static review ok"

if [[ "$MODE" == "static-only" ]]; then
    echo "security review ok [mode=static-only]"
    exit 0
fi

echo "== vulnerability scan =="
(cd backend && govulncheck ./...)

echo "== focused adversarial suites (replay/IDOR/SSRF/oversize/role/secret) =="
(cd backend && go test -count=1 \
    ./internal/platform/config/... \
    ./internal/platform/telemetry/... \
    ./internal/platform/httpserver/... \
    ./internal/modules/identity/profile/... \
    ./internal/modules/identity/domain/... \
    ./internal/modules/identity/adapters/auth/... \
    ./internal/modules/identity/application/... \
    ./internal/modules/official/adapters/source/... \
    ./internal/modules/evidence/adapters/storage/... \
    ./internal/modules/evidence/adapters/media/... \
    ./internal/modules/evidence/domain/... \
    ./internal/modules/community/adapters/http/... \
    ./internal/modules/moderation/domain/... \
    ./internal/modules/moderation/application/... \
    ./internal/modules/privacy/domain/... \
    ./internal/modules/privacy/application/...)

echo "security review ok [mode=full]"
