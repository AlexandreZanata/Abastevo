# EMP-FIN-001 — Payables domain: supplier, reimbursement and card statement obligations

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F3 pure-domain task in its own worktree
(`codex/enterprise-f3-fin001`). The coordinator owns the shared docs, backlog
state, PROGRESS, pushes and the PR. This task adds the pure Go subpackage
`backend/internal/modules/expense/domain/payable` (09-architecture places FIN
inside module `expense`) and one shared oracle
`contracts/testdata/enterprise/payable/payable-v1.psv`. Payables, settlement
and payment records are panel/server-only and online-only
([expenses-finance](../modules/expenses-finance.md) §8), so no Kotlin mirror is
built (EDR-139); the vectors are the single oracle for any future client. No
migration, SQL, route, OpenAPI, Android app/data module, DI, persistence, job,
producer, payment initiation or money movement is added. Synthetic data only.

Rules read: EDR-013; BR-EXP-001, BR-FIN-001, UC-FIN-01, §6 states (Payable
`OPEN`, `PARTIALLY_SETTLED`, `SETTLED`, `CANCELLED`) and the §10 example
([expenses-finance](../modules/expenses-finance.md)); BR-MONEY-001..010,
BR-TRUTH-001/003/004, BR-TEN-005/007, BR-REV-003/009 and BR-AUD-001
([shared rules](../04-business-rules.md)); REQ-11.1/11.2
([traceability](../17-traceability.md) §11); Payable/Payment/Advance in the
[domain model](../06-domain-model.md) §§3–5 and the Payable/Advance rows and
rules 4.2/4.5/4.7 of the [state model](../08-state-model.md); VAL-09, VAL-13,
VAL-14, VAL-19 and VAL-25 ([validation](../15-validation.md) §4); glossary
"Payable", "Payment", "Advance", "Reimbursement" and "Company amount". Task
blocks EMP-FIN-001..007 of
[EP10](../backlog/ep10-expenses-finance.md). Precedents: EMP-FUEL-001
(`21601ae8`), EMP-EXP-001 ([evidence](f3-exp001-expense-domain.md)) and the
server-only EMP-PLAT-016 ([evidence](f3-plat016-amendment-kernel.md)).

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-194..197, BUC-EMP-100..101, EDR-139..140. All are
used; none remains reserved.

Proposed contract section title: "Enterprise payables domain (EMP-FIN-001)".

Rules B-BR-EMP-163, B-BR-EMP-194, B-BR-EMP-195, B-BR-EMP-196, B-BR-EMP-197, BUC-EMP-100, BUC-EMP-101 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-FIN-001 — Payables domain: supplier, reimbursement and card statement obligations" (frozen before behaviour; text moved verbatim at integration).

Decision rows (EDR-139..140), recommended routine decisions taken unattended:

Decisions EDR-139, EDR-140 are recorded in [16-decisions](../16-decisions.md).

## Shape frozen before behaviour

Go package `payable` at `backend/internal/modules/expense/domain/payable`:

- Tokens: `Kind` (`Supplier`, `Reimbursement`, `CardStatement`, `ParseKind`)
  and `State` (`Open`, `PartiallySettled`, `Settled`, `Cancelled`,
  `ParseState`).
- References: `PayableRef`, `SettlementRef`, `StatementRef` (`NewPayableRef`,
  `NewSettlementRef`, `NewStatementRef`), scoped like FUEL-001/EXP-001.
  Counterparties reuse `fueling.EmployeeRef`, `fueling.SupplierRef` and
  `fueling.DocumentRef`; the payer reuses `fueling.Payer`.
- Recognition: `Source{Expense expense.Expense; State expense.ReviewState;
  Payer fueling.Payer; PaidBy *fueling.EmployeeRef}`, `Line{Source; Amount
  *string}`, `Input{Reference; Kind; Beneficiary; Supplier; Document;
  Statement; Lines}` and `New(scope, Input) (Payable, error)`.
- Payable (immutable value): `Reference`, `Kind`, `Beneficiary`, `Supplier`,
  `Document`, `Statement`, `Lines() []SourceLine` (`Expense`, `Key`,
  `AmountCentavos`, `CompanyCentavos`), `ObligationCentavos`,
  `SettledCentavos`, `RemainingCentavos`, `State`, `Settlements`,
  `CancelReason`, `Settle(scope, Settlement)`, `Cancel(scope, reason)` and
  `CheckState(asserted)`.
