package com.anpfuel.application.enterprise.route

import com.anpfuel.domain.enterprise.route.EnterpriseRouteComputation
import com.anpfuel.domain.enterprise.route.EnterpriseRoutePlan
import com.anpfuel.domain.enterprise.route.EnterpriseVehicleProfile
import com.anpfuel.domain.enterprise.route.GeoPointE7
import com.anpfuel.domain.enterprise.route.GuidanceDegradedState
import com.anpfuel.domain.enterprise.route.LocationConsumer
import com.anpfuel.domain.enterprise.route.LocationLeaseCoordinator
import com.anpfuel.domain.enterprise.route.PlannedDistanceSource
import com.anpfuel.domain.enterprise.route.RouteAvoidOptions
import com.anpfuel.domain.enterprise.route.RouteComputationKind
import com.anpfuel.domain.enterprise.route.RoutePlanOwnerKind
import com.anpfuel.domain.enterprise.route.RouteWaypoint
import com.anpfuel.domain.enterprise.route.RouteWaypointSource
import com.anpfuel.domain.enterprise.route.SharedLocationFix
import com.anpfuel.domain.enterprise.tracking.TrackingSessionState
import com.anpfuel.domain.enterprise.trip.TripLocalState
import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class EnterpriseTripNavigationTest {
    private val t0 = Instant.parse("2026-11-18T10:00:00Z")
    private val idPlan = "11111111-1111-4111-8111-111111111111"
    private val idTrip = "22222222-2222-4222-8222-222222222222"
    private val idPlace = "33333333-3333-4333-8333-333333333333"
    private val idStation = "44444444-4444-4444-8444-444444444444"
    private val idComp = "55555555-5555-4555-8555-555555555555"

    private class FakeCarRouter(
        var outcomeDistanceMetres: Long = 120_000L,
        var degradedOverride: GuidanceDegradedState? = null,
        val engineName: String = "comaps-car-test",
    ) : EnterpriseRoutingPort {
        override fun computeRoute(
            ownerKind: RoutePlanOwnerKind,
            ownerId: String,
            planRevision: Long,
            kind: RouteComputationKind,
            waypoints: List<RouteWaypoint>,
            options: RouteAvoidOptions,
            computedAt: Instant,
        ): EnterpriseRoutingOutcome {
            degradedOverride?.let { return EnterpriseRoutingOutcome.Degraded(it) }
            return EnterpriseRoutingOutcome.Computed(
                EnterpriseRouteComputation(
                    id = "66666666-6666-4666-8666-666666666666",
                    ownerKind = ownerKind,
                    ownerId = ownerId,
                    planRevision = planRevision,
                    engineId = engineName,
                    mapDataVersion = "2026.11",
                    kind = kind,
                    options = options,
                    distanceMetres = outcomeDistanceMetres,
                    durationSeconds = 5_400L,
                    line = waypoints.mapNotNull { it.point },
                    computedAt = computedAt,
                ),
            )
        }
    }

    @Test
    fun `VAL-29 trip-aware navigation lifecycle with off-route recalculation, arrival confirm and pending revision lock`() {
        val origin = GeoPointE7(-156_000_000, -561_000_000)
        val stop1 = GeoPointE7(-158_000_000, -562_000_000)
        val dest = GeoPointE7(-166_000_000, -492_000_000)

        val initialComp = EnterpriseRouteComputation(
            id = idComp,
            ownerKind = RoutePlanOwnerKind.TRIP,
            ownerId = idTrip,
            planRevision = 1L,
            engineId = "comaps-car-test",
            mapDataVersion = "2026.11",
            kind = RouteComputationKind.PLANNING,
            options = RouteAvoidOptions(avoidToll = true),
            distanceMetres = 120_000L,
            durationSeconds = 5_400L,
            line = listOf(origin, stop1, dest),
            computedAt = t0,
        )
        val plan = EnterpriseRoutePlan.create(
            id = idPlan,
            revision = 1L,
            ownerKind = RoutePlanOwnerKind.TRIP,
            ownerId = idTrip,
            waypoints = listOf(
                RouteWaypoint(1, "Garagem", RouteWaypointSource.ORGANIZATION_PLACE, placeId = idPlace, point = origin),
                RouteWaypoint(2, "Posto Parceiro", RouteWaypointSource.STATION_CATALOG, stationId = idStation, point = stop1),
                RouteWaypoint(3, "Destino Final", RouteWaypointSource.COORDINATES, point = dest),
            ),
            options = RouteAvoidOptions(avoidToll = true),
            plannedDistanceMetres = 120_000L,
            plannedDistanceSource = PlannedDistanceSource.ENGINE,
            heavyAssetNotice = true,
            latestComputation = initialComp,
            updatedAt = t0,
        )

        val router = FakeCarRouter(outcomeDistanceMetres = 125_000L)
        assertFalse(router.isServerRoutingUsed)
        val leases = LocationLeaseCoordinator()
        leases.acquire(LocationConsumer.TRACKING)

        // Start trip-aware navigation while tracking is OFF -> guidance works, sends nothing.
        var state = TripAwareNavigationReducer.start(
            tripId = idTrip,
            tripState = TripLocalState.IN_PROGRESS,
            plan = plan,
            vehicleProfile = EnterpriseVehicleProfile.HEAVY_TRUCK,
            routing = router,
            leases = leases,
            trackingState = TrackingSessionState.NOT_STARTED,
            now = t0,
        )
        assertEquals(TripNavigationStatus.NAVIGATING, state.status)
        assertTrue(state.heavyVehicleWarning.required)
        assertFalse(state.isTrackingSending)
        assertEquals(1, leases.hardwareStartCount)

        // Manager pushes revision 2 during IN_PROGRESS trip -> queued, never replaces active route mid-motion.
        val rev2Plan = EnterpriseRoutePlan.create(
            id = idPlan,
            revision = 2L,
            ownerKind = RoutePlanOwnerKind.TRIP,
            ownerId = idTrip,
            waypoints = listOf(
                RouteWaypoint(1, "Garagem", RouteWaypointSource.ORGANIZATION_PLACE, placeId = idPlace, point = origin),
                RouteWaypoint(2, "Cliente Texto", RouteWaypointSource.TEXT_ONLY),
                RouteWaypoint(3, "Destino Final", RouteWaypointSource.COORDINATES, point = dest),
            ),
            updatedAt = t0.plusSeconds(60),
        )
        state = TripAwareNavigationReducer.onSyncPlanRevision(state, rev2Plan, router, t0.plusSeconds(60))
        assertEquals(1L, state.activePlan.revision)
        assertNotNull(state.pendingPlanRevision)

        // Off-route recalculation updates latestComputation and recalculation count while preserving 120,000m plan distance.
        state = TripAwareNavigationReducer.onLocationFix(
            state = state,
            fix = SharedLocationFix(t0.plusSeconds(120), GeoPointE7(-157_000_000, -561_500_000), 10, speedCmPerSecond = 800),
            isOffRoute = true,
            routing = router,
            now = t0.plusSeconds(120),
        )
        assertEquals(1, state.recalculationCount)
        assertEquals(120_000L, state.originalPlannedDistanceMetres)
        assertEquals(125_000L, state.activePlan.latestComputation?.distanceMetres)

        // Enter stop1 arrival radius -> proposes "Cheguei" without auto-completing stop or trip.
        state = TripAwareNavigationReducer.onLocationFix(
            state = state,
            fix = SharedLocationFix(t0.plusSeconds(240), stop1, 8, speedCmPerSecond = 50),
            isOffRoute = false,
            routing = router,
            now = t0.plusSeconds(240),
        )
        assertNotNull(state.arrivalProposal)
        assertEquals(2, state.arrivalProposal!!.waypointSequence)
        assertTrue(state.completedStopSequences.isEmpty())

        // Explicit driver confirmation advances to destination and marks stop 2 done.
        state = TripAwareNavigationReducer.confirmArrival(state)
        assertNull(state.arrivalProposal)
        assertEquals(setOf(2), state.completedStopSequences)
        assertEquals(2, state.currentWaypointIndex)

        // When stopped, applying pending revision 2 succeeds and surfaces TEXT_ONLY stop prompt.
        state = TripAwareNavigationReducer.applyPendingRevision(state, stoppedOrConfirmed = true, routing = router, now = t0.plusSeconds(300))
        assertEquals(2L, state.activePlan.revision)
        assertNull(state.pendingPlanRevision)
        assertNotNull(state.textStopPrompt)
        assertEquals("Cliente Texto", state.textStopPrompt!!.label)

        // Pausing the trip releases GUIDANCE lease only while keeping TRACKING subscription alive.
        state = TripAwareNavigationReducer.onTripStateChanged(state, TripLocalState.PAUSED, TrackingSessionState.SUSPENDED, leases)
        assertEquals(TripNavigationStatus.PAUSED, state.status)
        assertEquals(setOf(LocationConsumer.TRACKING), leases.activeConsumers)
        assertTrue(leases.hasActiveHardwareSubscription)
    }

    @Test
    fun `EMP-ROUTE-010 routing port accepts alternate engine implementation and surfaces degraded states`() {
        val altEngine = FakeCarRouter(degradedOverride = GuidanceDegradedState.MAP_REGION_MISSING, engineName = "alt-routing-v2")
        val leases = LocationLeaseCoordinator()
        val plan = EnterpriseRoutePlan.create(
            id = idPlan,
            revision = 1L,
            ownerKind = RoutePlanOwnerKind.MISSION,
            ownerId = idTrip,
            waypoints = listOf(
                RouteWaypoint(1, "A", RouteWaypointSource.COORDINATES, point = GeoPointE7(-156_000_000, -561_000_000)),
                RouteWaypoint(2, "B", RouteWaypointSource.COORDINATES, point = GeoPointE7(-157_000_000, -562_000_000)),
            ),
            updatedAt = t0,
        )
        val state = TripAwareNavigationReducer.start(
            tripId = idTrip,
            tripState = TripLocalState.IN_PROGRESS,
            plan = plan,
            vehicleProfile = EnterpriseVehicleProfile.LIGHT_PASSENGER,
            routing = altEngine,
            leases = leases,
            trackingState = null,
            now = t0,
        )
        assertEquals(GuidanceDegradedState.MAP_REGION_MISSING, state.degradedState)
        assertTrue(state.degradedState!!.keepsTripActive)
        assertEquals(TripNavigationStatus.NAVIGATING, state.status)
    }
}
