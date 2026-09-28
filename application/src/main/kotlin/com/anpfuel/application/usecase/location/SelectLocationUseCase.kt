package com.anpfuel.application.usecase.location

import com.anpfuel.domain.event.CitySelected
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.EmptyMunicipalityResultRule
import com.anpfuel.domain.rule.MunicipalityDataAvailabilityRule
import com.anpfuel.domain.rule.SearchRequiresImportedDataRule
import com.anpfuel.domain.rule.ActiveSurveyWeekRule
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.SurveyWeek

data class LocationStatesResult(
    val surveyWeek: SurveyWeek,
    val states: List<BrazilianState>,
)

data class CatalogMunicipalityItem(
    val municipality: String,
    val dataAvailability: DataAvailability,
)

data class LocationMunicipalitiesResult(
    val surveyWeek: SurveyWeek,
    val state: BrazilianState,
    val municipalities: List<CatalogMunicipalityItem>,
) {
    val isEmpty: Boolean
        get() = EmptyMunicipalityResultRule.shouldReturnEmptyList(municipalities.size)
}

data class PreferredLocation(
    val state: BrazilianState,
    val municipality: String,
)

data class SelectLocationResult(
    val event: CitySelected,
    val preferences: UserPreferences,
)

class SelectLocationUseCase(
    private val municipalityCatalogRepository: MunicipalityCatalogRepository,
    private val priceTableRepository: PriceTableRepository,
    private val userPreferencesRepository: UserPreferencesRepository,
    private val eventPublisher: DomainEventPublisher,
) {

    suspend fun getStatesWithData(): LocationStatesResult {
        requireImportedData()
        val surveyWeek = resolveDisplaySurveyWeek()
        val states = municipalityCatalogRepository.getCatalogStates()

        return LocationStatesResult(
            surveyWeek = surveyWeek,
            states = states,
        )
    }

    suspend fun getMunicipalities(state: BrazilianState): LocationMunicipalitiesResult {
        requireImportedData()
        val surveyWeek = resolveDisplaySurveyWeek()
        val surveyWeekId = DomainId.forSurveyWeek(surveyWeek)
        val entries = municipalityCatalogRepository.getCatalogMunicipalities(state)
        val municipalitiesWithDataThisWeek =
            municipalityCatalogRepository.getLocationKeysWithDataForWeek(surveyWeek)
        val municipalitiesEverInAnp = municipalityCatalogRepository.getLocationKeysEverInAnp()
        val municipalities = entries.map { entry ->
            CatalogMunicipalityItem(
                municipality = entry.municipality,
                dataAvailability = MunicipalityDataAvailabilityRule.resolve(
                    entry = entry,
                    surveyWeekId = surveyWeekId,
                    municipalitiesWithDataThisWeek = municipalitiesWithDataThisWeek,
                    municipalitiesEverInAnp = municipalitiesEverInAnp,
                ),
            )
        }

        return LocationMunicipalitiesResult(
            surveyWeek = surveyWeek,
            state = state,
            municipalities = municipalities,
        )
    }

    suspend fun getPreferredLocation(): PreferredLocation? {
        val preferences = userPreferencesRepository.getPreferences()
        val state = preferences.preferredState ?: return null
        val municipality = preferences.preferredMunicipality ?: return null

        return PreferredLocation(
            state = state,
            municipality = municipality,
        )
    }

    suspend fun selectLocation(
        state: BrazilianState,
        municipality: String,
    ): SelectLocationResult {
        requireImportedData()
        val surveyWeek = resolveDisplaySurveyWeek()
        validateMunicipalitySelection(
            state = state,
            municipality = municipality,
        )

        val currentPreferences = userPreferencesRepository.getPreferences()
        val normalizedMunicipality = municipality.trim()
        val updatedPreferences = currentPreferences.copy(
            preferredState = state,
            preferredMunicipality = normalizedMunicipality,
        )
        userPreferencesRepository.savePreferences(updatedPreferences)

        val event = CitySelected.create(
            payload = CitySelected.Payload(
                municipality = normalizedMunicipality,
                state = state,
                surveyWeekId = DomainId.forSurveyWeek(surveyWeek),
            ),
        )
        eventPublisher.publish(event)

        return SelectLocationResult(
            event = event,
            preferences = updatedPreferences,
        )
    }

    private suspend fun requireImportedData() {
        SearchRequiresImportedDataRule.validate(
            priceTableRepository.countImportedSurveyWeeks(),
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

    private suspend fun validateMunicipalitySelection(
        state: BrazilianState,
        municipality: String,
    ) {
        val normalizedMunicipality = municipality.trim()
        if (normalizedMunicipality.isBlank()) {
            throw DomainException("Municipality must not be blank")
        }

        val catalogEntry = municipalityCatalogRepository.findCatalogEntry(
            state = state,
            municipality = normalizedMunicipality,
        )
        if (catalogEntry == null) {
            throw DomainException(
                "Selected municipality is not in the national catalog for ${state.abbreviation}",
            )
        }
    }
}
