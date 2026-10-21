package com.anpfuel.domain.community

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class CommunityFeedCacheRuleTest {
    private val city = FeedCity("5103403", BrazilianState.MATO_GROSSO, "Synthetic city")
    private val otherCity = FeedCity("5107925", BrazilianState.MATO_GROSSO, "Other city")
    private val origin = "https://teste.abastevo.com.br"

    @Test fun `feed keys isolate origin city fuel and sort`() {
        val base = CommunityFeedScope(origin, city, FuelProduct.ETHANOL, FeedSort.RECENT)
        val keys = setOf(
            CommunityFeedCacheRule.feedKey(base),
            CommunityFeedCacheRule.feedKey(base.copy(origin = "https://other.invalid")),
            CommunityFeedCacheRule.feedKey(base.copy(city = otherCity)),
            CommunityFeedCacheRule.feedKey(base.copy(fuel = FuelProduct.DIESEL_S10)),
            CommunityFeedCacheRule.feedKey(base.copy(sort = FeedSort.CHEAPEST)),
        )
        assertEquals(5, keys.size)
    }

    @Test fun `summary keys isolate origin and city but never collide with feed`() {
        val summary = CommunityFeedCacheRule.summaryKey(CommunitySummaryScope(origin, city))
        val otherSummary = CommunityFeedCacheRule.summaryKey(CommunitySummaryScope(origin, otherCity))
        val feed = CommunityFeedCacheRule.feedKey(CommunityFeedScope(origin, city, FuelProduct.ETHANOL, FeedSort.RECENT))
        assertNotEquals(summary, otherSummary)
        assertNotEquals(summary, feed)
    }

    @Test fun `origin normalization keeps case and slash variants on one key`() {
        val scope = CommunityFeedScope("HTTPS://TESTE.ABASTEVO.COM.BR/", city, FuelProduct.ETHANOL, FeedSort.RECENT)
        assertEquals(
            CommunityFeedCacheRule.feedKey(CommunityFeedScope(origin, city, FuelProduct.ETHANOL, FeedSort.RECENT)),
            CommunityFeedCacheRule.feedKey(scope),
        )
    }

    @Test fun `storage bounds stay explicit`() {
        assertEquals(64, CommunityFeedCacheRule.MAX_ENTRIES)
        assertEquals(7L * 24 * 60 * 60 * 1000, CommunityFeedCacheRule.MAX_STORAGE_AGE_MILLIS)
    }
}
