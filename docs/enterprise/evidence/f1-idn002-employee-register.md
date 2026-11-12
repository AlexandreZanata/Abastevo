# F1 employee register without login — EMP-IDN-002

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Date: 2026-11-12
Source parent: ORG-006 `067a934d`, owned enterprise execution worktree,
`codex/enterprise-f0-f1`; delivery target is existing PR202 (`dev` → `main`).
This task checkpoint is part of the open F1 construction phase. Phase publication,
final current-head/base CI, integration and production acceptance remain due.

Tested backend tree: `e4a6ea6eaa7db6aee238ff28c56d4c465d5454a2`.
Tested contract Git blob: `afc3463604433e15c8192d0f8ad0f26c136e1200`.
The enclosing task commit pins these objects; documentation-only closure does
not change the tested runtime inputs.

## Contract and decisions

B-BR-EMP-63/64 and BUC-EMP-32 preceded behavior. EDR-041 records the
recommended minimal profile and duplicate-review choices. Dependency review
found that ORG-007 requires IDN-002, so this real register precedes teams.
Team affiliation is owned by ORG-007; there are no placeholder teams or grants.

`membership` owns private employee profiles; the existing public contributor
`identity` module retains its separate purpose. Profiles have independent
revisions from the authoritative employee lifecycle. Old identities explicitly
remain profile-pending until a manager supplies their name. Account aliases and
contacts are never copied to invent employee data. Creation makes an ACTIVE
employee and profile, with no account, membership, role, credential, device or
licence eligibility. Completion and correction preserve identity/access state.
Employee state is readonly here; IDN-008/009/011 retain their corresponding
membership, offboarding and rehire commands and mandatory lifecycle guards.

Only display name is required. Registration, work email/phone, job title and
home unit have the purposes in identity §3. Text is trimmed and bounded in
UTF-8 bytes, with control/bidi refusals. Optional work email uses a bounded
ASCII single-`@` shape and lower case; optional phone uses international `+`
and 8–15 digits. These are unverified contact conventions, not proof of account
ownership, deliverability or phone control. CPF, birthdate, address, photo,
licence contents and authority fields are absent from these DTOs.

A nonempty registration is compared trimmed/case-insensitively within the
organization across all current employee states. Name plus a matching nonempty
work email **or** phone requires explicit review; name alone/contact alone
never deduplicates. Name comparison folds case and repeated ASCII spaces.
The authorized review endpoint returns bounded scoped candidates. A write
without acknowledgement returns private `DUPLICATE_REVIEW_REQUIRED` (409),
without names, contacts or candidate IDs in its error. `allow_possible_duplicate`
is an explicit manager create-anyway decision, included in command digest and
immutable receipt/audit reason. It never overrides exact registration uniqueness
or merges/links accounts. No signed review token or identity matching provider
is invented. Current registration corrections do not create a permanent alias
ledger; the permanent-code rule belongs to cost centres.

## Implementation

Migration `000176_employee_profiles.sql` adds forced-RLS profiles, immutable
command acknowledgements and immutable causal unit-clearance records. Composite
keys/FKs preserve organization identity. Existing identity and membership guards
are unchanged. Private malformed/duplicate SQL refusals use static P7902/P7901
codes before PostgreSQL can include descriptive data in a constraint detail.
All new functions are invoker functions with pinned search paths.

The identity application declares an explicit employee-authorization transaction
port. The organization adapter implements it with the proven account → current
company proof → capability → root → membership/role order and post-wait proof
recheck. No generated SQL package crosses its owner boundary. A request-bound
current AUTH decision is also required at application dispatch; actor metadata
alone cannot authorize. OWNER/ADMIN manage, and the literal employee-read row
controls other reads. Finer UNIT/TEAM assignments remain AUTH-005; STATION roles
remain unavailable until their own authority-scoped admission task.

Six company-cookie POST entries are composed by the actual default-off router:

