package com.anpfuel.app.ui.profile

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.contribution.CancelOwnedContributionUseCase
import com.anpfuel.application.usecase.contribution.GetOwnedContributionsUseCase
import com.anpfuel.application.usecase.contribution.OwnedContributionStatus
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.ensureActive

/**
 * P21-T03 private owner status (BUC-003, B-BR-003/005).
 *
 * Lists only this device's retained outbox commands with the frozen
 * P21-T01 states. Cancelled commands carry no status and render with an
 * explicit cancelled label. Cancel removes a command from dispatch; failed
 * commands otherwise retry automatically with bounded backoff.
 */
sealed interface ContributionStatusUiState {
    data object Loading : ContributionStatusUiState
    data object Empty : ContributionStatusUiState
    data object Error : ContributionStatusUiState
    data class Content(
        val items: List<OwnedContributionStatus>,
        val hasMore: Boolean = false,
        val loadingMore: Boolean = false,
        val moreFailed: Boolean = false,
    ) : ContributionStatusUiState
}

@HiltViewModel
class ContributionStatusViewModel @Inject constructor(
    private val getOwnedContributionsUseCase: GetOwnedContributionsUseCase,
    private val cancelOwnedContributionUseCase: CancelOwnedContributionUseCase,
) : ViewModel() {

    private val _uiState =
        MutableStateFlow<ContributionStatusUiState>(ContributionStatusUiState.Loading)
    val uiState: StateFlow<ContributionStatusUiState> = _uiState.asStateFlow()

    private var loadJob: kotlinx.coroutines.Job? = null
    private var generation = 0L
    private var nextOffset: Int? = null
    private var scopeKey: String? = null

    fun load() {
        loadJob?.cancel()
        val current = ++generation
        _uiState.value = ContributionStatusUiState.Loading
        loadJob = viewModelScope.launch {
            try {
                val page = getOwnedContributionsUseCase.page()
                kotlinx.coroutines.currentCoroutineContext().ensureActive()
                if (current != generation) return@launch
                nextOffset = page.nextOffset
                scopeKey = page.scopeKey
                _uiState.value = if (page.items.isEmpty() && page.nextOffset == null) ContributionStatusUiState.Empty
                    else ContributionStatusUiState.Content(page.items, page.nextOffset != null)
            } catch (cancelled: kotlinx.coroutines.CancellationException) { throw cancelled
            } catch (_: Exception) { if (current == generation) _uiState.value = ContributionStatusUiState.Error }
        }
    }

    fun loadMore() {
        val shown = _uiState.value as? ContributionStatusUiState.Content ?: return
        val offset = nextOffset ?: return
        if (shown.loadingMore || loadJob?.isActive == true) return
        val current = generation
        _uiState.value = shown.copy(loadingMore = true, moreFailed = false)
        loadJob = viewModelScope.launch {
            try {
                val page = getOwnedContributionsUseCase.page(offset = offset)
                kotlinx.coroutines.currentCoroutineContext().ensureActive()
                if (current != generation) return@launch
                if (page.scopeKey != scopeKey) { load(); return@launch }
                nextOffset = page.nextOffset
                _uiState.value = ContributionStatusUiState.Content((shown.items + page.items).distinctBy { it.commandId }, page.nextOffset != null)
            } catch (cancelled: kotlinx.coroutines.CancellationException) { throw cancelled
            } catch (_: Exception) { if (current == generation) _uiState.value = shown.copy(moreFailed = true) }
        }
    }

    fun onCancel(commandId: String) {
        viewModelScope.launch {
            runCatching { cancelOwnedContributionUseCase.invoke(commandId) }
            load()
        }
    }
}
