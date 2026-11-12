# EP00 — Governance, decisions and discovery

Status: **F0 IN PROGRESS**, 2026-11-10.

Task metadata was reconstructed from the existing [generated index](INDEX.md).
Task scope and acceptance follow the decision register, roadmap and F0 evidence.
Owner decisions are recorded in the linked evidence. The full documentation
gate and any real-data or implementation gate require separate verification.

### EMP-GOV-001 — Record the enterprise scope decision and propose the organization-tenancy ADR with the list of amended contracts
- **Meta:** module GOV · phase F0 · cut C1 · priority P0 · type DEC · size S · state LOCAL_DONE
- **Objective:** Establish the private Empresas organization boundary and its effect on existing contracts.
- **Rules and use cases:** EDR-001, EDR-005, CX-01, CX-02, BR-TEN-010
- **Depends on:** none
- **Read first:** [scope record](../evidence/f0-organization-scope.md), [Abastevo Empresas contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md)
- **Scope:** in: organization ownership, current membership and isolation; out: capabilities and activation
- **Areas:** existing: AGENTS.md, ADR-004 and individual and public product contracts
- **Steps:** Record the owner approval, the narrow ADR amendment and the separate product contract.
- **Deliverable:** [ADR-025](../../adr/025-private-abastevo-empresas-organization-boundary.md) and the linked contract amendments.
- **Acceptance:** One owning organization per private Empresas record. Anonymous public access and account ownership keep their existing scope.
- **Negative cases:** CNPJ root, staff status, or an account tier grants organization access.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** Approval is recorded. Runtime isolation requires implementation evidence.
- **Done when:** The accepted ADR and amended contracts pass the documentation gate.

### EMP-GOV-002 — Decide enterprise document authorization and retention, separate from the 24 h photo cap and from Stage D purchase evidence
- **Meta:** module GOV · phase F0 · cut C1 · priority P0 · type DEC · size S · state LOCAL_DONE
- **Objective:** Separate synthetic C1 receipt work from real-receipt authorization and retention.
- **Rules and use cases:** EDR-014, OQ-04, CX-05, CX-07, LV-11, BR-PRIV-004
- **Depends on:** EMP-GOV-001
- **Read first:** [M0 decision record](../evidence/f0-m0-decisions.md), [privacy and security](../11-privacy-security.md)
- **Scope:** in: private synthetic fueling receipts and the required real-data decision; out: Stage D purchase evidence and fiscal retrieval
- **Areas:** existing: privacy plan and Stage D plan; proposed: organization document scope
- **Steps:** Record the accepted synthetic path and the legal inputs required for real receipt categories.
- **Deliverable:** A conditional receipt decision and a retention review checklist.
- **Acceptance:** C1 uses synthetic receipts. Real intake requires an approved category, retention trigger and release gate.
- **Negative cases:** Real receipt accepted with no approved policy, a foreign capability, or a human review treated as fiscal authorization.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** The five-year hypothesis is a review input. Existing community photo and claim-scan rules still apply.
- **Done when:** Synthetic scope and the real-data hold are explicit and pass the documentation gate.

### EMP-GOV-003 — Decide the business web surface: stack, path or origin and session model, following the admin-web precedent
- **Meta:** module GOV · phase F0 · cut C1 · priority P0 · type DEC · size S · state LOCAL_DONE
- **Objective:** Define the company web path and isolate its session from privileged administration.
- **Rules and use cases:** EDR-006, EDR-019, OQ-05, BR-TEN-003
- **Depends on:** EMP-GOV-001
- **Read first:** [M0 decision record](../evidence/f0-m0-decisions.md), [architecture](../09-architecture.md)
- **Scope:** in: static company panel, exact origin, session and CSRF; out: station portal activation and deployment
- **Areas:** existing: admin-web/ and Caddy; proposed: business-web/
- **Steps:** Record the approved path and session boundary. Define the session and caching regression cases.
- **Deliverable:** Company web decision for the approved /empresas/ path.
- **Acceptance:** The build is separate and the session is accepted only by company middleware. Private responses use no-store.
- **Negative cases:** An admin cookie authorizes the company app, a business cookie authorizes admin, or a foreign origin can mutate.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** Host-only cookie Path=/ requires independent middleware and the existing admin fence.
- **Done when:** The owner decision and required regression cases pass the documentation gate.

