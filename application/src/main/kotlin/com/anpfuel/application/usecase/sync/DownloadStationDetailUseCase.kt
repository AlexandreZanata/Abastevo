package com.anpfuel.application.usecase.sync

import com.anpfuel.application.error.AppError
import com.anpfuel.application.error.AppErrorResolver
import com.anpfuel.domain.event.DomainEvent
import com.anpfuel.domain.event.PriceTableDownloaded
import com.anpfuel.domain.event.PriceTableImported
import com.anpfuel.domain.event.StationDetailRequested
import com.anpfuel.domain.event.SyncJobCompleted
import com.anpfuel.domain.event.SyncJobCompletedPayload
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.PriceTableSyncGateway
import com.anpfuel.domain.repository.SyncJobRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.ActiveSurveyWeekRule
import com.anpfuel.domain.rule.SyncJobConcurrencyRule
import com.anpfuel.domain.state.SyncJobState
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek

/**
 * UC-007 subset — On-demand station detail download for a municipality and survey week.
 */
data class DownloadStationDetailResult(
    val outcome: SyncJobOutcome,
    val error: AppError? = null,
    val stationDetailRequested: StationDetailRequested,
    val events: List<DomainEvent> = emptyList(),
)

class DownloadStationDetailUseCase(
    private val syncJobRepository: SyncJobRepository,
    private val priceTableRepository: PriceTableRepository,
    private val priceTableSyncGateway: PriceTableSyncGateway,
    private val userPreferencesRepository: UserPreferencesRepository,
    private val eventPublisher: DomainEventPublisher,
) {

    suspend operator fun invoke(
        state: BrazilianState,
        municipality: String,
        surveyWeek: SurveyWeek? = null,
    ): DownloadStationDetailResult {
        val events = mutableListOf<DomainEvent>()
        val normalizedMunicipality = municipality.trim()
        require(normalizedMunicipality.isNotBlank()) { "Municipality must not be blank" }

        prepareForSync()

        val resolvedSurveyWeek = surveyWeek ?: resolveDisplaySurveyWeek()
        val stationDetailRequested = StationDetailRequested.create(
            payload = StationDetailRequested.Payload(
                surveyWeekId = DomainId.forSurveyWeek(resolvedSurveyWeek),
                municipality = normalizedMunicipality,
                state = state,
            ),
        )
        events += stationDetailRequested
        eventPublisher.publish(stationDetailRequested)

        transitionTo(SyncJobState.DISCOVERING)

        return try {
            val stationTable = findStationTableForWeek(resolvedSurveyWeek)
            syncStationTable(stationTable, events)
        } catch (error: DomainException) {
            throw error
        } catch (error: Throwable) {
            failDownload(error, stationDetailRequested, events)
        }
    }

    private suspend fun syncStationTable(
        stationTable: PriceTable,
        events: MutableList<DomainEvent>,
    ): DownloadStationDetailResult {
        val stationDetailRequested = events.filterIsInstance<StationDetailRequested>().single()

        transitionTo(SyncJobState.DOWNLOADING)
        val downloaded = priceTableSyncGateway.downloadPriceTable(stationTable)
        priceTableRepository.saveDiscoveredPriceTable(downloaded)
        publishDownloaded(downloaded, events)

        transitionTo(SyncJobState.PARSING)
        transitionTo(SyncJobState.IMPORTING)
        val payload = priceTableSyncGateway.importStationDetail(downloaded)
        publishImported(payload, events)

        return completeDownload(
            outcome = SyncJobOutcome.SUCCESS,
            stationDetailRequested = stationDetailRequested,
            events = events,
        )
    }

    private suspend fun findStationTableForWeek(surveyWeek: SurveyWeek): PriceTable {
        val discovered = priceTableSyncGateway.discoverPriceTables()
        return discovered.firstOrNull { table ->
            table.tableType == PriceTableType.STATION_DETAIL && table.surveyWeek == surveyWeek
        } ?: throw DomainException(
            "No STATION_DETAIL PriceTable discovered for survey week ending ${surveyWeek.endDate}",
        )
    }

    private suspend fun failDownload(
        error: Throwable,
        stationDetailRequested: StationDetailRequested,
        events: MutableList<DomainEvent>,
    ): DownloadStationDetailResult {
        val appError = AppErrorResolver.fromThrowable(error)
        transitionTo(SyncJobState.FAILED)
        val completed = SyncJobCompleted.create(
            payload = SyncJobCompletedPayload(
                finalState = SyncJobState.FAILED,
                outcome = SyncJobOutcome.FAILED,
                detail = appError.code,
            ),
        )
        events += completed
        eventPublisher.publish(completed)
        transitionTo(SyncJobState.IDLE)

        return DownloadStationDetailResult(
            outcome = SyncJobOutcome.FAILED,
            error = appError,
            stationDetailRequested = stationDetailRequested,
            events = events.toList(),
        )
    }

    private suspend fun completeDownload(
        outcome: SyncJobOutcome,
        stationDetailRequested: StationDetailRequested,
        events: MutableList<DomainEvent>,
    ): DownloadStationDetailResult {
        transitionTo(SyncJobState.COMPLETED)
        val completed = SyncJobCompleted.create(
            payload = SyncJobCompletedPayload(
                finalState = SyncJobState.COMPLETED,
                outcome = outcome,
            ),
        )
        events += completed
        eventPublisher.publish(completed)
        transitionTo(SyncJobState.IDLE)

        return DownloadStationDetailResult(
            outcome = outcome,
            stationDetailRequested = stationDetailRequested,
            events = events.toList(),
        )
    }

    private suspend fun publishDownloaded(
        table: PriceTable,
        events: MutableList<DomainEvent>,
    ) {
        val event = PriceTableDownloaded.create(
            payload = PriceTableDownloaded.Payload(
                surveyWeekId = DomainId.forSurveyWeek(table.surveyWeek),
                tableType = table.tableType,
                localPath = table.sourceUrl.substringAfterLast('/'),
                sourceUrl = table.sourceUrl,
            ),
        )
        events += event
        eventPublisher.publish(event)
    }

    private suspend fun publishImported(
        payload: PriceTableImported.Payload,
        events: MutableList<DomainEvent>,
    ) {
        val event = PriceTableImported.create(payload = payload)
        events += event
        eventPublisher.publish(event)
    }

    private suspend fun prepareForSync() {
        val current = syncJobRepository.getCurrentState()
        val recovered = SyncJobConcurrencyRule.recoverOrphanedActiveState(current)
        if (recovered != current) {
            syncJobRepository.saveState(recovered)
        } else if (current.isTerminal) {
            transitionTo(SyncJobState.IDLE)
        }
        SyncJobConcurrencyRule.validateCanStartSync(syncJobRepository.getCurrentState())
    }

    private suspend fun resolveDisplaySurveyWeek(): SurveyWeek {
        val preferences = userPreferencesRepository.getPreferences()
        val importedSurveys = priceTableRepository.getImportedPriceSurveys()
        return ActiveSurveyWeekRule.resolveDisplayWeek(
            activeSurveyWeek = preferences.activeSurveyWeek,
            importedSurveys = importedSurveys,
        ) ?: throw DomainException("BR-006: No successfully imported SurveyWeek is available")
    }

    private suspend fun transitionTo(target: SyncJobState) {
        val next = syncJobRepository.getCurrentState().transitionTo(target)
        syncJobRepository.saveState(next)
    }
}
