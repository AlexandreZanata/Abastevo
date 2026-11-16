package com.anpfuel.app.ui.enterprise

import androidx.lifecycle.ViewModelStore
import com.anpfuel.app.ui.enterprise.tracking.MyTrackingViewModel
import com.anpfuel.app.ui.map.TripRecordingServiceState
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.application.enterprise.tracking.*
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.tracking.LocationPermission
import io.mockk.*
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/** EMP-GPS-015 (B-BR-EMP-548/550): the enterprise indicator follows only the organization collector of this process. */
@OptIn(ExperimentalCoroutinesApi::class)
class MyTrackingViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val holder = ViewModelStore()
    private val flow = mockk<MyTrackingFlow>(relaxed = true)
    private val fence = EnterpriseSessionFence()
    private fun id(n: Int) = "10000000-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val partition = EnterprisePartition.of(id(1), "https://example.invalid", id(2))
    private val context = EnterpriseContextSummary(partition, id(3), "Synthetic company", id(5))
    private val identity = EnterpriseLocalIdentity(partition.ownerAccount, partition.origin, id(6))
    private val enrollment = EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(partition, context.membershipId), id(7), 1)
    private val source = object : EnterpriseQueueSource {
        override suspend fun account(): EnterpriseQueueAccountLease = EnterpriseQueueAccountLease.capture(identity, 0, 0, id(8)) { true }
        override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt =
            account.forEnrollment(enrollment)
    }
    private var elapsed = 0L
    private val runtime = TrackingRuntime(TrackingStartGate { elapsed })
    private val platform = object : TrackingPlatform {
        var sink: ((TrackingFixSample) -> Unit)? = null
        override fun permission() = LocationPermission.PRECISE
        override fun notificationVisible() = true
        override fun bootId() = "boot-1"
        override fun attach(precision: LocationPermission, sink: (TrackingFixSample) -> Unit): Boolean { this.sink = sink; return true }
        override fun detach() { sink = null }
        override fun wallMillis() = 1_763_208_000_000L
        override fun elapsedMillis() = elapsed
        override fun batteryLow() = false
        override fun providerEnabled(precision: LocationPermission) = true
    }
    private val service = mockk<EnterpriseTrackingService>(relaxed = true)

    @BeforeEach fun setup() {
        Dispatchers.setMain(dispatcher)
        every { flow.state } returns MutableStateFlow(MyTrackingViewState())
        coEvery { service.collecting(any(), partition, id(41)) } returns TrackingCollecting(id(41), id(40), LocationPermission.PRECISE)
        coEvery { service.checkpoint(any(), any(), any(), any()) } returns true
    }
    @AfterEach fun cleanup() { holder.clear(); TripRecordingServiceState.update { it.copy(active = false) }; Dispatchers.resetMain() }
    private fun model() = MyTrackingViewModel(flow, fence, runtime).also { holder.put("tracking", it) }

    @Test fun `personal session displayed as corporate - the personal recorder never turns the enterprise indicator on, nor the reverse`(): Unit =
        runTest(dispatcher) {
            val model = model(); advanceUntilIdle()
            TripRecordingServiceState.update { it.copy(active = true, points = 12) }
            advanceUntilIdle()
            assertNull(model.activity.value, "a personal recording is never organization tracking")
            TripRecordingServiceState.update { it.copy(active = false, points = 0) }
            val collector = TrackingCollector(service, source, runtime.gate, platform)
            runtime.gate.arm(partition, id(41))
            assertEquals(TrackingBeginResult.STARTED, collector.begin(partition, id(41), context.organizationName))
            runtime.collector = collector
            advanceUntilIdle()
            val active = model.activity.value!!
            assertEquals("Synthetic company", active.organizationName); assertEquals(partition, active.partition)
            assertFalse(TripRecordingServiceState.state.value.active, "organization tracking is never shown as the personal recorder")
            collector.release()
            advanceUntilIdle()
            assertNull(model.activity.value, "hidden at once when the listener is released")
        }

    @Test fun `my tracking opens only for the rendered epoch and reloads only its own generation`(): Unit = runTest(dispatcher) {
        val model = model(); advanceUntilIdle()
        model.open(context, fence.epoch + 1); advanceUntilIdle()
        coVerify(exactly = 0) { flow.open(any(), any()) }
        model.open(context, fence.epoch); advanceUntilIdle()
        coVerify(exactly = 1) { flow.open(context, fence.epoch) }
        model.reload(fence.epoch, 0); advanceUntilIdle()
        coVerify(exactly = 1) { flow.reload(fence.epoch, 0) }
        model.withdraw()
        verify(exactly = 1) { flow.withdraw() }
    }
}
