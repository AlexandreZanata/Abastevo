package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.tracking.*
import com.anpfuel.domain.enterprise.tracking.*
import java.time.Instant
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/**
 * EMP-GPS-015 transparency through the real producers, queue fakes and collector (B-BR-EMP-548..550). Synthetic positions
 * only; the indicator state is the process runtime's, the history the partition's own sealed records.
 */
class EnterpriseTrackingTransparencyTest {
    private class Drive(val x: TrackingFixture = TrackingFixture()) {
        val runtime = TrackingRuntime(x.gate)
        val collector = TrackingCollector(x.tracking, x.f.source, x.gate, x.platform.port) { account, partition -> x.delivery.dispatch(account, partition) }
        var lon = -56.1
        lateinit var trip: String
        lateinit var session: String
        suspend fun open() {
            trip = x.startTrip(); x.refresh()
            session = x.tracking.start(x.account(), x.a, trip, x.device()).sessionId
        }
        suspend fun begin(): TrackingBeginResult {
            x.gate.arm(x.a.partition, session)
            return collector.begin(x.a.partition, session, x.a.organizationName).also { if (it == TrackingBeginResult.STARTED) runtime.collector = collector }
        }
        fun move(seconds: Long) {
            x.platform.wall += seconds * 1000; x.platform.elapsed += seconds * 1000; x.f.wall = x.platform.wall
            lon += seconds * 14.0 / 108_000.0
            x.platform.sink!!(TrackingFixSample(-15.6, lon, 8f, 14f, 90f, x.platform.wall, x.platform.elapsed, false))
        }
        fun idle(seconds: Long) { x.platform.wall += seconds * 1000; x.platform.elapsed += seconds * 1000; x.f.wall = x.platform.wall }
        suspend fun activity() = runtime.activity.first()
        suspend fun buffer() = x.store.read(x.f.source.attempt(x.account(), x.a.partition)).buffers?.session(session)
    }

    @Test fun `hidden status - the runtime states an active collector from its attach to its release and never from a stored session alone`() = runTest {
        val d = Drive(); d.open()
        assertNull(d.activity(), "a stored recording session is not an active collector")
        d.runtime.collector = d.collector
        assertNull(d.activity(), "a collector that does not listen is not active")
        assertEquals(TrackingBeginResult.STARTED, d.begin())
        val active = d.activity()!!
        assertEquals(d.x.a.partition, active.partition); assertEquals(d.session, active.sessionId); assertEquals(d.trip, active.tripId)
        assertEquals("Synthetic A", active.organizationName); assertEquals(LocationPermission.PRECISE, active.precision)
        assertEquals(Instant.ofEpochMilli(d.x.platform.wall), active.since); assertNull(active.gap)
        assertEquals("EnterpriseTrackingActivity(redacted)", active.toString())
        // Location off: the indicator says so; the next fix clears it.
        d.move(1); d.collector.drain()
        d.x.platform.providerEnabled = false
        assertTrue(d.collector.tick())
        assertEquals(TrackingGapReason.PROVIDER_DISABLED, d.activity()!!.gap)
        d.x.platform.providerEnabled = true
        d.move(10); d.collector.drain()
        assertNull(d.activity()!!.gap)
        // Approximate only: the indicator follows the reduction.
        d.x.platform.permission = LocationPermission.APPROXIMATE
        assertTrue(d.collector.tick())
        assertEquals(LocationPermission.APPROXIMATE, d.activity()!!.precision)
        // The stop releases the listener: the indicator disappears at once, before the service drops the collector.
        assertTrue(d.collector.stopByDriver())
        assertNull(d.activity())
        assertSame(d.collector, d.runtime.collector)
        // A watchdog interruption clears it too.
        val e = Drive(); e.open()
        assertEquals(TrackingBeginResult.STARTED, e.begin()); assertNotNull(e.activity())
        e.x.platform.permission = LocationPermission.NONE
        assertFalse(e.collector.tick()); assertNull(e.activity())
    }

