package com.anpfuel.maps.engine

import android.os.Looper
import app.organicmaps.sdk.Framework
import app.organicmaps.sdk.Router
import app.organicmaps.sdk.routing.ResultCodes
import app.organicmaps.sdk.routing.RouteMarkType
import app.organicmaps.sdk.routing.RoutingOptions
import app.organicmaps.sdk.settings.RoadType
import app.organicmaps.sdk.util.Distance
import com.anpfuel.domain.routing.*

/** Owns the SDK global route listener; SDK classes never cross the app port.
 * Uses the pinned native core directly: its Java controller calls onBuiltRoute
 * even for NEED_MORE_MAPS, which must never be accepted as a drivable success.
 */
internal class ComapsRoutePlanner(private val initialized: () -> Boolean) : NativeRoutePlanner {
    private var generation = 0L

    override fun buildRoute(request: NativeRouteRequest, onResult: (NativeRouteResult) -> Unit): Boolean {
        check(Looper.myLooper() == Looper.getMainLooper())
        request.validate()
        if (!initialized()) return false
        cancelRoute()
        val current = generation
        Router.set(Router.Vehicle)
        val selected = request.avoid.map { when(it) {
            RoadAvoidance.TOLL -> RoadType.Toll
            RoadAvoidance.MOTORWAY -> RoadType.Motorway
            RoadAvoidance.FERRY -> RoadType.Ferry
            RoadAvoidance.UNPAVED -> RoadType.Dirty
        }}.toSet()
        // No stale bicycle/ruler or previous avoidance option may affect a car route.
        RoadType.values().forEach { if (it in selected) RoutingOptions.addOption(it,Router.Vehicle) else RoutingOptions.removeOption(it,Router.Vehicle) }
        Framework.nativeSetRoutingListener { code, missing ->
            if (current != generation) return@nativeSetRoutingListener
            if (code == ResultCodes.NO_ERROR || code == ResultCodes.HAS_WARNINGS) {
                val info = Framework.nativeGetRouteFollowingInfo()
                val geometry = // The pinned SDK uses +infinity to disable interpolation. Zero
                // requests an unbounded number of interpolated points in JNI.
                Framework.nativeGetRouteJunctionPoints(Double.POSITIVE_INFINITY).orEmpty().map { RoutePoint(it.mLat,it.mLon,"") }
                if (!Framework.nativeIsRouteBuilt() || info == null || geometry.size < 2) {
                    onResult(NativeRouteResult.Failed(ResultCodes.ROUTE_NOT_FOUND))
                } else {
                    val steps = Framework.nativeGetRouteSteps(request.language).orEmpty().map { RouteManeuver(it.textualInstruction,it.distMeters,it.toStreetName.orEmpty()) }
                    onResult(NativeRouteResult.Ready(NativeRoutePreview(info.distToTarget.meters(),info.totalTimeInSeconds,geometry,steps,code == ResultCodes.HAS_WARNINGS)))
                }
            } else {
                onResult(NativeRouteResult.Failed(code,missing.orEmpty().distinct().sorted()))
            }
        }
        request.points.forEachIndexed { index, point ->
            val kind = when(index) { 0 -> RouteMarkType.Start; request.points.lastIndex -> RouteMarkType.Finish; else -> RouteMarkType.Intermediate }
            Framework.nativeAddRoutePoint(point.label,"",kind,if (kind == RouteMarkType.Intermediate) index-1 else 0,false,point.latitude,point.longitude,false)
        }
        Framework.nativeBuildRoute()
        return true
    }

    override fun cancelRoute() {
        generation++
        if (initialized()) {
            Framework.nativeCloseRouting()
            Framework.nativeRemoveRoutePoints()
        }
    }
}

internal fun Distance.meters(): Double = mDistance * when(mUnits) {
    Distance.Units.Kilometers -> 1000.0
    Distance.Units.Miles -> 1609.344
    Distance.Units.Feet -> 0.3048
    else -> 1.0
}
