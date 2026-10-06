# P32 UI completion

Status: PARTIAL; supported source/device slice tested, full acceptance blocked.
Branch: `codex/phase-32-profile-ui`.
Source: P33 `72b9e8f` (includes P34–P38); normal main merge `e7a1bff`.
User request 2026-10-05 authorizes remaining Android surfaces, actual device
acceptance and guarded integration. iOS remains archived; G09 requires its
separate production certification, not a mobile UI assertion.

## Bounded tasks and rules before implementation

- P32-T01A: anonymous HTTP profile and canonical UUID navigation/Compose.
  B-BR-P01/P02/P11/P12/P16, BUC-P01: allowlisted public business fields,
  representation separate from price quality, stale fallback never authority.
  Test malformed/foreign UUID DTOs, guest reads, unknown profile, failed refresh.
- P32-T02A: live owner-only claim export/status/cancel/reissue and bounded SAF
  signed-file submission, B-BR-P03–P10/P13–P16, BUC-P02–P05. Match actual
  backend roles/scopes/30-minute TTL; server acknowledgment required.
  Keep original file bytes; never collect keys/signing credentials. Private
  data stays memory-only; restore status via authenticated server, never
  persist a proof/URI/token in saved state. Lost URI asks re-selection.
  Do not auto-replay writes after process death. Retry reads/status first;
  explicit retry keeps the same submission key for claim creation.
- P32-T03A: Compose business edit/official reply on the existing server routes,
  BUC-P06–P09. Revision guard, grant/account/operator checks remain server-owned.
  Invitation/contest/reverification require real server commands; inspect
  support before displaying executable controls. Missing APIs are blocking
  obligations, not client-side grants or fake success.
- P32-T04A: meaningful ViewModel/HTTP/URI tests, Compose instrumentation,
  old API/large text/accessibility/device/performance rows. Freeze rows:
  connected physical Android + available old-API emulator; 2x font, bounded
  5 MiB file read, revoked URI, guest/expired session, restart/offline.
  TalkBack gestures and novice acceptance need actual observation.

Validation: affected domain/application/data/app tests, `assembleDebug`,
`assembleDebugAndroidTest`, `lintDebug`, backend HTTP/race + real PostGIS for
changed critical paths, OpenAPI, diff-check and secret scan. Record actual
commands/outcomes below; no acceptance inferred from compilation.

## P32-T01A actual source evidence

- RED: `:data:testDebugUnitTest --tests '*StationProfileHttpClientTest'`
  failed on absent client/revision/badge fields (19s).
- GREEN: `./gradlew :data:testDebugUnitTest --tests '*StationProfile*'
  :app:testDebugUnitTest --tests '*StationProfile*' :app:assembleDebug
  --no-daemon` PASS (56s). Initial Compose experimental opt-in compile
  failure corrected before acceptance.
- Privacy/identity/server refusal and stale-refresh tests pass. No private
  DTO/cache, price-quality badge or synthetic station. `git diff --check`
  and `bash scripts/scan-secrets.sh` PASS. Claim entry still auth-only until
  P32-T02A; end device evidence remains pending.

## P32-T02A actual source evidence

- Added owner claim routes/HTTP transport, real server scopes and expiry,
  role selection, list/status/reissue/cancel, SAF TXT declaration export,
  original PDF selection then explicit submit confirmation. Signature and
  authority remain independently reviewed. No PDF/key rendering or cache.
- Imported PDFs are memory-only, cap 5 MiB by stream (not provider size),
  wiped after submission/owner changes/ViewModel teardown. No persisted URI
  permissions; lost provider access requests re-selection. Reads/status
  precede writes; restart reconstructs owner status from the server. Stable
  open key survives transport retry in the live ViewModel, not auto-replay.
- RED backend reproduced missing export declaration id, 8 KiB proof cap,
  and owner status returning 404 after declaration consumption. Fixed
  additive id/state DTO + bounded base64 envelope; new LatestDeclaration
  query is status-only, never reused for proof/authority acceptance.
- `sqlc generate && sqlc vet`, stationprofile+apicontract `go test -race`
  PASS; real PostGIS stationprofile adapters `-race -count=1 -tags=integration`
  PASS (6.844s). Initial integration test table-name typo fixed; no failures
  deferred. `go vet` PASS; vacuum PASS (0 errors, 288 pre-existing naming/
  documentation warnings; direct CLI uses the default ruleset).
