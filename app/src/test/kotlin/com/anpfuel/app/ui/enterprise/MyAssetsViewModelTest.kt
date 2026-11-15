package com.anpfuel.app.ui.enterprise

import androidx.lifecycle.ViewModelStore
import com.anpfuel.app.ui.enterprise.fleet.MyAssetsViewModel
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.fleet.MyAssetsFlow
import com.anpfuel.application.enterprise.fleet.MyAssetsViewState
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.fleet.AssetKind
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import com.anpfuel.domain.enterprise.fleet.UsageBasis
import io.mockk.*
import java.time.Instant
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class MyAssetsViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val holder = ViewModelStore()
    private val flow = mockk<MyAssetsFlow>(relaxed = true)
    private val fence = EnterpriseSessionFence()
    private val online = MutableStateFlow(false)
    private val context = EnterpriseContextSummary(EnterprisePartition.of(
        "10000000-0000-4000-8000-000000000001", "https://example.invalid", "10000000-0000-4000-8000-000000000002"),
        "10000000-0000-4000-8000-000000000003", "Synthetic company")
    private val states = MutableStateFlow(MyAssetsViewState(generation = 7, epoch = 0, context = context, loaded = true))
    private val asset = "10000000-0000-4000-8000-000000000030"
    private val choice = EnterpriseAssetChoice(context.partition.organizationId, asset, 1, AssetKind.VEHICLE, UsageBasis.DISTANCE,
        emptyList(), Instant.parse("2026-11-15T12:00:00Z"), false)

    @BeforeEach fun setup() {
        Dispatchers.setMain(dispatcher)
        every { flow.state } returns states
        every { flow.choose(asset, any(), 0, 7) } returns choice
    }
    @AfterEach fun cleanup() { holder.clear(); Dispatchers.resetMain() }
    private fun model() = MyAssetsViewModel(flow, object : NetworkConnectivityGateway {
        override fun observeIsConnected() = online
    }, fence).also { holder.put("assets", it) }

    @Test fun `offline open reads the protected list without asking for a pull`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle()
        model.open(context, 0); advanceUntilIdle()
        coVerify(exactly = 1) { flow.open(context, 0, false) }
        model.refresh(0, 7); advanceUntilIdle()
        coVerify(exactly = 0) { flow.refresh(any(), any()) }
        online.value = true; advanceUntilIdle()
        model.open(context, 0); model.refresh(0, 7); advanceUntilIdle()
        coVerify(exactly = 1) { flow.open(context, 0, true) }
        coVerify(exactly = 1) { flow.refresh(0, 7) }
    }

    @Test fun `the picker returns the choice synchronously without any network`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle()
        assertFalse(model.connected.value)
        assertSame(choice, model.choose(asset, false, 0, 7))
        verify(exactly = 1) { flow.choose(asset, false, 0, 7) }
        assertNull(model.choose(asset, false, 1, 7), "A gesture rendered under another epoch never chooses")
        assertNull(model.choose(asset, false, 0, 6), "A gesture rendered under another generation never chooses")
    }

    @Test fun `queued old gestures cannot adopt a new generation or epoch`(): Unit = runTest(dispatcher) {
        online.value = true
        val model = model(); advanceUntilIdle(); clearMocks(flow, answers = false)
        model.refresh(0, 6); model.detail(asset, 1, 7); model.backToList(0, 6); model.represent(1, 7)
        advanceUntilIdle()
        coVerify(exactly = 0) { flow.refresh(any(), any()) }
        verify(exactly = 0) { flow.detail(any(), any(), any()) }
        verify(exactly = 0) { flow.backToList(any(), any()) }
        verify(exactly = 0) { flow.represent(any(), any()) }
        assertTrue(model.isCurrent(0, 7)); assertFalse(model.isCurrent(0, 6))
        states.value = states.value.copy(busy = true)
        model.refresh(0, 7); advanceUntilIdle()
        coVerify(exactly = 0) { flow.refresh(any(), any()) }
    }

    @Test fun `withdrawal and clearing close the owner`(): Unit = runTest(dispatcher) {
        // The initial epoch collection already asks the owner to invalidate itself once.
        val model = model(); advanceUntilIdle(); clearMocks(flow, answers = false)
        model.withdraw(); model.invalidateIfStale()
        verify(exactly = 1) { flow.withdraw() }
        verify(exactly = 1) { flow.invalidateIfStale() }
        holder.clear()
        verify(exactly = 1) { flow.close() }
        model.withdraw()
        verify(exactly = 1) { flow.withdraw() }
    }
}
