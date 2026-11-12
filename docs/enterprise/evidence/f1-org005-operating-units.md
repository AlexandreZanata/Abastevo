# F1 EMP-ORG-005 operating units

Status: LOCAL_DONE
Validation: PASS

2026-11-12. LOCAL_DONE / INTEGRATION_PENDING above ORG-004 0d91c7f7 in the
owned enterprise execution worktree, targeting
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202).
F1/M1 and completed-phase publication/final current-head/base CI remain open.
The earlier partial construction checkpoint is completed by the HTTP and
validation evidence below; no live/runtime/legal acceptance is inferred.

B-BR-EMP-59/60 and BUC-EMP-30 preceded behavior. EDR-039 records recommended
archived metadata correction before restore, captured zone defaults, retained
establishment history and source-owned current-usage guards. The existing
OWNER/ADMIN org.structure.manage row is unchanged. No provider, new dependency,
service, speculative usage ledger, production grant or flag is introduced.

The existing onboarding Unit/NewDefaultUnit moves into its own domain file and
extends to create/change/archive/restore. Snapshot identity/state/revision and
profile remain private, immutable values. Archived metadata may change to
resolve a restoration collision; only active names/codes are unique. Time zones
use the existing IANA kernel. An omitted-zone command keeps its original intent
in the hash; its first transaction resolves the current organization zone.
Tests capture reporting records before and after a unit zone change and prove
the original record/day is unchanged. Domain/race coverage is 93.3%, and the
existing orgtime domain suite remains 100% in the initial unit profile.

Additive migration 000174 preserves the existing unit table and adds optional
scoped establishment/municipality fields plus forced-RLS immutable receipts.
Its root-serialized SQL guard enforces active uniqueness, last-unit protection,
new declared-parent establishment links and retained existing links. Error
messages are static and do not expose failing unit names. Units remain
undeletable. A real unit FK makes legal withdrawal retain REVOKED history;
a newly selected revoked/foreign link is refused, while unrelated corrections
of an existing retained link remain possible.

The current source-owned usage function recognizes only the actual immutable
onboarding/command receipt FKs, which do not represent active work. Any unknown
referencing source refuses archive with P7704, including an empty future table.
Owning modules must extend the exact SQL function with actual scoped counts
and call the root-locking active-unit admission function in their transactions.
A disposable test source exercises this contract for active assets/employees,
open budgets and in-progress operations. Those tests do not claim that their
future business modules are implemented. Historical references are retained and
do not count as active work. Twelve archive/assignment races permit exactly one
action; racing archives retain one active unit.

The concrete UnitStore uses ordinary scoped transactions, current company proof,
org.structure.manage, memberships/roles and the independent organizations flag.
Its shared authority helper is extracted from the proven legal store, keeps the
same account/proof -> capability -> root -> current role lock order and accepts
only the concrete source-owned legal/unit policies. Legal/HTTP regressions pass
following extraction. Content, acknowledgement and audit commit together. Exact
retry returns the original result; changed intent and stale revisions refuse.
The initial restore test reproduced an audit-catalog refusal because metadata/
restored reason codes were missing. They are now exact registrations, with
unknown/absent reasons still denied; stale restore returns the expected conflict.
Twelve equal creations produce one identity and eleven replays. Deferred receipt
failure returns zero result/replay and leaves no unit. Source counts are exact
nonnegative int64 values with four mandatory keys; unknown/incomplete/null/
fractional/overflow counts fail closed. A blocked archive leaves no audit
reservation. Foreign references and keyset pages are tested.

The populated migration rehearsal starts at 000173 with initialized root/unit,
active OWNER/member, a real synthetic declared company/establishment and its
receipts/audit, plus a public station. An ordinary NOSUPERUSER/NOBYPASSRLS owner
applies the migration. Injected SQLSTATE 22012 restores the exact old unit guard,
old columns, data and ledger. Retry applies once, then no-ops. Old row snapshots
exclude only the two additive fields; prior schema compatibility and an actual
post-upgrade unit linked to the declared establishment pass under ordinary
ownership. No old container image or production credentials were exercised.

Passed on disposable local PostgreSQL 18/PostGIS 3.6, with an ordinary application
role and privileged fixture setup only:

- Organization/orgtime domain/application and unit decoding/audit unit tests.
- Fresh unit schema, active duplicate/restore, last-unit races and immutable
  deletion trigger, with explicit test-only DELETE grant to reach that trigger.
- Actual source counts/unknown consumer and twelve assignment/archive races.
- Unit-store create/read/list/change/archive/restore, replay/CAS and failure.
- Scoped establishment link, referenced legal withdrawal and retained history.
- Ordinary-owner populated upgrade/failure/recovery and legal/current-proof/
  real company HTTP/onboarding regressions after the authority extraction.

The initial wrong-directory command wrote nothing and ran no selected tests;
it is not counted as RED or validation. The corrected domain RED reproduced
missing behavior; the completed domain suite is green. An incorrect hard-coded
UTC test expectation was corrected to compare the actual organization default.
These changes did not relax a production rule or weaken a rejection assertion.

## Completed HTTP composition and validation

The actual production enterprise factory mounts six company-cookie POSTs under
/v1/orgs/{org_id}/units: create/list and unit read/change/archive/restore. Normal
PANEL cookie, exact Origin/Host, CSRF and current org.structure.manage apply.
Every mutation requires fresh proof, current organizations pilot capability
and the independently bounded org.structure class (identity v1 synthetic
200/hour on each axis). No flag/grant is installed or measured capacity claimed.
The full production-router gate matches every guarded path, selector and
OpenAPI authority/security declaration. An explicit default-off regression
checks both unit creation and read are unmounted. All responses are no-store
without ETags. No Android behavior changes require emulator acceptance here.

