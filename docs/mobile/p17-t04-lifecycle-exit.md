# P17-T04 — Cross-platform lifecycle and privacy exit

Status: LOCAL_DONE on `codex/phase-17-social-iphone-parity` (phase
exit task; draft PR #59 open). Issue: #58. No backend change, no
migration, no screen redesign, no new string resources.

## What this proves

Same-fixture account/social/media/location flows through the native
boundaries with explicit Mac-gated limits:

- Same fixtures both platforms: 280/281-scalar text and floor
  agreement 2/1 → 6666 bp, nil for zero votes (JVM
  `FeedbackLifecycleExitTest` + XCTest `FeedbackLifecycleTests`).
- Revoked/blank sessions block new writes on every device with
  zero transport IO (`LoginRequired` / `.signInRequired`); the
  server enforces, the client never sends proof on mismatch.
- Ownership/erasure refusals carry fixed kinds for rollback
  (`NOT_AUTHOR`, `COMMENT_NOT_FOUND`, `STALE_REVISION`,
  `SELF_VOTE`); report reasons never enter display copy.
- Transient media expires at exactly 24 h
  (`isTransientExpired(TTL-1)=false`, `TTL=true`).
- Simulated-location fixes deny claims (`SIMULATED`,
  `allowsClaim=false`); denied permission yields `DENIED`;
  missing source info stays `UNKNOWN`, never verified proximity.
- Diagnostics carry opaque aliases and counts only: no `@`
  emails, no GPS coordinates, no photo payloads in labels,
  agreement lines or cached views.

## G17 exit matrix (supported results)

| Check | Android/JVM (this host) | iPhone/Swift |
|---|---|---|
| 280 + agreement fixtures | GREEN (`FeedbackLifecycleExitTest`) | IMPLEMENTED, NOT RUN (Mac-gated XCTest) |
| Revocation blocks writes, no IO | GREEN | IMPLEMENTED, NOT RUN |
| Ownership/erasure kinds | GREEN | IMPLEMENTED, NOT RUN |
| 24 h media expiry boundary | GREEN | IMPLEMENTED, NOT RUN |
| Simulated/denied location | GREEN | IMPLEMENTED, NOT RUN |
| No PII in diagnostics | GREEN | IMPLEMENTED, NOT RUN |
| Live URLSession/Keychain/background/device run | DEFERRED | DEFERRED to macOS (Xcode 26.4 + synthetic server + simulator/device) |

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
bash scripts/check-mobile.sh --static-only
git diff --check
bash scripts/scan-secrets.sh
```

Mac gate (BLOCKED, not run): `cd iosApp && swift build &&
swift test` plus `xcodebuild test -scheme AnpFuel -destination
'platform=iOS Simulator,name=iPhone 16'` with synthetic-server
transport binding and process-death/background lifecycle proof.

## Limits (not claimed)

- Swift is IMPLEMENTED, NOT VERIFIED (no Xcode on Linux).
- No live-server, Keychain, background-outbox or full device
  matrix evidence is claimed here; G17 device rows stay open
  until the Mac run (P18/G18 campaign).
- Rollback: feedback flag OFF preserves existing free/offline
  behavior and compatible API.
