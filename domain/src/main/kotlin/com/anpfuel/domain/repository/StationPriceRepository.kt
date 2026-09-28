package com.anpfuel.domain.repository

import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * Port for station-level price queries and retention cleanup (UC-007, BR-013).
 */
interface StationPriceRepository {

    suspend fun getStationPrices(
        state: BrazilianState,
        municipality: String,
        fuelProduct: FuelProduct,
        surveyWeek: SurveyWeek,
    ): List<StationPrice>

    suspend fun hasStationData(
        surveyWeek: SurveyWeek,
        state: BrazilianState,
        municipality: String,
    ): Boolean

    suspend fun deleteStationPricesOlderThanRetention(retentionWeeks: Int)
}
