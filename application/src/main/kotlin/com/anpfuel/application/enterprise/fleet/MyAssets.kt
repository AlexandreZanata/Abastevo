package com.anpfuel.application.enterprise.fleet

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import com.anpfuel.domain.enterprise.fleet.MyAssets
import com.anpfuel.domain.enterprise.fleet.MyAssetsSnapshot
import com.anpfuel.domain.enterprise.fleet.ReferenceAsset
import com.anpfuel.domain.enterprise.fleet.ReferenceCache
import com.anpfuel.domain.enterprise.fleet.ReferenceFeedRequest
import com.anpfuel.domain.enterprise.fleet.ReferenceFeedResponse
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/*
 * "Meus ativos" and the asset picker (EMP-FLEET-013, B-BR-EMP-348..352). The cache is
 * a protected read copy of the signed reference feed; nothing here is authority.
 */

/** The protected per-partition reference cache (B-BR-EMP-348). */
interface EnterpriseReferenceCacheStore {
    /** The cache bound to the attempt's exact membership and enrollment; null when absent, foreign or unreadable. */
    suspend fun read(attempt: EnterpriseQueueAttempt): ReferenceCache?
    /** One protected write: commits [next] only while the stored cache is still [expected]; false otherwise. */
    suspend fun replace(attempt: EnterpriseQueueAttempt, expected: ReferenceCache?, next: ReferenceCache): Boolean
}

/** A 404 from the signed feed: a cursor the server no longer verifies, or an unknown source. */
class EnterpriseReferenceNotFound : IllegalStateException("Enterprise reference feed not found")

/** One signed pull: the exact request sent and its parsed, correlated response. */
class EnterpriseReferencePull(val request: ReferenceFeedRequest, val response: ReferenceFeedResponse) {
    override fun toString(): String = "EnterpriseReferencePull(redacted)"
}

/** Signed transport; returns only a fully parsed response bound to [heldRevision] (null without a cursor). */
fun interface EnterpriseReferenceTransport {
    suspend fun pull(attempt: EnterpriseQueueAttempt, cursor: String?, heldRevision: Long?): EnterpriseReferencePull
}

/** The cache a refresh left in place: [applied] is false when a concurrent refresh won the commit. */
class EnterpriseReferenceRefresh(val cache: ReferenceCache?, val applied: Boolean) {
    override fun toString(): String = "EnterpriseReferenceRefresh(redacted)"
}

