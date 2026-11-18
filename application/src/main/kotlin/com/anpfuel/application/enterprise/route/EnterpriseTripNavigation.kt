package com.anpfuel.application.enterprise.route

import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.domain.enterprise.route.EnterpriseRouteBook
import com.anpfuel.domain.enterprise.route.EnterpriseRouteComputation
import com.anpfuel.domain.enterprise.route.EnterpriseRoutePlan
import com.anpfuel.domain.enterprise.route.EnterpriseVehicleProfile
import com.anpfuel.domain.enterprise.route.GeoPointE7
import com.anpfuel.domain.enterprise.route.GuidanceDegradedState
import com.anpfuel.domain.enterprise.route.HeavyVehicleWarning
import com.anpfuel.domain.enterprise.route.LocationConsumer
import com.anpfuel.domain.enterprise.route.LocationLeaseCoordinator
import com.anpfuel.domain.enterprise.route.RouteAvoidOptions
import com.anpfuel.domain.enterprise.route.RouteComputationKind
import com.anpfuel.domain.enterprise.route.RoutePlanOwnerKind
import com.anpfuel.domain.enterprise.route.RouteWaypoint
import com.anpfuel.domain.enterprise.route.SharedLocationFix
import com.anpfuel.domain.enterprise.tracking.TrackingSessionState
import com.anpfuel.domain.enterprise.trip.TripLocalState
import java.time.Instant
import kotlin.math.atan2
import kotlin.math.cos
import kotlin.math.sin
import kotlin.math.sqrt

/*
 * Trip-aware turn-by-turn navigation coordination, routing port boundary, and
 * protected route book store contract (EMP-ROUTE-006..010, BR-ROUTE-011..018,
 * VAL-29). Keeps the application layer independent of CoMaps/OSRM internals.
 */

sealed interface EnterpriseRoutingOutcome {
    class Computed(val computation: EnterpriseRouteComputation) : EnterpriseRoutingOutcome
    class Degraded(val reason: GuidanceDegradedState) : EnterpriseRoutingOutcome
}

/** EMP-ROUTE-010: replaceable routing engine boundary; server routing stays disabled in F5. */
interface EnterpriseRoutingPort {
    val isServerRoutingUsed: Boolean get() = false

    fun computeRoute(
        ownerKind: RoutePlanOwnerKind,
        ownerId: String,
        planRevision: Long,
        kind: RouteComputationKind,
        waypoints: List<RouteWaypoint>,
        options: RouteAvoidOptions,
        computedAt: Instant,
    ): EnterpriseRoutingOutcome
}

object EnterpriseRouteProducers {
    const val BOOK_RECORD_ID = "f05e0b00-0000-4000-8000-0000000000b1"
}

interface EnterpriseRouteStore {
    suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseRouteBook?
    suspend fun savePlan(attempt: EnterpriseQueueAttempt, plan: EnterpriseRoutePlan)
}

data class ArrivalProposal(
    val waypointSequence: Int,
    val waypointLabel: String,
    val proposedAt: Instant,
)

data class RecalculationRecord(
    val at: Instant,
    val distanceMetres: Long,
    val durationSeconds: Long,
)

data class TextStopPrompt(
    val waypointSequence: Int,
    val label: String,
)

enum class TripNavigationStatus {
    IDLE, NAVIGATING, PAUSED, ENDED
}

data class TripNavigationSessionState(
    val tripId: String,
    val tripState: TripLocalState,
    val activePlan: EnterpriseRoutePlan,
    val pendingPlanRevision: EnterpriseRoutePlan? = null,
    val currentWaypointIndex: Int = 0,
    val completedStopSequences: Set<Int> = emptySet(),
    val arrivalProposal: ArrivalProposal? = null,
    val textStopPrompt: TextStopPrompt? = null,
    val heavyVehicleWarning: HeavyVehicleWarning,
    val degradedState: GuidanceDegradedState? = null,
    val recalculations: List<RecalculationRecord> = emptyList(),
    val status: TripNavigationStatus = TripNavigationStatus.IDLE,
    val isTrackingSending: Boolean = false,
    val attribution: String = EnterpriseRouteComputation.OSM_ATTRIBUTION,
) {
    val recalculationCount: Int get() = recalculations.size
    val originalPlannedDistanceMetres: Long? get() = activePlan.plannedDistanceMetres
}

object TripAwareNavigationReducer {
    const val DEFAULT_ARRIVAL_RADIUS_METRES = 150

