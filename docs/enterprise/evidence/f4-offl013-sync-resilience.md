# EMP-OFFL-013 — Sync resilience suite: duplicates, reordering, partial uploads, process death

Date: 2026-11-16.
Status: LOCAL_DONE
Validation: PARTIAL — server (real PostGIS, MinIO) and device host halves PASS; native emulator stage OWED (emulator locked)
Integration: INTEGRATION_PENDING

Bounded F4 TEST task (module OFFL, P1, size M; depends on OFFL-002, OFFL-003,
OFFL-005) on `codex/enterprise-f4-offl013` (worktree
`/data/dev/worktrees/f4-offl013`), based on the F4 integration head
`78de91be`. The coordinator integrates it and owns PROGRESS, the F4 ledger,
pushes and the PR. Synthetic data only. No migration (000261 unused), no SQL,
route, permission, role or job change. The physical phone was never
addressed; `emulator-5588` was not used (it is `RUNNING_LOCKED` behind its
synthetic lock-screen credential since 2026-11-16 11:07Z, see
`f4-logs/EMULATOR_5588_LOCKED_AFTER_REBOOT.md`).

Sources read: the EMP-OFFL-013 block of [EP04](../backlog/ep04-sync.md);
[10-offline-sync](../10-offline-sync.md) §§3–8, 10.2–10.4 and 12 (BR-OFFL-002,
004, 005, 007, 013, 015; UC-OFFL-01..03; acceptance 1–3 and 8);
[15-validation](../15-validation.md) §§1–4 (VAL-06, VAL-23) and §8; EDR-076..080,
202, 229, 265/266, 267..270, 284, 331..334 and the latest rows of
[16-decisions](../16-decisions.md); the evidence of OFFL-002 (intake ledger),
OFFL-003 (protected queue), OFFL-005 (attachment upload),
[OFFL-009](f4-offl009-post-revocation-window.md),
[OFFL-012](f4-offl012-org-state-queued-operations.md),
[TRIP-005](f4-trip005-app-trip.md), [FUEL-011](f4-fuel011-app-fueling.md),
[EXP-004](f4-exp004-app-expense.md), [TRIP-019](f4-trip019-checklist-gate.md),
[TRIP-023](f4-trip023-reservation-trip-link.md) and the
[sync-context fix](f4-fix-sync-context-intermittent.md).

IDs used: B-BR-EMP-540..543, BUC-EMP-277..278, EDR-351..353 (all of them;
none left unused). Migration 000261 was not needed.

## Observed before any change

- Each owner already had its own replay/reuse/race tests, but nothing drove
  the real signed operations of one field day together through disrupted
  delivery and compared the final state with in-order delivery.
- A soft reference names an entity, not an operation: the server holds a
  `trip.start` whose departure run (or mission) is not stored yet
  `ACCEPTED_FOR_REVIEW` `ORPHAN_REFERENCE` (or `ASSET_UNAVAILABLE` when the
  checklist is required) with nothing linked (BR-OFFL-007, TRIP-019/023), and
  `trip.start` may not declare dependencies (EDR-202, EDR-331).
- The Android dispatcher ordered only declared dependencies and decided due
  time per operation. A departure run whose first delivery failed sat in
  backoff while the trip start naming it was due at once, so the start went
  alone. **This is the defect the suite found** (below).

## Rules frozen before behaviour

B-BR-EMP-540..543 and BUC-EMP-277/278 are the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md)
section "EMP-OFFL-013 — Sync resilience suite: duplicates, reordering, partial
uploads, process death (2026-11-16)"; EDR-351..353 are appended to the EDR table
of [16-decisions](../16-decisions.md). Both were written before any code. The
recommended routine decisions, taken unattended:

