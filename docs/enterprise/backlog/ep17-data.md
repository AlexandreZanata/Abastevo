# EP17 — Abastevo Data

Status: **PLANNED**, 2026-11-10.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from [Data requirements](../17-traceability.md), [decisions](../16-decisions.md), [architecture](../09-architecture.md) and [privacy](../11-privacy-security.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

The [Data design](../modules/data.md) records the recovered dataset and key boundaries. Market products never ingest private organization data by default.

### EMP-DATA-001 — Dataset catalog and methodology records on top of the enterprise export module
- **Meta:** module DATA · phase F8 · cut - · priority P1 · type DOM · size M · state TODO
- **Objective:** Define datasets with source rights, coverage and methodology.
- **Rules and use cases:** EDR-025, BR-SRC-001, BR-SRC-005, BR-TRUTH-004, REQ-16.6
- **Depends on:** EMP-GOV-016
- **Read first:** [Data requirements](../17-traceability.md), [decisions](../16-decisions.md), [architecture](../09-architecture.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: approved catalog and immutable method references · out: market activation before rights review
- **Areas:** existing: enterpriseexport, scoped keys, jobs and private export ports · proposed: gated Data catalog and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Every offered dataset has a recorded provenance and current admission decision.
- **Acceptance:** Every offered dataset has a recorded provenance and current admission decision.
- **Negative cases:** Public availability treated as resale permission, source blending or private organization input.
- **Validation:** V-DOM, V-API, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Source-rights approval and legal and activation gates precede any market sale or private-data collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-DATA-002 — Organization API keys: issue, rotate, revoke and scopes
- **Meta:** module DATA · phase F8 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Issue, rotate and revoke scoped organization API keys.
- **Rules and use cases:** REQ-16.6, BR-TEN-003, BR-AUTH-010, BR-PRIV-008
- **Depends on:** EMP-ORG-003, EMP-AUTH-003
- **Read first:** [Data requirements](../17-traceability.md), [decisions](../16-decisions.md), [architecture](../09-architecture.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: current authority, bounded overlap and secret protection · out: keys granting administrative roles
- **Areas:** existing: enterpriseexport, scoped keys, jobs and private export ports · proposed: gated Data catalog and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A key is shown under approved issuance and later identified only by safe metadata. Revocation applies to the next request.
- **Acceptance:** A key is shown under approved issuance and later identified only by safe metadata. Revocation applies to the next request.
- **Negative cases:** Foreign key, revoked credential, secret in storage/logs or scope broadened by rotation.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Source-rights approval and legal and activation gates precede any market sale or private-data collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-DATA-003 — Metering, quotas and rate limits per key
- **Meta:** module DATA · phase F8 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Apply quotas and usage metering per authorized API scope.
- **Rules and use cases:** REQ-16.6, BR-PRIV-008, BR-AUTH-009
- **Depends on:** EMP-DATA-002, EMP-SUBS-009
- **Read first:** [Data requirements](../17-traceability.md), [decisions](../16-decisions.md), [architecture](../09-architecture.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: exact counters, period and rate controls · out: infinite admission or billing inferred from client data
- **Areas:** existing: enterpriseexport, scoped keys, jobs and private export ports · proposed: gated Data catalog and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Requests use current key and dataset permission and meter against the approved scope.
- **Acceptance:** Requests use current key and dataset permission and meter against the approved scope.
- **Negative cases:** Rotation bypasses quota, foreign usage, duplicate counting or unbounded labels.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Source-rights approval and legal and activation gates precede any market sale or private-data collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-DATA-004 — Regional reports and historical series endpoints
- **Meta:** module DATA · phase F8 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Serve admitted regional and historical data with explicit methodology.
- **Rules and use cases:** REQ-16.6, BR-SRC-001, BR-TRUTH-003, BR-TRUTH-004
- **Depends on:** EMP-DATA-001, EMP-DATA-002
- **Read first:** [Data requirements](../17-traceability.md), [decisions](../16-decisions.md), [architecture](../09-architecture.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: scoped pagination, coverage and source metadata · out: private-company rows
- **Areas:** existing: enterpriseexport, scoped keys, jobs and private export ports · proposed: gated Data catalog and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Pages share a defined basis and missing data is explicit.
- **Acceptance:** Pages share a defined basis and missing data is explicit.
- **Negative cases:** Foreign cursor, mixed revisions, fake zero or unlicensed dataset served.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Source-rights approval and legal and activation gates precede any market sale or private-data collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-DATA-005 — Station monitoring: watchlists and change notifications
- **Meta:** module DATA · phase F8 · cut - · priority P2 · type API · size M · state TODO
- **Objective:** Notify authorized watchlists of recorded station-data changes.
- **Rules and use cases:** REQ-16.6, BR-SRC-005, BR-PRIV-003, BR-TRUTH-006
- **Depends on:** EMP-DATA-004, EMP-NOTIF-001
- **Read first:** [Data requirements](../17-traceability.md), [decisions](../16-decisions.md), [architecture](../09-architecture.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: scoped watchlist and deduplicated admitted changes · out: private fleet monitoring
- **Areas:** existing: enterpriseexport, scoped keys, jobs and private export ports · proposed: gated Data catalog and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Each notice identifies its public source and change time within the permitted dataset scope.
- **Acceptance:** Each notice identifies its public source and change time within the permitted dataset scope.
- **Negative cases:** Foreign list, private data, duplicate alerts or old change represented as live.
- **Validation:** V-API, V-SQL, V-JOB, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Source-rights approval and legal and activation gates precede any market sale or private-data collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-DATA-006 — Market comparison and custom report requests
- **Meta:** module DATA · phase F8 · cut - · priority P2 · type JOB · size M · state TODO
- **Objective:** Support reviewed market comparisons and custom report requests.
- **Rules and use cases:** REQ-16.6, BR-SRC-005, BR-KPI-012, BR-TRUTH-004
- **Depends on:** EMP-DATA-004, EMP-RPT-001
- **Read first:** [Data requirements](../17-traceability.md), [decisions](../16-decisions.md), [architecture](../09-architecture.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: explicit scope, method and admitted data · out: assumed savings or arbitrary private-data queries
- **Areas:** existing: enterpriseexport, scoped keys, jobs and private export ports · proposed: gated Data catalog and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The output states source, coverage, method and limits and uses the authorized export path.
- **Acceptance:** The output states source, coverage, method and limits and uses the authorized export path.
- **Negative cases:** Unlicensed source, incomparable reference, privacy threshold bypass or incomplete result represented as complete.
- **Validation:** V-API, V-JOB, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Source-rights approval and legal and activation gates precede any market sale or private-data collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-DATA-007 — Privacy guard: no private enterprise data and aggregation thresholds
- **Meta:** module DATA · phase F8 · cut - · priority P1 · type TEST · size S · state TODO
- **Objective:** Prove datasets exclude private enterprise records and respect admitted aggregation.
- **Rules and use cases:** BR-SRC-005, BR-TEN-010, REQ-16.7, VAL-36
- **Depends on:** EMP-DATA-004
- **Read first:** [Data requirements](../17-traceability.md), [decisions](../16-decisions.md), [architecture](../09-architecture.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: source allowlist and synthetic leak probes · out: de-identification assumed sufficient without review
- **Areas:** existing: enterpriseexport, scoped keys, jobs and private export ports · proposed: gated Data catalog and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Market data includes only admitted sources and any approved threshold is enforced consistently.
- **Acceptance:** Market data includes only admitted sources and any approved threshold is enforced consistently.
- **Negative cases:** Employee, contract, receipt or location in output, tiny cohort leak or private row in a job.
- **Validation:** V-API, V-SQL, V-JOB, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Source-rights approval and legal and activation gates precede any market sale or private-data collection.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-DATA-008 — Portal: keys, usage and documentation
- **Meta:** module DATA · phase F8 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Expose authorized API keys, usage and dataset documentation in the company panel.
- **Rules and use cases:** REQ-16.6, BR-AUTH-010, BR-PRIV-008, BR-TRUTH-004
- **Depends on:** EMP-DATA-002, EMP-DATA-003, EMP-PLAT-008
- **Read first:** [Data requirements](../17-traceability.md), [decisions](../16-decisions.md), [architecture](../09-architecture.md) and [privacy](../11-privacy-security.md)
- **Scope:** in: scoped safe metadata and explicit method docs · out: redisplaying key secrets
- **Areas:** existing: enterpriseexport, scoped keys, jobs and private export ports · proposed: gated Data catalog and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The viewer sees only its organization's key metadata and admitted dataset permissions.
- **Acceptance:** The viewer sees only its organization's key metadata and admitted dataset permissions.
- **Negative cases:** Foreign usage, secret in navigation, stale revocation or a catalog entry implying rights approval.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** Source-rights approval and legal and activation gates precede any market sale or private-data collection.
- **Done when:** The stated result and denials pass the listed validation profiles.
