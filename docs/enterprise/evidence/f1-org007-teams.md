# F1 teams and employee-link periods — EMP-ORG-007

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Date: 2026-11-12
Source parent: IDN-002 `09639f41`, owned enterprise execution worktree,
`codex/enterprise-f0-f1`, targeting existing PR202 (`dev` → `main`).
F1 remains open; this task checkpoint is not phase publication, integration,
current-head/base CI evidence or production acceptance.

Tested backend tree: `e6b4f6089981372089ed96299ed734d26cd07082`.
Tested contract Git blob: `5400c1262323e26084257a1e8e2f4d1466751a07`.
The enclosing task commit pins these runtime inputs; closure documentation does
not change them.

## Contract and decisions

B-BR-EMP-65/66 and BUC-EMP-33 preceded behavior. EDR-042 records the
recommended retained-period, unit and reduction choices. ORG-003 and the
actual IDN-002 register are local dependency checkpoints. Employee references
may honestly have a pending descriptive profile; no name or login is invented.

A team has a stable scoped identity, private trimmed name, optional scoped
unit, ACTIVE/ARCHIVED state and revision. Only ACTIVE names are unique,
case-insensitively. Archived metadata can be corrected before restoration;
no name/code ledger, team hierarchy or implicit grants are invented. A newly
selected unit and restoration's retained unit must be ACTIVE. An empty active
team can retain its archived unit reference but accepts no new employee links
until that reference is changed/cleared or the unit is restored.

Employee/team links are retained periods with independent IDs, server start/end
instants and a static closure cause. At most one current period exists for an
employee/team. Removing ends it; adding again creates another period. Employee
inactivation/termination and membership suspension/end atomically close current
periods and increment the affected team revision. Reactivation restores none.
Causal employee/membership ID/revision facts are recorded at the actual source
transition; no manager identity or source audit event is fabricated when the
underlying lifecycle command has not supplied one. The future IDN lifecycle
commands retain their own audit/session/device obligations.

Joining a team changes no account, membership, role, delegation or permission.
Existing ORG/SELF role periods remain explicit. TEAM role-scope admission and
coverage still belong to AUTH-005; unavailable STATION scopes remain deferred.
The actual composed DRIVER test proves that joining creates no management power.

## Implementation and invariants

Migration `000177_organization_teams.sql` adds forced-RLS teams, retained link
periods and immutable command acknowledgements. Composite keys/FKs preserve
scope and ensure a link acknowledgement belongs to the same team. Static
P8001/P8002/P8003/P8005/P8006/P8007 refusals prevent private constraint details;
P8004 marks unknown/malformed usage unavailable. Names also reject stored
noncanonical Unicode edge whitespace, matching the application's trimming.
No applied migration is edited and no dependency/service is added.

All functions are invoker functions with pinned search paths. The existing
account-erasure definer already selects, locks and restores each organization
scope. Its membership-table owner receives only SELECT/UPDATE on the two new
projection tables it consumes. No new definer, unscoped RLS exception, PUBLIC
privilege or runtime login is created. Deployment still needs reviewed ordinary
runtime grants; fixture grants are not production provisioning. A two-organization
rehearsal reowns the existing definer to an ordinary non-login/NOBYPASSRLS role
and proves both organizations' current links close, retain causes and restore
the prior scope. Existing personal-erasure fixture DML privileges are explicit
and separate from that protected definer's restricted permissions.

Configuration reuses the current company authority's account/proof → capability
→ root → membership/role lock order and proof recheck after waits. The application
also requires the request-bound AUTH decision. Employee state is resolved through
the owning membership transaction port; no generated SQL package crosses owners.
Team metadata, link creation/end, revision, registered `org.team.changed` audit
and original acknowledgement commit together. Domain/SQL postconditions agree
on the resulting team snapshot. Exact retries preserve their original state,
revision and link ID after later edits or reductions; changed intent and stale
revisions mutate nothing. A failed deferred commit returns unavailable with no
provisional acknowledgement.

Actual ACTIVE employees in current links block team archive. The owning source
extends unit usage with the **distinct union** of ACTIVE home-unit employees
and ACTIVE employees in current links of teams associated with that unit. One
person in both paths or several teams is counted once; empty labels and ended
periods do not count. Unknown FK consumers still fail closed for teams and units.
Configuration admission, reductions and archive share the root lock. Future
operational sources must extend the actual guard and use `org_team_admit`, which
requires an ACTIVE root/team and any associated ACTIVE unit; no operational
budget/asset/work ledger is fabricated in this task.

## Entry points

Nine company-cookie POST routes are composed by the actual default-off router:

- `/v1/orgs/{org_id}/teams/create`
- `/v1/orgs/{org_id}/teams/list`
- `/v1/orgs/{org_id}/teams/{team_id}/read`
- `/v1/orgs/{org_id}/teams/{team_id}/change`
- `/v1/orgs/{org_id}/teams/{team_id}/archive`
- `/v1/orgs/{org_id}/teams/{team_id}/restore`
- `/v1/orgs/{org_id}/teams/{team_id}/members/{employee_id}/add`
- `/v1/orgs/{org_id}/teams/{team_id}/members/{employee_id}/remove`
- `/v1/orgs/{org_id}/teams/{team_id}/members/list`

