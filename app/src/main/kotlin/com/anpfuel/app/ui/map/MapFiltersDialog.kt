package com.anpfuel.app.ui.map

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
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
import com.anpfuel.app.R
import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.valueobject.FuelProduct

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
    selectedCityCode: String = "4106902",
    availableCities: List<MapCityOption> = DEFAULT_MAP_CITIES,
    onSelectCity: (MapCityOption) -> Unit = {},
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
    var fuel by remember(showFuel) { mutableStateOf(showFuel) }
    var charging by remember(showCharging) { mutableStateOf(showCharging) }
    var fuelProduct by remember(selectedFuelProduct) { mutableStateOf(selectedFuelProduct) }
    var connector by remember(selectedConnector) { mutableStateOf(selectedConnector) }
    var current by remember(selectedCurrent) { mutableStateOf(selectedCurrent) }
    var access by remember(selectedAccess) { mutableStateOf(selectedAccess) }
    var minPowerKw by remember(selectedMinPowerKw) { mutableStateOf(selectedMinPowerKw) }
    var cityCode by remember(selectedCityCode) { mutableStateOf(selectedCityCode) }

    val fuelLabel = stringResource(R.string.map_filter_fuel)
    val chargingLabel = stringResource(R.string.map_filter_charging)

    AlertDialog(
        onDismissRequest = onDismiss,
        confirmButton = {
            TextButton(
                onClick = {
                    availableCities.firstOrNull { it.ibgeCode == cityCode }?.let(onSelectCity)
                    onServiceFilter(fuel, charging)
                    onApplyExtendedFilters(
                        fuel,
                        charging,
                        fuelProduct,
                        connector,
                        current,
                        access,
                        minPowerKw,
                    )
                    onDismiss()
                },
                modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-apply"),
            ) { Text(stringResource(R.string.map_apply)) }
        },
        title = { Text(stringResource(R.string.map_filters)) },
        text = {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .padding(4.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    FilterChip(
                        selected = source == MapPriceSource.ANP,
                        onClick = { onSelectSource(MapPriceSource.ANP) },
                        label = { Text(stringResource(R.string.map_source_anp)) },
                        modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-source-anp"),
                    )
                    FilterChip(
                        selected = source == MapPriceSource.COMMUNITY,
                        onClick = { onSelectSource(MapPriceSource.COMMUNITY) },
                        label = { Text(stringResource(R.string.map_source_community)) },
                        modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-source-community"),
                    )
                }

                Text(stringResource(R.string.map_city_label), style = MaterialTheme.typography.labelLarge)
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    availableCities.forEach { city ->
                        FilterChip(
                            selected = cityCode == city.ibgeCode,
                            onClick = { cityCode = city.ibgeCode },
                            label = { Text("${city.name}/${city.state}") },
                            modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-city-${city.ibgeCode}"),
                        )
                    }
                }

                Text(stringResource(R.string.map_filter_service), style = MaterialTheme.typography.labelLarge)
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Checkbox(
                        checked = fuel,
                        onCheckedChange = { fuel = it },
                        modifier = Modifier.semantics { contentDescription = fuelLabel }.testTag("map-filter-fuel-checkbox"),
                    )
                    Text(fuelLabel)
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Checkbox(
                        checked = charging,
                        onCheckedChange = { charging = it },
                        modifier = Modifier.semantics { contentDescription = chargingLabel }.testTag("map-filter-charging-checkbox"),
                    )
                    Text(chargingLabel)
                }

                Text(stringResource(R.string.map_filter_fuel_product), style = MaterialTheme.typography.labelLarge)
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    FuelProduct.entries.forEach { product ->
                        FilterChip(
                            selected = fuelProduct == product,
                            onClick = { fuelProduct = product },
                            label = { Text(localizedMapProductLabel(product.name)) },
                            modifier = Modifier.heightIn(min = 48.dp).testTag("map-filter-product-${product.name}"),
                        )
                    }
                }

                Text(stringResource(R.string.map_filter_connector), style = MaterialTheme.typography.labelLarge)
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
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

                Text(stringResource(R.string.map_filter_current), style = MaterialTheme.typography.labelLarge)
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
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

                Text(stringResource(R.string.map_filter_min_power), style = MaterialTheme.typography.labelLarge)
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
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

                Text(stringResource(R.string.map_filter_access), style = MaterialTheme.typography.labelLarge)
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
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

                Text(stringResource(R.string.map_hours_unknown), style = MaterialTheme.typography.bodySmall)
                Text(
                    stringResource(R.string.map_filter_unknown_kept_note),
                    style = MaterialTheme.typography.bodySmall,
                )
            }
        },
    )
}
