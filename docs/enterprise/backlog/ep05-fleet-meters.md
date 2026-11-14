# EP05 — Fleet, assets and meters

Status: **IN_PROGRESS**, 2026-11-13.

This epic delivers the organization's asset register (vehicles, machines, implements, tanks, tenure, unit and cost-centre history), assignments, reservations, blocks, derived availability, the supplier registry and meter readings kept as append-only evidence with corrections, replacement, reset and rollover. The specification is [modules/fleet](../modules/fleet.md); rules are `BR-FLEET` and `BR-METER`. Tasks tagged cut C1 give the first operational cut an asset, an assignment and validated readings. Three panel screens of the specification have no task in [INDEX](INDEX.md) yet: SCR-FLEET-05 (lifecycle, tenure, transfer), SCR-FLEET-07 (suppliers) and the replacement dialog SCR-METER-04, which needs EMP-METER-005; they are named in the tasks that provide their commands.

### EMP-FLEET-001 — Asset domain: vehicles, machines, implements, tanks and compatible products
- **Meta:** module FLEET · phase F2 · cut C1 · priority P0 · type DOM · size M · state LOCAL_DONE
- **Objective:** Give the server and the app one pure model of an asset, so that kind, identifiers, tanks, accepted products and the activation check are decided the same way in Go and Kotlin instead of in each screen.
- **Rules and use cases:** BR-FLEET-001, BR-FLEET-002, BR-FLEET-003, BR-FLEET-004, BR-FLEET-005, BR-FLEET-006, BR-FLEET-007, UC-FLEET-01
- **Depends on:** EMP-PLAT-001, EMP-PLAT-009
- **Read first:** [modules/fleet](../modules/fleet.md) §3, §4 and §10.1; [04-business-rules](../04-business-rules.md) §4; [09-architecture](../09-architecture.md) §2.
- **Scope:** in: asset kinds and categories, identifier normalisation, tank and accepted-product model with the one-unit check, expected-consumption value types, usage basis, lifecycle state enumeration and the activation check · out: persistence, lifecycle commands, meters (EMP-METER-001), assignments, any read of personal vehicle data.
- **Areas:** existing: `backend/internal/modules/kernel/`, `contracts/testdata/premium/`, `domain/src/main/kotlin/com/anpfuel/domain/` · new (proposed): `backend/internal/modules/fleet/domain/`, `domain/src/main/kotlin/com/anpfuel/domain/enterprise/fleet/`, `contracts/testdata/enterprise/fleet/`
- **Steps:** 1) Write failing shared vectors for identifier normalisation, unit mismatch, bounds and activation minimum data. 2) Implement the Go domain package with the standard library and the tenancy and exact-value kernels only. 3) Implement the Kotlin model against the same vectors. 4) Declare the fuel product code as a port with test values, not a copied list.
- **Deliverable:** Two domain packages and one vector file that agree on every case.
- **Acceptance:** AC1 plates "abc-1d23" and "ABC1D23" normalise to the same key; AC2 a tank whose accepted products mix litres and cubic metres is rejected; AC3 a `VEHICLE` without a tank or a meter fails the activation check and an `IMPLEMENT` without them passes; AC4 expected consumption 8 500 m/L and capacity 275 000 mL round-trip exactly in both languages.
- **Negative cases:** no identifier at all; capacity zero, negative or above 10⁹; unknown kind or category; kind change after an operation exists; identifiers differing only by case or hyphen treated as distinct; a domain type that accepts a personal vehicle identifier.
- **Validation:** V-DOM, V-MONEY · migration: none · rollback: revert the change; no data is involved
- **Risks and assumptions:** The fuel product catalog is defined later by the fueling epic; this task relies only on the port. The category list is a hypothesis to confirm in discovery (EMP-GOV-010).
- **Done when:** Go and Kotlin pass the same asset vectors and the Go package imports nothing outside the standard library and the two kernels.
- **Local checkpoint:** LOCAL_DONE / INTEGRATION_PENDING, 2026-11-13; [task216](https://github.com/AlexandreZanata/Abastevo/issues/216), [phase215](https://github.com/AlexandreZanata/Abastevo/issues/215), cumulative PR203. Both pure domains consume all133 shared vectors; Go race/boundary/format/vet/static checks pass with98.0% statements, Kotlin143 tests pass with100% scoped lines and99.48% instructions. Typed scope, catalog-unit consistency under changing descriptors, Unicode ordering, exact quantities, privacy/defensive copies and activation facts pass independent review. [Evidence](../evidence/f2-fleet001-asset-domain.md). Runtime category registration, real meter presence and all persistence/operations remain owning tasks; F2/M1/M2/C1 is not closed.

### EMP-FLEET-002 — Asset persistence and management with per-organization identifier uniqueness
- **Meta:** module FLEET · phase F2 · cut C1 · priority P0 · type API · size M · state DONE
- **Objective:** Let a manager register, edit, activate, list and read the organization's assets, so that trips, fuelings and readings refer to a real asset instead of free text.
- **Rules and use cases:** BR-FLEET-001, BR-FLEET-003, BR-FLEET-004, BR-FLEET-008, BR-FLEET-011, BR-FLEET-015, BR-TEN-001, BR-TEN-006, UC-FLEET-01
- **Depends on:** EMP-FLEET-001, EMP-PLAT-002, EMP-AUTH-003, EMP-FUEL-001
- **Read first:** [modules/fleet](../modules/fleet.md) §3, §6.1, §7 and §10.5; [06-domain-model](../06-domain-model.md) §5; [09-architecture](../09-architecture.md) §2 to §5.
- **Scope:** in: tables for assets and tanks; create, edit with expected revision, discard a draft, activate, list with keyset pagination, read; the unit stored as the first allocation period; uniqueness of plate and fleet number among assets that are not archived; audit events · out: deactivate, dispose, archive (EMP-FLEET-003), tenure changes (EMP-FLEET-004), allocation changes (EMP-FLEET-005), meters (EMP-METER-002).
- **Areas:** existing: `backend/db/migrations/`, `backend/sqlc.yaml`, `backend/cmd/api/main.go`, `contracts/openapi/v1.yaml`, `scripts/check-manifest.txt` · new (proposed): `backend/internal/modules/fleet/application/`, `backend/internal/modules/fleet/adapters/`, `backend/db/queries/fleet/`, `contracts/testdata/enterprise/fleet/`
- **Steps:** 1) Migration with `org_id`, composite keys, the row-level-security policy and partial unique indexes on the normalised plate and fleet number. 2) Use cases behind the authorization pipeline, with audit events in the same transaction. 3) Routes under `/v1/orgs/{org_id}/fleet/assets` added to the OpenAPI contract with vectors. 4) Two-organization and authorization tests.
- **Deliverable:** Asset routes callable by the panel and present in the contract.
- **Acceptance:** AC1 create then activate returns an `ACTIVE` asset with one open allocation period; AC2 a second asset that is not archived with the same plate receives `ASSET_IDENTIFIER_TAKEN`; AC3 the same plate in another organization is accepted; AC4 an edit on a stale revision receives `CONFLICT` and changes nothing; AC5 every write produces exactly one audit event.
- **Negative cases:** another organization's asset identifier returns `NOT_FOUND`; a `DRIVER` is denied `fleet.asset.manage`; ended or suspended membership; suspended organization; two concurrent creates with the same plate leave one; a replayed operation identifier returns the first result; a query without the organization setting is denied by row-level security.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-IDEM, V-API · migration: additive tables and indexes whose names are frozen here, applied forward on a populated database · rollback: capability flag off; tables remain and the previous image keeps running
- **Risks and assumptions:** Assumes the operating-unit table of the organization epic exists; INDEX lists no direct dependency on it. The plan limit of active assets goes through the entitlement gate and is a no-op until plans exist.
- **Done when:** The route-versus-contract test and the two-organization suite pass for every asset route.

