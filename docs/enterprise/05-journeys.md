# End-to-end journeys

Status: **PLANNED**, 2026-11-10. Each journey follows real people through several modules, with the rules that apply at each step and the exceptions that must work. Journeys are the bridge between the business rules and the screens: a module is not complete until the journeys that cross it work end to end. Module detail is in `modules/`; shared rules are in [04-business-rules](04-business-rules.md), [07-permissions](07-permissions.md), [10-offline-sync](10-offline-sync.md) and [11-privacy-security](11-privacy-security.md).

People used in the examples are invented: Marta owns Transportes Exemplo; Ana supervises unit Norte; Bruno and Carla drive; Davi works in finance; Elisa manages a station.

## 1. Company and people

### JRN-01 — From sign-up to the first reviewed fueling

The first operational cut (C1 in [14-roadmap](14-roadmap.md)).

| Step | Who | What happens | Rules |
|---|---|---|---|
| 1 | Marta | Signs in to the business web with her Abastevo account and creates the organization "Transportes Exemplo" with its first unit. She becomes its owner. | BR-TEN-001, BR-AUTH-006; [organizations](modules/organizations.md) |
| 2 | Marta | Registers Bruno as an employee and issues an invitation. The link or code is shown to her to hand over. | BR-TEN-002; [identity](modules/identity.md) |
| 3 | Bruno | Opens the invitation in the app, signs in with his own account and accepts. Enterprise mode appears with the organization's name. | BR-AUTH-012, UC-AUTH-04 |
| 4 | Marta | Registers a vehicle and assigns it to Bruno. | [fleet](modules/fleet.md) |
| 5 | Bruno | With no signal, starts a trip with the odometer reading, adds a stop and a note. | BR-OFFL-001 to 003, UC-OFFL-01; [trips](modules/trips.md) |
| 6 | Bruno | Records a fueling: 150.000 L of diesel, R$ 973.50 paid, with a photo of the receipt and the odometer. | BR-MONEY-001 to 006; [fuelings](modules/fuelings.md), [documents](modules/documents.md) |
| 7 | Bruno | Finishes the trip with the final reading and marks it ready to send. Everything shows "Salvo no aparelho". | BR-REV-001, BR-REV-002 |
| 8 | Phone | Reaches the network; operations are delivered in order and once; the receipt uploads; records show "Aceito pela empresa". | BR-OFFL-004, 005, 007, 015 |
| 9 | Marta | Opens the trip timeline and the fueling with its receipt in the panel; approves the fueling; returns the trip asking for the toll receipt. | BR-REV-003 to 005, BR-AUTH-011, BR-AUD-003 |
| 10 | Bruno | Sees the result on the phone, adds the receipt and resubmits. | BR-REV-005 |

Exceptions that must work: the invitation expires before Bruno opens it (a new one is issued; the old one is useless); Bruno picks a vehicle that was reassigned while he was offline (the trip is kept for review with `ASSET_UNAVAILABLE`, not lost); the receipt photo is missing from the phone at upload time (the fueling stays accepted with the document pending and the app asks for a new capture); Marta tries to approve her own fueling (refused, BR-REV-003); a member of another organization requests the trip by identifier (not found, BR-TEN-006).

Result: one trip and one fueling exist once, belong to Transportes Exemplo, carry their evidence and their decisions, and are invisible to any other organization.

### JRN-02 — An employee without e-mail gets corporate access

| Step | Who | What happens | Rules |
|---|---|---|---|
| 1 | Marta | Registers Carla, who has no e-mail and no Abastevo account, and issues an activation code. It is shown once, to be handed over in person. | [identity](modules/identity.md); EDR-007 |
| 2 | Carla | Installs the app, chooses corporate access and types the code. The device enrols its key; Carla sets the local unlock. | BR-PRIV-019 |
| 3 | App | Opens enterprise mode for Transportes Exemplo only. There is no personal mode content in this account. | EDR-031, BR-AUTH-012 |
| 4 | Carla | Changes phone months later. Marta revokes the old device and issues a new code. | BR-PRIV-018 |

Exceptions: the code is typed wrong too many times (it locks; a new one is issued); the code leaks before use (Marta revokes it); someone knows Carla's registration number and the company's CNPJ (useless, BR-TEN-002); Marta wants to see Carla's access secret (there is none to see: recovery is always a new code).

### JRN-03 — A person who works for two companies

| Step | Who | What happens | Rules |
|---|---|---|---|
| 1 | Bruno | Also drives for Entregas Modelo on weekends, with the same personal account and a second membership. | [identity](modules/identity.md) |
| 2 | Bruno | Uses the context switcher. Each organization has its own enterprise mode, assets and pending queue. | UC-AUTH-04, BR-OFFL-003 |
| 3 | Bruno | Records a fueling in Entregas Modelo while two operations of Transportes Exemplo are still unsent. | BR-TEN-004, UC-OFFL-05 |
| 4 | Phone | Sends each operation to the organization it was created in. | BR-OFFL-003 |

