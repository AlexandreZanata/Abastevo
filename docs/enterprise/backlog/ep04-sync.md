# EP04 — Offline synchronization

Status: **PLANNED**, 2026-11-10.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from the [offline synchronization specification](../10-offline-sync.md), [privacy and security](../11-privacy-security.md) and [validation scenarios](../15-validation.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

The enterprise queue preserves the approved account, server origin and organization partition. C1 attachment validation uses synthetic receipts. The F4 late-intake and device-cleanup tasks retain their separate gates.

### EMP-OFFL-001 — Sync protocol contract: operation envelope, results, error codes, golden vectors
- **Meta:** module OFFL · phase F1 · cut C1 · priority P0 · type API · size M · state LOCAL_DONE
- **Objective:** Freeze an operation envelope and per-operation results for enterprise sync.
- **Rules and use cases:** BR-OFFL-001, BR-OFFL-003, BR-OFFL-004, BR-OFFL-007, BR-OFFL-008, BR-OFFL-018, UC-OFFL-02
- **Depends on:** EMP-PLAT-004
- **Read first:** [offline synchronization specification](../10-offline-sync.md) §§3–4 and 10.1–10.2, [architecture](../09-architecture.md) §§3–6
- **Scope:** in: typed envelope, result and error catalog, payload identity and shared vectors · out: storage, queue transport and feature commands
- **Areas:** existing: OpenAPI, personal cloud sync and fixture conventions · proposed: enterprise envelope, schema and golden vectors
- **Steps:** Pin required fields, declared and receipt time, hard dependencies, soft references and configured limits. Validate wire examples in both consumers.
- **Deliverable:** A versioned protocol contract and portable vectors for results and errors.
- **Acceptance:** Stable IDs and digests identify retries. Results distinguish accepted, held for review, rejected and retryable cases.
- **Negative cases:** Unknown schema, malformed or oversized request, wrong origin or organization, reused identifier with changed content and a retry marked final.
- **Validation:** V-API, V-SYNC, V-ISO · migration: none · rollback: keep the endpoint disabled until matching consumers exist
- **Risks and assumptions:** Reuse compatible conventions from personal sync. Enterprise batches carry independent per-operation transactions.
- **Done when:** The OpenAPI section, result catalog and shared vectors agree on the supported cases.
- **Local evidence:** [EMP-OFFL-001](../evidence/f2-offl001-sync-protocol.md), task217: both consumers pass260 shared vectors; no route/ledger/queue is activated. Protected integration remains pending.

### EMP-OFFL-002 — Server operation intake: receipt ledger, idempotency and dependency ordering
- **Meta:** module OFFL · phase F1 · cut C1 · priority P0 · type DB · size L · state LOCAL_DONE
- **Objective:** Intake one organization operation at a time with deterministic replay and ordering.
- **Rules and use cases:** BR-OFFL-004, BR-OFFL-005, BR-OFFL-006, BR-OFFL-007, BR-OFFL-008, BR-OFFL-018, BR-TEN-006, UC-OFFL-02
- **Depends on:** EMP-OFFL-001, EMP-AUTH-003, EMP-PLAT-010
- **Read first:** [offline synchronization specification](../10-offline-sync.md) §§4, 8 and 10.2, [permissions](../07-permissions.md) §§4 and 10, [validation scenarios](../15-validation.md) §4
- **Scope:** in: intake orchestration, receipt ledger and dependency checks · out: feature business logic and F4 post-revocation exceptions
- **Areas:** existing: PostGIS, sqlc, authorization and organization events · proposed: scoped intake and acknowledgement storage
- **Steps:** Pin final outcome, dependency retry and concurrent target cases. Commit the effect, audit and acknowledgement atomically under resolved scope.
- **Deliverable:** A durable intake ledger and scoped per-operation transaction boundary.
- **Acceptance:** Identical authorized retries reuse one final acknowledgement. An unresolved hard dependency retries without becoming final. Other operations in the batch still progress.
- **Negative cases:** Foreign receipt or dependency, changed digest, partial transaction, two concurrent effects, or one rejected item aborting unrelated work.
- **Validation:** V-SQL, V-API, V-ISO, V-AUTHZ, V-IDEM, V-SYNC · migration: additive with populated and prior-image rehearsal · rollback: stop intake and preserve receipts
- **Risks and assumptions:** Reconcile shared refusal rules and the decision table before freezing consumer cases. Server authority governs processing and receipt visibility.
- **Done when:** PostGIS proves ordered recovery, atomicity and scoped replay through normal intake.

[F2 intake acceptance](../evidence/f2-offl002-intake-ledger.md),
from tested/published PRIV-003 51c25146. B-BR-EMP-108..114, BUC-EMP-56/57 and
EDR-071..075 precede behavior. Feature/F4/runtime readiness is not inferred.
Final ordinary PostGIS/race union passes24 top-level/46 named cases, including
actual signed HTTP, immutable replay, dependencies/held facts,12-way contention,
late-source denial, deferred commit recovery, populated two-org RLS, original
enrollment/current read coverage, upgrade and personal-erasure/export boundaries.
Full affected API contract/cmd/api, protocol/application, generation/static/
format/secret checks pass. Protected integration remains pending.

### EMP-OFFL-003 — Android persistent operation queue with immutable organization context
- **Meta:** module OFFL · phase F1 · cut C1 · priority P0 · type APP · size L · state TODO
- **Objective:** Persist each Android operation with its original organization and actor context.
- **Rules and use cases:** BR-OFFL-001, BR-OFFL-002, BR-OFFL-003, BR-OFFL-005, BR-OFFL-013, BR-TEN-004, BR-PRIV-016, UC-OFFL-01
- **Depends on:** EMP-OFFL-001, EMP-PLAT-011
- **Read first:** [offline synchronization specification](../10-offline-sync.md) §§3–4 and 8, [M0 decisions](../evidence/f0-m0-decisions.md) §Android local protection
- **Scope:** in: durable queue, stable operation ID and digest, dependency-aware send and retry · out: moving work between partitions and treating a send attempt as acceptance
- **Areas:** existing: Android Room, WorkManager and enterprise partition foundation · proposed: scoped outbox and receipt application
- **Steps:** Commit queue entries before local success. Restore durable state after interruption and apply results to the matching partition and operation.
- **Deliverable:** A persistent Android operation queue with scoped receipt handling.
- **Acceptance:** An operation survives restart and retains account, origin, organization, membership and device. Transient failures leave it visible for retry.
- **Negative cases:** Crash after local success, process death during receipt handling, changed payload on retry, wrong partition and low storage dropping an unsent entry.
- **Validation:** V-APP, V-SYNC, V-IDEM, V-ISO, V-PRIV · migration: test upgrade and personal-storage preservation · rollback: pause sending without deleting queued data
- **Risks and assumptions:** The approved device protection applies to private queue payloads and files. Context changes cannot retag records.
- **Done when:** Queue durability and independent retry converge in process-death and two-organization tests.

### EMP-OFFL-004 — App: sync status model and the pending-sync screen
- **Meta:** module OFFL · phase F1 · cut C1 · priority P0 · type APP · size M · state TODO
- **Objective:** Show truthful per-record synchronization and attachment status in the app.
- **Rules and use cases:** BR-OFFL-010, BR-OFFL-013, BR-OFFL-015, BR-OFFL-017, UC-OFFL-01, SCR-OFFL-01, SCR-OFFL-02, SCR-OFFL-05
- **Depends on:** EMP-OFFL-003
- **Read first:** [offline synchronization specification](../10-offline-sync.md) §§4, 6 and 11, [state model](../08-state-model.md)
- **Scope:** in: pending queue, per-record history, retry and refusal visibility · out: declaring a business approval from sync status
- **Areas:** existing: durable Android queue and design system · proposed: sync status model and pending-work screens
- **Steps:** Map durable local state and server acknowledgements to the documented visible states. Exercise retry and failure feedback with accessible labels.
- **Deliverable:** A pending-sync screen and accurate per-record state presentation.
- **Acceptance:** Local save, send, acceptance, review and refusal stay distinct. Refused records remain visible with their reason until explicit dismissal.
- **Negative cases:** Successful-looking state without an acknowledgement, missing refusal reason, incorrect attachment state and another organization's pending record appearing.
- **Validation:** V-APP, V-SYNC, V-ISO · migration: none outside the queue task · rollback: hide new screens and preserve operations
- **Risks and assumptions:** Use current persisted state rather than inferring success from network connectivity.
- **Done when:** Affected Compose states, screen-reader text and pending queue actions pass targeted tests.

### EMP-OFFL-005 — Attachment upload: offline queue, retry and resume, linkage to operations
- **Meta:** module OFFL · phase F3 · cut C1 · priority P0 · type APP · size M · state TODO
- **Objective:** Deliver private synthetic C1 attachments independently of operation acknowledgement.
- **Rules and use cases:** BR-OFFL-003, BR-OFFL-013, BR-OFFL-015, BR-PRIV-004, BR-PRIV-020, OQ-04, UC-OFFL-03
- **Depends on:** EMP-OFFL-002, EMP-OFFL-003, EMP-PLAT-006
- **Read first:** [offline synchronization specification](../10-offline-sync.md) §§4–5, 8 and 12, [privacy and security](../11-privacy-security.md) §4, [M0 decisions](../evidence/f0-m0-decisions.md) §Private receipts
- **Scope:** in: synthetic C1 attachment linkage, private resumable delivery or safe retry and progress · out: real receipts before approved legal retention and release gates
- **Areas:** existing: durable Android queue and organization private storage port · proposed: attachment outbox and verified ingress flow
- **Steps:** Persist the file reference and declaration with the operation. Exercise interrupted delivery, missing files, digest mismatch and acknowledgement ordering.
- **Deliverable:** An offline attachment queue with scoped upload and durable verification results.
- **Acceptance:** Operation and attachment outcomes are independent. Only matching declared size and digest reach stored state through private ingress.
- **Negative cases:** Wrong organization or record, expired authority, mismatched or truncated bytes, duplicate file completion, missing local file and reporting success before verification.
- **Validation:** V-APP, V-SYNC, V-IDEM, V-ISO, V-PRIV · migration: rehearse any attachment queue upgrade · rollback: stop new uploads and preserve pending references
- **Risks and assumptions:** Synthetic receipts are the approved C1 validation scope. Keep private bytes out of diagnostics and backups.
- **Done when:** Interrupted uploads converge under retry and the accepted file matches its declaration.

### EMP-OFFL-006 — Reference data pull for the active organization: versioned deltas
- **Meta:** module OFFL · phase F2 · cut C1 · priority P0 · type API · size M · state TODO
- **Objective:** Pull authorized reference deltas for the selected organization.
- **Rules and use cases:** BR-OFFL-003, BR-OFFL-016, BR-PRIV-003, BR-AUTH-014, UC-OFFL-08
- **Depends on:** EMP-OFFL-002, EMP-FLEET-006
- **Read first:** [offline synchronization specification](../10-offline-sync.md) §§3–5 and 10.3, [permissions](../07-permissions.md) §4, [architecture](../09-architecture.md) §4
- **Scope:** in: versioned reference feed, scoped cursor and confirmation age · out: widening cached authority or syncing across organizations
- **Areas:** existing: organization authorization and fleet reference reads · proposed: versioned feed and client revision application
- **Steps:** Pin revision order and duplicate or invalid cursor handling. Exercise reassignment, revocation and stale cache age in two organizations.
- **Deliverable:** A scoped reference feed with explicit revision and confirmation state.
- **Acceptance:** A client applies only its authorized changes and displays confirmation age. Stale choices are checked again at intake.
- **Negative cases:** Foreign cursor, a missed deletion or reassignment, out-of-order delta, unauthorized cached reference and incorrect confirmed-at time.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-SYNC, V-PRIV · migration: additive if feed state needs persistence · rollback: disable the feed and mark cached data unconfirmed
- **Risks and assumptions:** Cached data is a read copy rather than proof of current authority.
- **Done when:** Feed revision and scope behavior pass with synthetic organizations and changing permissions.

### EMP-OFFL-007 — Device clock capture, skew estimation and time fields
- **Meta:** module OFFL · phase F1 · cut C1 · priority P0 · type DOM · size S · state TODO
- **Objective:** Keep device occurrence time, server receipt time and skew separate.
- **Rules and use cases:** BR-OFFL-008, BR-OFFL-009, BR-TIME-001, BR-TIME-002, BR-TIME-003, BR-TIME-006
- **Depends on:** EMP-OFFL-002, EMP-PLAT-015
- **Read first:** [offline synchronization specification](../10-offline-sync.md) §§4 and 10.1–10.2, [time rules](../04-business-rules.md#5-time--br-time)
- **Scope:** in: timestamp capture, offset and zone, skew evidence and warning vectors · out: changing authority or rewriting occurrence time
- **Areas:** existing: organization time kernel and operation contract · proposed: deterministic skew calculations and shared vectors
- **Steps:** Pin validity, offsets and skew boundaries. Keep the client declaration and server receipt unchanged and record skew as evidence.
- **Deliverable:** A tested sync time and skew representation.
- **Acceptance:** Authorization uses server time and history. Clock skew stays visible without silently changing a declared timestamp.
- **Negative cases:** Future declaration treated as authority, wrong offset sign, missing or malformed zone, skew without a flag and timestamp overwritten.
- **Validation:** V-DOM, V-SYNC · migration: none beyond the owning intake task · rollback: preserve raw recorded times
- **Risks and assumptions:** Warnings and eligibility for late intake are distinct decisions.
- **Done when:** Boundary vectors prove that client time cannot authorize an operation.

### EMP-OFFL-008 — Review queue for operations that need review, with resolution actions
- **Meta:** module OFFL · phase F2 · cut C1 · priority P0 · type API · size M · state TODO
- **Objective:** Resolve held operations once through scoped reviewer decisions.
- **Rules and use cases:** BR-OFFL-006, BR-OFFL-009, BR-AUTH-001, BR-REV-003, BR-REV-007, BR-REV-009, BR-AUD-001, UC-OFFL-04
- **Depends on:** EMP-OFFL-002
- **Read first:** [offline synchronization specification](../10-offline-sync.md) §§5–8, [permissions](../07-permissions.md) §§4–5, [validation scenarios](../15-validation.md) §4
- **Scope:** in: review item list, reason, resolution and audited result · out: automatic acceptance of held field work
- **Areas:** existing: intake receipt, organization audit and permissions · proposed: scoped review item transitions and API
- **Steps:** Pin each resolution and stale or concurrent reviewer outcomes. Apply allowed decisions atomically and expose the result to the author's next sync.
- **Deliverable:** An auditable review queue API with one committed resolution per item.
- **Acceptance:** Only a permitted reviewer resolves a matching revision. Held facts stay out of costs until the applicable approval.
- **Negative cases:** Self-approval, wrong scope, stale revision, double decision, missing refusal reason and held data counted as accepted.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-SYNC · migration: additive review storage · rollback: stop new resolutions and preserve evidence
- **Risks and assumptions:** The owning command defines whether a reviewer may change a field during resolution.
- **Done when:** Two-reviewer and retry races produce one final outcome and one audit record.

### EMP-OFFL-009 — Post-revocation intake window and quarantine
- **Meta:** module OFFL · phase F4 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Handle eligible pre-revocation work through a restricted F4 intake path.
- **Rules and use cases:** BR-OFFL-006, BR-OFFL-008, BR-OFFL-009, BR-OFFL-011, BR-OFFL-012, BR-IDN-029, UC-OFFL-07
- **Depends on:** EMP-OFFL-008, EMP-IDN-010
- **Read first:** [offline synchronization specification](../10-offline-sync.md) §§4 and 10.2, [identity specification](../modules/identity.md) §§4–5, [validation scenarios](../15-validation.md) §4
- **Scope:** in: intake-only policy, window and quarantine for review · out: ordinary private access or automatic approval after revocation
- **Areas:** existing: offboarding and device revocation history, intake ledger and review queue · proposed: bounded late-delivery path
- **Steps:** Pin the decision table and boundary instants using controlled server time. Verify revocation class, scope and server history before deciding delivery.
- **Deliverable:** A gated intake-only path that preserves eligible facts for review.
- **Acceptance:** Eligible pre-end operations inside the window enter review. Lost or compromised devices and closed windows remain refused. Intake grants no reads.
- **Negative cases:** Foreign organization, forged time treated as proof, new operation after end, revoked-lost device accepted or access restored through replay.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-SYNC, V-PRIV · migration: preserve revoked access evidence · rollback: disable the exception and retain held operations
- **Risks and assumptions:** The window and time rules come from server-side policy and history. This work stays outside C1.
- **Done when:** The late-delivery and denial vectors pass through PostGIS-backed intake without restoring membership authority.

### EMP-OFFL-010 — Context switch, logout and shared-device behaviour with a pending queue
- **Meta:** module OFFL · phase F1 · cut C1 · priority P0 · type APP · size M · state TODO
- **Objective:** Keep pending work safe through Android context switch and logout.
- **Rules and use cases:** BR-OFFL-003, BR-OFFL-013, BR-TEN-004, BR-AUTH-012, BR-PRIV-016, BR-PRIV-017, UC-OFFL-05, UC-OFFL-06, SCR-OFFL-04
- **Depends on:** EMP-OFFL-003, EMP-IDN-018
- **Read first:** [offline synchronization specification](../10-offline-sync.md) §§4–5 and 11, [M0 decisions](../evidence/f0-m0-decisions.md) §Android local protection
- **Scope:** in: context transition, send-or-keep choice and shared-device isolation · out: retagging work or sending under a different identity
- **Areas:** existing: persistent queue, context switch and account logout · proposed: enterprise logout and partition lock wiring
- **Steps:** Exercise switching while work is queued or in flight. Verify explicit logout choices, re-login and a different account on the same device.
- **Deliverable:** A scoped context and logout flow that preserves pending work.
- **Acceptance:** Pending work remains sealed to its original partition. Kept work is locked at logout and available only to its authorized original context.
- **Negative cases:** Wrong account or origin opens old data, late acknowledgement lands in another partition, silent deletion or worker uses replacement credentials.
- **Validation:** V-APP, V-SYNC, V-ISO, V-AUTHZ, V-PRIV · migration: preserve existing held queues · rollback: disable new entry while retaining stored work
- **Risks and assumptions:** Only send when the original session permits it. Explain the local consequences of reset or uninstall.
- **Done when:** Context and logout tests pass with two accounts, two organizations and pending writes.

### EMP-OFFL-011 — Cleanup of revoked sessions and devices on next contact
- **Meta:** module OFFL · phase F4 · cut - · priority P1 · type APP · size S · state TODO
- **Objective:** Apply local cleanup after server revocation or expiration of approved offline access.
- **Rules and use cases:** BR-OFFL-011, BR-OFFL-012, BR-OFFL-013, BR-OFFL-014, BR-IDN-038, BR-PRIV-018, UC-PRIV-03
- **Depends on:** EMP-OFFL-003, EMP-IDN-015
- **Read first:** [offline synchronization specification](../10-offline-sync.md) §§4–5, [identity specification](../modules/identity.md) §5, [privacy and security](../11-privacy-security.md) §§4–5
- **Scope:** in: partition lock and cleanup under the recorded revocation class · out: promising instant cleanup on a disconnected device
- **Areas:** existing: Android private partitions and scoped device revocation · proposed: on-contact cleanup and recovery
- **Steps:** Test contact, offline expiry, restart during cleanup and separate revocation classes. Follow the documented treatment of eligible intake-only work.
- **Deliverable:** A tested device cleanup flow with accurate server and device state.
- **Acceptance:** The server denies ordinary access immediately. Local cleanup targets only the revoked partition and truthfully reports pending or completed state.
- **Negative cases:** Foreign partition erased, lost device still sends work, interrupted cleanup exposes data, offline cleanup overclaim and eligible intake discarded silently.
- **Validation:** V-APP, V-SYNC, V-ISO, V-PRIV · migration: preserve unrelated partitions · rollback: keep revoked local data locked and preserve recovery evidence
- **Risks and assumptions:** Offline key expiry requires the approved clock and device protections.
- **Done when:** Revocation class, denied access and resumable local cleanup pass on isolated fixtures.

### EMP-OFFL-012 — Organization-state and plan-change handling for queued operations
- **Meta:** module OFFL · phase F4 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Evaluate queued work against organization and plan changes using server history.
- **Rules and use cases:** BR-OFFL-006, BR-OFFL-008, BR-OFFL-009, BR-ORG-030, BR-ORG-031, BR-AUTH-009, VAL-22, VAL-35
- **Depends on:** EMP-OFFL-008, EMP-ORG-009
- **Read first:** [offline synchronization specification](../10-offline-sync.md) §10.2, [organization lifecycle](../modules/organizations.md), [validation scenarios](../15-validation.md) §4
- **Scope:** in: suspension, grace and downgrade disposition for queued operations · out: plan sales, role changes and automatic held approvals
- **Areas:** existing: organization suspension, capability gate, intake and review queue · proposed: organization and plan history checks
- **Steps:** Pin eligible pre-change and ineligible post-change cases. Exercise concurrent state changes and authorized replay with controlled time.
- **Deliverable:** An auditable queued-operation decision matrix for changed organization or plan state.
- **Acceptance:** Eligible earlier work follows the configured grace or plan-change review rule. New forbidden work stays denied.
- **Negative cases:** Old clock assertion bypasses write block, downgrade changes membership, foreign history, replay turns held work into acceptance or duplicate cost.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-SYNC · migration: additive only if history requires it · rollback: preserve accepted records and held review state
- **Risks and assumptions:** Local context and billing state do not decide authorization. Approved review is distinct from intake.
- **Done when:** PostGIS and sync vectors prove grace boundaries, downgrade and reactivation outcomes.

### EMP-OFFL-013 — Sync resilience suite: duplicates, reordering, partial uploads, process death
- **Meta:** module OFFL · phase F4 · cut - · priority P1 · type TEST · size M · state TODO
- **Objective:** Prove end-to-end convergence across the enterprise sync failure cases.
- **Rules and use cases:** BR-OFFL-002, BR-OFFL-004, BR-OFFL-005, BR-OFFL-007, BR-OFFL-013, BR-OFFL-015, VAL-06, VAL-23
- **Depends on:** EMP-OFFL-002, EMP-OFFL-003, EMP-OFFL-005
- **Read first:** [offline synchronization specification](../10-offline-sync.md) §§8 and 12, [validation scenarios](../15-validation.md) §§4 and 8
- **Scope:** in: duplicates, reordering, partial upload, process death and observed outcomes · out: replacing the request and persistence boundaries with mocks
- **Areas:** existing: scoped PostGIS fixtures, Android queue and private attachment test path · proposed: resilience acceptance suite
- **Steps:** Run the same synthetic operations through in-order and disrupted delivery. Compare final effects, acknowledgements and preserved pending state.
- **Deliverable:** A repeatable resilience suite spanning server and Android boundaries.
- **Acceptance:** Replay and reordering converge on the same allowed effects. Partial files recover and process death loses no durable operation.
- **Negative cases:** Lost response, wrong dependency order, duplicate effects, missing acknowledgement, killed process, truncated upload and stale cross-scope state.
- **Validation:** V-SQL, V-SYNC, V-IDEM, V-ISO, V-PRIV · migration: none · rollback: keep the regression suite and fixture contracts
- **Risks and assumptions:** Use synthetic content and the normal entry points. Keep device and production evidence separate from this task.
- **Done when:** All documented resilience cases pass against a named revision with equivalent final state.

### EMP-OFFL-014 — Panel: sync review queue
- **Meta:** module OFFL · phase F2 · cut C1 · priority P0 · type WEB · size S · state TODO
- **Objective:** Expose scoped sync review and its actual server outcome in the company panel.
- **Rules and use cases:** BR-OFFL-006, BR-OFFL-009, BR-REV-003, BR-REV-007, BR-AUTH-013, UC-OFFL-04, SCR-OFFL-03
- **Depends on:** EMP-OFFL-008, EMP-PLAT-008
- **Read first:** [offline synchronization specification](../10-offline-sync.md) §§5–7 and 11, [M0 decisions](../evidence/f0-m0-decisions.md) §Company web surface
- **Scope:** in: scoped review list, reason and resolution UI · out: client-side approval authority or showing unrelated records
- **Areas:** existing: /empresas/ scaffold and review API · proposed: sync review queue screens
- **Steps:** Test reviewer scope, keyboard and screen-reader access, concurrent resolution conflict and failed or stale decisions through the panel.
- **Deliverable:** An accessible sync review UI with server-confirmed outcomes.
- **Acceptance:** Reviewers see only permitted items and their reasons. The server resolves once and conflict or refusal is visible.
- **Negative cases:** Foreign data after context switch, offline approval, self-review, hidden conflict and client status implying approval before acknowledgement.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-SYNC, V-PRIV · migration: none · rollback: hide the queue UI and preserve server-held work
- **Risks and assumptions:** Only the authorized record-specific review permission enables a resolution.
- **Done when:** The review flow and denied states pass through the company session and normal routes.
