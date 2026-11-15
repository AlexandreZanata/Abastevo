# EMP-FLEET-006 — Driver assignments: primary, temporary and multiple drivers

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Sole bounded F2 task in worktree `codex/enterprise-f3-fleet006`, opened from
the integrated head with migrations through 000208 (FLEET-003 201, PRIV-005
202, APPR-006 203, NOTIF-001 204, DOC-002 205, FLEET-005 206, RPT-001 207,
AUTH-006 208). It is the fourth link of the EDR-109 chain FLEET-003 →
FLEET-005 → AUTH-006 → FLEET-006 → METER-002 closure that gates every F3
persistence task. FLEET-002 (assets), FLEET-003 (lifecycle and disposal),
FLEET-005 (allocation periods and the `UnitAndCostCentreAt` port), AUTH-006
(structural scopes), IDN-003/IDN-010 (employee records, memberships and
offboarding with termination) and the employee merge are the real
dependencies used here; nothing is faked for them.

Read: task block `ep05-fleet-meters.md` EMP-FLEET-006 (and FLEET-013,
OFFL-006, METER-002); `modules/fleet.md` §3, §4 (BR-FLEET-018..024), §6.3,
§7, §8, §9, §10.5; `07-permissions.md` §4 (BR-AUTH-004 `SELF`,
`fleet.asset.assign`); EDR-109/110/118/143..145/154/155/160/161; evidence of
FLEET-003, FLEET-005, AUTH-006, METER-002, IDN-010 and PLAT-010. Shared
canonical docs are not edited here: the rule/use-case bullets, EDR rows and
backlog note are frozen between the markers below for the coordinator.

IDs used: B-BR-EMP-243..246, BUC-EMP-124..125, EDR-164..165 (all of them; no
pre-allocated ID is left unused). Migration `000210_*` (append-only; 000209 is
reserved by the parallel EMP-DOC-003 task and 000211 by EMP-AUTH-011; every
test derives the version from one file-name constant and builds the previous
chain as `name < thisMigration`).

## Rules frozen before behaviour

Current state observed before any change: no assignment storage exists.
`fleet.asset.assign` is `NOT_YET_ROUTED`; `fleet.asset.read` has `S` cells
for `DRIVER` and `OPERATIONS_STAFF`, but no resolver supplies an assignee fact
(`FleetRecords`/`FleetAuthority`: "SELF remains unproven"), so every SELF
fleet request is refused. Employee records change to `TERMINATED` through
offboarding with termination and through the employee merge (source record);
team links already end on that change through an `AFTER UPDATE OF state`
trigger in the same transaction (migration 177). FLEET-003 disposal sets
`org_assets.state = 'DISPOSED'` with `disposed_at` (microseconds) plus
`disposed_submicro` and, by EDR-144, ends no assignment and enqueues no event.
The signed meter intake treats non-`ORG` grants as unsupported.

Rules B-BR-EMP-243, B-BR-EMP-244, B-BR-EMP-245, B-BR-EMP-246, BUC-EMP-124, BUC-EMP-125 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-FLEET-006 — Driver assignments: primary, temporary and multiple drivers" (frozen before behaviour; text moved verbatim at integration).

### Decisions (EDR rows, frozen before behaviour)

Decisions EDR-164, EDR-165 are recorded in [16-decisions](../16-decisions.md).

### Backlog note


## Implementation

