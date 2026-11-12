# Trips, missions and operational logbook

Status: **PLANNED / RECONSTRUCTED**, 2026-11-11.
Recovered from the surviving enterprise requirements, shared rules and backlog.
The accepted F0 decisions and existing release gates govern any implementation.

## 1. Purpose

Represent planned work and recorded field activity for one organization's assets and members. A trip is an operational record with independent delivery and review.

## 2. Scope

C1 starts and completes a driver-initiated trip offline. F4 adds missions, planned work, handovers, supervisor closure, checklist integration and amendments. Tracking and navigation remain their own approved capabilities. Payroll and electronic time recording are outside this contract.

## 3. Entities and ownership

The organization owns mission, trip, leg, stop and typed logbook event. A trip references its authorized asset, employee and meter evidence. Events retain actor and original time. Operational, sync and review state are independent. A completed trip can still await delivery and review.

## 4. Business rules

- **BR-TRIP-001 — Scoped execution.** Every event retains its creation organization, trip, asset and actor (BR-TEN-004).
- **BR-TRIP-002 — One active asset use.** Concurrent trip starts serialize against the asset. The second admissible field fact is held for review under VAL-07.
- **BR-TRIP-003 — Evidence survives closure.** Late facts and corrections preserve approved revisions and use the BR-AMD path.
- **BR-TRIP-004 — Operational time.** Instants, zones, gaps and source readings remain explicit. No working-hour, wage or time-clock result is derived (EDR-028).

## 5. Use cases

**UC-TRIP-01 — Execute offline.** An eligible driver selects a cached assigned asset, records the departure reading, starts, adds typed stops and finishes with the final reading or stated absence. Each event is durable before success. On reconnect current authority and asset state determine acceptance or review.

**UC-TRIP-02 — Submit and review.** The member checks related expense and document status and submits the current revision. An independent scoped reviewer approves, returns with a reason or rejects. Stale decisions fail and previous revisions remain.

**UC-TRIP-03 — Hand over or close.** A currently permitted supervisor records the handover or justified closure of eligible open work. Missing final evidence remains missing and never becomes an invented distance.

## 6. States and transitions

The [state catalog](../08-state-model.md) owns mission and trip states. Planning or assignment does not start execution. Pause and resume retain a continuous trip identity and event history. A cancellation is recorded under the allowed current state. Completion changes operational state only. Review uses BR-REV and void preserves history. An ended membership triggers handover rather than deleting or automatically completing work.

## 7. Roles and permissions

Use [trip and mission permissions](../07-permissions.md) with resource scope. Record, plan, review and amendment are independent actions. Author and beneficiary exclusions apply. Checklist or meter corrections use the respective owning permission. General trip reads grant no document-content or track read.

## 8. Offline, concurrency, idempotency and corrections

The [offline matrix](../10-offline-sync.md) governs each action. UUIDs and context are final locally. Intake records acknowledgement, audit and effect atomically. Hard dependencies retry. Conflicting admissible facts enter review. Last-write-wins never resolves a stale revision. Reporting days use the unit context stored with the fact.

## 9. Events, notifications and integrations

Typed trip events feed timeline, assignment and approved projection consumers through the existing job queue. Relevant membership ending and asset-state changes trigger scoped handover or review. Tracking consumes trip lifecycle through its explicit port and remains separately authorized.

## 10. Data, formulas and examples

A trip from 184 320 000 m to 184 610 000 m records 290 000 m of meter evidence. Correcting the final reading to 184 600 000 m requires a reviewed amendment and gives 280 000 m after acceptance. GPS, planned and provider distances remain separate. Crossing midnight does not split the trip's identity or fabricate duration.

## 11. Screens

| ID | Surface | Objective | Actor | Data and actions | Validation |
|---|---|---|---|---|---|
| SCR-TRIP-01 | APP | Execute an eligible trip | Scoped driver | Reading, start, typed events, pause and finish | Durable offline state, accessible controls and explicit gaps |
| SCR-TRIP-02 | PANEL | Inspect and review | Scoped reviewer | Timeline, current revision and linked evidence | No self-approval, stale conflict and five shared states |
| SCR-TRIP-03 | PANEL | Plan and hand over work | Scoped planner | Missions, assignments and reasons | Current asset scope and permission, history retained |

## 12. Acceptance, dependencies, risks, phase and microtasks

[EP06](../backlog/ep06-trips.md) supplies the detailed tasks and phases. VAL-01, VAL-07 and VAL-26 in [validation](../15-validation.md) prove the first cut, concurrency and amendment. Immediate SQL races, replay, revoked authority and scope cases are required. Field and device acceptance remains separate from source integration.
