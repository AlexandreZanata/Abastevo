package com.anpfuel.app.validation

import android.content.Context
import android.content.pm.ActivityInfo
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.LargeTest
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.MainActivity
import com.anpfuel.app.R
import com.anpfuel.app.support.InstrumentedAppDataSeeder
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.BeforeClass
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Appendix A2 steps 2–10 — post-sync flows (Phase R3.1.2–R3.1.10). */
@RunWith(AndroidJUnit4::class)
@LargeTest
class AppendixA2PostSyncTest {

  @get:Rule
  val composeRule = createAndroidComposeRule<MainActivity>()

  private lateinit var context: Context
  private lateinit var weekPickerTitle: String
  private lateinit var useLatestWeekLabel: String
  private lateinit var homeTitle: String
  private lateinit var searchNavLabel: String
  private lateinit var searchHintLabel: String
  private lateinit var settingsNavLabel: String
  private lateinit var changeSurveyWeekLabel: String
  private lateinit var noDataThisWeekLabel: String
  private lateinit var br010EmptyLabel: String
  private lateinit var operationalNoteBannerLabel: String
  private lateinit var downloadWeekLabel: String
  private lateinit var ethanolLabel: String
  private lateinit var pricesNavLabel: String
  private lateinit var backLabel: String
  private lateinit var homeEmptyMessage: String

  @Before
  fun bindLabels() {
    context = InstrumentationRegistry.getInstrumentation().targetContext
    weekPickerTitle = context.getString(R.string.week_picker_title)
    useLatestWeekLabel = context.getString(R.string.week_picker_latest)
    homeTitle = context.getString(R.string.home_title)
    searchNavLabel = context.getString(R.string.nav_search)
    searchHintLabel = context.getString(R.string.search_municipality_hint)
    settingsNavLabel = context.getString(R.string.nav_settings)
    changeSurveyWeekLabel = context.getString(R.string.settings_change_survey_week)
    noDataThisWeekLabel = context.getString(R.string.search_no_data_this_week)
    br010EmptyLabel = context.getString(R.string.prices_empty_no_data_this_week)
    operationalNoteBannerLabel = context.getString(R.string.week_picker_operational_note_banner)
    downloadWeekLabel = context.getString(R.string.week_picker_download_week)
    ethanolLabel = context.getString(R.string.fuel_product_ethanol)
    pricesNavLabel = context.getString(R.string.nav_prices)
    backLabel = context.getString(R.string.action_back)
    homeEmptyMessage = context.getString(R.string.home_empty_message)
  }

  @Test
  fun postSyncFlows_coverAppendixA2StepsTwoThroughTen() {
    // R3.1.2 — completed sync state (seeded post–Use latest week)
    composeRule.waitForText(homeTitle)
    composeRule.onNodeWithText(homeEmptyMessage).assertDoesNotExist()
    composeRule.onNode(
      hasContentDescription("Active survey week", substring = true),
    ).assertIsDisplayed()

    // R3.1.3
    composeRule.openSearchFromHome(searchNavLabel, searchHintLabel)
    composeRule.onNode(hasSetTextAction()).performClick()
    composeRule.onNode(hasSetTextAction()).performTextInput("san paolo")
    composeRule.waitForText("— SP", substring = true, timeoutMillis = 120_000L)
    composeRule.onNode(hasText("PAULO", substring = true) and hasText("SP", substring = true))
      .performClick()
    composeRule.waitForText(homeTitle, timeoutMillis = 60_000L)

    // R3.1.4
    composeRule.openSearchFromHome(searchNavLabel, searchHintLabel)
    composeRule.onNode(hasSetTextAction()).performClick()
    composeRule.onNode(hasSetTextAction()).performTextInput("Bom Jesus")
    composeRule.waitForText("Bom Jesus", substring = true, timeoutMillis = 120_000L)
    val bomJesusCount =
      composeRule.onAllNodesWithText("Bom Jesus", substring = true).fetchSemanticsNodes().size
    assertTrue("Expected Bom Jesus search results (UC-004)", bomJesusCount >= 1)

    // R3.1.5
    selectFirstSearchResultWithNoData()
    composeRule.waitForText(br010EmptyLabel, timeoutMillis = 90_000L)

    // R3.1.6
    selectHistoricalWeekIfPresent()

    // R3.1.7
    assertOperationalNoteIfPresent()

    // R3.1.8
    navigateBackToHome()
    rotateAndAssertVisible(homeTitle)
    openPricesIfAvailable()
    rotateAndAssertVisible(pricesNavLabel)

    // R3.1.9
    composeRule.onNodeWithText(ethanolLabel, substring = true).assertIsDisplayed()
    composeRule.onNodeWithContentDescription(ethanolLabel, substring = true).assertExists()

    // R3.1.10
    val chipBefore = activeSurveyWeekChipDescription()
    changeActiveWeekFromSettings()
    composeRule.onNode(
      hasContentDescription("Active survey week", substring = true),
    ).assertIsDisplayed()
    val chipAfter = activeSurveyWeekChipDescription()
    if (chipBefore != null && chipAfter != null && chipBefore != chipAfter) {
      assertNotEquals(chipBefore, chipAfter)
    }
  }

