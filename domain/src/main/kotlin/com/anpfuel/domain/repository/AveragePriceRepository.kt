package com.anpfuel.domain.repository

import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * Port for municipality average price queries (UC-003, UC-005, UC-006).
 */
interface AveragePriceRepository {

    suspend fun getLatestImportedSurveyWeek(): SurveyWeek?

    suspend fun getPricesByMunicipality(
        state: BrazilianState,
        municipality: String,
        surveyWeek: SurveyWeek,
    ): List<AveragePrice>

    suspend fun getPriceHistory(
        state: BrazilianState,
        municipality: String,
        fuelProduct: FuelProduct,
    ): List<AveragePrice>

    suspend fun getStatesWithData(surveyWeek: SurveyWeek): List<BrazilianState>

    suspend fun getMunicipalitiesWithData(
        state: BrazilianState,
        surveyWeek: SurveyWeek,
    ): List<String>
}
