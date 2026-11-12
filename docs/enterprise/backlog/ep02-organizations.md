# EP02 — Organizations

Status: **IN_PROGRESS**, 2026-11-11.

Task metadata follows the existing [generated index](INDEX.md). The task
definitions were reconstructed from the [organization specification](../modules/organizations.md) and the approved
private organization boundary. Local F1 requires M0 closure and protected
integration of the current ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

### EMP-ORG-001 — Organization aggregate and lifecycle rules
- **Meta:** module ORG · phase F1 · cut C1 · priority P0 · type DOM · size S · state LOCAL_DONE
- **Objective:** Define the organization aggregate and its lifecycle before adapters.
- **Rules and use cases:** BR-ORG-001, BR-ORG-002, BR-ORG-004, BR-ORG-025, BR-ORG-027, BR-ORG-037
- **Depends on:** EMP-PLAT-001
- **Read first:** [organization specification](../modules/organizations.md) §§4 and 6, [architecture](../09-architecture.md)
- **Scope:** in: pure aggregate and allowed lifecycle transitions · out: persistence, onboarding and actual closure
- **Areas:** existing: platform scope types · proposed: organization domain package
- **Steps:** Write lifecycle vectors. Implement the aggregate and explicit allowed transitions.
- **Deliverable:** A pure organization lifecycle model.
- **Acceptance:** The same inputs yield the same state. An organization cannot transition out of CLOSED.
- **Negative cases:** Invalid kind, incomplete activation, resurrected closure and permission inferred from display attributes.
- **Validation:** V-DOM · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Acting authority and lifecycle side effects are checked by the owning application commands.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.

[Local aggregate and lifecycle evidence](../evidence/f1-org001-organization-domain.md).

### EMP-ORG-002 — Organization persistence as the tenant root
- **Meta:** module ORG · phase F1 · cut C1 · priority P0 · type DB · size S · state LOCAL_DONE
- **Objective:** Persist the organization as the ownership root for private records.
- **Rules and use cases:** BR-ORG-001, BR-ORG-002, BR-ORG-003, BR-TEN-005, BR-PRIV-001
- **Depends on:** EMP-ORG-001, EMP-PLAT-002
- **Read first:** [organization specification](../modules/organizations.md) §§3 and 10, [architecture](../09-architecture.md)
- **Scope:** in: organization root, scoped repositories and safe migration · out: consumer feature tables
- **Areas:** existing: sqlc, PostGIS fixture and platform schema conventions · proposed: organization root schema and adapter
- **Steps:** Add failing scoped storage cases. Define and rehearse an additive root migration and repository.
- **Deliverable:** A persisted organization boundary under the ordinary application role.
- **Acceptance:** Roots can be accessed only through resolved scope or a dedicated membership-aware entry point.
- **Negative cases:** Absent scope, cross-organization reference, role bypass and same display name leaking existence.
- **Validation:** V-SQL, V-ISO, V-IDEM · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Verify additive upgrade and previous-image compatibility on populated fixtures.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.

[Local persistence, isolation and populated-upgrade evidence](../evidence/f1-org002-organization-persistence.md).

### EMP-ORG-003 — Create-organization flow: the creator becomes owner and a default unit exists
- **Meta:** module ORG · phase F1 · cut C1 · priority P0 · type API · size M · state LOCAL_DONE
- **Objective:** Create a pending organization with its first owner and operating unit atomically.
- **Rules and use cases:** BR-ORG-006, BR-ORG-025, BR-ORG-026, BR-ORG-027, BR-ORG-028, UC-ORG-01, UC-ORG-02
- **Depends on:** EMP-ORG-002, EMP-IDN-003, EMP-AUTH-002, EMP-PLAT-004, EMP-PLAT-003
- **Read first:** [organization specification](../modules/organizations.md) §§4–5, [permission matrix](../07-permissions.md)
- **Scope:** in: creation, idempotency and setup completion contract · out: public enrollment or real-data activation
- **Areas:** existing: account session and platform audit · proposed: organization create and activate commands
- **Steps:** Test fresh authentication, rollback, replay and concurrent pending-setup limits. Compose the owner and unit writes in one transaction.
- **Deliverable:** A creation and setup API with real-PostGIS evidence.
- **Acceptance:** Successful creation yields one pending organization, unit, employee record and owner. A replay returns the same result.
- **Negative cases:** Managed or stale session, missing terms, invalid time zone, partial commit and racing limit bypass.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Synthetic validation uses synthetic terms evidence. Production terms depend on legal review.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.

