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

- **B-BR-EMP-69 — Platform suspension is scoped and audited.** Only the
  existing ADR-023 administrative credential plane may suspend/reactivate in
  this task, with fresh current proof, a bounded reason, expected root revision
  and one of BILLING/TERMS/SECURITY/LEGAL/OWNER_REQUEST. Company roles and staff
  badges grant no operator power. Root, retained suspension period, owner notice,
  admin audit and original acknowledgement commit together. Archived roots keep
  their state while a cause is opened/resolved. Pending and closed roots cannot
  be suspended; existing unknown historical causes fail closed. Reactivation
  resolves the matching cause without reviving roles, devices or held facts.

- **B-BR-EMP-70 — Preserve field facts without reopening write authority.**
  Suspension blocks new configuration/operational work while current authorized
  reads, exports, plan recovery and access reductions retain their source rules.
  An owning command captures server time/history under the root lock. Finishing
  a server-recorded entity opened before suspension, or a fact declared strictly
  before it and received within seven days (inclusive), requires review. Device
  time never proves permission. Outside grace, a new fact is refused. Historical
  suspension intervals still force review after reactivation; persisted review
  outcomes are not promoted automatically. Future sync/business owners persist
  facts and apply current proof/revocation/permission/window rules independently.

- **B-BR-EMP-71 — Role commands retain current authority and original receipts.**
  AUTH-005 admits only documented ORG/SELF templates, ACTIVE same-organization
  members and active employees. Each read, command and retry binds the real
  company session to the current account/member and rechecks current roles,
  source proof and organization state under retained transaction locks. New
  grants must fit the actor's literal permission union, including coverage and
  restrictions; OWNER is not an implicit permission wildcard. ADMIN cannot
  grant, change or end OWNER. Self-promotion is refused; OWNER self-reduction
  must be a subset and preserve another actual owner. Authorized reads and
  reductions continue on suspended/archived roots and without a capability;
  additions require the capability. State, one account-attributed audit event
  and immutable operation acknowledgement commit or roll back together. The
  private command receipt retains its human explanation; ending a role does
  not overwrite the immutable reason for the original grant.

- **B-BR-EMP-72 — Ownership consent is participant-scoped and current.**
  A current personal OWNER proposes to an ACTIVE personal member with an active
  employee. The explicit step-down choice is required. The server fixes validity
  at 168 hours, permits one pending proposal and rechecks both participants at
  acceptance. The existing OWNER must still be current. A new OWNER is created
  before optional step-down; an existing live ORG ADMIN assignment is reused.
  Remaining future OWNER periods are ended too. If only a future ADMIN period
  exists, bridge from now until its first start while preserving that plan.
  `org.ownership.respond` permits an attempt by a live fixed-role assignment,
  including SELF; only the actual initiator/recipient may inspect the proposal,
  only the recipient may accept, and cancellation/decline is derived from the
  current participant identity. No roleless implicit grant is invented. Session
  revocation and current authority apply to original commands and exact retries.
  Audit uses the account actor; immutable receipts preserve original outcomes.

- **B-BR-EMP-73 — Ownership reductions and notices preserve source truth.**
  Suspension/end of either participant or organization archive/closure cancels
  a pending proposal atomically, retaining the actual source cause/revision.
  Expiry is materialized at actual server observation and never trusts a client
  clock. Participant notices remain PENDING_NOTIFICATION_CORE until the owning
  notification workflow delivers them. Upgrading existing rows does not invent
  historical authors, receipts, notices or source facts. An audit, notice or
  receipt failure returns no provisional state and rolls back the handover.

- **B-BR-EMP-74 — Invitations reserve access without granting it.** A fresh
  authorized manager issues for an ACTIVE employee with no live access, using
  F1 ORG/SELF templates within the manager's actual permission union. Reserve an
  unbound INVITED membership without assignments. Capture the settings revision
  and its1..30-day validity at server intake. Explicit CAS resend supersedes the
  prior pending invitation and preserves its reservation. When no pending token
  or accepted account remains, end the reservation; later issue creates a new
  period. An explicit resend avoids accidental concurrent replacement.

- **B-BR-EMP-75 — Invitation secrets cannot be recovered or replayed.** Generate
  256 random bits; retain only a domain-separated keyed hash and random locator.
  Deliver code, fragment-only link and QR payload once after commit. Exact retry
  returns original public metadata without the secret; after lost delivery the
  manager explicitly reissues. No token/verifier appears in status, receipts,
  notices, central audit or generic formatting. Context changes revoke pending
  tokens with actual source evidence. Scoped management reads materialize server
  expiry. Failed commits expose no provisional secret. Public redemption and
  attempt limits remain IDN-005; no email, account or device provider is added.

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

- **BUC-EMP-35 — Suspend and recover without losing field history.**
  A fresh operator suspends an ACTIVE organization with reason and CAS. Current
  company writes stop; authorized reading and access reduction continue. A
  concurrent pre-existing completion or pre-suspension fact is classified for
  review using actual server time; malformed, foreign and late facts are refused.
  Resolve the same cause, leaving held facts in review. Concurrent replay/CAS,
  revoked/stale operator proof and late audit/storage failures expose no partial
  state or acknowledgement. The same controls retain causes on ARCHIVED roots.

- **BUC-EMP-36 — Manage access with current authority.** A fresh manager grants,
  changes or ends an assignment under the literal permission subset. Refuse
  forged member/session pairs, foreign targets, stale or revoked proof, ADMIN
  modification of OWNER, expanded coverage and last-owner removal. After a
  successful change, an exact retry returns the original acknowledgement;
  altered operation reuse fails. Reads and authorized reductions survive archive
  or capability removal. Concurrent retries/CAS and late commit failure retain
  one audit/receipt or no partial result.

- **BUC-EMP-37 — Propose and accept a private ownership handover.** A current
  OWNER selects an eligible personal recipient and explicitly chooses step-down.
  A nominated DRIVER with a SELF assignment can read and consent with fresh
  proof; another member receives no private transfer state. Recheck initiator
  ownership, target eligibility, session and capability before acceptance. A
  source reduction cancels rather than resurrects a proposal. Concurrent accept,
  decline, replay and owner removal serialize; deferred storage failures roll
  back roles/state/audit/notices/receipts and allow an honest retry. Normal
  transfer does not implement exceptional unreachable-owner recovery.

- **BUC-EMP-38 — Hand over and replace an invitation privately.** Issue an
  eligible DRIVER SELF invitation and receive its code/link once, with an
  INVITED reservation and no access. Retry without code, read/list status, then
  replace with the observed revision. Only one concurrent replacement succeeds;
  the old code is unusable. Revoke/expire the final pending token and end its
  reservation. Contact edits, employee reduction, issuer suspension/end and
  organization archive revoke with source evidence. Foreign targets, expanded
  grants, stale proof and storage failures disclose no token or private state.
  Redemption and privileged confirmation remain a separate task.

- **B-BR-EMP-76 — Redeem invitations with personal consent and current source
authority.** An authenticated personal account must review the current organization
name, legal verification, literal grants and versioned privacy notice before acceptance.
The consent digest binds that preview; no account identifiers or credentials come from
the business DTO. Check the secret in constant time, expiry, organization
capability/state, issuer rights and employee again in the committing transaction.
Decline grants nothing. Existing live membership refuses with ALREADY_MEMBER without
consuming the invitation. Unknown/expired/replaced/revoked/used/context-invalid tokens
share TOKEN_INVALID. Five wrong secrets revoke a pending invitation and retain a notice;
unknown locators have durable keyed account, browser-session and direct network budgets
of ten/hour. No raw token or network address enters persistence or logs.

- **B-BR-EMP-77 — Confirm privileged admission before granting access.** Any grant other
than DRIVER or OPERATIONS_STAFF, or the organization's optional
require_invitation_confirmation policy, binds the accepting account but leaves the
membership INVITED with no roles. A fresh authorized manager must explicitly confirm the
displayed account alias and revalidate the issuer and grants before activation. Reject,
actual source reduction or the exact seven-day deadline ends the reservation; no roles
are granted implicitly. Personal operation receipts are immutable and
caller/intent-bound; every retry rechecks proof and returns the original outcome without
restoring access. Ordinary account/root locking serializes acceptance, confirmation,
erasure and reductions. Native device enrolment and screens retain their separately
owned tasks; this source checkpoint exposes the existing company-cookie account
bootstrap.

- **BUC-EMP-39 — Review, accept and confirm a private invitation.** A signed-in personal
account previews a valid code, accepts the returned notice/digest or explicitly
declines. DRIVER SELF activates immediately unless confirmation is required; privileged
acceptance remains INVITED until a fresh manager confirms the account alias. The same
personal operation returns its original result; other callers/operations cannot reuse
the token. Exercise wrong-secret lockout, unknown-locator budgets, stale issuer/proof,
source reduction, confirmation equality deadline, simultaneous acceptance/confirmation
and deferred failure with real PostGIS. A failure exposes no organization details and
commits no provisional access.

- **B-BR-EMP-78 — Suspend organization access with current manager authority.** A fresh
current member with idn.membership.manage may suspend another ACTIVE membership with a
reason and observed revision. The server locks affected accounts in UUID order before
proof/capability/root and rechecks current proof after waits. Self-suspension and
removal of the last continuously covered owner are refused. Membership becomes SUSPENDED
with MANAGER origin; original role periods remain unchanged. The next organization
request is denied even with an otherwise valid personal session. Existing source
projections revoke outstanding issuer invitations, cancel ownership proposals and clear
team/designation links with actual history. Personal accounts, credentials, sessions and
other organizations are unaffected.

- **B-BR-EMP-79 — Reactivate only eligible recorded authority.** A fresh authorized
manager may reactivate a SUSPENDED membership only while the employee and bound account
are active and at least one original assignment is currently valid. Check assignment
periods using server time, including after lock waits; never extend or recreate an
expired/ended/future assignment. Employee-inactivity suspensions cannot be lifted while
the employee is inactive. No ENDED period is reopened, and no consumed token, cleared
team/designation or cancelled proposal is resurrected. Read/reduction remains available
under existing suspended/archived or capability-disabled policies; reactivation is a
configuration write. Every command has one immutable actor/intent-bound acknowledgement,
audit and pending notice; retries recheck current authority and preserve the original
result after later state changes.

- **BUC-EMP-40 — Pause and restore one membership.** A manager reads the current
revision, suspends another member with a reason and observes immediate denial on that
member's next request. Reactivate with the new revision when an original role is still
valid. Exercise self/foreign targets, no remaining role, inactive employee, managed
account isolation, last-owner races, identical and conflicting retries, stale/revoked
proof, source reductions and deferred storage failure/recovery. All accepted state,
audit, command and notice facts commit together. Native locking, device registry,
offboarding and offline review retain their owning tasks.

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

- **B-BR-EMP-80 — End current organization access with a reviewed handover.**
  Fresh current OWNER/ADMIN authority may end an ACTIVE or SUSPENDED existing-account
  membership, preserving its recorded cause and ending original role periods through
  the installed source policy. Last-owner protection is mandatory. A manager may
  retain the employee or terminate it at its exact reviewed revision. The server's
  bounded current handover inventory covers actual team links and settings
  designations; a digest binds that inventory and employee/membership revisions.
  Every present item must be explicitly retained as a pending manager flag; the
  current optional designations require no invented successor. Concurrent source
  changes invalidate review. Flags retain original source identifiers/revisions;
  no work is closed, reassigned or deleted by this command. Existing invitation,
  admission, proposal, team and settings reducers remain atomic.
- **B-BR-EMP-81 — Leaving is a personal action, not a role grant.** A fresh personal
  cookie and current locked source proof may end only that account's selected
  existing-account membership, including a suspended membership or one with no
  current role. The same last-owner and reviewed-responsibility rules apply.
  Self-leave cannot terminate the employee. An exact caller/intent-bound receipt
  can be read on retry after ending, while current account/proof and original
  binding still hold. Managers cannot use this entry to act for another account.
  Personal sessions, keys, private data and other memberships are untouched.
  Future managed provisioning/device/delegation/operational producers must extend
  this source boundary before activation; absent producers are identified as
  unavailable, never reported as successful revocations or empty live inventories.
  Managed provenance is refused until its full owning workflow exists. Late facts
  retain their dedicated intake/review contract; this task does not accept them.
- **BUC-EMP-41 — Review and end one organization membership.** Read the actual
  handover and revisions; explicitly acknowledge its pending flags, choose employee
  retention/termination (manager only), and submit a reason and operation UUID.
  Lock accounts in UUID order, current proof and root; recheck review, authority,
  owner coverage and source provenance. End access, persist source-backed flags,
  audit, immutable original acknowledgement and pending notice together. Return no
  provisional result on failure. Personal self-leave follows the same source
  boundary without needing a current organization role. Exercise changed review,
  stale/revoked proof after waits, wrong caller, missing/foreign source, last owner,
  retries/CAS, source reductions, upgrade recovery and actual next-request denial.

- **B-BR-EMP-82 — Rehire preserves identity and requires fresh access provisioning.**
  A fresh current OWNER/ADMIN with idn.membership.manage may reopen only a
  TERMINATED employee of this organization that was not merged away and has no
  INVITED, ACTIVE or SUSPENDED membership. Require its exact identity revision,
  canonical operation UUID and exact recorded reason. Only employee state/revision
  changes to ACTIVE; profile, ended memberships/assignments, tombstones, source
  reductions and private personal data remain unchanged. Account/proof,
  capability and root locks retain current authority after waits and before
  acknowledgement. Immutable actor/intent-bound receipts return the original
  outcome after later changes. A separate fresh invitation creates a distinct
  access period; no role, credential, device, token or former membership is
  restored, and no personal account is recovered. Managed provisioning retains
  GOV-013 and its owning implementation gates.
- **BUC-EMP-42 — Reopen a terminated employee, then invite separately.** Read the
  existing employee at its current revision; submit rehire with a reason. Prove
  current manager authority, same-organization identity, unmerged termination and
  absence of a live period under the retained root lock. Atomically reopen and
  record actual audit and immutable acknowledgement. Return no provisional result
  on SQL/commit failure. If access is needed, issue a new invitation through the
  existing guarded workflow. Exercise duplicate/distinct CAS, changed reason,
  foreign/merged/live-period target, revoked or expired proof after lock waits,
  disabled capability, late fault recovery and intact historical periods.

- **B-BR-EMP-83 — Review duplicates and retain both historical identities.** A fresh
  current organization-scoped idn.employee.manage holder chooses source and survivor
  explicitly. Existing match warnings never authorize a merge. Preview both current
  records and actual team links; confirmation requires their exact identity revisions,
  a server review digest, explicit reviewed=true and a recorded reason. Neither
  record may already be merged and both must belong to this organization. Keep the
  live membership holder as survivor; a source with any INVITED/ACTIVE/SUSPENDED
  period conflicts, including two live records. This preserves immutable membership
  bindings without implicitly ending access; reverse the explicit selection when
  only the source has access. Profiles are not automatically combined or overwritten.
  Mark the source TERMINATED with an immutable pointer, keeping original IDs in all
  prior records. Root-serialized, bounded pointer resolution finds the current
  survivor and refuses corrupt cycles/missing targets/depth overflow without partial
  output. A later merge may extend the chain, never rewrite earlier pointers.
- **B-BR-EMP-84 — Transfer current team associations as new periods.** Before marking
  the source merged, close each actual current source team link and create a survivor
  link only when none already exists. Preserve original link IDs and employee IDs;
  trace both periods through the actual merge audit/receipt. Current team/unit and
  survivor admission rules still apply; an ineligible transfer conflicts atomically.
  Bound one merge to 500 links and historical resolution to 64 hops; excessive work
  is refused without truncation. Preserve profiles, ended periods, roles, accounts,
  personal sessions, other organizations and business records. Immutable actor/intent
  receipts return the original result after later changes; current authority remains
  required. Audit, projection, pointer and receipt commit or roll back together.
- **BUC-EMP-43 — Merge a reviewed duplicate and resolve history.** Read the source and
  chosen survivor with current membership/team facts; confirm the digest, revisions
  and reason. Recheck fresh source authority under account/proof/capability/root
  locks. Transfer/deduplicate teams using real owning-module guards, then record the
  source pointer and original acknowledgement. Historical lookup returns the original
  record and the current survivor with a bounded chain. Exercise two live periods,
  wrong orientation, foreign/self/merged/cyclic targets, changed profiles/teams,
  simultaneous opposite merges, retries/CAS, proof waits, late failure and ordinary
  migration recovery. No personal account is merged, transferred or revoked.

- **B-BR-EMP-85 — Contact replacement consumes outstanding admission atomically.**
  A current fresh idn.employee.manage holder changes organization work contact
  through the existing complete profile replacement and exact profile revision.
  Persist canonical e-mail/phone; only a stored contact difference consumes PENDING
  invitations and WAITING acceptances, retaining CONTACT_CHANGED and the actual
  profile revision. Clearing a contact counts; canonical-equivalent spelling or
  unrelated metadata does not. Invalidation, reserved-period ending, notices,
  profile, actual audit and original acknowledgement commit together. A fresh
  invitation is explicit; an old receipt never restores a revoked secret. Already
  ACTIVE memberships, roles and personal/other-organization access stay unchanged.
  Uninstalled managed activation producers must consume the same contact source
  before activation under their owning implementation/GOV-013 gates.
- **B-BR-EMP-86 — Contact writes retain fresh authority through source completion.**
  Recheck current source-owned manager authority after profile/admission projection
  waits and before recording the acknowledgement. Retain account/proof/capability/
  root locks. Expired proof, a source failure or a deferred commit failure yields
  no partial profile, revocation, audit, notice or receipt. Exact retries require
  current authority and return the original profile acknowledgement, never repeat
  reductions against a later reissue. Concurrent redemption, confirmation, reissue
  and contact replacement serialize to one source-consistent result.
- **BUC-EMP-44 — Replace work contact, then reissue separately.** Read the employee
  profile; submit the full replacement and expected profile revision through the
  existing guarded change route. Prove current manager authority, persist the
  canonical profile and consume outstanding admissions through their owning SQL
  guards. Revalidate fresh authority, record the original acknowledgement and
  commit atomically. Reissue explicitly if access is still wanted. Test email,
  phone, clearing/equivalent contacts, waiting/active/no admission, source races,
  exact retries/CAS, stale proof after waits, foreign/merged/non-manager/disabled
  refusals and late rollback/recovery without affecting personal access.

- **B-BR-EMP-87 — Enrol actual signed keys per active membership.** An existing
  personal account proves its current family and actual bound signing key through
  the normal signature verifier; client IDs/flags never confirm possession.
  Enrol only for its current ACTIVE membership and ACTIVE employee under retained
  account/family/key/capability/root locks. Default to3 current devices, with a
  server-owned deployment limit1..5; callers cannot choose counts or limits.
  Count and insert atomically. Keep organization/membership periods independent
  of personal cloud devices and never reactivate a revoked enrolment. A lost or
  compromised key cannot enrol again in this organization. ACCESS_ENDED remains
  terminal in the old membership; a distinct newly ACTIVE membership after an
  explicit fresh invitation may enrol its still-valid personal key as a new period.
  Managed activation/recovery remains gated by GOV-013 and IDN-006..008.
- **B-BR-EMP-88 — Device revocation is scoped and terminal.** Fresh account-bound
  self-management and current fresh idn.credential.manage may list/revoke only
  the selected membership's organization enrolments. Manager reduction remains
  available in suspended/archived organizations. Require exact device revision,
  canonical operation UUID, reason and LOST_OR_COMPROMISED or ACCESS_ENDED.
  Retain original receipts, actual audit and server timestamps. Every installed
  signed enterprise device request except first-enrolment bootstrap checks the
  current exact enrolment and membership; recheck after waits before success.
  Revoked keys cannot use bootstrap to restore the same period. ACCESS_ENDED carries no normal
  authority; future intake-only delivery must implement its own approved window.
- **B-BR-EMP-89 — Preserve personal access and honest device metadata.** Membership
  ending revokes its remaining enrolments ACCESS_ENDED atomically with source
  membership revision; an earlier lost/compromised classification stays intact.
  Suspension denies use without inventing a membership ending. List only scoped
  label, enrolment/revocation state, timestamps and revision; never expose personal
  families, account sessions, key inventory or other organizations. Last-sync is
  null until a real sync producer records one. No remote/offline wipe is promised
  by server revocation. Device/audit/receipt/source failure commits no partial state.
- **BUC-EMP-45 — Enrol, inspect and revoke an organization device.** Sign enrolment
  with current account/key proof, supply a private label and exact operation ID,
  and count slots under source locks. A current signed check proves enrolment on
  the next request. From fresh personal self-management or authorized manager
  entry, list this membership's devices and revoke at the exact revision/class
  with a reason. Preserve original retries and unrelated/personal access. Exercise
  forged or unbound keys, expired/revoked proof after waits, last-slot and
  enrol/revoke/offboard contention, foreign selectors, missing permission,
  immutable history, late faults and populated ordinary migration recovery.

- **B-BR-EMP-90 — Discover only the proven account's current contexts.** The
  existing-account company session may discover its own organization/membership
  references through a narrowly scoped account source. Clients cannot select an
  account or infer authority from a cached directory. Ordinary forced RLS and
  current account/family proof remain mandatory; each returned context is checked
  against current membership, employee, organization, capability and role source.
  Selection revalidates that same binding. Expose only the current organization's
  permitted UI capabilities and literal grantable role/scope options; every
  business request still performs normal authorization. No public/global
  organization listing, managed activation or personal-data tenant boundary is
  introduced.
- **B-BR-EMP-91 — Panel state follows the current confirmed organization.**
  Employee and invitation screens use actual guarded APIs and explicit server
  acknowledgements. Employee access state is a current scoped membership read,
  separate from employee lifecycle and invitation state; an unhydrated view is
  UNKNOWN and never grants access. Preserve operation IDs and exact revisions for retries;
  report conflicts, fresh-authentication and unavailable outcomes truthfully.
  Organization/account changes, logout, expiry and access refusal immediately
  clear private records, in-flight commands and delivery secrets. Late responses
  cannot repopulate a previous scope. Invitation code/link delivery is ephemeral
  and appears only in an issuance/resend response, never in navigation, history,
  local storage or logs. Confirmation and rejection require explicit review.
- **BUC-EMP-46 — Manage employees and invitations in the selected company.**
  Sign in with an existing account, load its current contexts, explicitly select
  one and confirm current authority. List/filter/read or create employees, review
  actual duplicate warnings, issue an explicitly scoped invitation, deliver its
  one-time code/link and inspect metadata-only history. Resend/revoke or review
  and confirm/reject an acceptance through exact guarded revisions. Test keyboard
  access, empty/error states, unauthorized roles, wrong/changed accounts and
  organizations, stale responses, lost acknowledgements and source revocation.

- **B-BR-EMP-92 — Native invitation proof is ephemeral and identity-bound.**
  The normal Android entry uses its existing personal account and configured
  canonical HTTPS origin to open the current company-cookie plane. Bind every
  preview and decision to account, family, origin and session-fence epoch;
  recheck before and after network waits. Account/family changes, logout,
  background invalidation and definitive refusal clear private state, and late
  responses cannot restore it. Code, cookie, CSRF and consent digest stay in
  private memory, outside navigation, SavedState, persistence, backups and logs.
  The dedicated client refuses redirects, alternate authority headers, foreign
  cookies, oversized/unreadable responses and unsupported notice versions.
- **B-BR-EMP-93 — Native consent never predicts membership.** Preview the actual
  organization, verification status, literal role/scope grants and the supported
  localized privacy notice. Consent is never preselected. Accept or decline
  explicitly online; only a valid server acknowledgement may display ACTIVE,
  awaiting confirmation or declined. ACCEPTED with an INVITED membership remains
  awaiting confirmation and grants no access. A lost decision acknowledgement
  retries the same operation and decision, without contradictory new intent.
  Invalid invitation outcomes use generic copy. Fresh authentication uses an
  explicit existing-account key entry; no personal-family revocation, managed
  activation, device enrolment or offline grant is implied by acceptance.
- **BUC-EMP-47 — Review and answer an invitation from Android.** From Perfil,
  enter an invitation with an existing account, sign in or reconfirm when needed,
  request the current preview, review privacy and grants, then explicitly accept
  with consent or decline. Keep pending confirmation distinct from active access.
  Test offline and invalid inputs, unknown notices, missing consent, uncertain
  delivery retries, wrong accounts/origins/families, backgrounding and stale
  replies. Validate the actual navigation on the connected physical ADB phone
  using an isolated package, preserving existing apps and their data.

- **B-BR-EMP-94 — Context selection follows current source proof.** Discover
  only the existing account's bounded current organization page through the
  installed company-cookie context API. Bind metadata and selection to account,
  canonical origin, personal family and fence epoch. Read the exact organization/
  membership from the source before selecting it; a list entry or invitation
  receipt never grants access. Keep names only in memory. Backgrounding or
  transport loss withdraws confirmation and all local handles; retained names
  are visibly unconfirmed and cannot enable commands. Account/origin/family
  changes and definitive authority refusals clear metadata and selection.
  A fresh process starts in personal mode, requiring explicit reselection.
