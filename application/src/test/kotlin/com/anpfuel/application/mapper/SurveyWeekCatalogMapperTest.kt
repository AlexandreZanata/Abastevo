package com.anpfuel.application.mapper

import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class SurveyWeekCatalogMapperTest {

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

    @Test
    fun mapsCatalogEntryToSummaryAndStationPriceTables() {
        val entry = SurveyWeekCatalogEntry.create(
            surveyWeek = surveyWeek,
            summaryUrl = "https://example.com/summary.xlsx",
            stationUrl = "https://example.com/station.xlsx",
        )

        val tables = SurveyWeekCatalogMapper.toPriceTables(entry)

        assertEquals(2, tables.size)
        assertEquals(PriceTableType.WEEKLY_SUMMARY, tables[0].tableType)
        assertEquals(entry.summaryUrl, tables[0].sourceUrl)
        assertEquals(PriceTableType.STATION_DETAIL, tables[1].tableType)
        assertEquals(entry.stationUrl, tables[1].sourceUrl)
    }
}
