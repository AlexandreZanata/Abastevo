package com.anpfuel.app.ui.map

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.repository.ServerStationGateway
import com.anpfuel.maps.engine.MapEngine
import com.anpfuel.maps.engine.MapOverlayCluster
import com.anpfuel.maps.engine.MapOverlayListItem
import com.anpfuel.maps.engine.MapOverlayMarker
import com.anpfuel.maps.engine.MapOverlaySelection
import com.anpfuel.maps.engine.MapStationLocationQuality
import com.anpfuel.maps.engine.overlayListItems
import com.anpfuel.maps.engine.prepareOverlay
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class MapDiscoveryUi(
    val clusters: List<MapOverlayCluster> = emptyList(),
    val listItems: List<MapOverlayListItem> = emptyList(),
    val selectedUuid: String? = null,
    val selectedMembers: List<MapOverlayMarker> = emptyList(),
    val loading: Boolean = false,
    val error: String? = null,
    val generation: Long = 0,
    val showFuel: Boolean = true,
    val showCharging: Boolean = true,
    val searchQuery: String = "",
)

/**
 * MAP-18 viewport discovery + MAP-20 search/filters: bounded nearby/search
 * queries with debounce and stale discard. Canonical UUIDs are the stable
 * identity; proximity never merges records. Unknown/city-centroid stay
 * list-only via `prepareOverlay`. Unknown metadata is shown, never excluded.
 */