- **B-BR-EMP-95 — Switching cannot move previously created private work.**
  Invalidate the old local selection synchronously before waiting for a source
  read or entering personal mode. Local generation and current identity checks
  apply before and after I/O, including a read already in flight. A write that
  already committed may remain only in its original immutable partition; do
  not claim rollback, retag it, erase it or enumerate another queue. Selection
  does not enable the unavailable protected persistence/contact authority.
  The enterprise shell always names its company, offers explicit context
  selection and never fabricates operational modules, pending counts or offline
  authority. System back does not silently switch a selected company to personal.
- **BUC-EMP-48 — Choose a personal or company workspace.** From Perfil, open
  the company selector, fetch current eligible memberships, explicitly select a
  company and show its confirmed home shell. Use the selector to choose another
  company or return to personal mode. After background/offline, mark retained
  metadata unconfirmed and revalidate before selection. Exercise two companies,
  held original-partition operations, revoked membership, late replies, changed
  account/origin/family, empty paginated results and fresh-process restoration.
  Validate normal navigation and privacy on the connected physical ADB phone.

- **B-BR-EMP-96 — Enterprise assets have their own scoped identity.** Build
  the asset domain independently of personal vehicles, storage and their limits.
  An asset reference explicitly contains its organization and a distinct asset
  identifier type; personal DomainId/vehicle objects have no implicit conversion.
  A raw UUID does not prove its source table: registered-asset existence remains
  the owning persistence/application obligation. Category and fuel-product
  descriptors come from explicit catalogs, never arbitrary labels or a copied
  personal enum. Unknown codes fail closed. Catalog fixtures are synthetic;
  record the actual initial category catalog before asset persistence activation.
  Catalog codes remain opaque Unicode scalars; canonical product sets use
  Unicode code-point order, avoiding JVM UTF-16 versus UTF-8 sorting differences.
- **B-BR-EMP-97 — Asset normalization and quantities agree across clients.**
  Plate identifiers accept ASCII letters/digits, ASCII spaces and hyphens;
  comparison removes spaces/hyphens and uppercases ASCII letters, yielding
  exactly seven alphanumeric characters. Fleet numbers accept at most twenty
  printable ASCII characters, require a nonempty normalized key and use the
  same comparison transformation while retaining other printable punctuation.
  Non-ASCII/control identifier input is rejected explicitly, avoiding locale/
  Unicode-version-dependent case expansion. Display names and optional serial/
  chassis text remain bounded Unicode metadata with no control characters.
  Capacity and expected-consumption values use the existing exact integer
  kernels and catalog units, never floating-point arithmetic. Tank capacity is
  1..10^9 milli-units or watt-hours as specified by the existing quantity bound.
  An optional expected-consumption reference is a canonical positive signed
  64-bit integer with basis/product, bounded nonblank source text (280 Unicode
  characters) and an exact ISO civil date in years0001..9999. The integer bound
  is representational, not an invented physical consumption ceiling. Activation checks
  require same-organization unit/meter facts; pure fixture facts are not runtime
  enrollment or proof that a vehicle is complete. Shared Go/Kotlin vectors cover
  these boundaries before either implementation is accepted.
- **BUC-EMP-49 — Validate a company asset before registration.** Given an
  organization-scoped asset draft, resolve the declared category and products,
  normalize identifiers and validate tanks, exact consumption references and
  activation prerequisites. Refuse unknown codes, foreign references and an
  attempted kind change after an accepted operation. A vehicle/machine needs a
  real in-service meter and tank; an implement does not. Preserve personal data
  untouched and return redacted diagnostics. Persistence, actual meter evidence,
  assignments and lifecycle commands follow their owning tasks.

- **B-BR-EMP-98 — Sync identity seals the whole immutable operation.** Version1
  binds operation UUID, owner account, canonical HTTPS API origin, organization,
  membership, organization enrollment UUID, type/schema, entity/revision,
  declared time/zone/offset, hard dependencies, soft references, attachment
  declarations and payload. Resolve the enrollment to the actual signed key and
  current source facts later; none of those IDs establishes authority. Hash all
  immutable fields except the digest itself. Delivery family/access token,
  signature, attempt/clock sample and server receipt time stay outside that hash.
  Preserve creation ownership on retries and after login refresh.
- **B-BR-EMP-99 — Sync serialization and outcomes are unambiguous.** The
  enterprise canonical JSON profile permits null, booleans, well-formed Unicode
  strings, arrays, objects with ASCII lower_snake_case keys and canonical exact
  integer tokens. Reject floats, exponents, leading zeroes, negative zero,
  duplicate/aliased keys, invalid UTF-8 and unpaired surrogates before decoder
  replacement. Sort object keys; emit UTF-8 without HTML/Unicode normalization.
  Quote and backslash use two-character JSON escapes; JSON C0 controls use fixed
  lowercase six-character Unicode escapes rather than short control escapes.
  This is a small enterprise profile, not a claim of RFC8785 compliance.
  Derive SHA256 from canonical complete intent, preserving arbitrarily precise
  integer lexemes until the owning feature applies its numeric bounds.
  Canonical UTC declaration/receipt strings and zone/offset validation inherit
  PLAT-015; preserve a device offset even when it disagrees with its entry zone.
  Enforce50 operations,65536 raw UTF-8 batch bytes and8192 raw/canonical bytes per
  complete operation. Limit full transport JSON to64 container levels and
  operations to62, reserving its two transport wrappers. Limits are trusted
  configuration, never a request-selected bypass.
  ACCEPTED/ACCEPTED_FOR_REVIEW/REJECTED are final; RETRY is not. Caller-supplied
  final flags are invalid. Review requires a review ID; retry has no entity
  revision/review ID/final receipt. CLOCK_SKEW is a separate warning.
  Every result echoes operation ID and the original payload_digest so the
  client can correlate it with the immutable queued intent. Operation/result
  digests use the literal sha256: prefix and64 lowercase hexadecimal digits;
  attachment SHA256 declarations use the existing bare64-digit format. Pending
  hard dependencies retry without becoming final; a held predecessor requires
  actual source resolution before its effect is available, without rewriting
  the original acknowledgement. Missing/refused soft links are review facts,
  not a silent conversion of hard dependencies.
- **BUC-EMP-50 — Build and verify a retryable enterprise operation.** Capture
  immutable creation context, typed entity/time/dependency/file facts and exact
  payload; encode/hash the canonical intent and verify matching owner/origin/
  organization against an independently supplied source binding. Reject changed
  metadata with a reused digest and preserve stable IDs/bytes across delivery
  credential refresh. Validate supported result/code/finality combinations and
  bounded batches through shared Go/Kotlin vectors and OpenAPI components.
  This protocol task installs no route, intake ledger, queue or feature effect;
  actual current authority and independent transactions belong to OFFL-002.

  The batch wrapper may carry0..50 raw operation candidates. Parsing that wrapper
  does not certify its candidates: OFFL-002 must preflight trustworthy correlation
  identity and source binding before any effect/receipt, then handle well-formed
  per-operation version/feature failures independently. Uncorrelatable malformed
  items cause a body refusal before prefix effects. Operation schemas are
  positive signed64-bit values, with only1 supported initially. Credentials are
  untrimmed ASCII graphic strings of1..512 bytes, matching the existing signed
  gateway bound. Canonical operation/binding origins use exact lowercase HTTPS,
  ASCII DNS LDH labels or IP literals, no userinfo/path/query/fragment/zone IDs,
  and explicit1..65535 non-zero-padded ports except omitted default443. Preserve
  valid lowercase IPv6 spelling and a DNS trailing dot; neither is rewritten.
  Numeric-only ambiguous hosts must be valid IPv4, never resolved through DNS.
  MIME declarations use the frozen lowercase type/subtype token profile, with
  no parameters or wildcards; matching syntax is not content inspection.


### Authorization conformance checkpoint (EMP-AUTH-009)

- **B-BR-EMP-100 — Assert the fixed policy through current source authority.**
  The org-permissions-v4 catalog has85 registered keys and13 fixed roles;
  its1,105 independent descriptor cells are not authorization by themselves.
  The conformance suite covers every applicable routed permission through the
  current request pipeline and ordinary NOSUPERUSER/NOBYPASSRLS PostGIS source.
  Include explicit allow/deny, missing/expired/ended/suspended assignment or
  membership, exact organization, own versus other resource, capability,
  freshness and record-rule cases. Denials reveal no private resource data.
  Ending authority denies the next request without changing the session.
  Route inventory and the complete-router gate refuse undeclared or unguarded
  routes and unexplained coverage omissions. Future routes/scopes remain named
  obligations; a missing route is never a passed business acceptance.
- **BUC-EMP-51 — Reproduce organization authorization conformance.** Load the
  independent versioned matrix; reconcile every fixed descriptor and each
  current routed permission with an explicit representative route and normal
  current-source request cases. Inspect actual request effects/denials and
  next-request revocation using synthetic two-organization records. Assert
  ordinary database-role properties and keep future structural/delegation/
  station/feature checks assigned to their owning tasks. This test-only task
  changes no product permission, feature handler, source policy or runtime gate.


### Persisted company setup and settings (EMP-ORG-014)

- **B-BR-EMP-101 — Render current persisted setup through the owning server.**
  Extend the existing organization/settings read model with explicit current
  account, organization and membership binding, persisted profile/state and
  activation-checklist facts, and source-derived settings UI permission hints.
  Preserve separate settings revision and root revision: settings changes and
  historical reads use the former; activation uses the latter. Checklist items
  reuse the owning activation rules, not browser assumptions. Read-only members
  see permitted values/history without write controls. Every write independently
  repeats its current proof, role, scope, state, capability and freshness checks.
  Named responsibility choices come from a bounded organization-scoped eligible
  membership projection; employee IDs never become membership IDs, and editable
  UUID inputs never substitute for a verified named choice. Expose all current
  settings fields and revision history, with explicit conflict, validation and
  incomplete-checklist feedback. Historical settings are labeled as historical,
  separately from the current organization state.
- **B-BR-EMP-102 — Resume saved setup without inventing browser authority.**
  The isolated company setup/settings route first confirms the current server
  session and reads the selected organization through normal settings-read
  authority. A creation acknowledgement, URL or cached hint is only a selector;
  it never establishes current membership, permission, activation or general
  operational selection. Keep native context metadata and operational CanSelect
  policy unchanged. The organizations capability remains operator-owned and
  default-off; saved pending setup is shown truthfully while writes remain
  refused when the capability is unavailable. A bounded own-account projection
  in the existing onboarding module supports discovery of created setups and
  recovery after reload or a lost creation acknowledgement, filtering every item
  through freshly rechecked scoped settings-read authority. It grants no role,
  capability or foreign-organization discovery. Within a live view, retries reuse
  the exact immutable creation body and operation ID; changed intent is explicit.
  Replayed acknowledgements describe their original result, so the panel rereads
  current persisted state before presenting completion. Private drafts, receipts,
  settings and credentials are not persisted in browser storage. Switching
  account/organization, session loss, navigation or cancellation invalidates old
  work; late responses cannot restore its private contents or submit a new scope.
- **BUC-EMP-52 — Create, resume and configure a company through its panel.**
  A signed-in personal account starts from an empty or populated company list,
  reviews the source-pinned synthetic terms and submits one creation intent.
  After the server saves it, the panel opens persisted setup through current
  proof, including the truthful capability-pending case. It can rediscover only
  the account's authorized created setups after reload or an uncertain reply.
  The member reviews the text checklist, submits activation with the current
  root revision, reads/edits permitted settings with a reason and the current
  settings revision, chooses named eligible responsible members and substitutes,
  and inspects previous revisions. Conflict requires a new read and an explicit
  new decision; no automatic overwrite or early activation is claimed. Exercise
  duplicate submission, lost acknowledgement, incomplete persisted setup,
  unauthorized/read-only access, A/B isolation, stale replies and account/session
  replacement through ordinary real PostGIS and the actual HTTPS browser entry.
  Use accessible keyboard, focus and responsive states, narrowly allowlisted
  company routes and existing dependencies. Only synthetic data/terms are in
  scope; real-data, legal, provider, runtime and release gates remain separate.


- **B-BR-EMP-103 — Settings retries include invitation confirmation policy.**
  Every immutable settings field, including require_invitation_confirmation,
  participates in command identity. Correct the omitted flag with a versioned
  complete digest. Applied command receipts remain immutable. A legacy hash may
  replay only when the original immutable settings revision proves the complete
  submitted values, reason, operation/source and expected revision; matching a
  legacy hash alone never proves that a changed confirmation flag is a retry.
  Preserve original acknowledgement revision/time and current source authority.
- **BUC-EMP-53 — Retry old and new complete settings intent safely.** Capture a
  regression where flipping only confirmation policy reuses an old digest.
  Assert new complete digests differ; replay genuine legacy false/true intents
  from immutable stored revisions, refuse changed flag/other fields/reason and
  fail closed on missing or inconsistent historical evidence. Do not rewrite
  applied history or weaken receipts, revision checks or current authorization.


### Device protection completion (EMP-PRIV-003, F2 prerequisite)

- **B-BR-EMP-104 — Trust exact signed contact, never metadata or a generic denial.**
  Add a narrowly owned signed device contact action. It proves the current
  account/family/token, identity-v1 signature/nonce/key/contributor and exact
  configured origin, then binds request, organization, membership and enrollment.
  ACTIVE contact uses retained current-source locks and DB time and supplies the
  approved30-day offline validity. The existing-account pilot has explicit null
  managed-unlock policy; managed runtime remains unavailable until its owning
  AUTH/GOV policy exists. Preserve current suspended/archived/managed denials.
  Only an exact owned LOST_OR_COMPROMISED tombstone yields ERASE; ACCESS_ENDED
  yields LOCK and preserves ciphertext/pending work. Terminal contact reads only
  its owned enrollment tombstone, not ended-membership business data. Generic
  401/403/404, outage, malformed/foreign/late responses never authorize deletion.
  Contact is storage custody, not a business permission or F4 intake grant;
  each owning feature still enforces current permission/scope/cache eligibility.
- **B-BR-EMP-105 — Commit a nonrollbackable encrypted generation before local success.**
  Retain the bounded sealed-snapshot adapter without a new storage dependency.
  V2 binds partition, generation and exact membership/enrollment in authenticated
  ciphertext. An opaque Keystore committed head outside restorable app files
  selects the only accepted generation. Write and fsync ciphertext and encrypted
  maintenance descriptor plus directory before publishing its trusted commit
  marker; only then may local success be reported. Keep prior committed work
  through preparation failures. Uncommitted or old files/keys never become a
  fallback to an older lease/data snapshot; every new attempt uses a fresh key.
  Verify the committed head on every open. Bound preparation/obsolete cleanup
  and fail explicitly rather than evicting work or reusing an orphan key.
  No legacy or changed membership/enrollment/unlock policy silently rebinds
  existing ciphertext; preserve it locked until an explicitly reviewed fresh
  empty recovery, with acknowledged lost work and new current source proof.
- **B-BR-EMP-106 — Make maintenance discoverable, durable and scoped.**
  Enumerate encrypted maintenance descriptors by opaque tokens, with no readable
  account/organization/payload identifiers in filenames, aliases or worker input.
  Bind each descriptor to the same trusted head. Its maintenance key may run
  without managed unlock; payload keys retain the explicit managed policy.
  Persist denial/cleanup intent before erasure, independently attempt key/file
  cleanup, verify success before completion and retry failures after process
  death. Schedule bounded startup/periodic expiry work; every access still
  checks validity because background work can be delayed. Preserve other
  accounts/origins/organizations. Existing approved security revocation/30-day
  expiry key disposal remains distinct from ordinary retry/logout/access-ended
  preservation; report erased work honestly and require explicit empty recovery.
  Reboot or clock rollback alone locks/reconfirms unless existing expiry rules
  prove that the validity already ended. No instant offline remote wipe promise.
- **B-BR-EMP-107 — Handoff only the confirmed selection attempt.**
  Confirmation retains the original local-owner lease, account/family/epoch and
  background generation before durable I/O and before advancing the fence.
  A one-use expected E→E+1 handoff consumes that exact owner lease; it cannot
  adopt an arbitrary replacement epoch. Open the new metadata channel under
  E+1, recheck the source selection and only then open protected local context.
  New family/account/selection/background or late response cannot reopen data
  or withdraw a newer flow. Same-family token rotation keeps its documented
  identity semantics. Credential prompts never suppress lifecycle invalidation;
  start a fresh proof attempt after unlock when needed.
- **BUC-EMP-54 — Confirm or lock a device's exact protected context.**
  Use actual signed source contact and normal non-bypass PostGIS to prove active
  lease, exact identity/enrollment, terminal class, races, expired proof and
  failed commits. Bind the native lease to membership/enrollment and validate
  every response against the captured request/owner. Exercise account, origin,
  organization, family, enrollment and rehire mismatches without retagging data.
- **BUC-EMP-55 — Preserve committed private work through interruption and cleanup.**
  Prove all generation preparation/commit/recovery boundaries, old-ciphertext
  rollback refusal, missing/wrong keys, durable cleanup failures and independent
  partitions on host and actual Android. Use a new isolated emulator as now
  authorized, never reset unrelated AVD/app data. Prove actual Keystore,
  process/reboot/unlock/expiry and available OS backup/restore channels, retaining
  explicit owed evidence where the environment cannot prove a platform claim.
  Synthetic managed-key tests do not certify the missing managed server policy.

### EMP-OFFL-002 — Scoped intake and durable receipts

Recorded before behavior, 2026-11-13, from tested/published PRIV-003 51c25146.
This construction task implements the receipt/transaction boundary. Owning
feature handlers, historical authority, held-record/review producers and F4
exceptions remain explicit integrations, never fabricated defaults.

- **B-BR-EMP-108 — Validate the entire batch before intake writes.**
  The signed path selects one organization. Resolve actual account, configured
  origin, current membership, enrollment and signing key independently of the
  operation fields. Parse every candidate with the existing frozen codec and
  captured limits; compare every immutable owner/origin/organization/member/
  enrollment before any claim, intake budget, receipt or business effect. A
  broken envelope/digest/binding refuses the whole body without prefix effects.
  Identity nonce consumption keeps its existing independent authentication
  semantics. A structurally valid unsupported operation schema or malformed
  business payload can be classified independently only by an available owning
  type with actual current command/resource authority. No correlation-only
  parser or client-selected limits/policy are introduced.
- **B-BR-EMP-109 — Separate immutable claims, final ACKs and applied effects.**
  Reserve (organization, operation ID) and its complete immutable digest only
  after real owning policy/resource admission. An eligible DEPENDENCY_PENDING
  retry may retain that claim, but has no final ACK, business effect, audit or
  follow-up. Missing producer/history/review authority is UNAVAILABLE and cannot
  create a new claim. Final effect or held facts, audit, required follow-up,
  original ACK and effect-readiness proof commit in one ordinary scoped
  transaction. Each operation has its own transaction; one refusal/failure
  cannot roll back unrelated committed items. No success escapes before commit.
  The final received_at comes from fresh DB time, never device or reservation
  time. A failed commit returns uncertainty, not a provisional final result.
- **B-BR-EMP-110 — Dependencies follow actual effects, not receipt labels.**
  A hard dependency becomes ready only after its real committed effect or an
  accepted completed review. ACCEPTED_FOR_REVIEW alone is not readiness; missing,
  refused or foreign dependencies remain pending without revealing private
  receipt data. Do not lock another pending claim while holding this one or
  create placeholder claims for missing operations. The owning feature resolves
  soft references and actually holds orphan facts; generic intake never edits
  immutable intent/digest or pretends a missing review producer stored it.
  After base command/target admission, the owning handler must cover the actual
  parent records before a same-organization effect can release a dependency.
  Missing, foreign and unreadable parents return the same DEPENDENCY_PENDING;
  a coverage refusal rolls back this operation without a new claim or private
  parent data. Current covered cross-device dependencies remain valid. Missing
  historical/feature producers remain UNAVAILABLE. Confirm current authority
  after waits; never commit partially failed admission (EDR-075).
- **B-BR-EMP-111 — Replay is a current protected read.**
  Original ACK bytes are immutable and revalidated against the attempted
  operation before exposure. Reuse requires the same original owner/member/
  enrollment plus actual current receipt-read coverage from the owning type.
  A changed digest returns OPERATION_ID_REUSED for that attempt without replacing
  the first claim/ACK/effect. Corrupt stored bytes or missing read authority are
  unavailable/denied, never repaired into apparent success. Preserve minimal
  provenance and company records through existing personal deletion/unlink;
  do not add a live-account FK/cascade, personal export access or a personal
  30-day TTL to organizational receipts. Never store tokens/signatures or raw
  business payloads in the receipt ledger.
- **B-BR-EMP-112 — Recheck source authority after waits.**
  Current account/family/key/binding, membership/enrollment, record coverage,
  role/capability and clock-bound proof remain required inside each transaction
  and are confirmed with fresh DB time after target/audit/receipt waits before
  commit. A previous Prepare/Decision or custody lease is not lasting authority.
  Feature confirmation accounts for its own post-write state without applying
  twice. If authority ends after earlier items commit, stop with a safe transport
  refusal, retain those receipts and withhold private response data. No rollback
  of already committed items is claimed. Inability to verify the current source
  is equally fatal to private response exposure (bounded unavailable transport
  refusal); ordinary feature/storage availability remains per-operation retry.
  Current unknown/revoked/ended/suspended
  device/member proof stays a transport denial; F4 final/grace codes are not
  activated by the decision table. Declared time is evidence only; do not infer
  CLOCK_SKEW from legitimate offline delay without an actual timing source.
- **B-BR-EMP-113 — Meter intake without inventing a permission.**
  Use an explicit trusted transport budget through the existing database limiter
  and opaque organization/account/device axes. Charge max(1, operation count)
  admission units, including replay attempts, atomically before prefix effects;
  all axes roll back on budget failure. Prove source binding first so foreign or
  malformed bodies cannot spend another scope's budget. Policy/caps/rationale
  belong to source configuration, not the request. No unmeasured production
  default, borrowed credential permission or commercial-plan quota is implied.
  Missing configured admission stays unavailable; the first real feature
  registration must supply its reviewed budget. Tests may use an explicitly
  synthetic policy with real limiter storage and exhaustion races.
- **B-BR-EMP-114 — Compose existing audit and follow-up once.**
  Intake owns one generic offl.operation audit target per operation ID; feature
  work uses only the borrowed query port and does not commit/rollback/open a
  separate transaction or add a second same-ID audit. Event/consumer plans use
  the existing registered catalog, writer and queue; the bound audit hash is
  distinct from the immutable payload digest. Receipt event revision 1 does not
  invent an entity revision. The logical needs-review event uses the persisted
  name offl.operation.review, preserving the identifier vocabulary in applied
  migrations165/168; ACCEPTED_FOR_REVIEW and its wire code remain unchanged.
  Missing required real follow-up/held/effect
  producers stays unavailable. No production probe/no-op handler or new queue
  service is registered to manufacture acceptance.
- **BUC-EMP-56 — Recover a lost batch answer without duplicate effects.**
  Prove full-body preflight, ordinary PostGIS/signed normal intake, independent
  outcomes, original-byte replay, changed-intent refusal, twelve-way contention,
  target/proof/role expiry waits and rollback after every final projection stage.
  A rejected/retryable item must not abort another eligible operation.
- **BUC-EMP-57 — Preserve dependency and privacy boundaries.**
  Prove pending dependency claims, review without readiness, later actual effect,
  orphan holding through a real test consumer, current authorized receipt reads,
  foreign scope/device/member denial, late revocation response withholding and
  personal deletion preserving company history. Rehearse populated migration
  195→196, injected failure/recovery/noop and previous-image compatibility.
  Runtime producer absence, C1 feature readiness and release are distinct states.

### EMP-OFFL-003 — Protected Android operation queue

Recorded before behavior, 2026-11-13, from tested/published OFFL-002 d24b0d4f.
This task supplies durable local unit-of-work, delivery/retry and receipt ports;
the owning business producers and OFFL-004 pending screen remain separate work.

- **B-BR-EMP-115 — Seal one typed queue without reinterpreting legacy work.**
  Extend the accepted protected generation store, not a second plaintext Room
  outbox. Preserve the existing four entry ordinals and V1/V2 read contracts;
  append an explicitly versioned typed queue kind and framing. Existing opaque
  OPERATION entries remain retained and non-dispatchable, even if their bytes
  resemble the new format. A typed save cannot reuse a legacy operation ID:
  unknown complete intent/member/enrollment cannot prove replay or safe adoption.
  Refuse before projection/write and retain the legacy bytes; distinct genuinely
  new IDs may coexist. This restriction is on operation identity, not entity IDs.
  Queue reads/writes bind the complete frozen operation
  to the actual protected account/origin/organization/member/enrollment before
  use. A newer payload format cannot silently become an older generation or
  a different owner. Rollback pauses dispatch while retaining a reader for the
  accepted format; no APK downgrade compatibility or automatic data conversion
  is inferred. Preserve personal records, attachments and other partitions.
