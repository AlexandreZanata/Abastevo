# MEX-13 — Monthly indicators and comparison Android journeys

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `a5e98507` (maintained dev, EE-21 commit; PR #147 head stays
open per batch rule, no merge yet). Date: 2026-10-26. Depends MEX-10/
12, owns PRM09-T06. Rules B-BR-MEX-03/04/18, BUC-MEX-03. Risk:
financial presentation.

Before behavior: owners read month/vehicle totals, measured versus
estimated metrics, cost/km with its window explanation, coverage and
unknown reasons, month-over-month comparison and safe held-data CSV
export. No revenue/profit is computed and no full-month denominator
is guessed. Recovery: suppress invalid metrics, keep raw totals and
data exit.

## Delivered

- `GetMonthlyIndicatorsUseCase`: timezone month bounds, exact money/
  energy sums with overflow guards, full-to-full measured intervals
  reusing the exact domain engine, PHEV shared-distance basis
  (per-carrier ratios refused, combined money/km exact), estimated
  km/L fallback only when measured is unknown, queued-gap
  invalidation, previous-month comparison, stale-correction-safe
  rebuilds (no caching).
- `MonthlyIndicatorsCsv`: byte-exact rows with formula-injection-safe
  escaping (leading =, +, -, @ quoted with a literal prefix).
- `MonthlyIndicatorsScreen/ViewModel` + `monthly-indicators/{id}`
  route with entry from the entries screen: month navigator, totals,
  measured/estimated, cost/km, combined, comparison, categories,
  window notes, CSV share. 19 new strings in all 8 locales.

## Actual checks

From this worktree (2026-10-26, parent `a5e98507`):

- RED: use-case/VM tests failed to compile before each unit; a blank
  `until`-range misuse, nullable cross-module smart casts, an
  internal-metrics helper boundary, unawaited validation and a stale
  month-navigation test were fixed without weakening tests.
- Golden parity (hand-derived 5 km/L, 100/3 c/km), timezone/month
  placement with leap/DST crossing, late correction convergence,
  unequal weights (sum-first 3, never mean 35/12), mixed units and
  PHEV basis refusal with exact combined (1043/4), formula escaping
  (`'=cmd...`), empty-baseline unknowns, estimated fallback, unknown
  coverage; VM navigation/rebuild/export/timezone tests; Room
  migration-free (no schema change).
- Full `:domain:test :data:testDebugUnitTest :application:test
  :app:testDebugUnitTest --offline` → 2005 tests, 0 failures;
  `:app:lintDebug :data:lintDebug` green (8 locales);
  `git diff --check` + secret scan clean. No backend/OpenAPI change;
  no PII/secrets in Git/logs. Device reopen stays deferred per
  standing directive.

## Limits

- No revenue/profit metrics; no guessed denominators; comparison is
  month-over-month only. CSV shares text (no file provider needed).
- No device, no production, no deploy and no release acceptance is
  claimed.

Next: EE-23 (energy comparisons) once authorized; it consumes these
indicators unchanged.
