# EMP-AUTH-006 — Unit, cost-centre, team and asset scopes with resource scope resolution

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Sole bounded F2 task in worktree `codex/enterprise-f3-auth006`, opened from the
integrated head that contains FLEET-003 (201), PRIV-005 (202), APPR-006 (203),
NOTIF-001 (204), DOC-002 (205) and FLEET-005 (206, the
`UnitAndCostCentreAt` snapshot port). It is the third link of the EDR-109 chain
FLEET-003 → FLEET-005 → AUTH-006 → FLEET-006 → METER-002 closure and unblocks
EMP-APPR-002 (routing by scope) and unit-scoped finance periods.

Read: task block `ep03-identity-access.md` EMP-AUTH-006 and its consumers
EMP-APPR-002 (`ep10`) and EMP-FLEET-006 (`ep05`); `07-permissions.md` §3, §4
(BR-AUTH-004/005/007/013), §5 UC-AUTH-01/02, §7.3, §8, §10 (decision order and
example 3); `modules/organizations.md` §3/§4 (BR-ORG-008/009/011/012);
`04-business-rules.md` BR-TEN-005; evidence of AUTH-002/003/009, ORG-005/006/007,
FLEET-005 (EDR-154/155) and APPR-006 (EDR-148/149). Source: `orgaccess/**`
(scope types, `Prepare`, `CoveringRules`, role management), migrations
000171/000183 (role assignments), 000174/000175/000177 (structure usage and
admission sources), 000197/000199/000201/000206 (assets, held meter readings,
lifecycle, allocation periods) and the company authority adapters.

Shared canonical docs are not edited here: the rule/use-case bullets, EDR rows
and backlog note are frozen between the markers below for the coordinator.

IDs used: B-BR-EMP-235..238, BUC-EMP-120..121, EDR-160..161 (all of them; no
pre-allocated ID is left unused). Migration `000208_*` (append-only; 000207 is
reserved by the parallel EMP-RPT-001 export task; every test derives the
version from one file-name constant and builds the previous chain as
`name < thisMigration`).

## Rules frozen before behaviour

Current storage observed before any change: `org_role_assignments.scope_type`
accepts only `ORG` (target = organization) and `SELF` (target = membership);
`OWNER` is `ORG` only and station roles are refused. `CoveringRules` knows
`ORG` and `SELF`; every resource resolver supplies only the record ID (and the
author for documents/ownership). The only delivered record type that stores a
structural snapshot is the held meter reading (`org_meter_held_readings`,
migration 199): the asset and the unit the asset had when the reading was
held (the legacy single-unit column, equal to the FLEET-005 open period until
EMP-FLEET-008 introduces transfers), plus the review item's author employee and
the acknowledgement receipt instant. No delivered record stores a cost centre.
Assets resolve their allocation through the FLEET-005 port. Team links are
retained periods (`org_team_links.started_at/ended_at`, migration 177).

Rules B-BR-EMP-235, B-BR-EMP-236, B-BR-EMP-237, B-BR-EMP-238, BUC-EMP-120, BUC-EMP-121 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-AUTH-006 — Unit, cost-centre, team and asset scopes with resource scope resolution" (frozen before behaviour; text moved verbatim at integration).

### Decisions (EDR rows, frozen before behaviour)

Decisions EDR-160, EDR-161 are recorded in [16-decisions](../16-decisions.md).

### Backlog note


## Implementation

