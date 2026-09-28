package com.anpfuel.application.usecase.sync

import com.anpfuel.application.error.AppError
import com.anpfuel.application.sync.SyncExecutionLock
import com.anpfuel.application.usecase.settings.ApplyStationDetailRetentionUseCase
import com.anpfuel.domain.event.DomainEvent
import com.anpfuel.domain.event.PriceTableDiscovered
import com.anpfuel.domain.event.PriceTableDownloaded
import com.anpfuel.domain.event.PriceTableImported
import com.anpfuel.domain.event.SyncJobCompleted
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.event.SyncRequestSource
import com.anpfuel.domain.event.SyncRequested
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.PriceTableSyncGateway
import com.anpfuel.domain.repository.SyncJobRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.state.SyncJobState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertInstanceOf
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import java.io.IOException
import java.time.Instant

class SyncPriceTablesUseCaseTest {

    private val syncJobRepository = mockk<SyncJobRepository>()
    private val priceTableRepository = mockk<PriceTableRepository>(relaxed = true)
    private val priceTableSyncGateway = mockk<PriceTableSyncGateway>()
    private val userPreferencesRepository = mockk<UserPreferencesRepository>()
    private val eventPublisher = mockk<DomainEventPublisher>()
    private val applyStationDetailRetentionUseCase = mockk<ApplyStationDetailRetentionUseCase>()

    private lateinit var useCase: SyncPriceTablesUseCase
    private lateinit var syncState: SyncJobState
    private val publishedEvents = mutableListOf<DomainEvent>()

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val summaryUrl =
        "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
            "arquivos-lpc/2026/resumo_semanal_lpc_2026-06-07_2026-06-13.xlsx"
    private val stationUrl =
        "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
            "arquivos-lpc/2026/revendas_lpc_2026-06-07_2026-06-13.xlsx"

    @BeforeEach
    fun setUp() {
        syncState = SyncJobState.IDLE
        publishedEvents.clear()

        coEvery { syncJobRepository.getCurrentState() } answers { syncState }
        coEvery { syncJobRepository.saveState(any()) } answers {
            syncState = firstArg<SyncJobState>()
        }
        coEvery { eventPublisher.publish(any()) } answers {
            publishedEvents += firstArg<DomainEvent>()
        }
        coEvery { applyStationDetailRetentionUseCase.invoke() } returns Unit

        useCase = SyncPriceTablesUseCase(
            syncJobRepository = syncJobRepository,
            priceTableRepository = priceTableRepository,
            priceTableSyncGateway = priceTableSyncGateway,
            userPreferencesRepository = userPreferencesRepository,
            eventPublisher = eventPublisher,
            applyStationDetailRetentionUseCase = applyStationDetailRetentionUseCase,
            syncExecutionLock = SyncExecutionLock(),
        )
    }

    @Test
    fun usesPreDiscoveredWeekTablesWithoutListingDiscovery() = runTest {
        val preDiscovered = latestWeekTables()
        val downloadedSummary = preDiscovered.first().copy(checksum = "summary-sha")
        val downloadedStation = preDiscovered.last().copy(checksum = "station-sha")

        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(syncStationDetail = true)
        coEvery { priceTableRepository.findPriceSurveyByWeek(surveyWeek) } returns null
        coEvery { priceTableRepository.findPriceTableByUrl(any()) } returns null
        coEvery { priceTableSyncGateway.downloadPriceTable(preDiscovered.first()) } returns downloadedSummary
        coEvery { priceTableSyncGateway.downloadPriceTable(preDiscovered.last()) } returns downloadedStation
        coEvery { priceTableSyncGateway.importWeeklySummary(downloadedSummary) } returns summaryImportPayload()
        coEvery { priceTableSyncGateway.importStationDetail(downloadedStation) } returns stationImportPayload()

        val result = useCase.invoke(
            source = SyncRequestSource.FIRST_LAUNCH,
            targetSurveyWeek = surveyWeek,
            preDiscoveredWeekTables = preDiscovered,
        )

        assertEquals(SyncJobOutcome.SUCCESS, result.outcome)
        coVerify(exactly = 0) { priceTableSyncGateway.discoverPriceTables() }
    }