- `:application:test --tests '*StationProfileActionsTest'`, data profile/
  document tests + `assembleDebug` PASS (53s); claim ViewModel tests +
  `assembleDebug` PASS (21s). Covers guest/expired/reissued/terminal/no-store/
  original bytes/privacy/stable retry/restart/logout/server refusal.
- Live staging retry with normal TLS remains UNVERIFIED (`curl` 60);
  current device: physical 2311DRK48G Android 16/API36, emulator API26.
  Export TXT requires inclusion of exact declaration in an externally signed
  PDF; there is no server-generated PDF/export endpoint. Invites/contest/
  reverify commands are absent server-side and not invented client-side.

## P32-T03A source scope clarification

Backend inspection found `has_badge` always false and `ActiveGrant` ignoring
`valid_from`/`valid_to`. B-BR-P02/P14 require a real current badge and expired
capability denial; neither could be accepted as complete merely from a client
model. Public badge now uses bounded keyset grant pages with an injected
account-liveness port, a two-second lookup deadline (unavailable on error),
matching operator and validity window. No cross-module table read and no
private account/grant metadata in the public DTO. SQL management lookup now
also enforces the validity window. Existing server account/operator/scope
checks remain mandatory on each write. The grant-window test failed before
this fix (expired/future grant incorrectly allowed an edit).

- Additional risk run found an existing concurrent fixture ID counter race
  in `claimLifecyclePorts`; synchronized that counter without weakening
  concurrency assertions. Process API test also refused missing explicit
  disposable DB configuration; rerun uses `ANPFUEL_TEST_DATABASE_URL` for the
  existing local test DB, never the provided staging environment.

- GREEN management: `:domain:test --tests '*ProfileBusinessFieldsRuleTest'`
  + `:app:testDebugUnitTest --tests '*StationManagementViewModelTest'` +
  `:app:assembleDebug` PASS (2m17s). Service enum, UTF-8/scalar limits,
  revision conflict and revoked grant UI tested. Form sends changed fields
  only; official reply acknowledgment is required, no optimistic success.
- `sqlc generate && sqlc vet` PASS; grant expiry test RED then GREEN.
  Real PostGIS adapters + process API `-race -count=1 -tags=integration`
  PASS (19.729s / 4.779s) with explicit disposable DB. `go vet` and
  apicontract PASS. No migration changed.
- Invites/contest/reverification are explicitly unavailable because no
  executable server routes exist. P32-T03 full acceptance remains PARTIAL,
  despite the supported edit/reply slice being LOCAL_DONE. Do not close
  historical P32/P33/full acceptance tasks or claim G32/G33/G09 completion.

## Instrumentation dependency scope

The live device smoke must compile the production `:data` HTTP client whose
public constructor exposes OkHttp. Add only `androidTestImplementation(libs.okhttp)`
using the already present runtime pin 4.12.0 (Apache-2.0), with the same
transitive artifacts; no new runtime library/version/service. Test-only
exposure adds no production attack surface. Initial instrumentation compile
failed on the missing app androidTest classpath; corrected this dependency
configuration rather than deleting the live test.

## P32-T04A actual source and device evidence

- Final production behavior base `cbac4e8` plus T04A resource/permission tree.
  `./gradlew :app:testDebugUnitTest :domain:test :application:test
  :data:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest
  :app:lintDebug` PASS (2m57s): domain 445, application 271, data 239,
  app 183 tests pass. One existing opt-in live ANP catalog test is skipped;
  it is not a profile acceptance result. No profile test skipped.
- Initial lint failed on 217 missing keys in each of six supported languages
  and an unrecognized cached-location permission failure path. Added actual
  de/es/fr/ja/ru/zh-CN translations, preserving format placeholders; explicit
  SecurityException/provider-removal handling has a revocation regression.
  Final lint has 0 errors and 161 warnings; none suppressed or baselined.
- Final instrumentation correction + lint PASS (16s). The test-only provider
  initially crashed because its separate APK process lacks target Kotlin
  libraries. Replaced it with Java/Android-only code and strengthened failure
  assertions to distinguish size rejection from expired access.
