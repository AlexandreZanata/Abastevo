package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.tracking.*
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.tracking.*
import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** EMP-GPS-003 buffer, batches and gaps through the real producers and queue fakes (B-BR-EMP-501..505). Synthetic positions. */
class EnterpriseTrackingBufferTest {
    private class Drive(val x: TrackingFixture) {
        var lon = -56.1
        lateinit var session: String
        lateinit var trip: String
        val collector = x.collector()
        suspend fun begin() {
            trip = x.startTrip(); x.refresh()
            session = x.tracking.start(x.account(), x.a, trip, x.device()).sessionId
            x.gate.arm(x.a.partition, session)
            assertEquals(TrackingBeginResult.STARTED, collector.begin(x.a.partition, session))
        }
        /** Moves [seconds] at about 14 m/s and reports one fresh fix. */
        fun move(seconds: Long, metres: Double = seconds * 14.0, accuracy: Float? = 8f, mock: Boolean = false, speed: Float = 14f) {
            x.platform.wall += seconds * 1000; x.platform.elapsed += seconds * 1000; x.f.wall = x.platform.wall
            lon += metres / 108_000.0
            x.platform.sink!!(TrackingFixSample(-15.6, lon, accuracy, speed, 90f, x.platform.wall, x.platform.elapsed, mock))
        }
        fun idle(seconds: Long) { x.platform.wall += seconds * 1000; x.platform.elapsed += seconds * 1000; x.f.wall = x.platform.wall }
        suspend fun view() = x.session(trip)!!
    }

    @Test fun `recorded fixes are durable before they count, sealed after 60 s into one batch and stored once`() = runTest {
        val d = Drive(TrackingFixture()); d.begin()
        d.move(1); d.move(5); d.move(5); d.move(5)
        assertEquals(4, d.collector.drain())
        assertEquals(2, d.view().pendingFixes, "first fix and the 10-second record")
        assertTrue(d.view().batches.isEmpty())
        repeat(12) { d.move(5) }
        assertTrue(d.collector.tick())
        val view = d.view()
        assertEquals(1, view.batches.size); assertEquals(0, view.pendingFixes)
        val batch = TrackingBatchOperations.read(d.x.operation(view.batches.single().operationId))
        assertEquals(1L, batch.sequence)
        assertEquals(listOf(view.records.first().operationId), batch.dependsOn, "depends on the start that opened the period")
        assertEquals(batch.fixes.size.toLong(), 8L)
        assertEquals("boot-7", batch.bootId)
        assertEquals(1, d.x.deliveries)
        assertEquals(EnterpriseSyncStatus.ACCEPTED, d.view().batches.single().sync)
        assertEquals(1, d.x.server.batches.size)
        // A second tick has nothing due and sends nothing again.
        assertTrue(d.collector.tick()); assertEquals(1, d.x.server.batches.size)
        assertFalse(d.x.server.batches.single().toString().contains("-15.6"))
    }

    @Test fun `the stop seals what was recorded and nothing after it is recorded`() = runTest {
        val d = Drive(TrackingFixture()); d.begin()
        d.move(1); d.move(10); d.collector.drain()
        assertTrue(d.collector.stopByDriver())
        val stopped = d.view()
        assertEquals(TrackingSessionState.INTERRUPTED, stopped.state)
        assertEquals(1, stopped.batches.size); assertEquals(0, stopped.pendingFixes)
        val stopAt = stopped.since
        assertTrue(TrackingBatchOperations.read(d.x.operation(stopped.batches.single().operationId)).fixes.size == 2)
        // A late report of the released listener and a direct record after the stop store nothing.
        assertNull(d.x.platform.sink)
        assertEquals(TrackingObservationOutcome.IGNORED, d.x.tracking.record(d.x.account(), d.x.a.partition, d.session,
            TrackingObservation(-15.6, -56.0, 5f, 10f, 90f, false, d.x.platform.elapsed), false, d.x.platform.elapsed,
            Instant.ofEpochMilli(d.x.platform.wall), "boot-7"))
        assertEquals(0, d.view().pendingFixes)
        assertTrue(TrackingBatchOperations.read(d.x.operation(stopped.batches.single().operationId)).let { b ->
            b.fixes.all { !b.declared.occurredAt.plusMillis(it.offsetMillis).isAfter(stopAt) } })
    }

