# P35-T02 — Explore and station detail integration

Status: LOCAL_DONE on `codex/phase-35-live-discovery`. Task: P35-T02.
Binds B-BR-001 (community primary, ANP dated reference separate),
B-BR-011 (no GPS/secrets in origin/logs) and B-BR-014 (unknown location
honesty). Flag-gated additive change; legacy ANP/offline journeys
untouched; no device run.

## Behavior (DDD)

- Application `usecase/directory/GetServerStationsUseCase`:
  `GetServerStationsUseCase` (list) + `GetServerStationDetailUseCase`
  (detail), both gated by `CommunityReadsFlagProvider` (OFF default).
  Disabled never touches network/cache. Success saves last-known
  page/detail; transport failure replays cache as `StaleCache` or reports
  `Unavailable` — never an invented station, never a legacy rewrite.
- `StationsViewModel` (additive, nullable use cases default null):
  `serverStations`, `isServerLoading`, `serverFromCache`, `serverError`,
  `selectedServerStation`, `serverDetailFromCache`. `loadServerStations()`
  keeps the last good list on failure and reports first-load errors
  honestly. `onServerStationSelected(uuid)` opens canonical detail or
  yields none for unknown UUID. `onServerStationNavigate` emits
  `LaunchMaps("lat,lon")` only for reviewed coordinates; unknown
  locations emit nothing (no fabricated destination).
- UI: `ServerStationRow` (name + quality + coordinates) and
  `ServerStationDetailSheet` (quality, coordinates or explicit
  no-coordinates label, CNPJ short, community-primary slot reusing the
  honest P20 pending/confidence copy, route/update actions, stale-cache
  badge). `StationsScreen` renders the server section only when
  non-empty, the unavailable note only when empty + error, and the
  server sheet only when selected. en/pt-BR strings added (7 each).
- DI: `RepositoryModule` binds `ServerStationGateway/Cache`;
  `UseCaseModule` provides both use cases from the shared staging
  origin. Hilt graph resolves.

## Validation

- GREEN: `:application:test --tests ...GetServerStationsUseCaseTest`
  → BUILD SUCCESSFUL (5/5: disabled/fresh/stale/unavailable/blank-id).
- GREEN: `:app:testDebugUnitTest --tests
  ...StationsServerDiscoveryTest` → BUILD SUCCESSFUL (5/5: fresh list,
  stale label + honest first failure, select/dismiss, unknown yields
  none, unknown-location navigation emits nothing).
- Regression: existing `StationsViewModelTest` untouched and PASS
  (constructor defaults preserve behavior); `:domain:test` +
  `:application:test` full PASS; `:app:assembleDebug` PASS (resources
  + Hilt + Compose compile).
- Contracts (unchanged backend): `vacuum lint` PASS; `go test
  ./internal/platform/apicontract/...` PASS; live `curl` 60 unchanged
  (no bypass, no trust-all, no HTTP fallback).
- `git diff --check` PASS; `scan-secrets.sh` PASS.

## Limits and next

- BLOCKED_LIVE: server list/detail JSON unverified from this runner
  (TLS trust gap); community price-group reads against UUID stations
  use existing `/prices` ports and stay owed live proof. No fake uptime.
- OWED: end manual/device batch, provider/media proof. No PR/CI/merge/wiki
  per ADR-018. iOS archived. G09 UNCERTIFIED.
- Next: P35-T03 search/degraded discovery regression on this branch.