- Settlement: `NewSettlement(scope, SettlementRef, amount *string)` with
  `Reference` and `AmountCentavos`.
- Views: `ComputeViews(scope, period, basis, costs []expense.Entry, payables
  []Payable) (Views, error)` with `Cost() expense.CostTotals`, the per-state
  counts and `ObligationCentavos`, `SettledCentavos`, `RemainingCentavos`
  (`*big.Int`).
- Errors: `ErrInvalid`, `ErrScope`, `ErrNotEligible`, `ErrExceedsSource`,
  `ErrPartialReimbursement`, `ErrDuplicateSource`, `ErrDuplicateEntry`,
  `ErrExceedsRemaining`, `ErrCancelled`, `ErrHasSettlement`,
  `ErrStateMismatch` and `ErrSecondCost` (wraps
  `expense.ErrNotCostOrigin`). Private values are redacted in fmt, slog and
  JSON like FUEL-001.

Reuse: `expense.Expense`/`ExpenseRef`/`OriginKey`/`ReviewState`/`Totals`,
`fueling.Payer`/`EmployeeRef`/`SupplierRef`/`DocumentRef`,
`orgvalue.ParseCentavos` and `orgvalue.MaxAllocationLines`,
`metrics.MaxTotalCentavos` and `review.MaxReasonScalars`. None of these
packages is modified.

## RED

The exported API first compiled as stubs (zero values and an unexported
not-implemented error) with the frozen oracle and all tests. Final RED, rerun
against the saved stubs with the committed tests, `go test -race -count=1
-json ./internal/modules/expense/domain/payable/`, log
`/data/dev/worktrees/f3-logs/fin001-go-red.jsonl`: 17 top-level tests, 15
failing; 275 shared-vector subtests, 270 failing; no compile error, zero
skips; package 0.066 s. The passing parts never reach payable behaviour: the
two top-level tests check the oracle reader and the reused EMP-EXP-001
refusal of obligation tokens as cost origins, and the five subtests
(`ledger-{val13-reimbursement,payment,card-statement,payable,advance}-as-cost-origin`)
are refused by the accepted expense parser while the fixture builds a cost
item. They are kept as regression of the reused rule inside the single-cost
proof. The first RED run (log
`/data/dev/worktrees/f3-logs/fin001-go-red-initial.jsonl`, same counts) used
the test files before the REFACTOR input fix below.

## GREEN and REFACTOR

- GREEN, log `/data/dev/worktrees/f3-logs/fin001-go-green.jsonl`, coverage
  profile `/data/dev/worktrees/f3-logs/fin001-go-coverage.out`: 17/17
  top-level and 275/275 subtests pass under `-race` with atomic coverage,
  zero failures and skips, package 1.757 s. Statement coverage of
  `expense/domain/payable`: **100.0%**.
- REFACTOR (tests only, rerun green): the random settlement sequence drew
  `remaining + 1` = 1000000000001 centavos on the ceiling round, which the
  settlement constructor correctly refuses as INVALID (financial-v1); the
  draw is now capped at the ceiling, so overpayment is still exercised on
  every other round and after partial settlements. No expectation, vector or
  production rule changed; gofmt aligned one struct.
- Untouched dependencies, log `/data/dev/worktrees/f3-logs/fin001-go-affected.jsonl`
  (`-race`, zero skips): expense/domain 15/222, review/domain 9/232,
  fueling/domain 14/236, orgvalue/domain 5/112 and expense/domain/payable
  17/275 pass. `go build ./...` passes (build temp on the data volume; the
  root volume was full). Database adapters were not run (no Docker/PostGIS
  in this task).

The shared oracle has 275 cases (ENUM 27, SETTLEMENT 25, PAYABLE 183, LEDGER
40), SHA256 `5ac0b07637b66631dba97e441a66c750af9441c867461da8899dcec132904282`.
The consumer asserts the hash, counts, unique IDs, allowed argument names and
that every operation and each of the 14 error codes has a vector; a grammar
test proves the reader refuses malformed fields, hex, namespaces and foreign
or ambiguous errors. For every successful LEDGER case it recomputes the
EMP-EXP-001 totals of the costs alone and requires the cost view to equal
them. Unit and invariant tests add: the 1000-line bound and the 10^12 total
ceiling, immutability (no aliasing of lines, refs or settlements; refusals
return the zero value), verbatim reasons, zero values and scope order,
`ErrSecondCost` raised by the payable domain itself, redaction in
fmt/slog/JSON, 300 random settlement/cancel sequences against an independent
state derivation, 60 random selections of every kind proving the cost view
never changes and the settlement view equals the payables' facts, widened
sums over 10000 payables at the ceiling, concurrent reads and the payer rule.

