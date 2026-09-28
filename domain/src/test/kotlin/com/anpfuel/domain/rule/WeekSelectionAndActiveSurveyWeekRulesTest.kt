package com.anpfuel.domain.rule

import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.time.Instant

class WeekSelectionAndActiveSurveyWeekRulesTest {

    private val olderWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")
    private val latestWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

    @Test
    fun br018RequiresWeekSelectionWhenActiveWeekMissingAndAutoDownloadDisabled() {
        assertTrue(
            WeekSelectionBeforeSyncRule.requiresWeekSelection(
                activeSurveyWeek = null,
                autoDownloadLatestWeek = false,
            ),
        )
    }

    @Test
    fun br018SkipsPickerWhenActiveWeekSet() {
        assertTrue(!WeekSelectionBeforeSyncRule.requiresWeekSelection(activeSurveyWeek = latestWeek))
    }

    @Test
    fun br019UsesActiveWeekWhenImported() {
        val surveys = listOf(
            importedSurvey(olderWeek),
            importedSurvey(latestWeek),
        )

        val resolved = ActiveSurveyWeekRule.resolveDisplayWeek(
            activeSurveyWeek = olderWeek,
            importedSurveys = surveys,
        )

        assertEquals(olderWeek, resolved)
    }

    @Test
    fun br019FallsBackToBr006WhenActiveWeekNotImported() {
        val surveys = listOf(importedSurvey(latestWeek))

        val resolved = ActiveSurveyWeekRule.resolveDisplayWeek(
            activeSurveyWeek = olderWeek,
            importedSurveys = surveys,
        )

        assertEquals(latestWeek, resolved)
    }

    @Test
    fun br019FallsBackWhenActiveWeekNull() {
        val surveys = listOf(importedSurvey(olderWeek))

        val resolved = ActiveSurveyWeekRule.resolveDisplayWeek(
            activeSurveyWeek = null,
            importedSurveys = surveys,
        )

        assertEquals(olderWeek, resolved)
    }

    @Test
    fun br019ReturnsNullWhenNoImportedSummary() {
        val resolved = ActiveSurveyWeekRule.resolveDisplayWeek(
            activeSurveyWeek = latestWeek,
            importedSurveys = emptyList(),
        )

        assertNull(resolved)
    }

    private fun importedSurvey(week: SurveyWeek): PriceSurvey {
        val survey = PriceSurvey.create(week)
        survey.markSummaryImported(Instant.parse("2026-06-14T12:00:00Z"))
        return survey
    }
}
