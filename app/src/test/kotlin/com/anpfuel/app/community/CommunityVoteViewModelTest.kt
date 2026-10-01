package com.anpfuel.app.community

import com.anpfuel.application.port.CommunityVoteFlagProvider
import com.anpfuel.application.usecase.community.CommunityVoteOutcome
import com.anpfuel.application.usecase.community.SubmitCommunityVoteUseCase
import com.anpfuel.domain.repository.CommunityDisputeReasons
import com.anpfuel.domain.repository.CommunityVoteReceipt
import com.anpfuel.domain.repository.CommunityVoteRejectKind
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

/**
 * P10-T07: flag OFF never touches the use case; the shown snapshot
 * gates both actions; a second tap mid-flight is ignored so retries
 * reuse one stable submission id; rejection stays explicit.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class CommunityVoteViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private val observationId = "11111111-1111-4111-8111-111111111111"

    private val useCase = mockk<SubmitCommunityVoteUseCase>()
    private var enabled = false
    private val flags = object : CommunityVoteFlagProvider {
        override fun isEnabled(): Boolean = enabled
    }

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    private fun preparedVm(): CommunityVoteViewModel {
        val vm = CommunityVoteViewModel(useCase, flags)
        vm.prepare(
            observationId = observationId,
            productWire = "GASOLINE_REGULAR",
            amountMilliBrl = 5890L,
            unit = "BRL/L",
            conditionKind = "STANDARD",
        )
        return vm
    }

    @Test
    fun `disabled flag performs no io`() = runTest {
        enabled = false
        val vm = CommunityVoteViewModel(useCase, flags)

        vm.prepare(
            observationId = observationId,
            productWire = "GASOLINE_REGULAR",
            amountMilliBrl = 5890L,
            unit = "BRL/L",
            conditionKind = "STANDARD",
        )
        vm.confirm()
        advanceUntilIdle()

        assertTrue(vm.state.value is CommunityVoteUiState.Disabled)
        coVerify(exactly = 0) { useCase.confirm(any()) }
        coVerify(exactly = 0) { useCase.dispute(any()) }
    }

    @Test
    fun `prepare exposes shown snapshot before actions`() = runTest {
        enabled = true
        val vm = preparedVm()

        val state = vm.state.value
        assertTrue(state is CommunityVoteUiState.Ready)
        val ready = state as CommunityVoteUiState.Ready
        assertTrue(ready.summary.contains("GASOLINE_REGULAR"))
        assertTrue(ready.summary.contains("BRL/L"))
        assertTrue(ready.summary.contains("STANDARD"))
    }

    @Test
    fun `confirm success resolves to confirmed`() = runTest {
        enabled = true
        coEvery { useCase.confirm(any()) } returns
            CommunityVoteOutcome.Confirmed(
                CommunityVoteReceipt("vote-1", observationId, false),
                false,
            )
        val vm = preparedVm()

        vm.confirm()
        advanceUntilIdle()

        val state = vm.state.value
        assertTrue(state is CommunityVoteUiState.Confirmed)
        assertEquals("vote-1", (state as CommunityVoteUiState.Confirmed).voteId)
    }

    @Test
    fun `double tap issues a single flight with one stable id`() = runTest {
        enabled = true
        coEvery { useCase.confirm(any()) } returns
            CommunityVoteOutcome.Confirmed(
                CommunityVoteReceipt("vote-1", observationId, true),
                true,
            )
        val vm = preparedVm()

        vm.confirm()
        vm.confirm()
        advanceUntilIdle()

        coVerify(exactly = 1) { useCase.confirm(any()) }
        assertTrue(vm.state.value is CommunityVoteUiState.Confirmed)
    }

    @Test
    fun `self-confirm denial resolves to explicit rejected`() = runTest {
        enabled = true
        coEvery { useCase.confirm(any()) } returns
            CommunityVoteOutcome.Rejected(
                CommunityVoteRejectKind.SELF_CONFIRMATION,
                "contributors cannot confirm their own observations",
            )
        val vm = preparedVm()

        vm.confirm()
        advanceUntilIdle()

        val state = vm.state.value
        assertTrue(state is CommunityVoteUiState.Rejected)
        assertEquals("SELF", (state as CommunityVoteUiState.Rejected).kindLabel)
    }

    @Test
    fun `dispute success resolves to disputed`() = runTest {
        enabled = true
        coEvery { useCase.dispute(any()) } returns
            CommunityVoteOutcome.Disputed(
                CommunityVoteReceipt("dispute-3", observationId, false),
                "OPEN",
            )
        val vm = preparedVm()

        vm.dispute(CommunityDisputeReasons.WRONG_PRODUCT, null, null)
        advanceUntilIdle()

        val state = vm.state.value
        assertTrue(state is CommunityVoteUiState.Disputed)
        assertEquals("OPEN", (state as CommunityVoteUiState.Disputed).state)
    }
}
