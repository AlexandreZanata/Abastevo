# P15-T02A — Shared portable photo pipeline

Status: LOCAL_DONE on `codex/phase-15-lightweight-photos` (phase PR
#39, draft). Issue: #35 (slice 1 of P15-T02; issue stays open
until the phase PR merges). No live-provider contact; no backend,
migration, camera or storage changes in this slice.

## Slice acceptance (frozen before coding)

- T02A (this commit) — shared portable photo core:
  `domain/.../portable/PortablePhoto` (frozen T01 budget mirrors:
  JPEG wire, 150 KiB target/256 KiB cap, 1600 edge, 2 MP integer
  bound, 3 attempts, 32 MiB hypothesis, 24 h transient TTL; exact
  JPEG/PNG/HEIC/HEIF intent allowlist; power-of-2
  `sampleSizeForBounds` so decodes never allocate full camera
  pixels; wire-cap and capture+24 h expiry rules) +
  `application/.../portable/PortablePhotoFlow` (prepare: intent →
  header probe → sample plan → bounded ≤3 encodes inside the cap
  → transient publish only on success; corrupt/oversize/
  unsupported refuse with stable codes; interrupted captures leave
  no partial entry; discard + launch/resume sweep) behind narrow
  `PhotoPorts` (decoder probe, encoder, cache, clock, ids).
  Swift 1:1 transcription of the pure values + XCTest vectors
  (implemented-unverified, P12 practice).
- T02B (next) — Android AES/GCM blob seal + Keystore transient
  cache + Hilt module, JVM-tested via seams (zero new deps).
- Later — native camera capture + macOS run (release horizon).

## What changed

- `domain/.../portable/PortablePhoto.kt` — frozen mirrors, exact
  allowlist, sample-size math, cap/expiry rules.
- `application/.../portable/PortablePhotoFlow.kt` +
  `PortablePhotoPorts.kt` — prepare/discard/sweep use cases over
  five narrow ports; no null ports, no I/O, no clock inside.
- `iosApp/.../PortablePhoto.swift` +
  `PortablePhotoTests.swift` — transcription with NOT
  COMPILED/NOT RUN banners; no new toolchain claims.
- Tests: `PortablePhotoTest` (6: budgets, allowlist matrix,
  sample plans incl. power-of-2 + inside-bounds proof, cap
  edges, expiry edges) + `PortablePhotoFlowTest` (8: happy
  publish, unsupported/corrupt refusals, over-budget retry×3,
  second-try win, encode-failed, sample-plan wiring,
  discard/sweep).

## RED → GREEN

- RED proven by admitting `image/gif` to the intent allowlist:
  `intentAllowlistIsExact` fails (animated input accepted);
  GREEN on restore.
- Incidental fix in-task: `FakeClock` name collision across same-
  package test files broke compilation; renamed to
  `FakePhotoClock`. One wrong test expectation fixed
  (`sweepExpired` returns the fake's 0, assertion corrected).

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
bash scripts/check-mobile.sh
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: Android baseline BUILD SUCCESSFUL (173 application
tests incl. 8 new flow tests; domain incl. 6 new photo tests);
`check-mobile` full ok (static guards + Android green; Swift
SKIP recorded outstanding, never green); `diff --check` clean;
secrets scan clean.

## Limits (not claimed)

- No native codec, sealed cache, camera capture or encrypted
  storage (T02B); no backend revalidation (T03).
- 32 MiB hypothesis still unmeasured (device evidence in T02B
  wiring/T05 at the latest).
- Swift compiles/runs on macOS only (release horizon).

## Rollback

Pure addition (two portable files + ports, two Swift files, two
test files): deleting them restores the tree. No migration, no
flag, no behavior change to existing flows.

## Next

P15-T02B Android sealed transient cache + Hilt (#35, same
branch). Issue #35 stays open until the phase PR merges.
