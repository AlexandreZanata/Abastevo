# MAP-15 — Android public map persistence and download recovery

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (Go + JVM + real-phone Room/native-layout/migration/worker instrumentation)
Integration: INTEGRATION_PENDING

Tested tree: `codex/map-ev-implementation` plus the MAP-15 closure changes
below. Date: 2026-10-25. No map UI behavior was changed by the rework except
the downloads screen wiring; no ENGINE_GATE claim.

## Behavior frozen

- Room schema 27 keeps only public map metadata: `map_generation` (now with
  `data_version`, the numeric native version directory) and `map_package`
  with a composite `(generation, package_id)` key. Package binaries live
  outside Room. `MIGRATION_26_27` is append-only (`ADD COLUMN` with an empty
  default); pre-existing rows count as unpublished until re-activated.
- `MapManifestVerifier` verifies the MAP-12 Ed25519 envelope, manifest SHA-256
  and trusted `key_id` before any row is written. The production path takes
  raw 32-byte pinned keys: platform-first via an X.509 wrapper, falling back
  to the owned Monocypher/BSD-2-Clause bridge on API 26. Unknown key ids,
  wrong key sizes and absent providers fail closed. No digest-only fallback.
- `MonocypherEd25519` is a small owned JNI bridge over vendored audited
  `monocypher.c`/`monocypher-ed25519.c` (Monocypher 4.0.2, BSD-2-Clause OR
  CC0-1.0, byte-identical to the pinned MAP-14 source). No cryptographic
  algorithm is reimplemented in Kotlin.
- Distribution is configured, never invented: `MapDistributionConfig`
  (base URL + region + pinned raw keys from build-time operational config)
  drives `MapDistribution.fetchEnvelope` (1 MiB cap, HTTPS-only). Empty
  configuration shows the specific pendency (`missingUrl`/`missingRegion`/
  `missingKeys`, invalid keys flagged); no fake download button exists.
- `MapPackageRepository` downloads with HTTP `Range`/`If-Range`, keeps a
  `.part` file for resume, enforces the signed size cap *during* the transfer,
  validates `Content-Range` on resume (missing/mismatched range with a
  non-empty part fails closed), re-checks disk space when the server ignores
  `Range` and resends the whole file, and verifies exact size + SHA-256
  before atomically publishing (`publish-part` + rename; a crash can never
  expose a partial file). Oversize/digest/range failures delete the staged
  bytes; network `IOException` stays retryable and never becomes success.
- Activation verifies every staged final file, requires every package `READY`
  and a numeric `data_version`, publishes the verified bytes to the native
  `<filesDir>/<dataVersion>/` layout the pinned engine scans (flat files only;
  non-publishable paths fail closed), flips the Room generation under a mutex
  (re-verified inside), then reclaims: only orphan staging dirs, terminal
  inactive staging, other numeric version dirs and stale files. In-progress
  (PENDING/DOWNLOADING) staging is never deleted. A concurrent activation
  re-verifies under the mutex instead of corrupting state.
- The prior active generation is preserved on any failure (failed B never
  touches A's published files; open readers keep old bytes across an atomic
  rename swap). Crash between publish and Room flip heals on the next
  `activate` (idempotent re-publish).
- A 32 MiB reserve is checked before starting a download; insufficient space
  fails without making a network request.
- `MapDownloadWorker` is a Hilt `CoroutineWorker`: storage/digest/version
  failures are terminal, network `IOException` requests WorkManager retry,
  and cancellation leaves the `.part` file for the next run. An unsupported
  data version fails closed. No account-private table or route/favorite data
  is touched. `MapDownloadScheduler` enqueues unique `map-download-<generation>`
  work with `CONNECTED` + storage-not-low and exponential 30s backoff; inputs
  are generation + base URL only.
- Public maps require no login. Account switch/logout never cancels or leaks
  private state into the download.
- Legacy (pre-layout, empty `data_version`) active rows are repaired by a
  same-generation re-import, which keeps the active flag; any other active
  re-import is refused.

## Implementation

- `data/local/map/MapPackageEntities.kt`, `dao/MapPackageDao.kt`,
  `AnpFuelDatabase` v27, `MIGRATION_25_26`, `MIGRATION_26_27`.
- `data/map/MapManifestVerifier.kt` (raw-key `verify` overload +
  platform/X.509-first helper), `Ed25519Verifier.kt`,
  `MonocypherEd25519.kt`, `MapDistributionKeys.kt` (strict `keyid:b64`
  parsing), `MapDistribution.kt` (bounded envelope fetch),
  `MapPackageStore.kt`, `MapPackageRepository.kt` (native layout publish,
  mutex-guarded activate + retention, hardened transfer).
- `data/src/main/jni/`: vendored `monocypher/` (byte-identical to
  `/data/dev/src/comaps-4587b8ad/3party/monocypher`, hashes in
  `provenance.json`), owned `ed25519_bridge.c`, `CMakeLists.txt` with
  16 KB `max-page-size=16384`; `data/build.gradle.kts` externalNativeBuild
  plus `MAP_DIST_BASE_URL`/`MAP_DIST_REGION`/`MAP_DIST_KEYS` build config
  (empty by default = unconfigured).
- `data/worker/MapDownloadWorker.kt` and `MapDownloadScheduler.kt`,
  `data/di/MapModule.kt` (native root = `filesDir`, distribution config +
  distribution bindings).
- `data/schemas/.../26.json`, `27.json`.
- `app/.../MapDownloadsViewModel.kt` + `MapDownloadsScreen.kt` + `MapAppModule.kt`
  (WorkManager seam): check → envelope → verify → catalog/dependencies/size →
  explicit unique-work download with progress/cancel/retry → verified
  activation → observable published state.
