# Offline operation and synchronization

Status: **PLANNED**, 2026-11-10. Module code `OFFL`. This file is the module specification for offline behaviour and the transversal offline matrix for the whole suite. Tasks: `EMP-OFFL-001…014` in [backlog/ep04-sync](backlog/ep04-sync.md).

Evidence. The repository already has a private command protocol for one account's data: stable operation identifiers, expected revisions, durable acknowledgements, conflict responses and anti-resurrection fences ([PREMIUM_CLOUD_PROTOCOL](../backend/PREMIUM_CLOUD_PROTOCOL.md), `IMPLEMENTED` on the server; the Android side has separate queues per feature, `PARTIAL`). That protocol states "No tenant/fleet ownership". This module is `PROPOSED`: it reuses the protocol's identifiers, acknowledgement and replay rules, and changes what an organization needs changed. The differences are listed in §3.

## 1. Purpose

Field employees work where there is no signal. They must be able to record what happened, keep working, and trust that the record reaches the right company. Managers must be able to trust that what arrives was recorded by an authorized person, and must see and resolve what the server could not accept by itself. Nothing is ever dropped silently.

## 2. Scope

- **Basic (F1–F2, cut C1):** operation envelope and contract; server intake with idempotency and dependency order; Android persistent queue with fixed organization context; sync status on every record; attachment upload with retry; reference data pull; device clock handling; a review queue with basic resolution; behaviour on context switch and logout.
- **Next (F4):** intake after a membership ends; cleanup of revoked devices; organization and plan state changes; a resilience test suite.
- **Later:** background upload scheduling tuned by measured battery and data use; conflict resolution assistants in the panel.
- **Out of scope:** real-time collaboration, merging of concurrent edits to the same field, peer-to-peer sync between devices.

## 3. Entities and ownership

| Entity | Owner | Content | Invariants |
|---|---|---|---|
| Operation | Organization (created by a member on a device) | Identifier, organization, membership, device, type and schema version, target entity and identifier, expected revision, declared time and zone, dependencies, attachment references, payload, payload digest | Immutable after creation. Organization never changes (BR-TEN-004). |
| Acknowledgement | Organization | Operation identifier, result, code, resulting revision, receipt time, review item | One per (organization, operation identifier). Kept as long as the record it produced. |
| Review item | Organization | Operation, reason code, context, state, resolution, resolver | Resolved exactly once. |
| Attachment upload | Organization | Attachment identifier, declared size and hash, received size and hash, state | Bytes accepted only for a declared attachment of an accepted or pending operation. |
| Reference snapshot | Organization (read copy on device) | Versioned copy of what the member may see: assets, assignments, missions, templates, categories, policies | Carries the server revision and the time it was confirmed. |
| Device queue | Device, partitioned by (account, API origin, organization) | Pending operations and attachment bytes | Another account on the same device can neither read nor send it. |

What is reused from the personal cloud protocol, and what differs:

| Aspect | Personal protocol v1 | Organization operations |
|---|---|---|
| Owner | One account | One organization; the actor is a membership on a device |
| Unit of work | A batch of record writes, atomic as a whole | Each operation is its own transaction; a batch is only transport. One conflicting record must not hold back the others recorded in the field. |
| Shape | Generic typed record with expected revision | Domain commands (`trip.start`, `fueling.submit`, …) handled by the owning module |
| Replay | Same identifier and body returns the first result; changed body conflicts | Same |
| Conflict | Refuse with base, current and proposed | Field facts are kept for review instead of refused (BR-OFFL-006) |
| Consent mask and deletion epoch | Per-dataset opt-in by the account holder | Not applicable; activation is per organization, and personal-data rights follow [11-privacy-security](11-privacy-security.md) |
| Acknowledgement retention | 30 days | As long as the resulting record, because the acknowledgement is part of the audit trail |

## 4. Business rules

