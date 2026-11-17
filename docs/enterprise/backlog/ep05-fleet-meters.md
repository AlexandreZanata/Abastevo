# EP05 — Fleet, assets and meters

This epic delivers the organization's asset register (vehicles, machines, implements, tanks, tenure, unit and cost-centre history), assignments, reservations, blocks, derived availability, the supplier registry and meter readings kept as append-only evidence with corrections, replacement, reset and rollover. The specification is [modules/fleet](../modules/fleet.md); rules are `BR-FLEET` and `BR-METER`. Tasks tagged cut C1 give the first operational cut an asset, an assignment and validated readings. Three panel screens of the specification have no task in [INDEX](INDEX.md) yet: SCR-FLEET-05 (lifecycle, tenure, transfer), SCR-FLEET-07 (suppliers) and the replacement dialog SCR-METER-04, which needs EMP-METER-005; they are named in the tasks that provide their commands.

### EMP-FLEET-004 — Tenure kinds: owned, leased, rented, third-party, loaned
- **Meta:** module FLEET · phase F2 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Let the organization state how it holds each asset and until when, so that leased, rented, third-party and borrowed assets are managed beside owned ones.
- **Rules and use cases:** BR-FLEET-012, BR-FLEET-013, UC-FLEET-03
- **Depends on:** EMP-FLEET-002
- **Read first:** [modules/fleet](../modules/fleet.md) §3 and §4 (tenure rules); [06-domain-model](../domain-model.md) §5 and §6.
- **Scope:** in: tenure period table with an exclusion constraint, the five kinds, counterpart name and contract reference, expected end, read of the history, `fleet.tenure.changed` · out: linking the counterpart to a supplier record (after EMP-FLEET-009), recurring lease costs (expenses epic), delivery of the expected-end alert (notification epic), lending out (a block, EMP-FLEET-007).
- **Areas:** existing: `backend/db/migrations/`, `contracts/openapi/v1.yaml` · new (proposed): `backend/internal/modules/fleet/application/`, `backend/db/queries/fleet/`
- **Steps:** 1) Migration with the period table and a backfill of one `OWNED` period for existing assets from their creation instant. 2) Command "change tenure" that closes the current period and opens the next in one transaction. 3) Contract and vectors. 4) Tests for the one-period-per-instant invariant.
- **Deliverable:** Tenure history readable and changeable through the contract.
- **Acceptance:** AC1 at any instant from acquisition exactly one tenure period applies; AC2 a new period closes the previous one at its start and rewrites nothing earlier; AC3 a kind held from others without a counterpart is refused with `VALIDATION_FAILED`; AC4 the personal data stored about a third-party holder is the name and the contract reference only.
- **Negative cases:** two concurrent changes leave no overlap and no gap; a start before the current period's start; another organization's asset; a role without `fleet.asset.manage`.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-API, V-PRIV · migration: additive table with backfill · rollback: capability flag off; table remains
- **Risks and assumptions:** The backfill assumes assets created before this task are owned; the panel must let a manager correct that. The counterpart becomes a supplier reference only when the supplier registry exists.
- **Done when:** The invariant test passes after random sequences of tenure changes.

### EMP-FLEET-008 — Transfer between units with an effective date
- **Meta:** module FLEET · phase F2 · cut - · priority P2 · type API · size S · state TODO
- **Objective:** Let a manager move an asset to another unit or cost centre from a stated instant, so that the move is a recorded fact instead of an edit that silently re-attributes history.
- **Rules and use cases:** BR-FLEET-016, BR-FLEET-017, UC-FLEET-04
- **Depends on:** EMP-FLEET-005
- **Read first:** [modules/fleet](../modules/fleet.md) §4 (allocation rules) and §5 (transfer use case); [04-business-rules](../business-rules.md) §5.
- **Scope:** in: the transfer command with effective instant and reason, scheduled transfers and their listing, cancellation before the instant, `fleet.asset.transferred` · out: re-stamping any existing record (never done), approval of transfers, the panel screen SCR-FLEET-05 (no task in INDEX yet).
- **Areas:** existing: `contracts/openapi/v1.yaml` · new (proposed): `backend/internal/modules/fleet/application/`, `contracts/testdata/enterprise/fleet/`
- **Steps:** 1) Command that closes the current allocation at the effective instant and opens the next. 2) Scope check on the current unit and existence check on the destination. 3) Listing and cancellation of scheduled transfers. 4) Tests with past, present and future instants and with a trip in progress.
- **Deliverable:** A transfer command in the contract with its event.
- **Acceptance:** AC1 after a transfer with a past effective instant, records created before it keep their stored unit; AC2 a future transfer changes nothing until its instant and then applies without a job having to run, because allocation is read by period; AC3 a trip in progress at the effective instant keeps the unit it started with; AC4 the event carries identifiers only.
- **Negative cases:** destination unit archived or of another organization; actor whose scope covers only the destination; an effective instant before the asset's first allocation; two concurrent transfers; cancelling a transfer already in effect.
- **Validation:** V-SQL, V-ISO, V-AUTHZ, V-IDEM, V-API · migration: none beyond EMP-FLEET-005 · rollback: capability flag off
- **Risks and assumptions:** A past effective instant can surprise finance; the screen and the audit event state plainly that earlier records do not move. FLEET-005 period guards allow only closing an open period once and never delete; a scheduled-transfer cancellation needs an additive guard change here.
- **Done when:** The past-instant and trip-in-progress tests pass and the audit event names the reason.

