package com.anpfuel.application.usecase.sync

import com.anpfuel.application.error.AppError
import com.anpfuel.application.error.AppErrorResolver
import com.anpfuel.application.sync.SyncExecutionLock
import com.anpfuel.application.usecase.settings.ApplyStationDetailRetentionUseCase
import com.anpfuel.domain.event.DomainEvent
import com.anpfuel.domain.event.PriceTableDiscovered
import com.anpfuel.domain.event.PriceTableDownloaded
import com.anpfuel.domain.event.PriceTableImportFailed
import com.anpfuel.domain.event.PriceTableImported
import com.anpfuel.domain.event.SyncJobCompleted
import com.anpfuel.domain.event.SyncJobCompletedPayload
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.event.SyncRequestSource
import com.anpfuel.domain.event.SyncRequested
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.PriceTableSyncGateway
import com.anpfuel.domain.repository.SyncJobRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.rule.AutoDownloadLatestWeekRule
import com.anpfuel.domain.rule.SyncJobConcurrencyRule
import com.anpfuel.domain.state.SyncJobState
import com.anpfuel.domain.sync.PriceTableSyncPlanner
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek

data class SyncPriceTablesResult(
    val outcome: SyncJobOutcome,
    val error: AppError? = null,
    val events: List<DomainEvent> = emptyList(),
)

