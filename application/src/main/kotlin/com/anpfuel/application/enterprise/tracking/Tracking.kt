package com.anpfuel.application.enterprise.tracking

import com.anpfuel.application.enterprise.EnterpriseAccessResult
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueProducer
import com.anpfuel.application.enterprise.queue.EnterpriseQueueProducerDescriptor
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.application.enterprise.queue.EnterpriseQueueTransaction
import com.anpfuel.application.enterprise.trip.EnterpriseTripProducers
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncProjection
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.sync.SyncResult
import com.anpfuel.domain.enterprise.tracking.LocalTrackingSession
import com.anpfuel.domain.enterprise.tracking.LocationPermission
import com.anpfuel.domain.enterprise.tracking.TrackingBook
import com.anpfuel.domain.enterprise.tracking.TrackingCause
import com.anpfuel.domain.enterprise.tracking.TrackingCheckpoint
import com.anpfuel.domain.enterprise.tracking.TrackingDecisions
import com.anpfuel.domain.enterprise.tracking.TrackingDevice
import com.anpfuel.domain.enterprise.tracking.TrackingEnvelope
import com.anpfuel.domain.enterprise.tracking.TrackingLocalTrip
import com.anpfuel.domain.enterprise.tracking.TrackingOperationFact
import com.anpfuel.domain.enterprise.tracking.TrackingOperations
import com.anpfuel.domain.enterprise.tracking.TrackingOrigin
import com.anpfuel.domain.enterprise.tracking.TrackingPolicy
import com.anpfuel.domain.enterprise.tracking.TrackingPolicyRead
import com.anpfuel.domain.enterprise.tracking.TrackingSessionState
import com.anpfuel.domain.enterprise.tracking.TrackingTripFacts
import com.anpfuel.domain.enterprise.trip.LocalTrip
import com.anpfuel.domain.enterprise.trip.TripBook
import com.anpfuel.domain.enterprise.trip.TripDeclarations
import com.anpfuel.domain.enterprise.trip.TripLocalState
import java.time.Instant
import java.time.ZoneId

/*
 * EMP-GPS-002 — the offline organization tracking producer (B-BR-EMP-495..500, EDR-321..324). The session operations are
 * the facts; the sealed tracking book is their local index replayed through the EMP-GPS-001 domain; acceptance is only
 * ever read from each record's own retained receipt.
 */

/** One protected snapshot: the partition's tracking book, its trip book and every queue entry they name. */
class EnterpriseTrackingSnapshot(val book: TrackingBook?, val trips: TripBook?, entries: Map<String, EnterpriseQueueEntry>) {
    private val values = entries.toMap()
    val entries: Map<String, EnterpriseQueueEntry> get() = values.toMap()
    override fun toString(): String = "EnterpriseTrackingSnapshot(redacted)"
}

/** The protected tracking read and the two writes that carry no operation (policy cache and checkpoint). */
interface EnterpriseTrackingStore {
    suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseTrackingSnapshot
    suspend fun replacePolicy(attempt: EnterpriseQueueAttempt, read: TrackingPolicyRead)
    /** False when the session is not in the book; never creates one. */
    suspend fun checkpoint(attempt: EnterpriseQueueAttempt, sessionId: String, checkpoint: TrackingCheckpoint): Boolean
}

/** B-BR-EMP-495: the driver's read of the policy in force, online only. */
fun interface EnterpriseTrackingPolicyGateway {
    suspend fun mine(context: EnterpriseContextSummary, readAt: Instant): EnterpriseAccessResult<TrackingPolicyRead>
}

/** Local refusals before queueing; never a server outcome. */
enum class TrackingLocalFailure {
    UNKNOWN_TRIP, TRIP_NOT_OPEN, POLICY_OFF, POLICY_UNKNOWN, SESSION_ENDED, SESSION_REFUSED, NOT_RECORDING, REFUSED
}

class TrackingLocalException(val failure: TrackingLocalFailure) : IllegalStateException("tracking local: ${failure.name.lowercase()}")

private fun denied(failure: TrackingLocalFailure) = TrackingLocalException(failure)

