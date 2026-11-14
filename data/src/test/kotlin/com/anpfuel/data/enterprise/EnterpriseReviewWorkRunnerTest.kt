package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.domain.enterprise.EnterprisePartition
import java.util.concurrent.atomic.AtomicBoolean
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** The existing worker also polls review-only work, with separate counts and retry semantics. */
class EnterpriseReviewWorkRunnerTest {
    init {
        val tests = EnterpriseReviewWorkRunnerTest::class.java.declaredMethods.filter { it.isAnnotationPresent(Test::class.java) }
        check(tests.size == 3 && tests.all { it.returnType == Void.TYPE }) { "Review runner tests must be discoverable void methods" }
    }
    private val account = "7b0e2a54-0000-4000-8000-000000000001"
    private val organization = "7b0e2a54-0000-4000-8000-000000000002"
    private val other = "7b0e2a54-0000-4000-8000-000000000006"
    private val origin = "https://api.example.invalid"
    private fun bound(org: String) = EnterpriseEnrollmentBinding(
        EnterpriseDeviceTarget(EnterprisePartition.of(account, origin, org), "7b0e2a54-0000-4000-8000-000000000003"),
        "7b0e2a54-0000-4000-8000-000000000004", 1)
    private inner class Source : EnterpriseQueueSource {
        val retained = AtomicBoolean(true)
        val lease = EnterpriseQueueAccountLease.capture(EnterpriseLocalIdentity(account, origin, account), 0, 0,
            "7b0e2a54-0000-4000-8000-000000000004", retained = retained::get)
        override suspend fun account(): EnterpriseQueueAccountLease { lease.assertCurrent(); return lease }
        override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition) = account.forEnrollment(bound(partition.organizationId))
    }

    @Test fun reviewOnlyWorkIsPolledAfterEachDispatchEvenWithEmptyDelivery(): Unit = runBlocking {
        val source = Source(); val calls = mutableListOf<String>()
        val runner = EnterpriseOperationWorkRunner(source, { _, _, _ -> EnterpriseQueueDiscoveryPage(listOf(bound(organization), bound(other)), null, 0) },
            { _, p -> calls += "dispatch:" + p.organizationId; EnterpriseQueueDispatchResult() },
            { lease, p -> assertSame(source.lease, lease); calls += "review:" + p.organizationId
                EnterpriseReviewPollResult(polled = 2, resolved = 1, unresolved = 1) })
        val result = runner.runOnce()
        assertEquals(listOf("dispatch:$organization", "review:$organization", "dispatch:$other", "review:$other"), calls)
        assertEquals(2, result.reviewResolved); assertEquals(2, result.reviewUnresolved); assertEquals(0, result.reviewFailed)
        assertEquals(0, result.attempted); assertEquals(0, result.completed); assertEquals(0, result.retryable)
        assertFalse(result.needsRetry, "Unresolved company review must not create delivery retries")
    }

    @Test fun reviewFailureNeverChangesDeliveryCountsOrWorkerRetryAndDeliveryFailureStillPolls(): Unit = runBlocking {
        val source = Source(); val reviewed = mutableListOf<String>()
        val runner = EnterpriseOperationWorkRunner(source, { _, _, _ -> EnterpriseQueueDiscoveryPage(listOf(bound(organization), bound(other)), null, 0) },
            { _, p -> if (p.organizationId == organization) throw EnterpriseQueueDeliveryFailure(60_000) else EnterpriseQueueDispatchResult(attempted = 1, completed = 1) },
            { _, p -> reviewed += p.organizationId
                if (p.organizationId == other) throw EnterprisePersistenceUnavailable()
                EnterpriseReviewPollResult(polled = 50, failed = 50, retryAfterMillis = 900_000) })
        val result = runner.runOnce()
        assertEquals(listOf(organization, other), reviewed, "A delivery failure skipped the independent review lookup")
        assertEquals(1, result.failed, "Review failures were mixed into delivery failures")
        assertEquals(60_000L, result.retryAfterMillis, "A review Retry-After changed delivery scheduling")
        assertEquals(1, result.completed); assertEquals(51, result.reviewFailed)
        assertTrue(result.needsRetry)
        val reviewOnly = EnterpriseOperationWorkRunner(source, { _, _, _ -> EnterpriseQueueDiscoveryPage(listOf(bound(organization)), null, 0) },
            { _, _ -> EnterpriseQueueDispatchResult() }, { _, _ -> EnterpriseReviewPollResult(polled = 1, failed = 1, retryAfterMillis = 30_000) })
        val onlyReview = reviewOnly.runOnce()
        assertEquals(1, onlyReview.reviewFailed); assertFalse(onlyReview.needsRetry)
        assertNull(onlyReview.retryAfterMillis)
    }

    @Test fun replacedSourceStopsBeforeReviewLookup(): Unit = runBlocking {
        val source = Source(); var reviews = 0
        val runner = EnterpriseOperationWorkRunner(source, { _, _, _ -> EnterpriseQueueDiscoveryPage(listOf(bound(organization)), null, 0) },
            { _, _ -> source.retained.set(false); EnterpriseQueueDispatchResult() }, { _, _ -> reviews++; EnterpriseReviewPollResult() })
        assertTrue(runner.runOnce().needsRetry)
        assertEquals(0, reviews)
    }
}
