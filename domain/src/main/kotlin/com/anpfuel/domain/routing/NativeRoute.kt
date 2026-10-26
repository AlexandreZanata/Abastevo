package com.anpfuel.domain.routing

/** Points are local planning input, never telemetry or a public contribution. */
data class RoutePoint(val latitude: Double, val longitude: Double, val label: String, val approximateEntrance: Boolean = true)
enum class RoadAvoidance { TOLL, MOTORWAY, FERRY, UNPAVED }
data class NativeRouteRequest(val points: List<RoutePoint>, val avoid: Set<RoadAvoidance> = emptySet(), val language: String = "en") {
    fun validate() {
        // Pinned native RoutePointsLayout permits 100 intermediate points.
        require(points.size in 2..102)
        require(points.all { it.latitude.isFinite() && it.longitude.isFinite() && it.latitude in -90.0..90.0 && it.longitude in -180.0..180.0 && it.label.length <= 512 })
        require(points.map { it.latitude to it.longitude }.distinct().size == points.size)
        require(language.matches(Regex("[a-z]{2}(-[A-Za-z]{2,4})?")))
    }
}
data class RouteManeuver(val instruction: String, val distanceMeters: Double, val street: String)
data class NativeRoutePreview(val distanceMeters: Double, val seconds: Int, val geometry: List<RoutePoint>, val maneuvers: List<RouteManeuver>, val warnings: Boolean)
sealed interface NativeRouteResult {
    data class Ready(val preview: NativeRoutePreview): NativeRouteResult
    data class Failed(val code: Int, val missingMaps: List<String> = emptyList()): NativeRouteResult
}
interface NativeRoutePlanner {
    fun buildRoute(request: NativeRouteRequest, onResult: (NativeRouteResult) -> Unit): Boolean
    fun cancelRoute()
}
