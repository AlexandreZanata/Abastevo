# Forum final scoped construction closure — FOR-28 checkpoint

Status: **IN_PROGRESS / LOAD_MATRIX_PASS / DEVICE_PENDING**.
Validation: PASS for the full declared 5m/10m/60s load matrix and executed
failure/recovery suites; the native/device union and remaining measurement gaps
are explicitly OWED.

Baseline: maintained `dev` `0eebefa6` (FOR-27). GitHub `main` `99f21830`.
Owned change: bounded Forum load harness and this evidence. No production,
staging, publisher, VPS or device write is performed.

## Load harness

`backend/internal/modules/forum/adapters/http/forum_load_test.go` (build tags
`load && integration`) creates a disposable PostGIS database, applies all
migrations, seeds published/pending posts and comments, starts the real HTTP
router with the real Store/Service, then runs an open-loop HTTP workload with a
maximum in-flight cap. Workload mix is 4 feed reads (plain/category/UF) to 1
detail read; body bytes, latency, errors, drops, pool stats, aggregate SQL
queries, and process RSS are recorded.

Command shape:

```sh
ANPFUEL_TEST_DATABASE_URL=... FORUM_LOAD_FULL=1 FORUM_LOAD_POSTS=100000 \
FORUM_LOAD_RPS=13.9 FORUM_LOAD_BURST_RPS=139 FORUM_LOAD_MAX_INFLIGHT=20 \
go test -count=1 -timeout 25m -tags='integration load' \
  ./internal/modules/forum/adapters/http \
  -run '^TestForumLoad_BoundedScenario$' -v
```

`FORUM_LOAD_FULL=1` selects the declared 5-minute warm-up, 10-minute steady and
60-second burst. The matrix below was executed with that full mode; the earlier
short pilots are superseded and not used as acceptance evidence.

## Environment and data

- Host: 13th Gen Intel Core i7-13620H, 16 online CPUs, 31.05 GiB container memory
  limit, host `/tmp` filesystem 93% used (37.3 GB free). Docker container
  `abastevo-forum-for24-pg`.
- Go toolchain `go1.27.3`; PostgreSQL `18.6 (Debian 18.6-1.pgdg13+2)`, PostGIS
  image `postgis/postgis:18-3.6`; pool max 10 connections.
- Matrix fixture: 100,000 posts (90% published / 10% pending) + 20,000 comments.
- Earlier pilot fixture B: 10,000 posts + 20,000 comments (not used as the full
  matrix, retained only as harness smoke evidence).
- The DB and comments are dropped with the disposable database; no fixture is
  committed or written to staging/production.
- Post-load Docker sample: CPU 0.02%, memory 264.5 MiB / 31.05 GiB, network
  83 MB / 50.7 MB, block 1.72 GB / 1.53 GB. This is a single idle sample, not a
  resource-budget certification.

## Full declared load matrix

All scenarios used the 100,000-post / 20,000-comment fixture, full 5-minute
warm-up, 10-minute steady and 60-second burst, sequential runs. Targets are the
validation table's DAU hypotheses (not traffic predictions). Every row had zero
HTTP errors and zero drops. Feed/detail byte gates and p95 <=300 ms passed in
every phase.

| DAU | Target steady/burst RPS | Max in flight | Delivered steady | Delivered burst | p95 steady/burst | p99 steady/burst | Requests steady/burst | RSS KiB | SQL queries |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 100 | 0.0139 / 0.139 | 2 | 9 in 600s (0.015) | 9 in 60s (0.15) | 24.30 / 8.04 ms | 24.30 / 8.04 ms | 9 / 9 | 14,228 | 44 |
| 1,000 | 0.139 / 1.39 | 2 | 84 in 600s (0.14) | 84 in 60s (1.4) | 15.23 / 12.89 ms | 31.40 / 21.64 ms | 84 / 84 | 15,712 | 381 |
| 10,000 | 1.39 / 13.9 | 5 | 834 in 600s (1.39) | 834 in 60s (13.9) | 10.64 / 10.77 ms | 11.78 / 12.02 ms | 834 / 834 | 21,696 | 3,756 |
| 100,000 | 13.9 / 139 | 20 (declared cap 100) | 8,340 in 600s (13.9) | 8,336 in 60s (138.9) | 14.66 / 12.62 ms | 36.80 / 21.00 ms | 8,340 / 8,336 | 21,880 | 37,524 |

Max feed body was 10.0 KiB and max detail 0.665 KiB across the matrix; both are
below the 64/32 KiB gates. Aggregate SQL/request remained ≈1.8, matching the 4:1
feed/detail mix (feed 2 queries, detail 1). Pool empty-acquire counts were 1–3,
canceled 0, and new connections 1–3; total acquire duration stayed in the
3.2–61.4 ms range for the run (≈0.9–2 µs/acquire). The 100-DAU row has only nine
steady samples, so its p95 is descriptive, not statistically strong.

### Exploratory headroom beyond the matrix

A separate 139 RPS steady / 1,390 RPS burst run (10× the declared 100,000-DAU
burst, max 100 in flight) delivered 83,400 steady requests in 600s (139.0 RPS)
and 73,680 burst requests in 60s (1,228 RPS, 88.3% of the 1,390 target), with
zero errors/drops, p95 11.11/13.27 ms, p99 13.10/15.81 ms, RSS 29,508 KiB and
357,782 SQL queries. This is exploratory headroom, not an accepted 1,000,000-DAU
scenario; the burst target was not fully delivered by the single-client ticker.

## Failures and recovery executed

- Disabled/rollback switches: existing Forum HTTP tests cover reads, feed,
  detail and admin disabled 404/503 paths (full Forum unit suite PASS).