/** The device trip fact of a trip-book entry. */
internal fun localTrip(trip: LocalTrip): TrackingLocalTrip = when (trip.state) {
    TripLocalState.IN_PROGRESS -> TrackingLocalTrip.IN_PROGRESS
    TripLocalState.FINISHED -> TrackingLocalTrip.FINISHED
    TripLocalState.START_REFUSED -> TrackingLocalTrip.START_REFUSED
}

private fun trackingBookOf(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt): TrackingBook {
    val member = attempt.enrollment.target.membershipId
    val enrollment = attempt.enrollment.enrollmentId
    return tx.record(EnterpriseTrackingProducers.BOOK_RECORD_ID)?.let { TrackingBook.decode(it.envelope).requireBinding(member, enrollment) }
        ?: TrackingBook.empty(member, enrollment)
}

private fun tripBookOf(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt): TripBook? =
    tx.record(EnterpriseTripProducers.BOOK_RECORD_ID)?.let {
        TripBook.decode(it.envelope).requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
    }

/**
 * The exact device rule shared by the service (before a write) and the producer (inside it): the dependencies the
 * operation must carry and the stored session it yields (B-BR-EMP-497).
 */
internal object TrackingRules {
    fun expectedDependencies(fact: TrackingOperationFact, trip: LocalTrip, local: LocalTrackingSession?): List<String> {
        val previous = local?.lastOperationId
        return when {
            local == null -> listOf(trip.startOperationId)
            fact is TrackingOperationFact.Event && fact.cause == TrackingCause.TRIP_COMPLETED ->
                listOf(previous!!, requireNotNull(trip.finishOperationId)).sorted()
            else -> listOf(previous!!)
        }
    }

    fun apply(organizationId: String, membershipId: String, fact: TrackingOperationFact, trip: LocalTrip, book: TrackingBook): LocalTrackingSession {
        val local = book.sessionById(fact.sessionId)
        if (local == null && book.session(fact.tripId) != null) throw denied(TrackingLocalFailure.REFUSED)
        if (local != null && (local.tripId != fact.tripId || local.refused)) throw denied(TrackingLocalFailure.REFUSED)
        if (fact.dependsOn != expectedDependencies(fact, trip, local)) throw denied(TrackingLocalFailure.REFUSED)
        if (fact is TrackingOperationFact.DriverAction && fact.origin != TrackingOrigin.TRIP_SCREEN) throw denied(TrackingLocalFailure.REFUSED)
        // The end that follows a finish carries the finish's own instant (EDR-322).
        if (fact is TrackingOperationFact.Event && fact.cause == TrackingCause.TRIP_COMPLETED &&
            fact.declared.occurredAt != trip.finishedAt) throw denied(TrackingLocalFailure.REFUSED)
        val state = TrackingDecisions.tripState(localTrip(trip))
        val facts = TrackingDecisions.facts(organizationId, trip.tripId, membershipId, state)
        val existing = local?.let { TrackingDecisions.rebuild(organizationId, membershipId, it) }
        val policy = if (local == null) book.policy?.policy else local.policy
        val next = try { TrackingDecisions.decide(organizationId, membershipId, fact, facts, existing, policy) }
        catch (_: Exception) { throw denied(TrackingLocalFailure.REFUSED) }
        return TrackingDecisions.record(local, next, fact.operationId)
    }
}

/** One of the six registered `gps.session.*` producers: it indexes its operation in the book in the same protected write. */
class EnterpriseTrackingProducer(private val type: String) : EnterpriseQueueProducer {
    init { require(type in TrackingOperations.SESSION_TYPES) }
    override fun descriptor(): EnterpriseQueueProducerDescriptor = EnterpriseQueueProducerDescriptor(type)

    override fun save(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation) {
        require(operation.type == type)
        val fact = TrackingOperations.read(operation)
        val book = trackingBookOf(tx, attempt)
        val trip = tripBookOf(tx, attempt)?.trip(fact.tripId) ?: throw denied(TrackingLocalFailure.UNKNOWN_TRIP)
        val session = TrackingRules.apply(attempt.partition.organizationId, attempt.enrollment.target.membershipId, fact, trip, book)
        tx.put(EnterpriseLocalRecord(attempt.partition, EnterpriseTrackingProducers.BOOK_RECORD_ID, book.withSession(session).canonicalBytes))
    }

