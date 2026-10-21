# OPT home/community loading — Phase 5 evidence (acceptance + before/after)

Final tree: `b8c3a85a` + this file. Backend untouched across all phases
(`git diff fb85d4d1..HEAD -- backend/` empty); all behavior change is
Android-local. No push/PR/merge/deploy/device run performed (unauthorized).

## Acceptance matrix (all JVM unless noted)

- First open with cache: feed `fresh snapshot paints immediately without a
  network call`; Home `fresh community snapshot paints before the network
  answers` — content painted with zero `summary`/`feed` invocations, then
  replaced/augmented by the network answer.
- First open without cache: `missing city`, `no manually selected city`,
  skeleton paths unchanged; no fabricated content.
- Offline reopen: offline-entry tests on both surfaces — snapshot painted,
  `failed` mark set, exact 0 network calls verified.
- Slow network + failure without losing content: deferred-network tests
  (content visible while the fetch is still suspended), failure-retention
  tests (feed keeps same-scope cards; Home summary now retains validated
  prices instead of wiping), expired rows filtered locally.
- Valid empty response: empty first page decodes and renders the empty state
  (`city and fuel...` fetches an empty page); codec valid-empty round-trips.
- Expired data: sweep tests drop expired cards with exact call counts; expiry
  timers (`expireCommunity`, feed `expiresAt` filters) unchanged and green.
- Fast city/filter switch with late response: `late old city...`,
  `manual refresh replaces an in flight periodic request...`, cancellation
  tests, city/fuel scope-isolation tests.
- Simultaneous triggers: periodic-guard test, manual-replaces-periodic,
  reconnect resets backoff and converges to one query.
- Screen exit and return: `returning to a fresh screen...` (no re-query when
  fresh, re-query when stale), tab-exit stops work, Home session/lease tests.
- Connection loss and recovery: offline (0 calls) + reconnect (single
  conditional fetch) tests on both surfaces; no query is issued while offline.
- Manual refresh: always fetches when online (bypasses staleness/backoff),
  resets pagination, reports offline explicitly.
- Pagination preserved: `newUpdates` instead of card moves while browsing,
  200-row cap with explicit restart, failed page keeps cards, cursor pages
  never touch the snapshot cache.
- Contribution followed by update: returning from Capture re-fires START
  (`launchSingleTop` reuse in `AnpNavGraph`), which re-evaluates staleness and
  refreshes within the 60 s bound, plus one-tap manual refresh. End-to-end
  contribution→feed visibility on a real backend remains device/manual
  pending (Capture flow intentionally untouched).
- Origin and city isolation: store origin-isolation test, cache-key rule
  tests (origin/city/fuel/sort), city-mismatch and wrong-scope decode
  rejection tests.

## Final-tree verification (commands → results)

- `./gradlew :domain:test` → 549 tests, 0 failures/errors/skips.
- `./gradlew :data:testDebugUnitTest` (full) → 448 tests, 0 failures/errors,
  2 pre-existing skips.
- `./gradlew :app:testDebugUnitTest --tests community/home/stations` →
  95 tests, 0 failures/errors/skips.
- `./gradlew :app:assembleDebug` → BUILD SUCCESSFUL.
- `./gradlew :app:lintDebug` → BUILD SUCCESSFUL, zero `severity="Error"`.
- `go test ./internal/modules/community/adapters/http -count=1` → ok
  (backend unchanged; contract re-confirmed, not re-proven).
- `git diff --check` → clean; secret-surface grep on touched files → no hits.

## Before/after (feed first page, 5 foreground minutes, throwaway JVM scenario)

Measured on the final tree with virtual time (`StandardTestDispatcher`;
scenario file removed after recording). Old counts are analytic from the
replaced fixed-15 s loop (1 + 300/15 = 21).

- Fresh data: 21 calls → exactly 6 calls (−71%). Same payload shape per call
  (≤20 rows, 256 KiB cap), so ≈−71% bytes on this surface.
- Offline with snapshot: 21 failed attempts → 0 calls, content kept + mark.
- Total outage: 21 failing calls → exactly 5 (1 + 4 spaced retries), then
  silence until an explicit event; manual refresh still works immediately.
- First paint: with snapshot, content before any network completion
  (0 calls asserted); without snapshot, one call as before. Time-to-paint and
  call-count are reported separately; no millisecond or battery/capacity
  claims are made.
- SQL/backend delta: none (no new runtime queries; backend files untouched).

## Still pending (not claimed)

On-device Compose rendering of the new stale/failed-with-content branches,
Room reopen/upgrade on a physical device, staging end-to-end (including
contribution→feed visibility), and production latency measurement. The
deferred device batch owns these; source acceptance above is LOCAL_DONE /
INTEGRATION_PENDING, not a release certification.

Status: LOCAL_DONE
Validation: PASS