- **B-BR-EMP-116 — Commit queue and local projection together.**
  A local save commits its immutable sealed operation and owning local projection
  in one protected generation before success. A final ACK, owning projection
  and queue/retry status likewise commit together per operation. The producer
  uses only a borrowed, scoped local unit-of-work with no transaction lifecycle,
  network or unrelated storage; framework code owns commit. Same ID/intent
  replays without reapplying the projection; changed immutable intent is refused.
  Actual owning projection rules must preserve a newer local edit when an older
  operation's ACK arrives. A projection/write/key/capacity/commit failure keeps
  the previous generation when publication has not committed and reports
  uncertainty, never early success. After trusted-head publication, withhold a
  stale result honestly instead of claiming rollback; only the accepted security
  owner may erase a now-expired context. Recheck custody clock after commit I/O.
  Process death after one ACK leaves other operations recoverable independently.
- **B-BR-EMP-117 — A source attempt cannot adopt replacement ownership.**
  Capture an immutable attempt lease for the protected tuple, current account/
  family, fence epoch/background generation and unique attempt before local I/O
  or transport. Derive it from actual protected/current source, never selected
  context strings or caller-supplied authority.
  Local account capture/save must work offline with retained current identity,
  valid protected custody and actual owning offline eligibility; it never
  requires a network refresh merely because the access token expired. Delivery
  alone refreshes the same family before capturing its new attempt lease.
  Revalidate around signing, network and each local projection; release storage/
  fence locks before network.
  New account/family/origin/selection/background invalidates an in-flight attempt;
  never adopt a later epoch. Same-family token refresh may occur before a fresh
  capture; queued intent never includes or replaces delivery credentials.
  A worker starts its own new exact lease and never takes over a UI selection
  owner or retags work to the currently selected organization. Already committed
  original-scope work stays retained; stale results are not exposed under a new
  flow and no rollback of a committed generation is claimed.
- **B-BR-EMP-118 — Preflight the entire server answer before local writes.**
  Use the existing identity-v1 signer and exact configured HTTPS origin/path
  /v1/orgs/{org_id}/sync/operations, without cookies, redirects or cache.
  Decode the strict versioned response wrapper, count/order and every complete
  ACK against the captured original operation/digest before any receipt write.
  A malformed/foreign/duplicate/missing/trailing answer changes no local receipt.
  Store original canonical final bytes immutably; a changed final ACK is an
  unavailable inconsistency, not a repair. RETRY or transport success alone never
  becomes company acceptance. SENT is attempt-local; no durable SENDING flag
  can strand work after process death. Generic denials never authorize erasure;
  only the accepted signed custody/expiry owner can perform security cleanup.
- **B-BR-EMP-119 — Order local work without inventing server readiness.**
  Sort pending local parents before children while allowing unrelated work to
  progress. The server remains the authority for applied-effect dependencies;
  a missing, external or review-accepted parent does not become applied merely
  because an ACK exists. Keep such children retryable so a later actual server
  effect can release them without rewriting either original intent or ACK.
  Detect local dependency cycles, retain them with explicit attention and exclude
  them from automatic selection; never delete or rewrite them to break the cycle.
  Keep frozen50-operation/64KiB-batch/8KiB-operation limits and captured trusted
  reductions; select a bounded batch that really fits its delivery wrapper.
  The actual source may supply an opaque exact encoded-proof byte budget after
  capture/refresh, retaining no token in that value. Do not strand a valid
  reduced-limit operation by reserving an unnecessary512-byte worst-case token;
  actual signing still rechecks the private captured proof and complete bytes.
- **B-BR-EMP-120 — Retry without silent loss or an attempt ceiling.**
  Use source-owned operational defaults:30-second exponential base,6-hour cap,
  equal jitter within half..full calculated delay and saturating counters, with
  no transient attempt cap. Honor a valid server Retry-After without shortening
  it below the same 6-hour cap; longer or overflowing hints clamp to that cap
  (EDR-122). Delta-seconds remains the actual API's primary form. An HTTP-date hint is
  usable only with a unique valid Date header from the same authenticated HTTPS
  response: wait the full target-minus-Date interval, ignoring transit
  conservatively. Missing/invalid/duplicate Date makes that optional hint unusable
  and retains ordinary retry backoff; device wall time is never its authority.
  Clock/reboot uncertainty follows the existing protected lease/reconfirmation
  policy, not a guessed delivery deadline. Only after that actual custody guard
  permits access may an older-boot/backward-elapsed retry be rebased once onto
  the fresh clock, preserving its attempt count and restarting its full stored
  delay. This conservative restart never shortens Retry-After or strands work
  forever on an obsolete boot identifier. Locked, expired, wrong-family or
  unavailable source pauses/defers delivery while preserving work; managed
  locked keys require their accepted unlock flow. Retry status is protected
  data and cannot be updated by a replaced attempt. Competing valid attempts
  update retry state only by compare-and-swap against the captured complete
  attempts/clock/delay tuple. A changed/final row is a no-op; preserve the newer
  floor and re-read actual retained state for scheduling. Neither UUID nor wall
  time invents attempt ordering. Pressure warns at80% of the
  existing32MiB/10000-entry vault safety budgets; full storage refuses new work
  without eviction. Final ACKs and refused/review facts remain retained until
  an explicit owning dismissal/retention policy exists; no new TTL is invented.
- **B-BR-EMP-121 — Real producers own offline eligibility and projection.**
  Only explicitly registered owning offline-capable types/schemas with actual
  local eligibility, save and ACK projectors may enter/send through the typed
  queue. Custody/contact/context selection alone grants no business permission.
  Missing producer/policy/history/reference/attachment authority stays unavailable;
  retain older unknown work unsent. Production registry starts empty until a
  real owning feature is accepted. No probe/no-op producer or fictitious cached
  permission is installed. Online-only people/settings/approval commands are
  excluded; F4, managed-provider, real-receipt and release gates remain distinct.
- **B-BR-EMP-122 — Schedule scoped work with opaque system input.**
  Reuse WorkManager and existing private source/maintenance discovery. Jobs carry
  no readable account/org/member/enrollment, payload, token or queue cursor.
  Discover only the current account/origin's protected partitions; verify each
  partition and current source before opening its payload. Coalesce duplicate
  workers without relying on one process surviving; preserve at-least-once
  retry and independent organization progress. Use bounded cooperative work,
  cancellation and per-invocation ownership; no shared mutable janitor cursor,
  global selected-context dispatcher or new scheduling service is introduced.
- **BUC-EMP-58 — Recover durable local saves and partial ACK application.**
  Prove RED→GREEN atomic save/replay, every write/commit/projection failure,
  process interruption/restart and partial per-operation ACK recovery, original
  bytes/binding and two-organization/account/origin isolation. Prove legacy
  upgrade and personal/attachment preservation on host and the owned API36
  emulator, including actual Keystore and process death without clearing data.
- **BUC-EMP-59 — Deliver through the actual signed source without duplicate effects.**
  Use a named test-only real local producer and the actual ordinary PostGIS/
  signed intake plus SQL follow-up; lose an answer, restart and converge on the
  original ACK/effect. Validate full bad-answer preflight, independent retries,
  dependency/review/cycle ordering, actual worker duplication/cancellation and
  all ownership changes before/after I/O. Product APK contains no test producer.
- **BUC-EMP-60 — Preserve work while permission or storage is unavailable.**
  Prove protected32MiB/10000-entry refusal and80% warnings, locked/expired/key/
  clock/source failures, no retry exhaustion or automatic eviction, backoff and
  Retry-After bounds. A late account/family/context response changes no new
  source and never erases preserved work. Pause rollback retains the queue.

### EMP-OFFL-004 — Evidence-based local synchronization presentation

- **B-BR-EMP-123 — Project synchronization from retained evidence.**
  A durable operation without a final matching ACK stays SAVED_ON_DEVICE,
  regardless of connectivity, retry count or a completed HTTP request.
  Matching immutable ACCEPTED, ACCEPTED_FOR_REVIEW and REJECTED receipts map
  to ACCEPTED, NEEDS_REVIEW and REJECTED respectively. Every review/refusal
  retains its supported reason and exposes localized plain-language text.
  Acceptance never means business approval or payment. Review resolution needs
  its separate owning source; never rewrite a final review ACK to resolve it.
  A business record badge must use its owning projection's exact current
  operation ID/digest, never a UUID/declared-time guess or another older accepted
  operation for that entity. Generic transport errors do not create refusal ACKs.
  Unknown, corrupt or unavailable facts do not become successful empty state.
- **B-BR-EMP-124 — Read bounded sanitized protected snapshots.**
  Use an additive status port on the same accepted protected queue. A page has
  at most50 sanitized summaries, full-partition totals/pressure/legacy count
  and the latest confirmed server receipt from one trusted generation.
  Include retained final receipts; pending delivery alone is not the pending
  presentation. Support pending and retained-history views, and detail bound
  to the exact operation ID and complete intent digest. Never send payload,
  canonical operation/ACK bytes, delivery proof or private file bytes to UI.
  Cursor handles are memory-only, source/view-bound and generation-bound;
  a changed generation restarts the page rather than mixing snapshots.
  Each call rechecks the actual account/origin/org/member/enrollment/family,
  fence/background and custody before/after I/O. No plaintext Room fallback.
- **B-BR-EMP-125 — History names only facts actually retained.**
  The V1 queue proves durable save, latest retained retry observation and any
  final server receipt; it has no actual saved timestamp or complete attempt
  log. Label occurred_at as the author's declared time, never local save time.
  Show unknown times honestly. A server receipt is not the last successful
  device synchronization. Do not invent SENT timestamps or intermediate steps;
  a final receipt can directly establish its actual outcome. The latest known
  review result stays visible until a separate accepted resolution source exists.
- **B-BR-EMP-126 — Keep attachment delivery separate.**
  Show declared attachment count and unconfirmed delivery independently of
  operation acceptance. Detail may inspect actual protected local bytes in
  the same scope and distinguish missing bytes, matching size/hash, mismatch
  or unavailable inspection. No metadata ACK proves STORED, a percentage or
  successful upload. Upload retry, new capture, create-again and business-record
  navigation require real owning adapters; absent adapters expose no fake action.
- **B-BR-EMP-127 — Dismiss only a refused item's local visibility.**
  An explicit current gesture may hide a stored final REJECTED item from the
  pending view, conditional on its original ID/digest and retained final ACK.
  Add only a local dismissedRejected Boolean in inner queue frameV2, reading
  V1 as false. Refuse the flag without final REJECTED and reject unknown/trailing
  framing. Retain the exact original operation/ACK bytes, record projection,
  history and attachments. Do not alter retry or invoke a business projector.
  Repeated dismissal is a no-op; wrong digest, pending/review/accepted state,
  source changes and failed publication cannot silently hide another item.
  History includes explicitly hidden refusals. No TTL, deletion or eviction.
- **B-BR-EMP-128 — Scope the presentation owner and every gesture.**
  Keep list/detail inside the existing context destination and its owner;
  no selector IDs in routes or SavedState. Each visible flow owns a generation
  and captured selected account/origin/org/member, current family/fence/background.
  Clear private rows/detail/actions synchronously on replacement selection,
  personal mode, source change, stop or disposal. Check the rendered primitive
  epoch and exact flow generation before/after asynchronous work; late results
  never attach to a replacement owner. Reads use fresh offline source.account()
  and actual custody, without networking merely to refresh a personal token.
  Cached unconfirmed catalog metadata selects a local view, never operational
  permission. A status view never acquires or replaces LocalContext selection.
- **B-BR-EMP-129 — Observe actual attempts and respect retry floors.**
  A passive bounded memory-only delivery-activity port surrounds only the actual
  selected batch's transport and durable ACK/retry projection, and is removed
  in finally afterward, including cancellation. End-of-attempt refresh cannot
  run before the receipt publication it is meant to observe.
  Bound observation to64 concurrent tickets and the frozen50 IDs per batch;
  saturation omits a progress hint without preventing real delivery or changing
  durable state. This is an in-memory safety budget, not an attempt ceiling.
  Keep at most64 scoped completion hints with a monotonic revision. Conflated
  begin/end notifications and completion during a busy read cannot lose the
  durable reread: anchor observation before I/O and reconcile pending completion
  after publication, using bounded extra work. A history gap conservatively
  rereads only the still-current scope; the counter is never receipt evidence.
  Before awaiting renewed proof, withdraw old credential-bearing read handles
  and private presentation. Retain only the exact expected identity/family/
  epoch/background selector across failure; fresh proof must match it before
  another read or dispatch. Same-family rotation cannot race an old attempt
  handle into false invalidation, or permit adoption of a replacement family.
  It may show SENDING for those exact IDs as a saved-on-device progress hint;
  generic busy state/connectivity cannot create it. It does not persist SENDING
  or supply receipt/business authority. Refresh retained status after activity.
  A current manual synchronization check dispatches only its captured visible
  organization using fresh delivery proof. Honor the entire existing stored
  delay, which combines client backoff and server Retry-After; never reset it
  or promise immediate send. Connectivity only gates that network gesture.
  Missing producers stay unavailable and old/unknown work remains retained.
- **B-BR-EMP-130 — Present local emptiness, pressure and access honestly.**
  A pending badge counts each saved, review or visible refused record once,
  and includes accepted records whose declared attachments still lack delivery
  confirmation. Keep separate attachment totals; no double counting. Explicitly
  hidden refusals stay in history rather than reappearing through attachment counts.
  An empty
  local page never claims global delivery; nonzero legacy/unsupported data or
  unreadable source remains explicit. Show the existing80% pressure threshold
  and capacity guidance persistently in enterprise mode, without blocking or
  automatic cleanup. Oldest declared time is not measured unsent age.
  Use all supported locale catalogs, textual status/reasons, organization
  headings, accessible actions and polite state announcements. Protected
  unlock, unavailable storage and irreversible loss keep their distinct outcomes.
- **BUC-EMP-61 — Prove status projection and truthful history.**
  RED→GREEN every receipt/status/reason combination, transient/cancelled/unknown
  delivery, review finality, no fabricated times or attachment acceptance,
  local empty versus retained legacy, pressure and bounded generation paging.
- **BUC-EMP-62 — Prove privacy and stale-action boundaries.**
  Exercise two organizations/accounts/origins/families and source, membership,
  enrollment, epoch/background, selection and lifecycle changes while list,
  next page, detail, dismissal and manual dispatch wait. Withhold old results,
  clear private presentation synchronously and preserve other scope/data.
- **BUC-EMP-63 — Prove protected visibility upgrade and recovery.**
  Test V1 reads and frameV2 upgrade without canonical-byte changes, explicit
  refused dismissal/replay, wrong digest/state/source and corruption, generation
  changes, publication faults, process recovery and retained records/attachments.
- **BUC-EMP-64 — Prove the actual accessible app flow.**
  Exercise pending/detail/back/home in the existing context owner, truthful
  offline/manual-wait/receipt states and storage warning with meaningful
  Compose/ViewModel cases plus the owned API36 emulator. Validate normal DI,
  complete affected lint/locales and that no test producer enters product.


## Enterprise meter evidence (EMP-METER-001)

- **B-BR-EMP-131 — Preserve exact typed meter values.**
  ODOMETER uses metres in0..1000000000000 and HOUR_METER uses seconds in
  0..3600000000. A supplied positive int64 modulus further requires a value
  strictly below it. Canonical domain input uses ASCII digits and a dot,
  no sign/exponent/grouping/whitespace or redundant leading integer zero.
  Kilometres allow at most three fractional digits; hours at most two.
  Convert exactly without rounding. Hours-and-minutes uses separate canonical
  whole-hour and0..59 minute inputs, rather than an invented localized grammar.
  Missing observation carries a nonblank reason and remains distinct from zero.
- **B-BR-EMP-132 — Retain organization-bound evidence.**
  A meter/segment/reading and its asset, author membership, device, operation
  and optional photo references belong to the same supplied organization scope.
  Retain strict nonzero canonical identifiers, known kind/origin/method and
  validated declared instant, zone, offset and server receipt. These values
  do not authenticate an author, grant access or confirm a document upload.
  Reuse the accepted exact time kernel; no device clock acquisition. The
  reading does not own reporting-day fields: validate its time snapshot using
  UTC solely as the kernel validation calendar, without treating a device
  entry zone as the organization reporting zone or persisting that choice.
- **B-BR-EMP-133 — Derive deterministic current evidence.**
  Order by occurred_at, received_at, then canonical reading identifier.
  The latest reading is the latest VALID nonvoid/non-superseded observation
  in its exact meter and segment at or before the requested instant.
  Keep equal observations from different operation IDs. Reject duplicate
  evidence IDs and same-meter operation replay as a new fact. One parent
  operation may carry observations of different meter kinds. Supplied SUSPECT state is
  excluded, without implementing the future plausibility/confirmation policy.
- **B-BR-EMP-134 — Append corrections without rewriting observations.**
  Preserve original immutable reading evidence; append reason-bearing void or
  supersession facts. Void is terminal and each target is superseded at most
  once. A replacement has a fresh identity and the same owning scope, asset,
  meter, segment and kind; no cycle, duplicate ID, foreign link or second
  correction of a voided target. One fresh logical correction operation owns
  both its fresh replacement reading and its fresh supersession fact; those
  two parts share that operation ID. Reusing it for a separate later action
  is refused. A void similarly owns a fresh operation and fact. Reasons and
  missing-observation text follow the existing nonblank280-character metadata
  profile, without controls or invalid Unicode. Correction chains may continue through their
  distinct current replacements. Pure transitions grant no correction authority.
- **B-BR-EMP-135 — Keep usage within one real segment and kind.**
  Usage retains both actual latest-valid anchors at the requested endpoints
  and their exact difference, never interpolation. A missing anchor yields
  INSUFFICIENT_DATA/NO_READING_BEFORE rather than zero. Reject foreign/mixed
  meter, segment or kind subtraction and decreasing valid anchors. Odometer
  and hour-meter results remain independent. Reset, replacement, rollover,
  multiple segments and recalculation/persistence belong to later tasks.
- **B-BR-EMP-136 — Keep this checkpoint pure and bounded.**
  The first model includes one open segment per meter with observed opening
  evidence. The caller's supplied VALID/SUSPECT classification is not an
  automatic intake verdict. No route, database, runtime producer, fake meter
  presence, new dependency or guessed origin-method policy is introduced.
- **BUC-EMP-65 — Prove exact conversion and absence.**
  Shared Go/Kotlin vectors cover both documented conversions, zero, boundaries,
  modulus, excessive precision, malformed/overflow input and explicit missing.
- **BUC-EMP-66 — Prove immutable evidence and transitions.**
  Cover author/time/reference failures, all known origin/method values, duplicate
  identities/operations, supplied suspect exclusion, reason requirements,
  terminal void, one supersession, correction chains and original preservation.
- **BUC-EMP-67 — Prove deterministic series and usage.**
  Exercise all ordering ties/permutations, actual anchor selection, before-first
  insufficient data, foreign/mixed kinds/segments and worked examples B/E.
  Latest-valid must never return a voided or superseded observation.
- **BUC-EMP-68 — Keep both implementations conformant.**
  Strictly load one versioned shared corpus, detect unknown/missing/duplicate
  cases and compare exact results in Go and Kotlin. Run focused domain race,
  static/format and JVM regressions. No native/SQL acceptance is inferred from
  this pure-domain checkpoint.


## Enterprise pure fueling (EMP-FUEL-001)

- **B-BR-EMP-137 — Own the exact enterprise product catalog.**
  fuel-products-v1 contains exactly ETHANOL, GASOLINE_REGULAR,
  GASOLINE_ADDITIVED, GASOLINE_PREMIUM_GRADE, DIESEL_S10, DIESEL_S500,
  CNG, ELECTRICITY and ARLA_32 with the specification's fixed categories,
  units, storage units and families. Units use existing L/M3/KWH tokens;
  storage tokens are ML/MILLI_M3/WH and family tokens are ETHANOL, GASOLINE,
  DIESEL, NATURAL_GAS, ELECTRIC and NONE (additive has no fuel family).
  Cooking-cylinder LPG_P13 and unknown/normalized aliases are not products.
  Immutable catalog descriptors may supply Fleet ProductCatalog without
  duplicating the business list or changing public station/kernel vocabulary.
- **B-BR-EMP-138 — Validate a submit-ready candidate without accepting it.**
  Retain an immutable scoped fueling identity, asset, responsible EmployeeRef,
  author MembershipRef, occurrence declaration, product, quantity, paid total,
  explicit fill, payer and payment method. Private references belong to the
  same organization; public station references keep public ownership. A meter
  ReadingRef must match the exact org and asset, or an explicit Meter Missing
  reason is required. Reference shape does not prove existence, reading state,
  membership authority, approval, upload or server acceptance. Tank/tank-group,
  trip and supplier links remain optional as specified, never guessed.
- **B-BR-EMP-139 — Keep exact recorded amounts and unit semantics.**
  Reuse accepted canonical integer parsers/bounds: quantity1..1000000000 in
  the product's storage scale, paid/discount centavos0..1000000000000 and
  optional unit price1..1000000 milli-BRL per product unit. A supplied unit
  differing from the fixed product unit is PRODUCT_UNIT_MISMATCH. Preserve
  original optional price text without interpreting or quantizing it, and
  never recompute the entered paid total. Arithmetic warnings and derived
  prices belong to FUEL-002; do not use the L/M3-only reconciliation profile
  to reject KWH in this task.
- **B-BR-EMP-140 — Validate offline declarations without fictitious receipt.**
  A candidate has no received_at, clock acquisition, reporting-day snapshot
  or workflow transition. Expose a validation-only declaration entry from
  the existing time kernel, reusing exactly its instant/zone/offset checks
  and UTC normalization. Capture reuses those checks and preserves all its
  existing receipt/reporting behavior. Require declaration presence; preserve
  explicit minimum instants and offset/zone disagreement. Never invent a
  receipt or treat the entry zone as an organization reporting zone.
- **B-BR-EMP-141 — Preserve declared fill and payer.**
  FULL, PARTIAL and UNKNOWN are explicit; UNKNOWN remains recorded UNKNOWN
  with partial semantics. Quantity never implies FULL. ARLA_32 remains an
  additive with separate exact quantity/cost and no fuel-volume or consumption
  eligibility. A pure full/fuel eligibility fact is not an approved consumption
  anchor. Require COMPANY/EMPLOYEE and a documented payment method explicitly;
  those declarations do not prove payment, payer identity or reimbursement.
- **B-BR-EMP-142 — Preserve independent establishment and supplier roles.**
  Establishment is independently optional: public catalog station, private
  organization supplier, internal source tank or NOT_FOUND with a typed name.
  Destination asset tank/group and an internal establishment source are
  distinct references. Contracted supplier is a separate optional private
  reference. Document issuer belongs to DOC: represent it only in a separate
  read projection with its source document/revision, not as copied fueling
  truth. No stable issuer identifier exists in this source: the transient
  projection carries source DocumentRef, positive revision and DOC-provided
  issuer label, without inventing an issuer or SupplierRef identity. Its value
  validates declared ownership, not real document linkage or authority.
  NOT_FOUND names and known issuer labels are well formed, nonblank and
  control-free; add no numeric name cap absent from the specification.
  None of the three roles fills, replaces or corrects another.
  Missing public linkage is valid; no station, coordinate or fiscal status is
  created. Same-reference or different-reference roles both remain explicit.
- **B-BR-EMP-143 — Sum only explicitly compatible supplied quantities.**
  Retain per-product totals. Cross-product purchased totals require the same
  category and unit, never just a shared L token; additive and liquid fuel
  stay separate. Use exact widened sums without an invented total/count cap.
  This is purchased quantity, never consumption or a ratio. Snapshot mutable
  inputs before validation, preserve copies and redact private diagnostics.
  Keep this checkpoint pure, synthetic and dependency-free; runtime stores,
  intake, warnings, review, expenses, reimbursement and intervals remain owned
  by later tasks. Real-receipt legal gates remain closed.
- **BUC-EMP-69 — Prove the owned catalog and exact units.**
  Shared Go/Kotlin vectors cover all nine descriptors, every supported unit,
  wrong/unknown units/products, cooking-gas exclusion, exact bounds/precision,
  paid-total preservation and category-safe widened totals.
- **BUC-EMP-70 — Prove required fields and scoped evidence.**
  Cover missing declaration/employee/author/value/fill/payer/payment, foreign
  asset/meter/employee/private roles, observed-reference versus explicit
  missing reason, public station independence and immutable snapshots.
- **BUC-EMP-71 — Prove fill, payer and role independence.**
  UNKNOWN/large quantity never become FULL; additives never become fuel or
  anchors; employee payment never creates reimbursement. Preserve independent
  place, source document issuer/revision and contracted supplier, including
  absent/different references without automatic repair.
