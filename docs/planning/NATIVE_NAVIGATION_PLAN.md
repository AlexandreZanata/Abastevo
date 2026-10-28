# Native navigation like CoMaps (route from my position, follow camera, voice)

Status: PLANNED (2026-10-28). Owner track: MC-C7 in `MAP_COMPLETION_PLAN.md`.
Engine: pinned CoMaps 4587b8ad SDK (`maps/engine/vendor/sdk-release.aar`).
Delivery: one planning stage = one PR (`codex/phase-native-navigation`).

## 1. Goal

Inside the Abastevo map, a driver gets the same routing experience as the
CoMaps app: tap a station (or any point) → route from the **current
location** → preview → Start → animated follow/perspective camera with the
my-position arrow, turn panel, voice, automatic rerouting and arrival, in the
foreground and with the screen off, **offline** once the region is installed.

"Online" and "offline" use the same engine: CoMaps routes only from the local
`.mwm` routing sections, with no server routing or live traffic. Online only
adds downloading a missing region, which goes through the Abastevo signed
distribution, not CoMaps servers.

## 2. Baseline (what exists, verified 2026-10-28)

Already in place:

| Area | State |
|---|---|
| Route build | `ComapsRoutePlanner` (`maps/engine/.../ComapsRoutePlanner.kt`): car/avoid options, preview distance/time/geometry/steps. It owns the native routing listener, so the SDK `RoutingController` is bypassed. |
| Follow route | `ComapsMapEngine.startGuidance` calls `nativeFollowRoute`. The native camera (`DrapeEngine::FollowRoute`, navigation scale, 3D, rotation) runs once fixes arrive and a surface is rendered. Not yet verified visually. |
| Fix feed | `GuidanceService` (location foreground service, GPS at 1 Hz) → `NavigationFixPolicy` → `NavigationFixBridge` → `LocationState.nativeLocationUpdated`. |
| Reroute | Native reroute plus a 60 s "not recoverable" timeout (`GuidancePolicy`). |
| Voice | `LocalGuidanceVoice` (Android TTS, local voices only, audio focus). |
| Recovery | `GuidanceRecoveryStore` with explicit resume, and a map-generation lease. |
| Tests | Domain/application/engine JVM tests, plus device tests `GuidanceEngineInstrumentedTest`, `GuidanceInstrumentedTest`, `NativeRoutePlannerInstrumentedTest` and `NativeRouteCrossRegionInstrumentedTest`. |

Gaps against CoMaps:

1. **Start point.** Every point is added with `isMyPosition=false`. The start is a one-shot "Current" coordinate, and the station "Route" action fills only the destination.
2. **My position.** The SDK `LocationHelper` is never started. There is no `LocationState` mode machine, no my-position arrow outside guidance and no compass. `recenter()` reads `locationHelper.myPosition`, which is null, so it likely always reports "no-fix".
3. **Guidance UI.** Guidance controls live inside the dismissible route sheet (`MapScreen.kt:119`). There is no on-map turn panel or bottom navigation bar.
4. **Dropped turn data.** `toNavigationProgress` drops lanes, exit number, then-turn, speed limit, current street, road shields and `distToNextStop`.
5. **Voice.**
   - Only `notifications.firstOrNull()` is spoken.
   - `LocalGuidanceVoice.start()` waits up to 8 s on the main thread for TTS `onInit`, which is also delivered on main. This is a suspected stall followed by "voice unavailable". Confirm it first.
6. **No arrival handling.** `nativeIsRouteFinished` is never called.
7. **Notification.** It is static: no turn, distance or street.
8. **Permissions.** The guidance UI never requests FINE location or (API 33+) POST_NOTIFICATIONS, so `GuidanceService` refuses to start silently.
9. **No camera tuning.** There is no 3D/auto-zoom setting and no `updateMyPositionRoutingOffset` for the bottom panel.
10. **Contract conflict.** `docs/product/MAP_21_23_CONTRACT.md` says "browsing never starts continuous GPS". CoMaps tracks location whenever the map is visible and the user enabled it.

All SDK classes and natives needed are present in the shipped jar and in both `liborganicmaps.so` ABIs:

- **Classes:** `RoutingController`, `LocationHelper`, `LocationState`, `TtsPlayer`, `CarDirection`, `RoutingInfo`, `LaneInfo`, `RoadShieldUtils`, `RouteSimulationProvider`, `MapController.updateMyPositionRoutingOffset`, `SensorHelper`.
- **Framework natives:** `nativeFollowRoute`, `nativeIsRouteFinished`, `nativeSetAutoReroute`, `nativeSet3dMode`, `nativeSetAutoZoomEnabled`, `nativeGetIntermediateStopsProgress`.

The UI layer is app-only in CoMaps and must be ported:

- **Navigation views:** `NavigationController`, `NavMenu`, `LanesView`, `SpeedLimitView`.
- **Location and service:** `MyPositionButton`, `NavigationService`.
- **Routing plan fragments.**

## 3. Phases

Each task follows B-BR/BUC → RED → GREEN → REFACTOR, runs its targeted
tests, and is verified on the POCO with the release build (test key) before
the phase closes.

