# MAP-16 — Compose lifecycle and privacy-controlled map host

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (JVM view model + real-phone Compose host/lifecycle)
Integration: INTEGRATION_PENDING

Tested tree: `codex/map-ev-implementation` at `9abc1cf1` plus the MAP-16
changes. Date: 2026-10-24. No map tab or marker UI was added.

## Behavior frozen

- `MapHost` wraps the native `MapEngine` surface in Compose `AndroidView`; the
  native SDK type never leaks into the app/domain/data layers.
- The host creates the map without starting location. A location permission is
  requested only when the user taps recenter. Denied, approximate and
  denied-forever states never block browsing, search or manual viewport use.
- No Play Services dependency and no background/continuous GPS for simple
  browsing.
- `MapViewModel` owns readiness, permission flags, explicit recenter and camera
  persistence. It cancels stale search/route work on dispose and stores only the
  center/zoom in a platform camera store (no private route/favorite data).
- `MapHost` observes the host lifecycle and saves the camera on `ON_STOP`.
  Activity recreation is exercised; the map surface is recreated without a
  second initialization/`loadBookmarks()` call.
- Recenter is a 48 dp target with an explicit content description.

## Implementation

- `maps/engine/.../MapHost.kt`: Compose host, permission launcher, recenter.
- `maps/engine/.../MapViewModel.kt`: state machine and cancellation.
- `maps/engine/.../MapCameraStore.kt`: shared-preferences center/zoom store.
- `MapEngine`/`ComapsMapEngine`: `currentCenter()` and explicit `recenter()`.
- `maps/engine/build.gradle.kts`: Compose test/host dependencies; optional
  module remains gated by `-PcomapsSdkDir`.

## Verification executed

JVM (`:maps:engine:testDebugUnitTest`, 5 cases):

- saved camera restored and applied once;
- no permission never triggers recenter;
- approximate permission permits an explicit recenter;
- host stop persists the current center without changing zoom;
- dispose cancels search/route work.

Real phone instrumentation through adb (2311DRK48G, Android 16 / API 36):

```sh
./gradlew :maps:engine:connectedDebugAndroidTest \
  -PcomapsSdkDir=/tmp/comaps-aar-debug-fresh
# Starting 2 tests on 2311DRK48G - 16
# Finished 2 tests on 2311DRK48G - 16
# BUILD SUCCESSFUL
```

The tests prove the Compose host creates the map surface without calling
recenter/location on entry, and survives `ActivityScenario.recreate()`.
The optional app build with the engine and the normal app build both pass.

## Limits

- The Compose host tests use a fake `MapEngine`; the real native surface was
  exercised earlier on the x86_64 debug emulator, not on the arm64 phone (the
  phone still shows the blank `SurfaceView`/BLAST issue from T08).
- No runtime permission grant/denial dialog test; those states are covered by
  view-model tests and the launcher code is compiled.
- No map tab, quick sheet, marker cluster or source selector (MAP-17/18).
- ENGINE_GATE remains partial, so this is a local host checkpoint, not a
  release/integration acceptance.
