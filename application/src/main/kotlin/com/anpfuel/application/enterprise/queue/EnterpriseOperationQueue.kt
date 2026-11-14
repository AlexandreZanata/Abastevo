package com.anpfuel.application.enterprise.queue

import com.anpfuel.application.enterprise.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.sync.*
import com.anpfuel.application.enterprise.sync.EnterpriseDeliveryActivity
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive

/** Only the actual source adapter captures this lease; values do not grant authority. */
class EnterpriseQueueAccountLease private constructor(val identity: EnterpriseLocalIdentity, val epoch: Long,
    val background: Long, val attemptId: String, val proofBudget: SyncDeliveryBudget, private val retained: () -> Boolean) {
    fun assertCurrent() {
        if (!runCatching(retained).getOrDefault(false)) throw EnterpriseLocalDenied()
    }
    fun forEnrollment(enrollment: EnterpriseEnrollmentBinding): EnterpriseQueueAttempt {
        assertCurrent()
        if (!identity.owns(enrollment.target.partition)) throw EnterpriseLocalDenied()
        return EnterpriseQueueAttempt(this, enrollment)
    }
    override fun toString(): String = "EnterpriseQueueAccountLease(redacted)"
    companion object {
        fun capture(identity: EnterpriseLocalIdentity, epoch: Long, background: Long, attemptId: String,
            proofBudget: SyncDeliveryBudget = SyncDeliveryBudget.maximum(), retained: () -> Boolean): EnterpriseQueueAccountLease {
            require(epoch >= 0 && background >= 0)
            EnterprisePartition.requireId(attemptId)
            return EnterpriseQueueAccountLease(identity, epoch, background, attemptId, proofBudget, retained).also { it.assertCurrent() }
        }
    }
}

class EnterpriseQueueAttempt internal constructor(val account: EnterpriseQueueAccountLease,
    val enrollment: EnterpriseEnrollmentBinding) {
    val partition: EnterprisePartition get() = enrollment.target.partition
    fun assertCurrent() {
        account.assertCurrent()
        if (!account.identity.owns(partition)) throw EnterpriseLocalDenied()
    }
    override fun toString(): String = "EnterpriseQueueAttempt(redacted)"
}

interface EnterpriseQueueSource {
    suspend fun account(): EnterpriseQueueAccountLease
    /** Delivery may refresh before capture; offline local saves never require network. */
    suspend fun deliveryAccount(): EnterpriseQueueAccountLease = account()
    suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt
}

/** Borrowed for one synchronous protected snapshot callback; never retain it. */
interface EnterpriseQueueTransaction {
    fun record(id: String): EnterpriseLocalRecord?
    fun put(record: EnterpriseLocalRecord)
}

interface EnterpriseOperationQueue {
    suspend fun save(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        project: (EnterpriseQueueTransaction) -> Unit): Boolean
    suspend fun pending(attempt: EnterpriseQueueAttempt): List<EnterpriseQueueEntry>
    suspend fun entry(attempt: EnterpriseQueueAttempt, operationId: String): EnterpriseQueueEntry?
    suspend fun acknowledge(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        ack: OperationAcknowledgement, project: (EnterpriseQueueTransaction) -> Unit): Boolean
    /** Structural CAS; false preserves a newer retry or an already-final entry. */
    suspend fun retry(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        expected: QueueRetryState, next: QueueRetryState): Boolean
    suspend fun stats(attempt: EnterpriseQueueAttempt): EnterpriseQueueStats
}

class EnterpriseQueueProducerDescriptor(val type: String, val schema: Long = 1) {
    init {
        val segments = type.split('.')
        fun valid(segment: String): Boolean {
            if (segment.isEmpty() || segment.first() !in 'a'..'z' || segment.last() == '_') return false
            var underscore = false
            for (character in segment) {
                if (character == '_') { if (underscore) return false; underscore = true }
                else { if (character !in 'a'..'z' && character !in '0'..'9') return false; underscore = false }
            }
            return true
        }
        require(type.length <= 8192 && segments.size >= 2 && segments.all(::valid) && schema == 1L)
    }
    override fun toString(): String = "EnterpriseQueueProducerDescriptor(redacted)"
}

