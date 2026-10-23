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

    /**
     * MEX-11 restart recovery: crashed IN_FLIGHT claims whose lease
     * expired return to QUEUED with their original operation IDs.
     * Returns the released count.
     */
    suspend fun releaseExpired(nowMillis: Long): Int

    /**
     * MEX-14 atomic claim: at most one worker owns a command until the
     * lease lapses. Returns false when another worker won the race.
     */
    suspend fun claim(operationId: String, scope: PremiumOwnerScope, nonce: String, leaseUntilMillis: Long): Boolean

    /** Transient failure: bounded backoff, stays dispatchable. */
    suspend fun failTransient(scope: PremiumOwnerScope, operationId: String, nonce: String, nowMillis: Long)

    /** Terminal conflict (e.g. stale expected revision): never
     * dispatchable again, held for an explicit correction. */
    suspend fun markConflict(scope: PremiumOwnerScope, operationId: String, reason: String)

    /** Server acknowledgement: the command is durable remotely. */
    suspend fun acknowledge(scope: PremiumOwnerScope, operationId: String)
}

interface PremiumCursorStore {
    suspend fun load(scope: PremiumOwnerScope, dataset: String): PremiumCursor?
    suspend fun save(scope: PremiumOwnerScope, dataset: String, cursor: PremiumCursor)
    suspend fun clearOwner(scope: PremiumOwnerScope)
}
