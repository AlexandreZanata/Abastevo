# MEX-06 — Integrate fast state/city UI and measure perceived latency

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Parent: 3c6a987819a284fa0e3c615d847c816473a7dc9. Date: 2026-10-21.
Depends MEX-05 (LOCAL_DONE); existing location UI, no new backend contract.
Rules B-BR-MEX-07/18/19; BUC-MEX-05/10.

Before behavior: give the picker immediate bounded local results, fence
async loads by generation so obsolete responses can neither replace
current results nor surface stale errors, and record a measured debounce
policy. Preserve query/selection/scroll behavior, honest availability,
accessibility and locales. No new backend contract, no schema change, no
native timing claim. Risk: Android lifecycle. Recovery: scoped search
policy rollback (single constant), no data reset.

## Delivered

- `LocationPickerViewModel` fences every catalog load with one shared job
  slot plus a generation ticket: a new load cancels the previous one and
  only the latest generation publishes results or errors. `runCatching`
  blocks now rethrow `CancellationException` instead of surfacing
  "Job was cancelled" as an error message (same defect fixed on the save
  path). Loaded lists are never cleared on refresh failure.
- `LocationPickerScreen` memoizes composition work: state labels + filter
  run only when states/query/labels change; municipalities filter over a
  precomputed normalized index (`LocationPickerFilter.indexMunicipalities`
  + `filterIndexed`, parity-tested against the plain filter). LazyColumn
  keys, section grouping, empty/error/skeleton states and subtitles are
  unchanged.
- `LocationPickerSearchPolicy.QUERY_DEBOUNCE_MS = 0` records the measured
  policy: local lists are already loaded and bounded, the indexed keystroke
  costs ~0.3ms, so an artificial delay would only add latency. Async loads
  stay immediate and fenced instead of debounced. Changing the constant
  requires a new justifying measurement (B-BR-MEX-19).

## Actual checks

From the worktree (2026-10-21, HEAD 3c6a9878 before this commit):

- RED: `rapidStateTapsKeepOnlyLatestMunicipalities` failed on the unfenced
  code (stale São Paulo overwrote Rio) and
  `staleStatesLoadCannotOverwriteFreshLoad` failed (stale single-state
  load won); both pass after fencing. A third failure was a wrong test
  expectation about the ViewModel's alphabetical state ordering, fixed in
  the test.
- `./gradlew --offline :app:testDebugUnitTest --tests
  "com.anpfuel.app.ui.location.*"` PASS: 13 tests (5 new ViewModel:
  rapid taps, stale-load fencing, offline retention, no-week discovery,
  selection-error reset; 6 filter incl. 2 index parity/order; 1 grouper;
  1 benchmark), 0 failures.
- Benchmark (host JVM, 27 states + synthetic 645-row state list, 9
  keystrokes): old path cold 5.95ms / warm p50 2.43ms / p95 5.13ms; indexed
  path cold 0.35ms / warm p50 0.32ms / p95 0.35ms (~15×). Host filter
  computation only, not Compose keystroke-to-visible proof. The earlier
  in-test p95 < 16ms wall-clock assertion was removed after it failed on a
  shared CI runner; behavioral parity, ordering and pool bounds remain
  enforced and native keystroke-to-visible timing stays OWED.
- Full `./gradlew --offline :app:testDebugUnitTest` PASS: 375 tests, 0
  failures; `:app:assembleDebug`, `:app:assembleDebugUnitTest` and
  `:app:lintDebug` PASS. application/domain untouched by this unit
  (location use-case contracts unchanged since MEX-04).
- `git diff --check` PASS; `bash scripts/scan-secrets.sh` PASS; no
  PII/device/VPS data in tests.

Limits: no device execution (native keystroke-to-visible timing, scroll
and frame proof stay OWED under the deferred device matrix; existing
emulator constraints unchanged). No backend/migration/FTS/ranking change.
Next: MEX-07 hosted vehicle capability (depends MEX-01 + PRM sources).
