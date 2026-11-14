package com.anpfuel.data.enterprise.protectedqueue

import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.sync.EnterpriseSyncCursor
import com.anpfuel.application.enterprise.sync.EnterpriseSyncPage
import com.anpfuel.application.enterprise.sync.EnterpriseSyncStatusStore
import com.anpfuel.data.enterprise.ProtectedEnterprisePersistence
import com.anpfuel.data.enterprise.VaultEntry
import com.anpfuel.data.enterprise.VaultKind
import com.anpfuel.data.enterprise.VaultQueueChange
import com.anpfuel.data.enterprise.VaultSnapshot
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncDetail
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncMode
import com.anpfuel.domain.enterprise.sync.*
import com.anpfuel.application.enterprise.sync.EnterpriseSyncPageChanged
import java.security.MessageDigest

/** Sanitized presentation and refused visibility share the accepted protected generation. */
class ProtectedEnterpriseSyncStatusStore(private val store: ProtectedEnterprisePersistence) : EnterpriseSyncStatusStore {
    override suspend fun page(attempt: EnterpriseQueueAttempt, mode: EnterpriseSyncMode,
        cursor: EnterpriseSyncCursor?, limit: Int): EnterpriseSyncPage {
        require(limit in 1..50)
        val read = store.queueStatusAccess(attempt) { snapshot, generation ->
            val stamp = Stamp(attempt)
            val previous = cursor as? Cursor
            if (cursor != null && (previous == null || previous.stamp != stamp || previous.mode != mode || previous.generation != generation)) {
                VaultQueueChange(PageRead(null))
            } else {
                val entries = snapshot.entries.values.filter { it.kind == VaultKind.QUEUE }.map {
                    ProtectedQueueCodec.decode(it.bytes, snapshot, it.id)
                }.sortedBy { it.operation.operationId }
                val full = entries.map(EnterpriseSyncProjection::item)
                val totals = EnterpriseSyncProjection.totals(full, storage(snapshot, entries))
                val candidates = full.filter { (mode == EnterpriseSyncMode.HISTORY || it.pending) &&
                    (previous == null || it.identity.operationId > previous.after) }
                val items = candidates.take(limit)
                val next = if (candidates.size > items.size) Cursor(stamp, mode, generation, items.last().identity.operationId) else null
                VaultQueueChange(PageRead(EnterpriseSyncPage(items, totals, generation, next, mode)))
            }
        }
        // A changed cursor never skips the actual-source checks on the way out.
        return read.page ?: throw EnterpriseSyncPageChanged()
    }

    override suspend fun detail(attempt: EnterpriseQueueAttempt, operationId: String,
        digest: String): EnterpriseSyncDetail {
        EnterprisePartition.requireId(operationId)
        return store.queueAccess(attempt) { snapshot ->
            val entry = retained(snapshot, operationId, digest)
            val attachments = entry.operation.attachments.map { declaration ->
                val local = snapshot.entries["ATTACHMENT:" + declaration.id]
                val evidence = when {
                    local == null -> EnterpriseAttachmentEvidence.MISSING
                    local.bytes.size.toLong() != declaration.bytes -> EnterpriseAttachmentEvidence.MISMATCH
                    sha(local.bytes) != declaration.sha256 -> EnterpriseAttachmentEvidence.MISMATCH
                    else -> EnterpriseAttachmentEvidence.MATCHING
                }
                EnterpriseSyncAttachment(declaration.id, declaration.bytes, declaration.sha256, declaration.mime, evidence)
            }
            VaultQueueChange(EnterpriseSyncDetail(EnterpriseSyncProjection.item(entry), attachments))
        }
    }

    /** Visibility is bound to the effective refused outcome: an original REJECTED receipt or a REFUSED resolution. */
    override suspend fun dismissRejected(attempt: EnterpriseQueueAttempt, operationId: String,
        digest: String, expectedOutcomeDigest: String): Boolean {
        EnterprisePartition.requireId(operationId)
        return store.queueAccess(attempt) { snapshot ->
            val entry = retained(snapshot, operationId, digest)
            val receipt = entry.acknowledgement ?: throw EnterprisePersistenceUnavailable()
            val resolution = entry.resolution
            val outcome = when {
                receipt.result == SyncResult.REJECTED -> receipt.canonicalBytes
                entry.refusedOutcome && resolution != null -> resolution.canonicalBytes
                else -> throw EnterprisePersistenceUnavailable()
            }
            if (expectedOutcomeDigest != "sha256:" + sha(outcome)) throw EnterprisePersistenceUnavailable()
            if (entry.dismissedRejected) VaultQueueChange(false)
            else {
                snapshot.entries["QUEUE:$operationId"] = VaultEntry(VaultKind.QUEUE, operationId, "",
                    ProtectedQueueCodec.encode(EnterpriseQueueEntry(entry.operation, entry.retry, receipt, true, resolution, entry.reviewRetry)))
                VaultQueueChange(true, true)
            }
        }
    }

    private fun retained(snapshot: VaultSnapshot, operationId: String, digest: String): EnterpriseQueueEntry {
        val stored = snapshot.entries["QUEUE:$operationId"] ?: throw EnterprisePersistenceUnavailable()
        val entry = ProtectedQueueCodec.decode(stored.bytes, snapshot, stored.id)
        if (entry.operation.payloadDigest != digest) throw EnterprisePersistenceUnavailable()
        return entry
    }

    private fun storage(snapshot: VaultSnapshot, entries: List<EnterpriseQueueEntry>): EnterpriseQueueStats {
        val pending = entries.filter { it.acknowledgement == null }
        return EnterpriseQueueStats(pending.size, pending.sumOf { snapshot.entries.getValue("QUEUE:" + it.operation.operationId).bytes.size.toLong() },
            snapshot.entries.values.count { it.kind == VaultKind.OPERATION }, snapshot.entries.size,
            2048L + snapshot.entries.values.sumOf { it.bytes.size.toLong() + 512 })
    }

    private fun sha(bytes: ByteArray) = MessageDigest.getInstance("SHA-256").digest(bytes)
        .joinToString("") { "%02x".format(it.toInt() and 255) }

    private class PageRead(val page: EnterpriseSyncPage?)
    /** This handle retains no delivery proof, callback or private business payload. */
    private data class Stamp(val partition: EnterprisePartition, val member: String, val enrollment: String,
        val revision: Long, val family: String, val epoch: Long, val background: Long) {
        constructor(attempt: EnterpriseQueueAttempt) : this(attempt.partition, attempt.enrollment.target.membershipId,
            attempt.enrollment.enrollmentId, attempt.enrollment.revision, attempt.account.identity.family,
            attempt.account.epoch, attempt.account.background)
        override fun toString(): String = "EnterpriseSyncCursorOwner(redacted)"
    }
    private class Cursor(val stamp: Stamp, val mode: EnterpriseSyncMode, val generation: Long,
        val after: String) : EnterpriseSyncCursor {
        override fun toString(): String = "EnterpriseSyncCursor(redacted)"
    }
}
