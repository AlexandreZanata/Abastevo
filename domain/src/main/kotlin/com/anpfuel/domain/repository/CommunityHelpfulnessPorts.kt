package com.anpfuel.domain.repository

/**
 * CS08A — Helpfulness vote wire contract (B-BR-CS01).
 *
 * A useful vote is not a truth signal: `HELPFUL`/`NOT_HELPFUL` never
 * validate a price and never reuse confirmation/dispute or feedback
 * VALID/INVALID meanings. Tallies carry exact counts; percentages use
 * the shared floor basis-point convention and stay absent on zero.
 */
object CommunityHelpfulnessChoices {
    const val HELPFUL = "HELPFUL"
    const val NOT_HELPFUL = "NOT_HELPFUL"

    val ALL: Set<String> = setOf(HELPFUL, NOT_HELPFUL)
}

enum class CommunityHelpfulnessRejectKind {
    SELF_VOTE,
    STALE_REVISION,
    INELIGIBLE_TARGET,
    QUOTA_EXCEEDED,
    INVALID_VOTE,
    CONFLICT,
    TRANSPORT,
}

data class CommunityHelpfulnessReceipt(
    val voteId: String,
    val observationId: String,
    val helpful: Long,
    val notHelpful: Long,
    val total: Long,
    val helpfulBasisPoints: Long?,
    val myChoice: String?,
    val replayed: Boolean,
)

data class CommunityHelpfulnessTally(
    val observationId: String,
    val decisionSequence: Long,
    val helpful: Long,
    val notHelpful: Long,
    val total: Long,
    val helpfulBasisPoints: Long?,
    val myChoice: String?,
)

class CommunityHelpfulnessException(
    val kind: CommunityHelpfulnessRejectKind,
    message: String?,
) : Exception(message)

interface CommunityHelpfulnessGateway {
    suspend fun castHelpfulness(
        observationId: String,
        clientSubmissionId: String,
        choiceWire: String,
        decisionSequence: Long,
    ): CommunityHelpfulnessReceipt

    suspend fun removeHelpfulness(
        observationId: String,
        decisionSequence: Long,
    ): CommunityHelpfulnessTally

    suspend fun tallyFor(
        observationId: String,
        decisionSequence: Long,
    ): CommunityHelpfulnessTally
}
