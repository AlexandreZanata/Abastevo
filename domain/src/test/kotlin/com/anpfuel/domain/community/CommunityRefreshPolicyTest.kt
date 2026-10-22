package com.anpfuel.domain.community

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class CommunityRefreshPolicyTest {
    @Test fun `missing success is stale and freshness lasts one window`() {
        assertTrue(CommunityRefreshPolicy.isStale(60_000, null))
        assertFalse(CommunityRefreshPolicy.isStale(60_000, 60_000))
        assertFalse(CommunityRefreshPolicy.isStale(119_999, 60_000))
        assertTrue(CommunityRefreshPolicy.isStale(120_000, 60_000))
    }

    @Test fun `retries double from the base and stop after the budget`() {
        assertEquals(15_000L, CommunityRefreshPolicy.retryDelayMillis(1))
        assertEquals(30_000L, CommunityRefreshPolicy.retryDelayMillis(2))
        assertEquals(60_000L, CommunityRefreshPolicy.retryDelayMillis(3))
        assertEquals(120_000L, CommunityRefreshPolicy.retryDelayMillis(4))
        assertTrue(CommunityRefreshPolicy.retryDelayMillis(20) <= CommunityRefreshPolicy.RETRY_MAX_MILLIS)
        assertTrue(CommunityRefreshPolicy.shouldKeepRetrying(CommunityRefreshPolicy.MAX_AUTO_RETRIES - 1))
        assertFalse(CommunityRefreshPolicy.shouldKeepRetrying(CommunityRefreshPolicy.MAX_AUTO_RETRIES))
        assertEquals(5_000L, CommunityRefreshPolicy.MIN_LOOP_DELAY_MILLIS)
    }
}
