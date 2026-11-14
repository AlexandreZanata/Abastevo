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

### EMP-DOC-001 — document domain: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f3-doc001-document-domain.md). Pure Go `document/domain` and
Kotlin `enterprise.document` share the 251-case `document-v1.psv` oracle: six
independent status dimensions with fixed transition tables (no dimension
derives another; a digest never makes a document conferred or fiscal-
authorized), original identity with replacement by a stored successor,
same-organization links over closed target kinds where only EXPENSE is
allocatable (EDR-135, so a fueling and its expense cannot both be charged),
and exact allocations with derived reconciliation (zero tolerance,
OVER_ALLOCATED only after a downward correction, even split via
`orgvalue.Allocate`). RED: Go 13/14 top-level and 249/251 subtests, Kotlin
259/260 failing → GREEN: Go 14/256 under `-race` (99.5% statements; the miss
is an unreachable defensive branch), Kotlin enterprise suite 2089/0/0 on this
branch. B-BR-EMP-185..189, BUC-EMP-96..97, EDR-134..136 moved to canonical
docs; glossary "Reconciled" states zero tolerance. Retained: DOC-002..014
persistence/upload/links/conference/fiscal/DB no-double-counting/jobs; real
receipts stay legally gated (GOV-002).

### EMP-BUDG-001 — budget domain: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f3-budg001-budget-domain.md). Server-only Go `budget/domain` with
the 261-case `budget-v1.psv` oracle (EDR-141: no Kotlin mirror): one-dimension
lines (ORGANIZATION, UNIT, COST_CENTRE, ASSET, EMPLOYEE, optional category)
over explicit half-open reporting periods with a required exact limit (no
default, no unlimited); capacity = limit − approved company amounts − active
holds in exact big integers, pending reported but never deducted, a claimed
"available" only compared (STALE_AVAILABLE), reservation ACTIVATE/EXCEEDS and
posting decisions, holds ACTIVE → CONSUMED/RELEASED/EXPIRED once (EDR-142),
including the serialized VAL-14 sequence. RED 11/13 top-level and 261/261
subtests failing → GREEN 14/261 under `-race`, 100% statements; re-run on this
branch. B-BR-EMP-198..201, BUC-EMP-102..103, EDR-141/142 moved to canonical
docs. Retained: persistence, fixed-order line locks and the real PostGIS
VAL-14 race (BUDG-002), reservation API/expiry (BUDG-003), overrun policy
(BUDG-004), panel (BUDG-005), driver/asset limits (BUDG-006).

### EMP-FIN-001 — payables domain: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f3-fin001-payables-domain.md). Server-only Go
`expense/domain/payable` with the 275-case `payable-v1.psv` oracle (EDR-139:
no Kotlin mirror). Closed kinds SUPPLIER, REIMBURSEMENT, CARD_STATEMENT
(advances stay FIN-003) from APPROVED expenses with an explicit payer; lines
bounded by the source company amount, reimbursement exactly equal; state
OPEN/PARTIALLY_SETTLED/SETTLED derived from exact settlement entries,
overpayment refused, reasoned cancellation only from OPEN; separate cost and
settlement views where an obligation or payment never becomes a cost and one
expense backs one live payable (EDR-140). RED 15/17 top-level and 270/275
subtests failing → GREEN 17/275 under `-race`, 100% statements; dependents
green; re-run on this branch. B-BR-EMP-194..197, BUC-EMP-100..101,
EDR-139/140 moved to canonical docs; expenses-finance §6 states the derived
payable state. Retained: reimbursement API (FIN-002), advances (FIN-003),
payment records and the DB single-cost invariant (FIN-004), statements,
cash views, panel; an exported review reason check for reuse.

