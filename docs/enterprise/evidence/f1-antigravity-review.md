# Antigravity source review — AUTH-005 and ORG-010

Date: 2026-11-12. Status: LOCAL_DONE / INTEGRATION_PENDING.
Published source reviewed: `845d1d94` (AUTH-005) and `ca9df3cd`
(ORG-010), following `8a48c0a3` (ORG-009) in PR #203, dev -> main.
Owned worktree: `.worktrees/enterprise-suite-execution`, local branch
`codex/enterprise-org010`; fixes publish as new commits to maintained `dev`.
No merge, deployment or next task is part of this review.

## Reproduced defects and corrections

1. Role persistence trusted arbitrary account/member selectors and did not
   retain the company session's real source proof, current roles or capability
   through commit. Reads/replays shared the gap. The transaction now checks
   account -> proof -> capability -> root -> current member/roles, rechecking
   freshness after waits. A source-owned authority port connects modules; no
   generated SQL from another module is imported. Participant account locks
   are sorted before the root to avoid inverse-order deadlocks.
2. ADMIN could end OWNER and grant templates exceeding its actual permissions;
   blanket self-modification denial also rejected permitted OWNER reductions.
   Literal permission union containment now compares scope, coverage and
   restrictions without a wildcard. Only safe OWNER self-reductions are
   admitted. AUTH-005 manages ORG/SELF; AUTH-006 retains finer-scope work.
3. Role command hashes omitted scope target and used lossy time encoding;
   replay read the mutable assignment. Forward migration186 adds immutable
   operation receipts. Role hashes include the scope target and finite canonical
   instants; a retry returns its original result after later END/CHANGE.
   The private receipt also retains the actual command explanation, including
   an END reason distinct from the original immutable grant reason.
4. Ownership accept/cancel bypassed the normal guarded route and write admission;
   the full production-router contract test failed. Every route now uses the
   company cookie, CSRF/origin checks, AUTH lease and source policy. Propose uses
   `org.ownership.transfer`; read/accept/cancel use explicit catalog-v4
   `org.ownership.respond`. Concrete participants are checked transactionally
   before private state, receipts or mutations. SELF response facts only admit
   an attempt and never attest that a transfer belongs to the caller.
5. Ownership acceptance trusted stale initiator ownership; terminal requests
   could disclose another participant's transfer or become a replay with a new
   operation ID. Current personal source membership/employee/account/OWNER and
   both participants are rechecked. Forward migration187 adds immutable
   account-attributed audit-linked receipts. Altered operations fail; retained
   cancel replay is independent of the next pending proposal.
6. Voluntary step-down failed when the initiator already had ADMIN and could
   preserve a future OWNER period. Handover grants target OWNER first, ends
   remaining current/future initiator OWNER grants, and reuses a current ADMIN
   assignment. If ADMIN is scheduled for later, a bridge ends at its start without
   modifying that plan. No temporary ownerless state is introduced.
7. Reduction triggers lacked ordinary-runtime grants and only consumed target
   membership changes. They now consume either participant's suspension/end
   and organization archive/closure, retaining real source cause/revision.
   Projection/receipt tables force RLS and retain immutable history. Existing
   rows are not backfilled with invented actors, commands or delivered notices.
8. Expiration ignored SQL errors and could roll back its own update. Database
   time fixes new validity at 168 hours; a participant read materializes actual
   expiry and commits it. Audit/receipt/notice failures return no success or
   private provisional value and roll back the complete handover.
9. OpenAPI had nonexistent role names, a closed-allOf acknowledgement that could
   validate no response, missing ownership wire fields and unguarded routes.
   Strict schemas now describe actual registered operations and timestamps.
   Tests validate actual composed HTTP responses against the contract as well
   as malformed requests and guard drift. Pre-existing paths/schemas before
   Antigravity retain their meaning; EnterpriseError gains the runtime-emitted
   TRANSFER_PENDING code as a compatible enum extension.
10. Original evidence mislabeled GET role reads, nonexistent permission aliases,
    cookie name, BR-ORG-035 and broad validation/readiness. Those original files
    remain explicitly historical; this source review supersedes their claims.
    Actual role reads are POST, using `auth.role.assign`; the company cookie is
    `__Host-empresas-session`. BR-ORG-035 exceptional recovery is not implemented
    by normal ownership transfer. B-BR-EMP-71..73, BUC-EMP-36/37 and EDR-048 record
    the corrected source contract and decisions.

## Literal catalog decisions and retained obligations

Catalog v4 preserves all v3 cells and adds only ownership response, for a total
of 85 keys x 13 fixed roles = 1,105 cells. Station authority requirements remain
in force. An ACTIVE member with no live role receives no implicit permission;
a normal authorized grant is needed before response. A nominated DRIVER SELF
can inspect/accept with current proof; unrelated members cannot read the state.

Literal containment has intentional visible consequences: OWNER/ADMIN have
SELF-only `maint.request.create` and `doc.document.attach`; some target templates
have ORG coverage. A standalone OWNER therefore cannot grant FINANCE,
FLEET_MANAGER or SUPERVISOR at ORG coverage unless its actual union covers those
permissions. Their SELF-narrowed variants may fit. ADMIN cannot grant FINANCE
without the missing financial permissions; a real ADMIN+FINANCE union can.
This review preserves the source matrix rather than silently expanding it.

