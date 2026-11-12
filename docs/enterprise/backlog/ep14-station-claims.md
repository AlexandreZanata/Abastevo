# EP14 — Station claims and representation

Status: **PLANNED**, 2026-11-10.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from [station claims](../modules/station-claims.md), [decisions](../16-decisions.md) and [privacy](../11-privacy-security.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

The existing station claim specification and frozen representation contract remain the source for decisions.

### EMP-CLAIM-001 — Decide organization-held station authority, reconciled with ADR-017 and the contract freeze
- **Meta:** module CLAIM · phase F7 · cut - · priority P1 · type DEC · size S · state TODO
- **Objective:** Reconcile organization-held station authority with the frozen claim contract.
- **Rules and use cases:** BR-CLAIM-001, BR-CLAIM-002, BR-CLAIM-037, EDR-021, CX-11
- **Depends on:** EMP-GOV-015
- **Read first:** [station claims](../modules/station-claims.md), [decisions](../16-decisions.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: explicit holder and scoped transition decision · out: lifting Stage D by implication
- **Areas:** existing: stationprofile, organization scope and staff review · proposed: gated organization-held authority consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** An accepted amendment states exact holder, authority scope and inherited protections.
- **Acceptance:** An accepted amendment states exact holder, authority scope and inherited protections.
- **Negative cases:** Organization ownership inferred from account role, CNPJ or payment, or implementation before recorded override.
- **Validation:** V-DOC · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Stage D, verification staffing and current authority gates remain prerequisites. Recovering a task is not authorization to activate it.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-CLAIM-002 — Claim on behalf of an organization, bound to the requesting member, on the existing claim model
- **Meta:** module CLAIM · phase F7 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Create a claim through a currently authorized requesting member.
- **Rules and use cases:** BR-CLAIM-003, BR-CLAIM-005, BR-CLAIM-006, BR-CLAIM-007
- **Depends on:** EMP-CLAIM-001, EMP-ORG-011, EMP-AUTH-003
- **Read first:** [station claims](../modules/station-claims.md), [decisions](../16-decisions.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: verified entity and existing canonical station · out: creating public station or generic owner privilege
- **Areas:** existing: stationprofile, organization scope and staff review · proposed: gated organization-held authority consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** One request retains its human actor, organization and station under current proof.
- **Acceptance:** One request retains its human actor, organization and station under current proof.
- **Negative cases:** Foreign member, unverified entity, duplicate claim or arbitrary station creation.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Stage D, verification staffing and current authority gates remain prerequisites. Recovering a task is not authorization to activate it.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-CLAIM-003 — Station search and duplicate prevention at claim intake
- **Meta:** module CLAIM · phase F7 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Search the existing station and prevent duplicate open claim state.
- **Rules and use cases:** BR-CLAIM-003, BR-CLAIM-007, BR-CLAIM-008, BR-CLAIM-016
- **Depends on:** EMP-CLAIM-002
- **Read first:** [station claims](../modules/station-claims.md), [decisions](../16-decisions.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: canonical lookup and scoped claim limits · out: leaking rivals' private evidence
- **Areas:** existing: stationprofile, organization scope and staff review · proposed: gated organization-held authority consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Racing identical requests converge and the claimant sees only permitted state.
- **Acceptance:** Racing identical requests converge and the claimant sees only permitted state.
- **Negative cases:** Duplicate open claim, wrong station, cross-organization disclosure or limit bypass.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Stage D, verification staffing and current authority gates remain prerequisites. Recovering a task is not authorization to activate it.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-CLAIM-004 — Proportional evidence intake and the information-request loop
- **Meta:** module CLAIM · phase F7 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Collect proportional private evidence and preserve information requests.
- **Rules and use cases:** BR-CLAIM-010, BR-CLAIM-011, BR-CLAIM-012, BR-CLAIM-017, BR-CLAIM-036
- **Depends on:** EMP-CLAIM-002
- **Read first:** [station claims](../modules/station-claims.md), [decisions](../16-decisions.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: admitted evidence and bounded response loop · out: unreviewed document retention
- **Areas:** existing: stationprofile, organization scope and staff review · proposed: gated organization-held authority consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Evidence is private, scoped and handled under the existing claim retention contract.
- **Acceptance:** Evidence is private, scoped and handled under the existing claim retention contract.
- **Negative cases:** Unnecessary personal data, expired evidence served, external source treated as authority or foreign access.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Stage D, verification staffing and current authority gates remain prerequisites. Recovering a task is not authorization to activate it.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-CLAIM-005 — Review decisions, including concurrent claims
- **Meta:** module CLAIM · phase F7 · cut - · priority P1 · type INT · size M · state TODO
- **Objective:** Decide claims atomically with independent review and concurrent-holder safety.
- **Rules and use cases:** BR-CLAIM-009, BR-CLAIM-013, BR-CLAIM-014, BR-CLAIM-015, BR-CLAIM-016, VAL-17
- **Depends on:** EMP-CLAIM-004, EMP-BACK-002
- **Read first:** [station claims](../modules/station-claims.md), [decisions](../16-decisions.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: audited evidence decision and authority issuance · out: automatic approval or two live holders
- **Areas:** existing: stationprofile, organization scope and staff review · proposed: gated organization-held authority consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Each decision records basis and one current station authority survives concurrent approval.
- **Acceptance:** Each decision records basis and one current station authority survives concurrent approval.
- **Negative cases:** Self-review, stale decision, two live holders, unaudited result or private evidence leakage.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Stage D, verification staffing and current authority gates remain prerequisites. Recovering a task is not authorization to activate it.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-CLAIM-006 — Contest and appeal cases
- **Meta:** module CLAIM · phase F7 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Record contest and appeal without transferring authority automatically.
- **Rules and use cases:** BR-CLAIM-018, BR-CLAIM-019, BR-CLAIM-020
- **Depends on:** EMP-CLAIM-005
- **Read first:** [station claims](../modules/station-claims.md), [decisions](../16-decisions.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: scoped case, reason and reviewed outcome · out: authority inferred from filing a contest
- **Areas:** existing: stationprofile, organization scope and staff review · proposed: gated organization-held authority consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Case history and station authority remain separate and every allowed outcome is audited.
- **Acceptance:** Case history and station authority remain separate and every allowed outcome is audited.
- **Negative cases:** Foreign case, duplicate appeal where forbidden, automatic takeover or stale reversal.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Stage D, verification staffing and current authority gates remain prerequisites. Recovering a task is not authorization to activate it.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-CLAIM-007 — Authority lifecycle: suspend, expire, revoke, revalidate
- **Meta:** module CLAIM · phase F7 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Enforce live authority, expiry and reviewed revalidation at each use.
- **Rules and use cases:** BR-CLAIM-020, BR-CLAIM-021, BR-CLAIM-022, BR-CLAIM-023, BR-CLAIM-024
- **Depends on:** EMP-CLAIM-005
- **Read first:** [station claims](../modules/station-claims.md), [decisions](../16-decisions.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: current authority lifecycle · out: reopening a revoked or expired grant
- **Areas:** existing: stationprofile, organization scope and staff review · proposed: gated organization-held authority consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Capability decisions use current authority and retain prior decisions.
- **Acceptance:** Capability decisions use current authority and retain prior decisions.
- **Negative cases:** Cached expired authority, unreviewed revalidation, terminal grant revived by retry or foreign station scope.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Stage D, verification staffing and current authority gates remain prerequisites. Recovering a task is not authorization to activate it.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-CLAIM-008 — Station teams: representatives, managers and station scopes
- **Meta:** module CLAIM · phase F7 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Grant station roles only inside an authorized organization and station authority.
- **Rules and use cases:** BR-CLAIM-028, BR-CLAIM-029, BR-CLAIM-030, BR-AUTH-015
- **Depends on:** EMP-CLAIM-007, EMP-AUTH-006
- **Read first:** [station claims](../modules/station-claims.md), [decisions](../16-decisions.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: scoped teams and bounded delegation · out: organization-wide rights from one station
- **Areas:** existing: stationprofile, organization scope and staff review · proposed: gated organization-held authority consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Each action rechecks current station authority and role scope.
- **Acceptance:** Each action rechecks current station authority and role scope.
- **Negative cases:** Role applies to another station, lost authority ignored or delegation widens rights.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Stage D, verification staffing and current authority gates remain prerequisites. Recovering a task is not authorization to activate it.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-CLAIM-009 — Networks: several stations under one organization
- **Meta:** module CLAIM · phase F7 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Manage a network as separate proven station authorities.
- **Rules and use cases:** BR-CLAIM-003, BR-CLAIM-009, BR-CLAIM-033
- **Depends on:** EMP-CLAIM-007
- **Read first:** [station claims](../modules/station-claims.md), [decisions](../16-decisions.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: authorized per-station associations · out: authority from a common CNPJ root
- **Areas:** existing: stationprofile, organization scope and staff review · proposed: gated organization-held authority consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Network views include only stations with their own current authority.
- **Acceptance:** Network views include only stations with their own current authority.
- **Negative cases:** Another site included by identifier coincidence or private claim evidence shared across scopes.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Stage D, verification staffing and current authority gates remain prerequisites. Recovering a task is not authorization to activate it.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-CLAIM-010 — Operator succession, CNPJ change, closure and catalog merge effects
- **Meta:** module CLAIM · phase F7 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Respond to operator, legal-entity and catalog changes through reviewed authority rules.
- **Rules and use cases:** BR-CLAIM-025, BR-CLAIM-027, BR-CLAIM-035
- **Depends on:** EMP-CLAIM-007
- **Read first:** [station claims](../modules/station-claims.md), [decisions](../16-decisions.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: succession, closure and merge references · out: transferring authority by a click or rewriting the public catalog
- **Areas:** existing: stationprofile, organization scope and staff review · proposed: gated organization-held authority consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Changes produce reviewed bounded outcomes and retain history and canonical provenance.
- **Acceptance:** Changes produce reviewed bounded outcomes and retain history and canonical provenance.
- **Negative cases:** Automatic owner transfer, wrong CNPJ, merged station inherits unrelated authority or public data overwritten.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Stage D, verification staffing and current authority gates remain prerequisites. Recovering a task is not authorization to activate it.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-CLAIM-011 — Representation badge read model and wording
- **Meta:** module CLAIM · phase F7 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Publish a narrow representation badge with honest validity.
- **Rules and use cases:** BR-CLAIM-021, BR-CLAIM-022, BR-CLAIM-026, BR-CLAIM-034, BR-SRC-002
- **Depends on:** EMP-CLAIM-007
- **Read first:** [station claims](../modules/station-claims.md), [decisions](../16-decisions.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: approved public authority projection · out: quality, price or trust certification
- **Areas:** existing: stationprofile, organization scope and staff review · proposed: gated organization-held authority consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The badge identifies representation and current validity without paid influence.
- **Acceptance:** The badge identifies representation and current validity without paid influence.
- **Negative cases:** Expired badge, private evidence in projection, payment extends authority or misleading certification wording.
- **Validation:** V-API, V-SQL, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Stage D, verification staffing and current authority gates remain prerequisites. Recovering a task is not authorization to activate it.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-CLAIM-012 — Portal: claim wizard and status tracking
- **Meta:** module CLAIM · phase F7 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Present the gated station claim flow and progress in its portal.
- **Rules and use cases:** BR-CLAIM-003, BR-CLAIM-012, BR-CLAIM-017, BR-CLAIM-037
- **Depends on:** EMP-CLAIM-004, EMP-PLAT-008
- **Read first:** [station claims](../modules/station-claims.md), [decisions](../16-decisions.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: authorized claim UI and private status · out: opening Postos from company-panel approval
- **Areas:** existing: stationprofile, organization scope and staff review · proposed: gated organization-held authority consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Each step reflects current server state and the Stage D decision.
- **Acceptance:** Each step reflects current server state and the Stage D decision.
- **Negative cases:** Foreign claim, exposed proof, unapproved portal or optimistic approval.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Stage D, verification staffing and current authority gates remain prerequisites. Recovering a task is not authorization to activate it.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-CLAIM-013 — Representative exit and last-administrator recovery
- **Meta:** module CLAIM · phase F7 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** End representative access while preserving station authority and controlled recovery.
- **Rules and use cases:** BR-CLAIM-031, BR-CLAIM-032, BR-IDN-027
- **Depends on:** EMP-CLAIM-008
- **Read first:** [station claims](../modules/station-claims.md), [decisions](../16-decisions.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: scoped exit and last-administrator recovery · out: losing history or implicit replacement authority
- **Areas:** existing: stationprofile, organization scope and staff review · proposed: gated organization-held authority consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Departure ends the person's rights. Exceptional recovery uses reviewed evidence.
- **Acceptance:** Departure ends the person's rights. Exceptional recovery uses reviewed evidence.
- **Negative cases:** Account reset grants station authority, last-admin bypass, foreign membership or authority deleted on exit.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Stage D, verification staffing and current authority gates remain prerequisites. Recovering a task is not authorization to activate it.
- **Done when:** The stated result and denials pass the listed validation profiles.