- **BUC-EMP-72 — Prove conformance and declaration-only validation.**
  Strictly consume the entire frozen corpus in both languages, with complete
  expected maps and exact counts/hash. Capture actual RED→GREEN before pure
  behavior; test mutation, failures and exact widening. Regress the accepted
  time kernel without altering receipt/reporting semantics. Run affected
  domain race/static/JVM checks; no device/SQL/runtime acceptance is inferred.


## Enterprise asset persistence (EMP-FLEET-002)

- **B-BR-EMP-144 — Persist an owned category source with assets.**
  Use private categories with stable IDs and explicit labels. An authorized
  asset create/change chooses an existing same-org category ID or explicitly
  declares a new nonblank/control-free Unicode label in that same transaction.
  Declare exactly one alternative. Unknown/foreign existing IDs are not found.
  No canonical category enum exists: add no guessed vocabulary, case-folding
  uniqueness or category-kind matrix. The persisted row supplies the accepted
  Fleet CategoryCatalog; an explicit new-label declaration reuses only an
  exact literal existing label in the same org. No trimming, case-folding or
  Unicode normalization is introduced. Use nonunique hash lookup plus full
  literal equality under the root lock if an uncapped label cannot fit a
  PostgreSQL unique index; a hash never proves category identity.
  Product descriptors come from real fuel-products-v1.
  Keep personal vehicle storage, limits and public station data separate.
- **B-BR-EMP-145 — Store actual private asset state.**
  Add organization-scoped assets, tanks/products, expected references, initial
  unit allocations and immutable command receipts with composite private keys,
  parameterized SQL and forced RLS. Preserve the accepted pure field/unit/
  Unicode/identifier rules. Partial unique indexes compare canonical plate and
  fleet number among assets not ARCHIVED; absent identifiers do not collide,
  serial is not newly unique and another organization has independent keys.
  Create is DRAFT; profile edits cannot set lifecycle. Revisions use exact CAS.
  Retain immutable full primitive profile/tank/expected/unit snapshots for
  each committed revision, beyond audit hashes. A read-only versioned SQL
  projection of the nine owned fuel-products-v1 descriptors supplies FK unit/
  product integrity, with mandatory parity to the code owner; no runtime
  edits or second fleet-owned product enum.
- **B-BR-EMP-146 — Recheck real current authority on every boundary.**
  Only source-owned fleet.asset.manage/read policies enable commands/reads.
  Reuse the currently registered organizations C1 capability and enterprise
  flag, with fresh company proof for configuration writes; do not invent a
  fleet flag with no actual activation owner or treat a plan as runtime proof.
  Use a narrow FleetAuthority transaction port. Preserve account/proof,
  capability, root and current membership/role lock order and recheck proof
  after waits and before receipt/state publication. Actor/delegation/session
  inputs are server-bound. SELF read needs actual assignment facts; until
  FLEET-006, no owner/creator fallback supplies them. Foreign records are hidden.
- **B-BR-EMP-147 — Activate only from actual admission facts.**
  DRAFT activation validates the existing pure minimum data, real current unit
  admission and actual meter source when powered. Open the first unit period
  atomically at server time; ACTIVE has one current unit. Draft creation/change
  may omit or clear a provisional unit. Activation uses the stored actual unit;
  ACTIVE profile change preserves it when omitted/empty and refuses a different
  nonempty unit, because transfer has its own task. An IMPLEMENT may
  activate without tanks/meters as specified. VEHICLE/MACHINE meter availability
  remains unavailable until the actual METER-002 provider is integrated: no
  synthetic MeterFact, guessed presence, fabricated opening reading or runtime
  always-none provider. Implement-only acceptance does not close the full task.
  Preserve real employee/team unit-use counts while adding actual asset use;
  unit archive and asset activation serialize under the existing root lock.
- **B-BR-EMP-148 — Preserve original command acknowledgement and audit.**
  Bind operation digest, org and verified actor to immutable original metadata
  acknowledgement. Recheck current authority before receipt access, including
  after target deletion. Same-body replay returns the original outcome with
  no second category/asset/allocation/audit; a changed command reusing its ID
  conflicts. Store receipt target selectors independently of the live asset FK
  so discarded-draft replay works; create replay never recreates that asset.
  Structural org/asset facts authorize initial ORG command entry without live
  target lookup; the Store proves current authority and owned receipt before
  requiring a live row. Discard ACK has discarded=true, no invented lifecycle
  state and last revision plus one as immutable receipt metadata.
  One successful command creates one audit event in the same transaction;
  category declaration is part of its parent asset command, not a second event.
- **B-BR-EMP-149 — Discard only proven unused drafts.**
  Discard only current DRAFT with no reading/accepted operation; retain receipt
  and audit history. Keep accepted-use source checks real and scoped, and kind
  immutable after actual use. A new unknown FK consumer makes that check
  unavailable until its owning source integrates, as in current unit-use
  guards. Only the owned lower-snake sync entity_type fleet_asset can count
  a generic committed effect as asset use; another entity type sharing a UUID
  is not provenance. The actual protocol is retained. Proven-unused draft
  discard may remove its configuration/revision rows by guarded parent cascade;
  immutable receipt/audit remain. Do not invent always-empty history ports.
  Failed/stale/foreign/
  dependency-denied commands change no profile, index, allocation or audit.
- **B-BR-EMP-150 — Expose bounded private management routes.**
  Add company-cookie/CSRF POST create/list/read/change/discard/activate under
  /v1/orgs/{org_id}/fleet/assets using accepted orgapi envelopes, exact typed
  quantities, expected revisions and bounded stable keyset pages. Scope cursors
  and data to the current actor/org/filter; preserve private no-store behavior
  and current proof/source guards. List/detail project stored actual state;
  no client lifecycle/availability field grants authority. Diagnostics/audit
  use IDs and allowed reasons, not labels, plates, source secrets or GPS.
- **B-BR-EMP-151 — Keep source staging and acceptance truthful.**
  Append migrations with populated recovery/ordinary-role tests and explicit
  grants/retention/export consumer integration as affected. Preserve existing
  applied migrations and source counters. Complete the tested fleet foundation,
  then the actual METER-002 dependency from that head if needed, and return to
  full powered-route acceptance before marking FLEET-002 LOCAL_DONE. During
  that dependency work keep this parent IN_PROGRESS and one implementation
  task active; a backed-up tested partial head is not whole-task/phase PASS.
  No PR/CI wait or fake provider resolves this integration order. Whole
  F2/M1/M2/C1, F3 and deployment/release gates stay separate.
- **BUC-EMP-73 — Prove actual category and record validation.**
  RED→GREEN create/change with explicit declared/existing categories and actual
  product descriptors, exact identifiers/tanks/expected references, invalid
  profile/lifecycle edits, foreign IDs and personal-data independence.
- **BUC-EMP-74 — Prove real database boundaries and recovery.**
  Ordinary PostGIS two-org RLS/grants, populated196→197 recovery and rollback,
  retained command/audit history, new consumer registration and real unit
  usage including unchanged employee/team counts; deny unscoped/foreign SQL.
- **BUC-EMP-75 — Prove current proof and role boundaries.**
  Cover anonymous/stale/ended/suspended/disabled actors and organizations,
  absent capability, wrong member/target, after-wait proof expiry, foreign
  receipts/data and SELF without actual assignment source. No token bypass.
- **BUC-EMP-76 — Prove concurrent original effects.**
  Exercise twelve-way duplicate identifiers, original replay/CAS, changed-ID
  reuse, create replay after discard, late publication failures, commit rollback
  and activation versus unit archive; exactly one allowed effect/audit wins.
- **BUC-EMP-77 — Prove actual routes before full closure.**
  Test company entry/CSRF/current source, all six contract paths and bounded
  pages through actual HTTP/ordinary source. IMPLEMENT activation proves its
  own path; powered positive acceptance uses actual METER-002 before closure.
  Compile/static/sqlc/contract/secret checks must match the tested source head.


## EMP-METER-002 — Source-backed immutable meter intake (2026-11-14)

- **B-BR-EMP-152 — Capture the actual observed source.** Readings retain an
  explicit scoped ENROLLED_DEVICE or WEB_SESSION capture locator. The panel
  installation uses its actual currently verified company session; it never
  manufactures an Android enrollment. Historic locators are immutable and
  contain no token/key/proof, and survive ephemeral session deletion. The
  accepted pure Device input remains enrolled shorthand; an explicit capture
  input is mutually exclusive with that shorthand. Canonical reading capture
  is always explicit. Author membership and operation remain mandatory.
- **B-BR-EMP-153 — Install real meters and opening evidence atomically.**
  An authorized fresh fleet.asset.manage command installs a meter on a real
  scoped asset with an explicitly observed opening value/instant/zone/offset,
  stable opening-reading ID, kind, label and optional positive modulus.
  Store one initial INSTALLATION segment and one actual opening reading in
  the same transaction; enforce one in-service kind per asset. No invented
  zero, missing opening value, GPS estimate or client authority/state/receipt
  fields. Preserve original command acknowledgement on exact replay. Bounds
  use the accepted pure domain; monotonic/plausibility flags stay METER-003.
- **B-BR-EMP-154 — Preserve insert-only scoped evidence.** Append migration198
  after published197, with forced RLS/composite organization references and
  insert-only reading protection for ordinary application/maintenance roles.
  No product UPDATE/DELETE or parent cascade may erase accepted readings.
  A real opening reading makes a draft used and prevents physical discard or
  kind change. Keep source-use guards and employee/team/current asset counts
  correct; unknown consumers remain unavailable until explicitly integrated.
- **B-BR-EMP-155 — Serialize and order actual observations.** Source/current
  authority precedes asset/meter locks. The retained caller transaction owns
  embedded readings; the embedded port cannot commit or begin its own work.
  Use the real DB receipt time and occurred_at/received_at/ID order. Distinct
  operations with equal values remain distinct; replay returns original
  evidence. Reject an observation outside its meter/segment service interval.
  Declared evidence retains all accepted nine fractional digits: persist the
  microsecond-floored PostgreSQL instant plus an explicit0..999 nanosecond
  remainder; reconstruct the exact instant and compare/order the tuple for
  reading/service/segment boundaries. Do not let PostgreSQL rounding change
  observations or silently narrow the accepted time profile. Server receipt
  precision remains the actual database clock. Latest-valid is a reproducible
  SQL query on the open segment, not a stored
  current-value truth or the future METER-006 maintained projection.
- **B-BR-EMP-156 — Keep operational source and scope truthful.** Standalone
  meter.reading.record accepts only actual signed current enrolled-device
  operations. Use its actual membership/enrollment and source binding, never
  payload author/device/received_at. Current source/record coverage is checked
  before claims, original receipt reads and final publication. SELF remains
  denied without the real FLEET-006 assignment source; changed-history/held
  review needs its actual producer and cannot be accepted using a dummy item.
  Panel meter management/history uses fleet.asset.manage/read respectively.
  Invalid-shape/unsupported-schema operations may obtain only an actual ORG
  base permission to record a bounded MALFORMED rejection: the explicit root
  target is org_id, never an invented asset. Valid payloads never downgrade
  a foreign/missing target to that fallback. ASSET/UNIT/SELF cannot inherit ORG
  coverage. Real target/reference and accepted-reading queries are read-only
  before claims and cached ACK publication; no record attempt substitutes for
  a lookup.
- **B-BR-EMP-157 — Declare only actual asynchronous obligations.** An exact
  source-owned operation/schema may explicitly declare terminal outcomes whose
  complete business effect is already atomic and needs no async follow-up.
  Default missing declaration/plans remain unavailable; required consumer
  catalogs/plans and enqueue-failure rollback retain their existing behavior.
  Never infer readiness from an empty catalog, install a no-op consumer or
  duplicate audit delivery evidence. A handler's explicit outcome set is
  validated and frozen by registry; undeclared outcomes cannot commit. The
  first meter handler declares accepted/rejected only, with atomic audit,
  immutable reading, effect and original ACK. Review remains unavailable until
  real flags/history/review source integration; future maintained projections
  and consumers require their owning acceptance before declaration changes.
- **B-BR-EMP-158 — Expose real bounded meter management.** Company-cookie/CSRF
  install and private list/latest reads under asset routes use exact integer
  strings and current scoped proof. No labels, values, source locators or proof
  secrets enter diagnostics/audit payloads. Optional document evidence remains
  unavailable until actual DOC reference admission; no dangling photo fixture
  is a production source. Extend actual route/permission contracts and tests.
- **B-BR-EMP-159 — Close the real dependency chain.** Consume tested fleet
  foundation6e3dde65. After real meters exist, update the powered SQL guard and
  Fleet activation to query actual in-service facts, test missing-meter422 and
  powered-positive200, then close FLEET-002 only when all required source cases
  pass. Keep unfinished assignment/review dependencies explicit; no partial
  evidence may impersonate full METER-002, F2/M1/M2/C1/F3 or release acceptance. After the tested meter source satisfies
  powered activation, FLEET-002 can close on its own actual route acceptance;
  METER-002 remains IN_PROGRESS until real FLEET-006 assignment/history and
  OFFL-008 review cases pass. FLEET-005 supplies actual temporal structure and
  AUTH-006 structural scope; OFFL-007 only captures clocks. Use the existing
  AUTH-005 role periods rather than inventing authority history. Continue one
  dependency-ready task at a time without a circular whole-task requirement.
- **BUC-EMP-78 — Prove capture provenance across both pure domains.** Shared
  Go/Kotlin RED→GREEN cases cover both true source kinds, zero/both/malformed/
  cross-org inputs, snapshots and privacy while retaining all old meter vectors.
- **BUC-EMP-79 — Prove ordinary database history and recovery.** Real PostGIS
  RLS/two-org reads and writes, immutable readings including direct SQL and
  parent deletion, populated197→198 recovery/previous-image compatibility and
  exact actual usage/grants, with no source/test bypass.
- **BUC-EMP-80 — Prove atomic exactly-once evidence.** Three identical ACKs/one
  row, equal-valued distinct operations, two enrolled devices in contention,
  embedded rollback, late commit/audit/outbox failure, actual opening receipt
  replay and latest-by-observation independent of arrival all receive tests.
- **BUC-EMP-81 — Prove real current permission and source denials.** Missing,
  foreign, ended/suspended/revoked/expired-after-wait proof, absent capability,
  unknown history/assignment and invalid service interval cannot leak evidence
  or leave claims. Actual signed HTTP and company routes use ordinary roles.
- **BUC-EMP-82 — Prove declaration and runtime closure.** Source-owned explicit
  atomic outcomes do not weaken required-consumer failures or permit missing/
  undeclared/review plans. Verify actual runtime routing/inventory/OpenAPI and
  powered Fleet dependency. Static/format/sqlc/secret and targeted regressions
  must match the final source before any whole-task checkpoint.


## EMP-OFFL-008 — Actual held review and durable author outcomes (2026-11-14)

- **B-BR-EMP-160 — Hold only source-proven field evidence.** Keep current signed
  account, ACTIVE member/employee, exact enrolled device and enabled operational
  capability mandatory. For the first actual Meter owner, a narrow disposition
  port evaluates complete real ORG permission periods retained by AUTH-005,
  with actual creation/effective/end times. Current candidates alone are not
  historical proof. Continuous current coverage across declaration and receipt
  may apply. A formerly effective grant with a gap/removal/regrant permits only
  a held PERMISSION_CHANGED fact, including a client declaration after removal;
  the client clock cannot prove historical absence. Complete server history
  proving no ever-effective ORG grant by receipt is FORBIDDEN. Scheduled grants
  never grant present authority; missing/unknown history is unavailable.
  A declared clock never grants a write. SELF/structural and F4 late/grace
  sources remain closed until their actual owners integrate.
- **B-BR-EMP-161 — Store actual review before acknowledging it.** Persist an
  immutable review item and typed Meter held fact in the original retained
  transaction, with exact IDs/value/declaration/capture and actual server source
  context. No reading/effect is applied while held. Bind operation/digest/reason
  and review ID by deferred composite database references; a UUID alone is not
  review provenance. Existing unsupported review ACKs retain NULL legacy proof
  version and immutable bytes; they remain unproven/unready. A validated MATCH
  SIMPLE composite FK plus a new-insert guard requires version1 and actual typed
  held source for every new REVIEW. Never fabricate a backfill from ACK alone.
  The production SQL source validator knows only delivered owners; a named
  test-only extension must prove its actual held row without weakening Meter.
  No raw transport credentials or arbitrary payload blob is retained. Review source failure rolls back claim/item/audit/ACK together.
  Every REVIEW proof failure, including the owner's final post-receipt check,
  is a review-source failure that withholds the whole signed response; it is
  never downgraded to a per-operation RETRY that exposes buffered receipts.
  A held fact reserves its exact reading identity and operation against
  ordinary readings in both directions. Its unit snapshot is retained history,
  not current unit usage, so it never blocks or bypasses unit archive checks.
- **B-BR-EMP-162 — Preserve original immutable acknowledgements.** An operation
  keeps exactly its original ACK/canonical bytes forever, including REVIEW.
  Resolution is separate immutable evidence, with one terminal resolution per
  item and expected item revision1→2. Add a source-owned atomic-review mode only
  when a real review proof port and SQL source constraints verify that item;
  existing NoAsync accepted/rejected behavior and mandatory-consumer plans stay
  unchanged. REVIEW alone never becomes applied dependency readiness.
- **B-BR-EMP-163 — Resolve once with current reviewer authority.** The first
  Meter decision uses current meter.reading.correct under dedicated company
  cookie/Origin/CSRF and fresh proof, an enabled capability and ACTIVE root.
  Reviewer list/detail remain current private reads; a write-disabled capability
  never authorizes a decision and does not invent loss of retained read rights.
  Check authority under the same transaction after waits and before publication.
  Refuse self-review by captured account/member/employee, following the actual
  canonical employee merge chain so rehire/merge cannot evade it. Accept the
  unchanged actual fact or refuse with a reason; editing fields is unavailable
  until an owning command explicitly implements it. Reasons reuse the existing
  private280-Unicode-scalar/control-free bound, preserved exactly; acceptance
  may omit a reason. Same command/body returns its original result; changed-ID
  reuse, stale revision, competing decision or source change leaves no effect.
  The server decision instant lies between the original receipt and the
  current database clock.
- **B-BR-EMP-164 — Apply reviewed evidence without retagging.** Accepted Meter
  resolution appends the actual reading with original author/enrollment,
  declaration and initial server receipt. The current resolver is a separate
  audit actor; never manufacture a session/device for the original author.
  Append migration199's narrowly review-bound insertion branch to198's live
  source guard, proven against exact held fact/accepted resolution. Ordinary
  inserts retain current source and server receipt checks. Publish readiness
  only with the matching accepted resolution and actual applied reading in
  one transaction; refusal/review cannot create effects or costs. The database
  refuses an accepted resolution committed without that readiness effect.
- **B-BR-EMP-165 — Minimize the author's own result read.** Add a separate signed
  own-operation result endpoint with request correlation/current exact source
  tuple and at most50 operation/digest selectors. Return one ordered explicit
  slot per selector, unresolved or an immutable resolution. Author ownership
  permits only this minimal receipt/outcome, never restored fleet.read/record
  or access to private record previews. Current member/device proof remains
  mandatory; ended/revoked sources and foreign/mismatched selectors fail closed
  with no buffered prefix exposed. Do not add fields to the frozen V1 batch or
  replace its ACK. New endpoint request64KiB/response128KiB/resolution4KiB bounds
  are explicit and tested for complete worst-case Unicode; no suffix omission.
- **B-BR-EMP-166 — Persist resolution separately in the protected client.** Add
  a versioned resolution value/projection and queue V3, preserving V1/V2 reads,
  original operation/ACK bytes, source tuple, retry metadata and dismissal.
  Parse/correlate the full response before local changes; CAS against the exact
  original review receipt and intent/source lease. Equal replay changes nothing;
  divergent terminal evidence is unavailable. Poll review-only retained work
  even when pending delivery is empty, using the same verified session/signature
  attempt and no lock across network. Never retag or auto-delete pending work,
  fabricate SENDING for a result lookup or invent a business projector. Retain
  separate review retry/Retry-After state; the existing15minute worker polls
  unresolved reviews without altering operation-delivery retry counts. Bind
  dismissal to the effective outcome so dismissing REVIEW cannot hide its
  later resolution. Production DI must bind the real review polling service.
  Chosen client semantics (EDR-120/121): a resolution is accepted only for a
  retained final REVIEW whose ACK bytes, ReviewID, operation and digest match
  and whose server decision time is not before the original receipt. A
  successful unresolved answer ends only that entry's failure backoff, keeping
  an authenticated Retry-After as an uncounted floor; a failed or uncorrelated
  answer writes no resolution and advances the review retry with the stored
  policy and Retry-After floor. A failed durable write or divergent stored
  decision affects only its own entry. A review-only failure during a manual
  check never hides committed delivery state. Old V1/V2 frames stay readable
  and are rewritten only when their state actually changes.
- **B-BR-EMP-167 — Show the actual decision.** Effective operation status/counts
  can reflect verified accepted/refused resolution while details retain the
  original REVIEW ACK and add decision/reason/server time. Do not change the
  original received_at, imply attachments are stored or reveal reviewer proof.
  Keep the eight locale catalogs consistent. Durable scoped completion triggers
  a reread; context/account/server/enrollment changes fence late responses.
  An unresolved REVIEW states that no company decision has been received; only
  a refused effective outcome (original REJECTED or REFUSED resolution) offers
  hiding, and accepted-after-review records with attachments stay pending.
- **B-BR-EMP-168 — Derive expiration from the actual review rule.** No current
  organization setting owns review expiry; intake-window settings do not apply.
  Stamp source-owned review-expiry-v1/default60days (elapsed60×86400seconds,
  independent of SQL session timezone/DST) at creation and derive
  expired-unresolved visibility from original receipt and server time. Retain
  evidence/listing and permit late resolution. This does not claim a digest,
  notification or scheduled escalation worker that has no actual consumer.
- **B-BR-EMP-169 — Complete real acceptance before dependent lifecycle.** Build
  from tested/pushed71b995ae (full Fleet-002 local, Meter source foundation).
  Ordinary PostGIS negative/replay/two-reviewer/rollback/upgrade/privacy tests
  are immediate. Real Go HTTPS and owned API36 emulator must prove durable
  review-only polling, process death/response loss, original ACK identity and
  source isolation. Full OFFL-008 cannot claim author-next-sync behavior from
  API-only tests. Preserve separate assignment/history, full F2/F3 and legal/
  provider/live release gates. No no-op review consumer resolves this dependency.
- **BUC-EMP-83 — Prove the shared immutable resolution protocol.** RED→GREEN
  Go/Kotlin exact shapes, Unicode/bounds/correlation/order/id+digest/review ID,
  invalid epochs/source and revision; retain all old operation/ACK vectors.
- **BUC-EMP-84 — Prove actual historical hold admission.** Real current source,
  complete role periods, continuous replacement versus gap/removal/regrant,
  never allowed and missing source; declared clock does not authorize acceptance.
- **BUC-EMP-85 — Prove real review persistence and decisions.** Two organizations,
  forced RLS, actual held source references, two reviewers/one result, stale
  revision, same-body replay/reuse, self-review/rehire/merge, source expiry,
  orphan review IDs, audit/outbox/late COMMIT failure and reviewed readiness.
- **BUC-EMP-86 — Prove provenance and recovery.** Retain original author/capture/
  declaration/receipt after source cleanup; fresh resolver proof cannot retag
  it. Populated198→199 failure/recovery/previous image and real personal export/
  erasure preserve corporate history without private disclosure.
- **BUC-EMP-87 — Prove durable client resolution.** V1/V2→V3, same/different CAS,
  full response preflight, process crash, pressure/dismissal, stale source and
  normal review-only Worker, with actual signature/nonce/HTTPS and two contexts.
- **BUC-EMP-88 — Prove visible outcome and delivery.** Accepted/refused status,
  reason/history, localized counts and unchanged attachment truth on the owned
  emulator; final code/static/SQL/format/secret checks match the committed inputs.

## Enterprise backend review corrections (2026-11-14)

Code review of the F1/F2 backend found five defects. These rules precede the
corrections; migration 000200 is append-only after the committed 184–199 chain.

- **B-BR-EMP-170 — Account erasure unlinks invitation acceptances.** In the
  existing erasure transaction, under each affected organization's own scope
  and after its membership tombstone, every invitation acceptance linked to the
  deleted account loses that account reference and its copied alias (empty).
  State, revision, deadlines, decisions and notices stay; no notice is invented.
  Only the migrator-owned erasure path for an actually deleted account may make
  exactly this change; every other update, a live account, a foreign scope or
  deletion is refused. Manager acceptance reads and replays of the manager's own
  original receipts return an empty alias with `account_erased=true`. Migration
  000200 applies the same unlink to accounts deleted before the upgrade. Actor
  attribution in immutable command receipts and organization audit events stays
  as the organization's accountability record (EDR-123); no manager read exposes
  it (BR-IDN-042, BR-PRIV-014, BR-TEN-009).
