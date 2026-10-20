package com.anpfuel.app.ui.stations

import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.valueobject.BrazilianState
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Synthetic public rows only; no user preference, account or backend mutation. */
@RunWith(AndroidJUnit4::class)
class StationsPaginationDeviceTest {
    @get:Rule val compose = createComposeRule()
    private val context = InstrumentationRegistry.getInstrumentation().targetContext
    private fun station(index: Int) = ServerStation.create(
        "00000000-0000-4000-8000-%012d".format(index), "Synthetic station $index",
        StationLocationQuality.UNKNOWN, null, null, null, "3550308", "SP", null,
    )
    private fun render(state: () -> StationsUiState, more: () -> Unit = {}) {
        compose.setContent {
            AnpFuelTheme {
                StationsContent(state(), onFuelProductSelected = {}, onSearchQueryChanged = {},
                    onFindNearestStation = {}, onDownloadStationDetail = {}, onRetry = {}, onWeekChanged = {},
                    onStationSelected = {}, onNavigateToStation = {}, onDetailDismissed = {},
                    onNavigateToUpdatePrice = {}, onLoadMoreServerStations = more, weekPickerAction = {})
            }
        }
    }
    private fun state(count: Int) = StationsUiState(
        isLoading = false, municipality = "Synthetic City", state = BrazilianState.SAO_PAULO,
        serverStations = (0 until count).map(::station), serverFromCache = true,
    )

    @Test fun denseCatalogComposesOnlyVisibleRowsAndCanReachItsEnd() {
        render({ state(5_000) })
        val list = compose.onNode(hasScrollToIndexAction())
        for (index in listOf(10, 100, 1_000, 2_500, 4_990)) {
            list.performScrollToIndex(index)
            compose.waitForIdle()
            assertTrue("Lazy list must not compose the entire catalog",
                compose.onAllNodes(hasText("Synthetic station", substring = true)).fetchSemanticsNodes().size < 40)
        }
        list.performScrollToKey(StationsListKeys.directoryRow(station(4_999).stationId))
        compose.onNodeWithText("Synthetic station 4999").assertIsDisplayed()
    }

    @Test fun failedContinuationKeepsRowsAndRetryAppendsNextWindow() {
        val initial = state(20).copy(serverNextCursor = "page-2", serverMoreError = "offline")
        val shown = mutableStateOf(initial)
        var calls = 0
        render({ shown.value }) {
            calls++
            shown.value = state(40).copy(serverNextCursor = null)
        }
        val list = compose.onNode(hasScrollToIndexAction())
        val retry = context.getString(R.string.action_retry)
        list.performScrollToNode(hasText(retry))
        compose.onNodeWithText(retry).performClick()
        compose.runOnIdle { assertEquals(1, calls) }
        list.performScrollToNode(hasText("Synthetic station 39"))
        compose.onNodeWithText("Synthetic station 39").assertIsDisplayed()
        list.performScrollToIndex(0)
        list.performScrollToNode(hasText("Synthetic station 0"))
        compose.onNodeWithText("Synthetic station 0").assertIsDisplayed()
    }
}
