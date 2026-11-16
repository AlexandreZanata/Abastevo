# EMP-GPS-010 — Tracking interruptions: permission, approximate-only, reboot, process death

Date: 2026-11-16.
Status: LOCAL_DONE
Validation: PASS (host, build, guard and backend gates); native emulator acceptance OWED (see below)
Integration: INTEGRATION_PENDING

One bounded F4 task in worktree branch `codex/enterprise-f4-gps010`, built on
the F4 integration head `19e681a3` (EMP-GPS-002 and EMP-GPS-003 merged with
DOC-004/FUEL-011/EXP-004/TRIP-020/TRIP-019/TRIP-010/METER-005/006). Owner
decision EDR-271 (tracking implemented for real; real-device acceptance OWED).
The coordinator integrates it and owns PROGRESS, the F4 ledger, pushes and the
PR. Synthetic data only; the physical phone was never used; the emulator was
not touched (it is locked behind a credential nobody may enter).

Rules read: the EMP-GPS-010 block of [EP07](../backlog/ep07-gps-navigation.md)
and its neighbours; the [tracking specification](../modules/gps-navigation.md)
(BR-GPS-002/004/006/007/011/012/013/015/016/024/025, §6.1, §8.1, the §8.2
interruption matrix, §10.10, UC-GPS-01/03, SCR-GPS-01..03);
[11-privacy-security](../11-privacy-security.md) (BR-PRIV-006/008/016/018);
the [GPS-001](f4-gps001-tracking-session-domain.md),
[GPS-002](f4-gps002-tracking-service.md), [GPS-003](f4-gps003-sampling-buffer.md),
[GPS-004](f4-gps004-track-intake.md), [GPS-005](f4-gps005-track-processing.md),
[GPS-012](f4-gps012-location-access-audit.md) and
[tracking policy](f4-gps-policy-command.md) evidence and their retained lists
(GPS-002 retained this task's interruption matrix, the long-stationary guard
and a local notification after an interruption); EDR-271..279, EDR-298..302,
EDR-321..327; the GPS-002/003 code (`OrganizationTrackingService`,
`TrackingRuntime`, `TrackingStartGate`, `TrackingCollector`, the
`tracking-book` and `tracking-buffer` records, `TrackingCard`) and
`scripts/check-mobile.sh` section 5, whose refusals of background location,
boot receivers and sticky starts are unchanged.

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-544..547, BUC-EMP-279..280, EDR-354..356, all
used. Migration 000262 is not used: every cause this task records
(`PERMISSION_REVOKED`, `NOTIFICATION_BLOCKED`, `PROCESS_STOPPED`,
`DEVICE_RESTARTED`, `PRECISION_REDUCED`) is already accepted by the EMP-GPS-004
intake and the shared `session-operations-v1.psv` oracle, so no server change,
no v2 oracle and no route change.

Rules B-BR-EMP-544 (every interruption recorded with its cause and visible),
B-BR-EMP-545 (no interruption bridged), B-BR-EMP-546 (approximate fixes never
claim precision), B-BR-EMP-547 (resumption needs the visible tap and current
authority; the buffer survives) and the use cases BUC-EMP-279 (recover from an
interruption) and BUC-EMP-280 (track with approximate location) are in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-GPS-010 — Tracking interruptions …", written before behaviour. Decisions
in [16-decisions](../16-decisions.md), taken unattended as the recommended
routine choices:

- EDR-354: interruptions recorded as soon as observed (recovery at the next
  opening, plus a collector that consumed the visible grant but cannot listen,
  plus a service that cannot go to the foreground); cause order
  `DEVICE_RESTARTED` → `PERMISSION_REVOKED` → `NOTIFICATION_BLOCKED` →
  `PROCESS_STOPPED` (whose text becomes "stopped without your command"); a
  start intent with no grant writes nothing; the notification states open
  gaps; the local "tracking interrupted" notification stays retained.
- EDR-355: per-report precision bound, reduction at the report's own instant
  and before listening; batches sealed per recording run; an earlier run's
  gap closed at that run's end.
- EDR-356: resumption authority (policy re-read online at the tap, cached
  offline, `OFF`/never read refused also for an existing session, `BUSY` when
  another session is collected here), deliberately stricter than the intake;
  BR-GPS-016 is not among this task's rules.

## Shape frozen before behaviour

- Domain (Kotlin `enterprise.tracking`, new `TrackingInterruptions.kt`):
  `TrackingInterruption` and `TrackingInterruptions.of`, `observedCause`,
  `fixPrecision`, `enterRun` (stubs `TODO()`).
