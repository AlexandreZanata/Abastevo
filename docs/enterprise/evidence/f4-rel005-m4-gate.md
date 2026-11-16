# EMP-REL-005 — M4 gate: trips, logbook, sync and tracking fundamentals acceptance

Date: 2026-11-16.
Status: BLOCKED — gate run recorded; **M4 NOT ACCEPTED**
Validation: PARTIAL — source/host scenario union PASS on `1ee50c9e`; device evidence OWED; named scenario clauses not produced by any owner yet
Integration: INTEGRATION_PENDING

Bounded F4 TEST task (module REL, P1, size M; depends on OFFL-013, TRIP-015,
GPS-006, GPS-010, all LOCAL_DONE) on `codex/enterprise-f4-rel005` (worktree
`/data/dev/worktrees/f4-rel005`), created from `1ee50c9e`, the final F4
integration head in which every F4 implementation task is merged. The
coordinator integrates it and owns PROGRESS, the F4 ledger, pushes and the PR.
No behaviour, test, migration, SQL, route, permission or Android source
changed: a gate adds no behaviour and never stands in for a missing task test
([15-validation](../15-validation.md) §1). Owner decision EDR-271 stands. The
physical phone attached to this machine was never addressed; `emulator-5588`
was not touched (it is `RUNNING_LOCKED` behind a synthetic lock-screen
credential since 2026-11-16 11:07Z, `f4-logs/EMULATOR_5588_LOCKED_AFTER_REBOOT.md`);
no other emulator was started and no device step was run.

Sources read: the EMP-REL-005 block of [EP20](../backlog/ep20-release.md)
(BR-OFFL-011, BR-AMD-001, VAL-04, VAL-22, VAL-23, VAL-26, VAL-27, VAL-28);
[15-validation](../15-validation.md) §§1, 4 (VAL-02, VAL-04, VAL-21..28), 7 (the
device row "tracking interruption matrix on real phones of at least three
manufacturers") and 8 (the M4 row); [14-roadmap](../14-roadmap.md) §§2–3 (M4
capabilities, entry, exit and activation); [F0 decisions](f0-m0-decisions.md)
and the [F0 checkpoint](f0-local-checkpoint.md); the [F4 ledger](f4-execution.md)
(every task entry, the merged-tree verifications and the emulator incident);
every `f4-*.md` evidence file of this phase (the retained and OWED lists of
OFFL-009, OFFL-011, OFFL-012, OFFL-013, TRIP-001..023, CHECK-001..004,
FLEET-007/011/013, METER-003..006, DOC-004, FUEL-011, EXP-002/004, ROUTE-004,
GPS-001..006, GPS-010/012/015 and the tracking policy command); EDR-196,
EDR-271, EDR-272, EDR-318..359; `f4-logs/AGENT_BRIEF.md` and `HEAD_UPDATE.md`.

**No earlier milestone gate record exists** to copy: EMP-REL-001..004 are all
`TODO` and no `f*-rel00*` evidence is in the repository (the M0 record
[f0-m0-decisions](f0-m0-decisions.md) is a decision record, not a scenario
gate). The structure follows the F4 evidence files and the gate rules of
15-validation §§1, 7 and 8 (EDR-360).

IDs used: B-BR-EMP-552..555, BUC-EMP-283..284, EDR-360..362 (all; none left
unused). No migration was pre-allocated or used.

## Rules frozen before the run

B-BR-EMP-552..555 and BUC-EMP-283/284 are the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-REL-005 — M4 gate: trips, logbook, sync and tracking fundamentals
acceptance (2026-11-16)"; EDR-360..362 are appended to the EDR table of
[16-decisions](../16-decisions.md). Both were written before the scenario
union's results were read (EDR-361's list of open clauses was completed while
mapping VAL-02, not from a result). The recommended routine decisions, taken
unattended:

- **EDR-360 — gate shape**: one documented command set over the owners'
  scenario tests (G1–G3 and K below), each VAL clause and M4 exit criterion
  mapped to named tests; a clause no owner produces is recorded as not proven
  with its owner; the gate writes no test.
- **EDR-361 — decision**: source/host union PASS, device evidence OWED and
  named, M4 NOT accepted, the task `BLOCKED` naming its blockers.
- **EDR-362 — tracking hold** until the real-device interruption matrix passes
  (15-validation §8 M4 row), without changing EDR-271.

## Acceptance commands (the M4 scenario union)

