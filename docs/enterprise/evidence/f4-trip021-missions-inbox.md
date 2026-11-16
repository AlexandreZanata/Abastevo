# EMP-TRIP-021 — App: missions inbox and planned-trip execution

Date: 2026-11-16.
Status: LOCAL_DONE
Validation: PASS (host-side; native acceptance OWED, emulator locked)
Integration: INTEGRATION_PENDING

Bounded F4 Android task (module TRIP, P1, type APP, size M) on
`codex/enterprise-f4-trip021` (worktree `/data/dev/worktrees/f4-trip021`),
created from the F4 head `b2dfc674` and, on the coordinator's request before
any behaviour, merged with EMP-TRIP-019 (`c68f3537`, merge commit
`a690216d`). Real dependencies used: the EMP-TRIP-008 own list
(`missions/mine/list`) and signed `trip.mission.respond` owner, the
EMP-TRIP-009 plans, the EMP-TRIP-011 lifecycle owners, the EMP-TRIP-023
mission soft reference of `trip.start`, the EMP-TRIP-019 departure-run
reference, the EMP-TRIP-005 trip producer, the FLEET-013 picker and
reference cache, the OFFL-003 protected queue and the PRIV-003 sealed
generation, the EMP-CHECK-002 cookie-plane precedent. Synthetic data only. No
server production code, SQL, migration, route, permission or OpenAPI change:
migration 000259 is not used. The only Android device used is the owned
emulator `emulator-5588` (AVD `enterprise-f2-api36`); the physical phone was
never addressed.

Sources read: the EMP-TRIP-021 block and the TRIP-008/009/011/019/022/023
blocks ([EP06](../backlog/ep06-trips.md)); [10-offline-sync](../10-offline-sync.md)
BR-OFFL-003/016/017, §10.2 (the mission-response paragraph) and the §10.3
matrix rows "My assets, assignments, missions", "Accept or decline a mission",
"Start trip", "Logbook events, stops, pause, resume", "Finish trip";
[05-journeys](../05-journeys.md) JRN-06; REQ-07.2 in
[17-traceability](../17-traceability.md); the evidence of TRIP-005, TRIP-008,
TRIP-009, TRIP-011, TRIP-019, TRIP-022, TRIP-023, FLEET-013 and CHECK-002 and
their code (Go `ParseRespond`, `ParseTransition`, `ParseStart`, the respond
owner and its transport denials, the Kotlin trip book, producers, flow and
screen, the queue dispatch, the HTTP operation client, `CompanyAccessClient`);
EDR-202, 224..229, 242..245, 252..253, 265, 283, 331.

IDs used: B-BR-EMP-527..531, BUC-EMP-270..272, EDR-342..344 — all of them.
Migration 000259 was not needed and is not used.

## Observed before any change

- No Android consumer of missions existed. The own list
  (`POST /v1/orgs/{org_id}/missions/mine/list`, company cookie, `trip.trip.start`
  on the membership itself) returns items with plans but no server
  confirmation instant.
- `trip.mission.respond` answers a stale, reassigned or foreign response with
  a whole-request transport denial (`FORBIDDEN`/`NOT_FOUND`/`CONFLICT`) that
  stores nothing (EDR-229). On the device a non-200 batch answer is an
  `EnterpriseQueueDeliveryFailure` that retains every operation of the batch
  for retry: a denied answer batched with trip records would hold them
  forever. TRIP-008 retained "the device handling of a whole-batch denial" to
  this task.
- The protected queue has no ordering between independent operations and
  `trip.start` admits no dependency (EDR-202); a start naming a mission whose
  acceptance is only saved on the device could reach the server first and be
  held `ORPHAN_REFERENCE`.
- TRIP-011 retained the Android `trip.pause`/`trip.resume`/`trip.cancel`
  producer, the trip-book states and revision; TRIP-023 retained the device
  mission reference. TRIP-019 (merged first) made `trip-operations-v2.psv` the
  consumed oracle with the `checklist_run` reference and asserted that a start
  carrying `checklist_run` + `mission` stays unsupported on the device, while
  the Go `ParseStart` admits one of each.
