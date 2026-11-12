# EP06 — Operations, trips and logbook

Status: **PLANNED**, 2026-11-10.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

Recovered scope follows the surviving requirements and acceptance scenarios. Detailed trip rules are in [the recovered trip design](../modules/trips.md).

### EMP-TRIP-001 — Trip domain core: operational states and the driver-initiated trip
- **Meta:** module TRIP · phase F2 · cut C1 · priority P0 · type DOM · size M · state TODO
- **Objective:** Define the pure driver-initiated trip lifecycle.
- **Rules and use cases:** REQ-07.1, REQ-07.4, BR-REV-001, BR-TEN-004
- **Depends on:** EMP-FLEET-001, EMP-METER-001, EMP-PLAT-015
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: draft, start and operational completion · out: approval, missions and tracking
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Valid transitions preserve organization, asset and driver. Operational state is independent of review and delivery.
- **Acceptance:** Valid transitions preserve organization, asset and driver. Operational state is independent of review and delivery.
- **Negative cases:** Start without an eligible context, invalid transition or completed state mistaken for approval.
- **Validation:** V-DOM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-002 — Trip persistence and sync handlers with the one-active-trip-per-asset rule
- **Meta:** module TRIP · phase F2 · cut C1 · priority P0 · type DB · size L · state TODO
- **Objective:** Persist trips and serialize active use of one asset.
- **Rules and use cases:** REQ-07.1, BR-TEN-005, BR-OFFL-004, BR-OFFL-006, VAL-07
- **Depends on:** EMP-TRIP-001, EMP-OFFL-002, EMP-METER-002, EMP-FLEET-006
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: scoped PostGIS storage and intake · out: automatic winner changes
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Concurrent starts produce one active trip and a second fact held for review. Matching replay returns one acknowledgement.
- **Acceptance:** Concurrent starts produce one active trip and a second fact held for review. Matching replay returns one acknowledgement.
- **Negative cases:** Foreign asset, duplicate active trip, uncommitted receipt or silently dropped conflict.
- **Validation:** V-SQL, V-API, V-ISO, V-IDEM, V-SYNC · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-003 — Logbook events: the typed timeline
- **Meta:** module TRIP · phase F2 · cut C1 · priority P0 · type DB · size M · state TODO
- **Objective:** Append typed logbook facts with an understandable time-ordered history.
- **Rules and use cases:** REQ-07.3, BR-OFFL-001, BR-TIME-002, EDR-028
- **Depends on:** EMP-TRIP-002
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: typed event and source timestamp · out: time-clock or payroll output
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Authorized events retain stable identity, actor and original time. The timeline distinguishes fact type and receipt.
- **Acceptance:** Authorized events retain stable identity, actor and original time. The timeline distinguishes fact type and receipt.
- **Negative cases:** Duplicate delivery, another organization's target or an unsupported event changing trip state.
- **Validation:** V-DOM, V-API, V-SQL, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-004 — Trip submission and administrative review
- **Meta:** module TRIP · phase F2 · cut C1 · priority P0 · type API · size M · state TODO
- **Objective:** Submit and review the operational trip without changing source facts.
- **Rules and use cases:** REQ-07.2, BR-REV-001, BR-REV-003, BR-REV-005, BR-REV-007, VAL-01
- **Depends on:** EMP-TRIP-002, EMP-APPR-001
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: scoped submission, approval, return and rejection · out: wage calculations and automatic approval
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The reviewer decides one matching revision and records the reason and audit. Operational completion alone gives no administrative approval.
- **Acceptance:** The reviewer decides one matching revision and records the reason and audit. Operational completion alone gives no administrative approval.
- **Negative cases:** Self-approval, stale revision, missing reason, or repeat applying a second decision.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-005 — App: start a trip, log events and finish, offline first
- **Meta:** module TRIP · phase F2 · cut C1 · priority P0 · type APP · size L · state TODO
- **Objective:** Let the assigned driver record a trip offline in the approved partition.
- **Rules and use cases:** REQ-07.2, BR-OFFL-002, BR-OFFL-003, UC-OFFL-01, JRN-06
- **Depends on:** EMP-TRIP-002, EMP-TRIP-003, EMP-OFFL-003, EMP-FLEET-013
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: start, typed events and finish through the durable queue · out: tracking and planned missions in C1
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** After restart, the original context and trip timeline remain available and acceptance follows server receipts.
- **Acceptance:** After restart, the original context and trip timeline remain available and acceptance follows server receipts.
- **Negative cases:** Context switch retags work, local save without persistence or completion shown as accepted early.
- **Validation:** V-APP, V-SYNC, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-006 — Panel: trips list, timeline detail and review
- **Meta:** module TRIP · phase F2 · cut C1 · priority P0 · type WEB · size M · state TODO
- **Objective:** Show authorized trip detail, timeline and review in the company panel.
- **Rules and use cases:** REQ-07.3, BR-AUTH-013, BR-REV-007, EDR-028
- **Depends on:** EMP-TRIP-004, EMP-PLAT-008
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: scoped list, timeline and permitted decisions · out: computed work hours and client authority
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The panel exposes only scoped facts and distinct operational, sync and review states.
- **Acceptance:** The panel exposes only scoped facts and distinct operational, sync and review states.
- **Negative cases:** Foreign result, missing reason, stale decision or report shown as a time record.
- **Validation:** V-WEB, V-AUTHZ, V-ISO · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-007 — Asset-overlap conflict resolution
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Resolve overlapping asset claims through the review queue.
- **Rules and use cases:** REQ-07.5, BR-OFFL-006, BR-REV-003, BR-REV-007, VAL-07
- **Depends on:** EMP-TRIP-002, EMP-OFFL-008
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: conflict evidence and one audited resolution · out: automatically discarding the losing field fact
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Concurrent start evidence stays readable and resolution preserves one active trip per asset.
- **Acceptance:** Concurrent start evidence stays readable and resolution preserves one active trip per asset.
- **Negative cases:** Self-review, second active trip, repeated resolution or loss of the conflict evidence.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-008 — Missions: create, assign, accept and decline
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Create and assign missions with a clear accept or decline outcome.
- **Rules and use cases:** REQ-07.1, REQ-07.2, BR-AUTH-001, BR-REV-001
- **Depends on:** EMP-TRIP-002
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: scoped mission assignment and responses · out: treating an assignment as trip execution
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Only the assigned eligible member responds, and a response does not start the vehicle or alter review state.
- **Acceptance:** Only the assigned eligible member responds, and a response does not start the vehicle or alter review state.
- **Negative cases:** Wrong member, duplicate acceptance, ended assignment or offline result bypassing revalidation.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-009 — Manager-planned trips: destination, reason, customer or project, cargo or passengers
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Record manager-planned trip inputs separately from field facts.
- **Rules and use cases:** REQ-07.2, REQ-07.3, BR-PRIV-005, BR-AUD-001
- **Depends on:** EMP-TRIP-008
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: destination, reason and approved project, cargo or passenger fields · out: unnecessary personal data
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Planning revision records the authorized source and stays distinct from actual execution.
- **Acceptance:** Planning revision records the authorized source and stays distinct from actual execution.
- **Negative cases:** Foreign unit, unapproved sensitive data or a plan revision rewriting a completed trip.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-010 — Stops and legs: planned versus actual
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Preserve planned and actual stops and legs in the timeline.
- **Rules and use cases:** REQ-07.3, BR-TIME-001, BR-TIME-002, BR-OFFL-007
- **Depends on:** EMP-TRIP-003
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: typed stops, legs and temporal context · out: equating a route estimate to actual travel
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The member can distinguish the plan from the observed stop and leg with their recorded times.
- **Acceptance:** The member can distinguish the plan from the observed stop and leg with their recorded times.
- **Negative cases:** Out-of-scope stop, duplicate transport, changed planned data overwriting actual or incoherent time hidden.
- **Validation:** V-DOM, V-API, V-SQL, V-SYNC, V-ISO · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-011 — Pause, resume and cancellation
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type DOM · size S · state TODO
- **Objective:** Apply scoped pause, resume and cancellation as lifecycle events.
- **Rules and use cases:** REQ-07.5, BR-REV-001, BR-AUD-001, BR-OFFL-004
- **Depends on:** EMP-TRIP-002
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: state transitions and reasons under current authority · out: erasing prior facts
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** One accepted event changes one current operational state and keeps the prior timeline.
- **Acceptance:** One accepted event changes one current operational state and keeps the prior timeline.
- **Negative cases:** Resume after terminal state, stale or duplicated request, missing reason or cancellation of another organization's trip.
- **Validation:** V-DOM, V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-012 — Driver handover during a trip
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Record driver handover without moving historical responsibility.
- **Rules and use cases:** REQ-07.6, BR-TEN-003, BR-AUD-005, BR-TIME-002
- **Depends on:** EMP-TRIP-002
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: eligible receiving driver and effective handover evidence · out: implicit delegation or rewriting old events
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Future actions use the new authorized responsibility. Existing events retain their original actor.
- **Acceptance:** Future actions use the new authorized responsibility. Existing events retain their original actor.
- **Negative cases:** Foreign driver, closed trip, stale handover or overlapping ownership from concurrent commands.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-013 — Vehicle change within a mission
- **Meta:** module TRIP · phase F4 · cut - · priority P2 · type DOM · size S · state TODO
- **Objective:** Change the assigned vehicle while preserving each vehicle's trip evidence.
- **Rules and use cases:** REQ-07.1, REQ-07.6, BR-TEN-005, BR-TIME-002
- **Depends on:** EMP-TRIP-008
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: authorized vehicle replacement within the mission and scoped meter context · out: merging meter histories
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The previous vehicle history and readings remain distinct from the new vehicle's work.
- **Acceptance:** The previous vehicle history and readings remain distinct from the new vehicle's work.
- **Negative cases:** Foreign or unavailable vehicle, overlapping active trip or reading reassigned to a different asset.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-014 — Missing final reading and supervisor closure
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Close an eligible open trip with explicit missing-reading evidence.
- **Rules and use cases:** REQ-07.6, JRN-07, BR-TRUTH-003, BR-REV-003, BR-AUD-001
- **Depends on:** EMP-TRIP-004, EMP-METER-004
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: authorized supervisor closure and reason · out: fabricating distance or a meter value
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The trip closes under approved authority and missing evidence stays visible. No distance is invented.
- **Acceptance:** The trip closes under approved authority and missing evidence stays visible. No distance is invented.
- **Negative cases:** Self-decision where prohibited, nonexistent meter value, foreign trip or stale second closure.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-015 — Late and duplicate events, and amendments after closure
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Preserve late logbook facts and amend approved work through a new reviewed revision.
- **Rules and use cases:** REQ-07.5, REQ-07.6, BR-OFFL-004, BR-AMD-001, BR-AMD-005, VAL-26
- **Depends on:** EMP-TRIP-004, EMP-PLAT-016
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: duplicate detection, late intake and explicit amendment · out: editing approved records in place
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A repeat has one effect and the old revision remains effective until review accepts the amendment.
- **Acceptance:** A repeat has one effect and the old revision remains effective until review accepts the amendment.
- **Negative cases:** Late fact dropped, duplicate counted, stale decision or recalculation loses revision provenance.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-SYNC · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-016 — Period lock and authorised reopening for trips
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Apply period locks and permitted reopen or adjustment to trip changes.
- **Rules and use cases:** REQ-07.5, BR-AMD-004, BR-TIME-003, BR-TIME-004, JRN-09
- **Depends on:** EMP-TRIP-004, EMP-APPR-006
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: current reporting period authority · out: bypassing lock with a client date
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** A locked period keeps its approved facts. Authorized reopening or adjustment records a reason and retains history.
- **Acceptance:** A locked period keeps its approved facts. Authorized reopening or adjustment records a reason and retains history.
- **Negative cases:** Wrong reporting zone, stale authority, lock race or in-place rewrite.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-017 — Midnight and time-zone crossing rules
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type DOM · size S · state TODO
- **Objective:** Attribute cross-midnight and cross-zone trips consistently.
- **Rules and use cases:** REQ-07.6, BR-TIME-001, BR-TIME-002, BR-TIME-004, BR-TIME-005
- **Depends on:** EMP-TRIP-001
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: original time context and reporting day · out: inferring paid hours
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The trip keeps its start reporting day and events retain source zone and instant.
- **Acceptance:** The trip keeps its start reporting day and events retain source zone and instant.
- **Negative cases:** Local clock change invents duration, midnight splits one trip or reporting basis is lost.
- **Validation:** V-DOM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-018 — Open trips when a membership ends
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Hand over open trips when access ends.
- **Rules and use cases:** REQ-18.4, BR-IDN-027, BR-IDN-028, BR-OFFL-011, JRN-04
- **Depends on:** EMP-TRIP-014, EMP-IDN-010
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: successor or supervisor workflow and permitted late intake · out: reviving access or auto-closing work
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Work is flagged or handed over under scoped authority and remains auditable.
- **Acceptance:** Work is flagged or handed over under scoped authority and remains auditable.
- **Negative cases:** Ended member continues normal work, lost-device intake admitted or open trip silently deleted.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-SYNC · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-019 — Checklist gate at trip start and finish
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Apply recorded checklist policy at trip boundaries.
- **Rules and use cases:** REQ-07.2, BR-CHECK-005, BR-CHECK-007, BR-CHECK-008, BR-CHECK-013
- **Depends on:** EMP-TRIP-002, EMP-CHECK-002
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: departure gate, return evidence and offline discrepancy review · out: dropping field facts
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The departure decision follows current policy. Return evidence is attached without keeping a finished trip open.
- **Acceptance:** The departure decision follows current policy. Return evidence is attached without keeping a finished trip open.
- **Negative cases:** Cached policy bypasses server gate or a return checklist prevents finish against the contract.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-SYNC · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-020 — Trip closing conference: expenses and documents before submission
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type APP · size M · state TODO
- **Objective:** Review trip-related expenses and documents before submission.
- **Rules and use cases:** REQ-07.2, REQ-07.3, BR-PRIV-004, BR-REV-002, BR-TRUTH-001
- **Depends on:** EMP-TRIP-005, EMP-FUEL-011, EMP-EXP-004
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: scoped closing checklist and linked evidence states · out: counting a link as a cost or certifying a document
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The close view distinguishes missing, pending and available evidence and submits the correct revision.
- **Acceptance:** The close view distinguishes missing, pending and available evidence and submits the correct revision.
- **Negative cases:** Foreign attachment, duplicate cost, unreadable evidence shown as verified or offline approval.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-PRIV · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-021 — App: missions inbox and planned-trip execution
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type APP · size M · state TODO
- **Objective:** Let drivers act on scoped missions and planned trips in Android.
- **Rules and use cases:** REQ-07.2, BR-OFFL-003, BR-OFFL-016, BR-OFFL-017, JRN-06
- **Depends on:** EMP-TRIP-008, EMP-TRIP-009, EMP-TRIP-011, EMP-TRIP-005
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: mission inbox and execution states · out: granting new offline authority
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Cached assignments show age and eligible actions follow the approved offline matrix.
- **Acceptance:** Cached assignments show age and eligible actions follow the approved offline matrix.
- **Negative cases:** Foreign mission, stale policy unlabelled, queued command retagged or online-only action reported complete offline.
- **Validation:** V-APP, V-SYNC, V-AUTHZ, V-ISO · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-022 — Panel: mission planning and assignment board
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Present scoped mission planning and assignment in the company panel.
- **Rules and use cases:** REQ-07.2, BR-AUTH-004, BR-AUTH-013, BR-TRUTH-006
- **Depends on:** EMP-TRIP-009, EMP-FLEET-007, EMP-PLAT-008
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: plan and assignment board · out: editing field execution directly
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** Only eligible scoped assets and assignees are shown, and the board reports current server state.
- **Acceptance:** Only eligible scoped assets and assignees are shown, and the board reports current server state.
- **Negative cases:** Foreign assignment, concurrent update hidden or unconfirmed state presented as accepted.
- **Validation:** V-WEB, V-AUTHZ, V-ISO · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.

### EMP-TRIP-023 — Reservation-to-trip integration and no-show release
- **Meta:** module TRIP · phase F4 · cut - · priority P2 · type DOM · size S · state TODO
- **Objective:** Convert eligible reservations into trip use and release no-shows safely.
- **Rules and use cases:** REQ-07.1, BR-TEN-005, BR-OFFL-004, BR-TRUTH-006
- **Depends on:** EMP-FLEET-007, EMP-TRIP-008
- **Read first:** [trip requirements](../17-traceability.md), [state model](../08-state-model.md), [offline sync](../10-offline-sync.md) and [validation](../15-validation.md)
- **Scope:** in: authorized reservation consumption and no-show release · out: creating parallel active use
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** One eligible reservation yields one linked use or one recorded release.
- **Acceptance:** One eligible reservation yields one linked use or one recorded release.
- **Negative cases:** Race between release and start, expired reservation consumed or foreign asset assignment.
- **Validation:** V-API, V-SQL, V-JOB, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.
