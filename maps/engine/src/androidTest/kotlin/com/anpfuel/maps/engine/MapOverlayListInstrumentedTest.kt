package com.anpfuel.maps.engine

import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.performClick
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class MapOverlayListInstrumentedTest {
    @get:Rule val compose = createComposeRule()

    @Test
    fun listExposesTalkBackDescriptionsAnd48dpTargets() {
        val items = listOf(
            MapOverlayListItem(
                uuid = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
                title = "Posto Central",
                typeLabel = "Fuel and charging",
                locationLabel = "Reviewed location",
                contentDescription = "Posto Central, Fuel and charging, Reviewed location",
            ),
            MapOverlayListItem(
                uuid = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
                title = "Posto Sem Local",
                typeLabel = "Fuel",
                locationLabel = "Location unknown",
                contentDescription = "Posto Sem Local, Fuel, Location unknown",
            ),
        )
        var selected: String? = null
        compose.setContent {
            MapOverlayList(items = items, onSelect = { selected = it })
        }

        compose.onNodeWithContentDescription(items[0].contentDescription)
            .assertIsDisplayed()
            .assertHeightIsAtLeast(48.dp)
            .performClick()
        assertEquals(items[0].uuid, selected)
    }
}