Exceptions: Bruno records the fueling in the wrong company (it is voided there and recorded again in the right one, by people authorized in each; it is never moved); one membership is suspended (that context is shown as unavailable; the other keeps working).

### JRN-04 — An employee leaves with records still on the phone

| Step | Who | What happens | Rules |
|---|---|---|---|
| 1 | Marta | Offboards Bruno on Friday at 18:00. His permissions stop at once; his open trip is handed to Ana to close. | BR-AUTH-014; [identity](modules/identity.md), [trips](modules/trips.md) |
| 2 | Bruno | His phone was offline since Friday morning with three fuelings recorded before 18:00. On Monday it reconnects. | BR-OFFL-011, UC-OFFL-07 |
| 3 | Server | Accepts the three fuelings for review, because they were declared before the end and arrive inside the intake window. They do not count in costs yet. | BR-OFFL-006, 009 |
| 4 | Ana | Sees them in the review queue with declared time, receipt time and last sync, and decides each one. | UC-OFFL-04 |
| 5 | Bruno | Keeps his personal account, his personal vehicles and his community history. He can no longer read the company's data. | BR-TEN-009, BR-PRIV-014 |

Exceptions: a fourth fueling was recorded on Saturday (refused with `MEMBERSHIP_ENDED`); the phone reconnects after the window (refused with `INTAKE_WINDOW_CLOSED`; the app tells him to contact the company); Bruno was the last owner (the offboarding is refused until ownership is transferred, BR-AUTH-006).

### JRN-05 — A phone is lost

| Step | Who | What happens | Rules |
|---|---|---|---|
| 1 | Carla | Reports her phone lost. | — |
| 2 | Marta | Revokes the device as lost. The server refuses it from that moment. | BR-OFFL-012, UC-PRIV-03 |
| 3 | Phone | If it reconnects, the app erases the organization's local data. If it never reconnects, the data becomes unreadable when its offline validity expires. | BR-PRIV-018, BR-OFFL-014 |
| 4 | Marta | Is told that records not yet sent from that phone are lost, and issues a new activation code. | BR-OFFL-013 |

## 2. Operations and money

### JRN-06 — A driver's day without signal

| Step | Who | What happens | Rules |
|---|---|---|---|
| 1 | Carla | At the yard, with signal, opens enterprise mode. Missions, assets and policies refresh. | BR-OFFL-016, UC-OFFL-08 |
| 2 | Carla | Fills the departure checklist; one non-critical item is marked with a photo. | [maintenance](modules/maintenance.md) |
| 3 | Carla | Starts the trip; tracking starts from the trip screen with a visible indicator; navigation guides her offline. | BR-PRIV-006; [gps-navigation](modules/gps-navigation.md) |
| 4 | Carla | Stops twice, pays a toll, fuels, photographs both receipts. Detailed entry happens only while stopped. | [trips](modules/trips.md), [expenses-finance](modules/expenses-finance.md) |
| 5 | Carla | Finishes after midnight. The trip belongs to the day it started. | BR-TIME-004 |
| 6 | Phone | Back in coverage, sends operations, then track batches, then photos. | BR-OFFL-007, BR-OFFL-015 |
| 7 | Ana | Sees the trip with its timeline, its track with one gap in a tunnel, and both distances: odometer and GPS. | BR-TRUTH-004, BR-TRUTH-006 |

Exceptions: the phone restarts mid-trip (recording does not resume silently; Carla is asked; the gap is recorded); location permission is approximate only (the trip works; the track is marked insufficient for distance); the tank capacity is exceeded by the recorded volume (the fueling is accepted with a warning for review).

### JRN-07 — A supervisor closes a trip left open

| Step | Who | What happens | Rules |
|---|---|---|---|
| 1 | System | Flags a trip in progress for 30 hours with no communication. The live position is shown as stale, with its collection time. | BR-TRUTH-006 |
| 2 | Ana | Contacts the driver, then closes the trip on his behalf with a reason. The final reading is missing. | BR-AUD-005; [trips](modules/trips.md) |
| 3 | Ana | Records the reading from the yard the next morning as a separate reading with its own time and author. | [fleet](modules/fleet.md) |
| 4 | Phone | The driver's phone reconnects later with its own events for that trip. They are kept for review as late events. | BR-OFFL-006 |

Result: odometer distance is computed from the reading Ana recorded; the indicator shows which reading was used and that the trip was closed by a supervisor.

### JRN-08 — A fueling paid from the employee's pocket is reimbursed

| Step | Who | What happens | Rules |
|---|---|---|---|
| 1 | Bruno | Records a fueling of R$ 973.50 paid with his own money. | [fuelings](modules/fuelings.md) |
| 2 | System | Creates one expense of R$ 973.50 for that fueling. | EDR-013; [expenses-finance](modules/expenses-finance.md) |
| 3 | Ana, then Davi | Approve according to the policy: Ana as supervisor, Davi as finance. | BR-REV-008 |
| 4 | System | Creates a payable of R$ 973.50 to Bruno. | — |
| 5 | Davi | Records the payment made by bank transfer. | — |