- **BR-OFFL-001 — Local identifiers are final.** Every record and operation gets a UUID on the device. The server never renumbers.
- **BR-OFFL-002 — The queue is durable.** An operation is written to persistent storage before the interface reports success. It survives process death, restart and app update.
- **BR-OFFL-003 — Organization context is sealed.** The organization, membership and device of an operation are fixed when it is created. No later action on the device rewrites them.
- **BR-OFFL-004 — Exactly one acknowledgement.** The server stores one acknowledgement per (organization, operation identifier). An identical resend returns it. The same identifier with a different payload digest is refused with `OPERATION_ID_REUSED`.
- **BR-OFFL-005 — Delivery is at least once; effects are once.** The network may deliver an operation several times. BR-OFFL-004 makes the effect happen once. No component is described as delivering exactly once.
- **BR-OFFL-006 — Facts are kept; authority is decided.** A well-formed operation from a device that was legitimately enrolled is never discarded because of a business conflict. It is accepted, or accepted for review. Permanent refusal is reserved for: unauthenticated or unknown device, device revoked as lost or compromised, organization mismatch, malformed or unknown schema, identifier reuse, a declared time after the member's access ended, and a closed intake window (BR-OFFL-011).
- **BR-OFFL-007 — Order follows dependencies.** An operation lists the operations it depends on. The server applies it only after those were accepted; otherwise it answers `RETRY` with `DEPENDENCY_PENDING`. A soft reference (for example a fueling pointing to a trip) to a refused or missing target is accepted for review with the link removed and the code `ORPHAN_REFERENCE`.
- **BR-OFFL-008 — Two times, one authority.** The device declares `occurred_at`; the server sets `received_at`. Authorization uses server time and the server's own history of memberships, assignments, plans and locks (BR-TIME-003).
- **BR-OFFL-009 — Device time is evidence, not proof.** When the right to perform an operation changed between the declared time and receipt, the operation is accepted for review even if the member was authorized at the declared time. It is never accepted automatically on the strength of the device clock.
- **BR-OFFL-010 — Four visible states.** Every record shows whether it is saved on the device, sent, accepted by the company or needs review ([08-state-model](08-state-model.md) §2). A refused operation stays visible with its reason until the user dismisses it.
- **BR-OFFL-011 — Intake after access ends.** When a membership ends or is suspended, operations declared before that moment and received within the intake window (default 15 days, organization-configurable 0 to 60) are accepted for review with `ACCESS_ENDED_BEFORE_RECEIPT`. Operations declared after that moment are refused with `MEMBERSHIP_ENDED`. After the window everything is refused with `INTAKE_WINDOW_CLOSED`. Intake-only access gives no reading right.
- **BR-OFFL-012 — Revocation reasons differ.** A device revoked as lost or compromised is refused entirely from the moment of revocation. A device whose member was offboarded keeps intake-only access for the window of BR-OFFL-011.
- **BR-OFFL-013 — No silent loss on the device.** The app never deletes an unsent operation by itself: not on retry exhaustion, not on low storage, not on context switch, not on logout. It asks the user and says what will happen.
- **BR-OFFL-014 — No remote-wipe promise.** Revocation takes effect on the server at once. Local data of a disconnected device is erased only when the app next reaches the server or when its local access expires (BR-PRIV, [11-privacy-security](11-privacy-security.md)). The interface and the documentation say so.
- **BR-OFFL-015 — Attachments follow their operation.** An operation can be accepted before its attachment bytes arrive. The record then shows the attachment as pending. Bytes are accepted only when size and SHA-256 match the declaration.
- **BR-OFFL-016 — Reference data shows its age.** Cached lists (assets, missions, policies) show when they were last confirmed. Choices made on stale reference data are checked on the server like any other.
- **BR-OFFL-017 — Online-only actions say so.** An action that needs the server (see §10) is disabled offline with the reason, never queued as if it had happened.
- **BR-OFFL-018 — Limits.** A batch carries at most 50 operations and 64 KiB, 8 KiB per operation, following the existing protocol bounds. Limits are configuration, verified by tests, and may be revised with measurements.

## 5. Use cases

**UC-OFFL-01 — Record offline and deliver later.** Actor: member in enterprise mode. Preconditions: device enrolled; reference data cached. Flow: user records something → app validates locally → operation written to the queue → record shown as "Salvo no aparelho" → when a network is available the app sends operations in dependency order → server answers per operation → record shows "Aceito pela empresa". Alternatives: transient failure → stays saved, retried with backoff; server answers needs review → record shows "Precisa de revisão" with the reason; server refuses → "Não aceito" with the reason and the option to copy the data into a new record when that makes sense. Result: organization record created exactly once.

**UC-OFFL-02 — Resend after a lost answer.** Flow: the app sent a batch and the connection dropped before the answer → the app resends the same operations → the server returns the stored acknowledgements. Result: no duplicate record (BR-OFFL-004).

