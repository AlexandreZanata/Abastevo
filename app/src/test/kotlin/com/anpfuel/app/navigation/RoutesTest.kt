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
    fun authRouteIsRegisteredConstant() {
        assertEquals("auth", Routes.AUTH)
    }

    @Test
    fun exploreRouteMatchesHome() {
        assertEquals("home", Routes.EXPLORE)
        assertEquals(Routes.HOME, Routes.EXPLORE)
    }

    @Test
    fun communityRouteIsRegisteredConstant() {
        assertEquals("community", Routes.COMMUNITY)
    }

    @Test
    fun helpRouteIsRegisteredConstant() {
        assertEquals("help", Routes.HELP)
    }

    @Test
    fun profileRouteIsRegisteredConstant() {
        assertEquals("profile", Routes.PROFILE)
    }

    @Test
    fun stationsRouteIncludesFuelProductName() {
        assertEquals(
            "stations/GASOLINE_REGULAR",
            Routes.stations(FuelProduct.GASOLINE_REGULAR),
        )
    }
}
