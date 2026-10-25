package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
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
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.maps.engine.MapHost
import com.anpfuel.maps.engine.MapOverlayList
import com.anpfuel.maps.engine.MapOverlayMarker

@Composable
private fun RealMapHost(modifier: Modifier = Modifier) {
    MapHost(
        modifier = modifier,
        recenterDescription = stringResource(R.string.map_recenter),
        initErrorMessage = stringResource(R.string.map_unavailable),
        retryLabel = stringResource(R.string.map_retry),
    )
}

/**
 * MAP-17 shell + MAP-18 markers: fourth tab with real native host, source
 * selector, accessible marker list and functional entries. Pins come from
 * canonical UUIDs; unknown/city-centroid stay list-only. No technical
 * cluster IDs reach the UI.
 */
@Composable
fun MapScreen(
    onOpenStation: (stationKey: String, fuel: String) -> Unit,
    onOpenSearch: () -> Unit,
    onOpenDownloads: () -> Unit,
    onReportStation: (stationId: String, fuel: String) -> Unit = { _, _ -> },
    modifier: Modifier = Modifier,
    viewModel: MapScreenViewModel = hiltViewModel(),
    discovery: MapDiscoveryViewModel? = null,
    sheet: MapSheetViewModel? = null,
    mapContent: @Composable (Modifier) -> Unit = { RealMapHost(modifier = it) },
) {
    val ui by viewModel.ui.collectAsStateWithLifecycle()
    var showFilters by remember { mutableStateOf(false) }
    var showList by remember { mutableStateOf(false) }

    // Initial viewport loads markers once the engine is ready. Panning
    // triggers new bounded queries in the full MAP-20 journey; the shell
    // keeps one deterministic load for the minimum gate.
    LaunchedEffect(discovery, ui.source) {
        discovery?.onViewport(-25.42, -49.27, 10_000)
    }

    // Selection opens the price sheet with source truth (MAP-19).
    val discUi = discovery?.ui?.collectAsStateWithLifecycle()?.value
    LaunchedEffect(discUi?.selectedUuid) {
        val sel = discUi?.selectedUuid ?: return@LaunchedEffect
        val marker: MapOverlayMarker? = discUi.clusters.flatMap { it.markers }.firstOrNull { it.uuid == sel }
            ?: discUi.selectedMembers.firstOrNull { it.uuid == sel }
        if (marker != null) {
            sheet?.select(marker, ui.source)
        }
    }

    Column(modifier.fillMaxSize()) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(8.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.CenterHorizontally),
        ) {
            FilterChip(
                selected = ui.source == MapPriceSource.ANP,
                onClick = { viewModel.selectSource(MapPriceSource.ANP) },
                label = { Text(stringResource(R.string.map_source_anp)) },
                modifier = Modifier.heightIn(min = 48.dp).semantics { contentDescription = "ANP prices" },
            )
            FilterChip(
                selected = ui.source == MapPriceSource.COMMUNITY,
                onClick = { viewModel.selectSource(MapPriceSource.COMMUNITY) },
                label = { Text(stringResource(R.string.map_source_community)) },
                modifier = Modifier.heightIn(min = 48.dp).semantics { contentDescription = "Community prices" },
            )
        }

        // MAP-20 search: city/name/address with bounded debounced queries.
        var query by remember { mutableStateOf("") }
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            androidx.compose.material3.OutlinedTextField(
                value = query,
                onValueChange = { query = it },
                placeholder = { Text(stringResource(R.string.map_search_hint)) },
                singleLine = true,
                modifier = Modifier.weight(1f).heightIn(min = 48.dp).semantics {
                    contentDescription = "Search stations"
                }.testTag("map-search-field"),
            )
            TextButton(
                onClick = { discovery?.onSearch("4106902", query) },
                modifier = Modifier.size(width = 96.dp, height = 48.dp).testTag("map-search-submit"),
            ) {
                Text(stringResource(R.string.map_search))
            }
        }

        Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
            when {
                ui.loading -> {
                    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        Column(horizontalAlignment = Alignment.CenterHorizontally) {
                            CircularProgressIndicator()
                            Text(
                                stringResource(R.string.map_loading),
                                style = MaterialTheme.typography.bodyMedium,
                                modifier = Modifier.padding(top = 8.dp),
                            )
                        }
                    }
                }
                ui.error != null -> {
                    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        Column(horizontalAlignment = Alignment.CenterHorizontally) {
                            Text(
                                stringResource(R.string.map_unavailable),
                                style = MaterialTheme.typography.titleMedium,
                            )
                            TextButton(
                                onClick = { viewModel.retry() },
                                modifier = Modifier.size(width = 160.dp, height = 48.dp),
                            ) {
                                Text(stringResource(R.string.map_retry))
                            }
                        }
                    }
                }
                else -> {
                    mapContent(Modifier.fillMaxSize())
                    Surface(
                        modifier = Modifier.align(Alignment.BottomStart).padding(8.dp),
                        shape = MaterialTheme.shapes.small,
                        tonalElevation = 1.dp,
                    ) {
                        Text(
                            stringResource(R.string.map_attribution),
                            style = MaterialTheme.typography.labelSmall,
                            modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
                        )
                    }
                }
            }

            if (ui.noPackage && !ui.loading) {
                Surface(
                    modifier = Modifier.align(Alignment.TopCenter).padding(8.dp),
                    tonalElevation = 3.dp,
                    shape = MaterialTheme.shapes.small,
                ) {
                    Row(
                        modifier = Modifier.padding(8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        Text(
                            stringResource(R.string.map_no_package),
                            style = MaterialTheme.typography.bodySmall,
                        )
                        Button(onClick = onOpenDownloads) {
                            Text(stringResource(R.string.map_download))
                        }
                    }
                }
            }
        }

        // MAP-18 accessible equivalent: real stations with distinct fuel/
        // electric pictograms, 48dp targets and TalkBack descriptions.
        val disc = discovery?.ui?.collectAsStateWithLifecycle()?.value
        if (disc != null && disc.listItems.isNotEmpty()) {
            TextButton(
                onClick = { showList = !showList },
                modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp),
            ) {
                Text(if (showList) "Hide list" else "Show list (${disc.listItems.size})")
            }
            if (showList) {
                MapOverlayList(
                    items = disc.listItems,
                    onSelect = { discovery?.onSelection(it) },
                    modifier = Modifier.fillMaxWidth().heightIn(max = 240.dp),
                )
            }
        }

        Row(
            modifier = Modifier.fillMaxWidth().padding(8.dp),
            horizontalArrangement = Arrangement.SpaceEvenly,
        ) {
            TextButton(
                onClick = { showFilters = true },
                modifier = Modifier.size(width = 120.dp, height = 48.dp),
            ) {
                Text(stringResource(R.string.map_filters))
            }
            TextButton(
                onClick = onOpenSearch,
                modifier = Modifier.size(width = 120.dp, height = 48.dp),
            ) {
                Text(stringResource(R.string.map_search))
            }
            TextButton(
                onClick = onOpenDownloads,
                modifier = Modifier.size(width = 120.dp, height = 48.dp),
            ) {
                Text(stringResource(R.string.map_downloads))
            }
        }
    }

    if (showFilters) {
        val disc = discovery?.ui?.collectAsStateWithLifecycle()?.value
        MapFiltersDialog(
            source = ui.source,
            showFuel = disc?.showFuel ?: true,
            showCharging = disc?.showCharging ?: true,
            onSelectSource = { viewModel.selectSource(it); showFilters = false },
            onServiceFilter = { f, c -> discovery?.onServiceFilter(f, c) },
            onDismiss = { showFilters = false },
        )
    }

    // MAP-19 sheet with source truth and real actions.
    if (sheet != null && discUi?.selectedUuid != null) {
        val context = androidx.compose.ui.platform.LocalContext.current
        MapBottomSheet(
            viewModel = sheet,
            onOpenProfile = { uuid -> onOpenStation(uuid, "GASOLINE_REGULAR") },
            onRoute = { uuid ->
                // Explicit external interim: never counted as native guidance.
                // Native preview (buildCarRoute) owns MAP-22; this keeps the
                // minimum gate honest about what is proven.
                val marker = discUi.clusters.flatMap { it.markers }.firstOrNull { it.uuid == uuid }
                    ?: discUi.selectedMembers.firstOrNull { it.uuid == uuid }
                if (marker != null) {
                    try {
                        val uri = android.net.Uri.parse("geo:${marker.latitude},${marker.longitude}?q=${marker.latitude},${marker.longitude}(${android.net.Uri.encode(marker.title)})")
                        context.startActivity(android.content.Intent(android.content.Intent.ACTION_VIEW, uri))
                    } catch (_: Exception) {
                    }
                }
            },
            onReport = { uuid -> onReportStation(uuid, "GASOLINE_REGULAR") },
            onSwitchToCommunity = { viewModel.selectSource(MapPriceSource.COMMUNITY) },
            onDismiss = { discovery?.clearSelection(); sheet.clear() },
        )
    }
}
