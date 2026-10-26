package com.anpfuel.domain.ev

import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.map.saved.GeoPoint
import com.anpfuel.domain.routing.RoutePoint
import java.time.DateTimeException
import java.time.Instant
import java.time.LocalDateTime
import java.time.ZoneId
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class EvChargingPlanTest {
    private val origin = RoutePoint(-25.0, -49.0, "origin")
    private val destination = RoutePoint(-24.8, -49.0, "destination")
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
    )

    private fun candidate(
        site: String,
        connector: ChargingConnector = ChargingConnector.CCS2,
        equipment: String = "eq-$site",
        candidateId: String = "c-$site",
        access: EvAccess = EvAccess.PUBLIC,
        hours: EvHours = EvHours.Unknown,
        tariff: EvTariff = EvTariff.Unknown,
        powerW: Long? = 50_000,
        observedAt: Instant? = null,
        point: GeoPoint = GeoPoint.fromDouble(-24.9, -49.0),
    ) = EvChargingCandidate(
        candidateId = candidateId,
        siteId = site,
        siteName = site,
        equipmentId = equipment,
        connectorId = "conn-$site",
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

    @Test
    fun `mandatory failure vector uses energy at the failed primary and never its charge`() {
        val below = EvChargingPlanRules.failureReachability(
            battery = battery,
            arrivalSocBpAtPrimary = 1_500,
            consumption = consumption,
            alternativeDistanceMeters = 20_000,
        )
        assertEquals(EnergyEstimate.Known(900L), below)
        assertFalse(EvChargingPlanRules.isReachable((below as EnergyEstimate.Known).value, 1_000))

        val reachable = EvChargingPlanRules.failureReachability(
            battery = battery,
            arrivalSocBpAtPrimary = 1_500,
            consumption = consumption,
            alternativeDistanceMeters = 10_000,
        )
        assertEquals(EnergyEstimate.Known(1_200L), reachable)
        assertTrue(EvChargingPlanRules.isReachable((reachable as EnergyEstimate.Known).value, 1_000))
    }

    @Test
    fun `PHEV failure reachability uses the declared electric portion`() {
        val phev = battery.copy(usablePermille = 500)
        val result = EvChargingPlanRules.failureReachability(
            battery = phev,
            arrivalSocBpAtPrimary = 2_000,
            consumption = consumption,
            alternativeDistanceMeters = 10_000,
        )
        // 30,000 Wh electric portion, 2,000 bp = 6,000 Wh; 10 km = 1,800 Wh, arrival 4,200/30,000 = 1,400 bp.
        assertEquals(EnergyEstimate.Known(1_400L), result)
        assertEquals(EnergyScope.ELECTRIC_PORTION_ONLY, EvEnergyModel.scopeOf(phev))
    }

    @Test
    fun `charge target energy and cost are conservative and unknown tariff is never zero`() {
        assertEquals(EnergyEstimate.Known(30_000L), EvChargingPlanRules.energyToTargetWh(battery, 3_000, 8_000))
        assertEquals(EnergyEstimate.Known(0L), EvChargingPlanRules.energyToTargetWh(battery, 8_000, 8_000))
        assertEquals(EnergyEstimate.Known(15_000L), EvChargingPlanRules.energyToTargetWh(battery.copy(usablePermille = 500), 3_000, 8_000))
        assertEquals(EnergyEstimate.Unknown("tariff-unknown"), EvChargingPlanRules.chargeCostMilliBrl(30_000, EvTariff.Unknown))
        val tariff = EvTariff.Known(1_500, "STANDARD", "fixture", Instant.parse("2026-10-26T00:00:00Z"))
        assertEquals(EnergyEstimate.Known(45_000L), EvChargingPlanRules.chargeCostMilliBrl(30_000, tariff))
        assertEquals(EnergyEstimate.Known(0L), EvChargingPlanRules.chargeCostMilliBrl(0, tariff))
    }

    @Test
    fun `hours use the candidate zone and keep unknown arrival unknown`() {
        val saoPaulo = EvHours.Daily(20 * 60, 6 * 60, "America/Sao_Paulo")
        val manaus = EvHours.Daily(20 * 60, 6 * 60, "America/Manaus")
        val noonUtc = Instant.parse("2026-10-26T15:00:00Z") // 12:00 in Sao Paulo, 11:00 in Manaus
        assertEquals(EvHoursStatus.CLOSED, saoPaulo.statusAt(noonUtc))
        val late = Instant.parse("2026-10-27T02:00:00Z") // 23:00 in Sao Paulo, 22:00 in Manaus
        assertEquals(EvHoursStatus.OPEN, saoPaulo.statusAt(late))
        assertEquals(EvHoursStatus.OPEN, manaus.statusAt(late))
        val early = Instant.parse("2026-10-26T09:00:00Z") // 06:00 closed in Sao Paulo, 05:00 open in Manaus
        assertEquals(EvHoursStatus.CLOSED, saoPaulo.statusAt(early))
        assertEquals(EvHoursStatus.OPEN, manaus.statusAt(early))
        assertEquals(EvHoursStatus.UNKNOWN, saoPaulo.statusAt(null))
        assertEquals(EvHoursStatus.UNKNOWN, EvHours.Unknown.statusAt(late))
        assertEquals(EvHoursStatus.CLOSED, EvHours.Closed.statusAt(late))
        assertEquals(EvHoursStatus.CLOSED, EvHours.Daily(600, 600, "America/Sao_Paulo").statusAt(late))
        assertThrows(DateTimeException::class.java) {
            EvHours.Daily(0, 60, "Not/AZone")
        }
    }

    @Test
    fun `eligibility refuses private closed and incompatible but exposes unknown metadata`() {
        val arrival = LocalDateTime.of(2026, 10, 26, 23, 0).atZone(ZoneId.of("America/Sao_Paulo")).toInstant()
        val incompatible = EvChargingPlanRules.eligibility(candidate("a", connector = ChargingConnector.CHADEMO), input, arrival)
        assertTrue(EvIssueCode.INCOMPATIBLE_CONNECTOR in incompatible)

        val private = EvChargingPlanRules.eligibility(candidate("a", access = EvAccess.PRIVATE), input, arrival)
        assertTrue(EvIssueCode.ACCESS_PRIVATE in private)

        val closed = EvChargingPlanRules.eligibility(
            candidate("a", hours = EvHours.Daily(20 * 60, 6 * 60, "America/Sao_Paulo")),
            input.copy(departureAt = Instant.parse("2026-10-26T12:00:00Z")),
            Instant.parse("2026-10-26T15:00:00Z"),
        )
        assertTrue(EvIssueCode.HOURS_CLOSED in closed)

        val unknown = EvChargingPlanRules.eligibility(candidate("a", powerW = null), input, null)
        assertTrue(EvIssueCode.POWER_UNKNOWN in unknown)
        assertTrue(EvIssueCode.TARIFF_UNKNOWN in unknown)
        assertTrue(EvIssueCode.HOURS_UNKNOWN in unknown)
        assertTrue(EvIssueCode.REPORT_DATE_UNKNOWN in unknown)
        assertTrue(EvIssueCode.ACCESS_UNKNOWN !in unknown)
    }

    @Test
    fun `failure scopes exclude the right candidates and same-site fallback is never independent`() {
        val siteA = candidate("a")
        val otherConnectorA = candidate("a", candidateId = "c-a2")
        val siteB = candidate("b")
        assertTrue(EvChargingPlanRules.excluded(siteA, listOf(EvFailure(EvFailureScope.SITE, siteId = "a")), alternative = false))
        assertFalse(EvChargingPlanRules.excluded(siteB, listOf(EvFailure(EvFailureScope.SITE, siteId = "a")), alternative = false))
        assertTrue(EvChargingPlanRules.excluded(siteA, listOf(EvFailure(EvFailureScope.EQUIPMENT, siteId = "a", equipmentId = "eq-a")), alternative = false))
        assertTrue(EvChargingPlanRules.excluded(siteA, listOf(EvFailure(EvFailureScope.CONNECTOR, siteId = "a", connectorId = "conn-a")), alternative = false))
        assertTrue(EvChargingPlanRules.excluded(otherConnectorA, listOf(EvFailure(EvFailureScope.SITE, siteId = "a")), alternative = true))
        assertTrue(EvChargingPlanRules.excluded(siteA, listOf(EvFailure(EvFailureScope.CONNECTOR, siteId = "a", connectorId = "x")), alternative = true))
    }

    @Test
    fun `duplicate connectors are collapsed per site deterministically`() {
        val slow = candidate("a", candidateId = "c-slow", powerW = 22_000)
        val fast = candidate("a", candidateId = "c-fast", powerW = 150_000)
        val other = candidate("b", candidateId = "c-b", powerW = 50_000)
        val best = EvChargingPlanRules.bestPerSite(listOf(slow, fast, other))
        assertEquals(listOf("c-b", "c-fast"), best.map { it.candidateId }.sorted())
    }

    @Test
    fun `route geometry prefilter projects onto the polyline and keeps a bounded box`() {
        val route = listOf(
            RoutePoint(-25.0, -49.0, "a"),
            RoutePoint(-24.9, -49.0, "b"),
            RoutePoint(-24.8, -49.0, "c"),
        )
        val projection = EvRouteGeometry.project(GeoPoint.fromDouble(-24.95, -48.999), route)
        assertTrue(projection != null)
        assertTrue(projection!!.alongMetres in 4_000..7_000)
        assertTrue(projection.offRouteMetres < 150)
        val box = EvRouteGeometry.boundingBox(route, 1_000)
        assertTrue(box.minLatitude < -25.0)
        assertTrue(box.maxLatitude > -24.8)
        assertTrue(box.minLongitude < -49.0)
        assertTrue(box.maxLongitude > -49.0)
        assertNull(EvRouteGeometry.project(GeoPoint.fromDouble(-25.0, -49.0), listOf(route.first())))
    }

    @Test
    fun `validation separates missing data bad target and limits`() {
        assertEquals(emptyList<EvIssue>(), EvChargingPlanRules.validate(input))
        assertTrue(EvChargingPlanRules.validate(input.copy(vehicleConnector = null)).any { it.code == EvIssueCode.VEHICLE_CONNECTOR_MISSING })
        assertTrue(EvChargingPlanRules.validate(input.copy(chargeTargetSocBp = 900)).any { it.code == EvIssueCode.INVALID_TARGET_SOC })
        assertTrue(EvChargingPlanRules.validate(input.copy(maxRouteCalls = 0)).any { it.code == EvIssueCode.INVALID_LIMITS })
        assertTrue(
            EvChargingPlanRules.validate(input.copy(battery = battery.copy(capacityWh = null)))
                .any { it.code == EvIssueCode.BATTERY_MISSING },
        )
        assertTrue(
            EvChargingPlanRules.validate(input.copy(consumption = ConsumptionInput(null)))
                .any { it.code == EvIssueCode.CONSUMPTION_MISSING },
        )
        assertTrue(
            EvChargingPlanRules.validate(input.copy(origin = origin.copy(label = "x".repeat(513))))
                .any { it.code == EvIssueCode.INVALID_ROUTE },
        )
    }
}
