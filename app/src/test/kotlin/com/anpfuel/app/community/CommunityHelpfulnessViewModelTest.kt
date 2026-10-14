package com.anpfuel.app.community

import com.anpfuel.application.port.CommunityHelpfulnessFlagProvider
import com.anpfuel.application.usecase.community.HelpfulnessVoteOutcome
import com.anpfuel.application.usecase.community.SubmitHelpfulnessVoteUseCase
import com.anpfuel.domain.repository.CommunityHelpfulnessReceipt
import com.anpfuel.domain.repository.CommunityHelpfulnessRejectKind
import com.anpfuel.domain.repository.CommunityHelpfulnessTally
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/**
 * CS08B: flag OFF never touches the use case; guests see the login
 * explanation with no network IO; tally loads once per target with
 * counts, choice and review notice; a second tap mid-flight is
 * ignored so retries reuse one stable submission id; rejections stay
 * explicit and never claim a vote.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class CommunityHelpfulnessViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private val observationId = "11111111-1111-4111-8111-111111111111"

    private val useCase = mockk<SubmitHelpfulnessVoteUseCase>()
    private var enabled = false
    private val flags = object : CommunityHelpfulnessFlagProvider {
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

    private fun tally() = CommunityHelpfulnessTally(observationId, 2, 3, 1, 4, 7500, "HELPFUL")

    private fun preparedVm(accountId: String = "acct-1"): CommunityHelpfulnessViewModel {
        coEvery {
            useCase.tally(SubmitHelpfulnessVoteUseCase.TallyRequest(observationId, 2))
        } returns HelpfulnessVoteOutcome.Tallied(tally())
        val vm = CommunityHelpfulnessViewModel(useCase, flags).also { it.ioDispatcher = dispatcher }
        vm.prepare(observationId = observationId, decisionSequence = 2, accountId = accountId)
        return vm
    }

    @Test
    fun `disabled flag never calls use case`() = runTest(dispatcher) {
        val vm = CommunityHelpfulnessViewModel(useCase, flags).also { it.ioDispatcher = dispatcher }
        vm.prepare(observationId = observationId, decisionSequence = 2, accountId = "acct-1")
        advanceUntilIdle()
        assertTrue(vm.state.value is CommunityHelpfulnessUiState.Disabled)
        coVerify(exactly = 0) { useCase.tally(any()) }
        coVerify(exactly = 0) { useCase.cast(any()) }
    }

    @Test
    fun `guest sees login explanation with no network`() = runTest(dispatcher) {
        enabled = true
        val vm = CommunityHelpfulnessViewModel(useCase, flags).also { it.ioDispatcher = dispatcher }
        vm.prepare(observationId = observationId, decisionSequence = 2, accountId = "")
        advanceUntilIdle()
        assertTrue(vm.state.value is CommunityHelpfulnessUiState.Guest)
        coVerify(exactly = 0) { useCase.tally(any()) }
    }

    @Test
    fun `tally loads counts choice and review notice`() = runTest(dispatcher) {
        enabled = true
        val vm = preparedVm()
        advanceUntilIdle()
        val state = vm.state.value
        assertTrue(state is CommunityHelpfulnessUiState.Content)
        state as CommunityHelpfulnessUiState.Content
        assertEquals(3L, state.helpful)
        assertEquals(1L, state.notHelpful)
        assertEquals("HELPFUL", state.myChoice)
        assertEquals(7500L, state.helpfulBasisPoints)
    }

    @Test
    fun `vote records with stable submission id`() = runTest(dispatcher) {
        enabled = true
        coEvery {
            useCase.cast(any())
        } returns HelpfulnessVoteOutcome.Voted(
            CommunityHelpfulnessReceipt("vote-9", observationId, 4, 1, 5, 8000, "HELPFUL", false),
            false,
        )
        val vm = preparedVm()
        advanceUntilIdle()
        vm.vote("HELPFUL")
        advanceUntilIdle()
        val state = vm.state.value
        assertTrue(state is CommunityHelpfulnessUiState.Content)
        state as CommunityHelpfulnessUiState.Content
        assertEquals(4L, state.helpful)
        coVerify(exactly = 1) {
            useCase.cast(
                SubmitHelpfulnessVoteUseCase.CastRequest(observationId, state.submissionId, "HELPFUL", 2),
            )
        }
    }

    @Test
    fun `second tap mid-flight is ignored`() = runTest(dispatcher) {
        enabled = true
        coEvery {
            useCase.cast(any())
        } returns HelpfulnessVoteOutcome.Voted(
            CommunityHelpfulnessReceipt("vote-9", observationId, 4, 1, 5, 8000, "HELPFUL", false),
            false,
        )
        val vm = preparedVm()
        advanceUntilIdle()
        vm.vote("HELPFUL")
        vm.vote("NOT_HELPFUL")
        advanceUntilIdle()
        coVerify(exactly = 1) { useCase.cast(any()) }
    }

    @Test
    fun `rejection stays explicit`() = runTest(dispatcher) {
        enabled = true
        coEvery {
            useCase.cast(any())
        } returns HelpfulnessVoteOutcome.Rejected(CommunityHelpfulnessRejectKind.SELF_VOTE, "self")
        val vm = preparedVm()
        advanceUntilIdle()
        vm.vote("HELPFUL")
        advanceUntilIdle()
        val state = vm.state.value
        assertTrue(state is CommunityHelpfulnessUiState.Rejected)
        assertEquals(CommunityHelpfulnessRejectKind.SELF_VOTE, (state as CommunityHelpfulnessUiState.Rejected).kind)
    }
    @Test fun `late old account tally cannot overwrite current account state`()=runTest(dispatcher) {
        enabled=true
        val old=kotlinx.coroutines.CompletableDeferred<HelpfulnessVoteOutcome>()
        var calls=0
        coEvery {useCase.tally(any())} coAnswers {
            calls++
            if(calls==1) old.await() else HelpfulnessVoteOutcome.Tallied(
                CommunityHelpfulnessTally(observationId,2,9,0,9,10000,null))
        }
        val vm=CommunityHelpfulnessViewModel(useCase,flags)
        vm.ioDispatcher=dispatcher
        vm.prepare(observationId,2,"account-old")
        runCurrent()
        vm.prepare(observationId,2,"account-new")
        runCurrent()
        old.complete(HelpfulnessVoteOutcome.Tallied(CommunityHelpfulnessTally(observationId,2,1,0,1,10000,"HELPFUL")))
        advanceUntilIdle()
        val state=vm.state.value as CommunityHelpfulnessUiState.Content
        assertEquals(9L,state.helpful)
        assertEquals(null,state.myChoice)
    }

    @Test fun `changing a vote uses a new command while ambiguous retry keeps the old id`()=runTest(dispatcher) {
        enabled=true
        val requests=mutableListOf<SubmitHelpfulnessVoteUseCase.CastRequest>()
        coEvery {useCase.cast(any())} coAnswers {
            val request=firstArg<SubmitHelpfulnessVoteUseCase.CastRequest>();requests+=request
            if(requests.size==1) HelpfulnessVoteOutcome.Rejected(CommunityHelpfulnessRejectKind.TRANSPORT,"unavailable")
            else HelpfulnessVoteOutcome.Voted(CommunityHelpfulnessReceipt("v",observationId,1,0,1,10000,request.choiceWire,false),false)
        }
        val vm=preparedVm();advanceUntilIdle()
        vm.vote("HELPFUL");advanceUntilIdle()
        vm.vote("HELPFUL");advanceUntilIdle()
        vm.vote("NOT_HELPFUL");advanceUntilIdle()
        assertEquals(requests[0].clientSubmissionId,requests[1].clientSubmissionId)
        assertTrue(requests[1].clientSubmissionId!=requests[2].clientSubmissionId)
    }

    @Test fun `owner change before vote performs no write`()=runTest(dispatcher) {
        enabled=true
        val proof=mockk<com.anpfuel.data.remote.PhotoProofTransport>()
        var owner="account:old:key"
        io.mockk.every {proof.origin} returns "https://example.invalid"
        io.mockk.every {proof.ownerScope()} answers {owner}
        coEvery {useCase.tally(any())} returns HelpfulnessVoteOutcome.Tallied(tally())
        val vm=CommunityHelpfulnessViewModel(useCase,flags,proof);vm.ioDispatcher=dispatcher
        vm.prepare(observationId,2,owner);advanceUntilIdle()
        owner="account:new:key"
        vm.vote("HELPFUL");advanceUntilIdle()
        coVerify(exactly=0){useCase.cast(any())}
        assertTrue(vm.state.value is CommunityHelpfulnessUiState.Guest)
    }

}
