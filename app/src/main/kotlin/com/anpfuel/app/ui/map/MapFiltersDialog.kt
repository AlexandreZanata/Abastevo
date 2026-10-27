package com.anpfuel.app.ui.map

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.EvStation
import androidx.compose.material.icons.filled.LocalGasStation
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.FuelProductSelectorButton
import com.anpfuel.app.ui.components.FuelProductSelectorDialog
import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.valueobject.FuelProduct

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
fun MapFiltersDialog(
    source: MapPriceSource,
    showFuel: Boolean,
    showCharging: Boolean,
    onSelectSource: (MapPriceSource) -> Unit,
    onServiceFilter: (fuel: Boolean, charging: Boolean) -> Unit,
    onDismiss: () -> Unit,
    selectedFuelProduct: FuelProduct = FuelProduct.GASOLINE_REGULAR,
    selectedConnector: ChargingConnector? = null,
    selectedCurrent: ChargingCurrent? = null,
    selectedAccess: ChargingAccess? = null,
    selectedMinPowerKw: Int? = null,
    cityLabel: String = "",
    onOpenCity: () -> Unit = {},
    onApplyExtendedFilters: (
        fuel: Boolean,
        charging: Boolean,
        fuelProduct: FuelProduct,
        connector: ChargingConnector?,
        current: ChargingCurrent?,
        access: ChargingAccess?,
        minPowerKw: Int?,
    ) -> Unit = { f, c, _, _, _, _, _ -> onServiceFilter(f, c) },
) {
    var draftSource by rememberSaveable { mutableStateOf(source) }
    var fuel by rememberSaveable { mutableStateOf(showFuel) }
    var charging by rememberSaveable { mutableStateOf(showCharging) }
    var fuelProduct by rememberSaveable { mutableStateOf(selectedFuelProduct) }
    var fuelPickerOpen by rememberSaveable { mutableStateOf(false) }
    var connector by rememberSaveable { mutableStateOf(selectedConnector) }
    var current by rememberSaveable { mutableStateOf(selectedCurrent) }
    var access by rememberSaveable { mutableStateOf(selectedAccess) }
    var minPowerKw by rememberSaveable { mutableStateOf(selectedMinPowerKw) }

    val citySearchLabel = "$cityLabel, ${stringResource(R.string.location_search_municipality_hint)}"

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        containerColor = MaterialTheme.colorScheme.surface,
    ) {
        Column(Modifier.fillMaxWidth().fillMaxHeight(0.92f)) {
            Row(
                Modifier.fillMaxWidth().padding(start = 24.dp, end = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    stringResource(R.string.map_filters),
                    style = MaterialTheme.typography.headlineSmall,
                    modifier = Modifier.weight(1f),
                )
                IconButton(onClick = onDismiss, modifier = Modifier.testTag("map-filter-close")) {
                    Icon(Icons.Default.Close, stringResource(R.string.action_dismiss))
                }
            }
            Column(
                modifier = Modifier.weight(1f).verticalScroll(rememberScrollState())
                    .padding(horizontal = 24.dp, vertical = 16.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                // The same full-catalog state/city picker used by the Location page.
                Box(Modifier.fillMaxWidth()) {
                    OutlinedTextField(
                        value = cityLabel,
                        onValueChange = {},
                        readOnly = true,
                        label = { Text(stringResource(R.string.map_city_label)) },
                        placeholder = { Text(stringResource(R.string.location_search_municipality_hint)) },
                        leadingIcon = { Icon(Icons.Default.Search, null) },
                        trailingIcon = { Icon(Icons.AutoMirrored.Filled.ArrowForward, null) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Box(
                        Modifier.matchParentSize()
                            .clickable(
                                role = Role.Button,
                                onClickLabel = stringResource(R.string.location_search_municipality_hint),
                                onClick = onOpenCity,
                            ).semantics { contentDescription = citySearchLabel }.testTag("map-filter-city-search"),
                    )
                }
                MapFilterSection(stringResource(R.string.map_filter_source)) {
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        MapPriceSource.entries.forEach { option ->
                            FilterChip(
                                selected = draftSource == option,
                                onClick = { draftSource = option },
                                label = { Text(stringResource(if (option == MapPriceSource.ANP) R.string.map_source_anp else R.string.map_source_community)) },
                                leadingIcon = if (draftSource == option) { { Icon(Icons.Default.Check, null, Modifier.size(18.dp)) } } else null,
                                modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-source-${option.name.lowercase()}"),
                            )
                        }
                    }
                }
                MapFilterSection(stringResource(R.string.map_filter_service)) {
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        FilterChip(
                            selected = fuel,
                            onClick = { fuel = !fuel },
                            leadingIcon = { Icon(Icons.Default.LocalGasStation, null, Modifier.size(20.dp)) },
                            label = { Text(stringResource(R.string.map_filter_fuel)) },
                            modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-fuel-checkbox"),
                        )
                        FilterChip(
                            selected = charging,
                            onClick = { charging = !charging },
                            leadingIcon = { Icon(Icons.Default.EvStation, null, Modifier.size(20.dp)) },
                            label = { Text(stringResource(R.string.map_filter_charging)) },
                            modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-charging-checkbox"),
                        )
                    }
                }
                if (fuel) {
                    MapFilterSection(stringResource(R.string.map_filter_fuel_product)) {
                        FuelProductSelectorButton(
                            selected = fuelProduct,
                            onClick = { fuelPickerOpen = true },
                            modifier = Modifier.testTag("map-filter-fuel-selector"),
                        )
                    }
                }

                if (charging) {
                    HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                    MapFilterSection(stringResource(R.string.map_filter_connector)) {
                        FlowRow(
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalArrangement = Arrangement.spacedBy(4.dp),
                        ) {
                            FilterChip(
                                selected = connector == null,
                                onClick = { connector = null },
                                label = { Text(stringResource(R.string.map_filter_all)) },
                                modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-connector-ALL"),
                            )
                            listOf(
                                ChargingConnector.CCS2,
                                ChargingConnector.TYPE2,
                                ChargingConnector.CHADEMO,
                                ChargingConnector.NACS,
                            ).forEach { conn ->
                                FilterChip(
                                    selected = connector == conn,
                                    onClick = { connector = conn },
                                    label = { Text(localizedMapProductLabel(conn.name)) },
                                    modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-connector-${conn.name}"),
                                )
                            }
                        }
                    }

                    MapFilterSection(stringResource(R.string.map_filter_current)) {
                        FlowRow(
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalArrangement = Arrangement.spacedBy(4.dp),
                        ) {
                            FilterChip(
                                selected = current == null,
                                onClick = { current = null },
                                label = { Text(stringResource(R.string.map_filter_all)) },
                                modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-current-ALL"),
                            )
                            listOf(ChargingCurrent.AC, ChargingCurrent.DC).forEach { cur ->
                                FilterChip(
                                    selected = current == cur,
                                    onClick = { current = cur },
                                    label = { Text(cur.name) },
                                    modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-current-${cur.name}"),
                                )
                            }
                        }
                    }

                    MapFilterSection(stringResource(R.string.map_filter_min_power)) {
                        FlowRow(
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalArrangement = Arrangement.spacedBy(4.dp),
                        ) {
                            FilterChip(
                                selected = minPowerKw == null,
                                onClick = { minPowerKw = null },
                                label = { Text(stringResource(R.string.map_filter_all)) },
                                modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-power-ALL"),
                            )
                            listOf(22, 50, 150).forEach { kw ->
                                FilterChip(
                                    selected = minPowerKw == kw,
                                    onClick = { minPowerKw = kw },
                                    label = { Text("${kw} kW") },
                                    modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-power-$kw"),
                                )
                            }
                        }
                    }

                    MapFilterSection(stringResource(R.string.map_filter_access)) {
                        FlowRow(
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalArrangement = Arrangement.spacedBy(4.dp),
                        ) {
                            FilterChip(
                                selected = access == null,
                                onClick = { access = null },
                                label = { Text(stringResource(R.string.map_filter_all)) },
                                modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-access-ALL"),
                            )
                            FilterChip(
                                selected = access == ChargingAccess.PUBLIC,
                                onClick = { access = ChargingAccess.PUBLIC },
                                label = { Text(stringResource(R.string.charging_explore_public_only)) },
                                modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-access-PUBLIC"),
                            )
                        }
                    }

                    Text(
                        stringResource(R.string.map_hours_unknown),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Text(
                        stringResource(R.string.map_filter_unknown_kept_note),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
            Surface(color = MaterialTheme.colorScheme.surfaceContainerLow) {
                Row(
                    Modifier.fillMaxWidth().padding(horizontal = 24.dp, vertical = 12.dp),
                    horizontalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    OutlinedButton(
                        onClick = onDismiss,
                        modifier = Modifier.weight(1f).heightIn(min = 48.dp).testTag("map-filter-cancel"),
                    ) { Text(stringResource(R.string.action_cancel)) }
                    Button(
                        onClick = {
                            onSelectSource(draftSource)
                            onApplyExtendedFilters(fuel, charging, fuelProduct, connector, current, access, minPowerKw)
                            onDismiss()
                        },
                        modifier = Modifier.weight(1f).heightIn(min = 48.dp).testTag("map-filter-apply"),
                    ) { Text(stringResource(R.string.map_apply)) }
                }
            }
        }
    }
    if (fuel && fuelPickerOpen) {
        FuelProductSelectorDialog(
            selected = fuelProduct,
            onSelect = { product ->
                fuelProduct = product
                fuelPickerOpen = false
            },
            onDismiss = { fuelPickerOpen = false },
            modifier = Modifier.testTag("map-filter-fuel-picker"),
        )
    }

}

@Composable
private fun MapFilterSection(title: String, content: @Composable () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(title, style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        content()
    }
}
