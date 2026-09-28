package com.anpfuel.app.ui.onboarding

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.app.ui.weekpicker.WeekPickerSelectionPolicy
import com.anpfuel.application.error.AppError
import com.anpfuel.application.usecase.location.ResolveDeviceLocationOutcome
import com.anpfuel.app.location.LocationPermissionHandler
import com.anpfuel.application.usecase.location.ResolveDeviceLocationUseCase
import com.anpfuel.application.usecase.onboarding.CompleteLocationPromptUseCase
import com.anpfuel.application.usecase.onboarding.CompleteOnboardingResult
import com.anpfuel.application.usecase.onboarding.CompleteOnboardingUseCase
import com.anpfuel.application.usecase.onboarding.OnboardingSelectWeekAndSyncResult
import com.anpfuel.application.usecase.onboarding.OnboardingSelectWeekAndSyncUseCase
import com.anpfuel.application.usecase.sync.DiscoverSurveyWeekCatalogOutcome
import com.anpfuel.application.usecase.sync.DiscoverSurveyWeekCatalogUseCase
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.DeviceLocation
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

enum class OnboardingStep {
    INTRO,
    WEEK_PICKER,
    SYNCING,
    LOCATION_PROMPT,
}

data class OnboardingUiState(
    val step: OnboardingStep = OnboardingStep.INTRO,
    val pageIndex: Int = 0,
    val pageCount: Int = OnboardingViewModel.PAGE_COUNT,
    val catalog: List<SurveyWeekCatalogEntry> = emptyList(),
    val isLoadingCatalog: Boolean = false,
    val catalogError: AppError? = null,
    val error: AppError? = null,
    val pendingWeekSelection: SurveyWeekCatalogEntry? = null,
    val pendingSelectionMode: SurveyWeekSelectionMode? = null,
    val pendingConfirmation: SurveyWeekCatalogEntry? = null,
    val isResolvingLocation: Boolean = false,
) {
    val isOnLastPage: Boolean
        get() = pageIndex == pageCount - 1
}

sealed interface OnboardingNavigation {
    data object ToHome : OnboardingNavigation
    data object ToLocation : OnboardingNavigation
}

