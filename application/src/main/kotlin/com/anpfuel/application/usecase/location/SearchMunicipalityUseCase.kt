package com.anpfuel.application.usecase.location

import com.anpfuel.application.error.AppError
import com.anpfuel.domain.model.MunicipalitySearchResult
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.MunicipalitySearchRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.MinimumSearchLengthRule
import com.anpfuel.domain.rule.MunicipalityDataAvailabilityRule
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
        if (!MinimumSearchLengthRule.isSearchAllowed(query)) {
            return SearchMunicipalityOutcome.QueryTooShort
        }

        // MEX-04: location discovery never requires an imported ANP week.
        // Availability is separate enrichment; without a week every catalog
        // result is reported as unavailable, never as an error.
        val surveyWeek = resolveDisplaySurveyWeek()
        val surveyWeekId = surveyWeek?.let(DomainId::forSurveyWeek)

        val matches = municipalitySearchRepository.search(
            query = query.trim(),
            limit = limit,
        )
        if (matches.isEmpty()) {
            return SearchMunicipalityOutcome.NoResults()
        }

        val enrichment = municipalityCatalogRepository.enrichSearchResults(matches, surveyWeek)
        val results = enrichment.candidates.map { candidate ->
            val entry = candidate.entry
            val dataAvailability = when {
                entry == null ->
                    DataAvailability.NEVER_IN_ANP
                surveyWeekId == null ->
                    MunicipalityDataAvailabilityRule.resolveWithoutWeek(
                        entry = entry,
                        municipalitiesEverInAnp = enrichment.snapshot.everInAnp,
                    )
                else ->
                    MunicipalityDataAvailabilityRule.resolve(
                        entry = entry,
                        surveyWeekId = surveyWeekId,
                        municipalitiesWithDataThisWeek = enrichment.snapshot.withDataThisWeek,
                        municipalitiesEverInAnp = enrichment.snapshot.everInAnp,
                    )
            }
            candidate.result.copy(dataAvailability = dataAvailability)
        }

        return SearchMunicipalityOutcome.Success(results = results)
    }

    private suspend fun resolveDisplaySurveyWeek(): SurveyWeek? {
        val preferences = userPreferencesRepository.getPreferences()
        val importedSurveys = priceTableRepository.getImportedPriceSurveys()
        return ActiveSurveyWeekRule.resolveDisplayWeek(
            activeSurveyWeek = preferences.activeSurveyWeek,
            importedSurveys = importedSurveys,
        )
    }
}
