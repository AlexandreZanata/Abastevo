# F1 EMP-ORG-002 organization root persistence

Status: LOCAL_DONE
Validation: PASS

2026-11-11. Scoped local database acceptance, INTEGRATION_PENDING through
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202). F1/M1 remain open.

## Entry and bounded contract

The owner requested reconciliation of Claude's review, EMP-ORG-002 and
publication to the existing PR. Entry head 7af7d2e5 already contains the
second-review corrections; main ef96cd13 is its ancestor. The clean isolated
codex/enterprise-f0-f1 worktree follows remote dev while local dev remains
occupied by unrelated work. No other checkout is changed.

Read current progress/fast card, selected ORG-002, relevant organization,
architecture, scope, SQL/migration and audit contracts. B-BR-EMP-38/39 and
BUC-EMP-20 were recorded before behavior. The initial real-PostGIS RED case
proved the root table absent; the new migration makes it pass. No domain
rules, dependencies, providers or Android behavior are added.

## Implemented persistence

Migration 000169 adds organizations with one permanent nonzero UUID, forced
read/write RLS and no name uniqueness or name discovery. SQLC queries also
predicate every read/write on resolved org_id. Public/personal tables are
untouched. No activation, runtime role/grants or capability is seeded.

organization/application supplies Repository and TransactionRepository;
organization/adapters supplies pgx/sqlc storage. Create accepts only a valid
PENDING_SETUP snapshot at revision one. Save compares the expected revision
and advances exactly once. Matching retries return unchanged; changed
intent/stale revision conflicts. An incomplete pending profile is stored
without invented defaults. Fields are mapped explicitly because domain
JSON, slog and formatting intentionally redact private values.

Convenience methods use WithOrgTx and report no successful result after
commit failure, rollback or cancellation. Explicit-Tx methods verify both
the ordinary login and the exact installed transaction-local scope, without
starting/committing the owning transaction. The existing role guard is
shared unchanged. These ports accept resolved authority; an identifier or
display name supplies no membership proof.

SQL checks constrain snapshot shape and canonical kinds. Triggers enforce
initial pending/revision one, stable identity, revision+1, legal state edges
and terminal CLOSED; DELETE and TRUNCATE are refused. Domain validation on
rehydration also rejects corrupt/unknown stored metadata. Errors are bounded
and never expose private database causes.

## Populated upgrade and recovery

Existing audit/capability IDs become identity-only anchors with all metadata
NULL and has_snapshot=false. They are not fabricated PENDING_SETUP/ACTIVE
organizations, owners, terms or units. Get reports metadata-pending; normal
Create/Save cannot adopt them. Any future adoption needs a separately
audited factual reconciliation before activation.

Inside the atomic migration the table-owning migrator temporarily removes
FORCE from the two legacy sources under exclusive table locks, copies IDs,
restores FORCE, and validates all four root FKs (audit, capabilities, event
intents, event receipts). No concurrent connection observes relaxed RLS.
The FKs are initially deferred: the existing audit runner reserves its
event before invoking the new-root callback. Real composition proves this
order commits atomically and an orphan fails at commit with SQLSTATE 23503.
The executable private-table template now references this root; shared
synthetic fixtures seed identity-only roots only when the schema exists.

A populated chain through 000168 includes audit, enabled flag/revision,
event intent/receipt and a public catalog row. A non-superuser, non-bypass,
table-owning migration role applies 000169. Injected division-by-zero after
the migration rolls back root/functions/FKs/ledger and restores all FORCE
flags; retry succeeds once, then no-ops. Private payloads/IDs and public
catalog are unchanged. migrate.Check accepts both the previous and current
binary chains; schema_min_supported remains 000132. Previously invalid
orphan foundation writes are intentionally refused by the new FKs.

Operational recovery keeps the additive schema and valid identities; disable
the new entry point and return to the compatible image. Do not drop roots,
weaken policies, edit prior applied migrations or delete audit/history.

## Local validation

Disposable PostgreSQL 18/PostGIS 3.6 uses the repository-pinned image digest,
loopback-only port, tmpfs and synthetic credentials. Runtime tests use a
separate ordinary login with only the tested DML grants; a privileged pool
is explicitly refused. This is fixture evidence, not production grants.
Go module toolchain 1.27.4, sqlc 1.31.1; staticcheck v0.8.1 runs with its
Go 1.27.1 toolchain. No dependencies changed.

Commands from backend/ (ANPFUEL_TEST_DATABASE_URL points only to that fixture):

```sh
go test -race -tags=integration ./internal/modules/organization/... -coverprofile=/tmp/enterprise-org002-coverage.out -count=1 -timeout=180s
go test -race -tags=integration ./internal/modules/kernel ./internal/modules/orgtime/... ./internal/modules/organization/... ./internal/modules/orgaudit/... ./internal/modules/orgcapability/... ./internal/modules/orgevent/... ./internal/platform/database ./internal/platform/migrate -count=1 -timeout=240s
go vet -tags=integration ./internal/modules/organization/... ./internal/platform/database ./internal/platform/orgtest
staticcheck -tags=integration ./internal/modules/organization/... ./internal/platform/database ./internal/platform/orgtest
go build ./cmd/api ./cmd/worker ./cmd/ops
sqlc vet
sqlc generate
```

All pass, without skipped suites. Adapter statement coverage is 92.9%; the
unchanged domain remains 100%. Nine adapter test entry points cover:

- Missing/foreign scope, absent transaction, unsafe/closed pools, wrong or
  missing transaction GUC, raw foreign reads/updates/inserts, no pooled GUC
  leakage, same-name roots and legacy metadata refusal.
- Canonical/incomplete snapshot round trips, exact create/save replay and
  changed-intent conflicts; 12 simultaneous creates and 12 distinct CAS
  writes each have exactly one winner.
- Terminal closure, identity/revision/shape/kind guards, invalid initial
  activation, DELETE/TRUNCATE refusal and validated deferred root FKs.
- Actual audit-runner creation/replay and callback rollback; deferred commit
  failure returning no success/private cause; cancellation after observing
  the real database lock wait, with no saved revision.
- Ordinary-owner populated upgrade, injected migration recovery, old schema
  compatibility, payload preservation and no invented legacy metadata.

The unavailable old disposable endpoint was recreated before acceptance.
The first ordinary-migrator rehearsal caught a fixture URL-space encoding
error; it was corrected, and the injected failure now explicitly requires
SQLSTATE 22012 so connection failures cannot masquerade as rollback proof.

Format, generated-output stability, scoped static/build checks, plan checker
and its regression tests, new/changed document links, git diff --check and
secret-surface scans pass. New application/adapter packages are registered
in the quick manifest. No blanket aggregate/native run is claimed.

## Remaining gates and next work

[Review reconciliation](f1-pr202-review.md#emp-org-002-review-reconciliation-head-7af7d2e5)
retains AUTH/IDN single-family/current-provider decisions, real runtime
login/grants, composed-router gate, vault inventory/sweep/revoked-device
cleanup and native acceptance, EMP-DOC-014, staging lifecycle/backup roles,
PLAT-012/GOV-014, aggregate Kotlin coverage and legal receipt gates.

ORG-003 still requires IDN-003 and AUTH-002. Its atomic root/unit/employee/
owner onboarding is not implemented or activated here. Continue the remaining
authorized F1 identity/access foundations; no later phase, production
certification, VPS rollout, merge or wiki publication is inferred.
