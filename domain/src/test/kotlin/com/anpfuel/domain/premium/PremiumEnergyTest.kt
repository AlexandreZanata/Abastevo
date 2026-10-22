package com.anpfuel.domain.premium

import java.io.File
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

/**
 * EE-23 Go/Kotlin golden parity: executes
 * `contracts/testdata/premium/energy-execution-v1.csv` (generated from
 * the frozen `energy-v1.json`) against the pure engine. Every vector
 * must match the Go results exactly: same sums, same rationals, same
 * unknown vocabulary.
 */
class PremiumEnergyTest {

    private fun rows(): List<Map<String, String>> {
        val lines = File("../contracts/testdata/premium/energy-execution-v1.csv").readLines()
        val header = lines.first().split(',')
        return lines.drop(1).filter { it.isNotBlank() }.map { line ->
            header.zip(line.split(',')).toMap()
        }
    }

    @Test
    fun `corpus vectors match Go exactly`() {
        val cases = rows()
        assertEquals(11, cases.size)
        for (c in cases) {
            val id = c.getValue("id")
            if (c.getValue("expected_combined_centavos_per_km").isNotEmpty() || c.getValue("expected_combined_unknown").isNotEmpty()) {
                val got = PremiumEnergy.combinedPurchasePerKm(
                    c.getValue("fuel_cost_centavos").toLong(),
                    c.getValue("charge_cost_centavos").toLong(),
                    c.getValue("distance_metres").toLong(),
                )
                assertEquals(c.getValue("expected_combined_centavos_per_km"), got.centavosPerKm, id)
                assertEquals(c.getValue("expected_combined_unknown"), got.unknown, id)
                // Parity with Go: the method is set only on exact results.
                assertEquals(if (got.unknown.isEmpty()) "combined-purchase-interval" else "", got.method, id)
                continue
            }
            val got = PremiumEnergy.compute(
                PremiumEnergyInterval(
                    purchasedWh = c.getValue("purchased_wh").toLong(),
                    costCentavos = c.getValue("cost_centavos").toLong(),
                    distanceMetres = c.getValue("distance_metres").toLong(),
                    hasEnergy = c.getValue("has_energy").toBoolean(),
                    complete = c.getValue("complete").toBoolean(),
                    hasGap = c.getValue("has_gap").toBoolean(),
                    crossesSegment = c.getValue("crosses_segment").toBoolean(),
                    odometerMissing = c.getValue("odometer_missing").toBoolean(),
                    socInconsistent = c.getValue("soc_inconsistent").toBoolean(),
                    mixedCarrier = c.getValue("mixed_carrier").toBoolean(),
                ),
            )
            assertEquals(c.getValue("purchased_wh").toLong(), got.purchasedWh, id)
            assertEquals(c.getValue("cost_centavos").toLong(), got.costCentavos, id)
            assertEquals(c.getValue("expected_wh_per_km"), got.whPerKm, id)
            assertEquals(c.getValue("expected_centavos_per_km"), got.centavosPerKm, id)
            assertEquals(c.getValue("expected_unknown"), got.unknown, id)
        }
    }

    @Test
    fun `weighted comparison never averages ratios`() {
        // Halving the energy over the same distance doubles m/Wh: +100%.
        // (Doubling it would halve efficiency: -50%.)
        val base = PremiumEnergyResult(purchasedWh = 40000, costCentavos = 4000, distanceMetres = 150000)
        val current = PremiumEnergyResult(purchasedWh = 20000, costCentavos = 2000, distanceMetres = 150000)
        assertEquals("100", PremiumEnergy.compare(base, current).changePercent)
        assertEquals("", PremiumEnergy.compare(base, current).unknown)
        val worse = PremiumEnergyResult(purchasedWh = 80000, costCentavos = 8000, distanceMetres = 150000)
        assertEquals("-50", PremiumEnergy.compare(base, worse).changePercent)
        // Unknown baselines refuse instead of zero-filling.
        val unknown = PremiumEnergyResult(unknown = "energy-unmeasured")
        assertEquals("baseline-incomplete: energy-unmeasured", PremiumEnergy.compare(unknown, current).unknown)
        assertEquals("current-incomplete: energy-unmeasured", PremiumEnergy.compare(current, unknown).unknown)
    }

    @Test
    fun `equivalent km per litre is refused`() {
        assertThrows(PremiumValidationException::class.java) {
            PremiumEnergy.equivalentKmPerL(15000)
        }
    }
}
