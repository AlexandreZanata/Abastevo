package com.anpfuel.app.ui.map

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.domain.map.saved.SavedTrip
import com.anpfuel.domain.routing.RoadAvoidance
import com.anpfuel.domain.routing.RoutePoint
import com.anpfuel.maps.engine.MapHost
import com.anpfuel.maps.engine.MapOverlayLabels
import com.anpfuel.maps.engine.MapOverlayList
import com.anpfuel.maps.engine.MapOverlayMarker

@Composable
private fun RealMapHost(
    modifier: Modifier = Modifier,
    onViewportChanged: ((latitude: Double, longitude: Double, radiusMeters: Int) -> Unit)? = null,
    onReady: ((Boolean) -> Unit)? = null,
) {
    MapHost(
        modifier = modifier,
        recenterDescription = stringResource(R.string.map_recenter),
        initErrorMessage = stringResource(R.string.map_unavailable),
        retryLabel = stringResource(R.string.map_retry),
        locationDeniedMessage = stringResource(R.string.map_host_location_denied),
        gpsOffMessage = stringResource(R.string.map_host_gps_off),
        waitingForFixMessage = stringResource(R.string.map_host_waiting_fix),
        onViewportChanged = onViewportChanged,
        onReady = onReady,
        recenterBottomInset = 152.dp,
    )
}

/**
 * MAP-17 shell + MAP-18 markers + MAP-19 sheet + MAP-20 discovery/filters:
 * fourth tab with real native host, source selector, city selector, cluster
 * expansion, accessible marker list, and functional entries. Pins come from
 * canonical UUIDs with REVIEWED coordinates; unknown/city-centroid and
 * coordinate-less stations stay list-only and remain selectable by UUID.
 */
