package com.anpfuel.data.remote

import java.time.Clock
import java.time.Instant
import java.util.concurrent.atomic.AtomicReference
import javax.inject.Inject
import javax.inject.Singleton
import com.anpfuel.domain.rule.NominatimRateLimitRule

/**
 * In-memory 1 req/s throttle for Nominatim (BR-021).
 */
@Singleton
class NominatimRateLimiter @Inject constructor(
    private val clock: Clock,
) {

    private val lastRequestAt = AtomicReference<Instant?>(null)

    fun canRequest(): Boolean =
        NominatimRateLimitRule.canRequest(clock.instant(), lastRequestAt.get())

    fun recordRequest(at: Instant = clock.instant()) {
        lastRequestAt.set(at)
    }

    fun resetForTests() {
        lastRequestAt.set(null)
    }
}
