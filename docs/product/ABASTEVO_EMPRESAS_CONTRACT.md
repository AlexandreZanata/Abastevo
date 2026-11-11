# Abastevo Empresas product contract

Status: **F0 PRODUCT DECISIONS RECORDED**, 2026-11-10.
[ADR-025](../adr/025-private-abastevo-empresas-organization-boundary.md)
records the maintainer's decision. This contract governs the planned
Abastevo Empresas private fleet context. The
[F0 decision record](../enterprise/evidence/f0-m0-decisions.md) records
the approved panel, Android partition and private receipt scope.

## Business rules

- **B-BR-EMP-01 — Organization ownership.** Each private Empresas record
  belongs to one organization. Legal entities, CNPJs, operating units,
  teams and fleets are managed within that boundary. Ending a membership
  removes access and preserves records owned by the organization.
  [Shared tenancy rules](../enterprise/04-business-rules.md#1-tenancy-and-ownership--br-ten)
- **B-BR-EMP-02 — Current authority.** The server resolves organization
  access from a verified account and an active membership with the
  required permission on the resource. Account or staff status, payment,
  a shared CNPJ root and knowledge of an identifier grant no access.
  [Permission contract](../enterprise/07-permissions.md)
- **B-BR-EMP-03 — Isolated surfaces.** Reads, writes, lists, jobs, searches,
  caches, files, exports and local data carry organization scope. Access
  attempts reveal no foreign record or resource-existence hint. An
  offline operation retains the organization in which it was created.
  [Architecture](../enterprise/09-architecture.md)
- **B-BR-EMP-04 — Product separation.** Public catalog and community
  reads remain tenant-free. Referencing a public station grants no claim
  or write authority. Personal Motoristas and Premium records remain
  account-owned. The individual `ENTERPRISE` tier confers no organization
  privilege. Free participation, source separation and payment neutrality
  continue to apply. [Shared source rules](../enterprise/04-business-rules.md#2-source-separation-and-neutrality--br-src)
- **B-BR-EMP-05 — Company panel.** The company panel is served at
  `/empresas/` on the configured API origin. Its account session is
  separate from admin sessions. Each private request resolves current
  membership and permission. Mutations require exact-origin and CSRF
  checks. [Company web decision](../enterprise/evidence/f0-m0-decisions.md#company-web-surface)
- **B-BR-EMP-06 — Protected Android partition.** Local private data,
  attachments and queued operations are scoped by account, server origin
  and organization, protected by Keystore-backed keys and excluded from
  backup and transfer. An operation keeps its creation scope across retry
  and context changes. Logout locks the partition.
  [Android decision](../enterprise/evidence/f0-m0-decisions.md#android-local-protection)
- **B-BR-EMP-07 — Private fueling receipts.** A receipt belongs to the
  organization that owns the fueling. Member-bound short-lived access
  requires current permission and an audit record. C1 validation uses
  synthetic receipts. Real-receipt intake and retention require an approved
  category-specific policy and existing real-data gates. Human review is
  recorded independently of fiscal status.
  [Receipt decision](../enterprise/evidence/f0-m0-decisions.md#private-receipts)

EMP-PLAT-001 and EMP-PLAT-002 define the shared organization and database
boundaries.

- **B-BR-EMP-08 — Explicit organization scope.** A private organization
  repository call takes an explicit, nonzero OrgScope. Its OrgID is
  a validated non-nil UUID, stored in lowercase dashed form. Parsing accepts
  either hexadecimal letter case and rejects malformed, padded and nil
  identifiers. The application resolves the selected organization under
  current actor authority before constructing scope. Repository adapters
  reject a zero scope before querying and apply it to each private reference.
  [Organization boundary](../adr/025-private-abastevo-empresas-organization-boundary.md)

- **B-BR-EMP-09 — Transaction-local private database scope.** Organization
  adapters run work in one transaction with the validated scope installed
  through a parameterized transaction-local setting. Private tables use
  composite organization keys and foreign references, explicit scoped
  queries, and forced row-level policies for reads and writes. Missing scope
  exposes zero rows and forbids writes. Runtime logins own no RLS tables
  and cannot assume a superuser, BYPASSRLS or role-administration role.
  Commit, rollback, failure and cancellation release the transaction scope.

- **B-BR-EMP-10 — Atomic organization audit.** The organization audit runner
  reserves one event per organization and operation before executing the
  business callback in the same scoped transaction. Callback, audit or
  commit failure rolls back the change and event. A replay with the same
  server-computed request digest and metadata skips the callback. Reusing
  the operation for different input is refused. Events contain validated
  actor, optional delegation and target UUID references, server time, and
  registered action and reason codes. Human explanations remain in the
  referenced business record. Product flows cannot update, delete or
  truncate the trail. Reads require current organization-wide
  `audit.event.read` authority or an approved live support session, resolved
  through the authorization port on every read.

- **B-BR-EMP-11 — Organization HTTP boundary.** Organization POST routes
  validate one bounded JSON object before authentication, preserve the exact
  bytes for device proof, and resolve current membership and authority for
  the path organization on every request. Android requires the live account
  session and an owner-bound signed device proof. Company web uses its
  dedicated session verifier with exact origin and CSRF checks. Partial
  credentials or signed headers cannot fall back to web authentication.
  Foreign or absent membership has the same NOT_FOUND envelope. Private
  success and failure responses use no-store and omit ETags. Commands carry
  schema_version 1, operation_id and a positive revision when required.
  Registered routes declare permission, write/fresh/revision requirements
  and successful response status in their OpenAPI operation.

- **B-BR-EMP-12 — Organization page binding.** Pages default to 20 records
  and cap at 100. HMAC cursors bind the canonical server origin, organization,
  verified account and membership, route and canonical validated filters.
  A cursor expires at its 15-minute deadline. Invalid, foreign, changed or
  expired cursors return the same safe malformed-request error. The current
  authority check precedes page access. Route and contract drift fails tests.

## Use cases

- **BUC-EMP-01 — Act in an organization.** A verified member selects one
  organization. For each operation the server checks current membership
  and the permission on that resource. A foreign identifier yields no
  foreign data or existence hint. Switching the selected organization
  changes the context of future operations only.
- **BUC-EMP-02 — End membership.** An authorized member ends an employee's
  membership. Subsequent requests and access through replay or queued
  work recheck authority. The organization keeps its records and the
  employee retains access to their personal account according to its own
  contract.
- **BUC-EMP-03 — Attach and review a private receipt.** An authorized
  member attaches a synthetic receipt to a fueling in the same organization
  for C1 validation. A reviewer with the required permission opens it
  through the audited capability. Foreign, expired or revoked access is
  refused. The review state records the reviewer's decision.

- **BUC-EMP-04 — Pass resolved scope.** The application verifies the
  request's current account, membership, roles and capability, selects the
  permitted organization, and constructs a scope for the repository call.
  A missing scope is rejected. A later request selecting another
  organization creates a separate value and leaves the earlier value intact.

- **BUC-EMP-05 — Access private rows in a scoped transaction.** Given an
  authorized organization and an ordinary runtime login, the adapter opens
  a scoped transaction and reads or writes only that organization's rows.
  A foreign reference is refused. Concurrent transactions keep their own
  scope. After commit or failure, the reused connection exposes zero private
  rows until another scoped transaction begins. A privileged login is refused.

- **BUC-EMP-06 — Commit and inspect an audited change.** A module command
  verifies current write authority and supplies actor attribution, a digest
  of the complete canonical command, and registered audit metadata. The
  runner commits one event with the business change. A retry returns replay
  without repeating the callback. Failure leaves both absent. An authorized
  reader lists only its organization's events with bounded keyset pagination.
  A denied or revoked read stops before storage access.

- **BUC-EMP-07 — Enter an organization API operation.** The shared route
  entry validates the module DTO, authenticates one channel and verifies
  current authority for the selected organization and declared permission.
  It passes resolved scope and actor to the module callback. Invalid proof,
  foreign scope or malformed input stops before that callback. A paginated
  read opens a cursor only under the same resolved actor, route and filters.

Implementation requires the [F0 decision gates](../enterprise/16-decisions.md#5-what-is-actually-blocking).
Real-data use requires separate legal review and the existing production
gates. Tracking, Postos, billing and Data each have separate scope decisions.
