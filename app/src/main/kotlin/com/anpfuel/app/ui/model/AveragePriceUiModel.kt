package com.anpfuel.app.ui.model

import com.anpfuel.domain.community.CommunityConfirmationCounts
import com.anpfuel.domain.valueobject.FuelProduct

data class AveragePriceUiModel(
    val fuelProduct: FuelProduct,
    val averageFormatted: String?,
    val minimumFormatted: String?,
    val maximumFormatted: String?,
    val stationCount: Int?,
    val confirmationCounts: CommunityConfirmationCounts? = null,
)
