# MAP-15–20 review remediation — 2026-10-25

Status: LOCAL_DONE / INTEGRATION_PENDING (bounded defect-remediation checkpoint)
Validation: PASS for the scoped checks below; MINIMUM_MAP_GATE remains PARTIAL

This record supersedes the earlier claims about native registration, reader
leases, stub prices, complete filters and completed artwork. It does not accept
all remaining MAP-18/MAP-20 product criteria or certify a production release.
The inherited commits `483cff59` and `2d51eef6` and four unfinished native files
were preserved. Code checkpoint: `51cc445700761d782587ce0a38450b6e356addb2`.

## Repairs and contracts

- **B-BR-MAP-PUBLISH-03**: all candidate bytes and backups are prepared before
  replacing live files; a preparing/ready journal distinguishes incomplete
  backup from an uncommitted swap. Room failure, partial swap and cancellation
  restore A. Recovery itself is non-cancellable. Legacy complete journals remain
  recoverable. Availability reads observe one publication epoch.
- **B-BR-MAP-READERS-03**: lazy snapshot readers lease immutable generation files,
  rather than shared native paths that B replaces. Native readers are detached
  on Main before publication, then all regional maps are registered again after
  commit or restoration. Data knows an application-owned observer port, not SDK
  classes. A successful file copy is not treated as native-render evidence.
- **B-BR-MAP-DIST-CONFIG-03**: Gradle resolves the distribution properties with
  `get()`; previous builds accidentally embedded Provider descriptions. The
  default is genuinely unconfigured. Only an explicitly configured debug
  loopback origin permits HTTP, including a same-origin redirect guard; release
  remains HTTPS-only. Manifest reading is bounded during streaming.
- **B-BR-MAP-CAMERA-03**: native center and zoom drive discovery and are saved
  on host stop; zoom-only changes update the radius. Native availability queries
  marshal to Main. Rendering restoration reapplies the last native camera.
- **B-BR-MAP-SHEET-03**: conditional prices retain their terms; wrong unit,
  product, period and connector are refused. Source controls inside the fully
  expanded, scrollable sheet are reachable without touching the scrim behind
  it. Source/current/power/access chip rows scroll horizontally at enlarged
  font sizes. The former 100 ms / 300 ms fixed test waits now observe actual outcomes.
- **BUC-MAP-CITY-03**: the city chip reuses the national IBGE picker and returns
  to MAP. Unknown geometry stays selectable in the scoped Directory list and
  cannot become an exact pin or coordinate route. The existing ANP survey-week
  selector is reused. Product/access labels use localized resources; absent
  opening-hours metadata is explicitly unknown.
- `anpfuel://map` is registered in Android's manifest in addition to the
  Navigation graph. This closes the external-intent resolution gap.

## Verification

- Antigravity's activation and condition regressions remain in the permanent
  suites. `MapPublicationInterruptionTest` adds cancellation during swap,
  incomplete-backup recovery, native-reader coordination, cancellation during
  detach, and opening A lazily after a same-path B swap. Scoped data suites PASS.
- `MapCameraSyncTest`: RED on the old camera implementation, then 2/2 GREEN
  for zoom-only radius updates and persisting the actual native camera.
- JVM matrix during this batch: domain 675, application 454, data 609 (two
  unrelated external checks skipped), engine 32 and app 535; zero failures.
  `MapDistributionClientTest` also passes 3/3 in release, proving no release
  HTTP exception. Final build details are in the [receipt](MAP-15_20-final/receipt.json).
- `lintDebug` PASS after fixing explicit `includeSubdomains=false` on the
  debug-only loopback entries. No lint suppression or baseline was added.
- Real phone: 2311DRK48G, Android 16 / API 36, arm64. The opt-in production
  `MainActivity` + Hilt + real-engine download test passed 1/1 in 14.923 s.
  It requests two distinct signed generations through the app UI, waits for
  the exact active generation, proves a downloaded regional map at Curitiba,
  obtains non-empty native offline search results and recreates the Activity.
- Phone route/screen suite passed 5/5 in 16.219 s: real tab return/rotation/
  recreation plus source switching inside the sheet, second/last cluster
  member selection and coordinate-less profile access with route disabled.
  These device tests ran during construction; they are not an immutable
  commercial-release certification. The owner then removed the phone.
- Emulator UI: final `MapScreenInstrumentedTest` passed 5/5 in 11.269 s
  at font scale 2.0 and 320 dp width (840x1800, 420 dpi), including real
  chip selection/apply values and virtualized list scrolling. The emulator
  used a read-only/no-snapshot 16 KB API 35 image, ARM64 native bridge,
  SwiftShader and 4 GiB RAM; UI tests inject a fake map surface and do not
  recertify native map rendering. Android also resolved `anpfuel://map`
  to the real MainActivity. The initial hardware-GPU emulator exited 139;
  the 2 GiB software attempt was killed by Android's low-memory killer.
  These attempts are recorded, not counted as accepted runtime profiles.
  Earlier filter clicks scrolled only the inner horizontal parent: the test
  now exposes the row vertically, scrolls to the chip, requires it selected,
  and checks the values actually applied. No selection assertion was removed.
