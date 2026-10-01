# P16-T04 — Location device and recovery acceptance

Status: LOCAL_DONE on `codex/phase-16-location-integrity` (phase PR
#44, draft). Issue: #43 (slice 4 of 4; issue stays open until the
phase PR merges). No camera, live-provider contact, polling or
background tracking in this slice.

## Slice acceptance (frozen before coding)

- T04 (this commit) — BUC-L03 denial/degraded recovery over the
  frozen T01 contract: `PortableLocationRecovery` maps one
  `FixRisk` to a stable disclosure + recovery vocabulary
  (`none` / `open-settings` / `disable-simulation` /
  `enable-precise` / `retry-fix` / `browse-only`). Every blocked
  verdict (DENIED, SIMULATED, DEGRADED, MANUAL, UNKNOWN) preserves
  free use (browse + manual + offline true); only the
  location-dependent proximity claim stays gated (only VERIFIED
  allows). Recovery is pure: zero source reads, zero clock/I/O,
  zero polling, zero background API — revocation surfaces as
  DENIED on the next one-shot read (no cached fix reuse),
  coarse stays DEGRADED (never promotes), offline/absent resumes
  via retry/manual. Static release guards assert no
  `ACCESS_BACKGROUND_LOCATION`, no continuous-update APIs and no
  iOS background flags in shipped sources. G16
  capabilities/limits documented below; hardware matrices stay
  release-horizon, recorded — never claimed.

## What changed

- `application/.../portable/PortableLocationRecovery.kt` —
  pure recovery mapping (disclosure reuses `fix-accepted` /
  frozen reason; `preservesFreeUse` always true by construction).
- `application/.../portable/PortableLocationRecoveryTest.kt` — 8
  suites (verified allow + none; denied/simulated/degraded/
  manual free-use + stable codes; UNKNOWN 4-reason retry matrix;
  revocation/coarse/offline-resume honesty; purity with zero
  source touch).
- `iosApp/.../PortableLocationRecovery.swift` +
  `PortableLocationRecoveryTests.swift` — 1:1 transcription,
  force-unwrap-free (check-mobile guard); implemented-unverified
  per P12 practice.
- `scripts/check-mobile.sh` — P16-T04 no-background guard
  (background permission, `requestLocationUpdates` /
  `requestUpdates(`, `startUpdatingLocation` /
  `allowsBackgroundLocationUpdates` refuse in shipped sources).
- No backend migration, no OpenAPI change, no provider wiring
  change: `LocationPermissionHandler` stays one-shot
  (`requestSingleUpdate` + last-known, cancellable, no
  background permission).

## RED → GREEN

- RED proven by mapping DENIED recovery to `none`: the
  `deniedBlocksClaimButPreservesFreeUse` case fails (wrong
  recovery code); GREEN on restore.
- `check-mobile` guards proven by the existing T02 injection
  pattern plus negative grep (no background strings exist to
  trip the new guard; any future insertion fails the gate).

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
bash scripts/check-mobile.sh
cd backend && GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/community/...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: Android baseline BUILD SUCCESSFUL (8 new recovery
suites green, 9 flow + 6 boundary + prior suites still green);
`check-mobile` full ok (static guards incl. new no-background
gate + Android green; Swift SKIP recorded outstanding, never
green); backend community unit green (no backend change, no
regression); `vacuum` 0/0/27; `diff --check`/secrets clean.

## G16 capabilities/limits (device + recovery)

- Capabilities: platform-marked simulation blocks claims on both
  platforms (Android `isMock`/`isFromMockProvider`, iOS
  `isSimulatedBySoftware`); UNKNOWN is honest (no-fix /
  source-missing / stale / clock-anomaly refuse VERIFIED);
  backend distrusts client flags (P16-T03 server recompute +
  PostGIS proximity + teleport review); denial/degradation never
  blocks browsing, manual lookup or offline functions.
- Limits: detects platform-marked simulation only (no
  universal spoof-proof claim); provider wiring + energy/latency
  matrices unproven until hardware run (release horizon);
  Swift compiles/runs on macOS only; powered-off/offline
  deletion semantics unchanged from P15 (no new promise).
- No unauthorized background tracking: one-shot reads only,
  manual path touches no provider, recovery touches no source,
  no background permission or update API ships.

## Limits (not claimed)

- No device run (Android/iOS hardware stays release-horizon);
  energy/latency figures are one-shot design bounds until
  device matrices.
- Swift transcription implemented-unverified until macOS run.

## Rollback

Code rollback = revert recovery files, Swift mirrors and the
new static guard section; portable contract returns to T02/T03
flow. No migration, no flag, no behavior change to existing
intake.

## Next

G16 exit review + phase closure (#43, same branch; issues
#40–#43 stay open until PR #44 merges).
