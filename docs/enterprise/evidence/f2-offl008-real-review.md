# EMP-OFFL-008 — Actual held review and durable author outcomes

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: LOCAL_PASS (see closure below)
Integration: INTEGRATION_PENDING

Sole bounded implementation from clean tested/pushed71b995ae. Full FLEET-002
is LOCAL_DONE; full METER-002 stays open for assignment/history/review. Eleven
full checkpoints/53 audited open nodes. User authorizes full F2 then F3,
recommended routine decisions recorded without questions and owned API36 emulator.
No merge/deployment/real receipt retention or money movement is authorized.

B-BR-EMP-160..169, BUC-EMP-83..88 and EDR-110..119 precede their behavior. Read current
FAST_EXECUTION/PROGRESS, exact ep04 task, offline§5–8, permission§4–5 and review/
validation rules. Actual196 ACK/effect guards,198 meter source and current
protected V1/V2 frame confirm separate resolutions are required; no fake
consumer or original-source impersonation. Shared ports/wire shapes freeze
before dependent edits. Backend/source integration precedes native binding.

Owners: root application/reviewer authority/HTTP/OpenAPI/docs/composition;
persistence owner migration199/sqlc/review Store/actual reviewed Meter insert;
intake owner historical disposition/atomic proof/protocol/server delivery;
client owner shared resolution/native protected projection/status and actual
emulator acceptance. Root coordinates SQL generation and DB/Gradle/ADB leases.
First owner is actual Meter with unchanged accept or reasoned refusal; unknown
record types remain unavailable until their owning producers integrate.

Original ACK stays byte-identical. One current reviewer decision is immutable
and audited; held/expired/refused never becomes readiness or costs. Actual
canonical employee history prevents self-review after rehire/merge. Minimal
own-result permission restores no private record authority. Versioned expiry
is separate from intake settings; notification/escalation delivery remains its
real owner. Source/legal/provider/release gates are not inferred. No PASS is
claimed at opening.

EDR-117 precedes DDL behavior: preserve schema196 legacy Review ACKs unchanged
with nullable UNPROVEN proof version, add a validated composite FK that skips
only that historical NULL, and force every new REVIEW to version1/real item/
typed owner fact. Default SQL validator knows Meter only; generic validation
fixtures may add an explicit named owner branch querying their existing actual
held table, preserving the production Meter guard. No fabricated backfill or
NOT VALID acceptance. Pure resolution wrappers may use an internal explicit
128KiB review parser entry while the public frozen V1 parser stays64KiB; shared
operation/ACK/batch bytes and limits remain unchanged.

## Handoff to Claude Code — execution stopped

The maintainer requested this handoff instead of continuing execution. All
three agents stopped; no agent-owned Go/Gradle test process or DB lease remains
active. Their saved files are retained. No commit, push, merge, issue closure
or deployment was performed for the current OFFL-008 changes. This source
checkpoint is **not GREEN and not LOCAL_DONE**.

Start in the existing worktree:
`/home/iiii/PESSOAL-PROJETOS-ALEXANDRE/brazil-fuel-prices/.worktrees/enterprise-suite-execution`.
Branch `codex/enterprise-f2`, HEAD/remote backup
`71b995aedb7570477d8d29233ab514c843b3018d`. Preserve modified **and untracked**
files; do not stash/reset/clean them or overwrite the other occupied worktrees.
Current PROGRESS and the F2/F3 evidence ledgers are the task authority.

GitHub PR203 was read directly at handoff: OPEN, non-draft, `dev`→`main`, head
`6131d0a33520e0edecd8da4f8fc90f617b57e0a1`, base
`6a19bcc799f958adbf9252e956ec6f66ec2bca5a`.
<https://github.com/AlexandreZanata/Abastevo/pull/203>
Remote: `git@github.com:AlexandreZanata/Abastevo.git`.
There are12 F2 commits on this branch absent from dev and4 newer dev commits
absent here, common ancestor `fac002e7d507a230d4947a3ed563269ad48f8c8e`.
Those dev commits concern the official origin/portal and must be preserved.
Fetch/reconcile normally after preserving and completing this owned WIP;
do not overwrite dev or assume F2 is already in PR203. Re-read remote state.

### Exact unfinished work and known failures

