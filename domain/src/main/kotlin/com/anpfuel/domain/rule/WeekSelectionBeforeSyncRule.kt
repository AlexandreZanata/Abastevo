package com.anpfuel.domain.rule

import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * BR-018 — Week selection before sync when auto-download latest week is disabled (BR-020).
 */
object WeekSelectionBeforeSyncRule {

    fun requiresWeekSelection(
        activeSurveyWeek: SurveyWeek?,
        autoDownloadLatestWeek: Boolean = false,
    ): Boolean = AutoDownloadLatestWeekRule.requiresManualWeekSelection(
        activeSurveyWeek = activeSurveyWeek,
        autoDownloadLatestWeek = autoDownloadLatestWeek,
    )
}
