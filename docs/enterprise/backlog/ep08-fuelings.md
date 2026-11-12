# EP08 — Fuelings and consumption

Status: **PLANNED**, 2026-11-10.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from the [fueling specification](../modules/fuelings.md), [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

The C1 fueling path uses the approved synthetic receipt scope. Consumption, reference comparisons, amendments and other later work keep their indexed dependencies.

### EMP-FUEL-001 — Fueling domain: products, units, payer and supplier roles
- **Meta:** module FUEL · phase F3 · cut C1 · priority P0 · type DOM · size M · state TODO
- **Objective:** Define one scoped supply event with exact product and unit semantics.
- **Rules and use cases:** BR-FUEL-001, BR-FUEL-002, BR-FUEL-003, BR-FUEL-004, BR-FUEL-005, BR-FUEL-006, BR-FUEL-009, BR-FUEL-011, BR-MONEY-009
- **Depends on:** EMP-PLAT-009, EMP-FLEET-001
- **Read first:** [fueling specification](../modules/fuelings.md) §§4–12, [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: required fields, product, fill, payer and independent supplier roles · out: storage and inferred fill levels
- **Areas:** existing: exact-value and fleet types · proposed: pure fueling domain
- **Steps:** Pin valid liquids, incompatible units, unknown fill, additive and independent place and issuer vectors.
- **Deliverable:** A pure fueling domain with exact-value vectors.
- **Acceptance:** The product fixes the quantity unit. Unknown fill stays partial and additive stays separate from consumption.
- **Negative cases:** Wrong unit, absent required input, additive treated as fuel, or one supplier reference replacing the others.
- **Validation:** V-DOM, V-MONEY · migration: none in this task · rollback: close the new admission or projection path and preserve approved records, revisions and private data
- **Risks and assumptions:** The owning specification supplies business rules. Keep scope, synthetic C1 evidence and the applicable phase gates explicit.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FUEL-002 — Fueling persistence, sync handlers and arithmetic consistency
- **Meta:** module FUEL · phase F3 · cut C1 · priority P0 · type DB · size L · state TODO
- **Objective:** Persist one organization fueling and its reading evidence through durable sync.
- **Rules and use cases:** BR-FUEL-007, BR-FUEL-008, BR-FUEL-018, BR-FUEL-024, BR-MONEY-004, BR-MONEY-005, BR-OFFL-004, UC-FUEL-01, VAL-08
- **Depends on:** EMP-FUEL-001, EMP-OFFL-002, EMP-METER-002
- **Read first:** [fueling specification](../modules/fuelings.md) §§4–12, [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: scoped persistence, per-operation intake and arithmetic warnings · out: changing paid amounts, importing personal records or recognizing a second cost
- **Areas:** existing: PostGIS, sqlc, organization intake and fleet readings · proposed: fueling repository and command handler
- **Steps:** Exercise authorized retry, concurrent writes, reading failure and exact arithmetic. Commit the fueling and its reading evidence with the intake outcome.
- **Deliverable:** A durable scoped fueling handler and integration evidence.
- **Acceptance:** A matching resend returns the stored result. The paid total survives an arithmetic mismatch, which is recorded as a warning.
- **Negative cases:** Foreign asset, mismatched digest, transaction without its reading evidence, warning that rejects a legitimate fact or paid total recomputed.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY, V-SYNC · migration: prove additive upgrade and recovery if persistent state changes · rollback: close the new admission or projection path and preserve approved records, revisions and private data
- **Risks and assumptions:** The owning specification supplies business rules. Keep scope, synthetic C1 evidence and the applicable phase gates explicit.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FUEL-003 — Fueling review: check, approve, reject, return, cancel
- **Meta:** module FUEL · phase F3 · cut C1 · priority P0 · type API · size M · state TODO
- **Objective:** Review a fueling under current authority and preserve each decision.
- **Rules and use cases:** BR-FUEL-017, BR-FUEL-020, BR-FUEL-021, BR-REV-003, BR-REV-005, BR-REV-007, UC-FUEL-02, UC-FUEL-03, UC-FUEL-04
- **Depends on:** EMP-FUEL-002, EMP-APPR-001
- **Read first:** [fueling specification](../modules/fuelings.md) §§4–12, [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: check, approve, reject, return, cancel and scoped resubmission · out: approving one's own or a stale record
- **Areas:** existing: shared review component and scoped fueling store · proposed: fueling review routes and audit
- **Steps:** Pin each allowed transition, warning justifications and competing decisions. Keep prior revisions and expose authoritative results to the author.
- **Deliverable:** An audited fueling review API.
- **Acceptance:** One decision applies to the reviewed revision. Warning and duplicate resolution rules hold and cancellation follows the documented states.
- **Negative cases:** Author, payer or responsible employee approves, stale reviewer succeeds, missing required reason, or cancellation erases a reading.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: prove additive upgrade and recovery if persistent state changes · rollback: close the new admission or projection path and preserve approved records, revisions and private data
- **Risks and assumptions:** The owning specification supplies business rules. Keep scope, synthetic C1 evidence and the applicable phase gates explicit.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FUEL-004 — Supplier linkage: canonical station, organization supplier or not found
- **Meta:** module FUEL · phase F3 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Record a fueling establishment without modifying the public catalog.
- **Rules and use cases:** BR-FUEL-011, BR-FUEL-012, BR-FUEL-013, BR-FUEL-014, BR-SRC-004, UC-FUEL-05
- **Depends on:** EMP-FUEL-002, EMP-FLEET-009
- **Read first:** [fueling specification](../modules/fuelings.md) §§4–12, [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: read-only catalog link, private supplier or not-found entry · out: public station writes and device coordinates
- **Areas:** existing: directory read ports and organization suppliers · proposed: scoped fueling place links
- **Steps:** Verify authorized catalog reference, private supplier and typed fallback independently of issuer and contract roles.
- **Deliverable:** Scoped supplier linkage with a complete no-match path.
- **Acceptance:** A fueling can be complete without a public station. Linking changes only the fueling record.
- **Negative cases:** Foreign private supplier, typed name creates a station, raw location captured, or issuer overwritten by the place.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-PRIV · migration: prove additive upgrade and recovery if persistent state changes · rollback: close the new admission or projection path and preserve approved records, revisions and private data
- **Risks and assumptions:** The owning specification supplies business rules. Keep scope, synthetic C1 evidence and the applicable phase gates explicit.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FUEL-005 — Capacity tolerance and product compatibility validation
- **Meta:** module FUEL · phase F3 · cut - · priority P1 · type DOM · size S · state TODO
- **Objective:** Turn out-of-tolerance volume and incompatible products into review evidence.
- **Rules and use cases:** BR-FUEL-015, BR-FUEL-016, BR-FUEL-017, BR-FUEL-030
- **Depends on:** EMP-FUEL-002
- **Read first:** [fueling specification](../modules/fuelings.md) §§4–12, [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: exact capacity and compatibility warnings · out: silently rejecting or approving an observed event
- **Areas:** existing: fleet tank capacity and product rules · proposed: pure fueling warning decisions
- **Steps:** Pin the tolerance boundary, unavailable input and incompatible-product cases. Preserve the recorded event and require its documented review notes.
- **Deliverable:** Deterministic capacity and compatibility warning results.
- **Acceptance:** Warnings leave the field event intact and remain visible to the reviewer.
- **Negative cases:** Overflow, unknown capacity assumed zero, incompatible product treated as valid for consumption, or warnings silently dropped.
- **Validation:** V-DOM, V-MONEY · migration: none in this task · rollback: close the new admission or projection path and preserve approved records, revisions and private data
- **Risks and assumptions:** The owning specification supplies business rules. Keep scope, synthetic C1 evidence and the applicable phase gates explicit.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FUEL-006 — Suspected duplicate versus technical retry
- **Meta:** module FUEL · phase F3 · cut - · priority P1 · type DOM · size S · state TODO
- **Objective:** Distinguish a technical resend from two recorded fuelings that look alike.
- **Rules and use cases:** BR-FUEL-018, BR-FUEL-019, BR-FUEL-020, BR-OFFL-004, UC-FUEL-07
- **Depends on:** EMP-FUEL-002
- **Read first:** [fueling specification](../modules/fuelings.md) §§4–12, [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: deterministic suspicion and review prerequisites · out: automatic merge, rejection or financial finding
- **Areas:** existing: scoped intake receipts and accepted fueling facts · proposed: duplicate signal rules
- **Steps:** Pin same-operation replay, time-window boundaries, independent records and review outcomes with synthetic vectors.
- **Deliverable:** A pure duplicate-suspicion decision and regression vectors.
- **Acceptance:** An identical resend creates no suspicion. Matching independent records stay visible and require a decision before approval when flagged.
- **Negative cases:** Resend creates another fact, different organization compared, one record silently merged, or suspicion treated as proof of duplicate payment.
- **Validation:** V-DOM, V-IDEM, V-MONEY · migration: none in this task · rollback: close the new admission or projection path and preserve approved records, revisions and private data
- **Risks and assumptions:** The owning specification supplies business rules. Keep scope, synthetic C1 evidence and the applicable phase gates explicit.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FUEL-007 — Consumption engine: full-to-full intervals with partial fuelings
- **Meta:** module FUEL · phase F3 · cut - · priority P1 · type DOM · size L · state TODO
- **Objective:** Compute supported full-to-full intervals from approved fuel and meter evidence.
- **Rules and use cases:** BR-FUEL-025, BR-FUEL-026, BR-FUEL-027, BR-FUEL-028, BR-FUEL-029, BR-FUEL-030, BR-FUEL-031, BR-FUEL-032, EDR-011, VAL-10, VAL-11
- **Depends on:** EMP-FUEL-002, EMP-METER-006
- **Read first:** [fueling specification](../modules/fuelings.md) §§4–12, [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: anchors, ordered partial fills, exact sums and insufficiency · out: estimating consumption from one fill
- **Areas:** existing: exact-value and meter rules and personal full-to-full precedent · proposed: enterprise interval engine
- **Steps:** Pin opening and closing anchor arithmetic, readings, mixed products, gaps and partial-only histories with shared vectors.
- **Deliverable:** A pure full-to-full engine and reproducible interval results.
- **Acceptance:** An interval excludes the opening purchase and includes the closing one. Missing evidence produces the documented reason rather than a numeric result.
- **Negative cases:** Cross-segment readings, pending fact counted as approved, incompatible fuels, a partial fill used as an anchor or ratio averaging.
- **Validation:** V-DOM, V-MONEY · migration: none in this task · rollback: close the new admission or projection path and preserve approved records, revisions and private data
- **Risks and assumptions:** The owning specification supplies business rules. Keep scope, synthetic C1 evidence and the applicable phase gates explicit.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FUEL-008 — Consumption projections and recalculation after corrections
- **Meta:** module FUEL · phase F3 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Rebuild affected consumption intervals with revision and method provenance.
- **Rules and use cases:** BR-FUEL-032, BR-FUEL-033, BR-FUEL-034, BR-AMD-005, BR-TRUTH-004, UC-FUEL-09, VAL-20
- **Depends on:** EMP-FUEL-007
- **Read first:** [fueling specification](../modules/fuelings.md) §§4–12, [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: scoped projections, job replay and superseded results · out: changing source fueling or meter history
- **Areas:** existing: interval engine, owning events and jobs · proposed: interval projection and recomputation worker
- **Steps:** Pin corrections, voids and changed inputs. Rebuild the affected span and ensure duplicate or older jobs cannot overwrite a newer result.
- **Deliverable:** Versioned consumption projections with retry evidence.
- **Acceptance:** Changes affect only the relevant scope and produce an identifiable new revision when the result changes.
- **Negative cases:** Duplicate projections, stale event wins, missing basis, foreign asset or old number retained after insufficiency.
- **Validation:** V-SQL, V-JOB, V-IDEM, V-ISO, V-MONEY · migration: prove additive upgrade and recovery if persistent state changes · rollback: close the new admission or projection path and preserve approved records, revisions and private data
- **Risks and assumptions:** The owning specification supplies business rules. Keep scope, synthetic C1 evidence and the applicable phase gates explicit.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FUEL-009 — Multi-tank, flex, CNG and energy category rules
- **Meta:** module FUEL · phase F6 · cut - · priority P2 · type DOM · size M · state TODO
- **Objective:** Apply tank-group and shared-usage rules without combining incompatible energy sources.
- **Rules and use cases:** BR-FUEL-004, BR-FUEL-005, BR-FUEL-030, BR-FUEL-035, BR-FUEL-036, BR-MONEY-009
- **Depends on:** EMP-FUEL-007
- **Read first:** [fueling specification](../modules/fuelings.md) §§4–12, [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: multi-tank anchors, flex, CNG and energy category treatment · out: unsupported per-product consumption estimates
- **Areas:** existing: full-to-full engine and fleet tank definitions · proposed: grouped-anchor and category vectors
- **Steps:** Exercise full-group versus partial fills and shared-meter cases. Keep product totals separate even when a ratio is unavailable.
- **Deliverable:** Deterministic multi-tank and energy-category results.
- **Acceptance:** A group anchor needs the documented complete fill. Shared usage or unsupported methods yield the stated insufficiency reason.
- **Negative cases:** Incompatible units added, CNG interval presented as supported or one tank assumed to represent a group.
- **Validation:** V-DOM, V-MONEY · migration: none in this task · rollback: close the new admission or projection path and preserve approved records, revisions and private data
- **Risks and assumptions:** The owning specification supplies business rules. Keep scope, synthetic C1 evidence and the applicable phase gates explicit.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FUEL-010 — Per-trip fuel allocation and cost per distance or hour
- **Meta:** module FUEL · phase F6 · cut - · priority P1 · type DOM · size M · state TODO
- **Objective:** Attribute computed interval quantities and costs to trips without calling them measured consumption.
- **Rules and use cases:** BR-FUEL-034, BR-FUEL-037, BR-FUEL-038, BR-MONEY-007, BR-MONEY-008, UC-FUEL-10
- **Depends on:** EMP-FUEL-008, EMP-TRIP-004
- **Read first:** [fueling specification](../modules/fuelings.md) §§4–12, [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: exact per-trip and unassigned allocations and derived costs per observed usage · out: adding purchases to attributed consumption
- **Areas:** existing: computed intervals, trip readings and exact allocation rules · proposed: trip allocation domain
- **Steps:** Pin trips across an anchor, unassigned usage, residual rounding and insufficient intervals.
- **Deliverable:** Exact interval attribution and scope-aware trip figures.
- **Acceptance:** Shares sum to the interval totals and remain labelled allocated. Purchases are displayed separately.
- **Negative cases:** Double counting, missing or incompatible reading assumed usable, foreign trip, or labelled allocation presented as a measurement.
- **Validation:** V-DOM, V-MONEY, V-ISO · migration: none in this task · rollback: close the new admission or projection path and preserve approved records, revisions and private data
- **Risks and assumptions:** The owning specification supplies business rules. Keep scope, synthetic C1 evidence and the applicable phase gates explicit.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FUEL-011 — App: add a fueling offline with receipt and meter reading
- **Meta:** module FUEL · phase F3 · cut C1 · priority P0 · type APP · size M · state TODO
- **Objective:** Record a fueling offline in the approved Android partition.
- **Rules and use cases:** BR-FUEL-002, BR-FUEL-007, BR-FUEL-024, BR-OFFL-002, BR-OFFL-003, BR-OFFL-015, OQ-04, UC-FUEL-01
- **Depends on:** EMP-FUEL-002, EMP-OFFL-005, EMP-FLEET-013, EMP-DOC-004
- **Read first:** [fueling specification](../modules/fuelings.md) §§4–12, [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: meter or absence reason, required fueling inputs, durable operation and synthetic C1 receipt · out: using personal ledger or real receipts before approval
- **Areas:** existing: Android enterprise outbox, fleet references and private attachment flow · proposed: offline fueling entry
- **Steps:** Exercise local save, process restart, duplicate transport, missing reading reason and later receipt attachment.
- **Deliverable:** An accessible offline fueling flow with truthful sync state.
- **Acceptance:** The original scope and entered amounts survive restart. Acceptance follows the server outcome and attachment progress stays separate.
- **Negative cases:** Local save without durable queue, wrong account or organization, synthetic-scope bypass or accepted status before acknowledgement.
- **Validation:** V-APP, V-SYNC, V-ISO, V-PRIV, V-MONEY · migration: none in this task · rollback: close the new admission or projection path and preserve approved records, revisions and private data
- **Risks and assumptions:** The owning specification supplies business rules. Keep scope, synthetic C1 evidence and the applicable phase gates explicit.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FUEL-012 — Panel: fuelings list, detail and review
- **Meta:** module FUEL · phase F3 · cut C1 · priority P0 · type WEB · size M · state TODO
- **Objective:** List and review scoped fuelings in the company panel.
- **Rules and use cases:** BR-FUEL-017, BR-FUEL-021, BR-AUTH-011, BR-PRIV-004, BR-REV-007, UC-FUEL-02, SCR-FUEL-06, SCR-FUEL-07
- **Depends on:** EMP-FUEL-003, EMP-PLAT-008
- **Read first:** [fueling specification](../modules/fuelings.md) §§4–12, [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: authorized list, detail, warnings and review actions · out: implicit document-content permission and client-side decisions
- **Areas:** existing: /empresas/ scaffold and fueling review API · proposed: accessible company fueling views
- **Steps:** Check scope, fresh results, concurrent decision conflicts and separate receipt permission through the company session.
- **Deliverable:** Scoped company fueling list, detail and review surfaces.
- **Acceptance:** Review state and warning reasons are visible. Denials or conflicts leave the original revision and records intact.
- **Negative cases:** Foreign result after switch, receipt bytes served through general read, stale review or self-approval.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: none in this task · rollback: close the new admission or projection path and preserve approved records, revisions and private data
- **Risks and assumptions:** The owning specification supplies business rules. Keep scope, synthetic C1 evidence and the applicable phase gates explicit.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FUEL-013 — Retroactive fuelings and their effect on intervals
- **Meta:** module FUEL · phase F3 · cut - · priority P1 · type DOM · size S · state TODO
- **Objective:** Place retroactive fuelings in the true sequence without bypassing period locks.
- **Rules and use cases:** BR-FUEL-022, BR-FUEL-028, BR-FUEL-033, BR-AMD-004, BR-TIME-004, UC-FUEL-06
- **Depends on:** EMP-FUEL-007
- **Read first:** [fueling specification](../modules/fuelings.md) §§4–12, [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: effective ordering, late warning and affected interval basis · out: silently editing an approved or locked period
- **Areas:** existing: time and interval rules · proposed: retroactive-entry domain cases
- **Steps:** Pin neighboring readings, late thresholds, equal-time order and locked days. Test the recomputation handoff for affected intervals.
- **Deliverable:** A deterministic retrospective entry decision and affected-span result.
- **Acceptance:** The recorded instant is retained. Late and inconsistent evidence is surfaced under the current review and period rules.
- **Negative cases:** Device time overrides the lock, old interval silently stays current or a suspect reading fabricates a ratio.
- **Validation:** V-DOM, V-MONEY · migration: none in this task · rollback: close the new admission or projection path and preserve approved records, revisions and private data
- **Risks and assumptions:** The owning specification supplies business rules. Keep scope, synthetic C1 evidence and the applicable phase gates explicit.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FUEL-014 — Price reference snapshot at fueling time
- **Meta:** module FUEL · phase F3 · cut - · priority P2 · type API · size M · state TODO
- **Objective:** Preserve the source-specific price reference available at fueling acceptance.
- **Rules and use cases:** BR-FUEL-039, BR-SRC-001, BR-TRUTH-004, BR-KPI-012, UC-FUEL-11, VAL-15
- **Depends on:** EMP-FUEL-004
- **Read first:** [fueling specification](../modules/fuelings.md) §§4–12, [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: immutable snapshot of reference or unavailability per source · out: blending sources and treating a reference as a live paid price
- **Areas:** existing: official and community read models and fueling persistence · proposed: reference capture port
- **Steps:** Exercise no match, coverage and payment differences and repeated acceptance. Preserve origin, period, method and conditions.
- **Deliverable:** Scoped source-specific snapshot with deterministic replay.
- **Acceptance:** Each enabled source has an immutable value and basis or a reasoned unavailable state.
- **Negative cases:** Later source revision rewrites the snapshot, incompatible condition silently substituted, sources merged or no reference shown as zero.
- **Validation:** V-API, V-SQL, V-ISO, V-IDEM, V-MONEY · migration: prove additive upgrade and recovery if persistent state changes · rollback: close the new admission or projection path and preserve approved records, revisions and private data
- **Risks and assumptions:** The owning specification supplies business rules. Keep scope, synthetic C1 evidence and the applicable phase gates explicit.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FUEL-015 — Fueling amendment after approval and period lock
- **Meta:** module FUEL · phase F3 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Amend or void approved fuelings while preserving the original and period controls.
- **Rules and use cases:** BR-FUEL-023, BR-AMD-001, BR-AMD-002, BR-AMD-003, BR-AMD-004, BR-AMD-005, UC-FUEL-08
- **Depends on:** EMP-FUEL-003, EMP-PLAT-016, EMP-APPR-006
- **Read first:** [fueling specification](../modules/fuelings.md) §§4–12, [validation scenarios](../15-validation.md) and [F0 decisions](../evidence/f0-m0-decisions.md)
- **Scope:** in: scoped amendment, review and recalculation handoff · out: editing or deleting approved history in place
- **Areas:** existing: review API, period locks and amendment kernel · proposed: fueling amendment commands
- **Steps:** Pin prior revision visibility, period denial, competing decisions and downstream recalculation.
- **Deliverable:** Audited amendment and void commands with prior revision intact.
- **Acceptance:** The approved revision stays effective until a reviewed amendment applies. Locked-period work follows the authorized reopening or adjustment rules.
- **Negative cases:** Self-approval, stale revision, unreviewed lock bypass, lost original or duplicate effect in cost and consumption.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: prove additive upgrade and recovery if persistent state changes · rollback: close the new admission or projection path and preserve approved records, revisions and private data
- **Risks and assumptions:** The owning specification supplies business rules. Keep scope, synthetic C1 evidence and the applicable phase gates explicit.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.
