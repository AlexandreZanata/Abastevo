package com.anpfuel.domain.premium

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/**
 * PRM09-T01 Go/Kotlin parity: frozen `contracts/testdata/premium/financial-v1.json`
 * vectors must compute identical eligible sums and rational strings.
 * The executable shared corpus, including month/comparison cases, now runs in
 * FinancialExecutionVectorsTest; these focused examples retain local diagnostics.
 */
class PremiumConsumptionTest {

    private fun full(quantity: Long, cost: Long) =
        PremiumFill(quantity, cost, true, PremiumFuel.ETHANOL, PremiumUnit.LITRE)

    private fun partial(quantity: Long, cost: Long) =
        PremiumFill(quantity, cost, false, PremiumFuel.ETHANOL, PremiumUnit.LITRE)

    @Test
    fun `full-partial-full accumulates eligible fills excluding start`() {
        val result = PremiumConsumption.compute(
            PremiumInterval(
                startingFillExcluded = full(50000, 30000),
                fills = listOf(partial(20000, 12000), full(20000, 12400)),
                distanceMetres = 400000,
            ),
        )
        assertEquals(40000L, result.eligibleQuantityMilli)
        assertEquals(24400L, result.eligibleCostCentavos)
        assertEquals("10", result.kmPerUnit)
        assertEquals("61", result.centavosPerKm)
        assertEquals("", result.unknown)
    }

    @Test
    fun `discount keeps actual total not stated unit price`() {
        val result = PremiumConsumption.compute(
            PremiumInterval(
                startingFillExcluded = full(50000, 30000),
                fills = listOf(partial(20000, 11500), full(20000, 11500)),
                distanceMetres = 400000,
            ),
        )
        assertEquals(40000L, result.eligibleQuantityMilli)
        assertEquals(23000L, result.eligibleCostCentavos)
        assertEquals("10", result.kmPerUnit)
        assertEquals("115/2", result.centavosPerKm)
    }

    @Test
    fun `weighted intervals reduce rationals`() {
        val result = PremiumConsumption.compute(
            PremiumInterval(
                startingFillExcluded = full(1, 1),
                fills = listOf(full(10000, 6000), full(60000, 36000)),
                distanceMetres = 1000000,
            ),
        )
        assertEquals(70000L, result.eligibleQuantityMilli)
        assertEquals(42000L, result.eligibleCostCentavos)
        assertEquals("100/7", result.kmPerUnit)
        assertEquals("42", result.centavosPerKm)
    }

    @Test
    fun `noninteger distance reduces price rational`() {
        val result = PremiumConsumption.compute(
            PremiumInterval(
                startingFillExcluded = full(1, 1),
                fills = listOf(full(12345, 7406)),
                distanceMetres = 123450,
            ),
        )
        assertEquals("10", result.kmPerUnit)
        assertEquals("148120/2469", result.centavosPerKm)
    }

    @Test
    fun `unknown reasons match frozen vectors`() {
        fun unknown(interval: PremiumInterval): String =
            PremiumConsumption.compute(interval).unknown

        assertEquals(
            "full-anchors-required",
            unknown(
                PremiumInterval(
                    startingFillExcluded = partial(50000, 30000),
                    fills = listOf(partial(20000, 12000)),
                    distanceMetres = 400000,
                ),
            ),
        )
        assertEquals(
            "odometer-missing",
            unknown(
                PremiumInterval(
                    startingFillExcluded = full(50000, 30000),
                    fills = listOf(full(20000, 12000)),
                    distanceMetres = 400000,
                    odometerMissing = true,
                ),
            ),
        )
        assertEquals(
            "declared-fill-gap",
            unknown(
                PremiumInterval(
                    startingFillExcluded = full(50000, 30000),
                    fills = listOf(full(20000, 12000)),
                    distanceMetres = 400000,
                    hasGap = true,
                ),
            ),
        )
        assertEquals(
            "odometer-segment-boundary",
            unknown(
                PremiumInterval(
                    startingFillExcluded = full(50000, 30000),
                    fills = listOf(full(20000, 12000)),
                    distanceMetres = 400000,
                    crossesSegment = true,
                ),
            ),
        )
        assertEquals(
            "distance-nonpositive",
            unknown(
                PremiumInterval(
                    startingFillExcluded = full(50000, 30000),
                    fills = listOf(full(20000, 12000)),
                    distanceMetres = 0,
                ),
            ),
        )
        assertEquals(
            "unsupported-mixed-method",
            unknown(
                PremiumInterval(
                    startingFillExcluded = full(50000, 30000),
                    fills = listOf(
                        full(20000, 12000),
                        PremiumFill(20000, 13000, true, PremiumFuel.GASOLINE_REGULAR, PremiumUnit.LITRE),
                    ),
                    distanceMetres = 400000,
                ),
            ),
        )
        assertEquals(
            "unsupported-gnv-method",
            unknown(
                PremiumInterval(
                    startingFillExcluded = PremiumFill(50000, 30000, true, PremiumFuel.CNG, PremiumUnit.CUBIC_METRE),
                    fills = listOf(PremiumFill(20000, 12000, true, PremiumFuel.CNG, PremiumUnit.CUBIC_METRE)),
                    distanceMetres = 400000,
                ),
            ),
        )
        assertEquals(
            "incompatible-unit",
            unknown(
                PremiumInterval(
                    startingFillExcluded = full(50000, 30000),
                    fills = listOf(
                        full(20000, 12000),
                        PremiumFill(20000, 12000, true, PremiumFuel.ETHANOL, PremiumUnit.CUBIC_METRE),
                    ),
                    distanceMetres = 400000,
                ),
            ),
        )
        assertEquals(
            "input-out-of-bounds",
            unknown(
                PremiumInterval(
                    startingFillExcluded = full(50000, 30000),
                    fills = listOf(PremiumFill(1_000_000_001L, 12000, true, PremiumFuel.ETHANOL, PremiumUnit.LITRE)),
                    distanceMetres = 400000,
                ),
            ),
        )
    }

    @Test
    fun `cost aggregates expose numerator denominator window and method`() {
        val fuel = PremiumConsumption.fuelPurchasePerKm(24400, 400000)
        assertEquals("61", fuel.centavosPerKm)
        assertEquals("fuel-purchase-interval", fuel.method)
        assertEquals("", fuel.unknown)

        val zero = PremiumConsumption.fuelPurchasePerKm(100, 0)
        assertEquals("distance-nonpositive", zero.unknown)

        val window = PremiumWindow(
            startOdometerMetres = 100000,
            endOdometerMetres = 500000,
            startMillis = 1000,
            endMillis = 2000,
        )
        val matched = PremiumConsumption.matchedWindowCost(
            23000,
            400000,
            listOf(
                PremiumExpenseEntry("e1", 5000, 1500, 200000),
                PremiumExpenseEntry("e1", 5000, 1500, 200000),
                PremiumExpenseEntry("late", 9000, 3000, 200000),
            ),
            window,
        )
        assertEquals(28000L, matched.totalCentavos)
        assertEquals("matched-window", matched.method)

        val totals = PremiumConsumption.sumIntervals(
            listOf(
                PremiumConsumption.compute(
                    PremiumInterval(full(50000, 30000), listOf(partial(20000, 12000), full(20000, 12400)), 400000),
                ),
                PremiumConsumption.compute(
                    PremiumInterval(full(1, 1), listOf(full(10000, 6000), full(60000, 36000)), 1000000),
                ),
            ),
        )
        assertEquals(1400000L, totals.distanceMetres)
        assertEquals(110000L, totals.eligibleQuantityMilli)
    }
}
