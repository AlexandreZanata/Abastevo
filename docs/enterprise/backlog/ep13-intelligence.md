# EP13 — Intelligence, findings and reports

Status: **PLANNED**, 2026-11-10.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from the [intelligence, findings and reports](../modules/intelligence.md) specification, [validation scenarios](../15-validation.md) and [privacy and security](../11-privacy-security.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

Indicators and findings use scoped source records and their recorded methods. Organization exports retain their independent permissions, audit, expiry and real-data gates, including when requested for closure.

### EMP-KPI-001 — Indicator registry: formula, inputs, version and insufficiency states
- **Meta:** module KPI · phase F6 · cut - · priority P1 · type DOM · size M · state TODO
- **Objective:** Keep indicator definitions and missing-data semantics in a single registry.
- **Rules and use cases:** BR-KPI-001, BR-KPI-002, BR-KPI-003, BR-KPI-007, BR-KPI-015, BR-TRUTH-003
- **Depends on:** EMP-PLAT-009
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§4 and 10, [shared business rules](../04-business-rules.md) §4
- **Scope:** in: identifiers, formula basis, inclusion, units, versions and insufficiency · out: persistence and screen calculations
- **Areas:** existing: exact-value conventions and shared vectors · proposed: pure indicator registry
- **Steps:** Pin formula and coverage metadata. Test missing inputs, zero denominators, version changes and exact values.
- **Deliverable:** A pure indicator registry shared by reporting consumers.
- **Acceptance:** Each indicator has a declared basis and version. Missing or insufficient data remains a reasoned result without a number.
- **Negative cases:** An unknown formula, a zero denominator displayed as zero, or a view inventing an independent calculation.
- **Validation:** V-DOM, V-MONEY · migration: none · rollback: remove unused registrations before activation
- **Risks and assumptions:** Every consumer uses the method and scope emitted by the registry.
- **Done when:** Known, insufficient and versioned method vectors pass.

### EMP-KPI-002 — Spend and volume read models by dimension and period
- **Meta:** module KPI · phase F6 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Build scoped spend and volume projections by documented dimension and period.
- **Rules and use cases:** BR-KPI-002, BR-KPI-006, BR-KPI-007, BR-KPI-008, BR-KPI-009, BR-KPI-010, BR-KPI-011
- **Depends on:** EMP-KPI-001, EMP-EXP-002, EMP-FUEL-002
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§4 and 10.2–10.3, [shared business rules](../04-business-rules.md) §§4–5
- **Scope:** in: spend and volume read models with reporting-day attribution · out: recognizing a new cost
- **Areas:** existing: expense, fueling, organization and time inputs · proposed: scoped read projections
- **Steps:** Pin inclusion, source revisions, scope and dimension grouping. Recompute affected periods from approved facts and label the pending part explicitly.
- **Deliverable:** Versioned spend and volume projections with query evidence.
- **Acceptance:** Periods and dimensions use the recorded facts. Exact amounts stay separate by compatible unit and role scope.
- **Negative cases:** Duplicate cost, foreign record, changed assignment rewriting history, mixed units, or pending facts included without a label.
- **Validation:** V-SQL, V-ISO, V-IDEM, V-MONEY · migration: additive projection storage if needed · rollback: stop recompute and retain prior evidence
- **Risks and assumptions:** The owning modules remain the source of truth for revisions and costs.
- **Done when:** Scoped projection and recomputation tests pass on real PostGIS.

### EMP-KPI-003 — Consumption and cost-per-distance or hour indicators
- **Meta:** module KPI · phase F6 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Compute consumption and operating ratios over matching observed inputs.
- **Rules and use cases:** BR-KPI-003, BR-KPI-004, BR-KPI-005, BR-KPI-006, BR-KPI-009, BR-KPI-016, BR-MONEY-008
- **Depends on:** EMP-KPI-001, EMP-FUEL-008, EMP-FUEL-010
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§4 and 10.2–10.5, [validation scenarios](../15-validation.md) §4
- **Scope:** in: consumption and cost-per-distance or hour using compatible observed windows · out: estimating missing usage
- **Areas:** existing: fueling intervals, meter readings and exact-value registry · proposed: scoped ratio projections
- **Steps:** Compare ratios of sums across a mixed sample. Pin denominator, missing coverage, measurement gap and sample-size cases.
- **Deliverable:** Scoped ratio results with exact basis and coverage.
- **Acceptance:** Numerator and denominator use the same approved records and window. Small samples and missing inputs are visible.
- **Negative cases:** Average of per-asset ratios, mixed fuels, empty window reported as zero or whole-period costs divided by a partial usage window.
- **Validation:** V-SQL, V-MONEY, V-ISO, V-IDEM · migration: additive with replay evidence · rollback: stop derived refresh and preserve source facts
- **Risks and assumptions:** These indicators describe observed inputs and carry their coverage.
- **Done when:** Real PostGIS and exact-value vectors prove grouping, insufficient data and revision behavior.

### EMP-KPI-004 — Total operating cost, maintenance and availability indicators
- **Meta:** module KPI · phase F6 · cut - · priority P2 · type DB · size M · state TODO
- **Objective:** Compute operating cost, maintenance and availability from approved source facts.
- **Rules and use cases:** BR-KPI-002, BR-KPI-003, BR-KPI-007, BR-KPI-008, BR-KPI-010, BR-KPI-011, BR-REV-009
- **Depends on:** EMP-KPI-002, EMP-MAINT-006, EMP-FLEET-007
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§4 and 10.2, [maintenance specification](../modules/maintenance.md)
- **Scope:** in: cost components, maintenance and calendar availability projections · out: inventing history or creating expenses
- **Areas:** existing: fleet availability, maintenance facts and spend projections · proposed: versioned operating projections
- **Steps:** Exercise included cost components and actual active windows. Recalculate after amendments or voids and expose exclusions.
- **Deliverable:** Scoped indicators with exact basis and revision.
- **Acceptance:** Only eligible recorded facts count. Unknown availability remains insufficient and linked costs are counted once.
- **Negative cases:** Repair cost recognized twice, missing history assumed available, revoked rows included or a projected figure detached from its basis.
- **Validation:** V-SQL, V-ISO, V-IDEM, V-MONEY · migration: additive if required · rollback: stop projection changes and retain source records
- **Risks and assumptions:** Availability follows the module's calendar-time definition and reporting basis.
- **Done when:** Cost, availability and amendment cases pass on scoped fixtures.

### EMP-KPI-005 — Price comparison engine with a methodology snapshot
- **Meta:** module KPI · phase F6 · cut - · priority P1 · type DOM · size M · state TODO
- **Objective:** Compare a fueling against a matching reference and preserve the methodology.
- **Rules and use cases:** BR-KPI-012, BR-KPI-013, BR-KPI-014, BR-KPI-015, BR-SRC-001, BR-TRUTH-005, UC-KPI-03, VAL-15
- **Depends on:** EMP-KPI-001, EMP-FUEL-014
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§4–5 and 10.6–10.7, [validation scenarios](../15-validation.md) §4
- **Scope:** in: reference comparability and exact difference · out: blending sources or claiming losses or savings
- **Areas:** existing: source-specific reference snapshots and exact-value registry · proposed: pure comparison engine
- **Steps:** Pin the comparable and incompatible examples. Preserve source, coverage, method parameters and revision in each result.
- **Deliverable:** A deterministic comparison contract and method snapshot.
- **Acceptance:** Only matching product, unit, place, period and price conditions produce a difference. Each comparison identifies its source.
- **Negative cases:** Cash and credit mixed, source blending, missing reference interpreted as zero or a difference presented as proven loss.
- **Validation:** V-DOM, V-MONEY · migration: none in the pure engine · rollback: disable comparison while preserving prior snapshots
- **Risks and assumptions:** Later reference changes do not rewrite recorded analyses.
- **Done when:** The comparable and insufficient vectors pass with exact arithmetic and documented wording.

### EMP-KPI-006 — Panel: overview dashboard and drill-downs
- **Meta:** module KPI · phase F6 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Present scoped indicator values and explain their source records in the company panel.
- **Rules and use cases:** BR-KPI-003, BR-KPI-007, BR-KPI-008, BR-KPI-011, UC-KPI-01, UC-KPI-02
- **Depends on:** EMP-KPI-002, EMP-KPI-003, EMP-PLAT-008
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§5 and 11, [permission matrix](../07-permissions.md) §4
- **Scope:** in: approved dashboard, scope labels and drill-down · out: computing values in the client and revealing unpermitted records
- **Areas:** existing: /empresas/ scaffold and indicator APIs · proposed: scoped overview and detail screens
- **Steps:** Exercise approved versus pending values, insufficient data, superseded revisions and drill-down permission in the normal panel.
- **Deliverable:** An accessible indicator overview and scoped drill-down.
- **Acceptance:** The panel displays value or reason, scope, method and freshness. Record links respect the owning read permission.
- **Negative cases:** Foreign data after context switch, narrow total labelled organization-wide, hidden basis or insufficient value rendered as zero.
- **Validation:** V-WEB, V-AUTHZ, V-ISO · migration: none · rollback: hide views without changing source data
- **Risks and assumptions:** The API is authoritative for values and the records a member can open.
- **Done when:** Scope, negative states and keyboard-accessible drill-down pass.

### EMP-FIND-001 — Findings domain: types, severity, evidence and lifecycle
- **Meta:** module FIND · phase F6 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Represent reviewable findings with bounded evidence, severity and lifecycle.
- **Rules and use cases:** BR-FIND-001, BR-FIND-002, BR-FIND-003, BR-FIND-004, BR-FIND-006, BR-FIND-008, BR-FIND-009
- **Depends on:** EMP-PLAT-003, EMP-AUTH-003
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§3–4, 6 and 10.8, [permission matrix](../07-permissions.md) §4
- **Scope:** in: catalog, persistent evidence references and finding lifecycle · out: automatic record sanctions or detection jobs
- **Areas:** existing: organization audit and authorization · proposed: scoped finding model and repository
- **Steps:** Pin transitions, justification and same-subject races. Persist the evidence basis and verify at most one open finding per scoped subject and type.
- **Deliverable:** A scoped finding repository and lifecycle contract.
- **Acceptance:** A finding describes a difference, preserves its basis and leaves the source record unchanged.
- **Negative cases:** Duplicate open finding, missing justification, foreign evidence, accusations from severity or automatic source mutation.
- **Validation:** V-DOM, V-SQL, V-ISO, V-IDEM, V-PRIV · migration: additive with populated rehearsal · rollback: freeze new findings and preserve decisions
- **Risks and assumptions:** Evidence links require the owning permissions. Do not embed private originals in finding text.
- **Done when:** State, concurrency and scope checks pass with synthetic examples.

### EMP-FIND-002 — Detection batch 1: duplicate fueling, incoherent reading, tank over capacity
- **Meta:** module FIND · phase F6 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Detect documented duplicate fueling, incoherent reading and tank-capacity signals.
- **Rules and use cases:** BR-FIND-001, BR-FIND-002, BR-FIND-003, BR-FIND-004, BR-FIND-010, BR-FIND-012, FT-001, FT-002, FT-003, UC-FIND-01
- **Depends on:** EMP-FIND-001, EMP-FUEL-006, EMP-METER-003, EMP-FUEL-005
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§5 and 10.8–10.9, [validation scenarios](../15-validation.md) §4
- **Scope:** in: first catalog detector batch and deduplication · out: automatic void, denial or investigation claims
- **Areas:** existing: accepted fueling and meter events · proposed: first scoped detector workers
- **Steps:** Pin the catalog thresholds and required inputs. Test legitimate replay, missing input and repeated jobs.
- **Deliverable:** Versioned and retry-safe findings for the first catalog batch.
- **Acceptance:** Only an accepted source with enough evidence can raise a signal. Repeated evaluation of the same open subject adds nothing.
- **Negative cases:** Transport replay treated as duplicate fueling, held operation analysed, missing capacity guessed or foreign input compared.
- **Validation:** V-SQL, V-JOB, V-IDEM, V-ISO, V-MONEY · migration: reuse finding storage · rollback: disable detector types and retain evidence
- **Risks and assumptions:** Signals go to review and do not imply fraud or duplicate payment.
- **Done when:** Catalog, retry and scope cases pass without changing source records.

### EMP-FIND-003 — Detection batch 2: consumption outlier, document divergence, price above reference
- **Meta:** module FIND · phase F6 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Detect documented consumption, document and price differences using sufficient evidence.
- **Rules and use cases:** BR-FIND-001, BR-FIND-002, BR-FIND-003, BR-FIND-004, BR-FIND-012, BR-KPI-012, BR-KPI-016, FT-004, FT-005, FT-006, UC-FIND-01
- **Depends on:** EMP-FIND-001, EMP-FUEL-008, EMP-DOC-010, EMP-KPI-005
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§5 and 10.6–10.9, [validation scenarios](../15-validation.md) §4
- **Scope:** in: second catalog detector batch and explainable evidence · out: claiming misconduct or analysing held work
- **Areas:** existing: consumption intervals, reconciliation and stored price comparisons · proposed: additional scoped detector workers
- **Steps:** Pin accepted samples, comparable reference conditions and divergence cases. Verify that insufficient evidence produces no finding.
- **Deliverable:** Versioned findings with explicit baseline and rule inputs.
- **Acceptance:** A detector uses the approved source and threshold and stores the method version. Repeated runs converge.
- **Negative cases:** Small sample used as baseline, incompatible source triggers, human review treated as fiscal proof, or a held operation analysed.
- **Validation:** V-SQL, V-JOB, V-IDEM, V-ISO, V-MONEY, V-PRIV · migration: reuse finding storage · rollback: pause detector batch without rewriting history
- **Risks and assumptions:** The stored difference is a review signal. A decision about the source stays in the owning module.
- **Done when:** Scoped outlier, reconciliation and price test vectors pass with failure and retry cases.

### EMP-FIND-004 — Findings review workflow and panel
- **Meta:** module FIND · phase F6 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Let authorized company reviewers take and decide a finding without editing its source record.
- **Rules and use cases:** BR-FIND-005, BR-FIND-006, BR-FIND-007, BR-FIND-008, BR-FIND-009, BR-AUD-001, UC-FIND-02
- **Depends on:** EMP-FIND-001, EMP-PLAT-008
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§5–6 and 11, [permission matrix](../07-permissions.md) §4
- **Scope:** in: reviewer queue, assignment, decision and visible justification · out: automatically applying amendments or voids
- **Areas:** existing: /empresas/ panel and finding repository · proposed: scoped review endpoints and views
- **Steps:** Exercise assignment, denied self-review, required justification, simultaneous decisions and read permissions on linked evidence.
- **Deliverable:** An accessible scoped finding review workflow.
- **Acceptance:** One allowed reviewer decides against a current finding. The reason and actor are audited and source actions stay separate.
- **Negative cases:** Author or subject resolves their finding, foreign evidence appears, stale review succeeds or a decision mutates the source.
- **Validation:** V-WEB, V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: preserve prior findings · rollback: hide review and retain outcomes
- **Risks and assumptions:** The catalog explains what each signal does not establish.
- **Done when:** Normal review and denied or concurrent decisions pass across API and panel.

### EMP-FIND-005 — Reprocessing and analysis versioning
- **Meta:** module FIND · phase F6 · cut - · priority P1 · type JOB · size S · state TODO
- **Objective:** Reprocess findings with identifiable run metadata while preserving decisions.
- **Rules and use cases:** BR-FIND-002, BR-FIND-010, BR-FIND-011, BR-FIND-012, BR-TRUTH-004, UC-FIND-04
- **Depends on:** EMP-FIND-002, EMP-FIND-003
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§4–6 and 10, [architecture](../09-architecture.md) §7
- **Scope:** in: versioned run, bounded range and supersession · out: rewriting decided findings
- **Areas:** existing: detector workers and job deduplication · proposed: scoped reprocessing and run history
- **Steps:** Test changed methods, removed input, overlapping runs and interrupted execution. Record the run that changed each result.
- **Deliverable:** An auditable and retry-safe reprocessing job.
- **Acceptance:** Undecided findings no longer supported are superseded with a run reference. Decided findings retain their outcome.
- **Negative cases:** Duplicate active run, unbounded period, stale run replacing newer results or rewriting a confirmed decision.
- **Validation:** V-SQL, V-JOB, V-ISO, V-IDEM · migration: additive run metadata · rollback: stop reprocessing and preserve the run history
- **Risks and assumptions:** Configuration changes apply as documented and do not silently rewrite evidence.
- **Done when:** Failure, concurrent run and replay tests yield one consistent version history.

### EMP-RPT-001 — Asynchronous export infrastructure: job, private file and expiring link
- **Meta:** module RPT · phase F3 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Provide scoped asynchronous exports with private expiring delivery.
- **Rules and use cases:** BR-RPT-001, BR-RPT-002, BR-RPT-004, BR-RPT-005, BR-RPT-009, BR-PRIV-004, UC-RPT-01
- **Depends on:** EMP-PLAT-006, EMP-PLAT-010, EMP-PRIV-005
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§4–6 and 10, [architecture](../09-architecture.md) §§7 and 9, [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: export job lifecycle, bounded work, private file and audited capability · out: defining every export format
- **Areas:** existing: scoped jobs, private storage and access audit · proposed: export dispatcher and private delivery
- **Steps:** Prove job and file state through permission loss, expiry, duplicate work and storage failure. Separate ready files from provisional output.
- **Deliverable:** A scoped export infrastructure with expiry and audit.
- **Acceptance:** The worker rechecks authority, publishes only complete private results and issues a short-lived requester-bound API capability.
- **Negative cases:** Wrong requester, permission revoked before processing, incomplete file served, expired link works or unbounded work truncates the result.
- **Validation:** V-API, V-SQL, V-JOB, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: additive export state · rollback: stop new jobs and keep existing data private
- **Risks and assumptions:** Use the existing job and object store. Keep worker and download authority independently checked.
- **Done when:** Normal request, processing, retry, expiry and audited download pass on isolated storage and database fixtures.

### EMP-RPT-002 — CSV and spreadsheet exports for operational lists
- **Meta:** module RPT · phase F3 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Export operational lists using scoped, exact and non-executable CSV or spreadsheet data.
- **Rules and use cases:** BR-RPT-002, BR-RPT-003, BR-RPT-006, BR-RPT-007, BR-RPT-008, BR-RPT-010, BR-RPT-011, UC-RPT-01
- **Depends on:** EMP-RPT-001, EMP-AUTH-011
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§4–5 and 10.10, [permission matrix](../07-permissions.md) §4
- **Scope:** in: list filters, approved tabular formats, manifest basis and exact values · out: arbitrary spreadsheets or widened access
- **Areas:** existing: export jobs and standard CSV writer precedent · proposed: scoped list serializers
- **Steps:** Pin deterministic row ordering, exact units, sensitive column omissions and inert text encoding. Review any spreadsheet dependency before offering the format.
- **Deliverable:** Authorized operational list exports with reproducible fixtures.
- **Acceptance:** Files include the documented basis. Text remains text and excluded columns are reported without exposing their values.
- **Negative cases:** Formula interpretation, foreign rows, stale requester scope, format advertised before review or incomplete output returned as ready.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-MONEY, V-PRIV · migration: none beyond export infrastructure · rollback: disable new formats
- **Risks and assumptions:** A format that has not been admitted stays unavailable. Prefer existing standard-library options.
- **Done when:** Exact, scoped and inert output passes round-trip fixtures.

### EMP-RPT-003 — PDF reports: trip logbook, fueling statement and period summary
- **Meta:** module RPT · phase F6 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Render authorized human-readable trip and fueling reports from the same basis as the panel.
- **Rules and use cases:** BR-RPT-002, BR-RPT-003, BR-RPT-007, BR-RPT-008, BR-RPT-011, BR-RPT-015, UC-RPT-02
- **Depends on:** EMP-RPT-001, EMP-TRIP-004, EMP-FUEL-003
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§4–5 and 10.10, [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: trip logbook, fueling statement and period summary PDF where approved · out: payroll or working-hours claims
- **Areas:** existing: export job pipeline and report input ports · proposed: reviewed PDF renderer
- **Steps:** Verify dependency admission, basis fields, displayed rounding and operational wording on every logbook page. Exercise unavailable-format and failed-render cases.
- **Deliverable:** A private PDF report for each admitted report type.
- **Acceptance:** The displayed totals match the scoped method and period. Missing data is explicit and logbooks carry the operational wording.
- **Negative cases:** Working hours computed, foreign or sensitive content exposed, unreviewed format enabled, or broken rendering delivered as ready.
- **Validation:** V-JOB, V-AUTHZ, V-ISO, V-MONEY, V-PRIV · migration: none beyond job contract · rollback: disable PDF and keep other authorized exports
- **Risks and assumptions:** The file-format decision is required before this capability is exposed.
- **Done when:** Authorized reports, fallback and failure cases pass with synthetic records.

### EMP-RPT-004 — Scheduled reports and delivery
- **Meta:** module RPT · phase F6 · cut - · priority P2 · type JOB · size S · state TODO
- **Objective:** Run scheduled reports under each recipient's current permission and scope.
- **Rules and use cases:** BR-RPT-002, BR-RPT-004, BR-RPT-014, BR-AUTH-014, UC-RPT-03
- **Depends on:** EMP-RPT-002, EMP-NOTIF-001
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§4–5 and 6, [architecture](../09-architecture.md) §7
- **Scope:** in: approved schedule, per-recipient export and in-app link · out: sending report attachments or wider recipient data
- **Areas:** existing: export dispatcher, organization notices and time conventions · proposed: scoped report schedule runner
- **Steps:** Exercise owner and recipient permission loss, suspended organizations and retried triggers. Deduplicate each scheduled job.
- **Deliverable:** A bounded schedule that produces authorized recipient-specific export jobs.
- **Acceptance:** The run skips revoked recipients and pauses when the owner no longer has authority. Notifications carry only an authorized panel link.
- **Negative cases:** Recipients share the owner's scope, revoked user gets a file, duplicate job per run or a suspended organization still schedules delivery.
- **Validation:** V-SQL, V-JOB, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: additive schedule state · rollback: pause schedules and preserve history
- **Risks and assumptions:** External channels still require their own approval.
- **Done when:** Scoped scheduling, revocation and retry cases pass with a controlled clock.

### EMP-RPT-005 — Full organization data export
- **Meta:** module RPT · phase F6 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Produce a complete, verifiable export for an authorized organization and its closure workflow.
- **Rules and use cases:** BR-RPT-002, BR-RPT-003, BR-RPT-004, BR-RPT-005, BR-RPT-009, BR-RPT-012, BR-AUTH-010, BR-PRIV-012, UC-RPT-04
- **Depends on:** EMP-RPT-001
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§4–5, 10.10 and 12, [organization lifecycle](../modules/organizations.md), [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: category inventory, deterministic files and per-category manifest for scoped portability or closure · out: bypassing retention, permissions or existing erasure
- **Areas:** existing: export jobs, organization data inventory and standard ZIP/CSV tooling · proposed: full organization export job and consumer contract
- **Steps:** Require fresh organization-scope authority at request and current scope at execution and download. Verify category coverage, counts and hashes and notify all owners.
- **Deliverable:** An organization export contract and private archive with a verifiable manifest.
- **Acceptance:** Every eligible category appears with its file, count and digest. Document content requires its separate scope. Oversize jobs fail explicitly rather than returning a truncated archive.
- **Negative cases:** Foreign or secret data in any category, missing required scope, revoked access, wrong manifest, unapproved document bytes or incomplete archive marked ready.
- **Validation:** V-API, V-SQL, V-JOB, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: reuse export job storage · rollback: disable full export and preserve authorized evidence
- **Risks and assumptions:** The closure workflow consumes this contract. Closure and legal policy remain the authority for data availability and retention.
- **Done when:** Scoped portability, manifest integrity, revocation and incomplete-job recovery pass with synthetic organizations.

### EMP-RPT-006 — Audit trail viewer
- **Meta:** module RPT · phase F6 · cut - · priority P1 · type WEB · size S · state TODO
- **Objective:** Expose scoped append-only audit history to authorized company members.
- **Rules and use cases:** BR-RPT-005, BR-RPT-013, BR-AUD-004, BR-AUD-006, UC-RPT-05
- **Depends on:** EMP-PLAT-003, EMP-AUTH-011, EMP-PLAT-008
- **Read first:** [intelligence, findings and reports](../modules/intelligence.md) §§4–5 and 11, [permission matrix](../07-permissions.md) §4
- **Scope:** in: read-only audit list, filters and authorized target links · out: editing audit or surfacing sensitive content
- **Areas:** existing: organization audit records, scoped exports and panel scaffold · proposed: audit viewer
- **Steps:** Exercise actor, action, target and period filters. Verify a target link requires the underlying read permission and export writes its own audit.
- **Deliverable:** An accessible read-only audit trail and export entry.
- **Acceptance:** Only scoped references and bounded reasons are shown. Content is excluded and target visibility remains governed separately.
- **Negative cases:** Foreign event, hidden content exposed by a link, audit record changed or unaudited export.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: none · rollback: hide the new view and preserve audit history
- **Risks and assumptions:** The viewer is separate from the platform admin audit surface.
- **Done when:** Scoped list, accessible filtering and denied sensitive cases pass through normal panel routes.
