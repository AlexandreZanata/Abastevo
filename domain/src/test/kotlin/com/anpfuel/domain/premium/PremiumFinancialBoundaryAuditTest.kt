package com.anpfuel.domain.premium

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class PremiumFinancialBoundaryAuditTest {
    private val window = PremiumWindow(100, 200, 1000, 2000)

    @Test
    fun invalidOrUnmatchedFinancialWindowsDoNotProduceAValue() {
        for ((w, reason) in listOf(
            window.copy(endMillis = 1000) to "unmatched-period-window",
            window.copy(hasGap = true) to "declared-fill-gap",
            window.copy(crossesSegment = true) to "odometer-segment-boundary",
            window.copy(endOdometerMetres = 100) to "distance-nonpositive",
            window.copy(startOdometerMetres = -1) to "input-out-of-bounds",
            window.copy(endOdometerMetres = PremiumBounds.MAX_DISTANCE_METRES + 1) to "input-out-of-bounds",
        )) assertEquals(reason, PremiumConsumption.matchedWindowCost(100, 100, emptyList(), w).unknown)
        assertEquals("input-out-of-bounds", PremiumConsumption.matchedWindowCost(0, PremiumBounds.MAX_DISTANCE_METRES + 1, emptyList(), window).unknown)
        assertEquals("input-out-of-bounds", PremiumConsumption.matchedWindowCost(PremiumBounds.MAX_TOTAL_CENTAVOS + 1, 100, emptyList(), window).unknown)
        for (amount in listOf(-1L, PremiumBounds.MAX_TOTAL_CENTAVOS + 1)) {
            assertEquals("input-out-of-bounds", PremiumConsumption.matchedWindowCost(100, 100, listOf(PremiumExpenseEntry("bad", amount, 1000)), window).unknown)
        }
        val expenses = listOf(
            PremiumExpenseEntry("before", 999, 999), PremiumExpenseEntry("end", 999, 2000),
            PremiumExpenseEntry("odo-before", 999, 1500, 99), PremiumExpenseEntry("odo-after", 999, 1500, 201),
            PremiumExpenseEntry("included", 25, 1000, 100),
        )
        val result = PremiumConsumption.matchedWindowCost(100, 100, expenses, window)
        assertEquals(125L, result.totalCentavos)
        assertEquals("1250", result.centavosPerKm)
    }

    @Test
    fun rawFinancialBoundsAndEmptyAnchorsRefuse() {
        val fill = PremiumFill(1, 1, true, PremiumFuel.ETHANOL, PremiumUnit.LITRE)
        for (bad in listOf(fill.copy(quantityMilli = -1), fill.copy(quantityMilli = PremiumBounds.MAX_QUANTITY_MILLI + 1), fill.copy(totalCentavos = -1))) {
            assertEquals("input-out-of-bounds", PremiumConsumption.compute(PremiumInterval(fill, listOf(bad), 100)).unknown)
        }
        for (distance in listOf(-1L, PremiumBounds.MAX_DISTANCE_METRES + 1)) {
            assertEquals("input-out-of-bounds", PremiumConsumption.compute(PremiumInterval(fill, listOf(fill), distance)).unknown)
        }
        assertEquals("full-anchors-required", PremiumConsumption.compute(PremiumInterval(fill, emptyList(), 100)).unknown)
        assertEquals("full-anchors-required", PremiumConsumption.compute(PremiumInterval(fill, listOf(fill.copy(quantityMilli = 0)), 100)).unknown)
        assertEquals("input-out-of-bounds", PremiumConsumption.fuelPurchasePerKm(-1, 1).unknown)
        assertEquals("input-out-of-bounds", PremiumConsumption.fuelPurchasePerKm(1, -1).unknown)
    }

    @Test
    fun malformedPendingIntentsCannotEnterTheOwnerOutbox() {
        val scope = PremiumOwnerScope("account", "origin")
        for ((operation, dataset, record) in listOf(Triple("", "refuel", "r"), Triple("op", "", "r"), Triple("op", "refuel", ""))) {
            assertThrows<PremiumValidationException> { PremiumCommand(operation, scope, dataset, record, 0) }
        }
        assertThrows<PremiumValidationException> { PremiumConflict("", "refuel", "r", 0, 1, false, false) }
        assertThrows<PremiumValidationException> { PremiumConflict("op", "refuel", "r", -1, 1, false, false) }
        assertThrows<PremiumValidationException> { PremiumConflict("op", "refuel", "r", 0, -1, false, false) }
    }
}