- Migration `000210_organization_asset_assignments.sql` (append-only, no
  existing row written): `org_asset_assignment_ended()` (the derived-state
  predicate, immutable SQL); forced-RLS `org_asset_assignment_commands`
  (kinds `CREATE`/`END`, ordered `assignment_ids`, optional `replaced_id`,
  deferred audit foreign key, statement guard refusing UPDATE/DELETE/TRUNCATE)
  and forced-RLS `org_asset_assignments` (composite same-organization foreign
  keys to `org_assets` and `org_employee_identities`, deferred foreign keys to
  the creating and ending receipts, whole-microsecond bounds,
  `TEMPORARY ⇒ valid_to`, empty period only when ended, cause/command/source
  consistency checks, `org_asset_assignments_primary_no_overlap` GiST exclusion
  `WHERE kind='PRIMARY'`, asset-period and employee indexes); the row guard
  (insert: scope 42501, root lock with the organization `ACTIVE` (55000), asset
  and employee found (23503 with the FK constraint name) and admissible (23514:
  asset `ACTIVE`/`INACTIVE`, employee `ACTIVE`), no end data and no start before
  the transaction (23514), server `recorded_at`; update: identity immutable,
  not `ENDED` at the transaction start, end moves earlier only (55000), then
  per cause: `ENDED`/`REPLACED` under the `ACTIVE` root with command data and an
  end not before the transaction, `EMPLOYEE_TERMINATED` proven against the
  employee state and revision, `ASSET_DISPOSED` equal to
  `GREATEST(valid_from, disposed_at)` of the `DISPOSED` asset revision (23514),
  server `ended_at`); DELETE/TRUNCATE refused (55000); a deferred constraint
  trigger requiring a `REPLACED` primary's successor (same operation, starting
  at its end); the two `AFTER UPDATE OF state` consumers
  (`org_asset_assignment_end_employee`, `org_asset_assignment_end_disposal`);
  `CREATE OR REPLACE org_asset_current_usage` (208 body + the EMP-DOC-003
  `org_document_links_asset_fkey` line through `to_regclass` + the new
  `org_asset_assignments_asset_fkey` line; counts unchanged).
- Pure domain `fleet/domain/assignment.go`: closed kinds, derived states,
  `MaxAssignees = 100`, `AssignmentPeriod` (`NewAssignmentPeriod`, `Empty`,
  `Contains`, `StateAt`, `EndAt` with microsecond floor and empty period before
  the start), `ValidNewAssignment`, `ValidAssignees`, `ErrAssignmentEnded`.
- Application `fleet/application/assignments.go`: `AssignPolicy` (fresh
  `fleet.asset.assign`, configuration write), actions `FleetAssign` and
  `FleetReadOwnAssignments`, `AssignmentCreate`/`AssignmentEnd` commands with
  exact shape, non-aliasing snapshots and scoped SHA-256 digests,
  `AssignmentService` (`Create`, `End`, `List`, `Mine`: the live decision of the
  exact policy and target before and after the repository, full result/page
  validation including the state at the page instant), and the
  `AssignedAssetsPort` (`AssignedAtDBTX`, `AssetsAssignedAtDBTX`). `current`
  now delegates to the policy-generic `authorized` (same checks).
- Store `fleet/adapters/assignments.go` (+ sqlc `db/queries/fleet/assignments.sql`):
  authority → root lock → receipt replay → one audit event
  (`fleet.assignment.created` / `fleet.assignment.ended`) → asset found → DB
  clock → effect → receipt → authority, all in one transaction; replace locks
  and ends the covering primary first; the exclusion refusal maps to
  `ErrAssignmentOverlap`; list/own reads derive states at one DB instant; the
  port floors the instant and requires both the membership and the assignment
  periods to contain it.
- HTTP `fleet/adapters/http/assignments.go`: `POST .../fleet/assets/{asset_id}/assignments/create`
  (201), `.../assignments/{assignment_id}/end`, `.../assignments/list` and
  `/v1/orgs/{org_id}/fleet/assignments/mine/list` through `guarded.Register`;
  strict bodies; `ASSIGNMENT_OVERLAP` (409, registered in `orgapi`), ended →
  `CONFLICT`; cursors bound to route, asset and limit; `evaluated_at` on pages;
  the own list uses the requesting membership as its record (no structure or
  assignee lookup). `FleetRecords` accepts the assign policy.
- Authorization (EDR-165): `orgaccess/domain.SelfCandidate`;
  `Authorization.Assignee` consulted by `Prepare` only for a current positive
  SELF grant when the resolver stated no own record; organization adapter
  `ResourceAssignee` (pool and in-transaction forms, `fleet.asset.read` on an
  asset only) used by `companyAuthority` (also `OwnTarget` for the own list);
  `FleetAuthority` admits the two new actions; `MeterAuthority` web path uses
  the same assignee source; `ResourceStructure` resolves assets for
  `fleet.asset.assign` too.
- Composition `enterprisesuite/routes.go`: one `ResourceAssignee` over the asset
  store for the asset route group, the fleet and meter authorities;
  `RegisterAssetAssignments`; admission class `fleet.assignment`.
