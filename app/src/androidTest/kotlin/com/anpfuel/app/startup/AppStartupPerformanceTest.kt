package com.anpfuel.app.startup

import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.MainActivity
import com.anpfuel.app.support.InstrumentedAppDataSeeder
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertTrue
import org.junit.BeforeClass
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.system.measureTimeMillis

/**
 * Phase 9.2.3 — Cold start to home must complete within 2 seconds when cache/preferences exist.
 */
@RunWith(AndroidJUnit4::class)
class AppStartupPerformanceTest {

    @get:Rule
    val composeTestRule = createAndroidComposeRule<MainActivity>()

    @Test
    fun reachesHomeScreenWithinTwoSecondsWithCachedPreferences() {
        val elapsedMs = measureTimeMillis {
            composeTestRule.waitUntil(timeoutMillis = STARTUP_LIMIT_MS) {
                try {
                    composeTestRule.onNodeWithText(HOME_TITLE).assertExists()
                    true
                } catch (_: AssertionError) {
                    false
                }
            }
        }

        println("STARTUP_TO_HOME_MS=$elapsedMs")
        assertTrue(
            "Home screen not reached within ${STARTUP_LIMIT_MS}ms (took ${elapsedMs}ms)",
            elapsedMs < STARTUP_LIMIT_MS,
        )
    }

    companion object {
        const val HOME_TITLE = "ANP"
        const val STARTUP_LIMIT_MS = 2_000L

        @BeforeClass
        @JvmStatic
        fun seedCachedPreferences() {
            val context = InstrumentationRegistry.getInstrumentation().targetContext
            runBlocking {
                InstrumentedAppDataSeeder.seedReturningUserHomeState(context)
            }
        }
    }
}
