package com.anpfuel.application.usecase.contribution

import com.anpfuel.application.port.ContributionScopeProvider
import com.anpfuel.domain.contribution.ContributionLocalPhase
import com.anpfuel.domain.contribution.ContributionReview
import com.anpfuel.domain.contribution.ContributionState
import com.anpfuel.domain.contribution.ContributionStateRule
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.repository.ContributionOutboxRepository
import com.anpfuel.domain.repository.OwnedContributionPhase
import com.anpfuel.domain.repository.OutboxBudgets
import com.anpfuel.domain.valueobject.FuelProduct

/**
 * P21-T03 — Private owner status (BUC-003, B-BR-003/005).
 *
 * Maps durable outbox phases to the frozen P21-T01 presentation states.
 * Scoped durable receipts preserve actual server validation through restart.
 * Foreign account/environment commands are hidden; cancellation never
 * removes an already received fact.
 */
data class OwnedContributionStatus(
    val commandId: String,
    val revision: Int,
    val attempts: Int,
    val state: ContributionState?,
    val reason: String? = null,
    val fuelProduct: FuelProduct? = null,
)

class GetOwnedContributionsUseCase(
    private val outbox: ContributionOutboxRepository,
    private val scopeProvider: ContributionScopeProvider? = null,
) {
    /**
     * SEC06-C bounded history: newest-first page (default covers the
     * status screen; deeper pages pass explicit offsets). The full
     * table is never loaded for display.
     */
    suspend fun invoke(limit: Int = OutboxBudgets.HISTORY_PAGE_SIZE, offset: Int = 0): List<OwnedContributionStatus> {
        val scope = scopeProvider?.currentScope()
        return outbox.listOwnedPage(limit, offset).filter { scope == null || it.scope == scope }.map { owned ->
            val local = when (owned.phase) {
                OwnedContributionPhase.QUEUED -> ContributionLocalPhase.NOT_SENT
                OwnedContributionPhase.IN_FLIGHT -> ContributionLocalPhase.IN_FLIGHT
                OwnedContributionPhase.FAILED -> ContributionLocalPhase.TRANSPORT_FAILED
                OwnedContributionPhase.CANCELLED -> ContributionLocalPhase.CANCELLED
                OwnedContributionPhase.ACKNOWLEDGED -> ContributionLocalPhase.ACKNOWLEDGED
            }
            OwnedContributionStatus(
                commandId = owned.commandId,
                revision = owned.revision,
                attempts = owned.attempts,
                state = ContributionStateRule.resolve(local, owned.remoteStatus, ContributionReview.NONE),
                reason = owned.reason,
                fuelProduct = owned.fuelProduct,
            )
        }
    }
}

class CancelOwnedContributionUseCase(
    private val outbox: ContributionOutboxRepository,
    private val scopeProvider: ContributionScopeProvider? = null,
) {
    suspend fun invoke(commandId: String) {
        if (commandId.isBlank()) throw DomainException("command_id is blank")
        if (scopeProvider != null) {
            val scope = scopeProvider.currentScope()
            val command = outbox.findOwned(commandId)
                ?.takeIf { it.scope == scope }
                ?: throw DomainException("owned contribution unavailable")
            if (command.remoteStatus != null) throw DomainException("already submitted contribution cannot be cancelled locally")
        }
        outbox.cancel(commandId)
    }
}
