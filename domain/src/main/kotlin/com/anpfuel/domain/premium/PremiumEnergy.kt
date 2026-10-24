package com.anpfuel.domain.premium

import java.math.BigInteger

/**
 * EE-23 exact electric metrics, Kotlin parity with
 * `backend/internal/modules/metrics/energy.go` (same precedence, same
 * sum-first rationals, same unknown vocabulary). Both sides execute
 * `contracts/testdata/premium/energy-v1.json`.
 */
data class PremiumEnergyInterval(
    val purchasedWh: Long,
    val costCentavos: Long,
    val distanceMetres: Long,
    val hasEnergy: Boolean,
    val complete: Boolean,
    val hasGap: Boolean = false,
    val crossesSegment: Boolean = false,
    val odometerMissing: Boolean = false,
    val socInconsistent: Boolean = false,
    val mixedCarrier: Boolean = false,
)

data class PremiumEnergyResult(
    val purchasedWh: Long = 0L,
    val costCentavos: Long = 0L,
    val distanceMetres: Long = 0L,
    val whPerKm: String = "",
    val centavosPerKm: String = "",
    val unknown: String = "",
)

data class PremiumEnergyComparison(val changePercent: String = "", val unknown: String = "")

object PremiumEnergy {

    const val MAX_ENERGY_WH = 1_000_000_000_000L

    fun compute(input: PremiumEnergyInterval): PremiumEnergyResult {
        val out = PremiumEnergyResult(
            purchasedWh = input.purchasedWh,
            costCentavos = input.costCentavos,
            distanceMetres = input.distanceMetres,
        )
        if (input.purchasedWh < 0 || input.purchasedWh > MAX_ENERGY_WH ||
            input.costCentavos < 0 || input.costCentavos > PremiumBounds.MAX_TOTAL_CENTAVOS
        ) {
            return PremiumEnergyResult(unknown = "input-out-of-bounds")
        }
        if (input.distanceMetres < 0 || input.distanceMetres > PremiumBounds.MAX_DISTANCE_METRES) {
            return PremiumEnergyResult(unknown = "input-out-of-bounds")
        }
        val unknown = when {
            input.odometerMissing -> "odometer-missing"
            input.hasGap -> "declared-fill-gap"
            input.crossesSegment -> "odometer-segment-boundary"
            input.socInconsistent -> "soc-inconsistent"
            input.mixedCarrier -> "phev-shared-distance"
            input.distanceMetres <= 0L -> "distance-nonpositive"
            !input.hasEnergy -> "energy-unmeasured"
            !input.complete -> "partial-measurement"
            else -> null
        }
        if (unknown != null) return out.copy(unknown = unknown)
        return out.copy(
            whPerKm = PremiumBounds.scaledRational(input.purchasedWh, 1000, input.distanceMetres),
            centavosPerKm = PremiumBounds.scaledRational(input.costCentavos, 1000, input.distanceMetres),
        )
    }

    fun compare(base: PremiumEnergyResult, current: PremiumEnergyResult): PremiumEnergyComparison {
        if (base.unknown.isNotEmpty()) return PremiumEnergyComparison(unknown = "baseline-incomplete: " + base.unknown)
        if (current.unknown.isNotEmpty()) return PremiumEnergyComparison(unknown = "current-incomplete: " + current.unknown)
        if (base.purchasedWh <= 0L || current.purchasedWh <= 0L ||
            base.distanceMetres <= 0L || current.distanceMetres <= 0L
        ) {
            return PremiumEnergyComparison(unknown = "input-out-of-bounds")
        }
        fun n(v: Long) = BigInteger.valueOf(v)
        val numerator = (n(current.distanceMetres) * n(base.purchasedWh) - n(base.distanceMetres) * n(current.purchasedWh)) * n(100)
        val denominator = n(base.distanceMetres) * n(current.purchasedWh)
        val divisor = numerator.gcd(denominator)
        val a = numerator / divisor
        val b = denominator / divisor
        return PremiumEnergyComparison(if (b == BigInteger.ONE) a.toString() else "$a/$b")
    }

    fun combinedPurchasePerKm(fuelCostCentavos: Long, chargeCostCentavos: Long, distanceMetres: Long): PremiumCostResult {
        if (fuelCostCentavos < 0 || fuelCostCentavos > PremiumBounds.MAX_TOTAL_CENTAVOS ||
            chargeCostCentavos < 0 || chargeCostCentavos > PremiumBounds.MAX_TOTAL_CENTAVOS
        ) {
            return PremiumCostResult(version = PremiumBounds.CALCULATION_VERSION, unknown = "input-out-of-bounds")
        }
        val total = PremiumBounds.addNonNegative(fuelCostCentavos, chargeCostCentavos)
            ?: return PremiumCostResult(version = PremiumBounds.CALCULATION_VERSION, unknown = "input-out-of-bounds")
        if (distanceMetres < 0 || distanceMetres > PremiumBounds.MAX_DISTANCE_METRES) {
            return PremiumCostResult(version = PremiumBounds.CALCULATION_VERSION, unknown = "input-out-of-bounds")
        }
        if (distanceMetres <= 0L) {
            return PremiumCostResult(version = PremiumBounds.CALCULATION_VERSION, unknown = "distance-nonpositive")
        }
        return PremiumCostResult(
            version = PremiumBounds.CALCULATION_VERSION,
            totalCentavos = total,
            distanceMetres = distanceMetres,
            centavosPerKm = PremiumBounds.scaledRational(total, 1000, distanceMetres),
            method = "combined-purchase-interval",
        )
    }

    /**
     * Equivalent km/L from electric energy is refused: no kWh-to-litre
     * conversion exists, and inventing one would rewrite history.
     */
    fun equivalentKmPerL(@Suppress("UNUSED_PARAMETER") energyWh: Long): Nothing =
        throw PremiumValidationException("unsupported-unit-conversion")
}
