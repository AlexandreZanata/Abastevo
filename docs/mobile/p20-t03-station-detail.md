# P20-T03 — Station detail and dated ANP reference

Status: LOCAL_DONE on `codex/phase-20-community-discovery`. Issue: #78. Binds B-BR-C01/C06 and BUC-C01 to existing contracts. No backend, map or new index in this task.

## What this task adds

- Domain `StationDetailRule` (pure): resolves one station's local rows into `NoCoverage` (honest empty, never an error or invented price) or `AnpReference` with per-row freshness — `UNKNOWN` for a missing collection date (never fresh), `STALE` for an old survey week or stale cache, `FRESH` otherwise. Community stays `UNKNOWN` until P04; only the backend can report a dispute (`communityDisputed` passes through, never invented locally).
- `StationDetailSheet` (bottom sheet on the Stations journey): station header, dated ANP reference card (fuel, exact price, collection date or explicit unknown-date copy, survey-week range, stale badge, `SourceTimeBadge` ANP_DATED with text + icon + screen-reader label), community card (explicit no-coverage `community_pending_p04` + confidence note, or disputed copy), Route and Atualizar preço actions.
- Journey change: tapping a station row now opens the detail sheet (route still one tap away inside it); update-price navigates to capture. Hint copy updated (en + pt-BR).
- `StationsViewModel`: `selectedDetail` derived from the cached domain list (unknown CNPJ yields no detail); dismissed on fuel change.

## Reused (not rebuilt)

`StationPriceOrderingRule`, `SurveyWeekFreshnessRule`, `StationPriceUiMapper`, `SurveyWeekFormatter`, `FuelProductLabel`, `SourceTimeBadge`, `CommunityPriceDisplay` labels, existing route/capture destinations.

## Explicitly deferred (not waived)

- Condition qualifiers (APP/LOYALTY) appear only with backend community coverage; local rows carry none.
- Optional map: only after a provider/dependency/privacy audit, per frozen IA.
- `stations_tap_to_navigate_hint` and 5 new `station_detail_*` strings translated en + pt-BR; de/es/fr/ja/ru/zh-CN fall back to en until translators update (6 stale hint translations recorded, no behavior impact).

## Validation

- `StationDetailRuleTest` 6/0-fail (RED compile-fail → GREEN).
- `StationsViewModelTest` 9/0-fail (2 new: detail from cache + dismiss, unknown CNPJ).
- `./gradlew :domain:test`, `:app:testDebugUnitTest`, `:app:assembleDebug` (affected suites).
- `git diff --check` clean; scoped secret review (no secrets/PII/GPS/photo content).
