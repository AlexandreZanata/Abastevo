package com.anpfuel.domain.community

import com.anpfuel.domain.valueobject.FuelProduct
import java.time.Instant

/** One equal-weight station population, never a sample from a paginated feed. */
data class CommunitySummaryPrice(
    val fuel: FuelProduct,
    val averageMilliBrl: Long,
    val minimumMilliBrl: Long,
    val maximumMilliBrl: Long,
    val stationCount: Int,
    val updatedAt: Instant,
    val expiresAt: Instant,
) {
    init {
        require(minimumMilliBrl in 1..1_000_000 && maximumMilliBrl in minimumMilliBrl..1_000_000)
        require(averageMilliBrl in minimumMilliBrl..maximumMilliBrl && stationCount > 0)
        require(expiresAt > updatedAt)
    }
    val unit: String get() = when(fuel) { FuelProduct.CNG -> "M3"; FuelProduct.LPG_P13 -> "KG_13"; else -> "L" }
}
data class CityCommunitySummary(val city: FeedCity, val items: List<CommunitySummaryPrice>, val generatedAt: Instant) {
    init {
        require(items.size <= FuelProduct.entries.size && items.distinctBy { it.fuel }.size == items.size)
        require(items.all { it.updatedAt <= generatedAt && it.expiresAt > generatedAt })
    }
}
interface CityCommunitySummaryReader {
    suspend fun readSummary(city: FeedCity): CityCommunitySummary
}
