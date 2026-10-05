# P35-T01 — Directory UUID ports and cache

Status: LOCAL_DONE on `codex/phase-35-live-discovery`. Task: P35-T01.
Binds B-BR-001 (source separation), B-BR-011 (no GPS/secrets in
origin/logs) and B-BR-014 (unknown location honesty). No Room migration,
no legacy CNPJ rewrite, no device run.

## Behavior (DDD)

- Domain `discovery/ServerStation`: platform UUID key (lowercased),
  trimmed `display_name`, `StationLocationQuality` wire
  (`unknown`/`city-centroid`/`reviewed`), nullable lat/lon pair (bounds
  `-90..90`/`-180..180`, both-present-or-absent), alphanumeric
  `cnpj_normalized` preserved verbatim with leading zeros, optional
  municipality/state/revision (revision UUID when present). Plus
  `ServerStationPage` (items + next cursor) and `NearbyServerStation`
  (station + `distance_m >= 0`).
- Domain ports `repository/ServerStationPorts`: network-only
  `ServerStationGateway` (list/nearby/detail, throws, never cache) and
  last-known `ServerStationCache` (page/detail replay, clear).
- Data `remote/DirectoryStationHttpClient`: `GET /v1/stations`
  (limit 1..100 + cursor), `GET /v1/stations/nearby` (lat/lon/radius
  100..15000/limit bounded, refused before network), `GET
  /v1/stations/{id}`; `Accept: application/json`, non-2xx/empty throws
  `IOException`, never double `/v1`, no GPS logging.
- Data `remote/DirectoryStationJsonCodec`: strict `Station`/`StationList`/
  `NearbyResult` decode, JSON-null-safe optionals, malformed UUID/
  coordinates refused via domain, negative `distance_m` refused.
- Data `repository/DirectoryStationRepository`: `GatewayImpl`
  (HTTP + codec, transport/parse → `IOException`) and synchronized
  `MemoryCache` (last page + detail index, lowercase keys).
- DI `di/DirectoryModule`: client from shared `@Named("apiOrigin")`
  staging origin; cache is `@Inject` singleton. Legacy CNPJ/offline rows
  untouched; old full-CNPJ identities resolve explicitly by callers.

## Validation

- RED: `ServerStationTest`, `DirectoryStationJsonCodecTest`,
  `DirectoryStationHttpClientTest` failed compilation before sources existed;
  codec list case caught a malformed test UUID + `JSONObject.NULL`
  `optString` pitfall (fixed with null-safe optionals + valid fixture).
- GREEN: `./gradlew :domain:test --tests
  ...ServerStationTest` → BUILD SUCCESSFUL (5/5).
- GREEN: `./gradlew :data:testDebugUnitTest --tests
  ...DirectoryStationJsonCodecTest --tests
  ...DirectoryStationHttpClientTest --tests
  ...DirectoryStationRepositoryTest` → BUILD SUCCESSFUL.
- Regression: `:data:kspDebugKotlin` (Hilt `DirectoryModule` resolves) +
  `:app:assembleDebug` → BUILD SUCCESSFUL.
- Contracts (unchanged backend): `vacuum lint` PASS (0 errors, 27 informs);
  `go test ./internal/platform/apicontract/...` PASS; real-PostGIS
  `directory/... + health` PASS (reused P34 baseline, no SQL touched).
- Live (no bypass): `curl -sS https://teste.abastevo.com.br/health/live`
  → `curl: (60)` (Fortinet middlebox CA per P34-T01); no `-k`, no
  trust-all, no HTTP fallback. Staging JSON still UNVERIFIED.
- `git diff --check` PASS; secret-surface review PASS (no
  secrets/PII/GPS/photos in diff).

## Limits and next

- BLOCKED_LIVE: staging list/nearby/detail JSON unverified from this
  runner (TLS trust gap); not a VPS outage diagnosis. No fake uptime.
- OWED: end manual/device batch, provider/media proof. No PR/CI/merge/wiki
  per ADR-018. iOS archived. G09 UNCERTIFIED.
- Next: P35-T02 Explore/station-detail integration on this branch.