### EMP-FLEET-009 — Supplier registry
- **Meta:** module FLEET · phase F3 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Give the organization its own list of stations, workshops, lessors and other suppliers, so that fuelings, expenses, tenure and maintenance point to one record instead of free text, without touching the public catalog.
- **Rules and use cases:** BR-FLEET-034, BR-FLEET-035, BR-SRC-004, BR-TEN-005, UC-FLEET-11
- **Depends on:** EMP-PLAT-002, EMP-AUTH-003
- **Read first:** [modules/fleet](../modules/fleet.md) §3, §4 (supplier rules) and §7; [04-business-rules](../business-rules.md) §2; [11-privacy-security](../privacy-security.md) §4 (BR-PRIV-005).
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

### EMP-FLEET-012 — Panel: fleet list, detail and assignments
- **Meta:** module FLEET · phase F2 · cut C1 · priority P0 · type WEB · size M · state TODO
- **Objective:** Give managers the screens to register assets, see the fleet and assign drivers, so that the first cut can be set up without direct database work.
- **Rules and use cases:** UC-FLEET-01, UC-FLEET-05, BR-FLEET-003, BR-FLEET-004, BR-FLEET-020, BR-AUTH-013, SCR-FLEET-01, SCR-FLEET-02, SCR-FLEET-03, SCR-FLEET-04
- **Depends on:** EMP-FLEET-002, EMP-FLEET-006, EMP-PLAT-008
- **Read first:** [modules/fleet](../modules/fleet.md) §11 and §10.1; [09-architecture](../architecture.md) §2 (web) and §4; [07-permissions](../permissions.md) §11 (denial pattern).
- **Scope:** in: fleet list with filters and search, asset detail with tanks, meters and assignments, the register and edit form with activation, the assignment editor with single, bulk, end and replace-primary actions, the five shared screen states · out: lifecycle, tenure and transfer actions, reservations, suppliers, meter history and corrections (EMP-METER-007).
- **Areas:** existing: `admin-web/` (pattern only, not edited) · new (proposed): `business-web/src/fleet/`
- **Steps:** 1) List and detail on the asset routes with keyset pagination. 2) Form with field-level errors mapped from the error codes. 3) Assignment editor. 4) State tests for loading, empty, error, unavailable and forbidden. 5) Keyboard and screen-reader checks.
- **Deliverable:** Three working panel areas reachable from the navigation when the capability is on.
- **Acceptance:** AC1 a manager registers and activates an asset and assigns a driver without leaving the panel; AC2 `ASSET_IDENTIFIER_TAKEN` and missing minimum data appear beside the field; AC3 `ASSIGNMENT_OVERLAP` offers replacement from the new start; AC4 availability and lifecycle are conveyed by text, not colour alone; AC5 a member without `fleet.asset.read` sees the shared forbidden pattern with no asset data.
- **Negative cases:** stale revision on save shows the conflict and loses no typed data; a member scoped to one unit sees only that unit's assets; session expired mid-edit; another organization's asset identifier typed in the address returns not-found.
- **Validation:** V-WEB · migration: none · rollback: capability flag off hides the area
- **Risks and assumptions:** Follows the business-web decision (EMP-GOV-003); without a UI framework the form is the largest piece and may need its own shared field components.
- **Done when:** The cut C1 setup steps for a vehicle and its driver can be completed in the panel, with state tests passing.

### EMP-METER-007 — Panel: meter history and corrections
- **Meta:** module METER · phase F2 · cut - · priority P1 · type WEB · size S · state TODO
- **Objective:** Show managers what each meter said over time and let them decide suspect readings, so that corrections are made with the neighbours and the evidence in view.
- **Rules and use cases:** UC-METER-03, UC-METER-04, UC-METER-07, BR-METER-026, BR-METER-012, SCR-METER-02, SCR-METER-03, SCR-METER-05
- **Depends on:** EMP-METER-004, EMP-PLAT-008
- **Read first:** [modules/fleet](../modules/fleet.md) §11 and §6.5; [09-architecture](../architecture.md) §4.
- **Scope:** in: meter history with states, flags and supersession chains, the correct-or-void dialog showing neighbours and the figures that will be recalculated, the suspect readings list, the five shared screen states · out: opening photos without the document permission, the replacement, reset and rollover dialog SCR-METER-04 until EMP-METER-005 is done (INDEX lists no such dependency for this task).
- **Areas:** existing: `admin-web/` (pattern only, not edited) · new (proposed): `business-web/src/fleet/meters/`
- **Steps:** 1) History view on the read routes. 2) Dialog with reason field and conflict handling. 3) Suspect list with the three decisions. 4) State tests and keyboard and screen-reader checks.
- **Deliverable:** Meter history, correction dialog and suspect list in the panel.
- **Acceptance:** AC1 a voided reading is shown struck through with its reason and its replacement linked; AC2 the dialog shows both neighbours before saving; AC3 `READING_ALREADY_SUPERSEDED` reloads the row without losing the typed reason; AC4 the suspect list shows only readings in the member's scope; AC5 states and flags are text, not colour alone.
- **Negative cases:** a member with `fleet.asset.read` but without `meter.reading.correct` sees history and no action; two reviewers acting on one reading; a photo link when the member lacks `doc.document.view_content`; an empty history before the opening reading.
- **Validation:** V-WEB · migration: none · rollback: capability flag off hides the area
- **Risks and assumptions:** If EMP-METER-005 lands first, its dialog is added here; otherwise it needs its own small task.
- **Done when:** A supervisor can resolve example B of the specification entirely in the panel, with state tests passing.