[F2 actual asset persistence](../evidence/f2-fleet002-asset-persistence.md)
is LOCAL_DONE / INTEGRATION_PENDING, 2026-11-14. Foundation6e3dde65 is backed up;
actual Meter source integration now passes full powered/route/isolation and
contract acceptance. Final ordinary PostGIS/race union2243 nodes and63 actual
company HTTP assertions pass, with zero failures/skips and no fake meter.
Full METER-002 history/assignment/review remains its own open task. EDR-109
continues real OFFL-008 review before lifecycle; full F2/M1/M2/C1/F3 is unaccepted.

### EMP-FLEET-003 — Asset lifecycle: acquisition, active, sale, write-off, return, archive
- **Meta:** module FLEET · phase F2 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Record how an asset enters and leaves service without losing its history, replacing the habit of deleting or renaming a row when a vehicle is sold.
- **Rules and use cases:** BR-FLEET-008, BR-FLEET-009, BR-FLEET-010, BR-FLEET-011, UC-FLEET-02
- **Depends on:** EMP-FLEET-002
- **Read first:** [modules/fleet](../modules/fleet.md) §4 (lifecycle rules), §6.1 and §8; [08-state-model](../08-state-model.md) §4.
- **Scope:** in: acquisition data, deactivate, reactivate, dispose with kind (sale, write-off, return to the holder), revert disposal, archive, the open-work check through ports, lifecycle events · out: cancelling reservations (EMP-FLEET-007), ending assignments (EMP-FLEET-006 consumes the disposal event), any expense or revenue from a sale, the panel screen SCR-FLEET-05 (no task in INDEX yet).
- **Areas:** existing: `backend/db/migrations/`, `contracts/openapi/v1.yaml` · new (proposed): `backend/internal/modules/fleet/application/`, `backend/internal/modules/fleet/adapters/`
- **Steps:** 1) Add acquisition and disposal columns and one command per transition, each naming the expected current state. 2) Declare the open-work port and wire a "none" adapter until trips and work orders exist. 3) Emit the lifecycle events of §9. 4) Transition tests for every row of the lifecycle table.
- **Deliverable:** Lifecycle commands in the contract, with every transition audited.
- **Acceptance:** AC1 each transition of the lifecycle table succeeds from its source state and receives `CONFLICT` from any other; AC2 dispose with open work receives `ASSET_HAS_OPEN_OPERATIONS` naming the kind of work; AC3 after archive the plate can be used by a new asset; AC4 a disposed asset's earlier records stay readable; AC5 revert disposal returns `INACTIVE` and keeps the disposal data in history.
- **Negative cases:** concurrent deactivate and dispose leave one winner; missing reason; archive straight from `ACTIVE`; another organization's asset; a role without `fleet.asset.manage`; replayed command.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-IDEM, V-API · migration: additive columns · rollback: capability flag off; columns remain
- **Risks and assumptions:** The open-work check is only as complete as its adapters; the trip epic and the maintenance epic each replace a "none" adapter and add their own test.
- **Done when:** Every row of the lifecycle table in the specification has a passing transition test and a passing refusal test.

### EMP-FLEET-004 — Tenure kinds: owned, leased, rented, third-party, loaned
- **Meta:** module FLEET · phase F2 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Let the organization state how it holds each asset and until when, so that leased, rented, third-party and borrowed assets are managed beside owned ones.
- **Rules and use cases:** BR-FLEET-012, BR-FLEET-013, UC-FLEET-03
- **Depends on:** EMP-FLEET-002
- **Read first:** [modules/fleet](../modules/fleet.md) §3 and §4 (tenure rules); [06-domain-model](../06-domain-model.md) §5 and §6.
- **Scope:** in: tenure period table with an exclusion constraint, the five kinds, counterpart name and contract reference, expected end, read of the history, `fleet.tenure.changed` · out: linking the counterpart to a supplier record (after EMP-FLEET-009), recurring lease costs (expenses epic), delivery of the expected-end alert (notification epic), lending out (a block, EMP-FLEET-007).
- **Areas:** existing: `backend/db/migrations/`, `contracts/openapi/v1.yaml` · new (proposed): `backend/internal/modules/fleet/application/`, `backend/db/queries/fleet/`
- **Steps:** 1) Migration with the period table and a backfill of one `OWNED` period for existing assets from their creation instant. 2) Command "change tenure" that closes the current period and opens the next in one transaction. 3) Contract and vectors. 4) Tests for the one-period-per-instant invariant.
- **Deliverable:** Tenure history readable and changeable through the contract.
- **Acceptance:** AC1 at any instant from acquisition exactly one tenure period applies; AC2 a new period closes the previous one at its start and rewrites nothing earlier; AC3 a kind held from others without a counterpart is refused with `VALIDATION_FAILED`; AC4 the personal data stored about a third-party holder is the name and the contract reference only.
- **Negative cases:** two concurrent changes leave no overlap and no gap; a start before the current period's start; another organization's asset; a role without `fleet.asset.manage`.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-API, V-PRIV · migration: additive table with backfill · rollback: capability flag off; table remains
- **Risks and assumptions:** The backfill assumes assets created before this task are owned; the panel must let a manager correct that. The counterpart becomes a supplier reference only when the supplier registry exists.
- **Done when:** The invariant test passes after random sequences of tenure changes.

