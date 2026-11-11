# F1 EMP-IDN-003 membership persistence and erasure fence

Status: LOCAL_DONE
Validation: PASS

2026-11-11. Database/source acceptance, INTEGRATION_PENDING through
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202). F1/M1 remain open.
Actual AUTH-002 role/owner policy is **UNAVAILABLE by default**; the task does
not activate membership admission, a real authority provider or a new route.

## Entry and bounded scope

The owner requested the next enterprise task and GitHub publication.
The isolated codex/enterprise-f0-f1 worktree entered clean at published
3e88a5e9 (IDN-001), matching remote dev and the open dev-to-main PR202;
main ef96cd13 is its ancestor. Occupied local dev and unrelated work remain
preserved. IDN-003 depends on accepted IDN-001/ORG-002; ORG-003 still needs
actual role assignments. Current progress/fast card, task, identity entities,
state/privacy rules, account deletion, billing exits and scoped SQL precedents
were read. B-BR-EMP-43/44 and BUC-EMP-22 precede implementation; tracing the
existing billing flow clarified that protection must precede provider calls.

The first real-PostGIS RED test proved the private tables absent. Migration
000170 and the scoped adapter make it pass. No dependency is added. Scope
is minimal employee identities, membership periods, uniqueness/CAS and the
database account-erasure safeguard. Employee register fields, invitations,
managed provisioning, full offboarding and erasure UX/rights are separate.

## Persisted boundary and history

org_employee_identities holds only organization/employee ID, lifecycle,
revision and optional same-organization merge pointer. It seeds no people,
names, contact, login or fabricated record. org_memberships holds the period
ID, composite employee reference, optional existing account link, access
model, state/revision, suspension/end codes and server timestamps.
Organization/employee FKs and forced read/write RLS protect both tables.
Partial indexes reserve employee/account per organization for every non-ENDED
period. Account-less invited periods do not collide through NULL accounts.
Periods may share account IDs across independent organizations, not access.

Queries predicate every private operation on resolved org_id. Repository
convenience methods use the existing ordinary-login transaction guard;
explicit-Tx methods check installed scope and leave commit to the audit/
admission command. Field mapping is explicit because domain JSON/formatting
redacts values. Create requires initial identity or INVITED/revision one;
updates use expected revision+1 and exact replay. Changed intent conflicts.
Failed commit/cancellation reports no successful write. Ended periods cannot
reopen or be deleted/truncated; rehire creates a new period on the employee.

SQL guards enforce legal edges, one-time account binding, immutable IDs/model,
database-owned times and no silent history rewrite. Employee inactivation/
termination cannot bypass the associated membership effects. Legacy root
anchors cannot silently acquire employee records; new private identity/
admission requires initialized PENDING_SETUP/ACTIVE roots.

ActiveByAccount selects only ACTIVE periods for a live account with no
account-deletion reservation. A role or credential alone never qualifies an
invited, suspended or ended period. This is a candidate lookup, not an
authorization decision. Managed pending references do not provision or
verify a managed account; real identity/model proof remains gated.

## Last-owner and account-erasure contract

The SQL policy port org_membership_access_policy returns UNAVAILABLE until
AUTH-002 derives ALLOW/LAST_OWNER from actual assignments. Activation and
access reduction call it while holding the organization row lock. Missing
policy cannot be enabled by a request field or source default role. Tests
use an explicitly disposable role-fact table/function, never shipped as a
production catalog or acceptance of the real AUTH provider.

Normal membership changes lock the account with SHARE before the root;
SHARE conflicts with status updates, unlike KEY SHARE. Composing commands
prelock relevant account UUIDs before root/employee writes; multiple roots
are locked in UUID order. Future role mutations must use this same root
lock and exclude deletion-reserved accounts from surviving-owner counts.

The existing account-deletion flow first requests a durable billing exit,
then changes account status. A BEFORE INSERT trigger on its ACCOUNT_DELETION
intent checks access protection before the service can call a provider.
The intent commits as an admission fence; another owner cannot count that
account as its surviving owner. Intent account/key/reason are immutable and
the deletion reservation cannot be removed while the account is live or
still linked. OWNER_REQUEST cancellation remains distinct. Both store replay
paths reject changed reasons rather than reusing an unprotected intent.

An AFTER account status trigger repeats protection and ends/unlinks periods
in the same transaction as personal deletion/revocation/intents. It generates
a separate random tombstone for each period, preserving employee/root rows,
original end reasons/times and identifiers. A one-time privacy unlink can
advance an already-ENDED revision without reopening its state. Tombstones
rehydrate only on ended, account-less values and remain redacted.

