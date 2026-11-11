package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterprisePersistence
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterprisePendingOperation

/**
 * Synthetic foundation adapter; deliberately absent from production DI.
 * EMP-PRIV-003 must select/prove encryption of DB, journals and files first.
 */
class RoomEnterprisePersistence(private val dao: EnterpriseDao) : EnterprisePersistence {
    override suspend fun record(partition: EnterprisePartition, id: String): EnterpriseLocalRecord? =
        dao.record(partition.ownerAccount, partition.origin, partition.organizationId, EnterprisePartition.requireId(id))?.let {
            EnterpriseLocalRecord(EnterprisePartition.of(it.ownerAccount, it.origin, it.organizationId), it.id, it.envelope)
        }

    override suspend fun put(record: EnterpriseLocalRecord) {
        val p = record.partition
        dao.put(EnterpriseRecordRow(p.ownerAccount, p.origin, p.organizationId, record.id, record.envelope))
    }

    override suspend fun enqueue(operation: EnterprisePendingOperation): Boolean {
        val p = operation.partition
        return dao.enqueue(EnterpriseOperationRow(p.ownerAccount, p.origin, p.organizationId, operation.id, operation.targetId, operation.envelope))
    }

    override suspend fun pending(partition: EnterprisePartition, limit: Int): List<EnterprisePendingOperation> {
        require(limit in 1..50) { "Invalid enterprise page size" }
        return dao.pending(partition.ownerAccount, partition.origin, partition.organizationId, limit).map {
            EnterprisePendingOperation(EnterprisePartition.of(it.ownerAccount, it.origin, it.organizationId), it.id, it.targetId, it.envelope)
        }
    }
}