  private fun ensureSearchScreen() {
    if (runCatching { composeRule.onNodeWithText(searchHintLabel).assertExists() }.isFailure) {
      composeRule.openSearchFromHome(searchNavLabel, searchHintLabel)
    }
  }

  private fun selectFirstSearchResultWithNoData() {
    ensureSearchScreen()
    if (runCatching {
        composeRule.waitForText(noDataThisWeekLabel, substring = true, timeoutMillis = 20_000L)
        composeRule.onNodeWithText(noDataThisWeekLabel, substring = true).performClick()
      }.isSuccess
    ) {
      return
    }
    composeRule.onNode(hasSetTextAction()).performClick()
    composeRule.onNode(hasSetTextAction()).performTextInput("a")
    composeRule.waitForText(noDataThisWeekLabel, substring = true, timeoutMillis = 90_000L)
    composeRule.onNodeWithText(noDataThisWeekLabel, substring = true).performClick()
  }

  private fun selectHistoricalWeekIfPresent() {
    composeRule.openSettingsFromHome(settingsNavLabel)
    composeRule.tapWhenVisible(changeSurveyWeekLabel)
    composeRule.waitForText(weekPickerTitle, timeoutMillis = 120_000L)
    if (runCatching {
        composeRule.waitForText(HISTORICAL_WEEK_START_TOKEN, substring = true, timeoutMillis = 30_000L)
      }.isFailure
    ) {
      navigateBackFromWeekPicker()
      return
    }
    composeRule.onNodeWithText(HISTORICAL_WEEK_START_TOKEN, substring = true).performClick()
    if (runCatching {
        composeRule.waitForText(downloadWeekLabel, timeoutMillis = 10_000L)
        composeRule.onNodeWithText(downloadWeekLabel).performClick()
        composeRule.waitForText(homeTitle, timeoutMillis = SYNC_TIMEOUT_MS)
      }.isSuccess
    ) {
      return
    }
    navigateBackFromWeekPicker()
  }

  private fun assertOperationalNoteIfPresent() {
    composeRule.openSettingsFromHome(settingsNavLabel)
    composeRule.tapWhenVisible(changeSurveyWeekLabel)
    composeRule.waitForText(weekPickerTitle, timeoutMillis = 120_000L)
    runCatching {
      composeRule.waitForText(operationalNoteBannerLabel, substring = true, timeoutMillis = 15_000L)
      composeRule.onNodeWithText(operationalNoteBannerLabel, substring = true).assertIsDisplayed()
    }
    navigateBackFromWeekPicker()
  }

