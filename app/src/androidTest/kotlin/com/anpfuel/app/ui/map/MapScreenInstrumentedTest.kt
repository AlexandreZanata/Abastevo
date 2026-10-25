package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class MapScreenInstrumentedTest {
    @get:Rule val compose = createComposeRule()

    @Test fun shellShowsSourceSelectorAndFunctionalEntries() {
        compose.setContent {
            MapScreen(
                onOpenStation = { _, _ -> },
                onOpenSearch = {},
                onOpenDownloads = {},
                viewModel = MapScreenViewModel(),
                mapContent = { Box(Modifier.size(10.dp)) },
            )
        }
        compose.waitForIdle()
        // Top ANP/Community selector.
        compose.onNodeWithText("ANP", ignoreCase = true).assertIsDisplayed()
        // Functional entries.
        compose.onNodeWithText("Filters", ignoreCase = true).assertIsDisplayed()
        compose.onNodeWithTag("map-search-field", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithTag("map-search-submit", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithText("Downloads", ignoreCase = true).assertIsDisplayed()
        // OSM attribution stays visible.
        compose.onNodeWithText("OpenStreetMap", substring = true, ignoreCase = true).assertExists()
    }

    @Test fun sourceSwitchUpdatesSelector() {
        val vm = MapScreenViewModel()
        compose.setContent {
            MapScreen(
                onOpenStation = { _, _ -> },
                onOpenSearch = {},
                onOpenDownloads = {},
                viewModel = vm,
                mapContent = { Box(Modifier.size(10.dp)) },
            )
        }
        compose.waitForIdle()
        vm.selectSource(MapPriceSource.COMMUNITY)
        compose.waitForIdle()
        compose.onNodeWithText("Community", ignoreCase = true).assertIsDisplayed()
    }
}