- `CompanyAccessClient` maps a 403 to the safe `UNAVAILABLE`.

## Rules frozen before behaviour

B-BR-EMP-527..531 and BUC-EMP-270..272 are in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-021 — App: missions inbox and planned-trip execution (2026-11-16)";
EDR-342..344 in [16-decisions](../16-decisions.md). Both were appended before
any code; after the TRIP-019 merge the oracle name moved from v2 to v3 in the
rule texts (no rule changed). The recommended routine decisions, taken
unattended:

- **EDR-342 — inbox source and copy.** The existing own list on the cookie
  plane (one page of 100, strict, refused whole for another assignee), kept as
  one sealed `mission-inbox` v1 `RECORD` per partition bound to membership,
  enrollment and employee record, with the device receipt clock; the age uses
  the FLEET-013 rule (elapsed time within one boot, wall time after a reboot,
  24 h, unknown when negative). No server change.
- **EDR-343 — answers and delivery.** A `trip.mission.respond` producer indexes
  the answer in the copy in the queue's own write. `EnterpriseQueueProducerDescriptor`
  gains `alone` (default false; only this producer): dispatch sends every due
  ordinary operation first and an `alone` one only as the single operation of
  its batch, so a denial never holds another record. A denied answer stays
  saved and retried (no device-made receipt, no queue format change) and is
  explained from the copy after a refresh. "Iniciar viagem da missão" only after
  a confirmed acceptance.
- **EDR-344 — execution.** `trip.start` carries the optional mission
  reference (beside the EMP-TRIP-019 departure run), the device builds the
  EMP-TRIP-011 lifecycle commands at the local revision (1 plus the trip's
  earlier commands), the oracle becomes `trip-operations-v3.psv` (every v2 row
  byte for byte plus 33 rows), the trip index `trip-book` v2 (reads v1), the
  inbox a "Missões" section of the trip panel behind `ENTERPRISE_MISSION_UI_ENABLED`.

## Shape

- Domain (`domain/.../enterprise/trip/`): `TripOperations` (`start(..., missionId)`,
  `lifecycle`, `LIFECYCLE`, `TYPES` +3, `TripOperationFact.Start.missionId`,
  `TripOperationFact.Lifecycle`, decoder rules mirroring `ParseStart`/`ParseTransition`);
  `TripMissionOperations` (`respond`, `read` mirroring `ParseRespond`);
  `TripBook` v2 (`TripLifecycleEntry`, `LocalTrip.missionId/lifecycle/nextRevision/open`,
  states `PAUSED`/`CANCELLED`, `withPause/withResume/withCancel`, `missionTrip`,
  failures `NOT_RUNNING`, `NOT_PAUSED`, `MISSION_ALREADY_STARTED`, v1 decode);
  `TripPresentation` (lifecycle records, `TripView.missionId`);
  `TripMissionInbox.kt` (`MissionInboxItem`, `MissionPlanView`, `MissionInboxPage`,
  `MissionResponseLine`, `MissionInbox` codec and rules, `MissionInboxAge`,
  `MissionAnswerState`, `MissionAction`, `MissionView`, `MissionInboxView`,
  `MissionInboxPresentation` — the offline eligibility matrix).
- Application: `queue/EnterpriseOperationQueue.kt` (`alone`, dispatch order);
  `trip/Missions.kt` (`EnterpriseMissionStore`, `EnterpriseMissionGateway`,
  `EnterpriseMissionProducer`, `EnterpriseMissionService` — `inbox`, `refresh`,
  `respond`); `trip/Trips.kt` (lifecycle producers, `start(..., missionId)` with
  the inbox's own matrix, `lifecycle`, `TripFlow.refreshMissions/respond/startMission/lifecycle`,
  `TripViewState.missions/missionsUnavailable/missionRefusal/refreshFailed/refreshed`).
