package com.anpfuel.data.enterprise.checklist

import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.checklist.EnterpriseChecklistProducers
import com.anpfuel.application.enterprise.checklist.EnterpriseChecklistSnapshot
import com.anpfuel.application.enterprise.checklist.EnterpriseChecklistStore
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.data.enterprise.ProtectedEnterprisePersistence
import com.anpfuel.data.enterprise.VaultEntry
import com.anpfuel.data.enterprise.VaultKind
import com.anpfuel.data.enterprise.VaultQueueChange
import com.anpfuel.data.enterprise.VaultSnapshot
import com.anpfuel.data.enterprise.protectedqueue.ProtectedQueueCodec
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistBook
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistLocalException

/**
 * The EMP-CHECK-002 local checklist storage (EDR-283): the sealed `checklist-book` RECORD of the exact partition's
 * protected generation and the retained queue entries of its completed runs, read and written through the attempt's
 * custody, enrollment and lease checks. Answers are written without an operation in one locked read-modify-write; the
 * completion is written by the `check.run.complete` producer in the queue's own write. No plaintext table exists.
 */
class ProtectedChecklistStore(private val persistence: ProtectedEnterprisePersistence) : EnterpriseChecklistStore {
    /** The index is business evidence: an unreadable or foreign one is unavailable and never replaced (EDR-283). */
    private fun book(snapshot: VaultSnapshot, attempt: EnterpriseQueueAttempt): ChecklistBook? {
        val stored = snapshot.entries[KEY] ?: return null
        return try {
            ChecklistBook.decode(stored.bytes).requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
        } catch (_: Exception) { throw EnterprisePersistenceUnavailable() }
    }

    override suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseChecklistSnapshot = persistence.queueAccess(attempt) { snapshot ->
        val book = book(snapshot, attempt) ?: return@queueAccess VaultQueueChange(EnterpriseChecklistSnapshot(null, emptyMap()))
        val entries = book.runs.mapNotNull { it.operationId }.associateWith { id ->
            val entry = snapshot.entries["QUEUE:$id"] ?: throw EnterprisePersistenceUnavailable()
            ProtectedQueueCodec.decode(entry.bytes, snapshot, entry.id)
        }
        VaultQueueChange(EnterpriseChecklistSnapshot(book, entries))
    }

    override suspend fun update(attempt: EnterpriseQueueAttempt, change: (ChecklistBook) -> ChecklistBook): ChecklistBook {
        val member = attempt.enrollment.target.membershipId
        val enrollment = attempt.enrollment.enrollmentId
        var refusal: ChecklistLocalException? = null
        val result = persistence.queueAccess(attempt) { snapshot ->
            val current = book(snapshot, attempt) ?: ChecklistBook.empty(member, enrollment)
            // A local refusal is the caller's answer, not an outage: keep it and write nothing.
            val next = try { change(current) } catch (refused: ChecklistLocalException) {
                refusal = refused
                return@queueAccess VaultQueueChange<ChecklistBook?>(null)
            }
            next.requireBinding(member, enrollment)
            val bytes = next.canonicalBytes
            if (snapshot.entries[KEY]?.bytes?.contentEquals(bytes) == true) VaultQueueChange(next)
            else {
                snapshot.entries[KEY] = VaultEntry(VaultKind.RECORD, EnterpriseChecklistProducers.BOOK_RECORD_ID, "", bytes)
                VaultQueueChange(next, changed = true)
            }
        }
        refusal?.let { throw it }
        return result ?: throw EnterprisePersistenceUnavailable()
    }

    override fun toString(): String = "ProtectedChecklistStore(redacted)"

    private companion object { const val KEY = "RECORD:" + EnterpriseChecklistProducers.BOOK_RECORD_ID }
}