/** Real owning eligibility and projectors only. Custody is never business authority. */
interface EnterpriseQueueProducer {
    fun descriptor(): EnterpriseQueueProducerDescriptor
    fun save(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation)
    /** Must preserve a newer local edit when this original operation's ACK arrives. */
    fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt,
        operation: EnterpriseOperation, ack: OperationAcknowledgement)
}

class EnterpriseQueueRegistry(vararg producers: EnterpriseQueueProducer) {
    private val values = linkedMapOf<Pair<String, Long>, EnterpriseQueueProducer>()
    init {
        for (producer in producers) {
            val descriptor = producer.descriptor()
            require(values.put(descriptor.type to descriptor.schema, producer) == null)
        }
    }
    fun producer(operation: EnterpriseOperation): EnterpriseQueueProducer? = values[operation.type to operation.operationSchema]
    override fun toString(): String = "EnterpriseQueueRegistry(redacted)"
}

class EnterpriseOperationDelivery(acknowledgements: List<OperationAcknowledgement>, val retryAfterMillis: Long? = null) {
    private val values = acknowledgements.toList()
    val acknowledgements: List<OperationAcknowledgement> get() = values.toList()
    init { require(retryAfterMillis == null || retryAfterMillis >= 0) }
    override fun toString(): String = "EnterpriseOperationDelivery(redacted)"
}

class EnterpriseQueueDeliveryFailure(val retryAfterMillis: Long? = null) : IllegalStateException("Enterprise operation delivery unavailable") {
    init { require(retryAfterMillis == null || retryAfterMillis >= 0) }
}

fun interface EnterpriseOperationTransport {
    suspend fun deliver(attempt: EnterpriseQueueAttempt, operations: List<EnterpriseOperation>): EnterpriseOperationDelivery
}

class EnterpriseQueueDispatchResult(val attempted: Int = 0, val completed: Int = 0, val retryable: Int = 0,
    val attention: Int = 0, val retryAfterMillis: Long? = null) {
    override fun toString(): String = "EnterpriseQueueDispatchResult(redacted)"
}

