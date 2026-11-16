package com.anpfuel.application.enterprise.checklist

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueProducer
import com.anpfuel.application.enterprise.queue.EnterpriseQueueProducerDescriptor
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.application.enterprise.queue.EnterpriseQueueTransaction
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.maintenance.checklist.*
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.sync.SyncResult
import com.anpfuel.domain.enterprise.trip.TripDeclarations
import java.time.Instant
import java.time.ZoneId
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/*
 * EMP-CHECK-002 — the offline checklist producer (B-BR-EMP-438/443, EDR-283/286). The cached applicable read and every
 * answer live in the sealed checklist book; the completion is one `check.run.complete` operation written in the same
 * protected write as the run's record; the server's recheck is read back on the cookie plane after its ACCEPTED receipt.
 */

/** One protected snapshot of the partition's checklist book and the retained queue entries of its completed runs. */
class EnterpriseChecklistSnapshot(val book: ChecklistBook?, entries: Map<String, EnterpriseQueueEntry>) {
    private val values = entries.toMap()
    val entries: Map<String, EnterpriseQueueEntry> get() = values.toMap()
    override fun toString(): String = "EnterpriseChecklistSnapshot(redacted)"
}

/** The protected checklist port of the creation partition. */
interface EnterpriseChecklistStore {
    /** The book bound to the attempt's exact membership and enrollment, with every entry it names, in one snapshot. */
    suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseChecklistSnapshot
    /**
     * One protected read-modify-write of the book without an operation (cached reads, answers, server reads). A local
     * refusal ([ChecklistLocalException]) writes nothing and is rethrown as itself; an unreadable or foreign book is
     * unavailable and never replaced.
     */
    suspend fun update(attempt: EnterpriseQueueAttempt, change: (ChecklistBook) -> ChecklistBook): ChecklistBook
}

/** The EMP-CHECK-001 applicable read and the EMP-CHECK-002 run read on the account cookie plane (EDR-283). */
interface EnterpriseChecklistGateway {
    suspend fun applicable(context: EnterpriseContextSummary, assetId: String, checklistContext: ChecklistContext): EnterpriseAccessResult<ChecklistApplicable>
    suspend fun run(context: EnterpriseContextSummary, assetId: String, runId: String): EnterpriseAccessResult<ChecklistServerRun>
}

private fun bookOf(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt): ChecklistBook {
    val member = attempt.enrollment.target.membershipId
    val enrollment = attempt.enrollment.enrollmentId
    return tx.record(EnterpriseChecklistProducers.BOOK_RECORD_ID)?.let { ChecklistBook.decode(it.envelope).requireBinding(member, enrollment) }
        ?: ChecklistBook.empty(member, enrollment)
}

/** The registered `check.run.complete` producer: it completes the saved run in the book in the same protected write. */
class EnterpriseChecklistProducer : EnterpriseQueueProducer {
    override fun descriptor(): EnterpriseQueueProducerDescriptor = EnterpriseQueueProducerDescriptor(ChecklistRunOperations.COMPLETE)

    override fun save(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation) {
        require(operation.type == ChecklistRunOperations.COMPLETE)
        val fact = ChecklistRunOperations.read(operation)
        val book = bookOf(tx, attempt)
        val run = book.run(fact.runId) ?: throw ChecklistLocalException(ChecklistLocalFailure.UNKNOWN_RUN)
        // The operation must be exactly the saved run: what it pins and every answer as written before completion.
        require(fact.assetId == run.assetId && fact.context == run.context && fact.templateId == run.template.templateId &&
            fact.templateVersion == run.template.version && fact.policyRevision == run.policy.revision && fact.startedAt == run.startedAt &&
            run.answers.all { it.answer != null } && fact.answers == run.answers.map { ChecklistRunAnswer(it.answer!!, it.note) })
        val next = book.withCompletion(fact.runId, operation.operationId, fact.declared.occurredAt, fact.acknowledged)
        val completed = next.run(fact.runId)!!
        require(completed.deviceOutcome == fact.deviceOutcome && completed.acknowledged == fact.acknowledged)
        tx.put(EnterpriseLocalRecord(attempt.partition, EnterpriseChecklistProducers.BOOK_RECORD_ID, next.canonicalBytes))
    }

