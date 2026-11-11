# EP11 — Maintenance, checklists and complementary fleet controls

Status: **PLANNED**, 2026-11-10.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

The existing maintenance specification owns the threshold, state and evidence rules.

### EMP-CHECK-001 — Checklist templates per asset category
- **Meta:** module CHECK · phase F4 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Publish immutable checklist templates by category and approved scope.
- **Rules and use cases:** BR-CHECK-001, BR-CHECK-002, BR-CHECK-003, BR-CHECK-005
- **Depends on:** EMP-FLEET-002
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: template revision, applicable context and critical-item policy · out: retroactive changes to completed runs
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A run pins its applicable template version and the current organization policy.
- **Acceptance:** A run pins its applicable template version and the current organization policy.
- **Negative cases:** Foreign template, invalid answer policy or prior run rewritten after publication.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-CHECK-002 — Checklist execution offline with the critical-item policy
- **Meta:** module CHECK · phase F4 · cut - · priority P1 · type APP · size M · state TODO
- **Objective:** Run a checklist offline and preserve the server's independent outcome.
- **Rules and use cases:** BR-CHECK-004, BR-CHECK-005, BR-CHECK-006, BR-CHECK-007, BR-CHECK-008, BR-OFFL-002
- **Depends on:** EMP-CHECK-001, EMP-OFFL-002, EMP-OFFL-003
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: cached template, answers, durable queue and blocking state · out: treating offline authority as current
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The local outcome is visible and the server rechecks it. A disagreement preserves the field fact.
- **Acceptance:** The local outcome is visible and the server rechecks it. A disagreement preserves the field fact.
- **Negative cases:** Unpersisted answers, wrong template, critical block ignored or a server disagreement drops the record.
- **Validation:** V-APP, V-SYNC, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-CHECK-003 — Damage and incident reports with photos
- **Meta:** module CHECK · phase F4 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Record scoped damage or incident evidence without directly recognizing costs.
- **Rules and use cases:** BR-CHECK-014, BR-CHECK-015, BR-CHECK-016, BR-CHECK-018, BR-PRIV-004, OQ-04
- **Depends on:** EMP-DOC-003, EMP-TRIP-003
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: bounded description and authorized private attachments · out: unapproved real photos or duplicate expense
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A report is durable offline and its attachment status and permission are independent.
- **Acceptance:** A report is durable offline and its attachment status and permission are independent.
- **Negative cases:** Private content exposed, failed upload described as ready, report automatically books a cost or cross-scope link.
- **Validation:** V-API, V-SQL, V-APP, V-ISO, V-PRIV, V-SYNC · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-CHECK-004 — Panel: checklists, damages and incidents
- **Meta:** module CHECK · phase F4 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Let authorized company users review checklists, incidents and damages.
- **Rules and use cases:** BR-CHECK-010, BR-CHECK-011, BR-CHECK-017, BR-AUTH-011
- **Depends on:** EMP-CHECK-002, EMP-CHECK-003, EMP-PLAT-008
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: scoped list, treatment and approved release actions · out: implicit content access
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The panel shows the actual asset block and records an authorized release without altering historical evidence.
- **Acceptance:** The panel shows the actual asset block and records an authorized release without altering historical evidence.
- **Negative cases:** Self-release where forbidden, hidden block, document permission bypass or foreign incident.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-MAINT-001 — Maintenance domain: plans, triggers and work orders
- **Meta:** module MAINT · phase F6 · cut - · priority P1 · type DOM · size M · state TODO
- **Objective:** Define maintenance plans, triggers and work orders as independent facts.
- **Rules and use cases:** BR-MAINT-001, BR-MAINT-002, BR-MAINT-004, BR-MAINT-006
- **Depends on:** EMP-FLEET-001, EMP-METER-001
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: pure lifecycle and trigger invariants · out: persistence and recognizing cost
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A plan has an eligible trigger and a work order follows explicit transitions.
- **Acceptance:** A plan has an eligible trigger and a work order follows explicit transitions.
- **Negative cases:** Missing required meter, invalid state transition or overlapping open occurrence.
- **Validation:** V-DOM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-MAINT-002 — Preventive plans and due computation from meters and dates
- **Meta:** module MAINT · phase F6 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Compute preventive due dates and usage from approved meter and date evidence.
- **Rules and use cases:** BR-MAINT-001, BR-MAINT-003, BR-MAINT-004, BR-MAINT-005, BR-MAINT-006, BR-MAINT-007, VAL-31
- **Depends on:** EMP-MAINT-001, EMP-METER-006
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: plans, earliest trigger and recompute · out: guessing absent or suspect usage
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The first due trigger wins and corrections rebuild the basis while keeping one open occurrence.
- **Acceptance:** The first due trigger wins and corrections rebuild the basis while keeping one open occurrence.
- **Negative cases:** Wrong local day, replacement corrupts distance, invalid reading triggers a plan or duplicate due item.
- **Validation:** V-DOM, V-SQL, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-MAINT-003 — Corrective requests from checklists, incidents and drivers
- **Meta:** module MAINT · phase F6 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Create a corrective request from an eligible incident, checklist or driver report.
- **Rules and use cases:** BR-CHECK-014, BR-CHECK-017, BR-CHECK-018, BR-TEN-005
- **Depends on:** EMP-MAINT-001, EMP-CHECK-003
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: scoped requests and linked evidence · out: automatic work order costs
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The request preserves its source and repeated delivery converges.
- **Acceptance:** The request preserves its source and repeated delivery converges.
- **Negative cases:** Foreign evidence, duplicate request from one command, lost issue or implied expense.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-MAINT-004 — Work-order lifecycle with quote, approval, parts and services
- **Meta:** module MAINT · phase F6 · cut - · priority P1 · type API · size L · state TODO
- **Objective:** Manage an authorized work order from quote to recorded execution.
- **Rules and use cases:** BR-REV-003, BR-REV-007, BR-MONEY-001, REQ-12.1
- **Depends on:** EMP-MAINT-001, EMP-FLEET-009, EMP-APPR-002
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: parts, service lines, approval and lifecycle · out: provider payment and writing an expense twice
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The recorded decision applies to the exact work-order revision, with parts and service amounts separate.
- **Acceptance:** The recorded decision applies to the exact work-order revision, with parts and service amounts separate.
- **Negative cases:** Self-approval, invalid revision, amount overflow or unauthorized lifecycle change.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-MAINT-005 — Unavailability during maintenance and return to operation
- **Meta:** module MAINT · phase F6 · cut - · priority P1 · type DOM · size S · state TODO
- **Objective:** Apply maintenance unavailability and authorized return to operation.
- **Rules and use cases:** REQ-12.1, BR-CHECK-010, BR-AUD-001, BR-TEN-005
- **Depends on:** EMP-MAINT-004, EMP-FLEET-007
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: asset block lifecycle tied to recorded work · out: bypassing other outstanding blocks
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Return closes the applicable maintenance block and leaves any independent block in force.
- **Acceptance:** Return closes the applicable maintenance block and leaves any independent block in force.
- **Negative cases:** Wrong asset, unauthorized release, double transition or unavailable asset accepted as free.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-MAINT-006 — Maintenance cost posted once as an expense, with attachments
- **Meta:** module MAINT · phase F6 · cut - · priority P1 · type DOM · size S · state TODO
- **Objective:** Post the approved maintenance cost exactly once with scoped evidence.
- **Rules and use cases:** EDR-013, BR-MONEY-001, BR-REV-009, BR-PRIV-004, REQ-12.1, VAL-13
- **Depends on:** EMP-MAINT-004, EMP-EXP-002, EMP-DOC-003
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: work-order origin, company amount and private attachment references · out: duplicate cost recognition from invoices or settlement
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** One authorized work-order origin yields one recognized expense and a replay cannot increase operating cost.
- **Acceptance:** One authorized work-order origin yields one recognized expense and a replay cannot increase operating cost.
- **Negative cases:** Concurrent duplicate expense, foreign attachment, invalid amount or unreviewed real-data intake.
- **Validation:** V-SQL, V-API, V-AUTHZ, V-ISO, V-IDEM, V-MONEY, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-MAINT-007 — Expiry tracking and alerts for asset and driver documents
- **Meta:** module MAINT · phase F6 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Track reviewed expiries and notify the authorized scope.
- **Rules and use cases:** REQ-12.2, BR-IDN-040, BR-PRIV-005, BR-TIME-004
- **Depends on:** EMP-FLEET-010, EMP-IDN-014, EMP-NOTIF-001
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: document expiry state and deduplicated alerts · out: unrestricted document collection
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The expiry and alert basis are visible and private content remains governed by its own permission.
- **Acceptance:** The expiry and alert basis are visible and private content remains governed by its own permission.
- **Negative cases:** Wrong day, repeated alert, foreign employee or unapproved document collection.
- **Validation:** V-API, V-SQL, V-JOB, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-MAINT-008 — Panel: maintenance calendar, plans and work orders
- **Meta:** module MAINT · phase F6 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Present scoped maintenance calendar, plan and work-order detail.
- **Rules and use cases:** REQ-12.1, BR-TRUTH-003, BR-AUTH-013, VAL-31
- **Depends on:** EMP-MAINT-002, EMP-MAINT-004, EMP-PLAT-008
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: authorized lists, due basis and workflow states · out: inferring missing maintenance history
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The panel shows the recorded dates and evidence behind computed due state.
- **Acceptance:** The panel shows the recorded dates and evidence behind computed due state.
- **Negative cases:** Foreign work order, stale due result or unknown usage displayed as zero.
- **Validation:** V-WEB, V-AUTHZ, V-ISO · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-MAINT-009 — App: report a problem and see asset restrictions
- **Meta:** module MAINT · phase F6 · cut - · priority P1 · type APP · size S · state TODO
- **Objective:** Let a driver report a problem and see current asset restrictions.
- **Rules and use cases:** BR-CHECK-014, BR-CHECK-015, BR-OFFL-002, BR-OFFL-010
- **Depends on:** EMP-MAINT-003, EMP-MAINT-005
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: durable scoped report and labelled cached restriction · out: local removal of a block
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A report survives restart, and the restriction shows its confirmation age.
- **Acceptance:** A report survives restart, and the restriction shows its confirmation age.
- **Negative cases:** Report sent under another context, hidden restriction or unpersisted success.
- **Validation:** V-APP, V-SYNC, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-MAINT-010 — Tyres and component history
- **Meta:** module MAINT · phase F6 · cut - · priority P3 · type API · size M · state TODO
- **Objective:** Preserve tyre and component installation and removal history.
- **Rules and use cases:** REQ-12.2, BR-TEN-005, BR-AUD-001
- **Depends on:** EMP-MAINT-004
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: scoped component events and effective periods · out: rewriting old asset records
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Each event retains its asset and historical context.
- **Acceptance:** Each event retains its asset and historical context.
- **Negative cases:** Foreign component, overlapping installation or history lost after removal.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-MAINT-011 — Parts inventory
- **Meta:** module MAINT · phase F6 · cut - · priority P3 · type API · size M · state TODO
- **Objective:** Record scoped part movements and work-order consumption.
- **Rules and use cases:** REQ-12.2, BR-MONEY-001, EDR-013, BR-AUD-001
- **Depends on:** EMP-MAINT-004
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: approved inventory and issue records · out: recognizing duplicate cost
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The movement retains its work-order link and repeated commands do not change stock twice.
- **Acceptance:** The movement retains its work-order link and repeated commands do not change stock twice.
- **Negative cases:** Double issue, unauthorized adjustment, scope error or a movement counted as a second expense.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-MAINT-012 — Insurance policies and claim records
- **Meta:** module MAINT · phase F6 · cut - · priority P2 · type API · size S · state TODO
- **Objective:** Maintain insurance policy and claim references within the approved private scope.
- **Rules and use cases:** REQ-12.2, BR-PRIV-004, BR-PRIV-005, EDR-013
- **Depends on:** EMP-FLEET-010, EMP-CHECK-003
- **Read first:** [maintenance and checklist specification](../modules/maintenance.md), [validation](../15-validation.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: policy periods, claim state and linked approved evidence · out: provider settlement and unreviewed sensitive data
- **Areas:** existing: fleet, audit and queue conventions · proposed: maintenance and checklist packages and scoped consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Claims are scoped, statuses are explicit and costs follow the expense workflow.
- **Acceptance:** Claims are scoped, statuses are explicit and costs follow the expense workflow.
- **Negative cases:** Foreign claim, unapproved sensitive document, duplicate cost or insurer payment inferred from an internal state.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Detailed rules and exact thresholds come from the maintenance specification. Attachments and real-data activation retain their own gates.
- **Done when:** The stated result and denials pass the listed validation profiles.