- Data: `trip/EnterpriseMissionHttpGateway.kt` (cookie plane, strict, binding),
  `trip/ProtectedMissionStore.kt` (sealed record, CAS update, entries of the
  answers), DI (`EnterpriseModule`: mission store and service, registry adds the
  respond producer; `EnterpriseChecklistModule`: the gateway).
- App: `trip/MissionInbox.kt` (inbox section, mission detail, reason form),
  `TripScreen.kt` (modes, planned start through the picker restricted to the
  mission's vehicle, pause/resume/cancel, lifecycle records in the timeline,
  mission line), `TripViewModel` (offline actions; refresh online only),
  `EnterpriseContextScreen` wiring, build flag `ENTERPRISE_MISSION_UI_ENABLED`,
  71 strings × 8 locales (`enterprise_missions.xml`, generated by
  `f4-logs/trip021-scripts/gen_mission_strings.py`). Existing container tokens
  only.
- Oracle `contracts/testdata/enterprise/trip/trip-operations-v3.psv` (94 rows:
  START 34 — OK 13, MALFORMED 18, PROTOCOL 3; EVENT 17; FINISH 11; SUBMIT 4;
  PAUSE 8; RESUME 3; CANCEL 6; RESPOND 11; SHA-256
  `5f4acc4b31c82580c9484a29e13baef92950d5ccfa3023cb58ba39d7eb629f54`), generated by
  `f4-logs/trip021-scripts/gen_trip_operation_vectors_v3.py` (the TRIP-019
  oracle extended; it regenerates v2 byte for byte, and v2 is the first 61 rows
  of v3), documented in the trip testdata README. Go consumer
  `trip/application/operations_vectors_test.go`, Kotlin consumer
  `EnterpriseTripOperationVectorsTest`.
- Native: `organization/adapters/orgsync_mission_native_fixture_test.go`
  (tags `integration nativefixture`), `data/src/androidTest/.../EnterpriseMissionNativeDeviceTest.kt`,
  `app/src/enterpriseDeviceTest/.../EnterpriseMissionPresentationDeviceTest.kt`.

## RED

Logs in `/data/dev/worktrees/f4-logs/trip021-*`. Shape first (Kotlin `TODO()`
stubs, the frozen oracle and tests).

- Go consumer of the oracle (`trip021-go-vectors-1.jsonl` on the pre-merge
  draft, `trip021-go-vectors-v3.jsonl` on the merged tree): 1 top-level / 94
  subtests pass at once — it pins existing server parsers (`ParseStart`,
  `ParseTransition`, `ParseRespond`), so it has no behavioural RED (the
  TRIP-005 precedent); it proves the oracle is what the intake accepts.
- Kotlin domain RED, first on the pre-merge draft (`trip021-kotlin-domain-red.log`,
  XML kept): 110 tests, 48 failing. After the TRIP-019 merge the oracle became
  v3 and the RED was rerun on the merged tree (`trip021-kotlin-domain-red-2.log`,
  XML in `trip021-kotlin-domain-red-2-xml/`): 119 tests, 50 failing — vectors
  34/96 (the 33 new rows and the reference test whose superseded start
  expectation is named below), execution 6/6, inbox 9/9, trip-book codec 1/8
  (version 2); every failure `NotImplementedError` or the v1 encoding.
- Application and data RED (`trip021-kotlin-host-red.log`, XML kept):
  `EnterpriseMissionFlowTest` 7/7 failing (`NotImplementedError`; one NPE
  because the stub flow publishes no inbox), `EnterpriseMissionHttpGatewayTest`
  3/3, `ProtectedMissionStoreTest` 4/4; `EnterpriseTripFlowTest` 13/13 passing
  (regression, not RED).
- Not claimed as RED: `TripViewModelTest` (thin forwarding written with the
  methods), the presentation device tests and the native acceptance (written
  after the screens).

