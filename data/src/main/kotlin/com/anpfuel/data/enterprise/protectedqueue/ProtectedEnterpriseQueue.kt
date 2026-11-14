package com.anpfuel.data.enterprise.protectedqueue

import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.EnterpriseOperationReused
import com.anpfuel.application.enterprise.queue.EnterpriseOperationQueue
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueTransaction
import com.anpfuel.application.enterprise.queue.EnterpriseReviewQueue
import com.anpfuel.domain.enterprise.sync.ReviewResolution
import com.anpfuel.data.enterprise.*
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueStats
import com.anpfuel.domain.enterprise.sync.QueueRetryState
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncResult

/** Typed queue shares the accepted sealed snapshot and its trusted committed head. */
class ProtectedEnterpriseQueue(private val store: ProtectedEnterprisePersistence) : EnterpriseOperationQueue, EnterpriseReviewQueue {
    override suspend fun save(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        project: (EnterpriseQueueTransaction) -> Unit): Boolean = store.queueAccess(attempt) { snapshot ->
        ProtectedQueueCodec.checkBinding(operation, snapshot)
        if (snapshot.entries.containsKey("OPERATION:" + operation.operationId)) throw EnterprisePersistenceUnavailable()
        val key = "QUEUE:" + operation.operationId
        val existing = snapshot.entries[key]
        if (existing != null) {
            sameIntent(operation, ProtectedQueueCodec.decode(existing.bytes, snapshot, existing.id).operation)
            VaultQueueChange(false)
        } else {
            project(snapshot, project)
            snapshot.entries[key] = VaultEntry(VaultKind.QUEUE, operation.operationId, "", ProtectedQueueCodec.encode(EnterpriseQueueEntry(operation)))
            VaultQueueChange(true, true)
        }
    }

    override suspend fun pending(attempt: EnterpriseQueueAttempt): List<EnterpriseQueueEntry> = store.queueAccess(attempt) { snapshot ->
        VaultQueueChange(snapshot.entries.values.filter { it.kind == VaultKind.QUEUE }
            .map { ProtectedQueueCodec.decode(it.bytes, snapshot, it.id) }.filter { it.acknowledgement == null })
    }

    override suspend fun entry(attempt: EnterpriseQueueAttempt, operationId: String): EnterpriseQueueEntry? {
        EnterprisePartition.requireId(operationId)
        return store.queueAccess(attempt) { snapshot ->
            VaultQueueChange(snapshot.entries["QUEUE:$operationId"]?.let { ProtectedQueueCodec.decode(it.bytes, snapshot, it.id) })
        }
    }

    override suspend fun acknowledge(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        ack: OperationAcknowledgement, project: (EnterpriseQueueTransaction) -> Unit): Boolean = store.queueAccess(attempt) { snapshot ->
        ProtectedQueueCodec.checkBinding(operation, snapshot)
        if (!ack.isFinal || !ack.matches(operation)) throw EnterprisePersistenceUnavailable()
        val key = "QUEUE:" + operation.operationId
        val stored = snapshot.entries[key] ?: throw EnterprisePersistenceUnavailable()
        val entry = ProtectedQueueCodec.decode(stored.bytes, snapshot, stored.id)
        sameIntent(operation, entry.operation)
        val existing = entry.acknowledgement
        if (existing != null) {
            if (!existing.canonicalBytes.contentEquals(ack.canonicalBytes)) throw EnterprisePersistenceUnavailable()
            VaultQueueChange(false)
        } else {
            project(snapshot, project)
            snapshot.entries[key] = VaultEntry(VaultKind.QUEUE, stored.id, "",
                ProtectedQueueCodec.encode(EnterpriseQueueEntry(entry.operation, entry.retry, ack, entry.dismissedRejected,
                    entry.resolution, entry.reviewRetry)))
            VaultQueueChange(true, true)
        }
    }

    override suspend fun retry(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        expected: QueueRetryState, next: QueueRetryState): Boolean = store.queueAccess(attempt) { snapshot ->
        ProtectedQueueCodec.checkBinding(operation, snapshot)
        val key = "QUEUE:" + operation.operationId
        val stored = snapshot.entries[key] ?: throw EnterprisePersistenceUnavailable()
        val entry = ProtectedQueueCodec.decode(stored.bytes, snapshot, stored.id)
        sameIntent(operation, entry.operation)
        if (entry.acknowledgement != null || !sameRetry(entry.retry, expected)) VaultQueueChange(false)
        else if (sameRetry(entry.retry, next)) VaultQueueChange(true)
        else {
            snapshot.entries[key] = VaultEntry(VaultKind.QUEUE, stored.id, "",
                ProtectedQueueCodec.encode(EnterpriseQueueEntry(entry.operation, next, entry.acknowledgement,
                    entry.dismissedRejected, entry.resolution, entry.reviewRetry)))
            VaultQueueChange(true, true)
        }
    }

    override suspend fun unresolvedReviews(attempt: EnterpriseQueueAttempt): List<EnterpriseQueueEntry> = store.queueAccess(attempt) { snapshot ->
        VaultQueueChange(snapshot.entries.values.filter { it.kind == VaultKind.QUEUE }
            .map { ProtectedQueueCodec.decode(it.bytes, snapshot, it.id) }.filter { it.unresolvedReview })
    }

