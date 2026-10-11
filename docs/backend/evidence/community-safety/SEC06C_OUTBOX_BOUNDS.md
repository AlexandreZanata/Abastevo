# SEC06-C — Outbox SQL bounds, pages and retention

Date: 2026-10-11. Status: LOCAL_DONE. Scope: bounded dispatch
selection, paged history, retention purge, dispatch/history
indexes — no retry/backoff change (D), no measurements (E). Head:
maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC06, AUD-11). Contract: frozen revision/lease/owner/origin
semantics preserved; budgets in `OutboxBudgets` (dispatch page 50,
history page 200, terminal retain 1000, purge batch 500).
Constraint: loading all payloads to select one is forbidden;
unscoped legacy rows stay quarantined; live/retryable rows never
purge; no speculative abstractions.

Validation: PASS

## Changes

- `domain/.../ContributionOutboxPorts.kt`: `listDispatchable` /
  `listOwned` replaced by `findDispatchable` (one row, no scan),
  `listDispatchablePage` (keyset first, then chosen payloads),
  `findOwned`, `listOwnedPage` (newest-first); `purgeTerminalHistory`;
  `OutboxBudgets` frozen.
- `data/.../ContributionOutboxDao.kt`: single-row and keyset/page
  selects plus `deleteTerminalBeyond` (terminal beyond the newest
  retained window, oldest-first, bounded batch).
- Entities declare the dispatch/owner indexes;
  `MIGRATION_9_10` creates them with Room's auto-generated names
  (schema `10.json` verifies alignment); DB version 10, wired in
  `DatabaseModule`.
- `RoomContributionOutboxRepository`: bounded implementations;
  terminal purge amortized on both enqueue paths (best-effort,
  never fails the write); owner/origin parsing shared.
- Dispatch usecase (requested id → single find, else page head),
  history usecase (bounded newest-first page), cancel (single
  find), MainActivity resume (first history page).
- Tests: dispatch page cap (60→50, head identity), single-find
  with zero `listAll` calls, newest-first paging + offsets,
  retention window/batch/liveness, amortized convergence,
  migration statement pins; all prior dispatch/history/cancel/
  status suites re-green.

## Validation (actual)

- `:domain:test`, `:application:test`, `:data:testDebugUnitTest`
  (full), `:app:testDebugUnitTest` (full) green — incl. 5 new
  proofs and a migration test; schema `10.json` index names
  match the migration exactly.
  `git diff --check` clean; no secrets/PII (synthetic scopes).
- RED pre-GREEN: `listAll`-per-dispatch and unbounded history
  confirmed in code (audit shape); page/retention tests failed
  before the DAO bounds (full-table behavior).

## Limits / owed

- Erasure `purgeOwned` intentionally still scans (per-logout,
  not per-reconnect; documented frequency split).
- Retry/backoff/jitter budgets: SEC06-D. Measured histories:
  SEC06-E. No production claim.

Next microtask: SEC06-D retry and retention budgets.
