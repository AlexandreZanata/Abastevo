# Expenses, approvals, budgets and operational finance

Status: **PLANNED / RECONSTRUCTED**, 2026-11-11.
Recovered from the surviving enterprise requirements, shared rules and backlog.
The accepted F0 decisions and existing release gates govern any implementation.

## 1. Purpose

Give each economic cost one source, one review history and an exact company amount. Managers track obligations and settlement without recognizing the same expense again.

## 2. Scope

F3 delivers single-level C1 review plus complete expenses, policies, periods, budgets, holds, payables and recorded settlement. F6 extends recurring costs, statements and accounting views. Actual money movement, tax determination and bank access remain separately gated.

## 3. Entities and ownership

The organization owns expense, allocation, review case, approval step, payable, advance, payment, budget line and hold. A fueling-origin expense has a unique source. A payment settles an obligation. It does not create an expense. Structural targets remain inside the owning organization.

## 4. Business rules

- **BR-EXP-001 — One origin, one cost.** Fuelings and maintenance origins produce one expense each under EDR-013. Documents, reimbursements and statements never duplicate it.
- **BR-APPR-001 — Current revision and independent decision.** Shared BR-REV rules govern each approval. All required levels concern the same revision. The author and beneficiary cannot approve.
- **BR-FIN-001 — Settlement is distinct.** Exact obligation and payment amounts feed settlement views independently of cost recognition.
- **BR-BUDG-001 — Atomic remaining capacity.** Available budget is derived from committed consumption and active holds. Concurrent reservations serialize against the line and cannot spend the same remaining capacity twice.
- **BR-EXP-002 — Review basis.** Approved company amounts alone enter approved-cost totals. Pending views identify their pending part.

## 5. Use cases

**UC-EXP-01 — Submit a cost.** An authorized member creates a scoped source, category, actual amount and competence instant. Durable intake creates one record or returns its stored outcome. A duplicate source cannot create another cost.

**UC-APPR-01 — Return and decide.** A current reviewer returns a revision with a reason. The author resubmits a new revision. Required levels decide that revision. A stale decision fails, and partial approval cannot exceed the submitted company amount.

**UC-FIN-01 — Record reimbursement.** An eligible approved employee-paid cost produces one payable. An authorized member records an external payment once. The expense remains unchanged and the payable's remaining amount is derived from valid settlements.

**UC-BUDG-01 — Reserve a limit.** An authorized request selects its scoped line and amount. A transaction checks the remaining capacity and creates one active hold, or follows the allowed exception workflow. Release and expiry restore capacity once.

## 6. States and transitions

Expenses use BR-REV states plus void, preserving revisions. Approval steps use pending, approved, returned, rejected or explicitly skipped under their policy. Payables are open, partially settled, settled or cancelled. Advances retain their requested, approved, paid and accounted stages. Holds are active, consumed, released or expired. Periods are open, locked or reopened. The [state catalog](../08-state-model.md) is canonical. Every transition checks actor, scope, current revision and required reason. A locked-period correction uses authorized reopening or an open-period adjustment.

## 7. Roles and permissions

Use the fixed [permission matrix](../07-permissions.md). Expense read, record, review, allocation, budget management and financial settlement are separate powers. Scope is resolved at the server. A finance role does not remove self-approval or fresh-auth requirements. Auditor read never implies writes or full export.

## 8. Offline, concurrency, idempotency and corrections

Field entry uses the sealed enterprise queue. Approval, financial authorization and reservations requiring concurrent limits are online-only. Intake uses current server authority and retains eligible conflicts for review. Posting, source uniqueness, allocation and budget consumption are transactional. Amendments preserve approved history and trigger versioned derived data.

## 9. Events, notifications and integrations

Accepted source changes feed the expense projection and review component through existing jobs. Notification digests target currently permitted recipients. Accounting adapters consume private authorized exports. Actual payment providers remain independent from local payment-record commands.

## 10. Data, formulas and examples

Money is integer centavos. A fueling costing 97 350 creates one expense of 97 350. A matching reimbursement and payment leave cost at 97 350. For limit 500 000 with 450 000 used or held, 50 000 remains. Two concurrent holds of 30 000 cannot both activate. A split uses largest remainder so its centavos sum exactly to the source.

## 11. Screens

| ID | Surface | Objective | Actor | Data and actions | Validation |
|---|---|---|---|---|---|
| SCR-EXP-01 | APP | Record a scoped expense | Permitted member | Amount, category, private attachment and send state | Durable offline save, exact input, five shared states |
| SCR-APPR-01 | PANEL | Review the current submission | Scoped reviewer | Revision, reasons, levels and decision | No self-approval, stale conflict, accessible keyboard actions |
| SCR-FIN-01 | PANEL | Distinguish cost and settlement | Finance member | Payables, payments and exact remaining amount | No provider-success claim, current scope and explicit errors |
| SCR-BUDG-01 | PANEL | Manage budget basis | Budget manager | Limit, held, consumed and remaining | Calculated balance cannot be edited, scope and period visible |

## 12. Acceptance, dependencies, risks, phase and microtasks

[EP10](../backlog/ep10-expenses-finance.md) owns tasks and dependencies. Immediate PostGIS, money and concurrency proofs include VAL-09, VAL-13, VAL-14, VAL-19 and VAL-25 in [the validation plan](../15-validation.md). No source consumer bypasses its approval, legal or activation gate. Detailed approval thresholds remain reviewed policy data.