    @Test fun `collection after stop - nothing is recorded after a stop or a finish and the history shows each end`() = runTest {
        val d = Drive(); d.open()
        assertEquals(TrackingBeginResult.STARTED, d.begin())
        d.move(1); d.move(10); d.collector.drain()
        val sink = d.x.platform.sink!!
        assertTrue(d.collector.stopByDriver())
        val stoppedAt = d.x.tracking.screen(d.x.account(), d.x.a.partition).session(d.trip)!!.since
        val recorded = d.buffer()!!.recorded
        // A late report of the released listener is not recorded.
        d.x.platform.wall += 5_000; d.x.platform.elapsed += 5_000
        sink(TrackingFixSample(-15.6, -56.0, 5f, 14f, 90f, d.x.platform.wall, d.x.platform.elapsed, false))
        d.collector.drain()
        assertEquals(recorded, d.buffer()!!.recorded)
        // Resumed, recorded, then the trip finishes: the watchdog follows the finish and releases; nothing after it counts.
        d.idle(30)
        d.x.tracking.start(d.x.account(), d.x.a, d.trip, d.x.device())
        assertEquals(TrackingBeginResult.STARTED, d.begin())
        d.move(5); d.move(10); d.collector.drain()
        d.idle(5)
        d.x.trips.finish(d.x.account(), d.x.a.partition, d.trip, d.x.f.km("184400"))
        val late = d.x.platform.sink!!
        assertFalse(d.collector.tick())
        assertNull(d.activity())
        val afterFinish = d.buffer()!!.recorded
        d.x.platform.wall += 5_000; d.x.platform.elapsed += 5_000
        late(TrackingFixSample(-15.6, -55.9, 5f, 14f, 90f, d.x.platform.wall, d.x.platform.elapsed, false))
        d.collector.drain()
        assertEquals(afterFinish, d.buffer()!!.recorded)
        val entry = d.x.tracking.history(d.x.account(), d.x.a.partition).entries.single()
        assertEquals(TrackingSessionState.ENDED, entry.session.state)
        val end = entry.endedAt!!
        val batches = entry.session.batches.map { TrackingBatchOperations.read(d.x.operation(it.operationId)) }
        val instants = batches.flatMap { b -> b.fixes.map { b.declared.occurredAt.plusMillis(it.offsetMillis) } } + d.buffer()!!.pending.map { it.at }
        assertEquals(d.buffer()!!.recorded, instants.size.toLong())
        assertTrue(instants.all { !it.isAfter(end) }, "no position after the end")
        assertEquals(2, instants.count { !it.isAfter(stoppedAt) }, "the first run ends at the stop")
        assertEquals(listOf(TrackingCause.STOPPED_BY_DRIVER), entry.session.interruptions.map { it.cause })
    }

    @Test fun `personal session displayed as corporate - the history holds only this member's sessions of this organization on this phone`() = runTest {
        val x = TrackingFixture()
        assertTrue(x.tracking.history(x.account(), x.a.partition).entries.isEmpty(), "nothing tracked: empty history")
        val first = x.startTrip(); x.refresh()
        x.tracking.start(x.account(), x.a, first, x.device())
        x.f.later(1); x.tracking.stop(x.account(), x.a.partition, first)
        x.f.later(1)
        x.trips.start(x.account(), x.a, x.f.choice(asset = 71), x.f.label, x.f.km("184320"))
        val second = x.trips.trips(x.account(), x.a.partition).map { it.trip.tripId }.single { it != first }
        x.f.later(1); x.tracking.start(x.account(), x.a, second, x.device(LocationPermission.NONE))
        val history = x.tracking.history(x.account(), x.a.partition)
        assertEquals(listOf(second, first), history.entries.map { it.session.tripId }, "newest first")
        assertEquals(TrackingPolicy.EXPECTED, history.policy!!.policy)
        assertEquals(x.f.label.name, history.entries.last().vehicle!!.name)
        assertNull(history.entries.first().startedAt, "a session that never recorded has no tracking start")
        assertNotNull(history.entries.last().startedAt); assertNull(history.entries.last().endedAt)
        assertEquals("EnterpriseTrackingHistory(redacted)", history.toString())
        // Another organization's partition on this phone shows none of these; another account reads nothing here.
        assertTrue(x.tracking.history(x.account(), x.f.b.partition).entries.isEmpty())
        x.f.identity = EnterpriseLocalIdentity("7e000000-0000-4000-8000-000000000003", x.a.partition.origin, "7e000000-0000-4000-8000-000000000004")
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { x.tracking.history(x.account(), x.a.partition) } }
    }

    @Test fun `my tracking reads the own sessions offline, follows the fence and never shows another context`() = runTest {
        val x = TrackingFixture()
        val trip = x.startTrip(); x.refresh()
        x.tracking.start(x.account(), x.a, trip, x.device())
        val flow = MyTrackingFlow(x.f.source, x.tracking, x.f.fence)
        flow.open(x.a, x.f.fence.epoch)
        val state = flow.state.value
        assertTrue(state.loaded); assertNull(state.error)
        assertEquals(listOf(trip), state.history!!.entries.map { it.session.tripId })
        assertEquals("MyTrackingViewState(redacted)", state.toString())
        // A reload re-reads; an old generation does nothing.
        x.f.later(1); x.tracking.stop(x.account(), x.a.partition, trip)
        flow.reload(x.f.fence.epoch, state.generation)
        assertEquals(TrackingSessionState.INTERRUPTED, flow.state.value.history!!.entries.single().session.state)
        val generation = flow.state.value.generation
        flow.reload(x.f.fence.epoch, state.generation)
        assertEquals(generation, flow.state.value.generation)
        // Another organization's context reads its own (empty) history.
        flow.open(x.f.b, x.f.fence.epoch)
        assertTrue(flow.state.value.history!!.entries.isEmpty())
        // A storage failure says so; the fence changing withdraws the view.
        flow.open(x.a, x.f.fence.epoch)
        x.storeFails = true
        flow.reload(x.f.fence.epoch, flow.state.value.generation)
        assertEquals(EnterpriseAccessError.UNAVAILABLE, flow.state.value.error); assertNull(flow.state.value.history)
        x.storeFails = false
        flow.open(x.a, x.f.fence.epoch)
        x.f.identity = EnterpriseLocalIdentity("7e000000-0000-4000-8000-000000000003", x.a.partition.origin, "7e000000-0000-4000-8000-000000000004")
        flow.invalidateIfStale()
        assertEquals(EnterpriseAccessError.STALE, flow.state.value.error); assertNull(flow.state.value.history)
    }
}