class EnterpriseQueueService(private val source: EnterpriseQueueSource, private val queue: EnterpriseOperationQueue,
    private val transport: EnterpriseOperationTransport, private val registry: EnterpriseQueueRegistry,
    private val clock: () -> EnterpriseDeviceClock, private val entropy: (Long) -> Long,
    private val limits: SyncLimits = SyncLimits.DEFAULT, private val retryPolicy: QueueRetryPolicy = QueueRetryPolicy(),
    private val activity: EnterpriseDeliveryActivity? = null) {
    private suspend fun <T> guarded(work: suspend () -> T): T = try { work() }
    catch (cancelled: CancellationException) { throw cancelled }
    catch (denied: EnterpriseLocalDenied) { throw denied }
    catch (reused: EnterpriseOperationReused) { throw reused }
    catch (unlock: EnterpriseUnlockRequired) { throw unlock }
    catch (lost: EnterpriseVaultUnrecoverable) { throw lost }
    catch (_: Exception) { throw EnterprisePersistenceUnavailable() }

    private suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt {
        account.assertCurrent()
        if (!account.identity.owns(partition)) throw EnterpriseLocalDenied()
        val attempt = source.attempt(account, partition)
        if (attempt.account !== account || attempt.partition != partition) throw EnterpriseLocalDenied()
        attempt.assertCurrent()
        return attempt
    }

    private fun matches(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation) {
        val partition = attempt.partition
        if (operation.binding.ownerAccountId != partition.ownerAccount || operation.binding.apiOrigin != partition.origin ||
            operation.binding.organizationId != partition.organizationId || operation.membershipId != attempt.enrollment.target.membershipId ||
            operation.enrollmentId != attempt.enrollment.enrollmentId) throw EnterpriseLocalDenied()
        EnterpriseOperation.parse(operation.canonicalBytes, operation.binding, limits)
    }

    suspend fun save(partition: EnterprisePartition, operation: EnterpriseOperation): Boolean = guarded {
        val attempt = attempt(source.account(), partition)
        matches(attempt, operation)
        val producer = registry.producer(operation) ?: throw EnterprisePersistenceUnavailable()
        attempt.assertCurrent()
        val saved = queue.save(attempt, operation) { tx ->
            attempt.assertCurrent(); producer.save(tx, attempt, operation); attempt.assertCurrent()
        }
        attempt.assertCurrent()
        saved
    }

    suspend fun dispatch(partition: EnterprisePartition): EnterpriseQueueDispatchResult = guarded {
        dispatch(source.deliveryAccount(), partition)
    }

    suspend fun dispatch(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueDispatchResult = guarded {
        val attempt = attempt(account, partition)
        val entries = queue.pending(attempt).toList()
        attempt.assertCurrent()
        if (entries.size > 10000 || entries.any { it.acknowledgement != null }) throw EnterprisePersistenceUnavailable()
        entries.forEach { matches(attempt, it.operation) }
        val byId = entries.associateBy { it.operation.operationId }.toMutableMap()
        if (byId.size != entries.size) throw EnterprisePersistenceUnavailable()
        val order = EnterpriseQueueOrdering.order(entries.map { it.operation })
        val attention = order.cycleBlocked.toMutableSet()
        val capturedClock = clock()
        val selected = mutableListOf<EnterpriseOperation>()
        val remainingDelay = mutableMapOf<String, Long>()
        val completed = mutableSetOf<String>()
        suspend fun result(value: EnterpriseQueueDispatchResult): EnterpriseQueueDispatchResult {
            currentCoroutineContext().ensureActive()
            attempt.assertCurrent()
            return value
        }
        fun delayFor(retry: QueueRetryState, now: EnterpriseDeviceClock): Long {
            val at = retry.clock ?: return 0
            if (at.boot != now.boot || now.elapsedMillis < at.elapsedMillis) return retry.delayMillis
            return maxOf(0, retry.delayMillis - (now.elapsedMillis - at.elapsedMillis))
        }
        suspend fun currentEntry(operation: EnterpriseOperation): EnterpriseQueueEntry {
            attempt.assertCurrent()
            val current = queue.entry(attempt, operation.operationId) ?: throw EnterprisePersistenceUnavailable()
            attempt.assertCurrent(); matches(attempt, current.operation)
            if (current.operation.payloadDigest != operation.payloadDigest ||
                !current.operation.canonicalBytes.contentEquals(operation.canonicalBytes)) throw EnterprisePersistenceUnavailable()
            current.acknowledgement?.let { OperationAcknowledgement.parse(it.canonicalBytes, operation, limits) }
            return current.also { byId[operation.operationId] = it }
        }
        for (operation in order.ordered) {
            if (registry.producer(operation) == null) { attention.add(operation.operationId); continue }
            var retry = byId.getValue(operation.operationId).retry
            val priorClock = retry.clock
            if (priorClock != null && (priorClock.boot != capturedClock.boot || capturedClock.elapsedMillis < priorClock.elapsedMillis)) {
                // pending() has already enforced actual custody/reconfirmation.
                // Rebase cadence once without inventing authority or shortening
                // the previously stored delay/Retry-After or incrementing attempts.
                val rebased = QueueRetryState(retry.attempts, capturedClock, retry.delayMillis)
                attempt.assertCurrent()
                if (queue.retry(attempt, operation, retry, rebased)) {
                    retry = rebased
                    byId[operation.operationId] = EnterpriseQueueEntry(operation, retry)
                } else {
                    val current = currentEntry(operation)
                    if (current.acknowledgement != null) { completed.add(operation.operationId); continue }
                    retry = current.retry
                }
                attempt.assertCurrent()
            }
            if (!retry.isDue(capturedClock)) {
                remainingDelay[operation.operationId] = delayFor(retry, capturedClock)
                continue
            }
            remainingDelay[operation.operationId] = 0
            if (selected.size == limits.maxOperations) continue
            val candidate = selected + operation
            try {
                account.proofBudget.batchBytes(candidate, limits)
                selected.add(operation)
            } catch (_: SyncProtocolException) {
                if (selected.isEmpty()) { attention.add(operation.operationId); remainingDelay.remove(operation.operationId) }
            }
        }
        if (selected.isEmpty()) return@guarded result(EnterpriseQueueDispatchResult(completed = completed.size, attention = attention.size,
            retryable = entries.size - attention.size - completed.size, retryAfterMillis = remainingDelay.values.minOrNull()))

        val ticket = activity?.begin(attempt, selected.map { EnterpriseOperationIdentity(it.operationId, it.payloadDigest) })
        try {
        suspend fun retry(operation: EnterpriseOperation, floor: Long?, knownFinal: OperationAcknowledgement?): Long? {
            attempt.assertCurrent()
            val old = byId.getValue(operation.operationId).retry
            val count = retryPolicy.nextAttempts(old.attempts)
            val delay = retryPolicy.delayMillis(count, entropy, floor)
            val saved = queue.retry(attempt, operation, old, QueueRetryState(count, clock(), delay))
            attempt.assertCurrent()
            if (saved) return delay
            val current = currentEntry(operation)
            val receipt = current.acknowledgement
            if (receipt != null) {
                if (knownFinal != null && !knownFinal.canonicalBytes.contentEquals(receipt.canonicalBytes))
                    throw EnterprisePersistenceUnavailable()
                completed.add(operation.operationId)
                return null
            }
            return delayFor(current.retry, clock())
        }
        suspend fun retainForRetry(operation: EnterpriseOperation, floor: Long?, knownFinal: OperationAcknowledgement? = null) {
            try {
                val delay = retry(operation, floor, knownFinal)
                if (delay == null) remainingDelay.remove(operation.operationId) else remainingDelay[operation.operationId] = delay
            }
            catch (cancelled: CancellationException) { throw cancelled }
            catch (denied: EnterpriseLocalDenied) { throw denied }
            catch (_: Exception) {
                // The original committed entry remains. A failed retry-metadata
                // write cannot turn uncertain delivery into a final result.
                remainingDelay[operation.operationId] = floor ?: retryPolicy.baseMillis
            }
        }
        var floor: Long? = null
        val acknowledgements = try {
            attempt.assertCurrent()
            val delivery = transport.deliver(attempt, selected.toList())
            attempt.assertCurrent()
            floor = delivery.retryAfterMillis
            val values = delivery.acknowledgements
            if (values.size != selected.size) throw EnterprisePersistenceUnavailable()
            // Even an alternative transport must preflight the complete answer.
            values.mapIndexed { index, ack -> OperationAcknowledgement.parse(ack.canonicalBytes, selected[index], limits) }
        } catch (cancelled: CancellationException) { throw cancelled }
        catch (denied: EnterpriseLocalDenied) { throw denied }
        catch (failure: Exception) {
            attempt.assertCurrent()
            if (failure is EnterpriseQueueDeliveryFailure) floor = failure.retryAfterMillis
            for (operation in selected) retainForRetry(operation, floor)
            return@guarded result(EnterpriseQueueDispatchResult(selected.size, completed.size, retryable = entries.size - attention.size - completed.size,
                attention = attention.size, retryAfterMillis = remainingDelay.values.minOrNull()))
        }
        for ((index, ack) in acknowledgements.withIndex()) {
            val operation = selected[index]
            attempt.assertCurrent()
            if (!ack.isFinal) { retainForRetry(operation, floor); continue }
            try {
                val producer = registry.producer(operation) ?: throw EnterprisePersistenceUnavailable()
                queue.acknowledge(attempt, operation, ack) { tx ->
                    attempt.assertCurrent(); producer.acknowledge(tx, attempt, operation, ack); attempt.assertCurrent()
                }
                attempt.assertCurrent()
                completed.add(operation.operationId)
                remainingDelay.remove(operation.operationId)
            } catch (cancelled: CancellationException) { throw cancelled }
            catch (denied: EnterpriseLocalDenied) { throw denied }
            catch (_: Exception) { retainForRetry(operation, floor, ack) }
        }
        result(EnterpriseQueueDispatchResult(selected.size, completed.size, entries.size - attention.size - completed.size,
            attention.size, remainingDelay.values.minOrNull()))
        } finally {
            // Publication/retry processing is finished before an observer refreshes.
            // A retiring attempt can remove only its own progress ticket.
            activity?.end(ticket)
        }
    }
}
