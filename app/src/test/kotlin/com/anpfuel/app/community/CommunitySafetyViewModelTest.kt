package com.anpfuel.app.community

import com.anpfuel.app.ui.safety.FakeSafetyGateway
import com.anpfuel.application.usecase.safety.BlockCommunityUserUseCase
import com.anpfuel.application.usecase.safety.CommunityBlockState
import com.anpfuel.application.usecase.safety.FilterHiddenContentUseCase
import com.anpfuel.application.usecase.safety.ReportCommunityUserUseCase
import com.anpfuel.domain.safety.CommunitySafetyException
import com.anpfuel.domain.safety.SafetyContentKind
import com.anpfuel.domain.safety.SafetyContentRef
import com.anpfuel.domain.safety.UserReportReason
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/** Station comments: report user, block user and hidden filtering (BUC-SAFE-02/03, B-BR-SAFE-06). */
@OptIn(ExperimentalCoroutinesApi::class)
class CommunitySafetyViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private val gateway = FakeSafetyGateway()
    private val blockState = CommunityBlockState(gateway) { "viewer" }

    private fun ref(n: Int) = SafetyContentRef(SafetyContentKind.STATION_COMMENT, "00000000-0000-4000-8000-%012d".format(n))

    private val troll1 = ref(1)
    private val troll2 = ref(2)
    private val friend = ref(3)

    private fun viewModel() = CommunitySafetyViewModel(
        FilterHiddenContentUseCase(gateway, blockState),
        ReportCommunityUserUseCase(gateway),
        BlockCommunityUserUseCase(gateway, blockState),
    )

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        gateway.authors[troll1.id] = "troll"
        gateway.authors[troll2.id] = "troll"
        gateway.authors[friend.id] = "friend"
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun `comments show only after the check and hidden ones never show`() = runTest(dispatcher) {
        gateway.blockedAuthors += "troll"
        val vm = viewModel()
        vm.show(listOf(troll1, friend))
        assertFalse(vm.state.value.isVisible(friend), "nothing shows before the answer")
        advanceUntilIdle()
        assertTrue(vm.state.value.isVisible(friend))
        assertFalse(vm.state.value.isVisible(troll1))
    }

    @Test
    fun `already checked comments are not asked again`() = runTest(dispatcher) {
        gateway.blockedAuthors += "troll"
        val vm = viewModel()
        vm.show(listOf(troll1, friend))
        advanceUntilIdle()
        vm.show(listOf(troll1, friend, troll2))
        advanceUntilIdle()
        assertEquals(listOf(listOf(troll1, friend), listOf(troll2)), gateway.hiddenQuestions)
        assertFalse(vm.state.value.isVisible(troll2))
    }

    @Test
    fun `blocking from a station comment hides every comment of that author`() = runTest(dispatcher) {
        val vm = viewModel()
        vm.show(listOf(troll1, troll2, friend))
        advanceUntilIdle()
        assertTrue(vm.state.value.isVisible(troll2))

        vm.openBlockUser(troll1)
        assertEquals(troll1, vm.state.value.blockTarget)
        vm.confirmBlockUser()
        advanceUntilIdle()

        val state = vm.state.value
        assertNull(state.blockTarget)
        assertEquals(CommunitySafetyNotice.BLOCKED, state.notice)
        assertFalse(state.isVisible(troll1))
        assertFalse(state.isVisible(troll2))
        assertTrue(state.isVisible(friend))
    }

    @Test
    fun `reporting a user confirms and a repeat says it was already reported`() = runTest(dispatcher) {
        val vm = viewModel()
        vm.openReportUser(troll1)
        vm.submitReportUser(UserReportReason.SPAM, null)
        advanceUntilIdle()
        assertEquals(CommunitySafetyNotice.REPORTED, vm.state.value.notice)
        assertNull(vm.state.value.userReportTarget)

        gateway.reportReplayed = true
        vm.openReportUser(troll1)
        assertNull(vm.state.value.notice)
        vm.submitReportUser(UserReportReason.SPAM, null)
        advanceUntilIdle()
        assertEquals(CommunitySafetyNotice.REPORT_REPLAYED, vm.state.value.notice)
    }

    @Test
    fun `own comments are refused by the server with a friendly message`() = runTest(dispatcher) {
        gateway.reportFailure = CommunitySafetyException.SelfTarget()
        gateway.blockFailure = CommunitySafetyException.SelfTarget()
        val vm = viewModel()
        vm.openReportUser(friend)
        vm.submitReportUser(UserReportReason.OTHER, null)
        advanceUntilIdle()
        assertEquals(CommunitySafetyError.SelfTarget, vm.state.value.error)

        vm.openBlockUser(friend)
        vm.confirmBlockUser()
        advanceUntilIdle()
        assertEquals(CommunitySafetyError.SelfTarget, vm.state.value.error)
        assertFalse(vm.state.value.blocking)
    }

    @Test
    fun `rate limits and expired sessions are surfaced`() = runTest(dispatcher) {
        gateway.reportFailure = CommunitySafetyException.RateLimited(60)
        val vm = viewModel()
        vm.openReportUser(troll1)
        vm.submitReportUser(UserReportReason.SPAM, null)
        advanceUntilIdle()
        assertEquals(CommunitySafetyError.RateLimited(60), vm.state.value.error)

        gateway.blockFailure = CommunitySafetyException.AuthRequired()
        vm.openBlockUser(troll1)
        vm.confirmBlockUser()
        advanceUntilIdle()
        assertTrue(vm.state.value.requireSignIn)
        vm.consumeSignIn()
        assertFalse(vm.state.value.requireSignIn)
    }

    @Test
    fun `a failing check shows the comments`() = runTest(dispatcher) {
        gateway.blockedAuthors += "troll"
        gateway.hiddenFailure = CommunitySafetyException.Unavailable()
        val vm = viewModel()
        vm.show(listOf(troll1, friend))
        advanceUntilIdle()
        assertTrue(vm.state.value.isVisible(troll1))
        assertTrue(vm.state.value.isVisible(friend))
    }
}
