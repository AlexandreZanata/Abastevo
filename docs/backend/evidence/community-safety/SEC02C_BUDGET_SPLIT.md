# SEC02-C — Global/per-client split, viability and edge proof

Date: 2026-10-11. Status: LOCAL_DONE. Scope: deliberate global
breaker + viability pins + real local Caddy→API proof — no shared
VPS edge change. Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC02, AUD-03). Contract: B-BR-015 (identity quotas); Caddy edge
(`infra/caddy/Caddyfile`). Constraint: raw IPs stay hashed-only;
global shedding never leaks identity; multi-replica edge sizing
stays an operator decision.

Validation: PASS

## Changes

- `identity/adapters/http/breaker.go` (new): process-local
  `GlobalBreaker` (hourly per-operation ceilings, mutex-guarded,
  window reset, concurrent-safe) + `BreakerError` (503
  `identity.overloaded` + Retry-After). Zero/nil disables an
  operation (no global cap; fairness still enforced).
- `identity/adapters/http/http.go`: `Handler.GlobalBreaker`;
  `quota()` sheds global-first (503) before per-subject fairness
  (429), so overload never consumes fairness budget.
- `platform/config`: `ANPFUEL_GLOBAL_QUOTA_{CHALLENGE,REGISTER,WRITE}`
  (non-negative ints, default disabled/0, startup-validated).
- `cmd/api`: builds the breaker from config into the handler.
- Tests: breaker disabled/trip/reset/concurrency pins; handler
  viability ceremony (real quota path: 3 challenges + register +
  rotate = 10 units; 6/60 challenge + 4/10 register headroom);
  503 shape (code/header/no-echo); real docker Caddy
  (pinned `caddy:2.10.2-alpine`) → API proof: forged XFF replaced
  by observed loopback (billed subject matches), direct forgery
  bills the peer (spoof ignored end-to-end).

## Validation (actual)

- `go test ./internal/modules/identity/... ./internal/platform/config/...`
  green. `-race -tags=integration` green: identity adapters
  (existing quota/registration/rotation/retention re-green) +
  `TestLocalCaddyForwardsObservedClient` PASS with the real edge
  (0.3s, image cached).
- `go build ./...`, `go vet`, `gofmt -l` clean,
  `git diff --check` clean, `scan-secrets.sh` passed (synthetic
  TEST-NET IPs only).
- RED pre-GREEN: breaker tests failed before the handler shed
  (no 503 path); viability counts drifted before valid JWK/
  purposes (proves the test bills the real path, not stubs).

## Limits / owed

- Process-local breaker covers single-process overload;
  multi-replica/global-DDoS protection belongs to the separately
  authorized edge/network layer (not claimed here).
- Global ceilings default disabled (deliberate opt-in); numeric
  values remain operator-sized, not advertised capacity.
- Shared-VPS edge change out of scope; staging rollout rides
  SEC09. No live readiness inference.

Next microtask: SEC03-A atomic reservations.
