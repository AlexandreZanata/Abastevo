package com.anpfuel.app.ui.driver

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class DriverHubScreenTest {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()
    private val vehicle = Vehicle.create(
        displayName = "Gol", tankCapacity = TankCapacity.of(50.0),
        fuelProduct = FuelProduct.GASOLINE_REGULAR, priceSource = VehiclePriceSource.cheapest(),
    )

    private fun render(state: DriverHubUiState, onVehicles: () -> Unit = {}, onPremium: () -> Unit = {}, onSignIn: () -> Unit = {}, onRetry: () -> Unit = {}, onEntries: (String) -> Unit = {}, onMonthly: (String) -> Unit = {}, onRefuel: (String) -> Unit = {}, onCharge: (String) -> Unit = {}) {
        compose.setContent {
            AnpFuelTheme(dynamicColor = false) {
                DriverHubContent(
                    state = state, onNavigateBack = {}, onOpenVehicles = onVehicles,
                    onOpenEntries = onEntries, onOpenMonthly = onMonthly,
                    onOpenPremium = onPremium, onSignIn = onSignIn, onRetry = onRetry, onOpenRefuel = onRefuel, onOpenCharge = onCharge,
                )
            }
        }
    }

    private fun text(id: Int): String = compose.activity.getString(id)

    @Test fun firstUseOpensRealVehicleForm() {
        var opened = false
        render(DriverHubUiState(loading = false, access = DriverHubAccess.LAPSED), onVehicles = { opened = true })
        compose.onNodeWithText(text(com.anpfuel.app.R.string.driver_hub_first_vehicle_title)).assertExists()
        compose.onNodeWithText(text(com.anpfuel.app.R.string.driver_hub_add_vehicle)).performClick()
        assertTrue(opened)
    }

    @Test fun verifiedVehicleActionsAreConcrete() {
        var entries = ""; var monthly = ""; var vehicles = false; var refuel = ""; var charge = ""
        render(
            DriverHubUiState(loading = false, access = DriverHubAccess.VERIFIED, vehicles = listOf(vehicle)),
            onVehicles = { vehicles = true }, onEntries = { entries = it }, onMonthly = { monthly = it },
            onRefuel = { refuel = it }, onCharge = { charge = it },
        )
        compose.onNodeWithText(text(com.anpfuel.app.R.string.driver_hub_open_entries)).performClick()
        compose.onNodeWithText(text(com.anpfuel.app.R.string.driver_hub_open_export)).performClick()
        compose.onNodeWithText(text(com.anpfuel.app.R.string.driver_hub_open_vehicles)).performClick()
        compose.onNodeWithText(text(com.anpfuel.app.R.string.driver_hub_quick_refuel)).performClick()
        compose.onNodeWithText(text(com.anpfuel.app.R.string.driver_hub_quick_charge)).performClick()
        assertEquals(vehicle.id.value, entries)
        assertEquals(vehicle.id.value, monthly)
        assertEquals(vehicle.id.value, refuel)
        assertEquals(vehicle.id.value, charge)
        assertTrue(vehicles)
    }

    @Test fun lapseOffersPremiumAndRetainedNote() {
        var premium = false
        render(DriverHubUiState(loading = false, access = DriverHubAccess.LAPSED), onPremium = { premium = true })
        compose.onNodeWithText(text(com.anpfuel.app.R.string.driver_hub_open_premium)).performClick()
        assertTrue(premium)
        compose.onNodeWithText(text(com.anpfuel.app.R.string.driver_hub_retained_note)).assertExists()
    }

    @Test fun offlineOffersRetry() {
        var retry = false
        render(DriverHubUiState(loading = false, access = DriverHubAccess.OFFLINE), onRetry = { retry = true })
        compose.onNodeWithText(text(com.anpfuel.app.R.string.driver_hub_retry)).performClick()
        assertTrue(retry)
        compose.onNodeWithText(text(com.anpfuel.app.R.string.driver_hub_retained_note)).assertExists()
    }

    @Test fun signedOutOffersSignIn() {
        var signIn = false
        render(DriverHubUiState(loading = false, access = DriverHubAccess.SIGNED_OUT), onSignIn = { signIn = true })
        compose.onNodeWithText(text(com.anpfuel.app.R.string.driver_hub_sign_in)).performClick()
        assertTrue(signIn)
    }

}
