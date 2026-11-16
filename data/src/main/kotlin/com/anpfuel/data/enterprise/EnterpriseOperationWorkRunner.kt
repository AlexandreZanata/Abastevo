package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.attachment.EnterpriseAttachmentDeliveryResult
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueDispatchResult
import com.anpfuel.application.enterprise.queue.EnterpriseQueueDeliveryFailure
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.application.enterprise.queue.EnterpriseReviewPollResult
import com.anpfuel.application.enterprise.queue.EnterpriseRevocationOutcome
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.sync.RevocationCleanupState
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.yield

/** What the system worker does after one bounded invocation; it carries no private selector. */
sealed interface EnterpriseOperationWorkNext {
    /** Nothing actionable remains now (or nobody is signed in): no reschedule. */
    data object Done : EnterpriseOperationWorkNext
    /** An unknown failure or stalled source: ordinary system backoff. */
    data object Backoff : EnterpriseOperationWorkNext
    /** Retained work is due after a known delay, honoring stored backoff and server Retry-After. */
    class Continue(delayMillis: Long) : EnterpriseOperationWorkNext {
        /** Clamped to the documented Retry-After bound (EDR-122); never negative. */
        val delayMillis: Long = delayMillis.coerceIn(0, EnterpriseRetryAfter.MAX_MILLIS)
        override fun equals(other: Any?): Boolean = other is Continue && other.delayMillis == delayMillis
        override fun hashCode(): Int = delayMillis.hashCode()
        override fun toString(): String = "Continue($delayMillis)"
    }
}

/** Only aggregate counts leave a protected worker invocation. */
class EnterpriseOperationWorkResult(
    val attempted: Int = 0,
    val completed: Int = 0,
    val retryable: Int = 0,
    val attention: Int = 0,
    val failed: Int = 0,
    val retryAfterMillis: Long? = null,
    val coalesced: Boolean = false,
    /** Review result lookups keep separate counts; they never decide delivery retry. */
    val reviewResolved: Int = 0,
    val reviewUnresolved: Int = 0,
    val reviewFailed: Int = 0,
    /** No personal account session exists; background delivery goes idle (EDR-122). */
    val signedOut: Boolean = false,
    /** Earliest known delay before retained delivery work is due again; scheduling only. */
    val nextAttemptMillis: Long? = null,
    /** A failure without an authenticated floor or a stalled batch; use system backoff. */
    val backoff: Boolean = false,
    /** Attachment uploads keep separate counts (EMP-OFFL-005); they never decide operation delivery. */
    val attachmentsStored: Int = 0,
    val attachmentsPending: Int = 0,
    val attachmentsFailed: Int = 0,
    /** EMP-OFFL-011 revocation steps (EDR-348..350): locked and still delivering, erasure unverified, erased. */
    val revocationLocked: Int = 0,
    val revocationPending: Int = 0,
    val revocationCleaned: Int = 0,
    val revocationFailed: Int = 0,
) {
    init { require(nextAttemptMillis == null || nextAttemptMillis >= 0) }

    val next: EnterpriseOperationWorkNext get() = when {
        signedOut -> EnterpriseOperationWorkNext.Done
        coalesced || backoff -> EnterpriseOperationWorkNext.Backoff
        nextAttemptMillis != null -> EnterpriseOperationWorkNext.Continue(nextAttemptMillis)
        failed > 0 || retryable > 0 -> EnterpriseOperationWorkNext.Backoff
        else -> EnterpriseOperationWorkNext.Done
    }
    /** Another invocation is needed, either by system backoff or a delayed continuation. */
    val needsRetry: Boolean get() = next != EnterpriseOperationWorkNext.Done
    override fun toString(): String = "EnterpriseOperationWorkResult(redacted)"
}

