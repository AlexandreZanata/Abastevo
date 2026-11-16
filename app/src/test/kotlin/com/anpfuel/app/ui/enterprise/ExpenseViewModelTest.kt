package com.anpfuel.app.ui.enterprise

import androidx.lifecycle.ViewModelStore
import com.anpfuel.app.ui.enterprise.expense.ExpenseViewModel
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.expense.ExpenseFlow
import com.anpfuel.application.enterprise.expense.ExpenseForm
import com.anpfuel.application.enterprise.expense.ExpensePayer
import com.anpfuel.application.enterprise.expense.ExpenseViewState
import com.anpfuel.application.enterprise.sync.EnterpriseDeliveryActivity
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
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/** EMP-EXP-004: the SCR-EXP-01 owner forwards only current gestures, saves offline and sends only with a connection. */
@OptIn(ExperimentalCoroutinesApi::class)
class ExpenseViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val holder = ViewModelStore()
    private val flow = mockk<ExpenseFlow>(relaxed = true)
    private val fence = EnterpriseSessionFence()
    private val online = MutableStateFlow(false)
    private val context = EnterpriseContextSummary(EnterprisePartition.of(
        "10000000-0000-4000-8000-000000000001", "https://example.invalid", "10000000-0000-4000-8000-000000000002"),
        "10000000-0000-4000-8000-000000000003", "Synthetic company", "10000000-0000-4000-8000-000000000005")
    private val states = MutableStateFlow(ExpenseViewState(generation = 7, epoch = 0, context = context, loaded = true))
    private val choice = EnterpriseAssetChoice(context.partition.organizationId, "10000000-0000-4000-8000-000000000030", 1,
        AssetKind.VEHICLE, UsageBasis.DISTANCE, emptyList(), Instant.parse("2026-11-15T12:00:00Z"), false)
    private val form = ExpenseForm("TOLL", "12,50", "12,50", ExpensePayer.EMPLOYEE)

    @BeforeEach fun setup() { Dispatchers.setMain(dispatcher); every { flow.state } returns states }
    @AfterEach fun cleanup() { holder.clear(); Dispatchers.resetMain() }
    private fun model() = ExpenseViewModel(flow, object : NetworkConnectivityGateway { override fun observeIsConnected() = online },
        fence, EnterpriseDeliveryActivity()).also { holder.put("expenses", it) }

    @Test fun `saving works offline while sending needs a connection and old gestures do nothing`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle()
        model.record(choice, form, null, 0, 6); model.record(choice, form, null, 1, 7); advanceUntilIdle()
        coVerify(exactly = 0) { flow.record(any(), any(), any(), any(), any()) }
        model.record(choice, form, null, 0, 7); advanceUntilIdle()
        coVerify(exactly = 1) { flow.record(choice, form, null, 0, 7) }
        model.sendNow(0, 7); advanceUntilIdle()
        coVerify(exactly = 0) { flow.sendNow(any(), any()) }
        online.value = true; advanceUntilIdle()
        model.sendNow(0, 7); advanceUntilIdle()
        coVerify(exactly = 1) { flow.sendNow(0, 7) }
        model.withdraw()
        verify(exactly = 1) { flow.withdraw() }
    }
}
