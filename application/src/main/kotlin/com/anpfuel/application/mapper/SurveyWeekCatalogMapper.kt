package com.anpfuel.application.mapper

import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.valueobject.PriceTableType

object SurveyWeekCatalogMapper {

    fun toPriceTables(entry: SurveyWeekCatalogEntry): List<PriceTable> = listOf(
        PriceTable.create(
            surveyWeek = entry.surveyWeek,
            tableType = PriceTableType.WEEKLY_SUMMARY,
            sourceUrl = entry.summaryUrl,
        ),
        PriceTable.create(
            surveyWeek = entry.surveyWeek,
            tableType = PriceTableType.STATION_DETAIL,
            sourceUrl = entry.stationUrl,
        ),
    )
}
