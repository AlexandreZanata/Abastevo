package com.anpfuel.app.ui.accessibility

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.unit.Density
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.components.LoadingState
import com.anpfuel.app.ui.home.HomeContent
import com.anpfuel.app.ui.home.HomeUiState
import com.anpfuel.app.ui.search.SearchContent
import com.anpfuel.app.ui.search.SearchUiState
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.state.DataReadinessState
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class AccessibilityUiTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun loadingStateExposesTalkBackDescription() {
        val loadingDescription = InstrumentationRegistry.getInstrumentation()
            .targetContext
            .getString(R.string.a11y_loading)

        composeTestRule.setContent {
            AnpFuelTheme(dynamicColor = false) {
                LoadingState()
            }
        }

        composeTestRule.onNodeWithContentDescription(loadingDescription).assertIsDisplayed()
    }

    @Test
    fun errorStateRendersWithHeadingAndMessageAtDoubleFontScale() {
        composeTestRule.setContent {
            CompositionLocalProvider(LocalDensity provides Density(density = 1f, fontScale = 2f)) {
                AnpFuelTheme(dynamicColor = false) {
                    ErrorState(
                        message = "Network unavailable.",
                        onRetry = {},
                    )
                }
            }
        }

        composeTestRule.onNodeWithText("Something went wrong").assertIsDisplayed()
        composeTestRule.onNodeWithText("Network unavailable.").assertIsDisplayed()
        composeTestRule.onNodeWithText("Retry").assertIsDisplayed()
    }

    @Test
    fun homeScreenSupportsDoubleFontScaleWithoutClippingPrimaryContent() {
        composeTestRule.setContent {
            CompositionLocalProvider(LocalDensity provides Density(density = 1f, fontScale = 2f)) {
                AnpFuelTheme(dynamicColor = false) {
                    HomeContent(
                        uiState = HomeUiState(
                            isLoading = false,
                            readiness = DataReadinessState.EMPTY,
                            hasCachedData = false,
                        ),
                        darkTheme = false,
                        onToggleTheme = {},
                        onNavigate = {},
                        onRefresh = {},
                        onRetry = {},
                        onWeekChanged = {},
                        includeSurveyWeekChip = false,
                    )
                }
            }
        }

        composeTestRule.onNodeWithText("ANP").assertIsDisplayed()
        composeTestRule.onNodeWithText("No fuel price data synced yet.").assertIsDisplayed()
    }

    @Test
    fun searchScreenSupportsDoubleFontScaleWithoutClippingHint() {
        composeTestRule.setContent {
            CompositionLocalProvider(LocalDensity provides Density(density = 1f, fontScale = 2f)) {
                AnpFuelTheme(dynamicColor = false) {
                    SearchContent(
                        uiState = SearchUiState(
                            query = "CU",
                            showMinCharsHint = true,
                        ),
                        onQueryChange = {},
                        onResultSelected = {},
                        onBrowseByState = {},
                    )
                }
            }
        }

        composeTestRule.onNodeWithText("Type at least 2 characters to search.").assertIsDisplayed()
    }
}
