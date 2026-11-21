package com.anpfuel.domain.enterprise.route

import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class EnterpriseRouteDomainTest {
    private val t0 = Instant.parse("2026-11-18T10:00:00Z")
    private val id1 = "11111111-1111-4111-8111-111111111111"
    private val id2 = "22222222-2222-4222-8222-222222222222"
    private val id3 = "33333333-3333-4333-8333-333333333333"
    private val id4 = "44444444-4444-4444-8444-444444444444"

    @Test
    fun `route plan enforces 102 waypoint ceiling and redacts strings`() {
        val waypoints102 = (1..102).map { i ->
            RouteWaypoint(
                sequence = i,
                label = "Stop $i",
                source = RouteWaypointSource.COORDINATES,
                point = GeoPointE7(-156_000_000 + i * 1000, -561_000_000),
            )
        }
        val plan = EnterpriseRoutePlan.create(
            id = id1,
            revision = 1L,
            ownerKind = RoutePlanOwnerKind.TRIP,
            ownerId = id2,
            waypoints = waypoints102,
            heavyAssetNotice = true,
            updatedAt = t0,
        )
        assertEquals(102, plan.waypoints.size)
        assertFalse(plan.toString().contains("Stop 1"))
        assertFalse(plan.toString().contains("156000000"))

        val waypoints103 = waypoints102 + RouteWaypoint(
            sequence = 103,
            label = "Overflow",
            source = RouteWaypointSource.COORDINATES,
            point = GeoPointE7(-155_000_000, -561_000_000),
        )
        val err = assertThrows(RouteDomainException::class.java) {
            EnterpriseRoutePlan.create(
                id = id1,
                revision = 1L,
                ownerKind = RoutePlanOwnerKind.TRIP,
                ownerId = id2,
                waypoints = waypoints103,
                updatedAt = t0,
            )
        }
        assertEquals(RouteFailure.POINT_LIMIT, err.failure)
    }

    @Test
    fun `route book canonical round trip and partition binding`() {
        val comp = EnterpriseRouteComputation(
            id = id3,
            ownerKind = RoutePlanOwnerKind.MISSION,
            ownerId = id2,
            planRevision = 1L,
            engineId = "comaps-car-v1",
            mapDataVersion = "2026.11",
            kind = RouteComputationKind.PLANNING,
            options = RouteAvoidOptions(avoidToll = true),
            distanceMetres = 181_000L,
            durationSeconds = 7_200L,
            line = listOf(GeoPointE7(-156_000_000, -561_000_000), GeoPointE7(-166_000_000, -492_000_000)),
            computedAt = t0,
        )
        val plan = EnterpriseRoutePlan.create(
            id = id1,
            revision = 1L,
            ownerKind = RoutePlanOwnerKind.MISSION,
            ownerId = id2,
            waypoints = listOf(
                RouteWaypoint(1, "Base", RouteWaypointSource.COORDINATES, point = GeoPointE7(-156_000_000, -561_000_000)),
                RouteWaypoint(2, "Cliente sem coord", RouteWaypointSource.TEXT_ONLY),
                RouteWaypoint(3, "Filial", RouteWaypointSource.ORGANIZATION_PLACE, placeId = id4, point = GeoPointE7(-166_000_000, -492_000_000)),
            ),
            options = RouteAvoidOptions(avoidToll = true),
            plannedDistanceMetres = 181_000L,
            plannedDistanceSource = PlannedDistanceSource.ENGINE,
            heavyAssetNotice = true,
            latestComputation = comp,
            updatedAt = t0,
        )
        assertTrue(plan.hasUnresolvedTextStops)
        assertEquals(2, plan.routableWaypoints.size)

        val book = EnterpriseRouteBook.empty(id1, id4).withPlan(plan)
        val decoded = EnterpriseRouteBook.decode(book.canonicalBytes).requireBinding(id1, id4)
        assertEquals(book, decoded)

        val foreign = assertThrows(RouteDomainException::class.java) {
            decoded.requireBinding(id2, id4)
        }
        assertEquals(RouteFailure.FOREIGN_BINDING, foreign.failure)
    }

    @Test
    fun `location lease coordinator shares one hardware subscription across consumers`() {
        val coord = LocationLeaseCoordinator()
        val t1 = coord.acquire(LocationConsumer.TRACKING)
        assertTrue(t1.startHardwareSubscription)
        assertEquals(1, coord.hardwareStartCount)

        val t2 = coord.acquire(LocationConsumer.GUIDANCE)
        assertFalse(t2.startHardwareSubscription)
        assertEquals(1, coord.hardwareStartCount)
        assertEquals(setOf(LocationConsumer.TRACKING, LocationConsumer.GUIDANCE), t2.activeConsumers)

        val fix = SharedLocationFix(t0, GeoPointE7(-156_000_000, -561_000_000), 8, speedCmPerSecond = 200)
        assertEquals(setOf(LocationConsumer.TRACKING, LocationConsumer.GUIDANCE), coord.fanOut(fix).keys)
        assertFalse(fix.toString().contains("156000000"))

        val t3 = coord.release(LocationConsumer.GUIDANCE)
        assertFalse(t3.stopHardwareSubscription)
        assertTrue(coord.hasActiveHardwareSubscription)
        assertEquals(setOf(LocationConsumer.TRACKING), coord.fanOut(fix).keys)

        val t4 = coord.release(LocationConsumer.TRACKING)
        assertTrue(t4.stopHardwareSubscription)
        assertFalse(coord.hasActiveHardwareSubscription)
        assertTrue(coord.fanOut(fix).isEmpty())
    }

    @Test
    fun `driving mode locks detailed entry at 1_5 m_s and limits quick actions to 4`() {
        assertFalse(DrivingModePolicy.isDriving(null))
        assertFalse(DrivingModePolicy.isDriving(149))
        assertTrue(DrivingModePolicy.isDriving(150))
        assertTrue(DrivingModePolicy.isDriving(500))

        assertTrue(DrivingModePolicy.isDetailedInteractionAllowed(149, DetailedInteractionKind.FREE_TEXT_INPUT))
        assertFalse(DrivingModePolicy.isDetailedInteractionAllowed(150, DetailedInteractionKind.FREE_TEXT_INPUT))
        assertFalse(DrivingModePolicy.isDetailedInteractionAllowed(200, DetailedInteractionKind.WAYPOINT_REORDER))

        val actions = DrivingModePolicy.quickActionsFor(200, DrivingQuickAction.entries + DrivingQuickAction.CONFIRM_ARRIVAL)
        assertTrue(actions.size <= DrivingModePolicy.MAX_QUICK_ACTIONS)

        val warning = HeavyVehicleWarning.forProfile(EnterpriseVehicleProfile.HEAVY_TRUCK, noticeFlag = false)
        assertTrue(warning.required)
        assertEquals("CAR", warning.profileUsed)
        assertTrue(warning.doesNotEnforceRestrictions)
        assertTrue(GuidanceDegradedState.MAP_REGION_MISSING.keepsTripActive)
        assertTrue(GuidanceDegradedState.VOICE_UNAVAILABLE_VISUAL_ONLY.allowsExternalHandoff)
    }
}
