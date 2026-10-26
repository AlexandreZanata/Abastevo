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
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

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
    private val onQueued: (String) -> Unit = {},
) {
    private val mutex = Mutex()

    suspend fun recoverPendingSchedule() = mutex.withLock {
        val owner = session()?.accountId ?: return@withLock
        if (pending.load(owner, origin).isNotEmpty() && session()?.accountId == owner) onQueued(owner)
    }

    suspend fun targets(placeId: String) = gateway.targets(placeId)

    suspend fun snapshot(placeId: String, limit: Int = 20): ChargingCommunitySnapshot {
        val owner = session()?.accountId
        val local = session()?.takeIf { it.accountId == owner }?.let { pending.load(it.accountId, origin) } ?: emptyList()
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
        val current = session()?.takeIf { it.accountId == owner }
        if (current != null) {
            try {
                mine = gateway.mineAt(current.familyId, current.accessToken, placeId, limit)
            } catch (cancelled: kotlinx.coroutines.CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                unavailable = true
            }
        }
        return ChargingCommunitySnapshot(history, if (session()?.accountId == owner) mine.filter { it.placeId == placeId } else emptyList(),
            if (session()?.accountId == owner) local else emptyList(), unavailable)
    }

    suspend fun save(draft: ChargingObservationDraft, editing: OwnedChargingObservation? = null): ChargingSaveOutcome = mutex.withLock {
        val session = session() ?: return@withLock ChargingSaveOutcome.SignedOut
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
        if (!enqueue(session.accountId, operation)) return@withLock ChargingSaveOutcome.Refused("queue-full")
        dispatch(session.accountId)
    }

    suspend fun withdraw(target: OwnedChargingObservation): ChargingSaveOutcome = mutex.withLock {
        val session = session() ?: return@withLock ChargingSaveOutcome.SignedOut
        val accepted = enqueue(
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
        if (!accepted) return@withLock ChargingSaveOutcome.Refused("queue-full")
        dispatch(session.accountId)
    }

    suspend fun retryPending(expectedOwner: String? = null): ChargingSaveOutcome = mutex.withLock {
        val session = session() ?: return@withLock ChargingSaveOutcome.SignedOut
        if (expectedOwner != null && session.accountId != expectedOwner) return@withLock ChargingSaveOutcome.SignedOut
        if (expectedOwner == null && pending.load(session.accountId, origin).isNotEmpty()) onQueued(session.accountId)
        dispatch(session.accountId)
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

    suspend fun discard(operationId: String) = mutex.withLock {
        val current = session() ?: return@withLock
        pending.save(current.accountId, origin, pending.load(current.accountId, origin).filterNot { it.operationId == operationId })
    }

    private fun enqueue(accountId: String, operation: PendingChargingOperation): Boolean {
        val existing = pending.load(accountId, origin)
        if (existing.size >= MAX_PENDING) return false
        pending.save(accountId, origin, existing + operation)
        onQueued(accountId)
        return true
    }

    private suspend fun dispatch(accountId: String): ChargingSaveOutcome {
        var operations = pending.load(accountId, origin)
        while (operations.isNotEmpty()) {
            val operation = operations.first()
            val session = session()?.takeIf { it.accountId == accountId } ?: return ChargingSaveOutcome.SignedOut
            try {
                when (operation.kind) {
                    PendingChargingKind.SUBMIT -> gateway.submit(session.familyId, session.accessToken, operation.operationId, operation.draft!!)
                    PendingChargingKind.CORRECT -> gateway.correctOperation(session.familyId, session.accessToken, operation.targetId!!,
                        operation.expectedRevision!!, operation.draft!!, operation.operationId)
                    PendingChargingKind.WITHDRAW -> gateway.withdrawOperation(session.familyId, session.accessToken, operation.targetId!!,
                        operation.expectedRevision!!, operation.operationId)
                }
            } catch (cancelled: kotlinx.coroutines.CancellationException) { throw cancelled
            } catch (_: ChargingObservationException.SignedOut) { return ChargingSaveOutcome.SignedOut
            } catch (error: Exception) {
                if (session()?.accountId != accountId) return ChargingSaveOutcome.SignedOut
                val reason = when (error) {
                    is ChargingObservationException.Conflict -> "conflict"
                    is ChargingObservationException.Denied -> "refused"
                    is ChargingObservationException.Invalid -> "invalid"
                    else -> "offline"
                }
                operations = listOf(operation.copy(lastError = reason)) + operations.drop(1)
                pending.save(accountId, origin, operations)
                return when (reason) {
                    "conflict" -> ChargingSaveOutcome.Conflict(reason, operations)
                    "offline" -> ChargingSaveOutcome.Offline(operations)
                    else -> ChargingSaveOutcome.Refused(reason)
                }
            }
            // Lost replies and owner changes retain the stable command for exact replay.
            if (session()?.accountId != accountId) return ChargingSaveOutcome.SignedOut
            operations = operations.drop(1)
            pending.save(accountId, origin, operations)
        }
        return ChargingSaveOutcome.Saved(operations)
    }

    private fun session() = sessions.load()

    private companion object {
        const val MAX_PENDING = 50
    }
}
