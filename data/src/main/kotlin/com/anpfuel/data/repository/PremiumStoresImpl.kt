package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.PremiumCursorDao
import com.anpfuel.data.local.dao.PremiumOutboxDao
import com.anpfuel.data.mapper.PremiumMapper
import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumCursor
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.repository.PremiumCommandStore
import com.anpfuel.domain.repository.PremiumCursorStore
import javax.inject.Inject

/**
 * PRM09-T01 Room-backed durable stores.
 *
 * Every query is owner/origin-bound; callers are suspend and run off the
 * main thread. No generic replication: typed commands and opaque cursors
 * only.
 */
class PremiumCommandStoreImpl @Inject constructor(
    private val outbox: PremiumOutboxDao,
) : PremiumCommandStore {

    override suspend fun enqueue(command: PremiumCommand) {
        val existing = outbox.findById(
            command.operationId,
            command.scope.accountId,
            command.scope.origin,
        )
        if (existing != null) {
            val current = PremiumMapper.toDomain(existing)
            require(
                current.dataset == command.dataset &&
                    current.recordId == command.recordId &&
                    current.payloadJson == command.payloadJson,
            ) { "divergent operation reuse" }
            return
        }
        outbox.upsert(PremiumMapper.toEntity(command))
    }

    override suspend fun findPending(
        scope: PremiumOwnerScope,
        operationId: String,
    ): PremiumCommand? =
        outbox.findById(operationId, scope.accountId, scope.origin)
            ?.let(PremiumMapper::toDomain)

    override suspend fun listPending(scope: PremiumOwnerScope, limit: Int): List<PremiumCommand> =
        outbox.listOwned(scope.accountId, scope.origin, limit.coerceIn(1, 50))
            .map(PremiumMapper::toDomain)

    override suspend fun remove(scope: PremiumOwnerScope, operationId: String) {
        outbox.deleteById(operationId, scope.accountId, scope.origin)
    }

    override suspend fun clearOwner(scope: PremiumOwnerScope) {
        outbox.clearOwner(scope.accountId, scope.origin)
    }

    override suspend fun releaseExpired(nowMillis: Long): Int =
        outbox.releaseExpired(nowMillis)

    override suspend fun claim(
        operationId: String,
        scope: PremiumOwnerScope,
        nonce: String,
        leaseUntilMillis: Long,
    ): Boolean = outbox.claim(operationId, scope.accountId, scope.origin, nonce, leaseUntilMillis) > 0

    override suspend fun failTransient(
        scope: PremiumOwnerScope,
        operationId: String,
        nonce: String,
        nowMillis: Long,
    ) {
        val current = outbox.findById(operationId, scope.accountId, scope.origin) ?: return
        val attempts = current.attempts + 1
        val backoff = minOf(30_000L shl minOf(attempts, 7), 3_600_000L)
        outbox.failClaim(operationId, scope.accountId, scope.origin, nonce, attempts, nowMillis + backoff)
    }

    override suspend fun markConflict(scope: PremiumOwnerScope, operationId: String, reason: String) {
        outbox.markConflict(operationId, scope.accountId, scope.origin)
    }

    override suspend fun acknowledge(scope: PremiumOwnerScope, operationId: String) {
        outbox.deleteById(operationId, scope.accountId, scope.origin)
    }
}

class PremiumCursorStoreImpl @Inject constructor(
    private val cursors: PremiumCursorDao,
    private val clock: () -> Long = { System.currentTimeMillis() },
) : PremiumCursorStore {

    override suspend fun load(scope: PremiumOwnerScope, dataset: String): PremiumCursor? =
        cursors.find(scope.accountId, scope.origin, dataset)
            ?.let(PremiumMapper::cursorToDomain)

    override suspend fun save(scope: PremiumOwnerScope, dataset: String, cursor: PremiumCursor) {
        val existing = cursors.find(scope.accountId, scope.origin, dataset)
        cursors.upsert(
            PremiumMapper.cursorToEntity(
                scope,
                dataset,
                cursor,
                existing?.epoch ?: 1L,
                clock(),
            ),
        )
    }

    override suspend fun clearOwner(scope: PremiumOwnerScope) {
        cursors.clearOwner(scope.accountId, scope.origin)
    }
}
