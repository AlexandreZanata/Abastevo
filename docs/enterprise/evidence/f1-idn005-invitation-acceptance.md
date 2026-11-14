# F1 personal invitation acceptance — EMP-IDN-005

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-11-12
Source parent: `ed603f341242b4e9c90a57db90b0e603d4538295` (published dev).
Owned worktree: `.worktrees/enterprise-suite-execution`, branch
`codex/enterprise-idn005`; publish to maintained dev in cumulative PR #203.
Task: [#205](https://github.com/AlexandreZanata/Abastevo/issues/205), open until
protected integration. B-BR-EMP-76/77, BUC-EMP-39 and EDR-050 preceded behavior.
No occupied checkout was overwritten.

## Scope and behavior

Personal-account bootstrap uses the existing fresh company cookie and actual
account/session-family proof, same-origin/CSRF checks and strict private JSON.
Three POSTs under `/v1/org-access/invitations/`: `preview`, `accept`, `decline`.
Preview shows organization name, actual legal verification, literal grants,
confirmation requirement and a versioned privacy notice. Its HMAC binds the
accepting account, invitation, root/settings revisions, grants and notice. The
accept command explicitly consents to that current digest. A policy/name/grant
change requires another preview; another account cannot borrow its consent.
No account/proof selectors are accepted in the business DTO.

Resolve only an opaque invitation locator to scope through a forced-RLS index.
No new definer/BYPASSRLS login is introduced. The bootstrap runs as an ordinary
READ COMMITTED role and validates the resolved scope before private work.
Preflight current personal proof, lock affected account UUIDs in sorted order,
then retain proof, capability and root. Recheck proof after observed lock waits
and before mutation. Source-owned organization ports provide account, settings,
root and issuer facts; module adapters import only their own generated SQL.

Keep EDR-049's 256-bit AIN1 secret and domain-separated HMAC verifier. Raw code,
secret, personal credentials and socket address never enter persistence, events
or generic formatting. Constant-time secret comparison precedes private preview
or retry. A known locator with a malformed secret still counts. Five wrong
secrets revoke a pending invitation with TOO_MANY_ATTEMPTS and a pending notice.
Unknown/malformed locators spend the existing identity v1 registration budget
(10/hour), in separate keyed account, browser-session and direct socket-IP
subjects. These counters commit outside a failed business transaction. Client
forwarded addresses or device selectors cannot reset a budget. Browser session
is the available device axis of this cookie API; native enrolment is separate.
Network measurement/trusted-proxy activation remains a production obligation.

Unknown, expired, replaced, revoked, consumed and context-invalid invitations
return the same TOKEN_INVALID404 without organization details. Managed accounts
are refused; ALREADY_MEMBER409 preserves an otherwise usable invitation.
Every acceptance/confirmation checks actual issuer membership, employee,
organization/capability and current literal ORG/SELF grant containment. A decline
consumes the invitation and ends its unused reservation without granting roles.

Acceptance binds the personal account to the INVITED membership. DRIVER and
OPERATIONS_STAFF activate immediately unless the organization's prospective
`policies.require_invitation_confirmation` is true. Any other grant waits for a
fresh authorized manager, with zero roles while waiting. Old five-key settings
retain their immutable bytes/default false. The optional boolean is strict;
false/omission have the same meaning. It does not rewrite past settings or
accepted policy captures.

Three manager POSTs under `/v1/orgs/{org_id}/invitations/{invitation_id}`:
`acceptance/read`, `confirm`, `reject`, using current fresh
idn.invitation.manage and existing company guard/admission. Confirmation checks
the displayed accepted account alias, original issuer and current manager grant
containment, grants roles and activates once. Reject or the exact 168-hour
confirmation deadline ends the INVITED period. Database clock is authoritative;
expiry is enforced on confirmation and materialized by manager read/attempt.
No scheduled expiration worker is claimed.

Contacts, employee/issuer reductions and organization archive/close also end
waiting admissions, retaining actual source ID/type/revision and notice facts.
The membership guard's narrowly checked terminal-admission branch ends only a
never-active, role-free INVITED period without taking an account lock after the
root. It cannot alter account/history fields, requires the actual terminal
acceptance, and preserves existing erasure/last-owner checks. This avoids an
account/root inversion in contact/issuer/expiry producers.

Audit, invitation, binding, role periods, admission, receipts and pending notices
share one transaction. Late/deferred failure returns no provisional result and
allows honest retry. Receipts bind organization, actor, operation and immutable
intent. Retries require current proof and return their original metadata after
later state changes; they never restore access. No provider delivery, account
creation, managed activation, native screen or device enrolment is claimed.
Those retain their owning tasks and runtime gates. F1/M1 remains open.

## Validation record

Domain RED preceded the deterministic confirmation/deadline model. Pure tests
cover supported grants, privileged/optional confirmation, invalid states/source
time, exact seven-day equality and private formatting. Membership domain:95.1%.

Real PostgreSQL18/PostGIS3.6 uses disposable unique databases and an ordinary
non-owner/non-BYPASSRLS runtime. The passing acceptance covers one-use/replay; twelve-way
same/distinct account acceptance; confirm/reject/contact contention; duplicate
live account; malformed/correct wrong-secret lockout; revoked/fresh proof;
observed root/capability waits; changed issuer authority, organization and
managed provenance; consent, decline and immutable terminal outcomes; deferred
audit/receipt/notice rollback and recovery; confirmation expiry rollback;
ordinary populated188→189 failure/recovery, opaque-locator backfill, no invented
acceptance, FORCE RLS, retention and previous-image schema compatibility.
Actual composed HTTP validates response schemas, consent, source aliases,
optional settings, guards, missing/null/unknown intent, quota Retry-After and
non-disclosure. The real production router remains default-off and its route
contract gate recognizes only the three fixed authenticated personal paths.

Validation commands (repository root unless marked):

```sh
# Explicit disposable ANPFUEL_TEST_DATABASE_URL supplied outside Git.
go -C backend test -race -tags=integration ./internal/modules/organization/... ./internal/modules/membership/... ./internal/modules/orgaccess/... ./internal/platform/enterprisesuite ./internal/platform/orgapi ./internal/platform/database ./cmd/api -count=1
# Scoped completion of the organization package after local timeout:
go -C backend test -race -tags=integration ./internal/modules/organization/adapters -count=1 -timeout=20m
go -C backend test -race -tags=integration ./internal/modules/organization/adapters -run '^TestRedemption' -count=1 -timeout=10m
go -C backend test -race -tags=integration ./internal/modules/organization/adapters -run '^TestRedemptionUniformInvalidityLockoutConsentAndAlreadyMember$' -count=1
go -C backend test -race -tags=integration ./internal/modules/organization/adapters -run '^TestRedemptionPrivilegedConfirmationRejectionAndScope$' -count=1
go -C backend test -race -tags=integration ./internal/modules/organization/adapters -run '^TestRedemptionPersonalMembershipsStayIndependentAcrossOrganizations$' -count=1
go -C backend test -race -tags=integration ./internal/modules/organization/adapters -run '^TestRedemptionOrdinaryPopulatedUpgradeRollbackRecovery$' -count=1 -timeout=10m
go -C backend test ./internal/modules/membership/... ./internal/platform/database -count=1
go -C backend test ./internal/modules/membership/domain -cover -count=1
go -C backend test ./internal/platform/apicontract -count=1 -timeout=20m
go -C backend test ./cmd/api ./internal/platform/apicontract -run '^(TestFullProductionRouterEnterpriseContractAndDefaultOff|TestRedemption)' -count=1 -timeout=15m
go -C backend build ./...
go -C backend vet -tags=integration ./internal/modules/membership/... ./internal/modules/organization/... ./internal/platform/enterprisesuite ./internal/platform/orgapi ./internal/platform/apicontract ./internal/platform/database ./internal/platform/orgtest ./cmd/api
# From backend/, writable XDG/GOCACHE and compatible toolchain:
GOTOOLCHAIN=go1.27.1 staticcheck -tags=integration ./internal/modules/membership/... ./internal/modules/organization/... ./internal/platform/enterprisesuite ./internal/platform/orgapi ./internal/platform/apicontract ./internal/platform/database ./internal/platform/orgtest ./cmd/api
sqlc vet
sqlc diff
# Repository root:
bash scripts/check-compat.sh
python3 docs/enterprise/tools/check_plan.py
python3 scripts/tests/test-enterprise-local-sql.py
python3 scripts/tests/test-enterprise-protection.py
git diff --check
```

Go1.27.4 ran build/tests/vet. Initial full organization and OpenAPI executions
hit the default ten-minute process limit while progressing through an existing
fixture migration and base-schema validation, respectively; no assertion failed
in those timed-out runs. The remaining affected packages passed. Repeated only
those two incomplete packages with an explicit twenty-minute process limit;
all cases/assertions stayed enabled. Complete organization passed in731.630s;
complete OpenAPI passed in485.980s. The
final invitation suite passed in150.508s; final duplicate-member refusal,
source existence-oracle, two-organization and migration checks passed after
scoped source/fixture/format adjustments. The first managed-account negative
fixture incorrectly attempted to mutate immutable origin, then lacked INSERT
rights; it now creates managed provenance at birth through an admin fixture
transaction and authenticates a real family/cookie. Runtime account creation
permissions were not expanded. The optional settings HTTP fixture gained the
actual settings producer's command-table grants. No diagnostic logging remains.

Build/vet/staticcheck, generation, compatibility, plan and scoped harnesses pass;
vacuum reports zero errors with existing warnings retained. No remote CI, merge,
native or production acceptance is inferred from local success.

Migration recovery: a failed atomic189 leaves no new tables, altered functions,
columns, relaxed RLS or ledger entry; retry after recovery. Applied migrations
through188 remain byte-identical. Only actual opaque locators are indexed; no
consent, successful attempt, actor or delivery history is backfilled. To stop the
feature, disable the existing entry/capability, retain history and reductions;
never recover tokens or run a destructive down migration.

External providers/runtime grants, notification transport, native/device,
legal receipt, staging and production obligations remain open. Protected
integration and wiki publication are separate.

Tested backend tree: `c88e5a6d4902d5fff80280d6d1d96e371a9d3938`.
Tested contract Git blob: `aa1978be1945e88ce2714eefa9d23d46c5ede75d`.
Remote Quick verification is pending observation against the publication head.
