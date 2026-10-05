# P32 UI completion

Status: IN_PROGRESS. Branch: `codex/phase-32-profile-ui`.
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
