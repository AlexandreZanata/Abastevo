# EP01 — Platform foundations and privacy

Task metadata follows the existing [generated index](INDEX.md). The task
definitions reconstruct the missing backlog from the approved contract,
architecture, privacy rules and validation plan. The owning source phases
must produce implementation evidence after the applicable entry gates.
Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the sequencing decision.

### EMP-PLAT-012 — Transactional e-mail port and adapter for enterprise notices
- **Meta:** module PLAT · phase F1 · cut - · priority P1 · type INT · size S · state TODO
- **Objective:** Add transactional email only if the delivery decision selects it.
- **Rules and use cases:** EDR-032, OQ-09, BR-PRIV-005, BR-PRIV-008
- **Depends on:** EMP-GOV-014, EMP-PLAT-004
- **Read first:** [architecture](../architecture.md), [privacy and security](../privacy-security.md)
- **Scope:** in: approved notification port, bounded retries and scoped delivery · out: unapproved provider or production sends
- **Areas:** existing: job queue and planned in-app path · proposed: transactional email port and reviewed adapter
- **Steps:** Confirm the approved channel and review provider and dependency scope. Test in-app fallback and retries with synthetic fixtures.
- **Deliverable:** A delivery port and approved adapter behind its own off flag.
- **Acceptance:** The in-app path works independently. Retries are bounded and an accepted send is not duplicated.
- **Negative cases:** Wrong recipient or organization, credential in logs, disabled provider sends, retry storm and delivery outage.
- **Validation:** V-JOB, V-IDEM, V-PRIV · migration: owning notification storage remains additive · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** A channel approval is separate from credentials, provider setup and production use.
- **Done when:** The authorized adapter and in-app fallback pass scoped delivery tests.

### EMP-PRIV-001 — Enterprise data inventory and retention schedule enforcement
- **Meta:** module PRIV · phase F6 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Enforce approved retention by organization data category.
- **Rules and use cases:** BR-PRIV-009, BR-PRIV-010, BR-PRIV-011, BR-PRIV-012, LV-04, LV-11
- **Depends on:** EMP-PLAT-010, EMP-GOV-005
- **Read first:** [privacy and security](../privacy-security.md), F0 decisions, [validation scenarios](../testing.md)
- **Scope:** in: inventory, recorded policies, read-time expiry and cleanup · out: choosing retention without legal review
- **Areas:** existing: privacy cleanup and job patterns · proposed: organization inventory and policy enforcement
- **Steps:** Register categories and reviewed expiry rules. Exercise overdue, held and failed cleanup paths on scoped fixtures.
- **Deliverable:** Inventory and tested retention enforcement for approved categories.
- **Acceptance:** Expired content is inaccessible even during an outage. Holds and retry outcomes are scoped and audited.
- **Negative cases:** Unapproved default, expired content served, foreign hold, storage failure or duplicate cleanup.
- **Validation:** V-SQL, V-JOB, V-PRIV, V-ISO · migration: append-only with recovery evidence · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** No real receipt category activates without its legal policy.
- **Done when:** Expiry, legal hold, retry and recovery pass with controlled time and real storage boundaries.

### EMP-PRIV-002 — Data-subject request routing between the organization and the platform
- **Meta:** module PRIV · phase F6 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Route employee data requests to the responsible organization or platform flow.
- **Rules and use cases:** BR-PRIV-013, BR-PRIV-014, UC-PRIV-01, LV-02
- **Depends on:** EMP-IDN-003, EMP-GOV-005
- **Read first:** [privacy and security](../privacy-security.md), F0 decisions
- **Scope:** in: scoped request intake, operator handoff and outcomes · out: unapproved controller assignment or unrelated personal data
- **Areas:** existing: account/privacy interfaces · proposed: organization rights request tracking and export ports
- **Steps:** Use the reviewed role and purpose inventory to map requests. Test scoped handling and restricted items.
- **Deliverable:** An auditable request workflow that selects the correct responsible context.
- **Acceptance:** A response covers only authorized data in the named context and records any applicable restriction.
- **Negative cases:** Foreign organization records, access by ordinary staff, duplicate outcomes or improper deletion of an organization record.
- **Validation:** V-API, V-AUTHZ, V-ISO, V-PRIV · migration: additive with scoped request-state recovery · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** The legal review governs production responsibilities and policy.
- **Done when:** Routing, scoped evidence and negative access pass before activation.

### EMP-PRIV-004 — Backup, restore and deletion-ledger replay for enterprise data
- **Meta:** module PRIV · phase F6 · cut - · priority P1 · type OPS · size M · state TODO
- **Objective:** Prevent deleted or expired enterprise records from reappearing after restore.
- **Rules and use cases:** BR-PRIV-009, BR-PRIV-011, UC-PRIV-04
- **Depends on:** EMP-PLAT-002
- **Read first:** [privacy and security](../privacy-security.md), [validation scenarios](../testing.md)
- **Scope:** in: restore gating, deletion-ledger replay and scope verification · out: production restore until separately authorized
- **Areas:** existing: backup and privacy erasure replay · proposed: organization restore integration and rehearsal
- **Steps:** Restore an isolated populated snapshot. Replay the external deletion record before reads and verify failure keeps service closed.
- **Deliverable:** A tested organization restore procedure with replay receipts.
- **Acceptance:** Previously erased data stays inaccessible after restore and repeated replay converges.
- **Negative cases:** Missing ledger, partially restored bytes, foreign organization purge, replay interruption and stale read admission.
- **Validation:** V-SQL, V-PRIV, V-JOB, V-ISO · migration: test forward restore and previous image compatibility · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** The privacy ledger must survive independently of the restored snapshot.
- **Done when:** An isolated restore proves replay, failure recovery and access gating.
