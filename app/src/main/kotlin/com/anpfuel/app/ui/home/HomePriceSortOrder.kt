package com.anpfuel.app.ui.home

import androidx.annotation.StringRes
import com.anpfuel.app.R
import com.anpfuel.app.ui.model.AveragePriceUiModel

/**
 * Home price presentation order.
 *
 * Presentation-only: official ANP averages stay on their own source and
 * order never mixes, ranks or re-trusts anything. Missing averages sort
 * last in both directions so absent data is never presented first.
 */
enum class HomePriceSortOrder(@StringRes val labelRes: Int) {
    DEFAULT(R.string.home_filter_default),
    PRICE_ASC(R.string.home_filter_lowest),
    PRICE_DESC(R.string.home_filter_highest),
    ;

    fun sort(prices: List<AveragePriceUiModel>): List<AveragePriceUiModel> =
        when (this) {
            DEFAULT -> prices.sortedBy { it.fuelProduct.ordinal }
            PRICE_ASC ->
                prices.sortedWith(
                    compareBy<AveragePriceUiModel> { it.averageValue == null }
                        .thenBy { it.averageValue },
                )
            PRICE_DESC ->
                prices.sortedWith(
                    compareBy<AveragePriceUiModel> { it.averageValue == null }
                        .thenByDescending { it.averageValue },
                )
        }
}