- `app/.../MapScreenViewModel.kt`: no-package banner follows the real Room
  active generation.

## Verification executed

Backend (Go, real stack):

- `go test ./internal/modules/mapdist/ -run 'TestCommittedKotlinFixture|TestDistribution'`
  PASS: committed `contracts/testdata/map/dist-kotlin-v1` loads (`Load`
  verifies the real Go signature + package digests), unknown key ids are
  refused with `ErrUntrustedKey`, wrong key bytes with `ErrBadSignature`;
  pre-existing `TestDistributionHandlerManifestAndRange` and
  `TestDistributionRejectsTamperingAndUnsafePackages` still PASS.
- Local real HTTP server (`mapdist.Handler` over the committed fixture,
  temporary, removed after use): manifest 200, package 200 (4096 bytes,
  sha256 matches manifest), `Range: bytes=4-` → 206 with exact
  `Content-Range: bytes 4-2047/2047` + ETag = package sha256, unknown
  region/package → 404. The committed envelope's `manifest_sha256` and the
  served package bytes were independently re-verified (curl + sha256).

JVM tests (`:data:testDebugUnitTest`, all green):

- `MapManifestVerifierTest`: previous platform-path cases unchanged.
- `MapManifestGoFixtureTest` (new): the committed Go envelope verifies
  through the raw-key path; cloned key id, wrong key bytes, truncated
  signature and tampered manifest all fail closed; the Monocypher bridge is
  exercised with the same Go bytes where JNI is present and asserts
  fail-closed absence otherwise; `parseMapKeys` accepts the pinned entry and
  refuses empty/duplicate/malformed/wrong-size configs.
- `MapPackageRepositoryTest` (7 cases, kept): legacy platform-key paths still
  green; 206 mocks now carry spec-compliant `Content-Range`.
- `MapPublishTest` (new, 13 cases): Go-signed fixture publishes exact bytes
  to `native/<version>/`; failed B preserves published A; concurrent
  activations converge on one coherent generation; open readers keep old
  bytes across a same-path swap; partial publish heals on re-activation;
  oversize body fails closed without activation; `Content-Range`
  missing/mismatched fails closed and full resend recovers; wrong start
  fails; ignored-Range re-checks full-size space; unsupported version fails
  without touching native state; terminal staging recycled while
  in-progress/pending kept; legacy blank-version rows repair on
  same-generation re-import; `checkResumeRange` unit cases.
- `MapPackageRecoveryTest` (5 cases, kept): resume after process death,
  corrupt final, concurrent readers, cancellation, no private state.
- `Migration25To26UnitTest`: untouched, green.

Real phone instrumentation through adb (2311DRK48G, Android 16 / API 36):

```sh
./gradlew :data:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.data.map.MonocypherEd25519InstrumentedTest
# 1/1 PASS (Go seed 00..1f vectors, mutations, sizes, platform agreement)

./gradlew :data:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.data.local.map.V25ToV26DatabaseMigrationTest
# 2/2 PASS (18→26 preserves rows, rollback recovers)

./gradlew :data:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.data.local.map.V26ToV27DatabaseMigrationTest
# 1/1 PASS (26→27 preserves rows, data_version defaults to "")

./gradlew :data:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.data.worker.MapDownloadWorkerTest
# 7/7 PASS (terminal storage/digest/version failures, retry on IOException,
# success activates, missing inputs fail, scheduler constraints)

./gradlew :data:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.data.map.MapNativeLayoutInstrumentedTest
# 1/1 PASS (device-signed envelope verifies via raw-key path, exact bytes
# land in files/2601/, restart keeps Room+filesystem coherent, open reader
# survives the atomic same-path swap, no .part leftovers)

./gradlew :data:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.data.map.MapPersistenceInstrumentedTest
# 1/1 PASS (Room + platform Ed25519 device proof retained)
```

- Staging probe 2026-10-25: `GET /v1/map/regions/{world,brazil,brazil-parana-east,test-rect}/manifest`
  and `/v1/map/assertions?limit=1` all return 404 on
  `https://teste.abastevo.com.br` → operational distribution is not
  configured there, so the app shows the pending state. No host/key was
  invented to work around this.
- App JVM: `MapScreenViewModelTest` (active/no-package, source switch,
  retry) and `MapDownloadsViewModelTest` (unconfigured pendency, offline
  retryable, catalog check, enqueue/cancel, failure retry, activateActive,
  same-generation check) green.

## Limits / follow-up

- Keys/base URL/region default to empty (unconfigured) in this worktree: no
  download can start until operational values are supplied via
  `-PmapDistBaseUrl/-PmapDistRegion/-PmapDistKeys`. Publishing those values
  is an operational step, not part of this change.
- Two full-suite observations on 2311DRK48G are environmental and outside
  MAP-15: `RealImageOcrDeviceTest` requires a manually pushed private corpus
  (`run-as` step named in its own assertion), and
  `DatabasePocGateTest.databaseFileSizeUnderFifteenMegabytesAfterOneWeekStationImport`
  reports exactly 15 MB on this device while an empty-table schema delta
  cannot move megabytes (physical reasoning recorded; typical budget is
  device/storage-sensitive). The two migration-chain tests that surfaced as
  missing-path (`CatalogMigration17To18Test`, `CommunityCacheMigration16To17Test`)
  were repaired in this batch with the full chain to 27.
- No map UI behavior change beyond the downloads screen wiring; no ENGINE_GATE
  re-claim (the native side is proven by MAP-14 over the identical layout).