Named negatives map to: payable recognized as a second expense
(`ledger-*-as-cost-origin`, `ledger-{payable,settlement,statement}-as-direct-cost`,
`ledger-payable-as-fueling-source`, `ledger-settlement-as-work-order-source`,
`ledger-excluded-cost-still-not-payable`,
`ledger-cancelled-payable-identity-still-not-cost`, `payable-source-*`,
`payable-card-source-*`, `payable-settlement-is-*`,
`ledger-val13-second-expense-same-fueling`); ambiguous state
(`payable-assert-*`: nine STATE_MISMATCH, four unknown tokens); invalid
negative/overflow/non-canonical amount (`payable-amount-invalid-*`,
`payable-card-total-above-ceiling`, `settlement-invalid-*`,
`payable-settlement-{zero,negative,decimal}`); obligation exceeding its
source (`payable-*-above-company*`, `payable-reimbursement-paid-above-company`,
`payable-reimbursement-zero-company`) and partial reimbursement
(`payable-reimbursement-*-below*`, `payable-reimbursement-one-centavo-of-many`);
reimbursement of a non-approved or company-paid expense
(`payable-reimbursement-not-approved-*` for all seven states,
`payable-reimbursement-company-paid`, `payable-reimbursement-wrong-employee`);
settlement exceeding remaining (`payable-overpayment*`,
`payable-settle-a-settled-payable`,
`payable-supplier-settle-above-line-below-source`); settlement on a CANCELLED
payable (`payable-settle-cancelled`); cancellation after a settlement
(`payable-cancel-after-{partial,settled}`, `ledger-cancel-after-settlement`);
mixed organization sources (`payable-{card,supplier}-mixed-organizations*`,
`payable-foreign-*`, `ledger-foreign-payable`); duplicate source in one
obligation (`payable-card-same-expense-twice`,
`payable-supplier-same-origin-key`, `payable-card-same-direct-expense-twice`)
and across live obligations (`ledger-two-live-*`). VAL-13's domain part is
`ledger-val13-*` (cost 97 350 with a settled 97 350 reimbursement, never
194 700; replayed payment DUPLICATE_ENTRY).

## Validation

All gates pass. Go commands from `backend` with
`PATH=/data/dev/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.4.linux-amd64/bin:$PATH`,
`GOCACHE=/data/dev/cache/f3-go-build` (and `GOTMPDIR` on the data volume for
the build):

```sh
gofmt -l ./internal/modules/expense                                   # empty
go vet ./internal/modules/expense/...                                 # clean
go test -race -count=1 -json -covermode=atomic -coverprofile=/data/dev/worktrees/f3-logs/fin001-go-coverage.out ./internal/modules/expense/domain/payable/ > /data/dev/worktrees/f3-logs/fin001-go-green.jsonl
go test -race -count=1 -json ./internal/modules/expense/... ./internal/modules/review/domain ./internal/modules/fueling/domain ./internal/modules/orgvalue/domain
go build ./...
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/expense/...   # no findings (fin001-go-staticcheck.log empty)
python3 docs/enterprise/tools/check_plan.py   # 348 tasks, 545 rules, 167 use cases, 145 screens, problems 0
git diff --cached --check                      # clean
git diff --cached | grep -nEi 'password|secret|token|BEGIN .*KEY|@gmail|cpf'
git diff --cached --binary | gitleaks stdin --redact --no-banner   # no leaks found
```

The keyword hits are benign: the word "token" in enum vocabulary prose and
case IDs, and a redaction test whose probe strings and loop variable are
named "secret". All identifiers are synthetic `9e000000-…` UUIDs; no names,
CNPJ, CPF or other personal data. Source manifest
`/data/dev/worktrees/f3-logs/fin001-source.sha256` (five Go files, README and
oracle). No Kotlin gate: no client mirror is built (EDR-139). No dependency,
migration, SQL, route, OpenAPI, job, producer, app or panel change; the
expense, review, fueling and orgvalue packages are imported unchanged.

