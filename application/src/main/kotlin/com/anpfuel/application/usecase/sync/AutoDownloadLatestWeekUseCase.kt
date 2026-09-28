package com.anpfuel.application.usecase.sync

import com.anpfuel.application.error.AppError
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.event.SyncRequestSource
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.SurveyWeekSelectionMode

/**
 * UC-009 / BR-020 — Auto-select catalog latest week and sync when preference is enabled.
 */
sealed class AutoDownloadLatestWeekOutcome {
    data object Disabled : AutoDownloadLatestWeekOutcome()

    data object UpToDate : AutoDownloadLatestWeekOutcome()

    data class Success(
        val syncResult: SyncPriceTablesResult,
    ) : AutoDownloadLatestWeekOutcome()

    data class Failed(
        val error: AppError,
    ) : AutoDownloadLatestWeekOutcome()
}

class AutoDownloadLatestWeekUseCase(
    private val userPreferencesRepository: UserPreferencesRepository,
    private val priceTableRepository: PriceTableRepository,
    private val discoverSurveyWeekCatalogUseCase: DiscoverSurveyWeekCatalogUseCase,
    private val selectWeekAndSyncUseCase: SelectWeekAndSyncUseCase,
) {

    suspend operator fun invoke(
        source: SyncRequestSource = SyncRequestSource.SCHEDULED,
    ): AutoDownloadLatestWeekOutcome {
        val preferences = userPreferencesRepository.getPreferences()
        if (!preferences.autoDownloadLatestWeek) {
            return AutoDownloadLatestWeekOutcome.Disabled
        }

        return when (val catalogOutcome = discoverSurveyWeekCatalogUseCase()) {
            is DiscoverSurveyWeekCatalogOutcome.Failure -> {
                AutoDownloadLatestWeekOutcome.Failed(catalogOutcome.error)
            }

            is DiscoverSurveyWeekCatalogOutcome.Success -> {
                val latestEntry = catalogOutcome.catalog.firstOrNull()
                    ?: return AutoDownloadLatestWeekOutcome.Failed(AppError.SyncParseError)

                val latestWeek = latestEntry.surveyWeek
                val importedSurvey = priceTableRepository.findPriceSurveyByWeek(latestWeek)
                if (
                    preferences.activeSurveyWeek == latestWeek &&
                    importedSurvey?.hasSummaryData == true
                ) {
                    return AutoDownloadLatestWeekOutcome.UpToDate
                }

                when (
                    val syncResult = selectWeekAndSyncUseCase(
                        catalogEntry = latestEntry,
                        selectionMode = SurveyWeekSelectionMode.LATEST,
                        source = source,
                    )
                ) {
                    is SelectWeekAndSyncResult.Failed -> {
                        AutoDownloadLatestWeekOutcome.Failed(syncResult.error)
                    }

                    is SelectWeekAndSyncResult.Success -> {
                        if (syncResult.syncResult.outcome == SyncJobOutcome.FAILED) {
                            AutoDownloadLatestWeekOutcome.Failed(
                                syncResult.syncResult.error ?: AppError.SyncNetworkError,
                            )
                        } else {
                            AutoDownloadLatestWeekOutcome.Success(syncResult.syncResult)
                        }
                    }
                }
            }
        }
    }
}
