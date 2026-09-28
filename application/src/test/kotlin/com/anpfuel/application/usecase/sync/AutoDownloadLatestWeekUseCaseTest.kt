package com.anpfuel.application.usecase.sync

import com.anpfuel.application.error.AppError
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.event.SyncRequestSource
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.DomainId
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
import java.time.Instant

class AutoDownloadLatestWeekUseCaseTest {

    private val userPreferencesRepository = mockk<UserPreferencesRepository>()
    private val priceTableRepository = mockk<PriceTableRepository>()
    private val discoverSurveyWeekCatalogUseCase = mockk<DiscoverSurveyWeekCatalogUseCase>()
    private val selectWeekAndSyncUseCase = mockk<SelectWeekAndSyncUseCase>()

    private lateinit var useCase: AutoDownloadLatestWeekUseCase

    private val latestWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val latestEntry = SurveyWeekCatalogEntry.create(
        surveyWeek = latestWeek,
        summaryUrl = "https://example.com/summary.xlsx",
        stationUrl = "https://example.com/station.xlsx",
    )

    @BeforeEach
    fun setUp() {
        useCase = AutoDownloadLatestWeekUseCase(
            userPreferencesRepository = userPreferencesRepository,
            priceTableRepository = priceTableRepository,
            discoverSurveyWeekCatalogUseCase = discoverSurveyWeekCatalogUseCase,
            selectWeekAndSyncUseCase = selectWeekAndSyncUseCase,
        )
    }

    @Test
    fun returnsDisabledWhenPreferenceOff() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            autoDownloadLatestWeek = false,
        )

        val outcome = useCase()

        assertEquals(AutoDownloadLatestWeekOutcome.Disabled, outcome)
        coVerify(exactly = 0) { discoverSurveyWeekCatalogUseCase() }
    }

    @Test
    fun returnsUpToDateWhenLatestWeekAlreadyImported() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            autoDownloadLatestWeek = true,
            activeSurveyWeek = latestWeek,
        )
        coEvery { discoverSurveyWeekCatalogUseCase() } returns DiscoverSurveyWeekCatalogOutcome.Success(
            catalog = listOf(latestEntry),
        )
        coEvery { priceTableRepository.findPriceSurveyByWeek(latestWeek) } returns importedSurvey(latestWeek)

        val outcome = useCase()

        assertEquals(AutoDownloadLatestWeekOutcome.UpToDate, outcome)
        coVerify(exactly = 0) { selectWeekAndSyncUseCase(any(), any(), any()) }
    }

    @Test
    fun syncsLatestWeekWhenPreferenceEnabledAndDataMissing() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            autoDownloadLatestWeek = true,
            activeSurveyWeek = null,
        )
        coEvery { discoverSurveyWeekCatalogUseCase() } returns DiscoverSurveyWeekCatalogOutcome.Success(
            catalog = listOf(latestEntry),
        )
        coEvery { priceTableRepository.findPriceSurveyByWeek(latestWeek) } returns null
        coEvery {
            selectWeekAndSyncUseCase(
                catalogEntry = latestEntry,
                selectionMode = SurveyWeekSelectionMode.LATEST,
                source = SyncRequestSource.SCHEDULED,
            )
        } returns SelectWeekAndSyncResult.Success(
            syncResult = SyncPriceTablesResult(outcome = SyncJobOutcome.SUCCESS),
        )

        val outcome = useCase()

        assertInstanceOf(AutoDownloadLatestWeekOutcome.Success::class.java, outcome)
    }

    @Test
    fun returnsFailedWhenCatalogDiscoveryFails() = runTest {
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            autoDownloadLatestWeek = true,
        )
        coEvery { discoverSurveyWeekCatalogUseCase() } returns DiscoverSurveyWeekCatalogOutcome.Failure(
            error = AppError.SyncNetworkError,
        )

        val outcome = useCase() as AutoDownloadLatestWeekOutcome.Failed

        assertEquals(AppError.SyncNetworkError, outcome.error)
    }

    private fun importedSurvey(week: SurveyWeek): PriceSurvey {
        val survey = PriceSurvey.restore(
            id = DomainId.forSurveyWeek(week),
            surveyWeek = week,
            summaryImportedAt = Instant.parse("2026-06-14T10:00:00Z"),
            stationImportedAt = null,
        )
        return survey
    }
}
