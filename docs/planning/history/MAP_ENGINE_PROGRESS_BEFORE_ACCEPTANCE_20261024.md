# Superseded map engine progress

Retained before the local engine acceptance.

# Current execution state

- Map/EV execution 2026-10-24: MAP-00–07 LOCAL_DONE / INTEGRATION_PENDING;
  MAP-08 LOCAL_DONE; MAP-09–13 LOCAL_DONE / INTEGRATION_PENDING; MAP-14
  IN_PROGRESS / ENGINE_GATE_PARTIAL; MAP-15 and MAP-16 LOCAL_DONE /
  INTEGRATION_PENDING; MAP-17 next but still gated by ENGINE_GATE. Local commits
  dc3282fb..8b5a3a21 on `codex/map-ev-implementation` (base `721582b8`); tree
  clean, no push/PR/merge. [MAP-08](../backend/evidence/maps/MAP-08.md),
  [MAP-09](../backend/evidence/maps/MAP-09.md),
  [MAP-10](../backend/evidence/maps/MAP-10.md),
  [MAP-11](../backend/evidence/maps/MAP-11.md),
  [MAP-12](../backend/evidence/maps/MAP-12.md),
  [MAP-13](../backend/evidence/maps/MAP-13.md),
  [MAP-14](../backend/evidence/maps/MAP-14.md).
  MAP-12: Ed25519 signed distribution envelope, pinned public keys, immutable
  package IDs, Range/ETag, config pairing, OpenAPI and actual API proof.
  MAP-13 DATA_GATE: real Rust registry→Go/PostGIS→HTTP→Kotlin codec, native
  package verification, blocked-license refusal. MAP-14: pinned arm64 release
  `.so` 14,703,416 bytes with 16 KB LOAD alignment; optional `maps/engine`
  adapter module; debug Abastevo APK rendered Curitiba at 21–27 FPS, panned,
  returned 29 offline search results, built a 1,575.15 m car route, and rendered
  a UUID-titled overlay pin, and an `ABASTEVO:<uuid>` place-page hit-test. The
  release AAR/linker blocker is resolved. Remaining gates: custom pictograms/
  co-located symbols/clustering/accessibility, complete lifecycle matrix,
  transitive license resolution, actual arm64/16 KB runtime, and ENGINE_GATE
  closure. The user authorized 66.64 MiB and the release gate is 80 MB with
  `verifyReleaseApkSize` passing.
  Review reconciliation: MAP-10 now rejects `NaN`/`Inf` before storage and
  projects `price_known` with the source-specific price array; MAP-12 re-verifies
  package bytes per response and returns `409` on mutation; MAP-13 feeds the
  actual API response fixture into the new Kotlin `MapDiscoveryResponseCodec`
  and cross-checks it against the prepared corpus. MAP-14 adapter now serializes
  concurrent init, cancels stale search/route work and removes listeners on
  release; manual rotation/relaunch/permission-denied browsing passed.
  T07 technical audit: 193 submodules + 22 vendored = 215 components; 204
  license files + 11 resolved, 0 missing. 219-license-file notices bundle and
  CycloneDX 1.5 SBOM; 30 endpoint hosts classified, 0 unclassified, no secret
  hits; upstream service strings documented unused; ODbL derived-database and
  source-offer determination recorded. Legal sign-off and public offer URL
  remain. T08 static: release APK 69,898,847
  bytes (66.66 MiB), SHA `d8094048…`, zipalign/16 KB LOAD PASS; packaged arm64 core
  14,675,096 bytes, SHA `08342926…`, three 0x4000 LOAD segments. T08 arm64
  runtime PASS on the authorized physical phone (2311DRK48G, Android 16/API
  36, 4 KB pages) with the release AAR inside an isolated `mapProbe` variant:
  render, pan, double-tap zoom, 38-result offline search, 1.6 km/288 s offline
  route and overlay UUID selection all worked; rotation recreated the surface
  and repainted. Evidence: [MAP-14](../backend/evidence/maps/MAP-14.md).
  The prior blank screen was caused by unreadable adb-created external map
  files; the probe used internal `files/261004` data. Overlay KML now starts
  with the XML declaration and escapes titles, fixing `Could not parse KML`.
  Owner chose the physical phone/all-device direction; actual 16 KB runtime is
  DEFERRED and not proven. BLAST buffer warnings remain despite ~120 FPS
  rendering. ENGINE_GATE remains PARTIAL. The probe package was uninstalled and
  its data removed. Multi-ABI release artifact PASS: armeabi-v7a AAR
  34,517,731 bytes / core 10,499,236 bytes; merged with arm64; universal APK
  80,407,806 bytes (76.68 MB), SHA `b2ede0a6…`, both cores 0x4000-aligned,
  zipalign PASS and 80 MiB gate PASS (~3.32 MiB headroom). Armv7 runtime is not
  executed; 16 KB runtime remains deferred. Clean-build double-build PASS:
  generated `android/sdk/build`/`.cxx` removed, arm64 AAR rebuilt; AAR
  `1fe196f9…`, `classes.jar` `074122f4…` and core `08342926…` byte-identical
  (`cmp` 0, 4m 1s clean build). Fresh-checkout clone comparison remains.
  T06 LOCAL_DONE / INTEGRATION_PENDING: canonical UUID normalization/dedupe,
  fuel/charging flags,
  UNKNOWN/CITY_CENTROID/REVIEWED states, 60 m clustering, native
  Gas/ChargingStation/Information symbols and accessible Compose list (48 dp,
  content descriptions) implemented; the prepared Abastevo fuel/electric
  exports are now 24 dp list icons. The old overlay category is retained
  until the new generation commits; deferring `showBookmarkCategoryOnMap` fixed
  a reproduced native `CHECK(m_updatedGroups.empty())` SIGABRT. 12 engine JVM
  tests PASS; 3 phone instrumentation tests PASS after the list-icon addition
  (TalkBack, 48 dp, fuel/electric icon composition). Phone probe: generation 1
  (7 markers/6 unique/3 clusters) and generation 2 (8/7/3) committed, old file
  removed, cluster/gas pins rendered. Clean-location cluster tap PASS:
  `ABASTEVO_CLUSTER:c1-0`, 3 UUID members. Tapping the same cluster at a dense
  OSM POI opened the OSM POI (title 110139); MAP-17 must disambiguate or route
  through the accessible list. Native map artwork/theme review and the native
  marker 48 dp target remain MAP-18/integration items.
  MAP-15: Room v26 public map metadata, resumable `.part` download, Ed25519
  envelope/key rotation, SHA-256, reserve/disk failure, all-package activation,
  safe inactive cleanup and Hilt WorkManager worker. JVM data tests and a real
  phone Room/Ed25519 instrumentation test PASS; WorkManager retry/cancel and
  historical schema-25 migration remain follow-up.
  MAP-16: Compose `MapHost` over the `MapEngine` port, explicit recenter-only
  location permission, no hidden GPS, lifecycle camera persistence, dispose
  cancellation and 48 dp recenter. 5 JVM view-model tests and 2 real-phone
  Compose host/recreation tests PASS; the native arm64 surface is now proven on
  the physical phone by T08, and T06 supplies the overlay clustering and
  accessible list; MAP-15 package layout mapping remains open.
  Tests: `go test ./...`, targeted `-race -tags=integration`, MAP-12/13 process
  proofs, `MapPoiCodecTest`, `MapDiscoveryResponseCodecTest`, `:data` unit and
  phone instrumentation, `:maps:engine` 12 JVM tests and 3 phone instrumentation
  tests, normal app debug build and engine debug build, plus
  `:app:assembleMapProbe` with the release AAR, the physical-phone probe and
  `:app:assembleRelease :app:verifyReleaseApkSize` with the merged
  arm64/armv7 candidate.
  The Gradle UTP install was blocked by `INSTALL_FAILED_USER_RESTRICTED`; the
  self-instrumenting test APK was installed with `adb` and run with
  `am instrument`.
  Local commits only; no push/PR/merge, deploy, Play, production or
  RELEASE_CERTIFIED claim.
  Build recovery: [MAP-14 T03](../backend/evidence/maps/MAP-14_BUILD_RECOVERY.md)
  resolves the manual-linker/hybrid-AAR blocker. Actual arm64/API-26 release AAR
  35,160,271 bytes; stripped core 14,675,096 bytes; app release 69,881,875 bytes
  (66.64 MiB), 80 MiB gate and APK/64-bit ELF alignment PASS. No new native
  execution or full ENGINE_GATE acceptance. [Resume](MAP_EV_RESUME.md) records
  remaining tasks, the existing native Ed25519 implementation direction and
  prior DATA_GATE/distribution review obligations. Next: T06 dense-OSM
  disambiguation/native map artwork/native 48 dp target; T07 legal sign-off,
  public source offer and per-asset review; fresh-checkout build comparison,
  armv7 runtime and the deferred 16 KB runtime obligation before
  ENGINE_GATE/MAP-17. The multi-ABI artifact is under the existing 80 MiB
  budget. No further budget/linker owner decision.
