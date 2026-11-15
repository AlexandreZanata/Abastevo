# EMP-BUDG-001 — Pure budget domain: lines, periods, dimensions and capacity

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F3 pure-domain task in its own worktree
(`codex/enterprise-f3-budg001`). The coordinator owns the shared docs, backlog
state, PROGRESS, pushes and the PR. This task adds the pure Go package
`backend/internal/modules/budget/domain` and one shared oracle
`contracts/testdata/enterprise/budget/budget-v1.psv`. Budgets, holds and
capacity are panel/server decisions (reservations needing concurrent limits
are online-only, expenses-finance §8), so no Kotlin mirror is built (EDR-141);
the vectors are the single oracle for any future client. No migration, SQL,
route, OpenAPI, Android app/data module, DI, persistence, job or producer is
added. Synthetic data only.

Rules read: BR-BUDG-001, BR-EXP-002, UC-BUDG-01, §6 hold states, §8 online-only
reservations and the §10 example ([expenses-finance](../modules/expenses-finance.md));
BR-MONEY-001/010, BR-TIME-004, BR-REV-009, BR-TRUTH-003/004 and BR-TEN-007
([shared rules](../04-business-rules.md)); BR-AUTH-001/004/005 and the
`budg.budget.read`/`budg.budget.manage` keys ([permissions](../07-permissions.md));
the Budget/Budget line/Budget hold entities and the "Budgets" constraint
(row lock, `limit ≥ 0`, available computed and never stored as truth,
[domain model](../06-domain-model.md) §§3, 5); Budget hold
`ACTIVE`/`CONSUMED`/`RELEASED`/`EXPIRED` ([state model](../08-state-model.md));
REQ-11.3 ([traceability](../17-traceability.md) §11); VAL-14 and the fixture
budget line ([validation](../15-validation.md) §§3–4); the EMP-BUDG-002..006
task blocks so the API serves them. Reused, not modified: EMP-EXP-001
`expense/domain` (Expense, Entry, ReviewState/Counting, Category,
ReportingPeriod, ExpenseRef, OriginKey), `orgtime/domain`
(`AttributeReportingDay`) and `orgvalue/domain` (`ParseCentavos`).
Precedent: EMP-FUEL-001 (vectors), EMP-PLAT-016 (Go-only oracle, EDR-137).

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-198..201, BUC-EMP-102..103, EDR-141..142. All are
used; none remains reserved.

Proposed contract section title: "Enterprise pure budget (EMP-BUDG-001)".

Rules B-BR-EMP-198, B-BR-EMP-199, B-BR-EMP-200, B-BR-EMP-201, BUC-EMP-102, BUC-EMP-103 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-BUDG-001 — Budget domain: lines, periods, dimensions and capacity" (frozen before behaviour; text moved verbatim at integration).

Decision rows (EDR-141..142), recommended routine decisions taken unattended:

Decisions EDR-141, EDR-142 are recorded in [16-decisions](../16-decisions.md).

## Shape frozen before behaviour

Go package `backend/internal/modules/budget/domain` (stable exported names for
EMP-BUDG-002..006):

- Errors: `ErrInvalid`, `ErrScope`, `ErrOutOfPeriod`, `ErrDimension`
  (`DIMENSION_MISMATCH`), `ErrDuplicate`, `ErrHoldNotActive`,
  `ErrNotApproved`, `ErrStaleAvailable`.
- `DimensionKind` (`DimensionOrganization`, `DimensionUnit`,
  `DimensionCostCentre`, `DimensionAsset`, `DimensionEmployee`,
  `ParseDimensionKind`); `LineRef`/`HoldRef` (`NewLineRef`, `NewHoldRef`);
  `Target` (`NewTarget`, `Kind`); `Tags` (`NewTags`, `Get`); `Dimension`
  (`NewDimension(scope, kind, *Target, filter *string)`, `Kind`, `Target`,
  `Filter`); `LineInput`/`Line` (`NewLine`, `Reference`, `Period`,
  `Dimension`, `LimitCentavos`, `Covers`).
- `Consumption{Entry expense.Entry; Tags Tags}`.
- `HoldState` (`HoldActive`, `HoldConsumed`, `HoldReleased`, `HoldExpired`,
  `ParseHoldState`); `HoldInput`/`Hold` (`NewHold` restores a stored hold;
  `Release`, `Expire`; getters including `Reporting` and `ConsumedBy`).
- `Capacity` (`ComputeCapacity(scope, line, consumption, holds)`,
  `LimitCentavos`, `ConsumedCentavos`, `PendingCentavos`, `HeldCentavos`,
  `AvailableCentavos`, `OverCommitted`, counts, `Reconcile(claimed)`).
- `Outcome` (`OutcomeActivate`, `OutcomePost`, `OutcomeExceeds`);
  `HoldRequest` + `Reserve(scope, capacity, request) (Reservation, error)`;
  `Post(scope, capacity, consumption, *HoldRef) (Posting, error)`.

Raw token and amount inputs (dimension kind, hold state, limit, amount,
category filter, period days) are validated inside the package so every
refusal is one of the eight budget codes. Private values are redacted in
fmt/slog/JSON like EMP-EXP-001.

