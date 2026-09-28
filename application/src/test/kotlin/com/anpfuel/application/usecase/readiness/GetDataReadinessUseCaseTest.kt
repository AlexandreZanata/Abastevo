package com.anpfuel.application.usecase.readiness

import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.SyncJobRepository
import com.anpfuel.domain.state.DataReadinessState
import com.anpfuel.domain.state.SyncJobState
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import java.time.Instant
import java.time.LocalDate

class GetDataReadinessUseCaseTest {

    private val priceTableRepository = mockk<PriceTableRepository>()
    private val syncJobRepository = mockk<SyncJobRepository>()
    private val today = LocalDate.parse("2026-06-15")
    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

    private lateinit var useCase: GetDataReadinessUseCase

    @BeforeEach
    fun setUp() {
        useCase = GetDataReadinessUseCase(
            priceTableRepository = priceTableRepository,
            syncJobRepository = syncJobRepository,
            todayProvider = { today },
        )
    }

    @Test
    fun givenFreshImportedData_whenInvoked_thenReady() = runTest {
        coEvery { priceTableRepository.countImportedSurveyWeeks() } returns 1
        coEvery { syncJobRepository.getCurrentState() } returns SyncJobState.IDLE
        coEvery { priceTableRepository.getImportedPriceSurveys() } returns listOf(fullSurvey())

        val result = useCase.invoke()

        assertEquals(DataReadinessState.READY, result.readiness)
        assertEquals(surveyWeek, result.latestSurveyWeek)
        assertTrue(result.hasCachedData)
    }

    @Test
    fun givenNoImportedData_whenInvoked_thenEmpty() = runTest {
        coEvery { priceTableRepository.countImportedSurveyWeeks() } returns 0
        coEvery { syncJobRepository.getCurrentState() } returns SyncJobState.IDLE

        val result = useCase.invoke()

        assertEquals(DataReadinessState.EMPTY, result.readiness)
        assertEquals(null, result.latestSurveyWeek)
    }

    private fun fullSurvey(): PriceSurvey = PriceSurvey.restore(
        id = com.anpfuel.domain.valueobject.DomainId.forSurveyWeek(surveyWeek),
        surveyWeek = surveyWeek,
        summaryImportedAt = Instant.parse("2026-06-14T12:00:00Z"),
        stationImportedAt = Instant.parse("2026-06-14T12:05:00Z"),
    )
}
