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

- **B-BR-EMP-23 — Commit required enterprise follow-up atomically.** A
  source-registered event and its consumer jobs commit with the original
  business change and organization audit. The event uses the original
  operation identifier and target, a positive record revision and only
  bounded registered codes/references. Payloads contain no free text,
  coordinates, credentials or document bytes. The canonical command digest
  also binds the event and sorted consumer set. Identical operation replay
  schedules nothing new; altered event/consumer intent is operation reuse.
  The existing job queue, five-attempt budget and one-minute retry remain
  authoritative. Disabled producer configuration enqueues nothing.

- **B-BR-EMP-24 — Fence and deduplicate organization job effects.** Delivery
  is at least once. A registered consumer verifies the stored job/lease and
  immutable intent under the original organization, then commits its
  database-only effect and append-only receipt in one scoped transaction.
  Receipt identity is organization/event/consumer. Acknowledgement loss or
  duplicate execution can repeat delivery, never the committed effect.
  Changed/foreign payload, unknown version/consumer, lost or expired lease,
  callback failure or failed commit applies no effect/receipt. Lease validity
  is rechecked after the effect. No external call runs while these locks
  are held. Dead replay requires an explicit reason through the existing
  queue API; operator authorization/audit entry points retain BACK ownership.

- **B-BR-EMP-25 — Bind Android local ownership explicitly.** Enterprise
  records and pending operations carry a validated immutable account,
  canonical HTTPS server origin and organization. Every persistence query
  uses all three components. Enterprise Room storage is separate from
  personal tables and schemas; it never imports personal records. An
  operation identifier can converge only on identical target/payload in
  its original partition. Reusing it with changed intent is refused.

- **B-BR-EMP-26 — Refuse stale local context.** A bound store captures its
  partition and context generation. Switching context or locking invalidates
  old handles, including when the same partition is selected again. An
  operation from another partition is refused, never retagged. Context
  changes and in-flight local calls serialize. Process restart begins locked;
  stored work is loaded only through an explicitly selected original scope.
  Local selection is not membership or server authority. The production
  persistence port remains unavailable until EMP-PRIV-003 composes and proves
  protected storage, logout/revocation and backup behavior.

- **B-BR-EMP-27 — Bound enterprise telemetry at the existing HTTP boundary.**
  Organization request totals and elapsed seconds use only registered route
  templates, a fixed result vocabulary and allowlisted organization error
  codes. Missing/foreign routes collapse to unmatched. Raw paths, methods
  outside the HTTP vocabulary, identifiers, payloads and credentials never
  become labels. Record one observation per request, including rejected,
  disabled and panicked requests. Reuse the existing private metrics listener
  and registry cardinality cap; no new collector or public endpoint.

- **B-BR-EMP-28 — Keep private context out of logs.** Enterprise transport
  assigns its own request correlation ID. Access records use only the source
  route template, bounded method, actual status and elapsed time. Generic
  logging under a private request context drops free text, arbitrary fields
  and inherited request/job/bound-route metadata; it retains only fixed
  event codes and server correlation. Credentials, cursor, actor and principal
  DTOs redact default formatting and structured logging. Panic/provider
  content never enters telemetry. Authorization/audit evidence remains a
  separate append-only product record.

- **B-BR-EMP-29 — Admit enterprise writes under scoped rate budgets.**
  A source-owned operation class registers explicit organization, account
  and device budgets with a recorded rationale. A write route cannot
  register without a matching admission policy. After current authority
  and freshness succeed, consume all three budgets atomically through the
  existing limiter's write operation. Scope comes from the resolved
  principal, never request identifiers. Quota denial or transaction failure
  rolls back every counter and runs no module work. Admitted requests consume rate budget
  even if later record rules refuse them; this is not billing or a record
  quota. Reads and account-session lifecycle keep their existing policies.

- **B-BR-EMP-30 — Keep rate subjects private and stable.** HMAC subjects
  bind class, organization and axis, adding account and verified device
  where applicable. Organization/account budgets survive device-key
  rotation. Web uses a stable per-account web bucket, not a claimed browser
  identifier or cookie generation. Different organizations/classes remain
  isolated. Reuse fixed windows and expired-window cleanup; limits are
  explicit source configuration, not new global defaults. Missing policy,
  unsafe runtime role, unavailable storage or cancellation fails closed.
  Exhaustion returns RATE_LIMITED with positive rounded-up Retry-After;
  raw subjects, IDs, keys and SQL errors stay out of responses/telemetry.

- **B-BR-EMP-31 — Preserve operation time and reporting provenance.**
  Keep the declared occurred instant, trusted server receipt instant, entry
  IANA zone and declared device offset separately. Normalize instants to
  UTC without changing their value. Snapshot the reporting day and zone
  from the operating unit, or the organization when no unit is supplied;
  an invalid supplied unit zone cannot fall back. Trips use their start
  instant. Historical snapshots are not recomputed after zone changes.
  Periods include their start and exclude their end. The shared parser
  accepts years 0001–9999, seconds 00–59, up to nine fractional digits and
  Z or numeric minute offsets through ±18:00; unknown -00:00 is refused.
  Device offset metadata allows integral seconds through ±18 hours and
  remains independent of entry/reporting zones, including a misconfigured
  device. Invalid dates, zones and out-of-range normalized dates fail.

