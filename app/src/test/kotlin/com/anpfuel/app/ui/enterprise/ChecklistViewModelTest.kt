package com.anpfuel.app.ui.enterprise

import androidx.lifecycle.ViewModelStore
import com.anpfuel.app.ui.enterprise.checklist.ChecklistViewModel
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.checklist.ChecklistFlow
import com.anpfuel.application.enterprise.checklist.ChecklistViewState
import com.anpfuel.application.enterprise.sync.EnterpriseDeliveryActivity
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistAnswer
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistContext
import io.mockk.*
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/** EMP-CHECK-002: the execution owner forwards only current gestures, saves offline and reads the company only with a connection. */
@OptIn(ExperimentalCoroutinesApi::class)
class ChecklistViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val holder = ViewModelStore()
    private val flow = mockk<ChecklistFlow>(relaxed = true)
    private val fence = EnterpriseSessionFence()
    private val online = MutableStateFlow(false)
    private val activity = EnterpriseDeliveryActivity()
    private val context = EnterpriseContextSummary(EnterprisePartition.of(
        "10000000-0000-4000-8000-000000000001", "https://example.invalid", "10000000-0000-4000-8000-000000000002"),
        "10000000-0000-4000-8000-000000000003", "Synthetic company")
    private val asset = "10000000-0000-4000-8000-000000000030"
    private val run = "10000000-0000-4000-8000-000000000040"
    private val states = MutableStateFlow(ChecklistViewState(generation = 7, epoch = 0, context = context, assetId = asset,
        checklistContext = ChecklistContext.DEPARTURE, loaded = true))

    @BeforeEach fun setup() {
        Dispatchers.setMain(dispatcher)
        every { flow.state } returns states
    }
    @AfterEach fun cleanup() { holder.clear(); Dispatchers.resetMain() }
    private fun model() = ChecklistViewModel(flow, object : NetworkConnectivityGateway {
        override fun observeIsConnected() = online
    }, fence, activity).also { holder.put("checklists", it) }

    @Test fun `offline the book is read and answers are saved while company reads wait for a connection`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle()
        model.open(context, asset, ChecklistContext.DEPARTURE, 0); advanceUntilIdle()
        coVerify(exactly = 1) { flow.open(context, asset, ChecklistContext.DEPARTURE, 0) }
        coVerify(exactly = 0) { flow.refresh(any(), any()) }
        model.start(0, 7); model.answer(run, 2, ChecklistAnswer.NOT_OK, "Freios rangendo", 0, 7); advanceUntilIdle()
        coVerify(exactly = 1) { flow.start(0, 7) }
        coVerify(exactly = 1) { flow.answer(run, 2, ChecklistAnswer.NOT_OK, "Freios rangendo", 0, 7) }
        model.complete(run, false, 0, 7); advanceUntilIdle()
        coVerify(exactly = 1) { flow.complete(run, false, 0, 7) }
        model.refresh(0, 7); model.sendNow(0, 7); advanceUntilIdle()
        coVerify(exactly = 0) { flow.refresh(any(), any()) }
        coVerify(exactly = 0) { flow.sendNow(any(), any()) }
        online.value = true; advanceUntilIdle()
        model.sendNow(0, 7); model.refresh(0, 7); advanceUntilIdle()
        coVerify(exactly = 1) { flow.sendNow(0, 7) }
        coVerify(exactly = 1) { flow.refresh(0, 7) }
    }

    @Test fun `opening while connected also refreshes the cached template and the company's results`(): Unit = runTest(dispatcher) {
        online.value = true
        val model = model(); advanceUntilIdle()
        model.open(context, asset, ChecklistContext.DEPARTURE, 0); advanceUntilIdle()
        coVerifyOrder {
            flow.open(context, asset, ChecklistContext.DEPARTURE, 0)
            flow.refresh(0, 7)
        }
    }

    @Test fun `old gestures cannot adopt a new generation or epoch and a busy owner takes nothing`(): Unit = runTest(dispatcher) {
        online.value = true
        val model = model(); advanceUntilIdle(); clearMocks(flow, answers = false)
        model.start(0, 6); model.answer(run, 1, ChecklistAnswer.OK, null, 1, 7); model.complete(run, true, 0, 6)
        model.sendNow(0, 6); model.open(context, asset, ChecklistContext.DEPARTURE, 1)
        advanceUntilIdle()
        coVerify(exactly = 0) { flow.start(any(), any()) }
        coVerify(exactly = 0) { flow.answer(any(), any(), any(), any(), any(), any()) }
        coVerify(exactly = 0) { flow.complete(any(), any(), any(), any()) }
        coVerify(exactly = 0) { flow.sendNow(any(), any()) }
        coVerify(exactly = 0) { flow.open(any(), any(), any(), any()) }
        assertFalse(model.isCurrent(0, 6)); assertTrue(model.isCurrent(0, 7))
        states.value = states.value.copy(busy = true)
        model.answer(run, 1, ChecklistAnswer.OK, null, 0, 7); advanceUntilIdle()
        coVerify(exactly = 0) { flow.answer(any(), any(), any(), any(), any(), any()) }
    }

    @Test fun `a finished delivery re-reads the book and, when connected, the company's result`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle(); clearMocks(flow, answers = false)
        val partition = context.partition
        fun finishDelivery() {
            val attempt = com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease.capture(
                EnterpriseLocalIdentity(partition.ownerAccount, partition.origin, "10000000-0000-4000-8000-000000000009"), 0, 0,
                "10000000-0000-4000-8000-000000000010") { true }.forEnrollment(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(partition,
                context.membershipId), "10000000-0000-4000-8000-000000000011", 1))
            activity.end(activity.begin(attempt, listOf(com.anpfuel.domain.enterprise.sync.EnterpriseOperationIdentity(
                "10000000-0000-4000-8000-000000000012", "sha256:" + "a".repeat(64)))))
        }
        finishDelivery(); advanceUntilIdle()
        coVerify(atLeast = 1) { flow.reload(0, 7) }
        coVerify(exactly = 0) { flow.refresh(any(), any()) }
        online.value = true; advanceUntilIdle(); clearMocks(flow, answers = false)
        finishDelivery(); advanceUntilIdle()
        coVerify(atLeast = 1) { flow.refresh(0, 7) }
        assertNotNull(model)
    }

    @Test fun `withdrawal and clearing close the owner`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle(); clearMocks(flow, answers = false)
        model.withdraw(); model.invalidateIfStale()
        verify(exactly = 1) { flow.withdraw() }
        verify(exactly = 1) { flow.invalidateIfStale() }
        holder.clear()
        verify(exactly = 1) { flow.close() }
        model.withdraw(); model.start(0, 7); advanceUntilIdle()
        verify(exactly = 1) { flow.withdraw() }
        coVerify(exactly = 0) { flow.start(any(), any()) }
    }
}
