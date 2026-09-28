package com.anpfuel.domain.rule

import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * BR-020 — Latest survey week is auto-selected and synced unless the user opts out.
 */
object AutoDownloadLatestWeekRule {

    fun requiresManualWeekSelection(
        activeSurveyWeek: SurveyWeek?,
        autoDownloadLatestWeek: Boolean,
    ): Boolean = !autoDownloadLatestWeek && activeSurveyWeek == null

    fun shouldResolveSyncTargetFromCatalogLatest(
        autoDownloadLatestWeek: Boolean,
        explicitTargetSurveyWeek: SurveyWeek?,
    ): Boolean = autoDownloadLatestWeek && explicitTargetSurveyWeek == null

    fun skipsWeekPickerOnColdStart(autoDownloadLatestWeek: Boolean): Boolean =
        autoDownloadLatestWeek
}
