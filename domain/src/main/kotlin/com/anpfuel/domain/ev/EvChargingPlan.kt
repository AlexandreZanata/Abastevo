package com.anpfuel.domain.ev

import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.map.saved.GeoPoint
import com.anpfuel.domain.routing.RoadAvoidance
import com.anpfuel.domain.routing.RoutePoint
import java.math.BigInteger
import java.time.Instant
import java.time.ZoneId
import kotlin.math.cos
import kotlin.math.max
import kotlin.math.min

/** MAP-27 typed candidate access. PRIVATE is never an automatic stop; UNKNOWN stays visible. */
enum class EvAccess { PUBLIC, UNKNOWN, PRIVATE }

/** Static reference and dated community report stay distinct. No live status is admitted. */
enum class EvProvenance { STATIC_REFERENCE, COMMUNITY_REPORT }

enum class EvHoursStatus { OPEN, CLOSED, UNKNOWN }

/**
 * Opening hours are either unknown, explicitly closed, or a real daily window with a real zone.
 * No partial string is interpreted and no single Brazil zone is assumed.
 */
sealed interface EvHours {
    data object Unknown : EvHours
    data object Closed : EvHours

    data class Daily(
        val openMinuteOfDay: Int,
        val closeMinuteOfDay: Int,
        val zoneId: String,
    ) : EvHours {
        init {
            require(openMinuteOfDay in 0..1439)
            require(closeMinuteOfDay in 0..1439)
            require(zoneId.isNotBlank() && zoneId.length <= 64) { "invalid-zone" }
            ZoneId.of(zoneId)
        }
    }

    fun statusAt(instant: Instant?): EvHoursStatus = when (this) {
        Unknown -> EvHoursStatus.UNKNOWN
        Closed -> EvHoursStatus.CLOSED
        is Daily -> {
            if (instant == null) {
                EvHoursStatus.UNKNOWN
            } else if (openMinuteOfDay == closeMinuteOfDay) {
                EvHoursStatus.CLOSED
            } else {
                val local = instant.atZone(ZoneId.of(zoneId)).toLocalTime()
                val minute = local.hour * 60 + local.minute
                val open = if (openMinuteOfDay < closeMinuteOfDay) {
                    minute >= openMinuteOfDay && minute < closeMinuteOfDay
                } else {
                    minute >= openMinuteOfDay || minute < closeMinuteOfDay
                }
                if (open) EvHoursStatus.OPEN else EvHoursStatus.CLOSED
            }
        }
    }
}

/** Tariff is exact milli-BRL per kWh with provenance, or typed unknown. Unknown is never zero. */
sealed interface EvTariff {
    data object Unknown : EvTariff

    data class Known(
        val amountMilliBrlPerKwh: Long,
        val condition: String,
        val source: String,
        val observedAt: Instant,
    ) : EvTariff {
        init {
            require(amountMilliBrlPerKwh in 1..1_000_000_000)
            require(condition.isNotBlank() && condition.length <= 64)
            require(source.isNotBlank() && source.length <= 128)
        }
    }
}

/**
 * One connector-level charging candidate. Identity is explicit; site/equipment scope stays
 * available so a duplicate connector is never counted as an independent fallback.
 */
data class EvChargingCandidate(
    val candidateId: String,
    val siteId: String,
    val siteName: String,
    val equipmentId: String,
    val connectorId: String,
    val connector: ChargingConnector,
    val current: ChargingCurrent,
    val powerW: Long?,
    val access: EvAccess,
    val hours: EvHours,
    val tariff: EvTariff,
    val provenance: EvProvenance,
    val sourceRevision: String,
    val observedAt: Instant?,
    val point: GeoPoint,
) {
    init {
        require(candidateId.isNotBlank() && candidateId.length <= 256)
        require(siteId.isNotBlank() && siteId.length <= 256)
        require(siteName.isNotBlank() && siteName.length <= 512)
        require(equipmentId.isNotBlank() && equipmentId.length <= 256)
        require(connectorId.isNotBlank() && connectorId.length <= 256)
        require(powerW == null || powerW in 1..1_000_000_000)
        require(sourceRevision.isNotBlank() && sourceRevision.length <= 256)
    }
}

enum class EvFailureScope { SITE, EQUIPMENT, CONNECTOR }

