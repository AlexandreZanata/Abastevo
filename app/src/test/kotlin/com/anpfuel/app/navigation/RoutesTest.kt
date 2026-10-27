package com.anpfuel.app.navigation

import com.anpfuel.domain.valueobject.FuelProduct
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
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
    fun privacyRouteIsRegisteredConstant() {
        assertEquals("privacy", Routes.PRIVACY)
    }

    @Test
    fun profileRouteIsRegisteredConstant() {
        assertEquals("profile", Routes.PROFILE)
    }

    @Test
    fun driverHubRouteIsRegisteredConstant() {
        assertEquals("driver-hub", Routes.DRIVER_HUB)
        assertFalse(Routes.DRIVER_HUB.contains("{"))
        assertFalse(Routes.DRIVER_HUB.contains("?"))
    }

    @Test
    fun contributionsRouteIsRegisteredConstant() {
        assertEquals("contributions", Routes.CONTRIBUTIONS)
    }

    @Test
    fun premiumRoutesCarryNoAccountOrPurchaseArguments() {
        assertEquals("premium", Routes.PREMIUM)
        assertEquals("premium-management", Routes.PREMIUM_MANAGEMENT)
        assertFalse(Routes.PREMIUM.contains("{"))
        assertFalse(Routes.PREMIUM.contains("?"))
        assertFalse(Routes.PREMIUM_MANAGEMENT.contains("{"))
        assertFalse(Routes.PREMIUM_MANAGEMENT.contains("?"))
    }

    @Test
    fun stationsRouteIncludesFuelProductName() {
        assertEquals(
            "stations/GASOLINE_REGULAR",
            Routes.stations(FuelProduct.GASOLINE_REGULAR),
        )
    }
    @Test
    fun quickEntryRouteCarriesOnlyVehicleAndAction() {
        val id = "22222222-2222-4222-8222-222222222222"
        assertEquals("vehicle-entries/$id?action=refuel", Routes.vehicleEntriesAction(id, "refuel"))
        assertEquals("vehicle-entries/$id?action=charge", Routes.vehicleEntriesAction(id, "charge"))
        org.junit.jupiter.api.Assertions.assertThrows(IllegalArgumentException::class.java) {
            Routes.vehicleEntriesAction(id, "../auth")
        }
    }

    @Test
    fun stationPageBindsIdentityAndFuelWithoutAccountSecrets() {
        assertEquals("station/04218406000104?fuelProduct=ETHANOL", Routes.stationPage("04218406000104", FuelProduct.ETHANOL))
        org.junit.jupiter.api.Assertions.assertThrows(IllegalArgumentException::class.java) {
            Routes.stationPage("../auth", FuelProduct.ETHANOL)
        }
    }
}
