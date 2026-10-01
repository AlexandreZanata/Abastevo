package com.anpfuel.application.usecase.contribution

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
 * No receipt/review feed exists locally, so remote and review stay empty:
 * acknowledged commands are deleted on ack (server truth arrives with
 * review coverage). Cancelled commands resolve to no status.
 */
data class OwnedContributionStatus(
    val commandId: String,
    val revision: Int,
    val attempts: Int,
    val state: ContributionState?,
)

class GetOwnedContributionsUseCase(
    private val outbox: ContributionOutboxRepository,
) {
    suspend fun invoke(): List<OwnedContributionStatus> =
        outbox.listOwned().map { owned ->
            val local = when (owned.phase) {
                OwnedContributionPhase.QUEUED -> ContributionLocalPhase.NOT_SENT
                OwnedContributionPhase.IN_FLIGHT -> ContributionLocalPhase.IN_FLIGHT
                OwnedContributionPhase.FAILED -> ContributionLocalPhase.TRANSPORT_FAILED
                OwnedContributionPhase.CANCELLED -> ContributionLocalPhase.CANCELLED
            }
            OwnedContributionStatus(
                commandId = owned.commandId,
                revision = owned.revision,
                attempts = owned.attempts,
                state = ContributionStateRule.resolve(local, null, ContributionReview.NONE),
            )
        }
}

class CancelOwnedContributionUseCase(
    private val outbox: ContributionOutboxRepository,
) {
    suspend fun invoke(commandId: String) {
        if (commandId.isBlank()) throw DomainException("command_id is blank")
        outbox.cancel(commandId)
    }
}
