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

    @Test fun clusterExpandAllowsSelectingSecondAndLastMemberAndUnpinnedDisablesRoute() {
        val aId = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
        val bId = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
        val cId = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
        val unpinnedId = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"

        val gateway = object : com.anpfuel.domain.repository.ServerStationGateway {
            override suspend fun list(limit: Int, cursor: String?) =
                com.anpfuel.domain.discovery.ServerStationPage(emptyList(), null)
            override suspend fun listInMunicipality(municipalityCode: String, limit: Int, cursor: String?) =
                com.anpfuel.domain.discovery.ServerStationPage(emptyList(), null)
            override suspend fun search(municipalityCode: String, query: String, limit: Int, cursor: String?) =
                com.anpfuel.domain.discovery.ServerStationPage(emptyList(), null)
            override suspend fun nearby(lat: Double, lon: Double, radiusMeters: Int, limit: Int) = listOf(
                com.anpfuel.domain.discovery.NearbyServerStation(
                    com.anpfuel.domain.discovery.ServerStation.create(
                        aId, "Posto Alpha", com.anpfuel.domain.discovery.StationLocationQuality.REVIEWED,
                        -25.4200, -49.2700, null, "4106902", "PR", null,
                    ),
                    10.0,
                ),
                com.anpfuel.domain.discovery.NearbyServerStation(
                    com.anpfuel.domain.discovery.ServerStation.create(
                        bId, "Posto Beta", com.anpfuel.domain.discovery.StationLocationQuality.REVIEWED,
                        -25.4201, -49.2701, null, "4106902", "PR", null,
                    ),
                    20.0,
                ),
                com.anpfuel.domain.discovery.NearbyServerStation(
                    com.anpfuel.domain.discovery.ServerStation.create(
                        cId, "Posto Gamma", com.anpfuel.domain.discovery.StationLocationQuality.REVIEWED,
                        -25.4202, -49.2702, null, "4106902", "PR", null,
                    ),
                    30.0,
                ),
                com.anpfuel.domain.discovery.NearbyServerStation(
                    com.anpfuel.domain.discovery.ServerStation.create(
                        unpinnedId, "Posto Sem Coordenada", com.anpfuel.domain.discovery.StationLocationQuality.UNKNOWN,
                        null, null, null, "4106902", "PR", null,
                    ),
                    40.0,
                ),
            )
            override suspend fun byCnpj(cnpj: String) = null
            override suspend fun detail(stationId: String) = error("not used")
        }
        val fakeEngine = object : com.anpfuel.maps.engine.MapEngine {
            override fun initialize(context: android.content.Context, onReady: () -> Unit) = true
            override fun newMapView(context: android.content.Context): android.view.View = throw UnsupportedOperationException()
            override fun setViewport(latitude: Double, longitude: Double, zoom: Int) = Unit
            override fun currentCenter() = null
            override fun recenter() = false
            override fun isInitialized() = true
            override fun search(context: android.content.Context, query: String, onResult: (List<com.anpfuel.maps.engine.MapSearchResult>) -> Unit) = true
            override fun buildCarRoute(sLat: Double, sLon: Double, eLat: Double, eLon: Double, onResult: (com.anpfuel.maps.engine.MapRouteInfo?) -> Unit) = true
            override fun setOverlayMarkers(markers: List<com.anpfuel.maps.engine.MapOverlayMarker>) = true
            override fun overlayClusters() = emptyList<com.anpfuel.maps.engine.MapOverlayCluster>()
            override fun setOnOverlaySelected(listener: (com.anpfuel.maps.engine.MapOverlaySelection) -> Unit) = Unit
            override fun cancelSearch() = Unit
            override fun cancelRoute() = Unit
            override fun release() = Unit
        }
        val discoveryVm = MapDiscoveryViewModel(gateway, ChargingMarkersSource { emptyList() }, fakeEngine)
        val sheetVm = MapSheetViewModel(SheetPriceSource { _, _, _ -> null })
        var openedProfileUuid: String? = null

        compose.setContent {
            MapScreen(
                onOpenStation = { uuid, _ -> openedProfileUuid = uuid },
                onOpenSearch = {},
                onOpenDownloads = {},
                viewModel = MapScreenViewModel(emptyStore()),
                discovery = discoveryVm,
                sheet = sheetVm,
                mapContent = { Box(Modifier.size(10.dp)) },
            )
        }
        // Wait for initial 300ms viewport debounce.
        compose.mainClock.advanceTimeBy(500)
        compose.waitForIdle()

        val clusterId = discoveryVm.ui.value.clusters.single().id
        compose.onNodeWithTag("map-cluster-expand-$clusterId").performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("map-cluster-chooser").assertIsDisplayed()

        // Select 2nd member (Posto Beta).
        compose.onNodeWithTag("map-cluster-member-$bId").performClick()
        compose.mainClock.advanceTimeBy(200)
        compose.waitForIdle()
        compose.onNodeWithText("Posto Beta").assertIsDisplayed()

        // Expand again and select last member (Posto Gamma).
        compose.onNodeWithTag("map-cluster-expand-$clusterId").performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("map-cluster-member-$cId").performClick()
        compose.mainClock.advanceTimeBy(200)
        compose.waitForIdle()
        compose.onNodeWithText("Posto Gamma").assertIsDisplayed()

        // Open accessible list and select coordinate-less station: sheet opens with route unavailable and profile enabled.
        compose.onNodeWithTag("map-list-toggle").performClick()
        compose.waitForIdle()
        compose.onNodeWithText("Posto Sem Coordenada", substring = true).performClick()
        compose.mainClock.advanceTimeBy(200)
        compose.waitForIdle()
        compose.onNodeWithTag("map-sheet-route-unavailable").assertIsDisplayed()
        compose.onNodeWithTag("map-sheet-profile").performClick()
        compose.waitForIdle()
        org.junit.Assert.assertEquals(unpinnedId, openedProfileUuid)
    }
}
