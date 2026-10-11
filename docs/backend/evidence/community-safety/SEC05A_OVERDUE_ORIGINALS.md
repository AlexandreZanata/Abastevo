# SEC05-A — Overdue originals in audit and breaker

Date: 2026-10-11. Status: LOCAL_DONE. Scope: originals half of the
failing-policy signal — no scheduling change (B), no Android change
(C/D), no soak (E). Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC05, AUD-06). Contract: B-BR-M04 (intake breaker), CopyRetention
24 h. Constraint: strict deadline anchor (created_at); terminal
status, live jobs and stuck validation do not excuse overdue bytes;
alarms name the namespace and the worst lag.

Validation: PASS

## Changes

- `db/queries/evidence/evidence.sql` (+ regenerated sqlc, vet
  clean): `OverdueOriginals` — quarantine rows past deadline with
  no deletion marker, oldest-first (terminal/live-job/stuck
  agnostic by design: an overrun is itself a lag signal).
- `evidence/application/sweep.go`: `OverdueReport` gains
  `Originals` + `OldestOriginal`; `AuditOverdue` audits both
  namespaces (finals unchanged); `Unhealthy()` trips on either.
- `evidence/adapters/store.go`: `OverdueOriginals`
  implementation (bounded aggregates, no content).
- `cmd/api`: intake `Enforcement` now trips on
  `AuditOverdue(...).Unhealthy()` — an overdue original with a
  failed deletion stops new intake exactly like an overdue final.
- Tests: originals-alone trip (the AUD-06 repro: final-only
  healthy-zero is gone), healthy-zero needs both namespaces, bad
  batch refuses; real-PostGIS 48 h original trips, fresh stays
  silent, marked-deleted silences again (end-to-end audit proof).

## Validation (actual)

- `go test ./internal/modules/evidence/...` green (incl. 3 new
  audit pins; existing overdue/final suites re-green).
- `-race -tags=integration ./internal/modules/evidence/adapters/`
  green for the new proof (full-suite re-run rides the commit
  gate below with the other SEC05 letters).
- `go build ./...`, `go vet` (+integration, cmd/api),
  `gofmt -l` clean, `sqlc vet` clean, `git diff --check` clean,
  `scan-secrets.sh` passed (synthetic keys only).
- RED pre-GREEN: originals-alone reported healthy zero before
  the `OverdueOriginals` half (audit probe reproduced); the
  breaker wiring compiled before the store method existed.

## Limits / owed

- Deadline-aware scheduling margin/catch-up/alarms: SEC05-B.
  Android transient-cache cleanup: SEC05-C. Backup assessment:
  SEC05-D. Real 24 h every-copy observation: SEC05-E (time-gated,
  cannot complete in-turn).
- Multi-replica serializability beyond row locks not proven. No
  VPS/phone change; no production claim.

Next microtask: SEC05-B scheduling margin and catch-up.