- Contract: 4 paths, 6 schemas and the `ASSIGNMENT_OVERLAP` error code in
  `contracts/openapi/v1.yaml`; the SELF sentence of the six asset-scoped
  `fleet.asset.read` routes and of the organization list updated to the
  delivered behaviour. Inventory `authorization-routes-v1.json`:
  `fleet.asset.assign` ROUTED (2 routes), `fleet.asset.read` +2 routes
  (routed 20→21, not routed 64→63, normal 103→107, special 24 unchanged);
  router test paths +4; authorization matrix +1 representative
  (`fleet.asset.assign` on an asset of a second unit; 200→210 cells,
  77→81 allowed, 123→129 denied).
- Test fixtures adapted (no assertion weakened): `orgtest` grants the exact
  consumer columns of `org_asset_assignments` when installed (every existing
  termination/disposal producer runs the consumer); the matrix private-state
  snapshot includes the two new tables; the matrix and two DB tests grant DELETE
  only to prove the statement guard. The application aliasing test first
  mutated the shared `end` variable it later compared with (a test bug, fixed
  by using its own copy).

## Acceptance mapping

| Item | Evidence |
|---|---|
| AC1 SCHEDULED / ACTIVE / ENDED from server time, half-open | domain `TestAssignmentStateIsDerivedFromServerTimeWithHalfOpenPeriods`; DB `TestFleetAssignmentStoreDerivesStatesAtServerTimeAndKeepsReceipts` (future start SCHEDULED, now ACTIVE, ended ENDED, page states at the DB instant; port at start−1µs, start, start+999ns, end−1µs, end); HTTP page `state` and `evaluated_at` |
| AC2 overlapping primary → `ASSIGNMENT_OVERLAP`; replace ends the previous at the new start atomically | `TestFleetAssignmentPrimaryOverlapReplaceAndConcurrency` (refusal leaves nothing; replace sets `valid_to` = new start, `REPLACED`, ending operation; a replace reaching a later primary is refused whole and the shortened primary keeps its end); HTTP 409 code |
| AC3 TEMPORARY without end refused | domain/application shape tests; SQL check 23514 (`TestFleetAssignmentSchemaGuardsAndIsolation`); HTTP 422 |
| AC4 bulk N → N or none | store test (2 rows, one receipt, one audit event; an inactive employee in the list leaves no row, receipt or audit) |
| AC5 termination ends assignments with cause; disposal ends the asset's | `TestFleetAssignmentsEndWithEmployeeTerminationAndAssetDisposal` (actual offboarding with termination: current, scheduled (empty), INACTIVE-asset and replaced-in-future primaries end with `EMPLOYEE_TERMINATED` and the employee revision; retained employee and colleagues unchanged; rehire restores nothing; actual disposal ends at the exact microsecond floor of the disposal instant, scheduled emptied, `ASSET_DISPOSED` + revision; revert restores nothing; deactivation keeps); upgraded schema disposal |
| Employee of another organization / unknown | store `TestFleetAssignmentRefusesForeignInactiveAndUnassignableReferences` (NOT_FOUND, no row anywhere); HTTP 404 |
| Employee not ACTIVE; asset DRAFT/DISPOSED/ARCHIVED | same test (`VALIDATION_FAILED`; INACTIVE asset admitted); upgraded schema |
| Two concurrent primaries leave one | concurrency part: two real store commands blocked behind a held root lock (observed through `pg_blocking_pids`, no sleep), exactly one commits and the other gets `ErrAssignmentOverlap`; SQL: two overlapping primaries in one transaction → 23P01 on `org_asset_assignments_primary_no_overlap` |
| Unit-scoped supervisor of another unit denied | HTTP (Sul supervisor on a Norte asset → 403 `FORBIDDEN`, its list refused; Norte supervisor and Sul-on-Sul allowed); matrix `unit-scope` variant on the second-unit asset |
| Ending an ended assignment | store `ErrAssignmentEnded`; HTTP 409 `CONFLICT`; SQL 55000 |
| Replayed command | store and HTTP replay of create, replace and end (first result, `replayed: true`); reuse with another intent `OPERATION_ID_REUSED` |
| Ended membership not listed as assignee | store (membership ended with the employee retained: port false now, true before the end; row unchanged); termination test (retain path); HTTP own list empty after the end |
| RLS without organization setting | SQL test (0 rows, unscoped update 0 rows, insert 42501, row of another organization 42501); upgrade test |
| Populated upgrade previous head → 000210 with failure/recovery | `TestFleetAssignmentPopulatedUpgradeIsAdditiveAndRecoverable` (ACTIVE/DISPOSED/DRAFT assets, ACTIVE and TERMINATED employees; injected failure leaves schema, consumers, usage source and data unchanged; ordinary NOSUPERUSER/NOBYPASSRLS migrator with only REFERENCES/TRIGGER/function ownership; no-op replay; previous image check; guards, consumers and port on the upgraded schema) |
| SELF on assigned assets | HTTP (driver: assigned asset read, assignment/lifecycle/meter lists 200; unassigned asset 403 naming `fleet.asset.read`; organization list 403; cannot assign; own list; after the end 403 and empty own list); `orgaccess` unit tests (lookup only for a current positive SELF grant, sets only `AssignedTo`, fails closed) |