    @Test fun `a gap opens with its reason, seals what came before and no batch spans it`() = runTest {
        val d = Drive(TrackingFixture()); d.begin()
        d.move(1); d.move(10); d.collector.drain()
        d.idle(181); assertTrue(d.collector.tick())
        val gap = d.view().openGap!!
        assertEquals(TrackingGapReason.SIGNAL_LOST, gap.reason)
        assertEquals(1, d.view().batches.size, "the fixes before the gap left in their own batch")
        d.move(5, metres = 40.0); d.move(10); d.collector.drain()
        assertNull(d.view().openGap)
        d.x.platform.providerEnabled = false
        assertTrue(d.collector.tick())
        assertEquals(TrackingGapReason.PROVIDER_DISABLED, d.view().openGap!!.reason)
        assertEquals(2, d.view().batches.size)
        val batches = d.view().batches.map { TrackingBatchOperations.read(d.x.operation(it.operationId)) }
        assertEquals(listOf(1L, 2L), batches.map { it.sequence })
        val first = batches[0]; val second = batches[1]
        assertTrue(first.declared.occurredAt.plusMillis(first.fixes.last().offsetMillis) <= gap.start)
        assertTrue(second.declared.occurredAt > gap.start, "the next batch starts after the gap")
        // Mock reports only: counted, never recorded, and the gap says so.
        val m = Drive(TrackingFixture()); m.begin()
        m.move(1); m.collector.drain()
        repeat(4) { m.move(50, mock = true) }; m.collector.drain()
        assertEquals(1, m.view().pendingFixes)
        assertTrue(m.collector.tick())
        assertEquals(TrackingGapReason.MOCK_LOCATION, m.view().openGap!!.reason)
        assertEquals(4L, TrackingBatchOperations.read(m.x.operation(m.view().batches.single().operationId)).dropped.mock, "the mocks travel as counters")
    }

    @Test fun `after a process death the buffer survives, the interruption follows the last fix and the fixes are sealed`() = runTest {
        val x = TrackingFixture(); val d = Drive(x); d.begin()
        d.move(1); d.move(10); d.collector.drain()
        val lastFix = Instant.ofEpochMilli(x.platform.wall)
        // The process dies: no release, no seal. The next opening finds the session with no running collector.
        d.idle(600)
        assertEquals(1, x.tracking.recover(x.account(), x.a.partition, { false }, "boot-7"))
        val view = d.view()
        assertEquals(TrackingSessionState.INTERRUPTED, view.state); assertEquals(TrackingCause.PROCESS_STOPPED, view.cause)
        assertEquals(lastFix.plusMillis(1), view.since, "the gap starts just after the last recorded fix")
        assertEquals(0, view.pendingFixes); assertEquals(1, view.batches.size)
        val batch = TrackingBatchOperations.read(x.operation(view.batches.single().operationId))
        assertEquals(2, batch.fixes.size)
        // A resume on another boot records on that boot; the previous boot's fixes were all sealed first. The dead process's
        // listener is gone with it.
        x.platform.port.detach()
        x.tracking.start(x.account(), x.a, d.trip, x.device())
        x.gate.arm(x.a.partition, d.session); x.platform.boot = "boot-8"
        val again = x.collector()
        assertEquals(TrackingBeginResult.STARTED, again.begin(x.a.partition, d.session))
        x.platform.wall += 1000; x.platform.elapsed = 2_000; x.f.wall = x.platform.wall
        x.platform.sink!!(TrackingFixSample(-15.6, -56.0, 5f, 14f, 90f, x.platform.wall, x.platform.elapsed, false))
        again.drain()
        assertEquals(1, d.view().pendingFixes)
        assertTrue(again.stopByDriver())
        val second = TrackingBatchOperations.read(x.operation(d.view().batches.last().operationId))
        assertEquals("boot-8", second.bootId); assertEquals(2L, second.sequence)
        assertEquals(listOf(d.view().records.first { it.type == TrackingOperations.RESUME }.operationId), second.dependsOn)
    }

