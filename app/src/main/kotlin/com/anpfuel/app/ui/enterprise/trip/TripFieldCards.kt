package com.anpfuel.app.ui.enterprise.trip

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import com.anpfuel.app.ui.map.MapRouteTarget
import com.anpfuel.application.enterprise.route.TripNavigationSessionState
import com.anpfuel.application.enterprise.trip.TripFieldReference
import com.anpfuel.domain.enterprise.route.HeavyVehicleWarning
import com.anpfuel.domain.enterprise.trip.TripLocalState
import com.anpfuel.domain.routing.RoadAvoidance
import com.anpfuel.domain.routing.RoutePoint
import java.time.Instant
import kotlinx.coroutines.delay

@Composable
internal fun TripFieldCards(reference: TripFieldReference, tripState: TripLocalState, onNavigate: (MapRouteTarget) -> Unit) {
    reference.plan?.let { plan ->
        val avoid = remember(plan.options) {
            buildSet {
                if (plan.options.avoidToll) add(RoadAvoidance.TOLL)
                if (plan.options.avoidMotorway) add(RoadAvoidance.MOTORWAY)
                if (plan.options.avoidFerry) add(RoadAvoidance.FERRY)
                if (plan.options.avoidUnpaved) add(RoadAvoidance.UNPAVED)
            }
        }
        TripRouteCard(
            state = TripNavigationSessionState(
                tripId = reference.tripId,
                tripState = tripState,
                activePlan = plan,
                heavyVehicleWarning = HeavyVehicleWarning(required = plan.heavyAssetNotice),
            ),
            speedCmPerSecond = null,
            onNavigateStop = { stop ->
                stop.point?.let { point ->
                    onNavigate(MapRouteTarget.TripStop(RoutePoint(
                        point.latE7 / 10_000_000.0, point.lonE7 / 10_000_000.0, stop.label,
                        approximateEntrance = false,
                    ), avoid))
                }
            },
        )
    }
    TripFieldManagementCard(reference.management, speedCmPerSecond = null)
    val voucher = reference.voucher
    var now by remember(voucher) { mutableLongStateOf(Instant.now().epochSecond) }
    LaunchedEffect(voucher) {
        val until = voucher?.validUntilEpochSeconds ?: return@LaunchedEffect
        while (now < until) {
            delay((until - now).coerceAtMost(60L) * 1_000L)
            now = Instant.now().epochSecond
        }
    }
    StationAgreementCard(voucher, now)
}
