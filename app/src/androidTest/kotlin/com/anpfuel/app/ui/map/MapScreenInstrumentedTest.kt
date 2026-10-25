package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
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
}
