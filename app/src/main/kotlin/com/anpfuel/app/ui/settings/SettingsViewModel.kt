package com.anpfuel.app.ui.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.app.locale.AppLocaleHolder
import com.anpfuel.app.locale.AppLocales
import com.anpfuel.application.error.AppError
import com.anpfuel.application.error.AppErrorResolver
import com.anpfuel.app.notification.NotificationPermissionHandler
import com.anpfuel.application.usecase.settings.ClearCacheUseCase
import com.anpfuel.application.usecase.settings.GetSettingsUseCase
import com.anpfuel.application.usecase.settings.GetStorageUsageUseCase
import com.anpfuel.application.usecase.settings.UpdatePreferencesUseCase
import com.anpfuel.application.usecase.sync.AutoDownloadLatestWeekOutcome
import com.anpfuel.application.usecase.sync.AutoDownloadLatestWeekUseCase
import com.anpfuel.application.usecase.sync.SyncPriceTablesUseCase
import com.anpfuel.application.usecase.vehicle.ListVehiclesUseCase
import com.anpfuel.data.worker.SyncWorkScheduler
import com.anpfuel.domain.event.CacheClearScope
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.event.SyncRequestSource
import com.anpfuel.domain.model.StorageUsage
import com.anpfuel.domain.model.UserPreferences
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

data class SettingsUiState(
    val isLoading: Boolean = true,
    val hasLoaded: Boolean = false,
    val preferences: UserPreferences = UserPreferences(),
    val storageUsage: StorageUsage = StorageUsage(
        summaryRowCount = 0,
        stationRowCount = 0,
        importedWeekCount = 0,
    ),
    val isSyncing: Boolean = false,
    val isClearingCache: Boolean = false,
    val showClearAllDialog: Boolean = false,
    val syncMessage: String? = null,
    val showNotificationPermissionHint: Boolean = false,
    val error: AppError? = null,
)

sealed interface SettingsEffect {
    data object RecreateActivity : SettingsEffect
    data object NavigateToOnboarding : SettingsEffect
}

