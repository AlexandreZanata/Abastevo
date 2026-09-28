package com.anpfuel.application.usecase.sync

import com.anpfuel.domain.event.PriceTableImported
import com.anpfuel.domain.event.StationDetailRequested
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.PriceTableSyncGateway
import com.anpfuel.domain.repository.SyncJobRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.state.SyncJobState
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertInstanceOf
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import java.io.IOException

class DownloadStationDetailUseCaseTest {

    private val syncJobRepository = mockk<SyncJobRepository>()
    private val priceTableRepository = mockk<PriceTableRepository>(relaxed = true)
    private val priceTableSyncGateway = mockk<PriceTableSyncGateway>()
    private val userPreferencesRepository = mockk<UserPreferencesRepository>()
    private val eventPublisher = mockk<DomainEventPublisher>()

    private lateinit var useCase: DownloadStationDetailUseCase
    private lateinit var syncState: SyncJobState

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val state = BrazilianState.SAO_PAULO
    private val municipality = "São Paulo"
    private val stationUrl =
        "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/" +
            "arquivos-lpc/2026/revendas_lpc_2026-06-07_2026-06-13.xlsx"

    @BeforeEach
    fun setUp() {
        syncState = SyncJobState.IDLE
        useCase = DownloadStationDetailUseCase(
            syncJobRepository = syncJobRepository,
            priceTableRepository = priceTableRepository,
            priceTableSyncGateway = priceTableSyncGateway,
            userPreferencesRepository = userPreferencesRepository,
            eventPublisher = eventPublisher,
        )

        coEvery { syncJobRepository.getCurrentState() } answers { syncState }
        coEvery { syncJobRepository.saveState(any()) } answers {
            syncState = firstArg<SyncJobState>()
        }
        coEvery { eventPublisher.publish(any()) } returns Unit
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences()
        coEvery { priceTableRepository.getImportedPriceSurveys() } returns listOf(
            PriceSurvey.restore(
                id = DomainId.forSurveyWeek(surveyWeek),
                surveyWeek = surveyWeek,
                summaryImportedAt = java.time.Instant.parse("2026-06-14T10:00:00Z"),
                stationImportedAt = null,
            ),
        )
    }

    @Test
    fun downloadsAndImportsStationDetailOnDemand() = runTest {
        val stationTable = PriceTable.create(
            surveyWeek = surveyWeek,
            tableType = PriceTableType.STATION_DETAIL,
            sourceUrl = stationUrl,
        )
        val downloaded = stationTable.copy(checksum = "station-sha")
        val importPayload = PriceTableImported.Payload(
            surveyWeekId = DomainId.forSurveyWeek(surveyWeek),
            tableType = PriceTableType.STATION_DETAIL,
            rowCount = 19676,
        )

        coEvery { priceTableSyncGateway.discoverPriceTables() } returns listOf(stationTable)
        coEvery { priceTableSyncGateway.downloadPriceTable(stationTable) } returns downloaded
        coEvery { priceTableSyncGateway.importStationDetail(downloaded) } returns importPayload

        val result = useCase.invoke(state = state, municipality = municipality)

        assertEquals(SyncJobOutcome.SUCCESS, result.outcome)
        assertEquals(municipality, result.stationDetailRequested.payload.municipality)
        assertEquals(state, result.stationDetailRequested.payload.state)
        assertInstanceOf(StationDetailRequested::class.java, result.events.first())
        coVerify(exactly = 1) { priceTableSyncGateway.importStationDetail(downloaded) }
        coVerify(atLeast = 1) { eventPublisher.publish(any<StationDetailRequested>()) }
    }

    @Test
    fun recoversOrphanedActiveSyncStateBeforeStartingDownload() = runTest {
        syncState = SyncJobState.IMPORTING
        val stationTable = PriceTable.create(
            surveyWeek = surveyWeek,
            tableType = PriceTableType.STATION_DETAIL,
            sourceUrl = stationUrl,
        )
        val downloaded = stationTable.copy(checksum = "station-sha")
        val importPayload = PriceTableImported.Payload(
            surveyWeekId = DomainId.forSurveyWeek(surveyWeek),
            tableType = PriceTableType.STATION_DETAIL,
            rowCount = 19676,
        )

        coEvery { priceTableSyncGateway.discoverPriceTables() } returns listOf(stationTable)
        coEvery { priceTableSyncGateway.downloadPriceTable(stationTable) } returns downloaded
        coEvery { priceTableSyncGateway.importStationDetail(downloaded) } returns importPayload

        val result = useCase.invoke(state = state, municipality = municipality)

        assertEquals(SyncJobOutcome.SUCCESS, result.outcome)
        coVerify(exactly = 1) { priceTableSyncGateway.discoverPriceTables() }
    }

    @Test
    fun mapsDownloadFailureWithoutClearingExistingCache() = runTest {
        val stationTable = PriceTable.create(
            surveyWeek = surveyWeek,
            tableType = PriceTableType.STATION_DETAIL,
            sourceUrl = stationUrl,
        )

        coEvery { priceTableSyncGateway.discoverPriceTables() } returns listOf(stationTable)
        coEvery { priceTableSyncGateway.downloadPriceTable(stationTable) } throws IOException("HTTP 503")

        val result = useCase.invoke(state = state, municipality = municipality)

        assertEquals(SyncJobOutcome.FAILED, result.outcome)
        coVerify(exactly = 0) { priceTableSyncGateway.importStationDetail(any()) }
    }
}
