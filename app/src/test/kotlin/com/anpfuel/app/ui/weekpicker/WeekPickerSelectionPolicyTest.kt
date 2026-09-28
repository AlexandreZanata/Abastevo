package com.anpfuel.app.ui.weekpicker

import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class WeekPickerSelectionPolicyTest {

    private val latestWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val historicalWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")

    @Test
    fun requiresConfirmationForHistoricalWeekOnly() {
        val catalog = listOf(entry(latestWeek), entry(historicalWeek))

        assertFalse(WeekPickerSelectionPolicy.requiresConfirmation(catalog, entry(latestWeek)))
        assertTrue(WeekPickerSelectionPolicy.requiresConfirmation(catalog, entry(historicalWeek)))
    }

    @Test
    fun identifiesLatestCatalogEntry() {
        val catalog = listOf(entry(latestWeek), entry(historicalWeek))

        assertTrue(WeekPickerSelectionPolicy.isLatestCatalogEntry(catalog, entry(latestWeek)))
        assertFalse(WeekPickerSelectionPolicy.isLatestCatalogEntry(catalog, entry(historicalWeek)))
    }

    private fun entry(surveyWeek: SurveyWeek): SurveyWeekCatalogEntry =
        SurveyWeekCatalogEntry.create(
            surveyWeek = surveyWeek,
            summaryUrl = "https://example.com/resumo_${surveyWeek.startDate}.xlsx",
            stationUrl = "https://example.com/revendas_${surveyWeek.startDate}.xlsx",
        )
}
