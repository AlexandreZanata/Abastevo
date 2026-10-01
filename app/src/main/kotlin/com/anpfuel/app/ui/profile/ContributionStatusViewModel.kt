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

    fun load() {
        viewModelScope.launch {
            runCatching { getOwnedContributionsUseCase.invoke() }
                .onSuccess { items ->
                    _uiState.update {
                        if (items.isEmpty()) ContributionStatusUiState.Empty
                        else ContributionStatusUiState.Content(items)
                    }
                }
                .onFailure {
                    _uiState.update { ContributionStatusUiState.Error }
                }
        }
    }

    fun onCancel(commandId: String) {
        viewModelScope.launch {
            runCatching { cancelOwnedContributionUseCase.invoke(commandId) }
            load()
        }
    }
}