- **B-BR-EMP-32 — Separate clock observations from authority.** The
  authorization reference is the trusted server received_at, never
  occurred_at or a device clock sample. At sync, compare the device clock
  sample with its server receipt sample, retaining signed seconds and
  nanoseconds. Flag CLOCK_SKEW only when the absolute difference exceeds
  the organization's nonnegative seconds limit (default 300). An old
  operation's occurred_at is not a clock sample. The estimate may include
  transit delay; it neither grants authority nor silently rewrites times.
  Owning sync flows apply the flag to the batch and arrange review; this
  kernel does not accept batches or resolve membership/period locks.

- **B-BR-EMP-33 — Seal the complete local partition.** The protected
  adapter keeps records, pending intent, attachments, derived caches and
  lease metadata in one authenticated encrypted snapshot per account,
  origin and organization. AES-256-GCM uses provider-generated IVs and
  binds version and partition as authenticated data. No plaintext Room
  database, journal or temporary attachment is opened by this adapter.
  Files live under noBackupFilesDir; filenames/key aliases contain only a
  framed scope digest. Replacement is ciphertext-only, atomic and durable;
  failed writes retain the previous snapshot or an idempotently readable
  complete replacement. Local capacity refusal never drops held work.

- **B-BR-EMP-34 — Bound offline access and key custody.** A fresh trusted
  contact supplies the exact partition, session family, server time,
  offline validity (default 30 days) and managed unlock policy. Ordinary
  reads never create replacement keys. Access requires the same current
  identity/session, authenticated ciphertext and an unexpired same-boot
  monotonic lease. Reboot, decreasing monotonic time or wall-clock rollback
  requires fresh contact, preserving encrypted pending work. Monotonic
  expiry, or trusted contact proving expiry, discards the partition key.
  Device time cannot extend validity. Managed keys require device
  credential/strong biometric authorization for the explicitly configured
  duration; local unlock is never a server credential. A changed key policy
  requires an explicit migration rather than a silent downgrade.

- **B-BR-EMP-35 — Fence logout and revocation.** Clearing or changing the
  account/session family invalidates existing enterprise handles before
  releasing the change; same-family token rotation preserves them. Secure
  session/key removal reports failed durable commits, never memory-only
  logout success. AuthFlow attempts both local clears and remote logout
  and returns secure-storage-unavailable when durable cleanup fails.
  Logout retains encrypted unsent work for the same
  account's later trusted contact; another identity cannot open or send it.
  Definitive revocation or expiry first fences access, then persists a
  denial marker and deletes the key, attempting both even if one fails.
  Cleanup failure is reported and never acknowledged as completed erasure.
  A marker or missing key blocks reopening; unavailable authoritative
  contact never authorizes a new lease. Starting an empty replacement after
  erasure requires explicit acknowledgement of lost unsent work, a fresh
  server grant and successful cleanup before lifting denial; it never
  recovers the old key or authorizes another partition. Real persistence stays unavailable
  in the application graph until native proof and current server adapters
  are accepted. A disconnected device cannot receive immediate revocation.

- **B-BR-EMP-36 — Model the organization lifecycle without granting access.**
  A new pure aggregate starts in PENDING_SETUP with an explicit nonzero
  organization scope and revision one. Display name, server-known IANA zone
  and the descriptive DELIVERY/CARRIER/SERVICES/FARM/OTHER tag set never
  create an owner, unit, permission, plan, price or relation to another
  organization. Pending setup may be incomplete in the domain; the stricter
  creation command must atomically persist the first unit/employee/owner.
  Activation requires nonblank name, valid zone, positive active-unit and
  active-owner counts and recorded terms. Missing items use bounded codes.
  Kinds are canonicalized as a set, with no invented default kind.

- **B-BR-EMP-37 — Keep lifecycle decisions scoped, immutable and terminal.**
  Only the documented organization state edges are allowed. Every read or
  change requires the original scope; every change checks the expected
  revision, returns a new value and preserves the original on failure.
  CLOSED has no outgoing transition, including to itself. Closing a pending
  setup requires confirmed absence of operational records; suspension requires
  a registered reason; reactivation requires cause resolution; archiving
  requires a clear blocking-items report. Restore requires confirmed absence
  or cancellation of a closure schedule and either a resolved suspension
  or a current registered open cause. Archived closure requires confirmed
  elapsed closure period. Unknown/absent evidence fails closed. These are
  server-resolved facts, never client permissions. Owning commands still
  enforce actors, fresh authentication, timers, audit and lifecycle effects;
  this domain performs no actual closure or persistence.

- **B-BR-EMP-38 — Persist private ownership roots without inventing facts.**
  Each private organization ID has one nonzero, permanent database root.
  Root rows use forced RLS and explicit scoped queries; display names are
  not unique and support no discovery API. An additive migration attaches
  the existing audit, capability and event ownership references. Existing
  IDs receive identity-only anchors, with no fabricated name, lifecycle,
  owner, unit or consent. Reading such an anchor returns metadata-pending;
  normal create/save cannot silently adopt it. Audited factual reconciliation
  is required before activating any legacy context. Public/personal records
  never create or enter these roots.

