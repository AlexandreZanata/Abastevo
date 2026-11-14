# F3 — Execution ledger: fuelings, documents, expenses and approvals

Date: 2026-11-14.
Status: IN_PROGRESS
Validation: PENDING
Integration: INTEGRATION_PENDING

Opened by Claude Code from the coordinator session after PR203 merged
`dev → main` at `43b21389` (origin/dev = origin/main). Owned branch
`codex/enterprise-f3` in a dedicated worktree on the data volume (the root
volume had 8 GB free; the primary checkout and the other worktrees hold other
people's work and are untouched). The local `dev` branch with three unpublished
2026-10-24 Android commits is ignored as instructed. The scope is the
[authorized F3 sequence](f3-authorized-sequence.md): finish open prerequisites,
then implement F3 one bounded task at a time in dependency order, without
claiming M3/F3 acceptance from labels or partial work. No merge, deployment,
real company data, real receipt retention or real money movement is authorized.

## Reconciliation against source (2026-11-14, `43b21389`)

The canonical backlog lists 67 F3 tasks (113 relative cycles). Source state:

- **Consumed by C1/F2, not reimplemented:** EMP-FUEL-001 is LOCAL_DONE
  (`21601ae8`, [evidence](f2-fuel001-supply-domain.md)). It counts once.
- **Dependency-ready now (10):** APPR-001, AUTH-007, AUTH-011, DOC-001,
  EXP-001, FLEET-009, NOTIF-001, OFFL-005, PLAT-016, PRIV-005.
- **Blocked (56):** every persistence/API/app/panel task downstream of the open
  prerequisites below, plus FIN-001 and BUDG-001 which wait only for EXP-001.
- **Partially present in source:** PLAT-006 (`orgmedia`) already audits
  private original issue/read (`document.access.issued`,
  `document.content.accessed`); EMP-PRIV-005 generalizes this to exports and
  location and must reuse it, not duplicate it.

Open prerequisites outside F3 (12 nodes in the transitive closure):

| Task | Phase | State | Gates in F3 |
|---|---|---|---|
| EMP-METER-002 | F2 | IN_PROGRESS (assignment/history open, EDR-109) | FUEL-002 → every fueling/expense/budget/finance persistence task; IMPORT-004 |
| EMP-AUTH-006 | F2 | TODO, ready | APPR-002 → APPR-003/004, BUDG-004; part of the METER-002 closure chain |
| EMP-FLEET-006 | F2 | TODO, ready | OFFL-006 → FLEET-013 → FUEL-011; METER-002 closure |
| EMP-OFFL-006, EMP-FLEET-013 | F2 | TODO | FUEL-011 (app fueling) |
| EMP-METER-003, EMP-METER-005 | F2 | TODO | METER-006 → FUEL-007 → FUEL-008/013 (corrected consumption, M3) |
| EMP-IMPORT-001 | F2 | TODO | IMPORT-004 |
| EMP-GOV-004 → EMP-BACK-001 | F0/F1 | TODO | BACK-003 |
| EMP-GOV-014 → EMP-PLAT-012 | F0/F1 | TODO | NOTIF-002 |

EDR-109 additionally sequences FLEET-003 and FLEET-005 (F2, ready) before
AUTH-006/FLEET-006 and the remaining METER-002 closure; they are not formal
backlog dependencies of F3 but gate its first persistence task in practice.

Critical chain to the M3 gate (EMP-REL-004 needs DOC-010, FIN-004, BUDG-004,
APPR-004 and FUEL-008):

```
FLEET-003 → FLEET-005 → AUTH-006 → FLEET-006 → METER-002 closure
  → FUEL-002 (L) → EXP-002 → BUDG-002 → BUDG-004 (+ APPR-002 ← AUTH-006)
                           → FIN-004 (+ FIN-001)
                           → DOC-007 (+ DOC-006 ← DOC-002 ← DOC-001, PRIV-005) → DOC-010
  → METER-003 → METER-005 → METER-006 → FUEL-007 → FUEL-008
APPR-001 → APPR-002 → APPR-004
```

## Execution decision (EDR-127)

Start with the dependency-ready pure domains, each a bounded task with its own
rules, shared vectors and RED→GREEN, built in isolated owner worktrees and
integrated here as one atomic commit per task: APPR-001, EXP-001, DOC-001 and
PLAT-016 first, then FIN-001 and BUDG-001 on top of EXP-001. They pin the review,
one-cost, settlement, capacity and amendment invariants that REL-004 tests at
persistence level. The F2 chain above remains the gate of every F3 persistence
task: no fueling, expense, budget or payable table is created before
METER-002's actual closure, and no fake meter, assignment or scope adapter is
introduced to get there. Pure-domain checkpoints never count as M3.

## Task ledger

Status column is the backlog state at opening; the readiness column lists the
open direct dependencies.

| Task | Title | Cut | Type/size | State | Open dependencies |
|---|---|---|---|---|---|
| EMP-APPR-001 | Review lifecycle component: submit, approve, return, reject, no self-approval | C1 | DOM M | TODO | ready |
| EMP-APPR-002 | Approval policies: thresholds, levels and routing by scope | - | API M | TODO | APPR-001, AUTH-006 |
| EMP-APPR-003 | Substitutes and delegation in approval routing | - | DOM S | TODO | APPR-002, AUTH-007 |
| EMP-APPR-004 | Partial approval and the return-for-correction loop | - | DOM S | TODO | APPR-002 |
| EMP-APPR-005 | Panel: approvals inbox | C1 | WEB M | TODO | APPR-001 |
| EMP-APPR-006 | Period closing, lock and reopening | - | API M | TODO | APPR-001 |
| EMP-APPR-007 | App: my submissions and returned items | - | APP S | TODO | APPR-001 |
| EMP-AUTH-007 | Temporary delegation and substitutes | - | API S | TODO | ready |
| EMP-AUTH-011 | Auditor read-only role and export permission gating | - | API S | TODO | ready |
| EMP-BACK-003 | Support access: time-boxed, organization-approved and audited | - | API M | TODO | BACK-001 |
| EMP-BUDG-001 | Budget domain: lines, periods, dimensions and amounts | - | DOM S | TODO | EXP-001 |
| EMP-BUDG-002 | Budget persistence and concurrent consumption posting | - | DB M | TODO | BUDG-001, EXP-002 |
| EMP-BUDG-003 | Reservations: pre-authorisation, release and expiry | - | API M | TODO | BUDG-002 |
| EMP-BUDG-004 | Overrun policies and exception approval | - | API S | TODO | BUDG-002, APPR-002 |
| EMP-BUDG-005 | Panel: budgets setup and monitoring | - | WEB M | TODO | BUDG-002 |
| EMP-BUDG-006 | Driver and asset limits at submission and after sync | - | DOM S | TODO | BUDG-002 |
| EMP-DOC-001 | Document domain: container, independent statuses, links and allocations | C1 | DOM M | TODO | ready |
| EMP-DOC-002 | Document persistence, upload session and audited private access | C1 | API M | TODO | DOC-001, PRIV-005 |
| EMP-DOC-003 | Links between documents and operations | C1 | API S | TODO | DOC-002 |
| EMP-DOC-004 | App: capture and import with offline queue and legibility check | C1 | APP M | TODO | DOC-002, OFFL-005 |
| EMP-DOC-005 | Conference workflow | - | API S | TODO | DOC-003 |
| EMP-DOC-006 | Fiscal structure: access key, NFC-e QR code and XML structural validation | - | DOM M | TODO | DOC-002 |
| EMP-DOC-007 | Fiscal items and allocations without double counting | - | DB M | TODO | DOC-006, EXP-002 |
| EMP-DOC-008 | Duplicate detection by access key and content hash | - | DB S | TODO | DOC-002 |
| EMP-DOC-009 | Optional on-device extraction with per-field confidence | - | APP M | TODO | DOC-004 |
| EMP-DOC-010 | Reconciliation status between documents and operations | - | JOB M | TODO | DOC-007 |
| EMP-DOC-011 | Replacement and cancellation of documents | - | API S | TODO | DOC-005 |
| EMP-DOC-013 | Panel: document inbox, viewer, conference and allocation | - | WEB M | TODO | DOC-005, DOC-007 |
| EMP-DOC-014 | Upload safety checks: type, size and active content | C1 | JOB S | TODO | DOC-002 |
| EMP-DOC-015 | Panel: receipt viewer inside fueling review | C1 | WEB S | TODO | DOC-003, FUEL-012 |
| EMP-EXP-001 | Expense domain: categories, origin, competence date and company amount | - | DOM M | TODO | ready |
| EMP-EXP-002 | Expense persistence, sync handlers and the fueling-to-expense projection | - | DB M | TODO | EXP-001, FUEL-002 |
| EMP-EXP-003 | Expense categories configuration | - | API S | TODO | EXP-002 |
| EMP-EXP-004 | App: add an expense with receipt | - | APP M | TODO | EXP-002, DOC-004 |
| EMP-EXP-005 | Panel: expenses list, detail and review | - | WEB M | TODO | EXP-002, APPR-001 |
| EMP-EXP-006 | Cost allocation across cost centres and assets | - | DB M | TODO | EXP-002 |
| EMP-FIN-001 | Payables domain: supplier, reimbursement and card statement obligations | - | DOM S | TODO | EXP-001 |
| EMP-FIN-002 | Reimbursements for out-of-pocket expenses | - | API M | TODO | FIN-001, EXP-002, APPR-001 |
| EMP-FIN-003 | Advances and settlement against expenses | - | API M | TODO | FIN-002 |
| EMP-FIN-004 | Payment records and the single-cost invariant | - | DB M | TODO | FIN-001, EXP-002 |
| EMP-FIN-007 | Panel: payables, reimbursements and payments | - | WEB M | TODO | FIN-002, FIN-004 |
| EMP-FLEET-009 | Supplier registry | - | API S | TODO | ready |
| EMP-FUEL-001 | Fueling domain: products, units, payer and supplier roles | C1 | DOM M | LOCAL_DONE | ready |
| EMP-FUEL-002 | Fueling persistence, sync handlers and arithmetic consistency | C1 | DB L | TODO | METER-002 |
| EMP-FUEL-003 | Fueling review: check, approve, reject, return, cancel | C1 | API M | TODO | FUEL-002, APPR-001 |
| EMP-FUEL-004 | Supplier linkage: canonical station, organization supplier or not found | - | API M | TODO | FUEL-002, FLEET-009 |
| EMP-FUEL-005 | Capacity tolerance and product compatibility validation | - | DOM S | TODO | FUEL-002 |
| EMP-FUEL-006 | Suspected duplicate versus technical retry | - | DOM S | TODO | FUEL-002 |
| EMP-FUEL-007 | Consumption engine: full-to-full intervals with partial fuelings | - | DOM L | TODO | FUEL-002, METER-006 |
| EMP-FUEL-008 | Consumption projections and recalculation after corrections | - | JOB M | TODO | FUEL-007 |
| EMP-FUEL-011 | App: add a fueling offline with receipt and meter reading | C1 | APP M | TODO | FUEL-002, OFFL-005, FLEET-013, DOC-004 |
| EMP-FUEL-012 | Panel: fuelings list, detail and review | C1 | WEB M | TODO | FUEL-003 |
| EMP-FUEL-013 | Retroactive fuelings and their effect on intervals | - | DOM S | TODO | FUEL-007 |
| EMP-FUEL-014 | Price reference snapshot at fueling time | - | API M | TODO | FUEL-004 |
| EMP-FUEL-015 | Fueling amendment after approval and period lock | - | API S | TODO | FUEL-003, PLAT-016, APPR-006 |
| EMP-IMPORT-004 | Import historical fuelings and meter readings | - | API M | TODO | IMPORT-001, FUEL-002, METER-002 |
| EMP-METER-006 | Current-value projection and usage-between-instants query | - | DB S | TODO | METER-005 |
| EMP-NOTIF-001 | Notification core: inbox, preferences, de-duplication, limits and history | - | DB M | TODO | ready |
| EMP-NOTIF-002 | Delivery channels: Android local and push, and e-mail, with retries | - | INT M | TODO | NOTIF-001, PLAT-012 |
| EMP-NOTIF-003 | Operational alert subscriptions | - | API S | TODO | NOTIF-001 |
| EMP-NOTIF-004 | App and panel notification centres | - | WEB M | TODO | NOTIF-001 |
| EMP-OFFL-005 | Attachment upload: offline queue, retry and resume, linkage to operations | C1 | APP M | TODO | ready |
| EMP-PLAT-016 | Amendment kernel: append-only corrections for approved records | - | DOM S | TODO | ready |
| EMP-PRIV-005 | Access log for sensitive reads: documents, location and exports | C1 | API S | TODO | ready |
| EMP-REL-004 | M3 gate: fuelings, documents, expenses and approvals acceptance | - | TEST M | TODO | DOC-010, FIN-004, BUDG-004, APPR-004, FUEL-008 |
| EMP-RPT-001 | Asynchronous export infrastructure: job, private file and expiring link | - | JOB M | TODO | PRIV-005 |
| EMP-RPT-002 | CSV and spreadsheet exports for operational lists | - | API M | TODO | RPT-001, AUTH-011 |

## Execution log

Entries are appended per integrated task (commit, evidence, validation).

### EMP-PLAT-016 — amendment kernel: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f3-plat016-amendment-kernel.md). Pure server-side `orgamend/domain`
with the 193-case shared oracle (`amendment-v1.psv`); genuine RED (7/9
top-level, 193/193 subtests failing on assertions against compiled stubs) →
GREEN 9/9 and 193/193 under `-race`, 100% statement coverage, gofmt/vet/
staticcheck/check_plan clean, re-run green on this branch. B-BR-EMP-190..193,
BUC-EMP-98..99 and EDR-137/138 moved verbatim to the contract and decisions.
Added to the quick manifest. Retained: review binding (APPR-001), period
commands (APPR-006), consumer persistence/recalculation (FUEL-015 and others),
reporting-day moves across periods, runtime VAL-19/20/26.

### EMP-APPR-001 — review lifecycle component: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f3-appr001-review-lifecycle.md). Pure Go `review/domain` and Kotlin
`enterprise.review` share the 232-case `review-v1.psv` oracle (states,
commands, apply, replay, presentation). RED: Go 7/9 top-level and 230/230
subtests, Kotlin 237/238 failing against compiled stubs → GREEN: Go 9/232
under `-race` (100% statements), Kotlin 240 review tests and the whole
enterprise domain suite 1591/0 failures/0 skips (re-run on this branch).
B-BR-EMP-175..179, BUC-EMP-92..93, EDR-128..130 moved to the canonical docs;
suite added to the quick manifest. Retained: owner embedding, persistence of
cases/history, receipt-time authority and audit, wire-code mapping, real
PostgreSQL concurrency, levels/partial amounts (APPR-002/004), period lock
(APPR-006), panel/app.

### EMP-EXP-001 — expense domain: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f3-exp001-expense-domain.md), commit `0afb1329`. Pure Go
`expense/domain` and Kotlin `enterprise.expense` share the 222-case
`expense-v1.psv` oracle: closed cost origins (FUELING, WORK_ORDER, DIRECT;
settlement/evidence/stock kinds refused), exact paid/company centavos without
a false zero, structural category key, competence reporting day and
approved-versus-pending totals that fail closed. A receipt-free
`AttributeReportingDay` is extracted from the time kernel and reused by
Capture unchanged (EDR-094 precedent). RED: Go 222/222 vectors and 14/15
top-level, Kotlin 236/237 failing → GREEN: Go 15+8 top-level / 222+99 cases
under `-race` (100% statements), affected time-kernel consumers green;
Kotlin enterprise suite 1829/0/0 on this branch. B-BR-EMP-180..184,
BUC-EMP-94..95, EDR-131..133 moved to canonical docs. Retained: persistence,
uniqueness and fueling projection (EXP-002, VAL-13), binding `ReviewState` to
the integrated `review` vocabulary, categories (EXP-003), allocation (EXP-006).
