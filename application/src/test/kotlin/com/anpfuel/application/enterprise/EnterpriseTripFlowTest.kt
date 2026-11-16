package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.enterprise.trip.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.fleet.AssetKind
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import com.anpfuel.domain.enterprise.fleet.ReferenceMeter
import com.anpfuel.domain.enterprise.fleet.ReferenceMeterKind
import com.anpfuel.domain.enterprise.fleet.UsageBasis
import com.anpfuel.domain.enterprise.meter.MeterEntryForm
import com.anpfuel.domain.enterprise.meter.MeterKind
import com.anpfuel.domain.enterprise.meter.ReadingFlag
import com.anpfuel.domain.enterprise.sync.*
import com.anpfuel.domain.enterprise.trip.*
import java.time.Instant
import java.time.ZoneId
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

private fun tid(n: Int) = "7d000000-0000-4000-8000-" + n.toString().padStart(12, '0')
private const val TRIP_ORIGIN = "https://trips.example.invalid"

/** Durable per-partition queue bytes and records, like one protected generation per account/origin/organization. */
internal class TripDisk {
    class Partition { val entries = linkedMapOf<String, EnterpriseQueueEntry>(); val records = linkedMapOf<String, EnterpriseLocalRecord>() }
    val partitions = mutableMapOf<EnterprisePartition, Partition>()
    fun of(p: EnterprisePartition) = partitions.getOrPut(p) { Partition() }
}

