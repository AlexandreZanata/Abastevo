package com.anpfuel.domain.rule

import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.state.DataReadinessState
import com.anpfuel.domain.state.SyncJobState
import java.time.LocalDate

/**
 * Derives user-visible [DataReadinessState] from local storage and sync job state.
 */
object DataReadinessRule {

    fun resolve(
        importedWeekCount: Int,
        syncJobState: SyncJobState,
        latestSurvey: PriceSurvey?,
        today: LocalDate,
    ): DataReadinessState {
        if (importedWeekCount == 0) {
            return DataReadinessState.EMPTY
        }

        if (syncJobState.isActive) {
            return DataReadinessState.SYNCING
        }

        if (syncJobState == SyncJobState.FAILED) {
            return DataReadinessState.ERROR
        }

        if (latestSurvey == null || !latestSurvey.hasSummaryData) {
            return DataReadinessState.EMPTY
        }

        if (!latestSurvey.hasStationData) {
            return DataReadinessState.PARTIAL
        }

        if (SurveyWeekFreshnessRule.isStale(latestSurvey.surveyWeek.endDate, today)) {
            return DataReadinessState.STALE
        }

        return DataReadinessState.READY
    }
}
