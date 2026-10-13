package com.anpfuel.application.usecase.sync

import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.mockk
import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class GetDownloadedSurveyWeeksUseCaseTest {

    private val priceTableRepository = mockk<PriceTableRepository>()
    private lateinit var useCase: GetDownloadedSurveyWeeksUseCase

    private val downloadedWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val pendingWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")

    @BeforeEach
    fun setUp() {
        useCase = GetDownloadedSurveyWeeksUseCase(priceTableRepository)
    }

    @Test
    fun returnsOnlyWeeksWithSummaryData() = runTest {
        coEvery { priceTableRepository.getImportedPriceSurveys() } returns listOf(
            PriceSurvey.restore(
                id = com.anpfuel.domain.valueobject.DomainId.forSurveyWeek(downloadedWeek),
                surveyWeek = downloadedWeek,
                summaryImportedAt = Instant.parse("2026-06-14T10:00:00Z"),
                stationImportedAt = null,
            ),
            PriceSurvey.create(surveyWeek = pendingWeek),
        )

        assertEquals(setOf(downloadedWeek), useCase.invoke())
    }

    @Test
    fun returnsEmptyWhenNothingImported() = runTest {
        coEvery { priceTableRepository.getImportedPriceSurveys() } returns emptyList()

        assertEquals(emptySet<SurveyWeek>(), useCase.invoke())
    }
}
