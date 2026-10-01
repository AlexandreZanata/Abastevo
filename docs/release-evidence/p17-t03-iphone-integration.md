# P17-T03 — Swift iPhone full feature integration

Status: LOCAL_DONE on `codex/phase-17-social-iphone-parity` (third
task; phase draft PR #59 open). Issue: #57. No backend change, no
migration, no existing screen redesigned, no new Android resources.

## Slice acceptance (frozen before coding)

- T03 (this commit) — Thin Swift parity around the P17-T01 shared
  contract with the P17-T02 Android guards: pure display copy (280
  counter, floor agreement, fixed reject labels), flag/session/text/
  vote guards with no IO on local refusal, stable-op retry identity
  and server-kind → label mapping. Tests first: agreement 2/1,
  zero-votes, 280/281, fixed codes, flag OFF, blank session, over-280,
  bad vote, retry identity. Live URLSession execution, Keychain
  session store, background outbox replay and full panel composition
  stay explicit Mac-gated limitations (never marked ready).

## What changed

- `iosApp/Sources/AnpFuelCore/FeedbackDisplay.swift` — pure port of
  Kotlin `FeedbackDisplay`: `charsRemaining` via `PortableText`
  (280 scalars), `agreementLine(valid:invalid:)` with floor
  truncation + one decimal (`66.6% · 2 valid / 1 invalid`) and
  `No votes` for zero, `FeedbackRejectKind` (14 cases) with the
  exact Android labels (TARGET/STARS/EMPTY/TOO_LONG/NOT_AUTHOR/
  STALE/NOT_FOUND/SELF_VOTE/CHOICE/REPORT/QUOTA/TRANSPORT/SIGN_IN).
  Ratings transport refusal stays GATE_REQUIRED ("ratings transport
  not published", recorded P14 gap): the ViewModel exposes no rating
  action, mirroring Android.
- `iosApp/Sources/AnpFuelCore/FeedbackFlow.swift` — thin state
  machine porting `FeedbackViewModel` guards synchronously:
  flag OFF → `.disabled` (no IO, rollback is flag OFF); blank
  account → `.signInRequired` (no IO, anonymous reads stay open);
  over-280/blank text, blank reply target/revision, bad vote
  choice and blank report → `.rejected("INVALID")` (no IO);
  single-flight `.submitting` guard; `retry()` replays the same
  stable op id (`pendingOpId` equality); `rejectedState(kind:)`
  maps server kinds to the fixed Android labels for rollback.
- `iosApp/Sources/AnpFuelShell/FeedbackView.swift` — thin SwiftUI
  view rendering the same fixed copy (counter, agreement, state
  labels) with plain `Text` only; no network IO.
- `iosApp/Tests/.../FeedbackDisplayTests.swift` (4) — 1:1 of
  `FeedbackDisplayTest` (agreement, zero-votes, 280/281/é,
  fixed codes incl. double NOT_FOUND + SIGN_IN).
- `iosApp/Tests/.../FeedbackFlowTests.swift` (6) — flag OFF,
  blank session, over-280/blank, bad vote, retry identity,
  server-kind label mapping.

## Parity checklist (Android → iPhone)

- 280 counter / agreement / fixed labels: MATCH (pure ports).
- Flag/session/text/vote guards + no-IO refusals: MATCH.
- Stable-op retry identity + single-flight: MATCH.
- Ratings refusal (no fake success): MATCH (explicit GATE_REQUIRED).
- Live HTTP over 10 P14 routes / Keychain / background replay /
  full panel / device run: DEFERRED to macOS (Xcode 26.4,
  synthetic server + simulator/device), never claimed.

## RED → GREEN

- RED: new XCTest files referenced absent
  `FeedbackDisplay/FeedbackFlow` types (plus no Swift toolchain
  on Linux to execute them — recorded, never claimed green).
- GREEN (this host): `./gradlew :domain:test :application:test
  :data:testDebugUnitTest :app:testDebugUnitTest
  :app:assembleDebug` all BUILD SUCCESSFUL (up-to-date, 8s);
  `check-mobile --static-only` ok (incl. no force-unwrap);
  `diff --check`/secrets clean. Swift build/test stays
  BLOCKED until Mac access (same precedent as P12-T04).

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
bash scripts/check-mobile.sh --static-only
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: BUILD SUCCESSFUL; static-only ok; diff-check/secrets
clean. Mac gate (BLOCKED, not run):
`cd iosApp && swift build && swift test` plus
`xcodebuild test -scheme AnpFuel -destination
'platform=iOS Simulator,name=iPhone 16'` with synthetic-server
transport binding and device/lifecycle proof (P17-T04/G17).

## Limits (not claimed)

- Swift sources are IMPLEMENTED, NOT VERIFIED (no Xcode on
  Linux); no `swift build/test` evidence is claimed here.
- No URLSession/Keychain/background wiring in this slice; no
  full SwiftUI panel flow; no device-matrix proof (P17-T04/G17).
- Rollback: feedback flag OFF preserves existing free/offline
  behavior and compatible API.
