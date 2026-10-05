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
import com.anpfuel.application.usecase.directory.GetServerStationDetailUseCase
import com.anpfuel.application.usecase.directory.GetServerStationsUseCase
import com.anpfuel.application.usecase.directory.ServerStationDetailOutcome
import com.anpfuel.application.usecase.directory.ServerStationsOutcome
import com.anpfuel.application.usecase.station.BuildStationNavigationQueryUseCase
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.application.usecase.station.FindNearestBestPriceStationUseCase
import com.anpfuel.application.usecase.station.FindNearestStationOutcome
import com.anpfuel.application.usecase.sync.DownloadStationDetailUseCase
import com.anpfuel.app.mapper.StationPriceUiMapper
import com.anpfuel.app.mapper.SurveyWeekFormatter
import com.anpfuel.app.ui.model.StationDetailUiModel
import com.anpfuel.app.ui.model.StationPriceUiModel
import com.anpfuel.domain.discovery.StationDetailRule
import com.anpfuel.domain.discovery.StationDetailState
import com.anpfuel.domain.discovery.StationRowFreshness
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.valueobject.DeviceLocation
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import dagger.hilt.android.lifecycle.HiltViewModel
import java.time.LocalDate
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
    val searchQuery: String = "",
    val searchNoMatch: String? = null,
    val selectedDetail: StationDetailUiModel? = null,
    val showDownloadPrompt: Boolean = false,
    val showEmpty: Boolean = false,
    val showNoLocation: Boolean = false,
    val error: AppError? = null,
    val errorMessage: String? = null,
    /**
     * P35-T02 — canonical server discovery (UUID) alongside the legacy
     * CNPJ list. Empty by default; populated only when the community-reads
     * flag enables staging reads. Legacy ANP/offline journeys are never
     * replaced here.
     */
    val serverStations: List<ServerStation> = emptyList(),
    val isServerLoading: Boolean = false,
    val serverFromCache: Boolean = false,
    val serverError: String? = null,
    val selectedServerStation: ServerStation? = null,
    val serverDetailFromCache: Boolean = false,
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
    private val getServerStationsUseCase: GetServerStationsUseCase? = null,
    private val getServerStationDetailUseCase: GetServerStationDetailUseCase? = null,
) : ViewModel() {

    private val savedStateHandleRef: SavedStateHandle = savedStateHandle

    private val _uiState = MutableStateFlow(
        StationsUiState(
            selectedFuelProduct = savedStateHandle.get<String>(ARG_FUEL_PRODUCT)
                ?.let { runCatching { FuelProduct.valueOf(it) }.getOrNull() }
                ?: FuelProduct.GASOLINE_REGULAR,
            searchQuery = savedStateHandle.get<String>(ARG_SEARCH_QUERY).orEmpty(),
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

    /**
     * P20-T02 — last good domain list for local search and failed-refresh
     * cache recovery. Never cleared on load start; replaced only on success.
     */
    private var lastStations: List<com.anpfuel.domain.model.StationPrice> = emptyList()
    private var lastLocale: Locale? = null

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
        // Explicit filter change: drop the previous fuel's cache so stale
        // prices are never shown under the new fuel chip.
        lastStations = emptyList()
        _uiState.update {
            it.copy(
                selectedFuelProduct = fuelProduct,
                stations = emptyList(),
                searchNoMatch = null,
                selectedDetail = null,
            )
        }
        loadForFuel(fuelProduct, locale)
    }

    /**
     * P20-T02 — local station-name search over the cached domain list.
     * No reload: filter state survives rotation via [SavedStateHandle].
     */
    fun onSearchQueryChanged(query: String, locale: Locale) {
        lastLocale = locale
        savedStateHandleRef[ARG_SEARCH_QUERY] = query
        renderStations(query)
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

    /**
     * P20-T03 — opens the station detail sheet from the cached domain list.
     * Unknown CNPJ (or no survey week yet) yields no detail, never an
     * invented card.
     */
    fun onStationSelected(cnpjDigits: String) {
        val locale = lastLocale ?: return
        val state = _uiState.value
        val domain = lastStations.firstOrNull { it.station.cnpj.digits == cnpjDigits }
        val week = state.surveyWeek
        if (domain == null || week == null) {
            _uiState.update { it.copy(selectedDetail = null) }
            return
        }
        when (val detail = StationDetailRule.resolve(
            rows = listOf(domain),
            surveyWeek = week,
            today = LocalDate.now(),
            staleCache = false,
        )) {
            is StationDetailState.NoCoverage ->
                _uiState.update { it.copy(selectedDetail = null) }
            is StationDetailState.AnpReference -> {
                val row = detail.rows.first()
                _uiState.update {
                    it.copy(
                        selectedDetail = StationDetailUiModel(
                            station = StationPriceUiMapper.toUiModel(
                                stationPrice = domain,
                                locale = locale,
                                preferredState = state.state,
                                preferredMunicipality = state.municipality,
                            ),
                            surveyWeekLabel = SurveyWeekFormatter.formatRange(week, locale),
                            isStale = row.freshness == StationRowFreshness.STALE,
                            dateUnknown = row.freshness == StationRowFreshness.UNKNOWN,
                            communityDisputed = detail.communityDisputed,
                        ),
                    )
                }
            }
        }
    }

    fun onDetailDismissed() {
        _uiState.update { it.copy(selectedDetail = null) }
    }

    /**
     * P35-T02 — loads the canonical server page (UUID discovery).
     * Disabled flag or missing binding is a no-op so legacy ANP/offline
     * journeys keep working. Transport failure keeps the last good list
     * (`serverFromCache`) instead of an empty error; a first-load failure
     * reports an honest error without inventing stations.
     */
    fun loadServerStations(limit: Int = 20) {
        val useCase = getServerStationsUseCase ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(isServerLoading = true, serverError = null) }
            when (val outcome = useCase(limit, null)) {
                is ServerStationsOutcome.Disabled ->
                    _uiState.update { it.copy(isServerLoading = false) }
                is ServerStationsOutcome.Fresh ->
                    _uiState.update {
                        it.copy(
                            isServerLoading = false,
                            serverStations = outcome.page.items,
                            serverFromCache = false,
                            serverError = null,
                        )
                    }
                is ServerStationsOutcome.StaleCache ->
                    _uiState.update {
                        it.copy(
                            isServerLoading = false,
                            serverStations = outcome.page.items,
                            serverFromCache = true,
                            serverError = null,
                        )
                    }
                is ServerStationsOutcome.Unavailable ->
                    _uiState.update {
                        it.copy(
                            isServerLoading = false,
                            serverError = outcome.cause.message
                                ?: outcome.cause.javaClass.simpleName,
                        )
                    }
            }
        }
    }

    /**
     * P35-T02 — selects a canonical server station by UUID for the
     * community-first detail. Unknown UUID (or disabled reads) yields no
     * detail, never an invented card. Stale cache is labelled explicitly.
     */
    fun onServerStationSelected(stationId: String) {
        val useCase = getServerStationDetailUseCase ?: return
        viewModelScope.launch {
            when (val outcome = useCase(stationId)) {
                is ServerStationDetailOutcome.Disabled ->
                    _uiState.update { it.copy(selectedServerStation = null) }
                is ServerStationDetailOutcome.Fresh ->
                    _uiState.update {
                        it.copy(
                            selectedServerStation = outcome.station,
                            serverDetailFromCache = false,
                            serverError = null,
                        )
                    }
                is ServerStationDetailOutcome.StaleCache ->
                    _uiState.update {
                        it.copy(
                            selectedServerStation = outcome.station,
                            serverDetailFromCache = true,
                            serverError = null,
                        )
                    }
                is ServerStationDetailOutcome.Unavailable ->
                    _uiState.update {
                        it.copy(
                            selectedServerStation = null,
                            serverError = outcome.cause.message
                                ?: outcome.cause.javaClass.simpleName,
                        )
                    }
            }
        }
    }

    fun onServerDetailDismissed() {
        _uiState.update { it.copy(selectedServerStation = null, serverDetailFromCache = false) }
    }

    /**
     * P35-T02 — routes to a canonical server station by reviewed
     * coordinates. Unknown-location stations (no coordinates) cannot
     * navigate and emit no effect, never a fabricated destination.
     */
    fun onServerStationNavigate(station: ServerStation) {
        val lat = station.latitude
        val lon = station.longitude
        if (lat == null || lon == null) return
        viewModelScope.launch {
            _navigationEffects.emit(
                StationsNavigationEffect.LaunchMaps("$lat,$lon"),
            )
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
            lastLocale = locale
            // P20-T02: keep the last good list visible while reloading so a
            // failed refresh still shows cached data instead of an empty error.
            _uiState.update {
                it.copy(
                    isLoading = true,
                    error = null,
                    errorMessage = null,
                    showDownloadPrompt = false,
                    showEmpty = false,
                    showNoLocation = false,
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
                        lastStations = outcome.stations
                        _uiState.update {
                            it.copy(
                                isLoading = false,
                                municipality = outcome.municipality,
                                state = outcome.state,
                                surveyWeek = outcome.surveyWeek,
                                selectedFuelProduct = outcome.fuelProduct,
                                showEmpty = outcome.isEmpty,
                            )
                        }
                        renderStations(_uiState.value.searchQuery)
                    }
                }
            }.onFailure { error ->
                // P20-T02: failed refresh keeps cached stations; the error is
                // shown above the list instead of replacing it.
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

    private fun renderStations(query: String) {
        val locale = lastLocale ?: return
        val state = _uiState.value
        val filtered = com.anpfuel.domain.discovery.DiscoveryStationsRule.filterAndSort(
            stations = lastStations,
            search = query,
            sort = com.anpfuel.domain.discovery.DiscoverySort.PRICE_ASC,
        )
        val trimmed = query.trim()
        val noMatch = trimmed.length >=
            com.anpfuel.domain.rule.MinimumSearchLengthRule.MIN_LENGTH &&
            filtered.isEmpty()
        _uiState.update {
            it.copy(
                searchQuery = query,
                searchNoMatch = if (noMatch) trimmed else null,
                stations = StationPriceUiMapper.toUiModels(
                    stations = filtered,
                    locale = locale,
                    preferredState = state.state,
                    preferredMunicipality = state.municipality,
                ),
            )
        }
    }

    companion object {
        private const val ARG_FUEL_PRODUCT = "fuelProduct"
        private const val ARG_SEARCH_QUERY = "discoverySearch"
    }
}
