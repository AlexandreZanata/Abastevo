# DOU edition operations (P26-T03)

Scope: INLABS edition discovery, act classification, correction chains
and catch-up. Live access unavailable: all procedures below run against
synthetic fixtures or operator-fetched editions until an INLABS account
exists; unavailable facts stay pending and visible, never green.

## Review queue

- Ambiguous acts (`ClassAmbiguous`), multi-identifier acts and
  corrections without a resolvable prior act ID quarantine with
  counts (`StageActs` skip reasons). Review uses the existing
  moderation ports and roles — no new authority is invented here.
- Approve path: corrected snapshot staged as a new edition (new
  checksum → traceable row), then `LinkCorrection` edge persisted via
  `SetAssertionSuperseded` (refuses unrelated/self links). Reject
  path: record the reason in the run's `error_code` and keep the
  previous catalog; never delete staged history.
- Review concurrency and role denial follow moderation semantics:
  two reviewers cannot both consume the same quarantine item without
  one losing visibly (no silent overwrite).

## Catch-up and freshness

- `MissingDates(from, to, seen)` lists weekday editions missing from
  checkpoints (weekends never listed). After an outage, backfill in
  date order; checkpoints dedup replays. Late editions stage normally;
  their later effective dates never rewrite earlier chronology
  (correction chains link, republication converges by checksum on the
  same identity).
- Freshness/lag: `LastCompleteRegistryRun('dou-acts')` plus edition
  checkpoints show the last validated edition date. Retry with bounded
  backoff; escalate past 48 h with cause/status (B-BR-D12). No
  national opening SLA is asserted: grant dates are authorization
  evidence with edition/effective dates only.
- Conflicting CSV/API/DOU facts route to review (D02/D03); a DOU
  revocation projects `revoked` on the converged identity while the
  registry row stays for audit.

## G26 acceptance state

G26 stays BLOCKED: correct recovered state is proven in isolation
(unit + PostGIS suites below), but live edition access is unavailable
so the gate cannot pass. Recorded honestly per plan instead of a
pretended discovery run.