    @Test fun `the batch producer accepts only the buffer's oldest run and an oversized run is split under 8 KiB`() = runTest {
        val x = TrackingFixture(); val d = Drive(x); d.begin()
        val start = Instant.ofEpochMilli(x.platform.wall)
        // 200 coarse fixes far apart (no jump check in an approximate period) make a run above the 8 KiB bound.
        val pending = (0 until 200).map { i -> BufferedFix(start.plusSeconds(i * 120L + 1), 5_000L + i * 120_000L,
            if (i % 2 == 0) 899_999_999 else -899_999_999, if (i % 2 == 0) -1_799_999_999 else 1_799_999_999, 10_000, 100_000, 359, true) }
        // No fix was recorded yet, so the buffer does not exist: write one holding these fixes.
        x.store.updateBuffer(x.f.source.attempt(x.account(), x.a.partition), d.session) { current ->
            assertNull(current)
            SessionBuffer(d.session, d.trip, "boot-7", 1, pending, TrackingDropped(accuracy = 2), null, null, null, null, null, emptyList()) }
        assertEquals(2, x.tracking.seal(x.account(), x.a.partition, d.session, force = true))
        val batches = d.view().batches.map { x.operation(it.operationId) }
        assertTrue(batches.all { it.canonicalBytes.size <= 8192 })
        val facts = batches.map(TrackingBatchOperations::read)
        assertEquals(listOf(1L, 2L), facts.map { it.sequence })
        assertEquals(200, facts.sumOf { it.fixes.size })
        assertEquals(listOf(2L, 0L), facts.map { it.dropped.accuracy }, "the counters travel once")
        assertEquals(0, d.view().pendingFixes)
        // A batch built by hand that is not the oldest run (wrong sequence) is refused inside the protected write.
        x.store.updateBuffer(x.f.source.attempt(x.account(), x.a.partition), d.session) { b ->
            b!!.let { SessionBuffer(it.sessionId, it.tripId, it.bootId, it.nextSequence, pending.take(2), TrackingDropped(), null, null, null, null, null,
                emptyList(), batchOperationIds = it.batchOperationIds) } }
        val attempt = x.f.source.attempt(x.account(), x.a.partition)
        val envelope = TrackingEnvelope(com.anpfuel.domain.enterprise.sync.SyncBinding.of(x.a.partition.ownerAccount, x.a.partition.origin,
            x.a.partition.organizationId), x.a.membershipId, attempt.enrollment.enrollmentId, "7e000000-0000-4000-8000-000000009999",
            com.anpfuel.domain.enterprise.trip.TripDeclarations.at(pending.first().at.toEpochMilli(), x.zone))
        val wrong = TrackingBatchOperations.batch(envelope, d.session, d.trip, 9, "boot-7", pending.first().elapsedMillis,
            TrackingBatching.fixes(pending.take(2)), TrackingDropped(), listOf(d.view().records.first().operationId))
        assertThrows(Exception::class.java) { kotlinx.coroutines.runBlocking { x.delivery.save(x.a.partition, wrong) } }
        assertEquals(2, d.view().pendingFixes, "nothing left the buffer")
    }

    @Test fun `accepted fixes that are not recorded rewrite the buffer at most once a minute`() = runTest {
        val x = TrackingFixture(); val d = Drive(x); d.begin()
        d.move(1); d.collector.drain()
        val writes = x.bufferWrites
        // Parked: accepted every 5 s, recorded only at the 120-second heartbeat.
        repeat(11) { d.move(5, metres = 0.0, speed = 0f) }; d.collector.drain()
        assertTrue(x.bufferWrites - writes <= 1, "at most one anchor write in 55 s")
        repeat(14) { d.move(5, metres = 0.0, speed = 0f) }; d.collector.drain()
        assertEquals(2, d.view().pendingFixes, "the heartbeat recorded the second fix")
        d.idle(100); assertTrue(d.collector.tick())
        assertNull(d.view().openGap, "a running parked collector never opens a false gap")
    }
}