**UC-OFFL-03 — Upload an attachment.** Flow: operation declares an attachment (identifier, size, hash) → metadata operation is sent first → app requests an upload capability → sends bytes to the API ingress → server verifies size and hash and queues safety checks → document state becomes `STORED`. Alternatives: connection drops → the same upload is retried and is idempotent; hash mismatch → refused, app retries from the stored file; file missing on the device → record shows "comprovante não encontrado no aparelho" and asks for a new capture. Result: document linked to the operation.

**UC-OFFL-04 — Resolve a review item.** Actor: member with the permission of the record type (for example `trip.trip.review` for trip conflicts). Flow: open the review queue → see the operation, the reason and the context (declared time, receipt time, last successful sync) → choose accept, accept with a change (for example another asset) or refuse with a reason → server applies and audits → the author's device shows the outcome at next sync. Result: item resolved once.

**UC-OFFL-05 — Switch context with pending operations.** Flow: user switches from organization A to personal mode or to organization B → pending operations of A stay queued and keep being sent in the background while the session allows → the switcher shows a pending count per context. Result: no operation changes organization (BR-OFFL-003).

**UC-OFFL-06 — Log out with pending operations.** Flow: user asks to log out → app shows the pending count and offers "send now" (if online) or "keep on this device" → kept operations stay encrypted and bound to the account → at the next login of the same account they are sent. Alternatives: a different account logs in → it cannot see or send them; the device is reset or the app uninstalled → they are lost, and the logout dialog said so. Result: nothing is sent under another identity.

**UC-OFFL-07 — Deliver after offboarding.** Flow: a driver is offboarded while offline with three unsent fuelings → the driver's phone reconnects within the window → the app can still deliver, with intake-only access → the three fuelings are accepted for review → a manager decides each one. Alternative: reconnects after the window → refused with `INTAKE_WINDOW_CLOSED`; the app shows that the company must be contacted. Result: no automatic acceptance after access ended (BR-OFFL-009, 011).

**UC-OFFL-08 — Refresh reference data.** Flow: on opening enterprise mode and periodically, the app asks for changes since its revision → applies them → updates "confirmed at". Alternative: an asset the user is about to pick was reassigned → picker updates; if the user already started a trip with it offline, UC-OFFL-01 delivers it and the server puts it in review with `ASSET_UNAVAILABLE`.

## 6. States and transitions

Operation sync states: [08-state-model](08-state-model.md) §2. Review item:

| From | To | Actor | Validation | Effect | Reversal |
|---|---|---|---|---|---|
| — | `OPEN` | Server intake | A review reason applies | Record exists in a held form; excluded from costs and indicators | — |
| `OPEN` | `RESOLVED_ACCEPTED` | Member with the record type's review permission | Not the author (BR-REV-003 applies to the resolver too) | Operation applied, possibly with the stated change; audit | Amend or void the resulting record (BR-AMD) |
| `OPEN` | `RESOLVED_REFUSED` | Same | Reason required | Operation marked refused; device shows "Não aceito" | The author records again |
| `OPEN` | `EXPIRED_UNRESOLVED` | Time (default 60 days) | — | Escalated to organization administrators; stays listed | Resolve late |

Attachment upload: `DECLARED` → `UPLOADING` → `RECEIVED` → `STORED`, or `QUARANTINED` / `FAILED` (document storage states in [08-state-model](08-state-model.md)).

## 7. Roles and permissions

Sending operations needs the permission of the command itself (for example `fuel.fueling.record`). Resolving a review item needs the review permission of the record type (`trip.trip.review`, `fuel.fueling.review`, `exp.expense.review`, `meter.reading.correct`). Listing the review queue shows only items in the member's scope. Intake-only access after offboarding carries no permission other than delivering operations created before the end.

## 8. Offline, concurrency, idempotency and corrections

- **Concurrency on the server:** intake serialises per target entity (row lock on the entity or its parent). Two devices starting a trip on the same asset: the first received is accepted; the second is accepted for review with `ASSET_BUSY` ([modules/trips](modules/trips.md)).
- **Edits:** a change to a record already accepted is a new operation with the expected revision. A stale revision is accepted for review with `STALE_REVISION` and both versions shown, never overwritten by arrival order.
- **Corrections of refused operations:** the device offers "create again from this" which copies the payload into a new operation with a new identifier.
- **Retry policy on the device:** exponential backoff with jitter, no attempt cap for transient errors, manual "send now". After repeated failures the record shows "needs attention".
- **Storage pressure:** the app warns when the queue or attachments exceed a threshold and never deletes unsent data by itself (BR-OFFL-013).

