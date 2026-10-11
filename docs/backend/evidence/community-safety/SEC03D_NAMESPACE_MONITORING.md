# SEC03-D — Namespace isolation and intake monitoring

Date: 2026-10-11. Status: LOCAL_DONE. Scope: namespace pins +
worker-cap pin + failed-object reconciliation pin + intake-health
monitoring — no provider change (SEC03 exit). Head: maintained
`dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC03, AUD-02/04). Contract: B-BR-M01/M04 (retention), quarantine
`q/` vs server-only finals `f/`. Constraint: monitoring carries
counts/bytes/timestamp only — never keys, hashes or payloads; no
log/telemetry shape carries identity.

Validation: PASS

## Changes

- `evidence/adapters/storage/namespace_test.go` (new): 8-case
  isolation matrix — quarantine mints under `q/` only, finals
  under `f/` only, `public/` and bare/traversal keys refused
  everywhere. Bytes cannot enter a public prefix through this
  primitive (client wiring mints `q/`; worker mints `f/` with
  server credentials — code-inspected in `cmd/api` + `cmd/worker`).
- Worker caps pinned: `TestBoundaryDownloadBoundIsMaxPlusOne`
  (exactly one `MaxBytes+1` download) plus existing
  Snapshot/Inspect/Sanitize caps and the bounded worker GET.
- `evidence/application/sweep.go`: `IntakeHealth` snapshot
  (reserved sessions/bytes, accepted/rejected today + bytes,
  oldest incomplete) embedded in `SweepReport`; health-query
  failure degrades to zero instead of blocking cleanup.
- `db/queries/evidence/evidence.sql` (+ regenerated sqlc, vet
  clean): `SumReservedSessions`, `SumDecidedSessions`,
  `OldestIncompleteSession`.
- `evidence/adapters/store.go`: `IntakeHealth` implementation
  (3 bounded aggregates, no content).
- `evidence/adapters/jobs/sweep.go`: worker logs the posture
  alongside the sweep report (slog attrs only).
- Tests: health embed/degrade/rejected-reconciliation pins
  (unit); `TestSEC03DIntakeHealthOnRealSQL` (1 live + 1 rejected
  + 1 ready assert exact buckets on real PostGIS). Failed
  (REJECTED) originals delete+mark exactly like terminal copies
  (reconciliation pin; SQL already covered REJECTED in
  `QuarantineCandidates`).

## Validation (actual)

- `go test ./internal/modules/evidence/...` green (incl. 5 new
  pins). `-race -tags=integration ./internal/modules/evidence/adapters/`
  green (incl. new health proof; all prior SEC03 proofs re-green).
- `go build ./...`, `go vet` (+integration), `gofmt -l` clean,
  `sqlc vet` clean, `git diff --check` clean, `scan-secrets.sh`
  passed (synthetic keys only).
- RED pre-GREEN: health embed failed before the `SweepStore`
  extension (no posture source); namespace matrix failed before
  the cross-namespace cases were pinned (single-direction only).

## Limits / owed

- Byte sums are declared-bucket accounting (reservation intent),
  not measured wire payloads — labeled as such in code and here.
- R2 production behavior unverified; MinIO evidence rides
  SEC03-C. Multi-replica serializability beyond advisory xact
  locks not proven. No VPS/phone change; no production claim.

SEC03 exit complete (A/B/C/D). Next microtask: SEC04-A erasure inventory.
