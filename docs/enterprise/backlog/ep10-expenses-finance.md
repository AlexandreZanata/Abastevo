# EP10 — Expenses, approvals, budgets and operational finance

Status: **PLANNED**, 2026-11-10.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from the [shared rules](../04-business-rules.md), [domain model](../06-domain-model.md), [expense and finance requirements](../17-traceability.md) and [validation scenarios](../15-validation.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

The [expense and finance specification](../modules/expenses-finance.md) records the recovered design. This recovery preserves the approved and documented one-cost rule, scoped review, synthetic fixtures and the separation of accounting records from actual payment.

### EMP-APPR-001 — Review lifecycle component: submit, approve, return, reject, no self-approval
- **Meta:** module APPR · phase F3 · cut C1 · priority P0 · type DOM · size M · state LOCAL_DONE
- **Objective:** Share a revision-bound review lifecycle with an explicit separation-of-duties guard.
- **Rules and use cases:** BR-REV-001, BR-REV-002, BR-REV-003, BR-REV-004, BR-REV-005, BR-REV-006, BR-REV-007, BR-REV-009, EDR-012
- **Depends on:** EMP-AUTH-003, EMP-PLAT-003
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: submit, approve, return and reject with one-level C1 review · out: multi-level policies and specific persistence adapters
- **Areas:** existing: scoped authority, audit and review rules · proposed: pure review lifecycle
- **Steps:** Pin allowed transitions, reasons, self-approval and stale decisions using pure vectors.
- **Deliverable:** A reusable review state contract and negative tests.
- **Acceptance:** Only the currently submitted revision can be decided. A return retains history and an approval requires an eligible independent reviewer.
- **Negative cases:** Author or beneficiary decides, stale decision accepted, reason omitted or offline state presented as server approval.
- **Validation:** V-DOM, V-AUTHZ · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

[F3 pure review lifecycle](../evidence/f3-appr001-review-lifecycle.md)
is LOCAL_DONE / INTEGRATION_PENDING, 2026-11-14. Go `review/domain` and the
Kotlin `enterprise.review` mirror pass the same 232-case review-v1 oracle:
C1 transitions, revision binding, append-only return loop, bounded reasons,
canonical no self-approval, permission at receipt and honest local
presentation. Persistence, routes, panel/app, levels and amounts remain open.

### EMP-APPR-002 — Approval policies: thresholds, levels and routing by scope
- **Meta:** module APPR · phase F3 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Configure exact approval thresholds and scoped, ordered reviewers.
- **Rules and use cases:** BR-REV-003, BR-REV-004, BR-REV-008, BR-REV-009, BR-AUTH-004, BR-MONEY-001, REQ-11.3, VAL-25
- **Depends on:** EMP-APPR-001, EMP-AUTH-006
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: versioned routing policy and level thresholds · out: retroactively editing an active review decision
- **Areas:** existing: shared review and resource scope · proposed: policy commands and storage
- **Steps:** Test threshold boundaries, missing levels, scoped reviewers and policy revision conflict.
- **Deliverable:** A versioned approval-policy API.
- **Acceptance:** Required levels are explicit and all required approvals apply to the same revision.
- **Negative cases:** Approver outside scope, missing policy routes to unconditional approval, threshold rounding or concurrent update loses history.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-APPR-003 — Substitutes and delegation in approval routing
- **Meta:** module APPR · phase F3 · cut - · priority P1 · type DOM · size S · state TODO
- **Objective:** Route approvals to substitutes only under current delegation.
- **Rules and use cases:** BR-AUTH-007, BR-AUTH-008, BR-AUTH-014, BR-REV-003, BR-REV-004, BR-AUD-005, REQ-11.4
- **Depends on:** EMP-APPR-002, EMP-AUTH-007
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: pure routing and delegation checks · out: implied permanent permission or self-approval
- **Areas:** existing: authorized delegation and approval policy · proposed: approver resolution rules
- **Steps:** Pin the expiry boundary, revoked delegator, actor attribution and record-level exclusions.
- **Deliverable:** Deterministic routing with an explicit delegation basis.
- **Acceptance:** A substitute may act only while scope and time permit and each action records the delegation.
- **Negative cases:** Expired delegate, grant wider than delegator, self-review or missing attribution.
- **Validation:** V-DOM, V-AUTHZ · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-APPR-004 — Partial approval and the return-for-correction loop
- **Meta:** module APPR · phase F3 · cut - · priority P1 · type DOM · size S · state TODO
- **Objective:** Preserve returned revisions and bound partial company approval to the submitted amount.
- **Rules and use cases:** BR-REV-005, BR-REV-007, BR-REV-008, BR-REV-009, BR-MONEY-001, REQ-11.4, VAL-25
- **Depends on:** EMP-APPR-002
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: return, correction, resubmit and allowed partial amount · out: modifying old evidence or settling the payable
- **Areas:** existing: review state and exact amounts · proposed: correction and partial decision rules
- **Steps:** Pin revision rollover, stale approval, partial amount and each required level.
- **Deliverable:** Pure review extensions and exact-amount vectors.
- **Acceptance:** Resubmission has a new revision and the approved company amount alone enters approved costs.
- **Negative cases:** Stale approval applied, partial amount exceeds submission, pending amount counted or original revision removed.
- **Validation:** V-DOM, V-MONEY, V-IDEM · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-APPR-005 — Panel: approvals inbox
- **Meta:** module APPR · phase F3 · cut C1 · priority P0 · type WEB · size M · state TODO
- **Objective:** Present an authorized approval inbox in the company panel.
- **Rules and use cases:** BR-REV-003, BR-REV-004, BR-REV-005, BR-REV-007, BR-AUTH-012, BR-AUTH-013
- **Depends on:** EMP-APPR-001, EMP-PLAT-008
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: scoped pending records, status, reasons and decisions · out: client-side authorization and later policy-only actions in C1
- **Areas:** existing: /empresas/ scaffold and review APIs · proposed: approval inbox
- **Steps:** Test narrow scope, decision conflicts and accessibility through the company session.
- **Deliverable:** An accessible approval inbox with truthful server results.
- **Acceptance:** A reviewer sees only authorized items and server-confirmed decisions.
- **Negative cases:** Foreign item after context switch, stale result, unaudited approval or missing reason hidden by the UI.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-APPR-006 — Period closing, lock and reopening
- **Meta:** module APPR · phase F3 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Enforce period locking and authorized reopening without rewriting approved records.
- **Rules and use cases:** BR-AMD-001, BR-AMD-004, BR-TIME-003, BR-TIME-004, BR-AUD-001, REQ-11.4
- **Depends on:** EMP-APPR-001
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: scoped close, lock, reopen and reason · out: bypassing locks or rewriting approved revisions
- **Areas:** existing: review lifecycle, time kernel and audit · proposed: period commands and state
- **Steps:** Pin reporting-day boundaries and concurrent close versus write. Reopening records the authority and reason.
- **Deliverable:** A revision-aware period lock API.
- **Acceptance:** A locked reporting day refuses in-place changes and permits only the documented reopening or adjustment path.
- **Negative cases:** Wrong time zone, unauthorized reopening, race admits locked write or approval history lost.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-APPR-007 — App: my submissions and returned items
- **Meta:** module APPR · phase F3 · cut - · priority P1 · type APP · size S · state TODO
- **Objective:** Show a member their own submissions, returned reasons and pending corrections.
- **Rules and use cases:** BR-REV-002, BR-REV-005, BR-REV-007, BR-OFFL-010, BR-OFFL-013, BR-TEN-004
- **Depends on:** EMP-APPR-001, EMP-OFFL-004
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: Android submissions and correction entry with accurate state · out: offline approval or other members' data
- **Areas:** existing: app sync status and review outcome feed · proposed: own-submissions UI
- **Steps:** Exercise returned, resubmitted, offline and stale-revision flows in the original context.
- **Deliverable:** A scoped Android submissions and correction view.
- **Acceptance:** The member sees the original reason and current revision. Durable pending work is not shown as accepted early.
- **Negative cases:** Foreign record, correction retagged, missing reason or terminal decision inferred from transport success.
- **Validation:** V-APP, V-SYNC, V-AUTHZ, V-ISO · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-EXP-001 — Expense domain: categories, origin, competence date and company amount
- **Meta:** module EXP · phase F3 · cut - · priority P1 · type DOM · size M · state TODO
- **Objective:** Represent a distinct organization economic cost with its recorded source and competence date.
- **Rules and use cases:** EDR-013, BR-MONEY-001, BR-MONEY-004, BR-TIME-004, BR-REV-009, REQ-11.1
- **Depends on:** EMP-PLAT-009, EMP-PLAT-015
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: origin, category, exact company amount and reporting basis · out: payables, provider payment and persistence
- **Areas:** existing: exact-value and time kernels · proposed: pure expense domain
- **Steps:** Pin exact amounts, date attribution, source identity and approved-versus-pending inclusion.
- **Deliverable:** A pure expense model and invariants.
- **Acceptance:** A cost retains its origin and exact amount. Settlement does not create another cost.
- **Negative cases:** Missing origin, rounding away paid amount, false zero or settlement represented as a new economic cost.
- **Validation:** V-DOM, V-MONEY · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-EXP-002 — Expense persistence, sync handlers and the fueling-to-expense projection
- **Meta:** module EXP · phase F3 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Persist one cost per fueling and support scoped expense intake.
- **Rules and use cases:** EDR-013, BR-FUEL-010, BR-OFFL-004, BR-REV-009, BR-TEN-005, REQ-11.2, VAL-13
- **Depends on:** EMP-EXP-001, EMP-FUEL-002, EMP-OFFL-002
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: expense storage, sync and unique fueling-origin projection · out: duplicate costs from review replay, reimbursement or payment
- **Areas:** existing: organization intake, fueling facts and audit · proposed: expense repository and origin-constrained projection
- **Steps:** Pin replay, competing inserts, failed transaction and correction. Exercise VAL-13 with synthetic scoped records.
- **Deliverable:** An idempotent expense intake and fueling-to-expense projection.
- **Acceptance:** One fueling origin produces one expense under concurrent processing. Approvals, receipts and settlement do not duplicate the recognized amount.
- **Negative cases:** Foreign source, second cost for one fueling, partial transaction or stale event overwrites a newer revision.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-EXP-003 — Expense categories configuration
- **Meta:** module EXP · phase F3 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Configure scoped expense categories without losing historical references.
- **Rules and use cases:** BR-TEN-003, BR-AUD-001, BR-ORG-012, REQ-11.1
- **Depends on:** EMP-EXP-002
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: authorized category lifecycle and current availability · out: renaming past expense identity or deleting used categories
- **Areas:** existing: scoped expenses and organization settings · proposed: category commands
- **Steps:** Test concurrent duplicate, stale edit and retired category usage. Preserve references in old records.
- **Deliverable:** An audited category configuration API.
- **Acceptance:** Only allowed categories are selectable for new operations and history remains readable.
- **Negative cases:** Foreign category, stale edit, retired category accepted unexpectedly or historical cost reclassified silently.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-EXP-004 — App: add an expense with receipt
- **Meta:** module EXP · phase F3 · cut - · priority P1 · type APP · size M · state TODO
- **Objective:** Capture a scoped expense and authorized attachment on Android.
- **Rules and use cases:** BR-MONEY-001, BR-OFFL-002, BR-OFFL-003, BR-OFFL-015, BR-PRIV-004, OQ-04, REQ-11.1
- **Depends on:** EMP-EXP-002, EMP-DOC-004
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: exact amount, required data, durable queue and private attachment · out: unapproved real document intake or offline approval
- **Areas:** existing: enterprise queue and private document capture · proposed: Android expense entry
- **Steps:** Exercise restart, offline state and failed or missing attachments using synthetic data.
- **Deliverable:** An accessible expense form with durable pending work.
- **Acceptance:** The expense keeps its origin and scope and reports acceptance only after the server responds.
- **Negative cases:** Unsaved success, cross-account attachment, amount conversion error or bypassing legal receipt gates.
- **Validation:** V-APP, V-SYNC, V-ISO, V-PRIV, V-MONEY · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-EXP-005 — Panel: expenses list, detail and review
- **Meta:** module EXP · phase F3 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Expose scoped expense lists, detail and review in the company panel.
- **Rules and use cases:** BR-REV-003, BR-REV-004, BR-REV-007, BR-REV-009, BR-AUTH-011, REQ-11.1
- **Depends on:** EMP-EXP-002, EMP-APPR-001, EMP-PLAT-008
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: status, exact company amount and authorized review · out: implicit document-content access and client decisions
- **Areas:** existing: /empresas/ scaffold, expense and review APIs · proposed: expense views
- **Steps:** Test filters, pending versus approved values, forbidden document reads and concurrent decision feedback.
- **Deliverable:** Accessible expense list and review screens.
- **Acceptance:** The company amount, scope and review status are visible and reflect the authoritative server revision.
- **Negative cases:** Foreign expense visible, pending totals treated as approved, stale approval or receipt permission bypass.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-MONEY, V-PRIV · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-EXP-006 — Cost allocation across cost centres and assets
- **Meta:** module EXP · phase F3 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Allocate an expense across assets or cost centres without losing centavos.
- **Rules and use cases:** BR-MONEY-007, BR-MONEY-008, BR-TEN-005, BR-ORG-012, REQ-11.1, VAL-09
- **Depends on:** EMP-EXP-002, EMP-ORG-006
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: exact allocations and persistent scoped history · out: counting allocations as additional expense
- **Areas:** existing: exact allocation rule and organization dimensions · proposed: transaction-backed expense allocations
- **Steps:** Pin residual ordering, concurrent allocations and foreign targets. Check totals before commit.
- **Deliverable:** Scoped allocations with exact reconciliation evidence.
- **Acceptance:** Accepted splits sum to the source amount and retain the historical target scope.
- **Negative cases:** Allocation exceeds the source, fractional cent lost, foreign target or total counted once per allocation as a whole.
- **Validation:** V-SQL, V-MONEY, V-ISO, V-IDEM · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-EXP-007 — Fines and recurring costs such as lease and insurance
- **Meta:** module EXP · phase F6 · cut - · priority P2 · type API · size S · state TODO
- **Objective:** Record fines and recurring operating costs using the same scoped cost contract.
- **Rules and use cases:** EDR-013, BR-MONEY-001, BR-TIME-004, BR-REV-009, REQ-11.1
- **Depends on:** EMP-EXP-002, EMP-FLEET-004
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: eligible recurring or fine entries and provenance · out: payroll, provider integration and new cost recognition rules
- **Areas:** existing: expenses, asset history and category configuration · proposed: recurring and fine workflows
- **Steps:** Test repeat submissions and recurrence identity, approval and date attribution.
- **Deliverable:** Scoped recurring and fine expense entry points.
- **Acceptance:** A recorded occurrence retains its category, source and period without duplicating another cost.
- **Negative cases:** Same occurrence recorded twice, foreign asset, provider charge implied or pending cost presented as approved.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FIN-001 — Payables domain: supplier, reimbursement and card statement obligations
- **Meta:** module FIN · phase F3 · cut - · priority P1 · type DOM · size S · state TODO
- **Objective:** Model financial obligations independently of recognized cost.
- **Rules and use cases:** EDR-013, BR-MONEY-001, BR-TRUTH-001, REQ-11.1, REQ-11.2
- **Depends on:** EMP-EXP-001
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: supplier, reimbursement and card obligations and settlement state · out: creating payments or accounting entries
- **Areas:** existing: expense origin and exact amount · proposed: pure payable domain
- **Steps:** Pin allowed obligation and settlement states and linked-source amounts.
- **Deliverable:** A pure payable model and settlement-state vectors.
- **Acceptance:** Obligations reference their source, remain traceable and do not increase cost totals.
- **Negative cases:** Payable recognized as a second expense, ambiguous state or invalid negative amount.
- **Validation:** V-DOM, V-MONEY · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FIN-002 — Reimbursements for out-of-pocket expenses
- **Meta:** module FIN · phase F3 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Create a scoped reimbursement obligation for approved employee-paid expenses.
- **Rules and use cases:** EDR-013, BR-REV-003, BR-REV-009, BR-MONEY-001, REQ-11.2, VAL-13
- **Depends on:** EMP-FIN-001, EMP-EXP-002, EMP-APPR-001
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: authorized reimbursement and source link · out: external transfer or duplicating expense recognition
- **Areas:** existing: payable domain, approved expense and review rules · proposed: reimbursement commands
- **Steps:** Exercise employee-paid versus company-paid inputs, approval state, retry and duplicate obligations.
- **Deliverable:** An audited reimbursement API with single-source evidence.
- **Acceptance:** Reimbursement references the approved company amount and does not create another expense.
- **Negative cases:** Duplicate obligation, wrong employee or organization, self-approval or pending cost reimbursed as approved.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FIN-003 — Advances and settlement against expenses
- **Meta:** module FIN · phase F3 · cut - · priority P2 · type API · size M · state TODO
- **Objective:** Track advances and exact settlements against eligible expenses.
- **Rules and use cases:** EDR-013, BR-MONEY-001, BR-MONEY-007, BR-AUD-001, REQ-11.1, REQ-11.4
- **Depends on:** EMP-FIN-002
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: scoped advances, allocations and outstanding balance · out: real money movement or recording an advance as cost
- **Areas:** existing: reimbursements and exact allocation conventions · proposed: advance and settlement commands
- **Steps:** Pin partial settlement, excess amount, concurrent attempts and surviving balance.
- **Deliverable:** An audited advance and settlement workflow.
- **Acceptance:** Settlement records source links and exact balances. The recognized expense stays distinct.
- **Negative cases:** Over-settlement, cross-organization link, rounding loss or duplicate advance on replay.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FIN-004 — Payment records and the single-cost invariant
- **Meta:** module FIN · phase F3 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Record external payments once and keep the cost/settlement distinction.
- **Rules and use cases:** EDR-013, BR-MONEY-001, BR-OFFL-004, BR-AUD-001, REQ-11.2, REQ-11.5, VAL-13
- **Depends on:** EMP-FIN-001, EMP-EXP-002
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: scoped payment persistence and payable settlement · out: initiating provider payments or recording a second cost
- **Areas:** existing: payable and expense stores and audited commands · proposed: payment records and idempotent settlement
- **Steps:** Test exact amounts, partial and complete settlement, lost response and concurrent retry.
- **Deliverable:** A durable payment record with one settlement effect per authorized operation.
- **Acceptance:** Recording or replaying payment changes cash and obligation views only. The original cost remains once.
- **Negative cases:** Duplicate payment, invalid total, partial failure, foreign obligation or provider settlement inferred from local recording.
- **Validation:** V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FIN-005 — Statements: card and supplier invoices grouping payables
- **Meta:** module FIN · phase F6 · cut - · priority P2 · type API · size M · state TODO
- **Objective:** Group existing obligations into card or supplier statements without duplicating them.
- **Rules and use cases:** EDR-013, BR-MONEY-001, BR-MONEY-007, BR-TEN-005, REQ-11.2
- **Depends on:** EMP-FIN-004
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: scoped statement linkage and exact grouping · out: new expense recognition or provider imports
- **Areas:** existing: recorded payments and payables · proposed: statement commands and association records
- **Steps:** Test repeated grouping, incompatible scope, balance mismatch and prior settlements.
- **Deliverable:** Statement grouping with auditable payable references.
- **Acceptance:** A statement totals its eligible obligations and does not duplicate underlying costs or settlement.
- **Negative cases:** Same payable counted twice, foreign supplier, concealed discrepancy or a second expense from a statement.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FIN-006 — Accounting export: competence and cash views
- **Meta:** module FIN · phase F6 · cut - · priority P1 · type JOB · size M · state TODO
- **Objective:** Export competence and cash views with explicit independent bases.
- **Rules and use cases:** EDR-013, BR-MONEY-001, BR-TIME-004, BR-RPT-002, BR-RPT-005, BR-RPT-008, REQ-11.4, REQ-11.5
- **Depends on:** EMP-FIN-004, EMP-RPT-001
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: scoped accounting export and exact period views · out: posting accounting entries or combining cash and cost as one amount
- **Areas:** existing: export jobs, expense and settlement records · proposed: accounting export generator
- **Steps:** Test a cost and its later payment across periods, scoped authorization, expiry and retry.
- **Deliverable:** Private accounting exports with labelled competence and cash bases.
- **Acceptance:** A cost and its disbursement appear in the appropriate view with exact units and scope.
- **Negative cases:** Duplicate cost, cash date used as competence date, foreign rows, expired file or unauthorized scope.
- **Validation:** V-SQL, V-JOB, V-AUTHZ, V-ISO, V-IDEM, V-MONEY, V-PRIV · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FIN-007 — Panel: payables, reimbursements and payments
- **Meta:** module FIN · phase F3 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Present payables, reimbursements and recorded payments in the company panel.
- **Rules and use cases:** EDR-013, BR-TRUTH-001, BR-AUTH-013, BR-RPT-002, REQ-11.1
- **Depends on:** EMP-FIN-002, EMP-FIN-004, EMP-PLAT-008
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: scoped lists and authorized settlement actions · out: real payment initiation and duplicating expense totals
- **Areas:** existing: /empresas/ scaffold and financial record APIs · proposed: finance screens
- **Steps:** Verify scope, clear pending and settled states, permission denial and exact amount display.
- **Deliverable:** Accessible company settlement views.
- **Acceptance:** The member can distinguish an obligation, recognized expense and recorded payment.
- **Negative cases:** Foreign account data, apparent payment execution, rounded base amount or stale state shown as settled.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-MONEY, V-PRIV · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-BUDG-001 — Budget domain: lines, periods, dimensions and amounts
- **Meta:** module BUDG · phase F3 · cut - · priority P1 · type DOM · size S · state TODO
- **Objective:** Define exact budgets over scoped dimensions and reporting periods.
- **Rules and use cases:** BR-MONEY-001, BR-TIME-004, BR-AUTH-001, REQ-11.3, VAL-14
- **Depends on:** EMP-EXP-001
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: budget line, dimension, amount and available-basis rules · out: persistence and overrun policy decisions
- **Areas:** existing: exact expense amounts and time conventions · proposed: pure budget domain
- **Steps:** Pin period bounds, compatible dimensions and available amount derived from consumption and active holds.
- **Deliverable:** A budget domain with exact capacity vectors.
- **Acceptance:** Availability derives from authoritative facts rather than a stale independent number.
- **Negative cases:** Wrong period or dimension, overflow, mixed scope or approval derived from an unsupported default.
- **Validation:** V-DOM, V-MONEY · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-BUDG-002 — Budget persistence and concurrent consumption posting
- **Meta:** module BUDG · phase F3 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Persist budget lines and post consumption under concurrency.
- **Rules and use cases:** BR-MONEY-001, BR-AUTH-001, BR-REV-009, REQ-11.3, VAL-14
- **Depends on:** EMP-BUDG-001, EMP-EXP-002
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: scoped budget state, exact posting and locking · out: financial disbursement or stored false availability
- **Areas:** existing: expense and budget domain, PostGIS and sqlc · proposed: budget repository and consumption transactions
- **Steps:** Exercise VAL-14 and concurrent posting on a populated fixture. Reconcile computed availability from the committed rows.
- **Deliverable:** Transactional budget consumption and a race-test baseline.
- **Acceptance:** Concurrent requests cannot consume the same remaining amount twice. Retry converges on one posting.
- **Negative cases:** Overrun without policy, duplicate posting, cross-scope budget or a rollback that retains a charge.
- **Validation:** V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-BUDG-003 — Reservations: pre-authorisation, release and expiry
- **Meta:** module BUDG · phase F3 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Reserve capacity before authorization and release it exactly once.
- **Rules and use cases:** BR-MONEY-001, BR-AUTH-001, BR-AUD-001, REQ-11.3, VAL-14
- **Depends on:** EMP-BUDG-002
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: bounded hold, release and expiry · out: performing a payment or creating a duplicate expense
- **Areas:** existing: budget transactions and scoped jobs · proposed: reservation commands and expiry handling
- **Steps:** Pin last-capacity races, expiry, release and transition to the owning consumption outcome.
- **Deliverable:** A tested pre-authorization hold lifecycle.
- **Acceptance:** Only an active authorized hold reserves capacity. Release and expiry restore only the documented available amount once.
- **Negative cases:** Two holds consume the same last balance, wrong owner releases, late retry revives hold or expiry loses accepted cost.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-JOB, V-MONEY · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-BUDG-004 — Overrun policies and exception approval
- **Meta:** module BUDG · phase F3 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Route a budget overrun to an explicit exception only when allowed.
- **Rules and use cases:** BR-AUTH-001, BR-REV-003, BR-REV-008, BR-AUD-001, REQ-11.3, VAL-14
- **Depends on:** EMP-BUDG-002, EMP-APPR-002
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: reviewed policy, scoped reason and approval decision · out: allowing overrun by default or bypassing normal approval
- **Areas:** existing: budgets and approval policies · proposed: exception workflow
- **Steps:** Exercise deny and review paths, stale policy, wrong approver and repeated decisions.
- **Deliverable:** A reasoned and audited budget exception API.
- **Acceptance:** Unapproved overruns do not post. Authorized exceptions follow the configured policy and record the decision.
- **Negative cases:** Silent overrun, self-approval, duplicate posting or cross-organization policy reuse.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-BUDG-005 — Panel: budgets setup and monitoring
- **Meta:** module BUDG · phase F3 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Show scoped budget configuration and calculated usage in the company panel.
- **Rules and use cases:** BR-MONEY-001, BR-AUTH-012, BR-TRUTH-004, REQ-11.3
- **Depends on:** EMP-BUDG-002, EMP-PLAT-008
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: authorized setup, reporting period and available/held/consumed views · out: client-side authorization and editable calculated balance
- **Areas:** existing: /empresas/ scaffold and budget APIs · proposed: budget screens
- **Steps:** Exercise scope, exact amounts, stale data and rejected configuration updates.
- **Deliverable:** Accessible budget setup and monitoring views.
- **Acceptance:** The panel shows the server basis and current scoped availability.
- **Negative cases:** Foreign rows, rounded exact values, stale availability described as current or unauthorized change.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-MONEY · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-BUDG-006 — Driver and asset limits at submission and after sync
- **Meta:** module BUDG · phase F3 · cut - · priority P2 · type DOM · size S · state TODO
- **Objective:** Check driver and asset limits at submission and again at authorized intake.
- **Rules and use cases:** BR-AUTH-001, BR-OFFL-008, BR-OFFL-009, BR-REV-004, REQ-11.3
- **Depends on:** EMP-BUDG-002
- **Read first:** [shared rules](../04-business-rules.md) §§4–8, [domain model](../06-domain-model.md) §§3–5, [expense and finance requirements](../17-traceability.md) §11 and [validation scenarios](../15-validation.md) §4
- **Scope:** in: scope and time-aware limit decisions and review evidence · out: treating cached data or client time as authority
- **Areas:** existing: budget lines, fleet scope and sync history · proposed: pure limit decisions
- **Steps:** Pin exact boundary, changed configuration, wrong asset and stale reference cases.
- **Deliverable:** Deterministic limit decision vectors for online and synchronized submissions.
- **Acceptance:** The current scoped policy and recorded history decide the disposition. Offline facts follow the documented review rules.
- **Negative cases:** A stale cache authorizes an overrun, scope changes silently, missing limits treated as a guessed amount or no reason retained.
- **Validation:** V-DOM, V-SYNC, V-AUTHZ, V-MONEY · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.