- **EDR-351 — suite shape.** Server half over the real default enterprise
  runtime through the signed TLS routes on real PostGIS (+ MinIO for
  receipts); device half over the real protected vault, queue, product
  producers, queue service and signed HTTP client behind an edge ledger of the
  §10.2 rows, with a real child JVM for process death; the native stage
  against the DOC-004 receipt fixture with a new one-shot lost-answer fault,
  compiled and owed. One command per half (TEST_STRATEGY).
- **EDR-352 — device ordering for soft references (the fix).**
- **EDR-353 — convergence oracle.** The effect signature excludes server
  times and generated identifiers; the soft-reference-first server order is a
  named divergence; VAL-23 (f) `CLOCK_SKEW` retained.

## Defect found and fixed (RED → GREEN in this commit)

**Device sent a trip start before its departure checklist run was finally
acknowledged.** `EnterpriseQueueService.dispatch` checked each operation's own
due time; with the run in backoff and the start freshly saved, the start was
sent alone (or in the same batch as a run that was then answered `RETRY`) and
the server held it `ORPHAN_REFERENCE` for review with nothing linked — an
irreversible divergence from in-order delivery (the start would have been
accepted with its run linked). Fix (EDR-352):

- Domain `EnterpriseQueueOrdering.localReferences` maps each queued operation
  to the other unacknowledged operations of the same queue whose entity (type
  and identifier) one of its soft references names; `order(operations,
  localParents)` orders them first, and a cycle through either kind of parent
  is `cycleBlocked` (attention).
- Application `EnterpriseQueueService.dispatch` holds an operation until each
  referenced operation has a final receipt, with that operation's remaining
  delay; a dependent of a held operation is held with it (never sent to
  collect a `DEPENDENCY_PENDING`); a reference to an operation that needs
  attention needs attention. Without soft references nothing changes.

## Delivered surface

- **Server half** `backend/internal/modules/organization/adapters/sync_resilience_integration_test.go`
  (tag `integration`): the composed default runtime (no injected handler,
  store or plan; an explicit synthetic intake budget only so repeated attempts
  are admitted), the signed TLS intake with a transport that can close a
  committed request's connection before its answer, and one field day of the
  real operations: `trip.mission.respond` (ACCEPT), `check.run.complete`
  (DEPARTURE), `trip.start` naming the mission and the run, two
  `trip.event.record`, `meter.reading.record`, `fueling.submit`,
  `expense.submit`, `trip.finish` and `trip.submit`.
- **Device half** `data/src/test/kotlin/com/anpfuel/data/enterprise/`:
  `EnterpriseSyncResilienceTest`, `SyncResilienceHarness.kt` (the edge
  `ResilienceLedger`, file-backed so a child's commits survive its death; the
  `ResilienceDevice` graph with a real `AuthFlow` whose live synthetic session
  needs no network; `FieldDay` through the product builders) and
  `SyncResilienceProcessProbe` (child JVM: dies after the last save, or after
  the server committed at the first durable receipt write).
- **Native stage (owed)** `data/src/androidTest/.../EnterpriseSyncResilienceNativeDeviceTest.kt`
  (four processes: online custody, offline field day with both receipts,
  survival after process death, lost answer then truncated receipt) and the
  receipt fixture's one-shot `intake-plan` `lose` fault plus optional `lost`,
  `events` and `finishes` expectations (`orgsync_receipt_native_fixture_test.go`,
  earlier drivers unchanged). Driver: `f4-logs/offl013-scripts/native-run.sh`.
- Domain/application tests for the fix: `EnterpriseQueueTest` (2 new),
  `EnterpriseOperationQueueTest` (3 new).
- Docs: TEST_STRATEGY "Enterprise sync resilience suite (EMP-OFFL-013)", the
  backlog status note, this evidence.

## Server half on real PostGIS (`TestSyncResilience*`)