- **B-BR-EMP-39 — Commit root snapshots under ordinary scoped transactions.**
  Only a validated PENDING_SETUP/revision-one value can create a snapshot.
  A matching create/save retry converges; changed intent or stale revision
  conflicts. Updates require revision+1 and a documented lifecycle edge;
  closed identities cannot be reused, deleted, truncated or reopened.
  Explicit-transaction operations verify the installed scope and safe
  runtime role, allowing later onboarding/audit work in the same transaction.
  Rollback, failed commit and cancellation report no successful write.
  Scoped storage is not membership authority; real providers, grants and
  atomic root/unit/owner onboarding remain owning-task requirements.

- **B-BR-EMP-40 — Keep private identity concepts independent.**
  The membership domain references existing personal accounts; it creates
  no platform-wide person record, credential, session or permission. Its
  employee identity/lifecycle can exist without a membership or optional
  driver-profile association. Employee descriptive fields belong to the
  register task; licence contents/eligibility belong to the driver task.
  A personal account may be linked independently in several organizations;
  a managed reference is bound to exactly one organization and access model.

- **B-BR-EMP-41 — Preserve scoped identity periods.**
  Employee, membership and device values are immutable, scoped and revisioned.
  Only documented state edges are allowed. Rehire reuses the employee
  identity without reopening an ENDED membership or restoring assignments;
  a merged-away employee cannot be rehired. Activation requires a bound
  matching account, active employee, confirmed admission/confirmation and
  positive role-assignment facts. Suspension/end require their recorded
  reason and ownership/handover checks. Automatic employee-return
  reactivation applies only to an employee-inactivity suspension.
  These are server-resolved facts, not request permission booleans; current
  actor authority, concurrent uniqueness and atomic effects remain owning
  command/persistence obligations. A supplied-set invariant rejects duplicate
  live employee/account links but makes no claim about undiscovered rows.

- **B-BR-EMP-42 — Descriptors never become proof.**
  Credential descriptors contain only account reference and source kind,
  never secrets or a permission decision. Device enrolment associates an
  existing device/key fingerprint with one active membership after proof
  and limit checks; it remains separate from personal cloud devices.
  Revocation records LOST_OR_COMPROMISED or ACCESS_ENDED and is terminal;
  a later enrolment requires a new record. Formatting, slog and JSON redact
  identity values. No pure transition issues tokens, revokes personal
  sessions, erases a vault or changes another organization's history.

- **B-BR-EMP-43 — Persist private membership periods without implied access.**
  Organization-scoped employee identities and membership periods use forced
  RLS, composite references, revision CAS and exact replay. Every non-ENDED
  period reserves its employee and, when bound, account in that organization.
  Period IDs/history are permanent; rehire creates a fresh period. The
  employee identity table contains no descriptive register or login data.
  Only ACTIVE periods can be candidates for authorization, never grants by
  themselves. Account binding checks current account liveness under a lock;
  admission/access-reducing changes consult a database policy hook under the
  organization lock. The hook denies as unavailable until AUTH-002 installs
  actual role/owner policy; request fields cannot enable it.

- **B-BR-EMP-44 — Fence account erasure atomically at the database boundary.**
  The existing account-deletion transaction cannot bypass private access
  protection, even through a direct status update or missing organization
  scope. A trigger owned by the migrator reads only that account's links,
  locks affected organizations in deterministic order, checks every live
  access-reducing policy and atomically ends/unlinks the periods. Each
  link gets its own random tombstone, not a shared person identifier.
  Employee/organization records and original ended-period reasons remain.
  Last-owner refusal or unavailable policy rolls back personal deletion and
  every private change. Ordinary runtime roles cannot use the owner-only
  erasure RLS path or enable it by setting a GUC. Runtime account locks
  precede organization locks to serialize admission with deletion.
  Full erasure notice/rights/retention and role effects remain owning tasks.

- **B-BR-EMP-45 — Freeze the system catalog without creating authority.**
  org-roles-v1 and org-permissions-v1 contain only the canonical private
  organization roles and exact registered keys. A permission with no approved
  matrix cell stays registered but unassigned; no wildcard, management/read
  inheritance, paid tier, station-representation alias or unknown key creates
  a grant. Catalog definitions reject duplicates and unregistered references.
  Returned views are deterministic copies and cannot mutate shared source.

- **B-BR-EMP-46 — Preserve every matrix restriction.**
  Catalog rules distinguish assigned scope from own-record/assigned-asset
  coverage. Admin role assignment excludes OWNER; driver GPS access is own
  trips only; auditor tracks are recorded-only. Fleet and station agreement
  sides remain separate, and station rules need live station authority.
  OWNER receives the matrix's explicit cells, including SELF restrictions,
  without a record-rule or station-authority exemption. Every positive rule
  still requires active membership, a valid scoped assignment, organization/
  plan checks and record rules. Assigning OWNER requires a current OWNER.
  These are static descriptors, not session, grant or permission decisions.

