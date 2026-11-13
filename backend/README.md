# Backend workspace

Status: P01-T01…T12 foundations exist: API/worker/migrator roots, typed config,
redacted telemetry, HTTP/health lifecycle, pgx pool, migration runner, sqlc,
container definitions and base OpenAPI vectors. Business modules and
production infrastructure remain ahead. See [current state](../docs/planning/PROGRESS.md)
and [prior task evidence](../docs/planning/history/P01_FOUNDATION_EVIDENCE.md);
this documentation revision does not rerun those suites or verify remote CI.

Tooling pins: Go toolchain `go1.27.4` (`go.mod`); `sqlc v1.31.1`
(`sqlc vet && sqlc generate` from `backend/`; generated packages committed).
`chi v5.3.2`, `pgx v5.11.0` (both MIT, permissive transitives only).
`staticcheck v0.8.1` (MIT), `govulncheck v1.8.0`, `vacuum v0.30.6` (MIT).
Static binaries must be built with Go ≥1.27 or typechecking fails; install
with `GOTOOLCHAIN=go1.27.4 go install <tool>@<version>`. The pinned
staticcheck predates the go1.27.2 export format, so the fast gate runs
`staticcheck` itself under `GOTOOLCHAIN=go1.27.1` (see
`scripts/check-backend-fast.sh`).

## Verification entry points (P01-T13)

Shared local/remote composition lives in `Makefile` and `scripts/`:

```sh
make quick-verify    # bounded gate: manifest + selection + fast checks + govulncheck
make verify-release  # quick + full-matrix report (foundation subset, NOT CERTIFIED)
make test-gate       # focused harness for selection/failure behavior
```

Manifest `scripts/check-manifest.txt` lists required quick packages and
checks; missing packages or zero matched tests fail. Docs-only changes run
docs checks plus contract reference integrity; code changes run the full
quick set. Unclassified paths, missing tools and secrets in working tree or
committed PR diff fail. `go test -run '^$'` is compile-only and never counts
as behavioral evidence. Measured quick budget is 300s warm cache.

Required CI is the always-on **Quick verification** job (`.github/workflows/quick.yml`):
it runs `make quick-verify` on every PR including drafts plus main pushes, with
no path filter, so docs-only changes still report a meaningful required result.
`backend.yml` fast/integration and `ci.yml` test keep running path-triggered as
non-required signal; their task-level and phase-exit evidence stays mandatory
per [CI_PLAN](../docs/planning/CI_PLAN.md).

Existing gates below remain binding until G01-FLOW T17 migrates CI/protection.

Fast gate, no database (from the repository root):

```sh
bash scripts/check-backend-fast.sh   # gofmt, build, vet, unit tests,
                                     # staticcheck, sqlc vet/diff, vacuum lint,
                                     # tracked + changed-file secret scans
cd backend && govulncheck ./...
```

Integration gate, real disposable PostGIS (from the repository root):

```sh
cargo +1.99.0 build --locked --manifest-path tools/station-prep/Cargo.toml --bin prepare_registry --bin prepare_quality --bin prepare_sanctions --bin prepare_fine_history --bin prepare_enforcement --target-dir /tmp/abastevo-integration-rust
export ANPFUEL_TEST_PREPARER_PATH=/tmp/abastevo-integration-rust/debug/prepare_registry
chmod 755 /tmp/abastevo-integration-rust/debug/prepare_quality
export ANPFUEL_TEST_QUALITY_PREPARER_PATH=/tmp/abastevo-integration-rust/debug/prepare_quality
chmod 755 /tmp/abastevo-integration-rust/debug/prepare_sanctions
export ANPFUEL_TEST_SANCTIONS_PREPARER_PATH=/tmp/abastevo-integration-rust/debug/prepare_sanctions
chmod 755 /tmp/abastevo-integration-rust/debug/prepare_fine_history
export ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH=/tmp/abastevo-integration-rust/debug/prepare_fine_history
chmod 755 /tmp/abastevo-integration-rust/debug/prepare_enforcement
export ANPFUEL_TEST_ENFORCEMENT_PREPARER_PATH=/tmp/abastevo-integration-rust/debug/prepare_enforcement
docker compose -f infra/compose.dev.yml up -d db
(
  cd backend
  export ANPFUEL_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
  export ANPFUEL_TEST_DATABASE_URL="$ANPFUEL_DATABASE_URL"
  go run ./cmd/migrate && go test -race -count=1 -tags=integration ./...
)
```

Integration fails when PostGIS is unreachable; it never skips. CI
(`.github/workflows/backend.yml`) runs both gates path-aware on backend,
contract, infra and gate-script changes; the Android workflow keeps running
`./gradlew test` on Android paths and skips pure docs/contract/backend edits.

Continue with [P01-T13](../ROADMAP.md#p01-t13); read [architecture](../docs/backend/TARGET_ARCHITECTURE.md) and [test strategy](../docs/backend/TEST_STRATEGY.md) as the selected task requires. P02 starts after G01 and G01-FLOW; subsequent phases add owned business modules and explicit SQL incrementally.

The Android source remains in its original root modules. Do not move it under a new mobile directory.

## Complete local backend validation

From the repository root, `bash scripts/tests/test-local-backend.sh` creates a
unique disposable Compose project with tmpfs PostGIS and a private S3 emulator.
It runs all unit/integration suites under the race detector, actual API + worker
HTTP/media flows, static/vulnerability checks, infrastructure/deploy harnesses,
encrypted backup, restored deletion-ledger replay, bounded load and DB outage.
It clears inherited runtime DSN/storage settings and never selects the shared
`infra/compose.dev.yml` database. Only its own project is removed on exit.
Reports remain in a printed `/tmp` directory; set `ANPFUEL_LOCAL_REPORT_DIR` to
choose a local output path. The container credentials are synthetic test values.
These tmpfs services lose all data on removal; they are not production storage.

The restore harness also needs the disposable `ANPFUEL_TEST_STORAGE_{ENDPOINT,
BUCKET,REGION,ACCESS_KEY_ID,SECRET_ACCESS_KEY}` settings supplied by this runner.
It first proves that missing storage refuses an evidence-erasure acknowledgement,
then replays the restored deletion ledger with the private S3 fixture configured.
The enforcement integration cases require the trusted `prepare_enforcement` CLI,
which the full runner builds and exports alongside the other native preparers.

For an already running synthetic API on a local/private literal IP, run:

```sh
ANPFUEL_LOCAL_API_URL=http://172.19.2.11:18093 bash scripts/tests/test-local-edge.sh
```

This starts the pinned production Caddy image with an ephemeral internal CA,
validates TLS 1.2/1.3, health and route isolation, and checks synthetic location/
signature/cookie markers are absent from logs. It does not modify system trust
or obtain public certificates. The test edge and its keys are removed on exit.
Local test success is distinct from G09 certification and public deployment.

### Signed app writes against a local API

The debug app's local proof build (`-PmapApiLocalProof=true`, for example
`./gradlew :app:assembleMapProbe -PmapApiLocalProof=true`, which installs as a
separate `com.anpfuel.app.mapprobe` app) talks to `http://127.0.0.1:18080` through
`adb reverse tcp:18080 tcp:18080` and signs `@authority` with that port. Only in
development, `ANPFUEL_CANONICAL_HOST` therefore accepts a loopback `host:port`:

```sh
ANPFUEL_ENV=development ANPFUEL_HTTP_ADDR=127.0.0.1:18080 \
ANPFUEL_CANONICAL_HOST=127.0.0.1:18080 ANPFUEL_FORUM_ENABLED=true go run ./cmd/api
```

Staging and production keep refusing anything but a bare public hostname.

### Private Empresas activation (EMP-PLAT-005)

`ANPFUEL_ENTERPRISE_ENABLED` defaults to false. Future organization-route
composition passes `Config.EnterpriseEnabled` into `orgapi.Gateway.Enabled`
and `orgcapability.Service.Enabled`. Changing this startup flag requires the
normal rollout; it does not enable an organization capability or grant a role.
There are no enterprise business routes or concrete AUTH providers in this
foundation checkpoint.

Per-organization switches are registered by source and persisted under forced
RLS. Missing rows deny new writes. An authorized operator changes one switch
through the audited service with an operation ID and expected revision;
disabled capabilities preserve authorized reads/exports and all stored data.
See the [transaction and composition contract](../docs/enterprise/09-architecture.md#101-implemented-capability-foundation).
Use an ordinary non-owner, non-BYPASSRLS runtime login; backup/restore retains
the separately reviewed authority needed for forced-RLS tables.

### Final PR202 review corrections (enterprise plane)

Migrations 000179–000183 are additive corrections: team periods close only on
access reductions, settings re-prove availability only for changed
designations, the all-zero CNPJ is refused, accounts bind to memberships only
while INVITED, and new role periods cannot start in the past or be already
ended. Legal, structure and employee reads take shared account/root locks;
writes and settings reads keep exclusive locks. `WithOrgTx` pins READ
COMMITTED and `ValidateOrgTx` refuses other levels; account-module
transactions rely on the server default, which must stay READ COMMITTED.
Organization creation authenticates before body validation and passes an
account-axis write admission.

Before enabling `ANPFUEL_ENTERPRISE_ENABLED` for real accounts: ownership
transfer or BR-ORG-028 closure must exist (the creator is otherwise the
permanent sole OWNER and cannot delete the personal account), and the
ordinary runtime role needs the full grant inventory, including SELECT on
`org_settings_heads` for the role that deletes accounts. See the
[review record](../docs/enterprise/evidence/f1-pr202-review.md#final-review-head-45230e28).

### Organization write admission (EMP-PLAT-014)

Every orgapi.Rule.Write route must provide Gateway.WriteAdmission with a
matching source policy; registration fails without it. Compose orgquota.New
with an ordinary non-owner/non-BYPASSRLS pool, the shared privacy key and
reviewed Definition values. A class maps source permissions to explicit
organization/account/device budgets and a recorded measured-limit rationale.
Definitions/key are copied. No production class/default quota is selected
while business routes/providers remain unmounted. Owning feature tasks must
measure and record their limits before activation; synthetic test caps are
not production configuration.

After current membership/permission/capability and freshness succeed, the
gateway calls admission before module work. orgquota consumes three existing
identity Limiter.CheckTx counters in one WithOrgTx transaction, using the
existing write operation and identity_rate_windows table. Common organization
first, then account, then device gives one lock order. Quota denial or failed
transaction rolls back all axes; an admitted request still consumes budget
if later record rules fail. This is request admission, not billing/record
quota or a replacement for business transactions. Cancellation after admission
does not run module work or promise to refund an already committed admission.

Subjects are HMAC-framed class/axis/org, adding account and verified device
key as needed. All axes are organization-scoped. Android key rotation keeps
organization/account usage; web has a stable account/web bucket because the
cookie is not a physical-device proof. Cookie rotation cannot mint that
bucket anew. No raw organization/account/device/IP or key is persisted or
exposed. The existing limiter uses fixed windows; reads and pre-organization
company sessions keep their existing policies. Windows use whole seconds,
up to the existing 24-hour quota-window bound. The existing Cleanup API removes
expired windows; its scheduling/cadence must be wired and verified before
enterprise activation (no active cleanup caller was found in this source).

Exhaustion yields private RATE_LIMITED/429 with positive ceil-second
Retry-After covering every exhausted axis (the denied transaction still reads
the remaining axes before it rolls back); outages/unsafe roles/missing policies yield unavailable, with
no raw SQL/provider error. OpenAPI documents the optional retry header.
Use SELECT/INSERT/UPDATE grants on identity_rate_windows. Replicas must use
the same class policies, synchronized clocks and stable privacy key across
live windows. Changing class/window/key requires reviewed rollout; do not
silently reset active counters. This guard does not replace read-transfer
concurrency limits or approve any feature activation.

Run race/unit and real-PostGIS integration tests for orgquota/orgapi;
see [architecture](../docs/enterprise/09-architecture.md#33-implemented-scoped-rate-admission)
and [evidence](../docs/enterprise/evidence/f1-plat014-scoped-rate-admission.md).

### Private organization telemetry (EMP-PLAT-013)

The API composes orgapi.Observe on the existing httpserver.NewRouter and
metrics.Registry. It adds no route, collector or listener. Scrape the
existing private ANPFUEL_METRICS_ADDR listener; public /metrics stays absent.
No enterprise capability is enabled by observability.

org_requests_total and org_request_seconds_total carry only route, result
and code. Route is the matched source template or unmatched. Result is
success/refused/unavailable. Codes are OK, the existing allowlisted
organization error codes, or REFUSED for an untyped client-error response.
No request body is parsed for telemetry. The existing 512-series cap and
metrics_dropped_total remain authoritative. Cumulative seconds support
mean duration; this task adds no latency histogram or percentile claim.

The transport encloses private handlers/recovery in a private logging context
and replaces inbound request IDs with server correlation. Generic Info/Warn/
Error calls in that context drop free messages/attributes and inherited
client/job/bound-route fields, retaining only fixed event codes and server
trace. Use the supplied request context. Transport-owned access records
retain bounded method, source route, actual status and elapsed time. Never
pass raw URLs to Logger.Request. Session/command/actor/principal/cursor/config
DTOs redact default formatting and slog, including embedded session DTOs.
Module-specific values must maintain the same convention.

Install orgapi.Observe once inside the router's recovery boundary, as cmd/api
does. Panic observations are recorded safely and repanicked for existing
private recovery. Writer wrappers preserve initial status, implicit writes,
flush and ResponseController unwrapping. Audit authorization/storage stays
separate. See [architecture](../docs/enterprise/09-architecture.md#111-implemented-private-telemetry)
and [evidence](../docs/enterprise/evidence/f1-plat013-private-telemetry.md).

### Transactional organization events (EMP-PLAT-010)

orgevent joins required follow-up to the existing organization audit runner
and PostgreSQL job queue. Source modules register bounded event/target/consumer
codes in an immutable catalog. After checking current write authority, call
the default-off Service.Record with the original audit operation/target,
positive record revision and required consumers. Every source write uses
the supplied transaction; audit, immutable intent and consumer jobs commit
together. Replay binds the original command digest and canonical fan-out.
A work callback's registered orgapi code (CONFLICT, VALIDATION_FAILED,
PERIOD_LOCKED, ...) is returned bare, without its wrapping text, after the
rollback; any other error is an opaque outage. Producers map their module
refusals, such as capability guards, to registered codes inside the work.

Construct NewHandler with a database-only effect for every catalog consumer
and register it under org-event in the existing jobs.Dispatcher. It verifies
the stored intent, exact claimed row, token, worker, payload and live lease
inside WithOrgTx. Effect and scoped append-only receipt commit together;
queue acknowledgement stays with Dispatcher. Lost acknowledgement and
concurrent redelivery cannot repeat the committed database effect.
The five-claim budget also rejects expired-lease crash loops before another
effect; normal failures retain the existing one-minute retry and dead replay
requires an explicit reason. Keep external calls outside these transactions.

Apply additive migration 000168 first. Use an ordinary non-owner/non-BYPASSRLS
login with explicit queue DML and intent/receipt SELECT/INSERT grants.
No concrete business producer, effect, API route, worker registration or live
activation is supplied here; each owning module composes these ports.
Operator UI/audit, external delivery and retention retain their own tasks.
Do not delete receipts while their effects can be replayed.
Run `go test -race -tags=integration ./internal/modules/orgevent/...`
with an explicit disposable ANPFUEL_TEST_DATABASE_URL. See
[architecture](../docs/enterprise/09-architecture.md#71-implemented-transactional-event-foundation)
and [evidence](../docs/enterprise/evidence/f1-plat010-transactional-events.md).

### Organization aggregate (EMP-ORG-001)

organization/domain supplies the scoped immutable root and its lifecycle.
NewPending needs an explicit kernel.OrgScope and starts at revision one.
Snapshot requires that same scope. ConfigurePending and Transition check
the expected revision and return a new value without mutating the original.
Kind tags are descriptive only; no CNPJ/unit/member/permission is derived.
The activation checklist reports bounded missing-item codes. Restore and
closure require explicit confirmed conditions; CLOSED has no outgoing edge.

Rehydrate takes trusted storage state, never client input. Conditions are
facts resolved by the server, not permission booleans supplied by a client.
Owning commands must verify current actors/freshness, allocate a fresh ID
and prevent reusing any persisted ID, persist root/unit/owner atomically,
apply CAS/audit and perform the lifecycle effects. This task adds none of
those adapters, routes, jobs or actual closure. Zones use the operation-time
profile in kernel.LoadTimeZone, so host-only and abbreviated names fail. The
display name is bounded free text (120 code points, no control or
bidirectional override characters). Formatting, slog and JSON, including
nested values, print only redactions. No cross-client timezone
certification is implied.

```sh
go test -race ./internal/modules/organization/domain -count=1
```

See [111 domain vectors](../contracts/testdata/enterprise/README.md#organization-lifecycle-v1)
and [local acceptance](../docs/enterprise/evidence/f1-org001-organization-domain.md).

### Organization persistence (EMP-ORG-002)

`organization/application` declares scoped repository and explicit-Tx ports;
`organization/adapters` stores canonical domain snapshots with sqlc/pgx.
Migration 000169 adds permanent private roots, forced RLS and deferred root
FKs for audit/capability/events. Existing private IDs retain identity-only
anchors; missing metadata returns `ErrUninitialized` and cannot be adopted
by normal create/save. There is no name lookup or global name uniqueness.

Create requires pending setup/revision one; save uses CAS and exact replay.
Explicit-Tx methods validate the installed scope and ordinary role for the
owning audit/onboarding transaction. Root identity cannot be removed or
reopened after closure. Storage maps fields explicitly, retaining domain
redaction. No runtime providers/grants, onboarding route, owner/unit creation
or activation is installed. ORG-003 also requires IDN-003 and AUTH-002.

```sh
go test -race ./internal/modules/organization/... ./internal/platform/database -count=1
# Explicit disposable ANPFUEL_TEST_DATABASE_URL required:
go test -race -tags=integration ./internal/modules/organization/... ./internal/modules/orgaudit/... ./internal/modules/orgcapability/... ./internal/modules/orgevent/... ./internal/platform/database ./internal/platform/migrate -count=1
sqlc vet
sqlc generate
```

[Local persistence and upgrade evidence](../docs/enterprise/evidence/f1-org002-organization-persistence.md).

### Private identity domain (EMP-IDN-001)

`membership/domain` separates employee identity/lifecycle from membership
periods, existing personal/managed account references, credential source
descriptors, optional driver-profile associations and private device
enrolments. No platform-wide person record is introduced. Employee register
fields remain IDN-002; licence content/eligibility remains IDN-014/F2.

Values are scoped, immutable and revisioned. An employee exists without a
login. Account binding leaves a period INVITED; activation requires an
active matching employee, bound account and confirmed admission/roles.
ENDED memberships and REVOKED enrolments cannot reopen; rehire retains the
employee identity but requires a new period. Automatic employee-return
reactivation cannot lift a managerial suspension. Managed references cannot
cross organizations or mix with the existing-account access model.
ValidateLiveMemberships refuses duplicate live employee/account links within
a supplied scoped set; it cannot discover rows or replace database uniqueness.

Credential descriptors carry no material or grant. Enrolment checks explicit
server proof/count/limit facts (personal 1–5; managed 1); revocation preserves
its class. The existing fingerprint parser is reused. All value/snapshot
formatting, nested JSON and slog redact identifiers; storage must map fields
explicitly. Rehydrate functions accept trusted storage, never client DTOs.

Current AUTH/IDN providers, concurrent uniqueness, last-owner row locking,
atomic effects, tokens, storage, routes and native device acceptance remain
owning-task work. This package grants no authority and is not wired into
API/worker production composition.

```sh
go test -race ./internal/modules/membership/domain ./internal/modules/identity/domain ./internal/modules/account/domain ./internal/modules/kernel ./internal/modules/organization/domain -count=1
```

See [29 lifecycle vectors](../contracts/testdata/enterprise/README.md#identity-lifecycle-v1)
and [local acceptance](../docs/enterprise/evidence/f1-idn001-identity-domain.md).

### Membership persistence and account deletion (EMP-IDN-003)

`membership/application` declares ordinary scoped and explicit-Tx storage;
`membership/adapters` maps fields with sqlc/pgx. Migration 000170 adds minimal
employee identities and membership periods under forced RLS. Composite FKs,
non-ENDED employee/account uniqueness, server period times, immutable IDs,
terminal history and CAS/replay protect the boundary. Employee register
attributes remain IDN-002; this migration invents no employee/account data.

ActiveByAccount returns only an ACTIVE period for a live account without an
account-deletion reservation; it never authorizes an action. AUTH-002 now supplies the SQL
admission/reduction policy from actual role periods and continuous surviving
owner coverage. No production runtime table grant or route is installed.
Managed pending references remain descriptive, not account provisioning.

The existing personal deletion/billing-exit flow checks this protection at
the database boundary before provider cancellation and again at account
status deletion. An owner-only, trigger-only inventory sees just that
account's links across organizations. Mutations use normal per-org RLS;
account references become independent period tombstones while employee,
organization and original ended history remain. Invited periods can end
without a role; protected live periods refuse with LAST_OWNER or unavailable
policy. Personal-account domain remains standard-library-only; application/
adapters translate bounded refusals. HTTP documents 409/503 without private IDs.

Account locks precede organization locks; composers prelock relevant account
UUIDs before root/employee writes, then root UUIDs. Billing-exit deletion
reservations cannot be rewritten or removed while the account remains live.
The AUTH-002 policy excludes reserved accounts from surviving owners.
No external call occurs while those locks are held.

```sh
# ANPFUEL_TEST_DATABASE_URL must point only to disposable PostGIS:
go test -race -tags=integration ./internal/modules/membership/... -count=1
go test -race -tags=integration -p 1 ./internal/modules/membership/... ./internal/modules/organization/... ./internal/modules/orgaudit/... ./internal/modules/orgcapability/... ./internal/modules/orgevent/... ./internal/modules/account/adapters ./internal/modules/payments/adapters ./internal/platform/database ./internal/platform/migrate -count=1 -timeout=240s
go test -race ./internal/modules/account/... ./internal/modules/payments/... ./internal/modules/membership/... ./internal/modules/kernel -count=1
sqlc vet
sqlc generate
```

[Local persistence, ordinary-owner upgrade and failure evidence](../docs/enterprise/evidence/f1-idn003-membership-persistence.md).

### Fixed role and permission catalog (EMP-AUTH-001)

`orgaccess/domain.SystemCatalog` exposes only immutable source descriptors
from org-roles-v1/org-permissions-v1: 13 canonical roles and 84 exact keys.
Rules retain originating role, assigned-scope/own-record coverage and all
matrix restrictions. Live membership/assignment, organization/plan and record
rules remain mandatory server conditions, including for OWNER. Station
rules require live authority; fleet/station agreement sides stay separate.
Representation aliases are metadata, not accepted organization role names.

Lists/rules are copied and sorted. Unknown roles/keys, duplicates and invalid
compiled source fail closed; there is no dynamic/custom registration. The
23 keys without matrix rows stay registered but denied, including
stock.reconcile (the literal two-segment compatibility key). Manage does not
imply read and paid tiers imply no role. A new grant needs an explicit
canonical matrix and catalog version change.

The catalog itself contains no private data or current actor proof. AUTH-002
supplies separate scoped assignment storage and membership/owner SQL policy;
AUTH-003 implements the current request authority and bound HTTP entry below.

```sh
go test -race ./internal/modules/orgaccess/domain ./internal/modules/membership/domain ./internal/modules/staff/domain ./internal/modules/stationprofile/domain ./internal/modules/kernel -count=1
```

See [1,092 descriptor cells](../contracts/testdata/enterprise/README.md#authorization-catalog-v1)
and [catalog acceptance](../docs/enterprise/evidence/f1-auth001-role-permission-catalog.md).

### Cost-centre management (EMP-ORG-006)

Apply additive migration `000175` before the new API/worker. The enterprise
factory mounts seven private company-cookie POST operations under
`/v1/orgs/{org_id}/cost-centres`: create/list, code/read and
`{cost_centre_id}/read`, change, archive, restore. Current OWNER/ADMIN
org.structure.manage, exact Origin/Host, CSRF and current membership apply.
Writes require fresh proof, the organizations pilot flag and the existing
org.structure synthetic budget. No grants, flag or production budget is seeded.

Names are descriptive and can repeat. Codes are trimmed and compared
case-insensitively within the organization, permanently reserved across
current and archived centres. A code correction keeps every old code linked
to the same stable ID; code/read resolves both old and current labels to
current authorized metadata. Existing allocations/export snapshots are not
remapped. An archived code never becomes available to another centre.

The three new tables use forced RLS. The ordinary migration owner needs schema
creation and REFERENCES on organizations/accounts/audit. Review runtime
SELECT/INSERT/UPDATE on org_cost_centres and SELECT/INSERT on immutable
org_cost_centre_codes/commands alongside existing proof/authority/audit/quota
ports. Code reservations and their current owner have deferred composite FKs;
centre, alias, audit and receipt commit together. Commit failures return no
provisional result, with unavailable classification even for a deferred FK
failure. Keep additive schema/history and disable admission on recovery;
previous schema-version checks pass, with no destructive down migration.

Cost-centre usage/admission uses the root-locking structure contract. Unknown
FK consumers of either a centre or its reserved aliases block archive until
their owning migration supplies actual scoped counts. Immutable aliases and
command receipts are historical references. The disposable test consumer does
not implement future budgeting/asset/employee/operation modules. See
[local cost-centre evidence](../docs/enterprise/evidence/f1-org006-cost-centres.md).

### Operating unit management (EMP-ORG-005)

Apply additive migration `000174` before the new API/worker. The actual
enterprise factory mounts six company-cookie POST operations under
`/v1/orgs/{org_id}/units`: create/list and `{unit_id}/read`, change, archive,
restore. Canonical OpenAPI specifies strict fields and required revisions.
Current org.structure.manage (OWNER/ADMIN), normal Origin/Host/CSRF and
membership checks apply. All mutations require fresh company proof,
organizations pilot capability and the org.structure admission class, bounded
by the existing identity v1 200/hour on each org/account/web-or-device axis.
These are synthetic limits; measured production budgets and cleanup remain due.

Unit names/codes are trimmed and unique among active units. Archived units
retain identity and can be renamed before restoration. Time-zone defaults
are captured from the current organization on the first command execution;
later changes never rewrite reporting snapshots. New optional establishment
links require a scoped declared company; retained existing links survive legal
withdrawal. Default-off/source synthetic terms admission stays unchanged.

The ordinary schema owner must own org_units and org_unit_guard() for the
migration, with REFERENCES on organizations/accounts/establishments/audit.
Review runtime SELECT/INSERT/UPDATE on units and SELECT/INSERT on the new
forced-RLS immutable org_unit_commands, alongside existing authority/audit/
quota ports. No production grants or flags are provisioned by this change.
Keep schema/history and disable the entry on recovery; prior schema checks
pass and old columns remain. No destructive down migration is supplied.

Current usage is resolved by org_unit_current_usage inside the locked scoped
transaction. Only actual immutable receipts are known historical consumers.
An unknown FK blocks archive until its owning module installs its real scoped
counts and root-serialized admission through org_unit_admit. The test-only
consumer demonstrates the four categories and concurrency contract; it does
not implement future fleet/budget/operation modules. LAST_UNIT and
STRUCTURE_IN_USE return 409; use details expose category names and exact
decimal-string counts without records. Unknown/malformed counts return 503.
See [local unit evidence](../docs/enterprise/evidence/f1-org005-operating-units.md).

### Private declared legal parties (EMP-ORG-004)

Apply additive migration `000173` before the new API/worker. With the
enterprise surface enabled, the actual enterprise factory mounts eight
cookie-only POST operations under `/v1/orgs/{org_id}/legal-entities`:
create/list, `{legal_entity_id}/read`, `identifier/read`, `withdraw`, and
`establishments/create`, `list`, `{establishment_id}/change` beneath the
parent. Use the canonical OpenAPI for exact paths and strict JSON schemas.
The normal company-cookie Origin/Host/CSRF checks and current
org.legal_entity.manage apply. Configuration writes and explicit identifier
reads require fresh proof; writes also require the current organizations
pilot flag and the separately bounded org.legal admission class.

Only organizations initialized with the source synthetic terms fixture
accept these operations. COMPANY validation reuses the numeric/alphanumeric
CNPJ kernel and creates an explicit first establishment; NATURAL_PERSON
requires the pinned verification/agreement purpose and valid CPF. Neither
creates verification, representation, membership or station rights. Default
views always mask CPF, including for managers. Explicit fresh identifier
reads commit an access audit before releasing content. No raw identifier
or name enters receipts, audit, logs or validation details.

The three new tables use forced RLS. The deployment operator must review
ordinary-role grants for SELECT/INSERT/UPDATE/DELETE on org_legal_entities
and org_establishments and SELECT/INSERT on immutable org_legal_commands,
in addition to the existing proof/authority/audit/quota ports. The schema
owner also needs ownership of the existing audit/event tables for the
bounded snake_case vocabulary constraints. No production grant, flag or
legal approval is installed here. The synthetic org.legal budget inherits
identity v1's 200/hour on each axis; measured live budgets and cleanup
activation remain due.

Withdrawal removes unreferenced content without verification history;
actual immediate/deferred foreign keys instead retain a REVOKED record.
Stable metadata-only receipts allow exact retry after removal, without
restoring private content or reusing its identity. Keep the additive schema
and history on recovery, disable the entry, and use the compatible previous
image; no destructive down migration is supplied. Run the affected
organization/auth/audit/event/company modules with `-race -tags=integration`
and a disposable ANPFUEL_TEST_DATABASE_URL. See
[local declaration evidence](../docs/enterprise/evidence/f1-org004-legal-declarations.md).

### Scoped role-period persistence (EMP-AUTH-002)

`orgaccess/application` and its sqlc/pgx adapter store fixed-role periods
under migration 000171's forced RLS and composite membership/assigner FKs.
Only ORG and SELF targets are supported; OWNER requires ORG and personal
access. Station roles await STATION authority support. Candidates require
a currently valid period, ACTIVE membership/employee and live account
without an account-deletion reservation; they are not authorization.

Creation compares exact intent at PostgreSQL microsecond precision; changed
intent or overlapping membership/role/scope periods conflict. End is a
terminal revision with server time, retaining original validity and history.
Normal methods return zero results on transaction or deferred-commit failure;
explicit-Tx methods leave final commit/rollback to the composing command.
Prelock all affected accounts in UUID order before any roots/writes, then
organization roots in UUID order, matching membership and erasure.

Real assignment policy replaces the unavailable membership hook. Owner
intervals must cover future time continuously, excluding deletion-reserved
accounts; assignment changes and account/membership reductions share the
root lock. Membership end also ends assignments atomically. Production
ordinary logins need SELECT/INSERT/UPDATE on org_role_assignments and SELECT
on the referenced membership/employee/account/billing tables; no grants are
automatically applied to a production login. Ordinary migration ownership,
forced RLS, previous compatibility and injected-failure recovery are tested.
Actor authorization, fresh authentication, entitlement/audit and production
composition remain owning-task work. Existing catalog grants are unchanged.

```sh
go test -race ./internal/modules/orgaccess/... -count=1
# ANPFUEL_TEST_DATABASE_URL must select disposable PostGIS:
go test -race -tags=integration ./internal/modules/orgaccess/... ./internal/modules/membership/... ./internal/modules/organization/... ./internal/modules/account/... ./internal/modules/payments/... -count=1
go vet ./internal/modules/orgaccess/... ./internal/modules/membership/... ./internal/modules/organization/...
sqlc vet
sqlc generate
```

### Current request authorization (EMP-AUTH-003)

Use orgaccess/adapters/http.Register with a source-owned application Policy,
scoped resource resolver and mandatory capability/record gates. It binds the
current authorization service to orgapi's strict normal proof entry; use
AccountSessions.Validate/Confirm for Android and the existing company cookie
Peek/Confirm pair for web. Fresh Android routes require explicit confirmation.
Verified account, contributor and device-key attribution remains explicit.

The one-statement LoadAuthority snapshot uses ordinary forced RLS and never
caches roles or accepts a client-selected context as proof. Prepare validates
current membership/org/role/resource scope; completion confirms proof/freshness,
then capabilities/limits and record policy. A finished request or a second
completion cannot reuse a decision. Handler commands still own fresh record
validation, transaction/audit composition and use of the decision's scoped
target. No production business route or runtime grant is installed here.

```sh
go test -race ./internal/modules/orgaccess/... ./internal/platform/orgapi ./internal/platform/apicontract
# Explicit disposable PostGIS only:
go test -race -tags=integration ./internal/platform/orgapi ./internal/modules/orgaccess/... ./internal/modules/membership/... ./internal/modules/companysession/... -count=1
```

See [current authority and real proof acceptance](../docs/enterprise/evidence/f1-auth003-current-authorization.md).

EMP-AUTH-004's orgaccess/adapters.PilotEntitlements implements that capability
gate using the same source catalog and orgcapability.Store.State. Pass the
same default-off global activation as the gateway; never call the authorizing
capability Service recursively. Current flag state cannot grant a role or
widen SELF. Off/missing flags refuse new work, preserving authorized existing
reads and explicit safety effects. Provider failure denies every effect.
Business writes still require GuardWrite/Run in their existing transaction.
See [pilot entitlement acceptance](../docs/enterprise/evidence/f1-auth004-pilot-entitlements.md).

### Organization operation time (EMP-PLAT-015)

orgtime/domain preserves declared occurred_at and trusted server received_at
as UTC instants, with entry IANA zone and declared device offset. Capture
uses the resolved unit zone, or organization zone when no unit exists, to
snapshot reporting day/zone/source. Invalid supplied unit zones fail.
Trip callers supply the start instant. Persist all snapshot fields together;
do not recalculate historical days when organization/unit zones change.

AuthorityAt returns received_at. Owning server flows must set it from their
clock, resolve current authority and supply context zones from scoped data;
parsing a client timestamp grants no authority. InPeriod is half-open.
AssessClockSkew takes a separate sync sample pair, not an old occurred_at,
and returns signed seconds/nonnegative nanos plus CLOCK_SKEW strictly above
the organization limit (default 300 seconds). It changes no timestamp and
can include transit delay; sync review/batch propagation remains OFFL scope.

The shared parser rejects leap seconds, unknown -00:00, invalid/overflowing
dates and more than nanosecond precision; it accepts numeric offsets up to
±18:00. Entry offset metadata is integral seconds within ±18h, independent
of zone rules. Standard IANA data is used with a Go bundled fallback through
kernel.LoadTimeZone, shared with organizations. A zero server receipt fails.
No route, migration, external dependency or live feature is added.

```sh
go test -race ./internal/modules/orgtime/domain -count=1
```

See the [99 shared Go/Kotlin vectors](../contracts/testdata/enterprise/README.md#operation-time-vectors-v1)
and [local evidence](../docs/enterprise/evidence/f1-plat015-operation-time.md).
Native device time-zone parity and composed consumer acceptance remain owed.

### Exact organization values (EMP-PLAT-009)

orgvalue/domain provides canonical integer parsing, paid-preserving
reconciliation and largest-remainder allocation by explicit line order.
It reuses financial-v1 bounds and L/M3 units; it introduces no schema,
route, activation or dependency. Kotlin EnterpriseValues executes the same
112 synthetic vectors in the existing domain module. Unknown is distinct
from zero, source-price text is retained and the personal driver allocator
keeps its own tie rule. Run the Go domain package with race and Kotlin
EnterpriseValuesTest for parity; see the
[shared oracle](../contracts/testdata/enterprise/README.md) and
[evidence](../docs/enterprise/evidence/f1-plat009-exact-values.md).

### Company account web sessions (EMP-PLAT-007)

With `ANPFUEL_ENTERPRISE_ENABLED=true`, the API mounts three POSTs under
`/v1/org-access/session/`: `login`, `read`, `logout`. The canonical origin is
`https://` plus `ANPFUEL_CANONICAL_HOST`. No Caddy/panel activation is part of
this task. Apply additive migration `000167` before the new API/worker.

Login accepts only `{"family_id":"...","access_token":"..."}` with current
account proof issued less than 15 minutes ago. It returns account metadata,
a CSRF value and the separate `__Host-empresas-session` cookie. The cookie
is host-only, Secure, HttpOnly, SameSite=Strict, Path=/; its plaintext never
appears in JSON or storage. Read/logout accept `{}` and require that cookie,
`X-CSRF`, and exactly the configured Origin and Host. Use JSON, no query
parameters and no signed/bearer headers on this channel. Bodies are capped
at 16 KiB. Responses, including failures, are no-store without ETags.

Idle expiry is 30 minutes; absolute expiry is at most eight hours, bounded
by the source family. Activity never refreshes the 15-minute authentication
time. Re-login rotates the presented same-account cookie/CSRF atomically.
Logout affects only the company cookie; account-wide revocation also
invalidates company access. Admin credentials and network fencing remain
independent. Sessions identify accounts, with organization permission owned
by AUTH. Organization gateway composition must supply both `WebSession`
and `WebSessionConfirm`; the latter checks declared freshness only after
current scope/permission succeeds and then renews idle time.

The existing privacy-retention worker purges company credentials in bounded
batches while the surface is off as well. No migration or dependency is
removed on rollback; the prior API remains schema-compatible. Test with
`go test -race -tags=integration ./internal/modules/companysession/...` and
an explicit disposable `ANPFUEL_TEST_DATABASE_URL`.
[Architecture and evidence](../docs/enterprise/09-architecture.md#31-implemented-company-account-session).

### Organization device enrolments (EMP-IDN-015)

The default-off enterprise composition now has a distinct signed existing-account
plane: `POST /v1/orgs/{org_id}/devices/enroll` and `/devices/check`. Both require
the actual personal family/access proof and registered bound P256 signing key;
only enrolment is a bootstrap exception to the current-enrolment check. It does
not bypass current membership, employee, capability, proof or source locks.
The existing identity-v1 signature clock envelope is preserved and expiry is
rechecked against database time after waits. Client-selected proof flags, keys,
account IDs, counts and limits are not authority.

`ANPFUEL_ENTERPRISE_DEVICE_LIMIT` is server-owned deployment configuration,
default3, range1–5. Slot counts and insertion serialize under the source root;
an operator lowering the limit does not erase existing periods. Managed
activation/recovery remains uninstalled behind GOV-013/IDN-006..008.

Fresh cookie self list/revoke and fresh `idn.credential.manage` manager list/revoke
operate under `/v1/orgs/{org_id}/memberships/{membership_id}/devices/`. Managers
can reduce access while the organization is suspended/archived or its pilot flag
is disabled. Responses contain scoped enrolment metadata, never personal session
families, contributor/key inventory or other organizations. Last-sync remains
null until a real sync producer exists.

Append-only migration194 installs FORCE-RLS periods, immutable audited command
receipts and membership-ending revocation. LOST_OR_COMPROMISED bars that key
throughout this organization. ACCESS_ENDED remains terminal for the old period;
an explicitly invited distinct ACTIVE membership can create a new enrolment for
the still-valid personal key. Neither class grants normal access or an intake
window. Personal families/keys/cloud devices and other organizations survive.
Native erase/acceptance, future intake, production grants/providers and release
certification remain separate gates; no immediate remote wipe is promised.

Runtime needs the reviewed scoped device table/receipt and source row-lock/nonce
privileges; migration194 creates no production grants. Its migration owner must
own `org_memberships` to install the ending trigger. The ordinary disposable
upgrade test proves missing grants fail rather than silently provisioning them.
Run the real source/protocol/upgrade suite with an explicit disposable PostGIS:

```sh
go test -race -tags=integration -timeout=20m ./internal/modules/organization/adapters -run TestOrganizationDevice -count=1
```

[Device acceptance evidence](../docs/enterprise/evidence/f1-idn015-device-registry.md).

### Private original transfers (EMP-PLAT-006)

`orgmedia` supplies bounded organization storage and audited member-bound
access ports. It adds no route, document migration or live intake. Its
default-off service requires current AUTH/DOC authority, a synthetic
declaration, a content inspector and the existing organization audit runner.
Use the existing `jobs.NewUUIDv4` generator for access/request IDs.

Original keys are derived under `org/<org_id>/doc/<document_id>/<sha256>`.
Conditional PUT plus a bounded verified GET refuses overwrite or substitution.
Only the synthetic JPEG inspector is supplied; other format inspectors and
durable ready/quarantine state stay with EMP-DOC-014/EMP-DOC-002.
Member-bound API access expires within five minutes, rechecks revocation and
authority, and never returns a storage URL. A process runs at most four
transfers, two per organization and one per member, refusing at once beyond
that, and one full JPEG decode at a time. Access tokens are refused when
issued in the verifier's future, so API replicas need synchronized clocks. Controllers use authenticated POST
body tokens, the shared private error envelope and the supplied content writer.
HTTP intake must set the request read deadline as the existing upload gateway
does; a Go context alone cannot interrupt an arbitrary Reader.
See the [storage contract](../docs/enterprise/09-architecture.md#91-private-document-storage-foundation).

Integration tests require disposable `ANPFUEL_TEST_DATABASE_URL` and all
`ANPFUEL_TEST_STORAGE_{ENDPOINT,BUCKET,REGION,ACCESS_KEY_ID,SECRET_ACCESS_KEY}`
values. Use `infra/compose.validation.yml` with a unique project and its
`storage-init` fixture. The loopback S3 option is explicit; shared/provider
and real-receipt activation retain their own gates.

### Photo-intake safety budget (RVA07)

`ANPFUEL_EVIDENCE_MAX_PENDING_PHOTOS` defaults to **0 (closed)**. The API
requires an explicit measured budget in 1..100 pending photo sessions and
private storage configuration. Keep the same budget across API replicas;
the source ceiling is a conservative guard, never a supported-user or VPS
capacity claim. A session can require both an original and a final copy.
Qualify the current binary/provider/DB resource envelope and deadline margin
before enabling intake in staging/production; record the measured lower
budget in deployment evidence. Increasing the source ceiling requires renewed
qualification. There is no production activation in this source checkpoint.

Every-minute evidence cleanup records a singleton epoch-fenced observation.
Startup, a running/failed/unknown pass, a successful observation older than
three minutes, a copy at its 22h cleanup boundary, a full pending budget or
a DB outage pauses new bytes. Missing configuration is not healthy by default.
Fresh reservations serialize the global capacity check and insertion;
confirmed byte replay emits no PUT. The existing worker process has four
serial safety dispatchers alongside one general dispatcher, with no new service.
The operational copy deadline stays first receipt +24h; cleanup starts at 22h.
Provider/version/backup and autonomous elapsed-time proof remain required.

## Bounded ANP location-candidate discovery

See the [Sorriso reliability audit](../docs/backend/STATION_RELIABILITY_AUDIT_20261013.md).
`go run ./cmd/station-discover --snapshot <immutable-edition> --uf MT --municipio SORRISO`
uses `ANPFUEL_DATABASE_URL` and stages only official records/candidate points.
It runs no migrations, canonical publication or location review. Exact snapshot
replay reuses stored results; new source editions need new snapshot identities.
Do not use a persistent database for integration tests.

## Official price ranking (MEX-28/29)

`GET /v1/prices/ranking` is the anonymous typed ANP weekly ranking. Required
query parameters are `source=ANP_WEEKLY`, `survey_week_start`, `fuel`, `unit`,
`geography` (`COUNTRY`/`STATE`/`MUNICIPALITY`), `metric`
(`OBSERVATION_PRICE`/`AREA_MEAN`), `group_by` (`STATION`/`MUNICIPALITY`/`STATE`)
and `sort` (`ASC`/`DESC`); optional `uf`, `ibge_code`, `condition=STANDARD`,
`limit` (1..100, default20) and `cursor`. Only ANP weekly STANDARD observations
are ranked; community prices and other conditions are refused, never
substituted. Results bind the exact source revision, coverage and
fuel/unit/geography/time, with exact rational means and max-min variation.
`Sampled=true` and `national_completeness_claimed=false` are always explicit.

ETag and cursors bind every filter, the policy and the source revision: a
correction that switches the current revision invalidates both. The in-process
cache is bounded (`ANPFUEL_PRICE_RANKING_CACHE_ENTRIES`, default128, `0`
disables) and TTL-bound (`..._TTL_SECONDS`, default60); every request still
resolves the current published revision, and an outage answers `503` no-store
instead of a cached page. `ANPFUEL_PRICE_RANKING_ENABLED=false` disables the
route. See [MEX-29](../docs/backend/evidence/mobile-expansion/MEX-29.md).

## Fuel quality source reads (FQ-08/FQ-08H)

The additive public quality API has a live bounded revision catalog:
`GET /v1/quality/revisions?stream=pmqc-assay-v1&limit=20`, then
`GET /v1/quality/revisions/{id}?stream=pmqc-assay-v1` for frozen provenance.
Streams also include `enforcement-v1` and `sanction-v1`. Existing summary, assay,
sample, action and sanction routes accept optional `revision_id`; omission reads
current. Select the exact revision when continuing a historical record cursor.
Superseded verified history is explicitly labeled; withdrawn/legacy revisions
have unavailable facts and null denominators. Catalog metadata is live, so restart
catalog traversal after imports to include newly inserted revisions.

`ANPFUEL_QUALITY_READS_ENABLED=false` contains all official quality routes.

Station-scoped sanction attribution is a separate, default-off contract:
`ANPFUEL_QUALITY_COMPANY_READS_ENABLED=false` enables the optional `station_id`
filter only on the sanction page/detail routes. It never enables source ingest
or relaxes global reads, and it does not accept `station_id` on actions.
Scoped responses are explicitly current-company-only
(`association_scope=CURRENT_COMPANY_ONLY`,
`historical_business_window_known=false`), expose no CNPJ or private proof, and
never fall back to global rows when the scope is empty. Each scoped request
revalidates the frozen source revision, the canonical station and fresh
Directory identity evidence before answering `304`; `ETag`/cursor seal the
revision, station, filters, limit and private scope fingerprint. Scoped
responses use `Cache-Control: public, max-age=0, must-revalidate`; outages and
the independent kill switch answer no-store errors. Unknown, duplicate or empty
filters are refused. Keep the switch off until staging/operator acceptance; see
[MEX-19](../docs/backend/evidence/mobile-expansion/MEX-19.md).

Native quality worker imports have a separate default-off switch,
`ANPFUEL_QUALITY_IMPORTS_ENABLED=false`. The general dispatcher always excludes
`quality-import`, `quality-discovery`, `quality-revalidate`, `quality-recompute`,
`quality-sanctions-import` and `quality-sanctions-discovery`; disabled source
jobs retain zero claim attempts. Enablement adds fixed slots for the native
import (trusted `/prepare_quality` executable shipped in the worker image and a
separate PostgreSQL advisory-lock connection across fetch, preparation and
publication, 210-second attempt, five-minute job lease), the bounded Directory
revalidation sweep and bounded live discovery (verified official listing page,
one archive inspection per run, durable cursor, created-only enqueue budget).
Safety slots remain independent. Bounded daily schedules are separate explicit
default-off switches (`ANPFUEL_QUALITY_DISCOVERY_ENABLED`,
`ANPFUEL_QUALITY_REVALIDATION_ENABLED`); they only enqueue work and never
activate unrelated ANP/registry imports. Keep everything disabled until
deployment selection; see
[FQ-05W5](../docs/backend/evidence/fuel-quality/FQ05W5_WORKER_COMPOSITION.md),
[FQ-05W6](../docs/backend/evidence/fuel-quality/FQ05W6_DIRECTORY_REVALIDATION.md),
[FQ-05W7](../docs/backend/evidence/fuel-quality/FQ05W7_LIVE_DISCOVERY.md) and
[FQ-05W8](../docs/backend/evidence/fuel-quality/FQ05W8_IMAGE_SCHEDULING.md).
Native fine jobs use trusted `/prepare_sanctions` for2025 and
`/prepare_fine_history` for the six separately verified historical profiles, under
one whole-attempt source lease and verified/minimized typed publication. Discovery
inspects one of seven actual supported hrefs per persistent cursor step. Grouped
2016–2019 remains one resource;2021 is verified CSV despite its .xlsx filename.
Strict resource/profile admission precedes fetch, and raw private files are
removed before publication. Both import activation and the independent
`ANPFUEL_QUALITY_SANCTIONS_DISCOVERY_ENABLED=false` daily schedule stay off until
runtime acceptance. The seven files reconcile35,911 source records into35,612
accepted observations,293 CPF quarantines and six source metadata records; these
counts do not establish national case coverage or station association.

Fresh bounded Directory planning now records explicit current-company-only
associations and guards real bindings in the publication transaction. Unknown/
ambiguous/retired/foreign-branch company evidence remains unassociated; fine due
dates never establish incident dates. This does not prove historical operation
at a site. Public station filtering/live read fences and enforcement XLSX remain
owed. See
[FQ-07R5D](../docs/backend/evidence/fuel-quality/FQ07R5D_HISTORICAL_FINE_PUBLICATION.md),
[FQ-07R5E](../docs/backend/evidence/fuel-quality/FQ07R5E_HISTORICAL_FINE_WORKER.md)
and [FQ-07R6D](../docs/backend/evidence/fuel-quality/FQ07R6D_COMPANY_WORKER_COMPOSITION.md).
Private signed document-free report commands/get/list remain separate; disabling
`ANPFUEL_QUALITY_REPORTS_ENABLED` contains new/edit intake while preserving signed
owner appeal, withdrawal and reads. Existing account write quota applies per account
transactionally; acknowledged replay costs no second unit. No payment/GPS/document
or business portal requirement is introduced. See [review](../docs/backend/evidence/fuel-quality/FQ11_DOCUMENT_FREE_REVIEW.md)
and [historical validation](../docs/backend/evidence/fuel-quality/FQ08H_HISTORICAL_READS.md).

Native SICOM fine observations have a separate source contract and typed publisher.
The native CSV carries a GRM due day and month-only extraction precision: public
sanction responses expose `as_of: null`, `as_of_precision: MONTH`,
`extraction_month`, `due_on`, raw amount/payment text and minor units separately.
`identity_kind: CONTENT_OBSERVATION` identifies a minimized-content digest, not a
source-issued fine number. Exact-day legacy snapshots stay readable. Source age
uses the existing180-day policy independently of local import publication time;
a fresh import of old source data remains stale, including empty revisions.
Company names/CPF are discarded; company identifiers stay out of public responses.
See [native preparation](../docs/backend/evidence/fuel-quality/FQ07R1_OFFICIAL_FINE_PREPARER.md),
[typed boundary](../docs/backend/evidence/fuel-quality/FQ07R2_VERIFIED_FINE_BRIDGE.md),
[storage](../docs/backend/evidence/fuel-quality/FQ07R3A_NATIVE_FINE_STORAGE.md) and
[atomic publication/read clocks](../docs/backend/evidence/fuel-quality/FQ07R3B_NATIVE_FINE_PUBLICATION.md).
This source publication boundary is not yet a composed import/discovery handler;
no full fine history, station association or runtime activation is certified.

Critical quality persistence/read/job checks, from backend/ against disposable
local PostGIS (never the staging database):

```sh
ANPFUEL_TEST_QUALITY_PREPARER_PATH=/tmp/abastevo-integration-rust/debug/prepare_quality ANPFUEL_TEST_SANCTIONS_PREPARER_PATH=/tmp/abastevo-integration-rust/debug/prepare_sanctions ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH=/tmp/abastevo-integration-rust/debug/prepare_fine_history go test -race -tags=integration ./internal/modules/quality/... -count=1
go test ./internal/platform/apicontract -count=1
```

Explicit `-count=1` is required after changing external OpenAPI/vector inputs;
Go test cache does not track those files. Local source acceptance does not imply
live native imports, score calibration, mobile/device or production acceptance.

The quality prepared bridge additionally checks the real Rust CLI and atomic SQL
publication; the integration workflow builds both source preparers from the same
locked crate. Prepared input is integrity-checked before any read-only station
resolution, and public sample keys include full source/sample/CNPJ/date/fuel scope.
See [FQ-05C bridge evidence](../docs/backend/evidence/fuel-quality/FQ05C_PREPARED_BRIDGE.md).
This checkpoint does not enable native discovery/import in the deployed worker.


The native PMQC archive adapter (FQ-05W2) is file-backed and source-bound. It
checks bounded ZIP metadata before allocating entries, preserves each monthly
CSV as a separate edition, and runs only the configured trusted preparer in a
private workspace. Unsupported source contracts fail before Directory resolution.
Native worker composition/activation is still pending; this adapter alone does
not import anything into a deployed database.

For explicit offline source evidence, acquire the official archives separately
into a private directory and run from backend/ (paths are local, never committed):

```sh
ANPFUEL_TEST_QUALITY_PREPARER_PATH=/tmp/abastevo-integration-rust/debug/prepare_quality \
ANPFUEL_TEST_QUALITY_SEPTEMBER_ZIP=/private/pmqc_2026_09.zip \
ANPFUEL_TEST_QUALITY_ANNUAL_ZIP=/private/pmqc_2025.zip \
go test -count=1 -v -tags=integration,qualitysource ./internal/modules/quality/adapters/native -run TestOfficialArchivePreparationManual
```

The manual source test pins exact acquisition hashes, logs counts only and
publishes nothing. January 2025 source clocks are preserved by [FQ-02C](../docs/backend/evidence/fuel-quality/FQ02C_SOURCE_CLOCKS.md), with zero date-format quarantine and unchanged September manifest. Native Directory/fetch/worker composition is still required before historical activation. See [FQ-05W2](../docs/backend/evidence/fuel-quality/FQ05W2_ARCHIVE_PROCESS.md).

## Shutdown during staging rollouts

`ANPFUEL_SHUTDOWN_DRAIN_DELAY` defaults to `10s` and accepts `0s` through
`30s`. SIGTERM immediately makes `/health/ready` return 503/no-store while
normal traffic and liveness remain available during that delay. The listener
then drains active requests for `ANPFUEL_SHUTDOWN_TIMEOUT` (default `10s`).
Keep **delay + timeout + 5s < terminationGracePeriodSeconds**. The staging
manifest pins both values and 60s grace. Workers stop new claims, join all
loops and finalize held leases using a separate bounded 5s fenced operation;
expiry remains recovery when the database cannot acknowledge shutdown.