Result: the cost report shows R$ 973.50 once, in the month of the fueling. The disbursement report shows R$ 973.50 once, in the month of the transfer. Nothing is counted twice.

Exceptions: the receipt shows R$ 968.50 paid and R$ 5.00 discount (consistent, BR-MONEY-005); Davi approves only R$ 900.00 because R$ 73.50 was a personal purchase on the same receipt (the company amount is R$ 900.00; the payable is R$ 900.00); the same receipt is attached to a second fueling (the document's allocations cannot exceed its total).

### JRN-09 — Correcting an approved record in a closed period

| Step | Who | What happens | Rules |
|---|---|---|---|
| 1 | Davi | Locks October. | BR-AMD-004 |
| 2 | Ana | Finds in November that an approved October fueling has the wrong quantity. | — |
| 3 | Davi | Either reopens October with a reason and the amendment is reviewed there, or posts an adjustment in November linked to the original. | BR-AMD-001, 002, 004 |
| 4 | System | Recalculates the consumption interval and the indicators that used the fueling; the recalculation is identifiable. | BR-AMD-005, BR-TRUTH-004 |

Result: the original stays visible with its history; nothing was edited in place.

## 3. Stations, plans and data

### JRN-10 — A station proves representation, announces prices and serves a fleet

| Step | Who | What happens | Rules |
|---|---|---|---|
| 1 | Elisa | Creates the organization "Posto Modelo", finds the existing station in the catalog and submits a claim with the evidence asked. | [station-claims](modules/station-claims.md) |
| 2 | Platform reviewer | Reviews the evidence; asks for one more document; approves. A station authority exists; the badge appears. | BR-AUTH-015; [back-office](modules/back-office.md) |
| 3 | Elisa | Gives her manager a station role; he publishes today's diesel price with a validity. | BR-SRC-001, EDR-022; [station-commerce](modules/station-commerce.md) |
| 4 | Marta and Elisa | Sign an agreement: R$ 0.15 off the announced price per litre for Transportes Exemplo. | EDR-023 |
| 5 | Ana | Issues an authorization for Carla's vehicle, up to 200 L, valid today. | — |
| 6 | Attendant | Validates the code at the pump and confirms 150 L. | — |
| 7 | System | Creates the fueling for the fleet and the receivable for the station. The station sees only that transaction. | BR-TEN-005 |

Exceptions: another person claims the same station (both are reviewed; neither gains access automatically); Posto Modelo's plan lapses (commercial features stop; the authority and its history remain); the code is tried a second time (refused: single use); Elisa leaves the company (her role ends; the authority stays with the organization).

### JRN-11 — A plan is downgraded below the number of active vehicles

| Step | Who | What happens | Rules |
|---|---|---|---|
| 1 | Marta | Moves from a plan for 30 vehicles to one for 10 while 14 are active. | [subscriptions](modules/subscriptions.md) |
| 2 | System | Applies the change at the period end and asks which 10 stay active. | — |
| 3 | Marta | Chooses; the other four are archived, with all their history readable and exportable. | BR-AUTH-009 |
| 4 | Phone | A driver recorded a fueling for an archived vehicle before the change, offline. It arrives after. | — |
| 5 | Server | Accepts it for review with `PLAN_CHANGED`. | BR-OFFL-009 |

Result: no history is deleted; no role changes; nothing recorded in good faith is lost.

### JRN-12 — An analyst consumes a dataset by API

| Step | Who | What happens | Rules |
|---|---|---|---|
| 1 | Analyst's organization | Subscribes to a Data plan and issues an API key with a scope. | [data](modules/data.md) |
| 2 | Analyst's system | Requests a regional series; the response carries origin, period, coverage and method version. | BR-TRUTH-004, BR-SRC-001 |
| 3 | System | Counts usage against the quota. | — |
| 4 | Analyst's organization | Rotates the key; the old one works during the overlap, then stops. | — |

Result: the dataset contains only data from sources with recorded rights and nothing from any organization's private records (BR-SRC-005).

### JRN-13 — Platform support helps a company without reading its documents

| Step | Who | What happens | Rules |
|---|---|---|---|
| 1 | Marta | Asks for help with an import that fails. | — |
| 2 | Support agent | Requests a support session for the import area, for two hours. | BR-TEN-008; [back-office](modules/back-office.md) |
| 3 | Marta | Approves it in the panel. | — |
| 4 | Support agent | Sees the import batch and its errors. Documents and locations are not in the session's scope and stay closed. | BR-PRIV-021 |
| 5 | System | Records every access; the session ends by itself. Marta can read the record. | BR-AUD-003, BR-AUD-006 |
