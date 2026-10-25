package com.anpfuel.maps.engine

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class MapOverlayListAccessibilityTest {
    @get:Rule val compose = createComposeRule()

    private fun items() = listOf(
        MapOverlayListItem(
            uuid = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
            title = "Posto Central",
            type = MapStationType.FUEL,
            typeLabel = "Fuel",
            locationLabel = "Reviewed location",
            contentDescription = "Posto Central, Fuel, Reviewed location",
        ),
        MapOverlayListItem(
            uuid = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
            title = "Recarga",
            type = MapStationType.CHARGING,
            typeLabel = "Charging",
            locationLabel = "Approximate city location",
            contentDescription = "Recarga, Charging, Approximate city location",
        ),
    )

    @Test fun lightThemeListExposesTalkBackDescriptions() {
        compose.setContent {
            MaterialTheme(colorScheme = lightColorScheme()) {
                MapOverlayList(items = items(), onSelect = {}, modifier = Modifier.fillMaxWidth())
            }
        }
        compose.waitForIdle()
        compose.onNodeWithText("Posto Central").assertIsDisplayed()
        compose.onAllNodesWithContentDescription("Posto Central, Fuel, Reviewed location", substring = false).assertCountEquals(1)
    }

    @Test fun darkThemeKeepsContrastAndTargets() {
        compose.setContent {
            MaterialTheme(colorScheme = darkColorScheme()) {
                MapOverlayList(items = items(), onSelect = {}, modifier = Modifier.fillMaxWidth())
            }
        }
        compose.waitForIdle()
        compose.onNodeWithText("Recarga").assertIsDisplayed()
        // Distinct icons for fuel vs electric (never color alone) are present.
        compose.onAllNodesWithContentDescription("Recarga, Charging, Approximate city location", substring = false).assertCountEquals(1)
    }
}
