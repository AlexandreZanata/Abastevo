# P34-T01 — Staging contract inventory and trust verification

Status: LOCAL_DONE on `codex/phase-34-vps-connection`, base `0067e86`.
Task: P34-T01. Binds B-BR-011 (privacy, no GPS in logs/responses), B-BR-014
(unknown location honesty) and directory read honesty (no request-point echo,
no-store nearby). No server writes, no fixture seeding, no deployment.

## Client/flag inventory (source truth, no behavior change)

- `data/.../di/AuthModule.PREVIEW_BASE_URL = https://api.anpfuel.example.invalid`
  (`AccountHttpApi`, 10s connect/read, `Accept: application/json`).
- `AnonymousModule.PREVIEW_BASE_URL` same placeholder
  (`AnonymousProofHttpClient`: `POST /v1/identity/challenges`,
  `POST /v1/contributors`, `POST /v1/contributors/me/keys/rotate`, 64 KiB cap).
- `CommunityModule.PREVIEW_BASE_URL` same placeholder
  (`BackendStationPriceHttpClient`: `GET /v1/stations/{id}/prices`;
  `CommunityVoteHttpClient`: `POST /v1/observations/{id}/confirmations|disputes`).
- `ContributionModule.PREVIEW_BASE_URL` same placeholder
  (`ContributionUploadHttpClient`: `POST /v1/uploads`, `PUT /v1/uploads/{id}/bytes`,
  `POST /v1/uploads/{id}/complete`, `POST /v1/observations`).
- `FeedbackModule` reuses `CommunityModule.PREVIEW_BASE_URL`
  (`FeedbackHttpClient`: 13 P14/P22 routes under `/v1/feedback/*`).
- All placeholders are RFC 2606 `.invalid` (never resolve); reads stay disabled
  until explicit staging configuration (P34-T02 owns it).
- Flags default OFF, rollback OFF: `CommunityReadsFlagStore`,
  `CommunityVoteFlagStore`, `FeedbackFlagStore`, `CaptureOcrFlagStore`,
  `ContributionOutboxFlagStore`, `AnonymousContributionFlagStore`
  (`SharedPreferences ..._enabled = false`).
- `OkHttpClientFactory`: 30s connect, 60s read/write, `HttpsOnlyInterceptor` +
  `AnpUserAgentInterceptor` + `RetryInterceptor(max=3)`. No trust-all,
  no custom `HostnameVerifier`, no `-k`, no HTTP fallback. `HttpsOnlyInterceptor`
  throws `Cleartext HTTP is not allowed` for `http://`.
- `LocationModule.allowTestInjection = BuildConfig.DEBUG` only; nearby GPS stays
  transient, never logged/cached per contract.

## Deployed contract shapes (OpenAPI `1.0.0-draft`)

- `GET /health/live` -> `{"status":"ok"}`; `GET /health/ready` -> 200 `ready`
  or 503 `not_ready`, both `Cache-Control: no-store`.
- `GET /v1/stations` (q/state/municipality_code/limit 1..100/cursor) ->
  `StationList`, `Cache-Control: public, max-age=60`.
- `GET /v1/stations/nearby` (lat -90..90, lon -180..180, radius_m 100..15000,
  limit 1..100) -> `NearbyResult`, `Cache-Control: no-store`, never echoes
  request lat/lon.
- `GET /v1/stations/{station_id}` (UUID) -> `Station`, `ETag` +
  `Cache-Control: public, max-age=30, s-maxage=60`; unknown UUID 404
  `station.not-found`, malformed 400.
- `GET /v1/stations/{station_id}/prices` and `/official-prices` per
  `PriceGroups` (community primary, ANP dated reference).
- No `/version` endpoint in backend or OpenAPI; deployed revision cannot be
  inferred from a probe. Headers verified in source: `Accept: application/json`,
  `Idempotency-Key`/`X-Nonce` on writes, `If-None-Match`/`ETag` on detail.

## Live staging verification (no validation bypass)

Origin: `https://teste.abastevo.com.br`. Only the plan sample point
`lat=-23.55&lon=-46.63` was used; no user GPS collected, stored or logged.

- `openssl s_client -connect teste.abastevo.com.br:443 -servername ...`:
  `depth=0 CN = abastevo.com.br`, issuer
  `C=US/ST=California/L=Sunnyvale/O=Fortinet/OU=Certificate Authority/CN=FG6H0FTB23902129`,
  `verify error:20 unable to get local issuer certificate`,
  `Verify return code: 20`. Single-chain FortiGate issuance, not a public CA.
- `curl -sS -m 12` (system CA, no `-k`) for `/health/live`, `/health/ready`,
  `/v1/stations?limit=1`, `/v1/stations/nearby?lat=-23.55&lon=-46.63...`:
  all fail `curl: (60) SSL certificate problem: unable to get local issuer
  certificate`. No body received, no insecure retry performed.
- Result: live JSON UNVERIFIED from this runner. This records a runner/path
  trust gap (Fortinet middlebox CA), not a VPS outage diagnosis and not proof
  the server certificate is invalid. Staging data availability remains UNKNOWN;
  empty-200 handling was not exercised live. P34-T02 must not ship a trust-all
  client or HTTP fallback to work around it.

## Local contract proof (meaningful, before consumers)

- `vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml
  --no-update-check`: PASS, 0 errors, 27 informs, quality 97/100.
- `cd backend && go test ./internal/platform/apicontract/...`: PASS.
- `go test ./internal/platform/health/... ./internal/modules/directory/adapters/http/...`: PASS (unit, no DB).
- Real PostGIS: `ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable go test -count=1 -tags=integration ./internal/modules/directory/...`: PASS (7 packages, includes `TestNearbyHonesty` 400 on lat=91 and radius=50, no echo, no-store; `TestDetailEnvelope` 404 unknown UUID + 400 malformed + honest unknown location).
- Android: `./gradlew :data:testDebugUnitTest --tests ...OkHttpClientFactoryTest --tests ...HttpsOnlyInterceptorTest --tests ...BackendStationPriceHttpClientTest`: BUILD SUCCESSFUL (HTTPS-only + client routing proven; no device run per construction rule).
- `git diff --check`: PASS. Secret-surface review: no secrets/PII/precise GPS/photos in diff; probe used only the public sample coordinate.

## Limits and next

- BLOCKED_LIVE: staging reads (health/directory/invalid-lat/unknown-UUID live shapes) pending trusted chain + deployed contract/data availability. No fake uptime claimed.
- OWED: end manual/device batch (deferred per 2026-10-02 directive), provider/media proof, P34-T02 explicit origin configuration.
- Next: P34-T02 one explicit Android environment configuration on this branch.
