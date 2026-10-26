package com.anpfuel.app.ui.map

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.application.usecase.ev.PlanEvTripUseCase
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.ev.*
import com.anpfuel.domain.map.saved.GeoPoint
import com.anpfuel.domain.routing.RoutePoint
import com.anpfuel.maps.engine.MapEngineHolder
import com.anpfuel.maps.engine.NativeEvRoadRouter
import java.io.File
import java.time.Instant
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * MAP-27/MAP-28 real-core proof: the admitted Parana East MWM produces the main,
 * stop, detour and fallback road legs. Charging candidates and battery inputs are
 * synthetic and identified as such; no charger coverage or live state is claimed.
 */
@RunWith(AndroidJUnit4::class)
class EvTripRealEngineInstrumentedTest {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val engine get() = MapEngineHolder.engine

    private fun initialize() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        assertTrue("admitted regional fixture is required", File(context.filesDir, "261004/Brazil_Parana_East.mwm").isFile)
        val ready = CountDownLatch(1)
        instrumentation.runOnMainSync {
            assertTrue(engine.initialize(context) { ready.countDown() })
        }
        assertTrue("native readiness", ready.await(45, TimeUnit.SECONDS))
        instrumentation.runOnMainSync { assertTrue(engine.reloadMaps()) }
    }

    private class SyntheticSource(private val candidates: List<EvChargingCandidate>) : EvCandidateSource {
        override suspend fun candidates(query: EvCandidateQuery): EvCandidateResult {
            assertTrue("bounded corridor query", query.limit in 1..500)
            return EvCandidateResult(candidates, EvCoverage.PRESENT, 1, "synthetic-ev-real-road-v1")
        }
    }

    private fun candidate(site: String, point: RoutePoint): EvChargingCandidate = EvChargingCandidate(
        candidateId = "synthetic-$site",
        siteId = site,
        siteName = site,
        equipmentId = "synthetic-equipment-$site",
        connectorId = "synthetic-connector-$site",
        connector = ChargingConnector.CCS2,
        current = ChargingCurrent.DC,
        powerW = 50_000,
        access = EvAccess.PUBLIC,
        hours = EvHours.Unknown,
        tariff = EvTariff.Unknown,
        provenance = EvProvenance.STATIC_REFERENCE,
        sourceRevision = "synthetic-ev-real-road-v1",
        observedAt = null,
        point = GeoPoint.fromDouble(point.latitude, point.longitude),
    )

    private fun routeDestination(router: NativeEvRoadRouter, origin: RoutePoint, destination: RoutePoint): EvRoadLeg.Ready? =
        runBlocking {
            router.leg(listOf(origin, destination), emptySet(), "en") as? EvRoadLeg.Ready
        }

    @Test
    fun realRoadLegsBackPrimaryAlternativeAndFailureReplan() {
        initialize()
        val origin = RoutePoint(-25.4295, -49.2719, "synthetic origin")
        val destinations = listOf(
            RoutePoint(-25.53, -49.20, "synthetic south"),
            RoutePoint(-25.29, -49.22, "synthetic north"),
            RoutePoint(-25.59, -49.41, "synthetic west"),
        )
        val router = NativeEvRoadRouter(engine, timeoutMillis = 60_000)
        val selected = destinations.firstNotNullOfOrNull { destination ->
            routeDestination(router, origin, destination)?.let { it to destination }
        }
        assertTrue("no admitted-region destination produced a real road route", selected != null)
        val (main, destination) = selected!!
        assertTrue("real main distance", main.distanceMeters > 2_000)
        val geometry = main.geometry
        assertTrue("real route geometry", geometry.size >= 4)
        val primaryPoint = geometry[(geometry.size * 55 / 100).coerceIn(1, geometry.lastIndex - 1)]
        val altPoint = geometry[(geometry.size * 48 / 100).coerceIn(1, geometry.lastIndex - 1)]
        assertFalse(primaryPoint.latitude == altPoint.latitude && primaryPoint.longitude == altPoint.longitude)

        val capacityWh = maxOf(500L, main.distanceMeters * 4 / 10)
        val battery = BatteryInput(capacityWh, socBp = 5_000, reserveBp = 1_000)
        val input = EvTripInput(
            origin = origin,
            destination = destination,
            departureAt = Instant.parse("2026-10-26T12:00:00Z"),
            battery = battery,
            consumption = ConsumptionInput(180_000),
            vehicleConnector = ChargingConnector.CCS2,
            chargeTargetSocBp = 8_000,
            maxRouteCalls = 48,
        )
        val primary = candidate("a-primary", primaryPoint)
        val alternative = candidate("b-alt", altPoint)
        val plan = runBlocking {
            PlanEvTripUseCase(router, SyntheticSource(listOf(primary, alternative))).invoke(input)
        }
        assertEquals("plan: ${plan.issues}", EvPlanStatus.PLANNED, plan.status)
        val stop = plan.stops.single()
        assertEquals("a-primary", stop.candidate.siteId)
        assertTrue("real stop distance", (stop.arrival.road as EvRoadLeg.Ready).distanceMeters > 0)
        assertTrue("arrival above reserve", stop.arrivalSocBp >= 1_000)
        val alt = stop.alternative
        assertTrue("real alternative expected", alt != null)
        assertTrue("real alternative leg", (alt!!.leg.road as EvRoadLeg.Ready).distanceMeters > 0)
        assertTrue("alternative energy", alt.energyReachable)
        assertTrue("final distance", ((plan.finalLeg?.road as? EvRoadLeg.Ready)?.distanceMeters ?: 0L) > 0)

        val replanInput = input.copy(
            origin = RoutePoint(primaryPoint.latitude, primaryPoint.longitude, "synthetic failure"),
            initialPoint = RoutePoint(primaryPoint.latitude, primaryPoint.longitude, "synthetic failure"),
            initialSocBp = stop.arrivalSocBp,
            failures = listOf(EvFailure(EvFailureScope.SITE, siteId = "a-primary")),
            waypoints = emptyList(),
        )
        val replan = runBlocking {
            PlanEvTripUseCase(router, SyntheticSource(listOf(primary, alternative))).invoke(replanInput)
        }
        assertEquals("replan: ${replan.issues}", EvPlanStatus.PLANNED, replan.status)
        assertEquals("b-alt", replan.stops.single().candidate.siteId)
        assertTrue("replan alternative arrival", replan.stops.single().arrivalSocBp >= 1_000)
        instrumentation.runOnMainSync { engine.cancelRoute() }
    }
}
