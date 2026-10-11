package com.anpfuel.app.games

import android.content.Context
import android.graphics.Bitmap
import java.io.File
import java.io.FileOutputStream
import androidx.activity.ComponentActivity
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.width
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.graphics.toPixelMap
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.dp
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.unit.Density
import androidx.lifecycle.Lifecycle
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.games.GameId
import com.anpfuel.domain.games.GamePhase
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Uses an isolated preference file; never clears account data or real game records. */
@RunWith(AndroidJUnit4::class)
class GamesScreenDeviceTest {
    @get:Rule val rule = createAndroidComposeRule<ComponentActivity>()
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext
    private lateinit var preferences: GamesPreferences

    @Before fun prepareOnlyTestGameStorage() {
        assertTrue(context.getSharedPreferences(TEST_STORAGE, Context.MODE_PRIVATE).edit().clear().commit())
        preferences = GamesPreferences(context, TEST_STORAGE)
    }

    @Test fun guestCanChooseEitherGameWithoutAnAccount() {
        show()
        waitForSelector("FLIGHT")
        rule.onNodeWithTag("games_choose_FLIGHT").performClick()
        rule.onNodeWithTag("games_start").performScrollTo().assertIsDisplayed()
        rule.onNodeWithText(context.getString(R.string.games_flight_tutorial)).performScrollTo().assertIsDisplayed()
        rule.onNodeWithText(context.getString(R.string.games_select_other)).performScrollTo().performClick()
        rule.onNodeWithTag("games_choose_RUN").performScrollTo().performClick()
        rule.onNodeWithText(context.getString(R.string.games_run_tutorial)).performScrollTo().assertIsDisplayed()
    }

    @Test fun backgroundPausesAndForegroundRequiresExplicitContinue() {
        show(); waitForSelector("RUN")
        rule.onNodeWithTag("games_choose_RUN").performScrollTo().performClick()
        rule.onNodeWithTag("games_start").performScrollTo().performClick()
        rule.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
        rule.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
        rule.onNodeWithTag("games_resume").performScrollTo().assertIsDisplayed().performClick()
        rule.onNodeWithTag("games_pause").performScrollTo().performClick()
        rule.onNodeWithTag("games_resume").performScrollTo().assertIsDisplayed()
    }

    @Test fun largeFontKeepsBothSelectorsAndTutorialActionReachable() {
        show(fontScale = 2f); waitForSelector("RUN")
        rule.onNodeWithTag("games_choose_FLIGHT").performScrollTo().assertIsDisplayed()
        rule.onNodeWithTag("games_choose_RUN").performScrollTo().performClick()
        rule.onNodeWithTag("games_start").performScrollTo().assertIsDisplayed()
        rule.onNodeWithText(context.getString(R.string.games_select_other)).performScrollTo().assertIsDisplayed()
    }

    @Test fun completedRecordsPersistAndWrongPreferenceTypeRecovers() {
        val disk = context.getSharedPreferences(TEST_STORAGE, Context.MODE_PRIVATE)
        assertTrue(disk.edit().putString("RUN_mobile-v1_best", "corrupt").commit())
        assertEquals(0, preferences.records.best(GameId.RUN))
        assertEquals(BestSave(41, true), preferences.records.record(GameId.RUN, GamePhase.FINISHED, 41))
        assertEquals(BestSave(7, true), preferences.records.record(GameId.FLIGHT, GamePhase.FINISHED, 7))
        val reopened = GamesPreferences(context, TEST_STORAGE)
        assertEquals(41, reopened.records.best(GameId.RUN))
        assertEquals(7, reopened.records.best(GameId.FLIGHT))
        assertEquals(true, reopened.options().muted)
    }

    @Test fun captureOfflineScenesForVisualReview() {
        show(); waitForSelector("FLIGHT")
        capture("selector")
        rule.onNodeWithTag("games_choose_FLIGHT").performClick()
        capture("flight-tutorial")
        rule.onNodeWithTag("games_start").performScrollTo().performClick()
        capture("flight-play")
        rule.onNodeWithTag("games_pause").performScrollTo().performClick()
        rule.onNodeWithText(context.getString(R.string.games_select_other)).performScrollTo().performClick()
        rule.onNodeWithTag("games_choose_RUN").performScrollTo().performClick()
        capture("run-tutorial")
        rule.onNodeWithTag("games_start").performScrollTo().performClick()
        capture("run-play")
        rule.onNodeWithTag("games_pause").performScrollTo().performClick()
        capture("run-paused")
    }

    @Test fun previewCannotPaintIntoTextAreaBelowItsBounds() {
        rule.setContent {
            Column(Modifier.width(200.dp).height(200.dp).background(Color.White).testTag("preview_bounds")) {
                GameArtwork(GameId.FLIGHT, Modifier.width(200.dp).height(100.dp))
            }
        }
        val image = rule.onNodeWithTag("preview_bounds").captureToImage()
        val pixels = image.toPixelMap()
        assertEquals(Color.White.toArgb(), pixels[image.width / 2, image.height * 3 / 4].toArgb())
    }

    private fun capture(name: String) {
        rule.waitForIdle()
        val screenshot = rule.onRoot().captureToImage().asAndroidBitmap()
        val directory = File(context.getExternalFilesDir(null), "game-validation").apply { mkdirs() }
        FileOutputStream(File(directory, "$name.png")).use { screenshot.compress(Bitmap.CompressFormat.PNG, 100, it) }
        screenshot.recycle()
    }

    private fun show(fontScale: Float = 1f) {
        rule.setContent {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale)) {
                AnpFuelTheme(dynamicColor = false) { GamesScreen(onNavigateBack = {}, preferences = preferences) }
            }
        }
    }
    private fun waitForSelector(game: String) {
        rule.waitUntil(5_000) {
            rule.onAllNodesWithTag("games_choose_$game").fetchSemanticsNodes().singleOrNull()?.let {
                !it.config.contains(SemanticsProperties.Disabled)
            } ?: false
        }
    }
    private companion object { const val TEST_STORAGE = "offline_games_instrumentation" }
}
