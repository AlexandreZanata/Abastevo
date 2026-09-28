package com.anpfuel.data.repository

import androidx.room.Room
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.application.usecase.location.SearchMunicipalityOutcome
import com.anpfuel.application.usecase.location.SearchMunicipalityUseCase
import com.anpfuel.application.usecase.price.GetMunicipalityPricesUseCase
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.importing.ImportAuditLogger
import com.anpfuel.data.local.importing.ImportTestAssets
import com.anpfuel.data.local.importing.PriceTableBatchImporter
import com.anpfuel.data.local.importing.ImportTestCatalogSupport
import com.anpfuel.data.local.fts.MunicipalityFtsIndexer
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.BrazilianState
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * End-to-end: import sample XLSX → query via application use cases (Phase 5.10.3).
 */
@RunWith(AndroidJUnit4::class)
class RepositoryUseCaseIntegrationTest {

    private lateinit var database: AnpFuelDatabase
    private lateinit var priceTableRepository: PriceTableRepositoryImpl
    private lateinit var averagePriceRepository: AveragePriceRepositoryImpl
    private lateinit var municipalitySearchRepository: MunicipalitySearchRepositoryImpl
    private lateinit var getMunicipalityPricesUseCase: GetMunicipalityPricesUseCase
    private lateinit var searchMunicipalityUseCase: SearchMunicipalityUseCase

    @Before
    fun setUp() {
        val context = ImportTestAssets.applicationContext()
        database = Room.inMemoryDatabaseBuilder(context, AnpFuelDatabase::class.java)
            .allowMainThreadQueries()
            .build()

        val batchImporter = ImportTestCatalogSupport.createBatchImporter(context, database)

        priceTableRepository = PriceTableRepositoryImpl(
            database = database,
            surveyWeekDao = database.surveyWeekDao(),
            averagePriceDao = database.averagePriceDao(),
            stationPriceDao = database.stationPriceDao(),
            priceTableMetadataStore = InMemoryPriceTableMetadataStore(),
            ftsIndexer = MunicipalityFtsIndexer(database.municipalityFtsDao()),
        )
        averagePriceRepository = AveragePriceRepositoryImpl(
            averagePriceDao = database.averagePriceDao(),
            surveyWeekDao = database.surveyWeekDao(),
        )
        val municipalityCatalogRepository = ImportTestCatalogSupport.createCatalogRepository(context, database)
        municipalitySearchRepository = ImportTestCatalogSupport.createSearchRepository(context, database)

        getMunicipalityPricesUseCase = GetMunicipalityPricesUseCase(
            averagePriceRepository = averagePriceRepository,
            municipalityCatalogRepository = municipalityCatalogRepository,
            priceTableRepository = priceTableRepository,
            userPreferencesRepository = InMemoryUserPreferencesRepository(),
        )
        searchMunicipalityUseCase = SearchMunicipalityUseCase(
            municipalitySearchRepository = municipalitySearchRepository,
            municipalityCatalogRepository = municipalityCatalogRepository,
            priceTableRepository = priceTableRepository,
            userPreferencesRepository = InMemoryUserPreferencesRepository(),
        )

        runBlocking {
            val summaryFile = ImportTestAssets.resolveSampleFile(
                context,
                ImportTestAssets.SUMMARY_SAMPLE,
            )
            batchImporter.importWeeklySummary(summaryFile)
        }
    }

    @After
    fun tearDown() {
        database.close()
    }

    @Test
    fun importedSummaryIsQueryableThroughGetMunicipalityPricesUseCase() = runBlocking {
        val surveyWeekId = requireNotNull(
            database.surveyWeekDao().findAllOrderedByEndDateDesc().firstOrNull(),
        ).id
        val sampleRow = requireNotNull(
            database.averagePriceDao().findAnyBySurveyWeek(surveyWeekId),
        )
        val state = BrazilianState.fromAbbreviation(sampleRow.state)!!

        val result = getMunicipalityPricesUseCase(
            state = state,
            municipality = sampleRow.municipality,
        )

        assertFalse(result.isEmpty)
        assertEquals(DataAvailability.HAS_DATA, result.dataAvailability)
        assertTrue(result.prices.isNotEmpty())
        assertTrue(result.prices.all { it.matchesLocation(state, sampleRow.municipality) })
    }

    @Test
    fun importedSummaryIsSearchableThroughSearchMunicipalityUseCase() = runBlocking {
        val surveyWeekId = requireNotNull(
            database.surveyWeekDao().findAllOrderedByEndDateDesc().firstOrNull(),
        ).id
        val sampleRow = requireNotNull(
            database.averagePriceDao().findAnyBySurveyWeek(surveyWeekId),
        )
        val prefix = sampleRow.municipality.take(4)

        val outcome = searchMunicipalityUseCase.search(prefix)

        assertTrue(outcome is SearchMunicipalityOutcome.Success)
        val results = (outcome as SearchMunicipalityOutcome.Success).results
        assertTrue(results.any { it.municipality.equals(sampleRow.municipality, ignoreCase = true) })
    }
}
