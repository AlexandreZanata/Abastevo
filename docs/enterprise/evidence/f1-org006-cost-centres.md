# F1 EMP-ORG-006 cost centres and permanent code meaning

Status: LOCAL_DONE
Validation: PASS

2026-11-12. LOCAL_DONE / INTEGRATION_PENDING above ORG-005 dfb6fd78 in the
owned enterprise execution worktree, targeting
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202).
F1/M1, completed-phase publication and final current-head/base CI remain open.
No runtime grant, provider, flag, real-data approval or release is inferred.

## Contract and recommended decision

B-BR-EMP-61/62 and BUC-EMP-31 preceded behavior. EDR-040 resolves the two
applicable requirements together: UC-ORG-03 permits code corrections, while
BR-ORG-007 and historical exports require a code to retain its meaning forever,
including archived centres. A correction selects a new current code, with every
old normalized code permanently reserved to the same stable scoped centre ID.
Historical lookup returns that centre's current metadata; it never remaps an
old code to another centre. Names are descriptive and need not be unique.
Identical codes in different organizations remain independent.

This uses current OWNER/ADMIN org.structure.manage and the existing independent
org.structure admission class (identity v1 synthetic 200/hour on each axis).
No new role, permission cell, capability, paid-account rule, dependency, service
or production flag is added. Actual capacity measurement and cleanup activation
remain due. StructurePolicy, category usage and its typed error are shared by
the two real unit/cost-centre consumers, with compatibility aliases for the
completed unit API. Existing unit/legal behavior and authority lock order are
preserved; this is not a generic registry or speculative service.

## Domain and actual HTTP composition

The pure aggregate owns private name/code, immutable organization/ID, current
ACTIVE/ARCHIVED state and revision. Changes, archival and restoration preserve
identity. Source counts are nonnegative int64 category totals; archiving a used
centre refuses without source records. Domain/DTO fmt (including invalid verbs),
JSON and slog formatting redact names/codes. Snapshot rehydration rejects corrupt
storage without normalizing it into a valid value. Code reservation uniqueness
belongs to the transactional repository and SQL, not a guessed domain lookup.

The actual enterprise factory mounts seven POST operations under
/v1/orgs/{org_id}/cost-centres: create/list, code/read, and centre read/change/
archive/restore. All use the dedicated PANEL cookie, exact Origin/Host, CSRF,
current membership and org.structure.manage. Mutations require fresh proof,
current organizations pilot flag and the existing structure quota. Normal
source-pinned synthetic terms admission remains default-off. Reads preserve
already authorized capability-off/stale-authentication behavior while still
requiring a live family/cookie and current permission.

Strict DTOs refuse actor IDs, state, usage counts, generated code keys, duplicate
keys, aliases, nulls and unrelated unit fields. Lookup bodies contain only code;
body credentials cannot select a different account. UUID keyset lists use sealed
cursors bound to account/org/member/route/state/limit. Foreign/unknown IDs return
the same bounded 404. Code resolution is always scoped; another organization's
code has no effect on the local response. All responses are no-store without ETag.
Archived views retain ID and append "(arquivada)" to the current display name.

Code conflicts return VALIDATION_FAILED with only code/DUPLICATE. Actual usage
returns STRUCTURE_IN_USE and the existing four exact decimal-string category
counts. Unknown/malformed or uncounted sources return unavailable. No name,
code, alias owner identity or source table appears in errors, logs or audit.
Audit uses the plan's org.cost_centre.changed action with exact required reasons
created/metadata/archived/restored; human content is not event metadata.

## Schema and atomic transaction

Additive migration 000175 creates forced-RLS centres, immutable code reservations
and immutable command acknowledgements. Normalized code keys are PostgreSQL
lower(code) generated columns. The registry key is organization+code; current
and old codes cannot be reassigned. Deferred composite FKs require the current
code reservation to name the exact current centre, while every reservation
retains its scoped parent. The AFTER capture and guarded insert reserve the
new code in the same transaction; the old row is never changed or released.
Existing own aliases may become current again. All three tables deny public
access; centres/aliases/receipts cannot be deleted or rewritten as history.

The repository uses an ordinary scoped transaction and the proven company
account/proof -> capability -> root -> current membership/role lock order.
Proof is rechecked after lock waits. Centre, code reservation, audit and receipt
commit together. Matching operation/hash/actor returns its original outcome,
even after a later correction. Altered intent conflicts. Stale changes mutate
nothing. Failed or uncertain commits return zero value and no replay success.

A targeted RED reproduced a deferred current-code FK failure classified as
not-found after the work callback had succeeded. The repository now records
whether the callback was ready to commit: a later commit failure is unavailable
rather than a business lookup refusal. The test disables capture only inside a
disposable schema, reaches the actual deferred FK, then verifies no centre,
alias or acknowledgement survives. The corrected regression and real HTTP flow
pass. No FK or rejection assertion was relaxed.