- Migration `000208_role_structural_scopes.sql` (append-only): replaces
  `org_role_assignments_scope_type_check` (now `ORG`, `SELF`, `UNIT`,
  `COST_CENTRE`, `TEAM`, `ASSET`) and the 171 shape check `_check1` (now
  `org_role_assignments_scope_target_check`: ORG = organization, SELF =
  membership, structural ≠ both); four stored generated columns
  `scope_unit_id`, `scope_cost_centre_id`, `scope_team_id`, `scope_asset_id`
  with composite FKs `org_role_assignments_scope_{unit,cost_centre,team,asset}_fkey`;
  BEFORE INSERT trigger `org_role_assignment_structure` (WHEN structural; fires
  after `org_role_assignment_revision`, which holds the account and root
  locks): organization scope (42501), shape (23514), target found under RLS in
  the same organization and of the named kind (23503,
  `org_role_assignments_scope_target_fkey`), target live (23514: unit/cost
  centre/team `ACTIVE`; asset `ACTIVE`, `INACTIVE` or `DISPOSED`). `CREATE OR
  REPLACE` of `org_asset_current_usage`, `org_unit_current_usage`,
  `org_cost_centre_current_usage` (206 bodies) and `org_team_current_usage`
  (177 body), each adding only its new FK to the allow-list; counts unchanged.
- Domain `orgaccess/domain`: `UnitScope`, `CostCentreScope`, `TeamScope`,
  `AssetScope`, `ScopeType.Structural`, `ScopeWithin`; `RehydrateAssignment`
  admits structural scopes with a canonical target that is neither the
  organization nor the own membership (OWNER stays ORG, station roles
  refused); `ResourceFacts` gains `UnitID`, `CostCentreID`, `AssetID`,
  `Teams` (validated: canonical, no duplicate team) and `WithStructure`
  (merges only those four fields after proving the same record, authorship and
  restriction facts); `StructuralCandidate`; `CoveringRules` applies
  `scopeCovers` (ORG all, SELF own, equality for UNIT/COST_CENTRE/ASSET,
  membership for TEAM, empty never matches) intersected with own-records `S`;
  `CanAssignScopedRole` accepts structural scopes at organization coverage.
- Application `orgaccess/application`: `StructureSource` and optional
  `Authorization.Structure`; `Prepare` computes `StructuralCandidate` and, only
  then and only with a source, resolves the structure after the resolver (not
  found → `NOT_FOUND`, any other failure unavailable, identity change
  unavailable); the record gate receives a cloned team list. Role inputs admit
  structural scopes (organization or own-membership target refused as scope).
- Adapters `orgaccess/adapters/role_store.go`: owner self-change counts as a
  reduction only when `ScopeWithin(old, new)` (closes UNIT→ORG and
  UNIT→other-UNIT widening; SELF→ORG stays refused). Admission errors map
  through the existing store codes (23503 → `NOT_FOUND`, 23514 →
  `VALIDATION_FAILED`).
- Structure lookup `organization/adapters/resource_structure.go`:
  `ResourceStructure{Kind, Pool, Allocations}` with `AssetStructure`
  (FLEET-005 `UnitAndCostCentreAtTx`; unallocated → asset only; missing →
  not found) and `HeldMeterReadingStructure` (one statement: held asset and
  unit, author employee's `org_team_links` containing the acknowledgement
  `received_at`). The company authority re-check (`companyAuthority.check`)
  applies it in the owning transaction with database time; `FleetAuthority`,
  `MeterAuthority` (company path only) and `MeterReviewAuthority` carry it.
  Organization-level targets (the organization root) resolve to no structure.
- Composition `enterprisesuite/routes.go`: one `AssetStructure` source (the
  asset store as port) for the asset, lifecycle, allocation-history and meter
  route group and the fleet/meter authorities; one
  `HeldMeterReadingStructure` source for the meter review routes and their
  authority. No route, permission or catalog version is added.
- Contract `contracts/openapi/v1.yaml`: the role assign/change `scope` enum and
  the view/result `scope_type` enum list the six scopes; assign/change
  descriptions state the target rules. Route inventory, permission catalog
  and matrix cell counts are unchanged (routed 19, signed 1, not routed 65,
  normal 98, special 24; 190 cells, 72 allowed / 118 denied); the matrix
  gains the `unit-scope` variant (every organization-level representative is
  `FORBIDDEN` for a UNIT-scoped grant; OWNER skipped as not persistable).