[Atomic onboarding, current-proof, isolation and upgrade evidence](../evidence/f1-org003-atomic-onboarding.md).

### EMP-ORG-004 — Legal entities with CNPJ and CPF validation, declared versus verified
- **Meta:** module ORG · phase F1 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Record declared legal entities without implying verification or granting access.
- **Rules and use cases:** BR-ORG-013, BR-ORG-014, BR-ORG-015, BR-ORG-016, BR-ORG-017, BR-ORG-020, UC-ORG-04
- **Depends on:** EMP-ORG-003
- **Read first:** [organization specification](../modules/organizations.md) §§4–5 and 10, [privacy](../11-privacy-security.md)
- **Scope:** in: scoped entity declarations and establishment metadata · out: verification, provider calls and claim rights
- **Areas:** existing: kernel CNPJ value and privacy policy · proposed: organization declarations and reviewed CPF validation
- **Steps:** Verify authoritative identifier rules when the task starts. Reuse CNPJ validation and cover masking, duplicates and declaration status.
- **Deliverable:** A scoped declaration API and privacy-preserving display.
- **Acceptance:** A valid declaration is shown as declared. It conveys no access and exposes no foreign declaration.
- **Negative cases:** Invalid identifier, guessed head office, cross-organization duplicate oracle or CPF leakage in logs or exports.
- **Validation:** V-API, V-DOM, V-SQL, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Use synthetic generated identifiers in tests. Review the permitted CPF purpose before real data.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.

### EMP-ORG-005 — Operating units
- **Meta:** module ORG · phase F1 · cut C1 · priority P0 · type API · size S · state TODO
- **Objective:** Manage operating units while preserving the active-unit and time-zone rules.
- **Rules and use cases:** BR-ORG-006, BR-ORG-007, BR-ORG-008, BR-ORG-009, BR-ORG-010, BR-ORG-012, UC-ORG-03
- **Depends on:** EMP-ORG-003, EMP-PLAT-015
- **Read first:** [organization specification](../modules/organizations.md) §§4–5, [architecture](../09-architecture.md)
- **Scope:** in: unit create, edit, archive and restore · out: rewriting historical reporting days
- **Areas:** existing: organization and time kernels · proposed: unit commands and queries
- **Steps:** Pin uniqueness, last-unit and in-use races. Test reporting dates before and after a zone revision.
- **Deliverable:** Scoped unit management commands with history preserved.
- **Acceptance:** The organization retains one active unit. Active names and codes are scoped and current zone changes apply prospectively.
- **Negative cases:** Concurrent last-unit archive, foreign establishment, invalid zone, stale revision or history rewrite.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Archiving retains historical identity. Disable admission without deleting records if rollback is required.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.

### EMP-ORG-006 — Cost centres
- **Meta:** module ORG · phase F1 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Manage stable cost-centre codes and preserve historical exports.
- **Rules and use cases:** BR-ORG-007, BR-ORG-008, BR-ORG-009, BR-ORG-012, UC-ORG-03
- **Depends on:** EMP-ORG-003
- **Read first:** [organization specification](../modules/organizations.md) §§4–5, [permission matrix](../07-permissions.md)
- **Scope:** in: scoped cost-centre lifecycle · out: budgeting or remapping old allocations
- **Areas:** existing: organization structure commands · proposed: cost-centre repository and API
- **Steps:** Pin code normalization and permanent uniqueness. Verify archive blocks new assignments and retains history.
- **Deliverable:** A scoped cost-centre API with stable codes.
- **Acceptance:** An archived code still identifies the same centre. Foreign organizations can use their own codes independently.
- **Negative cases:** Reused archived code, in-use archive, foreign scope and concurrent duplicate creation.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Use append-only schema changes and preserve export references.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.