### EMP-GOV-004 — Decide how internal administration extends the ADR-023 admin suite and staff roles
- **Meta:** module GOV · phase F0 · cut - · priority P1 · type DEC · size S · state TODO
- **Objective:** Define enterprise support responsibilities before granting staff access.
- **Rules and use cases:** EDR-020, CX-08, BR-TEN-008, BR-PRIV-021
- **Depends on:** EMP-GOV-001
- **Read first:** [decision register](../16-decisions.md), [privacy and security](../11-privacy-security.md)
- **Scope:** in: staff roles and support authorization; out: default private-content access
- **Areas:** existing: ADR-023 and admin/staff modules; proposed: bounded support sessions
- **Steps:** Inventory existing staff routes and document the roles, approval, scope and audit per action.
- **Deliverable:** A reviewed staff permissions decision.
- **Acceptance:** Each allowed action has a role, scope and audit obligation.
- **Negative cases:** An ordinary staff session or another support case can open private content.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** An operator role has no inherent organization membership.
- **Done when:** The staff permissions and support boundaries are recorded.

### EMP-GOV-005 — Legal validation package: privacy roles, location, documents and labour boundary
- **Meta:** module GOV · phase F0 · cut C1 · priority P0 · type DISC · size M · state LOCAL_DONE
- **Objective:** Provide the privacy, employment and fiscal questions needed for a real-data decision.
- **Rules and use cases:** LV-01, LV-02, LV-04, LV-05, LV-07, LV-08, LV-11
- **Depends on:** EMP-GOV-001
- **Read first:** [M0 decision record](../evidence/f0-m0-decisions.md), [privacy and security](../11-privacy-security.md)
- **Scope:** in: review package and required sign-off; out: a legal opinion or real-data activation
- **Areas:** existing: legal question inventory, retention schedule and decision record
- **Steps:** Send the package to owner-designated reviewers in a separately authorized step and record the signed findings by purpose.
- **Deliverable:** Legal review package with owners, expected outputs and activation gates.
- **Acceptance:** Each relevant category and processing purpose has a question, expected decision and blocking effect.
- **Negative cases:** A completed package, unchecked default or synthetic run is treated as legal clearance.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** Review findings remain pending. Receipt retention and real tracking remain gated.
- **Done when:** The legal question package passes the documentation gate and the review obligation remains visible.

### EMP-GOV-006 — Spike: organization trip tracking on top of the existing location services, with upload, on target devices
- **Meta:** module GOV · phase F0 · cut - · priority P1 · type DISC · size M · state TODO
- **Objective:** Determine whether the existing Android location services can support a trip-bound tracking flow.
- **Rules and use cases:** EDR-015, CX-06, BR-PRIV-006
- **Depends on:** EMP-GOV-001
- **Read first:** [privacy and security](../11-privacy-security.md), [architecture](../09-architecture.md)
- **Scope:** in: synthetic coordinates on authorized devices and controlled upload; out: real employee tracking
- **Areas:** existing: Android location services and mobile guard; proposed: synthetic spike report
- **Steps:** Prepare an authorized device matrix. Exercise permission, pause, disconnect and process restart with synthetic fixes.
- **Deliverable:** A reproducible synthetic tracking report with gaps and device coverage.
- **Acceptance:** Record measured outcomes by device and identify any unavailable evidence.
- **Negative cases:** Tracking outside a trip, hidden collection, silent gaps or real coordinates in evidence.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** A source review alone does not prove foreground-service behavior on a device.
- **Done when:** The authorized synthetic spike yields a reviewed report or explicit blocked evidence.

### EMP-GOV-007 — Decide the panel web map and whether server-side routing is needed, with a cost model
- **Meta:** module GOV · phase F0 · cut - · priority P1 · type DEC · size M · state TODO
- **Objective:** Select the company panel map and decide whether server routing is necessary.
- **Rules and use cases:** EDR-016, EDR-017, CX-10, OQ-10
- **Depends on:** EMP-GOV-006
- **Read first:** [decision register](../16-decisions.md), [architecture](../09-architecture.md)
- **Scope:** in: candidate map and routing costs, license and fallback; out: adding a provider or dependency
- **Areas:** existing: native map engine and map delivery; proposed: panel map decision
- **Steps:** Use the tracking findings to compare feasible designs, terms, recurring costs and the list fallback.
- **Deliverable:** A reviewed map and routing decision with cost assumptions.
- **Acceptance:** The selected candidate meets attribution, availability and privacy needs within the recorded assumptions.
- **Negative cases:** A map design depends on unrestricted upstream use or sends private tracks to an unreviewed provider.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** License and cost assumptions need current primary sources when this task runs.
- **Done when:** Map, fallback and server-routing decisions are recorded.

