# EMP-PLAT-016 — Amendment kernel: append-only corrections for approved records

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F3 pure-domain task on `codex/enterprise-f3-plat016`. The kernel
decides how a consuming module (fueling, expense, trip and others) corrects a
record inside its own transaction: ordinary edit before approval, amendment
revision, void, adjustment in a later writable period, or a closed refusal.
It also defines the effect of a review result on a pending amendment and a
pure derived-data marker. No persistence, SQL, route, job, producer, period
command, review lifecycle or app/panel surface is added. Amendments of
approved records are server-side only, so no Kotlin mirror is built; the shared
vectors are the single oracle for any future client.

Sources read: BR-AMD-001..006, BR-TIME-003/004, BR-AUD-001/004, BR-REV-003/
004/005/007, the period states `OPEN`/`LOCKED`/`REOPENED` and rules 4.1/4.3/
4.7 of the state model, the expenses-finance module sections 6 and 8, UC-FUEL-08,
VAL-19, VAL-20, VAL-26, the permission catalog (`fuel.fueling.amend`,
`fin.period.reopen`) and the platform audit module (`orgaudit/domain`).

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-190..193, BUC-EMP-98..99, EDR-137..138. All are
used; none are left reserved.

Proposed contract section title: "Enterprise amendment kernel (EMP-PLAT-016)".

Rules B-BR-EMP-190, B-BR-EMP-191, B-BR-EMP-192, B-BR-EMP-193, BUC-EMP-98, BUC-EMP-99 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-PLAT-016 — Amendment kernel for approved records" (frozen before behaviour; text moved verbatim at integration).

| ID | Decision | Why | Rejected | Scope/refs |
|---|---|---|---|---|
Decisions EDR-137, EDR-138 are recorded in [16-decisions](../16-decisions.md).

## Decision order and vocabulary

`Decide(facts, request)` checks, first failure wins:

1. `INVALID` — record/actor UUID syntax (`orgaudit.ValidID`), closed intent,
   review-state and period tokens, revision consistency (latest at least 1,
   effective between 0 and latest; never-approved is never `APPROVED`/`VOIDED`;
   effective equal to latest is `APPROVED` or `VOIDED`; latest above effective
   is never `APPROVED`), reporting/posting days (`YYYY-MM-DD`, posting strictly
   later), posting fields only for `ADJUST`, a reopening fact only on
   `REOPENED`, nonzero bounded receipt time, expected revision at least 1 and
   nonnegative plan numbers.
2. `CONFLICT` — expected revision differs from latest.
3. `ALREADY_VOIDED`.
4. State — `EDIT`: `NOT_EDITABLE` or `APPROVED_IMMUTABLE`; corrections:
   `NOT_APPROVED`, then `AMENDMENT_PENDING`.
5. Append plan — `LOST_ORIGINAL` (overwrite or wrong link), `INVALID` (gap,
   or a revision on `VOID`/`ADJUST`).
6. Reason — `REASON_REQUIRED`, `REASON_INVALID`.
7. `UNAUTHORIZED`.
8. Period — record period for `AMEND`/`VOID`; record then posting period for
   `ADJUST` (`PERIOD_NOT_LOCKED` when the record period is writable).

`Resolve(facts, review)` checks `INVALID`, `CONFLICT` (judged revision is not
latest), `ALREADY_VOIDED`, `NOT_APPROVED`, `CONFLICT` (no pending amendment),
then for `APPROVED` only the record period gate.

## Validation

Toolchain Go 1.27.4 (`GOCACHE=/data/dev/cache/f3-go-build`), staticcheck under
Go 1.27.1; commands run from `backend/` unless noted.

Oracle frozen before behaviour: `amendment-v1.psv`, 193 cases (DECIDE 165,
RESOLVE 28), SHA256
`da7d99d9355d92a0c0869a1d97ceb1d55ab9b633e42d044eb348b89a08e81a2e`, pinned in
the test with exact counts. Every error code, outcome, cause and result, every
intent, review state and period state is asserted as exercised. A grammar test
proves the strict loader rejects malformed fields, non-canonical integers,
booleans, hex and instants, duplicate/blank/padded IDs, unknown operations and
arguments, and ambiguous fixture defaults; it also proves the error mapper
refuses foreign and ambiguous errors.

RED (exported API compiled, `Decide`/`Resolve` returning an unexported
not-implemented error): `go test -race -count=1 -json ./internal/modules/orgamend/...`
— 9 top-level tests, 7 fail and 2 pass (the corpus coverage and grammar tests,
which check the oracle and loader, not the kernel); 193/193 vector subtests
fail on assertion mismatches; no compile error, no skip; package 0.026s. Log
`/data/dev/worktrees/f3-logs/plat016-go-red.jsonl`.

GREEN after implementation: 9/9 top-level and 193/193 subtests pass, zero
failures and skips, package 2.897s under `-race` with atomic coverage. Log
`/data/dev/worktrees/f3-logs/plat016-go-green.jsonl`; coverage
`/data/dev/worktrees/f3-logs/plat016-go-cover.out`: **100.0%** of statements
of `orgamend/domain`.

