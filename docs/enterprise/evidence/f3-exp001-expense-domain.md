# EMP-EXP-001 — Pure expense domain: origin, category, competence and company amount

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F3 pure-domain task in its own worktree
(`codex/enterprise-f3-exp001`). The coordinator owns the shared docs, backlog
state, PROGRESS, pushes and the PR. This task adds a pure Go package
`backend/internal/modules/expense/domain`, its Kotlin mirror
`com.anpfuel.domain.enterprise.expense`, and one shared oracle
`contracts/testdata/enterprise/expense/expense-v1.psv`. No migration, SQL,
route, OpenAPI, Android app/data module, DI, persistence, job or producer is
added. Synthetic data only.

Rules read: EDR-013; BR-EXP-001, BR-EXP-002, BR-FIN-001 and UC-EXP-01
([expenses-finance](../modules/expenses-finance.md)); BR-MONEY-001..011,
BR-TIME-004, BR-TRUTH-003/004, BR-TEN-001/007, BR-REV-006/009 and BR-AMD-003
([shared rules](../04-business-rules.md)); REQ-11.1/11.2
([traceability](../17-traceability.md) §11); the Expense entity and the
one-cost-per-fueling constraint ([domain model](../06-domain-model.md) §§3, 5);
Expense = BR-REV states plus `VOIDED` ([state model](../08-state-model.md) §3);
VAL-09/13/14/19/25 ([validation](../15-validation.md) §4); glossary
"Company amount". Precedent: EMP-FUEL-001 (commit 21601ae8).

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-180..184, BUC-EMP-94..95, EDR-131..133. All are
used; none remains reserved.

Rules B-BR-EMP-180, B-BR-EMP-181, B-BR-EMP-182, B-BR-EMP-183, B-BR-EMP-184, BUC-EMP-94, BUC-EMP-95 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-EXP-001 — Expense domain: origin, competence and company amount" (frozen before behaviour; text moved verbatim at integration).

Decision rows (EDR-131..133), recommended routine decisions taken unattended:

Decisions EDR-131, EDR-132, EDR-133 are recorded in [16-decisions](../16-decisions.md).

## Shape frozen before behaviour

Go package `backend/internal/modules/expense/domain` (stable exported names for
EMP-FIN-001 and EMP-BUDG-001): `OriginKind` (`KindFueling`, `KindWorkOrder`,
`KindDirect`, `ParseOriginKind`), `ExpenseRef`/`SourceRef` (`NewExpenseRef`,
`NewSourceRef`), `Origin` (`NewOrigin`, `NewFuelingOrigin` taking the FUEL-001
`fueling.FuelingRef`), comparable `OriginKey`, `Amounts` (`NewAmounts` with
`*string` inputs, nil = absent), `Category` (`ParseCategory`), `ExpenseInput`,
`Expense` (`NewExpense`, `OriginKey`, `Reporting`), `ReviewState` (eight
constants, `ParseReviewState`, `Counting`), `Counting`, `Basis`
(`ParseBasis`), `ReportingPeriod` (`NewReportingPeriod`, `Contains`), `Entry`,
`CostTotals` and `Totals`. Errors: `ErrInvalid`, `ErrScope`,
`ErrNotCostOrigin`, `ErrCompanyExceedsPaid`, `ErrDuplicateCost`,
`ErrOutOfPeriod`. Private values are redacted in fmt/slog/JSON like FUEL-001.

Kotlin mirror `com.anpfuel.domain.enterprise.expense`: `ExpenseFailure` /
`ExpenseDomainException`, `OriginKind.parse`, `ExpenseRef`, `SourceRef`,
`ExpenseOrigin.create`/`ExpenseOrigin.fueling` (FUEL-001 `FuelingRef`),
`OriginKey`, `ExpenseAmounts.parse`, `ExpenseCategory.parse`, `ExpenseInput`,
`Expense.create(scope, input)`, `ReviewState`, `CostCounting`, `TotalsBasis`,
`ReportingPeriod.of`, `ExpenseEntry`, `CostTotals` and `ExpenseTotals.compute`.

Reuse: amounts call the accepted `orgvalue.ParseCentavos` /
`EnterpriseValues.parseCentavos` (bounds 0..1000000000000 from
`metrics.MaxTotalCentavos` / `PremiumBounds`); no new money ceiling. The
competence uses `orgtime.ValidateDeclaration` /
`EnterpriseTime.validateDeclaration` exactly as FUEL-001 does. The only kernel
change, following EDR-094, extracts the reporting-day branch of Capture into
`orgtime.AttributeReportingDay` / `EnterpriseTime.attributeReportingDay`
(returning `ReportingAttribution{Day, Zone, Source}`); Capture now calls it
with unchanged order and errors, so no dummy receipt is needed offline. The
fueling package is referenced only through `FuelingRef`; it is not re-modelled.
The parallel `review` package is not imported: the review state is a closed
input enum (binding retained below).

## RED