### EMP-FLEET-005 — Unit and cost-centre allocation history
- **Meta:** module FLEET · phase F2 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Keep the history of the unit and cost centre of each asset, so that access scope and cost attribution of past records never move when the asset does.
- **Rules and use cases:** BR-FLEET-015, BR-FLEET-016, BR-AUTH-005
- **Depends on:** EMP-FLEET-002, EMP-ORG-005, EMP-ORG-006
- **Read first:** [modules/fleet](../modules/fleet.md) §4 (allocation rules); [07-permissions](../07-permissions.md) §4 and §10 example 3; [06-domain-model](../06-domain-model.md) §6.
- **Scope:** in: allocation period tables for unit and cost centre with exclusion constraints, read of both histories, a snapshot port answering "unit and cost centre of this asset at this instant" for other modules · out: the transfer command with effective instant and scheduling (EMP-FLEET-008), evaluation of unit and cost-centre scopes (EMP-AUTH-006).
- **Areas:** existing: `backend/db/migrations/`, `contracts/openapi/v1.yaml` · new (proposed): `backend/internal/modules/fleet/application/`, `backend/db/queries/fleet/`
- **Steps:** 1) Migration that turns the single unit of EMP-FLEET-002 into period rows and adds cost-centre periods. 2) Snapshot port with tests at past, present and boundary instants. 3) History read routes in the contract. 4) Reproduce example 3 of the permissions document with the port.
- **Deliverable:** A snapshot port and history routes that other modules call when they create a record.
- **Acceptance:** AC1 the port returns the unit in effect at a past instant even after later changes; AC2 an `ACTIVE` asset always has exactly one unit period; AC3 the cost centre may be absent and is never invented; AC4 a period boundary is half-open, so the instant of change belongs to the new period.
- **Negative cases:** a unit or cost centre of another organization; an archived unit; overlapping periods under concurrency; removing the only unit period of an active asset.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-API · migration: additive tables with a data move from the single column, verified on a populated database · rollback: capability flag off; the original column is kept until the next phase
- **Risks and assumptions:** Records created in cut C1 before this task already store their own snapshot, so no historical record depends on this migration.
- **Done when:** The snapshot port answers the permissions example correctly before and after a change of unit.

### EMP-FLEET-006 — Driver assignments: primary, temporary and multiple drivers
- **Meta:** module FLEET · phase F2 · cut C1 · priority P0 · type API · size M · state TODO
- **Objective:** Let supervisors say who is responsible for and who may use each asset, with periods, so that the app knows which assets a driver can choose and history shows who had the asset at any moment.
- **Rules and use cases:** BR-FLEET-018, BR-FLEET-019, BR-FLEET-020, BR-FLEET-021, BR-FLEET-022, BR-FLEET-023, BR-FLEET-024, UC-FLEET-05
- **Depends on:** EMP-FLEET-002, EMP-IDN-002
- **Read first:** [modules/fleet](../modules/fleet.md) §4 (assignment rules), §6.3 and §8; [07-permissions](../07-permissions.md) §4 (BR-AUTH-004).
- **Scope:** in: assignment table with the three kinds, exclusion constraint for primary periods, create one or many, end, replace primary, derived state, ending on employee termination and asset disposal, the "assets assigned to this member" query used by the `SELF` scope and by the reference feed · out: reservations, a pool by team, driver-profile eligibility (added by EMP-IDN-014).
- **Areas:** existing: `backend/db/migrations/`, `contracts/openapi/v1.yaml`, `backend/cmd/worker/main.go` · new (proposed): `backend/internal/modules/fleet/application/`, `backend/db/queries/fleet/`, `contracts/testdata/enterprise/fleet/`
- **Steps:** 1) Migration with periods and the exclusion constraint. 2) Commands with the authorization pipeline, audit events and notifications to assignees. 3) Handlers for the termination and disposal events that end assignments. 4) Contract, vectors and concurrency tests.
- **Deliverable:** Assignment routes and the assigned-assets query.
- **Acceptance:** AC1 state is `SCHEDULED`, `ACTIVE` or `ENDED` from server time with half-open periods; AC2 an overlapping primary receives `ASSIGNMENT_OVERLAP` and "replace" ends the previous one at the new start in the same transaction; AC3 a temporary assignment without an end is refused; AC4 a bulk command for N employees creates N assignments or none; AC5 terminating an employee ends their assignments with the cause recorded.
- **Negative cases:** employee of another organization; employee record not `ACTIVE`; two concurrent primaries leave one; a supervisor scoped to another unit is denied; ending an assignment already ended; replayed command; an ended membership still listed as assignee in the reference feed.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-IDEM, V-API, V-JOB · migration: additive table · rollback: capability flag off; table remains
- **Risks and assumptions:** Fleets where everybody uses every vehicle need many rows; the bulk command is the answer in this cut and a pool is a later decision.
- **Done when:** The concurrency test for primary periods and the event-driven ending tests pass.

### EMP-FLEET-007 — Reservations, blocks and derived availability
- **Meta:** module FLEET · phase F4 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Let managers hold an asset for planned work or block it, and give every screen one derived answer to "can this asset start an operation now", instead of a typed status that goes stale.
- **Rules and use cases:** BR-FLEET-014, BR-FLEET-025, BR-FLEET-027, BR-FLEET-028, BR-FLEET-029, BR-FLEET-030, BR-FLEET-031, BR-FLEET-032, UC-FLEET-06, UC-FLEET-07
- **Depends on:** EMP-FLEET-006
- **Read first:** [modules/fleet](../modules/fleet.md) §6.2, §6.4 and §8; [10-offline-sync](../10-offline-sync.md) §10.2; [08-state-model](../08-state-model.md) §4.
- **Scope:** in: reservation and block tables, request, confirm, cancel, block and unblock commands, time-driven `EXPIRED` and `NO_SHOW` by job and at read time, the availability read combining lifecycle, the active-use port, the maintenance-hold port, blocks and reservations, and the intake check that holds a start with `ASSET_UNAVAILABLE` · out: linking a reservation to a mission and `FULFILLED` by mission trips (EMP-TRIP-023), maintenance holds themselves, the calendar screen SCR-FLEET-06 (delivered with EMP-TRIP-022).
- **Areas:** existing: `backend/db/migrations/`, `backend/cmd/worker/main.go`, `contracts/openapi/v1.yaml` · new (proposed): `backend/internal/modules/fleet/application/`, `backend/db/queries/fleet/`, `contracts/testdata/enterprise/fleet/`
- **Steps:** 1) Migration with an exclusion constraint on confirmed windows. 2) Commands and the availability read with its list of causes. 3) Job for time-driven outcomes, safe to re-run, plus the same evaluation at read time. 4) Intake hook used by trip starts. 5) Tests for every row of the availability and reservation tables.
- **Deliverable:** Reservation and block routes, the availability read, and held starts with a precise code.
- **Acceptance:** AC1 availability follows the precedence table and lists every cause; AC2 two overlapping confirmations leave one, the other receives `RESERVATION_CONFLICT` that reveals only the window; AC3 with the job stopped, an unconfirmed reservation past its start reads as `EXPIRED`; AC4 a start by someone outside the reserving mission inside the window is held with `ASSET_UNAVAILABLE`; AC5 a block added during a trip does not interrupt it and applies to the next start.
- **Negative cases:** reservation on another organization's asset; confirmation by a member whose scope does not cover the asset; concurrent confirm and cancel; a block removed without a reason; job run twice; a reservation over an asset that left service.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-IDEM, V-API, V-JOB · migration: additive tables · rollback: capability flag off; starts are then checked against lifecycle and active trips only
- **Risks and assumptions:** The matrix in the permissions document has no row for `fleet.reservation.manage`; the roles assumed in the specification must be confirmed first. Port adapters for trips and maintenance decide how complete the answer is.
- **Done when:** Every row of the availability table and of the reservation transition table has a passing test.

