package com.anpfuel.app.ui.map

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.discovery.bestPinnedStation
import com.anpfuel.domain.map.MapRegionPoint
import com.anpfuel.domain.map.chooseCityRegionPoint
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.ServerStationCache
import com.anpfuel.domain.repository.ServerStationGateway
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.maps.engine.MapCamera
import com.anpfuel.maps.engine.MapCameraStore
import com.anpfuel.maps.engine.MapEngine
import com.anpfuel.maps.engine.MapOverlayCluster
import com.anpfuel.maps.engine.MapOverlayLabels
import com.anpfuel.maps.engine.MapOverlayListItem
import com.anpfuel.maps.engine.MapOverlayMarker
import com.anpfuel.maps.engine.MapOverlaySelection
import com.anpfuel.maps.engine.MapStationLocationQuality
import com.anpfuel.maps.engine.MapStationType
import com.anpfuel.maps.engine.overlayListItems
import com.anpfuel.maps.engine.prepareOverlay
import dagger.hilt.android.lifecycle.HiltViewModel
import java.util.Locale
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class MapCityOption(
    val ibgeCode: String,
    val name: String,
    val state: String,
    val latitude: Double,
    val longitude: Double,
)

val DEFAULT_MAP_CITIES: List<MapCityOption> = listOf(
    MapCityOption("4106902", "Curitiba", "PR", -25.4284, -49.2733),
    MapCityOption("3550308", "São Paulo", "SP", -23.5505, -46.6333),
    MapCityOption("3304557", "Rio de Janeiro", "RJ", -22.9068, -43.1729),
    MapCityOption("5300108", "Brasília", "DF", -15.7939, -47.8828),
    MapCityOption("3106200", "Belo Horizonte", "MG", -19.9167, -43.9345),
    MapCityOption("4314902", "Porto Alegre", "RS", -30.0346, -51.2177),
    MapCityOption("2927408", "Salvador", "BA", -12.9777, -38.5016),
    MapCityOption("2611606", "Recife", "PE", -8.0476, -34.8770),
    MapCityOption("2304400", "Fortaleza", "CE", -3.7172, -38.5433),
    MapCityOption("1302603", "Manaus", "AM", -3.1190, -60.0217),
)

data class MapStationSearchResults(
    val items: List<MapOverlayListItem>,
    val byUuid: Map<String, MapOverlayMarker>,
    val offlineCached: Boolean,
)

data class MapDiscoveryUi(
    val clusters: List<MapOverlayCluster> = emptyList(),
    val listItems: List<MapOverlayListItem> = emptyList(),
    val selectableByUuid: Map<String, MapOverlayMarker> = emptyMap(),
    val selectedUuid: String? = null,
    val selectedMarker: MapOverlayMarker? = null,
    val expandedClusterId: String? = null,
    val selectedMembers: List<MapOverlayMarker> = emptyList(),
    val loading: Boolean = false,
    val error: String? = null,
    val generation: Long = 0,
    val showFuel: Boolean = true,
    val showCharging: Boolean = true,
    val fuelProduct: FuelProduct? = null,
    val connector: ChargingConnector? = null,
    val current: ChargingCurrent? = null,
    val access: ChargingAccess? = null,
    val minPowerKw: Int? = null,
    val surveyPeriod: String? = null,
    val searchQuery: String = "",
    val searchResults: MapStationSearchResults? = null,
    val searchFocus: MapOverlayMarker? = null,
    val searching: Boolean = false,
    val searchFailed: Boolean = false,
    val municipalityCode: String = "4106902",
    val municipalityName: String = "Curitiba",
    val availableCities: List<MapCityOption> = DEFAULT_MAP_CITIES,
    val viewportLatitude: Double = -25.42,
    val viewportLongitude: Double = -49.27,
    val viewportRadiusMeters: Int = 10_000,
    val chargingSnapshotPresent: Boolean = true,
    val chargingError: String? = null,
    val cityLocationUnknown: Boolean = false,
    val offlineCached: Boolean = false,
    /**
     * Coordinate that represents the opened city for the offline-map region
     * (see [chooseCityRegionPoint]); null while unknown. It follows the city,
     * not later panning.
     */
    val cityLatitude: Double? = -25.4284,
    val cityLongitude: Double? = -49.2733,
)

/**
 * MAP-18 viewport discovery + MAP-20 search/filters: bounded nearby/search
 * queries with debounce and stale discard. Canonical UUIDs are the stable
 * identity; proximity never merges records. Unknown/city-centroid and
 * coordinate-less stations stay list-only via `prepareOverlay` and remain
 * selectable by UUID (`selectableByUuid`). Unknown metadata is shown, never excluded.
 */
