# P35-T03 — Search and degraded discovery regression

Status: LOCAL_DONE on `codex/phase-35-live-discovery`. Task: P35-T03.
Binds B-BR-011 (transient GPS: never persisted/logged/cached) and
B-BR-014 (honest unknown/no-fix states). Additive; legacy search,
pagination and expert flows untouched; no device run.

## Behavior (DDD)

- Application `GetNearbyServerStationsUseCase` (same file): flag-gated
  transient nearby lookup. Disabled never touches network. Success
  returns `Fresh` with no cache write (coordinates stay transient by
  construction). Failure returns `Unavailable` — no stale-position
  replay. Bounds refused before network by the T01 client.
- `StationsViewModel` (additive):
  - `serverLoadJob`: in-flight server loads are cancellable, so a
    restart/refresh lands only the latest result (deterministic
    first-query/restart/cancellation case).
  - `refreshServerStations()`: reloads the page and re-resolves the
    selected UUID when still present; a failed refresh keeps cached
    stations exactly like the legacy P20-T02 recovery.
  - `loadNearbyServerStations()`: GPS-denied without grant requests
    permission and loads nothing (manual city browsing keeps working);
    granted fix runs the transient lookup; missing fix or transport
    failure reports an honest error (`location unavailable` / cause),
    never a stale position. New state: `nearbyStations`,
    `isNearbyLoading`.
  - Fixed along the way: restored the dropped
    `BuildStationNavigationQueryUseCase` import (KSP `NonExistentClass`
    failure) — no behavior change.
- Tests mirror the existing `backgroundScope + UnconfinedTestDispatcher`
  collection pattern after one emission-timing failure (plain `launch`
  collector missed the buffered permission request; fixed, not weakened).

## Validation

- GREEN: `:application:test --tests
  ...GetServerStationsUseCaseTest` → BUILD SUCCESSFUL (7/7: + nearby
  fresh/unavailable/disabled).
- GREEN: `:app:testDebugUnitTest --tests
  ...StationsServerDiscoveryTest` → BUILD SUCCESSFUL (8/8: + denied
  requests permission and loads nothing, granted fix loads transient
  results, refresh preserves selection).
- Regression: full `:domain:test` + `:application:test` +
  `:data:testDebugUnitTest` + `:app:testDebugUnitTest` (incl. untouched
  `StationsViewModelTest`) PASS; `:app:assembleDebug` PASS;
  `:data:kspDebugKotlin` PASS (Hilt graph resolves new nearby use case).
- Contracts (unchanged backend): `vacuum lint` PASS; `go test
  ./internal/platform/apicontract/...` PASS; live `curl` 60 unchanged
  (no bypass). Nearby GPS probes stay limited sample-point reads when
  trust is resolved; never a load campaign.
- `git diff --check` PASS; `scan-secrets.sh` PASS.

## Limits and next

- BLOCKED_LIVE: nearby/list/detail JSON unverified from this runner
  (TLS trust gap); cursor-pagination live walk and provider GPS proof
  stay owed to the end manual/device batch. No fake coverage.
- OWED: end manual/device batch, provider/media proof. No PR/CI/merge/wiki
  per ADR-018. iOS archived. G09 UNCERTIFIED.
- Next: P35 phase exit (LOCAL_DONE checkpoint + evidence), then P36
  (`codex/phase-36-live-community` via `--from-checkpoint`).