- **B-BR-EMP-47 — Preserve scoped role periods and their history.**
  Fixed-role assignments reference the same organization's membership and
  assigner, use ORG or SELF scope only in F1, and have half-open UTC validity.
  ORG targets that organization; SELF targets that membership. Station roles
  require the later STATION authority scope and cannot be widened to ORG.
  Identical creation retries converge; changed intent and overlapping periods
  for the same membership, role and scope conflict. Ending is revisioned,
  terminal and server-timed; cancellation before start grants no interval.
  History cannot be deleted, rekeyed or rewritten. Only current assignments
  of active, available memberships/accounts are authorization candidates.

- **B-BR-EMP-48 — Protect owners with current persisted facts.**
  Assignment, membership and account access reductions serialize on the
  organization root. The surviving active OWNER/ORG periods must cover the
  remaining time without gaps, including scheduled expiry. Reserved account
  deletions do not count as surviving owners. An admission transaction may
  stage assignments on a bound invited period before activation; these rows
  grant no access while invited. A managed account cannot own an organization.
  Membership end also ends its assignment candidates without deleting history.
  Storage does not replace current actor permission, fresh authentication,
  entitlement, audit or record-policy checks in their owning commands.

- **B-BR-EMP-49 — Resolve authority again for each private request.**
  Verified account/device or company-cookie proof precedes path scope.
  The server loads current organization, membership/account availability and
  effective fixed-role periods without trusting selected client context or
  cached decisions. No matching membership or a foreign resource reveals
  data or existence. A member lacking a registered permission receives only
  FORBIDDEN and its permission key. Unknown policy, missing provider or
  malformed persisted authority fails closed. An ended/revoked assignment
  or membership takes effect on the next request.

- **B-BR-EMP-50 — Bind permission, freshness, capability and record checks.**
  Source-declared route effects distinguish configuration, operational work,
  reads, access reductions, plan regularization and restoration. Pending
  setup refuses operational commands; suspension/archive preserve permitted
  reads and explicitly allowed access-reducing actions; closure denies access.
  ORG/SELF and every catalog restriction apply to server-resolved resource
  facts. Fresh proof, capability/limit and record policy remain independent
  gates, in that order after permission. OWNER bypasses none of them. A
  resolved request context belongs to this request, account, organization,
  permission and target; retaining or supplying an old context cannot extend
  authority. Missing or failed gates never dispatch a business handler.

- **B-BR-EMP-51 — Pilot capabilities do not grant authority.**
  After current proof, membership, permission and scope checks, the pilot
  adapter reads the registered capability's current state only in that
  organization. Global activation defaults off. Off or absent per-organization
  state refuses new business/configuration writes. Existing authorized reads
  and exports remain readable; source-declared access reductions, plan
  regularization and restoration retain the existing safety paths. Every
  path still requires an available provider, a known capability and the
  independent record policy. Account tiers, payment and client flags are
  not inputs and create no membership, assignment or covering scope.

- **B-BR-EMP-52 — Capability rollback preserves data and access history.**
  Disabling/re-enabling one organization's capability affects its next new
  action without changing another organization, roles, records or account
  ownership. Missing/unavailable state providers and corrupt state fail
  closed. The entry check is not a transaction lock: an actual business write
  still rechecks and locks the same capability inside its existing scoped
  business/audit transaction, so disabling cannot race past that boundary.

- **B-BR-EMP-53 — Create one complete pending organization.**
  A fresh, current personal company session supplies the creator; the body
  supplies only the operation, valid organization profile, first-unit name
  and acceptance of the configured terms version/digest. The server chooses
  all aggregate identifiers. One transaction records the pending root, active
  default unit, creator employee, active membership, ORG OWNER period,
  terms/creation receipt and audit. A matching retry returns the original
  acknowledgement; changed intent conflicts. Partial writes and failed
  commits return no successful creation. No real-data/legal approval is
  inferred from synthetic terms or ACTIVE business state.

- **B-BR-EMP-54 — Fence pending ownership and setup at storage boundaries.**
  Existing accounts remain personal unless created with an immutable explicit
  organization-managed binding; no account is converted from membership
  descriptors. Managed accounts cannot create/own an organization. Pending
  OWNER reservations serialize under the account lock and cannot exceed three
  organizations, including scheduled/invited/suspended periods that can become
  effective. Organization locks follow account locks. Activation resolves
  actual unit/owner/terms facts and current actor authority, requires the
  scoped capability transaction guard and records its own idempotent audit
  receipt. Missing terms, invalid zones, stale proof or forged counts cannot
  activate. Last-unit and permanent history safeguards protect the new unit.


- **B-BR-EMP-55 — Declare parties without claiming verification.**
  Under current `org.legal_entity.manage` authority and the `organizations`
  capability, accept a scoped COMPANY declaration from a valid full CNPJ or
  NATURAL_PERSON declaration from a valid CPF for the stated verification/
  agreement purpose. The company stores its CNPJ root and at least one
  explicitly declared establishment. Head-office status is never inferred
  from order digits. New declarations are DECLARED only; they grant no access,
  station representation, agreement eligibility or registry verification.
  No automatic external lookup or real-data legal approval is introduced.

