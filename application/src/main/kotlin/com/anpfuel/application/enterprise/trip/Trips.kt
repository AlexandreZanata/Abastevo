package com.anpfuel.application.enterprise.trip

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueProducer
import com.anpfuel.application.enterprise.queue.EnterpriseQueueProducerDescriptor
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.application.enterprise.queue.EnterpriseQueueTransaction
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import com.anpfuel.domain.enterprise.meter.MeterDomainException
import com.anpfuel.domain.enterprise.meter.MeterEntryForm
import com.anpfuel.domain.enterprise.meter.MeterKind
import com.anpfuel.domain.enterprise.meter.MeterPlausibility
import com.anpfuel.domain.enterprise.meter.MeterPoint
import com.anpfuel.domain.enterprise.meter.MeterPresence
import com.anpfuel.domain.enterprise.meter.MeterSeriesEntry
import com.anpfuel.domain.enterprise.meter.MeterValidation
import com.anpfuel.domain.enterprise.meter.MeterValue
import com.anpfuel.domain.enterprise.meter.MeterValues
import com.anpfuel.domain.enterprise.meter.ReadingFlag
import com.anpfuel.domain.enterprise.meter.ReadingState
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.sync.SyncResult
import com.anpfuel.domain.enterprise.trip.*
import java.time.Instant
import java.time.ZoneId
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/*
 * EMP-TRIP-005 — the offline trip producer (B-BR-EMP-375..380, EDR-242..245). The operations are the facts; the
 * sealed trip book is their local index; acceptance is only ever read from each record's own retained receipt.
 */

/** One protected snapshot of the partition's trip book and the retained queue entries it names (EDR-242). */
class EnterpriseTripSnapshot(val book: TripBook?, entries: Map<String, EnterpriseQueueEntry>) {
    private val values = entries.toMap()
    val entries: Map<String, EnterpriseQueueEntry> get() = values.toMap()
    override fun toString(): String = "EnterpriseTripSnapshot(redacted)"
}

/** The protected trip read port of the creation partition. */
fun interface EnterpriseTripStore {
    /** The book bound to the attempt's exact membership and enrollment, with every entry it names, in one snapshot. */
    suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseTripSnapshot
}

/** Display hints of a start for its producer; memory only, never authority (EDR-242). */
class EnterpriseTripStartHints {
    private val values = java.util.concurrent.ConcurrentHashMap<String, Pair<TripAssetLabel, Boolean>>()
    fun put(operationId: String, label: TripAssetLabel, reviewWarningAccepted: Boolean) {
        EnterprisePartition.requireId(operationId)
        require(values.size < 64) { "Too many pending trip starts" }
        values[operationId] = label to reviewWarningAccepted
    }
    fun peek(operationId: String): Pair<TripAssetLabel, Boolean>? = values[operationId]
    fun remove(operationId: String) { values.remove(operationId) }
    override fun toString(): String = "EnterpriseTripStartHints(redacted)"
}

private fun bookOf(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt): TripBook {
    val member = attempt.enrollment.target.membershipId
    val enrollment = attempt.enrollment.enrollmentId
    return tx.record(EnterpriseTripProducers.BOOK_RECORD_ID)?.let { TripBook.decode(it.envelope).requireBinding(member, enrollment) }
        ?: TripBook.empty(member, enrollment)
}

/** One of the four registered `trip.*` producers: it indexes its operation in the book in the same protected write. */
class EnterpriseTripProducer(private val type: String, private val hints: EnterpriseTripStartHints) : EnterpriseQueueProducer {
    init { require(type in TripOperations.TYPES) }
    override fun descriptor(): EnterpriseQueueProducerDescriptor = EnterpriseQueueProducerDescriptor(type)

