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
import com.anpfuel.domain.enterprise.tracking.SessionBuffer
import com.anpfuel.domain.enterprise.tracking.TrackingBatchOperations
import com.anpfuel.domain.enterprise.tracking.TrackingBatching
import com.anpfuel.domain.enterprise.tracking.TrackingBufferBook
import com.anpfuel.domain.enterprise.tracking.TrackingGap
import com.anpfuel.domain.enterprise.tracking.TrackingInterruption
import com.anpfuel.domain.enterprise.tracking.TrackingInterruptions
import com.anpfuel.domain.enterprise.tracking.TrackingObservation
import com.anpfuel.domain.enterprise.tracking.TrackingObservationOutcome
import com.anpfuel.domain.enterprise.tracking.TrackingOperationException
import com.anpfuel.domain.enterprise.tracking.TrackingSampler
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
class EnterpriseTrackingSnapshot(val book: TrackingBook?, val trips: TripBook?, entries: Map<String, EnterpriseQueueEntry>,
    /** EMP-GPS-003: the partition's sealed buffer record (B-BR-EMP-503). */
    val buffers: TrackingBufferBook? = null) {
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
    /**
     * EMP-GPS-003: one locked read-modify-write of a session buffer in the partition's sealed buffer record; [change] gets
     * the current buffer (null when absent) and returns the next one, or null to write nothing. Returns what is stored.
     */
    suspend fun updateBuffer(attempt: EnterpriseQueueAttempt, sessionId: String, change: (SessionBuffer?) -> SessionBuffer?): SessionBuffer?
}

/** B-BR-EMP-495: the driver's read of the policy in force, online only. */
fun interface EnterpriseTrackingPolicyGateway {
    suspend fun mine(context: EnterpriseContextSummary, readAt: Instant): EnterpriseAccessResult<TrackingPolicyRead>
}

/** Local refusals before queueing; never a server outcome. */
enum class TrackingLocalFailure {
    UNKNOWN_TRIP, TRIP_NOT_OPEN, POLICY_OFF, POLICY_UNKNOWN, SESSION_ENDED, SESSION_REFUSED, NOT_RECORDING, REFUSED,
    /** EMP-GPS-010 (B-BR-EMP-547): another session is being collected on this phone; one listener per device. */
    BUSY,
}

/** EMP-GPS-010 (B-BR-EMP-544): the location permission and tracking-notification visibility the platform reports now. */
fun interface TrackingDeviceObserver {
    fun observe(): TrackingDevice
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
    /**
     * The start or resume operation that opened the recording period of a fix at [at] (a reduction keeps its period). A fix
     * that a race left at or after the period's end still names that opening: the server counts it outside the session
     * (BR-GPS-022) and nothing waits on it forever.
     */
    fun periodOpening(session: LocalTrackingSession, at: Instant): String? =
        session.transitions.lastOrNull { !it.at.isAfter(at) && it.state == TrackingSessionState.RECORDING && it.cause == null }?.operationId

    /** EMP-GPS-010 (B-BR-EMP-545): the start or resume that opened the session's current recording run, if any. */
    fun runStart(session: LocalTrackingSession): Instant? =
        session.transitions.lastOrNull { it.state == TrackingSessionState.RECORDING && it.cause == null }?.at

    /** The end of the run before the one that opened at [runStart]: the interruption that the resume closed. */
    fun previousRunEnd(session: LocalTrackingSession, runStart: Instant): Instant? =
        TrackingInterruptions.of(session.transitions).lastOrNull { it.end == runStart }?.start

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

/**
 * EMP-GPS-003 (B-BR-EMP-504, EDR-326): the `gps.track.batch` producer. Inside the protected write it proves that the batch
 * is exactly the buffer's oldest continuous run with the next sequence and its counters, that it depends on the session
 * operation that opened that run's recording period, and removes those fixes in the same write.
 */
class EnterpriseTrackingBatchProducer : EnterpriseQueueProducer {
    override fun descriptor(): EnterpriseQueueProducerDescriptor = EnterpriseQueueProducerDescriptor(TrackingBatchOperations.TYPE)

    override fun save(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation) {
        require(operation.type == TrackingBatchOperations.TYPE)
        val fact = TrackingBatchOperations.read(operation)
        val session = trackingBookOf(tx, attempt).sessionById(fact.sessionId) ?: throw denied(TrackingLocalFailure.REFUSED)
        if (session.refused || session.tripId != fact.tripId) throw denied(TrackingLocalFailure.REFUSED)
        val buffers = bufferBookOf(tx, attempt)
        val buffer = buffers.session(fact.sessionId) ?: throw denied(TrackingLocalFailure.REFUSED)
        if (fact.dependsOn != listOfNotNull(TrackingRules.periodOpening(session, fact.declared.occurredAt))) throw denied(TrackingLocalFailure.REFUSED)
        val next = try { TrackingBatching.sealed(buffer, fact) } catch (_: IllegalArgumentException) { throw denied(TrackingLocalFailure.REFUSED) }
        tx.put(EnterpriseLocalRecord(attempt.partition, EnterpriseTrackingProducers.BUFFER_RECORD_ID, buffers.with(next).canonicalBytes))
    }

    /** The batch's receipt is the truth; the buffer no longer holds its fixes. */
    override fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        ack: OperationAcknowledgement) = Unit
    override fun toString(): String = "EnterpriseTrackingBatchProducer(redacted)"
}

