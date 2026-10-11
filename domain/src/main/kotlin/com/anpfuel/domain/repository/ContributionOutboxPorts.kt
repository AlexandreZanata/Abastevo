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
    /**
     * SEC06-C bounded dispatch selection: one requested command by id
     * (no scan), or null when absent/ineligible. Payloads of other
     * rows are never loaded to select one.
     */
    suspend fun findDispatchable(commandId: String, nowMillis: Long): QueuedContribution?
    /**
     * SEC06-C bounded dispatch selection: at most [limit] eligible
     * commands, oldest first, payloads included only for the page.
     * Backed by the dispatch index; never a full-history scan.
     */
    suspend fun listDispatchablePage(nowMillis: Long, limit: Int): List<QueuedContribution>
    suspend fun loadPayload(commandId: String): String?
    suspend fun markDispatched(commandId: String, nonce: String)
    suspend fun markAcknowledged(commandId: String, revision: Int)
    suspend fun markFailed(commandId: String, nowMillis: Long)
    suspend fun cancel(commandId: String)

    /**
     * P21-T03 private owner status, newest first, bounded page.
     * ACKED requires a persisted receipt; unscoped legacy commands
     * remain quarantined from dispatch.
     */
    suspend fun listOwnedPage(limit: Int, offset: Int): List<OwnedContribution>

    /** Single owned row by id, or null when absent. */
    suspend fun findOwned(commandId: String): OwnedContribution?

    /**
     * SEC06-C retention: delete terminal history (CANCELLED rows and
     * ACKED rows with a terminal receipt) beyond the retained newest
     * window, at most one purge batch, returning the purged count.
     * Live and retryable rows are never touched; terminal payload
     * bytes free with their rows.
     */
    suspend fun purgeTerminalHistory(): Int

    /**
     * SEC04-D owner erasure: delete every row whose payload scope matches
     * [ownerScope] and cancel its scheduled work, returning the purged
     * count. Rows without a parseable scope are kept (fail-closed: never
     * delete unattributable rows). After this returns, no pending
     * operation of the erased owner remains to send.
     */
    suspend fun purgeOwned(ownerScope: String): Int
}

/**
 * SEC06-C outbox budgets: dispatch reads one bounded page, history
 * reads one bounded page, and terminal history older than the
 * retention horizon purges in bounded batches. Live and retryable
 * rows are never purged; payloads stay small by construction
 * (price intents), and terminal receipts free their bytes on purge.
 */
object OutboxBudgets {
    /** Dispatch page: one worker run scans at most this many rows. */
    const val DISPATCH_PAGE_SIZE = 50
    /** History page: status screens read newest-first pages. */
    const val HISTORY_PAGE_SIZE = 200
    /** Terminal history retained (newest rows); older purge away. */
    const val MAX_RETAINED_TERMINAL = 1000
    /** Terminal purge batch per amortized run. */
    const val HISTORY_PURGE_BATCH = 500
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