### EMP-PRIV-005 — sensitive-access log: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f3-priv005-sensitive-access-log.md). `orgaudit` gains the
`AccessRecorder` contract (categories DOCUMENT_CONTENT, EXPORT_FILE,
LOCATION_LIVE, LOCATION_TRACK, each bound to its catalog permission) and
migration 000202 `org_sensitive_access_events`: one append-only detail row per
sensitive audit event in the same runner transaction, forced RLS, a guard
linking a served capability to its member-bound issuance, and a deferred check
that no sensitive event commits without its detail. `orgmedia` now records
before minting a capability, serves nothing on any error and maps a missing
content permission to FORBIDDEN. RED unit 7/34 and integration 10/18 new tests
failing → GREEN; agent sweep: untagged backend 192 packages, 22 enterprise
integration packages, organization adapters 340/2369 alone. Re-run on this
branch with a disposable MinIO from `infra/compose.validation.yml`
(project `f3-s3-20261114`, CI credentials): orgmedia + orgaudit integration
53 top-level / 38 subtests, 0 failures, 0 skips, including the two S3
access-path tests that the OFFL-008 closure could not run. B-BR-EMP-207..210,
BUC-EMP-106..107, EDR-146/147 moved to canonical docs. Retained: real document
authority/403 mapping (DOC-002/AUTH-003), export producer (RPT-001), location
producers (F4), support sessions (BACK-003; operator access impossible until
then), an audit read route, production grants on the new table.

### EMP-FLEET-003 (F2 prerequisite) — asset lifecycle: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f2-fleet003-asset-lifecycle.md). First link of the EDR-109 chain.
Migration 000201 adds acquisition/disposal columns, append-only
`org_asset_lifecycle_events` and guards that refuse a state change without a
history row; commands acquisition/record, deactivate, reactivate, dispose,
disposal/revert, archive (fresh `fleet.asset.manage`, revision CAS, one audit
event, replay) and lifecycle/list; an explicit open-work port answered by
`NoOpenWork` until trips/work orders exist; a reading declared at/after the
disposal instant is held as ASSET_UNAVAILABLE through the OFFL-008 review
(EDR-143..145). RED unit 16+13, PostGIS 11/11 (no migration) and 10+30
(stubbed) failing → GREEN: unit 191/1362, focused PostGIS 14/30, affected
race union 510/3573 and untagged 190 packages, 0 failures. The 198/199 upgrade
tests now apply later migrations before current-code smoke checks (their
upgrade assertions are unchanged). Retained: real open-work sources (trips,
maintenance), allocation periods (FLEET-005), ending assignments on disposal
(FLEET-006), reservations (FLEET-007), holds for DRAFT/INACTIVE assets via
derived availability, plan limit on reactivation, SCR-FLEET-05, live grants.
Integration-branch re-run after PRIV-005 (`59b57c19` tree): untagged backend
196 packages, 2498 top-level / 6827 subtests, 0 failures (4 pre-existing
opt-in skips); race/integration union over fleet, organization adapters,
orgsync, orgaudit, orgmedia (disposable MinIO) and migrate: 17 packages,
573 top-level / 3626 subtests, 0 failures, 0 skips.

### EMP-APPR-006 — period closing, lock and reopening: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f3-appr006-period-lock.md). Migration 000203 adds the immutable
`org_finance_period_transitions` history (also the command receipt) and the
trigger-maintained head `org_finance_periods` (forced RLS, guards).
`review/domain/period`, application and adapters provide organization-wide
calendar-month periods (implicit OPEN revision 1; LOCK, REOPEN with reason,
RELOCK; history never deleted), routes `finance/periods/{list,read,close,
reopen}` under fresh `fin.period.close`/`.reopen` ORG grants, and an
in-transaction `PeriodGate` (organization row FOR KEY SHARE vs commands FOR
UPDATE) whose fact feeds `orgamend` (EDR-148/149). RED unit 23 and integration
14 failing → GREEN; agent union 382/2914 with a 170-cell authorization matrix;
populated 200→203 upgrade/failure/recovery; zone boundary (America/Cuiaba),
8-way concurrent close, lock-vs-write race both orderings with barriers and a
test-only `orgamend` consumer (PERIOD_LOCKED → adjustment, reopen → amendment,
relock → refused). Merged with FLEET-003 (route inventory 17 routed / 67 not
routed / 93 routes); merged-tree union over review, organization adapters,
fleet adapters and migrate: 9 packages, 441 top-level / 3057 subtests,
0 failures, 0 skips; build/vet (both tags)/sqlc/gofmt/cmd/api/apicontract
clean. Retained: real consumers calling the gate and `PERIOD_LOCKED` sync
binding, unit/cost-centre periods (AUTH-006), an AUDITOR read key, panel,
production grants.