1. **Android is deliberately RED, not implemented.** Current three-file WIP:
   `domain/.../sync/EnterpriseQueue.kt` adds `resolution`/`reviewRetry` defaults;
   `EnterpriseSyncStatus.kt` adds sanitized resolution with a throwing factory
   stub; new `EnterpriseReviewStatusTest.kt` has3 intentional failures,0 errors/
   skips (`/tmp/enterprise-offl008-status-red.log`,0.300s). Pure protocol/domain
   1346 PASS applies to the earlier codec checkpoint, not this changed head.
   ProtectedQueueCodec is still V2. Implement protected V3/V1-V2 recovery,
   resolution/original-ACK CAS, separate review retries, full-response preflight,
   review-only polling with no pending delivery, real DI/Worker, presentation,
   outcome-specific dismissal,8 locales and actual Go HTTPS/API36 native tests.
   EDR120/121 and B-BR166/167 record the chosen ports/semantics before behavior.
2. **Two generic intake fixture tests currently fail SQL42501:**
   `TestSyncIntakeAtomicAcceptanceAndExactReplay` and
   `TestSyncIntakeDependencyClaimsReviewAndMissingProducer`. Actual diagnosis:
   migration199's effect guard reads `org_meter_readings`; the named validation
   fixture lacks SELECT on that source table. Fix the fixture's actual required
   read grant, preserve real item/held proof and the production Meter guard,
   then rerun these and the affected source union. Do not weaken proof/tests.
   Diagnostic `/tmp/enterprise-offl008-generic-fixture-code-debug.log`.
3. **Known DST bug remains unfixed:** both expiry expressions in
   `backend/db/queries/fleet/meter_reviews.sql` use `make_interval(days=>...)`,
   which depends on session timezone across DST. EDR119/B-BR168 approve elapsed
   60×86400seconds, matching Go/API, independent of session timezone. Capture
   real non-UTC/DST SQL RED, correct the shared rule/query, regenerate sqlc,
   verify exact as-of deadline/late-resolution behavior. No fake receipt age or
   new runtime clock/settings. The DST RED/GREEN has **not** run.
4. **Populated198→199 recovery and further SQL constraints/expiry remain
   unexecuted.** Drafts only, not accepted source:
   `/tmp/enterprise-offl008-upgrade-test.go` and
   `/tmp/enterprise-offl008-review-constraints-test.go`. Review them before use;
   they were not copied/compiled/run. Preserve actual old ACK bytes with NULL
   legacy proof and require actual version1/typed proof for new review ACKs.
5. Review `meterIntakeHandler.Confirm`'s REVIEW proof error classification:
   its final `CheckHeldTx` error is currently returned directly. Determine with
   a meaningful last-check failure test whether it needs `SourceDenied` to
   preserve fatal whole-response withholding. Store pre-ACK/replay proof paths
   already wrap fatal errors. This is an **unverified concern**, not a proven
   defect or a completed correction.
6. Remaining immediate critical acceptance includes actual proof/freshness
   waits, canonical employee self-review after merge/rehire as applicable,
   complete affected PostGIS union, static/vet/sqlc reproduction, Android host/
   lint/build/native and final secret/diff/plan checks. Current-head full CI is
   not certified. Do not substitute old manifests/earlier PASS for new inputs.

The final real intake corridor passed the Meter lost-right→held item/source/
ACK path, minimal original receipt replay without business read, refusal of a
review UUID without held source, and capability lock convergence. The cached
undeclared-review negative also passed. The two generic failures above remain.
Final intake source20-file manifest:
`/tmp/enterprise-offl008-A-handoff-source.sha256`, manifest SHA256
`ffbb545991a10b5cc95c59e70266089a35414fb8a657bae356f75d5c3b72a987`.
Pure protocol12-file manifest:
`/tmp/enterprise-offl008-protocol-source.sha256`, SHA256
`f898b509f7e817a10721c9595b09f15b67c2c249bf3c2cce3af5479eaa4a231c`.
The older HTTP5-file manifest predates added null-body/TLS tests and is not a
final current-head pin. The scoped construction results below remain useful
diagnostics, not a complete task or release certificate.

### Environment and next execution

