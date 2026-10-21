package com.anpfuel.data.repository

import com.anpfuel.data.local.catalog.MunicipalityCatalogSeeder
import com.anpfuel.data.local.dao.AveragePriceDao
import com.anpfuel.data.local.dao.MunicipalityCatalogDao
import com.anpfuel.data.local.entity.MunicipalityCatalogEntity
import com.anpfuel.data.local.model.MunicipalityLocationAvailabilityRow
import com.anpfuel.domain.model.MunicipalitySearchResult
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.MunicipalityLocationKey
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/**
 * MEX-04 — Batch catalog/availability reads. Availability resolution must use
 * one grouped availability query plus one indexed state read per involved
 * state; never a per-row or per-result catalog lookup. Accent/case spelling
 * variants keep resolving through the stored normalized_name, exactly like
 * the previous row-by-row lookup.
 */
class MunicipalityCatalogBatchTest {

    private val municipalityCatalogDao = mockk<MunicipalityCatalogDao>()
    private val averagePriceDao = mockk<AveragePriceDao>()
    private val catalogSeeder = mockk<MunicipalityCatalogSeeder>()

    private lateinit var repository: MunicipalityCatalogRepositoryImpl

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val surveyWeekId = DomainId.forSurveyWeek(surveyWeek).value

    private val saoPauloEntity = MunicipalityCatalogEntity(
        id = "sp-3550308",
        ibgeCode = "3550308",
        state = "SP",
        municipality = "São Paulo",
        normalizedName = "SAO PAULO",
    )
    private val campinasEntity = MunicipalityCatalogEntity(
        id = "sp-3509502",
        ibgeCode = "3509502",
        state = "SP",
        municipality = "Campinas",
        normalizedName = "CAMPINAS",
    )
    private val rioEntity = MunicipalityCatalogEntity(
        id = "rj-3304557",
        ibgeCode = "3304557",
        state = "RJ",
        municipality = "Rio de Janeiro",
        normalizedName = "RIO DE JANEIRO",
    )

    @BeforeEach
    fun setUp() {
        repository = MunicipalityCatalogRepositoryImpl(
            municipalityCatalogDao = municipalityCatalogDao,
            averagePriceDao = averagePriceDao,
            catalogSeeder = catalogSeeder,
        )
        coEvery { catalogSeeder.seedIfEmpty() } returns Unit
        coEvery { municipalityCatalogDao.findByState("SP") } returns listOf(
            saoPauloEntity,
            campinasEntity,
        )
        coEvery { municipalityCatalogDao.findByState("RJ") } returns listOf(rioEntity)
    }

    @Test
    fun snapshotResolvesAccentVariantsWithoutPerRowLookups() = runTest {
        coEvery { averagePriceDao.findLocationAvailability(surveyWeekId) } returns listOf(
            // Raw ANP spelling variant of the canonical "São Paulo".
            MunicipalityLocationAvailabilityRow("SP", "SAO PAULO", hasWeekData = true),
            MunicipalityLocationAvailabilityRow("SP", "campinas", hasWeekData = false),
            MunicipalityLocationAvailabilityRow("RJ", "Rio de Janeiro", hasWeekData = true),
            // Unknown state abbreviation is dropped, as before.
            MunicipalityLocationAvailabilityRow("XX", "Nowhere", hasWeekData = true),
            // Spelling with no catalog counterpart is dropped, as before.
            MunicipalityLocationAvailabilityRow("SP", "Atlantida", hasWeekData = false),
        )

        val snapshot = repository.resolveAvailabilitySnapshot(surveyWeek)

        assertEquals(
            setOf(
                MunicipalityLocationKey(BrazilianState.SAO_PAULO, "São Paulo"),
                MunicipalityLocationKey(BrazilianState.RIO_DE_JANEIRO, "Rio de Janeiro"),
            ),
            snapshot.withDataThisWeek,
        )
        assertEquals(
            setOf(
                MunicipalityLocationKey(BrazilianState.SAO_PAULO, "São Paulo"),
                MunicipalityLocationKey(BrazilianState.SAO_PAULO, "Campinas"),
                MunicipalityLocationKey(BrazilianState.RIO_DE_JANEIRO, "Rio de Janeiro"),
            ),
            snapshot.everInAnp,
        )
        coVerify(exactly = 1) { averagePriceDao.findLocationAvailability(surveyWeekId) }
        coVerify(exactly = 1) { municipalityCatalogDao.findByState("SP") }
        coVerify(exactly = 1) { municipalityCatalogDao.findByState("RJ") }
        coVerify(exactly = 0) { municipalityCatalogDao.findByState("XX") }
        // MEX-04: no per-row normalized-name lookup.
        coVerify(exactly = 0) {
            municipalityCatalogDao.findByStateAndNormalizedName(any(), any())
        }
        coVerify(exactly = 0) { averagePriceDao.findDistinctLocationsBySurveyWeek(any()) }
        coVerify(exactly = 0) { averagePriceDao.findDistinctLocationsEverImported() }
    }

