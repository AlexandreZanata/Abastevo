# EP10 — Expenses, approvals, budgets and operational finance

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from the [shared rules](../business-rules.md), [domain model](../domain-model.md), expense and finance requirements and [validation scenarios](../testing.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
sequencing decision.

The [expense and finance specification](../modules/expenses-finance.md) records the recovered design. This recovery preserves the approved and documented one-cost rule, scoped review, synthetic fixtures and the separation of accounting records from actual payment.

### EMP-APPR-002 — Approval policies: thresholds, levels and routing by scope
- **Meta:** module APPR · phase F3 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Configure exact approval thresholds and scoped, ordered reviewers.
- **Rules and use cases:** BR-REV-003, BR-REV-004, BR-REV-008, BR-REV-009, BR-AUTH-004, BR-MONEY-001, REQ-11.3, VAL-25
- **Depends on:** EMP-APPR-001, EMP-AUTH-006
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
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
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
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
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
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
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
- **Scope:** in: scoped pending records, status, reasons and decisions · out: client-side authorization and later policy-only actions in C1
- **Areas:** existing: /empresas/ scaffold and review APIs · proposed: approval inbox
- **Steps:** Test narrow scope, decision conflicts and accessibility through the company session.
- **Deliverable:** An accessible approval inbox with truthful server results.
- **Acceptance:** A reviewer sees only authorized items and server-confirmed decisions.
- **Negative cases:** Foreign item after context switch, stale result, unaudited approval or missing reason hidden by the UI.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-APPR-007 — App: my submissions and returned items
- **Meta:** module APPR · phase F3 · cut - · priority P1 · type APP · size S · state TODO
- **Objective:** Show a member their own submissions, returned reasons and pending corrections.
- **Rules and use cases:** BR-REV-002, BR-REV-005, BR-REV-007, BR-OFFL-010, BR-OFFL-013, BR-TEN-004
- **Depends on:** EMP-APPR-001, EMP-OFFL-004
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
- **Scope:** in: Android submissions and correction entry with accurate state · out: offline approval or other members' data
- **Areas:** existing: app sync status and review outcome feed · proposed: own-submissions UI
- **Steps:** Exercise returned, resubmitted, offline and stale-revision flows in the original context.
- **Deliverable:** A scoped Android submissions and correction view.
- **Acceptance:** The member sees the original reason and current revision. Durable pending work is not shown as accepted early.
- **Negative cases:** Foreign record, correction retagged, missing reason or terminal decision inferred from transport success.
- **Validation:** V-APP, V-SYNC, V-AUTHZ, V-ISO · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-EXP-003 — Expense categories configuration
- **Meta:** module EXP · phase F3 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Configure scoped expense categories without losing historical references.
- **Rules and use cases:** BR-TEN-003, BR-AUD-001, BR-ORG-012, REQ-11.1
- **Depends on:** EMP-EXP-002
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
- **Scope:** in: authorized category lifecycle and current availability · out: renaming past expense identity or deleting used categories
- **Areas:** existing: scoped expenses and organization settings · proposed: category commands
- **Steps:** Test concurrent duplicate, stale edit and retired category usage. Preserve references in old records.
- **Deliverable:** An audited category configuration API.
- **Acceptance:** Only allowed categories are selectable for new operations and history remains readable.
- **Negative cases:** Foreign category, stale edit, retired category accepted unexpectedly or historical cost reclassified silently.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-EXP-005 — Panel: expenses list, detail and review
- **Meta:** module EXP · phase F3 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Expose scoped expense lists, detail and review in the company panel.
- **Rules and use cases:** BR-REV-003, BR-REV-004, BR-REV-007, BR-REV-009, BR-AUTH-011, REQ-11.1
- **Depends on:** EMP-EXP-002, EMP-APPR-001, EMP-PLAT-008
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
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
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
- **Scope:** in: exact allocations and persistent scoped history · out: counting allocations as additional expense
- **Areas:** existing: exact allocation rule and organization dimensions · proposed: transaction-backed expense allocations
- **Steps:** Pin residual ordering, concurrent allocations and foreign targets. Check totals before commit.
- **Deliverable:** Scoped allocations with exact reconciliation evidence.
- **Acceptance:** Accepted splits sum to the source amount and retain the historical target scope.
- **Negative cases:** Allocation exceeds the source, fractional cent lost, foreign target or total counted once per allocation as a whole.
- **Validation:** V-SQL, V-MONEY, V-ISO, V-IDEM · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FIN-002 — Reimbursements for out-of-pocket expenses
- **Meta:** module FIN · phase F3 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Create a scoped reimbursement obligation for approved employee-paid expenses.
- **Rules and use cases:** EDR-013, BR-REV-003, BR-REV-009, BR-MONEY-001, REQ-11.2, VAL-13
- **Depends on:** EMP-FIN-001, EMP-EXP-002, EMP-APPR-001
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
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
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
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
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
- **Scope:** in: scoped payment persistence and payable settlement · out: initiating provider payments or recording a second cost
- **Areas:** existing: payable and expense stores and audited commands · proposed: payment records and idempotent settlement
- **Steps:** Test exact amounts, partial and complete settlement, lost response and concurrent retry.
- **Deliverable:** A durable payment record with one settlement effect per authorized operation.
- **Acceptance:** Recording or replaying payment changes cash and obligation views only. The original cost remains once.
- **Negative cases:** Duplicate payment, invalid total, partial failure, foreign obligation or provider settlement inferred from local recording.
- **Validation:** V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-MONEY · migration: prove additive upgrade and recovery if persistent state changes · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-FIN-007 — Panel: payables, reimbursements and payments
- **Meta:** module FIN · phase F3 · cut - · priority P1 · type WEB · size M · state TODO
- **Objective:** Present payables, reimbursements and recorded payments in the company panel.
- **Rules and use cases:** EDR-013, BR-TRUTH-001, BR-AUTH-013, BR-RPT-002, REQ-11.1
- **Depends on:** EMP-FIN-002, EMP-FIN-004, EMP-PLAT-008
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
- **Scope:** in: scoped lists and authorized settlement actions · out: real payment initiation and duplicating expense totals
- **Areas:** existing: /empresas/ scaffold and financial record APIs · proposed: finance screens
- **Steps:** Verify scope, clear pending and settled states, permission denial and exact amount display.
- **Deliverable:** Accessible company settlement views.
- **Acceptance:** The member can distinguish an obligation, recognized expense and recorded payment.
- **Negative cases:** Foreign account data, apparent payment execution, rounded base amount or stale state shown as settled.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-MONEY, V-PRIV · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.

### EMP-BUDG-002 — Budget persistence and concurrent consumption posting
- **Meta:** module BUDG · phase F3 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Persist budget lines and post consumption under concurrency.
- **Rules and use cases:** BR-MONEY-001, BR-AUTH-001, BR-REV-009, REQ-11.3, VAL-14
- **Depends on:** EMP-BUDG-001, EMP-EXP-002
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
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
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
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
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
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
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
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
- **Read first:** [shared rules](../business-rules.md) §§4–8, [domain model](../domain-model.md) §§3–5, expense and finance requirements §11 and [validation scenarios](../testing.md) §4
- **Scope:** in: scope and time-aware limit decisions and review evidence · out: treating cached data or client time as authority
- **Areas:** existing: budget lines, fleet scope and sync history · proposed: pure limit decisions
- **Steps:** Pin exact boundary, changed configuration, wrong asset and stale reference cases.
- **Deliverable:** Deterministic limit decision vectors for online and synchronized submissions.
- **Acceptance:** The current scoped policy and recorded history decide the disposition. Offline facts follow the documented review rules.
- **Negative cases:** A stale cache authorizes an overrun, scope changes silently, missing limits treated as a guessed amount or no reason retained.
- **Validation:** V-DOM, V-SYNC, V-AUTHZ, V-MONEY · migration: none in this task · rollback: close new admission and preserve accepted costs, exact balances, decisions and audit
- **Risks and assumptions:** The [expense and finance design](../modules/expenses-finance.md) and the owning reviewed policy govern detailed states and choices.
- **Done when:** The stated outcomes and negative cases pass the applicable profiles at the affected entry points.
