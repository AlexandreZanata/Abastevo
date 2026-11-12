# EP20 — Release gates and validation

Status: **PLANNED**, 2026-11-10.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from [milestone validation](../15-validation.md), [enterprise roadmap](../14-roadmap.md) and [F0 decisions](../evidence/f0-m0-decisions.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

Gate tasks require actual executed evidence. Recovery of this backlog accepts no runtime milestone.

### EMP-REL-001 — Cut C1 end-to-end scenario and isolation gate
- **Meta:** module REL · phase F2 · cut C1 · priority P0 · type TEST · size M · state TODO
- **Objective:** Prove C1 from company setup to reviewed fueling with a synthetic receipt.
- **Rules and use cases:** BR-TEN-007, BR-OFFL-004, VAL-01, VAL-02, VAL-05, VAL-06, VAL-07, VAL-08, VAL-21
- **Depends on:** EMP-TRIP-006, EMP-TRIP-005, EMP-FUEL-012, EMP-FUEL-011, EMP-DOC-015, EMP-DOC-014, EMP-APPR-005, EMP-OFFL-014, EMP-OFFL-010, EMP-OFFL-007, EMP-OFFL-004, EMP-ORG-014, EMP-IDN-016, EMP-FLEET-012, EMP-METER-003, EMP-AUTH-009, EMP-AUTH-004, EMP-PUB-001, EMP-PRIV-003, EMP-PLAT-013
- **Read first:** [milestone validation](../15-validation.md), [enterprise roadmap](../14-roadmap.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: end-to-end scenario, PostGIS, Android and two-organization fixture · out: real-data pilot acceptance
- **Areas:** existing: scenario fixtures and affected normal entry points · proposed: milestone evidence and acceptance commands
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Every declared C1 scenario passes on one revision and each unavailable manual item remains explicitly owed.
- **Acceptance:** Every declared C1 scenario passes on one revision and each unavailable manual item remains explicitly owed.
- **Negative cases:** Missing required scenario, mock-only proof, cross-organization exposure or synthetic success called production acceptance.
- **Validation:** V-E2E, V-SQL, V-APP, V-AUTHZ, V-ISO, V-SYNC, V-MONEY, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Each gate retains immediate critical checks, current-head evidence and distinct local, integration and production states.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-REL-002 — Cut C1 pilot activation, runbook and rollback
- **Meta:** module REL · phase F2 · cut C1 · priority P0 · type REL · size S · state TODO
- **Objective:** Rehearse scoped activation, rollback and recovery after C1 source acceptance.
- **Rules and use cases:** VAL-24, BR-AUTH-009, BR-PRIV-011, OQ-02
- **Depends on:** EMP-REL-001, EMP-PLAT-005, EMP-GOV-005, EMP-GOV-012
- **Read first:** [milestone validation](../15-validation.md), [enterprise roadmap](../14-roadmap.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: approved environment and named synthetic organizations · out: real pilot before legal and release gates
- **Areas:** existing: scenario fixtures and affected normal entry points · proposed: milestone evidence and acceptance commands
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Flag changes preserve existing records and rollback and restore follow tested commands.
- **Acceptance:** Flag changes preserve existing records and rollback and restore follow tested commands.
- **Negative cases:** Unreviewed environment, erased data, missing read-time privacy gate or real activation from a local checkpoint.
- **Validation:** V-E2E, V-SQL, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Each gate retains immediate critical checks, current-head evidence and distinct local, integration and production states.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-REL-003 — M1 gate: access and isolation acceptance
- **Meta:** module REL · phase F1 · cut - · priority P1 · type TEST · size S · state TODO
- **Objective:** Accept M1 only after current-scope access and isolation evidence.
- **Rules and use cases:** VAL-02, VAL-03, VAL-21, VAL-22, BR-AUTH-006
- **Depends on:** EMP-AUTH-009, EMP-IDN-008, EMP-IDN-010, EMP-ORG-009
- **Read first:** [milestone validation](../15-validation.md), [enterprise roadmap](../14-roadmap.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: routes added in F1 and revocation/last-owner cases · out: capability claim from compile-only evidence
- **Areas:** existing: scenario fixtures and affected normal entry points · proposed: milestone evidence and acceptance commands
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Current role and scope controls pass at normal entry points with the global flag off by default.
- **Acceptance:** Current role and scope controls pass at normal entry points with the global flag off by default.
- **Negative cases:** Missing denial case, overprivileged test role, ownerless organization or stale session access.
- **Validation:** V-E2E, V-SQL, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Each gate retains immediate critical checks, current-head evidence and distinct local, integration and production states.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-REL-004 — M3 gate: fuelings, documents, expenses and approvals acceptance
- **Meta:** module REL · phase F3 · cut - · priority P1 · type TEST · size M · state TODO
- **Objective:** Accept M3 after exact cost, review, allocation and consumption evidence.
- **Rules and use cases:** EDR-013, BR-MONEY-007, VAL-08, VAL-09, VAL-10, VAL-11, VAL-12, VAL-13, VAL-14, VAL-19, VAL-20, VAL-25
- **Depends on:** EMP-DOC-010, EMP-FIN-004, EMP-BUDG-004, EMP-APPR-004, EMP-FUEL-008
- **Read first:** [milestone validation](../15-validation.md), [enterprise roadmap](../14-roadmap.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: full milestone scenarios on one revision · out: narrowing known failure coverage
- **Areas:** existing: scenario fixtures and affected normal entry points · proposed: milestone evidence and acceptance commands
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** No duplicate cost or budget race remains and corrected inputs update versioned results.
- **Acceptance:** No duplicate cost or budget race remains and corrected inputs update versioned results.
- **Negative cases:** Double counting, missing money vectors, unsupported ratio or a failed scenario waived.
- **Validation:** V-E2E, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY, V-SYNC · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Each gate retains immediate critical checks, current-head evidence and distinct local, integration and production states.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-REL-005 — M4 gate: trips, logbook, sync and tracking fundamentals acceptance
- **Meta:** module REL · phase F4 · cut - · priority P1 · type TEST · size M · state TODO
- **Objective:** Accept M4 trips, sync resilience and authorized tracking fundamentals.
- **Rules and use cases:** BR-OFFL-011, BR-AMD-001, VAL-04, VAL-22, VAL-23, VAL-26, VAL-27, VAL-28
- **Depends on:** EMP-OFFL-013, EMP-TRIP-015, EMP-GPS-006, EMP-GPS-010
- **Read first:** [milestone validation](../15-validation.md), [enterprise roadmap](../14-roadmap.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: scenario union, source and device evidence · out: real tracking before its legal and device gates
- **Areas:** existing: scenario fixtures and affected normal entry points · proposed: milestone evidence and acceptance commands
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Required interruption and late-intake cases pass or tracking remains held with owed evidence named.
- **Acceptance:** Required interruption and late-intake cases pass or tracking remains held with owed evidence named.
- **Negative cases:** Missing device coverage called pass, private fixtures committed, silent queue loss or location resumed without authority.
- **Validation:** V-E2E, V-SQL, V-APP, V-AUTHZ, V-ISO, V-SYNC, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Each gate retains immediate critical checks, current-head evidence and distinct local, integration and production states.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-REL-006 — M5 gate: navigation and live operations acceptance
- **Meta:** module REL · phase F5 · cut - · priority P1 · type TEST · size M · state TODO
- **Objective:** Accept M5 navigation and live operations against the approved map decision.
- **Rules and use cases:** VAL-16, VAL-29, VAL-30, BR-GPS-032
- **Depends on:** EMP-ROUTE-006, EMP-GPS-008, EMP-GPS-013
- **Read first:** [milestone validation](../15-validation.md), [enterprise roadmap](../14-roadmap.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: integrated guidance, source/quality and freshness evidence · out: stale position shown current
- **Areas:** existing: scenario fixtures and affected normal entry points · proposed: milestone evidence and acceptance commands
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The tested revision passes trip-aware navigation and live-map privacy and freshness cases.
- **Acceptance:** The tested revision passes trip-aware navigation and live-map privacy and freshness cases.
- **Negative cases:** Unapproved adapter, missing interruption test, unsupported profile claim or unaudited location.
- **Validation:** V-E2E, V-APP, V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Each gate retains immediate critical checks, current-head evidence and distinct local, integration and production states.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-REL-007 — M6 gate: intelligence, maintenance and stock acceptance
- **Meta:** module REL · phase F6 · cut - · priority P1 · type TEST · size M · state TODO
- **Objective:** Accept M6 indicator, maintenance, stock and retention behavior.
- **Rules and use cases:** EDR-029, BR-PRIV-011, VAL-10, VAL-11, VAL-15, VAL-20, VAL-31, VAL-32, VAL-38
- **Depends on:** EMP-KPI-006, EMP-FIND-004, EMP-MAINT-008, EMP-STOCK-005
- **Read first:** [milestone validation](../15-validation.md), [enterprise roadmap](../14-roadmap.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: exact shared vectors and real failure/restore evidence · out: inferring capacity from a plan
- **Areas:** existing: scenario fixtures and affected normal entry points · proposed: milestone evidence and acceptance commands
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** All milestone scenarios agree on amounts, source revisions and privacy outcomes.
- **Acceptance:** All milestone scenarios agree on amounts, source revisions and privacy outcomes.
- **Negative cases:** Wrong ratio, duplicate stock cost, expired data after restore or known failure deferred.
- **Validation:** V-E2E, V-SQL, V-AUTHZ, V-ISO, V-MONEY, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Each gate retains immediate critical checks, current-head evidence and distinct local, integration and production states.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-REL-008 — M7 gate: station representation and commerce acceptance
- **Meta:** module REL · phase F7 · cut - · priority P1 · type TEST · size M · state TODO
- **Objective:** Accept M7 only under recorded Stage D scope and review staffing.
- **Rules and use cases:** VAL-17, VAL-33, VAL-34, CX-05, BR-SRC-002
- **Depends on:** EMP-CLAIM-006, EMP-STN-010, EMP-AGREE-009
- **Read first:** [milestone validation](../15-validation.md), [enterprise roadmap](../14-roadmap.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: claim concurrency, neutrality and transactional sharing · out: public authority from payment
- **Areas:** existing: scenario fixtures and affected normal entry points · proposed: milestone evidence and acceptance commands
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Approved scope and each scenario are evidenced on the named revision with staff readiness.
- **Acceptance:** Approved scope and each scenario are evidenced on the named revision with staff readiness.
- **Negative cases:** Missing override, competing live holders, private transaction leak or paid trust.
- **Validation:** V-E2E, V-SQL, V-WEB, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Each gate retains immediate critical checks, current-head evidence and distinct local, integration and production states.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-REL-009 — M8 gate: subscriptions, Data and integrations acceptance
- **Meta:** module REL · phase F8 · cut - · priority P1 · type TEST · size M · state TODO
- **Objective:** Accept M8 only with approved billing and redistribution rights.
- **Rules and use cases:** VAL-18, VAL-35, VAL-36, EDR-024, EDR-025
- **Depends on:** EMP-SUBS-005, EMP-DATA-007, EMP-SUBS-004
- **Read first:** [milestone validation](../15-validation.md), [enterprise roadmap](../14-roadmap.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: event replay, capability state and Data privacy scenarios · out: enabling sale from a test result alone
- **Areas:** existing: scenario fixtures and affected normal entry points · proposed: milestone evidence and acceptance commands
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Subscription effects and keys follow current authority and market datasets exclude private data.
- **Acceptance:** Subscription effects and keys follow current authority and market datasets exclude private data.
- **Negative cases:** Unverified provider event, role granted by payment, unlicensed source or hidden private row.
- **Validation:** V-E2E, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Each gate retains immediate critical checks, current-head evidence and distinct local, integration and production states.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-REL-010 — Capacity benchmark with a synthetic fleet fixture
- **Meta:** module REL · phase F6 · cut - · priority P1 · type TEST · size M · state TODO
- **Objective:** Measure declared synthetic capacity without claiming production readiness.
- **Rules and use cases:** VAL-37, BR-PRIV-008, BR-TRUTH-004
- **Depends on:** EMP-GPS-004, EMP-KPI-002
- **Read first:** [milestone validation](../15-validation.md), [enterprise roadmap](../14-roadmap.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: named workload, environment, budgets and raw aggregate results · out: extrapolating unmeasured VPS capacity
- **Areas:** existing: scenario fixtures and affected normal entry points · proposed: milestone evidence and acceptance commands
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The report records revision, repeatable command and observed throughput/latency under the accepted budget.
- **Acceptance:** The report records revision, repeatable command and observed throughput/latency under the accepted budget.
- **Negative cases:** Private fixture, unexplained failure, omitted workload or measured result presented as production certification.
- **Validation:** V-PERF, V-E2E, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Each gate retains immediate critical checks, current-head evidence and distinct local, integration and production states.
- **Done when:** The stated result and denials pass the listed validation profiles.
