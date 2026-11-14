package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.enterprise.sync.EnterpriseDeliveryActivity
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.sync.*
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseReviewServiceTest {
    init {
        val tests = EnterpriseReviewServiceTest::class.java.declaredMethods.filter { it.isAnnotationPresent(Test::class.java) }
        check(tests.size == 9 && tests.all { it.returnType == Void.TYPE }) { "Review service tests must be discoverable void methods" }
    }

    private fun id(n: Int) = "7d000000-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val partition = EnterprisePartition.of(id(1), "https://review.example.invalid", id(3))
    private fun op(n: Int): EnterpriseOperation = EnterpriseOperation.seal("""{"schema_version":1,"operation_id":"${id(n)}","owner_account_id":"${id(1)}","api_origin":"${partition.origin}","org_id":"${id(3)}","membership_id":"${id(4)}","enrollment_id":"${id(5)}","type":"validation.review","operation_schema":1,"entity":{"type":"record","id":"${id(100)}","expected_revision":null},"occurred_at":"2026-11-14T00:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[],"payload":{"private":"SYNTHETIC-REVIEW-FACT-$n"}}""".toByteArray(), SyncBinding.of(id(1), partition.origin, id(3)))
    private fun reviewId(n: Int) = id(5000 + n)
    private fun ack(op: EnterpriseOperation, n: Int, result: String = "ACCEPTED_FOR_REVIEW"): OperationAcknowledgement {
        val review = if (result == "ACCEPTED_FOR_REVIEW") "\"${reviewId(n)}\"" else "null"
        val code = if (result == "ACCEPTED_FOR_REVIEW") "\"PERMISSION_CHANGED\"" else "null"
        val revision = if (result == "ACCEPTED") "1" else "null"
        return OperationAcknowledgement.parse("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}","result":"$result","code":$code,"entity_revision":$revision,"received_at":"2026-11-14T00:01:00Z","review_item_id":$review,"warnings":[]}""".toByteArray(), op)
    }
    private fun resolution(op: EnterpriseOperation, n: Int, outcome: String = "ACCEPTED", review: String = reviewId(n),
        at: String = "2026-11-14T00:02:00Z") = ReviewResolution.create(ReviewResolutionSnapshot(op.operationId, op.payloadDigest, review,
        id(7000 + n), 2, outcome, if (outcome == "REFUSED") "Synthetic refusal" else "", at, if (outcome == "ACCEPTED") 3 else null))

    private inner class Fixture(reviews: Int = 2) {
        var retained = true
        var clockNow = EnterpriseDeviceClock("boot", 100_000, 1_000_000)
        val account = EnterpriseQueueAccountLease.capture(EnterpriseLocalIdentity(id(1), partition.origin, id(8)), 2, 3, id(9)) { retained }
        var attempts = 0
        val source = object : EnterpriseQueueSource {
            override suspend fun account() = account
            override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt {
                account.assertCurrent(); attempts++
                return account.forEnrollment(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(partition, id(4)), id(5), 1))
            }
        }
        val entries = linkedMapOf<String, EnterpriseQueueEntry>()
        val pendingDelivery = op(900)
        init {
            for (n in 1..reviews) { val o = op(n); entries[o.operationId] = EnterpriseQueueEntry(o, acknowledgement = ack(o, n)) }
            entries[pendingDelivery.operationId] = EnterpriseQueueEntry(pendingDelivery, QueueRetryState(2, clockNow, 60_000))
        }
        val resolveCalls = mutableListOf<String>()
        var failResolve: String? = null
        val reviewRetryCalls = mutableListOf<String>()
        var afterTransport: () -> Unit = {}
        var answer: (List<ReviewExpectation>) -> EnterpriseReviewDelivery = { expected -> EnterpriseReviewDelivery(expected.map { ReviewResult(it.selector, null) }) }
        val requests = mutableListOf<List<ReviewExpectation>>()
        val queue = object : EnterpriseReviewQueue {
            override suspend fun unresolvedReviews(attempt: EnterpriseQueueAttempt) = entries.values.filter { it.unresolvedReview }
            override suspend fun resolve(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, acknowledgement: OperationAcknowledgement,
                resolution: ReviewResolution): Boolean {
                attempt.assertCurrent(); resolveCalls += operation.operationId
                if (failResolve == operation.operationId) throw EnterprisePersistenceUnavailable()
                val old = entries.getValue(operation.operationId)
                require(old.acknowledgement!!.canonicalBytes.contentEquals(acknowledgement.canonicalBytes))
                old.resolution?.let { if (!it.canonicalBytes.contentEquals(resolution.canonicalBytes)) throw EnterprisePersistenceUnavailable(); return false }
                entries[operation.operationId] = EnterpriseQueueEntry(old.operation, old.retry, old.acknowledgement, resolution = resolution)
                return true
            }
            override suspend fun reviewRetry(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, acknowledgement: OperationAcknowledgement,
                expected: QueueRetryState, next: QueueRetryState): Boolean {
                attempt.assertCurrent(); reviewRetryCalls += operation.operationId
                val old = entries.getValue(operation.operationId)
                if (!old.unresolvedReview || !old.reviewRetry.sameAs(expected)) return false
                entries[operation.operationId] = EnterpriseQueueEntry(old.operation, old.retry, old.acknowledgement, reviewRetry = next)
                return true
            }
        }
        val transport = EnterpriseReviewTransport { attempt, expected ->
            attempt.assertCurrent(); requests += expected.toList()
            answer(expected).also { afterTransport() }
        }
        val activity = EnterpriseDeliveryActivity()
        val service = EnterpriseReviewService(source, queue, transport, { clockNow }, { 0L }, activity = activity)
        fun attempt() = account.forEnrollment(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(partition, id(4)), id(5), 1))
    }

    @Test fun completeCorrelatedAnswerRecordsSeparateResolutionWithoutSendingOrTouchingDelivery(): Unit = runTest {
        val f = Fixture(2); val first = op(1); val second = op(2)
        val originalAck = f.entries.getValue(first.operationId).acknowledgement!!.canonicalBytes
        val deliveryBefore = f.entries.getValue(f.pendingDelivery.operationId)
        val observer = f.attempt(); val revision = f.activity.completionRevision(observer)
        f.answer = { expected -> EnterpriseReviewDelivery(listOf(ReviewResult(expected[0].selector, resolution(first, 1)), ReviewResult(expected[1].selector, null))) }
        val result = f.service.poll(f.account, partition)
        assertEquals(2, result.polled); assertEquals(1, result.resolved); assertEquals(1, result.unresolved); assertEquals(0, result.failed)
        assertEquals(listOf(listOf(ReviewExpectation(ReviewSelector(first.operationId, first.payloadDigest), reviewId(1)),
            ReviewExpectation(ReviewSelector(second.operationId, second.payloadDigest), reviewId(2)))), f.requests)
        val resolved = f.entries.getValue(first.operationId)
        assertArrayEquals(originalAck, resolved.acknowledgement!!.canonicalBytes, "Original REVIEW receipt bytes changed")
        assertEquals(ReviewResolution.ACCEPTED, resolved.resolution!!.snapshot().outcome)
        assertSame(deliveryBefore, f.entries.getValue(f.pendingDelivery.operationId), "Review polling touched pending delivery")
        assertTrue(f.activity.current(observer).isEmpty(), "A result lookup must never present SENDING")
        assertTrue(f.activity.completedSince(observer, revision), "Durable resolution did not request a scoped reread")
        assertFalse(result.toString().contains(first.operationId))
    }

    @Test fun incompleteOrUncorrelatedAnswerWritesNothingAndAdvancesOnlyReviewRetry(): Unit = runTest {
        val first = op(1); val second = op(2)
        val bad: List<(List<ReviewExpectation>) -> EnterpriseReviewDelivery> = listOf(
            { e -> EnterpriseReviewDelivery(listOf(ReviewResult(e[0].selector, resolution(first, 1)))) },
            { e -> EnterpriseReviewDelivery(listOf(ReviewResult(e[1].selector, null), ReviewResult(e[0].selector, resolution(first, 1)))) },
            { e -> EnterpriseReviewDelivery(listOf(ReviewResult(e[0].selector, resolution(first, 1)), ReviewResult(e[1].selector, resolution(second, 2, review = id(99))))) },
            { e -> EnterpriseReviewDelivery(listOf(ReviewResult(e[0].selector, resolution(first, 1)), ReviewResult(e[1].selector, resolution(second, 2, at = "2026-11-14T00:00:59Z")))) },
            { e -> EnterpriseReviewDelivery(listOf(ReviewResult(e[0].selector, resolution(second, 2)), ReviewResult(e[1].selector, null))) },
        )
        for (answer in bad) {
            val f = Fixture(2); f.answer = answer
            val operationRetry = f.entries.getValue(f.pendingDelivery.operationId).retry
            val result = f.service.poll(f.account, partition)
            assertEquals(2, result.failed); assertEquals(0, result.resolved)
            assertTrue(f.resolveCalls.isEmpty(), "A valid prefix was published from an invalid answer")
            for (o in listOf(first, second)) {
                val e = f.entries.getValue(o.operationId)
                assertNull(e.resolution); assertEquals(1L, e.reviewRetry.attempts); assertEquals(15_000L, e.reviewRetry.delayMillis)
            }
            assertTrue(f.entries.getValue(f.pendingDelivery.operationId).retry.sameAs(operationRetry))
        }
    }

    @Test fun transportFailureKeepsRetryAfterFloorAndStopsLaterChunks(): Unit = runTest {
        val f = Fixture(60)
        f.answer = { throw EnterpriseQueueDeliveryFailure(retryAfterMillis = 120_000) }
        val result = f.service.poll(f.account, partition)
        assertEquals(1, f.requests.size); assertEquals(50, f.requests.single().size)
        assertEquals(50, result.failed); assertEquals(0, result.resolved); assertEquals(120_000L, result.retryAfterMillis)
        val retried = f.entries.values.filter { it.unresolvedReview && it.reviewRetry.attempts == 1L }
        assertEquals(50, retried.size); assertTrue(retried.all { it.reviewRetry.delayMillis == 120_000L && it.reviewRetry.clock == f.clockNow })
        assertEquals(10, f.entries.values.count { it.unresolvedReview && it.reviewRetry.initial }, "Later chunk was charged a failure")
        f.answer = { e -> EnterpriseReviewDelivery(e.map { ReviewResult(it.selector, null) }) }
        val second = f.service.poll(f.account, partition)
        assertEquals(10, second.polled); assertEquals(50, second.waiting); assertEquals(10, f.requests.last().size)
    }

    @Test fun notDueEntriesWaitAndSuccessfulUnresolvedAnswerResetsOnlyThatRetry(): Unit = runTest {
        val f = Fixture(1); val first = op(1)
        val old = f.entries.getValue(first.operationId)
        f.entries[first.operationId] = EnterpriseQueueEntry(first, old.retry, old.acknowledgement, reviewRetry = QueueRetryState(3, f.clockNow, 30_000))
        val waiting = f.service.poll(f.account, partition)
        assertEquals(0, waiting.polled); assertEquals(1, waiting.waiting); assertEquals(30_000L, waiting.retryAfterMillis); assertTrue(f.requests.isEmpty())
        f.clockNow = f.clockNow.copy(elapsedMillis = f.clockNow.elapsedMillis + 30_000, wallMillis = f.clockNow.wallMillis + 30_000)
        val due = f.service.poll(f.account, partition)
        assertEquals(1, due.polled); assertEquals(1, due.unresolved)
        assertTrue(f.entries.getValue(first.operationId).reviewRetry.initial)
        // A reboot cannot shorten the stored floor; it rebases once without counting an attempt.
        f.entries[first.operationId] = EnterpriseQueueEntry(first, old.retry, old.acknowledgement, reviewRetry = QueueRetryState(4, f.clockNow, 40_000))
        f.clockNow = EnterpriseDeviceClock("other-boot", 5, f.clockNow.wallMillis + 1)
        val rebooted = f.service.poll(f.account, partition)
        assertEquals(0, rebooted.polled); assertEquals(40_000L, rebooted.retryAfterMillis)
        val rebased = f.entries.getValue(first.operationId).reviewRetry
        assertEquals(4L, rebased.attempts); assertEquals(f.clockNow, rebased.clock); assertEquals(40_000L, rebased.delayMillis)
    }

    @Test fun emptyReviewQueueMakesNoRequestAndFinalOperationsAreNeverSelected(): Unit = runTest {
        val f = Fixture(0)
        val accepted = op(800); f.entries[accepted.operationId] = EnterpriseQueueEntry(accepted, acknowledgement = ack(accepted, 800, "ACCEPTED"))
        val result = f.service.poll(f.account, partition)
        assertEquals(0, result.polled); assertTrue(f.requests.isEmpty()); assertTrue(f.reviewRetryCalls.isEmpty())
    }

    @Test fun lateSourceLossWritesNothingAndDivergentStoredDecisionStaysClosed(): Unit = runTest {
        val f = Fixture(1); val first = op(1)
        f.answer = { e -> EnterpriseReviewDelivery(listOf(ReviewResult(e[0].selector, resolution(first, 1)))) }
        f.afterTransport = { f.retained = false }
        assertThrows(EnterpriseLocalDenied::class.java) { kotlinx.coroutines.runBlocking { f.service.poll(f.account, partition) } }
        assertTrue(f.resolveCalls.isEmpty()); assertNull(f.entries.getValue(first.operationId).resolution)
        val g = Fixture(1)
        val stored = g.entries.getValue(first.operationId)
        g.answer = { e -> EnterpriseReviewDelivery(listOf(ReviewResult(e[0].selector, resolution(first, 1, outcome = "REFUSED")))) }
        // Concurrent writer records the accepted decision after the list but before our CAS.
        g.afterTransport = { g.entries[first.operationId] = EnterpriseQueueEntry(first, stored.retry, stored.acknowledgement, resolution = resolution(first, 1)) }
        // Divergent terminal evidence is refused for that entry only (counted failed, never written).
        val divergent = g.service.poll(g.account, partition)
        assertEquals(1, divergent.failed); assertEquals(0, divergent.resolved)
        assertEquals(ReviewResolution.ACCEPTED, g.entries.getValue(first.operationId).resolution!!.snapshot().outcome)
        assertTrue(g.entries.getValue(first.operationId).reviewRetry.initial, "A resolved entry acquired retry metadata")
    }

    @Test fun equalConcurrentReplayIsHarmlessAndCountsAsResolved(): Unit = runTest {
        val f = Fixture(1); val first = op(1); val stored = f.entries.getValue(first.operationId)
        f.answer = { e -> EnterpriseReviewDelivery(listOf(ReviewResult(e[0].selector, resolution(first, 1)))) }
        f.afterTransport = { f.entries[first.operationId] = EnterpriseQueueEntry(first, stored.retry, stored.acknowledgement, resolution = resolution(first, 1)) }
        val result = f.service.poll(f.account, partition)
        assertEquals(1, result.resolved); assertEquals(0, result.failed)
        assertEquals(listOf(first.operationId), f.resolveCalls)
    }

    @Test fun oneFailedDurableWriteBacksOffOnlyThatEntryAndDoesNotStarveTheRest(): Unit = runTest {
        val f = Fixture(2); val first = op(1); val second = op(2)
        f.failResolve = first.operationId
        f.answer = { e -> EnterpriseReviewDelivery(listOf(ReviewResult(e[0].selector, resolution(first, 1)), ReviewResult(e[1].selector, resolution(second, 2)))) }
        val result = f.service.poll(f.account, partition)
        assertEquals(1, result.resolved); assertEquals(1, result.failed)
        assertEquals(ReviewResolution.ACCEPTED, f.entries.getValue(second.operationId).resolution!!.snapshot().outcome)
        val failed = f.entries.getValue(first.operationId)
        assertNull(failed.resolution); assertEquals(1L, failed.reviewRetry.attempts); assertEquals(15_000L, failed.reviewRetry.delayMillis)
    }

    @Test fun successfulUnresolvedAnswerHonorsAuthenticatedServerPacingWithoutCountingAFailure(): Unit = runTest {
        val f = Fixture(1); val first = op(1)
        f.answer = { e -> EnterpriseReviewDelivery(e.map { ReviewResult(it.selector, null) }, retryAfterMillis = 3_600_000) }
        val result = f.service.poll(f.account, partition)
        assertEquals(1, result.unresolved); assertEquals(0, result.failed)
        val paced = f.entries.getValue(first.operationId).reviewRetry
        assertEquals(0L, paced.attempts); assertEquals(f.clockNow, paced.clock); assertEquals(3_600_000L, paced.delayMillis)
        val again = f.service.poll(f.account, partition)
        assertEquals(0, again.polled); assertEquals(1, again.waiting); assertEquals(1, f.requests.size)
    }
}
