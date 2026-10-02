package com.anpfuel.data.worker

import android.content.Context
import android.util.Log
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.work.Configuration
import androidx.work.ListenableWorker
import androidx.work.WorkerFactory
import androidx.work.WorkerParameters
import androidx.work.testing.SynchronousExecutor
import androidx.work.testing.TestListenableWorkerBuilder
import androidx.work.testing.WorkManagerTestInitHelper
import com.anpfuel.application.usecase.alert.EvaluatePriceDropAlertsUseCase
import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.model.PriceDropAlertNotification
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.repository.AveragePriceRepository
import com.anpfuel.domain.repository.PriceDropAlertHistoryRepository
import com.anpfuel.domain.repository.PriceDropNotificationRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * P24-T01 — worker wiring without mockk: mockk cannot intercept
 * final use-case methods on old ART (real code runs → NPE), so the
 * use case runs for real against hand fakes. Same assertions as
 * before: success maps to success, transport failure maps to retry.
 */
@RunWith(AndroidJUnit4::class)
class PriceDropEvaluationWorkerTest {

    private lateinit var context: Context

    private val currentWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val previousWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")

    @Before
    fun setUp() {
        context = ApplicationProvider.getApplicationContext()
        val config = Configuration.Builder()
            .setMinimumLoggingLevel(Log.DEBUG)
            .setExecutor(SynchronousExecutor())
            .build()
        WorkManagerTestInitHelper.initializeTestWorkManager(context, config)
    }

    @After
    fun tearDown() {
        WorkManagerTestInitHelper.closeWorkDatabase()
    }

    @Test
    fun doWork_returnsSuccessWhenUseCaseSucceeds() = runBlocking {
        val notifications = FakeNotifications()
        val useCase = useCase(notifications, surveyWithDrop())

        val result = runWorker(useCase)

        assertEquals(ListenableWorker.Result.success(), result)
        assertEquals(1, notifications.shown.size)
    }

    @Test
    fun doWork_returnsRetryWhenUseCaseThrows() = runBlocking {
        val notifications = FakeNotifications(throwOnShow = RuntimeException("boom"))
        val useCase = useCase(notifications, surveyWithDrop())

        val result = runWorker(useCase)

        assertEquals(ListenableWorker.Result.retry(), result)
    }

    private fun useCase(
        notifications: FakeNotifications,
        surveys: SurveyFixture,
    ): EvaluatePriceDropAlertsUseCase = EvaluatePriceDropAlertsUseCase(
        vehicleRepository = FakeVehicles(listOf(vehicle("Gol"))),
        averagePriceRepository = FakeAverages(),
        stationPriceRepository = FakeStationPrices(surveys),
        priceTableRepository = FakePriceTables(listOf(surveys.current, surveys.previous)),
        userPreferencesRepository = FakePrefs(),
        priceDropNotificationRepository = notifications,
        priceDropAlertHistoryRepository = notifications,
    )

    private fun surveyWithDrop(): SurveyFixture = SurveyFixture(
        current = PriceSurvey.restore(
            id = DomainId.forSurveyWeek(currentWeek),
            surveyWeek = currentWeek,
            summaryImportedAt = java.time.Instant.parse("2026-06-14T10:00:00Z"),
            stationImportedAt = java.time.Instant.parse("2026-06-14T11:00:00Z"),
        ),
        previous = PriceSurvey.restore(
            id = DomainId.forSurveyWeek(previousWeek),
            surveyWeek = previousWeek,
            summaryImportedAt = java.time.Instant.parse("2026-06-07T10:00:00Z"),
            stationImportedAt = java.time.Instant.parse("2026-06-07T11:00:00Z"),
        ),
    )

    private fun vehicle(name: String): Vehicle = Vehicle.create(
        displayName = name,
        tankCapacity = TankCapacity.of(50.0),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        priceSource = VehiclePriceSource.cheapest(),
        priceDropAlertEnabled = true,
    )

    private fun station(price: String, week: SurveyWeek): StationPrice = StationPrice.create(
        priceSurveyId = DomainId.forSurveyWeek(week),
        surveyWeek = week,
        station = RetailStation.create(
            cnpj = Cnpj.parse("12345678000195"),
            legalName = "POSTO",
            tradeName = "POSTO",
            address = "RUA A",
            municipality = "CURITIBA",
            state = BrazilianState.PARANA,
            brand = "BR",
        ),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        price = PriceAmount.of(price),
    )

    private data class SurveyFixture(val current: PriceSurvey, val previous: PriceSurvey)