## Validation

Environment: worktree backend, Go 1.27.4 toolchain
(`/data/dev/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.4.linux-amd64/bin`),
`GOCACHE=/data/dev/cache/f3-go-build`, `GOTMPDIR`/`TMPDIR=/data/dev/cache/f3-tmp`,
`STATICCHECK_CACHE=/data/dev/cache/f3-staticcheck`; owned disposable PostGIS
control `127.0.0.1:32775` used only through orgtest isolated child databases,
ordinary NOSUPERUSER/NOBYPASSRLS runtime roles and, for the upgrade, an ordinary
migrator role. Logs in `/data/dev/worktrees/f3-logs/fleet006-*`.

### RED (failing first, against compiled stubs / without the migration)

- Unit RED `go test -json -count=1 ./internal/modules/fleet/... ./internal/modules/orgaccess/domain/ ./internal/modules/orgaccess/application/`
  → `fleet006-unit-red.jsonl`: 19 new top-level tests failed (fleet domain 6,
  fleet application 8, fleet HTTP 3, orgaccess domain 1, orgaccess application
  1; 2 subtests), 135 top-level / 1852 subtests passed.
- PostGIS schema RED (migration file moved out) `go test -json -race -tags=integration -timeout=15m ./internal/modules/organization/adapters -run '^TestFleetAssignment' -count=1`
  → `fleet006-db-red-schema.jsonl`: 7/7 failed (42P01 `org_asset_assignments`;
  the upgrade test failed on its fixture, then fixed: an ACTIVE membership needs
  a role), 17.2 s.
- PostGIS behaviour RED (migration present, Go stubs), same command →
  `fleet006-db-red-behaviour.jsonl`: 7/7 failed (stub commands/store); the
  ordinary migrator already applied 000210.
- Contract RED: routes wired, spec at `HEAD`: `go test -json -count=1 ./cmd/api/`
  → `fleet006-cmd-red.jsonl`: 3 failed (inventory unclassified route, company
  composition and full router "undocumented organization route");
  `go test -json -run TestFleetAssignmentContract ./internal/platform/apicontract/`
  → `fleet006-contract-red.jsonl`: failed (missing schemas, 6 subtests failed,
  then a nil-schema panic).

### GREEN

- Focused PostGIS race with coverage (`-run '^TestFleetAssignment|^TestAuthorizationMatrixActualCompanyHTTP$'`,
  `-coverpkg` fleet, organization adapters, orgaccess) →
  `fleet006-db-focused-final.jsonl`: 8/8 top-level passed, package 68.3 s;
  composed HTTP 50 request assertions; matrix 210 cells (81 allowed / 129
  denied).
- Affected PostGIS race union `go test -json -race -tags=integration -timeout=60m ./internal/modules/fleet/... ./internal/modules/organization/adapters ./internal/modules/orgsync/... ./internal/modules/orgaccess/... ./internal/modules/membership/... ./internal/platform/migrate/... -count=1`
  → `fleet006-db-union-1.jsonl`: 19 packages ok, 743 top-level / 5914 subtests
  passed, 0 failures/skips (organization adapters 798.1 s). After it two
  narrow changes (the REPLACED successor trigger now checks every replacement;
  the port test uses the borrowed intake DBTX) passed the focused run above;
  the same union rerun is `fleet006-db-union-final.jsonl`: 19 packages,
  743 top-level / 5914 subtests passed, 0 failures (completed after handback;
  counted by the coordinator).
- Unit race + coverage `go test -json -race -count=1 -coverprofile=... ./internal/modules/fleet/... ./internal/modules/orgaccess/domain/ ./internal/modules/orgaccess/application/ ./internal/platform/orgapi/ ./internal/platform/enterprisesuite/ ./cmd/api/`
  → `fleet006-unit-green.jsonl`: 230 top-level / 1988 subtests passed.
