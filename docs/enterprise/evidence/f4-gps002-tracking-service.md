# EMP-GPS-002 — Organization trip tracking service: explicit start from the trip screen, visible notification, location-guard update

Date: 2026-11-16.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 task in worktree branch `codex/enterprise-f4-gps002`, built on
the F4 head `1a566b45` (main `b0ab7d33` + TRIP-009/013, GPS-001, OFFL-005,
TRIP-011, TRIP-007, TRIP-014/018, GPS-004/012, UGC safety, CHECK-002, tracking
policy, GPS-005; compileSdk/targetSdk 36). Under the owner decision EDR-271
(tracking implemented for real; active per organization only with its
employee notice in force; real-device acceptance OWED) the Android app gains
the organization trip tracking service. The coordinator integrates it and owns
PROGRESS, the F4 ledger, pushes and the PR. Synthetic data only; the physical
phone was never used.

Rules read: the EMP-GPS-002/003 blocks of
[EP07](../backlog/ep07-gps-navigation.md); the whole
[tracking specification](../modules/gps-navigation.md) (BR-GPS-001..015,
BR-GPS-034, §2.5 existing constraints — the guard is to be updated
deliberately in the same change —, §6.1, §8.1, §8.2 interruption matrix,
§10.2, §10.10, SCR-GPS-01..03, acceptance 1..5); 12-ux-app (offline-first,
"Salvo no aparelho" until a receipt); the
[GPS-001](f4-gps001-tracking-session-domain.md),
[GPS-004](f4-gps004-track-intake.md) and
[tracking policy](f4-gps-policy-command.md) evidence and their retained
lists; [TRIP-005](f4-trip005-app-trip.md) (trip book, producers, native
acceptance), [TRIP-011](f4-trip011-pause-resume-cancel.md) (Android producer
of pause/resume/cancel retained to EMP-TRIP-021),
[TRIP-012](f4-trip012-driver-handover.md) (EDR-256: the old owner's device
ends its session; the handover feed is EMP-TRIP-021),
[FLEET-013](f4-fleet013-my-assets.md) and
[OFFL-005](f4-offl005-attachment-upload.md) (protected store patterns);
`scripts/check-mobile.sh` section 5 and its fixture tests; the existing
location services (`GuidanceService`, `TripRecordingService`,
`DriverGpsService`, `DriverGpsLifecycle`). Precedents copied: EMP-TRIP-005
(producer + sealed book + Kotlin vectors + native fixture), EMP-CHECK-002
(company-plane gateway, strings table, rollback flag).

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-495..500, BUC-EMP-252..254, EDR-321..324. All
are used.

Rules B-BR-EMP-495 (the driver sees the policy in force), B-BR-EMP-496
(recording starts only from the visible trip screen), B-BR-EMP-497 (every
transition is a signed queued operation), B-BR-EMP-498 (one listener, safe
stop, bound owner), B-BR-EMP-499 (the session follows the trip and the
device, never silently), B-BR-EMP-500 (never personal or foreign; always
visible) and the use cases BUC-EMP-252..254 are in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-GPS-002 — Organization trip tracking service …", written before any
behaviour. Decisions in [16-decisions](../16-decisions.md), taken unattended
as the recommended routine choices:

- EDR-321: a driver-level read `POST /v1/orgs/{org_id}/tracking/policy/mine`
  under `trip.trip.start` over the member's own record (drivers lack
  `org.settings.read`; the context read would need a new table grant in every
  context read); cached in the sealed book; `OFF`/never read hide tracking.
- EDR-322: Kotlin `TrackingOperations` + shared oracle
  `tracking/session-operations-v1.psv` (Go `ParseSession` consumer); sealed
  `tracking-book` v1 replayed through the GPS-001 domain (mirror of the Go
  `Rebuild`/`Decide`); dependency chain (first start on `trip.start`, each
  later operation on the previous one, the end also on `trip.finish` at the
  finish's own instant); the device follows the facts it knows (finish,
  refused start, another responsible member); handover feed retained.
- EDR-323: `OrganizationTrackingService` (non-exported, `location`,
  `START_NOT_STICKY`, no boot receiver, one pinned subscription, 5 s
  request), non-Hilt with a process host, one-time visible start grant (30 s),
  notification with stop action, 30-second watchdog; the guard change.
- EDR-324: checkpoint (boot identity + instant) and recovery at the next
  trip-screen opening (`DEVICE_RESTARTED`/`PROCESS_STOPPED` at the
  checkpoint); fixes only counted in this task.

## Shape frozen before behaviour

