# MAP-14 — Extracted engine adapter and render proof

Status: IN_PROGRESS / ENGINE_GATE_PARTIAL / T08_ARM64_RUNTIME_PASS / T08_16KB_RUNTIME_DEFERRED
Validation: PARTIAL (arm64 phone render/pan/zoom/offline search/route/select PASS; 16 KB runtime deferred by owner; clustering/accessibility/audit/ABI budgets pending)
Integration: NOT_INTEGRATED

Tested tree: `codex/map-ev-implementation` at `721582b8` plus the uncommitted
MAP-09–14 changes. Date: 2026-10-24.
Upstream: `comaps/comaps` `4587b8ad4d12c9f59982709fdaf03cc925948827`.

This record does **not** claim ENGINE_GATE PASS. It records the extracted
artifact, the real adapter module and the render/pan proof, and the exact
remaining blockers.

## T01/T03 — pinned artifact

- Upstream source is pinned at `4587b8ad…`; the native core was built for
  `arm64-v8a` with NDK `28.2.13676358`.
- Build recipe patch outside Git: `android/sdk/build.gradle.kts` adds
  `-DANDROID_SUPPORT_FLEXIBLE_PAGE_SIZES=ON`, `-DANDROID_LD=lld` and
  `-DCMAKE_SHARED_LINKER_FLAGS=-fuse-ld=lld -Wl,-z,max-page-size=16384`;
  `android/sdk/src/main/cpp/CMakeLists.txt` adds `-fuse-ld=lld` after the
  upstream target options.
- Upstream/AGP still injects a trailing `-fuse-ld=gold`; NDK r28 has no
  `LLVMgold.so`, so the Gradle release link fails. The accepted local `.so`
  was linked by running ninja with that trailing gold flag removed from the
  generated `build.ninja`, then stripped with NDK `llvm-strip --strip-unneeded`.
  This is a build-recipe blocker, not a clean reproducible release build.
- Result: `liborganicmaps-arm64-release.so`, 14,703,416 bytes, SHA-256
  `a12c23508fa227aac4bf0cea45f14a8f6bcab43bf7b5a27d5b8b9d203fba336c`.
  `llvm-readelf -l` shows all `LOAD` segments aligned `0x4000` (16 KB).
- Debug x86_64 AAR (`sdk-debug.aar`) 41,906,202 bytes, SHA-256
  `4194ccd6…82a7`; it contains classes, 289 resource files, assets and
  `liborganicmaps.so` for the emulator.
- A locally packaged arm64 AAR (`comaps-sdk-arm64-release.aar`, 35,234,857
  bytes, SHA-256 `80a40311…787a`) was assembled from the debug AAR with the
  stripped arm64 `.so`; it is a measurement/probe artifact, not a clean
  release AAR because its Java classes/assets are still debug-built.

## T02/T04 — Abastevo-owned adapter

Added the optional module `maps/engine` (included only when
`-PcomapsSdkDir=DIR` points at an unpacked AAR):

- `MapEngine` is the Abastevo-owned port: initialize, create map view, set
  viewport, initialization state. No `app.organicmaps.*` type is exposed.
- `ComapsMapEngine` owns `OrganicMaps`, `ConnectionState`, the Android-native
  location provider, the `MapView` lifecycle, offline search, car routing and a
  bookmarked/KML Abastevo overlay.
- `MapViewHelper` is the only package-private bridge needed to attach the
  location helper to `MapView`.
- Search uses `SearchEngine` with a timestamp-scoped listener. Routing uses
  `RoutingController` with two in-map `MapObject` checkpoints and a bounded
  container. The overlay is a temporary KML whose placemark titles are
  `ABASTEVO:<canonical-uuid>`.
- `app/src/comapsProbe` and the debug-only `MapProbeActivity` are compiled only
  with the same property; the normal app build does not include the engine.

## T05 — real render/pan proof

