package com.anpfuel.app.community

import com.anpfuel.application.port.FeedbackFlagProvider
import com.anpfuel.application.usecase.feedback.FeedbackWriteOutcome
import com.anpfuel.application.usecase.feedback.FeedbackPageOutcome
import com.anpfuel.application.usecase.feedback.GetFeedbackPageUseCase
import com.anpfuel.application.usecase.feedback.SubmitFeedbackUseCase
import com.anpfuel.domain.repository.CommentWriteReceipt
import com.anpfuel.domain.repository.FeedbackException
import com.anpfuel.domain.repository.FeedbackPage
import com.anpfuel.domain.repository.FeedbackRejectKind
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
 * P17-T02 RED: Android social flows through the shared use cases.
 *
 * Flag OFF performs no IO; blank sessions surface sign-in without
 * IO; over-280 text is rejected locally without IO; stale revisions
 * and denied self-votes surface typed rejections for rollback;
 * outages queue with the same op id on retry (no fake success, no
 * amplification).
 */
@OptIn(ExperimentalCoroutinesApi::class)
class FeedbackViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val writes = mockk<SubmitFeedbackUseCase>()
    private val reads = mockk<GetFeedbackPageUseCase>()
    private var enabled = false
    private val flags = object : FeedbackFlagProvider {
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

    private fun preparedVm(accountId: String = "acc-1"): FeedbackViewModel {
        val vm = FeedbackViewModel(writes, reads, flags)
        vm.prepare(stationId = "s-1", product = "GASOLINE", accountId = accountId)
        return vm
    }

    @Test
    fun `disabled flag performs no io`() = runTest {
        enabled = false
        val vm = FeedbackViewModel(writes, reads, flags)

        vm.prepare(stationId = "s-1", product = "GASOLINE", accountId = "acc-1")
        vm.submitComment("hello")
        vm.loadFirstPage()
        advanceUntilIdle()

        assertTrue(vm.state.value is FeedbackUiState.Disabled)
        coVerify(exactly = 0) { writes.comment(any(), any(), any(), any(), any()) }
        coVerify(exactly = 0) { reads.comments(any(), any(), any(), any()) }
    }

    @Test
    fun `blank session requires sign in without io`() = runTest {
        enabled = true
        val vm = preparedVm(accountId = "  ")

        vm.submitComment("hello")
        vm.submitVote("c-1", "VALID")
        vm.report("c-1", "spam")
        advanceUntilIdle()

        assertTrue(vm.state.value is FeedbackUiState.SignInRequired)
        coVerify(exactly = 0) { writes.comment(any(), any(), any(), any(), any()) }
        coVerify(exactly = 0) { writes.vote(any(), any(), any(), any()) }
        coVerify(exactly = 0) { writes.report(any(), any(), any(), any()) }
    }

    @Test
    fun `over 280 chars rejected locally without io`() = runTest {
        enabled = true
        coEvery { writes.comment(any(), any(), any(), any(), any()) } returns
            FeedbackWriteOutcome.Commented(CommentWriteReceipt("c-1", 1))
        val vm = preparedVm()

        vm.submitComment("x".repeat(281))
        advanceUntilIdle()

        val state = vm.state.value
        assertTrue(state is FeedbackUiState.Rejected)
        assertEquals("INVALID", (state as FeedbackUiState.Rejected).kindLabel)
        coVerify(exactly = 0) { writes.comment(any(), any(), any(), any(), any()) }
    }

    @Test
    fun `stale revision surfaces typed rejection`() = runTest {
        enabled = true
        coEvery { writes.edit(any(), any(), any(), any(), any()) } returns
            FeedbackWriteOutcome.Rejected(FeedbackRejectKind.STALE_REVISION, "stale")
        val vm = preparedVm()

        vm.editComment("c-1", "new text", 1)
        advanceUntilIdle()

        val state = vm.state.value
        assertTrue(state is FeedbackUiState.Rejected)
        assertEquals("STALE", (state as FeedbackUiState.Rejected).kindLabel)
    }

    @Test
    fun `denied self vote surfaces rejection`() = runTest {
        enabled = true
        coEvery { writes.vote(any(), any(), any(), any()) } returns
            FeedbackWriteOutcome.Rejected(FeedbackRejectKind.SELF_VOTE, "own comment")
        val vm = preparedVm()

        vm.submitVote("c-1", "VALID")
        advanceUntilIdle()

        val state = vm.state.value
        assertTrue(state is FeedbackUiState.Rejected)
        assertEquals("SELF_VOTE", (state as FeedbackUiState.Rejected).kindLabel)
    }

    @Test
    fun `outage queues and retry reuses the op id`() = runTest {
        enabled = true
        val seen = mutableListOf<String>()
        coEvery { writes.comment(any(), any(), any(), any(), capture(seen)) } answers {
            FeedbackWriteOutcome.Queued(
                com.anpfuel.domain.repository.PendingFeedbackOp(seen.last(), "comment", "s-1|GASOLINE"),
            )
        }
        val vm = preparedVm()

        vm.submitComment("hello")
        advanceUntilIdle()
        assertTrue(vm.state.value is FeedbackUiState.Queued)

        vm.retry()
        advanceUntilIdle()
        assertTrue(vm.state.value is FeedbackUiState.Queued)
        assertEquals(2, seen.size)
        assertEquals(seen[0], seen[1])
    }

    @Test
    fun `reads land fresh items`() = runTest {
        enabled = true
        coEvery { reads.comments(any(), any(), any(), any()) } returns
            FeedbackPageOutcome.Fresh(FeedbackPage(emptyList(), null))
        val vm = preparedVm()

        vm.loadFirstPage()
        advanceUntilIdle()

        val state = vm.state.value
        assertTrue(state is FeedbackUiState.Loaded)
        assertEquals(0, (state as FeedbackUiState.Loaded).items.size)
    }

    @Test
    fun `gateway transport failure maps from exception`() = runTest {
        enabled = true
        coEvery { writes.comment(any(), any(), any(), any(), any()) } throws
            FeedbackException(FeedbackRejectKind.TRANSPORT, "offline")
        val vm = preparedVm()

        vm.submitComment("hello")
        advanceUntilIdle()

        // The use case converts transport into Queued; a raw throw here
        // means the view model must not crash and reports transport.
        val state = vm.state.value
        assertTrue(state is FeedbackUiState.Rejected)
        assertEquals("TRANSPORT", (state as FeedbackUiState.Rejected).kindLabel)
    }
}
