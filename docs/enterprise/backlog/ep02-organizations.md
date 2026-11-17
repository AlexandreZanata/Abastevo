# EP02 — Organizations

Task metadata follows the existing [generated index](INDEX.md). The task
definitions were reconstructed from the [organization specification](../modules/organizations.md) and the approved
private organization boundary. Local F1 requires M0 closure and protected
integration of the current ADR-019 construction batch. See the
sequencing decision.

### EMP-ORG-011 — Legal entity verification workflow
- **Meta:** module ORG · phase F7 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Record reviewed legal-entity verification with validity and evidence provenance.
- **Rules and use cases:** BR-ORG-016, BR-ORG-017, BR-ORG-018, BR-ORG-019, BR-ORG-020, UC-ORG-05
- **Depends on:** EMP-ORG-004, EMP-BACK-002
- **Read first:** [organization specification](../modules/organizations.md) §§4–5, [privacy](../privacy-security.md)
- **Scope:** in: authorized submission and scoped staff review · out: live verification providers or any implied Stage D override
- **Areas:** existing: declaration workflow and staff review conventions · proposed: verification queue and decision records
- **Steps:** Define reviewer scope, expiry and revocation tests. Preserve evidence kind, decision time and the checks performed.
- **Deliverable:** A gated verification workflow with honest statuses.
- **Acceptance:** A format check alone cannot create VERIFIED. Lapse or revocation removes eligibility for gated capabilities.
- **Negative cases:** Foreign evidence, reviewer outside scope, status overclaim, concurrent decision and private identifier disclosure.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Follow F7 activation, review staffing and applicable legal gates.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.

### EMP-ORG-012 — Organization groups and consolidation grants
- **Meta:** module ORG · phase F6 · cut - · priority P2 · type API · size M · state TODO
- **Objective:** Allow explicit read-only consolidation between authorized organizations.
- **Rules and use cases:** BR-ORG-021, BR-ORG-022, BR-ORG-023, BR-ORG-024, BR-TEN-005, UC-ORG-10
- **Depends on:** EMP-ORG-003, EMP-AUTH-006
- **Read first:** [organization specification](../modules/organizations.md) §§4–5, [permission matrix](../permissions.md)
- **Scope:** in: two-party grants, read-only aggregate allowlist and revocation · out: cross-organization raw data or writes
- **Areas:** existing: organization scope and permission model · proposed: consolidation grants and read projection
- **Steps:** Test consent, origin labels, revocation and read-time state. Prove the allowlist on synthetic organizations.
- **Deliverable:** Scoped aggregation grants and source-labelled views.
- **Acceptance:** Only accepted grant fields and periods are visible. Either side ends access and suspension pauses it.
- **Negative cases:** Shared CNPJ grants visibility, expired proposal, foreign raw record, private attachments or a write through the grant.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Separate this grant from membership and any agreement with stations.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.

### EMP-ORG-013 — Archive and closure: open operations, export and retention schedule
- **Meta:** module ORG · phase F8 · cut - · priority P2 · type API · size M · state TODO
- **Objective:** Archive and close organizations with recoverable export and retention controls.
- **Rules and use cases:** BR-ORG-033, BR-ORG-036, BR-ORG-037, BR-PRIV-012, UC-ORG-11, UC-ORG-12
- **Depends on:** EMP-ORG-009, EMP-RPT-005, EMP-PRIV-001
- **Read first:** [organization specification](../modules/organizations.md) §§4–5, [privacy](../privacy-security.md)
- **Scope:** in: open-items check, archive, export window and terminal closure · out: unreviewed retention and destructive production operation
- **Areas:** existing: lifecycle and reviewed retention controls · proposed: archive and close workflows
- **Steps:** Test blocking open work, scoped export, cancel window and terminal closure with isolated fixtures.
- **Deliverable:** A versioned closure workflow and recovery evidence.
- **Acceptance:** Archival gates writes. Terminal closure ends access and hands deletion to the reviewed retention process.
- **Negative cases:** Open work dropped, data deleted before export window, reopened terminal organization or deletion of a legal hold.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Reviewed retention and backup recovery govern disposal. Historical identifiers stay distinct.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.

### EMP-ORG-015 — Panel: units, cost centres and teams management
- **Meta:** module ORG · phase F1 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Expose unit, cost-centre and team management through the company panel.
- **Rules and use cases:** BR-ORG-006, BR-ORG-007, BR-ORG-008, BR-ORG-009, BR-ORG-011, UC-ORG-03
- **Depends on:** EMP-ORG-005, EMP-ORG-006, EMP-ORG-007, EMP-PLAT-008
- **Read first:** [organization specification](../modules/organizations.md) §§5 and 11, approved company panel
- **Scope:** in: structure lists and editing, archive, restore and conflict feedback · out: changing roles through team UI
- **Areas:** existing: business web scaffold and structure APIs · proposed: company structure screens
- **Steps:** Verify keyboard and error flows against the existing APIs. Switch organizations with pending form state.
- **Deliverable:** Accessible company structure screens and regression evidence.
- **Acceptance:** Authorized users see and edit only the selected organization's structure. Archived records remain identifiable.
- **Negative cases:** Foreign results after switching, invalid edit silently accepted or archived structure shown as active.
- **Validation:** V-WEB, V-AUTHZ, V-ISO · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Structure UI never grants permission. The server enforces revision and scope.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.