class SyncPriceTablesUseCase(
    private val syncJobRepository: SyncJobRepository,
    private val priceTableRepository: PriceTableRepository,
    private val priceTableSyncGateway: PriceTableSyncGateway,
    private val userPreferencesRepository: UserPreferencesRepository,
    private val eventPublisher: DomainEventPublisher,
    private val applyStationDetailRetentionUseCase: ApplyStationDetailRetentionUseCase,
    private val syncExecutionLock: SyncExecutionLock,
) {

    suspend operator fun invoke(
        source: SyncRequestSource,
        targetSurveyWeek: SurveyWeek? = null,
        preDiscoveredWeekTables: List<PriceTable>? = null,
    ): SyncPriceTablesResult = syncExecutionLock.withLock {
        invokeInternal(
            source = source,
            targetSurveyWeek = targetSurveyWeek,
            preDiscoveredWeekTables = preDiscoveredWeekTables,
        )
    }

    private suspend fun invokeInternal(
        source: SyncRequestSource,
        targetSurveyWeek: SurveyWeek?,
        preDiscoveredWeekTables: List<PriceTable>?,
    ): SyncPriceTablesResult {
        val events = mutableListOf<DomainEvent>()

        suspend fun publish(event: DomainEvent) {
            events += event
            eventPublisher.publish(event)
        }

        prepareForSync()

        publish(
            SyncRequested.create(
                payload = SyncRequested.Payload(source = source),
            ),
        )

        transitionTo(SyncJobState.DISCOVERING)

        return try {
            val discovered = preDiscoveredWeekTables ?: priceTableSyncGateway.discoverPriceTables()
            val preferences = userPreferencesRepository.getPreferences()
            val effectiveTargetWeek = targetSurveyWeek ?: if (
                AutoDownloadLatestWeekRule.shouldResolveSyncTargetFromCatalogLatest(
                    autoDownloadLatestWeek = preferences.autoDownloadLatestWeek,
                    explicitTargetSurveyWeek = targetSurveyWeek,
                )
            ) {
                null
            } else {
                preferences.activeSurveyWeek
            }
            val weekTables = resolveWeekTables(discovered, effectiveTargetWeek)
            weekTables.forEach { table ->
                publish(
                    PriceTableDiscovered.create(
                        payload = PriceTableDiscovered.Payload(
                            url = table.sourceUrl,
                            tableType = table.tableType,
                            surveyWeek = table.surveyWeek,
                        ),
                    ),
                )
            }

            val candidateTables = PriceTableSyncPlanner.resolveTablesForSync(
                weekTables = weekTables,
                syncStationDetail = preferences.syncStationDetail,
            )
            val pendingTables = candidateTables.filter { isPendingImport(it) }

            if (pendingTables.isEmpty()) {
                completeWithNoNewData(events)
            } else {
                syncPendingTables(pendingTables, events)
            }
        } catch (error: DomainException) {
            throw error
        } catch (error: Throwable) {
            failSync(error, events)
        }
    }

    private fun resolveWeekTables(
        discovered: List<PriceTable>,
        targetSurveyWeek: SurveyWeek?,
    ): List<PriceTable> {
        if (targetSurveyWeek != null) {
            val weekTables = PriceTableSyncPlanner.selectWeekTables(discovered, targetSurveyWeek)
            if (weekTables.none { it.tableType == PriceTableType.WEEKLY_SUMMARY }) {
                throw DomainException(
                    "Target survey week not found in ANP listing: $targetSurveyWeek",
                )
            }
            return weekTables
        }
        return PriceTableSyncPlanner.selectLatestWeekTables(discovered)
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

    private suspend fun syncPendingTables(
        pendingTables: List<PriceTable>,
        events: MutableList<DomainEvent>,
    ): SyncPriceTablesResult {
        transitionTo(SyncJobState.DOWNLOADING)

        val downloadedTables = pendingTables.map { table ->
            val downloaded = priceTableSyncGateway.downloadPriceTable(table)
            priceTableRepository.saveDiscoveredPriceTable(downloaded)
            publishDownloaded(downloaded, events)
            downloaded
        }

        transitionTo(SyncJobState.PARSING)
        transitionTo(SyncJobState.IMPORTING)

        var summaryImported = false
        var stationImported = false
        var lastError: AppError? = null

        downloadedTables.forEach { table ->
            try {
                when (table.tableType) {
                    PriceTableType.WEEKLY_SUMMARY -> {
                        val payload = priceTableSyncGateway.importWeeklySummary(table)
                        summaryImported = true
                        publishImported(table, payload, events)
                    }

                    PriceTableType.STATION_DETAIL -> {
                        val payload = priceTableSyncGateway.importStationDetail(table)
                        stationImported = true
                        publishImported(table, payload, events)
                    }
                }
            } catch (error: Throwable) {
                lastError = AppErrorResolver.fromThrowable(error)
                publishImportFailed(table, error, events)
            }
        }

        if (stationImported) {
            applyStationDetailRetentionUseCase()
        }

        val outcome = when {
            summaryImported && (!requiresStationImport(pendingTables) || stationImported) ->
                SyncJobOutcome.SUCCESS

            summaryImported -> SyncJobOutcome.PARTIAL
            else -> SyncJobOutcome.FAILED
        }

        return completeSync(outcome, lastError, events)
    }

    private suspend fun completeWithNoNewData(
        events: MutableList<DomainEvent>,
    ): SyncPriceTablesResult {
        transitionTo(SyncJobState.COMPLETED)
        val completed = SyncJobCompleted.create(
            payload = SyncJobCompletedPayload(
                finalState = SyncJobState.COMPLETED,
                outcome = SyncJobOutcome.NO_NEW_DATA,
                detail = AppError.SyncNoNewData.code,
            ),
        )
        events += completed
        eventPublisher.publish(completed)
        transitionTo(SyncJobState.IDLE)

        return SyncPriceTablesResult(
            outcome = SyncJobOutcome.NO_NEW_DATA,
            error = AppError.SyncNoNewData,
            events = events.toList(),
        )
    }

    private suspend fun failSync(
        error: Throwable,
        events: MutableList<DomainEvent>,
    ): SyncPriceTablesResult {
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

        return SyncPriceTablesResult(
            outcome = SyncJobOutcome.FAILED,
            error = appError,
            events = events.toList(),
        )
    }

    private suspend fun completeSync(
        outcome: SyncJobOutcome,
        error: AppError?,
        events: MutableList<DomainEvent>,
    ): SyncPriceTablesResult {
        transitionTo(SyncJobState.COMPLETED)
        val completed = SyncJobCompleted.create(
            payload = SyncJobCompletedPayload(
                finalState = SyncJobState.COMPLETED,
                outcome = outcome,
                detail = error?.code,
            ),
        )
        events += completed
        eventPublisher.publish(completed)
        transitionTo(SyncJobState.IDLE)

        return SyncPriceTablesResult(
            outcome = outcome,
            error = error,
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
        table: PriceTable,
        payload: PriceTableImported.Payload,
        events: MutableList<DomainEvent>,
    ) {
        val event = PriceTableImported.create(payload = payload)
        events += event
        eventPublisher.publish(event)
    }

    private suspend fun publishImportFailed(
        table: PriceTable,
        error: Throwable,
        events: MutableList<DomainEvent>,
    ) {
        val event = PriceTableImportFailed.create(
            payload = PriceTableImportFailed.Payload(
                surveyWeekId = DomainId.forSurveyWeek(table.surveyWeek),
                tableType = table.tableType,
                detail = error.message ?: error.javaClass.simpleName,
            ),
        )
        events += event
        eventPublisher.publish(event)
    }

    private suspend fun isPendingImport(table: PriceTable): Boolean {
        val survey = priceTableRepository.findPriceSurveyByWeek(table.surveyWeek)
        val alreadyImported = when (table.tableType) {
            PriceTableType.WEEKLY_SUMMARY -> survey?.hasSummaryData == true
            PriceTableType.STATION_DETAIL -> survey?.hasStationData == true
        }
        if (!alreadyImported) {
            return true
        }

        val stored = priceTableRepository.findPriceTableByUrl(table.sourceUrl) ?: return true
        return stored.checksum == null
    }

    private fun requiresStationImport(pendingTables: List<PriceTable>): Boolean =
        pendingTables.any { it.tableType == PriceTableType.STATION_DETAIL }

    private suspend fun transitionTo(target: SyncJobState) {
        val next = syncJobRepository.getCurrentState().transitionTo(target)
        syncJobRepository.saveState(next)
    }
}