@Composable
fun MapScreen(
    onOpenStation: (stationKey: String, fuel: String) -> Unit,
    onOpenSearch: () -> Unit,
    onOpenDownloads: () -> Unit,
    onOpenSaved: () -> Unit = {},
    onReportStation: (stationId: String, fuel: String) -> Unit = { _, _ -> },
    modifier: Modifier = Modifier,
    initialPlan: SavedTrip? = null,
    onOpenCity: (() -> Unit)? = null,
    mapReady: Boolean = true,
    onMapReady: (Boolean) -> Unit = {},
    periodContent: @Composable (() -> Unit) -> Unit = {},
    viewModel: MapScreenViewModel = hiltViewModel(),
    discovery: MapDiscoveryViewModel? = null,
    sheet: MapSheetViewModel? = null,
    planner: NativeRouteViewModel? = null,
    saved: SavedViewModel? = null,
    guidance: GuidanceViewModel = hiltViewModel(),
    mapContent: @Composable (Modifier) -> Unit = {
        RealMapHost(
            modifier = it,
            onViewportChanged = { lat, lon, radius -> discovery?.onViewport(lat, lon, radius) },
            onReady = onMapReady,
        )
    },
) {
    val ui by viewModel.ui.collectAsStateWithLifecycle()
    val discUi = discovery?.ui?.collectAsStateWithLifecycle()?.value
    var showRoute by remember { mutableStateOf(false) }
    androidx.compose.runtime.LaunchedEffect(showRoute) {
        if (showRoute) guidance.refreshResume()
    }
    androidx.compose.runtime.LaunchedEffect(initialPlan?.id, planner) {
        val plan = initialPlan ?: return@LaunchedEffect
        val activePlanner = planner ?: return@LaunchedEffect
        activePlanner.restorePlan(
            plan.points.map { point ->
                RoutePoint(point.point.latitude, point.point.longitude, plan.name)
            },
            plan.avoid.mapNotNull { name -> RoadAvoidance.entries.firstOrNull { it.name == name } }.toSet(),
        )
        showRoute = true
    }
    val savedUi = saved?.ui?.collectAsStateWithLifecycle()?.value
    if (showRoute && planner != null) NativeRouteSheet(
        planner,
        onDismiss = { showRoute = false },
        onDownloads = { showRoute = false; onOpenDownloads() },
        onSavePlan = saved?.let { vm -> { name, points, avoid -> vm.saveRoutePlan(name, points, avoid) } },
        savedMessage = savedUi?.message,
    ) { preview, request ->
        GuidanceControls(preview = preview, request = request, viewModel = guidance)
    }
    var showFilters by remember { mutableStateOf(false) }
    var showFilterLocation by remember { mutableStateOf(false) }
    var showList by remember { mutableStateOf(false) }
    var routeError by remember(discUi?.selectedUuid) { mutableStateOf(false) }

    val overlayLabels = MapOverlayLabels(
        fuel = stringResource(R.string.map_filter_fuel),
        charging = stringResource(R.string.map_filter_charging),
        fuelAndCharging = stringResource(R.string.map_filter_fuel_charging),
        reviewedLocation = stringResource(R.string.map_location_reviewed),
        cityCentroidLocation = stringResource(R.string.map_location_city_centroid),
        unknownLocation = stringResource(R.string.map_location_unknown),
    )
    LaunchedEffect(discovery, overlayLabels) {
        discovery?.setOverlayLabels(overlayLabels)
    }

    val lifecycleOwner = androidx.compose.ui.platform.LocalLifecycleOwner.current
    androidx.compose.runtime.DisposableEffect(lifecycleOwner, viewModel) {
        val observer = androidx.lifecycle.LifecycleEventObserver { _, event ->
            if (event == androidx.lifecycle.Lifecycle.Event.ON_RESUME) {
                viewModel.refreshAvailability()
                discovery?.refreshPreferredMunicipality()
                saved?.refresh()
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }

    // Initial viewport loads markers once when discovery has not fetched yet.
    LaunchedEffect(discovery) {
        if (discovery != null && discovery.ui.value.generation == 0L) {
            val initial = discovery.ui.value
            discovery.onViewport(
                initial.viewportLatitude,
                initial.viewportLongitude,
                initial.viewportRadiusMeters,
            )
        }
    }

    // Selection and source/product/period/connector switches reload the price sheet (MAP-19).
    LaunchedEffect(
        discUi?.selectedUuid,
        discUi?.selectedMarker,
        discUi?.connector,
        ui.source,
        ui.selectedFuelProduct,
        ui.selectedSurveyPeriod,
    ) {
        val sel = discUi?.selectedUuid
        val marker: MapOverlayMarker? = if (sel != null) {
            discUi.selectedMarker?.takeIf { it.uuid == sel }
                ?: discUi.selectableByUuid[sel]
                ?: discUi.clusters.flatMap { it.markers }.firstOrNull { it.uuid == sel }
                ?: discUi.selectedMembers.firstOrNull { it.uuid == sel }
        } else if (discovery == null) {
            sheet?.ui?.value?.marker
        } else {
            null
        }
        if (marker != null) {
            sheet?.select(
                marker = marker,
                source = ui.source,
                product = ui.selectedFuelProduct,
                surveyPeriod = ui.selectedSurveyPeriod,
                chargingConnector = discUi?.connector,
            )
        }
    }


    Box(modifier.fillMaxSize()) {
        mapContent(Modifier.fillMaxSize())

        if (mapReady) {
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .statusBarsPadding(),
            ) {
                MapControlsCard(
                    ui = ui,
                    municipalityName = discUi?.municipalityName,
                    cityLocationUnknown = discUi?.cityLocationUnknown == true,
                    offlineCached = discUi?.offlineCached == true,
                    onOpenCity = { onOpenCity?.invoke() ?: run { showFilters = true } },
                    onSelectSource = viewModel::selectSource,
                    onSearch = { query -> discovery?.onSearch(discUi?.municipalityCode ?: "4106902", query) },
                    onClearSearch = {
                        if (!discUi?.searchQuery.isNullOrEmpty()) {
                            discovery?.onSearch(discUi?.municipalityCode ?: "4106902", "")
                        }
                    },
                    onRetry = viewModel::retry,
                    onOpenDownloads = onOpenDownloads,
                    onOpenRoute = if (planner != null) { { showRoute = true } } else null,
                    periodContent = { periodContent { viewModel.refreshAvailability() } },
                    modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
                )

                Spacer(Modifier.weight(1f))

                Surface(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 8.dp, vertical = 8.dp),
                    shape = androidx.compose.foundation.shape.RoundedCornerShape(20.dp),
                    color = MaterialTheme.colorScheme.surface.copy(alpha = 0.96f),
                    tonalElevation = 2.dp,
                ) {
                    Column(modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp)) {
                        Text(
                            stringResource(R.string.map_attribution),
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(horizontal = 4.dp, vertical = 2.dp),
                        )
                // B-BR-MAP-CLUSTER-01: Accessible cluster expansion controls and member chooser.
                val multiClusters = discUi?.clusters?.filter { it.isCluster }.orEmpty()
                if (multiClusters.isNotEmpty()) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .horizontalScroll(rememberScrollState())
                            .padding(horizontal = 8.dp, vertical = 4.dp),
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        multiClusters.forEach { cluster ->
                            FilterChip(
                                selected = discUi?.expandedClusterId == cluster.id,
                                onClick = { discovery?.onClusterExpanded(cluster.id) },
                                label = { Text(stringResource(R.string.map_cluster_expand, cluster.count)) },
                                modifier = Modifier
                                    .heightIn(min = 48.dp)
                                    .testTag("map-cluster-expand-${cluster.id}"),
                            )
                        }
                    }
                }

                if (discUi != null && discUi.expandedClusterId != null && discUi.selectedMembers.size > 1) {
                    Surface(
                        tonalElevation = 2.dp,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 8.dp, vertical = 4.dp)
                            .testTag("map-cluster-chooser"),
                    ) {
                        Column(modifier = Modifier.padding(8.dp)) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Text(
                                    stringResource(R.string.map_cluster_title, discUi.selectedMembers.size),
                                    style = MaterialTheme.typography.titleSmall,
                                )
                                TextButton(
                                    onClick = { discovery?.dismissClusterExpansion() },
                                    modifier = Modifier
                                        .heightIn(min = 48.dp)
                                        .testTag("map-cluster-dismiss"),
                                ) {
                                    Text(stringResource(R.string.action_dismiss))
                                }
                            }
                            discUi.selectedMembers.forEach { member ->
                                TextButton(
                                    onClick = { discovery?.onSelection(member.uuid) },
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .heightIn(min = 48.dp)
                                        .testTag("map-cluster-member-${member.uuid}"),
                                ) {
                                    Text(member.title)
                                }
                            }
                        }
                    }
                }

                // MAP-18 accessible equivalent: real stations with distinct fuel/
                // electric pictograms, 48dp targets and TalkBack descriptions.
                if (discUi != null && discUi.listItems.isNotEmpty()) {
                    TextButton(
                        onClick = { showList = !showList },
                        modifier = Modifier
                            .fillMaxWidth()
                            .heightIn(min = 48.dp)
                            .testTag("map-list-toggle"),
                    ) {
                        Text(
                            if (showList) {
                                stringResource(R.string.action_dismiss)
                            } else {
                                stringResource(R.string.map_list_toggle, discUi.listItems.size)
                            }
                        )
                    }
                    if (showList) {
                        MapOverlayList(
                            items = discUi.listItems,
                            onSelect = { discovery?.onSelection(it) },
                            modifier = Modifier.fillMaxWidth().heightIn(max = 240.dp).testTag("map-overlay-list"),
                        )
                    }
                }

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    TextButton(
                        onClick = { showFilters = true },
                        modifier = Modifier.weight(1f).heightIn(min = 48.dp).testTag("map-open-filters"),
                    ) {
                        Text(
                            stringResource(R.string.map_filters),
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            style = MaterialTheme.typography.labelMedium,
                        )
                    }
                    TextButton(
                        onClick = onOpenSearch,
                        modifier = Modifier.weight(1f).heightIn(min = 48.dp).testTag("map-open-search"),
                    ) {
                        Text(stringResource(R.string.map_search))
                    }
                    TextButton(
                        onClick = onOpenDownloads,
                        modifier = Modifier.weight(1f).heightIn(min = 48.dp).testTag("map-open-downloads"),
                    ) {
                        Text(
                            stringResource(R.string.map_download),
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            style = MaterialTheme.typography.labelMedium,
                        )
                    }
                    if (saved != null) TextButton(
                        onClick = onOpenSaved,
                        modifier = Modifier.weight(1f).heightIn(min = 48.dp).testTag("map-open-saved"),
                    ) {
                        Text(
                            stringResource(R.string.map25_open_saved),
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            style = MaterialTheme.typography.labelMedium,
                        )
                    }
                }
                    }
                }
            }
        } else {
            MapScreenSkeleton()
        }
    }

    if (showFilters) {
        MapFiltersDialog(
            source = ui.source,
            showFuel = discUi?.showFuel ?: true,
            showCharging = discUi?.showCharging ?: true,
            onSelectSource = viewModel::selectSource,
            onServiceFilter = { f, c -> discovery?.onServiceFilter(f, c) },
            onDismiss = { showFilters = false },
            selectedFuelProduct = ui.selectedFuelProduct,
            selectedConnector = discUi?.connector,
            selectedCurrent = discUi?.current,
            selectedAccess = discUi?.access,
            selectedMinPowerKw = discUi?.minPowerKw,
            cityLabel = discUi?.municipalityName.orEmpty(),
            onOpenCity = { showFilterLocation = true },
            onApplyExtendedFilters = { f, c, product, conn, cur, acc, power ->
                viewModel.selectFuelProduct(product)
                discovery?.onApplyFilters(
                    showFuel = f,
                    showCharging = c,
                    fuelProduct = product,
                    connector = conn,
                    current = cur,
                    access = acc,
                    minPowerKw = power,
                    surveyPeriod = ui.selectedSurveyPeriod,
                )
            },
        )
    }

    if (showFilterLocation) {
        // Reuse the actual location page/catalog while keeping filter drafts composed.
        val locationPicker: com.anpfuel.app.ui.location.LocationPickerViewModel =
            hiltViewModel(key = "map-filter-location")
        LaunchedEffect(locationPicker) { locationPicker.loadStates() }
        androidx.compose.ui.window.Dialog(
            onDismissRequest = { showFilterLocation = false },
            properties = androidx.compose.ui.window.DialogProperties(usePlatformDefaultWidth = false),
        ) {
            com.anpfuel.app.ui.location.LocationPickerScreen(
                onNavigateBack = { showFilterLocation = false },
                onNavigateToHome = {
                    discovery?.refreshPreferredMunicipality()
                    showFilterLocation = false
                },
                viewModel = locationPicker,
            )
        }
    }

    // MAP-19 sheet with source truth and real actions.
    val sheetUi = sheet?.ui?.collectAsStateWithLifecycle()?.value
    if (sheet != null && (discUi?.selectedUuid != null || sheetUi?.marker != null)) {
        val context = androidx.compose.ui.platform.LocalContext.current
        val activeMarker = sheetUi?.marker
            ?: discUi?.selectedMarker
            ?: discUi?.selectedUuid?.let { discUi.selectableByUuid[it] }
            ?: discUi?.clusters?.flatMap { it.markers }?.firstOrNull { it.uuid == discUi.selectedUuid }
            ?: discUi?.selectedMembers?.firstOrNull { it.uuid == discUi.selectedUuid }
        val targetFuel = if (activeMarker != null && !activeMarker.fuel && activeMarker.charging) {
            "CHARGING"
        } else {
            ui.selectedFuelProduct.name
        }
        MapBottomSheet(
            viewModel = sheet,
            onOpenProfile = { uuid -> onOpenStation(uuid, targetFuel) },
            onRoute = { uuid ->
                val marker = activeMarker?.takeIf { it.uuid == uuid }
                    ?: discUi?.selectableByUuid?.get(uuid)
                if (marker != null && marker.canRouteByCoordinates && planner != null) {
                    planner.point(RouteInputSlot.DESTINATION, com.anpfuel.domain.routing.RoutePoint(marker.latitude,marker.longitude,marker.title))
                    discovery?.clearSelection(); sheet.clear();routeError=false;showRoute=true
                } else { routeError=true }
            },
            onReport = { uuid -> onReportStation(uuid, targetFuel) },
            isFavorite = activeMarker?.uuid?.let { it in (savedUi?.favoriteStationIds ?: emptySet()) } ?: false,
            onToggleFavorite = if (saved != null && activeMarker != null) ({ saved.toggleFavorite(activeMarker) }) else null,
            onSwitchToCommunity = {
                viewModel.selectSource(MapPriceSource.COMMUNITY)
                if (discUi?.selectedUuid == null && activeMarker != null) {
                    sheet.select(
                        marker = activeMarker,
                        source = MapPriceSource.COMMUNITY,
                        product = ui.selectedFuelProduct,
                        surveyPeriod = ui.selectedSurveyPeriod,
                        chargingConnector = discUi?.connector,
                    )
                }
            },
            onDismiss = {
                routeError = false
                discovery?.clearSelection()
                sheet.clear()
            },
            routeError = routeError,
            onSelectSource = { viewModel.selectSource(it) },
        )
    }
}
