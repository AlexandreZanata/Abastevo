# EMP-CHECK-002 — Checklist execution offline with the critical-item policy

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 task (module CHECK, P1, type APP, size M) on
`codex/enterprise-f4-check002` (worktree `/data/dev/worktrees/f4-check002`),
based on the F4 head `45c3baf3` (main `7ab0b477` + TRIP-009/013, GPS-001,
OFFL-005, TRIP-011 with 000233) and then merged with the F4 heads `0912b48a`
and `2f031276` ("Merged-tree validation"). The coordinator integrates it into
`codex/enterprise-f4` and owns PROGRESS, the F4 ledger, pushes and the PR.
Synthetic data only; no real photo, person, plate or place. The only Android
device used is the owned emulator `emulator-5588` (AVD `enterprise-f2-api36`);
the physical phone was never addressed.

Read: the EMP-CHECK-001..004 task blocks ([EP11](../backlog/ep11-maintenance.md));
[maintenance](../modules/maintenance.md) §§3–12 (BR-CHECK-001..013,
UC-CHECK-01/02/05, the checklist run state table, §7 permissions, §8 offline,
§10 codes `CHECKLIST_BLOCKING`, `CHECKLIST_EXPIRED`, `TEMPLATE_SUPERSEDED`,
SCR-CHECK-01/02); [08-state-model](../08-state-model.md) (Checklist run row);
[10-offline-sync](../10-offline-sync.md) (BR-OFFL-002, the Checklist matrix
row); [fleet](../modules/fleet.md) §6.2 (derived availability) and
BR-FLEET-027/032; the evidence of CHECK-001 (templates, policy history, the
applicable read, `checklist-v1.psv`), CHECK-003 (signed intake owner, company
handling, `SELF_APPROVAL`, the retained asset block), FLEET-007 (availability
ports), OFFL-006/FLEET-013 (reference feed and its extension point), TRIP-005
and OFFL-005 (Android producers, sealed protected-store entries, native
acceptance); EDR-177, EDR-190, EDR-213..215, EDR-238..241, EDR-242..245,
EDR-267..270 in [16-decisions](../16-decisions.md); the F4 ledger.

IDs used: B-BR-EMP-438..443, BUC-EMP-222..224, EDR-283..286 — all of them; no
pre-allocated ID is left unused. Migration `000242_*` only; the tests derive
the version from one file-name constant (`runMigration`) and build the
previous image as `name < runMigration`, asserting it reaches 000233.

## Rules frozen before behaviour

Observed before any change: no run, answer, release or `check.run.complete`
code, table, route or owner existed; `check.checklist.execute` (S for every
holder) was `NOT_YET_ROUTED`; the applicable read already returned the
version, items and policy revision a run pins; FLEET-007 availability had no
checklist source (row 4 read only manual blocks, whose command key belongs to
fleet reservations); the device plane had a company cookie channel
(`CompanyAccessClient`) and sealed protected-store entries but no checklist
code; the acknowledgement review-code vocabulary is closed and shared with the
Kotlin protocol and `operations-v1.psv`.

Rules B-BR-EMP-438..443 and use cases BUC-EMP-222..224 are the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-CHECK-002 — Checklist execution offline with the critical-item policy
(2026-11-15)"; decisions EDR-283 (template source and device cache), EDR-284
(operation, recheck, disagreement), EDR-285 (persistence, availability source,
routes) and EDR-286 (Android, photos retained) are in
[16-decisions](../16-decisions.md). Both were appended before any code. They
are the recommended routine decisions, taken unattended:

- The template reaches the device through the existing CHECK-001 applicable
  read on the account cookie plane, cached sealed with its server
  `evaluated_at`; extending the OFFL-006 feed (closed entity set, 000224
  CHECK, the 129-row oracle in two languages, the FLEET-013 cache format) is
  retained (EDR-283).
- A disagreement is stored `ACCEPTED` with both outcomes and
  `review_required`, not `ACCEPTED_FOR_REVIEW`: a new review code would change
  the shared protocol vocabulary and needs an OFFL-008 review-result owner that
  does not exist for runs; the server outcome governs at once (the block). The
  author learns the server outcome through the run read (EDR-284).
