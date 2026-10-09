package com.anpfuel.domain.valueobject

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class FuelProductTest {

    @Test
    fun enumContainsEightDistinctValues() {
        assertEquals(8, FuelProduct.entries.size)
        assertEquals(
            setOf(
                FuelProduct.ETHANOL,
                FuelProduct.GASOLINE_REGULAR,
                FuelProduct.GASOLINE_PREMIUM,
                FuelProduct.DIESEL_S500,
                FuelProduct.DIESEL_S10,
                FuelProduct.CNG,
                FuelProduct.LPG_P13,
                FuelProduct.GASOLINE_PREMIUM_GRADE,
            ),
            FuelProduct.entries.toSet(),
        )
    }
}