- Untagged whole module `go test -count=1 -timeout=45m ./...` →
  `fleet006-untagged-all.log`: 211 packages ok, 77 without tests, 0 FAIL
  (apicontract 74.9 s).
- Coverage (statements, unit ∪ PostGIS): `fleet/domain/assignment.go` 97.8%,
  `fleet/application/assignments.go` 87.9%, `fleet/adapters/assignments.go`
  78.3% (PostGIS), `fleet/adapters/http/assignments.go` 86.3%,
  `organization/adapters/resource_assignee.go` 69.4% (uncovered: nil/invalid
  source and failure fail-closed branches). Profiles
  `fleet006-unit-coverage.out`, `fleet006-db-coverage.out`.
- Build/vet/format/static/generation: `go build ./...`, `go vet ./...`,
  `go vet -tags=integration ./...` clean; `gofmt -l ./internal ./cmd ./db`
  empty; `GOTOOLCHAIN=go1.27.1 staticcheck` on fleet, orgaccess, organization,
  enterprisesuite, apicontract, orgapi, cmd/api and with `-tags=integration`
  on organization/adapters, fleet, orgtest, orgaccess: no findings
  (`fleet006-staticcheck.log`); `sqlc generate` (fleet stanza + 000210),
  `sqlc diff` and `sqlc vet` clean.
- OpenAPI: `vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check`
  → 0 errors, 950 warnings / 615 informs vs 942 / 611 at `HEAD` (the existing
  route patterns: `enterprise-fleet` tag not global, POST request body).
- `git diff --check` clean; `python3 docs/enterprise/tools/check_plan.py` →
  0 problems. Secret/PII scan `git diff --cached | grep -nEi
  'password|secret|token|BEGIN .*KEY|@gmail|cpf'`: only runtime-generated
  synthetic session/CSRF tokens of disposable test accounts, the fixed
  synthetic test cursor secret (`bytes.Repeat([]byte{7}, 32)`, as in the
  FLEET-005 HTTP test), the `Credentials().AccessToken` emptiness assertion,
  the existing error-code enum line and this sentence; no credential, key,
  e-mail or personal datum. All data is synthetic.

## What remains for the EMP-METER-002 closure

The assignment periods, their history and the assigned-asset query now exist;
METER-002 consumes them, nothing here changes the signed intake:

1. Signed intake (`organization/adapters/meter_authority.go`, device path of
   `CheckMeterTx`): for a member whose current grant for `meter.reading.record`
   is `SELF` (DRIVER, OPERATIONS_STAFF), call
   `AssignedAssetsPort.AssignedAtDBTX(ctx, tx, scope, member, asset, at)` on the
   borrowed intake transaction (it accepts `syncapp.DBTX`; the PostGIS tests call
   it through `syncapp.Borrow(tx)`), after the root and asset locks, and set
   `ResourceFacts.AssignedTo = member` when it answers true. Today that path
   passes facts without an assignee, so a SELF reading stays unsupported/held.
2. Historical classification (EDR-118/OFFL-008): evaluate the assignment at the
   declared observation instant together with the AUTH-005 role periods, so a
   reading declared while the asset was assigned but received after the
   assignment ended is HELD as `PERMISSION_CHANGED`, while a never-assigned
   asset keeps the never-authorized outcome; the port answers any past instant
   (membership and assignment periods are retained).
3. Real review-item tests for those SELF cases (actual assignments, terminated
   employee, disposed asset) on the composed signed matrix, which still counts
   "2 explicit SELF denials".
4. The web meter reads of an assigned asset are already covered here.

## Retained obligations

- Notifications to assignees (fleet.md §9: start, change, end): EDR-164 —
  recipients are memberships, assignments name employee records, and "when an
  assignment starts" needs a scheduled producer.
- EMP-METER-002 adoption (above); EMP-OFFL-006 reference feed (consumes
  `AssetsAssignedAtDBTX` and the own list semantics); EMP-FLEET-013 app asset
  picker; EMP-IDN-014 driver-profile eligibility on `VEHICLE` assignments
  (BR-FLEET-022 second clause); EMP-FLEET-007 reservations/blocks and derived
  availability; a pool by team (not inferred: TEAM grants cover no asset).
- BR-FLEET-021 "acting responsible" display and BR-FLEET-024 trip interplay
  (no trip exists yet; ending an assignment never touches a trip).
- Employee merge: the merged source record is `TERMINATED`, so its assignments
  end (rule-consistent); moving them to the survivor is a manager action.
