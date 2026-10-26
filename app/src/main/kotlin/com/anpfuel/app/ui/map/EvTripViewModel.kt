package com.anpfuel.app.ui.map

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.ev.PlanEvTripUseCase
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.ev.*
import com.anpfuel.domain.map.saved.GeoPoint
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.routing.RoadAvoidance
import com.anpfuel.domain.routing.RoutePoint
import com.anpfuel.domain.valueobject.Propulsion
import dagger.hilt.android.lifecycle.HiltViewModel
import java.math.BigDecimal
import java.math.RoundingMode
import java.time.Instant
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class EvVehicleOption(
    val id: String,
    val name: String,
    val capacityWh: Long,
    val connector: ChargingConnector?,
    val propulsion: Propulsion,
)

data class EvTripUiState(
    val vehicles: List<EvVehicleOption> = emptyList(),
    val selectedVehicleId: String? = null,
    val socInput: String = "30",
    val reserveInput: String = "10",
    val consumptionInput: String = "180",
    val targetInput: String = "80",
    val phevShareInput: String = "100",
    val plan: EvTripPlan? = null,
    val planning: Boolean = false,
    val error: String? = null,
    val replannedAfterFailure: Boolean = false,
    val failureSiteId: String? = null,
) {
    val selectedVehicle: EvVehicleOption?
        get() = vehicles.firstOrNull { it.id == selectedVehicleId }
}

/**
 * MAP-28 owner-declared EV inputs over the MAP-27 planner. Inputs are persisted in
 * [SavedStateHandle]; a process restart can replan offline. A new calculation clears the old
 * result before any callback, and a failed primary is replanned from its arrival SOC with no
 * charge from the failed charger.
 */