Current `org.structure.manage` is mandatory. Writes require fresh proof, the
current `organizations` flag and the existing shared synthetic `org.structure`
200/hour org/account/web-or-device allowance. This is not measured live capacity.
Company cookie/CSRF, exact HTTPS origin/host, strict DTOs, no-store and no ETag
remain mandatory. Link pages expose scoped employee references and static causes,
not names/contacts/accounts. Opaque link cursors bind the exact team as well as
organization, actor, membership, route, state and page size. Twelve explicit
OpenAPI schemas/examples cover command, team and period views. Existing contract
content is retained; new entries are placed at section ends for a smaller diff.
A semantic-map equality check and route/example drift checks verify the reorder.

## Validation

Go 1.27.4; sqlc 1.31.1; staticcheck 0.8.1 with Go 1.27.1; vacuum 0.30.6.
Disposable PostgreSQL 18/PostGIS 3.6 with the pinned image. Ordinary runtime and
migration/definer owners were used for scoped operations. Administration creates
synthetic fixtures and injected failures only; no production data or endpoint
was used.

- Domain RED: team/profile/period symbols absent. GREEN/race: scoped identity,
  member-state facts, retained periods, stale revisions, archive/restore,
  no archived admission, private formatting and unforgeable application decisions.
- Real PostGIS/race: scoped active-name uniqueness and reuse after archive;
  twelve competing name creations, exact retries, current-link additions and
  profile CAS; twelve team archive/link races; twelve direct schema unit-archive
  versus team-link races (independent of actor/account or team revision locks).
  The latter must return either active-use refusal or inactive-reference refusal,
  and never commit an employee link in an archived unit.
- Real usage: one employee with a home unit and two same-unit teams counts once;
  inactive employees close current periods, empty teams allow unit archive,
  archived unit references reject new links/restoration, and correction allows
  later restoration. Unknown new FK consumers refuse archive even with no rows.
- Membership suspension/end, employee reductions and ordinary protected account
  erasure retain periods/causal revisions and advance team revision. Reactivation
  never revives links. Source reduction failure rolls membership state, role
  endings, team revision and period back together; the same command recovers.
  This also passes with a SECURITY-suspended organization, exercising the bounded
  metadata-only revision exception for safety cleanup.
- Deferred team/link receipt faults roll team/link/audit/ack back and return
  unavailable. Original create/add acknowledgements remain stable after edits,
  period removal and new periods. Team membership never modifies role rows.
- Composed company HTTP: cookie/CSRF/origin/alternate-proof and privileged-body
  refusals, real DRIVER denial after joining, archived display/admission, scoped
  employee/unit refusal, history and cross-team/filter cursor refusal, current
  pilot disable/read retention, deferred failure and actual family revocation.
- Populated previous-chain upgrade, with unit and employee data: ordinary
  migrator reaches an injected division failure, rolls DDL/history back, recovers
  with only 000177 and repeats as a no-op. Forced RLS/ordinary ownership hold;
  previous schema check and identity/profile repository reads remain compatible.
  New links preserve prior identity/profile revisions and home-unit data.
- Complete affected organization, membership, access, audit, capability,
  company-session, gateway and composition suites pass under PostGIS/race.
  New source grants in explicit fixtures preserve previous-chain grants; synthetic
  usage extensions retain actual home/team counts. Destructive audit/unit tests
  include the new FK dependents and retain their original append-only assertions.
- Full OpenAPI suite passed (226 seconds under composed coverage), plus actual
  production-router/default-off and targeted schema/example/guard-drift checks.
  Canonical vacuum: zero errors, unchanged 785 warnings and 356 informational
  items (30 additional duplicate descriptions).
- Scoped vet/staticcheck, API/worker/ops build, sqlc generate/vet, strict changed
  gofmt and diff checks pass. Composed organization coverage: domain 90.92%,
  application 76.18%, adapters 76.78%, HTTP 78.43%. This is scoped evidence,
  not whole-project/native acceptance. The later targeted source-failure/race
  cases add evidence without rerunning an unchanged aggregate suite.
- Plan checker: 348 tasks/545 rules/167 use cases/145 screens, zero structural
  problems; three checker tests pass. Structural validity is not acceptance.
  Added local links and staged secret surface are checked at task commit.

Representative scoped commands from the owned root, with the explicit disposable
PostGIS DSN set in the environment:

```sh
GOCACHE=/tmp/enterprise-go-build go -C backend test -race -tags=integration -p 2 \
  -coverpkg=./internal/modules/organization/... \
  -coverprofile=/tmp/enterprise-org007-coverage.out \
  ./internal/modules/organization/... ./internal/modules/membership/... \
  ./internal/modules/orgaccess/... ./internal/modules/orgaudit/... \
  ./internal/modules/orgcapability/... ./internal/modules/companysession/... \
  ./internal/platform/orgapi ./internal/platform/enterprisesuite \
  ./internal/platform/apicontract
GOCACHE=/tmp/enterprise-go-build go -C backend test ./cmd/api \
  -run '^TestFullProductionRouterEnterpriseContractAndDefaultOff$' -count=1
TZ=America/Cuiaba python3 docs/enterprise/tools/check_plan.py --write
python3 -m unittest discover -s docs/enterprise/tools -p 'test_*.py'
```

## Next and retained gates

ORG-008 settings is the next eligible organization task (ORG-003/PLAT-015 are
local checkpoints). F1/M1, completed-phase publication, final current-head/base
CI, actual provider/ordinary runtime grants, native/emulator consumer acceptance,
measured admission/cleanup, vault/staging/Kotlin coverage and real-data/legal
gates remain open. PLAT-012 still depends on GOV-014. This does not certify
TEAM role scopes, full IDN offboarding, integration, release or deployment.
