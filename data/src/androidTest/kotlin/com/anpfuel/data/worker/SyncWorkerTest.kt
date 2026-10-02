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
import androidx.work.workDataOf
import com.anpfuel.application.sync.SyncExecutionLock
import com.anpfuel.application.usecase.settings.ApplyStationDetailRetentionUseCase
import com.anpfuel.application.usecase.sync.AutoDownloadLatestWeekUseCase
import com.anpfuel.application.usecase.sync.DiscoverSurveyWeekCatalogUseCase
import com.anpfuel.application.usecase.sync.SelectSurveyWeekUseCase
import com.anpfuel.application.usecase.sync.SelectWeekAndSyncUseCase
import com.anpfuel.application.usecase.sync.SyncPriceTablesUseCase
import com.anpfuel.domain.event.DomainEvent
import com.anpfuel.domain.event.PriceTableImported
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.PriceTableSyncGateway
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.repository.SyncJobRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.state.SyncJobState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * P24-T01 — worker wiring without mockk: mockk cannot intercept
 * final use-case methods on old ART (real code runs → NPE), so the
 * workers run for real against hand fakes. Same assertions as
 * before: success maps to success, failed outcome and concurrency
 * refusal map to failure.
 */
@RunWith(AndroidJUnit4::class)
class SyncWorkerTest {

    private lateinit var context: Context

    private val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val summaryTable = PriceTable.create(
        surveyWeek = week,
        tableType = PriceTableType.WEEKLY_SUMMARY,
        sourceUrl = "https://example.invalid/summary.xlsx",
    )

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
        val events = FakeEvents()
        val gateway = FakeGateway(
            discover = Result.success(listOf(summaryTable)),
            download = Result.success(summaryTable),
        )
        val (useCase, autoDownload) = useCases(gateway, events)

        val result = runWorker(
            useCase = useCase,
            autoDownloadLatestWeekUseCase = autoDownload,
            inputData = workDataOf(SyncWorker.KEY_SOURCE to SyncWorker.SOURCE_SCHEDULED),
        )

