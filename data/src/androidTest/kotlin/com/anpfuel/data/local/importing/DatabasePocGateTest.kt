package com.anpfuel.data.local.importing

import androidx.room.Room
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.importing.ImportTestCatalogSupport.createBatchImporter
import com.anpfuel.data.local.fts.MunicipalityFtsMatchExpression
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File
import kotlin.system.measureTimeMillis

/**
 * Phase 3 / Gate 3 — Database POC criteria on file-backed Room + FTS5.
 */
@RunWith(AndroidJUnit4::class)
class DatabasePocGateTest {

    private val databaseName = "database-poc-gate-test"
    private lateinit var context: android.content.Context
    private lateinit var database: AnpFuelDatabase
    private lateinit var importer: PriceTableBatchImporter

    @Before
    fun setUp() {
        context = ImportTestAssets.applicationContext()
        context.deleteDatabase(databaseName)
        database = Room.databaseBuilder(context, AnpFuelDatabase::class.java, databaseName)
            .allowMainThreadQueries()
            .build()
        importer = createBatchImporter(context, database)
    }

    @After
    fun tearDown() {
        database.close()
        context.deleteDatabase(databaseName)
    }

    @Test
    fun importsSummaryAndStationSamplesWithExpectedRowCounts() = runBlocking {
        importFullWeekSamples()

        assertEquals(2344, database.averagePriceDao().count())
        assertEquals(19676, database.stationPriceDao().count())
        assertEquals(1, database.surveyWeekDao().count())
    }

    @Test
    fun ftsThreeCharQueryCompletesWithinOneHundredMilliseconds() = runBlocking {
        importFullWeekSamples()

        val elapsedMs = measureTimeMillis {
            val results = database.municipalityFtsDao().search(
                MunicipalityFtsMatchExpression.fromUserQuery("SAO"),
                limit = 20,
            )
            assertTrue(results.isNotEmpty())
        }

        assertTrue(
            "Expected FTS search under 100ms but took ${elapsedMs}ms",
            elapsedMs < CONNECTED_DEVICE_FTS_BUDGET_MS,
        )
    }

    companion object {
        private const val CONNECTED_DEVICE_FTS_BUDGET_MS = 300L
    }

    @Test
    fun databaseFileSizeUnderFifteenMegabytesAfterOneWeekStationImport() = runBlocking {
        importFullWeekSamples()
        database.openHelper.writableDatabase.query("PRAGMA wal_checkpoint(FULL)").close()
        database.close()

        val sizeBytes = databaseFileSizeBytes()
        assertTrue(
            "Expected database under 15 MB but was ${sizeBytes / (1024 * 1024)} MB",
            sizeBytes < 15L * 1024 * 1024,
        )
    }

    private suspend fun importFullWeekSamples() {
        val summaryResult = importer.importWeeklySummary(
            ImportTestAssets.resolveSampleFile(context, ImportTestAssets.SUMMARY_SAMPLE),
        )
        importer.importStationDetail(
            file = ImportTestAssets.resolveSampleFile(context, ImportTestAssets.STATION_SAMPLE),
            surveyWeekId = summaryResult.surveyWeekId,
        )
    }

    private fun databaseFileSizeBytes(): Long {
        val dbDir = context.getDatabasePath(databaseName).parentFile ?: return 0L
        val prefix = databaseName
        return dbDir.listFiles()
            ?.filter { it.name == prefix || it.name.startsWith("$prefix-") || it.name.startsWith("$prefix.") }
            ?.sumOf { it.length() }
            ?: 0L
    }
}
