package com.anpfuel.app.ui.stations

import android.content.ClipData
import android.content.ClipboardManager
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.rememberLazyListState
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
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.community.FeedbackViewModel
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.feedback.FeedbackTarget
import java.util.Locale

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
            Column {
                station.secondaryLegalName?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
                if (station.addressLine.isNotBlank()) Text(station.addressLine)
                Text(qualityLabel(station), style = MaterialTheme.typography.labelSmall)
            }
        },
        modifier = modifier.clickable { onSelect(station.stationId) },
    )
}

@Composable
private fun qualityLabel(station: ServerStation): String =
    when (station.locationQuality) {
        StationLocationQuality.REVIEWED -> stringResource(R.string.server_station_quality_reviewed)
        StationLocationQuality.OSM_MAPPED -> stringResource(R.string.server_station_quality_osm_mapped)
        StationLocationQuality.ANP_REPORTED -> stringResource(R.string.server_station_quality_anp_reported)
        StationLocationQuality.ADDRESS_POINT -> stringResource(R.string.server_station_quality_address_point)
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
        // Single virtualized list: station header, discussion rows keyed by
        // comment id, then actions. No nested scrollable lives here.
        val feedbackTarget = remember(station.stationId, fuelProductWire, accountId) {
            if (fuelProductWire == null) {
                null
            } else {
                runCatching { FeedbackTarget.create(station.stationId, fuelProductWire, accountId) }.getOrNull()
            }
        }
        val feedbackViewModel = if (feedbackTarget != null) hiltViewModel<FeedbackViewModel>() else null
        val feedbackState = feedbackViewModel?.state?.collectAsStateWithLifecycle()?.value
        if (feedbackTarget != null && feedbackViewModel != null) {
            LaunchedEffect(feedbackTarget) {
                feedbackViewModel.prepare(
                    feedbackTarget.stationId,
                    feedbackTarget.fuelProductWire,
                    feedbackTarget.accountId,
                )
                feedbackViewModel.loadFirstPage()
                feedbackViewModel.loadStats()
            }
        }
        val context = LocalContext.current
        val coordinates = station.latitude?.let { latitude ->
            station.longitude?.let { longitude -> String.format(Locale.ROOT, "%.5f, %.5f", latitude, longitude) }
        }
        LazyColumn(
            state = rememberLazyListState(),
            modifier = Modifier
                .fillMaxWidth()
                .navigationBarsPadding(),
            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item(key = "detail-title") {
                Text(
                    text = station.displayName,
                    style = MaterialTheme.typography.headlineSmall,
                    color = MaterialTheme.colorScheme.onSurface,
                    modifier = Modifier.semantics { heading() },
                )
            }
            station.secondaryLegalName?.let { name ->
                item(key = "detail-legal-name") {
                    Text(name, style = MaterialTheme.typography.bodyMedium)
                }
            }
            if (station.addressLine.isNotBlank()) {
                item(key = "detail-address") {
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text(station.addressLine, style = MaterialTheme.typography.bodyLarge)
                        if (station.hasApproximateAddress) {
                            Text(
                                stringResource(R.string.server_station_approximate_address),
                                style = MaterialTheme.typography.labelMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                }
            }
            item(key = "detail-quality") {
                Text(
                    text = qualityLabel(station),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (fromCache) {
                item(key = "detail-cache") {
                    Text(
                        text = stringResource(R.string.server_station_cached),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
            if (coordinates == null) {
                item(key = "detail-no-coords") {
                    Text(
                        text = stringResource(R.string.server_station_unknown_location),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
            station.cnpjNormalized?.let { cnpj ->
                item(key = "detail-cnpj") {
                    Text(
                        text = cnpj,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }

            val target = feedbackTarget
            if (target != null && feedbackViewModel != null && feedbackState != null) {
                stationFeedbackItems(
                    state = feedbackState,
                    isSignedIn = target.isSignedIn,
                    onRate = feedbackViewModel::rate,
                    onDeleteRating = feedbackViewModel::deleteRating,
                    onComment = feedbackViewModel::submitComment,
                    onVote = feedbackViewModel::submitVote,
                    onReport = feedbackViewModel::report,
                    onRetry = feedbackViewModel::retry,
                    onLoadMore = feedbackViewModel::loadMore,
                )
            } else {
                item(key = "detail-feedback-pending") {
                    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
                    Card(
                        modifier = Modifier.fillMaxWidth(),
                        colors = CardDefaults.cardColors(
                            containerColor = containerColor,
                        ),
                        border = AnpContainerDefaults.borderStroke(containerColor),
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
            }

            item(key = "detail-profile") {
                OutlinedButton(onClick = onProfile, modifier = Modifier.fillMaxWidth()) {
                    Text(stringResource(R.string.station_profile_title))
                }
            }

            if (coordinates != null) {
                item(key = "detail-copy-coordinates") {
                    val label = stringResource(R.string.server_station_copy_coordinates)
                    OutlinedButton(
                        onClick = {
                            context.getSystemService(ClipboardManager::class.java)
                                ?.setPrimaryClip(ClipData.newPlainText(label, coordinates))
                        },
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(label)
                    }
                }
            }

            item(key = "detail-actions") {
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
}