- `/v1/orgs/{org_id}/employees/create`
- `/v1/orgs/{org_id}/employees/{employee_id}/complete`
- `/v1/orgs/{org_id}/employees/{employee_id}/change`
- `/v1/orgs/{org_id}/employees/{employee_id}/read`
- `/v1/orgs/{org_id}/employees/list`
- `/v1/orgs/{org_id}/employees/matches`

Fresh management writes use the current `organizations` pilot flag and their
own `idn.employee` admission class: the existing synthetic identity allowance
of 200/hour on each org/account/web-or-device axis. This is not measured live
capacity. Read permission is `idn.employee.read`; possible-match discovery uses
`idn.employee.manage` with a read effect. Exact HTTPS origin/host, company
cookie/CSRF, strict JSON, no-store/no ETag and current proof remain mandatory.
UUID keyset cursors bind organization, actor, membership, route and filters;
match cursors carry a digest of private filters rather than their contents.
Nine explicit OpenAPI schemas/examples and the full-router guard cover these
entries. The error schema also names the existing LAST_UNIT/STRUCTURE_IN_USE
codes and the new registered duplicate-review refusal.

Identity/profile/audit/ack commit together. Server-chosen IDs and original state,
identity revision and profile revision are durably acknowledged. Exact retry
returns that acknowledgement after later edits; changed intent, stale revision,
foreign reference and failed commit leave no provisional data or result.

## Real home-unit source and archive consumption

The owning migration extends `org_unit_current_usage` with actual ACTIVE employee
profiles, including employees without login. Unknown FK consumers still refuse
archive. Configuration admission and archive share the root lock; a newly
selected unit must be ACTIVE in the same organization, including setup.

A successful audited unit archive clears remaining nonactive current home links,
increments only profile revision and inserts causal clearance evidence. Its
deferred composite FK requires the same unit ARCHIVE command, operation and
result revision. The cause is the existing `org.unit.archived` audit event;
no personal data is copied to audit. Inactive, terminated and retained merged
profiles preserve their identities/history. Unit restoration never reconstructs
cleared links, and stale profile edits cannot overwrite a clearance. No new
queue/service or speculative usage ledger is introduced.

The deferred-failure test reproduced a 404 for a failed unit archive commit.
`UnitStore` now distinguishes a successful callback followed by commit failure
and returns unavailable with zero result/replay. The profile, unit, causal
clearance, audit and receipt all roll back; the same operation can recover.

## Validation and environment

Go 1.27.4; sqlc 1.31.1; staticcheck 0.8.1 with Go 1.27.1; vacuum 0.30.6.
Disposable local PostgreSQL 18/PostGIS 3.6, pinned PostGIS image; ordinary runtime
login and ordinary non-login/NOBYPASSRLS migration owner. Administrative pools
set up synthetic fixtures and fault injection only. No production database,
credentials, employee data or storage was used.

- Domain RED: profile/record symbols absent. GREEN: minimal profile, independent
  lifecycle revision, scoped snapshots, pending completion, stale revision,
  private formatting and invalid/excess input under race detection.
- Real PostGIS/race: twelve concurrent registration reservations, likely matches,
  exact retries and profile CAS; twelve unit archive/admission races; distinct
  organization registration reuse; inactive/terminated registration protection;
  foreign employee/home unit; no access provisioning; pending creator completion;
  original replay after edits; explicit review without merge; failed audit/late
  commit rollback and recovery; real usage counts, clearing/restoration and
  retained merged history. Static SQL refusal regression reproduces before fix.
- Actual composed company HTTP: cookie/CSRF/origin/alternate proof refusal,
  forbidden body fields, current OWNER plus all other currently persistable role
  matrix cells, duplicate warning/candidates/acknowledgement, read/manage
  distinction, cursor isolation, pilot disable/read retention, failed commit and
  actual source-family revocation (including old retries).
