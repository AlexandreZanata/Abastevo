package com.anpfuel.app.ui.stations

import android.Manifest
import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.wrapContentWidth
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.MyLocation
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.repeatOnLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.AppErrorMapper
import com.anpfuel.app.navigation.MapAppChooser
import com.anpfuel.app.navigation.MapNavigationResult
import com.anpfuel.app.ui.components.EmptyState
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.components.FuelProductSelectorButton
import com.anpfuel.app.ui.components.FuelProductSelectorDialog
import com.anpfuel.app.ui.components.StationsNavigateHintBanner
import com.anpfuel.app.ui.components.OfflineBanner
import com.anpfuel.app.ui.components.SkeletonCard
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.components.SkeletonLine
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
    onOpenMasterExplore: () -> Unit = {},
    onStationProfile: (String) -> Unit = {},
    onOpenStation: (String, FuelProduct) -> Unit = { _, _ -> },
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

    val lifecycleOwner = LocalLifecycleOwner.current
    LaunchedEffect(viewModel, lifecycleOwner) {
        lifecycleOwner.lifecycle.repeatOnLifecycle(Lifecycle.State.RESUMED) {
            viewModel.syncServerStationsWhileVisible()
        }
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
        onStationSelected = { onOpenStation(it, uiState.selectedFuelProduct) },
        onNavigateToStation = viewModel::onNavigateToStation,
        onDetailDismissed = viewModel::onDetailDismissed,
        onNavigateToUpdatePrice = onNavigateToUpdatePrice,
        onServerStationSelected = { onOpenStation(it, uiState.selectedFuelProduct) },
        onServerDetailDismissed = viewModel::onServerDetailDismissed,
        onServerStationNavigate = viewModel::onServerStationNavigate,
        onServerUpdatePrice = { station, fuelWire ->
            onNavigateToUpdatePriceWithTarget(station.stationId, fuelWire)
        },
        onSuggestStation = onSuggestStation,
        onOpenMasterExplore = onOpenMasterExplore,
        onStationProfile = onStationProfile,
        onLoadMoreServerStations = viewModel::loadMoreServerStations,
        onLoadMoreAnpStations = viewModel::loadMoreAnpStations,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun StationsContent(
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
    onOpenMasterExplore: () -> Unit = {},
    onStationProfile: (String) -> Unit = {},
    onLoadMoreServerStations: () -> Unit = {},
    onLoadMoreAnpStations: () -> Unit = {},
    modifier: Modifier = Modifier,
    weekPickerAction: @Composable () -> Unit = { SurveyWeekChipAction(onWeekChanged = onWeekChanged) },
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
                actions = { weekPickerAction() },
            )
        },
    ) { innerPadding ->
        // Single virtualized list for headers and rows: ANP rows keyed by
        // origin ("anp:") and directory rows by origin ("dir:") so item
        // identity survives recomposition, filtering and refresh. No nested
        // scrollable lives inside this list.
        val anpStations = uiState.stations
        val directoryStations = uiState.directoryOnlyStations
        LazyColumn(
            state = rememberLazyListState(),
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding),
            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            if (uiState.isOffline) {
                item(key = StationsListKeys.OFFLINE_BANNER) {
                    OfflineBanner()
                }
            }

            item(key = StationsListKeys.FUEL_LABEL) {
                Text(
                    text = stringResource(R.string.history_fuel_product_label),
                    style = MaterialTheme.typography.titleSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            item(key = StationsListKeys.FUEL_SELECTOR) {
                var fuelPickerOpen by rememberSaveable { mutableStateOf(false) }
                FuelProductSelectorButton(
                    selected = uiState.selectedFuelProduct,
                    onClick = { fuelPickerOpen = true },
                    modifier = Modifier.fillMaxWidth(),
                    enabled = !uiState.isDownloading,
                )
                if (fuelPickerOpen) {
                    FuelProductSelectorDialog(
                        selected = uiState.selectedFuelProduct,
                        onSelect = { product ->
                            fuelPickerOpen = false
                            onFuelProductSelected(product)
                        },
                        onDismiss = { fuelPickerOpen = false },
                    )
                }
            }

            if (!uiState.showNoLocation && !uiState.showDownloadPrompt) {
                item(key = StationsListKeys.SEARCH) {
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
            }

            if (uiState.municipality != null && uiState.state != null) {
                item(key = StationsListKeys.SUMMARY) {
                    StationsListSummary(
                        municipality = uiState.municipality,
                        state = uiState.state,
                        fuelProduct = uiState.selectedFuelProduct,
                    )
                }
            }

            when {
                // P20-T02: a failed refresh keeps cached stations, so loading
                // and error replace the list only when there is nothing cached.
                (uiState.isLoading || uiState.isDownloading) && anpStations.isEmpty() -> {
                    item(key = StationsListKeys.LOADING) {
                        StationsLoadingSkeleton()
                    }
                }

                uiState.error != null && anpStations.isEmpty() -> {
                    item(key = StationsListKeys.ERROR) {
                        ErrorState(
                            message = stringResource(AppErrorMapper.toStringRes(uiState.error)),
                            modifier = Modifier.fillMaxWidth(),
                            onRetry = onRetry,
                        )
                    }
                }

                uiState.errorMessage != null && anpStations.isEmpty() -> {
                    item(key = StationsListKeys.ERROR) {
                        ErrorState(
                            message = uiState.errorMessage,
                            modifier = Modifier.fillMaxWidth(),
                            onRetry = onRetry,
                        )
                    }
                }

                uiState.showNoLocation -> {
                    item(key = StationsListKeys.EMPTY_STATE) {
                        EmptyState(
                            message = stringResource(R.string.home_no_location_message),
                            hint = stringResource(R.string.home_no_location_hint),
                            modifier = Modifier.fillMaxWidth(),
                        )
                    }
                }

                uiState.showDownloadPrompt -> {
                    item(key = StationsListKeys.EMPTY_STATE) {
                        EmptyState(
                            message = stringResource(R.string.stations_download_prompt),
                            modifier = Modifier.fillMaxWidth(),
                        )
                    }
                    item(key = StationsListKeys.DOWNLOAD_ACTION) {
                        Button(
                            onClick = onDownloadStationDetail,
                            modifier = Modifier.fillMaxWidth(),
                            enabled = !uiState.isOffline,
                        ) {
                            Text(text = stringResource(R.string.stations_download_action))
                        }
                    }
                }

                uiState.searchNoMatch != null -> {
                    item(key = StationsListKeys.EMPTY_STATE) {
                        EmptyState(
                            message = stringResource(
                                R.string.stations_search_no_match,
                                uiState.searchNoMatch,
                            ),
                            modifier = Modifier.fillMaxWidth(),
                        )
                    }
                }

                uiState.showEmpty -> {
                    item(key = StationsListKeys.EMPTY_STATE) {
                        EmptyState(
                            message = stringResource(R.string.stations_empty),
                            modifier = Modifier.fillMaxWidth(),
                        )
                    }
                }

                anpStations.isNotEmpty() -> {
                    if (uiState.error != null) {
                        item(key = StationsListKeys.LIST_ERROR) {
                            ErrorState(
                                message = stringResource(AppErrorMapper.toStringRes(uiState.error)),
                                modifier = Modifier.fillMaxWidth(),
                                onRetry = onRetry,
                            )
                        }
                    } else if (uiState.errorMessage != null) {
                        item(key = StationsListKeys.LIST_ERROR) {
                            ErrorState(
                                message = uiState.errorMessage,
                                modifier = Modifier.fillMaxWidth(),
                                onRetry = onRetry,
                            )
                        }
                    }
                    item(key = StationsListKeys.NEAREST_BUTTON) {
                        NearestStationButton(
                            isFindingNearest = uiState.isFindingNearest,
                            onClick = onFindNearestStation,
                            modifier = Modifier.padding(top = 4.dp),
                        )
                    }
                    item(key = StationsListKeys.NAVIGATE_HINT) {
                        StationsNavigateHintBanner(
                            modifier = Modifier.padding(top = 4.dp, bottom = 8.dp),
                        )
                    }
                    items(anpStations, key = { StationsListKeys.anpRow(it.cnpjDigits) }) { station ->
                        StationPriceRow(
                            station = station,
                            onNavigate = { onStationSelected(station.cnpjDigits) },
                        )
                    }
                    if (uiState.isAnpLoadingMore) {
                        item(key = StationsListKeys.ANP_MORE_LOADING) {
                            CircularProgressIndicator(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(vertical = 4.dp)
                                    .wrapContentWidth(Alignment.CenterHorizontally)
                                    .size(24.dp),
                                strokeWidth = 2.dp,
                            )
                        }
                    } else if (uiState.anpMoreError != null) {
                        item(key = StationsListKeys.ANP_MORE_ERROR) {
                            Text(
                                text = stringResource(R.string.server_stations_load_more_failed),
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.error,
                            )
                        }
                        item(key = StationsListKeys.ANP_MORE_RETRY) {
                            OutlinedButton(
                                onClick = onLoadMoreAnpStations,
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                Text(text = stringResource(R.string.action_retry))
                            }
                        }
                    } else if (uiState.anpHasMore) {
                        item(key = StationsListKeys.ANP_MORE) {
                            OutlinedButton(
                                onClick = onLoadMoreAnpStations,
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                Text(text = stringResource(R.string.feed_load_more))
                            }
                        }
                    }
                    item(key = StationsListKeys.ATTRIBUTION) {
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
            }

            // Supplement the visible weekly price rows using full CNPJ identity.
            // Keep the complete cached directory in state; filtering is presentation only.
            if (directoryStations.isNotEmpty() || uiState.serverNextCursor != null || uiState.serverNextCacheOffset != null || uiState.isServerLoadingMore || uiState.serverMoreError != null) {
                item(key = StationsListKeys.DIRECTORY_TITLE) {
                    Text(
                        text = stringResource(R.string.server_stations_title),
                        style = MaterialTheme.typography.titleMedium,
                        color = MaterialTheme.colorScheme.onBackground,
                        modifier = Modifier.semantics { heading() },
                    )
                }
                if (uiState.serverFromCache) {
                    item(key = StationsListKeys.DIRECTORY_CACHE_NOTE) {
                        Text(
                            text = stringResource(R.string.server_station_cached),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.error,
                        )
                    }
                }
                items(directoryStations, key = { StationsListKeys.directoryRow(it.stationId) }) { station ->
                    ServerStationRow(
                        station = station,
                        onSelect = onServerStationSelected,
                    )
                }
                if (uiState.isServerLoadingMore) {
                    item(key = StationsListKeys.DIRECTORY_MORE_LOADING) {
                        CircularProgressIndicator(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(vertical = 4.dp)
                                .wrapContentWidth(Alignment.CenterHorizontally)
                                .size(24.dp),
                            strokeWidth = 2.dp,
                        )
                    }
                } else if (uiState.serverMoreError != null) {
                    item(key = StationsListKeys.DIRECTORY_MORE_ERROR) {
                        Text(
                            text = stringResource(R.string.server_stations_load_more_failed),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.error,
                        )
                    }
                    item(key = StationsListKeys.DIRECTORY_MORE_RETRY) {
                        OutlinedButton(
                            onClick = onLoadMoreServerStations,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(text = stringResource(R.string.action_retry))
                        }
                    }
                } else if (uiState.serverNextCursor != null || uiState.serverNextCacheOffset != null) {
                    item(key = StationsListKeys.DIRECTORY_MORE) {
                        OutlinedButton(
                            onClick = onLoadMoreServerStations,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(text = stringResource(R.string.server_stations_load_more))
                        }
                    }
                }
                item(key = StationsListKeys.SUGGEST_ACTION) {
                    OutlinedButton(
                        onClick = onSuggestStation,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = stringResource(R.string.suggest_open_action))
                    }
                }
                item(key = "master_explore") {
                    OutlinedButton(
                        onClick = onOpenMasterExplore,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = stringResource(R.string.master_explore_open))
                    }
                }
            }
            if (uiState.serverError != null && uiState.serverStations.isEmpty()) {
                item(key = StationsListKeys.SERVER_ERROR) {
                    Text(
                        text = stringResource(R.string.server_stations_unavailable),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

/**
 * Stable list keys for the stations screen: headers use fixed slots and
 * rows are namespaced by origin, so an ANP row and a directory row never
 * share identity even for the same physical station.
 */
internal object StationsListKeys {
    const val OFFLINE_BANNER = "offline"
    const val FUEL_LABEL = "fuel-label"
    const val FUEL_SELECTOR = "fuel-selector"
    const val SEARCH = "search"
    const val SUMMARY = "summary"
    const val LOADING = "loading"
    const val ERROR = "error"
    const val EMPTY_STATE = "state"
    const val DOWNLOAD_ACTION = "download-action"
    const val LIST_ERROR = "list-error"
    const val NEAREST_BUTTON = "nearest"
    const val NAVIGATE_HINT = "navigate-hint"
    const val ATTRIBUTION = "attribution"
    const val DIRECTORY_TITLE = "directory-title"
    const val ANP_MORE = "anp-more"
    const val ANP_MORE_LOADING = "anp-more-loading"
    const val ANP_MORE_ERROR = "anp-more-error"
    const val ANP_MORE_RETRY = "anp-more-retry"
    const val DIRECTORY_MORE = "directory-more"
    const val DIRECTORY_MORE_LOADING = "directory-more-loading"
    const val DIRECTORY_MORE_ERROR = "directory-more-error"
    const val DIRECTORY_MORE_RETRY = "directory-more-retry"
    const val DIRECTORY_CACHE_NOTE = "directory-cache-note"
    const val SUGGEST_ACTION = "suggest"
    const val SERVER_ERROR = "server-error"

    fun anpRow(cnpjDigits: String) = "anp:$cnpjDigits"
    fun directoryRow(stationId: String) = "dir:$stationId"
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

@Composable
private fun StationsLoadingSkeleton() {
    SkeletonGroup(modifier = Modifier.fillMaxWidth()) {
        SkeletonLine(width = 220.dp, height = 28.dp)
        repeat(4) { SkeletonCard(height = 96.dp) }
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
