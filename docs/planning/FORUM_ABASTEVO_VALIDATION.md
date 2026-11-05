# Forum validation, performance and operations

Status: PLANNED CHECKS / NOT EXECUTED FOR FUTURE FEATURE.
Baseline and task order: [plan](FORUM_ABASTEVO_PLAN.md). Existing owning policies:
[test strategy](../backend/TEST_STRATEGY.md), [backend commands](../../backend/README.md)
and [CI cadence](CI_PLAN.md). This file defines commands/tests for implementation;
only [planning evidence](FORUM_ABASTEVO_PLANNING_EVIDENCE.md) records checks run
in the current documentation-only task.

## Command groups (run from repository root unless noted)

Commands referencing `forum` tests/packages require the named implementation
task to create them first. Verify expected nonzero test counts. A missing test,
empty `-run` match, skip or build-only pass is not acceptance. Use the existing
pinned toolchains/dependency cache; no dependency installation is part of this
planning task. Amend commands if main changes, recording the actual invocation.

**B1 — affected Go behavior and static checks:**

```sh
cd backend
go test -race -count=1 ./internal/modules/forum/...
go vet ./internal/modules/forum/...
```

Add affected account/identity/moderation/admin/official/jobs/cmd packages per
task; do not hide their failures. Check gofmt output is empty for changed Go
files. At a backend phase exit use `bash scripts/check-backend-fast.sh`, which
owns build/vet/staticcheck/sqlc/OpenAPI/secret integration; avoid manual duplicate
full gates for record-only edits.

**B2 — immediate real disposable PostGIS:** provision a unique disposable
Compose project with tmpfs PostGIS using the existing fixture ownership pattern
in `scripts/tests/test-local-backend.sh` and `.github/workflows/backend.yml`.
Never point a destructive integration fixture at staging, production or a shared
developer database. Document the unique project/container, mapped port and
cleanup ownership; set both actual fixture variables before these commands:

```sh
cd backend
# ANPFUEL_DATABASE_URL and ANPFUEL_TEST_DATABASE_URL must refer to that
# same owned disposable database, with no credentials printed in evidence.
go run ./cmd/migrate
go test -race -count=1 -tags=integration ./internal/modules/forum/...
go test -race -count=1 -tags=integration ./internal/platform/migrate/...
sqlc vet
sqlc generate
```

Run changed account/identity/moderation/admin/jobs and actual `cmd/api`/`cmd/worker`
integration cases immediately when touched. Preserve generated files in the
same atomic commit. Reuse existing test helpers; add a focused owned Forum DB
harness if needed rather than invoking the entire release suite every task.
At final affected acceptance, use the real full runner
`bash scripts/tests/test-local-backend.sh` only if the integration breadth
requires its SQL/S3/native-preparer/privacy recovery union. Backend README owns
its preparer paths and provider fixture requirements; do not omit them and claim
a skipped integration test passed. Required PostGIS unavailability is a failure.

**C — wire syntax and actual composition:**

```sh
cd backend
go test -count=1 ./internal/platform/apicontract/...
# After implementing tests with this stable nonempty FOR-specific prefix:
go test -race -count=1 -tags=integration ./cmd/api -run '^TestForum'
go test -race -count=1 -tags=integration ./cmd/worker -run '^TestForum'
```

From root:

```sh
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
```

API test must start the real composition, use actual signatures/account binding
and disposable SQL, not handler mocks alone. Test disabled gates, replay, expiry,
private/public split, bad fields and source provenance. Missing prefix tests
must be detected by inventory/count, not accepted as a green zero-test command.

**J — worker, moderation and privacy dependencies:**

```sh
cd backend
go test -race -count=1 ./internal/platform/jobs/... ./internal/modules/moderation/... ./internal/modules/account/...
go test -race -count=1 -tags=integration ./internal/platform/jobs/... ./internal/modules/moderation/... ./internal/modules/account/...
```

Also include affected privacy/identity suites when their code/restore logic
changes. No source request to a real publisher is a hermetic unit test; use a
controlled TLS fixture and explicit egress/redirect/DNS seam. Source approval
probes are separate bounded research evidence, not an always-network CI test.

