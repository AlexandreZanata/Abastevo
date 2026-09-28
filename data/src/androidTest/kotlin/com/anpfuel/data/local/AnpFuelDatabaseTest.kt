package com.anpfuel.data.local

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.dao.AveragePriceDao
import com.anpfuel.data.local.dao.ImportAuditLogDao
import com.anpfuel.data.local.dao.StationPriceDao
import com.anpfuel.data.local.dao.SurveyWeekDao
import com.anpfuel.data.local.entity.AveragePriceEntity
import com.anpfuel.data.local.entity.ImportAuditLogEntity
import com.anpfuel.data.local.entity.StationPriceEntity
import com.anpfuel.data.local.entity.SurveyWeekEntity
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class AnpFuelDatabaseTest {

    private lateinit var database: AnpFuelDatabase
    private lateinit var surveyWeekDao: SurveyWeekDao
    private lateinit var averagePriceDao: AveragePriceDao
    private lateinit var stationPriceDao: StationPriceDao
    private lateinit var importAuditLogDao: ImportAuditLogDao

    @Before
    fun setUp() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        database = Room.inMemoryDatabaseBuilder(context, AnpFuelDatabase::class.java)
            .allowMainThreadQueries()
            .build()
        surveyWeekDao = database.surveyWeekDao()
        averagePriceDao = database.averagePriceDao()
        stationPriceDao = database.stationPriceDao()
        importAuditLogDao = database.importAuditLogDao()
    }

    @After
    fun tearDown() {
        database.close()
    }

    @Test
    fun batchInsertIgnoresDuplicateAveragePrices() = runBlocking {
        val surveyWeek = surveyWeekEntity()
        surveyWeekDao.insert(surveyWeek)

        val row = averagePriceEntity(surveyWeekId = surveyWeek.id)
        averagePriceDao.insertAll(listOf(row, row.copy(id = "duplicate-id")))

        assertEquals(1, averagePriceDao.count())
    }

    @Test
    fun insertsSurveyWeekAverageStationAndAuditRows() = runBlocking {
        val surveyWeek = surveyWeekEntity()
        surveyWeekDao.insert(surveyWeek)

        averagePriceDao.insertAll(listOf(averagePriceEntity(surveyWeekId = surveyWeek.id)))
        stationPriceDao.insertAll(listOf(stationPriceEntity(surveyWeekId = surveyWeek.id)))
        importAuditLogDao.insert(
            ImportAuditLogEntity(
                id = "audit-1",
                surveyWeekId = surveyWeek.id,
                action = "IMPORTED",
                detail = "summary batch",
                occurredAt = surveyWeek.summaryImportedAt,
            ),
        )

        assertEquals(1, surveyWeekDao.count())
        assertEquals(1, averagePriceDao.count())
        assertEquals(1, stationPriceDao.count())
        assertEquals(1, importAuditLogDao.count())
    }

    private fun surveyWeekEntity(): SurveyWeekEntity =
        SurveyWeekEntity(
            id = "week-2025-01",
            startDate = "2025-01-05",
            endDate = "2025-01-11",
            summaryImportedAt = 1_735_689_600_000L,
            stationImportedAt = null,
        )

    private fun averagePriceEntity(surveyWeekId: String): AveragePriceEntity =
        AveragePriceEntity(
            id = "avg-1",
            surveyWeekId = surveyWeekId,
            state = "SP",
            municipality = "SAO PAULO",
            fuelProduct = "GASOLINE",
            stationCount = 120,
            unit = "R$/l",
            avgPrice = 5.89,
            minPrice = 5.50,
            maxPrice = 6.20,
            stdDev = 0.15,
        )

    private fun stationPriceEntity(surveyWeekId: String): StationPriceEntity =
        StationPriceEntity(
            id = "station-1",
            surveyWeekId = surveyWeekId,
            cnpj = "12345678000199",
            legalName = "Example Fuel Ltd",
            tradeName = "Example Posto",
            address = "Rua Example, 100",
            municipality = "SAO PAULO",
            state = "SP",
            brand = "Example",
            fuelProduct = "GASOLINE",
            price = 5.79,
            collectedAt = "2025-01-08",
        )
}