Expectations changed: one TRIP-005 codec assertion (an empty book encodes
`version` 2 and `version` 3 is refused instead of 1 and 2) and one TRIP-019
Kotlin assertion (a start naming a `checklist_run` and a `mission` is now
decoded with both, as the Go `ParseStart` already admits; on a finish it stays
`UNSUPPORTED`, and two references of one type stay `UNSUPPORTED`), both
superseded by EDR-344, not weakened.

## GREEN and REFACTOR

- Domain (`trip021-kotlin-domain-green-1.log`): the whole trip package, 1010
  tests, 0 failures on the first implementation run (vectors 96/96 including
  the 94 v3 rows; execution 6/6; inbox 9/9; trip-book codec 8/8).
- Host suites after the application and data implementation
  (`trip021-kotlin-host-green-1.log`, XML in `trip021-kotlin-host-green-1-xml/`):
  domain enterprise 4422, application enterprise 165, data enterprise 212, app
  enterprise 31 tests — 0 failures, errors or skips. The new classes passed on
  their first implementation run: `EnterpriseMissionFlowTest` 7/7,
  `EnterpriseMissionHttpGatewayTest` 3/3, `ProtectedMissionStoreTest` 4/4,
  `TripViewModelTest` 6/6; the existing `EnterpriseTripFlowTest` 13/13 and
  `EnterpriseOperationQueueTest` 15/15 are unchanged regressions.
- Found while building the validation APKs and fixed (no expectation changed):
  the first validation build (`trip021-validation-build-1.log`) failed Hilt
  because the mission gateway was provided from `EnterpriseChecklistModule`,
  which the enterprise validation graph replaces; the gateway now has its own
  `EnterpriseMissionModule` and the validation graph its own replacement
  (`EnterpriseMissionDeviceBindings.kt`, an `UNAVAILABLE` list like the
  checklist reads); `trip021-validation-build-2.log` passes. The data and app
  enterprise host suites and `:app:assembleDebug` were rerun after this change
  (`trip021-kotlin-host-final.log`).
- REFACTOR: the start eligibility of a planned trip is the inbox's own pure
  matrix (`MissionInboxPresentation`), reused by the trip service instead of a
  second rule; the presentation reads lifecycle records through the same
  owner check as every other record.

## Named negatives and acceptance → tests

