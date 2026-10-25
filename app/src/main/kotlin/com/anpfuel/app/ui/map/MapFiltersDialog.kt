package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.FilterChip
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
import com.anpfuel.app.R

@Composable
fun MapFiltersDialog(
    source: MapPriceSource,
    showFuel: Boolean,
    showCharging: Boolean,
    onSelectSource: (MapPriceSource) -> Unit,
    onServiceFilter: (fuel: Boolean, charging: Boolean) -> Unit,
    onDismiss: () -> Unit,
) {
    var fuel by remember(showFuel) { mutableStateOf(showFuel) }
    var charging by remember(showCharging) { mutableStateOf(showCharging) }
    AlertDialog(
        onDismissRequest = onDismiss,
        confirmButton = {
            TextButton(
                onClick = {
                    onServiceFilter(fuel, charging)
                    onDismiss()
                },
                modifier = Modifier.heightIn(min = 48.dp),
            ) { Text(stringResource(R.string.map_apply)) }
        },
        title = { Text(stringResource(R.string.map_filters)) },
        text = {
            Column(
                modifier = Modifier.fillMaxWidth().padding(4.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                FilterChip(
                    selected = source == MapPriceSource.ANP,
                    onClick = { onSelectSource(MapPriceSource.ANP) },
                    label = { Text(stringResource(R.string.map_source_anp)) },
                    modifier = Modifier.heightIn(min = 48.dp),
                )
                FilterChip(
                    selected = source == MapPriceSource.COMMUNITY,
                    onClick = { onSelectSource(MapPriceSource.COMMUNITY) },
                    label = { Text(stringResource(R.string.map_source_community)) },
                    modifier = Modifier.heightIn(min = 48.dp),
                )
                Text(stringResource(R.string.map_filter_service))
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Checkbox(
                        checked = fuel,
                        onCheckedChange = { fuel = it },
                        modifier = Modifier.semantics { contentDescription = "Fuel filter" },
                    )
                    Text(stringResource(R.string.map_filter_fuel))
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Checkbox(
                        checked = charging,
                        onCheckedChange = { charging = it },
                        modifier = Modifier.semantics { contentDescription = "Charging filter" },
                    )
                    Text(stringResource(R.string.map_filter_charging))
                }
            }
        },
    )
}
