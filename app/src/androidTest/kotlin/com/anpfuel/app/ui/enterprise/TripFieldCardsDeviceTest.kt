package com.anpfuel.app.ui.enterprise

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.anpfuel.app.ui.enterprise.trip.TripFieldCards
import com.anpfuel.app.ui.map.MapRouteTarget
import com.anpfuel.application.enterprise.agreement.FieldVoucherNotice
import com.anpfuel.application.enterprise.management.FieldManagementAdvisory
import com.anpfuel.application.enterprise.trip.TripFieldReference
import com.anpfuel.domain.enterprise.route.EnterpriseRoutePlan
import com.anpfuel.domain.enterprise.route.GeoPointE7
import com.anpfuel.domain.enterprise.route.RouteAvoidOptions
import com.anpfuel.domain.enterprise.route.RoutePlanOwnerKind
import com.anpfuel.domain.enterprise.route.RouteWaypoint
import com.anpfuel.domain.enterprise.route.RouteWaypointSource
import com.anpfuel.domain.enterprise.trip.TripLocalState
import com.anpfuel.domain.routing.RoadAvoidance
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test

class TripFieldCardsDeviceTest {
    @get:Rule val compose = createComposeRule()

    @Test
    fun scopedTripCardsShowTheirFactsAndHandOffTheSelectedStop() {
        val trip = "00000000-0000-4000-8000-000000000001"
        val plan = EnterpriseRoutePlan.create(
            id = "00000000-0000-4000-8000-000000000002", revision = 1,
            ownerKind = RoutePlanOwnerKind.TRIP, ownerId = trip,
            waypoints = listOf(
                RouteWaypoint(1, "Departure", RouteWaypointSource.COORDINATES, point = GeoPointE7(-150000000, -560000000)),
                RouteWaypoint(2, "Checked stop", RouteWaypointSource.COORDINATES, point = GeoPointE7(-151000000, -561000000)),
                RouteWaypoint(3, "Private note", RouteWaypointSource.TEXT_ONLY),
            ),
            options = RouteAvoidOptions(avoidToll = true),
            updatedAt = Instant.parse("2026-11-01T00:00:00Z"),
        )
        val reference = TripFieldReference(
            trip, plan, FieldManagementAdvisory(0, 1, 4, 2, 0),
            FieldVoucherNotice("00000000-0000-4000-8000-000000000003", "DIESEL_S10", "L",
                20_000, 12_000, Instant.now().plusSeconds(300).epochSecond, "ISSUED"),
        )
        var target: MapRouteTarget? = null
        compose.setContent {
            MaterialTheme {
                Column(Modifier.verticalScroll(rememberScrollState())) {
                    TripFieldCards(reference, TripLocalState.IN_PROGRESS) { target = it }
                }
            }
        }
        compose.onNodeWithTag("enterprise-trip-route-card").assertExists()
        compose.onNodeWithTag("enterprise-mgmt-compliance-due-soon").assertExists()
        compose.onNodeWithTag("enterprise-mgmt-maint-requests").assertExists()
        compose.onNodeWithTag("enterprise-station-agreement-card").assertExists()
        compose.onNodeWithTag("enterprise-route-stop-3").assertDoesNotExist()
        compose.onNodeWithTag("enterprise-route-stop-2").performScrollTo().performClick()
        compose.runOnIdle {
            val selected = target as MapRouteTarget.TripStop
            assertEquals(setOf(RoadAvoidance.TOLL), selected.avoid)
            assertEquals(-15.1, selected.point.latitude, 0.000001)
            assertEquals(-56.1, selected.point.longitude, 0.000001)
        }
    }
}
