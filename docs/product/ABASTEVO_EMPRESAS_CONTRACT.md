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

- **B-BR-EMP-13 — Capability activation never grants authority.** The global
  Empresas surface switch defaults off and hides organization routes. A
  source-registered capability is off when its organization row is absent.
  Current actor permission is required on every entry, independently of a
  flag, account tier or payment. Per-organization disable refuses new online
  writes with ENTITLEMENT_REQUIRED and preserves authorized reads and exports.
  Delayed offline commands retain their data for the owning sync review path;
  this gate neither accepts nor discards their payloads.

- **B-BR-EMP-14 — Audited, serialized capability changes.** An authorized
  operator enables or disables one registered organization capability with
  an operation ID and expected revision (zero for an absent row). The server
  hashes the complete canonical change and commits the state and bounded
  audit event together. A matching replay does not repeat a change; changed
  input under that operation and stale revisions fail. Accepted writes hold
  a shared capability-row lock through their business transaction, so a
  disable waits for earlier writes and blocks subsequent ones. A rollback
  deletes no business record, capability row, membership or role. Operator
  provisioning remains available while the product surface is globally off;
  it has a separate current-authority check and exposes no new HTTP route.

- **B-BR-EMP-15 — Private original identity and bounded intake.** Synthetic
  document transfers resolve current same-organization member authority and
  the server-owned declaration before reading bytes. Original identity is
  organization, document, revision, content digest, detected type and length.
  The server derives `org/<org_id>/doc/<document_id>/<sha256>`, bounds the
  snapshot and requires a configured content inspector. A declaration is
  not inspection. Unconfigured formats are refused. A conditional private
  write cannot replace an original; retries verify the stored snapshot.
  Default bounds are image 5 MiB, PDF 10 MiB and XML 1 MiB. The JPEG inspector
  is reused for synthetic C1; EMP-DOC-014 owns further format inspectors and
  safety-job state. This foundation activates no real-receipt intake.

- **B-BR-EMP-16 — Member-bound audited content access.** An API access token
  lasts at most five minutes and binds server origin, organization, verified
  account/member and exact document identity. It conveys no membership or
  permission. Issuance and every redemption resolve current authority and
  verified document state. Expiry, changed identity or revoked authority
  stops delivery; expiry and authority are rechecked after storage/audit.
  Auditing fails closed before any content response. Tokens and storage
  URLs never enter paths, logs, errors or audit metadata. Clients receive
  private API content with no-store, no ETag, nosniff and a fixed attachment
  filename. Original bytes are preserved; failed transfers claim no ready
  document state. Document persistence and lifecycle remain in EMP-DOC-002.

- **B-BR-EMP-17 — Independent company web session.** Fresh live account
  access proof issues a new account-owned company session, not an admin or
  organization privilege. The host-only `__Host-empresas-session` cookie is
  Secure, HttpOnly, SameSite=Strict and Path=/. Company tokens, verifier
  namespace, table and middleware are separate from admin. Every company
  POST checks one exact configured Origin/Host; authenticated requests also
  require the dedicated CSRF header. Browser identifiers never select an
  account or authorize an organization.

- **B-BR-EMP-18 — Current, bounded company authority.** Company sessions use
  the existing 30-minute idle and eight-hour absolute session precedent,
  bounded by their account family's lifetime. Issuance and sensitive actions
  require account proof issued less than 15 minutes ago. Cookie activity does
  not refresh authentication or extend absolute expiry. Only token lookup
  digests and separately salted token/CSRF verifiers persist. Live family and
  active-account checks apply at issuance and on every cookie request.
  Refresh/revocation races cannot turn stale account proof into a new session.
  Invalid CSRF/freshness does not touch idle time. Re-login replaces the
  presented same-account company session atomically; logout revokes only that session.
  Existing ten-session account admission precedent bounds this separate
  plane; expired/revoked rows have bounded cleanup. Admin and personal account
  logout stay independent, while account-wide revocation invalidates company
  access. Private responses are no-store without ETags.

- **B-BR-EMP-19 — Guard the company web shell.** The separate static
  TypeScript panel lives under /empresas/ and uses the existing account-key
  login to obtain fresh proof for the company cookie. Account proof and
  company CSRF stay in memory; reload and restored pages require login.
  Private POSTs use same-origin credentials, no-store, no redirects and
  bounded requests. Missing, expired, changed-account or refused sessions
  clear private UI state. Logout clears local state even if the server is
  unreachable and reports an unconfirmed server logout honestly.

