package com.anpfuel.application.usecase.onboarding

import com.anpfuel.application.usecase.sync.SyncPriceTablesResult
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import java.time.Instant

class CompleteOnboardingUseCaseTest {

    private val userPreferencesRepository = mockk<UserPreferencesRepository>()
    private val priceTableRepository = mockk<PriceTableRepository>()

    private lateinit var useCase: CompleteOnboardingUseCase

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

    @BeforeEach
    fun setUp() {
        useCase = CompleteOnboardingUseCase(
            userPreferencesRepository = userPreferencesRepository,
            priceTableRepository = priceTableRepository,
        )
    }

    @Test
    fun marksOnboardingCompleteAfterSuccessfulSummaryImport() = runTest {
        stubPreferences(UserPreferences(onboardingCompleted = false))
        stubImportedSummary()

        val result = useCase.completeAfterSync(successfulSyncResult())

        assertEquals(CompleteOnboardingResult.Completed, result)
        coVerify(exactly = 1) {
            userPreferencesRepository.savePreferences(
                match { it.onboardingCompleted },
            )
        }
    }

    @Test
    fun a1SkipSyncDoesNotMarkOnboardingComplete() = runTest {
        stubPreferences(UserPreferences(onboardingCompleted = false))

        val result = useCase.skipSync()

        assertEquals(CompleteOnboardingResult.Skipped, result)
        coVerify(exactly = 0) { userPreferencesRepository.savePreferences(any()) }
    }

    @Test
    fun a2FailedSyncDoesNotMarkOnboardingComplete() = runTest {
        stubPreferences(UserPreferences(onboardingCompleted = false))
        coEvery { priceTableRepository.getImportedPriceSurveys() } returns emptyList()

        val result = useCase.completeAfterSync(
            SyncPriceTablesResult(outcome = SyncJobOutcome.FAILED),
        )

        assertEquals(CompleteOnboardingResult.NotReady, result)
        coVerify(exactly = 0) { userPreferencesRepository.savePreferences(any()) }
    }

    @Test
    fun partialSyncWithSummaryImportedMarksOnboardingComplete() = runTest {
        stubPreferences(UserPreferences(onboardingCompleted = false))
        stubImportedSummary()

        val result = useCase.completeAfterSync(
            SyncPriceTablesResult(outcome = SyncJobOutcome.PARTIAL),
        )

        assertEquals(CompleteOnboardingResult.Completed, result)
    }

    @Test
    fun returnsAlreadyCompletedWithoutSavingAgain() = runTest {
        stubPreferences(UserPreferences(onboardingCompleted = true))

        val result = useCase.completeAfterSync(successfulSyncResult())

        assertEquals(CompleteOnboardingResult.AlreadyCompleted, result)
        coVerify(exactly = 0) { userPreferencesRepository.savePreferences(any()) }
    }

    @Test
    fun successWithoutImportedSummaryRemainsNotReady() = runTest {
        stubPreferences(UserPreferences(onboardingCompleted = false))
        coEvery { priceTableRepository.getImportedPriceSurveys() } returns emptyList()

        val result = useCase.completeAfterSync(successfulSyncResult())

        assertEquals(CompleteOnboardingResult.NotReady, result)
        coVerify(exactly = 0) { userPreferencesRepository.savePreferences(any()) }
    }

    @Test
    fun skipSyncPreservesExistingOnboardingCompletedFlag() = runTest {
        val saved = slot<UserPreferences>()
        coEvery { userPreferencesRepository.getPreferences() } returns
            UserPreferences(onboardingCompleted = false)
        coEvery { userPreferencesRepository.savePreferences(capture(saved)) } returns Unit

        useCase.skipSync()

        assertFalse(saved.isCaptured)
    }

    private fun stubPreferences(preferences: UserPreferences) {
        coEvery { userPreferencesRepository.getPreferences() } returns preferences
        coEvery { userPreferencesRepository.savePreferences(any()) } returns Unit
    }

    private fun stubImportedSummary() {
        coEvery { priceTableRepository.getImportedPriceSurveys() } returns listOf(
            PriceSurvey.restore(
                id = DomainId.forSurveyWeek(surveyWeek),
                surveyWeek = surveyWeek,
                summaryImportedAt = Instant.parse("2026-06-14T10:00:00Z"),
                stationImportedAt = null,
            ),
        )
    }

    private fun successfulSyncResult() = SyncPriceTablesResult(
        outcome = SyncJobOutcome.SUCCESS,
    )
}