### EMP-FLEET-008 — Transfer between units with an effective date
- **Meta:** module FLEET · phase F2 · cut - · priority P2 · type API · size S · state TODO
- **Objective:** Let a manager move an asset to another unit or cost centre from a stated instant, so that the move is a recorded fact instead of an edit that silently re-attributes history.
- **Rules and use cases:** BR-FLEET-016, BR-FLEET-017, UC-FLEET-04
- **Depends on:** EMP-FLEET-005
- **Read first:** [modules/fleet](../modules/fleet.md) §4 (allocation rules) and §5 (transfer use case); [04-business-rules](../04-business-rules.md) §5.
- **Scope:** in: the transfer command with effective instant and reason, scheduled transfers and their listing, cancellation before the instant, `fleet.asset.transferred` · out: re-stamping any existing record (never done), approval of transfers, the panel screen SCR-FLEET-05 (no task in INDEX yet).
- **Areas:** existing: `contracts/openapi/v1.yaml` · new (proposed): `backend/internal/modules/fleet/application/`, `contracts/testdata/enterprise/fleet/`
- **Steps:** 1) Command that closes the current allocation at the effective instant and opens the next. 2) Scope check on the current unit and existence check on the destination. 3) Listing and cancellation of scheduled transfers. 4) Tests with past, present and future instants and with a trip in progress.
- **Deliverable:** A transfer command in the contract with its event.
- **Acceptance:** AC1 after a transfer with a past effective instant, records created before it keep their stored unit; AC2 a future transfer changes nothing until its instant and then applies without a job having to run, because allocation is read by period; AC3 a trip in progress at the effective instant keeps the unit it started with; AC4 the event carries identifiers only.
- **Negative cases:** destination unit archived or of another organization; actor whose scope covers only the destination; an effective instant before the asset's first allocation; two concurrent transfers; cancelling a transfer already in effect.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-IDEM, V-API · migration: none beyond EMP-FLEET-005 · rollback: capability flag off
- **Risks and assumptions:** A past effective instant can surprise finance; the screen and the audit event state plainly that earlier records do not move.
- **Done when:** The past-instant and trip-in-progress tests pass and the audit event names the reason.

### EMP-FLEET-009 — Supplier registry
- **Meta:** module FLEET · phase F3 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Give the organization its own list of stations, workshops, lessors and other suppliers, so that fuelings, expenses, tenure and maintenance point to one record instead of free text, without touching the public catalog.
- **Rules and use cases:** BR-FLEET-034, BR-FLEET-035, BR-SRC-004, BR-TEN-005, UC-FLEET-11
- **Depends on:** EMP-PLAT-002, EMP-AUTH-003
- **Read first:** [modules/fleet](../modules/fleet.md) §3, §4 (supplier rules) and §7; [04-business-rules](../04-business-rules.md) §2; [11-privacy-security](../11-privacy-security.md) §4 (BR-PRIV-005).
- **Scope:** in: supplier table, create, edit, archive, list and read, kinds, optional CNPJ or CPF with check-digit validation through the existing kernel type, optional read-only reference to a catalog station, reference-data projection with name and kinds only · out: any write to the catalog, supplier invoices and payables (finance epic), agreements (station commerce), the panel screen SCR-FLEET-07 (no task in INDEX yet).
- **Areas:** existing: `backend/db/migrations/`, `backend/internal/modules/kernel/`, `backend/internal/modules/directory/` (read only), `contracts/openapi/v1.yaml` · new (proposed): `backend/internal/modules/fleet/application/`, `backend/db/queries/fleet/`
- **Steps:** 1) Migration with a partial unique index on the document among suppliers that are not archived. 2) Use cases with authorization and audit. 3) Read-only port to the station catalog for the optional reference. 4) Contract, vectors, isolation tests and a test that no catalog table is written.
- **Deliverable:** Supplier routes and a reference-data projection for field clients.
- **Acceptance:** AC1 a supplier with a valid alphanumeric CNPJ is accepted and one with wrong check digits is refused; AC2 a second supplier with the same document receives `SUPPLIER_DOCUMENT_TAKEN`; AC3 linking a catalog station changes no row of the catalog; AC4 the reference-data projection exposes name and kinds only; AC5 an archived supplier cannot be chosen for a new record and stays on old ones.
- **Negative cases:** supplier identifier of another organization; a member without `fleet.supplier.manage` reading the document or contact; a station identifier that does not exist; the same CNPJ in two organizations (accepted, invisible to each other); replayed create.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-IDEM, V-API, V-PRIV · migration: additive table · rollback: capability flag off; table remains
- **Risks and assumptions:** The permissions matrix has no row for `fleet.supplier.manage`; the roles assumed in the specification must be confirmed. A later merge of two catalog stations must not break the reference; it is stored as an identifier and resolved at read time.
- **Done when:** The isolation suite passes and the catalog-untouched test passes.

### EMP-FLEET-010 — Asset documents and expiry dates
- **Meta:** module FLEET · phase F6 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Let a manager attach an asset's documents and photos under a kind with a declared expiry date, so that expiry tracking has a source and the asset record never copies document content.
- **Rules and use cases:** BR-FLEET-036, BR-FLEET-007, BR-PRIV-005, BR-AUTH-011
- **Depends on:** EMP-FLEET-002, EMP-DOC-003
- **Read first:** [modules/fleet](../modules/fleet.md) §3 and §4 (documents rule); [modules/documents](../modules/documents.md) for links; [modules/maintenance](../modules/maintenance.md) for expiry tracking.
- **Scope:** in: asset document link with kind and declared expiry date, attach, replace, detach, list on the asset, one cover photo reference, a read port "documents of this asset with expiry" for the maintenance module · out: file storage and statuses (documents epic), expiry tracking, alerts and blocks (EMP-MAINT-007), driver documents (EMP-IDN-014).
- **Areas:** existing: `backend/db/migrations/`, `contracts/openapi/v1.yaml` · new (proposed): `backend/internal/modules/fleet/application/`, `backend/db/queries/fleet/`
- **Steps:** 1) Migration for the link table. 2) Commands that call the document-link use case of the document module through its port. 3) Read port for the maintenance module. 4) Contract and tests, including that opening content still needs its own permission.
- **Deliverable:** Document links on the asset and a read port for expiry tracking.
- **Acceptance:** AC1 the asset detail lists linked documents with kind and declared expiry and no document content; AC2 replacing a document keeps the previous link in history; AC3 opening the content requires `doc.document.view_content` and writes an access-log entry; AC4 the read port returns only the asking organization's links.
- **Negative cases:** linking a document of another organization; a member with `fleet.asset.read` but without the content permission opening a file; an expiry date before the document date; detaching twice.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-API, V-PRIV · migration: additive table · rollback: capability flag off; links remain
- **Risks and assumptions:** The compliance item that tracks an expiry is owned by the maintenance module; this task only supplies its input and must not grow a second tracker.
- **Done when:** The maintenance module's port test reads an asset's expiry dates and the content-permission test passes.

