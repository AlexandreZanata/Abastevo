package com.anpfuel.app.ui.components

import android.view.WindowManager
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.OutlinedTextField
import androidx.compose.runtime.getValue
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.getUnclippedBoundsInRoot
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.ui.theme.AnpFuelTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class AnpScaffoldTest {

    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun nativeKeyboardKeepsFocusedScrollableInputVisible() {
        var keyboardHeight = 0
        var rootHeight = 0
        var density = 1f
        composeTestRule.runOnUiThread {
            composeTestRule.activity.enableEdgeToEdge()
            composeTestRule.activity.window.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE)
            // Rule.setContent replaces platform input with a test session.
            // Direct Activity content is required to exercise the native IME.
            composeTestRule.activity.setContent {
                AnpFuelTheme(dynamicColor = false) {
                    density = LocalDensity.current.density
                    keyboardHeight = WindowInsets.ime.getBottom(LocalDensity.current)
                    Box(Modifier.fillMaxSize().onGloballyPositioned { rootHeight = it.size.height }) {
                        AnpScaffold { padding ->
                            Column(Modifier.fillMaxSize().padding(padding).imePadding().verticalScroll(rememberScrollState())) {
                                Spacer(Modifier.height(900.dp))
                                OutlinedTextField(
                                    value = "synthetic",
                                    onValueChange = {},
                                    singleLine = true,
                                    modifier = Modifier.testTag("focused-input"),
                                )
                            }
                        }
                    }
                }
            }
        }
        composeTestRule.waitForIdle()
        // Test-host attachment uses adjustPan; restore the production policy
        // after attachment/focus so API 26 reports IME resize insets.
        composeTestRule.onNodeWithTag("focused-input").performScrollTo().performClick()
        composeTestRule.runOnUiThread {
            composeTestRule.activity.window.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE)
            composeTestRule.activity.window.decorView.requestApplyInsets()
        }
        composeTestRule.waitUntil(10_000L) { keyboardHeight > 0 }
        composeTestRule.waitForIdle()
        val fieldBottom = composeTestRule.onNodeWithTag("focused-input").getUnclippedBoundsInRoot().bottom.value * density
        assertTrue("Focused input overlaps native keyboard", fieldBottom <= rootHeight - keyboardHeight + 2f * density)
    }

    @Test
    fun nestedInputInsetsAreReservedOnceForDifferentKeyboardHeights() {
        var bottomInset by androidx.compose.runtime.mutableIntStateOf(220)
        var contentHeight = 0
        var inputViewportHeight = 0
        composeTestRule.setContent {
            AnpFuelTheme(dynamicColor = false) {
                val insets = WindowInsets(top = 24, bottom = bottomInset)
                AnpScaffold(contentWindowInsets = insets) { padding ->
                    Box(Modifier.fillMaxSize().padding(padding).onGloballyPositioned { contentHeight = it.size.height }) {
                        Box(Modifier.fillMaxSize().windowInsetsPadding(insets)
                            .onGloballyPositioned { inputViewportHeight = it.size.height })
                    }
                }
            }
        }
        listOf(220, 360, 0).forEach { height ->
            composeTestRule.runOnIdle { bottomInset = height }
            composeTestRule.waitForIdle()
            assertTrue("Screen must retain a usable viewport", contentHeight > 0)
            assertEquals("Nested input reserved system/keyboard space twice", contentHeight, inputViewportHeight)
        }
    }

    @Test
    fun anpScaffold_appliesNonZeroTopAndBottomPaddingWhenInsetsMocked() {
        // Deterministically inject the same system bar sizes at the Scaffold
        // boundary; a decor listener before content attachment drops events.
        var pxPerDp = 1f
        var capturedPadding = PaddingValues()

        composeTestRule.setContent {
            AnpFuelTheme(dynamicColor = false) {
                pxPerDp = LocalDensity.current.density
                AnpScaffold(contentWindowInsets = WindowInsets(top = 48, bottom = 96)) { innerPadding ->
                    capturedPadding = innerPadding
                    Box(Modifier.fillMaxSize())
                }
            }
        }

        composeTestRule.waitForIdle()

        assertEquals(48f, capturedPadding.calculateTopPadding().value * pxPerDp, 0.1f)
        assertEquals(96f, capturedPadding.calculateBottomPadding().value * pxPerDp, 0.1f)
    }
}
