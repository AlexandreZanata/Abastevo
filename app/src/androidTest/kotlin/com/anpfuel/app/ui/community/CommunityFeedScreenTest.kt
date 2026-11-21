package com.anpfuel.app.ui.community

import androidx.activity.compose.setContent
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.graphics.asAndroidBitmap
import android.graphics.Bitmap
import java.io.File
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.unit.Density
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.MainActivity
import com.anpfuel.app.R
import com.anpfuel.app.community.CommunityHelpfulnessPanel
import com.anpfuel.app.community.CommunityHelpfulnessUiState
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.navigation.Routes
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.community.*
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class CommunityFeedScreenTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()
    private val context = InstrumentationRegistry.getInstrumentation().targetContext
    private val city = FeedCity("5107925", BrazilianState.MATO_GROSSO, "Sorriso")
    private val item = CommunityFeedItem("00000000-0000-0000-0000-000000000001", "Synthetic station", FuelProduct.ETHANOL, 3981,
        Instant.parse("2026-10-06T12:00:00Z"), Instant.parse("2026-10-07T12:00:00Z"), 1, 0, "LOW", 1)
    private fun render(state: CommunityFeedUiState, dark: Boolean = false, font: Float = 1f,
        navigate: (String) -> Unit = {}, sort: (FeedSort) -> Unit = {}, fuel: (FuelProduct) -> Unit = {}) {
        // The shared survey-week header resolves its ViewModel from the Hilt activity.
        setActivityContent {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, font)) {
                AnpFuelTheme(darkTheme = dark) {
                    CommunityFeedContent(state, dark, navigate, fuel, sort, {}, {})
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
        assertEquals(Routes.stationPage(item.stationId, FuelProduct.ETHANOL), route)
    }
    @Test fun darkFeedRetainsCardsDuringNetworkFailure() {
        render(CommunityFeedUiState(city = city, fuel = FuelProduct.ETHANOL, loading = false, failed = true, items = listOf(item)), dark = true)
        scrollFeedTo(hasText(context.getString(R.string.feed_unavailable))).assertIsDisplayed()
        capture("community-feed-dark.png")
        scrollFeedTo(hasText("Synthetic station")).assertIsDisplayed()
    }
    @Test fun staleSnapshotShowsAgeHintAndStandardPriceLabel() {
        render(CommunityFeedUiState(city = city, fuel = FuelProduct.ETHANOL, loading = false, stale = true, items = listOf(item)))
        compose.onNodeWithText(context.getString(R.string.feed_standard_prices)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText(context.getString(R.string.feed_cached_hint)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Synthetic station").performScrollTo().assertIsDisplayed()
        capture("opt-community-stale.png")
    }

    @Test fun noCoverageNeverInventsPricesAndOpensCaptureForContribution() {
        var route: String? = null
        render(CommunityFeedUiState(city = city, loading = false), navigate = { route = it })
        val emptyTitle = hasText(context.getString(R.string.feed_empty_title))
        scrollFeedTo(emptyTitle).assertIsDisplayed()
        compose.onNodeWithText("R$", substring = true).assertDoesNotExist()
        val emptyAction = hasText(context.getString(R.string.community_action_contribute)) and hasAnySibling(emptyTitle)
        scrollFeedTo(emptyAction).assertIsDisplayed().performClick()
        assertEquals(Routes.CAPTURE, route)
    }
    @Test fun filtersAndCitySelectorRemainAccessibleAtLargeFont() {
        var selected: FeedSort? = null
        var fuel: FuelProduct? = null
        var route: String? = null
        render(CommunityFeedUiState(city = city, loading = false), font = 2f, sort = { selected = it }, fuel = { fuel = it }, navigate = { route = it })
        scrollFeedTo(hasText(context.getString(R.string.feed_recent))).assertIsDisplayed().performClick()
        compose.onNodeWithText(context.getString(R.string.feed_cheapest)).performScrollTo().performClick()
        assertEquals(FeedSort.CHEAPEST, selected)
        val gasoline = context.getString(FuelProductI18n.toStringRes(FuelProduct.GASOLINE_REGULAR))
        scrollFeedTo(hasText(gasoline)).assertIsDisplayed().performClick()
        compose.onNodeWithText(context.getString(FuelProductI18n.toStringRes(FuelProduct.ETHANOL))).performScrollTo().performClick()
        assertEquals(FuelProduct.ETHANOL, fuel)
        compose.onNodeWithText("Sorriso, MT").performClick()
        assertEquals(Routes.LOCATION, route)
    }
    @Test fun authorGetsOnlyTheOwnershipLabel() {
        renderHelpfulness(CommunityHelpfulnessUiState.OwnContribution)
        compose.onNodeWithText(context.getString(R.string.community_own_contribution)).assertIsDisplayed()
        compose.onNodeWithText(context.getString(R.string.community_helpful_title)).assertDoesNotExist()
        compose.onNodeWithText(context.getString(R.string.community_helpful_report)).assertDoesNotExist()
    }

    @Test fun reportSpansTheTwoVoteButtonsAndOpensTheReportAction() {
        var reported = false
        renderHelpfulness(
            CommunityHelpfulnessUiState.Content("synthetic", 1, "synthetic", 0, 0, 0, null, "HELPFUL", false),
        ) { reported = true }
        fun node(label: Int) = compose.onNodeWithText(context.getString(label))
        val helpful = node(R.string.community_helpful_helpful).fetchSemanticsNode().boundsInRoot
        val notHelpful = node(R.string.community_helpful_not_helpful).fetchSemanticsNode().boundsInRoot
        val report = node(R.string.community_helpful_report)
        val reportBounds = report.fetchSemanticsNode().boundsInRoot
        assertEquals(helpful.left, reportBounds.left, 1f)
        assertEquals(notHelpful.right, reportBounds.right, 1f)
        assertTrue(reportBounds.top >= helpful.bottom)
        report.performClick()
        compose.runOnIdle { assertTrue(reported) }
        node(R.string.community_helpful_remove).assertIsDisplayed()
    }

    @Test fun forumCardIsDisplayedAndNavigatesToFeed() {
        var route: String? = null
        render(CommunityFeedUiState(city = city, loading = false), navigate = { route = it })
        compose.onNodeWithText(context.getString(R.string.forum_card_title)).assertIsDisplayed()
        compose.onNodeWithText(context.getString(R.string.forum_card_description)).assertIsDisplayed()
        // The whole forum card opens the feed; its action label is the card's click label.
        compose.onNodeWithText(context.getString(R.string.forum_card_title)).performClick()
        assertEquals(Routes.FORUM_FEED, route)
    }

    private fun renderHelpfulness(state: CommunityHelpfulnessUiState, onReport: () -> Unit = {}) {
        setActivityContent {
            AnpFuelTheme(darkTheme = true) {
                CommunityHelpfulnessPanel(state, {}, {}, {}, onReport, {})
            }
        }
    }

    private fun scrollFeedTo(matcher: SemanticsMatcher): SemanticsNodeInteraction {
        compose.onNode(hasScrollToIndexAction()).performScrollToNode(matcher)
        return compose.onNode(matcher)
    }

    private fun setActivityContent(content: @Composable () -> Unit) {
        compose.runOnUiThread { compose.activity.setContent(content = content) }
    }

    private fun capture(name: String) {
        val bitmap = compose.onRoot().captureToImage().asAndroidBitmap()
        File(context.cacheDir, name).outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
    }

}