- Go/gofmt1.27.4:
  `/data/dev/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.4.linux-amd64/bin`;
  use `GOCACHE=/tmp/enterprise-go-build`. Staticcheck0.8.1 is
  `/home/iiii/go/bin/staticcheck`, using the installed Go1.27.1 toolchain and
  a task-owned writable `STATICCHECK_CACHE` in `/tmp`.
- Owned PostGIS Compose project `emp-f2-20261113`,
  `infra/compose.validation.yml`, local control database port32775. Existing
  synthetic fixture connection:
  `ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:32775/anpfuel?sslmode=disable`.
  Use orgtest's isolated child DBs/ordinary runtime roles. Preserve the control
  DB, other Compose projects and all unrelated server/user data.
- JDK17 `/home/iiii/.gradle/jdks/eclipse_adoptium-17-amd64-linux.2`;
  SDK `/data/dev/android/sdk/Sdk`; CoMaps
  `$WORKTREE/build/comaps/7ad828883c4aca8a`. Existing Gradle invocation uses
  `--offline --no-daemon --no-parallel --max-workers=2 --console=plain`.
- Only the owned emulator `emulator-5588` / `enterprise-f2-api36` is authorized
  for these native checks. Verify ownership in
  `/tmp/emp-f2-avds-20261113/task-owner.json`; preserve apps/data. No phone.
  A synthetic PIN exists in the private task file; never print it or inject it
  unless the actual lockscreen PIN entry is verified. No active test HTTPS
  fixture/reverse was handed over. Verify resources before use.
- Read AGENTS, FAST_EXECUTION, current PROGRESS, DELIVERY_WORKFLOW, selected
  task, TEST_STRATEGY and backend README. Plan verifier:
  `python3 docs/enterprise/tools/check_plan.py` (last run:348tasks/545rules/
  167use cases/145screens,0problems). Finalize OFFL-008 first. Eleven full tasks
  are LOCAL_DONE/INTEGRATION_PENDING;53 audited nodes/full F2/M1/M2/C1 and F3
  remain open. Full METER-002 is not done;71b995ae contains its source foundation
  and full Fleet002 closure. Use `f2-execution.md`/`f3-authorized-sequence.md`
  for the actual remaining dependency order, not a claim of full phases.

The newest user request authorizes Claude to finish and repair the current
task, review **all** cumulative PR work, consolidate protected main, reconcile
issues and deploy the new version to the VPS. Choose recommended routine
decisions, document them and do not ask questions. Preserve unrelated dev/main
work. Consolidate via maintained dev→main PR203, never force/main-development
push/admin bypass. Revalidate the real head/base and every required check,
explicitly `Quick verification`; use the guarded finish helper with the actual
PR/base/check. Close completed issues through the merged PR; administrative
closure of incomplete related trackers is `not planned` with linked retained
obligations (ADR020), never false task/release acceptance. Do not close unrelated
repository issues blindly.

For VPS publication, discover the actual configured deployment path and read
`docs/operator/deploy.md`, current ADR019/Android-VPS evidence and staging CD
runbooks. A deployment plan is not proof the mechanism is active. The recorded
test target is `https://teste.abastevo.com.br`; origin/portal changes have since
advanced on dev and must be reconciled with actual configuration. Use merged
tested immutable SHA/images, real backup/recovery/migration compatibility,
health and smoke checks, ordinary runtime grants and existing deploy agent/
Compose/Kubernetes mechanism as actually installed. Preserve other VPS apps/
namespaces and private credentials; no env/token dumps. Verify rollback if
needed and report deployed SHA, actual health/smoke and retained obligations.
Real legal/provider/receipt/production release gates are not fabricated by
merge or staging deployment.

## Backend construction checkpoint — not task acceptance

The typed source, current reviewer, immutable separate resolution and minimal
author result read now exist. Whole OFFL-008 remains IN_PROGRESS/PENDING until
the actual intake/history integration, remaining recovery checks and protected
Android V3/native acceptance finish. No additional full task is counted done.

- Registry/atomic-review proof has actual RED→GREEN (22 top-level/38 named
  application and adapter cases). Descriptor proof is captured once; any
  missing/unknown owner proof stays closed, including asynchronous reviews.
- Temporal ORG history has actual source RED→GREEN (19 named cases), including
  retained removed/regranted periods, never-effective future grants and
  unresolved SELF. EDR-118 records why the device clock cannot prove absence.
  Handler decoding remains the frozen six fields; its pure RED→GREEN covers
  13 top-level/52 named cases with no APPLY fallback or fake held reference.
