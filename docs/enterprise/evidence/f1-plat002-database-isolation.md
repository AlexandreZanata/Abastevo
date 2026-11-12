# F1 EMP-PLAT-002 private database isolation

Status: LOCAL_DONE
Validation: PASS

Date 2026-11-11. Delivery state INTEGRATION_PENDING, F1 IN_PROGRESS.
Dependency EMP-PLAT-001 is committed at `4c4d6e3b` above the F0 checkpoint
`39a2d725`. The isolated construction branch is `codex/enterprise-f0-f1`.
Fetched main/dev remain at `d8bdc642`. Publication to GitHub was explicitly
requested for this task. Protected dev-to-main integration is pending.
Issue tracking is ISSUE_PENDING after automatic approval rejected optional
issue creation. The stable task ID remains the local tracker.

## Scope and tested source

B-BR-EMP-09 and BUC-EMP-05 were recorded before implementation.
`database.WithOrgTx` validates explicit organization scope, rejects unsafe
runtime roles and installs transaction-local scope with a parameterized
query. Its callback uses the existing pgx transaction lifecycle.

The executable SQL template demonstrates scoped primary/unique/foreign
keys, an indexed child reference and forced read/write row-level policies.
It lives outside the numbered production migration chain. EMP-ORG-002
owns the organization root. Subsequent feature tasks add their numbered
migrations and root references using this convention.

The shared integration-only `orgtest` harness creates its own fully
migrated database and ordinary login from an explicit disposable PostGIS
DSN. The JSON fixture has synthetic A/B organizations with the same record
ID and number. Runtime grants cover only the two example tables.

Tested source blobs are `f39639982f859307035b68848eaefc87e3ad4c7e`
for the transaction helper and `3bbf8891d1d67750879c96e9dcbaf9b185631c91`
for the SQL template. Source and evidence are committed together above
the dependency checkpoint.

## Executed validation

Linux amd64, Go 1.27.3 and the pinned PostGIS 18/3.6 image from
`infra/compose.validation.yml`. A dedicated tmpfs project
`enterprise-plat002-20261111` used an ephemeral loopback port.
All database work targeted its disposable databases and generated roles.
Commands ran in `backend/` with `GOCACHE=/tmp/enterprise-go-build` and
`ANPFUEL_TEST_DATABASE_URL` set to that project's database.

- RED first reproduced the absent `WithOrgTx` function.
- Before policies were added, the PostGIS tests observed both organizations
  without scope, an unfiltered cross-organization read, and an accepted
  foreign insert. All three isolation assertions failed as intended.
- `go test -race -tags=integration ./internal/platform/database -run 'Test(OrganizationTemplate|WithOrgTx)' -count=1 -v`
  passed the 18 isolation subtests, migration recovery and missing-scope test.
- `go test -race -tags=integration -coverprofile=/tmp/enterprise-plat002-coverage.out ./internal/platform/database ./internal/platform/migrate ./internal/modules/kernel -count=1`
  passed existing and new tests. Database coverage is 92.3%, migration
  coverage 82.9% and kernel coverage 94.4%.
- `go test -race ./internal/platform/database ./internal/platform/migrate ./internal/modules/kernel`
  passed the affected unit suites.
- `go vet -tags=integration ./internal/platform/database ./internal/platform/orgtest`
  and pinned staticcheck v0.8.1 with `GOTOOLCHAIN=go1.27.1` passed.
- `go build ./cmd/api ./cmd/worker ./cmd/ops` passed.
- Explicit gofmt, the enterprise general checker and checker regressions,
  diff/whitespace checks and changed-file secret review passed.

## Risk cases and recovery

The ordinary login reads zero rows without scope and cannot insert, alter
policies or truncate. Correctly scoped CRUD succeeds. Scoped duplicates fail,
while colliding identifiers and numbers in A/B coexist. Foreign reads,
updates, deletes and joins expose nothing. Foreign inserts, organization
changes and composite foreign references are refused.

Forced RLS applies even to a nonprivileged table owner. The transaction
helper also refuses owner logins, superuser migration connections,
BYPASSRLS/CREATEROLE attributes and privileged membership with inheritance
disabled. Disabling row security does not grant access.

The single-connection pool clears scope after commit and rollback. A failed
callback rolls back writes and reuses the same physical connection.
Cancellation rolls back writes and releases the pool. Two transactions
held at a deterministic barrier use independent organizations and each
unfiltered update affects exactly one row.

The migration rehearsal applies the production chain to an empty PostGIS
database, then injects the template as a test-only migration `999900`.
A deliberate division-by-zero after DDL leaves no example tables or ledger
entry. Retrying succeeds and repeated application is a no-op with rows
preserved. The previous embedded migration chain passes `migrate.Check`
against the additive schema and retains its compatibility floor.
A synthetic public catalog row created before the template remains
unchanged and readable under either organization and without organization
scope. A final targeted recovery run passed after adding this assertion.

Recovery removes the new caller entry point and preserves data. Each
feature's actual migration and previous-image release rehearsal follow its
own task. API authorization, role provisioning, audit, app and provider
activation continue in their owning tasks.

Next bounded task is EMP-PLAT-003, the organization audit trail.