### NAV-0 Decisions and contract (blocking, no code)

- **NAV-0.1 Product decision.** Continuous location while the Map tab is visible, **only after** the user taps my-position or starts a route. The default stays "no GPS while browsing". Stop on background unless navigating. Update `MAP_21_23_CONTRACT.md` and the `scripts/check-mobile.sh` location allowlist in the same task.
- **NAV-0.2 Licence record.** Porting CoMaps `android/app` code (Apache-2.0) to Kotlin is allowed if we:
  - keep the Apache header and NOTICE on ported files;
  - mark the files as modified (§4(b));
  - use no CoMaps/Organic Maps branding;
  - copy only drawables whose provenance in `data/copyright.html` allows it. The turn icons already ship in the AAR.

  Record this in `maps/engine/vendor/NOTICE` and in an evidence note.
- **NAV-0.3 Engine ownership.** Keep the Abastevo planner as the single owner of the native routing listener. It is stricter on NEED_MORE_MAPS than the SDK `RoutingController`. Do not mix in `RoutingController`; document why.

### NAV-1 My position and recenter (CoMaps parity)

- **NAV-1.1 Single location feed.**
  - Start the SDK `LocationHelper` (`start`/`restartWithNewMode`/`addListener`, 500 ms) while the map is foreground and location is enabled. Stop it on `ON_STOP` unless navigating.
  - `GuidanceService` reuses the same feed instead of its own `GPS_PROVIDER` subscription. One feed means one arrow.
  - Keep `NavigationFixPolicy` as a filter, but relax the 50 m accuracy cap to CoMaps behaviour: accept GPS-provider fixes and let the core weight accuracy. Add an urban-canyon test.
- **NAV-1.2 Mode machine.**
  - Register `LocationState.nativeSetListener`. Expose `MyPositionMode` (PENDING, NOT_FOLLOW_NO_POSITION, NOT_FOLLOW, FOLLOW, FOLLOW_AND_ROTATE) in `MapUiState`.
  - Report denial and GPS off through `nativeOnLocationError`.
  - Never call `getMode()` before the drape engine exists.
- **NAV-1.3 My-position button.** Port `MyPositionButton` to Compose, with a per-mode icon and a `nativeSwitchToNextMode` click. It replaces today's one-shot recenter. A permission request on first tap; the pending spinner state.
- **NAV-1.4 Compass.** `SensorHelper` → `Map.onCompassUpdated` for FOLLOW_AND_ROTATE.
- **Tests:**
  - JVM: mode reducer and permission states.
  - Device: the arrow appears, a pan leaves FOLLOW and a tap returns to it. Use simulated locations (`RouteSimulationProvider`) and no real-user GPS in fixtures.

### NAV-2 Route from my position

- **NAV-2.1** Add a "Minha localização" start point with `isMyPosition=true` (`nativeAddRoutePoint` type START). This is the default origin.
  - The core then routes and reroutes from the live MyPositionMark.
  - Map `NO_POSITION` (2) to a pending state that waits for the first fix (`nativeStartPendingPositionMode`), then builds.
- **NAV-2.2** The station sheet "Rota" sets origin = my position and destination = station, then opens the preview directly, as CoMaps `startLocationToPoint` does.
- **NAV-2.3** Missing region (`NEED_MORE_MAPS`) → the Abastevo regional download card or screen for the named regions, then rebuild.
- **Tests:** planner JVM tests for the start type and the NO_POSITION path; device test station → preview offline in Mato Grosso.

### NAV-3 Route preview on the map

- **NAV-3.1** Replace the modal sheet with an on-map preview. A bottom card shows time, distance, ETA (local clock), avoid chips and Start. A steps sheet shows `CarDirection.getTurnRes()` icons and street names. The route line is drawn by the core.
- **NAV-3.2** Before Start, request FINE location and POST_NOTIFICATIONS (API 33+) in context. On refusal, explain once and keep the preview usable. Mirror CoMaps `requestPostNotificationsPermission`.
- **NAV-3.3** Visible rect: `nativeSetVisibleRect` so the route fits above the card.

### NAV-4 Navigation mode (the "Google/CoMaps feel")

- **NAV-4.1 Data.** Extend `NavigationProgress` with:
  - `nextCarDirection`, `exitNum`, `lanes`, `currentStreet`, `nextStreet` (with road shields via `RoadShieldUtils`);
  - `speedLimitMps`, `distToNextStop`, `isLeftHandTraffic`, `shouldPlayWarningSignal`;
  - `nativeGetIntermediateStopsProgress`.

  Pure domain value objects; mapping tests.
