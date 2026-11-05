package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.size
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.getUnclippedBoundsInRoot
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.junit4.StateRestorationTester
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
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
                onOpenDownloads = {},
                viewModel = MapScreenViewModel(emptyStore()),
                mapContent = { Box(Modifier.size(10.dp)) },
            )
        }
        compose.waitForIdle()
        // Top ANP/Community selector.
        compose.onNodeWithText("ANP", ignoreCase = true).assertIsDisplayed()
        // Only the two compact bottom actions remain; search stays in the header.
        compose.onNodeWithTag("map-list-toggle").assertDoesNotExist()
        compose.onNodeWithTag("map-open-search").assertDoesNotExist()
        compose.onNodeWithTag("map-open-saved").assertDoesNotExist()
        compose.onNodeWithText("Filters", ignoreCase = true).assertIsDisplayed()
        compose.onNodeWithTag("map-search-field", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithTag("map-search-field", useUnmergedTree = true).performTextInput("Station")
        compose.onNodeWithTag("map-search-submit", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithTag("map-search-clear", useUnmergedTree = true).assertIsDisplayed().performClick()
        compose.onNodeWithTag("map-search-clear", useUnmergedTree = true).assertDoesNotExist()
        compose.onNodeWithText("Station", substring = false).assertDoesNotExist()
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
                SheetPrice("CCS2", "kWh", "2.45", MapPriceSource.COMMUNITY, java.time.LocalDate.parse("2026-10-25"))
            } else {
                null
            }
        })
        var reportedProduct: String? = null
        compose.setContent {
            MapScreen(
                guidance = testGuidance(),
                onOpenStation = { _, _ -> },
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
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        compose.onNodeWithTag("map-sheet-source-label")
            .assert(hasText(context.getString(R.string.charging_open_sources)))
        compose.onNodeWithText(
            context.getString(R.string.map_sheet_source_label, context.getString(R.string.map_source_anp)),
        ).assertDoesNotExist()
        // In ANP mode, charging shows anp-no-tariff message and "Show Community" button.
        compose.onNodeWithTag("map-sheet-switch-community").assertIsDisplayed()
        compose.onNodeWithTag("map-sheet-switch-community").performClick()
        compose.waitUntil(5_000) { sheetVm.ui.value.price?.source == MapPriceSource.COMMUNITY }
        compose.waitForIdle()
        compose.onNodeWithText("CCS2").assertIsDisplayed()
        compose.onNodeWithTag("map-sheet-price").assert(hasText("2.45 / kWh")).assertIsDisplayed()

        // Clicking top ANP chip switches back to ANP and shows Show Community button again.
        compose.onNodeWithTag("map-sheet-source-anp").performClick()
        compose.waitUntil(5_000) { sheetVm.ui.value.absentReason == "anp-no-tariff" }
        compose.onNodeWithTag("map-sheet-switch-community").assertIsDisplayed()

        // Clicking top Community chip reloads Community price on the same UUID.
        compose.onNodeWithTag("map-sheet-source-community").performClick()
        compose.waitUntil(5_000) { sheetVm.ui.value.price?.source == MapPriceSource.COMMUNITY }
        compose.waitForIdle()
        compose.onNodeWithText("CCS2").assertIsDisplayed()
        compose.onNodeWithTag("map-sheet-price").assert(hasText("2.45 / kWh")).assertIsDisplayed()

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
                searchQuery = "",
                onSearchQueryChanged = {},
                onSearch = {},
                searching = false,
                searchStatus = null,
                onRetry = {},
                onOpenDownloads = {},
                onOpenRoute = null,
                periodContent = {},
                modifier = Modifier.width(320.dp),
            )
        }
        val label = compose.onNodeWithTag("map-city-label", useUnmergedTree = true)
        val cardLeft = compose.onNodeWithTag("map-controls-card").getUnclippedBoundsInRoot().left
        val chipLeft = compose.onNodeWithTag("map-city-chip").getUnclippedBoundsInRoot().left
        // Check the chip alignment separately from its location icon and padding.
        org.junit.Assert.assertTrue(chipLeft - cardLeft <= 16.dp)
        org.junit.Assert.assertTrue(label.getUnclippedBoundsInRoot().left - chipLeft <= 44.dp)
        val longCity = "Vila Bela da Santíssima Trindade — a very long municipality name"
        compose.runOnIdle { city.value = longCity }
        val layouts = mutableListOf<androidx.compose.ui.text.TextLayoutResult>()
        label.performSemanticsAction(androidx.compose.ui.semantics.SemanticsActions.GetTextLayoutResult) { action -> action(layouts) }
        org.junit.Assert.assertEquals(1, layouts.single().lineCount)
        org.junit.Assert.assertTrue(layouts.single().isLineEllipsized(0))
        label.assert(hasText(longCity))
        compose.onNodeWithTag("map-controls-toggle").assertIsDisplayed()
    }

    @Test fun attributionIsSeparateAndStandardActionCardsWorkInBothThemesAndLargeText() {
        val presentation = androidx.compose.runtime.mutableStateOf(false to 1f)
        val calls = mutableListOf<String>()
        compose.setContent {
            val baseDensity = androidx.compose.ui.platform.LocalDensity.current
            androidx.compose.runtime.CompositionLocalProvider(
                androidx.compose.ui.platform.LocalDensity provides androidx.compose.ui.unit.Density(baseDensity.density, presentation.value.second),
            ) {
                com.anpfuel.app.ui.theme.AnpFuelTheme(darkTheme = presentation.value.first, dynamicColor = false) {
                    MapBottomControls(
                        onFilters = { calls += "filters" },
                        onDownloads = { calls += "downloads" },
                        modifier = Modifier.width(360.dp),
                    )
                }
            }
        }
        listOf(false to 1f, true to 1f, true to 2f).forEachIndexed { index, mode ->
            compose.runOnIdle { presentation.value = mode }
            val attribution = compose.onNodeWithTag("map-attribution").assertIsDisplayed().getUnclippedBoundsInRoot()
            val panel = compose.onNodeWithTag("map-action-panel").getUnclippedBoundsInRoot()
            org.junit.Assert.assertTrue(attribution.bottom <= panel.top)
            compose.onNode(hasTestTag("map-attribution") and hasAnyAncestor(hasTestTag("map-action-panel"))).assertDoesNotExist()
            listOf("filters", "downloads").forEach { action ->
                compose.onNodeWithTag("map-open-search").assertDoesNotExist()
                compose.onNodeWithTag("map-open-saved").assertDoesNotExist()
                val node = compose.onNodeWithTag("map-open-$action").performScrollTo().assertIsDisplayed()
                val bounds = node.getUnclippedBoundsInRoot()
                org.junit.Assert.assertTrue(bounds.bottom - bounds.top >= 48.dp)
                node.performClick()
                compose.onNodeWithTag("map-attribution").assertIsDisplayed()
            }
            org.junit.Assert.assertEquals(listOf("filters", "downloads"), calls)
            calls.clear()
            compose.onNodeWithTag("map-open-filters").performScrollTo()
            val context = androidx.test.platform.app.InstrumentationRegistry.getInstrumentation().targetContext
            val directory = java.io.File(context.getExternalFilesDir(null), "map-panel-ui-20261026").apply { mkdirs() }
            java.io.FileOutputStream(java.io.File(directory, "panel-$index.png")).use { output ->
                compose.onNodeWithTag("map-bottom-overlay").captureToImage().asAndroidBitmap()
                    .compress(android.graphics.Bitmap.CompressFormat.PNG, 100, output)
            }
        }
    }

    @Test fun recenterInsetTracksMeasuredBottomPanelWhenFontSizeChanges() {
        val fontScale = androidx.compose.runtime.mutableStateOf(1f)
        var measuredInset = 0.dp
        compose.setContent {
            val baseDensity = androidx.compose.ui.platform.LocalDensity.current
            androidx.compose.runtime.CompositionLocalProvider(
                androidx.compose.ui.platform.LocalDensity provides androidx.compose.ui.unit.Density(baseDensity.density, fontScale.value),
            ) {
                MapScreen(
                    modifier = Modifier.testTag("tested-map-screen"),
                    guidance = testGuidance(),
                    onOpenStation = { _, _ -> },
                    onOpenDownloads = {},
                    onBottomOverlayInsetChanged = { measuredInset = it },
                    viewModel = MapScreenViewModel(emptyStore()),
                    mapContent = { Box(Modifier.size(10.dp)) },
                )
            }
        }
        compose.waitForIdle()
        val normalInset = measuredInset
        val panel = compose.onNodeWithTag("map-action-panel").getUnclippedBoundsInRoot()
        // Recenter shares the attribution row on the opposite side, above the panel.
        // Bounds and onSizeChanged convert pixels independently; allow subpixel rounding.
        org.junit.Assert.assertEquals((panel.bottom - panel.top).value + 10f, normalInset.value, 0.01f)
        compose.onNodeWithTag("map-actions-toggle").performClick()
        compose.waitForIdle()
        org.junit.Assert.assertTrue(measuredInset < normalInset)
        compose.onNodeWithTag("map-attribution").assertIsDisplayed()
        compose.onNodeWithTag("map-actions-toggle").performClick()
        compose.waitForIdle()
        org.junit.Assert.assertEquals(normalInset, measuredInset)
        compose.runOnIdle { fontScale.value = 2f }
        compose.waitForIdle()
        org.junit.Assert.assertTrue(measuredInset > normalInset)
        val root = compose.onNodeWithTag("tested-map-screen").getUnclippedBoundsInRoot()
        val header = compose.onNodeWithTag("map-controls-card").getUnclippedBoundsInRoot()
        val recenterTop = root.bottom - measuredInset - 48.dp
        org.junit.Assert.assertTrue(recenterTop >= header.bottom + 8.dp)
        compose.onNodeWithTag("map-open-downloads").performScrollTo().assertIsDisplayed()
    }

    @Test fun minimalToggleHidesActionsAndRestoresSavedStateWithoutLosingCallbacks() {
        val restoration = StateRestorationTester(compose)
        var downloads = 0
        restoration.setContent {
            com.anpfuel.app.ui.theme.AnpFuelTheme(darkTheme = true, dynamicColor = false) {
                MapBottomControls(
                    onFilters = {},
                    onDownloads = { downloads++ },
                    modifier = Modifier.width(360.dp),
                )
            }
        }
        val context = androidx.test.platform.app.InstrumentationRegistry.getInstrumentation().targetContext
        val expanded = compose.onNodeWithTag("map-bottom-overlay").getUnclippedBoundsInRoot()
        val toggle = compose.onNodeWithTag("map-actions-toggle").assertIsDisplayed()
        val toggleBounds = toggle.getUnclippedBoundsInRoot()
        org.junit.Assert.assertTrue(toggleBounds.right - toggleBounds.left >= 48.dp)
        org.junit.Assert.assertTrue(toggleBounds.bottom - toggleBounds.top >= 48.dp)
        toggle.assert(hasContentDescription(context.getString(com.anpfuel.app.R.string.map_actions_hide))).performClick()
        compose.onNodeWithTag("map-action-panel").assertIsDisplayed()
        listOf("filters", "downloads").forEach { action ->
            compose.onNodeWithTag("map-open-$action").assertDoesNotExist()
        }
        compose.onNodeWithTag("map-attribution").assertIsDisplayed()
        val collapsed = compose.onNodeWithTag("map-bottom-overlay").getUnclippedBoundsInRoot()
        org.junit.Assert.assertTrue(collapsed.bottom - collapsed.top < expanded.bottom - expanded.top)
        restoration.emulateSavedInstanceStateRestore()
        compose.onNodeWithTag("map-action-panel").assertIsDisplayed()
        compose.onNodeWithTag("map-open-downloads").assertDoesNotExist()
        compose.onNodeWithTag("map-attribution").assertIsDisplayed()
        val directory = java.io.File(context.getExternalFilesDir(null), "map-panel-collapse-20261026").apply { mkdirs() }
        java.io.FileOutputStream(java.io.File(directory, "collapsed.png")).use { output ->
            compose.onNodeWithTag("map-bottom-overlay").captureToImage().asAndroidBitmap()
                .compress(android.graphics.Bitmap.CompressFormat.PNG, 100, output)
        }
        compose.onNodeWithTag("map-actions-toggle")
            .assert(hasContentDescription(context.getString(com.anpfuel.app.R.string.map_actions_show))).performClick()
        compose.onNodeWithTag("map-open-downloads").performScrollTo().assertIsDisplayed().performClick()
        org.junit.Assert.assertEquals(1, downloads)
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

    @Test fun discoveryRefreshKeepsPanelStableAndExplicitPickersPreserveStationActions() {
        val aId = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
        val bId = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
        val cId = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
        val unpinnedId = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"

        val nearbyRows = listOf(
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
        var resultCount = nearbyRows.size
        var refreshGate: kotlinx.coroutines.CompletableDeferred<Unit>? = null
        var selectionListener: ((com.anpfuel.maps.engine.MapOverlaySelection) -> Unit)? = null
        val gateway = object : com.anpfuel.domain.repository.ServerStationGateway {
            override suspend fun list(limit: Int, cursor: String?) =
                com.anpfuel.domain.discovery.ServerStationPage(emptyList(), null)
            override suspend fun listInMunicipality(municipalityCode: String, limit: Int, cursor: String?) =
                com.anpfuel.domain.discovery.ServerStationPage(emptyList(), null)
            override suspend fun search(municipalityCode: String, query: String, limit: Int, cursor: String?) =
                com.anpfuel.domain.discovery.ServerStationPage(nearbyRows.map { it.station }, null)
            override suspend fun nearby(lat: Double, lon: Double, radiusMeters: Int, limit: Int): List<com.anpfuel.domain.discovery.NearbyServerStation> {
                refreshGate?.await()
                return nearbyRows.take(resultCount)
            }
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
            override fun setOnOverlaySelected(listener: (com.anpfuel.maps.engine.MapOverlaySelection) -> Unit) {
                selectionListener = listener
            }
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
                onOpenDownloads = {},
                viewModel = MapScreenViewModel(emptyStore()),
                discovery = discoveryVm,
                sheet = sheetVm,
                mapContent = { Box(Modifier.size(10.dp)) },
            )
        }
        compose.waitUntil(5_000) { discoveryVm.ui.value.clusters.size == 1 }
        // Background empty/populated/clustered results cannot move or reopen actions.
        for (expanded in listOf(true, false)) {
            if (!expanded) compose.onNodeWithTag("map-actions-toggle").performClick()
            val before = compose.onNodeWithTag("map-bottom-overlay").getUnclippedBoundsInRoot()
            for (count in listOf(0, 1, 4)) {
                val gate = kotlinx.coroutines.CompletableDeferred<Unit>()
                compose.runOnIdle {
                    resultCount = count
                    refreshGate = gate
                    discoveryVm.onViewport(-25.42, -49.27, 10_000)
                }
                compose.waitUntil(5_000) { discoveryVm.ui.value.loading }
                org.junit.Assert.assertEquals(before, compose.onNodeWithTag("map-bottom-overlay").getUnclippedBoundsInRoot())
                compose.runOnIdle { gate.complete(Unit) }
                compose.waitUntil(5_000) { !discoveryVm.ui.value.loading }
                org.junit.Assert.assertEquals(before, compose.onNodeWithTag("map-bottom-overlay").getUnclippedBoundsInRoot())
                compose.onNodeWithTag("map-list-toggle").assertDoesNotExist()
                compose.onNodeWithTag("map-search-results").assertDoesNotExist()
                compose.onNodeWithTag("map-cluster-chooser").assertDoesNotExist()
                if (expanded) compose.onNodeWithTag("map-open-downloads").assertIsDisplayed()
                else compose.onNodeWithTag("map-open-downloads").assertDoesNotExist()
            }
        }
        compose.onNodeWithTag("map-actions-toggle").performClick()
        compose.runOnIdle {
            selectionListener?.invoke(com.anpfuel.maps.engine.MapOverlaySelection.Cluster(discoveryVm.ui.value.clusters.single().markers))
        }
        compose.waitForIdle()
        compose.onNodeWithTag("map-cluster-chooser").assertIsDisplayed()

        // Select 2nd member (Posto Beta).
        compose.onNodeWithText("Posto Beta", substring = true).performClick()
        compose.waitUntil(5_000) { sheetVm.ui.value.marker?.uuid == bId }
        compose.onNodeWithText("Posto Beta").assertIsDisplayed()
        androidx.test.platform.app.InstrumentationRegistry.getInstrumentation().sendKeyDownUpSync(android.view.KeyEvent.KEYCODE_BACK)
        compose.waitUntil(5_000) { discoveryVm.ui.value.selectedUuid == null }

        // Expand again and select last member (Posto Gamma).
        compose.runOnIdle {
            selectionListener?.invoke(com.anpfuel.maps.engine.MapOverlaySelection.Cluster(discoveryVm.ui.value.clusters.single().markers))
        }
        compose.waitForIdle()
        compose.onNodeWithText("Posto Gamma", substring = true).performClick()
        compose.waitUntil(5_000) { sheetVm.ui.value.marker?.uuid == cId }
        compose.onNodeWithText("Posto Gamma").assertIsDisplayed()
        androidx.test.platform.app.InstrumentationRegistry.getInstrumentation().sendKeyDownUpSync(android.view.KeyEvent.KEYCODE_BACK)
        compose.waitUntil(5_000) { discoveryVm.ui.value.selectedUuid == null }

        // Submit the top search to reach coordinate-less results without a permanent list.
        compose.onNodeWithTag("map-search-field").performTextInput("Posto")
        compose.onNodeWithTag("map-search-submit", useUnmergedTree = true).performClick()
        compose.waitUntil(5_000) { discoveryVm.ui.value.searchResults != null }
        compose.onNodeWithTag("map-picker-list").performScrollToNode(hasText("Posto Sem Coordenada", substring = true))
        compose.onNodeWithText("Posto Sem Coordenada", substring = true).performClick()
        compose.waitUntil(5_000) { sheetVm.ui.value.marker?.uuid == unpinnedId }
        compose.onNodeWithTag("map-sheet-route-unavailable").performScrollTo().assertIsDisplayed()
        compose.onNodeWithTag("map-sheet-profile").performScrollTo().performClick()
        compose.waitForIdle()
        org.junit.Assert.assertEquals(unpinnedId, openedProfileUuid)
    }
}
