package com.anpfuel.application.usecase.ev

import com.anpfuel.domain.ev.*
import com.anpfuel.domain.map.saved.GeoPoint
import com.anpfuel.domain.routing.RoadAvoidance
import com.anpfuel.domain.routing.RoutePoint
import java.time.Instant
import kotlin.coroutines.coroutineContext
import kotlinx.coroutines.ensureActive

/**
 * MAP-27 orchestration: native road legs stay behind [EvRoadRouter], candidates behind
 * [EvCandidateSource], and all energy/eligibility rules stay in the pure domain.
 *
 * Every native call consumes one bounded budget. A late callback cannot publish because the
 * router adapter owns its generation; a cancelled coroutine stops before the next call. The
 * planner never substitutes a straight line for a road leg and never uses charge from a failed
 * primary to claim a reachable fallback.
 */
class PlanEvTripUseCase(
    private val router: EvRoadRouter,
    private val candidates: EvCandidateSource,
    private val nanoTime: () -> Long = System::nanoTime,
    private val deadlineNanos: Long = 30_000_000_000L,
) {
    operator suspend fun invoke(input: EvTripInput): EvTripPlan {
        val issues = EvChargingPlanRules.validate(input).toMutableList()
        val reserveBp = input.battery.reserveBp
        val start = input.initialPoint ?: input.origin
        val effectiveBattery = input.battery.copy(socBp = input.initialSocBp ?: input.battery.socBp)
        val assumptions = EvPlanAssumptions(
            targetSocBp = input.chargeTargetSocBp,
            reserveBp = input.battery.reserveBp ?: 0L,
            consumptionMarginPermille = input.consumption.marginPermille ?: 0L,
            departureAt = input.departureAt,
            corridorRadiusMeters = input.corridorRadiusMeters,
            maxStops = input.maxStops,
            maxRouteCalls = input.maxRouteCalls,
        )
        if (issues.isNotEmpty()) {
            return emptyPlan(input, start, effectiveBattery, EvPlanStatus.INVALID_INPUT, EvCoverage.UNKNOWN, issues, assumptions, 0)
        }

        val budget = Budget(input.maxRouteCalls, nanoTime() + deadlineNanos, nanoTime)
        val orderedPoints = distinctPoints(listOf(start) + input.waypoints + listOf(input.destination))
        val mainRoad = call(orderedPoints, input.avoid, input.language, budget)
        val mainLeg = plannedLeg(start, input.destination, mainRoad, effectiveBattery, input.consumption)
        if (mainRoad !is EvRoadLeg.Ready) {
            val status = when ((mainRoad as EvRoadLeg.Unavailable).reason) {
                EvRoadUnavailable.CANCELLED -> EvPlanStatus.CANCELLED
                EvRoadUnavailable.BUDGET_EXHAUSTED -> EvPlanStatus.BUDGET_EXHAUSTED
                else -> EvPlanStatus.NO_ROUTE
            }
            issues += EvIssue(mainFailureCode(mainRoad.reason))
            return emptyPlan(input, start, effectiveBattery, status, EvCoverage.UNKNOWN, issues, assumptions, budget.calls, mainLeg)
        }
        if (mainLeg.arrivalSocBp !is EnergyEstimate.Known) issues += EvIssue(EvIssueCode.ENERGY_UNKNOWN)
        if (mainLeg.road.warnings()) issues += EvIssue(EvIssueCode.ROAD_WARNING)

        val routeGeometry = mainLeg.road.geometry().ifEmpty { orderedPoints }
        val query = runCatching {
            EvCandidateQuery(
                route = routeGeometry,
                origin = GeoPoint.fromDouble(start.latitude, start.longitude),
                connector = input.vehicleConnector!!,
                corridorRadiusMeters = input.corridorRadiusMeters,
                originRadiusMeters = input.corridorRadiusMeters,
                limit = input.maxCandidates,
            )
        }.getOrElse {
            issues += EvIssue(EvIssueCode.INVALID_ROUTE)
            return emptyPlan(input, start, effectiveBattery, EvPlanStatus.INVALID_INPUT, EvCoverage.UNKNOWN, issues, assumptions, budget.calls, mainLeg)
        }
        val candidateResult = candidates.candidates(query)
        if (candidateResult.coverage != EvCoverage.PRESENT) {
            issues += EvIssue(
                if (candidateResult.coverage == EvCoverage.MISSING) EvIssueCode.SNAPSHOT_MISSING else EvIssueCode.COVERAGE_UNKNOWN,
            )
        }
        val initialArrival = (mainLeg.arrivalSocBp as? EnergyEstimate.Known)?.value
        if (initialArrival != null && reserveBp != null && initialArrival >= reserveBp) {
            return EvTripPlan(
                status = EvPlanStatus.PLANNED,
                scope = EvEnergyModel.scopeOf(effectiveBattery),
                origin = input.origin,
                destination = input.destination,
                mainRoute = mainLeg,
                stops = emptyList(),
                finalLeg = null,
                arrivalSocBp = mainLeg.arrivalSocBp,
                arrivalAt = EvChargingPlanRules.arrivalAt(input.departureAt, mainRoad.durationSecondsOrNull()),
                coverage = candidateResult.coverage,
                candidateCount = 0,
                routeCalls = budget.calls,
                issues = issues.distinct(),
                assumptions = withSource(assumptions, candidateResult),
            )
        }

        val compatiblePool = candidateResult.candidates.filter { candidate ->
            if (candidate.connector != input.vehicleConnector) {
                issues += EvIssue(EvIssueCode.INCOMPATIBLE_CONNECTOR, candidate.candidateId)
            }
            if (candidate.access == EvAccess.PRIVATE) {
                issues += EvIssue(EvIssueCode.ACCESS_PRIVATE, candidate.candidateId)
            }
            if (EvChargingPlanRules.excluded(candidate, input.failures, alternative = false)) {
                issues += EvIssue(EvIssueCode.DUPLICATE_CANDIDATE, candidate.candidateId)
            }
            candidate.connector == input.vehicleConnector &&
                candidate.access != EvAccess.PRIVATE &&
                !EvChargingPlanRules.excluded(candidate, input.failures, alternative = false)
        }
        val projected = EvChargingPlanRules.bestPerSite(compatiblePool)
            .mapNotNull { candidate ->
                val projection = EvRouteGeometry.project(candidate.point, routeGeometry) ?: return@mapNotNull null
                if (projection.offRouteMetres > input.corridorRadiusMeters) return@mapNotNull null
                candidate to projection
            }

        var currentPoint = start
        var currentSoc = effectiveBattery.socBp ?: return emptyPlan(
            input, start, effectiveBattery, EvPlanStatus.INVALID_INPUT, candidateResult.coverage,
            issues + EvIssue(EvIssueCode.BATTERY_MISSING), assumptions, budget.calls, mainLeg,
        )
        var currentAlong = EvRouteGeometry.project(
            GeoPoint.fromDouble(currentPoint.latitude, currentPoint.longitude), routeGeometry,
        )?.alongMetres ?: 0L
        var currentDepartureAt = input.departureAt
        val stops = mutableListOf<EvPlannedStop>()
        var finalLeg: EvPlannedLeg? = null
        var status = EvPlanStatus.PLANNED
        val selectedSites = mutableSetOf<String>()

        while (true) {
            val remainingWaypoints = input.waypoints.filter {
                val projectedWaypoint = EvRouteGeometry.project(
                    GeoPoint.fromDouble(it.latitude, it.longitude), routeGeometry,
                )?.alongMetres ?: Long.MAX_VALUE
                projectedWaypoint > currentAlong + 1
            }
            val destinationPoints = distinctPoints(listOf(currentPoint) + remainingWaypoints + listOf(input.destination))
            val destinationRoad = if (stops.isEmpty()) mainRoad else call(destinationPoints, input.avoid, input.language, budget)
            if (destinationRoad is EvRoadLeg.Unavailable) {
                issues += EvIssue(mainFailureCode(destinationRoad.reason))
                status = when (destinationRoad.reason) {
                    EvRoadUnavailable.CANCELLED -> EvPlanStatus.CANCELLED
                    EvRoadUnavailable.BUDGET_EXHAUSTED -> EvPlanStatus.BUDGET_EXHAUSTED
                    else -> EvPlanStatus.NO_ROUTE
                }
                break
            }
            if (destinationRoad.warnings()) issues += EvIssue(EvIssueCode.ROAD_WARNING)
            val destinationLeg = plannedLeg(
                currentPoint, input.destination, destinationRoad,
                effectiveBattery.copy(socBp = currentSoc), input.consumption,
            )
            val destinationSoc = (destinationLeg.arrivalSocBp as? EnergyEstimate.Known)?.value
            if (destinationSoc != null && reserveBp != null && destinationSoc >= reserveBp) {
                finalLeg = destinationLeg
                break
            }
            if (stops.size >= input.maxStops) {
                issues += EvIssue(EvIssueCode.STOP_LIMIT_REACHED)
                status = EvPlanStatus.INSUFFICIENT_ENERGY
                break
            }

            val allowBehind = stops.isEmpty() && (input.initialPoint != null || input.failures.isNotEmpty())
            val nextWaypointAlong = if (allowBehind) {
                null
            } else {
                remainingWaypoints.firstOrNull()?.let {
                    EvRouteGeometry.project(GeoPoint.fromDouble(it.latitude, it.longitude), routeGeometry)?.alongMetres
                }
            }
            val ahead = projected.asSequence()
                .filter { allowBehind || it.second.alongMetres > currentAlong + 1 }
                .filter { nextWaypointAlong == null || it.second.alongMetres <= nextWaypointAlong + 1 }
                .filter { (candidate, _) -> candidate.siteId !in selectedSites }
                .filter { (candidate, _) ->
                    !EvChargingPlanRules.excluded(candidate, input.failures, alternative = false)
                }
                .sortedWith(
                    compareByDescending<Pair<EvChargingCandidate, EvRouteProjection>> { it.second.alongMetres }
                        .thenBy { it.first.siteId },
                )
                .toList()
            if (ahead.isEmpty()) {
                issues += EvIssue(EvIssueCode.NO_COMPATIBLE_CANDIDATE)
                status = if (candidateResult.coverage == EvCoverage.PRESENT) EvPlanStatus.NO_CANDIDATES else EvPlanStatus.INSUFFICIENT_DATA
                break
            }

            var chosenCandidate: EvChargingCandidate? = null
            var chosenLeg: EvPlannedLeg? = null
            var chosenProjection: EvRouteProjection? = null
            var chosenArrivalSoc = 0L
            var chosenArrivalAt: Instant? = null
            var chosenIssues: List<EvIssueCode> = emptyList()
            var chosenAlternative: EvAlternative? = null
            var fallbackCandidate: EvChargingCandidate? = null
            var fallbackLeg: EvPlannedLeg? = null
            var fallbackProjection: EvRouteProjection? = null
            var fallbackArrivalSoc = 0L
            var fallbackArrivalAt: Instant? = null
            var fallbackIssues: List<EvIssueCode> = emptyList()
            var probes = 0
            for ((candidate, projection) in ahead) {
                if (probes++ >= input.maxCandidateProbes) {
                    issues += EvIssue(EvIssueCode.INCOMPLETE, candidate.candidateId)
                    if (budget.exhausted()) status = EvPlanStatus.BUDGET_EXHAUSTED
                    break
                }
                val leg = call(distinctPoints(listOf(currentPoint, candidate.asRoutePoint())), input.avoid, input.language, budget)
                if (leg is EvRoadLeg.Unavailable) {
                    issues += EvIssue(roadFailureCode(leg.reason), candidate.candidateId)
                    if (leg.reason == EvRoadUnavailable.BUDGET_EXHAUSTED) {
                        status = EvPlanStatus.BUDGET_EXHAUSTED
                        break
                    }
                    continue
                }
                val planned = plannedLeg(
                    currentPoint, candidate.asRoutePoint(), leg,
                    effectiveBattery.copy(socBp = currentSoc), input.consumption,
                )
                if (leg.warnings()) issues += EvIssue(EvIssueCode.ROAD_WARNING, candidate.candidateId)
                val legSeconds = leg.durationSecondsOrNull()
                val arrivalAt = EvChargingPlanRules.arrivalAt(currentDepartureAt, legSeconds)
                val eligibility = EvChargingPlanRules.eligibility(candidate, input, arrivalAt)
                val blocking = eligibility.filter {
                    it == EvIssueCode.INCOMPATIBLE_CONNECTOR || it == EvIssueCode.ACCESS_PRIVATE || it == EvIssueCode.HOURS_CLOSED
                }
                if (blocking.isNotEmpty()) {
                    issues += blocking.map { EvIssue(it, candidate.candidateId) }
                    continue
                }
                val arrivalSoc = (planned.arrivalSocBp as? EnergyEstimate.Known)?.value
                if (arrivalSoc == null) {
                    issues += EvIssue(EvIssueCode.ENERGY_UNKNOWN, candidate.candidateId)
                    continue
                }
                if (!EvChargingPlanRules.isReachable(arrivalSoc, input.battery.reserveBp)) {
                    issues += EvIssue(EvIssueCode.STOP_UNREACHABLE, candidate.candidateId)
                    continue
                }
                if (arrivalSoc >= input.chargeTargetSocBp) {
                    issues += EvIssue(EvIssueCode.CHARGE_TARGET_ALREADY_MET, candidate.candidateId)
                    continue
                }
                val alternative = evaluateAlternative(
                    primary = candidate,
                    primaryArrivalSoc = arrivalSoc,
                    primaryArrivalAt = arrivalAt,
                    projected = projected,
                    allCandidates = compatiblePool,
                    selectedSites = selectedSites,
                    input = input,
                    battery = effectiveBattery,
                    budget = budget,
                    issues = issues,
                )
                if (budget.exhausted()) {
                    issues += EvIssue(EvIssueCode.ROUTE_BUDGET_EXHAUSTED, candidate.candidateId)
                    status = EvPlanStatus.BUDGET_EXHAUSTED
                    break
                }
                if (alternative != null) {
                    chosenCandidate = candidate
                    chosenLeg = planned
                    chosenProjection = projection
                    chosenArrivalSoc = arrivalSoc
                    chosenArrivalAt = arrivalAt
                    chosenIssues = eligibility
                    chosenAlternative = alternative
                    break
                }
                if (fallbackCandidate == null) {
                    fallbackCandidate = candidate
                    fallbackLeg = planned
                    fallbackProjection = projection
                    fallbackArrivalSoc = arrivalSoc
                    fallbackArrivalAt = arrivalAt
                    fallbackIssues = eligibility
                }
            }

            if (status == EvPlanStatus.BUDGET_EXHAUSTED) break
            if (chosenCandidate == null && fallbackCandidate != null) {
                chosenCandidate = fallbackCandidate
                chosenLeg = fallbackLeg
                chosenProjection = fallbackProjection
                chosenArrivalSoc = fallbackArrivalSoc
                chosenArrivalAt = fallbackArrivalAt
                chosenIssues = fallbackIssues
            }
            if (chosenCandidate == null || chosenLeg == null || chosenProjection == null) {
                issues += EvIssue(EvIssueCode.DESTINATION_UNREACHABLE)
                status = if (candidateResult.coverage == EvCoverage.PRESENT) EvPlanStatus.INSUFFICIENT_ENERGY else EvPlanStatus.INSUFFICIENT_DATA
                break
            }

            val chargeEnergy = when (val value = EvChargingPlanRules.energyToTargetWh(effectiveBattery, chosenArrivalSoc, input.chargeTargetSocBp)) {
                is EnergyEstimate.Known -> value.value
                is EnergyEstimate.Unknown -> {
                    issues += EvIssue(EvIssueCode.ENERGY_UNKNOWN, chosenCandidate.candidateId)
                    status = EvPlanStatus.INSUFFICIENT_DATA
                    break
                }
            }
            val alternative = chosenAlternative
            if (alternative == null) issues += EvIssue(EvIssueCode.NO_EVIDENCED_ALTERNATIVE, chosenCandidate.candidateId)
            stops += EvPlannedStop(
                candidate = chosenCandidate,
                arrival = chosenLeg,
                arrivalSocBp = chosenArrivalSoc,
                targetSocBp = input.chargeTargetSocBp,
                chargeEnergyWh = chargeEnergy,
                minimumChargeSeconds = EvEnergyModel.minimumChargeSeconds(chargeEnergy, chosenCandidate.powerW),
                estimatedCostMilliBrl = EvChargingPlanRules.chargeCostMilliBrl(chargeEnergy, chosenCandidate.tariff),
                arrivalAt = chosenArrivalAt,
                hoursStatus = chosenCandidate.hours.statusAt(chosenArrivalAt),
                issues = chosenIssues,
                alternative = alternative,
                alternativeIssues = if (alternative == null) listOf(EvIssueCode.NO_EVIDENCED_ALTERNATIVE) else alternative.issues,
            )
            selectedSites += chosenCandidate.siteId
            currentPoint = chosenCandidate.asRoutePoint()
            currentSoc = input.chargeTargetSocBp
            currentAlong = chosenProjection.alongMetres
            currentDepartureAt = null
        }

        return EvTripPlan(
            status = status,
            scope = EvEnergyModel.scopeOf(effectiveBattery),
            origin = input.origin,
            destination = input.destination,
            mainRoute = mainLeg,
            stops = stops,
            finalLeg = finalLeg,
            arrivalSocBp = finalLeg?.arrivalSocBp,
            arrivalAt = if (stops.isEmpty()) {
                EvChargingPlanRules.arrivalAt(input.departureAt, finalLeg?.road?.durationSecondsOrNull())
            } else {
                null
            },
            coverage = candidateResult.coverage,
            candidateCount = projected.size,
            routeCalls = budget.calls,
            issues = issues.distinct(),
            assumptions = withSource(assumptions, candidateResult),
        )
    }

    private suspend fun evaluateAlternative(
        primary: EvChargingCandidate,
        primaryArrivalSoc: Long,
        primaryArrivalAt: Instant?,
        projected: List<Pair<EvChargingCandidate, EvRouteProjection>>,
        allCandidates: List<EvChargingCandidate>,
        selectedSites: Set<String>,
        input: EvTripInput,
        battery: BatteryInput,
        budget: Budget,
        issues: MutableList<EvIssue>,
    ): EvAlternative? {
        val sameSite = allCandidates.any { it.siteId == primary.siteId && it.candidateId != primary.candidateId }
        val pool = EvChargingPlanRules.bestPerSite(allCandidates)
            .filter { it.siteId != primary.siteId }
            .filter { it.siteId !in selectedSites }
            .filter { !EvChargingPlanRules.excluded(it, input.failures, alternative = true) }
            .sortedWith(
                compareBy<EvChargingCandidate> {
                    EvRouteGeometry.distanceMeters(
                        primary.point.latitude, primary.point.longitude,
                        it.point.latitude, it.point.longitude,
                    )
                }.thenBy { it.candidateId },
            )
        var checks = 0
        for (candidate in pool) {
            if (checks++ >= input.maxAlternativeChecks) {
                issues += EvIssue(EvIssueCode.NO_EVIDENCED_ALTERNATIVE, primary.candidateId)
                break
            }
            val leg = call(distinctPoints(listOf(primary.asRoutePoint(), candidate.asRoutePoint())), input.avoid, input.language, budget)
            if (leg is EvRoadLeg.Unavailable) {
                issues += EvIssue(EvIssueCode.ALTERNATIVE_UNKNOWN_ROAD, candidate.candidateId)
                if (leg.reason == EvRoadUnavailable.BUDGET_EXHAUSTED) return null
                continue
            }
            if (leg.warnings()) issues += EvIssue(EvIssueCode.ROAD_WARNING, candidate.candidateId)
            val planned = plannedLeg(
                primary.asRoutePoint(), candidate.asRoutePoint(), leg,
                battery.copy(socBp = primaryArrivalSoc), input.consumption,
            )
            val arrivalAt = EvChargingPlanRules.arrivalAt(primaryArrivalAt, leg.durationSecondsOrNull())
            val eligibility = EvChargingPlanRules.eligibility(candidate, input, arrivalAt)
            val blocking = eligibility.filter {
                it == EvIssueCode.INCOMPATIBLE_CONNECTOR || it == EvIssueCode.ACCESS_PRIVATE || it == EvIssueCode.HOURS_CLOSED
            }
            if (blocking.isNotEmpty()) {
                issues += blocking.map { EvIssue(it, candidate.candidateId) }
                continue
            }
            val arrivalSoc = EvChargingPlanRules.failureReachability(
                battery = battery,
                arrivalSocBpAtPrimary = primaryArrivalSoc,
                consumption = input.consumption,
                alternativeDistanceMeters = leg.distanceMetersOrNull() ?: continue,
            )
            val reachable = arrivalSoc is EnergyEstimate.Known &&
                EvChargingPlanRules.isReachable(arrivalSoc.value, input.battery.reserveBp)
            if (reachable) {
                return EvAlternative(
                    candidate = candidate,
                    leg = planned,
                    arrivalSocBp = arrivalSoc,
                    energyReachable = true,
                    hoursStatus = candidate.hours.statusAt(arrivalAt),
                    issues = buildList {
                        addAll(eligibility)
                        if (leg.warnings()) add(EvIssueCode.ROAD_WARNING)
                    }.distinct(),
                )
            }
            issues += EvIssue(EvIssueCode.ALTERNATIVE_UNREACHABLE, candidate.candidateId)
        }
        if (sameSite) issues += EvIssue(EvIssueCode.ALTERNATIVE_SAME_SITE, primary.candidateId)
        return null
    }

    private suspend fun call(
        points: List<RoutePoint>,
        avoid: Set<RoadAvoidance>,
        language: String,
        budget: Budget,
    ): EvRoadLeg {
        coroutineContext.ensureActive()
        if (!budget.consume()) return EvRoadLeg.Unavailable(EvRoadUnavailable.BUDGET_EXHAUSTED)
        return router.leg(points, avoid, language)
    }

    private fun distinctPoints(points: List<RoutePoint>): List<RoutePoint> =
        points.distinctBy { it.latitude to it.longitude }

    private fun plannedLeg(
        from: RoutePoint,
        to: RoutePoint,
        road: EvRoadLeg,
        battery: BatteryInput,
        consumption: ConsumptionInput,
    ): EvPlannedLeg {
        val distance = road.distanceMetersOrNull()
        val energy = if (distance == null) {
            EnergyEstimate.Unknown("road-unavailable")
        } else {
            EvEnergyModel.segmentEnergyWh(distance, consumption)
        }
        val arrival = if (distance == null) {
            EnergyEstimate.Unknown("road-unavailable")
        } else {
            arrivalSocBattery(battery, distance, consumption)
        }
        return EvPlannedLeg(from, to, road, energy, arrival)
    }

    private fun arrivalSocBattery(
        battery: BatteryInput,
        distanceMeters: Long,
        consumption: ConsumptionInput,
    ): EnergyEstimate<Long> {
        val capacity = battery.capacityWh ?: return EnergyEstimate.Unknown("capacity-missing")
        val soc = battery.socBp ?: return EnergyEstimate.Unknown("soc-missing")
        val portion = battery.usablePermille
        val effectiveCapacity = if (portion == null) capacity else {
            (capacity * portion / 1_000L).coerceAtLeast(1L)
        }
        return EvEnergyModel.arrivalSocBp(effectiveCapacity, soc, distanceMeters, consumption)
    }

    private fun emptyPlan(
        input: EvTripInput,
        start: RoutePoint,
        battery: BatteryInput,
        status: EvPlanStatus,
        coverage: EvCoverage,
        issues: List<EvIssue>,
        assumptions: EvPlanAssumptions,
        routeCalls: Int,
        mainLeg: EvPlannedLeg? = null,
    ): EvTripPlan = EvTripPlan(
        status = status,
        scope = EvEnergyModel.scopeOf(input.battery),
        origin = input.origin,
        destination = input.destination,
        mainRoute = mainLeg,
        stops = emptyList(),
        finalLeg = null,
        arrivalSocBp = null,
        arrivalAt = null,
        coverage = coverage,
        candidateCount = 0,
        routeCalls = routeCalls,
        issues = issues.distinct(),
        assumptions = assumptions,
    )

    private fun withSource(assumptions: EvPlanAssumptions, result: EvCandidateResult): EvPlanAssumptions =
        assumptions.copy(sourceRevision = result.sourceRevision, generation = result.generation)

    private fun mainFailureCode(reason: EvRoadUnavailable): EvIssueCode = when (reason) {
        EvRoadUnavailable.CANCELLED -> EvIssueCode.ROUTE_CANCELLED
        EvRoadUnavailable.TIMEOUT -> EvIssueCode.ROUTE_TIMEOUT
        EvRoadUnavailable.MISSING_MAPS -> EvIssueCode.MISSING_MAPS
        EvRoadUnavailable.BUDGET_EXHAUSTED -> EvIssueCode.ROUTE_BUDGET_EXHAUSTED
        EvRoadUnavailable.NOT_READY -> EvIssueCode.ROUTE_NOT_READY
        EvRoadUnavailable.NO_ROUTE, EvRoadUnavailable.UNKNOWN -> EvIssueCode.NO_ROAD_ROUTE
    }

    private fun roadFailureCode(reason: EvRoadUnavailable): EvIssueCode = when (reason) {
        EvRoadUnavailable.CANCELLED -> EvIssueCode.ROUTE_CANCELLED
        EvRoadUnavailable.TIMEOUT -> EvIssueCode.ROUTE_TIMEOUT
        EvRoadUnavailable.MISSING_MAPS -> EvIssueCode.MISSING_MAPS
        EvRoadUnavailable.BUDGET_EXHAUSTED -> EvIssueCode.ROUTE_BUDGET_EXHAUSTED
        EvRoadUnavailable.NOT_READY -> EvIssueCode.ROUTE_NOT_READY
        EvRoadUnavailable.NO_ROUTE, EvRoadUnavailable.UNKNOWN -> EvIssueCode.NO_ROAD_ROUTE
    }

    private fun EvRoadLeg.warnings(): Boolean = this is EvRoadLeg.Ready && warnings

    private fun EvRoadLeg.geometry(): List<RoutePoint> = (this as? EvRoadLeg.Ready)?.geometry.orEmpty()

    private fun EvRoadLeg.distanceMetersOrNull(): Long? = (this as? EvRoadLeg.Ready)?.distanceMeters

    private fun EvRoadLeg.durationSecondsOrNull(): Long? = (this as? EvRoadLeg.Ready)?.durationSeconds

    private class Budget(
        private val maxCalls: Int,
        private val deadlineAtNanos: Long,
        private val nanoTime: () -> Long,
    ) {
        var calls: Int = 0
            private set

        fun consume(): Boolean {
            if (calls >= maxCalls || nanoTime() >= deadlineAtNanos) return false
            calls++
            return true
        }

        fun exhausted(): Boolean = calls >= maxCalls || nanoTime() >= deadlineAtNanos
    }

    private fun EvChargingCandidate.asRoutePoint(): RoutePoint =
        RoutePoint(point.latitude, point.longitude, siteName)
}
