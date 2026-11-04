package com.anpfuel.app.ui.map

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import android.widget.Toast
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.ContextCompat

import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Icon
import androidx.compose.material.icons.automirrored.filled.List
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Layers
import androidx.compose.material.icons.filled.MyLocation
import androidx.compose.material.icons.Icons
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
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
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.locale.AppLocaleHolder
import com.anpfuel.domain.map.saved.SavedTrip
import com.anpfuel.domain.routing.NativeRouteRequest
import com.anpfuel.domain.routing.RoadAvoidance
import com.anpfuel.domain.routing.RoutePoint
import com.anpfuel.maps.engine.MapHost
import com.anpfuel.maps.engine.MapOverlayLabels
import com.anpfuel.maps.engine.MapOverlayList
import com.anpfuel.maps.engine.MapOverlayMarker
import java.util.Locale

@Composable
private fun RealMapHost(
    modifier: Modifier = Modifier,
    onViewportChanged: ((latitude: Double, longitude: Double, radiusMeters: Int) -> Unit)? = null,
    onReady: ((Boolean) -> Unit)? = null,
    recenterBottomInset: Dp = 164.dp,
    darkMode: Boolean = com.anpfuel.app.ui.theme.LocalAnpDarkTheme.current,
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
        recenterBottomInset = recenterBottomInset,
        darkMode = darkMode,
        recenterIcon = { Icon(Icons.Default.MyLocation, contentDescription = null) },
    )
}

