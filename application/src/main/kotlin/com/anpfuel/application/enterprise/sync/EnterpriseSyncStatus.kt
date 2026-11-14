package com.anpfuel.application.enterprise.sync

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.domain.enterprise.sync.*
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.merge
import kotlinx.coroutines.flow.update

/** Memory-only capability. Only the protected adapter defines an implementation. */
interface EnterpriseSyncCursor
class EnterpriseSyncPageChanged : IllegalStateException("Enterprise sync snapshot changed")

class EnterpriseSyncPage(items: List<EnterpriseSyncItem>, val totals: EnterpriseSyncTotals, val generation: Long,
    val next: EnterpriseSyncCursor?, val mode: EnterpriseSyncMode) {
    private val values = items.toList()
    init { require(values.size <= 50 && generation > 0 && values.map { it.identity.operationId }.distinct().size == values.size) }
    val items: List<EnterpriseSyncItem> get() = values.toList()
    override fun toString(): String = "EnterpriseSyncPage(redacted)"
}

interface EnterpriseSyncStatusStore {
    suspend fun page(attempt: EnterpriseQueueAttempt, mode: EnterpriseSyncMode, cursor: EnterpriseSyncCursor? = null,
        limit: Int = 50): EnterpriseSyncPage
    suspend fun detail(attempt: EnterpriseQueueAttempt, operationId: String, digest: String): EnterpriseSyncDetail
    suspend fun dismissRejected(attempt: EnterpriseQueueAttempt, operationId: String, digest: String,
        expectedReceiptDigest: String): Boolean
}

/** Actual selected attempts only. Saturation omits a hint and never blocks delivery. */
class EnterpriseDeliveryActivity {
    class Ticket internal constructor()
    private class Active(val attempt: EnterpriseQueueAttempt, val identities: Set<EnterpriseOperationIdentity>)
    private data class Scope(val enrollment: EnterpriseEnrollmentBinding, val identity: EnterpriseLocalIdentity,
        val epoch: Long, val background: Long) {
        constructor(attempt: EnterpriseQueueAttempt) : this(attempt.enrollment, attempt.account.identity,
            attempt.account.epoch, attempt.account.background)
        override fun toString(): String = "EnterpriseDeliveryScope(redacted)"
    }
    private class Completion(val revision: Long, val scope: Scope)
    private class Snapshot(val active: Map<Ticket, Active> = emptyMap(), val revision: Long = 0,
        val completions: List<Completion> = emptyList())
    private val snapshots = MutableStateFlow(Snapshot())
    private val mutableChanges = MutableStateFlow(0L)
    val changes: StateFlow<Long> = mutableChanges.asStateFlow()
    fun begin(attempt: EnterpriseQueueAttempt, identities: List<EnterpriseOperationIdentity>): Ticket? {
        attempt.assertCurrent()
        if (identities.isEmpty() || identities.size > 50 || identities.map { it.operationId }.distinct().size != identities.size) return null
        val ticket = Ticket()
        val entry = Active(attempt, identities.toSet())
        while (true) {
            val before = snapshots.value
            if (before.active.size >= 64) return null
            if (snapshots.compareAndSet(before, Snapshot(before.active + (ticket to entry), before.revision, before.completions))) {
                mutableChanges.update { if (it == Long.MAX_VALUE) it else it + 1 }
                return ticket
            }
        }
    }
    fun end(ticket: Ticket?) {
        if (ticket == null) return
        while (true) {
            val before = snapshots.value
            val retiring = before.active[ticket] ?: return
            val revision = Math.addExact(before.revision, 1)
            val completions = (before.completions + Completion(revision, Scope(retiring.attempt))).takeLast(64)
            if (snapshots.compareAndSet(before, Snapshot(before.active - ticket, revision, completions))) {
                mutableChanges.update { if (it == Long.MAX_VALUE) it else it + 1 }
                return
            }
        }
    }
    fun current(attempt: EnterpriseQueueAttempt): Set<EnterpriseOperationIdentity> {
        attempt.assertCurrent()
        val matching = snapshots.value.active.values.filter {
            it.attempt.enrollment == attempt.enrollment && it.attempt.account.identity == attempt.account.identity &&
                it.attempt.account.epoch == attempt.account.epoch && it.attempt.account.background == attempt.account.background &&
                runCatching { it.attempt.assertCurrent() }.isSuccess
        }.flatMap { it.identities }.toSet()
        attempt.assertCurrent()
        return matching
    }
    fun observe(attempt: EnterpriseQueueAttempt): Flow<Set<EnterpriseOperationIdentity>> =
        snapshots.map { current(attempt) }.distinctUntilChanged()