- Ordinary PostGIS schema199 RED→GREEN protects three forced-RLS immutable
  tables and validated deferred source/ACK references. Actual Store tests cover
  original receipt/nanosecond/author/enrolment preservation, one immutable
  decision, twelve-way exact retries, twelve competing commands from two real
  reviewers, late commit rollback and whole-body private-selector withholding.
  Personal erasure preserves held evidence and permits a different real
  reviewer without pretending the original source is still live.
- Root application service RED3 top-level cases→GREEN now33 named application/
  company HTTP cases. Page suffixes, original decision intent, delivered owner
  forms and ended request proof are checked before publishing. A peer found an
  unset receipt; genuine focused RED0.003s precedes its nonzero receipt guard.
  Receipt minimum stays distinct from the valid minimum declared observation.
- Actual current company authority RED0.752s→GREEN race1.895s covers pending
  setup write refusal, ACTIVE fresh decision, wrong session/member and disabled
  capability write refusal with retained private reads. No arbitrary policy is
  admitted to the existing company guard.
- Actual runtime company review HTTP passes race2.101s: list/read, exact
  nanosecond fact, normal cookie/CSRF, decision/retry, changed-intent and stale
  decisions, hidden unknown records, unchanged original REVIEW ACK and one
  applied effect. An initial test expectation used the wrong spelling of the
  existing OPERATION_ID_REUSED wire code; corrected fixture expectation only.
- The independent cookie matrix now covers150 current role/permission cells
  (55 allowed/95 denied) in10 ordinary roles, passing10.410s. The route inventory
  retains all85 permission obligations:15 normal routed keys/82 normal routes,
  one signed operation key,69 future keys and19 specialized boundaries. Station
  and real assignment obligations remain unavailable, never inferred.
- Actual signed TLS own-results acceptance passes2 top-level PostGIS/race
  cases3.012s: accepted/refused/unresolved, exact correlation/original bytes,
  minimal read after business role loss/capability disable, foreign suffix/
  digest/enrolment withholding, cookie/tampered/expired proof and revoked device.
  These typed held fixtures prove delivery, not the pending historical intake
  classification path. The public V1 signed intake handler remains unchanged.
- Pure Go protocol passes447 named race cases; Kotlin enterprise domain passes
  1346 cases with no failures/errors/skips. Shared137 new review vectors extend
  the retained260 operation/ACK vectors; V1 remains64KiB and only the explicit
  review response parser permits128KiB. Request/response/resolution bounds are
  64KiB/128KiB/4KiB, at most50 distinct selectors and280 Unicode scalar reasons.
- Root OpenAPI schema RED8 negative cases→GREEN23 named cases and the actual
  full production router gate pass. A misplaced path insertion was caught and
  corrected before publication; all four real paths now have a regression.
  Impossible outcome/state/revision/null combinations and meter bounds are
  rejected. The new specialized gate additionally checks actual signed proof
  descriptors; existing null-body validation already refuses malformed drift.

Current diagnostics are retained under `/tmp/enterprise-offl008-*`; final
source manifests and affected revalidation will supersede these construction
checkpoints. Early test-compilation mistakes and fixture-only corrections are
not counted as behavioral RED evidence. EDR-119 precedes the newly discovered
non-UTC/DST expiry correction; upgrade/recovery, full source intake, remaining
critical checks and protected native acceptance are still required.

## Closure by Claude Code — 2026-11-14

EMP-OFFL-008 is **LOCAL_DONE / INTEGRATION_PENDING**. Full METER-002 remains
IN_PROGRESS (assignment/history); F2/M1/M2/C1 and F3 are not accepted here.

Backend (all handoff items closed, genuine RED→GREEN, logs `/tmp/enterprise-offl008-*`):

- SQL42501: the generic validation fixture receives only
  `SELECT(org_id,id,operation_id,review_resolution_id) ON org_meter_readings`
  in `syncFixture`; production grants and the Meter guard are unchanged. The
  fabricated-readiness negative now requires the real guard's 23514.