@HiltViewModel
class EvTripViewModel @Inject constructor(
    private val planner: PlanEvTripUseCase,
    private val vehicles: VehicleRepository,
    private val saved: SavedStateHandle,
) : ViewModel() {
    internal var nowProvider: () -> Instant = Instant::now

    private val keys = listOf(
        "ev-soc", "ev-reserve", "ev-consumption", "ev-target", "ev-phev-share", "ev-vehicle",
    )
    private val _ui = MutableStateFlow(
        EvTripUiState(
            socInput = saved.get<String>(keys[0]) ?: "30",
            reserveInput = saved.get<String>(keys[1]) ?: "10",
            consumptionInput = saved.get<String>(keys[2]) ?: "180",
            targetInput = saved.get<String>(keys[3]) ?: "80",
            phevShareInput = saved.get<String>(keys[4]) ?: "100",
        ),
    )
    val ui: StateFlow<EvTripUiState> = _ui.asStateFlow()
    private var job: Job? = null
    private var generation = 0L
    private var lastInput: EvTripInput? = null

    init {
        loadVehicles()
    }

    fun loadVehicles() {
        viewModelScope.launch {
            val loaded = try {
                vehicles.listAll()
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                _ui.update { it.copy(error = "vehicle-load-failed") }
                return@launch
            }
            val options = loaded.mapNotNull(::option)
            val persisted = saved.get<String>(keys[5])
            val selected = options.firstOrNull { it.id == persisted }?.id ?: options.firstOrNull()?.id
            if (selected != null) saved[keys[5]] = selected
            _ui.update {
                it.copy(
                    vehicles = options,
                    selectedVehicleId = selected,
                    error = if (options.isEmpty()) "no-electric-vehicle" else null,
                )
            }
        }
    }

    fun selectVehicle(id: String) {
        val option = _ui.value.vehicles.firstOrNull { it.id == id } ?: return
        saved[keys[5]] = option.id
        invalidate()
        _ui.update { it.copy(selectedVehicleId = option.id, error = null) }
    }

    fun onSocChanged(value: String) = updateInput(keys[0], value) { it.copy(socInput = value) }
    fun onReserveChanged(value: String) = updateInput(keys[1], value) { it.copy(reserveInput = value) }
    fun onConsumptionChanged(value: String) = updateInput(keys[2], value) { it.copy(consumptionInput = value) }
    fun onTargetChanged(value: String) = updateInput(keys[3], value) { it.copy(targetInput = value) }
    fun onPhevShareChanged(value: String) = updateInput(keys[4], value) { it.copy(phevShareInput = value) }

    fun plan(
        origin: RoutePoint,
        waypoints: List<RoutePoint>,
        destination: RoutePoint,
        avoid: Set<RoadAvoidance>,
        language: String,
    ) {
        val current = _ui.value
        val vehicle = current.selectedVehicle ?: return fail("no-electric-vehicle")
        if (vehicle.connector == null) return fail("vehicle-connector-missing")
        val soc = parsePercent(current.socInput, 0, 100) ?: return fail("invalid-soc")
        val reserve = parsePercent(current.reserveInput, 0, 100) ?: return fail("invalid-reserve")
        val target = parsePercent(current.targetInput, 0, 100) ?: return fail("invalid-target")
        val consumption = parseConsumption(current.consumptionInput) ?: return fail("invalid-consumption")
        val portion = if (vehicle.propulsion == Propulsion.PHEV) {
            parsePercent(current.phevShareInput, 1, 100)?.let { (it / 10).coerceAtLeast(1) }
                ?: return fail("invalid-phev-share")
        } else {
            null
        }
        val input = EvTripInput(
            origin = origin,
            destination = destination,
            waypoints = waypoints.take(100),
            departureAt = nowProvider(),
            battery = BatteryInput(vehicle.capacityWh, soc, reserve, portion),
            consumption = ConsumptionInput(consumption),
            vehicleConnector = vehicle.connector,
            avoid = avoid,
            chargeTargetSocBp = target,
            language = language,
        )
        val validation = EvChargingPlanRules.validate(input)
        if (validation.isNotEmpty()) return fail(validation.first().code.uiCode())
        run(input, replannedAfterFailure = false, failureSiteId = null)
    }

    /** The failed charger gives no energy: the replan starts from the arrival SOC at its site. */
    fun markPrimaryFailed() {
        val plan = _ui.value.plan ?: return
        val stop = plan.stops.firstOrNull() ?: return
        val previous = lastInput ?: return
        val geometry = (plan.mainRoute?.road as? EvRoadLeg.Ready)?.geometry.orEmpty()
        val primaryPoint = stop.candidate.point
        val primaryAlong = if (geometry.size >= 2) {
            EvRouteGeometry.project(primaryPoint, geometry)?.alongMetres ?: 0L
        } else {
            0L
        }
        val remaining = previous.waypoints.filter { waypoint ->
            if (geometry.size < 2) return@filter true
            val along = EvRouteGeometry.project(
                GeoPoint.fromDouble(waypoint.latitude, waypoint.longitude), geometry,
            )?.alongMetres ?: Long.MAX_VALUE
            along > primaryAlong + 1
        }
        val failurePoint = RoutePoint(primaryPoint.latitude, primaryPoint.longitude, stop.candidate.siteName)
        val failure = EvFailure(
            EvFailureScope.SITE,
            siteId = stop.candidate.siteId,
            equipmentId = stop.candidate.equipmentId,
            connectorId = stop.candidate.connectorId,
        )
        val next = previous.copy(
            origin = failurePoint,
            waypoints = remaining,
            initialPoint = failurePoint,
            initialSocBp = stop.arrivalSocBp,
            failures = previous.failures + failure,
            departureAt = nowProvider(),
        )
        run(next, replannedAfterFailure = true, failureSiteId = stop.candidate.siteId)
    }

    fun cancelPlan() {
        generation++
        job?.cancel()
        job = null
        _ui.update { it.copy(plan = null, planning = false, error = "cancelled") }
    }

    fun clear() {
        invalidate()
        _ui.update {
            it.copy(
                plan = null, planning = false, error = null,
                replannedAfterFailure = false, failureSiteId = null,
            )
        }
    }

    private fun run(input: EvTripInput, replannedAfterFailure: Boolean, failureSiteId: String?) {
        lastInput = input
        val token = invalidate()
        _ui.update {
            it.copy(
                plan = null,
                planning = true,
                error = null,
                replannedAfterFailure = replannedAfterFailure,
                failureSiteId = failureSiteId,
            )
        }
        job = viewModelScope.launch {
            try {
                val result = planner(input)
                if (token == generation) {
                    _ui.update { it.copy(plan = result, planning = false, error = null) }
                }
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                if (token == generation) {
                    _ui.update { it.copy(planning = false, error = "plan-failed") }
                }
            }
        }
    }

    private fun fail(code: String) {
        invalidate()
        _ui.update {
            it.copy(plan = null, planning = false, error = code, replannedAfterFailure = false, failureSiteId = null)
        }
    }

    private fun invalidate(): Long {
        val token = ++generation
        job?.cancel()
        job = null
        return token
    }

    private fun updateInput(key: String, value: String, update: (EvTripUiState) -> EvTripUiState) {
        saved[key] = value
        invalidate()
        _ui.update {
            update(it).copy(
                plan = null, planning = false, error = null,
                replannedAfterFailure = false, failureSiteId = null,
            )
        }
    }

    private fun option(vehicle: Vehicle): EvVehicleOption? {
        if (vehicle.propulsion != Propulsion.BEV && vehicle.propulsion != Propulsion.PHEV) return null
        val capacity = vehicle.batteryCapacity?.wattHours ?: return null
        return EvVehicleOption(
            id = vehicle.id.value,
            name = vehicle.displayName,
            capacityWh = capacity,
            connector = vehicle.chargeConnector,
            propulsion = vehicle.propulsion,
        )
    }

    private fun parsePercent(raw: String, min: Int, max: Int): Long? {
        val parsed = decimal(raw) ?: return null
        if (parsed.scale() > 2) return null
        val basisPoints = try {
            parsed.setScale(2, RoundingMode.UNNECESSARY).movePointRight(2).longValueExact()
        } catch (_: ArithmeticException) {
            return null
        }
        if (basisPoints < min * 100L || basisPoints > max * 100L) return null
        return basisPoints
    }

    private fun parseConsumption(raw: String): Long? {
        val parsed = decimal(raw) ?: return null
        if (parsed.scale() > 3) return null
        val milli = try {
            parsed.setScale(3, RoundingMode.UNNECESSARY).movePointRight(3).longValueExact()
        } catch (_: ArithmeticException) {
            return null
        }
        return milli.takeIf { it in 1..EvEnergyModel.MAX_CONSUMPTION_MILLI }
    }

    private fun decimal(raw: String): BigDecimal? =
        runCatching { BigDecimal(raw.trim().replace(',', '.')) }.getOrNull()

    private fun EvIssueCode.uiCode(): String = when (this) {
        EvIssueCode.BATTERY_MISSING -> "missing-battery"
        EvIssueCode.CONSUMPTION_MISSING, EvIssueCode.INVALID_CONSUMPTION -> "invalid-consumption"
        EvIssueCode.VEHICLE_CONNECTOR_MISSING -> "vehicle-connector-missing"
        EvIssueCode.INVALID_TARGET_SOC -> "invalid-target"
        EvIssueCode.INVALID_ROUTE -> "invalid-route"
        EvIssueCode.INVALID_BATTERY -> "invalid-battery"
        EvIssueCode.INVALID_LANGUAGE -> "invalid-language"
        else -> "invalid-input"
    }

    override fun onCleared() {
        generation++
        job?.cancel()
        super.onCleared()
    }
}