    @Test
    fun happyPathDiscoversDownloadsParsesAndImportsLatestWeek() = runTest {
        val discovered = latestWeekTables()
        val downloadedSummary = discovered.first().copy(checksum = "summary-sha")
        val downloadedStation = discovered.last().copy(checksum = "station-sha")

        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(syncStationDetail = true)
        coEvery { priceTableSyncGateway.discoverPriceTables() } returns discovered
        coEvery { priceTableRepository.findPriceSurveyByWeek(surveyWeek) } returns null
        coEvery { priceTableRepository.findPriceTableByUrl(any()) } returns null
        coEvery { priceTableSyncGateway.downloadPriceTable(discovered.first()) } returns downloadedSummary
        coEvery { priceTableSyncGateway.downloadPriceTable(discovered.last()) } returns downloadedStation
        coEvery { priceTableSyncGateway.importWeeklySummary(downloadedSummary) } returns summaryImportPayload()
        coEvery { priceTableSyncGateway.importStationDetail(downloadedStation) } returns stationImportPayload()

        val result = useCase.invoke(SyncRequestSource.MANUAL)

        assertEquals(SyncJobOutcome.SUCCESS, result.outcome)
        assertEquals(null, result.error)
        assertInstanceOf(SyncRequested::class.java, result.events.first())
        assertEquals(2, result.events.filterIsInstance<PriceTableDiscovered>().size)
        assertEquals(2, result.events.filterIsInstance<PriceTableDownloaded>().size)
        assertEquals(2, result.events.filterIsInstance<PriceTableImported>().size)
        assertInstanceOf(SyncJobCompleted::class.java, result.events.last())

        coVerify(exactly = 1) { priceTableSyncGateway.importWeeklySummary(downloadedSummary) }
        coVerify(exactly = 1) { priceTableSyncGateway.importStationDetail(downloadedStation) }
        coVerify(exactly = 2) { priceTableRepository.saveDiscoveredPriceTable(any()) }
        coVerify(exactly = 1) { applyStationDetailRetentionUseCase.invoke() }
    }

    @Test
    fun returnsSyncNoNewDataWhenLatestWeekAlreadyImportedWithChecksum() = runTest {
        val discovered = latestWeekTables()
        val importedSurvey = PriceSurvey.restore(
            id = DomainId.forSurveyWeek(surveyWeek),
            surveyWeek = surveyWeek,
            summaryImportedAt = Instant.parse("2026-06-14T10:00:00Z"),
            stationImportedAt = Instant.parse("2026-06-14T10:05:00Z"),
        )

        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(syncStationDetail = true)
        coEvery { priceTableSyncGateway.discoverPriceTables() } returns discovered
        coEvery { priceTableRepository.findPriceSurveyByWeek(surveyWeek) } returns importedSurvey
        coEvery { priceTableRepository.findPriceTableByUrl(summaryUrl) } returns discovered.first()
            .copy(checksum = "summary-sha")
        coEvery { priceTableRepository.findPriceTableByUrl(stationUrl) } returns discovered.last()
            .copy(checksum = "station-sha")

        val result = useCase.invoke(SyncRequestSource.SCHEDULED)

        assertEquals(SyncJobOutcome.NO_NEW_DATA, result.outcome)
        assertEquals(AppError.SyncNoNewData, result.error)
        coVerify(exactly = 0) { priceTableSyncGateway.downloadPriceTable(any()) }
        coVerify(exactly = 0) { priceTableSyncGateway.importWeeklySummary(any()) }
        coVerify(exactly = 0) { priceTableSyncGateway.importStationDetail(any()) }
    }

