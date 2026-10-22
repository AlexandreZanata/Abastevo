# MEX-04 — Batch local place lookup and availability

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Parent: d23aebc1f8847ab4997889c69257dc499cf29507. Date: 2026-10-21.
Depends MEX-02/03 (both LOCAL_DONE); existing Room/FTS schema and selection
contracts unchanged. Rules B-BR-MEX-07; BUC-MEX-05.

Before behavior: separate location discovery from imported-week/source
availability. Keep stable IBGE identity, accent/UF/code matching and the
FTS candidate cap; remove the mandatory imported-week gate on discovery
reads and every per-row/per-result catalog query. No schema migration, no
ranking change, no network/DB/producer change. Risk: SQL/selection
compatibility. Recovery: revert to row-by-row resolution; catalog and
imported data untouched.

## Delivered

Discovery reads no longer require an imported ANP week. `getStatesWithData`
and `getMunicipalities` resolve a nullable display week; without one every
entry reports availability from history only (NEVER_IN_ANP, or
NO_DATA_THIS_WEEK for locations seen in ANP history), never an error. The
global FTS search behaves the same. Selection (`selectLocation`) still
requires an imported week because the `CitySelected` event carries a
`surveyWeekId`; that week-bound write is outside this read-path unit.

Availability resolution is now batch and bounded:

- `AveragePriceDao.findLocationAvailability` returns one grouped row per
  imported (state, municipality) with the week flag computed in SQL, so the
  week set and the ever-imported set come from a single query.
- `MunicipalityCatalogRepositoryImpl` builds one prepared in-memory index
  per call from the existing indexed `findByState` read (one per involved
  state, unknown abbreviations skipped without a read) and resolves every
  row/result against it with the same `normalized_name` semantics as the
  previous row-by-row lookup (accent/case variants, unknown states and
  unmatched spellings resolve identically; verified by tests).
- New port methods `resolveAvailabilitySnapshot` and `enrichSearchResults`
  give the picker and the global search one batch pass for entries plus
  availability. The per-result `findCatalogEntry` loop in the search use
  case is gone; legacy `getLocationKeys*`/`resolveDataAvailability` keep
  their signatures and delegate to the snapshot.

Measured per ready global search: FTS (1) + grouped availability (1) +
one indexed state read per involved state; no imported week skips nothing
(the grouped query uniformly flags the week set empty). Per-state picker
load: state list (1) + grouped (1) + one indexed state read. The fuzzy
`findAll` fallback is untouched and stays owned by MEX-05.

## Actual checks

From the worktree (2026-10-21, HEAD d23aebc1 before this commit):

- `PYTHONDONTWRITEBYTECODE=1 python3 scripts/tests/test-mex04-batch-sqlite.py`
  PASS on host SQLite 3.50.6: grouped availability is exactly 1 query,
  state reads use the index (`USING INDEX`, no catalog scan), 7 distinct
  imported spellings resolve to week=3/ever=4 with unknown-state and
  unmatched rows dropped, and the no-week variant flags the week set
  empty while keeping history.
- `./gradlew --offline :data:testDebugUnitTest --tests
  com.anpfuel.data.repository.MunicipalityCatalogBatchTest` PASS (6 new
  tests): single grouped call, one indexed read per involved state, zero
  per-row/per-result lookups, accent variants, empty/no-week/unknown-state
  semantics and legacy delegations. Full `:data:testDebugUnitTest`: 440
  tests, 0 failures/errors, 2 pre-existing skips.
- `./gradlew --offline :application:test` PASS: 390 tests, 0 failures
  (location suites updated to the contracted behavior: discovery without
  imported data, batch enrichment verification, preserved selection gate).
- `./gradlew --offline :domain:test` PASS: 547 tests, 0 failures
  (including 2 new `resolveWithoutWeek` rule cases).
- `./gradlew --offline :app:testDebugUnitTest` PASS: 367 tests, 0
  failures; `:app:assembleDebug`, `:app:assembleDebugUnitTest`,
  `:app:lintDebug` and `:data:lintDebug` PASS.
- `git diff --check` PASS; `bash scripts/scan-secrets.sh` PASS (no tracked
  secrets); no PII/device/VPS data in fixtures or tests.

RED note: the pre-existing `rejectsSearchWhenNoImportedData` and
`rejectsSelectionWhenNoImportedData` tests pinned the old mandatory-week
gate and were inverted by this contracted change (reads succeed with
unavailable flags; only the week-bound selection write still throws).
One genuine defect was caught GREEN-side: the prepared index queried
`findByState` even for unknown abbreviations; now skipped before the read
(new test pins zero reads for `XX`).

Limits: host JVM/SQLite evidence only; Room DAO SQL validity comes from
compile-time Room verification. Native keystroke-to-visible timing, scroll
and device/PostGIS/network acceptance remain OWED under MEX-06/35/EE-28
and the deferred device matrix. No backend, migration, FTS ranking or
selection-semantics change. Next: MEX-05 bounded typo search.
