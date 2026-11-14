package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.enterprise.sync.EnterpriseDeliveryActivity
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.sync.*
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseDeliveryActivityTest {
    @Test fun completionHintsAreScopedAndHistoryOverrunOnlyRequestsRevalidation(): Unit = runTest {
        val f = SyncFlowFixture(); val account = f.source.account()
        val first = f.source.attempt(account, f.first.partition); val second = f.source.attempt(account, f.second.partition)
        val firstIdentity = f.item(f.first).identity; val secondIdentity = f.item(f.second).identity
        val initial = f.activity.completionRevision(first)
        f.activity.end(f.activity.begin(second, listOf(secondIdentity)))
        assertFalse(f.activity.completedSince(first, initial), "Known foreign completion is not evidence in this scope")
        assertTrue(f.activity.completedSince(second, initial))
        repeat(64) { f.activity.end(f.activity.begin(second, listOf(secondIdentity))) }
        assertTrue(f.activity.completedSince(first, initial), "Bounded history loss requests an honest local reread")
        val beforeOwn = f.activity.completionRevision(first)
        val ticket = f.activity.begin(first, listOf(firstIdentity))
        f.activity.end(ticket)
        assertTrue(f.activity.completedSince(first, beforeOwn)); assertFalse(f.activity.completedSince(second, beforeOwn))
        val after = f.activity.completionRevision(first)
        f.activity.end(ticket); f.activity.end(EnterpriseDeliveryActivity.Ticket())
        assertEquals(after, f.activity.completionRevision(first))
        assertTrue(f.activity.current(first).isEmpty())
    }

    @Test fun progressIsScopedBoundedAndTicketCleanupCannotRemoveAnotherAttempt(): Unit = runTest {
        val f = SyncFlowFixture(); val account = f.source.account()
        val first = f.source.attempt(account, f.first.partition); val second = f.source.attempt(account, f.second.partition)
        val operation = syncOperation(f.first)
        val identity = EnterpriseOperationIdentity(operation.operationId, operation.payloadDigest)
        val tickets = (0 until 64).map { f.activity.begin(first, listOf(identity)) }
        assertTrue(tickets.all { it != null })
        assertNull(f.activity.begin(first, listOf(identity)), "Saturation must omit observation, not impose a delivery ceiling")
        assertEquals(setOf(identity), f.activity.current(first)); assertTrue(f.activity.current(second).isEmpty())
        assertEquals(setOf(identity), f.activity.observe(first).first())
        val before = f.activity.changes.value
        f.activity.end(EnterpriseDeliveryActivity.Ticket())
        assertEquals(before, f.activity.changes.value)
        f.activity.end(tickets.first()); assertEquals(setOf(identity), f.activity.current(first))
        tickets.drop(1).forEach(f.activity::end); assertTrue(f.activity.current(first).isEmpty())
        val tooMany = (0 until 51).map { EnterpriseOperationIdentity(syncId(200 + it), "sha256:" + "1".repeat(64)) }
        assertNull(f.activity.begin(first, tooMany))
        f.fence.markBackground()
        assertThrows(EnterpriseLocalDenied::class.java) { f.activity.current(first) }
    }

    private class Store(private val operation: EnterpriseOperation,
        private val projected: suspend (EnterpriseQueueAttempt) -> Unit) : EnterpriseOperationQueue {
        private var entry = EnterpriseQueueEntry(operation)
        val records = mutableMapOf<String, EnterpriseLocalRecord>()
        private val tx = object : EnterpriseQueueTransaction {
            override fun record(id: String) = records[id]
            override fun put(record: EnterpriseLocalRecord) { records[record.id] = record }
        }
        override suspend fun save(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, project: (EnterpriseQueueTransaction) -> Unit) = false
        override suspend fun pending(attempt: EnterpriseQueueAttempt) = if (entry.acknowledgement == null) listOf(entry) else emptyList()
        override suspend fun entry(attempt: EnterpriseQueueAttempt, operationId: String) = entry
        override suspend fun acknowledge(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, ack: OperationAcknowledgement, project: (EnterpriseQueueTransaction) -> Unit): Boolean {
            project(tx); projected(attempt); entry = EnterpriseQueueEntry(operation, entry.retry, ack); return true
        }
        override suspend fun retry(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, expected: QueueRetryState, next: QueueRetryState): Boolean {
            entry = EnterpriseQueueEntry(operation, next); return true
        }
        override suspend fun stats(attempt: EnterpriseQueueAttempt) = EnterpriseQueueStats(1, 10, 0, 2, 2048)
    }
    private fun producer() = object : EnterpriseQueueProducer {
        override fun descriptor() = EnterpriseQueueProducerDescriptor("validation.status")
        override fun save(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation) {
            tx.put(EnterpriseLocalRecord(attempt.partition, operation.entity.reference.id, "SYNTHETIC-SAVED".toByteArray()))
        }
        override fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation, ack: OperationAcknowledgement) {
            tx.put(EnterpriseLocalRecord(attempt.partition, operation.entity.reference.id, "SYNTHETIC-ACCEPTED".toByteArray()))
        }
    }

    @Test fun activitySpansActualTransportAndDurableProjectionBeforeEndRefresh(): Unit = runTest {
        withTimeout(3000) {
            val f = SyncFlowFixture(); val operation = syncOperation(f.first)
            val identity = EnterpriseOperationIdentity(operation.operationId, operation.payloadDigest)
            val publication = CompletableDeferred<Unit>(); val finish = CompletableDeferred<Unit>()
            var committed = false
            val store = Store(operation) { attempt ->
                assertEquals(setOf(identity), f.activity.current(attempt))
                publication.complete(Unit); finish.await(); committed = true
            }
            val service = EnterpriseQueueService(f.source, store, EnterpriseOperationTransport { attempt, operations ->
                assertEquals(setOf(identity), f.activity.current(attempt))
                EnterpriseOperationDelivery(operations.map(::syncAck))
            }, EnterpriseQueueRegistry(producer()), { f.now }, { 0 }, activity = f.activity)
            val job = async { service.dispatch(f.first.partition) }
            publication.await()
            assertFalse(committed)
            val viewer = f.source.attempt(f.source.account(), f.first.partition)
            assertEquals(setOf(identity), f.activity.current(viewer))
            finish.complete(Unit)
            assertEquals(1, job.await().completed); assertTrue(committed)
            assertArrayEquals("SYNTHETIC-ACCEPTED".toByteArray(), store.records.getValue(operation.entity.reference.id).envelope)
            assertTrue(f.activity.current(viewer).isEmpty())
        }
    }

    @Test fun failedOrCancelledDeliveryAlwaysRemovesOnlyItsMemoryHint(): Unit = runTest {
        for (cancel in listOf(false, true)) {
            val f = SyncFlowFixture(); val operation = syncOperation(f.first)
            val store = Store(operation) { }
            var observed = false
            val service = EnterpriseQueueService(f.source, store, EnterpriseOperationTransport { attempt, _ ->
                observed = f.activity.current(attempt).isNotEmpty()
                if (cancel) throw CancellationException("Synthetic cancellation") else throw EnterpriseQueueDeliveryFailure()
            }, EnterpriseQueueRegistry(producer()), { f.now }, { 0 }, activity = f.activity)
            try {
                val result = service.dispatch(f.first.partition)
                assertFalse(cancel); assertEquals(1, result.retryable)
            } catch (_: CancellationException) { assertTrue(cancel) }
            assertTrue(observed)
            val viewer = f.source.attempt(f.source.account(), f.first.partition)
            assertTrue(f.activity.current(viewer).isEmpty())
            assertNull(store.entry(viewer, operation.operationId).acknowledgement)
        }
    }
}
