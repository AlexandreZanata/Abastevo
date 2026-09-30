# P13-T05A — Shared portable auth (login flows + ports)

Status: LOCAL_DONE on `codex/phase-13-free-accounts` (phase PR #27,
draft). Issue: #26 (slice 1 of P13-T05; slices per ROADMAP rules).
G12-basic satisfied; device evidence stays deferred to release per
owner decision. No live-provider contact.

## Slice acceptance (frozen before coding)

- T05A (this commit) — shared portable auth core: `PortableAuth`
  domain values (providers/issuers/audience, code shape, session
  liveness, nonce bind, strict callback parse) + `AuthFlow` use
  cases (email request/consume, refresh, logout-everywhere,
  rehydrate, provider begin/complete, self-delete) behind narrow
  ports (`AuthSessionStore`, `AuthWallClock`, `AuthAccountApi`,
  reused `PortableNonceSource`); server verdicts pass through
  verbatim for UI mapping. Swift 1:1 transcription of the pure
  core + XCTest vectors (implemented-unverified, P12 practice).
- T05B (next) — Android Keystore session adapter + HTTP API
  adapter (+ dep license review) + app wiring.
- Later — screens, Keychain adapter + macOS run (release horizon).

## What changed

- `domain/.../portable/PortableAuth.kt` — frozen backend mirrors
  (P13-T01): google/apple allowlist + issuers, `anpfuel-backend`
  audience, 6-digit shape (hint only), 900s/30d liveness,
  `anpfuel://auth/callback` strict parse (scheme/host/path, no
  blanks, no duplicated keys), exact nonce/state/provider bind
  refusing substitution (proof never sent on mismatch).
- `application/.../portable/PortableAuthFlow.kt` — `AuthFlow`
  over `AuthPorts`: email hint validation, session persist,
  refresh-before-expiry, local-first logout (storage always
  clears, even offline), rehydrate (Active/NeedsRefresh/LoggedOut,
  corrupt blobs refuse), provider begin (fresh nonce+state) /
  complete (spent-nonce replay refusal + mismatch refusal without
  API touch; server `nonce-reused` marks spent), self-delete.
- `iosApp/.../PortableAuth.swift` + `PortableAuthTests.swift` —
  transcription with NOT COMPILED/NOT RUN banners; no new
  toolchain claims.
- Tests: `PortableAuthTest` (5) + `PortableAuthFlowTest` (11):
  allowlist, shape, liveness edges, bind/substitution matrix,
  strict parse matrix, signup persist, malformed-local,
  verbatim verdicts, rotation, dead-session relogin, offline
  logout, process-death rehydrate, corrupt blob, link-once +
  replay, substitution-never-sends, delete-clears.

## RED → GREEN

- RED proven by neutering the spent-nonce replay guard:
  `providerLinkBindsNonceAndSendsOnce` fails (replay reaches the
  API); GREEN on restore.
- Baseline import-ban guard (`kotlinx.coroutines` etc.) holds:
  portable stays sync, coroutines remain native-side.

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
make quick-verify
git diff --check
```

Outcome: domain+application 491 tests (475 baseline + 16 new),
0 failures/errors/skips; data/app suites + `assembleDebug`
SUCCESS; `quick-verify` selection=full ok; `diff --check` clean;
secrets scan clean. Swift transcription statically reviewed
against the Kotlin vectors only — never claimed compiled.

## Limits (not claimed)

- No Keystore/Keychain adapters, no HTTP adapter, no screens;
  API/storage/clock/nonce ports are faked in tests.
- Swift runs on macOS only (release horizon); flow-class
  adoption in the shell likewise.
- Live providers stay P13-T05/G13 device evidence (deferred).

## Rollback

Pure addition under `portable/` + `iosApp`: deleting the four
Kotlin/Swift files restores the tree. No migration, no flag.

## Next

P13-T05B Android adapters (#26, same branch). Issue #26 stays
open until the phase PR merges.