/** EMP-TRIP-002/003/004 intake semantics in miniature: one active trip per asset, children wait for an accepted parent. */
internal class TripServerFake {
    val delivered = mutableListOf<String>()
    val accepted = mutableSetOf<String>()
    val active = mutableMapOf<String, String>()
    var unavailable = false
    fun busy(asset: String) { active[asset] = tid(9999) }
    private fun ack(op: EnterpriseOperation, result: String, code: String? = null, revision: Long? = null): OperationAcknowledgement {
        val final = result != "RETRY"
        return OperationAcknowledgement.parse(("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}",""" +
            """"result":"$result","code":${code?.let { "\"$it\"" } ?: "null"},"entity_revision":${revision ?: "null"},""" +
            """"received_at":${if (final) "\"2026-11-15T18:00:00Z\"" else "null"},"review_item_id":${if (result == "ACCEPTED_FOR_REVIEW") "\"${tid(8888)}\"" else "null"},"warnings":[]}""").toByteArray(), op)
    }
    fun answer(op: EnterpriseOperation): OperationAcknowledgement {
        delivered += op.operationId
        val fact = TripOperations.read(op)
        val parentReady = op.dependsOn.all { it in accepted }
        return when (fact) {
            is TripOperationFact.Start -> {
                val holder = active[fact.assetId]
                if (holder != null && holder != fact.tripId) ack(op, "ACCEPTED_FOR_REVIEW", "ASSET_BUSY")
                else { active[fact.assetId] = fact.tripId; accepted += op.operationId; ack(op, "ACCEPTED", revision = 1) }
            }
            is TripOperationFact.Event -> if (!parentReady) ack(op, "RETRY", "DEPENDENCY_PENDING") else { accepted += op.operationId; ack(op, "ACCEPTED", revision = 1) }
            is TripOperationFact.Finish -> if (!parentReady) ack(op, "RETRY", "DEPENDENCY_PENDING") else {
                active.values.remove(fact.tripId); accepted += op.operationId; ack(op, "ACCEPTED", revision = 2)
            }
            is TripOperationFact.Submit -> if (!parentReady) ack(op, "RETRY", "DEPENDENCY_PENDING") else { accepted += op.operationId; ack(op, "ACCEPTED", revision = 1) }
        }
    }
}

internal class TripFlowFixture(val disk: TripDisk = TripDisk(), val server: TripServerFake = TripServerFake()) {
    val fence = EnterpriseSessionFence()
    val a = EnterpriseContextSummary(EnterprisePartition.of(tid(1), TRIP_ORIGIN, tid(2)), tid(3), "Synthetic A", tid(5))
    val b = EnterpriseContextSummary(EnterprisePartition.of(tid(1), TRIP_ORIGIN, tid(20)), tid(30), "Synthetic B", tid(50))
    var identity = EnterpriseLocalIdentity(tid(1), TRIP_ORIGIN, tid(9))
    var wall = 1_763_208_000_000L
    var elapsed = 5_000L
    var failSave = false
    private var nonce = 1000
    private fun membership(p: EnterprisePartition) = if (p == a.partition) a.membershipId else b.membershipId
    val source = object : EnterpriseQueueSource {
        override suspend fun account(): EnterpriseQueueAccountLease {
            val captured = identity; val epoch = fence.epoch; val background = fence.backgroundEpoch
            return EnterpriseQueueAccountLease.capture(captured, epoch, background, tid(nonce++)) {
                identity == captured && fence.epoch == epoch && fence.backgroundEpoch == background
            }
        }
        override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt {
            account.assertCurrent()
            return account.forEnrollment(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(partition, membership(partition)), tid(4), 1))
        }
    }
    private fun tx(p: EnterprisePartition, values: MutableMap<String, EnterpriseLocalRecord>) = object : EnterpriseQueueTransaction {
        override fun record(id: String) = values[id]
        override fun put(record: EnterpriseLocalRecord) { require(record.partition == p); values[record.id] = record }
    }
    val queue = object : EnterpriseOperationQueue {
        override suspend fun save(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, project: (EnterpriseQueueTransaction) -> Unit): Boolean {
            attempt.assertCurrent()
            val p = disk.of(attempt.partition)
            p.entries[operation.operationId]?.let { require(it.operation.sameIntent(operation)); return false }
            val changed = p.records.toMutableMap(); project(tx(attempt.partition, changed))
            if (failSave) throw EnterprisePersistenceUnavailable()
            p.entries[operation.operationId] = EnterpriseQueueEntry(operation); p.records.clear(); p.records.putAll(changed); return true
        }
        override suspend fun pending(attempt: EnterpriseQueueAttempt) = disk.of(attempt.partition).entries.values.filter { it.acknowledgement == null }
        override suspend fun entry(attempt: EnterpriseQueueAttempt, operationId: String) = disk.of(attempt.partition).entries[operationId]
        override suspend fun acknowledge(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, ack: OperationAcknowledgement,
            project: (EnterpriseQueueTransaction) -> Unit): Boolean {
            val p = disk.of(attempt.partition)
            val existing = p.entries.getValue(operation.operationId)
            existing.acknowledgement?.let { return false }
            val changed = p.records.toMutableMap(); project(tx(attempt.partition, changed))
            p.entries[operation.operationId] = EnterpriseQueueEntry(operation, existing.retry, ack); p.records.clear(); p.records.putAll(changed); return true
        }
        override suspend fun retry(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, expected: QueueRetryState, next: QueueRetryState): Boolean {
            val p = disk.of(attempt.partition); val old = p.entries.getValue(operation.operationId)
            if (old.acknowledgement != null || !old.retry.sameAs(expected)) return false
            p.entries[operation.operationId] = EnterpriseQueueEntry(old.operation, next); return true
        }
        override suspend fun stats(attempt: EnterpriseQueueAttempt) = EnterpriseQueueStats(0, 0, 0, 0, 2048)
    }
    val store = EnterpriseTripStore { attempt ->
        attempt.assertCurrent()
        val p = disk.of(attempt.partition)
        val book = p.records[EnterpriseTripProducers.BOOK_RECORD_ID]?.let { TripBook.decode(it.envelope) }
            ?.requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
        EnterpriseTripSnapshot(book, book?.trips?.flatMap { it.operationIds }?.associateWith { p.entries.getValue(it) } ?: emptyMap())
    }
    val hints = EnterpriseTripStartHints()
    val clock = { EnterpriseDeviceClock("boot-7", elapsed, wall) }
    val queueService = EnterpriseQueueService(source, queue, { _, operations ->
        if (server.unavailable) throw EnterpriseQueueDeliveryFailure()
        EnterpriseOperationDelivery(operations.map(server::answer))
    }, EnterpriseQueueRegistry(*EnterpriseTripProducers.all(hints)), clock, { 0 })
    val service = EnterpriseTripService(source, queueService::save, store, hints, clock, { ZoneId.of("America/Cuiaba") }, { wall }, { tid(nonce++) })
    fun flow() = TripFlow(source, service, fence) { account, partition -> queueService.dispatch(account, partition) }

    val meter = ReferenceMeter(tid(60), ReferenceMeterKind.ODOMETER, null, tid(61), Instant.parse("2026-11-01T00:00:00Z"))
    fun choice(asset: Int = 70, org: EnterpriseContextSummary = a) = EnterpriseAssetChoice(org.partition.organizationId, tid(asset), 3,
        AssetKind.VEHICLE, UsageBasis.DISTANCE, listOf(meter), Instant.parse("2026-11-15T11:00:00Z"), false)
    val label = TripAssetLabel("Caminhão sintético", "FROTA-12", "ABC1D23")
    val meterChoice = TripMeterChoice(tid(60), tid(61), MeterKind.ODOMETER, null)
    fun km(value: String) = TripCapture.Reading(meterChoice, MeterEntryForm.KM, value)
    fun later(minutes: Long) { wall += minutes * 60_000; elapsed += minutes * 60_000 }
    fun entries(p: EnterprisePartition = a.partition) = disk.of(p).entries
}

class EnterpriseTripFlowTest {
    private suspend fun TripFlowFixture.recordWholeTrip(flow: TripFlow) {
        flow.open(a, fence.epoch)
        flow.start(choice(), label, km("184320"), fence.epoch, flow.state.value.generation)
        val trip = flow.state.value.trips.single().trip.tripId
        later(30); flow.log(trip, TripLogbookType.STOP, null, fence.epoch, flow.state.value.generation)
        later(5); flow.log(trip, TripLogbookType.NOTE, "Entrega conferida", fence.epoch, flow.state.value.generation)
        later(60); flow.finish(trip, km("184610"), fence.epoch, flow.state.value.generation)
        later(1); flow.markReady(trip, fence.epoch, flow.state.value.generation)
    }

    @Test fun `UC-OFFL-01 offline start, events, finish and ready to send are durable and only saved on the device`() = runTest {
        val f = TripFlowFixture(); val flow = f.flow()
        f.recordWholeTrip(flow)
        val state = flow.state.value
        assertTrue(state.loaded); assertFalse(state.saveFailed); assertNull(state.refusal); assertNull(state.error)
        val view = state.trips.single()
        assertEquals(TripLocalState.FINISHED, view.trip.state)
        assertEquals(TripReviewLine.READY_TO_SEND, view.review)
        val records = listOf(view.start, view.finish!!, view.submission!!) + view.events
        assertEquals(5, records.size)
        assertTrue(records.all { it.sync == EnterpriseSyncStatus.SAVED_ON_DEVICE }, "nothing is accepted before a receipt")
        assertEquals(listOf(TripLogbookType.STOP, TripLogbookType.NOTE), view.events.map { (it.fact as TripOperationFact.Event).type })
        val start = view.start.fact as TripOperationFact.Start
        assertEquals(f.a.employeeId, start.driverEmployeeId)
        assertEquals(184_320_000L, (start.evidence as TripEvidence.Reading).value.units)
        assertEquals(TripRunClock("boot-7", 5_000), start.clock)
        assertEquals("America/Cuiaba", start.declared.entryZone); assertEquals(-14_400, start.declared.deviceOffsetSeconds)
        assertEquals(TripRunClock("boot-7", 5_000 + 95 * 60_000), (view.finish!!.fact as TripOperationFact.Finish).clock)
        assertEquals(f.label, view.trip.label)
        assertTrue(f.server.delivered.isEmpty(), "nothing was sent while offline")
        // Every operation is sealed to partition A with its membership and enrollment (BR-OFFL-003).
        assertTrue(f.entries().values.all { it.operation.binding.organizationId == f.a.partition.organizationId &&
            it.operation.membershipId == f.a.membershipId && it.operation.enrollmentId == tid(4) })
        assertEquals(state.lastSaved, view.submission!!.fact.operationId)
    }

    @Test fun `acceptance follows server receipts in dependency order and each record is delivered once`() = runTest {
        val f = TripFlowFixture(); val flow = f.flow()
        f.recordWholeTrip(flow)
        flow.sendNow(f.fence.epoch, flow.state.value.generation)
        val view = flow.state.value.trips.single()
        assertTrue((listOf(view.start, view.finish!!, view.submission!!) + view.events).all { it.sync == EnterpriseSyncStatus.ACCEPTED })
        assertEquals(TripReviewLine.AWAITING_APPROVAL, view.review)
        assertEquals(5, f.server.delivered.size); assertEquals(5, f.server.delivered.distinct().size)
        flow.sendNow(f.fence.epoch, flow.state.value.generation)
        assertEquals(5, f.server.delivered.size, "an accepted record is never sent again")
    }

    @Test fun `a held start shows needs review with its code while its children wait`() = runTest {
        val f = TripFlowFixture(); val flow = f.flow()
        f.recordWholeTrip(flow)
        f.server.busy(tid(70))
        flow.sendNow(f.fence.epoch, flow.state.value.generation)
        val view = flow.state.value.trips.single()
        assertEquals(EnterpriseSyncStatus.NEEDS_REVIEW, view.start.sync); assertEquals("ASSET_BUSY", view.start.code)
        assertTrue(view.waitingForStartReview)
        assertTrue((view.events + listOf(view.finish!!, view.submission!!)).all { it.sync == EnterpriseSyncStatus.SAVED_ON_DEVICE })
        assertEquals(TripReviewLine.READY_TO_SEND, view.review, "the submission is never shown as received early")
        assertNull(flow.state.value.error)
    }

    @Test fun `B-BR-EMP-377 a second unfinished trip on the same asset is refused on the device and nothing is queued`() = runTest {
        val f = TripFlowFixture(); val flow = f.flow()
        flow.open(f.a, f.fence.epoch)
        flow.start(f.choice(), f.label, f.km("1000"), f.fence.epoch, flow.state.value.generation)
        val before = f.entries().size
        flow.start(f.choice(), f.label, f.km("1001"), f.fence.epoch, flow.state.value.generation)
        assertEquals(TripLocalFailure.ASSET_HAS_UNFINISHED_TRIP, flow.state.value.refusal)
        assertEquals(before, f.entries().size); assertEquals(1, flow.state.value.trips.size)
        // Another asset is independent; a clock that went back refuses the event before the start.
        flow.start(f.choice(71), f.label, TripCapture.Absent("Painel apagado"), f.fence.epoch, flow.state.value.generation)
        assertNull(flow.state.value.refusal); assertEquals(2, flow.state.value.trips.size)
        val trip = flow.state.value.trips.first { it.trip.assetId == tid(70) }.trip.tripId
        f.wall -= 3_600_000
        flow.log(trip, TripLogbookType.STOP, null, f.fence.epoch, flow.state.value.generation)
        assertEquals(TripLocalFailure.BEFORE_START, flow.state.value.refusal)
        flow.markReady(trip, f.fence.epoch, flow.state.value.generation)
        assertEquals(TripLocalFailure.NOT_FINISHED, flow.state.value.refusal)
        assertEquals(before + 1, f.entries().size)
    }

    @Test fun `a finally refused start frees the asset on this device`() = runTest {
        val f = TripFlowFixture(); val flow = f.flow()
        flow.open(f.a, f.fence.epoch)
        flow.start(f.choice(), f.label, f.km("1000"), f.fence.epoch, flow.state.value.generation)
        val start = f.entries().values.single()
        val refusal = OperationAcknowledgement.parse(("""{"schema_version":1,"operation_id":"${start.operation.operationId}","payload_digest":"${start.operation.payloadDigest}",""" +
            """"result":"REJECTED","code":"MALFORMED","entity_revision":null,"received_at":"2026-11-15T18:00:00Z","review_item_id":null,"warnings":[]}""").toByteArray(), start.operation)
        val queueService = EnterpriseQueueService(f.source, f.queue, { _, ops -> EnterpriseOperationDelivery(ops.map { refusal }) },
            EnterpriseQueueRegistry(*EnterpriseTripProducers.all(f.hints)), f.clock, { 0 })
        queueService.dispatch(f.a.partition)
        flow.reload(f.fence.epoch, flow.state.value.generation)
        val refused = flow.state.value.trips.single()
        assertEquals(EnterpriseSyncStatus.REJECTED, refused.start.sync); assertEquals("MALFORMED", refused.start.code)
        assertEquals(TripLocalState.START_REFUSED, refused.trip.state)
        flow.start(f.choice(), f.label, f.km("1000"), f.fence.epoch, flow.state.value.generation)
        assertNull(flow.state.value.refusal); assertEquals(2, flow.state.value.trips.size)
    }

    @Test fun `B-BR-EMP-376 a failed protected write reports that nothing was saved`() = runTest {
        val f = TripFlowFixture(); val flow = f.flow()
        flow.open(f.a, f.fence.epoch)
        f.failSave = true
        flow.start(f.choice(), f.label, f.km("1000"), f.fence.epoch, flow.state.value.generation)
        assertTrue(flow.state.value.saveFailed); assertNull(flow.state.value.lastSaved)
        assertTrue(flow.state.value.trips.isEmpty()); assertTrue(f.entries().isEmpty())
        assertTrue(f.disk.of(f.a.partition).records.isEmpty())
        f.failSave = false
        flow.start(f.choice(), f.label, f.km("1000"), f.fence.epoch, flow.state.value.generation)
        assertFalse(flow.state.value.saveFailed); assertEquals(1, flow.state.value.trips.size)
    }

    @Test fun `BR-OFFL-003 a context switch never retags queued work and B sees nothing of A`() = runTest {
        val f = TripFlowFixture(); val flow = f.flow()
        f.recordWholeTrip(flow)
        val sealed = f.entries().values.map { it.operation.canonicalBytes.toList() }
        flow.open(f.b, f.fence.epoch)
        assertTrue(flow.state.value.loaded); assertTrue(flow.state.value.trips.isEmpty())
        assertEquals(f.b, flow.state.value.context)
        flow.sendNow(f.fence.epoch, flow.state.value.generation)
        assertTrue(f.server.delivered.isEmpty(), "B's delivery sends nothing of A")
        assertEquals(sealed, f.entries().values.map { it.operation.canonicalBytes.toList() })
        assertTrue(f.disk.of(f.b.partition).entries.isEmpty())
        // A's work is still delivered to A, after the switch, by the background dispatcher.
        f.queueService.dispatch(f.a.partition)
        assertEquals(5, f.server.delivered.size)
        assertTrue(f.entries().values.all { it.acknowledgement?.result == SyncResult.ACCEPTED && it.operation.binding.organizationId == f.a.partition.organizationId })
    }

    @Test fun `after a process restart the same partition shows the same timeline`() = runTest {
        val first = TripFlowFixture(); first.recordWholeTrip(first.flow())
        val restarted = TripFlowFixture(first.disk, first.server)
        val flow = restarted.flow()
        flow.open(restarted.a, restarted.fence.epoch)
        val view = flow.state.value.trips.single()
        assertEquals(5, view.trip.operationIds.size); assertEquals(restarted.label, view.trip.label)
        assertTrue((listOf(view.start, view.finish!!, view.submission!!) + view.events).all { it.sync == EnterpriseSyncStatus.SAVED_ON_DEVICE })
        flow.sendNow(restarted.fence.epoch, flow.state.value.generation)
        assertEquals(TripReviewLine.AWAITING_APPROVAL, flow.state.value.trips.single().review)
    }

    @Test fun `EMP-TRIP-019 a start names its departure run as its one checklist-run reference`() = runTest {
        val f = TripFlowFixture(); val flow = f.flow()
        flow.open(f.a, f.fence.epoch)
        flow.start(f.choice(), f.label, f.km("1000"), f.fence.epoch, flow.state.value.generation, "not-a-run")
        assertTrue(flow.state.value.captureInvalid, "an invalid run identifier saves nothing"); assertTrue(f.entries().isEmpty())
        flow.start(f.choice(), f.label, f.km("184320"), f.fence.epoch, flow.state.value.generation, tid(90))
        val start = flow.state.value.trips.single().start.fact as TripOperationFact.Start
        assertEquals(tid(90), start.departureRunId)
        val queued = f.entries().getValue(start.operationId).operation
        assertEquals(listOf("checklist_run" to tid(90)), queued.softReferences.map { it.type to it.id })
        val trip = flow.state.value.trips.single().trip.tripId
        f.later(60); flow.finish(trip, f.km("184610"), f.fence.epoch, flow.state.value.generation)
        val finish = flow.state.value.trips.single().finish!!.fact as TripOperationFact.Finish
        assertNull(finish.returnRunId)
        assertTrue(f.entries().getValue(finish.operationId).operation.softReferences.isEmpty())
    }

    @Test fun `a missing driver identifier or an invalid entry saves nothing`() = runTest {
        val f = TripFlowFixture(); val flow = f.flow()
        flow.open(f.a.copy(employeeId = null), f.fence.epoch)
        flow.start(f.choice(), f.label, f.km("1000"), f.fence.epoch, flow.state.value.generation)
        assertTrue(flow.state.value.driverUnknown); assertTrue(f.entries().isEmpty())
        flow.open(f.a, f.fence.epoch)
        flow.start(f.choice(), f.label, f.km("12,5"), f.fence.epoch, flow.state.value.generation)
        assertTrue(flow.state.value.captureInvalid); assertTrue(f.entries().isEmpty())
        flow.start(f.choice(), f.label, TripCapture.Absent("  "), f.fence.epoch, flow.state.value.generation)
        assertTrue(flow.state.value.captureInvalid); assertTrue(f.entries().isEmpty())
        flow.start(f.choice(org = f.b), f.label, f.km("10"), f.fence.epoch, flow.state.value.generation)
        assertEquals(EnterpriseAccessError.STALE, flow.state.value.error, "a choice of another organization is never used")
        assertTrue(f.entries().isEmpty() && f.disk.of(f.b.partition).entries.isEmpty())
    }

    @Test fun `METER-003 warnings on the final reading never block the finish`() = runTest {
        val f = TripFlowFixture(); val flow = f.flow()
        flow.open(f.a, f.fence.epoch)
        flow.start(f.choice(), f.label, f.km("184320"), f.fence.epoch, flow.state.value.generation)
        val trip = flow.state.value.trips.single().trip.tripId
        f.later(60)
        assertEquals(listOf(ReadingFlag.READING_BELOW_PREVIOUS), flow.finishWarnings(trip, f.km("184000")))
        assertEquals(listOf(ReadingFlag.IMPLAUSIBLE_RATE), flow.finishWarnings(trip, f.km("185000")))
        assertEquals(emptyList<ReadingFlag>(), flow.finishWarnings(trip, f.km("184400")))
        assertEquals(emptyList<ReadingFlag>(), flow.finishWarnings(trip, TripCapture.Absent("Painel apagado")))
        flow.finish(trip, f.km("184000"), f.fence.epoch, flow.state.value.generation)
        assertEquals(TripLocalState.FINISHED, flow.state.value.trips.single().trip.state)
    }

    @Test fun `an account or epoch change withdraws the trips and old gestures do nothing`() = runTest {
        val f = TripFlowFixture(); val flow = f.flow()
        flow.open(f.a, f.fence.epoch)
        flow.start(f.choice(), f.label, f.km("1000"), f.fence.epoch, flow.state.value.generation)
        val oldEpoch = f.fence.epoch; val oldGeneration = flow.state.value.generation
        f.fence.lock()
        flow.invalidateIfStale()
        assertTrue(flow.state.value.trips.isEmpty()); assertEquals(EnterpriseAccessError.STALE, flow.state.value.error)
        flow.start(f.choice(71), f.label, f.km("1"), oldEpoch, oldGeneration)
        assertEquals(1, f.entries().size, "a stale gesture saved nothing")
    }

    @Test fun `the producers index only their own trip operations and project only a refused start`() {
        val f = TripFlowFixture()
        val producers = EnterpriseTripProducers.all(f.hints)
        assertEquals(TripOperations.TYPES, producers.map { it.descriptor().type }.toSet())
        assertTrue(producers.all { it.descriptor().schema == 1L })
    }
}