- Application: `TrackingLocalFailure.BUSY`; `TrackingDeviceObserver`;
  `TrackingSessionView.interruptions/coarse`;
  `EnterpriseTrackingService.start(…, collecting)`, `recover(…, device)`,
  `interruptUncollected` (stub), `record(…, permission)`;
  `TrackingRuntimeState.gap`; `TripTracking.device/collecting`;
  `TripFlow.startTracking(…, online)`.
- App: `TripViewModel.startTracking` passes the connection; the tracking card
  lists interruptions and approximate parts and states `BUSY`; the service
  states open gaps in its notification and interrupts a granted session it
  cannot run in the foreground; `TrackingDeviceObserver` provided by
  `EnterpriseTrackingPresentationModule`; 7 strings × 8 locales
  (`enterprise_tracking_interruptions.xml`) and the rewritten
  `enterprise_tracking_cause_process_stopped`, generated by
  `f4-logs/gps010-scripts/gen_interruption_strings.py`.
- Data DI: `TripTracking` gets the observed device and the collected session.
- Native: three interruption stages in
  `data/src/androidTest/.../EnterpriseTrackingNativeDeviceTest.kt` (the test
  platform now observes approximate-only grants and listens on the
  fused/network provider for them),
  `app/src/enterpriseDeviceTest/.../EnterpriseTrackingInterruptionDeviceTest.kt`,
  the driver and source scripts `f4-logs/gps010-scripts/native-{driver,source}.sh`;
  the Go native fixture (`integration && nativefixture` only) accepts the
  expected transitions through `EMP_GPS_NATIVE_WANT_TRANSITIONS`.

## RED

Run against the stubs (`gps010-kotlin-red.log`, XML `gps010-kotlin-red-xml/`):

- Domain `TrackingInterruptionsTest` 4/4 failing on `NotImplementedError`.
- Application `EnterpriseTrackingInterruptionTest` 8/10 failing (policy and
  `BUSY` refusals absent, no interruption written for a begin that cannot
  listen, recovery always `PROCESS_STOPPED`, no reduction before a coarse fix
  or before listening, interruptions not listed so the run split and the
  carried gap could not be checked). Two tests passed against the stubs and are
  not claimed as RED: `unapproved restart - recovery, a stale start intent and a
  grant for an interrupted session never resume collection` and `buffer loss -
  across every interruption …` pin behaviour that EMP-GPS-002/003 already
  guarantee (no silent resume; each recorded fix pending once or in one batch).
- The EMP-GPS-002 test `the collector refuses a session that is not recording
  and missing permission or notification` was updated before behaviour to the
  new rule (B-BR-EMP-544: a begin that consumed the visible grant but cannot
  listen now records the interruption with the observed cause, so each case
  resumes first) and failed against the stubs (1/12 of that class).
- App: the first RED run stopped at the data DI, whose positional
  `TripTracking(…) { boot }` no longer compiled against the new hooks (shape);
  the app `TripTrackingViewModelTest` RED was therefore taken in the first
  GREEN run, where the application and data changes were in but the ViewModel
  was not: 1/3 failing (`a tap with a connection asks for the policy in force
  first …`, the flow called with `online = false`), XML
  `gps010-kotlin-app-red-xml/`.

## GREEN and REFACTOR

- First GREEN run (`gps010-kotlin-green-1.log`): domain tracking 447/447
  (7 suites), application tracking 28/28 (`EnterpriseTrackingInterruptionTest`
  10/10, `EnterpriseTrackingServiceTest` 12/12, `EnterpriseTrackingBufferTest`
  6/6) on the first run; app 2/3 (the RED above).
- Then the ViewModel passes the connection; REFACTOR renamed a local that
  shadowed the collector's `accepted` counter (`closesGap`) and imported
  `TrackingGapReason`; `EnterpriseTrackingStringsTest` (two tests) pins the
  strings.

Named negatives mapped to tests:

