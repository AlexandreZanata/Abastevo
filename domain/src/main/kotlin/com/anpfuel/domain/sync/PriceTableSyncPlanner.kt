package com.anpfuel.domain.sync

import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * Selects which discovered [PriceTable] files belong to the latest survey week (UC-001).
 */
object PriceTableSyncPlanner {

    fun selectLatestWeekTables(discovered: List<PriceTable>): List<PriceTable> {
        val latestWeek = discovered.maxByOrNull { it.surveyWeek.endDate }?.surveyWeek
            ?: return emptyList()
        return selectWeekTables(discovered, latestWeek)
    }

    fun selectWeekTables(
        discovered: List<PriceTable>,
        surveyWeek: SurveyWeek,
    ): List<PriceTable> = discovered.filter { it.surveyWeek == surveyWeek }

    fun resolveTablesForSync(
        weekTables: List<PriceTable>,
        syncStationDetail: Boolean,
    ): List<PriceTable> {
        val summary = weekTables.firstOrNull { it.tableType == PriceTableType.WEEKLY_SUMMARY }
            ?: return emptyList()

        if (!syncStationDetail) {
            return listOf(summary)
        }

        val station = weekTables.firstOrNull { it.tableType == PriceTableType.STATION_DETAIL }
        return if (station == null) {
            listOf(summary)
        } else {
            listOf(summary, station)
        }
    }
}
