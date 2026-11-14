package com.anpfuel.application.enterprise.queue

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.sync.EnterpriseDeliveryActivity
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.sync.*
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive

/**
 * Review-only protected queue port. Operation delivery ports stay unchanged:
 * a final REVIEW is never resent, and its original acknowledgement is never rewritten.
 */
interface EnterpriseReviewQueue {
    /** Retained final REVIEW entries without a resolution; these are never pending delivery. */
    suspend fun unresolvedReviews(attempt: EnterpriseQueueAttempt): List<EnterpriseQueueEntry>
    /**
     * Compare-and-set against the exact retained original REVIEW bytes. Returns true when the
     * terminal resolution was recorded, false for an equal replay. A divergent terminal fact,
     * changed intent or a non-REVIEW receipt is unavailable and writes nothing.
     */
    suspend fun resolve(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        acknowledgement: OperationAcknowledgement, resolution: ReviewResolution): Boolean
    /** Structural CAS on review retry metadata only; false preserves newer state or a recorded resolution. */
    suspend fun reviewRetry(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        acknowledgement: OperationAcknowledgement, expected: QueueRetryState, next: QueueRetryState): Boolean
}

class EnterpriseReviewDelivery(results: List<ReviewResult>, val retryAfterMillis: Long? = null) {
    private val values = results.toList()
    val results: List<ReviewResult> get() = values.toList()
    init { require(retryAfterMillis == null || retryAfterMillis >= 0) }
    override fun toString(): String = "EnterpriseReviewDelivery(redacted)"
}

/** Signed own-result lookup. Implementations return only a fully parsed and correlated response. */
fun interface EnterpriseReviewTransport {
    suspend fun results(attempt: EnterpriseQueueAttempt, expected: List<ReviewExpectation>): EnterpriseReviewDelivery
}

/** Aggregate counts only; separate from operation delivery results. */
class EnterpriseReviewPollResult(val polled: Int = 0, val resolved: Int = 0, val unresolved: Int = 0,
    val waiting: Int = 0, val failed: Int = 0, val retryAfterMillis: Long? = null) {
    init { require(polled >= 0 && resolved >= 0 && unresolved >= 0 && waiting >= 0 && failed >= 0 && (retryAfterMillis == null || retryAfterMillis >= 0)) }
    override fun toString(): String = "EnterpriseReviewPollResult(redacted)"
}

/**
 * Polls retained unresolved REVIEW work under a fresh exact-source attempt; never marks anything as sending.
 * Every slot of an answer is correlated before any resolution is written. A failed or uncorrelated chunk
 * writes no resolution and advances only that chunk's separate review retry state.
 */