data class EvFailure(
    val scope: EvFailureScope,
    val siteId: String? = null,
    val equipmentId: String? = null,
    val connectorId: String? = null,
) {
    init {
        require(!siteId.isNullOrBlank())
        when (scope) {
            EvFailureScope.SITE -> Unit
            EvFailureScope.EQUIPMENT -> require(!equipmentId.isNullOrBlank())
            EvFailureScope.CONNECTOR -> require(!connectorId.isNullOrBlank())
        }
    }
}

enum class EvRoadUnavailable { NO_ROUTE, MISSING_MAPS, CANCELLED, TIMEOUT, BUDGET_EXHAUSTED, NOT_READY, UNKNOWN }

sealed interface EvRoadLeg {
    data class Ready(
        val distanceMeters: Long,
        val durationSeconds: Long?,
        val warnings: Boolean,
        val geometry: List<RoutePoint> = emptyList(),
        val missingMaps: List<String> = emptyList(),
    ) : EvRoadLeg {
        init {
            require(distanceMeters > 0)
            require(durationSeconds == null || durationSeconds > 0)
        }
    }

    data class Unavailable(val reason: EvRoadUnavailable) : EvRoadLeg
}

/** Real road distance and geometry for one ordered leg. Implemented by the pinned native adapter. */
interface EvRoadRouter {
    suspend fun leg(points: List<RoutePoint>, avoid: Set<RoadAvoidance>, language: String): EvRoadLeg

    suspend fun leg(points: List<RoutePoint>, language: String): EvRoadLeg =
        leg(points, emptySet(), language)

    suspend fun leg(from: RoutePoint, to: RoutePoint, language: String): EvRoadLeg =
        leg(listOf(from, to), emptySet(), language)
}

enum class EvCoverage { PRESENT, MISSING, UNKNOWN }

data class EvCandidateQuery(
    val route: List<RoutePoint>,
    val origin: GeoPoint,
    val connector: ChargingConnector,
    val corridorRadiusMeters: Int,
    val originRadiusMeters: Int,
    val limit: Int,
)

data class EvCandidateResult(
    val candidates: List<EvChargingCandidate>,
    val coverage: EvCoverage,
    val generation: Long? = null,
    val sourceRevision: String? = null,
    val savedAtMillis: Long? = null,
)

/** Offline-first charging candidate source. The caller never downloads a national base here. */
interface EvCandidateSource {
    suspend fun candidates(query: EvCandidateQuery): EvCandidateResult
}

enum class EvIssueCode {
    INVALID_ROUTE,
    INVALID_BATTERY,
    INVALID_CONSUMPTION,
    INVALID_TARGET_SOC,
    INVALID_LIMITS,
    INVALID_FAILURE,
    INVALID_LANGUAGE,
    BATTERY_MISSING,
    CONSUMPTION_MISSING,
    VEHICLE_CONNECTOR_MISSING,
    SNAPSHOT_MISSING,
    COVERAGE_UNKNOWN,
    NO_COMPATIBLE_CANDIDATE,
    INCOMPATIBLE_CONNECTOR,
    DUPLICATE_CANDIDATE,
    NO_ROAD_ROUTE,
    MISSING_MAPS,
    ROUTE_CANCELLED,
    ROUTE_TIMEOUT,
    ROUTE_NOT_READY,
    ROUTE_BUDGET_EXHAUSTED,
    ROAD_WARNING,
    ROAD_DISTANCE_UNKNOWN,
    ACCESS_PRIVATE,
    ACCESS_UNKNOWN,
    HOURS_CLOSED,
    HOURS_UNKNOWN,
    POWER_UNKNOWN,
    TARIFF_UNKNOWN,
    REPORT_DATE_UNKNOWN,
    STOP_UNREACHABLE,
    DESTINATION_UNREACHABLE,
    STOP_LIMIT_REACHED,
    CHARGE_TARGET_ALREADY_MET,
    ALTERNATIVE_SAME_SITE,
    ALTERNATIVE_UNREACHABLE,
    ALTERNATIVE_CLOSED,
    ALTERNATIVE_PRIVATE,
    ALTERNATIVE_UNKNOWN_ROAD,
    NO_EVIDENCED_ALTERNATIVE,
    ENERGY_UNKNOWN,
    INCOMPLETE,
    CANCELLED,
}

data class EvIssue(val code: EvIssueCode, val candidateId: String? = null)

