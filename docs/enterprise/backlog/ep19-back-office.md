# EP19 — Internal administration and support

Status: **PLANNED**, 2026-11-10.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from the [existing admin contract](../../product/ADMIN_TIERS_CONTRACT.md), [support journey](../05-journeys.md), [privacy and security](../11-privacy-security.md) and [decision register](../16-decisions.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

The [back-office](../modules/back-office.md) and [web](../13-ux-web.md) designs now record their boundaries. The recovered tasks preserve the existing staff boundaries and approved support rules. New staff powers, station expansion and billing activation retain their separate gates.

### EMP-BACK-001 — Platform operator model for enterprise: staff roles, admin sessions and audit reused from the admin suite
- **Meta:** module BACK · phase F1 · cut - · priority P1 · type DOM · size M · state TODO
- **Objective:** Define bounded platform staff authority for enterprise administration.
- **Rules and use cases:** EDR-020, CX-08, BR-TEN-008, BR-AUD-003, BR-AUD-004, BR-PRIV-021, REQ-17.4
- **Depends on:** EMP-GOV-004, EMP-PLAT-003
- **Read first:** [existing admin contract](../../product/ADMIN_TIERS_CONTRACT.md) §§2 and 5, [decision register](../16-decisions.md), [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: fixed staff permissions, admin session boundary and audit · out: ordinary organization membership or implicit private-content access
- **Areas:** existing: admin and staff modules and append-only audit · proposed: enterprise operator permission model
- **Steps:** Apply the recorded GOV-004 decision when available. Pin allow and deny cases for staff actions and keep the company session boundary separate.
- **Deliverable:** A pure staff action catalog and documented admin enforcement contract.
- **Acceptance:** A staff assignment grants only reviewed actions at explicit consumers. Private organization data requires a separate approved support scope.
- **Negative cases:** Default staff data access, company session accepted by admin, a public account tier granting staff authority or unaudited action.
- **Validation:** V-DOM, V-AUTHZ, V-ISO, V-PRIV · migration: none for the catalog · rollback: keep new enterprise admin routes closed
- **Risks and assumptions:** The GOV-004 decision and specific consumer tests are prerequisites. The [back-office specification](../modules/back-office.md) governs its consumers.
- **Done when:** The catalog and deny matrix agree with reviewed roles and session boundaries.

### EMP-BACK-002 — Review queues in the admin suite: claims, legal-entity verification and moderation hand-off
- **Meta:** module BACK · phase F7 · cut - · priority P1 · type INT · size M · state TODO
- **Objective:** Provide authorized internal review queues for claims and legal-entity verification, with moderation handoff.
- **Rules and use cases:** EDR-020, EDR-021, CX-05, BR-ORG-016, BR-ORG-018, BR-TEN-008, BR-AUD-001, BR-AUD-003, BR-PRIV-021, UC-ORG-05, VAL-17
- **Depends on:** EMP-BACK-001
- **Read first:** [organization verification](../modules/organizations.md), [station claims](../modules/station-claims.md), [existing admin contract](../../product/ADMIN_TIERS_CONTRACT.md) §5, [decision register](../16-decisions.md)
- **Scope:** in: scoped staff queues and existing decision ports for claims, entity verification and moderation referral · out: bypassing Stage D or unrestricted browsing
- **Areas:** existing: admin/staff audit, organization verification design and station claim workflows · proposed: queue registration and typed handoff
- **Steps:** Separate claim, verification and moderation decisions. Prove staff authority, applicable stage gate, review-state transitions, concurrency and bounded evidence handling.
- **Deliverable:** Authorized admin queues and a review decision handoff for the legal-entity workflow.
- **Acceptance:** Each item exposes only the evidence and actions allowed for its reviewer and workflow. A decision records what was checked, the outcome and its audit trail.
- **Negative cases:** Unapproved staff access, duplicate or stale decision, evidence of an unrelated case, a declaration shown as verified or a station action bypassing Stage D.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive queue state if needed · rollback: close admission and preserve recorded outcomes
- **Risks and assumptions:** Queue access is scoped to the approved workflow and does not confer general organization-content access. Retain the separate station and production gates.
- **Done when:** Normal review, forbidden access and concurrent decision checks pass with isolated synthetic cases.

### EMP-BACK-003 — Support access: time-boxed, organization-approved and audited
- **Meta:** module BACK · phase F3 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Permit exceptional support access only within a time-boxed organization-approved session.
- **Rules and use cases:** BR-TEN-008, BR-AUD-003, BR-AUD-004, BR-AUD-006, BR-PRIV-021, BR-AUTH-011, JRN-13
- **Depends on:** EMP-BACK-001, EMP-AUTH-003
- **Read first:** [support journey](../05-journeys.md), [privacy and security](../11-privacy-security.md) §4, [state model](../08-state-model.md), [existing admin contract](../../product/ADMIN_TIERS_CONTRACT.md) §2
- **Scope:** in: support request, approval, scoped access, expiry and audit · out: default access or broad document/location rights
- **Areas:** existing: separate staff session and organization authorization · proposed: support session lifecycle and per-access gate
- **Steps:** Test approval and expiry with controlled time. Verify that each access checks the exact approved organization and category and records an audit entry.
- **Deliverable:** A bounded support-access API and an organization-visible audit trail.
- **Acceptance:** An approved session grants only its named scope during its validity. Documents and location require explicit inclusion and their owning rules.
- **Negative cases:** Missing approval, expired session, foreign organization, unrelated content, ordinary role bypass or successful read without audit.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: additive support state · rollback: terminate new access while preserving the audit record
- **Risks and assumptions:** A support session is separate from regular staff or company access.
- **Done when:** Approval, scoped reads, expiry and negative access pass on isolated fixtures.

### EMP-BACK-004 — Organization administration commands: suspend, reactivate and quality tools
- **Meta:** module BACK · phase F1 · cut - · priority P1 · type INT · size S · state TODO
- **Objective:** Expose reviewed platform commands for organization state and authorized quality operations.
- **Rules and use cases:** EDR-020, BR-ORG-029, BR-ORG-030, BR-ORG-032, BR-AUD-001, BR-AUD-004, UC-ORG-08
- **Depends on:** EMP-BACK-001, EMP-ORG-009
- **Read first:** [existing admin contract](../../product/ADMIN_TIERS_CONTRACT.md) §§2 and 5, [organization lifecycle](../modules/organizations.md), [permission matrix](../07-permissions.md) §4
- **Scope:** in: audited suspension, reactivation and narrowly scoped quality controls · out: new unreviewed operational powers
- **Areas:** existing: admin sessions and organization lifecycle commands · proposed: authorized admin adapters
- **Steps:** Reuse the owning lifecycle command. Cover required reasons, current scope, replay and an organization whose policy blocks the action.
- **Deliverable:** Audited admin entry points for reviewed organization commands.
- **Acceptance:** Authorized commands change the selected organization once and preserve the existing write-block and recovery semantics.
- **Negative cases:** Staff without the specific permission changes state, wrong organization, replay without audit or quality tools reading unrelated private content.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: reuse owning state · rollback: close new admin commands and retain accepted state
- **Risks and assumptions:** Quality operations require explicit owning permissions and a reviewed scope before their inclusion.
- **Done when:** Admin and lifecycle checks pass through the normal command boundary.

### EMP-BACK-005 — Admin-web screens for enterprise administration and support
- **Meta:** module BACK · phase F7 · cut - · priority P2 · type WEB · size M · state TODO
- **Objective:** Expose scoped enterprise review and support workflows in the existing admin web.
- **Rules and use cases:** EDR-020, BR-TEN-008, BR-PRIV-021, BR-AUD-003, REQ-17.4, REQ-20.4
- **Depends on:** EMP-BACK-002, EMP-BACK-003
- **Read first:** [existing admin contract](../../product/ADMIN_TIERS_CONTRACT.md), [support journey](../05-journeys.md), [enterprise traceability](../17-traceability.md) §§17 and 20
- **Scope:** in: admin queue views, reviewed actions and support session states · out: company panel or Postos session reuse
- **Areas:** existing: admin-web and authorized admin endpoints · proposed: enterprise administration and support screens
- **Steps:** Exercise review and support states through the admin session. Verify bounded output, denial and stale or expired actions with accessible controls.
- **Deliverable:** Separate admin views for the approved enterprise workflows.
- **Acceptance:** Visible actions reflect current staff authority and support scope. The panel shows authoritative results without broad private data.
- **Negative cases:** Company cookie accepted, unapproved evidence visible, expired support read continues or the UI claims success after denial.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: none · rollback: hide views and preserve staff decisions and audit
- **Risks and assumptions:** The [web design](../13-ux-web.md) records the separate admin surface. Stage D gates continue to govern relevant actions.
- **Done when:** Allowed and denied views and accessible state transitions pass through the admin entry point.

### EMP-BACK-006 — Subscription administration: credits and manual adjustments
- **Meta:** module BACK · phase F8 · cut - · priority P2 · type INT · size S · state TODO
- **Objective:** Record reviewed organization subscription credits and manual adjustments with audit.
- **Rules and use cases:** EDR-024, CX-03, CX-12, BR-MONEY-001, BR-AUTH-009, BR-AUD-001, BR-AUD-004
- **Depends on:** EMP-BACK-001, EMP-SUBS-002
- **Read first:** [decision register](../16-decisions.md), [existing admin contract](../../product/ADMIN_TIERS_CONTRACT.md) §§2 and 5, [architecture](../09-architecture.md) §12
- **Scope:** in: approved organization billing adjustments and traceable scope · out: payment activation, changing individual entitlements or granting roles
- **Areas:** existing: admin mutation and organization subscription conventions · proposed: bounded adjustment commands
- **Steps:** Pin exact amounts, allowed reasons, duplicate requests and role separation. Reconcile each accepted change with the owning subscription record.
- **Deliverable:** An audited manual adjustment workflow under the approved billing contract.
- **Acceptance:** An allowed adjustment applies once to the named organization and preserves its reason and history. Role assignments stay independent.
- **Negative cases:** Foreign organization, invalid amount, missing authority, duplicate credit, private payment content in audit or inferred provider settlement.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY, V-PRIV · migration: additive if needed · rollback: disable new adjustments and preserve accounting history
- **Risks and assumptions:** The organization billing decision and subscription tasks define which adjustments are permitted.
- **Done when:** Exact-value, scope, concurrent replay and audit tests pass without activating a provider.
