# EMP-TRIP-019 — Checklist gate at trip start and finish

Date: 2026-11-16.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 task (module TRIP, P1, type API, size S) on
`codex/enterprise-f4-trip019` (worktree `/data/dev/worktrees/f4-trip019`),
first commit of the branch, based on the F4 head `03c803c4` (TRIP-023
integrated: latest trip bodies 000243/000249, usage functions 000244,
CHECK-002 runs 000242). Real dependencies used: the EMP-TRIP-002/011/012/014
signed start and finish owners, the EMP-TRIP-023 mission reference and its
precedent, the EMP-TRIP-007 held-fact queue, the CHECK-001 templates and
applicable-template rule, the CHECK-002 runs, recheck, block period (EDR-285)
and release, the FLEET-007 start decision. Nothing is faked; synthetic data
only. No route, panel, deployment, emulator or device was used.

Read: AGENTS, FAST_EXECUTION, 00-conventions; the EMP-TRIP-019, TRIP-009,
TRIP-010, TRIP-023 blocks of [EP06](../backlog/ep06-trips.md);
[trips](../modules/trips.md); [maintenance](../modules/maintenance.md) §§4
(BR-CHECK-001..013), 5 (UC-CHECK-02), 6, 8, 10; [10-offline-sync](../10-offline-sync.md)
BR-OFFL-004/006/007, §§10.1–10.3; EDR-262..266 and 283..286 in
[16-decisions](../16-decisions.md); the F4 ledger and the evidence of
CHECK-001, CHECK-002 (its retained trip gate), TRIP-002, TRIP-003, TRIP-005,
TRIP-007, TRIP-011, TRIP-012, TRIP-014, TRIP-023 and FLEET-007.

IDs used: B-BR-EMP-511..516, BUC-EMP-261..263, EDR-331..334 — all of them.
Migration `000255_organization_trip_checklist_gate.sql` (coordinator
assignment; EMP-TRIP-010 moves to 000256 and 000250 stays unused). The
upgrade test derives the version from one file-name constant
(`checklistGateMigration`) and builds the previous image as
`name < 000255` (asserting it contains 000242, 000244 and 000249).

## Rules frozen before behaviour

Observed before any change: `ParseStart` admitted only one `mission` soft
reference and `ParseFinish` none (`RETRY`/`UNAVAILABLE`); the device builders
sent no reference and `trip-operations-v1.psv` had none; no code, table or
read related a trip to a checklist run; a BLOCKING run already held a new
start `ASSET_UNAVAILABLE` through the FLEET-007 `CHECKLIST_RUN` cause from its
declared completion until its release was recorded; the acknowledgement
review-code set is closed in the shared protocol (Go `reviewCodes`, the
Kotlin decoder, `operations-v1.psv`) and has no `CHECKLIST_*` code; the
CHECK-001 applicable read resolves the current ACTIVE templates; the policy
history (`validity_seconds` 1–72 h) has an `effective_at` per revision; the
release takes the organization root lock in its company authority check
(`LockLegalOrganization … FOR UPDATE`) before the run row lock.

Rules B-BR-EMP-511..516 and BUC-EMP-261..263 are the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-019 — Checklist gate at trip start and finish (2026-11-15)";
decisions EDR-331 (soft reference `checklist_run`, Android builders,
`trip-operations-v2.psv`), EDR-332 (gate semantics), EDR-333 (closed
vocabulary plus stored reason) and EDR-334 (persistence, port, lock order,
migration number) in [16-decisions](../16-decisions.md). They are the
recommended routine decisions, taken unattended:

- **Reference, not schema 2 (EDR-331).** The start names its departure run
  and the finish its return run through one envelope soft reference
  `{type: "checklist_run", id}`; payloads and operation schema 1 unchanged.