        assertEquals(ListenableWorker.Result.success(), result)
        assertTrue(events.published.isNotEmpty())
    }

    @Test
    fun doWork_returnsFailureWhenUseCaseFails() = runBlocking {
        val events = FakeEvents()
        val gateway = FakeGateway(
            discover = Result.success(listOf(summaryTable)),
            download = Result.failure(RuntimeException("boom")),
        )
        val (useCase, autoDownload) = useCases(gateway, events)

        val result = runWorker(
            useCase = useCase,
            autoDownloadLatestWeekUseCase = autoDownload,
            inputData = workDataOf(SyncWorker.KEY_SOURCE to SyncWorker.SOURCE_MANUAL),
        )

        assertEquals(ListenableWorker.Result.failure(), result)
    }

    @Test
    fun doWork_returnsFailureWhenConcurrencyRuleRejectsSync() = runBlocking {
        val events = FakeEvents()
        val gateway = FakeGateway(
            discover = Result.failure(DomainException("Sync already in progress")),
            download = Result.success(summaryTable),
        )
        val (useCase, autoDownload) = useCases(gateway, events)

        val result = runWorker(
            useCase = useCase,
            autoDownloadLatestWeekUseCase = autoDownload,
        )

        assertEquals(ListenableWorker.Result.failure(), result)
    }

    private fun useCases(
        gateway: FakeGateway,
        events: FakeEvents,
    ): Pair<SyncPriceTablesUseCase, AutoDownloadLatestWeekUseCase> {
        val prefs = FakePrefs()
        val priceTables = FakePriceTables()
        val syncJobs = FakeSyncJobs()
        val retention = ApplyStationDetailRetentionUseCase(
            userPreferencesRepository = prefs,
            stationPriceRepository = FakeStationPrices(),
        )
        val sync = SyncPriceTablesUseCase(
            syncJobRepository = syncJobs,
            priceTableRepository = priceTables,
            priceTableSyncGateway = gateway,
            userPreferencesRepository = prefs,
            eventPublisher = events,
            applyStationDetailRetentionUseCase = retention,
            syncExecutionLock = SyncExecutionLock(),
        )
        val autoDownload = AutoDownloadLatestWeekUseCase(
            userPreferencesRepository = prefs,
            priceTableRepository = priceTables,
            discoverSurveyWeekCatalogUseCase = DiscoverSurveyWeekCatalogUseCase(gateway),
            selectWeekAndSyncUseCase = SelectWeekAndSyncUseCase(
                selectSurveyWeekUseCase = SelectSurveyWeekUseCase(prefs, events),
                syncPriceTablesUseCase = sync,
            ),
        )
        return sync to autoDownload
    }

    private class FakePrefs : UserPreferencesRepository {
        override suspend fun getPreferences(): UserPreferences = UserPreferences(
            autoDownloadLatestWeek = false,
            syncStationDetail = false,
        )

        override suspend fun savePreferences(preferences: UserPreferences) = Unit
    }

    private inner class FakePriceTables : PriceTableRepository {
        override suspend fun getImportedPriceSurveys() = emptyList<PriceSurvey>()
        override suspend fun findPriceTableByUrl(sourceUrl: String): PriceTable? = null
        override suspend fun savePriceSurvey(priceSurvey: PriceSurvey) = Unit
        override suspend fun saveDiscoveredPriceTable(priceTable: PriceTable) = Unit
        override suspend fun importAveragePrices(prices: List<AveragePrice>) = Unit
        override suspend fun importStationPrices(prices: List<StationPrice>) = Unit
        override suspend fun countImportedSurveyWeeks(): Int = 0
        override suspend fun findPriceSurveyById(id: DomainId): PriceSurvey? = null
        override suspend fun findPriceSurveyByWeek(surveyWeek: SurveyWeek): PriceSurvey? =
            PriceSurvey.restore(
                id = DomainId.forSurveyWeek(surveyWeek),
                surveyWeek = surveyWeek,
                summaryImportedAt = null,
                stationImportedAt = null,
            )
    }

    private class FakeGateway(
        var discover: Result<List<PriceTable>>,
        var download: Result<PriceTable>,
    ) : PriceTableSyncGateway {
        override suspend fun discoverPriceTables(): List<PriceTable> = discover.getOrThrow()

        override suspend fun discoverSurveyWeekCatalog(): List<SurveyWeekCatalogEntry> =
            emptyList()

        override suspend fun downloadPriceTable(priceTable: PriceTable): PriceTable =
            download.getOrThrow()

        override suspend fun importWeeklySummary(priceTable: PriceTable): PriceTableImported.Payload =
            PriceTableImported.Payload(
                surveyWeekId = DomainId.forSurveyWeek(priceTable.surveyWeek),
                tableType = PriceTableType.WEEKLY_SUMMARY,
                rowCount = 1,
            )

        override suspend fun importStationDetail(priceTable: PriceTable): PriceTableImported.Payload =
            throw UnsupportedOperationException("station detail out of scope")
    }

    private class FakeSyncJobs : SyncJobRepository {
        private var state: SyncJobState = SyncJobState.IDLE
        override suspend fun getCurrentState(): SyncJobState = state
        override suspend fun saveState(state: SyncJobState) {
            this.state = state
        }
    }

    private class FakeEvents : DomainEventPublisher {
        val published = mutableListOf<DomainEvent>()
        override suspend fun publish(event: DomainEvent) {
            published += event
        }
    }

    private class FakeStationPrices : StationPriceRepository {
        override suspend fun getStationPrices(
            state: com.anpfuel.domain.valueobject.BrazilianState,
            municipality: String,
            fuelProduct: FuelProduct,
            surveyWeek: SurveyWeek,
        ): List<StationPrice> = emptyList()

        override suspend fun hasStationData(
            surveyWeek: SurveyWeek,
            state: com.anpfuel.domain.valueobject.BrazilianState,
            municipality: String,
        ): Boolean = false

        override suspend fun deleteStationPricesOlderThanRetention(retentionWeeks: Int) = Unit
    }

    private fun runWorker(
        useCase: SyncPriceTablesUseCase,
        autoDownloadLatestWeekUseCase: AutoDownloadLatestWeekUseCase,
        inputData: androidx.work.Data = workDataOf(
            SyncWorker.KEY_SOURCE to SyncWorker.SOURCE_SCHEDULED,
        ),
    ): ListenableWorker.Result {
        val worker = TestListenableWorkerBuilder<SyncWorker>(context)
            .setWorkerFactory(
                object : WorkerFactory() {
                    override fun createWorker(
                        appContext: Context,
                        workerClassName: String,
                        workerParameters: WorkerParameters,
                    ): ListenableWorker? {
                        if (workerClassName != SyncWorker::class.java.name) {
                            return null
                        }
                        return SyncWorker.createForTest(
                            appContext,
                            workerParameters,
                            useCase,
                            autoDownloadLatestWeekUseCase,
                        )
                    }
                },
            )
            .setInputData(inputData)
            .build()

        return runBlocking { worker.doWork() }
    }
}