enum class EvPlanStatus {
    PLANNED,
    INVALID_INPUT,
    NO_ROUTE,
    INSUFFICIENT_DATA,
    NO_CANDIDATES,
    INSUFFICIENT_ENERGY,
    BUDGET_EXHAUSTED,
    CANCELLED,
}

data class EvTripInput(
    val origin: RoutePoint,
    val destination: RoutePoint,
    val waypoints: List<RoutePoint> = emptyList(),
    val departureAt: Instant?,
    val battery: BatteryInput,
    val consumption: ConsumptionInput,
    val vehicleConnector: ChargingConnector?,
    val avoid: Set<RoadAvoidance> = emptySet(),
    val chargeTargetSocBp: Long,
    val maxStops: Int = 3,
    val corridorRadiusMeters: Int = 25_000,
    val maxCandidates: Int = 200,
    val maxRouteCalls: Int = 48,
    val maxCandidateProbes: Int = 6,
    val maxAlternativeChecks: Int = 6,
    val language: String = "en",
    val failures: List<EvFailure> = emptyList(),
    val initialPoint: RoutePoint? = null,
    val initialSocBp: Long? = null,
)

data class EvPlanAssumptions(
    val targetSocBp: Long,
    val reserveBp: Long,
    val consumptionMarginPermille: Long,
    val departureAt: Instant?,
    val corridorRadiusMeters: Int,
    val maxStops: Int,
    val maxRouteCalls: Int,
    val elevationModelled: Boolean = false,
    val trafficModelled: Boolean = false,
    val liveAvailabilityAdmitted: Boolean = false,
    val sourceProfile: String? = null,
    val sourceRevision: String? = null,
    val generation: Long? = null,
)

data class EvPlannedLeg(
    val from: RoutePoint,
    val to: RoutePoint,
    val road: EvRoadLeg,
    val energyWh: EnergyEstimate<Long>,
    val arrivalSocBp: EnergyEstimate<Long>,
)

data class EvAlternative(
    val candidate: EvChargingCandidate,
    val leg: EvPlannedLeg,
    val arrivalSocBp: EnergyEstimate<Long>,
    val energyReachable: Boolean,
    val hoursStatus: EvHoursStatus,
    val issues: List<EvIssueCode>,
)

data class EvPlannedStop(
    val candidate: EvChargingCandidate,
    val arrival: EvPlannedLeg,
    val arrivalSocBp: Long,
    val targetSocBp: Long,
    val chargeEnergyWh: Long,
    val minimumChargeSeconds: EnergyEstimate<Long>,
    val estimatedCostMilliBrl: EnergyEstimate<Long>,
    val arrivalAt: Instant?,
    val hoursStatus: EvHoursStatus,
    val issues: List<EvIssueCode>,
    val alternative: EvAlternative?,
    val alternativeIssues: List<EvIssueCode>,
)

data class EvTripPlan(
    val status: EvPlanStatus,
    val scope: EnergyScope,
    val origin: RoutePoint,
    val destination: RoutePoint,
    val mainRoute: EvPlannedLeg?,
    val stops: List<EvPlannedStop>,
    val finalLeg: EvPlannedLeg?,
    val arrivalSocBp: EnergyEstimate<Long>?,
    val arrivalAt: Instant?,
    val coverage: EvCoverage,
    val candidateCount: Int,
    val routeCalls: Int,
    val issues: List<EvIssue>,
    val assumptions: EvPlanAssumptions,
)

data class EvRouteProjection(
    val alongMetres: Long,
    val offRouteMetres: Long,
    val segmentIndex: Int,
)

data class EvBoundingBox(
    val minLatitude: Double,
    val maxLatitude: Double,
    val minLongitude: Double,
    val maxLongitude: Double,
) {
    init {
        require(minLatitude in -90.0..90.0 && maxLatitude in -90.0..90.0)
        require(minLongitude in -180.0..180.0 && maxLongitude in -180.0..180.0)
        require(minLatitude <= maxLatitude)
        require(minLongitude <= maxLongitude)
    }
}

/**
 * Prefilter geometry only. Along/off-route metres are approximations used to bound the
 * candidate list; every drivable leg must still come from [EvRoadRouter].
 */
object EvRouteGeometry {
    private const val EARTH_RADIUS_METRES = 6_371_000.0

