# EP01 — Platform foundations and privacy

Status: **F1 IN_PROGRESS**, 2026-11-11.

Task metadata follows the existing [generated index](INDEX.md). The task
definitions reconstruct the missing backlog from the approved contract,
architecture, privacy rules and validation plan. The owning source phases
must produce implementation evidence after the applicable entry gates.
Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the [sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

### EMP-PLAT-001 — Tenancy kernel: OrgID, OrgScope, the scoped-repository convention and the module registration checklist
- **Meta:** module PLAT · phase F1 · cut C1 · priority P0 · type DOM · size M · state LOCAL_DONE
- **Objective:** Make resolved organization scope explicit in the Go domain and repository ports.
- **Rules and use cases:** BR-TEN-001, BR-TEN-003, BR-TEN-007, BR-TEN-010
- **Depends on:** EMP-GOV-001
- **Read first:** [architecture](../09-architecture.md), [ADR-025](../../adr/025-private-abastevo-empresas-organization-boundary.md)
- **Scope:** in: organization ID, resolved scope and registration checklist · out: schema, HTTP and module-specific behavior
- **Areas:** existing: backend/internal/modules/kernel/ and composition roots · proposed: organization scope types and repository convention
- **Steps:** Pin valid and invalid scope cases. Define typed scope and verify its use in the registration checklist.
- **Deliverable:** A small Go scope contract and module checklist.
- **Acceptance:** An organization repository port takes resolved scope. Public catalog and personal ports stay independent.
- **Negative cases:** Zero or absent scope, treating a client ID as authority, or reusing organization scope for a public lookup.
- **Validation:** V-DOM · migration: none · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** Scope resolution is separate from membership, role and capability decisions.
- **Done when:** The types and checklist pass the domain and dependency checks.

[Local implementation and test evidence](../evidence/f1-plat001-tenancy-kernel.md).

### EMP-PLAT-002 — Tenant schema conventions, row-level-security template and two-organization isolation harness
- **Meta:** module PLAT · phase F1 · cut C1 · priority P0 · type DB · size M · state LOCAL_DONE
- **Objective:** Establish the private database isolation baseline and shared PostGIS harness.
- **Rules and use cases:** EDR-005, BR-TEN-005, BR-TEN-006, BR-PRIV-001, BR-PRIV-002
- **Depends on:** EMP-PLAT-001
- **Read first:** [architecture](../09-architecture.md), [privacy and security](../11-privacy-security.md), [validation scenarios](../15-validation.md)
- **Scope:** in: organization keys, scoped foreign keys, row-level security and transaction scope · out: per-feature schema
- **Areas:** existing: backend/db/migrations/, sqlc and PostGIS integration fixtures · proposed: organization storage conventions and two-organization fixture
- **Steps:** Write negative cases using the ordinary application role. Add the schema conventions and harness with synthetic organizations.
- **Deliverable:** A migration template and a real-PostGIS isolation harness.
- **Acceptance:** A foreign reference is rejected. Rows are inaccessible without scope or under a foreign scope. A reused connection cannot inherit scope.
- **Negative cases:** Missing explicit predicate, pooled scope reuse, privileged role leakage, concurrent writes and cross-organization references.
- **Validation:** V-SQL, V-ISO, V-IDEM · migration: forward-only rehearsal with the previous image · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** Routine requests and jobs use roles subject to row-level policies. Migration authority is separate.
- **Done when:** The harness proves the database boundary under ordinary roles and additive migration recovery.

[Local isolation and migration evidence](../evidence/f1-plat002-database-isolation.md).

### EMP-PLAT-003 — Organization audit trail: append-only, per organization, following the admin audit pattern
- **Meta:** module PLAT · phase F1 · cut C1 · priority P0 · type DB · size M · state LOCAL_DONE
- **Objective:** Record organization changes in an atomic, append-only audit trail.
- **Rules and use cases:** BR-AUD-001, BR-AUD-002, BR-AUD-004, BR-AUD-005, BR-AUD-006
- **Depends on:** EMP-PLAT-002
- **Read first:** [shared business rules](../04-business-rules.md), [architecture](../09-architecture.md), [privacy and security](../11-privacy-security.md)
- **Scope:** in: audit writes, authorized scoped reads and actor attribution · out: access logs for sensitive reads
- **Areas:** existing: admin audit precedent and sqlc · proposed: organization audit records
- **Steps:** Pin transaction and replay cases. Append the audit entry with the state change and scope reads by permission.
- **Deliverable:** Organization audit port, storage and integration evidence.
- **Acceptance:** Each committed change has one bounded event. Failed changes leave no event. A replay adds no duplicate.
- **Negative cases:** Cross-organization read, unaudited mutation, double event, data payload or credential in audit.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-IDEM, V-PRIV · migration: additive with previous image · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** Audit content holds scoped references and bounded reasons. The retention policy governs lifecycle.
- **Done when:** PostGIS proves atomic, scoped and replay-safe audit behavior.

[Local audit and migration evidence](../evidence/f1-plat003-organization-audit.md).

### EMP-PLAT-004 — Organization API conventions: path scope, session and device proof, error catalog, pagination, OpenAPI section, route-versus-contract test
- **Meta:** module PLAT · phase F1 · cut C1 · priority P0 · type API · size M · state LOCAL_DONE
- **Objective:** Define the API contract for organization-scoped routes.
- **Rules and use cases:** EDR-006, BR-TEN-003, BR-TEN-006, BR-PRIV-003
- **Depends on:** EMP-PLAT-001
- **Read first:** [architecture](../09-architecture.md), [permissions](../07-permissions.md), [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: path scope, session and signed-device boundary, pagination and errors · out: feature handlers
- **Areas:** existing: API router, account/session proof and OpenAPI · proposed: organization API conventions
- **Steps:** Write contract vectors for valid and foreign scope, cursor binding and proof errors. Pin route and OpenAPI agreement.
- **Deliverable:** Organization API contract and registration test.
- **Acceptance:** Handlers resolve current authority. Foreign IDs and cursors reveal no existence hint and private responses use no-store.
- **Negative cases:** ID-only authorization, wrong origin, expired proof, foreign cursor, stale or malformed body and route drift.
- **Validation:** V-API, V-AUTHZ, V-ISO, V-PRIV · migration: none · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** Business sessions and Android device proof are separate entry points into the same authorization boundary.
- **Done when:** The contract, route and targeted negative tests agree.

[Local HTTP, contract and proof evidence](../evidence/f1-plat004-organization-api.md).

### EMP-PLAT-005 — Per-organization capability flags for activation and rollback
- **Meta:** module PLAT · phase F1 · cut C1 · priority P0 · type DB · size S · state LOCAL_DONE
- **Objective:** Control organization capabilities without destroying existing records.
- **Rules and use cases:** BR-AUTH-009, BR-TEN-003, EDR-003
- **Depends on:** EMP-PLAT-002
- **Read first:** [architecture](../09-architecture.md), [validation scenarios](../15-validation.md)
- **Scope:** in: global and per-organization feature guards and rollback · out: billing or production rollout
- **Areas:** existing: disabled-by-default config conventions · proposed: organization capability state and audit
- **Steps:** Define the denial matrix. Persist scoped changes and verify that disabling a capability blocks new writes.
- **Deliverable:** Capability flags and an audited disable/enable path.
- **Acceptance:** An off flag blocks new commands and preserves authorized reads and export. Other organizations are unaffected.
- **Negative cases:** Missing capability grants access, a stale client bypasses the gate, or rollback deletes records.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-IDEM · migration: additive with flag off · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** An account tier or payment never grants membership. Keep existing release gates.
- **Done when:** The isolation and disable/re-enable matrix passes on PostGIS.

[Local capability, audit and rollback evidence](../evidence/f1-plat005-capability-flags.md).

### EMP-PLAT-006 — Private storage namespace for enterprise documents with short-lived access
- **Meta:** module PLAT · phase F1 · cut C1 · priority P0 · type INT · size M · state LOCAL_DONE
- **Objective:** Provide scoped private object storage access for synthetic C1 documents.
- **Rules and use cases:** BR-PRIV-004, BR-PRIV-008, BR-PRIV-020, BR-TEN-007, OQ-04
- **Depends on:** EMP-GOV-002, EMP-PLAT-002
- **Read first:** [privacy and security](../11-privacy-security.md), [architecture](../09-architecture.md), [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: private object namespace, bounded uploads and audited member-bound access · out: real-receipt intake and public objects
- **Areas:** existing: private object storage and ingress conventions · proposed: organization storage port and access capability
- **Steps:** Verify object identity and scope at intake and retrieval. Test expiry, revocation and storage failure before enabling the synthetic path.
- **Deliverable:** Private storage port with bounded organization access.
- **Acceptance:** A permitted member opens only the named document through an audited short-lived API capability. Original bytes stay private.
- **Negative cases:** Wrong organization or member, revoked or expired capability, unsupported type, storage failure and data in logs.
- **Validation:** V-ISO, V-AUTHZ, V-PRIV · migration: document schema follows its owning task · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** Real intake and retention require a reviewed category-specific policy. Community photo rules continue.
- **Done when:** Negative access, upload limits and private-object checks pass on isolated fixtures.

[Local private original, access and failure evidence](../evidence/f1-plat006-private-document-storage.md).

### EMP-PLAT-007 — Account web session for the business web: cookie, CSRF, origin check and fresh authentication
- **Meta:** module PLAT · phase F1 · cut C1 · priority P0 · type API · size M · state LOCAL_DONE
- **Objective:** Issue and revoke the company panel account session independently of admin.
- **Rules and use cases:** EDR-006, EDR-019, BR-TEN-003, BR-PRIV-003
- **Depends on:** EMP-GOV-003, EMP-PLAT-004
- **Read first:** [F0 decisions](../evidence/f0-m0-decisions.md), [architecture](../09-architecture.md)
- **Scope:** in: fresh authentication, business cookie, CSRF, exact origin and session expiry · out: staff session or Postos
- **Areas:** existing: account auth and admin session regression patterns · proposed: company session store and middleware
- **Steps:** Define the session and mutation matrices. Wire the company session through its own middleware and verify revocation.
- **Deliverable:** A scoped company web session API.
- **Acceptance:** The dedicated host-only cookie uses the approved attributes. Login, logout and expiry stay independent of admin.
- **Negative cases:** Cross-origin or CSRF failure, fresh-auth omission, admin cookie confusion, stale sessions or private caching.
- **Validation:** V-API, V-AUTHZ, V-ISO, V-PRIV · migration: additive session storage with old API compatibility · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** The host-wide cookie path requires route and middleware separation. Keep the admin network fence.
- **Done when:** Company and admin session isolation and mutation checks pass through their entry points.

[Local company-session, isolation and migration evidence](../evidence/f1-plat007-company-web-session.md).

### EMP-PLAT-008 — Business web scaffold: build, routing, auth guard, organization switcher, design tokens, checks
- **Meta:** module PLAT · phase F1 · cut C1 · priority P0 · type WEB · size M · state LOCAL_DONE
- **Objective:** Provide a static company panel shell for approved organization workflows.
- **Rules and use cases:** EDR-019, OQ-05, BR-TEN-003, BR-PRIV-003
- **Depends on:** EMP-GOV-003, EMP-PLAT-007
- **Read first:** [F0 decisions](../evidence/f0-m0-decisions.md), [architecture](../09-architecture.md)
- **Scope:** in: approved /empresas/ entry, company session guard and organization switcher · out: Postos or private admin UI
- **Areas:** existing: admin-web build precedent and Caddy · proposed: business-web/
- **Steps:** Use the existing static build pattern. Exercise route entry, accessible switching and empty, loading and denial states.
- **Deliverable:** Static company panel scaffold and build checks.
- **Acceptance:** Only the authorized organization is selected and rendered. Private fetches are no-store and follow the company session.
- **Negative cases:** Mixed organization state, hidden navigation used as authorization, privileged cookie reuse and keyboard-inaccessible selection.
- **Validation:** V-WEB, V-AUTHZ, V-ISO · migration: none · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** The API remains authoritative. A route scaffold is not deployment approval.
- **Done when:** The shell passes build, route, accessibility and authorization checks.

[Local build, browser, isolation and Caddy evidence](../evidence/f1-plat008-business-web-shell.md).

### EMP-PLAT-009 — Exact-value kernel for organization records: reuse centavo, milli-unit and milli-BRL types, add reconciliation and allocation vectors
- **Meta:** module PLAT · phase F1 · cut C1 · priority P0 · type DOM · size S · state LOCAL_DONE
- **Objective:** Reuse the project exact-value conventions for organization finance and fuel.
- **Rules and use cases:** BR-MONEY-001, BR-MONEY-003, BR-MONEY-004, BR-MONEY-005, BR-MONEY-007, BR-MONEY-011
- **Depends on:** EMP-GOV-001
- **Read first:** [shared business rules](../04-business-rules.md), [validation scenarios](../15-validation.md)
- **Scope:** in: exact units, consistency, allocation and shared vectors · out: pricing, billing and financial workflows
- **Areas:** existing: financial-v1 value types and Premium vectors · proposed: enterprise golden vectors
- **Steps:** Write shared vectors for known totals, rounding boundaries, overflow and exact allocation. Reuse existing representations.
- **Deliverable:** Shared Go and Kotlin value rules backed by the same vectors.
- **Acceptance:** Allocations sum exactly to the original amount. Reconciliation preserves the paid total and unit semantics.
- **Negative cases:** Float rounding, overflow, invalid or unknown values treated as zero, or recomputation overwriting a paid total.
- **Validation:** V-DOM, V-MONEY · migration: none · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** Original precision and the recorded amount remain authoritative.
- **Done when:** Both languages pass the same money and unit vectors.

[Local exact-value and shared Go/Kotlin evidence](../evidence/f1-plat009-exact-values.md).

### EMP-PLAT-010 — Transactional enterprise events on the existing job queue
- **Meta:** module PLAT · phase F1 · cut C1 · priority P0 · type DB · size S · state LOCAL_DONE
- **Objective:** Publish organization work through the existing transaction and job queue.
- **Rules and use cases:** BR-PRIV-003, BR-AUD-001, BR-OFFL-004
- **Depends on:** EMP-PLAT-002
- **Read first:** [architecture](../09-architecture.md), [offline sync](../10-offline-sync.md)
- **Scope:** in: transactional jobs, bounded retry and handler deduplication · out: new queue infrastructure
- **Areas:** existing: PostgreSQL job queue, worker and transaction conventions · proposed: scoped enterprise event registration
- **Steps:** Write real-PostGIS rollback and replay tests. Commit job intent with the owning change and deduplicate handler effects.
- **Deliverable:** Scoped event registration and a tested transaction boundary.
- **Acceptance:** Rolled-back writes enqueue nothing. Replayed jobs converge under the original scope.
- **Negative cases:** Duplicate delivery, lease loss, concurrent retry, foreign scope and sensitive payload in the event.
- **Validation:** V-SQL, V-ISO, V-IDEM, V-JOB, V-PRIV · migration: additive if required by the owning task · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** Queue delivery is at least once. Avoid external calls while holding transaction locks.
- **Done when:** PostGIS proves rollback and safe retry using the existing queue.

[Local transactional publication, lease fencing and recovery evidence](../evidence/f1-plat010-transactional-events.md).

### EMP-PLAT-011 — Android enterprise foundation: packages, local store partitioned by account, origin and organization, DI
- **Meta:** module PLAT · phase F1 · cut C1 · priority P0 · type APP · size M · state LOCAL_DONE
- **Objective:** Establish the Android enterprise foundation and ownership of local work.
- **Rules and use cases:** OQ-06, BR-TEN-004, BR-PRIV-016, BR-PRIV-017
- **Depends on:** EMP-GOV-008
- **Read first:** [F0 decisions](../evidence/f0-m0-decisions.md), [architecture](../09-architecture.md), [privacy and security](../11-privacy-security.md)
- **Scope:** in: module wiring, partition key and local storage boundary · out: enterprise screens and feature handlers
- **Areas:** existing: com.anpfuel modules, Room and owner/origin patterns · proposed: enterprise packages and DI
- **Steps:** Pin context-switch and persistence cases. Wire the enterprise storage and queue ports to an explicit partition.
- **Deliverable:** An Android foundation with partitioned state and no cross-context access.
- **Acceptance:** The account, origin and organization present at creation bind stored records and queued work through process death.
- **Negative cases:** Personal table leakage, wrong origin, stale DI scope and a retry retagged to the current organization.
- **Validation:** V-APP, V-ISO, V-SYNC, V-PRIV · migration: test empty and existing personal stores · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** Encryption and revocation behavior are completed and proven by the protected-store task.
- **Done when:** Affected Android consumers compile and partition behavior passes targeted tests.

[Local ownership, Room/SQLite persistence and DI evidence](../evidence/f1-plat011-android-foundation.md).

### EMP-PLAT-012 — Transactional e-mail port and adapter for enterprise notices
- **Meta:** module PLAT · phase F1 · cut - · priority P1 · type INT · size S · state TODO
- **Objective:** Add transactional email only if the delivery decision selects it.
- **Rules and use cases:** EDR-032, OQ-09, BR-PRIV-005, BR-PRIV-008
- **Depends on:** EMP-GOV-014, EMP-PLAT-004
- **Read first:** [architecture](../09-architecture.md), [privacy and security](../11-privacy-security.md)
- **Scope:** in: approved notification port, bounded retries and scoped delivery · out: unapproved provider or production sends
- **Areas:** existing: job queue and planned in-app path · proposed: transactional email port and reviewed adapter
- **Steps:** Confirm the approved channel and review provider and dependency scope. Test in-app fallback and retries with synthetic fixtures.
- **Deliverable:** A delivery port and approved adapter behind its own off flag.
- **Acceptance:** The in-app path works independently. Retries are bounded and an accepted send is not duplicated.
- **Negative cases:** Wrong recipient or organization, credential in logs, disabled provider sends, retry storm and delivery outage.
- **Validation:** V-JOB, V-IDEM, V-PRIV · migration: owning notification storage remains additive · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** A channel approval is separate from credentials, provider setup and production use.
- **Done when:** The authorized adapter and in-app fallback pass scoped delivery tests.

### EMP-PLAT-013 — Enterprise telemetry: per-route metrics, redaction rules and organization-safe logs
- **Meta:** module PLAT · phase F1 · cut C1 · priority P0 · type INT · size S · state LOCAL_DONE
- **Objective:** Add bounded telemetry without exposing organization data.
- **Rules and use cases:** BR-PRIV-003, BR-PRIV-008, BR-AUD-004
- **Depends on:** EMP-PLAT-004
- **Read first:** [architecture](../09-architecture.md), [privacy and security](../11-privacy-security.md)
- **Scope:** in: route metrics, bounded labels and redaction at telemetry surfaces · out: collecting private payloads
- **Areas:** existing: slog and monitoring evaluator · proposed: organization route metrics
- **Steps:** Inventory sinks and seed synthetic sensitive values. Verify logs, labels and traces preserve only permitted fields.
- **Deliverable:** Telemetry conventions and negative regression checks.
- **Acceptance:** Counts and durations use bounded route and result labels. Private content stays out of telemetry.
- **Negative cases:** Token, coordinate, document bytes or organization identifier used as a metric label.
- **Validation:** V-PRIV, V-ISO · migration: none · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** Keep audit authorization and production metrics separate.
- **Done when:** All affected sinks pass the seeded redaction checks.

[Local telemetry, redaction and real-process evidence](../evidence/f1-plat013-private-telemetry.md).

### EMP-PLAT-014 — Organization rate limits per account, device and organization on the existing limiter
- **Meta:** module PLAT · phase F1 · cut - · priority P1 · type API · size S · state LOCAL_DONE
- **Objective:** Bound organization request usage through existing rate limiting.
- **Rules and use cases:** BR-TEN-003, BR-PRIV-003, BR-PRIV-008
- **Depends on:** EMP-PLAT-004
- **Read first:** [architecture](../09-architecture.md), [permissions](../07-permissions.md)
- **Scope:** in: account, device and organization scopes on the existing limiter · out: new service or invented global quotas
- **Areas:** existing: API request limiters and response catalog · proposed: organization-scoped limiter keys
- **Steps:** Confirm route classes and measured limits. Verify scope and failure behavior at the normal request entry point.
- **Deliverable:** Scoped limiter wiring and acceptance tests.
- **Acceptance:** A scoped limit is enforced without exposing private identifiers or affecting another organization.
- **Negative cases:** Rotating device scope bypasses the organization limit, a foreign ID consumes another quota, or limiter outage admits unintended work.
- **Validation:** V-API, V-ISO, V-PRIV · migration: none unless required by the reviewed limiter path · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** Limits require a recorded rationale and fit the existing admission path.
- **Done when:** Boundary, concurrent and negative quota cases pass.

[Local scoped admission and rollback evidence](../evidence/f1-plat014-scoped-rate-admission.md).

### EMP-PLAT-015 — Time kernel: instants, organization and unit time zones, reporting-day attribution
- **Meta:** module PLAT · phase F1 · cut C1 · priority P0 · type DOM · size S · state LOCAL_DONE
- **Objective:** Preserve declared operation time and compute reporting days consistently.
- **Rules and use cases:** BR-TIME-001, BR-TIME-002, BR-TIME-003, BR-TIME-004, BR-TIME-006
- **Depends on:** EMP-GOV-001
- **Read first:** [shared business rules](../04-business-rules.md), [validation scenarios](../15-validation.md)
- **Scope:** in: UTC instants, IANA time zone and reporting-day attribution · out: labour calculations or rewriting device time
- **Areas:** existing: time conventions and contract vector suites · proposed: enterprise time vectors
- **Steps:** Pin time-zone boundary and invalid-offset cases. Separate device time from server authority time.
- **Deliverable:** A tested organization time kernel.
- **Acceptance:** Records use the correct unit or organization reporting day. Authorization always uses server time.
- **Negative cases:** Invalid zone, boundary instant, clock skew and a device time incorrectly changing access authority.
- **Validation:** V-DOM · migration: none · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** Time-zone provenance is retained for historical reporting.
- **Done when:** Boundary and zone vectors pass with the same semantics in affected consumers.

[Local UTC, reporting-day and clock-skew evidence](../evidence/f1-plat015-operation-time.md).

### EMP-PLAT-016 — Amendment kernel: append-only corrections for approved records
- **Meta:** module PLAT · phase F3 · cut - · priority P1 · type DOM · size S · state TODO
- **Objective:** Preserve approved records through traceable corrections.
- **Rules and use cases:** BR-AMD-001, BR-AMD-002, BR-AMD-003, BR-AMD-004, BR-AMD-005
- **Depends on:** EMP-PLAT-003
- **Read first:** [shared business rules](../04-business-rules.md), [validation scenarios](../15-validation.md)
- **Scope:** in: amendment and void rules, revision and history · out: module-specific persistence and recalculation jobs
- **Areas:** existing: revision and audit conventions · proposed: shared amendment domain
- **Steps:** Write vectors for approved and locked records. Keep the effective revision until a reviewed correction succeeds.
- **Deliverable:** A pure shared amendment contract and tests.
- **Acceptance:** A correction names its prior revision and approval path. Locked periods and voids preserve history.
- **Negative cases:** In-place update, stale revision, lost original or unauthorized period reopen.
- **Validation:** V-DOM, V-IDEM · migration: none · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** Each consuming module owns its derived-data invalidation and transactional application.
- **Done when:** The domain proves correction, conflict and locked-period invariants.

### EMP-PRIV-001 — Enterprise data inventory and retention schedule enforcement
- **Meta:** module PRIV · phase F6 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Enforce approved retention by organization data category.
- **Rules and use cases:** BR-PRIV-009, BR-PRIV-010, BR-PRIV-011, BR-PRIV-012, LV-04, LV-11
- **Depends on:** EMP-PLAT-010, EMP-GOV-005
- **Read first:** [privacy and security](../11-privacy-security.md), [F0 decisions](../evidence/f0-m0-decisions.md), [validation scenarios](../15-validation.md)
- **Scope:** in: inventory, recorded policies, read-time expiry and cleanup · out: choosing retention without legal review
- **Areas:** existing: privacy cleanup and job patterns · proposed: organization inventory and policy enforcement
- **Steps:** Register categories and reviewed expiry rules. Exercise overdue, held and failed cleanup paths on scoped fixtures.
- **Deliverable:** Inventory and tested retention enforcement for approved categories.
- **Acceptance:** Expired content is inaccessible even during an outage. Holds and retry outcomes are scoped and audited.
- **Negative cases:** Unapproved default, expired content served, foreign hold, storage failure or duplicate cleanup.
- **Validation:** V-SQL, V-JOB, V-PRIV, V-ISO · migration: append-only with recovery evidence · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** No real receipt category activates without its legal policy.
- **Done when:** Expiry, legal hold, retry and recovery pass with controlled time and real storage boundaries.

### EMP-PRIV-002 — Data-subject request routing between the organization and the platform
- **Meta:** module PRIV · phase F6 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Route employee data requests to the responsible organization or platform flow.
- **Rules and use cases:** BR-PRIV-013, BR-PRIV-014, UC-PRIV-01, LV-02
- **Depends on:** EMP-IDN-003, EMP-GOV-005
- **Read first:** [privacy and security](../11-privacy-security.md), [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: scoped request intake, operator handoff and outcomes · out: unapproved controller assignment or unrelated personal data
- **Areas:** existing: account/privacy interfaces · proposed: organization rights request tracking and export ports
- **Steps:** Use the reviewed role and purpose inventory to map requests. Test scoped handling and restricted items.
- **Deliverable:** An auditable request workflow that selects the correct responsible context.
- **Acceptance:** A response covers only authorized data in the named context and records any applicable restriction.
- **Negative cases:** Foreign organization records, access by ordinary staff, duplicate outcomes or improper deletion of an organization record.
- **Validation:** V-API, V-AUTHZ, V-ISO, V-PRIV · migration: additive with scoped request-state recovery · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** The legal review governs production responsibilities and policy.
- **Done when:** Routing, scoped evidence and negative access pass before activation.

### EMP-PRIV-003 — Device-side protection: encrypted store, logout cleanup and shared-device rules
- **Meta:** module PRIV · phase F1 · cut C1 · priority P0 · type APP · size M · state IN_PROGRESS
- **Objective:** Protect Android organization data and handle session loss safely.
- **Rules and use cases:** BR-PRIV-016, BR-PRIV-017, BR-PRIV-018, BR-PRIV-019, UC-PRIV-03
- **Depends on:** EMP-PLAT-011
- **Read first:** [F0 decisions](../evidence/f0-m0-decisions.md), [privacy and security](../11-privacy-security.md), [validation scenarios](../15-validation.md)
- **Scope:** in: encrypted private store, partition lock, backup exclusion and revocation · out: instant offline remote wipe
- **Areas:** existing: Keystore adapters and backup/extraction rules · proposed: protected enterprise store
- **Steps:** Verify the protection covers journals, files and queue. Exercise restore, logout, key loss, revocation and context switching.
- **Deliverable:** Keystore-backed storage with scoped logout and lost-device evidence.
- **Acceptance:** Another account, origin or organization cannot open the partition. Server revocation blocks access and local cleanup follows the approved rules.
- **Negative cases:** Plaintext in a journal or backup, clock rollback, stale sessions, wrong partition and cleanup failure.
- **Validation:** V-APP, V-PRIV, V-ISO, V-SYNC · migration: preserve personal stores and test failed upgrade recovery · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** Any new storage dependency needs a separate recorded review.
- **Done when:** Device and persistence tests prove isolation and revocation behavior.

[Protected-store source and pending native acceptance](../evidence/f1-priv003-protected-device-store.md).
Host source checks are separate from actual Keystore, unlock, backup/transfer,
reboot and restore evidence. Production persistence remains unavailable.

### EMP-PRIV-004 — Backup, restore and deletion-ledger replay for enterprise data
- **Meta:** module PRIV · phase F6 · cut - · priority P1 · type OPS · size M · state TODO
- **Objective:** Prevent deleted or expired enterprise records from reappearing after restore.
- **Rules and use cases:** BR-PRIV-009, BR-PRIV-011, UC-PRIV-04
- **Depends on:** EMP-PLAT-002
- **Read first:** [privacy and security](../11-privacy-security.md), [validation scenarios](../15-validation.md)
- **Scope:** in: restore gating, deletion-ledger replay and scope verification · out: production restore until separately authorized
- **Areas:** existing: backup and privacy erasure replay · proposed: organization restore integration and rehearsal
- **Steps:** Restore an isolated populated snapshot. Replay the external deletion record before reads and verify failure keeps service closed.
- **Deliverable:** A tested organization restore procedure with replay receipts.
- **Acceptance:** Previously erased data stays inaccessible after restore and repeated replay converges.
- **Negative cases:** Missing ledger, partially restored bytes, foreign organization purge, replay interruption and stale read admission.
- **Validation:** V-SQL, V-PRIV, V-JOB, V-ISO · migration: test forward restore and previous image compatibility · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** The privacy ledger must survive independently of the restored snapshot.
- **Done when:** An isolated restore proves replay, failure recovery and access gating.

### EMP-PRIV-005 — Access log for sensitive reads: documents, location and exports
- **Meta:** module PRIV · phase F3 · cut C1 · priority P0 · type API · size S · state TODO
- **Objective:** Record permission-checked access to sensitive organization content.
- **Rules and use cases:** BR-AUD-003, BR-AUD-004, BR-AUTH-011, BR-PRIV-004, UC-PRIV-05
- **Depends on:** EMP-PLAT-003
- **Read first:** [privacy and security](../11-privacy-security.md), [shared business rules](../04-business-rules.md), [permissions](../07-permissions.md)
- **Scope:** in: audited document, export and location reads as authorized by their owning scope · out: new content activation
- **Areas:** existing: organization audit pattern · proposed: sensitive-access log and consumer ports
- **Steps:** Pin scope and permission cases. Commit a bounded access record before serving protected content.
- **Deliverable:** A scoped sensitive-access audit contract and tested consumer integration.
- **Acceptance:** An authorized read has an audit entry with scoped references and actor. No content or coordinate enters the log.
- **Negative cases:** Foreign item, missing permission, expired session, failed audit write and access-log content leakage.
- **Validation:** V-API, V-AUTHZ, V-PRIV, V-ISO, V-SQL · migration: additive under the audit module · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** A support session must explicitly cover the requested content category.
- **Done when:** Each affected protected read proves denial, audit and successful scoped access.
