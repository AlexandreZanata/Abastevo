package com.anpfuel.maps.engine

import android.content.Context
import android.view.View

data class MapSearchResult(val name: String, val latitude: Double, val longitude: Double)

data class MapRouteInfo(val distance: Double, val unit: String, val seconds: Int)

data class MapCenter(val latitude: Double, val longitude: Double)

data class MapCamera(val latitude: Double, val longitude: Double, val zoom: Int)

/** Narrow Abastevo-owned surface over the pinned native map core. */
interface MapEngine {
    fun initialize(context: Context, onReady: () -> Unit): Boolean
    fun newMapView(context: Context): View
    fun setViewport(latitude: Double, longitude: Double, zoom: Int)
    fun currentCenter(): MapCenter?
    fun recenter(): Boolean
    fun isInitialized(): Boolean

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
    fun cancelRoute()
    fun release()
}
