# EP12 — Internal fuel stock

Status: **PLANNED**, 2026-11-10.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from [stock requirements](../17-traceability.md), [domain model](../06-domain-model.md), [shared rules](../04-business-rules.md) and [validation](../15-validation.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

Stock receipts and consumption are separate economic events under the recorded valuation decision.

### EMP-STOCK-001 — Internal tank domain: movements and moving-average valuation
- **Meta:** module STOCK · phase F6 · cut - · priority P1 · type DOM · size M · state TODO
- **Objective:** Define internal tank movements and moving-average valuation with exact arithmetic.
- **Rules and use cases:** EDR-029, BR-MONEY-001, BR-MONEY-002, REQ-12.3
- **Depends on:** EMP-PLAT-009, EMP-FUEL-001
- **Read first:** [stock requirements](../17-traceability.md), [domain model](../06-domain-model.md), [shared rules](../04-business-rules.md) and [validation](../15-validation.md)
- **Scope:** in: pure receipt, dispense and valuation basis · out: recognizing purchase and use twice
- **Areas:** existing: fleet, fueling, expense and scoped job conventions · proposed: internal tank and movement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Stock quantity and value evolve from recorded movements and use explicit missing-data results.
- **Acceptance:** Stock quantity and value evolve from recorded movements and use explicit missing-data results.
- **Negative cases:** Float drift, mixed products, absent valuation treated as zero or invalid movement.
- **Validation:** V-DOM, V-MONEY · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [stock specification](../modules/fuel-stock.md) pins the method and boundaries. Real operations retain legal and activation gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-STOCK-002 — Tanks and receipts of purchased fuel into stock
- **Meta:** module STOCK · phase F6 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Persist scoped tanks and received stock with provenance.
- **Rules and use cases:** REQ-12.3, BR-TEN-005, BR-AUD-001, EDR-029
- **Depends on:** EMP-STOCK-001, EMP-FLEET-009, EMP-DOC-003
- **Read first:** [stock requirements](../17-traceability.md), [domain model](../06-domain-model.md), [shared rules](../04-business-rules.md) and [validation](../15-validation.md)
- **Scope:** in: authorized tank and receipt transaction · out: cost recognition at purchase
- **Areas:** existing: fleet, fueling, expense and scoped job conventions · proposed: internal tank and movement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** One accepted receipt changes the scoped stock once and preserves its source.
- **Acceptance:** One accepted receipt changes the scoped stock once and preserves its source.
- **Negative cases:** Foreign tank, replay doubles quantity, wrong unit or partially committed value.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [stock specification](../modules/fuel-stock.md) pins the method and boundaries. Real operations retain legal and activation gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-STOCK-003 — Dispensing to assets as internal fuelings at average cost
- **Meta:** module STOCK · phase F6 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Dispense to an asset as a linked internal fueling at the recorded valuation.
- **Rules and use cases:** REQ-12.3, BR-FUEL-001, BR-FUEL-010, EDR-029
- **Depends on:** EMP-STOCK-002, EMP-FUEL-002, EMP-EXP-002
- **Read first:** [stock requirements](../17-traceability.md), [domain model](../06-domain-model.md), [shared rules](../04-business-rules.md) and [validation](../15-validation.md)
- **Scope:** in: authorized stock issue and fueling handoff · out: duplicate expense or hidden negative balance
- **Areas:** existing: fleet, fueling, expense and scoped job conventions · proposed: internal tank and movement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The source and destination remain scoped and the once-only issue uses the approved cost basis.
- **Acceptance:** The source and destination remain scoped and the once-only issue uses the approved cost basis.
- **Negative cases:** Concurrent double issue, foreign asset, missing valuation or unreviewed overdraft hidden.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [stock specification](../modules/fuel-stock.md) pins the method and boundaries. Real operations retain legal and activation gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-STOCK-004 — Transfers, adjustments, measurements and reconciliation
- **Meta:** module STOCK · phase F6 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Record transfers, measurements and justified adjustments with reconciliation.
- **Rules and use cases:** REQ-12.3, BR-MONEY-001, BR-AUD-001, BR-TRUTH-005
- **Depends on:** EMP-STOCK-002
- **Read first:** [stock requirements](../17-traceability.md), [domain model](../06-domain-model.md), [shared rules](../04-business-rules.md) and [validation](../15-validation.md)
- **Scope:** in: scoped auditable movements · out: rewriting old stock balances or claiming loss without evidence
- **Areas:** existing: fleet, fueling, expense and scoped job conventions · proposed: internal tank and movement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Transfers preserve total compatible quantities and reconciliation states the observed difference.
- **Acceptance:** Transfers preserve total compatible quantities and reconciliation states the observed difference.
- **Negative cases:** Foreign target, duplicate adjustment, mixed product or unexplained negative change.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [stock specification](../modules/fuel-stock.md) pins the method and boundaries. Real operations retain legal and activation gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-STOCK-005 — Single-count guard between stock purchase and consumption
- **Meta:** module STOCK · phase F6 · cut - · priority P1 · type TEST · size S · state TODO
- **Objective:** Prove stock purchase and asset consumption never recognize the same cost twice.
- **Rules and use cases:** EDR-013, EDR-029, BR-MONEY-008, VAL-32
- **Depends on:** EMP-STOCK-003
- **Read first:** [stock requirements](../17-traceability.md), [domain model](../06-domain-model.md), [shared rules](../04-business-rules.md) and [validation](../15-validation.md)
- **Scope:** in: single-cost guard across stock and expense origin · out: changing settled money records
- **Areas:** existing: fleet, fueling, expense and scoped job conventions · proposed: internal tank and movement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Cost arises at the approved consumption event and technical replay adds no second amount.
- **Acceptance:** Cost arises at the approved consumption event and technical replay adds no second amount.
- **Negative cases:** Purchase and use both counted, lost source link or concurrent duplicate expense.
- **Validation:** V-SQL, V-IDEM, V-ISO, V-MONEY · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [stock specification](../modules/fuel-stock.md) pins the method and boundaries. Real operations retain legal and activation gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-STOCK-006 — Panel: tanks, movements and reconciliation
- **Meta:** module STOCK · phase F6 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Expose scoped stock movements and reconciliation in the company panel.
- **Rules and use cases:** REQ-12.3, BR-AUTH-012, BR-TRUTH-004
- **Depends on:** EMP-STOCK-004, EMP-PLAT-008
- **Read first:** [stock requirements](../17-traceability.md), [domain model](../06-domain-model.md), [shared rules](../04-business-rules.md) and [validation](../15-validation.md)
- **Scope:** in: authorized tank, history and basis views · out: editable computed balance
- **Areas:** existing: fleet, fueling, expense and scoped job conventions · proposed: internal tank and movement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The balance and variance have source, revision and current scope.
- **Acceptance:** The balance and variance have source, revision and current scope.
- **Negative cases:** Foreign tank, stale balance labelled current or unauthorized adjustment.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-MONEY · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [stock specification](../modules/fuel-stock.md) pins the method and boundaries. Real operations retain legal and activation gates.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-STOCK-007 — App: dispense from an internal tank
- **Meta:** module STOCK · phase F6 · cut - · priority P1 · type APP · size S · state TODO
- **Objective:** Record an internal dispense in Android with durable scoped delivery.
- **Rules and use cases:** REQ-12.3, BR-OFFL-002, BR-OFFL-003, BR-OFFL-006
- **Depends on:** EMP-STOCK-003, EMP-FUEL-011
- **Read first:** [stock requirements](../17-traceability.md), [domain model](../06-domain-model.md), [shared rules](../04-business-rules.md) and [validation](../15-validation.md)
- **Scope:** in: approved cached tank and queued issue · out: claiming current stock authority offline
- **Areas:** existing: fleet, fueling, expense and scoped job conventions · proposed: internal tank and movement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The operation survives restart and conflicts remain for review rather than silent loss.
- **Acceptance:** The operation survives restart and conflicts remain for review rather than silent loss.
- **Negative cases:** Wrong context, replay doubles issue, absent acknowledgement shown accepted or negative-balance warning hidden.
- **Validation:** V-APP, V-SYNC, V-ISO, V-MONEY · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [stock specification](../modules/fuel-stock.md) pins the method and boundaries. Real operations retain legal and activation gates.
- **Done when:** The stated result and denials pass the listed validation profiles.
