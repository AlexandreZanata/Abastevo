# EP06 — Operations, trips and logbook

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from trip requirements, [state model](../state-model.md), [offline sync](../offline-sync.md) and [validation](../testing.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
sequencing decision.

Recovered scope follows the surviving requirements and acceptance scenarios. Detailed trip rules are in [the recovered trip design](../modules/trips.md).

### EMP-TRIP-006 — Panel: trips list, timeline detail and review
- **Meta:** module TRIP · phase F2 · cut C1 · priority P0 · type WEB · size M · state TODO
- **Objective:** Show authorized trip detail, timeline and review in the company panel.
- **Rules and use cases:** REQ-07.3, BR-AUTH-013, BR-REV-007, EDR-028
- **Depends on:** EMP-TRIP-004, EMP-PLAT-008
- **Read first:** trip requirements, [state model](../state-model.md), [offline sync](../offline-sync.md) and [validation](../testing.md)
- **Scope:** in: scoped list, timeline and permitted decisions · out: computed work hours and client authority
- **Areas:** existing: organization scope, fleet, sync and review conventions · proposed: owning trip domain, adapters and consumers
- **Steps:** Pin the stated acceptance and denial cases before changing the owning entry point. Verify the affected consumers.
- **Deliverable:** The panel exposes only scoped facts and distinct operational, sync and review states.
- **Acceptance:** The panel exposes only scoped facts and distinct operational, sync and review states.
- **Negative cases:** Foreign result, missing reason, stale decision or report shown as a time record.
- **Validation:** V-WEB, V-AUTHZ, V-ISO · migration: rehearse additive upgrade and recovery if persistent state changes · rollback: close the new entry and preserve accepted evidence
- **Risks and assumptions:** The [trip design](../modules/trips.md) supplies the transition and policy boundaries. C1 remains driver-initiated and the logbook remains operational.
- **Done when:** The stated result and denials pass the listed validation profiles.
