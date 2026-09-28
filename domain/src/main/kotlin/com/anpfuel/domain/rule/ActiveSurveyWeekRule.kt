package com.anpfuel.domain.rule

import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * BR-019 — Active survey week overrides BR-006 default when imported locally.
 */
object ActiveSurveyWeekRule {

    fun resolveDisplayWeek(
        activeSurveyWeek: SurveyWeek?,
        importedSurveys: List<PriceSurvey>,
    ): SurveyWeek? {
        if (activeSurveyWeek != null) {
            val hasImportedSummary = importedSurveys.any {
                it.surveyWeek == activeSurveyWeek && it.hasSummaryData
            }
            if (hasImportedSummary) {
                return activeSurveyWeek
            }
        }
        return DefaultSurveyWeekRule.selectDefault(importedSurveys)
    }
}
