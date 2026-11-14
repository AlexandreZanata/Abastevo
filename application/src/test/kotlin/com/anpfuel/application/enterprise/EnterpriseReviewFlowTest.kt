package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.enterprise.sync.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.sync.*
import java.security.MessageDigest
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** Presentation flow over the actual review service, with in-memory protected-store stand-ins only. */
class EnterpriseReviewFlowTest {
    init {
        val tests = EnterpriseReviewFlowTest::class.java.declaredMethods.filter { it.isAnnotationPresent(Test::class.java) }
        check(tests.size == 5 && tests.all { it.returnType == Void.TYPE }) { "Review flow tests must be discoverable void methods" }
    }

    private fun sha(bytes: ByteArray) = "sha256:" + MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it.toInt() and 255) }
    private val reviewItem = syncId(600)

    private inner class Fixture {
        val fence = EnterpriseSessionFence()
        val context = EnterpriseContextSummary(EnterprisePartition.of(syncId(1), SYNC_ORIGIN, syncId(2)), syncId(3), "Synthetic review")
        val identity = EnterpriseLocalIdentity(syncId(1), SYNC_ORIGIN, syncId(9))
        val now = EnterpriseDeviceClock("boot", 1000, 10000)
        val operation = syncOperation(context, 40)
        val receipt: OperationAcknowledgement = OperationAcknowledgement.parse("""{"schema_version":1,"operation_id":"${operation.operationId}","payload_digest":"${operation.payloadDigest}","result":"ACCEPTED_FOR_REVIEW","code":"PERMISSION_CHANGED","entity_revision":null,"received_at":"2026-11-13T00:01:00Z","review_item_id":"$reviewItem","warnings":[]}""".toByteArray(), operation)
        var entry = EnterpriseQueueEntry(operation, acknowledgement = receipt)
        var decision: ReviewResolution? = null
        var lookups = 0; var operationRequests = 0
        var dismissed: List<String>? = null
        private var nonce = 100
        val activity = EnterpriseDeliveryActivity()
        val source = object : EnterpriseQueueSource {
            override suspend fun account(): EnterpriseQueueAccountLease {
                val epoch = fence.epoch; val background = fence.backgroundEpoch
                return EnterpriseQueueAccountLease.capture(identity, epoch, background, syncId(nonce++)) {
                    fence.epoch == epoch && fence.backgroundEpoch == background
                }
            }
            override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt {
                account.assertCurrent(); require(partition == context.partition)
                return account.forEnrollment(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(partition, context.membershipId), syncId(4), 1))
            }
        }
        val store = object : EnterpriseSyncStatusStore {
            override suspend fun page(attempt: EnterpriseQueueAttempt, mode: EnterpriseSyncMode, cursor: EnterpriseSyncCursor?, limit: Int): EnterpriseSyncPage {
                val all = listOf(EnterpriseSyncProjection.item(entry))
                val items = all.filter { mode == EnterpriseSyncMode.HISTORY || it.pending }
                return EnterpriseSyncPage(items, EnterpriseSyncProjection.totals(all, EnterpriseQueueStats(0, 0, 0, 1, 2048)), 1, null, mode)
            }
            override suspend fun detail(attempt: EnterpriseQueueAttempt, operationId: String, digest: String) =
                EnterpriseSyncDetail(EnterpriseSyncProjection.item(entry), emptyList())
            override suspend fun dismissRejected(attempt: EnterpriseQueueAttempt, operationId: String, digest: String, expectedOutcomeDigest: String): Boolean {
                dismissed = listOf(operationId, digest, expectedOutcomeDigest)
                entry = EnterpriseQueueEntry(entry.operation, entry.retry, entry.acknowledgement, true, entry.resolution, entry.reviewRetry)
                return true
            }
        }
        private val operations = object : EnterpriseOperationQueue {
            override suspend fun save(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, project: (EnterpriseQueueTransaction) -> Unit) = false
            override suspend fun pending(attempt: EnterpriseQueueAttempt) = emptyList<EnterpriseQueueEntry>()
            override suspend fun entry(attempt: EnterpriseQueueAttempt, operationId: String) = entry
            override suspend fun acknowledge(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, ack: OperationAcknowledgement, project: (EnterpriseQueueTransaction) -> Unit) = false
            override suspend fun retry(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, expected: QueueRetryState, next: QueueRetryState) = false
            override suspend fun stats(attempt: EnterpriseQueueAttempt) = EnterpriseQueueStats(0, 0, 0, 1, 2048)
        }
        var reviewStorageFailure = false
        val reviewQueue = object : EnterpriseReviewQueue {
            override suspend fun unresolvedReviews(attempt: EnterpriseQueueAttempt): List<EnterpriseQueueEntry> {
                if (reviewStorageFailure) throw EnterprisePersistenceUnavailable()
                return listOf(entry).filter { it.unresolvedReview }
            }
            override suspend fun resolve(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, acknowledgement: OperationAcknowledgement, resolution: ReviewResolution): Boolean {
                entry = EnterpriseQueueEntry(entry.operation, entry.retry, entry.acknowledgement, resolution = resolution); return true
            }
            override suspend fun reviewRetry(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, acknowledgement: OperationAcknowledgement, expected: QueueRetryState, next: QueueRetryState) = false
        }
        val delivery = EnterpriseQueueService(source, operations, EnterpriseOperationTransport { _, _ -> operationRequests++; error("No pending delivery") },
            EnterpriseQueueRegistry(), { now }, { 0 }, activity = activity)
        val reviews = EnterpriseReviewService(source, reviewQueue, EnterpriseReviewTransport { _, expected ->
            lookups++; EnterpriseReviewDelivery(expected.map { ReviewResult(it.selector, decision) })
        }, { now }, { 0 }, activity = activity)
        val flow = EnterpriseSyncFlow(source, store, delivery, activity, fence, reviews)
        fun decide(outcome: String) = ReviewResolution.create(ReviewResolutionSnapshot(operation.operationId, operation.payloadDigest, reviewItem,
            syncId(700), 2, outcome, if (outcome == "REFUSED") "Synthetic reviewer refusal" else "", "2026-11-13T00:05:00Z",
            if (outcome == "ACCEPTED") 2 else null))
    }

    @Test fun manualCheckPollsReviewOnlyWorkWithoutPendingDeliveryAndShowsTheDecision(): Unit = runTest {
        val f = Fixture(); f.flow.open(f.context, f.fence.epoch)
        assertEquals(EnterpriseSyncStatus.NEEDS_REVIEW, f.flow.state.value.items.single().status)
        f.decision = f.decide("ACCEPTED")
        val state = f.flow.state.value
        f.flow.manualCheck(true, f.fence.epoch, state.generation)
        assertEquals(1, f.lookups); assertEquals(0, f.operationRequests)
        val history = f.flow.state.value
        assertTrue(history.items.isEmpty(), "Accepted after review without attachments left pending")
        assertEquals(0, history.totals!!.reviewCount); assertEquals(1, history.totals!!.acceptedCount)
        assertTrue(history.active.isEmpty())
    }

    @Test fun unresolvedAnswerKeepsHonestReviewStateAndCannotBeHidden(): Unit = runTest {
        val f = Fixture(); f.flow.open(f.context, f.fence.epoch)
        f.flow.manualCheck(true, f.fence.epoch, f.flow.state.value.generation)
        assertEquals(1, f.lookups)
        val item = f.flow.state.value.items.single()
        assertEquals(EnterpriseSyncStatus.NEEDS_REVIEW, item.status); assertNull(item.resolution)
        f.flow.dismiss(item.identity.operationId, item.identity.payloadDigest, item.receipt!!.receiptDigest, f.fence.epoch, f.flow.state.value.generation)
        assertNull(f.dismissed, "An unresolved REVIEW was hidden")
    }

    @Test fun refusedDecisionDismissalIsBoundToTheResolutionNotTheOriginalReceipt(): Unit = runTest {
        val f = Fixture(); f.decision = f.decide("REFUSED"); f.flow.open(f.context, f.fence.epoch)
        f.flow.manualCheck(true, f.fence.epoch, f.flow.state.value.generation)
        val item = f.flow.state.value.items.single()
        assertEquals(EnterpriseSyncStatus.REJECTED, item.status)
        assertEquals(EnterpriseReviewOutcome.REFUSED, item.resolution!!.outcome)
        f.flow.dismiss(item.identity.operationId, item.identity.payloadDigest, item.receipt!!.receiptDigest, f.fence.epoch, f.flow.state.value.generation)
        assertNull(f.dismissed, "The old REVIEW receipt digest hid a later decision")
        val digest = sha(f.decision!!.canonicalBytes)
        assertEquals(digest, item.resolution!!.resolutionDigest)
        f.flow.dismiss(item.identity.operationId, item.identity.payloadDigest, digest, f.fence.epoch, f.flow.state.value.generation)
        assertEquals(listOf(item.identity.operationId, item.identity.payloadDigest, digest), f.dismissed)
        assertTrue(f.flow.state.value.items.isEmpty()); assertEquals(1, f.flow.state.value.totals!!.hiddenRejectedCount)
    }

    @Test fun workerResolutionRequestsAScopedRereadWithoutPresentingSending(): Unit = runTest {
        val f = Fixture(); f.flow.open(f.context, f.fence.epoch)
        f.decision = f.decide("ACCEPTED")
        val result = f.reviews.poll(f.source.account(), f.context.partition)
        assertEquals(1, result.resolved)
        f.flow.onActivityChanged()
        assertTrue(f.flow.state.value.items.isEmpty(), "Durable worker resolution did not trigger a reread")
        assertEquals(1, f.flow.state.value.totals!!.acceptedCount)
    }

    @Test fun reviewOnlyFailureDuringManualCheckDoesNotHideCommittedLocalState(): Unit = runTest {
        val f = Fixture(); f.flow.open(f.context, f.fence.epoch)
        f.reviewStorageFailure = true
        f.flow.manualCheck(true, f.fence.epoch, f.flow.state.value.generation)
        val state = f.flow.state.value
        assertNull(state.error, "A review lookup failure replaced readable delivery state with an error")
        assertFalse(state.busy)
        assertEquals(EnterpriseSyncStatus.NEEDS_REVIEW, state.items.single().status)
        assertNotNull(state.totals)
    }
}