- DB outage: new `TestHandler_Integration_DatabaseOutageReturns503` PASS
  (0.61s): after closing the real pool, feed and detail return 503
  `forum.unavailable`, no panic.
- Source failure/rights/304/dedup: `TestForumSourceFetch_*` 5/5 PASS (1.01s);
  real-PostGIS source jobs lifecycle/feed/coexistence 3/3 PASS (3.12s),
  including the blocked-fetch coexistence check.
- Privacy recovery: `TestForumErasure_PostgresIntegration_Gates` 6/6 PASS
  (0.64s); `TestFOR22_TopicPrivacyAndModerationRecovery_Integration` 6/6 PASS
  (0.81s); Android ForumRepository offline/cache tests PASS.
- Price/map regression: Official unit suite PASS; Official real-PostGIS suite
  PASS (read 11.37s). Full Android JVM union remains green (2,851 tests, 0
  failures, 4 pre-existing skips). No price/map source changed in FOR-28. A
  same-host comparative p95 baseline was not rerun; that comparison is OWED.
- A2 current-head check: `:app:assembleDebug`, `:app:assembleDebugAndroidTest`,
  `:app:lintDebug` (0 errors; 364 warnings, 2 information) and
  `scripts/check-mobile.sh --static-only` PASS.

## Device and unresolved capacity

- `adb devices` returned no attached device. `ForumNavigationDeviceTest` and
  `ForumScreenDeviceTest` do not yet exist; `BottomTabNavigationDeviceTest`
  exists but cannot run. A3 native/theme/font/TalkBack/back-stack/IME/manual
  traversal is **DEVICE_PENDING / OWED**, not accepted.
- The full declared 5m/10m/60s matrix is now executed. The 1,390 RPS exploratory
  burst was not fully delivered (1,228 RPS); no 1,000,000-DAU acceptance is
  claimed.
- No DB lock-wait percentile or CPU/steal time series was collected; only pool
  stats and an idle Docker sample. Per-endpoint SQL round trips were not
  separately traced; aggregate average is recorded. These are unresolved
  measurement gaps, not zero values.
- No staging, VPS, operator, legal, source activation, deployment, APK
  installation or merge is claimed.

## Next action

Run the A3 device union when a device is available and close the remaining
measurement gaps; then FOR-29 may prepare the authorized protected dev→main PR
with these OWED items retained.

## PR #192 validation review (2026-11-08)

- Remote gate failure on `a8c5d840`: gofmt listed 13 forum files; once
  formatted, staticcheck reported an unused `DiscussionHandler.now`, an unused
  `mustJSON` and an S1016 struct conversion. Fixed in `d3ba2112`.
- `222662dd`: production never configured the forum quota checker
  (`SetQuotaChecker` was test-only) and one shared limiter could not express
  both B-BR-FOR-11 windows. The store now takes per-window checkers; the API
  wires comments 20/hour and 100/day, topics 3/hour and 10/day.
  `TestTopic_DailyQuotaIndependentOfHourly` proves a tight daily window is not
  masked by a generous hourly one. Public post detail used the unrestricted
  lookup; it now uses `GetPublicPostByID` through `PublicPosts`, and comment
  and reply listings require a publicly visible post
  (`TestForumPublicDetail_HonorsSourceRights`).
- `a3103270`: Android comment edit/remove sent `sequence` as
  `expected_revision` (409 on every comment but the first, and after any
  edit). The mapper now reads `revision`; retries of an unchanged comment reuse
  their operation id; `CancellationException` is rethrown in forum ViewModels;
  topic removal pops from a `LaunchedEffect`.
- Retained OWED review items: admin publish takes no expected revision (a
  pending topic edited after review can be published unseen); report `detail`
  text survives account erasure; `forum-moderation-applied` jobs have no worker
  handler; comments have no approval route (the Android social UI stays behind
  its kill switch); comment/topic controls show without ownership hints; the
  offline cache drops bulletin provenance.

## Retained review items closed (2026-11-08)

- `00642c0f`: admin publish requires `expected_revision >= 1` and refuses a
  mismatch with 409 (`TestAdminService_*` stale-revision case, admin HTTP
  400/409 cases, FOR-09/FOR-13 integration).
- `7f06e626` + `66d6e6cd`: account erasure scrubs the reporter's
  `forum_reports.detail`. Moderation case history is immutable
  (`TestNoDestructiveSQLPaths`), so forum report cases now carry a fixed
  PII-free detail like quality reports instead of copying reporter text
  (`TestForumErasure_ScrubsReportDetail`).
- `408a6d5f`: `forum-moderation-applied` has a worker consumer that settles the
  case's RECEIVED reports (INVALIDATE/RESOLVE -> RESOLVED, DISMISS ->
  DISMISSED, REVIEW no-op); replays update nothing
  (`TestModerationApplied_*`, `TestForumReports_SettleByCaseIsIdempotent`).
- `091196be` + `7b8ba4af`: `GET /v1/admin/forum/comments` (oldest first, keyset
  cursor, `forum_comments_review_queue_idx` from migration 000163) and
  `POST /v1/admin/forum/comments/{id}/publish|reject` with the reviewed
  revision as audited mutations; the operator view omits the author account
  id and the audit omits the text. Rejection stores REJECTED+HIDDEN; the
  previous REMOVED-with-body query violated the comment body check on every
  call. The admin web lists and decides the queue, and its post filter now
  sends `publication_state` (it sent `status=draft`, which the API ignored).
  `TestCommentReview_AdminQueuePublishAndReject` covers pagination, 400/409/404,
  publish-to-public, reject-to-hidden and change records.
- Still OWED: Android social UI activation (kill switch), ownership hints and
  offline bulletin provenance on Android, A3 device acceptance.
