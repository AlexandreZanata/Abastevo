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
    val unknown: String = "",
)

data class PremiumComparisonResult(val changePercent: String = "", val unknown: String = "")

object PremiumConsumption {
    fun compare(base: PremiumConsumptionResult, current: PremiumConsumptionResult): PremiumComparisonResult {
        if (base.unknown.isNotEmpty()) return PremiumComparisonResult(unknown = "baseline-incomplete: ${base.unknown}")
        if (current.unknown.isNotEmpty()) return PremiumComparisonResult(unknown = "current-incomplete: ${current.unknown}")
        if (base.eligibleCostCentavos == 0L) return PremiumComparisonResult(unknown = "comparison-baseline-zero")
        if (base.distanceMetres <= 0L || base.eligibleQuantityMilli <= 0L ||
            current.distanceMetres <= 0L || current.eligibleQuantityMilli <= 0L ||
            base.eligibleCostCentavos < 0L || current.eligibleCostCentavos < 0L
        ) return PremiumComparisonResult(unknown = "input-out-of-bounds")
        fun n(v: Long) = java.math.BigInteger.valueOf(v)
        val numerator = (
            n(current.distanceMetres) * n(base.eligibleQuantityMilli) -
                n(base.distanceMetres) * n(current.eligibleQuantityMilli)
            ) * n(100)
        val denominator = n(base.distanceMetres) * n(current.eligibleQuantityMilli)
        val divisor = numerator.gcd(denominator)
        val a = numerator / divisor
        val b = denominator / divisor
        return PremiumComparisonResult(if (b == java.math.BigInteger.ONE) a.toString() else "$a/$b")
    }

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

        for (fill in all) {
            if (fill.unit != fill.fuel.unit) return PremiumConsumptionResult(unknown = "incompatible-unit")
        }
        val units = all.map { it.unit }.toSet()
        if (units.size > 1) return PremiumConsumptionResult(unknown = "incompatible-unit")
        val fuels = all.map { it.fuel }.toSet()
        for (fuel in fuels) {
            if (fuel == PremiumFuel.CNG) return PremiumConsumptionResult(unknown = "unsupported-gnv-method")
        }
        if (fuels.size > 1) return PremiumConsumptionResult(unknown = "unsupported-mixed-method")

        if (!input.startingFillExcluded.full || input.fills.isEmpty() || !input.fills.last().full) {
            return PremiumConsumptionResult(unknown = "full-anchors-required")
        }

        var quantity = 0L
        var cost = 0L
        for (f in input.fills) {
            quantity = PremiumBounds.addNonNegative(quantity, f.quantityMilli)
                ?: return PremiumConsumptionResult(unknown = "input-out-of-bounds")
            cost = PremiumBounds.addNonNegative(cost, f.totalCentavos)
                ?: return PremiumConsumptionResult(unknown = "input-out-of-bounds")
        }
        if (quantity <= 0L) return PremiumConsumptionResult(unknown = "full-anchors-required")

        return PremiumConsumptionResult(
            eligibleQuantityMilli = quantity,
            eligibleCostCentavos = cost,
            distanceMetres = input.distanceMetres,
            kmPerUnit = PremiumBounds.rational(input.distanceMetres, quantity),
            centavosPerKm = PremiumBounds.scaledRational(cost, 1000, input.distanceMetres),
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
            centavosPerKm = PremiumBounds.scaledRational(eligibleCostCentavos, 1000, distanceMetres),
            method = "fuel-purchase-interval",
        )
    }

    fun matchedWindowCost(
        fuelCostCentavos: Long,
        fuelDistanceMetres: Long,
        expenses: List<PremiumExpenseEntry>,
        window: PremiumWindow,
    ): PremiumCostResult {
        if (fuelCostCentavos !in 0..PremiumBounds.MAX_TOTAL_CENTAVOS ||
            fuelDistanceMetres !in 0..PremiumBounds.MAX_DISTANCE_METRES ||
            window.startOdometerMetres !in 0..PremiumBounds.MAX_DISTANCE_METRES ||
            window.endOdometerMetres !in 0..PremiumBounds.MAX_DISTANCE_METRES
        ) return PremiumCostResult(unknown = "input-out-of-bounds")
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
            total = PremiumBounds.addNonNegative(total, e.amountCentavos)
                ?: return PremiumCostResult(unknown = "input-out-of-bounds")
        }
        return PremiumCostResult(
            totalCentavos = total,
            distanceMetres = distance,
            centavosPerKm = PremiumBounds.scaledRational(total, 1000, distance),
            method = "matched-window",
        )
    }

    fun sumIntervals(results: List<PremiumConsumptionResult>): PremiumAggregateTotals {
        var distance = 0L
        var quantity = 0L
        var cost = 0L
        for (r in results) {
            if (r.unknown.isNotEmpty()) continue
            distance = PremiumBounds.addNonNegative(distance, r.distanceMetres)
                ?: return PremiumAggregateTotals(unknown = "input-out-of-bounds")
            quantity = PremiumBounds.addNonNegative(quantity, r.eligibleQuantityMilli)
                ?: return PremiumAggregateTotals(unknown = "input-out-of-bounds")
            cost = PremiumBounds.addNonNegative(cost, r.eligibleCostCentavos)
                ?: return PremiumAggregateTotals(unknown = "input-out-of-bounds")
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
                PremiumBounds.scaledRational(cost, 1000, distance)
            } else {
                ""
            },
        )
    }
}