    fun boundingBox(route: List<RoutePoint>, paddingMeters: Int): EvBoundingBox {
        require(route.isNotEmpty())
        require(paddingMeters >= 0)
        val minLat = route.minOf { it.latitude }
        val maxLat = route.maxOf { it.latitude }
        val minLon = route.minOf { it.longitude }
        val maxLon = route.maxOf { it.longitude }
        val meanLat = ((minLat + maxLat) / 2.0).coerceIn(-80.0, 80.0)
        val latPad = paddingMeters / 111_320.0
        val lonPad = paddingMeters / max(1.0, 111_320.0 * cos(Math.toRadians(meanLat)))
        return EvBoundingBox(
            minLatitude = (minLat - latPad).coerceIn(-90.0, 90.0),
            maxLatitude = (maxLat + latPad).coerceIn(-90.0, 90.0),
            minLongitude = (minLon - lonPad).coerceIn(-180.0, 180.0),
            maxLongitude = (maxLon + lonPad).coerceIn(-180.0, 180.0),
        )
    }

    fun distanceMeters(aLat: Double, aLon: Double, bLat: Double, bLon: Double): Double {
        val dLat = Math.toRadians(bLat - aLat)
        val dLon = Math.toRadians(bLon - aLon)
        val sLat = Math.sin(dLat / 2)
        val sLon = Math.sin(dLon / 2)
        val h = sLat * sLat + Math.cos(Math.toRadians(aLat)) * Math.cos(Math.toRadians(bLat)) * sLon * sLon
        return 2 * EARTH_RADIUS_METRES * Math.asin(min(1.0, Math.sqrt(h)))
    }

    /** Nearest projection of [point] on the route, with cumulative along-route metres. */
    fun project(point: GeoPoint, route: List<RoutePoint>): EvRouteProjection? {
        if (route.size < 2) return null
        val pLat = point.latitude
        val pLon = point.longitude
        val cosLat = cos(Math.toRadians(pLat))
        fun x(lon: Double) = Math.toRadians(lon - pLon) * EARTH_RADIUS_METRES * cosLat
        fun y(lat: Double) = Math.toRadians(lat - pLat) * EARTH_RADIUS_METRES
        var cumulative = 0.0
        var bestAlong = 0.0
        var bestOff = Double.MAX_VALUE
        var bestSegment = 0
        for (index in 0 until route.lastIndex) {
            val a = route[index]
            val b = route[index + 1]
            val ax = x(a.longitude)
            val ay = y(a.latitude)
            val bx = x(b.longitude)
            val by = y(b.latitude)
            val dx = bx - ax
            val dy = by - ay
            val lengthSquared = dx * dx + dy * dy
            val t = if (lengthSquared <= 0.0) 0.0 else ((-ax * dx - ay * dy) / lengthSquared).coerceIn(0.0, 1.0)
            val px = ax + t * dx
            val py = ay + t * dy
            val off = Math.sqrt(px * px + py * py)
            val segmentLength = distanceMeters(a.latitude, a.longitude, b.latitude, b.longitude)
            if (off < bestOff) {
                bestOff = off
                bestAlong = cumulative + t * segmentLength
                bestSegment = index
            }
            cumulative += segmentLength
        }
        return EvRouteProjection(
            alongMetres = bestAlong.toLong(),
            offRouteMetres = bestOff.toLong(),
            segmentIndex = bestSegment,
        )
    }
}

/** Pure MAP-27 rules shared by the orchestrator, data adapter and UI. */
object EvChargingPlanRules {
    const val MAX_STOPS = 5
    const val MAX_CANDIDATES = 500
    const val MAX_ROUTE_CALLS = 200
    const val MAX_PROBES = 20
    const val MAX_ALTERNATIVE_CHECKS = 20