- SELF on other permissions with S cells (trips, fuelings, expenses) when those
  records exist; signed intake keeps ORG-only until METER-002.
- Production grants (proposal 4), panel screens, Android and release remain
  separate. No M3/F3 acceptance is claimed.

## Proposed shared-doc changes

Not edited here; exact text for the coordinator.

1. Product contract: move the bullets between `CONTRACT:BEGIN`/`CONTRACT:END`
   verbatim under a new section after "EMP-AUTH-006 — Unit, cost-centre, team
   and asset scopes", preceded by
   `## EMP-FLEET-006 — Driver assignments: primary, temporary and multiple drivers (2026-11-14)`
   and the usual evidence link line pointing to this file.
2. `docs/enterprise/16-decisions.md`: insert the two rows between
   `EDR:BEGIN`/`EDR:END` in EDR number order.
3. `docs/enterprise/backlog/ep05-fleet-meters.md` EMP-FLEET-006: state `TODO` →
   `LOCAL_DONE` with the backlog note between `BACKLOG:BEGIN`/`BACKLOG:END`.
   EMP-METER-002 "Risks and assumptions" may add: "FLEET-006 delivers
   `AssignedAssetsPort` (`AssignedAtDBTX` on the borrowed intake transaction);
   the signed SELF adoption and historical HELD classification remain here."
4. `backend/README.md`, new subsection after "Structural role scopes (EMP-AUTH-006)":

   ```markdown
   ### Driver assignments (EMP-FLEET-006)

   Migration210 adds the forced-RLS `org_asset_assignments` (PRIMARY periods
   never overlap: a `btree_gist` exclusion) and `org_asset_assignment_commands`,
   two `AFTER UPDATE OF state` consumers that end assignments when an employee
   record becomes `TERMINATED` or an asset `DISPOSED`, and recognises the new
   asset key in `org_asset_current_usage`. The migration owner needs `TRIGGER`
   on `org_assets` and `org_employee_identities`, `REFERENCES` on
   `organizations`, `org_audit_events`, `org_assets` and
   `org_employee_identities`, and ownership of `org_asset_current_usage`. The
   Fleet principal needs `SELECT, INSERT, UPDATE ON org_asset_assignments`,
   `SELECT, INSERT ON org_asset_assignment_commands` and
   `SELECT (org_id, id, employee_id, started_at, ended_at) ON org_memberships`
   plus `SELECT (org_id, id, state, revision) ON org_employee_identities`.
   Every principal that terminates an employee record (offboarding, employee
   merge) or disposes an asset needs
   `SELECT (org_id, employee_id, asset_id, valid_from, valid_to),
   UPDATE (valid_to, end_cause, source_revision, end_operation_id, ended_by,
   end_reason) ON org_asset_assignments`. DELETE/TRUNCATE are refused by
   triggers regardless. Commands use fresh `fleet.asset.assign` (admission class
   `fleet.assignment`); reads use current `fleet.asset.read`, whose SELF grants
   now cover assigned assets. Nothing here provisions live grants.
   ```
5. `docs/enterprise/modules/fleet.md` §8 (after "Period invariants are
   exclusion constraints"): "Assignment endings caused by an employee
   termination or an asset disposal happen in the transaction of that change
   (EDR-165); an assignment ended before its start keeps an empty period."
   §7 Scope sentence: "`SELF` covers assets with an `ACTIVE` assignment to the
   member (EMP-FLEET-006: `fleet.asset.read` only; the member's own assignment
   list is the member's own record)."
6. `docs/enterprise/07-permissions.md` §8, after the EMP-AUTH-006
   interpretation: "EMP-FLEET-006 interpretation (2026-11-14, EDR-165): a
   current `SELF` grant with a positive `fleet.asset.read` rule covers an asset
   while it is assigned to the member at the evaluated instant (membership and
   assignment periods both contain it); no other permission gains SELF
   coverage from an assignment."
7. `scripts/check-manifest.txt`: no change (no new package).
8. Integration note: 000210 replaces `org_asset_current_usage` on the 208 body
   and includes the EMP-DOC-003 line as
   `OR(c.conrelid=to_regclass('public.org_document_links') AND c.conname='org_document_links_asset_fkey')`
   so the body is valid with and without 000209 (equivalent once 209 is
   applied); `org_document_links_employee_fkey` needs no change here because no
   employee usage source exists. Re-run the union after integration.
