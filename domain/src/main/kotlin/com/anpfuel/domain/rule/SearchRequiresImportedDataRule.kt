package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException

/**
 * BR-005 — Search and location browse require at least one imported survey week.
 */
object SearchRequiresImportedDataRule {

    fun validate(importedSurveyWeekCount: Int) {
        if (importedSurveyWeekCount < 1) {
            throw DomainException(
                "BR-005: Search requires at least one imported SurveyWeek",
            )
        }
    }

    fun isSatisfied(importedSurveyWeekCount: Int): Boolean = importedSurveyWeekCount >= 1
}
