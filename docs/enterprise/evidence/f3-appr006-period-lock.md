# EMP-APPR-006 — Period closing, lock and reopening

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F3 task (module APPR, priority P1, type API, size M) in worktree
branch `codex/enterprise-f3-appr006`; the coordinator integrates it into
`codex/enterprise-f3`. It delivers the authoritative source of the period
facts that the pure amendment kernel (`orgamend/domain`, EMP-PLAT-016) takes
as input: persisted organization period state, the close/lock, reopen and
re-lock commands with expected revisions, read/list routes, and a server-side
gate that an owning record command calls inside its own transaction.

Sources read: the EMP-APPR-006 task block (ep10); BR-AMD-001/004, BR-TIME-003,
BR-TIME-004 and BR-AUD-001 ([04-business-rules](../04-business-rules.md));
the Period row `OPEN`/`LOCKED`/`REOPENED` of
[08-state-model](../08-state-model.md); [expenses-finance](../modules/expenses-finance.md)
§6/§8; the `fin.period.close`/`fin.period.reopen` catalog rows and matrix cells
(OWNER and FINANCE, `O`) of [07-permissions](../07-permissions.md); REQ-11.4;
VAL-19 (October `2026-10-01` inclusive to `2026-11-01` exclusive, fueling at
`2026-11-01T02:30:00Z` in `America/Sao_Paulo` belongs to `2026-10-31`) of
[15-validation](../15-validation.md); the EMP-PLAT-016 kernel and
[its evidence](f3-plat016-amendment-kernel.md); the EMP-APPR-001 review
lifecycle; the time kernel `AttributeReportingDay`; ORG-005 units and ORG-008
settings (the published organization zone); the `orgaccess` pipeline, the
`orgaudit` transaction runner and the FLEET-002/METER-002 route conventions.

Out of scope: bypassing a lock, rewriting approved revisions, real consuming
modules (fuelings and expenses do not exist yet), notifications, unit or
cost-centre scoped locks, Android/Kotlin.

## Rules frozen before behaviour

Pre-allocated identifiers: B-BR-EMP-211..214, BUC-EMP-108..109,
EDR-148..149. All are used; none remains reserved.

Proposed contract section title: "EMP-APPR-006 — Period closing, lock and
reopening".

Rules B-BR-EMP-163, B-BR-EMP-211, B-BR-EMP-212, B-BR-EMP-213, B-BR-EMP-214, BUC-EMP-108, BUC-EMP-109 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-APPR-006 — Period closing, lock and reopening" (frozen before behaviour; text moved verbatim at integration).

### Decision rows

Decisions EDR-148, EDR-149 are recorded in [16-decisions](../16-decisions.md).

## Delivered surface

- Pure domain `backend/internal/modules/review/domain/period/period.go`:
  canonical `Month` (`ParseMonth`, `MonthOf`, half-open `Start`/`End`,
  `Contains`), `Head` with the parity invariant and implicit `Initial()`
  (`OPEN`, revision 1), `Decide(head, command)` (INVALID, CONFLICT,
  TRANSITION, then the reopening reason), `Transition.Validate/After` for
  stored facts and `CheckReason` (B-BR-EMP-163 profile in the exact order of
  the `orgamend` kernel). No I/O, clock or authority.
- Application `backend/internal/modules/review/application/periods.go`: fixed
  policies (`fin.period.close` read and fresh `ConfigurationWrite` close,
  fresh `ConfigurationWrite` `fin.period.reopen`), validated immutable
  `PeriodCommand` with a versioned organization-bound SHA-256 digest, ports
  (`PeriodAuthority`, `PeriodRepository`, `PeriodGate`), `PeriodService`
  binding the completed request decision before and after the repository and
  validating every result (revision chain, range, ordering, page bounds), and
  `PeriodFact` with `Validate`, `Writable` and `Amendment()` mapping to
  `orgamend.Period` (authority proven only from a stored `fin.period.reopen`
  transition of the head revision with an admissible reason, recorded no
  later than the check).