- Test fixtures adapted (no assertion weakened): the synthetic unit and
  cost-centre usage overrides allow-list the new FKs; the AUTH-005 domain
  assertion that `UNIT` was not yet grantable became the station-scope
  refusal (UNIT admission is now asserted positively in
  `structural_test.go`).
- Unchanged on purpose: invitation grants and context hints (ORG/SELF), the
  signed meter intake (non-ORG remains unsupported), every list/root resolver,
  documents, finance periods, the role catalog and route inventory.

## Acceptance and negative mapping

| Item | Evidence |
|---|---|
| Example 3 at a real entry point (held meter reading review, `POST .../sync/reviews/meter-readings/{id}/read`) | `TestStructuralScopesActualCompanyHTTPExampleThree/march`, `/april`: a reading held while V was in Norte stays readable by the Norte-scoped role and refused to the Sul-scoped role after V moves to Sul; the asset itself (`.../fleet/assets/{id}/read`, allocation history) becomes readable by Sul and refused to Norte; the decision (`/resolve`) is refused to Sul and recorded by Norte |
| Reassigned asset widening history (must not) | same test, `april` (Sul refused on the March record; cost centre Sul refused too) |
| Wrong cost-centre match | `march`/`april` (`COST_CENTRE` Norte vs Sul on the asset; any cost centre refused on the held reading, which stores none); domain `TestStructuralCoverageMatchesOnlyTheResolvedSnapshot` |
| Team membership granting access on its own (must not) | HTTP `march` (`teamMember` linked to team A with a team-B grant, `selfMember` linked to team A with SELF: both refused); domain team cases |
| Team history does not move | HTTP `author changes team` (team A keeps, team B never gains) |
| Foreign structure at grant | HTTP `grant command negatives` (foreign unit, cost centre as unit, unit as team → `NOT_FOUND`); SQL `TestStructuralScopeSchemaAdmitsOnlyLiveSameOrganizationTargets` (foreign unit/cost centre/team/asset, wrong kind, unknown → 23503) |
| Foreign structure at evaluation | HTTP unknown review/asset → `NOT_FOUND`; domain foreign facts → not found; SQL other organization sees no grant |
| Archived unit/cost centre/team targets; DRAFT/ARCHIVED asset | HTTP archived unit and DRAFT asset → `VALIDATION_FAILED`; SQL 23514 for archived unit, cost centre, team, DRAFT and ARCHIVED asset; archive not blocked and grants kept |
| Overlapping/concurrent grant changes | `TestStructuralScopeAdmissionSerializesWithArchiveAndOverlap` (observed lock wait via `pg_blocking_pids`, no sleep): identical concurrent grants → one 23505; archive-first → waiting grant 23514; grant-first → archive succeeds, grant kept; team archive-first → 23514; HTTP overlapping grant → `CONFLICT` |
| RLS without organization setting | SQL test (0 rows visible, insert 42501); upgrade test |
| Replayed grant command | HTTP replay → original id, `replayed: true`; same operation with another target → `OPERATION_ID_REUSED` |
| No escalation / self-change | HTTP structural `auth.role.assign` covers no role management; OWNER own UNIT→ORG and UNIT→other UNIT refused, ORG→UNIT allowed; domain `TestStructuralGrantsNarrowButNeverWidenTheGrantor`, `TestScopeWithinNeverWidens` |
| Organization-level resources not covered | HTTP lists → `FORBIDDEN`; matrix `unit-scope` variant over all 19 representatives × 9 roles |
| Populated upgrade 206 → 208 with failure and recovery | `TestStructuralScopePopulatedUpgradeKeepsAssignmentsAndAdmitsScopes` |

## Validation

