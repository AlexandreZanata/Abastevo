package com.anpfuel.app.ui.search

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.model.MunicipalitySearchResult
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class SearchScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun showsMinimumCharactersHintForShortQuery() {
        composeTestRule.setContent {
            AnpFuelTheme {
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

        composeTestRule.onNodeWithText("Type at least 2 characters to search.").assertIsDisplayed()
    }

    @Test
    fun showsMunicipalityResultsWithState() {
        composeTestRule.setContent {
            AnpFuelTheme {
                SearchContent(
                    uiState = SearchUiState(
                        query = "CUR",
                        results = listOf(
                            MunicipalitySearchResult(
                                municipality = "Curitiba",
                                state = BrazilianState.PARANA,
                                dataAvailability = DataAvailability.HAS_DATA,
                            ),
                        ),
                    ),
                    onQueryChange = {},
                    onResultSelected = {},
                    onBrowseByState = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Curitiba — PR").assertIsDisplayed()
    }
}