    /** One locked read-check-write: exact intent, original REVIEW bytes and its ReviewID; equal replay is a no-op. */
    override suspend fun resolve(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        acknowledgement: OperationAcknowledgement, resolution: ReviewResolution): Boolean = store.queueAccess(attempt) { snapshot ->
        ProtectedQueueCodec.checkBinding(operation, snapshot)
        val key = "QUEUE:" + operation.operationId
        val stored = snapshot.entries[key] ?: throw EnterprisePersistenceUnavailable()
        val entry = ProtectedQueueCodec.decode(stored.bytes, snapshot, stored.id)
        sameIntent(operation, entry.operation)
        val receipt = entry.acknowledgement ?: throw EnterprisePersistenceUnavailable()
        if (receipt.result != SyncResult.ACCEPTED_FOR_REVIEW || !receipt.canonicalBytes.contentEquals(acknowledgement.canonicalBytes))
            throw EnterprisePersistenceUnavailable()
        val existing = entry.resolution
        if (existing != null) {
            if (!existing.canonicalBytes.contentEquals(resolution.canonicalBytes)) throw EnterprisePersistenceUnavailable()
            VaultQueueChange(false)
        } else {
            val resolved = try {
                EnterpriseQueueEntry(entry.operation, entry.retry, receipt, entry.dismissedRejected, resolution)
            } catch (_: IllegalArgumentException) { throw EnterprisePersistenceUnavailable() }
            snapshot.entries[key] = VaultEntry(VaultKind.QUEUE, stored.id, "", ProtectedQueueCodec.encode(resolved))
            VaultQueueChange(true, true)
        }
    }

    override suspend fun reviewRetry(attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
        acknowledgement: OperationAcknowledgement, expected: QueueRetryState, next: QueueRetryState): Boolean = store.queueAccess(attempt) { snapshot ->
        ProtectedQueueCodec.checkBinding(operation, snapshot)
        val key = "QUEUE:" + operation.operationId
        val stored = snapshot.entries[key] ?: throw EnterprisePersistenceUnavailable()
        val entry = ProtectedQueueCodec.decode(stored.bytes, snapshot, stored.id)
        sameIntent(operation, entry.operation)
        val receipt = entry.acknowledgement ?: throw EnterprisePersistenceUnavailable()
        if (!receipt.canonicalBytes.contentEquals(acknowledgement.canonicalBytes)) throw EnterprisePersistenceUnavailable()
        if (!entry.unresolvedReview || !entry.reviewRetry.sameAs(expected)) VaultQueueChange(false)
        else if (entry.reviewRetry.sameAs(next)) VaultQueueChange(true)
        else {
            snapshot.entries[key] = VaultEntry(VaultKind.QUEUE, stored.id, "", ProtectedQueueCodec.encode(
                EnterpriseQueueEntry(entry.operation, entry.retry, receipt, entry.dismissedRejected, null, next)))
            VaultQueueChange(true, true)
        }
    }

    override suspend fun stats(attempt: EnterpriseQueueAttempt): EnterpriseQueueStats = store.queueAccess(attempt) { snapshot ->
        val pending = snapshot.entries.values.filter { it.kind == VaultKind.QUEUE }
            .filter { ProtectedQueueCodec.decode(it.bytes, snapshot, it.id).acknowledgement == null }
        VaultQueueChange(EnterpriseQueueStats(pending.size, pending.sumOf { it.bytes.size.toLong() },
            snapshot.entries.values.count { it.kind == VaultKind.OPERATION }, snapshot.entries.size,
            2048L + snapshot.entries.values.sumOf { it.bytes.size.toLong() + 512 }))
    }

    private fun sameIntent(attempted: EnterpriseOperation, stored: EnterpriseOperation) {
        if (attempted.operationId != stored.operationId || attempted.payloadDigest != stored.payloadDigest ||
            !attempted.canonicalBytes.contentEquals(stored.canonicalBytes)) throw EnterpriseOperationReused()
    }

    private fun sameRetry(a: QueueRetryState, b: QueueRetryState) = a.sameAs(b)

    private fun project(snapshot: VaultSnapshot, projector: (EnterpriseQueueTransaction) -> Unit) {
        val transaction = BorrowedProjection(snapshot)
        try { projector(transaction) } finally { transaction.close() }
    }

    /** There is no transaction lifecycle or unscoped storage handle to retain. */
    private class BorrowedProjection(private val snapshot: VaultSnapshot) : EnterpriseQueueTransaction {
        private val owner = Thread.currentThread()
        private var open = true
        private fun check() { if (!open || Thread.currentThread() !== owner) throw EnterpriseLocalDenied() }
        override fun record(id: String): EnterpriseLocalRecord? {
            check(); EnterprisePartition.requireId(id)
            return snapshot.entries["RECORD:$id"]?.let { EnterpriseLocalRecord(snapshot.partition, id, it.bytes) }
        }
        override fun put(record: EnterpriseLocalRecord) {
            check()
            if (record.partition != snapshot.partition) throw EnterpriseLocalDenied()
            snapshot.entries["RECORD:" + record.id] = VaultEntry(VaultKind.RECORD, record.id, "", record.envelope)
        }
        fun close() { open = false }
    }
}
