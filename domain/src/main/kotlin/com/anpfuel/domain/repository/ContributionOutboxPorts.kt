package com.anpfuel.domain.repository

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.ContributionDraft
import com.anpfuel.domain.model.ContributionScope

/**
 * P10-T05 durable contribution ports (BUC-003/004, B-BR-003/005/010/011).
 *
 * [ContributionOutboxRepository] is Room-only durable storage: one stable
 * client submission id holds one pending intent; re-enqueue bumps the
 * revision (attempts kept) so process death/duplicate send never creates
 * a second observation. [ContributionSubmissionGateway] is network-only
 * (throws on transport/finalize failure, never falls back); the worker
 * owns retry with a fresh nonce per send (nonce != operation id).
 * States queued/received/validated stay explicit end to end.
 */
data class QueuedContribution(
    val commandId: String,
    val revision: Int,
    val payloadJson: String,
    val historical: Boolean,
)

data class PendingContribution(val draft: ContributionDraft, val payloadJson: String)

interface ContributionOutboxRepository {
    /** All rows commit together. Unsupported adapters refuse instead of partially enqueueing. */
    suspend fun enqueueReview(commands: List<PendingContribution>): List<QueuedContribution> =
        throw DomainException("atomic review storage unavailable")

    suspend fun claimDispatch(commandId: String, revision: Int, nonce: String, nowMillis: Long): Boolean =
        throw DomainException("atomic dispatch unavailable")
    suspend fun recordReceipt(receipt: ContributionReceipt, nowMillis: Long): Unit =
        throw DomainException("durable receipt storage unavailable")
    suspend fun receipt(commandId: String): ContributionReceipt? = null
    suspend fun failDispatch(commandId: String, revision: Int, nonce: String, nowMillis: Long): Unit =
        throw DomainException("atomic failure storage unavailable")
    suspend fun enqueue(draft: ContributionDraft, payloadJson: String): QueuedContribution
    suspend fun listDispatchable(nowMillis: Long): List<QueuedContribution>
    suspend fun loadPayload(commandId: String): String?
    suspend fun markDispatched(commandId: String, nonce: String)
    suspend fun markAcknowledged(commandId: String, revision: Int)
    suspend fun markFailed(commandId: String, nowMillis: Long)
    suspend fun cancel(commandId: String)

    /**
     * P21-T03 private owner status: every locally retained command with
     * its durable phase and exact server receipt. ACKED requires a persisted
     * receipt; unscoped legacy commands remain quarantined from dispatch.
     */
    suspend fun listOwned(): List<OwnedContribution>
}

/**
 * Persisted owner phases; acknowledgement keeps receipt metadata, never photo bytes.
 */
enum class OwnedContributionPhase {
    QUEUED,
    IN_FLIGHT,
    FAILED,
    CANCELLED,
    ACKNOWLEDGED,
}

data class OwnedContribution(
    val commandId: String,
    val revision: Int,
    val attempts: Int,
    val phase: OwnedContributionPhase,
    val scope: ContributionScope? = null,
    val remoteStatus: ContributionRemoteStatus? = null,
    val reason: String? = null,
) {
    companion object {
        fun phaseOf(stored: String): OwnedContributionPhase = when (stored) {
            OwnedContributionPhase.QUEUED.name -> OwnedContributionPhase.QUEUED
            OwnedContributionPhase.IN_FLIGHT.name -> OwnedContributionPhase.IN_FLIGHT
            OwnedContributionPhase.FAILED.name -> OwnedContributionPhase.FAILED
            OwnedContributionPhase.CANCELLED.name -> OwnedContributionPhase.CANCELLED
            "ACKED" -> OwnedContributionPhase.ACKNOWLEDGED
            else -> throw DomainException("corrupt outbox state: $stored")
        }
    }
}

/** Local submission states mirrored from the backend lifecycle. */
enum class ContributionRemoteStatus {
    QUEUED,
    RECEIVED,
    VALIDATING,
    VALIDATED,
    REJECTED,
    EXPIRED,
}

data class ContributionReceipt(
    val commandId: String,
    val revision: Int,
    val status: ContributionRemoteStatus,
    val observationId: String? = null,
    val reason: String? = null,
)

interface ContributionSubmissionGateway {
    suspend fun refresh(receipt: ContributionReceipt, payloadJson: String): ContributionReceipt =
        throw DomainException("owner status refresh unavailable")

    suspend fun submit(
        commandId: String,
        revision: Int,
        payloadJson: String,
        nonce: String,
    ): ContributionReceipt
}