- Physical Poco (ADB model 2311DRK48G), Android 16/API36: `adb shell am
  instrument -w -r -e class
  com.anpfuel.app.ui.stationprofile.StationProfileUiDeviceTest,com.anpfuel.app.ui.stationprofile.SignedClaimDocumentDeviceTest
  com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner`
  PASS, 7/7 (9.077s). Same explicit classes on API26 emulator PASS, 7/7
  (8.195s). Guest/revoked/stale controls, explicit PDF confirmation,
  scrolling at injected 2x Compose font scale, unknown-size 5 MiB stream,
  actual ContentResolver expired URI and oversize refusal are proved.
- Original synthetic 5,234,697-byte PDF: Poco 182ms, sampled Java heap
  14,376,240 -> 29,790,160 bytes, max 268,435,456; API26 emulator 198ms,
  heap 7,544,336 -> 30,331,912, max 536,870,912. These are one-run samples,
  not peak-heap/battery/low-memory pressure or full performance certification.
  Synthetic captures inspected: 2x text wraps and controls remain scrollable.
- Final APK SHA-256 `690fdb5259c12ec3cc72c6a52d39b52016a055b289bbb791d492ed3428402665`;
  instrument APK `4125be0fc1562c9abb5f2433d79c2fb2551d7551e65062f9c777a27b5e82c31b`.
  Both installed successfully on Poco and API26. Opened the actual Poco
  application afterward: ADB COLD launch TotalTime 974ms, one debug sample.
- Live read-only smoke on Poco FAILED twice, including final APK, with
  SSLHandshakeException / trust anchor unavailable before the directory
  response. Selected with `-e p32LiveStaging true -e class
  com.anpfuel.app.ui.stationprofile.StagingAnonymousReadDeviceTest`.
  Live smoke is explicitly opt-in, separate from deterministic source tests;
  its failure is recorded, not interpreted as an empty catalog or passed read.
  Workstation `openssl s_client -verify_return_error` independently received
  a Fortinet-issued certificate and failed issuer verification. No TLS bypass,
  device trust-store change, endpoint deployment or private live write occurred.
- `git diff --check` and scoped secret scan PASS. Logs/screenshots use
  synthetic data in local `/tmp`/debug cache, not real proofs or contributor GPS.

## Delivery and outstanding acceptance

Task issues: [T01A #118](https://github.com/AlexandreZanata/abastevo/issues/118),
[T02A #119](https://github.com/AlexandreZanata/abastevo/issues/119),
[T03A supported slice #120](https://github.com/AlexandreZanata/abastevo/issues/120),
[T04A #121](https://github.com/AlexandreZanata/abastevo/issues/121), milestone 21.
All remain open until actual guarded integration; no historical phase gate closed.
Tested source behavior committed as `359f848` and pushed. Cumulative
[PR #122](https://github.com/AlexandreZanata/abastevo/pull/122) is OPEN/DRAFT;
it is attached to this task. No required remote check is asserted, and no
finish, merge, issue closure or wiki publication occurred. Subsequent
record-only documentation does not change either tested APK/source behavior.
Main fetched again at `938fc1f`; strict protection, enforce-admins and required
`Quick verification` reverified. The cumulative lineage includes P34–P38,
catalog P25–P27/P29 and profile P30–P33; no duplicate merge of those ancestors.

Full P32/G32 and G33 acceptance remain blocked by missing P31-owned executable
invitation/contest/reverification contracts, live TLS/provider/private storage
proof and observed TalkBack/manual novice/low-memory/performance rows. The
[P30 freeze](../release-evidence/p30-t01-contract-freeze.md) and
[P38 union matrix](p38-t02-manual-matrix.md) remain authoritative obligations.
No fake client grants, trust-all success, production certificate, G09 release,
deployment, tag or wiki integration is inferred. Draft review/backup is allowed;
the final ready/check/finish/merge/wiki sequence waits for actual acceptance.

Final composition review also found a critical inherited contract gap:
`cmd/api/main.go` passes only `accountService.ValidateAccess` to ClaimHandler
and ProofHandler; those callbacks do not authenticate the existing contributor
key proof required by `STATION_PROFILE_CONTRACT_FREEZE.md`. Token expiry is
checked, but token possession alone is not the specified two-proof ceremony.
ProofPorts.Bytes is explicitly nil in the same composition and therefore
refuses intake with 503. Transport/fixture tests prove the current bounded
adapter behavior, not missing runtime authentication/storage acceptance.
These are P30/P31 runtime blockers; no claim/proof production readiness or
security certification is asserted by T02A. Do not mark the batch ready or
deploy this candidate until the source authentication gap is corrected and
its negative/replay/account-binding tests pass immediately.
