package com.anpfuel.domain.rule

import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class PreviousSurveyWeekRuleTest {

    private val currentWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val previousWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")
    private val olderWeek = SurveyWeek.fromIsoDates("2026-05-24", "2026-05-30")

    @Test
    fun returnsAdjacentImportedWeekBeforeCurrent() {
        val surveys = listOf(
            survey(currentWeek),
            survey(previousWeek),
            survey(olderWeek),
        )

        assertEquals(
            previousWeek,
            PreviousSurveyWeekRule.resolvePreviousWeek(currentWeek, surveys),
        )
    }

    @Test
    fun returnsNullWhenOnlyOneWeekImported() {
        assertNull(
            PreviousSurveyWeekRule.resolvePreviousWeek(
                currentWeek = currentWeek,
                importedSurveys = listOf(survey(currentWeek)),
            ),
        )
    }

    @Test
    fun ignoresSurveysWithoutSummaryData() {
        val surveys = listOf(
            survey(currentWeek),
            PriceSurvey.restore(
                id = DomainId.forSurveyWeek(previousWeek),
                surveyWeek = previousWeek,
                summaryImportedAt = null,
                stationImportedAt = java.time.Instant.parse("2026-06-01T10:00:00Z"),
            ),
        )

        assertNull(
            PreviousSurveyWeekRule.resolvePreviousWeek(currentWeek, surveys),
        )
    }

    private fun survey(surveyWeek: SurveyWeek): PriceSurvey =
        PriceSurvey.restore(
            id = DomainId.forSurveyWeek(surveyWeek),
            surveyWeek = surveyWeek,
            summaryImportedAt = java.time.Instant.parse("2026-06-14T10:00:00Z"),
            stationImportedAt = null,
        )
}
