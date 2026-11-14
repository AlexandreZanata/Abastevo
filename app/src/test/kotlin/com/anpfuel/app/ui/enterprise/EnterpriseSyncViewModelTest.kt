package com.anpfuel.app.ui.enterprise

import androidx.lifecycle.ViewModelStore
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.sync.*
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.sync.*
import io.mockk.*
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class EnterpriseSyncViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val holder = ViewModelStore()
    private val flow = mockk<EnterpriseSyncFlow>(relaxed = true)
    private val fence = EnterpriseSessionFence()
    private val online = MutableStateFlow(false)
    private val activity = MutableSharedFlow<Long>(extraBufferCapacity = 4)
    private val context = EnterpriseContextSummary(EnterprisePartition.of(
        "10000000-0000-4000-8000-000000000001", "https://example.invalid", "10000000-0000-4000-8000-000000000002"),
        "10000000-0000-4000-8000-000000000003", "Synthetic company")
    private val states = MutableStateFlow(EnterpriseSyncViewState(generation = 7, epoch = 0, context = context))
    private val identity = EnterpriseOperationIdentity("10000000-0000-4000-8000-000000000004", "sha256:" + "a".repeat(64))

    @BeforeEach fun setup() {
        Dispatchers.setMain(dispatcher)
        every { flow.state } returns states
        every { flow.activityChanges } returns activity
    }
    @AfterEach fun cleanup() { holder.clear(); Dispatchers.resetMain() }
    private fun model() = EnterpriseSyncViewModel(flow, object : NetworkConnectivityGateway {
        override fun observeIsConnected() = online
    }, fence).also { holder.put("sync", it) }

    @Test fun `offline source reads are forwarded without treating connectivity as custody`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle()
        assertTrue(model.open(context, 0, 7))
        model.refresh(0, 7); model.loadMore(0, 7); model.detail(identity, 0, 7)
        advanceUntilIdle()
        assertFalse(model.connected.value)
        coVerify(exactly = 1) { flow.open(context, 0, EnterpriseSyncMode.PENDING) }
        coVerify(exactly = 1) { flow.refresh(0, 7) }
        coVerify(exactly = 1) { flow.more(0, 7) }
        coVerify(exactly = 1) { flow.showDetail(identity.operationId, identity.payloadDigest, 0, 7) }
    }

    @Test fun `queued old gestures cannot adopt a new view generation or epoch`(): Unit = runTest(dispatcher) {
        online.value = true
        val model = model(); advanceUntilIdle(); clearMocks(flow, answers = false)
        model.refresh(0, 7); model.loadMore(0, 7); model.detail(identity, 0, 7)
        model.dismiss(identity, "sha256:" + "b".repeat(64), 0, 7); model.manualCheck(0, 7)
        states.value = states.value.copy(generation = 8)
        fence.markBackground(); advanceUntilIdle()
        assertFalse(model.open(context, 0, 7))
        model.refresh(0, 7); model.loadMore(0, 7); model.detail(identity, 0, 7); advanceUntilIdle()
        coVerify(exactly = 0) { flow.refresh(any(), any()) }
        coVerify(exactly = 0) { flow.more(any(), any()) }
        coVerify(exactly = 0) { flow.showDetail(any(), any(), any(), any()) }
        coVerify(exactly = 0) { flow.dismiss(any(), any(), any(), any(), any()) }
        coVerify(exactly = 0) { flow.manualCheck(any(), any(), any()) }
        verify(atLeast = 1) { flow.invalidateIfStale() }
    }

    @Test fun `withdrawal and owner disposal clear synchronously before other work can run`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle()
        every { flow.withdraw() } answers { states.value = EnterpriseSyncViewState(generation = 8) }
        model.withdraw()
        verify(exactly = 1) { flow.withdraw() }
        assertNull(model.state.value.context)
        holder.clear()
        verify(exactly = 1) { flow.close() }
    }

    @Test fun `same account replacement withdraws before handoff even when the fence epoch is unchanged`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle()
        every { flow.withdraw() } answers { states.value = EnterpriseSyncViewState(generation = 8) }
        model.detail(identity, 0, 7)
        model.withdraw()
        assertEquals(0, fence.epoch)
        val other = EnterpriseContextSummary(EnterprisePartition.of(context.partition.ownerAccount, context.partition.origin,
            "10000000-0000-4000-8000-000000000005"), "10000000-0000-4000-8000-000000000006", "Other synthetic company")
        assertFalse(model.open(context, 0, 7))
        assertTrue(model.open(other, 0, 8))
        advanceUntilIdle()
        coVerify(exactly = 0) { flow.showDetail(any(), any(), any(), any()) }
        coVerify(exactly = 1) { flow.open(other, 0, EnterpriseSyncMode.PENDING) }
    }

    @Test fun `manual network checking is guarded while offline but never resets a retry floor`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle()
        model.manualCheck(0, 7); advanceUntilIdle()
        coVerify(exactly = 0) { flow.manualCheck(any(), any(), any()) }
        online.value = true; advanceUntilIdle()
        model.manualCheck(0, 7); advanceUntilIdle()
        coVerify(exactly = 1) { flow.manualCheck(true, 0, 7) }
        assertTrue(model.connected.value)
    }

    @Test fun `activity observations refresh committed status through the actual bounded port`(): Unit = runTest(dispatcher) {
        model(); advanceUntilIdle(); clearMocks(flow, answers = false)
        activity.emit(1); advanceUntilIdle()
        coVerify(exactly = 1) { flow.onActivityChanged() }
    }

    @Test fun `history mode and refusal dismissal retain rendered receipt correlation`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle()
        val receipt = "sha256:" + "b".repeat(64)
        model.changeMode(EnterpriseSyncMode.HISTORY, 0, 7)
        model.dismiss(identity, receipt, 0, 7)
        model.backToList(0, 7)
        advanceUntilIdle()
        coVerify(exactly = 1) { flow.changeMode(EnterpriseSyncMode.HISTORY, 0, 7) }
        coVerify(exactly = 1) { flow.dismiss(identity.operationId, identity.payloadDigest, receipt, 0, 7) }
        verify(exactly = 1) { flow.backToList(0, 7) }
    }
}
