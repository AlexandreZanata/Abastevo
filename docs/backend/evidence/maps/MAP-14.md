# MAP-14 — Extracted engine adapter and render proof

Status: IN_PROGRESS / ENGINE_GATE_PARTIAL / T08_ARM64_RUNTIME_PASS / MULTI_ABI_ARTIFACT_PASS / T08_16KB_RUNTIME_DEFERRED / T06_PARTIAL / T07_TECHNICAL_AUDIT_DONE_LEGAL_PENDING
Validation: PARTIAL (arm64 phone runtime PASS; multi-ABI release artifact PASS; T06 overlay model/clustering/symbols/accessible list/safe generation PASS; T07 technical audit/SBOM/notices/endpoint decision PASS; armv7 runtime, native cluster hit-test/artwork, legal sign-off and 16 KB pending)
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
- **T06:** overlay model, duplicate/type/location handling, 60 m clustering,
  native Gas/ChargingStation/Information symbols, safe generation swap and the
  48 dp accessible list are implemented and tested. Native cluster hit-test at
  a dense OSM POI and final Abastevo artwork remain.
- **T07:** technical audit, 215-component notices/SBOM, endpoint decision and
  patch/rollback record are complete; human legal sign-off remains.
- **T08:** arm64 runtime PASS on the physical phone; the release artifact now
  carries both arm64-v8a and armeabi-v7a under the 80 MiB gate. Armv7 runtime
  and actual 16 KB runtime remain.

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
- Remaining T08: actual 16 KB-page runtime, armv7 runtime, T06 native cluster
  hit-test/final artwork and full ENGINE_GATE closure.

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

Multi-ABI extension (2026-10-24): [MAP-14_MULTI_ABI](MAP-14_MULTI_ABI.md).
The release now carries both `arm64-v8a` and `armeabi-v7a`:
`app-release-unsigned.apk` **80,407,806 bytes (76.68 MB)**, SHA-256
`b2ede0a65b819ce59e227b5ece67bfbe555e07afeda654d15bbcac78e364b97b`, both
cores with 0x4000 LOAD alignment, `zipalign` PASS and the 80 MiB gate PASS
with ~3.32 MiB headroom. The armv7 runtime is not executed; 16 KB runtime
remains deferred.

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
claim that its root cause is fixed. T07 legal sign-off, T06 native cluster
hit-test/final artwork, armv7 runtime, measured budgets and actual 16 KB
runtime remain open. The release packages both arm64-v8a and armeabi-v7a
cores; armv7 runtime is not proven.

## T06 overlay model, clustering and accessibility — 2026-10-24

Status: **T06 PARTIAL / IN_PROGRESS**. The overlay model, duplicate/type
handling, 60 m clustering, native map symbols, safe generation swap and
accessible list are implemented and tested. The accessible list now uses the
prepared Abastevo fuel/electric pictograms. The native cluster hit-test at a
dense OSM POI location, native map artwork/theme review and the native 48 dp
target remain open.

Implemented in `maps/engine`:

- `MapOverlay.kt`: canonical UUID validation/normalization, exact-UUID duplicate
  merge (types and best location quality win), `UNKNOWN`/`CITY_CENTROID`/
  `REVIEWED` location states, fuel/charging flags, deterministic leader
  clustering, cluster IDs, KML with `Gas`/`ChargingStation`/`Information`
  symbols and orange/teal/bluegray styles, and flat accessible list items.
- `MapEngine`: `overlayClusters()` and
  `setOnOverlaySelected(Station|Cluster)`. A cluster selection carries all
  member markers; it never collapses distinct UUIDs into one station.
- `ComapsMapEngine`: the previous category is kept until a new category is
  found; the old category is deleted only after commit. The commit is posted
  after the bookmark callback because calling `showBookmarkCategoryOnMap`
  inside it reproduced a native `CHECK(m_updatedGroups.empty())` SIGABRT. The
  fix was verified on the phone. Overlay updates only touch
  `BookmarkManager`; no map package or routing graph is rebuilt.
