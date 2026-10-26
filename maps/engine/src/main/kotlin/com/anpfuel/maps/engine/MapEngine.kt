package com.anpfuel.maps.engine

import android.content.Context
import android.view.View

data class MapSearchResult(val name: String, val latitude: Double, val longitude: Double)

data class MapRouteInfo(val distance: Double, val unit: String, val seconds: Int)

data class MapCenter(val latitude: Double, val longitude: Double)

data class MapCamera(val latitude: Double, val longitude: Double, val zoom: Int)

/** Narrow Abastevo-owned surface over the pinned native map core. */
interface MapEngine : com.anpfuel.domain.routing.NativeRoutePlanner {
    override fun buildRoute(request: com.anpfuel.domain.routing.NativeRouteRequest, onResult: (com.anpfuel.domain.routing.NativeRouteResult) -> Unit): Boolean = false
    fun initialize(context: Context, onReady: () -> Unit): Boolean
    fun newMapView(context: Context): View
    fun setViewport(latitude: Double, longitude: Double, zoom: Int)
    fun currentCenter(): MapCenter?
    fun currentCamera(): MapCamera? = null
    fun recenter(): Boolean
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
    fun setOnOverlaySelected(listener: (MapOverlaySelection) -> Unit)
    fun cancelSearch()
    override fun cancelRoute()
    fun release()
}