- Backend (Go, `tracking`): `PolicyOwn` action (`trip.trip.start`,
  `access.Read`), `PolicyInForce`, `PolicyService.Mine`,
  `PolicyRepository.ReadPolicyInForce`, `PolicyStore.ReadPolicyInForce`,
  `PolicyMinePath` with the own-record resolver, `PolicyRecords.Check`
  admitting the own read only over the requester's own record,
  `TrackingPolicyAuthority` re-proof. Shared files:
  `authorization-routes-v1.json` (one route), `cmd/api` inventory (normal
  routes 160 → 161) and router list, AUTH-011 auditor walk (161 / 33 / 128),
  OpenAPI (path + `EnterpriseTrackingPolicyInForce`), `apicontract`
  tracking test. Authorization matrix unchanged (300 cells, 121/179).
  Go consumer `tracking/application/session_operations_vectors_test.go`.
- Domain (Kotlin `enterprise.tracking`): `TrackingOperations` (builders
  `start`, `resume`, `event`, decoder `read`, `TrackingEnvelope`,
  `TrackingOperationFact`, `TrackingOperationException`), `TrackingBook`
  (`TrackingPolicyRead`, `StoredTrackingTransition`, `TrackingCheckpoint`,
  `LocalTrackingSession`, `TrackingLocalTrip`), `TrackingDecisions`
  (`rebuild`, `decide`, `record`, `facts`, `tripState`).
- Application (`enterprise.tracking`): `EnterpriseTrackingSnapshot`,
  `EnterpriseTrackingStore`, `EnterpriseTrackingPolicyGateway`,
  `TrackingLocalFailure`/`Exception`, `EnterpriseTrackingProducer(s)` (six
  `gps.session.*` types), views (`TrackingScreen`, `TrackingSessionView`,
  `TrackingRecordView`, `TrackingCollecting`), `EnterpriseTrackingService`
  (`screen`, `refreshPolicy`, `start`, `stop`, `interrupt`, `reduce`,
  `follow`, `recover`, `checkpoint`, `collecting`); `TrackingCollector`
  (`begin`, `release`, `stopByDriver`, `tick`), `TrackingPlatform`,
  `TrackingStartGate`, `TrackingRuntime`; `TripFlow` gains `TripTracking`
  hooks (`startTracking`, `stopTracking`, `refreshTrackingPolicy`, recovery at
  `open`, follow at `finish`) and `TripViewState.tracking/trackingRefusal/
  trackingPolicyFailed`.
- Data: `ProtectedTrackingStore` (sealed record, no plaintext table),
  `EnterpriseTrackingPolicyHttpGateway`, DI (registry + store + gateway +
  service + runtime; `TripFlow` hooks).
- App: `OrganizationTrackingService` (+ `AndroidTrackingPlatform`,
  `OrganizationTrackingNotifications`, `OrganizationTrackingHost`,
  `TrackingServiceLauncher`), `TrackingCard` on the open trip,
  `TripViewModel.startTracking/stopTracking/refreshTrackingPolicy`, manifest
  entry, `ENTERPRISE_TRACKING_UI_ENABLED` rollback flag, 34 strings × 8
  locales (`enterprise_tracking.xml`, generated by
  `f4-logs/gps002-scripts/gen_tracking_strings.py`), application host
  installation in `AnpFuelApplication`.
- Guard: `scripts/check-mobile.sh` section 5 and
  `scripts/tests/test-gate-selection.sh`.

The shared oracle `contracts/testdata/enterprise/tracking/session-operations-v1.psv`
has 41 rows (START 9 OK / 8 MALFORMED / 1 PROTOCOL, RESUME 4/1, SUSPEND 2/1,
INTERRUPT 7/2, REDUCE 1/1, END 2/1/1 PROTOCOL), SHA-256
`e0ab1391496c811440e107e2f89fc5035798b566d8b52dc67c78b985c1181416`,
generated by the independent Python oracle
`f4-logs/gps002-scripts/gen_session_operation_vectors.py`.

## RED

- Go route (the forked sub-agent wrote the Go part against stubs): unit
  `gps002-go-unit-red.jsonl` 2/21 top-level failing (stub policy empty, stub
  resolver unavailable); contract `gps002-go-contract-red.jsonl` 41/128
  failing (the empty stub policy breaks router composition); PostGIS
  `gps002-go-db-red.jsonl` 1/1 failing (driver read 503).
- Go session vectors (`gps002-go-vectors-1.jsonl`): 41/41 rows passed on
  first run against the already existing `ParseSession` (EMP-GPS-004); this
  pins the oracle against the server and is not claimed as RED.
- Kotlin domain (`gps002-kotlin-domain-red.log`, XML
  `gps002-kotlin-domain-red-xml/`): `TrackingBookTest` 6/6 failing and
  `TrackingSessionOperationVectorsTest` 42/43 failing on `NotImplementedError`
  (the corpus-completeness test passes against the frozen corpus and is not
  claimed as RED).