## 9. Events, notifications and integrations

Produced: `offl.operation.accepted`, `offl.operation.review`, `offl.operation.refused`, `offl.review.resolved`, `offl.attachment.stored`. The logical needs-review event originally named `offl.operation.needs_review` is persisted as `offl.operation.review` (EDR-074), conforming to the existing audit/outbox SQL identifier vocabulary; the frozen `ACCEPTED_FOR_REVIEW` result and codes are unchanged. Consumed: `idn.membership.ended`, `idn.device.revoked`, `org.organization.suspended`, `subs.subscription.state_changed`. Notifications: the author is told when an item needs review, is resolved or is refused; reviewers are told when their queue has new items (digest, not one notification per item).

## 10. Data, formulas and examples

### 10.1 Frozen operation profile (EMP-OFFL-001)

The components `EnterpriseSyncOperation`, `EnterpriseSyncBatch` and
`EnterpriseSyncAcknowledgement` in [OpenAPI](../../contracts/openapi/v1.yaml)
freeze the wire structure. The pure Go `orgsync/protocol` and Kotlin
`domain.enterprise.sync` codecs enforce the lexical and semantic constraints
which JSON Schema cannot express. This source checkpoint installs no route,
intake ledger, delivery worker or business effect. Current source authority is
still mandatory when OFFL-002 admits an operation.

All fields are required, including explicit null revisions and result fields.
An operation carries schema_version, operation_id, owner_account_id, api_origin,
org_id, membership_id, enrollment_id, type, operation_schema, entity,
occurred_at, entry_zone, device_offset_seconds, depends_on, soft_references,
attachments, payload and payload_digest. `enrollment_id` names the organization
registration row; it is not a key ID or proof that a device is active. A type
has at least two lower snake_case segments, such as `example.record`. The
positive int64 operation_schema is structural; only version1 is supported now.
Payload business fields belong to the owning feature, not this transport.

Hash the complete immutable operation except payload_digest. Owner, exact API
origin, organization, member, enrollment, entity/revision, declared time/zone/
offset, dependencies, references, attachment metadata and payload are covered.
The digest is `sha256:` plus64 lowercase hexadecimal digits. Attachment hashes
are bare64-digit lowercase hexadecimal strings. Transient family/token,
signatures, batch attempts, clock samples and server receipt time are excluded.
Renewing delivery credentials preserves the captured operation bytes and digest.
A reused ID with changed immutable intent cannot become an identical replay.

The canonical JSON profile accepts null, booleans, well-formed Unicode strings,
arrays, objects and arbitrary-precision integer lexemes. It refuses fractions,
exponents, leading zeros, negative zero, duplicate decoded keys and invalid UTF-8
or surrogate escapes. Object keys must match
`^[a-z][a-z0-9]*(_[a-z0-9]+)*$` and sort by ASCII. Ordinary arrays retain their
order. Dependency, soft-reference and attachment sets sort canonically and
refuse duplicate identities. Quote and backslash use two-character JSON escapes;
C0 controls use lowercase six-character Unicode escapes; other valid Unicode
is emitted without normalization or HTML escapes. This is not RFC8785/JCS.

Operation origins are exact canonical lowercase HTTPS origins, with ASCII DNS
LDH labels or valid IP literals, no userinfo/path/query/fragment/IPv6 zone IDs.
Ambiguous all-numeric hosts must be valid IPv4; DNS trailing dots and valid
lowercase IPv6 spelling are preserved. Ports are1..65535 without zero padding;
default443 is omitted. Configuration normalization happens before creation and
cannot rewrite a queued operation. Neither codec performs a DNS lookup.

The transport wrapper has schema_version, family_id, access_token and operations.
Credentials are untrimmed ASCII graphic strings of1..512 bytes. It may contain
0..50 raw operation candidates. Parsing the wrapper is not admission: preserve
exact raw spans, then parse each operation against independently captured source
binding. OFFL-002 must preflight trustworthy correlation identities and source
bindings for the whole body before any intake budget, claim, receipt or effect.
Every candidate uses the complete frozen operation parser, not a partial
correlation parser. Broken envelopes, digests and source bindings refuse the
whole body before prefix effects. Structurally valid unsupported operation
schemas or malformed business payloads can be independent only after a real
owning handler establishes current command/resource authority. Duplicate valid
operation IDs are refused. Identity nonce consumption retains its independent
authentication semantics.
Signing must cover the exact transmitted body.

