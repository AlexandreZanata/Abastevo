package com.anpfuel.domain.premium

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class MEX09RegressionTest {
    private val full = PremiumFill(1, 1, true, PremiumFuel.ETHANOL, PremiumUnit.LITRE)

    @Test fun `initial and final anchors and anchor fuel are required`() {
        val partial = full.copy(full = false)
        for ((interval, reason) in listOf(
            PremiumInterval(partial, listOf(full, full), 1000) to "full-anchors-required",
            PremiumInterval(full, listOf(full, partial), 1000) to "full-anchors-required",
            PremiumInterval(full.copy(fuel = PremiumFuel.GASOLINE_REGULAR), listOf(full), 1000) to "unsupported-mixed-method",
            PremiumInterval(full.copy(unit = PremiumUnit.CUBIC_METRE), listOf(full.copy(unit = PremiumUnit.CUBIC_METRE)), 1000) to "incompatible-unit",
        )) assertEquals(reason, PremiumConsumption.compute(interval).unknown)
    }

    @Test fun `large valid cost does not wrap while scaling`() {
        val fills = List(10000) { full.copy(totalCentavos = PremiumBounds.MAX_TOTAL_CENTAVOS) }
        assertEquals("10000000000000000000", PremiumConsumption.compute(PremiumInterval(full, fills, 1)).centavosPerKm)
    }

    @Test fun `matched window rejects invalid base money`() {
        val result = PremiumConsumption.matchedWindowCost(-1, 1, emptyList(), PremiumWindow(1, 2, 1000, 2000))
        assertEquals("input-out-of-bounds", result.unknown)
    }
}
