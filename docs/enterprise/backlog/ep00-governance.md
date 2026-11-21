# EP00 — Governance, decisions and discovery

Task metadata was reconstructed from the existing [generated index](INDEX.md).
Task scope and acceptance follow the decision register, roadmap and F0 evidence.
Owner decisions are recorded in the linked evidence. The full documentation
gate and any real-data or implementation gate require separate verification.

### EMP-GOV-004 — Decide how internal administration extends the ADR-023 admin suite and staff roles
- **Meta:** module GOV · phase F0 · cut - · priority P1 · type DEC · size S · state TODO
- **Objective:** Define enterprise support responsibilities before granting staff access.
- **Rules and use cases:** EDR-020, CX-08, BR-TEN-008, BR-PRIV-021
- **Depends on:** EMP-GOV-001
- **Read first:** [decision register](../decisions.md), [privacy and security](../privacy-security.md)
- **Scope:** in: staff roles and support authorization; out: default private-content access
- **Areas:** existing: ADR-023 and admin/staff modules; proposed: bounded support sessions
- **Steps:** Inventory existing staff routes and document the roles, approval, scope and audit per action.
- **Deliverable:** A reviewed staff permissions decision.
- **Acceptance:** Each allowed action has a role, scope and audit obligation.
- **Negative cases:** An ordinary staff session or another support case can open private content.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** An operator role has no inherent organization membership.
- **Done when:** The staff permissions and support boundaries are recorded.

### EMP-GOV-006 — Spike: organization trip tracking on top of the existing location services, with upload, on target devices
- **Meta:** module GOV · phase F0 · cut - · priority P1 · type DISC · size M · state TODO
- **Objective:** Determine whether the existing Android location services can support a trip-bound tracking flow.
- **Rules and use cases:** EDR-015, CX-06, BR-PRIV-006
- **Depends on:** EMP-GOV-001
- **Read first:** [privacy and security](../privacy-security.md), [architecture](../architecture.md)
- **Scope:** in: synthetic coordinates on authorized devices and controlled upload; out: real employee tracking
- **Areas:** existing: Android location services and mobile guard; proposed: synthetic spike report
- **Steps:** Prepare an authorized device matrix. Exercise permission, pause, disconnect and process restart with synthetic fixes.
- **Deliverable:** A reproducible synthetic tracking report with gaps and device coverage.
- **Acceptance:** Record measured outcomes by device and identify any unavailable evidence.
- **Negative cases:** Tracking outside a trip, hidden collection, silent gaps or real coordinates in evidence.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** A source review alone does not prove foreground-service behavior on a device.
- **Done when:** The authorized synthetic spike yields a reviewed report or explicit blocked evidence.

### EMP-GOV-009 — Decide organization billing: extend the Stripe/Asaas offer policy and confirm the store-policy position
- **Meta:** module GOV · phase F0 · cut - · priority P2 · type DEC · size S · state TODO
- **Objective:** Decide a separate organization billing offer without changing individual tiers.
- **Rules and use cases:** EDR-024, CX-03, CX-12, OQ-11
- **Depends on:** EMP-GOV-001
- **Read first:** [decision register](../decisions.md), discovery protocol
- **Scope:** in: offer model, store position and evidence needs; out: charging or provider activation
- **Areas:** existing: Stripe/Asaas offer policy and account entitlements; proposed: organization offer decision
- **Steps:** Reconcile offer scope with current store policy, fiscal review and demand evidence.
- **Deliverable:** An offer, billing channel and entitlement decision.
- **Acceptance:** Organization rights come from membership and the approved organization plan policy.
- **Negative cases:** An existing account tier grants organization access or a payment changes public trust.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** Catalog, price and fiscal treatment require separate approval.
- **Done when:** The maintainer records a reviewed billing scope and prerequisites.