- Application (`gps002-kotlin-application-red.log`, XML
  `gps002-kotlin-application-red-xml/`): `EnterpriseTrackingServiceTest`
  12/12 failing on the stubbed service, producers and collector.
- Data (`gps002-kotlin-data-red.log`, XML `gps002-kotlin-data-red-xml/`):
  `ProtectedTrackingStoreTest` 3/3 and
  `EnterpriseTrackingPolicyHttpGatewayTest` 2/2 failing.
- App (`gps002-kotlin-app-red.log`, XML `gps002-kotlin-app-red-xml/`): the two
  new `TripTrackingViewModelTest` tests failing on the stubbed ViewModel
  actions (the four existing `TripViewModelTest` tests pass).
- Guard (`gps002-guard-red.log`): 3 failures — the repo tree is red (the new
  subscription is outside the allow-list), the acceptance case is red, and a
  declared service without its pinned source was not refused; the other new
  refusal cases passed only because the whole guard was red and are not
  claimed.

## GREEN and REFACTOR

- Go: unit `gps002-go-unit-green.jsonl` (`tracking/...` race, 41/41
  top-level, 443 subtests), contract `gps002-go-contract-green.jsonl`
  (128/128, 877 subtests), PostGIS `gps002-go-db-green.jsonl`
  (`TrackingPolicy|Auditor|AuthorizationMatrix`, `-race -p 2`; the auditor
  walk needs storage and passed separately with a throwaway MinIO
  `f4-gps002-s3`, `gps002-go-db-auditor.jsonl`, 128 subtests, `down -v`).
- Kotlin domain (`gps002-kotlin-domain-green-2.log`): tracking 413/413. The
  first GREEN run (`-green-1.log`) failed one assertion of my own test, which
  counted two precise recording periods for start → stop → resume → reduce →
  interrupt → resume → end; the GPS-001 domain (oracle-proven) correctly
  yields three precise and one approximate period, and the test now asserts
  the full period list.
- Application (`gps002-kotlin-application-green-1.log`): 12/12.
- Data (`gps002-kotlin-data-green-2.log`): 5/5. The first GREEN run failed
  the restart test because the test's second "process" reused the first
  process's synthetic operation identifiers (`EnterpriseOperationReused`, the
  queue correctly refusing a reused identifier with another body); each
  simulated process now draws from its own synthetic range, as production
  draws random identifiers.
- App (`gps002-kotlin-app-green-1.log`): 6/6.
- Guard (`gps002-guard-green.log`): `test-gate-selection.sh` 34/34; the
  existing skeleton case now carries the tracking sources because the real
  manifest declares the service.

Named negatives mapped to tests:

