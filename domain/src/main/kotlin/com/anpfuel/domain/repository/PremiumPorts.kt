package com.anpfuel.domain.repository

import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumCursor
import com.anpfuel.domain.premium.PremiumOwnerScope

/**
 * PRM09-T01 owner-scoped durable ports.
 *
 * Implementations live in `:data` on Room; every method is partition-bound
 * to [PremiumOwnerScope] so an account/origin switch can never read or
 * replay another owner's commands or cursors. Calls are suspend and must
 * run off the main thread. No generic replication: typed datasets only.
 */
interface PremiumCommandStore {
    suspend fun enqueue(command: PremiumCommand)
    suspend fun findPending(scope: PremiumOwnerScope, operationId: String): PremiumCommand?
    suspend fun listPending(scope: PremiumOwnerScope, limit: Int): List<PremiumCommand>
    suspend fun remove(scope: PremiumOwnerScope, operationId: String)
    suspend fun clearOwner(scope: PremiumOwnerScope)
}

interface PremiumCursorStore {
    suspend fun load(scope: PremiumOwnerScope, dataset: String): PremiumCursor?
    suspend fun save(scope: PremiumOwnerScope, dataset: String, cursor: PremiumCursor)
    suspend fun clearOwner(scope: PremiumOwnerScope)
}