The signed intake response is EnterpriseSyncBatchResponse, an object with
schema_version 1 and acknowledgements in request order (0..50). Each item is the
complete canonical frozen EnterpriseSyncAcknowledgement; a replay retains its
original bytes. This transport wrapper does not disclose account proof or add
batch atomicity. Global source denial returns the safe error envelope instead
of exposing buffered acknowledgement items.

Default limits are65536 raw UTF-8 batch bytes (including credentials),8192 raw
and complete canonical bytes per operation and64 transport container levels
(62 per operation, reserving the two wrappers). Trusted immutable limits may
reduce these maxima, never widen them or be selected by a request; the operation
byte limit cannot exceed the batch limit. Tests share literal byte-boundary
fixtures across both languages.

Results echo operation_id and the attempted payload_digest. The public decoder
requires the original immutable operation and checks both before exposing an
outcome. ACCEPTED requires a positive revision and null code/review item.
ACCEPTED_FOR_REVIEW requires a supported review code and review_item_id;
its revision may be null. REJECTED requires a refusal code and null revision/
review item. All three are final and require a nonzero canonical server
received_at. RETRY only permits DEPENDENCY_PENDING or UNAVAILABLE and has null
revision, received_at and review_item_id. Finality is derived, never a wire flag.
Warnings are a unique set, initially only CLOCK_SKEW; they do not rewrite device
time. An OPERATION_ID_REUSED response correlates the attempted digest without
replacing the first stored acknowledgement. Future reserved codes do not activate
future membership/grace/plan policies. Hard dependencies wait for an actual
accepted source effect or completed review, without rewriting the original ACK.

Shared literal vectors live in
[contracts/testdata/enterprise/sync](../../contracts/testdata/enterprise/sync/).
The [task evidence](evidence/f2-offl001-sync-protocol.md) records hashes, tests and
remaining runtime work. The minimum OrgTime instant is valid declared evidence;
it is forbidden as a final server receipt.

### 10.2 Intake decision table

The table describes the owning integrations' target behavior, not blanket
runtime authority. OFFL-002 requires a current signed account, source origin,
membership and enrollment for the whole batch and inside every operation's
transaction. Current unknown/revoked/ended/suspended proof remains a safe
transport denial; F4 historical/grace outcomes require their actual historical
authority and review producers. Missing type, history, held-fact/review,
follow-up or explicitly configured transport budget returns UNAVAILABLE without
a new claim. Unsupported operation schema is per-operation MALFORMED only
after the owning installed type can authorize it. No generic FORBIDDEN result
pretends to prove that a permission was never held.

Claims, final ACKs and applied-effect readiness are distinct. An authorized
DEPENDENCY_PENDING claim has no final receipt/audit/effect. Review acceptance
alone is not dependency readiness. Owning current parent-record read coverage
is also required; a same-organization receipt/effect does not prove visibility.
After base command/target admission, a coverage refusal rolls back this operation
and returns the same DEPENDENCY_PENDING as a missing parent without a new claim.
Covered cross-device dependencies remain valid (EDR-075). Each operation commits its real effect or
held facts, audit, required follow-up and immutable final receipt atomically;
replay rechecks original ownership and current read coverage. Fresh DB time and
current source/feature authority are confirmed after transactional waits and
before commit. A later source denial withholds the response without claiming
that earlier committed operations rolled back. Declared time is evidence only:
normal offline delivery delay cannot establish CLOCK_SKEW. See B-BR-EMP-108..114,
BUC-EMP-56/57 and [OFFL-002 evidence](evidence/f2-offl002-intake-ledger.md).

