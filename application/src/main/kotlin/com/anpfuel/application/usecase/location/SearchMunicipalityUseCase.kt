package com.anpfuel.application.usecase.location

import com.anpfuel.application.error.AppError
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.MunicipalitySearchResult
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.MunicipalitySearchRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.MinimumSearchLengthRule
import com.anpfuel.domain.rule.MunicipalityDataAvailabilityRule
import com.anpfuel.domain.rule.SearchRequiresImportedDataRule
import com.anpfuel.domain.rule.ActiveSurveyWeekRule
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * UC-004 — FTS municipality search. Debounce (300 ms) is handled in the ViewModel; this use case is pure.
 */
sealed class SearchMunicipalityOutcome {
    data object QueryTooShort : SearchMunicipalityOutcome()

    data class Success(
        val results: List<MunicipalitySearchResult>,
    ) : SearchMunicipalityOutcome()

    data class NoResults(
        val error: AppError = AppError.SearchNoResults,
    ) : SearchMunicipalityOutcome()
}

class SearchMunicipalityUseCase(
    private val municipalitySearchRepository: MunicipalitySearchRepository,
    private val municipalityCatalogRepository: MunicipalityCatalogRepository,
    private val priceTableRepository: PriceTableRepository,
    private val userPreferencesRepository: UserPreferencesRepository,
) {

    suspend fun search(
        query: String,
        limit: Int = MunicipalitySearchRepository.DEFAULT_LIMIT,
    ): SearchMunicipalityOutcome {
        SearchRequiresImportedDataRule.validate(
            priceTableRepository.countImportedSurveyWeeks(),
        )

        if (!MinimumSearchLengthRule.isSearchAllowed(query)) {
            return SearchMunicipalityOutcome.QueryTooShort
        }

        val surveyWeek = resolveDisplaySurveyWeek()
        val surveyWeekId = DomainId.forSurveyWeek(surveyWeek)
        val municipalitiesWithDataThisWeek =
            municipalityCatalogRepository.getLocationKeysWithDataForWeek(surveyWeek)
        val municipalitiesEverInAnp = municipalityCatalogRepository.getLocationKeysEverInAnp()

        val results = municipalitySearchRepository.search(
            query = query.trim(),
            limit = limit,
        ).map { result ->
            val entry = municipalityCatalogRepository.findCatalogEntry(
                state = result.state,
                municipality = result.municipality,
            )
            val dataAvailability = if (entry == null) {
                DataAvailability.NEVER_IN_ANP
            } else {
                MunicipalityDataAvailabilityRule.resolve(
                    entry = entry,
                    surveyWeekId = surveyWeekId,
                    municipalitiesWithDataThisWeek = municipalitiesWithDataThisWeek,
                    municipalitiesEverInAnp = municipalitiesEverInAnp,
                )
            }
            result.copy(dataAvailability = dataAvailability)
        }

        if (results.isEmpty()) {
            return SearchMunicipalityOutcome.NoResults()
        }

        return SearchMunicipalityOutcome.Success(results = results)
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
