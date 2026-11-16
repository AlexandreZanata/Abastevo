package com.anpfuel.application.enterprise.trip

import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.EnterpriseAccessResult
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueProducer
import com.anpfuel.application.enterprise.queue.EnterpriseQueueProducerDescriptor
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.application.enterprise.queue.EnterpriseQueueTransaction
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.trip.MissionInbox
import com.anpfuel.domain.enterprise.trip.MissionInboxPage
import com.anpfuel.domain.enterprise.trip.MissionInboxPresentation
import com.anpfuel.domain.enterprise.trip.MissionInboxView
import com.anpfuel.domain.enterprise.trip.MissionLocalException
import com.anpfuel.domain.enterprise.trip.MissionLocalFailure
import com.anpfuel.domain.enterprise.trip.MissionResponseLine
import com.anpfuel.domain.enterprise.trip.TripDeclarations
import com.anpfuel.domain.enterprise.trip.TripEnvelope
import com.anpfuel.domain.enterprise.trip.TripMissionOperations
import com.anpfuel.domain.enterprise.trip.TripMissionResponse
import com.anpfuel.domain.enterprise.trip.TripOperationException
import java.time.ZoneId

/*
 * EMP-TRIP-021 — the missions inbox (B-BR-EMP-527..531, EDR-342/343). The copy of the member's own list lives in one
 * sealed `mission-inbox` record per partition with its device receipt clock; an answer is one `trip.mission.respond`
 * operation written in the same protected write as its line; its acceptance is only ever read from its own receipt.
 */

/** One protected snapshot of the partition's inbox and the retained queue entries of its saved answers. */
class EnterpriseMissionSnapshot(val inbox: MissionInbox?, entries: Map<String, EnterpriseQueueEntry>) {
    private val values = entries.toMap()
    val entries: Map<String, EnterpriseQueueEntry> get() = values.toMap()
    override fun toString(): String = "EnterpriseMissionSnapshot(redacted)"
}

/** The protected inbox port of the creation partition. */
interface EnterpriseMissionStore {
    /** The copy bound to the attempt's exact membership and enrollment, with the entry of every saved answer, in one snapshot. */
    suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseMissionSnapshot
    /**
     * One protected read-modify-write of the copy without an operation (a server page). A local refusal
     * ([com.anpfuel.domain.enterprise.trip.MissionLocalException]) writes nothing and is rethrown as itself; an unreadable
     * or foreign copy is unavailable and never replaced.
     */
    suspend fun update(attempt: EnterpriseQueueAttempt, change: (MissionInbox) -> MissionInbox): MissionInbox
}

/** The EMP-TRIP-008 own list on the account cookie plane (EDR-342). */
fun interface EnterpriseMissionGateway {
    suspend fun mine(context: EnterpriseContextSummary): EnterpriseAccessResult<MissionInboxPage>
}

private fun inboxOf(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt): MissionInbox {
    val member = attempt.enrollment.target.membershipId
    val enrollment = attempt.enrollment.enrollmentId
    return tx.record(EnterpriseMissionProducers.INBOX_RECORD_ID)?.let { MissionInbox.decode(it.envelope).requireBinding(member, enrollment) }
        ?: MissionInbox.empty(member, enrollment)
}

/** The registered `trip.mission.respond` producer: it indexes the answer in the inbox in the same protected write (EDR-343). */
class EnterpriseMissionProducer : EnterpriseQueueProducer {
    override fun descriptor(): EnterpriseQueueProducerDescriptor = EnterpriseQueueProducerDescriptor(TripMissionOperations.RESPOND, alone = true)

    override fun save(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation) {
        require(operation.type == TripMissionOperations.RESPOND)
        val fact = TripMissionOperations.read(operation)
        // The copy's own rule inside the protected write: an assigned mission at that revision, answered once.
        val next = inboxOf(tx, attempt).withResponse(MissionResponseLine(fact.missionId, fact.operationId, fact.revision, fact.response))
        tx.put(EnterpriseLocalRecord(attempt.partition, EnterpriseMissionProducers.INBOX_RECORD_ID, next.canonicalBytes))
    }

    /** The receipt is the truth and stays in the queue entry; the copy is unchanged by it. */
    override fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        ack: OperationAcknowledgement) = Unit
    override fun toString(): String = "EnterpriseMissionProducer(redacted)"
}