- **B-BR-EMP-171 — Readable lifecycle states stay visible as contexts.**
  SUSPENDED and ARCHIVED organizations remain listed and revalidated for their
  current members, with additive `state` and `suspension_reason` for the
  SCR-ORG-12 notice. Employee/invitation management hints and grantable options
  are omitted while the organization does not accept writes; reading hints stay.
  CLOSED or unknown states are not contexts. The capability is read without a
  lock only after the current membership is bound, so a non-member always gets
  NOT_FOUND; a current member keeps ENTITLEMENT_REQUIRED when it is disabled
  (BR-ORG-030, BR-ORG-033).
- **B-BR-EMP-172 — Deferred reservation cleanup uses the event's own scope.**
  The deferred cleanup that ends an unused INVITED reservation reinstalls the
  scope of the invitation row that queued it and restores the caller's scope,
  like the existing settings and owner-coverage checks. A caller that restores
  an empty or different scope before COMMIT, including account erasure, can no
  longer leave the reservation live. No definer or RLS bypass is added.
- **B-BR-EMP-173 — A scheduled owner is already an owner for a transfer.**
  Propose and accept refuse a recipient that has any unended OWNER period not yet
  expired, current or scheduled, with the existing target-already-owner conflict
  (409), and persist nothing. A role-period conflict found during the handover
  is a 409 conflict, never a retryable 503. Scheduled plans are not rewritten.
- **B-BR-EMP-174 — Expired admissions are materialized where they block.**
  Invitation issue, the invitation list and personal redemption first mark every
  WAITING admission of that organization whose confirmation deadline passed by
  the database clock as EXPIRED (CONFIRMATION_EXPIRED), ending its INVITED
  reservation (ADMISSION_ENDED) with its notice under the root lock. Admissions
  before their deadline do not change; no scheduler is introduced.
- **BUC-EMP-89 — Prove erasure privacy.** The real deletion service on ordinary
  roles and an ordinary definer, two organizations, WAITING/ACTIVE/confirmed
  admissions, unrelated acceptances untouched, receipt replay masking, every
  forbidden guard mutation, history deletion, and populated199→200 failure,
  recovery, pre-upgrade deleted-account unlink and previous-image compatibility.
- **BUC-EMP-90 — Prove context lifecycle.** Suspended/archived discovery and
  revalidation with state and reduced hints, closed exclusion, non-member
  indistinguishability with enabled/disabled capability, no wait on a held
  capability row, and the additive OpenAPI shape checked against real HTTP.
- **BUC-EMP-91 — Prove cleanup, transfer and expiry corrections.** Issuer erasure
  ends the invitee's reservation after scope restore; scheduled-owner propose and
  repeated accept return 409 without changes; expired admissions unblock reissue,
  list and acceptance while an admission before its deadline stays intact.

## EMP-PLAT-016 — Amendment kernel for approved records (2026-11-14)

[Evidence](../enterprise/evidence/f3-plat016-amendment-kernel.md).

- **B-BR-EMP-190 — Classify a correction by approval, never in place.**
  A pure server-side kernel decides one correction from supplied facts: the
  effective approved revision (0 when never approved), the latest revision,
  the latest revision's BR-REV review state or `VOIDED`, the reporting day and
  that day's period state. `EDIT` applies only while the latest revision is
  `DRAFT` or `RETURNED` (BR-AMD-006), including a draft or returned pending
  amendment, and appends latest+1 linked to latest. An `EDIT` of an approved
  record without an editable pending amendment is `APPROVED_IMMUTABLE`; a
  record under review, or a never-approved terminal one, is `NOT_EDITABLE`.
  `AMEND`, `VOID` and `ADJUST` need an approved record, otherwise
  `NOT_APPROVED`. Every command names the latest revision it saw; any other
  latest is `CONFLICT` and changes nothing. The consumer states its append
  plan: revision latest+1 for `EDIT`/`AMEND` and none for `VOID`/`ADJUST`,
  linked to latest for `EDIT` and to the effective approved revision
  otherwise. A plan that would overwrite a retained revision or link anywhere
  else is `LOST_ORIGINAL`; a gap or malformed fact is `INVALID`.
- **B-BR-EMP-191 — One reviewed amendment keeps the effective revision.**
  `AMEND` appends latest+1 with author, server receipt time and reason, linked
  to the effective approved revision, which stays effective. At most one
  amendment is pending (latest above effective in `DRAFT`, `SUBMITTED`,
  `IN_REVIEW` or `RETURNED`); another `AMEND`, a `VOID` or an `ADJUST` is
  `AMENDMENT_PENDING` until it is decided or cancelled, and its draft or
  returned revision changes only by `EDIT`. A review result names the pending
  revision: `APPROVED` makes it effective and supersedes, never deletes, the
  previous one; `RETURNED` keeps it pending; `REJECTED` and `CANCELLED` close
  it; in those three the previous approved revision stays effective. A result
  on any other revision, or without a pending amendment, is `CONFLICT`.
  Review transitions, reviewer authority and self-approval stay with the
  review lifecycle (BR-REV-003/004).
- **B-BR-EMP-192 — Void and locked periods preserve history.** `VOID` marks
  the approved record voided from its server receipt and retains the effective
  revision; voiding again, and any later correction or review result of a
  voided record, is `ALREADY_VOIDED`. A record whose reporting day is in a
  `LOCKED` period cannot be amended, voided or have an amendment approved:
  `PERIOD_LOCKED`. A `REOPENED` period qualifies only with a supplied reopening
  whose authority was proven at its server receipt and which has a valid
  reason; otherwise `REOPEN_UNAUTHORIZED` or the reason codes. Without a
  reopening, `ADJUST` posts a correction linked to the original record and its
  effective revision on a strictly later posting day whose period is writable
  (`OPEN` or a qualifying `REOPENED`). `ADJUST` while the record's period is
  writable is `PERIOD_NOT_LOCKED` (amend instead); a locked posting period is
  `PERIOD_LOCKED`. Unknown intent, review, period or result tokens are
  `INVALID`, never mapped to a known one.
- **B-BR-EMP-193 — Reasons, receipt authority and derived-data markers.**
  `AMEND`, `VOID` and `ADJUST` require a reason and `EDIT` may carry one. A
  reason is valid UTF-8, nonblank, control-free and at most 280 code points,
  the existing free-text bound; absent or blank is `REASON_REQUIRED`, otherwise
  malformed is `REASON_INVALID`. The consumer evaluates authority at server
  receipt (BR-TIME-003) and supplies it as facts: Edit for a never-approved
  record, Amend for an amendment, an edit of a pending amendment and an
  adjustment, Void for a void; a missing one is `UNAUTHORIZED`. An approved
  amendment, a void and an adjustment each return one deterministic marker
  naming the record, the revision derived data must now use or exclude, the
  superseded revision, the cause (`AMENDED`, `VOIDED`, `ADJUSTED`), the day and
  the time; consumers own recalculation jobs (BR-AMD-005). The kernel writes no
  audit event: each consumer records one per decision with its registered
  action and reason code, keeping the free-text reason on the revision
  (BR-AUD-001/004).
- **BUC-EMP-98 — Prove correction, conflict and review effect.** Shared
  vectors cover edits before approval and of a returned amendment, amendment
  of an approved record, in-place update, stale revision, lost original
  (overwrite, detached base, gap), pending amendment, never-approved amendment,
  approved/returned/rejected/cancelled results, replays after an applied
  decision, the reason profile and every authority fact.
- **BUC-EMP-99 — Prove void, locked-period and adjustment paths.** Shared
  vectors cover void and void of a void, `LOCKED` refusal, qualifying,
  unproven and reasonless reopenings, an adjustment linked to the original on a
  later writable day, adjustment refused in a writable record period, a locked
  posting period or an earlier posting day, approval refused after a relock,
  unknown period and review tokens, and every derived-data marker.

## EMP-APPR-001 — Review lifecycle component (2026-11-14)

[Evidence](../enterprise/evidence/f3-appr001-review-lifecycle.md).

- **B-BR-EMP-175 — Keep one closed single-level review lifecycle.**
  review-v1 owns exactly DRAFT, SUBMITTED, IN_REVIEW, APPROVED, RETURNED,
  REJECTED and CANCELLED, and the commands SUBMIT, CANCEL, RESUBMIT (author)
  and OPEN, APPROVE, RETURN, REJECT (reviewer). Allowed transitions are the
  BR-REV diagram for one C1 level: DRAFT to SUBMITTED or CANCELLED, SUBMITTED
  to IN_REVIEW or CANCELLED, IN_REVIEW to APPROVED, RETURNED or REJECTED, and
  RETURNED to SUBMITTED or CANCELLED. A decision is accepted only from
  IN_REVIEW. APPROVED, REJECTED and CANCELLED refuse every command. Any other
  pair is INVALID_TRANSITION. Tokens are exact uppercase; unknown, padded,
  lowercase, sync (SAVED_ON_DEVICE, ACCEPTED), local (READY_TO_SEND),
  approval-step (PENDING) or owner-extension (VOIDED) tokens are INVALID. A
  case starts as DRAFT revision 1 and is the fold of its history. Levels,
  partial amounts, substitutes, period locks, amendments and void stay with
  their owners.
- **B-BR-EMP-176 — Bind every command to the current revision, append-only.**
  Each command names a positive revision. A revision different from the
  case's current one is CONFLICT and yields no new case; the input case is
  unchanged. RESUBMIT names the RETURNED revision n and creates revision n+1;
  no other command changes the revision. Every accepted command appends one
  entry (command, resulting revision, resulting state, exact reason); earlier
  entries, including the decision on revision n and its reason, are never
  changed or removed. Replaying a stored history starts from DRAFT revision 1
  and must reproduce every entry exactly. An unreachable state, a skipped
  OPEN, a wrong revision or state label, or a reason breaking the reason rule
  is INVALID, so APPROVED cannot be restored without its server path.
- **B-BR-EMP-177 — Decide only with current permission and independence.**
  Every command receives server facts at receipt: the actor, the record
  author and an optional beneficiary as canonical person identities (sets of
  canonical lowercase non-nil account, membership and employee UUIDs, at
  least one, already resolved through the actual employee merge chain), plus
  the permission fact from the server authority port. An absent or false
  permission is FORBIDDEN. An author command whose actor shares no identifier
  with the author is FORBIDDEN. A reviewer command, OPEN included, whose actor
  shares any identifier of any kind with the author or the beneficiary is
  SELF_APPROVAL, whatever role or permission that person holds. The first
  failure wins in the order INVALID, REASON_REQUIRED, FORBIDDEN,
  SELF_APPROVAL, CONFLICT, INVALID_TRANSITION.
- **B-BR-EMP-178 — Require a bounded reason to return or reject.**
  RETURN and REJECT require a reason; APPROVE and CANCEL accept an optional
  one; SUBMIT, RESUBMIT and OPEN carry none. Reasons reuse the existing
  private 280-Unicode-scalar control-free rule of B-BR-EMP-163 and are
  preserved exactly, including surrounding spaces. An empty or
  whitespace-only reason is REASON_REQUIRED when required and INVALID when
  optional or not accepted. Invalid UTF-8, any control scalar or more than
  280 scalars is INVALID.
- **B-BR-EMP-179 — Never present local readiness as a server review state.**
  The review state shown to a person comes only from a server-confirmed
  case. Without one, the record shows DRAFT, not server-confirmed, and
  "ready to send" only for a pending local SUBMIT. With one, it shows that
  server state and flags any pending local author command as ready to send
  without advancing it. A local OPEN, APPROVE, RETURN or REJECT, or a local
  RESUBMIT or CANCEL without a server case, is INVALID: decisions are
  online-only and a device never holds SUBMITTED or APPROVED by itself. The
  BR-REV-009 counting basis is COUNTED for APPROVED, PENDING for SUBMITTED
  and IN_REVIEW (labelled as pending wherever included) and EXCLUDED for
  DRAFT, RETURNED, REJECTED and CANCELLED.
- **BUC-EMP-92 — Prove the lifecycle, revisions and reasons.**
  Shared Go/Kotlin review-v1 vectors cover every token, every allowed and
  every refused state/command pair (all seven commands from each terminal
  state), the return loop to revisions 2 and 3 with retained history, stale
  and future revisions, replay of valid and forged histories, and required,
  blank, over-bound, control and multi-byte reasons.
- **BUC-EMP-93 — Prove independence, permission and honest presentation.**
  Vectors cover the author or the beneficiary deciding (sharing only an
  account, only a membership or only a canonical employee), an author who
  holds the review permission, absent or false permission, author commands by
  someone else, malformed identities, the first-failure order and local
  readiness that is never presented as SUBMITTED or APPROVED.

## EMP-EXP-001 — Expense domain: origin, competence and company amount (2026-11-14)

[Evidence](../enterprise/evidence/f3-exp001-expense-domain.md).

## Enterprise pure expense (EMP-EXP-001)

- **B-BR-EMP-180 — One cost origin with one source identity.**
  An expense records exactly one origin from the closed cost-origin kinds
  FUELING, WORK_ORDER and DIRECT. FUELING references the scoped FUEL-001
  FuelingRef and WORK_ORDER a scoped work-order reference, both in the
  expense's organization; DIRECT is its own source, carries no separate source
  and is keyed by the expense identity. The origin key (organization, kind,
  source or expense identity) is the one-origin-one-cost identity of EDR-013
  and BR-EXP-001. The settlement, evidence and stock-movement kinds PAYMENT,
  REIMBURSEMENT, CARD_STATEMENT, ADVANCE, PAYABLE, DOCUMENT, STOCK_PURCHASE and
  STOCK_TRANSFER are recognized only to be refused as NOT_COST_ORIGIN; unknown,
  empty, padded or differently cased tokens are INVALID. A missing origin is
  INVALID, never defaulted. A source reference does not prove that the fueling
  or work order exists, was accepted or was approved.
- **B-BR-EMP-181 — Exact paid and company amounts without a false zero.**
  The paid amount (the actual amount, authoritative) and the company amount
  (the part the organization bears) are both required canonical integer
  centavos parsed by the accepted exact-value kernel: 0..1000000000000, with
  no sign, decimal point, exponent, grouping, whitespace or redundant leading
  zero. Decimal or otherwise non-canonical text is INVALID and never rounded.
  The company amount is at most the paid amount (COMPANY_EXCEEDS_PAID
  otherwise). Neither is recomputed from quantity and unit price nor defaulted
  from the other. An absent amount is INVALID and never becomes 0; a present
  known 0 is a recorded fact (a zero-paid fueling still has its one expense).
- **B-BR-EMP-182 — Structural category code now, configured catalogue later.**
  The category is a required code matching the repository key profile
  `^[A-Z][A-Z0-9_]{0,63}$`. It is validated structurally only, without
  normalization or case folding. Membership in the organization's configured
  catalogue, retirement, labels and origin-to-category consistency belong to
  EMP-EXP-003; the REQ-11.1 default names are not hard-coded here.
- **B-BR-EMP-183 — Competence instant and its reporting day.**
  The competence instant is a declaration validated by the accepted
  declaration-only time entry (bounded instant, IANA entry zone, integral
  device offset, UTC normalization). Its reporting day is the local calendar
  day of that instant in the supplied operating-unit zone, otherwise the
  organization zone (BR-TIME-004), computed by a receipt-free attribution entry
  extracted from the time kernel and reused unchanged by Capture. Zones come
  from the resolved operating context; an invalid unit zone never falls back
  to the organization; no server receipt, clock read or entry-zone
  substitution occurs. A cost belongs to its competence day, never to a
  payment or settlement date.
- **B-BR-EMP-184 — Approved cost basis with a separately identified pending part.**
  A pure totals function takes one organization scope, one half-open
  reporting-day period [start, end), an explicit basis and expenses that each
  carry a closed review state (the BR-REV states plus VOIDED). APPROVED company
  amounts form the approved cost; SUBMITTED and IN_REVIEW form the separately
  identified pending part; DRAFT, RETURNED, REJECTED, CANCELLED and VOIDED are
  excluded and only counted. The figure equals the approved cost under basis
  APPROVED and approved plus pending under INCLUDES_PENDING, which alone sets
  the "includes pending" flag. Sums are exact widened integers without a total
  or count cap. A foreign-organization entry is SCOPE, an entry outside the
  period is OUT_OF_PERIOD and a repeated expense identity or origin key is
  DUPLICATE_COST whatever its state; the first failing entry in input order
  decides and no partial total is returned. Settlement never enters the basis.
- **BUC-EMP-94 — Prove one exact expense per cost origin.**
  Shared Go/Kotlin vectors cover each cost-origin kind, each refused
  settlement/evidence/stock kind, missing, foreign and malformed origins and
  identities, exact amount bounds and non-canonical decimals, absent versus
  known zero, a company amount above the paid amount, the category profile,
  competence validation and reporting-day attribution by unit or organization
  zone across local midnight (VAL-19's 2026-10-31).
- **BUC-EMP-95 — Prove the reporting basis.**
  Shared vectors cover VAL-13's single 97 350 fueling cost with no second cost
  from a reimbursement or payment, VAL-25's pending exclusion and "includes
  pending" figure, all eight review states and their exclusion counts, exact
  sums above the single-record ceiling, an empty selection, a foreign
  organization, entries at and beyond both period bounds and duplicate origin
  keys or identities across states.

## EMP-DOC-001 — Document domain: container, independent statuses, links and allocations (2026-11-14)

[Evidence](../enterprise/evidence/f3-doc001-document-domain.md).

## Enterprise pure document domain (EMP-DOC-001)

- **B-BR-EMP-185 — Keep the original identity immutable.**
  A document is one organization's container identified by its scoped
  canonical document UUID and the original content digest, exactly 64
  lowercase hexadecimal characters (the EMP-PLAT-006 private object SHA-256
  format). No command changes the organization, identifier or digest. A
  replacement is a new document: it must belong to the same organization,
  have a different identifier and already be STORED; the replaced document
  must be STORED or QUARANTINED. The old document becomes REPLACED and records
  its successor, the successor records what it replaces, and both originals,
  links and allocations are retained. A digest match answers only whether a
  supplied digest equals the original. Content bytes, file names, MIME/size,
  storage keys and personal data never enter the domain; storage, limits and
  integrity stay with EMP-PLAT-006/EMP-DOC-002.
- **B-BR-EMP-186 — Six independent closed status dimensions.**
  Storage (UPLOADING, RECEIVED, STORED, QUARANTINED, FAILED, REPLACED,
  DELETED), extraction (NONE, PENDING, DONE, FAILED), structure
  (NOT_APPLICABLE, UNCHECKED, STRUCTURE_OK, STRUCTURE_INVALID), conference
  (UNCONFERRED, CONFERRED_OK, DIVERGENT, ILLEGIBLE), reconciliation
  (UNALLOCATED, PARTIALLY_ALLOCATED, RECONCILED, OVER_ALLOCATED) and fiscal
  consultation (NOT_CONSULTED, AUTHORIZED_AS_OF, CANCELLED_AS_OF, UNAVAILABLE)
  are closed exact token sets; a new document starts at UPLOADING, NONE,
  NOT_APPLICABLE, UNCONFERRED, UNALLOCATED and NOT_CONSULTED. Unknown,
  lowercase, padded, cross-dimension or "VALIDATED" tokens are INVALID.
  A command changes at most one dimension. No dimension is derived from
  another: a digest match, extraction, structure check, storage outcome,
  allocation or conference never sets conference or fiscal consultation
  (BR-TRUTH-001/002). Reconciliation alone is derived, from exact amounts.
- **B-BR-EMP-187 — Explicit transition commands with the expected state.**
  Each command names its dimension, expected current state and target state;
  a different current state is CONFLICT and changes nothing; a pair outside
  the table is INVALID_TRANSITION. Storage: UPLOADING→RECEIVED|FAILED,
  RECEIVED→STORED|QUARANTINED|FAILED, STORED→DELETED, QUARANTINED→DELETED,
  REPLACED→DELETED; REPLACED is entered only through replacement; FAILED and
  DELETED are terminal. Extraction: NONE→PENDING, PENDING→DONE|FAILED,
  FAILED→PENDING. Structure: NOT_APPLICABLE→UNCHECKED,
  UNCHECKED→STRUCTURE_OK|STRUCTURE_INVALID, STRUCTURE_INVALID→UNCHECKED.
  Conference: any state to any different state (decisions and explicit
  reversal to UNCONFERRED). Fiscal: any state to AUTHORIZED_AS_OF,
  CANCELLED_AS_OF or UNAVAILABLE, never back to NOT_CONSULTED; each fiscal
  result requires its consultation instant (shared bounded time profile),
  strictly later than the previous one (otherwise CONFLICT); other dimensions
  refuse an instant. A direct reconciliation transition is DERIVED.
  Self-transitions are refused except a later fiscal re-consultation.
- **B-BR-EMP-188 — Owned same-organization links.**
  A link targets one closed kind: FUELING, EXPENSE, WORK_ORDER, TRIP, ASSET,
  EMPLOYEE or METER_READING, with a canonical identifier of the document's
  own organization (BR-TEN-001); a foreign target is SCOPE, an unknown kind or
  malformed identifier INVALID, and the same kind and identifier twice
  DUPLICATE. Links keep insertion order. A link grants no content access,
  recognizes no cost and changes no status (BR-DOC-002/003).
- **B-BR-EMP-189 — Exact allocations bounded by the document amount.**
  The document amount is integer centavos 0..1 000 000 000 000 (financial-v1)
  and may be recorded or corrected without touching allocations. An
  allocation set replaces the previous one; it needs a recorded amount
  (AMOUNT_MISSING), at most 1 000 lines, and lines in order whose target is
  same-organization (SCOPE), allocatable (only EXPENSE, NOT_ALLOCATABLE
  otherwise), linked (NOT_LINKED), distinct (DUPLICATE) and 1..1 000 000 000 000
  centavos (INVALID). A set whose exact sum exceeds the amount is
  EXCEEDS_AMOUNT and changes nothing. Reconciliation derives from exact sums
  with zero tolerance: 0 → UNALLOCATED, below → PARTIALLY_ALLOCATED, equal →
  RECONCILED, above → OVER_ALLOCATED, which is reachable only when a later
  amount correction falls below recorded allocations. An even split uses the
  BR-MONEY-007 largest-remainder allocator with equal weights in target order
  and omits zero shares, so parts always sum to the amount.
- **BUC-EMP-96 — Track an original through independent checks.**
  Actor: the owning document workflow (intake, extraction, structure,
  conference, consultation). Flow: create the container from a scoped
  identifier and original digest → apply one named transition at a time with
  its expected state (and consultation instant for fiscal results) → read six
  separate statuses. Alternatives: stale expected state → CONFLICT, nothing
  changes; pair outside the table or terminal storage → INVALID_TRANSITION;
  digest match → only a boolean; a corrected file → a new stored successor
  replaces the old document, both retained. Result: each status says only
  what its own check established.
- **BUC-EMP-97 — Allocate a document amount to linked expenses.**
  Actor: the owning allocation workflow (reviewer authority and locking stay
  with EMP-DOC-007). Flow: record the exact amount → link same-organization
  targets → submit an explicit allocation set or an even split over linked
  expenses → read the derived reconciliation and remaining centavos.
  Alternatives: foreign, unlinked, non-expense, duplicate, zero, negative or
  overflowing line → refused; sum above the amount → EXCEEDS_AMOUNT with the
  previous set intact; amount corrected below allocations → OVER_ALLOCATED
  until a fitting set is recorded. Result: allocations never exceed their
  source when recorded, and the document example 97 350 = 60 000 + 30 000 +
  7 350 reconciles while an extra 8 000 is refused.

## EMP-BUDG-001 — Budget domain: lines, periods, dimensions and capacity (2026-11-14)

[Evidence](../enterprise/evidence/f3-budg001-budget-domain.md).

- **B-BR-EMP-198 — One line: organization, half-open period, one dimension and a required exact limit.**
  A budget line is a scoped line identity in one organization with an
  explicit half-open reporting-day period [start, end) built by the
  EMP-EXP-001 reporting period (any whole-day span; VAL-14's November 2026 is
  2026-11-01 to 2026-12-01), exactly one dimension and a limit. The limit is a
  required canonical integer number of centavos parsed by the accepted
  exact-value kernel (0..1000000000000). An absent, empty, signed, decimal,
  grouped, padded, "unlimited" or above-ceiling limit is INVALID. A present 0
  is a known zero limit under which no positive amount fits. There is no
  implicit unlimited budget and no default limit: without a constructed line
  there is no capacity and no decision, never ACTIVATE or POST. Overlapping
  lines of one dimension, line versions and limit changes are persistence and
  configuration concerns (EMP-BUDG-002/005).
- **B-BR-EMP-199 — Closed dimensions matched exactly against recorded tags.**
  Dimension kinds are exactly ORGANIZATION, UNIT, COST_CENTRE, ASSET and
  EMPLOYEE (the driver); unknown, empty, padded or differently cased tokens
  are INVALID. A line has exactly one kind: ORGANIZATION carries no target;
  every other kind requires one scoped target of the same kind in the line's
  organization; any kind may add one optional category filter (the EMP-EXP-001
  category code). Multi-target intersections are not representable in v1. A
  fact carries tags: the unit, cost centre, asset and employee stored on its
  record when it was created (BR-AUTH-005), at most one per kind, supplied by
  the caller; an expense's category is its own and a hold names one required
  category. A fact matches a line when it belongs to the line's organization
  (SCOPE otherwise, including any foreign tag or target), its reporting day is
  inside the period (OUT_OF_PERIOD otherwise) and, unless the line is
  ORGANIZATION, its tag of the line's kind equals the target exactly, while
  with a filter its category equals the filter (DIMENSION_MISMATCH otherwise;
  an absent tag or a same identifier under another kind never matches). A
  hold must also name the line itself (DIMENSION_MISMATCH otherwise). One fact
  may match several lines, such as the organization line and its cost-centre
  line; each line is computed on its own.
- **B-BR-EMP-200 — Available capacity derived only from authoritative facts.**
  Capacity is computed for one organization scope and one line from the
  caller's facts: consumption entries (an EMP-EXP-001 expense, its review
  state and its tags) and stored holds. Committed consumption is the sum of
  APPROVED company amounts (BR-BUDG-001, BR-EXP-002, BR-REV-009). SUBMITTED and
  IN_REVIEW company amounts form a separately identified pending part that is
  never deducted; DRAFT, RETURNED, REJECTED, CANCELLED and VOIDED are only
  counted. Held is the sum of ACTIVE hold amounts; CONSUMED, RELEASED and
  EXPIRED holds contribute nothing. Available equals limit minus consumed
  minus held in exact widened integers; a negative result is reported as
  over-commitment and is never wrapped or clamped. Consumption entries and
  then holds are checked in input order: INVALID (unconstructed value or
  unknown state), SCOPE, OUT_OF_PERIOD, DIMENSION_MISMATCH, then DUPLICATE (a
  repeated expense identity or origin key, or a repeated hold identity); the
  first failure decides and no partial capacity is returned. No operation
  takes an available, held or consumed number as input. A claimed (stored,
  cached or displayed) available is only compared: an equal canonical signed
  integer is current, a different one is STALE_AVAILABLE and a malformed one
  is INVALID.
