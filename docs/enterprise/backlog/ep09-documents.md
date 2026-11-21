# EP09 — Receipts, fiscal documents and attachments

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from approved receipt scope, [domain model](../domain-model.md), [state model](../state-model.md), [privacy and security](../privacy-security.md) and document requirements.

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
sequencing decision.

The [document specification](../modules/documents.md) now records the recovered design. This backlog uses the surviving shared rules and approved synthetic C1 scope. Real-receipt intake, retention and fiscal consultation retain their separate gates.

### EMP-DOC-005 — Conference workflow
- **Meta:** module DOC · phase F3 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Record a human document review without changing fiscal or reconciliation claims.
- **Rules and use cases:** EDR-014, BR-TRUTH-001, BR-TRUTH-002, BR-REV-003, BR-REV-005, BR-REV-007, BR-AUD-001, REQ-10.2
- **Depends on:** EMP-DOC-003
- **Read first:** [state model](../state-model.md) §3, [domain model](../domain-model.md) §3, document requirements §10
- **Scope:** in: authorized human review, outcome, reason and revision · out: fiscal consultation and automatic reconciliation
- **Areas:** existing: review and audit conventions · proposed: document review commands
- **Steps:** Pin review outcomes, required reasons and a stale-decision race. Store the reviewed revision separately from receipt and fiscal status.
- **Deliverable:** A scoped human-review workflow and immutable review evidence.
- **Acceptance:** The reviewer, time and outcome refer to one document revision. The reviewed status does not change a fiscal assertion.
- **Negative cases:** Self-review, stale revision, missing justification, cross-organization reviewer and a review overwriting the original document.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive review history · rollback: stop new decisions and preserve prior outcomes
- **Risks and assumptions:** Use the status names and review rules in the [document design](../modules/documents.md).
- **Done when:** Authorized and denied reviews pass with one auditable outcome per accepted decision.

### EMP-DOC-007 — Fiscal items and allocations without double counting
- **Meta:** module DOC · phase F3 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Link exact fiscal item amounts to expense allocations without recognizing a duplicate cost.
- **Rules and use cases:** BR-MONEY-001, BR-MONEY-004, BR-MONEY-007, BR-TEN-005, VAL-09, VAL-12, REQ-10.5
- **Depends on:** EMP-DOC-006, EMP-EXP-002
- **Read first:** [domain model](../domain-model.md) §§4–5, [validation scenarios](../testing.md) §4, document requirements §10
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
- **Read first:** document requirements §10, [domain model](../domain-model.md) §5, [privacy and security](../privacy-security.md) §4
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
- **Read first:** document requirements §10, [state model](../state-model.md) §3, [privacy and security](../privacy-security.md) §4
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
- **Read first:** [domain model](../domain-model.md) §§4–5, [state model](../state-model.md) §3, [validation scenarios](../testing.md) §4
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
- **Read first:** [state model](../state-model.md) §3, [domain model](../domain-model.md) §5, document requirements §10
- **Scope:** in: authorized replacement and internal cancellation references · out: deleting original evidence in place or claiming fiscal cancellation
- **Areas:** existing: document review, revision and audit conventions · proposed: replacement and cancellation commands
- **Steps:** Pin allowed transitions, stale revisions and concurrent replacement. Re-evaluate affected links or projections through owning workflows.
- **Deliverable:** A revision-aware document history and audited cancellation result.
- **Acceptance:** Old and new references are traceable. An internal outcome does not change an externally verified status.
- **Negative cases:** Foreign replacement, missing reason, duplicate revision under replay, original silently lost or internal cancellation shown as fiscal cancellation.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: preserve historical references · rollback: close new commands and keep prior changes
- **Risks and assumptions:** Original bytes follow the approved retention schedule rather than the convenience of a UI action.
- **Done when:** Revision, race and denial tests pass with audit and history intact.

### EMP-DOC-013 — Panel: document inbox, viewer, conference and allocation
- **Meta:** module DOC · phase F3 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Give company reviewers a scoped inbox, viewer and allocation workflow.
- **Rules and use cases:** BR-AUTH-011, BR-PRIV-004, BR-AUD-003, BR-MONEY-007, BR-TRUTH-001, REQ-10.5
- **Depends on:** EMP-DOC-005, EMP-DOC-007, EMP-PLAT-008
- **Read first:** document requirements §§10 and 20, [privacy and security](../privacy-security.md) §4, approved receipt scope
- **Scope:** in: approved company panel document metadata, authorized content and review or allocation views · out: public or admin visibility
- **Areas:** existing: /empresas/ scaffold and owning document APIs · proposed: private document panel screens
- **Steps:** Exercise metadata-only access, content denial, revision conflicts and amount feedback through the existing company session.
- **Deliverable:** An accessible private document inbox and reviewer workflow.
- **Acceptance:** Each viewer action respects current permission and shows distinct review and reconciliation status.
- **Negative cases:** Foreign response after switching organizations, persistent object URL, misleading fiscal state and invalid allocation shown as saved.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-MONEY, V-PRIV · migration: none · rollback: hide new views without dropping accepted records
- **Risks and assumptions:** Detailed screens follow the restored web and document specifications.
- **Done when:** Company panel role, scope, accessibility and conflict cases pass.

### EMP-DOC-015 — Panel: receipt viewer inside fueling review
- **Meta:** module DOC · phase F3 · cut C1 · priority P0 · type WEB · size S · state TODO
- **Objective:** Show the authorized synthetic receipt within company fueling review.
- **Rules and use cases:** BR-AUTH-011, BR-PRIV-004, BR-AUD-003, BR-OFFL-015, BR-TRUTH-001, OQ-04, UC-PRIV-05
- **Depends on:** EMP-DOC-003, EMP-FUEL-012
- **Read first:** approved receipt scope, [privacy and security](../privacy-security.md) §4, [fueling specification](../modules/fuelings.md)
- **Scope:** in: same-organization C1 receipt viewer and pending or denied states · out: unreviewed real receipts and granting permission from fueling read access
- **Areas:** existing: company panel, fueling review and document link port · proposed: scoped embedded receipt viewer
- **Steps:** Exercise linked and pending synthetic receipts, separate content permission, expiry and audited access through the company session.
- **Deliverable:** A focused receipt view inside fueling review.
- **Acceptance:** An authorized reviewer opens only the linked document through an audited private capability. Review remains possible when the file is unavailable under its owning rules.
- **Negative cases:** Fueling read implies document bytes, expired or foreign capability works, or the UI treats a pending attachment as verified.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: none · rollback: hide the viewer and preserve review records
- **Risks and assumptions:** Synthetic scope is approved for C1. Real-data activation has separate gates.
- **Done when:** The integrated viewer proves allowed, expired, pending and denied cases.
