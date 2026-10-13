package com.anpfuel.application.usecase.sync

import com.anpfuel.application.error.AppError
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.event.SyncRequestSource
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertInstanceOf
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class DownloadSurveyWeekUseCaseTest {

    private val syncPriceTablesUseCase = mockk<SyncPriceTablesUseCase>()

    private lateinit var useCase: DownloadSurveyWeekUseCase

    private val week = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")
    private val entry = SurveyWeekCatalogEntry.create(
        surveyWeek = week,
        summaryUrl = "https://example.com/summary.xlsx",
        stationUrl = "https://example.com/station.xlsx",
    )

    @BeforeEach
    fun setUp() {
        useCase = DownloadSurveyWeekUseCase(
            syncPriceTablesUseCase = syncPriceTablesUseCase,
        )
    }

    @Test
    fun syncsTargetWeekWithoutPersistingSelection() = runTest {
        val syncResult = SyncPriceTablesResult(outcome = SyncJobOutcome.SUCCESS)
        coEvery {
            syncPriceTablesUseCase(
                source = SyncRequestSource.MANUAL,
                targetSurveyWeek = week,
                preDiscoveredWeekTables = any(),
            )
        } returns syncResult

        val result = useCase.invoke(catalogEntry = entry)

        val success = assertInstanceOf(DownloadSurveyWeekResult.Success::class.java, result)
        assertEquals(syncResult, success.syncResult)
    }

    @Test
    fun returnsFailedWhenSyncFails() = runTest {
        coEvery {
            syncPriceTablesUseCase(
                source = SyncRequestSource.MANUAL,
                targetSurveyWeek = week,
                preDiscoveredWeekTables = any(),
            )
        } returns SyncPriceTablesResult(
            outcome = SyncJobOutcome.FAILED,
            error = AppError.SyncNetworkError,
        )

        val result = useCase.invoke(catalogEntry = entry)

        val failed = assertInstanceOf(DownloadSurveyWeekResult.Failed::class.java, result)
        assertEquals(AppError.SyncNetworkError, failed.error)
    }
}
