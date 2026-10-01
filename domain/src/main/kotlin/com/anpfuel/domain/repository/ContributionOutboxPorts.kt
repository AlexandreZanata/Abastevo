package com.anpfuel.domain.repository

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.ContributionDraft

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

interface ContributionOutboxRepository {
    suspend fun enqueue(draft: ContributionDraft, payloadJson: String): QueuedContribution
    suspend fun listDispatchable(nowMillis: Long): List<QueuedContribution>
    suspend fun loadPayload(commandId: String): String?
    suspend fun markDispatched(commandId: String, nonce: String)
    suspend fun markAcknowledged(commandId: String, revision: Int)
    suspend fun markFailed(commandId: String, nowMillis: Long)
    suspend fun cancel(commandId: String)

    /**
     * P21-T03 private owner status: every locally retained command with
     * its durable phase. Acknowledged commands are deleted on ack, so only
     * QUEUED/IN_FLIGHT/FAILED/CANCELLED rows appear; anything else stored
     * is corruption and fails closed.
     */
    suspend fun listOwned(): List<OwnedContribution>
}

/**
 * P21-T03 persisted owner phases. No ACKED: ack deletes the row, so a
 * stored ACKED (or any unknown string) is corruption, never a status.
 */
enum class OwnedContributionPhase {
    QUEUED,
    IN_FLIGHT,
    FAILED,
    CANCELLED,
}

data class OwnedContribution(
    val commandId: String,
    val revision: Int,
    val attempts: Int,
    val phase: OwnedContributionPhase,
) {
    companion object {
        fun phaseOf(stored: String): OwnedContributionPhase = when (stored) {
            OwnedContributionPhase.QUEUED.name -> OwnedContributionPhase.QUEUED
            OwnedContributionPhase.IN_FLIGHT.name -> OwnedContributionPhase.IN_FLIGHT
            OwnedContributionPhase.FAILED.name -> OwnedContributionPhase.FAILED
            OwnedContributionPhase.CANCELLED.name -> OwnedContributionPhase.CANCELLED
            else -> throw DomainException("corrupt outbox state: $stored")
        }
    }
}

/** Local submission states mirrored from the backend lifecycle. */
enum class ContributionRemoteStatus {
    QUEUED,
    RECEIVED,
    VALIDATED,
}

data class ContributionReceipt(
    val commandId: String,
    val revision: Int,
    val status: ContributionRemoteStatus,
)

interface ContributionSubmissionGateway {
    suspend fun submit(
        commandId: String,
        revision: Int,
        payloadJson: String,
        nonce: String,
    ): ContributionReceipt
}