Built and installed the real Abastevo debug APK with the engine
`-PcomapsSdkDir=/tmp/comaps-aar`, pushed the MAP-08 Curitiba package plus
World/WorldCoasts to the app's `files/261004`, and launched the probe on the
owned API 26 x86_64 emulator:

```text
Metalines prepared: MwmId [Brazil_Parana_East, 261004]
Framework initialized
GL3: Zoom 13 | 21–27 FPS
Map data © OpenStreetMap
```

Screenshot inspection confirmed Curitiba streets, labels, fuel/charging
reference POIs and attribution. A vertical swipe moved the map, proving
pan/touch. The same probe showed the world map at zoom 1 at ~40 FPS.

Additional adapter evidence on the same installed APK:

```text
MapProbe: search=29 first=Museu do Holocausto
OCMcore: Route length: 1575.15 meters. ETA: 288.154 seconds.
RoutingController: onRoutingEvent(resultCode: 0)
MapProbe: route=1.6Kilometers eta=288
ComapsMapEngine: overlay marker category=1013 uuid=aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa
```

The overlay pin rendered on the map with the title containing the canonical
UUID. A landscape rotation and a process-kill/relaunch produced no new fatal
error; the singleton adapter avoids re-running `loadBookmarks()` in the same
process. A later tap on the canonical bookmark pin produced:

```text
ComapsMapEngine: place page activated title=ABASTEVO:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa
MapProbe: selected=aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa
```

The adapter now writes one KML generation for a marker list, deletes the prior
Abastevo category and reuses the same canonical UUID in the placemark title.
A coincident OSM POI can win the native hit-test instead of the bookmark; the
final app-owned place-page/list surface still has to disambiguate that case.

## Remaining ENGINE_GATE work

- **T03:** linker/release-AAR blocker RESOLVED by [build recovery](MAP-14_BUILD_RECOVERY.md). Real Gradle arm64/API-26 release classes and native code pass app assembly and the 80 MiB gate. Clean-checkout reproducibility, other supported ABIs and minimal admitted assets remain open.
- **T04:** adapter now serializes concurrent initialization, tracks the active
  search listener and routing container, cancels stale search/route work and
  removes the place-page listener on release. Double init, rotation and
  process-kill/relaunch were exercised manually; permission-denied browsing
  works without requesting location. Instrumented leak/cancellation tests and
  full process-death automation remain open.
- **T05:** proven in the Abastevo adapter: 29 offline search results and a
  1,575.15 m / 288 s car route between in-map checkpoints. Guidance UI and
  route cancellation remain open.
- **T06:** UUID render and hit-test callback are proven; batch generation
  replaces the prior category. Custom Abastevo pictograms, co-located symbol
  disambiguation, clustering and the accessible list remain open.
- **T07:** transitive/submodule license audit, SBOM/notices, patch reapplication,
  no-secret/endpoint scan and ODbL derived-data decision.
- **T08:** full evidence, budgets and ENGINE_GATE closure.

## Limits

- The app was exercised in **debug**; no Play/device (arm64 phone) or
  production claim.
- Measured APKs: debug with engine ~103 MB, normal debug ~26 MB, release with
  the arm64 AAR **66.59 MB (69,821,202 bytes)**. The user explicitly authorized
  accepting this size; the release-size gate was raised from 15 MB to 80 MB and
  `:app:verifyReleaseApkSize` now passes for the engine build.
- The release APK was built from the locally assembled AAR and is not a clean
  release-classes artifact; arm64 phone render/lifecycle remains unproven.
- No product map download/activation, Compose map tab or overlay selection is delivered. Search/route exist in the debug probe only.


## Subsequent build recovery — 2026-10-24

[MAP-14_BUILD_RECOVERY](MAP-14_BUILD_RECOVERY.md) supersedes the earlier manual
link/hybrid-AAR limitation: actual arm64 release AAR, minSdk 26, release
BuildConfig, stripped core and packaged alignment passed. App release is
66.64 MiB under the already accepted 80 MiB budget. The full phase remains
ENGINE_GATE_PARTIAL. [Bounded continuation](../../../planning/MAP_EV_RESUME.md)
records remaining engineering tasks and the existing native Ed25519 direction.

