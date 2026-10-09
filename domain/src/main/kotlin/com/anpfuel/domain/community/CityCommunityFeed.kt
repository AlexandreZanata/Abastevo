package com.anpfuel.domain.community

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import java.time.Instant
import java.util.UUID

/** Public, standard-condition facts only; no contributor identity or media. */
data class FeedCity(val code: String, val state: BrazilianState, val name: String) {
    init { require(code.matches(Regex("[0-9]{7}"))); require(name.isNotBlank()) }
}
enum class FeedSort { RECENT, CHEAPEST, BEST, WORST }
data class FeedQuery(val city: FeedCity, val fuel: FuelProduct, val sort: FeedSort = FeedSort.RECENT)
data class CommunityFeedItem(
    val stationId: String,
    val stationName: String,
    val fuel: FuelProduct,
    val amountMilliBrl: Long,
    val updatedAt: Instant,
    val expiresAt: Instant,
    val supporters: Int,
    val confirmations: Int,
    val confidence: String,
    val version: Long,
    val ratingsCount: Int = 0,
    val ratingsAvg: Double? = null,
) {
    init {
        require(UUID.fromString(stationId).toString() == stationId)
        require(stationName.isNotBlank())
        require(amountMilliBrl in 1..1_000_000)
        require(expiresAt > updatedAt)
        require(supporters >= 0 && confirmations >= 0 && version > 0)
        require(confidence in setOf("LOW", "MEDIUM", "HIGH"))
        require(ratingsCount >= 0)
        require(ratingsAvg == null || ratingsAvg in 1.0..5.0)
        require((ratingsAvg == null) == (ratingsCount == 0))
    }
    val unit: String get() = when (fuel) {
        FuelProduct.CNG -> "M3"
        FuelProduct.LPG_P13 -> "KG_13"
        else -> "L"
    }
}
data class CommunityFeedPage(val items: List<CommunityFeedItem>, val nextCursor: String?, val generatedAt: Instant)
interface CityCommunityFeedReader {
    suspend fun read(query: FeedQuery, cursor: String? = null): CommunityFeedPage
}