- **B-BR-EMP-201 — Reservation and posting decisions; each hold closes once.**
  A reservation request (hold identity, named line, amount
  1..1000000000000, a nonzero reservation instant attributed to its reporting
  day by the accepted time kernel, category and tags) is checked against a
  computed capacity like a stored hold, plus DUPLICATE when its identity is
  already among the line's holds whatever their state, and is then decided:
  ACTIVATE with one new ACTIVE hold when amount ≤ available, otherwise EXCEEDS
  with shortfall = amount − available and no hold. A consumption posting
  takes one APPROVED expense entry of the line (NOT_APPROVED otherwise) that is
  not already among the facts (DUPLICATE) and optionally one ACTIVE hold of
  the capacity to convert (HOLD_NOT_ACTIVE when unknown or not ACTIVE).
  Headroom is available plus that hold's amount; the result is POST when the
  company amount is zero or at most the headroom, converting the hold to
  CONSUMED linked to that expense, otherwise EXCEEDS with the shortfall and no
  change. Neither decision applies an overrun policy or exception
  (EMP-BUDG-004): EXCEEDS never approves. Hold states are exactly ACTIVE,
  CONSUMED, RELEASED and EXPIRED. Only ACTIVE → CONSUMED (through a posting),
  ACTIVE → RELEASED and ACTIVE → EXPIRED exist; any transition from another
  state is HOLD_NOT_ACTIVE, so a double release or expiry, consuming a
  released, expired or consumed hold and reviving a hold are impossible. A
  stored CONSUMED hold requires its consuming expense and other states refuse
  one. The caller runs these pure decisions under the line's row lock
  (EMP-BUDG-002), so concurrent requests serialize against one capacity.
- **BUC-EMP-102 — Prove derived budget capacity.**
  Shared vectors cover line construction (period, every dimension kind and
  target, the category filter, the limit bounds, absent, empty, unlimited,
  negative, decimal and overflowing limits and the known zero), exact matching
  over every kind with absent, wrong, other-kind and foreign tags and
  categories, reporting-day bounds in organization and unit zones, all eight
  review states, every hold state and its consuming expense, duplicate facts,
  over-commitment above the single-record ceiling, VAL-14's
  500 000 − 450 000 = 50 000 and current, stale and malformed claimed
  available numbers.
- **BUC-EMP-103 — Prove serialized reservations and the hold lifecycle.**
  Shared vectors replay serialized sequences: VAL-14's two 30 000 holds (one
  ACTIVATE leaving 20 000, one EXCEEDS by 10 000), release and expiry restoring
  50 000 once, a retry after release, holds that do not fit, postings with and
  without a converted hold, zero postings on an over-committed line,
  non-approved and repeated postings, double release or expiry, consuming
  released, expired or consumed holds and a late retry that cannot revive a
  released hold.

## EMP-FIN-001 — Payables domain: supplier, reimbursement and card statement obligations (2026-11-14)

[Evidence](../enterprise/evidence/f3-fin001-payables-domain.md).

## Enterprise payables domain (EMP-FIN-001)

- **B-BR-EMP-194 — Closed obligation kinds with one counterparty and approved expense sources.**
  A payable is an amount the organization owes, recorded with exactly one kind
  from the closed list SUPPLIER, REIMBURSEMENT and CARD_STATEMENT; ADVANCE,
  PAYMENT, PAYABLE and every unknown, empty, padded or differently cased
  token are INVALID (advances belong to EMP-FIN-003). Each kind names exactly
  its own counterparty and no other: REIMBURSEMENT the beneficiary employee,
  SUPPLIER the supplier plus an optional supplier document, CARD_STATEMENT the
  statement; a missing or extra counterparty is INVALID. A payable has 1..1000
  source lines (REIMBURSEMENT exactly one); each line attributes an amount to
  one EMP-EXP-001 expense (its identity, origin key and company amount)
  together with the server-resolved facts the owning command supplies: the
  current review state and the explicit payer, COMPANY or EMPLOYEE with the
  paying employee present exactly for EMPLOYEE. The payable, its counterparty,
  every source expense and every paying employee belong to the scoped
  organization; any other organization, including a mix within one payable,
  is SCOPE. A repeated expense identity or origin key within one payable is
  DUPLICATE_SOURCE. A source is eligible only when APPROVED; REIMBURSEMENT
  requires an EMPLOYEE payer who is the beneficiary, and SUPPLIER and
  CARD_STATEMENT require a COMPANY payer (NOT_ELIGIBLE otherwise). References
  prove no existence, authority, supplier consistency or payment.
- **B-BR-EMP-195 — Exact obligation bounded by its sources.**
  Each line amount is a required canonical integer number of centavos parsed
  by the accepted exact-value kernel and at least 1; an absent, zero, signed,
  decimal, padded or out-of-range amount is INVALID and never rounded or
  defaulted. A line never exceeds its source's company amount
  (EXCEEDS_SOURCE). A REIMBURSEMENT equals the approved company amount exactly
  (PARTIAL_REIMBURSEMENT below it): the review decision, including a partial
  approval, already fixed what the company bears, and installments are
  partial settlements, not partial obligations. SUPPLIER and CARD_STATEMENT
  lines may be smaller than their source. The obligation amount is the exact
  sum of its lines, never an independent input, and at most the financial-v1
  total ceiling of 1000000000000 centavos (INVALID above). Checks run in this
  order, lines in input order within each: INVALID (shape, tokens, presence,
  amounts, ceiling), SCOPE, NOT_COST_ORIGIN, DUPLICATE_SOURCE, NOT_ELIGIBLE,
  then EXCEEDS_SOURCE or PARTIAL_REIMBURSEMENT. Recognizing an obligation never
  changes, re-recognizes or replaces its source expense.
- **B-BR-EMP-196 — Settlement state derived from exact settlement entries.**
  A settlement entry has a scoped identity and a positive canonical amount
  (INVALID otherwise) and belongs to one payable. Recording one checks, in
  order, scope and shape, NOT_COST_ORIGIN (its identity equals the payable,
  statement, a source expense or that expense's origin source), CANCELLED (the
  payable is cancelled), DUPLICATE_ENTRY (its identity is already recorded)
  and EXCEEDS_REMAINING (overpayment, including any amount on a SETTLED
  payable); a refusal leaves the payable unchanged. The state is derived,
  never stored as truth: no settlement is OPEN, a positive sum below the
  obligation PARTIALLY_SETTLED and a sum equal to it SETTLED. CANCELLED is the
  only state set by a command: an explicit cancellation with a nonblank,
  control-free reason of at most 280 Unicode scalars (the B-BR-EMP-163 bound,
  INVALID otherwise) moves an OPEN payable to CANCELLED; it is CANCELLED for
  an already cancelled payable and HAS_SETTLEMENT once any settlement exists
  (reversal is not modelled). The remaining amount is derived as obligation
  minus settled, and 0 for a CANCELLED payable. A stored or displayed state
  asserted against the facts must equal the derived state (STATE_MISMATCH;
  an unknown token is INVALID).
- **B-BR-EMP-197 — Obligations and settlements never become a cost.**
  A pure views function computes the cost view with the unchanged EMP-EXP-001
  totals over the cost entries alone (their own failures returned unchanged)
  and, separately, the settlement view over the payables alone: payable count,
  counts per state and exact widened sums of the obligation of non-cancelled
  payables, the settled amount and the remaining amount. Any number of
  payables and settlements leaves every cost figure identical, and no cost
  figure includes an obligation or a payment. A payable, settlement or
  statement identity used as a cost entry's expense identity or origin source
  is NOT_COST_ORIGIN, as are the settlement-side kinds already refused by
  EMP-EXP-001. Each expense is the source of at most one non-cancelled payable
  in a selection (DUPLICATE_SOURCE), a payable or settlement identity appears
  once (DUPLICATE_ENTRY) and every payable belongs to the scope (SCOPE). Cost
  failures come first, then payables in input order, then identity
  separation.
- **BUC-EMP-100 — Recognize and settle one obligation.**
  Shared vectors cover each kind with its counterparty and source rules,
  ADVANCE and PAYMENT refused as kinds, every review state and both payers,
  a wrong beneficiary, mixed organizations, duplicate sources, exact amount
  grammar, the source bound and total ceiling, partial reimbursement, partial
  and exact settlement, overpayment, settlement on a cancelled payable,
  cancellation with and without settlements, reason bounds, identities posing
  as costs and asserted states that contradict the derived state.
- **BUC-EMP-101 — Prove one cost with its settlement view.**
  Shared vectors cover VAL-13's 97 350 fueling with a matching reimbursement
  and payment (cost 97 350, payable SETTLED, never 194 700), a replayed
  payment, many payables of every kind and many settlements leaving the cost
  unchanged, a source outside the cost period, a second expense of the same
  fueling, payable/settlement/statement identities posing as costs,
  settlement kinds refused as cost origins, two live obligations of one source
  and a cancelled obligation releasing its source.

## EMP-PRIV-005 — Sensitive-access log for documents, exports and location (2026-11-14)

[Evidence](../enterprise/evidence/f3-priv005-sensitive-access-log.md).

- **B-BR-EMP-207 — Four closed sensitive categories, each with its own
  permission.** A sensitive read is exactly one of `DOCUMENT_CONTENT`
  (`doc.document.view_content`, target `document`), `EXPORT_FILE`
  (`rpt.export.run`, target `export`), `LOCATION_LIVE`
  (`gps.location.read_live`, target `trip`) or `LOCATION_TRACK`
  (`gps.track.read`, target `trip`). A capability-delivered category records
  `ISSUED` when it grants a short-lived capability and `SERVED` when content
  leaves the API; a live position is served directly, so `LOCATION_LIVE` has
  only `SERVED`. Registered actions are `document.access.issued` and
  `document.content.accessed` (the existing EMP-PLAT-006 actions, unchanged),
  `export.access.issued`, `export.content.accessed`, `location.track.issued`,
  `location.track.accessed` and `location.live.accessed`, all with reason
  `member.authorized`. Reading the owning trip, expense or document record
  never implies these permissions (BR-AUTH-011).
- **B-BR-EMP-208 — Check, record, then serve; fail closed.** The owning read
  first resolves the organization scope, current session, membership, target
  and the category's own permission. Only then does it record the access, and
  only after a committed record does it mint and return a capability or
  release content. A foreign or non-member request is `NOT_FOUND` and a member
  without the category permission is `FORBIDDEN` (BR-AUTH-013); neither
  writes a record. An expired session, an expired or revoked capability or a
  changed target serves nothing and writes no `SERVED` record. Any failure to
  record (invalid facts, unavailable storage, conflict or replay) serves
  nothing.
- **B-BR-EMP-209 — References and a digest only.** A record holds the
  organization, the verified actor account and its membership, the category,
  the stage (also implied by its action), target type and UUID, the server-generated
  request UUID, the capability (grant) UUID, reason `member.authorized`, a
  SHA-256 digest of these facts and of the owner's bounded binding digest,
  and server time. It never holds content bytes, file names, MIME types,
  storage keys, URLs, capability tokens, coordinates, filters or free text
  (BR-AUD-004, BR-PRIV-008, BR-GPS-039). Every field is a UUID, a registered
  code or a 64-hex digest, so such values cannot be stored.
- **B-BR-EMP-210 — One record per request, linked to its grant, append-only
  and isolated.** An organization holds one record per request UUID: an
  identical retry is stored once and refused as a replay, the same request
  UUID with other facts is a conflict, and neither serves. `ISSUED` and
  `LOCATION_LIVE` use the request UUID as the grant UUID; a `SERVED` record of
  a capability category must reference an `ISSUED` record of the same
  organization, category, target, actor account and membership. Records are
  append-only under forced organization row-level security. Organization
  roles with `audit.event.read` over the whole organization read them through
  the existing audit read service. No platform-operator or support-session
  reason is registered, so operator access stays impossible until
  EMP-BACK-003 adds approved support sessions (BR-AUD-006, BR-PRIV-021).
- **BUC-EMP-106 — Prove audited private document access.** On real PostGIS,
  the private document service issues a capability and serves content to a
  member holding `doc.document.view_content`, writing exactly one `ISSUED` and
  one `SERVED` record with the expected references. A foreign document or
  organization, a member whose role lacks the permission (also after a role
  change), an expired session, an expired or revoked capability and a refused
  record write serve nothing and leave no new record in either organization.
- **BUC-EMP-107 — Prove the log itself.** On real PostGIS, prove replay and
  concurrent duplicates store one record, conflicting reuse is refused,
  two organizations are isolated, rows are append-only, the database refuses a
  category that does not match its action, a served record without its
  member-bound grant, a sensitive event without its detail and a
  foreign-organization insert, a failed detail write
  leaves no audit event, the stored rows carry no content-shaped value, the
  audit read service lists only in-scope records for an authorized reader and
  refuses others, and the additive migration upgrades and recovers.

## EMP-FLEET-003 — Asset lifecycle: acquisition, deactivation, disposal and archive (2026-11-14)

[Evidence](../enterprise/evidence/f2-fleet003-asset-lifecycle.md).

## Enterprise asset lifecycle (EMP-FLEET-003)

- **B-BR-EMP-202 — Move the asset lifecycle only by named, audited commands.**
  Six private company commands change lifecycle data, each naming the asset
  and its expected current revision: record acquisition (`DRAFT`, `ACTIVE`,
  `INACTIVE`; state unchanged), deactivate (`ACTIVE → INACTIVE`), reactivate
  (`INACTIVE → ACTIVE`, BR-FLEET-004 checklist and unit admission re-proved
  from actual sources), dispose (`ACTIVE`/`INACTIVE → DISPOSED`), revert
  disposal (`DISPOSED → INACTIVE`) and archive (`DISPOSED → ARCHIVED`;
  `ARCHIVED` is terminal and accepts no further update). A stale revision or
  any other source state is `CONFLICT` with no effect. Each command requires
  current fresh `fleet.asset.manage` through the existing company pipeline,
  is serialized by the organization root lock and commits exactly one audit
  event, one immutable revision snapshot, one immutable command receipt and,
  for a state change, one immutable lifecycle history row in one transaction.
  The same operation from the same actor with the same canonical intent
  returns the first result (`replayed`) without a live source state; any other
  reuse is `OPERATION_ID_REUSED`. No field edit can set the state.
- **B-BR-EMP-203 — Validate reasons, instants and disposal data exactly.**
  Deactivate, dispose and revert disposal require a reason of 1..280 Unicode
  scalars, valid UTF-8, without control characters and not blank; reactivate,
  archive and record acquisition carry no reason field. Dispose names exactly
  one kind `SALE`, `WRITE_OFF` or `RETURN_TO_HOLDER` and an exact instant
  (RFC 3339 UTC, nanoseconds preserved as microsecond timestamp plus
  sub-microsecond digits); acquisition names one exact instant. Neither
  instant may be later than the server clock at the command; a disposal may
  not precede the recorded acquisition. Amounts, counterparts and expenses
  are not collected (BR-FLEET-007; sale revenue stays with expenses-finance).
  Any refusal here is `VALIDATION_FAILED` with no effect.
- **B-BR-EMP-204 — Keep disposal history; archive frees identifiers only.**
  The current disposal (kind, instant, reason) lives on the asset row only
  while `DISPOSED` or `ARCHIVED`. Revert disposal returns `INACTIVE`, clears
  the current disposal and keeps the original disposal row in the immutable
  lifecycle history, together with the revert reason; the asset can be
  disposed again. Archive keeps the disposal data, removes the asset from
  default lists and frees its plate and fleet number for a new asset (the
  existing non-archived partial unique indexes); it never deletes, renames or
  moves earlier records. Assets, revisions, meters, readings, held reviews and
  history of a disposed or archived asset remain readable by current
  `fleet.asset.read` holders. Unit allocation periods are not closed here
  (EMP-FLEET-005), assignments are not ended (EMP-FLEET-006 consumes the
  disposal) and reservations do not exist yet (EMP-FLEET-007).
- **B-BR-EMP-205 — Refuse removal from service with open work through a port.**
  Deactivate, dispose and archive ask an explicit open-work port, inside the
  command transaction after the revision/state check, which kinds of work are
  open (`TRIP` for a trip `IN_PROGRESS`/`PAUSED`, `WORK_ORDER` for an open
  maintenance work order). A non-empty answer refuses the command with
  `ASSET_HAS_OPEN_OPERATIONS` (409) whose details name each open kind once
  (`field=open_operations`, `code=<kind>`) and nothing else. Until the trip and
  maintenance epics exist the runtime wires the named `NoOpenWork` adapter,
  which answers "none" because no such records can exist; each epic replaces
  it with its real adapter and test. A missing port fails closed
  (`UNAVAILABLE`). A test-only adapter proves the refusal; it is never wired.
- **B-BR-EMP-206 — Hold a field reading declared at or after the disposal.**
  The signed `meter.reading.record` intake, after retained authority yields
  APPLY, reads the asset's current disposal under the organization root and
  asset row locks. A reading whose declared instant is at or after the
  disposal instant (exact to the nanosecond) is accepted for review with code
  `ASSET_UNAVAILABLE` through the existing OFFL-008 held mechanism: one review
  item, one held reading, a version-1 review ACK, no applied reading and no
  effect; replay returns the original ACK. A reading declared before the
  instant is handled exactly as before. `PERMISSION_CHANGED` keeps precedence
  when both apply. The database refuses a live reading at or after the
  disposal (23514) and an `ASSET_UNAVAILABLE` held fact without an actual
  disposal at or before its instant. After revert disposal there is no current
  disposal and readings return to normal handling; items already held stay
  held until reviewed. Readings on `DRAFT`/`INACTIVE` assets keep their
  current normal handling until derived availability (BR-FLEET-025/027)
  exists.
- **BUC-EMP-104 — Take an asset out of service or dispose of it (UC-FLEET-02).**
  A manager with fresh `fleet.asset.manage` records the acquisition instant,
  deactivates with a reason, reactivates, disposes with kind/instant/reason,
  reverts a mistaken disposal with a reason or archives a disposed asset,
  each through `/v1/orgs/{org_id}/fleet/assets/{asset_id}/…` with the asset
  revision; reads the asset (current acquisition/disposal) and its paged
  lifecycle history with `fleet.asset.read`. Every §6.1 row is proved from its
  source state and refused with `CONFLICT` from every other state, on real
  PostGIS with forced RLS, two organizations and ordinary runtime roles.
- **BUC-EMP-105 — Hold a post-disposal field reading for review.**
  A device reading for a disposed asset arrives through the signed intake:
  declared one nanosecond before the disposal it is applied; declared at or
  after it is held `ASSET_UNAVAILABLE` and appears in the existing meter
  review list for `meter.reading.correct`, whose accept/refuse decision is
  unchanged; after a revert a new reading is applied again.

## EMP-APPR-006 — Period closing, lock and reopening (2026-11-14)

[Evidence](../enterprise/evidence/f3-appr006-period-lock.md).

- **B-BR-EMP-211 — A period is one organization calendar month of reporting
  days.** A period is the calendar month `YYYY-MM` of one organization,
  covering the half-open reporting-day range from its first day inclusive to
  the first day of the next month exclusive. A record belongs to the period
  that holds its BR-TIME-004 reporting day: the local calendar day of its
  `occurred_at` in the time zone of its operating unit, or of the organization
  when it has none. In this task a period is organization-wide; unit or
  cost-centre scoped periods are not modelled. A month without history is
  `OPEN` at the implicit revision 1, which is never stored; only a canonical
  month from `0001-01` to `9999-12` is a period, any other key is not found.
- **B-BR-EMP-212 — Lock, reopen and re-lock append history.** Close
  (`fin.period.close`) moves `OPEN` to `LOCKED` (kind `LOCK`) or `REOPENED` to
  `LOCKED` (kind `RELOCK`). Reopen (`fin.period.reopen`) moves `LOCKED` to
  `REOPENED` and requires a reason of 1 to 280 Unicode scalars that is not
  blank and has no control character (the B-BR-EMP-163 profile, kept
  verbatim); close carries no reason. Every command names the revision it
  saw: another current revision is `CONFLICT`; closing a `LOCKED` period or
  reopening an `OPEN` or `REOPENED` one is `CONFLICT`; a missing reason is
  `REASON_REQUIRED` and a malformed one `REASON_INVALID`; a refusal changes
  nothing. An accepted command appends exactly one immutable transition at
  revision+1 with its kind, previous and new state, the permission that
  authorized it, the reason, the acting account and membership and the
  server receipt time, and writes exactly one audit event in the same
  transaction. Transitions are never updated or deleted, so a reopening never
  removes the earlier lock and a re-lock never removes the reopening.
- **B-BR-EMP-213 — Authority at receipt and idempotent replay.** Period
  commands and reads are decided at server receipt by the existing company
  pipeline: current company proof, enabled `organizations` capability, the
  organization root, the current membership and a current role covering the
  whole organization. Close and reopen need fresh authentication and an
  `ACTIVE` organization; list and read need `fin.period.close`. A role without
  the permission is `FORBIDDEN`; another organization or an ended membership
  is `NOT_FOUND`; a suspended membership or organization keeps its existing
  code. Replaying the same operation ID by the same account with the same
  canonical command returns the original result with `replayed=true` and
  writes no second transition or audit event; any other command or account
  under that operation ID is `OPERATION_ID_REUSED`.
- **B-BR-EMP-214 — The period gate runs inside the owning record
  transaction.** A record command obtains the period fact of a reporting day
  from a server port inside its own transaction: the gate first takes a
  key-share lock on the organization root, which every period command holds
  exclusively until it commits, then reads the current state. A concurrent
  close and record write therefore serialize: either the write commits while
  the period is not `LOCKED`, or the close commits first and the write sees
  `LOCKED`; a write never commits into a period that was `LOCKED` when it
  committed. The gate can attribute an `occurred_at` itself with the current
  organization zone or the zone of a unit of the same organization (an
  unknown unit never falls back). A `REOPENED` fact carries the proven
  reopening (permission, reason, actor and time). Mapped to the amendment
  kernel, `LOCKED` refuses an in-place amendment or void with
  `PERIOD_LOCKED`, a proven reopening allows it, and an adjustment posts into
  a later writable period.
- **BUC-EMP-108 — Finance closes, reopens and re-locks a month.** An OWNER or
  FINANCE member lists the organization's periods and reads October 2026
  (`OPEN`, revision 1), closes it naming revision 1 (`LOCKED`, revision 2),
  reopens it naming revision 2 with a reason (`REOPENED`, revision 3) and
  re-locks it naming revision 3 (`LOCKED`, revision 4). The read shows the
  three transitions with their actors, permissions, times and the reopening
  reason; the audit trail holds exactly three events. A stale, repeated or
  unauthorized command changes nothing.
