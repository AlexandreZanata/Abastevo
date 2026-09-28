package com.anpfuel.application.usecase.location

import com.anpfuel.domain.event.CitySelected
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class SelectLocationUseCaseTest {

    private val municipalityCatalogRepository = mockk<MunicipalityCatalogRepository>()
    private val priceTableRepository = mockk<PriceTableRepository>()
    private val userPreferencesRepository = mockk<UserPreferencesRepository>()
    private val eventPublisher = mockk<DomainEventPublisher>()

    private lateinit var useCase: SelectLocationUseCase

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val state = BrazilianState.SAO_PAULO
    private val municipality = "SÃO PAULO"
    private val catalogEntry = MunicipalityCatalogEntry(
        state = state,
        municipality = municipality,
        ibgeCode = "3550308",
    )

    @BeforeEach
    fun setUp() {
        useCase = SelectLocationUseCase(
            municipalityCatalogRepository = municipalityCatalogRepository,
            priceTableRepository = priceTableRepository,
            userPreferencesRepository = userPreferencesRepository,
            eventPublisher = eventPublisher,
        )

        coEvery { priceTableRepository.countImportedSurveyWeeks() } returns 1
        coEvery { priceTableRepository.getImportedPriceSurveys() } returns listOf(
            PriceSurvey.restore(
                id = DomainId.forSurveyWeek(surveyWeek),
                surveyWeek = surveyWeek,
                summaryImportedAt = java.time.Instant.parse("2026-06-14T10:00:00Z"),
                stationImportedAt = null,
            ),
        )
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences()
        coEvery { municipalityCatalogRepository.getLocationKeysWithDataForWeek(any()) } returns emptySet()
        coEvery { municipalityCatalogRepository.getLocationKeysEverInAnp() } returns emptySet()
        coEvery { eventPublisher.publish(any()) } returns Unit
    }

    @Test
    fun listsAllCatalogStatesForLatestSurveyWeek() = runTest {
        coEvery { municipalityCatalogRepository.getCatalogStates() } returns listOf(
            BrazilianState.SAO_PAULO,
            BrazilianState.RIO_DE_JANEIRO,
        )

        val result = useCase.getStatesWithData()

        assertEquals(surveyWeek, result.surveyWeek)
        assertEquals(2, result.states.size)
        coVerify(exactly = 1) { municipalityCatalogRepository.getCatalogStates() }
    }

    @Test
    fun listsCatalogMunicipalitiesWithDataAvailability() = runTest {
        val campinasEntry = MunicipalityCatalogEntry(
            state = state,
            municipality = "CAMPINAS",
            ibgeCode = "3509502",
        )
        coEvery { municipalityCatalogRepository.getCatalogMunicipalities(state) } returns listOf(
            catalogEntry,
            campinasEntry,
        )
        coEvery {
            municipalityCatalogRepository.getLocationKeysWithDataForWeek(surveyWeek)
        } returns setOf(catalogEntry.locationKey)
        coEvery {
            municipalityCatalogRepository.getLocationKeysEverInAnp()
        } returns setOf(catalogEntry.locationKey, campinasEntry.locationKey)

        val result = useCase.getMunicipalities(state)

        assertEquals(surveyWeek, result.surveyWeek)
        assertEquals(state, result.state)
        assertEquals(
            listOf(
                CatalogMunicipalityItem(municipality, DataAvailability.HAS_DATA),
                CatalogMunicipalityItem("CAMPINAS", DataAvailability.NO_DATA_THIS_WEEK),
            ),
            result.municipalities,
        )
        coVerify(exactly = 1) { municipalityCatalogRepository.getLocationKeysWithDataForWeek(surveyWeek) }
        coVerify(exactly = 1) { municipalityCatalogRepository.getLocationKeysEverInAnp() }
    }

    @Test
    fun emptyMunicipalityListIsNotAnError() = runTest {
        coEvery { municipalityCatalogRepository.getCatalogMunicipalities(state) } returns emptyList()

        val result = useCase.getMunicipalities(state)

        assertTrue(result.isEmpty)
        assertTrue(result.municipalities.isEmpty())
    }

    @Test
    fun persistsPreferredLocationAndEmitsCitySelected() = runTest {
        val savedPreferences = slot<UserPreferences>()
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences()
        coEvery { userPreferencesRepository.savePreferences(capture(savedPreferences)) } returns Unit
        coEvery {
            municipalityCatalogRepository.findCatalogEntry(state, municipality)
        } returns catalogEntry

        val result = useCase.selectLocation(state, municipality)

        assertEquals(state, savedPreferences.captured.preferredState)
        assertEquals(municipality, savedPreferences.captured.preferredMunicipality)
        assertEquals(municipality, result.event.payload.municipality)
        assertEquals(state, result.event.payload.state)
        assertEquals(DomainId.forSurveyWeek(surveyWeek), result.event.payload.surveyWeekId)
        coVerify(exactly = 1) { eventPublisher.publish(any<CitySelected>()) }
    }

    @Test
    fun allowsCatalogMunicipalityWithoutCurrentWeekData() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences()
        coEvery { userPreferencesRepository.savePreferences(any()) } returns Unit
        coEvery {
            municipalityCatalogRepository.findCatalogEntry(BrazilianState.ACRE, "ACRELÂNDIA")
        } returns MunicipalityCatalogEntry(
            state = BrazilianState.ACRE,
            municipality = "ACRELÂNDIA",
            ibgeCode = "1200013",
        )

        val result = useCase.selectLocation(BrazilianState.ACRE, "ACRELÂNDIA")

        assertEquals("ACRELÂNDIA", result.preferences.preferredMunicipality)
        coVerify(exactly = 1) { userPreferencesRepository.savePreferences(any()) }
    }

    @Test
    fun rejectsSelectionWhenNoImportedData() = runTest {
        coEvery { priceTableRepository.countImportedSurveyWeeks() } returns 0

        assertThrows<DomainException> {
            useCase.getStatesWithData()
        }
    }

    @Test
    fun rejectsMunicipalityNotInCatalog() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences()
        coEvery {
            municipalityCatalogRepository.findCatalogEntry(state, municipality)
        } returns null

        assertThrows<DomainException> {
            useCase.selectLocation(state, municipality)
        }

        coVerify(exactly = 0) { userPreferencesRepository.savePreferences(any()) }
        coVerify(exactly = 0) { eventPublisher.publish(any()) }
    }

    @Test
    fun returnsPreferredLocationForExistingSelection() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            preferredState = state,
            preferredMunicipality = municipality,
        )

        val preferred = useCase.getPreferredLocation()

        assertEquals(state, preferred?.state)
        assertEquals(municipality, preferred?.municipality)
    }

    @Test
    fun returnsNullPreferredLocationWhenIncomplete() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            preferredState = state,
            preferredMunicipality = null,
        )

        assertNull(useCase.getPreferredLocation())
    }
}
