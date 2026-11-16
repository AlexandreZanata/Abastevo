package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.tracking.*
import com.anpfuel.application.enterprise.trip.TripFlow
import com.anpfuel.application.enterprise.trip.TripTracking
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.tracking.*
import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/**
 * EMP-GPS-010 interruptions through the real producers, queue fakes and collector (B-BR-EMP-544..547). Synthetic positions
 * only; a "process death" is a collector that is dropped without release, a "restart" another boot identity.
 */
class EnterpriseTrackingInterruptionTest {
    /** A tracking service over the fixture's store and queue whose buffer writes and batch saves can be made to fail. */
    private class Rig(val x: TrackingFixture = TrackingFixture()) {
        var failBuffer = false
        var failBatches = false
        private var nonce = 7000
        val store = object : EnterpriseTrackingStore by x.store {
            override suspend fun updateBuffer(attempt: EnterpriseQueueAttempt, sessionId: String, change: (SessionBuffer?) -> SessionBuffer?): SessionBuffer? {
                if (failBuffer) throw EnterprisePersistenceUnavailable()
                return x.store.updateBuffer(attempt, sessionId, change)
            }
        }
        val service = EnterpriseTrackingService(x.f.source, { partition: EnterprisePartition, operation: EnterpriseOperation ->
            if (failBatches && operation.type == TrackingBatchOperations.TYPE) throw EnterprisePersistenceUnavailable()
            x.delivery.save(partition, operation)
        }, store, x.gateway, { x.zone }, { x.f.wall }, { "7e000000-0000-4000-8000-" + (nonce++).toString().padStart(12, '0') })
        fun collector() = TrackingCollector(service, x.f.source, x.gate, x.platform.port) { account, partition -> x.delivery.dispatch(account, partition) }
        var lon = -56.1
        lateinit var trip: String
        lateinit var session: String
        suspend fun open(permission: LocationPermission = LocationPermission.PRECISE): String {
            trip = x.startTrip(); x.refresh()
            session = service.start(x.account(), x.a, trip, x.device(permission)).sessionId
            return session
        }
        suspend fun begin(collector: TrackingCollector): TrackingBeginResult { x.gate.arm(x.a.partition, session); return collector.begin(x.a.partition, session) }
        /** Moves [seconds] at about 14 m/s and reports one fresh fix through the attached listener. */
        fun move(seconds: Long) {
            x.platform.wall += seconds * 1000; x.platform.elapsed += seconds * 1000; x.f.wall = x.platform.wall
            lon += seconds * 14.0 / 108_000.0
            x.platform.sink!!(TrackingFixSample(-15.6, lon, 8f, 14f, 90f, x.platform.wall, x.platform.elapsed, false))
        }
        fun idle(seconds: Long) { x.platform.wall += seconds * 1000; x.platform.elapsed += seconds * 1000; x.f.wall = x.platform.wall }
        suspend fun view() = service.screen(x.account(), x.a.partition).session(trip)!!
        suspend fun buffer() = x.store.read(x.f.source.attempt(x.account(), x.a.partition)).buffers?.session(session)
        suspend fun batches() = view().batches.map { TrackingBatchOperations.read(x.operation(it.operationId)) }
        /** The instants of every fix the batches carry. */
        suspend fun batchInstants() = batches().flatMap { b -> b.fixes.map { b.declared.occurredAt.plusMillis(it.offsetMillis) } }
        /** B-BR-EMP-547: each recorded fix is pending once or in exactly one batch; none is lost or duplicated. */
        suspend fun assertConserved(message: String) {
            val buffer = buffer()!!
            val sealed = batchInstants()
            val pending = buffer.pending.map { it.at }
            assertEquals(buffer.recorded, (sealed.size + pending.size).toLong(), "$message: recorded = sealed + pending")
            assertEquals((sealed + pending).size, (sealed + pending).distinct().size, "$message: no fix twice")
        }
    }

