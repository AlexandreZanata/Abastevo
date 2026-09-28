package com.anpfuel.data.local.importing

import androidx.room.Room
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.importing.ImportTestCatalogSupport.createBatchImporter
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.system.measureTimeMillis

/**
 * Phase 9.2.1 — Full import pipeline must complete within 60 seconds on device/emulator.
 */
@RunWith(AndroidJUnit4::class)
class ImportPerformanceTest {

    private lateinit var database: AnpFuelDatabase
    private lateinit var importer: PriceTableBatchImporter

    @Before
    fun setUp() {
        val context = ImportTestAssets.applicationContext()
        database = Room.inMemoryDatabaseBuilder(context, AnpFuelDatabase::class.java)
            .allowMainThreadQueries()
            .build()
        importer = createBatchImporter(context, database)
    }

    @After
    fun tearDown() {
        database.close()
    }

    @Test
    fun importsTwentyThousandRowsWithinSixtySeconds() = runBlocking {
        val context = ImportTestAssets.applicationContext()
        val summaryFile = ImportTestAssets.resolveSampleFile(context, ImportTestAssets.SUMMARY_SAMPLE)
        val stationFile = ImportTestAssets.resolveSampleFile(context, ImportTestAssets.STATION_SAMPLE)

        var summaryRows = 0
        var stationRows = 0
        val elapsedMs = measureTimeMillis {
            val summaryResult = importer.importWeeklySummary(summaryFile)
            summaryRows = summaryResult.rowsImported
            val stationResult = importer.importStationDetail(
                file = stationFile,
                surveyWeekId = summaryResult.surveyWeekId,
            )
            stationRows = stationResult.rowsImported
        }

        val totalRows = summaryRows + stationRows
        println("IMPORT_20K_DURATION_MS=$elapsedMs ROWS=$totalRows")

        assertTrue("Expected at least 20K rows but imported $totalRows", totalRows >= 20_000)
        assertEquals(2344, summaryRows)
        assertEquals(19676, stationRows)
        assertTrue(
            "Import exceeded 60s limit: ${elapsedMs}ms",
            elapsedMs < IMPORT_LIMIT_MS,
        )
    }

    companion object {
        const val IMPORT_LIMIT_MS = 60_000L
    }
}