    /** A hint anchored before a read, never a receipt or proof of success. */
    internal fun completionRevision(attempt: EnterpriseQueueAttempt): Long {
        attempt.assertCurrent()
        val revision = snapshots.value.revision
        attempt.assertCurrent()
        return revision
    }
    internal fun completedSince(attempt: EnterpriseQueueAttempt, revision: Long): Boolean {
        attempt.assertCurrent()
        val snapshot = snapshots.value
        require(revision in 0..snapshot.revision)
        val oldest = snapshot.completions.firstOrNull()?.revision
        // A bounded history gap requests a conservative reread of this actual
        // current scope; it does not infer any status from another scope.
        val changed = revision < snapshot.revision && (oldest == null || revision < oldest - 1 ||
            snapshot.completions.any { it.revision > revision && it.scope == Scope(attempt) })
        attempt.assertCurrent()
        return changed
    }
}

data class EnterpriseSyncViewState(
    val generation: Long = 0,
    val epoch: Long? = null,
    val context: EnterpriseContextSummary? = null,
    val mode: EnterpriseSyncMode = EnterpriseSyncMode.PENDING,
    val busy: Boolean = false,
    val items: List<EnterpriseSyncItem> = emptyList(),
    val totals: EnterpriseSyncTotals? = null,
    val detail: EnterpriseSyncDetail? = null,
    val hasMore: Boolean = false,
    val error: EnterpriseAccessError? = null,
    val unlockRequired: Boolean = false,
    val unrecoverable: Boolean = false,
    val active: Set<EnterpriseOperationIdentity> = emptySet(),
    val manualWaitingMillis: Long? = null,
    val manualAttention: Int = 0,
) {
    override fun toString(): String = "EnterpriseSyncViewState(redacted)"
}

