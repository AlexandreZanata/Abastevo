package com.anpfuel.app.ui.community

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.graphics.asAndroidBitmap
import android.graphics.Bitmap
import java.io.File
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.Density
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.navigation.Routes
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.community.*
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class CommunityFeedScreenTest {
    @get:Rule val compose = createComposeRule()
    private val context = InstrumentationRegistry.getInstrumentation().targetContext
    private val city = FeedCity("5107925", BrazilianState.MATO_GROSSO, "Sorriso")
    private val item = CommunityFeedItem("00000000-0000-0000-0000-000000000001", "Synthetic station", FuelProduct.ETHANOL, 3981,
        Instant.parse("2026-10-06T12:00:00Z"), Instant.parse("2026-10-07T12:00:00Z"), 1, 0, "LOW", 1)
    private fun render(state: CommunityFeedUiState, dark: Boolean = false, font: Float = 1f,
        navigate: (String) -> Unit = {}, sort: (FeedSort) -> Unit = {}, fuel: (FuelProduct) -> Unit = {}) {
        compose.setContent {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, font)) {
                AnpFuelTheme(darkTheme = dark) {
                    CommunityFeedContent(state, dark, {}, navigate, fuel, sort, {}, {})
                }
            }
        }
    }
    @Test fun lightFeedShowsExactMoneyAndStationRoute() {
        var route: String? = null
        render(CommunityFeedUiState(city = city, fuel = FuelProduct.ETHANOL, loading = false, items = listOf(item)), navigate = { route = it })
        compose.onNodeWithText("Sorriso, MT").assertIsDisplayed()
        capture("community-feed-light.png")
        compose.onNodeWithText("R$", substring = true).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("3,981", substring = true).assertExists()
        compose.onNodeWithText("Synthetic station").performClick()
        assertEquals(Routes.stationProfile(item.stationId), route)
    }
    @Test fun darkFeedRetainsCardsDuringNetworkFailure() {
        render(CommunityFeedUiState(city = city, fuel = FuelProduct.ETHANOL, loading = false, failed = true, items = listOf(item)), dark = true)
        compose.onNodeWithText(context.getString(R.string.feed_unavailable)).performScrollTo().assertIsDisplayed()
        capture("community-feed-dark.png")
        compose.onNodeWithText("Synthetic station").performScrollTo().assertIsDisplayed()
    }
    @Test fun noCoverageNeverInventsPricesAndContributesThroughStationSelection() {
        var route: String? = null
        render(CommunityFeedUiState(city = city, loading = false), navigate = { route = it })
        compose.onNodeWithText(context.getString(R.string.feed_empty_title)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText(context.getString(R.string.community_action_contribute)).performScrollTo().performClick()
        assertEquals(Routes.STATIONS, route)
    }
    @Test fun filtersAndCitySelectorRemainAccessibleAtLargeFont() {
        var selected: FeedSort? = null
        var fuel: FuelProduct? = null
        var route: String? = null
        render(CommunityFeedUiState(city = city, loading = false), font = 2f, sort = { selected = it }, fuel = { fuel = it }, navigate = { route = it })
        compose.onNodeWithText(context.getString(R.string.feed_cheapest)).performScrollTo().performClick()
        assertEquals(FeedSort.CHEAPEST, selected)
        compose.onNodeWithText(context.getString(FuelProductI18n.toStringRes(FuelProduct.ETHANOL))).performScrollTo().performClick()
        assertEquals(FuelProduct.ETHANOL, fuel)
        compose.onNodeWithText(context.getString(R.string.feed_change_city)).performScrollTo().performClick()
        assertEquals(Routes.LOCATION, route)
    }
    private fun capture(name: String) {
        val bitmap = compose.onRoot().captureToImage().asAndroidBitmap()
        File(context.cacheDir, name).outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
    }

}