| Condition at receipt | Result | Code |
|---|---|---|
| Valid, authorized now, no conflict | `ACCEPTED` | — |
| Identical resend | stored acknowledgement | — |
| Same identifier, different digest | `REJECTED` | `OPERATION_ID_REUSED` |
| Unknown schema or malformed payload | `REJECTED` | `MALFORMED` |
| Device unknown or revoked as lost | `REJECTED` | `DEVICE_REVOKED` |
| Dependency not yet accepted | `RETRY` | `DEPENDENCY_PENDING` |
| Soft reference to refused or missing target | `ACCEPTED_FOR_REVIEW` | `ORPHAN_REFERENCE` |
| Membership ended or suspended; declared before; inside window | `ACCEPTED_FOR_REVIEW` | `ACCESS_ENDED_BEFORE_RECEIPT` |
| Membership ended; declared after the end | `REJECTED` | `MEMBERSHIP_ENDED` |
| Membership ended; window closed | `REJECTED` | `INTAKE_WINDOW_CLOSED` |
| Permission removed between declared time and receipt | `ACCEPTED_FOR_REVIEW` | `PERMISSION_CHANGED` |
| Never had the permission | `REJECTED` | `FORBIDDEN` |
| Asset reserved, in maintenance, disposed or in another trip | `ACCEPTED_FOR_REVIEW` | `ASSET_UNAVAILABLE`, `ASSET_BUSY` |
| Trip already closed by someone else | `ACCEPTED_FOR_REVIEW` | `TRIP_ALREADY_CLOSED` |
| Reporting day in a locked period | `ACCEPTED_FOR_REVIEW` | `PERIOD_LOCKED` |
| Stale expected revision | `ACCEPTED_FOR_REVIEW` | `STALE_REVISION` |
| Device clock skew above the limit | as otherwise decided, flagged | `CLOCK_SKEW` |
| Organization suspended; entity opened before suspension, or declared before it and inside the grace period (default 7 days) | `ACCEPTED_FOR_REVIEW` | `ORG_SUSPENDED_GRACE` |
| Organization suspended; new entity declared after suspension | `REJECTED` | `ORG_SUSPENDED` |
| Plan limit reached after a downgrade; declared before the change | `ACCEPTED_FOR_REVIEW` | `PLAN_CHANGED` |
| Server or database unavailable | `RETRY` | `UNAVAILABLE` |

### 10.3 Offline matrix

OFFL-003's construction contract extends the accepted generation-backed Android
vault with a versioned typed queue, atomic operation/local projection and
per-operation ACK/projection. Preserve legacy opaque entries unsent. Actual
owning offline eligibility/projectors are mandatory; custody is not permission
and the initial production registry is empty. A fresh exact account/origin/org/
member/enrollment/family/fence attempt governs I/O, without adopting a replacement
selection or holding local locks across network. Preflight the entire strict
batch answer before independent atomic receipt application. Topological local
ordering does not infer server effect readiness; children of external/review
parents remain retryable, while retained cycles require attention. Technical
retry defaults are30-second base/6-hour capped equal jitter without an attempt
limit, honoring valid Retry-After; pressure warns at80% of the existing32MiB/
10000-entry storage bounds, with no eviction. Retain final receipts until an
explicit policy exists. See B-BR-EMP-115..122, EDR-076..080 and
[queue evidence](evidence/f2-offl003-protected-operation-queue.md).

"Offline" means the action works with no connection and is delivered later. "Online" means it needs the server at that moment.

| Operation | Consult | Create | Change | Complete | Needs online validation for |
|---|---|---|---|---|---|
| Sign in, activate corporate access, accept invitation | — | Online | — | Online | Everything |
| Switch context | Offline (cached, marked unconfirmed) | — | — | — | Fresh membership list |
| My assets, assignments, missions | Offline (cached) | — | — | — | Latest assignments |
| Accept or decline a mission | Offline | Offline | — | Offline | Mission still assigned to me |
| Checklist | Offline | Offline | Offline until completed | Offline | Blocking policy is applied locally from cached policy and rechecked on the server |
| Start trip | — | Offline | — | — | Asset availability; one active trip per asset |
| Logbook events, stops, pause, resume | Offline | Offline | Offline (new operation) | Offline | Trip not closed by someone else |
| Meter reading | Offline (own history cached) | Offline | Online (correction by authorized role) | — | Monotonicity against readings from other devices |
| Tracking | Offline (own trip) | Offline (recorded locally) | — | Offline | Nothing; batches are uploaded later |
| Navigation | Offline with downloaded map region | Offline | Offline | Offline | Nothing; live traffic is not offered |
| Finish trip | — | — | — | Offline | Final reading plausibility |
| Fueling | Offline (own) | Offline | Offline until approved | Offline (submit) | Capacity, duplicates, budget limits |
| Fueling with an agreement voucher | Online | Online | — | Online | Authorization validity and single use |
| Receipt capture and attachment | Offline | Offline | Offline (replace before sending) | Upload needs network | Hash, type and safety checks |
| Expense | Offline (own) | Offline | Offline until approved | Offline (submit) | Category active, limits, duplicates |
| Incident or damage report, maintenance request | Offline | Offline | Offline | Offline | — |
| Dispense from an internal tank | Offline (cached tank list) | Offline | — | Offline | Stock balance; negative balance goes to review |
| Budget hold or pre-authorization | Online | Online | Online | Online | Concurrent limit |
| Review result of my records | Offline (last known) | — | — | — | Latest decisions |
| Correct a returned record | Offline | — | Offline | Offline (resubmit) | Revision still current |
| Approve, return, reject | Online | — | — | Online | Everything |
| People, roles, assets, missions planning (panel) | Online | Online | Online | Online | Everything |
| Exports, reports, indicators | Online | Online | — | Online | Everything |
| Station portal, Data portal, plan and billing | Online | Online | Online | Online | Everything |

