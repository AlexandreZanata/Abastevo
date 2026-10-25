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
)

/**
 * MAP-18 viewport discovery: bounded nearby queries with debounce and stale
 * discard. Canonical UUIDs are the stable identity; proximity never merges
 * records. Unknown/city-centroid stay list-only via `prepareOverlay`.
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

    init {
        engine.setOnOverlaySelected(::onNativeSelection)
    }

    fun onViewport(latitude: Double, longitude: Double, radiusMeters: Int = 10_000) {
        val boundedRadius = radiusMeters.coerceIn(500, 50_000)
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
                // Charging snapshot is offline Room; failures never block fuel pins.
                val chargeMarkers = try {
                    charging.list(100).mapNotNull { it.toMarker() }
                } catch (_: Exception) {
                    emptyList()
                }
                if (gen != generation) return@launch
                pushMarkers(fuelMarkers + chargeMarkers, gen)
            } catch (e: kotlinx.coroutines.CancellationException) {
                throw e
            } catch (e: Exception) {
                if (gen != generation) return@launch
                _ui.update { it.copy(loading = false, error = e.message ?: "discovery-failed") }
            }
        }
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

    private fun pushMarkers(markers: List<MapOverlayMarker>, gen: Long) {
        val preparation = prepareOverlay(markers, radiusMeters = 60.0, generation = gen)
        engine.setOverlayMarkers(
            // Only pin-eligible markers reach the native KML; unpinned stay
            // list-only. `prepareOverlay` already split them.
            preparation.clusters.flatMap { it.markers },
        )
        val items = overlayListItems(preparation.clusters, unpinned = preparation.unpinned)
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