object EnterpriseMissionProducers {
    /** The fixed identifier of the one mission-inbox record per partition. */
    const val INBOX_RECORD_ID = "f05e0b00-0000-4000-8000-000000000021"
    fun all(): Array<EnterpriseQueueProducer> = arrayOf(EnterpriseMissionProducer())
}

/** The own list could not be read now (offline or refused); the copy stays as it was. */
class MissionUnreachable(val error: EnterpriseAccessError) : IllegalStateException("Mission list unavailable")

/** Offline mission actions of one partition and the online refresh; every write is durable before it returns. */
class EnterpriseMissionService(
    private val source: EnterpriseQueueSource,
    private val save: suspend (EnterprisePartition, EnterpriseOperation) -> Boolean,
    private val store: EnterpriseMissionStore,
    private val gateway: EnterpriseMissionGateway,
    private val trips: EnterpriseTripStore,
    private val clock: () -> EnterpriseDeviceClock,
    private val zone: () -> ZoneId,
    private val wallMillis: () -> Long = System::currentTimeMillis,
    private val newId: () -> String = { java.util.UUID.randomUUID().toString() },
) {
    private suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt {
        account.assertCurrent()
        if (!account.identity.owns(partition)) throw EnterpriseLocalDenied()
        val attempt = source.attempt(account, partition)
        if (attempt.account !== account || attempt.partition != partition) throw EnterpriseLocalDenied()
        attempt.assertCurrent()
        return attempt
    }

    private suspend fun attempt(account: EnterpriseQueueAccountLease, context: EnterpriseContextSummary): EnterpriseQueueAttempt =
        attempt(account, context.partition).also { if (it.enrollment.target.membershipId != context.membershipId) throw EnterpriseLocalDenied() }

    private suspend fun view(attempt: EnterpriseQueueAttempt): MissionInboxView {
        val snapshot = store.read(attempt)
        attempt.assertCurrent()
        val book = trips.read(attempt).book
        attempt.assertCurrent()
        return MissionInboxPresentation.view(snapshot.inbox, snapshot.entries, book, clock())
    }

    /** The inbox as the screen may state it, from the protected copy, the answers' receipts and the trip book; no network. */
    suspend fun inbox(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): MissionInboxView = view(attempt(account, partition))

    /** Online: the own list, kept sealed with the device receipt clock (B-BR-EMP-527). */
    suspend fun refresh(account: EnterpriseQueueAccountLease, context: EnterpriseContextSummary): MissionInboxView {
        val employee = context.employeeId ?: throw TripDriverUnknown()
        val attempt = attempt(account, context)
        val page = when (val result = gateway.mine(context)) {
            is EnterpriseAccessResult.Ok -> result.value
            is EnterpriseAccessResult.Err -> throw MissionUnreachable(result.error)
        }
        attempt.assertCurrent()
        // The receipt clock is read after the answer arrived; the copy is replaced whole or not at all.
        val received = clock()
        store.update(attempt) { it.withPage(employee, page, received) }
        attempt.assertCurrent()
        return view(attempt)
    }

    /** Offline: the signed answer to an assigned mission of the copy (B-BR-EMP-528); returns its operation identifier. */
    suspend fun respond(account: EnterpriseQueueAccountLease, context: EnterpriseContextSummary, missionId: String, response: TripMissionResponse,
        reason: String?): String {
        val attempt = attempt(account, context)
        val inbox = store.read(attempt).inbox ?: throw MissionLocalException(MissionLocalFailure.UNKNOWN_MISSION)
        attempt.assertCurrent()
        val item = inbox.item(missionId) ?: throw MissionLocalException(MissionLocalFailure.UNKNOWN_MISSION)
        val operationId = newId()
        // The exact local rule first; the producer re-checks it inside the protected write.
        inbox.withResponse(MissionResponseLine(missionId, operationId, item.revision, response))
        val partition = attempt.partition
        val envelope = TripEnvelope(SyncBinding.of(partition.ownerAccount, partition.origin, partition.organizationId),
            attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId, operationId, TripDeclarations.at(wallMillis(), zone()))
        val operation = try {
            TripMissionOperations.respond(envelope, missionId, item.revision, response.name, if (response == TripMissionResponse.DECLINE) reason else null)
        } catch (_: TripOperationException) { throw TripCaptureInvalid() }
        save(partition, operation)
        return operationId
    }

    override fun toString(): String = "EnterpriseMissionService(redacted)"
}
