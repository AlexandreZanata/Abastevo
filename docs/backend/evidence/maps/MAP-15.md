# MAP-15 — Android public map persistence and download recovery

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (JVM data layer + real phone Room/Ed25519 instrumentation)
Integration: INTEGRATION_PENDING

Tested tree: `codex/map-ev-implementation` at `5268fb6a` plus the MAP-15
changes. Date: 2026-10-24. No map UI was added.

## Behavior frozen

- Room schema 26 adds only public map metadata: `map_generation` and
  `map_package` with a composite `(generation, package_id)` key. Package
  binaries live outside Room under the app files directory.
- `MapManifestVerifier` verifies the MAP-12 Ed25519 envelope, manifest SHA-256
  and trusted `key_id` before any row is written. It has no digest-only
  fallback; an unavailable Ed25519 provider fails closed.
- `MapPackageRepository` downloads with HTTP `Range`/`If-Range`, keeps a
  `.part` file for resume, verifies exact size and SHA-256, then atomically
  renames. A corrupt/short/oversized result is removed and marked `FAILED`.
- Activation requires every package in the generation to be `READY`. The prior
  active generation is preserved on any failure; cleanup deletes only inactive
  generation directories and never the active one.
- A 32 MiB reserve is checked before starting a download; insufficient space
  fails without making a network request.
- `MapDownloadWorker` is a Hilt `CoroutineWorker`: storage/digest failures are
  terminal, network `IOException` requests WorkManager retry, and cancellation
  leaves the `.part` file for the next run. No account-private table or route/
  favorite data is touched.
- Public maps require no login; the worker/inputs are generation + base URL
  only.

## Implementation

- `data/local/map/MapPackageEntities.kt`, `dao/MapPackageDao.kt`,
  `AnpFuelDatabase` v26 and `MIGRATION_25_26`.
- `data/map/MapManifestVerifier.kt`, `MapPackageStore.kt`,
  `MapPackageRepository.kt`.
- `data/worker/MapDownloadWorker.kt` and `data/di/MapModule.kt`.
- `data/schemas/.../26.json`.

## Verification executed

JVM tests (`:data:testDebugUnitTest`, all green; 7 repository cases + migration):

- valid envelope accepted; altered signature rejected;
- trusted key rotation with a new `key_id` accepted;
- re-importing an active generation as pending is refused;
- partial `.part` resumed with HTTP 206, exact digest verified and activated;
- wrong package bytes fail and leave the previous active generation untouched;
- disk budget refusal makes zero HTTP requests;
- active generation deletion refused;
- migration 25→26 creates only the two append-only map tables/indexes.

Real phone instrumentation through adb (2311DRK48G, Android 16 / API 36):

```sh
./gradlew :data:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.data.map.MapPersistenceInstrumentedTest
# Starting 1 tests on 2311DRK48G - 16
# Finished 1 tests on 2311DRK48G - 16
# BUILD SUCCESSFUL
```

The test opened a real Room database, inserted/updated/activated the map
generation and package rows, and verified Ed25519 sign/verify using the Android
provider. The normal app build and app unit suite also pass.

## Limits / follow-up

- WorkManager retry/cancel and process-death recovery are implemented but not
  yet exercised by an instrumentation test; the `.part` resume path is covered
  at repository level.
- The 25→26 migration is verified by SQL assertions and a fresh-schema device
  test, not a historical schema-25 `MigrationTestHelper` run.
- No actual package download was performed on the phone; the wire path is
  covered against MockWebServer.
- No map UI, Compose host, or ENGINE_GATE closure is claimed. MAP-16 remains
  dependent on the open engine gate.
