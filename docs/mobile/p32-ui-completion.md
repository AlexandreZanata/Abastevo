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
