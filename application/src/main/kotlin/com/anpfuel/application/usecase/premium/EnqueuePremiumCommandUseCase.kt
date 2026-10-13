package com.anpfuel.application.usecase.premium

import com.anpfuel.domain.premium.PremiumBounds
import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.repository.PremiumCommandStore
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * PRM09-T01 durable enqueue.
 *
 * Validates the typed command, persists it before any upload attempt and
 * returns the stored intent. Duplicate `operationId` converges (same id is
 * an idempotent replay, divergent payloads refuse). Runs off the main
 * thread; failures keep local records intact for retry. Paid gating rides
 * the server capability at dispatch, not here.
 */
class EnqueuePremiumCommandUseCase(
    private val store: PremiumCommandStore,
    private val io: CoroutineDispatcher = Dispatchers.IO,
) {
    suspend fun invoke(command: PremiumCommand): PremiumCommand = withContext(io) {
        val pending = store.findPending(command.scope, command.operationId)
        if (pending != null) {
            require(
                pending.dataset == command.dataset &&
                    pending.recordId == command.recordId &&
                    pending.payloadJson == command.payloadJson,
            ) { "divergent operation reuse" }
            return@withContext pending
        }
        val queued = store.listPending(command.scope, PremiumBounds.MAX_COMMANDS_PER_BATCH + 1)
        require(queued.size < PremiumBounds.MAX_COMMANDS_PER_BATCH + 1) {
            "batch-bound"
        }
        store.enqueue(command)
        command
    }
}
