package com.anpfuel.domain.rule

/**
 * P10-T02 backend cache freshness.
 *
 * Mirrors the backend `Cache-Control: public, max-age=60` on
 * `GET /v1/stations/{id}/prices`. A cached entry is fresh while
 * `now < expiresAt`; at or past expiry it is stale but still usable as
 * an explicit offline fallback (never silently presented as fresh).
 */
object BackendPriceCacheRule {

    const val MAX_AGE_MILLIS = 60_000L

    fun expiryFor(fetchedAtMillis: Long): Long =
        fetchedAtMillis + MAX_AGE_MILLIS

    fun isStale(nowMillis: Long, expiresAtMillis: Long): Boolean =
        nowMillis >= expiresAtMillis

    fun isFresh(nowMillis: Long, expiresAtMillis: Long): Boolean =
        !isStale(nowMillis, expiresAtMillis)
}