## T07/T08 continuation — 2026-10-24

- T07 draft inventory: [MAP-14_T07_AUDIT](MAP-14_T07_AUDIT.md). 193 pinned
  submodules are clean; 191 have own/inherited license/notice evidence; `glm`
  and `just_gtfs` need legal resolution. No secret-pattern hits; default upstream
  endpoints remain in the native binary. This is a draft, not acceptance.
- T08 packaging reproduced from the Codex release candidate: release APK
  69,881,875 bytes (66.64 MiB), SHA-256
  `79a3117eed91aa5dffb455f0ccafacb2cc6e2223ab37d8af8511ffe50e98e678`;
  `zipalign -c -P 16 4` PASS; packaged `liborganicmaps.so` arm64 14,675,096
  bytes with three 16 KB LOAD segments. Only the core is arm64; no engine
  libraries are packaged for the other ABIs.
- Physical arm64 phone (Android 16, 4 KB page size): the release AAR arm64
  library loaded and the engine created a SurfaceView, but the map surface
  stayed blank with repeated `BLASTBufferQueue ... Can't acquire next buffer`
  errors. The probe was uninstalled and its files removed. The blank result was
  later reproduced as a test-seeding fault (unreadable adb-created external
  map directory), not a renderer failure; see the arm64 runtime section below.
- Remaining T08: actual 16 KB-page runtime, T06 overlay/accessibility work,
  measured budgets and full ENGINE_GATE closure.

Fresh static re-verification of the release APK:

```text
app-release-unsigned.apk  69,898,847 bytes (66.66 MiB)
SHA-256 d8094048837bedbc52c517eb53daea4185f56b7bedc626852388154313f4bd92
zipalign -c -P 16 4  PASS
lib/arm64-v8a/liborganicmaps.so  14,675,096 bytes
SHA-256 08342926e3799d29851baf093e535c3ca107dd19e218eb69b3e1447e7d155ccb
LOAD segments: 0x4000 / 0x4000 / 0x4000
```

This rebuild includes the overlay KML fix; the earlier 69,881,875-byte artifact
`79a3117e…` is superseded for the current tree.

## T08 arm64 runtime on the physical phone — 2026-10-24

Status: **T08 arm64 runtime PASS** on the authorized physical phone;
**16 KB runtime DEFERRED by owner decision**. ENGINE_GATE remains **PARTIAL**,
not PASS.

Owner decision: use the physical phone (4 KB pages) and support the whole
device population rather than requiring a 16 KB-only runtime. The 16 KB runtime
proof is therefore not claimed; the static `0x4000` alignment remains recorded
above. No waiver is recorded for the other MAP-14 tasks.

Device: model `2311DRK48G`, Android 16/API 36, `arm64-v8a`, PAGE_SIZE=4096.
Engine: release AAR `1fe196f9…` / core `08342926…`, consumed by a new
debug-only `mapProbe` app variant (package `com.anpfuel.app.mapprobe`,
91,053,490 bytes, SHA-256 `008318f5…`). The probe was uninstalled and its
app/external data removed after the run.

Map data: `Brazil_Parana_East.mwm`, `World.mwm` and `WorldCoasts.mwm` copied
into the probe's **internal** `files/` directory (`files/261004/...`). An
earlier attempt to seed the probe's external `Android/data` directory through
`adb` produced `Permission denied`/`listFiles() null`; the engine selected that
unreadable storage and showed a beige map. This was a test-seeding fault, not a
renderer failure. The product download repository stores packages under
`context.filesDir/maps` (internal), so its storage is readable; mapping the
MAP-15 flat package layout into the engine's country-directory layout remains a
MAP-17 integration task.

Observed on the physical phone (airplane mode on for the offline proof):