/**
 * MAP-17 shell + MAP-18 markers + MAP-19 sheet + MAP-20 discovery/filters:
 * fourth tab with real native host, source selector, city selector, cluster
 * expansion, accessible marker list, and functional entries. Pins come from
 * canonical UUIDs with pin-tier coordinates (reviewed, osm-mapped,
 * anp-reported, address-point); unknown/city-centroid and
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
    onBottomOverlayInsetChanged: (Dp) -> Unit = {},
    mapContent: (@Composable (Modifier) -> Unit)? = null,
    onAddChargingSite: (() -> Unit)? = null,
    routeTarget: MapRouteTarget? = null,
    onRouteTargetConsumed: () -> Unit = {},
) {
    val density = LocalDensity.current
    var bottomOverlayInset by remember { mutableStateOf(164.dp) }
    var sheetHeight by remember { mutableStateOf(0.dp) }
    var controlsHeight by remember { mutableStateOf(0.dp) }
    val ui by viewModel.ui.collectAsStateWithLifecycle()
    val discUi = discovery?.ui?.collectAsStateWithLifecycle()?.value
    val context = LocalContext.current
    val routeLanguage = Locale.forLanguageTag(AppLocaleHolder.localeTag).language
    val currentPositionLabel = stringResource(R.string.map22_current)
    val guidanceUi by guidance.serviceState.collectAsStateWithLifecycle()
    val routeUi = planner?.ui?.collectAsStateWithLifecycle()?.value ?: NativeRouteUi()
    var showRoute by rememberSaveable { mutableStateOf(false) }
    var routeOnMap by rememberSaveable { mutableStateOf(false) }
    var requestOrigin by rememberSaveable { mutableStateOf(false) }
    var destinationQuery by remember { mutableStateOf("") }
    var searchText by rememberSaveable { mutableStateOf("") }
    var pendingStart by remember { mutableStateOf<NativeRouteRequest?>(null) }
    var showFilters by remember { mutableStateOf(false) }
    var showFilterLocation by remember { mutableStateOf(false) }
    var showList by remember { mutableStateOf(false) }
    var routeError by remember(discUi?.selectedUuid) { mutableStateOf(false) }

    fun updateBottomInset(inset: Dp) {
        if (bottomOverlayInset != inset) {
            bottomOverlayInset = inset
            onBottomOverlayInsetChanged(inset)
        }
    }

    fun granted(permission: String) =
        ContextCompat.checkSelfPermission(context, permission) == PackageManager.PERMISSION_GRANTED

    fun buildFromCurrentLocation() {
        planner?.buildFromCurrentLocation(routeLanguage) { planner.currentRoutePoint(context, currentPositionLabel) }
    }

    val originPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) {
        if (routeOnMap && !showRoute) {
            if (granted(Manifest.permission.ACCESS_FINE_LOCATION) || granted(Manifest.permission.ACCESS_COARSE_LOCATION)) {
                buildFromCurrentLocation()
            } else {
                planner?.fail("no-permission")
            }
        }
    }

    fun chooseCurrentOrigin() {
        if (granted(Manifest.permission.ACCESS_FINE_LOCATION) || granted(Manifest.permission.ACCESS_COARSE_LOCATION)) {
            buildFromCurrentLocation()
        } else {
            originPermission.launch(arrayOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION))
        }
    }

    val guidancePermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) {
        val request = pendingStart
        pendingStart = null
        if (request != null && routeOnMap && request == planner?.request(routeLanguage)) {
            when {
                !granted(Manifest.permission.ACCESS_FINE_LOCATION) ->
                    GuidanceServiceState.update { it.copy(error = "no-permission") }
                Build.VERSION.SDK_INT >= 33 && !granted(Manifest.permission.POST_NOTIFICATIONS) ->
                    GuidanceServiceState.update { it.copy(error = "notification-blocked") }
                else -> {
                    guidance.start(request)
                }
            }
        }
    }

    fun startGuidance() {
        val request = planner?.request(routeLanguage) ?: return
        val permissions = buildList {
            if (!granted(Manifest.permission.ACCESS_FINE_LOCATION)) {
                add(Manifest.permission.ACCESS_FINE_LOCATION)
                add(Manifest.permission.ACCESS_COARSE_LOCATION)
            }
            if (Build.VERSION.SDK_INT >= 33 && !granted(Manifest.permission.POST_NOTIFICATIONS)) {
                add(Manifest.permission.POST_NOTIFICATIONS)
            }
        }
        if (permissions.isEmpty()) guidance.start(request)
        else {
            pendingStart = request
            guidancePermission.launch(permissions.toTypedArray())
        }
    }

    fun closeRoute() {
        requestOrigin = false
        pendingStart = null
        if (guidanceUi.active || guidanceUi.starting) guidance.stop()
        else guidance.clearInactiveStatus()
        planner?.cancel()
        routeOnMap = false
    }

    fun openPoint(point: RoutePoint) {
        val activePlanner = planner ?: return
        guidance.clearInactiveStatus()
        activePlanner.prepareDestination(point)
        discovery?.clearSelection()
        sheet?.clear()
        routeError = false
        showList = false
        showRoute = false
        routeOnMap = true
        requestOrigin = true
    }

    LaunchedEffect(requestOrigin, mapReady) {
        if (requestOrigin && mapReady) {
            requestOrigin = false
            chooseCurrentOrigin()
        }
    }

    LaunchedEffect(routeTarget, mapReady, planner) {
        if (routeTarget != null && mapReady && planner != null) {
            onRouteTargetConsumed()
            if (guidanceUi.active || guidanceUi.starting) {
                Toast.makeText(context, context.getString(R.string.map_route_stop_first), Toast.LENGTH_SHORT).show()
            } else when (routeTarget) {
                is MapRouteTarget.Point -> openPoint(routeTarget.point)
                is MapRouteTarget.Address -> {
                    guidance.clearInactiveStatus()
                    planner.prepareDestination(null)
                    discovery?.clearSelection()
                    sheet?.clear()
                    destinationQuery = routeTarget.query
                    routeOnMap = false
                    showRoute = true
                }
            }
        }
    }

    DisposableEffect(planner) { onDispose { planner?.cancelOriginLookup() } }
    BackHandler(enabled = (routeOnMap || guidanceUi.arrived) && !guidanceUi.active && !showRoute) { closeRoute() }

    LaunchedEffect(initialPlan?.id, planner) {
        val plan = initialPlan ?: return@LaunchedEffect
        val activePlanner = planner ?: return@LaunchedEffect
        if (guidanceUi.active || guidanceUi.starting) return@LaunchedEffect
        activePlanner.restorePlan(
            plan.points.map { point -> RoutePoint(point.point.latitude, point.point.longitude, plan.name) },
            plan.avoid.mapNotNull { name -> RoadAvoidance.entries.firstOrNull { it.name == name } }.toSet(),
        )
        showRoute = true
    }

    val savedUi = saved?.ui?.collectAsStateWithLifecycle()?.value
    if (showRoute && planner != null && !guidanceUi.active && !guidanceUi.starting) NativeRouteSheet(
        planner,
        onDismiss = {
            showRoute = false
            destinationQuery = ""
            if (planner.ui.value.preview != null) routeOnMap = true
        },
        onDownloads = { showRoute = false; onOpenDownloads() },
        onSavePlan = saved?.let { vm -> { name, points, avoid -> vm.saveRoutePlan(name, points, avoid) } },
        savedMessage = savedUi?.message,
        initialDestinationQuery = destinationQuery,
        onDestinationPicked = if (destinationQuery.isBlank()) null else ::openPoint,
    )

    val overlayLabels = MapOverlayLabels(
        fuel = stringResource(R.string.map_filter_fuel),
        charging = stringResource(R.string.map_filter_charging),
        fuelAndCharging = stringResource(R.string.map_filter_fuel_charging),
        reviewedLocation = stringResource(R.string.map_location_reviewed),
        osmMappedLocation = stringResource(R.string.map_location_osm_mapped),
        anpReportedLocation = stringResource(R.string.map_location_anp_reported),
        addressPointLocation = stringResource(R.string.map_location_address_point),
        sourceChargingLocation = stringResource(R.string.map_location_charging_source),
        unnamedCharging = stringResource(R.string.charging_place_unnamed),
        confirmedCharging = stringResource(R.string.charging_place_two_sources),
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
                if (!GuidanceServiceState.current().active) discovery?.refreshPreferredMunicipality()
                saved?.refresh()
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }

    // The opened city's coordinate selects the offline-map region to offer.
    LaunchedEffect(discovery, discUi?.cityLatitude, discUi?.cityLongitude) {
        if (discovery != null) viewModel.onCityPoint(discUi?.cityLatitude, discUi?.cityLongitude)
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
        if (mapContent != null) {
            mapContent(Modifier.fillMaxSize())
        } else {
            RealMapHost(
                modifier = Modifier.fillMaxSize(),
                onViewportChanged = { lat, lon, radius -> discovery?.onViewport(lat, lon, radius) },
                onReady = onMapReady,
                recenterBottomInset = bottomOverlayInset,
            )
        }

        val routeVisible = routeOnMap || guidanceUi.active || guidanceUi.starting || guidanceUi.arrived
        if (mapReady && routeVisible) {
            MapRouteOverlay(
                route = routeUi,
                guidance = guidanceUi,
                onCurrentOrigin = ::chooseCurrentOrigin,
                onEdit = {
                    planner?.cancelOriginLookup()
                    destinationQuery = ""
                    showRoute = true
                },
                onDownloads = onOpenDownloads,
                onStart = ::startGuidance,
                onClose = ::closeRoute,
                onInsetChanged = ::updateBottomInset,
            )
        } else if (mapReady) {
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
                    searchQuery = searchText,
                    onSearchQueryChanged = { searchText = it },
                    onSearch = { query ->
                        discovery?.onSearch(discUi?.municipalityCode ?: "4106902", query) { results ->
                            if (searchText.trim() == query.trim()) {
                                val marker = results.singleOrNull()?.takeIf { it.canRouteByCoordinates }
                                if (marker != null && planner != null) {
                                    openPoint(RoutePoint(marker.latitude, marker.longitude, marker.title.take(512)))
                                } else {
                                    showList = results.isNotEmpty()
                                }
                            }
                        }
                    },
                    onClearSearch = {
                        if (!discUi?.searchQuery.isNullOrEmpty()) {
                            discovery?.onSearch(discUi?.municipalityCode ?: "4106902", "")
                        }
                    },
                    onRetry = viewModel::retry,
                    onOpenDownloads = onOpenDownloads,
                    onOpenRoute = if (planner != null) { { destinationQuery = ""; showRoute = true } } else null,
                    periodContent = { periodContent { viewModel.refreshAvailability() } },
                    modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
                )
                ui.regionSuggestion?.let { suggestion ->
                    MapRegionSuggestionCard(
                        suggestion = suggestion,
                        onDownload = { if (viewModel.acceptRegionSuggestion()) onOpenDownloads() },
                        onDismiss = viewModel::dismissRegionSuggestion,
                        modifier = Modifier.padding(horizontal = 12.dp),
                    )
                }

                Spacer(Modifier.weight(1f))
                // The two round controls sit together just above the sheet, beside
                // the attribution: recenter (host) at sheet + gap, layers one gap above.
                val layersBottom = (sheetHeight + MAP_CONTROL_GAP * 2 + 56.dp - controlsHeight).coerceAtLeast(0.dp)
                Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.CenterEnd) {
                    Surface(
                        onClick = { showFilters = true },
                        shape = CircleShape,
                        color = MaterialTheme.colorScheme.surfaceContainerHigh,
                        contentColor = MaterialTheme.colorScheme.primary,
                        shadowElevation = 6.dp,
                        modifier = Modifier.padding(end = 16.dp, bottom = layersBottom).size(56.dp).testTag("map-layers"),
                    ) {
                        Box(contentAlignment = Alignment.Center) {
                            Icon(Icons.Default.Layers, contentDescription = stringResource(R.string.map_filters))
                        }
                    }
                }

                MapBottomControls(
                    onFilters = { showFilters = true },
                    onSearch = onOpenSearch,
                    onDownloads = onOpenDownloads,
                    onSaved = onOpenSaved.takeIf { saved != null },
                    modifier = Modifier.onSizeChanged { size ->
                        controlsHeight = with(density) { size.height.toDp() }
                    },
                    onSheetHeightChanged = { height ->
                        sheetHeight = height
                        updateBottomInset(height + MAP_CONTROL_GAP)
                    },
                ) {
                if (discUi != null && discUi.showCharging && !discUi.loading && discUi.generation > 0) {
                    val message = when {
                        !discUi.chargingSnapshotPresent || discUi.chargingError != null ->
                            R.string.charging_catalog_unavailable
                        discUi.error == null && discUi.selectableByUuid.values.none { it.charging } ->
                            R.string.charging_map_gap
                        else -> R.string.charging_open_sources
                    }
                    Text(
                        text = stringResource(message),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(horizontal = 12.dp, vertical = 4.dp),
                    )
                    if (message == R.string.charging_map_gap && onAddChargingSite != null) {
                        TextButton(onClick = onAddChargingSite, modifier = Modifier.padding(horizontal = 12.dp)) {
                            Text(stringResource(R.string.charging_add_site))
                        }
                    }
                }
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
                        Icon(
                            if (showList) Icons.Default.Close else Icons.AutoMirrored.Filled.List,
                            contentDescription = null,
                            tint = MaterialTheme.colorScheme.primary,
                        )
                        Spacer(Modifier.width(10.dp))
                        Text(
                            if (showList) {
                                stringResource(R.string.action_dismiss)
                            } else {
                                stringResource(R.string.map_list_toggle, discUi.listItems.size)
                            },
                            style = MaterialTheme.typography.titleMedium,
                            color = MaterialTheme.colorScheme.onSurface,
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
                    openPoint(RoutePoint(marker.latitude, marker.longitude, marker.title.take(512)))
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

/** Gap between the bottom sheet and the round map controls, and between the controls. */
private val MAP_CONTROL_GAP = 10.dp
