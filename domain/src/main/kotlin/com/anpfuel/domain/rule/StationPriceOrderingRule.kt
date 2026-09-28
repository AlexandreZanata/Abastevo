package com.anpfuel.domain.rule

import com.anpfuel.domain.model.StationPrice

/**
 * UC-007 — Station prices are displayed sorted by ascending price.
 */
object StationPriceOrderingRule {

    fun orderByPriceAscending(prices: List<StationPrice>): List<StationPrice> =
        prices.sortedBy { it.price.value }
}