class EnterpriseReviewService(private val source: EnterpriseQueueSource, private val queue: EnterpriseReviewQueue,
    private val transport: EnterpriseReviewTransport, private val clock: () -> EnterpriseDeviceClock,
    private val entropy: (Long) -> Long, private val retryPolicy: QueueRetryPolicy = QueueRetryPolicy(),
    private val activity: EnterpriseDeliveryActivity? = null, private val limits: SyncLimits = SyncLimits.DEFAULT) {
    private suspend fun <T> guarded(work: suspend () -> T): T = try { work() }
    catch (cancelled: CancellationException) { throw cancelled }
    catch (denied: EnterpriseLocalDenied) { throw denied }
    catch (reused: EnterpriseOperationReused) { throw reused }
    catch (unlock: EnterpriseUnlockRequired) { throw unlock }
    catch (lost: EnterpriseVaultUnrecoverable) { throw lost }
    catch (_: Exception) { throw EnterprisePersistenceUnavailable() }

    private fun matches(attempt: EnterpriseQueueAttempt, entry: EnterpriseQueueEntry) {
        val operation = entry.operation
        val partition = attempt.partition
        if (operation.binding.ownerAccountId != partition.ownerAccount || operation.binding.apiOrigin != partition.origin ||
            operation.binding.organizationId != partition.organizationId || operation.membershipId != attempt.enrollment.target.membershipId ||
            operation.enrollmentId != attempt.enrollment.enrollmentId) throw EnterpriseLocalDenied()
        EnterpriseOperation.parse(operation.canonicalBytes, operation.binding, limits)
        val receipt = entry.acknowledgement ?: throw EnterprisePersistenceUnavailable()
        val reparsed = OperationAcknowledgement.parse(receipt.canonicalBytes, operation, limits)
        if (!entry.unresolvedReview || reparsed.result != SyncResult.ACCEPTED_FOR_REVIEW || reparsed.reviewItemId == null)
            throw EnterprisePersistenceUnavailable()
    }

    private fun delayFor(retry: QueueRetryState, now: EnterpriseDeviceClock): Long {
        val at = retry.clock ?: return 0
        if (at.boot != now.boot || now.elapsedMillis < at.elapsedMillis) return retry.delayMillis
        return maxOf(0, retry.delayMillis - (now.elapsedMillis - at.elapsedMillis))
    }

    /** Complete-answer preflight: one ordered slot per expectation and a valid terminal entry per resolution. */
    private fun preflight(chunk: List<EnterpriseQueueEntry>, expected: List<ReviewExpectation>,
        delivery: EnterpriseReviewDelivery): List<ReviewResolution?> {
        val results = delivery.results
        if (results.size != chunk.size) throw EnterprisePersistenceUnavailable()
        return results.mapIndexed { index, result ->
            if (result.selector != expected[index].selector) throw EnterprisePersistenceUnavailable()
            val supplied = result.resolution ?: return@mapIndexed null
            val raw = supplied.canonicalBytes
            val resolution = ReviewResolution.parse(raw)
            val decided = resolution.snapshot()
            val entry = chunk[index]
            if (!resolution.canonicalBytes.contentEquals(raw) || decided.operationId != entry.operation.operationId ||
                decided.payloadDigest != entry.operation.payloadDigest || decided.reviewItemId != expected[index].reviewItemId)
                throw EnterprisePersistenceUnavailable()
            // Validates the original ReviewID and nondecreasing server time against the retained receipt.
            EnterpriseQueueEntry(entry.operation, entry.retry, entry.acknowledgement, false, resolution)
            resolution
        }
    }

    suspend fun poll(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseReviewPollResult = guarded {
        account.assertCurrent()
        if (!account.identity.owns(partition)) throw EnterpriseLocalDenied()
        val attempt = source.attempt(account, partition)
        if (attempt.account !== account || attempt.partition != partition) throw EnterpriseLocalDenied()
        attempt.assertCurrent()
        val entries = queue.unresolvedReviews(attempt).toList()
        attempt.assertCurrent()
        if (entries.size > 10000 || entries.map { it.operation.operationId }.distinct().size != entries.size)
            throw EnterprisePersistenceUnavailable()
        entries.forEach { matches(attempt, it) }
        val captured = clock()
        val due = mutableListOf<EnterpriseQueueEntry>()
        val remaining = mutableMapOf<String, Long>()
        var waiting = 0
        for (entry in entries) {
            var retry = entry.reviewRetry
            val prior = retry.clock
            if (prior != null && (prior.boot != captured.boot || captured.elapsedMillis < prior.elapsedMillis)) {
                // Rebase once after a reboot without shortening the stored floor or counting an attempt.
                val rebased = QueueRetryState(retry.attempts, captured, retry.delayMillis)
                attempt.assertCurrent()
                val saved = try { queue.reviewRetry(attempt, entry.operation, entry.acknowledgement!!, retry, rebased) }
                catch (cancelled: CancellationException) { throw cancelled }
                catch (denied: EnterpriseLocalDenied) { throw denied }
                catch (_: Exception) { false }
                attempt.assertCurrent()
                if (!saved) {
                    // Another writer changed it, or the metadata write failed; the stored floor
                    // is kept and handled on a later turn without blocking other entries.
                    waiting++; remaining[entry.operation.operationId] = retry.delayMillis; continue
                }
                retry = rebased
            }
            if (!retry.isDue(captured)) { waiting++; remaining[entry.operation.operationId] = delayFor(retry, captured); continue }
            due += if (retry === entry.reviewRetry) entry
                else EnterpriseQueueEntry(entry.operation, entry.retry, entry.acknowledgement, reviewRetry = retry)
        }
        var polled = 0; var resolved = 0; var unresolved = 0; var failed = 0
        var changed = false
        suspend fun retainForRetry(entry: EnterpriseQueueEntry, floor: Long?) {
            val id = entry.operation.operationId
            try {
                attempt.assertCurrent()
                val old = entry.reviewRetry
                val count = retryPolicy.nextAttempts(old.attempts)
                val delay = retryPolicy.delayMillis(count, entropy, floor)
                val saved = queue.reviewRetry(attempt, entry.operation, entry.acknowledgement!!, old, QueueRetryState(count, clock(), delay))
                attempt.assertCurrent()
                remaining[id] = if (saved) delay else floor ?: retryPolicy.baseMillis
            }
            catch (cancelled: CancellationException) { throw cancelled }
            catch (denied: EnterpriseLocalDenied) { throw denied }
            catch (_: Exception) {
                // The original committed entry remains unresolved; a failed metadata write
                // never turns an unknown answer into a decision.
                remaining[id] = floor ?: retryPolicy.baseMillis
            }
        }
        try {
            for (chunk in due.chunked(ReviewResultsRequest.MAX_SELECTORS)) {
                currentCoroutineContext().ensureActive()
                attempt.assertCurrent()
                val expected = chunk.map {
                    ReviewExpectation(ReviewSelector(it.operation.operationId, it.operation.payloadDigest), it.acknowledgement!!.reviewItemId!!)
                }
                polled += chunk.size
                var pacing: Long? = null
                val decisions = try {
                    val delivery = transport.results(attempt, expected.toList())
                    attempt.assertCurrent()
                    pacing = delivery.retryAfterMillis?.takeIf { it > 0 }
                    preflight(chunk, expected, delivery)
                } catch (cancelled: CancellationException) { throw cancelled }
                catch (denied: EnterpriseLocalDenied) { throw denied }
                catch (failure: Exception) {
                    attempt.assertCurrent()
                    val floor = (failure as? EnterpriseQueueDeliveryFailure)?.retryAfterMillis
                    for (entry in chunk) retainForRetry(entry, floor)
                    failed += chunk.size
                    break // A failing source is not asked again for later chunks in this turn.
                }
                for ((index, entry) in chunk.withIndex()) {
                    attempt.assertCurrent()
                    val resolution = decisions[index]
                    val id = entry.operation.operationId
                    if (resolution != null) {
                        try {
                            if (queue.resolve(attempt, entry.operation, entry.acknowledgement!!, resolution)) changed = true
                            resolved++
                        } catch (cancelled: CancellationException) { throw cancelled }
                        catch (denied: EnterpriseLocalDenied) { throw denied }
                        catch (_: Exception) {
                            // A divergent stored decision stays closed; a failed durable write backs off
                            // only this entry and never starves the rest of the answer.
                            retainForRetry(entry, null)
                            failed++
                        }
                    } else {
                        // A successful unresolved answer ends this entry's failure backoff. An
                        // authenticated server pacing hint is kept as a floor without counting a failure.
                        val next = pacing?.let { QueueRetryState(0, clock(), it) } ?: QueueRetryState()
                        try {
                            if (!entry.reviewRetry.sameAs(next) &&
                                queue.reviewRetry(attempt, entry.operation, entry.acknowledgement!!, entry.reviewRetry, next)) changed = true
                        } catch (cancelled: CancellationException) { throw cancelled }
                        catch (denied: EnterpriseLocalDenied) { throw denied }
                        catch (_: Exception) { /* Metadata only; the next answer is not a decision either. */ }
                        pacing?.let { remaining[id] = it }
                        unresolved++
                    }
                    attempt.assertCurrent()
                }
            }
        } finally {
            // Durable scoped completion is only a reread hint for a current observer.
            if (changed) runCatching { activity?.completed(attempt) }
        }
        currentCoroutineContext().ensureActive()
        attempt.assertCurrent()
        EnterpriseReviewPollResult(polled, resolved, unresolved, waiting, failed, remaining.values.minOrNull())
    }
}
