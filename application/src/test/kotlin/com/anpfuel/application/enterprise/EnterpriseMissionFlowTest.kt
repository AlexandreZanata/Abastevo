package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.enterprise.trip.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.sync.*
import com.anpfuel.domain.enterprise.trip.*
import java.time.Instant
import java.time.ZoneId
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

private fun mid(n: Int) = "7d000000-0000-4000-8000-" + n.toString().padStart(12, '0')

/**
 * EMP-TRIP-008 answers in miniature (EDR-229): decided on current facts at receipt; another member, a stale revision or a
 * mission not stored here is a whole-request transport denial that stores nothing (EDR-343).
 */
internal class MissionServerFake {
    class Mission(var state: TripMissionState, var revision: Long, var assignee: String?)
    val missions = mutableMapOf<String, Mission>()
    val batches = mutableListOf<List<String>>()
    val answered = mutableListOf<String>()
    fun denies(op: EnterpriseOperation, member: String): Boolean {
        val fact = TripMissionOperations.read(op)
        val mission = missions[fact.missionId] ?: return true
        return mission.assignee != member || mission.state != TripMissionState.ASSIGNED || mission.revision != fact.revision
    }
    fun answer(op: EnterpriseOperation): OperationAcknowledgement {
        val fact = TripMissionOperations.read(op)
        val mission = missions.getValue(fact.missionId)
        mission.state = if (fact.response == TripMissionResponse.ACCEPT) TripMissionState.ACCEPTED else TripMissionState.DECLINED
        mission.revision += 1
        answered += op.operationId
        return OperationAcknowledgement.parse(("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}",""" +
            """"result":"ACCEPTED","code":null,"entity_revision":${mission.revision},"received_at":"2026-11-15T18:00:00Z","review_item_id":null,""" +
            """"warnings":[]}""").toByteArray(), op)
    }
}

internal class MissionFlowFixture {
    val trips = TripFlowFixture()
    val server = MissionServerFake()
    val a get() = trips.a
    val b get() = trips.b
    var page: EnterpriseAccessResult<MissionInboxPage>? = null
    val gatewayCalls = mutableListOf<EnterpriseContextSummary>()
    val gateway = EnterpriseMissionGateway { context -> gatewayCalls += context; page ?: EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
    val missionStore = object : EnterpriseMissionStore {
        private fun bound(attempt: EnterpriseQueueAttempt, raw: EnterpriseLocalRecord?) = raw?.let { MissionInbox.decode(it.envelope) }
            ?.requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
        override suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseMissionSnapshot {
            attempt.assertCurrent()
            val p = trips.disk.of(attempt.partition)
            val inbox = bound(attempt, p.records[EnterpriseMissionProducers.INBOX_RECORD_ID])
            return EnterpriseMissionSnapshot(inbox, inbox?.responses?.associate { it.operationId to p.entries.getValue(it.operationId) } ?: emptyMap())
        }
        override suspend fun update(attempt: EnterpriseQueueAttempt, change: (MissionInbox) -> MissionInbox): MissionInbox {
            attempt.assertCurrent()
            val p = trips.disk.of(attempt.partition)
            val current = bound(attempt, p.records[EnterpriseMissionProducers.INBOX_RECORD_ID])
                ?: MissionInbox.empty(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
            val next = change(current)
            p.records[EnterpriseMissionProducers.INBOX_RECORD_ID] = EnterpriseLocalRecord(attempt.partition, EnterpriseMissionProducers.INBOX_RECORD_ID,
                next.canonicalBytes)
            return next
        }
    }
    val hints = EnterpriseTripStartHints()
    val clock = { EnterpriseDeviceClock("boot-7", trips.elapsed, trips.wall) }
    /** The intake: a batch with a denied answer is refused whole (a transport denial), like the real endpoint. */
    val queueService = EnterpriseQueueService(trips.source, trips.queue, { attempt, operations ->
        server.batches += operations.map { it.operationId }
        if (trips.server.unavailable) throw EnterpriseQueueDeliveryFailure()
        val member = attempt.enrollment.target.membershipId
        if (operations.any { it.type == TripMissionOperations.RESPOND && server.denies(it, member) }) throw EnterpriseQueueDeliveryFailure()
        EnterpriseOperationDelivery(operations.map { if (it.type == TripMissionOperations.RESPOND) server.answer(it) else trips.server.answer(it) })
    }, EnterpriseQueueRegistry(*EnterpriseTripProducers.all(hints), *EnterpriseMissionProducers.all()), clock, { 0 })
    val tripService = EnterpriseTripService(trips.source, queueService::save, trips.store, hints, clock, { ZoneId.of("America/Cuiaba") },
        { trips.wall }, { mid(nonce++) }, missionStore)
    val missionService = EnterpriseMissionService(trips.source, queueService::save, missionStore, gateway, trips.store, clock,
        { ZoneId.of("America/Cuiaba") }, { trips.wall }, { mid(nonce++) })
    private var nonce = 5000
    fun flow() = TripFlow(trips.source, tripService, trips.fence, missions = missionService) { account, partition -> queueService.dispatch(account, partition) }

    fun item(mission: Int, state: TripMissionState = TripMissionState.ASSIGNED, revision: Long = 3, asset: Int? = 70, employee: String = a.employeeId!!,
        answer: TripMissionResponse? = null) = MissionInboxItem(mid(mission), mid(81), asset?.let(::mid), "Entrega no cliente Norte",
        Instant.parse("2026-11-20T11:00:00Z"), Instant.parse("2026-11-20T15:00:00Z"), state, revision, employee, answer, null,
        MissionPlanView(2, "Cliente Norte, doca 3", null, "Entrega programada", null, TripMissionLoad.NONE, null, null,
            Instant.parse("2026-11-15T10:10:00Z")), Instant.parse("2026-11-15T10:00:00Z"), Instant.parse("2026-11-15T10:30:00Z"))
    /** The server and the next page both hold [items]. */
    fun serve(vararg items: MissionInboxItem) {
        for (item in items) server.missions[item.missionId] = MissionServerFake.Mission(item.state, item.revision, a.membershipId)
        page = EnterpriseAccessResult.Ok(MissionInboxPage(items.toList(), false))
    }
    fun entries(p: EnterprisePartition = a.partition) = trips.disk.of(p).entries
    fun inbox(p: EnterprisePartition = a.partition) = trips.disk.of(p).records[EnterpriseMissionProducers.INBOX_RECORD_ID]?.let { MissionInbox.decode(it.envelope) }
}

class EnterpriseMissionFlowTest {
    private suspend fun opened(f: MissionFlowFixture, context: EnterpriseContextSummary = f.a): TripFlow =
        f.flow().also { it.open(context, f.trips.fence.epoch) }
    private val TripFlow.g: Long get() = state.value.generation
    private fun TripFlow.mission(id: Int) = state.value.missions!!.missions.first { it.missionId == mid(id) }

    @Test fun `B-BR-EMP-527 refresh is online, keeps the copy sealed with its age and a failure changes nothing`() = runTest {
        val f = MissionFlowFixture()
        val flow = opened(f)
        val never = flow.state.value.missions!!
        assertFalse(never.refreshed); assertTrue(never.stale)
        // Offline: the refresh reaches no answer and says so; nothing is cached or claimed.
        flow.refreshMissions(f.trips.fence.epoch, flow.g)
        assertEquals(EnterpriseAccessError.UNAVAILABLE, flow.state.value.refreshFailed)
        assertFalse(flow.state.value.refreshed)
        assertNull(f.inbox())
        f.serve(f.item(80), f.item(81, TripMissionState.ACCEPTED, 4, answer = TripMissionResponse.ACCEPT))
        flow.refreshMissions(f.trips.fence.epoch, flow.g)
        assertTrue(flow.state.value.refreshed)
        assertEquals(listOf(mid(80), mid(81)), f.inbox()!!.items.map { it.missionId })
        assertEquals(0L, flow.state.value.missions!!.ageMillis)
        assertEquals(listOf(f.a), f.gatewayCalls.takeLast(1))
        // Two hours later, offline: the age grows from the device receipt clock and the copy stays.
        f.trips.later(120)
        f.page = EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
        flow.refreshMissions(f.trips.fence.epoch, flow.g)
        assertEquals(EnterpriseAccessError.UNAVAILABLE, flow.state.value.refreshFailed)
        assertEquals(7_200_000L, flow.state.value.missions!!.ageMillis)
        assertEquals(2, f.inbox()!!.items.size)
        // A day later the copy is labelled as possibly out of date, never shown unlabelled.
        f.trips.later(24 * 60)
        flow.reload(f.trips.fence.epoch, flow.g)
        assertTrue(flow.state.value.missions!!.stale)
    }

    @Test fun `a page with another assignee's mission is refused whole and nothing of it is kept`() = runTest {
        val f = MissionFlowFixture()
        val flow = opened(f)
        f.page = EnterpriseAccessResult.Ok(MissionInboxPage(listOf(f.item(80), f.item(81, employee = mid(99))), false))
        flow.refreshMissions(f.trips.fence.epoch, flow.g)
        assertEquals(MissionLocalFailure.FOREIGN, flow.state.value.missionRefusal)
        assertNull(f.inbox())
        // Without the member's own employee record nothing is read.
        val unknown = f.flow().also { it.open(f.a.copy(employeeId = null), f.trips.fence.epoch) }
        val calls = f.gatewayCalls.size
        unknown.refreshMissions(f.trips.fence.epoch, unknown.g)
        assertTrue(unknown.state.value.driverUnknown)
        assertEquals(calls, f.gatewayCalls.size)
    }

    @Test fun `B-BR-EMP-528 an offline answer is saved durably, shown saved and accepted only with its own receipt`() = runTest {
        val f = MissionFlowFixture()
        f.serve(f.item(80), f.item(82))
        val flow = opened(f)
        flow.refreshMissions(f.trips.fence.epoch, flow.g)
        f.trips.server.unavailable = true
        flow.respond(mid(80), TripMissionResponse.ACCEPT, null, f.trips.fence.epoch, flow.g)
        val saved = flow.mission(80)
        assertEquals(MissionAnswerState.SAVED, saved.answer)
        assertEquals(TripMissionState.ASSIGNED, saved.state)
        assertEquals(MissionLocalFailure.NOT_CONFIRMED, saved.startBlocked)
        assertTrue(saved.actions.isEmpty())
        val op = f.entries().values.single().operation
        assertEquals(TripMissionOperations.RESPOND, op.type)
        assertEquals(listOf(f.a.membershipId, f.a.partition.organizationId), listOf(op.membershipId, op.binding.organizationId))
        assertEquals(3L, TripMissionOperations.read(op).revision)
        // A second answer for the same revision and an answer for a mission outside the copy are refused on the device.
        flow.respond(mid(80), TripMissionResponse.DECLINE, "Folga", f.trips.fence.epoch, flow.g)
        assertEquals(MissionLocalFailure.ALREADY_ANSWERED, flow.state.value.missionRefusal)
        flow.respond(mid(89), TripMissionResponse.ACCEPT, null, f.trips.fence.epoch, flow.g)
        assertEquals(MissionLocalFailure.UNKNOWN_MISSION, flow.state.value.missionRefusal)
        // A decline needs a valid reason; nothing is saved without one.
        flow.respond(mid(82), TripMissionResponse.DECLINE, "  ", f.trips.fence.epoch, flow.g)
        assertTrue(flow.state.value.captureInvalid)
        assertEquals(1, f.entries().size)
        // A failed protected write says nothing was saved and shows no answer.
        f.trips.failSave = true
        flow.respond(mid(82), TripMissionResponse.ACCEPT, null, f.trips.fence.epoch, flow.g)
        assertTrue(flow.state.value.saveFailed)
        assertEquals(MissionAnswerState.NONE, flow.mission(82).answer)
        f.trips.failSave = false
        // Back in coverage: the answer's own ACCEPTED receipt, nothing else, makes it accepted.
        f.trips.server.unavailable = false
        flow.sendNow(f.trips.fence.epoch, flow.g)
        val accepted = flow.mission(80)
        assertEquals(MissionAnswerState.ACCEPTED, accepted.answer)
        assertEquals(TripMissionState.ACCEPTED, accepted.state)
        assertEquals(setOf(MissionAction.START_TRIP), accepted.actions)
    }

    @Test fun `BR-OFFL-003 an answer queued in one organization is never shown, moved or retagged by another`() = runTest {
        val f = MissionFlowFixture()
        f.serve(f.item(80))
        val flow = opened(f)
        flow.refreshMissions(f.trips.fence.epoch, flow.g)
        f.trips.server.unavailable = true
        flow.respond(mid(80), TripMissionResponse.ACCEPT, null, f.trips.fence.epoch, flow.g)
        val bytes = f.entries().values.single().operation.canonicalBytes
        val other = opened(f, f.b)
        assertFalse(other.state.value.missions!!.refreshed)
        assertTrue(other.state.value.missions!!.missions.isEmpty())
        // B's own answer attempt cannot name A's mission.
        other.respond(mid(80), TripMissionResponse.ACCEPT, null, f.trips.fence.epoch, other.g)
        assertEquals(MissionLocalFailure.UNKNOWN_MISSION, other.state.value.missionRefusal)
        assertTrue(f.entries(f.b.partition).isEmpty())
        assertArrayEquals(bytes, f.entries().values.single().operation.canonicalBytes)
        // Delivery of A's partition sends A's answer to A only.
        f.trips.server.unavailable = false
        f.queueService.dispatch(f.a.partition)
        assertEquals(1, f.server.answered.size)
        assertEquals(f.a.partition.organizationId, f.entries().values.single().operation.binding.organizationId)
    }

    @Test fun `EDR-343 an answer goes alone after every other record, so its denial holds nothing else`() = runTest {
        val f = MissionFlowFixture()
        f.serve(f.item(80), f.item(81, TripMissionState.ACCEPTED, 4, answer = TripMissionResponse.ACCEPT))
        val flow = opened(f)
        flow.refreshMissions(f.trips.fence.epoch, flow.g)
        f.trips.server.unavailable = true
        flow.respond(mid(80), TripMissionResponse.ACCEPT, null, f.trips.fence.epoch, flow.g)
        flow.start(f.trips.choice(71), f.trips.label, f.trips.km("184320"), f.trips.fence.epoch, flow.g)
        val trip = flow.state.value.trips.single().trip.tripId
        flow.log(trip, TripLogbookType.STOP, null, f.trips.fence.epoch, flow.g)
        // Meanwhile the planner reassigned mission 80 to another member.
        f.server.missions.getValue(mid(80)).assignee = mid(77)
        f.trips.server.unavailable = false
        f.server.batches.clear()
        f.queueService.dispatch(f.a.partition)
        f.queueService.dispatch(f.a.partition)
        val answer = f.entries().values.first { it.operation.type == TripMissionOperations.RESPOND }
        // Every trip record was delivered without the answer; the answer went alone and was denied.
        assertTrue(f.server.batches.first().none { it == answer.operation.operationId })
        assertTrue(f.server.batches.filter { answer.operation.operationId in it }.all { it.size == 1 })
        assertTrue(f.entries().values.filter { it.operation.type != TripMissionOperations.RESPOND }.all { it.acknowledgement != null })
        assertNull(answer.acknowledgement)
        assertTrue(answer.retry.attempts > 0)
        // After a refresh the mission is gone from the member's list: the answer is shown not confirmed.
        f.page = EnterpriseAccessResult.Ok(MissionInboxPage(listOf(f.item(81, TripMissionState.ACCEPTED, 4, answer = TripMissionResponse.ACCEPT)), false))
        flow.refreshMissions(f.trips.fence.epoch, flow.g)
        assertEquals(MissionAnswerState.NOT_CONFIRMED_REMOVED, flow.mission(80).answer)
    }

    @Test fun `B-BR-EMP-529 a planned trip starts only from a confirmed acceptance, on the mission's vehicle, once`() = runTest {
        val f = MissionFlowFixture()
        f.serve(f.item(80), f.item(81, TripMissionState.ACCEPTED, 4, asset = 70, answer = TripMissionResponse.ACCEPT))
        val flow = opened(f)
        flow.refreshMissions(f.trips.fence.epoch, flow.g)
        f.trips.server.unavailable = true
        // Saved but unconfirmed acceptance: no planned start, nothing queued for it.
        flow.respond(mid(80), TripMissionResponse.ACCEPT, null, f.trips.fence.epoch, flow.g)
        flow.startMission(mid(80), f.trips.choice(70), f.trips.label, f.trips.km("184320"), f.trips.fence.epoch, flow.g)
        assertEquals(MissionLocalFailure.NOT_CONFIRMED, flow.state.value.missionRefusal)
        assertTrue(flow.state.value.trips.isEmpty())
        // A confirmed mission starts only on its own vehicle.
        flow.startMission(mid(81), f.trips.choice(72), f.trips.label, f.trips.km("184320"), f.trips.fence.epoch, flow.g)
        assertEquals(MissionLocalFailure.WRONG_VEHICLE, flow.state.value.missionRefusal)
        flow.startMission(mid(81), f.trips.choice(70), f.trips.label, f.trips.km("184320"), f.trips.fence.epoch, flow.g)
        val trip = flow.state.value.trips.single()
        assertEquals(mid(81), trip.missionId)
        assertEquals(mid(81), trip.trip.missionId)
        val start = f.entries().getValue(trip.trip.startOperationId).operation
        assertEquals(listOf(TripOperations.MISSION_REFERENCE to mid(81)), start.softReferences.map { it.type to it.id })
        assertEquals(setOf(MissionAction.OPEN_TRIP), flow.mission(81).actions)
        // Never twice from this device, even on another vehicle once the first trip is finished.
        flow.finish(trip.trip.tripId, f.trips.km("184400"), f.trips.fence.epoch, flow.g)
        flow.startMission(mid(81), f.trips.choice(70), f.trips.label, f.trips.km("184500"), f.trips.fence.epoch, flow.g)
        assertEquals(MissionLocalFailure.ALREADY_STARTED, flow.state.value.missionRefusal)
        assertEquals(1, flow.state.value.trips.size)
    }

    @Test fun `B-BR-EMP-530 pause, resume and cancel are signed commands at the local revision and a cancel frees the vehicle`() = runTest {
        val f = MissionFlowFixture()
        val flow = opened(f)
        f.trips.server.unavailable = true
        flow.start(f.trips.choice(70), f.trips.label, f.trips.km("184320"), f.trips.fence.epoch, flow.g)
        val trip = flow.state.value.trips.single().trip.tripId
        f.trips.later(30)
        flow.lifecycle(trip, TripCommand.PAUSE, null, f.trips.fence.epoch, flow.g)
        assertEquals(TripLocalState.PAUSED, flow.state.value.trips.single().trip.state)
        flow.lifecycle(trip, TripCommand.PAUSE, null, f.trips.fence.epoch, flow.g)
        assertEquals(TripLocalFailure.NOT_RUNNING, flow.state.value.refusal)
        f.trips.later(10)
        flow.lifecycle(trip, TripCommand.RESUME, "Retomada após almoço", f.trips.fence.epoch, flow.g)
        // A cancellation needs a reason; nothing is saved without one.
        flow.lifecycle(trip, TripCommand.CANCEL, null, f.trips.fence.epoch, flow.g)
        assertTrue(flow.state.value.captureInvalid)
        flow.lifecycle(trip, TripCommand.CANCEL, "Cliente cancelou a entrega", f.trips.fence.epoch, flow.g)
        val view = flow.state.value.trips.single()
        assertEquals(TripLocalState.CANCELLED, view.trip.state)
        assertEquals(listOf(1L, 2L, 3L), view.lifecycle.map { (it.fact as TripOperationFact.Lifecycle).revision })
        assertEquals(listOf(TripCommand.PAUSE, TripCommand.RESUME, TripCommand.CANCEL), view.lifecycle.map { (it.fact as TripOperationFact.Lifecycle).command })
        assertTrue(view.lifecycle.all { it.sync == EnterpriseSyncStatus.SAVED_ON_DEVICE })
        assertTrue(view.lifecycle.all { it.fact.dependsOn == listOf(view.trip.startOperationId) })
        // The cancelled trip no longer holds its vehicle on this device; delivery accepts each record with its own receipt.
        flow.start(f.trips.choice(70), f.trips.label, f.trips.km("184330"), f.trips.fence.epoch, flow.g)
        assertEquals(2, flow.state.value.trips.size)
        f.trips.server.unavailable = false
        flow.sendNow(f.trips.fence.epoch, flow.g)
        val delivered = flow.state.value.trips.first { it.trip.tripId == trip }
        assertTrue(delivered.lifecycle.all { it.sync == EnterpriseSyncStatus.ACCEPTED })
    }
}
