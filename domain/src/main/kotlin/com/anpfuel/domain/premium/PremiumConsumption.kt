package com.anpfuel.domain.premium

/**
 * PRM09-T01 pure full-to-full consumption and cost engine.
 *
 * Exact Kotlin parity with `backend/internal/modules/metrics` (PRM06-T01/T02):
 * integer math only, starting-fill exclusion, partial accumulation,
 * mixed-fuel/segment/gap exclusion and frozen UNKNOWN reasons. No stored
 * state; callers recompute from raw records after repair.
 */
data class PremiumFill(
    val quantityMilli: Long,
    val totalCentavos: Long,
    val full: Boolean,
    val fuel: PremiumFuel,
    val unit: PremiumUnit,
)

data class PremiumInterval(
    val startingFillExcluded: PremiumFill,
    val fills: List<PremiumFill>,
    val distanceMetres: Long,
    val hasGap: Boolean = false,
    val crossesSegment: Boolean = false,
    val odometerMissing: Boolean = false,
)

data class PremiumConsumptionResult(
    val eligibleQuantityMilli: Long = 0L,
    val eligibleCostCentavos: Long = 0L,
    val distanceMetres: Long = 0L,
    val kmPerUnit: String = "",
    val centavosPerKm: String = "",
    val unknown: String = "",
)

data class PremiumCostResult(
    val version: Int = PremiumBounds.CALCULATION_VERSION,
    val totalCentavos: Long = 0L,
    val distanceMetres: Long = 0L,
    val centavosPerKm: String = "",
    val method: String = "",
    val unknown: String = "",
)

data class PremiumWindow(
    val startOdometerMetres: Long,
    val endOdometerMetres: Long,
    val startMillis: Long,
    val endMillis: Long,
    val hasGap: Boolean = false,
    val crossesSegment: Boolean = false,
)

data class PremiumExpenseEntry(
    val id: String,
    val amountCentavos: Long,
    val occurredAtMillis: Long,
    val odometerMetres: Long? = null,
)

data class PremiumAggregateTotals(
    val distanceMetres: Long = 0L,
    val eligibleQuantityMilli: Long = 0L,
    val eligibleCostCentavos: Long = 0L,
    val kmPerUnit: String = "",
    val centavosPerKm: String = "",
)

object PremiumConsumption {

    fun compute(input: PremiumInterval): PremiumConsumptionResult {
        val all = listOf(input.startingFillExcluded) + input.fills
        for (f in all) {
            if (f.quantityMilli < 0L || f.quantityMilli > PremiumBounds.MAX_QUANTITY_MILLI ||
                f.totalCentavos < 0L || f.totalCentavos > PremiumBounds.MAX_TOTAL_CENTAVOS
            ) {
                return PremiumConsumptionResult(unknown = "input-out-of-bounds")
            }
        }
        if (input.distanceMetres < 0L || input.distanceMetres > PremiumBounds.MAX_DISTANCE_METRES) {
            return PremiumConsumptionResult(unknown = "input-out-of-bounds")
        }
        if (input.odometerMissing) return PremiumConsumptionResult(unknown = "odometer-missing")
        if (input.hasGap) return PremiumConsumptionResult(unknown = "declared-fill-gap")
        if (input.crossesSegment) return PremiumConsumptionResult(unknown = "odometer-segment-boundary")
        if (input.distanceMetres <= 0L) return PremiumConsumptionResult(unknown = "distance-nonpositive")

        val units = input.fills.map { it.unit }.toSet()
        if (units.size > 1) return PremiumConsumptionResult(unknown = "incompatible-unit")
        val fuels = input.fills.map { it.fuel }.toSet()
        for (fuel in fuels) {
            if (fuel == PremiumFuel.CNG) return PremiumConsumptionResult(unknown = "unsupported-gnv-method")
        }
        if (fuels.size > 1) return PremiumConsumptionResult(unknown = "unsupported-mixed-method")

        var fullAnchors = 0
        if (input.startingFillExcluded.full) fullAnchors += 1
        for (f in input.fills) if (f.full) fullAnchors += 1
        if (fullAnchors < 2) return PremiumConsumptionResult(unknown = "full-anchors-required")

        var quantity = 0L
        var cost = 0L
        for (f in input.fills) {
            quantity += f.quantityMilli
            cost += f.totalCentavos
        }
        if (quantity <= 0L) return PremiumConsumptionResult(unknown = "full-anchors-required")

        return PremiumConsumptionResult(
            eligibleQuantityMilli = quantity,
            eligibleCostCentavos = cost,
            distanceMetres = input.distanceMetres,
            kmPerUnit = PremiumBounds.rational(input.distanceMetres, quantity),
            centavosPerKm = PremiumBounds.rational(cost * 1000L, input.distanceMetres),
        )
    }

