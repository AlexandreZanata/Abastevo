package com.anpfuel.domain.sync

import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PriceTableSyncPlannerTest {

    private val weekA = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val weekB = SurveyWeek.fromIsoDates("2026-06-14", "2026-06-20")

    @Test
    fun selectsLatestSurveyWeekTables() {
        val discovered = listOf(
            table(weekA, PriceTableType.WEEKLY_SUMMARY, "a-summary"),
            table(weekA, PriceTableType.STATION_DETAIL, "a-station"),
            table(weekB, PriceTableType.WEEKLY_SUMMARY, "b-summary"),
            table(weekB, PriceTableType.STATION_DETAIL, "b-station"),
        )

        val latest = PriceTableSyncPlanner.selectLatestWeekTables(discovered)

        assertEquals(2, latest.size)
        assertEquals(weekB, latest.first().surveyWeek)
    }

    @Test
    fun selectsTargetSurveyWeekTables() {
        val discovered = listOf(
            table(weekA, PriceTableType.WEEKLY_SUMMARY, "a-summary"),
            table(weekA, PriceTableType.STATION_DETAIL, "a-station"),
            table(weekB, PriceTableType.WEEKLY_SUMMARY, "b-summary"),
            table(weekB, PriceTableType.STATION_DETAIL, "b-station"),
        )

        val targetWeek = PriceTableSyncPlanner.selectWeekTables(discovered, weekA)

        assertEquals(2, targetWeek.size)
        assertTrue(targetWeek.all { it.surveyWeek == weekA })
    }

    @Test
    fun selectWeekTablesReturnsEmptyWhenWeekMissing() {
        val discovered = listOf(
            table(weekB, PriceTableType.WEEKLY_SUMMARY, "b-summary"),
        )

        assertTrue(PriceTableSyncPlanner.selectWeekTables(discovered, weekA).isEmpty())
    }

    @Test
    fun resolvesSummaryOnlyWhenStationDetailDisabled() {
        val latestWeekTables = listOf(
            table(weekA, PriceTableType.WEEKLY_SUMMARY, "summary"),
            table(weekA, PriceTableType.STATION_DETAIL, "station"),
        )

        val resolved = PriceTableSyncPlanner.resolveTablesForSync(
            weekTables = latestWeekTables,
            syncStationDetail = false,
        )

        assertEquals(1, resolved.size)
        assertEquals(PriceTableType.WEEKLY_SUMMARY, resolved.single().tableType)
    }

    @Test
    fun resolvesSummaryAndStationWhenEnabled() {
        val latestWeekTables = listOf(
            table(weekA, PriceTableType.WEEKLY_SUMMARY, "summary"),
            table(weekA, PriceTableType.STATION_DETAIL, "station"),
        )

        val resolved = PriceTableSyncPlanner.resolveTablesForSync(
            weekTables = latestWeekTables,
            syncStationDetail = true,
        )

        assertEquals(2, resolved.size)
    }

    @Test
    fun resolvesSummaryOnlyWhenStationFileMissing() {
        val latestWeekTables = listOf(
            table(weekA, PriceTableType.WEEKLY_SUMMARY, "summary"),
        )

        val resolved = PriceTableSyncPlanner.resolveTablesForSync(
            weekTables = latestWeekTables,
            syncStationDetail = true,
        )

        assertEquals(1, resolved.size)
        assertEquals(PriceTableType.WEEKLY_SUMMARY, resolved.single().tableType)
    }

    @Test
    fun selectLatestWeekTablesReturnsEmptyForEmptyDiscovery() {
        assertTrue(PriceTableSyncPlanner.selectLatestWeekTables(emptyList()).isEmpty())
    }

    @Test
    fun selectWeekTablesFiltersBySurveyWeek() {
        val discovered = listOf(
            table(weekA, PriceTableType.WEEKLY_SUMMARY, "a-summary"),
            table(weekA, PriceTableType.STATION_DETAIL, "a-station"),
            table(weekB, PriceTableType.WEEKLY_SUMMARY, "b-summary"),
            table(weekB, PriceTableType.STATION_DETAIL, "b-station"),
        )

        val weekBTables = PriceTableSyncPlanner.selectWeekTables(discovered, weekB)

        assertEquals(2, weekBTables.size)
        assertTrue(weekBTables.all { it.surveyWeek == weekB })
    }

    @Test
    fun resolveTablesForSyncReturnsEmptyWhenSummaryMissing() {
        val latestWeekTables = listOf(
            table(weekA, PriceTableType.STATION_DETAIL, "station"),
        )

        val resolved = PriceTableSyncPlanner.resolveTablesForSync(
            weekTables = latestWeekTables,
            syncStationDetail = true,
        )

        assertTrue(resolved.isEmpty())
    }

    private fun table(
        surveyWeek: SurveyWeek,
        tableType: PriceTableType,
        suffix: String,
    ): PriceTable = PriceTable.create(
        surveyWeek = surveyWeek,
        tableType = tableType,
        sourceUrl = "https://example.com/$suffix.xlsx",
    )
}
