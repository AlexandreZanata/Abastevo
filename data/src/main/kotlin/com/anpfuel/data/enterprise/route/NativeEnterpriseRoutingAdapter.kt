package com.anpfuel.data.enterprise.route

import com.anpfuel.application.enterprise.route.EnterpriseRoutingOutcome
import com.anpfuel.application.enterprise.route.EnterpriseRoutingPort
import com.anpfuel.domain.enterprise.route.EnterpriseRouteComputation
import com.anpfuel.domain.enterprise.route.GeoPointE7
import com.anpfuel.domain.enterprise.route.GuidanceDegradedState
import com.anpfuel.domain.enterprise.route.RouteAvoidOptions
import com.anpfuel.domain.enterprise.route.RouteComputationKind
import com.anpfuel.domain.enterprise.route.RoutePlanOwnerKind
import com.anpfuel.domain.enterprise.route.RouteWaypoint
import java.time.Instant
import java.util.UUID

/** Raw on-device CoMaps CAR router outcome prior to domain normalization. */
sealed interface NativeCarRouterResult {
    class Success(
        val engineId: String,
        val mapDataVersion: String,
        val distanceMetres: Long,
        val durationSeconds: Long,
        val geometry: List<GeoPointE7>,
    ) : NativeCarRouterResult

    data object MapRegionMissing : NativeCarRouterResult
    data object RouteNotFound : NativeCarRouterResult
}

fun interface NativeCarRouterBridge {
    fun buildCarRoute(points: List<GeoPointE7>, options: RouteAvoidOptions): NativeCarRouterResult
}

/**
 * EMP-ROUTE-010 / EMP-GOV-007: executes multi-stop CAR routing purely on
 * device over CoMaps offline map regions while keeping the server routing slot
 * disabled in F5.
 */
class NativeEnterpriseRoutingAdapter(
    private val bridge: NativeCarRouterBridge,
    private val nextComputationId: () -> String = { UUID.randomUUID().toString() },
) : EnterpriseRoutingPort {
    override val isServerRoutingUsed: Boolean = false

    override fun computeRoute(
        ownerKind: RoutePlanOwnerKind,
        ownerId: String,
        planRevision: Long,
        kind: RouteComputationKind,
        waypoints: List<RouteWaypoint>,
        options: RouteAvoidOptions,
        computedAt: Instant,
    ): EnterpriseRoutingOutcome {
        val points = waypoints.mapNotNull { it.point }
        if (points.size < 2) {
            return EnterpriseRoutingOutcome.Degraded(GuidanceDegradedState.ROUTE_NOT_FOUND)
        }
        return when (val raw = bridge.buildCarRoute(points, options)) {
            is NativeCarRouterResult.MapRegionMissing ->
                EnterpriseRoutingOutcome.Degraded(GuidanceDegradedState.MAP_REGION_MISSING)
            is NativeCarRouterResult.RouteNotFound ->
                EnterpriseRoutingOutcome.Degraded(GuidanceDegradedState.ROUTE_NOT_FOUND)
            is NativeCarRouterResult.Success -> EnterpriseRoutingOutcome.Computed(
                EnterpriseRouteComputation(
                    id = nextComputationId(),
                    ownerKind = ownerKind,
                    ownerId = ownerId,
                    planRevision = planRevision,
                    engineId = raw.engineId,
                    mapDataVersion = raw.mapDataVersion,
                    profile = EnterpriseRouteComputation.PROFILE_CAR,
                    kind = kind,
                    options = options,
                    distanceMetres = raw.distanceMetres,
                    durationSeconds = raw.durationSeconds,
                    line = raw.geometry,
                    attribution = EnterpriseRouteComputation.OSM_ATTRIBUTION,
                    computedAt = computedAt,
                ),
            )
        }
    }
}
