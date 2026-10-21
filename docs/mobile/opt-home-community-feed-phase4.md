# OPT home/community loading — Phase 4 evidence (local-first + event-driven refresh)

## What changed (user-visible)

- Community feed and the Home community summary now paint locally available data
  immediately when it exists (private Room snapshot from Phase 3, else retained
  in-memory content), then refresh from the network only when the data is stale
  (older than 60 s), the scope changed, connectivity returned, or the user
  asked explicitly. The screen is never blanked for a refresh.
- Failure and offline states keep the last validated cards with the existing
  honest wording (`feed_unavailable` + `feed_cached_hint` / `home_community_*`
  error only when there is nothing to show). A discreet "previously loaded"
  hint also appears when painted data is older than the window, without any
  new strings or translations.
- The fixed 15 s foreground polling is gone from both surfaces. Refresh timing
  is owned by one central policy (`CommunityRefreshPolicy`: 60 s stale window
  as a measurable initial candidate, 15→30→60…s backoff capped at 5 min,
  max 5 auto-retries, 30 s local sweep tick, 5 s loop floor) evaluated on
  open/return, city/fuel/sort change, reconnect, manual refresh and the
  scheduled tick. No periodic query runs off-screen or offline.
- Pagination behavior is preserved byte-for-byte: first-page refresh while
  browsing older pages advertises `newUpdates` instead of moving cards;
  explicit refresh resets to page one; cursor pages stay network-only and
  isolated per scope; the 200-row cap and `earlierItemsTrimmed` restart survive.

## Mechanics (kept or added)

- Single equivalent query in flight per surface (feed `refreshJob` cancel +
  `pageJob` guard; Home `communityGeneration` tickets + in-flight periodic
  guard). Simultaneous triggers converge: manual/scope changes cancel and
  replace, periodic ticks never preempt, reconnect resets backoff and
  re-evaluates. Stale responses after a scope change are discarded by
  cancellation + ticket/generation checks (unchanged, re-tested).
- Failure retention changed for the Home summary (it used to wipe prices on
  failure): non-expired prices are now retained with `failed` + `stale` marks,
  matching the feed. The old wipe assertion was updated to the new contract.
- Local sweep every 30 s of foreground time expires cards / recomputes age with
  zero network calls. A 5 s loop floor plus a minimum gap between attempts
  makes zero-delay busy-loops impossible (see below).

## Validation (worktree `codex/opt-home-community-feed`)

- New: `CommunityRefreshPolicyTest` (domain, 2 tests) — stale window, backoff
  doubling/cap, retry budget, loop floor.
- Feed: 10 pre-existing tests updated to the event-driven timing (same
  assertions, 60 s window / clock advanced explicitly) + 7 new tests —
  snapshot-first paint with zero network calls, aged-snapshot stale mark,
  offline paint with error mark and zero calls, reconnect refresh, return
  without re-query vs stale re-query, backoff budget then silence then manual
  recovery, local sweep with exact call counts.
- Home: failure-retention assertion updated to the new contract + 5 new tests —
  snapshot-first paint, aged-snapshot stale mark, offline mark without calls,
  reconnect refresh, backoff budget then silence then manual recovery.
- Suites: full `:domain:test` (549 tests) PASS; `:app:testDebugUnitTest`
  community+home+stations (95 tests) PASS; data community-cache/http-decoder
  suites (23 tests) PASS; `:app:assembleDebug` PASS; `:app:lintDebug` PASS with
  zero errors. Backend untouched in this phase.
- During development the new loop exposed a real zero-delay busy-loop (a stale
  window combined with failing/min-delay math could schedule `delay(0)` and
  OOM the test worker). Fixed with the 5 s loop floor and a minimum gap
  between attempts; regression-covered by the backoff/silence/sweep tests
  above, which assert exact network call counts across virtual time.
- `git diff --check` clean; secret-surface grep on touched files: no hits.
- Not run here (recorded pending, consistent with the deferred device batch):
  on-device Compose rendering of the new stale/failed-with-content branches and
  Room reopen on a physical device; no device claim.

Status: LOCAL_DONE
Validation: PASS
