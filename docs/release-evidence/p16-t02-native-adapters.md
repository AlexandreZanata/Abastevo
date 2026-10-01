# P16-T02 — Native mock and simulation adapters

Status: LOCAL_DONE on `codex/phase-16-location-integrity` (phase PR
#44, draft). Issue: #41 (slice 2 of 4; issue stays open until the
phase PR merges). No backend intake, camera, polling, blacklist or
live-provider contact in this slice.

## Slice acceptance (frozen before coding)

- T02 (this commit) — native simulation boundary over the frozen
  T01 contract: `application/.../portable/PortableLocationFlow`
  (one-shot `assess` + `authorizeClaim` + stable disclosure codes)
  behind narrow `LocationPorts` (one-shot `LocationSignalSource`,
  `LocationEnvironment`); Android `AndroidLocationSignals`
  (best last-known fix, framework mock flag with API-31 branch,
  elapsed-realtime age, permission-first mapping, never requests
  updates) + pure `LocationReading` boundary reduction + Hilt
  `LocationModule` (`BuildConfig.DEBUG` feeds the release guard);
  Swift `LocationSignals` Core Location reduction + XCTest
  (implemented-unverified, P12 practice). OS-provided source
  signals only: no app blacklists, no busy polling, no
  developer-option blanket denial, no background tracking.
  Release builds refuse app-known injection as simulation; debug
  admits it for isolated tests; a static gate asserts production
  sources carry no injection hook.

## What changed

- `application/.../portable/PortableLocationFlow.kt` +
  `PortableLocationPorts.kt` — assess/authorize/disclosure over
  source + environment ports; manual path skips the provider;
  each call reads exactly once.
- `application/.../portable/PortableLocationFlowTest.kt` — 9
  suites (mock block + one-shot, verified allow, source-missing,
  denied, absent/null, manual skip + block, release injection
  refusal, debug admission, stale/coarse codes).
- `data/.../local/location/AndroidLocationSignals.kt` —
  permission → denied-signal; SecurityException → denied;
  missing manager/fix → null/absent; framework `isMock` /
  `isFromMockProvider` version branch; age from elapsed-realtime,
  skew left for the server (T03).
- `data/.../local/location/LocationReading.kt` +
  `LocationReadingTest.kt` — pure reduction (6 suites: denied,
  absent, verbatim, negative-accuracy fold, mock passthrough,
  unknown age).
- `data/.../di/LocationModule.kt` — Hilt singletons (signals,
  environment, flow); `data/build.gradle.kts` gains
  `buildFeatures.buildConfig` (additive, feeds the guard).
- `iosApp/.../LocationSignals.swift` +
  `LocationSignalsTests.swift` — Core Location reduction
  (`isSimulatedBySoftware` nil → unavailable; negative accuracy
  → missing) with NOT COMPILED/NOT RUN banners.
- `scripts/check-mobile.sh` — static release-artifact guard:
  `testInjected = true` in production sources fails the gate.

## Dependency review (recorded need/license/security)

- Added: none (buildConfig feature flag is AGP-builtin, not a
  dependency). Framework `Location`/`LocationManager`/`SystemClock`
  are platform APIs (minSdk 26); androidx `ContextCompat` predates
  this slice. `LocationCompat` deliberately unused: it wraps the
  same two framework mock calls, so it adds no signal.
- Secret surface: `scan-secrets.sh` + PR-diff scan clean; no
  coordinates in logs, tests or fixtures (bands only).

## RED → GREEN

- RED proven by admitting release test-injection as normal input:
  `releaseRefusesTestInjectionAsSimulation` fails (injected fix
  verifies); GREEN on restore.
- Incidental fixes in-task: `data/build.gradle.kts` gains the
  additive `buildConfig` feature (BuildConfig unresolved
  otherwise); one-shot test expectation corrected (exactly one
  read per assess call, not per test).

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
bash scripts/check-mobile.sh
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: Android baseline BUILD SUCCESSFUL (9 new flow + 6 new
boundary suites green); `check-mobile` full ok (static guards
incl. new injection-hook guard + Android green; Swift SKIP
recorded outstanding, never green); `diff --check` clean;
secrets scan clean. The ~25 provider-wiring lines run only on
device: statically reviewed + lint-guarded, behavior suites
cover the rest.

## Limits (not claimed)

- No backend intake distrust (T03), no device acceptance (T04);
  provider wiring unproven until hardware run (release horizon).
- Swift compiles/runs on macOS only (release horizon).

## Rollback

Code rollback = revert flow/ports, Android package, module,
buildConfig flag, Swift files, gate addition and tests; portable
contract returns to T01. No migration, no flag.

## Next

P16-T03 backend risk and proximity checks (#42, same branch).
Issue #41 stays open until the phase PR merges.