- Persistence: migration `backend/db/migrations/000203_finance_periods.sql`
  (`org_finance_period_transitions` immutable history + receipt with a
  deferred audit FK, `org_finance_periods` trigger-maintained head, forced RLS,
  history triggers, transition guard under the root `FOR UPDATE`, head
  guard), sqlc package `backend/db/queries/review` (new `sqlc.yaml` stanza),
  `backend/internal/modules/review/adapters/periods.go` (`PeriodStore`,
  `PeriodGate`, audit vocabulary `fin.period.locked/reopened/relocked` on
  target type `finance_period`).
- Authority: `backend/internal/modules/organization/adapters/period_authority.go`
  (existing company pipeline on the organization target, ACTIVE root for
  writes) and the three period policies added to the fixed whitelist of
  `company_authority.go`.
- HTTP: `backend/internal/modules/review/adapters/http/periods.go` —
  `POST /v1/orgs/{org_id}/finance/periods/list`, `.../{period}/read`
  (`fin.period.close`), `.../{period}/close` (`fin.period.close`, fresh,
  revision) and `.../{period}/reopen` (`fin.period.reopen`, fresh, revision,
  reason); strict lossless DTOs, opaque NOT_FOUND for non-canonical months,
  keyset cursor bound to filter and caller; composed in
  `internal/platform/enterprisesuite/routes.go` with quota class `fin.period`
  (identity v1 write allowance, synthetic) and the audit definitions.
- Contract and inventories: four operations, tag `enterprise-finance` and
  nine schemas in `contracts/openapi/v1.yaml`;
  `contracts/testdata/enterprise/authorization-routes-v1.json` routes both
  permissions (17 routed, 67 not yet routed, 86 normal routes); the inventory
  test counts, the full-router mounted list, the authorization matrix
  (two new representatives, private-state snapshot extended to both period
  tables) and `scripts/check-manifest.txt` (four quick packages).
- No Kotlin mirror and no shared vector corpus: periods are server-side
  facts with no client consumer (as EMP-PLAT-016); the oracles are the Go
  tables, the exhaustive invariant test and the real PostGIS tests.

## Design notes

- Locking protocol (B-BR-EMP-214). A period command takes the company
  authority (account, proof, capability, organization root `FOR UPDATE`) and
  then `PeriodRootLock` again explicitly; the SQL transition guard locks the
  root `FOR UPDATE` too, so even direct SQL serializes. The gate runs inside
  the owning record transaction (READ COMMITTED, scope validated), takes
  `FOR KEY SHARE` on the root, then reads the committed head and, for
  `REOPENED`, the reopening transition, and only then takes the check instant.
  `FOR KEY SHARE` conflicts only with `FOR UPDATE`, so it does not serialize
  ordinary root updates (`FOR NO KEY UPDATE`), only period commands and other
  company writes that already lock the root exclusively.
- Revisions: an untouched month is `OPEN` at revision 1 (never stored);
  `LOCK` writes revision 2, then `REOPEN`/`RELOCK` alternate, so `LOCKED` is
  even and `REOPENED` odd in Go, in the CHECK constraints and in the guards.
- Replay: the transition row is the receipt (`UNIQUE(org_id,operation_id)`).
  Same account and same digest returns the stored result with
  `replayed=true` after re-checking current authority; anything else, or an
  operation ID already audited by another module (`orgaudit.ErrReused`), is
  `OPERATION_ID_REUSED`.
- Time: the gate's attribution reads the published organization zone
  (`organizations.time_zone`, owned by ORG-008 settings) or the zone of a unit
  of the same organization under RLS; an unknown or foreign unit is
  NOT_FOUND and never falls back.
- Reason order: the first implementation followed the review lifecycle
  (control before blank); before GREEN it was aligned to the `orgamend`
  order (blank before control) so a whitespace-only reason containing a tab
  is `REASON_REQUIRED` in both; the application test expectation for `"\t"`
  was updated with that decision and a `"Synthetic\treason"` case added (both
  still refused). A unit-scoped role cannot be persisted before EMP-AUTH-006,
  so the "grant not covering the organization" case uses a SELF-scoped
  FINANCE grant.

## Negative cases → tests

