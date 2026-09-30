package com.anpfuel.application.portable

/**
 * Portable offline read policy (P12-T03).
 *
 * Pure Kotlin with zero `java.*`/Android imports so this file moves unchanged
 * into a future `commonMain` source set. The rule is explicit: a present
 * remote payload wins while online, otherwise the cache serves reads with
 * honest staleness, and the absence of both is EMPTY — never a silent empty
 * list presented as fresh data. Backend outage and airplane mode resolve to
 * the same states through [isOnline]; the policy never performs I/O itself.
 */
enum class ReadOrigin {
    REMOTE_FRESH,
    CACHE,
    CACHE_STALE,
    EMPTY,
}

data class CachedValue(
    val payload: String,
    val revision: Long,
    val cachedTick: Long,
)

data class ReadResult(
    val origin: ReadOrigin,
    val payload: String?,
    val revision: Long,
)

object PortableSyncState {

    fun resolve(
        remotePayload: String?,
        remoteRevision: Long,
        cache: CachedValue?,
        isOnline: Boolean,
        tickMillis: Long,
        staleAfterMillis: Long,
    ): ReadResult {
        if (isOnline && remotePayload != null) {
            return ReadResult(ReadOrigin.REMOTE_FRESH, remotePayload, remoteRevision)
        }
        if (cache != null) {
            val age = tickMillis - cache.cachedTick
            if (age >= 0 && age > staleAfterMillis) {
                return ReadResult(ReadOrigin.CACHE_STALE, cache.payload, cache.revision)
            }
            return ReadResult(ReadOrigin.CACHE, cache.payload, cache.revision)
        }
        return ReadResult(ReadOrigin.EMPTY, null, 0L)
    }
}