### EMP-FLEET-011 — Implements: attach to and detach from a powered asset
- **Meta:** module FLEET · phase F4 · cut - · priority P2 · type API · size S · state TODO
- **Objective:** Record which implement was working with which tractor or truck and when, so that trips list the implements used and an implement is never counted as working in two places.
- **Rules and use cases:** BR-FLEET-033, BR-FLEET-002, UC-FLEET-10
- **Depends on:** EMP-FLEET-002
- **Read first:** [modules/fleet](../modules/fleet.md) §3, §4 (implements rule) and §6.2.
- **Scope:** in: attachment period table with an exclusion constraint per implement, attach and detach commands, "implements attached to this asset at this instant" port for the trip module, availability of an attached implement following its powered asset · out: implement-specific meters beyond the generic meter model, checklists per implement.
- **Areas:** existing: `backend/db/migrations/`, `contracts/openapi/v1.yaml` · new (proposed): `backend/internal/modules/fleet/application/`, `backend/db/queries/fleet/`
- **Steps:** 1) Migration with the exclusion constraint. 2) Commands with kind checks. 3) Port for the trip module. 4) Contract and concurrency tests.
- **Deliverable:** Attachment routes and the port.
- **Acceptance:** AC1 an implement attached to one asset cannot be attached to another in an overlapping period (`CONFLICT`); AC2 a trip start on the powered asset lists the implements attached at that instant; AC3 an implement shows `IN_USE` while its powered asset has a trip in progress; AC4 an `IMPLEMENT` is refused as the main asset of a trip.
- **Negative cases:** attaching to an implement; attaching an asset of another organization; two concurrent attachments; detach before attach instant.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-IDEM, V-API · migration: additive table · rollback: capability flag off
- **Risks and assumptions:** Farms may swap implements several times a day; if panel entry proves too slow, an app action is a later evolution.
- **Done when:** The overlap test and the trip-port test pass.

### EMP-FLEET-012 — Panel: fleet list, detail and assignments
- **Meta:** module FLEET · phase F2 · cut C1 · priority P0 · type WEB · size M · state TODO
- **Objective:** Give managers the screens to register assets, see the fleet and assign drivers, so that the first cut can be set up without direct database work.
- **Rules and use cases:** UC-FLEET-01, UC-FLEET-05, BR-FLEET-003, BR-FLEET-004, BR-FLEET-020, BR-AUTH-013, SCR-FLEET-01, SCR-FLEET-02, SCR-FLEET-03, SCR-FLEET-04
- **Depends on:** EMP-FLEET-002, EMP-FLEET-006, EMP-PLAT-008
- **Read first:** [modules/fleet](../modules/fleet.md) §11 and §10.1; [09-architecture](../09-architecture.md) §2 (web) and §4; [07-permissions](../07-permissions.md) §11 (denial pattern).
- **Scope:** in: fleet list with filters and search, asset detail with tanks, meters and assignments, the register and edit form with activation, the assignment editor with single, bulk, end and replace-primary actions, the five shared screen states · out: lifecycle, tenure and transfer actions, reservations, suppliers, meter history and corrections (EMP-METER-007).
- **Areas:** existing: `admin-web/` (pattern only, not edited) · new (proposed): `business-web/src/fleet/`
- **Steps:** 1) List and detail on the asset routes with keyset pagination. 2) Form with field-level errors mapped from the error codes. 3) Assignment editor. 4) State tests for loading, empty, error, unavailable and forbidden. 5) Keyboard and screen-reader checks.
- **Deliverable:** Three working panel areas reachable from the navigation when the capability is on.
- **Acceptance:** AC1 a manager registers and activates an asset and assigns a driver without leaving the panel; AC2 `ASSET_IDENTIFIER_TAKEN` and missing minimum data appear beside the field; AC3 `ASSIGNMENT_OVERLAP` offers replacement from the new start; AC4 availability and lifecycle are conveyed by text, not colour alone; AC5 a member without `fleet.asset.read` sees the shared forbidden pattern with no asset data.
- **Negative cases:** stale revision on save shows the conflict and loses no typed data; a member scoped to one unit sees only that unit's assets; session expired mid-edit; another organization's asset identifier typed in the address returns not-found.
- **Validation:** V-WEB · migration: none · rollback: capability flag off hides the area
- **Risks and assumptions:** Follows the business-web decision (EMP-GOV-003); without a UI framework the form is the largest piece and may need its own shared field components.
- **Done when:** The cut C1 setup steps for a vehicle and its driver can be completed in the panel, with state tests passing.

### EMP-FLEET-013 — App: my assets and asset selection
- **Meta:** module FLEET · phase F2 · cut C1 · priority P0 · type APP · size S · state TODO
- **Objective:** Show a driver the assets they may use and let them choose one for an operation, online or offline, so that a trip or fueling starts from a known asset.
- **Rules and use cases:** UC-FLEET-08, BR-FLEET-024, BR-FLEET-027, BR-OFFL-016, BR-TEN-004, SCR-FLEET-08, SCR-FLEET-09, SCR-FLEET-10
- **Depends on:** EMP-FLEET-006, EMP-OFFL-006, EMP-IDN-018
- **Read first:** [modules/fleet](../modules/fleet.md) §8 and §11; [10-offline-sync](../10-offline-sync.md) §4 (BR-OFFL-016) and §10.3; `docs/mobile/UI_CONTAINER_GUIDELINES.md`.
- **Scope:** in: local tables for assigned assets, tanks, meters and last known readings fed by the reference pull, the list of my assets, the read-only asset detail, the reusable picker with the not-confirmed marker and the offline warning · out: starting a trip (EMP-TRIP-005), fuelings, any use of the personal `vehicle` table.
- **Areas:** existing: `app/src/main/kotlin/com/anpfuel/app/ui/`, `data/src/main/kotlin/com/anpfuel/data/`, `application/src/main/kotlin/com/anpfuel/application/` · new (proposed): `app/src/main/kotlin/com/anpfuel/app/ui/enterprise/fleet/`, `data/src/main/kotlin/com/anpfuel/data/enterprise/fleet/`, `application/src/main/kotlin/com/anpfuel/application/enterprise/fleet/`
- **Steps:** 1) Room entities in the enterprise partition keyed by account, origin and organization. 2) Use case that reads assigned assets with their confirmation time. 3) List, detail and picker composables with the five states. 4) ViewModel and state tests.
- **Deliverable:** "Meus ativos" and the asset picker in enterprise mode.
- **Acceptance:** AC1 after a reference pull the list shows exactly the assets with an active assignment; AC2 offline, the list shows its confirmation time and remains usable; AC3 an asset shown as not available can be chosen after the warning; AC4 switching organization shows the other organization's assets only; AC5 the personal vehicle list is unchanged by any action here.
- **Negative cases:** no assignment (empty state names who can assign); assignment ended on the server while offline (still listed, marked not confirmed, decided by the server later); another account on the same device sees nothing; process death while the list refreshes.
- **Validation:** V-APP, V-SYNC, V-PRIV · migration: new Room tables in the enterprise partition with a schema migration test · rollback: capability flag off hides enterprise fleet screens; local tables are left unused
- **Risks and assumptions:** Depends on the partition decision (EMP-GOV-008) through its foundations. Content cards follow the container guidelines of the app.
- **Done when:** The state tests pass and the picker returns an asset to a calling screen with no network.

