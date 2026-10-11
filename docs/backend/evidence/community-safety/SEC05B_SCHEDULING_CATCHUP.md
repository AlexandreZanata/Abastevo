# SEC05-B — Deadline-aware scheduling, catch-up and alarms

Date: 2026-10-11. Status: LOCAL_DONE. Scope: post-pass overdue
signal + bounded catch-up proof + warn-level alarms — no cadence
or schema change (hourly sweep stands with measured margin), no
Android change (C/D), no soak (E). Head: maintained `dev` + this
task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC05, AUD-06). Contract: B-BR-M04, CopyRetention 24 h, hourly
sweep with batch 100/category. Constraint: retries stay idempotent
and batch-bounded; alarms name namespace + worst lag; no new
migration (none needed — stated explicitly per plan).

Validation: PASS

## Changes

- `evidence/application/sweep.go`: `SweepReport.Overdue` —
  post-pass bounded re-audit after each run's work; errors degrade
  to zero instead of failing succeeded cleanup.
- `evidence/adapters/jobs/sweep.go`: warn-level alarm
  (`evidence.sweep.overdue`) with oldest stamps per namespace
  when the backlog is unhealthy; quiet info otherwise. Health
  and overdue fields ride the existing slog report (counts/bytes/
  timestamps only).
- `evidence/application/sweep_test.go`: batch-honoring fake
  inventory (mirrors bounded SQL) for `ExpireIdleSessions` and
  `StaleFinalCandidates`; `OverdueFinals` fake mirrors SQL
  semantics (unprocessed past-deadline finals read overdue).
- Tests: 250-backlog converges in exactly 3 bounded runs then
  goes quiet (measured 100/run catch-up; hourly cadence vs 24 h
  deadline = 4% lifetime margin plus backlog absorption);
  namespace signal names finals/originals worst lags and alarms.

## Validation (actual)

- `go test ./internal/modules/evidence/...` green (incl. 2 new
  scheduling proofs; all prior SEC03/SEC05-A suites re-green).
- `go build ./...`, `go vet` (+cmd/worker), `gofmt -l` clean,
  `git diff --check` clean, `scan-secrets.sh` passed.
- RED pre-GREEN: catch-up test stalled before batch-honoring
  fakes (single-run assumed drain); alarm signal failed before
  the post-pass audit (backlog invisible after work).

## Scheduling posture (measured, not assumed)

- Cadence hourly, batch 100/category/run: worst-case fresh
  expiry waits <1 h + backlog/100 h; a 250-copy surge absorbs in
  3 runs with the alarm naming the remainder each run.
- Storage-outage copies retry next runs idempotently (counted,
  never aborting); live-job sessions skip (SkippedActive);
  crash resumes from DB markers (convergence suites);
  clock-skew residual documented (app clock selects, DB `now()`
  marks — single-digit seconds on NTP-synced hosts, ops-owned).
- No migration added: no schema change was needed.

## Limits / owed

- Android transient-cache cleanup: SEC05-C. Backup assessment:
  SEC05-D. Real 24 h every-copy observation: SEC05-E (time-gated).
- Multi-replica serializability beyond row locks not proven. No
  VPS/phone change; no production claim.

Next microtask: SEC05-C Android transient-cache cleanup.