    /** A final REJECTED receipt stops the session on this device (B-BR-EMP-497); the receipt itself stays the truth. */
    override fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        ack: OperationAcknowledgement) {
        if (ack.result != SyncResult.REJECTED) return
        val fact = TrackingOperations.read(operation)
        val book = trackingBookOf(tx, attempt)
        val next = book.withRefusal(fact.sessionId, operation.operationId)
        if (next !== book) tx.put(EnterpriseLocalRecord(attempt.partition, EnterpriseTrackingProducers.BOOK_RECORD_ID, next.canonicalBytes))
    }
    override fun toString(): String = "EnterpriseTrackingProducer(redacted)"
}

object EnterpriseTrackingProducers {
    /** The fixed identifier of the one tracking-book record per partition. */
    const val BOOK_RECORD_ID = "f05e0b00-0000-4000-8000-0000000000a2"
    fun all(): Array<EnterpriseQueueProducer> = TrackingOperations.SESSION_TYPES.sorted().map { EnterpriseTrackingProducer(it) }.toTypedArray()
}

/** One session record with its own sync state and, when final, its receipt code. */
class TrackingRecordView(val operationId: String, val type: String, val cause: TrackingCause?, val at: Instant, val sync: EnterpriseSyncStatus,
    val code: String?) {
    override fun toString(): String = "EnterpriseTrackingRecordView(redacted)"
}

/** What the trip screen may state about one trip's session (B-BR-EMP-500); no coordinate. */
class TrackingSessionView(
    val sessionId: String,
    val tripId: String,
    val policy: TrackingPolicy,
    val state: TrackingSessionState,
    /** The cause of the last entry (an attempt's PERMISSION_DENIED or NOTIFICATION_BLOCKED keeps the state). */
    val cause: TrackingCause?,
    val since: Instant,
    val precision: LocationPermission?,
    /** A final server refusal stopped this session on the device; [refusalCode] is its receipt code. */
    val refused: Boolean,
    val refusalCode: String?,
    val checkpointAt: Instant?,
    records: List<TrackingRecordView>,
) {
    private val values = records.toList()
    val records: List<TrackingRecordView> get() = values.toList()
    override fun toString(): String = "EnterpriseTrackingSessionView(redacted)"
}

/** The partition's tracking state for the trip screen. */
class TrackingScreen(val policy: TrackingPolicyRead?, sessions: List<TrackingSessionView>) {
    private val values = sessions.associateBy { it.tripId }
    fun session(tripId: String): TrackingSessionView? = values[tripId]
    val sessions: List<TrackingSessionView> get() = values.values.toList()
    override fun toString(): String = "EnterpriseTrackingScreen(redacted)"
}

/** What a running collector may record now: the session's trip and precision (BR-GPS-001/007). */
class TrackingCollecting(val sessionId: String, val tripId: String, val precision: LocationPermission) {
    override fun toString(): String = "EnterpriseTrackingCollecting(redacted)"
}

