# EP15 — Station commerce and agreements

Status: **PLANNED**, 2026-11-10.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

The [commerce design](../modules/station-commerce.md) records the recovered scope. Explicit source separation, neutrality and narrow contractual sharing remain binding.

### EMP-STN-001 — Commercial profile fields and publication through profile revisions
- **Meta:** module STN · phase F7 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Publish commercial profile fields through reviewed station revisions.
- **Rules and use cases:** REQ-15.1, BR-CLAIM-021, BR-CLAIM-035, BR-AUD-001
- **Depends on:** EMP-CLAIM-007
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: authorized business fields · out: regulatory record overwrite
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Current station authority gates each revision and source boundaries remain visible.
- **Acceptance:** Current station authority gates each revision and source boundaries remain visible.
- **Negative cases:** Foreign station, stale authority, unreviewed publication or official data replaced.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-STN-002 — Station-announced prices: own source, validity, conditions and history
- **Meta:** module STN · phase F7 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Record announced prices as a separate attributed source.
- **Rules and use cases:** REQ-15.2, EDR-022, BR-SRC-001, BR-SRC-002
- **Depends on:** EMP-CLAIM-007, EMP-PLAT-009
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: product, unit, condition, author, validity and history · out: community consensus writes
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Announcements retain their basis and expiry and never alter organic trust.
- **Acceptance:** Announcements retain their basis and expiry and never alter organic trust.
- **Negative cases:** Missing conditions, expired price shown current, wrong station or community source overwritten.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-STN-003 — Price display rules across announced, community, official and private prices
- **Meta:** module STN · phase F7 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Keep announced, official, community and negotiated prices distinguishable.
- **Rules and use cases:** REQ-15.2, BR-SRC-001, BR-TRUTH-006, BR-KPI-012
- **Depends on:** EMP-STN-002
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: source-specific display and comparability · out: averaging sources
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Every displayed price has source, condition and age and private negotiated terms stay scoped.
- **Acceptance:** Every displayed price has source, condition and age and private negotiated terms stay scoped.
- **Negative cases:** Private term exposed, mixed source labelled cheapest or expired data shown current.
- **Validation:** V-DOM, V-APP, V-WEB, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-STN-004 — Promotions: labelled and time-bound
- **Meta:** module STN · phase F7 · cut - · priority P2 · type API · size S · state TODO
- **Objective:** Publish only authorized labelled promotions with explicit validity.
- **Rules and use cases:** REQ-15.3, BR-SRC-002, BR-TRUTH-006
- **Depends on:** EMP-STN-002
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: bounded promotion lifecycle · out: purchased trust or concealed offer conditions
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The promotion's owner and conditions are visible and expiry applies at read time.
- **Acceptance:** The promotion's owner and conditions are visible and expiry applies at read time.
- **Negative cases:** Unlabelled advertising, inactive authority or expired promotion served.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-STN-005 — Official replies and moderation requests
- **Meta:** module STN · phase F7 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Route official replies and moderation requests through existing feedback rules.
- **Rules and use cases:** REQ-15.3, BR-SRC-002, BR-CLAIM-021
- **Depends on:** EMP-CLAIM-008
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: attributed representative response and reviewed request · out: deleting legitimate criticism
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Replies identify the permitted representative and moderation stays independent.
- **Acceptance:** Replies identify the permitted representative and moderation stays independent.
- **Negative cases:** Paid exemption, foreign reply, direct criticism deletion or ordinary employee acting as representative.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-STN-006 — Catalog correction requests through directory governance
- **Meta:** module STN · phase F7 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Submit catalog corrections through Directory governance.
- **Rules and use cases:** REQ-14.7, BR-CLAIM-035, BR-SRC-004
- **Depends on:** EMP-CLAIM-007
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: attributed suggestion · out: writing official catalog directly
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The suggestion preserves canonical ownership and follows the existing review path.
- **Acceptance:** The suggestion preserves canonical ownership and follows the existing review path.
- **Negative cases:** Commercial authority overwrites regulatory data, duplicate record or scope leak.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-STN-007 — Interaction metrics with aggregation thresholds
- **Meta:** module STN · phase F7 · cut - · priority P2 · type JOB · size M · state TODO
- **Objective:** Count only admitted interactions with privacy-preserving aggregation.
- **Rules and use cases:** REQ-15.4, BR-PRIV-005, BR-PRIV-008, BR-SRC-005
- **Depends on:** EMP-STN-001
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: documented purpose, thresholds and real observed counts · out: invented conversion or customer location marketing
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Metrics expose their coverage and insufficient groups remain withheld.
- **Acceptance:** Metrics expose their coverage and insufficient groups remain withheld.
- **Negative cases:** Private visitor data, tiny cohort, absent collection reported as zero or inflated retry count.
- **Validation:** V-SQL, V-JOB, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-STN-008 — Portal: station dashboard, profile, prices, promotions and reviews
- **Meta:** module STN · phase F7 · cut - · priority P1 · type WEB · size L · state TODO
- **Objective:** Present authorized station commercial actions in the gated portal.
- **Rules and use cases:** REQ-15.1, REQ-15.3, BR-CLAIM-021, BR-AUTH-015
- **Depends on:** EMP-STN-001, EMP-STN-002, EMP-STN-004, EMP-STN-005, EMP-PLAT-008
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: dashboard, profile, price and feedback controls · out: enabling Postos from a company session alone
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Actions follow current authority and the owning server APIs.
- **Acceptance:** Actions follow current authority and the owning server APIs.
- **Negative cases:** Foreign station, expired authority, optimistic publication or unreviewed portal activation.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-STN-009 — Portal: network overview
- **Meta:** module STN · phase F7 · cut - · priority P2 · type WEB · size S · state TODO
- **Objective:** Summarize a network using only the viewer's authorized station scopes.
- **Rules and use cases:** REQ-15.1, BR-CLAIM-033, BR-AUTH-004
- **Depends on:** EMP-STN-008, EMP-CLAIM-009
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: scoped station summary · out: common-root inferred access
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Every station in the summary is currently authorized and source-labelled.
- **Acceptance:** Every station in the summary is currently authorized and source-labelled.
- **Negative cases:** Foreign station included, private role leaked or authority gap hidden.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-STN-010 — Ranking neutrality and trust-independence tests
- **Meta:** module STN · phase F7 · cut - · priority P1 · type TEST · size S · state TODO
- **Objective:** Prove commercial status cannot change trust or organic ordering.
- **Rules and use cases:** BR-SRC-002, EDR-022, VAL-33
- **Depends on:** EMP-STN-002
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: source, trust and ranking invariance tests · out: weakening the public ranking contract
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Changing plan or announcement leaves trust, moderation and organic ordering unchanged.
- **Acceptance:** Changing plan or announcement leaves trust, moderation and organic ordering unchanged.
- **Negative cases:** Tier buys confidence, review immunity or undisclosed ranking position.
- **Validation:** V-SQL, V-API, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-AGREE-001 — Agreement domain: parties, scope, price terms and validity
- **Meta:** module AGREE · phase F7 · cut - · priority P1 · type DOM · size M · state TODO
- **Objective:** Define agreement parties, scope, terms and validity.
- **Rules and use cases:** REQ-15.5, EDR-023, BR-TEN-005, BR-MONEY-001
- **Depends on:** EMP-PLAT-009
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: pure agreement and allowed field boundary · out: organization-wide sharing
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Terms are exact, time-bound and limited to named parties and transaction scope.
- **Acceptance:** Terms are exact, time-bound and limited to named parties and transaction scope.
- **Negative cases:** Unknown party, incompatible unit, unbounded term or general private access.
- **Validation:** V-DOM, V-MONEY · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-AGREE-002 — Agreement lifecycle: propose, accept, suspend, end
- **Meta:** module AGREE · phase F7 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Record two-party agreement proposal and lifecycle.
- **Rules and use cases:** REQ-15.5, EDR-023, BR-AUD-001
- **Depends on:** EMP-AGREE-001, EMP-CLAIM-007, EMP-ORG-011
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: authorized accept, suspend and end · out: unilateral expansion of shared fields
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Only an eligible party confirms its side and each transition is audited.
- **Acceptance:** Only an eligible party confirms its side and each transition is audited.
- **Negative cases:** Foreign acceptance, stale proposal, automatic authority from payment or duplicate decision.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-AGREE-003 — Private negotiated price tables
- **Meta:** module AGREE · phase F7 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Maintain private negotiated price tables for valid agreements.
- **Rules and use cases:** REQ-15.5, BR-SRC-001, BR-TEN-005, BR-MONEY-001
- **Depends on:** EMP-AGREE-002
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: exact product, unit and conditions · out: public publication or mixing sources
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A table is visible only within its contract and recorded validity.
- **Acceptance:** A table is visible only within its contract and recorded validity.
- **Negative cases:** Expired term, foreign table, implicit public source or incompatible units.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-AGREE-004 — Authorisations and vouchers: limits, expiry, single use and cancellation
- **Meta:** module AGREE · phase F7 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Issue bounded expiring fueling authorizations and vouchers.
- **Rules and use cases:** REQ-15.5, BR-TEN-005, BR-MONEY-001, BR-AUD-001
- **Depends on:** EMP-AGREE-002, EMP-BUDG-003
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: limit, single use and cancellation · out: bearer code becoming general organization access
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A voucher carries the documented party and transaction limits and is consumed once.
- **Acceptance:** A voucher carries the documented party and transaction limits and is consumed once.
- **Negative cases:** Expired or foreign use, duplicate issue, overspend or code disclosed in logs.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-AGREE-005 — Redemption at the station: validation, attempt limit and idempotent confirmation
- **Meta:** module AGREE · phase F7 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Redeem the authorized transaction online once with bounded attempts.
- **Rules and use cases:** REQ-15.5, BR-TEN-005, BR-OFFL-017
- **Depends on:** EMP-AGREE-004
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: current authority, voucher validation and replay · out: offline double redemption
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Matching retries return the same authorized redemption and invalid attempts reveal no private data.
- **Acceptance:** Matching retries return the same authorized redemption and invalid attempts reveal no private data.
- **Negative cases:** Two concurrent consumes, expired code, wrong station or unbounded retry.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-AGREE-006 — Transaction to fleet fueling and station receivable with minimal sharing
- **Meta:** module AGREE · phase F7 · cut - · priority P1 · type DOM · size M · state TODO
- **Objective:** Project the contracted transaction into fleet fueling and station settlement with minimal sharing.
- **Rules and use cases:** REQ-15.6, EDR-013, EDR-023, BR-TEN-005, VAL-34
- **Depends on:** EMP-AGREE-005, EMP-FUEL-002
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: allowlisted transaction facts and once-only origins · out: full fleet, employee, document or track visibility
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The two parties see only their permitted transaction fields and recognized costs remain once.
- **Acceptance:** The two parties see only their permitted transaction fields and recognized costs remain once.
- **Negative cases:** Foreign transaction, private record leak, duplicate cost or broad cross-organization joins.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-AGREE-007 — Statements and invoice reconciliation
- **Meta:** module AGREE · phase F7 · cut - · priority P2 · type API · size M · state TODO
- **Objective:** Reconcile contracted statements without generating duplicate economic costs.
- **Rules and use cases:** REQ-15.5, EDR-013, BR-MONEY-001, BR-TRUTH-001
- **Depends on:** EMP-AGREE-006, EMP-FIN-005
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: scoped transaction and invoice references · out: claiming provider settlement
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Statement sums and discrepancies are exact and linked to approved transactions.
- **Acceptance:** Statement sums and discrepancies are exact and linked to approved transactions.
- **Negative cases:** Duplicate receivable, foreign invoice, mismatch concealed or internal state called payment confirmation.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-AGREE-008 — Panel and portal: agreements, vouchers and redemption
- **Meta:** module AGREE · phase F7 · cut - · priority P1 · type WEB · size L · state TODO
- **Objective:** Present agreement and redemption workflows separately to each authorized party.
- **Rules and use cases:** REQ-15.5, REQ-15.6, BR-AUTH-012, BR-TEN-005
- **Depends on:** EMP-AGREE-004, EMP-AGREE-005, EMP-PLAT-008
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: company and station scoped views · out: shared unrestricted workspace
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Each side sees its own allowed fields and current outcomes.
- **Acceptance:** Each side sees its own allowed fields and current outcomes.
- **Negative cases:** Opposite party's private data, stale redemption shown complete or scope switch leak.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-AGREE-009 — Cross-organization sharing guard tests
- **Meta:** module AGREE · phase F7 · cut - · priority P1 · type TEST · size S · state TODO
- **Objective:** Prove each cross-organization surface obeys the transaction allowlist.
- **Rules and use cases:** REQ-15.6, EDR-023, BR-TEN-005, BR-PRIV-002, VAL-34
- **Depends on:** EMP-AGREE-006
- **Read first:** [commerce requirements](../17-traceability.md), [decisions](../16-decisions.md), [domain model](../06-domain-model.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: API, job, file and UI denials · out: tolerating private field leakage
- **Areas:** existing: scoped station authority, profile, public source and organization finance ports · proposed: gated commerce and agreement consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Foreign requests expose only explicitly contracted fields at all supported entry points.
- **Acceptance:** Foreign requests expose only explicitly contracted fields at all supported entry points.
- **Negative cases:** Hidden private field in export, background job, link or error.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The commerce design, Stage D and each agreement's reviewed field allowlist govern implementation and activation.
- **Done when:** The stated result and denials pass the listed validation profiles.
