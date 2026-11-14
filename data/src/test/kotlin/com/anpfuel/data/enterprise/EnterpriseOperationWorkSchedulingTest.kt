package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.domain.enterprise.EnterprisePartition
import java.util.concurrent.atomic.AtomicBoolean
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** Worker scheduling: bounded draining, signed-out idleness and honored server pacing (EDR-122). */
class EnterpriseOperationWorkSchedulingTest {
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
        override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition) =
            account.forEnrollment(bound(partition.organizationId))
    }

    /** Models one partition's retained due work: a batch holds at most 50 operations. */
    private class Partition(var pending: Int, val fail: Boolean = false) {
        var calls = 0
        fun dispatch(): EnterpriseQueueDispatchResult {
            calls++
            val sent = minOf(50, pending)
            if (fail) return EnterpriseQueueDispatchResult(attempted = sent, retryable = pending, retryAfterMillis = if (pending > sent) 0 else 15_000)
            pending -= sent
            return EnterpriseQueueDispatchResult(attempted = sent, completed = sent, retryable = pending, retryAfterMillis = if (pending > 0) 0 else null)
        }
    }

    private fun runner(source: Source, vararg partitions: Pair<String, Partition>) = EnterpriseOperationWorkRunner(source,
        { _, _, _ -> EnterpriseQueueDiscoveryPage(partitions.map { bound(it.first) }, null, 0) },
        { _, p -> partitions.single { it.first == p.organizationId }.second.dispatch() })

    @Test fun queueLargerThanOneBatchDrainsInTheSameInvocation(): Unit = runBlocking {
        val large = Partition(120); val small = Partition(3)
        val result = runner(Source(), organization to large, other to small).runOnce()
        assertEquals(3, large.calls, "A queue over 50 waited for system backoff between batches")
        assertEquals(1, small.calls)
        assertEquals(123, result.completed); assertEquals(123, result.attempted); assertEquals(0, result.retryable)
        assertFalse(result.needsRetry)
    }

    @Test fun drainingIsBoundedPerInvocation(): Unit = runBlocking {
        val huge = Partition(10_000)
        val result = runner(Source(), organization to huge).runOnce()
        assertEquals(10, huge.calls, "One invocation must stop after its bounded dispatch rounds")
        assertEquals(500, result.completed); assertEquals(9_500, result.retryable)
        assertTrue(result.needsRetry)
    }

    @Test fun aFailingBatchNeverSendsTheNextPageInTheSameInvocation(): Unit = runBlocking {
        val failing = Partition(120, fail = true)
        val result = runner(Source(), organization to failing).runOnce()
        assertEquals(1, failing.calls, "A failed batch must not hammer the source with the next page")
        assertEquals(0, result.completed); assertEquals(120, result.retryable)
        assertTrue(result.needsRetry)
        assertEquals(EnterpriseOperationWorkNext.Backoff, result.next, "Stalled due work keeps ordinary system backoff")
    }

    @Test fun boundedDrainContinuesImmediatelyInANewInvocation(): Unit = runBlocking {
        val result = runner(Source(), organization to Partition(10_000)).runOnce()
        assertEquals(EnterpriseOperationWorkNext.Continue(0), result.next)
    }

    @Test fun signedOutWorkerGoesIdleWithoutRetryingOrDiscovering(): Unit = runBlocking {
        var discovered = 0
        val signedOut = object : EnterpriseQueueSource {
            override suspend fun account(): EnterpriseQueueAccountLease = throw EnterpriseLocalDenied()
            override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition) = throw EnterpriseLocalDenied()
            override fun hasSession(): Boolean = false
        }
        val result = EnterpriseOperationWorkRunner(signedOut, { _, _, _ -> discovered++; EnterpriseQueueDiscoveryPage(emptyList(), null, 0) },
            { _, _ -> error("signed-out work dispatched") }).runOnce()
        assertTrue(result.signedOut)
        assertEquals(0, result.failed); assertEquals(0, discovered)
        assertFalse(result.needsRetry, "A signed-out worker retried forever")
        assertEquals(EnterpriseOperationWorkNext.Done, result.next)
    }

    @Test fun signOutDuringAnInvocationAlsoGoesIdleButAReplacedLiveSessionBacksOff(): Unit = runBlocking {
        for (session in listOf(false, true)) {
            val source = object : EnterpriseQueueSource {
                val inner = Source()
                override suspend fun account() = inner.account()
                override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition) = inner.attempt(account, partition)
                override fun hasSession(): Boolean = session
            }
            val result = EnterpriseOperationWorkRunner(source, { _, _, _ ->
                source.inner.retained.set(false)
                EnterpriseQueueDiscoveryPage(listOf(bound(organization)), null, 0)
            }, { _, _ -> error("replaced source dispatched") }).runOnce()
            assertEquals(!session, result.signedOut)
            assertEquals(if (session) EnterpriseOperationWorkNext.Backoff else EnterpriseOperationWorkNext.Done, result.next)
        }
    }

    @Test fun storedDelayAndServerRetryAfterScheduleADelayedContinuation(): Unit = runBlocking {
        val waiting = EnterpriseOperationWorkRunner(Source(), { _, _, _ -> EnterpriseQueueDiscoveryPage(listOf(bound(organization), bound(other)), null, 0) },
            { _, p -> if (p.organizationId == organization) EnterpriseQueueDispatchResult(retryable = 3, retryAfterMillis = 120_000)
                else EnterpriseQueueDispatchResult(retryable = 1, retryAfterMillis = 3_600_000) }).runOnce()
        assertEquals(0, waiting.failed)
        assertEquals(EnterpriseOperationWorkNext.Continue(120_000), waiting.next, "The earliest due work decides the wake-up")
        val limited = EnterpriseOperationWorkRunner(Source(), { _, _, _ -> EnterpriseQueueDiscoveryPage(listOf(bound(organization)), null, 0) },
            { _, _ -> throw EnterpriseQueueDeliveryFailure(900_000) }).runOnce()
        assertEquals(1, limited.failed)
        assertEquals(EnterpriseOperationWorkNext.Continue(900_000), limited.next, "Server Retry-After was replaced by blind backoff")
        val parked = EnterpriseOperationWorkRunner(Source(), { _, _, _ -> EnterpriseQueueDiscoveryPage(listOf(bound(organization)), null, 0) },
            { _, _ -> throw EnterpriseQueueDeliveryFailure(Long.MAX_VALUE) }).runOnce()
        assertEquals(EnterpriseOperationWorkNext.Continue(21_600_000), parked.next, "A continuation is bounded by EDR-122")
    }

    @Test fun unknownFailuresKeepBackoffEvenWhenOtherWorkHasAKnownDelay(): Unit = runBlocking {
        val result = EnterpriseOperationWorkRunner(Source(), { _, _, _ -> EnterpriseQueueDiscoveryPage(listOf(bound(organization), bound(other)), null, 0) },
            { _, p -> if (p.organizationId == organization) throw EnterprisePersistenceUnavailable()
                else EnterpriseQueueDispatchResult(retryable = 1, retryAfterMillis = 3_600_000) }).runOnce()
        assertEquals(1, result.failed)
        assertEquals(EnterpriseOperationWorkNext.Backoff, result.next)
        val unavailable = EnterpriseOperationWorkRunner(Source(), { _, _, _ -> EnterpriseQueueDiscoveryPage(emptyList(), null, 2) },
            { _, _ -> error("nothing discovered") }).runOnce()
        assertEquals(2, unavailable.failed); assertEquals(EnterpriseOperationWorkNext.Backoff, unavailable.next)
    }

    @Test fun reconfirmationWaitIsAttentionNotAFailure(): Unit = runBlocking {
        val result = EnterpriseOperationWorkRunner(Source(), { _, _, _ -> EnterpriseQueueDiscoveryPage(emptyList(), null, 0, contactRequired = 2) },
            { _, _ -> error("waiting work dispatched") }).runOnce()
        assertEquals(2, result.attention); assertEquals(0, result.failed)
        assertEquals(EnterpriseOperationWorkNext.Done, result.next)
    }
}
