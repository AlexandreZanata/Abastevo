package com.anpfuel.domain.rule

import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class BackendPriceCacheRuleTest {

    @Test
    fun `fresh before max-age`() {
        val fetchedAt = 1_000_000L
        val expiresAt = BackendPriceCacheRule.expiryFor(fetchedAt)

        assertTrue(BackendPriceCacheRule.isFresh(fetchedAt, expiresAt))
        assertTrue(BackendPriceCacheRule.isFresh(expiresAt - 1, expiresAt))
        assertFalse(BackendPriceCacheRule.isStale(fetchedAt, expiresAt))
    }

    @Test
    fun `stale at and after expiry`() {
        val fetchedAt = 1_000_000L
        val expiresAt = BackendPriceCacheRule.expiryFor(fetchedAt)

        assertTrue(BackendPriceCacheRule.isStale(expiresAt, expiresAt))
        assertTrue(BackendPriceCacheRule.isStale(expiresAt + 1, expiresAt))
        assertFalse(BackendPriceCacheRule.isFresh(expiresAt, expiresAt))
    }

    @Test
    fun `ttl matches backend max-age 60s`() {
        assertTrue(BackendPriceCacheRule.MAX_AGE_MILLIS == 60_000L)
    }
}
