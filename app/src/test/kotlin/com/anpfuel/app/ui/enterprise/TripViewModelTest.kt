package com.anpfuel.app.ui.enterprise

import androidx.lifecycle.ViewModelStore
import com.anpfuel.app.ui.enterprise.trip.TripViewModel
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.sync.EnterpriseDeliveryActivity
import com.anpfuel.application.enterprise.trip.TripCapture
import com.anpfuel.application.enterprise.trip.TripFlow
import com.anpfuel.application.enterprise.trip.TripViewState
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.fleet.AssetKind
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import com.anpfuel.domain.enterprise.fleet.UsageBasis
import com.anpfuel.domain.enterprise.trip.TripAssetLabel
import com.anpfuel.domain.enterprise.trip.TripLogbookType
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

/** EMP-TRIP-005: the SCR-TRIP-01 owner forwards only current gestures and sends only with a connection. */
@OptIn(ExperimentalCoroutinesApi::class)
class TripViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val holder = ViewModelStore()
    private val flow = mockk<TripFlow>(relaxed = true)
    private val fence = EnterpriseSessionFence()
    private val online = MutableStateFlow(false)
    private val activity = EnterpriseDeliveryActivity()
    private val context = EnterpriseContextSummary(EnterprisePartition.of(
        "10000000-0000-4000-8000-000000000001", "https://example.invalid", "10000000-0000-4000-8000-000000000002"),
        "10000000-0000-4000-8000-000000000003", "Synthetic company", "10000000-0000-4000-8000-000000000005")
    private val states = MutableStateFlow(TripViewState(generation = 7, epoch = 0, context = context, loaded = true))
    private val trip = "10000000-0000-4000-8000-000000000040"
    private val choice = EnterpriseAssetChoice(context.partition.organizationId, "10000000-0000-4000-8000-000000000030", 1,
        AssetKind.VEHICLE, UsageBasis.DISTANCE, emptyList(), Instant.parse("2026-11-15T12:00:00Z"), false)
    private val label = TripAssetLabel("Caminhão sintético", "F-1", "")

    @BeforeEach fun setup() {
        Dispatchers.setMain(dispatcher)
        every { flow.state } returns states
    }
    @AfterEach fun cleanup() { holder.clear(); Dispatchers.resetMain() }
    private fun model() = TripViewModel(flow, object : NetworkConnectivityGateway {
        override fun observeIsConnected() = online
    }, fence, activity).also { holder.put("trips", it) }

    @Test fun `offline actions are forwarded and saved locally while sending needs a connection`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle()
        model.open(context, 0); advanceUntilIdle()
        coVerify(exactly = 1) { flow.open(context, 0) }
        val departure = TripCapture.Absent("Painel apagado")
        model.start(choice, label, departure, 0, 7); advanceUntilIdle()
        coVerify(exactly = 1) { flow.start(choice, label, departure, 0, 7) }
        model.log(trip, TripLogbookType.STOP, null, 0, 7); model.markReady(trip, 0, 7); advanceUntilIdle()
        coVerify(exactly = 1) { flow.log(trip, TripLogbookType.STOP, null, 0, 7) }
        coVerify(exactly = 1) { flow.markReady(trip, 0, 7) }
        model.sendNow(0, 7); advanceUntilIdle()
        coVerify(exactly = 0) { flow.sendNow(any(), any()) }
        online.value = true; advanceUntilIdle()
        model.sendNow(0, 7); advanceUntilIdle()
        coVerify(exactly = 1) { flow.sendNow(0, 7) }
    }

    @Test fun `old gestures cannot adopt a new generation or epoch and a busy owner takes nothing`(): Unit = runTest(dispatcher) {
        online.value = true
        val model = model(); advanceUntilIdle(); clearMocks(flow, answers = false)
        model.start(choice, label, TripCapture.Absent("x"), 0, 6); model.finish(trip, TripCapture.Absent("x"), 1, 7)
        model.sendNow(0, 6); model.openTrip(trip, 1, 7); model.open(context, 1)
        advanceUntilIdle()
        coVerify(exactly = 0) { flow.start(any(), any(), any(), any(), any()) }
        coVerify(exactly = 0) { flow.finish(any(), any(), any(), any()) }
        coVerify(exactly = 0) { flow.sendNow(any(), any()) }
        coVerify(exactly = 0) { flow.open(any(), any()) }
        verify(exactly = 0) { flow.openTrip(any(), any(), any()) }
        states.value = states.value.copy(busy = true)
        model.log(trip, TripLogbookType.NOTE, "x", 0, 7); advanceUntilIdle()
        coVerify(exactly = 0) { flow.log(any(), any(), any(), any(), any()) }
    }

    @Test fun `a finished delivery re-reads the protected book of the visible owner`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle(); clearMocks(flow, answers = false)
        val partition = context.partition
        val attempt = com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease.capture(
            EnterpriseLocalIdentity(partition.ownerAccount, partition.origin, "10000000-0000-4000-8000-000000000009"), 0, 0,
            "10000000-0000-4000-8000-000000000010") { true }.forEnrollment(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(partition,
            context.membershipId), "10000000-0000-4000-8000-000000000011", 1))
        activity.end(activity.begin(attempt, listOf(com.anpfuel.domain.enterprise.sync.EnterpriseOperationIdentity(
            "10000000-0000-4000-8000-000000000012", "sha256:" + "a".repeat(64)))))
        advanceUntilIdle()
        coVerify(atLeast = 1) { flow.reload(0, 7) }
        assertNotNull(model)
    }

    @Test fun `withdrawal and clearing close the owner`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle(); clearMocks(flow, answers = false)
        model.withdraw(); model.invalidateIfStale()
        verify(exactly = 1) { flow.withdraw() }
        verify(exactly = 1) { flow.invalidateIfStale() }
        holder.clear()
        verify(exactly = 1) { flow.close() }
        model.withdraw(); model.start(choice, label, TripCapture.Absent("x"), 0, 7); advanceUntilIdle()
        verify(exactly = 1) { flow.withdraw() }
        coVerify(exactly = 0) { flow.start(any(), any(), any(), any(), any()) }
    }

    @Test fun `a photo save is forwarded only for the current gesture and works offline`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle(); clearMocks(flow, answers = false)
        val bytes = ByteArray(32) { 7 }.also { it[0] = 0xff.toByte(); it[1] = 0xd8.toByte(); it[2] = 0xff.toByte(); it[30] = 0xff.toByte(); it[31] = 0xd9.toByte() }
        val original = (com.anpfuel.domain.enterprise.attachment.ReceiptIntake.inspect(bytes, com.anpfuel.domain.enterprise.attachment.ReceiptSource.CAMERA)
            as com.anpfuel.domain.enterprise.attachment.ReceiptInspection.Accepted).original
        val draft = com.anpfuel.application.enterprise.attachment.ReceiptDraft(original, com.anpfuel.domain.enterprise.attachment.LegibilityHint.NOT_CHECKED)
        val target = com.anpfuel.domain.enterprise.attachment.AttachmentTarget(com.anpfuel.domain.enterprise.document.DocumentTargetKind.METER_READING,
            "10000000-0000-4000-8000-000000000062")
        model.attach(trip, target, draft, 0, 6); model.attach(trip, target, draft, 1, 7); advanceUntilIdle()
        coVerify(exactly = 0) { flow.attach(any(), any(), any(), any(), any()) }
        model.attach(trip, target, draft, 0, 7); advanceUntilIdle()
        coVerify(exactly = 1) { flow.attach(trip, target, draft, 0, 7) }
    }
}
