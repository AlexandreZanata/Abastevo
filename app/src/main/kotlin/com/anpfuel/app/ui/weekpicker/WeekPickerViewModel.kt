package com.anpfuel.app.ui.weekpicker

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.sync.DiscoverSurveyWeekCatalogOutcome
import com.anpfuel.application.usecase.sync.DiscoverSurveyWeekCatalogUseCase
import com.anpfuel.application.usecase.sync.SelectWeekAndSyncResult
import com.anpfuel.application.usecase.sync.SelectWeekAndSyncUseCase
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.valueobject.SurveyWeekSelectionMode
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

sealed interface WeekPickerNavigation {
    data object Completed : WeekPickerNavigation
}

@HiltViewModel
class WeekPickerViewModel @Inject constructor(
    private val discoverSurveyWeekCatalogUseCase: DiscoverSurveyWeekCatalogUseCase,
    private val selectWeekAndSyncUseCase: SelectWeekAndSyncUseCase,
) : ViewModel() {

    private val _uiState = MutableStateFlow(WeekPickerUiState(isLoadingCatalog = true))
    val uiState: StateFlow<WeekPickerUiState> = _uiState.asStateFlow()

    private val _navigation = MutableSharedFlow<WeekPickerNavigation>(extraBufferCapacity = 1)
    val navigation: SharedFlow<WeekPickerNavigation> = _navigation.asSharedFlow()

    init {
        loadCatalog()
    }

    fun loadCatalog() {
        if (_uiState.value.isSyncing) {
            return
        }

        _uiState.update { it.copy(isLoadingCatalog = true, catalogError = null) }
        viewModelScope.launch {
            when (val outcome = discoverSurveyWeekCatalogUseCase()) {
                is DiscoverSurveyWeekCatalogOutcome.Success -> {
                    _uiState.update {
                        it.copy(
                            catalog = outcome.catalog,
                            isLoadingCatalog = false,
                            catalogError = null,
                        )
                    }
                }

                is DiscoverSurveyWeekCatalogOutcome.Failure -> {
                    _uiState.update {
                        it.copy(
                            isLoadingCatalog = false,
                            catalogError = outcome.error,
                        )
                    }
                }
            }
        }
    }

    fun useLatestWeek() {
        val latestEntry = _uiState.value.catalog.firstOrNull() ?: return
        selectWeek(latestEntry, SurveyWeekSelectionMode.LATEST)
    }

    fun onWeekRowTapped(entry: SurveyWeekCatalogEntry) {
        if (_uiState.value.isSyncing) {
            return
        }

        val catalog = _uiState.value.catalog
        if (WeekPickerSelectionPolicy.requiresConfirmation(catalog, entry)) {
            _uiState.update { it.copy(pendingConfirmation = entry) }
            return
        }

        val mode = if (WeekPickerSelectionPolicy.isLatestCatalogEntry(catalog, entry)) {
            SurveyWeekSelectionMode.LATEST
        } else {
            SurveyWeekSelectionMode.SPECIFIC
        }
        selectWeek(entry, mode)
    }

    fun confirmPendingWeek() {
        val entry = _uiState.value.pendingConfirmation ?: return
        _uiState.update { it.copy(pendingConfirmation = null) }
        selectWeek(entry, SurveyWeekSelectionMode.SPECIFIC)
    }

    fun dismissPendingConfirmation() {
        _uiState.update { it.copy(pendingConfirmation = null) }
    }

    fun retrySync() {
        val entry = _uiState.value.pendingWeekSelection ?: return
        val mode = _uiState.value.pendingSelectionMode ?: SurveyWeekSelectionMode.LATEST
        selectWeek(entry, mode)
    }

    private fun selectWeek(
        entry: SurveyWeekCatalogEntry,
        selectionMode: SurveyWeekSelectionMode,
    ) {
        if (_uiState.value.isSyncing) {
            return
        }

        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    isSyncing = true,
                    syncError = null,
                    pendingConfirmation = null,
                    pendingWeekSelection = entry,
                    pendingSelectionMode = selectionMode,
                )
            }

            when (
                val result = selectWeekAndSyncUseCase(
                    catalogEntry = entry,
                    selectionMode = selectionMode,
                )
            ) {
                is SelectWeekAndSyncResult.Failed -> {
                    _uiState.update {
                        it.copy(
                            isSyncing = false,
                            syncError = result.error,
                        )
                    }
                }

                is SelectWeekAndSyncResult.Success -> {
                    _uiState.update { it.copy(isSyncing = false, syncError = null) }
                    _navigation.emit(WeekPickerNavigation.Completed)
                }
            }
        }
    }
}
