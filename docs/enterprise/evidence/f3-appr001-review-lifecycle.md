# EMP-APPR-001 — Pure revision-bound review lifecycle

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F3 pure-domain task in worktree branch
`codex/enterprise-f3-appr001`; the coordinator integrates it into
`codex/enterprise-f3`. It delivers the shared single-level (C1) review
lifecycle that future trip, fueling, expense and work-order owners embed
(EDR-012): states, author and reviewer commands, revision binding,
append-only history, required bounded reasons, separation of duties on
canonical person identity, permission at receipt as an explicit input fact,
and an honest local presentation that never turns "ready to send" into a
server state. Rules: BR-REV-001..007 and BR-REV-009, BR-APPR-001, 07-permissions
decision order, 08-state-model §1, VAL-01 and VAL-25 expectations.

Out of scope and untouched: multi-level policies and thresholds
(EMP-APPR-002), substitutes (EMP-APPR-003), partial amounts (EMP-APPR-004),
period lock (EMP-APPR-006), amendments and void of approved records
(EMP-PLAT-016, built in parallel; not imported or duplicated), persistence,
SQL, routes, OpenAPI, app/panel UI, jobs and producers. No dependency is added.

## Rules frozen before behaviour

Pre-allocated identifiers used: B-BR-EMP-175..179, BUC-EMP-92..93,
EDR-128..130. All allocated identifiers are used; none stays reserved.

Rules B-BR-EMP-163, B-BR-EMP-175, B-BR-EMP-176, B-BR-EMP-177, B-BR-EMP-178, B-BR-EMP-179, BUC-EMP-92, BUC-EMP-93 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-APPR-001 — Review lifecycle component" (frozen before behaviour; text moved verbatim at integration).

Decisions EDR-128, EDR-129, EDR-130 are recorded in [16-decisions](../16-decisions.md).

## Design notes

- API (Go `backend/internal/modules/review/domain`, Kotlin
  `com.anpfuel.domain.enterprise.review`): closed `State` and command `Kind`
  tokens with parsers; `Identity` input sets; `Facts` (actor, author,
  optional beneficiary, permitted); `Command` (kind, named revision, reason);
  immutable `Case` built only by `NewDraft`, `Replay` and `Apply`; append-only
  `Entry` history returned as a copy; `Present` for the local view; `Basis`
  for BR-REV-009.
- An error returns no new case. The input case is a value and never changes;
  both vector consumers assert the case is identical after every refusal.
- The revision only grows by one per RESUBMIT entry, so it is bounded by the
  history length and cannot overflow a signed 64-bit integer.
- Future extension: EMP-APPR-002/004 add a level and an optional company
  amount to a decision and a policy to the case in a review-v2 oracle. A
  single-level policy keeps review-v1 behaviour, so its rows stay valid.

## RED against compiled stubs

The exported Go and Kotlin APIs first compiled with zero-value or `TODO()`
bodies, so the tests compiled and failed on behaviour.

- Go (`go test -count=1 -json`, 230-row corpus): 9 top-level tests, 7 fail
  and 2 pass (failure-vocabulary distinctness and the strict loader grammar,
  which do not depend on behaviour); all 230 vector subtests fail; 0 skips;
  package 0.008 s. Log `/data/dev/worktrees/f3-logs/appr001-go-red.jsonl`.
- Kotlin (`:domain:test --tests 'com.anpfuel.domain.enterprise.review.*'`):
  238 tests, 237 fail (the grammar test passes), 0 skips. Lifecycle 6/6 fail
  in 0.100 s; vectors 231 of 232 fail in 0.296 s; Gradle 35 s. Logs
  `/data/dev/worktrees/f3-logs/appr001-kotlin-red.log` and
  `appr001-kotlin-red-*.xml` beside it.

## GREEN, REFACTOR and the frozen oracle

The first implementation passed both consumers unchanged: Go 9 top-level /
230 subtests and Kotlin 238 tests, 0 failures or skips (Gradle 14 s). The
REFACTOR pass built replayed histories linearly in both languages, shared one
Kotlin transition helper between apply and replay, removed a redundant Go
revision guard and escaped two format characters in test literals that
staticcheck ST1018 reported. Coverage review then showed an untested Kotlin
dash-position branch, so two APPLY rows (misplaced dash, undashed 36 hex
characters) were added before freezing. They passed on first run in both
languages (Go reuses `kernel.ParseOrgID`); they are not claimed as RED.

Frozen `review-v1.psv`: 232 cases, STATE 16, COMMAND 15, APPLY 155, REPLAY
22, PRESENT 24; errors INVALID 80, INVALID_TRANSITION 48, FORBIDDEN 15,
SELF_APPROVAL 15, CONFLICT 11, REASON_REQUIRED 6. SHA256
`fac6c92c9c6929dc59f724b6f9912149d2ec92206138ada14beeec2739dffa08`. Both
consumers assert the hash, counts, the closed error set, all 49
state/command pairs, an unchanged input case after every refusal and an
exact replay of every accepted history.

