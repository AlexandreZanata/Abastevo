package com.anpfuel.app.ui.components

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.core.graphics.Insets
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.ui.theme.AnpFuelTheme
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class AnpScaffoldTest {

    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun anpScaffold_appliesNonZeroTopAndBottomPaddingWhenInsetsMocked() {
        composeTestRule.runOnUiThread {
            val decorView = composeTestRule.activity.window.decorView
            ViewCompat.setOnApplyWindowInsetsListener(decorView) { view, windowInsets ->
                val mocked = WindowInsetsCompat.Builder(windowInsets)
                    .setInsets(
                        WindowInsetsCompat.Type.statusBars(),
                        Insets.of(0, 48, 0, 0),
                    )
                    .setInsets(
                        WindowInsetsCompat.Type.navigationBars(),
                        Insets.of(0, 0, 0, 96),
                    )
                    .setInsets(
                        WindowInsetsCompat.Type.displayCutout(),
                        Insets.NONE,
                    )
                    .build()
                ViewCompat.onApplyWindowInsets(view, mocked)
            }
            decorView.requestApplyInsets()
        }

        var capturedPadding = PaddingValues()

        composeTestRule.setContent {
            AnpFuelTheme(dynamicColor = false) {
                AnpScaffold { innerPadding ->
                    capturedPadding = innerPadding
                    Box(Modifier.fillMaxSize())
                }
            }
        }

        composeTestRule.waitForIdle()

        assertTrue(
            "Expected non-zero top inset padding, got ${capturedPadding.calculateTopPadding()}",
            capturedPadding.calculateTopPadding().value > 0f,
        )
        assertTrue(
            "Expected non-zero bottom inset padding, got ${capturedPadding.calculateBottomPadding()}",
            capturedPadding.calculateBottomPadding().value > 0f,
        )
    }
}
