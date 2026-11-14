package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueDispatchResult
import com.anpfuel.application.enterprise.queue.EnterpriseQueueDeliveryFailure
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.application.enterprise.queue.EnterpriseReviewPollResult
import com.anpfuel.domain.enterprise.EnterprisePartition
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.yield

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
) {
    val needsRetry: Boolean get() = failed > 0 || retryable > 0 || coalesced
    override fun toString(): String = "EnterpriseOperationWorkResult(redacted)"
}

/** Every invocation owns its source lease and opaque cursor; the UI owns neither. */
class EnterpriseOperationWorkRunner(
    private val source: EnterpriseQueueSource,
    private val discover: suspend (EnterpriseQueueAccountLease, String?, Int) -> EnterpriseQueueDiscoveryPage,
    private val dispatch: suspend (EnterpriseQueueAccountLease, EnterprisePartition) -> EnterpriseQueueDispatchResult,
    private val review: (suspend (EnterpriseQueueAccountLease, EnterprisePartition) -> EnterpriseReviewPollResult)? = null,
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
        fun observeRetryAfter(value: Long?) {
            if (value != null && value >= 0) retryAfter = maxOf(retryAfter ?: 0, value)
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
                val enrollments = page.enrollments
                if (enrollments.size > 50) throw EnterpriseQueueDeliveryFailure()
                for (enrollment in enrollments) {
                    currentCoroutineContext().ensureActive()
                    account.assertCurrent()
                    val partition = enrollment.target.partition
                    if (!account.identity.owns(partition)) {
                        failed = add(failed, 1)
                        continue
                    }
                    if (!partitions.add(partition)) continue
                    try {
                        val result = dispatch(account, partition)
                        account.assertCurrent()
                        val candidateDelay = result.retryAfterMillis
                        require(result.attempted >= 0 && result.completed >= 0 && result.retryable >= 0 && result.attention >= 0 &&
                            (candidateDelay == null || candidateDelay >= 0))
                        attempted = add(attempted, result.attempted)
                        completed = add(completed, result.completed)
                        retryable = add(retryable, result.retryable)
                        attention = add(attention, result.attention)
                        observeRetryAfter(candidateDelay)
                    } catch (cancelled: CancellationException) {
                        throw cancelled
                    } catch (failure: EnterpriseQueueDeliveryFailure) {
                        account.assertCurrent()
                        failed = add(failed, 1)
                        observeRetryAfter(failure.retryAfterMillis)
                    } catch (_: Exception) {
                        // A paused organization never widens to the next account.
                        account.assertCurrent()
                        failed = add(failed, 1)
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
                    yield()
                }
                account.assertCurrent()
                val next = page.nextAfterOpaque
                if (next != null && (!OPAQUE.matches(next) || !cursors.add(next))) throw EnterpriseQueueDeliveryFailure()
                after = next
                yield()
            } while (after != null)
            account.assertCurrent()
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (failure: EnterpriseQueueDeliveryFailure) {
            failed = add(failed, 1)
            observeRetryAfter(failure.retryAfterMillis)
        } catch (_: Exception) {
            failed = add(failed, 1)
        } finally {
            invocation.unlock()
        }
        return EnterpriseOperationWorkResult(attempted, completed, retryable, attention, failed, retryAfter,
            reviewResolved = reviewResolved, reviewUnresolved = reviewUnresolved, reviewFailed = reviewFailed)
    }
    override fun toString(): String = "EnterpriseOperationWorkRunner(redacted)"

    private fun add(left: Int, right: Int): Int {
        require(left >= 0 && right >= 0)
        return (left.toLong() + right.toLong()).coerceAtMost(Int.MAX_VALUE.toLong()).toInt()
    }
    companion object { private val OPAQUE = Regex("[0-9a-f]{64}-[0-9a-f]{64}") }
}