A manual mutation spot-check of the Go source (decision from SUBMITTED,
beneficiary check removed, revision check removed, control check narrowed to
NUL, reason bound off by one, cancel from IN_REVIEW) failed 3 to 12 tests per
mutation; the source was restored byte-identical and retested.

## Validation

Final results, worktree `/data/dev/worktrees/enterprise-f3-appr001`:

- Go 1.27.4 from `backend`: `gofmt -l ./internal/modules/review` empty;
  `go vet ./internal/modules/review/...` clean; `go test -race -count=1
  -covermode=atomic -coverprofile=... -json ./internal/modules/review/...`
  passes 9 top-level / 232 subtests, 0 failures, 0 skips, 1.041 s; statement
  coverage 100.0% for `review/domain`; `GOTOOLCHAIN=go1.27.1 staticcheck
  ./internal/modules/review/...` clean. Logs `appr001-go-green.jsonl`,
  `appr001-go-coverage.out`, `appr001-go-staticcheck.log`.
- Kotlin (JDK 17, serialized Gradle lock): review package 240 tests
  (lifecycle 6, vectors 234 = 232 shared + grammar + pair coverage); whole
  enterprise domain suite `--tests 'com.anpfuel.domain.enterprise.*'`
  31 suites / 1591 tests, 0 failures, 0 errors, 0 skips, Gradle 24 s. JaCoCo
  for `com.anpfuel.domain.enterprise.review`: instructions 1202/1211
  (99.26%), branches 125/128 (97.66%), lines 99/99 (100%); the misses are
  generated enum `valueOf` and the unreachable empty-map branch of
  `terminal`. `:domain:tasks --all` lists no ktlint, detekt or other lint
  task, so none applies. Logs `appr001-kotlin-green-*.xml`,
  `appr001-kotlin-enterprise-final.log`, `appr001-kotlin-jacoco.xml`.
- `python3 docs/enterprise/tools/check_plan.py`: 348 tasks, 545 rules,
  167 use cases, 145 screens, 0 problems.
- `git diff --cached --check` clean. The secret grep matches only the word
  "token" (state and command tokens); `gitleaks stdin --redact` finds no
  leaks. Data is synthetic (fixture UUIDs, invented reasons); no PII.

No dependency, SQL, migration, route, OpenAPI, app/data module, DI, job or
producer is added. The component imports only the accepted `kernel` UUID
parser and does not import or duplicate `orgamend` (EMP-PLAT-016).

## Retained obligations

- Owners (trips, fuelings, expenses, work orders) still have to embed the
  component, bind organization scope, author and beneficiary references,
  persist cases and append-only history (`unique (review case, level,
  revision)` per 06-domain-model), and replay stored history.
- A server use case must resolve canonical identities through the actual
  employee merge chain, ask the current authority port at receipt (BR-REV-004,
  BR-TIME-003), compose OPEN and a decision in one transaction if wanted,
  write the audit event with actor and reason (BR-AUD-001) and map the
  domain codes to the orgapi vocabulary (FORBIDDEN and SELF_APPROVAL as is;
  CONFLICT and INVALID_TRANSITION to CONFLICT; INVALID and REASON_REQUIRED to
  its validation codes).
- Real competing decisions, idempotent replays and concurrency need
  PostgreSQL tests with the first persistence owner.
- Levels and routing (EMP-APPR-002), substitutes (EMP-APPR-003), partial
  amounts (EMP-APPR-004, review-v2 oracle), period lock (EMP-APPR-006),
  amendments and void (EMP-PLAT-016), cost totals using the basis
  (EMP-EXP-001), panel SCR-APPR-01 and app presentation remain open.
- No persistence, API, app, M3, F3 or C1 acceptance is claimed.

## Proposed shared-doc changes

1. `docs/product/ABASTEVO_EMPRESAS_CONTRACT.md`: add a section
   `## Enterprise pure review lifecycle (EMP-APPR-001)` holding the CONTRACT
   block above.
2. `docs/enterprise/16-decisions.md`: append the EDR block rows.
3. `docs/enterprise/backlog/ep10-expenses-finance.md`, task EMP-APPR-001:
   update its state per the coordinator's process and add the note below.
4. `scripts/check-manifest.txt` `[quick-packages]`: add
   `backend/internal/modules/review/domain` after
   `backend/internal/modules/fueling/domain`.
5. `docs/enterprise/09-architecture.md` module table: the `review` row may
   note that the pure C1 lifecycle exists in `review/domain` (persistence,
   policies and periods still proposed).