Real default runtime, real signed TLS requests (identity-v1 proof of an
enrolled device), orgtest child databases of `127.0.0.1:32775`, the throwaway
MinIO project `f4-offl013-s3`. The in-order field day is the reference state
(every operation `ACCEPTED`; trip `COMPLETED`, two events, one finish, review
`SUBMIT:SUBMITTED:1`, readings `TRIP_START`/`STANDALONE`/`FUELING`/`TRIP_END`,
one fueling with its one `FUELING` cost and the `DIRECT` toll, one
`COMPLETED_OK` run linked `DEPARTURE`, one mission link and the mission
`COMPLETED`, no review item, nothing held; 10 claims / 10 receipts / 10
effects). The meter domain marks the readings `SUSPECT` (a jump from the
fixture's 0 installation reading) in every delivery order: classification is
order-independent, so it is part of the equivalence.

| Test (subtest) | Scenario and observation |
|---|---|
| `TestSyncResilienceFieldDayConvergesOnTheInOrderState/in_order` | Reference signature (EDR-353). |
| `…/lost_response,_killed_process_and_wrong_dependency_order` | **Lost response**: the reversed queue (dependencies reversed, soft targets kept first) is committed and its connection closed before the answer: submit, finish, arrival and stop `RETRY` `DEPENDENCY_PENDING` with no receipt, effect or audit; the six others `ACCEPTED`. **Killed process**: the restarted device resends its whole queue; the six committed acknowledgements come back byte for byte, the finish and events are accepted, the submission ahead of its finish is retried and then accepted. **Duplicates**: three more whole-queue resends in both orders, every acknowledgement byte-identical. The final signature equals the in-order one. **Reuse**: each of the ten identifiers with another digest is `REJECTED` `OPERATION_ID_REUSED`; signature and original acknowledgements unchanged. |
| `…/concurrent_duplicates_in_both_orders` | Six copies (three in device order, three reversed) signed first, released together behind the held organization root that every intake transaction locks (six runtime sessions observed blocked); every final acknowledgement equals the settled one, any other answer is `RETRY`; signature equals the in-order one. |
| `TestSyncResilienceSoftReferenceBeforeItsTargetIsHeldNotLinked` | The named divergence (EDR-353): a start delivered before its run and mission acceptance is held `ACCEPTED_FOR_REVIEW` `ORPHAN_REFERENCE`; no trip, no checklist or mission link; its replay is unchanged after the targets arrive. |
| `TestSyncResilienceMissingAcknowledgementStoresNothingAndRedecides` | A `RETRY` stores no receipt, effect or audit; after the start the same stop and finish are decided afresh and stored once. A batch with a foreign enrollment (400) and an expired signature (401) answer no acknowledgement and leave every owner's state unchanged; the resent fueling is stored once and replays. |
| `TestSyncResilienceTruncatedReceiptUploadsRecoverOnlyWithTheDeclaredBytes` | For the `FUELING` and the `EXPENSE` receipt: truncated, altered and interrupted transfers are refused and leave `UPLOADING` revision 1; a retry session writes nothing and the declared bytes are `STORED` with the declared SHA-256 and size; a repeat is a replay; one received and one stored event and one link each; two documents in all. |
| `TestSyncResilienceCrossScopeStateNeverApplies` | The same account in organization B with its own enrollment: A's field day on B's route and B's operation on A's route are refused as a whole and store nothing; B's own expense carrying A's start identifier gets B's own acknowledgement; B holds exactly that expense; A's state and acknowledgement are unchanged; one acknowledgement per (organization, identifier); a bare runtime connection reads none. |

## Device half on the host JVM (`EnterpriseSyncResilienceTest`)

| Test | Scenario and observation |
|---|---|
| `killed processes lose no saved operation …` | **Killed process**: a child JVM saves the field day through the product producers and halts right after the last save: all seven operations are pending with their exact bytes, in save order, nothing sent. A second child delivers and halts after the ledger committed, at the first durable receipt write: every operation is still pending; the restarted device replays the committed ones (no new effect) and applies the rest once; every local receipt equals the server's bytes and is projected once. |
| `a lost answer and an answer missing an acknowledgement …` | **Lost response** and **missing acknowledgement**: nothing is acknowledged from a lost answer or from an answer missing its last acknowledgement; no new server effect on resend; then everything converges once (the run was sent at least three times). |
| `the trip start naming its departure run waits …` | **Wrong dependency order (the defect)**: run in backoff, start saved; the start reaches the server only after the run's final receipt and is `ACCEPTED`. RED before the fix: `ACCEPTED_FOR_REVIEW` `ORPHAN_REFERENCE`. |
| `dependent operations sent before their start …` | The stop and finish due while the start is in backoff are sent, answered `RETRY` `DEPENDENCY_PENDING`, nothing stored or applied; then converge. |
| `duplicate saves and receipts …` | **Duplicate effects**: identical resaves are idempotent, another intent under an identifier is refused (`EnterpriseOperationReused`), a replayed receipt projects nothing and other receipt bytes are refused. |
| `another organization's partition …` | **Stale cross-scope state**: B's toll reusing A's identifiers is never sent with A's delivery; A's receipt cannot be applied in B (`EnterpriseLocalDenied`); B's own receipt differs and A's stays byte-identical. |

## RED → GREEN → REFACTOR

Logs in `/data/dev/worktrees/f4-logs/offl013-*` (XML in `offl013-kotlin-red-xml/`).

- **Shape first**: the domain `localReferences` returning an empty map and
  `order(operations, localParents)` ignoring the local parents; the dispatcher
  unchanged; then the frozen tests.
- **Kotlin RED** (`offl013-kotlin-red.log`): domain `EnterpriseQueueTest` 10
  tests, 2 failing (the two new ordering tests); application
  `EnterpriseOperationQueueTest` 18 tests, 3 failing (the three new dispatcher
  tests: the start sent before its run, the start sharing a batch with a run
  answered `RETRY`, references to unsendable work not marked attention). The
  first data run failed 6/6, five for harness reasons — the edge ledger wrote
  acknowledgements with unsorted keys while the device keeps the canonical
  sorted form, and the killed-process test rebuilt the completed run from its
  closed book — both corrected without changing any expectation; the genuine
  data RED (`offl013-kotlin-red-2.log`) is 6 tests, 1 failing for the defect
  ("held ORPHAN_REFERENCE").
- **GREEN 1** (`offl013-kotlin-green-1.log`): domain 10/10, application
  18/18, data 5/6: the lost-answer test showed a flaw in the first fix (a
  held operation kept the delay its referenced operation had before the batch,
  0 while selected, so a failed batch reported work due now). The service now
  settles held delays after each batch; the application test also asserts the
  settled delay. **GREEN 2** (`offl013-kotlin-green-2.log`): 10/10, 18/18,
  6/6.
- **Go** (acceptance on the unchanged server, not RED): the first run
  (`offl013-go-suite-1.jsonl`) passed 5 of 8 named tests; the failures were my
  fixture assumptions — `VALID` meter states (the domain says `SUSPECT`) and a
  reuse builder that resealed the stored digest and re-encoded numbers — fixed
  without changing a product expectation; then 8/8
  (`offl013-go-suite-3.jsonl`, `offl013-go-green.jsonl`).
- **REFACTOR**: the early-start test pins `ORPHAN_REFERENCE` (observed; EDR-333
  makes a named but non-counting run hold that code even when a checklist is
  required).

## Validation

```sh
# backend/, go1.27.4 toolchain, PostGIS 127.0.0.1:32775 through orgtest, MinIO f4-offl013-s3
gofmt -l ./internal ./cmd ./db                                       # empty (toolchain gofmt)
go build ./... && go vet ./... && go vet -tags=integration ./...      # clean
go vet -tags='integration nativefixture' ./internal/modules/organization/adapters/   # clean
GOTOOLCHAIN=go1.27.1 staticcheck [-tags=integration | -tags='integration nativefixture'] ./internal/modules/organization/adapters/
    # no findings in the three builds (offl013-go-static.log)
go test -race -count=1 -tags=integration -p 2 -run '^TestSyncResilience' ./internal/modules/organization/adapters/
    # offl013-go-green.jsonl: 5 top-level + 3 subtests, all pass (43.7 s)
go test -race -count=1 -json -tags=integration -p 2 -timeout=120m ./internal/modules/organization/adapters/
    # offl013-race-union.jsonl: 672 top-level / 6074 named, 0 failures, 0 skips, 53:38 under load
GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...                      # offl013-untagged-all.log: 230 packages ok, exit 0
# repository root, JDK 17, Gradle under flock /data/dev/worktrees/.f4-gradle.lock
gradle :domain:test --tests 'com.anpfuel.domain.enterprise.*' :application:test --tests 'com.anpfuel.application.enterprise.*' \
  :data:testDebugUnitTest --tests 'com.anpfuel.data.enterprise.*' :app:testDebugUnitTest --tests 'com.anpfuel.app.ui.enterprise.*' \
  :app:assembleDebug :app:lintDebug :data:lintDebug
    # offl013-kotlin-suites-lint.log: domain 4581, application 171, data 212, app 31 tests, 0 failures/errors/skips;
    # lint 0 errors (app 354, data 17 warnings, as at the head; none in a touched source)
gradle -PenterpriseDeviceValidation=true :app:assembleDebug :app:assembleDebugAndroidTest :data:assembleDebugAndroidTest
    # offl013-validation-build.log: pass (data test APK 5f15305d…ab73, offl013-validation-apks.sha256)
python3 docs/enterprise/tools/check_plan.py                           # 0 problems
git diff --cached --check                                             # clean
```

- No SQL, migration, sqlc input, route, OpenAPI, permission, role, job or
  usage source changed: `sqlc`, the route inventory, the authorization matrix
  and the AUTH-011 walk are untouched (they ran unchanged inside the
  `organization/adapters` union; counts stay 165 / 165-33-132 / 300 / 20).
  No new Go package, so `scripts/check-manifest.txt` is unchanged. No new
  dependency.
- The Go suite is test-only; the receipt native fixture change is behind the
  `nativefixture` tag and keeps every earlier driver's behaviour (no
  `intake-plan` file, `lost` defaulting to 0, `events`/`finishes` checked only
  when given).
