package com.anpfuel.app.ui.community

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.community.GetCityCommunityFeedUseCase
import com.anpfuel.domain.community.CommunityFeedItem
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
    val newUpdates: Boolean = false,
)

/** Foreground-only refresh. Filters and city changes cancel obsolete HTTP work. */
@HiltViewModel
class CommunityFeedViewModel @Inject constructor(private val feed: GetCityCommunityFeedUseCase) : ViewModel() {
    private val mutable = MutableStateFlow(CommunityFeedUiState())
    val state = mutable.asStateFlow()
    private var foreground = false
    private var refreshJob: Job? = null
    private var pageJob: Job? = null
    private var resetPagination = false
    internal var now: () -> Instant = { Instant.now() }

    fun setForeground(value: Boolean) {
        if (foreground == value) return
        foreground = value
        if (value) startRefresh() else {
            refreshJob?.cancel()
            pageJob?.cancel()
            mutable.update { it.copy(loading = false, loadingMore = false) }
        }
    }
    fun selectFuel(fuel: FuelProduct) {
        if (fuel == mutable.value.fuel) return
        mutable.update { it.copy(fuel = fuel, items = emptyList(), nextCursor = null, lastUpdated = null, failed = false, newUpdates = false) }
        startRefresh()
    }
    fun selectSort(sort: FeedSort) {
        if (sort == mutable.value.sort) return
        mutable.update { it.copy(sort = sort, items = emptyList(), nextCursor = null, lastUpdated = null, failed = false, newUpdates = false) }
        startRefresh()
    }
    fun refresh() {
        resetPagination = true
        startRefresh()
    }
    private fun startRefresh() {
        refreshJob?.cancel()
        pageJob?.cancel()
        if (!foreground) return
        refreshJob = viewModelScope.launch {
            while (isActive) {
                refreshFirstPage()
                delay(15_000)
            }
        }
    }
    private suspend fun refreshFirstPage() {
        mutable.update { it.copy(loading = it.items.isEmpty(), loadingMore = false, items = it.items.filter { row -> row.expiresAt > now() }) }
        try {
            val city = feed.city()
            if (city == null) {
                mutable.update { it.copy(city = null, items = emptyList(), loading = false, noCity = true, failed = false, nextCursor = null, lastUpdated = null, newUpdates = false) }
                return
            }
            if (city != mutable.value.city) { pageJob?.cancel() }
            if (city != mutable.value.city) mutable.update {
                it.copy(city = city, items = emptyList(), nextCursor = null, lastUpdated = null, newUpdates = false, noCity = false)
            }
            val query = FeedQuery(city, mutable.value.fuel, mutable.value.sort)
            val page = feed(query)
            val items = page.items.filter { it.expiresAt > now() }.distinctBy { it.stationId }
            mutable.update {
                // Preserve scroll/pagination while browsing older rows. Advertise new
                // first-page facts instead of moving cards underneath the reader.
                if (it.items.size > 20 && !resetPagination) it.copy(loading = false, failed = false, noCity = false,
                    items = it.items.filter { row -> row.expiresAt > now() }, newUpdates = items != it.items.take(20),
                    lastUpdated = page.generatedAt)
                else it.copy(city = city, items = items, nextCursor = page.nextCursor, loading = false,
                    failed = false, noCity = false, lastUpdated = page.generatedAt, newUpdates = false)
            }
            resetPagination = false
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            mutable.update { it.copy(loading = false, failed = true, items = it.items.filter { row -> row.expiresAt > now() }) }
        }
    }
    fun loadMore() {
        val snapshot = mutable.value
        val cursor = snapshot.nextCursor ?: return
        val city = snapshot.city ?: return
        if (!foreground || snapshot.loadingMore || snapshot.loading || snapshot.items.size >= 200) return
        pageJob = viewModelScope.launch {
            mutable.update { it.copy(loadingMore = true) }
            try {
                val page = feed(FeedQuery(city, snapshot.fuel, snapshot.sort), cursor)
                mutable.update {
                    if (it.city != city || it.fuel != snapshot.fuel || it.sort != snapshot.sort) return@update it
                    it.copy(items = (it.items + page.items).distinctBy { row -> row.stationId }.filter { row -> row.expiresAt > now() },
                        nextCursor = page.nextCursor, loadingMore = false, failed = false)
                }
            } catch (error: CancellationException) { throw error
            } catch (error: Exception) { mutable.update { it.copy(loadingMore = false, failed = true) } }
        }
    }
}
