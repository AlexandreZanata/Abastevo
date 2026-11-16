package com.anpfuel.app.ui.safety

import com.anpfuel.app.community.CommunitySafetyError
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.safety.CommunityBlockState
import com.anpfuel.application.usecase.safety.FilterHiddenContentUseCase
import com.anpfuel.application.usecase.safety.ListBlockedUsersUseCase
import com.anpfuel.application.usecase.safety.UnblockCommunityUserUseCase
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.safety.CommunitySafetyException
import com.anpfuel.domain.safety.SafetyContentKind
import com.anpfuel.domain.safety.SafetyContentRef
import java.time.ZoneOffset
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

/** BUC-SAFE-04: Account > Blocked users. */
@OptIn(ExperimentalCoroutinesApi::class)
class BlockedUsersViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private val gateway = FakeSafetyGateway()
    private val session = PortableAuth.Session(
        familyId = "fam-1",
        accountId = "viewer",
        accessToken = "tok",
        refreshToken = "ref",
        accessExpiresAt = Long.MAX_VALUE,
        absoluteExpiresAt = Long.MAX_VALUE,
    )
    private var current: PortableAuth.Session? = session
    private val sessions = object : AuthSessionStore {
        override fun save(session: PortableAuth.Session) = Unit
        override fun load(): PortableAuth.Session? = current
        override fun clear() = Unit
    }
    private val blockState = CommunityBlockState(gateway) { current?.accountId }

    private fun viewModel() = BlockedUsersViewModel(
        ListBlockedUsersUseCase(gateway, blockState),
        UnblockCommunityUserUseCase(gateway, blockState),
        sessions,
    )

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        gateway.blockedAuthors += listOf("older", "newer")
        gateway.labels["newer"] = "Ana"
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun `lists the account blocks newest first`() = runTest(dispatcher) {
        val vm = viewModel()
        advanceUntilIdle()
        val state = vm.state.value
        assertFalse(state.loading)
        assertEquals(listOf(gateway.blockIdOf("newer"), gateway.blockIdOf("older")), state.items.map { it.blockId })
        assertEquals("Ana", state.items.first().label)
        assertEquals("", state.items.last().label)
        assertFalse(state.isEmpty)
    }

    @Test
    fun `no blocks shows the empty state`() = runTest(dispatcher) {
        gateway.blockedAuthors.clear()
        val vm = viewModel()
        advanceUntilIdle()
        assertTrue(vm.state.value.isEmpty)
    }

    @Test
    fun `visitors are asked to sign in and nothing loads`() = runTest(dispatcher) {
        current = null
        val vm = viewModel()
        advanceUntilIdle()
        assertFalse(vm.state.value.signedIn)
        assertFalse(vm.state.value.isEmpty)
        assertEquals(0, gateway.listCalls)
    }

    @Test
    fun `a failed load offers retry`() = runTest(dispatcher) {
        gateway.listFailure = CommunitySafetyException.Offline()
        val vm = viewModel()
        advanceUntilIdle()
        assertTrue(vm.state.value.loadFailed)
        assertFalse(vm.state.value.isEmpty)

        gateway.listFailure = null
        vm.load()
        advanceUntilIdle()
        assertFalse(vm.state.value.loadFailed)
        assertEquals(2, vm.state.value.items.size)
    }

    @Test
    fun `unblocking asks first and then removes the row`() = runTest(dispatcher) {
        val vm = viewModel()
        advanceUntilIdle()
        val target = vm.state.value.items.first()
        vm.askUnblock(target)
        assertEquals(target, vm.state.value.pendingUnblock)
        vm.cancelUnblock()
        assertNull(vm.state.value.pendingUnblock)
        assertEquals(2, gateway.blockedAuthors.size)

        vm.askUnblock(target)
        vm.confirmUnblock()
        advanceUntilIdle()
        val state = vm.state.value
        assertNull(state.pendingUnblock)
        assertTrue(state.unblockedNotice)
        assertEquals(1, state.items.size)
        assertEquals(setOf("older"), gateway.blockedAuthors)
    }

    @Test
    fun `unblocking the last block stops hidden filtering`() = runTest(dispatcher) {
        gateway.blockedAuthors.remove("older")
        gateway.authors["00000000-0000-4000-8000-000000000001"] = "newer"
        val filter = FilterHiddenContentUseCase(gateway, blockState)
        val vm = viewModel()
        advanceUntilIdle()
        vm.askUnblock(vm.state.value.items.single())
        vm.confirmUnblock()
        advanceUntilIdle()
        val asked = SafetyContentRef(SafetyContentKind.FORUM_COMMENT, "00000000-0000-4000-8000-000000000001")
        assertEquals(emptySet<SafetyContentRef>(), filter(listOf(asked)))
        assertTrue(gateway.hiddenQuestions.isEmpty(), "no blocks left, nothing to ask")
        assertEquals(1, gateway.listCalls)
    }

    @Test
    fun `a block already gone is dropped and the list reloads`() = runTest(dispatcher) {
        val vm = viewModel()
        advanceUntilIdle()
        val target = vm.state.value.items.first()
        gateway.blockedAuthors.remove("newer")
        vm.askUnblock(target)
        vm.confirmUnblock()
        advanceUntilIdle()
        assertEquals(listOf(gateway.blockIdOf("older")), vm.state.value.items.map { it.blockId })
        assertEquals(2, gateway.listCalls)
        assertNull(vm.state.value.error)
    }

    @Test
    fun `unblock refusals keep the row and explain`() = runTest(dispatcher) {
        val vm = viewModel()
        advanceUntilIdle()
        gateway.removeFailure = CommunitySafetyException.RateLimited(15)
        vm.askUnblock(vm.state.value.items.first())
        vm.confirmUnblock()
        advanceUntilIdle()
        assertEquals(CommunitySafetyError.RateLimited(15), vm.state.value.error)
        assertEquals(2, vm.state.value.items.size)

        gateway.removeFailure = CommunitySafetyException.AuthRequired()
        vm.askUnblock(vm.state.value.items.first())
        vm.confirmUnblock()
        advanceUntilIdle()
        assertTrue(vm.state.value.requireSignIn)
    }

    @Test
    fun `block dates are shown as local dates and never crash`() {
        assertTrue(formatBlockDate("2026-11-15T10:00:00Z", ZoneOffset.UTC).contains("2026"))
        assertTrue(formatBlockDate("2026-11-15T10:00:00-03:00", ZoneOffset.UTC).contains("2026"))
        assertEquals("not-a-date", formatBlockDate("not-a-date-at-all", ZoneOffset.UTC))
    }
}
