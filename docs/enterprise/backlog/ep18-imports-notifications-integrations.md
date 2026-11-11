# EP18 — Imports, notifications and integrations

Status: **PLANNED**, 2026-11-10.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

Organization import and notification paths remain scoped. External integrations require their own recorded approvals and preserve manual fallback.

### EMP-IMPORT-001 — Import pipeline core: upload, parse, map, preview and validate rows
- **Meta:** module IMPORT · phase F2 · cut - · priority P1 · type API · size L · state TODO
- **Objective:** Preview and validate bounded organization imports before applying writes.
- **Rules and use cases:** REQ-17.1, BR-TEN-001, BR-PRIV-020, BR-AUD-001
- **Depends on:** EMP-PLAT-006, EMP-AUTH-003, EMP-PLAT-010
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: file admission, mapping, preview and row results · out: automatic writes or executable input
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Preview is scoped and repeatable. Only explicit confirmation can apply valid rows.
- **Acceptance:** Preview is scoped and repeatable. Only explicit confirmation can apply valid rows.
- **Negative cases:** Unsafe file, foreign mapping, permission loss, partial preview treated as committed.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-IMPORT-002 — Import employees
- **Meta:** module IMPORT · phase F2 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Import employees through the existing scoped register rules.
- **Rules and use cases:** REQ-17.1, BR-IDN-002, BR-IDN-006, BR-IDN-034
- **Depends on:** EMP-IMPORT-001, EMP-IDN-002
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: mapped employee fields and duplicate warnings · out: automatic credentials or merging records
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Confirmed rows apply employee rules and report exact outcomes.
- **Acceptance:** Confirmed rows apply employee rules and report exact outcomes.
- **Negative cases:** Foreign unit, duplicate number, excess personal fields or access granted automatically.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-IMPORT-003 — Import assets
- **Meta:** module IMPORT · phase F2 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Import scoped assets using existing asset identity and structure validation.
- **Rules and use cases:** REQ-17.1, BR-TEN-005, BR-ORG-012
- **Depends on:** EMP-IMPORT-001, EMP-FLEET-002
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: mapped assets and reference validation · out: moving personal vehicles or rewriting history
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Valid rows produce one authorized asset each with explicit reject reasons for other rows.
- **Acceptance:** Valid rows produce one authorized asset each with explicit reject reasons for other rows.
- **Negative cases:** Foreign structure, repeated row creates duplicate, or current import changes a historical assignment.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-IMPORT-004 — Import historical fuelings and meter readings
- **Meta:** module IMPORT · phase F3 · cut - · priority P2 · type API · size M · state TODO
- **Objective:** Import historical fuelings and readings under normal scope and period rules.
- **Rules and use cases:** REQ-17.1, BR-FUEL-022, BR-FUEL-024, BR-MONEY-004, BR-AMD-004
- **Depends on:** EMP-IMPORT-001, EMP-FUEL-002, EMP-METER-002
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: source and historical time provenance · out: personal records and silent approval
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Imported facts retain their source and exact inputs, and conflicts are routed through review.
- **Acceptance:** Imported facts retain their source and exact inputs, and conflicts are routed through review.
- **Negative cases:** Wrong unit, locked period bypass, foreign asset, duplicate cost or altered paid total.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-IMPORT-005 — Error report, correction and undo strategy
- **Meta:** module IMPORT · phase F2 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Provide scoped error reports and a safe correction or undo path.
- **Rules and use cases:** REQ-17.1, BR-AUD-001, BR-AMD-001, BR-PRIV-008
- **Depends on:** EMP-IMPORT-001
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: row error, replay and reviewed recovery · out: deleting approved history to undo a batch
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Errors identify the authorized row and reason. Corrections preserve committed facts and scope.
- **Acceptance:** Errors identify the authorized row and reason. Corrections preserve committed facts and scope.
- **Negative cases:** Sensitive content in reports, undo affects unrelated rows or retry duplicates previously accepted work.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-IMPORT-006 — Panel: import wizard
- **Meta:** module IMPORT · phase F2 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Expose mapping, preview and confirmation in the company panel.
- **Rules and use cases:** REQ-17.1, BR-AUTH-012, BR-TRUTH-001, BR-PRIV-004
- **Depends on:** EMP-IMPORT-001, EMP-IMPORT-005, EMP-PLAT-008
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: accessible scoped wizard and outcome summary · out: applying a preview without confirmation
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Each stage reflects server state and users can distinguish applied and rejected rows.
- **Acceptance:** Each stage reflects server state and users can distinguish applied and rejected rows.
- **Negative cases:** Wrong organization after switch, preview reported as applied or foreign error file.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-NOTIF-001 — Notification core: inbox, preferences, de-duplication, limits and history
- **Meta:** module NOTIF · phase F3 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Provide the scoped in-app notification inbox and retry-safe dispatch contract.
- **Rules and use cases:** REQ-17.2, EDR-032, BR-PRIV-003, BR-PRIV-008, BR-AUTH-014
- **Depends on:** EMP-PLAT-010, EMP-AUTH-003
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: per-member inbox, preferences, deduplication, bounds and delivery history · out: external provider activation or sharing private content
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** One eligible event generates its allowed scoped notice once. A report job can notify its authorized recipient with a panel link under current permissions.
- **Acceptance:** One eligible event generates its allowed scoped notice once. A report job can notify its authorized recipient with a panel link under current permissions.
- **Negative cases:** Wrong recipient or organization, private attachment in the notice, ignored preference, repeated dispatch or expired authority.
- **Validation:** V-API, V-SQL, V-JOB, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-NOTIF-002 — Delivery channels: Android local and push, and e-mail, with retries
- **Meta:** module NOTIF · phase F3 · cut - · priority P1 · type INT · size M · state TODO
- **Objective:** Add only approved notification channels with scoped retry and fallback.
- **Rules and use cases:** REQ-17.2, EDR-032, OQ-09, BR-PRIV-008
- **Depends on:** EMP-NOTIF-001, EMP-PLAT-012
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: local notification and separately approved push or email · out: enabling unreviewed delivery
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** In-app notices remain available and delivery respects preference, permission and authorized configuration.
- **Acceptance:** In-app notices remain available and delivery respects preference, permission and authorized configuration.
- **Negative cases:** Disabled provider contacted, repeated delivery after accepted result, credentials or sensitive payload logged.
- **Validation:** V-JOB, V-APP, V-IDEM, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-NOTIF-003 — Operational alert subscriptions
- **Meta:** module NOTIF · phase F3 · cut - · priority P2 · type API · size S · state TODO
- **Objective:** Manage scoped subscriptions to documented operational alerts.
- **Rules and use cases:** REQ-17.2, BR-AUTH-001, BR-PRIV-003, BR-TRUTH-006
- **Depends on:** EMP-NOTIF-001
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: authorized preference, rule scope and freshness · out: granting access through notification settings
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Alerts follow current permission and expose only bounded content for the selected context.
- **Acceptance:** Alerts follow current permission and expose only bounded content for the selected context.
- **Negative cases:** Foreign alert, revoked recipient, stale event described as current or disabling an unrelated subscription.
- **Validation:** V-API, V-SQL, V-JOB, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-NOTIF-004 — App and panel notification centres
- **Meta:** module NOTIF · phase F3 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Show scoped app and company-panel notification centres.
- **Rules and use cases:** REQ-17.2, BR-AUTH-012, BR-AUTH-013, BR-PRIV-016
- **Depends on:** EMP-NOTIF-001, EMP-PLAT-008, EMP-IDN-018
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: accessible notices, history and permission-aware deep links · out: cross-context notification views
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Context and read state remain explicit, and opening a link still requires current authorization.
- **Acceptance:** Context and read state remain explicit, and opening a link still requires current authorization.
- **Negative cases:** Wrong organization after switch, a revoked link opens content or private payload appears on lock screen.
- **Validation:** V-APP, V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-INTEG-001 — Integration registry: connections, secrets and health
- **Meta:** module INTEG · phase F8 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Register scoped integration connections and bounded health state.
- **Rules and use cases:** REQ-17.3, BR-PRIV-005, BR-PRIV-008, BR-AUD-001
- **Depends on:** EMP-PLAT-002, EMP-AUTH-003
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: connection metadata, secret references and health visibility · out: live provider setup or exposing credentials
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Only approved scoped consumers can use a connection, and health states do not reveal secret values.
- **Acceptance:** Only approved scoped consumers can use a connection, and health states do not reveal secret values.
- **Negative cases:** Wrong organization, credential in logs, unconfigured connection treated as active or unsafe retry.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-INTEG-002 — Telematics ingestion port
- **Meta:** module INTEG · phase F8 · cut - · priority P3 · type INT · size M · state TODO
- **Objective:** Define a reviewed telematics intake port with scoped ownership.
- **Rules and use cases:** REQ-17.3, BR-PRIV-006, BR-PRIV-008, BR-TEN-007
- **Depends on:** EMP-INTEG-001, EMP-GPS-004
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: vendor-neutral synthetic input and allowed provenance · out: unauthorized hardware collection or personal trips
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Only authorized organization input is admitted and source and quality remain visible.
- **Acceptance:** Only authorized organization input is admitted and source and quality remain visible.
- **Negative cases:** Unreviewed real location, foreign device, private coordinates in telemetry or duplicate segment.
- **Validation:** V-API, V-SQL, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-INTEG-003 — Fuel card statement import
- **Meta:** module INTEG · phase F8 · cut - · priority P2 · type INT · size M · state TODO
- **Objective:** Import approved fuel card statements for scoped reconciliation.
- **Rules and use cases:** REQ-17.3, EDR-013, BR-MONEY-001, BR-TEN-005
- **Depends on:** EMP-INTEG-001, EMP-FIN-005, EMP-IMPORT-001
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: exact statement input and existing cost linkage · out: initiating card transactions or creating duplicate costs
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A row maps to an existing fact or a review item under normal permission and amount rules.
- **Acceptance:** A row maps to an existing fact or a review item under normal permission and amount rules.
- **Negative cases:** Retry doubles cost, foreign account, unverified match settled or incorrect money basis.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-INTEG-004 — Authorised fiscal document retrieval with the organization's certificate
- **Meta:** module INTEG · phase F8 · cut - · priority P2 · type INT · size L · state TODO
- **Objective:** Retrieve fiscal evidence only through an approved organization-specific authority.
- **Rules and use cases:** REQ-10.6, REQ-17.3, BR-TRUTH-002, BR-PRIV-004, OQ-04
- **Depends on:** EMP-INTEG-001, EMP-GOV-011, EMP-DOC-006
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: reviewed certificate custody, scope and fiscal result · out: unrestricted lookups, unapproved real intake and provider activation
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A result records exactly what the authorized source returned, with reviewed access and retention.
- **Acceptance:** A result records exactly what the authorized source returned, with reviewed access and retention.
- **Negative cases:** Foreign certificate, lookup without consent or scope, structure treated as authorization or secret exposure.
- **Validation:** V-DOC, V-API, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-INTEG-005 — Optional server-side extraction provider
- **Meta:** module INTEG · phase F8 · cut - · priority P3 · type INT · size M · state TODO
- **Objective:** Offer server extraction only after explicit privacy and dependency review.
- **Rules and use cases:** REQ-10.3, REQ-17.3, BR-TRUTH-002, BR-PRIV-005, BR-PRIV-008
- **Depends on:** EMP-INTEG-001, EMP-DOC-009
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: approved extraction adapter and manual fallback · out: silent third-party transfer or document approval
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Extraction remains optional, records its basis and does not replace human review.
- **Acceptance:** Extraction remains optional, records its basis and does not replace human review.
- **Negative cases:** Unapproved provider, content leaked, low-confidence result silently accepted or fallback broken.
- **Validation:** V-DOC, V-API, V-JOB, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-INTEG-006 — Outbound webhooks for organization systems
- **Meta:** module INTEG · phase F8 · cut - · priority P2 · type INT · size M · state TODO
- **Objective:** Deliver approved scoped events through bounded outbound webhooks.
- **Rules and use cases:** REQ-17.3, BR-PRIV-003, BR-PRIV-008, BR-AUD-001
- **Depends on:** EMP-INTEG-001, EMP-PLAT-010
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: reviewed endpoint and allowlisted payload contract · out: sending private contents by default
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Only approved organization events and fields are sent under bounded retry and recorded outcomes.
- **Acceptance:** Only approved organization events and fields are sent under bounded retry and recorded outcomes.
- **Negative cases:** Foreign delivery, leaked credentials or content, unbounded attempts or duplicate effect claimed as exactly once.
- **Validation:** V-API, V-SQL, V-JOB, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-INTEG-007 — Accounting system connectors on top of the export layouts
- **Meta:** module INTEG · phase F8 · cut - · priority P3 · type INT · size M · state TODO
- **Objective:** Connect accounting consumers through approved export layouts.
- **Rules and use cases:** REQ-11.5, REQ-17.3, BR-RPT-002, BR-RPT-008, EDR-013
- **Depends on:** EMP-INTEG-001, EMP-FIN-006
- **Read first:** [traceability](../17-traceability.md), [architecture](../09-architecture.md), [privacy](../11-privacy-security.md) and [offline sync](../10-offline-sync.md)
- **Scope:** in: versioned exact export adapter and manual fallback · out: unapproved provider execution or new cost recognition
- **Areas:** existing: scoped storage, jobs, imports and private export conventions · proposed: dedicated organization capabilities and reviewed adapters
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Authorized export basis and exact amounts survive transport. Failure leaves the manual export available.
- **Acceptance:** Authorized export basis and exact amounts survive transport. Failure leaves the manual export available.
- **Negative cases:** Duplicate posting, foreign data, rounded amounts, missing permission or provider activation without review.
- **Validation:** V-API, V-JOB, V-AUTHZ, V-ISO, V-IDEM, V-MONEY, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [integration design](../modules/imports-notifications-integrations.md) records the module boundary. Existing manual paths and separate provider, legal and activation approvals apply.
- **Done when:** The stated result and denials pass the listed validation profiles.
