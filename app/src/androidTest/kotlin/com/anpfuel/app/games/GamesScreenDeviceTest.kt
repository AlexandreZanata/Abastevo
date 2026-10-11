package com.anpfuel.app.games

import android.app.UiAutomation
import android.content.Context
import android.content.res.Configuration
import android.view.View
import android.view.ViewGroup
import java.util.Locale
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
import androidx.compose.runtime.remember
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipe
import androidx.compose.ui.test.click
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
import com.anpfuel.domain.games.FuelFlight
import com.anpfuel.domain.games.FuelRun
import com.anpfuel.domain.games.GameId
import com.anpfuel.domain.games.GamePhase
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.After
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
        InstrumentationRegistry.getInstrumentation().uiAutomation.setRotation(UiAutomation.ROTATION_FREEZE_0)
        rule.waitUntil(5_000) {
            val config = rule.activity.resources.configuration
            config.screenHeightDp > config.screenWidthDp
        }
        assertTrue(context.getSharedPreferences(TEST_STORAGE, Context.MODE_PRIVATE).edit().clear().commit())
        preferences = GamesPreferences(context, TEST_STORAGE)
    }

    @After fun releaseTestRotation() {
        InstrumentationRegistry.getInstrumentation().uiAutomation.setRotation(UiAutomation.ROTATION_UNFREEZE)
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

    @Test fun nativeRunnerAcceptsSwipeLaneButtonsJumpAndSlide() {
        show(); waitForSelector("RUN")
        rule.onNodeWithTag("games_choose_RUN").performScrollTo().performClick()
        rule.onNodeWithTag("games_start").performScrollTo().performClick()
        rule.onNodeWithTag("games_playfield").performTouchInput {
            swipe(center, Offset(0f, center.y), 60)
        }
        waitForGame { (session.simulation as FuelRun).lane == -1 }
        rule.onNodeWithTag("games_action_RIGHT").performClick()
        waitForGame { (session.simulation as FuelRun).lane == 0 }
        rule.onNodeWithTag("games_action_JUMP").performClick()
        waitForGame { (session.simulation as FuelRun).jumpTicks > 0 }
        waitForGame { (session.simulation as FuelRun).jumpTicks == 0 }
        rule.onNodeWithTag("games_action_SLIDE").performClick()
        waitForGame { (session.simulation as FuelRun).slideTicks > 0 }
        capture("run-input")
        rule.onNodeWithTag("games_pause").performClick()
        waitForGame { session.phase == GamePhase.PAUSED }
    }

    @Test fun flightFinishesAndRestartAcceptsButtonAndNativeTap() {
        show(); waitForSelector("FLIGHT")
        rule.onNodeWithTag("games_choose_FLIGHT").performClick()
        rule.onNodeWithTag("games_start").performScrollTo().performClick()
        waitForGame { session.phase == GamePhase.FINISHED }
        rule.onNodeWithTag("games_final_score").performScrollTo().assertIsDisplayed()
        capture("flight-results")
        rule.onNodeWithTag("games_restart").performScrollTo().performClick()
        rule.onNodeWithTag("games_action_IMPULSE").performClick()
        waitForGame { session.phase == GamePhase.RUNNING && (session.simulation as FuelFlight).velocity < 0 }
        rule.onNodeWithTag("games_playfield").performTouchInput { click(center) }
        waitForGame { (session.simulation as FuelFlight).velocity < 0 }
        capture("flight-input")
        rule.onNodeWithTag("games_pause").performClick()
        rule.onNodeWithTag("games_resume").assertIsDisplayed()
    }

    @Test fun settingsPersistAcrossStoreRecreation() {
        show(); waitForSelector("RUN")
        rule.onNodeWithText(context.getString(R.string.games_muted)).performScrollTo().performClick()
        rule.onNodeWithText(context.getString(R.string.games_reduced_motion)).performScrollTo().performClick()
        rule.onNodeWithText(context.getString(R.string.games_low_power)).performScrollTo().performClick()
        rule.runOnIdle {
            assertEquals(GameOptions(muted = false, reducedMotion = true, lowPower = true),
                GamesPreferences(context, TEST_STORAGE).options())
        }
    }

    @Test fun portugueseDarkThemeKeepsTutorialAndBothGamesPlayable() {
        show(dark = true, language = "pt-BR"); waitForSelector("FLIGHT")
        capture("selector-pt-dark")
        rule.onNodeWithTag("games_choose_FLIGHT").performClick()
        rule.onNodeWithTag("games_start").performScrollTo().assertIsDisplayed()
        capture("flight-tutorial-pt-dark")
        rule.onNodeWithTag("games_start").performClick()
        // Bounded test driver uses the same native action port as the buttons.
        waitForGame(timeout = 10_000) {
            val flight = session.simulation as FuelFlight
            if (flight.y > 650 && flight.velocity >= 0) action(com.anpfuel.domain.games.GameAction.IMPULSE)
            flight.tick >= 180
        }
        rule.onNodeWithTag("games_action_IMPULSE").assertIsDisplayed()
        rule.onNodeWithTag("games_pause").assertIsDisplayed()
        capture("flight-play-pt-dark")
        rule.onNodeWithTag("games_pause").performClick()
        val localized = context.createConfigurationContext(Configuration(context.resources.configuration).apply {
            setLocale(Locale.forLanguageTag("pt-BR"))
        })
        rule.onNodeWithText(localized.getString(R.string.games_select_other)).performScrollTo().performClick()
        rule.onNodeWithTag("games_choose_RUN").performScrollTo().performClick()
        rule.onNodeWithTag("games_start").performScrollTo().performClick()
        waitForGame(timeout = 10_000) { session.simulation.tick >= 180 }
        capture("run-play-pt-dark")
        rule.onNodeWithTag("games_pause").performClick()
        rule.onNodeWithTag("games_resume").assertIsDisplayed()
    }

    private fun waitForGame(timeout: Long = 5_000, predicate: ArcadeView.() -> Boolean) {
        rule.waitUntil(timeout) {
            var matches = false
            rule.runOnUiThread { matches = checkNotNull(findArcade(rule.activity.window.decorView)).predicate() }
            matches
        }
    }
    private fun findArcade(view: View): ArcadeView? {
        if (view is ArcadeView) return view
        if (view is ViewGroup) for (i in 0 until view.childCount) findArcade(view.getChildAt(i))?.let { return it }
        return null
    }

    private fun capture(name: String) {
        rule.waitForIdle()
        val screenshot = rule.onRoot().captureToImage().asAndroidBitmap()
        val directory = File(context.getExternalFilesDir(null), "game-validation").apply { mkdirs() }
        FileOutputStream(File(directory, "$name.png")).use { screenshot.compress(Bitmap.CompressFormat.PNG, 100, it) }
        screenshot.recycle()
    }

    private fun show(fontScale: Float = 1f, dark: Boolean = false, language: String? = null) {
        rule.setContent {
            val density = LocalDensity.current
            val host = LocalContext.current
            val localized = remember(host, language) {
                if (language == null) host else host.createConfigurationContext(Configuration(host.resources.configuration).apply {
                    setLocale(Locale.forLanguageTag(language))
                })
            }
            CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale),
                LocalContext provides localized, LocalConfiguration provides localized.resources.configuration) {
                AnpFuelTheme(dynamicColor = false, darkTheme = dark) { GamesScreen(onNavigateBack = {}, preferences = preferences) }
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