Both languages first compiled against exported stubs (Go zero values and
`errNotImplemented`, Kotlin `TODO()`), with the frozen oracle and tests.

- Go (`go test -race -count=1 -json`), log
  `/data/dev/worktrees/f3-logs/exp001-go-red.jsonl`: expense 15 top-level
  tests, 14 failing, and 222/222 shared-vector subtests failing (0.033 s); the
  one passing top-level test exercises only the fixture grammar reader.
  orgtime: the 2 new attribution tests fail, the 6 existing tests and 99
  subtests pass (0.018 s). Zero skips. A first RED draft let the fixture
  reader itself refuse a malformed organization, so `expense-malformed-scope`
  passed against the stub; the reader now hands the domain a zero scope and
  the rerun RED has all 222 vector cases failing. No expectation changed.
- Kotlin (`:domain:test`, flock), log
  `/data/dev/worktrees/f3-logs/exp001-kotlin-red.log`, XML in
  `/data/dev/worktrees/f3-logs/exp001-kotlin-red-xml/`: 237 tests, 236
  failures, 0 errors, 0 skipped (0.509 s test time, Gradle 38 s). Only the
  fixture-grammar method passes; 222 vectors, 12 unit methods and 2
  attribution methods fail.

## GREEN and REFACTOR

- Go, log `/data/dev/worktrees/f3-logs/exp001-go-green.jsonl` and coverage
  profile `/data/dev/worktrees/f3-logs/exp001-go-green-coverage.out`: expense
  15/15 top-level and 222/222 subtests pass (1.269 s, race, atomic coverage);
  orgtime 8/8 and 99/99 pass (1.021 s). Zero failures and skips. Statement
  coverage, per package without cross-package attribution: expense 100.0%,
  orgtime 100.0%.
- Affected Go regressions (time-kernel importers that run without a database),
  log `/data/dev/worktrees/f3-logs/exp001-go-affected.jsonl`: fueling/domain
  14/236, fleet/domain/meter 16/355, organization/domain 35/145,
  orgsync/protocol 24/423 and orgvalue/domain 5/112 pass under -race, zero
  skips. `go build ./...` passes; database adapters were not run (no
  Docker/PostGIS in this task).
- Kotlin targeted, log `/data/dev/worktrees/f3-logs/exp001-kotlin-green.log`:
  341 tests (expense vectors 223, expense unit 12, attribution 2, existing
  EnterpriseTimeTest 104) pass, 0 failures/errors/skips (Gradle 19 s).
- REFACTOR: the Kotlin `ExpenseOrigin.fueling` redundant scope check was
  removed (create already checks it first); a typed value-equality test was
  added (refs of different types or organizations, category, key, period).
  staticcheck ST1018 flagged a literal zero-width character in a Go test
  string; both language tests now use the `\u200b` escape (same input).
- Kotlin final whole enterprise suite (`--tests
  'com.anpfuel.domain.enterprise.*'`), log
  `/data/dev/worktrees/f3-logs/exp001-kotlin-enterprise-final.log`, XML in
  `/data/dev/worktrees/f3-logs/exp001-kotlin-final-xml/`: 1589 tests, 0
  failures, 0 errors, 0 skipped (Gradle 33 s), including expense 236 (223
  vectors + 13 unit), attribution 2, time 104 and fueling 201. JaCoCo
  (`/data/dev/worktrees/f3-logs/exp001-kotlin-jacoco.xml`) for package
  `com.anpfuel.domain.enterprise.expense`: lines 140/141 (99.29%),
  instructions 1364/1411 (96.67%), branches 98/101 (97.03%);
  `attributeReportingDay` 77/77 instructions.

The shared oracle has 222 cases (ENUM 40, AMOUNTS 36, CATEGORY 24, PERIOD 17,
EXPENSE 57, TOTALS 48), SHA256
`b411e1cfa55c0aca4e4c9cadfab2f286ec22c3215e3103c119a767e013885dfe`. Both
consumers assert the hash, counts, unique IDs, allowed argument names and that
every operation and each of the six error codes has at least one vector. The
task's named negatives map to: missing origin (`expense-missing-origin`),
settlement as cost (`enum-origin-refused-*`, `expense-settlement-origin-*`,
`totals-val13-*-not-second-cost`), duplicate key (`totals-duplicate-*`,
`totals-val13-second-expense-same-fueling`), rounding away the paid amount
(`amounts-invalid-decimal-*`, `amounts-invalid-half-centavo`), false zero
(`amounts-invalid-absent-*`, `amounts-invalid-empty-*`,
`amounts-known-zero-*`), totals overflow (`amounts-invalid-int64-*`,
`totals-exact-above-record-ceiling`, Go/Kotlin 5000 x 10^12 unit tests),
mixed organization (`totals-mixed-organization`, `totals-foreign-scope`) and
pending counted as approved (`totals-val25-*`, `totals-all-states-*`).

## Validation

