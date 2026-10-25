# MAP-15 — Android public map persistence and download recovery

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (JVM data layer + real phone Room/Ed25519/JNI/migration/worker instrumentation)
Integration: INTEGRATION_PENDING

Tested tree: `codex/map-ev-implementation` at `6106b4d9` plus the MAP-15
closure changes. Date: 2026-10-25. No map UI was added.

## Behavior frozen

- Room schema 26 adds only public map metadata: `map_generation` and
  `map_package` with a composite `(generation, package_id)` key. Package
  binaries live outside Room under the app files directory.
- `MapManifestVerifier` verifies the MAP-12 Ed25519 envelope, manifest SHA-256
  and trusted `key_id` before any row is written. The platform provider is
  preferred; API 26 without the provider falls back to the owned
  Monocypher/BSD-2-Clause bridge. There is no digest-only fallback; absent
  providers fail closed.
- `MonocypherEd25519` is a small owned JNI bridge over vendored audited
  `monocypher.c`/`monocypher-ed25519.c` (Monocypher 4.0.2, BSD-2-Clause OR
  CC0-1.0, byte-identical to the pinned MAP-14 source). No cryptographic
  algorithm is reimplemented in Kotlin.
- `MapPackageRepository` downloads with HTTP `Range`/`If-Range`, keeps a
  `.part` file for resume, verifies exact size and SHA-256, then atomically
  renames. A corrupt/short/oversized result is removed and marked `FAILED`.
- Activation verifies every final file still matches before flipping the
  active flag, requires every package `READY`, preserves the prior active
  generation on any failure, and deletes only inactive generation
  directories. `activeMapFiles()` exposes the verified files under `filesDir`,
  which the SDK `StoragePathManager.findMapsStorage` recursive `.mwm` scan
  discovers without adb seeding or probe fixtures.
- A 32 MiB reserve is checked before starting a download; insufficient space
  fails without making a network request.
- `MapDownloadWorker` is a Hilt `CoroutineWorker`: storage/digest failures are
  terminal, network `IOException` requests WorkManager retry, and cancellation
  leaves the `.part` file for the next run. No account-private table or route/
  favorite data is touched. `MapDownloadScheduler` enqueues unique
  `map-download-<generation>` work with `CONNECTED` + storage-not-low and
  exponential 30s backoff; inputs are generation + base URL only.
- Public maps require no login; the worker/inputs are generation + base URL
  only. Account switch/logout never cancels or leaks private state into the
  download.

## Implementation

- `data/local/map/MapPackageEntities.kt`, `dao/MapPackageDao.kt`,
  `AnpFuelDatabase` v26 and `MIGRATION_25_26`.
- `data/map/MapManifestVerifier.kt`, `Ed25519Verifier.kt`,
  `MonocypherEd25519.kt`, `MapPackageStore.kt`,
  `MapPackageRepository.kt` (open for worker tests, `activeMapFiles()` +
  `containsNativeMapData()`).
- `data/src/main/jni/`: vendored `monocypher/` (byte-identical to
  `/data/dev/src/comaps-4587b8ad/3party/monocypher`, hashes in
  `provenance.json`), owned `ed25519_bridge.c`, `CMakeLists.txt` with
  16 KB `max-page-size=16384`; `data/build.gradle.kts` externalNativeBuild.
- `data/worker/MapDownloadWorker.kt` and `MapDownloadScheduler.kt`,
  `data/di/MapModule.kt`.
- `data/schemas/.../26.json`.

## Verification executed

JVM tests (`:data:testDebugUnitTest`, all green):

- `MapManifestVerifierTest`: valid envelope accepted; altered
  message/key/signature rejected; invalid 32/16-byte sizes fail closed;
  unknown key refused; rotation to a new `key_id` accepted while the old
  key still verifies; platform-unavailable fallback fails closed without
  JNI; empty-message round-trip proves exact-byte handling.
- `MapPackageRepositoryTest` (7 cases): signature, rotation,
  active-reimport refusal, resumable 206 + digest + activation, digest
  mismatch preserves prior generation, disk budget makes zero requests,
  active deletion refused.
- `MapPackageRecoveryTest` (5 cases): interrupted download resumes after
  simulated process death via a new repository over the same files;
  corrupt final file fails activation and preserves prior generation;
  concurrent readers keep prior generation files readable during a failed
  update; cancellation leaves `.part` for resume without activation;
  public files root contains only `map-packages` (no private state).
- `Migration25To26UnitTest`: append-only SQL creates only the two map
  tables/indexes, no drops/deletes.

Real phone instrumentation through adb (2311DRK48G, Android 16 / API 36):

```sh
./gradlew :data:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.data.map.MonocypherEd25519InstrumentedTest
# Starting 1 tests on 2311DRK48G - 16
# Finished 1 tests on 2311DRK48G - 16
# BUILD SUCCESSFUL

./gradlew :data:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.data.local.map.V25ToV26DatabaseMigrationTest
# Starting 2 tests on 2311DRK48G - 16
# Finished 2 tests on 2311DRK48G - 16
# BUILD SUCCESSFUL

./gradlew :data:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.data.worker.MapDownloadWorkerTest
# Starting 6 tests on 2311DRK48G - 16
# Finished 6 tests on 2311DRK48G - 16
# BUILD SUCCESSFUL

./gradlew :data:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.data.map.MapPersistenceInstrumentedTest
# Starting 1 tests on 2311DRK48G - 16
# Finished 1 tests on 2311DRK48G - 16
# BUILD SUCCESSFUL
```

- `MonocypherEd25519InstrumentedTest` proves the JNI bridge on-device:
  Go `crypto/ed25519` fixtures with seed `00..1f` (pub
  `03a107bf…`, sig `61f67425…` for `abastevo-map-test`, empty-message sig
  `9ca53579…`) verify true; mutated key/message/signature and wrong
  16/32-byte sizes fail closed; platform round-trip agrees with Monocypher.
- `V25ToV26DatabaseMigrationTest` proves the real historical upgrade
  18→26 preserves `vehicle` rows, creates only `map_generation`/`map_package`
  with no BLOB column, and recovers after an injected failure.
- `MapDownloadWorkerTest` proves terminal failure on storage/digest,
  retry on `IOException`, success activates atomically, missing inputs fail
  without network, and the scheduler uses unique `CONNECTED` work with
  exponential backoff.
- `MapPersistenceInstrumentedTest` retains the Room + platform Ed25519
  device proof. The normal app build and `:data:assembleDebug` (with the
  new `libmonocypher_ed25519.so` for all four ABIs) pass.

## Limits / follow-up

- No actual package download was performed on the phone; the wire path is
  covered against MockWebServer plus the scheduler/worker device tests.
- The 25.json export remains absent (only 18.json + 26.json are versioned);
  the historical proof runs 18→26 through every append-only migration,
  which includes 25→26 as the final step. A future 26→27 gate will use the
  versioned 26.json.
- No map UI, Compose host, or ENGINE_GATE closure is claimed. MAP-16 remains
  the next integration task.
