package com.anpfuel.domain.repository

import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * Port for ANP [PriceTable] discovery, download metadata, and batch import (UC-001).
 */
interface PriceTableRepository {

    suspend fun getImportedPriceSurveys(): List<PriceSurvey>

    suspend fun findPriceTableByUrl(sourceUrl: String): PriceTable?

    suspend fun savePriceSurvey(priceSurvey: PriceSurvey)

    suspend fun saveDiscoveredPriceTable(priceTable: PriceTable)

    suspend fun importAveragePrices(prices: List<AveragePrice>)

    suspend fun importStationPrices(prices: List<StationPrice>)

    suspend fun countImportedSurveyWeeks(): Int

    suspend fun findPriceSurveyById(id: DomainId): PriceSurvey?

    suspend fun findPriceSurveyByWeek(surveyWeek: SurveyWeek): PriceSurvey?
}
