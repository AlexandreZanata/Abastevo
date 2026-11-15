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
- **Meta:** module TRIP · phase F2 · cut C1 · priority P0 · type DOM · size M · state LOCAL_DONE
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

[F4 pure trip domain](../evidence/f4-trip001-trip-domain.md) is LOCAL_DONE /
INTEGRATION_PENDING, 2026-11-15. The closed operational lifecycle (START
creates IN_PROGRESS, FINISH completes; pause, resume and cancel refused as
unsupported until EMP-TRIP-011), the eligible start context with its ordered
missing list, the start reporting day, completion by the starting member and
never-invented meter usage pass all 207 shared Go/Kotlin vectors with 100% Go
statements. Persistence, intake, authority and review binding remain open.

### EMP-TRIP-002 — Trip persistence and sync handlers with the one-active-trip-per-asset rule
- **Meta:** module TRIP · phase F2 · cut C1 · priority P0 · type DB · size L · state LOCAL_DONE
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

[F4 trip persistence](../evidence/f4-trip002-trip-persistence.md) is
LOCAL_DONE / INTEGRATION_PENDING, 2026-11-15. Signed `trip.start` and
`trip.finish` (migration 000220) store one trip and its one completion with
TRIP_START/TRIP_END readings; at most one trip per asset is in progress; a
second admissible start is held `ASSET_BUSY` (both race orders on PostGIS) and
a second finish `TRIP_ALREADY_CLOSED`; FLEET-003 open work and the FLEET-007
active use are trip-backed and the FLEET-007 start decision holds blocked or
reserved starts. Held-item resolution stays with EMP-TRIP-007.

### EMP-TRIP-003 — Logbook events: the typed timeline
- **Meta:** module TRIP · phase F2 · cut C1 · priority P0 · type DB · size M · state LOCAL_DONE
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

[F4 logbook events](../evidence/f4-trip003-logbook-events.md) is
LOCAL_DONE / INTEGRATION_PENDING, 2026-11-15. Signed `trip.event.record`
(migration 000225) appends one insert-only typed fact (NOTE, STOP, ARRIVAL,
DEPARTURE) with its identity, author, original instant and zone and server
receipt to the member's own trip and never changes the trip; reserved types
stay queued, an event before the start is `MALFORMED`, one after a completed
trip's finish is held `TRIP_ALREADY_CLOSED`; the timeline order and vocabulary
are shared with Kotlin through `logbook-v1.psv`. No read route (EMP-TRIP-006).

### EMP-TRIP-004 — Trip submission and administrative review
- **Meta:** module TRIP · phase F2 · cut C1 · priority P0 · type API · size M · state LOCAL_DONE
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

[F4 trip review](../evidence/f4-trip004-trip-review.md) is LOCAL_DONE /
INTEGRATION_PENDING, 2026-11-15. The author's signed `trip.submit` (entity
`trip_review`, expected revision, empty payload, migration 000226) appends
the APPR-001 SUBMIT entry to a COMPLETED trip; an in-progress or undelivered
trip is `DEPENDENCY_PENDING`, another member `FORBIDDEN`, a non-draft case
stays `RETRY`. Company routes under `trip.trip.review` list the queue (ORG),
read the decision context and decide one revision (fresh proof; OPEN and
APPROVE/RETURN/REJECT in one audited transaction) with SELF_APPROVAL,
CONFLICT, VALIDATION_FAILED, OPERATION_ID_REUSED, NOT_FOUND and structural
FORBIDDEN proven on PostGIS. Resubmission after return, cancel, amendments
and the panel remain EMP-TRIP-015/006.

### EMP-TRIP-005 — App: start a trip, log events and finish, offline first
- **Meta:** module TRIP · phase F2 · cut C1 · priority P0 · type APP · size L · state LOCAL_DONE
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

[F4 app trip](../evidence/f4-trip005-app-trip.md) is LOCAL_DONE /
INTEGRATION_PENDING: B-BR-EMP-375..380, BUC-EMP-189..191 and EDR-242..245
precede behaviour; native acceptance on the owned API36 emulator against a
real local backend. Retained: the author's own review-result read, held-start
resolution (TRIP-007), the current-activity card and driving layout, two
physical devices (OWED), visual/TalkBack review.

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
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type API · size M · state LOCAL_DONE
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

[F4 conflict resolution](../evidence/f4-trip007-conflict-resolution.md) is
LOCAL_DONE / INTEGRATION_PENDING, 2026-11-15. Held trip starts, finishes,
events and submissions are listed, read and resolved once under
`trip.trip.review` (`/v1/orgs/{org_id}/sync/reviews/trips/…`, structure
through the held fact): refuse with a reason, or accept a held start as the
trip it declared (in progress, its reading at the original receipt, its
pending finish then accepted) only when no trip of the asset is in progress
or finished after its declared start; migration 000237 re-proves every
acceptance; the held evidence stays and the author's review-results read
answers trip items. Accept with another asset and the Android presentation
of trip resolutions remain retained.

