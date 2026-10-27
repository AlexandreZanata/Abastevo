package com.anpfuel.app.ui.contributors

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.contributor.GetContributorsUseCase
import com.anpfuel.domain.contributor.Contributor
import com.anpfuel.domain.contributor.ContributorException
import dagger.hilt.android.lifecycle.HiltViewModel
import java.io.IOException
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** Honest public-list states; loading, empty, error-with-retry and offline. */
enum class ContributorsNotice { OFFLINE, DENIED, ERROR }

data class ContributorsUiState(
    val loading: Boolean = true,
    val items: List<Contributor> = emptyList(),
    val notice: ContributorsNotice? = null,
    val hasMore: Boolean = false,
    val loadingMore: Boolean = false,
)

/**
 * B-BR-CTB public contributors list (CTB-09/10).
 *
 * Manual cursor paging: pages accumulate, `next_cursor` drives the next
 * fetch. Late responses are fenced by generation; nothing here needs a
 * session because the endpoint is anonymous.
 */
@HiltViewModel
class ContributorsViewModel @Inject constructor(
    private val contributors: GetContributorsUseCase,
) : ViewModel() {
    private val state = MutableStateFlow(ContributorsUiState())
    val uiState = state.asStateFlow()
    private var generation = 0L
    private var cursor: String? = null

    fun refresh() {
        val request = ++generation
        cursor = null
        state.value = ContributorsUiState(loading = true)
        viewModelScope.launch {
            try {
                val page = contributors(null)
                if (request != generation) return@launch
                state.value = ContributorsUiState(
                    loading = false,
                    items = page.items,
                    hasMore = page.nextCursor.isNotEmpty(),
                )
                cursor = page.nextCursor.ifEmpty { null }
            } catch (e: Exception) {
                if (request != generation) return@launch
                state.value = ContributorsUiState(loading = false, notice = noticeOf(e))
            }
        }
    }

    fun loadMore() {
        val current = state.value
        if (current.loading || current.loadingMore || !current.hasMore || current.notice != null) return
        val request = ++generation
        state.value = current.copy(loadingMore = true)
        viewModelScope.launch {
            try {
                val page = contributors(cursor)
                if (request != generation) return@launch
                state.value = current.copy(
                    items = current.items + page.items,
                    hasMore = page.nextCursor.isNotEmpty(),
                    loadingMore = false,
                )
                cursor = page.nextCursor.ifEmpty { null }
            } catch (e: Exception) {
                if (request != generation) return@launch
                state.value = current.copy(loadingMore = false, notice = noticeOf(e))
            }
        }
    }

    private fun noticeOf(e: Exception): ContributorsNotice = when (e) {
        is ContributorException.Offline, is IOException -> ContributorsNotice.OFFLINE
        is ContributorException.Denied -> ContributorsNotice.DENIED
        else -> ContributorsNotice.ERROR
    }
}