## Oracle frozen before behaviour

`contracts/testdata/enterprise/budget/budget-v1.psv`, 261 cases (ENUM 24,
LINE 44, CAPACITY 94, RESERVE 40, POST 35, SEQUENCE 24), SHA256
`fb9b99c33a0e82a131d2ae375276bd4b49380cfdb30ed479f089080fbaf03985`, with its
[README](../../../contracts/testdata/enterprise/budget/README.md) fixing the
grammar, defaults, check order and outputs. Expected values are hand-written
literals; the consumer pins the hash, the per-operation counts, unique IDs,
allowed case and list-entry keys, and asserts that each of the eight error
codes, each outcome (ACTIVATE, EXCEEDS, POST, RELEASED, EXPIRED), each
dimension kind and the hold states have a vector. The task's named negatives
map to: wrong period (`capacity-expense-before-start`, `-end-excluded`,
`capacity-hold-end-excluded`, `capacity-unit-zone-next-day`,
`reserve-end-excluded`, `post-out-of-period`), wrong dimension
(`capacity-*-wrong-target`, `capacity-cost-centre-absent-tag`,
`capacity-asset-same-uuid-other-kind`, `capacity-*-filter-*`,
`capacity-hold-other-line`, `reserve-other-line`, `post-wrong-dimension`),
mixed scope/foreign organization (`capacity-mixed-organizations`,
`capacity-foreign-*`, `line-foreign-*`, `reserve-foreign-*`,
`post-foreign-*`), overflow (`line-limit-invalid-above-ceiling`,
`-int64-overflow`, `reserve-amount-invalid-*`,
`capacity-exact-above-record-ceiling`), a stale independent available
(`capacity-claimed-stale*`, `capacity-claimed-limit-as-available`,
`capacity-claimed-sign-dropped`), unsupported defaults
(`line-limit-invalid-absent`, `-empty`, `-unlimited`, `-infinite`,
`capacity-line-without-limit`, `reserve-without-limit`,
`post-without-limit`, `line-limit-known-zero`, `reserve-known-zero-limit`),
double release or expiry (`sequence-double-*`, `sequence-release-after-expiry`,
`sequence-expire-after-release`, `sequence-*-consumed`), consuming a released
hold (`sequence-consume-released`, `-expired`, `-twice`,
`post-released-hold`, `post-expired-hold`, `post-consumed-hold`), a hold
that does not fit (`reserve-val14-second-hold-exceeds`,
`reserve-one-centavo-over`, `reserve-overcommitted-line`,
`sequence-val14-two-holds-serialize`) and a revived hold
(`reserve-late-retry-cannot-revive`, `sequence-revive-released`).

## RED

The exported API first compiled as stubs (zero values and an unexported
not-implemented error) with the frozen oracle and both test files.
`go test -race -count=1 -json ./internal/modules/budget/...`, log
`/data/dev/worktrees/f3-logs/budg001-go-red.jsonl`: 13 top-level tests, 11
failing; 261/261 shared-vector subtests failing on assertion mismatches
(`UNEXPECTED` against the expected maps); zero skips; package 0.038 s. The two
passing top-level tests check only the fixture grammar reader and the
structural absence of derived-number input fields, not domain behaviour. No
compile error.

## GREEN and REFACTOR

After implementation, log `/data/dev/worktrees/f3-logs/budg001-go-green.jsonl`
and atomic coverage profile `/data/dev/worktrees/f3-logs/budg001-go-cover.out`:
14/14 top-level tests and 261/261 vector subtests pass under `-race`, zero
failures and skips, package 2.3 s. Statement coverage of
`budget/domain`: **100.0%**. No expectation changed between RED and GREEN.

Unit tests prove: closed vocabularies and refused casts; zero values never
decide (no line, capacity, hold, reference or limit yields a decision, and a
zero reservation or posting carries no outcome or hold); tags hold one target
per kind and refuse foreign targets; `Covers` routes one expense to every
matching line (organization, unit, cost centre, filtered asset) and refuses
employee, other cost centre, same UUID under another kind and other filter;
exact sums with 10 000 expenses and 10 000 holds at the record ceiling
(available 10^12 − 2·10^16, EXCEEDS with the exact shortfall, claims beyond
int64 compared exactly); a 1 500-step seeded serialized replay where no
decision over-commits the line, every reported AvailableAfter equals the
recomputation from the new facts, release/expiry restore exactly the hold
amount and a hold leaves ACTIVE at most once; concurrent readers of one
capacity under `-race`; immutable hold values with UTC normalization and
unit-zone attribution; copies from every big-integer getter; redaction in
fmt, slog and JSON.

REFACTOR (re-run green): the fixture reader's request helper takes
`*testing.T` first; coverage showed three constructor branches the oracle
cannot reach through the fixture (a dimension of another organization passed
to `NewLine`, an unconstructed or foreign hold line, an unconstructed
consuming expense), so `TestConstructorsRefuseForeignAndUnconstructedParts`
was added for them (it passed on first run; it is not part of RED).

## Validation