### EMP-TRIP-008 — Missions: create, assign, accept and decline
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type API · size M · state LOCAL_DONE
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

[F4 missions](../evidence/f4-trip008-missions.md) are LOCAL_DONE /
INTEGRATION_PENDING, 2026-11-15. Pure mission domain shared with Kotlin
through `mission-v1.psv`; migration 000227 (guarded head, insert-only
transitions and receipts). Company routes under `trip.mission.manage` create
(DRAFT), list, read, assign (eligible `trip.trip.start` member, FLEET-007
start decision as a warning only) and cancel with revision CAS; the own list
under `trip.trip.start`; the assignee answers offline through the signed
`trip.mission.respond`, decided on current facts at receipt (FORBIDDEN,
NOT_FOUND, CONFLICT store nothing). No response starts a trip; the trip link,
planning edits, inbox and board remain EMP-TRIP-009/013/021/022/023.

### EMP-TRIP-009 — Manager-planned trips: destination, reason, customer or project, cargo or passengers
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type API · size M · state LOCAL_DONE
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

[F4 planned trips](../evidence/f4-trip009-planned-trips.md) are LOCAL_DONE /
INTEGRATION_PENDING, 2026-11-15. `PLAN` moves a DRAFT, PLANNED, ASSIGNED or
DECLINED mission to PLANNED (assignee and answer withdrawn; ACCEPTED,
IN_PROGRESS and terminal refused) and appends an insert-only planning revision
(migration 000231) bound to its transition, receipt and audit event:
destination text or unit, reason, the cost centre as customer or project, and
a load NONE, CARGO with a description or PASSENGERS with only a count; free
texts refuse CPF-shaped tokens. Shared with Kotlin through `mission-v2.psv`
(superseding v1); `POST …/missions/{mission_id}/plan` under fresh
`trip.mission.manage`; the plan is read with the mission. The app display
remains EMP-TRIP-021.

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
- **Meta:** module TRIP · phase F4 · cut - · priority P2 · type DOM · size S · state LOCAL_DONE
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

[F4 vehicle change](../evidence/f4-trip013-vehicle-change.md) is LOCAL_DONE /
INTEGRATION_PENDING, 2026-11-15. A server-only mission operation replaces the
vehicle of a PLANNED, ASSIGNED, ACCEPTED or DECLINED mission (state,
assignee, answer and plan kept) with an insert-only change record (migration
000232); it is refused while the assignee runs a trip on the current
vehicle, for a vehicle that is not an ACTIVE main asset, is unavailable at the
planned start or runs another trip, and when the planner or the assignee does
not cover the mission with it. `POST …/missions/{mission_id}/vehicle/change`
under fresh `trip.mission.manage`; trips, readings and meters never move.

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
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type API · size M · state LOCAL_DONE
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

[F4 late events and amendments](../evidence/f4-trip015-late-events-amendments.md)
is LOCAL_DONE / INTEGRATION_PENDING, 2026-11-15. The author resubmits a
returned field revision with `trip.submit` (revision + 1); an event inside a
submitted, decided or amended trip and a stale or duplicate submission are
held once `STALE_REVISION` (migration 000229); an approved trip is amended
under `meter.reading.correct` as a new reviewed revision through the
orgamend kernel, whose approval supersedes the corrected readings through
the METER-004 path (290 000 m until approval, 280 000 m after, usage
derived per named revision), and METER-004 refuses readings of approved
trips `AMENDMENT_REQUIRED`. Held-item resolution stays with EMP-TRIP-007.

### EMP-TRIP-016 — Period lock and authorised reopening for trips
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type API · size S · state LOCAL_DONE
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

[F4 trip period lock](../evidence/f4-trip016-trip-period-lock.md) is
LOCAL_DONE / INTEGRATION_PENDING, 2026-11-15. A trip review decision reads
the APPR-006 period of the trip's stored start reporting day inside its
transaction after the lifecycle checks: `OPEN` or an authorised `REOPENED`
month admits it, a `LOCKED` one is `PERIOD_LOCKED` with nothing written;
replays, reads, the queue and field intake are not gated and trips have no
adjustment form; migration 000228 makes the database refuse an opening or
decision in a locked month. Wrong zone, stale reopening, both race orders,
in-place rewrites and the populated upgrade are proven on PostGIS.

### EMP-TRIP-017 — Midnight and time-zone crossing rules
- **Meta:** module TRIP · phase F4 · cut - · priority P1 · type DOM · size S · state LOCAL_DONE
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

[F4 trip time crossing](../evidence/f4-trip017-time-crossing.md) is
LOCAL_DONE / INTEGRATION_PENDING, 2026-11-15. A trip keeps its start
reporting day across midnight and zones (informational end day in the stored
zone), every event keeps its own instant, entry zone and offset, and elapsed
time comes only from instants, confirmed by an optional monotonic run clock
or flagged as a gap; all 66 shared Go/Kotlin vectors pass with 100% Go
statements. The Android clock source and persistence remain open.

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
