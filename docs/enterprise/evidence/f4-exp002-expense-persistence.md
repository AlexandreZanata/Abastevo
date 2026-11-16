# EMP-EXP-002 — Expense persistence, sync handlers and the fueling-to-expense projection

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F3 DB task (an F4 prerequisite of EMP-TRIP-020 through
EMP-EXP-004) on `codex/enterprise-f4-exp002` (worktree
`/data/dev/worktrees/f4-exp002`), based on the F4 head `4f9170fc`
(migrations through `000237`; `000238..000243` belong to other sessions),
then merged with the F4 head `0912b48a` (EMP-TRIP-014/018 `000235/000236`,
UGC safety `000239`, EMP-GPS-004/012 `000240`) before its final gates; the
usage sources are still the 000232/000231 bodies there.
Real dependencies used: the EMP-EXP-001 pure expense domain and its totals,
the EMP-FUEL-002 fueling store, handler and authority, the OFFL-002 signed
intake ledger, the METER-002 history classifier (`historyDisposition`), the
FLEET-005 allocation snapshot port, the FLEET-006 assigned-asset source and
the fleet asset lifecycle locks. Nothing is faked; synthetic data only. No
route, panel, Android app, job or deployment change.

Read: AGENTS, FAST_EXECUTION, 00-conventions; the F4 ledger and the F3 ledger
(EXP-002 was the next dependency-ready F3 task); the EMP-EXP-001..007 blocks
of [EP10](../backlog/ep10-expenses-finance.md); [expenses-finance](../modules/expenses-finance.md)
(BR-EXP-001/002, BR-FIN-001, UC-EXP-01, §§8–10); EDR-013 and the latest EDR
rows; BR-FUEL-010 ([fuelings](../modules/fuelings.md)); BR-OFFL-004/005
([10-offline-sync](../10-offline-sync.md)); BR-REV-009, BR-TEN-005, BR-TIME-004
([04-business-rules](../04-business-rules.md)); REQ-11.1/11.2
([17-traceability](../17-traceability.md) §11); the one-cost-per-fueling
constraint of [06-domain-model](../06-domain-model.md) §5; VAL-13
([15-validation](../15-validation.md)); the `exp.expense.*` rows of
[07-permissions](../07-permissions.md) and `authorization-catalog-v5.psv`; the
evidence of [EXP-001](f3-exp001-expense-domain.md),
[FUEL-002](f3-fuel002-fueling-persistence.md),
[APPR-001](f3-appr001-review-lifecycle.md),
[BUDG-001](f3-budg001-budget-domain.md), [FIN-001](f3-fin001-payables-domain.md)
and [APPR-006](f3-appr006-period-lock.md).

IDs used: B-BR-EMP-444..449, BUC-EMP-225..227, EDR-287..290 — all of them; no
pre-allocated ID is left unused. Migration `000244_organization_expenses.sql`.

## Rules frozen before behaviour

Observed before any change: no expense table, store, handler or projection
exists; the fueling owner declares `NO_ASYNC_FOLLOWUP` "until EXP-002 owns the
expense projection" (EDR-175); the org-event intent/receipt tables (000168)
and the intake event plans exist, but no worker registers the `org-event` job
kind and no production consumer is composed (`IntakeConsumers` is used only
by tests); the asset, unit and cost-centre usage sources fail closed on any
unlisted foreign key (latest bodies: 000232 asset, 000231 unit and cost
centre); `exp.expense.record` is an `S` cell for OWNER, ADMIN, FLEET_MANAGER,
SUPERVISOR, FINANCE, DRIVER and OPS_STAFF and absent for APPROVER and AUDITOR;
`organizations` and every enterprise table are forced-RLS, so an ordinary
migrator cannot enumerate organizations to backfill data.

