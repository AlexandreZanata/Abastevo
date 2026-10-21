package com.anpfuel.data.remote

import com.anpfuel.domain.community.CityCommunitySummary
import com.anpfuel.domain.community.CommunityFeedItem
import com.anpfuel.domain.community.CommunityFeedPage
import com.anpfuel.domain.community.CommunitySummaryPrice
import com.anpfuel.domain.community.ContributionReference
import com.anpfuel.domain.community.FeedCity
import com.anpfuel.domain.community.FeedQuery
import com.anpfuel.domain.community.FeedSort
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import java.time.Instant
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class CommunityFeedCacheCodecTest {
    private val city = FeedCity("5103403", BrazilianState.MATO_GROSSO, "Synthetic city")
    private val at = Instant.parse("2026-10-14T12:00:00Z")
    private val query = FeedQuery(city, FuelProduct.ETHANOL, FeedSort.RECENT)

    private fun item() = CommunityFeedItem(
        stationId = "00000000-0000-0000-0000-000000000001",
        stationName = "Synthetic station",
        fuel = FuelProduct.ETHANOL,
        amountMilliBrl = 5032,
        updatedAt = at.minusSeconds(60),
        expiresAt = at.plusSeconds(3600),
        supporters = 3,
        confirmations = 1,
        confidence = "LOW",
        version = 2,
        ratingsCount = 2,
        ratingsAvg = 4.5,
        contribution = ContributionReference("00000000-0000-0000-0000-000000000002", 7),
    )

    private fun price() = CommunitySummaryPrice(
        FuelProduct.ETHANOL, 5032, 5000, 5064, 65, at.minusSeconds(3600), at.plusSeconds(3600),
    )

    @Test fun `feed round trip preserves rows dates and source through strict decode`() {
        val page = CommunityFeedPage(listOf(item()), "live-cursor", at)
        val decoded = CityCommunityFeedHttpClient.decode(CommunityFeedCacheCodec.encodeFeed(query, page), query)
        assertEquals(page.items, decoded.items)
        assertEquals(at, decoded.generatedAt)
        // Snapshots are first-page only; live cursors never persist.
        assertNull(decoded.nextCursor)
    }

    @Test fun `valid empty feed stays empty and decodable`() {
        val decoded = CityCommunityFeedHttpClient.decode(
            CommunityFeedCacheCodec.encodeFeed(query, CommunityFeedPage(emptyList(), null, at)), query,
        )
        assertTrue(decoded.items.isEmpty())
        assertEquals(at, decoded.generatedAt)
    }

    @Test fun `tampered feed scope or source fails closed`() {
        val payload = CommunityFeedCacheCodec.encodeFeed(query, CommunityFeedPage(listOf(item()), null, at))
        val wrongCity = JSONObject(payload).put("municipality_code", "5107925").toString()
        assertThrows(IllegalArgumentException::class.java) { CityCommunityFeedHttpClient.decode(wrongCity, query) }
        val wrongSource = JSONObject(payload).also {
            it.getJSONArray("items").getJSONObject(0).put("source", "ANP")
        }.toString()
        assertThrows(IllegalArgumentException::class.java) { CityCommunityFeedHttpClient.decode(wrongSource, query) }
    }

    @Test fun `summary round trip preserves statistics and dates`() {
        val summary = CityCommunitySummary(city, listOf(price()), at)
        val decoded = CityCommunityFeedHttpClient.decodeSummary(CommunityFeedCacheCodec.encodeSummary(summary), city)
        assertEquals(summary, decoded)
    }

    @Test fun `valid empty summary stays empty and tampered city fails closed`() {
        val empty = CityCommunitySummary(city, emptyList(), at)
        assertTrue(CityCommunityFeedHttpClient.decodeSummary(CommunityFeedCacheCodec.encodeSummary(empty), city).items.isEmpty())
        val payload = CommunityFeedCacheCodec.encodeSummary(CityCommunitySummary(city, listOf(price()), at))
        val wrongCity = JSONObject(payload).put("state", "SP").toString()
        assertThrows(IllegalArgumentException::class.java) { CityCommunityFeedHttpClient.decodeSummary(wrongCity, city) }
    }
}