## Retained obligations

- EMP-FIN-002: the audited reimbursement command and API, resolving the
  payer and paying employee from the approved source, the persisted
  duplicate-obligation guard, self-approval at review, retry and V-SQL/V-ISO
  proof.
- EMP-FIN-003: advances (REQUESTED..ACCOUNTED), their settlement against
  expenses and outstanding balances; ADVANCE is not a payable kind here.
- EMP-FIN-004: durable payment records, one settlement effect per authorized
  operation (lost response, concurrent retry, expected-state CONFLICT),
  payment dates, reversal or correction of a recorded payment, rebinding an
  obligation when its source is amended or voided, and the database
  single-cost invariant (VAL-13 at V-SQL/V-IDEM in EMP-REL-004).
- EMP-FIN-005: statements grouping payables (several payables per statement,
  supplier/card consistency, discrepancies). EMP-FIN-006: competence and cash
  views. EMP-FIN-007: the panel (SCR-FIN-01).
- Review package: an exported reason validator would let payables reuse the
  B-BR-EMP-163 rule itself rather than its bound (proposal 7 below).
- No persistence, API, app, panel, M3 or F3 acceptance is claimed; no real
  payment, provider integration or money movement exists or is authorized.

## Proposed shared-doc changes

The coordinator copies the CONTRACT block (into
`docs/product/ABASTEVO_EMPRESAS_CONTRACT.md`, as a section titled "Enterprise
payables domain (EMP-FIN-001)" after the EMP-EXP-001 section) and the EDR
block (into `docs/enterprise/16-decisions.md` after the last row)
mechanically. Further exact proposals:

1. `docs/enterprise/backlog/ep10-expenses-finance.md`, EMP-FIN-001 Meta:
   `state TODO` becomes `state LOCAL_DONE`, plus the BACKLOG block below the
   task.
2. `docs/enterprise/09-architecture.md` module table row for `expense`:
   ``| `expense` (pure expense and payable domains LOCAL_DONE; runtime pending) | EXP, FIN | One cost per origin, exact paid/company amounts, competence reporting day and review-basis totals; supplier, reimbursement and card statement payables with derived settlement state and separate cost and settlement views (subpackage `domain/payable`); payments, advances, allocations and persistence remain their owning tasks | `fueling` refs, accepted time/value kernels, `review` reason bound; future `document` |``
3. `scripts/check-manifest.txt` `[quick-packages]`: add
   `backend/internal/modules/expense/domain/payable` after
   `backend/internal/modules/expense/domain`.
4. `contracts/testdata/enterprise/README.md`: append

   ````markdown
   ## Payables vectors v1

   EMP-FIN-001 executes all 275 rows of
   [payable/payable-v1.psv](payable/payable-v1.psv) in Go (ENUM 27,
   SETTLEMENT 25, PAYABLE 183, LEDGER 40). The oracle pins supplier,
   reimbursement and card statement obligations from approved expenses, exact
   line and settlement amounts, the derived settlement state, cancellation and
   separate cost and settlement views in which obligations and payments never
   become a cost. It is server-side only: no client mirror exists; any future
   client must consume the same file. See [payable/README.md](payable/README.md).

   ```sh
   (cd backend && go test -race ./internal/modules/expense/domain/payable -count=1)
   ```
   ````
5. `docs/enterprise/modules/expenses-finance.md` §6, after "Payables are open,
   partially settled, settled or cancelled.": add "The payable state is
   derived from the exact obligation and its settlement entries; only an
   explicit, reasoned cancellation of an open payable sets cancelled
   (B-BR-EMP-196)."
6. `docs/enterprise/08-state-model.md` §3 Payable row: no change (states
   unchanged); optionally mark it derived like Asset availability by adding
   "(derived except `CANCELLED`)" after `L`.
7. `backend/internal/modules/review/domain` (future task, not this one):
   export the B-BR-EMP-163 reason check (for example `ValidReason(string)
   bool`) so payables, amendments and other owners reuse the rule itself; the
   payable package currently reuses `review.MaxReasonScalars` and restates the
   same control-free, nonblank check.
