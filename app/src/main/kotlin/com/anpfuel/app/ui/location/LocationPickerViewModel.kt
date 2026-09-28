package com.anpfuel.app.ui.location

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.app.location.LocationPermissionHandler
import com.anpfuel.application.usecase.location.CatalogMunicipalityItem
import com.anpfuel.application.usecase.location.PreferredLocation
import com.anpfuel.application.usecase.location.ResolveDeviceLocationOutcome
import com.anpfuel.application.usecase.location.ResolveDeviceLocationUseCase
import com.anpfuel.application.usecase.location.SelectLocationUseCase
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DeviceLocation
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

sealed interface LocationPickerStep {
    data object StateList : LocationPickerStep
    data class MunicipalityList(val state: BrazilianState) : LocationPickerStep
}

data class LocationPickerUiState(
    val isLoading: Boolean = true,
    val step: LocationPickerStep = LocationPickerStep.StateList,
    val states: List<BrazilianState> = emptyList(),
    val municipalities: List<CatalogMunicipalityItem> = emptyList(),
    val stateSearchQuery: String = "",
    val municipalitySearchQuery: String = "",
    val preferredLocation: PreferredLocation? = null,
    val isSaving: Boolean = false,
    val isResolvingLocation: Boolean = false,
    val locationResolveError: String? = null,
    val errorMessage: String? = null,
) {
    val municipalitiesEmpty: Boolean
        get() = step is LocationPickerStep.MunicipalityList && municipalities.isEmpty() && !isLoading
}

sealed interface LocationPickerNavigation {
    data object ToHome : LocationPickerNavigation
}

@HiltViewModel
class LocationPickerViewModel @Inject constructor(
    private val selectLocationUseCase: SelectLocationUseCase,
    private val resolveDeviceLocationUseCase: ResolveDeviceLocationUseCase,
    private val locationPermissionHandler: LocationPermissionHandler,
) : ViewModel() {

    private val _uiState = MutableStateFlow(LocationPickerUiState())
    val uiState: StateFlow<LocationPickerUiState> = _uiState.asStateFlow()

    private val _navigation = MutableSharedFlow<LocationPickerNavigation>(extraBufferCapacity = 1)
    val navigation: SharedFlow<LocationPickerNavigation> = _navigation.asSharedFlow()

    private val _locationPermissionRequest = MutableSharedFlow<Unit>(extraBufferCapacity = 1)
    val locationPermissionRequest: SharedFlow<Unit> = _locationPermissionRequest.asSharedFlow()

    init {
        loadStates()
    }

    fun onStateSearchQueryChange(query: String) {
        _uiState.update { it.copy(stateSearchQuery = query) }
    }

    fun onMunicipalitySearchQueryChange(query: String) {
        _uiState.update { it.copy(municipalitySearchQuery = query) }
    }

    fun loadStates() {
        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    isLoading = true,
                    errorMessage = null,
                    step = LocationPickerStep.StateList,
                    municipalitySearchQuery = "",
                )
            }
            runCatching {
                val preferred = selectLocationUseCase.getPreferredLocation()
                val result = selectLocationUseCase.getStatesWithData()
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        states = result.states.sortedBy { state -> state.abbreviation },
                        preferredLocation = preferred,
                    )
                }
            }.onFailure { error ->
                _uiState.update {
                    it.copy(isLoading = false, errorMessage = error.message ?: error.javaClass.simpleName)
                }
            }
        }
    }

    fun onStateSelected(state: BrazilianState) {
        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    isLoading = true,
                    errorMessage = null,
                    step = LocationPickerStep.MunicipalityList(state),
                    municipalities = emptyList<CatalogMunicipalityItem>(),
                    municipalitySearchQuery = "",
                )
            }
            runCatching {
                val result = selectLocationUseCase.getMunicipalities(state)
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        municipalities = result.municipalities,
                    )
                }
            }.onFailure { error ->
                _uiState.update {
                    it.copy(isLoading = false, errorMessage = error.message ?: error.javaClass.simpleName)
                }
            }
        }
    }

    fun onBackToStates() {
        _uiState.update {
            it.copy(
                step = LocationPickerStep.StateList,
                municipalities = emptyList<CatalogMunicipalityItem>(),
                municipalitySearchQuery = "",
                errorMessage = null,
            )
        }
    }

    fun onMunicipalitySelected(municipality: String) {
        val state = (_uiState.value.step as? LocationPickerStep.MunicipalityList)?.state ?: return
        if (_uiState.value.isSaving) {
            return
        }

        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, errorMessage = null) }
            runCatching {
                selectLocationUseCase.selectLocation(state = state, municipality = municipality)
                _uiState.update { it.copy(isSaving = false) }
                _navigation.emit(LocationPickerNavigation.ToHome)
            }.onFailure { error ->
                _uiState.update {
                    it.copy(
                        isSaving = false,
                        errorMessage = error.message ?: error.javaClass.simpleName,
                    )
                }
            }
        }
    }

    fun onUseCurrentLocationClick() {
        if (_uiState.value.isResolvingLocation) {
            return
        }

        _uiState.update { it.copy(locationResolveError = null) }

        if (locationPermissionHandler.hasLocationPermission()) {
            resolveDeviceLocation(locationPermissionHandler.getLastKnownLocation())
        } else {
            viewModelScope.launch {
                _locationPermissionRequest.emit(Unit)
            }
        }
    }

    fun onLocationPermissionGranted() {
        resolveDeviceLocation(locationPermissionHandler.getLastKnownLocation())
    }

    fun onLocationPermissionDenied() {
        // Permission denied — user must choose manually
    }

    private fun resolveDeviceLocation(location: DeviceLocation?) {
        if (_uiState.value.isResolvingLocation) {
            return
        }

        if (location == null) {
            _uiState.update {
                it.copy(locationResolveError = "location_current_failed")
            }
            return
        }

        viewModelScope.launch {
            _uiState.update { it.copy(isResolvingLocation = true, locationResolveError = null) }

            when (resolveDeviceLocationUseCase(location)) {
                is ResolveDeviceLocationOutcome.Success -> {
                    _uiState.update { it.copy(isResolvingLocation = false) }
                    _navigation.emit(LocationPickerNavigation.ToHome)
                }

                ResolveDeviceLocationOutcome.MunicipalityNotInCatalog,
                ResolveDeviceLocationOutcome.RateLimited,
                ResolveDeviceLocationOutcome.NetworkError,
                ResolveDeviceLocationOutcome.InvalidGeocodeResponse,
                -> {
                    _uiState.update {
                        it.copy(
                            isResolvingLocation = false,
                            locationResolveError = "location_current_failed",
                        )
                    }
                }
            }
        }
    }
}