    override fun save(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation) {
        require(operation.type == type)
        val fact = TripOperations.read(operation)
        val book = bookOf(tx, attempt)
        val id = operation.operationId
        val at = fact.declared.occurredAt
        val next = when (fact) {
            is TripOperationFact.Start -> {
                val (label, warning) = hints.peek(id) ?: (TripAssetLabel("", "", "") to false)
                book.withStart(LocalTrip(fact.tripId, fact.assetId, label, warning, id, at))
            }
            is TripOperationFact.Event -> {
                // A device event depends on its own trip's start, nothing else.
                require(fact.dependsOn == listOfNotNull(book.trip(fact.tripId)?.startOperationId))
                book.withEvent(fact.tripId, id, at)
            }
            is TripOperationFact.Finish -> {
                require(fact.dependsOn == listOfNotNull(book.trip(fact.tripId)?.startOperationId))
                book.withFinish(fact.tripId, id, at)
            }
            is TripOperationFact.Submit -> {
                require(fact.revision == 1L && fact.dependsOn == listOfNotNull(book.trip(fact.tripId)?.finishOperationId))
                book.withSubmission(fact.tripId, id)
            }
        }
        tx.put(EnterpriseLocalRecord(attempt.partition, EnterpriseTripProducers.BOOK_RECORD_ID, next.canonicalBytes))
    }

    /** Only a final REJECTED start changes the index: its asset is free again on this device (B-BR-EMP-377). */
    override fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        ack: OperationAcknowledgement) {
        if (type != TripOperations.START || ack.result != SyncResult.REJECTED) return
        val fact = TripOperations.read(operation) as TripOperationFact.Start
        val book = bookOf(tx, attempt)
        // The receipt is the truth; an index that does not know this start keeps the receipt and changes nothing.
        if (book.trip(fact.tripId)?.startOperationId != operation.operationId) return
        tx.put(EnterpriseLocalRecord(attempt.partition, EnterpriseTripProducers.BOOK_RECORD_ID,
            book.withStartRefused(fact.tripId, operation.operationId).canonicalBytes))
    }
    override fun toString(): String = "EnterpriseTripProducer(redacted)"
}

object EnterpriseTripProducers {
    /** The fixed identifier of the one trip-book record per partition. */
    const val BOOK_RECORD_ID = "f05e0b00-0000-4000-8000-000000000005"
    fun all(hints: EnterpriseTripStartHints): Array<EnterpriseQueueProducer> =
        listOf(TripOperations.START, TripOperations.EVENT, TripOperations.FINISH, TripOperations.SUBMIT).map { EnterpriseTripProducer(it, hints) }.toTypedArray()
}

/** A meter the driver may read at the start or finish: the picker's in-service meter or the departure one. */
class TripMeterChoice(val meterId: String, val segmentId: String, val kind: MeterKind, val modulus: Long?) {
    override fun toString(): String = "TripMeterChoice(redacted)"
}

/** What the driver entered for a departure or final reading. */
sealed class TripCapture {
    class Reading(val meter: TripMeterChoice, val form: MeterEntryForm, val raw: String, val minutes: String? = null) : TripCapture()
    class Absent(val reason: String) : TripCapture()
    final override fun toString(): String = "TripCapture(redacted)"
}

/** The entered value or reason does not fit its profile; nothing was saved. */
class TripCaptureInvalid : IllegalArgumentException("Trip capture invalid")
/** The selected context has no employee identifier (B-BR-EMP-380); confirm the organization again. */
class TripDriverUnknown : IllegalStateException("Trip driver unknown")