All gates pass. Commands (Go from `backend`, toolchain go1.27.4,
`GOCACHE=/data/dev/cache/f3-go-build`; Gradle from the worktree root with
JDK 17 under `flock /data/dev/worktrees/.f3-gradle.lock`):

```sh
gofmt -l ./internal/modules/expense ./internal/modules/orgtime        # empty
go vet ./internal/modules/expense/... ./internal/modules/orgtime/... ./internal/modules/fueling/... ./internal/modules/fleet/... ./internal/modules/organization/... ./internal/modules/orgsync/... ./internal/modules/membership/...
go build ./...
go test -race -count=1 -json -covermode=atomic -coverprofile=/data/dev/worktrees/f3-logs/exp001-go-green-coverage.out -coverpkg=./internal/modules/expense/domain,./internal/modules/orgtime/domain ./internal/modules/expense/domain ./internal/modules/orgtime/domain
go test -race -count=1 -cover ./internal/modules/expense/domain ./internal/modules/orgtime/domain
go test -race -count=1 -json ./internal/modules/fueling/domain ./internal/modules/fleet/domain/meter ./internal/modules/organization/domain ./internal/modules/orgsync/protocol ./internal/modules/orgvalue/domain
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/expense/... ./internal/modules/orgtime/...   # clean
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.*' --offline --no-daemon --no-parallel --max-workers=2 --console=plain -PcomapsSdkDir=<worktree>/build/comaps/7ad828883c4aca8a
python3 docs/enterprise/tools/check_plan.py   # 348 tasks, 545 rules, 167 use cases, 145 screens, problems 0
git diff --cached --check                      # clean
git diff --cached | grep -nEi 'password|secret|token|BEGIN .*KEY|@gmail|cpf'
git diff --cached --binary | gitleaks stdin --redact --no-banner   # no leaks found
```

The domain module has no ktlint/detekt task (only the Kotlin JVM and JaCoCo
plugins in `domain/build.gradle.kts`); Kotlin compiles with zero warnings.
The grep hits are benign: the word `token` in docs about enum tokens and a
test variable named `secret` that asserts redaction. Data is synthetic.

## Retained obligations

- EMP-EXP-002: persistence with unique (organization, origin kind, source) and
  the DIRECT primary key, the fueling-to-expense projection (copying the
  fueling paid total exactly and choosing the configured fuel category), sync
  intake handlers, received_at and server-resolved reporting zones, and real
  PostGIS/concurrency proof of VAL-13.
- EMP-APPR-001 / review package: bind `ReviewState` to the canonical review
  vocabulary once that package integrates (same eight names), revisions,
  partial approval of the company amount and self-approval checks.
- EMP-EXP-003: configured category catalogue, retirement and origin-to-category
  consistency. EMP-EXP-006: allocation across cost centres and assets.
- EMP-FIN-001 / EMP-BUDG-001: payables, payments, reimbursements, advances and
  budget consumption build on these types; settlement never creates a cost.
- MAINT (F6): a typed work-order reference may replace the generic SourceRef
  for WORK_ORDER without changing the key.
- No route, SQL, migration, OpenAPI, app/panel screen, DI, job or producer is
  added; no persistence, API, app, M3 or F3 acceptance is claimed.

## Proposed shared-doc changes

The coordinator copies the CONTRACT block (into
`docs/product/ABASTEVO_EMPRESAS_CONTRACT.md`, after the EMP-FUEL-001 section)
and the EDR block (into `docs/enterprise/16-decisions.md` after the last row)
mechanically. Further exact proposals:

1. `docs/enterprise/backlog/ep10-expenses-finance.md`, EMP-EXP-001 Meta:
   `state TODO` becomes `state LOCAL_DONE`, plus the BACKLOG block below the
   task.
2. `docs/enterprise/09-architecture.md` module table row for `expense`:
   ``| `expense` (pure expense domain LOCAL_DONE; runtime pending) | EXP, FIN | One cost per origin, exact paid/company amounts, competence reporting day and review-basis totals; payables, payments, advances, allocations and persistence remain their owning tasks | `fueling` refs, accepted time/value kernels; future `review`, `document` |``
3. `scripts/check-manifest.txt` `[quick-packages]`: add
   `backend/internal/modules/expense/domain` after
   `backend/internal/modules/fueling/domain`.
4. `contracts/testdata/enterprise/README.md`: append

   ````markdown
   ## Expense vectors v1

   EMP-EXP-001 executes all 222 rows of
   [expense/expense-v1.psv](expense/expense-v1.psv) in Go and Kotlin (ENUM 40,
   AMOUNTS 36, CATEGORY 24, PERIOD 17, EXPENSE 57, TOTALS 48). The oracle pins
   cost-origin kinds and refused settlement kinds, exact paid/company amounts
   without a false zero, the category key profile, competence reporting days
   and approved-versus-pending totals. See [expense/README.md](expense/README.md).
   No persistence, review, settlement or runtime acceptance is claimed.

   ```sh
   (cd backend && go test -race ./internal/modules/expense/domain -count=1)
   ```
   ````
