# Forum final scoped construction closure — FOR-28 checkpoint

Status: **IN_PROGRESS / LOAD_PILOT / DEVICE_PENDING**.
Validation: PASS for the executed bounded pilots and failure/recovery suites;
full declared load durations, remaining DAU scenarios and the native/device
union are explicitly OWED.

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
ANPFUEL_TEST_DATABASE_URL=... \
FORUM_LOAD_POSTS=100000 FORUM_LOAD_RPS=13.9 FORUM_LOAD_BURST_RPS=139 \
FORUM_LOAD_MAX_INFLIGHT=20 FORUM_LOAD_WARMUP=20s FORUM_LOAD_STEADY=60s \
FORUM_LOAD_BURST=15s \
go test -count=1 -tags='integration load' ./internal/modules/forum/adapters/http \
  -run '^TestForumLoad_BoundedScenario$' -v
```

`FORUM_LOAD_FULL=1` selects the declared 5-minute warm-up, 10-minute steady and
60-second burst. The two runs below are **pilots**, not the full duration matrix.

## Environment and data

- Host: 13th Gen Intel Core i7-13620H, 16 online CPUs, 31.05 GiB container memory
  limit, host `/tmp` filesystem 93% used (37.3 GB free). Docker container
  `abastevo-forum-for24-pg`.
- Go toolchain `go1.27.3`; PostgreSQL `18.6 (Debian 18.6-1.pgdg13+2)`, PostGIS
  image `postgis/postgis:18-3.6`; pool max 10 connections.
- Fixture A: 100,000 posts (90% published / 10% pending) + 20,000 comments.
- Fixture B: 10,000 posts (90% published / 10% pending) + 20,000 comments
  (10,000 published posts selected for comments).
- The DB and comments are dropped with the disposable database; no fixture is
  committed or written to staging/production.
- Post-load Docker sample: CPU 0.02%, memory 264.5 MiB / 31.05 GiB, network
  83 MB / 50.7 MB, block 1.72 GB / 1.53 GB. This is a single idle sample, not a
  resource-budget certification.

## Measured pilots

Both runs: 13.9 RPS steady (10,000-DAU model hypothesis), 139 RPS burst,
maximum 20 in flight, zero HTTP errors and zero drops.

### Fixture A — 100,000 posts

| Phase | Requests | p50 | p95 | p99 | Max feed | Max detail |
|---|---:|---:|---:|---:|---:|---:|
| Warm-up 20s | 278 | 1.212 ms | 11.858 ms | 13.208 ms | 10.0 KiB | 0.665 KiB |
| Steady 60s | 834 | 1.192 ms | 11.769 ms | 12.725 ms | 10.0 KiB | 0.665 KiB |
| Burst 15s @139 RPS | 2,085 | 1.067 ms | 11.883 ms | 12.785 ms | 10.0 KiB | 0.665 KiB |

Aggregate SQL queries: 5,757 over 3,197 requests (≈1.8/request, consistent with
feed 2 + detail 1 at the 4:1 mix). Pool acquires: 5,761; total acquire duration
11.61 ms; empty acquires 2; canceled 0; new connections 2. Process RSS
16,788 KiB → 21,948 KiB.

### Fixture B — 10,000 posts

| Phase | Requests | p50 | p95 | p99 | Max feed | Max detail |
|---|---:|---:|---:|---:|---:|---:|
| Warm-up 15s | 209 | 6.689 ms | 7.968 ms | 9.451 ms | 9.95 KiB | 0.658 KiB |
| Steady 45s | 626 | 6.431 ms | 7.512 ms | 8.195 ms | 9.95 KiB | 0.659 KiB |
| Burst 15s @139 RPS | 2,085 | 6.083 ms | 7.024 ms | 7.641 ms | 9.95 KiB | 0.664 KiB |

Aggregate SQL queries: 5,258 over 2,920 requests (≈1.8/request). Pool acquires:
5,262; total acquire duration 10.48 ms; empty acquires 2; canceled 0; new
connections 2. Process RSS 16,512 KiB → 22,436 KiB.

Executed acceptance gates: zero errors/drops, steady p95 <=300 ms, feed
<=64 KiB, detail <=32 KiB. All passed for the delivered pilot load. The harness
stopped cleanly and the disposable DB cleanup ran.

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
- Full declared 5-minute warm-up, 10-minute steady and 60-second burst were not
  run; only the pilots above. The 100/1,000/100,000-DAU matrix and 100,000-DAU
  burst are OWED.
- No DB lock-wait percentile or CPU/steal time series was collected; only pool
  stats and an idle Docker sample. Per-endpoint SQL round trips were not
  separately traced; aggregate average is recorded. These are unresolved
  measurement gaps, not zero values.
- No staging, VPS, operator, legal, source activation, deployment, APK
  installation or merge is claimed.

## Next action

Run the full load matrix and A3 device union when a device/window is available;
then FOR-29 may prepare the authorized protected dev→main PR with these OWED
items retained.