### EMP-METER-001 — Meter and reading domain: append-only evidence, sources and corrections
- **Meta:** module METER · phase F2 · cut C1 · priority P0 · type DOM · size M · state LOCAL_DONE
- **Objective:** Model a meter reading as evidence with origin, instant, author and a supersession chain, so that no part of the product can treat a meter value as one overwritable number.
- **Rules and use cases:** BR-METER-001, BR-METER-002, BR-METER-003, BR-METER-004, BR-METER-005, BR-METER-006, BR-METER-007, BR-METER-008, BR-METER-013, BR-METER-014, BR-METER-021, BR-METER-022, BR-METER-023, BR-METER-024, UC-METER-01
- **Depends on:** EMP-FLEET-001
- **Read first:** [modules/fleet](../modules/fleet.md) §3, §4 (meter rules), §10.2 to §10.4; [04-business-rules](../04-business-rules.md) §4 and §5.
- **Scope:** in: meter kinds and units, the segment model (one segment per meter until EMP-METER-005), the reading value with origin and method, exact conversion from kilometre and hour text, ordering, supersession and void as pure transitions, the latest-valid selection, usage inside one segment · out: suspect flags (EMP-METER-003), replacement, reset and rollover (EMP-METER-005), persistence.
- **Areas:** existing: `contracts/testdata/premium/`, `domain/src/main/kotlin/com/anpfuel/domain/` · new (proposed): `backend/internal/modules/fleet/domain/meter/`, `domain/src/main/kotlin/com/anpfuel/domain/enterprise/meter/`, `contracts/testdata/enterprise/meter/`
- **Steps:** 1) Shared vectors for conversion, ordering, supersession and the one-segment usage, including examples B and E of the specification. 2) Go domain package. 3) Kotlin model on the same vectors. 4) Property test: the latest-valid selection never returns a voided or superseded reading.
- **Deliverable:** Meter domain packages in both languages with shared vectors.
- **Acceptance:** AC1 "184532.4" km converts to 184 532 400 m and "4212.5" h to 15 165 000 s; AC2 a reading can be superseded once and a second attempt fails; AC3 the series is ordered by `occurred_at`, then `received_at`, then identifier; AC4 odometer and hour-meter series of one asset are independent types that cannot be subtracted from each other; AC5 a missing reading is represented explicitly, never as zero.
- **Negative cases:** negative value; value above the bound; four decimal places in kilometres; hour text that does not convert to whole seconds; superseding a voided reading; a reading without author or instant.
- **Validation:** V-DOM, V-MONEY · migration: none · rollback: revert the change; no data is involved
- **Risks and assumptions:** Building segments into the first model costs little now and avoids a data migration when replacement arrives.
- **Done when:** Both languages pass the shared meter vectors.

[F2 meter domain construction](../evidence/f2-meter001-reading-domain.md)
is LOCAL_DONE / INTEGRATION_PENDING from tested/published OFFL-004 ea42ef9d. B-BR-EMP-131..136,
BUC-EMP-65..68 and EDR-087..090 precede behavior; persistence, plausibility
and meter replacement remain their owning tasks.

### EMP-METER-002 — Meter persistence and reading intake
- **Meta:** module METER · phase F2 · cut C1 · priority P0 · type DB · size M · state IN_PROGRESS
- **Objective:** Store meters and readings as insert-only rows and accept readings from the field exactly once, so that trips and fuelings can attach readings that survive retries and corrections.
- **Rules and use cases:** BR-METER-001, BR-METER-002, BR-METER-005, BR-METER-006, BR-METER-008, BR-METER-024, BR-OFFL-004, BR-TIME-002, UC-METER-01
- **Depends on:** EMP-METER-001, EMP-FLEET-002, EMP-OFFL-002
- **Read first:** [modules/fleet](../modules/fleet.md) §3, §8 and §10.3; [10-offline-sync](../10-offline-sync.md) §4 and §8; [06-domain-model](../06-domain-model.md) §5 (readings are evidence).
- **Scope:** in: tables for meters, segments and readings with the application role limited to insert on readings, the opening reading at meter creation, the sync handler `meter.reading.record`, the port other modules use to record an embedded reading inside their own transaction, per-meter serialisation, the latest-valid query, meter management on the asset routes · out: flags other than bounds (EMP-METER-003), correction and void (EMP-METER-004), the maintained projection (EMP-METER-006).
- **Areas:** existing: `backend/db/migrations/`, `backend/sqlc.yaml`, `backend/cmd/api/main.go`, `contracts/openapi/v1.yaml` · new (proposed): `backend/internal/modules/fleet/adapters/`, `backend/db/queries/fleet/`, `contracts/testdata/enterprise/meter/`
- **Steps:** 1) Migration with row-level security and a trigger or grant that forbids update and delete on readings for the application role. 2) Handler registered in the intake with the reading's own identifier. 3) Port and its transaction contract. 4) Integration tests on real PostGIS for replay, ordering and isolation.
- **Deliverable:** Readings stored and readable; a port for trip and fueling handlers.
- **Acceptance:** AC1 the same reading operation sent three times creates one row and three identical acknowledgements; AC2 an attempt to update or delete a reading as the application role fails in the database; AC3 the latest-valid query orders by `occurred_at`, not by arrival; AC4 two readings with the same value from different operations are both stored; AC5 a second meter of the same kind in service receives `METER_ALREADY_IN_SERVICE`.
- **Negative cases:** a reading for a meter of another organization; a member without an assignment recording on an asset (`FORBIDDEN` when never allowed, held when the right changed); two devices recording at once on one meter; an embedded reading whose parent transaction rolls back leaves no row; a reading declared when the meter was not in service.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-IDEM, V-SYNC, V-API · migration: additive tables with insert-only protection, applied on a populated database · rollback: capability flag off; tables remain
- **Risks and assumptions:** Insert-only protection must also hold for maintenance jobs; any repair runs through a documented operator role and is audited.
- **Done when:** The replay, insert-only and isolation tests pass on real PostGIS.

