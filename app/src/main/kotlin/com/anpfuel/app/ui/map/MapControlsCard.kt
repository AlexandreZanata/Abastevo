package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
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
@OptIn(ExperimentalLayoutApi::class, androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
internal fun MapControlsCard(
    ui: MapScreenUi,
    municipalityName: String?,
    cityLocationUnknown: Boolean,
    offlineCached: Boolean,
    onOpenCity: () -> Unit,
    onSelectSource: (MapPriceSource) -> Unit,
    searchQuery: String,
    onSearchQueryChanged: (String) -> Unit,
    onSearch: (String) -> Unit,
    searching: Boolean,
    searchStatus: String?,
    onRetry: () -> Unit,
    onOpenDownloads: () -> Unit,
    onOpenRoute: (() -> Unit)?,
    periodContent: @Composable () -> Unit,
    modifier: Modifier = Modifier,
) {
    var expanded by rememberSaveable { mutableStateOf(false) }
    val focusManager = LocalFocusManager.current
    val submitSearch = { focusManager.clearFocus(); onSearch(searchQuery) }
    val sourceLabel = stringResource(if (ui.source == MapPriceSource.ANP) R.string.map_source_anp else R.string.map_source_community)
    Surface(
        modifier = modifier.fillMaxWidth().testTag("map-controls-card"),
        shape = RoundedCornerShape(28.dp),
        color = MaterialTheme.colorScheme.surfaceContainer.copy(alpha = 0.96f),
        shadowElevation = 8.dp,
    ) {
        Column(
            Modifier.padding(horizontal = 14.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Row(
                Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                MapPill(
                    onClick = onOpenCity,
                    modifier = Modifier.weight(1f, fill = false).testTag("map-city-chip"),
                ) {
                    Icon(Icons.Default.LocationOn, null, Modifier.size(20.dp), tint = MaterialTheme.colorScheme.primary)
                    Text(
                        municipalityName ?: stringResource(R.string.map_city_label),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        style = MaterialTheme.typography.titleSmall,
                        modifier = Modifier.weight(1f, fill = false).testTag("map-city-label"),
                    )
                    Icon(Icons.Default.ExpandMore, null, Modifier.size(20.dp))
                }
                Spacer(Modifier.size(10.dp))
                MapPill(
                    onClick = { expanded = !expanded },
                    modifier = Modifier.testTag("map-controls-toggle"),
                    contentDescription = stringResource(if (expanded) R.string.map_controls_collapse else R.string.map_controls_expand),
                ) {
                    FuelProductIcon(ui.selectedFuelProduct, size = 20.dp)
                    Text(sourceLabel, style = MaterialTheme.typography.titleSmall)
                    Icon(if (expanded) Icons.Default.ExpandLess else Icons.Default.ExpandMore, null, Modifier.size(20.dp))
                }
            }
            OutlinedTextField(
                value = searchQuery,
                onValueChange = onSearchQueryChanged,
                placeholder = { Text(stringResource(R.string.map_search_hint), maxLines = 1, overflow = TextOverflow.Ellipsis) },
                leadingIcon = { Icon(Icons.Default.Search, null) },
                trailingIcon = if (searchQuery.isNotBlank()) {
                    {
                        Row {
                            IconButton(
                                onClick = {
                                    onSearchQueryChanged("")
                                    focusManager.clearFocus()
                                },
                                modifier = Modifier.testTag("map-search-clear"),
                            ) {
                                Icon(Icons.Default.Close, stringResource(R.string.map_clear_search))
                            }
                            IconButton(onClick = submitSearch, modifier = Modifier.testTag("map-search-submit")) {
                                Icon(Icons.AutoMirrored.Filled.ArrowForward, stringResource(R.string.map_route_search), tint = MaterialTheme.colorScheme.primary)
                            }
                        }
                    }
                } else {
                    null
                },
                shape = RoundedCornerShape(24.dp),
                singleLine = true,
                colors = OutlinedTextFieldDefaults.colors(
                    unfocusedContainerColor = MaterialTheme.colorScheme.surfaceContainerHighest.copy(alpha = 0.6f),
                    focusedContainerColor = MaterialTheme.colorScheme.surfaceContainerHighest.copy(alpha = 0.6f),
                    unfocusedBorderColor = MaterialTheme.colorScheme.outlineVariant,
                ),
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
                    modifier = Modifier.padding(start = 4.dp),
                )
            }
            searchStatus?.let {
                Text(it, style = MaterialTheme.typography.labelSmall, modifier = Modifier.padding(start = 4.dp))
            }
            if (ui.loading || searching) LinearProgressIndicator(Modifier.fillMaxWidth())
            if (expanded) {
                FlowRow(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
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

/** Pill-shaped tonal control used in the map header (city, price source). */
@Composable
private fun MapPill(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    contentDescription: String? = null,
    content: @Composable RowScope.() -> Unit,
) {
    Surface(
        onClick = onClick,
        shape = CircleShape,
        color = MaterialTheme.colorScheme.surfaceContainerHighest,
        contentColor = MaterialTheme.colorScheme.onSurface,
        modifier = modifier
            .heightIn(min = 48.dp)
            .then(if (contentDescription != null) Modifier.semantics { this.contentDescription = contentDescription } else Modifier),
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 14.dp, vertical = 10.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalAlignment = Alignment.CenterVertically,
            content = content,
        )
    }
}