- Secret/PII review of the staged diff: synthetic identifiers
  (`7b0e2a54-…`, `7c000000-…`, `7d000000-…`), synthetic tokens and signature
  headers (`synthetic-access`, `synthetic-signature`), synthetic receipts (the
  existing 4×4 JPEG with a marker, device-encoded pictures in the owed native
  test); the disposable MinIO credentials appear only in local commands and
  `f4-logs`. The throwaway MinIO project was removed with `down -v`.
- Load: the host had a load average of 15–22 throughout; nothing failed only
  under load.

## Native stage on `emulator-5588` — OWED

`EnterpriseSyncResilienceNativeDeviceTest` (data androidTest, validation
build) compiles (`offl013-validation-build.log`) but was not run:
`emulator-5588` is `RUNNING_LOCKED` behind its synthetic lock-screen
credential, which this agent does not have and must not enter; no other
emulator or device was started or addressed. One-line driver, owed once the
emulator is unlocked: `/data/dev/worktrees/f4-logs/offl013-scripts/native-run.sh`
(validation APK build; `native-source.sh` = `TestReceiptNativeSourceFixture`
over an orgtest child database and the `f4-offl013-s3` bucket; `native-driver.sh`
= the four stages under `flock /data/dev/worktrees/.f4-emulator.lock`, refusing
to run while user 0 is locked; expected server effects: 1 trip with 1 event
and 1 finish, 1 fueling, 1 direct expense, 2 documents stored, links
`FUELING` 1 and `EXPENSE` 1, uploads truncate 1 / clean 2, 1 lost intake
answer, nothing in organization B).