- **Gate (EDR-332).** Required from the current CHECK-001 applicable
  DEPARTURE template (never the device cache, never a backdatable history);
  a named run counts when it is the start author's own unlinked DEPARTURE run
  of the asset; it is judged at the declared start instant by history:
  BLOCKING if its recheck was BLOCKING and its release was recorded after
  that instant (the EDR-285 period), valid in `[completion, completion +
  validity)` with the validity of the run template's policy revision in force
  at that instant. First failure first: `CHECKLIST_REQUIRED`,
  `ORPHAN_REFERENCE`, `CHECKLIST_BLOCKING`, `CHECKLIST_EXPIRED`.
- **Vocabulary (EDR-333).** No acknowledgement code is added (an unknown
  code would make installed devices refuse the ACK; EDR-284 precedent): a
  failed gate holds `ASSET_UNAVAILABLE` (required/blocking/expired) or
  `ORPHAN_REFERENCE` (a named run that does not count), after the existing
  held codes and before the mission reference; the specific reason is stored
  beside the held start whatever code won.
- **Return (EDR-332, BR-CHECK-013).** A finish links its counting RETURN run
  (whatever its outcome) and is never held for it; anything else links
  nothing.
- **Lock order (EDR-334).** The gate reads runs, releases and policy history
  without row locks under the root lock the start already holds; run inserts
  (source trigger) and releases (company authority) take the root first, so
  the release code is unchanged.

## Shape

- Trip application `trip/application/checklist_gate.go`:
  `ChecklistRunReferenceType`, contexts, reasons, `ChecklistRun`,
  the `ChecklistGate` port (`DepartureRequiredDBTX`, `ChecklistRunDBTX`),
  `GateDecision`, `DepartureGate`, `ReturnLinkable`; `StartInput.DepartureRunID`,
  `FinishInput.ReturnRunID`; `Store` gains `ChecklistLinkedTx`,
  `ChecklistLinkTx`, `LinkChecklistTx`, `HoldChecklistTx`; `ParseStart` admits
  at most one mission and one checklist run, `ParseFinish` at most one
  checklist run.
- Trip adapters: both owners take the gate port (`NewStartHandler(…, gate,
  store)`, `NewFinishHandler(…, gate, store)`); the start decides the gate
  after the FLEET-007 decision, holds with the closed codes and stores the
  reason, links the run after the trip (and mission); the finish links a
  counting return run after the finish; `Confirm` re-reads the link of a
  named run; `checklist_links.go` writes links and reasons with raw
  parametrized SQL (the trip stanza does not include checklist tables).
- Maintenance adapters `trip_gate.go`: `TripChecklistGate` (the rehearsal-only
  `PreGateImage` flag, production false) over the shared helper
  `applicable` (extracted unchanged from `ReadApplicable`, which now calls
  it) and the new sqlc query `ChecklistRunGateFacts` plus the existing
  `ChecklistPolicyHistory` (`db/queries/maintenance/runs.sql`; stanza
  unchanged).
- Composition `enterprisesuite/sync.go`: one `maintpg.TripChecklistGate{}`
  for both owners.
- Migration 000255 (EDR-334): `org_trip_checklist_validity`,
  `org_trip_checklist_blocking_at`; insert-only forced-RLS
  `org_trip_checklist_links` (trip/context key, unique run, unique operation,
  keys to trip, run, operation; the source trigger re-proves departure and
  return) and `org_trip_checklist_holds` (operation key, keys to the held
  start, its review item and the run; the source trigger proves BLOCKING and
  EXPIRED from history); immutability triggers; no existing function, table
  or row changes, no usage-function change (no key to assets, units, cost
  centres or employees).
- Contract: no route; the OpenAPI descriptions of the trip.start and
  trip.finish payloads name the references; inventory, matrix, AUTH-011
  walk and signed owner counts unchanged (162/30/54/26; 162/33/129; 300; 20).
- Shared operation table `contracts/testdata/enterprise/trip/trip-operations-v2.psv`
  (61 rows; every v1 row byte for byte plus 7; SHA-256
  `08e0c63235a470ce11756f9700f99d29ec640cf03e8803e3e845e6341dda04ef`) from
  `f4-logs/trip019-scripts/gen_trip_operation_vectors_v2.py`, which still
  regenerates v1 byte for byte; Go `TestSharedTripOperationVectors` and
  Kotlin `EnterpriseTripOperationVectorsTest` both hash-check it.
