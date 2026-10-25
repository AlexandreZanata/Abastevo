package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity
import com.anpfuel.data.map.MapPackageStore
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

private fun emptyStore() = object : MapPackageStore {
    override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) = Unit
    override suspend fun generation(generation: String): MapGenerationEntity? = null
    override suspend fun activeGeneration(): MapGenerationEntity? = null
    override suspend fun packages(generation: String) = emptyList<MapPackageEntity>()
    override suspend fun updatePackage(generation: String, packageId: String, status: String, downloadedBytes: Long, etag: String?, error: String?, updatedAtMillis: Long) = Unit
    override suspend fun activate(generation: String, activatedAtMillis: Long) = Unit
    override suspend fun deleteInactive(generation: String) = Unit
}

@RunWith(AndroidJUnit4::class)
class MapScreenInstrumentedTest {
    @get:Rule val compose = createComposeRule()

    @Test fun shellShowsSourceSelectorAndFunctionalEntries() {
        compose.setContent {
            MapScreen(
                onOpenStation = { _, _ -> },
                onOpenSearch = {},
                onOpenDownloads = {},
                viewModel = MapScreenViewModel(emptyStore()),
                mapContent = { Box(Modifier.size(10.dp)) },
            )
        }
        compose.waitForIdle()
        // Top ANP/Community selector.
        compose.onNodeWithText("ANP", ignoreCase = true).assertIsDisplayed()
        // Functional entries.
        compose.onNodeWithText("Filters", ignoreCase = true).assertIsDisplayed()
        compose.onNodeWithTag("map-search-field", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithTag("map-search-submit", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithText("Downloads", ignoreCase = true).assertIsDisplayed()
        // OSM attribution stays visible.
        compose.onNodeWithText("OpenStreetMap", substring = true, ignoreCase = true).assertExists()
    }

    @Test fun sourceSwitchUpdatesSelector() {
        val vm = MapScreenViewModel(emptyStore())
        compose.setContent {
            MapScreen(
                onOpenStation = { _, _ -> },
                onOpenSearch = {},
                onOpenDownloads = {},
                viewModel = vm,
                mapContent = { Box(Modifier.size(10.dp)) },
            )
        }
        compose.waitForIdle()
        vm.selectSource(MapPriceSource.COMMUNITY)
        compose.waitForIdle()
        compose.onNodeWithText("Community", ignoreCase = true).assertIsDisplayed()
    }

    @Test fun clickingSourceSelectorAndShowCommunityButtonReloadsChargingSheet() {
        val vm = MapScreenViewModel(emptyStore())
        val charger = com.anpfuel.maps.engine.MapOverlayMarker(
            uuid = "33333333-3333-4333-8333-333333333333",
            latitude = -25.42,
            longitude = -49.27,
            title = "Eletroposto Centro",
            fuel = false,
            charging = true,
            locationQuality = com.anpfuel.maps.engine.MapStationLocationQuality.REVIEWED,
        )
        val sheetVm = MapSheetViewModel(SheetPriceSource { uuid, source, _ ->
            if (uuid == charger.uuid && source == MapPriceSource.COMMUNITY) {
                SheetPrice("CCS2", "kWh", "2.45", MapPriceSource.COMMUNITY, "2026-10-25")
            } else {
                null
            }
        })
        var reportedProduct: String? = null
        compose.setContent {
            MapScreen(
                onOpenStation = { _, _ -> },
                onOpenSearch = {},
                onOpenDownloads = {},
                onReportStation = { _, product -> reportedProduct = product },
                viewModel = vm,
                sheet = sheetVm,
                mapContent = { Box(Modifier.size(10.dp)) },
            )
        }
        compose.runOnIdle {
            sheetVm.select(charger, vm.ui.value.source)
        }
        compose.waitForIdle()
        // In ANP mode, charging shows anp-no-tariff message and "Show Community" button.
        compose.onNodeWithTag("map-sheet-switch-community").assertIsDisplayed()
        compose.onNodeWithTag("map-sheet-switch-community").performClick()
        // Wait for 100ms debounce + lookup completion.
        compose.mainClock.advanceTimeBy(250)
        compose.waitForIdle()
        compose.onNodeWithText("CCS2 2.45 kWh", substring = true).assertIsDisplayed()

        // Clicking top ANP chip switches back to ANP and shows Show Community button again.
        compose.onNodeWithTag("map-source-anp").performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("map-sheet-switch-community").assertIsDisplayed()

        // Clicking top Community chip reloads Community price on the same UUID.
        compose.onNodeWithTag("map-source-community").performClick()
        compose.mainClock.advanceTimeBy(250)
        compose.waitForIdle()
        compose.onNodeWithText("CCS2 2.45 kWh", substring = true).assertIsDisplayed()

        // Clicking Report on a charging-only station passes CHARGING (not GASOLINE_REGULAR).
        compose.onNodeWithTag("map-sheet-report").performClick()
        compose.waitForIdle()
        org.junit.Assert.assertEquals("CHARGING", reportedProduct)
    }
}
