package com.anpfuel.application.usecase.contribution

import com.anpfuel.application.port.ContributionScopeProvider
import com.anpfuel.domain.contribution.ContributionLocalPhase
import com.anpfuel.domain.contribution.ContributionReview
import com.anpfuel.domain.contribution.ContributionState
import com.anpfuel.domain.contribution.ContributionStateRule
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.repository.ContributionOutboxRepository
import com.anpfuel.domain.repository.OwnedContributionPhase

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
)

class GetOwnedContributionsUseCase(
    private val outbox: ContributionOutboxRepository,
    private val scopeProvider: ContributionScopeProvider? = null,
) {
    suspend fun invoke(): List<OwnedContributionStatus> {
        val scope = scopeProvider?.currentScope()
        return outbox.listOwned().filter { scope == null || it.scope == scope }.map { owned ->
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
            val command = outbox.listOwned().firstOrNull { it.commandId == commandId && it.scope == scope }
                ?: throw DomainException("owned contribution unavailable")
            if (command.remoteStatus != null) throw DomainException("already submitted contribution cannot be cancelled locally")
        }
        outbox.cancel(commandId)
    }
}
