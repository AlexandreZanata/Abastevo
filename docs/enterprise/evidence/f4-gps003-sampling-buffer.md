# EMP-GPS-003 — Adaptive sampling, local buffer and batch envelope

Date: 2026-11-16.
Status: LOCAL_DONE
Validation: PASS (host, build, guard and backend gates); native emulator acceptance OWED (see below)
Integration: INTEGRATION_PENDING

Second bounded F4 task of worktree branch `codex/enterprise-f4-gps002`, after
EMP-GPS-002 (`de8e2eaf`) and the merge of the F4 head `c7e407e6` (TRIP-022,
TRIP-023) at the coordinator's request (`283e9a3a`). Owner decision EDR-271;
real-device acceptance OWED; the physical phone was never used. Synthetic
positions only (unit tests and the emulator's simulated route).

Rules read: the EMP-GPS-003 block and its neighbours; the tracking
specification (BR-GPS-012, BR-GPS-017..020, BR-GPS-024, §8.1 offline and clock,
§8.2 interruption matrix rows for signal, provider and mock, §10.2
`sampling-v1`, §10.3 batch envelope, §10.10 gap reasons, acceptance 3 and 6);
the EMP-GPS-004 codec (`ParseBatch`, `Admit`, bounds 200 fixes / 8 KiB,
EDR-277..280) and EMP-GPS-005 (server gaps and filters, EDR-298..301); the
EMP-GPS-002 evidence and code.

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-501..505, BUC-EMP-255..257, EDR-325..327, all
used. Rules B-BR-EMP-501 (approved `sampling-v1`), B-BR-EMP-502 (per-fix
filter and flags), B-BR-EMP-503 (sealed buffer surviving restarts),
B-BR-EMP-504 (bounded batch envelopes delivered in order), B-BR-EMP-505 (gaps
are data; no coverage claimed through them) and BUC-EMP-255..257 are in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-GPS-003 — Adaptive sampling, local buffer and batch envelope". Decisions
in [16-decisions](../16-decisions.md):

- EDR-325: the specification's `sampling-v1` defaults without organization
  overrides; 5-second platform request; the fix instant is the device wall
  clock minus the report's monotonic age; integer batch units; economy not
  recorded on the server.
- EDR-326: `TrackingBatchOperations` builder/decoder and the shared oracle
  `tracking/track-batch-v1.psv`; a `gps.track.batch` producer re-checks the
  oldest run, the sequence, the counters and the dependency on the period's
  opening operation, and removes the fixes in the same write; splitting at
  200 fixes and 8 KiB, never across a gap, a boot or 24 hours; sealing
  triggers; delivery through the ordinary queue.
- EDR-327: a second sealed record `tracking-buffer` v1 per partition; recovery
  instant after the last buffered fix; bounds (2 000 unsealed fixes, 500
  gaps); fix-level gap reasons stay on the device.

## Shape

- Domain (Kotlin `enterprise.tracking`): `TrackingBatchOperations`
  (`batch`, `read`), `TrackingBatchFix`, `TrackingBatchFact`,
  `TrackingDropped`, `TrackingDropReason`; `TrackingSamplingPolicy`,
  `TrackingObservation`, `TrackingObservationOutcome`, `TrackingGap`,
  `TrackingGapReason`, `BufferedFix`, `SessionBuffer` (`onBoot`,
  `checkpointAt`, `openGap`), `TrackingSampler` (`observe`, `record`, `tick`,
  `closePeriod`), `TrackingBufferBook` (sealed record codec), `TrackingBatching`
  (`nextRun`, `fixes`, `due`, `sealed`); `TrackingOperations.sealOperation`
  shared by both builders.
- Application: `EnterpriseTrackingBatchProducer`,
  `EnterpriseTrackingProducers.BUFFER_RECORD_ID`,
  `EnterpriseTrackingStore.updateBuffer`, snapshot `buffers`,
  `EnterpriseTrackingService.record/checkGap/seal` (and sealing after every
  period end and at recovery), `TrackingRules.periodOpening`,
  `TrackingSessionView.pendingFixes/batches/gaps/openGap/overflow`;
  `TrackingCollector` inbox (`drain`, `run`), delivery hook, watchdog gap
  check and due sealing; `TrackingPlatform.wallMillis/elapsedMillis/
  batteryLow/providerEnabled`.
- Data: `ProtectedTrackingStore` holds the buffer record;
  `di/EnterpriseTrackingModule` (the policy gateway, replaceable).
- App: the platform's clocks, battery and provider state; the service's
  consumer loop and delivery hook (`OrganizationTrackingHost.Installed.deliver`);
  `EnterpriseTrackingPresentationModule` (the launcher, kept by screen tests);
  the tracking card's pending fixes, batch states and gap lines; 6 more
  strings × 8 locales (40 keys).
- Go: `tracking/application/track_batch_vectors_test.go` (consumer of the
  batch oracle); `organization/adapters/orgsync_tracking_native_fixture_test.go`
  (tags `integration && nativefixture`).
- Native tests: `data/src/androidTest/.../EnterpriseTrackingNativeDeviceTest.kt`
  (backend path, real emulator GNSS through a test-only listener, process
  death between stages) and
  `app/src/enterpriseDeviceTest/.../EnterpriseTrackingServiceDeviceTest.kt`
  (the shipped service, its notification, grant and stop action) with
  `EnterpriseTrackingDeviceBindings`; driver and source scripts
  `f4-logs/gps002-scripts/native-{driver,source}.sh`.

The batch oracle has 24 rows (11 OK, 12 MALFORMED, 1 PROTOCOL), SHA-256
`9cbdd508ff2843dda9df0b3eb64a8f0aa4c30f82b4e30d281922a0aa38ccf4f6`.

## RED

- Go batch vectors (`gps003-go-vectors-1.jsonl`): 24/24 rows pass against the
  existing `ParseBatch` on first run — an oracle check, not RED.
- Kotlin domain (`gps003-kotlin-domain-red.log`, XML
  `gps003-kotlin-domain-red-xml/`): `TrackingSamplerTest` 5/5 and
  `TrackingBatchVectorsTest` 24/25 failing (the corpus-completeness test passes
  against the frozen corpus and is not claimed).
- Application (`gps003-kotlin-application-red.log`, XML
  `gps003-kotlin-application-red-xml/`): `EnterpriseTrackingBufferTest` 6/6
  failing; 7 of the 12 EMP-GPS-002 tests also failed because every period end
  now seals through the stubbed buffer path.
- Data (`gps003-kotlin-data-red.log`, XML `gps003-kotlin-data-red-xml/`): the
  new buffer restart test and the EMP-GPS-002 restart test (whose recovery now
  seals) failing; 2/4.

## GREEN and REFACTOR

- Domain (`gps003-kotlin-domain-green-2.log`): tracking 443/443. The first run
  failed one assertion of my own test that counted five recorded fixes where
  the scenario records six (the first fix plus five triggers); the test now
  says so.
- Application (`gps003-kotlin-application-green-3.log`): 18/18. Two failures of
  the first run were test setup errors (a buffer that did not exist yet, a
  "dead" process whose listener was still attached). The third run exposed a
  real defect: a fix recorded at the same millisecond as a stop stayed outside
  its period on the device and blocked every later seal. Fixed twice over: a
  period now ends one millisecond after its last recorded fix, and a run is
  sealed under the opening of its period even if a race left it outside (the
  server counts such a fix `OUTSIDE_SESSION`; nothing waits forever).
- Data (`gps003-kotlin-data-green-1.log`): 6/6.
- The first enterprise validation build (`gps003-validation-build-1.log`)
  failed at the device-test Hilt graph: the EMP-GPS-002 policy gateway and
  service launcher were provided by modules that the device tests replace
  (`EnterpriseModule` needing `CompanyAccessClient`; `EnterprisePresentationModule`).
  The gateway moved to its own replaceable `EnterpriseTrackingModule` (device
  tests bind a synthetic `UNAVAILABLE` gateway) and the launcher to
  `EnterpriseTrackingPresentationModule`; the second build passed
  (`gps003-validation-build-2.log`, APK hashes `gps003-validation-apks.sha256`).
  This also repairs the EMP-GPS-002 commit, whose validation build would have
  failed the same way.

Named negatives mapped to tests:

| Negative | Tests |
|---|---|
| Raw coordinates in telemetry | every buffer, fix, batch fact and view prints redacted (`TrackingSamplerTest`, `EnterpriseTrackingBufferTest` first test); notification text has no coordinate (`EnterpriseTrackingServiceDeviceTest`); the native test scans the protected files for the trip, session, a latitude, the operation type and the record format |
| Uncontrolled growth | `the buffer is bounded …` (2 000 pending, overflow counted, nothing pending dropped, a batch ≤ 200); `… an oversized run is split under 8 KiB` (two batches, each ≤ 8 192 bytes, counters sent once); anchor-only writes at most once a minute |
| Lost checkpoint | `after a process death the buffer survives …` (application) and `recorded fixes survive a restart …` (data, reopened protected generation): pending fixes kept, interruption one millisecond after the last fix, fixes sealed at recovery, next boot's fixes in their own batch |
| Fabricated fixes | `gaps open after 180 seconds …` (a tick adds no fix; nothing recorded in a gap); filter test (mock, stale, before the period, inaccurate, jump, non-finite never recorded) |
| Coverage claimed through a gap | `a gap opens with its reason, seals what came before and no batch spans it`; `nextRun` stops at a gap start |
| Batch not matching the buffer | `… the batch producer accepts only the buffer's oldest run …` (wrong sequence refused inside the protected write, buffer unchanged) |
| Collection after stop | `the stop seals what was recorded and nothing after it is recorded` |
| Go/Kotlin divergence | `track-batch-v1.psv` consumed by `TestSharedTrackBatchVectors` and `TrackingBatchVectorsTest` |

## Validation

Gates on the merged tree that contains EMP-GPS-002, the F4 head `c7e407e6`
and this task (Go from `backend/` with go1.27.4, `f4-logs/gps002-env.sh`;
Gradle through `f4-logs/gps002-scripts/gradle.sh`, JDK 17, Gradle flock,
`--max-workers=2`):

```sh
gofmt -l ./internal ./cmd ./db; go build ./... && go vet ./... && go vet -tags=integration ./... \
  && go vet -tags='integration nativefixture' ./internal/modules/organization/adapters/            # gps003-go-static.log
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/tracking/... ./internal/platform/apicontract/... ./cmd/api/...
GOTOOLCHAIN=go1.27.1 staticcheck -tags=integration ./internal/modules/tracking/... ./internal/modules/organization/adapters/...
go test -count=1 -race -json -run 'TestSharedTrackBatchVectors|TestSharedTrackingSessionOperationVectors' ./internal/modules/tracking/application/
union.sh: go test -tags=integration -race -count=1 -p 2 -json -timeout=120m ./internal/modules/organization/adapters ./internal/modules/tracking/... \
  ./internal/modules/trip/... ./internal/modules/orgsync/... ./internal/platform/... ./cmd/api/...          # throwaway MinIO f4-gps002-s3
go test -count=1 -timeout=45m ./...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
gradle.sh --continue :domain:test --tests 'com.anpfuel.domain.enterprise.*' :application:test \
  :data:testDebugUnitTest --tests 'com.anpfuel.data.enterprise.*' :app:testDebugUnitTest --tests 'com.anpfuel.app.ui.enterprise.*' \
  :app:lintDebug :data:lintDebug :app:assembleDebug
gradle.sh -PenterpriseDeviceValidation=true :app:assembleDebug :app:assembleDebugAndroidTest :data:assembleDebugAndroidTest
bash scripts/check-mobile.sh --static-only && bash scripts/tests/test-gate-selection.sh
make quick-verify
python3 docs/enterprise/tools/check_plan.py; git diff --cached --check
```

Results:

- Go static (`gps003-go-static.log`): gofmt clean on everything this branch
  touched; it flagged `internal/platform/apicontract/trip_mission_test.go`,
  which arrived unformatted with the F4 head `c7e407e6` (TRIP-022/023, one
  map-literal alignment); `make quick-verify` refuses it, so this commit applies
  `gofmt -w` to that one line (formatting only; the coordinator's head needs
  the same). build and vet (plain, integration, nativefixture) clean;
  staticcheck plain and integration clean.
- Go vectors (`gps003-go-vectors-1.jsonl`): session 41/41 and batch 24/24 rows.
- Race/integration union (`gps003-db-union.jsonl`, `.start` 11:36Z, `.end`
  12:46Z, exit 1): 32 packages (29 pass, 2 without tests), 1 199 top-level
  tests and 8 524 named subtests; the only failures are two tests of code this
  task does not touch, under a host load average of 37–59 from several agents:
  `TestFuelingIntakeConcurrentDuplicatesLeaveOneFueling` ("concurrent
  duplicate failed outside RETRY") and
  `TestSignedAttachmentSessionRefusesWrongRecordsAuthorityAndReuse` ("actual
  local TLS signed request failed"). Rerun alone on the quieter host, three
  times each, with the same MinIO (`gps003-union-rerun.jsonl`): 3/3 and 3/3
  pass. Every tracking, trip, policy, auditor-walk and matrix test passed in
  the union.
- Untagged `go test ./...` (`gps003-untagged-all.log`): exit 0, 231 packages
  ok, 85 without tests.
- OpenAPI (`gps003-vacuum.log`): 0 errors, 1 129 warnings, 682 informs (the
  merged F4 head's counts; this task changes no OpenAPI).
- Kotlin (`gps003-kotlin-suites-lint-1.log`, BUILD SUCCESSFUL 6 min 21 s):
  domain enterprise 71 suites / 4 372 tests, application 104 / 705, data
  enterprise 37 / 210, app enterprise 8 / 30, 0 failures, errors or skips;
  lint app and data 0 errors and no warning in tracking sources;
  `:app:assembleDebug` built.
- Enterprise validation build (`gps003-validation-build-2.log`) passed after
  the module fix above; APK hashes in `gps003-validation-apks.sha256`.
- Mobile guard static-only ok and its fixture tests 34/34
  (`gps003-guard-tests.log`); plan check 0 problems; `git diff --check` clean.
- `make quick-verify`: the first run (`gps003-quick-verify.log`) stopped at
  gofmt on the merged `trip_mission_test.go` above; after the one-line format
  the second run (`gps003-quick-verify-2.log`, selection full) passed:
  `quick-verify: ok`, 352 s (warm budget 300 s exceeded under the shared host
  load, measured not enforced); its gate-selection, deploy-agent (229/229),
  backend fast gate (gofmt, build, vet, unit tests, staticcheck, sqlc vet and
  drift, OpenAPI lint, secret scans), vulnerability scan and the mobile static
  guard all passed.
- MinIO `f4-gps002-s3` removed with `down -v` after the union and the reruns
  (`gps003-s3-down.log`).

## Native acceptance on `emulator-5588`: OWED

Prepared and partly executed, not completed. The source fixture
(`gps002-native-source.log`) composed the real runtime over an orgtest child
database of the owned PostGIS with the throwaway MinIO `f4-gps002-s3`, recorded
the policy `EXPECTED` with its notice in force and wrote the private fixture;
the driver (`gps002-native-driver.out`, `gps002-native-device.log`, under
`flock /data/dev/worktrees/.f4-emulator.lock`) installed the data test APK
but could not place the fixture: the emulator had been rebooted at about
11:07Z by another agent after a host memory stall (see
`f4-logs/EMULATOR_5588_LOCKED_AFTER_REBOOT.md`) and user 0 is
`RUNNING_LOCKED` — a synthetic lock-screen PIN is set, so credential-encrypted
storage (every app's data directory) is unavailable until the credential is
entered. The coordinator ruled that nobody may enter that credential, wipe or
replace the emulator, or use the physical phone, so the acceptance is
recorded OWED. The PIN is not in the repository and was neither guessed nor
entered. The fixture was stopped through its own stop file (its child
database dropped by its cleanup). The source fixture, both device test
classes, the driver and the APK pair are ready (`gps003-validation-apks.sha256`;
rebuild with `-PenterpriseDeviceValidation=true :app:assembleDebug
:app:assembleDebugAndroidTest :data:assembleDebugAndroidTest` if the tree
changed). One-line rerun once the emulator is unlocked by its owner:

```sh
(bash /data/dev/worktrees/f4-logs/gps002-scripts/native-source.sh > /data/dev/worktrees/f4-logs/gps002-native-source.log 2>&1 &) && flock /data/dev/worktrees/.f4-emulator.lock bash /data/dev/worktrees/f4-logs/gps002-scripts/native-driver.sh
```

The driver sends a simulated route with `adb emu geo fix`, runs the two data
stages around a force-stop (policy read, trip start, explicit tracking start,
recording, stop from the notification path, resume, process death, recovery,
finish, every operation and both batches accepted), touches the fixture's
stop file so the server asserts the stored transitions
`NOT_STARTED, RECORDING, INTERRUPTED/STOPPED_BY_DRIVER, RECORDING,
INTERRUPTED/PROCESS_STOPPED, ENDED/TRIP_COMPLETED`, at least two segments with
every received fix stored once inside the session and consecutive sequences,
and then runs the app service device test (notification, one-time grant,
one listener, stop action). It needs the throwaway MinIO
`f4-gps002-s3` up (`down -v` afterwards).

## Retained obligations

- Native acceptance on `emulator-5588` (blocked above) for both EMP-GPS-002
  and EMP-GPS-003; real-device acceptance OWED (EDR-271).
- A server intake for device gap reasons (the batch format carries only fixes
  and counters); recording the battery economy on the session.
- EMP-GPS-010 (interruption matrix on devices, long-stationary guard),
  EMP-GOV-006 (battery and device matrix), EMP-GPS-015 (transparency screens).
- `internal/platform/apicontract/trip_mission_test.go` from the merged F4 head
  is not gofmt-clean with the go1.27.4 toolchain (not touched here).