- Populated previous-chain upgrade with an ordinary migrator: injected division
  failure rolls DDL/history back; retry applies only 000176, repeated apply is a
  no-op, forced RLS/ordinary ownership hold, previous schema check and old employee
  repository reads remain compatible. Existing identities remain pending; ordinary
  profile completion/new registration work without changing legacy lifecycle.
- Affected membership, organization, access, audit, capability, company-session,
  gateway and composition suites ran under real PostGIS/race. The first broader
  run exposed test fixtures unaware of the new FKs; their explicit destructive-
  grant tests now reach the same append-only trigger, and the synthetic usage
  extension retains actual employee counts plus unknown-consumer refusal. All
  five reproduced cases passed; both affected organization/audit adapter packages
  were then rerun completely and passed. No production grants or assertions were
  weakened. Remaining affected packages passed in the broader run.
- Full OpenAPI suite passed (195 seconds under composed coverage); new schema
  examples reject extra authority, and changed cookie/policy/selector metadata
  fails the route gate. Actual API production-router/default-off test passed.
- Scoped vet/staticcheck, API/worker/ops build, sqlc generate/vet, strict changed
  gofmt and diff checks passed. Canonical vacuum: zero errors, unchanged 785
  warnings, 326 informational items (39 added descriptions classified duplicate).
- Composed membership coverage: domain 96.40%, application 68.09%, adapters 83.26%,
  HTTP 75.76%; aggregate 84.6%. This is scoped evidence, not whole-project or
  native acceptance. Plan checker: 348 tasks/545 rules/167 use cases/145 screens,
  zero structural problems; its three tests pass. Structural validity is not
  functional acceptance. Added local links and staged secret review are checked
  at task commit.

Representative commands from the owned root, with the disposable DSN explicitly
set in the environment (not production):

```sh
GOCACHE=/tmp/enterprise-go-build go -C backend test -race -tags=integration -p 2 \
  -coverpkg=./internal/modules/membership/... \
  -coverprofile=/tmp/enterprise-idn002-coverage.out \
  ./internal/modules/membership/... ./internal/modules/organization/... \
  ./internal/modules/orgaccess/... ./internal/modules/orgaudit/... \
  ./internal/modules/orgcapability/... ./internal/modules/companysession/... \
  ./internal/platform/enterprisesuite ./internal/platform/orgapi \
  ./internal/platform/apicontract
# After the fixture failures above were corrected:
GOCACHE=/tmp/enterprise-go-build go -C backend test -race -tags=integration -p 2 \
  -coverpkg=./internal/modules/membership/... \
  -coverprofile=/tmp/enterprise-idn002-repaired-coverage.out \
  ./internal/modules/organization/adapters ./internal/modules/orgaudit/adapters
GOCACHE=/tmp/enterprise-go-build go -C backend test ./cmd/api \
  -run '^TestFullProductionRouterEnterpriseContractAndDefaultOff$' -count=1
TZ=America/Cuiaba python3 docs/enterprise/tools/check_plan.py --write
python3 -m unittest discover -s docs/enterprise/tools -p 'test_*.py'
```

## Remaining obligations

ORG-007 can now consume the actual employee register. IDN invitation/acceptance,
managed activation/device providers and employee lifecycle commands remain their
own planned tasks. The register's HTTP surface is PANEL; Android/native consumer
acceptance is not inferred. F1/M1, phase publication/final current-head/base CI,
actual provider/ordinary production grants, measured quota/cleanup activation,
vault/staging/native/Kotlin coverage and real-data/legal gates remain open.
PLAT-012 still depends on GOV-014. No merge, release or deployment is certified.

## Final PR202 review note (2026-11-12)

Employee replay digests include contact fields as plain SHA-256 in the
append-only command and audit rows; like the CPF digest in ORG-004, keyed
fingerprints need a dedicated versioned key (privacy/retention work).
Employee reads take shared locks. [Review record](f1-pr202-review.md#final-review-head-45230e28).