    fun validate(input: EvTripInput): List<EvIssue> {
        val issues = mutableListOf<EvIssue>()
        val orderedPoints = listOf(input.origin) + input.waypoints + listOf(input.destination)
        if (input.waypoints.size > 100 || orderedPoints.any { !validPoint(it) }) {
            issues += EvIssue(EvIssueCode.INVALID_ROUTE)
        } else if (orderedPoints.map { it.latitude to it.longitude }.distinct().size != orderedPoints.size) {
            issues += EvIssue(EvIssueCode.INVALID_ROUTE)
        }
        if (!input.language.matches(Regex("[a-z]{2}(-[A-Za-z]{2,4})?"))) issues += EvIssue(EvIssueCode.INVALID_LANGUAGE)
        val batteryIssues = when (val usable = EvEnergyModel.usableEnergyWh(input.battery)) {
            is EnergyEstimate.Unknown -> listOf(
                EvIssue(
                    when (usable.reason) {
                        "capacity-missing" -> EvIssueCode.BATTERY_MISSING
                        else -> EvIssueCode.INVALID_BATTERY
                    },
                ),
            )
            is EnergyEstimate.Known -> if (input.initialSocBp != null && input.initialSocBp !in 0..10_000) {
                listOf(EvIssue(EvIssueCode.INVALID_BATTERY))
            } else {
                emptyList()
            }
        }
        issues += batteryIssues
        if (EvEnergyModel.segmentEnergyWh(0, input.consumption) is EnergyEstimate.Unknown) {
            val reason = (EvEnergyModel.segmentEnergyWh(0, input.consumption) as EnergyEstimate.Unknown).reason
            issues += EvIssue(if (reason == "consumption-missing") EvIssueCode.CONSUMPTION_MISSING else EvIssueCode.INVALID_CONSUMPTION)
        }
        if (input.vehicleConnector == null || input.vehicleConnector == ChargingConnector.UNKNOWN) {
            issues += EvIssue(EvIssueCode.VEHICLE_CONNECTOR_MISSING)
        }
        val reserve = input.battery.reserveBp
        if (input.chargeTargetSocBp !in 0..10_000 || (reserve != null && input.chargeTargetSocBp <= reserve)) {
            issues += EvIssue(EvIssueCode.INVALID_TARGET_SOC)
        }
        if (input.maxStops !in 0..MAX_STOPS ||
            input.corridorRadiusMeters !in 1_000..100_000 ||
            input.maxCandidates !in 1..MAX_CANDIDATES ||
            input.maxRouteCalls !in 1..MAX_ROUTE_CALLS ||
            input.maxCandidateProbes !in 1..MAX_PROBES ||
            input.maxAlternativeChecks !in 1..MAX_ALTERNATIVE_CHECKS
        ) {
            issues += EvIssue(EvIssueCode.INVALID_LIMITS)
        }
        if (input.failures.any { failure ->
                failure.siteId.isNullOrBlank() ||
                    (failure.scope == EvFailureScope.EQUIPMENT && failure.equipmentId.isNullOrBlank()) ||
                    (failure.scope == EvFailureScope.CONNECTOR && failure.connectorId.isNullOrBlank())
            }
        ) {
            issues += EvIssue(EvIssueCode.INVALID_FAILURE)
        }
        return issues
    }

    fun eligibility(
        candidate: EvChargingCandidate,
        input: EvTripInput,
        arrivalAt: Instant?,
    ): List<EvIssueCode> {
        val issues = mutableListOf<EvIssueCode>()
        if (candidate.connector != input.vehicleConnector) issues += EvIssueCode.INCOMPATIBLE_CONNECTOR
        when (candidate.access) {
            EvAccess.PRIVATE -> issues += EvIssueCode.ACCESS_PRIVATE
            EvAccess.UNKNOWN -> issues += EvIssueCode.ACCESS_UNKNOWN
            EvAccess.PUBLIC -> Unit
        }
        when (candidate.hours.statusAt(arrivalAt)) {
            EvHoursStatus.CLOSED -> issues += EvIssueCode.HOURS_CLOSED
            EvHoursStatus.UNKNOWN -> issues += EvIssueCode.HOURS_UNKNOWN
            EvHoursStatus.OPEN -> Unit
        }
        if (candidate.powerW == null) issues += EvIssueCode.POWER_UNKNOWN
        if (candidate.tariff == EvTariff.Unknown) issues += EvIssueCode.TARIFF_UNKNOWN
        if (candidate.observedAt == null) issues += EvIssueCode.REPORT_DATE_UNKNOWN
        return issues
    }

    fun excluded(candidate: EvChargingCandidate, failures: List<EvFailure>, alternative: Boolean): Boolean {
        if (alternative && failures.any { it.siteId == candidate.siteId }) return true
        return failures.any { failure ->
            when (failure.scope) {
                EvFailureScope.SITE -> failure.siteId == candidate.siteId
                EvFailureScope.EQUIPMENT -> failure.equipmentId == candidate.equipmentId
                EvFailureScope.CONNECTOR -> failure.connectorId == candidate.connectorId
            }
        }
    }

