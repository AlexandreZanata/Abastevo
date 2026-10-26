package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.size
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.getUnclippedBoundsInRoot
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
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
                guidance = testGuidance(),
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
        compose.onNodeWithTag("map-open-downloads").assertIsDisplayed()
        // OSM attribution stays visible.
        compose.onNodeWithText("OpenStreetMap", substring = true, ignoreCase = true).assertExists()
    }

    @Test fun sourceSwitchUpdatesSelector() {
        val vm = MapScreenViewModel(emptyStore())
        compose.setContent {
            MapScreen(
                guidance = testGuidance(),
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
                guidance = testGuidance(),
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
        compose.waitUntil(5_000) { sheetVm.ui.value.price?.source == MapPriceSource.COMMUNITY }
        compose.waitForIdle()
        compose.onNodeWithText("CCS2 2.45 kWh", substring = true).assertIsDisplayed()

        // Clicking top ANP chip switches back to ANP and shows Show Community button again.
        compose.onNodeWithTag("map-sheet-source-anp").performClick()
        compose.waitUntil(5_000) { sheetVm.ui.value.absentReason == "anp-no-tariff" }
        compose.onNodeWithTag("map-sheet-switch-community").assertIsDisplayed()

        // Clicking top Community chip reloads Community price on the same UUID.
        compose.onNodeWithTag("map-sheet-source-community").performClick()
        compose.waitUntil(5_000) { sheetVm.ui.value.price?.source == MapPriceSource.COMMUNITY }
        compose.waitForIdle()
        compose.onNodeWithText("CCS2 2.45 kWh", substring = true).assertIsDisplayed()

        // Clicking Report on a charging-only station passes CHARGING (not GASOLINE_REGULAR).
        compose.onNodeWithTag("map-sheet-report").performScrollTo().performClick()
        compose.waitForIdle()
        org.junit.Assert.assertEquals("CHARGING", reportedProduct)
    }

    @Test fun extendedFiltersRemainReachableAndApplyTheirSelectedValues() {
        var selectedSource: MapPriceSource? = null
        var applied: List<Any?>? = null
        compose.setContent {
            MapFiltersDialog(
                source = MapPriceSource.ANP,
                showFuel = true,
                showCharging = true,
                onSelectSource = { selectedSource = it },
                onServiceFilter = { _, _ -> },
                onDismiss = {},
                onApplyExtendedFilters = { fuel, charging, product, connector, current, access, power ->
                    applied = listOf(fuel, charging, product, connector, current, access, power)
                },
            )
        }
        compose.onNodeWithTag("map-filter-source-community").performScrollTo().performClick()
        org.junit.Assert.assertNull(selectedSource)
        compose.onNodeWithTag("map-filter-current-DC").performScrollTo().assertIsDisplayed().performClick().assertIsSelected()
        compose.onNodeWithTag("map-filter-power-150").performScrollTo().assertIsDisplayed().performClick().assertIsSelected()
        compose.onNodeWithTag("map-filter-access-PUBLIC").performScrollTo().assertIsDisplayed().performClick().assertIsSelected()
        compose.onNodeWithTag("map-filter-apply").performClick()
        org.junit.Assert.assertEquals(MapPriceSource.COMMUNITY, selectedSource)
        org.junit.Assert.assertEquals(
            listOf(
                true, true, com.anpfuel.domain.valueobject.FuelProduct.GASOLINE_REGULAR, null,
                com.anpfuel.domain.charging.ChargingCurrent.DC,
                com.anpfuel.domain.charging.ChargingAccess.PUBLIC, 150,
            ),
            applied,
        )
    }

    @Test fun citySearchOpensStandardPickerAndCancelDoesNotApplyDrafts() {
        var opened = false
        var applied = false
        var dismissed = false
        compose.setContent {
            MapFiltersDialog(
                source = MapPriceSource.ANP,
                showFuel = true,
                showCharging = true,
                onSelectSource = { applied = true },
                onServiceFilter = { _, _ -> applied = true },
                onDismiss = { dismissed = true },
                cityLabel = "Sorriso / MT",
                onOpenCity = { opened = true },
            )
        }
        compose.onNodeWithTag("map-filter-city-search").performClick()
        org.junit.Assert.assertTrue(opened)
        compose.onNodeWithTag("map-filter-source-community").performScrollTo().performClick()
        selectFuelFromSharedDialog(com.anpfuel.domain.valueobject.FuelProduct.DIESEL_S10)
        compose.onNodeWithTag("map-filter-cancel").performClick()
        org.junit.Assert.assertTrue(dismissed)
        org.junit.Assert.assertFalse(applied)
    }

    @Test fun everyFuelCanBeSelectedThroughTheSharedModalWithoutApplyingTheDraft() {
        var selected: com.anpfuel.domain.valueobject.FuelProduct? = null
        compose.setContent {
            MapFiltersDialog(
                source = MapPriceSource.ANP,
                showFuel = true,
                showCharging = true,
                onSelectSource = {},
                onServiceFilter = { _, _ -> },
                onDismiss = {},
                onApplyExtendedFilters = { _, _, product, _, _, _, _ -> selected = product },
            )
        }
        compose.onNodeWithTag("map-filter-fuel-picker").assertDoesNotExist()
        com.anpfuel.domain.valueobject.FuelProduct.entries.forEach { product ->
            selectFuelFromSharedDialog(product)
            org.junit.Assert.assertNull(selected)
        }
        compose.onNodeWithTag("map-filter-apply").performClick()
        org.junit.Assert.assertEquals(com.anpfuel.domain.valueobject.FuelProduct.entries.last(), selected)
    }

    @Test fun filterActionsAndAllFuelChoicesRemainReachableAtDoubleFontScale() {
        compose.setContent {
            val density = androidx.compose.ui.platform.LocalDensity.current
            androidx.compose.runtime.CompositionLocalProvider(
                androidx.compose.ui.platform.LocalDensity provides androidx.compose.ui.unit.Density(density.density, 2f),
            ) {
                com.anpfuel.app.ui.theme.AnpFuelTheme(darkTheme = true, dynamicColor = false) {
                    MapFiltersDialog(
                        source = MapPriceSource.ANP,
                        showFuel = true,
                        showCharging = true,
                        onSelectSource = {},
                        onServiceFilter = { _, _ -> },
                        onDismiss = {},
                    )
                }
            }
        }
        compose.onNodeWithTag("map-filter-apply").assertIsDisplayed()
        compose.onNodeWithTag("map-filter-cancel").assertIsDisplayed()
        com.anpfuel.domain.valueobject.FuelProduct.entries.forEach { product ->
            selectFuelFromSharedDialog(product)
        }
        compose.onNodeWithTag("map-filter-access-PUBLIC").performScrollTo().assertIsDisplayed()
        compose.onNodeWithTag("map-filter-apply").assertIsDisplayed()
    }

    @Test fun mapControlsStartCompactAndCanExpandWithoutReplacingMap() {
        compose.setContent {
            MapScreen(
                guidance = testGuidance(),
                onOpenStation = { _, _ -> },
                onOpenSearch = {},
                onOpenDownloads = {},
                viewModel = MapScreenViewModel(emptyStore()),
                mapContent = { Box(Modifier.size(10.dp).testTag("test-native-map")) },
            )
        }
        compose.onNodeWithTag("map-search-field").assertIsDisplayed()
        compose.onNodeWithTag("map-source-anp").assertDoesNotExist()
        compose.onNodeWithTag("map-controls-toggle").performClick()
        compose.onNodeWithTag("map-source-community").assertIsDisplayed()
        compose.onNodeWithTag("map-controls-toggle").performClick()
        compose.onNodeWithTag("map-source-community").assertDoesNotExist()
        compose.onNodeWithTag("test-native-map").assertExists()
    }

    @Test fun cityStartsAtTheLeftAndLongNamesAreVisuallyEllipsized() {
        val city = androidx.compose.runtime.mutableStateOf("Sorriso")
        compose.setContent {
            MapControlsCard(
                ui = MapScreenUi(),
                municipalityName = city.value,
                cityLocationUnknown = false,
                offlineCached = false,
                onOpenCity = {},
                onSelectSource = {},
                onSearch = {},
                onClearSearch = {},
                onRetry = {},
                onOpenDownloads = {},
                onOpenRoute = null,
                periodContent = {},
                modifier = Modifier.width(320.dp),
            )
        }
        val label = compose.onNodeWithTag("map-city-label", useUnmergedTree = true)
        val cardLeft = compose.onNodeWithTag("map-controls-card").getUnclippedBoundsInRoot().left
        org.junit.Assert.assertTrue(label.getUnclippedBoundsInRoot().left - cardLeft <= 48.dp)
        val longCity = "Vila Bela da Santíssima Trindade — a very long municipality name"
        compose.runOnIdle { city.value = longCity }
        val layouts = mutableListOf<androidx.compose.ui.text.TextLayoutResult>()
        label.performSemanticsAction(androidx.compose.ui.semantics.SemanticsActions.GetTextLayoutResult) { action -> action(layouts) }
        org.junit.Assert.assertEquals(1, layouts.single().lineCount)
        org.junit.Assert.assertTrue(layouts.single().isLineEllipsized(0))
        label.assert(hasText(longCity))
        compose.onNodeWithTag("map-controls-toggle").assertIsDisplayed()
    }

    private fun selectFuelFromSharedDialog(product: com.anpfuel.domain.valueobject.FuelProduct) {
        val label = androidx.test.platform.app.InstrumentationRegistry.getInstrumentation()
            .targetContext.getString(com.anpfuel.app.mapper.FuelProductI18n.toStringRes(product))
        compose.onNodeWithTag("map-filter-fuel-selector").performScrollTo().performClick()
        compose.onNode(
            hasText(label) and hasAnyAncestor(hasTestTag("map-filter-fuel-picker")),
        ).performScrollTo().assertIsDisplayed()
        compose.onNodeWithTag("fuel-selector-icon-${product.name}", useUnmergedTree = true).assertExists()
        compose.onNode(
            hasText(label) and hasAnyAncestor(hasTestTag("map-filter-fuel-picker")),
        ).performClick()
        compose.onNodeWithTag("map-filter-fuel-picker").assertDoesNotExist()
        compose.onNodeWithTag("map-filter-fuel-selector").assert(hasText(label))
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
                guidance = testGuidance(),
                onOpenStation = { uuid, _ -> openedProfileUuid = uuid },
                onOpenSearch = {},
                onOpenDownloads = {},
                viewModel = MapScreenViewModel(emptyStore()),
                discovery = discoveryVm,
                sheet = sheetVm,
                mapContent = { Box(Modifier.size(10.dp)) },
            )
        }
        compose.waitUntil(5_000) { discoveryVm.ui.value.clusters.size == 1 }
        val clusterId = discoveryVm.ui.value.clusters.single().id
        compose.onNodeWithTag("map-cluster-expand-$clusterId").performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("map-cluster-chooser").assertIsDisplayed()

        // Select 2nd member (Posto Beta).
        compose.onNodeWithTag("map-cluster-member-$bId").performClick()
        compose.waitUntil(5_000) { sheetVm.ui.value.marker?.uuid == bId }
        compose.onNodeWithText("Posto Beta").assertIsDisplayed()
        androidx.test.platform.app.InstrumentationRegistry.getInstrumentation().sendKeyDownUpSync(android.view.KeyEvent.KEYCODE_BACK)
        compose.waitUntil(5_000) { discoveryVm.ui.value.selectedUuid == null }

        // Expand again and select last member (Posto Gamma).
        compose.onNodeWithTag("map-cluster-expand-$clusterId").performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("map-cluster-member-$cId").performClick()
        compose.waitUntil(5_000) { sheetVm.ui.value.marker?.uuid == cId }
        compose.onNodeWithText("Posto Gamma").assertIsDisplayed()
        androidx.test.platform.app.InstrumentationRegistry.getInstrumentation().sendKeyDownUpSync(android.view.KeyEvent.KEYCODE_BACK)
        compose.waitUntil(5_000) { discoveryVm.ui.value.selectedUuid == null }

        // Open accessible list and select coordinate-less station: sheet opens with route unavailable and profile enabled.
        compose.onNodeWithTag("map-list-toggle").performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("map-overlay-list").performScrollToNode(hasText("Posto Sem Coordenada", substring = true))
        compose.onNodeWithText("Posto Sem Coordenada", substring = true).performClick()
        compose.waitUntil(5_000) { sheetVm.ui.value.marker?.uuid == unpinnedId }
        compose.onNodeWithTag("map-sheet-route-unavailable").performScrollTo().assertIsDisplayed()
        compose.onNodeWithTag("map-sheet-profile").performScrollTo().performClick()
        compose.waitForIdle()
        org.junit.Assert.assertEquals(unpinnedId, openedProfileUuid)
    }
}