### EMP-ORG-007 — Teams
- **Meta:** module ORG · phase F1 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Manage teams as an organizational structure with explicit authorization scope.
- **Rules and use cases:** BR-ORG-007, BR-ORG-008, BR-ORG-009, BR-ORG-011, BR-ORG-012, UC-ORG-03
- **Depends on:** EMP-ORG-003, EMP-IDN-002
- **Read first:** [organization specification](../modules/organizations.md) §§4–5, [permission matrix](../07-permissions.md)
- **Scope:** in: scoped teams and employee membership · out: implicit role grants
- **Areas:** existing: organization structures and employee records · proposed: team commands and queries
- **Steps:** Exercise same-organization member edits and active name uniqueness. Prove team assignment has no independent permission effect.
- **Deliverable:** Scoped team lifecycle and membership commands.
- **Acceptance:** Authorized changes affect team structure only. History and existing scoped roles remain explicit.
- **Negative cases:** Foreign employee, duplicate active name, archived team assignment and privilege inferred from membership.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Keep team changes separate from role assignment and delegation.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.

### EMP-ORG-008 — Organization settings and policy holder: time zone, units, approval defaults
- **Meta:** module ORG · phase F1 · cut C1 · priority P0 · type API · size S · state TODO
- **Objective:** Apply versioned organization settings from the correct effective revision.
- **Rules and use cases:** BR-ORG-010, BR-ORG-038, BR-ORG-039, BR-ORG-040, BR-ORG-041, UC-ORG-06, UC-ORG-07
- **Depends on:** EMP-ORG-003, EMP-PLAT-015
- **Read first:** [organization specification](../modules/organizations.md) §§4–5, [permission matrix](../07-permissions.md)
- **Scope:** in: settings revision, time zone, display units, policy defaults and contacts · out: changing past records
- **Areas:** existing: time and exact-value conventions · proposed: versioned organization settings
- **Steps:** Pin effective revision, display conversion and delegation separation. Test stale settings updates.
- **Deliverable:** An organization settings contract and API.
- **Acceptance:** An accepted change keeps its previous revision and affects later server intake. Display units preserve stored values.
- **Negative cases:** Lost update, silent unit rewrite, invalid zone or a substitute acquiring authority without a role.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Owning workflows define which policy keys they consume.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.

### EMP-ORG-009 — Suspension and reactivation with write-block semantics
- **Meta:** module ORG · phase F1 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Apply scoped suspension and reactivation while preserving allowed recovery and reads.
- **Rules and use cases:** BR-ORG-029, BR-ORG-030, BR-ORG-031, BR-ORG-032, UC-ORG-08
- **Depends on:** EMP-ORG-003
- **Read first:** [organization specification](../modules/organizations.md) §§4–5, [offline sync](../10-offline-sync.md)
- **Scope:** in: suspension authority, write gate and reactivation behavior · out: payment or production enablement
- **Areas:** existing: organization lifecycle, audit and operation intake · proposed: scoped suspension command and gate
- **Steps:** Pin a permission and intake matrix. Test state transitions racing with writes and queue replay.
- **Deliverable:** An audited suspension and reactivation path.
- **Acceptance:** Suspension blocks new writes. Existing authorized reads, exports and access-reducing actions follow the scoped exceptions.
- **Negative cases:** Unaudited staff action, role bypass, lost held operation or another organization suspended by mistake.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-SYNC · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Reactivation sends held facts to review under their existing rules.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.

### EMP-ORG-010 — Ownership transfer and the last-owner invariant
- **Meta:** module ORG · phase F1 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Transfer ownership without leaving the organization ownerless.
- **Rules and use cases:** BR-ORG-026, BR-ORG-034, BR-ORG-035, BR-AUTH-006, UC-ORG-09
- **Depends on:** EMP-ORG-003, EMP-AUTH-005
- **Read first:** [organization specification](../modules/organizations.md) §§4–5, [permission matrix](../07-permissions.md)
- **Scope:** in: proposal, acceptance and last-owner protections · out: back-office recovery or implicit owner assignment
- **Areas:** existing: membership and role model · proposed: transactional ownership transfer
- **Steps:** Test fresh proof on both sides, expiry and concurrent transfer or removal. Apply accepted roles atomically.
- **Deliverable:** A two-party, audited ownership transfer.
- **Acceptance:** The recipient has an active eligible account. An owner exists before, during and after the transfer.
- **Negative cases:** Managed account, stale membership, expired replay, competing transfer and last-owner removal.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Exceptional owner recovery requires the separate audited workflow.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.

