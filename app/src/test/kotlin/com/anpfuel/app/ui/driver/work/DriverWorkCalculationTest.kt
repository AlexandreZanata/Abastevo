package com.anpfuel.app.ui.driver.work

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class DriverWorkCalculationTest {
    @Test fun `recorded result excludes personal cost share and needs distance for per-km`() {
        val cost = DriverWorkViewModel.recordedCost(100_000, 20)
        assertEquals(80_000L, cost)
        assertEquals(20_000L, DriverWorkViewModel.recordedResult(100_000, cost))
        assertEquals("40", DriverWorkViewModel.perKm(20_000, 500_000))
        assertNull(DriverWorkViewModel.perKm(20_000, 0))
        assertNull(DriverWorkViewModel.perKm(20_000, null))
    }
}
