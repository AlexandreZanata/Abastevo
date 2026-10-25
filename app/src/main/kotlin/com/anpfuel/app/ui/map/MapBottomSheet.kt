package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.maps.engine.MapStationType

/**
 * MAP-19 quick sheet: name, services, exact source/product/unit/price/date,
 * quality/access and real actions (route, profile, report). ANP absence is
 * explicit; community never mixes silently; EV in ANP offers explicit switch.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MapBottomSheet(
    viewModel: MapSheetViewModel,
    onOpenProfile: (stationUuid: String) -> Unit,
    onRoute: (stationUuid: String) -> Unit,
    onReport: (stationUuid: String) -> Unit,
    onSwitchToCommunity: () -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val ui by viewModel.ui.collectAsStateWithLifecycle()
    val marker = ui.marker ?: return
    ModalBottomSheet(onDismissRequest = onDismiss, modifier = modifier) {
        Column(
            modifier = Modifier.fillMaxWidth().padding(16.dp).heightIn(min = 48.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(marker.title, style = MaterialTheme.typography.titleLarge)
            Text(
                when {
                    marker.fuel && marker.charging -> "Fuel and charging"
                    marker.charging -> "Charging"
                    else -> "Fuel"
                },
                style = MaterialTheme.typography.bodyMedium,
            )
            Text(
                "Source: ${if (ui.source == MapPriceSource.ANP) "ANP" else "Community"}",
                style = MaterialTheme.typography.bodySmall,
            )
            when {
                ui.loading -> Text("Loading price…")
                ui.price != null -> {
                    val p = ui.price!!
                    Text("${p.product} ${p.amount} ${p.unit}", style = MaterialTheme.typography.titleMedium)
                    Text(p.dateLabel, style = MaterialTheme.typography.bodySmall)
                }
                ui.absentReason == "anp-no-tariff" -> {
                    Text("ANP does not supply electric tariffs.")
                    Button(onClick = onSwitchToCommunity) { Text("Show Community") }
                }
                else -> Text("No price for this station.")
            }
            Text(
                "Location: ${marker.locationQuality}",
                style = MaterialTheme.typography.bodySmall,
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = { onRoute(marker.uuid) }) { Text("Route") }
                TextButton(onClick = { onOpenProfile(marker.uuid) }) { Text("Profile") }
                TextButton(onClick = { onReport(marker.uuid) }) { Text("Report") }
            }
        }
    }
}
