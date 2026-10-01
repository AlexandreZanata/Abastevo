# P20-T02 — Explore city and fuel journey

Status: LOCAL_DONE on `codex/phase-20-community-discovery`. Issue: #77. Binds B-BR-C01/C06 and BUC-C01 to existing contracts. No backend, map or new index in this task.

## What already existed (reused, not rebuilt)

- Manual city via preferred location header (`SelectLocationUseCase.getPreferredLocation`, no GPS required).
- Fuel filter chips (`FuelProduct.entries`) with `SavedStateHandle` persistence.
- Price-ascending sort (`StationPriceOrderingRule`, `GetStationPricesUseCase` UC-007).
- Offline banner + cached ANP reads, download prompt, BR-010 empty state, route action.
- Guest access: no signup gate on this journey.

## What this task adds

- Domain `DiscoveryStationsRule` (pure): station-name search over trade/legal name, brand and address (case-insensitive; blank or below-minimum-length typing does not exclude; committed queries still enforce >= 2 chars via `DiscoveryQuery`), `PRICE_ASC` / `RECENCY_DESC` (unknown dates last, price tie-break), `paginate` with `hasMore`/`totalCount` and 1..100 page-size bounds. No match is an honest empty list.
- `StationsViewModel`: `searchQuery` state persisted in `SavedStateHandle`, local filtering over the cached domain list without reload; failed refresh keeps the last good list and shows the error above it instead of replacing it; explicit fuel change still drops the previous fuel's cache.
- `StationsScreen`: station-name search field; cached list stays visible during reload/failed refresh with retry; explicit no-match copy (`stations_search_no_match`, en + pt-BR, other locales fall back to en).

## Explicitly deferred (not waived)

- Optional map: only after a provider/dependency/privacy audit, per frozen IA.
- `RECENCY_DESC` list toggle and backend paginated endpoint: rule-level only until measured fixture scale justifies an index (P20-T01 note).

## Validation

- `DiscoveryStationsRuleTest` 7/0-fail (RED compile-fail → GREEN).
- `StationsViewModelTest` 7/0-fail (2 new: failed-refresh cache, local search).
- `./gradlew :domain:test`, `:app:testDebugUnitTest`, `:app:assembleDebug` (affected suites).
- `git diff --check` clean; scoped secret review (no secrets/PII/GPS/photo content).