### EMP-NOTIF-001 — notification core: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f3-notif001-notification-core.md). New module `orgnotify` and
migration 000204 (seven forced-RLS tables: requests, recipients, notices,
delivery history, preferences, hourly budgets, member receipts, with guards).
Producers register keyed requests inside their own audited transaction; the
`notif.dispatch` consumer re-checks current AUTH-003 authority per recipient,
honours per-kind preferences (mandatory kinds cannot be muted), caps 20 notices
per recipient per kind per UTC hour and records exactly one IN_APP history row
(DELIVERED or SUPPRESSED INELIGIBLE/MUTED/LIMITED). Four SELF_NOTICE member
routes (list, read mark, preference read/update) with replay and revision CAS
(EDR-150/151). No kind, producer or worker is active in production. RED
unit 13/14, contract 2/3, inventory 1/3, integration 12/12 failing → GREEN:
unit 14/81, integration 24/106, contract 89/641, composition users 345/2379,
untagged 199 packages; merged tree (after FLEET-003/APPR-006) union over
orgnotify, organization adapters, orgaccess, review, enterprisesuite and
migrate: 16 packages, 509 top-level / 4259 subtests, 0 failures, 0 skips.
Retained: real producers (RPT-004, approvals, alerts) and dispatcher
composition, external channels (NOTIF-002), subscriptions (NOTIF-003), app
centre (NOTIF-004), retention/erasure of notices, rate measurement, grants.

### EMP-DOC-002 — document persistence and audited access: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f3-doc002-document-persistence.md). Migration 000205
`org_documents` owns the orgmedia original identity; six independent status
columns with the DOC-001 graph enforced in SQL, forced RLS, retention guard;
upload create (≤5-minute capability), capability-bound `DOCUMENT_INGRESS`
(JPEG only in C1; RECEIVED/STORED only after verified bytes), audited content
access/read through the PRIV-005 log (EDR-152/153). Merged with FLEET-003,
APPR-006 and NOTIF-001 (route inventory 19 routed / 65 not routed / 96 normal
/ 24 special; authorization matrix 190 cells, 72 allowed / 118 denied;
OpenAPI blocks inserted structurally). Merged-tree verification: untagged
206 packages, 2551/7034, 0 failures (4 pre-existing skips); race/integration
union with disposable MinIO over document, orgmedia, orgaudit, organization
adapters, review, orgnotify, fleet adapters, orgapi and migrate: 24 packages,
580/3773, 0 failures, 0 skips. Retained: Android producer (DOC-004/OFFL-005),
safety job (DOC-014), other status commands, links, duplicates, panel viewer,
real storage provider, production grants; real receipts legally gated.

## Stop point — 2026-11-14 (usage limit)

Integrated here: opening ledger, PLAT-016, APPR-001, EXP-001, DOC-001,
BUDG-001, FIN-001, PRIV-005, FLEET-003 (F2), APPR-006, NOTIF-001, DOC-002.
Not integrated: EMP-FLEET-005 is LOCAL_DONE on branch
`codex/enterprise-f3-fleet005` (`dc83111e`, migration 000206, backed up to
origin) and must be cherry-picked with its route-count/usage-guard merges;
EMP-RPT-001 was stopped mid-work in `/data/dev/worktrees/enterprise-f3-rpt001`
(uncommitted, unvalidated). Not started: AUTH-006, FLEET-006, METER-002
closure, FUEL-002 and everything downstream, DOC-003/006/008/014, AUTH-007/011,
FLEET-009, OFFL-005, RPT-002 and REL-004. M3/F3 are not accepted. The branch
is not yet merged with origin/dev `018f2df1`, not pushed to `dev`, and no PR
is open. The disposable MinIO project `f3-s3-20261114` is still running.