    @Test fun `unapproved restart - recovery, a stale start intent and a grant for an interrupted session never resume collection`() = runTest {
        val r = Rig(); r.open()
        val first = r.collector()
        assertEquals(TrackingBeginResult.STARTED, r.begin(first))
        r.move(1); r.move(10); first.drain()
        // The process dies: its listener goes with it, nothing is written on the way out.
        r.x.platform.port.detach()
        r.idle(300)
        assertEquals(1, r.service.recover(r.x.account(), r.x.a.partition, { false }, "boot-7", r.x.device()))
        assertEquals(TrackingSessionState.INTERRUPTED, r.view().state)
        val second = r.collector()
        // A start intent without the screen's grant (a worker, a stale pending intent, a system restart) writes and listens to nothing.
        val before = r.x.entries().size
        assertEquals(TrackingBeginResult.NO_GRANT, second.begin(r.x.a.partition, r.session))
        // Even a fresh grant cannot resume a session the driver has not resumed on the visible screen.
        assertEquals(TrackingBeginResult.NOT_RECORDING, r.begin(second))
        assertEquals(1, r.x.platform.attaches); assertNull(r.x.platform.sink)
        assertEquals(before, r.x.entries().size, "nothing was written")
        assertNull(r.service.collecting(r.x.account(), r.x.a.partition, r.session))
        assertEquals(TrackingObservationOutcome.IGNORED, r.service.record(r.x.account(), r.x.a.partition, r.session,
            TrackingObservation(-15.6, -56.0, 5f, 10f, 90f, false, r.x.platform.elapsed), false, r.x.platform.elapsed,
            Instant.ofEpochMilli(r.x.platform.wall), "boot-7", LocationPermission.PRECISE))
        assertEquals(0, r.service.recover(r.x.account(), r.x.a.partition, { false }, "boot-9", r.x.device()), "a later opening resumes nothing")
        // The driver's tap resumes; only then does a grant start collection.
        assertEquals(TrackingSessionState.RECORDING, r.service.start(r.x.account(), r.x.a, r.trip, r.x.device()).state)
        assertEquals(TrackingBeginResult.STARTED, r.begin(second))
    }

    @Test fun `unapproved restart - every tap re-checks the policy in force and the one-listener rule before writing`() = runTest {
        val r = Rig(); r.open()
        r.service.stop(r.x.account(), r.x.a.partition, r.trip)
        val written = r.x.entries().size
        // The organization switched tracking off: the resume is refused and nothing is written, the interruption stays.
        r.x.policyAnswer = { at -> EnterpriseAccessResult.Ok(TrackingPolicyRead(TrackingPolicy.OFF, 4, null, null, at)) }
        r.x.refresh()
        assertEquals(TrackingLocalFailure.POLICY_OFF, assertThrows(TrackingLocalException::class.java) {
            kotlinx.coroutines.runBlocking { r.service.start(r.x.account(), r.x.a, r.trip, r.x.device()) } }.failure)
        assertEquals(written, r.x.entries().size)
        assertEquals(TrackingSessionState.INTERRUPTED, r.view().state)
        // Back on: another session already collected on this phone refuses the tap (one listener per device).
        r.x.policyAnswer = { at -> EnterpriseAccessResult.Ok(TrackingPolicyRead(TrackingPolicy.OPTIONAL, 5, "aviso-2026.3", Instant.parse("2026-11-01T00:00:00Z"), at)) }
        r.x.refresh()
        assertEquals(TrackingLocalFailure.BUSY, assertThrows(TrackingLocalException::class.java) {
            kotlinx.coroutines.runBlocking { r.service.start(r.x.account(), r.x.a, r.trip, r.x.device(), collecting = "7e000000-0000-4000-8000-000000000077") } }.failure)
        assertEquals(written, r.x.entries().size)
        // Its own running collector is not "another session".
        assertEquals(TrackingSessionState.RECORDING, r.service.start(r.x.account(), r.x.a, r.trip, r.x.device(), collecting = r.session).state)
        // A first start under a policy never read is refused as before, also for a retry of a session that only recorded attempts.
        val y = Rig(); y.x.startTrip().also { y.trip = it }
        y.x.refresh()
        val attempt = y.service.start(y.x.account(), y.x.a, y.trip, y.x.device(LocationPermission.NONE))
        assertEquals(TrackingSessionState.NOT_STARTED, attempt.state)
        y.x.policyAnswer = { at -> EnterpriseAccessResult.Ok(TrackingPolicyRead(TrackingPolicy.OFF, 4, null, null, at)) }
        y.x.refresh()
        assertEquals(TrackingLocalFailure.POLICY_OFF, assertThrows(TrackingLocalException::class.java) {
            kotlinx.coroutines.runBlocking { y.service.start(y.x.account(), y.x.a, y.trip, y.x.device()) } }.failure)
    }

