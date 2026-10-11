# CS03B — Limiter cardinality, IP normalization, rotation

Date: 2026-10-11. Status: LOCAL_DONE. Scope: identity admission
hardening only — no migration, no new tables, no VPS/APK change.
Head: maintained `dev`.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md),
[CS03A quota](CS03A_HELPFULNESS_QUOTA.md). Rules: B-BR-CS03.

## Fixes (RED first, in the owning module)

`HashIPSubject` (`identity/domain/quota.go`) had two subject-forging
gaps, both caught by the new pins before the fix:
- IPv4-mapped IPv6 (`::ffff:192.0.2.10`) digested differently from
  `192.0.2.10`, letting one host hold two NAT budgets. Fixed with
  `Unmap()` before hashing.
- Scoped zone identifiers (`fe80::1%eth0`) accepted, smuggling
  interface identity into subjects. Fixed with explicit rejection.
Uppercase hex already converged; bad key/empty/malformed inputs were
already refused. The single HTTP caller fails closed on error
(unchanged path).

## Proofs (actual)

- Unit `identity/domain`: mapped/v4 equivalence, case convergence,
  zone/whitespace/invalid rejection, rotation retires digests
  (pre-existing), no raw-IP leak (pre-existing).
- Integration `identity/adapters`: 50 farm subjects hold exactly 50
  live rows (linear, no leaks); expiry + `CleanupExpiredWindows`
  sweeps all 50 and budgets return — cardinality is bounded by live
  windows, never history. No global subject ceiling added: with
  per-subject budgets plus sweep, a hard ceiling would risk
  shared-NAT availability for no measured gain; recorded residual.
- Integration `identity/adapters`: attribution token identical before
  and after full key rotation — a rotated device keeps one
  helpfulness vote (no fork), and the revoked old key stays dead
  (existing happy-path proof cited, not duplicated).
- Full identity unit + integration suites green (no regressions from
  the normalization change).

## Checks

`go build`, `go vet` (incl. integration tags), `gofmt -l`,
`git diff --check` clean; `scan-secrets.sh` passed.

## Limits / owed

- Per-route/account/verified-installation/trusted-IP layering beyond
  fingerprint+write, concurrent admission-key farm soak: CS07
  measurement or a dedicated admission phase if the plan splits one.
- Erasure account → refs fan-out: CS09. Moderation target: CS05.

Next microtask: CS04 — bounded feeds and query plans.