- **BUC-EMP-109 — A record command consults the period at receipt.** For the
  VAL-19 fueling (`occurred_at` `2026-11-01T02:30:00Z`, zone
  `America/Sao_Paulo`) the gate attributes reporting day `2026-10-31`. With
  October `LOCKED` an in-place amendment is `PERIOD_LOCKED` and an adjustment
  posts into the open November; after an authorized reopening with a reason
  the amendment is allowed; after a re-lock it is refused again. A close that
  races the record write admits exactly one ordering.

## EMP-NOTIF-001 — Notification core: inbox, preferences, de-duplication, limits and history (2026-11-14)

[Evidence](../enterprise/evidence/f3-notif001-notification-core.md).

- **B-BR-EMP-215 — Register bounded reference-only notices in the producer transaction.**
  A producer registers notice requests through the orgnotify producer port
  inside its own audited orgevent transaction, bound to that source event and
  target. A request names a kind from the closed composition-registered
  catalog, one of the kind's template keys, exactly the kind's declared
  parameters and 1..50 distinct recipient memberships of the same
  organization; at most 10 requests per source event. Parameters are typed
  references only: canonical UUID, a code from the kind's enumerated list, a
  canonical count 0..1000000000 or a calendar date. No free text, attachment,
  document bytes, coordinates, contact data or token can be expressed; any
  undeclared, missing, malformed or oversized field is refused before writes.
  The optional panel link is rendered only from the kind's relative link
  template and its UUID parameters (no scheme, host, query or fragment); it is
  never a capability and its own route re-authorizes it when opened. A
  producer-supplied idempotency key de-duplicates: the same key with the same
  canonical content returns the first request; the same key with different
  content is refused as a reused operation; concurrent duplicates leave one
  request.
- **B-BR-EMP-216 — Dispatch once with current authority.**
  The `notif.dispatch` consumer runs from the existing job queue through
  orgevent (one receipt per event and consumer) and claims the event's
  PENDING requests atomically, so concurrent dispatch creates no second
  notice. Each recipient is evaluated at dispatch with current AUTH-003
  authority at database time: a non-ended membership bound to an active
  account, ACTIVE membership and employee, an organization that is not
  CLOSED and, when the kind names a permission, a live assignment whose role
  grants it. An ineligible recipient receives nothing. At most one notice
  exists per request and recipient, and a foreign recipient or organization
  is never addressed.
- **B-BR-EMP-217 — Respect preferences and bounds and keep delivery history.**
  A member may mute a non-mandatory kind for IN_APP; a mandatory kind cannot
  be muted and ignores preferences. Delivery is bounded to 20 notices per
  recipient and kind per UTC hour. Every evaluated recipient gets exactly one
  IN_APP history row: DELIVERED with its notice, or SUPPRESSED with
  INELIGIBLE, MUTED or LIMITED; nothing is silently dropped. IN_APP is the
  only active channel; Android local, push and e-mail stay disabled and are
  never contacted or recorded until EMP-NOTIF-002 and its delivery decision.
  FAILED is reserved for those external channels.
- **B-BR-EMP-218 — Read and change only the own current inbox.**
  Self routes use the dedicated company cookie, exact Origin and CSRF, and
  admit only the account-bound membership in the path when it is current
  (ACTIVE membership and employee, web-capable access model, organization not
  CLOSED); a foreign organization, another member or an ended membership gets
  the same NOT_FOUND, a suspended member MEMBERSHIP_SUSPENDED. The inbox is a
  newest-first keyset page bound to organization, account, membership, route
  and filter. Marking read is idempotent and preference changes use the
  expected revision. Each command carries an operation ID: the same ID and
  content replays the original result without a second change; a different
  content is OPERATION_ID_REUSED. Responses are no-store and carry only
  references, codes, counts, dates and the link path.
- **BUC-EMP-110 — Prove scoped once-only dispatch end to end.**
  With real PostGIS, ordinary roles and forced RLS, a test-only producer
  publishes through orgevent and the queue; the real handler dispatches to an
  authorized recipient with a panel link. Prove two-organization isolation,
  RLS without a scope, producer-port privacy refusals, idempotency-key replay
  and conflict, concurrent duplicate producers and dispatchers, ended
  membership and lost permission, muted and mandatory kinds, the hourly bound
  and exact history rows.
- **BUC-EMP-111 — Prove the member routes and the upgrade.**
  Through the real cookie plane, list, mark read and manage preferences as the
  current member; deny another member, another organization, an ended member
  and a missing CSRF; replay and reuse operation IDs. Rehearse the populated
  upgrade from the previous chain to 000204 with failure, recovery, retry,
  previous-chain compatibility and no seeded work. Keep the route/contract
  gate and authorization inventory exact.

## EMP-DOC-002 — Document persistence, upload session and audited private access (2026-11-14)

[Evidence](../enterprise/evidence/f3-doc002-document-persistence.md).

- **B-BR-EMP-219 — The document record owns the original identity.** One
  organization-scoped row persists each EMP-DOC-001 container: a
  client-declared document UUID, kind `RECEIPT`, data mode `SYNTHETIC`, the
  declared original (lower-case SHA-256, exact byte count, media type), the
  creating membership and operation, timestamps and the six status dimensions
  as independent columns. Storage, extraction, structure, conference and
  fiscal each carry their own revision, which grows by exactly one per change
  of that dimension and only along the EMP-DOC-001 transition graph from its
  current state; reconciliation is derived and stays `UNALLOCATED` until
  allocations exist. The private original's `orgmedia` identity is this row:
  its document UUID, digest, size and type come only from the row, its
  revision is the storage revision and it is ready only when storage is
  `STORED`. Rows start at the initial states, keep their identity, are never
  deleted and live under forced organization row-level security, so a session
  without the organization setting sees and writes nothing.
- **B-BR-EMP-220 — Upload session: declare, then verified bytes.** A member
  whose current `doc.document.attach` covers a record they author creates the
  session with an operation UUID, the document UUID and the declared original;
  C1 accepts only `image/jpeg` up to the 5 MiB budget. The document starts
  `UPLOADING` and the response carries an upload capability that expires
  within five minutes and is bound to the canonical origin, organization,
  document, account and membership. Repeating the same operation with
  identical facts returns the first record and a fresh capability while it is
  pending; the same operation with other facts or another actor is
  `OPERATION_ID_REUSED`, another operation naming an existing document UUID is
  `CONFLICT`, and concurrent duplicates write one row and one audit event.
  Bytes reach the document ingress with the capability; the server rechecks
  the creator, the current authority and the pending state, then runs the
  existing bounded `orgmedia` upload (declared size plus one, exact digest,
  type by content, conditional private write and verified read). Only after
  that does one transaction move storage `UPLOADING → RECEIVED → STORED` and
  record `document.upload.stored`. A refused, interrupted, oversized,
  mismatched or wrong-type transfer leaves the document `UPLOADING` with no
  event; a retry after `STORED` returns the stored record and writes nothing.
- **B-BR-EMP-221 — Content only through an audited, member-bound
  capability.** Opening content requires the current session, membership and
  `doc.document.view_content` covering the document (own records for `S`
  roles) on a `STORED` document. The `ISSUED` access record (EMP-PRIV-005)
  commits before a capability is minted; it expires within five minutes and is
  bound to the member and the document's verified revision. Reading presents
  that capability in the request body together with the same session and
  authority, and the `SERVED` record commits before the original is returned,
  base64-encoded in a no-store JSON body. Another organization's document,
  a non-member, another member's or organization's capability, an expired
  capability, a pending document and a refused or unavailable audit record all
  serve nothing and write no `SERVED` record; a member lacking the permission
  is `FORBIDDEN`, an unavailable access log is `UNAVAILABLE`, and every
  other refusal of a document is `NOT_FOUND` (BR-TEN-006, BR-AUTH-013).
- **B-BR-EMP-222 — Telemetry never carries content.** Logs, metric labels,
  error bodies, audit rows and redacted value formatting carry route
  templates, bounded codes and UUIDs only: never original bytes or their
  encoding, content digests, storage keys or URLs, or capability tokens
  (BR-PRIV-008). The declared digest is returned only to the authorized
  creator in its own no-store session response.
- **BUC-EMP-112 — Prove the upload session on real PostGIS and isolated
  storage.** A member declares a synthetic JPEG, uploads it through the
  capability and observes `STORED` with exactly one creation and one stored
  event; the object exists only under the organization prefix. Retried and
  concurrent creates keep one row, a replayed upload writes nothing, and
  unverified, oversized, mismatched or wrong-type bytes, a foreign or expired
  capability and another member's capability leave the document `UPLOADING`.
  Forced row-level security, the transition guard (equal to the domain graph),
  immutable identity and a populated upgrade from the previous head with
  failure recovery are proven.
- **BUC-EMP-113 — Prove audited private reads over the company API.** Through
  the composed enterprise routes with real sessions and roles, a permitted
  member receives a capability and reads the exact original with one `ISSUED`
  and one `SERVED` record, while foreign organizations, members without the
  permission, `S` roles on another member's document, expired or foreign
  capabilities and an unavailable audit serve nothing. Captured logs and
  metrics contain no bytes, digest, storage key or capability.

## EMP-FLEET-005 — Unit and cost-centre allocation history (2026-11-14)

[Evidence](../enterprise/evidence/f2-fleet005-allocation-history.md).

## Enterprise asset allocation history (EMP-FLEET-005)

- **B-BR-EMP-223 — Keep unit and cost-centre allocations as half-open,
  non-overlapping periods.** Each organization asset has two append-only
  period histories, one of operating units and one of cost centres. A period
  names one unit (or cost centre) of the same organization, a start and an
  optional end, and covers `[start, end)`: the instant of a change belongs to
  the new period. Boundaries are whole microseconds. Two periods of one asset
  in the same history never overlap; the database enforces this with an
  exclusion constraint, so of two concurrent overlapping writes at most one
  commits. A new period must reference a unit or cost centre that is `ACTIVE`
  at the write, of an asset that has left `DRAFT`, and may not start before
  the asset's first unit period. A period is never deleted or truncated; the
  only change is closing an open period once, with an end later than its
  start. Rows are private to the organization under forced row-level
  security; a session without the organization setting sees none and writes
  none.
- **B-BR-EMP-224 — An activated asset has exactly one unit at every instant
  from its activation.** The unit history of an asset that has left `DRAFT`
  starts at its activation instant (the instant of its FLEET-002 initial
  allocation), has no gap and ends in exactly one open-ended period; a
  `DRAFT` asset has none. Activation opens the first period in the same
  transaction, at exactly the initial allocation instant and with the
  asset's unit. Deactivation, reactivation, disposal, revert and archive
  neither open nor close a period: an out-of-service asset keeps its unit and
  reactivation keeps the one open period. A transaction that would leave the
  chain broken (closing the only open period without a successor starting at
  the same instant, a gap, a second open period, a first period not starting
  at activation) is refused at commit with no effect. The cost-centre history
  is optional: an asset may have no cost centre, or gaps, but at most one at
  any instant, and none is ever derived from the unit or invented. Archiving
  a unit or cost centre stays refused while an `ACTIVE` asset has a current or
  future period referencing it; ended periods and out-of-service assets do
  not block it (unchanged FLEET-002/003 semantics for units).
