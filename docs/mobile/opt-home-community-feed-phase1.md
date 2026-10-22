# OPT home/community loading inventory — Phase 1 checkpoint

Scope: Home community summary (`HomeScreen`/`HomeViewModel` COMMUNITY source) and Community feed
(`CommunityScreen`/`CommunityFeedViewModel`). The plans named `HOME_OFFLINE_EFFICIENCY_PLAN`
(OPT-A00–A07), `APP_CONSOLIDATION_PLAN`, and `APP_CONSOLIDATION_TARGET_CONTRACT` do not exist in
this checkout; the implemented contract below is anchored on actual source at `fb85d4d1`.

## Behavior contract recorded before any behavior change

Existing accepted baseline (reused, not redefined): `docs/mobile/p23-city-community-feed.md`
B-BR-C07/C08/C09 and BUC-C06/C07 — canonical IBGE city scope, dated COMMUNITY projections with
server+client expiry, HMAC cursor pagination, foreground 15 s refresh, failure retains same-city
cards, city/fuel/sort change clears unrelated scope.

New optimization bounds for this task (no new product rules):

- B-BR-OPT01: `no-store` on `/v1/community/feed` and `/v1/community/summary` is honored as written.
  Public responses are not written to HTTP cache or any shared cache; a future Room snapshot is a
  per-query private copy keyed by full scope (origin, city code+state, fuel, sort, cursor/page), and
  never revalidates or serves credentials, private data, errors, or empty-as-failure.
- B-BR-OPT02: `refresh_after_seconds: 15` is an advisory hint only. It is not a latency proof and it
  does not authorize background polling, polling without connectivity, or overlapping equivalent
  queries. One equivalent query in flight at a time; simultaneous triggers converge; stale responses
  after a scope change are discarded.
- B-BR-OPT03: cached/expired data is never presented as current. Expired rows are filtered at read
  time; an empty valid response clears only its own scope; a failure preserves existing content with
  an explicit stale/error indication. Pagination state (cursor, 200-row cap, `earlierItemsTrimmed`,
  `newUpdates`) survives a first-page refresh and is reset only by explicit refresh or scope change.
- BUC-OPT01: GIVEN cached local data for the current scope, WHEN opening or returning to
  Home-summary/Community, THEN show it immediately with a correct age/stale mark, THEN refresh once
  when the scope/freshness policy requires it, without clearing the screen during the fetch.
- BUC-OPT02: GIVEN no cache, offline, slow network, failure, empty valid response, or expired data,
  WHEN loading, THEN keep empty/offline/error/empty-valid/expired-empty states distinct, preserve
  loaded content on failure, and offer explicit retry/manual refresh.

Out of scope (explicitly not implemented here): B/C/D tracks, portal, receipts, iOS, trust rules,
ETag/304 (unsupported by the server), push, deploy, device runs, capacity/battery claims.

## Current implementation (commit `fb85d4d1`, verified by reading source)

- Community feed refresh: `CommunityFeedViewModel.startRefresh()` runs `while(isActive) {
  refreshFirstPage(); delay(15_000) }`. Started on `setForeground(true)` / fuel/sort change /
  manual `refresh()`; cancelled on `setForeground(false)` or before each restart. No background
  polling outside the foreground lease; no connectivity gate inside the ViewModel.
- Home community summary refresh: `HomeScreen` RESUMED loop runs `refreshCommunity(periodic=true)`
  then `delay(15_000)` while `priceSource==COMMUNITY`. `refreshCommunity(periodic)` skips only when
  a previous community job is still active; an expiry timer (`communityExpiresAt`) clears prices at
  expiry. Manual refresh and source switch reuse the same path.
- Calls per foreground session: 1 initial + ~4/min while the screen stays foreground/RESUMED with the
  community surface selected. `loadMore()` is cursor-gated (`nextCursor != null`, single page job,
  cursor-advance `require`, `distinctBy(stationId)`, `takeLast(200)`).
- Initial load: feed shows skeleton only when `items.isEmpty()`; otherwise filters locally-expired
  rows and keeps the list (`loading=false`). Home summary keeps non-expired `communityPrices`
  across a periodic tick (`communityLoading=true` + linear indicator) and empties only when the
  snapshot is expired or the city changed.
