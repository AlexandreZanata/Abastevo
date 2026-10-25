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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.maps.engine.MapStationLocationQuality

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
    routeError: Boolean = false,
) {
    val ui by viewModel.ui.collectAsStateWithLifecycle()
    val marker = ui.marker ?: return
    val sourceName = if (ui.source == MapPriceSource.ANP) {
        stringResource(R.string.map_source_anp)
    } else {
        stringResource(R.string.map_source_community)
    }
    val locationQualityLabel = when (marker.locationQuality) {
        MapStationLocationQuality.REVIEWED -> stringResource(R.string.map_location_reviewed)
        MapStationLocationQuality.CITY_CENTROID -> stringResource(R.string.map_location_city_centroid)
        MapStationLocationQuality.UNKNOWN -> stringResource(R.string.map_location_unknown)
    }
    ModalBottomSheet(onDismissRequest = onDismiss, modifier = modifier) {
        Column(
            modifier = Modifier.fillMaxWidth().padding(16.dp).heightIn(min = 48.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(marker.title, style = MaterialTheme.typography.titleLarge)
            Text(
                when {
                    marker.fuel && marker.charging -> stringResource(R.string.map_filter_fuel_charging)
                    marker.charging -> stringResource(R.string.map_filter_charging)
                    else -> stringResource(R.string.map_filter_fuel)
                },
                style = MaterialTheme.typography.bodyMedium,
            )
            Text(
                stringResource(R.string.map_sheet_source_label, sourceName),
                style = MaterialTheme.typography.bodySmall,
            )
            when {
                ui.loading -> Text(stringResource(R.string.map_sheet_loading_price))
                ui.price != null -> {
                    val p = ui.price!!
                    Text("${p.product} ${p.amount} ${p.unit}", style = MaterialTheme.typography.titleMedium)
                    Text(p.dateLabel, style = MaterialTheme.typography.bodySmall)
                    if (p.stale) {
                        Text(
                            stringResource(R.string.map_sheet_cached_price),
                            style = MaterialTheme.typography.labelSmall,
                        )
                    }
                }
                ui.absentReason == "anp-no-tariff" -> {
                    Text(stringResource(R.string.map_sheet_anp_no_tariff))
                    Button(
                        onClick = onSwitchToCommunity,
                        modifier = Modifier.heightIn(min = 48.dp).testTag("map-sheet-switch-community"),
                    ) {
                        Text(stringResource(R.string.map_sheet_show_community))
                    }
                }
                ui.absentReason == "load-failed" -> {
                    Text(stringResource(R.string.map_sheet_price_load_failed))
                }
                else -> Text(stringResource(R.string.map_sheet_no_price))
            }
            Text(
                stringResource(R.string.map_sheet_location_label, locationQualityLabel),
                style = MaterialTheme.typography.bodySmall,
            )
            if (!marker.canRouteByCoordinates) {
                Text(
                    stringResource(R.string.map_sheet_route_unavailable),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.testTag("map-sheet-route-unavailable"),
                )
            }
            if (routeError) {
                Text(
                    stringResource(R.string.map_sheet_route_failed),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.testTag("map-sheet-route-error"),
                )
            }
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Button(
                    onClick = { onRoute(marker.uuid) },
                    enabled = marker.canRouteByCoordinates,
                    modifier = Modifier.weight(1f).heightIn(min = 48.dp).testTag("map-sheet-route"),
                ) { Text(stringResource(R.string.map_sheet_route)) }
                TextButton(
                    onClick = { onOpenProfile(marker.uuid) },
                    modifier = Modifier.weight(1f).heightIn(min = 48.dp).testTag("map-sheet-profile"),
                ) { Text(stringResource(R.string.map_sheet_profile)) }
                TextButton(
                    onClick = { onReport(marker.uuid) },
                    modifier = Modifier.weight(1f).heightIn(min = 48.dp).testTag("map-sheet-report"),
                ) { Text(stringResource(R.string.map_sheet_report)) }
            }
        }
    }
}