All gates pass. Commands (from `backend/` unless noted; toolchain go1.27.4,
`GOCACHE=/data/dev/cache/f3-go-build`; staticcheck under go1.27.1):

```sh
gofmt -l ./internal/modules/budget                                    # empty
go vet ./internal/modules/budget/...                                  # clean
go build ./...                                                        # ok
go test -race -count=1 -json -covermode=atomic -coverprofile=/data/dev/worktrees/f3-logs/budg001-go-cover.out ./internal/modules/budget/... > /data/dev/worktrees/f3-logs/budg001-go-green.jsonl
go test -race -count=1 ./internal/modules/expense/domain ./internal/modules/orgtime/domain ./internal/modules/orgvalue/domain   # ok (reused, unmodified)
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/budget/...   # no findings, log budg001-staticcheck.log
python3 docs/enterprise/tools/check_plan.py   # (root) 348 tasks, 545 rules, 167 use cases, 145 screens, problems 0
git diff --cached --check                     # clean
git diff --cached | grep -nEi 'password|secret|token|BEGIN .*KEY|@gmail|cpf'
git diff --cached --binary | gitleaks stdin --redact --no-banner   # no leaks found
```

The keyword scan matches only the word "token" in this evidence's prose
(dimension-kind tokens, raw token inputs) and the scan command itself; there
is no credential, key or personal data. All identifiers are synthetic `9e000000-…` UUIDs. No Kotlin
gate: no client mirror is required (EDR-141). No dependency, migration, SQL,
route, OpenAPI, job, producer, app or panel change; `expense`, `orgtime` and
`orgvalue` are imported unchanged.

## Retained obligations

- EMP-BUDG-002: persisted budgets and lines (`limit ≥ 0`, overlap exclusion
  per dimension and period, versions), consumption posting at approval in the
  same transaction under `SELECT … FOR UPDATE` of every matching line (in a
  fixed lock order), the real PostGIS race proof of VAL-14 and idempotent
  retries mapped from `DUPLICATE`, with the stored tags taken from the
  record's creation-time unit, cost centre, asset and employee.
- EMP-BUDG-003: reservation commands, owner/actor authorization
  (`budg.budget.manage`), bounded expiry instants and the expiry job, release
  by the right owner and the binding of a hold to its consuming outcome.
- EMP-BUDG-004: overrun policies and the exception workflow for EXCEEDS.
- EMP-BUDG-005: panel setup and monitoring (server basis, held, consumed,
  pending and available shown; no editable balance).
- EMP-BUDG-006: driver and asset limits at submission and after sync, using
  these lines and decisions with recorded history; no client-side authority.
- EMP-EXP-006: allocation parts across cost centres/assets as consumption
  facts (budget-v2 would consume parts instead of whole company amounts).
- No route, SQL, migration, OpenAPI, app/panel screen, DI, job or producer is
  added; no persistence, API, app, M3 or F3 acceptance is claimed.

## Proposed shared-doc changes

The coordinator copies the CONTRACT block (into
`docs/product/ABASTEVO_EMPRESAS_CONTRACT.md`, after the EMP-EXP-001 section,
under the heading "## EMP-BUDG-001 — Budget domain: lines, periods,
dimensions and capacity (2026-11-14)" with the evidence link, then
"## Enterprise pure budget (EMP-BUDG-001)") and the EDR block (into
`docs/enterprise/16-decisions.md` after the last row) mechanically. Further
exact proposals:

1. `docs/enterprise/backlog/ep10-expenses-finance.md`, EMP-BUDG-001 Meta:
   `state TODO` becomes `state LOCAL_DONE`, plus the BACKLOG block below the
   task.
2. `docs/enterprise/09-architecture.md` module table row for `budget`:
   ``| `budget` (pure budget domain LOCAL_DONE; runtime pending) | BUDG | One-dimension lines over half-open reporting periods with exact limits, capacity derived from approved consumption and active holds, reservation/posting decisions and the hold lifecycle; persistence, locks, reservation API, overrun policy, panel and driver/asset limits remain their owning tasks | `expense`, accepted time/value kernels |``
3. `scripts/check-manifest.txt` `[quick-packages]`: add
   `backend/internal/modules/budget/domain` after
   `backend/internal/modules/expense/domain`.
4. `contracts/testdata/enterprise/README.md`: append

   ````markdown
   ## Budget vectors v1

   EMP-BUDG-001 executes all 261 rows of
   [budget/budget-v1.psv](budget/budget-v1.psv) in Go (ENUM 24, LINE 44,
   CAPACITY 94, RESERVE 40, POST 35, SEQUENCE 24). The oracle pins closed
   dimensions and hold states, required exact limits, exact dimension and
   period matching, capacity derived from approved consumption and active
   holds, claimed-available comparison, reservation and posting decisions and
   serialized hold sequences including VAL-14. See
   [budget/README.md](budget/README.md). It is server-side only: no client
   mirror exists; any future client must consume the same file. No
   persistence, concurrency proof, overrun policy or runtime acceptance is
   claimed.

   ```sh
   (cd backend && go test -race ./internal/modules/budget/domain -count=1)
   ```
   ````
