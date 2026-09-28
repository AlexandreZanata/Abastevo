package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException
import java.time.Duration
import java.time.Instant

/**
 * BR-021 — Client-side throttle for public Nominatim API (max 1 request per second).
 */
object NominatimRateLimitRule {

    const val MIN_INTERVAL_MILLIS = 1_000L

    fun canRequest(now: Instant, lastRequestAt: Instant?): Boolean {
        if (lastRequestAt == null) {
            return true
        }
        return Duration.between(lastRequestAt, now).toMillis() >= MIN_INTERVAL_MILLIS
    }

    fun requireCanRequest(now: Instant, lastRequestAt: Instant?) {
        if (!canRequest(now, lastRequestAt)) {
            throw DomainException("Nominatim rate limit: max 1 request per second")
        }
    }
}
