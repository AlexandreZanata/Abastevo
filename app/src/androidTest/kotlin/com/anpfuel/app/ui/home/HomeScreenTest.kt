package com.anpfuel.app.ui.home

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.navigation.Routes
import com.anpfuel.app.ui.model.AveragePriceUiModel
import com.anpfuel.app.ui.model.TankFillCostEstimateUiModel
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.model.TankFillCostUnitPriceSource
import com.anpfuel.domain.state.DataReadinessState
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class HomeScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun showsMunicipalityPricesWhenDataIsAvailable() {
        composeTestRule.setContent {
            AnpFuelTheme {
                HomeContent(
                    uiState = HomeUiState(
                        isLoading = false,
                        readiness = DataReadinessState.READY,
                        hasCachedData = true,
                        hasLocation = true,
                        municipality = "Curitiba",
                        state = BrazilianState.PARANA,
                        surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"),
                        prices = listOf(
                            AveragePriceUiModel(
                                fuelProduct = FuelProduct.ETHANOL,
                                averageFormatted = "R$ 3,42",
                                minimumFormatted = "R$ 3,10",
                                maximumFormatted = "R$ 3,80",
                                stationCount = 42,
                            ),
                        ),
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

        composeTestRule.onNodeWithText("Curitiba, PR").assertIsDisplayed()
        composeTestRule.onNodeWithText("R$ 3,42", substring = true).assertIsDisplayed()
    }

    @Test
    fun tappingFuelCardOpensStationListForThatFuel() {
        var navigatedRoute: String? = null

        composeTestRule.setContent {
            AnpFuelTheme {
                HomeContent(
                    uiState = HomeUiState(
                        isLoading = false,
                        readiness = DataReadinessState.READY,
                        hasCachedData = true,
                        hasLocation = true,
                        municipality = "Curitiba",
                        state = BrazilianState.PARANA,
                        surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"),
                        prices = listOf(
                            AveragePriceUiModel(
                                fuelProduct = FuelProduct.ETHANOL,
                                averageFormatted = "R$ 3,42",
                                minimumFormatted = "R$ 3,10",
                                maximumFormatted = "R$ 3,80",
                                stationCount = 42,
                            ),
                        ),
                    ),
                    darkTheme = false,
                    onToggleTheme = {},
                    onNavigate = { route -> navigatedRoute = route },
                    onRefresh = {},
                    onRetry = {},
                    onWeekChanged = {},
                    includeSurveyWeekChip = false,
                )
            }
        }

        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val fuelLabel = context.getString(FuelProductI18n.toStringRes(FuelProduct.ETHANOL))
        val cardDescription = context.getString(R.string.a11y_fuel_price_card, fuelLabel, "R$ 3,42")

        composeTestRule.onNodeWithContentDescription(cardDescription).performClick()

        assertEquals(Routes.stations(FuelProduct.ETHANOL), navigatedRoute)
    }

    @Test
    fun showsEmptyStateWhenNoCachedData() {
        composeTestRule.setContent {
            AnpFuelTheme {
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

        composeTestRule.onNodeWithText("No fuel price data synced yet.").assertIsDisplayed()
    }

    @Test
    fun showsTankFillPlaceholderWhenNoVehicles() {
        composeTestRule.setContent {
            AnpFuelTheme {
                HomeContent(
                    uiState = HomeUiState(
                        isLoading = false,
                        readiness = DataReadinessState.READY,
                        hasCachedData = true,
                        hasLocation = true,
                        municipality = "Curitiba",
                        state = BrazilianState.PARANA,
                        surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"),
                        prices = emptyList(),
                        tankFillCostEstimates = emptyList(),
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

        composeTestRule.onNodeWithText("Track your real fill-up cost").assertIsDisplayed()
    }

    @Test
    fun showsTankFillCostCardWhenVehicleRegistered() {
        composeTestRule.setContent {
            AnpFuelTheme {
                HomeContent(
                    uiState = HomeUiState(
                        isLoading = false,
                        readiness = DataReadinessState.READY,
                        hasCachedData = true,
                        hasLocation = true,
                        municipality = "Curitiba",
                        state = BrazilianState.PARANA,
                        surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"),
                        prices = emptyList(),
                        tankFillCostEstimates = listOf(
                            TankFillCostEstimateUiModel(
                                vehicleId = DomainId.generate(),
                                displayName = "Gol",
                                tankCapacityLiters = 50,
                                tankCapacityLitersLabel = "50",
                                fuelProduct = FuelProduct.GASOLINE_REGULAR,
                                totalCostFormatted = "R$ 274,50",
                                stationDisplayName = null,
                                stationNavigationQuery = null,
                                unitPriceSource = TankFillCostUnitPriceSource.CHEAPEST_STATION,
                            ),
                        ),
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

        composeTestRule.onNodeWithText("Gol").assertIsDisplayed()
        composeTestRule.onNodeWithText("R$ 274,50", substring = true).assertIsDisplayed()
    }
}
