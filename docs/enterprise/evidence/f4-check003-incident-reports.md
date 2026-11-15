# EMP-CHECK-003 — Damage and incident reports with photos

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 API task (module CHECK, P1, size M) on
`codex/enterprise-f4-check003` (worktree `/data/dev/worktrees/f4-check003`),
based on the F4 integration head `22c521b2` (CHECK-001 `maintenance` module,
TRIP-001..004, DOC-002/003/014, PRIV-005, catalog v5) and then merged with the
F4 heads carrying EMP-FLEET-013 and EMP-TRIP-008, then EMP-TRIP-016 and
EMP-TRIP-015 (section "Merged-tree validation"). The coordinator integrates it into `codex/enterprise-f4` and owns
PROGRESS, the F4 ledger, pushes and the PR. Synthetic data only; no real photo,
person, plate or place.

Read: the EMP-CHECK-001..004 and EMP-MAINT-003 task blocks
([EP11](../backlog/ep11-maintenance.md)); [maintenance](../modules/maintenance.md)
§§3–12 (Damage and Incident entities, BR-CHECK-009..018, UC-CHECK-03/04/05,
the incident and damage state table, §7 permissions, §8 offline, §9 events,
§10 text bounds, SCR-CHECK-03/05); [08-state-model](../08-state-model.md)
(Incident and damage row); [10-offline-sync](../10-offline-sync.md)
(BR-OFFL-004/006/007/011/015, UC-OFFL-03, the offline matrix row "Incident or
damage report"); [11-privacy-security](../11-privacy-security.md) BR-PRIV-004;
[07-permissions](../07-permissions.md) §7.3 and the v5 amendment; OQ-04 and
EDR-162/163/169/202/207..209/220..223 in [16-decisions](../16-decisions.md); the
evidence of CHECK-001, TRIP-003 (the reserved `INCIDENT`/`DAMAGE` answer),
TRIP-004 (company decision routes), DOC-002, DOC-003 (resolver registry) and
DOC-014 (storage reasons); the F4 ledger.

IDs used: B-BR-EMP-369..374, BUC-EMP-186..188, EDR-238..241 — all of them; no
pre-allocated ID is left unused. Migration `000230_*` only; every test derives
the version and the previous image from one file-name constant
(`incidentMigration`) and asserts that the previous image reaches 000226.

## Rules frozen before behaviour

Observed before any change: no damage, incident or report code, table, route
or operation existed; `check.incident.report` (S) and `check.incident.manage`
(O, v5) were `NOT_YET_ROUTED`; TRIP-003 answered `trip.event.record` with
`INCIDENT`/`DAMAGE` RETRY/UNAVAILABLE "until their owners"; DOC-003's closed
link registry had ASSET, EMPLOYEE and METER_READING; the DOC-003 link port
takes a `pgx.Tx` while intake owners receive a borrowed DBTX; no organization
expense or payable table exists.

Rules B-BR-EMP-369..374 and use cases BUC-EMP-186..188 are the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-CHECK-003 — Damage and incident reports with photos (2026-11-15)";
decisions EDR-238 (operation, payload, intake authority and outcomes, storage
and guards, usage sources), EDR-239 (photos through the DOC-002/003 chain with
the `INCIDENT` kind), EDR-240 (trip timeline by read join; `trip.event.record`
`INCIDENT`/`DAMAGE` → `MALFORMED`) and EDR-241 (company routes and policies,
the pure domain, Kotlin mirror and oracle) are in
[16-decisions](../16-decisions.md). They are the recommended routine
decisions, taken unattended. Deviations from the starting brief, each in an EDR:

- One operation `check.incident.report` (with `report_type` DAMAGE|INCIDENT)
  instead of the specification's two names; damages and incidents share the
  state model, permission and handling (EDR-238).
- The report operation carries no document reference; photos are linked by
  the existing DOC-003 routes with the new `INCIDENT` kind, so a report's
  acceptance is independent of its photos by construction (EDR-239). Envelope
  attachment declarations stay RETRY/UNAVAILABLE for EMP-OFFL-005.
- `check.incident.report` is routed by the reporter's own list, so the
  authorization matrix gains two representatives (the manage queue and the
  own list), not one (EDR-241).
- `RESOLVE`/`DISMISS` of a `blocks_use` report by its reporter is
  `SELF_APPROVAL` (BR-CHECK-011 applied to reports); the asset block itself and
  the release of a BLOCKING checklist run stay EMP-CHECK-002/004.

## Shape

- Pure Go `backend/internal/modules/maintenance/domain/incident` (standard
  library only): `ReportType`, `Kind`, `State`, `Action` closed vocabularies,
  `Allowed` (08-state-model), `NewReport` (B-BR-EMP-369), `Decide`
  (B-BR-EMP-373; errors `ErrInvalid`, `ErrConflict`, `ErrTransition`,
  `ErrSelfApproval` in that order), `ValidText` (SQL `org_asset_text_valid`
  profile), `CanonicalID`; text redacted on fmt, JSON and slog.
- Kotlin mirror `domain/src/main/kotlin/com/anpfuel/domain/enterprise/maintenance/incident/Incident.kt`.
- Oracle `contracts/testdata/enterprise/checklist/incident-v1.psv` (98 cases:
  REPORT 35, DECIDE 63; SHA-256
  `401ba762533077b059ffd5c39161451622adb6cf1fcf40b7de8ca8b235549119`) with its
  README section; both consumers assert the digest, counts, unique IDs, known
  operations and keys, complete expected maps, every operation and error code,
  an INVALID vector per operation and every action and state in an accepted
  step.
- DOC-001 `INCIDENT` target kind (Go, Kotlin, one `document-v1.psv` ENUM row:
  252 cases, SHA-256 `47b8a29c…6e9202f`), DOC-003 `BindIncident` and the link
  insert's typed reference.
- Application `maintenance/application/incidents.go`: the codec
  `ParseReport`, `ReportPolicy`, the owner ports (`ReportAuthority`,
  `AssetLocks`, `Allocations`, `Trips`, `ReportStore`), `TripWindow`, the
  company policies `IncidentPolicy`, `IncidentAuthority`, `DecisionCommand`
  (digest binds every field), views and `IncidentService` (decision bound
  before and after every repository call, result validation).
- Adapters `maintenance/adapters/incident_intake.go` (the owner:
  Admit/Apply/Confirm, no held path) and `incidents.go` (intake store, the
  trip timeline source `TripReportsTx`, the `IncidentDocumentTargets`
  resolver, the company owner `DecideReport`/`ReadReport`/`ListReports`/
  `ListOwnReports`, audit definitions `check.report.acknowledged|treated|
  resolved|dismissed` on target `incident_report`); sqlc
  `db/queries/maintenance/incidents.sql`.
- HTTP `maintenance/adapters/http/incidents.go`: `IncidentRecords`, four
  routes, composed by `enterprisesuite` (admission class `check.incident`,
  `IncidentStructure`, the `INCIDENT` resolver in the document registry, the
  owner in the intake registry with NO_ASYNC_FOLLOWUP for ACCEPTED/REJECTED,
  the report source on the trip `EventStore`).
- Organization adapters: `incident_authority.go` (`ReportAuthority` over the
  meter authority with the EDR-177 rule for `check.incident.report`;
  `IncidentAuthority` over the company authority), `IncidentStructure` in
  `resource_structure.go`, the three incident policies in the company
  authority's allow-list.
- Trip: `TimelineReport`/`TimelineReports` port, `EventStore.Reports`
  (required by `TimelineTx`), `ParseEvent` refusing `INCIDENT`/`DAMAGE`.
- Migration `000230_organization_incident_reports.sql` (EDR-238/239):
  `org_incident_reports` (insert guard under the root, asset and trip locks:
  the exact `check.incident.report` operation of an ACTIVE enrolled reporter,
  a non-DRAFT asset, the allocation at the declared instant, the trip's asset,
  author and window; the only update is the next revision written by its
  history row; delete/truncate refused), `org_incident_report_transitions`
  (step from the current state and revision, note policy, duplicate of the
  same asset, active handler, separation of handler and reporter for a
  blocking report by account, membership and canonical employee; the AFTER
  trigger applies the step; append-only), deferred publication checks (the
  ACCEPTED receipt and effect at revision 1; each step's own audit event and
  digest), forced RLS on both, `org_document_links.incident_report_id` with its
  same-organization key and the widened kind and binding checks, and the
  asset, unit and cost-centre usage sources redefined from their latest bodies
  with the report keys as associations.
- Contract: four paths, 14 schemas, tag `enterprise-incidents`, the
  `check.incident.report` owner, the `INCIDENT` link kind and the updated
  `trip.event.record` description in `contracts/openapi/v1.yaml`; inventory
  `check.incident.manage` ROUTED (3 routes, representative the queue) and
  `check.incident.report` ROUTED (1 route, the own list): normal 134 → 138,
  routed 24 → 26, not routed 60 → 58, specials 25; router mount list +4;
  matrix +2 representatives (240 → 260 cells, 93 → 103 allowed, 147 → 157
  denied; S cells of the own list cover exactly the member's own list); the
  AUTH-011 walk 138 routes / 30 positive / 108 FORBIDDEN; operation owners
  6 → 7; the matrix private-state snapshot includes both new tables.

## RED

Logs in `/data/dev/worktrees/f4-logs/check003-*`.

- Go domain (stubs returning `errNotImplemented`, `Allowed`/`ValidText`
  false) `go test -race -json ./internal/modules/maintenance/domain/incident/`
  → `check003-go-domain-red.jsonl`: 6 top-level, 4 failed; 98/98 vectors
  failed. The two passing tests exercise only the closed vocabularies and the
  grammar loader.
- Kotlin (`TODO()` stubs, flock) `:domain:test --tests
  '…maintenance.incident.*'` → `check003-kotlin-red.log`, XML in
  `check003-kotlin-red-xml/`: 103 tests, 101 failed (98 vectors, 3 of 4 unit
  tests); the grammar and the vocabulary tests passed.
- Router (routes wired, inventory at the base) → `check003-cmd-red.jsonl`: 3
  top-level failed (`TestAuthorizationInventoryCoversFullProductionRouter`,
  `TestCompanyProductionCompositionIsOptInAndContractChecked`,
  `TestFullProductionRouterEnterpriseContractAndDefaultOff/true`:
  unclassified/undocumented organization route).
- Contract (spec at the base) → `check003-contract-red.jsonl`: 3/3 failed (the
  owner/payload test, the guard-binding test and the owner count).
- Trip codec (the `INCIDENT`/`DAMAGE` refusal reverted) →
  `check003-trip-app-red.jsonl`: `TestParseEventRefusesMalformedAndKeepsReservedTypesUnavailable`
  failed ("malformed event accepted incident-own-operation").
- PostGIS schema RED (migration file moved out) `go test -json -race
  -tags=integration -run '^TestIncidentReport'` → `check003-db-red-schema.jsonl`:
  10/10 failed (42P01 at the grants; the upgrade test on the missing file).
- PostGIS behaviour RED (migration present; the owner's Apply, the four
  company store entry points, `TripReportsTx` and the `INCIDENT` resolver
  stubbed) → `check003-db-red-behaviour.jsonl`: 10/10 failed at their first
  behaviour check (the first report RETRY/UNAVAILABLE; the upgrade test after
  its migration half).

The adapters, application and tests were written before these RED runs (the
TRIP-002/TRIP-004 precedent); the RED was taken against stubbed entry points
with the frozen tests and the migration as written. No expectation was
changed to pass.

## GREEN and REFACTOR

- Go domain → `check003-go-domain-green.jsonl`: 6/6 top-level, 98/98 vectors
  under race, 98.0 % statements (`check003-go-domain-cover.out`).
- Kotlin → `check003-kotlin-green.log`: enterprise suite 3022 tests, 0
  failures (incident 103/103, document vectors 253/253 with the new row).
- Contract → `check003-contract-green.jsonl`: 7/7 top-level (incident, meter
  owner count, document link, trip event owner), 47 subtests; router
  `cmd/api` green.
- PostGIS, while developing (`check003-db-dev1.jsonl`): two fixture fixes —
  the logbook fixture's new grant on `org_incident_reports` is now taken only
  when the table exists (the TRIP-003 upgrade rehearsal stops at 000225), and
  the handling fixture grants `SELECT ON org_document_links, org_documents`
  for the attachment states; the composed HTTP test uses the DOC-003
  `linkGrants`. One expectation was wrong, not weakened: a report with two
  dependencies never reaches the owner's codec in the intake (the framework
  answers the unknown dependencies first), so that case stays only in the
  codec unit test.
- Matrix and walk (`check003-matrix.jsonl`, `check003-matrix2.jsonl`): the
  first matrix run returned 503 for the two new representatives because the
  matrix fixture lacked the incident grants; with `incidentGrants` the matrix
  passes 260 cells (103/157) and the walk 138/30/108.
- Focused PostGIS GREEN (`check003-db-green.jsonl`: incident, logbook,
  document-link, trip-review and checklist tests over organization adapters
  and document): 31/33 top-level, 35 subtests on the first run; the two
  failures were existing EMP-DOC-003 expectations that the new rule changes,
  updated without weakening: the link table's exact column list now includes
  the typed `incident_report_id` reference (still no amount, content or
  locator column), and the 000209 upgrade rehearsal applies the remaining
  embedded chain before exercising the current link store, which writes the
  000230 column. Both reran green (with the rest of `TestDocumentLink*`).
- Added after GREEN as characterization (not counted as RED): application
  `TestParseReport*`, `TestTripWindowChecksAssetAuthorAndInstant`,
  `TestIncidentPoliciesAreFixed`, `TestDecisionCommandValidatesAndDigestsEveryField`,
  `TestIncidentValuesAreRedacted`; adapters `TestReportHandlerNeedsEveryPortAndHasNoHeldPath`,
  `TestIncidentAuditActionsAreOnePerStep`, `TestIncidentDocumentTargetsResolveOnlyTheirKind`,
  `TestIncidentStoreIntakeRefusesWithoutTransaction`; HTTP
  `TestIncidentRulesAreExact`, `TestIncidentRefusalsMapToRegisteredCodes`.
- REFACTOR: test literals with invisible characters written as escapes
  (staticcheck ST1018).

## Acceptance and negatives mapped to tests

| Item | Evidence |
|---|---|
| A report is durable offline (signed operation) | `TestIncidentReportIntakeStoresTheFactAndReplays` (exact stored columns incl. the unit snapshot, reporter account/membership/employee, enrollment, digest, sub-microsecond remainder, zone, offset, receipt ≤ ACK; DAMAGE and INCIDENT), `TestIncidentReportConcurrentDuplicatesAndRollback` (12 concurrent identical deliveries → one report, one ACK; an injected store failure stores nothing and the same operation then commits) |
| Attachment status and permission are independent; a report is ACCEPTED whatever its photos | composed `TestIncidentReportsThroughComposedCompanyHTTP`: the report stays REPORTED/1 while its photo is UPLOADING, after it FAILED and after another is STORED; photos are linked only by the reporter (`doc.document.attach` on own document + `check.incident.report` on own report) |
| Failed upload described as ready | the read shows `FAILED` and `STORED` exactly, `UPLOADING` while pending; contract: the attachment state enum has no invented READY; a FAILED document cannot be linked (409) |
| Private content exposed | the report read and the queue carry no `content_base64`, capability, digest, unit or cost centre; the queue carries no description; attachment items have only `document_id`, `storage_state`, `linked_at`; content stays the PRIV-005 path (DOC-002 tests) |
| A report automatically books a cost | `costSnapshot` (every `org_*` table matching expense/payable/budget/fueling/reconcil/allocation_line/cost_entr/charge, plus documents/links) unchanged across reports (intake test), handling (handling test) and the composed routes (cost tables) |
| Cross-scope link | foreign asset → whole-response NOT_FOUND; unknown asset NOT_FOUND; foreign or unknown trip RETRY/DEPENDENCY_PENDING with nothing stored or revealed; another organization's report → NOT_FOUND (read, decide, link); a document of another member → 403 `doc.document.attach`; another member's report → 403 `check.incident.report` (link and by-target list) |
| Replay / OPERATION_ID_REUSED | intake: byte-identical ACK on resend, OPERATION_ID_REUSED for another digest; handling: `replayed=true` with the same recorded instant and no second row or audit event, OPERATION_ID_REUSED (store and HTTP) |
| Manager outside structural scope FORBIDDEN | `TestIncidentReportHandlingAuthorityAndIsolation`: a SUPERVISOR of another unit → FORBIDDEN `check.incident.manage` (read and decide); a SUPERVISOR of the report's stored unit and a FLEET_MANAGER of its asset handle it; structural grants never cover the organization queue |
| AUDITOR FORBIDDEN on commands | handling test, composed test (queue, read, decide, own list), AUTH-011 walk (all four routes FORBIDDEN with unchanged private state) |
| Handling (UC-CHECK-04) | ACKNOWLEDGE → TREAT → RESOLVE with notes; DISMISS as a duplicate of a report of the same asset; stale/future revision and disallowed steps CONFLICT; missing/blank/oversized notes and a note on an acknowledgement VALIDATION_FAILED; a terminal report never changes; one audit event per step bound to the history row's digest and actor, without text |
| SELF_APPROVAL | the reporter (ADMIN, holds the key) resolving or dismissing its own blocking report → SELF_APPROVAL (store and HTTP); acknowledging/treating it is allowed; database guard 23514 |
| Race (V-IDEM) | `TestIncidentReportHandlingRaceLeavesOneStepInBothOrders`: two handlers queued behind a held root lock (both observed waiting) — the first wins, the other CONFLICT, one row and one audit event, in both orders |
| Inside a trip (BR-CHECK-014) | `TestIncidentReportInsideATripIsATimelineFact`: DAMAGE and INCIDENT facts between START and FINISH in the shared order with no text, no logbook row copied, the trip unchanged; before start / after finish / another asset's trip MALFORMED; another member's trip FORBIDDEN; a report at the finish instant accepted; `trip.event.record` INCIDENT/DAMAGE MALFORMED (TRIP-003 tests updated with the rule) |
| SELF reporter scope | an unassigned DRIVER → FORBIDDEN; an assigned DRIVER → ACCEPTED |
| Malformed and unavailable | 15 payload shapes (incl. a health key and an amount key) MALFORMED; DRAFT asset MALFORMED; identity reuse MALFORMED; an attachment declaration RETRY/UNAVAILABLE with nothing stored |
| Forced RLS, guards | `TestIncidentReportSchemaIsForcedRLSAppendOnlyAndGuarded`: stale/skipped/self/duplicate/note-policy/unaudited steps refused by the database even when the owner is bypassed; direct state change 55000; description update and delete 42501 for the runtime role; owner update/delete/truncate 55000; another scope sees nothing; a second report on one operation 23514; the INCIDENT link binding and key in the catalog; usage sources answer |
| Populated upgrade 000226 → 000230 | `TestIncidentReportPopulatedUpgradeFailureRecoveryAndPreviousImage`: completed and in-progress trips, a document with an ASSET link; injected failure leaves schema, the three functions, the binding and data unchanged; ordinary migrator (REFERENCES + ownership of the link table and functions); recovery once; no-op replay; previous image compatible; forced-RLS ownership; every previous allow-list line kept; the old link survives; the pre-existing trip takes a report on its timeline |

## Validation

Environment: worktree backend, Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f4-go-build`,
`GOTMPDIR`/`TMPDIR=/data/dev/cache/f4-tmp`; shared PostGIS control
`127.0.0.1:32775` used only through orgtest isolated child databases; the
coordinator MinIO (`127.0.0.1:32819`, bucket `validation-private`) for storage
tests; Kotlin through the shared Gradle flock with JDK 17.

Pre-merge tree (task commit):

- Toolchain `gofmt -l ./internal ./cmd ./db` empty; `go build ./...`,
  `go vet ./...`, `go vet -tags=integration ./...` clean.
- `GOTOOLCHAIN=go1.27.1 staticcheck` on maintenance, document, trip,
  organization, enterprisesuite, apicontract, cmd/api, `db/queries/{maintenance,document}`
  and with `-tags=integration` on maintenance, organization adapters,
  document, trip, orgtest: no findings after the ST1018 fix
  (`check003-staticcheck.log`).
- `sqlc vet` and `sqlc diff` clean (maintenance stanza widened to the parents
  it reads; document stanza + 000230).
- OpenAPI `vacuum lint -r contracts/openapi/vacuum-rules.yaml`: 0 errors;
  1,034 warnings / 660 informs vs 1,018 / 660 at the base: the 16 new warnings
  are house-style descriptions on the new schemas, operations and the
  `report_id` parameter (`check003-vacuum.log`).
- Kotlin `:domain:test --tests 'com.anpfuel.domain.enterprise.*'`: 3022, 0
  failures.
- `python3 docs/enterprise/tools/check_plan.py` → 0 problems; `git diff
  --check` clean; staged diff reviewed for secrets and personal data
  (synthetic identifiers and Portuguese sample texts only; the disposable
  MinIO credentials live only in the log environment file).

## Merged-tree validation

The F4 head `3345865b` (EMP-FLEET-013, EMP-TRIP-008 with migration 000227 and
the `trip.mission.respond` owner) was merged into this branch after the task
commit `680505b3` (merge commit "Merge codex/enterprise-f4 (FLEET-013,
TRIP-008) into EMP-CHECK-003"). Conflicts were resolved as unions: EDR rows by
number, contract sections and backend README subsections, the OpenAPI sync
description and operation owners structurally (8 owners), the company
authority allow-list, the structure kinds (`MissionStructure`,
`IncidentStructure`), the intake handlers and no-async plans, the unit
fixture copy. Counts on the union: 144 normal routes (28 routed / 56 not
routed / 25 special), AUTH-011 walk 144 / 30 positive / 114 FORBIDDEN,
matrix 280 cells (112 allowed / 168 denied), operation owners 8. 000230 now
redefines `org_asset_current_usage` and `org_unit_current_usage` from the
000227 bodies (every mission line kept, diffed line by line) plus the report
key; the cost-centre source keeps the 000220 body (000227 does not redefine
it); the upgrade test also asserts the mission lines.

On the merged tree:

- Toolchain gofmt empty; `go build ./...`, `go vet` (both tags), `sqlc vet`
  and `sqlc diff` clean; staticcheck plain and `-tags=integration` on the
  touched packages clean (`check003-staticcheck-merged.log`).
- Unit: `cmd/api`, maintenance, trip, document, enterprisesuite and the
  incident/meter/mission/logbook/document-link/checklist apicontract tests
  pass.
- OpenAPI vacuum: 0 errors, 1,052 warnings / 660 informs vs 1,036 / 660 at the
  F4 head (the same 16 house-style description warnings;
  `check003-vacuum-merged.log`).
- Kotlin `:domain:test --tests 'com.anpfuel.domain.enterprise.*'`: 3398
  tests, 0 failures, 0 skipped (`check003-kotlin-merged.log`).
- Race/integration union `go test -json -race -tags=integration -count=1
  -timeout=90m -p 2 ./internal/modules/organization/adapters
  ./internal/modules/{maintenance,document,trip,fleet,orgsync,orgmedia}/...
  ./internal/platform/{enterprisesuite,apicontract,migrate,orgtest,orgapi}/...
  ./cmd/api/...` with PostGIS and the coordinator MinIO
  (`check003-db-union.jsonl`): 33 packages, 32 pass and `orgtest` has no
  tests; 1,099 top-level / 8,674 subtests pass, 0 failures, 0 skips; 38 min
  wall, `organization/adapters` 2,258 s under load.
- Untagged `GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...`
  (`check003-untagged-all.log`): exit 0, 223 packages ok, 81 without tests, 0 FAIL.
- `check_plan.py` 0 problems; `git diff --check` clean; gitleaks on the task
  diff: no leaks.

### Second merge: F4 head `95c0e389` (EMP-TRIP-016, EMP-TRIP-015)

The F4 head then advanced with migrations 000228 (trip period lock) and 000229
(late events and amendments) and was merged again (merge commit "Merge
codex/enterprise-f4 (TRIP-016, TRIP-015) into EMP-CHECK-003"). 000228/000229
redefine only the trip review and event sources, not the asset, unit or
cost-centre usage sources or `org_document_links`, so 000230 is unchanged.
Conflicts were resolved as unions: EDR-232..237 before EDR-238..241; the
OpenAPI `trip.submit` description takes the TRIP-015 resubmission text ahead
of the incident schemas (duplicate-key YAML check clean); the resource
structure keeps the `IncidentStructure` case and the `TripStructure` case
widened to `meter.reading.correct`. Counts: 145 normal routes (28 routed / 56
not routed), AUTH-011 walk 145 / 30 positive / 115 FORBIDDEN; the matrix stays
280 cells (112 allowed / 168 denied) because the amendment route reuses a
routed permission. A report naming a trip that is already submitted, decided
or amended is still a separate `incident_report` (EDR-240 read join): it is
shown on the timeline and never joins a TRIP-015 review revision.

On this merged tree:

- Toolchain gofmt empty; `go build ./...`, `go vet` (both tags), `sqlc vet`
  and `sqlc diff` clean; staticcheck plain and `-tags=integration` on
  maintenance, document, trip, fleet, organization, enterprisesuite,
  apicontract, cmd/api and the touched query packages clean
  (`check003-staticcheck-merged2.log`).
- OpenAPI vacuum: 0 errors, 1,057 warnings / 660 informs vs 1,041 / 660 at the
  F4 head (the same 16 house-style description warnings;
  `check003-vacuum-merged2.log`).
- Race/integration union (same command and packages as above;
  `check003-db-union2.jsonl`): 33 packages, 32 pass and `orgtest` has no
  tests; 1,119 top-level / 8,681 subtests pass, 0 failures, 0 skips; 34 min
  wall, `organization/adapters` 2,022 s, `cmd/api` 126 s.
- Untagged `GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...`
  (`check003-untagged-all2.log`): exit 0, 223 packages ok, 81 without tests, 0 FAIL.
- Kotlin not rerun: the F4 delta changes no Kotlin source, oracle or test
  input (only the backend, the OpenAPI file and the route inventory, which no
  Kotlin test reads), so the 3398 / 0 result above stands for this tree.
- `check_plan.py` 0 problems (348 tasks, 545 rules, 167 use cases, 145
  screens); `git diff --check` clean; gitleaks over the branch commits since
  `22c521b2`: no leaks.

## Retained

- The asset block from `blocks_use` and its release (BR-CHECK-010/011,
  UC-CHECK-05): EMP-CHECK-002 (runs, damages from `NOT_OK` answers, release of
  a BLOCKING run) and EMP-CHECK-004 (panel, scoped queue filters).
- Held reports: a report whose right changed after its declaration, late
  intake after access ended (BR-OFFL-011) and a report outside its named
  trip's window stay RETRY/UNAVAILABLE or MALFORMED until a held owner exists.
- Device attachments: envelope declarations, document creation from them and
  the signed link path (EMP-OFFL-005, EMP-DOC-004); photo kinds beyond the C1
  synthetic JPEG receipt container; unlink/replacement (EMP-DOC-011) and
  retention (EMP-DOC-012). Real photos stay legally gated (OQ-04, LV-09/10).
- Notice/police numbers and linked damages of an incident, work orders and
  corrective requests from a report (EMP-MAINT-003), insurance claims.
- Report events (`check.*.reported`, `check.report.*`) and notifications to
  the reporter; the Android producer and report screens (EMP-CHECK-002/
  Android); the panel timeline (EMP-TRIP-006).
- Team-scope structural facts for reports; production grants (backend README
  subsection) and live composition review.
- No milestone, phase or gate is claimed.
