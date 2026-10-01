package com.anpfuel.application.portable

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class PortableSyncStateTest {

    @Test
    fun onlineRemoteWinsAsFresh() {
        val cache = CachedValue("old", revision = 1L, cachedTick = 0L)
        val result = PortableSyncState.resolve(
            remotePayload = "new",
            remoteRevision = 2L,
            cache = cache,
            isOnline = true,
            tickMillis = 10_000L,
            staleAfterMillis = 1000L,
        )
        assertEquals(ReadOrigin.REMOTE_FRESH, result.origin)
        assertEquals("new", result.payload)
        assertEquals(2L, result.revision)
    }

    @Test
    fun offlineCacheHitServesCachedPayload() {
        val cache = CachedValue("cached", revision = 4L, cachedTick = 9000L)
        val result = PortableSyncState.resolve(
            remotePayload = null,
            remoteRevision = 0L,
            cache = cache,
            isOnline = false,
            tickMillis = 9500L,
            staleAfterMillis = 1000L,
        )
        assertEquals(ReadOrigin.CACHE, result.origin)
        assertEquals("cached", result.payload)
    }

    @Test
    fun expiredCacheIsMarkedStaleNeverSilentFresh() {
        val cache = CachedValue("old", revision = 1L, cachedTick = 0L)
        val result = PortableSyncState.resolve(
            remotePayload = null,
            remoteRevision = 0L,
            cache = cache,
            isOnline = false,
            tickMillis = 60_001L,
            staleAfterMillis = 60_000L,
        )
        assertEquals(ReadOrigin.CACHE_STALE, result.origin)
        assertEquals("old", result.payload)
    }

    @Test
    fun clockSkewNeverMarksFreshCacheStale() {
        val cache = CachedValue("cached", revision = 1L, cachedTick = 10_000L)
        val result = PortableSyncState.resolve(
            remotePayload = null,
            remoteRevision = 0L,
            cache = cache,
            isOnline = false,
            tickMillis = 9000L,
            staleAfterMillis = 1000L,
        )
        assertEquals(ReadOrigin.CACHE, result.origin)
    }

    @Test
    fun noRemoteAndNoCacheIsEmpty() {
        val result = PortableSyncState.resolve(
            remotePayload = null,
            remoteRevision = 0L,
            cache = null,
            isOnline = false,
            tickMillis = 0L,
            staleAfterMillis = 1000L,
        )
        assertEquals(ReadOrigin.EMPTY, result.origin)
        assertNull(result.payload)
    }

    @Test
    fun outageWithoutRemoteFallsBackToCacheNeverEmpty() {
        val cache = CachedValue("cached", revision = 4L, cachedTick = 9000L)
        val result = PortableSyncState.resolve(
            remotePayload = null,
            remoteRevision = 0L,
            cache = cache,
            isOnline = true,
            tickMillis = 9500L,
            staleAfterMillis = 1000L,
        )
        assertEquals(ReadOrigin.CACHE, result.origin)
        assertEquals("cached", result.payload)
    }
}