Environment: worktree backend, Go 1.27.4 toolchain
(`/data/dev/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.4.linux-amd64/bin`),
`GOCACHE=/data/dev/cache/f3-go-build`, `GOTMPDIR`/`TMPDIR=/data/dev/cache/f3-tmp`,
`STATICCHECK_CACHE=/data/dev/cache/f3-staticcheck`; owned disposable PostGIS
control `127.0.0.1:32775` (PostgreSQL 18.6, PostGIS 3.6) used only through
orgtest isolated child databases, ordinary NOSUPERUSER/NOBYPASSRLS runtime
roles and, for the upgrade, an ordinary migrator role; the coordinator's
disposable S3 for the document packages. Logs in
`/data/dev/worktrees/f3-logs/auth006-*`.

### RED (failing first)

- Unit RED against compiled stubs (`Structural`/`ScopeWithin` false,
  `WithStructure` refusing, `StructuralCandidate` false, unchanged
  rehydration/coverage/grant/role-input rules): `go test -json -count=1
  ./internal/modules/orgaccess/domain/ ./internal/modules/orgaccess/application/`
  → `auth006-go-red-unit.jsonl`: 9 new top-level tests failed (7 domain, 2
  application; 20 subtests), 40 top-level / 1174 subtests passed.
- Contract RED (spec at `HEAD`): `go test -json -count=1 -run TestRoleContract
  ./internal/platform/apicontract/` → `auth006-contract-red.jsonl`:
  `TestRoleContractAdmitsExactlyTheStructuralScopes` failed, the existing role
  contract test passed.
- PostGIS schema RED (migration file absent): `go test -json -race
  -tags=integration -timeout=30m ./internal/modules/organization/adapters -run
  '^TestStructuralScope' -count=1` → `auth006-db-red-schema.jsonl`: 4/4
  failed (scope check `org_role_assignments_check1` 23514 on every
  structural grant; HTTP assign 422; migration file missing). A first attempt
  also exposed a test assumption (OWNER cannot grant SUPERVISOR at any scope,
  BR-AUTH-007); the HTTP test was corrected to ADMIN grants before this run.
- PostGIS behaviour RED (migration present, composition wiring and the
  self-change fix reverted to `HEAD`): same command →
  `auth006-db-red-behaviour.jsonl`: the example-3 HTTP test failed (the
  Norte-scoped role got `FORBIDDEN` on the March review: no structure source),
  the schema test failed on an expectation later aligned with the shape-first
  trigger; the concurrency and upgrade tests already passed (SQL only).
- The first affected union (`auth006-db-union-1.jsonl`) found one regression,
  `TestRoleAssignmentScopePeriodsAndHistory/unsupported` (a structural row
  targeting a membership reached the structure read and hit a missing
  privilege, 42501, instead of 23514); fixed by checking the target shape
  before any structure read, then the whole union was rerun.

### GREEN

- Focused PostGIS race with coverage: `go test -json -race -tags=integration
  -timeout=30m -count=1 -run '^TestStructuralScope|^TestResourceStructure'
  -coverpkg=./internal/modules/organization/adapters,./internal/modules/orgaccess/...
  ./internal/modules/organization/adapters` → `auth006-db-focused-final.jsonl`:
  5/5 top-level passed (HTTP example-3 test: 7 subtests), package 11.7 s.
- Affected PostGIS race union `go test -json -race -tags=integration
  -timeout=60m ./internal/modules/orgaccess/... ./internal/modules/organization/adapters
  ./internal/modules/fleet/... ./internal/modules/review/... ./internal/modules/document/...
  ./internal/modules/orgsync/... ./internal/modules/membership/...
  ./internal/platform/migrate/... -count=1` → `auth006-db-union-final.jsonl`:
  28 packages ok, 776 top-level / 6155 subtests passed, 0 failures, 0 skips
  (organization adapters 1176.3 s; wall 19 min 56 s), including the
  authorization matrix (still 190 cells, 72 allowed / 118 denied, plus 171
  `unit-scope` variant requests) and every existing role, unit, cost-centre,
  team, fleet, meter, review, document, intake and upgrade test.
