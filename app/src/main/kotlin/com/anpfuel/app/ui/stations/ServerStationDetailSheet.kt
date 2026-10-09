package com.anpfuel.app.ui.stations

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationLocationQuality

/**
 * P35-T02 — canonical server station row and detail.
 *
 * Community-first: the server UUID identity is primary; the dated ANP
 * reference stays a separate secondary read (existing price-group
 * ports, not duplicated here). Unknown locations render an explicit
 * no-coordinates label, never a fabricated point. Stale cache is
 * labelled, never presented as fresh. Route is offered only for
 * reviewed coordinates.
 */
@Composable
fun ServerStationRow(
    station: ServerStation,
    onSelect: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    ListItem(
        leadingContent = { StationArtworkIcon(Modifier.size(48.dp), station.artwork) },
        headlineContent = { Text(text = station.displayName) },
        supportingContent = {
            Text(text = qualityLabel(station))
        },
        trailingContent = {
            if (station.latitude != null) {
                Text(
                    text = "%.3f, %.3f".format(station.latitude, station.longitude),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        },
        modifier = modifier.clickable { onSelect(station.stationId) },
    )
}

@Composable
private fun qualityLabel(station: ServerStation): String =
    when (station.locationQuality) {
        StationLocationQuality.REVIEWED -> stringResource(R.string.server_station_quality_reviewed)
        StationLocationQuality.CITY_CENTROID -> stringResource(R.string.server_station_quality_centroid)
        StationLocationQuality.UNKNOWN -> stringResource(R.string.server_station_quality_unknown)
    }

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ServerStationDetailSheet(
    station: ServerStation,
    fromCache: Boolean,
    onDismiss: () -> Unit,
    onRoute: (ServerStation) -> Unit,
    onUpdatePrice: () -> Unit,
    modifier: Modifier = Modifier,
    /**
     * Canonical discussion target: wire fuel product (e.g.
     * `GASOLINE_REGULAR`) plus caller-held account id (blank = guest
     * reads). Null wire keeps the honest pending card without network.
     */
    fuelProductWire: String? = null,
    accountId: String = "",
    onProfile: () -> Unit = {},
) {
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        modifier = modifier,
        contentWindowInsets = { WindowInsets(0) },
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .navigationBarsPadding()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(
                text = station.displayName,
                style = MaterialTheme.typography.headlineSmall,
                color = MaterialTheme.colorScheme.onSurface,
                modifier = Modifier.semantics { heading() },
            )
            Text(
                text = qualityLabel(station),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (fromCache) {
                Text(
                    text = stringResource(R.string.server_station_cached),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error,
                )
            }
            if (station.latitude != null && station.longitude != null) {
                Text(
                    text = "%.5f, %.5f".format(station.latitude, station.longitude),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                Text(
                    text = stringResource(R.string.server_station_unknown_location),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error,
                )
            }
            station.cnpjNormalized?.let { cnpj ->
                Text(
                    text = cnpj,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            if (fuelProductWire != null) {
                StationFeedbackSection(
                    stationId = station.stationId,
                    fuelProductWire = fuelProductWire,
                    accountId = accountId,
                    modifier = Modifier.fillMaxWidth(),
                )
            } else {
                Card(
                    modifier = Modifier.fillMaxWidth(),
                    colors = CardDefaults.cardColors(
                        containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
                    ),
                ) {
                    Column(
                        modifier = Modifier.padding(16.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        Text(
                            text = stringResource(R.string.community_section_title),
                            style = MaterialTheme.typography.titleMedium,
                            modifier = Modifier.semantics { heading() },
                        )
                        Text(
                            text = stringResource(R.string.community_pending_p04),
                            style = MaterialTheme.typography.bodyMedium,
                        )
                        Text(
                            text = stringResource(R.string.community_confidence_note),
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }

            OutlinedButton(onClick = onProfile, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.station_profile_title))
            }

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Button(
                    onClick = { onRoute(station) },
                    modifier = Modifier.weight(1f),
                    enabled = station.latitude != null,
                ) {
                    Text(text = stringResource(R.string.stations_navigate_action))
                }
                OutlinedButton(
                    onClick = onUpdatePrice,
                    modifier = Modifier.weight(1f),
                ) {
                    Text(text = stringResource(R.string.nav_update_price))
                }
            }
        }
    }
}