| Negative | Tests |
|---|---|
| Unapproved restart | `unapproved restart - recovery, a stale start intent and a grant for an interrupted session never resume collection` (no grant → `NO_GRANT`, nothing written; fresh grant on an interrupted session → `NOT_RECORDING`, no listener; record ignored; later recovery resumes nothing; only the tap resumes); `unapproved restart - every tap re-checks the policy in force and the one-listener rule before writing` (`POLICY_OFF` for an interrupted session and for a retry of an attempts-only session, `BUSY`, nothing written); `unapproved restart - with a connection the trip screen reads the policy first and a failed read keeps the cache` (online read before the write; `OFF` read refuses; failed read keeps the cache and says so; `BUSY` from the flow); app `a tap with a connection asks for the policy in force first …`; existing GPS-002 `the collector starts only from a fresh grant …`, guard refusals of sticky/boot/background unchanged |
| Missing gap | `missing gap - a begin that cannot listen after the visible grant interrupts its session with the observed cause` (permission → `PERMISSION_REVOKED`, notification → `NOTIFICATION_BLOCKED`, platform refusal and another listening session → `PROCESS_STOPPED`, at the last proof, listed); `missing gap - recovery names the cause observed now …` (`PERMISSION_REVOKED`, `NOTIFICATION_BLOCKED`, `PROCESS_STOPPED` for a reduction found after a death with no reduction invented, `DEVICE_RESTARTED` first); `missing gap - a batch never spans an interruption even when the seal at the interruption failed` (two runs waiting together → one batch per run, each on its own opening); `missing gap - a gap left open by a dead run is closed at that run's end …`; domain `missing gap - every interruption opens an interval …`, `missing gap - a fix-level gap left open by an earlier run …`; updated GPS-002 collector test |
| Precision overclaim | `precision overclaim - an approximate report inside a precise period reduces first and is recorded coarse` (reduction at the report's instant, precise fix stays precise, coarse fix inside the approximate period, one reduction, no-permission report not recorded, batch flags `[0, COARSE]`, `coarse` kept after a precise resume); `precision overclaim - a collector that begins with only approximate permission reduces before it listens`; domain `precision overclaim - a fix never claims more precision …` |
| Buffer loss | `buffer loss - across every interruption each recorded fix stays pending once or leaves in exactly one batch` (driver stop, permission, notification, owner change with the other partition untouched, reduction, provider gap, process death, reboot, failed seal sealed later); the conservation check also closes the two missing-gap run tests and the reduction test |

## Validation (at this commit)

Commands: Gradle through `f4-logs/gps010-scripts/gradle.sh` (JDK 17, `flock
/data/dev/worktrees/.f4-gradle.lock`, `--offline --no-daemon --no-parallel
--max-workers=2`); Go from `backend/` with the go1.27.4 toolchain
(`f4-logs/gps010-env.sh`).

```sh
gradle.sh --continue :domain:test --tests 'com.anpfuel.domain.enterprise.*' :application:test :data:testDebugUnitTest \
  :app:testDebugUnitTest :app:lintDebug :data:lintDebug :app:assembleDebug                      # gps010-kotlin-suites-lint.log
gradle.sh -PenterpriseDeviceValidation=true :app:assembleDebug :app:assembleDebugAndroidTest :data:assembleDebugAndroidTest   # gps010-validation-build.log
bash scripts/check-mobile.sh --static-only                                                         # gps010-guard-static.log
gofmt -l ./internal/modules/organization/adapters/; go build ./...                                  # gps010-go-static.log
go vet ./internal/modules/organization/adapters/ && go vet -tags=integration ./internal/modules/organization/adapters/ \
  && go vet -tags='integration nativefixture' ./internal/modules/organization/adapters/
GOTOOLCHAIN=go1.27.1 staticcheck -tags='integration nativefixture' ./internal/modules/organization/adapters/
python3 docs/enterprise/tools/check_plan.py; git diff --cached --check
```

Results:

- Kotlin (`gps010-kotlin-suites-lint.log`, BUILD SUCCESSFUL in 8 min 59 s):
  domain enterprise 79 suites / 4 662 tests, application (whole module)
  110 / 749, data (whole module) 211 / 979 (4 skipped, pre-existing), app
  (whole module) 127 / 832, 0 failures or errors; `:app:lintDebug` and
  `:data:lintDebug` 0 errors, the only tracking-related warning being the
  EMP-GPS-003 `enterprise_tracking_batches` `PluralsCandidate` (not touched
  here); `:app:assembleDebug` built.
- Enterprise validation build (`gps010-validation-build.log`, BUILD SUCCESSFUL
  in 2 min 36 s): the debug app, its device-test APK (with
  `EnterpriseTrackingInterruptionDeviceTest`) and the data device-test APK
  (with the three interruption stages) compile; APK hashes in
  `gps010-validation-apks.sha256`.
- Mobile guard static-only ok (`gps010-guard-static.log`): the background
  location, boot receiver, sticky start, pinned subscription and visible
  grant checks are unchanged and pass; the guard and its fixture tests were
  not touched, so `scripts/tests/test-gate-selection.sh` was not rerun.
- Go (`gps010-go-static.log`): the only backend change is the test-only
  `orgsync_tracking_native_fixture_test.go` (build tags `integration &&
  nativefixture`, never compiled in the ordinary, integration or race
  builds); gofmt clean, `go build ./...`, vet plain, integration and
  `integration nativefixture`, and staticcheck with both tags clean. The
  race/integration union and the untagged `go test ./...` were not rerun: no
  compiled backend code changed (the fixture runs only in the native
  acceptance below).
- Plan check 0 problems; `git diff --cached --check` clean; staged diff
  reviewed for secrets and PII (synthetic identifiers, organization names and
  emulator route only).

## Native acceptance on `emulator-5588`: OWED

The emulator is `RUNNING_LOCKED` behind a synthetic lock-screen credential
since 2026-11-16 11:07Z (`f4-logs/EMULATOR_5588_LOCKED_AFTER_REBOOT.md`);
nobody may enter it, wipe it or start another emulator, and the physical
phone is never used, so nothing ran on a device. Written and compiled in the
validation build:

- `EnterpriseTrackingNativeDeviceTest` (data, real protected store, queue,
  producers, signer and HTTP clients against the EMP-GPS-002 Go fixture):
  `interruption1PreciseRecordingWithALocationOffGapThenTheProcessEnds`
  (precise recording; `location_mode 0` through the instrumentation shell
  opens `PROVIDER_DISABLED` at the watchdog check; the next fix closes it),
  `interruption2FineLocationRemovedIsAStopAndTheResumeIsApproximate` (after
  the driver force-stops the process and revokes fine location: recovery
  records `PROCESS_STOPPED`, no reduction invented, the tap resumes an
  approximate period, one coarse fix within 11 minutes),
  `interruption3NoPermissionIsARevocationNothingResumesAndEverythingIsAccepted`
  (after coarse is revoked too: `PERMISSION_REVOKED`, a tap without permission
  is a recorded attempt, a fresh grant listens to nothing, finish, every
  operation and batch accepted, coarse flags exactly on the approximate run,
  no protected value outside encryption).
- `EnterpriseTrackingInterruptionDeviceTest` (app, the shipped service with an
  in-memory protected queue): the ongoing notification says "Localização
  desligada …" within one watchdog period of `location_mode 0` and returns to
  normal with the next fix; a session granted while the service listens for
  another one is interrupted at once (`PROCESS_STOPPED`) and the first keeps
  recording.
- One-line rerun once the owner unlocks the emulator (needs a throwaway MinIO
  `f4-gps010-s3`, `down -v` afterwards; rebuild the validation APKs if the
  tree changed):

```sh
(bash /data/dev/worktrees/f4-logs/gps010-scripts/native-source.sh > /data/dev/worktrees/f4-logs/gps010-native-source.log 2>&1 &) && flock /data/dev/worktrees/.f4-emulator.lock bash /data/dev/worktrees/f4-logs/gps010-scripts/native-driver.sh
```

The source sets `EMP_GPS_NATIVE_WANT_TRANSITIONS` so the fixture expects
`NOT_STARTED, RECORDING, INTERRUPTED/PROCESS_STOPPED, RECORDING,
INTERRUPTED/PERMISSION_REVOKED, INTERRUPTED/PERMISSION_DENIED,
ENDED/TRIP_COMPLETED` with its unchanged storage checks (each received fix
stored once, none outside the session, consecutive sequences, every operation
accepted). A device reboot is not driven (it would restart the shared
emulator); `DEVICE_RESTARTED` stays host-proven and owed on a real device.

## Retained obligations

- Native acceptance on `emulator-5588` (blocked above) and real-device
  acceptance (EDR-271), including a real reboot and manufacturer process
  kills (§8.2 `UNVERIFIED` rows, EMP-GOV-006).
- A local "tracking interrupted" notification after a process death (needs
  the periodic worker; its timing after a restart is EMP-GOV-006's) and in a
  live process (EDR-354).
- BR-GPS-016 long-stationary guard and the device-side `LONG_STATIONARY`
  suspension (not among this task's rules).
- A server-side policy check at resume (the intake checks the policy only
  when a session opens, EDR-275; the device is stricter, EDR-356) and a
  device read of a policy withdrawal that is not `OFF` (403 from the read is
  generic `UNAVAILABLE` on the account plane, so a failed read keeps the
  cache).
- EMP-GPS-002/003 retained items still open: EMP-TRIP-021 feeds, EDR-256
  server handover end cause, a device gap intake on the server, battery
  economy on the session.
- "Every enterprise screen shows the same state" and "Meu rastreamento"
  belong to EMP-GPS-015 (next commit of this branch).
- No M4/F4 acceptance is claimed.