**A1 — affected Android domain/application/data/ViewModel behavior:**

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest
```

Use task-specific `--tests '...Forum*'` filters for the owning module during
microtasks, include existing changed Community/navigation tests explicitly, and
verify names/counts. At phase exit run the union above, once for final unchanged
inputs. Room migration/device tests remain a separate requirement; JVM fake
cache tests cannot certify on-device SQLite upgrades.

**A2 — affected build, formatting and Android static checks:**

```sh
./gradlew :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug
bash scripts/check-mobile.sh --static-only
```

Check current Gradle formatting tasks and follow the existing owning module's
conventions; do not invent a `ktlint` target or add a dependency merely to run it.
Assemble is compilation evidence only. Preserve explicit Room migrations and
schema exports; no fallbackToDestructiveMigration.

**A3 — final native/device union (only actual run proves acceptance):**

```sh
./gradlew :app:connectedDebugAndroidTest -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.app.ui.forum.ForumNavigationDeviceTest,com.anpfuel.app.ui.forum.ForumScreenDeviceTest,com.anpfuel.app.ui.community.CommunityFeedScreenTest,com.anpfuel.app.navigation.BottomTabNavigationDeviceTest
```

The two Forum class names are a required future test location, not currently
existing tests. Add Room upgrade tests in their actual owning module. Record
device/API/locale/theme/font, exact classes/count/results and screenshots plus
manual TalkBack traversal. No attached device means DEVICE_PENDING/OWED, not
source-fake acceptance. Existing final-batch deferral still applies. Actual live
staging acceptance, device installation and VPS writes require their own scope.

**W — existing administrative UI:**

```sh
npm --prefix admin-web test
npm --prefix admin-web run typecheck
npm --prefix admin-web run check
npm --prefix admin-web run build
```

Use installed pinned dependencies. Add DOM-free client/validator tests and
browser review for source/queue/post controls, escape text, same-origin
credentials, CSRF, stale revision and no privileged data cache. No new admin app.

**D — documentation and delivery:** `git diff --check`, changed-document link/
task-ID/reference checks and changed-file secret-surface review (the general
`scripts/scan-secrets.sh` excludes Markdown, so it is insufficient by itself).
At scoped phase/final closure run `make quick-verify` once with proper tool PATH.
Reverify required remote `Quick verification` against actual PR head/base;
pending CI is recorded, never polled during construction. No final merge without
current-head/base/check guard, no bypass, no install/deploy in this plan.

## Required negative and concurrency matrix

| Area | Must demonstrate before consumer/launch | Owning tasks |
|---|---|---|
| Authorization | Absent/forged/replayed proof, wrong authority/path/query/body, duplicate proof headers, expired/revoked session, wrong key binding, disabled admin and CSRF/Origin rejection | FOR-04/05/15/17/18/21 |
| Ownership/privacy | Foreign pending item/author UUID never exposed; deletion/suspension while writer waits; safe replay after erase; export isolation; no draft in Room/log/audit; restored deleted content fenced before reads | FOR-14–18/20–22 |
| Transactions | Two DB connections + >=12 simultaneous contenders: same create intent, different keys same account, budget admission, edit CAS, delete/reply, report unique/case convergence, audit/job rollback | FOR-03/12/14/15/18/20/22 |
| Pagination/cache | Equal timestamps/IDs, >2 pages, publish while paging, late commit, source correction/hide, tampered/expired/filter-swapped cursor, no repeat, no hidden row in 304/cache, offline 24h expiry | FOR-03/05/07/09/17/24 |
| Ingestion | Stable GUID/URL, divergent collision, semantic URL queries, 304 after partial failure, timeout/429/503, invalid date/timezone/rights, hostile XML, decompression bomb, IP/redirect/DNS SSRF and cancellation | FOR-10–13 |
| Worker safety | Expired lease/restart, transaction fails after fetch, source pause, recovery after DB outage, bounded retries, queue failures; a blocked Forum fetch cannot starve privacy/evidence safety loops | FOR-12/16/18/27/28 |
| Moderation | Pending never public, report alone keeps current visibility, no permission from badges/payment, hide/restore/audit atomicity, erased/removed cannot restore, parent hides descendants, operator/reporter/author receipt privacy | FOR-04/18/19/22 |
| Bulletin | No missing/zero denominator or mixed unit/sample/source; exact overflow/tie/signed rounding; deterministic key, correction link, source revision coverage and provenance in Go/API/Kotlin | FOR-25–27 |
| Android | Card order, no-city read, four tabs unchanged, child selection, true back/scroll state, IME, duplicate taps, expired sign-in return, late HTTP/account changes, load-more/refresh/offline/error/empty, theme/font/TalkBack/locale and Room upgrade | FOR-06–09/19/23/24/27/28 |

## Proposed load scenarios — hypotheses, not measured capacity

Model active users as **daily active users** for this experiment. Assumptions:
4 feed reads +1 detail read per user/day, 10% of daily requests in the busiest
hour and a 10× short burst over that hour's mean. Thus daily reads = 5×DAU and
modeled burst RPS = `DAU * 5 * 0.10 / 3600 * 10`. These are input assumptions,
not a traffic prediction or capacity claim. Writes and ingestion are additional.

| DAU hypothesis | Read requests/day | Busiest-hour average RPS | Modeled burst RPS | Initial load concurrency cap |
|---:|---:|---:|---:|---:|
| 100 | 500 | 0.0139 | 0.139 | 2 |
| 1,000 | 5,000 | 0.139 | 1.39 | 5 |
| 10,000 | 50,000 | 1.39 | 13.9 | 20 |
| 100,000 | 500,000 | 13.9 | 139 | 100 |

At FOR-28 create a bounded local Forum load harness if none exists: explicit
owned local target, fixture seed with 10k then 100k posts/representative pending
and comments, 5-minute warm-up, 10-minute steady stage and 60-second burst;
open-loop rate plus maximum in-flight cap. Run scenarios sequentially, stop on
errors/resource limits and record drops/timeouts as failures. A capped test that
cannot deliver target RPS proves only its delivered load. Never aim this at a
publisher, staging or production without separate authorization.

Add signed writes (1% of read volume), admin review, one source job/hour and
concurrent price/map reads + safety cleanup. Record workload mix, data sizes,
Go/build SHA, Postgres plans/versions, CPU/RAM/disk quota/steal, pool size,
connection wait, p50/p95/p99, errors/drops, RSS, query count and lock wait. No
hardware or VPS capacity is guessed from user count.

Initial acceptance targets to freeze in FOR-01: p95 public read <=300ms on the
declared local fixture, no 5xx/data loss at achieved target, no growing queue or
safety delay, <=3 SQL round trips per feed/detail excluding transport health,
feed <=64 KiB and detail <=32 KiB. Compare price/map p95 against a baseline on
the same host; >10% regression triggers investigation, not adjusted thresholds.
These are desired gates, not results or universal SLO guarantees.

Expected pressure points: FTS/filter indices, per-post comment lock, shared feed
epoch lock during editorial bursts, DB pool contention with existing jobs,
unreviewed candidate growth and human moderation throughput. Trigger a measured
optimization when p95 exceeds target in three consecutive steady intervals,
query plan scans >10× delivered rows, sustained pool wait >50ms, feed-state lock
p95 >20ms, oldest review exceeds 24h or storage reaches the configured high
watermark. Test a composite/partial index, narrower projection/query or lower
collection/admission budget first. Do not introduce Redis/Kafka/microservices
on hypothetical 100k-DAU demand. Operator coverage is assessed separately from
request throughput; a fast API is not an adequately moderated Forum.

## Operational runbook requirements

- Feature flags separate entry/reads, social writes, collection and automatic
  publication; default off. Health/readiness remains independent of source
  availability. Start rollout with original editorial data and approved admins.
- Log only request/job IDs, typed outcomes, durations and aggregate counts.
  Never text/session/signature/email/account UUID/precise GPS/source response.
  Source code and coarse category labels are bounded; no per-user metric labels.
- Metrics: Forum request/error/latency, query/pool/lock wait, job lease/retry/age,
  last successful source fetch, rejected/deduped candidates, pending-review age,
  privacy cleanup backlog, bytes/rows/cache eviction and price/map regression.
- Alert on three failed source runs or >6h freshness debt (operational target,
  not publisher promise), review backlog >24h, cleanup failure, and any privacy
  invariant violation. Source outage stops only that source; never falsify dates
  or replace source material with invented fixtures.
- Existing admin UI offers source pause, draft preview/review, target hide,
  reason/audit and case lookup. Existing ops dispatch supports Forum targets.
  A switch rollback keeps erasure/moderation operating; no destructive schema
  rollback. Recovery after backup restore replays account deletion before reads.
- Release gates: code/tests → SOURCE_READY; actual external rights/source config
  → NEWS_ENABLED; operator/privacy/terms → WRITES_ELIGIBLE; actual Android
  native/manual run → DEVICE_ACCEPTED; protected merge → INTEGRATED. Existing
  G09 production certification remains separate. Do not invent a new release
  gate name that silently replaces the repository's canonical state machine.

## Evidence template for later evaluation

For each task record ID/B-BR/BUC, source and base SHAs, owned files, pre-change
failure, exact commands, nonzero counts/duration/results, disposable fixture
identity/cleanup, concurrency/failure cases, screenshots when applicable,
remaining obligations, flags and next task. Phase evidence declares LOCAL_DONE
and PASS only for its actually verified scope. Final report must list failed,
skipped, blocked and unexecuted cases explicitly and link the actual PR/commits.
