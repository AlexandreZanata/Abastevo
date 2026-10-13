package com.anpfuel.app.ui.weekpicker

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.error.AppError
import com.anpfuel.application.usecase.sync.DiscoverSurveyWeekCatalogOutcome
import com.anpfuel.application.usecase.sync.DiscoverSurveyWeekCatalogUseCase
import com.anpfuel.application.usecase.sync.DownloadSurveyWeekResult
import com.anpfuel.application.usecase.sync.DownloadSurveyWeekUseCase
import com.anpfuel.application.usecase.sync.GetDownloadedSurveyWeeksUseCase
import com.anpfuel.application.usecase.sync.SelectWeekAndSyncResult
import com.anpfuel.application.usecase.sync.SelectWeekAndSyncUseCase
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.valueobject.SurveyWeekSelectionMode
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Job
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
    private val downloadSurveyWeekUseCase: DownloadSurveyWeekUseCase,
    private val getDownloadedSurveyWeeksUseCase: GetDownloadedSurveyWeeksUseCase,
) : ViewModel() {

    private val _uiState = MutableStateFlow(WeekPickerUiState(isLoadingCatalog = true))
    val uiState: StateFlow<WeekPickerUiState> = _uiState.asStateFlow()

    private val _navigation = MutableSharedFlow<WeekPickerNavigation>(extraBufferCapacity = 1)
    val navigation: SharedFlow<WeekPickerNavigation> = _navigation.asSharedFlow()

    private var syncJob: Job? = null
    private var cancelledByUser = false

    init {
        loadCatalog()
    }

    fun loadCatalog() {
        if (_uiState.value.isSyncing) {
            return
        }

        _uiState.update { it.copy(isLoadingCatalog = true, catalogError = null) }
        viewModelScope.launch {
            val downloadedWeeks = runCatching { getDownloadedSurveyWeeksUseCase() }
                .getOrDefault(_uiState.value.downloadedWeeks)
            when (val outcome = discoverSurveyWeekCatalogUseCase()) {
                is DiscoverSurveyWeekCatalogOutcome.Success -> {
                    _uiState.update {
                        it.copy(
                            catalog = outcome.catalog,
                            downloadedWeeks = downloadedWeeks,
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
        val mode = if (WeekPickerSelectionPolicy.isLatestCatalogEntry(catalog, entry)) {
            SurveyWeekSelectionMode.LATEST
        } else {
            SurveyWeekSelectionMode.SPECIFIC
        }
        selectWeek(entry, mode)
    }

    fun cancelSync() {
        cancelledByUser = true
        syncJob?.cancel()
    }

    fun retrySync() {
        val entry = _uiState.value.pendingWeekSelection ?: return
        if (_uiState.value.pendingDownloadOnly) {
            downloadWeek(entry)
            return
        }
        val mode = _uiState.value.pendingSelectionMode ?: SurveyWeekSelectionMode.LATEST
        selectWeek(entry, mode)
    }

    /**
     * Downloads a week in place without changing the active survey week, so
     * several weeks can be downloaded in a row from the same list.
     */
    fun downloadWeek(entry: SurveyWeekCatalogEntry) {
        if (_uiState.value.isSyncing) {
            return
        }

        val catalog = _uiState.value.catalog
        val mode = if (WeekPickerSelectionPolicy.isLatestCatalogEntry(catalog, entry)) {
            SurveyWeekSelectionMode.LATEST
        } else {
            SurveyWeekSelectionMode.SPECIFIC
        }
        launchSync(entry = entry, selectionMode = mode, persistSelection = false)
    }

    private fun selectWeek(
        entry: SurveyWeekCatalogEntry,
        selectionMode: SurveyWeekSelectionMode,
    ) {
        launchSync(entry = entry, selectionMode = selectionMode, persistSelection = true)
    }

    private fun launchSync(
        entry: SurveyWeekCatalogEntry,
        selectionMode: SurveyWeekSelectionMode,
        persistSelection: Boolean,
    ) {
        if (_uiState.value.isSyncing) {
            return
        }

        cancelledByUser = false
        syncJob = viewModelScope.launch {
            _uiState.update {
                it.copy(
                    isSyncing = true,
                    syncError = null,
                    pendingWeekSelection = entry,
                    pendingSelectionMode = selectionMode,
                    pendingDownloadOnly = !persistSelection,
                )
            }

            try {
                if (persistSelection) {
                    when (
                        val result = selectWeekAndSyncUseCase(
                            catalogEntry = entry,
                            selectionMode = selectionMode,
                        )
                    ) {
                        is SelectWeekAndSyncResult.Failed -> handleSyncFailed(result.error)
                        is SelectWeekAndSyncResult.Success -> {
                            refreshDownloadedWeeks()
                            _navigation.emit(WeekPickerNavigation.Completed)
                        }
                    }
                } else {
                    when (val result = downloadSurveyWeekUseCase(catalogEntry = entry)) {
                        is DownloadSurveyWeekResult.Failed -> handleSyncFailed(result.error)
                        is DownloadSurveyWeekResult.Success -> refreshDownloadedWeeks()
                    }
                }
            } finally {
                cancelledByUser = false
                syncJob = null
                _uiState.update {
                    if (it.isSyncing) {
                        it.copy(
                            isSyncing = false,
                            syncError = null,
                            pendingWeekSelection = null,
                            pendingSelectionMode = null,
                            pendingDownloadOnly = false,
                        )
                    } else {
                        it
                    }
                }
            }
        }
    }

    private fun handleSyncFailed(error: AppError) {
        if (cancelledByUser) {
            _uiState.update {
                it.copy(
                                    isSyncing = false,
                                    syncError = null,
                                    pendingWeekSelection = null,
                                    pendingSelectionMode = null,
                                    pendingDownloadOnly = false,
                                )
                            }
                        } else {
            _uiState.update {
                it.copy(
                    isSyncing = false,
                    syncError = error,
                )
            }
        }
    }

    private suspend fun refreshDownloadedWeeks() {
        val downloadedWeeks = runCatching { getDownloadedSurveyWeeksUseCase() }
            .getOrDefault(_uiState.value.downloadedWeeks)
        _uiState.update {
            it.copy(
                isSyncing = false,
                syncError = null,
                downloadedWeeks = downloadedWeeks,
            )
        }
    }
}
