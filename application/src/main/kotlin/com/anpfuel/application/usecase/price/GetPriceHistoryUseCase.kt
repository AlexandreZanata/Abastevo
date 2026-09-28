package com.anpfuel.application.usecase.price

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.repository.AveragePriceRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.PriceHistoryAvailabilityRule
import com.anpfuel.domain.rule.PriceHistoryOrderingRule
import com.anpfuel.domain.rule.SearchRequiresImportedDataRule
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct

/**
 * UC-006 — Read-only municipality fuel price history from immutable local records (BR-003).
 */
sealed class PriceHistoryOutcome {
    data object HistoryDisabled : PriceHistoryOutcome()

    data object InsufficientData : PriceHistoryOutcome()

    data class Success(
        val state: BrazilianState,
        val municipality: String,
        val fuelProduct: FuelProduct,
        val entries: List<AveragePrice>,
    ) : PriceHistoryOutcome()
}

class GetPriceHistoryUseCase(
    private val averagePriceRepository: AveragePriceRepository,
    private val priceTableRepository: PriceTableRepository,
    private val userPreferencesRepository: UserPreferencesRepository,
) {

    suspend operator fun invoke(
        fuelProduct: FuelProduct? = null,
        state: BrazilianState? = null,
        municipality: String? = null,
    ): PriceHistoryOutcome {
        val preferences = userPreferencesRepository.getPreferences()
        if (!preferences.showPriceHistory) {
            return PriceHistoryOutcome.HistoryDisabled
        }

        SearchRequiresImportedDataRule.validate(
            priceTableRepository.countImportedSurveyWeeks(),
        )

        val resolvedFuelProduct = fuelProduct ?: preferences.preferredFuelProduct
            ?: throw DomainException("Fuel product is not set")
        val resolvedState = state ?: preferences.preferredState
            ?: throw DomainException("Preferred state is not set")
        val resolvedMunicipality = municipality?.trim()?.takeIf { it.isNotBlank() }
            ?: preferences.preferredMunicipality?.trim()?.takeIf { it.isNotBlank() }
            ?: throw DomainException("Preferred municipality is not set")

        val history = averagePriceRepository.getPriceHistory(
            state = resolvedState,
            municipality = resolvedMunicipality,
            fuelProduct = resolvedFuelProduct,
        )

        if (!PriceHistoryAvailabilityRule.hasSufficientHistory(history)) {
            return PriceHistoryOutcome.InsufficientData
        }

        return PriceHistoryOutcome.Success(
            state = resolvedState,
            municipality = resolvedMunicipality,
            fuelProduct = resolvedFuelProduct,
            entries = PriceHistoryOrderingRule.orderBySurveyWeekStartDate(history),
        )
    }
}
