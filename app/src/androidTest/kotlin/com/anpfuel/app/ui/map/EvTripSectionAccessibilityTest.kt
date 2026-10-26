package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.assertHasClickAction
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import androidx.lifecycle.SavedStateHandle
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.application.usecase.ev.PlanEvTripUseCase
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.ev.*
import com.anpfuel.domain.map.saved.GeoPoint
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.routing.RoadAvoidance
import com.anpfuel.domain.routing.RoutePoint
import com.anpfuel.domain.valueobject.BatteryCapacity
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.Propulsion
import com.anpfuel.domain.valueobject.VehiclePriceSource
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import androidx.test.ext.junit.runners.AndroidJUnit4

/** MAP-28 accessibility layout at 200% font and a narrow width, over the real section. */
@RunWith(AndroidJUnit4::class)
class EvTripSectionAccessibilityTest {
    @get:Rule val compose = createComposeRule()

    private val origin = RoutePoint(-25.0, -49.0, "origin")
    private val destination = RoutePoint(-24.1, -49.0, "destination")
    private val primaryPoint = RoutePoint(-24.55, -49.0, "primary")
    private val altPoint = RoutePoint(-24.55, -48.995, "alternative")

    private class FakeVehicles : VehicleRepository {
        private val vehicle = Vehicle.create(
            displayName = "Synthetic EV",
            tankCapacity = null,
            fuelProduct = null,
            priceSource = VehiclePriceSource.cheapest(),
            propulsion = Propulsion.BEV,
            batteryCapacity = BatteryCapacity.ofWh(60_000),
            chargeConnector = ChargingConnector.CCS2,
            id = DomainId.generate(),
        )
        override suspend fun listAll() = listOf(vehicle)
        override suspend fun findById(id: DomainId) = vehicle.takeIf { it.id == id }
        override suspend fun save(vehicle: Vehicle) = Unit
        override suspend fun delete(id: DomainId) = Unit
        override suspend fun count() = 1
    }

    private inner class FakeRouter : EvRoadRouter {
        override suspend fun leg(points: List<RoutePoint>, avoid: Set<RoadAvoidance>, language: String): EvRoadLeg {
            val key = points.joinToString(">") { it.label }
            return when (key) {
                "origin>destination" -> ready(100_000, listOf(origin, primaryPoint, destination))
                "origin>primary" -> ready(50_000)
                "primary>destination" -> ready(50_000)
                "primary>alternative" -> ready(10_000)
                else -> EvRoadLeg.Unavailable(EvRoadUnavailable.NO_ROUTE)
            }
        }
        private fun ready(metres: Long, geometry: List<RoutePoint> = emptyList()) =
            EvRoadLeg.Ready(metres, metres / 50, false, geometry)
    }

    private fun candidate(site: String, point: RoutePoint) = EvChargingCandidate(
        candidateId = "c-$site", siteId = site, siteName = site, equipmentId = "e-$site",
        connectorId = "k-$site", connector = ChargingConnector.CCS2, current = ChargingCurrent.DC,
        powerW = 100_000, access = EvAccess.PUBLIC, hours = EvHours.Unknown,
        tariff = EvTariff.Unknown, provenance = EvProvenance.STATIC_REFERENCE,
        sourceRevision = "synthetic-accessibility-v1", observedAt = null,
        point = GeoPoint.fromDouble(point.latitude, point.longitude),
    )

    @Test
    fun fieldsAndPlanRemainUsableAtTwoHundredPercentFontOnANarrowScreen() {
        val source = object : EvCandidateSource {
            override suspend fun candidates(query: EvCandidateQuery) = EvCandidateResult(
                listOf(candidate("primary", primaryPoint), candidate("alternative", altPoint)),
                EvCoverage.PRESENT,
            )
        }
        val viewModel = EvTripViewModel(
            PlanEvTripUseCase(FakeRouter(), source),
            FakeVehicles(),
            SavedStateHandle(),
        )
        compose.setContent {
            AnpFuelTheme {
                CompositionLocalProvider(LocalDensity provides Density(density = 1f, fontScale = 2f)) {
                    Box(Modifier.fillMaxSize()) {
                        Column(
                            Modifier.width(320.dp).verticalScroll(rememberScrollState()).testTag("ev-accessibility-scroll"),
                        ) {
                            EvTripSection(
                                origin = origin,
                                waypoints = emptyList(),
                                destination = destination,
                                avoid = emptySet(),
                                savedMessage = null,
                                onSavePlan = null,
                                onDownloads = {},
                                viewModel = viewModel,
                            )
                        }
                    }
                }
            }
        }
        compose.waitUntil(10_000) {
            compose.onAllNodesWithTag("ev-vehicle-0").fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithTag("ev-vehicle-0").performScrollTo().performClick()
        compose.onNodeWithTag("ev-soc").performScrollTo().performTextReplacement("30")
        compose.onNodeWithTag("ev-reserve").performScrollTo().performTextReplacement("10")
        compose.onNodeWithTag("ev-consumption").performScrollTo().performTextReplacement("180")
        compose.onNodeWithTag("ev-target").performScrollTo().performTextReplacement("80")
        compose.onNodeWithTag("ev-plan").performScrollTo().assertHasClickAction().performClick()
        compose.waitUntil(30_000) {
            compose.onAllNodesWithTag("ev-stop-0").fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithTag("ev-stop-0").performScrollTo().assertIsDisplayed()
        compose.onNodeWithTag("ev-alternative").performScrollTo().assertIsDisplayed()
        compose.onNodeWithTag("ev-fail-primary").performScrollTo().assertHasClickAction().assertIsDisplayed()
    }
}
