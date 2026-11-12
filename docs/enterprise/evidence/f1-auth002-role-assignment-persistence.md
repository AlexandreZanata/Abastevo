# F1 EMP-AUTH-002 scoped role-assignment persistence

Status: LOCAL_DONE
Validation: PASS

2026-11-11. Local source acceptance above published 1c7fd66b;
INTEGRATION_PENDING in cumulative [PR202](https://github.com/AlexandreZanata/Abastevo/pull/202).
F1/M1 remains open. The maintainer authorized the whole enterprise batch,
atomic task commits and phase publication, recommended decisions recorded
for later review, emulator tests when needed and final current-head/base CI.
The occupied dev checkout and unrelated work remain preserved.

## Contract and decisions

AUTH-001 and IDN-003 supply the catalog and persisted membership periods.
Read the selected backlog, permissions §§3–5/8, identity §6, current progress,
fast execution and delivery policy. B-BR-EMP-47/48 and BUC-EMP-24 precede
behavior. The domain RED failed on missing Assignment APIs; the PostGIS RED
failed with SQLSTATE 42P01 for the not-yet-created assignment table.

Recommended interpretations adopted under the maintainer's authorization:

- F1 supports ORG targeting the organization and SELF targeting the same
  membership. OWNER uses ORG; station roles await STATION/live-authority
  support. Structural scopes and custom/delegated roles remain later tasks.
- A bound INVITED period can stage assignments for an atomic admission.
  Staging never makes it an authorization candidate; managed access cannot
  receive OWNER. This resolves the activation/assignment ordering without
  trusting a client-provided role count.
- Last-owner safety covers future expiry as well as the instant of a write.
  Surviving active owners' half-open periods must continuously cover future
  time. PostgreSQL's existing multirange union allows adjoining finite and
  scheduled periods without a new extension, expiry job or blanket ban on
  finite OWNER assignments. Deleted/suspended accounts and deletion billing
  reservations never count as surviving owners.
- Ending retains original validity and adds a terminal server timestamp and
  revision. Cancellation before start has an empty effective interval.
  Persisted time uses PostgreSQL microsecond precision; replay comparisons
  use that precision explicitly. Current actor grants, fresh authentication,
  entitlement, command idempotency/audit and record policy remain AUTH-003/009
  and the owning business commands, not storage assumptions.

## Implementation and risk boundaries

Migration 000171 adds org_role_assignments, forced RLS, composite membership
and assigner references, supported role/scope/target checks, server creation
and end timestamps, overlap refusal under the root lock, immutable intent,
revision checks and DELETE/TRUNCATE/history refusal. Existing migrations
are unchanged. The catalog and its 23 unassigned permission keys are unchanged.

The real role policy replaces 000170's unavailable hook. Admission requires
an effective stored period. Access reductions share the organization lock
and preserve continuous owner coverage. The deferred owner check selects
the immutable row scope and restores the transaction scope; changing the
current GUC before commit cannot skip it. Membership end also ends every
remaining assignment in the same transaction. Personal-account status
reductions use the existing trigger-only, owner-checked erasure preflight;
the billing intent continues to fence ownership before provider calls.

The domain is immutable, validates scope/roles/periods and redacts fmt, JSON
and slog views. Application ports and the owned sqlc/pgx adapter provide
scoped Get/Create/End/Candidates and explicit-Tx composition. Create retries
compare original intent even after an assignment ends. End retries converge;
stale revisions conflict. Normal methods return zero results on rollback,
cancel or deferred commit failure. Candidates filter current period,
membership, employee/account status and deletion reservations; they never
constitute actor, organization/plan, permission or record authorization.

Composers prelock all affected account UUIDs, then roots, in sorted order
before writes. New production runtime grants are not applied automatically.
Runtime composition needs SELECT/INSERT/UPDATE on role history, including
UPDATE for the membership-end trigger; reference reads require the existing
membership/employee/account/billing grants. Test fixture grants model this
explicitly. No provider, business route or production capability is activated.

## Validation

Disposable pinned PostGIS 18/3.6, ordinary runtime login and actual forced RLS:

- Foreign membership/target/scope and unsupported/station/unknown roles;
  managed OWNER, unbound or ended memberships; overlapping periods;
  history mutation, deletion/truncation and ending twice.
- Current/expired/scheduled/ended periods and invited/suspended/deleted
  account candidates; suspension/reactivation resolves current roles again.
- Twelve simultaneous identical creates and twelve ends each have one
  changing result. Two concurrent owner reductions preserve one owner.
  Concurrent deletion billing reservations likewise have one winner.
- Finite owner expiry cannot create a future gap; adjacent successor
  periods cover it. Changing the transaction scope cannot skip a deferred
  owner check. Failed owner commit returns no changed result/value.
- Exact/changed retries, rollback, explicit scope mismatch, unsafe runtime
  role, context cancellation and blocked-account lock timeout.
  Final review reproduced a root-first wait for an unbound first assignment;
  creation now refuses an unbound membership before taking that root lock.
  Immutable historical intent is checked before locking, then checked again
  after the root lock for concurrent first creates. An exact creation retry
  after account erasure remains read-only and converges. Both focused REDs
  and the complete post-review authorization suite pass.
- Existing account erasure and nonowner offboarding end assignments
  atomically. Account suspension cannot remove the last available owner.
- Populated 000170 → 000171 upgrade under an ordinary non-BYPASS migrator;
  injected SQLSTATE 22012 rolls back DDL, policy, triggers and ledger;
  recovery, migration replay, previous-binary compatibility and unchanged
  prior private/public rows. Ordinary table-owning definer erasure both
  refuses the last owner and succeeds with a surviving owner.

Commands from backend/ (explicit disposable ANPFUEL_TEST_DATABASE_URL):

```sh
go test -race ./internal/modules/orgaccess/... -count=1
go test -race -tags=integration ./internal/modules/orgaccess/... ./internal/modules/membership/... ./internal/modules/organization/... ./internal/modules/account/... ./internal/modules/payments/... -count=1 -coverprofile=/tmp/enterprise-auth002-final-coverage.out
go test -race -tags=integration ./internal/modules/orgaccess/adapters -run 'TestConcurrentAccountExit|TestRoleGuards' -count=1
go test -race -tags=integration ./internal/modules/orgaccess/... -count=1 -coverprofile=/tmp/enterprise-auth002-postreview-coverage.out
go vet ./internal/modules/orgaccess/... ./internal/modules/membership/... ./internal/modules/organization/...
GOTOOLCHAIN=go1.27.1 XDG_CACHE_HOME=/tmp/enterprise-staticcheck staticcheck ./internal/modules/orgaccess/... ./internal/modules/membership/... ./internal/modules/organization/...
go build ./cmd/api ./cmd/worker ./cmd/ops
sqlc vet
sqlc generate
```

All pass. New domain coverage 99.5%; adapter coverage 85.0% in the complete
post-review run (84.3% before review), with critical guards/failures/concurrency tested above.
Organization and membership adapters retain 92.9% and 93.6%. Counts are
coverage evidence, not production acceptance. Added race tests after that
run pass separately. No Android behavior changed in this task.

Strict Go format, plan verifier and its three regressions, added Markdown
links, diff whitespace and secret review accompany the task commit.
The plan still has 348 tasks, 545 rules, 167 use cases and 145 screens;
consistency does not mean runtime completion. The manifest adds the real
orgaccess adapter suite; no check is removed, skipped or weakened.

## Next checkpoint

AUTH-003 current authorization/context middleware is next on the critical
path; ORG-003 can then compose root/employee/owner/default-unit onboarding
without inventing authority. F1/M1, final remote CI, emulator/device evidence,
aggregate Kotlin coverage and external legal/provider/runtime/staging gates
remain open. Phase publication follows its completed functional checkpoint.
Rollback disables new routes and retains role history; no destructive down
migration, merge, release or production deployment is part of this task.