```text
StoragePathManager: Found map files at /data/data/com.anpfuel.app.mapprobe/files/
SetWritableDir(): Writable path = /data/data/com.anpfuel.app.mapprobe/files/
Framework initialized
RegisterMap(): Loaded Brazil_Parana_East map, of version 261004
ComapsMapEngine: overlay generation category=1022 markers=2
MapProbe: search=38 first=Museu do Holocausto
Route length: 1575.15 meters. ETA: 288.154 seconds.
MapProbe: route=1.6Kilometers eta=288
ComapsMapEngine: place page activated title=ABASTEVO:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa
MapProbe: selected=aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa
```

The SurfaceFlinger producer queued the map SurfaceView at ~117–120 FPS during
the interactive checks. Screenshot evidence is in [map14-phone/](map14-phone/):
[render-curitiba.jpg](map14-phone/render-curitiba.jpg),
[overlay-pan.jpg](map14-phone/overlay-pan.jpg),
[zoom.jpg](map14-phone/zoom.jpg),
[landscape.jpg](map14-phone/landscape.jpg) and
[offline-airplane.jpg](map14-phone/offline-airplane.jpg).

- Pan: a 500 ms swipe changed 61.97% of sampled pixels; both fuel/charging UUID
  labels stayed visible.
- Zoom: a double-tap changed 74.32% of sampled pixels; the scale bar moved from
  500 m to 100 m.
- Rotation: the Activity and SurfaceView were recreated; the map, route and both
  overlay labels rendered in landscape without a fatal error.
- Offline: with airplane mode on, render, 38-result search and the 1.6 km /
  288 s route still worked from the local package.

The overlay KML generator was corrected to emit the XML declaration as the
first bytes and to escape titles (`buildOverlayKml`); the previous indented
declaration caused `Could not parse KML` and empty overlay IDs on this device.
A JVM regression test covers the declaration, canonical UUIDs, coordinates and
escaping. `getBookmarkIdByPosition` is asynchronous and is no longer used for
the generation log; selection is proven by the native place-page title.

Commands actually run:

```sh
JAVA_HOME=/data/dev/java/candidates/java/current \
  ./gradlew :maps:engine:testDebugUnitTest \
    -PcomapsSdkDir=/tmp/abastevo-comaps-release-arm64 \
    --offline --no-daemon --max-workers=2 --console=plain

JAVA_HOME=/data/dev/java/candidates/java/current \
  ./gradlew :app:assembleMapProbe \
    -PcomapsSdkDir=/tmp/abastevo-comaps-release-arm64 \
    --offline --no-daemon --max-workers=2 --console=plain

JAVA_HOME=/data/dev/java/candidates/java/current \
  ./gradlew :app:assembleDebug :app:testDebugUnitTest \
    --offline --no-daemon --max-workers=2 --console=plain

JAVA_HOME=/data/dev/java/candidates/java/current \
  ./gradlew :app:assembleRelease :app:verifyReleaseApkSize \
    -PcomapsSdkDir=/tmp/abastevo-comaps-release-arm64 \
    --offline --no-daemon --max-workers=2 --console=plain

adb install -r app/build/outputs/apk/mapProbe/app-mapProbe.apk
# seed internal files/ (not Android/data), then:
adb shell am start -n com.anpfuel.app.mapprobe/com.anpfuel.app.map.MapProbeActivity
adb shell input swipe 610 1356 700 1280 500
adb shell input tap 610 1356
adb shell cmd connectivity airplane-mode enable
adb uninstall com.anpfuel.app.mapprobe
adb shell rm -rf /sdcard/Android/data/com.anpfuel.app.mapprobe
```

Residual limits: the `BLASTBufferQueue ... Can't acquire next buffer` warnings
still appear in logcat, although frames render and the screenshots are
non-blank; this is recorded as a residual vendor/SurfaceFlinger warning, not a
claim that its root cause is fixed. T06 clustering/co-located symbol
disambiguation/accessible list, T07 transitive licenses/SBOM/notices, measured
budgets, armeabi-v7a packaging and actual 16 KB runtime remain open. The release
currently packages only the arm64 engine; supporting the whole device
population requires the 32-bit release ABI and a size-budget check.
