package com.anpfuel.app.ui.stations

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollToNode
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.data.repository.ChargingDiscoveryRepository
import com.anpfuel.data.repository.PlaceDetail
import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingCatalogConfidence
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.charging.ChargingSourceAttribution
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class StationLinkedChargersTest {
    @get:Rule val compose = createComposeRule()

    @Test fun linkedChargerOpensOfflineDetailsAndReportsItsOwnId() {
        val place = ChargingPlace(
            id = "22222222-2222-4222-8222-222222222222", entityKey = "a".repeat(64),
            externalId = "synthetic", name = "Synthetic Charging Site", country = "BR",
            latitude = null, longitude = null, publicAccess = ChargingAccess.PUBLIC,
            reportedPoints = 0, groups = emptyList(), catalogConfidence = ChargingCatalogConfidence.SINGLE_SOURCE,
            linkedStationId = "11111111-1111-4111-8111-111111111111",
            operator = "Synthetic operator",
            sources = listOf(ChargingSourceAttribution("OSM", "node:7", "synthetic", null, "ODbL-1.0", "Synthetic OSM")),
        )
        var reportId: String? = null
        compose.setContent {
            AnpFuelTheme(dynamicColor = false) {
                StationPageContent(
                    state = StationPageState(
                        name = "Synthetic Fuel Station", loading = false,
                        linkedChargers = listOf(PlaceDetail(place, ChargingDiscoveryRepository.NATIONAL_PROFILE, "synthetic", 1, 1)),
                    ),
                    onBack = {}, onRetry = {}, onFuel = {}, onSignIn = {}, onUpdatePrice = { _, _ -> },
                    onRoute = {}, onReportCharging = { reportId = it },
                )
            }
        }
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val section = context.getString(R.string.station_linked_chargers)
        compose.onNode(hasScrollAction()).performScrollToNode(hasText(section))
        compose.onNodeWithText(section).assertIsDisplayed()
        compose.onNode(hasScrollAction()).performScrollToNode(hasText(place.name))
        compose.onNodeWithText(place.name).performClick()
        compose.onNodeWithText(context.getString(R.string.charging_place_operator, "Synthetic operator")).assertIsDisplayed()
        compose.onNodeWithText(context.getString(R.string.charging_place_report_status)).performClick()
        compose.runOnIdle { assertEquals(place.id, reportId) }
    }
}
