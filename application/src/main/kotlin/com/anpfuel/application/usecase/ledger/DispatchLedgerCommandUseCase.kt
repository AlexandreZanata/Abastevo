package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.repository.LedgerSyncConflict
import com.anpfuel.domain.repository.LedgerSyncDenied
import com.anpfuel.domain.repository.LedgerSyncGateway
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
) {

    companion object {
        /** Claim lease: a crashed worker's claim becomes releasable after this. */
        const val CLAIM_LEASE_MILLIS = 5L * 60L * 1000L
    }

    suspend fun invoke(requestedId: String?): LedgerDispatchOutcome {
        if (!enabled()) return LedgerDispatchOutcome.DONE
        val session = sessions.load() ?: return LedgerDispatchOutcome.REFUSED
        val scope = PremiumOwnerScope(session.accountId, origin)
        val next = if (requestedId.isNullOrBlank()) {
            outbox.listPending(scope, 1).firstOrNull()
        } else {
            outbox.findPending(scope, requestedId)?.takeIf { it.operationId.isNotEmpty() }
        } ?: return LedgerDispatchOutcome.DONE
        val attempt = attemptId()
        if (!outbox.claim(next.operationId, scope, attempt, nowMillis() + CLAIM_LEASE_MILLIS)) {
            return LedgerDispatchOutcome.RETRY
        }
        return try {
            val receipt = gateway.submit(scope, next, attempt, session.familyId, session.accessToken)
            if (receipt.operationId != next.operationId) {
                throw LedgerSyncDenied("ledger.invalid-receipt")
            }
            outbox.acknowledge(scope, next.operationId)
            LedgerDispatchOutcome.DONE
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (conflict: LedgerSyncConflict) {
            outbox.markConflict(scope, next.operationId, conflict.message ?: "ledger.conflict")
            LedgerDispatchOutcome.REFUSED
        } catch (denied: LedgerSyncDenied) {
            outbox.failTransient(scope, next.operationId, attempt, nowMillis())
            LedgerDispatchOutcome.REFUSED
        } catch (_: Exception) {
            outbox.failTransient(scope, next.operationId, attempt, nowMillis())
            LedgerDispatchOutcome.RETRY
        }
    }
}
