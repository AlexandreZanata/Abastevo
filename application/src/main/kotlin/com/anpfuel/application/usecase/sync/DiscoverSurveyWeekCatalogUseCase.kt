package com.anpfuel.application.usecase.sync

import com.anpfuel.application.error.AppError
import com.anpfuel.application.error.AppErrorResolver
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.repository.PriceTableSyncGateway

/**
 * UC-009 — Discover ordered ANP survey week catalog from the public listing page.
 */
sealed class DiscoverSurveyWeekCatalogOutcome {
    data class Success(
        val catalog: List<SurveyWeekCatalogEntry>,
    ) : DiscoverSurveyWeekCatalogOutcome()

    data class Failure(
        val error: AppError,
    ) : DiscoverSurveyWeekCatalogOutcome()
}

class DiscoverSurveyWeekCatalogUseCase(
    private val priceTableSyncGateway: PriceTableSyncGateway,
) {

    suspend operator fun invoke(): DiscoverSurveyWeekCatalogOutcome {
        return try {
            val catalog = priceTableSyncGateway.discoverSurveyWeekCatalog()
                .sortedByDescending { it.surveyWeek.endDate }
            if (catalog.isEmpty()) {
                DiscoverSurveyWeekCatalogOutcome.Failure(AppError.SyncParseError)
            } else {
                DiscoverSurveyWeekCatalogOutcome.Success(catalog)
            }
        } catch (error: DomainException) {
            throw error
        } catch (error: Throwable) {
            DiscoverSurveyWeekCatalogOutcome.Failure(AppErrorResolver.fromThrowable(error))
        }
    }
}