@HiltViewModel
class OnboardingViewModel @Inject constructor(
    private val discoverSurveyWeekCatalogUseCase: DiscoverSurveyWeekCatalogUseCase,
    private val onboardingSelectWeekAndSyncUseCase: OnboardingSelectWeekAndSyncUseCase,
    private val completeOnboardingUseCase: CompleteOnboardingUseCase,
    private val completeLocationPromptUseCase: CompleteLocationPromptUseCase,
    private val resolveDeviceLocationUseCase: ResolveDeviceLocationUseCase,
    private val locationPermissionHandler: LocationPermissionHandler,
    private val userPreferencesRepository: UserPreferencesRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(OnboardingUiState())
    val uiState: StateFlow<OnboardingUiState> = _uiState.asStateFlow()

    private val _navigation = MutableSharedFlow<OnboardingNavigation>(extraBufferCapacity = 1)
    val navigation: SharedFlow<OnboardingNavigation> = _navigation.asSharedFlow()

    private val _locationPermissionRequest = MutableSharedFlow<Unit>(extraBufferCapacity = 1)
    val locationPermissionRequest: SharedFlow<Unit> = _locationPermissionRequest.asSharedFlow()

    fun onNextPage() {
        _uiState.update { state ->
            if (state.pageIndex < state.pageCount - 1) {
                state.copy(pageIndex = state.pageIndex + 1, error = null)
            } else {
                state
            }
        }
    }

    fun onPreviousPage() {
        _uiState.update { state ->
            if (state.pageIndex > 0) {
                state.copy(pageIndex = state.pageIndex - 1, error = null)
            } else {
                state
            }
        }
    }

    fun onPageSelected(index: Int) {
        _uiState.update { it.copy(pageIndex = index.coerceIn(0, it.pageCount - 1), error = null) }
    }

    fun proceedToWeekPicker() {
        if (_uiState.value.isLoadingCatalog || _uiState.value.step == OnboardingStep.SYNCING) {
            return
        }

        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    step = OnboardingStep.WEEK_PICKER,
                    isLoadingCatalog = true,
                    catalogError = null,
                    error = null,
                )
            }

            when (val outcome = discoverSurveyWeekCatalogUseCase()) {
                is DiscoverSurveyWeekCatalogOutcome.Success -> {
                    val preferences = userPreferencesRepository.getPreferences()
                    if (preferences.autoDownloadLatestWeek) {
                        val latestEntry = outcome.catalog.firstOrNull()
                        if (latestEntry != null) {
                            _uiState.update {
                                it.copy(
                                    catalog = outcome.catalog,
                                    isLoadingCatalog = false,
                                    catalogError = null,
                                )
                            }
                            selectWeekAndSync(latestEntry, SurveyWeekSelectionMode.LATEST)
                            return@launch
                        }
                    }

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

    fun retryCatalogDiscovery() {
        proceedToWeekPicker()
    }

    fun useLatestWeekAndSync() {
        val latestEntry = _uiState.value.catalog.firstOrNull() ?: return
        selectWeekAndSync(latestEntry, SurveyWeekSelectionMode.LATEST)
    }

    fun selectWeekAndSync(entry: SurveyWeekCatalogEntry) {
        if (_uiState.value.step == OnboardingStep.SYNCING) {
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
        selectWeekAndSync(entry, mode)
    }

    fun confirmPendingWeek() {
        val entry = _uiState.value.pendingConfirmation ?: return
        _uiState.update { it.copy(pendingConfirmation = null) }
        selectWeekAndSync(entry, SurveyWeekSelectionMode.SPECIFIC)
    }

    fun dismissPendingConfirmation() {
        _uiState.update { it.copy(pendingConfirmation = null) }
    }

    fun retrySync() {
        val entry = _uiState.value.pendingWeekSelection ?: return
        val mode = _uiState.value.pendingSelectionMode ?: SurveyWeekSelectionMode.SPECIFIC
        selectWeekAndSync(entry, mode)
    }

    fun skipSync() {
        viewModelScope.launch {
            completeOnboardingUseCase.skipSync()
            _navigation.emit(OnboardingNavigation.ToHome)
        }
    }

    fun backToIntro() {
        _uiState.update {
            it.copy(
                step = OnboardingStep.INTRO,
                catalogError = null,
                error = null,
            )
        }
    }

    fun onChooseManualLocation() {
        viewModelScope.launch {
            completeLocationPromptUseCase()
            _navigation.emit(OnboardingNavigation.ToLocation)
        }
    }

    fun onUseDeviceLocationClick() {
        if (_uiState.value.isResolvingLocation) {
            return
        }

        if (locationPermissionHandler.hasLocationPermission()) {
            onResolveDeviceLocation(locationPermissionHandler.getLastKnownLocation())
        } else {
            viewModelScope.launch {
                _locationPermissionRequest.emit(Unit)
            }
        }
    }

    fun onLocationPermissionGranted() {
        onResolveDeviceLocation(locationPermissionHandler.getLastKnownLocation())
    }

    fun onLocationPermissionDenied() {
        viewModelScope.launch {
            completeLocationPromptUseCase()
            _navigation.emit(OnboardingNavigation.ToLocation)
        }
    }

    fun onResolveDeviceLocation(location: DeviceLocation?) {
        if (_uiState.value.isResolvingLocation) {
            return
        }

        if (location == null) {
            viewModelScope.launch {
                completeLocationPromptUseCase()
                _navigation.emit(OnboardingNavigation.ToLocation)
            }
            return
        }

        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    isResolvingLocation = true,
                )
            }

            when (resolveDeviceLocationUseCase(location)) {
                is ResolveDeviceLocationOutcome.Success -> {
                    completeLocationPromptUseCase()
                    _uiState.update { it.copy(isResolvingLocation = false) }
                    _navigation.emit(OnboardingNavigation.ToHome)
                }

                ResolveDeviceLocationOutcome.MunicipalityNotInCatalog,
                ResolveDeviceLocationOutcome.RateLimited,
                ResolveDeviceLocationOutcome.NetworkError,
                ResolveDeviceLocationOutcome.InvalidGeocodeResponse,
                -> {
                    completeLocationPromptUseCase()
                    _uiState.update { it.copy(isResolvingLocation = false) }
                    _navigation.emit(OnboardingNavigation.ToLocation)
                }
            }
        }
    }

    private fun selectWeekAndSync(
        entry: SurveyWeekCatalogEntry,
        selectionMode: SurveyWeekSelectionMode,
    ) {
        if (_uiState.value.step == OnboardingStep.SYNCING) {
            return
        }

        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    step = OnboardingStep.SYNCING,
                    error = null,
                    pendingConfirmation = null,
                    pendingWeekSelection = entry,
                    pendingSelectionMode = selectionMode,
                )
            }

            when (
                val result = onboardingSelectWeekAndSyncUseCase(
                    catalogEntry = entry,
                    selectionMode = selectionMode,
                )
            ) {
                is OnboardingSelectWeekAndSyncResult.SyncFailed -> {
                    _uiState.update {
                        it.copy(
                            step = OnboardingStep.WEEK_PICKER,
                            error = result.error,
                        )
                    }
                }

                is OnboardingSelectWeekAndSyncResult.Completed -> {
                    handleOnboardingCompletion(result.onboardingResult)
                }
            }
        }
    }

    private suspend fun handleOnboardingCompletion(onboardingResult: CompleteOnboardingResult) {
        when (onboardingResult) {
            CompleteOnboardingResult.Completed,
            CompleteOnboardingResult.AlreadyCompleted,
            -> navigateAfterCompletion()

            CompleteOnboardingResult.NotReady -> {
                _uiState.update {
                    it.copy(
                        step = OnboardingStep.WEEK_PICKER,
                        error = AppError.SyncNetworkError,
                    )
                }
            }

            CompleteOnboardingResult.Skipped -> {
                _uiState.update { it.copy(step = OnboardingStep.WEEK_PICKER) }
            }
        }
    }

    private suspend fun navigateAfterCompletion() {
        val preferences = userPreferencesRepository.getPreferences()
        when {
            !preferences.preferredMunicipality.isNullOrBlank() -> {
                _uiState.update { it.copy(step = OnboardingStep.SYNCING, error = null) }
                _navigation.emit(OnboardingNavigation.ToHome)
            }

            preferences.locationPromptCompleted -> {
                _uiState.update { it.copy(step = OnboardingStep.SYNCING, error = null) }
                _navigation.emit(OnboardingNavigation.ToLocation)
            }

            else -> {
                _uiState.update {
                    it.copy(
                        step = OnboardingStep.LOCATION_PROMPT,
                        error = null,
                        isResolvingLocation = false,
                    )
                }
            }
        }
    }

    companion object {
        const val PAGE_COUNT = 3
    }
}