| Negative case | Test (all real PostGIS unless noted) |
|---|---|
| Wrong time zone | `TestPeriodGateAttributesBoundaryInOrganizationAndUnitZone`: `2026-11-01T03:59:59.999999999Z` is `2026-10-31` (LOCKED) in America/Cuiaba while its UTC day is `2026-11-01`; `04:00:00Z` is November (OPEN); `2026-10-01T03:59:59.999999999Z` is September; unit America/Sao_Paulo decides `03:30Z` as November; VAL-19 `02:30Z` is `2026-10-31`; unknown/foreign unit NOT_FOUND |
| Unauthorized close/reopen | `TestPeriodStoreAuthorizationAtReceipt` (DRIVER and ADMIN forbidden, borrowed membership, SELF-scoped FINANCE forbidden, other organization NOT_FOUND, suspended organization ORG_SUSPENDED with reads kept, ended membership NOT_FOUND, pending setup ORG_SETUP_INCOMPLETE); HTTP 403 with permission key; matrix cells |
| Reopen without reason | domain/app tables; HTTP 422 for missing, blank, control and 281-scalar reasons, 400 for a lone surrogate; SQL CHECK refuses a reasonless REOPEN |
| Stale revision / double close | `TestPeriodStoreRefusalsChangeNothing`, HTTP 409 CONFLICT, `TestPeriodStoreConcurrentClosesAdmitOne` (8 concurrent closes, exactly one transition and one audit) |
| Race admits locked write | `TestPeriodGateRaceCloseVersusRecordWrite`: write holds the gate and the close waits (lock-wait barrier), then locks after the write; the re-lock holds the root and the write waits, then sees LOCKED revision 4 and writes nothing; unsynchronized writers never commit after the lock |
| Approval history lost | `TestPeriodSchemaKeepsHistoryAppendOnlyAndScoped` (UPDATE/DELETE/TRUNCATE refused, forged/gap/overwrite transitions and head rewrites refused, unaudited transition fails at commit); history read keeps LOCK, REOPEN and RELOCK |
| Replayed command | `TestPeriodStoreReplayAndOperationReuse`, HTTP replay with identical acknowledgement |
| RLS without org setting | same schema test: nothing visible or insertable without `anpfuel.org_id`; cross-organization insert refused |
| Populated upgrade | `TestPeriodPopulatedUpgradeFailureRecoveryAndPreviousImage` (previous head = every migration before `000203`): injected failure leaves no table/function/ledger row and identical populated rows; ordinary NOSUPERUSER migrator recovery applies exactly `000203`; repeat is a no-op; previous embedded image compatible; ordinary ownership and forced RLS; upgraded commands and gate work, another organization stays OPEN |
| BR-AMD-004 end to end | `TestPeriodGateFeedsAmendmentKernel` (test-only consumer): OPEN amend allowed; LOCKED amend/void `PERIOD_LOCKED`, adjustment into open November; after reopening amend allowed and adjustment `PERIOD_NOT_LOCKED`; after re-lock `PERIOD_LOCKED` |

## Validation

Toolchain Go 1.27.4 (`export PATH=/data/dev/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.4.linux-amd64/bin:$PATH GOCACHE=/data/dev/cache/f3-go-build GOTMPDIR=/data/dev/cache/f3-tmp TMPDIR=/data/dev/cache/f3-tmp`),
staticcheck under Go 1.27.1, sqlc v1.31.1, vacuum; commands from `backend/`
unless noted. `ANPFUEL_TEST_DATABASE_URL` selects the owned disposable
control database on 127.0.0.1:32775; `orgtest` creates an isolated child
database and an ordinary NOSUPERUSER/NOBYPASSRLS runtime login per test.

RED (exported API compiled with stub bodies, no migration):

- Unit: `go test -count=1 -race -json ./internal/modules/review/...` plus
  `-run TestPeriod` (organization/adapters), `-run TestFinancePeriod`
  (apicontract) and `-run TestAuthorizationInventory` (cmd/api): 23
  top-level tests fail by assertion (8 domain, 5 application, 3 adapters,
  4 HTTP, 1 authority, 1 contract, 1 full-router inventory), 11 pre-existing
  top-level tests pass, no panic, no compile error. Log
  `/data/dev/worktrees/f3-logs/appr006-go-red-unit.jsonl`.
