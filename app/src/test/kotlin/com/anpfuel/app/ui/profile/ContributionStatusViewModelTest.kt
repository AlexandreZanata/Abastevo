package com.anpfuel.app.ui.profile

import com.anpfuel.application.usecase.contribution.CancelOwnedContributionUseCase
import com.anpfuel.application.usecase.contribution.GetOwnedContributionsUseCase
import com.anpfuel.application.usecase.contribution.OwnedContributionStatus
import com.anpfuel.domain.contribution.ContributionState
import com.anpfuel.domain.repository.ContributionOutboxRepository
import com.anpfuel.domain.repository.OwnedContribution
import com.anpfuel.domain.repository.OwnedContributionPhase
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ContributionStatusViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val outbox = mockk<ContributionOutboxRepository>()

    private lateinit var viewModel: ContributionStatusViewModel

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        viewModel = ContributionStatusViewModel(
            getOwnedContributionsUseCase = GetOwnedContributionsUseCase(outbox),
            cancelOwnedContributionUseCase = CancelOwnedContributionUseCase(outbox),
        )
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun emptyOutboxRendersEmpty() = runTest(dispatcher) {
        coEvery { outbox.listOwned() } returns emptyList()

        viewModel.load()
        advanceUntilIdle()

        assertTrue(viewModel.uiState.value is ContributionStatusUiState.Empty)
    }

    @Test
    fun ownedCommandsRenderContent() = runTest(dispatcher) {
        coEvery { outbox.listOwned() } returns listOf(
            OwnedContribution("cmd-1", 2, 1, OwnedContributionPhase.FAILED),
        )

        viewModel.load()
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertTrue(state is ContributionStatusUiState.Content)
        val items = (state as ContributionStatusUiState.Content).items
        assertEquals(1, items.size)
        assertEquals(
            ContributionState.Queued(retryable = true),
            items.first().state,
        )
    }

    @Test
    fun loadFailureRendersError() = runTest(dispatcher) {
        coEvery { outbox.listOwned() } throws RuntimeException("boom")

        viewModel.load()
        advanceUntilIdle()

        assertTrue(viewModel.uiState.value is ContributionStatusUiState.Error)
    }

    @Test
    fun cancelDelegatesAndReloads() = runTest(dispatcher) {
        coEvery { outbox.listOwned() } returns listOf(
            OwnedContribution("cmd-1", 1, 0, OwnedContributionPhase.QUEUED),
        )
        coEvery { outbox.cancel("cmd-1") } returns Unit

        viewModel.load()
        advanceUntilIdle()
        viewModel.onCancel("cmd-1")
        advanceUntilIdle()

        coVerify(exactly = 1) { outbox.cancel("cmd-1") }
        assertTrue(viewModel.uiState.value is ContributionStatusUiState.Content)
    }

}
