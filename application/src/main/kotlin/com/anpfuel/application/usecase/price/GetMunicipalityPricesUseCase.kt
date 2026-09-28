package com.anpfuel.application.usecase.price

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.repository.AveragePriceRepository
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.ActiveSurveyWeekRule
import com.anpfuel.domain.rule.EmptyMunicipalityResultRule
import com.anpfuel.domain.rule.SearchRequiresImportedDataRule
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * UC-005 — Loads municipality average prices from the local cache (BR-004).
 */
data class MunicipalityPricesResult(
    val surveyWeek: SurveyWeek,
    val state: BrazilianState,
    val municipality: String,
    val prices: List<AveragePrice>,
    val dataAvailability: DataAvailability,
    val operationalNote: String? = null,
) {
    val isEmpty: Boolean
        get() = EmptyMunicipalityResultRule.shouldReturnEmptyList(prices.size)
}

class GetMunicipalityPricesUseCase(
    private val averagePriceRepository: AveragePriceRepository,
    private val municipalityCatalogRepository: MunicipalityCatalogRepository,
    private val priceTableRepository: PriceTableRepository,
    private val userPreferencesRepository: UserPreferencesRepository,
) {

    suspend operator fun invoke(
        state: BrazilianState? = null,
        municipality: String? = null,
        surveyWeek: SurveyWeek? = null,
    ): MunicipalityPricesResult {
        SearchRequiresImportedDataRule.validate(
            priceTableRepository.countImportedSurveyWeeks(),
        )

        val resolvedState = state ?: requirePreferredState()
        val resolvedMunicipality = municipality?.trim()?.takeIf { it.isNotBlank() }
            ?: requirePreferredMunicipality()
        val resolvedSurveyWeek = surveyWeek ?: resolveDisplaySurveyWeek()

        val prices = averagePriceRepository.getPricesByMunicipality(
            state = resolvedState,
            municipality = resolvedMunicipality,
            surveyWeek = resolvedSurveyWeek,
        )

        val dataAvailability = if (prices.isNotEmpty()) {
            DataAvailability.HAS_DATA
        } else {
            municipalityCatalogRepository.resolveDataAvailability(
                state = resolvedState,
                municipality = resolvedMunicipality,
                surveyWeek = resolvedSurveyWeek,
            )
        }

        return MunicipalityPricesResult(
            surveyWeek = resolvedSurveyWeek,
            state = resolvedState,
            municipality = resolvedMunicipality,
            prices = prices,
            dataAvailability = dataAvailability,
            operationalNote = municipalityCatalogRepository.getOperationalNote(resolvedSurveyWeek),
        )
    }

    private suspend fun requirePreferredState(): BrazilianState {
        return userPreferencesRepository.getPreferences().preferredState
            ?: throw DomainException("Preferred state is not set")
    }

    private suspend fun requirePreferredMunicipality(): String {
        return userPreferencesRepository.getPreferences().preferredMunicipality?.trim()?.takeIf { it.isNotBlank() }
            ?: throw DomainException("Preferred municipality is not set")
    }

    private suspend fun resolveDisplaySurveyWeek(): SurveyWeek {
        val preferences = userPreferencesRepository.getPreferences()
        val importedSurveys = priceTableRepository.getImportedPriceSurveys()
        return ActiveSurveyWeekRule.resolveDisplayWeek(
            activeSurveyWeek = preferences.activeSurveyWeek,
            importedSurveys = importedSurveys,
        ) ?: throw DomainException("BR-006: No successfully imported SurveyWeek is available")
    }
}
