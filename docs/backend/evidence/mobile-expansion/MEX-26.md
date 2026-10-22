# MEX-26 — National occurrence explorer and source detail Android

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `30228626` (maintained dev, PR #146 head). Date: 2026-10-22.
Depends MEX-25, accepted. User-authorized Android scope (G09/G24).
Rules B-BR-MEX-10–15/18; BUC-MEX-07/09. Risk: consumer meaning/UI
(critical: no guilt/trust badge, honest empty/unavailable copy,
accessible locales, bounded traversal).

Before behavior: the app explores official occurrences through
discover → filters → lazy list → source detail, consuming the tested
MEX-25 types without new transport or cache. Labels distinguish
fiscal actions, reported infractions, quality assays and fine records
with source wording; no guilt, trust or safety badge appears. Empty,
unavailable and unsupported states use reviewed copy; stale cache
discloses itself. Filters, pagination and detail survive cancellation
and process restart via saved state and generation guards. No bottom
tab is added; existing navigation, DI and feature flags are reused.
Recovery: disable the explorer entry, keep direct navigation.

## Delivered

- `app/.../ui/occurrence/OccurrenceExplorerScreen.kt`: filter chips
  (BR/SP/RJ/DF), lazy list with keys and headings semantics,
  coverage line, stale/unavailable/empty states with retry, and
  detail navigation. No guilt language anywhere.
- `app/.../ui/occurrence/OccurrenceDetailScreen.kt` (+ ViewModel):
  source detail with wording, procedure, geography, segment,
  document number/day, stale flag and the no-guilt disclaimer.
- `app/.../ui/occurrence/OccurrenceExplorerViewModel.kt`: StateFlow
  pagination with cursor, filter reset, coverage fetch, generation
  guards against late responses, SavedStateHandle filters, and the
  OCCURRENCE_UI_ENABLED gate (default true, same pattern as quality).
- `app/navigation/Routes.kt` + `AnpNavGraph.kt`:
  `occurrence-explorer` and `occurrence-detail/{recordId}` (UUID
  validated) without a new tab.
- `app/.../di` via existing Hilt (`OccurrenceModule` in data) and
  `OCCURRENCE_UI_ENABLED` BuildConfig flag.
- `res/values*/strings_occurrence.xml` in all eight locales with
  reviewed copy (empty/unavailable/unsupported/stale/coverage/
  no-guilt/company-note/labels).
- `OccurrenceExplorerViewModelTest` (4 JVM cases): first page with
  coverage, filter reset, failure-with-empty unavailable, stale
  surfacing.

## Actual checks

From this worktree (2026-10-22, parent `30228626`):

- `./gradlew :app:testDebugUnitTest --offline --tests
  "com.anpfuel.app.ui.occurrence.*"` → PASS, 4 tests.
- `./gradlew :app:testDebugUnitTest --offline --tests
  "com.anpfuel.app.navigation.*"` → PASS (no navigation regression).
- `./gradlew :domain:test :data:testDebugUnitTest --offline`
  (occurrence + local cache scopes) → PASS.
- `./gradlew :app:lint :data:lint --offline` → PASS (19
  MissingTranslation errors fixed with eight-locale strings).
- `git diff --check` PASS; `bash scripts/scan-secrets.sh` PASS.
- No backend, contract, worker, iOS or Room schema changes; no
  source activation; no PII/secrets in Git/fixtures; `com.anpfuel`
  and MIT notices preserved.

## Limits

- This unit ships UI + navigation only. Transport, cache and DTOs
  are MEX-25's tested boundary, reused unchanged. No new API, table
  or migration.
- Detail is fetched by record ID through the gateway; deep links
  without a loaded list resolve via network or fail closed to the
  unavailable state (never an invented record).
- Non-PT locales carry translated labels; TalkBack headings, large
  fonts and RTL follow standard Compose behavior (no custom
  accessibility service).
- Process restart restores filters via SavedStateHandle; in-flight
  pages restart from the first page (cursors are not persisted
  across process death by design).
- No device farm, no production, no Play release and no release
  acceptance is claimed.

Next: MEX-27 (occurrence filters/period/source Android refinements)
needs its own authorization; it extends this tested explorer.
