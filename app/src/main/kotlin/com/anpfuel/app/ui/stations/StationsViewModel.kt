package com.anpfuel.app.ui.stations

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.error.AppError
import com.anpfuel.application.error.AppErrorResolver
import com.anpfuel.application.usecase.location.SelectLocationUseCase
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.price.GetStationPricesUseCase
import com.anpfuel.application.usecase.price.StationPricesOutcome
import com.anpfuel.app.location.LocationPermissionHandler
import com.anpfuel.application.usecase.station.BuildStationNavigationQueryUseCase
import com.anpfuel.application.usecase.station.FindNearestBestPriceStationUseCase
import com.anpfuel.application.usecase.station.FindNearestStationOutcome
import com.anpfuel.application.usecase.sync.DownloadStationDetailUseCase
import com.anpfuel.app.mapper.StationPriceUiMapper
import com.anpfuel.app.ui.model.StationPriceUiModel
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.valueobject.DeviceLocation
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import dagger.hilt.android.lifecycle.HiltViewModel
import java.util.Locale
import javax.inject.Inject
import androidx.lifecycle.SavedStateHandle
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.launch

data class StationsUiState(
    val isLoading: Boolean = true,
    val isDownloading: Boolean = false,
    val isFindingNearest: Boolean = false,
    val isOffline: Boolean = false,
    val selectedFuelProduct: FuelProduct = FuelProduct.GASOLINE_REGULAR,
    val municipality: String? = null,
    val state: BrazilianState? = null,
    val surveyWeek: SurveyWeek? = null,
    val stations: List<StationPriceUiModel> = emptyList(),
    val showDownloadPrompt: Boolean = false,
    val showEmpty: Boolean = false,
    val showNoLocation: Boolean = false,
    val error: AppError? = null,
    val errorMessage: String? = null,
)

sealed interface StationsNavigationEffect {
    data class LaunchMaps(val navigationQuery: String) : StationsNavigationEffect
}

/**
 * UC-015 — one-shot user feedback for the nearest best-price station action.
 */
enum class StationsMessage {
    NearestNeedsLocation,
    NearestNoFix,
    NearestUnavailable,
}

