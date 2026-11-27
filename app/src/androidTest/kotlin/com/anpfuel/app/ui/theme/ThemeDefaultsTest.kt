package com.anpfuel.app.ui.theme

import android.content.res.Configuration
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.SideEffect
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ThemeDefaultsTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun defaultsToLightEvenWhenTheSystemUsesDarkMode() {
        val configuration = Configuration(
            InstrumentationRegistry.getInstrumentation().targetContext.resources.configuration,
        ).apply {
            uiMode = (uiMode and Configuration.UI_MODE_NIGHT_MASK.inv()) or
                Configuration.UI_MODE_NIGHT_YES
        }
        var isDark: Boolean? = null

        compose.setContent {
            CompositionLocalProvider(LocalConfiguration provides configuration) {
                AnpFuelTheme {
                    val dark = LocalAnpDarkTheme.current
                    SideEffect { isDark = dark }
                }
            }
        }

        compose.runOnIdle { assertFalse(checkNotNull(isDark)) }
    }

    @Test
    fun explicitManualDarkModeStillUsesTheDarkTheme() {
        var isDark: Boolean? = null

        compose.setContent {
            AnpFuelTheme(darkTheme = true) {
                val dark = LocalAnpDarkTheme.current
                SideEffect { isDark = dark }
            }
        }

        compose.runOnIdle { assertTrue(checkNotNull(isDark)) }
    }
}