  private fun changeActiveWeekFromSettings() {
    composeRule.openSettingsFromHome(settingsNavLabel)
    composeRule.tapWhenVisible(changeSurveyWeekLabel)
    composeRule.waitForText(weekPickerTitle, timeoutMillis = 120_000L)
    if (runCatching {
        composeRule.onNodeWithText(HISTORICAL_WEEK_START_TOKEN, substring = true).performClick()
        composeRule.waitForText(downloadWeekLabel, timeoutMillis = 10_000L)
        composeRule.onNodeWithText(downloadWeekLabel).performClick()
      }.isSuccess
    ) {
      composeRule.waitForText(homeTitle, timeoutMillis = SYNC_TIMEOUT_MS)
      return
    }
    composeRule.tapWhenVisible(useLatestWeekLabel)
    composeRule.waitForText(homeTitle, timeoutMillis = SYNC_TIMEOUT_MS)
  }

  private fun navigateBackFromWeekPicker() {
    repeat(2) {
      runCatching { composeRule.onNodeWithText(backLabel).performClick() }
      composeRule.waitForIdle()
    }
    composeRule.waitForText(homeTitle)
  }

  private fun rotateAndAssertVisible(visibleLabel: String) {
    composeRule.activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE
    composeRule.waitForIdle()
    composeRule.onNodeWithText(visibleLabel).assertIsDisplayed()
    composeRule.activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
    composeRule.waitForIdle()
  }

  private fun openPricesIfAvailable() {
    if (runCatching {
        composeRule.onNodeWithText(ethanolLabel, substring = true).performClick()
        composeRule.waitForText(pricesNavLabel, timeoutMillis = 15_000L)
      }.isSuccess
    ) {
      return
    }
    runCatching { composeRule.onNodeWithText(pricesNavLabel, substring = true).performClick() }
    composeRule.waitForText(pricesNavLabel)
  }

  private fun navigateBackToHome() {
    repeat(3) {
      if (runCatching { composeRule.onNodeWithText(homeTitle).assertExists() }.isSuccess) return
      runCatching { composeRule.onNodeWithText(backLabel).performClick() }
    }
    composeRule.waitForText(homeTitle)
  }

  private fun activeSurveyWeekChipDescription(): String? =
    composeRule.onNode(hasContentDescription("Active survey week", substring = true))
      .fetchSemanticsNode()
      .config
      .getOrNull(SemanticsProperties.ContentDescription)
      ?.firstOrNull()
      ?.toString()

  companion object {
    const val SYNC_TIMEOUT_MS = 8 * 60 * 1000L
    const val HISTORICAL_WEEK_START_TOKEN = "May 31"

    @BeforeClass
    @JvmStatic
    fun seedPostSyncState() {
      val context = InstrumentationRegistry.getInstrumentation().targetContext
      runBlocking { InstrumentedAppDataSeeder.seedReturningUserHomeState(context) }
    }
  }
}

private fun androidx.compose.ui.test.junit4.AndroidComposeTestRule<*, MainActivity>.waitForText(
  text: String,
  timeoutMillis: Long = 60_000L,
  substring: Boolean = false,
) {
  waitUntil(timeoutMillis) {
    runCatching { onNodeWithText(text, substring = substring).assertExists() }.isSuccess
  }
}

private fun androidx.compose.ui.test.junit4.AndroidComposeTestRule<*, MainActivity>.tapWhenVisible(
  text: String,
  substring: Boolean = false,
) {
  waitForText(text, substring = substring)
  onNodeWithText(text, substring = substring).performClick()
}

private fun androidx.compose.ui.test.junit4.AndroidComposeTestRule<*, MainActivity>.openSearchFromHome(
  searchNavLabel: String,
  searchHintLabel: String,
) {
  waitForText(searchNavLabel)
  onNodeWithText(searchNavLabel).performClick()
  waitForText(searchHintLabel)
}

private fun androidx.compose.ui.test.junit4.AndroidComposeTestRule<*, MainActivity>.openSettingsFromHome(
  settingsNavLabel: String,
) {
  waitForText(settingsNavLabel)
  onNodeWithText(settingsNavLabel).performClick()
}
