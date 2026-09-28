package com.anpfuel.domain.rule

import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * BR-006 — Default to the most recent successfully imported survey week by end date.
 */
object DefaultSurveyWeekRule {

    fun selectDefault(importedSurveys: List<PriceSurvey>): SurveyWeek? =
        importedSurveys
            .asSequence()
            .filter { it.hasSummaryData }
            .maxByOrNull { it.surveyWeek.endDate }
            ?.surveyWeek
}
