package com.anpfuel.domain.rule

import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test
import java.time.Instant

class DefaultSurveyWeekRuleTest {

    @Test
    fun selectsMostRecentSummaryImportedWeekByEndDate() {
        val olderWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")
        val newerWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
        val olderSurvey = PriceSurvey.create(olderWeek).apply {
            markSummaryImported(Instant.parse("2026-06-07T10:00:00Z"))
        }
        val newerSurvey = PriceSurvey.create(newerWeek).apply {
            markSummaryImported(Instant.parse("2026-06-14T10:00:00Z"))
        }

        val selected = DefaultSurveyWeekRule.selectDefault(listOf(olderSurvey, newerSurvey))

        assertEquals(newerWeek, selected)
    }

    @Test
    fun ignoresSurveysWithoutSummaryData() {
        val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
        val surveyWithoutSummary = PriceSurvey.create(week)

        assertNull(DefaultSurveyWeekRule.selectDefault(listOf(surveyWithoutSummary)))
    }
}
