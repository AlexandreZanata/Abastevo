package com.anpfuel.application.usecase.ev

import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.ev.*
import com.anpfuel.domain.map.saved.GeoPoint
import com.anpfuel.domain.routing.RoadAvoidance
import com.anpfuel.domain.routing.RoutePoint
import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PlanEvTripUseCaseTest {
    private val origin = RoutePoint(-25.0, -49.0, "origin")
    private val primaryPoint = RoutePoint(-24.55, -49.0, "primary")
    private val destination = RoutePoint(-24.1, -49.0, "destination")
    private val alt10Point = RoutePoint(-24.55, -48.995, "alt10")
    private val alt20Point = RoutePoint(-24.55, -48.99, "alt20")
    private val battery = BatteryInput(capacityWh = 60_000, socBp = 3_000, reserveBp = 1_000)
    private val consumption = ConsumptionInput(consMilliWhPerKm = 180_000)
    private val input = EvTripInput(
        origin = origin,
        destination = destination,
        departureAt = Instant.parse("2026-10-26T12:00:00Z"),
        battery = battery,
        consumption = consumption,
        vehicleConnector = ChargingConnector.CCS2,
        chargeTargetSocBp = 8_000,
        maxRouteCalls = 48,
    )

    private fun ready(
        metres: Long,
        seconds: Long = metres / 50,
        geometry: List<RoutePoint> = emptyList(),
        warnings: Boolean = false,
    ) = EvRoadLeg.Ready(metres, seconds, warnings, geometry)

    private class FakeRouter(private val legs: MutableMap<String, EvRoadLeg> = mutableMapOf()) : EvRoadRouter {
        val calls = mutableListOf<String>()
        fun add(from: RoutePoint, to: RoutePoint, leg: EvRoadLeg) {
            legs["${from.label}>${to.label}"] = leg
        }
        override suspend fun leg(points: List<RoutePoint>, avoid: Set<RoadAvoidance>, language: String): EvRoadLeg {
            val key = points.joinToString(">") { it.label }
            calls += key
            return legs[key] ?: EvRoadLeg.Unavailable(EvRoadUnavailable.NO_ROUTE)
        }
    }

    private class FakeSource(private val result: EvCandidateResult) : EvCandidateSource {
        var query: EvCandidateQuery? = null
        override suspend fun candidates(query: EvCandidateQuery): EvCandidateResult {
            this.query = query
            return result
        }
    }

    private fun candidate(
        site: String,
        id: String = "c-$site",
        point: GeoPoint,
        siteName: String = site,
        equipment: String = "eq-$site",
        connector: ChargingConnector = ChargingConnector.CCS2,
        access: EvAccess = EvAccess.PUBLIC,
        hours: EvHours = EvHours.Unknown,
        tariff: EvTariff = EvTariff.Unknown,
        powerW: Long? = 150_000,
        observedAt: Instant? = null,
    ) = EvChargingCandidate(
        candidateId = id,
        siteId = site,
        siteName = siteName,
        equipmentId = equipment,
        connectorId = "conn-$id",
        connector = connector,
        current = ChargingCurrent.DC,
        powerW = powerW,
        access = access,
        hours = hours,
        tariff = tariff,
        provenance = EvProvenance.STATIC_REFERENCE,
        sourceRevision = "synthetic-v1",
        observedAt = observedAt,
        point = point,
    )

    private fun primary() = candidate("a-primary", "c-primary", GeoPoint.fromDouble(primaryPoint.latitude, primaryPoint.longitude), "primary")
    private fun alt10() = candidate("b-alt10", "c-alt10", GeoPoint.fromDouble(alt10Point.latitude, alt10Point.longitude), "alt10")
    private fun alt20() = candidate("c-alt20", "c-alt20", GeoPoint.fromDouble(alt20Point.latitude, alt20Point.longitude), "alt20")

    private fun routerWithCommonLegs(warnings: Boolean = false): FakeRouter {
        val router = FakeRouter()
        router.add(origin, destination, ready(100_000, 7_200, listOf(origin, primaryPoint, destination), warnings))
        router.add(origin, primaryPoint, ready(50_000, 3_600, warnings = warnings))
        router.add(primaryPoint, destination, ready(50_000, 3_600, warnings = warnings))
        router.add(primaryPoint, alt10Point, ready(10_000, 720))
        router.add(primaryPoint, alt20Point, ready(20_000, 1_440))
        router.add(alt10Point, destination, ready(40_000, 2_880))
        router.add(alt20Point, destination, ready(30_000, 2_160))
        return router
    }

    @Test
    fun `road legs energy reserve and a reachable alternative are proven`() = runTest {
        val router = routerWithCommonLegs()
        val source = FakeSource(EvCandidateResult(listOf(primary(), alt10(), alt20()), EvCoverage.PRESENT, 42, "fixture-v1"))
        val plan = PlanEvTripUseCase(router, source).invoke(input)
        assertEquals(EvPlanStatus.PLANNED, plan.status)
        assertEquals(1, plan.stops.size)
        val stop = plan.stops.single()
        assertEquals("c-primary", stop.candidate.candidateId)
        assertEquals(1_500, stop.arrivalSocBp)
        assertEquals(39_000, stop.chargeEnergyWh)
        assertEquals(6_500, (plan.arrivalSocBp as EnergyEstimate.Known).value)
        assertTrue(stop.minimumChargeSeconds is EnergyEstimate.Known)
        assertEquals(EvPlanStatus.PLANNED, plan.status)
        assertEquals("c-alt10", stop.alternative?.candidate?.candidateId)
        assertEquals(1_200, (stop.alternative?.arrivalSocBp as EnergyEstimate.Known).value)
        assertTrue(stop.alternative!!.energyReachable)
        assertEquals(4, plan.routeCalls)
        assertTrue(router.calls.contains("primary>alt10"))
        assertTrue(router.calls.contains("primary>destination"))
        assertEquals(ChargingConnector.CCS2, source.query?.connector)
        assertEquals(200, source.query?.limit)
        assertEquals(42L, plan.assumptions.generation)
        assertEquals("fixture-v1", plan.assumptions.sourceRevision)
    }

    @Test
    fun `unreachable alternative after the failed primary is not claimed`() = runTest {
        val router = routerWithCommonLegs()
        val source = FakeSource(EvCandidateResult(listOf(primary(), alt20()), EvCoverage.PRESENT))
        val plan = PlanEvTripUseCase(router, source).invoke(input)
        assertEquals(EvPlanStatus.PLANNED, plan.status)
        assertNull(plan.stops.single().alternative)
        assertTrue(plan.issues.any { it.code == EvIssueCode.ALTERNATIVE_UNREACHABLE })
        assertTrue(plan.issues.any { it.code == EvIssueCode.NO_EVIDENCED_ALTERNATIVE })
    }

    @Test
    fun `same-site connector is not an independent fallback`() = runTest {
        val router = routerWithCommonLegs()
        val sameSite = candidate(
            site = "a-primary",
            id = "c-primary-other",
            point = GeoPoint.fromDouble(primaryPoint.latitude, primaryPoint.longitude),
            siteName = "primary",
            equipment = "eq-other",
        )
        val source = FakeSource(EvCandidateResult(listOf(primary(), sameSite), EvCoverage.PRESENT))
        val plan = PlanEvTripUseCase(router, source).invoke(input)
        assertEquals(EvPlanStatus.PLANNED, plan.status)
        assertNull(plan.stops.single().alternative)
        assertTrue(plan.issues.any { it.code == EvIssueCode.ALTERNATIVE_SAME_SITE })
    }

    @Test
    fun `private incompatible closed and unknown metadata stay distinct`() = runTest {
        val router = routerWithCommonLegs()
        val privatePlan = PlanEvTripUseCase(
            router,
            FakeSource(EvCandidateResult(listOf(primary().copy(access = EvAccess.PRIVATE)), EvCoverage.PRESENT)),
        ).invoke(input)
        assertEquals(EvPlanStatus.NO_CANDIDATES, privatePlan.status)
        assertTrue(privatePlan.issues.any { it.code == EvIssueCode.ACCESS_PRIVATE })

        val incompatible = primary().copy(connector = ChargingConnector.CHADEMO)
        val incompatiblePlan = PlanEvTripUseCase(
            router,
            FakeSource(EvCandidateResult(listOf(incompatible), EvCoverage.PRESENT)),
        ).invoke(input)
        assertEquals(EvPlanStatus.NO_CANDIDATES, incompatiblePlan.status)
        assertTrue(incompatiblePlan.issues.any { it.code == EvIssueCode.INCOMPATIBLE_CONNECTOR })

        val closed = primary().copy(
            hours = EvHours.Daily(20 * 60, 6 * 60, "America/Sao_Paulo"),
        )
        val closedPlan = PlanEvTripUseCase(
            routerWithCommonLegs(),
            FakeSource(EvCandidateResult(listOf(closed), EvCoverage.PRESENT)),
        ).invoke(input)
        assertEquals(EvPlanStatus.INSUFFICIENT_ENERGY, closedPlan.status)
        assertTrue(closedPlan.issues.any { it.code == EvIssueCode.HOURS_CLOSED })

        val unknown = primary().copy(hours = EvHours.Unknown, powerW = null, tariff = EvTariff.Unknown)
        val unknownPlan = PlanEvTripUseCase(
            routerWithCommonLegs(),
            FakeSource(EvCandidateResult(listOf(unknown), EvCoverage.PRESENT)),
        ).invoke(input)
        assertEquals(EvPlanStatus.PLANNED, unknownPlan.status)
        assertTrue(unknownPlan.stops.single().issues.contains(EvIssueCode.HOURS_UNKNOWN))
        assertTrue(unknownPlan.stops.single().minimumChargeSeconds is EnergyEstimate.Unknown)
        assertTrue(unknownPlan.stops.single().estimatedCostMilliBrl is EnergyEstimate.Unknown)
    }

    @Test
    fun `missing snapshot no route and missing maps never fabricate a stop`() = runTest {
        val missing = PlanEvTripUseCase(
            routerWithCommonLegs(),
            FakeSource(EvCandidateResult(emptyList(), EvCoverage.MISSING)),
        ).invoke(input)
        assertEquals(EvPlanStatus.INSUFFICIENT_DATA, missing.status)
        assertTrue(missing.issues.any { it.code == EvIssueCode.SNAPSHOT_MISSING })

        val noRouteRouter = FakeRouter()
        val noRoute = PlanEvTripUseCase(
            noRouteRouter,
            FakeSource(EvCandidateResult(emptyList(), EvCoverage.PRESENT)),
        ).invoke(input)
        assertEquals(EvPlanStatus.NO_ROUTE, noRoute.status)
        assertTrue(noRoute.issues.any { it.code == EvIssueCode.NO_ROAD_ROUTE })

        val mapsRouter = FakeRouter()
        mapsRouter.add(origin, destination, EvRoadLeg.Unavailable(EvRoadUnavailable.MISSING_MAPS))
        val maps = PlanEvTripUseCase(
            mapsRouter,
            FakeSource(EvCandidateResult(emptyList(), EvCoverage.PRESENT)),
        ).invoke(input)
        assertEquals(EvPlanStatus.NO_ROUTE, maps.status)
        assertTrue(maps.issues.any { it.code == EvIssueCode.MISSING_MAPS })
    }

    @Test
    fun `replanning starts from the real failure charge and excludes the failed site`() = runTest {
        val router = FakeRouter()
        router.add(primaryPoint, destination, ready(50_000, 3_600, listOf(primaryPoint, alt10Point, destination)))
        router.add(primaryPoint, alt10Point, ready(10_000, 720))
        router.add(alt10Point, destination, ready(40_000, 2_880))
        val source = FakeSource(EvCandidateResult(listOf(primary(), alt10()), EvCoverage.PRESENT))
        val replanInput = input.copy(
            origin = primaryPoint,
            initialPoint = primaryPoint,
            initialSocBp = 1_500,
            failures = listOf(EvFailure(EvFailureScope.SITE, siteId = "a-primary")),
        )
        val plan = PlanEvTripUseCase(router, source).invoke(replanInput)
        assertEquals(EvPlanStatus.PLANNED, plan.status)
        assertEquals("c-alt10", plan.stops.single().candidate.candidateId)
        assertEquals(1_200, plan.stops.single().arrivalSocBp)
        assertFalse(plan.stops.any { it.candidate.siteId == "a-primary" })
        assertFalse(router.calls.any { it.contains("primary>primary") })
        assertEquals(6_800, (plan.arrivalSocBp as EnergyEstimate.Known).value)
    }

    @Test
    fun `replanning can use a reachable charger behind the failure point`() = runTest {
        val behind = RoutePoint(-24.60, -49.0, "behind")
        val router = FakeRouter()
        router.add(primaryPoint, destination, ready(50_000, 3_600, listOf(primaryPoint, behind, destination)))
        router.add(primaryPoint, behind, ready(8_000, 576))
        router.add(behind, destination, ready(45_000, 3_240))
        val behindCandidate = candidate("b-behind", "c-behind", GeoPoint.fromDouble(behind.latitude, behind.longitude), "behind")
        val excludedPrimary = primary()
        val source = FakeSource(EvCandidateResult(listOf(excludedPrimary, behindCandidate), EvCoverage.PRESENT))
        val plan = PlanEvTripUseCase(router, source).invoke(
            input.copy(
                origin = primaryPoint,
                initialPoint = primaryPoint,
                initialSocBp = 1_500,
                failures = listOf(EvFailure(EvFailureScope.SITE, siteId = "a-primary")),
            ),
        )
        assertEquals(EvPlanStatus.PLANNED, plan.status)
        assertEquals("c-behind", plan.stops.single().candidate.candidateId)
        assertTrue(router.calls.contains("primary>behind"))
    }

    @Test
    fun `budget exhaustion is explicit and never presented as an optimum`() = runTest {
        val plan = PlanEvTripUseCase(
            routerWithCommonLegs(),
            FakeSource(EvCandidateResult(listOf(primary(), alt10()), EvCoverage.PRESENT)),
        ).invoke(input.copy(maxRouteCalls = 3))
        assertEquals(EvPlanStatus.BUDGET_EXHAUSTED, plan.status)
        assertTrue(plan.issues.any { it.code == EvIssueCode.ROUTE_BUDGET_EXHAUSTED || it.code == EvIssueCode.INCOMPLETE })
        assertTrue(plan.stops.isEmpty())
    }

    @Test
    fun `road warning is surfaced and unknown arrival time cannot confirm hours`() = runTest {
        val warningPlan = PlanEvTripUseCase(
            routerWithCommonLegs(warnings = true),
            FakeSource(EvCandidateResult(listOf(primary()), EvCoverage.PRESENT)),
        ).invoke(input)
        assertEquals(EvPlanStatus.PLANNED, warningPlan.status)
        assertTrue(warningPlan.issues.any { it.code == EvIssueCode.ROAD_WARNING })

        val unknownArrival = PlanEvTripUseCase(
            routerWithCommonLegs(),
            FakeSource(
                EvCandidateResult(
                    listOf(primary().copy(hours = EvHours.Daily(0, 1_439, "America/Sao_Paulo"))),
                    EvCoverage.PRESENT,
                ),
            ),
        ).invoke(input.copy(departureAt = null))
        assertEquals(EvPlanStatus.PLANNED, unknownArrival.status)
        assertTrue(unknownArrival.stops.single().issues.contains(EvIssueCode.HOURS_UNKNOWN))
    }

    @Test
    fun `PHEV plan keeps electric-portion scope`() = runTest {
        val plan = PlanEvTripUseCase(
            routerWithCommonLegs(),
            FakeSource(EvCandidateResult(listOf(primary()), EvCoverage.PRESENT)),
        ).invoke(input.copy(battery = battery.copy(socBp = 8_000, usablePermille = 500)))
        assertEquals(EnergyScope.ELECTRIC_PORTION_ONLY, plan.scope)
        assertEquals(EvPlanStatus.PLANNED, plan.status)
    }
}
