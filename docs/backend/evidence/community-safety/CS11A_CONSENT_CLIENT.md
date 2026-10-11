# CS11A — Training-consent Android client layers

Date: 2026-10-11. Status: LOCAL_DONE. Scope: domain ports,
HTTP client, local state store and use case with JVM tests only
— no ViewModel/panel/strings/DI, no screen placement, no
per-photo plumbing (CS11B/C). Head: maintained `dev` + this task.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md)
(CS11, B-BR-CS06), [server contract](../community-safety/CS09C_CONSENT_HTTP.md).
Rules: B-BR-CS06, BUC-CS05.

Validation: PASS

## Added (mirrors the helpfulness stack, CS08A/B)

- `domain/.../repository/TrainingConsentPorts.kt`: frozen
  reject kinds (`AUTH_REQUIRED/TERMS_OUTDATED/STALE_REVISION/
  CONFLICT/INVALID/TRANSPORT/OFFLINE`), receipt (state, revision,
  terms, replayed, deletion signal), owner status (granted, state,
  terms pair, revision, regrant flag) and the gateway
  (grant/revoke/status).
- `data/.../remote/TrainingConsentHttpClient.kt`: the three
  owner endpoints with strict parsing (required keys throw,
  absent consent parses to nulls + revision zero) and frozen
  `privacy.*` code mapping; transports, empty bodies and unknown
  statuses never claim consent; no client-side retries (server
  converges identical revisions).
- `data/.../local/preferences/TrainingConsentStateStore.kt`:
  switch preference plus per-device monotonic revision and the
  queued offline revocation (SharedPreferences, default OFF,
  synchronized revision bump); server stays authoritative with a
  reload every foreground (CS11B).
- `application/.../usecase/privacy/TrainingConsentUseCase.kt`:
  null caller is Guest with zero IO; grants refuse offline
  without queueing (fail-closed ON, blank terms refuse before
  IO); revokes queue offline as `RevocationPending` (toggle
  clears immediately); `syncPending` uploads the queued
  revocation and clears it; every decision burns one persisted
  revision; rejections carry fixed kinds.

## Validation (actual)

- `TrainingConsentHttpClientTest` 8/8 (receipt, replay flag,
  deletion signal, granted view, default-off view, 5 frozen
  codes, unknown/empty/malformed, blank-input refusal with zero
  requests) via MockWebServer. RED observed before GREEN
  (compile failure on the missing client).
- `TrainingConsentUseCaseTest` 8/8 (guest silence with zero IO,
  grant + toggle, per-decision revisions, offline grant
  refusal with no queue/revision/toggle/pending change,
  offline revoke queueing, sync upload + clear, fixed kinds,
  blank-terms refusal). RED before GREEN.
- Full `:data:testDebugUnitTest`, `:application:test`,
  `:domain:test` suites green (no regressions).
  `git diff --check` clean. No PII/secrets in code or this
  file. Emulator/device matrix stays CS14-owned.

## Limits / owed

- ViewModel/panel/strings/DI bindings and screen placement:
  CS11B. Per-photo opt-in plumbing at review: CS11C.
- Portuguese product copy: values/ stays English-only here
  (CS08B precedent); wired only to delivered behavior in CS11B.
- Verified production controller/contact/processors: no
  verified values exist yet — provisional digest stands, CS11
  close-out tracks it. `.invalid` contact stays staging-only.
- Soak SLO proof after 2026-10-12T16:11:11Z: unchanged.

Next microtask: CS11B — consent ViewModel/panel/DI/strings.