### EMP-ORG-011 — Legal entity verification workflow
- **Meta:** module ORG · phase F7 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Record reviewed legal-entity verification with validity and evidence provenance.
- **Rules and use cases:** BR-ORG-016, BR-ORG-017, BR-ORG-018, BR-ORG-019, BR-ORG-020, UC-ORG-05
- **Depends on:** EMP-ORG-004, EMP-BACK-002
- **Read first:** [organization specification](../modules/organizations.md) §§4–5, [privacy](../11-privacy-security.md)
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
- **Read first:** [organization specification](../modules/organizations.md) §§4–5, [permission matrix](../07-permissions.md)
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
- **Read first:** [organization specification](../modules/organizations.md) §§4–5, [privacy](../11-privacy-security.md)
- **Scope:** in: open-items check, archive, export window and terminal closure · out: unreviewed retention and destructive production operation
- **Areas:** existing: lifecycle and reviewed retention controls · proposed: archive and close workflows
- **Steps:** Test blocking open work, scoped export, cancel window and terminal closure with isolated fixtures.
- **Deliverable:** A versioned closure workflow and recovery evidence.
- **Acceptance:** Archival gates writes. Terminal closure ends access and hands deletion to the reviewed retention process.
- **Negative cases:** Open work dropped, data deleted before export window, reopened terminal organization or deletion of a legal hold.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Reviewed retention and backup recovery govern disposal. Historical identifiers stay distinct.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.

### EMP-ORG-014 — Panel: onboarding wizard and organization settings
- **Meta:** module ORG · phase F1 · cut C1 · priority P0 · type WEB · size M · state TODO
- **Objective:** Expose organization onboarding and settings through the approved company panel.
- **Rules and use cases:** BR-ORG-025, BR-ORG-026, BR-ORG-027, BR-ORG-038, UC-ORG-01, UC-ORG-02, UC-ORG-06
- **Depends on:** EMP-ORG-003, EMP-ORG-005, EMP-ORG-008, EMP-PLAT-008
- **Read first:** [organization specification](../modules/organizations.md) §§5 and 11, [approved company panel](../evidence/f0-m0-decisions.md#company-web-surface)
- **Scope:** in: company setup wizard, settings, active state and error feedback · out: station portal and real-data use
- **Areas:** existing: business web scaffold · proposed: onboarding and settings views
- **Steps:** Exercise fresh login, idempotent submit, incomplete checklist and settings conflict through the company entry point.
- **Deliverable:** Company onboarding and settings UI with accessible states.
- **Acceptance:** The panel reflects persisted setup and settings states. Retried creation does not create a second organization.
- **Negative cases:** Guest sees private settings, stale revision hides conflict, inaccessible checklist or client claims activation early.
- **Validation:** V-WEB, V-AUTHZ, V-ISO · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** The API is authoritative. Use synthetic onboarding data in local validation.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.

### EMP-ORG-015 — Panel: units, cost centres and teams management
- **Meta:** module ORG · phase F1 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Expose unit, cost-centre and team management through the company panel.
- **Rules and use cases:** BR-ORG-006, BR-ORG-007, BR-ORG-008, BR-ORG-009, BR-ORG-011, UC-ORG-03
- **Depends on:** EMP-ORG-005, EMP-ORG-006, EMP-ORG-007, EMP-PLAT-008
- **Read first:** [organization specification](../modules/organizations.md) §§5 and 11, [approved company panel](../evidence/f0-m0-decisions.md#company-web-surface)
- **Scope:** in: structure lists and editing, archive, restore and conflict feedback · out: changing roles through team UI
- **Areas:** existing: business web scaffold and structure APIs · proposed: company structure screens
- **Steps:** Verify keyboard and error flows against the existing APIs. Switch organizations with pending form state.
- **Deliverable:** Accessible company structure screens and regression evidence.
- **Acceptance:** Authorized users see and edit only the selected organization's structure. Archived records remain identifiable.
- **Negative cases:** Foreign results after switching, invalid edit silently accepted or archived structure shown as active.
- **Validation:** V-WEB, V-AUTHZ, V-ISO · migration: rehearse additive changes when persistence is affected · rollback: preserve the existing state, remove only the new entry point and follow the scoped recovery evidence.
- **Risks and assumptions:** Structure UI never grants permission. The server enforces revision and scope.
- **Done when:** The stated behavior and negative cases pass the listed profiles on the affected entry points.
