package com.anpfuel.domain.repository

import com.anpfuel.domain.exception.DomainException

/**
 * P10-T07 community corroboration ports (BUC-005, B-BR-006/007/011/012).
 *
 * The gateway is network-only (throws on transport failure or a typed
 * [CommunityVoteException], never returns a local fallback). The
 * application use case owns validation, stable idempotency keys and
 * status mapping so one contributor never amplifies a vote by retrying:
 * the same `client_submission_id` with the same body converges to the
 * same vote server-side (B-BR-005/006); a changed price is a new
 * observation via the contribution outbox (B-BR-003), never an edit.
 * Dispute detail stays private (B-BR-011): it travels only in the
 * dispute body and never appears in error messages or receipts.
 */
enum class CommunityVoteRejectKind {
    SELF_CONFIRMATION,
    ALREADY_RECORDED,
    INELIGIBLE_TARGET,
    CONFLICT,
    TRANSPORT,
}

data class CommunityVoteReceipt(
    val voteId: String,
    val observationId: String,
    val replayed: Boolean,
)

/** Typed vote failure; [message] never carries private dispute detail. */
class CommunityVoteException(
    val kind: CommunityVoteRejectKind,
    message: String,
) : DomainException(message)

/** Wire dispute reasons (backend `DisputeReason` enum, BUC-005). */
object CommunityDisputeReasons {
    const val PRICE_CHANGED = "PRICE_CHANGED"
    const val WRONG_STATION = "WRONG_STATION"
    const val WRONG_PRODUCT = "WRONG_PRODUCT"
    const val WRONG_CONDITION = "WRONG_CONDITION"
    const val EVIDENCE_MISMATCH = "EVIDENCE_MISMATCH"
    const val OTHER = "OTHER"

    val ALL: Set<String> = setOf(
        PRICE_CHANGED,
        WRONG_STATION,
        WRONG_PRODUCT,
        WRONG_CONDITION,
        EVIDENCE_MISMATCH,
        OTHER,
    )
}

interface CommunityVoteGateway {
    suspend fun submitConfirmation(
        observationId: String,
        clientSubmissionId: String,
    ): CommunityVoteReceipt

    suspend fun submitDispute(
        targetObservationId: String,
        clientSubmissionId: String,
        reasonWire: String,
        detail: String?,
        replacementObservationId: String?,
    ): CommunityVoteReceipt
}
