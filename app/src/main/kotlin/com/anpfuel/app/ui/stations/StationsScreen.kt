package com.anpfuel.app.ui.stations

import android.Manifest
import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.MyLocation
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.AppErrorMapper
import com.anpfuel.app.navigation.MapAppChooser
import com.anpfuel.app.navigation.MapNavigationResult
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.ui.components.AnpAttributionFooter
import com.anpfuel.app.ui.components.EmptyState
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.components.FuelProductIcon
import com.anpfuel.app.ui.components.StationsNavigateHintBanner
import com.anpfuel.app.ui.components.FuelProductLabel
import com.anpfuel.app.ui.components.LoadingState
import com.anpfuel.app.ui.components.OfflineBanner
import com.anpfuel.app.ui.components.StationPriceRow
import com.anpfuel.app.ui.weekpicker.SurveyWeekChipAction
import com.anpfuel.app.ui.model.StationPriceUiModel
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct

@Composable
fun StationsScreen(
    onNavigateBack: (() -> Unit)? = null,
    onNavigateToUpdatePrice: () -> Unit = {},
    onNavigateToUpdatePriceWithTarget: (String, String) -> Unit = { _, _ -> },
    onSuggestStation: () -> Unit = {},
    modifier: Modifier = Modifier,
    viewModel: StationsViewModel = hiltViewModel(),
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val locale = LocalConfiguration.current.locales[0]
    val context = LocalContext.current

    val locationPermissionLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.RequestMultiplePermissions(),
    ) { permissions ->
        if (permissions.values.any { it }) {
            viewModel.onLocationPermissionGranted()
        } else {
            viewModel.onLocationPermissionDenied()
        }
    }

    LaunchedEffect(viewModel) {
        viewModel.locationPermissionRequest.collect {
            locationPermissionLauncher.launch(
                arrayOf(
                    Manifest.permission.ACCESS_FINE_LOCATION,
                    Manifest.permission.ACCESS_COARSE_LOCATION,
                ),
            )
        }
    }

    LaunchedEffect(viewModel) {
        viewModel.messages.collect { message ->
            val messageRes = when (message) {
                StationsMessage.NearestNeedsLocation -> R.string.stations_nearest_needs_location
                StationsMessage.NearestNoFix -> R.string.stations_nearest_no_fix
                StationsMessage.NearestUnavailable -> R.string.stations_nearest_failed
            }
            Toast.makeText(context, context.getString(messageRes), Toast.LENGTH_LONG).show()
        }
    }

    LaunchedEffect(viewModel) {
        viewModel.navigationEffects.collect { effect ->
            when (effect) {
                is StationsNavigationEffect.LaunchMaps -> {
                    when (MapAppChooser.openNavigation(context, effect.navigationQuery)) {
                        MapNavigationResult.NoAppFound -> {
                            Toast.makeText(
                                context,
                                context.getString(R.string.stations_navigate_no_app),
                                Toast.LENGTH_SHORT,
                            ).show()
                        }
                        MapNavigationResult.Launched -> Unit
                    }
                }
            }
        }
    }

    LaunchedEffect(locale) {
        viewModel.load(locale)
    }

    LaunchedEffect(viewModel) {
        viewModel.loadServerStations()
    }

    StationsContent(
        uiState = uiState,
        onNavigateBack = onNavigateBack,
        onFuelProductSelected = { fuelProduct -> viewModel.onFuelProductSelected(fuelProduct, locale) },
        onSearchQueryChanged = { query -> viewModel.onSearchQueryChanged(query, locale) },
        onFindNearestStation = viewModel::onFindNearestStation,
        onDownloadStationDetail = { viewModel.downloadStationDetail(locale) },
        onRetry = { viewModel.load(locale) },
        onWeekChanged = { viewModel.load(locale) },
        onStationSelected = viewModel::onStationSelected,
        onNavigateToStation = viewModel::onNavigateToStation,
        onDetailDismissed = viewModel::onDetailDismissed,
        onNavigateToUpdatePrice = onNavigateToUpdatePrice,
        onServerStationSelected = viewModel::onServerStationSelected,
        onServerDetailDismissed = viewModel::onServerDetailDismissed,
        onServerStationNavigate = viewModel::onServerStationNavigate,
        onServerUpdatePrice = { station, fuelWire ->
            onNavigateToUpdatePriceWithTarget(station.stationId, fuelWire)
        },
        onSuggestStation = onSuggestStation,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
private fun StationsContent(
    uiState: StationsUiState,
    onNavigateBack: (() -> Unit)? = null,
    onFuelProductSelected: (FuelProduct) -> Unit,
    onSearchQueryChanged: (String) -> Unit,
    onFindNearestStation: () -> Unit,
    onDownloadStationDetail: () -> Unit,
    onRetry: () -> Unit,
    onWeekChanged: () -> Unit,
    onStationSelected: (String) -> Unit,
    onNavigateToStation: (String) -> Unit,
    onDetailDismissed: () -> Unit,
    onNavigateToUpdatePrice: () -> Unit,
    onServerStationSelected: (String) -> Unit = {},
    onServerDetailDismissed: () -> Unit = {},
    onServerStationNavigate: (com.anpfuel.domain.discovery.ServerStation) -> Unit = {},
    onServerUpdatePrice: (com.anpfuel.domain.discovery.ServerStation, String) -> Unit = { _, _ -> },
    onSuggestStation: () -> Unit = {},
    modifier: Modifier = Modifier,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            AnpTopAppBar(
                onNavigateUp = onNavigateBack,
                title = {
                    Text(
                        text = stringResource(R.string.stations_title),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                },
                actions = {
                    SurveyWeekChipAction(onWeekChanged = onWeekChanged)
                },
            )
        },
        bottomBar = { AnpAttributionFooter() },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            if (uiState.isOffline) {
                OfflineBanner()
            }

            Text(
                text = stringResource(R.string.history_fuel_product_label),
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            FlowRow(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                for (product in FuelProduct.entries) {
                    FilterChip(
                        selected = uiState.selectedFuelProduct == product,
                        onClick = { onFuelProductSelected(product) },
                        label = {
                            Text(text = stringResource(FuelProductI18n.toStringRes(product)))
                        },
                        leadingIcon = {
                            FuelProductIcon(
                                product = product,
                                size = 18.dp,
                                contentDescription = null,
                            )
                        },
                        enabled = !uiState.isDownloading,
                    )
                }
            }

            if (!uiState.showNoLocation && !uiState.showDownloadPrompt) {
                androidx.compose.material3.OutlinedTextField(
                    value = uiState.searchQuery,
                    onValueChange = onSearchQueryChanged,
                    modifier = Modifier.fillMaxWidth(),
                    placeholder = { Text(text = stringResource(R.string.stations_search_placeholder)) },
                    leadingIcon = {
                        Icon(
                            imageVector = Icons.Default.Search,
                            contentDescription = null,
                        )
                    },
                    singleLine = true,
                )
            }

            when {
                // P20-T02: a failed refresh keeps cached stations, so loading
                // and error replace the list only when there is nothing cached.
                (uiState.isLoading || uiState.isDownloading) && uiState.stations.isEmpty() -> {
                    LoadingState(
                        modifier = Modifier.fillMaxWidth(),
                    )
                }

                uiState.error != null && uiState.stations.isEmpty() -> {
                    ErrorState(
                        message = stringResource(AppErrorMapper.toStringRes(uiState.error)),
                        modifier = Modifier.fillMaxWidth(),
                        onRetry = onRetry,
                    )
                }

                uiState.errorMessage != null && uiState.stations.isEmpty() -> {
                    ErrorState(
                        message = uiState.errorMessage,
                        modifier = Modifier.fillMaxWidth(),
                        onRetry = onRetry,
                    )
                }

                uiState.showNoLocation -> {
                    EmptyState(
                        message = stringResource(R.string.home_no_location_message),
                        hint = stringResource(R.string.home_no_location_hint),
                        modifier = Modifier.fillMaxWidth(),
                    )
                }

                uiState.showDownloadPrompt -> {
                    if (uiState.municipality != null && uiState.state != null) {
                        Text(
                            text = stringResource(
                                R.string.home_location_format,
                                uiState.municipality,
                                uiState.state.abbreviation,
                            ),
                            style = MaterialTheme.typography.headlineSmall,
                            color = MaterialTheme.colorScheme.onBackground,
                        )
                    }
                    EmptyState(
                        message = stringResource(R.string.stations_download_prompt),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Button(
                        onClick = onDownloadStationDetail,
                        modifier = Modifier.fillMaxWidth(),
                        enabled = !uiState.isOffline,
                    ) {
                        Text(text = stringResource(R.string.stations_download_action))
                    }
                }

                uiState.searchNoMatch != null -> {
                    EmptyState(
                        message = stringResource(
                            R.string.stations_search_no_match,
                            uiState.searchNoMatch,
                        ),
                        modifier = Modifier.fillMaxWidth(),
                    )
                }

                uiState.showEmpty -> {
                    EmptyState(
                        message = stringResource(R.string.stations_empty),
                        modifier = Modifier.fillMaxWidth(),
                    )
                }

                uiState.stations.isNotEmpty() -> {
                    if (uiState.error != null) {
                        ErrorState(
                            message = stringResource(AppErrorMapper.toStringRes(uiState.error)),
                            modifier = Modifier.fillMaxWidth(),
                            onRetry = onRetry,
                        )
                    } else if (uiState.errorMessage != null) {
                        ErrorState(
                            message = uiState.errorMessage,
                            modifier = Modifier.fillMaxWidth(),
                            onRetry = onRetry,
                        )
                    }
                    if (uiState.municipality != null && uiState.state != null) {
                        Text(
                            text = stringResource(
                                R.string.home_location_format,
                                uiState.municipality,
                                uiState.state.abbreviation,
                            ),
                            style = MaterialTheme.typography.headlineSmall,
                            color = MaterialTheme.colorScheme.onBackground,
                        )
                    }
                    FuelProductLabel(
                        product = uiState.selectedFuelProduct,
                        modifier = Modifier.padding(top = 4.dp),
                    )
                    Text(
                        text = stringResource(R.string.stations_sort_by_price),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    NearestStationButton(
                        isFindingNearest = uiState.isFindingNearest,
                        onClick = onFindNearestStation,
                        modifier = Modifier.padding(top = 4.dp),
                    )
                    StationsNavigateHintBanner(
                        modifier = Modifier.padding(top = 4.dp, bottom = 8.dp),
                    )
                    uiState.stations.forEach { station ->
                        StationPriceRow(
                            station = station,
                            onNavigate = { onStationSelected(station.cnpjDigits) },
                        )
                    }
                    Text(
                        text = stringResource(R.string.geocoding_osm_attribution),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(top = 12.dp),
                    )
                }
            }

            // P35-T02 — canonical server directory (flag-gated, additive).
            // Legacy ANP rows above are never replaced; an empty server list
            // renders nothing, never an error.
            if (uiState.serverStations.isNotEmpty()) {
                Text(
                    text = stringResource(R.string.server_stations_title),
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.onBackground,
                )
                if (uiState.serverFromCache) {
                    Text(
                        text = stringResource(R.string.server_station_cached),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
                uiState.serverStations.forEach { station ->
                    ServerStationRow(
                        station = station,
                        onSelect = onServerStationSelected,
                    )
                }
                OutlinedButton(
                    onClick = onSuggestStation,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Text(text = stringResource(R.string.suggest_open_action))
                }
            }
            if (uiState.serverError != null && uiState.serverStations.isEmpty()) {
                Text(
                    text = stringResource(R.string.server_stations_unavailable),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }

        uiState.selectedDetail?.let { detail ->
            val locationLabel =
                if (uiState.municipality != null && uiState.state != null) {
                    stringResource(
                        R.string.home_location_format,
                        uiState.municipality,
                        uiState.state.abbreviation,
                    )
                } else {
                    null
                }
            StationDetailSheet(
                detail = detail,
                fuelProduct = uiState.selectedFuelProduct,
                locationLabel = locationLabel,
                onDismiss = onDetailDismissed,
                onRoute = { onNavigateToStation(detail.station.cnpjDigits) },
                onUpdatePrice = onNavigateToUpdatePrice,
            )
        }

        uiState.selectedServerStation?.let { serverStation ->
            val fuelWire = com.anpfuel.data.mapper.WireFuelMapper.toWire(
                uiState.selectedFuelProduct,
            )
            ServerStationDetailSheet(
                station = serverStation,
                fromCache = uiState.serverDetailFromCache,
                onDismiss = onServerDetailDismissed,
                onRoute = onServerStationNavigate,
                onUpdatePrice = { onServerUpdatePrice(serverStation, fuelWire) },
                fuelProductWire = fuelWire,
                accountId = uiState.serverAccountId,
            )
        }
    }
}

@Composable
private fun NearestStationButton(
    isFindingNearest: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Button(
        onClick = onClick,
        modifier = modifier.fillMaxWidth(),
        enabled = !isFindingNearest,
    ) {
        if (isFindingNearest) {
            CircularProgressIndicator(
                modifier = Modifier.size(18.dp),
                strokeWidth = 2.dp,
                color = MaterialTheme.colorScheme.onPrimary,
            )
        } else {
            Icon(
                imageVector = Icons.Default.MyLocation,
                contentDescription = null,
            )
        }
        Spacer(modifier = Modifier.width(8.dp))
        Text(
            text = stringResource(R.string.stations_nearest_action),
            style = MaterialTheme.typography.labelLarge,
        )
    }
}

@Preview(showBackground = true)
@Composable
private fun StationsScreenPreview() {
    AnpFuelTheme {
        StationsContent(
            uiState = StationsUiState(
                isLoading = false,
                municipality = "Curitiba",
                state = BrazilianState.PARANA,
                stations = listOf(
                    StationPriceUiModel(
                        cnpjDigits = "12345678000195",
                        displayName = "Posto Centro",
                        brand = "BR",
                        address = "Rua XV de Novembro, 1000",
                        priceFormatted = "R$ 5,79",
                        collectedAtLabel = "Jun 10, 2026",
                        navigationQuery = "Rua XV de Novembro, 1000, Curitiba - PR, Brazil",
                    ),
                ),
            ),
            onNavigateBack = {},
            onFuelProductSelected = {},
            onSearchQueryChanged = {},
            onFindNearestStation = {},
            onDownloadStationDetail = {},
            onRetry = {},
            onWeekChanged = {},
            onStationSelected = {},
            onNavigateToStation = {},
            onDetailDismissed = {},
            onNavigateToUpdatePrice = {},
        )
    }
}