## 11. Screens

| ID | Surface | Objective | Actor | Data | Actions | Rules and permissions | Validations and states |
|---|---|---|---|---|---|---|---|
| SCR-OFFL-01 | APP | See everything not yet accepted by the company | Any member | Pending operations grouped by state, with type, time, organization and reason | Send now, open record, dismiss a refused item | BR-OFFL-010, 013 | Empty: "Tudo enviado". Offline: banner with last successful sync. Error: per-item reason in plain language. States announced to screen readers as text, not colour only. |
| SCR-OFFL-02 | APP | Understand one record's sync history | Author | Timeline: saved, sent, accepted, review, outcome; attachment progress | Retry attachment, create again from a refused record | BR-OFFL-010, 015 | Attachment missing on device: asks for a new capture. |
| SCR-OFFL-03 | PANEL | Resolve what the server kept for review | Reviewers by record type | Queue with reason, declared and receipt times, last successful sync, author, record preview | Accept, accept with change, refuse with reason | BR-OFFL-006, 009; scope filtering | Empty: "Nenhum registro aguardando decisão". Forbidden: standard pattern. Conflict when two reviewers act: second sees "já resolvido". |
| SCR-OFFL-04 | APP | Decide what happens to pending data before logout or context removal | Any member | Count and size of pending operations | Send now, keep on device, cancel | BR-OFFL-013, 014 | Explains that uninstalling or resetting the device loses unsent data. |
| SCR-OFFL-05 | APP | Warn about storage or long-unsent data | Any member | Queue size, oldest unsent item | Open pending list, free space guidance | BR-OFFL-013 | Shown as a persistent banner in enterprise mode, not a blocking dialog. |

## 12. Acceptance, dependencies, risks, phase and microtasks

Acceptance criteria:

1. Killing the app process right after the interface reports "Salvo no aparelho" loses nothing; the record is sent after restart.
2. Sending the same batch three times produces one record and three identical acknowledgements.
3. Operations delivered out of order converge to the same final state as in-order delivery.
4. Switching context or logging out and in with another account never changes the organization or author of a pending operation, and the other account cannot see it.
5. Every row of §10.2 is covered by a test with the stated result and code.
6. A fueling recorded offline before offboarding and delivered inside the window appears in the review queue and is not counted in costs until a manager accepts it.
7. A refused operation remains visible on the device with its reason until dismissed.
8. An attachment interrupted at any byte is completed by retry and the stored file matches the declared hash.

Dependencies: [07-permissions](07-permissions.md) (authorization at receipt), [modules/identity](modules/identity.md) (devices, membership history), [09-architecture](09-architecture.md) (API conventions, storage), EMP-GOV-008 (device storage protection).

Risks: review queue becoming a dumping ground (mitigated by precise reason codes, digests and an escalation after 60 days); device clocks far from real time (BR-TIME-006); storage growth on devices with many photos (thresholds and Wi-Fi-only uploads); divergence between this queue and the existing per-feature queues on Android (one shared queue component is built for enterprise mode; personal features keep theirs).

Phase: F1 core, F2 reference data and review queue, F3 attachments, F4 hardening. Microtasks: EMP-OFFL-001 to EMP-OFFL-014.
