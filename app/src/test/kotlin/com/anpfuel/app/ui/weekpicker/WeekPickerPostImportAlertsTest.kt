package com.anpfuel.app.ui.weekpicker

import com.anpfuel.application.error.AppError
import com.anpfuel.application.usecase.sync.DiscoverSurveyWeekCatalogOutcome
import com.anpfuel.application.usecase.sync.DiscoverSurveyWeekCatalogUseCase
import com.anpfuel.application.usecase.sync.DownloadSurveyWeekResult
import com.anpfuel.application.usecase.sync.DownloadSurveyWeekUseCase
import com.anpfuel.application.usecase.sync.GetDownloadedSurveyWeeksUseCase
import com.anpfuel.application.usecase.sync.SelectWeekAndSyncUseCase
import com.anpfuel.application.usecase.sync.SyncPriceTablesResult
import com.anpfuel.data.worker.SyncWorkScheduler
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import java.time.LocalDate
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/** B-BR-NOTIF-05: a week imported from the list runs the post-import alerts, like a background sync. */
@OptIn(ExperimentalCoroutinesApi::class)
class WeekPickerPostImportAlertsTest {

    private val dispatcher = StandardTestDispatcher()
    private val latest = entry("2026-11-22", "2026-11-28")
    private val older = entry("2026-11-15", "2026-11-21")
    private val discover = mockk<DiscoverSurveyWeekCatalogUseCase>()
    private val download = mockk<DownloadSurveyWeekUseCase>()
    private val downloaded = mockk<GetDownloadedSurveyWeeksUseCase>()
    private val scheduler = mockk<SyncWorkScheduler>(relaxed = true)

    private fun entry(start: String, end: String) = SurveyWeekCatalogEntry.create(
        surveyWeek = SurveyWeek.fromIsoDates(start, end),
        summaryUrl = "https://example.invalid/$end-summary.xlsx",
        stationUrl = "https://example.invalid/$end-station.xlsx",
        publishedAt = LocalDate.parse(end),
    )

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        coEvery { discover() } returns DiscoverSurveyWeekCatalogOutcome.Success(catalog = listOf(latest, older))
        coEvery { downloaded() } returns emptySet()
    }

    @AfterEach
    fun tearDown() = Dispatchers.resetMain()

    private fun viewModel() = WeekPickerViewModel(discover, mockk<SelectWeekAndSyncUseCase>(), download, downloaded, scheduler)

    @Test
    fun `a downloaded week runs the post-import alerts`() = runTest(dispatcher) {
        coEvery { download(older) } returns DownloadSurveyWeekResult.Success(SyncPriceTablesResult(SyncJobOutcome.SUCCESS))
        val vm = viewModel()
        advanceUntilIdle()

        vm.downloadWeek(older)
        advanceUntilIdle()

        verify(exactly = 1) { scheduler.enqueuePriceDropEvaluation() }
    }

    @Test
    fun `a failed download runs nothing`() = runTest(dispatcher) {
        coEvery { download(older) } returns DownloadSurveyWeekResult.Failed(AppError.SyncNetworkError)
        val vm = viewModel()
        advanceUntilIdle()

        vm.downloadWeek(older)
        advanceUntilIdle()

        verify(exactly = 0) { scheduler.enqueuePriceDropEvaluation() }
    }
}