    private inner class FakeStationPrices(
        private val surveys: SurveyFixture,
    ) : StationPriceRepository {
        override suspend fun getStationPrices(
            state: BrazilianState,
            municipality: String,
            fuelProduct: FuelProduct,
            surveyWeek: SurveyWeek,
        ): List<StationPrice> {
            val price = if (surveyWeek == currentWeek) "5.40" else "5.60"
            return listOf(station(price, surveyWeek))
        }

        override suspend fun hasStationData(
            surveyWeek: SurveyWeek,
            state: BrazilianState,
            municipality: String,
        ): Boolean = true

        override suspend fun deleteStationPricesOlderThanRetention(retentionWeeks: Int) = Unit
    }

    private class FakeVehicles(private val vehicles: List<Vehicle>) : VehicleRepository {
        override suspend fun listAll(): List<Vehicle> = vehicles
        override suspend fun findById(id: DomainId): Vehicle? = vehicles.firstOrNull { it.id == id }
        override suspend fun save(vehicle: Vehicle) = Unit
        override suspend fun delete(id: DomainId) = Unit
        override suspend fun count(): Int = vehicles.size
    }

    private class FakeAverages : AveragePriceRepository {
        override suspend fun getLatestImportedSurveyWeek(): SurveyWeek? = null
        override suspend fun getPricesByMunicipality(
            state: BrazilianState,
            municipality: String,
            surveyWeek: SurveyWeek,
        ): List<AveragePrice> = emptyList()

        override suspend fun getPriceHistory(
            state: BrazilianState,
            municipality: String,
            fuelProduct: FuelProduct,
        ): List<AveragePrice> = emptyList()

        override suspend fun getStatesWithData(surveyWeek: SurveyWeek): List<BrazilianState> =
            emptyList()

        override suspend fun getMunicipalitiesWithData(
            state: BrazilianState,
            surveyWeek: SurveyWeek,
        ): List<String> = emptyList()
    }

    private inner class FakePriceTables(
        private val surveys: List<PriceSurvey>,
    ) : PriceTableRepository {
        override suspend fun getImportedPriceSurveys(): List<PriceSurvey> = surveys
        override suspend fun findPriceTableByUrl(sourceUrl: String) = null
        override suspend fun savePriceSurvey(priceSurvey: PriceSurvey) = Unit
        override suspend fun saveDiscoveredPriceTable(priceTable: com.anpfuel.domain.model.PriceTable) =
            Unit

        override suspend fun importAveragePrices(prices: List<AveragePrice>) = Unit
        override suspend fun importStationPrices(prices: List<StationPrice>) = Unit
        override suspend fun countImportedSurveyWeeks(): Int = surveys.size
        override suspend fun findPriceSurveyById(id: DomainId): PriceSurvey? =
            surveys.firstOrNull { it.id == id }

        override suspend fun findPriceSurveyByWeek(surveyWeek: SurveyWeek): PriceSurvey? =
            surveys.firstOrNull { it.surveyWeek == surveyWeek }
    }

    private inner class FakePrefs : UserPreferencesRepository {
        override suspend fun getPreferences(): UserPreferences = UserPreferences(
            preferredState = BrazilianState.PARANA,
            preferredMunicipality = "Curitiba",
            activeSurveyWeek = currentWeek,
        )

        override suspend fun savePreferences(preferences: UserPreferences) = Unit
    }

    private class FakeNotifications(
        var throwOnShow: Throwable? = null,
    ) : PriceDropAlertHistoryRepository, PriceDropNotificationRepository {
        val shown = mutableListOf<PriceDropAlertNotification>()

        override fun hasPostNotificationsPermission(): Boolean = true

        override suspend fun showPriceDropAlert(notification: PriceDropAlertNotification) {
            throwOnShow?.let { throw it }
            shown += notification
        }

        override suspend fun cancelForVehicle(vehicleId: DomainId) = Unit

        override suspend fun lastNotifiedWeek(vehicleId: DomainId): SurveyWeek? = null

        override suspend fun recordNotified(vehicleId: DomainId, week: SurveyWeek) = Unit
    }

    private fun runWorker(
        useCase: EvaluatePriceDropAlertsUseCase,
    ): ListenableWorker.Result {
        val worker = TestListenableWorkerBuilder<PriceDropEvaluationWorker>(context)
            .setWorkerFactory(
                object : WorkerFactory() {
                    override fun createWorker(
                        appContext: Context,
                        workerClassName: String,
                        workerParameters: WorkerParameters,
                    ): ListenableWorker? {
                        if (workerClassName != PriceDropEvaluationWorker::class.java.name) {
                            return null
                        }
                        return PriceDropEvaluationWorker.createForTest(
                            appContext,
                            workerParameters,
                            useCase,
                        )
                    }
                },
            )
            .build()

        return runBlocking { worker.doWork() }
    }
}