- `MapOverlayList`: Compose list with 48 dp minimum item height, merged
  semantics and a TalkBack content description per station. The prepared
  Abastevo 128 px Android exports are 24 dp leading icons for fuel/electric
  (both side by side for a co-located station); they are decorative and the
  textual type label remains the accessible source. Labels are injectable; the
  future MAP-17 screen supplies localized strings.

Probe fixture: 7 markers, 6 unique UUIDs after duplicate suppression, three
clusters (one single, two multi), including a co-located fuel+charging station
and all three location states. A second generation adds one station.

Phone results (release AAR, isolated `mapProbe` variant):

```text
overlay generation=1 markers=7 clusters=3 pending
overlay committed generation=1 category=1002 markers=7 clusters=3 rejected=0 duplicates=1
MapProbe: first clusters=3 singles=1 multi=2 list=6
overlay generation=2 markers=8 clusters=3 pending
overlay committed generation=2 category=1003 markers=8 clusters=3 rejected=0 duplicates=1
MapProbe: second clusters=3 singles=0 multi=3 list=7
```

After generation 2 only `files/bookmarks/abastevo-overlay-2.kml` remained; the
generation-1 file was removed with its category. The cluster pin rendered with
the `Information` symbol and label `ABASTEVO_CLUSTER:c2-1`; the fuel station
rendered with the orange `Gas` symbol. The charging-only `ChargingStation`
mapping is asserted by the KML test; it is not visible as an individual pin in
the fixture capture because that marker is inside a cluster. Evidence image:
[t06-clusters.jpg](map14-phone/t06-clusters.jpg) (cropped to exclude an
unrelated user notification).

Tests: **12** engine JVM tests PASS (KML declaration/escaping, icons/styles,
duplicate merge, co-located distinct UUIDs, nearby/far clustering, invalid
rejection, cluster KML, accessible labels/types, selection mapping). **3** phone
instrumentation tests PASS before the pictogram addition (2 existing MapHost
lifecycle/host tests plus `MapOverlayListInstrumentedTest`, which asserts the
TalkBack description and `assertHeightIsAtLeast(48.dp)`). The current release
build includes both drawables; the icon list's device visual check remains
pending. Gradle's UTP install was blocked by `INSTALL_FAILED_USER_RESTRICTED`;
the self-instrumenting test APK was installed with `adb install` and run with
`am instrument`, 3/3 OK.

Known T06 limits:

- Tapping the cluster pin at the fixture location opened an OSM POI
  (`place page activated title=110139`) instead of the bookmark; this is the
  known OSM-vs-bookmark hit-test collision. The app-owned cluster selection
  mapping (`overlaySelectionFor`) is unit-tested, and individual station
  selection was proven in T08, but native cluster hit-test needs a clean
  location or an app-owned disambiguation/quick-sheet path in MAP-17.
- The accessible list uses the prepared Abastevo PNG exports. The native map
  overlay still uses the SDK `Gas`/`ChargingStation` symbols; native marker
  backing/outline/selection, map-background/20 px review and final artwork
  remain MAP-18. The list icon's device visual check remains pending.
- The accessible list has 48 dp targets; the native marker's own touch target
  was not separately measured.
- Clustering uses a fixed 60 m radius, not a zoom-dependent radius yet.
- A failed overlay import retains the previous category by construction; the
  failure path was not separately injected after the crash fix.
- The list labels default to English; MAP-17 must pass localized labels.
- The release APK remains 69,898,847 bytes because no production screen
  references the engine yet; R8 strips the adapter until MAP-17 wires the map
  tab. T06 is exercised in the `mapProbe` variant, engine unit tests and the
  self-instrumenting phone test APK, not in the current release APK.

Commands for the T06 checks:

