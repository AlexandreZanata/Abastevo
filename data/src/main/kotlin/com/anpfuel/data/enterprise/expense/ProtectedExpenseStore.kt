package com.anpfuel.data.enterprise.expense

import com.anpfuel.application.enterprise.expense.EnterpriseExpenseStore
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.data.enterprise.ProtectedEnterprisePersistence
import com.anpfuel.data.enterprise.VaultKind
import com.anpfuel.data.enterprise.VaultQueueChange
import com.anpfuel.data.enterprise.protectedqueue.ProtectedQueueCodec
import com.anpfuel.domain.enterprise.expense.ExpenseOperations
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry

/**
 * The EMP-EXP-004 local expense read (EDR-336): the retained `expense.submit` entries of the exact partition's
 * protected generation, read in one snapshot through the attempt's custody, enrollment and lease checks. The
 * operation is the whole record; no separate index, plaintext table or file exists.
 */
class ProtectedExpenseStore(private val persistence: ProtectedEnterprisePersistence) : EnterpriseExpenseStore {
    override suspend fun read(attempt: EnterpriseQueueAttempt): List<EnterpriseQueueEntry> = persistence.queueAccess(attempt) { snapshot ->
        // Every retained operation is decoded under the partition's own framing, like the queue's pending read;
        // an unreadable entry is unavailable, never skipped (it could be an expense).
        VaultQueueChange(snapshot.entries.values.filter { it.kind == VaultKind.QUEUE }
            .map { ProtectedQueueCodec.decode(it.bytes, snapshot, it.id) }
            .filter { it.operation.type == ExpenseOperations.SUBMIT })
    }
    override fun toString(): String = "ProtectedExpenseStore(redacted)"
}