    /** The receipt is the truth and stays in the queue entry; the book is unchanged by it. */
    override fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        ack: OperationAcknowledgement) = Unit
    override fun toString(): String = "EnterpriseChecklistProducer(redacted)"
}

object EnterpriseChecklistProducers {
    /** The fixed identifier of the one checklist-book record per partition. */
    const val BOOK_RECORD_ID = "c4ec0b00-0000-4000-8000-000000000002"
    fun all(): Array<EnterpriseQueueProducer> = arrayOf(EnterpriseChecklistProducer())
}

/** A company read could not be made now (offline or refused); the cached data stays as it was. */
class ChecklistUnreachable(val error: EnterpriseAccessError) : IllegalStateException("Checklist read unavailable")

/** Offline checklist actions of one partition; every write is durable before it returns (B-BR-EMP-438). */
class EnterpriseChecklistService(
    private val source: EnterpriseQueueSource,
    private val save: suspend (EnterprisePartition, EnterpriseOperation) -> Boolean,
    private val store: EnterpriseChecklistStore,
    private val gateway: EnterpriseChecklistGateway,
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

    private fun <T> remote(result: EnterpriseAccessResult<T>): T = when (result) {
        is EnterpriseAccessResult.Ok -> result.value
        is EnterpriseAccessResult.Err -> throw ChecklistUnreachable(result.error)
    }

    /** The device wall instant at millisecond precision. */
    fun now(): Instant = Instant.ofEpochMilli(wallMillis())

    suspend fun snapshot(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseChecklistSnapshot {
        val attempt = attempt(account, partition)
        return store.read(attempt).also { attempt.assertCurrent() }
    }

    /** Online: the applicable read of the asset and moment, kept sealed with the server instant that confirmed it. */
    suspend fun fetch(account: EnterpriseQueueAccountLease, context: EnterpriseContextSummary, assetId: String,
        checklistContext: ChecklistContext): ChecklistApplicable {
        val attempt = attempt(account, context)
        val value = remote(gateway.applicable(context, assetId, checklistContext))
        if (value.assetId != assetId || value.context != checklistContext) throw ChecklistUnreachable(EnterpriseAccessError.UNAVAILABLE)
        attempt.assertCurrent()
        store.update(attempt) { it.withApplicable(value) }
        return value
    }

    suspend fun start(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, assetId: String, checklistContext: ChecklistContext): String {
        val attempt = attempt(account, partition)
        val runId = newId()
        val at = now()
        store.update(attempt) { it.withStart(runId, assetId, checklistContext, at) }
        return runId
    }

    /** Durable before it returns; a refusal or a failed write saves nothing. */
    suspend fun answer(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, runId: String, position: Int, answer: String,
        note: String?) {
        val attempt = attempt(account, partition)
        val at = now()
        store.update(attempt) { it.withAnswer(runId, position, answer, note?.takeIf { text -> text.isNotEmpty() }, at) }
    }

    /** The local outcome with the cached policy, then one durable `check.run.complete` with the run's record (BR-OFFL-002). */
    suspend fun complete(account: EnterpriseQueueAccountLease, partition: EnterprisePartition, runId: String, acknowledged: Boolean): String {
        val attempt = attempt(account, partition)
        val book = store.read(attempt).book ?: throw ChecklistLocalException(ChecklistLocalFailure.UNKNOWN_RUN)
        attempt.assertCurrent()
        val declared = TripDeclarations.at(wallMillis(), zone())
        val operationId = newId()
        // The exact local rule first; the producer re-checks it inside the protected write.
        val evaluation = book.evaluate(runId, declared.occurredAt)
        book.withCompletion(runId, operationId, declared.occurredAt, acknowledged)
        val run = book.run(runId)!!
        val envelope = ChecklistRunEnvelope(SyncBinding.of(partition.ownerAccount, partition.origin, partition.organizationId),
            attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId, operationId, declared)
        val operation = try {
            ChecklistRunOperations.complete(envelope, runId, run.assetId, run.context.name, run.template.templateId, run.template.version,
                run.policy.revision, EnterpriseTime.formatInstant(run.startedAt), run.answers.map { ChecklistRunAnswerInput(it.answer!!, it.note) },
                evaluation.outcome.name, evaluation.acknowledgementRequired)
        } catch (_: ChecklistRunOperationException) { throw EnterprisePersistenceUnavailable() }
        save(partition, operation)
        return operationId
    }

    /**
     * Online: after a run's own ACCEPTED receipt, the run read stores the server's recheck. A BLOCKING result is read again
     * because another member may release it (B-BR-EMP-442); any other server result is final and not read again.
     */
    suspend fun readServer(account: EnterpriseQueueAccountLease, context: EnterpriseContextSummary, assetId: String): Int {
        val attempt = attempt(account, context)
        val snapshot = store.read(attempt)
        attempt.assertCurrent()
        val book = snapshot.book ?: return 0
        val pending = book.runs.filter { run ->
            run.completed && run.assetId == assetId && (run.server == null || run.server!!.outcome == ChecklistOutcome.BLOCKING) &&
                snapshot.entries[run.operationId]?.acknowledgement?.result == SyncResult.ACCEPTED
        }.sortedByDescending { it.completedAt }.take(MAX_SERVER_READS)
        var updated = 0
        for (run in pending) {
            val read = remote(gateway.run(context, assetId, run.runId))
            // A read of another run or of different facts is refused before anything is written.
            try { book.withServerResult(read) } catch (_: IllegalArgumentException) { throw ChecklistUnreachable(EnterpriseAccessError.UNAVAILABLE) }
            attempt.assertCurrent()
            store.update(attempt) { it.withServerResult(read) }
            updated++
        }
        return updated
    }
    override fun toString(): String = "EnterpriseChecklistService(redacted)"

    private companion object { const val MAX_SERVER_READS = 5 }
}

data class ChecklistViewState(
    val generation: Long = 0,
    val epoch: Long? = null,
    val context: EnterpriseContextSummary? = null,
    val assetId: String? = null,
    val checklistContext: ChecklistContext? = null,
    val busy: Boolean = false,
    /** The protected book was read; no cached read then means the template was never downloaded here. */
    val loaded: Boolean = false,
    /** The cached applicable read of the asset and moment, with its confirmation instant. */
    val applicable: ChecklistApplicable? = null,
    /** The run in progress for the asset and moment. */
    val open: ChecklistRunView? = null,
    /** The latest completed run of the asset and moment. */
    val latest: ChecklistRunView? = null,
    /** The latest valid DEPARTURE run of the asset (the trip start form). */
    val departure: ChecklistRunView? = null,
    /** The device instant the views were computed at (template age, validity). */
    val at: Instant? = null,
    /** The latest action was refused on the device; nothing was saved. */
    val refusal: ChecklistLocalFailure? = null,
    /** The latest write failed; nothing was saved (B-BR-EMP-438). */
    val saveFailed: Boolean = false,
    /** The company could not be read now; the cached data stays. */
    val refreshFailed: Boolean = false,
    val sendFailed: Boolean = false,
    /** The identifier of the latest durable save, for one polite announcement. */
    val lastSaved: String? = null,
    /** The run completed by this owner, whose outcome is shown. */
    val completed: String? = null,
    val error: EnterpriseAccessError? = null,
    val unlockRequired: Boolean = false,
    val unrecoverable: Boolean = false,
) {
    override fun toString(): String = "ChecklistViewState(redacted)"
}

/** One visible checklist owner per screen, bound to account, epoch and background lease like the trip owner. */
class ChecklistFlow(
    private val source: EnterpriseQueueSource,
    private val service: EnterpriseChecklistService,
    private val fence: EnterpriseSessionFence,
    private val deliver: suspend (EnterpriseQueueAccountLease, EnterprisePartition) -> Unit,
) {
    private val mutable = MutableStateFlow(ChecklistViewState())
    val state: StateFlow<ChecklistViewState> = mutable.asStateFlow()
    private val monitor = Any()
    private var serial = 0L
    private var closed = false
    private class Owner(val token: Long, val context: EnterpriseContextSummary, val assetId: String, val checklistContext: ChecklistContext,
        val epoch: Long, val background: Long, var account: EnterpriseQueueAccountLease? = null)
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
        mutable.value = ChecklistViewState(generation = next(), error = error)
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
            val request = Owner(next(), before.context, before.assetId, before.checklistContext, before.epoch, before.background, before.account)
            owner = request
            mutable.value = mutable.value.copy(generation = request.token, busy = true, refusal = null, saveFailed = false, refreshFailed = false,
                sendFailed = false, lastSaved = null)
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

    private suspend fun publish(request: Owner, saved: String? = null, completed: String? = null, sendFailed: Boolean = false,
        refreshFailed: Boolean = false) {
        val snapshot = service.snapshot(account(request), request.context.partition)
        val at = service.now()
        val book = snapshot.book
        val entries = snapshot.entries
        val applicable = book?.applicable(request.assetId, request.checklistContext)
        val open = book?.openRun(request.assetId, request.checklistContext)?.let { ChecklistPresentation.view(it, entries, at) }
        val latest = book?.runs?.filter { it.completed && it.assetId == request.assetId && it.context == request.checklistContext }
            ?.maxWithOrNull(compareBy<LocalChecklistRun> { it.completedAt }.thenBy { it.runId })?.let { ChecklistPresentation.view(it, entries, at) }
        val departure = book?.let { ChecklistPresentation.latestDeparture(it, entries, request.assetId, at) }
        if (!retained(request)) { invalidateIfStale(); return }
        synchronized(monitor) {
            if (owner !== request || !viewCurrent(request)) return
            val done = completed ?: mutable.value.completed?.takeIf { id -> book?.run(id)?.completed == true }
            mutable.value = mutable.value.copy(busy = false, loaded = true, applicable = applicable, open = open, latest = latest, departure = departure,
                at = at, lastSaved = saved, completed = done, sendFailed = sendFailed, refreshFailed = refreshFailed, error = null,
                unlockRequired = false, unrecoverable = false)
        }
    }

    private fun failed(request: Owner, failure: Exception, write: Boolean) {
        val sourceCurrent = retained(request)
        synchronized(monitor) {
            if (owner !== request || closed) return
            val current = mutable.value
            mutable.value = when {
                failure is EnterpriseLocalDenied || !sourceCurrent || !viewCurrent(request) -> { clearLocked(EnterpriseAccessError.STALE); return }
                failure is ChecklistLocalException -> current.copy(busy = false, refusal = failure.failure)
                failure is ChecklistUnreachable -> current.copy(busy = false, refreshFailed = true)
                failure is EnterpriseUnlockRequired || failure is EnterpriseVaultUnrecoverable -> current.copy(busy = false, loaded = false,
                    applicable = null, open = null, latest = null, departure = null, error = EnterpriseAccessError.UNAVAILABLE,
                    unlockRequired = failure is EnterpriseUnlockRequired, unrecoverable = failure is EnterpriseVaultUnrecoverable)
                // A failed write saved nothing and says so; the screen keeps what it showed before it.
                write -> current.copy(busy = false, saveFailed = true)
                else -> current.copy(busy = false, loaded = false, applicable = null, open = null, latest = null, departure = null,
                    error = EnterpriseAccessError.UNAVAILABLE)
            }
        }
    }

    private suspend fun perform(request: Owner, write: Boolean, block: suspend () -> Unit) {
        try { currentCoroutineContext().ensureActive(); block() }
        catch (cancelled: CancellationException) { synchronized(monitor) { if (owner === request) clearLocked() }; throw cancelled }
        catch (failure: Exception) { failed(request, failure, write) }
    }

    /** Reads the protected book of the exact partition for one asset and moment; no network. */
    suspend fun open(context: EnterpriseContextSummary, assetId: String, checklistContext: ChecklistContext, renderedEpoch: Long) {
        invalidateIfStale()
        val request = synchronized(monitor) {
            if (closed || fence.epoch != renderedEpoch || !Checklist.canonicalId(assetId)) return
            val created = Owner(next(), context, assetId, checklistContext, renderedEpoch, fence.backgroundEpoch)
            owner = created
            mutable.value = ChecklistViewState(generation = created.token, epoch = created.epoch, context = context, assetId = assetId,
                checklistContext = checklistContext, busy = true)
            created
        }
        perform(request, write = false) { publish(request) }
    }

    suspend fun reload(renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation) ?: return
        perform(request, write = false) { publish(request) }
    }

    /** Online: the applicable read and the company's results of this asset; a failed read keeps the cache. */
    suspend fun refresh(renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation) ?: return
        perform(request, write = false) {
            val account = account(request)
            var failedRead = false
            try { service.fetch(account, request.context, request.assetId, request.checklistContext) } catch (_: ChecklistUnreachable) { failedRead = true }
            try { service.readServer(account, request.context, request.assetId) } catch (_: ChecklistUnreachable) { failedRead = true }
            if (!retained(request)) { invalidateIfStale(); return@perform }
            publish(request, refreshFailed = failedRead)
        }
    }

    private suspend fun write(renderedEpoch: Long, generation: Long, completes: Boolean, action: suspend (Owner, EnterpriseQueueAccountLease) -> String) {
        val request = begin(renderedEpoch, generation) ?: return
        perform(request, write = true) {
            val saved = try { action(request, account(request)) } catch (refused: ChecklistLocalException) {
                // Nothing was saved; the screen is read again so it states the run as it now is (an expired run offers a new one).
                if (!retained(request)) { invalidateIfStale(); return@perform }
                publish(request)
                synchronized(monitor) { if (owner === request && viewCurrent(request)) mutable.value = mutable.value.copy(refusal = refused.failure) }
                return@perform
            }
            if (!retained(request)) { invalidateIfStale(); return@perform }
            publish(request, saved = saved, completed = if (completes) saved else null)
        }
    }

    suspend fun start(renderedEpoch: Long, generation: Long) {
        synchronized(monitor) { if (owner?.token == generation) mutable.value = mutable.value.copy(completed = null) }
        write(renderedEpoch, generation, completes = false) { request, account ->
            service.start(account, request.context.partition, request.assetId, request.checklistContext)
        }
    }

    suspend fun answer(runId: String, position: Int, answer: String, note: String?, renderedEpoch: Long, generation: Long) =
        write(renderedEpoch, generation, completes = false) { request, account ->
            service.answer(account, request.context.partition, runId, position, answer, note)
            "$runId:$position"
        }

    suspend fun complete(runId: String, acknowledged: Boolean, renderedEpoch: Long, generation: Long) =
        write(renderedEpoch, generation, completes = true) { request, account ->
            service.complete(account, request.context.partition, runId, acknowledged)
            runId
        }

    /** Online only: one delivery attempt of this partition's queue, then the company's results of this asset. */
    suspend fun sendNow(renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation) ?: return
        perform(request, write = false) {
            val failedSend = try { deliver(account(request, delivery = true), request.context.partition); false }
            catch (cancelled: CancellationException) { throw cancelled }
            catch (denied: EnterpriseLocalDenied) { throw denied }
            catch (_: Exception) { true }
            if (!retained(request)) { invalidateIfStale(); return@perform }
            val failedRead = try { service.readServer(account(request), request.context, request.assetId); false } catch (_: ChecklistUnreachable) { true }
            if (!retained(request)) { invalidateIfStale(); return@perform }
            publish(request, sendFailed = failedSend, refreshFailed = failedRead)
        }
    }
}