## Named negatives → tests

| Negative case | Server half (PostGIS) | Device half (host JVM) |
|---|---|---|
| Lost response | `…FieldDay…/lost_response,…` (committed answer lost; stored bytes replayed) | `a lost answer and an answer missing an acknowledgement …`; native stage 4 (owed) |
| Wrong dependency order | `…/lost_response,…` (reversed queue: `RETRY` `DEPENDENCY_PENDING`, nothing stored, convergence); `…MissingAcknowledgement…` | `dependent operations sent before their start …`; the defect test; `EnterpriseQueueTest` and `EnterpriseOperationQueueTest` new tests |
| Soft reference before its target | `TestSyncResilienceSoftReferenceBeforeItsTargetIsHeldNotLinked` | `the trip start naming its departure run waits …` (RED → GREEN) |
| Duplicate effects | three resends in both orders; six concurrent copies behind the held root; ten reused identifiers `OPERATION_ID_REUSED` | `duplicate saves and receipts keep one operation one receipt and one projection` |
| Missing acknowledgement | `TestSyncResilienceMissingAcknowledgementStoresNothingAndRedecides` | the answer missing its last acknowledgement applies nothing |
| Killed process | whole-queue resend after the lost answer | two real child-JVM deaths (after the last save; after the server commit, before the receipt write); native stages 2 → 3 (owed) |
| Truncated upload | `TestSyncResilienceTruncatedReceiptUploadsRecoverOnlyWithTheDeclaredBytes` (`FUELING`, `EXPENSE`) | OFFL-005 host and process tests (unchanged); native stage 4 (owed) |
| Stale cross-scope state | `TestSyncResilienceCrossScopeStateNeverApplies` | `another organization's partition never sends receives or applies …` |

