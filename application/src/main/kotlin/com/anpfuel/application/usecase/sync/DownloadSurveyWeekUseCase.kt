package com.anpfuel.application.usecase.sync

import com.anpfuel.application.error.AppError
import com.anpfuel.application.error.AppErrorResolver
import com.anpfuel.application.mapper.SurveyWeekCatalogMapper
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.event.SyncRequestSource
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.SurveyWeekCatalogEntry

sealed class DownloadSurveyWeekResult {
    data class Success(
        val syncResult: SyncPriceTablesResult,
    ) : DownloadSurveyWeekResult()

    data class Failed(
        val error: AppError,
    ) : DownloadSurveyWeekResult()
}

/**
 * Downloads and imports a single catalog week without changing the active
 * survey week, so the picker can download several weeks in a row.
 */
class DownloadSurveyWeekUseCase(
    private val syncPriceTablesUseCase: SyncPriceTablesUseCase,
) {

    suspend operator fun invoke(
        catalogEntry: SurveyWeekCatalogEntry,
    ): DownloadSurveyWeekResult {
        return try {
            val syncResult = syncPriceTablesUseCase(
                source = SyncRequestSource.MANUAL,
                targetSurveyWeek = catalogEntry.surveyWeek,
                preDiscoveredWeekTables = SurveyWeekCatalogMapper.toPriceTables(catalogEntry),
            )

            if (syncResult.outcome == SyncJobOutcome.FAILED) {
                DownloadSurveyWeekResult.Failed(
                    error = syncResult.error ?: AppError.SyncNetworkError,
                )
            } else {
                DownloadSurveyWeekResult.Success(syncResult = syncResult)
            }
        } catch (exception: DomainException) {
            DownloadSurveyWeekResult.Failed(
                error = AppErrorResolver.fromThrowable(exception),
            )
        }
    }
}
