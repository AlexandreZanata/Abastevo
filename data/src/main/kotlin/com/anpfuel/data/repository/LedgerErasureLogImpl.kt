package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.LedgerErasedDao
import com.anpfuel.data.local.entity.LedgerErasedEntity
import com.anpfuel.domain.repository.LedgerErasureLog
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class LedgerErasureLogImpl @Inject constructor(
    private val dao: LedgerErasedDao,
) : LedgerErasureLog {

    override suspend fun recordErased(accountId: String, origin: String, dataset: String, recordId: String, nowMillis: Long) {
        if (accountId.isBlank() || dataset.isBlank() || recordId.isBlank()) return
        dao.record(
            LedgerErasedEntity(
                ownerAccount = accountId, origin = origin, dataset = dataset,
                recordId = recordId, erasedAtMillis = nowMillis,
            ),
        )
    }

    override suspend fun isErased(accountId: String, origin: String, dataset: String, recordId: String): Boolean {
        if (accountId.isBlank() || dataset.isBlank() || recordId.isEmpty()) return false
        return dao.isErased(accountId, origin, dataset, recordId)
    }

    override suspend fun clearOwner(accountId: String, origin: String) {
        if (accountId.isBlank()) return
        dao.clearOwner(accountId, origin)
    }
}
