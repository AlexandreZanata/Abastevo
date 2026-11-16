package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.enterprise.tracking.*
import com.anpfuel.application.enterprise.trip.*
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.sync.*
import com.anpfuel.domain.enterprise.tracking.*
import com.anpfuel.domain.enterprise.trip.TripBook
import com.anpfuel.domain.enterprise.trip.TripDeclarations
import com.anpfuel.domain.enterprise.trip.TripLocalState
import java.time.Instant
import java.time.ZoneId
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

private fun kid(n: Int) = "7e000000-0000-4000-8000-" + n.toString().padStart(12, '0')

/** EMP-GPS-002 intake in miniature: dependencies must be accepted; an organization with tracking OFF refuses the start. */
internal class TrackingServerFake(private val trips: TripServerFake) {
    val delivered = mutableListOf<String>()
    val accepted = mutableSetOf<String>()
    var policyOff = false
    private fun ack(op: EnterpriseOperation, result: String, code: String? = null, revision: Long? = null): OperationAcknowledgement {
        val final = result != "RETRY"
        return OperationAcknowledgement.parse(("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}",""" +
            """"result":"$result","code":${code?.let { "\"$it\"" } ?: "null"},"entity_revision":${revision ?: "null"},""" +
            """"received_at":${if (final) "\"2026-11-15T18:00:00Z\"" else "null"},"review_item_id":null,"warnings":[]}""").toByteArray(), op)
    }
    val batches = mutableListOf<TrackingBatchFact>()
    fun answer(op: EnterpriseOperation): OperationAcknowledgement {
        if (op.type == TrackingBatchOperations.TYPE) {
            delivered += op.operationId
            if (!op.dependsOn.all { it in accepted }) return ack(op, "RETRY", "DEPENDENCY_PENDING")
            batches += TrackingBatchOperations.read(op); accepted += op.operationId
            return ack(op, "ACCEPTED", revision = 1)
        }
        if (op.type !in TrackingOperations.SESSION_TYPES) return trips.answer(op)
        delivered += op.operationId
        if (!op.dependsOn.all { it in accepted || it in trips.accepted }) return ack(op, "RETRY", "DEPENDENCY_PENDING")
        if (op.type == TrackingOperations.START && policyOff) return ack(op, "REJECTED", "FORBIDDEN")
        accepted += op.operationId
        return ack(op, "ACCEPTED", revision = accepted.size.toLong())
    }
}

/** A platform with one listener slot; it counts every subscription and refuses a second concurrent one. */
internal class FakeTrackingPlatform {
    var permission = LocationPermission.PRECISE
    var notification = true
    var boot = "boot-7"
    var attaches = 0
    var refuse = false
    var sink: ((TrackingFixSample) -> Unit)? = null
    var precision: LocationPermission? = null
    var wall = 1_763_208_000_000L
    var elapsed = 5_000L
    var batteryLow = false
    var providerEnabled = true
    val port = object : TrackingPlatform {
        override fun permission() = permission
        override fun notificationVisible() = notification
        override fun bootId() = boot
        override fun attach(precision: LocationPermission, sink: (TrackingFixSample) -> Unit): Boolean {
            check(this@FakeTrackingPlatform.sink == null) { "a second listener was attached" }
            if (refuse) return false
            attaches++; this@FakeTrackingPlatform.sink = sink; this@FakeTrackingPlatform.precision = precision
            return true
        }
        override fun detach() { sink = null; precision = null }
        override fun wallMillis() = wall
        override fun elapsedMillis() = elapsed
        override fun batteryLow() = batteryLow
        override fun providerEnabled(precision: LocationPermission) = providerEnabled
    }
    fun fix(at: Long = 1_000) = sink?.invoke(TrackingFixSample(-15.6, -56.1, 5f, 10f, 90f, at, at, false))
}

internal class TrackingFixture {
    val f = TripFlowFixture()
    val server = TrackingServerFake(f.server)
    val zone: ZoneId = ZoneId.of("America/Cuiaba")
    private var nonce = 5000
    var policyAnswer: (Instant) -> EnterpriseAccessResult<TrackingPolicyRead> = { at ->
        EnterpriseAccessResult.Ok(TrackingPolicyRead(TrackingPolicy.EXPECTED, 2, "aviso-2026.1", Instant.parse("2026-11-01T00:00:00Z"), at))
    }
    val policyReads = mutableListOf<EnterpriseContextSummary>()
    val gateway = EnterpriseTrackingPolicyGateway { context, at -> policyReads += context; policyAnswer(at) }
    private fun records(p: EnterprisePartition) = f.disk.of(p).records
    private fun book(attempt: EnterpriseQueueAttempt): TrackingBook? =
        records(attempt.partition)[EnterpriseTrackingProducers.BOOK_RECORD_ID]?.let { TrackingBook.decode(it.envelope) }
            ?.requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
    private fun put(attempt: EnterpriseQueueAttempt, next: TrackingBook) {
        records(attempt.partition)[EnterpriseTrackingProducers.BOOK_RECORD_ID] =
            EnterpriseLocalRecord(attempt.partition, EnterpriseTrackingProducers.BOOK_RECORD_ID, next.canonicalBytes)
    }
    var storeFails = false
    var bufferWrites = 0
    val store = object : EnterpriseTrackingStore {
        override suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseTrackingSnapshot {
            attempt.assertCurrent()
            if (storeFails) throw EnterprisePersistenceUnavailable()
            val p = f.disk.of(attempt.partition)
            val trips = p.records[EnterpriseTripProducers.BOOK_RECORD_ID]?.let { TripBook.decode(it.envelope) }
            val tracking = book(attempt)
            val buffers = p.records[EnterpriseTrackingProducers.BUFFER_RECORD_ID]?.let { TrackingBufferBook.decode(it.envelope) }
            val ids = (tracking?.sessions?.flatMap { it.operationIds } ?: emptyList()) + (trips?.trips?.flatMap { it.operationIds } ?: emptyList()) +
                (buffers?.sessions?.flatMap { it.batchOperationIds } ?: emptyList())
            return EnterpriseTrackingSnapshot(tracking, trips, ids.associateWith { p.entries.getValue(it) }, buffers)
        }
        override suspend fun updateBuffer(attempt: EnterpriseQueueAttempt, sessionId: String, change: (SessionBuffer?) -> SessionBuffer?): SessionBuffer? {
            attempt.assertCurrent()
            if (book(attempt)?.sessionById(sessionId) == null) return null
            val records = records(attempt.partition)
            val current = records[EnterpriseTrackingProducers.BUFFER_RECORD_ID]?.let { TrackingBufferBook.decode(it.envelope) }
                ?: TrackingBufferBook.empty(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
            val next = change(current.session(sessionId)) ?: return current.session(sessionId)
            bufferWrites++
            records[EnterpriseTrackingProducers.BUFFER_RECORD_ID] = EnterpriseLocalRecord(attempt.partition, EnterpriseTrackingProducers.BUFFER_RECORD_ID,
                current.with(next).canonicalBytes)
            return next
        }
        override suspend fun replacePolicy(attempt: EnterpriseQueueAttempt, read: TrackingPolicyRead) {
            attempt.assertCurrent()
            put(attempt, (book(attempt) ?: TrackingBook.empty(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)).withPolicy(read))
        }
        override suspend fun checkpoint(attempt: EnterpriseQueueAttempt, sessionId: String, checkpoint: TrackingCheckpoint): Boolean {
            attempt.assertCurrent()
            val current = book(attempt) ?: return false
            if (current.sessionById(sessionId) == null) return false
            put(attempt, current.withCheckpoint(sessionId, checkpoint)); return true
        }
    }
    val registry = EnterpriseQueueRegistry(*EnterpriseTripProducers.all(f.hints), *EnterpriseTrackingProducers.all())
    val delivery = EnterpriseQueueService(f.source, f.queue, { _, operations -> EnterpriseOperationDelivery(operations.map(server::answer)) },
        registry, f.clock, { 0 })
    val trips = EnterpriseTripService(f.source, delivery::save, f.store, f.hints, f.clock, { zone }, { f.wall }, { kid(nonce++) })
    val tracking = EnterpriseTrackingService(f.source, delivery::save, store, gateway, { zone }, { f.wall }, { kid(nonce++) })
    var running: String? = null
    var boot = "boot-7"
    fun flow() = TripFlow(f.source, trips, f.fence, tracking = TripTracking(tracking, { it == running }, { boot })) { account, partition ->
        delivery.dispatch(account, partition)
    }
    var elapsed = 0L
    val gate = TrackingStartGate { elapsed }
    val platform = FakeTrackingPlatform()
    var deliveries = 0
    fun collector() = TrackingCollector(tracking, f.source, gate, platform.port) { account, partition -> deliveries++; delivery.dispatch(account, partition) }

    val a get() = f.a
    suspend fun account() = f.source.account()
    suspend fun startTrip(): String {
        trips.start(account(), a, f.choice(), f.label, f.km("184320"))
        return trips.trips(account(), a.partition).single().trip.tripId
    }
    suspend fun refresh() = tracking.refreshPolicy(account(), a)
    suspend fun session(trip: String) = tracking.screen(account(), a.partition).session(trip)
    fun entries() = f.entries()
    fun device(permission: LocationPermission = LocationPermission.PRECISE, visible: Boolean = true) = TrackingDevice(permission, visible)
    fun operation(id: String) = entries().getValue(id).operation
}

class EnterpriseTrackingServiceTest {
    @Test fun `the policy is read online with its instant and decides what may start`() = runTest {
        val x = TrackingFixture()
        val trip = x.startTrip()
        val before = x.entries().size
        val unknown = assertThrows(TrackingLocalException::class.java) { kotlinx.coroutines.runBlocking { x.tracking.start(x.account(), x.a, trip, x.device()) } }
        assertEquals(TrackingLocalFailure.POLICY_UNKNOWN, unknown.failure)
        x.policyAnswer = { at -> EnterpriseAccessResult.Ok(TrackingPolicyRead(TrackingPolicy.OFF, 0, null, null, at)) }
        assertEquals(TrackingPolicy.OFF, x.refresh()!!.policy)
        val off = assertThrows(TrackingLocalException::class.java) { kotlinx.coroutines.runBlocking { x.tracking.start(x.account(), x.a, trip, x.device()) } }
        assertEquals(TrackingLocalFailure.POLICY_OFF, off.failure)
        assertEquals(before, x.entries().size, "a refused start writes nothing")
        // A failed read keeps the cached value; the read names the selected context only.
        x.policyAnswer = { EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
        assertNull(x.refresh())
        assertEquals(TrackingPolicy.OFF, x.tracking.screen(x.account(), x.a.partition).policy!!.policy)
        assertTrue(x.policyReads.all { it == x.a })
        x.policyAnswer = { at -> EnterpriseAccessResult.Ok(TrackingPolicyRead(TrackingPolicy.OPTIONAL, 3, "aviso-2026.2", Instant.parse("2026-11-01T00:00:00Z"), at)) }
        val read = x.refresh()!!
        assertEquals(Instant.ofEpochMilli(x.f.wall), read.readAt)
        assertEquals(TrackingSessionState.RECORDING, x.tracking.start(x.account(), x.a, trip, x.device()).state)
        // A context of another membership cannot read or write through this partition.
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking {
            x.tracking.refreshPolicy(x.account(), x.a.copy(membershipId = kid(1))) } }
    }

    @Test fun `a start on the open trip writes one bound start after the trip start and nothing for a repeat`() = runTest {
        val x = TrackingFixture(); val trip = x.startTrip(); x.refresh()
        val tripStart = x.trips.trips(x.account(), x.a.partition).single().trip.startOperationId
        val view = x.tracking.start(x.account(), x.a, trip, x.device())
        assertEquals(TrackingSessionState.RECORDING, view.state)
        assertEquals(LocationPermission.PRECISE, view.precision)
        assertEquals(TrackingPolicy.EXPECTED, view.policy)
        val start = x.operation(view.records.single().operationId)
        assertEquals(TrackingOperations.START, start.type)
        assertEquals(listOf(tripStart), start.dependsOn)
        val fact = TrackingOperations.read(start) as TrackingOperationFact.DriverAction
        assertEquals(TrackingOrigin.TRIP_SCREEN, fact.origin); assertEquals(trip, fact.tripId)
        assertEquals(listOf(x.a.partition.organizationId, x.a.membershipId), listOf(start.binding.organizationId, start.membershipId))
        assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, view.records.single().sync)
        val count = x.entries().size
        assertEquals(TrackingSessionState.RECORDING, x.tracking.start(x.account(), x.a, trip, x.device()).state)
        assertEquals(count, x.entries().size, "a repeated start adds no operation")
        assertNull(x.tracking.screen(x.account(), x.f.b.partition).session(trip), "another organization reads nothing")
        val unknown = assertThrows(TrackingLocalException::class.java) { kotlinx.coroutines.runBlocking { x.tracking.start(x.account(), x.a, kid(1), x.device()) } }
        assertEquals(TrackingLocalFailure.UNKNOWN_TRIP, unknown.failure)
        assertEquals("EnterpriseTrackingSessionView(redacted)", view.toString())
    }

    @Test fun `a denied permission and a hidden notification are recorded attempts and a retry starts the same session`() = runTest {
        val x = TrackingFixture(); val trip = x.startTrip(); x.refresh()
        val denied = x.tracking.start(x.account(), x.a, trip, x.device(LocationPermission.NONE))
        assertEquals(TrackingSessionState.NOT_STARTED, denied.state); assertEquals(TrackingCause.PERMISSION_DENIED, denied.cause)
        val hidden = x.tracking.start(x.account(), x.a, trip, x.device(visible = false))
        assertEquals(TrackingCause.NOTIFICATION_BLOCKED, hidden.cause); assertEquals(denied.sessionId, hidden.sessionId)
        val started = x.tracking.start(x.account(), x.a, trip, x.device(LocationPermission.APPROXIMATE))
        assertEquals(TrackingSessionState.RECORDING, started.state); assertEquals(LocationPermission.APPROXIMATE, started.precision)
        val ops = started.records.map { x.operation(it.operationId) }
        assertEquals(listOf(TrackingOperations.START, TrackingOperations.START, TrackingOperations.START), ops.map { it.type })
        assertEquals(listOf(ops[0].operationId), ops[1].dependsOn); assertEquals(listOf(ops[1].operationId), ops[2].dependsOn)
    }

    @Test fun `stop, resume and finish are ordered operations that the server accepts in order`() = runTest {
        val x = TrackingFixture(); val trip = x.startTrip(); x.refresh()
        x.tracking.start(x.account(), x.a, trip, x.device())
        x.f.later(5)
        val stopped = x.tracking.stop(x.account(), x.a.partition, trip)
        assertEquals(TrackingSessionState.INTERRUPTED, stopped.state); assertEquals(TrackingCause.STOPPED_BY_DRIVER, stopped.cause)
        assertEquals(TrackingLocalFailure.NOT_RECORDING, assertThrows(TrackingLocalException::class.java) {
            kotlinx.coroutines.runBlocking { x.tracking.stop(x.account(), x.a.partition, trip) } }.failure)
        x.f.later(5)
        val resumed = x.tracking.start(x.account(), x.a, trip, x.device())
        assertEquals(TrackingOperations.RESUME, x.operation(resumed.records.last().operationId).type)
        x.f.later(30)
        x.trips.finish(x.account(), x.a.partition, trip, x.f.km("184610"))
        val finish = x.trips.trips(x.account(), x.a.partition).single().finish!!.fact
        val ended = x.tracking.follow(x.account(), x.a.partition, trip)!!
        assertEquals(TrackingSessionState.ENDED, ended.state); assertEquals(TrackingCause.TRIP_COMPLETED, ended.cause)
        val end = x.operation(ended.records.last().operationId)
        assertEquals(finish.declared, end.declaration, "the end carries the finish's own instant")
        assertEquals(listOf(resumed.records.last().operationId, finish.operationId).sorted(), end.dependsOn)
        assertNull(x.tracking.follow(x.account(), x.a.partition, trip), "an ended session is followed once")
        assertNull(x.tracking.collecting(x.account(), x.a.partition, ended.sessionId))
        x.delivery.dispatch(x.account(), x.a.partition)
        val delivered = x.tracking.screen(x.account(), x.a.partition).session(trip)!!
        assertTrue(delivered.records.all { it.sync == EnterpriseSyncStatus.ACCEPTED })
        assertEquals(4, x.server.accepted.size)
        assertEquals(TrackingLocalFailure.TRIP_NOT_OPEN, assertThrows(TrackingLocalException::class.java) {
            kotlinx.coroutines.runBlocking { x.tracking.start(x.account(), x.a, trip, x.device()) } }.failure)
    }

    /** EMP-TRIP-021 (B-BR-EMP-530, BR-PRIV-006): a device pause suspends and a device cancellation ends the recording session. */
    @Test fun `a trip pause suspends and a cancellation ends the session at the command's own instant`() = runTest {
        val x = TrackingFixture(); val flow = x.flow()
        val trip = x.startTrip(); x.refresh()
        flow.open(x.a, x.f.fence.epoch)
        flow.startTracking(trip, x.device(), x.f.fence.epoch, flow.state.value.generation)
        x.running = flow.state.value.tracking!!.session(trip)!!.sessionId
        x.f.later(10)
        flow.lifecycle(trip, com.anpfuel.domain.enterprise.trip.TripCommand.PAUSE, null, x.f.fence.epoch, flow.state.value.generation)
        assertEquals(TripLocalState.PAUSED, flow.state.value.trips.single().trip.state)
        val paused = flow.state.value.tracking!!.session(trip)!!
        assertEquals(TrackingSessionState.SUSPENDED, paused.state); assertEquals(TrackingCause.TRIP_PAUSED, paused.cause)
        val pause = flow.state.value.trips.single().lifecycle.single().fact
        val suspend = x.operation(paused.records.last().operationId)
        assertEquals(TrackingOperations.SUSPEND, suspend.type)
        assertEquals(pause.declared, suspend.declaration, "the suspension carries the pause's own instant")
        assertTrue(pause.operationId in suspend.dependsOn)
        // Tracking does not start while paused; after the resume the driver restarts it explicitly.
        flow.startTracking(trip, x.device(), x.f.fence.epoch, flow.state.value.generation)
        assertEquals(TrackingLocalFailure.TRIP_NOT_OPEN, flow.state.value.trackingRefusal)
        x.f.later(10)
        flow.lifecycle(trip, com.anpfuel.domain.enterprise.trip.TripCommand.RESUME, null, x.f.fence.epoch, flow.state.value.generation)
        assertEquals(TrackingSessionState.SUSPENDED, flow.state.value.tracking!!.session(trip)!!.state)
        flow.startTracking(trip, x.device(), x.f.fence.epoch, flow.state.value.generation)
        assertEquals(TrackingSessionState.RECORDING, flow.state.value.tracking!!.session(trip)!!.state)
        x.f.later(10)
        flow.lifecycle(trip, com.anpfuel.domain.enterprise.trip.TripCommand.CANCEL, "Cliente cancelou", x.f.fence.epoch, flow.state.value.generation)
        val ended = flow.state.value.tracking!!.session(trip)!!
        assertEquals(TrackingSessionState.ENDED, ended.state); assertEquals(TrackingCause.TRIP_CANCELLED, ended.cause)
        val cancel = flow.state.value.trips.single().lifecycle.last().fact
        assertEquals(cancel.declared, x.operation(ended.records.last().operationId).declaration)
        // Delivered in order, each accepted by the intake's dependency rule.
        x.delivery.dispatch(x.account(), x.a.partition)
        assertTrue(x.tracking.screen(x.account(), x.a.partition).session(trip)!!.records.all { it.sync == EnterpriseSyncStatus.ACCEPTED })
    }

    @Test fun `recovery interrupts a recording session with no running collector at its checkpoint and never resumes it`() = runTest {
        val x = TrackingFixture(); val trip = x.startTrip(); x.refresh()
        val started = x.tracking.start(x.account(), x.a, trip, x.device())
        x.f.later(1)
        assertTrue(x.tracking.checkpoint(x.account(), x.a.partition, started.sessionId, "boot-7"))
        val checkpointAt = Instant.ofEpochMilli(x.f.wall)
        x.f.later(20)
        // A running collector of this process keeps its session.
        assertEquals(0, x.tracking.recover(x.account(), x.a.partition, { it == started.sessionId }, "boot-7"))
        assertEquals(1, x.tracking.recover(x.account(), x.a.partition, { false }, "boot-7"))
        val stopped = x.session(trip)!!
        assertEquals(TrackingSessionState.INTERRUPTED, stopped.state); assertEquals(TrackingCause.PROCESS_STOPPED, stopped.cause)
        assertEquals(checkpointAt, stopped.since, "the gap starts at the last checkpoint")
        assertEquals(0, x.tracking.recover(x.account(), x.a.partition, { false }, "boot-8"), "nothing resumes by itself")
        x.tracking.start(x.account(), x.a, trip, x.device())
        x.tracking.checkpoint(x.account(), x.a.partition, started.sessionId, "boot-7")
        x.f.later(3)
        assertEquals(1, x.tracking.recover(x.account(), x.a.partition, { false }, "boot-8"))
        assertEquals(TrackingCause.DEVICE_RESTARTED, x.session(trip)!!.cause)
        // A trip finished while the process was dead ends the session at the finish instead.
        x.tracking.start(x.account(), x.a, trip, x.device())
        x.f.later(2); x.trips.finish(x.account(), x.a.partition, trip, x.f.km("184700"))
        assertEquals(1, x.tracking.recover(x.account(), x.a.partition, { false }, "boot-8"))
        assertEquals(TrackingSessionState.ENDED, x.session(trip)!!.state)
    }

    @Test fun `the producer refuses inconsistent operations inside the protected write and saves nothing`() = runTest {
        val x = TrackingFixture(); val trip = x.startTrip(); x.refresh()
        val view = x.tracking.start(x.account(), x.a, trip, x.device())
        val attempt = x.f.source.attempt(x.account(), x.a.partition)
        fun envelope(op: Int) = TrackingEnvelope(SyncBinding.of(x.a.partition.ownerAccount, x.a.partition.origin, x.a.partition.organizationId),
            x.a.membershipId, attempt.enrollment.enrollmentId, kid(op), TripDeclarations.at(x.f.wall + 60_000, x.zone))
        val previous = view.records.last().operationId
        val refused = listOf(
            // A resume of a recording session, a wrong dependency, an implicit origin, a second session and an unknown trip.
            TrackingOperations.resume(envelope(901), view.sessionId, trip, "TRIP_SCREEN", "PRECISE", true, listOf(previous)),
            TrackingOperations.event(TrackingOperations.INTERRUPT, envelope(902), view.sessionId, trip, "PROCESS_STOPPED", listOf(kid(1))),
            TrackingOperations.event(TrackingOperations.INTERRUPT, envelope(903), view.sessionId, trip, "PROCESS_STOPPED", emptyList()),
            TrackingOperations.start(envelope(904), kid(77), trip, "TRIP_SCREEN", "PRECISE", true,
                listOf(x.trips.trips(x.account(), x.a.partition).single().trip.startOperationId)),
            TrackingOperations.event(TrackingOperations.END, envelope(905), view.sessionId, trip, "TRIP_COMPLETED", listOf(previous)),
            TrackingOperations.event(TrackingOperations.INTERRUPT, envelope(906), view.sessionId, kid(2), "PROCESS_STOPPED", listOf(previous)),
        )
        val before = x.f.disk.of(x.a.partition).records.mapValues { it.value.envelope.toList() }
        for (operation in refused) {
            assertThrows(Exception::class.java) { kotlinx.coroutines.runBlocking { x.delivery.save(x.a.partition, operation) } }
            assertNull(x.entries()[operation.operationId], "nothing was queued")
        }
        assertEquals(before, x.f.disk.of(x.a.partition).records.mapValues { it.value.envelope.toList() }, "the book is unchanged")
        // An implicit origin built by hand is refused by the producer even with a correct dependency.
        val implicit = TrackingOperations.resume(envelope(907), view.sessionId, trip, "BOOT", "PRECISE", true, listOf(previous))
        x.tracking.stop(x.account(), x.a.partition, trip)
        assertThrows(Exception::class.java) { kotlinx.coroutines.runBlocking { x.delivery.save(x.a.partition, implicit) } }
    }

    @Test fun `a final refusal stops the session on the device and a failed save writes nothing`() = runTest {
        val x = TrackingFixture(); val trip = x.startTrip(); x.refresh()
        x.server.policyOff = true
        val view = x.tracking.start(x.account(), x.a, trip, x.device())
        x.delivery.dispatch(x.account(), x.a.partition)
        val refused = x.session(trip)!!
        assertTrue(refused.refused); assertEquals("FORBIDDEN", refused.refusalCode)
        assertEquals(EnterpriseSyncStatus.REJECTED, refused.records.single().sync)
        assertNull(x.tracking.collecting(x.account(), x.a.partition, view.sessionId))
        assertEquals(TrackingLocalFailure.SESSION_REFUSED, assertThrows(TrackingLocalException::class.java) {
            kotlinx.coroutines.runBlocking { x.tracking.start(x.account(), x.a, trip, x.device()) } }.failure)
        // Another trip, a failed protected write: neither the operation nor the book changes.
        val y = TrackingFixture(); val other = y.startTrip(); y.refresh()
        val before = y.entries().size
        y.f.failSave = true
        assertThrows(Exception::class.java) { kotlinx.coroutines.runBlocking { y.tracking.start(y.account(), y.a, other, y.device()) } }
        y.f.failSave = false
        assertEquals(before, y.entries().size)
        assertNull(y.session(other))
    }

    @Test fun `the trip flow recovers at opening, ends the session with the finish and states refusals`() = runTest {
        val x = TrackingFixture(); val flow = x.flow()
        val trip = x.startTrip(); x.refresh()
        flow.open(x.a, x.f.fence.epoch)
        assertEquals(TrackingPolicy.EXPECTED, flow.state.value.tracking!!.policy!!.policy)
        flow.startTracking(trip, x.device(), x.f.fence.epoch, flow.state.value.generation)
        val sessionId = flow.state.value.tracking!!.session(trip)!!.sessionId
        assertEquals(TrackingSessionState.RECORDING, flow.state.value.tracking!!.session(trip)!!.state)
        x.running = sessionId
        flow.open(x.a, x.f.fence.epoch)
        assertEquals(TrackingSessionState.RECORDING, flow.state.value.tracking!!.session(trip)!!.state, "a running collector keeps recording")
        x.running = null
        flow.open(x.a, x.f.fence.epoch)
        assertEquals(TrackingCause.PROCESS_STOPPED, flow.state.value.tracking!!.session(trip)!!.cause)
        flow.startTracking(trip, x.device(), x.f.fence.epoch, flow.state.value.generation)
        flow.stopTracking(trip, x.f.fence.epoch, flow.state.value.generation)
        assertEquals(TrackingCause.STOPPED_BY_DRIVER, flow.state.value.tracking!!.session(trip)!!.cause)
        flow.stopTracking(trip, x.f.fence.epoch, flow.state.value.generation)
        assertEquals(TrackingLocalFailure.NOT_RECORDING, flow.state.value.trackingRefusal)
        x.f.later(10)
        flow.finish(trip, x.f.km("184400"), x.f.fence.epoch, flow.state.value.generation)
        assertEquals(TripLocalState.FINISHED, flow.state.value.trips.single().trip.state)
        assertEquals(TrackingSessionState.ENDED, flow.state.value.tracking!!.session(trip)!!.state)
        // A tracking read failure never hides the trip list.
        x.storeFails = true
        flow.open(x.a, x.f.fence.epoch)
        assertTrue(flow.state.value.loaded); assertEquals(1, flow.state.value.trips.size); assertNull(flow.state.value.tracking)
    }

    @Test fun `the collector starts only from a fresh grant for its session and holds one listener`() = runTest {
        val x = TrackingFixture(); val trip = x.startTrip(); x.refresh()
        val session = x.tracking.start(x.account(), x.a, trip, x.device()).sessionId
        val collector = x.collector()
        assertEquals(TrackingBeginResult.NO_GRANT, collector.begin(x.a.partition, session), "no visible screen armed it")
        x.gate.arm(x.a.partition, kid(9))
        assertEquals(TrackingBeginResult.NO_GRANT, collector.begin(x.a.partition, session), "a grant for another session")
        x.gate.arm(x.a.partition, session); x.elapsed += TrackingStartGate.TTL_MILLIS + 1
        assertEquals(TrackingBeginResult.NO_GRANT, collector.begin(x.a.partition, session), "an expired grant")
        x.gate.arm(x.f.b.partition, session)
        assertEquals(TrackingBeginResult.NO_GRANT, collector.begin(x.a.partition, session), "a grant of another organization")
        assertEquals(0, x.platform.attaches)
        x.gate.arm(x.a.partition, session)
        assertEquals(TrackingBeginResult.STARTED, collector.begin(x.a.partition, session))
        assertEquals(session, collector.runningSessionId)
        assertNotNull(x.session(trip)!!.checkpointAt, "the checkpoint is written before collection")
        x.gate.arm(x.a.partition, session)
        assertEquals(TrackingBeginResult.ALREADY_RUNNING, collector.begin(x.a.partition, session))
        assertEquals(1, x.platform.attaches, "a repeated start adds no listener")
        x.platform.fix(); x.platform.fix()
        assertEquals(2L, collector.state.value.accepted)
        val sink = x.platform.sink!!
        collector.release()
        sink(TrackingFixSample(-15.6, -56.1, 5f, 10f, 90f, 1, 1, false))
        assertEquals(2L, collector.state.value.accepted, "nothing is collected after the stop")
        assertNull(x.platform.sink); assertNull(collector.runningSessionId)
        assertFalse(collector.state.value.active)
        assertEquals("TrackingRuntimeState(redacted)", collector.state.value.toString())
    }

    @Test fun `the collector refuses a session that is not recording and missing permission or notification`() = runTest {
        val x = TrackingFixture(); val trip = x.startTrip(); x.refresh()
        val session = x.tracking.start(x.account(), x.a, trip, x.device()).sessionId
        x.tracking.stop(x.account(), x.a.partition, trip)
        val collector = x.collector()
        x.gate.arm(x.a.partition, session)
        assertEquals(TrackingBeginResult.NOT_RECORDING, collector.begin(x.a.partition, session))
        x.tracking.start(x.account(), x.a, trip, x.device())
        x.platform.permission = LocationPermission.NONE; x.gate.arm(x.a.partition, session)
        assertEquals(TrackingBeginResult.PERMISSION_MISSING, collector.begin(x.a.partition, session))
        // EMP-GPS-010 (B-BR-EMP-544): the granted session that cannot be collected is interrupted with the observed cause,
        // never left shown as recording; each case below resumes it first.
        assertEquals(TrackingCause.PERMISSION_REVOKED, x.session(trip)!!.cause)
        x.platform.permission = LocationPermission.PRECISE; x.tracking.start(x.account(), x.a, trip, x.device())
        x.platform.notification = false; x.gate.arm(x.a.partition, session)
        assertEquals(TrackingBeginResult.NOTIFICATION_HIDDEN, collector.begin(x.a.partition, session))
        assertEquals(TrackingCause.NOTIFICATION_BLOCKED, x.session(trip)!!.cause)
        x.platform.notification = true; x.tracking.start(x.account(), x.a, trip, x.device())
        x.platform.refuse = true; x.gate.arm(x.a.partition, session)
        assertEquals(TrackingBeginResult.PLATFORM_REFUSED, collector.begin(x.a.partition, session))
        assertEquals(TrackingCause.PROCESS_STOPPED, x.session(trip)!!.cause)
        assertNull(collector.runningSessionId); assertEquals(0, x.platform.attaches)
    }

    @Test fun `the watchdog records the cause each failed check requires and releases the listener`() = runTest {
        for (case in listOf("permission", "notification", "driver", "owner", "finish")) {
            val x = TrackingFixture(); val trip = x.startTrip(); x.refresh()
            val session = x.tracking.start(x.account(), x.a, trip, x.device()).sessionId
            val collector = x.collector()
            x.gate.arm(x.a.partition, session)
            assertEquals(TrackingBeginResult.STARTED, collector.begin(x.a.partition, session))
            assertTrue(collector.tick(), "a healthy tick keeps collecting")
            x.f.later(1)
            when (case) {
                "permission" -> { x.platform.permission = LocationPermission.NONE; assertFalse(collector.tick()) }
                "notification" -> { x.platform.notification = false; assertFalse(collector.tick()) }
                "driver" -> assertTrue(collector.stopByDriver())
                "owner" -> { x.f.identity = EnterpriseLocalIdentity(kid(3), x.a.partition.origin, kid(4)); assertFalse(collector.tick()) }
                "finish" -> { x.trips.finish(x.account(), x.a.partition, trip, x.f.km("184400")); assertFalse(collector.tick()) }
            }
            assertNull(x.platform.sink, case); assertNull(collector.runningSessionId, case)
            if (case == "owner") {
                // Another account never writes into this partition; the original owner finds the session at its next opening.
                x.f.identity = EnterpriseLocalIdentity(x.a.partition.ownerAccount, x.a.partition.origin, kid(9))
            }
            val view = x.session(trip)!!
            val expected = mapOf("permission" to TrackingCause.PERMISSION_REVOKED, "notification" to TrackingCause.NOTIFICATION_BLOCKED,
                "driver" to TrackingCause.STOPPED_BY_DRIVER, "owner" to null, "finish" to TrackingCause.TRIP_COMPLETED)
            if (case == "owner") assertEquals(TrackingSessionState.RECORDING, view.state, "nothing was written by another owner")
            else assertEquals(expected[case], view.cause, case)
        }
    }

    @Test fun `a reduced precision keeps recording with one approximate listener`() = runTest {
        val x = TrackingFixture(); val trip = x.startTrip(); x.refresh()
        val session = x.tracking.start(x.account(), x.a, trip, x.device()).sessionId
        val collector = x.collector()
        x.gate.arm(x.a.partition, session)
        collector.begin(x.a.partition, session)
        x.f.later(1)
        x.platform.permission = LocationPermission.APPROXIMATE
        assertTrue(collector.tick())
        assertEquals(LocationPermission.APPROXIMATE, x.platform.precision)
        assertEquals(2, x.platform.attaches)
        val view = x.session(trip)!!
        assertEquals(TrackingSessionState.RECORDING, view.state); assertEquals(TrackingCause.PRECISION_REDUCED, view.cause)
        assertEquals(LocationPermission.APPROXIMATE, view.precision)
        assertTrue(collector.tick(), "the reduction is recorded once")
        assertEquals(2, x.session(trip)!!.records.size)
    }
}