- Android: `TripOperations.start(…, departureRunId)` /
  `finish(…, returnRunId)` (optional, sealed as the one reference) and the
  strict decoder (exactly one `checklist_run` reference, else
  `UNSUPPORTED`; `TripOperationFact.Start.departureRunId`,
  `Finish.returnRunId`); `ChecklistPresentation.departureRun` (the latest
  valid departure view when it does not block); `EnterpriseTripService.start`,
  `TripFlow.start`, `TripViewModel.start` gain an optional run;
  `EnterpriseContextScreen` passes `departureRun(renderedChecklist, asset)`
  behind `ENTERPRISE_CHECKLIST_UI_ENABLED`. No string, layout or screen
  changes; the finish form's return line is retained.

## RED

Logs in `/data/dev/worktrees/f4-logs/trip019-*`. Tests were written against
the compiled shape (codec unchanged, `DepartureGate`/`ReturnLinkable`
zero-value, store and gate adapter stubs, the migration as written).

- Go application `go test -race -json ./internal/modules/trip/application/`
  → `trip019-go-app-red.jsonl`: 4 top-level failing
  (`TestDepartureGateDecidesTheReasonFirstFailureFirst`,
  `TestParseStartAndFinishReadTheChecklistRunReference`,
  `TestReturnLinkableChecksIdentityAndTheTripWindow`,
  `TestSharedTripOperationVectors` with exactly its 4 new OK rows failing),
  42 top-level / 104 subtests passing.
- PostGIS behaviour RED 1 (codec stub) → `trip019-db-red-behaviour.jsonl`:
  7/7 top-level failing (every reference `RETRY`/`UNAVAILABLE`).
- PostGIS behaviour RED 2 (codec and decision real, store and maintenance
  gate stubbed) → `trip019-db-red-behaviour2.jsonl`: 7/7 top-level and 6
  subtests failing at their first gate assertion (no reason stored, no link,
  wrong held code).
- Kotlin (`TripOperations` parameters accepted and ignored, decoder
  unchanged) → `trip019-kotlin-red.log`: `EnterpriseTripOperationVectorsTest`
  63 tests, 7 failing — exactly the 7 new rows.

No expectation was changed to pass.

## GREEN and REFACTOR

- Go unit `-race` trip and maintenance → `trip019-unit-green.jsonl`: 145
  top-level / 1,320 subtests, 0 failures.
- PostGIS focused `-race -tags=integration -run '^TestTripChecklist'`
  (ordinary NOSUPERUSER/NOBYPASSRLS runtime role) → `trip019-db-green1.jsonl`:
  7/7 top-level, 30 subtests, first run.
- Lock-order check: with an explicit root lock added to `ReleaseRun` and then
  with it removed (`trip019-db-red-lockorder.jsonl`), the release race passes
  in both orders, because the release's company authority already locks the
  organization root before the run row; the redundant edit was reverted and
  the release code is unchanged.
- REFACTOR: `applicable` extracted from `ReadApplicable` (behaviour
  unchanged, covered by the CHECK-001 tests in the union).

- Kotlin (Gradle flock, JDK 17) → `trip019-kotlin-green.log`: domain
  enterprise suite 4,301 tests and application enterprise suite 135 tests, 0
  failures (vectors 63/63, the book test with `departureRun`, the new trip
  flow test); app enterprise unit tests (compiling the app with the screen
  wiring) 28 tests, 0 failures (`trip019-kotlin-app.log`).

## Acceptance and negatives mapped to tests

