# F1 EMP-PLAT-003 organization audit trail

Status: LOCAL_DONE
Validation: PASS

Date 2026-11-11. Delivery state INTEGRATION_PENDING, F1 IN_PROGRESS.
Dependency EMP-PLAT-002 is committed and published at `da669216`.
The isolated construction branch remains `codex/enterprise-f0-f1`.
Fetched main is `d8bdc642` and dev is `231e63b7`, whose changes leave the
backend migrations and sqlc configuration unchanged. The occupied local dev
and other worktrees are preserved. GitHub publication was requested for this
task. Protected dev-to-main integration remains pending.

## Scope and source

B-BR-EMP-10 and BUC-EMP-06 were recorded before implementation.
The new `orgaudit` module exposes an atomic mutation runner and an
authorization-gated read service, backed by module-owned sqlc queries.
It reuses the admin reservation/audit/transaction pattern and the explicit
organization scope from EMP-PLAT-002.

Source-owned catalogs copy their action and reason definitions. Events
carry verified actor account, optional delegation and target UUID references,
registered codes, the server-computed canonical command digest and database
time. Human explanations stay in their business record. The catalog accepts
registered vocabulary and rejects content or credentials passed as metadata.

The runner reserves the event before the business callback and commits both
together. Matching retries skip the callback. Different digest or metadata
under the same organization/operation returns a bounded conflict.

Migration `000164` has a composite key, forced read/write RLS, a scoped
pagination index and append-only triggers covering UPDATE, DELETE and
TRUNCATE. The compatibility floor stays `000132`. The organization-root FK
follows in EMP-ORG-002 through another migration. Actor references remain
historical through account offboarding.

Tested source blobs are `64bdc931577f5edadbca0ccbe2eec101bad224eb` for
the migration and `0485bab1841a3c8b1a06184236017508e0cf3042` for the adapter.
The tested source and evidence travel in the same task commit.

## Executed validation

Linux amd64, Go 1.27.3, sqlc v1.31.1 and the pinned PostGIS 18/3.6 image.
The unique tmpfs Compose project `enterprise-plat003-20261111` supplied the
explicit disposable `ANPFUEL_TEST_DATABASE_URL`. Its database and generated
ordinary runtime logins were the only test destinations.
Go commands used `GOCACHE=/tmp/enterprise-go-build`.
After the tool environment restarted, the disposable database was recreated
on a new loopback port and the final affected PostGIS/race suites were rerun.

- RED reproduced the missing domain implementation before the event and
  vocabulary rules were added. GREEN passed domain and application negatives.
- `go test -race -tags=integration -coverprofile=/tmp/enterprise-plat003-coverage.out ./internal/modules/orgaudit/... ./internal/platform/database ./internal/platform/migrate ./internal/modules/kernel -count=1`
  passed all affected suites. Orgaudit coverage is 100% domain, 100%
  application and 95.8% adapter.
- A final targeted `TestAuditMigrationUpgrade` run passed after constraining
  the historical upgrade fixture to migrations through `000164`.
- `go test -race ./internal/modules/orgaudit/...` passed the unit suites.
- `go vet -tags=integration ./internal/modules/orgaudit/... ./internal/platform/orgtest`
  and staticcheck v0.8.1 under `GOTOOLCHAIN=go1.27.1` passed.
- `sqlc generate`, `sqlc vet` and `sqlc diff` passed.
- `go build ./cmd/api ./cmd/worker ./cmd/ops` passed.
- The quick manifest includes the three new behavioral packages. Its focused
  selection harness, explicit gofmt checks, enterprise general checker and
  checker regressions, diff/whitespace and secret review passed.

## Risk evidence

Twelve concurrent attempts at one operation produce one event and one
business change, with eleven replays. Reusing the operation with changed
request digest or actor metadata is refused before the callback.
Two organizations can hold transactions at a deterministic barrier under
the same operation ID and each commit their own event and change.

Business failure rolls back both and permits a later retry. An unavailable
audit INSERT prevents the business callback. Cancellation and a deliberately
deferred commit failure leave neither new event nor business change.
The actor and delegation survive the storage round trip with server time.

Reads enforce current authority on each application entry. Tests refuse
foreign and revoked decisions before storage. SQL predicates and RLS hide
foreign rows, and an absent scope sees zero events. Even with an accidental
full table grant, UPDATE, DELETE and TRUNCATE fail. Foreign inserts and
credential/document-shaped reason payloads are rejected by the database.
Pagination preserves order across equal timestamps using the operation ID.

## Migration and previous runtime

The shared harness accepts a previous migration chain for upgrade rehearsals.
The test upgrades a populated pre-000164 database, injects a failure after
DDL, verifies schema/ledger rollback, retries the original file and confirms
idempotent reapplication. Previous public data and the compatibility floor
are preserved. The previous embedded chain accepts the additive schema.

An actual API executable was built from `git archive da669216 backend`
with `go build -buildvcs=false` because the source archive has no Git metadata.
Its SHA-256 was
`473e88e4bdbb1f70557b68ec669a419472c99853beb33a60a5f4c93f20d42ae3`.
The previous API started on loopback against the disposable database after
the current migrator applied through `000164`. `GET /health/ready` returned
200 with `{"status":"ready"}` and the process stopped cleanly. Inherited
ANPFUEL settings were cleared and external discovery was explicitly disabled.

## Following integration

Feature modules register vocabulary and use the runner after their current
write-authority check. The authorizer port requires organization-wide
`audit.event.read` or approved current support access. Binding the concrete
membership/support policy, HTTP routes and private feature commands belongs
to their AUTH, BACK and API tasks. Retention requires its separately reviewed
lifecycle path. Next bounded task is EMP-PLAT-004.