- **B-BR-EMP-225 — Move the existing single unit into the first period
  without inventing data.** Migration 000206 gives every asset that has left
  `DRAFT` exactly one open unit period whose unit is its single FLEET-002
  unit (the initial allocation row, which must equal the asset's unit column)
  and whose start and recording instant are the database instant at which
  that initial allocation was recorded at activation. `DRAFT` assets receive
  none (their unit is only a proposal) and no cost-centre period is created
  (no asset stores a cost centre). If any asset is inconsistent — a
  non-`DRAFT` asset without exactly one open initial allocation equal to its
  unit column, or a `DRAFT` asset with one — the migration fails as a whole
  and changes nothing. The original unit column and the initial allocation
  table stay unchanged until the next phase; the previous binary keeps
  working because activation still records the initial allocation, from which
  the database opens the first period. Rollback is the capability flag on the
  history routes.
- **B-BR-EMP-226 — Answer "unit and cost centre of this asset at this
  instant" from the periods.** The snapshot port
  `UnitAndCostCentreAt(organization, asset, instant)` (also offered inside the
  caller's transaction) returns the unit of the unit period containing the
  instant and the cost centre of the cost-centre period containing it, or no
  cost centre when none does. The instant is floored to the microsecond,
  which keeps the half-open rule exact because every boundary is a whole
  microsecond. A missing asset or an asset of another organization is not
  found; an instant not covered by a unit period (a `DRAFT` asset, or before
  activation) is unallocated, never a guessed unit. A later period change
  never changes the answer for an instant inside an unchanged period
  (BR-FLEET-016); the calling module stores the answer on its record at
  creation and coverage uses that stored value (BR-AUTH-005). The history
  reads list each history in start order with unit or cost centre, start,
  end when present and recording instant, under current `fleet.asset.read`
  covering the asset; `SELF` stays closed until assignments exist.
- **BUC-EMP-114 — Read the allocation history of an asset.** Actor: member
  with current `fleet.asset.read` covering the asset. Main flow: open the unit
  or the cost-centre allocation history of one asset → the server rechecks
  membership, permission and organization → a start-ordered keyset page
  (limit 1..100, default 20; opaque cursor bound to account, organization,
  membership, route, asset and limit). Alternatives: foreign or missing asset
  → `NOT_FOUND`; missing permission → `FORBIDDEN` naming `fleet.asset.read`;
  `DRAFT` asset → empty unit history; no cost centre ever → empty page;
  tampered or foreign cursor → `MALFORMED_REQUEST`. Result: read only, no
  effect.
- **BUC-EMP-115 — Snapshot the allocation when a record is created.** Actor:
  the owning module (trip, fueling, expense, reading, document) inside its
  own creating transaction. Main flow: call the port with the record's
  authoritative instant → store the returned unit and optional cost centre
  on the record. Alternatives: unallocated → the caller applies its own
  refusal or review rule and never stores a guessed unit; no cost centre →
  the record stores none. Example (07-permissions §10 example 3): vehicle V
  is in unit Norte until the change instant and in unit Sul from it; a
  fueling at an instant before the change stores Norte, the port still
  answers Norte for that instant after the change, and answers Sul at the
  change instant and later. Result: the stored snapshot never moves.

## EMP-RPT-001 — Asynchronous export infrastructure: job, private file and expiring link (2026-11-14)

[Evidence](../enterprise/evidence/f3-rpt001-export-infrastructure.md).

- **B-BR-EMP-227 — An export is one organization job, requested once.** A
  member whose current role covers `rpt.export.run` for the whole
  organization requests an export with fresh company proof, an `ACTIVE`
  organization and the `organizations` capability on. The kind must be
  registered in the closed format registry; a kind without an admitted format
  is `EXPORT_FORMAT_UNAVAILABLE` (422) and creates nothing. Parameters are a
  JSON object of at most 4 096 canonical bytes with lower-snake keys and no
  null. In one transaction the server stores one `QUEUED` job (requester
  account and membership, kind, canonical parameters and their SHA-256
  digest, the receipt instant as the "as of" instant, the format's row, byte
  and time ceilings capped by the infrastructure ceilings of 500 000 rows,
  16 MiB and 120 seconds, three attempts and a retention of 7 days), writes
  one `rpt.export.requested` audit event holding references and digests only,
  and enqueues one reference-only worker job carrying the organization and the
  job. The operation ID is the idempotency key: the same requester and
  command returns the stored job with `replayed=true` and writes no second
  job, event or queue entry; another command or account under it is
  `OPERATION_ID_REUSED`. A requester who already holds three `QUEUED` or
  `RUNNING` jobs gets `EXPORT_QUEUE_FULL` (429).
- **B-BR-EMP-228 — The worker rechecks authority and fails loudly.** Export
  work runs on its own serial dispatcher. The worker claims a job with a
  fenced lease, so exactly one holder works on it; a concurrent claimer finds
  it busy and retries later, and a `READY` or terminal job is left untouched.
  Before producing anything it re-evaluates the requester's current authority
  without a session: an active account, the same `ACTIVE` membership, an
  `ACTIVE` organization and a current role covering `rpt.export.run` at
  organization scope. Lost authority ends the job
  `FAILED` with `EXPORT_RIGHTS_LOST` and no file; a kind whose format is no
  longer registered ends `FAILED` with `EXPORT_FORMAT_UNAVAILABLE`. The format
  writes through a bounded sink: exceeding the row, byte or time ceiling ends
  `FAILED` with `EXPORT_TOO_LARGE` and never yields a truncated file. A
  transient failure releases the lease (`RUNNING` back to `QUEUED`) and is
  retried; a failure on the third attempt, or a lease that expired on the
  third attempt, ends `FAILED` with `EXPORT_ATTEMPTS_EXHAUSTED`.
- **B-BR-EMP-229 — Only a complete, verified private object becomes ready.**
  The file is stored create-only in private storage under
  `org/<org_id>/export/<export_id>/<sha256>`. Its digest, size and row count
  are recorded on the job as provisional under the lease before the upload;
  the media type is the admitted format's, fixed at request. The job becomes `READY`, with ready and expiry instants, only
  after the stored object has been read back and matches that digest and
  size, and only while the same lease still holds it. A failed, partial or
  substituted upload or a lost lease leaves the job not `READY`, a stored
  copy that does not match is deleted before the retry, and nothing that is
  not `READY` is ever served. Content is deterministic for the same
  parameters and "as of" instant, so a re-run converges on the same object;
  a different provisional object is deleted before another is recorded, so a
  job never publishes two files. The provisional object of a failed job is
  deleted.
- **B-BR-EMP-230 — Download is requester-bound, audited, short-lived and
  expires.** Only the requester's own membership can list, read or download
  a job; anybody else, including another member or organization, gets
  `NOT_FOUND`. Delivery goes through the API in two steps, each re-checking
  fresh current `rpt.export.run` authority and the same membership. Access
  requires a `READY` job before its expiry (`EXPORT_NOT_READY` 409,
  `EXPORT_EXPIRED` 410), records `EXPORT_FILE`/`ISSUED` through the
  sensitive-access log and only then returns a capability bound to the API
  origin, organization, account, membership, job and file identity that
  expires after at most 5 minutes. Content verifies that capability, re-checks
  authority and state, reads and verifies the private object, records
  `EXPORT_FILE`/`SERVED` and only then returns the bytes. Any audit failure
  serves nothing. No storage address or key reaches a client, a log or the
  audit trail, and no capability appears outside the access response. After
  the retention window the job becomes `EXPIRED` (also enforced at read time),
  keeps its metadata, is never downloadable again, and a scheduled cleanup
  deletes its object and confirms its absence before recording the deletion.
- **BUC-EMP-116 — A finance member exports and downloads a list.** A
  FINANCE member requests a synthetic export with an operation ID; the
  response is the `QUEUED` job and a retry returns the same job. The worker
  re-checks the member's authority, produces the file within its ceilings,
  stores and verifies the private object and marks the job `READY` with row
  count, size, SHA-256 and expiry. The member lists and reads the job, asks
  for access (one `ISSUED` record and a capability valid for at most 5
  minutes) and opens the content (one `SERVED` record and the exact bytes).
  Another member and another organization see nothing. After expiry the job
  is `EXPIRED`, the capability and the content are refused and the object is
  deleted.
- **BUC-EMP-117 — A failed or unsafe export publishes nothing.** The
  requester loses the role before the worker starts: the job ends `FAILED`
  with `EXPORT_RIGHTS_LOST` and no object. A format exceeds its row ceiling:
  `FAILED` with `EXPORT_TOO_LARGE`, no file. Storage refuses or corrupts the
  upload: the job is retried and stays not `READY` until a verified object
  exists, or ends `FAILED` after three attempts with its provisional object
  deleted. Two workers claim the same job concurrently: one produces, one
  object. An audit outage at access or content issues no capability and
  returns no byte.

## EMP-DOC-006 — Fiscal structure: access key, NFC-e QR code and XML structural validation (2026-11-14)

[Evidence](../enterprise/evidence/f3-doc006-fiscal-structure.md).

- **B-BR-EMP-231 — A structure check says only what it checked.**
  A fiscal structure check is pure and offline: it never contacts a provider
  or tax authority, never fetches the QR URL, never verifies a signature,
  authorization protocol, QR hash or CSC and never reads protNFe status codes.
  Its result has exactly one outcome — STRUCTURE_OK, STRUCTURE_INVALID (a
  pinned layout or bound is violated) or UNSUPPORTED (the input is not
  identified as a handled layout, version, variant, host or model; never
  reported as invalid) — one closed reason for a non-OK outcome, and the
  ordered checks that ran, the last one deciding a non-OK outcome. A non-OK
  result exposes no extracted value. The result has no authorization,
  cancellation, consultation or "validated" field. It feeds only the document
  Structure dimension, through the EMP-DOC-001 command
  UNCHECKED→STRUCTURE_OK or UNCHECKED→STRUCTURE_INVALID; UNSUPPORTED commands
  nothing (Structure stays UNCHECKED); fiscal consultation is never touched
  (BR-TRUTH-001/002, EDR-014, EDR-134). Comparing two STRUCTURE_OK results
  that carry keys gives STRUCTURE_OK on equal keys or STRUCTURE_INVALID
  KEY_MISMATCH (check KEY_MATCH); any other operand is NOT_COMPARABLE.
  String, log and JSON forms of keys and results are redacted.
- **B-BR-EMP-232 — Access key profile.**
  Exactly 44 ASCII characters: positions 1–6 digits, 7–18 digits or
  uppercase A–Z, 19–44 digits (the NT 2025.001 alphanumeric-CNPJ layout of
  18-references §3, AS-10). Check order (KEY_FORMAT): a character outside
  0–9/A–Z → KEY_CHARACTER; length other than 44 → KEY_LENGTH; position
  profile → KEY_CHARACTER. KEY_CHECK_DIGIT: modulo 11 over the first 43
  characters, each valued as its ASCII code minus 48, weights 2..9 repeating
  from the right; the digit is 11 − remainder, or 0 when the remainder is 0 or
  1; a different 44th character → KEY_CHECK_DIGIT. KEY_FIELDS: UF code outside
  the 27 IBGE state codes → KEY_STATE; month outside 01..12 → KEY_MONTH; model
  other than 55 or 65 → UNSUPPORTED KEY_MODEL_UNSUPPORTED (another document
  type sharing the layout). Nothing is trimmed, case-folded or reformatted. A
  STRUCTURE_OK key exposes UF, year-month, issuer field, model, series,
  number, emission type, numeric code and check digit as text. The issuer
  field's own CNPJ/CPF check digits, the emission type and the series/number
  ranges are not checked in a key. Because remainders 0 and 1 both give digit
  0, a key whose digit is 0 can collide with a mistyped key; a structure
  check is never identity or authenticity.
- **B-BR-EMP-233 — NFC-e QR code reference.**
  QR_SIZE: more than 2953 UTF-8 bytes (QR symbol capacity) → QR_TOO_LARGE.
  QR_URL: not starting with `http://` or `https://` (scheme ASCII
  case-insensitive) → UNSUPPORTED QR_NOT_URL; an authority (up to the first
  `/` or `?`) that is not a plain DNS host name ending in `.gov.br` with at
  least one label before it (ASCII case-insensitive; labels of 1–63 letters,
  digits or inner hyphens; at most 253 characters; no user info, port, IP
  literal or trailing dot) → UNSUPPORTED QR_HOST_UNSUPPORTED; then any
  character outside visible ASCII, a `#` fragment or a `%` escape that is not
  two hex digits decoding to visible ASCII → QR_MALFORMED. QR_PARAMETERS: no query parameter named exactly `p`
  (including the legacy version-1 `chNFe` query) → UNSUPPORTED
  QR_FORMAT_UNSUPPORTED; any other or repeated parameter →
  QR_UNEXPECTED_PARAMETER. The `p` value is percent-decoded once and split at
  `|`. QR_VERSION: fewer than two fields or a version other than `2` or `3` →
  UNSUPPORTED QR_VERSION_UNSUPPORTED; version 3 with other than three fields
  (its contingency variant) → UNSUPPORTED QR_VARIANT_UNSUPPORTED. QR_FIELDS
  (→ QR_FIELDS): version 2 online is key|2|environment|CSC id|hash and
  version 2 offline is key|2|environment|day|total|digest|CSC id|hash; any
  other version-2 field count is invalid; version 3 online is
  key|3|environment; environment is `1` (PRODUCTION) or `2` (HOMOLOGATION);
  CSC id 1–6 digits; hash 40 hex digits; day 01–31; digest 1–128 hex digits;
  total in the exact decimal profile of B-BR-EMP-234. Then the key rules of
  B-BR-EMP-232 on the first field, then QR_MODEL: model other than 65 →
  UNSUPPORTED QR_MODEL_UNSUPPORTED. A STRUCTURE_OK reference exposes key,
  version, environment, layout (ONLINE/OFFLINE) and, offline, the exact total
  text and its centavos. The hash, digest and day are never verified against
  anything and the URL is never opened.
- **B-BR-EMP-234 — Bounded NF-e/NFC-e XML envelope (server only).**
  XML_SIZE: more than 1 MiB (BR-PRIV-020) → XML_TOO_LARGE before any parsing;
  a leading UTF-8 byte-order mark is skipped. XML_PARSE is one strict
  streaming pass with no entity map and no charset conversion that stops at
  the first offending token: any `<!` declaration (DOCTYPE, ENTITY, external
  or internal subset, anywhere) → XML_DOCTYPE, never expanded; a processing
  instruction other than an XML declaration as the very first token →
  XML_INSTRUCTION; nesting deeper than 32 → XML_DEPTH; more than 100 000
  elements → XML_ELEMENTS; input not well formed, not UTF-8, with an
  undefined entity reference, non-blank text outside the root, no root or a
  second root → XML_MALFORMED. Then XML_ROOT: a root other than `NFe` or
  `nfeProc` in namespace `http://www.portalfiscal.inf.br/nfe` → UNSUPPORTED
  XML_ROOT_UNSUPPORTED. XML_ENVELOPE: `NFe` exactly once under `nfeProc`, and
  `infNFe` exactly once with exactly one `Id` starting `NFe` and one `versao`
  → XML_ENVELOPE. XML_VERSION: `versao` other than `4.00` → UNSUPPORTED
  XML_VERSION_UNSUPPORTED. The rest of `Id` follows B-BR-EMP-232. XML_FIELDS:
  exactly one leaf each of ide/cUF, cNF, mod, serie, nNF, dhEmi, tpEmis, cDV,
  exactly one ide, emit, total, total/ICMSTot and total/ICMSTot/vNF, at most
  one emit/CNPJ, emit/CPF and `protNFe`, and exactly one protNFe/infProt/chNFe when
  `protNFe` exists; a watched leaf containing an element → XML_FIELDS.
  XML_ISSUER: emit/CNPJ not exactly 14 characters accepted by the kernel CNPJ
  type → XML_ISSUER; emit/CPF without emit/CNPJ → UNSUPPORTED
  XML_ISSUER_UNSUPPORTED; neither → XML_ISSUER. XML_KEY_CONSISTENCY: the key
  differs from cUF, the dhEmi year-month (`YYYY-MM-` prefix), emit/CNPJ, mod,
  the canonical serie (`0|[1-9][0-9]{0,2}`) and nNF (`[1-9][0-9]{0,8}`)
  zero-padded to 3 and 9, tpEmis, cNF, cDV or protNFe chNFe →
  XML_KEY_MISMATCH. XML_ITEMS: `det` count outside 1..990 → XML_ITEMS.
  XML_TOTAL: vNF not exactly matching `0|0\.[0-9]{2}|[1-9][0-9]{0,12}(\.[0-9]{2})?`
  → XML_TOTAL. Values are compared as exact text, never trimmed. A
  STRUCTURE_OK document exposes the key, the item count and the exact total
  text; centavos come only from the existing exact parser within
  0..1 000 000 000 000 and are otherwise absent, never rounded. Signatures,
  protocol status, items, taxes and any other element are not checked.
- **BUC-EMP-118 — Read a scanned NFC-e QR code during capture.**
  Actor: the app capture flow (EMP-DOC-009) of an authorized field member.
  Flow: the scanned payload is checked on the device → STRUCTURE_OK offers
  the key, environment and, for an offline code, the exact total as
  suggestions labelled as structure checked → the member confirms or types
  the key, and a typed key is compared with the QR key. Alternatives:
  UNSUPPORTED says the code is not a handled NFC-e reference and keeps manual
  entry; STRUCTURE_INVALID names the failed check and keeps manual entry; a
  HOMOLOGATION environment is shown as a test document. Nothing is fetched
  and no fiscal status is shown. Result: a structure-checked suggestion or
  manual entry; the document fiscal consultation stays NOT_CONSULTED.
- **BUC-EMP-119 — Check a stored NF-e/NFC-e XML on the server.**
  Actor: server-side document processing (EMP-DOC-014 job; consumers
  EMP-DOC-007/008). Flow: a STORED XML original is moved
  NOT_APPLICABLE→UNCHECKED, checked within its bounds, and the result
  commands Structure → STRUCTURE_OK keeps the key (duplicate signal,
  EMP-DOC-008), item count and exact total (EMP-DOC-007) and may be compared
  with a captured QR or typed key. Alternatives: STRUCTURE_INVALID records
  the reason while the original stays preserved; UNSUPPORTED leaves
  Structure UNCHECKED; unsafe XML is refused without expansion. Result: the
  Structure status states only what was checked; fiscal consultation remains
  with its authorized port (EMP-INTEG-004).

## EMP-AUTH-006 — Unit, cost-centre, team and asset scopes with resource scope resolution (2026-11-14)

[Evidence](../enterprise/evidence/f2-auth006-structural-scopes.md).

- **B-BR-EMP-235 — Store unit, cost-centre, team and asset role scopes as
  same-organization references admitted only while the target is live.** A
  role assignment may be scoped to `ORG`, `SELF`, `UNIT`, `COST_CENTRE`,
  `TEAM` or `ASSET`. A structural scope names exactly one unit, cost centre,
  team or asset of the assignment's own organization; the database enforces
  this with a composite foreign key per type, so a target of another
  organization or an identifier of another kind is refused (`NOT_FOUND`). A
  new structural assignment needs an `ACTIVE` unit, cost centre or team, or an
  asset that is `ACTIVE`, `INACTIVE` or `DISPOSED`; a `DRAFT` asset (still
  discardable) or any `ARCHIVED` target is refused (`VALIDATION_FAILED`,
  BR-ORG-008). `OWNER` stays `ORG` only and station roles stay unavailable.
  Scope type and target never change after creation; a change is a new
  assignment (UC-AUTH-02). Overlap is refused per (membership, role, scope
  type, target) exactly as for `ORG`/`SELF`, and admission, archiving and
  overlap are serialized on the organization root lock. Archiving a unit,
  cost centre, team or asset neither ends nor is blocked by structural
  assignments (they are not "use" under BR-ORG-009); such an assignment keeps
  covering records that carry the archived structure (BR-ORG-012) and no new
  assignment can name it. Rows remain private under forced row-level
  security.
- **B-BR-EMP-236 — Decide structural coverage from the record's stored
  snapshot.** `UNIT`, `COST_CENTRE` and `ASSET` cover a record only when the
  unit, cost centre or asset that the server resolves for that record equals
  the assignment's target; a missing value never matches and a cost centre is
  never derived from a unit. A record that stores a snapshot is resolved from
  what it stored at creation, never from the asset's or person's current
  allocation (BR-AUTH-005): a held meter reading resolves its stored asset and
  unit and no cost centre. An asset itself resolves to itself and to the unit
  and cost centre the FLEET-005 port answers at the evaluated instant (a
  `DRAFT` asset has no unit). Organization-level resources (lists, roots,
  settings, role management, ownership, documents and periods today) carry no
  structure, so a structural assignment does not cover them. An own-records
  (`S`) descriptor under a structural scope requires both conditions: the
  record is the member's own and it carries the scope. Example 3 holds at a
  real entry point: a reading held while asset V was in unit Norte stays
  readable by a Norte-scoped role and not by a Sul-scoped role after V moves
  to Sul, while the asset itself becomes readable by the Sul-scoped role.
- **B-BR-EMP-237 — A team scope covers records of the team's employees at
  the record's instant; team membership alone grants nothing.** A `TEAM`
  assignment covers a record when the record's author or assignee employee had
  a link to that team containing the record's stored instant (half-open,
  server instants). Joining a team gives the joining member no access
  (BR-ORG-011); only an assignment naming the team does. A later link change
  never moves history: the old team keeps covering the records created while
  the link was current and the new team does not gain them. A record type
  that stores no author/assignee instant (assets today; assignments arrive
  with EMP-FLEET-006) is covered by no team; assets "assigned to a team" are
  not inferred.
- **B-BR-EMP-238 — Grant structural scopes without escalation.** Assigning or
  changing to a structural scope follows UC-AUTH-01/02 and BR-AUTH-007: the
  grantor needs a current `ORG`-scoped `auth.role.assign` and its current
  `ORG`-scoped role union must contain every descriptor of the granted role at
  organization coverage (a structural scope only narrows); a structurally
  scoped `auth.role.assign` covers no role management. An owner reducing an
  own assignment may only keep or narrow its scope (`ORG` contains every
  scope; any other scope contains only itself), never widen it. Commands keep
  the AUTH-002 idempotency: an exact retry returns the original result and a
  changed target under the same operation is `OPERATION_ID_REUSED`.
  Invitation grants and context hints stay `ORG`/`SELF`, and signed offline
  intake keeps treating any non-`ORG` assignment as unsupported (held for
  review), so no offline path is widened.
- **BUC-EMP-120 — Grant or change a structurally scoped role.** Actor: member
  with current `ORG`-scoped `auth.role.assign` and a fresh session. Main flow:
  choose member, role, scope (`UNIT`, `COST_CENTRE`, `TEAM` or `ASSET`),
  target, optional period and reason → the server rechecks proof,
  organization, permission and BR-AUTH-007, then the database admits the
  target under the root lock → assignment and audit event committed → the
  member's next request sees it. Alternatives: target missing or of another
  organization → `NOT_FOUND`; archived target or `DRAFT` asset →
  `VALIDATION_FAILED`; `OWNER` with a structural scope → `FORBIDDEN`;
  overlapping identical assignment → `CONFLICT`; replay → original result;
  same operation with another target → `OPERATION_ID_REUSED`. Result: an
  assignment that covers only records carrying its target.
- **BUC-EMP-121 — Read a protected record under a structural scope.** Actor:
  member whose only covering assignment is structural. Main flow: request a
  record → `Prepare` loads current authority; because a current structural
  assignment carries the permission, the route's structure source resolves
  the record's stored (or, for an asset, current) unit, cost centre, asset and
  team facts → coverage is decided on those facts → the owning transaction
  re-resolves them under its locks with database time before reading.
  Alternatives: no matching snapshot → `FORBIDDEN` naming the permission and
  no record data; record of another organization → `NOT_FOUND`; an
  organization-level list → `FORBIDDEN`. Result: the record is served only to
  holders whose scope matches what the record stored.

## EMP-DOC-003 — Links between documents and operations (2026-11-14)

[Evidence](../enterprise/evidence/f3-doc003-document-links.md).

- **B-BR-EMP-239 — A link is an owned same-organization reference.** One
  organization-scoped, insert-only row associates one existing document with
  one target record, identified by the EMP-DOC-001 target kind and the
  record's UUID; at most one row exists per document, kind and target. Only a
  kind whose owning module registered a target resolver is accepted — in C1
  `ASSET`, `EMPLOYEE` and `METER_READING` (a stored or a held reading, which
  share one identity); `FUELING`, `EXPENSE`, `WORK_ORDER`, `TRIP` and any other
  token are `VALIDATION_FAILED` and are never stored as dangling references.
  Each reference is a real same-organization foreign key to the record's own
  table, so a document or target of another organization, a missing record,
  or an identifier of another table presented under a kind is `NOT_FOUND`
  (BR-TEN-006) with no row and no event. A `DRAFT` asset, which may still be
  discarded, and a document whose storage is `FAILED`, `REPLACED` or
  `DELETED` refuse new links as `CONFLICT`. Links are never updated or
  deleted by a product flow and live under forced organization row-level
  security, so a session without the organization setting sees and writes
  nothing.
- **B-BR-EMP-240 — Authority is checked on both endpoints at receipt.**
  Creating a link requires, in the same transaction and from current server
  facts: the member's current `doc.document.attach` covering the document
  (own documents for `S` roles) and the target owner's own permission covering
  the target record, which the owning module declares and resolves —
  `fleet.asset.manage` for an asset, `meter.reading.record` for a reading
  (own readings for `S` roles, the author being the stored reading's member or
  the held operation's member) and `idn.employee.manage` for an employee. A
  positive owner permission is required before the target is resolved, so a
  member without it learns nothing about the record. A missing permission is
  `FORBIDDEN` with that permission's key. Listing a document's links requires
  `doc.document.attach` covering the document; listing a target's links
  requires `doc.document.attach` and the owner's permission covering the
  target. Neither list is filtered into a content grant.
- **B-BR-EMP-241 — Link creation is idempotent, audited and transactional.**
  The operation UUID is the idempotency key: an identical replay returns the
  first link; the same operation with another document, target or member is
  `OPERATION_ID_REUSED`; another operation naming an existing link returns
  that link with no new row or event; concurrent duplicates write one row and
  one audit event. Every new link writes exactly one `document.link.created`
  event targeting the document (BR-AUD-001). The document link port runs inside
  the consumer's own scoped transaction, so a fueling or reading and its
  receipt link commit or roll back together.
- **B-BR-EMP-242 — A link conveys neither content nor cost.** A document that
  is still `UPLOADING` can be linked and stays pending (BR-OFFL-015); a link
  shows only the document's storage status. Opening content still requires
  `doc.document.view_content` through the audited EMP-DOC-002 capability
  (BR-PRIV-004, BR-AUTH-011): the target permission never grants content, and
  a pending document has no content. A link carries no amount: it never
  allocates money, changes reconciliation, fiscal or any other status or
  revision of the document, or creates an expense (BR-DOC-003, EDR-013).
- **BUC-EMP-122 — Prove links against real records on PostGIS.** With real
  assets, stored and held meter readings, employees, documents and current
  role assignments, a member links a pending receipt to each registered kind
  through the store and through the port inside a consumer transaction; the
  replay, the duplicate under another operation and twelve concurrent
  duplicates keep one row and one event; foreign, missing, forged-kind,
  unregistered-kind and draft-asset references and members lacking either
  endpoint's permission leave no row; the schema refuses unscoped, foreign,
  mismatched-binding, eventless and mutated rows, and a populated upgrade from
  the previous head recovers from a failed migration.
- **BUC-EMP-123 — Prove the link routes over the company API.** Through the
  composed enterprise routes with real sessions and roles, a member links an own
  pending receipt to an own reading and a manager links an own receipt to an
  asset; the lists by document and by target page through sealed cursors;
  refusals carry the bounded envelope and change nothing; a member who can see
  the link but lacks content permission over the document, and anyone asking
  for a pending document's content, read nothing; the document's statuses and
  revisions are unchanged by linking.

## EMP-AUTH-011 — Auditor read-only role and export permission gating (2026-11-14)

[Evidence](../enterprise/evidence/f3-auth011-auditor-export-gating.md).

- **B-BR-EMP-247 — The auditor reads and never writes.** The fixed `AUDITOR`
  role holds exactly the documented set of `org-permissions-v4`:
  `idn.employee.read`, `org.settings.read`, `fleet.asset.read`,
  `trip.trip.read`, `fuel.fueling.read`, `exp.expense.read`,
  `kpi.dashboard.read`, `audit.event.read`, `gps.track.read` (recorded tracks
  only, never `gps.location.read_live`), `doc.document.view_content`,
  `rpt.export.run` and the personal consent key `org.ownership.respond`; every
  other key is denied. On every routed organization route an AUDITOR request
  is one of: a documented read (success, nothing changed); an audited
  sensitive or export read under its own permission (B-BR-EMP-248..250); a
  consent attempt that the transaction participant check confines to a
  proposal addressed to the auditor (any other proposal `NOT_FOUND`); or
  `FORBIDDEN` with the route's permission key, no record read and no private
  state changed. Every write route other than the export request and the own
  ownership response is refused. A record of another organization, or an
  organization without the auditor's membership, is `NOT_FOUND`
  (BR-TEN-006). Finance period list and read stay under `fin.period.close`
  (EDR-149), so the auditor has no period read until a matrix revision adds
  a read key.
- **B-BR-EMP-248 — An export needs both its own and the data's permission.**
  Requesting an export needs fresh `rpt.export.run` covering the whole
  organization and, in the same transaction, the read permission declared by
  the export kind, held now by the same ACTIVE membership for the whole
  organization; neither implies the other, and a structural or own-records
  read grant does not cover an organization-level export. Every admitted kind
  declares exactly one registered `<module>.<resource>.read` key outside
  location (`gps.*`); a kind that declares `rpt.export.run`, any non-read key
  (including `doc.document.view_content`), a location key or an unknown key
  is not admitted. A missing data permission is `FORBIDDEN` with that key,
  and no job, audit event or queue job is written.
- **B-BR-EMP-249 — The data permission is re-checked at every later step.**
  Each worker attempt re-evaluates the recorded requester's `rpt.export.run`
  and the kind's declared read permission; losing either (role ended or
  changed, membership suspended or ended, organization not ACTIVE) ends the
  job `FAILED` with `EXPORT_RIGHTS_LOST` and no file. Issuing a download
  capability and serving the file re-check both in the delivery transaction
  (BR-AUTH-014): a requester who lost the data permission is `FORBIDDEN` with
  that key and no `EXPORT_FILE` record is written; a ready job whose kind is
  no longer admitted is `EXPORT_FORMAT_UNAVAILABLE`. Job metadata (list and
  read of the requester's own jobs) needs `rpt.export.run` only.
- **B-BR-EMP-250 — Sensitive bytes need their own permission whatever else is
  held.** Document content is served only by the content routes under
  `doc.document.view_content` and is recorded as `DOCUMENT_CONTENT`
  `ISSUED`/`SERVED` (EMP-PRIV-005) for every holder, AUDITOR included. A
  member with broad read permissions but without that key (for example
  SUPERVISOR) is `FORBIDDEN` with `doc.document.view_content` and no access
  record is written. Because no export kind may declare content or location
  (B-BR-EMP-248), no general read or export path reaches document bytes, live
  positions or tracks.
- **BUC-EMP-126 — Audit an organization read-only.** Actor: member with an
  organization-scoped AUDITOR assignment. Main flow: read employees,
  settings and assets → open a document's content (recorded) → request an
  export of an admitted kind whose read permission the auditor holds →
  download it (recorded). Alternatives: any other write route → `FORBIDDEN`
  with its key and nothing changes; a record or organization outside the
  membership → `NOT_FOUND`; an ownership proposal addressed to someone else →
  `NOT_FOUND`; period list/read → `FORBIDDEN` `fin.period.close`; a
  UNIT-scoped auditor → `FORBIDDEN` `rpt.export.run` for exports. Result: the
  auditor sees what the matrix grants and changes nothing.
- **BUC-EMP-127 — Gate an export by the data's permission (system).** Actor:
  requester and export worker. Main flow: request → `rpt.export.run` and the
  kind's read permission checked in the request transaction → job `QUEUED` →
  each worker attempt re-checks both → `READY` → access and content re-check
  both and record `EXPORT_FILE`. Alternatives: no `rpt.export.run` (a
  read-only role or a UNIT-scoped grant) → `FORBIDDEN` `rpt.export.run`; no
  data permission → `FORBIDDEN` with that key, nothing written; role or data
  permission lost before the attempt → `FAILED` `EXPORT_RIGHTS_LOST`, no
  file; data permission lost after `READY` → `FORBIDDEN` on access/content,
  nothing recorded. Result: an export never widens what its requester may
  read now.

## EMP-FLEET-006 — Driver assignments: primary, temporary and multiple drivers (2026-11-14)

[Evidence](../enterprise/evidence/f2-fleet006-driver-assignments.md).

- **B-BR-EMP-243 — An assignment names one asset and one active employee
  record for a half-open period with a derived state.** An assignment belongs
  to one organization and names one of its assets, one of its employee
  records and a kind: `PRIMARY` (the responsible), `ADDITIONAL` (another
  allowed user) or `TEMPORARY` (a substitute; it requires an end). Its period
  `[start, end)` has whole-microsecond bounds; the end is optional except for
  `TEMPORARY`. A start omitted by the command is the server instant of the
  command; a start given by the command must be later than that instant; an
  end must be later than the start. The state is never stored: at a server
  instant it is `SCHEDULED` before the start, `ACTIVE` inside the period and
  `ENDED` from the end on; an assignment ended before it started keeps an
  empty period (`end = start`) and is `ENDED`. Only an `ACTIVE` employee record
  and an asset in `ACTIVE` or `INACTIVE` can be named when the assignment is
  created; a `DRAFT`, `DISPOSED` or `ARCHIVED` asset or a non-`ACTIVE`
  employee is refused with `VALIDATION_FAILED`, and an asset or employee of
  another organization (or unknown) is not found. An optional reason has at
  most 280 characters. Rows are private to the organization under forced
  row-level security (a session without the organization setting sees and
  writes none) and are never deleted; the only change is ending an assignment
  that is not `ENDED`, which moves its end earlier (never later, and never
  before the change except to an asset's disposal instant) and records the
  cause of that ending. Driver-profile
  eligibility (EMP-IDN-014), reservations and team pools are not part of an
  assignment; an assignment reserves nothing and ending it does not end a trip.
- **B-BR-EMP-244 — One primary at a time; a command creates all or nothing;
  replacing and ending are atomic and idempotent.** Two `PRIMARY` periods of
  one asset never overlap: the database enforces it with an exclusion
  constraint, so of two concurrent overlapping primaries exactly one commits
  and the other is refused with `ASSIGNMENT_OVERLAP` and leaves nothing. A
  create command names one asset, one kind, one period and 1 to 100 distinct
  employee records and creates one assignment per employee or none; a
  `PRIMARY` command names exactly one employee. A `PRIMARY` command marked
  "replace" ends, in the same transaction, the primary whose period contains
  the new start, at the new start (cause `REPLACED`); a primary that starts
  later still refuses with `ASSIGNMENT_OVERLAP`, and with nothing to replace
  the command simply creates. An end command ends one assignment of the asset
  that is not `ENDED` at the server instant at that instant (at its start when
  it has not started), with cause `ENDED`; ending an `ENDED` assignment is
  refused with `CONFLICT` and changes nothing. Every write records exactly one
  audit event and one receipt bound to the operation identifier; the same
  intent replayed returns the first result, another intent under the same
  identifier is `OPERATION_ID_REUSED`.
- **B-BR-EMP-245 — Assignments end with the employee record or the asset, in
  the same transaction.** When an employee record becomes `TERMINATED`
  (offboarding with termination, the source record of an employee merge or any
  later producer), every assignment of that employee that is not `ENDED` at
  the database instant is ended at that instant (at its start when it has not
  started) with cause `EMPLOYEE_TERMINATED` and the employee record revision.
  When an asset becomes `DISPOSED`, every assignment of that asset that is not
  `ENDED` at the database instant is ended at the disposal instant (its
  microsecond floor; at the assignment's start when that is later) with cause
  `ASSET_DISPOSED` and the asset revision. Both happen inside the source
  change's transaction, so the source change and the endings commit or fail
  together. An `INACTIVE` employee, a deactivated asset, a reverted disposal,
  a rehire and a reactivation neither end nor restore assignments.
- **B-BR-EMP-246 — The assets assigned to a member at an instant drive the
  `SELF` scope on assets.** The assets assigned to membership M at instant T
  are the assets of the assignments of M's employee record whose period
  contains T, while M's membership period contains T (started at or before T
  and not ended at T); an ended or not yet started membership lists nothing
  even when its employee record keeps assignments. A current `SELF` grant
  whose role has a positive `fleet.asset.read` rule covers an asset exactly
  while that asset is assigned to the member at the server instant (asset
  read, lifecycle, allocation and assignment history, meter reads); it never
  covers the organization list nor any other permission (assignment commands,
  asset management), and `ORG` and structural coverage are unchanged. The member's own assigned-asset read lists the assignments that
  are `ACTIVE` at the server instant. Changing an assignment never changes a
  stored record; the signed meter intake keeps its `ORG`-only rule until
  EMP-METER-002 adopts this query.
- **BUC-EMP-124 — Assign, replace and end assignments of an asset.** Actor:
  member with fresh `fleet.asset.assign` covering the asset (`ORG`, or a
  `UNIT`, `COST_CENTRE` or `ASSET` grant matching the asset's FLEET-005
  allocation at the server instant). Main flow: choose the asset, the kind, one
  or several employee records, the optional start, end and reason → the server
  checks B-BR-EMP-243/244 under the organization root lock → assignments
  created, one audit event and receipt → the asset enters each assignee's
  `SELF` scope from the start. Alternatives: overlapping primary →
  `ASSIGNMENT_OVERLAP`, or "replace" ends the previous primary at the new
  start; employee not `ACTIVE` or asset not assignable → `VALIDATION_FAILED`;
  `TEMPORARY` without end → `VALIDATION_FAILED`; end of an ended assignment →
  `CONFLICT`; supervisor scoped to another unit → `FORBIDDEN`; replay → first
  result. Result: assignments with periods; history kept.
- **BUC-EMP-125 — Read the assignments of an asset and my assigned assets.**
  Actor: member with current `fleet.asset.read` covering the asset (including
  `SELF` on an asset assigned to the member), or any member for the own list
  (`ORG` or `SELF` grant with `fleet.asset.read`). Main flow: the asset's
  assignments in start order with kind, employee record, period, derived state
  at the server instant, recording and end data; or the requesting member's
  `ACTIVE` assignments with asset, kind and period. Alternatives: foreign or
  unknown asset → `NOT_FOUND`; a member without a covering grant →
  `FORBIDDEN`. Result: read only; no notification is sent in this task.