- **B-BR-EMP-56 — Keep legal identifiers private and local.**
  Identifier format/check digits are validated from recorded primary sources.
  A declaration is unique only within its organization; the same identifier
  elsewhere has no effect on its response. CPFs are masked on default views
  and excluded from logs/events/default exports. Only an explicit current
  legal-management read with fresh proof and access audit can reveal one.
  Domain/request formatting and encoding redact names and identifiers.
  A CPF is optional for fleet use and never collected to grant access.

- **B-BR-EMP-57 — Preserve declaration identity and corrections.**
  The identifier, kind and owning organization are immutable. Every
  establishment belongs to a COMPANY with the same CNPJ root; its role is
  HEAD_OFFICE, BRANCH or NOT_INFORMED by explicit declaration. Optional
  municipality references are seven-digit IBGE reference text, not a claim
  that an external registry confirmed the address. A foreign or unknown
  entity/establishment yields the same scoped not-found. A mistaken
  declaration without verification history or references may be removed,
  retaining only bounded audit/acknowledgement metadata; otherwise withdrawal
  revokes it without removing references or verification history.

- **B-BR-EMP-58 — Serialize audited declaration commands.**
  Server-generated identities, legal/establishment rows, revisioned changes
  and command acknowledgement commit with the audit. Same-operation matching
  intent returns the original result; changed intent, stale revisions,
  concurrent same-organization duplicate or failed commit returns no mutation.
  New work rechecks current proof/member/role/capability inside its scoped
  transaction. Source-registered audit/event vocabulary accepts bounded
  lower-case snake_case segments, including the specified legal_entity action;
  unknown actions/targets/consumers still require exact source registration.
  Declarations remain synthetic/default-off until the independent
  legal/runtime/pilot gates are satisfied.

- **B-BR-EMP-59 — Maintain scoped operating units.**
  Current org.structure.manage and the organizations capability permit
  online revisioned unit create, metadata change, archive and restore.
  Persist an explicit IANA time zone, defaulted from the current organization
  when omitted; later organization/unit zone changes never rewrite records
  whose reporting zone/day was already captured. Names and codes are trimmed,
  bounded private text, unique case-insensitively among active units only.
  Archived units retain identity/history and may be renamed before restoration
  to resolve a current uniqueness conflict. They accept no new operations.
  Optional establishment references resolve inside the organization; a newly
  selected link must name an existing declared COMPANY establishment. An
  existing retained link is not rewritten when its declaration is withdrawn.

- **B-BR-EMP-60 — Protect the last and currently used unit.**
  Archive requires another active unit and no active assets/employees, open
  budgets or operations in progress. Return LAST_UNIT or STRUCTURE_IN_USE
  with bounded category counts only, never source records. Root serialization
  covers archive, restore and assignment admission so competing changes cannot
  evade the rule. Counts come from actual source-owned transactional guards;
  an absent/unknown usage source fails closed. No historical reference alone
  prevents archival. SQL and command receipts preserve unit identity, original
  acknowledgements and scoped establishment links; matching retries return the
  original result, changed intent and stale revisions mutate nothing.

- **B-BR-EMP-61 — Keep cost-centre code identity permanent.**
  An optional cost centre belongs to one organization and has a stable ID,
  private trimmed name/code, ACTIVE or ARCHIVED state and revision. Code
  comparison is trimmed and case-insensitive, unique across every current
  and historical centre in that organization. A code correction keeps the
  old code permanently reserved to the same centre; historical code lookup
  resolves that ID and never maps it to another centre. Archived centres and
  their aliases are retained, not deleted. Foreign organizations can use their
  own identical codes independently. Names are descriptive, not unique.

- **B-BR-EMP-62 — Serialize audited cost-centre configuration.**
  Under current org.structure.manage, fresh company proof and organizations
  capability, create/change/archive/restore and acknowledge the original
  operation atomically with audit and permanent code reservations. Stale
  revisions, changed replay intent and racing duplicate code reservations
  leave no partial state. Archive requires zero current assets/employees,
  open budgets or in-progress operations, derived by source-owned scoped
  transactional guards. Unknown/malformed sources fail closed. Root-locked
  admission refuses new assignments to archived centres; historical records
  and export snapshots remain unchanged. Refusals expose only safe code
  field details or category counts, never names or source records.

- **B-BR-EMP-63 — Maintain employees without granting access.**
  A scoped employee profile requires only a bounded display name; optional
  registration number, unverified work email/phone, job title and current home
  unit serve the purposes in identity §3. Profile creation also creates an
  ACTIVE identity, never an account, membership, role, credential or driver
  eligibility. Existing identities may explicitly complete a pending profile.
  Profile revisions are separate from authoritative lifecycle revisions;
  existing last-owner, membership and offboarding guards remain mandatory.
  Only current idn.employee.manage may create/change/complete or review possible
  matches; idn.employee.read governs register reads. Configuration uses fresh
  company proof, organizations capability and atomic private audit/receipt.