    fun start(
        tripId: String,
        tripState: TripLocalState,
        plan: EnterpriseRoutePlan,
        vehicleProfile: EnterpriseVehicleProfile,
        routing: EnterpriseRoutingPort,
        leases: LocationLeaseCoordinator,
        trackingState: TrackingSessionState?,
        now: Instant,
    ): TripNavigationSessionState {
        val heavy = HeavyVehicleWarning.forProfile(vehicleProfile, plan.heavyAssetNotice)
        if (tripState != TripLocalState.IN_PROGRESS) {
            leases.release(LocationConsumer.GUIDANCE)
            val status = if (tripState == TripLocalState.PAUSED) TripNavigationStatus.PAUSED else TripNavigationStatus.ENDED
            return TripNavigationSessionState(
                tripId = tripId,
                tripState = tripState,
                activePlan = plan,
                heavyVehicleWarning = heavy,
                status = status,
                isTrackingSending = false,
            )
        }

        leases.acquire(LocationConsumer.GUIDANCE)
        val firstTextOnly = plan.waypoints.firstOrNull { !it.resolvable }
        val routable = plan.routableWaypoints
        val (updatedPlan, degraded) = if (routable.size >= 2) {
            when (val res = routing.computeRoute(
                ownerKind = plan.ownerKind,
                ownerId = plan.ownerId,
                planRevision = plan.revision,
                kind = RouteComputationKind.DEPARTURE,
                waypoints = routable,
                options = plan.options,
                computedAt = now,
            )) {
                is EnterpriseRoutingOutcome.Computed -> plan.withComputation(res.computation) to null
                is EnterpriseRoutingOutcome.Degraded -> plan to res.reason
            }
        } else {
            plan to if (firstTextOnly != null) null else GuidanceDegradedState.ROUTE_NOT_FOUND
        }

        val initialIndex = if (routable.size >= 2) 1 else 0
        return TripNavigationSessionState(
            tripId = tripId,
            tripState = tripState,
            activePlan = updatedPlan,
            currentWaypointIndex = initialIndex,
            textStopPrompt = firstTextOnly?.let { TextStopPrompt(it.sequence, it.label) },
            heavyVehicleWarning = heavy,
            degradedState = degraded,
            status = TripNavigationStatus.NAVIGATING,
            isTrackingSending = trackingState == TrackingSessionState.RECORDING,
            attribution = updatedPlan.latestComputation?.attribution ?: EnterpriseRouteComputation.OSM_ATTRIBUTION,
        )
    }

    /**
     * Blocks replacing the active route while the trip is IN_PROGRESS (BR-ROUTE-013,
     * VAL-29): stores the newer revision as [TripNavigationSessionState.pendingPlanRevision]
     * until stopped/confirmed.
     */
    fun onSyncPlanRevision(
        state: TripNavigationSessionState,
        updated: EnterpriseRoutePlan,
        routing: EnterpriseRoutingPort,
        now: Instant,
    ): TripNavigationSessionState {
        if (updated.revision <= state.activePlan.revision) return state
        if (state.tripState == TripLocalState.IN_PROGRESS && state.status == TripNavigationStatus.NAVIGATING) {
            return state.copy(pendingPlanRevision = updated)
        }
        return applyUpdatedPlan(state, updated, routing, now)
    }

    fun applyPendingRevision(
        state: TripNavigationSessionState,
        stoppedOrConfirmed: Boolean,
        routing: EnterpriseRoutingPort,
        now: Instant,
    ): TripNavigationSessionState {
        val pending = state.pendingPlanRevision ?: return state
        if (!stoppedOrConfirmed) return state
        return applyUpdatedPlan(state.copy(pendingPlanRevision = null), pending, routing, now)
    }

    private fun applyUpdatedPlan(
        state: TripNavigationSessionState,
        updated: EnterpriseRoutePlan,
        routing: EnterpriseRoutingPort,
        now: Instant,
    ): TripNavigationSessionState {
        val routable = updated.routableWaypoints
        val (resolvedPlan, degraded) = if (routable.size >= 2) {
            when (val res = routing.computeRoute(
                ownerKind = updated.ownerKind,
                ownerId = updated.ownerId,
                planRevision = updated.revision,
                kind = RouteComputationKind.PLANNING,
                waypoints = routable,
                options = updated.options,
                computedAt = now,
            )) {
                is EnterpriseRoutingOutcome.Computed -> updated.withComputation(res.computation) to null
                is EnterpriseRoutingOutcome.Degraded -> updated to res.reason
            }
        } else {
            updated to state.degradedState
        }
        return state.copy(
            activePlan = resolvedPlan,
            pendingPlanRevision = null,
            textStopPrompt = resolvedPlan.waypoints.firstOrNull { !it.resolvable }?.let { TextStopPrompt(it.sequence, it.label) },
            degradedState = degraded,
        )
    }

