package com.anpfuel.app.ui.stations

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.error.AppError
import com.anpfuel.application.error.AppErrorResolver
import com.anpfuel.application.usecase.location.SelectLocationUseCase
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.price.GetStationPricesUseCase
import com.anpfuel.application.usecase.price.StationPricesPageOutcome
import com.anpfuel.app.location.LocationPermissionHandler
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.directory.GetNearbyServerStationsUseCase
import com.anpfuel.application.usecase.directory.GetServerStationDetailUseCase
import com.anpfuel.application.usecase.directory.GetServerStationsUseCase
import com.anpfuel.application.usecase.directory.NearbyServerStationsOutcome
import com.anpfuel.application.usecase.directory.ServerStationDetailOutcome
import com.anpfuel.application.usecase.directory.ServerStationsOutcome
import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.application.usecase.station.BuildStationNavigationQueryUseCase
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
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.filterNotNull
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive

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
    /**
     * ANP list continuation. The visible rows come from bounded local
     * queries; a failed continuation never wipes them and retries from
     * the same offset.
     */
    val anpHasMore: Boolean = false,
    val isAnpLoadingMore: Boolean = false,
    val anpMoreError: String? = null,
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
    /**
     * Directory continuation state. The next cursor belongs to the last
     * stored page; a null cursor ends traversal. A failed continuation
     * never wipes displayed stations: the error lands in
     * [serverMoreError] with a retry that resumes the same cursor.
     */
    val serverNextCursor: String? = null,
    val isServerLoadingMore: Boolean = false,
    val serverMoreError: String? = null,
    val selectedServerStation: ServerStation? = null,
    val serverDetailFromCache: Boolean = false,
    /**
     * P35-T03 — bounded nearby lookup (explicit permission, transient
     * GPS, never cached) and its loading state.
     */
    val nearbyStations: List<NearbyServerStation> = emptyList(),
    val isNearbyLoading: Boolean = false,
    /**
     * P36-T02 — caller-held account id for the canonical discussion
     * target. Blank is an honest guest (reads only); session binding is
     * owned by P36-T03.
     */
    val serverAccountId: String = "",
) {
    val directoryOnlyStations: List<ServerStation>
        get() = com.anpfuel.domain.discovery.StationDirectoryVisibilityRule.additionalStations(
            serverStations,
            if (!showNoLocation && !showDownloadPrompt && searchNoMatch == null && !showEmpty) {
                stations.map { it.cnpjDigits }
            } else {
                emptyList()
            },
        )
}

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
    private val getNearbyServerStationsUseCase: GetNearbyServerStationsUseCase? = null,
    /**
     * P36-T03 — secure session source for the discussion account id.
     * Null keeps legacy construction working; a missing/foreign blob
     * resolves to honest guest ("") and never to another account.
     */
    private val sessionStore: AuthSessionStore? = null,
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

    /**
     * Next ANP page index for continuation; reset on fuel, search or
     * reload. Displayed rows accumulate per loaded page and are replaced
     * only by a fresh first page, never wiped by a failed continuation.
     */
    private var anpNextPage = 0
    private var lastLocale: Locale? = null

    private val connectivity = MutableStateFlow<Boolean?>(null)

    init {
        viewModelScope.launch {
            observeNetworkConnectivityUseCase().collect { isConnected ->
                _uiState.update { it.copy(isOffline = !isConnected) }
                connectivity.value = isConnected
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
        // Explicit filter change: drop the previous fuel's rows so stale
        // prices are never shown under the new fuel chip.
        anpNextPage = 0
        _uiState.update {
            it.copy(
                selectedFuelProduct = fuelProduct,
                stations = emptyList(),
                searchNoMatch = null,
                selectedDetail = null,
                anpHasMore = false,
                anpMoreError = null,
            )
        }
        loadForFuel(fuelProduct, locale)
    }

    /**
     * P20-T02 — local station-name search answered by a bounded query.
     * Each keystroke restarts from the first page; filter state survives
     * rotation via [SavedStateHandle].
     */
    fun onSearchQueryChanged(query: String, locale: Locale) {
        lastLocale = locale
        savedStateHandleRef[ARG_SEARCH_QUERY] = query
        anpNextPage = 0
        anpMoreJob?.cancel()
        _uiState.update {
            it.copy(
                searchQuery = query,
                anpHasMore = false,
                anpMoreError = null,
            )
        }
        viewModelScope.launch {
            loadAnpPage(_uiState.value.selectedFuelProduct, query, locale, append = false)
        }
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
     * P20-T03 — opens the station detail sheet from a targeted local
     * read. Unknown CNPJ (or no survey week yet) yields no detail, never
     * an invented card.
     */
    fun onStationSelected(cnpjDigits: String) {
        val locale = lastLocale ?: return
        val state = _uiState.value
        val week = state.surveyWeek
        val fuel = state.selectedFuelProduct
        val locationState = state.state
        val municipality = state.municipality
        if (week == null || locationState == null || municipality == null) {
            _uiState.update { it.copy(selectedDetail = null) }
            return
        }
        viewModelScope.launch {
            val domain = getStationPricesUseCase.findStationPrice(
                state = locationState,
                municipality = municipality,
                fuelProduct = fuel,
                surveyWeek = week,
                cnpjDigits = cnpjDigits,
            )
            currentCoroutineContext().ensureActive()
            if (domain == null) {
                _uiState.update { it.copy(selectedDetail = null) }
                return@launch
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
     *
     * P35-T03 — the in-flight load is cancellable: a restart/refresh
     * cancels the previous request so only the latest result lands.
     */
    private var serverLoadJob: kotlinx.coroutines.Job? = null

    /** Called inside the screen's resumed lifecycle; cancellation stops polling and pending reads. */
    suspend fun syncServerStationsWhileVisible() {
        try {
            connectivity.filterNotNull().collectLatest { connected ->
                if (!connected) {
                    serverDetailJob?.cancel()
                    _uiState.update {
                        it.copy(
                            serverFromCache = it.serverStations.isNotEmpty(),
                            serverDetailFromCache = it.selectedServerStation != null,
                        )
                    }
                    loadServerStations(cachedOnly = true)
                    serverLoadJob?.join()
                } else {
                    while (true) {
                        if (serverLoadJob?.isActive != true) refreshServerStations()
                        serverLoadJob?.join()
                        serverDetailJob?.join()
                        delay(SERVER_REFRESH_INTERVAL_MILLIS)
                    }
                }
            }
        } finally {
            serverLoadJob?.cancel()
            serverDetailJob?.cancel()
            serverMoreJob?.cancel()
            _uiState.update { it.copy(isServerLoading = false, isServerLoadingMore = false) }
        }
    }

    fun loadServerStations(limit: Int = 20, cachedOnly: Boolean = false) {
        val useCase = getServerStationsUseCase ?: return
        serverLoadJob?.cancel()
        serverMoreJob?.cancel()
        serverLoadJob = viewModelScope.launch {
            _uiState.update {
                it.copy(
                    isServerLoading = true,
                    isServerLoadingMore = false,
                    serverError = null,
                    serverMoreError = null,
                )
            }
            val outcome = if (cachedOnly) useCase.cached() else useCase(limit, null)
            currentCoroutineContext().ensureActive()
            when (outcome) {
                is ServerStationsOutcome.Disabled ->
                    _uiState.update { it.copy(isServerLoading = false) }
                is ServerStationsOutcome.Fresh ->
                    _uiState.update {
                        it.copy(
                            isServerLoading = false,
                            serverStations = outcome.page.items,
                            serverNextCursor = outcome.page.nextCursor,
                            serverFromCache = false,
                            serverError = null,
                        )
                    }
                is ServerStationsOutcome.StaleCache ->
                    _uiState.update {
                        it.copy(
                            isServerLoading = false,
                            serverStations = outcome.page.items,
                            serverNextCursor = outcome.page.nextCursor,
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
     * P35-T03 — refreshes the server page without losing the selected
     * detail when it is still present; a failed refresh keeps cached
     * stations exactly like the legacy P20-T02 recovery.
     */
    fun refreshServerStations(limit: Int = 20) {
        val selected = _uiState.value.selectedServerStation?.stationId
        loadServerStations(limit)
        if (selected != null) {
            onServerStationSelected(selected)
        }
    }

    /**
     * Appends the next directory page to the displayed list. A fresh
     * load supersedes a pending continuation; concurrent taps do not
     * start parallel continuations. Displayed stations are never wiped
     * here: failures only set [StationsUiState.serverMoreError] for
     * retry, and an expired cursor restarts from the first page through
     * the use case.
     */
    private var serverMoreJob: kotlinx.coroutines.Job? = null

    fun loadMoreServerStations() {
        val useCase = getServerStationsUseCase ?: return
        if (_uiState.value.isServerLoadingMore || _uiState.value.serverNextCursor == null) return
        if (serverMoreJob?.isActive == true) return
        serverMoreJob = viewModelScope.launch {
            _uiState.update { it.copy(isServerLoadingMore = true, serverMoreError = null) }
            val outcome = useCase.loadMore()
            currentCoroutineContext().ensureActive()
            when (outcome) {
                is ServerStationsOutcome.Disabled ->
                    _uiState.update { it.copy(isServerLoadingMore = false) }
                is ServerStationsOutcome.Fresh ->
                    _uiState.update {
                        it.copy(
                            isServerLoadingMore = false,
                            serverStations = outcome.page.items,
                            serverNextCursor = outcome.page.nextCursor,
                            serverFromCache = false,
                            serverMoreError = null,
                        )
                    }
                is ServerStationsOutcome.StaleCache ->
                    _uiState.update {
                        it.copy(
                            isServerLoadingMore = false,
                            serverStations = outcome.page.items,
                            serverNextCursor = outcome.page.nextCursor,
                            serverFromCache = true,
                            serverMoreError = outcome.cause.message
                                ?: outcome.cause.javaClass.simpleName,
                        )
                    }
                is ServerStationsOutcome.Unavailable ->
                    _uiState.update {
                        it.copy(
                            isServerLoadingMore = false,
                            serverMoreError = outcome.cause.message
                                ?: outcome.cause.javaClass.simpleName,
                        )
                    }
            }
        }
    }

    /** Test seam: cancels the in-flight continuation, mirroring screen stops. */
    internal fun cancelServerMoreForTest() {
        serverMoreJob?.cancel()
    }

    /**
     * P35-T02 — selects a canonical server station by UUID for the
     * community-first detail. Unknown UUID (or disabled reads) yields no
     * detail, never an invented card. Stale cache is labelled explicitly.
     */
    private var serverDetailJob: kotlinx.coroutines.Job? = null

    fun onServerStationSelected(stationId: String) {
        val useCase = getServerStationDetailUseCase ?: return
        // P36-T03 — bind the caller-held account id once per selection so
        // discussion writes authenticate as the session owner; guests stay
        // blank (reads only). Store failures fail closed to guest.
        val accountId = runCatching { sessionStore?.load()?.accountId.orEmpty() }.getOrDefault("")
        _uiState.update { it.copy(serverAccountId = accountId) }
        serverDetailJob?.cancel()
        serverDetailJob = viewModelScope.launch {
            val outcome = useCase(stationId)
            currentCoroutineContext().ensureActive()
            when (outcome) {
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
        serverDetailJob?.cancel()
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

    /**
     * P35-T03 — bounded nearby lookup with explicit permission.
     * GPS denied without a prior grant requests permission and loads
     * nothing; manual city browsing keeps working. A granted fix runs
     * the transient lookup (never cached); no fix or transport failure
     * reports an honest error without stale positions.
     *
     * A new lookup cancels the in-flight one so only the latest result
     * lands, mirroring the server page/detail jobs.
     */
    private var nearbyJob: kotlinx.coroutines.Job? = null

    fun loadNearbyServerStations(radiusMeters: Int = 2000, limit: Int = 20) {
        val useCase = getNearbyServerStationsUseCase ?: return
        if (!locationPermissionHandler.hasLocationPermission()) {
            _locationPermissionRequest.tryEmit(Unit)
            return
        }
        nearbyJob?.cancel()
        nearbyJob = viewModelScope.launch {
            val fix = locationPermissionHandler.getCurrentLocation()
            if (fix == null) {
                _uiState.update {
                    it.copy(
                        isNearbyLoading = false,
                        serverError = "location unavailable",
                    )
                }
                return@launch
            }
            _uiState.update { it.copy(isNearbyLoading = true, serverError = null) }
            when (val outcome = useCase(fix.latitude, fix.longitude, radiusMeters, limit)) {
                is NearbyServerStationsOutcome.Disabled ->
                    _uiState.update { it.copy(isNearbyLoading = false) }
                is NearbyServerStationsOutcome.Fresh -> {
                    currentCoroutineContext().ensureActive()
                    _uiState.update {
                        it.copy(
                            isNearbyLoading = false,
                            nearbyStations = outcome.stations,
                            serverError = null,
                        )
                    }
                }
                is NearbyServerStationsOutcome.Unavailable ->
                    _uiState.update {
                        it.copy(
                            isNearbyLoading = false,
                            serverError = outcome.cause.message
                                ?: outcome.cause.javaClass.simpleName,
                        )
                    }
            }
        }
    }

    /** Test seam: cancels the in-flight nearby lookup, mirroring screen stops. */
    internal fun cancelNearbyForTest() {
        nearbyJob?.cancel()
    }

    fun onNavigateToStation(cnpjDigits: String) {
        val state = _uiState.value
        val week = state.surveyWeek
        val locationState = state.state
        val municipality = state.municipality
        if (week == null || locationState == null || municipality == null) return

        viewModelScope.launch {
            val domain = getStationPricesUseCase.findStationPrice(
                state = locationState,
                municipality = municipality,
                fuelProduct = state.selectedFuelProduct,
                surveyWeek = week,
                cnpjDigits = cnpjDigits,
            ) ?: return@launch
            currentCoroutineContext().ensureActive()
            val result = buildStationNavigationQueryUseCase(domain.station)
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
                    anpHasMore = false,
                    isAnpLoadingMore = false,
                    anpMoreError = null,
                )
            }

            anpNextPage = 0
            anpMoreJob?.cancel()
            loadAnpPage(fuelProduct, _uiState.value.searchQuery, locale, append = false)
            _uiState.update { it.copy(isLoading = false) }
        }
    }

    /**
     * Bounded ANP discovery window. Filtering, ordering and pagination
     * run in the local query; the full-list read stays reserved for
     * cheapest/nearest computation over the complete eligible set.
     * Displayed rows are replaced by a fresh first page and extended by
     * continuations; a failed read never wipes them.
     */
    private var anpMoreJob: kotlinx.coroutines.Job? = null

    private suspend fun loadAnpPage(
        fuelProduct: FuelProduct,
        query: String,
        locale: Locale,
        append: Boolean,
    ) {
        val pageIndex = if (append) anpNextPage else 0
        if (append) {
            _uiState.update { it.copy(isAnpLoadingMore = true, anpMoreError = null) }
        }
        runCatching {
            val preferred = selectLocationUseCase.getPreferredLocation()
            if (preferred == null) {
                _uiState.update { it.copy(showNoLocation = true) }
                return
            }

            when (
                val outcome = getStationPricesUseCase.page(
                    fuelProduct = fuelProduct,
                    search = query,
                    page = pageIndex,
                )
            ) {
                is StationPricesPageOutcome.StationDetailMissing -> {
                    _uiState.update {
                        it.copy(
                            showDownloadPrompt = true,
                            municipality = preferred.municipality,
                            state = preferred.state,
                            selectedFuelProduct = fuelProduct,
                        )
                    }
                }

                is StationPricesPageOutcome.Success -> {
                    currentCoroutineContext().ensureActive()
                    anpNextPage = pageIndex + 1
                    val mapped = StationPriceUiMapper.toUiModels(
                        stations = outcome.page.items,
                        locale = locale,
                        preferredState = outcome.state,
                        preferredMunicipality = outcome.municipality,
                    )
                    val trimmed = query.trim()
                    val noMatch = trimmed.length >=
                        com.anpfuel.domain.rule.MinimumSearchLengthRule.MIN_LENGTH &&
                        outcome.page.totalCount == 0
                    _uiState.update {
                        it.copy(
                            municipality = outcome.municipality,
                            state = outcome.state,
                            surveyWeek = outcome.surveyWeek,
                            selectedFuelProduct = outcome.fuelProduct,
                            searchQuery = query,
                            searchNoMatch = if (noMatch) trimmed else null,
                            showEmpty = outcome.page.totalCount == 0 && !noMatch,
                            stations = if (append) it.stations + mapped else mapped,
                            anpHasMore = outcome.page.hasMore,
                            isAnpLoadingMore = false,
                            anpMoreError = null,
                        )
                    }
                }
            }
        }.onFailure { error ->
            if (append) {
                // A failed continuation keeps every displayed row; retry
                // resumes from the same offset.
                _uiState.update {
                    it.copy(
                        isAnpLoadingMore = false,
                        anpMoreError = error.message ?: error.javaClass.simpleName,
                    )
                }
            } else {
                // P20-T02: failed refresh keeps cached stations; the error
                // is shown above the list instead of replacing it.
                _uiState.update {
                    it.copy(
                        error = AppErrorResolver.fromThrowable(error),
                        errorMessage = error.message ?: error.javaClass.simpleName,
                    )
                }
            }
        }
    }

    /**
     * Appends the next ANP window to the displayed list. A reload, fuel
     * change or new search supersedes a pending continuation; concurrent
     * taps do not start parallel continuations.
     */
    fun loadMoreAnpStations() {
        if (_uiState.value.isAnpLoadingMore || !_uiState.value.anpHasMore) return
        if (anpMoreJob?.isActive == true) return
        val locale = lastLocale ?: return
        anpMoreJob = viewModelScope.launch {
            loadAnpPage(
                fuelProduct = _uiState.value.selectedFuelProduct,
                query = _uiState.value.searchQuery,
                locale = locale,
                append = true,
            )
        }
    }

    /** Test seam: cancels the in-flight ANP continuation, mirroring screen stops. */
    internal fun cancelAnpMoreForTest() {
        anpMoreJob?.cancel()
    }

    companion object {
        internal const val SERVER_REFRESH_INTERVAL_MILLIS = 60_000L
        private const val ARG_FUEL_PRODUCT = "fuelProduct"
        private const val ARG_SEARCH_QUERY = "discoverySearch"
    }
}
