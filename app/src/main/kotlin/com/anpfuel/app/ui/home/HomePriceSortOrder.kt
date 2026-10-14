package com.anpfuel.app.ui.home

import androidx.annotation.StringRes
import com.anpfuel.app.R

/**
 * Home featured price per fuel.
 *
 * Presentation-only: official ANP statistics stay on their own source,
 * the fuel order never changes and nothing is re-ranked or re-trusted.
 * DEFAULT highlights the average; PRICE_ASC the surveyed minimum and
 * PRICE_DESC the surveyed maximum, each falling back to the average.
 */
enum class HomePriceSortOrder(@StringRes val labelRes: Int) {
    DEFAULT(R.string.home_filter_default),
    PRICE_ASC(R.string.home_filter_lowest),
    PRICE_DESC(R.string.home_filter_highest),
}

/** The three filters select a statistic from the chosen source; they never combine sources. */
internal fun HomePriceSortOrder.featuredPrice(price: com.anpfuel.app.ui.model.AveragePriceUiModel): String? =
    when(this) {
        HomePriceSortOrder.DEFAULT -> price.averageFormatted
        HomePriceSortOrder.PRICE_ASC -> price.minimumFormatted ?: price.averageFormatted
        HomePriceSortOrder.PRICE_DESC -> price.maximumFormatted ?: price.averageFormatted
    }