    @Test
    fun emptyAvailabilitySkipsCatalogReads() = runTest {
        coEvery { averagePriceDao.findLocationAvailability(surveyWeekId) } returns emptyList()

        val snapshot = repository.resolveAvailabilitySnapshot(surveyWeek)

        assertEquals(emptySet<MunicipalityLocationKey>(), snapshot.withDataThisWeek)
        assertEquals(emptySet<MunicipalityLocationKey>(), snapshot.everInAnp)
        coVerify(exactly = 0) { municipalityCatalogDao.findByState(any()) }
    }

    @Test
    fun nullWeekQueriesWithEmptyIdAndYieldsEmptyWeekSet() = runTest {
        coEvery { averagePriceDao.findLocationAvailability("") } returns listOf(
            MunicipalityLocationAvailabilityRow("SP", "Campinas", hasWeekData = false),
        )

        val snapshot = repository.resolveAvailabilitySnapshot(null)

        assertEquals(emptySet<MunicipalityLocationKey>(), snapshot.withDataThisWeek)
        assertEquals(
            setOf(MunicipalityLocationKey(BrazilianState.SAO_PAULO, "Campinas")),
            snapshot.everInAnp,
        )
        coVerify(exactly = 1) { averagePriceDao.findLocationAvailability("") }
    }

    @Test
    fun enrichSearchResultsUsesSinglePassForEntriesAndAvailability() = runTest {
        val results = listOf(
            MunicipalitySearchResult("São Paulo", BrazilianState.SAO_PAULO, DataAvailability.HAS_DATA),
            MunicipalitySearchResult("Niterói", BrazilianState.RIO_DE_JANEIRO, DataAvailability.HAS_DATA),
        )
        coEvery { averagePriceDao.findLocationAvailability(surveyWeekId) } returns listOf(
            MunicipalityLocationAvailabilityRow("SP", "São Paulo", hasWeekData = true),
        )

        val enrichment = repository.enrichSearchResults(results, surveyWeek)

        assertEquals(
            "3550308",
            enrichment.candidates[0].entry?.ibgeCode,
        )
        // Niterói is not in the loaded RJ catalog page: null entry, never an error.
        assertNull(enrichment.candidates[1].entry)
        assertEquals(
            setOf(MunicipalityLocationKey(BrazilianState.SAO_PAULO, "São Paulo")),
            enrichment.snapshot.withDataThisWeek,
        )
        coVerify(exactly = 1) { averagePriceDao.findLocationAvailability(surveyWeekId) }
        coVerify(exactly = 1) { municipalityCatalogDao.findByState("SP") }
        coVerify(exactly = 1) { municipalityCatalogDao.findByState("RJ") }
        coVerify(exactly = 0) {
            municipalityCatalogDao.findByStateAndNormalizedName(any(), any())
        }
    }

    @Test
    fun emptySearchResultsSkipAllReads() = runTest {
        val enrichment = repository.enrichSearchResults(emptyList(), surveyWeek)

        assertEquals(0, enrichment.candidates.size)
        coVerify(exactly = 0) { averagePriceDao.findLocationAvailability(any()) }
        coVerify(exactly = 0) { municipalityCatalogDao.findByState(any()) }
    }

    @Test
    fun legacyLocationKeyAccessorsDelegateToSnapshot() = runTest {
        coEvery { averagePriceDao.findLocationAvailability(surveyWeekId) } returns listOf(
            MunicipalityLocationAvailabilityRow("SP", "São Paulo", hasWeekData = true),
        )
        coEvery { averagePriceDao.findLocationAvailability("") } returns listOf(
            MunicipalityLocationAvailabilityRow("SP", "São Paulo", hasWeekData = false),
        )

        assertEquals(
            setOf(MunicipalityLocationKey(BrazilianState.SAO_PAULO, "São Paulo")),
            repository.getLocationKeysWithDataForWeek(surveyWeek),
        )
        assertEquals(
            setOf(MunicipalityLocationKey(BrazilianState.SAO_PAULO, "São Paulo")),
            repository.getLocationKeysEverInAnp(),
        )
    }
}