[F2 meter persistence/intake construction](../evidence/f2-meter002-persistence-intake.md)
consumes the tested/pushed Fleet source foundation6e3dde65 while keeping full
FLEET-002 powered acceptance open. B-BR-EMP-152..159, BUC-EMP-78..82 and
EDR-100..104 precede behavior; no assignment/history/review producer is faked.

### EMP-METER-003 — Reading validation: monotonicity, retroactive inserts and suspect flags
- **Meta:** module METER · phase F2 · cut C1 · priority P0 · type DOM · size S · state TODO
- **Objective:** Detect readings that contradict their neighbours or are implausible and keep them aside as suspect, so that one typing error does not corrupt distance, consumption or maintenance figures and no reading is refused in the field.
- **Rules and use cases:** BR-METER-008, BR-METER-009, BR-METER-010, BR-METER-011, BR-METER-012, BR-METER-017, BR-OFFL-006, UC-METER-02
- **Depends on:** EMP-METER-002
- **Read first:** [modules/fleet](../modules/fleet.md) §4 (meter rules), §6.5, §10.2 and §10.4 examples A and B.
- **Scope:** in: pure validation of a reading against its predecessor and successor, the rate limits per category, the future-instant check, the `RETROACTIVE` note, wiring into intake so that a failing reading is stored `SUSPECT`, exclusion of suspect readings from the latest-valid query, automatic re-evaluation of flagged neighbours · out: resolution commands (EMP-METER-004), rollover detection (EMP-METER-005), the review screen.
- **Areas:** existing: `contracts/testdata/enterprise/` (proposed by EMP-PLAT-009) · new (proposed): `backend/internal/modules/fleet/domain/meter/`, `domain/src/main/kotlin/com/anpfuel/domain/enterprise/meter/`, `contracts/testdata/enterprise/meter/`
- **Steps:** 1) Vectors from examples A and B, including the above-successor and too-fast variants. 2) Pure validator in Go and Kotlin with integer cross-multiplication only. 3) Intake wiring and the re-evaluation step. 4) Test that a suspect reading inside a trip start does not hold the trip.
- **Deliverable:** A validator shared by server and app, and suspect readings set aside at intake.
- **Acceptance:** AC1 example A passes: the inserted reading is `VALID` with `RETROACTIVE` and the two new intervals sum to 532 000 m; AC2 example B passes: the wrong reading is `SUSPECT` with `READING_BELOW_PREVIOUS` and the current value does not move; AC3 a reading of 194 400 000 m one day after 184 190 000 m is `SUSPECT` with `IMPLAUSIBLE_RATE`; AC4 an hour-meter reading that counts more than elapsed time plus 300 s is suspect; AC5 the app warns with the same validator and still saves.
- **Negative cases:** reading equal to its predecessor (valid); two readings at the same instant with different values; a reading before the opening reading; an instant later than receipt plus the skew limit; a suspect neighbour ignored when validating the next reading.
- **Validation:** V-DOM, V-MONEY, V-SQL, V-SYNC · migration: none · rollback: disable the wiring by configuration; readings are then stored without flags
- **Risks and assumptions:** Default rate limits are hypotheses per category and are configuration; a limit that is too tight fills the queue, one too loose hides errors.
- **Done when:** The shared vectors pass in both languages and the intake test shows a suspect reading excluded from the current value.

### EMP-METER-004 — Reading correction and void with audit
- **Meta:** module METER · phase F2 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Let an authorised person correct or void a reading by supersession with a reason, so that errors are fixed without losing what was originally recorded.
- **Rules and use cases:** BR-METER-011, BR-METER-012, BR-METER-013, BR-METER-014, BR-METER-015, BR-METER-016, BR-METER-026, BR-AUD-001, UC-METER-03, UC-METER-04, UC-METER-07
- **Depends on:** EMP-METER-003, EMP-PLAT-003
- **Read first:** [modules/fleet](../modules/fleet.md) §4 (meter rules), §5 and §6.5; [04-business-rules](../04-business-rules.md) §6 and §8.
- **Scope:** in: supersede, void and confirm commands, the list of suspect readings in scope, the full history read with chains, audit events with reasons, the recalculation event `meter.usage.recalculated`, the hook that routes a correction of an approved record through an amendment · out: the amendment component itself (EMP-PLAT-016), period lock checks (expenses epic), replacement and rollover (EMP-METER-005), the panel (EMP-METER-007).
- **Areas:** existing: `contracts/openapi/v1.yaml` · new (proposed): `backend/internal/modules/fleet/application/`, `contracts/testdata/enterprise/meter/`
- **Steps:** 1) Commands with expected state and the supersede-once check. 2) Re-evaluation of neighbours and the recalculation event in the same transaction. 3) History and suspect-list reads with keyset pagination. 4) Contract, vectors and tests.
- **Deliverable:** Correction routes and the suspect list.
- **Acceptance:** AC1 after a correction two rows exist, the original is `VOIDED` and linked, and the current value is the corrected one; AC2 a second correction of the same original receives `READING_ALREADY_SUPERSEDED`; AC3 a monotonicity flag cannot be confirmed away and a rate flag can, with a reason; AC4 each command writes an audit event with actor and reason and no reading content beyond identifiers; AC5 correcting one reading returns its consistent suspect neighbours to `VALID`.
- **Negative cases:** a `DRIVER` calling the command; a supervisor of another unit; another organization's reading; two reviewers correcting the same reading at once; a correction that itself breaks monotonicity (stored suspect and reported); missing reason.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-IDEM, V-API · migration: none · rollback: capability flag off; existing corrections stay
- **Risks and assumptions:** Until the amendment component exists, corrections of readings that belong to approved records are refused with a clear message rather than applied in place.
- **Done when:** Example B of the specification runs end to end through the routes with the stated states.

