package com.anpfuel.data.enterprise.attachment

import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.attachment.EnterpriseAttachmentSnapshot
import com.anpfuel.application.enterprise.attachment.EnterpriseAttachmentStore
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.data.enterprise.ProtectedEnterprisePersistence
import com.anpfuel.data.enterprise.VaultEntry
import com.anpfuel.data.enterprise.VaultKind
import com.anpfuel.data.enterprise.VaultQueueChange
import com.anpfuel.data.enterprise.VaultSnapshot
import com.anpfuel.data.enterprise.protectedqueue.ProtectedQueueCodec
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.attachment.AttachmentBook
import com.anpfuel.domain.enterprise.attachment.AttachmentLocalException
import com.anpfuel.domain.enterprise.attachment.AttachmentLocalFailure
import com.anpfuel.domain.enterprise.attachment.AttachmentPolicy
import com.anpfuel.domain.enterprise.attachment.AttachmentState
import com.anpfuel.domain.enterprise.attachment.EnterpriseAttachment

/**
 * The EMP-OFFL-005 local attachment storage (EDR-267): the sealed `attachment-book` RECORD and one sealed
 * `ATTACHMENT` entry per attachment of the exact partition's protected generation, read and written through the
 * attempt's custody, enrollment and lease checks. No plaintext file, path or table exists.
 */
class ProtectedAttachmentStore(private val persistence: ProtectedEnterprisePersistence) : EnterpriseAttachmentStore {
    private fun book(snapshot: VaultSnapshot, attempt: EnterpriseQueueAttempt): AttachmentBook? {
        val stored = snapshot.entries[BOOK_KEY] ?: return null
        // The index is local evidence of unsent work: an unreadable or foreign one is unavailable and never replaced.
        return try {
            AttachmentBook.decode(stored.bytes).requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
        } catch (_: Exception) { throw EnterprisePersistenceUnavailable() }
    }

    private fun put(snapshot: VaultSnapshot, book: AttachmentBook) {
        snapshot.entries[BOOK_KEY] = VaultEntry(VaultKind.RECORD, AttachmentBook.RECORD_ID, "", book.canonicalBytes)
    }

    override suspend fun capture(attempt: EnterpriseQueueAttempt, attachment: EnterpriseAttachment, bytes: ByteArray) {
        val copy = bytes.copyOf()
        try {
            if (copy.size.toLong() != attachment.bytes || AttachmentPolicy.sha256(copy) != attachment.sha256) throw EnterprisePersistenceUnavailable()
            val refused = persistence.queueAccess(attempt) { snapshot ->
                val owner = snapshot.entries["QUEUE:" + attachment.ownerOperationId]
                    ?: return@queueAccess VaultQueueChange<AttachmentLocalFailure?>(AttachmentLocalFailure.UNKNOWN_OPERATION)
                // The owner must be this partition's own retained operation (BR-OFFL-003).
                ProtectedQueueCodec.decode(owner.bytes, snapshot, owner.id)
                val current = book(snapshot, attempt) ?: AttachmentBook.empty(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
                val next = try { current.withCapture(attachment) } catch (local: AttachmentLocalException) {
                    return@queueAccess VaultQueueChange<AttachmentLocalFailure?>(local.failure)
                }
                put(snapshot, next)
                snapshot.entries[ATTACHMENT_PREFIX + attachment.documentId] = VaultEntry(VaultKind.ATTACHMENT, attachment.documentId, "", copy.copyOf())
                VaultQueueChange<AttachmentLocalFailure?>(null, true)
            }
            if (refused != null) throw AttachmentLocalException(refused)
        } finally { copy.fill(0) }
    }

    override suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseAttachmentSnapshot = persistence.queueAccess(attempt) { snapshot ->
        val book = book(snapshot, attempt)
        val owners = book?.items.orEmpty().map { it.ownerOperationId }.distinct().mapNotNull { id ->
            snapshot.entries["QUEUE:$id"]?.let { id to ProtectedQueueCodec.decode(it.bytes, snapshot, it.id) }
        }.toMap()
        VaultQueueChange(EnterpriseAttachmentSnapshot(book, owners))
    }

    override suspend fun bytes(attempt: EnterpriseQueueAttempt, documentId: String): ByteArray? {
        EnterprisePartition.requireId(documentId)
        return persistence.queueAccess(attempt) { snapshot ->
            VaultQueueChange(snapshot.entries[ATTACHMENT_PREFIX + documentId]?.bytes?.copyOf())
        }
    }

    override suspend fun transition(attempt: EnterpriseQueueAttempt, expected: EnterpriseAttachment, next: EnterpriseAttachment): Boolean =
        persistence.queueAccess(attempt) { snapshot ->
            val current = book(snapshot, attempt) ?: return@queueAccess VaultQueueChange(false)
            val replaced = current.replace(expected, next) ?: return@queueAccess VaultQueueChange(false)
            put(snapshot, replaced)
            // Verified bytes leave the device in the write that records the verification (EDR-267).
            if (next.state == AttachmentState.STORED) snapshot.entries.remove(ATTACHMENT_PREFIX + next.documentId)
            VaultQueueChange(true, true)
        }

    override fun toString(): String = "ProtectedAttachmentStore(redacted)"

    private companion object {
        val BOOK_KEY = "RECORD:" + AttachmentBook.RECORD_ID
        const val ATTACHMENT_PREFIX = "ATTACHMENT:"
    }
}