- Unit race + coverage `go test -json -race -count=1 ./internal/modules/orgaccess/...
  ./internal/platform/enterprisesuite/ ./cmd/api/` →
  `auth006-go-green-unit-final.jsonl`: 76 top-level / 1242 subtests passed;
  `./internal/platform/apicontract/` (`-timeout=40m`; a first run under the
  concurrent union hit the default 10-minute timeout) →
  `auth006-go-green-apicontract.jsonl`: 72 top-level / 694 subtests passed,
  878.4 s. Authorization inventory unchanged (routed 19, signed 1, not routed
  65, normal 98, special 24).
- Untagged whole module `go test -count=1 ./...` →
  `auth006-untagged-all.log`: 206 packages ok, 76 without tests, 0 FAIL.
- Coverage (statements): `orgaccess/domain` 91.2% (new functions
  `Structural`, `ScopeWithin`, `CanAssignScopedRole`, `canonicalID` 100%,
  `RehydrateAssignment` 96.2%, `WithStructure` 87.5%, `scopeCovers` 87.5%,
  `StructuralCandidate` 83.3%), `orgaccess/application` 66.2% (`Prepare`
  84.2%), `organization/adapters/resource_structure.go` 80.0% PostGIS ∪ unit
  (uncovered: unavailable-source and corrupted-row fail-closed branches).
  Profiles `auth006-unit-orgaccess-coverage.out`,
  `auth006-unit-coverage-final.out`, `auth006-db-coverage.out`.
- Build/vet/format/static/generation: `go build ./...`, `go vet ./...`,
  `go vet -tags=integration ./...` clean; `gofmt -l ./internal ./cmd ./db`
  empty; `GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck` on orgaccess,
  organization, enterprisesuite, apicontract, fleet, cmd/api and with
  `-tags=integration` on orgaccess, organization/adapters and orgtest: no
  findings (`auth006-staticcheck.log`); `sqlc vet` and `sqlc diff` clean (no
  query or stanza change).
- OpenAPI: `vacuum lint -r contracts/openapi/vacuum-rules.yaml
  contracts/openapi/v1.yaml --no-update-check` → 0 errors, 937 warnings / 605
  informs, identical to `HEAD` (`auth006-vacuum.log`).
- `git diff --check` clean; `python3 docs/enterprise/tools/check_plan.py` →
  0 problems. Secret/PII scan `git diff --cached | grep -nEi
  'password|secret|token|BEGIN .*KEY|@gmail|cpf'`: only the synthetic fixed
  test cursor secret (`bytes.Repeat([]byte{67}, 32)`, as in the FLEET-005 and
  meter-review HTTP tests) and this sentence; no credential, key, e-mail or
  personal datum. All data is synthetic.

## Retained obligations

- Structure sources for the other record types when they store a snapshot
  through the FLEET-005 port (EMP-FUEL-002 fuelings, EMP-EXP-002 expenses,
  trips, document links EMP-DOC-003) and unit/cost-centre scoped finance
  periods (APPR-006 periods stay organization-wide); EMP-APPR-002 consumes
  this coverage for routing by scope. Until a record type gets a source, a
  structural grant covers none of its records (fail closed).
- Scoped list filtering: organization-level lists (assets, reviews, roles,
  employees…) remain `FORBIDDEN` to a member whose only grant is structural;
  filtering a list by scope is not invented here.
- Assignee facts and `SELF` on assets (EMP-FLEET-006 assignments); TEAM
  coverage of assets assigned to a team is not inferred (no storage; a pool
  by team is out of FLEET-006).
- The held reading's unit comes from the legacy single-unit column equal to
  the open FLEET-005 period; EMP-FLEET-008 (transfers) re-points it to the
  period snapshot at hold time.
- Creating a record into a structure (for example asset creation by a
  UNIT-scoped fleet manager) targets the organization root and stays
  refused; it needs the owning command to carry the target structure.
