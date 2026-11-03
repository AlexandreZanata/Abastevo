package com.anpfuel.app.ui.charging

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.data.repository.PlaceDetail
import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingCatalogConfidence
import com.anpfuel.domain.charging.ChargingConnectionGroup
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.charging.ChargingSourceAttribution
import java.io.File
import java.util.Locale
import org.junit.Assert.assertEquals
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
    @Test fun emptyCatalogDescribesOnlyWhatOpenSourcesMapped() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        var opened = false
        compose.setContent {
            AnpFuelTheme(dynamicColor = false) {
                ChargingExploreContent(
                    uiState = ChargingExploreUiState(isLoading = false, sourceRevision = "synthetic"),
                    locale = Locale.US,
                    onConnectorChanged = {}, onCurrentChanged = {}, onAccessChanged = {},
                    onRefresh = {}, onLoadMore = {}, onOpenDetail = {}, onDismissDetail = {},
                    onDismissError = {}, onNavigateBack = {}, onAddChargingSite = { opened = true },
                )
            }
        }
        compose.onNodeWithText(context.getString(R.string.charging_open_sources)).assertIsDisplayed()
        compose.onNodeWithText(context.getString(R.string.charging_explore_empty)).assertIsDisplayed()
        compose.onNodeWithText(context.getString(R.string.charging_add_site)).performScrollTo().performClick()
        compose.runOnIdle { assertEquals(true, opened) }
    }

    @Test fun nationalDetailInLightTheme() = checkNationalDetail(darkTheme = false)

    @Test fun nationalDetailInDarkTheme() = checkNationalDetail(darkTheme = true)

    private fun checkNationalDetail(darkTheme: Boolean) {
        val id = "11111111-1111-4111-8111-111111111111"
        val key = "a".repeat(64)
        val place = ChargingPlace(
            id = id, entityKey = key, externalId = "synthetic", name = "", country = "BR",
            latitude = "-15.7939", longitude = "-47.8828", publicAccess = ChargingAccess.PUBLIC,
            reportedPoints = 0, catalogConfidence = ChargingCatalogConfidence.TWO_SOURCE,
            operator = "Synthetic operator", addressLine = "Synthetic Street, 7 – Synthetic City/SP",
            sources = listOf(
                ChargingSourceAttribution("OSM", "node:7", "osm-edition", null, "ODbL-1.0", "Synthetic OSM"),
                ChargingSourceAttribution("OCM", "7", "ocm-edition", 40, "CC-0", "Synthetic OCM"),
            ),
            groups = listOf(
                ChargingConnectionGroup(
                    "22222222-2222-4222-8222-222222222222", key, "synthetic-osm-group",
                    ChargingConnector.TYPE2, "type2", ChargingCurrent.AC, 22125, null, "osm:node:7",
                ),
                ChargingConnectionGroup(
                    "33333333-3333-4333-8333-333333333333", key, "synthetic-ocm-group",
                    ChargingConnector.TYPE2, "Type 2", ChargingCurrent.AC, 50000, null, "ocm:poi:7",
                ),
            ),
        )
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        var reportId: String? = null
        compose.setContent {
            AnpFuelTheme(darkTheme = darkTheme, dynamicColor = false) {
                ChargingExploreContent(
                    uiState = ChargingExploreUiState(
                        isLoading = false, isOffline = true,
                        detail = PlaceDetail(place, "charging-br-national-v1", "synthetic-revision", 1, 0),
                    ),
                    locale = Locale.US,
                    onConnectorChanged = {}, onCurrentChanged = {}, onAccessChanged = {},
                    onRefresh = {}, onLoadMore = {}, onOpenDetail = {}, onDismissDetail = {},
                    onDismissError = {}, onNavigateBack = {}, onOpenCommunity = { reportId = it },
                )
            }
        }
        compose.onNodeWithText(context.getString(R.string.charging_place_unnamed)).assertIsDisplayed()
        compose.onNodeWithText(context.getString(R.string.charging_place_two_sources)).assertIsDisplayed()
        compose.onNodeWithText(context.getString(R.string.charging_place_operator, "Synthetic operator")).assertIsDisplayed()
        compose.onNodeWithText("Synthetic Street, 7 – Synthetic City/SP").assertIsDisplayed()
        compose.onNodeWithText("-15.7939", substring = true).assertDoesNotExist()
        compose.onNodeWithText("type2 · AC · 22.125 kW · Synthetic OSM").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Type 2 · AC · 50 kW · Synthetic OCM").performScrollTo().assertIsDisplayed()

        if (InstrumentationRegistry.getArguments().getString("chargingScreenshots") == "true") {
            compose.waitForIdle()
            // The dialog's platform window animates outside the Compose clock.
            android.os.SystemClock.sleep(500)
            val theme = if (darkTheme) "dark" else "light"
            val screenshot = requireNotNull(InstrumentationRegistry.getInstrumentation().uiAutomation.takeScreenshot())
            File(context.cacheDir, "charging-detail-$theme.png").outputStream().use {
                check(screenshot.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it))
            }
        }
        compose.onNodeWithText(context.getString(R.string.charging_place_report_status)).performClick()
        compose.runOnIdle { assertEquals(id, reportId) }
    }

}