The internal functions are SECURITY DEFINER with fixed pg_catalog search
path, qualified table names and PUBLIC execution revoked. Their exact owner
must match the private table owner. The only cross-organization RLS exception
is owner-only SELECT inventory for the current deletion account. Ordinary
roles cannot satisfy it by setting a GUC. All writes then use normal org
RLS, one organization at a time, restoring transaction-local settings.
No BYPASSRLS login or table grant is created. The upgrade test uses a
NOLOGIN/NOSUPERUSER/NOBYPASSRLS migrator and proves both organizations unlink.
Owner drift, unknown policy or last-owner refusal rolls back the account and
private effects. Physical account deletion cannot bypass the retained FK.

Account domain stays standard-library-only. The account application/adapters
translate the shared SQL/payment refusal to bounded account errors. Existing
HTTP deletion returns 409 LAST_OWNER or 503 policy unavailable, without
organization identifiers, and the OpenAPI operation describes both.

## Validation and recovery

Disposable PostgreSQL 18/PostGIS 3.6 uses the repository-pinned immutable image,
loopback-only port and tmpfs; synthetic accounts/fixtures only. Ordinary runtime
roles own no RLS tables. Erasure tests grant the legacy account-table DML
needed by the existing deletion flow inside their disposable databases;
this is not a production grant manifest. The separate ordinary migrator test
needs only its owned tables plus explicit root reference/read/lock privileges.
Go toolchain 1.27.4, sqlc 1.31.1; staticcheck v0.8.1 uses Go 1.27.1.

Commands from backend/ (test DSN points only to the disposable fixture):

```sh
go test -race -tags=integration ./internal/modules/membership/... -coverprofile=/tmp/enterprise-idn003-coverage.out -count=1 -timeout=180s
go test -race -tags=integration -p 1 ./internal/modules/membership/... ./internal/modules/organization/... ./internal/modules/orgaudit/... ./internal/modules/orgcapability/... ./internal/modules/orgevent/... ./internal/modules/account/adapters ./internal/modules/payments/adapters ./internal/platform/database ./internal/platform/migrate -count=1 -timeout=240s
go test -race ./internal/modules/account/... ./internal/modules/payments/... ./internal/modules/membership/... ./internal/modules/kernel -count=1
go vet -tags=integration ./internal/modules/membership/... ./internal/modules/account/... ./internal/modules/payments/... ./internal/modules/kernel
staticcheck -tags=integration ./internal/modules/membership/... ./internal/modules/account/... ./internal/modules/payments/... ./internal/modules/kernel
go build ./cmd/api ./cmd/worker ./cmd/ops
sqlc vet
sqlc generate
```

All affected suites pass without skips. Adapter coverage **93.6%**; domain
**99.6%**. Sixteen adapter test entry points (15 using real PostGIS) cover
scope/role denial, composite references, per-org independence, duplicate live
account/employee races, 12-way CAS, exact replay, server times, terminal SQL
guards, two-owner end/reservation races, policy-unavailable refusal, actual
account/billing service deletion, raw status/physical-delete refusal,
GUC-spoof denial, cross-org tombstones, audit rollback, failed commit,
observed-lock cancellation and binding losing to account deletion.

The upgrade from a populated 000169 chain preserves account/audit/billing/
public rows, seeds no identity/access rows and accepts previous/current
migrate.Check. Injected SQLSTATE 22012 rolls back tables/functions/triggers/
ledger; retry applies once and then no-ops. Schema floor remains 000132.
Recovery retains the additive schema/history and disables the new entry
point; no destructive down or weakening of RLS is a rollback strategy.

The ordinary-owner test caught a real RLS visibility failure after unlinking
the new tuple; per-org writes fixed it without a write-policy exception.
The account stdlib guard caught a cross-domain import; translation moved to
application/adapters. An unused six-line payment integration-test helper was
removed after scoped staticcheck found it; no assertion was removed. Local
HTTP fixture tests needed network permission and then passed unchanged.

OpenAPI validation/lint (zero errors), changed Go formatting, stable sqlc
generation, plan verifier/three regressions, added document links, diff check
and staged secret scan pass. The plan retains 348 tasks, 545 rules, 167 use
cases, 145 screens and zero problems. New application/adapter packages enter
the quick manifest. Existing OpenAPI warnings are not declared eliminated.

## Next and retained gates

AUTH-001 is the next eligible identity/access foundation, then AUTH-002
installs the actual locked role/last-owner policy. IDN-002 still needs
AUTH-003; ORG-003 still needs AUTH-002. This task does not claim F1/M1 closure.

Existing single-family revocation decisions, real AUTH/IDN providers,
production non-bypass grants, composed business-router gate, vault inventory/
sweep/revoked-device cleanup, native acceptance, EMP-DOC-014, staging lifecycle/
backup roles, PLAT-012/GOV-014, aggregate Kotlin coverage and legal receipt
gates remain owed. F6 owns complete erasure notice/rights/retention and
restore-ledger reconciliation. No VPS rollout, merge or certification occurs.