/** Every invocation owns its source lease and opaque cursor; the UI owns neither. */
class EnterpriseOperationWorkRunner(
    private val source: EnterpriseQueueSource,
    private val discover: suspend (EnterpriseQueueAccountLease, String?, Int) -> EnterpriseQueueDiscoveryPage,
    private val dispatch: suspend (EnterpriseQueueAccountLease, EnterprisePartition) -> EnterpriseQueueDispatchResult,
    private val review: (suspend (EnterpriseQueueAccountLease, EnterprisePartition) -> EnterpriseReviewPollResult)? = null,
    /** EMP-OFFL-005: attachments of the partition, after its operations and review lookups (EDR-268). */
    private val attachments: (suspend (EnterpriseQueueAccountLease, EnterprisePartition) -> EnterpriseAttachmentDeliveryResult)? = null,
    /** EMP-OFFL-011: after each partition's delivery, with whether it saw a revocation hint (EDR-348). */
    private val cleanup: (suspend (EnterpriseQueueAccountLease, EnterprisePartition, Boolean) -> EnterpriseRevocationOutcome)? = null,
) {
    private val invocation = Mutex()

    suspend fun runOnce(): EnterpriseOperationWorkResult {
        if (!invocation.tryLock()) return EnterpriseOperationWorkResult(coalesced = true)
        var attempted = 0
        var completed = 0
        var retryable = 0
        var attention = 0
        var failed = 0
        var retryAfter: Long? = null
        var reviewResolved = 0
        var reviewUnresolved = 0
        var reviewFailed = 0
        var signedOut = false
        var attachmentsStored = 0
        var attachmentsPending = 0
        var attachmentsFailed = 0
        var nextAttempt: Long? = null
        var backoff = false
        var revocationLocked = 0
        var revocationPending = 0
        var revocationCleaned = 0
        var revocationFailed = 0
        var resumed = false
        fun observeRetryAfter(value: Long?) {
            if (value != null && value >= 0) retryAfter = maxOf(retryAfter ?: 0, value)
        }
        fun observeNext(value: Long) { nextAttempt = minOf(nextAttempt ?: value, value) }
        /** An authenticated server floor schedules a continuation; anything else uses system backoff. */
        fun observeFailure(floor: Long?) {
            failed = add(failed, 1)
            if (floor != null && floor >= 0) { observeRetryAfter(floor); observeNext(floor) } else backoff = true
        }
        try {
            currentCoroutineContext().ensureActive()
            val account = source.deliveryAccount()
            account.assertCurrent()
            // No cursor or mutable sweep belongs to the singleton runner.
            var after: String? = null
            val cursors = mutableSetOf<String>()
            val partitions = mutableSetOf<EnterprisePartition>()
            do {
                currentCoroutineContext().ensureActive()
                account.assertCurrent()
                val page = discover(account, after, 50)
                account.assertCurrent()
                failed = add(failed, page.unavailable)
                if (page.unavailable > 0) backoff = true
                // Waiting for the person's reconfirmation is attention, not a worker failure.
                attention = add(attention, page.contactRequired)
                val enrollments = page.enrollments
                if (enrollments.size > 50) throw EnterpriseQueueDeliveryFailure()
                for (enrollment in enrollments) {
                    currentCoroutineContext().ensureActive()
                    account.assertCurrent()
                    var hinted = false
                    val partition = enrollment.target.partition
                    if (!account.identity.owns(partition)) {
                        failed = add(failed, 1)
                        backoff = true
                        continue
                    }
                    if (!partitions.add(partition)) continue
                    try {
                        // A fully acknowledged batch that left more due work sends the next page now,
                        // bounded per invocation; a failed or partial batch never asks for more.
                        var rounds = 0
                        var drained: Boolean
                        var last: EnterpriseQueueDispatchResult
                        do {
                            currentCoroutineContext().ensureActive()
                            account.assertCurrent()
                            last = dispatch(account, partition)
                            account.assertCurrent()
                            hinted = hinted || last.revocationHint
                            val candidateDelay = last.retryAfterMillis
                            require(last.attempted >= 0 && last.completed >= 0 && last.retryable >= 0 && last.attention >= 0 &&
                                (candidateDelay == null || candidateDelay >= 0))
                            attempted = add(attempted, last.attempted)
                            completed = add(completed, last.completed)
                            rounds++
                            drained = candidateDelay == 0L && last.attempted > 0 && last.completed >= last.attempted
                            if (drained && rounds < MAX_DISPATCH_ROUNDS) yield()
                        } while (drained && rounds < MAX_DISPATCH_ROUNDS)
                        retryable = add(retryable, last.retryable)
                        attention = add(attention, last.attention)
                        observeRetryAfter(last.retryAfterMillis)
                        when (val delay = last.retryAfterMillis) {
                            null -> Unit
                            // Due work left by a failed/partial batch must not be hammered immediately.
                            0L -> if (drained) observeNext(0) else backoff = true
                            else -> observeNext(delay)
                        }
                    } catch (cancelled: CancellationException) {
                        throw cancelled
                    } catch (failure: EnterpriseQueueDeliveryFailure) {
                        account.assertCurrent()
                        hinted = hinted || failure.revocationHint
                        observeFailure(failure.retryAfterMillis)
                    } catch (_: Exception) {
                        // A paused organization never widens to the next account.
                        account.assertCurrent()
                        observeFailure(null)
                    }
                    val clean = cleanup
                    if (clean != null) {
                        // EMP-OFFL-011 (EDR-348..350): only counts leave; a revoked partition gets no review lookup or upload.
                        var revoked: EnterpriseRevocationOutcome? = null
                        try {
                            currentCoroutineContext().ensureActive()
                            account.assertCurrent()
                            revoked = clean(account, partition, hinted)
                        } catch (cancelled: CancellationException) {
                            throw cancelled
                        } catch (_: Exception) {
                            revocationFailed = add(revocationFailed, 1)
                        }
                        when (revoked?.state) {
                            RevocationCleanupState.INTAKE_ONLY -> revocationLocked = add(revocationLocked, 1)
                            RevocationCleanupState.CLEANUP_PENDING -> revocationPending = add(revocationPending, 1)
                            RevocationCleanupState.CLEANED -> revocationCleaned = add(revocationCleaned, 1)
                            null -> Unit
                        }
                        // A new lock or erasure withdrew this invocation's own lease: continue at once with a fresh one.
                        if (!runCatching { account.assertCurrent() }.isSuccess) { resumed = true; observeNext(0); break }
                        if (revoked?.state != null) { yield(); continue }
                    }
                    val lookup = review
                    if (lookup != null) {
                        // Review-only work is polled after delivery under its own fresh attempt, even
                        // when nothing was pending. Its counts never alter delivery retry decisions.
                        try {
                            currentCoroutineContext().ensureActive()
                            account.assertCurrent()
                            val reviewed = lookup(account, partition)
                            account.assertCurrent()
                            reviewResolved = add(reviewResolved, reviewed.resolved)
                            reviewUnresolved = add(reviewUnresolved, reviewed.unresolved)
                            reviewFailed = add(reviewFailed, reviewed.failed)
                        } catch (cancelled: CancellationException) {
                            throw cancelled
                        } catch (_: Exception) {
                            account.assertCurrent()
                            reviewFailed = add(reviewFailed, 1)
                        }
                    }
                    val upload = attachments
                    if (upload != null) {
                        // Attachments follow their operations under the same account lease; their counts and
                        // failures never alter operation delivery, only a known due time schedules a continuation.
                        try {
                            currentCoroutineContext().ensureActive()
                            account.assertCurrent()
                            val delivered = upload(account, partition)
                            account.assertCurrent()
                            require(delivered.attempted >= 0 && delivered.stored >= 0 && delivered.pending >= 0)
                            attachmentsStored = add(attachmentsStored, delivered.stored)
                            attachmentsPending = add(attachmentsPending, delivered.pending)
                            delivered.nextAttemptMillis?.let { observeNext(it.coerceIn(0, EnterpriseRetryAfter.MAX_MILLIS)) }
                        } catch (cancelled: CancellationException) {
                            throw cancelled
                        } catch (_: Exception) {
                            account.assertCurrent()
                            attachmentsFailed = add(attachmentsFailed, 1)
                        }
                    }
                    yield()
                }
                if (resumed) break
                account.assertCurrent()
                val next = page.nextAfterOpaque
                if (next != null && (!OPAQUE.matches(next) || !cursors.add(next))) throw EnterpriseQueueDeliveryFailure()
                after = next
                yield()
            } while (after != null)
            if (!resumed) account.assertCurrent()
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (failure: EnterpriseQueueDeliveryFailure) {
            observeFailure(failure.retryAfterMillis)
        } catch (denied: EnterpriseLocalDenied) {
            // Signed out: nothing can be delivered until a session exists again. The periodic
            // work and the next app start resume delivery; no backoff loop runs meanwhile.
            if (!source.hasSession()) signedOut = true else observeFailure(null)
        } catch (_: Exception) {
            observeFailure(null)
        } finally {
            invocation.unlock()
        }
        return EnterpriseOperationWorkResult(attempted, completed, retryable, attention, failed, retryAfter,
            reviewResolved = reviewResolved, reviewUnresolved = reviewUnresolved, reviewFailed = reviewFailed,
            signedOut = signedOut, nextAttemptMillis = nextAttempt, backoff = backoff,
            attachmentsStored = attachmentsStored, attachmentsPending = attachmentsPending, attachmentsFailed = attachmentsFailed,
            revocationLocked = revocationLocked, revocationPending = revocationPending, revocationCleaned = revocationCleaned,
            revocationFailed = revocationFailed)
    }
    override fun toString(): String = "EnterpriseOperationWorkRunner(redacted)"

    private fun add(left: Int, right: Int): Int {
        require(left >= 0 && right >= 0)
        return (left.toLong() + right.toLong()).coerceAtMost(Int.MAX_VALUE.toLong()).toInt()
    }
    companion object {
        private val OPAQUE = Regex("[0-9a-f]{64}-[0-9a-f]{64}")
        /** At most 10 batches (500 operations at the frozen 50-operation limit) per partition per invocation. */
        internal const val MAX_DISPATCH_ROUNDS = 10
    }
}
