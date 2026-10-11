package com.anpfuel.app.games

import android.app.UiAutomation
import android.content.Context
import android.graphics.Bitmap
import androidx.activity.ComponentActivity
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.text.TextLayoutResult
import com.anpfuel.app.R
import org.junit.Assert.assertEquals
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.unit.Density
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.ui.theme.AnpFuelTheme
import java.io.File
import java.io.FileOutputStream
import org.junit.Assert.assertTrue
import org.junit.After
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Explicit instrumentation rotation; execute only on the selected disposable emulator. */
@RunWith(AndroidJUnit4::class)
class GamesLandscapeDeviceTest {
    @get:Rule val rule = createAndroidComposeRule<ComponentActivity>()
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext
    private lateinit var preferences: GamesPreferences

    @Before fun prepareOnlyTestStorage() {
        InstrumentationRegistry.getInstrumentation().uiAutomation.setRotation(UiAutomation.ROTATION_FREEZE_90)
        rule.waitUntil(5_000) {
            val config = rule.activity.resources.configuration
            config.screenWidthDp > config.screenHeightDp
        }
        val config = rule.activity.resources.configuration
        assertTrue("Requires landscape activity", config.screenWidthDp > config.screenHeightDp)
        assertTrue(context.getSharedPreferences(STORAGE, Context.MODE_PRIVATE).edit().clear().commit())
        preferences = GamesPreferences(context, STORAGE)
    }

    @After fun releaseTestRotation() {
        InstrumentationRegistry.getInstrumentation().uiAutomation.setRotation(UiAutomation.ROTATION_UNFREEZE)
    }

    @Test fun landscapeGameAndSideControlsRemainReachable() {
        show(1f)
        rule.onNodeWithTag("games_choose_RUN").performScrollTo().performClick()
        rule.onNodeWithTag("games_start").performScrollTo().performClick()
        for (action in listOf("LEFT", "RIGHT", "JUMP", "SLIDE")) {
            rule.onNodeWithTag("games_action_$action").performScrollTo().assertIsDisplayed()
        }
        for (label in listOf(R.string.games_left, R.string.games_right, R.string.games_jump, R.string.games_slide)) {
            val layouts = mutableListOf<TextLayoutResult>()
            rule.onNodeWithText(context.getString(label), useUnmergedTree = true)
                .performSemanticsAction(SemanticsActions.GetTextLayoutResult) { it(layouts) }
            assertEquals("Default-scale control label must fit one line", 1, layouts.single().lineCount)
        }
        capture("landscape-run")
        rule.onNodeWithTag("games_pause").performScrollTo().performClick()
        rule.onNodeWithTag("games_resume").performScrollTo().assertIsDisplayed()
        capture("landscape-paused")
        rule.onNodeWithTag("games_resume").performScrollTo().performClick()
        rule.onNodeWithTag("games_action_LEFT").performScrollTo().assertIsDisplayed().performClick()
        rule.onNodeWithTag("games_pause").performScrollTo().performClick()
    }

    @Test fun landscapeDoubleFontKeepsTutorialActionReachable() {
        show(2f)
        rule.onNodeWithTag("games_choose_FLIGHT").performScrollTo().performClick()
        rule.onNodeWithTag("games_start").performScrollTo().assertIsDisplayed()
        capture("landscape-font2")
        rule.onNodeWithTag("games_start").performClick()
        rule.onNodeWithTag("games_action_IMPULSE").performScrollTo().assertIsDisplayed().performClick()
        rule.onNodeWithTag("games_pause").performScrollTo().assertIsDisplayed().performClick()
        rule.onNodeWithTag("games_resume").performScrollTo().assertIsDisplayed()
    }

    private fun show(fontScale: Float) {
        rule.setContent {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale)) {
                AnpFuelTheme(dynamicColor = false, darkTheme = true) {
                    GamesScreen(onNavigateBack = {}, preferences = preferences)
                }
            }
        }
        rule.waitUntil(5_000) {
            rule.onNodeWithTag("games_choose_RUN").fetchSemanticsNode().config.let {
                !it.contains(androidx.compose.ui.semantics.SemanticsProperties.Disabled)
            }
        }
    }
    private fun capture(name: String) {
        val bitmap = rule.onRoot().captureToImage().asAndroidBitmap()
        val directory = File(context.getExternalFilesDir(null), "game-validation").apply { mkdirs() }
        FileOutputStream(File(directory, "$name.png")).use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
        bitmap.recycle()
    }
    private companion object { const val STORAGE = "offline_games_landscape_instrumentation" }
}