- Signed offline intake still treats non-`ORG` assignments as unsupported
  (held for review, unchanged); panel scope picker (SCR-AUTH-02), Android,
  delegation (EMP-AUTH-007), `STATION` scope and production grants (proposal
  3 below) remain separate. No M3/F3 acceptance is claimed.

## Proposed shared-doc changes

Not edited here; exact text for the coordinator.

1. Product contract: move the bullets between `CONTRACT:BEGIN`/`CONTRACT:END`
   verbatim under a new section after "EMP-FLEET-005 — Unit and cost-centre
   allocation history", preceded by
   `## EMP-AUTH-006 — Unit, cost-centre, team and asset scopes (2026-11-14)`
   and the usual evidence link line pointing to this file.
2. `docs/enterprise/16-decisions.md`: insert the two rows between
   `EDR:BEGIN`/`EDR:END` in EDR number order. `ep03-identity-access.md`
   EMP-AUTH-006: state `TODO` → `LOCAL_DONE` with the backlog note between
   `BACKLOG:BEGIN`/`BACKLOG:END`.
3. `backend/README.md`, new subsection after "Allocation history
   (EMP-FLEET-005)":

   ```markdown
   ### Structural role scopes (EMP-AUTH-006)

   Migration208 widens `org_role_assignments` to `UNIT`, `COST_CENTRE`,
   `TEAM` and `ASSET` scopes with generated same-organization foreign keys
   and an insert admission trigger, and lets the unit, cost-centre, team and
   asset usage sources recognise the new keys (assignments are not use). The
   migration owner needs ownership of `org_role_assignments` and of the four
   `*_current_usage` functions and `REFERENCES` on `org_units`,
   `org_cost_centres`, `org_teams` and `org_assets`. The role-management
   principal additionally needs `SELECT (org_id, id, state)` on those four
   tables for structural grants. The meter-review principal needs
   `SELECT (org_id, team_id, employee_id, started_at, ended_at) ON
   org_team_links`; the fleet principal reuses its FLEET-005 snapshot-port
   grants. Nothing here provisions live grants.
   ```
4. `docs/enterprise/07-permissions.md` §8, after the EMP-AUTH-002 persistence
   interpretation paragraph:
   "EMP-AUTH-006 interpretation (2026-11-14, EDR-160/161): `UNIT`,
   `COST_CENTRE`, `TEAM` and `ASSET` assignments name one live structure of
   the same organization (foreign → `NOT_FOUND`, archived or `DRAFT` asset →
   `VALIDATION_FAILED`); archiving neither ends nor is blocked by them.
   Coverage uses the record's stored snapshot (held meter readings today) or
   an asset's FLEET-005 allocation at the evaluated instant; `TEAM` covers
   records whose author or assignee was linked to the team at the record's
   instant. Organization-level resources carry no structure. A structural
   `auth.role.assign` manages no roles; an owner may only keep or narrow an
   own assignment's scope." In §12, replace the sentence "Finer
   UNIT/COST_CENTRE/TEAM/ASSET grant management belongs to AUTH-006 (F2)."
   by "UNIT/COST_CENTRE/TEAM/ASSET grants are managed through the same role
   routes since AUTH-006 (B-BR-EMP-235..238)." Status line: add
   "EMP-AUTH-006 LOCAL_DONE / INTEGRATION_PENDING".
5. `docs/enterprise/modules/organizations.md` §4, append to BR-ORG-011:
   "A `TEAM` assignment covers records whose author or assignee was linked to
   the team at the record's instant; later link changes never move history
   (B-BR-EMP-237)." and to the status paragraph: "TEAM role scopes:
   EMP-AUTH-006."
6. `scripts/check-manifest.txt`: no change (no new package). The authorization
   inventory and its count assertions are unchanged.
7. Integration note: 000208 replaces the latest bodies of four usage
   functions (206 and 177). If the parallel 000207 (EMP-RPT-001) also
   replaces one of them, 208 must be rebased on 207's body before merge.