- **B-BR-EMP-64 — Review duplicates and retain unit history.**
  A nonempty trimmed case-insensitive registration number is unique in an
  organization across all employee states. A matching normalized display name
  and at least one matching nonempty normalized work contact requires explicit
  review and create-anyway acknowledgement; neither comparison nor contact
  ownership links or merges accounts/people. Authorized candidate discovery is
  scoped and bounded; errors contain no personal data. Root serialization
  covers matching, registration races and home-unit admission. A newly selected
  unit must be active in the same organization. Active employees count as unit
  usage without needing a login. Successful unit archival clears remaining
  inactive/terminated current home links atomically, records the causal unit
  audit/receipt and increments profile revision; restoration never recreates
  those links. Historical command acknowledgements remain unchanged.

- **B-BR-EMP-65 — Keep teams structural and scoped.**
  A team has a stable scoped identity, bounded trimmed private name, optional
  same-organization unit, ACTIVE/ARCHIVED state and revision. Only ACTIVE names
  are unique, trimmed and case-insensitive. Metadata correction while archived
  may resolve a restoration collision; no history is deleted or rewritten.
  A newly selected unit and a restored team's retained unit must be active.
  An empty active team may retain its historical reference when its unit is
  archived, but receives no new employee links until the unit is changed,
  cleared or restored. Team membership grants no account, role, permission,
  delegation or membership access. All configuration uses current fresh
  org.structure.manage, organizations capability and atomic audit/receipt.

- **B-BR-EMP-66 — Retain team-link periods and consume access reductions.**
  Adding an ACTIVE same-organization employee creates a new immutable-identity
  link period; at most one current link exists per employee/team. Pending
  descriptive profiles are explicit, never invented; no login is required.
  Removing ends the current period. Inactivation/termination and membership
  suspension/end close current links atomically with source-cause metadata;
  later reactivation never restores them. Changing current team links increments
  the team revision, so stale commands and altered retries cannot overwrite
  reductions. Current active linked employees block team archive. Unit usage
  counts the distinct union of active home-unit employees and active employees
  in current team links; an empty team or historical link does not count.
  Root locks serialize assignment, structure changes, reductions and archive.
  Unknown source consumers fail closed; counts and errors reveal no names or
  contacts. Account erasure consumes the same reduction through its existing
  protected definer, preserving history and isolation.

- **B-BR-EMP-67 — Capture immutable effective settings.**
  Each organization settings revision records its typed values, server effective
  instant, source, author when known and bounded private reason. Changes use CAS
  on the settings head, current fresh org.settings.manage, organizations capability
  and atomic audit/receipt. Earlier revisions remain readable with current
  org.settings.read. Defaults require review for trips/fuelings/expenses, one
  approval level, KM_PER_L, administrator/owner review fallback, creator contacts,
  fixed seven-day ownership transfer and thirty-day closure. Known source-owned
  intake/skew, delegation and invitation/activation parameters use their frozen
  source defaults/ranges; retention admission remains the privacy owner's legal
  gate. There is no arbitrary unvalidated policy bag or retrospective rewrite.
  Intake captures the settings revision and server receipt together under the
  root lock; declared device time never selects policy. A regressed clock fails
  closed. Changing display units preserves stored base integers and arithmetic.

- **B-BR-EMP-68 — Route responsibilities without granting authority.**
  Four scoped primary/substitute pairs cover review fallback, platform notices,
  data protection and billing; references must be distinct, currently available
  ACTIVE members, never raw names, emails or account credentials. Designation
  grants no role or delegation. The consuming workflow still checks the current
  permission on the actual record. Unassigned responsibilities fall back to
  current administrators/owners; naming a substitute alone never proves review
  authority. Membership suspension/end clears only the affected slots in a new
  immutable system revision, retaining the other designee and causal source
  facts; reactivation restores nothing. Account erasure consumes the same guarded
  path. A durable designation-cleared notice remains pending for the owning
  notification core, never falsely marked delivered. Initial settings are atomic
  with authentic onboarding; legacy roots lacking that receipt stay uninitialized.
  Legacy bootstrap uses actual initialization time/system source rather than invented historical authoring.
  Settings time zone is the versioned writer for the root default. Existing unit
  reporting zones remain captured; new/edited omitted-zone units use the new
  default, and historical reporting snapshots are untouched.

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

- **BUC-EMP-13 — Retry an enterprise follow-up safely.** A verified module
  command commits a scoped change, audit and required consumer jobs. A
  worker validates the leased references, applies its scoped database
  effect and records the receipt. If its acknowledgement is lost, another
  claim sees the receipt and completes without repeating the effect.
  Rollback, foreign scope, changed intent, stale lease or unavailable
  storage returns no successful mutation; bounded retries eventually park
  failed work for explicit operator recovery.

- **BUC-EMP-14 — Resume work in its original Android context.** Select a
  verified account/server/organization context and bind a local store.
  Persist a synthetic record or immutable pending operation in that scope.
  After context change, old handles fail. After process restart, select
  the original scope explicitly to load its work; another account, server
  or organization sees none of it. The foundation dispatches no operation
  and opens no real-data storage before protected-store integration.

