package com.anpfuel.app.ui.charging

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.EmptyState
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.components.OfflineBanner
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.data.repository.DiscoveryRow
import com.anpfuel.data.repository.PlaceDetail
import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.charging.Compatibility
import com.anpfuel.domain.valueobject.DomainId
import java.text.DateFormat
import java.util.Date
import java.util.Locale

@Composable
fun ChargingExploreScreen(
    vehicleId: String,
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: ChargingExploreViewModel = hiltViewModel(),
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val locale = LocalConfiguration.current.locales[0]

    LaunchedEffect(vehicleId) {
        viewModel.load(DomainId.from(vehicleId))
    }

    ChargingExploreContent(
        uiState = uiState,
        locale = locale,
        onConnectorChanged = viewModel::onConnectorChanged,
        onCurrentChanged = viewModel::onCurrentChanged,
        onAccessChanged = viewModel::onAccessChanged,
        onRefresh = viewModel::refresh,
        onLoadMore = viewModel::loadMore,
        onOpenDetail = viewModel::openDetail,
        onDismissDetail = viewModel::dismissDetail,
        onDismissError = viewModel::dismissError,
        onNavigateBack = onNavigateBack,
        modifier = modifier,
    )
}

@OptIn(ExperimentalLayoutApi::class, ExperimentalMaterial3Api::class)
@Composable
internal fun ChargingExploreContent(
    uiState: ChargingExploreUiState,
    locale: Locale,
    onConnectorChanged: (ChargingConnector?) -> Unit,
    onCurrentChanged: (ChargingCurrent?) -> Unit,
    onAccessChanged: (ChargingAccess?) -> Unit,
    onRefresh: () -> Unit,
    onLoadMore: () -> Unit,
    onOpenDetail: (String) -> Unit,
    onDismissDetail: () -> Unit,
    onDismissError: () -> Unit,
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            AnpTopAppBar(
                title = {
                    Text(
                        text = uiState.vehicleName.ifBlank { stringResource(R.string.charging_explore_title) },
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                },
                onNavigateUp = onNavigateBack,
            )
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .padding(horizontal = 16.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            if (uiState.isOffline) {
                OfflineBanner()
            }
            Text(
                text = stringResource(R.string.charging_explore_cached_notice),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            uiState.sourceRevision?.let { revision ->
                Text(
                    text = stringResource(
                        R.string.charging_explore_source,
                        revision,
                        uiState.sourceSavedAt?.let {
                            DateFormat.getDateInstance(DateFormat.SHORT, locale).format(Date(it))
                        }.orEmpty(),
                    ),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Text(
                text = stringResource(R.string.charging_explore_connector_label),
                style = MaterialTheme.typography.titleSmall,
            )
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(
                    selected = uiState.connector == null,
                    onClick = { onConnectorChanged(null) },
                    label = { Text(text = stringResource(R.string.charging_explore_all)) },
                )
                ChargingConnector.entries.forEach { connector ->
                    FilterChip(
                        selected = uiState.connector == connector,
                        onClick = { onConnectorChanged(if (uiState.connector == connector) null else connector) },
                        label = { Text(text = connectorLabel(connector)) },
                    )
                }
            }
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(
                    selected = uiState.current == null,
                    onClick = { onCurrentChanged(null) },
                    label = { Text(text = stringResource(R.string.charging_explore_all_current)) },
                )
                listOf(ChargingCurrent.AC, ChargingCurrent.DC).forEach { current ->
                    FilterChip(
                        selected = uiState.current == current,
                        onClick = { onCurrentChanged(if (uiState.current == current) null else current) },
                        label = { Text(text = current.name) },
                    )
                }
                FilterChip(
                    selected = uiState.access == ChargingAccess.PUBLIC,
                    onClick = { onAccessChanged(if (uiState.access == ChargingAccess.PUBLIC) null else ChargingAccess.PUBLIC) },
                    label = { Text(text = stringResource(R.string.charging_explore_public_only)) },
                )
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedButton(onClick = onRefresh, modifier = Modifier.weight(1f)) {
                    Text(text = stringResource(R.string.charging_explore_refresh))
                }
            }
            if (uiState.isLoading) {
                CircularProgressIndicator(modifier = Modifier.align(Alignment.CenterHorizontally))
            } else if (uiState.rows.isEmpty()) {
                EmptyState(
                    message = stringResource(R.string.charging_explore_empty),
                    modifier = Modifier.fillMaxWidth(),
                )
            } else {
                LazyColumn(
                    modifier = Modifier.weight(1f, fill = false),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    items(uiState.rows, key = { it.place.id }) { row ->
                        ChargingPlaceRow(row = row, locale = locale, onClick = { onOpenDetail(row.place.id) })
                    }
                    if (uiState.hasMore) {
                        item {
                            Button(onClick = onLoadMore, modifier = Modifier.fillMaxWidth()) {
                                Text(text = stringResource(R.string.charging_explore_more))
                            }
                        }
                    }
                }
            }
            uiState.errorMessage?.let {
                ErrorState(
                    message = it,
                    modifier = Modifier.fillMaxWidth(),
                    onRetry = { onDismissError() },
                )
            }
        }
    }

    uiState.detail?.let { detail ->
        AlertDialog(
            onDismissRequest = onDismissDetail,
            title = { Text(text = detail.place.name) },
            text = {
                Column(
                    modifier = Modifier.verticalScroll(rememberScrollState()),
                    verticalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    Text(text = stringResource(R.string.charging_explore_detail_access, detail.place.publicAccess.name))
                    detail.place.latitude?.let { Text(text = "Lat $it") }
                    detail.place.longitude?.let { Text(text = "Lon $it") }
                    Text(text = stringResource(R.string.charging_explore_detail_points, detail.place.reportedPoints.toInt()))
                    detail.place.groups.forEach { group ->
                        Text(
                            text = "${group.rawConnectorCode} · ${group.current} · " +
                                (group.powerW?.let { "${it / 1000} kW" } ?: "?") +
                                (group.reportedQuantity?.let { " · ×$it" } ?: ""),
                            style = MaterialTheme.typography.bodyMedium,
                        )
                    }
                    Text(
                        text = stringResource(R.string.charging_explore_detail_source, detail.sourceRevision),
                        style = MaterialTheme.typography.bodySmall,
                    )
                    Text(
                        text = stringResource(R.string.charging_explore_detail_no_tariff),
                        style = MaterialTheme.typography.bodySmall,
                    )
                }
            },
            confirmButton = {
                TextButton(onClick = onDismissDetail) {
                    Text(text = stringResource(R.string.action_cancel))
                }
            },
        )
    }
}

@Composable
private fun connectorLabel(connector: ChargingConnector): String = when (connector) {
    ChargingConnector.CCS2 -> "CCS 2"
    ChargingConnector.TYPE2 -> "Type 2"
    ChargingConnector.CHADEMO -> "CHAdeMO"
    ChargingConnector.NACS -> "NACS"
    ChargingConnector.UNKNOWN -> stringResource(R.string.charging_explore_unknown)
}

@Composable
private fun ChargingPlaceRow(
    row: DiscoveryRow,
    locale: Locale,
    onClick: () -> Unit,
) {
    Card(
        onClick = onClick,
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow),
    ) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(text = row.place.name, style = MaterialTheme.typography.titleMedium)
            Text(
                text = when (row.verdict) {
                    Compatibility.COMPATIBLE -> stringResource(R.string.charging_explore_compatible)
                    Compatibility.UNKNOWN -> stringResource(R.string.charging_explore_unknown_compat)
                    Compatibility.INCOMPATIBLE -> stringResource(R.string.charging_explore_incompatible)
                },
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = row.place.groups.joinToString { "${it.rawConnectorCode}/${it.current}" }
                    .ifEmpty { stringResource(R.string.charging_explore_no_groups) },
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = stringResource(
                    R.string.charging_explore_row_meta,
                    row.profile,
                    DateFormat.getDateInstance(DateFormat.SHORT, locale).format(Date(row.savedAtMillis)),
                ),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Preview(showBackground = true, name = "Charging explore")
@Composable
private fun ChargingExplorePreview() {
    AnpFuelTheme {
        ChargingExploreContent(
            uiState = ChargingExploreUiState(isLoading = false, vehicleName = "Dolphin"),
            locale = Locale.US,
            onConnectorChanged = {},
            onCurrentChanged = {},
            onAccessChanged = {},
            onRefresh = {},
            onLoadMore = {},
            onOpenDetail = {},
            onDismissDetail = {},
            onDismissError = {},
            onNavigateBack = {},
        )
    }
}
