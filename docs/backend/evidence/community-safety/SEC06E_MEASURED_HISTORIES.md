# SEC06-E — Measured growing synthetic histories

Date: 2026-10-11. Status: LOCAL_DONE. Scope: measured outbox
histories + honest parser sample (no sync-flow, parser, or quota
change). Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC06, AUD-10/11). Contract: SEC06-C/D frozen dispatch/history/
retention/attempt budgets; backend intake bounded by SEC01 (aggregate
mail 20/h, key-usernames 100/h, sessions 10/acct, comments 100/acct)
and SEC03 (reserves 10/day + 30MiB live bytes); parser ceilings
SEC06-A/B (64MiB download, XLSX entry/string/worksheet caps).
Constraint: sizes are test loads, not supported-user promises.

Validation: PASS

## Changes

- `data/.../repository/OutboxHistoryScaleTest.kt` (new, 3 tests):
  counting DAO mirrors the Room eligibility predicate/keyset/
  history/retention shape; seeds 1/100/1_000/10_000 rows directly;
  asserts dispatch page ≤50, history page ≤200, single-id lookup,
  zero `listAll` scans on dispatch/history/reconnect paths, point
  lookups bounded by the page, retention batch ≤500 (1000 kept).
- No prod-code change: dispatch keyset + single-row lookup + paged
  history + indexed eligibility (SEC06-C) and attempt park at 25
  (SEC06-D) already bound reconnect work to one page per run.
- Backend intake side already bounded (quotas above) — reconnect
  cannot produce unbounded intake; `purgeOwned` full-scan stays on
  the explicit erasure path only (SEC04-D, fail-closed), never the
  reconnect hot path.

## Validation (actual)

- New suite 3/3 green; `:data:testDebugUnitTest` (full) green.
  `git diff --check` clean; no secrets/PII (synthetic ids).
- Measured (JVM, counting fake; println in test XML):
  N=1 scanned=3 dispatch=1 history=1 0ms/94MiB;
  N=100 scanned=201 dispatch=50 history=100 0ms/95MiB;
  N=1000 scanned=1201 dispatch=50 history=200 1ms/95MiB;
  N=10000 scanned=10201 dispatch=50 history=200 1ms/98MiB.
  Pages flat across 100x growth — no quadratic full-history scan.
- Offline start: fresh repo over the same 200 durable rows serves
  head `cmd-0` + full paged history with zero scans. Reconnect:
  one bounded head page per run (single command per worker run,
  SEC06-D budgets).
- Honest parser sample within ceilings: 19 676-row station file
  parses under `ParserMemoryPerformanceTest` (64MiB heap pin +
  duration sanity); SEC06-B crafted bombs still refuse tiny/fast.
- RED pre-GREEN: none needed — measurement pins existing bounds
  (bounds proven RED in SEC06-C/D).

## Limits / owed

- Fake counts N per keyset by construction (Room serves the same
  shape from the dispatch index); wall-clock is record-only.
- `purgeOwned` still scans (erasure-only, rare, owner-scoped).
- No production claim; no VPS/phone run.

Next microtask: SEC07-A refresh reuse proof.