| Item | Evidence |
|---|---|
| The departure decision follows current policy (AC) | `TestTripChecklistGateLinksAValidDepartureRunOnce`: no template → starts pass as before; ACTIVE DEPARTURE template → a valid own run links, nothing named holds `ASSET_UNAVAILABLE`/`CHECKLIST_REQUIRED`; the policy changed to one hour after the device's run (cached 12 h) → `CHECKLIST_EXPIRED` (`TestTripChecklistGateBlockingExpiredAndTheServerPolicy`) |
| Missing run | unknown named run with nothing required → `ORPHAN_REFERENCE`/`ORPHAN_REFERENCE`; nothing named while required → `ASSET_UNAVAILABLE`/`CHECKLIST_REQUIRED` |
| Blocking run | named BLOCKING run → `ASSET_UNAVAILABLE` (the availability code first) with `CHECKLIST_BLOCKING` and the run; after the release a start declared later passes and links it |
| Expired run | one nanosecond before completion and at completion + validity → `CHECKLIST_EXPIRED`; one microsecond inside → linked; Go table at both bounds |
| Foreign run | a planted run of another organization (forced RLS) → `ORPHAN_REFERENCE`/`CHECKLIST_REQUIRED`, no run stored |
| Another asset's run | a run of a second vehicle → `ORPHAN_REFERENCE`/`CHECKLIST_REQUIRED` naming it; also a RETURN run and another member's run; that member's own start with it links |
| Replay / reuse / duplicates | 6 concurrent identical deliveries → one trip, one link, identical bytes; replays byte for byte (accepted and held); same operation with another run → `OPERATION_ID_REUSED`, link unchanged; the same run on a second start → `ORPHAN_REFERENCE`/`CHECKLIST_REQUIRED` (one run, one trip) |
| Race between a release and a start | `TestTripChecklistGateReleaseRacesStartInBothOrders`: both queued behind a held organization root, both orders, no deadlock; the start declared before the release's recording is held `CHECKLIST_BLOCKING` with nothing linked, one release row; a later start passes and links |
| Return evidence without keeping the trip open (AC) | `TestTripChecklistReturnRunLinksAtTheFinishWithoutHolding`: a BLOCKING return run links to the finish, the trip completes, the vehicle is BLOCKED for the next use; a return run not stored yet, already linked, before the start, of the DEPARTURE context or of another member → finish accepted, nothing linked; a held finish links nothing |
| Cached policy bypasses server gate (negative) | the 1-hour server revision over the device's 12-hour cache; requirement read from the current templates at receipt |
| Return checklist prevents finish (negative) | every return case above finishes `ACCEPTED` at revision 2 |
| Unsupported forms | another reference type and two runs → `RETRY`/`UNAVAILABLE`, nothing stored (PostGIS); Go codec and Kotlin decoder tables |
| Forced RLS, insert-only, database proofs | `TestTripChecklistGateSchemaGuards`: both tables forced RLS and invisible without the setting; update/delete/truncate 55000; a departure link of a run completed after the start, of a RETURN run, by another operation, a return link without the finish (23514), an unknown run (23503), a second link (23505); a BLOCKING or EXPIRED reason the history does not prove, a reason without a held start, BLOCKING without a run (23514), a second reason (23505) |
| Populated upgrade 000254 image → 000255, failure and recovery | `TestTripChecklistGatePopulatedUpgradeFailureRecoveryAndPreviousImage`: completed and in-progress trips, a held `ASSET_BUSY` start, an ACTIVE departure template and an accepted run on the previous image (the flagged gate); injected failure leaves schema, every function and data unchanged; ordinary migrator (REFERENCES only); recovery once; no-op replay; previous image compatible; both tables forced RLS and migrator-owned; every existing function unchanged and exactly four added; earlier receipts replay; the pre-upgrade trip finishes, the pre-upgrade run gates a new start once, nothing named holds `CHECKLIST_REQUIRED` |

## Validation

Environment: worktree backend, Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f4-go-build`,
`GOTMPDIR`/`TMPDIR=/data/dev/cache/f4-tmp`; the shared PostGIS control
`127.0.0.1:32775` only through orgtest isolated child databases; a throwaway
MinIO project `f4-trip019-s3` from `infra/compose.validation.yml` (torn down
with `down -v` at the end); Kotlin through the shared Gradle flock (JDK 17).

- Toolchain `gofmt -l ./internal ./cmd ./db`: only
  `internal/platform/apicontract/trip_mission_test.go`, which this task does
  not touch (drift already on the base `03c803c4`; left to the coordinator);
  every touched file is formatted. `go build ./...`, `go vet ./...`,
  `go vet -tags=integration ./...` and `go vet -tags='integration
  nativefixture'` on organization adapters clean.