private fun bufferBookOf(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt): TrackingBufferBook {
    val member = attempt.enrollment.target.membershipId
    val enrollment = attempt.enrollment.enrollmentId
    return tx.record(EnterpriseTrackingProducers.BUFFER_RECORD_ID)?.let { TrackingBufferBook.decode(it.envelope).requireBinding(member, enrollment) }
        ?: TrackingBufferBook.empty(member, enrollment)
}

object EnterpriseTrackingProducers {
    /** The fixed identifier of the one tracking-book record per partition. */
    const val BOOK_RECORD_ID = "f05e0b00-0000-4000-8000-0000000000a2"
    /** EMP-GPS-003: the fixed identifier of the one tracking-buffer record per partition. */
    const val BUFFER_RECORD_ID = "f05e0b00-0000-4000-8000-0000000000a3"
    fun all(): Array<EnterpriseQueueProducer> = (TrackingOperations.SESSION_TYPES.sorted().map { EnterpriseTrackingProducer(it) } +
        EnterpriseTrackingBatchProducer()).toTypedArray()
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
    /** EMP-GPS-003: fixes recorded and waiting to be sealed, the batches with their own sync state, the gaps. */
    val pendingFixes: Int = 0,
    batches: List<TrackingRecordView> = emptyList(),
    gaps: List<TrackingGap> = emptyList(),
    val overflow: Long = 0,
    /** EMP-GPS-010 (B-BR-EMP-544): every interval the session stopped recording, with its cause, start and end. */
    interruptions: List<TrackingInterruption> = emptyList(),
    /** EMP-GPS-010 (B-BR-EMP-546): any recording period was approximate; no GPS distance comes from those parts. */
    val coarse: Boolean = false,
) {
    private val values = records.toList()
    private val batchValues = batches.toList()
    private val gapValues = gaps.toList()
    private val interruptionValues = interruptions.toList()
    val records: List<TrackingRecordView> get() = values.toList()
    val batches: List<TrackingRecordView> get() = batchValues.toList()
    val gaps: List<TrackingGap> get() = gapValues.toList()
    val interruptions: List<TrackingInterruption> get() = interruptionValues.toList()
    val openGap: TrackingGap? get() = gapValues.lastOrNull()?.takeIf { it.end == null }
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

    private fun view(local: LocalTrackingSession, entries: Map<String, EnterpriseQueueEntry>, buffer: SessionBuffer? = null): TrackingSessionView {
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
        val batches = buffer?.batchOperationIds.orEmpty().mapNotNull { id ->
            val entry = entries[id] ?: return@mapNotNull null
            TrackingRecordView(id, TrackingBatchOperations.TYPE, null, entry.operation.declaration.occurredAt,
                EnterpriseSyncProjection.status(EnterpriseSyncProjection.item(entry)), entry.acknowledgement?.code)
        }
        // EMP-GPS-010: the interruptions and any approximate period stay visible for the whole session (B-BR-EMP-544/546).
        val coarse = local.transitions.any { it.state == TrackingSessionState.RECORDING && it.permission == LocationPermission.APPROXIMATE }
        return TrackingSessionView(local.sessionId, local.tripId, local.policy, local.state, last.cause, last.at, precision, local.refused,
            refusal, listOfNotNull(local.checkpoint?.at, buffer?.checkpointAt).maxOrNull(), records, buffer?.pending?.size ?: 0, batches,
            buffer?.gaps.orEmpty(), buffer?.overflow ?: 0, TrackingInterruptions.of(local.transitions), coarse)
    }

    suspend fun screen(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): TrackingScreen {
        val attempt = attempt(account, partition)
        val snapshot = snapshot(attempt)
        val book = snapshot.book ?: return TrackingScreen(null, emptyList())
        return TrackingScreen(book.policy, book.sessions.map { view(it, snapshot.entries, snapshot.buffers?.session(it.sessionId)) })
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
        device: TrackingDevice,
        /** EMP-GPS-010: the session this phone is collecting for now, if any (B-BR-EMP-547). */
        collecting: String? = null): TrackingSessionView {
        val attempt = attempt(account, context.partition)
        if (attempt.enrollment.target.membershipId != context.membershipId) throw EnterpriseLocalDenied()
        val snapshot = snapshot(attempt)
        val trip = trip(snapshot, tripId)
        if (trip.state != TripLocalState.IN_PROGRESS) throw denied(TrackingLocalFailure.TRIP_NOT_OPEN)
        val book = book(attempt, snapshot)
        val local = book.session(tripId)
        val permission = (device.permission ?: throw denied(TrackingLocalFailure.REFUSED)).name
        if (local != null) {
            if (local.refused) throw denied(TrackingLocalFailure.SESSION_REFUSED)
            if (local.state == TrackingSessionState.ENDED) throw denied(TrackingLocalFailure.SESSION_ENDED)
        }
        // EMP-GPS-010 (B-BR-EMP-547): every tap re-checks the latest policy read on this device, also for an existing session.
        when (book.policy?.policy) {
            null -> throw denied(TrackingLocalFailure.POLICY_UNKNOWN)
            TrackingPolicy.OFF -> throw denied(TrackingLocalFailure.POLICY_OFF)
            else -> Unit
        }
        if (local?.state == TrackingSessionState.RECORDING) return view(local, snapshot.entries, snapshot.buffers?.session(local.sessionId))
        // One listener per phone (BR-GPS-013): another session being collected here refuses the tap; nothing is written.
        if (collecting != null && collecting != local?.sessionId) throw denied(TrackingLocalFailure.BUSY)
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
        // A period ends after its last recorded fix, so every buffered fix stays inside it (EMP-GPS-003, B-BR-EMP-504).
        val afterFix = snapshot.buffers?.session(local.sessionId)?.checkpointAt?.plusMillis(1)
        val declared = if (at == null && (afterFix == null || afterFix.toEpochMilli() <= wallMillis())) now()
            else declaration(listOfNotNull(at, local.lastAt, afterFix).maxOrNull()!!)
        val stored = write(attempt, snapshot, trip, { envelope ->
            TrackingOperations.event(type, envelope, local.sessionId, local.tripId, cause.name, listOf(local.lastOperationId))
        }, declared)
        if (type != TrackingOperations.REDUCE) afterPeriod(attempt, local.sessionId, declared.occurredAt)
        return view(stored, snapshot(attempt).entries)
    }

    /**
     * EMP-GPS-003: a recording period ended (stop, interruption, suspension, end): an open gap ends with it and every
     * recorded fix is sealed now (B-BR-EMP-504/505). A failure here keeps the fixes for the next opening.
     */
    private suspend fun afterPeriod(attempt: EnterpriseQueueAttempt, sessionId: String, at: Instant) {
        try {
            store.updateBuffer(attempt, sessionId) { current -> current?.let { TrackingSampler.closePeriod(it, at) } }
            sealAll(attempt, sessionId, force = true)
        } catch (cancelled: kotlinx.coroutines.CancellationException) { throw cancelled }
        catch (denied: EnterpriseLocalDenied) { throw denied }
        catch (_: Exception) { }
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
        afterPeriod(attempt, local.sessionId, declared.occurredAt)
        return view(stored, snapshot(attempt).entries)
    }

    /**
     * BR-GPS-011/012 at the next opening: every recording session with no running collector in this process is
     * interrupted at its checkpoint (DEVICE_RESTARTED on another boot, else PROCESS_STOPPED); finished trips end instead.
     * Returns the number of sessions changed.
     */
    suspend fun recover(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, running: (String) -> Boolean, bootId: String,
        /** EMP-GPS-010: the permission and notification visibility observed now, null when not observed (B-BR-EMP-544). */
        device: TrackingDevice? = null): Int {
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
            if (uncollected(account, partition, local, snapshot, bootId, device) != null) changed++
        }
        return changed
    }

    /**
     * EMP-GPS-010 (B-BR-EMP-544): interrupts a recording session nobody collects for, with the cause the device observes now,
     * after its last proof of recording: the book checkpoint, its last transition or, just after it, the last buffered fix.
     */
    private suspend fun uncollected(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, local: LocalTrackingSession,
        snapshot: EnterpriseTrackingSnapshot, bootId: String, device: TrackingDevice?): TrackingSessionView? {
        val checkpoint = local.checkpoint
        val buffer = snapshot.buffers?.session(local.sessionId)
        val cause = TrackingInterruptions.observedCause(listOfNotNull(checkpoint?.bootId, buffer?.bootId), bootId, device?.permission,
            device?.notificationVisible)
        val at = listOfNotNull(checkpoint?.at, local.lastAt, buffer?.checkpointAt?.plusMillis(1)).maxOrNull()
        return interrupt(account, partition, local.sessionId, cause, at)
    }

    /**
     * EMP-GPS-010 (B-BR-EMP-544): a recording session that this process does not collect for is interrupted now at its last
     * proof of recording, with the cause the device observes; never resumed. Null when it is not recording.
     */
    suspend fun interruptUncollected(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, sessionId: String, bootId: String,
        device: TrackingDevice?): TrackingSessionView? {
        val attempt = attempt(account, partition)
        val snapshot = snapshot(attempt)
        val local = book(attempt, snapshot).sessionById(sessionId) ?: return null
        if (local.refused || local.state != TrackingSessionState.RECORDING) return null
        val trip = snapshot.trips?.trip(local.tripId) ?: return null
        // A trip that finished meanwhile ends the session instead (B-BR-EMP-499).
        if (trip.state != TripLocalState.IN_PROGRESS) return follow(account, partition, local.tripId)
        return uncollected(account, partition, local, snapshot, bootId, device)
    }

    /** The running collector's checkpoint (BR-GPS-012); false when the session is not recording any more. */
    suspend fun checkpoint(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, sessionId: String, bootId: String): Boolean {
        val attempt = attempt(account, partition)
        val local = book(attempt, snapshot(attempt)).sessionById(sessionId)
        if (local == null || local.state != TrackingSessionState.RECORDING || local.refused) return false
        return store.checkpoint(attempt, sessionId, TrackingCheckpoint(bootId, maxOf(Instant.ofEpochMilli(wallMillis()), local.lastAt)))
    }

    /**
     * EMP-GPS-003 (B-BR-EMP-501..503): filters, samples and durably records one platform report of a recording session,
     * observed at [nowElapsed] / [nowWall] on boot [bootId]; nothing is recorded for a session that is not collecting.
     */
    suspend fun record(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, sessionId: String, observation: TrackingObservation,
        economy: Boolean, nowElapsed: Long, nowWall: Instant, bootId: String,
        /** EMP-GPS-010: the permission granted with this report, null when not observed (B-BR-EMP-546). */
        permission: LocationPermission? = null): TrackingObservationOutcome {
        val attempt = attempt(account, partition)
        val snapshot = snapshot(attempt)
        var local = book(attempt, snapshot).sessionById(sessionId) ?: return TrackingObservationOutcome.IGNORED
        val collecting = collecting(account, partition, sessionId) ?: return TrackingObservationOutcome.IGNORED
        // EMP-GPS-010 (B-BR-EMP-546): never more precise than the permission granted with the report; none records nothing.
        val precision = TrackingInterruptions.fixPrecision(collecting.precision, permission) ?: return TrackingObservationOutcome.IGNORED
        if (precision != collecting.precision) {
            // The first approximate report of a precise period reduces it at that report's own instant before it is recorded.
            val fixAt = nowWall.minusMillis((nowElapsed - observation.elapsedMillis).coerceAtLeast(0))
            event(account, partition, { it.sessionById(sessionId)?.takeIf { s -> s.state == TrackingSessionState.RECORDING } },
                TrackingOperations.REDUCE, TrackingCause.PRECISION_REDUCED, fixAt) ?: return TrackingObservationOutcome.IGNORED
            local = book(attempt, snapshot(attempt)).sessionById(sessionId) ?: return TrackingObservationOutcome.IGNORED
        }
        val period = TrackingDecisions.rebuild(partition.organizationId, attempt.enrollment.target.membershipId, local).recordingPeriods.last()
        val runStart = TrackingRules.runStart(local) ?: period.start
        val previousEnd = TrackingRules.previousRunEnd(local, runStart)
        // Fixes of another boot are sealed first; a batch never mixes boots (B-BR-EMP-504).
        if (snapshot.buffers?.session(sessionId)?.let { it.bootId != bootId && it.pending.isNotEmpty() } == true) sealAll(attempt, sessionId, force = true)
        var outcome = TrackingObservationOutcome.IGNORED
        store.updateBuffer(attempt, sessionId) { current ->
            val stored = (current ?: SessionBuffer.empty(sessionId, local.tripId, bootId)).let { if (it.pending.isEmpty()) it.onBoot(bootId) else it }
            if (stored.bootId != bootId) return@updateBuffer null
            // A gap left open by an earlier run ends with that run, never inside this one (B-BR-EMP-545).
            val buffer = TrackingInterruptions.enterRun(stored, runStart, previousEnd)
            val (next, result) = TrackingSampler.observe(buffer, observation, precision, economy, period.start, nowElapsed, nowWall)
            outcome = result
            // An accepted fix that is not recorded only moves the anchors: written at most once a minute (EDR-327), well inside
            // the 180-second gap threshold, so a running process never opens a false gap and the vault is not rewritten every 5 s.
            val anchorOnly = result == TrackingObservationOutcome.ACCEPTED_NOT_RECORDED && buffer.openGap == null &&
                buffer.lastAcceptedAt?.let { next.lastAcceptedAt!!.toEpochMilli() - it.toEpochMilli() < ANCHOR_MILLIS } == true
            next.takeIf { result != TrackingObservationOutcome.IGNORED && !anchorOnly || buffer !== stored }
        }
        return outcome
    }

    /** EMP-GPS-003 (B-BR-EMP-505): the watchdog's gap check of a recording session; a gap that opens seals what came before it. */
    suspend fun checkGap(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, sessionId: String, providerEnabled: Boolean,
        nowWall: Instant, bootId: String): TrackingGap? {
        val attempt = attempt(account, partition)
        val local = book(attempt, snapshot(attempt)).sessionById(sessionId) ?: return null
        collecting(account, partition, sessionId) ?: return null
        val period = TrackingDecisions.rebuild(partition.organizationId, attempt.enrollment.target.membershipId, local).recordingPeriods.last()
        val runStart = TrackingRules.runStart(local) ?: period.start
        val previousEnd = TrackingRules.previousRunEnd(local, runStart)
        var opened: TrackingGap? = null
        store.updateBuffer(attempt, sessionId) { current ->
            val stored = current ?: SessionBuffer.empty(sessionId, local.tripId, bootId)
            // EMP-GPS-010 (B-BR-EMP-545): a gap of an earlier run ends with it and no longer hides this run's gaps.
            val buffer = TrackingInterruptions.enterRun(stored, runStart, previousEnd)
            val next = TrackingSampler.tick(buffer, nowWall, providerEnabled, period.start)
            if (next.openGap != null && buffer.openGap == null) opened = next.openGap
            next.takeIf { it !== stored }
        }
        if (opened != null) sealAll(attempt, sessionId, force = true)
        return opened
    }

    /** EMP-GPS-003 (B-BR-EMP-504): seals the session's due fixes into batch operations; returns how many were queued. */
    suspend fun seal(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, sessionId: String, force: Boolean): Int =
        sealAll(attempt(account, partition), sessionId, force)

    private suspend fun sealAll(attempt: EnterpriseQueueAttempt, sessionId: String, force: Boolean): Int {
        var queued = 0
        repeat(MAX_SEALS_PER_CALL) {
            val snapshot = snapshot(attempt)
            val local = book(attempt, snapshot).sessionById(sessionId) ?: return queued
            if (local.refused) return queued
            val buffer = snapshot.buffers?.session(sessionId) ?: return queued
            if (!TrackingBatching.due(buffer, Instant.ofEpochMilli(wallMillis()), force)) return queued
            val opening = TrackingRules.periodOpening(local, TrackingBatching.nextRun(buffer).first().at) ?: return queued
            // EMP-GPS-010 (B-BR-EMP-545): one recording run per batch, even when an earlier seal failed and two runs wait together.
            val run = TrackingBatching.nextRun(buffer).takeWhile { TrackingRules.periodOpening(local, it.at) == opening }
            val declared = declaration(run.first().at)
            var size = run.size
            var operation: EnterpriseOperation? = null
            // At most 200 fixes, then fewer until the canonical operation fits the 8 KiB protocol bound.
            while (size >= 1 && operation == null) {
                val part = run.take(size)
                operation = try {
                    TrackingBatchOperations.batch(envelope(attempt, newId(), declared), sessionId, local.tripId, buffer.nextSequence, buffer.bootId,
                        part.first().elapsedMillis, TrackingBatching.fixes(part), buffer.dropped, listOf(opening))
                } catch (_: TrackingOperationException) { size = if (size > 1) size * 3 / 4 else 0; null }
            }
            save(attempt.partition, operation ?: return queued)
            queued++
        }
        return queued
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

    private companion object {
        const val MAX_SEALS_PER_CALL = 50
        const val ANCHOR_MILLIS = 60_000L
    }
}
