package com.anpfuel.application.usecase.sync

import com.anpfuel.application.error.AppError
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.repository.PriceTableSyncGateway
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertInstanceOf
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows
import java.io.IOException

class DiscoverSurveyWeekCatalogUseCaseTest {

    private val priceTableSyncGateway = mockk<PriceTableSyncGateway>()
    private lateinit var useCase: DiscoverSurveyWeekCatalogUseCase

    private val latestWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val olderWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")

    @BeforeEach
    fun setUp() {
        useCase = DiscoverSurveyWeekCatalogUseCase(priceTableSyncGateway)
    }

    @Test
    fun returnsCatalogOrderedNewestFirst() = runTest {
        val olderEntry = catalogEntry(olderWeek)
        val latestEntry = catalogEntry(latestWeek)
        coEvery { priceTableSyncGateway.discoverSurveyWeekCatalog() } returns listOf(
            olderEntry,
            latestEntry,
        )

        val outcome = useCase.invoke()

        val success = assertInstanceOf(DiscoverSurveyWeekCatalogOutcome.Success::class.java, outcome)
        assertEquals(listOf(latestEntry, olderEntry), success.catalog)
    }

    @Test
    fun mapsNetworkFailureToStructuredError() = runTest {
        coEvery { priceTableSyncGateway.discoverSurveyWeekCatalog() } throws IOException("HTTP 503")

        val outcome = useCase.invoke()

        val failure = assertInstanceOf(DiscoverSurveyWeekCatalogOutcome.Failure::class.java, outcome)
        assertEquals(AppError.SyncNetworkError, failure.error)
    }

    @Test
    fun emptyCatalogReturnsParseFailure() = runTest {
        coEvery { priceTableSyncGateway.discoverSurveyWeekCatalog() } returns emptyList()

        val outcome = useCase.invoke()

        val failure = assertInstanceOf(DiscoverSurveyWeekCatalogOutcome.Failure::class.java, outcome)
        assertEquals(AppError.SyncParseError, failure.error)
    }

    @Test
    fun rethrowsDomainException() = runTest {
        coEvery { priceTableSyncGateway.discoverSurveyWeekCatalog() } throws DomainException("BR-001")

        assertThrows<DomainException> {
            useCase.invoke()
        }
    }

    private fun catalogEntry(surveyWeek: SurveyWeek): SurveyWeekCatalogEntry =
        SurveyWeekCatalogEntry.create(
            surveyWeek = surveyWeek,
            summaryUrl = "https://example.com/resumo_${surveyWeek.startDate}.xlsx",
            stationUrl = "https://example.com/revendas_${surveyWeek.startDate}.xlsx",
        )
}
