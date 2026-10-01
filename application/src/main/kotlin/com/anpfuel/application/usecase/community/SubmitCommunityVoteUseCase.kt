package com.anpfuel.application.usecase.community

import com.anpfuel.application.port.CommunityVoteFlagProvider
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.repository.CommunityDisputeReasons
import com.anpfuel.domain.repository.CommunityVoteException
import com.anpfuel.domain.repository.CommunityVoteGateway
import com.anpfuel.domain.repository.CommunityVoteReceipt
import com.anpfuel.domain.repository.CommunityVoteRejectKind

/**
 * P10-T07 — Structured corroboration and correction (BUC-005,
 * B-BR-005/006/007/011/012).
 *
 * Flag disabled: [Outcome.Disabled], no network IO, submitted records
 * untouched. Otherwise a confirmation affirms the exact amount, product,
 * unit and full condition the contributor was shown (the caller passes
 * the shown snapshot; blank snapshots refuse so a stale or different
 * condition is never confirmed accidentally). A dispute carries one of
 * the six wire reasons with an optional private detail (max 500 chars,
 * never echoed in errors) and an optional distinct replacement
 * observation id — a changed price is recorded as a new observation via
 * the contribution outbox (`supersedes_observation_id`), never as an
 * edit or a negative vote.
 *
 * No vote amplification: the caller reuses one stable
 * `client_submission_id` per intent; an identical retry converges to the
 * same vote (`replayed = true`) instead of adding weight, and an
 * already-recorded pair surfaces as [Outcome.Rejected] with
 * [CommunityVoteRejectKind.ALREADY_RECORDED] rather than a second vote.
 */
sealed interface CommunityVoteOutcome {
    data object Disabled : CommunityVoteOutcome
    data class Confirmed(
        val receipt: CommunityVoteReceipt,
        val replayed: Boolean,
    ) : CommunityVoteOutcome
    data class Disputed(
        val receipt: CommunityVoteReceipt,
        val state: String,
    ) : CommunityVoteOutcome
    data class Rejected(
        val kind: CommunityVoteRejectKind,
        val message: String,
    ) : CommunityVoteOutcome
}

class SubmitCommunityVoteUseCase(
    private val flagProvider: CommunityVoteFlagProvider,
    private val gateway: CommunityVoteGateway,
) {
    data class ConfirmRequest(
        val observationId: String,
        val clientSubmissionId: String,
        val shownConditionKind: String,
        val shownUnit: String,
    )

    data class DisputeRequest(
        val targetObservationId: String,
        val clientSubmissionId: String,
        val reasonWire: String,
        val detail: String? = null,
        val replacementObservationId: String? = null,
        val shownConditionKind: String,
    )

    suspend fun confirm(request: ConfirmRequest): CommunityVoteOutcome {
        if (!flagProvider.isEnabled()) {
            return CommunityVoteOutcome.Disabled
        }
        if (request.observationId.isBlank()) {
            throw DomainException("observation_id is blank")
        }
        if (request.clientSubmissionId.isBlank()) {
            throw DomainException("client_submission_id is blank")
        }
        if (request.shownConditionKind.isBlank()) {
            throw DomainException("shown condition is blank")
        }
        if (request.shownUnit.isBlank()) {
            throw DomainException("shown unit is blank")
        }
        return try {
            val receipt = gateway.submitConfirmation(
                request.observationId,
                request.clientSubmissionId,
            )
            CommunityVoteOutcome.Confirmed(receipt, receipt.replayed)
        } catch (voteError: CommunityVoteException) {
            CommunityVoteOutcome.Rejected(voteError.kind, voteError.message ?: voteError.kind.name)
        } catch (transport: Exception) {
            CommunityVoteOutcome.Rejected(
                CommunityVoteRejectKind.TRANSPORT,
                "vote transport failed",
            )
        }
    }

    suspend fun dispute(request: DisputeRequest): CommunityVoteOutcome {
        if (!flagProvider.isEnabled()) {
            return CommunityVoteOutcome.Disabled
        }
        if (request.targetObservationId.isBlank()) {
            throw DomainException("target_observation_id is blank")
        }
        if (request.clientSubmissionId.isBlank()) {
            throw DomainException("client_submission_id is blank")
        }
        if (request.reasonWire !in CommunityDisputeReasons.ALL) {
            throw DomainException("unknown dispute reason")
        }
        if (request.shownConditionKind.isBlank()) {
            throw DomainException("shown condition is blank")
        }
        val detail = request.detail?.takeIf { it.isNotBlank() }
        if (detail != null && detail.length > MAX_DISPUTE_DETAIL_CHARS) {
            throw DomainException("dispute detail over $MAX_DISPUTE_DETAIL_CHARS chars")
        }
        val replacement = request.replacementObservationId?.takeIf { it.isNotBlank() }
        if (replacement != null && replacement == request.targetObservationId) {
            throw DomainException("replacement must differ from target")
        }
        return try {
            val receipt = gateway.submitDispute(
                request.targetObservationId,
                request.clientSubmissionId,
                request.reasonWire,
                detail,
                replacement,
            )
            CommunityVoteOutcome.Disputed(receipt, DISPUTE_STATE_OPEN)
        } catch (voteError: CommunityVoteException) {
            CommunityVoteOutcome.Rejected(voteError.kind, voteError.message ?: voteError.kind.name)
        } catch (transport: Exception) {
            CommunityVoteOutcome.Rejected(
                CommunityVoteRejectKind.TRANSPORT,
                "vote transport failed",
            )
        }
    }

    companion object {
        /** Backend dispute detail cap (API_PLAN); never echoed in errors. */
        const val MAX_DISPUTE_DETAIL_CHARS = 500
        const val DISPUTE_STATE_OPEN = "OPEN"
    }
}