Rules B-BR-EMP-444..449 and use cases BUC-EMP-225..227 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-EXP-002 — Expense persistence, sync handlers and the fueling-to-expense
projection (2026-11-15)"; decisions EDR-287 (projection inside the fueling
transaction, neither half alone, no migration backfill, idempotent catch-up),
EDR-288 (`org_expenses` store, v1 categories as one replaceable constraint,
usage sources), EDR-289 (`expense.submit` schema 1, authority, retained forms,
Kotlin builder and shared vectors) and EDR-290 (idempotent, never-overwriting
projection and how races and partial transactions are proven) in
[16-decisions](../16-decisions.md). They are the recommended routine
decisions taken unattended.

Decision on the partial transaction (task question): **neither half may
exist alone.** A fueling inserted after 000244 cannot commit without its
`FUELING` expense (deferred constraint trigger `org_fueling_expense_complete`)
and an expense cannot reference a fueling that is not stored (composite
foreign key plus the source trigger). The only fuelings without a cost are
the ones accepted before 000244, which the catch-up projects.

## Shape

- Migration `000244_organization_expenses.sql`: forced-RLS, insert-only
  `org_expenses` (origin kinds `FUELING`/`DIRECT`; origin key unique on
  (organization, kind, source); unique (organization, fueling); unique
  creating operation; exact centavos with company ≤ paid; v1 category
  constraint; composite same-organization keys to the sync operation,
  fueling, asset, author membership and employee, enrollment, unit and cost
  centre); `org_expense_source()` (BEFORE INSERT: organization root lock, the
  FUELING branch mirrors the stored fueling exactly, the DIRECT branch proves
  the `expense.submit` operation, the enrolled author, a non-draft asset and
  the FLEET-005 snapshot at the instant; both re-prove the reporting
  day/zone/source); deferred `org_expense_complete()` (the ACCEPTED receipt of
  the creating operation) and `org_fueling_expense_complete()` (on
  `org_fuelings`: the one FUELING expense); 55000 guard on update, delete and
  truncate; the three usage sources redefined from their 000232/000231 bodies
  with exactly one added allow-list line each (associations, not counted).
- `db/queries/expense` (new sqlc stanza): author employee, reference facts,
  reporting zones, fueling source, pending fueling costs, insert (`ON
  CONFLICT DO NOTHING`, rows affected), read, id by origin.
- `expense/application`: `OperationType` `expense.submit`, `EntityType`
  `expense`, `FuelCategory`, `SourceRevision`, `Submitted`,
  `DirectCategories()`, errors (`ErrMalformed`, `ErrUnsupported`,
  `ErrUnavailable`, `ErrNotFound`, `ErrStale`), `Policy`, `Disposition`,
  `Target`, ports `Authority`, `Lifecycle`, `Store`; `Command`/`CommandInput`,
  `Record` (with `Expense(scope)` re-proving a stored row as the EXP-001
  value), `Prepared`, `FuelingSource`, `ProjectFueling`, `DecideProjection`
  (`Insert`/`Unchanged`), `ParseOperation`.
- `expense/adapters`: `Store` (`ProjectFuelingTx`,
  `ProjectPendingFuelingsTx`, `AuthorEmployeeTx`, `PrepareExpenseTx`,
  `InsertDirectTx`, `ReadExpenseTx`) and `NewIntakeHandler` (the
  `expense.submit` owner).
- `fueling/application.CostProjection` port; `fueling/adapters.NewIntakeHandler`
  takes it as a mandatory fourth port and projects right after the fueling
  insert.
- `organization/adapters.ExpenseAuthority` (`CheckExpenseTx`,
  `ClassifyExpenseTx`, `expenseHistoryRule`).
- `enterprisesuite`: one shared expense store; the fueling owner gets it as
  its projection; the `expense.submit` owner is registered with
  `NO_ASYNC_FOLLOWUP` for ACCEPTED and REJECTED.
- Kotlin `com.anpfuel.domain.enterprise.expense.ExpenseOperations`
  (`submit`, `read`, `DIRECT_CATEGORIES`), `ExpenseEnvelope`,
  `ExpenseSubmitFact`, `ExpenseOperationException`/`Failure`.
