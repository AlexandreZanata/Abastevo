# MAP-16 — Compose lifecycle and privacy-controlled map host

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (JVM view model + real-phone Compose host + real native engine lifecycle)
Integration: INTEGRATION_PENDING

Tested tree: `codex/map-ev-implementation` at `6106b4d9` plus the MAP-16
closure changes. Date: 2026-10-25. No map tab or marker UI was added.

## Behavior frozen

- One `MapEngine` per process via Hilt singleton (`MapEngineModule` +
  `MapEngineHolder`); Compose hosts share it and never create a second native
  core. The native SDK type never leaks into app/domain/data layers.
- `MapHost` wraps the native surface in Compose `AndroidView` only after
  readiness; `newMapView` before readiness throws, and a bare `Context`
  without an AndroidX lifecycle owner is rejected by `MapViewHelper`.
- The host creates the map without starting location. A location permission is
  requested only on explicit recenter tap. Denied, approximate,
  denied-forever, GPS-off and no-fix states never block browsing, search or
  manual viewport use; they annotate recenter (`no-permission`, `gps-off`,
  `no-fix`) with visible text.
- No Play Services dependency and no background/continuous GPS for simple
  browsing. `release()` stops location polling; disposal is idempotent and
  late readiness/permission callbacks cannot revive a discarded host.
- `MapViewModel` (Hilt) owns readiness, permission flags, GPS state, explicit
  recenter and camera persistence. It cancels stale search/route work on
  dispose and stores only center/zoom in `SharedPreferencesMapCameraStore`
  (no private route/favorite data).
- `MapHost` observes `ON_RESUME` (permission + GPS refresh) and `ON_STOP`
  (camera save). Camera restores after recreation; the surface is recreated
  without a second native initialization.
- Recenter is a 48 dp target with an explicit content description.

## Implementation

- `maps/engine/.../MapEngineModule.kt`: Hilt singleton + process holder.
- `maps/engine/.../MapHost.kt`: product Hilt host + testable explicit-engine
  overload, permission/GPS UI states, readiness-gated `AndroidView`.
- `maps/engine/.../MapViewModel.kt`: `@HiltViewModel` state machine with
  `gpsEnabled`/`recenterError`, disposal and camera persistence.
- `maps/engine/.../MapCameraStore.kt`: shared-preferences center/zoom store.
- `MapEngine`/`ComapsMapEngine`: `currentCenter()` and explicit `recenter()`.
- `maps/engine/build.gradle.kts`: Hilt + navigation-compose; optional module
  remains gated by `-PcomapsSdkDir`.

## Verification executed

JVM (`:maps:engine:testDebugUnitTest`, 25 cases: 3 + 7 + 9 + 6):

- saved camera restored and applied once;
- no permission never triggers recenter;
- approximate permission permits an explicit recenter;
- host stop persists the current center without changing zoom;
- dispose cancels search/route work and rejects late callbacks;
- gps-off blocks recenter but keeps browsing;
- no-fix surfaces explicit error without moving camera;
- denied-forever never triggers recenter;
- camera restores after recreation;
- singleton holder returns one engine per process.

Real phone instrumentation through adb (2311DRK48G, Android 16 / API 36):

```sh
./gradlew :maps:engine:connectedDebugAndroidTest \
  -PcomapsSdkDir=/data/dev/artifacts/abastevo-map-engine/d2ec37881ede4e22 \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.maps.engine.MapHostInstrumentedTest
# Starting 2 tests on 2311DRK48G - 16
# Finished 2 tests on 2311DRK48G - 16
# BUILD SUCCESSFUL

./gradlew :maps:engine:connectedDebugAndroidTest \
  -PcomapsSdkDir=/data/dev/artifacts/abastevo-map-engine/d2ec37881ede4e22 \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.maps.engine.MapHostRealEngineInstrumentedTest
# Starting 1 tests on 2311DRK48G - 16
# Finished 1 tests on 2311DRK48G - 16
# BUILD SUCCESSFUL
```

- Fake-host tests prove the surface waits for readiness and creates the map
  without calling recenter/location on entry.
- Real-engine test proves on-device native init, readiness-gated surface,
  lifecycle-owner rejection, recenter-without-fix fail-closed, viewport/center
  round-trip and idempotent release/re-init. Rendering with map data stays in
  MAP-14; this proves the host contract with the real core.

## Limits

- No map tab, quick sheet, marker cluster or source selector (MAP-17/18).
- No runtime permission grant/denial dialog test; those states are covered by
  view-model tests and the launcher code is compiled. GPS-off is read from
  `LocationManager` on resume; a device-level toggle test remains manual.
- ENGINE_GATE remains the MAP-14 receipt; this is a local host checkpoint, not
  a release/integration acceptance.

## Engine checkpoint integration correction — 2026-10-24

[MAP-14_ENGINE_GATE](MAP-14_ENGINE_GATE.md) adds a failing-then-passing
instrumented asynchronous-init case: MapHost creates no native surface before
readiness. Disposal is idempotent, releases the adapter and rejects late
readiness/permission callbacks. MapViewHelper forwards the complete SDK
lifecycle and pauses before buffer teardown. Actual native probe recreation is
recorded separately from the controlled Compose tests. Native package-layout,
first-fix/recenter, error states and the real navigation screen remain the
retained MAP-15/MAP-17 integration work; this record does not accept fake GPS or
a production map tab.
