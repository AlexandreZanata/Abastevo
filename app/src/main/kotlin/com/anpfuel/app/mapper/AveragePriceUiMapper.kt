package com.anpfuel.app.mapper

import com.anpfuel.app.ui.model.AveragePriceUiModel
import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.valueobject.FuelProduct
import java.util.Locale

object AveragePriceUiMapper {

    fun toUiModels(
        prices: List<AveragePrice>,
        locale: Locale,
    ): List<AveragePriceUiModel> =
        prices
            .sortedBy { it.fuelProduct.ordinal }
            .map { toUiModel(it, locale) }

    fun toUiModel(price: AveragePrice, locale: Locale): AveragePriceUiModel =
        AveragePriceUiModel(
            fuelProduct = price.fuelProduct,
            averageFormatted = PriceFormatter.formatAmount(price.average, locale),
            minimumFormatted = PriceFormatter.formatAmount(price.minimum, locale),
            maximumFormatted = PriceFormatter.formatAmount(price.maximum, locale),
            stationCount = price.stationCount,
        )

    fun orderedProductsWithData(prices: List<AveragePriceUiModel>): List<AveragePriceUiModel> =
        FuelProduct.entries.mapNotNull { product ->
            prices.firstOrNull { it.fuelProduct == product }
        }
}