- **BUC-EMP-15 — Observe an enterprise request without private data.** An
  operator scrapes the existing private metrics endpoint and sees bounded
  route/result/code counts and request seconds for success and refusal.
  Access/recovery logs correlate through a server-generated trace. Malformed,
  revoked, foreign and panicked requests contribute no private content;
  telemetry does not authorize a request or replace an audit event.

- **BUC-EMP-16 — Retry a rate-limited organization write.** A currently
  authorized client sends a scoped command. Admission checks the registered
  organization/account/device budgets before module work. An exhausted
  budget returns a private 429 and Retry-After without charging other axes.
  Retry with fresh transport proof after the fixed window permits it;
  changing the device cannot reset organization/account usage. Foreign or
  revoked authority never consumes the target organization's budget.

- **BUC-EMP-17 — Attribute a delayed operation without backdating access.**
  Given a declared instant near midnight in a unit different from its
  organization, receipt on a later day preserves both times and attributes
  reporting to the unit's local day. A record without a unit uses the
  organization. Receipt at an access period's end is outside that period,
  even when the device declares an earlier time. A sync sample twelve
  minutes ahead flags CLOCK_SKEW under the default limit, without changing
  the operation. Invalid zone/offset input produces no time snapshot.

- **BUC-EMP-18 — Keep unsent work protected after session loss.** Given
  encrypted work in A, logout locks old handles without deleting that work.
  B sees none of it. After the same account signs in again and the server
  confirms A, the original work can reopen under its original scope. A
  revoked device, expired lease, restored ciphertext without its Keystore
  key, modified envelope or failed cleanup reveals no content. Managed
  access requires local device unlock; the production binding remains
  closed until device and server integration evidence exists.

- **BUC-EMP-19 — Decide an organization lifecycle change.** Given a scoped
  pending aggregate, incomplete activation returns every missing checklist
  item without changing its revision. Complete activation returns ACTIVE.
  Current confirmed conditions allow suspension, archive and restore only
  on the documented edges. A closed aggregate never revives. A foreign
  scope, stale revision, invalid kind/zone or missing lifecycle evidence
  returns no changed value; a display name or kind never replaces proof.

- **BUC-EMP-20 — Store and reload an organization root safely.** Under a
  resolved scope and ordinary login, create/read/update only that root.
  Another root with the same name remains invisible. Identical retries
  converge, competing changed revisions have one winner, and a failed
  composed transaction leaves no root or related write. An upgrade retains
  prior private IDs/content and public data, keeping unknown legacy metadata
  unavailable. No root entry point grants access from a name or identifier.

- **BUC-EMP-21 — Model people and access without provisioning it.**
  Create an employee identity without login or driver data; associate a
  pending membership with an existing account using exactly one access
  model. Apply scoped lifecycle decisions from confirmed server facts.
  Ending a period preserves the employee/account references, and rehire
  requires a new period. Foreign scope, missing facts, stale revisions,
  reopening ended/revoked values or inferring rights from a credential fail.

- **BUC-EMP-22 — Persist access history and protect account deletion.**
  Under resolved scope, store an employee identity and pending period;
  bind, activate, suspend or end it only with current database policy.
  Competing live reservations have one winner; an exact retry converges.
  Deleting the existing account either ends/unlinks its allowed periods in
  every organization with the personal transaction, or refuses everything
  when ownership policy is unavailable or the last owner is protected.
  Ending access never deletes the employee, organization or history.

  The existing account-deletion billing-exit intent checks and reserves this
  protection before provider cancellation. Pending deletion intents fence
  new membership admission and are excluded from surviving-owner policy;
  a replay with a changed billing-exit reason is a conflict, not admission.

- **BUC-EMP-23 — Resolve fixed role descriptors safely.**
  Read an immutable system role's exact permissions and restrictions from
  the versioned source catalog. Unknown roles/keys fail; registered keys
  without a matrix cell return denied coverage. Combining role views keeps
  each rule's role, scope and restrictions; no catalog lookup authorizes an
  account, grants a role or replaces current server policy.

- **BUC-EMP-24 — Persist role periods and refuse ownership gaps.**
  Store an ORG or SELF assignment within one resolved organization; retrying
  the same creation returns the same period. Unsupported/foreign scopes,
  duplicate overlapping periods and changed retries fail. Candidates exclude
  not-yet-started, ended and expired periods or unavailable memberships.
  Concurrent reductions leave continuous owner coverage; refusing the last
  owner rolls back the entire membership, account or assignment transaction.

- **BUC-EMP-25 — Recheck a guarded private request after revocation.**
  Authenticate normal proof, resolve the path organization and current role
  scope, confirm declared freshness, then require capability and record
  policy before dispatch. Repeat the request after ending the role or
  membership: it is refused, including with an old cookie, cached client
  context or retained server decision. A foreign record and an unknown one
  produce the same scoped not-found response and no foreign content.

