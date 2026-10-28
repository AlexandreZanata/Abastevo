package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material.icons.filled.LocationOn
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.FuelProductIcon

/** Compact overlay; expanding controls never changes the native map bounds. */
@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun MapControlsCard(
    ui: MapScreenUi,
    municipalityName: String?,
    cityLocationUnknown: Boolean,
    offlineCached: Boolean,
    onOpenCity: () -> Unit,
    onSelectSource: (MapPriceSource) -> Unit,
    onSearch: (String) -> Unit,
    onClearSearch: () -> Unit,
    onRetry: () -> Unit,
    onOpenDownloads: () -> Unit,
    onOpenRoute: (() -> Unit)?,
    periodContent: @Composable () -> Unit,
    modifier: Modifier = Modifier,
) {
    var expanded by rememberSaveable { mutableStateOf(false) }
    var query by rememberSaveable { mutableStateOf("") }
    val focusManager = LocalFocusManager.current
    val submitSearch = { focusManager.clearFocus(); onSearch(query) }
    val sourceLabel = stringResource(if (ui.source == MapPriceSource.ANP) R.string.map_source_anp else R.string.map_source_community)
    Surface(
        modifier = modifier.fillMaxWidth().testTag("map-controls-card"),
        shape = RoundedCornerShape(24.dp),
        color = MaterialTheme.colorScheme.surface.copy(alpha = 0.97f),
        shadowElevation = 3.dp,
    ) {
        Column(Modifier.padding(horizontal = 12.dp, vertical = 6.dp)) {
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                TextButton(
                    onClick = onOpenCity,
                    contentPadding = PaddingValues(horizontal = 4.dp),
                    modifier = Modifier.weight(1f).heightIn(min = 48.dp).testTag("map-city-chip"),
                ) {
                    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                        Icon(Icons.Default.LocationOn, null, Modifier.size(18.dp))
                        Text(
                            municipalityName ?: stringResource(R.string.map_city_label),
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            style = MaterialTheme.typography.titleSmall,
                            modifier = Modifier.weight(1f).padding(start = 4.dp).testTag("map-city-label"),
                        )
                    }
                }
                FuelProductIcon(ui.selectedFuelProduct, size = 20.dp)
                Text(sourceLabel, style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(start = 6.dp))
                IconButton(
                    onClick = { expanded = !expanded },
                    modifier = Modifier.testTag("map-controls-toggle"),
                ) {
                    Icon(
                        if (expanded) Icons.Default.ExpandLess else Icons.Default.ExpandMore,
                        stringResource(if (expanded) R.string.map_controls_collapse else R.string.map_controls_expand),
                    )
                }
            }
            OutlinedTextField(
                value = query,
                onValueChange = { value ->
                    query = value
                    if (value.trim().length < 2) onClearSearch()
                },
                placeholder = { Text(stringResource(R.string.map_search_hint), maxLines = 1, overflow = TextOverflow.Ellipsis) },
                trailingIcon = {
                    IconButton(onClick = submitSearch, modifier = Modifier.testTag("map-search-submit")) {
                        Icon(Icons.Default.Search, stringResource(R.string.map_search))
                    }
                },
                shape = RoundedCornerShape(16.dp),
                singleLine = true,
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                keyboardActions = KeyboardActions(onSearch = { submitSearch() }),
                modifier = Modifier.fillMaxWidth().testTag("map-search-field"),
            )
            if (ui.error != null || (!expanded && (ui.noPackage || offlineCached || cityLocationUnknown))) {
                Text(
                    text = when {
                        ui.error != null -> stringResource(R.string.map_unavailable)
                        offlineCached -> stringResource(R.string.server_station_cached)
                        ui.noPackage -> stringResource(R.string.map_no_package)
                        else -> stringResource(R.string.map_location_unknown)
                    },
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(start = 4.dp, top = 6.dp, bottom = 2.dp),
                )
            }
            if (ui.loading) LinearProgressIndicator(Modifier.fillMaxWidth().padding(top = 4.dp))
            if (expanded) {
                FlowRow(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier.padding(top = 4.dp),
                ) {
                    MapPriceSource.entries.forEach { source ->
                        FilterChip(
                            selected = ui.source == source,
                            onClick = { onSelectSource(source) },
                            label = { Text(stringResource(if (source == MapPriceSource.ANP) R.string.map_source_anp else R.string.map_source_community)) },
                            modifier = Modifier.heightIn(min = 48.dp).testTag("map-source-${source.name.lowercase()}"),
                        )
                    }
                    if (onOpenRoute != null) {
                        TextButton(onClick = onOpenRoute, modifier = Modifier.heightIn(min = 48.dp).testTag("native-route-open")) {
                            Text(stringResource(R.string.map22_title))
                        }
                    }
                    if (ui.source == MapPriceSource.ANP) periodContent()
                }
                if (cityLocationUnknown) {
                    Text(
                        "${municipalityName.orEmpty()}: ${stringResource(R.string.map_location_unknown)}",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                if (offlineCached) {
                    Text(
                        stringResource(R.string.server_station_cached),
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.testTag("map-offline-cache-banner"),
                    )
                }
                if (ui.error != null) {
                    TextButton(onClick = onRetry) { Text(stringResource(R.string.map_retry)) }
                }
                // The region suggestion card replaces the generic row when shown.
                if (ui.noPackage && ui.regionSuggestion == null) {
                    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                        Text(stringResource(R.string.map_no_package), style = MaterialTheme.typography.bodySmall, modifier = Modifier.weight(1f))
                        TextButton(onClick = onOpenDownloads) { Text(stringResource(R.string.map_download)) }
                    }
                }
            }
        }
    }
}