### EMP-METER-005 — Meter replacement, reset and rollover with cumulative usage
- **Meta:** module METER · phase F2 · cut - · priority P1 · type DOM · size M · state TODO
- **Objective:** Keep distance and hours correct when a meter is replaced, restarts its count or wraps around, so that a new panel showing 412 km does not erase 184 532 km of history.
- **Rules and use cases:** BR-METER-007, BR-METER-018, BR-METER-019, BR-METER-020, BR-METER-022, UC-METER-05, UC-METER-06
- **Depends on:** EMP-METER-003
- **Read first:** [modules/fleet](../modules/fleet.md) §4 (meter rules), §10.2, §10.3 and §10.4 examples C and D.
- **Scope:** in: segment closing and opening rules, cumulative usage and equivalent display across segments and meters, the rollover candidate rule, the unknown-final-value gap, and the three commands (replace, reset, confirm rollover) on the existing tables with audit · out: the maintained projection and the usage query for other modules (EMP-METER-006), the dialog SCR-METER-04 (no task in INDEX depends on this one yet).
- **Areas:** existing: `contracts/openapi/v1.yaml` · new (proposed): `backend/internal/modules/fleet/domain/meter/`, `backend/internal/modules/fleet/application/`, `domain/src/main/kotlin/com/anpfuel/domain/enterprise/meter/`, `contracts/testdata/enterprise/meter/`
- **Steps:** 1) Vectors from examples C and D and from a reset. 2) Pure functions for `U(t)`, usage between readings and the equivalent display. 3) Rollover candidate detection that only flags. 4) Commands and their contract. 5) Tests for the gap with an unknown final value.
- **Deliverable:** Segment-aware usage in both languages and the three commands.
- **Acceptance:** AC1 example C: cumulative usage is 1 944 000 m and usage across the replacement is 754 000 m; AC2 example D: the low reading is flagged `POSSIBLE_ROLLOVER`, and after confirmation usage is 170 000 m; AC3 a rollover is never applied without a person's command; AC4 a replacement with an unknown final value makes usage across it `INSUFFICIENT_DATA` with `METER_GAP`; AC5 monotonicity is checked inside each segment only.
- **Negative cases:** installing a meter while one of the same kind is in service; an opening value above the modulus; confirming a rollover on a meter without a modulus; a reset whose "before" value is lower than the last valid reading; concurrent replacement and reading intake on the same meter.
- **Validation:** V-DOM, V-MONEY, V-SQL, V-AUTHZ, V-API · migration: none (segments exist since EMP-METER-002) · rollback: capability flag off; segments already recorded stay and keep counting
- **Risks and assumptions:** The task type is domain, yet the three commands are included so that the rules are usable; if that proves too large, the commands split off under a letter suffix. A wrong replacement record distorts later figures, so each command shows and stores the implied usage.
- **Done when:** Examples C and D pass as shared vectors and through the commands.

### EMP-METER-006 — Current-value projection and usage-between-instants query
- **Meta:** module METER · phase F3 · cut - · priority P1 · type DB · size S · state TODO
- **Objective:** Give other modules one maintained answer for "current reading" and "usage between two instants" that names the readings it used, so that consumption, maintenance and trip distance never compute from raw displayed values.
- **Rules and use cases:** BR-METER-006, BR-METER-007, BR-METER-016, BR-TRUTH-003, BR-TRUTH-004, UC-METER-08
- **Depends on:** EMP-METER-005
- **Read first:** [modules/fleet](../modules/fleet.md) §10.3 and §12 (acceptance 10 and 14); [09-architecture](../09-architecture.md) §5 (read models).
- **Scope:** in: a projection table with current value, instant, reading and revision per meter, its update in the transaction of every reading change, a rebuild command, the usage query with anchors and `INSUFFICIENT_DATA` reasons, a port for other modules · out: consumption intervals (fueling epic), maintenance due computation, indicators.
- **Areas:** existing: `backend/db/migrations/`, `backend/cmd/ops/main.go` · new (proposed): `backend/internal/modules/fleet/application/`, `backend/db/queries/fleet/`
- **Steps:** 1) Migration for the projection. 2) Update on record, correction, void and segment change. 3) Usage query built on the domain functions. 4) Rebuild command and a test that rebuild equals incremental. 5) Port with tests for missing anchors and gaps.
- **Deliverable:** A projection that can be deleted and rebuilt, and a usage port.
- **Acceptance:** AC1 deleting and rebuilding the projection yields identical values and revisions for a fixture with corrections, a replacement and a rollover; AC2 the usage answer names both anchor readings and the projection revision; AC3 no anchor before an instant returns `INSUFFICIENT_DATA` with `NO_READING_BEFORE`; AC4 the query never interpolates; AC5 a correction increases the revision and emits the recalculation event once.
- **Negative cases:** usage asked across organizations; instants in reverse order; an asset without a meter of the asked kind; concurrent correction and query; rebuild run twice.
- **Validation:** V-SQL, V-ISO, V-IDEM, V-MONEY · migration: additive projection table, filled by the rebuild command · rollback: drop the use of the projection and fall back to the direct query; readings are untouched
- **Risks and assumptions:** The projection is a convenience, never the source of truth; any doubt is settled by rebuilding from readings.
- **Done when:** The rebuild-equals-incremental test passes on a fixture containing every example of the specification.

### EMP-METER-007 — Panel: meter history and corrections
- **Meta:** module METER · phase F2 · cut - · priority P1 · type WEB · size S · state TODO
- **Objective:** Show managers what each meter said over time and let them decide suspect readings, so that corrections are made with the neighbours and the evidence in view.
- **Rules and use cases:** UC-METER-03, UC-METER-04, UC-METER-07, BR-METER-026, BR-METER-012, SCR-METER-02, SCR-METER-03, SCR-METER-05
- **Depends on:** EMP-METER-004, EMP-PLAT-008
- **Read first:** [modules/fleet](../modules/fleet.md) §11 and §6.5; [09-architecture](../09-architecture.md) §4.
- **Scope:** in: meter history with states, flags and supersession chains, the correct-or-void dialog showing neighbours and the figures that will be recalculated, the suspect readings list, the five shared screen states · out: opening photos without the document permission, the replacement, reset and rollover dialog SCR-METER-04 until EMP-METER-005 is done (INDEX lists no such dependency for this task).
- **Areas:** existing: `admin-web/` (pattern only, not edited) · new (proposed): `business-web/src/fleet/meters/`
- **Steps:** 1) History view on the read routes. 2) Dialog with reason field and conflict handling. 3) Suspect list with the three decisions. 4) State tests and keyboard and screen-reader checks.
- **Deliverable:** Meter history, correction dialog and suspect list in the panel.
- **Acceptance:** AC1 a voided reading is shown struck through with its reason and its replacement linked; AC2 the dialog shows both neighbours before saving; AC3 `READING_ALREADY_SUPERSEDED` reloads the row without losing the typed reason; AC4 the suspect list shows only readings in the member's scope; AC5 states and flags are text, not colour alone.
- **Negative cases:** a member with `fleet.asset.read` but without `meter.reading.correct` sees history and no action; two reviewers acting on one reading; a photo link when the member lacks `doc.document.view_content`; an empty history before the opening reading.
- **Validation:** V-WEB · migration: none · rollback: capability flag off hides the area
- **Risks and assumptions:** If EMP-METER-005 lands first, its dialog is added here; otherwise it needs its own small task.
- **Done when:** A supervisor can resolve example B of the specification entirely in the panel, with state tests passing.
