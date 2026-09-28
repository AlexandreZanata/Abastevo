package com.anpfuel.app.navigation

import com.anpfuel.domain.valueobject.FuelProduct
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class RoutesTest {

    @Test
    fun vehiclesRouteIsRegisteredConstant() {
        assertEquals("vehicles", Routes.VEHICLES)
    }

    @Test
    fun stationsRouteIncludesFuelProductName() {
        assertEquals(
            "stations/GASOLINE_REGULAR",
            Routes.stations(FuelProduct.GASOLINE_REGULAR),
        )
    }
}
