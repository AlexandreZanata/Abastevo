# MAP-14 spike — CoMaps native engine render/search/route

Status: SPIKE_PASS / PHASE_INTEGRATION_PENDING
Validation: PARTIAL (real native behavior; adapter/ABI/16 KB/lifecycle pending)
Integration: NOT_INTEGRATED

Date: 2026-10-24. Upstream: `comaps/comaps`
`4587b8ad4d12c9f59982709fdaf03cc925948827`.

This is an ENGINE_GATE technical spike, not MAP-14 completion and not the
Abastevo map tab. The CoMaps source was built and exercised outside the
Abastevo app; no CoMaps code, AAR, APK, map package or brand asset entered Git
or the Abastevo APK.

## What was built

- `:sdk:assembleDebug -Px86_64 -PdisableHWAsan` → `sdk-debug.aar`
  26 MB (classes + resources + assets + x86_64 native libs).
- `:app:assembleFdroidDebug -Px86_64` → CoMaps F-Droid debug APK
  144 MB, SHA-256
  `29a2fe980320c792b8678417c9d774c1418d6a6199a67e42bb6899f1b3dc1c02`.
- `liborganicmaps.so` x86_64 45 MB, SHA-256
  `b412680d6f43ab74d439b3620e19f352793e77d60bcd2ed84ce061eed34058bc`.
- Toolchain: NDK 28.2.13676358, SDK CMake 3.22.1 + Ninja, Java 21, Gradle
  8.10.2. Build-only tools: Qt 6.5.3 + Qt Positioning (offscreen skin
  generator) and Kothic drules generator; no Qt library is shipped in the
  Abastevo app.
- Missing generated assets were produced from the pinned source: `classificator`,
  `types`, `visibility`, `colors`, `patterns`, `drules_proto` via Kothic and
  `symbols.png`/`symbols.sdf` via skin generator.

## Real behavior observed

Owned disposable emulator `anpfuel-low26`: API 26, x86_64, no Play Store,
SwiftShader GLES3.

- **Render/lifecycle:** CoMaps `MwmActivity` opened; map tiles, labels and map
  controls rendered; FPS counter showed 29 FPS at 13 zoom and continued
  rendering (UIAutomator's idle wait timed out because the map surface was
  continuously drawing). Log: `Framework(): Classificator initialized`,
  `OnContextCreate`, `Metalines prepared: MwmId [Belarus_Minsk Region, 261004]`.
- **Offline search:** with airplane mode/network unreachable, query `Minsk`
  returned local street/category results (`Minskaja vulica`, `Minski zavulak`,
  `Minsk`, etc.); native log showed `Belarus_Minsk Region geocoding complete`
  and `Emitting a new batch of results: 30`. With network up, 75 pre-rank
  results were reported.
- **POI selection:** selecting `Minsk` opened the place page with name, type,
  distance and actions (route/save/open profile equivalents).
- **Offline car route:** destination `53.91,27.57` from current fix
  `53.9024,27.5618` on the local `.mwm`:
  `CalculateRoute(): ... astar-bidirectional-Car`,
  `Route length: 2007.85 meters. ETA: 304.027 seconds.`,
  `onRoutingEvent(resultCode: 0)`. A second attempt to an out-of-map fix
  correctly failed with `Can't find start or end node`, showing honest
  missing-road behavior rather than a straight-line route.
- Airplane-mode route retry initially failed because the emulator had no GPS
  fix after relaunch (`resultCode: 2`); a subsequent `adb emu geo fix` restored
  location. The measured 2,007.85 m route ran on the local map; the router has no
  network input. A clean airplane-mode acceptance still needs one recorded run
  after GPS fix.

## Test inputs outside Git

- Official CoMaps test map series `2026.06.28/261004`:
  `World.mwm` 52 MB SHA-256 `77a35c16…22e4a5`, `WorldCoasts.mwm` 8.1 MB
  SHA-256 `d42a71b1…43626`, `Belarus_Minsk Region.mwm` 105 MB SHA-256
  `23b090c1…8ea3d`.
- These are test inputs downloaded for this spike only; no production CDN
  entitlement or redistribution is claimed. The generated `Belarus_Minsk`
  package is ODbL data with attribution.

## Not yet proven

- No Abastevo adapter/module, no `com.anpfuel` integration, no Compose host,
  no custom station overlay/hit test, no offline search UI inside Abastevo.
- Only x86_64 debug; arm64/armeabi-v7a, release, 16 KB page size, minSdk 26
  and ABI packaging are not measured.
- No memory/APK/disk/battery budget, no lifecycle/rotation/leak test, no
  TalkBack/200% font/narrow-screen test, no route cancellation/recalculation.
- No generated Brazil map package yet; MAP-08 generation and MAP-09/13 data
  publication remain pending.
- No Android Auto, device, production or release claim.

Next for the phase: build the narrow Abastevo adapter over the AAR, pass the
offline airplane-mode route/search acceptance, then run ABI/16 KB/lifecycle
measurements before MAP-15/16.
