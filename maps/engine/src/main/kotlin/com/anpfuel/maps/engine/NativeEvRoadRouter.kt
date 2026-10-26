package com.anpfuel.maps.engine

import com.anpfuel.domain.ev.EvRoadLeg
import com.anpfuel.domain.ev.EvRoadRouter
import com.anpfuel.domain.ev.EvRoadUnavailable
import com.anpfuel.domain.routing.NativeRoutePlanner
import com.anpfuel.domain.routing.NativeRouteRequest
import com.anpfuel.domain.routing.NativeRouteResult
import com.anpfuel.domain.routing.RoadAvoidance
import com.anpfuel.domain.routing.RoutePoint
import kotlin.coroutines.resume
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull

/**
 * MAP-27 native adapter. The SDK route listener is global and accepts one callback per
 * generation; a token makes a late callback from a cancelled plan harmless. A timeout and
 * coroutine cancellation cancel the native route instead of leaving it running.
 *
 * It implements the domain port only; it does not decide eligibility, energy or fallback.
 */
class NativeEvRoadRouter(
    private val planner: NativeRoutePlanner,
    private val dispatcher: CoroutineDispatcher = Dispatchers.Main.immediate,
    private val timeoutMillis: Long = 60_000,
) : EvRoadRouter {
    private var generation = 0L

    override suspend fun leg(
        points: List<RoutePoint>,
        avoid: Set<RoadAvoidance>,
        language: String,
    ): EvRoadLeg =
        withContext(dispatcher) {
            if (points.size !in 2..102) return@withContext EvRoadLeg.Unavailable(EvRoadUnavailable.NO_ROUTE)
            if (points.map { it.latitude to it.longitude }.distinct().size != points.size) {
                return@withContext EvRoadLeg.Unavailable(EvRoadUnavailable.NO_ROUTE)
            }
            withTimeoutOrNull(timeoutMillis) {
                suspendCancellableCoroutine<EvRoadLeg> { continuation ->
                    val token = ++generation
                    planner.cancelRoute()
                    val request = NativeRouteRequest(points, avoid, language)
                    val accepted = try {
                        planner.buildRoute(request) { result ->
                            if (token != generation || !continuation.isActive) return@buildRoute
                            continuation.resume(result.toDomain())
                        }
                    } catch (_: Exception) {
                        false
                    }
                    if (!accepted && continuation.isActive) {
                        continuation.resume(EvRoadLeg.Unavailable(EvRoadUnavailable.NOT_READY))
                    }
                    continuation.invokeOnCancellation {
                        if (token == generation) {
                            runCatching { planner.cancelRoute() }
                        }
                    }
                }
            } ?: EvRoadLeg.Unavailable(EvRoadUnavailable.TIMEOUT)
        }
}

private fun NativeRouteResult.toDomain(): EvRoadLeg = when (this) {
    is NativeRouteResult.Ready -> {
        val distance = preview.distanceMeters
        if (!distance.isFinite() || distance <= 0.0) {
            EvRoadLeg.Unavailable(EvRoadUnavailable.NO_ROUTE)
        } else {
            EvRoadLeg.Ready(
                distanceMeters = kotlin.math.round(distance).toLong().coerceAtLeast(1),
                durationSeconds = preview.seconds.toLong().takeIf { it > 0 },
                warnings = preview.warnings,
                geometry = preview.geometry,
            )
        }
    }
    is NativeRouteResult.Failed -> EvRoadLeg.Unavailable(
        if (missingMaps.isNotEmpty()) EvRoadUnavailable.MISSING_MAPS else EvRoadUnavailable.NO_ROUTE,
    )
}