- Pagination protection (already fixed, must be preserved): >20 rows / `loadingMore` /
  `earlierItemsTrimmed` suppresses first-page replacement and sets `newUpdates` instead; explicit
  `refresh()` (`resetPagination=true`) restores page one and clears `earlierItemsTrimmed`.
- Expiry: `expiresAt > now()` enforced on every ingest path (first page, `loadMore`, failure path,
  summary `live` filter). `initialHead` only tracks pages ≤20 rows and not trimmed. Feed
  `lastUpdated=page.generatedAt`; Home `communityGeneratedAt/UpdatedAt/ExpiresAt` from summary.
- Failures: `failed=true` + retain same-scope non-expired cards; `noCity` makes zero `invoke` calls;
  city mismatch clears items/cursor and reloads the new scope. Backend failures are redacted
  (`community.feed-unavailable`, `community.summary-unavailable`, no private fields in body).
- HTTP contract (actual): `GET /v1/community/feed?state&municipality_code&fuel_product&sort&limit&cursor?`
  and `GET /v1/community/summary?state&municipality_code`; `Cache-Control: no-store` on both
  (`feed_test.go` asserts it); no `ETag`/`Last-Modified`/`If-None-Match`/`304` support anywhere in
  `backend/` or `data/` (grep: zero hits). Client limit 20 (feed) / 8 (summary), 256 KiB body cap,
  depth ≤12, `distinctBy(stationId)`, cursor HMAC-bound to city+fuel+sort+limit with 16 min expiry.
- Persistence (actual): community feed/summary have no Room cache — in-memory `StateFlow` only.
  Room `AnpFuelDatabase` v16 has no community feed/summary tables/entities. ANP prices use Room
  (`AveragePrice`, `StationPrice`, catalog, etc.) and are independent of the community path.
- Reusable without new inventions: `ObserveNetworkConnectivityUseCase` (already drives
  `HomeViewModel.isOffline`), single-flight `communityGeneration`/`refreshJob`+`pageJob` tickets,
  `resetPagination`, `require(summary.city == city)` scope guard, existing `feed_cached_hint`,
  `feed_unavailable*`, `feed_new_updates`, `feed_restart_list`, `OfflineBanner`, skeleton components.
- UI string to remove in Phase 2: `feed_auto_refresh` in 8 locales
  (`values`, `values-de/es/fr/ja/pt-rBR/ru/zh-rCN`) — currently rendered under `feed_prices_title`
  in `CommunityScreen.kt:125`. Other `15_000` hits are only the two loops above plus tests; backend
  `refresh_after_seconds: 15` echo is asserted by `feed_test.go` and the Android decoders.

## Baseline measurements (this checkout, before any change)

- `:app:testDebugUnitTest --tests CommunityFeedViewModelTest --tests HomeViewModelTest`: PASS
  (29 s, 53 tasks, 18 executed).
- `go test ./internal/modules/community/adapters/http -run TestFeed|TestSummary`: PASS (0.002 s).
- Latency vs call-count are separate: no timing claim is made here. Phase 5 will measure
  time-to-first-paint (cache vs network) independently from calls/session and payload bytes.

## Acceptance criteria for the coming phases

- Phase 2: no `feed_auto_refresh` string/resource, no leftover separator/blank line, all 8 locales
  consistent, refresh affordance + a11y preserved, affected JVM/lint green.
- Phase 3: only if a backend prerequisite is truly missing; otherwise client-only safe snapshot with
  scope-keyed Room rows, intact validity dates, explicit storage bounds, empty-vs-failure split,
  migration on current version only, real reopen/isolation/expiry/failure tests.
- Phase 4: local-first paint, content preserved during refresh/failure with honest age, event-driven
  refresh (open/return, scope change, reconnect, contribution, manual), single-flight + stale-guard,
  no off-screen/offline polling, bounded retries, local age tick without network, pagination intact.
- Phase 5: matrix in the task brief, before/after calls+bytes+SQL+paint timings, no inferred
  battery/capacity claims, device checks recorded as pending.

Status: LOCAL_DONE
Validation: PASS
