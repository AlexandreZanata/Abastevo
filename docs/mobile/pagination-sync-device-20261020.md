# Pagination and optional sync: Android device follow-up

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

The user made the ADB device available for tests on 2026-10-20, authorizing this scoped native follow-up to [the source review](pagination-sync-review-20261019.md). This overrides the earlier native deferral for this task only. Source parent: `46bdf737`, worktree/branch `codex/mobile-pagination-review`. Other occupied checkouts are unchanged.

## Device safety and candidate

Physical Android API36, arm64, model2311DRK48G. Serial and full ADB inventory remain in temporary local files. Updated only `com.anpfuel.app.debug` and its app/data test packages using `adb install -r`; the installed and candidate signer SHA256 matched. No uninstall, package-data clear, user-data seeder, account/consent action, contribution, GPS request or photo capture was part of these tests. Direct instrumentation explicitly passes `-e clearPackageData false`, avoiding the repository's data-clearing orchestrator configuration. Production package `com.anpfuel.app` is untouched.

The debug app's existing SQLite header was11 before update and16 afterward. All migrations were registered; the targeted tests independently prove the 11→12→13→14→15 and15→16 segments with retained fixtures. The actual app was not reset or seeded. This observes its schema transition, not an independent census of its private rows.

Candidate product APK SHA256: `5531e0cdad0e6402087b075d6369046b89caae9a0140fcd88e407bddc2727680`. The product remains installed. Network test recorded Wi-Fi/mobile-data as1/1, restored them and observed validated connectivity returning. Screen timeout and device-wide locale were not changed. Synthetic light/dark screenshots were visually inspected and stay in `/tmp`; no personal screen capture or APK is versioned.

## Native findings and changes

- The initial data run found invalid fixture UUIDs and missing `survey_week` parents. After those fixes, the targeted-CNPJ fixture exposed that every synthetic station shared one CNPJ; assigned distinct fixture identities. Corrected the recency fixture's expected id tie order to match the established date/price/id contract. Production SQL and identity validation were not relaxed.
- The new full-list Compose test initially used a plain ComponentActivity with a Hilt-only weekly selector. `StationsContent` is now internal and accepts a composable weekly-action slot, preserving its existing default. The list test supplies an empty action and exercises actual production list content in isolation.
- Two existing feed tests still expected pre-October14 navigation. Verified source history: contribution already uses `Routes.CAPTURE` (`9494e3118`); cards already use the station/fuel page (`e2a58f254`). Updated exact route assertions to those established contracts, retaining click and value checks.
- The staging smoke now requests twenty rows, validates unique city-bound pages and conditionally continues one page. Removed the invalid assumption that every city fits one100-row response; no larger live traversal or provider load test is performed.
- Added a persistent125-station Room scenario: close/reopen, twenty-row offline windows, no network/GPS calls, origin isolation and public clearing that retains detail rows. Added a5,000-station UI scenario with fewer than40 composed station semantics nodes at sampled positions, reaching the final row. Failed continuation retains the first window and retry appends the next window.

## Executed acceptance

Data instrumentation:27 cases, zero failures/skips. Classes:

- `DirectoryCacheMigration15To16Test` (1)
- `DirectoryCacheTransactionTest` (7)
- `DirectoryCatalogOfflineDeviceTest` (1)
- `StationPriceDaoPagingTest` (4)
- `DirectorySyncWorkerTest` (7)
- `StorageStatsRepositoryIntegrationTest` (1; actual fixture import counts2,344 summaries/19,676 station prices)
- `V11ToV12DatabaseMigrationTest` (3), `V12ToV13DatabaseMigrationTest` (1), `V13ToV14QualityOutboxMigrationTest` (1), `V14ToV15QualityKeyErasureMigrationTest` (1)

App UI instrumentation:8 cases, zero failures/skips: `StationsListSummaryTest` (1), `StationsPaginationDeviceTest` (2), `CommunityFeedScreenTest` (4), `StationsScrollFrameDeviceTest` (1). Staging HTTPS/validated connectivity:2 cases PASS, with a separate physical loss/return case PASS. Total:38 executed native cases, zero failures/errors/skips. These explicit probes execute rather than skip; they use public reads and an isolated in-memory cache. The synthetic WorkManager registration test in the real app package was intentionally excluded because it overwrites the user's schedule; isolated native worker tests cover schedule parameters and execution guards.

Commands after APK assembly/install:

```sh
adb shell am instrument -w -r -e clearPackageData false \
  -e class '<comma-separated data classes above>' \
  com.anpfuel.data.test/androidx.test.runner.AndroidJUnitRunner
adb shell am instrument -w -r -e clearPackageData false \
  -e class com.anpfuel.app.ui.stations.StationsListSummaryTest,com.anpfuel.app.ui.stations.StationsPaginationDeviceTest,com.anpfuel.app.ui.community.CommunityFeedScreenTest,com.anpfuel.app.ui.stations.StationsScrollFrameDeviceTest \
  com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner
adb shell am instrument -w -r -e clearPackageData false -e directoryLiveStaging true \
  -e class 'com.anpfuel.app.ui.stations.StagingDirectorySyncDeviceTest#actualValidatedInternetIsObserved,com.anpfuel.app.ui.stations.StagingDirectorySyncDeviceTest#actualVpsReadSurvivesOutageAndReconnect' \
  com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner
adb shell am instrument -w -r -e clearPackageData false -e directoryLiveStaging true \
  -e class 'com.anpfuel.app.ui.stations.StagingDirectorySyncDeviceTest#physicalInternetLossAndReturnReachTheConnectivityObserver' \
  com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner
```

Affected app/data test APK builds and app lint PASS; no backend/domain behavior changed during this follow-up, so their previous source-review acceptance is retained instead of rerunning aggregate suites. Final whitespace and tracked/changed-file secret checks PASS. Installed product bytes were independently pulled and matched the candidate SHA256 above.

## Frame diagnostic and limits

The first experiment collected FrameMetrics during Compose test-clock scrolling. It produced high numbers (full semantic scan:2,255 frames, p50≈43.3ms/p95≈100.6ms; short injected gestures:34 frames, p50≈177.4ms/p95≈333.9ms). These mixed virtual-clock/automation samples are retained as diagnostic history and do not measure normal user scrolling. The final functional Compose test checks virtualization and continuation only.

`StationsScrollFrameDeviceTest` separately uses MainActivity, the ordinary Choreographer clock, six actual touchscreen gestures and a synthetic5,000-row catalog. Its state is supplied only to composition; it does not seed storage. FrameMetrics excludes first-draw frames. Final physical diagnostic:828 frames, p50=9.263231ms, p95=16.781043ms. These normal-clock results supersede the unsuitable virtual-clock experiments for this device diagnostic. The test asserts successful input/rendering and records timings; no invented FPS threshold or performance acceptance follows from a passing diagnostic test. This is a debug/instrumented single-device sample, not release profiling or certification.

Only the named native scenarios are accepted. Remaining whole-app flows, a representative profileable/release performance budget and protected integration/revalidation against newer maintained dev remain owed. No production certification, deployment, PR/merge or wiki publication is claimed.

Ephemeral logs: `/tmp/mobile-pagination-review-device-{data,ui,staging,network,frame,real-frames}.log`, build/install logs with the same prefix. Test sources and this record are the reproducible evidence.