/** Offline read and online refresh of one partition's cache (B-BR-EMP-349). */
class EnterpriseReferenceService(
    private val source: EnterpriseQueueSource,
    private val store: EnterpriseReferenceCacheStore,
    private val transport: EnterpriseReferenceTransport,
    private val clock: () -> EnterpriseDeviceClock,
) {
    private suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt {
        account.assertCurrent()
        if (!account.identity.owns(partition)) throw EnterpriseLocalDenied()
        val attempt = source.attempt(account, partition)
        if (attempt.account !== account || attempt.partition != partition) throw EnterpriseLocalDenied()
        attempt.assertCurrent()
        return attempt
    }

    /** No network: the protected cache of the exact partition and enrollment, or null. */
    suspend fun cached(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): ReferenceCache? {
        val attempt = attempt(account, partition)
        return store.read(attempt).also { attempt.assertCurrent() }
    }

    /** Pull with the held cursor, apply whole and commit by compare-and-set; failures change nothing. */
    suspend fun refresh(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseReferenceRefresh {
        val attempt = attempt(account, partition)
        val prior = store.read(attempt)
        attempt.assertCurrent()
        val pull = try { transport.pull(attempt, prior?.cursor, prior?.revision) }
        catch (notFound: EnterpriseReferenceNotFound) {
            // A cursor the server no longer verifies must not lock this device out: one full pull, once.
            if (prior == null) throw notFound
            attempt.assertCurrent()
            transport.pull(attempt, null, null)
        }
        attempt.assertCurrent()
        val next = ReferenceCache.apply(prior.takeIf { pull.request.snapshot().cursor != null }, pull.request, pull.response, clock())
        if (store.replace(attempt, prior, next)) { attempt.assertCurrent(); return EnterpriseReferenceRefresh(next, true) }
        // Another refresh committed first: show its whole state, never a mix.
        return EnterpriseReferenceRefresh(store.read(attempt).also { attempt.assertCurrent() }, false)
    }
}

data class MyAssetsViewState(
    val generation: Long = 0,
    val epoch: Long? = null,
    val context: EnterpriseContextSummary? = null,
    val busy: Boolean = false,
    /** The protected cache was read; [snapshot] null then means nothing was ever pulled. */
    val loaded: Boolean = false,
    val snapshot: MyAssetsSnapshot? = null,
    /** The latest refresh in this view failed; the cached list stays as it was. */
    val refreshFailed: Boolean = false,
    val error: EnterpriseAccessError? = null,
    val unlockRequired: Boolean = false,
    val unrecoverable: Boolean = false,
    val detailAssetId: String? = null,
) {
    val detail: ReferenceAsset? get() = detailAssetId?.let { id -> snapshot?.assets?.firstOrNull { it.asset.id == id }?.asset }
    override fun toString(): String = "MyAssetsViewState(redacted)"
}

/** One visible owner per screen; it clears itself when the account, origin, epoch or background lease changes. */
class MyAssetsFlow(
    private val source: EnterpriseQueueSource,
    private val service: EnterpriseReferenceService,
    private val fence: EnterpriseSessionFence,
    private val clock: () -> EnterpriseDeviceClock,
) {
    private val mutable = MutableStateFlow(MyAssetsViewState())
    val state: StateFlow<MyAssetsViewState> = mutable.asStateFlow()
    private val monitor = Any()
    private var serial = 0L
    private var closed = false
    private class Owner(val token: Long, val context: EnterpriseContextSummary, val epoch: Long, val background: Long,
        var account: EnterpriseQueueAccountLease? = null, var cache: ReferenceCache? = null)
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
        mutable.value = MyAssetsViewState(generation = next(), error = error)
    }
    fun withdraw() = synchronized(monitor) { clearLocked() }
    fun close() = synchronized(monitor) { if (!closed) { clearLocked(); closed = true } }
    fun invalidateIfStale() {
        val captured = synchronized(monitor) { owner } ?: return
        if (!retained(captured)) synchronized(monitor) { if (owner === captured) clearLocked(EnterpriseAccessError.STALE) }
    }

    /** A new generation of the current owner for a rendered gesture, or null for an old one. */
    private fun begin(renderedEpoch: Long, generation: Long, busy: Boolean): Owner? {
        invalidateIfStale()
        return synchronized(monitor) {
            val before = owner ?: return@synchronized null
            if (closed || (busy && mutable.value.busy) || before.epoch != renderedEpoch || before.token != generation ||
                fence.epoch != renderedEpoch || fence.backgroundEpoch != before.background) return@synchronized null
            val request = Owner(next(), before.context, before.epoch, before.background, before.account, before.cache)
            owner = request
            mutable.value = mutable.value.copy(generation = request.token, busy = busy || mutable.value.busy)
            request
        }
    }

    private fun publish(request: Owner, cache: ReferenceCache?, refreshFailed: Boolean) {
        val snapshot = cache?.let { MyAssets.present(it, clock()) }
        synchronized(monitor) {
            if (owner !== request || !viewCurrent(request)) return
            request.cache = cache
            val detail = mutable.value.detailAssetId?.takeIf { id -> snapshot?.assets?.any { it.asset.id == id } == true }
            mutable.value = mutable.value.copy(busy = false, loaded = true, snapshot = snapshot, refreshFailed = refreshFailed,
                error = null, unlockRequired = false, unrecoverable = false, detailAssetId = detail)
        }
    }

    private fun failed(request: Owner, failure: Exception) {
        val sourceCurrent = retained(request)
        synchronized(monitor) {
            if (owner !== request || closed) return
            if (failure is EnterpriseLocalDenied || !sourceCurrent || !viewCurrent(request)) clearLocked(EnterpriseAccessError.STALE)
            else mutable.value = mutable.value.copy(busy = false, loaded = false, snapshot = null, detailAssetId = null,
                error = EnterpriseAccessError.UNAVAILABLE, unlockRequired = failure is EnterpriseUnlockRequired,
                unrecoverable = failure is EnterpriseVaultUnrecoverable)
        }
    }

    private suspend fun perform(request: Owner, block: suspend () -> Unit) {
        try { currentCoroutineContext().ensureActive(); block() }
        catch (cancelled: CancellationException) { synchronized(monitor) { if (owner === request) clearLocked() }; throw cancelled }
        catch (failure: Exception) { failed(request, failure) }
    }

    /** Pulls when [online]; a failed pull keeps the cached list and only says it is not confirmed now. */
    private suspend fun pull(request: Owner) {
        val refreshed = try {
            // Delivery proof may refresh the session; its failure is a failed pull, not a lost list.
            val account = source.deliveryAccount()
            if (!account.identity.owns(request.context.partition) || account.epoch != request.epoch || account.background != request.background)
                throw EnterpriseLocalDenied()
            // A same-family refresh replaces the stored credentials without moving the fence and retires the lease
            // `open` captured: adopt the fresh delivery lease first, then ask whether the view is current (EDR-335).
            synchronized(monitor) { if (owner === request) request.account = account }
            if (!retained(request)) { invalidateIfStale(); return }
            service.refresh(account, request.context.partition)
        }
        catch (cancelled: CancellationException) { throw cancelled }
        catch (denied: EnterpriseLocalDenied) { throw denied }
        catch (unlock: EnterpriseUnlockRequired) { throw unlock }
        catch (lost: EnterpriseVaultUnrecoverable) { throw lost }
        catch (_: Exception) {
            if (retained(request)) publish(request, request.cache, refreshFailed = true) else invalidateIfStale()
            return
        }
        if (!retained(request)) { invalidateIfStale(); return }
        publish(request, refreshed.cache, refreshFailed = false)
    }

    /** Reads the protected cache offline, then refreshes when [online]. */
    suspend fun open(context: EnterpriseContextSummary, renderedEpoch: Long, online: Boolean) {
        invalidateIfStale()
        val request = synchronized(monitor) {
            if (closed || fence.epoch != renderedEpoch) return
            val created = Owner(next(), context, renderedEpoch, fence.backgroundEpoch)
            owner = created
            mutable.value = MyAssetsViewState(generation = created.token, epoch = created.epoch, context = context, busy = true)
            created
        }
        perform(request) {
            val account = source.account()
            if (!account.identity.owns(context.partition) || account.epoch != request.epoch || account.background != request.background)
                throw EnterpriseLocalDenied()
            synchronized(monitor) { if (owner === request) request.account = account }
            val cache = service.cached(account, context.partition)
            if (!retained(request)) { invalidateIfStale(); return@perform }
            if (cache != null && cache.membershipId != context.membershipId) throw EnterpriseLocalDenied()
            if (!online) { publish(request, cache, refreshFailed = false); return@perform }
            synchronized(monitor) { if (owner === request) request.cache = cache }
            pull(request)
        }
    }

    suspend fun refresh(renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation, busy = true) ?: return
        perform(request) { pull(request) }
    }

    fun detail(assetId: String, renderedEpoch: Long, generation: Long) {
        if (mutable.value.snapshot?.assets?.none { it.asset.id == assetId } != false) return
        val request = begin(renderedEpoch, generation, busy = false) ?: return
        synchronized(monitor) { if (owner === request && viewCurrent(request)) mutable.value = mutable.value.copy(detailAssetId = assetId) }
    }

    fun backToList(renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation, busy = false) ?: return
        synchronized(monitor) { if (owner === request && viewCurrent(request)) mutable.value = mutable.value.copy(detailAssetId = null) }
    }

    /** Re-evaluates confirmation against the current device clock without any I/O. */
    fun represent(renderedEpoch: Long, generation: Long) {
        val request = begin(renderedEpoch, generation, busy = false) ?: return
        val cache = synchronized(monitor) { request.cache } ?: return
        if (!retained(request)) { invalidateIfStale(); return }
        val snapshot = MyAssets.present(cache, clock())
        synchronized(monitor) { if (owner === request && viewCurrent(request)) mutable.value = mutable.value.copy(snapshot = snapshot) }
    }

    /** No network: the rendered row as a hint for the calling screen (B-BR-EMP-352). */
    fun choose(assetId: String, warningAccepted: Boolean, renderedEpoch: Long, generation: Long): EnterpriseAssetChoice? {
        invalidateIfStale()
        val (request, snapshot) = synchronized(monitor) {
            val current = owner ?: return null
            if (closed || current.epoch != renderedEpoch || current.token != generation || fence.epoch != renderedEpoch) return null
            current to (mutable.value.snapshot ?: return null)
        }
        if (snapshot.forbidden || !retained(request)) return null
        return MyAssets.choose(request.context.partition.organizationId, snapshot, assetId, warningAccepted)
    }
}
