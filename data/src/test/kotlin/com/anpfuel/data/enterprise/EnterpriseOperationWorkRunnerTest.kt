package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.domain.enterprise.EnterprisePartition
import java.util.concurrent.atomic.AtomicBoolean
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseOperationWorkRunnerTest {
    private val account = "7b0e2a54-0000-4000-8000-000000000001"
    private val organization = "7b0e2a54-0000-4000-8000-000000000002"
    private val other = "7b0e2a54-0000-4000-8000-000000000006"
    private val member = "7b0e2a54-0000-4000-8000-000000000003"
    private val enrollment = "7b0e2a54-0000-4000-8000-000000000004"
    private val origin = "https://api.example.invalid"
    private val cursor = "a".repeat(64) + "-" + "b".repeat(64)
    private fun bound(org: String) = EnterpriseEnrollmentBinding(
        EnterpriseDeviceTarget(EnterprisePartition.of(account, origin, org), member), enrollment, 1)

    private inner class Source : EnterpriseQueueSource {
        val retained = AtomicBoolean(true)
        var captures = 0
        val lease = EnterpriseQueueAccountLease.capture(EnterpriseLocalIdentity(account, origin, account), 0, 0, enrollment, retained = retained::get)
        override suspend fun account(): EnterpriseQueueAccountLease { captures++; lease.assertCurrent(); return lease }
        override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt =
            account.forEnrollment(bound(partition.organizationId))
    }

    @Test fun `worker advances scoped discovery and an unavailable organization cannot hold another`(): Unit = runBlocking {
        val source = Source()
        val seen = mutableListOf<String?>()
        val dispatched = mutableListOf<EnterprisePartition>()
        val runner = EnterpriseOperationWorkRunner(source, { lease, after, limit ->
            assertSame(source.lease, lease); assertEquals(50, limit); seen += after
            if (after == null) EnterpriseQueueDiscoveryPage(listOf(bound(organization)), cursor, 0)
            else EnterpriseQueueDiscoveryPage(listOf(bound(other)), null, 0)
        }, { lease, partition ->
            assertSame(source.lease, lease); dispatched += partition
            if (partition.organizationId == organization) throw EnterpriseQueueDeliveryFailure(120000)
            EnterpriseQueueDispatchResult(attempted = 2, completed = 2)
        })
        val result = runner.runOnce()
        assertEquals(listOf(null, cursor), seen)
        assertEquals(listOf(organization, other), dispatched.map { it.organizationId })
        assertEquals(1, source.captures)
        assertEquals(2, result.completed)
        assertEquals(1, result.failed)
        assertTrue(result.needsRetry)
        assertEquals(120000L, result.retryAfterMillis)
    }

    @Test fun `a replaced source after discovery cannot dispatch under its replacement`(): Unit = runBlocking {
        val source = Source()
        var dispatched = 0
        val runner = EnterpriseOperationWorkRunner(source, { _, _, _ ->
            source.retained.set(false)
            EnterpriseQueueDiscoveryPage(listOf(bound(organization)), null, 0)
        }, { _, _ -> dispatched++; EnterpriseQueueDispatchResult(completed = 1) })
        assertTrue(runner.runOnce().needsRetry)
        assertEquals(0, dispatched)
        assertEquals(1, source.captures)
    }

    @Test fun `overlapping workers coalesce without sharing a discovery cursor`(): Unit = runBlocking {
        val source = Source()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val runner = EnterpriseOperationWorkRunner(source, { _, after, _ ->
            assertNull(after); entered.complete(Unit); release.await()
            EnterpriseQueueDiscoveryPage(emptyList(), null, 0)
        }, { _, _ -> error("empty discovery dispatched work") })
        val first = async { runner.runOnce() }
        withTimeout(5_000) { entered.await() }
        val second = runner.runOnce()
        assertTrue(second.coalesced)
        assertTrue(second.needsRetry)
        release.complete(Unit)
        assertFalse(withTimeout(5_000) { first.await() }.needsRetry)
        assertEquals(1, source.captures)
    }

    @Test fun `cancellation propagates and releases only invocation ownership`(): Unit = runBlocking {
        val source = Source()
        val entered = CompletableDeferred<Unit>()
        var blocked = true
        val runner = EnterpriseOperationWorkRunner(source, { _, _, _ -> EnterpriseQueueDiscoveryPage(listOf(bound(organization)), null, 0) },
            { _, _ -> if (blocked) { entered.complete(Unit); awaitCancellation() }; EnterpriseQueueDispatchResult(completed = 1) })
        val first = launch { runner.runOnce() }
        withTimeout(5_000) { entered.await() }
        withTimeout(5_000) { first.cancelAndJoin() }
        assertTrue(first.isCancelled)
        blocked = false
        val result = runner.runOnce()
        assertFalse(result.coalesced)
        assertEquals(1, result.completed)
        assertEquals(2, source.captures)
    }

    @Test fun `bad repeated discovery cursors fail bounded and never persist into the next invocation`(): Unit = runBlocking {
        val source = Source()
        val seen = mutableListOf<String?>()
        val runner = EnterpriseOperationWorkRunner(source, { _, after, _ ->
            seen += after
            EnterpriseQueueDiscoveryPage(emptyList(), cursor, 0)
        }, { _, _ -> error("empty discovery dispatched work") })
        assertTrue(runner.runOnce().needsRetry)
        assertEquals(listOf(null, cursor), seen)
        assertTrue(runner.runOnce().needsRetry)
        assertEquals(listOf(null, cursor, null, cursor), seen)
        assertEquals(2, source.captures)
    }
}