/** One visible owner. No route/SavedState selector and no LocalContext selection ownership. */
class EnterpriseSyncFlow(private val source: EnterpriseQueueSource, private val store: EnterpriseSyncStatusStore,
    private val delivery: EnterpriseQueueService, private val activity: EnterpriseDeliveryActivity,
    private val fence: EnterpriseSessionFence) {
    private val mutable = MutableStateFlow(EnterpriseSyncViewState())
    val state: StateFlow<EnterpriseSyncViewState> = mutable.asStateFlow()
    private val deferredRefresh = MutableStateFlow(0L)
    val activityChanges: Flow<Long> = merge(activity.changes, deferredRefresh)
    private val monitor = Any()
    private var serial = 0L
    private var closed = false
    private class Owner(val token: Long, val context: EnterpriseContextSummary, val epoch: Long, val background: Long,
        var account: EnterpriseQueueAccountLease? = null, var attempt: EnterpriseQueueAttempt? = null,
        var expectedIdentity: EnterpriseLocalIdentity? = account?.identity, var completionReadThrough: Long? = null)
    private var owner: Owner? = null
    private var cursor: EnterpriseSyncCursor? = null
    private var storedGeneration: Long? = null

    private fun next(): Long = Math.addExact(serial, 1).also { serial = it }
    private fun viewCurrent(request: Owner): Boolean = synchronized(monitor) {
        !closed && owner === request && mutable.value.generation == request.token &&
            fence.epoch == request.epoch && fence.backgroundEpoch == request.background
    }
    private fun retained(request: Owner): Boolean {
        if (!viewCurrent(request)) return false
        if (runCatching { request.account?.assertCurrent(); request.attempt?.assertCurrent() }.isFailure) return false
        return viewCurrent(request)
    }
    private fun continueVisible(request: Owner): Boolean = viewCurrent(request).also { if (!it) invalidateIfStale() }
    private fun continueRetained(request: Owner): Boolean = retained(request).also { if (!it) invalidateIfStale() }
    private fun clearLocked(error: EnterpriseAccessError? = null) {
        owner = null; cursor = null; storedGeneration = null
        mutable.value = EnterpriseSyncViewState(generation = next(), error = error)
    }
    fun withdraw() = synchronized(monitor) { clearLocked() }
    fun close() = synchronized(monitor) { if (!closed) { clearLocked(); closed = true } }
    fun invalidateIfStale() {
        val captured = synchronized(monitor) { owner } ?: return
        if (!retained(captured)) synchronized(monitor) { if (owner === captured) clearLocked(EnterpriseAccessError.STALE) }
    }

    private fun begin(renderedEpoch: Long, generation: Long, mode: EnterpriseSyncMode? = null,
        clearRows: Boolean = false): Owner? {
        invalidateIfStale()
        return synchronized(monitor) {
            val before = owner ?: return@synchronized null
            if (closed || mutable.value.busy || before.epoch != renderedEpoch || before.token != generation ||
                fence.epoch != renderedEpoch || fence.backgroundEpoch != before.background) return@synchronized null
            val request = Owner(next(), before.context, before.epoch, before.background, before.account, before.attempt,
                before.expectedIdentity, before.completionReadThrough)
            owner = request
            val state = mutable.value
            if (clearRows) { cursor = null; storedGeneration = null }
            mutable.value = state.copy(generation = request.token, mode = mode ?: state.mode, busy = true,
                items = if (clearRows) emptyList() else state.items, totals = if (clearRows) null else state.totals,
                detail = if (clearRows) null else state.detail, hasMore = if (clearRows) false else state.hasMore,
                active = if (clearRows) emptySet() else state.active,
                error = null, unlockRequired = false, unrecoverable = false, manualWaitingMillis = null, manualAttention = 0)
            request
        }
    }

    private suspend fun capture(request: Owner, deliveryProof: Boolean = false): EnterpriseQueueAttempt? {
        if (!continueVisible(request)) return null
        val expectedIdentity = request.expectedIdentity
        if (deliveryProof && expectedIdentity == null) throw EnterpriseLocalDenied()
        if (deliveryProof) synchronized(monitor) {
            if (owner === request) {
                // A same-family refresh deliberately retires the old full-token
                // lease. Keep only the intended identity until the fresh source
                // returns, and expose no facts under an unverified transition.
                request.account = null; request.attempt = null
                cursor = null; storedGeneration = null
                mutable.value = mutable.value.copy(items = emptyList(), totals = null, detail = null,
                    hasMore = false, active = emptySet())
            }
        }
        val account = if (deliveryProof) source.deliveryAccount() else source.account()
        if (!continueVisible(request)) return null
        if (!account.identity.owns(request.context.partition) || account.epoch != request.epoch ||
            account.background != request.background || (expectedIdentity != null && account.identity != expectedIdentity)) throw EnterpriseLocalDenied()
        account.assertCurrent()
        if (!continueVisible(request)) return null
        synchronized(monitor) { if (owner === request) {
            request.account = account; request.attempt = null; request.expectedIdentity = account.identity
        } }
        val attempt = source.attempt(account, request.context.partition)
        if (!continueVisible(request)) return null
        if (attempt.account !== account || attempt.partition != request.context.partition ||
            attempt.enrollment.target.membershipId != request.context.membershipId) throw EnterpriseLocalDenied()
        attempt.assertCurrent()
        synchronized(monitor) { if (owner === request) request.attempt = attempt }
        return attempt.takeIf { continueRetained(request) }
    }

    private fun failed(request: Owner, failure: Exception) {
        // Source predicates may consult the actual session/fence. Evaluate them
        // outside the view monitor, and let current-source denial outrank I/O.
        val sourceCurrent = retained(request)
        synchronized(monitor) {
            if (owner !== request || closed) return@synchronized
            if (failure is EnterpriseLocalDenied || !sourceCurrent || !viewCurrent(request)) clearLocked(EnterpriseAccessError.STALE)
            else {
                cursor = null; storedGeneration = null
                mutable.value = mutable.value.copy(busy = false, items = emptyList(), totals = null, detail = null, hasMore = false,
                    active = emptySet(), error = EnterpriseAccessError.UNAVAILABLE,
                    unlockRequired = failure is EnterpriseUnlockRequired, unrecoverable = failure is EnterpriseVaultUnrecoverable)
            }
        }
    }
    private suspend fun perform(request: Owner, block: suspend () -> Unit) {
        try { currentCoroutineContext().ensureActive(); block(); reconcileCompletion(request) }
        catch (cancelled: CancellationException) {
            synchronized(monitor) { if (owner === request) clearLocked() }; throw cancelled
        } catch (failure: Exception) { failed(request, failure) }
    }
    private suspend fun reconcileCompletion(request: Owner) {
        if (!continueRetained(request)) return
        val attempt = request.attempt ?: return
        val observed = synchronized(monitor) {
            val revision = request.completionReadThrough
            mutable.value.takeIf { owner === request && !it.busy && it.error == null && it.totals != null && revision != null }
                ?.let { it to requireNotNull(revision) }
        } ?: return
        val (before, revision) = observed
        if (!activity.completedSince(attempt, revision)) return
        synchronized(monitor) {
            if (owner !== request || !viewCurrent(request) || mutable.value.busy) return
            mutable.value = mutable.value.copy(busy = true)
        }
        // One extra read per turn. A completion racing that read remains dirty
        // and schedules another turn instead of an unbounded synchronous loop.
        load(request, attempt, before.mode, detailIdentity = before.detail?.item?.identity,
            waiting = before.manualWaitingMillis, attention = before.manualAttention)
        if (continueRetained(request) && request.completionReadThrough?.let { activity.completedSince(attempt, it) } == true)
            deferredRefresh.update { Math.addExact(it, 1) }
    }
    private fun verifiedDetail(value: EnterpriseSyncDetail, identity: EnterpriseOperationIdentity): EnterpriseSyncDetail {
        if (value.item.identity != identity) throw EnterprisePersistenceUnavailable()
        return value
    }
    private suspend fun load(request: Owner, attempt: EnterpriseQueueAttempt, mode: EnterpriseSyncMode,
        append: Boolean = false, detailIdentity: EnterpriseOperationIdentity? = null,
        waiting: Long? = null, attention: Int = 0) {
        if (!continueRetained(request)) return
        val previousCursor = synchronized(monitor) { if (owner === request && append) cursor else null }
        var adding = append
        var readRevision = activity.completionRevision(attempt)
        val page = try { store.page(attempt, mode, previousCursor, 50) }
        catch (_: EnterpriseSyncPageChanged) {
            currentCoroutineContext().ensureActive()
            if (!continueRetained(request)) return
            adding = false
            readRevision = activity.completionRevision(attempt)
            store.page(attempt, mode, null, 50)
        }
        if (!retained(request)) { invalidateIfStale(); return }
        if (page.mode != mode || (adding && storedGeneration != page.generation)) throw EnterpriseSyncPageChanged()
        val detail = detailIdentity?.let { verifiedDetail(store.detail(attempt, it.operationId, it.payloadDigest), it) }
        currentCoroutineContext().ensureActive()
        if (!retained(request)) { invalidateIfStale(); return }
        val active = activity.current(attempt)
        if (!continueRetained(request)) return
        synchronized(monitor) {
            if (owner !== request || !viewCurrent(request)) return@synchronized
            val items = if (adding) mutable.value.items + page.items else page.items
            if (items.size > 10000 || items.map { it.identity.operationId }.distinct().size != items.size ||
                (adding && page.items.isEmpty() && page.next != null)) throw EnterprisePersistenceUnavailable()
            cursor = page.next; storedGeneration = page.generation; request.completionReadThrough = readRevision
            mutable.value = mutable.value.copy(busy = false, items = items.toList(), totals = page.totals, detail = detail,
                hasMore = page.next != null, active = active, error = null, unlockRequired = false, unrecoverable = false,
                manualWaitingMillis = waiting, manualAttention = attention)
        }
    }

    suspend fun open(context: EnterpriseContextSummary, renderedEpoch: Long, mode: EnterpriseSyncMode = EnterpriseSyncMode.PENDING) {
        invalidateIfStale()
        val request = synchronized(monitor) {
            if (closed || fence.epoch != renderedEpoch) return
            val created = Owner(next(), context, renderedEpoch, fence.backgroundEpoch)
            owner = created; cursor = null; storedGeneration = null
            mutable.value = EnterpriseSyncViewState(generation = created.token, epoch = created.epoch, context = context, mode = mode, busy = true)
            created
        }
        perform(request) { capture(request)?.let { load(request, it, mode) } }
    }
    suspend fun refresh(renderedEpoch: Long, generation: Long) {
        val detail = mutable.value.detail?.item?.identity
        val request = begin(renderedEpoch, generation) ?: return
        val mode = mutable.value.mode
        perform(request) { capture(request)?.let { load(request, it, mode, detailIdentity = detail) } }
    }
    suspend fun more(renderedEpoch: Long, generation: Long) {
        invalidateIfStale()
        if (!mutable.value.hasMore) return
        val request = begin(renderedEpoch, generation) ?: return
        val mode = mutable.value.mode
        perform(request) { capture(request)?.let { load(request, it, mode, append = true) } }
    }
    suspend fun showDetail(operationId: String, digest: String, renderedEpoch: Long, generation: Long) {
        invalidateIfStale()
        val identity = EnterpriseOperationIdentity(operationId, digest)
        if (mutable.value.items.none { it.identity == identity }) return
        val request = begin(renderedEpoch, generation) ?: return
        perform(request) {
            val attempt = capture(request) ?: return@perform
            val detail = verifiedDetail(store.detail(attempt, operationId, digest), identity)
            currentCoroutineContext().ensureActive()
            if (!retained(request)) { invalidateIfStale(); return@perform }
            synchronized(monitor) { if (owner === request && viewCurrent(request)) mutable.value = mutable.value.copy(busy = false, detail = detail) }
        }
    }
    fun backToList(renderedEpoch: Long, generation: Long) {
        invalidateIfStale()
        synchronized(monitor) {
            val before = owner ?: return
            if (closed || before.epoch != renderedEpoch || before.token != generation || !viewCurrent(before)) return
            val request = Owner(next(), before.context, before.epoch, before.background, before.account, before.attempt,
                before.expectedIdentity, before.completionReadThrough)
            owner = request
            mutable.value = mutable.value.copy(generation = request.token, busy = false, detail = null)
        }
    }
    suspend fun changeMode(mode: EnterpriseSyncMode, renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation, mode, clearRows = true) ?: return
        perform(request) { capture(request)?.let { load(request, it, mode) } }
    }
    suspend fun dismiss(operationId: String, digest: String, expectedReceiptDigest: String, renderedEpoch: Long, generation: Long) {
        invalidateIfStale()
        val identity = EnterpriseOperationIdentity(operationId, digest)
        val item = mutable.value.detail?.item?.takeIf { it.identity == identity } ?: mutable.value.items.firstOrNull { it.identity == identity } ?: return
        val receipt = item.receipt ?: return
        if (receipt.result != SyncResult.REJECTED || item.dismissedRejected || receipt.receiptDigest != expectedReceiptDigest) return
        val request = begin(renderedEpoch, generation) ?: return
        val mode = mutable.value.mode
        perform(request) {
            val attempt = capture(request) ?: return@perform
            store.dismissRejected(attempt, operationId, digest, expectedReceiptDigest)
            if (!retained(request)) { invalidateIfStale(); return@perform }
            load(request, attempt, mode)
        }
    }
    suspend fun manualCheck(online: Boolean, renderedEpoch: Long, generation: Long) {
        invalidateIfStale()
        if (!online) return
        val detail = mutable.value.detail?.item?.identity
        val request = begin(renderedEpoch, generation) ?: return
        val mode = mutable.value.mode
        perform(request) {
            val attempt = capture(request, deliveryProof = true) ?: return@perform
            val outcome = delivery.dispatch(attempt.account, request.context.partition)
            if (!retained(request)) { invalidateIfStale(); return@perform }
            load(request, attempt, mode, detailIdentity = detail, waiting = outcome.retryAfterMillis, attention = outcome.attention)
        }
    }
    suspend fun onActivityChanged() {
        val request = synchronized(monitor) { owner } ?: return
        perform(request) {
            if (!continueRetained(request)) return@perform
            val attempt = request.attempt ?: return@perform
            val active = activity.current(attempt)
            if (!continueRetained(request)) return@perform
            synchronized(monitor) {
                if (owner === request && viewCurrent(request)) mutable.value = mutable.value.copy(active = active)
            }
        }
    }
}
