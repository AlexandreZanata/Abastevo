package com.anpfuel.application.usecase.charging

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.charging.ChargingObservationDraft
import com.anpfuel.domain.charging.ChargingObservationException
import com.anpfuel.domain.charging.ChargingObservationGateway
import com.anpfuel.domain.charging.ChargingPendingStore
import com.anpfuel.domain.charging.ChargingReportReason
import com.anpfuel.domain.charging.ChargingTally
import com.anpfuel.domain.charging.ChargingVote
import com.anpfuel.domain.charging.CommunityChargingObservation
import com.anpfuel.domain.charging.OwnedChargingObservation
import com.anpfuel.domain.charging.PendingChargingKind
import com.anpfuel.domain.charging.PendingChargingOperation
import java.util.UUID

data class ChargingCommunitySnapshot(
    val history: List<CommunityChargingObservation>,
    val mine: List<OwnedChargingObservation>,
    val pending: List<PendingChargingOperation>,
    val historyUnavailable: Boolean,
)

sealed interface ChargingSaveOutcome {
    data class Saved(val pending: List<PendingChargingOperation>) : ChargingSaveOutcome
    data class Conflict(val message: String, val pending: List<PendingChargingOperation>) : ChargingSaveOutcome
    data class Offline(val pending: List<PendingChargingOperation>) : ChargingSaveOutcome
    data class Refused(val message: String) : ChargingSaveOutcome
    data object SignedOut : ChargingSaveOutcome
}

/**
 * EE-22 community EV journeys over the existing free account and EE-12/13
 * backend. Writes are durable before the first attempt, keyed by a stable
 * client UUID, and retried verbatim after offline/process death. Votes and
 * reports are online-only because they are not content the owner edits;
 * failure is surfaced instead of silently claiming a vote.
 */
class ChargingCommunityUseCase(
    private val gateway: ChargingObservationGateway,
    private val pending: ChargingPendingStore,
    private val sessions: AuthSessionStore,
    private val origin: String,
) {
    suspend fun snapshot(placeId: String, limit: Int = 20): ChargingCommunitySnapshot {
        val local = session()?.let { pending.load(it.accountId, origin) } ?: emptyList()
        var history: List<CommunityChargingObservation> = emptyList()
        var mine: List<OwnedChargingObservation> = emptyList()
        var unavailable = false
        try {
            history = gateway.history(placeId, limit)
        } catch (cancelled: kotlinx.coroutines.CancellationException) {
            throw cancelled
        } catch (_: Exception) {
            unavailable = true
        }
        val current = session()
        if (current != null) {
            try {
                mine = gateway.mine(current.familyId, current.accessToken, limit)
            } catch (cancelled: kotlinx.coroutines.CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                unavailable = true
            }
        }
        return ChargingCommunitySnapshot(history, mine, local, unavailable)
    }

    suspend fun save(draft: ChargingObservationDraft, editing: OwnedChargingObservation? = null): ChargingSaveOutcome {
        val session = session() ?: return ChargingSaveOutcome.SignedOut
        val operation = if (editing == null) {
            PendingChargingOperation(
                operationId = UUID.randomUUID().toString(),
                kind = PendingChargingKind.SUBMIT,
                targetId = null,
                expectedRevision = null,
                draft = draft,
                createdAtMillis = System.currentTimeMillis(),
            )
        } else {
            PendingChargingOperation(
                operationId = UUID.randomUUID().toString(),
                kind = PendingChargingKind.CORRECT,
                targetId = editing.id,
                expectedRevision = editing.revision,
                draft = draft,
                createdAtMillis = System.currentTimeMillis(),
            )
        }
        enqueue(session.accountId, operation)
        return dispatch(session.accountId)
    }

    suspend fun withdraw(target: OwnedChargingObservation): ChargingSaveOutcome {
        val session = session() ?: return ChargingSaveOutcome.SignedOut
        enqueue(
            session.accountId,
            PendingChargingOperation(
                operationId = UUID.randomUUID().toString(),
                kind = PendingChargingKind.WITHDRAW,
                targetId = target.id,
                expectedRevision = target.revision,
                draft = null,
                createdAtMillis = System.currentTimeMillis(),
            ),
        )
        return dispatch(session.accountId)
    }

    suspend fun retryPending(): ChargingSaveOutcome {
        val session = session() ?: return ChargingSaveOutcome.SignedOut
        return dispatch(session.accountId)
    }

    suspend fun vote(observationId: String, choice: ChargingVote): ChargingTally {
        val session = session() ?: throw ChargingObservationException.SignedOut()
        return gateway.vote(session.familyId, session.accessToken, observationId, choice, UUID.randomUUID().toString())
    }

    suspend fun report(observationId: String, reason: ChargingReportReason, detail: String) {
        val session = session() ?: throw ChargingObservationException.SignedOut()
        gateway.report(session.familyId, session.accessToken, observationId, reason, detail)
    }

    fun pendingForCurrentOwner(): List<PendingChargingOperation> {
        val session = session() ?: return emptyList()
        return pending.load(session.accountId, origin)
    }

    private fun enqueue(accountId: String, operation: PendingChargingOperation) {
        val existing = pending.load(accountId, origin)
        pending.save(accountId, origin, (existing + operation).takeLast(MAX_PENDING))
    }

    private suspend fun dispatch(accountId: String): ChargingSaveOutcome {
        var operations = pending.load(accountId, origin)
        var conflict: String? = null
        var offline = false
        val remaining = mutableListOf<PendingChargingOperation>()
        for ((index, operation) in operations.withIndex()) {
            val session = session()
            if (session == null || session.accountId != accountId) return ChargingSaveOutcome.SignedOut
            try {
                when (operation.kind) {
                    PendingChargingKind.SUBMIT -> gateway.submit(
                        session.familyId, session.accessToken, operation.operationId, operation.draft!!,
                    )
                    PendingChargingKind.CORRECT -> gateway.correct(
                        session.familyId, session.accessToken, operation.targetId!!,
                        operation.expectedRevision!!, operation.draft!!,
                    )
                    PendingChargingKind.WITHDRAW -> gateway.withdraw(
                        session.familyId, session.accessToken, operation.targetId!!, operation.expectedRevision!!,
                    )
                }
            } catch (cancelled: kotlinx.coroutines.CancellationException) {
                throw cancelled
            } catch (error: ChargingObservationException.Conflict) {
                conflict = "conflict"
                remaining += operation.copy(lastError = "conflict")
            } catch (error: ChargingObservationException.Denied) {
                remaining += operation.copy(lastError = "refused")
            } catch (error: ChargingObservationException.Invalid) {
                remaining += operation.copy(lastError = "invalid")
            } catch (error: ChargingObservationException.SignedOut) {
                pending.save(accountId, origin, remaining + operations.drop(index))
                return ChargingSaveOutcome.SignedOut
            } catch (error: Exception) {
                offline = true
                remaining += operation
            }
        }
        pending.save(accountId, origin, remaining)
        operations = remaining
        return when {
            conflict != null -> ChargingSaveOutcome.Conflict("conflict", operations)
            offline -> ChargingSaveOutcome.Offline(operations)
            operations.isNotEmpty() -> ChargingSaveOutcome.Refused("pending")
            else -> ChargingSaveOutcome.Saved(operations)
        }
    }

    private fun session() = sessions.load()

    private companion object {
        const val MAX_PENDING = 50
    }
}
