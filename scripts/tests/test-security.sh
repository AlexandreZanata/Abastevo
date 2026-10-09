#!/usr/bin/env bash
# P08-T08 harness: security gate selection and failure behavior.
# Proves the gate accepts the reviewed tree and refuses one fault per
# class (public origin ports, floating images, open proxies, dev
# secrets, unpinned/root images, dependency replace). Live scans run
# once against the real tree; mutants use --static-only for speed.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

PASS=0
FAIL=0

assert_pass() {
    local name="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        echo "PASS: $name"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $name (expected success)" >&2
        FAIL=$((FAIL + 1))
    fi
}

assert_fail() {
    local name="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        echo "FAIL: $name (expected refusal)" >&2
        FAIL=$((FAIL + 1))
    else
        echo "PASS: $name"
        PASS=$((PASS + 1))
    fi
}

# Reviewed tree passes the static review.
assert_pass "reviewed static tree accepted" bash scripts/check-security.sh --static-only

# Mutant staging helper: copies the reviewed security files into a temp
# root so each fault is isolated to one class (RED proof per class).
mutant_root() {
    local dir="$1"
    rm -rf "$dir"
    mkdir -p "$dir/infra/caddy" "$dir/infra/docker" "$dir/backend"
    cp infra/compose.staging.yml "$dir/infra/compose.staging.yml"
    cp infra/compose.prod.yml "$dir/infra/compose.prod.yml"
    cp infra/caddy/Caddyfile "$dir/infra/caddy/Caddyfile"
    cp infra/docker/Dockerfile "$dir/infra/docker/Dockerfile"
    cp infra/env.staging.example "$dir/infra/env.staging.example"
    cp infra/env.prod.example "$dir/infra/env.prod.example"
    cp backend/.env.example "$dir/backend/.env.example"
    cp backend/go.mod "$dir/backend/go.mod"
    cp backend/go.sum "$dir/backend/go.sum"
}

with_overrides() {
    local dir="$1"
    export SECURITY_COMPOSE_STAGING="$dir/infra/compose.staging.yml"
    export SECURITY_COMPOSE_PROD="$dir/infra/compose.prod.yml"
    export SECURITY_CADDY="$dir/infra/caddy/Caddyfile"
    export SECURITY_DOCKERFILE="$dir/infra/docker/Dockerfile"
    export SECURITY_ENV_STAGING="$dir/infra/env.staging.example"
    export SECURITY_ENV_PROD="$dir/infra/env.prod.example"
    export SECURITY_BACKEND_ENV_EXAMPLE="$dir/backend/.env.example"
    export SECURITY_GOMOD="$dir/backend/go.mod"
    export SECURITY_GOSUM="$dir/backend/go.sum"
}

clear_overrides() {
    unset SECURITY_COMPOSE_STAGING SECURITY_COMPOSE_PROD SECURITY_CADDY \
        SECURITY_DOCKERFILE SECURITY_ENV_STAGING SECURITY_ENV_PROD \
        SECURITY_BACKEND_ENV_EXAMPLE SECURITY_GOMOD SECURITY_GOSUM || true
}

# 1. Public database port in staging compose.
M1="$(mktemp -d)"
mutant_root "$M1"
python3 - "$M1/infra/compose.staging.yml" <<'EOF'
import sys
p = sys.argv[1]
s = open(p).read()
anchor = "    # No published ports: the database lives on the private back\n"
assert anchor in s
s = s.replace(anchor, anchor + "    ports:\n      - \"5432:5432\"\n", 1)
open(p, "w").write(s)
EOF
with_overrides "$M1"
assert_fail "public db port refused" bash scripts/check-security.sh --static-only
clear_overrides

# 2. Floating image tag.
M2="$(mktemp -d)"
mutant_root "$M2"
sed -i 's|image: caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d|image: caddy:latest|' "$M2/infra/compose.staging.yml"
with_overrides "$M2"
assert_fail "floating image refused" bash scripts/check-security.sh --static-only
clear_overrides

# 3. Open trusted_proxies.
M3="$(mktemp -d)"
mutant_root "$M3"
sed -i 's/trusted_proxies static private_ranges/trusted_proxies 0.0.0.0\/0/' "$M3/infra/caddy/Caddyfile"
with_overrides "$M3"
assert_fail "open trusted_proxies refused" bash scripts/check-security.sh --static-only
clear_overrides

# 4. Dev secret smuggled into the staging example.
M4="$(mktemp -d)"
mutant_root "$M4"
sed -i 's/^ANPFUEL_DB_PASSWORD=.*/ANPFUEL_DB_PASSWORD=anpfuel/' "$M4/infra/env.staging.example"
with_overrides "$M4"
assert_fail "dev secret refused" bash scripts/check-security.sh --static-only
clear_overrides

# 5. Unpinned builder image (digest stripped).
M5="$(mktemp -d)"
mutant_root "$M5"
sed -i 's|golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61|golang:1.27.2-bookworm|' "$M5/infra/docker/Dockerfile"
with_overrides "$M5"
assert_fail "unpinned builder refused" bash scripts/check-security.sh --static-only
clear_overrides

# 6. Root runtime user.
M6="$(mktemp -d)"
mutant_root "$M6"
sed -i 's/^USER 65532:65532/USER root/' "$M6/infra/docker/Dockerfile"
with_overrides "$M6"
assert_fail "root runtime refused" bash scripts/check-security.sh --static-only
clear_overrides

# 7. Dependency replace directive (unreviewed source).
M7="$(mktemp -d)"
mutant_root "$M7"
printf '\nreplace example.com/unreviewed => ../unreviewed\n' >> "$M7/backend/go.mod"
with_overrides "$M7"
assert_fail "go.mod replace refused" bash scripts/check-security.sh --static-only
clear_overrides

rm -rf "$M1" "$M2" "$M3" "$M4" "$M5" "$M6" "$M7"

# Live gates on the real tree: vulnerability scan clean and one focused
# adversarial package executes (proves the selection is executable, not
# a compile-only claim).
assert_pass "govulncheck clean" bash -c 'cd backend && govulncheck ./...'
assert_pass "adversarial selection executes" bash -c 'cd backend && go test -count=1 ./internal/platform/config/...'

echo "security: $PASS passed, $FAIL failed"
[[ "$FAIL" -eq 0 ]]