| Negative / acceptance | Test(s) |
|---|---|
| AC: cached assignments show their age | domain `BR-OFFL-016 the age comes from the device receipt clock…` (elapsed within one boot, wall after a reboot, unknown when negative, labelled at 24 h); flow `B-BR-EMP-527 refresh is online…` (age grows 2 h offline, labelled after 24 h); presentation device test `theCopyAlwaysShowsItsAgeAndRefreshingIsOnlineOnly` (written, native run OWED) |
| AC: eligible actions follow the approved offline matrix | domain `the approved offline matrix decides each mission's eligible actions` (ASSIGNED → accept/decline, ACCEPTED → planned start, other states nothing), `a planned trip on this device is opened, never started twice…`; flow tests for answers, planned start and pause/resume/cancel offline; ViewModel test (offline actions forwarded, refresh only online) |
| Foreign mission (another member's) | domain `B-BR-EMP-527 … bound to its employee record` (a page with another assignee refused whole; another employee record foreign); gateway `a malformed page is refused whole and another member's mission is a denial` (ACCESS_DENIED, nothing returned; no assignee also denied); flow `a page with another assignee's mission is refused whole and nothing of it is kept`; store `the producer refuses an answer the copy does not allow` |
| Foreign mission (another organization's) | flow `BR-OFFL-003 …` (B's flow sees no copy, its answer naming A's mission is `UNKNOWN_MISSION`, nothing queued in B); store `… another organization of the same account reads nothing`, `a foreign or unreadable copy is never replaced`; gateway `another account … sends nothing`; server side the EMP-TRIP-008 PostGIS proofs (another organization's mission `NOT_FOUND`, nothing stored) |
| Stale policy unlabelled | domain age test (≥ 24 h and unknown age are `stale`; a never-refreshed inbox is `stale` and not `refreshed`); flow (stale after 24 h); presentation device test (age line always present, stale and unknown notices) |
| Queued command retagged | flow `BR-OFFL-003 an answer queued in one organization is never shown, moved or retagged by another` (bytes unchanged, bound to A's organization and membership, delivered to A's intake only); producers bound by the queue's attempt check; native stage 3 (written, OWED) |
| Online-only action reported complete offline | flow `B-BR-EMP-527 …` (an offline refresh sets `refreshFailed`, never `refreshed`, the copy and age unchanged); ViewModel test (no refresh forwarded without a connection); presentation device test (refresh disabled with "precisa de conexão"); an answer saved offline is `SAVED` until its own `ACCEPTED` receipt (domain `B-BR-EMP-531 …`, flow `B-BR-EMP-528 …`) |
| A transport-denied answer holds nothing else (EDR-343) | flow `EDR-343 an answer goes alone after every other record…` (fake intake refuses any batch with a reassigned answer: every trip record delivered in the first batch, the answer only ever alone, retried, never acknowledged, shown `NOT_CONFIRMED_REMOVED` after a refresh); native fixture asserts every refused batch held exactly one answer (OWED) |
| A planned trip only from a confirmed acceptance, on its vehicle, once | flow `B-BR-EMP-529 …` (`NOT_CONFIRMED`, `WRONG_VEHICLE`, the start carries `{type: mission}`, `ALREADY_STARTED` after the finish); domain execution test (`MISSION_ALREADY_STARTED`, freed only by a refused start); vectors (`start-with-mission-reference[-and-departure-run]` accepted by `ParseStart`) |
| Pause, resume, cancel (EMP-TRIP-011 retained) | domain execution tests (local states, revision 1 + earlier commands, events and finish while paused, nothing after a cancellation, cancel frees the vehicle, v2 codec and v1 read); flow `B-BR-EMP-530 …` (revisions 1, 2, 3, reasons, cancel needs a reason, each record accepted with its own receipt); vectors PAUSE/RESUME/CANCEL rows through `ParseTransition` |
| Local save reported without durable persistence | flow `B-BR-EMP-528 …` (a failed protected write says nothing was saved and shows no answer); store test (no plaintext mission fact in the sealed files) |

## Validation

Environment: Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f4-go-build`,
`GOTMPDIR`/`TMPDIR=/data/dev/cache/f4-tmp`; Kotlin through the shared Gradle
flock with JDK 17, `--offline --no-daemon --no-parallel --max-workers=2` and
the comaps path (`f4-logs/trip021-scripts/gradle.sh`).

- Toolchain `gofmt -l` on trip and organization/adapters: empty. `go build
  ./...`, `go vet ./...`, `go vet -tags=integration ./...` clean
  (`trip021-go-build-vet.log`); `go vet -tags='integration nativefixture'` on
  organization/adapters clean.
- `GOTOOLCHAIN=go1.27.1 staticcheck` on trip/application and with
  `-tags='integration nativefixture'` on organization/adapters: no findings
  (`trip021-staticcheck.log`, empty).
- Go oracle consumer (`trip021-go-vectors-v3.jsonl`): 1 top-level / 94
  subtests pass; trip packages under race (`trip021-go-trip-race.jsonl`): 4
  packages, 1133 named tests, 0 failures.
- No SQL, migration, sqlc input, route, permission or OpenAPI changed, so
  `sqlc`, the route inventory, the authorization matrix, the AUTH-011 walk and
  the organization/adapters race union were not touched or rerun.
- Untagged whole module `GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...`
  (`trip021-untagged-all.log`): exit 0, 230 packages ok, 0 FAIL.
- Kotlin host suites: see GREEN (4422 / 165 / 212 / 31, 0 failures).
- `:app:assembleDebug :app:lintDebug :data:lintDebug`
  (`trip021-lint-normal-build-1.log`, reports `trip021-lint-app.txt`,
  `trip021-lint-data.txt`): 0 errors; 354 app and 17 data existing warnings,
  none in a touched file or in `enterprise_missions.xml`.
- Enterprise validation build `:app:assembleDebug :app:assembleDebugAndroidTest
  :data:assembleDebugAndroidTest -PenterpriseDeviceValidation=true`
  (`trip021-validation-build-2.log`): passes; APK SHA-256 in
  `trip021-validation-apks.sha256` (app `9cf3b29d…3e03`, app test
  `7f25c91e…9652`, data test `1f330cef…576d`).
- `python3 docs/enterprise/tools/check_plan.py`: 0 problems. `git diff --check`
  clean. Staged diff reviewed for secrets/PII: synthetic identifiers, labels,
  places and reasons only; the CPF-shaped strings are the existing invalid
  synthetic `123.456.789-09` used to prove refusal.

## Native acceptance on `emulator-5588`

**OWED.** The device stages were not run: `emulator-5588` was rebooted by
another agent after a system_server crash and is `RUNNING_LOCKED` behind a
synthetic lock-screen credential that nobody may enter
(`f4-logs/EMULATOR_5588_LOCKED_AFTER_REBOOT.md`); the coordinator ordered
host-side gates only and no wait. What exists and was exercised:

- The opt-in source `TestMissionNativeSourceFixture` (actual default
  enterprise runtime over an orgtest child database of the owned PostGIS,
  signed HTTPS with a disposable CA, two organizations, two planned missions
  created, planned and assigned through the real mission store, a second
  member for the reassignment) compiled, vetted, staticchecked and reached
  "ordinary two-organization signed mission source ready"
  (`trip021-native-source.log`); it was then stopped by this agent (its own
  process, verified by its environment) because the device could not run.
- The five-stage device class `EnterpriseMissionNativeDeviceTest` (data
  androidTest: refresh and age; offline answers with the online-only refresh
  failing; process death after the owner's reassignment; reconnection with one
  answer accepted and the reassigned one denied alone, the planned trip
  started, paused, resumed; the finish completing the mission) and the
  presentation class `EnterpriseMissionPresentationDeviceTest` (five methods,
  pt-BR) are built into the validation APKs above.
- One-line rerun once the emulator is unlocked by its owner:
  `bash /data/dev/worktrees/f4-logs/trip021-scripts/native-run.sh` (source in
  the background, the driver `trip021-scripts/native-driver.sh` and the app
  classes `trip021-scripts/app-device.sh EnterpriseMissionPresentationDeviceTest
  EnterpriseTripPresentationDeviceTest EnterpriseMyAssetsPresentationDeviceTest
  EnterpriseContextDeviceTest` under `flock /data/dev/worktrees/.f4-emulator.lock`,
  `emulator-5588` only).

## Retained (with reasons; not part of this task's acceptance)

- Native acceptance on the owned emulator (above): OWED, with its rerun
  command; a two-physical-device run is also OWED (never the phone).
- A device-final state for a transport-denied answer (withdrawing it from
  delivery): it stays saved and retried with the OFFL-003 backoff (cap 6 h)
  and is explained after a refresh; a final local state needs an OFFL-003
  queue-entry owner (EDR-343 alternative).
- Following the own list beyond one page of 100 missions (stated as "only the
  first 100"); a server confirmation instant on the own list (not needed for
  the age).
- Unit names and locations for `UNIT` destinations and plan display beyond
  text, reason and load (cost-centre names); notifications of assignment.
- The answer's server read (the decline reason and answer history) and the
  planner's view of device answers (EMP-TRIP-022 panel).
- A supervisor's or held-command resolution of `STALE_REVISION`/
  `TRIP_ALREADY_CLOSED` lifecycle commands (EMP-TRIP-007/014); handover on the
  device (EMP-TRIP-012); the current-activity card and driving layout.
- Retention/pruning of the missions copy and of answered lines (no owning
  policy, like OFFL-003).
- Rendered light/dark/200 % font review and a TalkBack pass on the inbox
  (`VISUAL_VALIDATION_PENDING`).
- Merged-tree verification, production grants (none new) and any
  staging/release. No milestone, phase or gate is claimed.

## Shared text edited on this branch

- `docs/enterprise/10-offline-sync.md`: §10.2 (responses are delivered alone,
  EDR-343) and §10.3 (the inbox, answers, planned start and lifecycle,
  EDR-342..344).
- `data/src/main/kotlin/com/anpfuel/data/enterprise/README.md`: the lifecycle
  and respond producers and the `mission-inbox` record.
- `contracts/testdata/enterprise/trip/README.md`: the v3 oracle section; v2
  marked superseded.
- The EDR rows were placed after EDR-334 (the last row); note that the
  EMP-TRIP-019 rows EDR-331..334 sit after a blank line that splits the table
  (left for the coordinator).

## Merged-tree validation (F4 head `19e681a3`)

The task commit `e2c47185` was merged with the F4 integration head `19e681a3`
(EXP-004, TRIP-020, TRIP-010, METER-005/006, GPS-002/003) on the
coordinator's request. Conflicts resolved as unions keeping every feature:
the contract and the EDR table (EDR-335..341 before EDR-342..344), the build
flags, the queue registry (expense, tracking and mission producers), the
`TripFlow` constructor (`receipts, related, tracking, missions`, then the
trailing delivery lambda), its state, `begin` and `publish` (closing, tracking
and inbox read side by side), the trip detail (`state`, `tracking`, `photos`,
`closing` and the lifecycle actions) and the company screen.

Two behaviours the merged features required, made in the merge commit and
tested:

- **Tracking follows the device lifecycle commands (EMP-GPS-002 × EMP-TRIP-021,
  BR-PRIV-006).** `TrackingLocalTrip` gains `PAUSED` and `CANCELLED`; the
  tracking `follow` suspends a recording session `TRIP_PAUSED` at a device
  pause and ends it `TRIP_CANCELLED` at a device cancellation, each at the
  command's own instant and depending on it (the server's FOLLOW applies the
  same causes); `TripFlow.lifecycle` calls it like the finish does; recovery
  follows a paused trip; tracking is not started while paused (the existing
  `TRIP_NOT_OPEN`). Test: `EnterpriseTrackingServiceTest` "a trip pause
  suspends and a cancellation ends the session at the command's own instant".
  Its first merged run failed (the producer's dependency rule admitted only
  the finish), the rule now admits the latest lifecycle command for those two
  causes; no expectation changed.
- **EMP-TRIP-010 planned stops in the own list.** `EnterpriseMissionPlan`
  gained an optional `stops` array; the strict gateway would have refused
  every page with stops. It now checks their exact shape (1..20 objects
  `{sequence, kind: TEXT, text}` in order) and does not keep them (the inbox
  shows the destination only; stops belong to EMP-TRIP-010). Test: the gateway
  test's stops case.

Merged-tree gates: Kotlin host suites domain enterprise 4706, application
enterprise 194, data enterprise 219, app enterprise 34 tests, 0 failures
(`trip021-merged-host-1.log`, `trip021-merged-application-2.log`);
`:app:assembleDebug` passes; Go `gofmt`, build, vet (plain, integration,
`integration nativefixture`) and staticcheck clean
(`trip021-merged-staticcheck.log`), the v3 oracle consumer passes, the trip
packages under race 1194 named tests, 0 failures
(`trip021-merged-go-trip-race.jsonl`); `:app:lintDebug :data:lintDebug`
0 errors, 355 app and 17 data existing warnings, none in a touched file
(`trip021-merged-lint.log`); the enterprise validation build passes
(`trip021-merged-validation-build.log`, APK hashes in
`trip021-merged-validation-apks.sha256`). The
native acceptance stays OWED (emulator locked); the native fixture still
compiles on the merged tree.