- Integration: `go test -count=1 -race -tags=integration -json ./internal/modules/organization/adapters -run 'TestPeriod|TestAuthorizationMatrixActualCompanyHTTP'`:
  14 top-level fail, 12 reaching `relation "org_finance_period_transitions" does not exist (42P01)` on real PostGIS, the
  upgrade test refusing the missing `000203` file, the authority stub refusing; 16.6 s. Log
  `/data/dev/worktrees/f3-logs/appr006-go-red-integration.jsonl`.

GREEN:

- Review packages, unit with atomic coverage: 20 top-level / 32 subtests,
  0 failures/skips; `/data/dev/worktrees/f3-logs/appr006-go-green-unit.jsonl`,
  profile `/data/dev/worktrees/f3-logs/appr006-go-cover-unit.out`:
  `review/domain/period` **100.0%**, `review/application` 87.8%,
  `review/adapters/http` 50.5%, `review/adapters` 20.5% (unit only; DB paths
  are exercised by the integration run below).
- Integration union `go test -count=1 -race -tags=integration -covermode=atomic -coverpkg=./internal/modules/review/... -json ./internal/modules/organization/adapters ./internal/modules/review/...`:
  6 packages pass, 382 top-level / 2914 subtests,
  0 failures/skips (organization/adapters 584.3 s under race and coverage).
  It includes the 13 period integration tests (each on its own isolated
  PostGIS database), the authorization matrix now at 170 runtime cells
  (59 allowed / 111 denied; two new representatives `fin.period.close` list
  and `fin.period.reopen`, each allowed cell reopening a month the owner
  locked first; every refusal compares the private state including both
  period tables) and every pre-existing organization, membership, fleet,
  meter and sync integration test. In the unsynchronized writer sub-case all
  6 writes happened to commit before the lock; the deterministic barrier
  sub-cases prove both orderings. Log
  `/data/dev/worktrees/f3-logs/appr006-go-green-integration.jsonl`; combined
  coverage `/data/dev/worktrees/f3-logs/appr006-go-cover-integration.out`:
  `review/domain/period` **100.0%**, `review/application` 88.8%,
  `review/adapters/http` 84.6%, `review/adapters` 81.6% (uncovered lines are
  fail-closed branches for corrupt stored rows and provider errors).
- `go test -count=1 -race -tags=integration ./internal/platform/migrate`: 10
  top-level / 15 subtests pass (full chain including `000203`, idempotent);
  `/data/dev/worktrees/f3-logs/appr006-go-migrate-integration.jsonl`.
- Contract/router: `go test -count=1 ./internal/platform/apicontract ./cmd/api`
  84 top-level / 615 subtests pass (route-vs-OpenAPI gate, authorization
  inventory with mutations, full production router, finance contract test);
  `/data/dev/worktrees/f3-logs/appr006-go-contract.jsonl`.
- Untagged whole module (CI): `go test -count=1 -json ./...` — 198 packages
  pass, 73 without tests, 2462 top-level / 6241 subtests pass, 0 failures,
  4 pre-existing skips; `/data/dev/worktrees/f3-logs/appr006-go-untagged-all.jsonl`.

Gates, all passing:

```sh
go build ./... && go vet ./...                     # clean
go vet -tags=integration ./internal/modules/organization/adapters ./internal/platform/migrate ./cmd/api ./internal/modules/review/...
gofmt -l ./internal ./cmd ./db                     # empty
/home/iiii/go/bin/sqlc vet && /home/iiii/go/bin/sqlc diff   # clean (generate reproduces db/queries/review)
GOTOOLCHAIN=go1.27.1 STATICCHECK_CACHE=/data/dev/cache/f3-staticcheck /home/iiii/go/bin/staticcheck ./internal/modules/review/... ./internal/modules/organization/adapters/... ./internal/platform/enterprisesuite ./internal/platform/apicontract ./cmd/api   # no findings
GOTOOLCHAIN=go1.27.1 STATICCHECK_CACHE=/data/dev/cache/f3-staticcheck /home/iiii/go/bin/staticcheck -tags=integration ./internal/modules/organization/adapters ./internal/modules/review/...   # no findings
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check   # repo root: 0 errors; 905 warnings / 575 info vs 901 / 570 at base (4 operation-description, 1 description-duplication style warnings, 5 info)
python3 docs/enterprise/tools/check_plan.py         # repo root: 348 tasks, 545 rules, 167 use cases, 145 screens, 0 problems
git diff --cached --check                           # clean
git diff --cached | grep -nEi 'password|secret|token|BEGIN .*KEY|@gmail|cpf'   # see below
```

