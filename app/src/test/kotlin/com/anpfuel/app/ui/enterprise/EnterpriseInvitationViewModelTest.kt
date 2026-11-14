package com.anpfuel.app.ui.enterprise

import androidx.lifecycle.ViewModelStore
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.port.NetworkConnectivityGateway
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class EnterpriseInvitationViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val holder = ViewModelStore()
    private val gateway = mockk<EnterpriseInvitationGateway>(relaxed = true)
    private val online = MutableStateFlow(false)
    private val fence = EnterpriseSessionFence()
    private val binding get() = EnterpriseAccessBinding(
        "10000000-0000-4000-8000-000000000001", "https://example.invalid",
        "10000000-0000-4000-8000-000000000002", fence.epoch,
    )
    private val code = "AIN1.10000000-0000-4000-8000-000000000003." + "a".repeat(64)

    @BeforeEach fun setup() {
        Dispatchers.setMain(dispatcher)
        every { gateway.currentBinding() } answers { binding }
        coEvery { gateway.open() } answers { EnterpriseAccessResult.Ok(binding) }
        coEvery { gateway.preview(any(), any()) } returns EnterpriseAccessResult.Ok(
            EnterpriseInvitationPreview("Synthetic private organization", false,
                listOf(EnterpriseInvitationGrant("DRIVER", "SELF")), false, "d".repeat(64)),
        )
    }
    @AfterEach fun cleanup() { holder.clear(); Dispatchers.resetMain() }
    private fun model() = EnterpriseInvitationViewModel(
        EnterpriseInvitationFlow(gateway) { "10000000-0000-4000-8000-000000000004" },
        object : NetworkConnectivityGateway { override fun observeIsConnected() = online }, fence,
    ).also { holder.put("invitation", it) }

    @Test fun `offline actions never reach the server and connection loss removes the preview`() = runTest(dispatcher) {
        val vm = model(); advanceUntilIdle()
        vm.preview(code); vm.accept(true); vm.decline(); advanceUntilIdle()
        coVerify(exactly = 0) { gateway.open() }
        assertFalse(vm.connected.value)
        online.value = true; advanceUntilIdle()
        vm.preview(code); advanceUntilIdle()
        assertNotNull(vm.state.value.preview)
        online.value = false; advanceUntilIdle()
        assertEquals(EnterpriseInvitationStage.ENTRY, vm.state.value.stage)
        assertNull(vm.state.value.preview)
        assertNull(vm.state.value.acknowledgement)
    }

    @Test fun `background fence invalidates visible consent without another user action`() = runTest(dispatcher) {
        online.value = true
        val vm = model(); advanceUntilIdle(); vm.preview(code); advanceUntilIdle()
        assertEquals(EnterpriseInvitationStage.PREVIEW, vm.state.value.stage)
        fence.markBackground(); advanceUntilIdle()
        assertEquals(EnterpriseInvitationStage.ENTRY, vm.state.value.stage)
        assertNull(vm.state.value.preview)
        assertEquals(EnterpriseAccessError.STALE, vm.state.value.error)
        vm.accept(true); advanceUntilIdle()
        coVerify(exactly = 0) { gateway.accept(any(), any(), any(), any()) }
        assertFalse(vm.state.value.toString().contains("Synthetic"))
    }

    @Test fun `unsent input cannot adopt a new identity epoch while its action is queued`() = runTest(dispatcher) {
        online.value = true
        val vm = model(); advanceUntilIdle()
        val typedEpoch = fence.epoch
        vm.preview(code, typedEpoch)
        fence.lock()
        advanceUntilIdle()
        coVerify(exactly = 0) { gateway.open() }
        assertNull(vm.state.value.preview)
        // An already stale UI callback is refused before scheduling, too.
        vm.preview(code, typedEpoch); advanceUntilIdle()
        coVerify(exactly = 0) { gateway.open() }
    }
}
