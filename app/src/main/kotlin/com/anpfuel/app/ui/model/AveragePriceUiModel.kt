package com.anpfuel.app.ui.model

import com.anpfuel.domain.valueobject.FuelProduct
import java.math.BigDecimal

data class AveragePriceUiModel(
    val fuelProduct: FuelProduct,
    val averageFormatted: String?,
    val minimumFormatted: String?,
    val maximumFormatted: String?,
    val stationCount: Int?,
    val averageValue: BigDecimal? = null,
)
