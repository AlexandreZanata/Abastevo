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
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.ServerStationCache
import com.anpfuel.domain.repository.ServerStationGateway
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.valueobject.FuelProduct
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
    private var lastMarkers: List<MapOverlayMarker> = emptyList()
    private var lastNullStations: List<ServerStation> = emptyList()
    private var lastNullCharging: List<ChargingPlace> = emptyList()
    private var overlayLabels: MapOverlayLabels = MapOverlayLabels()
    private var lastPreferredMunicipalityCode: String? = null

    init {
        engine.setOnOverlaySelected(::onNativeSelection)
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
        val normalizedCode = ibgeCode.trim().ifBlank { _ui.value.municipalityCode }
        val knownCity = DEFAULT_MAP_CITIES.firstOrNull { it.ibgeCode == normalizedCode }
        val resolvedName = cityName.trim().ifBlank { knownCity?.name ?: normalizedCode }
        _ui.update {
            it.copy(
                municipalityCode = normalizedCode,
                municipalityName = resolvedName,
            )
        }
        val activeQuery = _ui.value.searchQuery.trim()
        if (activeQuery.length >= 2) {
            onSearch(normalizedCode, activeQuery)
        } else if (moveViewport && knownCity != null) {
            engine.setViewport(knownCity.latitude, knownCity.longitude, 13)
            onViewport(knownCity.latitude, knownCity.longitude, _ui.value.viewportRadiusMeters)
        } else if (moveViewport) {
            loadCityStations(normalizedCode)
        }
    }

    private fun loadCityStations(code: String) {
        fetchJob?.cancel()
        val gen = ++generation
        _ui.update { it.copy(loading = true, error = null, generation = gen) }
        fetchJob = viewModelScope.launch {
            try {
                val page = stations.listInMunicipality(code, 100, null)
                if (gen != generation) return@launch
                val reviewed = page.items.firstOrNull {
                    it.locationQuality == StationLocationQuality.REVIEWED && it.latitude != null && it.longitude != null
                }
                _ui.update { it.copy(cityLocationUnknown = reviewed == null, offlineCached = false) }
                saveMapCache(code, page.items)
                pushMarkers(
                    page.items.mapNotNull { it.toMarker() }, gen,
                    page.items.filter { it.latitude == null || it.longitude == null },
                )
                if (reviewed != null) {
                    engine.setViewport(checkNotNull(reviewed.latitude), checkNotNull(reviewed.longitude), 13)
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (gen != generation) return@launch
                val cached = loadMapCache(code)
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

    fun onViewport(latitude: Double, longitude: Double, radiusMeters: Int = 10_000) {
        if (!latitude.isFinite() || !longitude.isFinite()) return
        val boundedRadius = radiusMeters.coerceIn(100, 15_000)
        // Search is a mode: native camera sync must not silently replace
        // searched pins and the list with nearby results. Clearing the query
        // below re-enters viewport mode explicitly.
        val current = _ui.value
        if (current.searchQuery.isNotBlank()) {
            _ui.update {
                it.copy(
                    viewportLatitude = latitude,
                    viewportLongitude = longitude,
                    viewportRadiusMeters = boundedRadius,
                )
            }
            return
        }
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

    /**
     * MAP-20 search by city/name/address. Bounded, debounced, stale-safe.
     * Short/cleared queries immediately invalidate in-flight searches so a
     * prior debounced query can never overwrite cleared state.
     */
    fun onSearch(municipalityCode: String, query: String, limit: Int = 50) {
        val q = query.trim()
        val matchedCity = DEFAULT_MAP_CITIES.firstOrNull {
            it.name.equals(q, ignoreCase = true) || it.ibgeCode == q
        }
        val effectiveCode = municipalityCode.trim().ifBlank {
            matchedCity?.ibgeCode ?: _ui.value.municipalityCode
        }
        val effectiveName = DEFAULT_MAP_CITIES.firstOrNull { it.ibgeCode == effectiveCode }?.name
            ?: _ui.value.municipalityName
        // B-BR-MAP-DISCOVERY-01: Always cancel and increment generation immediately,
        // even when query < 2 chars, so no prior debounced search can overwrite.
        fetchJob?.cancel()
        val gen = ++generation
        if (q.length < 2) {
            _ui.update {
                it.copy(
                    searchQuery = q,
                    municipalityCode = effectiveCode,
                    municipalityName = effectiveName,
                    loading = false,
                    error = null,
                    generation = gen,
                )
            }
            if (q.isEmpty()) {
                val state = _ui.value
                onViewport(state.viewportLatitude, state.viewportLongitude, state.viewportRadiusMeters)
            }
            return
        }
        _ui.update {
            it.copy(
                loading = true,
                error = null,
                generation = gen,
                searchQuery = q,
                municipalityCode = effectiveCode,
                municipalityName = effectiveName,
            )
        }
        fetchJob = viewModelScope.launch {
            delay(300)
            if (gen != generation) return@launch
            try {
                val page = stations.search(effectiveCode, q, limit.coerceIn(1, 100), null)
                if (gen != generation) return@launch
                val markers = page.items.mapNotNull { it.toMarker() }
                val nullCoords = page.items.filter { it.latitude == null || it.longitude == null }
                val uiSnapshot = _ui.value
                val chargingResult = try {
                    charging.query(
                        ChargingMarkersQuery(
                            searchQuery = q,
                            connector = uiSnapshot.connector,
                            current = uiSnapshot.current,
                            access = uiSnapshot.access,
                            minPowerKw = uiSnapshot.minPowerKw,
                            limit = limit.coerceIn(1, 100),
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
                _ui.update { it.copy(offlineCached = false) }
                pushMarkers(
                    markers = markers + chargeMarkers,
                    gen = gen,
                    nullCoordsStations = nullCoords,
                    nullCoordsCharging = nullCharging,
                )
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (gen != generation) return@launch
                val cached = loadMapCache(effectiveCode).filter {
                    it.displayName.contains(q, ignoreCase = true)
                }
                if (cached.isNotEmpty()) {
                    pushMarkers(
                        cached.mapNotNull { it.toMarker() }, gen,
                        cached.filter { it.latitude == null || it.longitude == null },
                    )
                    _ui.update { it.copy(loading = false, error = null, offlineCached = true) }
                } else {
                    _ui.update { it.copy(loading = false, error = e.message ?: "search-failed") }
                }
            }
        }
    }

    fun onServiceFilter(showFuel: Boolean, showCharging: Boolean) {
        val fuel = showFuel || !showCharging
        val charge = showCharging || !showFuel
        val gen = ++generation
        _ui.update { it.copy(showFuel = fuel, showCharging = charge, generation = gen) }
        val filtered = lastMarkers.filter {
            (it.fuel && fuel) || (it.charging && charge)
        }
        val nullFiltered = if (fuel) lastNullStations else emptyList()
        val nullChargingFiltered = if (charge) lastNullCharging else emptyList()
        pushMarkers(
            markers = filtered,
            gen = gen,
            nullCoordsStations = nullFiltered,
            nullCoordsCharging = nullChargingFiltered,
            preserveLast = false,
        )
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
        viewModelScope.launch {
            val uiSnapshot = _ui.value
            val fuelOnlyMarkers = lastMarkers.filter { it.fuel && !it.charging }
            val chargingResult = if (charge) {
                try {
                    charging.query(
                        ChargingMarkersQuery(
                            latitude = uiSnapshot.viewportLatitude.takeIf { uiSnapshot.searchQuery.length < 2 },
                            longitude = uiSnapshot.viewportLongitude.takeIf { uiSnapshot.searchQuery.length < 2 },
                            radiusMeters = uiSnapshot.viewportRadiusMeters,
                            searchQuery = uiSnapshot.searchQuery.takeIf { it.length >= 2 },
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
            lastMarkers = fuelOnlyMarkers + chargeMarkers
            lastNullCharging = nullCharging
            val filtered = lastMarkers.filter {
                (it.fuel && fuel) || (it.charging && charge)
            }
            pushMarkers(
                markers = filtered,
                gen = gen,
                nullCoordsStations = if (fuel) lastNullStations else emptyList(),
                nullCoordsCharging = if (charge) lastNullCharging else emptyList(),
                preserveLast = false,
            )
        }
    }

    /**
     * B-BR-MAP-UNPINNED-SELECT-01: Select any station by canonical UUID,
     * whether pinned (REVIEWED), unpinned (CITY_CENTROID / UNKNOWN), or
     * coordinate-less (`hasValidCoordinates == false`).
     */
    fun onSelection(uuid: String) {
        val key = uuid.trim().lowercase(Locale.ROOT)
        val current = _ui.value
        val marker = current.selectableByUuid[key]
            ?: current.clusters.flatMap { it.markers }.firstOrNull { it.uuid == key }
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
        val uiState = _ui.value
        val filtered = markers.filter {
            (it.fuel && uiState.showFuel) || (it.charging && uiState.showCharging)
        }
        val preparation = prepareOverlay(filtered, radiusMeters = 60.0, generation = gen)
        engine.setOverlayMarkers(
            preparation.clusters.flatMap { it.markers },
        )
        val labels = overlayLabels
        val nullStationMarkers = nullCoordsStations.map { it.toUnlocatedMarker() }
        val nullChargingMarkers = nullCoordsCharging.map { it.toUnlocatedMarker() }
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
            MapOverlayListItem(
                uuid = m.uuid,
                title = m.title,
                type = MapStationType.CHARGING,
                typeLabel = labels.charging,
                locationLabel = labels.unknownLocation,
                contentDescription = listOf(m.title, labels.charging, labels.unknownLocation, m.subtitle)
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

        _ui.update { prev ->
            val preservedUuid = prev.selectedUuid?.takeIf { sel ->
                byUuid.containsKey(sel) || items.any { it.uuid == sel }
            }
            val preservedMarker = preservedUuid?.let { byUuid[it] }
            val reconciledMembers = if (prev.selectedMembers.size > 1) {
                prev.selectedMembers.mapNotNull { byUuid[it.uuid.lowercase(Locale.ROOT)] }
            } else {
                preservedMarker?.let { listOf(it) } ?: emptyList()
            }
            val preservedExpandedClusterId = if (prev.expandedClusterId != null && reconciledMembers.size > 1) {
                val memberSet = reconciledMembers.map { it.uuid }.toSet()
                preparation.clusters.firstOrNull { c ->
                    c.markers.map { it.uuid }.toSet() == memberSet
                }?.id ?: prev.expandedClusterId
            } else {
                null
            }
            prev.copy(
                clusters = preparation.clusters,
                listItems = items,
                selectableByUuid = byUuid,
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
            StationLocationQuality.CITY_CENTROID -> MapStationLocationQuality.CITY_CENTROID
            StationLocationQuality.UNKNOWN -> MapStationLocationQuality.UNKNOWN
        },
        subtitle = addressLabel(),
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
    subtitle = addressLabel(),
    hasValidCoordinates = false,
)

private fun ServerStation.addressLabel(): String =
    listOfNotNull(municipalityCode, state).joinToString("/")

private fun ChargingPlace.toMarker(): MapOverlayMarker? {
    val lat = latitude?.toDoubleOrNull() ?: return null
    val lon = longitude?.toDoubleOrNull() ?: return null
    if (lat !in -90.0..90.0 || lon !in -180.0..180.0) return null
    // OCM snapshot coordinates are admitted reference geometry; pin-eligible
    // when present, list-only when absent (null returns above).
    return MapOverlayMarker(
        uuid = id.lowercase(Locale.ROOT),
        latitude = lat,
        longitude = lon,
        title = name,
        fuel = false,
        charging = true,
        locationQuality = MapStationLocationQuality.REVIEWED,
        subtitle = country,
        hasValidCoordinates = true,
    )
}

private fun ChargingPlace.toUnlocatedMarker(): MapOverlayMarker = MapOverlayMarker(
    uuid = id.lowercase(Locale.ROOT),
    latitude = 0.0,
    longitude = 0.0,
    title = name,
    fuel = false,
    charging = true,
    locationQuality = MapStationLocationQuality.UNKNOWN,
    subtitle = country,
    hasValidCoordinates = false,
)
