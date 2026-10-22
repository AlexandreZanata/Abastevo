package com.anpfuel.app.ui.occurrence

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.domain.occurrence.OccurrenceLease
import com.anpfuel.domain.occurrence.OccurrenceRecord
import com.anpfuel.domain.occurrence.RecordCoverage
import com.anpfuel.domain.repository.OccurrenceGateway
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class OccurrenceFilter(
    val uf: String? = null,
    val fromDay: String? = null,
    val toDay: String? = null,
)

data class OccurrenceUiState(
    val filter: OccurrenceFilter = OccurrenceFilter(),
    val items: List<OccurrenceRecord> = emptyList(),
    val cursor: String? = null,
    val loading: Boolean = false,
    val loadingMore: Boolean = false,
    val unavailable: Boolean = false,
    val stale: Boolean = false,
    val coverage: RecordCoverage? = null,
    val lease: OccurrenceLease? = null,
)

@HiltViewModel
class OccurrenceExplorerViewModel @Inject constructor(
    savedStateHandle: SavedStateHandle,
    private val gateway: OccurrenceGateway,
) : ViewModel() {
    private val enabled = com.anpfuel.app.BuildConfig.OCCURRENCE_UI_ENABLED
    private val mutable = MutableStateFlow(OccurrenceUiState(
        filter = OccurrenceFilter(
            uf = savedStateHandle["uf"],
            fromDay = savedStateHandle["fromDay"],
            toDay = savedStateHandle["toDay"],
        ),
        unavailable = !enabled,
    ))
    val state = mutable.asStateFlow()
    private var generation = 0L
    private var readJob: Job? = null

    init { if (enabled) refresh() }

    fun refresh() {
        if (!enabled) return
        mutable.update { it.copy(items = emptyList(), cursor = null, coverage = null) }
        load(first = true)
    }

    fun selectFilter(filter: OccurrenceFilter) {
        if (!enabled) return
        mutable.update { it.copy(filter = filter, items = emptyList(), cursor = null, coverage = null) }
        load(first = true)
    }

    fun nextPage() {
        if (!enabled) return
        val current = mutable.value
        if (current.loadingMore || current.cursor == null) return
        load(first = false)
    }

    private fun load(first: Boolean) {
        readJob?.cancel()
        val myGeneration = ++generation
        readJob = viewModelScope.launch {
            mutable.update { if (first) it.copy(loading = true, unavailable = false) else it.copy(loadingMore = true, unavailable = false) }
            try {
                val filter = mutable.value.filter
                val page = gateway.records(
                    uf = filter.uf, fromDay = filter.fromDay, toDay = filter.toDay,
                    limit = 20, cursor = if (first) null else mutable.value.cursor)
                if (myGeneration != generation) return@launch
                val (coverage, lease) = try {
                    gateway.recordCoverage()
                } catch (_: Exception) { null to null }
                if (myGeneration != generation) return@launch
                mutable.update {
                    it.copy(
                        items = if (first) page.items else it.items + page.items,
                        cursor = page.nextCursor, loading = false, loadingMore = false,
                        stale = page.stale, coverage = coverage,
                        lease = lease ?: page.lease,
                        unavailable = false,
                    )
                }
            } catch (e: CancellationException) { throw e }
            catch (_: Exception) {
                if (myGeneration != generation) return@launch
                mutable.update { it.copy(loading = false, loadingMore = false, unavailable = it.items.isEmpty()) }
            }
        }
    }
}