/** Offline trip actions of one partition; every write is durable before it returns (B-BR-EMP-376). */
class EnterpriseTripService(
    private val source: EnterpriseQueueSource,
    private val save: suspend (EnterprisePartition, EnterpriseOperation) -> Boolean,
    private val store: EnterpriseTripStore,
    private val hints: EnterpriseTripStartHints,
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

    private suspend fun book(attempt: EnterpriseQueueAttempt): TripBook {
        val snapshot = store.read(attempt)
        attempt.assertCurrent()
        return snapshot.book ?: TripBook.empty(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
    }

    /** B-BR-EMP-378: the device wall instant, zone and offset; the run clock only when it can be read. */
    private fun now(): Pair<TimeDeclaration, TripClockReading?> {
        val device = try { clock() } catch (_: Exception) { null }
        val declared = TripDeclarations.at(wallMillis(), zone())
        val run = device?.let { TripClockReading(it.boot, it.elapsedMillis) }
            ?.takeIf { runCatching { TripRunClock(it.bootId, it.elapsedMillis) }.isSuccess }
        return declared to run
    }

    private fun envelope(attempt: EnterpriseQueueAttempt, operationId: String, declared: TimeDeclaration): TripEnvelope {
        val p = attempt.partition
        return TripEnvelope(SyncBinding.of(p.ownerAccount, p.origin, p.organizationId), attempt.enrollment.target.membershipId,
            attempt.enrollment.enrollmentId, operationId, declared)
    }

    private fun value(capture: TripCapture.Reading): MeterValue = try {
        MeterValues.parse(capture.meter.kind, capture.form, capture.raw.trim(), capture.minutes?.trim(), capture.meter.modulus)
    } catch (_: MeterDomainException) { throw TripCaptureInvalid() }

    private fun evidence(capture: TripCapture): Pair<TripReadingInput?, String?> = when (capture) {
        is TripCapture.Absent -> {
            try { MeterPresence.missing(capture.reason) } catch (_: MeterDomainException) { throw TripCaptureInvalid() }
            null to capture.reason
        }
        is TripCapture.Reading -> {
            val parsed = value(capture)
            TripReadingInput(capture.meter.meterId, capture.meter.segmentId, newId(), parsed.kind.name, parsed.units.toString()) to null
        }
    }

    private inline fun build(block: () -> EnterpriseOperation): EnterpriseOperation =
        try { block() } catch (_: TripOperationException) { throw TripCaptureInvalid() }

    suspend fun trips(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): List<TripView> {
        val attempt = attempt(account, partition)
        val snapshot = store.read(attempt)
        attempt.assertCurrent()
        val book = snapshot.book ?: return emptyList()
        return TripPresentation.views(book, snapshot.entries)
    }

    suspend fun start(account: EnterpriseQueueAccountLease, context: EnterpriseContextSummary, choice: EnterpriseAssetChoice,
        label: TripAssetLabel, departure: TripCapture): String {
        val driver = context.employeeId ?: throw TripDriverUnknown()
        val partition = context.partition
        if (choice.organizationId != partition.organizationId) throw EnterpriseLocalDenied()
        val attempt = attempt(account, partition)
        if (attempt.enrollment.target.membershipId != context.membershipId) throw EnterpriseLocalDenied()
        val (reading, reason) = evidence(departure)
        val (declared, run) = now()
        val tripId = newId()
        val operationId = newId()
        // The exact local rule first (B-BR-EMP-377); the producer re-checks it inside the protected write.
        book(attempt).withStart(LocalTrip(tripId, choice.assetId, label, choice.reviewWarningAccepted, operationId, declared.occurredAt))
        val operation = build { TripOperations.start(envelope(attempt, operationId, declared), tripId, choice.assetId, driver, reading, reason, run) }
        hints.put(operationId, label, choice.reviewWarningAccepted)
        try { save(partition, operation) } finally { hints.remove(operationId) }
        return operationId
    }

    suspend fun log(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, tripId: String, type: TripLogbookType,
        text: String?): String {
        val attempt = attempt(account, partition)
        val book = book(attempt)
        val trip = book.trip(tripId) ?: throw TripLocalException(TripLocalFailure.UNKNOWN_TRIP)
        val (declared, _) = now()
        val operationId = newId()
        book.withEvent(tripId, operationId, declared.occurredAt)
        val note = text?.takeIf { it.isNotBlank() }
        val operation = build { TripOperations.event(envelope(attempt, operationId, declared), newId(), tripId, type.name, note, trip.startOperationId) }
        save(partition, operation)
        return operationId
    }

    suspend fun finish(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, tripId: String, final: TripCapture): String {
        val attempt = attempt(account, partition)
        val book = book(attempt)
        val trip = book.trip(tripId) ?: throw TripLocalException(TripLocalFailure.UNKNOWN_TRIP)
        val (reading, reason) = evidence(final)
        val (declared, run) = now()
        val operationId = newId()
        book.withFinish(tripId, operationId, declared.occurredAt)
        val operation = build { TripOperations.finish(envelope(attempt, operationId, declared), tripId, reading, reason, run, trip.startOperationId) }
        save(partition, operation)
        return operationId
    }

    suspend fun markReady(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, tripId: String): String {
        val attempt = attempt(account, partition)
        val book = book(attempt)
        val operationId = newId()
        val finish = book.withSubmission(tripId, operationId).trip(tripId)!!.finishOperationId
        val (declared, _) = now()
        val operation = build { TripOperations.submit(envelope(attempt, operationId, declared), tripId, 1, finish) }
        save(partition, operation)
        return operationId
    }

    /** METER-003: a final reading below the departure or at an implausible rate is warned, never blocked. */
    fun finishWarnings(trip: TripView, final: TripCapture): List<ReadingFlag> {
        if (final !is TripCapture.Reading) return emptyList()
        val departure = ((trip.start.fact as? TripOperationFact.Start)?.evidence as? TripEvidence.Reading) ?: return emptyList()
        if (departure.meterId != final.meter.meterId || departure.segmentId != final.meter.segmentId) return emptyList()
        val value = try { value(final) } catch (_: TripCaptureInvalid) { return emptyList() }
        val startedAt = trip.trip.startedAt
        val at = Instant.ofEpochMilli(wallMillis())
        if (value.kind != departure.value.kind || !at.isAfter(startedAt)) return emptyList()
        val series = listOf(MeterSeriesEntry(MeterPoint(departure.readingId, departure.value.units, startedAt, startedAt), ReadingState.VALID))
        return MeterValidation.check(MeterPlausibility.DEFAULT, value.kind, series, MeterPoint("final", value.units, at, at)).flags
            .filter { it != ReadingFlag.FUTURE_INSTANT }
    }
    override fun toString(): String = "EnterpriseTripService(redacted)"
}

data class TripViewState(
    val generation: Long = 0,
    val epoch: Long? = null,
    val context: EnterpriseContextSummary? = null,
    val busy: Boolean = false,
    /** The protected book was read; an empty list then means no trip on this device. */
    val loaded: Boolean = false,
    val trips: List<TripView> = emptyList(),
    val openTripId: String? = null,
    /** The latest action was refused on the device (B-BR-EMP-377); nothing was saved. */
    val refusal: TripLocalFailure? = null,
    val captureInvalid: Boolean = false,
    /** The latest write failed; nothing was saved (B-BR-EMP-376). */
    val saveFailed: Boolean = false,
    val driverUnknown: Boolean = false,
    /** The operation identifier of the latest durable save, for one polite announcement. */
    val lastSaved: String? = null,
    val sendFailed: Boolean = false,
    val error: EnterpriseAccessError? = null,
    val unlockRequired: Boolean = false,
    val unrecoverable: Boolean = false,
) {
    val openTrip: TripView? get() = openTripId?.let { id -> trips.firstOrNull { it.trip.tripId == id } }
    override fun toString(): String = "TripViewState(redacted)"
}

/** One visible trip owner per screen, bound to account, epoch and background lease like "Meus ativos". */
class TripFlow(
    private val source: EnterpriseQueueSource,
    private val service: EnterpriseTripService,
    private val fence: EnterpriseSessionFence,
    private val deliver: suspend (EnterpriseQueueAccountLease, EnterprisePartition) -> Unit,
) {
    private val mutable = MutableStateFlow(TripViewState())
    val state: StateFlow<TripViewState> = mutable.asStateFlow()
    private val monitor = Any()
    private var serial = 0L
    private var closed = false
    private class Owner(val token: Long, val context: EnterpriseContextSummary, val epoch: Long, val background: Long,
        var account: EnterpriseQueueAccountLease? = null)
    private var owner: Owner? = null

    private fun next(): Long = Math.addExact(serial, 1).also { serial = it }
    private fun viewCurrent(request: Owner): Boolean = synchronized(monitor) {
        !closed && owner === request && mutable.value.generation == request.token &&
            fence.epoch == request.epoch && fence.backgroundEpoch == request.background
    }
    private fun retained(request: Owner): Boolean {
        if (!viewCurrent(request)) return false
        if (runCatching { request.account?.assertCurrent() }.isFailure) return false
        return viewCurrent(request)
    }
    private fun clearLocked(error: EnterpriseAccessError? = null) {
        owner = null
        mutable.value = TripViewState(generation = next(), error = error)
    }
    fun withdraw() = synchronized(monitor) { clearLocked() }
    fun close() = synchronized(monitor) { if (!closed) { clearLocked(); closed = true } }
    fun invalidateIfStale() {
        val captured = synchronized(monitor) { owner } ?: return
        if (!retained(captured)) synchronized(monitor) { if (owner === captured) clearLocked(EnterpriseAccessError.STALE) }
    }

    /** A new generation of the current owner for a rendered gesture; transient outcomes of the previous one are dropped. */
    private fun begin(renderedEpoch: Long, generation: Long): Owner? {
        invalidateIfStale()
        return synchronized(monitor) {
            val before = owner ?: return@synchronized null
            if (closed || mutable.value.busy || before.epoch != renderedEpoch || before.token != generation ||
                fence.epoch != renderedEpoch || fence.backgroundEpoch != before.background) return@synchronized null
            val request = Owner(next(), before.context, before.epoch, before.background, before.account)
            owner = request
            mutable.value = mutable.value.copy(generation = request.token, busy = true, refusal = null, captureInvalid = false,
                saveFailed = false, driverUnknown = false, lastSaved = null, sendFailed = false)
            request
        }
    }

    private suspend fun account(request: Owner, delivery: Boolean = false): EnterpriseQueueAccountLease {
        val account = if (delivery) source.deliveryAccount() else source.account()
        if (!account.identity.owns(request.context.partition) || account.epoch != request.epoch || account.background != request.background)
            throw EnterpriseLocalDenied()
        synchronized(monitor) { if (owner === request) request.account = account }
        return account
    }

    private suspend fun publish(request: Owner, saved: String? = null, sendFailed: Boolean = false) {
        val trips = service.trips(account(request), request.context.partition)
        if (!retained(request)) { invalidateIfStale(); return }
        synchronized(monitor) {
            if (owner !== request || !viewCurrent(request)) return
            val open = mutable.value.openTripId?.takeIf { id -> trips.any { it.trip.tripId == id } }
            mutable.value = mutable.value.copy(busy = false, loaded = true, trips = trips, openTripId = open, lastSaved = saved,
                sendFailed = sendFailed, error = null, unlockRequired = false, unrecoverable = false)
        }
    }

    private fun failed(request: Owner, failure: Exception, write: Boolean) {
        val sourceCurrent = retained(request)
        synchronized(monitor) {
            if (owner !== request || closed) return
            val current = mutable.value
            mutable.value = when {
                failure is EnterpriseLocalDenied || !sourceCurrent || !viewCurrent(request) -> { clearLocked(EnterpriseAccessError.STALE); return }
                failure is TripLocalException -> current.copy(busy = false, refusal = failure.failure)
                failure is TripCaptureInvalid -> current.copy(busy = false, captureInvalid = true)
                failure is TripDriverUnknown -> current.copy(busy = false, driverUnknown = true)
                failure is EnterpriseUnlockRequired || failure is EnterpriseVaultUnrecoverable -> current.copy(busy = false, loaded = false,
                    trips = emptyList(), openTripId = null, error = EnterpriseAccessError.UNAVAILABLE,
                    unlockRequired = failure is EnterpriseUnlockRequired, unrecoverable = failure is EnterpriseVaultUnrecoverable)
                // A failed write saved nothing and says so; the list read before it stays as it was.
                write -> current.copy(busy = false, saveFailed = true)
                else -> current.copy(busy = false, loaded = false, trips = emptyList(), openTripId = null, error = EnterpriseAccessError.UNAVAILABLE)
            }
        }
    }

    private suspend fun perform(request: Owner, write: Boolean, block: suspend () -> Unit) {
        try { currentCoroutineContext().ensureActive(); block() }
        catch (cancelled: CancellationException) { synchronized(monitor) { if (owner === request) clearLocked() }; throw cancelled }
        catch (failure: Exception) { failed(request, failure, write) }
    }

    /** Reads the protected trip book of the exact partition; no network. */
    suspend fun open(context: EnterpriseContextSummary, renderedEpoch: Long) {
        invalidateIfStale()
        val request = synchronized(monitor) {
            if (closed || fence.epoch != renderedEpoch) return
            val created = Owner(next(), context, renderedEpoch, fence.backgroundEpoch)
            owner = created
            mutable.value = TripViewState(generation = created.token, epoch = created.epoch, context = context, busy = true)
            created
        }
        perform(request, write = false) { publish(request) }
    }

    suspend fun reload(renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation) ?: return
        perform(request, write = false) { publish(request) }
    }

    private suspend fun write(renderedEpoch: Long, generation: Long, action: suspend (Owner, EnterpriseQueueAccountLease) -> String) {
        val request = begin(renderedEpoch, generation) ?: return
        perform(request, write = true) {
            val saved = action(request, account(request))
            if (!retained(request)) { invalidateIfStale(); return@perform }
            publish(request, saved = saved)
        }
    }

    suspend fun start(choice: EnterpriseAssetChoice, label: TripAssetLabel, departure: TripCapture, renderedEpoch: Long, generation: Long) =
        write(renderedEpoch, generation) { request, account -> service.start(account, request.context, choice, label, departure) }

    suspend fun log(tripId: String, type: TripLogbookType, text: String?, renderedEpoch: Long, generation: Long) =
        write(renderedEpoch, generation) { request, account -> service.log(account, request.context.partition, tripId, type, text) }

    suspend fun finish(tripId: String, final: TripCapture, renderedEpoch: Long, generation: Long) =
        write(renderedEpoch, generation) { request, account -> service.finish(account, request.context.partition, tripId, final) }

    suspend fun markReady(tripId: String, renderedEpoch: Long, generation: Long) =
        write(renderedEpoch, generation) { request, account -> service.markReady(account, request.context.partition, tripId) }

    /** Online only: one delivery attempt of this partition's queue, then a local re-read. */
    suspend fun sendNow(renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation) ?: return
        perform(request, write = false) {
            val failedSend = try { deliver(account(request, delivery = true), request.context.partition); false }
            catch (cancelled: CancellationException) { throw cancelled }
            catch (denied: EnterpriseLocalDenied) { throw denied }
            catch (_: Exception) { true }
            if (!retained(request)) { invalidateIfStale(); return@perform }
            publish(request, sendFailed = failedSend)
        }
    }

    fun openTrip(tripId: String?, renderedEpoch: Long, generation: Long) {
        invalidateIfStale()
        synchronized(monitor) {
            val current = owner ?: return
            if (closed || current.epoch != renderedEpoch || current.token != generation || fence.epoch != renderedEpoch) return
            if (tripId != null && mutable.value.trips.none { it.trip.tripId == tripId }) return
            mutable.value = mutable.value.copy(openTripId = tripId)
        }
    }

    fun finishWarnings(tripId: String, final: TripCapture): List<ReadingFlag> =
        mutable.value.trips.firstOrNull { it.trip.tripId == tripId }?.let { service.finishWarnings(it, final) } ?: emptyList()
}
