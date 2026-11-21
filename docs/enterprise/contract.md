# Abastevo Empresas product contract

[ADR-025](../decisions/025-private-abastevo-empresas-organization-boundary.md) records the maintainer's decision. This contract governs the planned Abastevo Empresas private fleet context.

## Business rules

- **B-BR-EMP-01 — Organization ownership.** Each private Empresas record
  belongs to one organization. Legal entities, CNPJs, operating units,
  teams and fleets are managed within that boundary. Ending a membership
  removes access and preserves records owned by the organization.
  [Shared tenancy rules](business-rules.md#1-tenancy-and-ownership--br-ten)
- **B-BR-EMP-02 — Current authority.** The server resolves organization
  access from a verified account and an active membership with the
  required permission on the resource. Account or staff status, payment,
  a shared CNPJ root and knowledge of an identifier grant no access.
  [Permission contract](permissions.md)
- **B-BR-EMP-03 — Isolated surfaces.** Reads, writes, lists, jobs, searches,
  caches, files, exports and local data carry organization scope. Access
  attempts reveal no foreign record or resource-existence hint. An
  offline operation retains the organization in which it was created.
  [Architecture](architecture.md)
- **B-BR-EMP-04 — Product separation.** Public catalog and community
  reads remain tenant-free. Referencing a public station grants no claim
  or write authority. Personal Motoristas and Premium records remain
  account-owned. The individual `ENTERPRISE` tier confers no organization
  privilege. Free participation, source separation and payment neutrality
  continue to apply. [Shared source rules](business-rules.md#2-source-separation-and-neutrality--br-src)
- **B-BR-EMP-05 — Company panel.** The company panel is served at
  `/empresas/` on the configured API origin. Its account session is
  separate from admin sessions. Each private request resolves current
  membership and permission. Mutations require exact-origin and CSRF
  checks. Company web decision
- **B-BR-EMP-06 — Protected Android partition.** Local private data,
  attachments and queued operations are scoped by account, server origin
  and organization, protected by Keystore-backed keys and excluded from
  backup and transfer. An operation keeps its creation scope across retry
  and context changes. Logout locks the partition.
  Android decision
- **B-BR-EMP-07 — Private fueling receipts.** A receipt belongs to the
  organization that owns the fueling. Member-bound short-lived access
  requires current permission and an audit record. C1 validation uses
  synthetic receipts. Real-receipt intake and retention require an approved
  category-specific policy and existing real-data gates. Human review is
  recorded independently of fiscal status.
  Receipt decision

EMP-PLAT-001 and EMP-PLAT-002 define the shared organization and database
boundaries.

- **B-BR-EMP-08 — Explicit organization scope.** A private organization
  repository call takes an explicit, nonzero OrgScope. Its OrgID is
  a validated non-nil UUID, stored in lowercase dashed form. Parsing accepts
  either hexadecimal letter case and rejects malformed, padded and nil
  identifiers. The application resolves the selected organization under
  current actor authority before constructing scope. Repository adapters
  reject a zero scope before querying and apply it to each private reference.
  [Organization boundary](../decisions/025-private-abastevo-empresas-organization-boundary.md)

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

Implementation requires the [F0 decision gates](decisions.md#5-what-is-actually-blocking).
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

### Authorization conformance checkpoint

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

### Persisted company setup and settings

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

### Device protection completion

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

### Scoped intake and durable receipts

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

### Protected Android operation queue

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

### Evidence-based local synchronization presentation

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

## Enterprise meter evidence

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

## Enterprise pure fueling

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

## Enterprise asset persistence

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

## Source-backed immutable meter intake (2026-11-14)

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

## Actual held review and durable author outcomes (2026-11-14)

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
  A declared clock never grants a write. SELF follows B-BR-EMP-259..262;
  structural and F4 late/grace sources remain closed until their actual owners
  integrate.
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

## Amendment kernel for approved records (2026-11-14)

Evidence.

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

## Review lifecycle component (2026-11-14)

Evidence.

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

## Expense domain: origin, competence and company amount (2026-11-14)

Evidence.

## Enterprise pure expense

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
  reporting-day period start, end), an explicit basis and expenses that each
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

## Document domain: container, independent statuses, links and allocations (2026-11-14)

[Evidence.

## Enterprise pure document domain

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

## Budget domain: lines, periods, dimensions and capacity (2026-11-14)

Evidence.

- **B-BR-EMP-198 — One line: organization, half-open period, one dimension and a required exact limit.**
  A budget line is a scoped line identity in one organization with an
  explicit half-open reporting-day period start, end) built by the
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

## Payables domain: supplier, reimbursement and card statement obligations (2026-11-14)

[Evidence.

## Enterprise payables domain

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

## Sensitive-access log for documents, exports and location (2026-11-14)

Evidence.

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

## Asset lifecycle: acquisition, deactivation, disposal and archive (2026-11-14)

Evidence.

## Enterprise asset lifecycle

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

## Period closing, lock and reopening (2026-11-14)

Evidence.

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

## Notification core: inbox, preferences, de-duplication, limits and history (2026-11-14)

Evidence.

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

## Document persistence, upload session and audited private access (2026-11-14)

Evidence.

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
  type by content, conditional private write and verified read). Only after that does storage move `UPLOADING → RECEIVED`, and only the EMP-DOC-014 safety check makes it `STORED` (B-BR-EMP-251..253). A refused, interrupted, oversized or mismatched transfer leaves the document `UPLOADING` with no event; content refused for bytes that match the declaration closes it (B-BR-EMP-252); a retry after `STORED` returns the stored record and writes nothing.
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

## Unit and cost-centre allocation history (2026-11-14)

Evidence.

## Enterprise asset allocation history

- **B-BR-EMP-223 — Keep unit and cost-centre allocations as half-open,
  non-overlapping periods.** Each organization asset has two append-only
  period histories, one of operating units and one of cost centres. A period
  names one unit (or cost centre) of the same organization, a start and an
  optional end, and covers `start, end)`: the instant of a change belongs to
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

## Asynchronous export infrastructure: job, private file and expiring link (2026-11-14)

[Evidence.

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

## Fiscal structure: access key, NFC-e QR code and XML structural validation (2026-11-14)

Evidence.

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

## Unit, cost-centre, team and asset scopes with resource scope resolution (2026-11-14)

Evidence.

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

## Links between documents and operations (2026-11-14)

Evidence.

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

## Auditor read-only role and export permission gating (2026-11-14)

Evidence.

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

## Driver assignments: primary, temporary and multiple drivers (2026-11-14)

Evidence.

- **B-BR-EMP-243 — An assignment names one asset and one active employee
  record for a half-open period with a derived state.** An assignment belongs
  to one organization and names one of its assets, one of its employee
  records and a kind: `PRIMARY` (the responsible), `ADDITIONAL` (another
  allowed user) or `TEMPORARY` (a substitute; it requires an end). Its period
  `start, end)` has whole-microsecond bounds; the end is optional except for
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
  stored record; the signed meter intake adopts this query for
  `meter.reading.record` (EMP-METER-002, B-BR-EMP-259).
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

## Upload safety checks: type, size and active content (2026-11-14)

[Evidence.

- **B-BR-EMP-251 — Verified bytes are received, then checked before they are
  ready.** The ingress keeps the EMP-DOC-002 integrity gate: a body whose size
  or SHA-256 differs from the declaration, and an oversized, truncated,
  interrupted or empty body, are refused (`VALIDATION_FAILED`) and leave the
  document `UPLOADING` with no object and no event, so the device can retry
  from its stored file (BR-OFFL-015, UC-OFFL-03). Bytes that match the
  declaration exactly are inspected by content; clean bytes are written
  create-only under `org/<org>/doc/<document>/<sha256>` and verified by
  read-back, then one creator-bound transaction moves storage
  `UPLOADING → RECEIVED`, records `document.upload.received` and queues the
  safety job. `STORED` is reached only by the safety check over the stored
  object: a bounded read of at most the declared size plus one byte, the exact
  size and digest, the type decided by content equal to the declared type, the
  active-content rules (B-BR-EMP-252) and an unchanged snapshot after
  inspection; it records `document.upload.stored`. The upload request runs the
  check at once and normally answers `STORED`; if the check cannot complete it
  answers `RECEIVED` and the job finishes it. This amends the single
  `UPLOADING → RECEIVED → STORED` transaction and the "wrong-type stays
  `UPLOADING`" sentence of B-BR-EMP-220.
- **B-BR-EMP-252 — Refused content keeps a truthful terminal state and stays
  inaccessible.** The declared digest pins the bytes, so content that matches
  the declaration but is refused can never succeed on retry and is closed at
  once, passing through `RECEIVED`: bytes after the JPEG end-of-image marker,
  or a markup, script, entity, document, archive or executable signature
  inside the image (metadata segments for every signature, the whole stream
  for the long markup ones) make it `QUARANTINED` with reason `UNSAFE_CONTENT`;
  content that is not an admissible image of the declared type (magic, marker
  structure, pixel caps, decoding, inspected type different from the declared
  one) makes it `FAILED` with `UNSUPPORTED_CONTENT`; a stored object whose
  size or digest differs from the declaration makes it `FAILED` with
  `INTEGRITY_MISMATCH`. Every `QUARANTINED` or `FAILED` row entered after this
  change carries exactly one closed reason that matches its transition, never
  changes and is enforced by the database; each terminal step records one
  `document.upload.quarantined` or `document.upload.failed` event with the
  reason code. The ingress never writes refused bytes; an object that the
  check finds unsafe is retained, isolated and never served, until the
  reviewed retention workflow (EMP-DOC-012); the object of a `FAILED` document
  is removed and its absence confirmed. Only `STORED` documents are served:
  content access and reads refuse `UPLOADING`, `RECEIVED`, `QUARANTINED` and
  `FAILED` documents as `NOT_FOUND`, also from a freshly started process over
  the same database and bucket, and orgmedia's read-time re-inspection
  refuses unsafe bytes even on a `STORED` row. An upload retry after a
  terminal refusal is `CONFLICT` and reads no byte. C1 stays JPEG-only.
- **B-BR-EMP-253 — The safety job is retry-safe and converges.** Queue kind
  `org-document-safety` carries only `{version, org_id, document_id}` and is
  queued twice per document under separate de-duplication keys: at
  `RECEIVED` (check, one minute later) and at declaration (session close plus
  grace, B-BR-EMP-254). It acts on the current row only: `RECEIVED` runs the
  check; `UPLOADING` closes an expired session; `FAILED` removes the object;
  every other state, a missing document and another organization's document
  are no-ops. The verdict is a pure function of the immutable declaration and
  the create-only object, and every settlement is a compare-and-set on the
  expected state and storage revision under the row lock, so the inline
  check, upload retries and repeated or concurrent jobs commit exactly one
  outcome and one event while the others observe it. A storage, database or
  inspection outage leaves the document `RECEIVED` with nothing written and is
  retried; the job's final attempt settles `FAILED` with
  `CHECK_ATTEMPTS_EXHAUSTED`. A failed object removal leaves `FAILED` and is
  retried. No partial, unverified or refused object ever becomes `STORED`.
- **B-BR-EMP-254 — Upload sessions close.** An upload session is open for 24
  hours after its declaration (database clock): a declaration replay mints an
  upload capability only if it expires by then. A document still `UPLOADING`
  five minutes after the close is closed `FAILED` with reason
  `UPLOAD_ABANDONED` by the job, and any object under its key is removed; the
  database refuses that closure earlier. Documents declared before this
  change keep their state and metadata; the migration queues their closure
  job.
- **BUC-EMP-128 — Prove pass and refusal paths on real PostGIS and isolated
  storage.** Through the service and the stored-object check on the
  disposable S3 bucket: digest-mismatched, oversized, truncated, interrupted
  and empty bodies leave `UPLOADING` with no object; content with a matching
  digest that is not a JPEG becomes `FAILED UNSUPPORTED_CONTENT`, a JPEG with
  a trailing payload or embedded script becomes `QUARANTINED UNSAFE_CONTENT`,
  neither writes an object and a retry is `CONFLICT`; a clean JPEG becomes
  `STORED` at revision 3 with one received and one stored event; substituted,
  truncated or unsafe stored objects under `RECEIVED` become `FAILED
  INTEGRITY_MISMATCH` (object removed) or `QUARANTINED` (object kept) and are
  never served; another organization's job or key reaches nothing; the schema
  enforces closed reasons per transition and forced row-level security.
- **BUC-EMP-129 — Prove convergence under retry, concurrency, storage failure
  and restart.** Concurrent checks of one received document commit one
  outcome and one event; a fault-injected object store leaves `RECEIVED` with
  no event, a fresh process over the same database and bucket serves nothing
  until the queued job, claimed through the real queue and dispatcher, stores
  it; the final attempt settles `CHECK_ATTEMPTS_EXHAUSTED`; a failed removal
  keeps `FAILED` and is retried; an expired session closes only after the
  window; a populated database upgrades with preserved metadata and queued
  closure jobs.

## CSV exports for operational lists (2026-11-14)

Evidence.

- **B-BR-EMP-255 — Three operational lists are admitted, scoped and
  re-authorized in the reading transaction.** The export registry admits
  exactly three list kinds: `fleet.asset_list.csv` (asset register, data
  permission `fleet.asset.read`; optional filters `state` — one asset state,
  absent meaning every state except `ARCHIVED` as on the asset list — and
  `kind`), `fleet.meter_reading_list.csv` (server-accepted meter readings,
  `fleet.asset.read` as every meter read; optional `asset_id`, `meter_kind`,
  `state` `VALID`/`SUSPECT` and a half-open period `occurred_from`/
  `occurred_to` of canonical UTC instants with from before to) and
  `idn.employee_list.csv` (employee register, `idn.employee.read`; optional
  `state`). Parameters are a closed object: an unknown key, an empty or
  non-canonical value or a reversed period is `VALIDATION_FAILED` at request
  and nothing is written. Each kind keeps the infrastructure ceilings (500 000
  rows, 16 MiB, 120 s). The worker reads in one scoped transaction of the
  job's organization under forced row-level security: it first re-checks the
  recorded requester's `rpt.export.run` and the kind's permission for the whole
  organization (only organization-wide grants export; unit, cost-centre, team
  and own-records row scoping is retained), then switches the transaction to
  read-only and reads the rows in one parametrized statement bound to the
  organization. Authority lost at that point ends the job `FAILED` with
  `EXPORT_RIGHTS_LOST` and no file, exactly as at the worker's earlier check.
  Every other kind — including the spreadsheet kinds `*.xlsx` — stays
  `EXPORT_FORMAT_UNAVAILABLE` (BR-RPT-011).
- **B-BR-EMP-256 — The table is exact, inert and deterministic.** The CSV is
  UTF-8 with a byte-order mark, comma separated, with LF line ends, written by
  Go `encoding/csv` (a field holding a separator, quote, line break or leading
  space is quoted and embedded quotes are doubled), with one header row of
  stable lower-snake column names. Integer cells carry the server's own exact
  integers in canonical decimal, in the base unit named by the column
  (`_m` metres, `_s` seconds) or as plain counts, and are never altered, a
  negative offset included. Every other cell is text: a text cell that begins
  with `=`, `+`, `-`, `@`, tab, carriage return, line feed or an apostrophe is
  written with one extra leading apostrophe, so no cell can start a formula,
  and a reader recovers the stored value by removing exactly one leading
  apostrophe from a text cell that has one. Text must be valid UTF-8. Instants
  are canonical RFC 3339 UTC with the stored sub-microsecond digits. An absent
  value is an empty cell, never 0. Rows are ordered deterministically: assets
  and employees by identifier, readings by exact occurrence instant and then
  identifier. Registers are read as stored at the generation instant (the
  reading transaction's start); readings include only those the server
  received at or before the job's "as of" instant. The same stored data and job
  produce byte-identical CSV content; between two runs only the manifest's
  `generated_at` (and, across jobs, the job's own identifier and "as of")
  differs.
- **B-BR-EMP-257 — The manifest states the basis and the omissions, never
  omitted values.** Each file is a ZIP package (`application/zip`, deflated
  entries timed at the "as of" instant) holding exactly `<list>.csv` and
  `manifest.json`. The manifest (schema `orgexport.list.v1`) states the kind,
  export, organization identifier, name and time zone, requester membership,
  declared read permission, "as of" and generation instants, the canonical
  filters, the review basis (readings `APPROVED_ONLY`, "somente aprovados":
  readings held for review are not included; registers `NOT_APPLICABLE`), the
  content basis, the ordering, the row count, the CSV's name, rows, bytes and
  SHA-256, every column with its type and unit, the encoding rules (UTF-8,
  byte-order mark, separator, line end, inert prefix) and every omitted
  sensitive column with a reason code and a note, never a value. The employee
  list omits the work e-mail and phone (`PURPOSE_LIMITED`: collected only to
  hand over invitations, BR-PRIV-005); the reading list omits the capture
  session or device-enrolment locator (`CAPTURE_LOCATOR`, BR-PRIV-008).
  Location and document-content columns belong to no admitted list.
- **B-BR-EMP-258 — A ceiling or a failure never yields a partial file.** Data
  rows count against the job's row ceiling and packaged bytes against its byte
  ceiling as they are written; reaching either fails the whole production with
  `EXPORT_TOO_LARGE` and stores nothing. A read failure, invalid stored text or
  an unfinished package is a retryable failure that publishes nothing; the
  last attempt ends `EXPORT_ATTEMPTS_EXHAUSTED`. Only a complete, closed
  package is stored, read back and made `READY` (B-BR-EMP-229).
- **BUC-EMP-130 — Export an operational list.** Actor: a member whose current
  organization-wide roles cover `rpt.export.run` and the list's read
  permission (today OWNER, ADMIN, FLEET_MANAGER, FINANCE and AUDITOR for the
  three lists). Main flow: on the list choose "Exportar" with the current
  filters → the server validates the filters and creates the `QUEUED` job → the
  worker re-checks authority and reads the organization's rows in one
  read-only transaction → writes the CSV and its manifest into the package
  under the ceilings → stores, verifies and publishes it `READY` → the member
  takes a capability and downloads the package. Alternatives: an unknown or
  invalid filter → `VALIDATION_FAILED`; a spreadsheet kind →
  `EXPORT_FORMAT_UNAVAILABLE`; no data permission → `FORBIDDEN` naming the key;
  authority lost before or while reading → `FAILED`/`EXPORT_RIGHTS_LOST`, no
  file; over a ceiling → `FAILED`/`EXPORT_TOO_LARGE` with advice to narrow the
  filter. Result: a private, expiring package with only the organization's
  rows the member may read.
- **BUC-EMP-131 — Read and verify an exported list.** Actor: the member who
  downloaded the package. Main flow: open `manifest.json` → compare the CSV's
  SHA-256, bytes and row count → read the basis (organization, filters, "as
  of", generation instant, review basis, ordering, units) and the omitted
  columns with their reasons → parse the CSV with any RFC 4180 reader after the
  byte-order mark → remove one leading apostrophe from a text cell that has one
  → read integers in the unit named by the column. Alternatives: a digest or
  count mismatch → the file is not the published one and is discarded.
  Result: values identical to the stored records, with nothing a spreadsheet
  can execute.

## Full closure: assignment-backed SELF intake (2026-11-14)

Evidence.

- **B-BR-EMP-259 — A SELF grant records a field reading only on an asset
  assigned to the member, continuously.** In the signed `meter.reading.record`
  intake a role assignment with scope `SELF` whose role has a positive
  `meter.reading.record` rule covers an asset at the instants where (a) the
  grant is effective (its recorded period: from the later of its start and its
  server recording to the earlier of its end and its ending), (b) an
  assignment of the member's employee record to that asset contains the
  instant (B-BR-EMP-243 half-open periods of any kind) and (c) the member's
  membership period contains it (B-BR-EMP-246). `ORG` grants with an `O` cell
  keep their B-BR-EMP-160 coverage unchanged; a reading's coverage is the
  union of both. A reading is applied only when that union covers without a
  gap every instant from the earlier to the later of its declared instant and
  its server receipt, and the current source check at receipt confirms it: for
  a SELF grant the assigned-asset port answers that the asset is assigned to
  the member at the database receipt instant. Assignment history comes only
  from the retained assignment periods of the membership's employee record,
  never from current candidates, current assignments or the client. The replay
  of an accepted reading reads it under current `fleet.asset.read` coverage,
  where a SELF grant covers the asset while it is assigned to the member
  (B-BR-EMP-246); a held receipt replays from its review proof without a
  business read. Web meter commands are unchanged: the panel records no
  reading, installs under `fleet.asset.manage` (no SELF cell) and reads under
  B-BR-EMP-246.
- **B-BR-EMP-260 — Never covered is a final FORBIDDEN; formerly covered is
  held PERMISSION_CHANGED.** When no `ORG` grant and no SELF grant with an
  assignment of that asset covered it at any instant up to the receipt, the
  operation is refused `FORBIDDEN`: the signed response is withheld and
  nothing is claimed, stored, audited or held. This includes a member never
  assigned to the asset, a member assigned only to another asset, an
  assignment that starts after the receipt and a SELF grant that was never
  effective while an assignment existed. When coverage existed at some instant
  up to the receipt but does not cover the whole span between the declared
  instant and the receipt, the reading is held for review with
  `PERMISSION_CHANGED` through the existing OFFL-008 mechanism (one review
  item, one held reading, a version-1 review ACK, no applied reading, no
  effect): an assignment or grant ended before the receipt, coverage that
  started after the declared instant, a gap between two assignments, and an
  assignment that ended before the declared instant (the declared instant is a
  client claim and cannot prove that the member never held the right,
  EDR-118). Boundaries are half-open: a reading declared exactly at an
  assignment's start is covered and one declared exactly at its end is not.
  An assignment command and an intake serialize on the organization root
  lock: an intake that holds it first decides on the still-active assignment
  and the end is recorded after its receipt; an intake that waits decides on
  the ended period.
- **B-BR-EMP-261 — A hold caused only by the asset's disposal stays
  ASSET_UNAVAILABLE.** A disposal ends the asset's assignments at the disposal
  instant (B-BR-EMP-245). When a reading would be applied if the member's
  assignments ended by the asset's disposal (cause `ASSET_DISPOSED`) were not
  ended, it is held `ASSET_UNAVAILABLE` when declared at or after the asset's
  current disposal instant (B-BR-EMP-206, exact to the nanosecond) and
  `PERMISSION_CHANGED` otherwise, including after the disposal is reverted
  (reverting restores no assignment). A real change of the member's right (a
  grant ended, an assignment ended by a command, a replacement or a
  termination) keeps the `PERMISSION_CHANGED` precedence of B-BR-EMP-206, and
  `ORG` handling is unchanged.
- **B-BR-EMP-262 — SELF is resolvable; unsupported grants stay unavailable.**
  With the assigned-asset source composed (the default runtime), a SELF-only
  member is no longer unavailable: the outcome is `FORBIDDEN`, held or applied
  by B-BR-EMP-259/260. Grants the intake cannot resolve stay unsupported:
  `UNIT`, `COST_CENTRE`, `TEAM` and `ASSET` grants (EDR-161 resolves them for
  web records only), an `ORG` grant whose `meter.reading.record` cell is
  own-records (`S`: `DRIVER` or `OPERATIONS_STAFF` granted at `ORG`), and SELF
  when no assigned-asset source is composed. An unsupported grant never
  applies a reading; when one was effective at or before the receipt and no
  `ORG` or SELF coverage ever was, the request is unavailable (503, whole
  response withheld, nothing stored) because the server cannot prove absence.
  The malformed-payload admission keeps using `ORG` grants only.
- **BUC-EMP-132 — Record a reading on my assigned asset from the field.**
  Actor: member with a current SELF grant for `meter.reading.record`
  (`DRIVER`, `OPERATIONS_STAFF` or another role granted at SELF) on an
  enrolled device. Main flow: the app records the typed reading with the
  device's declared instant and sends the signed `meter.reading.record` → the
  server checks the current device proof, the asset and meter, the retained
  grant and assignment periods (B-BR-EMP-259) and the asset's disposal →
  coverage continuous from the declared instant to the receipt → one reading,
  one audit event, one effect and one `ACCEPTED` ACK; a replay returns the
  same ACK while the asset stays assigned. Alternatives: never assigned to
  that asset, or only to another one → `FORBIDDEN`, nothing stored; the
  assignment ended (command, replacement, termination) or started after the
  declared instant → `ACCEPTED_FOR_REVIEW` `PERMISSION_CHANGED`; declared at
  or after the asset's disposal → `ACCEPTED_FOR_REVIEW` `ASSET_UNAVAILABLE`;
  declared outside the meter's service → `REJECTED` `MALFORMED`. Result: an
  applied reading or a held review item, never a silent drop.
- **BUC-EMP-133 — Hold a field reading after the member's assignment changed
  (system).** Actor: the signed intake, then an existing reviewer. Main flow:
  a reading arrives whose author had SELF coverage of the asset at some
  instant up to the receipt but not across the declared instant and the
  receipt → the reading is held with `PERMISSION_CHANGED` (or
  `ASSET_UNAVAILABLE` under B-BR-EMP-261) in the original transaction → the
  existing OFFL-008 reviewer reads it and accepts or rejects it. Alternatives:
  an assignment end racing the intake → whichever holds the organization root
  lock first decides, consistently with the end instant; replay → the first
  ACK; an asset or assignment of another organization → `NOT_FOUND` or no
  coverage, never a cross-organization effect. Result: one held item and no
  reading until a reviewer accepts it.

## Fueling persistence, sync handler and arithmetic consistency (2026-11-14)

Evidence.

- **B-BR-EMP-263 — One immutable fueling per signed `fueling.submit`.**
  The operation `fueling.submit` schema 1 (entity `fueling`, id = the
  device fueling UUID, no expected revision) creates exactly one
  organization-scoped, forced-RLS, insert-only fueling holding its original
  revision-1 fact as the FUEL-001 candidate validated it: asset, responsible
  employee, author membership and its employee record, capture enrollment,
  the envelope declaration (microsecond floor plus 0..999 remainder, entry
  zone, device offset), product and unit from `fuel-products-v1`, quantity in
  milli-units, paid total and optional discount in centavos, optional unit
  price in milli-BRL with its optional original text, fill, payer, payment
  method, origin `APP`, an optional `NOT_FOUND` typed place, the server
  receipt instant and the operation's payload digest. The stored unit and
  cost centre are the FLEET-005 snapshot of the asset at the declared
  instant (none when unallocated), never the current allocation
  (BR-AUTH-005). The review state is `SUBMITTED` at revision 1, produced by
  the APPR-001 lifecycle and kept as an append-only first review entry;
  later decisions and revisions belong to FUEL-003/015. No expense, document
  link, price snapshot or duplicate signal is created here.
- **B-BR-EMP-264 — Arithmetic is a stored warning, never a correction.**
  At acceptance the server reconciles with the accepted `orgvalue` rule
  (BR-MONEY-005): gross = paid + discount, where an absent discount
  reconciles as zero but stays absent in the record; with a unit price,
  expected = round-half-up(quantity × unit price / 10 000) and the absolute
  difference are stored, and a difference above one centavo stores the
  warning `ARITHMETIC_MISMATCH`; without a unit price only the exact derived
  price rational is stored, labelled derived. `KWH` uses the same integer
  formula. The paid total is stored exactly as entered and never recomputed
  (BR-MONEY-004). A product that no configured tank of the asset accepts
  stores `PRODUCT_NOT_ACCEPTED` (BR-FUEL-016). Warnings never refuse a fact:
  the ACK stays `ACCEPTED`; ACK warnings keep the OFFL-001 vocabulary and the
  fueling warnings live on the record.
- **B-BR-EMP-265 — Reading evidence commits with the fueling.** Exactly one
  of a reading or a missing-reading reason is required. A reading (meter,
  segment, reading id, base value of the meter kind) is recorded through the
  fleet embedded-reading port with origin `FUELING`, the fueling's operation
  id, the envelope declaration and method `TYPED`, in the same transaction
  as the fueling; a rollback leaves neither. A reason is non-blank,
  control-free and at most 280 characters. Neither, both, a reading of
  another kind or outside the meter's service interval, an unknown or
  non-active responsible employee, a `DRAFT` asset, a payload unit that is
  not the product's unit, or a fueling id already used by another operation
  is `MALFORMED` with nothing stored; a meter or segment not found on the
  asset is `DEPENDENCY_PENDING` like the fleet reading intake. The database
  proves at commit that a stored fueling has its accepted receipt and that
  its reading has origin `FUELING`, the same operation, author, instant and
  asset.
- **B-BR-EMP-266 — Authority at receipt.** Only a current enrolled
  operational device of the author records. History of
  `fuel.fueling.record` follows EDR-118 continuity from the declared instant
  to the receipt: an `ORG` grant with a positive rule covers the asset (the
  new fueling is the member's own record in that scope); a `SELF` grant
  covers it only while the asset is assigned to the member (FLEET-006
  periods) and only when the member's own employee record is the
  responsible employee; structural grants stay unsupported in the signed
  intake (unavailable when alone, as B-BR-EMP-262). Never covered is
  `FORBIDDEN` and a missing or foreign asset `NOT_FOUND`, both refusing the
  whole response with nothing stored. Formerly covered (changed rights) or a
  declared instant at or after the asset's disposal stores nothing and is
  `RETRY`/`UNAVAILABLE` until a held-fueling review owner exists. The
  embedded reading also needs current `meter.reading.record` through the
  fleet port. A receipt replay needs current `fuel.fueling.read` over the
  stored record (author, asset, stored unit and cost centre). Intake never
  consults the APPR-006 period gate: an accepted field fact is not blocked by
  a locked reporting day; locked-period edits belong to FUEL-015.
- **B-BR-EMP-267 — Exact replay and concurrency.** The intake ledger decides
  by operation id: the same id and digest returns the stored ACK byte for
  byte and creates no second fueling, reading, warning, review entry or
  audit; the same id with another digest is `REJECTED`
  `OPERATION_ID_REUSED` and changes nothing; concurrent identical
  operations leave exactly one fueling. The receipt replay re-proves that the
  stored fueling equals the operation (identity, author, enrollment, digest,
  amounts and reading). Optional references whose owners do not exist yet
  (trip, contracted supplier, catalog-station/supplier/internal-tank place,
  attachments, dependencies, soft references) are recognised and stay
  `RETRY`/`UNAVAILABLE` with nothing stored; an unknown payload key is
  `MALFORMED`.
- **BUC-EMP-134 — Record a fueling through the signed intake.** A covered
  member's device sends `fueling.submit` with a reading or a missing reason;
  the server stores the fueling, its reading, warnings and first review
  entry with one ACCEPTED ACK in one transaction; resends return the same
  ACK; a mismatching paid total is accepted with `ARITHMETIC_MISMATCH`.
- **BUC-EMP-135 — Prove the fueling store on real PostGIS.** Replay,
  digest reuse, 12-way concurrency, embedded-reading rollback in both
  directions, foreign asset, FORBIDDEN/held SELF history, malformed and
  unavailable forms, forced RLS without an organization setting,
  insert-only history and a populated 000212 → 000215 upgrade with injected
  failure, recovery, no-op replay and previous-image compatibility.

## Trip domain core: operational states and the driver-initiated trip (2026-11-15)

Evidence.

- **B-BR-EMP-268 — Closed operational vocabulary and the C1 lifecycle.**
  Trip operational states are exactly PLANNED, ASSIGNED, ACCEPTED,
  IN_PROGRESS, PAUSED, COMPLETED and CANCELLED (08-state-model §3);
  COMPLETED and CANCELLED are terminal. Operational commands are exactly
  START, FINISH, PAUSE, RESUME and CANCEL. A C1 driver-initiated trip is
  created by START directly in IN_PROGRESS (it never passes through the
  mission or manager-planned states) and FINISH moves IN_PROGRESS to
  COMPLETED. The transition table answers, first failure first: an unknown
  state or command token is `INVALID`; any command on a terminal state is
  `INVALID_TRANSITION` (a new attempt is a new trip, 08-state-model §4.3);
  PAUSE, RESUME and CANCEL, and any command on PLANNED, ASSIGNED, ACCEPTED or
  PAUSED, are `UNSUPPORTED_COMMAND` until their owners (EMP-TRIP-011 for
  pause, resume and cancellation; EMP-TRIP-008/009/021 for missions and
  planned trips) define them; START on an existing trip is
  `INVALID_TRANSITION`. Approval words (APPROVED, APPROVE, SUBMIT) are not
  operational tokens.
- **B-BR-EMP-269 — A start needs an eligible context.** START takes the
  scoped trip identity and the author membership (absent or malformed
  `INVALID`, another organization `SCOPE`) and the eligible context: the
  asset, the driver's employee record, the departure meter evidence and the
  validated time-kernel declaration (instant, entry zone, device offset).
  Departure evidence is exactly one of a reading of the trip's asset with
  its exact typed meter value (the METER-001 reading reference and value of
  that meter's kind, within its modulus) or a nonblank, control-free missing
  reason of at most 280 characters (the METER-001 profile). Present parts
  are checked in the order asset, driver, departure evidence, declaration:
  another organization, or a reading of another asset, is `SCOPE`; a
  malformed declaration is `INVALID`. Absent parts are then reported
  together as `INCOMPLETE` with the ordered list ASSET, DRIVER,
  DEPARTURE_EVIDENCE, DECLARATION. Membership, permission, assignment, asset
  availability and the one-active-trip rule are not decided by this pure
  value; their owners check them at receipt or from cached grants.
- **B-BR-EMP-270 — A trip belongs to the day it started.** The start
  attributes the reporting day of its instant with the accepted
  `orgtime.AttributeReportingDay` (operating-unit zone when supplied,
  otherwise the organization zone; an invalid unit zone never falls back;
  the device entry zone never decides) and keeps day, zone and source. An
  unusable zone is `INVALID`. No server receipt is required or invented, so a
  trip starts and finishes offline.
- **B-BR-EMP-271 — Transitions preserve the trip.** FINISH takes only its
  actor, its declaration and its final evidence. It cannot change the trip
  identity, organization, asset, driver, start event or start reporting
  attribution; a refused command returns no trip and leaves the input value
  unchanged.
- **B-BR-EMP-272 — Operational completion.** FINISH on a trip of another
  organization is `SCOPE`. Its actor is validated like the author, then the
  final evidence (a reading of another organization or asset is `SCOPE`; a
  reading identity equal to the departure reading is `INVALID`) and the
  declaration (`INVALID` when malformed); absent parts are `INCOMPLETE` with
  FINAL_EVIDENCE, DECLARATION. In C1 only the member who started the trip
  finishes it (`FORBIDDEN` otherwise; handover and supervisor closure are
  EMP-TRIP-012/014). A command the lifecycle refuses is
  `INVALID_TRANSITION`. A finish instant strictly before the start instant
  is `FINISH_BEFORE_START`; an equal instant is accepted.
- **B-BR-EMP-273 — Meter evidence is never invented (BR-TRIP-004).** A trip
  reports usage `MEASURED` only when it is COMPLETED and both readings exist
  on the same meter and segment with a final value not below the departure
  value; the usage is then the exact difference in the meter's base unit
  (184 320 000 m to 184 610 000 m gives 290 000 m). Otherwise it is
  `NOT_MEASURED` with the reason NOT_FINISHED, READING_ABSENT,
  DIFFERENT_SEGMENT or DECREASING, never zero, interpolated, GPS, planned or
  provider distance. A decreasing or cross-segment final reading does not
  refuse the finish: the field fact is kept and final-reading plausibility
  stays with the server and review (10-offline-sync matrix).
- **B-BR-EMP-274 — Operational state is not review or delivery.** The trip
  holds no review or sync state and needs no receipt; completion never sets
  or implies a review state. The trip review vocabulary is the closed input
  DRAFT, SUBMITTED, IN_REVIEW, APPROVED, RETURNED, REJECTED, CANCELLED and
  VOIDED; a standing of one operational and one review state is completed
  only for COMPLETED and approved only for APPROVED, so a COMPLETED trip is
  never approved by itself (BR-REV-001).
- **B-BR-EMP-275 — Pure, redacted and shared.** The domain lives in Go
  `trip/domain` and its Kotlin mirror `com.anpfuel.domain.enterprise.trip`,
  both executing the one synthetic `trip/trip-v1.psv` oracle. Identities,
  readings, reasons and inputs print as redacted values in fmt, slog, JSON
  and `toString`. No persistence, intake, route, OpenAPI, app or panel
  screen, job, producer or dependency is added.
- **BUC-EMP-136 — Start a driver-initiated trip offline.** The driver's
  device supplies the trip identity, its membership, the asset, the driver
  record, the departure reading 184 320 000 m (or a stated absence) and the
  start declaration; the trip is IN_PROGRESS with its start reporting day and
  no usage yet. Missing parts are named together.
- **BUC-EMP-137 — Finish it operationally without approving it.** The same
  member finishes with the final reading 184 610 000 m or a stated absence;
  the trip is COMPLETED with 290 000 m of meter evidence (or NOT_MEASURED
  with its reason), keeps its asset, driver and start day, and its standing
  with review DRAFT, SUBMITTED or RETURNED is not approved.
- **BUC-EMP-138 — Prove the lifecycle and its denials in both languages.**
  Go and Kotlin load the same oracle, assert its hash, counts, unique IDs and
  the complete state and command table, and cover start without an eligible
  context, invalid and terminal transitions, unsupported pause, resume and
  cancel, completion mistaken for approval, foreign asset, driver, actor or
  reading, and a finish before its start.

## Midnight and time-zone crossing rules (2026-11-15)

Evidence.

- **B-BR-EMP-276 — One trip keeps its start reporting day.** A trip
  belongs to the reporting day of its start (BR-TIME-004) for its whole
  life. FINISH takes no zone and never re-attributes: the stored day, zone
  and source are unchanged when the trip crosses local midnight, several
  days, a device entry zone or a device offset. The end day is informational
  only: the local day of the finish instant in the stored reporting zone, so
  crossing midnight means the end day differs from the start day. Nothing
  splits the trip identity or adds a second trip day. A finish whose local
  day in the stored zone falls outside years 0001..9999 is `INVALID`,
  checked after `FINISH_BEFORE_START`.
- **B-BR-EMP-277 — Every event keeps its source time context.** Start and
  finish each retain their own UTC instant with nanoseconds, device entry
  zone and device offset exactly as declared, even when the offset
  disagrees with the zone's rules. The crossing view reports
  `entry_zone_changed` (the entry zones differ) and `offset_changed` (the
  device offsets differ) separately: a daylight-saving change in one zone is
  an offset change, a zone with the same offset is an entry-zone change.
  Neither decides the reporting day.
- **B-BR-EMP-278 — Elapsed time comes only from instants.** The elapsed
  time of a finished trip is the exact difference of its finish and start
  instants in seconds and non-negative nanoseconds, computed without a
  saturating duration and never from local wall times, entry zones, offsets
  or zone rules: daylight-saving gaps and folds and zone crossings neither
  add nor remove time. An unfinished trip reports `NOT_FINISHED` with no
  value. No working hours, wage or time-clock result is derived (EDR-028).
- **B-BR-EMP-279 — A wall-clock change or reboot is a flagged gap
  (BR-TIME-005).** Each event may carry a device run clock: a boot identity
  of 1 to 64 characters from `A-Z a-z 0-9 . _ -` and a non-negative
  monotonic count in milliseconds (validated after the declaration; a
  malformed one is `INVALID`). With both witnesses on one boot, a
  non-negative monotonic difference within the caller's explicit
  non-negative tolerance in milliseconds (no domain default; negative is
  `INVALID`) of the instant difference, the elapsed time is `CONFIRMED`; a
  different boot is a `GAP` with `BOOT_CHANGED`; a negative monotonic
  difference or one beyond the tolerance is a `GAP` with
  `CLOCK_DISCONTINUITY`; a witness missing on either event gives `DECLARED`
  (instants only, unconfirmed). A gap carries no elapsed value, so no
  duration is invented; the facts are kept and the finish is never refused
  for clock reasons.
- **BUC-EMP-139 — Finish after midnight in another zone.** A trip started
  at 23:30 in Cuiabá on 2026-11-14 and finished at 01:10 São Paulo time
  keeps the day 2026-11-14 and its reporting zone, reports its end day,
  the midnight crossing and the entry-zone and offset changes, and remains
  one trip with one identity.
- **BUC-EMP-140 — Measure elapsed time honestly.** Daylight-saving
  spring-forward and fall-back and zone crossings give the instant
  difference whatever the local clock faces show; a device clock moved
  against its monotonic clock, or a reboot, gives a gap for review instead
  of a duration.
- **BUC-EMP-141 — Prove the time crossing in both languages.** Go and
  Kotlin load the same `trip/trip-time-v1.psv` oracle, assert its hash,
  counts and error codes, and cover the full 0001..9999 instant range, the
  largest monotonic counts and tolerances without overflow, and the named
  negatives: a local clock change inventing duration, midnight splitting
  one trip and a lost reporting basis.

## AUTH catalog amendment org-permissions-v5 — reservation and checklist grants (2026-11-15)

Evidence.

- **B-BR-EMP-292 — Reservations and checklist handling have named roles.**
  The permission catalog `org-permissions-v5` grants, with organization or
  assigned-scope coverage (`O`) and only the four standing conditions (active
  membership, live scoped assignment, organization and plan, record rules):
  `fleet.reservation.manage` (request, confirm and cancel reservations; block
  and unblock assets) to `OWNER`, `ADMIN`, `FLEET_MANAGER` and `SUPERVISOR`,
  exactly the roles of `fleet.asset.assign`; `check.template.manage`
  (checklist templates and the critical-item policy) to `OWNER`, `ADMIN` and
  `FLEET_MANAGER`, exactly the roles of `maint.workorder.manage`;
  `check.incident.manage` (handle reports, release a blocked asset, open claim
  records) to `OWNER`, `ADMIN`, `FLEET_MANAGER` and `SUPERVISOR`. Every other
  cell of `org-permissions-v4` is unchanged: `AUDITOR`, `FINANCE`, `APPROVER`,
  `DRIVER`, `OPERATIONS_STAFF` and the station and data roles gain nothing, and
  the 18 other unassigned keys stay denied to every role. A role assigned to a
  unit, cost centre, team or asset applies these grants inside that scope only.
- **B-BR-EMP-293 — A catalog version changes grants, not stored data or
  routes.** Role assignments store the role and its scope, never a permission
  set or a catalog version, so a current assignment holds the new grants at
  its next request evaluation and nothing is migrated. Each earlier oracle
  (`v1`–`v4`) stays unchanged beside the new one, and every consumer that pins
  the version (route inventory, authorization matrix, export gating) moves to
  `v5` in the same change. A granted key admits no request until its owning
  task routes it; separation-of-duty record rules (for example a release by
  someone other than the author, BR-CHECK-011) remain obligations of that task.

## Reservations, blocks and derived availability (2026-11-15)

Evidence.

- **B-BR-EMP-294 — A reservation names one active asset, a future window, a
  purpose and optionally one employee record.** A reservation belongs to one
  organization and names one of its assets in `ACTIVE`, a half-open window
  `start, end)` with whole-microsecond bounds that starts later than the
  server instant of the request and ends after its start, a purpose of 1 to 280
  characters and, when known, one `ACTIVE` employee record of the organization
  as the reserved employee. Its state is one of `REQUESTED`, `CONFIRMED`,
  `FULFILLED`, `CANCELLED`, `NO_SHOW`, `EXPIRED`; it is created `REQUESTED`,
  or `CONFIRMED` when the request says so. `FULFILLED` has no source until a
  trip intake exists (EMP-TRIP-023): no command or job reaches it. The no-show
  tolerance is the 60-minute default of fleet.md §6.4, stored with the
  reservation. An asset in another state is refused with `VALIDATION_FAILED`;
  an asset or employee of another organization (or unknown) is not found.
  Rows are private under forced row-level security and never deleted; the
  state changes only through the rows of fleet.md §6.4.
- **B-BR-EMP-295 — Confirmed windows never overlap; a conflict reveals only
  the window.** A request or a confirmation whose window overlaps a
  `CONFIRMED` reservation of the asset, or a block of the asset whose period
  overlaps it, changes nothing and answers `RESERVATION_CONFLICT` (409) whose
  details carry only the conflicting window — its start and, when it has one,
  its end — never the other record's identifier, requester, employee,
  purpose, note or kind. `REQUESTED` windows may overlap each other until one
  is confirmed. The database enforces non-overlap with an exclusion
  constraint and refuses a confirmation over a block; commands are serialised
  under the organization root lock, so of two concurrent overlapping
  confirmations exactly one commits and the other receives the winner's window.
- **B-BR-EMP-296 — Time-driven outcomes are derived at read time, settled by a
  job and before any reservation command.** A `REQUESTED` reservation is
  `EXPIRED` from its start; a `CONFIRMED` one is `NO_SHOW` from its no-show
  deadline, the earlier of start plus tolerance and its end, because no
  qualifying trip can start in this cut. Every read reports the effective state
  at its own server instant, with the instant at which the outcome happened,
  even when nothing was stored yet. A per-reservation job queued in the
  command transaction for the due instant stores the outcome once
  (compare-and-set: a second run changes nothing), and every reservation
  command first settles the due outcomes of the asset, so a stopped job never
  keeps a stale window. Each stored outcome records one audit event
  (`fleet.reservation.expired`, `fleet.reservation.missed`) with the requesting
  account as actor.
- **B-BR-EMP-297 — Cancelling releases the window; leaving service cancels.**
  A reservation that is `REQUESTED` or `CONFIRMED` and has not reached its
  time-driven outcome can be cancelled; a reason of 1 to 280 characters is
  required unless the requesting account cancels. A cancelled, expired or
  missed reservation refuses confirmation and cancellation with `CONFLICT` and
  changes nothing. When an asset becomes `INACTIVE` or `DISPOSED`, every
  reservation of it that is `REQUESTED` or `CONFIRMED` and has not reached its
  time-driven outcome is `CANCELLED` in the same transaction, with the cause
  `ASSET_LEFT_SERVICE` and the asset revision.
- **B-BR-EMP-298 — Blocks state a reason kind and a period and are removed
  only with a reason.** A block names an asset in `ACTIVE` or `INACTIVE`, a
  reason kind (`ADMINISTRATIVE`, `SAFETY`, `DOCUMENT`, `INVESTIGATION`,
  `LENT_OUT`), an optional note of up to 280 characters and a half-open period
  whose start is the server instant when omitted or a later instant when
  given, with an optional end. Removing a block requires a reason of 1 to 280
  characters (its absence is `VALIDATION_FAILED`) and ends it at the server
  instant, or at its start when it has not started; removing a removed or
  ended block is `CONFLICT`. Blocks are never deleted. Creating a block never
  cancels or changes a reservation nor interrupts an operation in progress.
- **B-BR-EMP-299 — Availability is derived on read and lists every cause.**
  At the server instant, availability is the first applying row of fleet.md
  §6.2 — `UNAVAILABLE` (lifecycle not `ACTIVE`), `IN_USE` (a trip in progress
  or paused), `IN_MAINTENANCE` (an open hold), `BLOCKED` (a block covering the
  instant), `RESERVED` (a confirmed reservation holding the asset at the
  instant), otherwise `AVAILABLE` — and the answer lists every applying cause
  in that order with its kind, record identifier and period (and the block
  reason kind), never a person, purpose or note. A reservation holds the asset
  from its start until the earliest of its end, its no-show deadline and its
  cancellation (only if it was confirmed). Trips and maintenance holds are
  answered by ports that, until their owners exist, answer "no source"; the
  answer then names them as missing sources and never claims their absence.
  Availability is never stored.
- **B-BR-EMP-300 — A start is admitted or held with a precise code.** The
  intake decision for a start of an asset by a member at a declared instant
  holds the start for review with `ASSET_UNAVAILABLE` when the asset is not
  `ACTIVE`, in maintenance, blocked, held by a reservation that does not name
  the member's employee record, or available but not assigned to the member,
  and with `ASSET_BUSY` when another trip holds it; a reservation naming the
  member admits the start without an assignment. A trip already in progress
  is never re-decided, so a block added during a trip applies to the next
  start. The decision is exposed for the trip-start intake (EMP-TRIP-002); no
  start intake exists yet.
- **B-BR-EMP-301 — Reservation and block commands need fresh
  `fleet.reservation.manage` on the asset; reads need `fleet.asset.read`.**
  Request, confirm, cancel, block and unblock require fresh
  `fleet.reservation.manage` covering the asset (`ORG`, or a `UNIT`,
  `COST_CENTRE` or `ASSET` grant matching the asset's allocation at the server
  instant); the reservation list, block list and availability read require
  current `fleet.asset.read` covering the asset, including `SELF` on an asset
  assigned to the member. Every write records exactly one audit event and one
  receipt bound to its operation identifier; the same intent replayed returns
  the first result, another intent under the same identifier is
  `OPERATION_ID_REUSED`.
- **BUC-EMP-148 — Request and confirm a reservation.** Actor: member with
  fresh `fleet.reservation.manage` covering the asset. Main flow: choose the
  window, the purpose, optionally the reserved employee and whether to confirm
  → the server settles due outcomes of the asset, checks B-BR-EMP-294/295
  under the organization root lock → reservation `REQUESTED` or `CONFIRMED`,
  one audit event and receipt, its time-driven outcome queued; a `REQUESTED`
  reservation is confirmed by a later command. Alternatives: overlap with a
  confirmed reservation or a block → `RESERVATION_CONFLICT` with only the
  window; asset not `ACTIVE`, employee not `ACTIVE` or window not in the
  future → `VALIDATION_FAILED`; supervisor scoped to another unit →
  `FORBIDDEN`; replay → first result. Result: the asset is held for the
  window against others only.
- **BUC-EMP-149 — Cancel a reservation; outcomes follow the clock.** Actor:
  member with fresh `fleet.reservation.manage`; time. Main flow: cancel with a
  reason (none needed by the requester) → `CANCELLED`, window released; or the
  start passes unconfirmed → `EXPIRED`; or the deadline passes → `NO_SHOW`,
  window released. Alternatives: ended reservation → `CONFLICT`; job stopped →
  reads still show the outcome; job run twice → one stored outcome. Result:
  every reservation reaches a terminal state with its cause.
- **BUC-EMP-150 — Block and unblock an asset.** Actor: member with fresh
  `fleet.reservation.manage`. Main flow: choose reason kind, optional note,
  start and end → block recorded, the asset is `BLOCKED` for new starts →
  remove with a reason. Alternatives: removal without reason →
  `VALIDATION_FAILED`; already removed → `CONFLICT`; lending the asset out →
  `LENT_OUT` with the expected return as end; trip in progress → it continues.
  Result: block recorded and audited.
- **BUC-EMP-151 — Read availability, reservations and blocks.** Actor: member
  with current `fleet.asset.read` covering the asset. Main flow: the
  availability with every cause and the missing sources at the server
  instant; the reservations with their effective states; the blocks with
  their removal facts. Alternatives: foreign or unknown asset → `NOT_FOUND`;
  no covering grant → `FORBIDDEN`. Result: read only; nothing is stored.

## Reading validation: monotonicity, retroactive inserts and suspect flags (2026-11-15)

[Evidence.

- **B-BR-EMP-280 — A reading is checked against its valid neighbours in
  its segment.** The series of a segment is ordered by `occurred_at`, then
  server `received_at`, then reading identifier (BR-METER-008). The
  predecessor of a candidate is the last reading ordered before it whose
  current state is `VALID`; the successor is the first `VALID` reading
  ordered after it. `SUSPECT` and `VOIDED` readings are never neighbours (a
  suspect reading does not contaminate the check of the next one). The
  segment's opening reading is its first reading; a candidate ordered before
  it is outside the segment and is refused exactly as a reading outside the
  meter's service interval already is (signed intake `MALFORMED`, embedded
  path `VALIDATION_FAILED`), never stored.
- **B-BR-EMP-281 — Monotonicity flags.** A candidate whose value is lower
  than its predecessor's is flagged `READING_BELOW_PREVIOUS`; one higher than
  its successor's is flagged `READING_ABOVE_NEXT`; a value equal to a
  neighbour's passes. A meter with a modulus gets no rollover verdict here
  (`POSSIBLE_ROLLOVER` belongs to EMP-METER-005); a lower value is
  `READING_BELOW_PREVIOUS`.
- **B-BR-EMP-282 — Plausible rate with exact integers.** For each new pair of
  consecutive valid readings that the candidate forms (predecessor →
  candidate and candidate → successor) whose values do not decrease, with
  value difference `Δv` in base units and elapsed time `Δt` in nanoseconds
  between the two `occurred_at` instants, an odometer pair is plausible when
  `Δv × 3 600 × 10⁹ ≤ L × Δt + 3 600 000 × 10⁹` (`L` the maximum average
  distance in metres per hour: default 150 000, configurable from 20 000 to
  200 000, a hypothesis per category; the constant allows 1 000 m of display
  rounding) and an hour-meter pair is plausible when
  `Δv × 10⁹ ≤ Δt + 300 × 10⁹`. Equal instants are a pair with `Δt = 0`, so
  two readings declared at the same instant may differ by at most 1 000 m
  or 300 s. An implausible pair flags the candidate `IMPLAUSIBLE_RATE` once.
  The comparison uses exact integers of unbounded width (no floating point,
  no rounding, no overflow).
- **B-BR-EMP-283 — Future instants.** A candidate whose `occurred_at` is
  later than its receipt instant plus the clock-skew limit (default 300 s,
  any non-negative number of whole seconds) is flagged `FUTURE_INSTANT`;
  exactly the limit passes. The receipt instant is the server clock read
  under the meter lock immediately before the insertion; for a held reading
  applied by an accepted review it is the original receipt. The flag is
  decided once, at intake: re-evaluation keeps it, and only a confirmation,
  correction or void (EMP-METER-004) removes it.
- **B-BR-EMP-284 — Classification, storage and the current value.** A
  candidate with at least one flag is `SUSPECT`, otherwise `VALID`; a
  candidate that has a successor carries the note `RETROACTIVE` (it was
  placed before a later valid reading), which is never a reason for
  `SUSPECT`. A failing reading is never refused for its flags: it is stored,
  its signed operation is answered `ACCEPTED` with entity revision 1, and the
  trip or fueling that carries it is not held. The reading row keeps its
  recorded classification (`VALID` or `SUSPECT`) and an append-only state
  history keeps revision 1 (`INTAKE`, flags in canonical order
  `READING_BELOW_PREVIOUS`, `READING_ABOVE_NEXT`, `IMPLAUSIBLE_RATE`,
  `FUTURE_INSTANT`, note, operation) and every later revision; the current
  state of a reading is its latest revision, or its recorded classification
  when it has no history (readings recorded before this rule). The
  latest-valid reading, the current value and every usage read use only
  readings whose current state is `VALID`, so a suspect reading never moves
  the current value. The opening reading recorded with a meter installation
  is the segment's anchor and is recorded `VALID` without checks and without
  history. The validation is a server configuration that is on by default;
  turned off, readings are stored `VALID` without flags and without history,
  exactly as before this rule, and nothing is re-evaluated.
- **B-BR-EMP-285 — Re-evaluation of suspect neighbours.** When a `VALID`
  reading enters a segment, in the same transaction and under the same meter
  lock every `SUSPECT` reading of the segment is checked again in series
  order against the then-valid neighbours, repeating until a full pass
  promotes nothing: a reading that now passes becomes `VALID` (cause
  `REEVALUATION`, the triggering operation), a reading whose flags changed
  gets a new `SUSPECT` revision with the new flags, `FUTURE_INSTANT` is kept,
  and a `VALID` reading is never demoted by the system. The device uses the
  same validator on its last known readings to warn the person; the warning
  never blocks saving or sending, and the server decides.
- **BUC-EMP-142 — Record a field reading that is validated.** A covered
  member's device sends a standalone reading (or a fueling with its reading);
  the server orders it in its segment, checks it against the valid
  neighbours, stores it `VALID` or `SUSPECT` with its flags and note, answers
  `ACCEPTED`, and the current value moves only for a `VALID` reading.
- **BUC-EMP-143 — Insert a retroactive reading.** A reading recorded offline
  earlier arrives after later readings; it is placed by `occurred_at`, is
  `VALID` with `RETROACTIVE` when it lies between its neighbours (the two new
  intervals sum to the previous one) or `SUSPECT` otherwise, and suspect
  readings that it makes consistent return to `VALID`.
- **BUC-EMP-144 — Warn on the device without blocking.** The app checks a
  typed value with the shared validator against the last known readings and
  shows the flags as a warning; the person can still save and send it.

## Reading correction and void with audit (2026-11-15)

Evidence.

- **B-BR-EMP-286 — One decision command per reading.** A member with
  `meter.reading.correct` covering the reading's asset decides one reading of
  a meter's open segment through one company command naming the reading, the
  expected current history revision of that reading (0 when it has none), a
  decision `SUPERSEDE`, `VOID` or `CONFIRM`, and a reason (1 to 280
  characters, no control characters). A `SUPERSEDE` also states the meter
  kind and the corrected value in exact base units, and optionally a corrected
  declaration (instant, zone and offset together). The command needs fresh company proof,
  the enabled organizations capability and an `ACTIVE` organization. A
  `VOIDED` target is refused `READING_ALREADY_SUPERSEDED` before any other
  state check; a stale expected revision is `CONFLICT`; the segment's
  installation opening reading is its anchor and is refused
  `VALIDATION_FAILED` (replacement and reset belong to EMP-METER-005). A
  reading of another organization, asset or meter is `NOT_FOUND`.
- **B-BR-EMP-287 — Supersession and void.** `SUPERSEDE` records a new
  reading with the corrected value and either the original declaration or a
  corrected one (instant, zone, offset), the deciding member as author, the
  company web session as capture, the original's origin, method `TYPED` and
  the command's operation; the original becomes `VOIDED` (cause
  `SUPERSEDED`, linked to the new reading, actor and reason) and is never
  changed otherwise; the new reading is classified by the EMP-METER-003
  validator against the series without the original, so a correction that
  still breaks monotonicity is stored `SUSPECT` and its flags are returned.
  `VOID` makes a `VALID` or `SUSPECT` reading `VOIDED` (cause `VOIDED`) with
  actor and reason. `VOIDED` is terminal; a reading is superseded at most
  once.
- **B-BR-EMP-288 — Confirmation.** `CONFIRM` makes a `SUSPECT` reading
  `VALID` (cause `CONFIRMED`) only when its flags are `IMPLAUSIBLE_RATE` and/or
  `FUTURE_INSTANT`; a reading flagged `READING_BELOW_PREVIOUS` or
  `READING_ABOVE_NEXT` cannot be confirmed (`VALIDATION_FAILED`), and a
  reading that is not `SUSPECT` cannot be confirmed (`CONFLICT`). Monotonicity
  flags clear only when a neighbour is corrected or voided (BR-METER-012).
- **B-BR-EMP-289 — Recalculation in the same transaction.** Every decision
  re-evaluates the segment's `SUSPECT` readings exactly as B-BR-EMP-285, under
  the root, asset and meter locks, in the command's transaction and under its
  operation, so correcting one reading returns its now consistent suspect
  neighbours to `VALID`. The decision, the new reading, every new history
  revision, one audit event (`meter.reading.corrected`, `meter.reading.voided`
  or `meter.reading.confirmed`; actor, target reading, command digest; no
  value, instant or reason) and, when a consumer is configured, one
  `meter.usage.recalculated` intent per consumer (identifiers and the target
  reading's new revision only) commit together or not at all. The same
  operation with the same digest returns the first result; with another
  digest it is `OPERATION_ID_REUSED`. Concurrent decisions on one reading
  serialise: the first wins, the second sees the result
  (`READING_ALREADY_SUPERSEDED` or `CONFLICT`).
- **B-BR-EMP-290 — Approved parent records need an amendment.** When the
  reading belongs to a parent record that is `APPROVED` (today a fueling, by
  its review state), the decision is refused `AMENDMENT_REQUIRED` and nothing
  changes, because no amendment consumer can apply it yet (EMP-PLAT-016 owns
  the kernel); a parent in any other review state is decided directly.
- **B-BR-EMP-291 — History and suspect list.** The history of a meter lists
  every reading of its open segment in series order with value, both
  instants, origin, method, author, capture, recorded and current state,
  flags, note, current revision, the supersession links in both directions
  and the reason of a person decision, filterable by current state, keyset
  paginated, under `fleet.asset.read` (SELF on an assigned asset included).
  The suspect list returns the meter's `SUSPECT` readings with their flags and
  their current valid neighbours, under `meter.reading.correct`.
- **BUC-EMP-145 — Correct a typing error.** A supervisor opens the suspect
  list, supersedes the wrong reading with the right value and a reason; the
  original stays visible as `VOIDED`, the correction becomes current, and
  readings that were suspect because of the error return to `VALID`.
- **BUC-EMP-146 — Void a reading.** A supervisor voids a reading recorded for
  the wrong asset with a reason; it leaves the current value and usage and
  stays visible.
- **BUC-EMP-147 — Confirm a rate or future flag.** A supervisor confirms an
  implausible-rate or future-instant reading with a reason; a monotonicity
  flag cannot be confirmed away.

## Post-revocation intake window and quarantine (2026-11-15)

Evidence.

- **B-BR-EMP-302 — Intake-only late source.** A device whose exact
  enrollment period was revoked `ACCESS_ENDED` — by the end of its membership
  (EMP-IDN-010) or by an `ACCESS_ENDED` revocation command (EMP-IDN-015) —
  keeps an intake-only source for operations sealed to that period, through
  the existing signed route only. The source needs current personal proof
  (active, non-managed account, live family and token, signing key bound to
  the account), the same organization root `ACTIVE` with its base capability
  enabled, the period still owned by the same account and key, and no
  `LOST_OR_COMPROMISED` revocation of that key anywhere in the organization.
  It confers no permission, role, record read, receipt read or review-result
  read: every current route, including the signed review-results read, keeps
  refusing the ended member. A key revoked `LOST_OR_COMPROMISED` is refused as
  a whole with `DEVICE_REVOKED` before any claim, receipt, audit or fact; an
  unknown, foreign or still-enrolled period keeps its existing refusal.
- **B-BR-EMP-303 — Server history decides; device time is evidence.** The
  access end instant E is the membership's `ended_at` when the enrollment was
  revoked by the membership end, else the enrollment's `revoked_at`. The
  window W is `policies.intake_window_days` of the settings version in force
  at E (latest `effective_at ≤ E`, ties by revision), 15 days when no version
  exists; a later settings change neither reopens nor shortens it. Receipt R
  is the database clock of the final acknowledgement. When R > E + W × 86 400
  seconds the operation is `REJECTED` `INTAKE_WINDOW_CLOSED` (the window
  includes its end instant). Otherwise it is eligible for review when the
  server already holds its claim from before E (`claimed_at < E`) or, failing
  that, when its declared instant is before E; else it is `REJECTED`
  `MEMBERSHIP_ENDED` (membership end) or `DEVICE_REVOKED` (device command
  end). The declared instant can only choose review over refusal: it never
  applies an operation, never moves the window and never opens a read. The
  enrollment's last contact is not recorded and is not evidence.
- **B-BR-EMP-304 — Quarantine, never application.** An eligible operation is
  `ACCEPTED_FOR_REVIEW` with `ACCESS_ENDED_BEFORE_RECEIPT` only when its
  installed owner retains a typed held fact with a resolution path. The meter
  owner holds the reading as an OFFL-008 review item and held reading with
  version-1 proof after validating the payload and the target in the
  organization without current authority; a malformed payload, a target that
  does not exist in the organization or a declaration outside the meter's
  service window is `REJECTED` `MALFORMED`. A held fact creates no reading,
  effect, cost or dependency readiness until a reviewer who is not the author
  resolves it (OFFL-008). An owner without a held path answers eligible work
  `RETRY`/`UNAVAILABLE` with no claim — fuelings until FUEL-003 (EDR-177) —
  while its `MEMBERSHIP_ENDED`, `DEVICE_REVOKED` and `INTAKE_WINDOW_CLOSED`
  refusals are final. An eligible operation with unaccepted hard dependencies
  is `RETRY`/`DEPENDENCY_PENDING` with its claim kept (BR-OFFL-007).
- **B-BR-EMP-305 — One recorded decision per late operation.** Every final
  late acknowledgement commits with one immutable, forced-RLS
  `org_sync_late_intakes` row holding its evidence: end source and instant,
  window and settings revision, declared instant, receipt, disposition,
  result, code and policy `late-intake-v1`. The database re-derives E, W and
  the disposition from history and refuses a mismatching row; the codes
  `ACCESS_ENDED_BEFORE_RECEIPT`, `MEMBERSHIP_ENDED`, `INTAKE_WINDOW_CLOSED` and
  `DEVICE_REVOKED` are stored only with their decision row; a late held meter
  reading is stored only for an `ACCESS_ENDED` period whose declaration or
  earlier claim precedes E.
- **B-BR-EMP-306 — Replay never restores access.** A late resend of an
  operation with a late decision returns its stored acknowledgement byte for
  byte, also after the window closed or the review was resolved, and applies
  nothing; the same identifier with another digest is `REJECTED`
  `OPERATION_ID_REUSED` without storage; a resend of an operation finalized
  through the normal path before the end is `RETRY`/`UNAVAILABLE` and its
  stored acknowledgement is unchanged. Concurrent identical late deliveries
  leave one claim, one decision and one held fact.
- **B-BR-EMP-307 — Late routing and isolation.** The signed route enters the
  late path only when the current device check refuses the signing key as
  revoked or ended, or when the batch's sealed period differs from the
  current one; the period is read from the batch's operations with the full
  frozen parser and must be the exact owned period of the signing key in the
  URL organization (another organization's period is `NOT_FOUND`). The late
  source is confirmed again inside every operation transaction and once more
  before the response; a failure withholds the whole response (OFFL-002).
- **BUC-EMP-152 — Deliver after offboarding (UC-OFFL-07).** A driver
  offboarded while offline reconnects inside the window: readings declared
  before the end are held `ACCESS_ENDED_BEFORE_RECEIPT` and wait for a
  manager, a reading declared after the end is `MEMBERSHIP_ENDED`, resends
  return the same acknowledgements, and the driver can read nothing.
- **BUC-EMP-153 — Closed window and lost device.** After the window every
  late operation is `INTAKE_WINDOW_CLOSED`, whatever its declared time; a
  device revoked as lost or compromised is refused as a whole with nothing
  stored.
- **BUC-EMP-154 — Prove late intake on real PostGIS.** Server-history
  evidence, a forged declared time, the window in force at the end instant,
  earlier-claim evidence, exact replay, 12-way concurrency, intake racing the
  membership end in both orders, foreign organization, database guards and a
  populated upgrade with injected failure and recovery.

## Organization-state and plan-change handling for queued operations (2026-11-15)

Evidence.

- **B-BR-EMP-308 — Organization state decides before the owner.** Every new
  signed operation of a current member's enrolled device is first assessed
  against the organization's recorded suspension history (EMP-ORG-009
  `AssessSuspension`: server-recorded periods, the database receipt, a 7-day
  grace that includes its end instant) and, when that allows it, against the
  plan rule (B-BR-EMP-309); only then does the owning handler run. A
  suspension `REVIEW` is `ACCEPTED_FOR_REVIEW` `ORG_SUSPENDED_GRACE`; a
  `REFUSE` `ORG_SUSPENDED` is a final `REJECTED` `ORG_SUSPENDED` with nothing
  stored; archived, setup and closed refusals keep their whole-response codes.
  While the organization is `SUSPENDED` the signed route admits a non-empty
  batch from the current member's enrolled device to this assessment only: it
  never reaches an owner's write or read authority, and an empty batch keeps
  `ORG_SUSPENDED`. After reactivation the same history still holds work
  declared during the resolved interval and refuses work that missed its grace.
- **B-BR-EMP-309 — A plan change is the capability's recorded state.** No
  billing exists: the plan is the operation's write capability
  (`organizations` for meter and fueling). Enabled at receipt, the owner
  decides as before. Disabled at receipt after a change, the instant the
  current state was set is the downgrade instant: work declared before it is
  `ACCEPTED_FOR_REVIEW` `PLAN_CHANGED`; work declared at or after it, or under
  a capability created disabled or absent, is new forbidden work and the owner
  keeps refusing it with `ENTITLEMENT_REQUIRED`. A plan change never changes a
  membership, role or enrollment (BR-AUTH-009).
- **B-BR-EMP-310 — State work is quarantined, recorded and replayed.** A held
  state decision uses the owner's typed held path (the meter's OFFL-008 review
  with the given code); an owner without one answers `RETRY`/`UNAVAILABLE`
  with no claim (fuelings until FUEL-003). Every final state acknowledgement
  commits with one immutable forced-RLS `org_sync_state_intakes` row
  (condition, its server instant, declaration, receipt, result and code); the
  database re-derives the condition from history and stores
  `ORG_SUSPENDED_GRACE`, `ORG_SUSPENDED` and `PLAN_CHANGED` only with such a
  row. A resend returns the stored acknowledgement byte for byte through that
  row, also while suspended, and never applies, re-decides or adds a cost; the
  same identifier with another digest is `OPERATION_ID_REUSED`.
- **B-BR-EMP-311 — Device time never bypasses a block.** The declared instant
  only chooses review over refusal: work declared before a suspension or a
  downgrade is reviewed, never applied; a declaration cannot extend the grace;
  current permission never certifies work recorded while a suspension was
  open. Another organization's history never decides, and its member, device
  or operation is not found.
- **BUC-EMP-155 — Deliver into a suspended organization (VAL-22 part 2).** A
  reading declared before the suspension and delivered inside the grace is
  held `ORG_SUSPENDED_GRACE`; one declared after the suspension is
  `ORG_SUSPENDED` and stays visible with its reason; after reactivation new
  readings apply, resends replay, and nothing was deleted or reassigned.
- **BUC-EMP-156 — Deliver across a plan downgrade (VAL-24/35).** A reading
  declared before the capability was turned off is held `PLAN_CHANGED`; one
  declared after it is refused `ENTITLEMENT_REQUIRED`; no role, membership or
  enrollment changed; held work is never counted until a reviewer decides.

## Trip persistence and sync handlers with the one-active-trip-per-asset rule (2026-11-15)

Evidence.

- **B-BR-EMP-312 — One trip per signed `trip.start`.** The operation
  `trip.start` schema 1 (entity `trip`, id = the device trip UUID, no
  expected revision, dependency, soft reference or attachment) carries the
  exact payload `asset_id`, `driver_employee_id`, `reading` (`meter_id`,
  `segment_id`, `reading_id`, `kind`, `value`, or null),
  `missing_reading_reason` (or null) and `run_clock` (`boot_id`,
  `elapsed_millis`, or null) of canonical strings; the envelope declaration
  is the start declaration. It is validated through the EMP-TRIP-001
  `Start` and stores one organization-scoped, forced-RLS trip: identity,
  operation and digest, asset, driver, author membership and its employee
  record, capture enrollment, the declaration (microsecond floor plus
  0..999 remainder, entry zone, device offset), the optional run clock, the
  FLEET-005 unit and cost-centre snapshot at the declared instant, the start
  reporting day attributed in that unit's zone or else the organization's
  zone, the departure reading or missing reason and the server receipt;
  state IN_PROGRESS. A trip identity already used by another operation is
  `MALFORMED`; a `DRAFT` asset or a driver that is not an active, unmerged
  employee is `MALFORMED`.
- **B-BR-EMP-313 — One active trip per asset (VAL-07).** At most one
  IN_PROGRESS trip per asset is a database invariant. The intake decides
  under the organization root and asset row locks: the first admissible
  start received is `ACCEPTED`; a later admissible start on an asset with a
  trip in progress is kept as a held start `ACCEPTED_FOR_REVIEW`
  `ASSET_BUSY`; an asset that is not ACTIVE at receipt, or a start declared
  at or after its disposal, is held `ASSET_UNAVAILABLE`; a start whose right
  changed after its declaration (EDR-118 continuity) is held
  `PERMISSION_CHANGED`. The FLEET-007 start decision is consulted in the
  same transaction after those checks: a block, a confirmed reservation of
  another employee, maintenance, a missing assignment or a trip holding the
  declared instant holds the start `ASSET_UNAVAILABLE` or `ASSET_BUSY`. A
  held start stores its whole fact, its reading values and a version-1
  review item in the same transaction as its acknowledgement; it creates no
  trip and no reading and is never active use. Its resolution belongs to
  EMP-TRIP-007.
- **B-BR-EMP-314 — Reading evidence commits with its event.** Exactly one
  of a reading or a missing reason (the METER-001 profile) is required. A
  departure reading is recorded through the fleet embedded-reading port with
  origin `TRIP_START`, a final reading with `TRIP_END`, the event's
  operation, its declaration and method `TYPED`, in the same transaction as
  the event; a rollback leaves neither. A reading of another kind, of a
  meter of another asset or outside the meter's service interval is
  `MALFORMED`; an unknown meter or segment is `RETRY` `DEPENDENCY_PENDING`
  with nothing stored. Plausibility never refuses an event (B-BR-EMP-273).
- **B-BR-EMP-315 — One operational completion per trip through
  `trip.finish`.** The operation `trip.finish` schema 1 (entity `trip`, id =
  the trip UUID, no expected revision, at most one dependency, no soft
  reference or attachment) carries the exact payload `reading`,
  `missing_reading_reason` and `run_clock`. It stores one insert-only finish
  fact (operation, digest, author, enrollment, declaration, run clock, the
  informational end day in the stored reporting zone, final evidence,
  receipt) and moves the trip from IN_PROGRESS to COMPLETED in the same
  transaction, the only update a trip ever receives. Only the member who
  started the trip finishes it (another member is a whole-response
  `FORBIDDEN`); a finish declared before the start instant is `REJECTED`
  `MALFORMED`; a trip that is not stored in this organization (missing,
  held or another organization's) is `RETRY` `DEPENDENCY_PENDING` with
  nothing stored; a trip already completed by another finish operation
  keeps the new finish fact as `ACCEPTED_FOR_REVIEW` `TRIP_ALREADY_CLOSED`.
- **B-BR-EMP-316 — Authority at receipt.** Both operations need current
  `trip.trip.start` on the enrolled device. History follows EDR-118
  continuity through the generalised classifier (EDR-177): a positive `ORG`
  rule covers the member's own trip; `SELF` covers it only while the asset
  is assigned to the member (FLEET-006 periods) and the member's own employee
  record is the driver. Never covered is `FORBIDDEN` and a missing or
  foreign asset `NOT_FOUND`, both refusing the whole response with nothing
  stored. A receipt replay needs current `trip.trip.read` over the stored
  trip (author, asset, stored unit and cost centre), except the author's own
  held receipt.
- **B-BR-EMP-317 — Exact replay and concurrency.** The intake ledger
  decides by operation id: the same id and digest returns the stored
  acknowledgement byte for byte and creates nothing; the same id with
  another digest is `REJECTED` `OPERATION_ID_REUSED`; concurrent identical
  operations leave one fact. Concurrent admissible starts from two devices
  on one asset leave exactly one IN_PROGRESS trip and one held `ASSET_BUSY`
  start in either order, and a replay of the held start returns its held
  acknowledgement.
- **B-BR-EMP-318 — Review state and active use.** A stored trip begins its
  review case as the APPR-001 draft (DRAFT, revision 1, no entry); neither
  start nor finish submits it (EMP-TRIP-004 owns submission). An IN_PROGRESS
  trip is open work: FLEET-003 deactivation and disposal of its asset are
  refused with the `TRIP` open-work kind. Stored trips are the FLEET-007
  active-use source: a trip started at or before an instant and not finished
  at or before it holds the asset then, so availability reads `IN_USE` and no
  longer lists active use among its missing sources. A block added during a
  trip never interrupts it and applies to the next start.
- **B-BR-EMP-319 — Bounded sources.** Trips, finishes and held starts and
  finishes are asset operations in the asset usage source (they fix the kind
  and keep a draft from being discarded); unit and cost-centre snapshots are
  associations, never current use. The owners declare no asynchronous
  follow-up for `ACCEPTED`/`REJECTED` and an atomic review for
  `ACCEPTED_FOR_REVIEW`. No route, panel read, OpenAPI path, review
  decision, own-result read for trips or Android change is added.
- **BUC-EMP-157 — Record a driver-initiated trip through the signed
  intake.** A covered driver's device sends `trip.start` with the departure
  reading 184 320 000 m and later `trip.finish` with 184 610 000 m; the
  server stores one IN_PROGRESS trip, then completes it with 290 000 m of
  meter evidence, each with one ACK in one transaction; resends return the
  same ACKs.
- **BUC-EMP-158 — Two devices start one vehicle (VAL-07).** Two drivers
  start the same vehicle offline; whichever arrives first is the active
  trip and the other is kept for review with `ASSET_BUSY`, never dropped or
  retried.
- **BUC-EMP-159 — Keep a second completion.** A second finish operation for
  a trip already completed is kept for review with `TRIP_ALREADY_CLOSED`.
- **BUC-EMP-160 — Prove the trip store on real PostGIS.** Replay, digest
  reuse, races with barriers in both orders, reading rollback in both
  directions, foreign asset, authority history, open work, forced RLS, the
  guarded state update and a populated 000215 → 000220 upgrade with injected
  failure, recovery and previous-image compatibility.

## Implements: attach to and detach from a powered asset (2026-11-15)

Evidence.

- **B-BR-EMP-325 — An implement works with at most one powered asset at a
  time.** An attachment belongs to one organization and names one of its
  assets of kind `IMPLEMENT` and one of kind `VEHICLE` or `MACHINE`, both in
  `ACTIVE` or `INACTIVE`, for a half-open period `start, end)` with
  whole-microsecond bounds; the start is the server instant when omitted and
  may not be earlier when given; the end is optional. An implement is
  attached to at most one powered asset at any instant: the database
  enforces it with an exclusion constraint, so of two concurrent overlapping
  attachments of one implement exactly one commits and the other is refused
  with `CONFLICT` and leaves nothing. A powered asset may carry several
  implements at once. Attaching to an implement, or attaching an asset that is
  not an implement, is `VALIDATION_FAILED`; an asset of another organization
  (or unknown) is not found. Rows are private under forced row-level security
  and never deleted.
- **B-BR-EMP-326 — Detaching ends the period, never before it began.** A
  detach ends an attachment that is not `ENDED` at the server instant, at the
  end the command states or at the server instant (at its start when it has
  not started, leaving an empty period). A stated end earlier than the server
  instant, earlier than the attach instant or later than a planned end is
  `VALIDATION_FAILED`; detaching an ended attachment is `CONFLICT`; an
  optional reason has 1 to 280 characters. The command names the attachment
  through either of its assets; through another asset it is not found.
- **B-BR-EMP-327 — An attached implement follows its powered asset's trips.**
  The implements attached to a powered asset at an instant, and the powered
  asset an implement is attached to at an instant, are answered by a port on
  the caller's organization transaction (half-open periods). An attached
  implement's availability lists, as `IN_USE` causes naming the powered
  asset, the trips in progress or paused of its powered asset at that instant;
  maintenance holds, blocks and reservations of the powered asset do not
  transfer to the implement. Without a trip source the answer names the
  missing source instead of claiming the implement is free of trips.
- **B-BR-EMP-328 — An implement is never a trip's main asset; an attachment
  fixes both kinds.** The trip-start eligibility check refuses an `IMPLEMENT`
  as the main asset of a trip (the trip intake applies it). An attachment
  counts as an operation of both assets, so neither asset's kind can change
  once an attachment names it (BR-FLEET-002).
- **B-BR-EMP-329 — Attachments are managed with `fleet.asset.manage` on both
  assets.** Attach and detach require fresh `fleet.asset.manage` covering the
  asset in the path and, inside the transaction, the other asset of the
  attachment (`ORG`, or a `UNIT`, `COST_CENTRE` or `ASSET` grant matching each
  asset's allocation); the list requires current `fleet.asset.read` covering
  the asset. Every write records exactly one audit event
  (`fleet.implement.attached` or `fleet.implement.detached`) and one receipt
  bound to its operation identifier; the same intent replayed returns the
  first result, another intent under the same identifier is
  `OPERATION_ID_REUSED`.
- **BUC-EMP-164 — Attach and detach an implement.** Actor: member with fresh
  `fleet.asset.manage` covering both assets. Main flow: choose the implement,
  the powered asset and the optional start and end → the server checks
  kinds, states and overlap under the organization root lock → attachment
  recorded with one audit event and receipt; later, detach with an optional
  end and reason. Alternatives: implement already attached in an overlapping
  period → `CONFLICT`; powered asset is an implement → `VALIDATION_FAILED`;
  detach before the attach instant → `VALIDATION_FAILED`; ended →
  `CONFLICT`; a grant scoped to another unit on either asset → `FORBIDDEN`.
  Result: the implement's history of powered assets by period.
- **BUC-EMP-165 — Read attachments and the implements of a trip start.**
  Actor: member with current `fleet.asset.read` covering the asset; the trip
  intake (system). Main flow: the attachments in which the asset is the
  implement or the powered asset, in start order with their state at the
  server instant; the trip start asks the port for the implements attached
  to the powered asset at its instant. Alternatives: foreign or unknown asset
  → `NOT_FOUND`. Result: read only.

## Reference data pull for the active organization: versioned deltas (2026-11-15)

[Evidence.

- **B-BR-EMP-336 — Purpose-bound reference content.** The reference feed of
  a selected organization contains only the assets currently assigned to the
  requesting member (EMP-FLEET-006 periods at the server instant) that the
  member's current `fleet.asset.read` grants cover (ORG; SELF through the
  assignment; UNIT and ASSET through the asset's current allocation). Each is
  one minimised `asset` entity: identifier, revision, kind, usage basis,
  state, name, fleet number, plate, the products its tanks accept under the
  `fuel-products-v1` catalog name, its in-service meters (identifier, kind,
  modulus, installation instant, open segment) and the covering assignment's
  kind and end. Nothing else is cached: no serial number, unit, cost centre,
  acquisition, value, other drivers, reservations or readings (BR-PRIV-003).
- **B-BR-EMP-337 — Cached data is a read copy, never authority.** Every
  response carries `confirmed_at`, the database clock after the final source
  check; the client shows that age and replaces nothing on its own clock.
  Every choice made from cached data is checked again at intake exactly as if
  no cache existed (BR-OFFL-016).
- **B-BR-EMP-338 — One scoped, signed cursor.** The feed is pulled on the
  signed device route with the current exact enrollment. The cursor is opaque
  and server-signed, and binds organization, membership, enrollment, feed
  epoch and revision; a cursor that does not verify for the exact requesting
  period is `NOT_FOUND` and reveals nothing.
- **B-BR-EMP-339 — Monotonic revisions with durable tombstones.** Each member
  has one feed whose revision only grows. A pull appends one revision per
  changed entity (an upsert when its minimised content changed, a `DELETE`
  tombstone when it left the authorized set, whatever the reason: unassigned,
  reassigned, disposed, archived or no longer covered). A delta answers every
  entity changed after the cursor with its latest state, ordered by revision,
  with `from_revision` and `revision` so a client can detect any gap; a
  tombstone is delivered even after its source row is gone.
- **B-BR-EMP-340 — Reset instead of stale authority.** No cursor, a cursor of
  another feed epoch or a revision ahead of the feed answers `RESET` with the
  complete current set; a member who no longer holds any covering
  `fleet.asset.read` grant gets `RESET` with reason `FORBIDDEN` and no
  entities, so the client must drop every cached entity of that partition and
  never keeps the old data.
- **B-BR-EMP-341 — Closed, versioned entity set.** Entity types are a closed
  set versioned with the response (`asset` today). Checklist templates
  (EMP-CHECK-001) and availability join only as new entity types through their
  owners; a client refuses an unknown type instead of guessing.
- **BUC-EMP-169 — Refresh reference data (UC-OFFL-08).** The app pulls with
  its last cursor, applies the ordered changes and tombstones, stores the new
  cursor and shows "confirmed at" from the server.
- **BUC-EMP-170 — A reassignment reaches the cache.** After a reassignment
  the next pull removes the asset with a tombstone; a reading already queued
  for it is decided at intake as before (held or refused), never by the cache.
- **BUC-EMP-171 — Prove the feed on real PostGIS.** Foreign and tampered
  cursors, tombstones after the cursor, monotonic gap-detectable revisions,
  revoked grants, server-clock confirmation, concurrent pulls, database
  guards and a populated upgrade.

## Checklist templates per asset category (2026-11-15)

Evidence.

- **B-BR-EMP-330 — A template version is one immutable, bounded
  definition.** A checklist template belongs to one organization and has a
  fixed context, `DEPARTURE`, `RETURN` or `AD_HOC`. Each version states a name
  of 1 to 80 characters, exactly one target — the organization default
  (`ORGANIZATION`), 1 to 100 distinct asset categories (`CATEGORIES`) or 1 to
  100 distinct assets (`ASSETS`) of the organization — and 1 to 40 ordered
  items, each with a label of 1 to 120 characters and three flags: critical,
  optional (`NOT_APPLICABLE` accepted) and photo required when not OK. A
  critical item is never optional. Names and labels are valid UTF-8, free of
  control characters and not blank. An item is identified by its position
  1..n inside its version. Any other shape is `VALIDATION_FAILED`; a category
  or asset of another organization, or unknown, is `NOT_FOUND`; an asset
  target must be `ACTIVE` or `INACTIVE` when the version is published (a
  draft, disposed or archived asset is `VALIDATION_FAILED`).
- **B-BR-EMP-331 — Publication appends and never rewrites.** Creating a
  template publishes version 1 with policy revision 1 and leaves the template
  `ACTIVE` at template revision 1. Publishing a change appends version n+1,
  changing the critical-item policy appends a policy revision, and archiving
  moves the template to `ARCHIVED`, which is terminal; each of these commands
  names the expected template revision and increments it by one. A stale
  revision or an archived template is `CONFLICT` and changes nothing.
  Versions with their items and targets, and policy revisions, are
  insert-only: the database refuses their update, delete or truncate, a gap
  or a repeated version or revision number, and an item or a target added to
  a committed version. A run of EMP-CHECK-002 names the version and the policy
  revision it used, so a later publication never changes a completed run
  (BR-CHECK-001).
- **B-BR-EMP-332 — One applicable template per asset and context.** Among the
  `ACTIVE` templates of one context, a category, an asset and the organization
  default are each named by at most one template's current version; a
  publication that would name a taken target is `CONFLICT`, decided under the
  organization root lock so that two concurrent publications cannot both take
  it. For an asset and a context, the applicable template is the `ACTIVE` one
  whose current version names the asset, else the one naming the asset's
  category, else the organization default, else none, and then no checklist
  is required (BR-CHECK-002). The applicable read answers, at its server
  instant, the template identifier, name, current version with its items, the
  match kind (`ASSET`, `CATEGORY`, `ORGANIZATION` or `NONE`) and the current
  policy revision with its mode, validity and effective instant — exactly what
  a run pins — and never the template's other targets.
- **B-BR-EMP-333 — The critical-item policy is revisioned per template.** A
  policy revision states the mode `BLOCK` (default) or `WARN` and the run
  validity in seconds from 3 600 to 259 200 (1 to 72 hours), default 43 200
  (12 hours) (BR-CHECK-005, BR-CHECK-012); any other value is
  `VALIDATION_FAILED`. Revisions are numbered from 1 and carry the server
  instant at which they took effect. The policy in force at an instant is the
  revision with the latest effective instant not after it, ties by the higher
  revision; before the first revision none is in force. Changing the policy
  creates no template version and rewrites no earlier revision.
- **B-BR-EMP-334 — Answers are checked and the outcome is computed by one
  shared rule.** For the items of a version and a policy, an answer set has
  exactly one entry per item: `OK`, `NOT_OK`, `NOT_APPLICABLE` or unanswered,
  each with an optional note of at most 140 control-free characters and an
  optional photo declaration. In this order: a malformed set (wrong count,
  unknown answer, invalid note) is `INVALID`; `NOT_APPLICABLE` on an item that
  is not optional is `ANSWER_NOT_ALLOWED`; `NOT_OK` without a photo on an
  item that requires one is `PHOTO_REQUIRED`; an unanswered item is
  `INCOMPLETE` (BR-CHECK-003, BR-CHECK-015). The outcome is `COMPLETED_OK`
  when no answer is `NOT_OK`, `BLOCKING` when a critical item is `NOT_OK`
  under `BLOCK`, and `COMPLETED_WITH_ISSUES` otherwise (BR-CHECK-004); the
  result lists the positions answered `NOT_OK` and the critical ones among
  them, and requires the author's acknowledgement exactly when a critical item
  is `NOT_OK` under `WARN` (BR-CHECK-005). Go and Kotlin share this rule, the
  target resolution and the policy-in-force lookup through one oracle; no
  route evaluates a run before EMP-CHECK-002.
- **B-BR-EMP-335 — Template commands need fresh `check.template.manage`; the
  applicable read needs `fleet.asset.read` on the asset.** Templates are
  organization-level resources: create, publish, policy and archive require
  fresh `check.template.manage` with `ORG` coverage (a `UNIT`, `COST_CENTRE`,
  `ASSET`, `TEAM` or `SELF` grant covers none), an `ACTIVE` organization and
  the organizations capability; the template list and read require current
  `check.template.manage` with `ORG` coverage. The applicable read requires
  current `fleet.asset.read` covering the asset (`ORG`, a `UNIT`,
  `COST_CENTRE` or `ASSET` grant matching its allocation at the server
  instant, or `SELF` on an asset assigned to the member). A template, category
  or asset of another organization is `NOT_FOUND` with no data. Rows are
  private under forced row-level security. Every write records exactly one
  audit event and one receipt bound to its operation identifier; the same
  intent replayed returns the first result, another intent under the same
  identifier is `OPERATION_ID_REUSED`.
- **BUC-EMP-166 — Create, publish and archive a template.** Actor: member
  with fresh `check.template.manage` at `ORG`. Main flow: choose the context,
  name, target and items, and optionally the policy → the server validates
  B-BR-EMP-330 and that the target is free under the organization root lock →
  template `ACTIVE`, version 1, policy revision 1, one audit event and
  receipt → later, publish a change with the revision read → version n+1, the
  earlier versions still readable; archive with the revision read →
  `ARCHIVED`, its targets free and no checklist required by it from then on.
  Alternatives: target taken by another `ACTIVE` template of the context →
  `CONFLICT`; stale revision or archived template → `CONFLICT`; a critical
  optional item, 41 items or a blank label → `VALIDATION_FAILED`; foreign or
  unknown category or asset → `NOT_FOUND`; supervisor or driver →
  `FORBIDDEN`; replay → first result. Result: an append-only version history.
- **BUC-EMP-167 — Set the critical-item policy.** Actor: member with fresh
  `check.template.manage` at `ORG`. Main flow: choose `BLOCK` or `WARN` and
  the run validity with the revision read → policy revision m+1 effective at
  the server instant; versions unchanged. Alternatives: validity outside 1 to
  72 hours or an unknown mode → `VALIDATION_FAILED`; stale revision or
  archived template → `CONFLICT`. Result: the policy history answers which
  revision was in force at any instant (the EMP-CHECK-002 server recheck).
- **BUC-EMP-168 — Read the applicable template of an asset.** Actor: member
  with current `fleet.asset.read` covering the asset, such as a driver on an
  assigned asset. Main flow: choose the context → the server resolves asset →
  category → organization default at its instant → the template version with
  its items and the current policy revision, or "no checklist required".
  Alternatives: foreign or unknown asset → `NOT_FOUND`; an unassigned asset
  for a `SELF` driver or another unit's asset for a `UNIT` supervisor →
  `FORBIDDEN`. Result: the run of EMP-CHECK-002 pins the version and the
  policy revision; nothing is stored by the read.

## Logbook events: the typed timeline (2026-11-15)

Evidence.

- **B-BR-EMP-320 — Closed logbook vocabulary.** A logbook event has exactly
  one type from the closed `trip-logbook-v1` set. Supported now: `NOTE`
  (text required), `STOP`, `ARRIVAL` and `DEPARTURE` (text optional), each an
  instantaneous fact of the trip. Reserved for their owners and never
  recorded here: `FUELING`, `TOLL` and `PARKING` (the fueling and expense
  links of EMP-FUEL-011, EMP-EXP-004 and EMP-TRIP-020), `INCIDENT` and
  `DAMAGE` (EMP-CHECK-003), `HANDOVER` (EMP-TRIP-012), `PAUSE`, `RESUME` and
  `CANCEL` (EMP-TRIP-011). Any other token — empty, padded, lowercase,
  `START`, `END`, `FINISH` — is unknown. A text is nonblank, free of control
  characters and at most 280 Unicode scalars. A planned-stop link and legs
  (EMP-TRIP-010), photos, amounts and corrections (EMP-TRIP-015) are not part
  of v1. No logbook event changes the trip's state, meter evidence, asset
  availability, open work, reporting day or review case, and none computes
  working time (EDR-028).
- **B-BR-EMP-321 — One logbook fact per signed `trip.event.record`.** The
  operation `trip.event.record` schema 1 (entity `trip_event`, id = the
  device event UUID, no expected revision, at most one dependency, no soft
  reference or attachment) carries the exact payload `trip_id`, `type` and
  `text` (a string or null) of canonical strings; the envelope declaration is
  the event's original instant, entry zone and device offset (BR-TIME-001/002).
  An accepted event stores one insert-only, forced-RLS logbook fact: its
  stable identity (BR-OFFL-001), trip, type, text, operation and digest,
  author membership and employee record, capture enrollment, the declaration
  (microsecond floor plus 0..999 remainder, entry zone, device offset) and
  the server receipt; the acknowledgement is `ACCEPTED` at revision 1. An
  event identity already used by another operation is `MALFORMED`.
- **B-BR-EMP-322 — Authority and target at receipt.** The operation needs
  current `trip.trip.start` on the enrolled device. Only the member who
  started the trip appends to it (another member is a whole-response
  `FORBIDDEN` with nothing stored); history follows EDR-118 continuity on the
  trip's asset at the event's instant, and a right changed after the
  declaration stores nothing and stays `RETRY`/`UNAVAILABLE`, as for a
  finish. A trip that is not stored in this organization — not yet delivered,
  held, or another organization's, which forced RLS makes indistinguishable —
  is `RETRY` `DEPENDENCY_PENDING` with nothing stored or revealed. A receipt
  replay needs current `trip.trip.read` over the trip (author, asset, stored
  unit and cost centre), except the author's own held receipt.
- **B-BR-EMP-323 — The event's time against its trip.** An event declared
  before its trip's start instant cannot be a fact of that trip and is
  `REJECTED` `MALFORMED`. While the trip is IN_PROGRESS any instant at or
  after its start is accepted; on a COMPLETED trip an event at or before the
  finish instant is accepted as a late delivery of a fact recorded during
  the trip, and the trip stays COMPLETED. An event declared after the finish
  of a completed trip is kept as a held event `ACCEPTED_FOR_REVIEW`
  `TRIP_ALREADY_CLOSED` with its whole fact and a version-1 review item in
  the same transaction as its acknowledgement; it is not a logbook fact and
  its resolution is retained (EMP-TRIP-007/015). An unknown type is
  `REJECTED` `MALFORMED`; a reserved type is `RETRY` `UNAVAILABLE` (a
  recognised future form: nothing is stored, nothing changes, the device
  keeps it queued).
- **B-BR-EMP-324 — Exact replay, concurrency and the timeline.** The same
  operation id and digest returns the stored acknowledgement byte for byte;
  the same id with another digest is `REJECTED` `OPERATION_ID_REUSED`;
  concurrent identical deliveries leave one fact. An event and the finish of
  its trip are serialized under the organization root and asset locks and the
  trip row: an event at or before the finish instant is accepted in either
  order and the finish completes the trip in either order, never a lost or
  duplicated fact; the receipt order decides only an event declared after a
  finish (accepted while the trip is still in progress, held once it is
  completed), which device dependencies (BR-OFFL-007) keep in order. A trip's timeline is its
  start, its logbook facts and its finish ordered by original instant (equal
  instants: start, then events by identity, then finish); each item keeps its
  fact type, actor, declared instant and entry zone, and server receipt, so
  type and receipt stay distinguishable. Held events are not timeline facts.
  No read route is added (panel reads belong to EMP-TRIP-006).
- **BUC-EMP-161 — Log a trip's facts offline.** During the VAL-01 trip the
  driver records an arrival, a note and a departure; each is delivered as its
  own `trip.event.record`, accepted once with one acknowledgement, and the
  timeline shows start, the three events and finish in order of occurrence
  with their receipts.
- **BUC-EMP-162 — Keep what cannot be appended.** An event declared after
  the trip's finish is held `TRIP_ALREADY_CLOSED`; a reserved type (`PAUSE`)
  stays queued with `RETRY` `UNAVAILABLE` and changes nothing; an unknown
  type is refused `MALFORMED`.
- **BUC-EMP-163 — Prove the logbook on real PostGIS and the shared oracle.**
  Replay, digest reuse, concurrent duplicates, an event racing the finish
  with barriers in both orders, another member, another organization's
  trip, forced RLS, insert-only facts, deferred publication guards and a
  populated 000224 → 000225 upgrade with injected failure and recovery; Go and
  Kotlin consume the same `logbook-v1.psv` oracle.

## Trip submission and administrative review (2026-11-15)

Evidence.

- **B-BR-EMP-342 — Submission is the author's signed `trip.submit`.** The
  operation `trip.submit` schema 1 (entity `trip_review`, id = the trip
  UUID, `expected_revision` = the review revision being submitted, at most
  one dependency — the trip's finish —, no soft reference or attachment)
  carries the exact empty payload `{}`; its envelope declaration is the
  instant the author marked the trip ready to send. The device keeps it
  queued as "ready to send" until the server answers; the case becomes
  `SUBMITTED` only when the server accepts it (BR-REV-002). Acceptance
  appends one APPR-001 `SUBMIT` entry (DRAFT → SUBMITTED, revision kept)
  with the operation, the author membership and its employee record in the
  same transaction as the `ACCEPTED` acknowledgement, the audit event and
  the effect, whose entity revision is the submitted review revision.
- **B-BR-EMP-343 — Only a completed trip, only by its author.** Only the
  member who started the trip submits it, with current `trip.trip.start`
  over the stored trip and the EDR-118 history continuity of the finish
  (another member is a whole-response `FORBIDDEN`). Only an operationally
  `COMPLETED` trip whose review case is `DRAFT` at the named revision is
  submitted. A trip that is not stored in this organization (missing, held
  or another organization's) or still `IN_PROGRESS` is `RETRY`
  `DEPENDENCY_PENDING` with nothing stored; an absent, non-positive or
  out-of-range expected revision, any payload key, a soft reference, an
  attachment or more than one dependency is `REJECTED` `MALFORMED`; a case
  that is no longer `DRAFT` at the named revision (submitted by another
  operation, returned, decided) and a submission whose right changed after
  its declaration store nothing and stay `RETRY` `UNAVAILABLE` until
  EMP-TRIP-015 owns resubmission, late duplicates and their held review.
  Same id and digest replays the stored acknowledgement byte for byte; the
  same id with another digest is `OPERATION_ID_REUSED`. Operational
  completion alone never submits or approves a trip.
- **B-BR-EMP-344 — One decision per revision by an independent scoped
  reviewer.** `POST /v1/orgs/{org_id}/trips/{trip_id}/review/decide`
  (`trip.trip.review`, configuration write, fresh proof, `revision` =
  the review revision judged) decides `APPROVE` (optional reason), `RETURN`
  or `REJECT` (reason required) with the B-BR-EMP-163 reason profile (1..280
  scalars, no control character, blank is missing). Under the organization
  root and the trip row lock the stored history is replayed through the
  APPR-001 lifecycle; a `SUBMITTED` case is opened (`OPEN`, SUBMITTED →
  IN_REVIEW) and decided in the same transaction, an `IN_REVIEW` case is
  decided directly. The entries carry the reviewer's membership and employee
  record and the command operation; one audit event
  `trip.trip.approved|returned|rejected` (target `trip`, command digest, no
  reason text) commits with them. The reviewer's permission is checked at
  receipt over the trip's stored structure: its asset, the unit and cost
  centre stored at the start, and the teams of its author and driver at the
  start receipt (EMP-AUTH-006 `UNIT`, `COST_CENTRE`, `TEAM`, `ASSET`).
- **B-BR-EMP-345 — Refusals change nothing.** A reviewer who shares an
  account, a membership or a canonical employee record (merge chain) with
  the trip's author or driver is `SELF_APPROVAL`, whatever roles that person
  holds (BR-REV-003); an expected revision other than the current one, or a
  case that is not `SUBMITTED` or `IN_REVIEW` (draft, already decided), is
  `CONFLICT` (BR-REV-007); a missing, blank or invalid reason is
  `VALIDATION_FAILED`; an unknown or another organization's trip is
  `NOT_FOUND`; a member without `trip.trip.review` (DRIVER, FINANCE,
  AUDITOR) or whose grant does not cover the trip's structure is
  `FORBIDDEN`. Two decisions racing on one revision leave exactly one
  decision; the other is `CONFLICT`. The same operation and digest returns
  the stored result with `replayed=true` and writes no second entry or
  audit event; the same operation with another digest is
  `OPERATION_ID_REUSED`. The history is append-only and forced-RLS; the
  database re-proves the transition, the separation of reviewer and author
  and, at commit, the receipt or audit event of every entry.
- **B-BR-EMP-346 — Minimal reviewer reads.** `POST
  /v1/orgs/{org_id}/trips/{trip_id}/review/read` (`trip.trip.review`, read,
  same structural coverage) returns only the decision context: trip, asset,
  driver employee, operational state, start and finish instants, start
  reporting day, review state and revision, and the entry history (kind,
  revision, resulting state, reason, actor membership, recorded instant). No
  reading value, run clock, coordinate or document content. `POST
  /v1/orgs/{org_id}/trips/review/list` (`trip.trip.review`, read, an
  organization-level resource that only an `ORG` grant covers) pages the
  trips whose latest entry is `SUBMITTED` or `IN_REVIEW` in submission order
  with the same minimal fields and the submission instant. Lists filtered by
  structural scope, the timeline and the panel screens remain EMP-TRIP-006.
- **B-BR-EMP-347 — What return and rejection mean now.** `RETURNED` records
  what to fix for the author and is not terminal; the correction loop — a
  new revision through events and amendments, then `RESUBMIT` — belongs to
  EMP-TRIP-015, so until then a `trip.submit` on a returned case stays
  `RETRY` `UNAVAILABLE`. `APPROVED` and `REJECTED` are terminal (BR-REV-006).
  No decision changes the trip's operational state, readings, meter usage
  or any time or wage figure; nothing approves automatically, and the author
  has no cancel command yet.
- **BUC-EMP-172 — Submit a finished trip (VAL-01 steps 4–5).** The driver
  finishes the trip offline and marks it ready to send; the queued
  `trip.submit` waits for its finish, is accepted on receipt and the trip
  becomes `SUBMITTED`; a resend returns the same acknowledgement.
- **BUC-EMP-173 — Review a submitted trip (VAL-01 step 6).** A scoped
  reviewer reads the decision context and approves, returns with a reason or
  rejects the current revision; the decision, its reviewer, its reason and
  one audit event are kept, and the previous entries remain.
- **BUC-EMP-174 — Prove the review on real PostGIS.** Self-approval, stale
  revision, missing reason, a repeated and a second decision, another
  organization's trip, an uncovering structural grant, an AUDITOR, a race in
  both orders, forced RLS and the history guards, and a populated upgrade
  with injected failure and recovery.

## Damage and incident reports with photos (2026-11-15)

Evidence.

- **B-BR-EMP-369 — A report is the reporter's signed
  `check.incident.report`.** The operation `check.incident.report` schema 1
  on the OFFL-002 intake (entity `incident_report`, id = the device report
  UUID, no expected revision, at most one dependency, no soft reference)
  carries exactly `report_type` (`DAMAGE` or `INCIDENT`), `incident_kind`
  (`BREAKDOWN`, `ACCIDENT`, `THEFT`, `FINE_NOTICE` or `OTHER` for an
  incident, `null` for a damage), `asset_id`, `trip_id` (UUID or `null`),
  `description` (1..1000 Unicode scalars, no control character, not only
  white space, under the BR-CHECK-015 hint), `location` (1..140 scalars of
  the same profile, or `null`: where on the asset, or the place) and
  `blocks_use` (the answer "no" to "O veículo pode continuar em uso?"). The
  envelope declaration is when it happened. No field exists for injuries,
  health, third-party identity, amounts, costs, notice numbers or document
  content. An envelope attachment declaration stays `RETRY` `UNAVAILABLE`
  until EMP-OFFL-005/EMP-DOC-004 own the device attachment path.
  Acceptance stores one report in `REPORTED` at revision 1 with the asset,
  the unit and cost centre the asset had at the declared instant
  (BR-AUTH-005), the trip when named, the reporter's account, membership,
  employee record and enrollment, the declaration and the server receipt,
  in the same transaction as the `ACCEPTED` acknowledgement (entity revision
  1), the intake audit event and the effect.
- **B-BR-EMP-370 — Who reports, and what is refused.** The report needs the
  current enrolled device and `check.incident.report` from retained role
  history at the declared instant (EDR-118 continuity): an `ORG` grant
  covers every asset of the organization; a `SELF` grant covers only an
  asset assigned to the member at that instant; a structural grant is not
  yet an intake source. An unknown or another organization's asset is a
  whole-response `NOT_FOUND`; a member without the permission is
  `FORBIDDEN`; a `DRAFT` asset, an unknown token, a wrong shape, an incident
  kind on a damage or none on an incident, an invalid description or
  location, more than one dependency or a report identity already held by
  another operation is `REJECTED` `MALFORMED`; a right that changed after
  the declaration stores nothing and stays `RETRY` `UNAVAILABLE` (no held
  owner yet). The same id and digest replays the stored acknowledgement
  byte for byte; the same id with another digest is `OPERATION_ID_REUSED`.
- **B-BR-EMP-371 — Inside a trip the report is a timeline fact.** A named
  trip must be stored in this organization (else `RETRY`
  `DEPENDENCY_PENDING`, nothing stored or revealed), be on the report's
  asset (else `MALFORMED`), have been started by the reporter (else
  `FORBIDDEN`) and contain the declared instant — from its start, up to the
  finish of a completed trip — (else `MALFORMED`); the check runs under the
  organization root and asset locks that the trip finish also takes. An
  accepted report with a trip is an `INCIDENT` or `DAMAGE` fact of that
  trip's timeline with its identity, reporter, declaration and receipt and
  without its description, in the shared order (B-BR-EMP-324); it is read
  from the report, never copied into the logbook. `trip.event.record` with
  `INCIDENT` or `DAMAGE` is therefore a final `MALFORMED`: those facts are
  their own operation, as the start and the finish are.
- **B-BR-EMP-372 — Photos are linked documents with their own state.** A
  report stores no byte, file name, digest or locator. Photos are EMP-DOC-002
  synthetic documents linked through EMP-DOC-003 with the target kind
  `INCIDENT`, resolved by the maintenance module: a link needs
  `doc.document.attach` on the member's own document and
  `check.incident.report` on the member's own report (the reporter alone);
  another member's document or report is `FORBIDDEN`, another organization's
  or an unknown one `NOT_FOUND`. A report is accepted, read and handled
  whatever the state of its photos: every read shows each linked document
  only with its own storage state (`UPLOADING`, `RECEIVED`, `STORED`,
  `QUARANTINED`, `FAILED`, …) and its link instant, never as ready unless it
  is `STORED`, and never with content; opening a photo stays the
  `doc.document.view_content` path audited by EMP-PRIV-005 (BR-CHECK-016,
  BR-PRIV-004). Real photos stay legally gated (OQ-04).
- **B-BR-EMP-373 — Handling is a recorded, scoped state change.** `POST
  /v1/orgs/{org_id}/incidents/reports/{report_id}/decide`
  (`check.incident.manage`, configuration write, fresh proof, `revision`)
  applies `ACKNOWLEDGE` (`REPORTED` → `ACKNOWLEDGED`, no note), `TREAT`
  (`REPORTED` or `ACKNOWLEDGED` → `IN_TREATMENT`, the stated action),
  `RESOLVE` (any open state → `RESOLVED`, the resolution note) or `DISMISS`
  (any open state → `DISMISSED`, the reason and optionally `duplicate_of`,
  another report of the same asset). Notes have 1..500 scalars of the
  description profile. Each step adds one append-only history row with the
  handler's account, membership and employee record, the operation and the
  command digest, and one audit event
  `check.report.acknowledged|treated|resolved|dismissed` (target
  `incident_report`, no text) in the same transaction as the report's next
  revision. A stale revision or a step not allowed from the current state is
  `CONFLICT`; a missing or invalid note, a note on an acknowledgement, a
  duplicate on another action, the report itself or a report of another
  asset is `VALIDATION_FAILED`; resolving or dismissing a report with
  `blocks_use` by its reporter (shared account, membership or employee
  record) is `SELF_APPROVAL` (BR-CHECK-011); an unknown or another
  organization's report (or duplicate) is `NOT_FOUND`; a member without the
  key, or whose grant does not cover the report's stored asset, unit or cost
  centre, is `FORBIDDEN`. Two handlers racing on one revision leave one
  step; the other is `CONFLICT`. The same operation and digest returns the
  stored step with `replayed=true`; another digest is
  `OPERATION_ID_REUSED`. Nothing is ever deleted.
- **B-BR-EMP-374 — A report books no cost; minimal reads.** No report,
  link or handling step creates or changes an expense, payable, budget,
  allocation, fueling, trip, reading, availability or document state
  (BR-CHECK-018); the asset block and the release of a blocking checklist
  run (BR-CHECK-010/011) remain EMP-CHECK-002/004. `POST
  /v1/orgs/{org_id}/incidents/reports/list` (`check.incident.manage`, read,
  an organization-level resource that only an `ORG` grant covers) pages the
  reports by identifier with their summary; `…/{report_id}/read` (same key,
  structural coverage over the stored asset, unit and cost centre) returns
  the report, its handling history and its linked documents' states; `POST
  /v1/orgs/{org_id}/incidents/reports/mine` (`check.incident.report`, read,
  the member's own record) pages the member's own reports. AUDITOR holds
  neither key and is `FORBIDDEN` on all of them.
- **BUC-EMP-186 — Report a damage or an incident offline.** A driver
  answers "Relatar problema" without signal; the queued
  `check.incident.report` is accepted on receipt (also inside the trip in
  progress, where it appears on the trip timeline), a resend returns the
  same acknowledgement, and nothing waits for the photos.
- **BUC-EMP-187 — Attach photos to one's own report.** The reporter links
  own synthetic documents to the report; the report and every read show each
  photo's own storage state, a failed upload stays visibly failed, and
  nobody obtains content without `doc.document.view_content`.
- **BUC-EMP-188 — Handle a report.** A scoped manager reads the queue and
  one report, acknowledges it, records an action, resolves it with a note or
  dismisses it with a reason (or as a duplicate); every step is in the
  history and the audit trail, and no cost appears.

## App: my assets and asset selection (2026-11-15)

Evidence.

- **B-BR-EMP-348 — The asset cache lives in the protected partition.** The
  app keeps the reference feed of one organization as one versioned cache
  inside the accepted protected generation of its exact account, API origin
  and organization (EMP-PRIV-003), bound to the membership and enrollment it
  was pulled with: the minimised `asset` entities of B-BR-EMP-336, the opaque
  cursor, the feed revision, the server `confirmed_at` and the device clock of
  the commit. A cache bound to another membership or enrollment, or written in
  an unknown cache version, reads as absent and is replaced by the next full
  pull; it is never shown. No plaintext table, journal or file holds it, it is
  never read under another account, origin or organization, and nothing here
  reads or writes the personal `vehicle` table.
- **B-BR-EMP-349 — A response is applied whole or not at all.** The app
  parses the frozen wire strictly (exact keys, closed enums including the
  entity type `asset`, OpenAPI bounds, canonical UTC instants) and binds it
  to its own request: request, membership and enrollment identifiers equal
  the request; a request without a cursor accepts only `RESET` with
  `NEW_CURSOR` or `FORBIDDEN`; a request with a cursor accepts `DELTA` only
  when `from_revision` equals the revision held with that cursor, or `RESET`
  with `CURSOR_EXPIRED`, `CURSOR_AHEAD` or `FORBIDDEN`. Changes are strictly
  increasing revisions not above `revision` (and above `from_revision` in a
  delta), one per entity; an upsert carries its asset with the same
  identifier and a tombstone carries none; a reset holds only upserts and
  `FORBIDDEN` holds none. `RESET` replaces the whole cached set (empty and
  marked forbidden for `FORBIDDEN`); `DELTA` replaces or removes only the
  named entities, and a tombstone for an entity the app never held changes
  nothing. Any violation leaves the cache untouched. The new set, cursor,
  revision and confirmation commit in one protected write and only when the
  cache is still the one the pull started from, so a process death or a
  concurrent refresh never leaves a mixed state. A `NOT_FOUND` answer to a
  request with a cursor is retried once without a cursor; nothing else is
  retried within one refresh.
- **B-BR-EMP-350 — Confirmation is shown, never invented.** The list shows
  the server `confirmed_at` of the last applied response. It is "não
  confirmado" when 24 hours have passed since that commit, measured by the
  device's elapsed time within one boot or by its wall clock after a reboot,
  or when the wall clock after a reboot is earlier than the commit. An asset
  whose cached assignment end has passed on the device clock is marked not
  confirmed on its own. A refresh that fails says that the list could not be
  confirmed now. The device clock never removes, hides or disables an asset:
  only an applied response does (BR-OFFL-016, B-BR-EMP-337).
- **B-BR-EMP-351 — "Meus ativos" shows exactly the cached set.** The list
  holds exactly the entities of the last applied response — the assets with
  an active assignment covered by the member's `fleet.asset.read` at that
  server instant — one per row with name, fleet number, plate, kind, the
  assignment kind and end, and the state; an asset whose state is not
  `ACTIVE` is shown as not available with its state (§6.2 row 1); no other
  availability is claimed because the feed carries none. The read-only
  detail adds the usage basis, the accepted products and each in-service
  meter with its kind and installation instant; it shows no reading or tank
  capacity because the feed carries none. With no cached entity the list says
  that no asset is assigned and that a company manager who assigns assets
  can do it; after `FORBIDDEN` it shows the no-permission pattern and no
  asset. Reading needs no network; refreshing does.
- **B-BR-EMP-352 — The picker returns a hint, the server decides.** The
  picker lists the same cached assets, optionally narrowed by the calling
  operation to what it accepts, one per row with large targets, and returns
  to the calling screen the organization, the asset identifier and revision,
  its kind, usage basis and in-service meters and the confirmation instant,
  with no network. Choosing an asset that is not `ACTIVE`, not confirmed or
  whose assignment end has passed first shows a warning that the company
  will review the choice; after it the asset is returned with the warning
  recorded (BR-FLEET-027). The returned value grants nothing: the operation
  carries the asset identifier and intake decides as if no cache existed.
- **BUC-EMP-175 — See my assets (UC-FLEET-08, SCR-FLEET-08/10).** Actor:
  member with an enrollment in the selected organization. Main flow: open
  "Meus ativos" → the protected cache is shown at once with its confirmation
  instant → when connected, a signed pull with the held cursor applies the
  changes and the list and its instant update → open an asset for its
  read-only detail. Alternatives: no cache yet and offline → unavailable
  state saying a connection is needed once; no assigned asset → empty state
  naming who can assign; `FORBIDDEN` → no-permission state; pull failure →
  the cached list stays with "não foi possível confirmar agora"; 24 hours
  without a pull → "não confirmado"; organization switch → only the newly
  selected organization's cache; another account → nothing.
- **BUC-EMP-176 — Choose an asset for an operation (SCR-FLEET-09).** Actor:
  member starting an operation. Main flow: the calling screen opens the
  picker → the member chooses a row → the picker returns the choice to the
  calling screen without network. Alternatives: a not available or not
  confirmed asset → review warning, then returned; cancel → nothing returned;
  no asset → the empty state of BUC-EMP-175.
- **BUC-EMP-177 — Prove the client against the feed.** One shared wire
  oracle consumed by the Go server parser/encoder and the Kotlin client, host
  tests for application, cache, process death and partition isolation, and
  native acceptance on the owned API 36 emulator against an ordinary PostGIS
  signed feed: online pull, process death, server unassignment while
  offline, reconnection tombstones and two organizations kept apart.

## Missions: create, assign, accept and decline (2026-11-15)

Evidence.

- **B-BR-EMP-353 — Closed mission lifecycle.** A mission has exactly one
  operational state from the 08-state-model catalog: `DRAFT`, `PLANNED`,
  `ASSIGNED`, `ACCEPTED`, `DECLINED`, `IN_PROGRESS`, `COMPLETED`,
  `CANCELLED`; `COMPLETED` and `CANCELLED` are terminal. Its commands are
  exactly `PLAN`, `ASSIGN`, `ACCEPT`, `DECLINE`, `START`, `COMPLETE` and
  `CANCEL`. A created mission is `DRAFT` at revision 1. The table answers,
  first failure first: a terminal state `INVALID_TRANSITION`; `PLAN`
  (EMP-TRIP-009), `START` and `COMPLETE` (the trip link of EMP-TRIP-021/023)
  and every change of an `IN_PROGRESS` mission `UNSUPPORTED_COMMAND`;
  `ACCEPT` and `DECLINE` only from `ASSIGNED` (else `INVALID_TRANSITION`);
  `ASSIGN` from `DRAFT`, `PLANNED`, `ASSIGNED`, `ACCEPTED` and `DECLINED` to
  `ASSIGNED`; `CANCEL` from the same five states to `CANCELLED`. Every command
  on a stored mission names the revision it was decided on: a stale revision
  is `CONFLICT` and changes nothing; an accepted transition increments the
  revision by exactly one and appends exactly one transition record. Planning
  or assignment never starts a trip, and no mission state is reached from a
  trip in this task.
- **B-BR-EMP-354 — Mission content.** A mission belongs to one organization
  and names an operating unit (`ACTIVE` at creation), a title (nonblank, free
  of control characters, at most 280 Unicode scalars), a planned window
  `starts_at, ends_at)` with whole-microsecond bounds and `ends_at` after
  `starts_at`, and optionally an asset, which must be an `ACTIVE` main asset of
  the organization (never an `IMPLEMENT`, EDR-219). A unit, asset
  or employee of another organization is not found. Unit, asset, title and
  window never change in this task (planning edits belong to EMP-TRIP-009,
  vehicle change to EMP-TRIP-013).
- **B-BR-EMP-355 — Assignment.** `ASSIGN` names an employee record of the
  organization that is `ACTIVE`, not merged, and whose live membership is
  `ACTIVE` and currently holds `trip.trip.start` covering the mission as the
  member's own record (the same coverage the response needs); otherwise it is
  a validation failure. The mission stores the employee and that membership as
  its current assignee. From `ASSIGNED` or `ACCEPTED` the assignee must change
  (a reassignment; the previous assignment and response stay in the
  transition history); from `DRAFT`, `PLANNED` or `DECLINED` any eligible
  member may be named, including the one who declined. When the mission names
  an asset, the EMP-FLEET-007 start decision for the new assignee at the
  planned start is returned once as a warning (`ASSET_BUSY` or
  `ASSET_UNAVAILABLE` with the derived availability) and kept with the command
  receipt; it never refuses the assignment, reserves, blocks or starts
  anything.
- **B-BR-EMP-356 — The assignee's response, decided at receipt.** Only the
  member who is the mission's current assignee responds, with `ACCEPT`
  (`ASSIGNED` → `ACCEPTED`, no reason) or `DECLINE` (`ASSIGNED` → `DECLINED`,
  reason required, same text profile). The response travels offline as the
  signed operation `trip.mission.respond` schema 1 (entity `mission`, id the
  mission, expected revision the revision the member answered; payload exactly
  `response` and `reason`) and is decided on the server only from current
  facts at receipt: the current device, membership and organization, current
  `trip.trip.start` covering the mission as the member's own (stored unit and
  asset as its structure), the current assignee and the current revision. The
  declared instant is evidence and never grants a response. Another member, or
  a member whose assignment or coverage ended, is a whole-response `FORBIDDEN`;
  a mission not stored in this organization is `NOT_FOUND`; a stale revision
  or a mission no longer awaiting the response (already answered, reassigned,
  cancelled) is `CONFLICT`; all three store nothing and change nothing. A
  malformed payload is `REJECTED` `MALFORMED`. An accepted response is
  `ACCEPTED` with the new mission revision. A response never starts a trip,
  reserves or blocks the asset, changes its availability, creates a trip or a
  review entry, or changes any review state (BR-REV-001).
- **B-BR-EMP-357 — Authorization and scope.** Mission creation and the
  organization mission list are organization-level actions under
  `trip.mission.manage` (only `ORG` grants cover them, EDR-161); reading,
  assigning and cancelling one mission need `trip.mission.manage` covering
  the mission's stored unit and asset (`UNIT` and `ASSET` grants by equality).
  Commands require fresh proof. The assignee's own mission list targets the
  membership itself under `trip.trip.start`. A mission of another
  organization is `NOT_FOUND` with no data. `AUDITOR` holds neither key and is
  `FORBIDDEN` on every mission route; reads of missions by auditors belong to
  the trip panel reads (EMP-TRIP-006/022).
- **B-BR-EMP-358 — History, receipts and audit.** The mission head, its
  insert-only transitions and insert-only command receipts are forced-RLS and
  never deleted; the head changes only together with the transition of its
  new revision. A company command records exactly one audit event
  (`trip.mission.created`, `trip.mission.assigned`, `trip.mission.cancelled`)
  and one receipt; the same operation identifier with the same intent replays
  the first result (with its warning), with another intent it is
  `OPERATION_ID_REUSED`. A response is audited by its intake receipt; the same
  operation id and digest replays the stored acknowledgement byte for byte and
  another digest is `REJECTED` `OPERATION_ID_REUSED`.
- **BUC-EMP-178 — Plan and assign a mission.** A fleet manager creates a
  mission for a unit, a title and a window, optionally naming a vehicle,
  assigns a driver (receiving an availability warning when the driver could
  not start the vehicle at the planned start), reassigns it after a decline,
  or cancels it with a reason; a stale revision or another manager's earlier
  change is `CONFLICT`; a unit-scoped manager acts only on missions of the
  unit.
- **BUC-EMP-179 — Accept or decline offline.** The assigned driver answers
  the mission on the device without signal; when the response reaches the
  server it is applied only if the driver is still the assignee with current
  coverage and the mission is still at the answered revision; the mission
  shows `ACCEPTED` or `DECLINED` and nothing starts.
- **BUC-EMP-180 — Read missions.** A scoped planner lists the organization's
  missions and reads one mission with its current assignee and response; an
  assigned member lists the missions currently assigned to the membership.

## Period lock and authorised reopening for trips (2026-11-15)

[Evidence.

- **B-BR-EMP-359 — Trip decisions follow the period of the start reporting
  day.** A review decision on a trip (`POST
  /v1/orgs/{org_id}/trips/{trip_id}/review/decide`: the opening composed
  with `APPROVE`, `RETURN` or `REJECT`) is admitted only when the APPR-006
  period (the organization calendar month) holding the trip's stored start
  reporting day is `OPEN`, or `REOPENED` with its stored, reasoned
  `fin.period.reopen` transition. The day is the one attributed at the start
  receipt in the stored unit or organization zone (BR-TIME-004, EDR-182) and
  never the device entry zone or offset, the UTC day of the start, the finish
  day or the day the decision is received. The period is read at receipt
  inside the decision transaction after the organization root lock and after
  every lifecycle check (`SELF_APPROVAL`, `CONFLICT`, `VALIDATION_FAILED`
  win first). A `LOCKED` period refuses the decision `PERIOD_LOCKED` (409)
  and writes no entry and no audit event. A replay of a decision recorded
  before the lock returns its stored result; reads are not gated.
- **B-BR-EMP-360 — Field intake is not gated by a locked period.**
  `trip.start`, `trip.finish`, `trip.event.record` and `trip.submit` are
  accepted, held or refused exactly as before whatever the period of their
  reporting day (the B-BR-EMP-266 precedent): a late field fact whose trip
  lies in a locked month is an accepted fact, never a held `PERIOD_LOCKED`
  intake item. A trip submitted in a locked month stays `SUBMITTED` or
  `IN_REVIEW` in the review queue, which is its review; deciding it needs an
  authorised reopening of the month.
- **B-BR-EMP-361 — Reopening is the only path for trips; no adjustment.**
  A trip carries no amount posted to a period, so the adjustment branch of
  BR-AMD-004 does not apply to it now: a decision or correction of a trip in
  a `LOCKED` month is refused `PERIOD_LOCKED`; after an authorised
  reopening (`fin.period.reopen`, fresh proof, reason) decisions are
  admitted; a re-lock refuses later decisions again and keeps every
  decision already recorded. A locked month keeps its approved trips
  unchanged.
- **B-BR-EMP-362 — The database refuses a decision in a locked month and
  keeps history.** Under the organization root lock that every period
  command and review entry takes, the database refuses an opening or a
  decision entry whose trip's start reporting month is `LOCKED`, so a
  defective handler cannot decide in a locked period. Review entries, trips,
  finishes and period transitions stay insert-only (update, delete and
  truncate refused); the lock, the reopening with its actor and reason, the
  re-lock and the decisions remain in their append-only histories in the
  order the root lock serialised them.
- **BUC-EMP-181 — Decide a trip of a closed month (JRN-09 for trips).**
  Finance locks October; the approval of a submitted October trip is
  refused `PERIOD_LOCKED`; finance reopens October with a reason; the
  reviewer approves; finance re-locks October and the approval stays.
- **BUC-EMP-182 — Prove the trip period lock on real PostGIS.** A trip
  whose stored reporting day is in another month than its UTC, device and
  receipt days; a reopening withdrawn by a re-lock before receipt; lock and
  decision racing in both orders; a direct decision insert and in-place
  rewrites refused by the database; a populated upgrade with injected
  failure and recovery.

## Late and duplicate events, and amendments after closure (2026-11-15)

Evidence.

- **B-BR-EMP-363 — Revisions and resubmission after a return.** A trip's
  review revisions are its record revisions: revision 1 is the submitted
  field record. While the case is `DRAFT`, or a field revision is
  `RETURNED` and the trip was never approved, the author's logbook events
  declared at or before the finish are ordinary edits of the revision being
  prepared (BR-AMD-006; events after the finish stay `TRIP_ALREADY_CLOSED`).
  The author's signed `trip.submit` whose `expected_revision` names the
  returned revision appends `RESUBMIT` (RETURNED → SUBMITTED, revision + 1)
  with its `ACCEPTED` acknowledgement at the new revision (BR-REV-005); the
  returned revision, its reason and its decision are kept, and a decision
  that names the returned revision is `CONFLICT`. An approved or rejected
  field record is never resubmitted.
- **B-BR-EMP-364 — Late and duplicate field facts after submission are kept
  for review, never applied.** A supported `trip.event.record` declared at
  or after the start and at or before the finish of a trip whose latest
  revision is `SUBMITTED`, `IN_REVIEW`, `APPROVED` or `REJECTED`, or that
  has an amendment, is `ACCEPTED_FOR_REVIEW` `STALE_REVISION`: an OFFL-008
  held fact that belongs to no revision and changes no figure. A
  `trip.submit` by the author whose `expected_revision` is no longer the
  current `DRAFT` or returned field revision (submitted by another
  operation, decided, superseded by a resubmission or an amendment) is
  `ACCEPTED_FOR_REVIEW` `STALE_REVISION` (a held submission; the case is
  unchanged). An identical resend returns the stored acknowledgement, the
  same identifier with another digest is `OPERATION_ID_REUSED`, and an event
  identity used by another operation stays `MALFORMED`, so a repeat has
  exactly one effect. The database refuses a direct logbook fact on a
  submitted, decided or amended revision. Held items are resolved by
  EMP-TRIP-007; changed rights on an event or submission still store nothing
  (`RETRY`/`UNAVAILABLE`) until that owner exists.
- **B-BR-EMP-365 — An approved trip is amended through a new reviewed
  revision.** `POST /v1/orgs/{org_id}/trips/{trip_id}/amendments/propose`
  (`meter.reading.correct` covering the trip's stored structure — asset,
  stored unit and cost centre, start teams —, configuration write, fresh
  proof, ACTIVE organization, `revision` = the latest review revision the
  actor saw) carries a reason (1..280 scalars, no control character) and a
  corrected start and/or end reading (meter kind and exact base units). The
  orgamend kernel decides it on server facts: `AMEND` from the approved
  revision in effect (base = that revision) or after a rejected amendment,
  `EDIT` of a returned pending amendment (base = that revision); the next
  revision is appended `SUBMITTED` as one `AMEND` history entry with the
  reason, the amendment record (intent, base, each corrected reading of the
  effective revision with its value there, the corrected value and the
  pre-allocated superseding reading) and one `trip.trip.amended` audit event
  (digest, no value or reason). A never-approved trip (`NOT_APPROVED`), a
  pending amendment (`AMENDMENT_PENDING`) and a stale `revision` are
  `CONFLICT`; a missing or invalid reason, a correction of a reading the
  trip does not have, of another meter kind, out of range or equal to the
  current value is `VALIDATION_FAILED`; a locked month `PERIOD_LOCKED`;
  another organization's trip `NOT_FOUND`; a role or grant that does not
  cover it `FORBIDDEN`. The same operation and digest replays the first
  result; another digest is `OPERATION_ID_REUSED`. Until the amendment is
  approved the previous approved revision stays in effect (BR-AMD-002).
- **B-BR-EMP-366 — Amendment review and its effect.** The amendment revision
  is decided on the existing decide route (`trip.trip.review`, B-BR-EMP-344)
  by a reviewer who is neither the trip's author nor its driver nor the
  amendment's author (BR-REV-003), under the EMP-TRIP-016 period gate. The
  orgamend kernel resolves the result: `APPROVE` makes the amendment the
  effective revision only in a writable period, `RETURN` keeps it pending
  for an `EDIT`, `REJECT` leaves the previous revision in effect. On
  approval, in the same transaction, each corrected reading is superseded
  through the METER-004 path (a new reading with the corrected value, the
  original declaration, origin and segment, method `TYPED`, the reviewer as
  author and its company session as capture, the decision's operation; the
  original `VOIDED` with cause `SUPERSEDED`, the link and the amendment
  reason, and kept; the segment re-evaluated). Nothing changes before the
  approval and nothing is edited in place (BR-AMD-001).
- **B-BR-EMP-367 — Derived usage names its revision.** The trip's meter
  usage is derived from the readings of one named revision with the
  EMP-TRIP-001 rule (measured only for two non-decreasing readings of one
  segment; otherwise `NOT_MEASURED` with its reason, never invented). The
  decision context returns the effective revision with its usage and every
  amendment with its corrections (original and corrected value), state,
  author, reason and usage; before approval the effective usage stays that
  of the previous revision (290 000 m in the trips §10 example) and after
  approval it is the amendment's (280 000 m). No stored figure is
  rewritten; the original readings stay readable with their supersession.
- **B-BR-EMP-368 — Readings of an approved trip need an amendment.** A
  METER-004 decision on a trip's start or finish reading, or on a reading
  that an approved trip amendment created, of a trip with an approved
  revision is refused `AMENDMENT_REQUIRED` (B-BR-EMP-290 extended to trips),
  so an approved trip's usage never changes in place; readings of a
  never-approved trip are decided directly. The database keeps amendments
  insert-only and forced-RLS, re-proves the amendment lineage, the current
  readings and values, the period lock, the reviewer's separation from the
  amendment's author and, at commit, that an approved amendment superseded
  exactly its readings in its decision operation.
- **BUC-EMP-183 — Correct and resubmit a returned trip (VAL-01 step 6).**
  The reviewer returns the trip with "Informe o motivo da parada"; the
  driver adds a note offline and resubmits; the trip becomes `SUBMITTED` at
  revision 2 and is approved; revision 1 and its return stay in the history.
- **BUC-EMP-184 — Amend an approved trip's final reading (VAL-26, trips
  §10).** A fleet manager corrects the final reading from 184 610 000 m to
  184 600 000 m with a reason; until an independent reviewer approves, the
  effective usage stays 290 000 m; after the approval revision 2 is in
  effect with 280 000 m, the original reading is `VOIDED` and superseded by
  the corrected one, and the audit trail holds the amendment and the
  decision.
- **BUC-EMP-185 — Prove late, duplicate and amended trips on real PostGIS.**
  A late event after submission and after approval (held, resent once), a
  duplicate and a stale submission, the resubmission loop, the amendment
  with self-approval, stale and locked refusals, two proposals racing in
  both orders, the METER-004 refusal, the database guards and a populated
  upgrade with injected failure and recovery.

## App: start a trip, log events and finish, offline first (2026-11-15)

Evidence.

- **B-BR-EMP-375 — A trip is four signed operations sealed to their creation
  partition.** The app records a driver-initiated trip only as the frozen
  schema 1 operations of EMP-TRIP-002/003/004 with their exact payloads:
  `trip.start` (entity `trip`, the device trip UUID; the asset chosen in the
  FLEET-013 picker, the member's own employee record as driver, the departure
  reading — meter, its open segment, a new reading UUID, kind and exact value
  in base units — or a stated absence reason, and the run clock; no
  dependency), `trip.event.record` (entity `trip_event`; `NOTE` with a text,
  `STOP`, `ARRIVAL` or `DEPARTURE` with an optional text; depends on the
  start), `trip.finish` (the final reading or a stated absence and the run
  clock; depends on the start) and `trip.submit` (entity `trip_review` with
  `expected_revision` 1 and the empty payload; depends on the finish). Each
  envelope carries the account, API origin, organization, membership and
  enrollment of the exact enrollment that created it (BR-OFFL-003); switching
  to personal mode or another organization never rewrites, moves or hides
  them, and they keep being delivered to their own organization in the
  background. Reserved logbook types, attachments and soft references are
  never produced.
- **B-BR-EMP-376 — "Salvo no aparelho" only after a durable write.** The
  operation and the trip's local index are committed together in one
  protected write of the creation partition (EMP-PRIV-003 generation) before
  the screen reports the record as saved; when the write fails the screen
  says that nothing was saved and shows no new record. The local index is one
  sealed record per partition (versioned `trip-book` format) listing each
  trip's asset label, departure instant, the identifiers of its start,
  events, finish and submission, and whether the start was finally refused;
  a process death leaves either both or neither. After a restart the
  creation partition shows the same trips and records; another account,
  origin or organization never reads them.
- **B-BR-EMP-377 — Local eligibility is checked on the device and decides
  nothing on the server.** Before queueing, the app refuses: a start for an
  asset that already has an unfinished trip of this partition on this device
  (the first one must be finished; a trip whose start the server finally
  refused no longer counts); an event, a finish or "ready to send" on a trip
  that is not in the required local state (events and the finish only while
  unfinished and not refused, the finish once, "ready to send" once and only
  after the finish); and an event or finish declared before the trip's start
  instant (the device clock went back; the EMP-TRIP-001/003 order rules).
  The server still decides every fact as if no device rule existed:
  `ASSET_BUSY` between devices, availability and permissions are its
  outcomes.
- **B-BR-EMP-378 — Declaration and run clock come from the device, never
  invented.** Each record declares the device wall instant, the device time
  zone when it is an accepted zone identifier (otherwise `UTC`) and the
  device offset at that instant. The start and the finish carry the run clock
  read from the same source as the protected lease clock: the boot identity
  and the elapsed real time in milliseconds since that boot. When the clock
  cannot be read the run clock is absent; nothing is estimated.
- **B-BR-EMP-379 — Acceptance follows each record's own receipt.** The
  timeline shows the start, each event in occurrence order and the finish,
  each with its own sync state from its own retained queue entry: "Salvo no
  aparelho" without a final receipt, "Aceito pela empresa" only after an
  `ACCEPTED` receipt (or an accepted review resolution), "Precisa de revisão"
  with the reason for an `ACCEPTED_FOR_REVIEW` receipt (`ASSET_BUSY`,
  `ASSET_UNAVAILABLE`, `TRIP_ALREADY_CLOSED`, `PERMISSION_CHANGED` and the
  other review codes) and "Não aceito" with the reason for a refusal. The
  trip's operational line is local ("Em andamento", "Encerrada no aparelho")
  and never claims a server state. The review line shows only what a receipt
  proves: "Pronto para enviar" while the submission waits, "Aguardando
  aprovação" after its `ACCEPTED` receipt; approval, return or rejection are
  never shown or inferred. When the start needs review or was refused, the
  trip says that its other records wait for, or depend on, that outcome. A
  start or finish whose reading the METER-003 validator keeps aside as
  `SUSPECT` is accepted like any other: the warning belongs to the reading
  (B-BR-EMP-280..285) and never holds the trip's own record back.
- **B-BR-EMP-380 — The member's own employee record identifier.** The
  context read of the selected organization (`/v1/org-access/contexts/read`)
  also returns `employee_id`, the employee record of the requesting
  membership, which never changes for that membership (EDR-054). The app
  keeps it only in memory with the selected context and writes it only as
  the driver of its own `trip.start`; it is never displayed and grants
  nothing. Without it the start is unavailable and the app asks to confirm
  the organization again.
- **BUC-EMP-189 — Start a trip offline (UC-TRIP-01, UC-OFFL-01, JRN-06,
  SCR-TRIP-01).** Actor: assigned driver in the selected organization. Main
  flow: "Iniciar viagem" → choose the asset in the picker (cached assets,
  warning for a not available or not confirmed one) → enter the departure
  reading of an in-service meter or state why it is missing → save → the
  trip appears in progress with its start "Salvo no aparelho".
  Alternatives: the asset already has an unfinished trip here → refused with
  the reason, nothing saved; the protected write fails → error, nothing
  saved; the employee identifier is unknown → confirm the organization
  again.
- **BUC-EMP-190 — Log events, finish and mark ready to send.** Actor: the
  same driver. Main flow: add `STOP`, `ARRIVAL`, `DEPARTURE` or a `NOTE` with
  text → each appears in the timeline "Salvo no aparelho" → "Encerrar
  viagem" with the final reading or its absence (a reading below the
  departure or an implausible rate is warned, never blocked) → "Marcar
  pronta para enviar" → when a connection exists the queue sends in
  dependency order and each record changes only with its own receipt.
  Alternatives: a held start (`ASSET_BUSY` from another member's device)
  shows "Precisa de revisão" with the reason while its children wait;
  process death at any step → the same timeline after restart.
- **BUC-EMP-191 — Prove the producer.** One shared operation oracle
  (`trip/trip-operations-v1.psv`) whose exact bytes the Kotlin builders
  produce and the Go intake parsers accept or refuse identically; host tests
  for the trip book, producers, service and flow, the protected store
  format, partition isolation and process death; and native acceptance on
  the owned API 36 emulator against an ordinary PostGIS signed intake:
  offline start, events, finish and submission, process death, reconnection
  with each record accepted exactly once, and a held start against another
  member's earlier start on the same asset.

## Manager-planned trips: destination, reason, customer or project, cargo or passengers (2026-11-15)

Evidence.

- **B-BR-EMP-381 — `PLAN` in the mission table.** `PLAN` moves a `DRAFT`,
  `PLANNED`, `ASSIGNED` or `DECLINED` mission to `PLANNED` at exactly the
  next revision and withdraws its current assignee and answer (both stay in
  the transition history); the planner assigns the planned mission again,
  any eligible member including the previous one, and an answer still in
  flight is a whole-response `FORBIDDEN` because its member is no longer the
  assignee (B-BR-EMP-356 order). From `ACCEPTED` it is
  `INVALID_TRANSITION`: an accepted plan is the work the member committed
  to, so the planner reassigns or cancels instead of rewriting it. From
  `IN_PROGRESS` it is `UNSUPPORTED_COMMAND` (every change of a running
  mission belongs to the trip link of EMP-TRIP-021/023) and from `COMPLETED`
  or `CANCELLED` `INVALID_TRANSITION`. Every other row of the B-BR-EMP-353
  table is unchanged; `START` and `COMPLETE` stay `UNSUPPORTED_COMMAND`. The
  command fails first on scope, then on the plan content, then on a
  non-positive expected revision, then `CONFLICT` for a stale revision (which
  changes nothing), then the table. `PLAN` never changes the mission's unit,
  asset, title or window.
- **B-BR-EMP-382 — Minimised plan content.** A plan has exactly: a
  destination that is either a free-text place or an `ACTIVE` operating unit
  of the organization (never both, never neither); a required reason; an
  optional customer or project reference, which is an `ACTIVE` cost centre of
  the organization (the organization's approved representation of a client
  contract, project or route); and a load that is `NONE`, `CARGO` with a
  required description, or `PASSENGERS` with a count from 1 to 999 and
  nothing else. Free texts (place, reason, cargo description) follow the
  METER-001 text profile (nonblank, no control character, at most 280
  Unicode scalars) and refuse any standalone CPF-shaped token (the portal
  privacy shape) as unapproved sensitive personal data. A plan holds no
  passenger name, document, contact or description, no cargo count, no
  coordinates and no other personal field; a request carrying any other
  field is refused by the strict body. An unknown unit or cost centre, or one
  of another organization, is `NOT_FOUND`; an archived one is
  `VALIDATION_FAILED`.
- **B-BR-EMP-383 — A planning revision is separate from execution.** Each
  accepted `PLAN` appends one insert-only planning revision bound to its
  `PLAN` transition and command receipt: the mission revision it produced,
  the content, the planner's account and membership and the company command
  that authorized it, and the server time. The mission's current plan is its
  latest planning revision and survives later assignment, response and
  cancellation; earlier revisions stay in the history. A plan never creates,
  starts, changes or completes a trip, reading, logbook event, review entry,
  reservation or block, never changes an asset or its availability, and no
  field fact changes a plan; planned and actual destinations stay distinct.
- **B-BR-EMP-384 — Authorization and scope.** `PLAN` is a mission command
  under fresh `trip.mission.manage` covering the mission's stored unit and
  asset (`UNIT` and `ASSET` grants by equality; a structural grant that does
  not cover the mission is `FORBIDDEN`), at the expected revision, in an
  `ACTIVE` organization. A mission of another organization is `NOT_FOUND`
  with no data. `AUDITOR` holds no mission key and is `FORBIDDEN`. The
  current plan is returned with the mission on the planner's read and list
  and on the assignee's own list.
- **B-BR-EMP-385 — Receipts, replay, concurrency and audit.** An accepted
  `PLAN` records exactly one `trip.mission.planned` audit event and one
  receipt; the same operation identifier with the same intent replays the
  first result, with another intent it is `OPERATION_ID_REUSED`. Two
  commands decided on one revision resolve to one: the loser is `CONFLICT`
  and nothing of it is stored.
- **B-BR-EMP-386 — The database keeps plans honest.** The database refuses a
  planning revision without its `PLAN` transition at the same revision and
  operation, a `PLAN` transition without its planning revision at commit, a
  `PLAN` from `ACCEPTED`, `IN_PROGRESS` or a terminal state, a `PLAN` that
  keeps an assignee or answer, a CPF-shaped token in a plan text, an
  archived or foreign unit or cost centre, a passenger description or cargo
  count, and any update, delete or truncate of planning revisions. The unit
  and cost-centre usage sources recognise the two plan keys as associations
  without counting them.
- **BUC-EMP-192 — Plan a mission.** A fleet manager plans a `DRAFT` mission:
  destination "Centro de distribuição Norte" (or the organization's
  Southern unit), reason "Entrega de peças de reposição", the client
  contract's cost centre and cargo "Peças de reposição"; the mission becomes
  `PLANNED` and is assigned afterwards; the driver's own list shows the plan.
- **BUC-EMP-193 — Re-plan before acceptance.** The assigned driver declines,
  or has not answered yet; the manager changes the destination and assigns
  again; an answer the driver sent for the withdrawn assignment is
  `FORBIDDEN` and stores nothing; an accepted mission's plan cannot be
  rewritten, and a unit-scoped manager plans only missions of the unit.
- **BUC-EMP-194 — Prove planned trips on real PostGIS.** Foreign units and
  cost centres, structural and organization isolation, CPF-shaped and
  unapproved fields, plans of accepted, in-progress and terminal missions,
  replay, reuse and racing commands in both orders, the database guards, and
  a populated upgrade from the 000229 head with injected failure and
  recovery.

## Vehicle change within a mission (2026-11-15)

Evidence.

- **B-BR-EMP-387 — Authorized replacement of the mission's vehicle.** A
  vehicle change names the mission's new vehicle and a reason (METER-001 text
  profile) at the expected revision. It is accepted only for a `PLANNED`,
  `ASSIGNED`, `ACCEPTED` or `DECLINED` mission; a `DRAFT` mission exists only
  at revision 1 (B-BR-EMP-353), so it keeps its creation vehicle until it is
  planned (`INVALID_TRANSITION`); an `IN_PROGRESS` mission is
  `UNSUPPORTED_COMMAND` (the trip link of EMP-TRIP-021/023) and a terminal one
  `INVALID_TRANSITION`; naming the current vehicle again is
  `INVALID_TRANSITION`. The change fails first on scope, then on the vehicle
  reference and the reason, then on a non-positive expected revision, then
  `CONFLICT` for a stale revision, then on the state. It changes only the
  mission's vehicle and revision (exactly one more): state, assignee, answer,
  plan, unit, title and window stay, so an answer still in flight for the
  earlier revision is `CONFLICT`. The new vehicle must be an `ACTIVE` main
  asset of the organization (never an `IMPLEMENT`, EDR-219; unknown or another
  organization's `NOT_FOUND`, otherwise `VALIDATION_FAILED`), must not be in
  use by another trip (`IN_USE`, the EMP-TRIP-002 one-active-trip rule:
  `CONFLICT`) and must not be `UNAVAILABLE`, `IN_MAINTENANCE` or `BLOCKED` at
  the planned start (EMP-FLEET-007 derived availability: `VALIDATION_FAILED`);
  a reservation is never refused or changed by it. When the mission has a
  current assignee (`ASSIGNED`, `ACCEPTED`), that member must still hold
  `trip.trip.start` covering the mission with the new vehicle, otherwise
  `VALIDATION_FAILED`. The planner needs fresh `trip.mission.manage` covering
  the mission as stored and as it would be with the new vehicle (a structural
  grant covering only one of them is `FORBIDDEN`); another organization's
  mission is `NOT_FOUND`; `AUDITOR` is `FORBIDDEN`.
- **B-BR-EMP-388 — Each vehicle keeps its own evidence.** A change is refused
  `CONFLICT` while the mission's assignee has a trip in progress on the
  current vehicle: that trip is finished on its own vehicle with its final
  reading first, and work on the new vehicle is a new trip that starts with
  the new vehicle's own reading. A change never moves, splits, merges or
  rewrites a trip, reading, meter, segment, logbook event, review entry,
  reservation or block; meter histories are never merged; the database
  refuses reassigning a reading or a trip to another asset.
- **B-BR-EMP-389 — History, receipts and database guards.** An accepted change
  appends one `VEHICLE` transition (same state on both sides) and one
  insert-only vehicle-change record (previous and new vehicle, reason, the
  planner and the command), records exactly one `trip.mission.vehicle_changed`
  audit event and one receipt; replay and reuse follow B-BR-EMP-358; two
  commands decided on one revision resolve to one. The database moves the
  mission's vehicle only together with that record, re-proves the new
  vehicle's lifecycle and that it runs no trip, refuses a change while the
  assignee runs a trip on the previous vehicle, keeps the records insert-only
  and forced-RLS, and its asset usage source recognises the two record keys as
  associations without counting them.
- **BUC-EMP-195 — Replace a vehicle in a mission.** The accepted mission's
  vehicle needs maintenance; the driver finishes the running trip on it with
  the final reading; the fleet manager names a free vehicle of the unit with
  the reason "Veículo em manutenção"; the mission stays `ACCEPTED`, the
  driver's next trip starts on the new vehicle with its own reading, and the
  first vehicle's trip and readings are unchanged.
- **BUC-EMP-196 — Prove vehicle changes on real PostGIS.** Foreign, draft,
  implement, blocked and busy vehicles; a running trip on the current
  vehicle; the assignee's and the planner's coverage; another organization;
  stale revisions and racing commands in both orders; a reading or trip moved
  to another asset; the database guards and a populated upgrade with injected
  failure and recovery.

## Tracking session domain: trip-bound lifecycle, permission and quality states (2026-11-15)

Evidence.

- **B-BR-EMP-421 — A session belongs to one organization trip of its
  driver.** A tracking session is opened only for an organization trip of the
  scope that is `IN_PROGRESS`, by the member who runs that trip, from the
  visible trip screen (origin `TRIP_SCREEN`), while the organization's
  tracking policy is `OPTIONAL` or `EXPECTED`. It opens `NOT_STARTED` with
  cause `NOT_STARTED` and binds the organization, the session identity, the
  trip and the owning membership for its whole life; nothing is collected.
  Opening fails first on scope (`SCOPE`), then on the session identity, the
  trip facts (trip, driver, operational state), the author, the origin,
  policy and instant tokens (absent or malformed `INVALID`, another
  organization `SCOPE`), then on the supplied existing session (another
  organization or another trip `SCOPE`), then `PERSONAL_TRIP` for a personal
  origin, `IMPLICIT_START` for any origin other than the trip screen,
  `FORBIDDEN` when the author is not the trip's driver, `TRACKING_OFF` for
  policy `OFF` (an inactive capability or an organization whose employee
  notice is not in force resolves to `OFF`, EDR-271), `TRIP_STATE` for a trip
  that is not `IN_PROGRESS`, and `SESSION_EXISTS` when the trip already has a
  session in any state, ended included.
- **B-BR-EMP-422 — Recording starts and resumes only by the driver's visible
  action, with the permission stated.** `START` (from `NOT_STARTED`) and
  `RESUME` (from `SUSPENDED` or `INTERRUPTED`) are the owner's own actions on
  the visible trip screen while the trip is `IN_PROGRESS` and still driven by
  the owner. Each carries the device's location permission (`PRECISE`,
  `APPROXIMATE` or `NONE`) and whether the tracking notification is visible;
  an absent permission is `INVALID`, never assumed. `PRECISE` or
  `APPROXIMATE` with a visible notification gives `RECORDING` with that
  precision and opens a recording period. `NONE` keeps the state and records
  the attempt with cause `PERMISSION_DENIED`; a hidden notification keeps the
  state and records `NOTIFICATION_BLOCKED`; the refusal is the session's
  visible cause, never hidden and never an error that drops the attempt.
  Notification, boot, worker, push, geofence and server origins are
  `IMPLICIT_START`; a personal origin is `PERSONAL_TRIP`; another member is
  `FORBIDDEN`; any other state is `INVALID_TRANSITION`; a trip that is not
  `IN_PROGRESS` is `TRIP_STATE`.
- **B-BR-EMP-423 — Pause suspends; interruptions and reduced precision are
  typed causes.** `SUSPEND` moves `RECORDING` to `SUSPENDED` with
  `TRIP_PAUSED` (the trip is `PAUSED`) or `LONG_STATIONARY` (the trip is
  `IN_PROGRESS`). `INTERRUPT` moves `RECORDING` to `INTERRUPTED` while the
  trip is `IN_PROGRESS` with exactly one of `PERMISSION_REVOKED`,
  `NOTIFICATION_BLOCKED`, `PROCESS_STOPPED`, `DEVICE_RESTARTED`,
  `STOPPED_BY_DRIVER` (only the owner) or `OWNER_CHANGED`. `REDUCE` keeps
  `RECORDING` with cause `PRECISION_REDUCED`: it closes the precise period and
  opens an approximate one at the same instant; a raise of precision takes
  effect only at the driver's next start or resume. A cause that does not
  belong to its command is `INVALID`. No command resumes recording by itself:
  only `RESUME` leaves `SUSPENDED` or `INTERRUPTED`.
- **B-BR-EMP-424 — End ends, for good.** `END` moves any open state to
  `ENDED` with `TRIP_COMPLETED` (the trip is `COMPLETED`), `TRIP_CANCELLED`
  (the trip is `CANCELLED`), `MEMBERSHIP_ENDED` or `DEVICE_REVOKED` (any
  started trip state). `ENDED` is terminal: every command on it is
  `INVALID_TRANSITION`, a session never restarts and its trip never receives
  a second session.
- **B-BR-EMP-425 — The trip never depends on the session.** No trip
  transition reads tracking. `FOLLOW` maps the trip's current state onto the
  session and never refuses with `INVALID_TRANSITION`: `PAUSED` suspends a
  `RECORDING` session with `TRIP_PAUSED`; `COMPLETED` and `CANCELLED` end any
  open session; `IN_PROGRESS` changes nothing, because recording returns only
  through `RESUME`; a trip state that a started trip cannot have is
  `TRIP_STATE`. A fix may be collected only while the bound trip is
  `IN_PROGRESS` and the session is `RECORDING`, and it carries the precision
  of the current period (`APPROXIMATE` fixes are the coarse ones); a session
  that has not followed an ended trip collects nothing.
- **B-BR-EMP-426 — Every transition is recorded and values stay private.**
  Opening, every transition and every refused attempt append one history
  entry with the resulting state, the cause, the actor kind (`DRIVER`,
  `DEVICE`, `TRIP`, `SERVER`, fixed by the command and cause), the permission
  observed at a start, resume or reduction, and the instant. An instant
  before the last entry is `OUT_OF_ORDER` (an equal one is accepted); an
  unset instant is `INVALID`. Recording periods are derived from the history
  as half-open intervals with their precision. Commands return a new value
  and never change their input; another trip of the organization is `SCOPE`.
  Sessions, inputs, history, periods and references print redacted in fmt,
  slog and JSON; the domain holds no coordinates.
- **B-BR-EMP-427 — Track quality has one classification.** Coverage is
  `floor((expected − gap) × 1000 / expected)` permille over whole seconds of
  the trip's `IN_PROGRESS` time. Quality is `INSUFFICIENT` when expected time
  is zero (coverage unknown), with fewer than two accepted fixes, or below 500;
  otherwise `APPROXIMATE` when any recording period was approximate or
  coverage is below 900; otherwise `PRECISE`. Negative values, a gap longer
  than the expected time or an expected time above 10¹² seconds are
  `INVALID`. The worked example (20 400 s expected, 1 020 s of gaps,
  precise) is 950 permille and `PRECISE`.
- **BUC-EMP-214 — Track a trip, pause, resume and finish.** The driver starts
  a trip with tracking on the trip screen: the session opens, starts
  `RECORDING` with precise permission, is `SUSPENDED` when the trip pauses,
  records again only after the driver resumes on the trip screen, and is
  `ENDED` when the trip completes; its history names every step and its two
  recording periods. With approximate permission only, the same trip records
  coarse fixes and its quality can never be better than `APPROXIMATE`.
- **BUC-EMP-215 — Missing permission and interruptions stay visible.** A
  driver who denies location still runs the trip; the session stays
  `NOT_STARTED` with `PERMISSION_DENIED`. A process death, a reboot or a
  revoked permission makes a recording session `INTERRUPTED` with its cause,
  nothing resumes it except the driver's `RESUME` on the trip screen, and a
  boot, worker, push or notification origin is refused.
- **BUC-EMP-216 — Prove the session domain in Go and Kotlin.** One shared
  oracle checks every vocabulary, opening, command, trip follow, collection
  and quality rule, with the negatives: a personal trip, an implicit start, a
  duplicate session, another member, another trip or organization, a hidden
  or absent permission, a session continuing after its trip ended, a
  terminal session and an out-of-order instant.

## Attachment upload: offline queue, retry and resume, linkage to operations (2026-11-15)

Evidence.

- **B-BR-EMP-415 — An attachment is captured offline, sealed and bound to its
  operation.** The app captures a C1 attachment only as a synthetic JPEG
  (`image/jpeg`, starting with the JPEG start-of-image marker and ending with
  the end-of-image marker, 1 byte to 5 MiB, the EMP-DOC-002 budget) for an
  operation already saved in the same partition's protected queue, naming the
  record it documents as one EMP-DOC-003 link target (kind and identifier).
  The device computes the size and SHA-256 from the exact bytes and fixes, at
  capture, the attachment (document) identifier, the declaration operation
  identifier and the link operation identifier; none of them ever changes. The
  bytes are one sealed `ATTACHMENT` entry and the attachment index one sealed
  `attachment-book` v1 record of the creation partition (EMP-PRIV-003
  generation, account, origin, organization, membership and enrollment), both
  written in one protected write before the app reports the capture as saved;
  a failed write reports that nothing was saved. Unsettled attachments of a
  partition hold at most 16 MiB of bytes and the index at most 500 entries;
  beyond that the capture is refused with that reason and nothing is evicted.
  Capture and delivery stay off unless the build flag
  `ENTERPRISE_ATTACHMENT_UPLOAD_ENABLED` is on (default off: real receipts stay
  gated by OQ-04; validation builds turn it on).
- **B-BR-EMP-416 — Operation and attachment outcomes are independent.** The
  owning operation is delivered and acknowledged only by the EMP-OFFL-003
  queue; an attachment never delays, changes, blocks or re-sends it. The
  attachment waits until its operation has a final receipt: `ACCEPTED` or
  `ACCEPTED_FOR_REVIEW` let it be declared; `REJECTED` makes it refused
  (`OWNER_REJECTED`) with its bytes kept on the device; `RETRY` or no receipt
  keeps it waiting. A record can be accepted by the company while its
  attachment is still saved, sending, being verified or refused, and each is
  shown with its own state. The attachment is never shown as verified before
  a server answer reports its document `STORED`.
- **B-BR-EMP-417 — One signed device route declares, links and grants the
  upload.** `POST /v1/orgs/{org_id}/sync/attachments` (special boundary
  `SIGNED_ATTACHMENT_SESSION`) takes the exact signed device proof of the
  reference feed (current account session, identity-v1 signature, current
  enrollment, the body's membership and enrollment equal to the enrolled
  ones) and one declaration: request, declaration, document and link
  identifiers, kind `RECEIPT`, SHA-256, size, media type `image/jpeg`, target
  kind and target identifier. The server declares the document through
  EMP-DOC-002 with the device as actor (channel `android`, creator the
  enrolled membership, `doc.document.attach`, idempotent by the declaration
  identifier), links it through EMP-DOC-003 (the target owner's permission and
  coverage, idempotent by the link identifier) and answers the document's
  storage state and revision and, only while it is `UPLOADING` inside its
  24-hour session, a fresh five-minute upload capability. A repeated request
  answers the current state and a fresh capability: an expired capability is
  renewed by asking again. A target that does not exist or belongs to another
  organization is `NOT_FOUND`; another member's record or a missing permission
  is `FORBIDDEN` with the permission key; a declaration outside C1 is
  `VALIDATION_FAILED`; an identifier reused with other facts is
  `OPERATION_ID_REUSED`; a document identifier taken by another declaration is
  `CONFLICT`; a document already `FAILED`, `REPLACED` or `DELETED` is answered
  with its state and no link or capability. No SQL, migration, permission or
  job is added; a declared document whose link was refused closes after its
  session (B-BR-EMP-254).
- **B-BR-EMP-418 — Bytes reach stored state only through the private ingress
  and its verification.** The app sends the exact sealed bytes, after checking
  their size and SHA-256 again, to the EMP-DOC-002 ingress
  `content/upload` with the capability header and no cookie or signature. A
  `RECEIVED` answer is shown as being verified, `STORED` as verified;
  verified sealed bytes are then removed from the device in the same protected
  write. An ingress refusal for mismatched, truncated or oversized bytes or an
  expired capability leaves the document `UPLOADING` and the attachment
  pending for retry; content refused by the safety check (`QUARANTINED`,
  `FAILED`) makes the attachment refused with that state. Only bytes whose
  size and SHA-256 equal the declaration ever become `STORED`.
- **B-BR-EMP-419 — Retry, resume and process death converge safely.** The
  ingress accepts whole bodies only, so delivery is a whole-body safe retry:
  an interrupted session request or transfer (lost answer, timeout, process
  death at any byte) leaves the attachment pending and the next delivery asks
  for the session again and resends the sealed bytes; the server's replay
  rules (`UPLOADING` accepts the bytes, `RECEIVED` finishes the check,
  `STORED` reads and writes nothing) make a duplicate completion idempotent.
  Pending attachments are retried by the operation worker after the queue
  with the EMP-OFFL-003 policy (30-second base, 6-hour cap, equal jitter, no
  attempt limit, a valid `Retry-After` honoured within the cap), each with its
  own retry state in the index. A sealed entry that is missing, unreadable or
  no longer matches its declaration makes the attachment `FILE_MISSING`
  ("comprovante não encontrado no aparelho"; a new capture is needed), never
  a silent skip or success.
- **B-BR-EMP-420 — Attachment bytes stay private and in their partition.**
  Bytes, digests, identifiers and capabilities never appear in a plaintext
  file, path, URI, cache, log, WorkManager input, notification or
  diagnostics; the capability lives in memory for one transfer only; another
  account or organization on the device can neither read nor send them; they
  share the partition's erasure; the device never deletes unsent bytes by
  itself (BR-OFFL-013), only verified ones.
- **BUC-EMP-211 — Capture offline, upload when the network returns.** A driver
  starts a trip offline with the odometer reading and captures a synthetic
  photo of the odometer for that reading; both show "Salvo no aparelho".
  On reconnection the trip start is accepted, the photo is declared, linked to
  the reading, sent and verified, and the reading's documents list it as
  `STORED`.
- **BUC-EMP-212 — Interrupted and refused uploads converge.** A transfer cut
  in the middle, bytes altered on the way, a capability that expired and an
  app killed during the transfer each leave the attachment pending; the next
  delivery completes it with the same document and the stored original equals
  the declaration; a photo whose sealed bytes are missing is shown as missing;
  a photo of a refused record is shown as refused and kept.
- **BUC-EMP-213 — Prove it on real PostGIS, a private bucket and the
  emulator.** Another organization's route, document or capability, another
  member's or a missing record, an invalid declaration, reused identifiers,
  mismatched and truncated bytes, repeated completion, an expired session
  request and process death are exercised against the composed API with
  PostGIS and MinIO, host suites prove the device rules and the protected
  store across process death, and the owned emulator runs the whole flow
  against a real local backend.

## Pause, resume and cancellation (2026-11-15)

Evidence.

- **B-BR-EMP-390 — Pause, resume and cancellation keep one trip.** The pure
  trip (Go `trip/domain`, Kotlin `enterprise.trip`) gains the commands of
  REQ-07.5 with a new transition table (it replaces the EMP-TRIP-001
  `UNSUPPORTED_COMMAND` refusals): `IN_PROGRESS` + PAUSE is `PAUSED`,
  `PAUSED` + RESUME is `IN_PROGRESS`, CANCEL from `IN_PROGRESS` or `PAUSED`
  is `CANCELLED`, FINISH from `IN_PROGRESS` or `PAUSED` is `COMPLETED` (a
  paused trip may be ended); START, PAUSE on `PAUSED` and RESUME on
  `IN_PROGRESS` are `INVALID_TRANSITION`; any command on `COMPLETED` or
  `CANCELLED` is `INVALID_TRANSITION` (terminal); `PLANNED`, `ASSIGNED` and
  `ACCEPTED` stay `UNSUPPORTED_COMMAND` (missions own them). A trip carries
  an operational revision: 1 after START and exactly one more after each
  PAUSE, RESUME, CANCEL or FINISH. A lifecycle command takes its author
  membership, the expected revision, an optional reason (required for CANCEL;
  when present nonblank, control-free and at most 280 characters, the
  METER-001 profile) and the time-kernel declaration, and fails first on
  scope (`SCOPE`; an unconstructed trip `INVALID`), then the author (absent
  `INVALID`, another organization `SCOPE`), the present reason and
  declaration (`INVALID`), a non-positive expected revision (`INVALID`), then
  the absent parts as `INCOMPLETE` with the ordered list REASON (CANCEL only),
  DECLARATION, then a member other than the trip's responsible member — the
  member who started it — (`FORBIDDEN`), then a terminal trip
  (`INVALID_TRANSITION`), then a stale revision (`CONFLICT`), then the table
  (`INVALID_TRANSITION`), then an instant strictly before the start instant
  (`BEFORE_START`; an equal instant is accepted). An accepted command returns
  the next trip value with the same identity, organization, asset, driver,
  start event and start reporting attribution and appends one lifecycle fact
  (command, revision, previous and next state, author, declaration, reason);
  earlier facts never change. A cancelled trip is terminal, has no finish and
  reports usage `NOT_MEASURED` with the reason `CANCELLED` even with a
  departure reading (no distance is fabricated); a paused trip keeps
  `NOT_FINISHED`. A logbook event of a cancelled trip strictly after the
  cancellation instant is `EVENT_AFTER_FINISH`, as after a finish; a paused
  trip takes events. Both languages execute the shared `trip/trip-v2.psv`
  oracle, which supersedes `trip-v1.psv` (kept unchanged, no longer
  consumed).
- **B-BR-EMP-391 — Signed lifecycle operations decided at receipt.** The
  device sends `trip.pause`, `trip.resume` and `trip.cancel` schema 1:
  envelope entity `trip` with the trip identity and the required
  `expected_revision` (1..1 000 000), payload exactly `{"reason": string |
  null}`, at most one dependency, no soft references or attachments
  (`RETRY`/`UNAVAILABLE`). Each is decided on current facts under the
  organization root and asset locks with the `trip.trip.start` authority of
  EMP-TRIP-002: a wrong shape, a CANCEL without reason or an instant before
  the start is a final `MALFORMED`; a member other than the trip's
  responsible member is `FORBIDDEN`; a trip not stored here (undelivered,
  held or another organization's) is `RETRY`/`DEPENDENCY_PENDING` and
  reveals nothing; a trip already `COMPLETED` or `CANCELLED` (a resume after
  a terminal state) is held `ACCEPTED_FOR_REVIEW` `TRIP_ALREADY_CLOSED`; a
  stale expected revision, or a command the trip's current state no longer
  admits, is held `STALE_REVISION` (EMP-TRIP-015) and changes nothing;
  changed rights store nothing (`RETRY`/`UNAVAILABLE`, as for the finish).
  An accepted command is acknowledged `ACCEPTED` at the trip's new revision;
  the identical resend returns the stored acknowledgement byte for byte and
  the same identifier with another payload is `OPERATION_ID_REUSED`
  (BR-OFFL-004). A `trip.finish` is acknowledged at the trip's revision after
  the finish (2 for a trip without lifecycle commands, as before). The
  reserved logbook types PAUSE, RESUME and CANCEL of `trip.event.record`
  keep their EMP-TRIP-003 answer (`RETRY`/`UNAVAILABLE`, shared device vectors
  unchanged): the lifecycle has its own operations.
- **B-BR-EMP-392 — A paused trip holds its asset; a cancelled one releases
  it.** The one-active-trip rule, FLEET-003 open work, the EMP-TRIP-013
  running-trip check and the `ASSET_BUSY` re-proof count `IN_PROGRESS` and
  `PAUSED` trips; FLEET-007 active use holds the asset from the start until
  the finish or the cancellation instant. After a cancellation the next
  start on the asset is admitted. A finish of a cancelled trip and a logbook
  event declared after its cancellation are held `TRIP_ALREADY_CLOSED`; an
  event at or before it is a late delivery and is accepted. A cancelled trip
  is never submitted: its `trip.submit` is a final `MALFORMED`. Cancellation
  writes no review entry and no reading; the review case stays `DRAFT`.
- **B-BR-EMP-393 — Lifecycle history and database guards.** Each accepted
  command appends one insert-only, forced-RLS lifecycle row (trip, revision,
  command, previous and next state, reason, operation, payload digest,
  author membership and employee, enrollment, declaration and server
  receipt) and one audit event and receipt through the intake. The database
  moves a trip's state only together with the lifecycle row of its next
  revision (or the finish), re-proves the command's operation, author,
  table, revision, reason policy and start instant, publishes a row only with
  its `ACCEPTED` acknowledgement at that revision, and keeps held lifecycle
  facts whose condition (`TRIP_ALREADY_CLOSED` or `STALE_REVISION`) it
  re-proves. Transitions reference only the trip, operation, membership,
  employee record and enrollment, so no asset, unit or cost-centre usage
  source changes. Synthetic data only.
- **BUC-EMP-197 — Pause for lunch, resume, or cancel a trip.** A driver
  starts at 184 320 000 m, pauses at 12:30 with the reason "Almoço" (revision
  2), resumes at 13:30 (revision 3) and finishes (revision 4, 290 000 m); the
  timeline keeps every fact with its actor and time. In another trip the
  customer cancels: the driver cancels with the reason "Cliente cancelou";
  the trip is `CANCELLED` with its start reading and no distance, and the
  vehicle is free for the next start.
- **BUC-EMP-198 — Prove the lifecycle on real PostGIS.** Pause, resume and
  cancel through the signed intake with exact replay and identifier reuse;
  resume after a terminal state; stale and duplicated requests; a missing
  reason; another organization's trip; another member's pause; a paused
  trip holding its asset and a cancelled one releasing it; the database
  guards and a populated upgrade from 000232 with injected failure and
  recovery.

## Asset-overlap conflict resolution (2026-11-15)

Evidence.

- **B-BR-EMP-407 — The held trip facts are a company review queue.** Every
  trip fact the intake keeps for review is listable and readable by a member
  holding `trip.trip.review`: a held `trip.start` (`ASSET_BUSY`,
  `ASSET_UNAVAILABLE`, `PERMISSION_CHANGED`), a held `trip.finish`
  (`TRIP_ALREADY_CLOSED`), a held `trip.event.record` (`TRIP_ALREADY_CLOSED`,
  `STALE_REVISION`), a held `trip.submit` (`STALE_REVISION`) and, since
  EMP-TRIP-011, a held `trip.pause`, `trip.resume` or `trip.cancel`
  (`TRIP_ALREADY_CLOSED`, `STALE_REVISION`). The queue is
  an organization-level list (only an `ORG` grant covers it, as the trip
  review queue) filtered by state (`OPEN`, `EXPIRED_UNRESOLVED` after the
  60-day review-expiry-v1 period, `RESOLVED_ACCEPTED`, `RESOLVED_REFUSED`) in
  item order; one item is read through a structural grant covering its
  structure: a held start the asset it declared, that asset's FLEET-005 unit
  and cost centre at the declared instant and the teams covering its author
  or driver at the receipt; any other held fact the structure of the trip it
  names. A read shows the item (operation, type, payload digest, code, state,
  revision 1 or 2, author account, membership, employee record, enrollment,
  receipt and expiry), the exact held fact (declaration, asset, driver,
  reading or stated absence, run clock, event, submitted revision, lifecycle
  command with its named revision and reason) and the trips it conflicts
  with: for a held start every trip of the asset in progress or paused, or
  ended (finished or cancelled) after its declared start, otherwise the trip
  it names with its operational state and end. Another organization's item, an unknown item
  and a meter item are `NOT_FOUND`; a member without the permission or whose
  structural grant does not cover the item is `FORBIDDEN`; `AUDITOR` is
  `FORBIDDEN`. A held fact is never a trip, active use, cost or indicator.
- **B-BR-EMP-408 — Two outcomes, accept only as the declared trip.** A
  resolution is `ACCEPT` or `REFUSE` with a reason (METER-001 text profile,
  280 characters) and names the item revision 1. `ACCEPT` exists only for a
  held start: the start is stored as the trip it declared (its identity,
  asset, driver, declaration, run clock and reading or stated absence; the
  FLEET-005 unit and cost centre and the start reporting day of its declared
  instant; the author's membership and enrollment) in progress at entity
  revision 1, its departure reading becomes a `TRIP_START` reading of the
  same operation received at the original receipt and classified there
  (EMP-METER-003), and the operation becomes ready, so the author's pending
  `trip.finish` is accepted on its next delivery. Accepting a held finish,
  event or submission is `CONFLICT`: the trip already has its finish, its
  logbook is closed after the finish and a submitted or decided revision is
  never rewritten (BR-TRIP-003); a late correction is an amendment.
  Accepting a held lifecycle command is `CONFLICT` too: its trip is closed or
  its named revision is stale, and the author records the command again. Accepting
  with another asset is not offered (the held reading belongs to the declared
  asset's meter). No resolution changes the winning trip, its readings, its
  logbook or its review history.
- **B-BR-EMP-409 — A resolution never makes two trips active.** A held start
  is accepted only when the asset is still `ACTIVE` and was not disposed at
  the declared instant, the author's membership is `ACTIVE` with the same
  enrolled device, the driver is an `ACTIVE` unmerged employee record, and no
  stored trip of the asset is in progress or finished after the declared
  start (the accepted trip is in progress from its declared start, so at no
  instant do two trips of the asset count as in progress, VAL-07; a paused
  trip still holds the asset and a cancelled one holds it until its
  cancellation, EMP-TRIP-011);
  otherwise the resolution is `CONFLICT` and writes nothing, and the item
  stays `OPEN`. The database re-proves every condition in the same
  transaction, including against a concurrent start or a concurrent
  acceptance of another held start of the asset.
- **B-BR-EMP-410 — One audited decision, once.** Resolving needs fresh
  `trip.trip.review` covering the item and an `ACTIVE` organization; the
  reviewer is never the author (account, membership or canonical employee
  record after merges) nor, for a held start, the declared driver
  (`SELF_APPROVAL`, BR-REV-003). The decision names revision 1; a resolved
  item or another revision is `CONFLICT` (BR-REV-007), so of two reviewers
  deciding at once exactly one resolution applies and the other gets
  `CONFLICT`. One resolution, its audit event `offl.review.resolved`
  (target `offl.review`, carrying the command digest, never the reason) and
  for an accepted start the trip, reading and readiness are written in one
  transaction; the same operation identifier and body replays the stored
  resolution (`replayed`), the same identifier with another body is
  `OPERATION_ID_REUSED`.
- **B-BR-EMP-411 — The conflict evidence is kept and the author learns the
  outcome.** No resolution deletes, rewrites or detaches the held fact: the
  review item, the held row, the original `ACCEPTED_FOR_REVIEW`
  acknowledgement bytes and the resolution stay stored and readable after
  either outcome, and the database refuses any update or delete of them. The
  author's signed review-results read (EMP-OFFL-008) answers the held trip
  items like the meter ones: the immutable resolution (`ACCEPTED` with entity
  revision 1, or `REFUSED` with the reviewer's reason) or none while open,
  only to the original author, organization, membership and enrolled device.
- **BUC-EMP-206 — Resolve two starts on one vehicle (VAL-07).** Bruno and
  Diego start FR-001 offline at 08:00 and 08:05; Diego's start is received
  first and runs; Bruno's is held `ASSET_BUSY`. The reviewer opens the queue,
  reads Bruno's held start beside Diego's trip, and cannot accept it (it
  would overlap Diego's trip): Bruno's start is refused with the reason
  "Veículo em uso por outra viagem". Two reviewers deciding at once leave one
  resolution; the other sees `CONFLICT`. Bruno's phone receives the refusal
  at its next review-results poll; the held start stays readable.
- **BUC-EMP-207 — Accept a start held while the asset was blocked.** A driver
  starts a vehicle offline while a document block was registered by mistake;
  the start is held `ASSET_UNAVAILABLE`. With no trip of the vehicle after
  that instant, the fleet manager accepts it: the trip runs from its declared
  start with its departure reading at the original receipt, and the
  driver's finish, waiting as a pending dependency, is accepted on the next
  delivery.
- **BUC-EMP-208 — Prove conflict resolution on real PostGIS.** Self-review
  and the declared driver, a second active trip produced by a resolution
  (racing a new start and another acceptance, both orders), repeated
  resolution (replay, reuse, two reviewers racing), loss of the conflict
  evidence, another organization's item, an uncovering structural grant,
  `AUDITOR`, accepting a finish, event or submission, the author's own
  result read, the database guards and a populated upgrade from 000232 with

## Missing final reading and supervisor closure (2026-11-15)

Evidence.

- **B-BR-EMP-399 — A permitted supervisor closes another member's open
  trip.** A member holding `trip.trip.close_for_other` (org-permissions-v5:
  OWNER, ADMIN, FLEET_MANAGER, SUPERVISOR) covering the trip — an `ORG`
  grant, or a `UNIT`, `COST_CENTRE`, `TEAM` or `ASSET` grant through the
  trip's stored structure (EMP-TRIP-004 `TripStructure`) — closes an open
  trip (`IN_PROGRESS`, or `PAUSED`, EMP-TRIP-011) at its current operational
  revision (1 after the start, one more per pause, resume or cancellation),
  with fresh company proof
  in an `ACTIVE` organization, stating why it is closed (reason) and why no
  final reading exists (missing-reading reason); both use the METER-001
  text profile (1..280 Unicode scalars, no control character). The trip
  becomes `COMPLETED` at the next operational revision and stops holding its asset
  (the one-active-trip rule). The closure is operational only: the review
  case stays as it was (`DRAFT`), nothing is submitted or approved, and the
  author may submit it through `trip.submit` like any completed trip.
  Closing is not gated by a finance period lock (it is a field fact, not a
  review decision; EMP-TRIP-016). `AUDITOR`, `FINANCE`, `APPROVER` and
  `DRIVER` are `FORBIDDEN`; a structural grant that does not cover the trip
  is `FORBIDDEN`; an unknown or another organization's trip is `NOT_FOUND`.
- **B-BR-EMP-400 — Missing evidence stays visible; nothing is invented.**
  A closure carries no meter reading, value, segment or distance: the
  request has no such field (a body naming one is `MALFORMED`), the stored
  closure has no such column, no `TRIP_END` reading is created, and the
  trip's usage is `NOT_MEASURED` with reason `READING_ABSENT`, never a
  distance (BR-TRUTH-003). The closure (instant, closing member, reason and
  missing-reading reason) is returned by the command and shown in the
  trip's review decision context beside the absent finish. A reading taken
  later, such as the yard reading of JRN-07, is a separate meter reading
  with its own author and time; tying it to the trip is a reviewed
  amendment, never an edit of the closure.
- **B-BR-EMP-401 — No self-decision (BR-REV-003).** The closing member is
  never the trip's author or driver: sharing the account, the membership or
  a canonical employee record (merge chain) with either is `SELF_APPROVAL`;
  the trip's own author finishes it through `trip.finish` instead. The
  member who closed a trip never opens or decides any review revision of
  that trip (`SELF_APPROVAL`), whatever roles that person holds; another
  scoped reviewer decides it. The database re-proves both separations.
- **B-BR-EMP-402 — Field facts after a closure are kept for review.** The
  driver's `trip.finish` received after the closure is held
  `TRIP_ALREADY_CLOSED` with its declared reading values (the EMP-TRIP-002
  held finish, never a stored reading or a completion); a logbook event
  received after the closure is held `TRIP_ALREADY_CLOSED` whatever its
  declared instant, because a closure has no device instant to compare
  with (BR-OFFL-006); nothing is dropped. A driver finish and a supervisor
  closure of the same trip serialize on the organization root and the trip
  row: when the finish commits first the closure is `CONFLICT` and writes
  nothing; when the closure commits first the finish is held. A closed trip
  holds its asset only until the closure is recorded; no end instant is
  invented for it.
- **B-BR-EMP-403 — Command, list, replay, audit and database guards.**
  `POST /v1/orgs/{org_id}/trips/{trip_id}/close` (fresh write, expected
  revision required) and the organization list of trips in progress
  `POST /v1/orgs/{org_id}/trips/open/list` (read; an organization-level
  resource that only `ORG` grants cover; open trips, `IN_PROGRESS` or
  `PAUSED`, oldest start first; trip, asset, driver employee, author
  membership, start instant, start reporting day, operational state
  and revision only). A stale revision or a completed or cancelled trip is
  `CONFLICT`; a missing or invalid reason `VALIDATION_FAILED`. The same
  operation and digest returns the first result with `replayed=true` and no
  second write or audit event; another digest is `OPERATION_ID_REUSED`. One
  `trip.trip.closed` audit event (target `trip`, digest, no reason text).
  The database keeps the closure insert-only and forced-RLS, accepts it
  only for an open trip from an active, unmerged member separated
  from the author and driver, makes it the only completion besides the
  finish, and publishes it only with its audit event.
- **BUC-EMP-202 — Close a trip left open (JRN-07).** A trip has been in
  progress for 30 hours with no contact; the supervisor of its unit finds
  it in the open-trip list, contacts the driver, and closes it with the
  reason "Motorista sem sinal; viagem encerrada pela supervisão" and the
  missing-reading reason "Leitura final não registrada". The trip is
  `COMPLETED`, its usage is `NOT_MEASURED` (`READING_ABSENT`), the review
  case is still `DRAFT`; the driver's phone later delivers its finish and a
  stop, both held for review.
- **BUC-EMP-203 — Prove supervisor closure on real PostGIS.** The
  supervisor closing their own trip and later reviewing the closed trip; a
  request carrying a meter value; another organization's and an unknown
  trip; an uncovering structural grant and denied roles; a stale second
  closure; a driver finish and a supervisor closure racing in both orders;
  replay and operation reuse; the database guards and a populated upgrade
  with injected failure and recovery.

## Open trips when a membership ends (2026-11-15)

Evidence.

- **B-BR-EMP-404 — A trip in progress is an open responsibility of its
  member.** The EMP-IDN-010 offboarding review lists every trip still
  open (`IN_PROGRESS` or `PAUSED`) that the ending membership started as the
  handover item
  `TRIP:<trip id>` at source revision 1 (a fixed item revision), beside the
  team and settings items, inside the reviewed digest. Completing the
  offboarding needs an explicit pending-manager flag for it
  (`OPEN_RESPONSIBILITIES` otherwise); a trip started, finished or closed
  between the review and the completion changes the digest
  (`OPEN_RESPONSIBILITIES`). The database refuses an offboarding that leaves a
  trip in progress of that membership unflagged or flags anything else as a
  trip. The flag stays readable after the end. Another member's trips and
  another organization's trips are never listed.
- **B-BR-EMP-405 — Offboarding never closes, hands over or deletes the
  trip.** The trip stays `IN_PROGRESS` with every stored fact unchanged
  (rows remain insert-only; a delete is refused) and keeps holding its asset.
  It is resolved under scoped authority: the open-trip list shows it with the
  author membership state `ENDED`, and a member holding
  `trip.trip.close_for_other` covering it closes it with the
  EMP-TRIP-014 closure (the successor responsible for the trip in this
  phase); a successor driver taking over a running trip is EMP-TRIP-012's.
  The ended member can neither finish nor close it as normal work.
- **B-BR-EMP-406 — Late intake follows EMP-OFFL-009.** From the exact
  enrollment period that the membership end revoked `ACCESS_ENDED`: a
  `trip.finish` of the member's own trip declared (or claimed) before the
  end and received inside the intake window is held
  `ACCESS_ENDED_BEFORE_RECEIPT` with its declared reading as an OFFL-008 held
  finish, never applied (the trip stays in progress; a reviewer or the
  closure decides); a finish of an unknown trip, of another member's trip or
  declared before the trip's start is `MALFORMED`; anything declared after
  the end is `MEMBERSHIP_ENDED`, after the window `INTAKE_WINDOW_CLOSED`,
  and a key revoked lost or compromised is refused as a whole
  (`DEVICE_REVOKED`). Late starts, events and submissions keep the OFFL-009
  default for owners without a held path (`RETRY`/`UNAVAILABLE`, never
  dropped) until their owners add one.
- **BUC-EMP-204 — Offboard a driver with a trip in progress (JRN-04).** The
  manager offboards Bruno on Friday; the review lists his open trip, which
  is flagged for the manager; his access ends at once; Ana finds the trip in
  the open-trip list marked `ENDED` and closes it without a final reading;
  on Monday his phone delivers the finish he recorded before 18:00, which is
  held for review, while a stop recorded on Saturday is refused.
- **BUC-EMP-205 — Prove the membership end on real PostGIS.** An offboarding
  that omits the trip flag or whose source omits the trip; the ended
  member's later operations; a lost device; another organization; the trip
  never deleted or closed by the offboarding; a populated upgrade with
  injected failure and recovery.

## Track intake: batch endpoint, de-duplication and segment storage (2026-11-15)

Evidence.

- **B-BR-EMP-428 — Tracking is on only with a policy and its notice in
  force.** The server resolves an organization's tracking policy from its
  insert-only recorded policy versions at receipt: the latest version
  effective at that instant gives `OPTIONAL` or `EXPECTED` only when its
  employee notice (a named notice version) is in force at that instant;
  no version, a version `OFF` or a notice not yet in force is `OFF`
  (EDR-271). A session opens only under `OPTIONAL` or `EXPECTED` and keeps
  the policy version it opened under; opening under `OFF` is refused with a
  final `REJECTED` `FORBIDDEN` receipt and nothing else is stored. Turning
  the policy off never touches a stored session, transition or segment.
  Recording a policy version is not part of this task (no route writes it).
- **B-BR-EMP-429 — Session transitions are signed device operations.** The
  owner's enrolled device records the session through the OFFL-002 signed
  intake with six schema-1 operations on the entity `tracking_session`
  (the session identity): `gps.session.start` and `gps.session.resume`
  carry the trip, the origin, the device permission and whether the tracking
  notification is visible; `gps.session.suspend`, `gps.session.interrupt`,
  `gps.session.reduce` and `gps.session.end` carry the trip and the cause.
  The first `start` of a trip opens the session `NOT_STARTED` at its
  declared instant and applies the start in the same transaction. Every
  operation needs current `trip.trip.start` coverage over the stored trip
  (receipt reads need `trip.trip.read`), and the EMP-GPS-001 domain decides
  it at receipt with the trip's facts at the declared instant: `IN_PROGRESS`
  from its start, then `PAUSED`, `IN_PROGRESS`, `CANCELLED` or `COMPLETED`
  from its latest stored pause, resumption, cancellation or finish at or
  before that instant (EMP-TRIP-011). Before deciding, the stored session
  follows (EMP-GPS-001 `FOLLOW`) every stored pause, cancellation and finish
  of the trip from its last transition up to, not including, the operation's
  instant: a pause suspends a recording session (`TRIP_PAUSED`, actor
  `TRIP`), a cancellation or finish ends it; those rows are stored with the
  operation, and recording returns only through the driver's resume. An accepted operation is
  `ACCEPTED` with the session's history length as its entity revision; a
  refused permission or hidden notification is accepted as the recorded
  attempt (B-BR-EMP-422). Policy `OFF` is `REJECTED` `FORBIDDEN`; another
  member is a whole-request `FORBIDDEN` that stores nothing; every other
  domain refusal (personal or implicit origin, second session, invalid
  transition, trip state, out-of-order or malformed instant or value) is a
  final `REJECTED` `MALFORMED` that stores nothing; a trip or session not
  stored here is `RETRY` `DEPENDENCY_PENDING` without a claim; an identical
  resend returns the stored receipt byte for byte and the same identifier
  with another digest is `OPERATION_ID_REUSED`.
- **B-BR-EMP-430 — One session per trip, kept insert-only.** The database
  holds at most one session per trip, bound to the organization, the trip,
  the trip's driver membership and the policy version; its transitions are
  insert-only rows with a contiguous sequence, non-decreasing instants, the
  closed state, cause, actor and permission tokens, nothing after `ENDED`,
  and only operations of the owner's enrolled device. A stored session is
  rebuilt by replaying its rows through the EMP-GPS-001 domain, which must
  reproduce them exactly, or the intake is unavailable. Rows are never
  updated or deleted.
- **B-BR-EMP-431 — A track batch is one bounded operation.** A batch is the
  schema-1 operation `gps.track.batch` on the entity `tracking_session`; its
  operation identifier is the batch identifier. The payload names the trip,
  a positive sequence, the source `PHONE`, an optional boot identity with
  its base elapsed milliseconds, a base position in 10⁻⁷ degrees, 1 to 200
  fixes `[offset_ms, Δlat_e7, Δlon_e7, accuracy_m, speed_cm_s, bearing_deg,
  flags]` and the device's dropped counters. The base instant is the
  declared instant (whole microseconds); offsets increase strictly up to 24
  hours; coordinates stay within ±90° and ±180°; accuracy is 0 to 10 000 m,
  speed 0 to 100 000 cm/s or null, bearing 0 to 359 or null, flags bit 0
  coarse and bit 1 mock. An operation is at most 8 KiB (one upload request
  at most 64 KiB and 50 operations); more than 200 fixes or any invalid value
  is a final `REJECTED` `MALFORMED` that stores nothing.
- **B-BR-EMP-432 — Only fixes inside the session are stored.** A fix is
  stored only when its instant lies in a recording period of the stored
  session as it follows the trip (the latest transition at or before it is
  `RECORDING` and no trip pause lies between the session's last transition
  and the fix) while the trip runs: at or after its start, outside its paused
  intervals and before its finish or cancellation; it is stored coarse when
  that period is approximate. Other fixes are discarded and counted as
  `OUTSIDE_SESSION`; a mock-flagged fix is discarded and counted as mock. A
  batch whose session is not stored yet is `RETRY` `DEPENDENCY_PENDING`; a
  batch naming another trip than its session's is `MALFORMED`. The database
  refuses a stored fix outside a recording period, while the trip is paused,
  at or after its finish or cancellation, or with the wrong precision.
- **B-BR-EMP-433 — One accepted batch has one effect.** The first accepted
  delivery of a batch stores exactly one segment, keyed by the organization
  and the batch identifier and unique per session sequence, and answers
  `ACCEPTED` with the sequence as entity revision; resends return the stored
  receipt; a second batch with a used sequence is `MALFORMED`. A fix whose
  instant is already stored for the session is counted once, as a duplicate
  of the later batch. Batches of one session (and the trip's finish) are
  serialized on the trip row, and the database refuses an instant stored
  twice for a session, in either arrival order. The received, stored,
  duplicate, outside-session and mock counters and the device's dropped
  counters are stored with the segment; the receipt carries none of them.
- **B-BR-EMP-434 — Segments are raw, scoped and private.** A segment keeps
  the exact integer positions, instants, accuracy, speed, bearing and coarse
  flags as arrays, a database-derived `MULTIPOINT` of SRID 4326, the source
  `PHONE`, the device enrollment and author, the boot identity and the
  receipt instant; it is insert-only (BR-GPS-023), organization-scoped with
  forced row security and composite organization keys. No coordinate appears
  in an error, log, receipt, audit event or printed value.
- **BUC-EMP-217 — The driver's device records its session.** The driver
  starts tracking on the trip screen: the device's `gps.session.start` opens
  and starts the session on the server under the organization's policy;
  later suspend, resume, interrupt, reduce and end operations append the
  session's history, and the server's recording periods are exactly the
  domain's.
- **BUC-EMP-218 — The device uploads a track batch.** Every minute the
  device sends its oldest batches after its pending operations; the server
  checks the signed source, the trip coverage and the session, stores the
  fixes inside the recording periods as one segment and answers `ACCEPTED`;
  fixes outside the session or after the trip's finish are counted and
  dropped; a batch for a session not stored yet is retried.
- **BUC-EMP-219 — Resends and overlaps converge.** A batch resent after a
  lost answer returns the stored receipt; the same identifier with another
  body is refused; a second batch overlapping stored fixes stores only the
  new ones; two batches of one session racing in either order store each
  instant once; a foreign trip, a personal or implicit start, a fix while the
  trip is paused or after it ended or was cancelled, an oversized batch and
  policy `OFF` store nothing.

## Location access permission and access audit (2026-11-15)

Evidence.

- **B-BR-EMP-435 — Recorded tracks are read only under `gps.track.read`.**
  The company route `POST /v1/orgs/{org_id}/trips/{trip_id}/track/read`
  serves a stored trip's recorded track only to a current member whose
  `gps.track.read` grant covers that trip, evaluated on server facts: the
  trip's author membership is its own record and its driver, the read is a
  recorded track, and structural grants use the trip's stored asset, unit,
  cost centre and teams. A `DRIVER` reads only the tracks of trips it drives
  (`OWN_TRIPS_ONLY`); an `AUDITOR` reads recorded tracks (`RECORDED_TRACKS_ONLY`);
  `trip.trip.read`, another permission, a platform staff assignment or a
  support role never substitutes. A member without the permission, or whose
  scope does not cover the trip, is `FORBIDDEN` naming `gps.track.read`; a
  trip not stored in the organization, another organization and an account
  without a current membership are `NOT_FOUND`. Authority is proved again
  inside the read transaction with the company proof.
- **B-BR-EMP-436 — Every read is recorded before any byte is served.** Each
  successful authorization records, through the PRIV-005 access log, one
  `LOCATION_TRACK` issuance (`location.track.issued`) and then one served
  access (`location.track.accessed`) referencing it, each with a
  server-generated request identifier, the reader's account and membership,
  the trip as target and a 64-hex digest of the served page's identities
  (trip, session, segment sequences) — never a coordinate. Only after both
  records commit is the response written; a failed or refused record answers
  `UNAVAILABLE` and serves no track byte. Live positions (`LOCATION_LIVE`,
  `gps.location.read_live`) have no route in F4: the live projection belongs
  to EMP-GPS-007, and an `AUDITOR` holds no live grant.
- **B-BR-EMP-437 — The read serves what was recorded, page by page.** A
  page names the trip, its session (identity, current state, policy, opening
  instant and every transition with state, cause, actor, permission and
  instant) and up to 20 segments in sequence order (default 20) with their
  source `PHONE`, receipt instant, counters and fixes (instant, 10⁻⁷ degree
  latitude and longitude, accuracy, optional speed and bearing, coarse flag);
  a cursor bound to the reader, trip and page size continues after the last
  sequence. A trip without a session or segments is an empty page. Nothing is
  derived (no distance, gap, quality or live position) and no coordinate
  appears in an error, log or audit record.
- **BUC-EMP-220 — A supervisor or an auditor replays a recorded track.** A
  fleet manager, a supervisor of the trip's unit or an auditor opens a trip's
  track page by page; each page is recorded as issued and accessed before it
  is shown.
- **BUC-EMP-221 — Location reads refuse without a grant.** A driver opening
  another driver's trip, a finance member who reads trips, a supervisor of
  another unit, a platform staff account, another organization and a failing
  access log all get nothing; a driver reads only its own trips.

## Expense persistence, sync handlers and the fueling-to-expense projection (2026-11-15)

Evidence.

- **B-BR-EMP-444 — One stored cost per origin, enforced by the database.**
  `org_expenses` keeps the original revision-1 fact of each expense, insert
  only, under its origin key (organization, kind, source): a `FUELING`
  expense is keyed by its fueling, a `DIRECT` expense by its own identity
  (EDR-131). The database refuses a second expense for one origin (the
  unique origin key and the domain-model unique (organization, fueling)),
  any origin kind other than `FUELING` and `DIRECT` (`WORK_ORDER` waits for
  its maintenance owner; payment, reimbursement, card statement, advance,
  payable, document and stock kinds are never stored as a cost), a `FUELING`
  expense whose fueling is not a stored fueling of the same organization or
  whose facts differ from it, a `DIRECT` expense that names a fueling, and
  any update, delete or truncate. Amounts are exact integer centavos with
  0 ≤ company ≤ paid ≤ 10^12. The stored review state is `SUBMITTED` at
  revision 1 with source revision 1; decisions, returns, amendments and voids
  belong to their review owners (FUEL-003 for fuelings, EXP-005 for direct
  expenses).
- **B-BR-EMP-445 — A fueling and its expense commit together.** Accepting a
  `fueling.submit` stores, in the same intake transaction and after the
  fueling, exactly one `FUELING` expense derived from the stored fueling
  only (never from the device payload): category `FUEL`, paid amount and
  company amount equal to the fueling's paid total (the discount is not a
  cost; a partial company amount is a later review decision), the fueling's
  payer, asset, author membership and employee, enrollment, operation and
  payload digest, competence equal to the fueling's declared instant, unit
  and cost-centre snapshot equal to the fueling's stored snapshot, and the
  reporting day of that instant in the snapshot unit's zone or else the
  organization's zone (BR-TIME-004). Neither half may exist alone: a fueling
  inserted after migration 000244 cannot commit without its expense and an
  expense cannot reference a fueling that is not stored; a failure before or
  after either insert rolls both back and the operation stays
  `RETRY`/`UNAVAILABLE` with nothing stored. Fuelings accepted before 000244
  have no expense until the idempotent per-organization catch-up projects
  them: each then has exactly one, and a second run changes nothing.
- **B-BR-EMP-446 — The projection is idempotent and never overwrites.** The
  projection names the fueling and the fueling revision it projects. Run for
  a fueling whose expense already exists at that source revision it returns
  the stored expense unchanged; a request for an older source revision than
  the stored expense is stale and writes nothing; a newer source revision
  stays unavailable until the fueling revision owners (FUEL-003/015) exist.
  Review decisions, document links, payables, reimbursements and payments
  never insert or change an expense row, so the recognized amount is counted
  once: the EMP-EXP-001 totals over the stored expenses give one cost of the
  fueling's paid total per fueling, `includes pending` while it is
  `SUBMITTED`.
- **B-BR-EMP-447 — The signed `expense.submit` (schema 1) records one direct
  expense.** The entity is `expense` with the device's expense identity and
  no expected revision; the competence is the envelope declaration. The
  payload has exactly the keys `asset_id`, `category`, `paid_total`,
  `company_amount`, `payer` and `trip_id`. The asset is required: it scopes
  the expense with the asset's FLEET-005 unit and cost centre at the
  competence instant and the reporting day follows from that unit's zone or
  the organization's. Until the configured catalogue of EMP-EXP-003 exists
  every organization has the v1 direct categories `TOLL`, `PARKING`,
  `MAINTENANCE`, `LEASE`, `INSURANCE`, `FINE` and `OTHER`; `FUEL` is not a
  direct category (a fuel cost is a fueling, EDR-013) and is `MALFORMED`,
  another code of the key profile is a configured-catalogue form kept
  `RETRY`/`UNAVAILABLE`, any other text `MALFORMED`. `paid_total` and
  `company_amount` are canonical centavos with company ≤ paid; `payer` is
  `COMPANY` or `EMPLOYEE`; a canonical `trip_id` (trip closing conference),
  attachments, dependencies and soft references are recognised forms kept
  `RETRY`/`UNAVAILABLE` with nothing stored; an unknown or missing key, a
  non-canonical value, a `DRAFT` asset or an expense identity already used
  by another operation is `MALFORMED`. An accepted expense is stored
  `SUBMITTED` at revision 1 with one `ACCEPTED` ACK at entity revision 1.
  Intake never consults the APPR-006 period gate (B-BR-EMP-266 precedent).
- **B-BR-EMP-448 — Authority at receipt for direct expenses.** Only a
  current enrolled operational device of the author records. History of
  `exp.expense.record` (an `S` cell for every holder) follows EDR-118
  continuity from the declared instant to the receipt: an `ORG` grant with a
  positive rule covers the member's own expense on any asset of the
  organization; a `SELF` grant covers it only while the asset is assigned to
  the member (FLEET-006 periods); structural grants stay unsupported in the
  signed intake. Never covered (for example `AUDITOR`, or a `SELF` member on
  an asset never assigned to them) is `FORBIDDEN`; a missing or foreign
  asset is `NOT_FOUND`; both refuse the whole response with nothing stored.
  Changed rights or a competence at or after the asset's disposal store
  nothing and stay `RETRY`/`UNAVAILABLE` until a held-expense owner exists. A
  receipt replay needs current `exp.expense.read` over the stored record
  (author, asset, stored unit and cost centre).
- **B-BR-EMP-449 — Exact replay, isolation and history.** The same operation
  id and digest returns the stored ACK byte for byte and creates no second
  expense or audit; the same id with another digest is `REJECTED`
  `OPERATION_ID_REUSED` and changes nothing; concurrent identical
  deliveries leave exactly one expense. `org_expenses` is forced-RLS (no row
  without an organization setting, none of another organization), refuses
  update, delete and truncate even with grants, and uses composite
  same-organization foreign keys; the asset, unit and cost-centre usage
  sources list its keys as associations, never as current use.
- **BUC-EMP-225 — One fueling, one cost (VAL-13, intake part).** Bruno's
  fueling of 97 350 centavos paid from his own pocket is delivered, resent
  twice and resent with another digest; twelve identical deliveries race.
  The organization has one fueling and one `FUEL` expense of 97 350
  (company 97 350, payer `EMPLOYEE`); its period figure is 97 350 including
  pending and 0 approved, never 194 700; a second expense for the fueling
  cannot be inserted in either order relative to the fueling's transaction.
- **BUC-EMP-226 — Record a toll offline.** A driver assigned FR-001 records a
  toll of R$ 12,50 offline; at the next connection the server stores one
  `DIRECT` `TOLL` expense of 1 250 centavos with the vehicle's unit at that
  instant and acknowledges it once; a resend returns the same ACK.
- **BUC-EMP-227 — Prove the expense store on real PostGIS.** A foreign
  fueling or asset, a second cost for one fueling (both orders), a partial
  transaction in both directions, a stale projection, replay and
  `OPERATION_ID_REUSED`, concurrent duplicates, wrong role and wrong scope,
  forced RLS and insert-only history, and a populated upgrade from the 000237
  head with injected failure, recovery and the catch-up run twice.

## Driver handover during a trip (2026-11-15)

Evidence.

- **B-BR-EMP-394 — A handover is a supervisor's online command on an open
  trip.** A handover moves the responsibility for one open trip
  (`IN_PROGRESS` or `PAUSED`) of the organization from its current
  responsible member to another member. It is recorded online on a company
  session by a member holding `trip.trip.close_for_other` with fresh proof
  over the trip's stored structure (ORG, or the unit, cost centre, team or
  asset of the start snapshot) on an `ACTIVE` organization:
  `POST /v1/orgs/{org_id}/trips/{trip_id}/handover` with `schema_version`
  1, `operation_id`, the trip's operational `revision` the supervisor saw,
  `receiver_membership_id` and a `reason` (nonblank, control-free, at most
  280 characters). Its instant is the server clock at recording, never a
  device claim (BR-TIME-003). The receiver must be an `ACTIVE` membership of
  the organization other than the current responsible member, whose
  employee record is `ACTIVE` and not merged, who currently holds
  `trip.trip.start` for the trip as its own record and who is assigned
  (FLEET-006) to the trip's asset at the handover instant; an unknown,
  ended, suspended, foreign or otherwise ineligible receiver is
  `VALIDATION_FAILED` with no detail about the person. A closed trip
  (`COMPLETED` or `CANCELLED`) or a revision that is not the current one is
  `CONFLICT`; a trip not stored in the organization is `NOT_FOUND`; a
  body with another field is `MALFORMED`. An accepted handover is one
  insert-only fact (trip, the revision it creates, giving membership and
  employee, receiving membership and employee, recorder account,
  membership and employee, reason, operation and request digest, instant)
  and one `trip.trip.handed_over` audit event carrying the digest and no
  reason text; the identical request returns the stored answer marked
  `replayed`, the same operation with another body is
  `OPERATION_ID_REUSED`. The pure trip (Go `trip/domain`, Kotlin
  `enterprise.trip`) applies a handover with the order scope and trip,
  recorder, receiver and receiving employee (absent `INVALID`, another
  organization `SCOPE`), present reason and instant (`INVALID`), a
  non-positive expected revision (`INVALID`), a missing reason
  (`INCOMPLETE` `REASON`), a terminal trip (`INVALID_TRANSITION`), a stale
  revision (`CONFLICT`), a recorder who is the current responsible member
  or the receiver (`FORBIDDEN`), a receiver who already is responsible
  (`INVALID`), then an instant before the previous handover's (`INVALID`);
  both languages execute the shared `trip/handover-v1.psv` oracle.
- **B-BR-EMP-395 — Future actions use the new responsibility; history keeps
  its actors.** After a handover accepted at instant h: `trip.pause`,
  `trip.resume`, `trip.cancel`, `trip.finish` and `trip.submit` are
  admitted only from the current responsible member (the receiver of the
  latest handover); a logbook event (`trip.event.record`) is admitted only
  from the member responsible at its declared instant (the giving member
  before h, the receiver at or after h), so field facts the giving member
  declared offline before the handover still arrive; any other member is
  `FORBIDDEN` and stores nothing, also on a held path (a late finish of a
  member that is no longer responsible is `MALFORMED`, as for any other
  member). The authority, the history continuity rule (EDR-118), an
  incident report inside the trip, the tracking trip facts and the database
  consult the member responsible at that point and that member's employee
  as the trip's current driver. When a membership ends, its open work
  (`TRIP:<id>` flags, EMP-TRIP-018) is the trips it is currently
  responsible for: a giving member's offboarding no longer needs the
  handed-over trip's flag, the receiver's does. Nothing is rewritten: the start, its readings, every
  stored event, lifecycle row, finish, review entry and acknowledgement
  keep their original actor, instants and receipt; the trip keeps its
  identity, asset, start driver, structure snapshot, reporting day and
  usage rule. A member who was responsible may still re-read the receipts
  of the operations it sent.
- **B-BR-EMP-396 — One revision, one responsible member.** A handover is a
  trip revision: the operational revision is 1 after the start and one
  more per lifecycle command, finish and handover (08-state-model rule 7).
  Of two commands naming the same revision under the organization root and
  trip row locks exactly one applies: a second handover is `CONFLICT`, a
  device lifecycle command is held `STALE_REVISION` (or `FORBIDDEN` when
  its author no longer holds the trip), so at no instant two members are
  responsible. The database re-proves, under the same locks, the open
  state, the next revision, the giving member as the current responsible
  member, the receiver's active membership and employee and the
  separation, and moves no trip column (responsibility is derived from the
  handover facts).
- **B-BR-EMP-397 — Separation.** The recorder is neither the current
  responsible member nor the receiver, by account, membership or canonical
  employee (`SELF_APPROVAL`, nothing written). Every party of every
  handover of a trip — giving member, receiving member and recorder, by
  account, membership and canonical employee — is excluded from every
  review decision of that trip like its author (BR-REV-003), in the
  application and in the review-entry guard. The member currently
  responsible for a trip never closes it for another (EMP-TRIP-014,
  `SELF_APPROVAL`), and a closed trip takes no handover (`CONFLICT`). The
  resolution of a held fact (EMP-TRIP-007) keeps its own author and driver
  separation.
- **B-BR-EMP-398 — Who sees the handover; tracking ownership.** Under
  `trip.trip.close_for_other` the organization's handovers are listed
  (`POST /v1/orgs/{org_id}/trips/handovers/list`, organization level, only
  an ORG grant covers it, keyset by instant and operation). The receiving
  member learns its responsibility through
  `POST /v1/orgs/{org_id}/trips/mine/list` under `trip.trip.start` on its
  own membership: the open trips it is currently responsible for, with the
  asset, state, revision, start instant and, when it was handed over, the
  latest handover's instant, giving member and reason. A tracking session
  belongs to the member who opened it (B-BR-EMP-421): after a handover the
  giving member's session collects nothing and cannot start or resume (the
  pure session refuses an owner who no longer runs the trip); the device
  of the giving member stops collection and ends its session, and the
  receiving member's device opens its own session for the remaining trip,
  which the tracking owner must admit (EDR-256).
- **BUC-EMP-199 — Hand a trip over at a shift change.** A driver started a
  trip at 184 320 000 m; at 13:00 the supervisor, who is not driving,
  hands it over at revision 1 to a second driver assigned to the same
  vehicle with the reason "Troca de turno": the trip stays one trip at
  revision 2, the audit trail names the supervisor and the second driver
  sees it in the own list. A second handover of the same revision, a
  handover to a driver not assigned to the vehicle, to someone outside the
  organization or of a finished trip changes nothing.
- **BUC-EMP-200 — Continue a handed-over trip.** The second driver pauses
  (revision 3), resumes, records stops and finishes at 184 610 000 m; a
  stop the first driver noted offline at 12:40 still arrives under the
  first driver, while the first driver's later pause or stop is
  `FORBIDDEN`. The timeline shows each fact with the member who recorded
  it.
- **BUC-EMP-201 — Review a handed-over trip.** The second driver submits
  the trip; neither driver nor the supervisor who recorded the handover
  can approve, return or reject it (`SELF_APPROVAL`); another reviewer
  decides. Proven on real PostGIS with the races between two handovers and
  between a handover and a pause, in both orders.

## Checklist execution offline with the critical-item policy (2026-11-15)

Evidence.

- **B-BR-EMP-438 — A run is filled offline from the cached template and
  decided on the device.** The app obtains the applicable template of an
  asset and context (version, ordered items, policy revision, mode and
  validity) from the EMP-CHECK-001 applicable read under `fleet.asset.read`
  while online and keeps it sealed in the protected store with the server
  instant that confirmed it; offline is the normal state and shows that age.
  A run is started for one asset, context and cached template; every answer
  (`OK`, `NOT_OK`, `NOT_APPLICABLE`, an optional note of at most 140
  characters) is written to the sealed run before the screen shows it as
  saved, so answers survive closing the app or process death; a failed write
  reports that nothing was saved. Completion is refused locally with the
  shared EMP-CHECK-001 answer rule (`ANSWER_NOT_ALLOWED`, `PHOTO_REQUIRED`,
  `INCOMPLETE`) and when the run is older than the cached policy validity
  (`CHECKLIST_EXPIRED`, a new run is started). The device computes the
  outcome (B-BR-EMP-334) from the cached version and policy revision, names
  both in the run and shows it at once: `BLOCKING` is stated in text and
  icon with the items that block, `COMPLETED_WITH_ISSUES` with critical items
  under `WARN` asks the driver to confirm "Estou ciente das pendências". The
  completed run is one durable `check.run.complete` operation queued in the
  same protected write as the run's local record (BR-OFFL-002); the run is
  attachable by its identifier (the departure gate of a trip start is
  EMP-TRIP-019).
- **B-BR-EMP-439 — The signed run operation.** `check.run.complete` schema 1:
  envelope entity `checklist_run` with the run identity and no expected
  revision, no dependency, no soft reference or attachment (those are
  `RETRY`/`UNAVAILABLE`), `occurred_at` the completion instant, payload
  exactly `asset_id`, `context`, `template_id`, `template_version` (≥ 1),
  `policy_revision` (≥ 1), `started_at` (not after the completion),
  `answers` (1..40 objects of exactly `answer`, `note` — null or 1..140
  nonblank control-free characters — and `photo`, true only with `NOT_OK`),
  `device_outcome` and `acknowledged` (true only with
  `COMPLETED_WITH_ISSUES`). Anything else is a final `MALFORMED`. A `photo`
  declaration is `RETRY`/`UNAVAILABLE` until the run-photo channel exists
  (retained), so a photo is never claimed without its bytes.
- **B-BR-EMP-440 — The server rechecks by its own history.** On receipt the
  server reads the named template version of this organization, its items
  and target, every version's publication instant, the archive instant and
  the whole policy history, and recomputes with one pure rule (Go
  `maintenance/domain/checklist.RecheckRun`, mirrored by the database):
  an unknown version or policy revision, another context, an answer count
  other than the version's items, a version whose target does not cover the
  asset (organization default, the asset, or the asset's category), a start
  after the completion, or an answer set the shared rule refuses is
  `MALFORMED`; the policy is the revision in force at the declared completion
  instant (the first revision when the instant precedes it); the run is
  `TEMPLATE_SUPERSEDED` when a later version of that template was published,
  or the template archived, at or before the completion. The server outcome
  is stored and governs; the device's own claim never does, so a critical
  `NOT_OK` under `BLOCK` declared `COMPLETED_OK` is stored `BLOCKING`.
- **B-BR-EMP-441 — A disagreement keeps the field fact.** A well-formed run
  is always stored and acknowledged `ACCEPTED` at revision 1, with the
  answers, the device outcome and the named policy revision as declared, the
  server outcome, the server policy revision, `template_superseded` and
  `outcome_differs`; a run whose outcomes differ is held for review
  (`review_required`) and never dropped or rewritten. The author is told by
  reading the run: `POST /v1/orgs/{org_id}/checklists/assets/{asset_id}/runs/{run_id}/read`
  under `fleet.asset.read` on the asset exactly as the applicable read (its
  allocation for structural grants; an assigned driver through the
  assignment; a run of another asset is not found) returns both
  outcomes, the flags and the positions of the issues — never a note, a
  photo or the author's identity. A run whose asset is another
  organization's, unknown or not covered is not found; a `DRAFT` asset is
  `MALFORMED`; changed rights store nothing (`RETRY`/`UNAVAILABLE`); the same
  operation returns the stored acknowledgement byte for byte and another
  digest is `OPERATION_ID_REUSED`; a second operation for a stored run
  identity is `MALFORMED`. The intake authority is `check.checklist.execute`
  with the EDR-177 history rule (S: own records and assigned assets).
- **B-BR-EMP-442 — A blocking run blocks the asset until another member
  releases it.** From the declared completion instant a run stored
  `BLOCKING` makes the asset `BLOCKED` for every member (FLEET-007 §6.2 row
  4, new cause kind `CHECKLIST_RUN` with the run as record and its period; a
  new start is held `ASSET_UNAVAILABLE` by the existing start check; the
  `CHECKLIST_BLOCKING` gate code is EMP-TRIP-019). `POST
  /v1/orgs/{org_id}/checklists/runs/{run_id}/release` under fresh
  `check.incident.manage` covering the run's stored asset, unit or cost
  centre takes an operation identifier, the expected revision and a reason
  (1..500): a `BLOCKING` run at that revision becomes `COMPLETED_WITH_ISSUES`
  at revision 2, the block ends at the release instant, one audit event
  `check.run.released` is written and the answers, device outcome and
  recheck outcome never change. The author of the run (same account,
  membership or canonical employee record) is refused `SELF_APPROVAL`; a run
  that is not `BLOCKING` or a stale revision is `CONFLICT`; a missing or
  blank reason is `VALIDATION_FAILED`; replay returns the first answer and
  another intent is `OPERATION_ID_REUSED`. Migration 000242 stores runs,
  answers and releases with forced RLS, same-organization keys to the
  template version, items and policy revisions, insert-only answers and
  releases, a database recheck of the outcome, the flags and the in-force
  policy, publication only with the `ACCEPTED` receipt or the release audit
  event, and the asset, unit and cost-centre usage sources redefined from
  their latest bodies with the run keys as associations.
- **B-BR-EMP-443 — The execution screens.** An enterprise "Checklists" entry
  and the trip start form open the execution screen for an asset: one item
  per row with large targets, critical marks, the three answers and the note,
  progress, the cached template's confirmation age, and "Nenhum checklist
  exigido para este veículo" when none applies. The outcome screen shows the
  local outcome, the blocking items and that the company rechecks it; after
  delivery it shows the server outcome and, when it differs, "A empresa
  recalculou o resultado" with both outcomes. The trip start form shows the
  latest valid departure run of the chosen vehicle and disables the start
  while it is `BLOCKING` (the device effect of BR-CHECK-006). Items that
  require a photo when not OK cannot be completed `NOT_OK` until the
  run-photo channel exists, and the screen says so. Strings exist in the
  eight app locales; the visual direction is the app's (black solid header
  and bars, white containers, light by default, no grey fills).
- **BUC-EMP-222 — Fill a departure checklist with no connection.** A driver
  opens the cached template of a vehicle (BLOCK, item 2 "Freios" critical),
  answers item 1 OK, closes the app, reopens it and finds the answer, answers
  "Freios" NOT_OK with a note and completes: the screen shows "Bloqueando o
  uso" with "Freios" as the blocking item and the trip start of that vehicle
  is disabled; the run waits in the queue.
- **BUC-EMP-223 — The company rechecks the run.** On reconnection the run is
  accepted; the server stores `BLOCKING` and the vehicle is `BLOCKED` for a
  second member. A run computed on a cached `WARN` revision while the
  server's policy at that time was `BLOCK` is stored `BLOCKING` with the
  device's `COMPLETED_WITH_ISSUES`, held for review, and the author's screen
  shows both outcomes; nothing was refused or rewritten.
- **BUC-EMP-224 — Release a blocked vehicle.** A fleet manager of the
  vehicle's unit, not the author, releases the run with the reason "Freios
  verificados na oficina"; the vehicle is available again, the run reads
  `COMPLETED_WITH_ISSUES` at revision 2 with its answers unchanged, and the
  audit trail has the release. The author trying the same is refused
  `SELF_APPROVAL`.

## EMP-GPS-002 prerequisite — Tracking policy command (2026-11-15)

Evidence. Coordinator-assigned
prerequisite of the EMP-GPS-002 activation: since EMP-GPS-004 nothing wrote
`org_tracking_policies`, so no organization could turn tracking on.

- **B-BR-EMP-467 — Only a fresh organization administrator changes the
  tracking policy.** The company route
  `POST /v1/orgs/{org_id}/tracking/policy/change` requires the current
  company proof with fresh authentication, `org.settings.manage` covering the
  organization itself (OWNER and ADMIN; the catalog key that the tracking
  specification names for the tracking policy), the organizations capability
  and an `ACTIVE` organization. The body names `schema_version` 1, an
  `operation_id`, the expected current `revision` (absent only while the
  organization has no recorded version), the `policy` (`OFF`, `OPTIONAL`,
  `EXPECTED`) and, for `OPTIONAL` and `EXPECTED`, the employee notice
  (`notice_version`, `notice_in_force_at`). One accepted change writes, in one
  transaction, the next insert-only policy version (revision + 1, effective at
  the database clock), one immutable command receipt (actor account and
  membership, request digest, resulting revision) and one audit event
  `gps.policy.changed`. Another role (including `AUDITOR`), a member without
  coverage and a stale or premature revision change nothing: `FORBIDDEN`
  naming `org.settings.manage`, `CONFLICT`. Another organization, an unknown
  organization and an account without a current membership are `NOT_FOUND`.
  The same operation and digest returns the first result with
  `replayed: true` and no second version or audit event; the same operation
  with another body is `OPERATION_ID_REUSED`.
- **B-BR-EMP-468 — Tracking is enabled only with the employee notice in
  force.** `OPTIONAL` and `EXPECTED` require a notice version
  (`^[a-z0-9][a-z0-9._-]{0,63}$`) and the instant it entered into force,
  which must be at or before the effective instant the database assigns to
  the new version; a missing notice, a malformed version, a notice that is
  not yet in force and any notice field sent with `OFF` are
  `VALIDATION_FAILED` naming the field with a frozen code and write nothing.
  The platform stores only the version identifier and instant that the
  organization declares for its own employee notice (EDR-271); the notice
  text is the organization's publication and is shown to the driver by
  EMP-GPS-002/015, never stored or invented here. A new version governs new
  sessions only; existing sessions keep the policy revision they opened under
  and their stored data (BR-GPS-006). Policy `OFF` carries no notice.
- **B-BR-EMP-469 — The policy read states the stored and the effective
  policy.** `POST /v1/orgs/{org_id}/tracking/policy/read` requires
  `org.settings.read` covering the organization (OWNER, ADMIN, FLEET_MANAGER,
  SUPERVISOR, FINANCE, APPROVER, AUDITOR) and answers the current revision
  (`0` with `OFF` and no notice when nothing was recorded), the stored policy,
  notice version and in-force instant, the version's effective instant and the
  policy in force now (the EMP-GPS-004 rule: `OFF` until a notice is in
  force). `DRIVER` and the other roles are `FORBIDDEN` naming
  `org.settings.read`. The read carries no session, member or location data.
- **BUC-EMP-237 — An administrator turns tracking on.** After publishing the
  employee notice, an owner or administrator reads the current policy, sets
  `EXPECTED` with the notice version and its in-force instant on the read
  revision, and from then on drivers' trips open tracking sessions under that
  version; turning it `OFF` later stops new sessions and keeps what was stored.
- **BUC-EMP-238 — The policy cannot be changed around its owners.** A
  supervisor, a fleet manager, an auditor, a driver, another organization, a
  stale revision, two administrators racing on one revision (one wins, the
  other is `CONFLICT`) and a notice not yet in force all leave the policy
  unchanged.

## Track processing: filtering, gaps, GPS distance and simplified geometry (2026-11-15)

Evidence.

- **B-BR-EMP-461 — An ended session is processed by one fenced job.** When
  a session's `ENDED` transition is stored, or a segment is stored for a
  session that already ended, the tracking store queues one
  `org-tracking-process` job in the same transaction; its payload names only
  the organization and the session, and its de-duplication key names the
  input (the session's stored segment and transition counts), so a resend
  converges and a new input is a new job. The job runs in one
  organization-scoped transaction that first holds its own queue row under
  its lease token (a superseded lease writes nothing) and then the trip row
  (the lock of the intake and of the trip finish). A session not stored in
  the job's organization, and a session not ended yet, complete with no
  effect.
- **B-BR-EMP-462 — Fixes are filtered, counted and never edited.** In
  instant order, a stored fix outside the trip's running time or the
  session's recording periods, or whose coarse flag disagrees with its
  period, is counted `OUTSIDE`; a precise fix whose accuracy is above 50 m is
  counted `ACCURACY`; a precise fix whose speed implied from the previous
  accepted precise fix is above 70 m/s is counted `SPEED`; a coarse fix is
  accepted without those checks and is never measured (BR-GPS-007/017). The
  device's own dropped counters (accuracy, stale, jump, mock) and the intake
  counters (duplicates, outside the session, mock) of the session's segments
  are summed into the summary. Raw fixes are never edited, reordered in
  storage or deleted by processing (BR-GPS-023).
- **B-BR-EMP-463 — Gaps are data with a reason and a detector.** Inside the
  trip's `IN_PROGRESS` time — from its start until its finish, cancellation
  or supervisor closure, pauses excluded, and until the session's end when the
  trip has none stored — every interval without recording is a gap whose
  reason is the session's cause (before the session opened `NOT_STARTED`;
  after an early end its end cause), detected by the device when the driver
  or the device reported it and by the server otherwise; inside a recording
  period any interval longer than 180 s without an accepted fix (from the
  period's start, between accepted fixes, until its end) is a `SIGNAL_LOST`
  gap detected by the server. Gaps are ordered, never overlap, merge only
  with an adjacent gap of the same reason and detector, and store start, end,
  reason and detector (BR-GPS-024). Expected seconds are the whole seconds of
  that time (floor), gap seconds the ceiling of the gaps' length bounded by
  it, and coverage and quality are the EMP-GPS-001 classification (any
  approximate recording period caps it at `APPROXIMATE`, BR-GPS-026).
- **B-BR-EMP-464 — GPS distance is measured inside runs only.** When the
  quality is not `INSUFFICIENT` and a precise run holds two accepted fixes,
  the GPS distance is the sum, on a sphere of 6 371 008.8 m, of the
  great-circle lengths between consecutive accepted precise fixes of one run
  (one precise recording period split at every `SIGNAL_LOST` gap), a step
  counted only once it reaches 10 m from the last counted fix, floored to
  whole metres, with method `haversine-r6371008.8-step10-v1`
  (BR-GPS-025). Nothing is added across a gap or from a coarse period; when no
  precise run can be measured, or the quality is `INSUFFICIENT`, the distance
  is absent, never zero.
- **B-BR-EMP-465 — One insert-only summary revision records its basis.**
  Each processed input stores one summary revision (method `gps-track-v1`)
  with the input it names, the job and lease that wrote it, the database
  processing instant, expected and gap seconds, coverage, quality, the filter
  and intake counters, the count of `CLOCK_UNCERTAIN` segments (no boot
  identity, or a wall base more than 2 s away from its boot's monotonic base;
  instants are never rewritten), the gaps, the distance with its method and
  the simplified geometry with its tolerance: every precise run of at least
  two accepted fixes simplified with Douglas–Peucker at 20 m on a local
  projection, the tolerance doubled until at most 1 000 vertices remain, kept
  as integer arrays and a database-derived `MULTILINESTRING` of SRID 4326 whose
  parts break at every gap; no geometry when the quality is `INSUFFICIENT`, no
  run has two fixes or the run end points alone exceed 1 000 vertices. No
  coordinate appears in the job payload, a log or an error.
- **B-BR-EMP-466 — Retries converge and a stale job never overwrites.** The
  database stores a summary only for an ended session, only when it names
  exactly the stored input (segment and transition counts) under the trip
  lock, only when that input is strictly newer than the current revision's
  and at the next revision, and only when its fix, intake and device totals
  are the session's own; a summary of an older or equal input, of another
  session's fixes or of another organization is refused, and summaries are
  never updated or deleted. A job whose input is already summarized, a job
  run twice, two concurrent runs and a retried job converge on one revision
  per input; a late batch makes a newer input whose revision supersedes the
  previous one (BR-TRUTH-004), which stays stored.
- **BUC-EMP-234 — A finished trip's track is summarized.** The driver
  finishes the trip and the device ends the session; the job filters the
  stored fixes, records the gaps, measures the runs, simplifies the line and
  stores the summary with its quality, coverage and methods.
- **BUC-EMP-235 — A late batch supersedes the summary.** Fixes collected
  before the end arrive later; a new job writes the next revision over the
  complete input and the earlier revision stays as history.
- **BUC-EMP-236 — Retries, duplicates and races converge.** A redelivered
  job, a job run twice, two workers on one session, a superseded lease and a
  foreign organization's job leave one revision per input and nothing else.

## Panel: mission planning and assignment board (2026-11-15)

Evidence.

- **B-BR-EMP-470 — Eligible assignees of a mission are a server read.** `POST
  /v1/orgs/{org_id}/missions/{mission_id}/assignees/list` lists, under current
  `trip.mission.manage` covering the mission (read, not fresh; structural scope
  as the mission read: another organization's or an unknown mission
  `NOT_FOUND`, a planner whose grant does not cover it, `AUDITOR` and `DRIVER`
  `FORBIDDEN`), exactly the employees the assignment command would accept: an
  `ACTIVE`, unmerged employee of the organization with an `ACTIVE`
  account-linked membership whose current `trip.trip.start` covers the mission
  (the EMP-TRIP-008 `EligibleAssigneeTx` rule, evaluated per candidate inside
  the read transaction). Each item carries only `employee_id`, `display_name`
  and `registration_number` (empty strings when the profile has none); no
  contact, membership, role, scope or account data. Pages scan candidates in
  employee order and return the eligible ones; the opaque cursor is bound to
  the route, mission and limit and advances by the last scanned employee, so a
  page can be empty while a cursor remains. The read changes nothing and is
  answered `no-store` like every private company read. Eligibility shown here
  is never authority: the assignment command re-decides it.
- **B-BR-EMP-471 — The board reports server state only.** The company panel
  board (SCR-TRIP-03) shows a mission's state, revision, vehicle, assignee,
  answer and plan only as returned by the latest successful mission list or
  read, with the instant it was read (BR-TRUTH-006). A pending command never
  changes the shown mission; an acknowledged command is followed by a new read
  of that mission and only that read replaces it. `ACCEPTED` and `DECLINED`
  appear only when a server read says so; an assignment acknowledgement must
  say `ASSIGNED` for the requested employee, otherwise it is unusable. The
  FLEET-007 assignment warning (`ASSET_UNAVAILABLE`/`ASSET_BUSY` with the
  availability) is shown as returned and is never a refusal.
- **B-BR-EMP-472 — Only eligible scoped choices are offered.** Vehicles offered
  for creation and vehicle change are the planner's server-listed `ACTIVE`
  assets that are not `IMPLEMENT` (never the mission's current vehicle for a
  change); assignees offered are only those of B-BR-EMP-470 for that mission
  (never the current assignee again). A choice that is not in the current
  server list is refused before any request. Cost centres are offered only
  from the server's `ACTIVE` cost-centre list when the member can read it;
  otherwise the plan is saved without one. The vehicle's agenda (SCR-FLEET-06
  list view) shows its FLEET-007 availability with causes and evaluation
  instant, and its reservations and blocks by state and window; free-text
  purposes and notes are not displayed. A displayed option or control is not
  authorization: the server decides every read and command.
- **B-BR-EMP-473 — Commands keep their contracts.** Create, plan, assign,
  vehicle change and cancel use the existing routes with schema 1, one
  immutable operation id and body per decision, and the mission revision the
  planner saw. An unknown outcome may be retried only with the same id and
  body; a new decision gets a new id. `CONFLICT` (stale revision, a state that
  no longer allows the action, a busy vehicle) clears the pending decision,
  never retries, and re-reads the mission so the current server state is
  shown with the conflict. `FORBIDDEN` and `NOT_FOUND` clear every private
  board record and show fixed copy that reveals nothing about the target;
  `FRESH_AUTH_REQUIRED` and `UNAUTHENTICATED` end the company session view;
  `VALIDATION_FAILED` keeps the planner's input.
- **B-BR-EMP-474 — Isolation, freshness and privacy of the board.** Changing
  organization, losing authority, signing out or reloading clears the board
  and aborts its requests; a late response of an earlier scope or decision is
  discarded. Nothing is kept in browser storage; private responses must be
  `no-store`; the board displays no personal data beyond the fields the routes
  return and never sends a value it did not receive from the server as a
  reference.
- **BUC-EMP-239 — Plan and hand over a mission from the panel.** A fleet
  manager opens the board, creates "Entrega sintética no cliente" with FR-001
  for tomorrow 08:00–10:00, plans destination text, reason, cost centre and
  cargo, sees FR-001's agenda (a confirmed reservation the same morning),
  assigns Bruno from the eligible list and reads the server's
  `ASSET_UNAVAILABLE` warning; the mission shows `ASSIGNED` (awaiting answer)
  until a later read shows Bruno's `ACCEPTED`.
- **BUC-EMP-240 — A concurrent change is never hidden.** Two planners open the
  same mission at revision 3; the first reassigns it; the second's cancel gets
  `CONFLICT`, the board re-reads and shows revision 4 assigned to the other
  member with the conflict message, and the second planner decides again.
- **BUC-EMP-241 — Prove the board and its read.** Real PostGIS proves the
  eligible-assignee read (FINANCE, AUDITOR, suspended, inactive, merged and
  account-less candidates excluded; unit-scoped coverage; empty pages with a
  cursor; another organization and out-of-scope planners refused; the read
  changes nothing) and its composed HTTP route; the web tests prove a foreign
  or stale assignment is not offered, a conflict refreshes server state, an
  unconfirmed state is never shown as accepted and a `403` clears the board.

## Reservation-to-trip integration and no-show release (2026-11-15)

Evidence.

- **B-BR-EMP-412 — A trip start may name the mission it executes.** A
  signed `trip.start` keeps operation schema 1 and its exact payload; it may
  carry in its envelope `soft_references` exactly one reference
  `{type: "mission", id}` (BR-OFFL-007). No reference keeps the earlier
  behaviour; another reference type or more than one reference stays a
  recognised but unsupported form (`RETRY`/`UNAVAILABLE`, nothing stored).
  The reference is decided at receipt, under the organization root and
  asset locks and after the existing final refusals (`MALFORMED`,
  `FORBIDDEN`, `DEPENDENCY_PENDING`): it is eligible only when the mission
  belongs to the organization, is `ACCEPTED`, its current assignee is the
  start's author membership and driver employee, its current vehicle (when
  it names one) is the start's asset, and no trip is linked to it yet. An
  eligible reference admits the start inside a confirmed reservation of that
  mission (BR-FLEET-031, as a reservation naming the starter does). The
  existing held codes keep their order (`PERMISSION_CHANGED`,
  `ASSET_UNAVAILABLE`, `ASSET_BUSY`); a start that would otherwise be
  accepted with a missing, foreign or ineligible mission is held
  `ORPHAN_REFERENCE` (`ACCEPTED_FOR_REVIEW`, no trip, no link). A held start
  never links: when the EMP-TRIP-007 resolution accepts it, the trip it
  declared is created without the mission (the link removed) and consumes no
  reservation. An accepted start with an eligible mission stores, in the same
  transaction as the trip, one insert-only link (one trip, one mission, the
  start operation); the database refuses a link whose mission is not
  `ACCEPTED` for the trip's author, driver and asset, a second link of the
  trip or of the mission, and any change or deletion of a link. Replays
  return the stored acknowledgement byte for byte.
- **B-BR-EMP-413 — The mission follows its linked trip.** The shared mission
  table (Go and Kotlin, `trip/mission-v3.psv` superseding v2) gains
  `START` (`ACCEPTED` → `IN_PROGRESS`) and `COMPLETE` (`IN_PROGRESS` →
  `COMPLETED`); `CANCEL` is also allowed from `IN_PROGRESS`; every other
  command on an `IN_PROGRESS` mission, and `START` or `COMPLETE` from any
  other open state, is `INVALID_TRANSITION` (the mission table no longer
  answers `UNSUPPORTED_COMMAND`). No company or device command produces
  `START` or `COMPLETE`: the linked start appends the mission's `START`
  transition (source `TRIP`: the trip, its start operation, the author
  membership and account) and moves the mission to `IN_PROGRESS` at the next
  revision in the start's transaction; the transaction that completes the
  linked trip — the driver's finish or a supervisor closure (EMP-TRIP-014) —
  appends `COMPLETE` (source `TRIP`: the finish or closure operation and its
  member) and moves the mission to `COMPLETED`. A cancelled trip leaves the
  mission `IN_PROGRESS`; its planner may cancel it with a reason, and a
  later completion of a trip whose mission was cancelled changes nothing.
  A planner command other than `CANCEL` on an `IN_PROGRESS` mission is
  `CONFLICT`; the vehicle change stays refused.
- **B-BR-EMP-414 — A mission's reservation is consumed once by its linked
  start or released once.** A planner holding fresh `trip.mission.manage`
  over the mission's structure requests the reservation of the mission's
  vehicle: `POST /v1/orgs/{org_id}/missions/{mission_id}/reservations/request`
  with `schema_version` 1, `operation_id`, the mission's `revision` and a
  `purpose` (nonblank, control-free, at most 280 characters). The mission
  must be `PLANNED`, `ASSIGNED`, `ACCEPTED` or `DECLINED` and name a
  vehicle (else `VALIDATION_FAILED`); `DRAFT`, `IN_PROGRESS`, terminal or a
  stale revision is `CONFLICT`. One `REQUESTED` reservation of that vehicle
  for the mission's planned window is created, linked to the mission, with
  no reserved employee and the 60-minute no-show tolerance, after settling
  the vehicle's due outcomes; a window that has started is
  `VALIDATION_FAILED`, an overlap with a confirmed window or a block is
  `RESERVATION_CONFLICT` showing only that window. The mission gains a
  `RESERVE` revision keeping its state, assignee and answer; one
  `trip.mission.reservation_requested` audit event, the mission command and
  the fleet `REQUEST` receipts are written and the outcome job is queued.
  Confirmation and cancellation stay under `fleet.reservation.manage`. In
  the linked start's transaction, the mission's reservation on the trip's
  asset that is stored `CONFIRMED`, not yet due at the receipt instant
  (before its no-show deadline) and whose window extended by the 30-minute
  early tolerance (`start − 30 min, end)`) contains the declared start
  becomes `FULFILLED` with that trip (the earliest such window); it is an
  effect of the start operation, covered by that operation's receipt and
  audit event, with no event of its own. The FLEET-007 outcome job,
  which now takes the organization root lock before the reservation, records
  `NO_SHOW` once at the deadline; a `FULFILLED` reservation is never missed
  and a `NO_SHOW`, `EXPIRED` or `CANCELLED` one is never consumed (the start
  is decided without it). The database refuses `FULFILLED` from any state
  but `CONFIRMED`, at or after the deadline, without the linked trip of the
  reservation's mission on its asset with a declared start inside the
  extended window, or for a second trip.
- **BUC-EMP-209 — Execute an accepted mission offline.** A planner creates
  a mission for the synthetic vehicle tomorrow 08:00–12:00, assigns the
  driver, who accepts on the phone, and reserves the vehicle; a fleet
  manager confirms. At 07:50 the driver, offline, starts the trip naming
  the mission; on reconnect the start is accepted, the mission is
  `IN_PROGRESS` and the reservation `FULFILLED` with the trip; another
  driver's start on the vehicle at 09:00 is held `ASSET_BUSY`. The finish at
  184 610 000 m completes the trip and the mission. A start naming a
  cancelled mission, another driver's mission or a mission of another
  organization is held `ORPHAN_REFERENCE`; nothing is linked.
- **BUC-EMP-210 — Release a no-show.** The driver never starts: at 09:00
  (start plus the 60-minute tolerance) the outcome job records `NO_SHOW`
  once and the vehicle is free for others from then on; running the job
  again changes nothing. A start the driver declared at 08:30 and delivered
  at 09:05 is decided normally and is not linked to the released
  reservation, whose `NO_SHOW` stays; a start delivered at 08:59 while the
  job waits behind it consumes the reservation, and the job then finds it
  `FULFILLED` and changes nothing.

## App: capture and import with offline queue and legibility check (2026-11-15)

[Evidence.

- **B-BR-EMP-450 — Two ways to bring an original, both without a new
  dependency.** Beside a record already saved in the partition's protected
  queue whose evidence has an EMP-DOC-003 link target (C1: the departure or
  final reading of a trip, target `METER_READING`; later owners add their own
  targets), the app offers "Fotografar" — the system camera writing into a
  private cache file of the app, only after the `CAMERA` permission is
  granted — and "Importar foto" — the Android photo picker, images only,
  which needs no storage permission. A denied camera permission, a cancelled
  camera or picker, or an empty selection saves nothing, says so, and keeps
  the other way available. Capture is shown only when the attachment queue is
  enabled (`ENTERPRISE_ATTACHMENT_UPLOAD_ENABLED`) and the rollback switch
  `ENTERPRISE_RECEIPT_CAPTURE_UI_ENABLED` (default on) is on; switching it off
  hides the entry points and keeps every saved attachment and its delivery.
- **B-BR-EMP-451 — The type is decided by content and the original is kept
  unmodified.** The selected file is read once, bounded to 5 MiB plus one
  byte, and judged by its bytes, never by its name or declared type: a JPEG
  (start-of-image and end-of-image markers) of up to 5 MiB is accepted
  exactly as read — no re-encoding, cropping, resizing or metadata change;
  no readable bytes (file gone, access revoked, empty) is "file not found on
  the device, choose again"; more than 5 MiB is "too large"; anything else
  (PNG, HEIC, WebP, PDF, a renamed file) is "unsupported in this phase". A
  refused file is never queued, listed or shown as saved. The camera's
  temporary file is deleted as soon as its bytes are read, and any leftover
  is deleted when capture opens again.
- **B-BR-EMP-452 — Legibility is a hint, never a validation.** Before saving,
  the device decodes a downscaled luminance sample of the accepted original
  and reports either that the image looks sharp or the problems it sees (low
  resolution, too dark, too bright, low contrast, blurred), always with the
  statement that the check does not validate the receipt. It never blocks
  saving, never claims fiscal, legal or document validity, performs no text
  recognition, and its result is neither stored nor sent. An image that
  cannot be decoded is "could not be checked" and may still be saved; the
  person keeps the photo or takes another one.
- **B-BR-EMP-453 — Saved means sealed in the creation partition.** "Salvar
  no aparelho" hands the exact original to the EMP-OFFL-005 capture of the
  record's own partition (account, origin, organization, membership,
  enrollment) with the target fixed; "Salvo no aparelho" appears only after
  that protected write returns. A failed write says that nothing was saved
  and keeps the unsaved photo in memory for another attempt; a local refusal
  (record not in this partition, index full, byte budget, another
  enrollment, capture disabled) is shown with its reason. An unsaved photo
  lives only in memory and is gone after a context change or a process death;
  only sealed originals survive, and nothing else of the photo is written.
- **B-BR-EMP-454 — Each attachment shows its own state.** Under its record,
  each attachment is shown "Salvo no aparelho", "Enviando", "Arquivo em
  verificação", "Arquivo guardado pela empresa" (only after the server
  reported it `STORED`; it says the file arrived intact, never that the
  receipt is valid), "Não aceito" with its reason, "Comprovante não encontrado
  no aparelho" with the offer to photograph it again, or "Precisa de atenção"
  after repeated failures — always independent of the record's own sync
  state, which never waits for or changes with its attachments. Every capture
  screen states that the C1 phase takes synthetic images only.
- **BUC-EMP-228 — Photograph a reading offline and send it later.** Bruno
  starts a trip with no signal, opens the start record, takes a photo of the
  odometer, sees "A imagem parece nítida — esta conferência não valida o
  comprovante", saves it ("Salvo no aparelho"), the phone restarts, the photo
  is still saved, and after reconnection the start is "Aceito pela empresa"
  while the photo moves to "Arquivo guardado pela empresa" only when the
  server stored it; an interrupted upload stays pending and is retried.
- **BUC-EMP-229 — Import, refused types and hints.** Bruno imports a PNG
  screenshot: "Formato não aceito nesta fase", nothing saved; he imports a
  dark JPEG: "A imagem pode estar escura", he keeps it anyway and it is
  saved unmodified.
- **BUC-EMP-230 — Negatives.** Camera permission denied (import still
  offered, nothing saved), a picked file that disappeared before reading
  ("não encontrado no aparelho"), a protected write that fails ("Nada foi
  salvo"), a record of another organization or enrollment (refused, never
  listed in the other partition), a sealed original missing at delivery
  (shown "Comprovante não encontrado no aparelho", never sent, never
  verified).

## App: add a fueling offline with receipt and meter reading (2026-11-15)

Evidence.

- **B-BR-EMP-455 — The device builds the exact `fueling.submit` the intake
  reads.** The app produces only schema 1 of `fueling.submit` (EDR-175):
  entity `fueling` with a new device identifier and no expected revision, no
  dependency, soft reference or envelope attachment, and the exact 17-key
  payload — the chosen asset, the member's own employee record as
  responsible, the product and its unit, quantity in milli-units, paid total
  and optional discount in centavos, optional unit price in milli-BRL, fill,
  payer, payment method, exactly one of a reading (meter, segment, a new
  reading identifier, kind and base value) or the reason for its absence, an
  optional typed `NOT_FOUND` place name, and `trip_id`,
  `contracted_supplier_id` and `unit_price_text` null. The device decoder
  refuses, in the intake's order and class, everything the Go parser refuses
  (`MALFORMED`) or keeps unavailable (`UNSUPPORTED`: trip, contracted
  supplier, catalog station, organization supplier, internal tank); the
  registered producer admits only an operation the decoder reads back, inside
  the protected write. One shared oracle pins the bytes for both languages.
- **B-BR-EMP-456 — A fueling is saved durably in its creation partition.**
  "Salvar abastecimento" queues the operation through the EMP-OFFL-003
  protected write of the selected organization's partition (account, origin,
  organization, membership, enrollment); "Salvo no aparelho" appears only
  after that write returns; a failed write says that nothing was saved and
  keeps the form. Nothing else of the fueling is stored: the list and detail
  are read back from the partition's retained `fueling.submit` entries, so
  the original scope and every entered amount survive restart exactly as
  queued. Another organization or account never lists or sends it.
- **B-BR-EMP-457 — Entry is exact and offline.** The asset comes from the
  EMP-FLEET-013 picker (implements excluded); the products offered are the
  cached asset's accepted products (all nine when the cache lists none, the
  server then warns `PRODUCT_NOT_ACCEPTED`); quantities and unit prices take
  up to three decimals and money two, with a decimal comma or point and no
  grouping, converted to integers without rounding, anything else refused
  before saving ("confira os valores"); the paid total is never recomputed.
  Fill `FULL`/`PARTIAL`/`UNKNOWN`, payer `COMPANY`/`EMPLOYEE` and the offline
  payment methods (fuel card, credit, debit, Pix, cash, supplier account,
  other — an agreement voucher is online-only and internal stock belongs to
  F6) are explicit choices. A member without an employee record in the
  selected context is told to confirm the company again; nothing is saved.
- **B-BR-EMP-458 — Warnings inform; acceptance follows the server.** The form
  shows the BR-MONEY-005 arithmetic warning when the entered price, quantity,
  discount and paid total differ by more than one centavo, and the METER-003
  warning when the reading is below or implies an implausible rate against
  the latest reading of the same meter and segment among this device's own
  fuelings; neither blocks saving. The fueling shows "Salvo no aparelho"
  until its own final receipt: "Aceito pela empresa" with the review line
  "Aguardando aprovação" only from an `ACCEPTED` receipt, "Precisa de
  revisão" or "Não aceito" with the reason otherwise; a held fueling (changed
  rights, after disposal) stays saved and retried (EDR-177). Nothing is shown
  as accepted before its acknowledgement.
- **B-BR-EMP-459 — The receipt is a separate, optional photo linked to the
  fueling.** In the form, "Comprovante (opcional)" uses the EMP-DOC-004 sheet
  (camera or import, legibility hint, keep or retake, synthetic images only);
  "Sem comprovante agora" is simply not taking one. After the fueling is
  saved, its photo is sealed in a second protected write with target
  `FUELING` (the fueling identifier) and owner the `fueling.submit` operation;
  if that second write fails or is refused the fueling stays saved and the
  screen says the receipt was not saved, keeping the photo for another
  attempt; a receipt can also be added later from the fueling's detail. The
  photo is declared only after the fueling's final receipt (EDR-268/269): it
  links only to a stored fueling, through the new `FUELING` link kind of
  EMP-DOC-003 resolved by the fueling owner under `fuel.fueling.record`
  (the author's own fueling; another member's is `FORBIDDEN`, another
  organization's or a missing one `NOT_FOUND`); its progress is shown
  separately and never changes the fueling's state.
- **B-BR-EMP-460 — Rollback and scope.** `ENTERPRISE_FUELING_UI_ENABLED`
  (default on) hides the entry; queued fuelings and receipts keep their
  delivery. No personal-ledger record is read, copied or converted (BR-FUEL-
  024); no real receipt is captured before OQ-04 is decided; no trip, place
  reference other than a typed name, contracted supplier or envelope
  attachment is produced.
- **BUC-EMP-231 — Fuel offline with a receipt.** Bruno, with no signal,
  opens "Abastecimentos", chooses his truck, enters 150 L of Diesel S10
  for R$ 970,00 at R$ 6,490/L (the arithmetic warning appears; he keeps the
  paid total), full tank, company fuel card, odometer 184 600 km, photographs
  the synthetic receipt ("A imagem parece nítida"), saves: "Abastecimento
  salvo no aparelho com o comprovante". The phone restarts; both are still
  saved. On reconnection the fueling becomes "Aceito pela empresa —
  aguardando aprovação"; the first upload of the photo is interrupted and
  shows "Enviando" while the fueling stays accepted; the retry stores it
  ("Arquivo guardado pela empresa").
- **BUC-EMP-232 — Without reading, without receipt, later receipt.** The
  odometer is broken: Bruno states why; he skips the photo and adds it the
  next morning from the fueling's detail.
- **BUC-EMP-233 — Negatives.** A failed protected write (nothing saved), a
  fueling of organization A never listed or sent under B, a member without an
  employee record, a value with too many decimals, a receipt refused locally
  (fueling kept, receipt not saved), an attempt to link a receipt to another
  member's or another organization's fueling on the server, and the shared
  oracle rows that the intake refuses.

## Panel: checklists, damages and incidents (2026-11-16)

Evidence.

- **B-BR-EMP-506 — The organization's checklist runs are a server queue.**
  `POST /v1/orgs/{org_id}/checklists/runs/list` lists, under current
  `check.incident.manage` on the whole organization (read, not fresh; exactly
  the incident queue's policy: an `ORG` grant covers it, a `UNIT`,
  `COST_CENTRE`, `ASSET` or `SELF` grant does not, and `AUDITOR`, `DRIVER`,
  `FINANCE` and `APPROVER` are `FORBIDDEN` naming the key), every run of the
  organization with exactly the run read's projection (both outcomes, the
  policy revisions, the flags, the issue and critical-issue positions, the
  revision and the release instant; never a note, a photo, the reason or the
  author's identity). Runs whose current outcome is `BLOCKING` come first,
  then runs whose device and server outcomes differ (`review_required`), then
  every other run; inside each group the newest declared completion comes
  first, then the run identifier. The opaque cursor is bound to the caller,
  the route and the limit (1..100). The read changes nothing; another
  organization's runs are never listed.
- **B-BR-EMP-507 — Document content is offered only with its own
  permission.** `contexts/read` adds the hint `doc.document.view_content`
  when a current `ORG`-scope assignment of the member covers it on the
  organization (a read hint, independent of proof freshness and of the
  organization being writable); a `UNIT` or `SELF` grant, an expired
  assignment or a role without the key gives no hint. The hint is never
  authority. The panel offers the content action of an attachment only with
  that hint and only while the attachment is `STORED`; otherwise the row says
  that a photo exists and shows its storage state. The action runs only on
  the member's explicit request and only through the audited DOC-002/PRIV-005
  path (`content/access`, then `content/read` with the returned capability,
  which stays in memory for that one exchange); the bytes are handed to the
  browser as a file the member saves and are never kept in panel state, the
  page or browser storage. A `403` hides the action for the rest of the view.
- **B-BR-EMP-508 — The asset block shown is the server's.** For an opened run
  or report the panel reads the asset's FLEET-007 availability (availability,
  every cause with its kind and record, missing sources, evaluation instant)
  and shows it as read, naming a `CHECKLIST_RUN` cause that is the opened run.
  It reads it again after every acknowledged release or handling step. The
  block is never inferred from a run's outcome or from a report's mark: if
  the availability read fails, the block is shown as not confirmed, never as
  available. A report's `blocks_use` is shown as the reporter's declaration;
  the asset block from reports is not applied by the server yet.
- **B-BR-EMP-509 — Release and handling keep their contracts and history.**
  The release of a `BLOCKING` run (fresh `check.incident.manage`, the run
  revision read, a reason of 1..500 scalars) and the handling steps of a
  report (`ACKNOWLEDGE` without a note; `TREAT` with the stated action;
  `RESOLVE` with the resolution note; `DISMISS` with the reason, optionally as
  a duplicate of another listed report of the same asset) use the existing
  routes with schema 1, one immutable operation id and body per decision and
  the revision read. An unknown outcome is retried only with the same id and
  body. `SELF_APPROVAL` is shown as "another member must decide", keeps the
  record shown and is never retried; `CONFLICT` clears the pending decision,
  re-reads the record and its asset availability and shows the current
  server state with the conflict; `FORBIDDEN` and `NOT_FOUND` clear every
  private panel record (a foreign or vanished report leaves nothing of it on
  screen); `FRESH_AUTH_REQUIRED` and `UNAUTHENTICATED` end the session view;
  `VALIDATION_FAILED` keeps the input. A released run is shown with its
  device, server and current outcomes, the unchanged issue positions and the
  release instant from the server read; the panel never edits evidence.
- **B-BR-EMP-510 — Isolation, freshness and privacy of the panel.** The
  panel opens only on the member's request, shows each list and record with
  the instant it was read (BR-TRUTH-006) and clears on a change of
  organization, a refusal, sign-out or reload, discarding late responses.
  Nothing is kept in browser storage; private responses must be `no-store`;
  references sent are only those the server returned; no free identifier is
  typed.
- **BUC-EMP-258 — Release a blocked vehicle from the panel.** A fleet manager
  opens the checklists: FR-001's `BLOCKING` departure run is first, item 2
  critical; the vehicle is `BLOCKED` by that run (`CHECKLIST_RUN`). The
  manager enters "Freios verificados na oficina" and releases; the panel
  re-reads: the run is `COMPLETED_WITH_ISSUES` at revision 2 with the device
  and server outcomes and the release instant, the vehicle `AVAILABLE` as the
  server evaluated it. The run's author trying the same release gets "another
  member must release it" (`SELF_APPROVAL`) and the run stays blocked.
- **BUC-EMP-259 — Treat a damage report.** A manager opens the open reports,
  opens a blocking damage with its description, history and a stored photo,
  acknowledges it, records the action, resolves it with a note; each step
  shows the new revision from the server. With `doc.document.view_content`
  the manager saves the photo through the audited path; a supervisor without
  it sees only "Foto anexada (armazenada)". A second handler at the same
  revision sees the report already updated and decides again.
- **BUC-EMP-260 — Prove the panel and its queue.** Real PostGIS proves the run
  queue (order, pagination, `ORG`-only coverage, refusals, another
  organization, the read changes nothing) and its composed HTTP route; the
  domain and HTTP tests prove the content hint; the web tests prove that a
  self-release surfaces `SELF_APPROVAL`, a block is never hidden, no content
  action exists without the permission, a foreign report clears the panel and
  a `CONFLICT` re-reads; a local Chromium check proves the DOM.

## Checklist gate at trip start and finish (2026-11-15)

Evidence.

- **B-BR-EMP-511 — A trip start names its departure run; a finish may name
  its return run.** `trip.start` and `trip.finish` keep operation schema 1
  and their exact payloads. A start may carry in its envelope
  `soft_references` at most one `{type: "checklist_run", id}` beside the
  EMP-TRIP-023 mission reference; a finish may carry at most one
  `checklist_run` reference and nothing else. Any other reference type, two
  references of one type, or a reference on another trip operation stays a
  recognised but unsupported form (`RETRY`/`UNAVAILABLE`, nothing stored);
  a reference id that is not a canonical lowercase UUID is refused by the
  shared envelope profile. The Android producer names, on the start, the
  latest completed departure run of the chosen vehicle that its sealed
  checklist book holds as valid and not `BLOCKING`, and nothing when there
  is none; the shared table `trip/trip-operations-v2.psv` (superseding v1,
  hash-checked by Go and Kotlin) pins those bytes. The device's cached
  template and policy never decide the server gate.
- **B-BR-EMP-512 — The departure gate is decided by the server at receipt.**
  Under the organization root and asset locks of the start, after the
  final refusals (`MALFORMED`, `FORBIDDEN`, `DEPENDENCY_PENDING`) and the
  FLEET-007 start decision: a departure checklist is *required* when the
  CHECK-001 applicable-template rule (BR-CHECK-002) finds an `ACTIVE`
  `DEPARTURE` template for the asset in the organization's current
  configuration. A named run *counts* only when it is a stored run of the
  organization, of the `DEPARTURE` context, of the start's asset, authored
  by the start's author membership and not yet linked to any trip. A
  counting run is *blocking* at the start instant when its server recheck
  is `BLOCKING` and it was not released before that instant (the EDR-285
  block period), and *valid* when the start instant lies in
  `completion, completion + validity)`, where the validity is that of the
  run template's policy revision in force at the start instant by the
  server's history (the latest revision effective at or before it, else
  revision 1). The gate passes when nothing is required and nothing is
  named, or when a counting run is neither blocking nor outside its
  validity. Otherwise its reason is, first failure first:
  `CHECKLIST_REQUIRED` (required and nothing named, or required and the
  named run does not count), `ORPHAN_REFERENCE` (not required and the named
  run does not count), `CHECKLIST_BLOCKING`, `CHECKLIST_EXPIRED` (outside
  the validity, before the completion included). A named run is decided
  even when no checklist is required.
- **B-BR-EMP-513 — A failed gate holds the start with the closed
  acknowledgement vocabulary and stores its reason.** No acknowledgement
  code is added. The existing held codes keep their order
  (`PERMISSION_CHANGED`, `ASSET_UNAVAILABLE`, `ASSET_BUSY`); then a failed
  gate holds an otherwise accepted start `ASSET_UNAVAILABLE` for
  `CHECKLIST_REQUIRED` without a named run, `CHECKLIST_BLOCKING` and
  `CHECKLIST_EXPIRED`, and `ORPHAN_REFERENCE` (BR-OFFL-007, the link
  removed) when the named run does not count; then the EMP-TRIP-023
  mission reference. Whatever held code wins, a failed gate stores one
  insert-only gate reason beside the held start (its operation and review
  item, the reason and the named run when it is stored in the
  organization); the database refuses a reason without its held start, a
  `BLOCKING` or `EXPIRED` reason the run's history does not prove at the
  held start's declared instant, and any change or deletion. A held start
  never links; the EMP-TRIP-007 acceptance creates the declared trip
  without the run.
- **B-BR-EMP-514 — An accepted start links its departure run once.** A
  start that passes the gate with a named run stores, in the trip's
  transaction, one insert-only link (trip, `DEPARTURE`, run, start
  operation): one departure link per trip and one trip per run, so a second
  start naming the same run does not count it. The database re-proves the
  link against the stored trip (its own start operation, `IN_PROGRESS`, the
  run's context, asset, author, not blocking and valid at the declared
  start) and refuses any change or deletion. The link is read back by the
  receipt confirmation; replays return the stored acknowledgement byte for
  byte, and the same operation with another digest (another run) is
  `OPERATION_ID_REUSED`.
- **B-BR-EMP-515 — A return run never holds a trip open (BR-CHECK-013).** A
  finish naming a stored `RETURN` run of the trip's asset, authored by the
  finishing member, completed at or after the trip's declared start and at
  or before the finish, and not linked to any trip, links it (trip,
  `RETURN`, run, finish operation) in the finish's transaction, whatever
  the run's outcome; a `BLOCKING` return run blocks the asset's next use
  through EDR-285. A finish whose return run is missing, foreign, of
  another asset, context or member, outside the trip or already linked is
  accepted exactly as without the reference and links nothing; a held
  finish (`TRIP_ALREADY_CLOSED`) links nothing.
- **B-BR-EMP-516 — One lock order decides a release and a start.** The
  gate reads runs, releases and policy history without row locks, under
  the organization root lock that every run insert and every release
  takes first, so a release and a start serialize in either order without
  a deadlock and are decided by history: a start declared before a
  release's recording is gated `CHECKLIST_BLOCKING` in both orders, and a
  start declared after it passes.
- **BUC-EMP-261 — Start after a valid departure check.** A vehicle whose
  category has an active departure template (BLOCK, 12 hours): the driver,
  offline, completes the check at 07:40 with every item OK and starts at
  07:55 at 184 320 000 m; the start names the run. On reconnect the run is
  accepted, then the start is accepted and linked to the run. A second
  driver's start on another day naming the same run is held
  `ORPHAN_REFERENCE` with the reason `CHECKLIST_REQUIRED`.
- **BUC-EMP-262 — Start held by the gate.** The same vehicle: a start naming
  no run, a run of another vehicle or another organization's run is held
  (`ASSET_UNAVAILABLE`, `CHECKLIST_REQUIRED`, or `ORPHAN_REFERENCE` for the
  named one); a start naming a run whose brakes item was `NOT_OK` under
  BLOCK is held `ASSET_UNAVAILABLE` with `CHECKLIST_BLOCKING`; a device
  whose cached policy still said 72 hours starts 13 hours after its check
  and is held `CHECKLIST_EXPIRED` because the server's policy says 12. The
  facts are kept and a reviewer decides them in the EMP-TRIP-007 queue.
- **BUC-EMP-263 — Return evidence at the finish.** The driver completes a
  return check with a damaged mirror (`BLOCKING` under BLOCK) and finishes
  at 184 610 000 m naming it: the trip completes, the run is linked to the
  finish and the vehicle is blocked for the next use until released. A
  finish naming a return run the server does not hold yet still completes
  the trip and links nothing.

## Meter replacement, reset and rollover with cumulative usage (2026-11-15)

[Evidence.

- **B-BR-EMP-481 — Segments and the segment a reading counts in.** A meter
  counts in ordered, contiguous segments. A segment opens for a reason
  (`INSTALLATION`, `RESET`, `ROLLOVER`) at an instant with an opening value
  (the value of its opening reading for `INSTALLATION` and `RESET`; zero for
  `ROLLOVER`) and closes at most once, with a closing kind (`REMOVAL`,
  `RESET`, `ROLLOVER`), the closing instant and the closing value (`REMOVAL`:
  the observed final value, or unknown; `RESET`: the value observed before;
  `ROLLOVER`: the meter's modulus). A reset or rollover closes the open
  segment and opens the next one at the same instant; a removal closes the
  meter's last segment and the meter at the same instant. A reading counts in
  the segment of its meter whose half-open interval `opened, closed)`
  contains its instant, except a `METER_REMOVAL` reading, which counts in the
  segment it was recorded in (the segment its removal closes). The segment recorded on the
  reading row is the one that was open at intake and never changes; a reading
  whose instant lies after a later boundary counts in the next segment.
  Classification, re-evaluation, the current value, the history, the suspect
  list and decisions use this segment, so monotonicity and plausibility are
  checked inside one segment only (BR-METER-007, BR-METER-009).
- **B-BR-EMP-482 — Cumulative usage, usage between instants and the
  equivalent display.** For one asset and one kind, take its meters in
  installation order and their segments in order, and only `VALID`
  readings. The anchor at instant `t` is the latest such reading at or before
  `t` (segment order, then BR-METER-008 order). `U(t)` is the sum of
  `close − open` of every segment before the anchor's segment plus
  `anchor − open` of the anchor's segment; `E(t) = open(first segment) +
  U(t)`; `usage(t1, t2)` runs from the anchor at `t1` to the anchor at `t2`:
  inside one segment the difference, across segments `close − anchor₁` of the
  first, `close − open` of every segment between and `anchor₂ − open` of the
  last. A missing anchor gives `INSUFFICIENT_DATA` with `NO_READING_BEFORE`
  (an anchor that exists is still named); a segment in the summed span that
  closed with an unknown value gives `INSUFFICIENT_DATA` with `METER_GAP`;
  `t1` after `t2` is invalid; nothing is interpolated and kinds are never
  mixed (BR-METER-021, BR-TRUTH-003). Example C gives `U = 1 944 000` m,
  usage R4′ → M2 reading `= 754 000` m and `E = 184 944 000` m; example D
  gives usage S1 → S2 `= 170 000` m and `E = 1 000 120 000` m.
- **B-BR-EMP-483 — A rollover is only a candidate until a person confirms
  it.** When the meter has a modulus `M`, a candidate below its valid
  predecessor `v₁`, with no valid successor in its segment, whose wrap usage
  `(M − v₁) + v₂` passes the §10.2 rate check over the elapsed time, is
  stored `SUSPECT` with `POSSIBLE_ROLLOVER` instead of
  `READING_BELOW_PREVIOUS` (canonical flag order: after `FUTURE_INSTANT`).
  The same rule applies at intake, at correction and at re-evaluation; a
  meter without a modulus never produces the flag. Nothing applies a
  rollover by itself; a `CONFIRM` of a reading flagged `POSSIBLE_ROLLOVER` is
  `VALIDATION_FAILED`, like a monotonicity flag.
- **B-BR-EMP-484 — Replacing a meter.** A member with `meter.reading.correct`
  replaces the meter in service by one company command naming the meter, its
  expected segment count (the meter's revision for these commands), the
  removal instant, the final value of the old meter or `unknown`, and the new
  meter's label, optional modulus and opening value (exact base units of the
  same kind). It is `VALIDATION_FAILED` when the instant is not after the open
  segment's opening, is later than the database clock, or has a not-`VOIDED`
  reading of the old meter at or after it; when the final value is below the
  latest `VALID` reading before the instant or not below the old modulus;
  when the new modulus is not positive or the opening value is not below it.
  In one transaction it stores the final value as a `METER_REMOVAL` reading
  (the closing reading; none when unknown), closes the last segment
  (`REMOVAL`), removes the meter at the instant, installs the new meter with
  its `INSTALLATION` segment and `METER_INSTALLATION` opening reading at the
  same instant, and returns and stores the implied usage (`final − latest
  valid reading`, or `INSUFFICIENT_DATA`/`METER_GAP` when unknown). A second
  meter of the same kind in service stays impossible
  (`METER_ALREADY_IN_SERVICE`, BR-METER-005).
- **B-BR-EMP-485 — Resetting a meter.** The same command with `RESET` names
  the instant, the value observed before and the value observed after. It is
  `VALIDATION_FAILED` when the instant is not after the open segment's
  opening or is later than the database clock, when a `VALID` reading of the
  open segment lies at or after it or any not-`VOIDED` reading lies exactly
  at it, when the before value is below the latest `VALID` reading before the
  instant, or when a value is not below the modulus. It closes the open
  segment at the instant with the before value as its closing value and
  stores the after value as the opening reading of a new `RESET` segment
  opened at the instant (`STANDALONE`, `TYPED`, by the deciding member
  through the company session; one operation carries one reading per meter),
  re-evaluates the `SUSPECT` readings now counting in the new
  segment (BR-METER-012: a segment boundary between two readings clears a
  monotonicity flag), and returns and stores the implied usage `before −
  latest valid reading`.
- **B-BR-EMP-486 — Confirming a rollover, anchors and audit.** The decision
  `CONFIRM_ROLLOVER` on a reading of the open segment (expected revision and
  reason, as every decision) is `VALIDATION_FAILED` on a meter without a
  modulus, on a reading not flagged `POSSIBLE_ROLLOVER`, when a `VALID`
  reading of the segment follows it, or when another not-`VOIDED` reading or
  its valid predecessor lies at its instant; a reading that is not `SUSPECT`
  is `CONFLICT`. It closes the open segment at the reading's instant with the
  modulus, opens a `ROLLOVER` segment there whose opening value is zero and
  whose first reading is the confirmed one, makes the reading `VALID` (cause
  `ROLLOVER`, actor and reason), re-evaluates the new segment and returns and
  stores the implied usage `(M − v₁) + v₂`. Every opening or closing reading of a
  segment is an anchor and refuses any decision with `VALIDATION_FAILED`.
  Each command writes one audit event (`meter.replaced`, `meter.reset`,
  `meter.rollover.confirmed`; actor, target, command digest; no value or
  reason) and commits with all its effects or not at all, serialised by the
  root, asset and meter locks; the same operation and digest returns the
  first result, another digest is `OPERATION_ID_REUSED`; a stale segment
  count is `CONFLICT`; another organization's or asset's meter is
  `NOT_FOUND`.
- **BUC-EMP-245 — Replace an instrument panel.** A fleet manager records the
  replacement of "Caminhão 07" at 184 532 km with a new odometer at 0; a
  reading of 412 km on the new meter gives 1 944 000 m of cumulative usage
  and 754 000 m since R4′ (example C).
- **BUC-EMP-246 — Record a reset.** A supervisor records that an hour meter
  restarted: before, after and the instant; readings typed after the restart
  and flagged below their predecessor return to `VALID` in the new segment.
- **BUC-EMP-247 — Confirm a rollover.** A supervisor opens the reading of
  120 km flagged `POSSIBLE_ROLLOVER` on "Caminhão 11", sees the implied
  170 000 m and confirms; the reading becomes `VALID` in the new segment
  (example D).

## Current-value projection and usage-between-instants query (2026-11-15)

[Evidence.

- **B-BR-EMP-487 — The current value is a maintained projection.** Every meter
  has one projection row: its open segment (none once removed), the latest
  currently `VALID` reading counting in that segment (B-BR-EMP-481) with its
  value and instant (none when there is no such reading), and a revision equal
  to the number of the meter's source facts (readings, history revisions,
  segments, segment closures and the removal). The database writes it in the
  transaction of every such fact, so intake, fuelings, trips, held reviews,
  decisions, rollovers, replacements, resets and EMP-TRIP-015 amendment
  supersessions all maintain it; a change of any fact raises the revision.
  The projection is never the source of truth (BR-METER-006): a figure is
  computed from readings, and any doubt is settled by a rebuild.
- **B-BR-EMP-488 — Rebuild.** An operator rebuild deletes one organization's
  projection rows and recomputes them from the readings in one transaction;
  the rebuilt rows equal the incrementally maintained rows (values and
  revisions) and a second rebuild changes nothing. The rebuild needs an
  operator identity and changes no business fact.
- **B-BR-EMP-489 — Usage between two instants.** A server port takes an asset,
  a kind and two instants (`from` at or before `to`) and answers with the
  B-BR-EMP-482 figure over the asset's meters of that kind, their segment
  chain and their currently `VALID` readings: status (`OK` or
  `INSUFFICIENT_DATA`), reason (`NO_READING_BEFORE`, `METER_GAP`), usage
  (absent unless `OK`), both anchor readings that exist (identifier, value,
  instant), the method version `meter-usage-v1` and the projection revision
  (the sum of the revisions of the asset's meters of that kind) read with
  them. It never interpolates. Instants
  in reverse order are `VALIDATION_FAILED`; another organization's asset, or
  an asset without a meter of that kind, is `NOT_FOUND`.
- **B-BR-EMP-490 — Consistency and recalculation.** A usage answer reads
  readings, history, segments and projection under a shared lock of the
  asset's meters of that kind, which every reading change locks for update,
  so a concurrent correction is either wholly before or wholly after it. A correction raises the projection revision in its own
  transaction and, when consumers are composed, emits
  `meter.usage.recalculated` once (EMP-METER-004); the projection itself needs
  no event.
- **BUC-EMP-248 — Ask for usage between two instants.** A module asks for the
  odometer usage of an asset between a trip's start and finish and receives
  the figure, both readings it used and the projection revision, or
  `INSUFFICIENT_DATA` with the reason.
- **BUC-EMP-249 — Rebuild the projection.** An operator suspects drift and
  rebuilds one organization's projection; nothing changes when it was right.

## Stops and legs: planned versus actual (2026-11-16)

Evidence.

- **B-BR-EMP-475 — Planned stops belong to the planning revision.** A
  `PLAN` command (EMP-TRIP-009) may carry `stops`: 1 to 20 ordered
  intermediate stops between the trip's start and the plan's destination
  (a technical bound, not a product rule), each a free-text place with the
  plan text profile (B-BR-EMP-382: nonblank, control-free, at most 280
  characters, no standalone CPF-shaped token). No unit, coordinate, time,
  person or contact is accepted; an absent list means no planned stop and
  an empty list is refused. The stops are stored insert-only with the
  planning revision their `PLAN` produced (sequence 1..n), in the `PLAN`
  transaction and before its receipt; a re-plan appends a new revision with
  its own stops and keeps every earlier one. The plan of an `ACCEPTED`,
  `IN_PROGRESS`, `COMPLETED` or `CANCELLED` mission cannot change
  (B-BR-EMP-381, B-BR-EMP-413), so the stops shown beside a trip are those
  the member accepted. A plan without stops keeps the `trip.mission.plan.v1`
  digest byte for byte, so stored receipts replay; a plan with stops binds
  them in `trip.mission.plan.v2`. Every plan projection carries `stops` only
  when the revision has stops. The planned legs are derived, never stored:
  start → stop 1 → … → stop n → destination; no planned distance, duration
  or route estimate is shown or compared with travel (BR-ROUTE-008).
- **B-BR-EMP-476 — Observed stops and legs are derived from the trip's
  typed facts.** Nothing new is recorded on the device: the observed side is
  derived at read time from the trip's start, its end (the finish, or the
  cancellation; a supervisor closure has no device instant and ends
  nothing) and its accepted `STOP`, `ARRIVAL` and `DEPARTURE` logbook events
  (EMP-TRIP-003), in the shared timeline order (B-BR-EMP-324: declared
  instant, then identity; the receipt is never an ordering key). A `STOP`
  or an `ARRIVAL` opens a stop and ends the leg that led to it; a
  `DEPARTURE` closes the open stop and opens the next leg; the trip's start
  opens the first leg and its end closes the last one; a trip that ends
  while stopped ends at that stop. Every boundary keeps its fact identity,
  type, declared instant, entry zone and device offset (BR-TIME-001) and the
  server receipt (BR-TIME-002). Notes, pauses, incidents and other facts are
  not stops. No distance, duration or pairing with a planned stop is
  derived. The derivation is shared by Go and Kotlin through
  `trip/stops-v1.psv`.
- **B-BR-EMP-477 — Incoherent observations are surfaced, never paired,
  reordered or dropped.** A `DEPARTURE` with no open stop yields a stop
  without arrival (`ARRIVAL_MISSING`), and the leg that should have ended
  there ends without arrival (`ARRIVAL_MISSING`); a `STOP` or `ARRIVAL`
  while a stop is still open leaves that stop without departure
  (`DEPARTURE_MISSING`) and starts the next leg without departure
  (`DEPARTURE_MISSING`). A boundary declared after its own server receipt
  is flagged `after_receipt` (BR-TIME-006: reported, never rewritten).
  Observations outside the trip keep their EMP-TRIP-003/015 handling: before
  the start `MALFORMED` (nothing stored); after the finish or on a decided
  revision held for review (`TRIP_ALREADY_CLOSED`, `STALE_REVISION`); the
  read lists the trip's held `STOP`, `ARRIVAL` and `DEPARTURE` observations
  with their code and declared time, never as stops. An observation accepted
  while the trip was open but declared after the end that was recorded later
  (a device clock that moved) is listed apart as `after_end`, in timeline
  order, never as a stop or leg boundary and never dropped.
- **B-BR-EMP-478 — The member reads the plan beside the observed stops and
  legs of an own trip.** `POST
  /v1/orgs/{org_id}/trips/mine/{trip_id}/stops/read` (company session,
  `trip.trip.start`, read; the target is the requesting membership, the
  EMP-TRIP-012 own-trips policy) answers for a trip whose starting member or
  current responsible member is the requester: the trip summary with its
  start and end boundaries; `planned` — `null` when the trip executes no
  mission (EMP-TRIP-023 link) or its mission has no planning revision, else
  the mission's current planning revision (revision, planning instant,
  mission window, destination, stops, derived legs); and `observed` — stops,
  legs and held observations with their recorded times. Another member's trip,
  another organization's trip and an unknown trip are the same `NOT_FOUND`
  with nothing revealed; a member without `trip.trip.start` is `FORBIDDEN`.
- **B-BR-EMP-479 — Planned data never rewrites observations.** Planning
  revisions and their stops live in their own insert-only tables; the
  observations are the insert-only logbook rows. A re-plan before
  acceptance, or a `PLAN` refused on an executing mission (`CONFLICT`),
  changes no trip, event, held event or observed stop or leg. The database
  refuses a stop row outside its `PLAN` transaction (after its receipt, on
  a head at another revision or state, out of sequence), a stop row without
  its published `PLAN` receipt, and any change or deletion.
- **B-BR-EMP-480 — Replays change nothing.** The same `PLAN` operation with
  the same digest replays its stored receipt and stores no second stop; the
  same operation with other stops is `OPERATION_ID_REUSED`. An observation
  resend returns the stored acknowledgement byte for byte and the same
  identifier with another digest is `OPERATION_ID_REUSED` (EMP-TRIP-003);
  the observed stops and legs read after a replay are identical.
- **BUC-EMP-242 — Plan the stops of a delivery.** A planner plans a mission
  to the destination "Centro de distribuição Norte" with the stops "Cliente
  Jardim Primavera" and "Posto de apoio BR-163", then, before assigning it,
  re-plans with a third stop "Cliente Vila Rica". The second planning
  revision holds three stops and four planned legs; the first keeps its two
  stops. After the driver accepts, a re-plan is refused.
- **BUC-EMP-243 — Distinguish the plan from what happened.** The assigned
  driver starts the linked trip at 08:00 (America/Cuiaba, −04:00), records
  `ARRIVAL` 09:10, `DEPARTURE` 09:40, `STOP` 11:00, `DEPARTURE` 11:15 and
  finishes at 12:30. The member's read shows the three planned stops and
  four planned legs and, separately, two observed stops (09:10–09:40,
  11:00–11:15) and three observed legs (08:00–09:10, 09:40–11:00,
  11:15–12:30), each time with its zone, offset and receipt; nothing says
  which planned stop was visited.
- **BUC-EMP-244 — Incoherent observations stay visible.** The driver forgets
  an arrival and records two departures in a row, then two arrivals in a
  row, and a device clock ahead of the server declares one arrival after it
  was received; an arrival recorded after the finish is held. The read shows
  a stop without arrival and a stop without departure with their legs
  flagged, the arrival flagged `after_receipt` and, because the finish was
  then declared before it, listed apart as `after_end`, and the held arrival
  with `TRIP_ALREADY_CLOSED`; no time is moved and no event is dropped.

## App: add an expense with receipt (2026-11-15)

Evidence.

- **B-BR-EMP-517 — The device builds the exact `expense.submit` the intake
  reads.** The app produces only schema 1 of `expense.submit` through the
  EMP-EXP-002 Kotlin builder (EDR-289): entity `expense` with a new device
  identifier and no expected revision, no dependency, soft reference or
  envelope attachment, and the exact six-key payload — the chosen asset, a
  v1 direct category, the paid total and the company amount as exact
  centavos texts, the payer and `trip_id` null. The registered producer
  admits only an operation the device decoder reads back, inside the
  protected write; the shared oracle `expense/expense-submit-v1.psv` pins the
  bytes for both languages.
- **B-BR-EMP-518 — An expense is saved durably in its creation partition.**
  "Salvar despesa" queues the operation through the EMP-OFFL-003 protected
  write of the selected organization's partition (account, origin,
  organization, membership, enrollment); "Salvo no aparelho" appears only
  after that write returns; a failed write says that nothing was saved and
  keeps the form. Nothing else of the expense is stored: list and detail are
  read back from the partition's retained `expense.submit` entries, so the
  original scope and both amounts survive restart exactly as queued. Another
  organization or account never lists or sends it.
- **B-BR-EMP-519 — Entry is exact and offline.** The asset comes from the
  EMP-FLEET-013 picker (implements excluded); the category is one of the
  EMP-EXP-002 accepted direct categories (toll, parking, maintenance, lease,
  insurance, fine, other — a fuel cost is a fueling, recorded in
  "Abastecimentos"); "Valor pago" and "Valor para a empresa" are both
  required, take reais with at most two decimals, a decimal comma or point
  and no grouping, and are converted to integer centavos without rounding;
  a company amount above the paid total, a third decimal, a grouping
  separator, a currency sign or a value beyond the money profile is refused
  before saving ("confira os valores") and nothing is saved; the payer
  (company or employee) is an explicit choice. Nothing is recomputed.
- **B-BR-EMP-520 — The receipt is a separate, optional photo linked to the
  expense.** In the form, "Comprovante (opcional)" uses the EMP-DOC-004 sheet
  (camera or import, legibility hint, keep or retake; synthetic images only
  until OQ-04); not taking one is allowed. After the expense is saved, its
  photo is sealed in a second protected write with target `EXPENSE` (the
  expense identifier) and owner the `expense.submit` operation of the same
  partition — a receipt can never name an operation of another organization
  or account (`UNKNOWN_OPERATION`, nothing saved). If that second write fails
  or is refused the expense stays saved and the screen says the receipt was
  not saved, keeping the photo for another attempt; a receipt can also be
  added later from the expense's detail. The photo is declared only after
  the expense's final receipt (EDR-268/269) and links only to a stored
  direct expense, through the new `EXPENSE` link kind of EMP-DOC-003 resolved
  by the expense owner under `exp.expense.record` (the author's own expense;
  another member's is `FORBIDDEN`; another organization's, a missing one or a
  fueling's own expense `NOT_FOUND`). A link is evidence, never a cost: it
  changes no amount and creates no expense; its progress is shown
  separately and never changes the expense's state.
- **B-BR-EMP-521 — Acceptance follows the server; rollback and scope.** The
  expense shows "Salvo no aparelho" until its own final receipt: "Aceito pela
  empresa" with "Aguardando aprovação" only from an `ACCEPTED` receipt, "Não
  aceito" with the reason otherwise; a held expense (changed rights, after
  disposal) stays saved and retried. No approval, return or decision is ever
  made or shown as made on the device. `ENTERPRISE_EXPENSE_UI_ENABLED`
  (default on) hides the entry; queued expenses and receipts keep their
  delivery. No trip reference (`trip_id` stays null; EMP-TRIP-020 decides),
  configured category (EMP-EXP-003) or real receipt is produced.
- **BUC-EMP-264 — Record a toll offline with a receipt.** Bruno, with no
  signal, opens "Despesas", chooses his truck, picks "Pedágio", enters
  R$ 12,50 paid and R$ 12,50 for the company, paid by himself, photographs
  the synthetic receipt and saves: "Despesa salva no aparelho com o
  comprovante". The phone restarts; both are still saved. On reconnection the
  expense becomes "Aceito pela empresa — aguardando aprovação"; the receipt
  shows "Enviando" until it is stored ("Arquivo guardado pela empresa").
- **BUC-EMP-265 — Without receipt now, receipt later.** Parking of R$ 8,00
  paid by the company is saved without a photo; the receipt is added the next
  morning from the expense's detail.
- **BUC-EMP-266 — Negatives.** A failed protected write (nothing saved), an
  expense of organization A never listed or sent under B, "12,345", "1.234,56"
  or a company amount above the paid total (nothing saved), a receipt refused
  locally (expense kept, receipt not saved), a receipt naming an operation of
  another partition, an attempt to link a receipt to another member's,
  another organization's or a fueling's expense on the server, a saved
  expense never shown as accepted before its receipt, and the shared oracle
  rows that the intake refuses.

## Trip closing conference: expenses and documents before submission (2026-11-15)

Evidence.

- **B-BR-EMP-522 — The conference is device-local.** Before a finished trip
  is marked ready to send, the trip detail shows "Conferência de
  fechamento": the trip's own departure and final readings and the
  fuelings and expenses saved on this device in the trip's organization
  (its own protected partition) for the trip's vehicle, declared between the
  trip's start and its finish (since the start while it is open). This
  grouping exists only on the device: no trip reference is sent with a
  fueling or an expense (`trip_id` stays null and RETRY/UNAVAILABLE at the
  intake), the server learns nothing new, and records of another vehicle,
  organization or account, or outside the interval, never appear.
- **B-BR-EMP-523 — Each item keeps its own record state.** A reading, fueling
  or expense shows only what its own retained receipt proves: "Salvo neste
  aparelho", "Enviando", "Aceito pela empresa", "Precisa de revisão" or "Não
  aceito" with the reason. Nothing is shown as accepted, approved or
  returned without that receipt.
- **B-BR-EMP-524 — Evidence is missing, pending or available, never
  certified.** Each item's evidence comes only from the photos that its own
  operation owns and that target that record: no usable photo (none, a
  missing file or a refused transfer) is "faltando"; a photo saved on the
  device or still being sent or checked is "pendente"; every photo stored by
  the company is "guardado pela empresa" — stored, never "válido",
  "verificado" or "certificado". A photo of another record never counts; an
  attachment index this device cannot read shows every item's evidence as
  unreadable, never as available; unreadable fuelings or expenses are said
  ("a lista pode estar incompleta"), never shown as nothing to confer.
- **B-BR-EMP-525 — A link is never a cost.** Each fueling or expense shows its
  own entered paid amount; the conference adds nothing up, and a fueling's
  own expense (projected on the server by EMP-EXP-002) is never listed as a
  second item. Duplicated entries count once.
- **B-BR-EMP-526 — Submission names the right revision and decides
  nothing.** From the conference, "Marcar como pronta para enviar" queues the
  trip's first `trip.submit` exactly as EMP-TRIP-005 does: entity
  `trip_review` at revision 1, depending on the finish; a trip already marked
  queues nothing more (`ALREADY_SUBMITTED`). Approval, return and rejection
  stay the company's: the device never shows or makes a decision offline. A
  resubmission after a return names the returned revision (EMP-TRIP-015) and
  needs the author's own review read, which does not exist yet: retained.
- **BUC-EMP-267 — Confer before sending.** Bruno finishes the trip, opens it
  and sees the departure reading with its photo "guardado pela empresa", the
  fueling of R$ 970,00 "Aceito pela empresa" with its receipt "pendente", the
  toll of R$ 12,50 "Salvo neste aparelho" with "Comprovante: faltando" and
  the final reading; he adds the toll's receipt from "Despesas", returns and
  marks the trip ready to send.
- **BUC-EMP-268 — Incomplete device data.** The device cannot read its
  fuelings: the conference says the list may be incomplete and still lists
  the readings and the expenses; marking ready remains possible and nothing
  is invented.
- **BUC-EMP-269 — Negatives.** A fueling of another vehicle, a parking of
  another organization and an expense saved after the finish are absent; a
  photo owned by the fueling but targeting the expense documents neither; a
  missing file is never "guardado"; an unreadable index never shows
  "guardado"; the amounts are never summed; marking twice queues nothing.

## Organization trip tracking service: explicit start from the trip screen, visible notification, location-guard update (2026-11-15)

Evidence. Owner
decision EDR-271: tracking is implemented for real and is active per
organization only with its employee notice in force; real-device acceptance
is owed.

- **B-BR-EMP-495 — The driver sees the organization's tracking policy in
  force.** `POST /v1/orgs/{org_id}/tracking/policy/mine` (company session,
  empty body) requires `trip.trip.start` covering the requesting membership's
  own record (OWNER, ADMIN, FLEET_MANAGER, SUPERVISOR, DRIVER) and answers the
  policy in force now (`OFF` until the version's employee notice is in force;
  revision `0` and `OFF` when nothing was recorded) with, only when it is not
  `OFF`, the notice version and its in-force instant. AUDITOR, FINANCE,
  APPROVER and members without the key are `FORBIDDEN` naming
  `trip.trip.start`; another or unknown organization is `NOT_FOUND`; the read
  writes nothing and carries no session, member or location data. The app
  keeps the last read and its read instant in the partition's sealed
  tracking book: `OFF` and a policy never read hide every tracking control,
  `OPTIONAL` offers "Iniciar rastreamento" beside "Continuar sem rastrear",
  `EXPECTED` says that the company expects this trip to be tracked and still
  lets the trip run without it (BR-GPS-005). The cached value is a display
  hint; the intake decides (B-BR-EMP-428).
- **B-BR-EMP-496 — Recording starts only from the visible trip screen.** Only
  the open trip screen of an `IN_PROGRESS` organization trip of the driver's
  own trip book offers the control, whose text names tracking; before each
  start or resume the screen shows what is collected and the organization's
  notice version (BR-GPS-034), requests fine and coarse location together and,
  on Android 13 and later, the notification permission. The result is
  recorded as it is: permission `NONE` (`PERMISSION_DENIED`) or a notification
  that cannot be shown (`NOTIFICATION_BLOCKED`) keeps the session where it was
  and starts no service. Recording runs only in the non-exported foreground
  service of type `location`, which starts listening only after consuming a
  one-time start grant armed by the resumed trip screen for that exact
  partition and session (valid 30 seconds); a start intent at boot, from a
  worker, a push, a notification, another process or with an expired or
  foreign grant starts nothing. The service is never sticky and never
  restarts itself; the app never declares or requests
  `ACCESS_BACKGROUND_LOCATION`; the static mobile guard fails the build when
  the background permission, an unlisted continuous subscription, a boot
  receiver, an exported or untyped tracking service or a sticky tracking
  service appears.
- **B-BR-EMP-497 — Every session transition is a signed queued operation.**
  The six `gps.session.start|resume|suspend|interrupt|reduce|end` schema-1
  operations are built exactly as the EMP-GPS-004 intake parses them (entity
  `tracking_session` with the session identifier, payload
  `{trip_id, origin, permission, notification_visible}` for start and resume
  with origin `TRIP_SCREEN`, `{trip_id, cause}` for the others) and saved in
  the protected queue together with the partition's sealed `tracking-book`
  record in one write; a failed write saves neither. The transition is
  decided again inside that write by replaying the stored transitions through
  the EMP-GPS-001 session domain with the trip facts of the partition's trip
  book, so the device never stores a transition the domain refuses. The first
  start depends on the trip's `trip.start`, each later session operation on
  the previous one and the end also on the trip's `trip.finish`; instants
  never go back. The trip screen states each record's sync state from its own
  receipt only ("Salvo no aparelho" until the server answers). A final
  refusal of any session operation stops the session on this device: nothing
  is collected or continued for it, and the screen shows the refusal with its
  code while the trip continues.
- **B-BR-EMP-498 — One listener, safe stop, bound owner.** The service holds
  at most one location listener for the one recording session of the
  process; a repeated start adds no listener and resets no counter; a stop
  releases only this service's listener before leaving the foreground. The
  session and its collection are bound to the account, origin, organization,
  membership, enrollment and trip that started it; a change of account or
  context, a locked or lost partition, or another responsible member of the
  trip stops collection at once, and when the partition is still writable the
  session is interrupted `OWNER_CHANGED`; nothing is ever rebound to another
  owner (BR-GPS-014, BR-OFFL-003).
- **B-BR-EMP-499 — The session follows the trip and the device, never
  silently.** Finishing the trip ends the session at the finish's own instant
  (`TRIP_COMPLETED`) and stops the service; a trip start finally refused by
  the server ends it `TRIP_CANCELLED`. "Parar rastreamento" in the
  notification or on the trip screen interrupts it `STOPPED_BY_DRIVER` while
  the trip continues. Every 30 seconds while recording the service re-checks
  permission (lost: `PERMISSION_REVOKED`), precision (fine reduced to
  approximate: the session keeps recording with `PRECISION_REDUCED`),
  notification visibility (`NOTIFICATION_BLOCKED`), owner and trip state, and
  writes a checkpoint with its boot identity and instant. After a process
  death, a force-stop or a restart the service does not come back; at the
  next opening of the trip screen a `RECORDING` session with no running
  service is interrupted at its last checkpoint instant with
  `DEVICE_RESTARTED` (another boot) or `PROCESS_STOPPED`, and the screen asks
  "Retomar rastreamento?"; recording returns only by that tap.
- **B-BR-EMP-500 — Never for personal or foreign trips; always visible.**
  Personal mode, a trip of another organization or membership, a trip that is
  not `IN_PROGRESS`, policy `OFF` and a policy never read start nothing. While
  recording, the ongoing notification names the organization, says that
  tracking is active and offers the stop action; the trip screen shows
  "Rastreamento ativo", "Rastreamento suspenso" or "Rastreamento
  interrompido" with its reason in text, not colour only. No coordinate
  appears in notifications, logs, errors or the trip screen; in this task the
  service only counts accepted platform fixes in memory (the buffer and the
  batches are EMP-GPS-003).
- **BUC-EMP-252 — A driver starts a trip and its tracking.** With the
  organization's policy `OPTIONAL` or `EXPECTED` read, the driver opens the
  trip in progress, reads what is collected and the notice version, taps
  "Iniciar rastreamento", grants location and notifications, and sees the
  ongoing notification and "Rastreamento ativo"; the start operation waits in
  the queue as "Salvo no aparelho" until the server accepts it. Denied
  location or blocked notifications leave the trip running without tracking,
  with the reason shown; policy `OFF` shows no tracking control.
- **BUC-EMP-253 — The driver stops tracking, resumes it and finishes the
  trip.** "Parar rastreamento" interrupts the session and removes the
  notification while the trip continues; "Retomar rastreamento" on the trip
  screen records again after the same checks; finishing the trip ends the
  session and stops the service; every transition is one queued operation
  delivered in order.
- **BUC-EMP-254 — Tracking after a process death or restart.** The app is
  killed while recording; nothing restarts it. When the driver opens the trip
  again, the session is shown interrupted with its reason and the instant of
  the last checkpoint, and "Retomar rastreamento" is offered; the driver may
  also continue without tracking.

## Adaptive sampling, local buffer and batch envelope (2026-11-16)

Evidence. Builds on
EMP-GPS-002 (the service and its session operations) and EMP-GPS-004 (the
`gps.track.batch` intake); owner decision EDR-271.

- **B-BR-EMP-501 — Approved sampling `sampling-v1`.** While a session records,
  the service requests a location every 5 seconds (GPS for precise, the fused
  or network provider for approximate) and records, among the fixes that pass
  the filter: the first fix of the session; while moving (speed of at least
  1.5 m/s) a fix 10 seconds after the last recorded one, or earlier on a turn
  of at least 30° once 5 seconds have passed; while slow, a fix 50 m away from
  the last recorded one; a heartbeat every 120 seconds; with approximate
  precision the heartbeat only. Below 15 % battery and not charging the
  routine interval becomes 30 seconds and the turn trigger is off; the trip
  screen says so and no battery-optimisation exemption is requested. The
  values are the documented defaults of the specification (§10.2) and change
  only through a new policy version.
- **B-BR-EMP-502 — Each fix is filtered and flagged before it counts.** A
  report with non-finite or out-of-range coordinates is ignored; a mock
  report is never recorded and is counted `mock`; a report older than 15
  seconds at collection, or earlier than the start of the recording period,
  is counted `stale`; a report without accuracy or, with precise permission,
  less accurate than 50 m is counted `accuracy`; a precise report implying
  more than 70 m/s from the previous accepted fix is counted `jump`. A fix's
  instant is the device clock at collection minus the report's monotonic age;
  fixes of an approximate period carry the coarse flag. Rejected reports are
  counted, never stored; no fix is ever created, moved or interpolated.
- **B-BR-EMP-503 — A sealed local buffer survives restarts.** Every recorded
  fix is written to the partition's sealed `tracking-buffer` record before it
  counts, with the session's next batch sequence, the counters since the last
  batch, the sampler's anchors and the fix-level gaps; the last recorded or
  accepted fix is the checkpoint that an interruption after a process death
  starts from. The buffer belongs to its account, organization, membership,
  enrollment and session and is never rebound. At most 2 000 fixes wait
  unsealed: past that bound a fix is not recorded and is counted as overflow;
  the app never deletes an unsent fix by itself.
- **B-BR-EMP-504 — Batches are bounded envelopes delivered in order.**
  Pending fixes are sealed into `gps.track.batch` schema-1 operations exactly
  as the EMP-GPS-004 intake parses them: at most 200 fixes and 8 KiB per
  operation, the first fix as base and declared instant, deltas in 10⁻⁷
  degrees, strictly increasing offsets of at most 24 hours, the boot identity
  and base elapsed time, source `PHONE`, the device counters since the
  previous batch, consecutive sequences per session from 1, and a dependency
  on the session operation that opened the fixes' recording period. Sealing
  removes exactly those fixes from the buffer in the same protected write as
  the queued operation; a refused or failed write keeps both unchanged.
  Fixes are sealed when the oldest has waited 60 seconds, when 200 wait, when
  a gap opens, when recording stops, ends or is found interrupted, and before
  a new boot's fixes; delivery is the ordinary queue's, after the session
  operations it depends on. Go and Kotlin share the batch vectors.
- **B-BR-EMP-505 — Gaps are data; coverage is never claimed through them.**
  Inside a recording period, more than 180 seconds without an accepted fix
  opens a fix-level gap from the last accepted fix (or the period start):
  `MOCK_LOCATION` when only mock reports arrived, `SIGNAL_LOST` otherwise;
  a disabled provider opens `PROVIDER_DISABLED` at once. The next accepted fix
  closes the gap at its own instant; the end of the period closes an open one.
  No batch spans the start of a gap, and nothing is recorded inside one. The
  trip screen shows the open gap with its reason and start. The gap reasons
  stay on the device in this task (the `gps.track.batch` format carries only
  fixes and counters; the server derives its own gaps from the stored fixes,
  EMP-GPS-005).
- **BUC-EMP-255 — A tracked trip is recorded and uploaded in batches.** The
  driver drives with tracking active; fixes are recorded by the sampling
  policy, sealed into batches every minute and sent with the trip's other
  operations when online; each batch shows "Salvo no aparelho" until its own
  receipt; the server stores each fix once.
- **BUC-EMP-256 — Signal, provider and mock problems become gaps.** In a
  tunnel, with the location provider off or with a mock location app, nothing
  is recorded; after 180 seconds (at once for the provider) the screen shows
  the gap and its reason; the next good fix closes it; the batches before and
  after never claim the interval.
- **BUC-EMP-257 — A restart keeps the buffer.** The app dies with fixes not
  yet sealed; at the next opening the session is interrupted at the last
  recorded fix, the waiting fixes are sealed into a batch and queued, and the
  driver may resume; nothing is lost, duplicated or invented.

## Cleanup of revoked sessions and devices on next contact (2026-11-16)

Evidence.

- **B-BR-EMP-536 — The revocation class comes only from the exact signed
  contact.** The device learns that a partition's enrollment was revoked,
  and its class, only from the EMP-PRIV-003 signed device contact for that
  exact account, origin, organization, membership and enrollment:
  `LOST_OR_COMPROMISED` (verdict ERASE) or `ACCESS_ENDED` (verdict LOCK). A
  whole-response 403 or 404 of the signed delivery plane and a late-intake
  acknowledgement code (`ACCESS_ENDED_BEFORE_RECEIPT`, `MEMBERSHIP_ENDED`,
  `INTAKE_WINDOW_CLOSED`, `DEVICE_REVOKED`) are only hints: the operation
  worker then makes one exact contact for that partition and applies its
  verdict. ACTIVE, a generic refusal, an outage and a foreign, malformed or
  late answer change nothing. A disconnected device changes nothing and
  claims nothing; its data stays under the offline validity of BR-PRIV-018
  (30 days without contact, then the key is discarded).
- **B-BR-EMP-537 — Cleanup touches only the revoked partition.** Locking and
  erasure act on the one partition (account, origin, organization) whose
  exact enrollment the verdict names. Another organization of the same
  account, another account or origin on the same device and personal
  storage keep their keys, files, queues and records.
- **B-BR-EMP-538 — Access ended: locked for use, still delivering eligible
  work.** After `ACCESS_ENDED` the partition is locked durably: no local
  read, no feature write, no new operation, no review lookup and no
  attachment upload. Its sealed unsent operations keep being delivered,
  unchanged, through the EMP-OFFL-009 intake-only path with the normal
  retry policy; an answer that is not final (`RETRY`, for example
  `UNAVAILABLE` for a type without a held path) keeps the operation pending
  and visible as pending. The partition is erased only when every operation
  has a final acknowledgement, or once the server answered
  `INTAKE_WINDOW_CLOSED` for that sealed period (the window never reopens and
  the remaining operations would be refused; they are counted, not sent).
  `LOST_OR_COMPROMISED` erases at once and the device never sends that
  partition's work again (UC-PRIV-03: unsent records on that phone are
  lost). The verdict never extends the partition's offline validity.
- **B-BR-EMP-539 — Cleanup is durable, resumable and reported as it is.** The
  erasure intent is persisted before any key or file is removed; an
  interrupted cleanup leaves the partition denied (never readable, never
  sent) and the existing maintenance retries it until every key and file is
  verified gone. The device states one of: locked and still delivering N
  records (`INTAKE_ONLY`), cleanup decided but not verified
  (`CLEANUP_PENDING`), or removed (`CLEANED`, only after verified disposal).
  The app never says data was removed while disconnected, before contact or
  before the erasure completed. When a company selection meets a LOCK
  verdict the app says that access to this company ended on this device,
  that records saved before are still being sent for the company's review,
  and that this company's data stays locked here and is removed after
  sending; nothing is said to be removed yet.
- **BUC-EMP-275 — Access ended while work is pending.** Ana is offboarded
  while her phone, offline, holds three fuelings and one meter reading of
  company A; she also belongs to company B. When the phone reconnects, the
  worker's delivery gets late-intake answers and asks for an exact contact:
  LOCK. Company A's data is locked at once (no list, no new record), the
  reading is held `ACCESS_ENDED_BEFORE_RECEIPT`, the fuelings stay `RETRY`
  `UNAVAILABLE` and pending, so nothing is removed. When the server answers
  `INTAKE_WINDOW_CLOSED`, company A's partition is erased; company B and her
  personal data are untouched.
- **BUC-EMP-276 — Lost phone reconnects; restart during cleanup.** A manager
  revokes Bruno's phone as lost. The phone's next delivery is refused as a
  whole; the worker's exact contact answers ERASE and only that company's
  partition is erased; its queued work is never sent again. If the app is
  killed in the middle of the erasure, the partition stays denied after
  restart, maintenance finishes the disposal, and only then does the state
  become `CLEANED`. While the phone stays offline nothing changes and
  nothing is claimed.

## Distance sources on a trip and the divergence signal (2026-11-16)

Evidence.

- **B-BR-EMP-491 — Each distance source stands alone.** A trip's distance
  answer carries four sources — odometer, GPS, planned and engine estimate —
  each with its own status (`OK` or `INSUFFICIENT_DATA`; for GPS also
  `WITHHELD`), a reason whenever it is not `OK`, and its own evidence; a value
  exists only on an `OK` source, in exact whole metres from 0 to 10¹² m, and
  sources are never added, averaged or substituted (BR-GPS-027, BR-TRUTH-003,
  BR-MONEY-002/009/010). The odometer source is the EMP-METER-006 usage of the
  trip's asset's odometer between the trip's start instant and its finish
  instant, with both anchor readings, the method `meter-usage-v1` and the
  projection revision; it is `OK` only when the usage is known and both
  anchors are currently `VALID` odometer readings at exactly those two
  instants, so a usage computed from an earlier reading is never reported.
  Otherwise its reason is `TRIP_NOT_FINISHED` (in progress or paused),
  `TRIP_CANCELLED`, `NO_FINAL_READING` (a supervisor closure, or no valid
  reading at the finish instant), `NO_START_READING`, `NO_METER` (no
  odometer: an hour meter is never a distance), `NO_READING_BEFORE`,
  `METER_GAP` or `METER_UNAVAILABLE` (the projection is not built or the
  stored chain is not a valid track). The GPS source is the latest stored
  EMP-GPS-005 summary revision of the trip's tracking session: distance with
  its method, quality, coverage, expected and gap seconds, gap count, session,
  revision and processing instant; its reason is `NO_TRACK` (no session),
  `NOT_PROCESSED` (no summary yet) or `NO_GPS_DISTANCE` (the summary has no
  distance; its quality and coverage are still stated). The planned source is
  `INSUFFICIENT_DATA` with `NO_ROUTE_PLAN` and the engine estimate with
  `NO_ROUTE_COMPUTATION` until a route owner records them.
- **B-BR-EMP-492 — Travelled distance basis.** The figure that needs
  travelled distance is the odometer distance with basis `ODOMETER` when that
  source is `OK`; otherwise the GPS distance with basis `GPS` (shown as "por
  GPS") only when the GPS source is `OK` with quality `PRECISE`; otherwise
  `INSUFFICIENT_DATA` with the odometer's reason and no number. A planned
  distance or an engine estimate is never travelled distance, a GPS estimate
  never replaces an odometer distance, and a consumer receives a value only
  together with its basis, so nothing (including money) can be computed from
  an absent basis (BR-GPS-028, BR-TRUTH-004).
- **B-BR-EMP-493 — Divergence is data for review.** The divergence signal is
  evaluated only when the odometer and GPS sources are both `OK` and the
  quality is `PRECISE`; otherwise it is `NOT_EVALUATED` with the reason
  `NO_ODOMETER_DISTANCE`, `GPS_WITHHELD`, `NO_GPS_DISTANCE` or
  `GPS_NOT_PRECISE`, in that order. With `diff = |odometer − gps|` in whole
  metres it is `RAISED` when `diff > M` and `diff × 100 > P × odometer`
  (integer arithmetic; defaults M = 2 000 m and P = 10 %, accepted ranges
  500–10 000 m and 5–30 %), otherwise `NOT_RAISED`. It carries both values,
  the difference and the thresholds beside the GPS source's coverage and
  gaps, asks for review and names no person, cause or loss (BR-GPS-029,
  BR-TRUTH-005). It is derived when the answer is read, so a later reading
  correction or summary revision changes it; nothing is stored or emitted.
- **B-BR-EMP-494 — The GPS part is a recorded-track read.** The answer is a
  server port with no route: its caller proves the reader's trip read first
  (EMP-TRIP-006). Inside the read transaction the GPS source is served only
  when the EMP-GPS-012 authority proves `gps.track.read` over the stored trip
  for the reader's current company proof and membership; without that grant
  the GPS source is `WITHHELD` with no value, quality or coverage, the basis
  never uses GPS and the divergence is `NOT_EVALUATED` with `GPS_WITHHELD`.
  Every answer that serves the GPS source, even one without a summary, records
  one `LOCATION_TRACK` access `ISSUED` and one `SERVED` (fresh identifiers and
  a digest of the trip, session and summary revision, no coordinate) before it
  is returned, and a failed record returns nothing. An unknown trip or another
  organization's trip is `NOT_FOUND` and records nothing; an invalid proof is
  refused before anything is read.
- **BUC-EMP-250 — Read a trip's distances.** A trip owner (the future trip
  panel, EMP-TRIP-006) asks for a finished trip's distances and receives the
  odometer distance with its two readings, the GPS distance with coverage,
  quality and gaps, the planned distance and the estimate as not informed,
  the basis and the divergence evaluation; a missing value says why and never
  shows zero.
- **BUC-EMP-251 — Review a divergence signal.** On a precise track the
  odometer distance and the GPS distance differ by more than both thresholds;
  the answer raises the signal with both values, the difference, coverage and
  gaps for a reviewer, and states nothing about the driver.

## App: missions inbox and planned-trip execution (2026-11-16)

Evidence.

- **B-BR-EMP-527 — The missions inbox is a sealed copy of the member's own
  list that always shows its age.** With a connection and on request
  ("Atualizar missões") the app reads the EMP-TRIP-008 own list
  (`/v1/orgs/{org_id}/missions/mine/list`, the account cookie plane the app
  already uses for its context reads) for the selected organization. The
  answer is parsed strictly (closed fields, the EMP-TRIP-009 plan shape,
  `data_mode`); an answer with any item whose assignee is not the member's own
  employee record (EMP-TRIP-005 `employee_id`) is refused as a whole and
  nothing of it is kept. An accepted answer replaces the previous copy in one
  protected write of the creation partition (EMP-PRIV-003 generation): one
  sealed record per partition (versioned `mission-inbox` format) bound to the
  membership, enrollment and employee record, holding at most the first 100
  missions in identifier order (more are flagged as not shown) and the device
  receipt clock (wall instant, boot identity and elapsed time). The inbox
  always states its age from that clock: elapsed time within one boot, wall
  time after a reboot (FLEET-013, EDR-226); at 24 hours or more, or when the
  age cannot be known (the clock moved back), it is labelled as possibly out
  of date. A copy is never shown without its age, never updated without a
  server answer, and another account, origin, organization, membership or
  enrollment never reads it. Refreshing is an online action: without a
  connection it is disabled with the reason; a failed or refused refresh keeps
  the previous copy and its age and says that nothing was updated; a refused
  read (`FORBIDDEN`) is the same safe answer and clears nothing silently.
- **B-BR-EMP-528 — Accept or decline offline as the signed response, sealed to
  its partition.** "Aceitar" and "Recusar" exist only for a mission of the
  partition's copy that is `ASSIGNED` and has no answer saved on this device
  for that revision. The answer is the frozen EMP-TRIP-008 `trip.mission.respond`
  schema 1 operation (entity `mission` with `expected_revision` = the copy's
  revision, payload exactly `response` and `reason`; a decline needs a reason
  in the METER-001 text profile, an acceptance has none; no dependency, soft
  reference or attachment), carrying the account, API origin, organization,
  membership and enrollment of the exact enrollment that created it
  (BR-OFFL-003). The operation and its line in the inbox record are committed
  in one protected write before the screen says "Resposta salva no aparelho";
  a failed write says that nothing was saved. Switching to personal mode or
  another organization never shows, moves or rewrites it; it keeps being
  delivered to its own organization. The server decides it on current facts
  at receipt only (EDR-229); a stale, reassigned or foreign answer is a
  transport denial that stores nothing. The device therefore delivers each
  response in a batch of its own after every other due operation, so a denial
  never holds another record; a denied response stays saved and unconfirmed
  and keeps being retried with the OFFL-003 backoff.
- **B-BR-EMP-529 — A planned trip starts only from a confirmed acceptance.**
  "Iniciar viagem da missão" exists only for a mission whose acceptance the
  company confirmed — the copy shows it `ACCEPTED`, or this device's own
  acceptance of the copy's revision has its `ACCEPTED` receipt — and that has
  no trip of this partition on this device linked to it whose start was not
  finally refused. A saved but unconfirmed acceptance shows why the start is
  not offered yet; an ordinary trip stays available. The start is the
  EMP-TRIP-005 `trip.start` with one soft reference `{type: "mission", id}`
  (EMP-TRIP-023) beside the EMP-TRIP-019 departure run when one is named; the vehicle is the mission's own (the picker offers only it;
  a mission without a vehicle offers the member's assets), and the departure
  rules of EMP-TRIP-005 apply unchanged. The server alone links the trip, moves
  the mission to `IN_PROGRESS` and consumes its reservation; a start whose
  mission is no longer eligible at receipt is held `ORPHAN_REFERENCE` and shown
  "Precisa de revisão" with that reason.
- **B-BR-EMP-530 — Pause, resume and cancel are their own signed operations.**
  The trip screen records the EMP-TRIP-011 `trip.pause`, `trip.resume` and
  `trip.cancel` schema 1 operations (entity `trip` with `expected_revision` =
  1 plus the trip's earlier lifecycle commands recorded on this device;
  payload exactly `reason`, optional for a pause or resume and required for a
  cancellation in the METER-001 profile; depending on the start). Local rules
  before queueing: pause only a running trip, resume only a paused one, cancel
  a running or paused one; events and the finish while running or paused;
  nothing after a cancellation; a cancelled trip no longer holds its vehicle on
  this device. The reserved `PAUSE`, `RESUME` and `CANCEL` logbook types are
  never produced. The server decides every command (`STALE_REVISION`,
  `TRIP_ALREADY_CLOSED` are shown "Precisa de revisão" with the reason). The
  local trip index becomes `trip-book` v2 (mission and lifecycle commands per
  trip) and reads every v1 index unchanged.
- **B-BR-EMP-531 — The inbox states only what a server answer proves.** A
  mission's state comes from the copy with its age; an answer's state from its
  own receipt ("Resposta salva no aparelho" without one, "Aceita pela empresa"
  or "Recusa registrada pela empresa" after its `ACCEPTED` receipt). When a
  later copy shows the mission at a revision after a saved answer that has no
  receipt, or no longer lists it, the inbox says the answer was not confirmed
  and the mission changed or is no longer assigned to the member. `IN_PROGRESS`
  and `COMPLETED` are shown only from a copy; the local trip line ("Em
  andamento", "Pausada", "Cancelada no aparelho") never claims a server state.
  No online-only action is ever reported as done offline.
- **BUC-EMP-270 — Answer a mission without signal (JRN-06 step 1, UC-TRIP-03,
  UC-OFFL-01).** Actor: assigned driver. Main flow: at the yard with signal
  "Atualizar missões" → the inbox lists the missions with title, window, plan
  (destination, reason, load) and "Atualizadas agora" → later, offline, opens
  one `ASSIGNED` mission → "Aceitar" → "Resposta salva no aparelho"; the age
  keeps growing; "Atualizar missões" says it needs a connection → back in
  coverage the response is sent alone and shows "Aceita pela empresa".
  Alternatives: "Recusar" asks for the reason; the planner reassigned the
  mission meanwhile → the answer stays unconfirmed and, after a refresh, the
  inbox says the mission is no longer assigned to the member; another
  organization's inbox never shows it.
- **BUC-EMP-271 — Execute a planned trip.** Actor: the same driver. Main flow:
  the acceptance confirmed → "Iniciar viagem da missão" → the picker offers
  the mission's vehicle → departure reading → the trip starts with its mission
  → "Pausar" and "Retomar" with optional reasons → events → "Encerrar viagem"
  → delivery; a refresh shows the mission `IN_PROGRESS` and, after the finish,
  `COMPLETED`. Alternatives: "Cancelar viagem" with a reason (the vehicle is
  free on the device; the mission stays as the server keeps it); a stale
  command held `STALE_REVISION` with its reason.
- **BUC-EMP-272 — Prove the inbox.** The shared operation oracle
  `trip/trip-operations-v3.psv` (every EMP-TRIP-019 v2 row plus the mission
  reference, alone or beside the departure run, the lifecycle commands and the
  response) produced byte for byte by the Kotlin
  builders and accepted or refused identically by the Go intake parsers; host
  tests for the inbox codec, age and eligibility matrix, the producers, the
  isolated delivery of responses, the gateway, the protected stores and the
  flow; and native acceptance on the owned API 36 emulator against an
  ordinary PostGIS signed intake and company read: refresh, offline answer and
  process death, organization switch, reconnection, a planned trip linked and
  completed, and a reassigned mission whose answer is denied without holding
  any other record.

## Navigation phase 1: open the existing in-app route and guidance for a trip destination (2026-11-16)

Evidence.

- **B-BR-EMP-532 — Only the approved destination of the member's own
  mission or trip is handed to the app's own route.** "Navegar" exists on a
  mission of the partition's missions copy (EMP-TRIP-021) and on a trip of the
  partition's trip book whose start names that mission (EMP-TRIP-023
  reference). It hands over exactly one thing: the free-text destination of
  that mission's current plan (EMP-TRIP-009), as the query of the existing
  in-app route preview's destination search (`MapRouteTarget.Address`), where
  the driver confirms a search result before any route is computed by the
  existing on-device engine (BR-ROUTE-009). No coordinate is guessed or
  parsed from the text, no other trip's or mission's destination is ever used,
  and a trip or mission that is not in the visible partition's own records
  opens nothing (wrong trip).
- **B-BR-EMP-533 — Unsupported destinations say so and open nothing.** A plan
  whose destination is a company unit (the device holds neither the unit's
  name nor its location), a mission without a plan, a trip that executes no
  mission and a trip whose mission is no longer in the copy offer no
  "Navegar" and show why; nothing is opened, searched or recorded.
- **B-BR-EMP-534 — Guidance alone records and sends nothing.** Opening the
  route preview or guidance from a mission or trip writes no operation, starts,
  stops or pauses no tracking session, changes no trip or mission state and
  sends no position to the server (BR-ROUTE-011, BR-ROUTE-015); the screen says
  that this navigation is not recorded on the trip. The trip stays in the
  partition's book and is reopened from the company screen.
- **B-BR-EMP-535 — No external hand-off.** The mission and trip screens never
  send a destination or a position to an external maps application: no
  external intent, `geo:` or navigation URI is built; the hand-off is the app's
  own map tab (BR-ROUTE-012). A failure to open it leaves the planned flow as
  it was.
- **BUC-EMP-273 — Navigate to a planned trip's destination (UC-ROUTE-03,
  SCR-ROUTE-06).** Actor: assigned driver with the mission's trip in
  progress. Main flow: open the trip → "Navegar" (with the notice that this
  navigation is not recorded) → the app's map opens its route preview with the
  plan's destination as the search query → the driver picks the result → the
  existing engine computes the route with the downloaded map region and
  guidance starts. Alternatives: the destination is a company unit, the
  mission has no plan or the trip has no mission → no "Navegar", with the
  reason; guidance active elsewhere → the map's own "stop the current route
  first" message.
- **BUC-EMP-274 — Prove that navigation is local.** Pure tests of the
  destination resolution (own record, text only, unit and absent
  destinations, wrong trip); a source guard that the enterprise mission/trip
  screens build no external intent and reference no tracking or network
  port; presentation device tests that "Navegar" hands exactly the plan text
  to the in-app map and calls nothing else; and native acceptance on the
  owned API 36 emulator through the real app map: the route preview opens
  with the destination query while the enterprise queue, trip book and
  intake stay unchanged.

## Sync resilience suite: duplicates, reordering, partial uploads, process death (2026-11-16)

Evidence.

- **B-BR-EMP-540 — One effect and one acknowledgement under any delivery.**
  For every real signed operation of the batch (`trip.mission.respond`,
  `check.run.complete`, `trip.start` naming its mission and its departure
  run, `trip.event.record`, `meter.reading.record`, `fueling.submit`,
  `expense.submit`, `trip.finish`, `trip.submit`) the server keeps exactly
  one claim, one effect and one final acknowledgement per (organization,
  operation identifier) whatever the network does: a resend after a lost
  answer, a resend of the whole device queue after the device process was
  killed, sequential resends and concurrent resends in device order and in
  reverse order all return the stored acknowledgement byte for byte and
  change nothing. The same identifier with another payload digest is
  `REJECTED` `OPERATION_ID_REUSED`, stores nothing and leaves the original
  acknowledgement and effect unchanged. A `RETRY` answer is never a stored
  acknowledgement: a later delivery decides that operation afresh.
- **B-BR-EMP-541 — Reordering converges; references wait on the device.**
  Delivering the same operations with every declared dependency reversed
  answers each dependent operation `RETRY` `DEPENDENCY_PENDING` with no
  receipt, audit or effect, and after redelivery the organization's final
  state equals the in-order state: the same result, code and revision per
  operation and the same effects (trip, logbook, finish, review submission,
  readings, fueling with its one cost, direct expense, checklist run,
  checklist and mission links, mission state, review items), compared
  without server times and generated identifiers. A soft reference names
  its target's entity, not an operation, and the server holds a reference
  whose target is not stored yet `ACCEPTED_FOR_REVIEW` `ORPHAN_REFERENCE`
  with nothing linked (BR-OFFL-007). The device therefore never sends an
  operation before the final receipt of another unacknowledged operation of
  the same partition whose entity one of its soft references names (the
  departure or return checklist run of the trip start or finish naming it);
  an operation caught in a cycle through such references, or waiting on an
  operation that needs attention, needs attention instead of being retried
  forever.
- **B-BR-EMP-542 — Process death and missing answers lose nothing.** An
  operation the device reports saved survives the death of its process with
  its exact bytes; delivery after a restart is at least once and its effect
  once. A lost answer (the server committed, the device never read it), an
  answer with an acknowledgement missing, and a whole-request refusal leave
  every sent operation pending on the device with no acknowledgement
  applied; an acknowledgement is applied only from a complete answer that
  matches the sent operations, and applying it again projects nothing more.
- **B-BR-EMP-543 — Partial uploads and foreign scope.** A truncated or
  altered receipt transfer (fueling and expense receipts) is refused, keeps
  the document `UPLOADING` at its storage revision, and becomes `STORED`
  only when a retry delivers exactly the declared size and SHA-256; a repeat
  after `STORED` stores nothing more. Another organization never receives,
  applies or reveals state of this one: operations of A posted to B's route
  are refused as a whole and store nothing in either; B's own operation
  carrying A's identifier gets its own acknowledgement while A's stays
  byte-identical; on the device an acknowledgement of A's operation is never
  applied in B's partition and B's delivery never sends A's operations.
- **BUC-EMP-277 — A field day under a bad network (server).** Bruno's phone
  delivers a mission acceptance, a departure checklist, the trip start naming
  both, two logbook events, a reading, a fueling, a toll, the finish and the
  submission. The first answer is lost; the phone restarts and resends its
  whole queue, then resends it again while other copies race in both orders;
  each receipt upload is cut halfway or altered before it succeeds. The
  company ends with exactly the records of an undisturbed day, and the other
  organization of the same account has none of them.
- **BUC-EMP-278 — A field day under a bad network (device).** The same kinds
  of operations, saved offline through the product producers, survive a
  killed process; the queue sends the checklist before the trip that names it
  even when the checklist is in backoff, keeps everything "Salvo no
  aparelho" across a lost or incomplete answer, and applies each receipt
  once. Native acceptance on the emulator is owed while it is locked.

## Tracking interruptions: permission, approximate-only, reboot, process death (2026-11-16)

Evidence. Builds
on EMP-GPS-002 (the service, its session operations and the recovery at the
next opening of the trip screen) and EMP-GPS-003 (the sealed buffer, the
batches and the fix-level gaps); owner decision EDR-271. No server change:
every cause used here is already accepted by the EMP-GPS-004 intake.

- **B-BR-EMP-544 — Every interruption is recorded with its cause and stays
  visible.** A recording session that this process does not collect for is
  interrupted, never left shown as recording: at the next opening of the trip
  screen (EMP-GPS-002) and also as soon as the tracking service, after
  consuming the visible screen's grant, cannot listen for it (no permission,
  hidden notification, platform refusal, another session already listening)
  or cannot show its foreground notification. The interruption carries the
  last proof of recording as its instant (the checkpoint, the last transition
  or one millisecond after the last buffered fix) and the cause the device
  observes, in this order: `DEVICE_RESTARTED` when the checkpoint or the buffer
  names another boot, `PERMISSION_REVOKED` when no location permission is
  granted now, `NOTIFICATION_BLOCKED` when the tracking notification cannot be
  seen now, otherwise `PROCESS_STOPPED` (said as "the collection stopped on
  this phone without your command"). A precision reduction found after a
  process death is that `PROCESS_STOPPED` interruption; the resume then
  records the approximate grant (no reduction is invented for a period nobody
  recorded). The trip screen lists every interruption of the session with its
  cause, start and end (open until the driver resumes or the trip ends); the
  ongoing notification says when location is off, the signal is lost or only
  simulated locations arrive, and that nothing is recorded meanwhile.
- **B-BR-EMP-545 — No interruption is bridged.** Fixes recorded in one
  recording run (from a start or resume to the next interruption, suspension
  or end; a precision reduction does not end a run) are sealed into batches
  apart from the fixes of any other run, even when the seal at the
  interruption failed and both runs' fixes wait in the buffer together; each
  batch depends on the opening of its own run. A fix-level gap still open when
  its run ended is closed at that run's end, never carried into the next run.
  No position, path or distance is derived across an interruption or a gap
  (BR-GPS-024).
- **B-BR-EMP-546 — Approximate fixes never claim precision.** A fix is
  recorded with the lower of its recording period's precision and the location
  permission the platform grants at that report; a report observed without any
  location permission is not recorded. The first approximate report inside a
  precise period writes the reduction (`PRECISION_REDUCED`) at that report's
  instant before the fix is recorded, and a service that begins listening
  with only approximate permission for a precise period writes the reduction
  first, so every coarse-flagged fix lies in an approximate period and every
  precise-flagged fix in a precise one. A raise to precise waits for the
  driver's next resume (EDR-275). The trip screen says, for as long as the
  session lasts, that no GPS distance is derived from its approximate parts.
- **B-BR-EMP-547 — Resumption needs the driver's visible tap and current
  authority; the buffer survives every interruption.** Nothing resumes by
  itself: no recovery, start intent without the screen's fresh grant, worker,
  notification, boot or server path starts collection. A start or resume tap
  writes only when this account still holds the partition, the context's
  membership is the enrollment's, the trip is the member's own open trip on
  this device, the session is neither refused nor ended, the latest policy
  read on the device is `OPTIONAL` or `EXPECTED` (with a connection the screen
  reads the policy in force first; a failed read keeps the last value and says
  so; `OFF` or never read refuses with nothing written) and no other session
  is being collected on this phone; only then does the screen arm the
  one-time grant. No interruption, recovery, reduction, owner change or failed
  write removes, rebinds or duplicates a recorded fix: each stays pending in
  its owner's sealed buffer or leaves in exactly one batch.
- **BUC-EMP-279 — Recover from an interruption.** The phone restarted, the
  app was closed or the location permission was removed while tracking: at the
  next opening of the trip the session shows "Rastreamento interrompido" with
  the reason and since when, the waiting positions leave in batches, and
  "Retomar rastreamento" resumes only after the driver's tap, with the policy
  re-checked; declining keeps the trip untracked and the interval stays a gap
  for the company.
- **BUC-EMP-280 — Track with approximate location.** The driver grants only
  approximate location, or reduces it during the trip: the session records
  coarse positions at the heartbeat rate inside an approximate period, the
  screen says that no GPS distance is derived from those parts, and precise
  recording returns only through a stop and a resume with precise permission.

## App: tracking transparency (indicator, what is collected, session history) (2026-11-16)

Evidence. Builds on
EMP-GPS-002 (the service and its notification), EMP-GPS-003 (the sealed
buffer) and EMP-GPS-010 (interruptions); owner decision EDR-271. No server
change: no driver-facing read of one's own sessions exists, so the history is
the phone's own sealed records.

- **B-BR-EMP-548 — One indicator on every enterprise screen, only while this
  phone collects.** While this phone's organization collector listens, the
  header shared by every enterprise screen (the home and the trips, fuelings,
  expenses, assets, checklists, sync and "Meu rastreamento" panels) shows one
  indicator in text and icon, never colour alone: "Rastreamento da empresa
  ativo" with the organization's name, since when, precise or approximate,
  that the position is this phone's and not the vehicle's, and an open
  location-off, no-signal or simulated-location gap; a tap opens "Meu
  rastreamento". It is derived only from the process's running organization
  collector, never from a stored session alone and never from the personal
  recorder; it appears when the listener attaches and disappears when it is
  released. On the trip screen a session stored as recording that this phone is
  not collecting says so instead of looking active, until the next opening
  records the interruption (B-BR-EMP-544). The ongoing notification stays the
  indicator outside the app (EMP-GPS-002/010).
- **B-BR-EMP-549 — "Meu rastreamento" shows what is collected and one's own
  sessions, from this phone only.** Any member in enterprise mode can open it
  offline from the selected organization's sealed tracking book and buffer:
  what is collected and when, what is not collected, who can see it, the
  default retention, what stays on this phone, the policy and notice version
  last read, whether this phone collects now, and the member's own sessions on
  this phone, newest first, each with its vehicle, tracking start, end or "em
  andamento", interruptions with cause, start and end, location gaps with
  reason, approximate parts, and positions waiting or batches accepted and still
  on the phone; never a coordinate, never another member's or another
  organization's session (the book is bound to this membership and
  enrollment). After a stop or a finish nothing more is recorded for that
  session and its end is shown. Empty: "Nenhuma viagem rastreada"; the list says
  it is this phone's.
- **B-BR-EMP-550 — Organization and personal tracking are never mixed up.**
  Organization tracking is always labelled with the organization's name and as
  this phone's position, in enterprise screens and its own notification
  channel only; the personal Abastevo recorder ("Gravar percurso") and the
  driver suite never feed the enterprise indicator or "Meu rastreamento", and
  the organization collector never feeds the personal recorder's state or its
  saved tracks.
- **B-BR-EMP-551 — No false promise about erasure.** The transparency copy says
  that positions stay protected on this phone until sent; that after a device
  revocation or leaving the organization this phone's copy is erased when the
  app next reaches the server; that a phone without a connection is not erased
  remotely; and that what the organization received follows its retention
  (raw positions 90 days after the trip by default, 30 to 365 by organization
  choice). No tracking text in any locale promises remote or immediate erasure
  of a disconnected phone or deletion of what the organization received
  (BR-PRIV-018, BR-OFFL-014).
- **BUC-EMP-281 — See my tracking.** In enterprise mode the member opens "Meu
  rastreamento" from the home or from the indicator, reads what is collected,
  sees whether this phone collects now and goes through their own sessions on
  this phone with their interruptions and gaps, offline.
- **BUC-EMP-282 — Know from any screen that tracking is on.** While a trip is
  tracked, the member moves between the enterprise screens and the indicator
  stays visible on each, naming the organization and since when; when tracking
  stops, ends or is interrupted it disappears at once.

## M4 gate: trips, logbook, sync and tracking fundamentals acceptance (2026-11-16)

Evidence. A TEST gate over the
integrated F4 head: it adds no behaviour and no test of its own
([15-validation](testing.md) §1); it runs the owners'
scenario tests as one command set, maps them to VAL-04, VAL-22 (part 2),
VAL-23, VAL-26, VAL-27, VAL-28 and the M4 exit criteria, names what is owed and
decides. Owner decision EDR-271 stands; the physical phone attached to this
machine is never used and the owned emulator is never unlocked, wiped or
replaced by an agent.

- **B-BR-EMP-552 — M4 is accepted only when every required case passes,
  device coverage included.** The M4 gate records a pass only when, on one
  named revision, VAL-04, VAL-22 part 2, VAL-23, VAL-26, VAL-27 and VAL-28
  pass as written, VAL-02 and VAL-21 pass over the routes and permissions F4
  added, the entry conditions hold (M3 accepted through EMP-REL-004;
  EMP-GOV-005 and EMP-GOV-006 done) and the tracking interruption matrix has
  run on real phones of at least three manufacturers (screen off, app swiped
  away, reboot, force stop, battery saver, Doze). A device stage that was not
  run, was skipped, was only compiled or ran only on an emulator is never
  recorded as a pass, and a host-JVM or PostGIS proof of a device rule is named
  as a host proof. A scenario clause that no owner produces yet (for example a
  fueling held for review after an offboarding or a suspension before
  EMP-FUEL-003) is recorded as not proven with its owner, never as proven by a
  neighbouring operation.
- **B-BR-EMP-553 — The gate's negative cases are pinned to named evidence.**
  "Silent queue loss" is answered by the tests that keep every saved operation
  pending across process death, lost or incomplete answers and an ended
  membership until a final receipt or a verified disposal (B-BR-EMP-542,
  EMP-OFFL-011); "location resumed without authority" by the tests that resume
  collection only from the driver's visible tap after a fresh policy and
  membership check, never from a restart, a stale intent or a new grant
  (B-BR-EMP-547, EMP-GPS-001 FOLLOW, the mobile guard refusing background
  location, boot receivers and sticky starts); "private fixtures committed" by
  a scan of the phase diff for secrets, personal identifiers, photos and
  coordinates recorded with the gate; "missing device coverage called pass" by
  B-BR-EMP-552 and the device ledger of the record.
- **B-BR-EMP-554 — Tracking stays held while the device matrix is owed.**
  Until a recorded M4 gate pass includes the real-device interruption matrix,
  tracking collects from no real employee of any organization: the tracking
  policy of every real organization stays absent or `OFF`, and `OPTIONAL` or
  `EXPECTED` is set only in synthetic validation organizations. EDR-271's
  authorization (tracking implemented for real and prepared for production,
  synthetic validation, activation per organization only with its employee
  notice and terms in force) is unchanged; its per-organization activation for
  real employees waits on this gate's device evidence or on a new explicit
  owner decision recorded as an EDR that names the accepted risk.
- **B-BR-EMP-555 — A gate record states what was run and what is owed.** The
  record names the tested revision, the exact commands, top-level and named
  test counts, every failure with its rerun (a failure seen only under load is
  rerun alone and reported, a genuine failure is fixed with a failing test
  first, never weakened), the clauses not proven with their owners, and every
  owed device acceptance with who provides it and its prepared one-line
  driver. A gate run is LOCAL_DONE / INTEGRATION_PENDING evidence at most; it
  never claims F4 or M4 complete, INTEGRATED or RELEASE_CERTIFIED, and while M4
  is not accepted the gate task is `BLOCKED` naming its blockers.
- **BUC-EMP-283 — Run the M4 gate.** An engineer runs the documented scenario
  union on the integrated F4 head (real PostGIS with a throwaway object store,
  the Kotlin host suites, the mobile guard), maps each VAL item and each M4
  exit criterion to named tests, scans the phase diff for private data, lists
  the owed device evidence with its drivers and records the decision.
- **BUC-EMP-284 — Supply the owed device evidence.** The owner unlocks the
  owned emulator once so the prepared native drivers can run, and the owner or
  pilot users run the interruption matrix on phones of at least three
  manufacturers that they consent to use, with a synthetic organization and
  synthetic trips; each result is recorded per device and case, and the gate
  is run again on the head that includes them.

## Runtime role, grant inventory and staging activation runbook (2026-11-16)

Evidence. A prerequisite
of the scoped activation that EMP-REL-002 rehearses: the owner chose to turn
Abastevo Empresas on in the VPS environment (EDR-272) and the organization
role guard (`requireSafeOrgRole`) refuses the superuser login that the API
and worker use there today, so every organization operation would answer
unavailable. This delivery adds the ordinary runtime login, its reviewed
grant inventory, the proof that the whole composition works under it, the
deployment wiring and the activation runbook with rollback. It does not
activate anything, provision a live role or certify a release; EMP-REL-001
and the VAL-24 rehearsal stay owed.

- **B-BR-EMP-556 — The API and the worker log in as an ordinary runtime role
  that owns nothing.** The production API and worker connect as one login
  (`abastevo_runtime` on the VPS) created `LOGIN NOSUPERUSER NOBYPASSRLS
  NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION`, a member of no role and the
  owner of no object, so the organization role guard admits it and forced RLS
  applies to it. The migration owner (the login that runs migrations, the
  pre-deploy backup, the schema probe and the registry job) is never the
  runtime login and stays refused by the guard. No password, DSN or secret
  value is ever stored in Git or printed: the password comes from a secret at
  apply time.
- **B-BR-EMP-557 — The runtime grant inventory is derived, reviewed and
  converged by one idempotent script.** `infra/db/runtime-role.sql` grants,
  per relation of the head schema, only `SELECT`, `INSERT`, `UPDATE` and
  `DELETE` as the production binaries need them: what the SQL of the `cmd/api`
  and `cmd/worker` build closure does (an `UPDATE` also for a row lock or an
  `ON CONFLICT … DO UPDATE`), plus what the head's SECURITY INVOKER functions,
  RLS policies, column defaults, checks and views do when they run as that
  login, plus a reviewed exception list. It never grants `TRUNCATE`,
  `REFERENCES`, `TRIGGER`, `MAINTAIN`, `CREATE`, ownership, role membership or
  `EXECUTE` on a SECURITY DEFINER function, and never re-opens a function a
  migration closed to operators (the meter projection rebuild stays
  operator-only). It runs as the migration owner in one transaction, refuses a
  database at another schema version, a role that owns something or is a
  member of a role, and the owner itself, converges a widened role back to the
  inventory and verifies the result before committing. The checked-in script
  must equal the generator's output for the migrated head.
- **B-BR-EMP-558 — Deployment keeps the owner DSN for schema work and gives
  the runtime DSN only to the serving containers.** The API and worker
  containers read `ANPFUEL_DATABASE_URL` from the secret key
  `ANPFUEL_RUNTIME_DATABASE_URL`; the secret's `ANPFUEL_DATABASE_URL` stays the
  owner DSN that the deploy agent's backup, schema and migration Jobs and the
  registry job read, and is never removed. `ANPFUEL_ENTERPRISE_ENABLED=true` is
  set on both containers together. Objects that a later migration creates get
  ordinary DML for the runtime role by default until the script of the
  deployed release is applied again, which converges them to the derived
  inventory; the script is applied again after every release whose script
  differs from the previous one.
- **B-BR-EMP-559 — Activation is scoped, reversible and preserves data.** The
  VPS activation happens only after the automatic deploy of the release that
  carries the script, a verified backup and the owner's final confirmation,
  while the deploy agent is held off by its own lock. Verification uses
  synthetic organizations only (the only accepted terms are the synthetic DPA)
  and tracking stays absent or `OFF` for every real organization (EDR-362).
  Rollback removes the two container settings and rolls out again: the routes
  stop being served, every stored row stays, and the role, its grants and the
  new secret key are additive and may remain.
- **BUC-EMP-285 — Provision the runtime login and activate on the VPS.** The
  coordinator, holding the deploy agent's lock, records the current revision,
  takes and verifies a backup with the agent's backup Job, applies the
  release's runtime script as the owner, sets the password from a newly
  generated secret with statement logging off for that session, stores the
  runtime DSN in the new secret key, sets both containers' settings, waits for
  the rollout and runs the smoke: readiness, public reads, the enterprise
  routes answering an unauthenticated caller with `401 UNAUTHENTICATED`, a
  synthetic organization created and read, a capability-gated write answering
  `ENTITLEMENT_REQUIRED`, and no `permission denied` in the database log.
- **BUC-EMP-286 — Roll back the activation.** On any failed step after the
  settings change, or on the owner's request, the coordinator removes the two
  container settings with a delete patch (images untouched) and waits for the
  rollout; readiness and public reads return to the owner login, the
  organization routes answer `404`, and nothing stored is deleted.

## Operator capability command (2026-11-16)

Evidence.
A follow-up of the runtime-role delivery above: the audited capability store
(EMP-PLAT-005) had no production caller, so every organization stayed
capability-off and its writes answered `ENTITLEMENT_REQUIRED`. This adds the
operator path; it does not enable anything by itself.

- **B-BR-EMP-560 — An operator changes an organization's capability only
  through the audited capability service.** `ops enterprise capability set`
  requires the operator identity as a canonical UUID (`--operator` or
  `ANPFUEL_OPERATOR_ID`), the organization UUID, a registered capability code
  (`organizations`), exactly one of `--enabled` or `--disabled`, a canonical
  operation ID and the expected revision (0 for the first state). One change
  writes the state and one audit event (`capability.enabled` or
  `capability.disabled`, reason `operator.request`, actor = the operator)
  atomically; the same operation with the same command is acknowledged as a
  replay (with the current state) and writes nothing; the same operation with another
  command is refused as a reused operation; a stale expected revision is
  refused as a conflict with nothing changed; malformed input or an
  unregistered capability is refused before any database work; an unknown
  organization is refused as not found with nothing written. The command
  never runs raw SQL.
- **B-BR-EMP-561 — The command runs only as the ordinary runtime login.** It
  opens the database named by `ANPFUEL_DATABASE_URL` in its environment —
  inside the `api` container after activation that is the runtime DSN — and
  refuses, before any read or write, a login that the organization role guard
  refuses (the owner or any superuser, BYPASSRLS or table-owning login),
  saying why. `ops enterprise capability show` reads the current state
  (enabled and revision; no row reads as disabled at revision 0) through the
  same service and the same refusal.
- **B-BR-EMP-562 — The production API image ships the operator tool, which
  grants nothing else.** The `api` image carries `/ops`. A capability change
  is independent of `ANPFUEL_ENTERPRISE_ENABLED` (it may precede or follow the
  switch), mounts no route, assigns no role and changes no membership;
  disabling keeps every row and every authorized read and export
  (BR-AUTH-009, VAL-24). Output names only the organization, the capability,
  the state, the revision and whether the answer is a replay.
- **BUC-EMP-287 — Turn a capability on or off for one organization.** The
  coordinator runs `show` inside the `api` container, then `set` with a new
  operation ID and the revision read, checks that a capability-gated write
  now succeeds (or answers `ENTITLEMENT_REQUIRED` after disabling) and keeps
  the operation ID, the resulting revision and the operator identity in the
  receipt; a retry of the same command is safe.

### F5 Live Navigation, Operations, Geofences and Routing (2026-11-18)

- **B-BR-EMP-563 — Panel map and routing abstraction (`EMP-GOV-007`, `EMP-ROUTE-001/002/010`).** `business-web` isolates live markers, replay polylines, and geofence shapes behind `MapPort` with a default vector canvas implementation and complete accessible table/list fallback requiring zero external map-tile SaaS calls. Android routes strictly on-device through `EnterpriseRoutingPort` (`CAR` profile, `© OpenStreetMap contributors` attribution) with `isServerRoutingUsed = false` in F5.
- **B-BR-EMP-564 — Live supervision and route permissions (`org-permissions-v6`).** `gps.location.read_live` grants `OWNER`, `ADMIN`, `FLEET_MANAGER`, `SUPERVISOR` (`O`) and `DRIVER` (`S`, own trips only), while `AUDITOR` is explicitly refused (`FORBIDDEN` naming `gps.location.read_live`) and retains only recorded replay (`gps.track.read`). `gps.geofence.manage` and `route.plan.manage` grant `OWNER`, `ADMIN`, `FLEET_MANAGER`, `SUPERVISOR` (`O`).
- **B-BR-EMP-565 — Monotonic live positions, read-time freshness and 24-hour expiry (`EMP-GPS-007/008`).** Each tracked trip projects one monotonic row in `org_tracking_live_positions`; older gap fixes in later batches never rewind `lat_e7`, `lon_e7`, or `collected_at`, while `last_communication_at` advances monotonically. Freshness (`CURRENT` / `STALE` / `NO_SIGNAL`) and 24-hour post-trip expiry (`NOT_FOUND` on single read, hidden on list) are computed at read time, and every served live/replay read writes a coordinate-free audit event (`LOCATION_LIVE`, `LOCATION_TRACK`).
- **B-BR-EMP-566 — Raw segment retention, purge receipts and end-trimming (`EMP-GPS-009/011`).** Expired raw segments and live rows are removed by `org-tracking-retention` using insert-only `org_tracking_purge_receipts` (`RETENTION_EXPIRED` / `TRIMMED_AFTER_END`) while preserving `org_tracking_summaries`; expired raw tracks answer `raw_expired = true` and `404`/`INSUFFICIENT` geometry without reconstructing deleted points.
- **B-BR-EMP-567 — Server-side geofences and operational alerts (`EMP-GPS-013/014`).** Circle (`50..50 000 m`) and non-self-intersecting polygon (`3..32` vertices; self-intersections rejected `422 GEOFENCE_INVALID_GEOMETRY`) geofences plus stale-tracker and long-stop alerts are evaluated server-side only on non-coarse precise fixes (`accuracy <= 50 m`); late offline batches record historical events with `late_suppressed = true` without triggering false live notifications.
- **B-BR-EMP-568 — Versioned route plans, driving safety and trip-aware leases (`EMP-GPS-006`, `EMP-ROUTE-003..009`).** Route plans enforce `<= 102` waypoints (`422 ROUTE_POINT_LIMIT`), handle `TEXT_ONLY` unlocated stops explicitly, keep `CAR` heavy-vehicle disclaimers prominent, queue sync plan updates while `IN_PROGRESS` until stopped, propose `"Cheguei"` arrival confirmation manually without auto-finishing, enforce `>= 1.5 m/s` driving-mode restrictions (`<= 4` quick actions), and share one hardware location lease between `TRACKING` and `GUIDANCE`.
- **B-BR-EMP-569 — Neutral detour fuel-station evaluation (`EMP-ROUTE-008`, §10.7).** Candidate fuel stations compute detour fuel, detour cost, and net savings via exact `BigInteger`/integer half-up arithmetic, refuse incomparable price sources/products/units/payment conditions/stale prices/missing consumption, and rank solely on comparable economic metrics without `has_paid_plan` or `has_active_agreement` weighting.
- **B-BR-EMP-570 — M5 gate state (`EMP-REL-006`).** Local `VAL-16`, `VAL-21` (`org-permissions-v6`), `VAL-29`, `VAL-30`, and `EMP-ROUTE-010` replacement drills pass on F5 backend, `business-web`, and Android codebases; physical multi-manufacturer M4 device testing and VPS activation remain explicit pre-release obligations.
- **BUC-EMP-288 — Swap the panel map or routing adapter (`EMP-GOV-007`, `EMP-ROUTE-010`).** An engineer replaces the `MapPort` implementation in `business-web` or `EnterpriseRoutingPort` in Android with a synthetic adapter and confirms attribution, fallback views, and trip semantics remain unchanged.
- **BUC-EMP-289 — Supervise live operations and replay recorded tracks (`VAL-16`).** A supervisor inspects current, stale, and paused trips on list and map views (`LOCATION_LIVE` audited without coordinates) and opens post-trip replay with explicit `collected_at` / `received_at` timestamps and gap indicators; an auditor reads replay (`LOCATION_TRACK`) and is denied live views (`FORBIDDEN`).
- **BUC-EMP-290 — Enforce raw track retention and post-trip live expiration (`EMP-GPS-011`).** Twenty-four hours after trip end the live endpoint returns `404 NOT_FOUND`, and once the retention window elapses raw segments purge with durable `org_tracking_purge_receipts` while summaries remain intact.
- **BUC-EMP-291 — Configure geofences and suppress late/coarse noise (`VAL-30`).** A manager creates circle and polygon geofences, rejects invalid geometries (`GEOFENCE_INVALID_GEOMETRY`), verifies precise fixes generate entry/exit alerts, coarse fixes do not trigger crossings, and late batches suppress live alerts.
- **BUC-EMP-292 — Execute trip-aware offline navigation (`VAL-29`).** A driver navigates a multi-stop route offline with a single location lease, off-route recalculations keep the planned distance unchanged, driving mode locks detailed entry at `>= 1.5 m/s`, plan revisions wait until stopped, and stops complete only on explicit `"Cheguei"` confirmation.
- **BUC-EMP-293 — Evaluate neutral fuel-station detours along a route (§10.7).** A driver or planner compares nearby stations sharing identical source/payment/freshness terms and observes exact detour net cost (`1 742` centavos in §10.7) with paid plans gaining zero ranking boost.

### F6 Management, Maintenance, Internal Fuel Stock, Intelligence, Reporting and Governance (2026-11-18)

- **B-BR-EMP-571 — Management and intelligence permission catalog (`org-permissions-v7`, `EDR-375`).** Preserves every `v6` cell and all 13 fixed roles (`DATA_MANAGER`, `STATION_*` denied on ordinary company routes) and activates 11 F6 routed keys (`stock.tank.manage`, `stock.movement.record`, `maint.plan.manage`, `maint.workorder.manage`, `maint.workorder.review`, `maint.request.create` with `DRIVER`/`OPERATIONS_STAFF` `SELF` scope `S`, `kpi.dashboard.read`, `find.finding.read`, `find.finding.resolve`, `audit.event.read`, `org.group.manage` fresh-auth), yielding `44` routed permissions (`440` matrix cells: `174` allowed, `266` denied) and `195` routed endpoints (`37` `AUDITOR` positive reads/own consent/export request, `158` denied `FORBIDDEN`).
- **B-BR-EMP-572 — Exact internal tank moving-average valuation (`EMP-STOCK-001/002/004`, `VAL-32`).** Each active tank binds to one product and compatible unit (`L`, `M3`, `KWH`); receipts, dispenses, transfers, adjustments, and reconciliations serialize row locks on `org_fuel_tanks` (`MOVING_AVG_UNIT_V1`, `BigInt` `HALF_UP` rounding), reject incompatible products (`422 STOCK_PRODUCT_MISMATCH`) and non-negative overdrafts (`409 STOCK_INSUFFICIENT`), exhaust exact residual cents when stock hits zero, and conserve value across transfers while requiring reasons on reconciliations/adjustments.
- **B-BR-EMP-573 — Single-count cost recognition across fuel stock (`EMP-STOCK-003/005`, `VAL-32`).** Fuel stock receipts change tank quantity and valuation with zero operating expense; dispensing to an active asset atomically creates one append-only movement, one `org_internal_fuelings` record, and one `org_operating_cost_entries` row (`INTERNAL_STOCK_DISPENSE`, category `FUEL`), preventing double-counting purchase plus usage.
- **B-BR-EMP-574 — Multi-dimension preventive maintenance & projected due dates (`EMP-MAINT-001/002`, `VAL-31`).** Plans require an in-service odometer or hour meter (`org_meters.removed_at IS NULL`, else `422 METER_REQUIRED`) when distance or hour triggers are configured; evaluation computes `UPCOMING`/`DUE`/`OVERDUE` from the earliest trigger, supports `ACTUAL` vs `SCHEDULED` rebase, keeps at most one open occurrence per plan, and projects ceiling due dates upon corrected readings.
- **B-BR-EMP-575 — Asset expiries, corrective requests and work-order lifecycle (`EMP-MAINT-003/004/006`, `EMP-FLEET-010`).** Asset document links record declared expiry days without copying content; corrective requests (`maint.request.create` `S` on own requests) advance through quotes, SoD review (`409 SELF_APPROVAL`, `409 QUOTE_EXPIRED`), scheduling, execution, invoice checks (`409 INVOICE_REQUIRED`), tolerance overrun guards (`409 OVERRUN_NOT_APPROVED`), closed finance period checks (`LOCKED` on `org_finance_periods`), and one-time `WORK_ORDER` posting to `org_operating_cost_entries`.
- **B-BR-EMP-576 — Live maintenance availability holds (`EMP-MAINT-005`).** Starting or holding an active work order places the asset in live `IN_MAINTENANCE` with zero `missing_sources` on `/fleet/assets/{asset_id}/availability/read`; explicit hold release restores `AVAILABLE` while merging overlapping unavailability intervals second-for-second.
- **B-BR-EMP-577 — Document & driver licence expiry controls (`EMP-MAINT-007`).** Driver licence compliance items cannot be written while `DRIVER_LICENCE_CONTROL` is disabled (`409 CONTROL_DISABLED`); asset compliance and enabled driver licence items evaluate deterministically as `VALID`, `DUE_SOON`, `EXPIRED`, or `CONTROL_DISABLED`.
- **B-BR-EMP-578 — Components, parts stock & insurance extensions (`EMP-MAINT-010..012`).** Tracks asset components (serial/position/mileage install/remove history), moving-average workshop parts stock linked to work orders, and fleet insurance policies/incident claim states without embedding financial ledgers inside policy records.
- **B-BR-EMP-579 — Ratio-of-sums fleet KPI registry (`EMP-KPI-001..004`, `EMP-FUEL-009/010`, `VAL-10`, `VAL-11`, `VAL-20`).** Evaluates all 20 indicators (`IND-001..020`, `kpi-v1`) using exact integer ratio-of-sums arithmetic (`km/L`, `L/100km`, `L/h`, cost/km, cost/h, availability `1dp`), returns explicit `INSUFFICIENT_DATA` reasons instead of zeroes on missing denominators or unsupported `CNG`/mixed-fuel groups, labels `< 3` samples `small_sample`, separates pending counts, and allocates closed intervals to trips proportionally (`AllocateIntervalToTrips`).
- **B-BR-EMP-580 — Shared Go and Kotlin F6 domain parity.** `contracts/testdata/enterprise/{stock,maintenance,intelligence}/*.psv` bind identical exact results across Go (`fuelstock/domain`, `maintenance/domain/plan`, `intelligence/domain`) and Android Kotlin (`EnterpriseFuelStock`, `EnterpriseMaintenanceCalculator`, `EnterpriseKpiCalculator`).
- **B-BR-EMP-581 — Non-judgmental price comparisons (`EMP-KPI-005`, `VAL-15`).** `EvaluatePriceComparison` (`price-comparison-v1`) isolates comparable sources (`OFFICIAL`, `COMMUNITY`, `ANNOUNCED`, `NEGOTIATED`), refuses mismatched payment conditions (`CONDITION_MISMATCH`), stale references, or low station coverage, and never emits `"prejuízo"`/`"economia"` claim wording.
- **B-BR-EMP-582 — Bounded findings detection and reprocessing (`EMP-FIND-001..003/005`).** Thresholded detectors (`FT-001..006`) enforce bounded configuration ranges, create at most one open finding per `(finding_type, subject_key, method_version)`, and reprocessing runs (`org_finding_runs`) supersede only undecided findings (`SUPERSEDED`) while leaving terminal decisions intact.
- **B-BR-EMP-583 — Findings workflow & self-resolve separation (`EMP-FIND-004`).** Finding authors cannot take or decide their own findings (`409 SELF_APPROVAL`); decided findings (`CONFIRMED`, `EXPLAINED`, `FALSE_POSITIVE`) require justification and reject subsequent mutations (`409 FINDING_CLOSED`).
- **B-BR-EMP-584 — Recurring/fine costs, payable statements & accounting views (`EMP-EXP-007`, `EMP-FIN-005/006`).** Records recurring/fine obligations (`INTERNAL_STOCK_DISPENSE`/`WORK_ORDER`/`FINE_OR_RECURRING`), groups payables into statements (`CARD_STATEMENT`, `SUPPLIER_INVOICE`), and exports separate `COMPETENCE` and `CASH` accounting CSVs (`fin.accounting_competence.csv`, `fin.accounting_cash.csv`).
- **B-BR-EMP-585 — Standard-library PDF, scheduled & full ZIP reports (`EMP-RPT-003..005`).** Generates deterministic `%PDF-1.4` reports (`rpt.trip_logbook.pdf`, `rpt.fueling_statement.pdf`, `rpt.period_summary.pdf`) without unreviewed PDF packages (`CX-10`), manages recurring schedules (`org_report_schedules`), and produces SHA-256-manifested `org.full_export.zip` bundles enforcing ceiling limits (`422 EXPORT_TOO_LARGE`).
- **B-BR-EMP-586 — Scoped audit trail viewer (`EMP-RPT-006`).** `/audit/events/list` filters organization audit events under `audit.event.read` (`OWNER`, `ADMIN`, `AUDITOR`) without exposing credentials, document contents, or location coordinates.
- **B-BR-EMP-587 — Voluntary fueling price publication (`EMP-PUB-002`).** Opt-in publication writes strictly an allowlisted station/product/unit/price/condition record into `org_voluntary_price_publications` with zero leakage of internal contracts or documents.
- **B-BR-EMP-588 — Retention policies, legal holds, DSR & erasure tombstones (`EMP-PRIV-001/002/004`, `EMP-DOC-012`, `EMP-IDN-019`, `VAL-38`).** Active legal holds (`HOLD_OPEN` 409) block `DELETION_REPLAY`; DSR requests record responsible context (`ORGANIZATION` vs `PLATFORM`); erasure tombstones pseudonymize personal identity linkage without erasing company-owned financial history.
- **B-BR-EMP-589 — Bilateral organization group consolidation (`EMP-ORG-012`).** Requires bilateral proposal/acceptance between organizations before granting strictly read-only dataset visibility.
- **B-BR-EMP-590 — Allowlisted custom roles (`EMP-AUTH-010`).** Custom role bundles validate every permission against the fixed catalog and forbid delegating `org.group.manage` or `org.lifecycle.manage`.
- **B-BR-EMP-591 — Trigger-guarded F6 schema & runtime role grants (`000259`, `infra/db/runtime-role.sql`).** All 26 F6 tables enforce `FORCE ROW LEVEL SECURITY`, validate cross-module references (`org_assets`, `org_units`, `org_cost_centres`) inside BEFORE triggers without structural FKs, avoid mixed `TIMESTAMPTZ`/`DATE` parameter types (`SQLSTATE 42P08`), and match `TestRuntimeScriptMatchesHead`.
- **B-BR-EMP-592 — Company intelligence panel & redacted mobile field views (`business-web`, Android `:data`/`:app`).** `business-web` renders stock, maintenance, KPIs, findings, audit, schedules, and governance panels; Android stores only compliance advisories, field maintenance requests, and offline internal dispense drafts in encrypted vault slots (`f05e0b00-...-00c1/00c2`), hiding all company financial totals from drivers.
- **B-BR-EMP-593 — Measured synthetic capacity baseline (`EMP-REL-010`, `VAL-37`).** `BenchmarkSyntheticFleet1000VehiclesKPIAndFindings` measures `17 115 ns/op` (`1 393 B/op`, `81 allocs/op`) across `1 000` synthetic vehicles on `linux/amd64`; capacity numbers describe measured host behavior and never claim unverified production scale.
- **B-BR-EMP-594 — M6 local gate checkpoint (`EMP-REL-007`).** Verifies `VAL-10`, `VAL-11`, `VAL-15`, `VAL-20`, `VAL-21` (`org-permissions-v7`), `VAL-31`, `VAL-32`, `VAL-37`, and `VAL-38` locally against real PostGIS, disposable MinIO, OpenAPI contracts, `business-web`, and Android host suites while retaining physical M4 device matrix, VPS activation, and production `G09` obligations.
- **BUC-EMP-294 — Verify F6 role containment and auditor read boundaries (`org-permissions-v7`).** Every F6 endpoint admits only authorized ordinary roles (and `maint.request.create` `SELF` ownership) while `AUDITOR` reads KPIs, findings, and audit events (`200`) and receives `403 FORBIDDEN` on every mutation and finding resolution.
- **BUC-EMP-295 — Receive fuel into internal tanks and dispense once at moving average (`VAL-32`).** Manager records receipt (`100 L`, R\$ 600,00 -> zero expense), rejects mismatched product (`422 STOCK_PRODUCT_MISMATCH`) and overdraft (`409 STOCK_INSUFFICIENT`), and dispenses `40 L` (`24 000` centavos recognized once, `60 L` / `36 000` centavos remaining).
- **BUC-EMP-296 — Schedule preventive maintenance and project corrected readings (`VAL-31`).** Manager attaches distance/hour/date triggers (refused `422 METER_REQUIRED` if no in-service meter exists), observes `DUE` state cleanly, and projects updated due dates when a corrected odometer supersedes an errant reading.
- **BUC-EMP-297 — Execute work order lifecycle with SoD, holds, invoice and one-time expense.** Requester creates/quotes work order, self-approval fails (`409 SELF_APPROVAL`), `FINANCE` approves, starting puts asset into `IN_MAINTENANCE`, invoice-verified close posts cost once and releasing hold restores `AVAILABLE`.
- **BUC-EMP-298 — Toggle driver licence compliance control (`CONTROL_DISABLED`).** Disabling `DRIVER_LICENCE_CONTROL` hides employee items and rejects new `EMPLOYEE` compliance writes with `409 CONTROL_DISABLED`.
- **BUC-EMP-299 — Inspect fleet KPI dashboard with insufficient-data honesty (`VAL-10/11/20`).** Manager views all 20 indicators (`IND-001..020`) with exact rational strings (`160/39`, `4.10 km/L`), small-sample badges, pending separations, and explicit insufficiency reasons instead of `0`.
- **BUC-EMP-300 — Compare fuel prices non-judgmentally (`VAL-15`).** Price comparisons compute exact centavo/milli-BRL differences when conditions match and return `INSUFFICIENT_DATA` (`CONDITION_MISMATCH`) otherwise without asserting fraud or loss.
- **BUC-EMP-301 — Detect, review and supersede anomaly findings under SoD.** Detector raises deduplicated `FT-001..006` findings, author self-resolution is blocked (`409 SELF_APPROVAL`), finance confirms/explains with justification, and reprocessing supersedes only open/in-review findings.
- **BUC-EMP-302 — Export competence/cash accounting CSVs, PDF reports and full ZIP (`EMP-FIN-006`, `EMP-RPT-003/005`).** Authorized requester enqueues background jobs, downloads `fin.accounting_competence.csv`, `%PDF-1.4` period summary, and SHA-256 `org.full_export.zip` via single-use capability without exposing storage secrets.
- **BUC-EMP-303 — Audit organization actions without leaking content.** Authorized manager or `AUDITOR` queries `/audit/events/list` across maintenance, stock, findings, and governance events with redacted payloads.
- **BUC-EMP-304 — Enforce legal holds against deletion replay (`VAL-38`).** Recording an active legal hold blocks `DELETION_REPLAY` (`409 HOLD_OPEN`) until released, preserving audit and replay provenance.
- **BUC-EMP-305 — Capture driver compliance alerts and maintenance requests offline.** Driver inspects expiring vehicle/licence alerts and queues a maintenance request or internal dispense offline inside the encrypted vault partition without viewing org financial totals.

- **B-BR-EMP-595 — Phase F7 scope and the recorded Stage-D commerce override (`EDR-386`).** Delivers the maintainer-requested slice: station-announced prices, labelled promotions, bilateral fleet agreements with private negotiated terms, bounded single-use fueling authorizations, the once-only station redemption and cross-organization sharing guards. Station claims, the station portal, official replies, catalog corrections, interaction metrics, statement reconciliation and legal-entity verification stay gated and honestly open.
- **B-BR-EMP-596 — Station-announced prices and promotions are a separate attributed source (`000260`, `EDR-387`, BR-STN-001/002).** One active announcement per station, product and unit supersedes in one guarded statement; reusing a live label withdraws the stored promotion first; history is insert-only and expiry applies at read time. Announced or promoted values never enter official or community consensus, ranking, moderation or trust (VAL-33), and the `price.publish` scope extends only new administrator and manager grants while `attendant` gains only `redemption.confirm`.
- **B-BR-EMP-597 — Fleet-side agreement routing on the organization matrix (`EDR-389`, extension of B-BR-EMP-571).** `agree.agreement.manage` (six endpoints) and `agree.authorization.issue` (four endpoints) route to `46` routed permissions (`460` matrix cells: `182` allowed, `278` denied) and `205` routed endpoints, preserving every catalog cell; station-scope keys stay unrouted with the recorded reason, and station actors act only through their live station grant surface.
- **B-BR-EMP-598 — Agreements, negotiated terms and authorizations are exact, bounded and bilateral (`000261`, `EDR-388`, BR-AGREE-001).** An agreement names two explicit parties, carries append-only transitions and audited station decisions; negotiated price terms are private per agreement; vouchers are bounded, expiring, single-use with at most five failed attempts before a terminal lock; `org_fuelings` keeps its single signed APP origin so a redemption never writes a second fueling cost.
- **B-BR-EMP-599 — The redemption is once-only and its bounded attempts survive refusals (`EDR-390`, BR-AGREE-002, BR-TEN-005).** The station resolves the voucher by code digest under the station scope, joins the organization scope after the target resolves, consumes under a retained row lock, answers matching retries with the stored receipt, and commits each recorded invalid attempt with its typed refusal; the fifth invalid try locks the voucher and unknown codes reveal nothing.
- **B-BR-EMP-600 — M7 local gate checkpoint (`EMP-REL-011`).** Verifies `VAL-33`, `VAL-34`, the 460-cell authorization matrix, the 205-endpoint production router inventory, runtime-grant regeneration, the F7 end-to-end integration flow with its concurrent-receipt proof, `business-web` 72/72 and the shared Android golden vectors locally against real PostGIS while retaining the physical M4 device matrix, VPS activation, station-claim and portal activation, and production `G09` obligations.

- **BUC-EMP-306 — Propose, accept and manage a station agreement.** An eligible fleet representative proposes a bilateral agreement with its negotiated price terms under `agree.agreement.manage`; the station party accepts or declines through its live station grant surface; suspension stops both new authorizations and new redemptions, and ending keeps the settlement history for reconciliation.
- **BUC-EMP-307 — Announce a station price or promotion.** A current station representative with the `price.publish` scope publishes an attributed announcement or labelled promotion with explicit validity; supersession and withdrawal are recorded state transitions, public reads are anonymous and source-labelled, and expiry applies at read time.
- **BUC-EMP-308 — Issue a bounded fueling authorization.** An authorized fleet member issues a single-use voucher with product, unit, volume and total limits and an explicit expiry under `agree.authorization.issue`; the one-time code is answered exactly once and only its SHA-256 digest is stored; cancellation is recorded with a reason and a redeemed or locked voucher is refused.
- **BUC-EMP-309 — Confirm a redemption once at the station.** A permitted station actor submits the valid transaction and voucher code under `redemption.confirm`; the server derives the total from the negotiated term, consumes the voucher once, answers matching retries with the stored receipt, and records bounded invalid attempts that lock the voucher after five failures.
- **BUC-EMP-310 — Review the company's agreements, authorizations and settlements.** The company panel lists agreements with derived states, authorizations with their derived eligibility (never the one-time code) and station-confirmed settlements with server-derived totals under the fleet-side permissions.
- **BUC-EMP-311 — Keep cross-organization sharing on the transaction allowlist.** The station party sees only its own agreements' organization label, state, validity and negotiated terms; no fleet, employee, document or location field crosses the boundary, and the integration flow proves the allowlist on every station-side read.

- **B-BR-EMP-601 — Versioned organization plan catalog (EMP-SUBS-001).** An organization plan is identified by its product code, plan code and positive revision. Each revision fixes the explicitly configured capabilities and integer meter limits. A missing or unknown limit does not permit new usage. An explicit zero permits zero units. Catalog facts alone confer no membership, permissions, billing state or personal Premium rights. Commercial hypotheses are excluded from the catalog.
- **BUC-EMP-312 — Resolve the exact organization plan version (EMP-SUBS-001).** The server reads the version named by an organization subscription and checks its registered capabilities and explicitly defined ceilings. Older versions remain readable after newer versions are recorded. An unknown version, unregistered capability or missing meter gives no entitlement.

- **B-BR-EMP-602 — Organization subscription transitions carry server evidence (EMP-SUBS-002).** Every transition binds the organization, subscription, expected revision, event time and evidence reference. Approved trial policy can start a trial. Only verified billing can confirm a settlement or overdue invoice. An approved operator can suspend and restore a subscription without changing its prior billing state. Cancellation records the prior state and ends on verified cancellation or at the recorded term boundary. A terminal record stays terminal. No transition changes roles, personal Premium, public access or station authority.
- **BUC-EMP-313 — Restore subscription state from a verified fact (EMP-SUBS-002, VAL-18).** A verified settlement restores the corresponding past-due subscription and records one versioned transition. A foreign subscription, wrong evidence source, stale revision or ended subscription cannot move. Provider event deduplication and durable acknowledgements are handled by billing intake.

- **B-BR-EMP-603 — Subscription capabilities are an additional organization authority (EMP-SUBS-003).** Evaluators use the current scoped subscription and its exact catalog version. An active trial or paid term can grant explicit capabilities while the term is current. A pending cancellation can retain an entitlement from its prior entitled state through the recorded term boundary. The existing global switch, explicit operator denial, membership, role, record and proof gates remain mandatory. Administrative pilot grants retain their authority for organizations without subscription history.
- **BUC-EMP-314 — Check a subscription capability and its resource ceiling (EMP-SUBS-003).** An authorized organization command resolves the eligible current plan and its declared meter ceiling. Unknown capability, absent limit, wrong organization, stale or ended term and a request above the ceiling are refused. Exact-limit usage is allowed. No arithmetic overflow permits an extra unit.

- **B-BR-EMP-604 — Organization billing binds a canonical provider record to an approved offer (EMP-SUBS-004).** The company offer is independent of the account catalog and records its exact plan revision, provider channel, environment, merchant, provider offer reference, approved BRL amount and review reference. An admitted billing event must match the enrolled organization and subscription, the provider contract and customer, and the bound offer. Redirects, authorization, unsettled or unknown statuses and amount or environment mismatches create no entitlement. An approved and funded future term cannot change today's entitlement.
- **BUC-EMP-315 — Reconcile a verified organization payment by exact binding (EMP-SUBS-004, VAL-18).** A provider event is retrieved and reconciled with the organization-specific binding and approved catalog. Matching settlement can produce one lifecycle fact. Repeated event keys or canonical charges cannot extend access again. Unexpected source, amount, term or status remains a reconciliation exception.

- **B-BR-EMP-605 — Recheck plan writes in the owning transaction (EMP-SUBS-005, VAL-35).** An authorization prepared before a plan change cannot grant a later operational or configuration write. The owning transaction checks current subscription and administrative policy under the same organization lock used by lifecycle changes. Existing membership, scoped record, organization state and role gates still apply. Retained reads and allowed access-reducing actions continue through their established authority.
- **BUC-EMP-316 — Reject a prepared company write after entitlement ends (EMP-SUBS-005).** If subscription suspension, expiry or an operator denial takes effect before the owning transaction checks eligibility, the command returns the existing entitlement error and records no business change. An entitled subscription can satisfy the capability without a pilot enrollment. A different organization's plan cannot satisfy the check.

- **B-BR-EMP-606 — Hold eligible earlier offline work after a subscription capability ends (EMP-SUBS-005, VAL-35).** A valid device intake can use the declared instant as review evidence. The server confirms a recorded eligible subscription and capability at declaration and no qualifying subscription at receipt. The existing state intake records the exact historical subscription revision and returns a review acknowledgement without creating the operational effect. A declaration while the capability was absent cannot produce this hold. Explicit operator capability decisions keep their existing offline policy.
- **BUC-EMP-317 — Resume delivery of earlier work after losing a plan capability (EMP-SUBS-005).** Submit a queued fact through normal signed receipt and owner review. An eligible declaration before the loss produces one held fact and a stable replay. A new declaration after loss receives the ordinary entitlement denial. Current membership and enrollment still govern both paths.

- **B-BR-EMP-607 — Measure vehicle activations under the organization write lock (EMP-SUBS-006, EMP-SUBS-009, VAL-35).** An active vehicle is a fleet record with kind VEHICLE and state ACTIVE. Draft creation, machines, implements and retained inactive or archived records consume no active vehicle slot. An activation, reactivation or allowed classification change into an active vehicle checks current scoped use under the existing fleet write lock and the organization plan lock. A request that would exceed the ceiling, or relies on a plan with no vehicle ceiling, fails with LIMIT_REACHED and leaves the existing records intact.
- **BUC-EMP-318 — Retain a draft that cannot enter the active vehicle count (EMP-SUBS-006).** An authorized source may create a draft. Activating it requires current eligibility and one remaining unit on the reviewed plan. Two simultaneous activations for the last unit produce one change and one limit denial. The denied draft has no activation receipt and can retry after a selected vehicle leaves the active count.

- **B-BR-EMP-608 — Apply an approved plan change at the recorded term boundary (EMP-SUBS-006, JRN-11).** A reviewed next-term billing binding can name a different revision or plan of the same product. The target term starts at the current subscription's recorded end. Only verified settlement of that exact bound charge records the new plan revision, and early proof waits for the effective term. Unsettled, foreign and mismatched charges leave the subscription and vehicle records intact. The transition never selects resources to remove or changes their history.
- **BUC-EMP-319 — Apply the reviewed downgrade with one retained receipt (EMP-SUBS-006).** A confirmed next-term charge updates the current plan once. The preceding revision remains available to historical review. The new ceiling blocks additional activations above the limit while authorized users decide which resources leave active use.

- **B-BR-EMP-609 — Start a trial from recorded organization eligibility (EMP-SUBS-007).** The restricted enterprise operator command uses a scoped trial grant. The grant records the approved plan revision, acceptance window, duration and approval reference. The server clock sets the first activation time. A new grant cannot replace a nonterminal subscription of the same product. A matching operation replay returns the original term. A different operation cannot consume the grant again. Trial activation creates no charge or role.
- **BUC-EMP-320 — Activate a reviewed organization trial (EMP-SUBS-007).** The operator selects an eligible organization's approved grant and records one audited transition. A missing, foreign, expired or consumed grant cannot open another trial. The command records no trial length or price from its input.

- **B-BR-EMP-610 — Read organization plans and confirmed billing in the company panel (EMP-SUBS-008).** The company session and current organization-wide `subs.plan.read` grant authorize one organization. The panel shows its exact catalog revisions, lifecycle states, term boundaries, limits and reviewed upcoming changes. A successful payment is shown only when the scoped settled receipt exists for that term. Provider identifiers, approval references, individual Premium and unrelated organization records are excluded.
- **BUC-EMP-321 — Inspect the current plan and review the next term (EMP-SUBS-008).** Open the organization billing panel with the authorized company session. A trial, paid term, overdue status, upcoming plan or unconfigured organization is described according to the server facts. Switching companies clears pending results from the previous selection.

- **B-BR-EMP-611 — Check company session time after the proof lock (EMP-SUBS-008, BR-AUTH-010).** Concurrent authorized panel reads can both use the same company cookie. Each request uses its server instant after acquiring the session row lock. A request that waited for another touch must apply the idle, absolute-expiry, proof freshness and revocation rules to the latest row. A request begun earlier cannot reject a still-live session merely because another request touched it first.
- **BUC-EMP-322 — Load company billing while other private panels refresh (EMP-SUBS-008).** Multiple permitted requests retain the company session while its proof remains current. A request crossing a proof deadline is refused after the lock wait. The recorded last activity never moves backwards.

- **B-BR-EMP-612 — Pin Data publication to reviewed official inputs (EMP-DATA-001, VAL-36).** A regional dataset revision names the bounded enterprise export snapshot, method revision, geographic scope and sample threshold. Its two input pins identify the public registry file and official price revision by digest. Each pin requires its latest recorded commercial-redistribution approval with an attribution source. Pending, withdrawn, foreign or missing reviews keep the dataset held. Private company sources and mixed editorial snapshots cannot serve this dataset.
- **BUC-EMP-323 — Resolve a regional market dataset against current source rights (EMP-DATA-001).** Load its exact catalog revision and approved source pins before preparing or reading market results. Record the method and scope in the result. A correction or updated review produces a new eligible revision. Previously admitted inputs are refused when their recorded approval is withdrawn.

- **B-BR-EMP-613 — Company Data keys prove only their current scope (EMP-DATA-002).** The restricted company action requires fresh organization-wide key-management authority and the Data capability. An issued key binds the organization, exact read scopes, expiry and a one-time random bearer whose verifier is stored separately from display metadata. Rotation cannot widen the old scope. The old key can remain valid only until its recorded overlap, and revocation applies to the next read.
- **BUC-EMP-324 — Issue, rotate and revoke a Data credential (EMP-DATA-002).** An authorized actor records a scoped key command once. Only the successful first execution reveals the bearer. An operation replay returns non-secret metadata. Foreign, expired, revoked and out-of-scope credentials cannot authorize a market query.
