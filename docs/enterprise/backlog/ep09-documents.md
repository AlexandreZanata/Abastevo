# EP09 — Receipts, fiscal documents and attachments

Status: **PLANNED**, 2026-11-10.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from [approved receipt scope](../evidence/f0-m0-decisions.md#private-receipts), [domain model](../06-domain-model.md), [state model](../08-state-model.md), [privacy and security](../11-privacy-security.md) and [document requirements](../17-traceability.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

The [document specification](../modules/documents.md) now records the recovered design. This backlog uses the surviving shared rules and approved synthetic C1 scope. Real-receipt intake, retention and fiscal consultation retain their separate gates.

### EMP-DOC-001 — Document domain: container, independent statuses, links and allocations
- **Meta:** module DOC · phase F3 · cut C1 · priority P0 · type DOM · size M · state TODO
- **Objective:** Represent a private document independently of its links, review and fiscal outcomes.
- **Rules and use cases:** EDR-014, BR-TRUTH-001, BR-TRUTH-002, BR-TEN-001, BR-MONEY-007
- **Depends on:** EMP-PLAT-001
- **Read first:** [domain model](../06-domain-model.md) §§3–5, [state model](../08-state-model.md), [document requirements](../17-traceability.md) §10
- **Scope:** in: pure document state, links and allocation invariants · out: uploads, persistence and fiscal consultation
- **Areas:** existing: organization scope and exact-value conventions · proposed: document domain package
- **Steps:** Pin independent status transitions and allocation bounds. Model original document identity and owned links explicitly.
- **Deliverable:** A pure document domain model with state and amount vectors.
- **Acceptance:** Receiving, extraction, structural checks, human review, reconciliation and fiscal consultation remain distinct. Allocations cannot exceed their source.
- **Negative cases:** A hash marks a document fiscal-authorized, an invalid transition overwrites the original, or an allocation crosses scope or exceeds its amount.
- **Validation:** V-DOM, V-MONEY · migration: none · rollback: remove unused registration before consumers ship
- **Risks and assumptions:** The [document specification](../modules/documents.md) supplies the detailed boundary and states.
- **Done when:** Independent status and allocation vectors pass with synthetic data.

### EMP-DOC-002 — Document persistence, upload session and audited private access
- **Meta:** module DOC · phase F3 · cut C1 · priority P0 · type API · size M · state TODO
- **Objective:** Persist scoped documents and deliver private originals through authorized access.
- **Rules and use cases:** BR-TEN-006, BR-PRIV-004, BR-PRIV-008, BR-PRIV-020, BR-AUD-003, OQ-04, UC-PRIV-05
- **Depends on:** EMP-DOC-001, EMP-PLAT-006, EMP-AUTH-003, EMP-PRIV-005
- **Read first:** [privacy and security](../11-privacy-security.md) §§4–5, [offline synchronization](../10-offline-sync.md) §5, [approved receipt scope](../evidence/f0-m0-decisions.md#private-receipts)
- **Scope:** in: synthetic C1 metadata, upload session and audited private reads · out: real receipt use or unrestricted storage access
- **Areas:** existing: private storage and sensitive-read audit conventions · proposed: document metadata, upload and read adapters
- **Steps:** Pin private ingress, allowed and denied reads, stale sessions and storage failure. Bind an audited short-lived access capability to the member and document.
- **Deliverable:** A scoped document API, storage integration and access audit.
- **Acceptance:** Only the permitted member can read the named private document. An upload without verified bytes remains pending.
- **Negative cases:** Foreign or expired capability, denied or unavailable audit, retried upload creating another record, or content exposed in telemetry.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: additive with populated upgrade and recovery · rollback: stop new intake and keep retained objects private
- **Risks and assumptions:** The approved C1 path uses synthetic receipts. Real intake requires legal and release gates.
- **Done when:** Real PostGIS and isolated storage prove authorization, audit, failure and replay behavior.

### EMP-DOC-003 — Links between documents and operations
- **Meta:** module DOC · phase F3 · cut C1 · priority P0 · type API · size S · state TODO
- **Objective:** Associate documents with same-organization operations and records without granting new access.
- **Rules and use cases:** BR-TEN-003, BR-TEN-005, BR-TEN-006, BR-TEN-007, BR-PRIV-004, BR-AUD-001, BR-OFFL-015, REQ-10.1, REQ-10.4
- **Depends on:** EMP-DOC-002
- **Read first:** [domain model](../06-domain-model.md) §§3–5, [offline synchronization](../10-offline-sync.md) §§3–5, [approved receipt scope](../evidence/f0-m0-decisions.md#private-receipts)
- **Scope:** in: scoped link creation and reads for approved consumers, including fueling and meter evidence · out: copying original bytes, allocating money or changing fiscal status
- **Areas:** existing: organization scope, operation envelope and document storage conventions · proposed: document link port and scoped transaction
- **Steps:** Check authority on both endpoints of a link. Cover a pending attachment, repeated commands and references that cannot be resolved in the organization.
- **Deliverable:** An auditable association API and a document link port for operation consumers.
- **Acceptance:** Authorized records can reference the correct private document in the same organization. Link creation is idempotent and conveys no content-read privilege.
- **Negative cases:** Foreign target or document, duplicate link under replay, forged target type, unauthorized file access through the linked record, or a link counted as a cost.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: additive scoped relationships · rollback: close the write path without removing accepted evidence
- **Risks and assumptions:** Each consumer owns the target record's permissions. An association and a financial allocation are separate.
- **Done when:** Same-organization link, pending upload, duplicate retry and foreign-target denials pass through normal consumer entry points.

### EMP-DOC-004 — App: capture and import with offline queue and legibility check
- **Meta:** module DOC · phase F3 · cut C1 · priority P0 · type APP · size M · state TODO
- **Objective:** Capture or import a synthetic receipt into the protected Android attachment queue.
- **Rules and use cases:** BR-OFFL-002, BR-OFFL-003, BR-OFFL-013, BR-OFFL-015, BR-PRIV-016, BR-PRIV-020, UC-OFFL-03, REQ-10.1
- **Depends on:** EMP-DOC-002, EMP-OFFL-005
- **Read first:** [offline synchronization](../10-offline-sync.md) §§4–5 and 10.3, [privacy and security](../11-privacy-security.md) §4, [approved receipt scope](../evidence/f0-m0-decisions.md#private-receipts)
- **Scope:** in: camera or file selection, legibility feedback and durable synthetic attachment queue · out: OCR or real receipt use
- **Areas:** existing: Android camera/file flows and scoped queue · proposed: enterprise capture and import entry points
- **Steps:** Exercise capture, import, cancellation and restart. Preserve the selected original and show pending and failed attachment states explicitly.
- **Deliverable:** An accessible receipt capture and import flow with durable private state.
- **Acceptance:** A synthetic file stays in its creation partition and persists until upload can be verified. A legibility hint does not claim fiscal validation.
- **Negative cases:** Missing file, denied permission, process death after local save, wrong organization or unsupported file presented as stored.
- **Validation:** V-APP, V-SYNC, V-ISO, V-PRIV · migration: preserve personal data and queued bytes · rollback: hide new capture and retain pending state
- **Risks and assumptions:** Only document types authorized by the owning design are offered. C1 uses synthetic receipts.
- **Done when:** Capture, import and interrupted upload work through the normal Android flow.

### EMP-DOC-005 — Conference workflow
- **Meta:** module DOC · phase F3 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Record a human document review without changing fiscal or reconciliation claims.
- **Rules and use cases:** EDR-014, BR-TRUTH-001, BR-TRUTH-002, BR-REV-003, BR-REV-005, BR-REV-007, BR-AUD-001, REQ-10.2
- **Depends on:** EMP-DOC-003
- **Read first:** [state model](../08-state-model.md) §3, [domain model](../06-domain-model.md) §3, [document requirements](../17-traceability.md) §10
- **Scope:** in: authorized human review, outcome, reason and revision · out: fiscal consultation and automatic reconciliation
- **Areas:** existing: review and audit conventions · proposed: document review commands
- **Steps:** Pin review outcomes, required reasons and a stale-decision race. Store the reviewed revision separately from receipt and fiscal status.
- **Deliverable:** A scoped human-review workflow and immutable review evidence.
- **Acceptance:** The reviewer, time and outcome refer to one document revision. The reviewed status does not change a fiscal assertion.
- **Negative cases:** Self-review, stale revision, missing justification, cross-organization reviewer and a review overwriting the original document.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive review history · rollback: stop new decisions and preserve prior outcomes
- **Risks and assumptions:** Use the status names and review rules in the [document design](../modules/documents.md).
- **Done when:** Authorized and denied reviews pass with one auditable outcome per accepted decision.

### EMP-DOC-006 — Fiscal structure: access key, NFC-e QR code and XML structural validation
- **Meta:** module DOC · phase F3 · cut - · priority P1 · type DOM · size M · state TODO
- **Objective:** Distinguish locally checked fiscal structure from authorized fiscal status.
- **Rules and use cases:** EDR-014, BR-TRUTH-001, BR-TRUTH-002, BR-PRIV-020, REQ-10.2, REQ-10.4
- **Depends on:** EMP-DOC-002
- **Read first:** [state model](../08-state-model.md) §3, [document requirements](../17-traceability.md) §10, [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: pure access-key, QR reference and XML structure checks · out: provider queries, remote downloads and fiscal authorization
- **Areas:** existing: exact parsing conventions and upload guardrails · proposed: bounded fiscal structure parser
- **Steps:** Confirm authoritative formats when implementation is authorized. Pin accepted structures and malformed, unsupported and unsafe inputs.
- **Deliverable:** A pure structure result with separate unsupported and invalid outcomes.
- **Acceptance:** A parse reports only the checks performed. Valid structure gives no assurance about current fiscal authorization.
- **Negative cases:** Malformed or oversized XML, external entities, unexpected QR input and a structurally valid item marked fiscally authorized.
- **Validation:** V-DOM, V-PRIV · migration: none · rollback: disable optional structure checks while preserving originals
- **Risks and assumptions:** Review any new parser dependency and the applicable formats separately.
- **Done when:** Synthetic structural and rejection vectors pass without provider contact.

### EMP-DOC-007 — Fiscal items and allocations without double counting
- **Meta:** module DOC · phase F3 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Link exact fiscal item amounts to expense allocations without recognizing a duplicate cost.
- **Rules and use cases:** BR-MONEY-001, BR-MONEY-004, BR-MONEY-007, BR-TEN-005, VAL-09, VAL-12, REQ-10.5
- **Depends on:** EMP-DOC-006, EMP-EXP-002
- **Read first:** [domain model](../06-domain-model.md) §§4–5, [validation scenarios](../15-validation.md) §4, [document requirements](../17-traceability.md) §10
- **Scope:** in: fiscal item persistence and exact scoped allocations · out: creating a second expense from the same source
- **Areas:** existing: document and expense ports, exact-value conventions · proposed: item and allocation persistence
- **Steps:** Pin rounding vectors and concurrent allocation against the same source. Validate both item and document totals in the transaction.
- **Deliverable:** Fiscal items and allocations with tested amount and scope constraints.
- **Acceptance:** Allocated centavos reconcile with the recorded basis and cannot exceed the item or document total. A link alone does not recognize a cost.
- **Negative cases:** Race exceeds the source amount, incompatible units are added, foreign expense linked, or one receipt counted twice.
- **Validation:** V-SQL, V-ISO, V-IDEM, V-MONEY · migration: additive with populated upgrade · rollback: stop new allocations and keep reviewed history
- **Risks and assumptions:** Use the owning expense workflow for recognition and the canonical document rules for detailed fields.
- **Done when:** Exact arithmetic, race and rollback vectors pass on real PostGIS.

### EMP-DOC-008 — Duplicate detection by access key and content hash
- **Meta:** module DOC · phase F3 · cut - · priority P1 · type DB · size S · state TODO
- **Objective:** Identify same-organization document duplicates without merging records automatically.
- **Rules and use cases:** BR-TEN-006, BR-TRUTH-002, BR-PRIV-008, BR-OFFL-004, REQ-10.4
- **Depends on:** EMP-DOC-002
- **Read first:** [document requirements](../17-traceability.md) §10, [domain model](../06-domain-model.md) §5, [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: scoped key and digest matching for review · out: fiscal verification, cross-organization lookup and automatic merges
- **Areas:** existing: document metadata and digest verification · proposed: scoped duplicate checks
- **Steps:** Exercise matching key, matching bytes and changed-content retry cases with synthetic data. Keep duplicate evidence scoped.
- **Deliverable:** A scoped duplicate signal distinct from upload replay.
- **Acceptance:** A matching document can be presented for authorized review. An operation retry retains its original result.
- **Negative cases:** Cross-organization existence hint, hash treated as authenticity, a legitimate multi-link receipt automatically removed, or inconsistent concurrent matching.
- **Validation:** V-SQL, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive lookup indexes · rollback: disable detection without changing documents
- **Risks and assumptions:** A repeated source and repeated transport are distinct. A signal does not determine review outcome.
- **Done when:** Matching, denial and concurrent intake tests pass on scoped fixtures.

### EMP-DOC-009 — Optional on-device extraction with per-field confidence
- **Meta:** module DOC · phase F3 · cut - · priority P2 · type APP · size M · state TODO
- **Objective:** Offer optional on-device field extraction with explicit human confirmation.
- **Rules and use cases:** EDR-014, BR-TRUTH-001, BR-TRUTH-002, BR-PRIV-005, BR-PRIV-008, REQ-10.3
- **Depends on:** EMP-DOC-004
- **Read first:** [document requirements](../17-traceability.md) §10, [state model](../08-state-model.md) §3, [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: opt-in device extraction, field confidence and manual fallback · out: sending receipt content to an external extractor or approving values automatically
- **Areas:** existing: Android private attachment flow and device extraction precedent · proposed: enterprise extraction entry and field review
- **Steps:** Exercise unreadable and ambiguous fields. Preserve the original and distinguish suggestions from confirmed values.
- **Deliverable:** An optional on-device extraction flow with confidence and manual entry.
- **Acceptance:** Manual entry works independently. Extracted data has its own status and remains reviewable.
- **Negative cases:** Low-confidence value silently accepted, parser error blocks manual entry, raw content logged or extraction described as fiscal authorization.
- **Validation:** V-APP, V-PRIV, V-ISO · migration: preserve stored originals and user-confirmed fields · rollback: disable extraction and keep manual entry
- **Risks and assumptions:** The feature follows its own dependency and data-use review. It is outside synthetic C1 acceptance.
- **Done when:** Synthetic readable and unreadable cases pass without moving private bytes outside the approved partition.

### EMP-DOC-010 — Reconciliation status between documents and operations
- **Meta:** module DOC · phase F3 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Recompute document reconciliation from the current authorized operations and allocations.
- **Rules and use cases:** BR-TRUTH-001, BR-TRUTH-003, BR-TRUTH-004, BR-MONEY-007, BR-AMD-005, VAL-12, REQ-10.5
- **Depends on:** EMP-DOC-007
- **Read first:** [domain model](../06-domain-model.md) §§4–5, [state model](../08-state-model.md) §3, [validation scenarios](../15-validation.md) §4
- **Scope:** in: idempotent reconciliation status and basis · out: fiscal status and changing the underlying paid amounts
- **Areas:** existing: scoped jobs and document/expense linkage · proposed: reconciliation worker
- **Steps:** Pin matched, divergent and insufficient-input outcomes. Recompute from versioned source state and converge on retry.
- **Deliverable:** A reconciliation result and reason tied to source revisions.
- **Acceptance:** The result describes reconciliation only. Reprocessing after changed inputs creates an identifiable result without duplicate cost.
- **Negative cases:** Incomplete data becomes matched, foreign operation enters a job, stale job supersedes a newer result or retry adds a second amount.
- **Validation:** V-SQL, V-ISO, V-IDEM, V-JOB, V-MONEY · migration: rehearse additive result storage if needed · rollback: pause jobs and preserve recorded results
- **Risks and assumptions:** The [document design](../modules/documents.md) and state catalog govern reconciliation.
- **Done when:** Recompute, job failure, out-of-order delivery and scoped arithmetic cases pass.

### EMP-DOC-011 — Replacement and cancellation of documents
- **Meta:** module DOC · phase F3 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Replace or cancel a document with an auditable history.
- **Rules and use cases:** BR-TRUTH-001, BR-TRUTH-002, BR-AMD-001, BR-AMD-003, BR-AUD-001, REQ-10.4
- **Depends on:** EMP-DOC-005
- **Read first:** [state model](../08-state-model.md) §3, [domain model](../06-domain-model.md) §5, [document requirements](../17-traceability.md) §10
- **Scope:** in: authorized replacement and internal cancellation references · out: deleting original evidence in place or claiming fiscal cancellation
- **Areas:** existing: document review, revision and audit conventions · proposed: replacement and cancellation commands
- **Steps:** Pin allowed transitions, stale revisions and concurrent replacement. Re-evaluate affected links or projections through owning workflows.
- **Deliverable:** A revision-aware document history and audited cancellation result.
- **Acceptance:** Old and new references are traceable. An internal outcome does not change an externally verified status.
- **Negative cases:** Foreign replacement, missing reason, duplicate revision under replay, original silently lost or internal cancellation shown as fiscal cancellation.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: preserve historical references · rollback: close new commands and keep prior changes
- **Risks and assumptions:** Original bytes follow the approved retention schedule rather than the convenience of a UI action.
- **Done when:** Revision, race and denial tests pass with audit and history intact.

### EMP-DOC-012 — Retention, deletion and legal hold by category
- **Meta:** module DOC · phase F6 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Enforce approved private document retention, legal holds and erasure.
- **Rules and use cases:** OQ-04, LV-04, LV-11, BR-PRIV-009, BR-PRIV-010, BR-PRIV-011, VAL-38
- **Depends on:** EMP-DOC-002, EMP-GOV-002
- **Read first:** [privacy and security](../11-privacy-security.md) §§4 and 10, [approved receipt scope](../evidence/f0-m0-decisions.md#private-receipts), [validation scenarios](../15-validation.md) §4
- **Scope:** in: reviewed category rules, holds, read-time expiration and recoverable cleanup · out: selecting retention without legal review
- **Areas:** existing: deletion ledger and private storage conventions · proposed: organization document retention jobs
- **Steps:** Require a recorded policy for each real-data category. Test originals, derivatives, backups and restoration with a controlled clock.
- **Deliverable:** A tested retention and hold lifecycle for approved categories.
- **Acceptance:** Expired files stay inaccessible during a cleanup outage. Approved holds and erasures remain scoped and survive replay.
- **Negative cases:** Unreviewed duration becomes a default, a held object is erased, a derivative survives confirmed erasure, or a restore serves expired content.
- **Validation:** V-SQL, V-JOB, V-ISO, V-IDEM, V-PRIV · migration: rehearsal with retained files and deletion history · rollback: preserve read-time denial and pending evidence
- **Risks and assumptions:** This task does not amend community photo or claim-scan rules. Real receipts still require legal and production gates.
- **Done when:** Expiry, hold, cleanup failure and isolated restore pass on real scoped fixtures.

### EMP-DOC-013 — Panel: document inbox, viewer, conference and allocation
- **Meta:** module DOC · phase F3 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Give company reviewers a scoped inbox, viewer and allocation workflow.
- **Rules and use cases:** BR-AUTH-011, BR-PRIV-004, BR-AUD-003, BR-MONEY-007, BR-TRUTH-001, REQ-10.5
- **Depends on:** EMP-DOC-005, EMP-DOC-007, EMP-PLAT-008
- **Read first:** [document requirements](../17-traceability.md) §§10 and 20, [privacy and security](../11-privacy-security.md) §4, [approved receipt scope](../evidence/f0-m0-decisions.md#private-receipts)
- **Scope:** in: approved company panel document metadata, authorized content and review or allocation views · out: public or admin visibility
- **Areas:** existing: /empresas/ scaffold and owning document APIs · proposed: private document panel screens
- **Steps:** Exercise metadata-only access, content denial, revision conflicts and amount feedback through the existing company session.
- **Deliverable:** An accessible private document inbox and reviewer workflow.
- **Acceptance:** Each viewer action respects current permission and shows distinct review and reconciliation status.
- **Negative cases:** Foreign response after switching organizations, persistent object URL, misleading fiscal state and invalid allocation shown as saved.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-MONEY, V-PRIV · migration: none · rollback: hide new views without dropping accepted records
- **Risks and assumptions:** Detailed screens follow the restored web and document specifications.
- **Done when:** Company panel role, scope, accessibility and conflict cases pass.

### EMP-DOC-014 — Upload safety checks: type, size and active content
- **Meta:** module DOC · phase F3 · cut C1 · priority P0 · type JOB · size S · state TODO
- **Objective:** Inspect uploads before allowing storage state to reach ready.
- **Rules and use cases:** BR-PRIV-020, BR-PRIV-004, BR-PRIV-008, BR-OFFL-015, UC-OFFL-03
- **Depends on:** EMP-DOC-002
- **Read first:** [privacy and security](../11-privacy-security.md) §4, [offline synchronization](../10-offline-sync.md) §§5–6, [approved receipt scope](../evidence/f0-m0-decisions.md#private-receipts)
- **Scope:** in: bounded type, size, integrity and active-content checks · out: serving unverified bytes or real receipt activation
- **Areas:** existing: private ingress, worker and upload declaration · proposed: document safety job
- **Steps:** Exercise declared and observed bounds, unsafe formats, interrupted checks and repeated jobs. Preserve the authoritative quarantine or failure state.
- **Deliverable:** A retry-safe safety check in the private upload flow.
- **Acceptance:** Only a verified, permitted object becomes available. Failure keeps bytes inaccessible and leaves a truthful status.
- **Negative cases:** Oversized or unexpected format accepted, mismatched digest, partial upload marked ready, cross-organization object or unverified bytes served after restart.
- **Validation:** V-JOB, V-ISO, V-IDEM, V-PRIV · migration: preserve existing metadata and test restart · rollback: close access to unverified objects
- **Risks and assumptions:** Use synthetic fixtures and reuse approved bounded primitives.
- **Done when:** Pass and refusal cases converge across worker retry and storage failure.

### EMP-DOC-015 — Panel: receipt viewer inside fueling review
- **Meta:** module DOC · phase F3 · cut C1 · priority P0 · type WEB · size S · state TODO
- **Objective:** Show the authorized synthetic receipt within company fueling review.
- **Rules and use cases:** BR-AUTH-011, BR-PRIV-004, BR-AUD-003, BR-OFFL-015, BR-TRUTH-001, OQ-04, UC-PRIV-05
- **Depends on:** EMP-DOC-003, EMP-FUEL-012
- **Read first:** [approved receipt scope](../evidence/f0-m0-decisions.md#private-receipts), [privacy and security](../11-privacy-security.md) §4, [fueling specification](../modules/fuelings.md)
- **Scope:** in: same-organization C1 receipt viewer and pending or denied states · out: unreviewed real receipts and granting permission from fueling read access
- **Areas:** existing: company panel, fueling review and document link port · proposed: scoped embedded receipt viewer
- **Steps:** Exercise linked and pending synthetic receipts, separate content permission, expiry and audited access through the company session.
- **Deliverable:** A focused receipt view inside fueling review.
- **Acceptance:** An authorized reviewer opens only the linked document through an audited private capability. Review remains possible when the file is unavailable under its owning rules.
- **Negative cases:** Fueling read implies document bytes, expired or foreign capability works, or the UI treats a pending attachment as verified.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: none · rollback: hide the viewer and preserve review records
- **Risks and assumptions:** Synthetic scope is approved for C1. Real-data activation has separate gates.
- **Done when:** The integrated viewer proves allowed, expired, pending and denied cases.