Receipts start with the corrected code. Old accepted operations that have no
immutable receipt are refused on retry rather than reconstructed from mutable
state (normally OPERATION_REUSED). No original acknowledgement or historical
actor is fabricated. Deployment of the original commits has not been inferred
from planning or Git state.

Notices are `PENDING_NOTIFICATION_CORE`, not delivered. Production AUTH/IDN
providers/runtime grants, legal receipt gates, staging lifecycle/backup roles,
PLAT-012/GOV-014, Android coverage/device acceptance and the remaining enterprise
backlog retain their existing owners and gates. This backend review does not
certify deployment, real-data admission, G24 or G09. The Android code is unchanged;
no emulator test is needed for these backend-only corrections.

## Validation

Immediate tests use real PostgreSQL18/PostGIS3.6 disposable databases with an
ordinary non-BYPASSRLS runtime, current source account families/company sessions
and isolated fixtures. No production data or credential is committed.

- Production full-router gate and original ownership lifecycle tests: PASS.
- Adversarial ownership receipts, participant privacy, forged/revoked/stale proof,
  capability reduction, existing ADMIN, stale owner, deferred audit/receipt/notice
  rollback/recovery, twelve-way retries/conflicts: PASS.
- Source OWNER-removal ordering and proof expiry while observed blocked on root
  and capability: PASS.
- Ordinary populated migration185 ->187, injected failure/ledger recovery,
  historical-row preservation, forced-RLS receipts/notices and two-organization
  erasure/last-owner/failure/GUC restoration: PASS.
- Full affected modules with race and real PostGIS: PASS after fixing the
  original root-fixture grants regression. All organization, role access,
  membership persistence, company gateway, composition and cmd/api suites pass.
- Complete OpenAPI package: PASS (71.857s); actual production router is included
  in the cmd/api run. Vacuum: zero errors, existing warnings retained.
- Build of all backend packages, scoped staticcheck, sqlc drift, compatibility,
  plan checker (348 tasks,545 rules,167 use cases,145 screens,0 problems) and
  enterprise SQL/protection harnesses: PASS.
- Domain coverage: orgaccess91.6%, organization90.8%. Android coverage and its
  separate acceptance obligations have not been relabeled.
- Final private command-reason retention and independent ordinary migration186
  failure/recovery/RLS/immutability/deferred-audit tests: PASS. All orgaccess
  suites rerun with race after the last codec change: PASS (30.436s adapters).
- Changed Go formatting, git whitespace and staged gitleaks: PASS (no leaks).

The earlier broad PASS statements are not reused as evidence.

Tested backend tree: `65707289a58f4805d4778d837ddca670cbc4be91`.
Tested contract Git blob: `9ffa2c0b44cb418d4c83fe292d901633572828ef`.
Only documentation changes followed these staged artifact pins. Published
migrations through185 have no diff; new source migrations are186/187.

Completed commands (backend working directory unless shown otherwise):

```sh
# Disposable ANPFUEL_TEST_DATABASE_URL explicitly configured outside Git.
go test -race -tags=integration ./internal/modules/organization/... ./internal/modules/orgaccess/... ./internal/modules/membership/adapters ./internal/platform/orgapi ./internal/platform/enterprisesuite ./cmd/api -count=1
# Final reason-retention change and migration186 test:
go test -race -tags=integration ./internal/modules/orgaccess/... -count=1
go test ./internal/platform/apicontract -count=1
go test ./internal/modules/orgaccess/domain ./internal/modules/organization/domain -cover
go build ./...
go vet -tags=integration ./internal/modules/orgaccess/... ./internal/modules/organization/... ./internal/modules/membership/adapters ./internal/platform/apicontract ./internal/platform/enterprisesuite ./internal/platform/orgapi ./internal/platform/orgtest ./cmd/api
GOTOOLCHAIN=go1.27.1 staticcheck -tags=integration ./internal/modules/orgaccess/... ./internal/modules/organization/... ./internal/modules/membership/adapters ./internal/platform/apicontract ./internal/platform/enterprisesuite ./internal/platform/orgapi ./internal/platform/orgtest ./cmd/api
sqlc diff
# From repository root:
bash scripts/check-compat.sh
python3 docs/enterprise/tools/check_plan.py
python3 scripts/tests/test-enterprise-local-sql.py
python3 scripts/tests/test-enterprise-protection.py
git diff --check
git diff --cached --binary | gitleaks stdin --redact --no-banner
```

Go1.27.4 ran tests/build/vet with an isolated writable GOCACHE; staticcheck used
compatible Go1.27.1 and a writable XDG cache. The first full affected run exposed
one missing ordinary test-fixture grant and the first static run exposed dead
Antigravity mocks; both were fixed and rerun successfully. Expanded actual HTTP
response validation found TRANSFER_PENDING absent from the error enum; corrected
before final contract acceptance. Root/domain coverage returned above90% through
retained-history/privacy negative cases. No test was weakened or skipped.

Remote required Quick verification belongs to the new published PR head; local
checks do not certify that remote check or authorize merge. Publication reuses
PR203/dev; integration, wiki publication and deployment retain their own gates.