    @Test fun `unapproved restart - with a connection the trip screen reads the policy first and a failed read keeps the cache`() = runTest {
        val r = Rig()
        var collecting: String? = null
        val flow = TripFlow(r.x.f.source, r.x.trips, r.x.f.fence, tracking = TripTracking(r.service, { false }, { "boot-7" },
            { r.x.device() }, { collecting })) { account, partition -> r.x.delivery.dispatch(account, partition) }
        r.trip = r.x.startTrip(); r.x.refresh()
        flow.open(r.x.a, r.x.f.fence.epoch)
        flow.startTracking(r.trip, r.x.device(), r.x.f.fence.epoch, flow.state.value.generation, online = true)
        val session = flow.state.value.tracking!!.session(r.trip)!!
        assertEquals(TrackingSessionState.RECORDING, session.state)
        assertEquals(2, r.x.policyReads.size, "the tap read the policy in force before writing")
        flow.stopTracking(r.trip, r.x.f.fence.epoch, flow.state.value.generation)
        // Online and the organization now says OFF: the resume is refused in the same gesture, nothing written.
        r.x.policyAnswer = { at -> EnterpriseAccessResult.Ok(TrackingPolicyRead(TrackingPolicy.OFF, 6, null, null, at)) }
        val written = r.x.entries().size
        flow.startTracking(r.trip, r.x.device(), r.x.f.fence.epoch, flow.state.value.generation, online = true)
        assertEquals(TrackingLocalFailure.POLICY_OFF, flow.state.value.trackingRefusal)
        assertEquals(written, r.x.entries().size)
        assertEquals(TrackingSessionState.INTERRUPTED, flow.state.value.tracking!!.session(r.trip)!!.state)
        // A failed online read keeps the cached value (here OFF) and says so.
        r.x.policyAnswer = { EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
        flow.startTracking(r.trip, r.x.device(), r.x.f.fence.epoch, flow.state.value.generation, online = true)
        assertEquals(TrackingLocalFailure.POLICY_OFF, flow.state.value.trackingRefusal)
        assertTrue(flow.state.value.trackingPolicyFailed)
        // On again (cached read refreshed online): the tap resumes; another collected session refuses the tap.
        r.x.policyAnswer = { at -> EnterpriseAccessResult.Ok(TrackingPolicyRead(TrackingPolicy.EXPECTED, 7, "aviso-2026.4", Instant.parse("2026-11-01T00:00:00Z"), at)) }
        collecting = "7e000000-0000-4000-8000-000000000078"
        flow.startTracking(r.trip, r.x.device(), r.x.f.fence.epoch, flow.state.value.generation, online = true)
        assertEquals(TrackingLocalFailure.BUSY, flow.state.value.trackingRefusal)
        collecting = null
        flow.startTracking(r.trip, r.x.device(), r.x.f.fence.epoch, flow.state.value.generation, online = false)
        assertEquals(TrackingSessionState.RECORDING, flow.state.value.tracking!!.session(r.trip)!!.state)
        assertNull(flow.state.value.trackingRefusal)
    }

    @Test fun `missing gap - a begin that cannot listen after the visible grant interrupts its session with the observed cause`() = runTest {
        val expected = mapOf("permission" to TrackingCause.PERMISSION_REVOKED, "notification" to TrackingCause.NOTIFICATION_BLOCKED,
            "platform" to TrackingCause.PROCESS_STOPPED, "busy" to TrackingCause.PROCESS_STOPPED)
        for ((case, cause) in expected) {
            val r = Rig(); r.open()
            val startedAt = r.view().since
            val collector = r.collector()
            if (case == "busy") {
                // The process's one collector already listens for this session: a grant for another one cannot be collected.
                assertEquals(TrackingBeginResult.STARTED, r.begin(collector))
                r.x.f.later(1)
                r.x.trips.start(r.x.account(), r.x.a, r.x.f.choice(asset = 71), r.x.f.label, r.x.f.km("184320"))
                val second = r.x.trips.trips(r.x.account(), r.x.a.partition).map { it.trip.tripId }.single { it != r.trip }
                val secondSession = r.service.start(r.x.account(), r.x.a, second, r.x.device()).sessionId
                r.x.gate.arm(r.x.a.partition, secondSession)
                assertEquals(TrackingBeginResult.BUSY, collector.begin(r.x.a.partition, secondSession), case)
                val view = r.service.screen(r.x.account(), r.x.a.partition).session(second)!!
                assertEquals(TrackingSessionState.INTERRUPTED, view.state, case); assertEquals(cause, view.cause, case)
                assertEquals(TrackingSessionState.RECORDING, r.view().state, "the listening session keeps recording")
                assertEquals(1, r.x.platform.attaches)
                continue
            }
            when (case) {
                "permission" -> r.x.platform.permission = LocationPermission.NONE
                "notification" -> r.x.platform.notification = false
                "platform" -> r.x.platform.refuse = true
            }
            r.x.f.later(1)
            assertNotEquals(TrackingBeginResult.STARTED, r.begin(collector), case)
            val view = r.view()
            assertEquals(TrackingSessionState.INTERRUPTED, view.state, case); assertEquals(cause, view.cause, case)
            assertEquals(startedAt, view.since, "$case: the interruption starts at the last proof of recording")
            assertEquals(listOf(startedAt), view.interruptions.map { it.start }, case)
            assertNull(r.x.platform.sink, case)
        }
        // A start intent without any grant writes nothing: it may not come from the visible screen.
        val r = Rig(); r.open()
        r.x.platform.permission = LocationPermission.NONE
        assertEquals(TrackingBeginResult.NO_GRANT, r.collector().begin(r.x.a.partition, r.session))
        assertEquals(TrackingSessionState.RECORDING, r.view().state)
    }

    @Test fun `missing gap - recovery names the cause observed now and a reduction found after a death is not invented`() = runTest {
        for ((device, cause) in listOf(TrackingDevice(LocationPermission.NONE, true) to TrackingCause.PERMISSION_REVOKED,
            TrackingDevice(LocationPermission.PRECISE, false) to TrackingCause.NOTIFICATION_BLOCKED,
            TrackingDevice(LocationPermission.APPROXIMATE, true) to TrackingCause.PROCESS_STOPPED)) {
            val r = Rig(); r.open()
            r.x.f.later(5)
            val flow = TripFlow(r.x.f.source, r.x.trips, r.x.f.fence, tracking = TripTracking(r.service, { false }, { "boot-7" }, { device })) { _, _ -> }
            flow.open(r.x.a, r.x.f.fence.epoch)
            val view = flow.state.value.tracking!!.session(r.trip)!!
            assertEquals(TrackingSessionState.INTERRUPTED, view.state); assertEquals(cause, view.cause)
            assertTrue(view.records.none { it.type == TrackingOperations.REDUCE }, "no reduction is invented")
            if (device.permission == LocationPermission.APPROXIMATE) {
                // The resume records the approximate grant: the next period is approximate and says so.
                flow.startTracking(r.trip, device, r.x.f.fence.epoch, flow.state.value.generation)
                val resumed = flow.state.value.tracking!!.session(r.trip)!!
                assertEquals(LocationPermission.APPROXIMATE, resumed.precision); assertTrue(resumed.coarse)
                assertEquals(listOf(cause), resumed.interruptions.map { it.cause })
                assertNotNull(resumed.interruptions.single().end)
            }
        }
        // Another boot wins over every other observation.
        val r = Rig(); r.open()
        assertTrue(r.service.checkpoint(r.x.account(), r.x.a.partition, r.session, "boot-7"))
        assertEquals(1, r.service.recover(r.x.account(), r.x.a.partition, { false }, "boot-8", TrackingDevice(LocationPermission.NONE, false)))
        assertEquals(TrackingCause.DEVICE_RESTARTED, r.view().cause)
    }

    @Test fun `missing gap - a batch never spans an interruption even when the seal at the interruption failed`() = runTest {
        val r = Rig(); r.open()
        val first = r.collector()
        assertEquals(TrackingBeginResult.STARTED, r.begin(first))
        r.move(1); r.move(10); first.drain()
        // The stop is written but its seal fails: the run's fixes stay pending.
        r.failBatches = true
        assertTrue(first.stopByDriver())
        r.failBatches = false
        val interruptedAt = r.view().interruptions.single().start
        assertEquals(2, r.buffer()!!.pending.size)
        // The driver resumes and records again; both runs' fixes wait together with no gap between them.
        r.idle(60)
        assertEquals(TrackingSessionState.RECORDING, r.service.start(r.x.account(), r.x.a, r.trip, r.x.device()).state)
        val second = r.collector()
        assertEquals(TrackingBeginResult.STARTED, r.begin(second))
        r.move(5); r.move(10); second.drain()
        assertEquals(4, r.buffer()!!.pending.size, "both runs wait together")
        assertTrue(r.buffer()!!.gaps.isEmpty())
        assertTrue(second.stopByDriver())
        val batches = r.batches()
        val records = r.view().records
        val start = records.first { it.type == TrackingOperations.START }.operationId
        val resume = records.first { it.type == TrackingOperations.RESUME }.operationId
        assertEquals(listOf(listOf(start), listOf(resume)), batches.map { it.dependsOn }, "one batch per run, each on its own opening")
        assertEquals(listOf(2, 2), batches.map { it.fixes.size })
        assertTrue(batches[0].let { b -> b.fixes.all { !b.declared.occurredAt.plusMillis(it.offsetMillis).isAfter(interruptedAt) } })
        assertTrue(batches[1].declared.occurredAt.isAfter(interruptedAt))
        r.assertConserved("split runs")
    }

    @Test fun `missing gap - a gap left open by a dead run is closed at that run's end, never inside the next run`() = runTest {
        val r = Rig(); r.open()
        val first = r.collector()
        assertEquals(TrackingBeginResult.STARTED, r.begin(first))
        r.move(1); r.move(10); first.drain()
        r.x.platform.providerEnabled = false
        assertTrue(first.tick())
        assertEquals(TrackingGapReason.PROVIDER_DISABLED, r.buffer()!!.openGap!!.reason)
        // The process dies; the next opening interrupts it but the buffer cannot be written (no close, no seal).
        r.x.platform.port.detach(); r.idle(240)
        r.failBuffer = true
        assertEquals(1, r.service.recover(r.x.account(), r.x.a.partition, { false }, "boot-7", r.x.device()))
        r.failBuffer = false
        val interruptedAt = r.view().interruptions.single().start
        assertNotNull(r.buffer()!!.openGap, "the dead run's gap is still open in the buffer")
        // The driver resumes and records again: the old gap closes at the dead run's end, not at the new fix.
        r.idle(60)
        r.x.platform.providerEnabled = true
        assertEquals(TrackingSessionState.RECORDING, r.service.start(r.x.account(), r.x.a, r.trip, r.x.device()).state)
        val second = r.collector()
        assertEquals(TrackingBeginResult.STARTED, r.begin(second))
        r.move(5); second.drain()
        val gap = r.buffer()!!.gaps.single()
        assertEquals(TrackingGapReason.PROVIDER_DISABLED, gap.reason)
        assertEquals(interruptedAt, gap.end, "closed at the interruption, never inside the new run")
        // A gap of the new run is detected again (the old one no longer blocks the watchdog).
        r.x.platform.providerEnabled = false
        assertTrue(second.tick())
        assertEquals(2, r.buffer()!!.gaps.size)
        assertTrue(r.buffer()!!.openGap!!.start.isAfter(interruptedAt))
        r.assertConserved("gap carried")
    }

    @Test fun `precision overclaim - an approximate report inside a precise period reduces first and is recorded coarse`() = runTest {
        val r = Rig(); r.open()
        val collector = r.collector()
        assertEquals(TrackingBeginResult.STARTED, r.begin(collector))
        r.move(1); collector.drain()
        // Fine location removed, coarse kept, before the watchdog's next tick: the very next report reduces the period.
        r.x.platform.permission = LocationPermission.APPROXIMATE
        r.idle(119); r.move(1); collector.drain()
        val view = r.view()
        assertEquals(TrackingCause.PRECISION_REDUCED, view.cause); assertEquals(LocationPermission.APPROXIMATE, view.precision)
        assertTrue(view.coarse)
        val reducedAt = view.since
        val pending = r.buffer()!!.pending
        assertEquals(listOf(false, true), pending.map { it.coarse }, "the precise fix stays precise, the approximate one is coarse")
        assertFalse(pending.last().at.isBefore(reducedAt), "the coarse fix lies in the approximate period")
        assertTrue(pending.first().at.isBefore(reducedAt))
        // The watchdog replaces the listener and writes no second reduction.
        assertTrue(collector.tick())
        assertEquals(LocationPermission.APPROXIMATE, r.x.platform.precision)
        assertEquals(1, r.view().records.count { it.type == TrackingOperations.REDUCE })
        // A report observed without any permission is not recorded.
        r.x.platform.permission = LocationPermission.NONE
        val recorded = r.buffer()!!.recorded
        r.idle(130); r.move(1); collector.drain()
        assertEquals(recorded, r.buffer()!!.recorded)
        assertFalse(collector.tick())
        assertEquals(TrackingCause.PERMISSION_REVOKED, r.view().cause)
        // The sealed batch carries the coarse flag exactly on the approximate fix.
        val batch = r.batches().single()
        assertEquals(listOf(0, TrackingBatchOperations.FLAG_COARSE), batch.fixes.map { it.flags })
        r.assertConserved("reduction")
        // Resumed precise later: the screen keeps saying that part of the session was approximate.
        r.x.platform.permission = LocationPermission.PRECISE
        val resumed = r.service.start(r.x.account(), r.x.a, r.trip, r.x.device())
        assertEquals(LocationPermission.PRECISE, resumed.precision); assertTrue(resumed.coarse)
    }

    @Test fun `precision overclaim - a collector that begins with only approximate permission reduces before it listens`() = runTest {
        val r = Rig(); r.open()
        r.x.platform.permission = LocationPermission.APPROXIMATE
        r.x.f.later(1)
        assertEquals(TrackingBeginResult.STARTED, r.begin(r.collector()))
        assertEquals(LocationPermission.APPROXIMATE, r.x.platform.precision)
        val view = r.view()
        assertEquals(TrackingCause.PRECISION_REDUCED, view.cause); assertEquals(LocationPermission.APPROXIMATE, view.precision)
        // A session started approximate is never shown precise; no reduction is written for it.
        val y = Rig(); y.open(LocationPermission.APPROXIMATE)
        y.x.platform.permission = LocationPermission.APPROXIMATE
        assertEquals(TrackingBeginResult.STARTED, y.begin(y.collector()))
        assertTrue(y.view().records.none { it.type == TrackingOperations.REDUCE })
        assertTrue(y.view().coarse)
    }

    @Test fun `buffer loss - across every interruption each recorded fix stays pending once or leaves in exactly one batch`() = runTest {
        for (case in listOf("driver", "permission", "notification", "owner", "reduce", "provider", "death", "reboot", "seal-failure")) {
            val r = Rig(); r.open()
            val collector = r.collector()
            assertEquals(TrackingBeginResult.STARTED, r.begin(collector), case)
            r.move(1); r.move(10); r.move(10); collector.drain()
            when (case) {
                "driver" -> assertTrue(collector.stopByDriver())
                "permission" -> { r.x.platform.permission = LocationPermission.NONE; assertFalse(collector.tick()) }
                "notification" -> { r.x.platform.notification = false; assertFalse(collector.tick()) }
                "owner" -> {
                    r.x.f.identity = EnterpriseLocalIdentity("7e000000-0000-4000-8000-000000000003", r.x.a.partition.origin, "7e000000-0000-4000-8000-000000000004")
                    assertFalse(collector.tick())
                    // Another account never reads or receives this buffer; the owner finds it intact.
                    r.x.f.identity = EnterpriseLocalIdentity(r.x.a.partition.ownerAccount, r.x.a.partition.origin, "7e000000-0000-4000-8000-000000000009")
                }
                "reduce" -> { r.x.platform.permission = LocationPermission.APPROXIMATE; r.idle(120); r.move(1); collector.drain(); assertTrue(collector.tick()) }
                "provider" -> { r.x.platform.providerEnabled = false; assertTrue(collector.tick()); r.x.platform.providerEnabled = true; r.move(10); collector.drain() }
                "death" -> { r.x.platform.port.detach(); r.idle(60); r.service.recover(r.x.account(), r.x.a.partition, { false }, "boot-7", r.x.device()) }
                "reboot" -> { r.x.platform.port.detach(); r.idle(60); r.service.recover(r.x.account(), r.x.a.partition, { false }, "boot-8", r.x.device()) }
                "seal-failure" -> { r.failBatches = true; assertTrue(collector.stopByDriver()); r.failBatches = false }
            }
            r.assertConserved(case)
            assertTrue(r.buffer()!!.recorded >= 3, case)
            if (case == "owner") assertNull(r.x.f.disk.of(r.x.f.b.partition).records[EnterpriseTrackingProducers.BUFFER_RECORD_ID], case)
            if (case == "seal-failure") {
                assertEquals(3, r.buffer()!!.pending.size, "a failed seal keeps the fixes for the next opening")
                assertEquals(1, r.service.seal(r.x.account(), r.x.a.partition, r.session, force = true))
                r.assertConserved("$case, sealed later")
            }
        }
    }
}