- `GOTOOLCHAIN=go1.27.1 staticcheck` on trip, maintenance, enterprisesuite,
  `db/queries/maintenance`, and with `-tags=integration` on organization
  adapters, trip and maintenance: no findings (`trip019-staticcheck.log`,
  empty; one SA4006 in the new test was fixed first).
- `sqlc generate` (maintenance stanza unchanged), `sqlc vet`, `sqlc diff`
  clean; only `db/queries/maintenance/runs.sql(.go)` change.
- OpenAPI `vacuum lint -r contracts/openapi/vacuum-rules.yaml`: 0 errors,
  1,129 warnings / 682 informs, identical to `HEAD` (two description
  edits; `trip019-vacuum.log`, `trip019-vacuum-head.log`).
- Affected race/integration union `go test -json -race -tags=integration
  -count=1 -timeout=150m -p 2 ./internal/modules/organization/adapters
  ./internal/modules/trip/... ./internal/modules/maintenance/...
  ./internal/modules/fleet/... ./internal/modules/tracking/...
  ./internal/modules/orgsync/... ./internal/platform/... ./cmd/api/...` with
  the MinIO up → `trip019-db-union.jsonl` (06:09–07:17, shared machine,
  `organization/adapters` 3,076 s): 42 packages (38 ok, 3 without tests, 1
  failing), 1,418 top-level / 9,653 subtests passing, 0 skips, 2 failures:
  `TestChecklistTemplateSchemaGuardsAndIsolation` (the CHECK-001 schema test
  truncates template versions with `CASCADE`, which now reaches the two new
  tables through their run keys; fixed in the fixture only by granting
  `TRUNCATE` on them so the append-only guard is still reached, the CHECK-002
  precedent; no expectation changed) and
  `TestLegalDeclarationsThroughComposedCompanyHTTP` (a 401 instead of 201 on
  the legal-entity create after many calls under load; unrelated code, no
  change). Rerun of both with every `TestTripChecklist*` and
  `TestChecklistRun*` test → `trip019-db-union-rerun.jsonl`: 17/17
  top-level, 39 subtests passing.
- Untagged whole module `GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...`
  → `trip019-untagged-all.log`: exit 0, 230 packages ok, 86 without tests, 0 FAIL.
- `python3 docs/enterprise/tools/check_plan.py` → 0 problems (348 tasks,
  545 rules, 167 use cases, 145 screens); `git diff --check` clean; the
  staged diff was reviewed for secrets and personal data (synthetic
  identifiers and Portuguese sample texts only; the disposable MinIO
  credentials live only in the log environment file).
- `cmd/worker`, `directory` and `quality` integration tests were not run
  (outside the union; they need pinned Rust preparer CLIs this machine
  lacks; untouched). No emulator or device run (not authorized for this
  task); the native flow of a gated start is retained.

## Retained

- Showing the stored gate reason (and the named run) in the EMP-TRIP-007
  conflict view and the panel (TRIP-006/CHECK-004); held-start acceptance
  linking the run.
- The finish form's return-checklist line on Android (the builder and
  decoder support the return run; the app sends none yet); the mission
  reference from the device (EMP-TRIP-021).
- Requirement by template history at the declared instant (EDR-332 keeps the
  current configuration); configurable author rules (another member's run
  never counts today).
- Run photos, damages and incident blocks (CHECK-004/DOC-003), notifications,
  production grants (README), real-device acceptance; the native emulator
  flow of a gated start.
- EMP-TRIP-010 (planned versus actual stops and legs), the second task of
  this prompt, was not started: its pre-allocated rules 475..480, use cases
  242..244, decisions 307..310 and migration 000256 stay reserved and unused.
- No milestone, phase or gate is claimed.
