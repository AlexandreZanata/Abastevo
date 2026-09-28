package com.anpfuel.domain.rule

import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * Resolves the survey week immediately before the active week for UC-014 comparisons.
 */
object PreviousSurveyWeekRule {

    fun resolvePreviousWeek(
        currentWeek: SurveyWeek,
        importedSurveys: List<PriceSurvey>,
    ): SurveyWeek? {
        val importedWeeks = importedSurveys
            .asSequence()
            .filter { it.hasSummaryData }
            .map { it.surveyWeek }
            .distinct()
            .sortedByDescending { it.endDate }
            .toList()

        val currentIndex = importedWeeks.indexOf(currentWeek)
        if (currentIndex >= 0 && currentIndex + 1 < importedWeeks.size) {
            return importedWeeks[currentIndex + 1]
        }

        return importedWeeks
            .asSequence()
            .filter { it.endDate.isBefore(currentWeek.startDate) }
            .maxByOrNull { it.endDate }
    }
}