@HiltViewModel
class MapDiscoveryViewModel @Inject constructor(
    private val stations: ServerStationGateway,
    private val charging: ChargingMarkersSource,
    private val engine: MapEngine,
    private val userPreferences: UserPreferencesRepository?,
    private val catalog: MunicipalityCatalogRepository?,
    private val stationCache: ServerStationCache = NoopServerStationCache,
    private val cameraStore: MapCameraStore? = null,
) : ViewModel() {

    constructor(
        stations: ServerStationGateway,
        charging: ChargingMarkersSource,
        engine: MapEngine,
    ) : this(
        stations = stations,
        charging = charging,
        engine = engine,
        userPreferences = null,
        catalog = null,
        stationCache = NoopServerStationCache,
    )

    private val _ui = MutableStateFlow(MapDiscoveryUi())
    val ui: StateFlow<MapDiscoveryUi> = _ui.asStateFlow()

    private var fetchJob: Job? = null
    private var generation = 0L
    private var searchJob: Job? = null
    private var searchGeneration = 0L
    private var cityJob: Job? = null
    private var cityGeneration = 0L
    private var lastMarkers: List<MapOverlayMarker> = emptyList()
    private var lastNullStations: List<ServerStation> = emptyList()
    private var lastNullCharging: List<ChargingPlace> = emptyList()
    private var overlayLabels: MapOverlayLabels = MapOverlayLabels()
    private var lastPreferredMunicipalityCode: String? = null
    /**
     * City the camera was last explicitly moved to. Resume, recomposition,
     * refresh and marker reloads must never move the camera: only a genuine
     * city change (user picks a new city, or the first map open) recenters it.
     * The recenter button still centers on the user on demand.
     */
    private var lastViewportCityCode: String? = null

    init {
        engine.setOnOverlaySelected(::onNativeSelection)
        viewModelScope.launch {
            // Only pins have a map point; list-only stations clear the highlight.
            _ui.map { ui -> (ui.selectedMarker ?: ui.searchFocus)?.takeIf { it.canRouteByCoordinates } }
                .distinctUntilChanged()
                .collect(engine::setHighlightedMarker)
        }
        if (userPreferences != null && catalog != null) {
            viewModelScope.launch {
                loadPreferredMunicipality()
            }
        }
    }

    fun refreshPreferredMunicipality() {
        if (userPreferences != null && catalog != null) {
            viewModelScope.launch { loadPreferredMunicipality() }
        }
    }

    private suspend fun loadPreferredMunicipality() {
        val prefs = try {
            userPreferences?.getPreferences()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            null
        } ?: return
        val state = prefs.preferredState ?: return
        val cityName = prefs.preferredMunicipality?.takeIf { it.isNotBlank() } ?: return
        val entry = try {
            catalog?.findCatalogEntry(state, cityName)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            null
        }
        val code = entry?.ibgeCode
            ?: DEFAULT_MAP_CITIES.firstOrNull {
                it.name.equals(cityName, ignoreCase = true) && it.state.equals(state.abbreviation, ignoreCase = true)
            }?.ibgeCode
            ?: return
        if (lastPreferredMunicipalityCode == code) return
        lastPreferredMunicipalityCode = code
        onSelectCity(code, cityName)
    }

    fun setOverlayLabels(labels: MapOverlayLabels) {
        if (overlayLabels == labels) return
        overlayLabels = labels
        if (lastMarkers.isNotEmpty() || lastNullStations.isNotEmpty() || lastNullCharging.isNotEmpty()) {
            pushMarkers(
                markers = lastMarkers,
                gen = _ui.value.generation,
                nullCoordsStations = lastNullStations,
                nullCoordsCharging = lastNullCharging,
                preserveLast = false,
            )
        }
    }

    fun onSelectCity(ibgeCode: String, cityName: String = "", moveViewport: Boolean = true) {
        val previousCode = _ui.value.municipalityCode
        val normalizedCode = ibgeCode.trim().ifBlank { previousCode }
        val knownCity = DEFAULT_MAP_CITIES.firstOrNull { it.ibgeCode == normalizedCode }
        val resolvedName = cityName.trim().ifBlank { knownCity?.name ?: normalizedCode }
        // The camera moves only when the city actually changes: a repeated
        // selection, resume or refresh keeps the user's current framing.
        val shouldMoveViewport = moveViewport && normalizedCode != lastViewportCityCode
        if (shouldMoveViewport) {
            lastViewportCityCode = normalizedCode
        }
        // A built-in city is where the camera is centered below; other cities
        // resolve from their loaded stations in loadCityStations.
        val cityPoint = chooseCityRegionPoint(null, emptyList(), builtInCityPoint(normalizedCode))
        _ui.update {
            it.copy(
                municipalityCode = normalizedCode,
                municipalityName = resolvedName,
                cityLatitude = cityPoint?.latitude,
                cityLongitude = cityPoint?.longitude,
            )
        }
        if (shouldMoveViewport && knownCity != null) {
            moveCameraToCity(knownCity.latitude, knownCity.longitude)
            onViewport(knownCity.latitude, knownCity.longitude, _ui.value.viewportRadiusMeters)
        } else if (knownCity == null && moveViewport) {
            // Directory city: (re)load its stations, recentering only when the
            // city actually changed. Same-city reselect/refresh keeps the camera.
            loadCityStations(normalizedCode, moveViewport = shouldMoveViewport)
        }
        if (normalizedCode != previousCode) {
            val query = _ui.value.searchQuery
            clearSearch()
            if (query.isNotBlank()) onSearch(normalizedCode, query)
        }
    }

    /**
     * The chosen city is also the camera the engine restores when it becomes
     * ready: on a fresh start the city often resolves before native init ends,
     * and readiness would otherwise reset the camera to the stored/default one.
     */
    private fun moveCameraToCity(latitude: Double, longitude: Double) {
        cameraStore?.save(MapCamera(latitude, longitude, 13))
        engine.setViewport(latitude, longitude, 13)
    }

    private fun loadCityStations(code: String, moveViewport: Boolean) {
        // The city load owns its own job: a nearby refresh (map readiness
        // reporting the default camera, native camera sync) may replace the
        // pins, but must never cancel the city camera and region point.
        fetchJob?.cancel()
        cityJob?.cancel()
        val gen = ++generation
        val cityGen = ++cityGeneration
        _ui.update { it.copy(loading = true, error = null, generation = gen) }
        cityJob = viewModelScope.launch {
            try {
                val cityStations = linkedMapOf<String, ServerStation>()
                val cursors = mutableSetOf<String>()
                var cursor: String? = null
                do {
                    val page = stations.listInMunicipality(code, 100, cursor)
                    if (cityGen != cityGeneration) return@launch
                    page.items.forEach { cityStations[it.stationId] = it }
                    cursor = page.nextCursor
                    check(cursor == null || (cursors.size < 24 && cursors.add(cursor))) {
                        "Municipal station pages exceeded the cursor budget"
                    }
                } while (cursor != null)
                val items = cityStations.values.toList()
                // Center on the best pin tier (reviewed > osm-mapped > anp-reported > address-point).
                val pinned = items.bestPinnedStation()
                val cityPoint = chooseCityRegionPoint(
                    pinned?.let { MapRegionPoint(checkNotNull(it.latitude), checkNotNull(it.longitude)) },
                    items.stationPoints(),
                    builtInCityPoint(code),
                )
                _ui.update {
                    it.copy(
                        cityLocationUnknown = pinned == null,
                        offlineCached = false,
                        cityLatitude = cityPoint?.latitude,
                        cityLongitude = cityPoint?.longitude,
                    )
                }
                saveMapCache(code, items)
                if (gen == generation) {
                    pushMarkers(
                        items.mapNotNull { it.toMarker() }, gen,
                        items.filter { it.latitude == null || it.longitude == null },
                    )
                }
                if (pinned != null && moveViewport && _ui.value.searchFocus == null) {
                    moveCameraToCity(checkNotNull(pinned.latitude), checkNotNull(pinned.longitude))
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (cityGen != cityGeneration) return@launch
                val cached = loadMapCache(code)
                val cityPoint = chooseCityRegionPoint(null, cached.stationPoints(), builtInCityPoint(code))
                if (cached.isNotEmpty()) {
                    _ui.update { it.copy(cityLatitude = cityPoint?.latitude, cityLongitude = cityPoint?.longitude) }
                }
                // Pins and loading state belong to the newest fetch only.
                if (gen != generation) return@launch
                if (cached.isNotEmpty()) {
                    pushMarkers(
                        cached.mapNotNull { it.toMarker() }, gen,
                        cached.filter { it.latitude == null || it.longitude == null },
                    )
                    _ui.update { it.copy(loading = false, error = null, offlineCached = true) }
                } else {
                    _ui.update { it.copy(loading = false, error = e.message ?: "city-unavailable") }
                }
            }
        }
    }

    private fun builtInCityPoint(code: String): MapRegionPoint? =
        DEFAULT_MAP_CITIES.firstOrNull { it.ibgeCode == code }?.let { MapRegionPoint(it.latitude, it.longitude) }

    private fun List<ServerStation>.stationPoints(): List<MapRegionPoint> = mapNotNull { station ->
        val lat = station.latitude ?: return@mapNotNull null
        val lon = station.longitude ?: return@mapNotNull null
        MapRegionPoint(lat, lon)
    }

    fun onViewport(latitude: Double, longitude: Double, radiusMeters: Int = 10_000) {
        if (!latitude.isFinite() || !longitude.isFinite()) return
        val boundedRadius = radiusMeters.coerceIn(100, 15_000)
        // B-BR-MAP-DISCOVERY-01: Cancel in-flight job and advance generation BEFORE debounce.
        fetchJob?.cancel()
        val gen = ++generation
        _ui.update {
            it.copy(
                loading = true,
                error = null,
                generation = gen,
                cityLocationUnknown = false,
                viewportLatitude = latitude,
                viewportLongitude = longitude,
                viewportRadiusMeters = boundedRadius,
            )
        }
        fetchJob = viewModelScope.launch {
            delay(300)
            if (gen != generation) return@launch
            try {
                val nearby: List<NearbyServerStation> = stations.nearby(
                    latitude,
                    longitude,
                    boundedRadius,
                    100,
                )
                if (gen != generation) return@launch
                val fuelMarkers = nearby.mapNotNull { it.station.toMarker() }
                saveMapCache(_ui.value.municipalityCode, nearby.map { it.station })
                // Stations without coordinates stay list-only with explicit warning.
                val nullCoords = nearby.map { it.station }.filter { it.latitude == null || it.longitude == null }
                val uiSnapshot = _ui.value
                val chargingResult = try {
                    charging.query(
                        ChargingMarkersQuery(
                            latitude = latitude,
                            longitude = longitude,
                            radiusMeters = boundedRadius,
                            connector = uiSnapshot.connector,
                            current = uiSnapshot.current,
                            access = uiSnapshot.access,
                            minPowerKw = uiSnapshot.minPowerKw,
                            limit = 100,
                        )
                    )
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    ChargingMarkersResult(
                        places = emptyList(),
                        snapshotPresent = false,
                        error = e.message ?: "charging-snapshot-failed",
                    )
                }
                if (gen != generation) return@launch
                val chargeMarkers = chargingResult.places.mapNotNull { it.toMarker() }
                val nullCharging = chargingResult.places.filter {
                    it.latitude?.toDoubleOrNull() == null || it.longitude?.toDoubleOrNull() == null
                }
                _ui.update {
                    it.copy(
                        chargingSnapshotPresent = chargingResult.snapshotPresent,
                        chargingError = chargingResult.error,
                    )
                }
                pushMarkers(
                    markers = fuelMarkers + chargeMarkers,
                    gen = gen,
                    nullCoordsStations = nullCoords,
                    nullCoordsCharging = nullCharging,
                )
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (gen != generation) return@launch
                val cached = loadMapCache(_ui.value.municipalityCode)
                if (cached.isNotEmpty()) {
                    pushMarkers(
                        cached.mapNotNull { it.toMarker() }, gen,
                        cached.filter { it.latitude == null || it.longitude == null },
                    )
                    _ui.update { it.copy(loading = false, error = null, offlineCached = true) }
                } else {
                    _ui.update { it.copy(loading = false, error = e.message ?: "discovery-failed") }
                }
            }
        }
    }

    /** Station results and viewport overlays have independent cancellation and state. */
    fun onSearch(
        municipalityCode: String,
        query: String,
        limit: Int = 50,
        onResults: ((List<MapOverlayMarker>) -> Unit)? = null,
    ) {
        val text = query.trim()
        clearSearch()
        if (text.length < 2) return
        val code = municipalityCode.trim().ifBlank {
            DEFAULT_MAP_CITIES.firstOrNull { it.name.equals(text, ignoreCase = true) || it.ibgeCode == text }
                ?.ibgeCode ?: _ui.value.municipalityCode
        }
        loadSearch(code, text, limit.coerceIn(1, 100), moveToMatch = true, onResults)
    }

    fun clearSearch() {
        searchJob?.cancel()
        ++searchGeneration
        if (_ui.value.selectedUuid in _ui.value.searchResults?.byUuid.orEmpty()) clearSelection()
        _ui.update {
            it.copy(searchQuery = "", searchResults = null, searchFocus = null, searching = false, searchFailed = false)
        }
    }

    private fun refreshSearch() {
        val state = _ui.value
        if (state.searchQuery.isNotBlank()) {
            loadSearch(state.municipalityCode, state.searchQuery, 50, moveToMatch = false)
        }
    }

    private fun loadSearch(
        code: String,
        query: String,
        limit: Int,
        moveToMatch: Boolean,
        onResults: ((List<MapOverlayMarker>) -> Unit)? = null,
    ) {
        searchJob?.cancel()
        val token = ++searchGeneration
        _ui.update { it.copy(searchQuery = query, searching = true, searchFailed = false) }
        searchJob = viewModelScope.launch {
            delay(300)
            try {
                val page = stations.search(code, query, limit, null)
                if (token != searchGeneration) return@launch
                val filters = _ui.value
                val places = try {
                    charging.query(
                        ChargingMarkersQuery(
                            searchQuery = query,
                            connector = filters.connector,
                            current = filters.current,
                            access = filters.access,
                            minPowerKw = filters.minPowerKw,
                            limit = limit,
                        ),
                    ).places
                } catch (e: CancellationException) {
                    throw e
                } catch (_: Exception) {
                    emptyList()
                }
                if (token != searchGeneration) return@launch
                publishSearch(page.items, places, offlineCached = false, moveToMatch, onResults)
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                val cached = loadMapCache(code).filter { it.matchesName(query) }
                if (token != searchGeneration) return@launch
                if (cached.isNotEmpty()) {
                    publishSearch(cached, emptyList(), offlineCached = true, moveToMatch, onResults)
                } else {
                    _ui.update {
                        it.copy(searching = false, searchFailed = true, searchResults = null, searchFocus = null)
                    }
                }
            }
        }
    }

    private fun publishSearch(
        stations: List<ServerStation>,
        places: List<ChargingPlace>,
        offlineCached: Boolean,
        moveToMatch: Boolean,
        onResults: ((List<MapOverlayMarker>) -> Unit)?,
    ) {
        val result = prepareDiscoveryOverlay(
            markers = stations.mapNotNull { it.toMarker() } + places.mapNotNull { it.toMarker() },
            gen = searchGeneration,
            nullCoordsStations = stations.filter { it.latitude == null || it.longitude == null },
            nullCoordsCharging = places.filter { it.latitude?.toDoubleOrNull() == null || it.longitude?.toDoubleOrNull() == null },
        )
        val matches = result.byUuid.values.toList()
        val pin = if (moveToMatch) matches.singleOrNull()?.takeIf { it.canRouteByCoordinates }
            else result.byUuid[_ui.value.searchFocus?.uuid]?.takeIf { it.canRouteByCoordinates }
        _ui.update {
            it.copy(
                searching = false,
                searchResults = MapStationSearchResults(result.items, result.byUuid, offlineCached),
                searchFocus = pin,
            )
        }
        if (moveToMatch && pin != null) engine.setViewport(pin.latitude, pin.longitude, 15)
        onResults?.invoke(matches)
    }

    fun onSearchResultSelected(uuid: String) {
        val marker = _ui.value.searchResults?.byUuid?.get(uuid) ?: return
        _ui.update { it.copy(searchFocus = marker.takeIf { it.canRouteByCoordinates }) }
        if (marker.canRouteByCoordinates) engine.setViewport(marker.latitude, marker.longitude, 15)
        onSelection(uuid)
    }

    fun onServiceFilter(showFuel: Boolean, showCharging: Boolean) {
        val fuel = showFuel || !showCharging
        val charge = showCharging || !showFuel
        val gen = ++generation
        _ui.update { it.copy(showFuel = fuel, showCharging = charge, generation = gen) }
        pushMarkers(lastMarkers, gen, lastNullStations, lastNullCharging, preserveLast = false)
        refreshSearch()
    }

    fun onApplyFilters(
        showFuel: Boolean,
        showCharging: Boolean,
        fuelProduct: FuelProduct? = _ui.value.fuelProduct,
        connector: ChargingConnector? = null,
        current: ChargingCurrent? = null,
        access: ChargingAccess? = null,
        minPowerKw: Int? = null,
        surveyPeriod: String? = _ui.value.surveyPeriod,
    ) {
        val fuel = showFuel || !showCharging
        val charge = showCharging || !showFuel
        fetchJob?.cancel()
        val gen = ++generation
        _ui.update {
            it.copy(
                showFuel = fuel,
                showCharging = charge,
                fuelProduct = fuelProduct,
                connector = connector,
                current = current,
                access = access,
                minPowerKw = minPowerKw,
                surveyPeriod = surveyPeriod,
                generation = gen,
            )
        }
        refreshSearch()
        viewModelScope.launch {
            val uiSnapshot = _ui.value
            val fuelOnlyMarkers = lastMarkers.filter { it.fuel && !it.charging }
            val chargingResult = if (charge) {
                try {
                    charging.query(
                        ChargingMarkersQuery(
                            latitude = uiSnapshot.viewportLatitude,
                            longitude = uiSnapshot.viewportLongitude,
                            radiusMeters = uiSnapshot.viewportRadiusMeters,
                            connector = connector,
                            current = current,
                            access = access,
                            minPowerKw = minPowerKw,
                            limit = 100,
                        )
                    )
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    ChargingMarkersResult(emptyList(), snapshotPresent = false, error = e.message)
                }
            } else {
                ChargingMarkersResult(emptyList(), snapshotPresent = uiSnapshot.chargingSnapshotPresent, error = null)
            }
            if (gen != generation) return@launch
            val chargeMarkers = chargingResult.places.mapNotNull { it.toMarker() }
            val nullCharging = chargingResult.places.filter {
                it.latitude?.toDoubleOrNull() == null || it.longitude?.toDoubleOrNull() == null
            }
            _ui.update {
                it.copy(chargingSnapshotPresent = chargingResult.snapshotPresent, chargingError = chargingResult.error)
            }
            lastMarkers = fuelOnlyMarkers + chargeMarkers
            lastNullCharging = nullCharging
            pushMarkers(lastMarkers, gen, lastNullStations, lastNullCharging, preserveLast = false)
        }
    }

    /**
     * B-BR-MAP-UNPINNED-SELECT-01: Select any station by canonical UUID,
     * whether pinned (any pin tier), unpinned (CITY_CENTROID / UNKNOWN), or
     * coordinate-less (`hasValidCoordinates == false`).
     */
    fun onSelection(uuid: String) {
        val key = uuid.trim().lowercase(Locale.ROOT)
        val current = _ui.value
        val marker = current.selectableByUuid[key]
            ?: current.clusters.flatMap { it.markers }.firstOrNull { it.uuid == key }
            ?: current.searchResults?.byUuid?.get(key)
            ?: current.selectedMembers.firstOrNull { it.uuid == key }
        _ui.update {
            val keepClusterMembers = it.selectedMembers.size > 1 && it.selectedMembers.any { m -> m.uuid == key }
            it.copy(
                selectedUuid = key,
                selectedMarker = marker,
                expandedClusterId = null,
                selectedMembers = when {
                    keepClusterMembers -> it.selectedMembers
                    marker != null -> listOf(marker)
                    else -> it.selectedMembers
                },
            )
        }
    }

    /**
     * B-BR-MAP-CLUSTER-01: Expanding a multi-station cluster exposes all
     * members for explicit choice instead of silently selecting only the first.
     */
    fun onClusterExpanded(clusterId: String) {
        val cluster = _ui.value.clusters.firstOrNull { it.id == clusterId } ?: return
        if (cluster.markers.size == 1) {
            onSelection(cluster.markers.single().uuid)
            return
        }
        _ui.update {
            it.copy(
                expandedClusterId = cluster.id,
                selectedMembers = cluster.markers,
                selectedUuid = null,
                selectedMarker = null,
            )
        }
    }

    fun dismissClusterExpansion() {
        _ui.update { it.copy(expandedClusterId = null) }
    }

    fun clearSelection() {
        _ui.update {
            it.copy(
                selectedUuid = null,
                selectedMarker = null,
                expandedClusterId = null,
                selectedMembers = emptyList(),
            )
        }
    }

    private fun onNativeSelection(selection: MapOverlaySelection) {
        when (selection) {
            is MapOverlaySelection.Station -> onSelection(selection.uuid)
            is MapOverlaySelection.Cluster -> {
                if (selection.members.size == 1) {
                    onSelection(selection.members.single().uuid)
                } else {
                    val memberIds = selection.members.map { it.uuid.lowercase(Locale.ROOT) }.toSet()
                    val matchingCluster = _ui.value.clusters.firstOrNull { c ->
                        c.markers.map { it.uuid }.toSet() == memberIds
                    }
                    _ui.update {
                        it.copy(
                            expandedClusterId = matchingCluster?.id ?: "cluster",
                            selectedMembers = selection.members,
                            selectedUuid = null,
                            selectedMarker = null,
                        )
                    }
                }
            }
        }
    }

    private fun pushMarkers(
        markers: List<MapOverlayMarker>,
        gen: Long,
        nullCoordsStations: List<ServerStation> = emptyList(),
        nullCoordsCharging: List<ChargingPlace> = emptyList(),
        preserveLast: Boolean = true,
    ) {
        if (preserveLast) {
            lastMarkers = markers
            lastNullStations = nullCoordsStations
            lastNullCharging = nullCoordsCharging
        }
        val overlay = prepareDiscoveryOverlay(markers, gen, nullCoordsStations, nullCoordsCharging)
        engine.setOverlayMarkers(overlay.clusters.flatMap { it.markers })
        _ui.update { prev ->
            val preservedMarker = prev.selectedUuid?.let { uuid ->
                overlay.byUuid[uuid] ?: prev.searchResults?.byUuid?.get(uuid)
            }
            val preservedUuid = preservedMarker?.uuid
            val reconciledMembers = if (prev.selectedMembers.size > 1) {
                prev.selectedMembers.mapNotNull { overlay.byUuid[it.uuid.lowercase(Locale.ROOT)] }
            } else {
                preservedMarker?.let { listOf(it) } ?: emptyList()
            }
            val preservedExpandedClusterId = if (prev.expandedClusterId != null && reconciledMembers.size > 1) {
                val memberSet = reconciledMembers.map { it.uuid }.toSet()
                overlay.clusters.firstOrNull { c ->
                    c.markers.map { it.uuid }.toSet() == memberSet
                }?.id ?: prev.expandedClusterId
            } else {
                null
            }
            prev.copy(
                clusters = overlay.clusters,
                listItems = overlay.items,
                selectableByUuid = overlay.byUuid,
                selectedUuid = preservedUuid,
                selectedMarker = preservedMarker,
                expandedClusterId = preservedExpandedClusterId,
                selectedMembers = reconciledMembers,
                loading = false,
                error = null,
                offlineCached = false,
            )
        }
    }

    private data class DiscoveryOverlay(
        val clusters: List<MapOverlayCluster>,
        val items: List<MapOverlayListItem>,
        val byUuid: Map<String, MapOverlayMarker>,
    )

    private fun prepareDiscoveryOverlay(
        markers: List<MapOverlayMarker>,
        gen: Long,
        nullCoordsStations: List<ServerStation>,
        nullCoordsCharging: List<ChargingPlace>,
    ): DiscoveryOverlay {
        val uiState = _ui.value
        val labels = overlayLabels
        val filtered = markers.filter {
            (it.fuel && uiState.showFuel) || (it.charging && uiState.showCharging)
        }.map { marker ->
            if (marker.charging && marker.title.isBlank()) marker.copy(title = labels.unnamedCharging)
            else marker
        }
        val preparation = prepareOverlay(filtered, radiusMeters = 60.0, generation = gen)
        val nullStationMarkers = nullCoordsStations.filter { uiState.showFuel }.map { it.toUnlocatedMarker() }
        val nullChargingMarkers = nullCoordsCharging.filter { uiState.showCharging }.map { it.toUnlocatedMarker(labels.unnamedCharging) }
        val nullItems = nullStationMarkers.map { m ->
            MapOverlayListItem(
                uuid = m.uuid,
                title = m.title,
                type = MapStationType.FUEL,
                typeLabel = labels.fuel,
                locationLabel = labels.unknownLocation,
                contentDescription = listOf(m.title, labels.fuel, labels.unknownLocation, m.subtitle)
                    .filter { it.isNotBlank() }
                    .joinToString(", "),
            )
        } + nullChargingMarkers.map { m ->
            val typeLabel = if (m.confirmedByTwoSources) {
                "${labels.charging} · ${labels.confirmedCharging}"
            } else labels.charging
            MapOverlayListItem(
                uuid = m.uuid,
                title = m.title,
                type = MapStationType.CHARGING,
                typeLabel = typeLabel,
                locationLabel = labels.unknownLocation,
                contentDescription = listOf(m.title, typeLabel, labels.unknownLocation, m.subtitle)
                    .filter { it.isNotBlank() }
                    .joinToString(", "),
            )
        }

        val items = (overlayListItems(
            clusters = preparation.clusters,
            labels = labels,
            unpinned = preparation.unpinned,
        ) + nullItems).distinctBy { it.uuid }

        val byUuid = LinkedHashMap<String, MapOverlayMarker>()
        for (c in preparation.clusters) {
            for (m in c.markers) {
                byUuid[m.uuid] = m
            }
        }
        for (m in preparation.unpinned) {
            byUuid.putIfAbsent(m.uuid, m)
        }
        for (m in nullStationMarkers) {
            byUuid.putIfAbsent(m.uuid, m)
        }
        for (m in nullChargingMarkers) {
            byUuid.putIfAbsent(m.uuid, m)
        }

        return DiscoveryOverlay(preparation.clusters, items, byUuid)
    }

    private suspend fun saveMapCache(code: String, stations: List<ServerStation>) {
        try {
            stationCache.saveScopedPage(mapScope(code), ServerStationPage(stations, null))
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // Cache failure never blocks the live map.
        }
    }

    private suspend fun loadMapCache(code: String): List<ServerStation> = try {
        stationCache.loadScopedPage(mapScope(code))?.items.orEmpty()
    } catch (e: CancellationException) {
        throw e
    } catch (_: Exception) {
        emptyList()
    }

    private fun mapScope(code: String): String = "map:${code.ifBlank { "national" }}"
}

private object NoopServerStationCache : ServerStationCache {
    override suspend fun savePage(page: ServerStationPage) = Unit
    override suspend fun loadPage(): ServerStationPage? = null
    override suspend fun saveDetail(station: ServerStation) = Unit
    override suspend fun loadDetail(stationId: String): ServerStation? = null
    override suspend fun clear() = Unit
}

private fun ServerStation.toMarker(): MapOverlayMarker? {
    val lat = latitude ?: return null
    val lon = longitude ?: return null
    if (lat !in -90.0..90.0 || lon !in -180.0..180.0) return null
    return MapOverlayMarker(
        uuid = stationId.lowercase(Locale.ROOT),
        latitude = lat,
        longitude = lon,
        title = displayName,
        fuel = true,
        charging = false,
        locationQuality = when (locationQuality) {
            StationLocationQuality.REVIEWED -> MapStationLocationQuality.REVIEWED
            StationLocationQuality.OSM_MAPPED -> MapStationLocationQuality.OSM_MAPPED
            StationLocationQuality.ANP_REPORTED -> MapStationLocationQuality.ANP_REPORTED
            StationLocationQuality.ADDRESS_POINT -> MapStationLocationQuality.ADDRESS_POINT
            StationLocationQuality.CITY_CENTROID -> MapStationLocationQuality.CITY_CENTROID
            StationLocationQuality.UNKNOWN -> MapStationLocationQuality.UNKNOWN
        },
        subtitle = addressLine,
        legalName = secondaryLegalName,
        hasValidCoordinates = true,
    )
}

private fun ServerStation.toUnlocatedMarker(): MapOverlayMarker = MapOverlayMarker(
    uuid = stationId.lowercase(Locale.ROOT),
    latitude = 0.0,
    longitude = 0.0,
    title = displayName,
    fuel = true,
    charging = false,
    locationQuality = MapStationLocationQuality.UNKNOWN,
    subtitle = addressLine,
    legalName = secondaryLegalName,
    hasValidCoordinates = false,
)

private fun ChargingPlace.toMarker(): MapOverlayMarker? {
    val lat = latitude?.toDoubleOrNull() ?: return null
    val lon = longitude?.toDoubleOrNull() ?: return null
    if (lat !in -90.0..90.0 || lon !in -180.0..180.0) return null
    // The catalog point determines pin eligibility, independent of source agreement.
    return MapOverlayMarker(
        uuid = id.lowercase(Locale.ROOT),
        latitude = lat,
        longitude = lon,
        title = name,
        fuel = false,
        charging = true,
        locationQuality = if (catalogConfidence != null) MapStationLocationQuality.CHARGING_SOURCE
            else MapStationLocationQuality.REVIEWED,
        subtitle = addressLine.orEmpty(),
        hasValidCoordinates = true,
        confirmedByTwoSources = confirmedByTwoSources,
    )
}

private fun ChargingPlace.toUnlocatedMarker(unnamed: String): MapOverlayMarker = MapOverlayMarker(
    uuid = id.lowercase(Locale.ROOT),
    latitude = 0.0,
    longitude = 0.0,
    title = name.ifBlank { unnamed },
    fuel = false,
    charging = true,
    locationQuality = MapStationLocationQuality.UNKNOWN,
    subtitle = addressLine.orEmpty(),
    hasValidCoordinates = false,
    confirmedByTwoSources = confirmedByTwoSources,
)