Strict business DTOs reject authority, state, counts, aliases, nulls and unknown
fields. fmt/JSON and nested DTO values redact private names. Explicit response
mappings release only authorized unit fields and preserve stable historical
identity; archived display names append "(arquivada)". Lists use UUID keysets
and opaque cursors bound to account, organization, membership, route, state and
limit. The same operation returns its original acknowledgement after later unit
edits, without reverting current content.

LAST_UNIT and STRUCTURE_IN_USE are registered 409 refusals. Current use exposes
only active_assets, active_employees, open_budgets and open_operations, each
with a canonical exact decimal-string count. The optional shared ApiError
count field is omitted by existing details. Zero/negative/unknown source
refusals without validated counts fail closed; no records, names or table
identities enter the error. Exact int64 maximum, absent/incomplete/null/
fractional/overflow counts and unknown fields are tested. Unit duplicates
expose only name/code and DUPLICATE. Response revision schemas admit the final
int64 revision; command revisions must still be incrementable. Matching legal
response bounds are corrected too; no domain limit or mutable-history rule is
weakened.

Actual HTTP/PostGIS tests execute account-key proof, company login, synthetic
organization activation and declared establishment linkage. They cover create,
metadata change, list/read, archive/restore, exact retry after later mutation,
changed intent/stale revision, name/code duplicate details, invalid zone,
foreign references and changed scope/filter/limit cursors. A live DRIVER
membership/SELF role with a normally issued cookie receives FORBIDDEN for
unit management; the OWNER cookie succeeds. Cookie/bearer/admin/signed-plane
confusion, missing CSRF, foreign Origin, stale authentication and revoked
cookie all refuse. Pilot disable blocks writes and preserves authorized
existing reads. Active source usage returns only category counts; historical
references no longer block archive. A deferred receipt commit failure returns
503 without an acknowledgement or retained provisional unit.

Twelve distinct competing name creations produce one creation/eleven bounded
name refusals; the analogous code race produces one creation/eleven code
refusals. Neither leaves extra audit/receipt rows. Competing expected-revision
changes produce one revision and one conflict. These race-only follow-up tests
pass after the broader final suite; production code is unchanged by them.

The audit TRUNCATE regression fixture grants/disables/re-enables the new
referencing receipt trigger so it retains its original exact audit assertion.
The broader organization/audit run also found a fixture-permission regression:
TRUNCATE org_units CASCADE reaches the new receipt table. A test-only TRUNCATE
grant restores the original expected 55000 without changing that assertion.
Both regressions pass, including the complete final organization/audit suites.
A sandbox-denied loopback socket in the pre-existing HTTP helper suite was
rerun with loopback authorization and passes; it is not a production defect.

The completed final PostGIS/race run includes all organization, audit, access,
membership and company-session packages plus orgapi/httpapi, apicontract,
orgquota and the actual enterprise factory. API tests, including the full
production-router/default-off test, also pass. Cross-package instrumentation
covers production HTTP/application paths executed by the external PostGIS
suite. The organization profile records domain **95.01%**, application
**79.47%**, adapters **77.37%** and HTTP **79.35%**. Other packages' zero lines
in that command mean outside its -coverpkg scope, not failed tests; no Kotlin
or whole-project coverage claim is made.

Reproduction requires a disposable ANPFUEL_TEST_DATABASE_URL:

```sh
cd backend
GOCACHE=/tmp/enterprise-go-build go test -race -tags=integration \
  -coverpkg=./internal/modules/organization/... \
  ./internal/modules/organization/... ./internal/modules/orgaudit/... \
  ./internal/modules/orgaccess/... ./internal/modules/companysession/... \
  ./internal/modules/membership/... ./internal/platform/orgapi \
  ./internal/platform/httpapi ./internal/platform/apicontract \
  ./internal/platform/orgquota ./internal/platform/enterprisesuite \
  -count=1 -coverprofile=/tmp/enterprise-org005-final-coverage.out
GOCACHE=/tmp/enterprise-go-build go test -race ./cmd/api -count=1
GOCACHE=/tmp/enterprise-go-build go test -race -tags=integration \
  ./internal/modules/organization/adapters \
  -run TestOperatingUnitStoreSerializesDistinctDuplicateAndRevisionRaces -count=1
sqlc vet
sqlc generate
sqlc diff
```

Affected go vet/staticcheck (v0.8.1, Go 1.27.1 toolchain), API/worker/ops builds,
strict gofmt, git diff --check, added local links and staged secret review pass.
Go tests use 1.27.4 and disposable pinned PostgreSQL 18/PostGIS 3.6 on loopback
32771. Privileged pools prepare fixtures only; ordinary application/migrator
roles provide the tested behavior. Plan validation reports 348 tasks, 545 BRs,
167 UCs, 145 screens and zero structural problems; its three tests pass.
Canonical OpenAPI has zero errors, the same 785 pre-existing warnings and
256 informational findings (27 additional duplicate descriptions). None of
those checks certifies actual providers, runtime grants, legal acceptance or
external deployment. IDN/device, vault/staging, GOV-014/PLAT-012, real-data/legal,
Kotlin coverage, F1/M1 and final current-head/base CI remain independent.

## Final PR202 review corrections (2026-11-12)

Unit names and codes refuse Unicode format characters and non-NFC text, so
a zero-width or decomposed variant can no longer duplicate an active name or
code. Unit reads take shared locks (see ORG-004). A change that omits
`time_zone` still resets the unit to the organization zone, as the contract
states; clients must round-trip it. [Review record](f1-pr202-review.md#final-review-head-45230e28).