### EMP-GOV-008 — Decide the Android enterprise partition and local data protection per account, origin and organization
- **Meta:** module GOV · phase F0 · cut C1 · priority P0 · type DEC · size S · state LOCAL_DONE
- **Objective:** Protect Android data across account, server and organization changes.
- **Rules and use cases:** OQ-06, BR-PRIV-016, BR-PRIV-017, BR-PRIV-018, BR-TEN-004
- **Depends on:** EMP-GOV-001
- **Read first:** [M0 decision record](../evidence/f0-m0-decisions.md), [privacy and security](../11-privacy-security.md)
- **Scope:** in: partition identity, keys, restore and queued work; out: choosing an unreviewed encryption dependency
- **Areas:** existing: Keystore adapters, backup and extraction rules; proposed: protected enterprise data store
- **Steps:** Record the approved partition and required isolation tests. Review storage coverage before implementation.
- **Deliverable:** The Android partition and protection decision.
- **Acceptance:** Records, journals, files and queue stay within the creation scope and are excluded from backups.
- **Negative cases:** A switch or restore opens another partition, or offline revocation is portrayed as immediate.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** Key expiry and process-death behavior require implementation evidence.
- **Done when:** The approved partition and negative cases pass the documentation gate.

### EMP-GOV-009 — Decide organization billing: extend the Stripe/Asaas offer policy and confirm the store-policy position
- **Meta:** module GOV · phase F0 · cut - · priority P2 · type DEC · size S · state TODO
- **Objective:** Decide a separate organization billing offer without changing individual tiers.
- **Rules and use cases:** EDR-024, CX-03, CX-12, OQ-11
- **Depends on:** EMP-GOV-001
- **Read first:** [decision register](../16-decisions.md), [discovery protocol](../evidence/f0-discovery-protocol.md)
- **Scope:** in: offer model, store position and evidence needs; out: charging or provider activation
- **Areas:** existing: Stripe/Asaas offer policy and account entitlements; proposed: organization offer decision
- **Steps:** Reconcile offer scope with current store policy, fiscal review and demand evidence.
- **Deliverable:** An offer, billing channel and entitlement decision.
- **Acceptance:** Organization rights come from membership and the approved organization plan policy.
- **Negative cases:** An existing account tier grants organization access or a payment changes public trust.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** Catalog, price and fiscal treatment require separate approval.
- **Done when:** The maintainer records a reviewed billing scope and prerequisites.

### EMP-GOV-010 — Discovery protocol with pilot companies and stations, including pricing hypotheses
- **Meta:** module GOV · phase F0 · cut - · priority P1 · type DISC · size S · state LOCAL_DONE
- **Objective:** Prepare discovery about real fleet and station workflows and charging units.
- **Rules and use cases:** OQ-15, REQ-16.5
- **Depends on:** none
- **Read first:** [discovery protocol](../evidence/f0-discovery-protocol.md)
- **Scope:** in: interview guide, synthetic walkthrough and synthesis method; out: contacting people or starting a pilot
- **Areas:** existing: product vision and F0 discovery protocol
- **Steps:** Define participants, operational questions, price prompts and the evidence returned to product review.
- **Deliverable:** A discovery protocol suitable for later authorized interviews.
- **Acceptance:** The protocol separates fleet and station hypotheses and uses synthetic examples.
- **Negative cases:** Names or operational evidence enter Git, or stated willingness is counted as a purchase.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** Participant outreach and pricing decisions are separate.
- **Done when:** The protocol passes the documentation gate.

### EMP-GOV-011 — Decide the viability of authorised fiscal document consultation
- **Meta:** module GOV · phase F0 · cut - · priority P2 · type DEC · size S · state TODO
- **Objective:** Assess whether authorized fiscal document consultation is viable.
- **Rules and use cases:** BR-TRUTH-002, EDR-014, OQ-04
- **Depends on:** EMP-GOV-005
- **Read first:** [privacy and security](../11-privacy-security.md), [decision register](../16-decisions.md)
- **Scope:** in: authorization, certificate custody, permitted routes and evidence; out: connecting to fiscal providers
- **Areas:** existing: legal package and document design; proposed: fiscal consultation decision
- **Steps:** Review current primary sources and legal findings. Compare manual review with authorized consultation.
- **Deliverable:** A scoped viability decision and deferred prerequisites.
- **Acceptance:** The decision distinguishes access, structure checks and fiscal authorization.
- **Negative cases:** An extracted key is treated as authority for an unrestricted query.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** Certificate and provider choices require separate review.
- **Done when:** The permitted workflow and prerequisites are recorded or consultation remains deferred.

