package com.anpfuel.app.ui.components

import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onRoot
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.ui.home.HomeContent
import com.anpfuel.app.ui.home.HomeUiState
import com.anpfuel.app.ui.search.SearchContent
import com.anpfuel.app.ui.search.SearchUiState
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.app.ui.weekpicker.WeekPickerScreen
import com.anpfuel.app.ui.weekpicker.WeekPickerUiState
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.state.DataReadinessState
import com.anpfuel.domain.valueobject.SurveyWeek
import java.time.LocalDate
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Safe-area screenshot matrix (Phase 13.3.2).
 *
 * Run on these emulator profiles for visual regression:
 * - Phone with display cutout (e.g. Pixel 8, API 34)
 * - Phone with gesture navigation enabled
 * - Pixel Tablet API 34 when tablet layout is supported
 */
@RunWith(AndroidJUnit4::class)
class SafeAreaScreenshotMatrixTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun homeScreen_safeAreaLayoutCapture() {
        composeTestRule.setContent {
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

        composeTestRule.onRoot().captureToImage().apply {
            assertTrue(width > 0)
            assertTrue(height > 0)
        }
    }

    @Test
    fun searchScreen_safeAreaLayoutCapture() {
        composeTestRule.setContent {
            AnpFuelTheme(dynamicColor = false) {
                SearchContent(
                    uiState = SearchUiState(query = ""),
                    onQueryChange = {},
                    onResultSelected = {},
                    onBrowseByState = {},
                )
            }
        }

        composeTestRule.onRoot().captureToImage().apply {
            assertTrue(width > 0)
            assertTrue(height > 0)
        }
    }

    @Test
    fun weekPickerScreen_safeAreaLayoutCapture() {
        val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
        composeTestRule.setContent {
            AnpFuelTheme(dynamicColor = false) {
                WeekPickerScreen(
                    uiState = WeekPickerUiState(
                        catalog = listOf(
                            SurveyWeekCatalogEntry.create(
                                surveyWeek = week,
                                summaryUrl = "https://example.com/summary.xlsx",
                                stationUrl = "https://example.com/station.xlsx",
                                publishedAt = LocalDate.parse("2026-06-12"),
                            ),
                        ),
                    ),
                    onRetryCatalog = {},
                    onNavigateBack = {},
                    onUseLatestWeek = {},
                )
            }
        }

        composeTestRule.onRoot().captureToImage().apply {
            assertTrue(width > 0)
            assertTrue(height > 0)
        }
    }
}
