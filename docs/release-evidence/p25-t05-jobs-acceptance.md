# P25-T05 — Registry jobs, outage recovery and phase acceptance

Status: LOCAL_DONE on `codex/phase-25-national-registry`. Task: P25-T05.
Binds B-BR-D03/D05/D10/D12 and BUC-D01/D06/D08. Durable reconcile
operations on the platform queue with operator controls; schedule
disabled until live source access is verified (same honest pattern as
`geocode-hourly`).

## Behavior

- `directory/adapters/jobs` `Reconcile` handler (`registry-reconcile`
  v1): versioned `{source, snapshot}` envelope (bad version/shape →
  error for dispatcher retry/dead handling), dedupe key
  `registry-reconcile:<source>:<snapshot>`, publishes complete runs
  only via `ReconcileRun`. No publication pointer exists by design:
  projections are append-only, revocation sticks, wrong facts are
  corrected by new revisions — recovery conserves IDs and last-good
  state structurally.
- Worker wiring (`cmd/worker`): handler registered with the PG store +
  canonicalizer; `registry-reconcile-daily` schedule present but
  **disabled** (`P25 pending: no live source access yet`); staging
  stays operator-triggered per the runbook. `go build ./cmd/worker`
  PASS.
- Operator surface: `LastCompleteRegistryRun(source)` freshness query
  (snapshot/finished/counts) + `docs/operator/registry-runs.md`
  (schedule, stage/reconcile/pause/retry/quarantine/last-success/
  rollback procedures + failure table). Late-source and >48 h backlog
  escalate with cause/status; eligibility is never redefined to hide
  backlog.
- Risk coverage: worker death/stale-lease fencing/duplicate-enqueue/
  retry-exhaustion via the platform dispatcher guarantees (lease TTL +
  token fencing + dedupe + backoff-to-dead, suites rerun below); dup
  snapshot enqueue converges (dedupe key + assertion key); DB outage
  surfaces with the run left joinable; partial snapshots refused by
  `ReconcileRun`; quota-hit traversals quarantine instead of
  completing.

## Validation

- NEW: jobs `Reconcile` unit 3/3 PASS (publish + envelope refusals +
  row-failure surfacing); `LastCompleteRegistryRun` integration PASS.
- Regression: `directory/...` unit + real-PostGIS integration PASS
  (9 pkgs incl. new jobs/reconcile suites); `platform/jobs` unit +
  integration PASS (lease/replay/backlog); `go vet` clean; `sqlc vet`
  + `generate` clean; `vacuum lint` + `apicontract` PASS.
- `git diff --check` PASS; `scan-secrets.sh` PASS.
- G25 specialized exit recorded once below; no Android visibility or
  production claim inferred.

## Limits and next

- Schedule enablement + live staging await verified source access
  (explicitly pending, not silently skipped).
- Next: P25 phase exit (LOCAL_DONE checkpoint + evidence), then P26
  (`codex/phase-26-regulatory-discovery` via `--from-checkpoint`).