    fun fuelPurchasePerKm(eligibleCostCentavos: Long, distanceMetres: Long): PremiumCostResult {
        if (eligibleCostCentavos < 0L || eligibleCostCentavos > PremiumBounds.MAX_TOTAL_CENTAVOS ||
            distanceMetres < 0L || distanceMetres > PremiumBounds.MAX_DISTANCE_METRES
        ) {
            return PremiumCostResult(unknown = "input-out-of-bounds")
        }
        if (distanceMetres <= 0L) return PremiumCostResult(unknown = "distance-nonpositive")
        return PremiumCostResult(
            totalCentavos = eligibleCostCentavos,
            distanceMetres = distanceMetres,
            centavosPerKm = PremiumBounds.rational(eligibleCostCentavos * 1000L, distanceMetres),
            method = "fuel-purchase-interval",
        )
    }

    fun matchedWindowCost(
        fuelCostCentavos: Long,
        fuelDistanceMetres: Long,
        expenses: List<PremiumExpenseEntry>,
        window: PremiumWindow,
    ): PremiumCostResult {
        if (window.endMillis <= window.startMillis) {
            return PremiumCostResult(unknown = "unmatched-period-window")
        }
        if (window.hasGap) return PremiumCostResult(unknown = "declared-fill-gap")
        if (window.crossesSegment) return PremiumCostResult(unknown = "odometer-segment-boundary")
        val distance = window.endOdometerMetres - window.startOdometerMetres
        if (distance <= 0L) return PremiumCostResult(unknown = "distance-nonpositive")
        if (fuelDistanceMetres != distance) return PremiumCostResult(unknown = "unmatched-period-window")
        val seen = mutableSetOf<String>()
        var total = fuelCostCentavos
        for (e in expenses) {
            if (e.amountCentavos < 0L || e.amountCentavos > PremiumBounds.MAX_TOTAL_CENTAVOS) {
                return PremiumCostResult(unknown = "input-out-of-bounds")
            }
            if (!seen.add(e.id)) continue
            if (e.occurredAtMillis < window.startMillis || e.occurredAtMillis >= window.endMillis) continue
            val odo = e.odometerMetres
            if (odo != null && (odo < window.startOdometerMetres || odo > window.endOdometerMetres)) continue
            total += e.amountCentavos
        }
        return PremiumCostResult(
            totalCentavos = total,
            distanceMetres = distance,
            centavosPerKm = PremiumBounds.rational(total * 1000L, distance),
            method = "matched-window",
        )
    }

    fun sumIntervals(results: List<PremiumConsumptionResult>): PremiumAggregateTotals {
        var distance = 0L
        var quantity = 0L
        var cost = 0L
        for (r in results) {
            if (r.unknown.isNotEmpty()) continue
            distance += r.distanceMetres
            quantity += r.eligibleQuantityMilli
            cost += r.eligibleCostCentavos
        }
        return PremiumAggregateTotals(
            distanceMetres = distance,
            eligibleQuantityMilli = quantity,
            eligibleCostCentavos = cost,
            kmPerUnit = if (distance > 0L && quantity > 0L) {
                PremiumBounds.rational(distance, quantity)
            } else {
                ""
            },
            centavosPerKm = if (distance > 0L) {
                PremiumBounds.rational(cost * 1000L, distance)
            } else {
                ""
            },
        )
    }
}
