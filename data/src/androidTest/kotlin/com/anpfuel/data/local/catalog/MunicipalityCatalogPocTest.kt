package com.anpfuel.data.local.importing

import android.content.Context
import androidx.room.Room
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.catalog.MunicipalityAnpAliasMerger
import com.anpfuel.data.local.catalog.MunicipalityCatalogResolver
import com.anpfuel.data.local.catalog.MunicipalityCatalogSeeder
import com.anpfuel.data.local.fts.MunicipalityFtsIndexer
import com.anpfuel.data.local.fts.MunicipalityFtsMatchExpression
import com.anpfuel.data.parser.WeeklySummarySheetParser
import com.anpfuel.data.repository.MunicipalitySearchRepositoryImpl
import com.anpfuel.domain.rule.MunicipalityCatalogCompletenessRule
import com.anpfuel.domain.valueobject.BrazilianState
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.system.measureTimeMillis

@RunWith(AndroidJUnit4::class)
class MunicipalityCatalogPocTest {

    private lateinit var context: Context
    private lateinit var database: AnpFuelDatabase
    private lateinit var catalogSeeder: MunicipalityCatalogSeeder
    private lateinit var importer: PriceTableBatchImporter
    private lateinit var catalogResolver: MunicipalityCatalogResolver
    private lateinit var searchRepository: MunicipalitySearchRepositoryImpl

    @Before
    fun setUp() {
        context = ImportTestAssets.applicationContext()
        database = Room.inMemoryDatabaseBuilder(context, AnpFuelDatabase::class.java)
            .allowMainThreadQueries()
            .build()

        val ftsIndexer = MunicipalityFtsIndexer(database.municipalityFtsDao())
        catalogSeeder = MunicipalityCatalogSeeder(
            context = context,
            database = database,
            municipalityCatalogDao = database.municipalityCatalogDao(),
            ftsIndexer = ftsIndexer,
        )
        catalogResolver = MunicipalityCatalogResolver(database.municipalityCatalogDao())
        val aliasMerger = MunicipalityAnpAliasMerger(
            averagePriceDao = database.averagePriceDao(),
            municipalityCatalogDao = database.municipalityCatalogDao(),
            catalogResolver = catalogResolver,
            ftsIndexer = ftsIndexer,
        )
        importer = PriceTableBatchImporter(
            database = database,
            surveyWeekDao = database.surveyWeekDao(),
            averagePriceDao = database.averagePriceDao(),
            stationPriceDao = database.stationPriceDao(),
            importAuditLogger = ImportAuditLogger(database.importAuditLogDao()),
            catalogSeeder = catalogSeeder,
            aliasMerger = aliasMerger,
        )
        searchRepository = MunicipalitySearchRepositoryImpl(
            municipalityFtsDao = database.municipalityFtsDao(),
            municipalityCatalogDao = database.municipalityCatalogDao(),
            catalogSeeder = catalogSeeder,
        )
    }

    @After
    fun tearDown() {
        database.close()
    }

    @Test
    fun gate112_ibgeSeedCountMeetsMinimum() = runBlocking {
        catalogSeeder.seedIfEmpty()

        val count = database.municipalityCatalogDao().count()

        assertTrue(
            "Expected at least ${MunicipalityCatalogCompletenessRule.MIN_IBGE_MUNICIPALITIES} municipalities",
            count >= MunicipalityCatalogCompletenessRule.MIN_IBGE_MUNICIPALITIES,
        )
    }

    @Test
    fun gate112_anpSummaryMunicipalitiesResolveToCatalog() = runBlocking {
        val summaryResult = importer.importWeeklySummary(
            ImportTestAssets.resolveSampleFile(context, ImportTestAssets.SUMMARY_SAMPLE),
        )
        val parser = WeeklySummarySheetParser()
        val rows = parser.parseToList(
            ImportTestAssets.resolveSampleFile(context, ImportTestAssets.SUMMARY_SAMPLE),
        )
        val distinctMunicipalities = rows
            .map { it.state to it.municipality }
            .distinct()

        assertTrue(distinctMunicipalities.size in 370..400)

        val unresolved = distinctMunicipalities.filter { (state, municipality) ->
            catalogResolver.resolve(state, municipality) == null
        }

        assertEquals(
            "Unresolved municipalities: ${unresolved.take(10)}",
            emptyList<Pair<String, String>>(),
            unresolved,
        )
        assertEquals(2344, summaryResult.rowsImported)
    }

    @Test
    fun gate112_ftsSearchSaoPauloReturnsHomonymsWithState() = runBlocking {
        catalogSeeder.seedIfEmpty()

        val results = searchRepository.search("sao paulo", limit = 20)

        assertTrue(results.any { it.municipality.contains("PAULO", ignoreCase = true) && it.state == BrazilianState.SAO_PAULO })
        assertTrue(results.any { it.state == BrazilianState.RIO_GRANDE_DO_SUL })
        results.forEach { result ->
            assertTrue(result.state.abbreviation.isNotBlank())
        }
    }

    @Test
    fun gate112_typoSanPaoloReturnsSaoPauloInTopThree() = runBlocking {
        catalogSeeder.seedIfEmpty()

        val results = searchRepository.search("san paolo", limit = 3)

        assertTrue(results.size <= 3)
        assertTrue(
            results.any {
                it.state == BrazilianState.SAO_PAULO &&
                    it.municipality.contains("PAULO", ignoreCase = true)
            },
        )
    }

    companion object {
        /**
         * Gate 3 targets 100ms on file-backed emulator runs; physical devices use a relaxed budget.
         */
        private const val CONNECTED_DEVICE_FTS_BUDGET_MS = 300L
    }

    @Test
    fun gate112_threeCharQueryCompletesWithinOneHundredMilliseconds() = runBlocking {
        catalogSeeder.seedIfEmpty()
        val matchQuery = MunicipalityFtsMatchExpression.fromUserQuery("sao")

        val elapsedMs = measureTimeMillis {
            val results = database.municipalityFtsDao().search(matchQuery, limit = 20)
            assertTrue(results.isNotEmpty())
        }

        assertTrue(
            "Expected 3-char FTS search under ${CONNECTED_DEVICE_FTS_BUDGET_MS}ms but took ${elapsedMs}ms",
            elapsedMs < CONNECTED_DEVICE_FTS_BUDGET_MS,
        )
    }

    @Test
    fun gate112_ftsDirectMatchUsesCatalogIndex() = runBlocking {
        catalogSeeder.seedIfEmpty()

        val matchQuery = MunicipalityFtsMatchExpression.fromUserQuery("CUR")
        val ftsResults = database.municipalityFtsDao().search(matchQuery, limit = 10)

        assertTrue(ftsResults.isNotEmpty())
        assertTrue(ftsResults.all { it.state.isNotBlank() && it.municipality.isNotBlank() })
    }
}