    /** Deterministic one-per-site choice; another connector at the same site is not an option. */
    fun bestPerSite(candidates: List<EvChargingCandidate>): List<EvChargingCandidate> =
        candidates.groupBy { it.siteId }.values.map { group ->
            group.sortedWith(
                compareByDescending<EvChargingCandidate> { it.powerW ?: -1 }
                    .thenByDescending { it.current == ChargingCurrent.DC }
                    .thenBy { it.candidateId },
            ).first()
        }

    fun energyToTargetWh(
        battery: BatteryInput,
        currentSocBp: Long,
        targetSocBp: Long,
    ): EnergyEstimate<Long> {
        val capacity = battery.capacityWh ?: return EnergyEstimate.Unknown("capacity-missing")
        val portion = battery.usablePermille
        if (capacity !in 1..EvEnergyModel.MAX_ENERGY_WH) return EnergyEstimate.Unknown("input-out-of-range")
        if (currentSocBp !in 0..10_000 || targetSocBp !in 0..10_000) return EnergyEstimate.Unknown("input-out-of-range")
        if (portion != null && portion !in 1..1_000) return EnergyEstimate.Unknown("input-out-of-range")
        if (targetSocBp <= currentSocBp) return EnergyEstimate.Known(0)
        val effectivePortion = portion ?: 1_000
        val numerator = BigInteger.valueOf(capacity)
            .multiply(BigInteger.valueOf(targetSocBp - currentSocBp))
            .multiply(BigInteger.valueOf(effectivePortion))
        val denominator = BigInteger.valueOf(10_000L * 1_000L)
        val value = numerator.add(denominator).subtract(BigInteger.ONE).divide(denominator)
        return if (value > BigInteger.valueOf(EvEnergyModel.MAX_ENERGY_WH)) {
            EnergyEstimate.Unknown("out-of-bounds")
        } else {
            EnergyEstimate.Known(value.toLong())
        }
    }

    fun chargeCostMilliBrl(energyWh: Long, tariff: EvTariff): EnergyEstimate<Long> {
        if (energyWh < 0) return EnergyEstimate.Unknown("negative-input")
        return when (tariff) {
            EvTariff.Unknown -> EnergyEstimate.Unknown("tariff-unknown")
            is EvTariff.Known -> {
                val numerator = BigInteger.valueOf(energyWh).multiply(BigInteger.valueOf(tariff.amountMilliBrlPerKwh))
                val denominator = BigInteger.valueOf(1_000)
                EnergyEstimate.Known(numerator.add(denominator).subtract(BigInteger.ONE).divide(denominator).toLong())
            }
        }
    }

    fun arrivalAt(departureAt: Instant?, seconds: Long?): Instant? {
        if (departureAt == null || seconds == null || seconds < 0) return null
        return runCatching { departureAt.plusSeconds(seconds) }.getOrNull()
    }

    private fun validPoint(point: RoutePoint): Boolean =
        point.latitude.isFinite() && point.longitude.isFinite() &&
            point.latitude in -90.0..90.0 && point.longitude in -180.0..180.0 &&
            point.label.length <= 512

    /** Independent failure vector helper: state at the failed primary, no energy from it. */
    fun failureReachability(
        battery: BatteryInput,
        arrivalSocBpAtPrimary: Long,
        consumption: ConsumptionInput,
        alternativeDistanceMeters: Long,
    ): EnergyEstimate<Long> {
        val capacity = battery.capacityWh ?: return EnergyEstimate.Unknown("capacity-missing")
        val portion = battery.usablePermille
        val effectiveCapacity = if (portion == null) capacity else (capacity * portion / 1_000L).coerceAtLeast(1L)
        return EvEnergyModel.arrivalSocBp(
            capacityWh = effectiveCapacity,
            socBp = arrivalSocBpAtPrimary,
            distanceMetres = alternativeDistanceMeters,
            consumption = consumption,
        )
    }

    fun isReachable(arrivalSocBp: Long?, reserveBp: Long?): Boolean =
        arrivalSocBp != null && reserveBp != null && arrivalSocBp >= reserveBp
}
