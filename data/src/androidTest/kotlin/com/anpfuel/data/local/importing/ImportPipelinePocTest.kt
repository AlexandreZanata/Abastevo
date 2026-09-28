package com.anpfuel.data.local.importing

import androidx.room.Room
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.importing.ImportTestCatalogSupport.createBatchImporter
import com.anpfuel.data.local.importing.ImportAuditAction.DOWNLOADED
import com.anpfuel.data.local.importing.ImportAuditAction.IMPORTED
import com.anpfuel.data.mapper.EntityDomainMapper
import com.anpfuel.domain.valueobject.SurveyWeek
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.system.measureTimeMillis

@RunWith(AndroidJUnit4::class)
class ImportPipelinePocTest {

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
    fun importsSummarySampleInBatchesOfOneThousand() = runBlocking {
        val summaryFile = ImportTestAssets.resolveSampleFile(
            ImportTestAssets.applicationContext(),
            ImportTestAssets.SUMMARY_SAMPLE,
        )

        val result = importer.importWeeklySummary(summaryFile)

        assertEquals(2344, result.rowsImported)
        assertEquals(2344, database.averagePriceDao().count())
        assertEquals(1, database.surveyWeekDao().count())
        assertTrue(database.importAuditLogDao().count() >= 3)
    }

    @Test
    fun importsStationSampleWithExpectedRowCount() = runBlocking {
        val context = ImportTestAssets.applicationContext()
        val summaryResult = importer.importWeeklySummary(
            ImportTestAssets.resolveSampleFile(context, ImportTestAssets.SUMMARY_SAMPLE),
        )
        val stationResult = importer.importStationDetail(
            file = ImportTestAssets.resolveSampleFile(context, ImportTestAssets.STATION_SAMPLE),
            surveyWeekId = summaryResult.surveyWeekId,
        )

        assertEquals(19676, stationResult.rowsImported)
        assertEquals(19676, database.stationPriceDao().count())
    }

    @Test
    fun queryByMunicipalityAndWeekCompletesWithinFiftyMilliseconds() = runBlocking {
        val context = ImportTestAssets.applicationContext()
        val summaryResult = importer.importWeeklySummary(
            ImportTestAssets.resolveSampleFile(context, ImportTestAssets.SUMMARY_SAMPLE),
        )
        val sampleRow = requireNotNull(
            database.averagePriceDao().findAnyBySurveyWeek(summaryResult.surveyWeekId),
        )

        val elapsedMs = measureTimeMillis {
            val rows = database.averagePriceDao().findByLocation(
                surveyWeekId = summaryResult.surveyWeekId,
                state = sampleRow.state,
                municipality = sampleRow.municipality,
            )
            assertTrue(rows.isNotEmpty())
        }

        assertTrue(
            "Expected indexed query under 50ms but took ${elapsedMs}ms",
            elapsedMs < 50,
        )
    }

    @Test
    fun appendImportAuditEntriesForEachStage() = runBlocking {
        val context = ImportTestAssets.applicationContext()
        val summaryResult = importer.importWeeklySummary(
            ImportTestAssets.resolveSampleFile(context, ImportTestAssets.SUMMARY_SAMPLE),
        )
        importer.importStationDetail(
            file = ImportTestAssets.resolveSampleFile(context, ImportTestAssets.STATION_SAMPLE),
            surveyWeekId = summaryResult.surveyWeekId,
        )

        val auditEntries = database.importAuditLogDao().findBySurveyWeek(summaryResult.surveyWeekId)
        val actions = auditEntries.map { it.action }.toSet()

        assertTrue(actions.contains(DOWNLOADED.name))
        assertTrue(actions.contains(IMPORTED.name))
        assertTrue(auditEntries.any { it.detail?.contains("WEEKLY_SUMMARY imported rows=2344") == true })
        assertTrue(auditEntries.any { it.detail?.contains("STATION_DETAIL imported rows=19676") == true })
    }

    @Test
    fun importedAveragePriceMapsBackToDomainModel() = runBlocking {
        val summaryFile = ImportTestAssets.resolveSampleFile(
            ImportTestAssets.applicationContext(),
            ImportTestAssets.SUMMARY_SAMPLE,
        )
        val result = importer.importWeeklySummary(summaryFile)
        val surveyWeekEntity = requireNotNull(database.surveyWeekDao().findById(result.surveyWeekId))
        val surveyWeek = SurveyWeek.fromIsoDates(surveyWeekEntity.startDate, surveyWeekEntity.endDate)
        val storedRow = requireNotNull(database.averagePriceDao().findAnyBySurveyWeek(result.surveyWeekId))
        val domain = EntityDomainMapper.toAveragePrice(storedRow, surveyWeek)

        assertTrue(domain.municipality.isNotBlank())
        assertTrue(domain.hasPriceStatistics())
    }
}
