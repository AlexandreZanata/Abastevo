package com.anpfuel.app.ui.community

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.community.GetCityCommunityFeedUseCase
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.domain.community.CommunityFeedItem
import com.anpfuel.domain.community.CommunityRefreshPolicy
import com.anpfuel.domain.community.CommunityFeedLocalSnapshot
import com.anpfuel.domain.community.FeedCity
import com.anpfuel.domain.community.FeedQuery
import com.anpfuel.domain.community.FeedSort
import com.anpfuel.domain.valueobject.FuelProduct
import dagger.hilt.android.lifecycle.HiltViewModel
import java.time.Instant
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

data class CommunityFeedUiState(
    val city: FeedCity? = null,
    val fuel: FuelProduct = FuelProduct.GASOLINE_REGULAR,
    val sort: FeedSort = FeedSort.RECENT,
    val items: List<CommunityFeedItem> = emptyList(),
    val loading: Boolean = true,
    val loadingMore: Boolean = false,
    val failed: Boolean = false,
    val noCity: Boolean = false,
    val nextCursor: String? = null,
    val lastUpdated: Instant? = null,
    val stale: Boolean = false,
    val newUpdates: Boolean = false,
    val earlierItemsTrimmed: Boolean = false,
)

/** Foreground-only refresh. Filters and city changes cancel obsolete HTTP work. */
@HiltViewModel
class CommunityFeedViewModel @Inject constructor(
    private val feed: GetCityCommunityFeedUseCase,
    private val snapshots: CommunityFeedLocalSnapshot,
    observeNetworkConnectivityUseCase: ObserveNetworkConnectivityUseCase,
) : ViewModel() {
    private val mutable = MutableStateFlow(CommunityFeedUiState())
    val state = mutable.asStateFlow()
    private var foreground = false
    private var connected = true
    private var refreshJob: Job? = null
    private var pageJob: Job? = null
    private var firstPageJob: Job? = null
    private var needsLiveCursor = false
    private var resetPagination = false
    private var initialHead: List<CommunityFeedItem> = emptyList()
    private var lastAttemptMillis: Long? = null
    private var consecutiveFailures = 0
    internal var now: () -> Instant = { Instant.now() }

    init {
        viewModelScope.launch {
            observeNetworkConnectivityUseCase().distinctUntilChanged().collect { isConnected ->
                val wasOffline = !connected && isConnected
                connected = isConnected
                // A new network is an explicit event: reset backoff and re-evaluate.
                if (wasOffline && foreground) {
                    consecutiveFailures = 0
                    startRefreshLoop()
                }
            }
        }
    }

    fun setForeground(value: Boolean) {
        if (!value) {
            foreground = false
            refreshJob?.cancel()
            pageJob?.cancel()
            mutable.update { it.copy(loading = false, loadingMore = false) }
            return
        }
        if (!foreground) consecutiveFailures = 0
        foreground = true
        startRefreshLoop()
    }
    fun selectFuel(fuel: FuelProduct) {
        if (fuel == mutable.value.fuel) return
        mutable.update { it.copy(fuel = fuel, items = emptyList(), nextCursor = null, lastUpdated = null, failed = false, stale = false, newUpdates = false, earlierItemsTrimmed = false) }
        resetScope()
        startRefreshLoop()
    }
    fun selectSort(sort: FeedSort) {
        if (sort == mutable.value.sort) return
        mutable.update { it.copy(sort = sort, items = emptyList(), nextCursor = null, lastUpdated = null, failed = false, stale = false, newUpdates = false, earlierItemsTrimmed = false) }
        resetScope()
        startRefreshLoop()
    }
    fun refresh() {
        resetPagination = true
        consecutiveFailures = 0
        startRefreshLoop()
    }

    /**
     * Time until the next policy evaluation, always honoring the loop floor.
     * Local expiry sweeps never issue a query on their own.
     */
    internal fun nextDelayMillis(): Long {
        // Mirror evaluate(): without a city no fetch can happen, so re-check
        // the scope on the sweep tick instead of busy-looping on zero.
        if (mutable.value.city == null || !connected || !foreground) {
            return CommunityRefreshPolicy.SWEEP_INTERVAL_MILLIS
        }
        if (shouldFetchNow()) return CommunityRefreshPolicy.MIN_LOOP_DELAY_MILLIS
        val nowMillis = now().toEpochMilli()
        var delay = CommunityRefreshPolicy.SWEEP_INTERVAL_MILLIS
        mutable.value.lastUpdated?.let { updated ->
            val remaining = CommunityRefreshPolicy.STALE_AFTER_MILLIS - (nowMillis - updated.toEpochMilli())
            if (remaining > 0) delay = minOf(delay, remaining)
        }
        if (consecutiveFailures > 0 && CommunityRefreshPolicy.shouldKeepRetrying(consecutiveFailures)) {
            val lastAttempt = lastAttemptMillis ?: nowMillis
            val remaining = CommunityRefreshPolicy.retryDelayMillis(consecutiveFailures) - (nowMillis - lastAttempt)
            if (remaining > 0) delay = minOf(delay, remaining)
        }
        return maxOf(delay, CommunityRefreshPolicy.MIN_LOOP_DELAY_MILLIS)
    }

    private fun resetScope() {
        consecutiveFailures = 0
        lastAttemptMillis = null
        initialHead = emptyList()
        needsLiveCursor = false
    }

    private fun startRefreshLoop() {
        refreshJob?.cancel()
        pageJob?.cancel()
        // Cancellation has no page result to clear the spinner. Fresh-scope
        // reconnects may skip the head query, so release it at cancellation.
        mutable.update { it.copy(loadingMore = false) }
        if (!foreground) return
        refreshJob = viewModelScope.launch {
            evaluate()
            while (isActive) {
                delay(nextDelayMillis())
                if (!isActive) break
                evaluate()
            }
        }
    }

    private suspend fun evaluate() {
        sweepExpiredLocally()
        val city = try {
            feed.city()
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            // Local lookup failed: keep painted content, report only when empty.
            if (mutable.value.items.isEmpty() && mutable.value.lastUpdated == null) {
                mutable.update { it.copy(loading = false, failed = true) }
            }
            refreshStale()
            return
        }
        currentCoroutineContext().ensureActive()
        if (city == null) {
            mutable.update { it.copy(city = null, items = emptyList(), loading = false, noCity = true, failed = false, nextCursor = null, lastUpdated = null, stale = false, newUpdates = false, earlierItemsTrimmed = false) }
            return
        }
        if (city != mutable.value.city) { pageJob?.cancel() }
        if (city != mutable.value.city) {
            resetScope()
            mutable.update {
                it.copy(city = city, items = emptyList(), nextCursor = null, lastUpdated = null, stale = false, newUpdates = false, earlierItemsTrimmed = false, noCity = false)
            }
        }
        paintSnapshotIfEmpty(city)
        refreshStale()
        if (!connected) {
            // No query without a connection; aged snapshot stays with an error mark.
            resetPagination = false
            mutable.update { it.copy(loading = false, failed = true) }
            refreshStale()
            return
        }
        if (shouldFetchNow() && pageJob?.isActive != true) {
            refreshFirstPage(city)
        } else {
            mutable.update { it.copy(loading = false) }
        }
        refreshStale()
    }

    private suspend fun paintSnapshotIfEmpty(city: FeedCity) {
        if (mutable.value.items.isNotEmpty() || mutable.value.lastUpdated != null) return
        val snapshot = try {
            snapshots.snapshotFeed(city, mutable.value.fuel, mutable.value.sort)
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            null
        } ?: return
        currentCoroutineContext().ensureActive()
        // Cursors are deliberately not persisted. A full page needs a live
        // head before it can truthfully offer a continuation.
        needsLiveCursor = snapshot.items.size >= 20
        mutable.update {
            it.copy(
                items = snapshot.items,
                nextCursor = null,
                loading = false,
                failed = false,
                noCity = false,
                lastUpdated = snapshot.generatedAt,
                newUpdates = false,
                earlierItemsTrimmed = false,
            )
        }
        if (snapshot.items.size <= 20) initialHead = snapshot.items
    }

    private fun shouldFetchNow(): Boolean {
        if (!connected || !foreground) return false
        if (resetPagination) return true
        val nowMillis = now().toEpochMilli()
        // Never refetch within the loop floor of the previous attempt; the loop
        // delay already guarantees progress, so back-to-back evaluations converge.
        val lastAttempt = lastAttemptMillis
        if (lastAttempt != null && nowMillis - lastAttempt < CommunityRefreshPolicy.MIN_LOOP_DELAY_MILLIS) return false
        if (consecutiveFailures > 0) {
            if (!CommunityRefreshPolicy.shouldKeepRetrying(consecutiveFailures)) return false
            if (nowMillis - lastAttempt!! < CommunityRefreshPolicy.retryDelayMillis(consecutiveFailures)) return false
        }
        if (needsLiveCursor || (mutable.value.items.isEmpty() && mutable.value.lastUpdated == null)) return true
        return CommunityRefreshPolicy.isStale(nowMillis, mutable.value.lastUpdated?.toEpochMilli())
    }

    private fun sweepExpiredLocally() {
        val now = now()
        mutable.update { state ->
            val live = state.items.filter { row -> row.expiresAt > now }
            if (live.size == state.items.size) state else state.copy(items = live)
        }
        refreshStale()
    }

    private fun refreshStale() {
        val nowMillis = now().toEpochMilli()
        mutable.update { state ->
            state.copy(stale = state.lastUpdated?.let { updated -> CommunityRefreshPolicy.isStale(nowMillis, updated.toEpochMilli()) } ?: false)
        }
    }
    private suspend fun refreshFirstPage(city: FeedCity) {
        mutable.update { it.copy(loading = it.items.isEmpty(), items = it.items.filter { row -> row.expiresAt > now() }) }
        lastAttemptMillis = now().toEpochMilli()
        val owner = currentCoroutineContext()[Job]
        firstPageJob = owner
        try {
            val query = FeedQuery(city, mutable.value.fuel, mutable.value.sort)
            val page = feed(query)
            currentCoroutineContext().ensureActive()
            consecutiveFailures = 0
            needsLiveCursor = false
            val items = page.items.filter { it.expiresAt > now() }.distinctBy { it.stationId }
            mutable.update {
                // Preserve scroll/pagination while browsing older rows. Advertise new
                // first-page facts instead of moving cards underneath the reader.
                if ((it.items.size > 20 || it.loadingMore || it.earlierItemsTrimmed) && !resetPagination) it.copy(loading = false, failed = false, noCity = false,
                    items = it.items.filter { row -> row.expiresAt > now() }, newUpdates = items != initialHead,
                    lastUpdated = page.generatedAt)
                else it.copy(city = city, items = items, nextCursor = page.nextCursor, loading = false, loadingMore = false,
                    failed = false, noCity = false, lastUpdated = page.generatedAt, stale = false, newUpdates = false, earlierItemsTrimmed = false)
            }
            if (!mutable.value.earlierItemsTrimmed && mutable.value.items.size <= 20) initialHead = items
            resetPagination = false
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            consecutiveFailures++
            resetPagination = false
            mutable.update { it.copy(loading = false, failed = true, items = it.items.filter { row -> row.expiresAt > now() }) }
        } finally {
            if (firstPageJob === owner) firstPageJob = null
        }
    }
    fun loadMore() {
        val snapshot = mutable.value
        val cursor = snapshot.nextCursor ?: return
        val city = snapshot.city ?: return
        if (!foreground || snapshot.loadingMore || snapshot.loading || pageJob?.isActive == true || firstPageJob?.isActive == true) return
        if (!connected) {
            mutable.update { it.copy(failed = true) }
            return
        }
        pageJob = viewModelScope.launch {
            mutable.update { it.copy(loadingMore = true) }
            try {
                val page = feed(FeedQuery(city, snapshot.fuel, snapshot.sort), cursor)
                currentCoroutineContext().ensureActive()
                require(page.nextCursor != cursor) { "feed cursor did not advance" }
                mutable.update {
                    if (it.city != city || it.fuel != snapshot.fuel || it.sort != snapshot.sort) return@update it
                    val merged = (it.items + page.items).distinctBy { row -> row.stationId }.filter { row -> row.expiresAt > now() }
                    it.copy(items = merged.takeLast(200), earlierItemsTrimmed = it.earlierItemsTrimmed || merged.size > 200,
                        nextCursor = page.nextCursor, loadingMore = false, failed = false)
                }
            } catch (error: CancellationException) { throw error
            } catch (error: Exception) { mutable.update { it.copy(loadingMore = false, failed = true) } }
        }
    }
}