- **B-BR-EMP-20 — Switch only through current organization authority.**
  A path or browser selection never grants a membership. The shell lists
  only the current account's server-resolved memberships and confirms
  current authority before rendering a selected organization. Switching,
  retrying or logging out immediately clears prior organization content
  and cancels in-flight work; late responses cannot restore another scope.
  Missing membership integration is unavailable, never an invented company
  or an authoritative empty result. Loading, empty, denied, unavailable and
  authenticated states are explicit and usable by keyboard and assistive
  technology. Static shell availability does not activate business APIs.

- **B-BR-EMP-21 — Exact organization value checks.** Organization helpers
  reuse financial-v1 integer centavos, milli-litres/milli-cubic-metres and
  milli-BRL, the existing bounds and canonical L/M3 units. Canonical
  unsigned integer text refuses signs, spaces, exponent/fraction notation,
  non-ASCII digits and overflow. Missing values remain unknown. For known
  quantity/price, expected centavos use half-up on their product divided
  by 10 000; paid plus discount differs by at most one centavo without a
  warning. Larger differences yield ARITHMETIC_MISMATCH without replacing
  or rejecting the paid fact. Missing price yields an explicitly derived
  exact rational milli-BRL price from paid/quantity, not a recorded price.
  Original source-price text is retained unchanged beside the supplied
  milli-BRL value; this helper does not parse or quantize that source text.
  Missing facts do not erase independently calculable expected/derived
  values; difference/warning requires both gross and expected to be known.

- **B-BR-EMP-22 — Allocation follows recorded line order.** Allocation uses
  exact largest remainders: floor each weighted share, then award residual
  centavos by descending remainder and ascending explicit line order.
  Source and weights are nonnegative; line orders are unique nonnegative
  integers. Missing source/weight or zero total weight stays unknown, not
  a fabricated zero allocation. Known zero source with positive basis
  allocates known zeros. Products and weight sums use exact widened
  intermediates. The existing 1 000-item processing budget applies.
  Output is canonical by line order, sums exactly to the source and does
  not mutate inputs. Personal driver allocation keeps its own ID tie rule.

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

- **BUC-EMP-08 — Disable and restore one organization capability.** Given A
  and B with the same capability enabled, an authorized operator disables A.
  New and stale-client writes for A stop before their business callback.
  Authorized reads and exports retain A's records, and B's writes continue.
  Re-enabling A restores writes without changing roles or historical data.
  A failed state/audit transaction changes neither, and a retry is safe.
  Turning the global surface off hides routes; turning it on again restores
  access subject to current authority and the persisted organization flags.

- **BUC-EMP-09 — Transfer and inspect a synthetic private original.** Given a
  server-resolved synthetic declaration and current member permission, the
  API bounds one snapshot, checks its hash and detected type, conditionally
  stores it under the derived organization key and confirms the same bytes.
  A permitted member requests short-lived access and redeems it through the
  API under the same current identity. Access is audited before delivery.
  Foreign member/organization, expired or changed access, unsafe/unsupported
  bytes, unavailable storage or audit failure returns no content. Retries
  cannot overwrite a stored original or turn a partial upload into ready.

- **BUC-EMP-10 — Use the company web session.** Given fresh current account
  proof and the exact company origin, the server mints a new company cookie
  and CSRF value without storing either plaintext. The panel reads session
  state with that cookie and header. Organization routes separately resolve
  current membership/permission; sensitive operations additionally check the
  fixed authentication time. Wrong/ambiguous origin, cookie, CSRF, expired
  timing, account suspension or family revocation fails closed. Company
  logout/expiry does not affect an admin or personal account session.

- **BUC-EMP-11 — Enter and switch the company panel.** A person enters
  /empresas/, signs in with their existing account key and receives the
  independent company session. The shell verifies it before requesting
  current memberships. Selecting an available company confirms current
  membership again before displaying that scope. A foreign deep link,
  expired session, revoked membership or late previous response displays
  no private company content. A missing provider shows unavailability;
  an authoritative empty list offers an honest empty state.

- **BUC-EMP-12 — Reconcile and allocate exact amounts.** Given a recorded
  paid amount, quantity, optional unit price and discount, the helper
  returns a consistency check or an explicit unknown reason and preserves
  paid/source precision. A caller allocates the same recorded amount across
  ordered weighted lines. Fractional ties follow line order. Go and Kotlin
  execute the same synthetic vectors for rounding, discount, one-cent
  tolerance, unknowns, invalid values, large intermediates and exact sums.
  This foundation submits no fueling, document, payment or finance workflow.

Implementation requires the [F0 decision gates](../enterprise/16-decisions.md#5-what-is-actually-blocking).
Real-data use requires separate legal review and the existing production
gates. Tracking, Postos, billing and Data each have separate scope decisions.