    @Test
    fun networkFailurePreservesCacheAndMapsSyncNetworkError() = runTest {
        val discovered = listOf(latestWeekTables().first())

        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(syncStationDetail = false)
        coEvery { priceTableSyncGateway.discoverPriceTables() } returns discovered
        coEvery { priceTableRepository.findPriceSurveyByWeek(surveyWeek) } returns null
        coEvery { priceTableRepository.findPriceTableByUrl(summaryUrl) } returns null
        coEvery { priceTableSyncGateway.downloadPriceTable(any()) } throws IOException("HTTP 503")

        val result = useCase.invoke(SyncRequestSource.MANUAL)

        assertEquals(SyncJobOutcome.FAILED, result.outcome)
        assertEquals(AppError.SyncNetworkError, result.error)
        coVerify(exactly = 0) { priceTableSyncGateway.importWeeklySummary(any()) }
        coVerify(exactly = 0) { priceTableSyncGateway.importStationDetail(any()) }
        coVerify(exactly = 0) { priceTableRepository.importAveragePrices(any()) }
        coVerify(exactly = 0) { priceTableRepository.importStationPrices(any()) }
    }

    @Test
    fun recoversOrphanedActiveSyncStateBeforeStartingSync() = runTest {
        syncState = SyncJobState.DISCOVERING
        val discovered = latestWeekTables()
        val downloadedSummary = discovered.first().copy(checksum = "summary-sha")
        val downloadedStation = discovered.last().copy(checksum = "station-sha")

        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(syncStationDetail = true)
        coEvery { priceTableSyncGateway.discoverPriceTables() } returns discovered
        coEvery { priceTableRepository.findPriceSurveyByWeek(surveyWeek) } returns null
        coEvery { priceTableRepository.findPriceTableByUrl(any()) } returns null
        coEvery { priceTableSyncGateway.downloadPriceTable(discovered.first()) } returns downloadedSummary
        coEvery { priceTableSyncGateway.downloadPriceTable(discovered.last()) } returns downloadedStation
        coEvery { priceTableSyncGateway.importWeeklySummary(downloadedSummary) } returns summaryImportPayload()
        coEvery { priceTableSyncGateway.importStationDetail(downloadedStation) } returns stationImportPayload()

        val result = useCase.invoke(SyncRequestSource.MANUAL)

        assertEquals(SyncJobOutcome.SUCCESS, result.outcome)
        coVerify(exactly = 1) { priceTableSyncGateway.discoverPriceTables() }
    }

    @Test
    fun syncsTargetSurveyWeekWhenExplicitlyProvided() = runTest {
        val olderWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")
        val olderSummaryUrl =
            "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
                "arquivos-lpc/2026/resumo_semanal_lpc_2026-05-31_2026-06-06.xlsx"
        val olderStationUrl =
            "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
                "arquivos-lpc/2026/revendas_lpc_2026-05-31_2026-06-06.xlsx"
        val discovered = latestWeekTables() + listOf(
            PriceTable.create(
                surveyWeek = olderWeek,
                tableType = PriceTableType.WEEKLY_SUMMARY,
                sourceUrl = olderSummaryUrl,
            ),
            PriceTable.create(
                surveyWeek = olderWeek,
                tableType = PriceTableType.STATION_DETAIL,
                sourceUrl = olderStationUrl,
            ),
        )
        val downloadedSummary = discovered[2].copy(checksum = "older-summary-sha")
        val downloadedStation = discovered[3].copy(checksum = "older-station-sha")

        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(syncStationDetail = true)
        coEvery { priceTableSyncGateway.discoverPriceTables() } returns discovered
        coEvery { priceTableRepository.findPriceSurveyByWeek(olderWeek) } returns null
        coEvery { priceTableRepository.findPriceTableByUrl(any()) } returns null
        coEvery { priceTableSyncGateway.downloadPriceTable(discovered[2]) } returns downloadedSummary
        coEvery { priceTableSyncGateway.downloadPriceTable(discovered[3]) } returns downloadedStation
        coEvery { priceTableSyncGateway.importWeeklySummary(downloadedSummary) } returns
            summaryImportPayload(olderWeek)
        coEvery { priceTableSyncGateway.importStationDetail(downloadedStation) } returns
            stationImportPayload(olderWeek)

        val result = useCase.invoke(SyncRequestSource.MANUAL, targetSurveyWeek = olderWeek)

        assertEquals(SyncJobOutcome.SUCCESS, result.outcome)
        assertEquals(2, result.events.filterIsInstance<PriceTableDiscovered>().size)
        coVerify(exactly = 0) { priceTableSyncGateway.downloadPriceTable(discovered.first()) }
        coVerify(exactly = 1) { priceTableSyncGateway.downloadPriceTable(discovered[2]) }
        coVerify(exactly = 1) { priceTableSyncGateway.downloadPriceTable(discovered[3]) }
    }

