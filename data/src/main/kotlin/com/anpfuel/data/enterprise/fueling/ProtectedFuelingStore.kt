package com.anpfuel.data.enterprise.fueling

import com.anpfuel.application.enterprise.fueling.EnterpriseFuelingStore
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.data.enterprise.ProtectedEnterprisePersistence
import com.anpfuel.data.enterprise.VaultKind
import com.anpfuel.data.enterprise.VaultQueueChange
import com.anpfuel.data.enterprise.protectedqueue.ProtectedQueueCodec
import com.anpfuel.domain.enterprise.fueling.FuelingOperations
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry

/**
 * The EMP-FUEL-011 local fueling read (EDR-295): the retained `fueling.submit` entries of the exact partition's
 * protected generation, read in one snapshot through the attempt's custody, enrollment and lease checks. The
 * operation is the whole record; no separate index, plaintext table or file exists.
 */
class ProtectedFuelingStore(private val persistence: ProtectedEnterprisePersistence) : EnterpriseFuelingStore {
    override suspend fun read(attempt: EnterpriseQueueAttempt): List<EnterpriseQueueEntry> = persistence.queueAccess(attempt) { snapshot ->
        // Every retained operation is decoded under the partition's own framing, like the queue's pending read;
        // an unreadable entry is unavailable, never skipped (it could be a fueling).
        VaultQueueChange(snapshot.entries.values.filter { it.kind == VaultKind.QUEUE }
            .map { ProtectedQueueCodec.decode(it.bytes, snapshot, it.id) }
            .filter { it.operation.type == FuelingOperations.TYPE })
    }
    override fun toString(): String = "ProtectedFuelingStore(redacted)"
}
