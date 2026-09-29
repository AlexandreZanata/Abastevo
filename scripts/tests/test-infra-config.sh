#!/usr/bin/env bash
# P08-T01 harness: infra gate selection and failure behavior.
# Proves the gate passes reviewable configs and refuses dev secrets,
# public database ports, floating images and untrusted proxy rules.
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

# Reviewable configs pass in both environments.
assert_pass "staging config accepted" bash scripts/check-infra-config.sh staging
assert_pass "prod config accepted" bash scripts/check-infra-config.sh prod

# Mutants: copies under a temp root with one injected fault each must
# be refused (RED proof per fault class).
mutant_root() {
    local dir="$1"
    rm -rf "$dir"
    mkdir -p "$dir/infra/caddy"
    cp infra/compose.staging.yml "$dir/infra/compose.staging.yml"
    cp infra/caddy/Caddyfile "$dir/infra/caddy/Caddyfile"
    cp infra/env.staging.example "$dir/infra/env.staging.example"
}

# 1. Public database port: db must never publish.
M1="$(mktemp -d)"
mutant_root "$M1"
python3 - "$M1/infra/compose.staging.yml" <<'EOF'
import sys
p = sys.argv[1]
s = open(p).read()
anchor = "    image: postgis/postgis:18-3.6@sha256:60f6ad1d21ea86a67d47780b9a0d1e1d200500f62b19293fa834d0dea80b8677\n"
assert anchor in s
s = s.replace(anchor, anchor + '    ports:\n      - "5432:5432"\n', 1)
open(p, "w").write(s)
EOF
assert_fail "public db port refused" bash scripts/check-infra-config.sh staging \
    "$M1/infra/compose.staging.yml" "$M1/infra/caddy/Caddyfile" "$M1/infra/env.staging.example"

# 2. Dev secret in the example: operator templates must stay placeholders.
M2="$(mktemp -d)"
mutant_root "$M2"
sed -i 's/^ANPFUEL_DB_PASSWORD=.*/ANPFUEL_DB_PASSWORD=anpfuel/' "$M2/infra/env.staging.example"
assert_fail "dev secret refused" bash scripts/check-infra-config.sh staging \
    "$M2/infra/compose.staging.yml" "$M2/infra/caddy/Caddyfile" "$M2/infra/env.staging.example"

# 3. Floating image: latest tags are never pinned.
M3="$(mktemp -d)"
mutant_root "$M3"
sed -i 's|image: caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d|image: caddy:latest|' "$M3/infra/compose.staging.yml"
assert_fail "floating image refused" bash scripts/check-infra-config.sh staging \
    "$M3/infra/compose.staging.yml" "$M3/infra/caddy/Caddyfile" "$M3/infra/env.staging.example"

# 4. Untrusted forwarded headers: open trusted_proxies must fail.
M4="$(mktemp -d)"
mutant_root "$M4"
sed -i 's/trusted_proxies private_ranges/trusted_proxies 0.0.0.0\/0/' "$M4/infra/caddy/Caddyfile"
assert_fail "open trusted_proxies refused" bash scripts/check-infra-config.sh staging \
    "$M4/infra/compose.staging.yml" "$M4/infra/caddy/Caddyfile" "$M4/infra/env.staging.example"

# 5. Missing TLS block must fail.
M5="$(mktemp -d)"
mutant_root "$M5"
sed -i '/^[[:space:]]*tls {/d' "$M5/infra/caddy/Caddyfile"
assert_fail "missing TLS refused" bash scripts/check-infra-config.sh staging \
    "$M5/infra/compose.staging.yml" "$M5/infra/caddy/Caddyfile" "$M5/infra/env.staging.example"

rm -rf "$M1" "$M2" "$M3" "$M4" "$M5"

echo "infra-config: $PASS passed, $FAIL failed"
[[ "$FAIL" -eq 0 ]]