- Shared oracle `contracts/testdata/enterprise/expense/expense-submit-v1.psv`
  (40 rows: OK 7, MALFORMED 28, UNSUPPORTED 4, PROTOCOL 1; SHA-256
  `5c69de3c71bdeb9495ddef9ae9a39ae6597f0e791cdf1500abebdffc100de6e6`),
  generated by the independent script
  `f4-logs/exp002-scripts/gen_expense_submit_vectors.py`.
- OpenAPI: the `expense.submit` owner, `EnterpriseExpenseSubmitPayload` and
  description sentences for both owners (no path, status or route change).

Every exported function first compiled as a stub (`errNotImplemented`, zero
values, Kotlin `TODO()`, handler methods unavailable) with the frozen tests,
oracle and migration.

## RED

- Go unit (`go test -json -count=1 ./internal/modules/expense/... ./internal/modules/fueling/...`,
  `f4-logs/exp002-go-red-unit.jsonl`): `expense/application` 7 top-level, 6
  failing, 77 named subtests, 76 failing. The passing top-level test pins the
  policy and catalogue constants (shape); the passing subtest is the
  `PROTOCOL` vector, refused by the shared envelope profile before any owner.
  The expense domain (15/222), payable, fueling domain/application and the
  updated fueling adapter unit test kept passing (no behaviour there changed).
- PostGIS (`go test -json -race -tags=integration -count=1 ./internal/modules/organization/adapters -run '^(TestExpense.*|TestFueling.*|TestMeterValidation.*Fueling.*|TestUnitSchema.*|TestCostCentreSchema.*)$'`,
  `f4-logs/exp002-db-red.jsonl`): 25 top-level, 21 failing; 27 named
  subtests, 12 failing. Every failure is behavioural (stubbed projection:
  "legitimate fueling was not accepted orgsync: unavailable"; stubbed owner:
  every direct expense unavailable; stubbed catch-up). The passing tests are
  the pure fueling history rule, the two cost-centre schema tests (their
  allow-list copies already list the new key) and the 000215 upgrade
  rehearsal. A first RED attempt aborted because a racing goroutine called
  `t.Fatal` after its subtest ended; the race helpers now never touch the
  test from a goroutine (no expectation changed) and the rerun is the RED
  above.
- Contract (`f4-logs/exp002-contract-red.jsonl`, against the previous spec
  image): 2/2 failing (owner missing; owner count 11).
- Kotlin (`:domain:test --tests 'com.anpfuel.domain.enterprise.expense.*'`,
  `f4-logs/exp002-kotlin-red.log`): 280 tests, 43 failed (40 dynamic vectors
  and 3 unit tests, `NotImplementedError`), the 237 EXP-001 tests and the
  corpus-completeness test pass.

## GREEN and REFACTOR

- Go unit race, all expense and fueling packages pass
  (`f4-logs/exp002-go-green-unit.jsonl`); statement coverage under Validation.
- PostGIS focused (`f4-logs/exp002-db-green.jsonl`, same selection plus the
  new pure rule test): 26 top-level / 65 named subtests, 0 failures, 0 skips.
- Contract: `internal/platform/apicontract` passes (12 owners on the base,
  19 after the merge with the GPS-004 owners).
- Kotlin (`f4-logs/exp002-kotlin-green.log`, XML in
  `f4-logs/exp002-kotlin-green-xml/`): 280 tests, 0 failures, errors or
  skips; no compiler warning.
- Corrections made while going GREEN, none changing an expectation: one unit
  case (`payload-array`) was removed because the shared envelope profile
  refuses a non-object payload before any owner, so the fixture could not be
  sealed; the forced-RLS test's "insert without an organization setting" was
  an `INSERT … SELECT` that inserts zero rows under RLS and now inserts
  explicit values (refused 42501); `TestFuelingSchemaIsForcedRLSAndInsertOnly`
  grants TRUNCATE on the new cascade target `org_expenses` too, so
  `TRUNCATE org_fuelings CASCADE` still reaches the insert-only guards (55000,
  assertion unchanged).
- REFACTOR: the projection reports whether it inserted, so the catch-up
  counts only its own inserts (a concurrent projection that wins the origin
  key is re-read and compared, never counted or overwritten); the Kotlin
  decoder applies the intake's order (final refusals before later forms).
