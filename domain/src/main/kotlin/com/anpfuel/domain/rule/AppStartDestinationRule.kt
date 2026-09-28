package com.anpfuel.domain.rule

import com.anpfuel.domain.model.AppStartDestination
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * BR-018, BR-019 — Resolve cold-start destination for returning users (UC-009 A1).
 */
object AppStartDestinationRule {

    fun resolve(
        onboardingCompleted: Boolean,
        activeSurveyWeek: SurveyWeek?,
        importedSurveys: List<PriceSurvey>,
        autoDownloadLatestWeek: Boolean,
    ): AppStartDestination {
        if (!onboardingCompleted) {
            return AppStartDestination.ONBOARDING
        }

        if (AutoDownloadLatestWeekRule.skipsWeekPickerOnColdStart(autoDownloadLatestWeek)) {
            return AppStartDestination.HOME
        }

        if (WeekSelectionBeforeSyncRule.requiresWeekSelection(activeSurveyWeek, autoDownloadLatestWeek)) {
            return AppStartDestination.WEEK_PICKER
        }

        val activeWeekImported = importedSurveys.any {
            it.surveyWeek == activeSurveyWeek && it.hasSummaryData
        }

        return if (activeWeekImported) {
            AppStartDestination.HOME
        } else {
            AppStartDestination.WEEK_PICKER
        }
    }
}