    @Test
    fun usesActiveSurveyWeekFromPreferencesWhenTargetNotProvided() = runTest {
        val olderWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")
        val olderSummaryUrl =
            "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
                "arquivos-lpc/2026/resumo_semanal_lpc_2026-05-31_2026-06-06.xlsx"
        val olderSummary = PriceTable.create(
            surveyWeek = olderWeek,
            tableType = PriceTableType.WEEKLY_SUMMARY,
            sourceUrl = olderSummaryUrl,
        )
        val discovered = latestWeekTables() + olderSummary
        val downloadedSummary = olderSummary.copy(checksum = "older-summary-sha")

        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            syncStationDetail = false,
            autoDownloadLatestWeek = false,
            activeSurveyWeek = olderWeek,
        )
        coEvery { priceTableSyncGateway.discoverPriceTables() } returns discovered
        coEvery { priceTableRepository.findPriceSurveyByWeek(olderWeek) } returns null
        coEvery { priceTableRepository.findPriceTableByUrl(olderSummaryUrl) } returns null
        coEvery { priceTableSyncGateway.downloadPriceTable(olderSummary) } returns downloadedSummary
        coEvery { priceTableSyncGateway.importWeeklySummary(downloadedSummary) } returns
            summaryImportPayload(olderWeek)

        val result = useCase.invoke(SyncRequestSource.MANUAL)

        assertEquals(SyncJobOutcome.SUCCESS, result.outcome)
        coVerify(exactly = 1) { priceTableSyncGateway.downloadPriceTable(olderSummary) }
        coVerify(exactly = 0) { priceTableSyncGateway.downloadPriceTable(discovered.first()) }
    }

    @Test
    fun throwsWhenTargetSurveyWeekMissingFromDiscovery() = runTest {
        val missingWeek = SurveyWeek.fromIsoDates("2026-01-01", "2026-01-07")

        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences()
        coEvery { priceTableSyncGateway.discoverPriceTables() } returns latestWeekTables()

        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking {
                useCase.invoke(SyncRequestSource.MANUAL, targetSurveyWeek = missingWeek)
            }
        }

        coVerify(exactly = 0) { priceTableSyncGateway.downloadPriceTable(any()) }
    }

    private fun latestWeekTables(): List<PriceTable> = listOf(
        PriceTable.create(
            surveyWeek = surveyWeek,
            tableType = PriceTableType.WEEKLY_SUMMARY,
            sourceUrl = summaryUrl,
        ),
        PriceTable.create(
            surveyWeek = surveyWeek,
            tableType = PriceTableType.STATION_DETAIL,
            sourceUrl = stationUrl,
        ),
    )

    private fun summaryImportPayload(
        surveyWeek: SurveyWeek = this.surveyWeek,
    ) = PriceTableImported.Payload(
        surveyWeekId = DomainId.forSurveyWeek(surveyWeek),
        tableType = PriceTableType.WEEKLY_SUMMARY,
        rowCount = 2344,
    )

    private fun stationImportPayload(
        surveyWeek: SurveyWeek = this.surveyWeek,
    ) = PriceTableImported.Payload(
        surveyWeekId = DomainId.forSurveyWeek(surveyWeek),
        tableType = PriceTableType.STATION_DETAIL,
        rowCount = 19676,
    )
}
