# P17-T04 — Cross-platform lifecycle and privacy exit

Status: LOCAL_DONE on `codex/phase-17-social-iphone-parity` (phase
exit task; draft PR #59 open). Issue: #58. No backend change, no
migration, no existing screen redesigned, no new string resources.

## Slice acceptance (frozen before coding)

- T04 (this commit) — Prove same-fixture account/social/media/
  location flows through native boundaries: revoked sessions block
  writes without IO, ownership/erasure refusals carry fixed kinds,
  24 h transient expiry and simulated-location denial hold, and
  diagnostics carry aliases/counts only. Tests first: shared
  280/agreement vectors, revocation, erasure kinds, expiry
  boundary, location verdicts, no-PII diagnostics. Live
  transport/Keychain/background/device rows stay explicit
  Mac-gated limitations (never marked ready).

## What changed

- `application/.../feedback/FeedbackLifecycleExitTest.kt` (6) —
  same fixtures (280/281, 6666 bp, zero-votes nil), revoked
  session blocks comment/vote/report with zero gateway calls,
  NOT_AUTHOR/COMMENT_NOT_FOUND/STALE_REVISION kinds, 24 h
  expiry boundary, SIMULATED/DENIED verdicts with no claim, and
  alias/count diagnostics without `@`.
- `iosApp/.../FeedbackLifecycleTests.swift` (6) — 1:1 XCTest
  transcription (280/281, 6666/nil, `.signInRequired` with nil
  op id, fixed NOT_AUTHOR/NOT_FOUND/STALE labels, TTL-1/TTL
  expiry, SIMULATED/DENIED denial, `@`-free diagnostics).
- `docs/mobile/p17-t04-lifecycle-exit.md` — G17 exit matrix with
  supported Android/JVM results and explicit Mac-gated iPhone
  rows; no device claim.

## RED → GREEN

- RED: new tests referenced the absent exit class/vectors
  (`Unresolved reference FeedbackLifecycleExitTest` JVM-side;
  XCTest bodies absent Swift-side).
- GREEN (this host): `./gradlew :domain:test :application:test
  :data:testDebugUnitTest :app:testDebugUnitTest
  :app:assembleDebug` BUILD SUCCESSFUL; `check-mobile
  --static-only` ok (incl. no force-unwrap); `diff --check`/
  secrets clean. Swift build/test stays BLOCKED until Mac
  access (same precedent as P12-T04), never claimed.

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
bash scripts/check-mobile.sh --static-only
git diff --check
bash scripts/scan-secrets.sh
```

Outcome recorded after the run below. Mac gate (BLOCKED):
`cd iosApp && swift build && swift test` plus simulator run
with synthetic-server binding and lifecycle proof (P18/G18).

## Limits (not claimed)

- Swift sources are IMPLEMENTED, NOT VERIFIED (no Xcode on
  Linux); no `swift build/test` evidence is claimed here.
- No live-server, Keychain, background-outbox or device-matrix
  proof in this slice; G17 device rows stay open until the Mac
  run. Rollback: feedback flag OFF preserves existing
  free/offline behavior and compatible API.
