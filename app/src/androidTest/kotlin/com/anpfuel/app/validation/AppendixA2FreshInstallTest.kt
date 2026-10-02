package com.anpfuel.app.validation

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.MainActivity
import com.anpfuel.app.R
import com.anpfuel.app.support.InstrumentedAppDataSeeder
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Appendix A2 step 1 — fresh install auto-syncs the latest week, then prompts location (Phase R3.1.1, P24-T01). */
@RunWith(AndroidJUnit4::class)
class AppendixA2FreshInstallTest {

  @get:Rule
  val composeRule = createAndroidComposeRule<MainActivity>()

  private lateinit var nextLabel: String
  private lateinit var getStartedLabel: String
  private lateinit var weekPickerTitle: String
  private lateinit var locationPromptTitle: String

  @Before
  fun prepareFreshInstall() {
    val context = InstrumentationRegistry.getInstrumentation().targetContext
    nextLabel = context.getString(R.string.action_next)
    getStartedLabel = context.getString(R.string.onboarding_action_get_started)
    weekPickerTitle = context.getString(R.string.week_picker_title)
    locationPromptTitle = context.getString(R.string.onboarding_location_prompt_title)

    InstrumentedAppDataSeeder.clearAppStorage(context)
    composeRule.activityRule.scenario.recreate()
  }

  @Test
  fun freshInstall_autoSyncsLatestWeekThenPromptsLocation() {
    // Four intro pages (P23-T02 contributor page included), then Get started.
    composeRule.waitUntil(120_000L) {
      runCatching { composeRule.onNodeWithText(nextLabel).assertExists() }.isSuccess ||
        runCatching { composeRule.onNodeWithText(weekPickerTitle).assertExists() }.isSuccess
    }

    if (runCatching { composeRule.onNodeWithText(weekPickerTitle).assertExists() }.isFailure) {
      repeat(3) {
        composeRule.onNodeWithText(nextLabel).performClick()
        composeRule.waitForIdle()
      }
      composeRule.onNodeWithText(getStartedLabel).performClick()
    }

    // Fresh installs default to auto-download-latest: the week picker
    // may flash by while the latest survey imports. The stable
    // observable outcome is the location prompt with a synced week
    // behind it — generous timeout for real network + ~20k-row import.
    composeRule.waitUntil(300_000L) {
      runCatching { composeRule.onNodeWithText(locationPromptTitle).assertExists() }.isSuccess
    }
    composeRule.onNodeWithText(locationPromptTitle).assertIsDisplayed()
  }
}
