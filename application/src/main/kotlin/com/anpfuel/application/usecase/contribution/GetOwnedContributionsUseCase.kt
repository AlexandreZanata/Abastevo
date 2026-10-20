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

data class OwnedContributionsPage(val items: List<OwnedContributionStatus>, val nextOffset: Int?, val scopeKey: String?)

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
        return page(limit, offset).items
    }

    suspend fun page(limit: Int = 20, offset: Int = 0): OwnedContributionsPage {
        require(limit in 1..OutboxBudgets.HISTORY_PAGE_SIZE && offset >= 0)
        val scope = scopeProvider?.currentScope()
        if (scopeProvider != null && scope == null) return OwnedContributionsPage(emptyList(), null, null)
        val raw = outbox.listOwnedPage(limit, offset)
        if (scopeProvider != null && scopeProvider.currentScope() != scope) return OwnedContributionsPage(emptyList(), null, null)
        val items = raw.filter { scopeProvider == null || it.scope == scope }.map { owned ->
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
        // Advance by rows actually read, including filtered foreign rows: no owner is hidden behind an empty page.
        return OwnedContributionsPage(items, if (raw.size == limit) offset + raw.size else null,
            scope?.let { "${it.ownerScope}|${it.origin}" })
    }
}

class CancelOwnedContributionUseCase(
    private val outbox: ContributionOutboxRepository,
    private val scopeProvider: ContributionScopeProvider? = null,
) {
    suspend fun invoke(commandId: String) {
        if (commandId.isBlank()) throw DomainException("command_id is blank")
        if (scopeProvider != null) {
            val scope = scopeProvider.currentScope() ?: throw DomainException("owned contribution unavailable")
            val command = outbox.findOwned(commandId)
                ?.takeIf { it.scope == scope }
                ?: throw DomainException("owned contribution unavailable")
            if (command.remoteStatus != null) throw DomainException("already submitted contribution cannot be cancelled locally")
        }
        outbox.cancel(commandId)
    }
}