- **NAV-4.2 Turn panel** (top, Compose port of `NavigationController` and `LanesView`): turn icon, distance to the turn (`nativeFormatSpeed`/distance formatting), then-turn, roundabout exit, lanes, street with shields.
- **NAV-4.3 Bottom bar** (port of `NavMenu` and `RouteProgressBar`): ETA clock, time left, distance left, progress, mute, stop. Current speed vs speed limit (`SpeedLimitView`).
- **NAV-4.4 Camera.**
  - `nativeSet3dMode(true, buildings)` and `nativeSetAutoZoomEnabled(true)` as the defaults, both editable in settings.
  - `updateMyPositionRoutingOffset(bottomBarHeight)` through a new same-package accessor in `MapViewHelper`.
  - Feed fixes at 2 Hz.
  - The native follow camera returns to FOLLOW 20 s after a pan (`kMaxNotFollowRoutingTimeSec`). Show a "Recentralizar" chip meanwhile.
- **NAV-4.5 Surface rules.** Navigation keeps a live `MapView` while foreground. `nativeFollowRoute` without a rendered surface crashed (MAP-23 defect 1). Background continues through the service with the surface released safely; add a regression test.
- **NAV-4.6 Screen.** `FLAG_KEEP_SCREEN_ON` while navigating. Prompt once about battery optimisation, as CoMaps does.

### NAV-5 Voice

- **NAV-5.1** Initialise TTS asynchronously, never blocking main. Fix the suspected 8 s stall with a device test that starts the service from the main thread.
- **NAV-5.2** Speak every string from `nativeGenerateNotifications(announceStreets)`, in order, as `TtsPlayer.playTurnNotifications` does. Turn texts come from the native `sound-strings` (pt-BR ships in the AAR).
- **NAV-5.3** Voice settings: on/off, language (supported list from `TtsPlayer` natives), street names, and a test voice. The mute toggle sits in the bottom bar.

### NAV-6 Background, rerouting and arrival

- **NAV-6.1** Live notification: turn bitmap, distance and street; `CATEGORY_NAVIGATION`, low importance; a tap opens the map in navigation mode.
- **NAV-6.2** `nativeSetAutoReroute(true)` at init.
  - Show "Recalculando…" on OffRoute/RouteRebuilding.
  - Native rebuild errors are silent (no NEED_MORE_MAPS callback), so keep the 60 s recoverable timeout and offer the download when the position leaves the installed regions.
- **NAV-6.3 Arrival.** After each fix, if `nativeIsRouteFinished()`:
  - speak the last notification;
  - stop guidance and the service;
  - show an arrival card with the station's profile and price contribution.
- **NAV-6.4 Speed cameras.** `nativeSetSpeedCamManagerMode`, plus a beep through `MediaPlayerWrapper` and our own raw asset (none ships in the AAR).

### NAV-7 Offline and cross-region

- **NAV-7.1** Airplane-mode journey (MC-C8 leg): MT installed, airplane on, station → route → navigation → arrival.
- **NAV-7.2** Cross-region route with the generated packages, for example Sorriso (MT) → Goiânia (GO). The routers match border twins by OSM way and segment, so the generated `cross_mwm` sections must agree.
  - If routes fail or detour, regenerate the neighbouring regions from the same PBF date with cross-MWM transitions intact (and `--world-roads` if long leaps need it).
  - Add a per-region minimum-transitions check to `generate-regional-map.sh`.
- **NAV-7.3** Missing-map detection relies on SDK `countries.txt`/`packed_polygons.bin` ids matching the generated file names (`Brazil_Mato Grosso`). Lock this in with a test over all 23 regions.

### NAV-8 Acceptance

- **Instrumented journeys** with `LocationHelper.startNavigationSimulation(nativeGetRouteJunctionPoints)`: preview → start → turn panel updates → off-route → reroute → arrival. Offline and online, light and dark.
- **Field drive** in Sorriso with screen recording: follow camera, rotation, voice, a wrong turn and its reroute, arrival, screen off and on. Record timings in `docs/backend/evidence/maps/NAV.md`.
- **Gates:**
  - Android: `:domain:test :data:testDebugUnitTest :application:test :maps:engine:testDebugUnitTest :app:testDebugUnitTest :app:lintDebug :app:assembleRelease` (APK ≤ 80 MiB).
  - Mobile guard: `scripts/check-mobile.sh` with the updated allowlist.

## 4. Order, size and risks

Order: NAV-0 → NAV-1 → NAV-2 → NAV-3 → NAV-4 → NAV-5 → NAV-6 → NAV-7 → NAV-8.
NAV-5.1 can start in parallel at any time, because it is a likely defect today.

Rough size, one engineer with the device:

| Phases | Days |
|---|---|
| NAV-0 | 0.5 |
| NAV-1 | 2 |
| NAV-2 | 1 |
| NAV-3 | 2 |
| NAV-4 | 4 |
| NAV-5 | 1 |
| NAV-6 | 2 |
| NAV-7 | 1–3 (depends on regeneration) |
| NAV-8 | 1.5 |
| **Total** | **≈ 15–17 working days** |

Risks:

- **Product contract.** The NAV-0.1 location decision is blocking.
- **Native crashes** without a surface. Covered by NAV-4.5.
- **Cross-region routing** on generated packages. Covered by NAV-7.2.
- **TTS stall.** Covered by NAV-5.1.
- **Guard updates.** The `check-mobile.sh` location allowlist must change in the same commits as the location code.