@HiltViewModel
class MapDiscoveryViewModel @Inject constructor(
    private val stations: ServerStationGateway,
    private val charging: ChargingMarkersSource,
    private val engine: MapEngine,
) : ViewModel() {
    private val _ui = MutableStateFlow(MapDiscoveryUi())
    val ui: StateFlow<MapDiscoveryUi> = _ui.asStateFlow()

    private var fetchJob: Job? = null
    private var generation = 0L
    private var lastMarkers: List<MapOverlayMarker> = emptyList()
    private var lastNullStations: List<ServerStation> = emptyList()

    init {
        engine.setOnOverlaySelected(::onNativeSelection)
    }

    fun onViewport(latitude: Double, longitude: Double, radiusMeters: Int = 10_000) {
        val boundedRadius = radiusMeters.coerceIn(100, 15_000)
        fetchJob?.cancel()
        fetchJob = viewModelScope.launch {
            delay(300)
            val gen = ++generation
            _ui.update { it.copy(loading = true, error = null, generation = gen) }
            try {
                val nearby: List<NearbyServerStation> = stations.nearby(
                    latitude,
                    longitude,
                    boundedRadius,
                    100,
                )
                if (gen != generation) return@launch
                val fuelMarkers = nearby.mapNotNull { it.station.toMarker() }
                // Stations without coordinates stay list-only with explicit warning.
                val nullCoords = nearby.map { it.station }.filter { it.latitude == null }
                // Charging snapshot is offline Room; failures never block fuel pins.
                val chargeMarkers = try {
                    charging.list(100).mapNotNull { it.toMarker() }
                } catch (_: Exception) {
                    emptyList()
                }
                if (gen != generation) return@launch
                pushMarkers(fuelMarkers + chargeMarkers, gen, nullCoords)
            } catch (e: kotlinx.coroutines.CancellationException) {
                throw e
            } catch (e: Exception) {
                if (gen != generation) return@launch
                _ui.update { it.copy(loading = false, error = e.message ?: "discovery-failed") }
            }
        }
    }

    /**
     * MAP-20 search by city/name/address. Bounded, debounced, stale-safe.
     * Unknown metadata is preserved; it never excludes a station.
     */
    fun onSearch(municipalityCode: String, query: String, limit: Int = 50) {
        val q = query.trim()
        if (q.length < 2) {
            _ui.update { it.copy(searchQuery = q) }
            return
        }
        fetchJob?.cancel()
        fetchJob = viewModelScope.launch {
            delay(300)
            val gen = ++generation
            _ui.update { it.copy(loading = true, error = null, generation = gen, searchQuery = q) }
            try {
                val page = stations.search(municipalityCode, q, limit.coerceIn(1, 100), null)
                if (gen != generation) return@launch
                val markers = page.items.mapNotNull { it.toMarker() }
                val nullCoords = page.items.filter { it.latitude == null }
                pushMarkers(markers, gen, nullCoords)
            } catch (e: kotlinx.coroutines.CancellationException) {
                throw e
            } catch (e: Exception) {
                if (gen != generation) return@launch
                _ui.update { it.copy(loading = false, error = e.message ?: "search-failed") }
            }
        }
    }

    fun onServiceFilter(showFuel: Boolean, showCharging: Boolean) {
        val fuel = showFuel || !showCharging
        val charge = showCharging || !showFuel
        _ui.update { it.copy(showFuel = fuel, showCharging = charge) }
        val gen = ++generation
        val filtered = lastMarkers.filter {
            (it.fuel && fuel) || (it.charging && charge)
        }
        // Null-coords fuel stations respect the fuel filter; charging nulls
        // are snapshot-based and follow the charging flag.
        val nullFiltered = if (fuel) lastNullStations else emptyList()
        pushMarkers(filtered, gen, nullFiltered, preserveLast = false)
    }

    fun onSelection(uuid: String) {
        val current = _ui.value
        val marker = current.clusters.flatMap { it.markers }.firstOrNull { it.uuid == uuid }
        if (marker != null) {
            _ui.update { it.copy(selectedUuid = uuid, selectedMembers = listOf(marker)) }
        } else {
            // Cluster expansion or list-only selection: keep UUID even if the
            // pin is a cluster member resolved via native callback.
            _ui.update { it.copy(selectedUuid = uuid) }
        }
    }

    fun onClusterExpanded(clusterId: String) {
        val cluster = _ui.value.clusters.firstOrNull { it.id == clusterId } ?: return
        _ui.update { it.copy(selectedUuid = clusterId, selectedMembers = cluster.markers) }
    }

    fun clearSelection() {
        _ui.update { it.copy(selectedUuid = null, selectedMembers = emptyList()) }
    }

    private fun onNativeSelection(selection: MapOverlaySelection) {
        when (selection) {
            is MapOverlaySelection.Station -> onSelection(selection.uuid)
            is MapOverlaySelection.Cluster -> _ui.update {
                it.copy(
                    selectedUuid = selection.members.firstOrNull()?.uuid,
                    selectedMembers = selection.members,
                )
            }
        }
    }

    private fun pushMarkers(
        markers: List<MapOverlayMarker>,
        gen: Long,
        nullCoordsStations: List<ServerStation> = emptyList(),
        preserveLast: Boolean = true,
    ) {
        if (preserveLast) {
            lastMarkers = markers
            lastNullStations = nullCoordsStations
        }
        val uiState = _ui.value
        val filtered = markers.filter {
            (it.fuel && uiState.showFuel) || (it.charging && uiState.showCharging)
        }
        val preparation = prepareOverlay(filtered, radiusMeters = 60.0, generation = gen)
        engine.setOverlayMarkers(
            preparation.clusters.flatMap { it.markers },
        )
        val nullItems = nullCoordsStations.map { s ->
            com.anpfuel.maps.engine.MapOverlayListItem(
                uuid = s.stationId.lowercase(),
                title = s.displayName,
                type = com.anpfuel.maps.engine.MapStationType.FUEL,
                typeLabel = "Fuel",
                locationLabel = "Location unknown",
                contentDescription = "${s.displayName}, Fuel, Location unknown",
            )
        }
        val items = overlayListItems(preparation.clusters, unpinned = preparation.unpinned) + nullItems
        _ui.update { prev ->
            val preserved = prev.selectedUuid?.takeIf { sel ->
                items.any { it.uuid == sel } || preparation.clusters.any { c -> c.markers.any { it.uuid == sel } }
            }
            prev.copy(
                clusters = preparation.clusters,
                listItems = items,
                selectedUuid = preserved,
                selectedMembers = preserved?.let { sel ->
                    preparation.clusters.flatMap { it.markers }.filter { it.uuid == sel }
                } ?: emptyList(),
                loading = false,
                error = null,
            )
        }
    }
}

private fun ServerStation.toMarker(): MapOverlayMarker? {
    val lat = latitude ?: return null
    val lon = longitude ?: return null
    if (lat !in -90.0..90.0 || lon !in -180.0..180.0) return null
    return MapOverlayMarker(
        uuid = stationId.lowercase(),
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
    )
}

private fun ServerStation.addressLabel(): String =
    listOfNotNull(municipalityCode, state).joinToString("/")

private fun com.anpfuel.domain.charging.ChargingPlace.toMarker(): MapOverlayMarker? {
    val lat = latitude?.toDoubleOrNull() ?: return null
    val lon = longitude?.toDoubleOrNull() ?: return null
    if (lat !in -90.0..90.0 || lon !in -180.0..180.0) return null
    // OCM snapshot coordinates are admitted reference geometry; pin-eligible
    // when present, list-only when absent (null returns above).
    return MapOverlayMarker(
        uuid = id.lowercase(),
        latitude = lat,
        longitude = lon,
        title = name,
        fuel = false,
        charging = true,
        locationQuality = MapStationLocationQuality.REVIEWED,
        subtitle = country,
    )
}