- `TestExpenseHistoryRuleCoversOwnExpensesOnAssignedAssets` (pure, 10 cases)
  was added with the authority implementation, after the RED run; it pins the
  rule constants of the PostGIS authority tests that did run RED.

## Acceptance and negatives mapped to tests (real PostGIS, signed intake)

| Item | Test(s) |
|---|---|
| One fueling origin → one expense; replay/digest reuse never duplicate; VAL-13 period total | `TestExpenseFuelingProjectionStoresOneCostPerFueling` (97 350 paid by the employee: one `FUEL` expense of 97 350/97 350 with the fueling's facts, unit snapshot and unit-zone reporting day; two resends byte-identical; a reuse carrying 194 700 is `OPERATION_ID_REUSED`; projection re-run and catch-up write nothing; EXP-001 totals over the stored rows: 97 350 including pending, 0 approved) |
| One expense under concurrent processing | `TestFuelingIntakeConcurrentDuplicatesLeaveOneFueling` (12-way, now also asserting one expense), `TestExpenseDirectIntakeStoresOneExpenseAndReplays` (12-way direct), every `accepted` fueling helper asserts exactly one expense |
| Second cost for one fueling, both orders | `TestExpenseSecondCostForOneFuelingRacesBothOrders` (queued behind the open fueling transaction → 23505; taken first with the root lock → 23503, then the fueling commits with its one cost); `TestExpenseFuelingProjectionStoresOneCostPerFueling` (after commit → 23505) |
| Partial transaction (decided: neither half alone) | `TestExpenseFuelingPartialTransactionStoresNeither` (failure after the real expense insert; a projection that silently stores nothing is refused at commit by the deferred check; nothing stored, then the same operation records both; an expense without its fueling 23503; a DIRECT expense naming a fueling and settlement/unowned kinds 23514); existing `TestFuelingIntakeRollsBackTheReadingAndTheFuelingTogether` now also proves no expense |
| Foreign source / isolation | `TestExpenseForeignFuelingIsNeverASource` (another organization cannot reference or project this organization's real fueling, cannot write this organization's row (42501), sees no row); `TestExpenseDirectIntakeAuthorityAtReceipt/foreign-asset-not-found`, `unknown-asset-not-found` |
| Stale event never overwrites a newer revision | `TestDecideProjectionIsIdempotentAndNeverOverwrites` (stored newer revision → `ErrStale`, divergent stored cost or newer request → unavailable, equal → unchanged); database: insert-only guard and unique origin key (`TestExpenseSchemaIsForcedRLSAndInsertOnly`, second-cost tests) |
| Replay / `OPERATION_ID_REUSED` (direct) | `TestExpenseDirectIntakeStoresOneExpenseAndReplays` |
| Wrong role / wrong scope / SELF | `TestExpenseDirectIntakeAuthorityAtReceipt` (AUDITOR and APPROVER `FORBIDDEN`; FINANCE/ORG accepted; SUPERVISOR/UNIT alone an unavailable source; SELF never assigned `FORBIDDEN`; SELF assigned accepted and replayed; declared before the assignment held `HOLD_PERMISSION_CHANGED` and stored nothing; another asset assigned `FORBIDDEN`; receipt read under current `exp.expense.read`) |
| Malformed and retained forms | `TestExpenseDirectIntakeRefusesMalformedAndRetainsLaterForms` (FUEL, company > paid, decimal, payer, unknown key, draft asset, expected revision, identity of another operation → final MALFORMED replayed; trip, configured category, attachments → RETRY/UNAVAILABLE, nothing stored); parser `TestParseOperation*`; shared vectors |
| Disposal | `TestExpenseDirectIntakeDisposalStoresNothingAtOrAfterTheInstant` |
| Forced RLS, insert-only, usage keys | `TestExpenseSchemaIsForcedRLSAndInsertOnly`; unit/cost-centre fixture copies list the new keys |
| Default runtime wiring | `TestExpenseIntakeDefaultRuntimeSignedTLSAcceptsAndReplays`, `TestFuelingIntakeDefaultRuntimeSignedTLSAcceptsAndReplays` (now asserts the fueling's expense) |
| Populated upgrade 000237 head → 000244 | `TestExpensePopulatedUpgradeFailureRecoveryAndCatchUp` (two fuelings through the real intake on the previous image; injected failure leaves schema, functions, the `org_fuelings` trigger and data unchanged; recovery once; no-op replay; previous image compatible; migrator-owned forced RLS; no cost fabricated; each usage source equals its previous body plus exactly one line and answers unchanged; earlier receipts replay; catch-up run twice inserts 2 then 0 with exact values; a new fueling commits with its cost) |

## Validation

Environment: Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f4-go-build`,
shared PostGIS `127.0.0.1:32775` (orgtest child databases/roles), own MinIO
project `f4-exp002-s3` for storage tests (removed with `down -v`). Logs in
`/data/dev/worktrees/f4-logs/exp002-*`.

Every gate below ran on the merged tree (`a83c1927`, the task commit
`41adb3d6` plus the F4 head `0912b48a`) after the last code edit.

```sh
gofmt -l ./internal ./cmd ./db                                  # empty (toolchain gofmt)
go build ./... && go vet ./... && go vet -tags=integration ./... # pass
go vet -tags=integration,nativefixture ./internal/modules/organization/adapters
GOTOOLCHAIN=go1.27.1 staticcheck [-tags=integration] ./internal/modules/expense/... ./internal/modules/fueling/... \
  ./internal/modules/organization/adapters ./internal/platform/enterprisesuite ./internal/platform/apicontract   # no findings
sqlc vet && sqlc generate                                       # no diff
go test -json -race -tags=integration -count=1 -timeout=90m -p 2 ./internal/modules/organization/adapters \
  ./internal/modules/expense/... ./internal/modules/fueling/... ./internal/modules/budget/... ./internal/modules/review/... \
  ./internal/modules/orgsync/... ./internal/platform/... ./cmd/api/...
go test -count=1 -timeout=45m ./...
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.*' (flock, JDK 17, offline)
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml
python3 docs/enterprise/tools/check_plan.py
git diff --check; gitleaks on the staged diff
```

- Race/integration union (`f4-logs/exp002-db-union.jsonl`, start/end/exit
  files beside it): 37 packages (35 with tests, all pass; `legalterms` and
  `orgtest` have no test files), 1124 top-level / 8353 named subtests, 0
  failures, 0 skips, exit 0; organization adapters 3464 s under a shared-host
  load average of ~17. Storage tests used the own MinIO project
  `f4-exp002-s3` (removed afterwards with `down -v`). A first pre-merge union
  was stopped deliberately when the coordinator published the new F4 head
  (`exp002-db-union-premerge-aborted.jsonl`, no failure before the stop).
- Untagged `go test -count=1 -timeout=45m ./...`
  (`f4-logs/exp002-untagged-all.log`): 229 packages ok, exit 0.
- Unit race with coverage (`f4-logs/exp002-go-green-unit.jsonl`,
  `exp002-go-green-unit-cover.out`): expense/application 7 top / 84 named,
  expense/adapters 1, expense domain and payable unchanged (15/222, 17/275),
  fueling packages 20 top / 286 named, all pass; statements:
  expense/application 82.0%, expense domain and payable 100%, fueling
  application 86.0%, fueling domain 90.9%. Integration coverage of the new
  code (`f4-logs/exp002-cover-integration.out`, the `TestExpense*` set):
  `expense/adapters/intake.go` 69.5%, `store.go` 70.8%,
  `application/command.go` 88.2%, `projection.go` 71.4%,
  `organization/adapters/expense_authority.go` 75.0%; the uncovered lines
  are unavailable and invalid-evidence refusals.
- Kotlin enterprise suite (`f4-logs/exp002-kotlin-enterprise-final.log`):
  4138 tests, 0 failures, errors or skips (expense 280).
- staticcheck plain and `-tags=integration`: no findings
  (`f4-logs/exp002-staticcheck.log`); build/vet log
  `f4-logs/exp002-build-vet.log`.
- vacuum: 0 errors; warnings/informs 1109/681, identical to the merged base
  spec (`exp002-vacuum.log`).
- `check_plan.py`: 348 tasks, 545 rules, 167 use cases, 145 screens, 0
  problems; `git diff --check` clean; gitleaks: no leaks; the keyword scan of
  the staged diff found no credential, CPF, CNPJ, real name or real place
  (synthetic identifiers and Portuguese synthetic labels only).
- Not run: the Android emulator or device (no app change), the 15 Rust
  preparer tests of `cmd/worker`, `directory` and `quality` (not in this
  union; no change there).

## Retained

- Production trigger of the catch-up (`ProjectPendingFuelingsTx`): a job or
  command per organization must call it once for fuelings accepted before
  000244 (synthetic only today: enterprise routes are not enabled anywhere,
  EDR-272). Until then those fuelings have no expense.
- Fueling revisions and decisions (FUEL-003/015): the expense row is
  revision 1 / source revision 1 / `SUBMITTED`; projecting a newer fueling
  revision, approving, returning, partially approving the company amount and
  voiding are their owners' (the projection already refuses stale and newer
  requests without writing).
- EMP-EXP-003: configured categories replace `org_expenses_category_v1` and
  the v1 list in `DirectCategories`/`DIRECT_CATEGORIES`; a structurally valid
  unknown category stays RETRY/UNAVAILABLE until then.
- EMP-EXP-004: the Android form and queue (the builder and vectors are
  ready); receipts/attachments (DOC-004) stay RETRY/UNAVAILABLE.
- EMP-EXP-005: held direct expenses (changed rights, post-disposal) store
  nothing and stay RETRY/UNAVAILABLE until a held-expense review owner and an
  OFFL-008 review-source extension exist; the panel list/detail/review and
  review history rows.
- TRIP-020: a `trip_id` link stays RETRY/UNAVAILABLE.
- EMP-EXP-006 allocation; organization-level expenses without an asset (a
  later schema); WORK_ORDER origin (MAINT); FIN/BUDG persistence consuming
  approved company amounts; settlement never creates an expense (the database
  refuses settlement kinds as an origin).
- Production grants (proposal below); no route, matrix, inventory or auditor
  change (counts unchanged from the merged head `0912b48a`: 153 normal
  routes, routed 30 / not routed 54 / specials 26; auditor walk 153/31/122;
  matrix 300 cells); signed operation owners in OpenAPI 18 → 19.
- No F3/F4/M3/M4 acceptance, deployment, real data or release is claimed.

## Proposed shared-doc changes (coordinator)

1. `backend/README.md`, after the fueling intake grants paragraph, new
   subsection `### Expense intake and fueling projection (EMP-EXP-002)`:
   "Migration244 adds forced-RLS, insert-only `org_expenses`. Every principal
   that inserts fuelings (the signed intake) needs `SELECT, INSERT ON
   org_expenses`: a fueling commits only with its one FUEL expense. The same
   principal records `expense.submit` with the meter/fueling sources it
   already has. The migrator needs `REFERENCES` on organizations,
   org_sync_operations, org_fuelings, org_assets, org_memberships,
   org_employee_identities, org_device_enrollments, org_units and
   org_cost_centres, `TRIGGER` on org_fuelings and ownership of
   `org_asset_current_usage`, `org_unit_current_usage` and
   `org_cost_centre_current_usage`. Fuelings accepted before 000244 receive
   their expense from the per-organization catch-up; nothing provisions live
   grants."
2. `docs/enterprise/09-architecture.md` module row for `expense`: add
   "persistence, fueling projection and `expense.submit` LOCAL_DONE".
3. `docs/enterprise/modules/fuelings.md` §9: "Accepted fuelings store their
   one expense in the same transaction (EMP-EXP-002, EDR-287)."
4. `docs/enterprise/10-offline-sync.md` operation catalogue: add
   `expense.submit` (schema 1, EDR-289).
5. `scripts/check-manifest.txt`, `contracts/openapi/v1.yaml`, `sqlc.yaml`
   and the enterprisesuite composition were edited here as listed above.
