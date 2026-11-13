package com.anpfuel.app.ui.enterprise

import androidx.lifecycle.ViewModelStore
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.domain.enterprise.EnterprisePartition
import io.mockk.*
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class EnterpriseContextViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val holder = ViewModelStore()
    private val flow = mockk<EnterpriseContextFlow>(relaxed = true)
    private val online = MutableStateFlow(false)
    private val fence = EnterpriseSessionFence()

    @BeforeEach fun setup() {
        Dispatchers.setMain(dispatcher)
        every { flow.state } returns MutableStateFlow(EnterpriseContextState())
    }
    @AfterEach fun cleanup() { holder.clear(); Dispatchers.resetMain() }
    private fun model() = EnterpriseContextViewModel(flow, object : NetworkConnectivityGateway {
        override fun observeIsConnected() = online
    }, fence).also { holder.put("context", it) }

    @Test fun `offline source actions are refused and connectivity loss withdraws authority`() = runTest(dispatcher) {
        val vm = model(); advanceUntilIdle()
        vm.refresh(); vm.loadMore(); advanceUntilIdle()
        coVerify(exactly = 0) { flow.refresh() }
        coVerify(exactly = 0) { flow.loadMore() }
        verify(atLeast = 1) { flow.unconfirm() }
        online.value = true; advanceUntilIdle()
        coVerify(exactly = 1) { flow.refresh() }
        online.value = false; advanceUntilIdle()
        assertFalse(vm.connected.value)
        verify(atLeast = 2) { flow.unconfirm() }
    }

    @Test fun `a queued or already stale screen action cannot adopt a new session`() = runTest(dispatcher) {
        online.value = true
        val vm = model(); advanceUntilIdle()
        clearMocks(flow, answers = false)
        val renderedEpoch = fence.epoch
        vm.refresh(renderedEpoch); vm.loadMore(renderedEpoch)
        val context = EnterpriseContextSummary(EnterprisePartition.of(
            "10000000-0000-4000-8000-000000000001", "https://example.invalid",
            "10000000-0000-4000-8000-000000000002",
        ), "10000000-0000-4000-8000-000000000003", "Synthetic company")
        vm.select(context, renderedEpoch)
        fence.markBackground(); advanceUntilIdle()
        vm.refresh(renderedEpoch); vm.loadMore(renderedEpoch); advanceUntilIdle()
        vm.select(context, renderedEpoch); advanceUntilIdle()
        coVerify(exactly = 0) { flow.refresh() }
        coVerify(exactly = 0) { flow.loadMore() }
        coVerify(exactly = 0) { flow.select(any()) }
        verify(atLeast = 1) { flow.invalidateIfStale() }
    }

    @Test fun `leaving the owning view model closes its private context synchronously`() = runTest(dispatcher) {
        model(); advanceUntilIdle()
        holder.clear()
        verify(exactly = 1) { flow.close() }
    }

    @Test fun `explicit empty recovery is forwarded only from a current rendered attempt`() = runTest(dispatcher) {
        online.value = true
        val vm = model(); advanceUntilIdle()
        clearMocks(flow, answers = false)
        val context = EnterpriseContextSummary(EnterprisePartition.of(
            "10000000-0000-4000-8000-000000000001", "https://example.invalid",
            "10000000-0000-4000-8000-000000000002",
        ), "10000000-0000-4000-8000-000000000003", "Synthetic company")
        val rendered = fence.epoch
        vm.select(context, rendered, acknowledgeErasedWork = true)
        fence.markBackground(); advanceUntilIdle()
        vm.select(context, rendered, acknowledgeErasedWork = true); advanceUntilIdle()
        coVerify(exactly = 0) { flow.select(any(), true) }
        vm.select(context, fence.epoch, acknowledgeErasedWork = true); advanceUntilIdle()
        coVerify(exactly = 1) { flow.select(context, true) }
    }
}
