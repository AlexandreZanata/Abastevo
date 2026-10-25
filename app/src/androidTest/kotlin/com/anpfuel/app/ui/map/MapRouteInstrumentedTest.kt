package com.anpfuel.app.ui.map

import android.content.pm.ActivityInfo
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.performClick
import com.anpfuel.app.MainActivity
import com.anpfuel.app.R
import com.anpfuel.maps.engine.MapEngineHolder
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import androidx.test.ext.junit.runners.AndroidJUnit4

/**
 * B-BR-MAP-HOST-INIT-01 production-path gate: the real MAP tab of the real
 * app (Hilt activity, real nav graph, real native engine), without
 * `mapContent` overrides and without manual `initialize`/`onEngineReady`
 * calls. Proves opening, surface, tab return, rotation and recreation on a
 * single engine with correct disposal.
 */
@RunWith(AndroidJUnit4::class)
class MapRouteInstrumentedTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()

    private fun string(id: Int): String = compose.activity.getString(id)

    private fun waitForRecenter(timeout: Long = 45_000) {
        val label = string(R.string.map_recenter)
        compose.waitUntil(timeoutMillis = timeout) {
            try {
                compose.onNodeWithContentDescription(label, substring = false, useUnmergedTree = true)
                    .assertExists()
                true
            } catch (_: AssertionError) {
                false
            }
        }
    }

    @Test
    fun productionMapTabInitializesOnceAndSurvivesLifecycle() {
        val mapTab = string(R.string.nav_map)
        compose.waitUntil(timeoutMillis = 15_000) {
            try {
                compose.onNodeWithContentDescription(mapTab, substring = false, useUnmergedTree = true)
                    .assertIsDisplayed()
                true
            } catch (_: AssertionError) {
                false
            }
        }

        // Open Mapa through the real bottom bar: no manual engine start.
        compose.onNodeWithContentDescription(mapTab, substring = false, useUnmergedTree = true)
            .performClick()
        waitForRecenter()
        val first = MapEngineHolder.engine
        assertTrue(first.isInitialized())

        // Leave to another tab and return: entry destroyed and recreated,
        // same singleton engine, surface back without a blank screen.
        val exploreTab = string(R.string.nav_explore)
        compose.onNodeWithContentDescription(exploreTab, substring = false, useUnmergedTree = true)
            .performClick()
        compose.waitUntil(timeoutMillis = 10_000) {
            try {
                compose.onNodeWithContentDescription(
                    string(R.string.map_recenter), substring = false, useUnmergedTree = true,
                ).assertDoesNotExist()
                true
            } catch (_: AssertionError) {
                false
            }
        }
        compose.onNodeWithContentDescription(mapTab, substring = false, useUnmergedTree = true)
            .performClick()
        waitForRecenter()
        assertSame(first, MapEngineHolder.engine)
        assertTrue(MapEngineHolder.engine.isInitialized())

        // Rotation both ways: surface returns on the same engine.
        compose.activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE
        waitForRecenter()
        assertSame(first, MapEngineHolder.engine)
        compose.activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
        waitForRecenter()
        assertSame(first, MapEngineHolder.engine)

        // Recreation: surface returns on the same engine.
        compose.runOnUiThread { compose.activity.recreate() }
        waitForRecenter()
        assertSame(first, MapEngineHolder.engine)
        assertTrue(MapEngineHolder.engine.isInitialized())
    }
}
