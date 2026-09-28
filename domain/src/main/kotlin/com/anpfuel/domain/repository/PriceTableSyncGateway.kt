package com.anpfuel.domain.repository

import com.anpfuel.domain.event.PriceTableImported
import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.model.SurveyWeekCatalogEntry

/**
 * Port for ANP price table discovery, download, and file import (UC-001).
 */
interface PriceTableSyncGateway {

    suspend fun discoverPriceTables(): List<PriceTable>

    suspend fun discoverSurveyWeekCatalog(): List<SurveyWeekCatalogEntry>

    suspend fun downloadPriceTable(priceTable: PriceTable): PriceTable

    suspend fun importWeeklySummary(priceTable: PriceTable): PriceTableImported.Payload

    suspend fun importStationDetail(priceTable: PriceTable): PriceTableImported.Payload
}