- **BUC-EMP-26 — Disable and restore a pilot capability without granting access.**
  With current scoped authority, enabled state admits the feature's next
  action. Disable it: new work stops while permitted existing reads and
  safety reductions remain available. Re-enable it: the same membership and
  role are checked again. An enabled flag in B, a paid tier, old client
  state or a missing adapter never grants A's action or widens SELF scope.

- **BUC-EMP-27 — Create and finish synthetic setup atomically.**
  Accept the explicitly synthetic configured terms with a fresh personal
  company session. Creation returns one pending root, default unit and owner
  tuple. Repeat the operation: the same tuple returns. Complete setup under
  current owner/configuration authority and the named pilot capability; the
  server validates stored facts and returns ACTIVE without certifying real
  data. Invalid/missing terms, managed access or a failed transaction changes
  none of the related records.

- **BUC-EMP-28 — Race the pending-organization limit.**
  Concurrent fresh creation/OWNER-reservation commands for one account leave
  at most three pending organizations. A fourth refuses and rolls back its
  entire unit/member/role/audit/receipt group. A matching earlier retry still
  returns its acknowledgement. Another account has its own independent limit.


- **BUC-EMP-29 — Declare and correct a party privately.**
  Declare a synthetic company and explicitly label its first establishment
  without interpreting its order digits; add another matching-root
  establishment. Declare a generated natural-person identifier for the
  stated purpose and read its masked default view. Only current fresh
  legal-management authority can reveal it, with an access audit. An
  organization may independently declare the same identifier. Foreign
  references, malformed identifiers, changed retries or a failed transaction
  expose no foreign information and leave no partial rows. Withdraw an
  unreferenced declaration or retain a revoked referenced declaration.

- **BUC-EMP-30 — Maintain a unit without changing its past.**
  Create a synthetic unit, edit its metadata/time zone and link a scoped
  establishment. Capture reporting days before and after the zone revision;
  the old record stays unchanged. Archive an empty non-last unit and restore
  it after resolving any active name/code conflict. Refuse a foreign link,
  duplicate active name/code, stale revision, last-unit race or current usage;
  return counts without personal/operational content. Exact retry and failed
  commit leave their original acknowledgement or no change, respectively.

- **BUC-EMP-31 — Correct a cost centre while retaining historical meaning.**
  Create a synthetic centre, rename or correct its current code, then resolve
  the previous code to the same ID. Archive/restore without reusing its current
  or historical codes for another centre. Another organization may independently
  use the same code. Refuse foreign references, active usage, racing duplicate
  reservations, stale changes and altered retries. A failed commit leaves no
  centre, alias, audit or acknowledgement; past allocations/exports keep their
  captured identity and code.

- **BUC-EMP-32 — Register and review an employee without login.**
  An authorized manager creates a synthetic employee with only a display name,
  reads and edits its profile, then explicitly completes the creator's pending
  profile without changing membership/identity revisions. Reject a duplicate
  registration, foreign unit/employee, stale revision, revoked proof or changed
  retry. Show scoped likely-match candidates and require explicit create-anyway
  acknowledgement without merging. Competing exact/likely duplicates and unit
  archive/admission serialize; failed audit/deferred commit leaves no identity,
  profile or acknowledgement. Verify that unit restoration does not restore a
  home link cleared by its earlier audited archive.

- **BUC-EMP-33 — Maintain a team without granting access.**
  Create a synthetic team, assign an existing employee without login, remove
  and explicitly add a new period, then archive/restore after current use ends.
  Refuse foreign employee/unit, duplicate ACTIVE name, stale revision, archived
  team/unit admission and racing duplicate links. A real member gains no role
  or new permission by joining. Employee/access reductions close current links
  once, retain their source facts, and invalidate stale team commands; account
  erasure behaves the same with an ordinary protected definer. Actual current
  usage and assignment/archive races protect teams and units without double
  counting. Audit, link, revision and acknowledgement commit or roll back
  together; exact retry returns its original result after later changes.

- **BUC-EMP-34 — Change settings and route a substitute prospectively.**
  Read defaults, capture an intake with revision and base integers, then change
  zone, display and review defaults with a reason. A later server intake captures
  the new revision even for an earlier device declaration; the old record keeps
  its original revision, zone and integers. Refuse stale/altered retries, invalid
  bounds/zone and foreign or unavailable designees. A nominated DRIVER gains no
  review permission. Suspension/end/erasure clears only that member's slots with
  causal history and pending notice; later activation restores none. Concurrent
  settings/receipt and reduction races serialize. Audit, head, root default,
  revision and receipt commit together or leave no provisional result.

Implementation requires the [F0 decision gates](../enterprise/16-decisions.md#5-what-is-actually-blocking).
Real-data use requires separate legal review and the existing production
gates. Tracking, Postos, billing and Data each have separate scope decisions.

Synthetic onboarding source admission decision (EMP-ORG-003): activation uses
its own `org.setup` org/account/web-or-device buckets with the existing identity
v1 write allowance (200 admissions/hour on each axis), not payment limits.
This bounds synthetic validation; it is not measured production capacity.
Measure and review feature budgets and enable the existing quota cleanup before
live enterprise activation. Pre-organization creation retains fresh-cookie
proof and the transactionally enforced three-pending-organization account cap.
