# F1 manager-mediated invitations — EMP-IDN-004

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-11-12
Source parent: `587fb6eb82234859cd6eccac9a75bf64d0a850ca` (reviewed dev).
Owned worktree: `.worktrees/enterprise-suite-execution`, branch
`codex/enterprise-idn004`; publish to maintained dev in PR #203.
Task: [#204](https://github.com/AlexandreZanata/Abastevo/issues/204), open until
actual protected integration. No other occupied checkout was changed.

B-BR-EMP-74/75, BUC-EMP-38 and EDR-049 preceded implementation. This is the next
unblocked F1 backend task; ORG-011 is F7 and has separate dependencies.

## Delivered behavior

Five real guarded company POST routes under `/v1/orgs/{org_id}/invitations`:
`issue`, `{invitation_id}/resend`, `{invitation_id}/revoke`,
`{invitation_id}/read`, `list`. The normal company cookie, origin/CSRF, current
AUTH request lease and fresh `idn.invitation.manage` authority apply to every
route. Writes use the identity v1 allowance (200/hour per organization,
account and web/device axis) in their own scoped admission class. Reads and
reductions retain authorized suspended/archived/capability-disabled behavior.

The owning membership transaction rechecks actual account/session/family,
account-member binding, current membership/roles, root and write capability,
including freshness after root/capability lock waits. Capture actual server time
and settings validity through the organization-owned settings port; no module
imports another module's generated SQL. ORG/SELF grants use the literal current
permission union and restrictions. No OWNER wildcard or station grant is added.

Issuance creates an unbound `INVITED` membership with no roles, accounts,
credentials or device enrolments. A live reservation requires explicit CAS
resend. Resend validates current grants, supersedes the old pending token and
preserves the reservation while its replacement is pending. Deferred cleanup
observes the whole replacement transaction. A terminal final token ends its
unused unbound reservation with ADMISSION_ENDED; later issue creates a new
membership period, never resurrecting an ended one.

The secret contains 256 random bits with a separate random UUID locator. The
existing configured enterprise secret derives an invitation-only HMAC key with
domain separation; key rotation requires reissue. Only the keyed verifier is
stored. First successful issue/resend returns code, fragment-only link and QR
payload after commit. There is no token in a query string. Raw credentials are
never passed to persistence. Exact operation retry rechecks current authority
and returns its original immutable metadata without delivery. An uncertain/lost
first delivery requires another explicit resend; no encrypted recoverable replay
or fabricated original code exists.

Migration188 adds forced-RLS invitations, metadata-only command receipts and
immutable pending notification facts. It preserves applied migrations/history.
Current heads retain the settings revision and fixed-hour validity. Contact
change (including first nonempty profile completion), employee reduction, issuer suspension/end and organization archive/close
consume pending tokens with actual source type/ID/revision. Existing membership
producers end/reduce before employee state changes, as their source guard
requires; the projection retains the actual first consumed source. No unknown
historical actor or delivered notification is invented.

Manager access materializes actual database-clock expiry and ends unused
reservations. Failure in audit, receipt, notice or deferred cleanup rolls back
all state and returns no provisional metadata/code. Source getters and explicit
codecs redact generic domain/application/DTO formatting and never expose a
verifier in read/list/receipt responses. The scoped cursor binds route, account,
organization and filters.

## Acceptance

- Domain RED: invitation expiry and role-plan tests initially failed to compile
  before the pure model existed. GREEN covers strict expiry equality, invalid
  identity/scope/history/source, supported bounded plans, copies and redaction.
  Membership domain coverage: 95.6%.
- Real PostgreSQL18/PostGIS3.6, disposable unique databases, ordinary runtime
  without table ownership or BYPASSRLS. Issue/retry, old/new membership periods,
  CAS resend/revoke and immutable original acknowledgements pass. Database dumps
  contain neither code, raw base64 secret nor recoverable hex encoding. No role
  or account is provisioned.
- Twelve concurrent identical replacements yield one mutation and 11 replays;
  distinct CAS replacements yield one winner and 11 conflicts. Only one pending
  token and one unbound reservation remain.
- Revoked company cookie/family, stale proof, current grant containment, self
  invitation, foreign employee/invitation and disabled/archived behavior pass.
  Actual observed root/capability waits age a fresh proof past the deadline;
  both are refused without invitation/audit/receipt/notices.
- Contact creation/edit and employee/issuer/organization reductions retain causal history and
  notices while ending unused reservations. Initial contact completion honestly
  records profile revision 1; later edits retain their actual revision. Settings
  change from 7 to 30 days
  affects the new resend only; earlier expiry/settings/receipts stay unchanged.
- Deferred audit/receipt/notice failure returns no provisional result or secret,
  leaves no partial reservation, and permits retry after recovery. Expiry notice
  commit failure leaves PENDING/INVITED intact; a later read durably expires it.
  Elapsed-time fixture explicitly backdates isolated synthetic rows using admin
  privileges; runtime has no clock input or DDL permission.
- Ordinary populated187→188 migration: injected DDL failure leaves no table,
  trigger/function or ledger entry and preserves old data; recovery and repeat
  no-op pass. No backfill. Prior image schema check, owner FORCE RLS and retained
  UPDATE/DELETE/TRUNCATE refusal pass. Invoker projection grants are included for
  the existing parent-table owners; ordinary runtime grants remain a deployment
  obligation, not automatic production provisioning.
- Actual composed HTTP validates every response against OpenAPI: one-time
  delivery, fragment-only link, no delivery on original/terminal retry or status,
  nested strict intent, foreign organization, named EMPLOYEE_HAS_ACCESS for active/other-model access, contact edit, late503 and write429
  with Retry-After/read availability. A genuine DRIVER/SELF account receives403
  on all five manager routes, including known IDs and replay operations.
- Full affected real PostGIS/race suites pass, including original organization,
  role, membership, company gateway and production-router behavior. Complete
  OpenAPI suite passes; new issuer route/schema/drift tests pass. Build, vet,
  staticcheck, generation, compatibility and scoped harnesses pass.

Completed commands (repository root unless noted):

```sh
# Explicit disposable ANPFUEL_TEST_DATABASE_URL configured outside Git.
go -C backend test -race -tags=integration ./internal/modules/organization/... ./internal/modules/membership/... ./internal/modules/orgaccess/... ./internal/platform/enterprisesuite ./internal/platform/orgapi ./cmd/api -count=1
go -C backend test -race -tags=integration ./internal/modules/organization/adapters ./internal/modules/membership/... -run '^TestInvitation' -count=1
go -C backend test -race -tags=integration ./internal/modules/organization/adapters -run '^TestInvitationCaptures' -count=1
go -C backend test ./internal/platform/apicontract -count=1
go -C backend test ./internal/platform/apicontract ./internal/modules/membership/adapters/http -run '^TestInvitation' -count=1
go -C backend test -race -tags=integration ./internal/modules/organization/adapters ./internal/platform/orgapi ./internal/platform/apicontract -run '^TestInvitation' -count=1
go -C backend test ./internal/platform/orgapi -count=1
go -C backend test ./cmd/api -run '^TestFullProductionRouterEnterpriseContractAndDefaultOff$' -count=1
go -C backend test ./internal/modules/membership/domain -cover
go -C backend build ./...
go -C backend vet -tags=integration ./internal/modules/membership/... ./internal/modules/organization/... ./internal/modules/orgaccess/... ./internal/platform/enterprisesuite ./internal/platform/orgtest ./cmd/api
# From backend/, compatible toolchain and writable caches:
GOTOOLCHAIN=go1.27.1 staticcheck -tags=integration ./internal/modules/membership/... ./internal/modules/organization/... ./internal/modules/orgaccess/... ./internal/platform/apicontract ./internal/platform/enterprisesuite ./internal/platform/orgtest ./cmd/api
sqlc diff
# From repository root:
bash scripts/check-compat.sh
python3 docs/enterprise/tools/check_plan.py
python3 scripts/tests/test-enterprise-local-sql.py
python3 scripts/tests/test-enterprise-protection.py
git diff --check
```

Go1.27.4 ran tests/build/vet with a writable GOCACHE; staticcheck used compatible
Go1.27.1 and writable XDG cache. Vacuum reports zero errors with repository
warnings retained. The first real test caught an object-only decoder used on the
stored grants array; the array decoder was corrected without changing strict
request/receipt parsing. Isolated test grants were completed for actual profile
and settings producers; their runtime invariants were preserved. A final source review added invalidation for
first nonempty contact completion; the invitation and affected producer suites
were rerun successfully. No required
check was skipped or weakened.

## Limits and next owner

This task supplies manager lifecycle, not redemption. IDN-005 must implement
uniform TOKEN_INVALID, secret attempts/lockout, preview/consent, current issuer
and grant revalidation, one-use redemption, privileged confirmation and relevant
enrolment. No public token endpoint, provider email, scheduled notification
transport, account creation or native screen is claimed. The API returns a QR
payload; rendering belongs to the panel task. No emulator test is needed for
these backend changes. Remaining providers/runtime grants, legal receipt,
staging and production gates stay open. M1/F1 is not closed by this task.

Recovery: stop issuance with the existing feature/capability controls, retain
history and access reductions, reissue when a code/key is lost; no destructive
down migration. Protected integration, wiki and deployment remain separate.

Tested backend tree: `e2bcc9a10bd5a251c688540b0da5c90a676b9319`.
Tested contract Git blob: `a857299a0c2b7cc8302e37c73c6cf6874e2cfccb`.
The final UC-IDN-02 named EMPLOYEE_HAS_ACCESS refusal passed through the actual
HTTP composition and contract; affected build/vet/staticcheck/sqlc, error tests,
production-router and compatibility checks passed again after that adjustment.
Remote Quick verification is evaluated only against the new published head.
