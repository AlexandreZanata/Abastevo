package com.anpfuel.app.ui.map

import androidx.lifecycle.SavedStateHandle
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
import java.time.Instant
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class EvTripViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val origin = RoutePoint(-25.0, -49.0, "origin")
    private val primaryPoint = RoutePoint(-24.55, -49.0, "primary")
    private val altPoint = RoutePoint(-24.55, -48.995, "alt")
    private val destination = RoutePoint(-24.1, -49.0, "destination")

    @BeforeEach fun setUp() { Dispatchers.setMain(dispatcher) }
    @AfterEach fun tearDown() { Dispatchers.resetMain() }

    private class FakeVehicles(private val vehicles: List<Vehicle>) : VehicleRepository {
        override suspend fun listAll() = vehicles
        override suspend fun findById(id: DomainId) = vehicles.firstOrNull { it.id == id }
        override suspend fun save(vehicle: Vehicle) = Unit
        override suspend fun delete(id: DomainId) = Unit
        override suspend fun count() = vehicles.size
    }

    private inner class FakeRouter : EvRoadRouter {
        val calls = mutableListOf<String>()
        override suspend fun leg(points: List<RoutePoint>, avoid: Set<RoadAvoidance>, language: String): EvRoadLeg {
            val key = points.joinToString(">") { it.label }
            calls += key
            return when (key) {
                "origin>destination" -> ready(100_000, listOf(origin, primaryPoint, destination))
                "origin>primary" -> ready(50_000)
                "primary>destination" -> ready(50_000, listOf(primaryPoint, altPoint, destination))
                "primary>alt" -> ready(10_000)
                "alt>destination" -> ready(40_000)
                else -> EvRoadLeg.Unavailable(EvRoadUnavailable.NO_ROUTE)
            }
        }
        private fun ready(metres: Long, geometry: List<RoutePoint> = emptyList()) =
            EvRoadLeg.Ready(metres, metres / 50, false, geometry)
    }

    private class FakeSource(private val candidates: List<EvChargingCandidate>) : EvCandidateSource {
        override suspend fun candidates(query: EvCandidateQuery) =
            EvCandidateResult(candidates, EvCoverage.PRESENT, 9, "fixture-v1", 1)
    }

    private fun candidate(site: String, point: GeoPoint) = EvChargingCandidate(
        candidateId = "c-$site", siteId = site, siteName = site, equipmentId = "e-$site",
        connectorId = "k-$site", connector = ChargingConnector.CCS2, current = ChargingCurrent.DC,
        powerW = 100_000, access = EvAccess.PUBLIC, hours = EvHours.Unknown,
        tariff = EvTariff.Unknown, provenance = EvProvenance.STATIC_REFERENCE,
        sourceRevision = "fixture-v1", observedAt = null, point = point,
    )

    private fun vehicle(connector: ChargingConnector? = ChargingConnector.CCS2, propulsion: Propulsion = Propulsion.BEV): Vehicle =
        Vehicle.create(
            displayName = "Test EV",
            tankCapacity = null,
            fuelProduct = null,
            priceSource = VehiclePriceSource.cheapest(),
            propulsion = propulsion,
            batteryCapacity = BatteryCapacity.ofWh(60_000),
            chargeConnector = connector,
        )

    private fun useCase(router: EvRoadRouter, source: EvCandidateSource) = PlanEvTripUseCase(router, source)

    @Test
    fun `valid inputs plan and changing an input clears the old result`() = runTest {
        val router = FakeRouter()
        val primary = candidate("primary", GeoPoint.fromDouble(primaryPoint.latitude, primaryPoint.longitude))
        val alt = candidate("alt", GeoPoint.fromDouble(altPoint.latitude, altPoint.longitude))
        val vm = EvTripViewModel(useCase(router, FakeSource(listOf(primary, alt))), FakeVehicles(listOf(vehicle())), SavedStateHandle())
        vm.nowProvider = { Instant.parse("2026-10-26T12:00:00Z") }
        advanceUntilIdle()
        vm.plan(origin, emptyList(), destination, emptySet(), "en")
        advanceUntilIdle()
        assertNotNull(vm.ui.value.plan)
        assertEquals(EvPlanStatus.PLANNED, vm.ui.value.plan!!.status)
        vm.onSocChanged("25")
        assertNull(vm.ui.value.plan)
        assertNull(vm.ui.value.error)
    }

    @Test
    fun `invalid numeric input and missing connector never call the planner`() = runTest {
        val vm = EvTripViewModel(
            useCase(FakeRouter(), FakeSource(emptyList())),
            FakeVehicles(listOf(vehicle(connector = null), vehicle())),
            SavedStateHandle(),
        )
        advanceUntilIdle()
        vm.plan(origin, emptyList(), destination, emptySet(), "en")
        assertEquals("vehicle-connector-missing", vm.ui.value.error)
        vm.selectVehicle(vm.ui.value.vehicles[1].id)
        vm.onSocChanged("abc")
        vm.plan(origin, emptyList(), destination, emptySet(), "en")
        assertEquals("invalid-soc", vm.ui.value.error)
    }

    @Test
    fun `failed primary replans from its arrival charge and excludes the failed site`() = runTest {
        val router = FakeRouter()
        val primary = candidate("primary", GeoPoint.fromDouble(primaryPoint.latitude, primaryPoint.longitude))
        val alt = candidate("alt", GeoPoint.fromDouble(altPoint.latitude, altPoint.longitude))
        val vm = EvTripViewModel(useCase(router, FakeSource(listOf(primary, alt))), FakeVehicles(listOf(vehicle())), SavedStateHandle())
        vm.nowProvider = { Instant.parse("2026-10-26T12:00:00Z") }
        advanceUntilIdle()
        vm.onSocChanged("30")
        vm.plan(origin, emptyList(), destination, emptySet(), "en")
        advanceUntilIdle()
        val first = vm.ui.value.plan!!
        assertEquals("c-primary", first.stops.single().candidate.candidateId)
        vm.markPrimaryFailed()
        assertNull(vm.ui.value.plan)
        assertTrue(vm.ui.value.planning)
        advanceUntilIdle()
        val second = vm.ui.value.plan!!
        assertEquals("c-alt", second.stops.single().candidate.candidateId)
        assertTrue(vm.ui.value.replannedAfterFailure)
        assertEquals("primary", vm.ui.value.failureSiteId)
        assertFalse(router.calls.any { it.contains("primary>primary") })
    }

    @Test
    fun `inputs survive process recreation and no vehicle is an explicit state`() = runTest {
        val saved = SavedStateHandle()
        val vm = EvTripViewModel(useCase(FakeRouter(), FakeSource(emptyList())), FakeVehicles(listOf(vehicle())), saved)
        advanceUntilIdle()
        vm.onSocChanged("42,5")
        vm.onReserveChanged("12")
        val restored = EvTripViewModel(useCase(FakeRouter(), FakeSource(emptyList())), FakeVehicles(listOf(vehicle())), saved)
        advanceUntilIdle()
        assertEquals("42,5", restored.ui.value.socInput)
        assertEquals("12", restored.ui.value.reserveInput)
        assertNotNull(restored.ui.value.selectedVehicleId)

        val empty = EvTripViewModel(useCase(FakeRouter(), FakeSource(emptyList())), FakeVehicles(emptyList()), SavedStateHandle())
        advanceUntilIdle()
        assertEquals("no-electric-vehicle", empty.ui.value.error)
    }

    @Test
    fun `a stale planning generation cannot publish after a new input`() = runTest {
        val router = FakeRouter()
        val primary = candidate("primary", GeoPoint.fromDouble(primaryPoint.latitude, primaryPoint.longitude))
        val vm = EvTripViewModel(useCase(router, FakeSource(listOf(primary))), FakeVehicles(listOf(vehicle())), SavedStateHandle())
        advanceUntilIdle()
        vm.plan(origin, emptyList(), destination, emptySet(), "en")
        vm.onSocChanged("31")
        advanceUntilIdle()
        assertNull(vm.ui.value.plan)
        assertNull(vm.ui.value.error)
    }
}
