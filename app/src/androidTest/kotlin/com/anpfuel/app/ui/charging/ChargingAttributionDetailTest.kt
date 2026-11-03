package com.anpfuel.app.ui.charging

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.data.repository.PlaceDetail
import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.charging.ChargingSourceAttribution
import java.util.Locale
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ChargingAttributionDetailTest {
    @get:Rule val compose = createComposeRule()

    @Test fun offlineDetailShowsBothProviderNotices() {
        val place = ChargingPlace(
            id = "11111111-1111-4111-8111-111111111111", entityKey = "a".repeat(64),
            externalId = "7", name = "Synthetic Charge", country = "BR",
            latitude = null, longitude = null, publicAccess = ChargingAccess.PUBLIC,
            reportedPoints = 1, groups = emptyList(),
            sources = listOf(
                ChargingSourceAttribution("OSM", "node:7", "osm-edition", null, "ODbL-1.0", "© OpenStreetMap contributors (ODbL)"),
                ChargingSourceAttribution("OCM", "7", "ocm-edition", 40, "CC-0", "Easy Volt"),
            ),
        )
        compose.setContent {
            AnpFuelTheme(dynamicColor = false) {
                ChargingExploreContent(
                    uiState = ChargingExploreUiState(
                        isLoading = false, isOffline = true,
                        detail = PlaceDetail(place, "synthetic", "synthetic-revision", 1, 0),
                    ),
                    locale = Locale.ROOT,
                    onConnectorChanged = {}, onCurrentChanged = {}, onAccessChanged = {},
                    onRefresh = {}, onLoadMore = {}, onOpenDetail = {}, onDismissDetail = {},
                    onDismissError = {}, onNavigateBack = {},
                )
            }
        }
        compose.onNodeWithText("© OpenStreetMap contributors (ODbL) · ODbL-1.0").assertIsDisplayed()
        compose.onNodeWithText("Easy Volt · CC-0").assertIsDisplayed()
    }
}