REFACTOR (tests only, re-run green): the exhaustive invariant enumeration
replays only accepted decisions for determinism, compares values without
reflection and leaves unknown tokens and misplaced reopenings to the vectors,
cutting `TestDecideInvariants` from 7.5s to 1.6s under `-race`. It still
enumerates every known state, effective 0..2, latest 1..3, five period facts,
all four intents, stale/current expectations, three revision and base plans,
authority on/off, reason on/off and three posting periods, and proves: no stale,
voided or unauthorized success; effective revision never changed by `Decide`;
EDIT/AMEND append exactly latest+1 linked to latest/effective; VOID and ADJUST
append nothing, link the effective revision and emit the exact marker; AMEND
and VOID only in writable periods; ADJUST only from `LOCKED` to a later
writable day; refusals are closed and return zero values. `Resolve` is
enumerated over all states, periods, results and judged revisions. Unit tests
prove UTC normalization without a monotonic reading, bounded instants and days,
no mutation or sharing of inputs and markers, and that applied decisions replay
as `CONFLICT`/`ALREADY_VOIDED` (V-IDEM).

Gates, all passing:

```sh
export PATH=/data/dev/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.4.linux-amd64/bin:$PATH GOCACHE=/data/dev/cache/f3-go-build
gofmt -l ./internal/modules/orgamend                       # empty
go vet ./internal/modules/orgamend/...                     # clean
go test -race -count=1 -coverprofile=/data/dev/worktrees/f3-logs/plat016-go-cover.out -covermode=atomic -json ./internal/modules/orgamend/... > /data/dev/worktrees/f3-logs/plat016-go-green.jsonl
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/orgamend/...   # no findings
python3 docs/enterprise/tools/check_plan.py                 # root: 348 tasks, 545 rules, 167 use cases, 145 screens, 0 problems
git diff --cached --check                                   # clean
git diff --cached | grep -nEi 'password|secret|token|BEGIN .*KEY|@gmail|cpf'
git diff --cached --binary | gitleaks stdin --redact --no-banner                       # no leaks
```

The keyword scan only matches the word "token" in vocabulary prose (state,
period and intent tokens); there is no credential, key or personal data. All
identifiers are synthetic `9e000000-…` UUIDs. Source manifest
`/data/dev/worktrees/f3-logs/plat016-source.sha256` (four Go files, README and
oracle). No Kotlin gate: no client mirror is required (EDR-137). No
dependency, migration, SQL, route, OpenAPI, job, producer, app or panel change.

## Proposed shared-doc changes

1. `docs/product/ABASTEVO_EMPRESAS_CONTRACT.md`: append a section titled
   "Enterprise amendment kernel (EMP-PLAT-016)" holding the CONTRACT block
   above.
2. `docs/enterprise/16-decisions.md`: append the EDR block above (EDR-137,
   EDR-138) after the last existing decision row.
3. `docs/enterprise/backlog/ep01-platform.md`: set EMP-PLAT-016 `state
   LOCAL_DONE` and add the BACKLOG block below under its task block.
4. `scripts/check-manifest.txt`: add `backend/internal/modules/orgamend/domain`
   to `[quick-packages]` after `backend/internal/modules/orgaudit/adapters`, so
   quick verification runs the new suite.
5. `contracts/testdata/enterprise/README.md`: append a section
   "Amendment kernel v1" reading: "EMP-PLAT-016 executes all 193 rows of
   amendment/amendment-v1.psv (DECIDE 165, RESOLVE 28) in Go. The oracle
   README in that directory fixes the grammar, defaults, decision order and
   outputs. It is server-side only: no client mirror exists; any future client
   must consume the same file." followed by the command
   `(cd backend && go test -race ./internal/modules/orgamend/domain -count=1)`
   in a `sh` block, with the two file names linked relative to that README.


## Retained obligations

- Bind the BR-REV state tokens and `Resolve` to the review lifecycle of
  EMP-APPR-001 once it lands (typed states, transition legality, reviewer
  authority, self-approval BR-REV-003/004, all required levels BR-REV-008);
  until then the kernel trusts the consumer to pass only legal review results.
- Period close/lock/reopen commands, the reopening record (actor, receipt
  authority, reason, audit) and the source of `Period` facts belong to
  EMP-APPR-006; the kernel only consumes them.
- Consumers (EMP-FUEL-015, expense, trip, meter-linked amendments) own the
  revision persistence and unique `(record, revision)` constraint, the row
  lock that makes the facts current, receipt-time permission evaluation
  (`fuel.fueling.amend`, `fin.period.reopen`, record permissions), one audit
  event per decision with a registered reason code (BR-AUD-001/004), the
  adjustment record and its amounts, and the recalculation jobs and their
  idempotency (BR-AMD-005, VAL-20 "no third revision").
- An amendment that moves the reporting day into another period must gate both
  periods; the kernel gates only the supplied reporting day. The consumer must
  refuse or check the target day until a reviewed rule extends the kernel.
- First approval of a never-approved record, and approvals in a locked period
  of records that were never approved, are review/period policy (EMP-APPR-001/
  006), not this kernel.
- VAL-19, VAL-20 and VAL-26 remain runtime acceptance scenarios (EMP-REL-004/
  005/007); this task proves only their pure decision steps. No persistence,
  API, app, M3 or F3 acceptance is claimed.
- The 280-code-point reason profile duplicates the unexported meter reason
  check; extracting one shared free-text reason helper is a possible later
  refactor, not done here to avoid touching another module.