## Retained (with reasons; not part of this task's acceptance)

- **Native emulator stage** (above): owed until `emulator-5588` is unlocked
  by its owner; a two-physical-device run stays owed (phone excluded).
- **VAL-23 (f) `CLOCK_SKEW`**: no owner flags skew at intake and normal
  delivery delay never establishes it (OFFL-002); it needs the BR-TIME-006
  owner (EDR-353).
- **Device attachment truncation on the host**: proven by the OFFL-005
  service, store and process tests and the composed ingress tests; this suite
  adds the server half for both receipt owners and the owed native stage.
- **Soft references the device does not produce**: the mission reference on
  `trip.start` and `trip.mission.respond` have no Android producer yet; the
  EDR-352 rule is generic (any soft reference naming a queued entity) and the
  server half covers both references.
- **Hard dependencies in backoff**: a dependent operation whose declared
  parent is merely in backoff is still sent and answered `RETRY`
  `DEPENDENCY_PENDING` (OFFL-003 behaviour, convergent; asserted by the
  dependent-operation test); only references and dependents of held
  operations are held.
- **`SUSPECT` readings** of the fixture vehicle (0 installation reading) are a
  fixture property, identical in every order; plausibility tuning belongs to
  METER.
- Merged-tree verification, the cumulative PR, production and release.

## Proposed shared-doc changes

Not edited here; exact text for the coordinator.

1. `docs/enterprise/10-offline-sync.md` §10.3, after the FUEL-011 sentence:
   "A queued operation whose soft reference names the entity of another
   unacknowledged operation of the same partition waits for that operation's
   final receipt (EMP-OFFL-013, EDR-352); the resilience suite is listed in
   TEST_STRATEGY (EDR-351)."
2. `docs/enterprise/15-validation.md` VAL-23: after "Built by EMP-OFFL-013"
   add "(server half `TestSyncResilience*`, device half
   `EnterpriseSyncResilienceTest`; (f) retained, EDR-353)".
3. `data/src/main/kotlin/com/anpfuel/data/enterprise/README.md`: "EMP-OFFL-013:
   the dispatcher holds an operation naming a queued entity in a soft
   reference until that operation's final receipt (EDR-352)."
