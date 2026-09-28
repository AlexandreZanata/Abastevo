package com.anpfuel.app.ui.weekpicker

import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class SurveyWeekCatalogGrouperTest {

    @Test
    fun groupsCatalogByYearNewestFirstPreservingEntryOrderWithinYear() {
        val week2026 = entry("2026-06-07", "2026-06-13")
        val week2026Older = entry("2026-05-31", "2026-06-06")
        val week2025 = entry("2025-12-21", "2025-12-27")

        val sections = groupSurveyWeekCatalogByYear(
            catalog = listOf(week2026, week2026Older, week2025),
        )

        assertEquals(2, sections.size)
        assertEquals(2026, sections[0].year)
        assertEquals(listOf(week2026, week2026Older), sections[0].entries)
        assertEquals(2025, sections[1].year)
        assertEquals(listOf(week2025), sections[1].entries)
    }

    @Test
    fun emptyCatalogReturnsNoSections() {
        assertEquals(emptyList<SurveyWeekCatalogSection>(), groupSurveyWeekCatalogByYear(emptyList()))
    }

    private fun entry(start: String, end: String): SurveyWeekCatalogEntry {
        val surveyWeek = SurveyWeek.fromIsoDates(start, end)
        return SurveyWeekCatalogEntry.create(
            surveyWeek = surveyWeek,
            summaryUrl = "https://example.com/resumo_$start.xlsx",
            stationUrl = "https://example.com/revendas_$start.xlsx",
        )
    }
}