```sh
JAVA_HOME=/data/dev/java/candidates/java/current \
  ./gradlew :maps:engine:testDebugUnitTest \
    -PcomapsSdkDir=/tmp/abastevo-comaps-release-arm64 \
    --offline --no-daemon --max-workers=2 --console=plain
# 12 tests, 0 failures

JAVA_HOME=/data/dev/java/candidates/java/current \
  ./gradlew :maps:engine:assembleDebugAndroidTest \
    -PcomapsSdkDir=/tmp/abastevo-comaps-release-arm64 \
    --offline --no-daemon --max-workers=2 --console=plain
adb install -r maps/engine/build/outputs/apk/androidTest/debug/engine-debug-androidTest.apk
adb shell am instrument -w -r \
  app.organicmaps.sdk.test/androidx.test.runner.AndroidJUnitRunner
# OK (3 tests)

adb install -r app/build/outputs/apk/mapProbe/app-mapProbe.apk
adb shell am start -n com.anpfuel.app.mapprobe/com.anpfuel.app.map.MapProbeActivity \
  --ez overlayOnly true
```

The probe and instrumentation packages were uninstalled and their data removed
after the run.

## T07 rights, SBOM and endpoint audit — 2026-10-24

Status: **TECHNICAL_AUDIT_COMPLETE / LEGAL_SIGNOFF_PENDING**. This is a
technical inventory, not a legal opinion and not an ENGINE_GATE closure.
Full record: [MAP-14_T07_AUDIT](MAP-14_T07_AUDIT.md).

- Source inventory: 193 pinned submodules (all clean) plus 22 vendored
  `3party` directories = **215 components**. 204 have a license/notice file and
  11 are resolved from an exact source header or a retained repository license;
  **0 unresolved**.
- Resolutions: `glm` MIT (dual Happy Bunny/MIT; MIT selected), `just_gtfs` MIT
  (pinned tree predates `LICENSE.MIT`; upstream `b5b1df6` supplies it),
  `monocypher` BSD-2-Clause, plus the 10 vendored source-header licenses
  (`GL` MIT, `agg` `LicenseRef-AGG`, `ankerl` MIT, `kdtree++` Artistic-2.0,
  `minizip` `Zlib AND Info-ZIP`, `opening_hours` MIT, `robust` public domain,
  `skarupke` BSL-1.0, `stb_image` MIT, `vulkan_wrapper` Apache-2.0).
- Notices/SBOM: a 219-license-file notice bundle (226 files total, 3.0 MB;
  deterministic tar SHA-256
  `c2d84594a4e32bc9f8d8909fc57c908fb8b579500d869a9f616764a491a401a8`) and a
  CycloneDX 1.5 SBOM with 219 components. Durable artifacts are committed under
  [map14-t07/](map14-t07/): `THIRD_PARTY_NOTICES.md`, `sbom.cdx.json`,
  `audit.json` and `SOURCE_OFFER.md`.
- Endpoint scan: 30 native URL hosts, **0 unclassified**; 10 upstream service
  hosts are `UPSTREAM_SERVICE_UNUSED`, 12 OSM/data/schema and 8
  social/reference. No secret-pattern hits. Decision `DOCUMENTED_UNUSED`: the
  adapter configures no CoMaps account/key and does not invoke the upstream
  download service; the T08 airplane-mode render/search/route proof shows no
  required private service. The strings remain in the binary.
- ODbL: the regional `.mwm` is a Derived Database under ODbL 1.0 with
  `Map data © OpenStreetMap contributors`; the source/derived-data recipe and
  package are retained, and the public offer URL remains a release obligation.
  The Abastevo overlay is a separate database, not merged into the OSM data or
  relicensed as ODbL.
- Patch reapplication/rollback is recorded in
  [maps/engine/upstream/README.md](../../../../maps/engine/upstream/README.md);
  a changed SHA/ABI invalidates this audit and the T08 evidence.

Remaining T07 work: human legal acceptance of the resolved licenses and ODbL
determination, publication of the durable source/derived-data offer, and the
final per-asset font/symbol/style admission review. No legal sign-off or public
offer URL is claimed here.
