# P10-T02 — Community read ports and local cache

Status: LOCAL_DONE on `codex/phase-10-functional-integration` (phase PR
#54 pending, second task push). Issue: #46 (slice 2 of 8 local;
P10-T09 pilot stays deferred until G18/G09 and is not part of G10-LOCAL).
No ANP path touched, no legacy enum rename, no backend change.

## Slice acceptance (frozen before coding)

- T02 (this commit) — backend reads without removing ANP offline paths:
  domain `BackendPriceGroups` (wire `GASOLINE_ADDITIVED`, community stays
  null until P04, exact milli money) + `BackendPriceHttpGateway` /
  `BackendPriceCacheRepository` ports + 60s `BackendPriceCacheRule`
  (matches `Cache-Control: public, max-age=60`) + `CommunityReadsConfig`
  default OFF; application `GetCommunityPriceGroupsUseCase` (Disabled /
  Fresh+save / StaleCache-explicit / Unavailable); data Room
  `backend_price_cache` (key `station|fuel`, source/version/expiry) with
  explicit MIGRATION_4_5, OkHttp `BackendStationPriceHttpClient`, Room+HTTP
  impls, `CommunityReadsFlagStore` default OFF, preview base URL
  `.invalid` (P13 precedent, never resolves).

## What changed

- `domain/.../model/BackendPriceGroups.kt` — official/group/groups with
  UUID + 7-value wire + milli validation.
- `domain/.../repository/BackendPricePorts.kt` — network-only vs
  cache-only ports.
- `domain/.../rule/BackendPriceCacheRule.kt` — TTL 60s, stale/fresh.
- `domain/.../feature/CommunityReadsConfig.kt` — DISABLED default.
- `application/.../port/CommunityReadsFlagProvider.kt` + `usecase/community/GetCommunityPriceGroupsUseCase.kt`.
- `data/.../entity/BackendPriceCacheEntity.kt` + `dao/BackendPriceCacheDao.kt`.
- `AnpFuelDatabase` v4→v5, `MIGRATION_4_5`, `DatabaseModule`, `5.json`.
- `data/.../remote/BackendPriceGroupsJsonCodec.kt` (null-community
  refused) + `BackendStationPriceHttpClient.kt` (network-only).
- `data/.../repository/BackendPriceRepositories.kt` + `preferences/CommunityReadsFlagStore.kt`.
- `RepositoryModule` binds, `CommunityModule` preview URL, `UseCaseModule` wiring.
- Tests: 5 domain-model + 3 rule + 5 use-case (disabled/fresh/down-no-cache/down-stale/blank) + 3 codec + 2 HTTP (path/filter, down/empty) + 3 cache-repo + 1 migration-unit + 1 androidTest V4→V5 (vehicles/history preserved).

## RED → GREEN

- RED proven by new symbols before implementation (same pattern as
  P10-T01): `BackendPriceHttpGateway`, `BackendPriceCacheRule`,
  `GetCommunityPriceGroupsUseCase`, `BackendPriceCacheDao`,
  `BackendStationPriceHttpClient` did not exist; new tests referenced
  them. GREEN after adding bounded adapters only.

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest --no-daemon --rerun-tasks
./gradlew :app:testDebugUnitTest :app:assembleDebug --no-daemon
bash scripts/check-mobile.sh --static-only
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: `:domain:test` + `:application:test` + `:data:testDebugUnitTest`
green (incl. 22 new unit suites listed above); `:app:testDebugUnitTest`
+ `assembleDebug` BUILD SUCCESSFUL (ANP screens unchanged);
`check-mobile --static-only` ok; `diff --check`/secrets clean.
Limits: `:data:connectedDebugAndroidTest` NOT run locally (no
emulator); V4→V5 device migration test added for CI/phase exit.
No backend change; no production URL invented.