- The block is a new FLEET-007 cause kind `CHECKLIST_RUN` behind an explicit
  port, not a row in `org_asset_blocks` (whose manual removal would bypass
  `SELF_APPROVAL`). Blocks from open incident reports with `blocks_use` and
  their release without resolution are retained for EMP-CHECK-004 (EDR-285).
- Photos of not-OK items need a DOC-003 `CHECKLIST_RUN` link kind across
  DOC-001/003, OFFL-005 and two shared oracles: retained. The server never
  accepts a photo claim (`RETRY`/`UNAVAILABLE`) and the device cannot complete
  a photo-required item as `NOT_OK` and says why (EDR-286).

Deviation found by the Android half and fixed in the contract text: the run
read path is `/v1/orgs/{org_id}/checklists/assets/{asset_id}/runs/{run_id}/read`
(authorized exactly as the applicable read), as OpenAPI and the code have it.

## Shape

Backend (Go):

- Pure `maintenance/domain/checklist/run.go` (standard library):
  `ParseOutcome`, `ValidNote`, `RunFacts`/`RunClaim`/`Recheck`,
  `RecheckRun` (B-BR-EMP-440: `ErrInvalid` for an unknown version or policy
  revision, another context, a wrong answer count, a target that does not
  cover the asset, a start after the completion, an acknowledgement other
  than with `COMPLETED_WITH_ISSUES`; the shared answer rule's own errors; the
  policy in force at the completion by history, the first revision before it;
  `Superseded`; `Differs`).
- Application `maintenance/application/runs.go`: `ParseRun` (exact schema 1
  codec; `ErrMalformed`; `ErrUnsupported` for a photo declaration, attachments
  and soft references), `RunPolicy` (`check.checklist.execute`), ports
  `RunAuthority`, `RunStore`, `RecheckClaim`; company `RunCompanyPolicy` (the
  read is exactly `Policy(ApplicableRead)`, the release exactly
  `IncidentPolicy(IncidentDecide)`), `RunCompanyAuthority`, `ReleaseCommand`
  (digest binds every field), `RunView`, `RunRepository`, `RunService` (the
  decision bound before and after the repository, result validation).
- Adapters `maintenance/adapters/run_intake.go` (owner `NewRunHandler`:
  Admit/Apply/Confirm, no held path, NO_ASYNC_FOLLOWUP) and `runs.go` (intake
  store, the `ChecklistBlocks` FLEET-007 source, the company read and release
  with audit `check.run.released` on target `checklist_run`); sqlc
  `db/queries/maintenance/runs.sql` (maintenance stanza + 000242).