- Final app JVM 535/535 and lint PASS at the source checkpoint above. Final
  unsigned release APK: 78,121,062 bytes / 74.502 MiB, SHA-256
  `4374b4320c7939bc37cb62c346fbcb4f310936b9d37f6ed06621c57d4abbefcb`.
  All eight packaged native libraries retain 16 KB ELF LOAD and ZIP alignment;
  both ARM cores match the admitted MAP-14 receipt. Final debug/release builds
  contain empty distribution settings, without the proof origin/key.
  The proof server and temporary emulator were stopped; the phone was left
  untouched after disconnection. Python map harness tests pass 9/9.
- A native abort was reproduced during the initial proof because an
  instrumentation-thread availability query entered a Main-only JNI method.
  The adapter now marshals the query; the succeeding real-engine proof is
  retained. Earlier failed proof attempts remain local evidence.
- No SQL schema or migration changed in this continuation. The real Room
  activation is exercised by the production download flow on the phone.
- Scoped compilation/static checks, secret scan, diff-check, release size and
  native alignment are recorded with the [final receipt](MAP-15_20-final/receipt.json). Existing engine
  binary/resource admission stays pinned; no SDK core was rebuilt.

## Reproducing the local native proof

`scripts/maps/native-proof-server.py` binds only to 127.0.0.1, verifies all
three map inputs against the committed MAP-14 receipt, signs with an ephemeral
in-memory Ed25519 key, and writes only its public key. The operator's existing
Python cryptography 50.0.0 is a local harness dependency (Apache-2.0 OR
BSD-3-Clause); no Android/backend dependency was added or downloaded.

```sh
python3 scripts/maps/native-proof-server.py \
  --inputs /path/to/admitted-map-dist \
  --public-key-output /tmp/map-native-proof-public-key.txt
# Build debug/test with comapsSdkDir and mapDistBaseUrl=http://127.0.0.1:18765,
# mapDistRegion=brazil-parana-east, mapDistKeys=local-native-proof:<public key>.
# The public key is operational test configuration, never a private key.
adb -s DEVICE reverse tcp:18765 tcp:18765
adb -s DEVICE shell am instrument -w -r \
  -e class com.anpfuel.app.ui.map.MapNativeDownloadInstrumentedTest \
  -e mapProofRun true -e mapProofOrigin http://127.0.0.1:18765 \
  com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner
```

The three packages total 72,067,998 bytes. The regional/world files are actual
MWM data from the admitted MAP-14 receipt, not arbitrary bytes or synthetic map
files. No files were seeded through adb, no app data was cleared to make the
proof pass, and no public CDN/staging database was changed.

## Reproduction of final scoped checks

```sh
./gradlew --offline :app:testDebugUnitTest :app:lintDebug \
  -PcomapsSdkDir=/path/to/admitted-engine --max-workers=2
./gradlew --offline :app:assembleRelease :app:verifyReleaseApkSize \
  -PcomapsSdkDir=/path/to/admitted-engine --max-workers=2
python3 -m unittest discover -s scripts/maps -p '*test*.py'
bash scripts/scan-secrets.sh
git diff --check
```

The earlier five-module JVM matrix and release HTTP-negative tests retain
separate logs in the receipt. Two unchanged external data checks were skipped
(training-consent Go/PostGIS interop and live ANP listing), not represented as
executed. The final source-only UI change reran affected app tests/lint/builds
and the emulator suite; unchanged backend/domain aggregates were not repeated.
The legacy `git-flow.sh checkpoint` helper supports `dev`/`codex/phase-*`, not
this inherited `codex/map-ev-implementation` branch. Atomic source commits and
this tracked receipt preserve the checkpoint without changing the helper or
renaming/resetting an occupied construction branch. No remote checks were polled.

## Remaining acceptance boundaries

- MAP-18 remains PARTIAL for the Abastevo-owned native combined pictogram and
  its visual-size/theme inspection. Native SDK glyphs/style plus an accessible
  list are not represented as completion of that artwork criterion.
- MAP-20 / MINIMUM_MAP_GATE remains PARTIAL: staging has no admitted reviewed
  station geometry/prices for the live pin → truthful quick price → profile
  journey, and no production distribution origin/key set is configured.
- The present charging/directory contracts do not provide opening hours.
  The UI states unknown; no fictional open-now filter is claimed. Public
  offline overlay acceptance remains OWED separately from the native basemap
  and native offline-search proof.
- MAP-21–33, native guided navigation, production distribution/activation,
  release signing, pilot/G09 certification and remote delivery are not accepted
  by this checkpoint. No owner authorization, rights or source review is invented.
