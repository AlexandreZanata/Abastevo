package com.anpfuel.application.usecase.onboarding

import com.anpfuel.application.error.AppError
import com.anpfuel.application.usecase.sync.SelectSurveyWeekResult
import com.anpfuel.application.usecase.sync.SelectSurveyWeekUseCase
import com.anpfuel.application.usecase.sync.SyncPriceTablesResult
import com.anpfuel.application.usecase.sync.SyncPriceTablesUseCase
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

class OnboardingSelectWeekAndSyncUseCaseTest {

    private val selectSurveyWeekUseCase = mockk<SelectSurveyWeekUseCase>()
    private val syncPriceTablesUseCase = mockk<SyncPriceTablesUseCase>()
    private val completeOnboardingUseCase = mockk<CompleteOnboardingUseCase>()

    private lateinit var useCase: OnboardingSelectWeekAndSyncUseCase

    private val latestWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val historicalWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")
    private val latestEntry = SurveyWeekCatalogEntry.create(
        surveyWeek = latestWeek,
        summaryUrl = "https://example.com/summary.xlsx",
        stationUrl = "https://example.com/station.xlsx",
    )
    private val historicalEntry = SurveyWeekCatalogEntry.create(
        surveyWeek = historicalWeek,
        summaryUrl = "https://example.com/summary-old.xlsx",
        stationUrl = "https://example.com/station-old.xlsx",
    )

    @BeforeEach
    fun setUp() {
        useCase = OnboardingSelectWeekAndSyncUseCase(
            selectSurveyWeekUseCase = selectSurveyWeekUseCase,
            syncPriceTablesUseCase = syncPriceTablesUseCase,
            completeOnboardingUseCase = completeOnboardingUseCase,
        )
    }

    @Test
    fun selectsWeekSyncsTargetWeekAndCompletesOnboarding() = runTest {
        val syncResult = SyncPriceTablesResult(outcome = SyncJobOutcome.SUCCESS)
        stubSelectWeek(latestWeek, SurveyWeekSelectionMode.LATEST)
        coEvery {
            syncPriceTablesUseCase(
                source = SyncRequestSource.FIRST_LAUNCH,
                targetSurveyWeek = latestWeek,
                preDiscoveredWeekTables = any(),
            )
        } returns syncResult
        coEvery { completeOnboardingUseCase.completeAfterSync(syncResult) } returns
            CompleteOnboardingResult.Completed

        val result = useCase.invoke(
            catalogEntry = latestEntry,
            selectionMode = SurveyWeekSelectionMode.LATEST,
        )

        val completed = assertInstanceOf(
            OnboardingSelectWeekAndSyncResult.Completed::class.java,
            result,
        )
        assertEquals(CompleteOnboardingResult.Completed, completed.onboardingResult)
        coVerify(exactly = 1) {
            selectSurveyWeekUseCase(latestWeek, SurveyWeekSelectionMode.LATEST)
        }
        coVerify(exactly = 1) {
            syncPriceTablesUseCase(
                source = SyncRequestSource.FIRST_LAUNCH,
                targetSurveyWeek = latestWeek,
                preDiscoveredWeekTables = any(),
            )
        }
    }

    @Test
    fun syncsSpecificHistoricalWeek() = runTest {
        val syncResult = SyncPriceTablesResult(outcome = SyncJobOutcome.SUCCESS)
        stubSelectWeek(historicalWeek, SurveyWeekSelectionMode.SPECIFIC)
        coEvery {
            syncPriceTablesUseCase(
                source = SyncRequestSource.FIRST_LAUNCH,
                targetSurveyWeek = historicalWeek,
                preDiscoveredWeekTables = any(),
            )
        } returns syncResult
        coEvery { completeOnboardingUseCase.completeAfterSync(syncResult) } returns
            CompleteOnboardingResult.Completed

        useCase.invoke(
            catalogEntry = historicalEntry,
            selectionMode = SurveyWeekSelectionMode.SPECIFIC,
        )

        coVerify(exactly = 1) {
            selectSurveyWeekUseCase(historicalWeek, SurveyWeekSelectionMode.SPECIFIC)
        }
    }

    @Test
    fun retriesFirstLaunchSyncOnceAfterFailure() = runTest {
        val syncResult = SyncPriceTablesResult(outcome = SyncJobOutcome.SUCCESS)
        stubSelectWeek(latestWeek, SurveyWeekSelectionMode.LATEST)
        coEvery {
            syncPriceTablesUseCase(
                source = SyncRequestSource.FIRST_LAUNCH,
                targetSurveyWeek = latestWeek,
                preDiscoveredWeekTables = any(),
            )
        } returnsMany listOf(
            SyncPriceTablesResult(
                outcome = SyncJobOutcome.FAILED,
                error = AppError.SyncNetworkError,
            ),
            syncResult,
        )
        coEvery { completeOnboardingUseCase.completeAfterSync(syncResult) } returns
            CompleteOnboardingResult.Completed

        val result = useCase.invoke(
            catalogEntry = latestEntry,
            selectionMode = SurveyWeekSelectionMode.LATEST,
        )

        assertInstanceOf(OnboardingSelectWeekAndSyncResult.Completed::class.java, result)
        coVerify(exactly = 2) {
            syncPriceTablesUseCase(
                source = SyncRequestSource.FIRST_LAUNCH,
                targetSurveyWeek = latestWeek,
                preDiscoveredWeekTables = any(),
            )
        }
    }

    @Test
    fun returnsSyncFailedWithoutCompletingOnboardingWhenBothAttemptsFail() = runTest {
        stubSelectWeek(latestWeek, SurveyWeekSelectionMode.LATEST)
        coEvery {
            syncPriceTablesUseCase(
                source = SyncRequestSource.FIRST_LAUNCH,
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

        val failed = assertInstanceOf(
            OnboardingSelectWeekAndSyncResult.SyncFailed::class.java,
            result,
        )
        assertEquals(AppError.SyncNetworkError, failed.error)
        coVerify(exactly = 0) { completeOnboardingUseCase.completeAfterSync(any()) }
        coVerify(exactly = 2) {
            syncPriceTablesUseCase(
                source = SyncRequestSource.FIRST_LAUNCH,
                targetSurveyWeek = latestWeek,
                preDiscoveredWeekTables = any(),
            )
        }
    }

    @Test
    fun propagatesNotReadyOnboardingWhenSummaryNotImported() = runTest {
        val syncResult = SyncPriceTablesResult(outcome = SyncJobOutcome.PARTIAL)
        stubSelectWeek(latestWeek, SurveyWeekSelectionMode.LATEST)
        coEvery {
            syncPriceTablesUseCase(
                source = SyncRequestSource.FIRST_LAUNCH,
                targetSurveyWeek = latestWeek,
                preDiscoveredWeekTables = any(),
            )
        } returns syncResult
        coEvery { completeOnboardingUseCase.completeAfterSync(syncResult) } returns
            CompleteOnboardingResult.NotReady

        val result = useCase.invoke(
            catalogEntry = latestEntry,
            selectionMode = SurveyWeekSelectionMode.LATEST,
        )

        val completed = assertInstanceOf(
            OnboardingSelectWeekAndSyncResult.Completed::class.java,
            result,
        )
        assertEquals(CompleteOnboardingResult.NotReady, completed.onboardingResult)
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
