# SEC02-B — Trusted-proxy client resolver

Date: 2026-10-11. Status: LOCAL_DONE. Scope: narrow resolver +
explicit allowlist + wiring — no global breaker (SEC02-C), no VPS
edge change. Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC02, AUD-03). Contract: B-BR-015 (identity quotas); Caddy edge
(`infra/caddy/Caddyfile`: `trusted_proxies private_ranges`,
`X-Forwarded-For {remote_host}`). Constraint: raw IPs stay out of
logs/storage (HMAC subjects only); never trust all private peers
or arbitrary X-Forwarded-For by default.

Validation: PASS

## Changes

- `identity/adapters/http/resolver.go` (new): `Resolver` bills the
  proxy-observed forwarding address only when the immediate TCP
  peer matches the explicit allowlist; otherwise bills the peer
  and ignores headers (spoof-proof by construction). Chain rule:
  last valid entry wins (older entries are client-asserted; the
  supplied Caddy replaces the chain with the single observed
  host). Malformed/absent data falls back to the peer; malformed
  peers fail closed (400, historical shape).
  `ParseTrustedProxies` parses CSV CIDRs, empty trusts nothing,
  non-CIDR fails closed naming the variable.
- `identity/adapters/http/http.go`: `Handler.TrustedProxies`
  allowlist; `quota()` resolves via the resolver before hashing.
  IPv4-mapped forms share budgets via the existing `Unmap`
  normalization (CS03B).
- `platform/config`: `ANPFUEL_TRUSTED_PROXIES` (CSV CIDR,
  default empty = trust nothing), validated at startup.
- `cmd/api`: parses the allowlist (startup fails on invalid) and
  wires it into the identity handler.
- `infra/env.{staging,prod}.example`: documented operator-owned
  variable (commented default, never public ranges).
- Tests: 3/3 SEC02-A regressions GREEN unskipped (trusted probe
  `10.0.0.0/8`); 13-case resolver matrix (direct/spoof/malformed/
  chain/loopback/unlisted peer/bad peer/default-deny) + parse
  matrix + config matrix.

## Validation (actual)

- `go test ./internal/modules/identity/... ./internal/platform/config/...`
  green. `-race -tags=integration ./internal/modules/identity/adapters/`
  green on disposable local PostGIS (quota/registration/rotation/
  retention suites re-green, no behavior change below HTTP).
- `go build ./...`, `go vet`, `gofmt -l` clean,
  `git diff --check` clean, `scan-secrets.sh` passed (synthetic
  TEST-NET IPs only).
- RED pre-GREEN: SEC02-A proved the shared-bucket FAIL unskipped;
  this task turns all 3 GREEN.

## Limits / owed

- Global vs per-client budget split + multi-step viability +
  local Caddy→API integration proof: SEC02-C.
- Actual shared-VPS edge change is out of scope; operators set
  the allowlist to their compose network. No live readiness
  inference; no production data.

Next microtask: SEC02-C global/per-client split + viability.