Secret/PII scan: three hits, all synthetic or literal — the repeated-byte
`CursorSecret` of the HTTP test router, the string "synthetic private provider
secret" asserting that provider errors stay private, and the scan command in
this file; `git diff --cached --binary | gitleaks stdin --redact` finds no
leaks. Data is synthetic only (no names, CNPJ or real accounts).

Logs: `/data/dev/worktrees/f3-logs/appr006-staticcheck.log` (empty),
`/data/dev/worktrees/f3-logs/appr006-vacuum.log`.

## Retained obligations

- Real consumers: fueling, expense, trip and work-order record commands must
  call `PeriodGate` inside their own transactions, map facts with
  `PeriodFact.Amendment()` into `orgamend`, post adjustments, and bind the
  existing sync review code `PERIOD_LOCKED` for late intake (EMP-FUEL-015,
  EMP-EXP-002 and owners). Derived-data recalculation (BR-AMD-005) stays with
  them. Only a test-only consumer table exists here.
- Unit/cost-centre scoped periods (EMP-AUTH-006 and the finance policy); a
  policy on locking current or future months and close ordering, if a source
  ever states one.
- A read permission for AUDITOR/period list (catalog change) — today only
  OWNER and FINANCE (`fin.period.close`) can read periods.
- Notifications on lock/reopen, panel screens, production runtime grants
  (see proposed README text), quota measurement and real data.
- Integration renumbering of `000203` changes only `periodMigration` in
  `period_upgrade_integration_test.go`, the migration file name and the
  `sqlc.yaml` schema entry.
- No F3, M3 or C1 acceptance is claimed.

## Proposed shared-doc changes

1. Product contract: new section "EMP-APPR-006 — Period closing, lock and
   reopening (2026-11-14)" with the evidence link and the CONTRACT block above
   verbatim.
2. `16-decisions.md`: append the EDR-148 and EDR-149 rows above.
3. `backlog/ep10-expenses-finance.md`: EMP-APPR-006 `state LOCAL_DONE` and the
   BACKLOG note below after its block.
4. `backend/README.md` runtime rollout grants (no live provisioning): "EMP-APPR-006
   (migration 000203): `GRANT SELECT,INSERT ON org_finance_period_transitions`
   and `GRANT SELECT,INSERT,UPDATE ON org_finance_periods` to the runtime role
   (head writes are only admitted through the transition trigger and its
   guards); owning record commands that call the period gate need `SELECT` on
   both tables, `SELECT` on `org_units` and the existing root row-lock
   privilege on `organizations`."
5. `07-permissions.md` §7.2, after the `fin.period.close` / `fin.period.reopen`
   row: "Until a period read key exists, `fin.period.close` also gates period
   list and read (EDR-149); period grants must cover the whole organization."
6. `08-state-model.md` Period row note: "A month without history is `OPEN` at
   the implicit revision 1; transitions `LOCK`, `REOPEN` (reason) and
   `RELOCK` append history; `LOCKED` revisions are even (EDR-148)."
7. `modules/expenses-finance.md` §6: "In F3 a period is the organization-wide
   calendar month of reporting days; record commands read it through the
   period gate at receipt (B-BR-EMP-211..214)."
8. `09-architecture.md` review module: "Period persistence (`review/adapters`,
   migration 000203) and routes `finance/periods/*`."
9. `PROGRESS.md`: EMP-APPR-006 LOCAL_DONE / INTEGRATION_PENDING with this
   evidence link.
