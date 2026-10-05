# P34-T02 — One explicit Android environment configuration

Status: LOCAL_DONE on `codex/phase-34-vps-connection`. Task: P34-T02.
Binds B-BR-011 (no secret/identity/GPS in origin, logs or errors) and existing
transport honesty (HTTPS-only, bounded timeouts/retries, stale sessions fail
closed). No backend/SQL/migration change; no device run.

## Behavior (DDD)

- Single source of truth: `data/.../remote/ApiEnvironment` (`PREVIEW`,
  `STAGING`). `STAGING.origin = https://teste.abastevo.com.br`; `PREVIEW` is
  the RFC 2606 non-resolving placeholder. Release stays explicit and separately
  certified: never inferred from staging.
- `requireValidOrigin` refuses blank, non-HTTPS, missing host, userinfo,
  query/fragment and any origin path (notably `/v1`), so route joining can
  never silently produce `/v1/v1/...`. `join(path)` requires a leading `/`
  and returns `origin.trimEnd('/') + path`.
- Hilt: `NetworkModule.provideApiEnvironment()` (`@Named("apiOrigin")`)
  returns `STAGING` explicitly for P34-P38 construction. Auth, anonymous
  identity, community (prices+votes), feedback and contribution providers all
  inject `@Named("apiOrigin")` and use `environment.origin`. Per-module
  `PREVIEW_BASE_URL` constants stay as reference values only; DI no longer
  reads them (except the unused client companion default).
- Preserved: `OkHttpClientFactory` 30s/60s/60s + `HttpsOnlyInterceptor` +
  retry max 3; `AuthModule` 10s account client; all feature flags default OFF
  (rollback OFF); `KeystoreSessionStore` foreign/stale blobs fail closed to
  logged-out and `FeedbackHttpClient` enforces `GATE_REQUIRED` without a live
  session, so a preview-to-staging switch never reuses authority silently.

## Validation

- RED: `ApiEnvironmentTest` failed compilation before `ApiEnvironment.kt`
  existed (`compileDebugUnitTestKotlin FAILED`).
- GREEN: `./gradlew :data:testDebugUnitTest --tests
  ...ApiEnvironmentTest` → BUILD SUCCESSFUL (staging/preview values, 9-route
  join without double `/v1`, malformed/non-HTTPS refusal, no secret/GPS in
  origin).
- Regression: same suite plus `OkHttpClientFactoryTest`,
  `HttpsOnlyInterceptorTest`, `BackendStationPriceHttpClientTest`,
  `FeedbackHttpClientTest`, `CommunityVoteHttpClientTest`,
  `ContributionUploadHttpClientTest`, `AnonymousProofHttpClientTest`,
  `AccountHttpApiTest`, `KeystoreSessionStoreTest` → BUILD SUCCESSFUL.
- Compile: `:data:compileDebugKotlin` executed GREEN; `:app:kspDebugKotlin
  --rerun-tasks` → BUILD SUCCESSFUL (Hilt graph resolves `@Named("apiOrigin")`);
  `:app:assembleDebug` → BUILD SUCCESSFUL.
- Contracts (unchanged backend): `vacuum lint` PASS (0 errors, 27 informs);
  `go test ./internal/platform/apicontract/... ./internal/platform/health/...`
  PASS. No migrations touched.
- Live (no bypass): `curl -sS https://teste.abastevo.com.br/health/live`
  → `curl: (60) SSL certificate problem` (Fortinet middlebox CA per P34-T01);
  no `-k`, no trust-all, no HTTP fallback introduced.
- `git diff --check` PASS; secret-surface review PASS (no secrets/PII/GPS in
  diff; origin carries no token/key/coordinates by test).

## Limits and next

- BLOCKED_LIVE: staging JSON still unverified from this runner (trust gap);
  not a VPS outage diagnosis. No fake uptime.
- OWED: end manual/device batch, provider/media proof, P34-T03 synthetic
  catalog spec. No PR/CI/merge/wiki per ADR-018. iOS archived. G09 UNCERTIFIED.
- Next: P34-T03 on this branch.