### EMP-GOV-011 — Decide the viability of authorised fiscal document consultation
- **Meta:** module GOV · phase F0 · cut - · priority P2 · type DEC · size S · state TODO
- **Objective:** Assess whether authorized fiscal document consultation is viable.
- **Rules and use cases:** BR-TRUTH-002, EDR-014, OQ-04
- **Depends on:** EMP-GOV-005
- **Read first:** [privacy and security](../privacy-security.md), [decision register](../decisions.md)
- **Scope:** in: authorization, certificate custody, permitted routes and evidence; out: connecting to fiscal providers
- **Areas:** existing: legal package and document design; proposed: fiscal consultation decision
- **Steps:** Review current primary sources and legal findings. Compare manual review with authorized consultation.
- **Deliverable:** A scoped viability decision and deferred prerequisites.
- **Acceptance:** The decision distinguishes access, structure checks and fiscal authorization.
- **Negative cases:** An extracted key is treated as authority for an unrestricted query.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** Certificate and provider choices require separate review.
- **Done when:** The permitted workflow and prerequisites are recorded or consultation remains deferred.

### EMP-GOV-013 — Security review of managed credentials: activation code, device binding, recovery
- **Meta:** module GOV · phase F0 · cut - · priority P1 · type DEC · size S · state TODO
- **Objective:** Review organization-managed credentials before making them available.
- **Rules and use cases:** EDR-007, EDR-031, OQ-13, BR-TEN-002
- **Depends on:** EMP-GOV-001
- **Read first:** [decision register](../decisions.md), [privacy and security](../privacy-security.md)
- **Scope:** in: activation, device binding, recovery and revocation; out: passwords known to managers and real issuance
- **Areas:** existing: account session and device binding; proposed: managed credential decision
- **Steps:** Define enrollment authority, single-use recovery and the response to loss and offboarding.
- **Deliverable:** A security review with abuse and failure cases.
- **Acceptance:** Recovery preserves organization boundaries and exposes no personal account credentials.
- **Negative cases:** Replay, a guessed code, a revoked device or shared registration grants access.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** The C1 invitation path can proceed independently after its other gates.
- **Done when:** Managed access is approved with explicit checks or stays deferred.

### EMP-GOV-014 — Decide external delivery channels: transactional e-mail and push for enterprise notices
- **Meta:** module GOV · phase F0 · cut - · priority P1 · type DEC · size XS · state TODO
- **Objective:** Decide if external delivery is needed beyond the in-app path.
- **Rules and use cases:** EDR-032, CX-12, OQ-09
- **Depends on:** EMP-GOV-001
- **Read first:** [decision register](../decisions.md)
- **Scope:** in: transactional email and push decision; out: provider contact or enablement
- **Areas:** existing: in-app notification proposal; proposed: delivery channel decision
- **Steps:** Record the use case, opt-out, data sent, retry limits, costs and provider prerequisites.
- **Deliverable:** An approved or deferred channel decision.
- **Acceptance:** In-app notices remain available. Any external channel has a bounded purpose.
- **Negative cases:** Invitation credentials or private receipt content leak to an unreviewed channel.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** A provider configuration is a separate activation step.
- **Done when:** The maintainer records the selected channel scope or deferral.

### EMP-GOV-015 — Record the scoped Stage D override for station claims, management and portal (OPT-D00)
- **Meta:** module GOV · phase F0 · cut - · priority P1 · type DEC · size XS · state BLOCKED
- **Objective:** Record any explicitly authorized Stage D reopening for the station front.
- **Rules and use cases:** CX-05, OQ-07, EDR-021
- **Depends on:** EMP-GOV-001
- **Read first:** [decision register](../decisions.md), Stage D
- **Scope:** in: named station claim, management or portal scope if authorized; out: lifting containment by implication
- **Areas:** existing: OPT-D00, Stage D and station profile rules
- **Steps:** Collect the exact owner authorization and readiness evidence before recording any override.
- **Deliverable:** A scoped OPT-D00 decision or an explicit continuing hold.
- **Acceptance:** The decision names capabilities, environment, staffing and evidence obligations.
- **Negative cases:** Company-panel or synthetic receipt approval is read as authority for a station portal.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** Stage D remains gated while the required decision and evidence are absent.
- **Done when:** The exact override and readiness are recorded, or F7 remains held.

EMP-GOV-016 closed by the owner decision on 2026-11-21 (EDR-394).
Source guardrails and exact runtime admission remain requirements of EMP-DATA-001 and M8.
