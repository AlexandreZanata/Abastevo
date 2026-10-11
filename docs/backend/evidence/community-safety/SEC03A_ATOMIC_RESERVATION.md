# SEC03-A — Atomic reservations with byte budgets

Date: 2026-10-11. Status: LOCAL_DONE. Scope: count + live-byte
budgets with the row in one transaction — no transfer-boundary
change (SEC03-B), no provider matrix (SEC03-C). Head: maintained
`dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC03, AUD-02/04). Contract: B-BR-005 (natural-key convergence),
abuse-signal defaults (10/day). Constraint: free contribution
stays free inside ceilings; retries never multiply storage or
extend lifetime; hashed refs only.

Validation: PASS

## Changes

- `evidence/domain/session.go`: `ErrQuotaDenied` +
  `ReserveBudget{Now, DayStart, MaxCount, MaxBytes}` (expired and
  terminal sessions release bytes; created-today rows keep
  counting).
- `evidence/application/reserve.go`: frozen
  `MaxReserveBytesPerDay` (10 x 3MiB); `Store` shrinks to one
  atomic `ReserveSession(ctx, s, budget)`; the separate
  count-then-insert race is gone. Over-budget maps to
  `QuotaDeniedError` (next-midnight retry); identical replays
  converge on the winner even at full quota; divergent bodies
  conflict.
- `evidence/adapters/store.go` (PG): replay-first lookup, then
  count (`CountSessionsSince`) plus live bytes (new
  `SumLiveBytesSince`: ISSUED/VERIFYING, unexpired, created
  today), then insert — one tx under
  `pg_advisory_xact_lock('reserve:'+ref)`.
- `db/queries/evidence/evidence.sql` (+ regenerated sqlc, vet
  clean): `SumLiveBytesSince`.
- `evidence/application` fakeStore: same contract under one
  mutex.
- Call sites updated to the budgeted signature (adapters
  integration/e2e suites use open budgets to isolate
  converge/conflict behavior).
- Tests: parallel 12-over-9 admits exactly 10 (unit + real
  PostGIS); replay-at-full-quota reads the winner; divergent
  conflicts; byte cap binds and releases on expiry; real-SQL
  overflow denies with zero new rows.

## Validation (actual)

- `go test ./internal/modules/evidence/...` green (incl. 3 new
  budget tests; daily-cap test re-green on the atomic path).
- `-race -tags=integration ./internal/modules/evidence/adapters/`
  green (incl. 2 new SEC03-A proofs; converge/conflict/e2e
  suites re-green).
- `go build ./...`, `go vet` (+integration), `gofmt -l` clean,
  `sqlc vet` clean, `git diff --check` clean, `scan-secrets.sh`
  passed (synthetic refs only).
- RED pre-GREEN: the audit probe (21 durable rows for a budget
  of 10) is exactly what the new parallel tests now forbid; the
  byte-release test failed before the expiry filter.

## Limits / owed

- Transfer-boundary selection/proof (exact-length PUT vs bounded
  application path): SEC03-B. Provider-compatible matrix:
  SEC03-C. Namespace/monitoring: SEC03-D.
- Multi-replica strict serializability beyond advisory xact locks
  not proven here. No VPS/phone change; no production claim.

Next microtask: SEC03-B transfer boundary.