- DST expiry: both expressions use `make_interval(secs=>expiry_days*86400)`
  (EDR-119/B-BR-EMP-168). RED under a role-wide non-UTC POSIX DST session showed
  a one-hour early deadline; GREEN proves the exact deadline, open at
  deadline−1µs and expired at the deadline. A real 60-day-late decision cannot
  be exercised without fabricating receipt age; no decision path reads expiry.
- Populated 198→199 failure/recovery: legacy REVIEW ACK bytes preserved with
  NULL (UNPROVEN) proof, validated deferrable MATCH SIMPLE FK, no backfill/NOT
  VALID, promotion refused (55000), fabricated item refused (23503), new typed
  review proof version 1. Mutation checks caught NOT VALID, missing forced
  version and missing held-source check.
- `meterIntakeHandler.Confirm` final `CheckHeldTx` failure was a real defect
  (per-operation RETRY instead of whole-response withholding); now wrapped as
  `SourceDenied`, preserving existing SourceDenied errors.
- Canonical self-review refusal across employee merge (SelfApproval, P9006),
  proof freshness re-evaluated after lock waits and authority change cannot
  interleave a decision (pg_stat_activity barriers, no sleeps).
- Union regressions fixed: unit archiving (P7704) by allowlisting the held
  reading unit FK in 199's `org_unit_current_usage` (one-line diff from 197);
  sync route guard; signed TLS Meter matrix now expects held REVIEW for
  formerly authorized roles plus never-authorized FINANCE 403 and SELF-only 503;
  TRUNCATE…CASCADE fixtures (one pre-existing HEAD failure fixed).
- SQL reviewer fixes: held reading identity reservation both ways; accepted
  resolution requires its readiness effect in the same transaction; decision
  time bounded by receipt and `clock_timestamp()`.
- Results: affected PostGIS/race union 6630 pass / 5 fail — 4 orgmedia S3 tests
  need an `ANPFUEL_TEST_STORAGE_*` endpoint (same failure at HEAD) and 1
  onboarding test fixed after start and re-run green; focused review/intake
  race ×3 249/0; untagged `go test ./...` 190 packages ok; gofmt/build/vet,
  staticcheck (affected), sqlc reproduction, vacuum OpenAPI lint,
  `check_plan.py` (348 tasks/545 rules, 0 problems), diff and secret scans pass.
  Manifest `/tmp/enterprise-offl008-backend-final.sha256` (79 files).

Android/client (EDR-120/121, B-BR-EMP-166/167 refined before behaviour):

- Protected queue codec V3 (written only when a resolution/review retry
  exists; V1/V2 still decode), resolution and review-retry compare-and-set,
  full-response preflight, review-only polling with no pending delivery, real
  Hilt bindings and Worker, truthful held/accepted/refused/unresolved
  presentation with outcome-digest-bound dismissal, 9 strings × 8 locales.
- Host RED→GREEN (status 3/3, review service/flow/queue/http/runner suites),
  Compose RED 4/4 → GREEN 10/10; reviewer-found starvation, Retry-After floor
  and manual-check masking defects fixed after RED.
- Domain 2293 (1351 enterprise), application 632, data enterprise 146, app
  enterprise 14 tests, 0 failures; `:app:assembleDebug`, lint 0 errors.
- Native API36 `emulator-5588` against a real Go HTTPS fixture
  (`orgsync_review_native_fixture_test.go`, tags `integration nativefixture`):
  held reviews seeded → force-stop → recovery after process death and a lost
  answer, B refused / A accepted, original ACK unchanged, 0 operations resent;
  V2→V3 recovery, sync context and normal Hilt worker checks pass. Native tests
  are acceptance (no native RED). Manifest
  `/tmp/enterprise-offl008-android-final.sha256` (36 files).

Retained obligations (not accepted here): orgmedia S3 tests need a storage
endpoint; signed review-results read budget/429 contract (EDR pending); review
held on a later-closed segment (no segment close exists yet); one malformed
operation from a formerly authorized member withholds the batch; PENDING_SETUP
review decision maps to 503; per-item authority locks in review lists (cost);
lenient SQL JSON check on stored canonical resolutions (Go re-validates);
client confirmation of SELF-only 503 (EDR-118); no per-run cap on review
lookups and the runner ignores `retryAfterMillis` for reviews (EDR-120);
full rehire-with-new-account end-to-end test not run.
