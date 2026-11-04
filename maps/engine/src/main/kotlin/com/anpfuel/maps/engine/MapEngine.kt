package com.anpfuel.maps.engine

import android.content.Context
import android.location.Location
import android.view.View
import androidx.compose.ui.unit.IntRect
import androidx.compose.ui.unit.IntSize

data class MapSearchResult(val name: String, val latitude: Double, val longitude: Double)

data class MapRouteInfo(val distance: Double, val unit: String, val seconds: Int)

data class MapCenter(val latitude: Double, val longitude: Double)

data class MapCamera(val latitude: Double, val longitude: Double, val zoom: Int)

enum class GuidanceCameraAction { RECENTER, ORIENTATION, OVERVIEW, ZOOM_IN, ZOOM_OUT }

/** Explicit foreground guidance feed result; poor fixes never advance progress. */
data class GuidanceFeed(
    val progress: com.anpfuel.domain.routing.NavigationProgress?,
    val notifications: List<String> = emptyList(),
    val rerouteRecoverable: Boolean = false,
    val arrived: Boolean = false,
)

/** Narrow Abastevo-owned surface over the pinned native map core. */
interface MapEngine : com.anpfuel.domain.routing.NativeRoutePlanner {
    override fun buildRoute(request: com.anpfuel.domain.routing.NativeRouteRequest, onResult: (com.anpfuel.domain.routing.NativeRouteResult) -> Unit): Boolean = false
    fun initialize(context: Context, onReady: () -> Unit): Boolean
    fun newMapView(context: Context): View
    fun setViewport(latitude: Double, longitude: Double, zoom: Int)
    fun currentCenter(): MapCenter?
    fun currentCamera(): MapCamera? = null
    fun recenter(): Boolean
    /**
     * Foreground-only location after an explicit recenter tap: shows the
     * my-position marker and reports the first fix once. Never started by
     * browsing; [stopMyLocation] ends it when the map leaves the screen.
     */
    fun startMyLocation(onFix: (MapCenter) -> Unit): Boolean = false
    fun stopMyLocation() = Unit
    /** The foreground map’s last sample, including its original time and accuracy. */
    fun lastMapLocation(): Location? = null
    fun isRenderingReady(): Boolean = isInitialized()
    fun isInitialized(): Boolean
    fun reloadMaps(): Boolean = false
    fun suspendMapReads(): Boolean = false
    fun writableDir(): String? = null
    fun isDownloadedMapAtScreenCenter(): Boolean = false
    fun downloadedMapsCount(): Int = 0

    fun search(context: Context, query: String, onResult: (List<MapSearchResult>) -> Unit): Boolean
    fun buildCarRoute(
        startLatitude: Double,
        startLongitude: Double,
        endLatitude: Double,
        endLongitude: Double,
        onResult: (MapRouteInfo?) -> Unit,
    ): Boolean
    fun setOverlayMarkers(markers: List<MapOverlayMarker>): Boolean
    fun overlayClusters(): List<MapOverlayCluster>
    /** Global bookmark-label visibility; hides clutter when zoomed out. Defaults to shown. */
    fun setOverlayLabelsVisible(visible: Boolean) = Unit
    fun setOnOverlaySelected(listener: (MapOverlaySelection) -> Unit)
    /** Draws the selected pin enlarged in place of the SDK selection circle; null clears it. */
    fun setHighlightedMarker(marker: MapOverlayMarker?) = Unit
    fun cancelSearch()
    override fun cancelRoute()
    fun startGuidance(request: com.anpfuel.domain.routing.NativeRouteRequest): Boolean = false
    fun feedGuidanceFix(
        fix: com.anpfuel.domain.routing.NavigationFix,
        nowNanos: Long,
    ): GuidanceFeed? = null
    fun guidanceProgress(): com.anpfuel.domain.routing.NavigationProgress? = null
    fun setGuidanceViewport(visible: IntRect, surface: IntSize) = Unit
    fun controlGuidanceCamera(action: GuidanceCameraAction) = Unit
    fun guidanceManeuvers(language: String): List<com.anpfuel.domain.routing.RouteManeuver> = emptyList()
    fun stopGuidance() = Unit
    fun isGuiding(): Boolean = false
    fun release()
}
