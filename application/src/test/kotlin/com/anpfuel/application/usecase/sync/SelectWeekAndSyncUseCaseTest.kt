package com.anpfuel.application.usecase.sync

import com.anpfuel.application.error.AppError
import com.anpfuel.domain.event.PreferencesUpdated
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.event.SyncRequestSource
import com.anpfuel.domain.event.SurveyWeekSelected
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.valueobject.SurveyWeekSelectionMode
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertInstanceOf
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class SelectWeekAndSyncUseCaseTest {

    private val selectSurveyWeekUseCase = mockk<SelectSurveyWeekUseCase>()
    private val syncPriceTablesUseCase = mockk<SyncPriceTablesUseCase>()

    private lateinit var useCase: SelectWeekAndSyncUseCase

    private val latestWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val latestEntry = SurveyWeekCatalogEntry.create(
        surveyWeek = latestWeek,
        summaryUrl = "https://example.com/summary.xlsx",
        stationUrl = "https://example.com/station.xlsx",
    )

    @BeforeEach
    fun setUp() {
        useCase = SelectWeekAndSyncUseCase(
            selectSurveyWeekUseCase = selectSurveyWeekUseCase,
            syncPriceTablesUseCase = syncPriceTablesUseCase,
        )
    }

    @Test
    fun useLatestWeekPersistsAndSyncsUsingCatalogUrls() = runTest {
        stubSelectWeek(latestWeek, SurveyWeekSelectionMode.LATEST)
        val syncResult = SyncPriceTablesResult(outcome = SyncJobOutcome.SUCCESS)
        coEvery {
            syncPriceTablesUseCase(
                source = SyncRequestSource.MANUAL,
                targetSurveyWeek = latestWeek,
                preDiscoveredWeekTables = any(),
            )
        } returns syncResult

        val result = useCase.invoke(
            catalogEntry = latestEntry,
            selectionMode = SurveyWeekSelectionMode.LATEST,
        )

        val success = assertInstanceOf(SelectWeekAndSyncResult.Success::class.java, result)
        assertEquals(syncResult, success.syncResult)
        coVerify(exactly = 1) {
            selectSurveyWeekUseCase(latestWeek, SurveyWeekSelectionMode.LATEST)
        }
    }

    @Test
    fun returnsFailedWhenSyncFails() = runTest {
        stubSelectWeek(latestWeek, SurveyWeekSelectionMode.LATEST)
        coEvery {
            syncPriceTablesUseCase(
                source = SyncRequestSource.MANUAL,
                targetSurveyWeek = latestWeek,
                preDiscoveredWeekTables = any(),
            )
        } returns SyncPriceTablesResult(
            outcome = SyncJobOutcome.FAILED,
            error = AppError.SyncNetworkError,
        )

        val result = useCase.invoke(
            catalogEntry = latestEntry,
            selectionMode = SurveyWeekSelectionMode.LATEST,
        )

        val failed = assertInstanceOf(SelectWeekAndSyncResult.Failed::class.java, result)
        assertEquals(AppError.SyncNetworkError, failed.error)
    }

    private fun stubSelectWeek(
        surveyWeek: SurveyWeek,
        selectionMode: SurveyWeekSelectionMode,
    ) {
        coEvery {
            selectSurveyWeekUseCase(surveyWeek, selectionMode)
        } returns SelectSurveyWeekResult(
            preferences = UserPreferences(activeSurveyWeek = surveyWeek),
            surveyWeekSelected = SurveyWeekSelected.create(
                payload = SurveyWeekSelected.Payload(
                    surveyWeek = surveyWeek,
                    selectionMode = selectionMode,
                ),
            ),
            preferencesUpdated = PreferencesUpdated.create(
                payload = PreferencesUpdated.Payload(
                    changedKeys = setOf(SelectSurveyWeekUseCase.KEY_ACTIVE_SURVEY_WEEK),
                ),
            ),
        )
    }
}
