package com.anpfuel.domain.discovery

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.rule.MinimumSearchLengthRule
import com.anpfuel.domain.rule.StationPriceOrderingRule

/**
 * P20-T02 — Pure list-first Explore filtering, sorting and pagination.
 *
 * Works on the local station list (manual city + fuel already resolved by
 * the caller, no GPS/account required). Search matches trade/legal name,
 * brand and address case-insensitively. A blank search — or one below the
 * [MinimumSearchLengthRule] minimum while the user is still typing — does
 * not exclude stations; the committed-query validator ([DiscoveryQuery])
 * still enforces the minimum for backend reads. No match is an honest
 * empty list, never an error.
 */
object DiscoveryStationsRule {

    data class DiscoveryPage(
        val items: List<StationPrice>,
        val hasMore: Boolean,
        val totalCount: Int,
    )

    fun filterAndSort(
        stations: List<StationPrice>,
        search: String?,
        sort: DiscoverySort,
    ): List<StationPrice> {
        val normalized = search?.trim().orEmpty()
        val filtered = if (normalized.length < MinimumSearchLengthRule.MIN_LENGTH) {
            stations
        } else {
            stations.filter { matches(it.station, normalized) }
        }
        return when (sort) {
            DiscoverySort.PRICE_ASC -> StationPriceOrderingRule.orderByPriceAscending(filtered)
            DiscoverySort.RECENCY_DESC -> filtered.sortedWith { first, second ->
                when {
                    first.collectedAt == null && second.collectedAt == null ->
                        first.price.value.compareTo(second.price.value)
                    first.collectedAt == null -> 1
                    second.collectedAt == null -> -1
                    else -> {
                        val dateOrder = second.collectedAt.compareTo(first.collectedAt)
                        if (dateOrder != 0) {
                            dateOrder
                        } else {
                            first.price.value.compareTo(second.price.value)
                        }
                    }
                }
            }
        }
    }

    fun paginate(
        stations: List<StationPrice>,
        page: Int,
        pageSize: Int,
    ): DiscoveryPage {
        if (page < 0) throw DomainException("discovery page must be >= 0")
        if (pageSize !in 1..DiscoveryQuery.MAX_PAGE_SIZE) {
            throw DomainException("discovery pageSize must be 1..${DiscoveryQuery.MAX_PAGE_SIZE}")
        }
        val fromIndex = page * pageSize
        if (fromIndex >= stations.size) {
            return DiscoveryPage(items = emptyList(), hasMore = false, totalCount = stations.size)
        }
        val toIndex = minOf(fromIndex + pageSize, stations.size)
        return DiscoveryPage(
            items = stations.subList(fromIndex, toIndex).toList(),
            hasMore = toIndex < stations.size,
            totalCount = stations.size,
        )
    }

    private fun matches(station: RetailStation, search: String): Boolean {
        val candidates = listOfNotNull(
            station.tradeName,
            station.legalName,
            station.brand,
            station.address,
        )
        return candidates.any { it.contains(search, ignoreCase = true) }
    }
}