### EMP-GOV-012 — Register the enterprise program in ROADMAP and PROGRESS sequencing
- **Meta:** module GOV · phase F0 · cut C1 · priority P0 · type DEC · size XS · state LOCAL_DONE
- **Objective:** Register F0 and establish the authorized place for future F1 source work.
- **Rules and use cases:** EDR-004, EDR-027, CX-09, OQ-03
- **Depends on:** EMP-GOV-001
- **Read first:** [enterprise roadmap](../14-roadmap.md), [repository roadmap](../../../ROADMAP.md#phase-order)
- **Scope:** in: F0 registration and sequencing decision; out: changing G09, Stage D or other acceptance gates
- **Areas:** existing: ROADMAP.md, PROGRESS.md, ADR-019 and current acceptance evidence
- **Steps:** Record F0 locally. Reconcile current acceptance and document the authorized F1 position before starting source work.
- **Deliverable:** An indexed F0 and a recorded F1 sequencing decision.
- **Acceptance:** The owner decision requires M0 closure and protected integration of the current ADR-019 construction batch before local F1 work.
- **Negative cases:** A planning decision is treated as a completed Android, release or real-data gate.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** The owner confirmed the sequence. M0 closure and integration require separate evidence.
- **Done when:** F1 sequencing is recorded and the roadmap and progress entries agree.

### EMP-GOV-013 — Security review of managed credentials: activation code, device binding, recovery
- **Meta:** module GOV · phase F0 · cut - · priority P1 · type DEC · size S · state TODO
- **Objective:** Review organization-managed credentials before making them available.
- **Rules and use cases:** EDR-007, EDR-031, OQ-13, BR-TEN-002
- **Depends on:** EMP-GOV-001
- **Read first:** [decision register](../16-decisions.md), [privacy and security](../11-privacy-security.md)
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
- **Read first:** [decision register](../16-decisions.md)
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
- **Read first:** [decision register](../16-decisions.md), [Stage D](../../planning/FUTURE_EVIDENCE_BUSINESS_PLAN.md)
- **Scope:** in: named station claim, management or portal scope if authorized; out: lifting containment by implication
- **Areas:** existing: OPT-D00, Stage D and station profile rules
- **Steps:** Collect the exact owner authorization and readiness evidence before recording any override.
- **Deliverable:** A scoped OPT-D00 decision or an explicit continuing hold.
- **Acceptance:** The decision names capabilities, environment, staffing and evidence obligations.
- **Negative cases:** Company-panel or synthetic receipt approval is read as authority for a station portal.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** Stage D remains gated while the required decision and evidence are absent.
- **Done when:** The exact override and readiness are recorded, or F7 remains held.

### EMP-GOV-016 — Establish redistribution rights for public source data in commercial Data products
- **Meta:** module GOV · phase F0 · cut - · priority P2 · type DISC · size M · state TODO
- **Objective:** Establish the provenance and redistribution rights needed for Data products.
- **Rules and use cases:** EDR-025, CX-04, OQ-08, LV-13, BR-SRC-005
- **Depends on:** EMP-GOV-001
- **Read first:** [decision register](../16-decisions.md), [privacy and security](../11-privacy-security.md)
- **Scope:** in: per-source license and redistribution review; out: enabling sales or exporting private organization data
- **Areas:** existing: public source admission and Enterprise Data Offering; proposed: rights evidence
- **Steps:** Inventory the source and license for each dataset. Record permitted transformations and redistribution or the unresolved rights.
- **Deliverable:** A source rights register with citations and blockers.
- **Acceptance:** Every sale-eligible dataset has documented redistribution rights and provenance.
- **Negative cases:** Public availability is treated as resale permission or a private organization record enters a market dataset.
- **Validation:** V-DOC · migration: none · rollback: revert the planning record and preserve the decision history.
- **Risks and assumptions:** Unclear rights keep the affected datasets in the rights-pending state.
- **Done when:** Rights are recorded per source and unapproved sale paths remain gated.
