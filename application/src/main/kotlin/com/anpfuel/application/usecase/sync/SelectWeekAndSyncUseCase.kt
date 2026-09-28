package com.anpfuel.application.usecase.sync

import com.anpfuel.application.error.AppError
import com.anpfuel.application.error.AppErrorResolver
import com.anpfuel.application.mapper.SurveyWeekCatalogMapper
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.event.SyncRequestSource
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.valueobject.SurveyWeekSelectionMode

sealed class SelectWeekAndSyncResult {
    data class Success(
        val syncResult: SyncPriceTablesResult,
    ) : SelectWeekAndSyncResult()

    data class Failed(
        val error: AppError,
    ) : SelectWeekAndSyncResult()
}

/**
 * UC-009 — Persist survey week choice and sync only the selected week (returning users).
 */
class SelectWeekAndSyncUseCase(
    private val selectSurveyWeekUseCase: SelectSurveyWeekUseCase,
    private val syncPriceTablesUseCase: SyncPriceTablesUseCase,
) {

    suspend operator fun invoke(
        catalogEntry: SurveyWeekCatalogEntry,
        selectionMode: SurveyWeekSelectionMode,
        source: SyncRequestSource = SyncRequestSource.MANUAL,
    ): SelectWeekAndSyncResult {
        return try {
            selectSurveyWeekUseCase(
                surveyWeek = catalogEntry.surveyWeek,
                selectionMode = selectionMode,
            )

            val syncResult = syncPriceTablesUseCase(
                source = source,
                targetSurveyWeek = catalogEntry.surveyWeek,
                preDiscoveredWeekTables = SurveyWeekCatalogMapper.toPriceTables(catalogEntry),
            )

            if (syncResult.outcome == SyncJobOutcome.FAILED) {
                SelectWeekAndSyncResult.Failed(
                    error = syncResult.error ?: AppError.SyncNetworkError,
                )
            } else {
                SelectWeekAndSyncResult.Success(syncResult = syncResult)
            }
        } catch (exception: DomainException) {
            SelectWeekAndSyncResult.Failed(
                error = AppErrorResolver.fromThrowable(exception),
            )
        }
    }
}
