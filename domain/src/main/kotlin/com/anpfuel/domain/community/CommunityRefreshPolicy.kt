package com.anpfuel.domain.community

/**
 * Centralized refresh policy for the community surfaces (city feed and Home
 * community summary). One place owns every timing decision; callers only
 * evaluate it on lifecycle, scope, connectivity and manual events.
 *
 * `STALE_AFTER_MILLIS` is an initial candidate subject to measurement, not a
 * latency claim: it bounds how old foreground data may be, while time to
 * first paint is measured separately against the local snapshot.
 */
object CommunityRefreshPolicy {
    /** Foreground data older than this is refreshed on the next evaluation. */
    const val STALE_AFTER_MILLIS = 60_000L

    /** First retry waits this long; each consecutive failure doubles it. */
    const val RETRY_BASE_MILLIS = 15_000L

    /** Retry backoff never exceeds this, even after repeated failures. */
    const val RETRY_MAX_MILLIS = 300_000L

    /**
     * Auto-refresh keeps retrying while consecutive failures stay below this
     * budget (initial attempt plus spaced retries); at the budget it stops
     * until an explicit event (manual refresh, scope change, reconnect, return).
     */
    const val MAX_AUTO_RETRIES = 5

    /** Local expiry sweep / age recompute interval; never issues network calls. */
    const val SWEEP_INTERVAL_MILLIS = 30_000L

    /**
     * Loop wakeups never run closer than this, even when the computed delay
     * collapses to zero. A zero delay would freeze virtual time in tests and
     * busy-loop against the network in production; every loop pass is
     * local-only work, so the floor costs no extra call.
     */
    const val MIN_LOOP_DELAY_MILLIS = 5_000L

    fun isStale(nowMillis: Long, lastSuccessMillis: Long?): Boolean =
        lastSuccessMillis == null || nowMillis - lastSuccessMillis >= STALE_AFTER_MILLIS

    fun retryDelayMillis(failureCount: Int): Long {
        require(failureCount >= 1)
        var delay = RETRY_BASE_MILLIS
        repeat(failureCount - 1) {
            delay = (delay * 2).coerceAtMost(RETRY_MAX_MILLIS)
        }
        return delay.coerceAtMost(RETRY_MAX_MILLIS)
    }

    fun shouldKeepRetrying(failureCount: Int): Boolean = failureCount < MAX_AUTO_RETRIES
}
