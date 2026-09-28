package com.anpfuel.application.usecase.price

import com.anpfuel.application.error.AppError
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.ActiveSurveyWeekRule
import com.anpfuel.domain.rule.EmptyMunicipalityResultRule
import com.anpfuel.domain.rule.SearchRequiresImportedDataRule
import com.anpfuel.domain.rule.StationDetailOptInRule
import com.anpfuel.domain.rule.StationPriceOrderingRule
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.exception.DomainException

/**
 * UC-007 — Loads station-level prices from local cache sorted by ascending price.
 */
sealed class StationPricesOutcome {
    data class Success(
        val surveyWeek: SurveyWeek,
        val state: BrazilianState,
        val municipality: String,
        val fuelProduct: FuelProduct,
        val stations: List<StationPrice>,
    ) : StationPricesOutcome() {
        val isEmpty: Boolean
            get() = EmptyMunicipalityResultRule.shouldReturnEmptyList(stations.size)
    }

    data class StationDetailMissing(
        val error: AppError = AppError.StationDetailNotSynced,
        val requiresOnDemandDownload: Boolean,
    ) : StationPricesOutcome()
}

class GetStationPricesUseCase(
    private val stationPriceRepository: StationPriceRepository,
    private val priceTableRepository: PriceTableRepository,
    private val userPreferencesRepository: UserPreferencesRepository,
) {

    suspend operator fun invoke(
        fuelProduct: FuelProduct? = null,
        state: BrazilianState? = null,
        municipality: String? = null,
        surveyWeek: SurveyWeek? = null,
    ): StationPricesOutcome {
        SearchRequiresImportedDataRule.validate(
            priceTableRepository.countImportedSurveyWeeks(),
        )

        val preferences = userPreferencesRepository.getPreferences()
        val resolvedFuelProduct = fuelProduct ?: preferences.preferredFuelProduct
            ?: throw DomainException("Fuel product is not set")
        val resolvedState = state ?: preferences.preferredState
            ?: throw DomainException("Preferred state is not set")
        val resolvedMunicipality = municipality?.trim()?.takeIf { it.isNotBlank() }
            ?: preferences.preferredMunicipality?.trim()?.takeIf { it.isNotBlank() }
            ?: throw DomainException("Preferred municipality is not set")
        val resolvedSurveyWeek = surveyWeek ?: resolveDisplaySurveyWeek()

        val hasLocalStationData = stationPriceRepository.hasStationData(
            surveyWeek = resolvedSurveyWeek,
            state = resolvedState,
            municipality = resolvedMunicipality,
        )
        if (!hasLocalStationData) {
            if (isStationDetailImported(resolvedSurveyWeek)) {
                return StationPricesOutcome.Success(
                    surveyWeek = resolvedSurveyWeek,
                    state = resolvedState,
                    municipality = resolvedMunicipality,
                    fuelProduct = resolvedFuelProduct,
                    stations = emptyList(),
                )
            }
            return StationPricesOutcome.StationDetailMissing(
                requiresOnDemandDownload = StationDetailOptInRule.requiresOnDemandDownload(
                    syncStationDetail = preferences.syncStationDetail,
                    hasLocalStationData = false,
                ),
            )
        }

        val stations = StationPriceOrderingRule.orderByPriceAscending(
            stationPriceRepository.getStationPrices(
                state = resolvedState,
                municipality = resolvedMunicipality,
                fuelProduct = resolvedFuelProduct,
                surveyWeek = resolvedSurveyWeek,
            ),
        )

        return StationPricesOutcome.Success(
            surveyWeek = resolvedSurveyWeek,
            state = resolvedState,
            municipality = resolvedMunicipality,
            fuelProduct = resolvedFuelProduct,
            stations = stations,
        )
    }

    private suspend fun resolveDisplaySurveyWeek(): SurveyWeek {
        val preferences = userPreferencesRepository.getPreferences()
        val importedSurveys = priceTableRepository.getImportedPriceSurveys()
        return ActiveSurveyWeekRule.resolveDisplayWeek(
            activeSurveyWeek = preferences.activeSurveyWeek,
            importedSurveys = importedSurveys,
        ) ?: throw DomainException("BR-006: No successfully imported SurveyWeek is available")
    }

    private suspend fun isStationDetailImported(surveyWeek: SurveyWeek): Boolean =
        priceTableRepository.findPriceSurveyByWeek(surveyWeek)?.hasStationData == true
}
