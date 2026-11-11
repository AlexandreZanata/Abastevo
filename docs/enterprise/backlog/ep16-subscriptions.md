# EP16 — Subscriptions, entitlements and public products

Status: **PLANNED**, 2026-11-10.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from [decision register](../16-decisions.md), [state model](../08-state-model.md), [product vision](../02-product-vision.md) and [journeys](../05-journeys.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

The [subscription design](../modules/subscriptions.md) records the separate plan boundary. Account entitlements and existing public access keep their scope.

### EMP-SUBS-001 — Organization product, plan and entitlement catalog as data, beside the account tier catalog
- **Meta:** module SUBS · phase F8 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Define a versioned organization plan catalog independent of account tiers.
- **Rules and use cases:** EDR-024, BR-AUTH-009, REQ-16.2, REQ-16.5
- **Depends on:** EMP-PLAT-005
- **Read first:** [decision register](../16-decisions.md), [state model](../08-state-model.md), [product vision](../02-product-vision.md) and [journeys](../05-journeys.md)
- **Scope:** in: plans, capability IDs and measurable limits · out: pricing commitments or live charging
- **Areas:** existing: account entitlements, scoped capabilities and payment policy · proposed: independent organization catalog, billing state and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Plans are data and confer no role. Commercial hypotheses remain outside runtime until approved.
- **Acceptance:** Plans are data and confer no role. Commercial hypotheses remain outside runtime until approved.
- **Negative cases:** Account tier grants organization access, unknown limit silently allows or price hard-coded from hypothesis.
- **Validation:** V-DOM, V-API, V-AUTHZ · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [subscription design](../modules/subscriptions.md) and billing approvals govern detailed transitions, fiscal scope and activation. This recovery does not admit a provider or price.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-SUBS-002 — Subscription lifecycle state machine
- **Meta:** module SUBS · phase F8 · cut - · priority P1 · type DOM · size M · state TODO
- **Objective:** Model explicit organization subscription states and eligible transitions.
- **Rules and use cases:** EDR-024, BR-AUTH-009, BR-TRUTH-001, REQ-16.3, VAL-35
- **Depends on:** EMP-SUBS-001
- **Read first:** [decision register](../16-decisions.md), [state model](../08-state-model.md), [product vision](../02-product-vision.md) and [journeys](../05-journeys.md)
- **Scope:** in: trial, active, past due, suspension and cancellation history · out: provider execution and implied roles
- **Areas:** existing: account entitlements, scoped capabilities and payment policy · proposed: independent organization catalog, billing state and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A transition retains its recorded source, time and basis and cannot claim successful billing without authority.
- **Acceptance:** A transition retains its recorded source, time and basis and cannot claim successful billing without authority.
- **Negative cases:** Invalid state change, payment inferred locally or ended state reopens by replay.
- **Validation:** V-DOM, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [subscription design](../modules/subscriptions.md) and billing approvals govern detailed transitions, fiscal scope and activation. This recovery does not admit a provider or price.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-SUBS-003 — Organization entitlement evaluation: capability gates and measurable limits as a new entitlement source
- **Meta:** module SUBS · phase F8 · cut - · priority P1 · type DOM · size M · state TODO
- **Objective:** Resolve organization capabilities and measurable limits through the existing gate.
- **Rules and use cases:** BR-AUTH-001, BR-AUTH-009, BR-TEN-003, REQ-16.2
- **Depends on:** EMP-SUBS-001, EMP-AUTH-004
- **Read first:** [decision register](../16-decisions.md), [state model](../08-state-model.md), [product vision](../02-product-vision.md) and [journeys](../05-journeys.md)
- **Scope:** in: server-side plan/capability evaluation · out: granting membership
- **Areas:** existing: account entitlements, scoped capabilities and payment policy · proposed: independent organization catalog, billing state and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Only current scoped entitlements enable new actions and independent role checks still apply.
- **Acceptance:** Only current scoped entitlements enable new actions and independent role checks still apply.
- **Negative cases:** Missing state allows, foreign entitlement, tier used as membership or stale cache widens scope.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [subscription design](../modules/subscriptions.md) and billing approvals govern detailed transitions, fiscal scope and activation. This recovery does not admit a provider or price.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-SUBS-004 — Organization billing on the existing payment providers: offers, idempotent event intake and reconciliation
- **Meta:** module SUBS · phase F8 · cut - · priority P1 · type INT · size L · state TODO
- **Objective:** Map approved provider events to organization billing records once.
- **Rules and use cases:** EDR-024, CX-12, BR-AUTH-009, REQ-16.4, VAL-18
- **Depends on:** EMP-GOV-009, EMP-SUBS-002
- **Read first:** [decision register](../16-decisions.md), [state model](../08-state-model.md), [product vision](../02-product-vision.md) and [journeys](../05-journeys.md)
- **Scope:** in: reviewed offer binding, event identity and reconciliation · out: live activation or billing a different account
- **Areas:** existing: account entitlements, scoped capabilities and payment policy · proposed: independent organization catalog, billing state and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Authorized verified events converge on one transition and reconciliation reports unsupported or inconsistent events.
- **Acceptance:** Authorized verified events converge on one transition and reconciliation reports unsupported or inconsistent events.
- **Negative cases:** Duplicate event, wrong offer scope, failure treated as settled or unapproved provider called.
- **Validation:** V-API, V-SQL, V-IDEM, V-AUTHZ, V-ISO, V-MONEY · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [subscription design](../modules/subscriptions.md) and billing approvals govern detailed transitions, fiscal scope and activation. This recovery does not admit a provider or price.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-SUBS-005 — Organization effects per subscription state, including queued offline operations
- **Meta:** module SUBS · phase F8 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Apply subscription state to writes, reads, export and queued work explicitly.
- **Rules and use cases:** BR-AUTH-009, BR-OFFL-009, BR-PRIV-012, REQ-16.3, VAL-35
- **Depends on:** EMP-SUBS-002, EMP-ORG-009, EMP-OFFL-012
- **Read first:** [decision register](../16-decisions.md), [state model](../08-state-model.md), [product vision](../02-product-vision.md) and [journeys](../05-journeys.md)
- **Scope:** in: state-specific access effects and pre-change intake · out: deleting history or changing role assignments
- **Areas:** existing: account entitlements, scoped capabilities and payment policy · proposed: independent organization catalog, billing state and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Old authorized facts follow the documented review path. Authorized retained reads and exports follow the current state policy.
- **Acceptance:** Old authorized facts follow the documented review path. Authorized retained reads and exports follow the current state policy.
- **Negative cases:** Prior data erased, current state guessed, old session ignores downgrade or role changed by a plan.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-SYNC · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [subscription design](../modules/subscriptions.md) and billing approvals govern detailed transitions, fiscal scope and activation. This recovery does not admit a provider or price.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-SUBS-006 — Upgrade, downgrade, overage and resource archive or reactivation
- **Meta:** module SUBS · phase F8 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Apply plan changes and resolve overage without destroying data.
- **Rules and use cases:** BR-AUTH-009, BR-OFFL-009, REQ-16.3, JRN-11
- **Depends on:** EMP-SUBS-003
- **Read first:** [decision register](../16-decisions.md), [state model](../08-state-model.md), [product vision](../02-product-vision.md) and [journeys](../05-journeys.md)
- **Scope:** in: effective plan change and archive/reactivation selection · out: silently archiving assets or erasing history
- **Areas:** existing: account entitlements, scoped capabilities and payment policy · proposed: independent organization catalog, billing state and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The documented period change and selected assets produce a scoped result. Archived records remain readable and exportable where allowed.
- **Acceptance:** The documented period change and selected assets produce a scoped result. Archived records remain readable and exportable where allowed.
- **Negative cases:** Wrong effective date, unchosen data loss, duplicate transition or unauthorized reactivation.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [subscription design](../modules/subscriptions.md) and billing approvals govern detailed transitions, fiscal scope and activation. This recovery does not admit a provider or price.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-SUBS-007 — Trial and activation
- **Meta:** module SUBS · phase F8 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Record trial and activation only under an approved catalog and policy.
- **Rules and use cases:** EDR-024, BR-AUTH-009, REQ-16.3
- **Depends on:** EMP-SUBS-002
- **Read first:** [decision register](../16-decisions.md), [state model](../08-state-model.md), [product vision](../02-product-vision.md) and [journeys](../05-journeys.md)
- **Scope:** in: server-confirmed trial and activation transitions · out: inventing trial length or starting charges
- **Areas:** existing: account entitlements, scoped capabilities and payment policy · proposed: independent organization catalog, billing state and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Eligibility and activation have a recorded authority and do not alter membership.
- **Acceptance:** Eligibility and activation have a recorded authority and do not alter membership.
- **Negative cases:** Replayed trial extends period, client enables it or unreviewed policy becomes a runtime default.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [subscription design](../modules/subscriptions.md) and billing approvals govern detailed transitions, fiscal scope and activation. This recovery does not admit a provider or price.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-SUBS-008 — Panel: plan and billing
- **Meta:** module SUBS · phase F8 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Present scoped plan, capabilities, limits and billing status in the company panel.
- **Rules and use cases:** EDR-024, BR-AUTH-009, BR-TRUTH-001, REQ-16.2
- **Depends on:** EMP-SUBS-006, EMP-SUBS-007, EMP-PLAT-008
- **Read first:** [decision register](../16-decisions.md), [state model](../08-state-model.md), [product vision](../02-product-vision.md) and [journeys](../05-journeys.md)
- **Scope:** in: authorized company billing views · out: purchase links in Android and provider activation
- **Areas:** existing: account entitlements, scoped capabilities and payment policy · proposed: independent organization catalog, billing state and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Plan state and pending changes come from server records. Personal tier remains separate.
- **Acceptance:** Plan state and pending changes come from server records. Personal tier remains separate.
- **Negative cases:** Wrong organization, payment success inferred, unapproved price displayed or action hidden instead of checked.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [subscription design](../modules/subscriptions.md) and billing approvals govern detailed transitions, fiscal scope and activation. This recovery does not admit a provider or price.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-SUBS-009 — Usage metering: active vehicles, stations, storage and API volume
- **Meta:** module SUBS · phase F8 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Meter approved organization resources and usage from named sources.
- **Rules and use cases:** BR-AUTH-009, BR-PRIV-008, BR-TRUTH-004, REQ-16.2
- **Depends on:** EMP-SUBS-001
- **Read first:** [decision register](../16-decisions.md), [state model](../08-state-model.md), [product vision](../02-product-vision.md) and [journeys](../05-journeys.md)
- **Scope:** in: active vehicle, station, storage and API counters · out: charging or exposing private data
- **Areas:** existing: account entitlements, scoped capabilities and payment policy · proposed: independent organization catalog, billing state and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A meter states the period, unit and source and duplicate events do not inflate usage.
- **Acceptance:** A meter states the period, unit and source and duplicate events do not inflate usage.
- **Negative cases:** Foreign usage, negative count, retired resource counted as active or unbounded identity in telemetry.
- **Validation:** V-SQL, V-JOB, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [subscription design](../modules/subscriptions.md) and billing approvals govern detailed transitions, fiscal scope and activation. This recovery does not admit a provider or price.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-SUBS-010 — Individual Premium kept separate from organization plans in evaluation and interface
- **Meta:** module SUBS · phase F8 · cut - · priority P2 · type DOM · size M · state TODO
- **Objective:** Keep individual Premium and organization plans independent.
- **Rules and use cases:** EDR-002, EDR-024, BR-AUTH-009, BR-TEN-010, REQ-16.2
- **Depends on:** EMP-SUBS-001
- **Read first:** [decision register](../16-decisions.md), [state model](../08-state-model.md), [product vision](../02-product-vision.md) and [journeys](../05-journeys.md)
- **Scope:** in: separate entitlement evaluation and UI context · out: converting account tier into organization permission
- **Areas:** existing: account entitlements, scoped capabilities and payment policy · proposed: independent organization catalog, billing state and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Account rights and company capability labels remain distinct even on one device.
- **Acceptance:** Account rights and company capability labels remain distinct even on one device.
- **Negative cases:** Personal plan unlocks staff role, organization plan alters free participation or context mixes billing data.
- **Validation:** V-DOM, V-APP, V-AUTHZ, V-ISO · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [subscription design](../modules/subscriptions.md) and billing approvals govern detailed transitions, fiscal scope and activation. This recovery does not admit a provider or price.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-SUBS-011 — Invoices and the service tax document boundary
- **Meta:** module SUBS · phase F8 · cut - · priority P2 · type INT · size S · state TODO
- **Objective:** Record the approved invoice and service tax document boundary.
- **Rules and use cases:** EDR-024, BR-TRUTH-002, BR-MONEY-001, CX-12
- **Depends on:** EMP-SUBS-004
- **Read first:** [decision register](../16-decisions.md), [state model](../08-state-model.md), [product vision](../02-product-vision.md) and [journeys](../05-journeys.md)
- **Scope:** in: invoice facts and reviewed status · out: issuing fiscal documents or promising tax compliance before review
- **Areas:** existing: account entitlements, scoped capabilities and payment policy · proposed: independent organization catalog, billing state and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A billing record distinguishes internal status from fiscal verification and preserves exact totals.
- **Acceptance:** A billing record distinguishes internal status from fiscal verification and preserves exact totals.
- **Negative cases:** Provider confirmation invented, undisclosed real-data use or duplicate financial record.
- **Validation:** V-DOC, V-API, V-SQL, V-IDEM, V-MONEY · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [subscription design](../modules/subscriptions.md) and billing approvals govern detailed transitions, fiscal scope and activation. This recovery does not admit a provider or price.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-PUB-001 — Personal and enterprise separation in app storage and interface
- **Meta:** module PUB · phase F1 · cut C1 · priority P0 · type APP · size M · state TODO
- **Objective:** Keep personal and company records separate across local storage and UI.
- **Rules and use cases:** BR-TEN-004, BR-TEN-010, BR-SRC-003, BR-PRIV-016, REQ-01.4
- **Depends on:** EMP-PLAT-011
- **Read first:** [decision register](../16-decisions.md), [state model](../08-state-model.md), [product vision](../02-product-vision.md) and [journeys](../05-journeys.md)
- **Scope:** in: explicit context and partitioned state · out: sharing queues or record ownership
- **Areas:** existing: account entitlements, scoped capabilities and payment policy · proposed: independent organization catalog, billing state and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Personal mode and each organization load only their authorized stores.
- **Acceptance:** Personal mode and each organization load only their authorized stores.
- **Negative cases:** Personal data appears in company mode or a queued operation changes context.
- **Validation:** V-APP, V-SYNC, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [subscription design](../modules/subscriptions.md) and billing approvals govern detailed transitions, fiscal scope and activation. This recovery does not admit a provider or price.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-PUB-002 — Voluntary price publication from an enterprise fueling
- **Meta:** module PUB · phase F6 · cut - · priority P2 · type API · size M · state TODO
- **Objective:** Offer voluntary publication only through a reviewed public-field allowlist.
- **Rules and use cases:** BR-SRC-001, BR-SRC-002, BR-SRC-003, BR-SRC-004, BR-PRIV-005
- **Depends on:** EMP-FUEL-003
- **Read first:** [decision register](../16-decisions.md), [state model](../08-state-model.md), [product vision](../02-product-vision.md) and [journeys](../05-journeys.md)
- **Scope:** in: explicit action and permitted contribution data · out: automatic publishing or sharing receipts, contracts and employee records
- **Areas:** existing: account entitlements, scoped capabilities and payment policy · proposed: independent organization catalog, billing state and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Public contribution follows existing eligibility, provenance and neutrality rules.
- **Acceptance:** Public contribution follows existing eligibility, provenance and neutrality rules.
- **Negative cases:** Private fields leak, plan changes trust or publication happens without explicit user action.
- **Validation:** V-API, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [subscription design](../modules/subscriptions.md) and billing approvals govern detailed transitions, fiscal scope and activation. This recovery does not admit a provider or price.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-PUB-003 — Personal vehicles and history stay personal, with explicit copy only
- **Meta:** module PUB · phase F2 · cut - · priority P1 · type DOM · size S · state TODO
- **Objective:** Preserve personal vehicle history and any separately authorized copy boundary.
- **Rules and use cases:** BR-TEN-004, BR-TEN-010, BR-FUEL-024, BR-SRC-003
- **Depends on:** EMP-PUB-001, EMP-FLEET-002
- **Read first:** [decision register](../16-decisions.md), [state model](../08-state-model.md), [product vision](../02-product-vision.md) and [journeys](../05-journeys.md)
- **Scope:** in: explicit allowed copy choice under the owning contract · out: moving histories, fuelings, queues or ownership
- **Areas:** existing: account entitlements, scoped capabilities and payment policy · proposed: independent organization catalog, billing state and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** An allowed copy creates distinct data under explicit scope and does not change the original.
- **Acceptance:** An allowed copy creates distinct data under explicit scope and does not change the original.
- **Negative cases:** Implicit transfer, copied private history, shared record identity or organization overwrite of personal data.
- **Validation:** V-APP, V-SYNC, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [subscription design](../modules/subscriptions.md) and billing approvals govern detailed transitions, fiscal scope and activation. This recovery does not admit a provider or price.
- **Done when:** The stated result and denials pass the listed validation profiles.
