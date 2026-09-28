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

/**
 * BR-003 — re-import must append audit history and never delete existing price rows.
 */
@RunWith(AndroidJUnit4::class)
class ImmutableImportHistoryTest {

    private lateinit var database: AnpFuelDatabase
    private lateinit var importer: PriceTableBatchImporter

    @Before
    fun setUp() {
        val context = ImportTestAssets.applicationContext()
        database = Room.inMemoryDatabaseBuilder(context, AnpFuelDatabase::class.java)
            .allowMainThreadQueries()
            .build()
        importer = ImportTestCatalogSupport.createBatchImporter(context, database)
    }

    @After
    fun tearDown() {
        database.close()
    }

    @Test
    fun reimportSameWeekAppendsAuditEntriesWithoutDeletingPrices() = runBlocking {
        val context = ImportTestAssets.applicationContext()
        val summaryFile = ImportTestAssets.resolveSampleFile(context, ImportTestAssets.SUMMARY_SAMPLE)

        val firstImport = importer.importWeeklySummary(summaryFile)
        val priceCountAfterFirst = database.averagePriceDao().count()
        val auditCountAfterFirst = database.importAuditLogDao().count()
        val sampleRowBefore = requireNotNull(
            database.averagePriceDao().findAnyBySurveyWeek(firstImport.surveyWeekId),
        )

        val secondImport = importer.importWeeklySummary(summaryFile)
        val auditEntries = database.importAuditLogDao().findBySurveyWeek(firstImport.surveyWeekId)

        assertEquals(firstImport.surveyWeekId, secondImport.surveyWeekId)
        assertEquals(2344, priceCountAfterFirst)
        assertEquals(2344, database.averagePriceDao().count())
        assertEquals(2344, secondImport.rowsImported)
        assertTrue(auditCountAfterFirst < database.importAuditLogDao().count())
        assertTrue(auditEntries.count { it.action == ImportAuditAction.IMPORTED.name } >= 2)

        val sampleRowAfter = requireNotNull(
            database.averagePriceDao().findAnyBySurveyWeek(firstImport.surveyWeekId),
        )
        assertEquals(sampleRowBefore.id, sampleRowAfter.id)
        assertEquals(sampleRowBefore.avgPrice, sampleRowAfter.avgPrice)
    }

    @Test
    fun reimportStationDetailPreservesExistingRows() = runBlocking {
        val context = ImportTestAssets.applicationContext()
        val summaryResult = importer.importWeeklySummary(
            ImportTestAssets.resolveSampleFile(context, ImportTestAssets.SUMMARY_SAMPLE),
        )
        val stationFile = ImportTestAssets.resolveSampleFile(context, ImportTestAssets.STATION_SAMPLE)

        importer.importStationDetail(stationFile, summaryResult.surveyWeekId)
        val countAfterFirst = database.stationPriceDao().count()

        importer.importStationDetail(stationFile, summaryResult.surveyWeekId)

        assertEquals(19676, countAfterFirst)
        assertEquals(19676, database.stationPriceDao().count())
    }
}