@HiltViewModel
class SettingsViewModel @Inject constructor(
    private val getSettingsUseCase: GetSettingsUseCase,
    private val getStorageUsageUseCase: GetStorageUsageUseCase,
    private val updatePreferencesUseCase: UpdatePreferencesUseCase,
    private val clearCacheUseCase: ClearCacheUseCase,
    private val syncPriceTablesUseCase: SyncPriceTablesUseCase,
    private val autoDownloadLatestWeekUseCase: AutoDownloadLatestWeekUseCase,
    private val syncWorkScheduler: SyncWorkScheduler,
    private val listVehiclesUseCase: ListVehiclesUseCase,
    private val notificationPermissionHandler: NotificationPermissionHandler,
) : ViewModel() {

    private val _uiState = MutableStateFlow(SettingsUiState())
    val uiState: StateFlow<SettingsUiState> = _uiState.asStateFlow()

    private val _effects = MutableSharedFlow<SettingsEffect>(extraBufferCapacity = 1)
    val effects: SharedFlow<SettingsEffect> = _effects.asSharedFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            runCatching {
                val preferences = getSettingsUseCase()
                val storageUsage = getStorageUsageUseCase()
                val vehicles = listVehiclesUseCase()
                val showNotificationPermissionHint = vehicles.any { it.priceDropAlertEnabled } &&
                    !notificationPermissionHandler.hasPostNotificationsPermission()
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        hasLoaded = true,
                        preferences = preferences,
                        storageUsage = storageUsage,
                        showNotificationPermissionHint = showNotificationPermissionHint,
                    )
                }
            }.onFailure { error ->
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        error = AppErrorResolver.fromThrowable(error),
                    )
                }
            }
        }
    }

    fun onLocaleSelected(localeTag: String) {
        if (localeTag !in AppLocales.supportedLocaleTags) {
            return
        }
        if (_uiState.value.preferences.localeTag == localeTag) {
            return
        }
        updatePreferences(_uiState.value.preferences.copy(localeTag = localeTag, localeUserSelected = true)) {
            AppLocaleHolder.localeTag = localeTag
            _effects.emit(SettingsEffect.RecreateActivity)
        }
    }

    fun onSyncStationDetailChanged(enabled: Boolean) {
        updatePreferences(_uiState.value.preferences.copy(syncStationDetail = enabled))
    }

    fun onAutoDownloadLatestWeekChanged(enabled: Boolean) {
        updatePreferences(_uiState.value.preferences.copy(autoDownloadLatestWeek = enabled))
    }

    fun onAutoSyncOnWifiChanged(enabled: Boolean) {
        updatePreferences(_uiState.value.preferences.copy(autoSyncOnWifi = enabled)) {
            syncWorkScheduler.schedulePeriodicSync()
        }
    }

    fun onShowPriceHistoryChanged(enabled: Boolean) {
        updatePreferences(_uiState.value.preferences.copy(showPriceHistory = enabled))
    }

    fun onRetentionWeeksSelected(weeks: Int) {
        if (_uiState.value.preferences.stationDetailRetentionWeeks == weeks) {
            return
        }
        updatePreferences(_uiState.value.preferences.copy(stationDetailRetentionWeeks = weeks))
    }

    /** UC-015 — search radius for the nearest best-price station (BR-028). */
    fun onNearestStationRadiusSelected(radiusKm: Int) {
        if (radiusKm !in UserPreferences.NEAREST_STATION_RADIUS_KM_OPTIONS) {
            return
        }
        if (_uiState.value.preferences.nearestStationRadiusKm == radiusKm) {
            return
        }
        updatePreferences(_uiState.value.preferences.copy(nearestStationRadiusKm = radiusKm))
    }

    fun syncNow() {
        if (_uiState.value.isSyncing) {
            return
        }

        viewModelScope.launch {
            _uiState.update { it.copy(isSyncing = true, error = null, syncMessage = null) }
            val syncError = runCatching {
                when (val outcome = autoDownloadLatestWeekUseCase(SyncRequestSource.MANUAL)) {
                    is AutoDownloadLatestWeekOutcome.Disabled -> {
                        val result = syncPriceTablesUseCase(SyncRequestSource.MANUAL)
                        if (result.outcome == SyncJobOutcome.FAILED) {
                            result.error ?: AppError.SyncNetworkError
                        } else {
                            null
                        }
                    }
                    is AutoDownloadLatestWeekOutcome.UpToDate -> null
                    is AutoDownloadLatestWeekOutcome.Success -> {
                        if (outcome.syncResult.outcome == SyncJobOutcome.FAILED) {
                            outcome.syncResult.error ?: AppError.SyncNetworkError
                        } else {
                            null
                        }
                    }
                    is AutoDownloadLatestWeekOutcome.Failed -> outcome.error
                }
            }.getOrElse { AppErrorResolver.fromThrowable(it) }

            if (syncError != null) {
                _uiState.update {
                    it.copy(
                        isSyncing = false,
                        error = syncError,
                    )
                }
                return@launch
            }

            val storageUsage = getStorageUsageUseCase()
            syncWorkScheduler.enqueuePriceDropEvaluation()
            _uiState.update {
                it.copy(
                    isSyncing = false,
                    storageUsage = storageUsage,
                    syncMessage = SYNC_COMPLETED_MESSAGE,
                )
            }
        }
    }

    fun openNotificationSettings(launchContext: android.content.Context) {
        notificationPermissionHandler.openNotificationSettings(launchContext)
    }

    fun requestClearAllCache() {
        _uiState.update { it.copy(showClearAllDialog = true) }
    }

    fun dismissClearAllDialog() {
        _uiState.update { it.copy(showClearAllDialog = false) }
    }

    fun clearStationCache() {
        clearCache(CacheClearScope.STATION_DETAIL_ONLY)
    }

    fun confirmClearAllCache() {
        _uiState.update { it.copy(showClearAllDialog = false) }
        clearCache(CacheClearScope.ALL) {
            _effects.emit(SettingsEffect.NavigateToOnboarding)
        }
    }

    private fun clearCache(
        scope: CacheClearScope,
        onSuccess: suspend () -> Unit = {},
    ) {
        if (_uiState.value.isClearingCache) {
            return
        }

        viewModelScope.launch {
            _uiState.update { it.copy(isClearingCache = true, error = null) }
            runCatching {
                clearCacheUseCase(scope)
                val preferences = getSettingsUseCase()
                val storageUsage = getStorageUsageUseCase()
                _uiState.update {
                    it.copy(
                        isClearingCache = false,
                        preferences = preferences,
                        storageUsage = storageUsage,
                    )
                }
                onSuccess()
            }.onFailure { error ->
                _uiState.update {
                    it.copy(
                        isClearingCache = false,
                        error = AppErrorResolver.fromThrowable(error),
                    )
                }
            }
        }
    }

    private fun updatePreferences(
        updated: UserPreferences,
        onSuccess: suspend () -> Unit = {},
    ) {
        viewModelScope.launch {
            runCatching {
                val result = updatePreferencesUseCase(updated)
                _uiState.update { it.copy(preferences = result.preferences, error = null) }
                onSuccess()
            }.onFailure { error ->
                _uiState.update {
                    it.copy(error = AppErrorResolver.fromThrowable(error))
                }
            }
        }
    }

    companion object {
        const val SYNC_COMPLETED_MESSAGE = "sync_completed"
        val retentionWeekOptions: List<Int> = listOf(4, 8, 12, 24)
    }
}
