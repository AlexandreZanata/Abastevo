package com.anpfuel.application.usecase.location

import com.anpfuel.application.error.AppError
import com.anpfuel.domain.model.EnrichedSearchCandidate
import com.anpfuel.domain.model.MunicipalitySearchResult
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.SearchEnrichment
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.MunicipalitySearchRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.AvailabilitySnapshot
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertInstanceOf
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class SearchMunicipalityUseCaseTest {

    private val municipalitySearchRepository = mockk<MunicipalitySearchRepository>()
    private val municipalityCatalogRepository = mockk<MunicipalityCatalogRepository>()
    private val priceTableRepository = mockk<PriceTableRepository>()
    private val userPreferencesRepository = mockk<UserPreferencesRepository>()

    private lateinit var useCase: SearchMunicipalityUseCase

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

    @BeforeEach
    fun setUp() {
        useCase = SearchMunicipalityUseCase(
            municipalitySearchRepository = municipalitySearchRepository,
            municipalityCatalogRepository = municipalityCatalogRepository,
            priceTableRepository = priceTableRepository,
            userPreferencesRepository = userPreferencesRepository,
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
        coEvery { municipalityCatalogRepository.getLocationKeysWithDataForWeek(surveyWeek) } returns emptySet()
        coEvery { municipalityCatalogRepository.getLocationKeysEverInAnp() } returns emptySet()
    }

    @Test
    fun br007ShortQueryReturnsWithoutCallingFts() = runTest {
        val outcome = useCase.search("a")

        assertEquals(SearchMunicipalityOutcome.QueryTooShort, outcome)
        coVerify(exactly = 0) { municipalitySearchRepository.search(any(), any()) }
    }

    @Test
    fun br007SingleCharacterAfterTrimReturnsWithoutCallingFts() = runTest {
        val outcome = useCase.search("  s  ")

        assertEquals(SearchMunicipalityOutcome.QueryTooShort, outcome)
        coVerify(exactly = 0) { municipalitySearchRepository.search(any(), any()) }
    }

    @Test
    fun annotatesResultsWithDataAvailability() = runTest {
        val saoPaulo = MunicipalitySearchResult(
            municipality = "SÃO PAULO",
            state = BrazilianState.SAO_PAULO,
            dataAvailability = DataAvailability.HAS_DATA,
        )
        val acrelandia = MunicipalitySearchResult(
            municipality = "ACRELÂNDIA",
            state = BrazilianState.ACRE,
            dataAvailability = DataAvailability.HAS_DATA,
        )
        val saoPauloEntry = MunicipalityCatalogEntry(
            state = BrazilianState.SAO_PAULO,
            municipality = "SÃO PAULO",
            ibgeCode = "3550308",
        )
        coEvery { municipalitySearchRepository.search("sao", 20) } returns listOf(saoPaulo, acrelandia)
        coEvery {
            municipalityCatalogRepository.enrichSearchResults(listOf(saoPaulo, acrelandia), surveyWeek)
        } returns SearchEnrichment(
            candidates = listOf(
                EnrichedSearchCandidate(result = saoPaulo, entry = saoPauloEntry),
                EnrichedSearchCandidate(result = acrelandia, entry = null),
            ),
            snapshot = AvailabilitySnapshot(
                withDataThisWeek = setOf(saoPauloEntry.locationKey),
                everInAnp = setOf(saoPauloEntry.locationKey),
            ),
        )

        val outcome = useCase.search("  sao ")

        assertInstanceOf(SearchMunicipalityOutcome.Success::class.java, outcome)
        val results = (outcome as SearchMunicipalityOutcome.Success).results
        assertEquals(DataAvailability.HAS_DATA, results[0].dataAvailability)
        assertEquals(DataAvailability.NEVER_IN_ANP, results[1].dataAvailability)
        coVerify(exactly = 1) { municipalitySearchRepository.search("sao", 20) }
        coVerify(exactly = 1) {
            municipalityCatalogRepository.enrichSearchResults(listOf(saoPaulo, acrelandia), surveyWeek)
        }
        // MEX-04: no per-result database lookup for enrichment.
        coVerify(exactly = 0) {
            municipalityCatalogRepository.findCatalogEntry(any(), any())
        }
    }

    @Test
    fun returnsSearchNoResultsWhenFtsIsEmpty() = runTest {
        coEvery { municipalitySearchRepository.search("xyz", 20) } returns emptyList()

        val outcome = useCase.search("xyz")

        assertInstanceOf(SearchMunicipalityOutcome.NoResults::class.java, outcome)
        assertEquals(
            AppError.SearchNoResults,
            (outcome as SearchMunicipalityOutcome.NoResults).error,
        )
    }

    @Test
    fun discoversResultsWhenNoImportedData() = runTest {
        // MEX-04 (B-BR-MEX-07/BUC-MEX-05): discovery works without an imported
        // week; every result is reported unavailable, never an error.
        coEvery { priceTableRepository.getImportedPriceSurveys() } returns emptyList()
        val match = MunicipalitySearchResult(
            municipality = "SÃO PAULO",
            state = BrazilianState.SAO_PAULO,
            dataAvailability = DataAvailability.HAS_DATA,
        )
        coEvery { municipalitySearchRepository.search("sao", 20) } returns listOf(match)
        coEvery {
            municipalityCatalogRepository.enrichSearchResults(listOf(match), null)
        } returns SearchEnrichment(
            candidates = listOf(
                EnrichedSearchCandidate(
                    result = match,
                    entry = MunicipalityCatalogEntry(
                        state = BrazilianState.SAO_PAULO,
                        municipality = "SÃO PAULO",
                        ibgeCode = "3550308",
                    ),
                ),
            ),
            snapshot = AvailabilitySnapshot.empty(),
        )

        val outcome = useCase.search("sao")

        assertInstanceOf(SearchMunicipalityOutcome.Success::class.java, outcome)
        val results = (outcome as SearchMunicipalityOutcome.Success).results
        assertEquals(DataAvailability.NEVER_IN_ANP, results[0].dataAvailability)
        coVerify(exactly = 1) { municipalitySearchRepository.search("sao", 20) }
        coVerify(exactly = 1) {
            municipalityCatalogRepository.enrichSearchResults(listOf(match), null)
        }
    }

    @Test
    fun reportsPreviouslyImportedLocationWithoutWeek() = runTest {
        // MEX-04: without a selected week, a location seen in ANP history is
        // NO_DATA_THIS_WEEK, distinct from NEVER_IN_ANP.
        coEvery { priceTableRepository.getImportedPriceSurveys() } returns emptyList()
        val match = MunicipalitySearchResult(
            municipality = "CAMPINAS",
            state = BrazilianState.SAO_PAULO,
            dataAvailability = DataAvailability.HAS_DATA,
        )
        val entry = MunicipalityCatalogEntry(
            state = BrazilianState.SAO_PAULO,
            municipality = "CAMPINAS",
            ibgeCode = "3509502",
        )
        coEvery { municipalitySearchRepository.search("camp", 20) } returns listOf(match)
        coEvery {
            municipalityCatalogRepository.enrichSearchResults(listOf(match), null)
        } returns SearchEnrichment(
            candidates = listOf(EnrichedSearchCandidate(result = match, entry = entry)),
            snapshot = AvailabilitySnapshot(
                withDataThisWeek = emptySet(),
                everInAnp = setOf(entry.locationKey),
            ),
        )

        val outcome = useCase.search("camp")

        assertInstanceOf(SearchMunicipalityOutcome.Success::class.java, outcome)
        val results = (outcome as SearchMunicipalityOutcome.Success).results
        assertEquals(DataAvailability.NO_DATA_THIS_WEEK, results[0].dataAvailability)
    }

    @Test
    fun respectsCustomResultLimit() = runTest {
        coEvery { municipalitySearchRepository.search("camp", 5) } returns emptyList()

        useCase.search("camp", limit = 5)

        coVerify(exactly = 1) { municipalitySearchRepository.search("camp", 5) }
    }
}