    fun onLocationFix(
        state: TripNavigationSessionState,
        fix: SharedLocationFix,
        isOffRoute: Boolean,
        arrivalRadiusMetres: Int = DEFAULT_ARRIVAL_RADIUS_METRES,
        routing: EnterpriseRoutingPort,
        now: Instant,
    ): TripNavigationSessionState {
        if (state.status != TripNavigationStatus.NAVIGATING) return state
        var next = state
        if (isOffRoute) {
            val routable = state.activePlan.routableWaypoints
            if (routable.size >= 2) {
                when (val res = routing.computeRoute(
                    ownerKind = state.activePlan.ownerKind,
                    ownerId = state.activePlan.ownerId,
                    planRevision = state.activePlan.revision,
                    kind = RouteComputationKind.RECALCULATION,
                    waypoints = routable,
                    options = state.activePlan.options,
                    computedAt = now,
                )) {
                    is EnterpriseRoutingOutcome.Computed -> {
                        val comp = res.computation
                        // Preserve original plannedDistanceMetres while recording the recalculation event.
                        val nextPlan = EnterpriseRoutePlan.create(
                            id = state.activePlan.id,
                            revision = state.activePlan.revision,
                            ownerKind = state.activePlan.ownerKind,
                            ownerId = state.activePlan.ownerId,
                            waypoints = state.activePlan.waypoints,
                            options = state.activePlan.options,
                            plannedDistanceMetres = state.activePlan.plannedDistanceMetres,
                            plannedDistanceSource = state.activePlan.plannedDistanceSource,
                            heavyAssetNotice = state.activePlan.heavyAssetNotice,
                            latestComputation = comp,
                            updatedAt = now,
                        )
                        next = next.copy(
                            activePlan = nextPlan,
                            degradedState = null,
                            recalculations = next.recalculations + RecalculationRecord(now, comp.distanceMetres, comp.durationSeconds),
                        )
                    }
                    is EnterpriseRoutingOutcome.Degraded -> next = next.copy(degradedState = res.reason)
                }
            }
        }

        val currentWp = next.activePlan.waypoints.getOrNull(next.currentWaypointIndex)
        val targetPt = currentWp?.point
        if (currentWp != null && targetPt != null &&
            currentWp.sequence !in next.completedStopSequences &&
            distanceMetres(fix.point, targetPt) <= arrivalRadiusMetres
        ) {
            next = next.copy(arrivalProposal = ArrivalProposal(currentWp.sequence, currentWp.label, now))
        }
        return next
    }

    /** Driver explicitly confirms "Cheguei" (BR-ROUTE-017); never auto-finishes the trip. */
    fun confirmArrival(state: TripNavigationSessionState): TripNavigationSessionState {
        val proposal = state.arrivalProposal ?: return state
        val completed = state.completedStopSequences + proposal.waypointSequence
        val nextIndex = (state.currentWaypointIndex + 1).coerceAtMost((state.activePlan.waypoints.size - 1).coerceAtLeast(0))
        return state.copy(
            completedStopSequences = completed,
            currentWaypointIndex = nextIndex,
            arrivalProposal = null,
        )
    }

    fun onTripStateChanged(
        state: TripNavigationSessionState,
        newTripState: TripLocalState,
        trackingState: TrackingSessionState?,
        leases: LocationLeaseCoordinator,
    ): TripNavigationSessionState {
        val newStatus = when (newTripState) {
            TripLocalState.IN_PROGRESS -> TripNavigationStatus.NAVIGATING
            TripLocalState.PAUSED -> {
                leases.release(LocationConsumer.GUIDANCE)
                TripNavigationStatus.PAUSED
            }
            TripLocalState.FINISHED, TripLocalState.CANCELLED, TripLocalState.START_REFUSED -> {
                leases.release(LocationConsumer.GUIDANCE)
                TripNavigationStatus.ENDED
            }
        }
        return state.copy(
            tripState = newTripState,
            status = newStatus,
            isTrackingSending = newTripState == TripLocalState.IN_PROGRESS && trackingState == TrackingSessionState.RECORDING,
        )
    }

    private fun distanceMetres(a: GeoPointE7, b: GeoPointE7): Double {
        val r = 6_371_000.0
        val lat1 = Math.toRadians(a.latE7 / 1e7)
        val lat2 = Math.toRadians(b.latE7 / 1e7)
        val dLat = lat2 - lat1
        val dLon = Math.toRadians((b.lonE7 - a.lonE7) / 1e7)
        val h = sin(dLat / 2) * sin(dLat / 2) + cos(lat1) * cos(lat2) * sin(dLon / 2) * sin(dLon / 2)
        return 2 * r * atan2(sqrt(h), sqrt(1 - h))
    }
}
