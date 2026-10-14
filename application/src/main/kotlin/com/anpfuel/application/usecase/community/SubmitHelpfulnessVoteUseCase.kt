package com.anpfuel.application.usecase.community

import com.anpfuel.application.port.CommunityHelpfulnessFlagProvider
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.repository.CommunityHelpfulnessChoices
import com.anpfuel.domain.repository.CommunityHelpfulnessException
import com.anpfuel.domain.repository.CommunityHelpfulnessGateway
import com.anpfuel.domain.repository.CommunityHelpfulnessReceipt
import com.anpfuel.domain.repository.CommunityHelpfulnessRejectKind
import com.anpfuel.domain.repository.CommunityHelpfulnessTally

/**
 * CS08A — Helpful/unhelpful votes (B-BR-CS01, BUC-CS02).
 *
 * Flag disabled: [Outcome.Disabled], no network IO, recorded votes
 * untouched. Otherwise a cast records one frozen choice bound to the
 * immutable decision sequence the contributor saw; choice changes reuse
 * the caller's intent (the backend rewrites in place) and removals
 * converge as idempotent success. Rejections carry fixed kinds with
 * sanitized messages — counts are exact tallies, never truth claims.
 *
 * No vote amplification: the caller reuses one stable
 * `client_submission_id` per intent; an identical retry converges to
 * the same vote (`replayed = true`) instead of adding weight.
 */
sealed interface HelpfulnessVoteOutcome {
    data object Disabled : HelpfulnessVoteOutcome
    data class Voted(
        val receipt: CommunityHelpfulnessReceipt,
        val replayed: Boolean,
    ) : HelpfulnessVoteOutcome
    data class Removed(
        val tally: CommunityHelpfulnessTally,
    ) : HelpfulnessVoteOutcome
    data class Tallied(
        val tally: CommunityHelpfulnessTally,
    ) : HelpfulnessVoteOutcome
    data class Rejected(
        val kind: CommunityHelpfulnessRejectKind,
        val message: String,
    ) : HelpfulnessVoteOutcome
}

class SubmitHelpfulnessVoteUseCase(
    private val flagProvider: CommunityHelpfulnessFlagProvider,
    private val gateway: CommunityHelpfulnessGateway,
) {
    data class CastRequest(
        val observationId: String,
        val clientSubmissionId: String,
        val choiceWire: String,
        val decisionSequence: Long,
        val expectedOwnerScope: String? = null,
    )

    data class RemoveRequest(
        val observationId: String,
        val decisionSequence: Long,
        val expectedOwnerScope: String? = null,
    )

    data class TallyRequest(
        val observationId: String,
        val decisionSequence: Long,
        val expectedOwnerScope: String? = null,
    )

    suspend fun cast(request: CastRequest): HelpfulnessVoteOutcome {
        if (!flagProvider.isEnabled()) {
            return HelpfulnessVoteOutcome.Disabled
        }
        if (request.observationId.isBlank()) {
            throw DomainException("observation_id is blank")
        }
        if (request.clientSubmissionId.isBlank()) {
            throw DomainException("client_submission_id is blank")
        }
        if (request.choiceWire !in CommunityHelpfulnessChoices.ALL) {
            throw DomainException("unknown helpfulness choice")
        }
        if (request.decisionSequence < 1) {
            throw DomainException("decision_sequence must be positive")
        }
        return try {
            val receipt = gateway.castHelpfulness(
                request.observationId,
                request.clientSubmissionId,
                request.choiceWire,
                request.decisionSequence,
                request.expectedOwnerScope,
            )
            HelpfulnessVoteOutcome.Voted(receipt, receipt.replayed)
        } catch (voteError: CommunityHelpfulnessException) {
            HelpfulnessVoteOutcome.Rejected(voteError.kind, voteError.message ?: voteError.kind.name)
        } catch (transport: Exception) {
            HelpfulnessVoteOutcome.Rejected(
                CommunityHelpfulnessRejectKind.TRANSPORT,
                "helpfulness transport failed",
            )
        }
    }

    suspend fun remove(request: RemoveRequest): HelpfulnessVoteOutcome {
        if (!flagProvider.isEnabled()) {
            return HelpfulnessVoteOutcome.Disabled
        }
        if (request.observationId.isBlank()) {
            throw DomainException("observation_id is blank")
        }
        if (request.decisionSequence < 1) {
            throw DomainException("decision_sequence must be positive")
        }
        return try {
            HelpfulnessVoteOutcome.Removed(
                gateway.removeHelpfulness(request.observationId, request.decisionSequence,request.expectedOwnerScope),
            )
        } catch (voteError: CommunityHelpfulnessException) {
            HelpfulnessVoteOutcome.Rejected(voteError.kind, voteError.message ?: voteError.kind.name)
        } catch (transport: Exception) {
            HelpfulnessVoteOutcome.Rejected(
                CommunityHelpfulnessRejectKind.TRANSPORT,
                "helpfulness transport failed",
            )
        }
    }

    suspend fun tally(request: TallyRequest): HelpfulnessVoteOutcome {
        if (!flagProvider.isEnabled()) {
            return HelpfulnessVoteOutcome.Disabled
        }
        if (request.observationId.isBlank()) {
            throw DomainException("observation_id is blank")
        }
        if (request.decisionSequence < 1) {
            throw DomainException("decision_sequence must be positive")
        }
        return try {
            HelpfulnessVoteOutcome.Tallied(
                gateway.tallyFor(request.observationId, request.decisionSequence,request.expectedOwnerScope),
            )
        } catch (voteError: CommunityHelpfulnessException) {
            HelpfulnessVoteOutcome.Rejected(voteError.kind, voteError.message ?: voteError.kind.name)
        } catch (transport: Exception) {
            HelpfulnessVoteOutcome.Rejected(
                CommunityHelpfulnessRejectKind.TRANSPORT,
                "helpfulness transport failed",
            )
        }
    }
}