/** Offline tracking actions of one partition; every write is durable before it returns. */
class EnterpriseTrackingService(
    private val source: EnterpriseQueueSource,
    private val save: suspend (EnterprisePartition, EnterpriseOperation) -> Boolean,
    private val store: EnterpriseTrackingStore,
    private val gateway: EnterpriseTrackingPolicyGateway,
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

    private suspend fun snapshot(attempt: EnterpriseQueueAttempt): EnterpriseTrackingSnapshot = store.read(attempt).also { attempt.assertCurrent() }

    private fun book(attempt: EnterpriseQueueAttempt, snapshot: EnterpriseTrackingSnapshot): TrackingBook =
        snapshot.book ?: TrackingBook.empty(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)

    private fun envelope(attempt: EnterpriseQueueAttempt, operationId: String, declared: TimeDeclaration): TrackingEnvelope {
        val p = attempt.partition
        return TrackingEnvelope(SyncBinding.of(p.ownerAccount, p.origin, p.organizationId), attempt.enrollment.target.membershipId,
            attempt.enrollment.enrollmentId, operationId, declared)
    }

    private fun declaration(at: Instant): TimeDeclaration = TripDeclarations.at(at.toEpochMilli(), zone())
    private fun now(): TimeDeclaration = TripDeclarations.at(wallMillis(), zone())

    /** Builds, re-checks with the producer's own rule and saves one operation; returns the stored session. */
    private suspend fun write(attempt: EnterpriseQueueAttempt, snapshot: EnterpriseTrackingSnapshot, trip: LocalTrip,
        build: (TrackingEnvelope) -> EnterpriseOperation, declared: TimeDeclaration): LocalTrackingSession {
        val operation = try { build(envelope(attempt, newId(), declared)) }
        catch (_: com.anpfuel.domain.enterprise.tracking.TrackingOperationException) { throw denied(TrackingLocalFailure.REFUSED) }
        val fact = TrackingOperations.read(operation)
        val session = TrackingRules.apply(attempt.partition.organizationId, attempt.enrollment.target.membershipId, fact, trip,
            book(attempt, snapshot))
        save(attempt.partition, operation)
        return session
    }

    private fun view(local: LocalTrackingSession, entries: Map<String, EnterpriseQueueEntry>): TrackingSessionView {
        val records = local.operationIds.map { id ->
            val entry = entries[id] ?: throw com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable()
            val fact = TrackingOperations.read(entry.operation)
            val receipt = entry.acknowledgement
            TrackingRecordView(id, fact.type, (fact as? TrackingOperationFact.Event)?.cause, fact.declared.occurredAt,
                EnterpriseSyncProjection.status(EnterpriseSyncProjection.item(entry)), receipt?.code)
        }
        val last = local.transitions.last()
        val precision = if (local.state == TrackingSessionState.RECORDING) local.transitions.last { it.permission != null }.permission else null
        val refusal = local.refusedOperationId?.let { entries[it]?.acknowledgement?.code }
        return TrackingSessionView(local.sessionId, local.tripId, local.policy, local.state, last.cause, last.at, precision, local.refused,
            refusal, local.checkpoint?.at, records)
    }

    suspend fun screen(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): TrackingScreen {
        val attempt = attempt(account, partition)
        val snapshot = snapshot(attempt)
        val book = snapshot.book ?: return TrackingScreen(null, emptyList())
        return TrackingScreen(book.policy, book.sessions.map { view(it, snapshot.entries) })
    }

    /** B-BR-EMP-495: one online read of the policy in force, kept in the book; null when it could not be read. */
    suspend fun refreshPolicy(account: EnterpriseQueueAccountLease, context: EnterpriseContextSummary): TrackingPolicyRead? {
        val attempt = attempt(account, context.partition)
        if (attempt.enrollment.target.membershipId != context.membershipId) throw EnterpriseLocalDenied()
        val read = when (val result = gateway.mine(context, Instant.ofEpochMilli(wallMillis()))) {
            is EnterpriseAccessResult.Ok -> result.value
            is EnterpriseAccessResult.Err -> return null
        }
        attempt.assertCurrent()
        store.replacePolicy(attempt, read)
        return read
    }

    private fun trip(snapshot: EnterpriseTrackingSnapshot, tripId: String): LocalTrip =
        snapshot.trips?.trip(tripId) ?: throw denied(TrackingLocalFailure.UNKNOWN_TRIP)

    /**
     * The driver's tap on the visible trip screen (B-BR-EMP-496): starts, retries or resumes the trip's session with the
     * permission and notification visibility observed now; an attempt that cannot record is recorded as such.
     */
    suspend fun start(account: EnterpriseQueueAccountLease, context: EnterpriseContextSummary, tripId: String,
        device: TrackingDevice): TrackingSessionView {
        val attempt = attempt(account, context.partition)
        if (attempt.enrollment.target.membershipId != context.membershipId) throw EnterpriseLocalDenied()
        val snapshot = snapshot(attempt)
        val trip = trip(snapshot, tripId)
        if (trip.state != TripLocalState.IN_PROGRESS) throw denied(TrackingLocalFailure.TRIP_NOT_OPEN)
        val book = book(attempt, snapshot)
        val local = book.session(tripId)
        val permission = (device.permission ?: throw denied(TrackingLocalFailure.REFUSED)).name
        if (local == null) {
            when (book.policy?.policy) {
                null -> throw denied(TrackingLocalFailure.POLICY_UNKNOWN)
                TrackingPolicy.OFF -> throw denied(TrackingLocalFailure.POLICY_OFF)
                else -> Unit
            }
        } else {
            if (local.refused) throw denied(TrackingLocalFailure.SESSION_REFUSED)
            when (local.state) {
                TrackingSessionState.ENDED -> throw denied(TrackingLocalFailure.SESSION_ENDED)
                TrackingSessionState.RECORDING -> return view(local, snapshot.entries)
                else -> Unit
            }
        }
        val sessionId = local?.sessionId ?: newId()
        val first = local == null || local.state == TrackingSessionState.NOT_STARTED
        val deps = listOf(local?.lastOperationId ?: trip.startOperationId)
        val declared = now()
        val stored = write(attempt, snapshot, trip, { envelope ->
            if (first) TrackingOperations.start(envelope, sessionId, tripId, TrackingOrigin.TRIP_SCREEN.name, permission, device.notificationVisible, deps)
            else TrackingOperations.resume(envelope, sessionId, tripId, TrackingOrigin.TRIP_SCREEN.name, permission, device.notificationVisible, deps)
        }, declared)
        return view(stored, snapshot(attempt).entries)
    }

    private suspend fun event(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, select: (TrackingBook) -> LocalTrackingSession?,
        type: String, cause: TrackingCause, at: Instant?): TrackingSessionView? {
        val attempt = attempt(account, partition)
        val snapshot = snapshot(attempt)
        val local = select(book(attempt, snapshot)) ?: return null
        if (local.refused) return null
        val trip = trip(snapshot, local.tripId)
        val declared = if (at == null) now() else declaration(maxOf(at, local.lastAt))
        val stored = write(attempt, snapshot, trip, { envelope ->
            TrackingOperations.event(type, envelope, local.sessionId, local.tripId, cause.name, listOf(local.lastOperationId))
        }, declared)
        return view(stored, snapshot(attempt).entries)
    }

    /** "Parar rastreamento": INTERRUPTED with STOPPED_BY_DRIVER while the trip continues (B-BR-EMP-499). */
    suspend fun stop(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, tripId: String): TrackingSessionView {
        val attempt = attempt(account, partition)
        val local = book(attempt, snapshot(attempt)).session(tripId) ?: throw denied(TrackingLocalFailure.NOT_RECORDING)
        if (local.state != TrackingSessionState.RECORDING) throw denied(TrackingLocalFailure.NOT_RECORDING)
        return event(account, partition, { it.session(tripId) }, TrackingOperations.INTERRUPT, TrackingCause.STOPPED_BY_DRIVER, null)
            ?: throw denied(TrackingLocalFailure.NOT_RECORDING)
    }

    /** A watchdog, owner or recovery interruption of a recording session at [at] (never before its last entry). */
    suspend fun interrupt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, sessionId: String, cause: TrackingCause,
        at: Instant? = null): TrackingSessionView? =
        event(account, partition, { it.sessionById(sessionId)?.takeIf { s -> s.state == TrackingSessionState.RECORDING } },
            TrackingOperations.INTERRUPT, cause, at)

    /** Fine location reduced to approximate while recording: the session keeps recording with coarse precision. */
    suspend fun reduce(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, sessionId: String): TrackingSessionView? =
        event(account, partition, { it.sessionById(sessionId)?.takeIf { s -> s.state == TrackingSessionState.RECORDING } },
            TrackingOperations.REDUCE, TrackingCause.PRECISION_REDUCED, null)

    /**
     * The session follows the trip facts this device knows (B-BR-EMP-499): a finished trip ends it at the finish's own
     * instant, a finally refused start ends it as cancelled. Null when there is nothing to follow.
     */
    suspend fun follow(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, tripId: String): TrackingSessionView? {
        val attempt = attempt(account, partition)
        val snapshot = snapshot(attempt)
        val local = book(attempt, snapshot).session(tripId) ?: return null
        if (local.refused || local.state == TrackingSessionState.ENDED) return null
        val trip = trip(snapshot, tripId)
        val (cause, declared, deps) = when (trip.state) {
            TripLocalState.IN_PROGRESS -> return null
            TripLocalState.FINISHED -> {
                val finish = snapshot.entries[trip.finishOperationId]?.operation ?: throw com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable()
                Triple(TrackingCause.TRIP_COMPLETED, finish.declaration, listOf(local.lastOperationId, finish.operationId))
            }
            TripLocalState.START_REFUSED -> Triple(TrackingCause.TRIP_CANCELLED, declaration(maxOf(Instant.ofEpochMilli(wallMillis()), local.lastAt)),
                listOf(local.lastOperationId))
        }
        val stored = write(attempt, snapshot, trip, { envelope ->
            TrackingOperations.event(TrackingOperations.END, envelope, local.sessionId, local.tripId, cause.name, deps)
        }, declared)
        return view(stored, snapshot(attempt).entries)
    }

    /**
     * BR-GPS-011/012 at the next opening: every recording session with no running collector in this process is
     * interrupted at its checkpoint (DEVICE_RESTARTED on another boot, else PROCESS_STOPPED); finished trips end instead.
     * Returns the number of sessions changed.
     */
    suspend fun recover(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, running: (String) -> Boolean, bootId: String): Int {
        val attempt = attempt(account, partition)
        val snapshot = snapshot(attempt)
        var changed = 0
        for (local in book(attempt, snapshot).sessions) {
            if (local.refused || local.state == TrackingSessionState.ENDED) continue
            val trip = snapshot.trips?.trip(local.tripId) ?: continue
            if (trip.state != TripLocalState.IN_PROGRESS) {
                if (follow(account, partition, local.tripId) != null) changed++
                continue
            }
            if (local.state != TrackingSessionState.RECORDING || running(local.sessionId)) continue
            val checkpoint = local.checkpoint
            val cause = if (checkpoint != null && checkpoint.bootId != bootId) TrackingCause.DEVICE_RESTARTED else TrackingCause.PROCESS_STOPPED
            if (interrupt(account, partition, local.sessionId, cause, checkpoint?.at ?: local.lastAt) != null) changed++
        }
        return changed
    }

    /** The running collector's checkpoint (BR-GPS-012); false when the session is not recording any more. */
    suspend fun checkpoint(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, sessionId: String, bootId: String): Boolean {
        val attempt = attempt(account, partition)
        val local = book(attempt, snapshot(attempt)).sessionById(sessionId)
        if (local == null || local.state != TrackingSessionState.RECORDING || local.refused) return false
        return store.checkpoint(attempt, sessionId, TrackingCheckpoint(bootId, maxOf(Instant.ofEpochMilli(wallMillis()), local.lastAt)))
    }

    /** What may be collected for a session now (BR-GPS-001/007/014), or null: only a recording, unrefused session of an open trip. */
    suspend fun collecting(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, sessionId: String): TrackingCollecting? {
        val attempt = attempt(account, partition)
        val snapshot = snapshot(attempt)
        val local = book(attempt, snapshot).sessionById(sessionId) ?: return null
        if (local.refused) return null
        val trip = snapshot.trips?.trip(local.tripId) ?: return null
        val member = attempt.enrollment.target.membershipId
        val organization = partition.organizationId
        val facts: TrackingTripFacts = TrackingDecisions.facts(organization, trip.tripId, member, TrackingDecisions.tripState(localTrip(trip)))
        val precision = TrackingDecisions.rebuild(organization, member, local).collecting(organization, facts) ?: return null
        return TrackingCollecting(local.sessionId, local.tripId, precision)
    }

    override fun toString(): String = "EnterpriseTrackingService(redacted)"
}
