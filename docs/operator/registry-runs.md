# Registry runs operator runbook (P25-T05)

Scope: national registry staging/reconciliation jobs. Staging is
operator-triggered until live source access is verified; the daily
reconcile schedule is disabled for the same reason. Publication is
append-only with no pointer to roll back.

## Schedule

- `registry-reconcile-daily` (`registry-reconcile` v1, every 24 h):
  **disabled** — reason `P25 pending: no live source access yet`.
  Enable only after T02-style live source reconfirmation plus a
  successful operator-triggered staging of the same snapshot shape.
- Daily CSV reconciliation stays the target cadence (B-BR-D12);
  targeted API lookups run on demand for reported/changed CNPJs.

## Operator procedures

- **Stage a snapshot** (operator step, local secrets only): fetch the
  approved CSV/API origin per `docs/product/REGISTRY_SOURCES.md`,
  then stage with the bounded `registry.StageCSV`/`Discover` entry
  points (100 MB / 500k rows / page-100 caps). Never hand-edit
  `registry_assertions`; never `TRUNCATE`.
- **Reconcile**: enqueue `registry-reconcile` v1
  `{source, snapshot}` (dedupe key `registry-reconcile:<source>:
  <snapshot>` converges duplicates) or run the worker with the
  schedule enabled. Only `complete` runs publish; anything else is
  refused by `ReconcileRun`.
- **Pause**: keep the schedule disabled (default) or redeploy with the
  entry disabled; in-flight runs finish their batch and report —
  nothing half-publishes. Resume by re-enqueueing the snapshot (replay
  is idempotent).
- **Retry**: failed jobs retry with backoff to the cap via the
  platform dispatcher, then park as dead; replay dead or re-enqueue
  the same snapshot after fixing the cause. Stale leases expire and
  are fenced by token (a dead worker never double-publishes: the
  assertion key converges).
- **Quarantine**: inspect `registry_source_runs` (`quarantined` +
  `error_code` + counts); fix the source/layout and stage a new
  snapshot. Last-good catalog stays served throughout (D05).
- **Last success / freshness**: `LastCompleteRegistryRun(source)`
  (snapshot, finished_at, counts). Alert when age exceeds 24 h past
  the daily target or the 48 h escalation threshold (B-BR-D12
  objective, measured from sufficient evidence, not inauguration).
- **Rollback**: no publication pointer exists by design. Revocation
  sticks via status; wrong projections are corrected by new audited
  revisions, never deletes. Disable the source flow to stop further
  publication.

## Failure table

| symptom | cause | action |
|---|---|---|
| run `failed/oversize`, `row_cap` | snapshot exceeds caps | split scope or recalibrate `[CALIBRATE]` budgets with measured data |
| run `quarantined/missing_columns`, `unknown_columns` | layout drift | freeze new header mapping in policy; new snapshot |
| run `failed/truncated`, `transport`, `provider_error` | source/outage | retry after cause fixed; last good preserved |
| run `quarantined/page_quota`, `request_quota` | traversal quota hit | raise quota deliberately or accept partial explicitly (never complete) |
| job dead after retries | persistent failure | inspect `last_error`, fix, replay dead |
| complete runs age > 48 h | missed source/operator | escalate with cause/status (B-BR-D12), do not redefine eligibility |