Usage/admission is the root-serialized structure contract. Current known FKs
are immutable reservations/current-code pointers and command receipts, not
active work. Unknown consumers of either centres or aliases block archive until
their owning migration adds actual scoped counts and admission. A disposable
source tests active assets/employees, open budgets and operations, historical
references and assignment/archive races. It does not implement those future
business modules. Historical records keep their identity/code snapshots; no
allocation/export remapping or budgeting operation is introduced.

## Migration and recovery evidence

A populated prior 000174 database contains initialized organization/unit,
active member/OWNER, synthetic declared legal entity/establishment, audit/setup
receipts and a public station. An ordinary NOSUPERUSER/NOBYPASSRLS/NOLOGIN schema
owner with only creation/ledger/reference grants applies 000175. Injected SQLSTATE
22012 rolls back all three tables/functions/ledger without changing prior rows.
Retry applies once, then no-ops. Forced RLS and ordinary ownership of all three
new tables are asserted; actual post-upgrade create/correction/historical lookup
passes using a separate ordinary runtime login. The prior migration chain's
schema-version check passes. This proves schema compatibility, not execution of
an old container image or production credentials.

Recovery retains additive schema/history, disables the entry and uses the
compatible previous binary. No applied migration edit, destructive down,
production DDL/grant or live data was used. The existing audit TRUNCATE CASCADE
fixture explicitly grants and disables/re-enables only the new referencing
receipt trigger for its original exact 55000 audit assertion.

## Validation

On local disposable pinned PostgreSQL 18/PostGIS 3.6, ordinary application and
migration roles provide behavior; privileged pools prepare fixtures only.
Go 1.27.4, sqlc 1.31.1, staticcheck v0.8.1 (Go 1.27.1 toolchain) and vacuum
0.30.6 were used. Passed meaningful cases include:

- Canonical code/name, private representations, invalid input/storage, lifecycle,
  stale/overflow revisions and unforgeable request-bound application decisions.
- Permanent old/current/archived code reservations, own-alias correction,
  independent foreign codes, non-unique names and keyset continuation.
- Twelve distinct same-code creations: one creation/eleven bounded duplicates;
  twelve equal operations: one creation/eleven original replays.
- Two simultaneous corrections competing for one new code: one succeeds, one
  refuses, both original codes still resolve their own original centres.
- Active usage in all four categories; historical references allow archive;
  twelve archive/admission races permit exactly one action; archived centres
  refuse new work; unknown centre or alias consumers fail closed.
- Immutable reservation update/delete and invented reservation refusals.
- Failed deferred receipt/cyclic ownership commits leave no mutation, new alias,
  acknowledgement or successful replay; no partial correction remains.
- Current cookie/family revocation, stale authentication, flag disable/read
  preservation and real valid-cookie DRIVER denial through company HTTP.
- Real HTTP create/change/archive/restore, historical code reads, duplicate field
  errors, stale/altered retry, foreign IDs, cursor filter/scope changes, missing
  CSRF, foreign Origin, bearer/admin/signed-plane confusion and late failure.
- Populated ordinary-owner upgrade/failure/recovery plus all affected unit,
  legal, onboarding, membership, access, audit, company-session and quota flows.
- Full production-router contract/default-off checks and explicit metadata/
  selector/cookie drift rejection against the canonical OpenAPI.

The final composed organization coverage profile is domain **94.14%**,
application **77.71%**, adapters **77.61%** and HTTP **79.19%**. -coverpkg counts
HTTP/application paths executed by external real-PostGIS tests; other packages'
zero coverage entries are outside that scope, not failed tests. The added
correction-only race test passes after this run without production changes.
No whole-project or Kotlin coverage claim is made.

Reproduction requires a disposable ANPFUEL_TEST_DATABASE_URL:

```sh
cd backend
GOCACHE=/tmp/enterprise-go-build go test -race -tags=integration \
  -coverpkg=./internal/modules/organization/... \
  ./internal/modules/organization/... ./internal/modules/orgaudit/... \
  ./internal/modules/orgaccess/... ./internal/modules/companysession/... \
  ./internal/modules/membership/... ./internal/platform/orgapi \
  ./internal/platform/apicontract ./internal/platform/orgquota \
  ./internal/platform/enterprisesuite -count=1 \
  -coverprofile=/tmp/enterprise-org006-final-coverage.out
GOCACHE=/tmp/enterprise-go-build go test -race ./cmd/api -count=1
GOCACHE=/tmp/enterprise-go-build go test -race -tags=integration \
  ./internal/modules/organization/adapters \
  -run TestCostCentreConcurrentCorrectionsCannotReassignOneHistoricalCode -count=1
sqlc vet
sqlc generate
sqlc diff
```

Affected vet/staticcheck and API/worker/ops builds pass, with strict gofmt,
git diff --check, added local links and staged secret review. Plan validation
reports 348 tasks, 545 BRs, 167 UCs, 145 screens and zero structural problems;
its three tests pass. Canonical OpenAPI has zero errors, the same 785 existing
warnings and 287 informational findings (31 additional duplicate descriptions).
Android source is unchanged, so no emulator/native acceptance is claimed.
Actual providers/runtime grants, vault/staging, GOV-014/PLAT-012, real-data/legal,
Kotlin coverage, F1/M1 and final current-head/base CI remain independent.
