package com.anpfuel.app.ui.enterprise

import androidx.lifecycle.ViewModelStore
import com.anpfuel.app.ui.enterprise.trip.TripViewModel
import com.anpfuel.app.ui.enterprise.tracking.TrackingServiceLauncher
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.sync.EnterpriseDeliveryActivity
import com.anpfuel.application.enterprise.tracking.TrackingScreen
import com.anpfuel.application.enterprise.tracking.TrackingSessionView
import com.anpfuel.application.enterprise.trip.TripCapture
import com.anpfuel.application.enterprise.trip.TripFlow
import com.anpfuel.application.enterprise.trip.TripViewState
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.tracking.LocationPermission
import com.anpfuel.domain.enterprise.tracking.TrackingDevice
import com.anpfuel.domain.enterprise.tracking.TrackingPolicy
import com.anpfuel.domain.enterprise.tracking.TrackingSessionState
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

/** EMP-GPS-002 (B-BR-EMP-496/499): the trip owner starts the service only for a session that is now recording. */
@OptIn(ExperimentalCoroutinesApi::class)
class TripTrackingViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val holder = ViewModelStore()
    private val flow = mockk<TripFlow>(relaxed = true)
    private val fence = EnterpriseSessionFence()
    private val online = MutableStateFlow(false)
    private val context = EnterpriseContextSummary(EnterprisePartition.of(
        "10000000-0000-4000-8000-000000000001", "https://example.invalid", "10000000-0000-4000-8000-000000000002"),
        "10000000-0000-4000-8000-000000000003", "Synthetic company", "10000000-0000-4000-8000-000000000005")
    private val states = MutableStateFlow(TripViewState(generation = 7, epoch = 0, context = context, loaded = true))
    private val trip = "10000000-0000-4000-8000-000000000040"
    private val session = "10000000-0000-4000-8000-000000000041"
    private val launched = mutableListOf<Pair<EnterprisePartition, String>>()
    private val released = mutableListOf<String>()
    private val launcher = object : TrackingServiceLauncher {
        override fun launch(partition: EnterprisePartition, sessionId: String, organizationName: String) { launched += partition to sessionId }
        override fun release(sessionId: String) { released += sessionId }
    }
    private fun view(state: TrackingSessionState, refused: Boolean = false) = TrackingSessionView(session, trip, TrackingPolicy.EXPECTED, state,
        null, Instant.parse("2026-11-15T12:00:00Z"), if (state == TrackingSessionState.RECORDING) LocationPermission.PRECISE else null,
        refused, if (refused) "FORBIDDEN" else null, null, emptyList())
    private fun tracked(state: TrackingSessionState, refused: Boolean = false) =
        states.value.copy(tracking = TrackingScreen(null, listOf(view(state, refused))))

    @BeforeEach fun setup() { Dispatchers.setMain(dispatcher); every { flow.state } returns states }
    @AfterEach fun cleanup() { holder.clear(); Dispatchers.resetMain() }
    private fun model() = TripViewModel(flow, object : NetworkConnectivityGateway { override fun observeIsConnected() = online },
        fence, EnterpriseDeliveryActivity(), launcher).also { holder.put("tracking", it) }

    @Test fun `only a session recording after the tap launches the service, and an attempt launches nothing`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle()
        val device = TrackingDevice(LocationPermission.NONE, true)
        coEvery { flow.startTracking(trip, device, 0, 7) } answers { states.value = tracked(TrackingSessionState.NOT_STARTED) }
        model.startTracking(trip, device, 0, 7); advanceUntilIdle()
        coVerify(exactly = 1) { flow.startTracking(trip, device, 0, 7) }
        assertTrue(launched.isEmpty(), "a denied attempt starts no service")
        val granted = TrackingDevice(LocationPermission.PRECISE, true)
        coEvery { flow.startTracking(trip, granted, 0, 7) } answers { states.value = tracked(TrackingSessionState.RECORDING) }
        model.startTracking(trip, granted, 0, 7); advanceUntilIdle()
        assertEquals(listOf(context.partition to session), launched)
        coEvery { flow.startTracking(trip, granted, 0, 7) } answers { states.value = tracked(TrackingSessionState.RECORDING, refused = true) }
        model.startTracking(trip, granted, 0, 7); advanceUntilIdle()
        assertEquals(1, launched.size, "a refused session never listens")
    }

    @Test fun `stop and finish release the listener after the write and old gestures do nothing`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle()
        states.value = tracked(TrackingSessionState.RECORDING)
        model.stopTracking(trip, 0, 7); advanceUntilIdle()
        coVerify(exactly = 1) { flow.stopTracking(trip, 0, 7) }
        assertEquals(listOf(session), released)
        coEvery { flow.finish(trip, any(), 0, 7) } answers { states.value = tracked(TrackingSessionState.ENDED) }
        model.finish(trip, TripCapture.Absent("Painel apagado"), 0, 7); advanceUntilIdle()
        assertEquals(listOf(session, session), released)
        model.startTracking(trip, TrackingDevice(LocationPermission.PRECISE, true), 1, 7); model.stopTracking(trip, 0, 6); advanceUntilIdle()
        coVerify(exactly = 0) { flow.startTracking(any(), any(), any(), any()) }
        coVerify(exactly = 1) { flow.stopTracking(any(), any(), any()) }
        model.refreshTrackingPolicy(0, 7); advanceUntilIdle()
        coVerify(exactly = 0) { flow.refreshTrackingPolicy(any(), any()) }
        online.value = true; advanceUntilIdle()
        model.refreshTrackingPolicy(0, 7); advanceUntilIdle()
        coVerify(exactly = 1) { flow.refreshTrackingPolicy(0, 7) }
    }
}
