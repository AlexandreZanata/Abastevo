# F1 EMP-PLAT-005 organization capability switches

Status: LOCAL_DONE
Validation: PASS

Date 2026-11-11. Delivery state INTEGRATION_PENDING; F1/M1 remain open.
The owner requested implementation and delivery into existing
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202), whose head is
`dev` and base is `main`. The reviewed construction head and fetched remote
dev are `68c1d1fa`, above normal main merge `2ad1abaf`. Main is `ef96cd13`.
The occupied local dev worktree has independent changes and is preserved.
Construction continues in the clean isolated `codex/enterprise-f0-f1`
worktree; publication targets the existing PR's dev ref, without a new PR.

## Behavior and source

B-BR-EMP-13, B-BR-EMP-14 and BUC-EMP-08 were recorded before behavior.
The `orgcapability` module has a copied source-owned catalog, an explicit
current-authority application port and module-owned sqlc queries.
An absent capability is disabled. A flag or account tier grants no role.
Read/export authority is still required while a capability is disabled.

`ANPFUEL_ENTERPRISE_ENABLED` defaults off in the existing configuration
loader. `orgapi.Gateway.Enabled` defaults off and returns the safe private
NOT_FOUND envelope before authentication/dispatch. Future composition passes
the same configuration to the gateway and capability service.
Per-organization writes read and lock current PostgreSQL state rather than
trusting cached client decisions. A shared row lock stays until the business
transaction finishes. A disable waits for preceding writes, then later
writes stop before business work with ENTITLEMENT_REQUIRED.

Operator changes validate current Manage authority even on replay and while
the product surface is globally off. Each change uses an operation ID,
expected revision and server-computed digest covering the canonical command.
The existing organization audit runner commits the flag and event together.
Registered actions distinguish enable/disable; metadata contains references,
server time and the registered operator reason. Changed operation input or
actor conflicts; stale revisions do not leave another event.

Migration `000166_org_capabilities.sql` follows the review-renumbered audit
migration `000165`. It adds a composite organization/capability key, forced
read/write RLS, default-false state, positive revision and database time.
It seeds no activation and retains compatibility floor `000132`.
EMP-ORG-002 owns the later organization-root FK.

`Service.GuardWrite` composes inside an existing scoped audit/business
transaction and rechecks current authority. Feature owners must also check
authority before their audit runner, including a replay that skips its
callback. The composition test proves one transaction for feature audit,
capability lock and mutation; transaction-opening runners are never nested.

Tested blobs: domain `2b78cb86448cdeb797fc5807883309323b6bf7e9`,
application `1a558db62fe44c5c326ab2983ea66b4020f1f866`,
adapter `f8e4b7870a671b4160150fdb2f9337867356661e`,
migration `75adb0dd9c2b8194cd51459c199f6a1c2d118745` and HTTP entry
`a02263066d580ef7b7df2b951018468d124491bd`.
Source and evidence travel in the same task commit.

## Executed checks

Linux amd64, backend Go 1.27.4, pinned sqlc v1.31.1 and staticcheck v0.8.1.
Go commands ran from `backend/` with `GOCACHE=/tmp/enterprise-go-build`.
The unique tmpfs Compose project `enterprise-plat005-20261111` supplied
pinned PostGIS 18/3.6 on ephemeral loopback port 32775. Every SQL test used
its own disposable databases and generated ordinary runtime logins through
`ANPFUEL_TEST_DATABASE_URL`. Nothing targeted staging or production.

- RED reproduced missing domain/application behavior and the absent global
  switch, followed by GREEN.
- `go test -race -tags=integration -coverprofile=/tmp/enterprise-plat005-coverage.out ./internal/modules/orgcapability/... -count=1`
  passed. Domain/application coverage is 100%, adapter 98.8%, total 99.3%.
- PostGIS tests proved absent/off denial, separate A/B state, stale-client
  refusal, disable/re-enable, retained authorized reads/exports, no authority
  from a flag, current operator revocation and independent global rollback.
- Twelve concurrent identical changes produced one state/event and eleven
  replays. Concurrent conflicting revisions produced one winner and one
  conflict. Changed code/state/revision/actor under an existing operation
  was refused. The same operation ID is independent in another organization.
- An observed PostgreSQL row-lock wait proved disable waits for an accepted
  write. The next write was refused after disable committed. Explicit
  predicates and forced RLS refused foreign reads, updates and inserts.
  Reused unscoped connections saw zero capability rows.
- State-write failure, unavailable audit, deferred commit failure, callback
  failure and cancellation rolled back their respective transaction.
  Recovery retried the same command safely. Missing audit/authority, unsafe
  runtime login and unavailable capability reads/locks all failed closed.
- The migration rehearsal upgraded the chain through `000165`, injected
  failure after DDL, verified absent schema/ledger, retried successfully,
  proved idempotent reapplication and previous-chain compatibility, and
  preserved public catalog data. The new table was empty; an explicit
  default row was disabled.
- Scoped `go test -race -tags=integration` passed for orgaudit, database,
  migrate, kernel, orgapi, config, httpapi, httpserver and apicontract.
  Existing proof/session, revocation, contract and private-cache regressions
  remained green. Global-switch HTTP tests toggled off/on/off/on.
- `go vet -tags=integration` and staticcheck with `GOTOOLCHAIN=go1.27.1`
  passed for orgcapability, config and orgapi. The pinned staticcheck compiler
  workaround follows backend/README; runtime builds use Go 1.27.4.
- `sqlc generate`, `sqlc vet` and `sqlc diff` passed; generated queries are
  committed. `go build ./cmd/api ./cmd/worker ./cmd/ops` passed.
- The actual previous API was built from `git archive 68c1d1fa backend`
  with `-buildvcs=false`. SHA-256:
  `24157ea75aec98e8d155cd585c6d7082c6d4318df50cec4106b0ec20ccd1d389`.
  After the current migrator applied schema `000166` to another disposable
  database, the previous executable returned 200 `{"status":"ready"}` at
  `/health/ready` and stopped cleanly. Inherited ANPFUEL configuration was
  cleared and external source discovery explicitly disabled.
- The enterprise checker and its regression suite, explicit gofmt,
  changed/new-file whitespace and secret-surface review passed.
  The quick manifest includes three new packages with behavioral tests.

## Remaining scope

This task activates no real organization and adds no business HTTP route,
panel, Android UI, operator CLI, billing or production rollout.
Concrete membership/operator authorization, nonprivileged runtime-login
provisioning and actual module/controller wiring retain AUTH and feature
ownership. Backup/restore roles still need the separately recorded forced-RLS
authority. Offline intake/review must retain delayed payloads in its owning
sync task; this gate performs no discard. No new dependency was introduced.

Next bounded task: EMP-PLAT-006. Protected integration, remote checks, wiki
publication and release/deployment acceptance retain their existing gates.