Scripts: `/data/dev/worktrees/f4-logs/rel005-scripts/{env,go-postgis,go-unit,kotlin}.sh`
(also listed in [TEST_STRATEGY](../../backend/TEST_STRATEGY.md) "Enterprise M4
gate scenario union"). Environment: go1.27.4 toolchain, `GOCACHE` on `/data`,
the shared PostGIS `127.0.0.1:32775` through `orgtest` child databases (never
the control database), the throwaway MinIO project `f4-rel005-s3` (removed with
`down -v` by the script's exit trap, `rel005-s3-down.log`), JDK 17 with Gradle
serialized behind `/data/dev/worktrees/.f4-gradle.lock`.

```sh
# G1 — backend/, real PostGIS + MinIO: the named scenario tests (240 top-level tests match on this head: 239 integration, 1 untagged)
go test -race -count=1 -json -tags=integration -p 2 -timeout=150m \
  -run '^(TestLateIntake|TestTripLateIntake|TestStateIntake|TestSuspensionPreservesPersistedFieldReviewAfterReactivation|TestFuelingIntake|TestSyncResilience|TestSyncIntake|TestTrip|TestTracking|TestTrackProcessing|TestTrackRead|TestMeterCorrectionSuspectReplacementAmendmentIsolationAndRecalculation|TestMeterProjection|TestMeterSegment|TestChecklist|TestIncident|TestFleetReservation|TestAuthorizationMatrixActualCompanyHTTP|TestAuditorReadWriteMatrixActualCompanyHTTP)' \
  ./internal/modules/organization/adapters/
# G2 — backend/, real PostGIS: SQL oracles of late/state intake, the sensitive-access log, meter schema
go test -race -count=1 -json -tags=integration -p 2 ./internal/modules/orgsync/adapters/ ./internal/modules/orgaudit/adapters/ ./internal/modules/fleet/adapters/
# G3 — backend/, pure and application packages, then the route inventory
go test -race -count=1 -json -p 2 ./internal/modules/trip/... ./internal/modules/tracking/... ./internal/modules/orgsync/... \
  ./internal/modules/fleet/domain/... ./internal/modules/maintenance/... ./internal/platform/apicontract/...
go test -race -count=1 -json -p 2 -run '^(TestAuthorizationInventory|TestFullProductionRouterEnterpriseContractAndDefaultOff)' ./cmd/api/
# K — repository root, JDK 17, Gradle lock, --offline --no-daemon --no-parallel --max-workers=2
./gradlew --continue :domain:test --tests 'com.anpfuel.domain.enterprise.*' :application:test --tests 'com.anpfuel.application.enterprise.*' \
  :data:testDebugUnitTest --tests 'com.anpfuel.data.enterprise.*' :app:testDebugUnitTest --tests 'com.anpfuel.app.ui.enterprise.*'
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.*' --rerun --no-build-cache   # see "Results": the first :domain:test was restored from the build cache
bash scripts/check-mobile.sh --static-only
```

## Results on `1ee50c9e`

Every command exited 0 and no test failed or was skipped, so no load-only
rerun and no RED → GREEN fix was needed; the only rerun is the
cache-restored `:domain:test` (below). Host load average 6–20
throughout (other agents' Gradle and race runs). Logs and counts in
`/data/dev/worktrees/f4-logs/rel005-*` (`rel005-scripts/summarize.py` counts the
`go test -json` events and the JUnit XML).

| Command | Window (UTC) | Result |
|---|---|---|
| G1 PostGIS scenario tests | 16:51:23–17:07:51 | `rel005-go-postgis.jsonl`: 1 package, **240 top-level / 5 266 named, 0 failures, 0 skips**; every expected name ran (`rel005-go-postgis-expected.txt`); package 945 s; authorization matrix 69.7 s, auditor walk 11.4 s; stderr empty |
| G2 adapter packages | 17:07:51–17:08:09 | `rel005-go-postgis-pkgs.jsonl`: 3 packages, **46 top-level / 117 named, 0 failures** |
| G3 pure and application packages | 16:55:53–17:20:22 | `rel005-go-unit.jsonl`: 20 packages, **445 top-level / 4 625 named, 0 failures** (`apicontract` 1 416 s under `-race`) |
| G3 route inventory (`cmd/api`) | same window | `rel005-go-inventory.jsonl`: **4 top-level / 29 named, 0 failures** (`TestAuthorizationInventoryCoversFullProductionRouter`, `…MatchesRegisteredMeterDescriptor`, `…RejectsMissingOrFalseObligations`, `TestFullProductionRouterEnterpriseContractAndDefaultOff`) |
| K Kotlin host suites | 16:51:26–16:56:03 | `rel005-kotlin.log`, BUILD SUCCESSFUL: domain enterprise 83 classes / 4 721 tests; application enterprise 25 / 214; data enterprise 43 / 232; app enterprise 14 / 46; **0 failures, errors or skips**. `:domain:test` was **restored from the Gradle build cache** (`FROM-CACHE`), so it was not counted as executed here |
| K `:domain:test --rerun --no-build-cache` | 16:56:37–16:58:02 | `rel005-kotlin-domain-rerun.log`: executed on this tree, **83 classes / 4 721 tests, 0 failures** |
| K mobile guard | after K | `rel005-guard-static.log`: `check-mobile: static-only ok`, including "no background location tracking in shipped sources" |
| MinIO `f4-rel005-s3` | — | removed with `down -v` (`rel005-s3-down.log`); no container or volume left |

Named classes in K (tests / failures): domain `EnterpriseQueueTest` 10/0,
`EnterpriseSyncStatusTest` 7/0, `RevocationCleanupTest` 6/0,
`TrackingInterruptionsTest` 4/0, `EnterpriseTrackingSessionTest` 5/0,
`EnterpriseTrackingVectorsTest` 359/0, `TrackingSessionOperationVectorsTest`
43/0, `TrackingBatchVectorsTest` 25/0, `TrackingSamplerTest` 5/0,
`TrackingBookTest` 6/0, `EnterpriseTripExecutionLocalTest` 6/0; application
`EnterpriseOperationQueueTest` 19/0, `EnterpriseTrackingInterruptionTest` 11/0,
`EnterpriseTrackingServiceTest` 13/0, `EnterpriseTrackingBufferTest` 6/0,
`EnterpriseTrackingTransparencyTest` 4/0; data `EnterpriseSyncResilienceTest`
6/0, `EnterpriseRevocationCleanupTest` 6/0, `ProtectedTrackingStoreTest` 4/0,
`ProtectedEnterpriseQueueProcessTest` 1/0; app `TripTrackingViewModelTest` 4/0,
`EnterpriseTrackingStringsTest` 4/0.

Every Go test named in the mapping below passed in G1–G3 (64 names checked
against the JSON events); every Kotlin host class named below ran in K; the
device classes named below did not run (OWED).

Not run by this gate: the whole `organization/adapters` package and the other
enterprise packages beyond the selection (the coordinator's merged-tree union
owns them), the untagged `go test ./...` (no Go source changed), the
business-web suites of TRIP-022/CHECK-004 (not VAL clauses; their task and
integration runs are in the ledger), the 15 Rust-preparer-gated tests, and every
device stage (ledger below).

## Scenario mapping

PASS means the clause ran and passed on this head at the stated level (PostGIS,
pure Go, host JVM, static guard). **Not proven** means no owner produces the
behaviour yet; it is never replaced by a neighbouring operation. **OWED** means
written (and usually compiled) but not run on a device.

### Scenario VAL-04 — An offboarded driver delivers three fuelings (BR-OFFL-006, BR-OFFL-009, BR-OFFL-011, BR-TIME-003)

| Clause | Evidence | Result |
|---|---|---|
| Work declared before the end, received inside the 15-day window, is `ACCEPTED_FOR_REVIEW` `ACCESS_ENDED_BEFORE_RECEIPT`, open for review, applied only by another reviewer | G1 `TestLateIntakeHoldsPreEndReadingsAndRefusesLaterOnesFromServerHistory`, `TestLateIntakeHeldReadingIsResolvedOnlyByAnotherReviewer` (meter readings), `TestTripLateIntakeAfterTheMembershipEnds` (late trip finish); G2 `TestLateIntakeSQLDispositionMatchesTheSharedOracle`; G3 `orgsync/application` `TestLateIntake*` | PASS for readings and trip facts |
| … for **fuelings** (the operation VAL-04 names), not counted in costs until a manager who is not the driver resolves it | Eligible late fuelings answer `RETRY`/`UNAVAILABLE` (EDR-177/198): no held-fueling owner exists until **EMP-FUEL-003** | **Not proven** (FUEL-003) |
| A fourth operation declared after the end is `REJECTED` `MEMBERSHIP_ENDED` | G1 `…HoldsPreEndReadings…` (E, E+1 ns, E+1 h), `TestLateIntakeDeviceCommandEndUsesTheRevocationInstant` | PASS |
| Intake-only access gives no reading right | G1 `TestLateIntakeSignedRouteHoldsAndRefusesThroughTheDefaultRuntime` | PASS |
| Case 2: after the window every operation is `REJECTED` `INTAKE_WINDOW_CLOSED` | G1 `TestLateIntakeWindowIsTheSettingsInForceAtTheEnd`; G2 SQL oracle | PASS |
| Nothing accepted on the strength of the device clock | G1 `…/forged-early-declaration-is-only-evidence`; G3 `TestLateIntakeDeclaredTimeNeverAppliesOrMovesTheWindow`; `TestLateIntakeEarlierClaimIsEvidenceAndDependenciesKeepTheirOrder` | PASS |
| Lost/compromised device refused as a whole; identical deliveries keep one decision; race with the end in both orders | G1 `TestLateIntakeRefusesLostKeysForeignAndCurrentPeriods`, `TestLateIntakeSignedRouteRefusesALostDeviceAsAWhole`, `TestTripLateIntakeRefusesALostDevice`, `TestLateIntakeConcurrentIdenticalDeliveriesKeepOneDecision`, `TestLateIntakeRacesTheMembershipEndInBothOrders` | PASS |
| The phone keeps the refused work visible with its reason; a locked partition still delivers sealed pre-end work intake-only and is erased only after that or after the window closes; the app says the company must be contacted | K domain `EnterpriseSyncStatusTest` (review vs refusal codes), `RevocationCleanupTest`; data `EnterpriseRevocationCleanupTest` (6, incl. `accessEndedDeliversEligibleIntakeThenErasesOnlyItsOwnPartition`, `aClosedIntakeWindowErasesAndTheRemainingWorkIsNeverSentAgain`) | PASS (host); native `EnterpriseRevocationCleanupDeviceTest` **OWED** |

### Scenario VAL-22 part 2 — A suspended organization: field writes held or refused, reads kept (BR-AUTH-001, BR-OFFL-006, BR-OFFL-010)

| Clause | Evidence | Result |
|---|---|---|
| Work declared before the suspension, delivered inside the 7-day grace, is `ACCEPTED_FOR_REVIEW` `ORG_SUSPENDED_GRACE` | G1 `TestStateIntakeSuspensionGraceNewWorkAndReactivation`, `TestStateIntakeSignedRouteWhileSuspended` (meter readings through the signed route and default runtime), `TestStateIntakeRacesTheSuspensionInBothOrders`; G2 `TestStateIntakeSQLPlanDispositionMatchesTheSharedOracle`; G1 `TestStateIntakeSQLSuspensionMirrorsAssessSuspension` | PASS for readings |
| … for the **fueling** VAL-22 names | State holds of fuelings answer `RETRY`/`UNAVAILABLE` until **EMP-FUEL-003** (OFFL-012 retained) | **Not proven** (FUEL-003) |
| Work declared at or after the suspension is `REJECTED` `ORG_SUSPENDED` (the 12:00 trip) | G1 `TestStateIntakeSuspensionGraceNewWorkAndReactivation` (S, S+1 ns, S+1 min on the reading owner); the disposition is evaluated before every owner (OFFL-012), but no test drives a `trip.start` through it | PASS on the reading owner; the trip operation is covered only by the owner-independent rule |
| After reactivation writes succeed; no data, membership, role or enrollment changed | G1 `…SuspensionGraceNewWorkAndReactivation` (authority snapshot), `TestSuspensionPreservesPersistedFieldReviewAfterReactivation`, `TestStateIntakePlanDowngradeHoldsEarlierWorkOnly` | PASS |
| The refused trip stays visible on the phone with its reason | K domain `EnterpriseSyncStatusTest` (`ORG_SUSPENDED` refusal, `ORG_SUSPENDED_GRACE` review) | PASS (host classification); not run on a device |

### Scenario VAL-23 — Sync resilience: process death, reordering and an interrupted attachment (EMP-OFFL-013)

| Clause | Evidence | Result |
|---|---|---|
| (a) process killed right after "Salvo no aparelho": every operation still queued after the restart | K data `EnterpriseSyncResilienceTest` `killed processes lose no saved operation …` (two real child-JVM deaths), `ProtectedEnterpriseQueueProcessTest` | PASS (host); native stages 2 → 3 **OWED** |
| (b) the batch sent three times: one record per operation, identical acknowledgements | G1 `TestSyncResilienceFieldDayConvergesOnTheInOrderState/{lost_response,…,concurrent_duplicates_in_both_orders}`, `TestSyncIntakeTwelveWayDuplicatesAndTargetContention`; K `duplicate saves and receipts …` | PASS |
| (c) reverse order: `RETRY` `DEPENDENCY_PENDING`, final state equals the in-order state | G1 `…FieldDay…/lost_response,_killed_process_and_wrong_dependency_order`, `TestSyncResilienceSoftReferenceBeforeItsTargetIsHeldNotLinked`, `TestSyncResilienceMissingAcknowledgementStoresNothingAndRedecides`; K `dependent operations sent before their start …`, `the trip start naming its departure run waits …`, `EnterpriseQueueTest`, `EnterpriseOperationQueueTest` | PASS |
| (d) upload cut and retried: stored with the declared size and SHA-256 | G1 `TestSyncResilienceTruncatedReceiptUploadsRecoverOnlyWithTheDeclaredBytes` (fueling and expense receipts) | PASS (server); native stage 4 **OWED** |
| (e) one byte changed: refused, the app retries from its stored file | G1 same test (altered transfer refused, `UPLOADING` kept); K data enterprise attachment tests (OFFL-005) | PASS (server + host); native **OWED** |
| (f) clock 12 min ahead: operations flagged `CLOCK_SKEW`, declared times kept | No owner flags skew at intake (BR-TIME-006; EDR-353) | **Not proven** (retained) |
| Nothing dropped without a visible state; another organization never applies the state | G1 `TestSyncResilienceCrossScopeStateNeverApplies`; K `a lost answer and an answer missing an acknowledgement …`, `another organization's partition …` | PASS |
| Given "Bruno's emulator" | `EnterpriseSyncResilienceNativeDeviceTest` (four stages) compiled, not run | **OWED** |

### Scenario VAL-26 — A late event and an amendment after a trip is closed (BR-AMD-001/002/005, BR-OFFL-006, BR-AUD-001)

| Clause | Evidence | Result |
|---|---|---|
| (a) an event recorded offline during the trip arrives after approval: kept for review (`STALE_REVISION`), approved revision unchanged | G1 `TestTripAmendmentIsAReviewedRevisionVAL26`, `TestTripLateAndDuplicateFactsAreHeldOnceAndTheReturnIsResubmitted` (an `ARRIVAL` event; the database refuses a direct fact on the approved revision) | PASS — the example's **toll** is not a logbook type yet (`TOLL` is reserved and answered `RETRY`/`UNAVAILABLE`, EDR-207..209); a toll is recorded as a DIRECT expense (EXP-002) |
| (b) the same event again: one event | G1 same tests (byte-identical ACK, one held row), `TestTripDuplicateSubmissionsRaceLeaveOneSubmit`, `TestTripEventIntakeRollsBackAndConcurrentDuplicatesLeaveOneEvent` | PASS |
| (c) the final reading corrected 184 610 000 → 184 600 000 m with a reason: a new reading supersedes the kept one, the amendment is a new revision; 290 000 m until approval, 280 000 m after; derived figures name the revision; the audit trail holds correction, amendment and decision | G1 `TestTripAmendmentIsAReviewedRevisionVAL26`, `TestTripAmendmentThroughComposedCompanyHTTP` (`effective_usage` 290000 → 280000), `TestTripAmendmentProposalsRaceInBothOrders`, `TestTripAmendmentSchemaGuards`, `TestMeterCorrectionSuspectReplacementAmendmentIsolationAndRecalculation`, `TestTripDistanceAndReadingCorrectionSerialiseInBothOrders`; G3 `TestRevisionUsageNamesItsRevision` | PASS |
| Self-approval, stale decisions and period lock | G1 VAL-26 test (`SELF_APPROVAL`, `CONFLICT`, `PERIOD_LOCKED` then admitted after reopening), `TestTripPeriod*` | PASS |

### Scenario VAL-27 — GPS distance against odometer distance (BR-TRUTH-003/004/005, BR-MONEY-009)

| Clause | Evidence | Result |
|---|---|---|
| 290 000 m odometer vs 262 000 m GPS: 28 000 m = 9.66 % above 5 % → signal raised; 284 000 m: 2.07 % → not raised; no usable track → `INSUFFICIENT_DATA`, not zero | G3 `tracking/domain` `TestDistanceVal27`, `TestDistanceDivergenceBoundaries`, `TestDistanceAbsentBasisIsNeverZero`, `TestDistanceDivergenceStatesNoMisconduct` | PASS (V-DOM) |
| The GPS distance and its gaps come from the processed track (V-JOB) | G1 `TestTrackProcessingDerivesTheSummaryOnceTheSessionEnds`, `TestTrackProcessingConcurrentRunsConvergeInBothOrders`, `TestTrackProcessingRefusesStaleForeignAndOpenInputs`; G3 `TestGapsAreNeverBridged` | PASS |
| Both distances stored as separate figures with their sources; neither replaces the other | G1 `TestTripDistancesComposeTheMeterAndTheTrackUnderTheTrackGrant` (real processed track and METER-004 corrections: `NOT_RAISED` at 1 500 m, `RAISED` at 8 000 m), `TestTripDistanceWithoutAFinalReadingIsNeverZero`; G3 `TestDistanceMixedSourcesAreRefusedOrKeptApart` | PASS |
| Both **shown**; the signal **asks for review and shows the gap** | No read route composes the distance port (EMP-TRIP-006, EDR-318); the review outcome UC-GPS-08 and `gps.divergence.detected` are retained by GPS-006 | **Not proven** (TRIP-006, UC-GPS-08) |

### Scenario VAL-28 — A tracking interruption becomes a gap (BR-PRIV-006, BR-PRIV-015, BR-TIME-005, EDR-015)

| Clause | Evidence | Result |
|---|---|---|
| (a) permission revoked 10:20:00, granted 10:35:00: `INTERRUPTED`, 900 s gap, no invented fixes | G3 `tracking/domain` `TestVal28InterruptionsBecomeGaps` (gaps 1200–2100 `PERMISSION_REVOKED` and 3600–3960 `PROCESS_STOPPED`, 1 260 s, quality `APPROXIMATE`), `TestNotRecordingTimeIsAGapWithItsReason`; K domain `TrackingInterruptionsTest`, application `EnterpriseTrackingInterruptionTest` `missing gap - …` | PASS (pure Go + host); emulator stage 3 **OWED**; real phones **OWED** |
| (b) process killed 11:00:00, reopened 11:06:00: no silent resume, the app asks, 360 s gap | G3 `TestVal28InterruptionsBecomeGaps`; K `unapproved restart - …` (3 tests), app `TripTrackingViewModelTest` | PASS (host); emulator stage 2 and GPS-002 force-stop stage **OWED**; real phones **OWED** |
| (c) approximate location only: quality `APPROXIMATE`, shown as such | K `precision overclaim - …` (3 tests); G3 `TestCoarsePeriodsAreApproximateAndUnmeasured` | PASS (host); emulator stage 2 **OWED** |
| (d) paused then ended: no fix while paused or after the end, session `ENDED` | G1 `TestTrackingIntakeFollowsTripPauseResumeAndCancellation`, `TestTrackingIntakeStoresNothingItMustRefuse`, `TestTrackingIntakeRecordsTheSessionAndStoresEachBatchOnce`; K `missing gap - a device pause and cancellation compose …`, `a pause or a cancellation releases the listener …` | PASS |
| The manifest requests no background location; the guard passes with the tracking service listed | K `scripts/check-mobile.sh --static-only` ("no background location tracking in shipped sources") | PASS |
| Given "on an emulator" | GPS-010 `EnterpriseTrackingNativeDeviceTest` stages 1–3 and `EnterpriseTrackingInterruptionDeviceTest` compiled, not run | **OWED** |
| Behaviour on real devices of several manufacturers (§7) | — | **OWED** (owner / pilot users, EMP-GOV-006) |

### Scenarios VAL-21 and VAL-02 over the F4 routes and permissions (§8: rerun at every gate)

| Clause | Evidence | Result |
|---|---|---|
| VAL-21: every matrix cell, the decision order, and no route outside the pipeline | G1 `TestAuthorizationMatrixActualCompanyHTTP` (300 cells, 121 allowed / 179 denied, each with the other-organization, ended/suspended membership, suspended organization, proof and origin variants), `TestAuditorReadWriteMatrixActualCompanyHTTP` (166 routes / 33 positive / 133 `FORBIDDEN`); G3 `cmd/api` `TestAuthorizationInventory*` (166 normal / 30 routed / 1 signed / 54 not routed; specials 26) and `TestFullProductionRouterEnterpriseContractAndDefaultOff`; `apicontract` (20 signed operation owners) | PASS |
| VAL-02: another organization sees nothing on the F4 surfaces | The matrix's other-organization variant per permission representative; every F4 owner's foreign-organization `NOT_FOUND`, forced-RLS schema and scope-isolation tests in G1 (`TestTrip*ScopeIsolationAndAuditor`, `TestTrip*SchemaIsForcedRLS*`, `TestTrackProcessingRefusesStaleForeignAndOpenInputs`, `TestTripDistancesCompose…/foreign-unknown-and-unproven-reads-not-found`, `TestSyncResilienceCrossScopeStateNeverApplies`, `TestChecklistTemplateSchemaGuardsAndIsolation`, `TestFleetReservationSchemaGuardsAndIsolation`, `TestIncidentReportHandlingAuthorityAndIsolation`); G2 `TestSensitiveAccessIsolationAppendOnlyAndDatabaseGuards`; G3 redaction tests (`TestProcessingValuesAreRedacted` and the module redaction checks) | PASS per owner; the single cross-surface VAL-02 sweep (every route with A's identifiers, sealed cursors, capabilities, job payloads, exports, a query without the organization setting, log scan of the whole run) is **not run as one harness** by any gate yet |

## M4 exit and entry criteria

| Criterion ([14-roadmap](../14-roadmap.md) §3, [15-validation](../15-validation.md) §8) | State on `1ee50c9e` |
|---|---|
| Entry: M3 accepted | **Not met** — EMP-REL-004 (M3 gate) is `TODO`; no M3 acceptance is recorded |
| Entry: tracking spike and legal validation (EMP-GOV-005, EMP-GOV-006) | GOV-005 `LOCAL_DONE` (declared reviewed by the owner, EDR-271); GOV-006 `TODO` — EDR-271 accepts the emulator for the spike, but the GPS-002/003/010/015 emulator stages never completed (emulator locked) and physical target devices are OWED |
| Exit: sync resilience suite | Server and host PASS (VAL-23 a–e); (f) not proven; native stage OWED |
| Exit: amendments after closure | PASS (VAL-26), with the toll-as-logbook-type note |
| Exit: GPS distance and divergence | Arithmetic, processing and composition PASS; display and review request not proven (TRIP-006, UC-GPS-08) |
| Exit: tracking interruption matrix on real devices (owed evidence listed, not assumed) | **OWED**, listed below; tracking held (EDR-362) |
| §8: every listed scenario passed on one named revision | **No**: VAL-04 and VAL-22 part 2 (fuelings), VAL-23 (f), VAL-27 (display/review), VAL-28 (emulator/real devices) |
| §8: VAL-02 and VAL-21 rerun over the phase's routes | VAL-21 PASS; VAL-02 per owner (no single sweep) |
| §8: the §7 owed items listed | Yes (device ledger below) |
| M4 activation: tracking enabled per organization after the employee notice exists; matrix owed → tracking enabled for no organization | Held (EDR-362) |
| Capabilities at exit (missions, planned trips, stops and legs, pause and resume, handover, supervisor closure, amendments, period lock; checklists and incidents; tracking sessions with upload; GPS distance; post-offboarding intake) | Server suites PASS in G1 (`TestTripMission*`, `TestTripMissionPlan*`, `TestTripStops*`, `TestTripLifecycle*`, `TestTripHandover*`, `TestTripClosure*`, `TestTripAmendment*`, `TestTripPeriod*`, `TestChecklist*`, `TestTripChecklistGate*`, `TestIncident*`, `TestFleetReservation*`, `TestTripMissionReservation*`, `TestTracking*`, `TestTrackProcessing*`, `TestTrackRead*`, `TestTripDistance*`, `TestLateIntake*`); app host suites PASS in K; their native stages OWED (ledger) |

## Named negative cases

| Negative case | Evidence | Result |
|---|---|---|
| Missing device coverage called pass | This record: every device stage is OWED with its driver; B-BR-EMP-552; EDR-361; the backlog block is `BLOCKED` | Held |
| Private fixtures committed | Scan of the F4 diff (below) | PASS |
| Silent queue loss | K `EnterpriseSyncResilienceTest` (process death after the last save and after the server commit; lost and incomplete answers apply nothing), `ProtectedEnterpriseQueueProcessTest`, `EnterpriseRevocationCleanupTest` (an ended membership still delivers sealed pre-end work; removal only after verified disposal; an interrupted cleanup stays denied), `EnterpriseTrackingInterruptionTest` `buffer loss - …` (each recorded fix pending once or in exactly one batch); G1 `TestSyncResilienceMissingAcknowledgementStoresNothingAndRedecides` | PASS (host + PostGIS); native OWED |
| Location resumed without authority | K `unapproved restart - …` (no grant → nothing written; a fresh grant on an interrupted session listens to nothing; every tap re-checks policy, membership and the one-listener rule; online policy read first), `TripTrackingViewModelTest` (only a session recording after the tap launches the service; a resume after a pause launches nothing), `EnterpriseTrackingServiceTest`; G1 `TestTrackingIntakeOpensOnlyUnderAPolicyWithItsNoticeInForce`, `TestTrackingIntakeFollowsTripPauseResumeAndCancellation`, `TestTrackingPolicyMineReadsThePolicyInForceForTheDriver`; static guard (no background location, boot receivers or sticky starts) | PASS (host + PostGIS + guard); a real reboot and manufacturer process kills OWED |

## Device evidence ledger (OWED — nothing below was run)

None of these steps was run by this task. Every native stage needs
`emulator-5588` unlocked once by its owner (the agents may not enter, wipe or
replace it, nor start another emulator) and runs under
`flock /data/dev/worktrees/.f4-emulator.lock`; each driver refuses a locked user
0. The drivers were prepared by the owning tasks in their own worktrees, so a
run proves that task's tree (named): for M4 acceptance the validation APKs must
be rebuilt from the gate head (`-PenterpriseDeviceValidation=true
:app:assembleDebug :app:assembleDebugAndroidTest :data:assembleDebugAndroidTest`;
`exp004-scripts/app-device.sh` accepts `ROOT=` for app classes), and each
source needs its throwaway MinIO project, removed with `down -v` afterwards.

Required by the M4 scenarios:

| # | Owed acceptance | Scenario | Prepared one-line driver (`/data/dev/worktrees/f4-logs/…`) |
|---|---|---|---|
| D1 | OFFL-013 `EnterpriseSyncResilienceNativeDeviceTest`, four stages (online custody, offline field day with both receipts, survival after process death, lost answer then truncated receipt); expected 1 trip, 1 event, 1 finish, 1 fueling, 1 direct expense, 2 documents stored, links `FUELING` 1 / `EXPENSE` 1, 1 lost intake answer, nothing in B. Tree `f4-offl013` `dd8588c8`; MinIO `f4-offl013-s3` | VAL-23 (a), (d), (e) on the emulator | `bash /data/dev/worktrees/f4-logs/offl013-scripts/native-run.sh` |
| D2 | GPS-002/003 service and sampling: policy read, trip start, explicit tracking start, recording on a simulated `adb emu geo fix` route, stop from the notification, resume, process death, recovery, finish; server transitions `NOT_STARTED, RECORDING, INTERRUPTED/STOPPED_BY_DRIVER, RECORDING, INTERRUPTED/PROCESS_STOPPED, ENDED/TRIP_COMPLETED`; then the app service device test. Tree `f4-gps002` `d971686f`; MinIO `f4-gps002-s3` | VAL-28 (b) base | `(bash /data/dev/worktrees/f4-logs/gps002-scripts/native-source.sh > /data/dev/worktrees/f4-logs/gps002-native-source.log 2>&1 &) && flock /data/dev/worktrees/.f4-emulator.lock bash /data/dev/worktrees/f4-logs/gps002-scripts/native-driver.sh` |
| D3 | GPS-010 interruption stages 1–3 (`location_mode 0` gap; force-stop + fine location revoked → `PROCESS_STOPPED`, approximate resume; coarse revoked → `PERMISSION_REVOKED`, nothing resumes; every operation accepted) and `EnterpriseTrackingInterruptionDeviceTest` (notification states the gap; a second session interrupted); fixture transitions `NOT_STARTED, RECORDING, INTERRUPTED/PROCESS_STOPPED, RECORDING, INTERRUPTED/PERMISSION_REVOKED, INTERRUPTED/PERMISSION_DENIED, ENDED/TRIP_COMPLETED`. Tree `f4-gps010` `763a498d`; MinIO `f4-gps010-s3`. A reboot is not driven (it would restart the shared emulator) | VAL-28 (a)–(c) on the emulator | `(bash /data/dev/worktrees/f4-logs/gps010-scripts/native-source.sh > /data/dev/worktrees/f4-logs/gps010-native-source.log 2>&1 &) && flock /data/dev/worktrees/.f4-emulator.lock bash /data/dev/worktrees/f4-logs/gps010-scripts/native-driver.sh` |
| D4 | OFFL-011 `EnterpriseRevocationCleanupDeviceTest` (real AndroidKeyStore `ended_` marker; LOCK delivery then erasure of exactly one partition; interrupted erasure stays denied). Tree `f4-offl011` `90bcb693`; build first with `offl011-scripts/gradle.sh :data:assembleDebugAndroidTest` | VAL-04 device side | `/data/dev/worktrees/f4-logs/offl011-scripts/native-driver.sh` |
| D5 | **Real-device tracking interruption matrix** on phones of at least three manufacturers: screen off, app swiped away, reboot (`DEVICE_RESTARTED`, host-proven only), force stop, battery saver, Doze; battery and mobile data per hour of tracking (§7 second device row). Synthetic organization and trips; phones the owner or pilot users consent to use and operate themselves; results per device and case | VAL-28 §7, M4 exit, EMP-GOV-006 | None (human-run); the phone attached to this machine is never used by an agent |

Other F4 native acceptance owed (task-level; not M4 scenario clauses):

| # | Owed acceptance | Prepared one-line driver (`/data/dev/worktrees/f4-logs/…`) |
|---|---|---|
| D6 | GPS-015 `EnterpriseTrackingTransparencyDeviceTest` (indicator and "Meu rastreamento" in pt-BR) and, in D3's app stage, `theIndicatorSourceFollowsTheShippedServiceFromItsListenerToItsStop`; on-screen indicator on every enterprise panel with the real Hilt graph | `flock /data/dev/worktrees/.f4-emulator.lock bash /data/dev/worktrees/f4-logs/gps015-scripts/native-driver.sh` |
| D7 | TRIP-021 five-stage `EnterpriseMissionNativeDeviceTest` and the mission, trip, my-assets and context presentation classes (tree `f4-trip021` `cd133382`) | `bash /data/dev/worktrees/f4-logs/trip021-scripts/native-run.sh` |
| D8 | ROUTE-004 `EnterpriseRouteHandoffPresentationDeviceTest` with the TRIP-021/TRIP-005 classes; plus a manual run through the real MAP tab (route sheet, search result, guidance) with the queue, trip book and intake unchanged | `bash /data/dev/worktrees/f4-logs/route004-scripts/native-run.sh` |
| D9 | EXP-004 four-stage `EnterpriseExpenseNativeDeviceTest` (2 documents stored, links `EXPENSE` 2, uploads clean 2 / truncate 1, 2 DIRECT expenses) and `EnterpriseExpensePresentationDeviceTest`; `EnterpriseSyncContextDeviceTest` with the expense owner (tree `f4-exp004` `0dcbcfe1`; MinIO `f4-exp004-s3`) | `(bash /data/dev/worktrees/f4-logs/exp004-scripts/native-source.sh > /data/dev/worktrees/f4-logs/exp004-native-source.log 2>&1 &) && flock /data/dev/worktrees/.f4-emulator.lock bash /data/dev/worktrees/f4-logs/exp004-scripts/native-driver.sh` |
| D10 | TRIP-020 `EnterpriseTripPresentationDeviceTest` (7 methods incl. the closing conference) and a real-backend trip run conferring a fueling and an expense with receipts | `flock /data/dev/worktrees/.f4-emulator.lock bash /data/dev/worktrees/f4-logs/exp004-scripts/app-device.sh EnterpriseTripPresentationDeviceTest EnterpriseExpensePresentationDeviceTest` |
| D11 | Sync-context fix (EDR-335): two regression classes, then 20 consecutive `EnterpriseSyncContextDeviceTest` runs on the kept fixed APK pair | `APKDIR=/data/dev/cache/f4-exp004-apks/fix bash /data/dev/worktrees/f4-logs/exp004-scripts/flake-green.sh` |
| D12 | Two-physical-device runs (TRIP-005 concurrent starts, TRIP-021, OFFL-005, OFFL-013, FUEL-011, CHECK-002 second device); native other-account and 24 h boundary checks of the trip book and "Meus ativos" (TRIP-005, FLEET-013); CHECK-002 real Hilt UI end to end and release while the device watches; TalkBack and visual reviews of the F4 screens | None (human-run on consenting devices) |

Already run on `emulator-5588` before the lock (task trees, recorded in their
evidence): FLEET-013, TRIP-005, OFFL-005, CHECK-002, DOC-004, FUEL-011 and the
DOC-004/FUEL-011 integration regression. They are not rerun on the gate head.

## Private-data scan of the F4 diff (`b0ab7d33..1ee50c9e`)

`git diff b0ab7d33..HEAD --stat`: 720 files, 119 909 insertions, 3 254
deletions; added files are source and resources only (204 `.go`, 144 `.kt`, 80
string `.xml` in eight locales, 39 `.md`, 26 `.sql`, 15 `.psv`, 4 `.ts`, 2
`.py` and 2 `.mjs` web tests) — no image, archive, key, certificate, CSV,
recording or binary file. Fixture and test-data files added: the 15 shared
vector files under `contracts/testdata/enterprise/` (checklist run, expense and
fueling submit, meter segments, attachment session, tracking session, session
operations and track batch, trip handover, mission v2/v3, stops,
trip-operations v2/v3, trip v2), five `nativefixture` Go sources and the Kotlin
tracking fixtures (list: `f4-logs/rel005-f4-fixture-files.txt`). Results:

- `scripts/scan-secrets.sh`: passed (`rel005-scan-secrets.log`);
  `git diff b0ab7d33..HEAD | gitleaks stdin --redact`: "no leaks found" over
  9.3 MB (`rel005-gitleaks-f4-diff.log`).
- Personal identifiers in added lines: the only CPF-shaped value is the
  synthetic `123.456.789-09` used by refusal tests (11 occurrences); no CNPJ;
  e-mail-shaped strings are Kotlin `this@…` labels and `synthetic@example.invalid`;
  phone-shaped matches are UUID fragments; plate-shaped values are the
  synthetic `ABC1D23` and `WRP1A23`.
- Coordinates: positions exist only as synthetic E7 integers and literals on a
  digit-pattern grid (`-15.6012345, -56.1098765` and neighbours: 427 E7 pairs
  and 7 decimal literals; three `-10.0, -50.0` pairs), in the tracking vectors, the tracking/processing
  tests and the OpenAPI examples; the native drivers send a simulated route with
  `adb emu geo fix` from scripts outside the repository. No recorded or
  contributor GPS trace, no real place of a person.
- Photos and receipts: none committed; device tests generate synthetic JPEGs at
  run time. Logs, native fixture files and credentials of the throwaway MinIO
  stay outside the repository (`f4-logs`, `/data/dev/cache`). No OCR training
  data.

## Gate decision (EDR-361, EDR-362)

- **Source/host scenario union: PASS** on `1ee50c9e` (counts above), with the
  clauses below recorded as not proven.
- **Not proven (no owner produces them yet):** held late and suspended-grace
  fuelings (EMP-FUEL-003; VAL-04, VAL-22 part 2); VAL-23 (f) `CLOCK_SKEW`
  (BR-TIME-006 owner); VAL-27 display, review request and gap with the signal
  (EMP-TRIP-006, UC-GPS-08); a `trip.start` driven through the suspension
  disposition (the owner-independent rule is proven on readings); VAL-02 as
  one cross-surface sweep.
- **Device evidence: OWED** (D1–D12), including the real-device interruption
  matrix on at least three manufacturers (D5).
- **Entry conditions not met:** M3 (EMP-REL-004 not run); EMP-GOV-006 `TODO`.
- **M4 is NOT accepted.** EMP-REL-005 is `BLOCKED` naming these blockers.
  Nothing here claims F4 or M4 complete, INTEGRATED or RELEASE_CERTIFIED.
- **Tracking held (EDR-362):** while D5 is owed, tracking collects from no real
  employee of any organization; every real organization's tracking policy stays
  absent or `OFF`; `OPTIONAL`/`EXPECTED` only in synthetic validation
  organizations. EDR-271 is unchanged.

## Validation of this commit

```sh
python3 docs/enterprise/tools/check_plan.py                       # tasks 348; rules 545; use cases 167; screens 145; problems 0
git diff --cached --check                                          # clean
git diff --cached | gitleaks stdin --redact                        # no leaks found
bash scripts/scan-secrets.sh                                       # passed
```

The commit changes documentation only (this record, the contract section, three
EDR rows, the backlog block and a TEST_STRATEGY section): no Go, Kotlin, SQL,
OpenAPI, route inventory or check-manifest file, so gofmt, vet, staticcheck,
sqlc and the build gates have nothing new to check. The staged diff was
reviewed for secrets and personal data: synthetic identifiers and commands
only; the disposable MinIO credentials appear only in the scripts under
`f4-logs`.

## Retained

- D1–D12 above (owner unlock of `emulator-5588`; owner or pilot users for real
  devices; never the attached phone).
- EMP-FUEL-003 (held fuelings for late and state intake), the BR-TIME-006 skew
  owner, EMP-TRIP-006 (distance read and display), UC-GPS-08 and
  `gps.divergence.detected`, a single cross-surface VAL-02 harness.
- EMP-REL-004 (M3 gate) and EMP-GOV-006 (device spike) before any M4 rerun.
- The rerun of this gate on the head that contains the device results; the
  coordinator's merged-tree union and the cumulative PR.
- Proposed shared text for the coordinator (not edited here):
  [15-validation](../15-validation.md) §8 M4 row, after "or listed as owed and
  tracking not enabled for any organization": "(gate run on `1ee50c9e`,
  not accepted: `evidence/f4-rel005-m4-gate.md`, EDR-361/362)".
