# Retention operations (P07-T05)

Schedules, metrics and per-category behavior for the inventory
periods in [SECURITY_PRIVACY](../security/SECURITY_PRIVACY.md) and the
draft [privacy notice](../backend/PRIVACY_NOTICE.md). Values are
initial minimization choices pending pilot review, not statutory
periods.

## Schedules

| Schedule | Kind | Cadence | Covers |
|---|---|---|---|
| `evidence-sweep-hourly` | `evidence-sweep` | Hourly | Every app-owned copy 24 h from first receipt (P15-T04; case extensions no longer extend photo bytes), orphan/stuck sessions, 90 d hash purge |
| `privacy-retention-daily` | `privacy-retention` | Daily | Challenges, idempotency windows, closed moderation cases (12 mo), expired export bytes, ledger horizon (35 d), observation-age metric |

Both enqueue through the durable job queue with dedupe keys; poison
jobs cap retries then park for audited replay. There is no manual
purge procedure: ad-hoc SQL deletes outside these jobs are findings,
not operations.

## Reading the metrics

Each `privacy-retention` run logs one structured line:

```text
retention sweep generated_at=... challenges/purged=12 challenges/oldest_overdue=... ...
```

- `*/purged`: rows cleared per category this run.
- `*/oldest_overdue`: oldest instant that was past its period before
  purging (empty when nothing was due). A non-empty value that never
  shrinks across runs means the purge is not keeping up: investigate
  the category before widening batches.

## Per-category behavior

- Challenges/idempotency: strictly-expired rows only (`expires_at`
  equality stays); 500-row batches, oldest first; totals plus oldest
  reported.
- Moderation closed cases: only `RESOLVED`/`REJECTED` older than 12
  months; audit rows delete before their cases each batch; open and
  triaged cases are never touched.
- Export bytes: only `READY` rows past the 24 h window; receipts stay
  for audit.
- Ledger: only rows older than 35 days (no existing backup needs
  them); restores replay present rows before traffic (see
  [recovery.md](recovery.md)).
- Observations: metric only (`community-observations` reports the
  oldest fact, purges zero). The 24-month purge needs an
  FK-consistent cascade design across votes/decisions/projections;
  until that ADR lands, the metric keeps the horizon visible and no
  job deletes history.
- Rate windows self-clean on quota check; exact GPS/OCR have no
  long-lived store in v1 (derived-then-deleted by construction).

## Failure handling

A failing category fails its job after all categories ran, so one
stuck category cannot hide the others' metrics; completed categories
converge and the next tick resumes. Never reduce thresholds, add
skips or suppress errors to green a sweep: fix the cause and rerun.
