package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.attachment.EnterpriseAttachmentDeliveryResult
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.domain.enterprise.EnterprisePartition
import java.util.concurrent.atomic.AtomicBoolean
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** EMP-OFFL-005: the operation worker also delivers attachments, after each partition's operations (EDR-268). */
class EnterpriseAttachmentWorkRunnerTest {
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

    @Test fun attachmentsRunAfterOperationsAndReviewsOfEachPartitionWithSeparateCounts(): Unit = runBlocking {
        val source = Source(); val calls = mutableListOf<String>()
        val runner = EnterpriseOperationWorkRunner(source, { _, _, _ -> EnterpriseQueueDiscoveryPage(listOf(bound(organization), bound(other)), null, 0) },
            { _, p -> calls += "dispatch:" + p.organizationId; EnterpriseQueueDispatchResult(attempted = 1, completed = 1) },
            { _, p -> calls += "review:" + p.organizationId; EnterpriseReviewPollResult() },
            { lease, p -> assertSame(source.lease, lease); calls += "attachments:" + p.organizationId
                EnterpriseAttachmentDeliveryResult(attempted = 2, stored = 1, pending = 1, nextAttemptMillis = 45_000) })
        val result = runner.runOnce()
        assertEquals(listOf("dispatch:$organization", "review:$organization", "attachments:$organization",
            "dispatch:$other", "review:$other", "attachments:$other"), calls)
        assertEquals(2, result.attachmentsStored); assertEquals(2, result.attachmentsPending); assertEquals(0, result.attachmentsFailed)
        assertEquals(2, result.completed); assertEquals(0, result.failed)
        // A pending attachment with a known due time schedules the worker's continuation.
        assertEquals(EnterpriseOperationWorkNext.Continue(45_000), result.next)
    }

    @Test fun attachmentFailuresNeverChangeOperationDeliveryAndADeliveryFailureStillUploads(): Unit = runBlocking {
        val source = Source(); val uploaded = mutableListOf<String>()
        val runner = EnterpriseOperationWorkRunner(source, { _, _, _ -> EnterpriseQueueDiscoveryPage(listOf(bound(organization), bound(other)), null, 0) },
            { _, p -> if (p.organizationId == organization) throw EnterpriseQueueDeliveryFailure(60_000) else EnterpriseQueueDispatchResult(attempted = 1, completed = 1) },
            null,
            { _, p -> uploaded += p.organizationId
                if (p.organizationId == other) throw EnterprisePersistenceUnavailable()
                EnterpriseAttachmentDeliveryResult(attempted = 1, pending = 1) })
        val result = runner.runOnce()
        assertEquals(listOf(organization, other), uploaded, "an operation delivery failure skipped the independent attachment delivery")
        assertEquals(1, result.failed, "attachment failures were mixed into operation failures")
        assertEquals(60_000L, result.retryAfterMillis)
        assertEquals(1, result.attachmentsFailed); assertEquals(1, result.attachmentsPending)
        assertTrue(result.needsRetry)
        // Settled or waiting attachments with no due time never keep the worker busy.
        val idle = EnterpriseOperationWorkRunner(source, { _, _, _ -> EnterpriseQueueDiscoveryPage(listOf(bound(organization)), null, 0) },
            { _, _ -> EnterpriseQueueDispatchResult() }, null, { _, _ -> EnterpriseAttachmentDeliveryResult(stored = 1) })
        assertFalse(idle.runOnce().needsRetry)
        // A replaced source stops before any attachment work.
        var attachments = 0
        val replaced = EnterpriseOperationWorkRunner(source, { _, _, _ -> EnterpriseQueueDiscoveryPage(listOf(bound(organization)), null, 0) },
            { _, _ -> source.retained.set(false); EnterpriseQueueDispatchResult() }, null, { _, _ -> attachments++; EnterpriseAttachmentDeliveryResult() })
        assertTrue(replaced.runOnce().needsRetry)
        assertEquals(0, attachments)
    }
}
