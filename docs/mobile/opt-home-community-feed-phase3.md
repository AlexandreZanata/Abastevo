# OPT home/community loading — Phase 3 evidence (HTTP contract + safe persistence)

## HTTP contract verification (actual, commit `8de22c24` + working tree)

- `GET /v1/community/feed` and `GET /v1/community/summary` answer
  `Cache-Control: no-store` (asserted by `backend/.../http/feed_test.go`); no
  `ETag`/`Last-Modified`/`If-None-Match`/`304` anywhere in `backend/` or `data/`
  (grep zero hits). OkHttp clients are built without a `Cache`, so no-store is
  honored at transport: nothing is reused implicitly.
- `refresh_after_seconds: 15` remains an advisory echo only. It is recorded in
  snapshot payloads as observed but never drives refresh policy and is never
  sent back to the server. No 304 support was invented: no conditional request
  is issued, and the "304 refreshes last-check" clause is not applicable.
- No backend prerequisite was missing, so no OPT-A02/backend change was needed.
  Backend files are untouched in this phase (client-only change).

## Persistence implemented (Room, additive, scope-keyed)

- New `community_feed_cache` table (DB v16→v17, `MIGRATION_16_17` creates only
  this table; schema `17.json` generated and committed). Key is
  `KIND|origin|state|code|fuel|sort` (summary omits fuel/sort), so origin,
  city, fuel and sort are fully isolated; pagination cursors are never stored
  (first-page/summary snapshots only; deeper pages stay network-only).
- Payloads are the validated wire JSON, decoded through the existing strict
  decoders (fail closed on drift). Original `generated_at`/`updated_at`/
  `expires_at` are preserved; snapshots filter `expiresAt > now` at read time
  and never extend validity. Valid-empty responses are stored (a checked empty
  scope, not a miss). Errors never overwrite the cache; cache failures never
  fail a validated read (best-effort write, fail-soft snapshot).
- Explicit storage bounds: 64 newest entries + 7-day fetched-age hygiene,
  enforced on every write (`deleteOlderThan` + `trimToNewest`).
- Wiring: `CommunityFeedCacheStore` (singleton) wraps the existing HTTP client
  and is bound as `CityCommunityFeedReader`, `CityCommunitySummaryReader` and
  `CommunityFeedLocalSnapshot`. Existing use-cases, ViewModels and HTTP
  decoders are byte-identical in behavior except for the best-effort snapshot
  write on validated success. Local-first display wiring is Phase 4.

## Validation (worktree `codex/opt-home-community-feed`)

- New: `CommunityFeedCacheRuleTest` (domain, 4 tests PASS) — key isolation per
  origin/city/fuel/sort, normalization, explicit bounds.
- New: `CommunityFeedCacheCodecTest` (data, 5 tests PASS) — feed/summary and
  valid-empty round-trips through the strict decoders, cursor dropped,
  tampered scope/source fails closed.
- New: `CommunityFeedCacheStoreTest` (data, 8 tests PASS) — write on first-page
  success + prune, cursor bypass, failure never overwrites, write-failure never
  fails reads, expiry filtering with preserved age, null on miss/corrupt/
  oversized/DAO failure, origin isolation, summary write/failure semantics.
- New: `Migration16To17UnitTest` (PASS) — creates only the snapshot table, no
  DELETE/DROP/ALTER, no other table touched. Schema `17.json` (v17, 17 tables)
  matches the migration SQL.
- Affected suites: `:domain:test` PASS; `:application:test --tests community.*`
  PASS; `:app:testDebugUnitTest --tests community/home/stations` PASS;
  full `:data:testDebugUnitTest` PASS (448 tests, 0 failures/errors, 2 skipped);
  `:app:assembleDebug` PASS; `:app:lintDebug` PASS with zero errors.
- `git diff --check` clean; secret-surface grep on touched files: no hits.
- Not run here (recorded pending, consistent with the deferred device batch):
  on-device Room reopen/upgrade and screenshot verification; no device claim.

Status: LOCAL_DONE
Validation: PASS
