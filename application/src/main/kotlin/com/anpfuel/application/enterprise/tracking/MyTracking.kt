package com.anpfuel.application.enterprise.tracking

import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.enterprise.EnterpriseUnlockRequired
import com.anpfuel.application.enterprise.EnterpriseVaultUnrecoverable
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.tracking.LocationPermission
import com.anpfuel.domain.enterprise.tracking.TrackingGapReason
import com.anpfuel.domain.enterprise.tracking.TrackingPolicyRead
import com.anpfuel.domain.enterprise.trip.TripAssetLabel
import java.time.Instant
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/*
 * EMP-GPS-015 — tracking transparency (B-BR-EMP-548..551, EDR-357..359): what this phone's organization collector is doing
 * now, for the indicator of every enterprise screen, and the member's own sessions on this phone for "Meu rastreamento".
 * Counts, states, causes and instants only; never a coordinate.
 */

/** What the running organization collector does now (B-BR-EMP-548); null in [TrackingRuntime.activity] when nothing listens. */
class TrackingActivity(val partition: EnterprisePartition, val sessionId: String, val tripId: String, val organizationName: String?,
    val since: Instant, val precision: LocationPermission, val gap: TrackingGapReason?) {
    override fun equals(other: Any?): Boolean = other is TrackingActivity && partition == other.partition && sessionId == other.sessionId &&
        tripId == other.tripId && organizationName == other.organizationName && since == other.since && precision == other.precision && gap == other.gap
    override fun hashCode(): Int = 31 * sessionId.hashCode() + since.hashCode()
    override fun toString(): String = "EnterpriseTrackingActivity(redacted)"
}

/** One of the member's own sessions on this phone (B-BR-EMP-549): its view, its vehicle and its tracking bounds. */
class TrackingHistoryEntry(val session: TrackingSessionView, val vehicle: TripAssetLabel?, val startedAt: Instant?, val endedAt: Instant?) {
    override fun toString(): String = "EnterpriseTrackingHistoryEntry(redacted)"
}

/** The member's own sessions on this phone, newest first, with the policy last read. */
class TrackingHistory(val policy: TrackingPolicyRead?, entries: List<TrackingHistoryEntry>) {
    private val values = entries.toList()
    val entries: List<TrackingHistoryEntry> get() = values.toList()
    override fun toString(): String = "EnterpriseTrackingHistory(redacted)"
}

/** SCR-GPS-04 state: read only, from the selected partition's sealed records. */
data class MyTrackingViewState(
    val generation: Long = 0,
    val epoch: Long? = null,
    val context: EnterpriseContextSummary? = null,
    val busy: Boolean = false,
    /** The sealed records were read; an empty history then means no tracked trip on this phone. */
    val loaded: Boolean = false,
    val history: TrackingHistory? = null,
    val error: EnterpriseAccessError? = null,
    val unlockRequired: Boolean = false,
    val unrecoverable: Boolean = false,
) {
    override fun toString(): String = "MyTrackingViewState(redacted)"
}

/** One visible "Meu rastreamento" owner per screen, bound to account, epoch and background lease like the other panels. */
class MyTrackingFlow(
    private val source: EnterpriseQueueSource,
    private val service: EnterpriseTrackingService,
    private val fence: EnterpriseSessionFence,
) {
    private val mutable = MutableStateFlow(MyTrackingViewState())
    val state: StateFlow<MyTrackingViewState> = mutable.asStateFlow()
    private val monitor = Any()
    private var serial = 0L
    private var closed = false
    private class Owner(val token: Long, val context: EnterpriseContextSummary, val epoch: Long, val background: Long,
        var account: EnterpriseQueueAccountLease? = null)
    private var owner: Owner? = null

    private fun next(): Long = Math.addExact(serial, 1).also { serial = it }
    private fun viewCurrent(request: Owner): Boolean = synchronized(monitor) {
        !closed && owner === request && mutable.value.generation == request.token &&
            fence.epoch == request.epoch && fence.backgroundEpoch == request.background
    }
    private fun retained(request: Owner): Boolean {
        if (!viewCurrent(request)) return false
        if (runCatching { request.account?.assertCurrent() }.isFailure) return false
        return viewCurrent(request)
    }
    private fun clearLocked(error: EnterpriseAccessError? = null) {
        owner = null
        mutable.value = MyTrackingViewState(generation = next(), error = error)
    }
    fun withdraw() = synchronized(monitor) { clearLocked() }
    fun close() = synchronized(monitor) { if (!closed) { clearLocked(); closed = true } }
    fun invalidateIfStale() {
        val captured = synchronized(monitor) { owner } ?: return
        if (!retained(captured)) synchronized(monitor) { if (owner === captured) clearLocked(EnterpriseAccessError.STALE) }
    }

    private suspend fun account(request: Owner): EnterpriseQueueAccountLease {
        val account = source.account()
        if (!account.identity.owns(request.context.partition) || account.epoch != request.epoch || account.background != request.background)
            throw EnterpriseLocalDenied()
        synchronized(monitor) { if (owner === request) request.account = account }
        return account
    }

    private suspend fun publish(request: Owner) {
        val history = service.history(account(request), request.context.partition)
        if (!retained(request)) { invalidateIfStale(); return }
        synchronized(monitor) {
            if (owner !== request || !viewCurrent(request)) return
            mutable.value = mutable.value.copy(busy = false, loaded = true, history = history, error = null, unlockRequired = false,
                unrecoverable = false)
        }
    }

    private suspend fun perform(request: Owner) {
        try { currentCoroutineContext().ensureActive(); publish(request) }
        catch (cancelled: CancellationException) { synchronized(monitor) { if (owner === request) clearLocked() }; throw cancelled }
        catch (failure: Exception) {
            val sourceCurrent = retained(request)
            synchronized(monitor) {
                if (owner !== request || closed) return
                mutable.value = when {
                    failure is EnterpriseLocalDenied || !sourceCurrent || !viewCurrent(request) -> { clearLocked(EnterpriseAccessError.STALE); return }
                    else -> mutable.value.copy(busy = false, loaded = false, history = null, error = EnterpriseAccessError.UNAVAILABLE,
                        unlockRequired = failure is EnterpriseUnlockRequired, unrecoverable = failure is EnterpriseVaultUnrecoverable)
                }
            }
        }
    }

    /** Reads the sealed tracking records of the exact partition; no network. */
    suspend fun open(context: EnterpriseContextSummary, renderedEpoch: Long) {
        invalidateIfStale()
        val request = synchronized(monitor) {
            if (closed || fence.epoch != renderedEpoch) return
            val created = Owner(next(), context, renderedEpoch, fence.backgroundEpoch)
            owner = created
            mutable.value = MyTrackingViewState(generation = created.token, epoch = created.epoch, context = context, busy = true)
            created
        }
        perform(request)
    }

    suspend fun reload(renderedEpoch: Long, generation: Long) {
        invalidateIfStale()
        val request = synchronized(monitor) {
            val before = owner ?: return
            if (closed || mutable.value.busy || before.epoch != renderedEpoch || before.token != generation ||
                fence.epoch != renderedEpoch || fence.backgroundEpoch != before.background) return
            val created = Owner(next(), before.context, before.epoch, before.background, before.account)
            owner = created
            mutable.value = mutable.value.copy(generation = created.token, busy = true)
            created
        }
        perform(request)
    }

    override fun toString(): String = "MyTrackingFlow(redacted)"
}