| Negative | Tests |
|---|---|
| Hidden start (boot, worker, push, notification, another process, expired or foreign grant) | `the collector starts only from a fresh grant for its session and holds one listener` (no grant, other session, expired, other organization → `NO_GRANT`, no listener); domain `the domain refuses what the device must never store` (BOOT/WORKER/PERSONAL origins); producer refusal of a hand-built `BOOT` resume; guard `tracking start without the visible grant refused`, `sticky tracking service refused`, `boot receiver refused`, `exported tracking service refused` |
| Duplicate listener | collector test (`ALREADY_RUNNING`, one attach); fake platform refuses a second concurrent attach; reduced precision replaces the listener (`a reduced precision keeps recording with one approximate listener`) |
| Foreign owner | watchdog `owner` case (another account: listener released, nothing written into the partition); `a start on the open trip …` (organization B reads nothing; operations sealed to A's membership and enrollment); data store test (another account denied); `refreshPolicy` with another membership denied |
| Collection after stop | collector test (a report after `release` is not counted); watchdog `driver`, `permission`, `notification`, `finish` cases release the listener; `stop, resume and finish …` (no collecting after the end) |
| Personal trip, policy off or unknown | `the policy is read online …` (`POLICY_UNKNOWN`, `POLICY_OFF`, nothing written); domain refusals (PERSONAL origin, policy OFF/null) |
| Permission denied / notification blocked hidden | `a denied permission and a hidden notification are recorded attempts …`; collector `PERMISSION_MISSING`, `NOTIFICATION_HIDDEN`; watchdog `PERMISSION_REVOKED`, `NOTIFICATION_BLOCKED` |
| Silent resume after process death | `recovery interrupts a recording session …` (running collector untouched; `PROCESS_STOPPED`/`DEVICE_RESTARTED` at the checkpoint; nothing resumes by itself); data `after a restart …` over a reopened protected generation; flow `the trip flow recovers at opening …` |
| Session continuing after the trip | `stop, resume and finish …` (end at the finish's instant, dependent on the finish, accepted in order); recovery of a trip finished while the process was dead; flow finish |
| Server refusal ignored | `a final refusal stops the session on the device …` (REJECTED FORBIDDEN → refused, no collection, `SESSION_REFUSED`) |
| Failed write | same test (`failSave`: no operation, no book) |
| Background permission | guard `background location permission refused` (repo static check also) |

## Validation (at this commit)

Commands: Go from `backend/` with toolchain go1.27.4
(`f4-logs/gps002-env.sh`); Gradle through
`f4-logs/gps002-scripts/gradle.sh` (JDK 17, `flock
/data/dev/worktrees/.f4-gradle.lock`, `--max-workers=2`).

```sh
gofmt -l ./internal/modules/tracking ./internal/modules/organization/adapters ./cmd ./internal/platform/apicontract   # empty
go build ./... && go vet ./... && go vet -tags=integration ./...                                                        # clean (fork, gps002-go-static.log)
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/tracking/...                                      # clean
go test -race -count=1 ./internal/modules/tracking/...                                                                  # pass
gradle.sh --continue :domain:test --tests 'com.anpfuel.domain.enterprise.*' :application:test \
  :data:testDebugUnitTest --tests 'com.anpfuel.data.enterprise.*' :app:testDebugUnitTest --tests 'com.anpfuel.app.ui.enterprise.*' \
  :app:lintDebug :data:lintDebug :app:assembleDebug                                                                     # gps002-kotlin-suites-lint-1.log
bash scripts/check-mobile.sh --static-only && bash scripts/tests/test-gate-selection.sh                                 # ok, 34/34
python3 docs/enterprise/tools/check_plan.py                                                                             # 348 tasks, 545 rules, 167 use cases, 145 screens, problems 0
git diff --cached --check
```

Results: Go static clean; Kotlin (`gps002-kotlin-suites-lint-1.log`, BUILD
SUCCESSFUL in 5 min 55 s): domain enterprise 69 suites / 4 342 tests,
application 103 suites / 699 tests, data enterprise 37 suites / 209 tests, app
enterprise 8 suites / 30 tests, 0 failures, errors or skips; `:app:lintDebug`
and `:data:lintDebug` 0 errors (two `ObsoleteSdkInt` warnings on the new
service were removed — minSdk 26 — and the app enterprise tests rerun,
`gps002-kotlin-app-green-2.log`); `:app:assembleDebug` built. Guard
static-only ok and its fixture tests 34/34; plan check 0 problems.

Follow-up recorded with EMP-GPS-003 (see
[its evidence](f4-gps003-sampling-buffer.md)): the enterprise validation
build of this commit's tree fails at the device-test Hilt graph (the policy
gateway and the launcher were provided by modules the device tests replace);
EMP-GPS-003 moves them to their own modules and the validation build passes;
the emulator acceptance is OWED (the emulator is locked behind a credential
nobody may enter; drivers and APKs are ready).

The cumulative gates that cover both tracking tasks run once on the merged
tree that contains this commit and EMP-GPS-003: the race/integration union with
`organization/adapters`, the untagged `go test ./...`, `make quick-verify`, the
enterprise validation build and the native acceptance on `emulator-5588`
(trip start → explicit tracking start with its notification → simulated route
→ stop/resume → process death → finish → session operations and batches
accepted, segments stored).

## Retained obligations

- EMP-TRIP-021: the Android producer of trip pause, resume and cancel and the
  device feed of handovers and supervisor closures. Until then the device
  session follows only the finish and a refused start; a server-side pause or
  closure is followed by the server (GPS-004 FOLLOW) and a later device
  operation is refused (`MALFORMED`) and shown as refused; the handover rule
  (another responsible member → `OWNER_CHANGED`, nothing collected) is
  host-proven with synthetic facts only.
- EDR-256 server side: an end cause for a handover and one open session per
  trip with at most one per owner (the old owner's session can only be
  interrupted today).
- "Every enterprise screen shows the same state" (BR-GPS-004): the trip
  screen and the ongoing notification show it; the other enterprise screens
  and "Meu rastreamento" (SCR-GPS-04) belong to EMP-GPS-015.
- BR-GPS-016 long-stationary guard (EMP-GPS-010), the battery and device
  matrix (EMP-GOV-006), a local notification after an interruption, Android 17
  location button/indicator behaviour.
- Production grants for the new read (runtime `SELECT` on
  `org_tracking_policies` already listed for GPS-004).
- Real-device acceptance stays OWED (EDR-271); the physical phone was never
  used. No M4/F4 acceptance is claimed.
