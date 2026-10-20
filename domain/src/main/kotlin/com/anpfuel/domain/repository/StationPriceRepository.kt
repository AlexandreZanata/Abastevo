package com.anpfuel.domain.repository

import com.anpfuel.domain.discovery.DiscoverySort
import com.anpfuel.domain.discovery.DiscoveryStationsRule
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

    /**
     * Bounded discovery window over the same eligible set: filtering,
     * ordering (with a stable identity tiebreak) and pagination happen
     * in the query, never by loading the whole set. A short [search]
     * matches everything, exactly like the in-memory rule.
     */
    suspend fun getStationPricesPage(
        state: BrazilianState,
        municipality: String,
        fuelProduct: FuelProduct,
        surveyWeek: SurveyWeek,
        search: String?,
        sort: DiscoverySort,
        page: Int,
        pageSize: Int,
    ): DiscoveryStationsRule.DiscoveryPage

    /** Targeted single-station read for detail/navigation; null when absent. */
    suspend fun findStationPrice(
        state: BrazilianState,
        municipality: String,
        fuelProduct: FuelProduct,
        surveyWeek: SurveyWeek,
        cnpjDigits: String,
    ): StationPrice?

    suspend fun hasStationData(
        surveyWeek: SurveyWeek,
        state: BrazilianState,
        municipality: String,
    ): Boolean

    suspend fun deleteStationPricesOlderThanRetention(retentionWeeks: Int)
}
