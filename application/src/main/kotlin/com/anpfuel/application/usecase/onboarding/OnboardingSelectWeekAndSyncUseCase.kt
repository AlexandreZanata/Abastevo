package com.anpfuel.application.usecase.onboarding

import com.anpfuel.application.error.AppError
import com.anpfuel.application.error.AppErrorResolver
import com.anpfuel.application.mapper.SurveyWeekCatalogMapper
import com.anpfuel.application.usecase.sync.SelectSurveyWeekUseCase
import com.anpfuel.application.usecase.sync.SyncPriceTablesResult
import com.anpfuel.application.usecase.sync.SyncPriceTablesUseCase
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.event.SyncRequestSource
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.valueobject.SurveyWeekSelectionMode
import kotlinx.coroutines.delay

sealed class OnboardingSelectWeekAndSyncResult {
    data class SyncFailed(
        val error: AppError,
    ) : OnboardingSelectWeekAndSyncResult()

    data class Completed(
        val syncResult: SyncPriceTablesResult,
        val onboardingResult: CompleteOnboardingResult,
    ) : OnboardingSelectWeekAndSyncResult()
}

/**
 * UC-002 (v2) — After week selection in onboarding, persist choice and sync that week only.
 */
class OnboardingSelectWeekAndSyncUseCase(
    private val selectSurveyWeekUseCase: SelectSurveyWeekUseCase,
    private val syncPriceTablesUseCase: SyncPriceTablesUseCase,
    private val completeOnboardingUseCase: CompleteOnboardingUseCase,
) {

    suspend operator fun invoke(
        catalogEntry: SurveyWeekCatalogEntry,
        selectionMode: SurveyWeekSelectionMode,
        source: SyncRequestSource = SyncRequestSource.FIRST_LAUNCH,
    ): OnboardingSelectWeekAndSyncResult {
        return try {
            selectSurveyWeekUseCase(
                surveyWeek = catalogEntry.surveyWeek,
                selectionMode = selectionMode,
            )

            val syncResult = syncWithFirstLaunchRetry(
                catalogEntry = catalogEntry,
                source = source,
            )

            if (syncResult.outcome == SyncJobOutcome.FAILED) {
                OnboardingSelectWeekAndSyncResult.SyncFailed(
                    error = syncResult.error ?: AppError.SyncNetworkError,
                )
            } else {
                val onboardingResult = completeOnboardingUseCase.completeAfterSync(syncResult)
                OnboardingSelectWeekAndSyncResult.Completed(
                    syncResult = syncResult,
                    onboardingResult = onboardingResult,
                )
            }
        } catch (exception: DomainException) {
            OnboardingSelectWeekAndSyncResult.SyncFailed(
                error = AppErrorResolver.fromThrowable(exception),
            )
        }
    }

    private suspend fun syncWithFirstLaunchRetry(
        catalogEntry: SurveyWeekCatalogEntry,
        source: SyncRequestSource,
    ): SyncPriceTablesResult {
        val preDiscoveredWeekTables = SurveyWeekCatalogMapper.toPriceTables(catalogEntry)
        val firstResult = syncPriceTablesUseCase(
            source = source,
            targetSurveyWeek = catalogEntry.surveyWeek,
            preDiscoveredWeekTables = preDiscoveredWeekTables,
        )
        if (
            source != SyncRequestSource.FIRST_LAUNCH ||
            firstResult.outcome != SyncJobOutcome.FAILED
        ) {
            return firstResult
        }

        delay(FIRST_LAUNCH_RETRY_DELAY_MS)
        return syncPriceTablesUseCase(
            source = source,
            targetSurveyWeek = catalogEntry.surveyWeek,
            preDiscoveredWeekTables = preDiscoveredWeekTables,
        )
    }

    companion object {
        internal const val FIRST_LAUNCH_RETRY_DELAY_MS = 750L
    }
}