@HiltViewModel
class StationsViewModel @Inject constructor(
    private val getStationPricesUseCase: GetStationPricesUseCase,
    private val buildStationNavigationQueryUseCase: BuildStationNavigationQueryUseCase,
    private val downloadStationDetailUseCase: DownloadStationDetailUseCase,
    private val findNearestBestPriceStationUseCase: FindNearestBestPriceStationUseCase,
    private val locationPermissionHandler: LocationPermissionHandler,
    private val selectLocationUseCase: SelectLocationUseCase,
    observeNetworkConnectivityUseCase: ObserveNetworkConnectivityUseCase,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val _uiState = MutableStateFlow(
        StationsUiState(
            selectedFuelProduct = savedStateHandle.get<String>(ARG_FUEL_PRODUCT)
                ?.let { runCatching { FuelProduct.valueOf(it) }.getOrNull() }
                ?: FuelProduct.GASOLINE_REGULAR,
        ),
    )
    val uiState: StateFlow<StationsUiState> = _uiState.asStateFlow()

    private val _navigationEffects = MutableSharedFlow<StationsNavigationEffect>(extraBufferCapacity = 1)
    val navigationEffects: SharedFlow<StationsNavigationEffect> = _navigationEffects.asSharedFlow()

    private val _messages = MutableSharedFlow<StationsMessage>(extraBufferCapacity = 1)
    val messages: SharedFlow<StationsMessage> = _messages.asSharedFlow()

    private val _locationPermissionRequest = MutableSharedFlow<Unit>(extraBufferCapacity = 1)
    val locationPermissionRequest: SharedFlow<Unit> = _locationPermissionRequest.asSharedFlow()

    private val stationByCnpj = mutableMapOf<String, RetailStation>()

    init {
        viewModelScope.launch {
            observeNetworkConnectivityUseCase().collect { isConnected ->
                _uiState.update { it.copy(isOffline = !isConnected) }
            }
        }
    }

    fun load(locale: Locale) {
        loadForFuel(_uiState.value.selectedFuelProduct, locale)
    }

    fun onFuelProductSelected(fuelProduct: FuelProduct, locale: Locale) {
        if (_uiState.value.selectedFuelProduct == fuelProduct) {
            return
        }
        _uiState.update { it.copy(selectedFuelProduct = fuelProduct) }
        loadForFuel(fuelProduct, locale)
    }

    /**
     * UC-015 — resolves the nearest best-price station for the selected fuel.
     * Requests the location permission when it was never granted (UC-012 already does this once).
     */
    fun onFindNearestStation() {
        if (_uiState.value.isFindingNearest) {
            return
        }

        if (!locationPermissionHandler.hasLocationPermission()) {
            _locationPermissionRequest.tryEmit(Unit)
            return
        }

        viewModelScope.launch {
            // UC-015: cached fixes can be days old (a week-old fix once pointed at the
            // wrong region), so request a fresh fix with a bounded timeout first.
            val deviceLocation = locationPermissionHandler.getCurrentLocation()
            if (deviceLocation == null) {
                _messages.tryEmit(StationsMessage.NearestNoFix)
                return@launch
            }
            findNearestStation(deviceLocation)
        }
    }

    fun onLocationPermissionGranted() {
        viewModelScope.launch {
            val deviceLocation = locationPermissionHandler.getCurrentLocation()
            if (deviceLocation == null) {
                _messages.tryEmit(StationsMessage.NearestNoFix)
                return@launch
            }
            findNearestStation(deviceLocation)
        }
    }

    fun onLocationPermissionDenied() {
        _messages.tryEmit(StationsMessage.NearestNeedsLocation)
    }

    private fun findNearestStation(deviceLocation: DeviceLocation) {
        viewModelScope.launch {
            _uiState.update { it.copy(isFindingNearest = true, error = null, errorMessage = null) }

            runCatching {
                findNearestBestPriceStationUseCase(
                    fuelProduct = _uiState.value.selectedFuelProduct,
                    deviceLocation = deviceLocation.coordinates,
                )
            }.onSuccess { outcome ->
                _uiState.update { it.copy(isFindingNearest = false) }
                when (outcome) {
                    is FindNearestStationOutcome.Success ->
                        _navigationEffects.emit(
                            StationsNavigationEffect.LaunchMaps(outcome.navigationQuery),
                        )

                    FindNearestStationOutcome.NoStationWithinRadius,
                    FindNearestStationOutcome.StationDetailMissing,
                    FindNearestStationOutcome.NoStations,
                    FindNearestStationOutcome.GeocodingFailed,
                    -> _messages.emit(StationsMessage.NearestUnavailable)
                }
            }.onFailure { error ->
                _uiState.update {
                    it.copy(
                        isFindingNearest = false,
                        error = AppErrorResolver.fromThrowable(error),
                        errorMessage = error.message ?: error.javaClass.simpleName,
                    )
                }
            }
        }
    }

    fun onNavigateToStation(cnpjDigits: String) {
        val station = stationByCnpj[cnpjDigits] ?: return

        viewModelScope.launch {
            val result = buildStationNavigationQueryUseCase(station)
            _navigationEffects.emit(
                StationsNavigationEffect.LaunchMaps(result.navigationQuery),
            )
        }
    }

    fun downloadStationDetail(locale: Locale) {
        val state = _uiState.value.state ?: return
        val municipality = _uiState.value.municipality ?: return
        if (_uiState.value.isDownloading) {
            return
        }

        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    isDownloading = true,
                    error = null,
                    errorMessage = null,
                )
            }

            runCatching {
                val result = downloadStationDetailUseCase(
                    state = state,
                    municipality = municipality,
                    surveyWeek = _uiState.value.surveyWeek,
                )

                if (result.outcome == SyncJobOutcome.FAILED) {
                    _uiState.update {
                        it.copy(
                            isDownloading = false,
                            error = result.error ?: AppError.StationDetailNotSynced,
                        )
                    }
                    return@launch
                }

                _uiState.update { it.copy(isDownloading = false) }
                loadForFuel(_uiState.value.selectedFuelProduct, locale)
            }.onFailure { error ->
                _uiState.update {
                    it.copy(
                        isDownloading = false,
                        error = AppErrorResolver.fromThrowable(error),
                        errorMessage = error.message ?: error.javaClass.simpleName,
                    )
                }
            }
        }
    }

    private fun loadForFuel(fuelProduct: FuelProduct, locale: Locale) {
        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    isLoading = true,
                    error = null,
                    errorMessage = null,
                    showDownloadPrompt = false,
                    showEmpty = false,
                    showNoLocation = false,
                    stations = emptyList(),
                )
            }

            runCatching {
                val preferred = selectLocationUseCase.getPreferredLocation()
                if (preferred == null) {
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            showNoLocation = true,
                        )
                    }
                    return@launch
                }

                when (val outcome = getStationPricesUseCase(fuelProduct = fuelProduct)) {
                    is StationPricesOutcome.StationDetailMissing -> {
                        _uiState.update {
                            it.copy(
                                isLoading = false,
                                showDownloadPrompt = true,
                                municipality = preferred.municipality,
                                state = preferred.state,
                                selectedFuelProduct = fuelProduct,
                            )
                        }
                    }

                    is StationPricesOutcome.Success -> {
                        stationByCnpj.clear()
                        outcome.stations.forEach { stationPrice ->
                            stationByCnpj[stationPrice.station.cnpj.digits] = stationPrice.station
                        }
                        _uiState.update {
                            it.copy(
                                isLoading = false,
                                municipality = outcome.municipality,
                                state = outcome.state,
                                surveyWeek = outcome.surveyWeek,
                                selectedFuelProduct = outcome.fuelProduct,
                                stations = StationPriceUiMapper.toUiModels(
                                    stations = outcome.stations,
                                    locale = locale,
                                    preferredState = outcome.state,
                                    preferredMunicipality = outcome.municipality,
                                ),
                                showEmpty = outcome.isEmpty,
                            )
                        }
                    }
                }
            }.onFailure { error ->
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        error = AppErrorResolver.fromThrowable(error),
                        errorMessage = error.message ?: error.javaClass.simpleName,
                    )
                }
            }
        }
    }

    companion object {
        private const val ARG_FUEL_PRODUCT = "fuelProduct"
    }
}
