package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.repository.LedgerSyncConflict
import com.anpfuel.domain.repository.LedgerSyncDenied
import com.anpfuel.domain.repository.LedgerSyncGateway
import com.anpfuel.domain.repository.LedgerSyncAckStore
import com.anpfuel.domain.repository.LedgerSyncScope
import com.anpfuel.domain.repository.PremiumCommandStore
import java.util.UUID
import kotlinx.coroutines.CancellationException

enum class LedgerDispatchOutcome { DONE, RETRY, REFUSED }

/**
 * MEX-14 ledger dispatch: one durable queued command per call with an
 * atomic claim, live owner proof and ack-by-receipt.
 *
 * Lapsed sessions and disabled sync never touch the wire (pending work
 * stays queued). Accepted and replayed commands converge to the same
 * acknowledgement; lost replies retry with the identical operation ID
 * (the backend replays instead of duplicating). Stale revisions park as
 * conflicts without deleting held work; denied reads keep their retry
 * budget. Duplicate workers lose the atomic claim race.
 */
class DispatchLedgerCommandUseCase(
    private val outbox: PremiumCommandStore,
    private val gateway: LedgerSyncGateway,
    private val sessions: AuthSessionStore,
    private val origin: String,
    private val nowMillis: () -> Long = System::currentTimeMillis,
    private val attemptId: () -> String = { UUID.randomUUID().toString() },
    private val enabled: () -> Boolean = { true },
    private val syncScope: LedgerSyncScope? = null,
    private val acks: LedgerSyncAckStore? = null,
    private val erasures: com.anpfuel.domain.repository.LedgerErasureLog? = null,
    private val drafts: com.anpfuel.domain.repository.LedgerDraftStore? = null,
) {

    companion object {
        /** Claim lease: a crashed worker's claim becomes releasable after this. */
        const val CLAIM_LEASE_MILLIS = 5L * 60L * 1000L
    }

    suspend fun invoke(requestedId: String?): LedgerDispatchOutcome {
        if (!enabled()) return LedgerDispatchOutcome.DONE
        if (syncScope != null && !syncScope.isDeviceEnabled()) return LedgerDispatchOutcome.DONE
        val session = sessions.load() ?: return LedgerDispatchOutcome.REFUSED
        val ownerScope = PremiumOwnerScope(session.accountId, origin)
        val next = if (requestedId.isNullOrBlank()) {
            pickHead(ownerScope)
        } else {
            outbox.findPending(ownerScope, requestedId)?.takeIf { it.operationId.isNotEmpty() }
        } ?: return LedgerDispatchOutcome.DONE
        if (syncScope?.isDatasetEnabled(next.dataset) == false) return LedgerDispatchOutcome.DONE
        if (sessions.load()?.accountId != session.accountId) return LedgerDispatchOutcome.REFUSED
        val attempt = attemptId()
        if (!outbox.claim(next.operationId, ownerScope, attempt, nowMillis() + CLAIM_LEASE_MILLIS)) {
            return LedgerDispatchOutcome.RETRY
        }
        // A Room claim can suspend while account or participation changes.
        // Leave its bounded lease intact; no intent is removed or sent after revocation.
        if (!enabled() || syncScope?.isDeviceEnabled() == false || syncScope?.isDatasetEnabled(next.dataset) == false) {
            return LedgerDispatchOutcome.DONE
        }
        if (sessions.load()?.accountId != session.accountId) return LedgerDispatchOutcome.REFUSED
        return try {
            if (!next.delete && drafts != null) {
                val current = drafts.find(ownerScope.accountId, origin, com.anpfuel.domain.premium.LedgerDataset.parse(next.dataset), next.recordId)
                if (current == null || current.draft.bodyJson() != next.payloadJson) {
                    outbox.markConflict(ownerScope, next.operationId, "ledger.local-record-changed")
                    return LedgerDispatchOutcome.REFUSED
                }
            }
            val receipt = gateway.submit(ownerScope, next, attempt, session.familyId, session.accessToken)
            if (receipt.operationId != next.operationId) {
                throw LedgerSyncDenied("ledger.invalid-receipt")
            }
            if (next.delete) {
                erasures?.recordErased(session.accountId, origin, next.dataset, next.recordId, nowMillis())
            }
            outbox.acknowledge(ownerScope, next.operationId)
            if (!next.delete) {
                acks?.recordAck(session.accountId, origin, next.dataset, receipt.revision, nowMillis())
            }
            LedgerDispatchOutcome.DONE
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (conflict: LedgerSyncConflict) {
            outbox.markConflict(ownerScope, next.operationId, conflict.message ?: "ledger.conflict")
            LedgerDispatchOutcome.REFUSED
        } catch (denied: LedgerSyncDenied) {
            outbox.failTransient(ownerScope, next.operationId, attempt, nowMillis())
            LedgerDispatchOutcome.REFUSED
        } catch (_: Exception) {
            outbox.failTransient(ownerScope, next.operationId, attempt, nowMillis())
            LedgerDispatchOutcome.RETRY
        }
    }

    private suspend fun pickHead(scope: PremiumOwnerScope): PremiumCommand? {
        val head = outbox.listPending(scope, 1).firstOrNull() ?: return null
        val gate = syncScope ?: return head
        if (gate.isDatasetEnabled(head.dataset)) return head
        return outbox.listPending(scope, 50).firstOrNull { gate.isDatasetEnabled(it.dataset) }
    }
}
