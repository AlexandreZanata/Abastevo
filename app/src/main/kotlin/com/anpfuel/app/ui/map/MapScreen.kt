package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
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
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.maps.engine.MapHost

@Composable
private fun RealMapHost(modifier: Modifier = Modifier) {
    MapHost(modifier = modifier)
}

/**
 * MAP-17 minimal map shell: fourth tab content with real native host.
 *
 * Order is Explorar | Comunidade | Mapa | Perfil. The shell owns the
 * ANP/Comunidade selector, filter/search/download entries, loading/error/
 * no-package states and OSM attribution. It never shows a fake map, WebView
 * or external shortcut.
 */
@Composable
fun MapScreen(
    onOpenStation: (stationKey: String, fuel: String) -> Unit,
    onOpenSearch: () -> Unit,
    onOpenDownloads: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: MapScreenViewModel = hiltViewModel(),
    mapContent: @Composable (Modifier) -> Unit = { RealMapHost(modifier = it) },
) {
    val ui by viewModel.ui.collectAsStateWithLifecycle()
    var showFilters by remember { mutableStateOf(false) }

    Column(modifier.fillMaxSize()) {
        // Top ANP/Community selector: permanent, theme-aware, 48dp targets.
        Row(
            modifier = Modifier.fillMaxWidth().padding(8.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.CenterHorizontally),
        ) {
            FilterChip(
                selected = ui.source == MapPriceSource.ANP,
                onClick = { viewModel.selectSource(MapPriceSource.ANP) },
                label = { Text(stringResource(R.string.map_source_anp)) },
                modifier = Modifier.semantics { contentDescription = "ANP prices" },
            )
            FilterChip(
                selected = ui.source == MapPriceSource.COMMUNITY,
                onClick = { viewModel.selectSource(MapPriceSource.COMMUNITY) },
                label = { Text(stringResource(R.string.map_source_community)) },
                modifier = Modifier.semantics { contentDescription = "Community prices" },
            )
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
                    // Real native host; browsing never requires GPS.
                    mapContent(Modifier.fillMaxSize())
                    // OSM attribution stays visible over the map.
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

        // Functional entries: filters dialog, search navigation, downloads navigation.
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
        MapFiltersDialog(
            source = ui.source,
            onSelectSource = { viewModel.selectSource(it); showFilters = false },
            onDismiss = { showFilters = false },
        )
    }
}