- HTTP `maintenance/adapters/http/runs.go`: `RunReadPath` (the applicable
  read's authorization, asset structure and SELF assignee) and
  `RunReleasePath` (`IncidentRecords`, a new `ChecklistRunStructure`).
- Fleet: `domain.ChecklistBlockFact`, `ChecklistCause` (`CHECKLIST_RUN`,
  row 4 after manual blocks), `AvailabilityFacts.Checklists`;
  `application.ChecklistBlockPort`/`ChecklistBlockAnswer`,
  missing source `CHECKLIST_RUN`; `AssetStore.ChecklistBlocks` (nil fails
  closed), `NoChecklistBlocks`; missing sources sorted.
- Organization adapters: `run_authority.go` (`RunAuthority` over the meter
  authority with the EDR-177 rule for `check.checklist.execute`;
  `RunCompanyAuthority`), the own-record intake checks of
  `ReportAuthority` and the company check of `IncidentAuthority` extracted as
  shared helpers (`checkRecordTx`, `classifyRecordTx`, `checkTx`) used by both
  owners, `ChecklistRunStructure` in `resource_structure.go`.
- Composition (`enterprisesuite`): the run owner in the intake registry with
  its no-async plan, the two routes, the audit definition, the
  `ChecklistBlocks` source on the asset store.
- Migration `000242_organization_checklist_runs.sql` (EDR-285): forced-RLS
  `org_checklist_runs`, `org_checklist_run_answers`,
  `org_checklist_run_releases`; insert guard under the root and asset locks
  (the exact operation of an ACTIVE enrolled author, non-DRAFT asset,
  allocation at the declared instant, template of the same context, revision
  1); answers only before the run's receipt; the only update is the release
  (`BLOCKING` → `COMPLETED_WITH_ISSUES`, revision 2) written by its row; release
  source with the separation of releaser and author by account, membership and
  canonical employee; a deferred database recheck (every item answered once,
  the answer rule, the target covering the asset, the in-force policy
  revision, the outcome, both flags) with publication only with the
  `ACCEPTED` receipt at revision 1, and the release only with its audit event;
  the asset (000232 body), unit and cost-centre (000231 bodies) usage sources
  with the three run keys as associations.
- Contract: two paths, seven schemas (`EnterpriseChecklistRunOutcome`,
  `…RunAnswer`, `…RunPayload`, `…RunReadRequest`, `…RunReleaseRequest`,
  `…Run`, `…RunRelease`), the `check.run.complete` owner and its description
  in `/sync/operations`, the availability cause kind and missing source
  `CHECKLIST_RUN` (`contracts/openapi/v1.yaml`); inventory
  `authorization-routes-v1.json`: `fleet.asset.read` +1 route,
  `check.incident.manage` +1 route (both already routed with matrix
  representatives, so the matrix is unchanged); router test paths +2; the
  AUTH-011 walk gains a positive read (404 for a run that does not exist; the
  stored-run read is proven by the run tests) and a FORBIDDEN release;
  signed operation owners +1.
- Shared oracle `contracts/testdata/enterprise/checklist/run-v1.psv` (56 rows:
  OK 15, MALFORMED 37, UNSUPPORTED 2, PROTOCOL 2; SHA-256
  `fbbea99443fd5aadf49f7d55bf132b34208d2ee7e545188a078af56dff14a24e`) from an
  independent oracle (`f4-logs/check002-scripts/gen_run_vectors.py`),
  documented in the checklist README; Go consumer
  `maintenance/application/run_vectors_test.go`, Kotlin consumer
  `EnterpriseChecklistRunVectorsTest`.

Android (B-BR-EMP-438/443, EDR-283/286):

- Domain `domain/.../enterprise/maintenance/checklist/ChecklistRuns.kt`: the
  `check.run.complete` builder (exact oracle bytes through the existing seal)
  and strict decoder; the `checklist-book` v1 sealed RECORD (cached applicable
  reads with `evaluated_at`, runs in progress with every answer, completed runs
  with their operation, device outcome and the server read; bounds 200 runs /
  200 templates; closed local failures `CHECKLIST_EXPIRED`,
  `ANSWER_NOT_ALLOWED`, `PHOTO_REQUIRED`, `INCOMPLETE`,
  `ACKNOWLEDGEMENT_REQUIRED`); presentation. It reuses `Checklist.evaluate`.
- Application `application/.../enterprise/checklist/Checklists.kt`: store and
  gateway ports, the `check.run.complete` producer (book written in the
  queue's own write), service and `ChecklistFlow` (answers written before they
  show as saved; a failed write says nothing was saved; after an `ACCEPTED`
  receipt the run is read and the server outcome stored; a `BLOCKING` server
  result is re-read, at most 5 reads per refresh).
- Data: `ProtectedChecklistStore` (one `queueAccess` snapshot; an unreadable
  or foreign book is unavailable, never replaced), `EnterpriseChecklistHttpGateway`
  (strict parsing of the applicable read and the run read on the cookie
  plane), `EnterpriseChecklistModule`; the producer is registered with the
  trip producers in `EnterpriseModule`.
- App: `ChecklistRunScreen`/`ChecklistViewModel` from the enterprise context
  ("Checklists" → FLEET-013 picker → context) and from the trip start form
  (departure line; "Salvar início" disabled while the latest valid departure
  run of the vehicle is `BLOCKING`, a local effect only); one item per row,
  critical marks, "Não se aplica" only on optional items, note ≤ 140, progress,
  confirmation date and time of the cached template, empty, not-downloaded and
  expired states; "Bloqueando o uso" in text and icon with the blocking items;
  "Estou ciente das pendências"; "A empresa recalculou o resultado" with both
  outcomes; the photo notice. Rollback flag `ENTERPRISE_CHECKLIST_UI_ENABLED`
  (default on). 66 strings × 8 locales (`enterprise_checklists.xml`), generated
  by `f4-logs/check002-scripts/gen_checklist_strings.py`. Existing tokens only:
  black header/bars, white containers, no grey fill.
- Native fixture (test-only, tags `integration nativefixture`):
  `organization/adapters/orgsync_checklist_native_fixture_test.go`.

## RED

Logs in `/data/dev/worktrees/f4-logs/check002-*`.

- Go pure recheck (zero-value stubs returning a not-implemented error)
  `go test -race -json ./internal/modules/maintenance/domain/checklist/` →
  `check002-go-domain-red.jsonl`: 7 new top-level and 35 new subtests failed;
  the 8 passing top-level tests and 127 subtests are CHECK-001's.
- Go codec (`ParseRun` stub) `-run TestSharedRunOperationVectors` →
  `check002-go-codec-red.jsonl`: 54/56 vectors failed; the two `PROTOCOL` rows
  passed because they exercise only the shared envelope profile. (A first run
  showed the oracle had classed two envelope-refused rows as MALFORMED; the
  oracle reclassified them `PROTOCOL` before any parser existed.)
- Fleet availability (the checklist rows ignored) →
  `check002-fleet-domain-red.jsonl`: the new
  `TestAvailabilityBlocksOnABlockingChecklistRun` failed, the two existing
  availability tests passed.
- Contract (spec at the base) → `check002-contract-red.jsonl`: both new tests
  failed (owner count 11; undocumented run route); the full apicontract
  package failed 34 actual-router tests on the undocumented routes
  (`check002-apicontract-red-all.jsonl`).
- Router (routes wired, inventory at the base) → `check002-cmd-red.jsonl`: 3
  top-level failed (inventory, company composition, full router).
- PostGIS schema RED (migration moved out) `go test -json -race
  -tags=integration -run '^TestChecklistRun'` → `check002-db-red-schema.jsonl`:
  8/8 failed (42P01 at the grants; the upgrade test on the missing file).
- PostGIS behaviour RED (migration present; `RunTemplateTx`, `InsertRunTx`,
  `ReadRunTx`, `ChecklistBlocksDBTX`, `ReadRun`, `ReleaseRun` stubbed) →
  `check002-db-red-behaviour.jsonl`: 8/8 failed at their first behaviour check
  (a legitimate run answered `MALFORMED`); 4 refusal subtests that act before
  the store (foreign/unknown asset, unassigned driver) passed and are not
  claimed as RED.
- Android (`TODO()` stubs) → `check002-android-red.log`, XML in
  `check002-android-red-xml/`, summary `check002-android-red-summary.txt`:
  vectors 55/58, book 8/8, flow 6/6, gateway 4/4, store 4/4, process death 1/1,
  ViewModel 5/5 failed; the three passing vector methods (two `PROTOCOL` rows,
  the corpus check) read no builder and are not claimed.

The adapters, migration and PostGIS tests were written before these RED runs
(the CHECK-003 precedent); the RED was taken against stubbed entry points with
the frozen tests and the migration as written. No expectation was changed to
pass.

## GREEN and REFACTOR

- Go domain → `check002-go-domain-green.jsonl`: 15/15 top-level, 162 subtests
  under race, 99.4 %; REFACTOR removed an unreachable branch of `covers` and
  added `TestValidNoteIsThePresentNoteProfile` → 16/16, 100.0 %
  (`check002-go-domain-refactor.{jsonl,out}`).
- Go codec → `check002-go-codec-green.jsonl`: 56/56.
- Fleet domain → `check002-fleet-domain-green.jsonl`: 35/35, 163 subtests.
- Contract → `check002-contract-green.jsonl`: apicontract 96/96 top-level, 852
  subtests; router `check002-cmd-green.jsonl` 22/22.
- PostGIS focused (`-race -tags=integration`, ordinary NOSUPERUSER /
  NOBYPASSRLS runtime role): first run `check002-db-green1.jsonl` 4/8 — four
  expectations were wrong for the fixture, none weakened: the start check one
  second before the run used an instant before the fixture's own assignment
  began (removed; the cause's start equals the declared completion is still
  asserted); a dependency and a photo declaration answer `ErrUnavailable`
  through the store (the intake service turns it into `RETRY`) — nothing
  stored is still asserted; `TRUNCATE` was executed with arguments; the
  upgrade fixture keeps an in-progress trip, so the vehicle is `IN_USE` with
  the run as a further `BLOCKED` cause. Then 8/8, 9 subtests
  (`check002-db-green.jsonl`).
- The affected union found 7 failures caused by this task, fixed without
  weakening (`check002-db-union.jsonl`, rerun `check002-db-union-rerun.jsonl`
  7/7): the unit and cost-centre schema fixtures carry their own copy of the
  usage sources (the FUEL-002 trap) and now list the run keys; the CHECK-001
  schema test truncates template versions with `CASCADE`, which now reaches
  the runs, so it also grants `TRUNCATE` on the three run tables to keep
  reaching the append-only guard (`55000` unchanged).
- Android GREEN (`check002-android-domain-green-1.log`,
  `check002-android-host-green-{1,2}.log`): domain 66/66 on the first run;
  application 5/6 then 6/6 (a local refusal now re-reads the book so an expired
  run shows as expired); data 9/9 after one compile fix; ViewModel 5/5. Two
  application expectations were inconsistent with the frozen rule and were
  corrected without weakening: the shared rule reports `PHOTO_REQUIRED`
  before `INCOMPLETE` (both still asserted), and a fixture made the second
  applicable read older than the first (the server's `evaluated_at` only
  grows).
- Added after GREEN as characterization (not counted as RED): adapters
  `TestRunHandlerNeedsEveryPortAndHasNoHeldPath`,
  `TestRunAuditActionIsTheRelease`,
  `TestRunStoreIntakeAndBlockSourceRefuseWithoutTransaction`; application
  `TestRunPoliciesAreFixed`, `TestReleaseCommandValidatesAndDigests`.

## Acceptance and negatives mapped to tests

| Item | Evidence |
|---|---|
| The local outcome is visible | Android book/flow/ViewModel tests; presentation device test (7/7): "Bloqueando o uso" in text and icon with the blocking items, acknowledgement gate, trip start disabled while `BLOCKING`; native stage 3 |
| The server rechecks it (BR-CHECK-007) | `TestChecklistRunIntakeRechecksStoresAndBlocks` (exact stored fact with server policy revision, recheck and flags; answers stored as declared), Go `TestRecheck*`, the database's deferred recheck (`TestChecklistRunSchemaIsForcedRLSAndGuarded`); native stage 4 (server agrees) |
| A disagreement preserves the field fact (BR-CHECK-008) | `TestChecklistRunDisagreementKeepsTheFieldFact`: stale WARN revision after BLOCK in force → stored `BLOCKING`, device `COMPLETED_WITH_ISSUES`, `outcome_differs`, the vehicle blocked; stale BLOCK while WARN in force → server `COMPLETED_WITH_ISSUES`, differs; the author reads both outcomes; native stages 5–7 ("A empresa recalculou o resultado", nothing refused) |
| Unpersisted answers reported saved | Android flow test (a failed write reports nothing saved), store tests (sealed, no plaintext), `ProtectedChecklistProcessTest` (process death keeps the run), native stage 2/3 (force-stop between answers) |
| Wrong or superseded template | `TestChecklistRunIntakeRefusals/malformed`: unknown version, unknown policy revision, unknown template, other context, a RETURN template as DEPARTURE, a template whose target names another asset → `MALFORMED`; superseded version flagged `TEMPLATE_SUPERSEDED` and accepted (disagreement test); Go `TestRecheckFlagsASupersededTemplate`, `TestRecheckTargetsCoverTheAsset` |
| A critical block ignored | A device `COMPLETED_OK` with a critical `NOT_OK` under BLOCK is stored `BLOCKING`, differs, and blocks the vehicle (intake test); Go `TestRecheckStoresTheServerOutcomeWhenTheDeviceDisagrees`; database recheck refuses a forged `recheck_outcome` |
| A server disagreement dropping the record | Every well-formed run is `ACCEPTED` and stored with its answers (counts asserted); runs never update except by release; answers immutable (schema test) |
| Foreign organization / asset | foreign asset and unknown asset → transport `NOT_FOUND`, nothing stored; another organization's scope sees no run, answer or release; a run read through another asset → not found |
| Replay / `OPERATION_ID_REUSED` | byte-identical ACK on resend (×2), another digest `OPERATION_ID_REUSED`, a second operation for a stored run `MALFORMED`; 12 concurrent identical deliveries → one run, one ACK (`TestChecklistRunConcurrentDuplicateDeliveries`); release replay `replayed=true`, another reason `OPERATION_ID_REUSED` |
| Authorization (V-AUTHZ) | intake: unassigned SELF driver `FORBIDDEN`, assigned driver accepted; release: the author `SELF_APPROVAL` (store) and 23514 (database, owner bypassed); a SUPERVISOR of another unit and an AUDITOR refused; a FLEET_MANAGER of the run's unit releases; the AUDITOR reads (fleet.asset.read) and the out-of-scope supervisor cannot read; contract binding tests for both routes; the AUTH-011 walk |
| Release (BR-CHECK-011) | one release row and one `check.run.released` audit event, outcome `COMPLETED_WITH_ISSUES` at revision 2, answers unchanged, the vehicle `AVAILABLE` again; a stale revision, a second release and a non-`BLOCKING` run `CONFLICT`; blank reason `VALIDATION_FAILED`; unknown run not found |
| Race (V-IDEM) | `TestChecklistRunReleaseRaceLeavesOneReleaseInBothOrders`: two releases queued behind a held root lock (both observed waiting) — one wins, the other `CONFLICT`, one row and one audit event, both orders |
| Block on the asset (BR-CHECK-010) | intake test: `BLOCKED` with the `CHECKLIST_RUN` cause from the declared completion, a new start held `ASSET_UNAVAILABLE`; released → `AVAILABLE`; fleet domain test; fail closed without the port (`fleet_reservation_integration_test`) |
| Photos | a `photo` declaration stays unavailable with nothing stored; the device cannot complete a photo-required `NOT_OK` and says why (EDR-286) |
| Forced RLS, guards | `TestChecklistRunSchemaIsForcedRLSAndGuarded`: answer update/delete, outcome and recheck rewrites, a release without its row, run delete and truncates refused; a late answer refused; another scope sees nothing; usage sources answer |
| Populated upgrade 000233 → 000242 | `TestChecklistRunPopulatedUpgradeFailureRecoveryAndPreviousImage`: completed and in-progress trips, an ACTIVE template; injected failure leaves schema, the three usage functions and data unchanged; ordinary migrator (REFERENCES + function ownership); recovery once; no-op replay; previous image compatible; forced-RLS ownership; every previous allow-list line kept with the run keys; the pre-existing template takes a `BLOCKING` run |

## Validation (task tree, before the merge)

Environment: worktree backend, Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f4-go-build`,
`GOTMPDIR`/`TMPDIR=/data/dev/cache/f4-tmp`; shared PostGIS control
`127.0.0.1:32775` used only through orgtest child databases; a throwaway MinIO
project `f4-check002-s3`; Kotlin/Gradle through the shared flock with JDK 17.

- Toolchain `gofmt -l ./internal ./cmd ./db` empty; `go build ./...`,
  `go vet ./...`, `go vet -tags=integration ./...` and
  `go vet -tags='integration nativefixture'` on organization adapters clean.
- staticcheck (go1.27.1) on maintenance, fleet, organization,
  enterprisesuite, apicontract, cmd/api, the maintenance query package and with
  `-tags=integration` on maintenance, fleet, organization adapters, orgtest: no
  findings (`check002-staticcheck.log`; native fixture tags:
  `check002-android-staticcheck.log`).
- `sqlc vet` and `sqlc diff` clean.
- OpenAPI `vacuum lint`: 0 errors; 1,071 warnings / 666 informs vs 1,067 / 665
  at the base (house-style descriptions on the new schemas;
  `check002-vacuum.log`, `check002-vacuum-head.log`).
- Affected race/integration union `go test -json -race -tags=integration
  -count=1 -timeout=90m -p 2 ./internal/modules/organization/adapters
  ./internal/modules/{maintenance,fleet,orgsync,trip}/... ./internal/platform/...
  ./cmd/api/...` → `check002-db-union.jsonl`: 1,257 top-level / 8,669
  subtests, 7 failures caused by this task (see GREEN), all fixed and rerun
  green; the full union was repeated on the final merged tree below.
- Android (from the Android half): enterprise host suites domain 4160,
  application 134, data 204, app 28 tests, 0 failures, errors or skips
  (`check002-android-suites-lint-final.log`); `:app:assembleDebug
  :app:lintDebug :data:lintDebug` 0 errors, warnings unchanged (app 352, data
  17), none in touched sources; enterprise validation builds pass
  (`check002-android-validation-build-{2,3}.log`; the first failed because the
  validation graph replaces the invitation module, fixed by a separate
  `EnterpriseChecklistModule` and a test binding).
- `python3 docs/enterprise/tools/check_plan.py` → 0 problems (348 tasks, 545
  rules, 167 use cases, 145 screens); `git diff --check` clean; staged diff
  reviewed for secrets and personal data (synthetic identifiers and
  Portuguese sample texts only; the disposable MinIO credentials live only in
  the log environment file).

## Native acceptance on `emulator-5588`

Acceptance, not RED: written after the screens and services. Real signed
backend (`check002-android-native-source.log`, `…native-device.log`,
`…native-driver.out`, driver `f4-logs/check002-scripts/native-driver.sh`):
`TestChecklistNativeSourceFixture` PASS (223.4 s) with
`EnterpriseChecklistNativeDeviceTest` in seven processes, only the target app
force-stopped between stages:

1. Online: the applicable template cached (Luzes optional, Freios critical,
   Pneus; BLOCK revision 1).
2. Offline: run started, item 1 answered.
3. New process after force-stop: the answer is there; Freios `NOT_OK` with a
   note; completion shows `BLOCKING` with Freios as the blocking item, the
   trip start is disabled, the run is queued, nothing sent, nothing outside
   the sealed store.
4. Reconnected: `ACCEPTED`; the run read shows the server agrees (`BLOCKING`).
5. The server switches the policy to WARN (revision 2); the device caches it.
6. Offline; the server switches to BLOCK (revision 3); completion first asks
   for the acknowledgement, then completes locally `COMPLETED_WITH_ISSUES`.
7. Reconnected: `ACCEPTED`; the server outcome `BLOCKING` with the difference
   ("A empresa recalculou o resultado"), the trip start disabled; nothing
   refused.

Server counts exact: intake [200, 200], applicable [200, 200], run read
[200, 200, 200]; 2 runs, 6 answers, 2 operations, 2 `ACCEPTED` receipts;
stored `BLOCKING:BLOCKING:BLOCKING:false:false:1:1:1` and
`COMPLETED_WITH_ISSUES:BLOCKING:BLOCKING:true:false:2:3:1`. The partition,
identity and session keys, the private fixture and the route were removed
afterwards. Presentation `EnterpriseChecklistPresentationDeviceTest` 7/7 in
pt-BR (the first run 6/7: one assertion read text on a card container; it now
reads the card's children with the same expected text); regressions on the
same APKs: trip presentation 6/6, My Assets presentation 6/6, the five
`EnterpriseContextDeviceTest` methods (`check002-android-native-*`, APK hashes
`check002-android-validation-apks.sha256`).

## Proposed shared text (for the coordinator)

- `data/.../enterprise/README.md`: "EMP-CHECK-002 registers
  `check.run.complete`; its projection is one sealed `checklist-book` v1 RECORD
  per partition, holding cached applicable reads, answers written without an
  operation, and completions written in the queue's own write."
- Backend README production grants (runtime role, as the fixtures):
  `SELECT, INSERT` on `org_checklist_runs`, `org_checklist_run_answers`,
  `org_checklist_run_releases`; `UPDATE(outcome, revision, updated_at)` on
  `org_checklist_runs`; `SELECT` on the template tables it reads.
- [10-offline-sync](../10-offline-sync.md) §10.3 Checklist row: the server
  stores its outcome and flags a disagreement for review (EDR-284).

## Retained

- The trip gate: a trip start naming its departure run, `CHECKLIST_REQUIRED`
  and `CHECKLIST_BLOCKING` holds, the run validity at the start
  (`CHECKLIST_EXPIRED` on the server) and return runs linked to the finish
  (EMP-TRIP-019).
- Photos of not-OK items: a DOC-003 `CHECKLIST_RUN` link kind through the
  OFFL-005 session (DOC-001/003, two shared oracles); until then the server
  answers a photo declaration `RETRY`/`UNAVAILABLE`.
- Damages from `NOT_OK` answers and the link to known open damages
  (BR-CHECK-009); blocks from open incident reports with `blocks_use` and a
  release without resolution (EMP-CHECK-004); the panel lists of runs and of
  held (`review_required`) runs and the review acknowledgement (EMP-CHECK-004).
- The OFFL-006 reference-feed entity type for templates (EDR-283).
- Run events (`check.run.completed|blocked|released`) and notifications to
  handlers; protocol-level `ACCEPTED_FOR_REVIEW` for disagreements (needs a
  shared review code and an OFFL-008 review-result owner).
- `data_mode` is `SYNTHETIC` only in the contract and the gateway enforces it;
  real data needs a contract change and the legal gates (LV-09/10).
- OWED (Android half): an end-to-end run through the real Hilt UI on the
  device; a release made while the device watches (only the re-read after a
  release is tested, against a fake server); two devices showing the block to
  a second member; visual and accessibility review (dark mode, 200 % font,
  TalkBack); retention of the checklist book beyond its bounds.
- Production grants (README subsection) and live composition review.
- No milestone, phase or gate is claimed.

## Merged-tree validation

The task commit `3e91dab5` was merged twice with `codex/enterprise-f4`:

1. `0912b48a` (TRIP-007/014/018 with 000235..000237, GPS-004/012 with
   000240, UGC safety 000239) — merge commit `9a5ba2b7`. Conflicts resolved
   as unions: EDR rows by number, contract sections, the sync owners
   description, the structure kinds (`ChecklistRunStructure` and
   `HeldTripStructure`), the intake no-async plans; counts 155 normal routes
   (routed 30 / not routed 54 / specials 26), AUTH-011 walk 155 / 32 positive
   / 123 FORBIDDEN, matrix unchanged (300 cells). None of 000235..000240
   redefines a usage source. On that tree the Android host suites passed
   (domain 4160, application 134, data 204, app 28, 0 failures;
   `:app:assembleDebug :app:lintDebug :data:lintDebug` 0 errors, app warnings
   354 vs 352 before the merge — the two new ones are in the head's own files,
   none in a file this task touched; `check002-android-merged.log`).
2. `2f031276` (EXP-002 with 000244) — merge commit `3fdab0b8`. 000244 runs
   after 000242 and redefines `org_asset_current_usage`,
   `org_unit_current_usage` and `org_cost_centre_current_usage`, so the three
   run keys of 000242 are merged into the 000244 bodies (one line each, every
   EXP-002 line kept); 000242 keeps its own redefinition for an image that
   stops at it (the 000242 upgrade rehearsal). The coordinator owns 000244 and
   may instead move the three lines to a later migration (see the final
   report). The unit and cost-centre schema fixture copies list both the
   expense and the run keys; signed operation owners 20.

On the final merged tree `3fdab0b8`:

- Toolchain gofmt empty; `go build ./...`, `go vet ./...`, `go vet
  -tags=integration ./...`, `go vet -tags='integration nativefixture'` on
  organization adapters clean; `sqlc vet` and `sqlc diff` clean; staticcheck
  (go1.27.1) plain, `-tags=integration` and with the native fixture tags on
  the touched packages: no findings (`check002-staticcheck-merged.log`).
- Race/integration union `go test -json -race -tags=integration -count=1
  -timeout=150m -p 2 ./internal/modules/organization/adapters
  ./internal/modules/{maintenance,fleet,orgsync,trip,tracking,expense}/...
  ./internal/platform/... ./cmd/api/...` with PostGIS and the throwaway MinIO
  (`check002-db-union-merged.jsonl`): 46 packages, 42 pass and 4 have no
  tests; 1,387 top-level / 10,075 subtests pass, 0 failures, 0 skips; 45.7 min
  wall, `organization/adapters` 2,706 s. (A first merged union on `9a5ba2b7`
  was stopped when the EXP-002 head arrived; it is not counted.)
- Untagged whole module `GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...`
  (`check002-untagged-all.log`): exit 0, 229 packages ok, 87 without tests,
  0 FAIL.
- Kotlin `:domain:test --tests 'com.anpfuel.domain.enterprise.*'`: 4204 tests,
  0 failures (`check002-kotlin-domain-merged2.log`); EXP-002 changed only
  Kotlin domain files, so the application, data and app suites of the first
  merged tree stand.
- OpenAPI vacuum: 0 errors; 1,113 warnings / 682 informs vs 1,118 / 681 at
  the head (the description rules count differently with the added schemas;
  `check002-vacuum-merged2.log`, `check002-vacuum-f4head2.log`).
- `check_plan.py` 0 problems; `git diff --check` clean.
