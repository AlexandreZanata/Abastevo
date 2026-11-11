package com.anpfuel.app.ui.home

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.ScrollState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.Map
import androidx.compose.material.icons.filled.FilterList
import androidx.compose.material.icons.outlined.DirectionsCar
import androidx.compose.material.icons.outlined.History
import androidx.compose.material.icons.outlined.Groups
import androidx.compose.material.icons.outlined.LocalGasStation
import androidx.compose.material.icons.outlined.LocationOn
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.AssistChipDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.RadioButton
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.SnackbarDuration
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.designsystem.AbastevoSelectionDefaults
import com.anpfuel.app.ui.designsystem.AssistChip
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.header.AbastevoHeader
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.Alignment
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.repeatOnLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.AppErrorMapper
import com.anpfuel.app.mapper.LastSyncFormatter
import com.anpfuel.app.mapper.SurveyWeekFormatter
import com.anpfuel.app.navigation.Routes
import com.anpfuel.app.ui.components.Br010EmptyState
import com.anpfuel.app.ui.components.EmptyState
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.components.OfflineBanner
import com.anpfuel.app.ui.components.SkeletonButton
import com.anpfuel.app.ui.components.SkeletonCard
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.components.SkeletonLine
import com.anpfuel.app.ui.components.SyncStatusBanner
import com.anpfuel.app.ui.accessibility.liveRegionSemantics
import com.anpfuel.app.ui.components.TankFillCostCard
import com.anpfuel.app.ui.model.AveragePriceUiModel
import com.anpfuel.app.ui.model.TankFillCostEstimateUiModel
import com.anpfuel.app.ui.theme.AbastevoSpacing
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.state.DataReadinessState
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import java.util.Locale

@Composable
fun HomeScreen(
    darkTheme: Boolean,
    onNavigate: (String) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: HomeViewModel = hiltViewModel(),
    onRouteToStation: (String, String) -> Unit = { _, _ -> },
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val locale = LocalConfiguration.current.locales[0]
    val lifecycleOwner = LocalLifecycleOwner.current

    LaunchedEffect(lifecycleOwner, locale, uiState.priceSource) {
        lifecycleOwner.lifecycle.repeatOnLifecycle(Lifecycle.State.RESUMED) {
            if(uiState.priceSource==HomePriceSource.ANP) viewModel.load(locale)
            else {
                val session=viewModel.startCommunitySession()
                try { while(true) { viewModel.refreshCommunity(periodic = true);kotlinx.coroutines.delay(viewModel.communityAutoRefreshDelayMillis()) } }
                finally {viewModel.stopCommunityRefresh(session)}
            }
        }
    }

    LaunchedEffect(uiState.priceSource, uiState.communityExpiresAt) {
        val expiry = uiState.communityExpiresAt
        if (uiState.priceSource == HomePriceSource.COMMUNITY && expiry != null) {
            val remaining = java.time.Duration.between(java.time.Instant.now(), expiry).toMillis().coerceAtLeast(0)
            kotlinx.coroutines.delay(remaining)
            viewModel.expireCommunity(expiry)
        }
    }

    HomeContent(
        uiState = uiState,
        darkTheme = darkTheme,
        onNavigate = onNavigate,
        onRefresh = { viewModel.refresh(locale) },
        onRetry = { viewModel.load(locale, showLoadingIndicator = true) },
        onWeekChanged = { viewModel.load(locale, showLoadingIndicator = true) },
        onPriceSortChanged = viewModel::onPriceSortChanged,
        onDismissBenefits = viewModel::dismissBenefits,
        onPriceSourceChanged = { viewModel.selectPriceSource(it, locale) },
        onSyncNoticeShown = viewModel::onSyncNoticeShown,
        onRouteToStation = onRouteToStation,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
internal fun HomeContent(
    uiState: HomeUiState,
    darkTheme: Boolean,
    onNavigate: (String) -> Unit,
    onRefresh: () -> Unit,
    onRetry: () -> Unit,
    onWeekChanged: () -> Unit,
    modifier: Modifier = Modifier,
    includeSurveyWeekChip: Boolean = true,
    onPriceSortChanged: (HomePriceSortOrder) -> Unit = {},
    onDismissBenefits: () -> Unit = {},
    onPriceSourceChanged: (HomePriceSource) -> Unit = {},
    onSyncNoticeShown: () -> Unit = {},
    onRouteToStation: (String, String) -> Unit = { _, _ -> },
) {
    val snackbarHostState = remember { SnackbarHostState() }
    val noticeText = uiState.syncNotice?.let { syncNoticeText(it, uiState) }
    LaunchedEffect(uiState.syncNotice) {
        if (noticeText != null) {
            // Read once, then gone: a verified week leaves nothing on screen.
            snackbarHostState.showSnackbar(noticeText, duration = SnackbarDuration.Long)
            onSyncNoticeShown()
        }
    }
    val scrollState = rememberSaveable(saver = ScrollState.Saver) {
        ScrollState(0)
    }
    var filterOpen by rememberSaveable { mutableStateOf(false) }
    if (filterOpen) {
        HomePriceSortDialog(
            current = uiState.priceSort,
            onSelect = { order ->
                filterOpen = false
                onPriceSortChanged(order)
            },
            onDismiss = { filterOpen = false },
        )
    }

    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        snackbarHost = { SnackbarHost(snackbarHostState) },
        topBar = {
            val municipality = uiState.municipality
            val state = uiState.state
            AbastevoHeader(
                onNotificationsClick = { onNavigate(Routes.SETTINGS) },
                onProfileClick = { onNavigate(Routes.PROFILE) },
                locationLabel = if (municipality != null && state != null) {
                    stringResource(R.string.home_location_format, municipality, state.abbreviation)
                } else {
                    null
                },
                onLocationClick = { onNavigate(Routes.LOCATION) },
                onWeekChanged = if (includeSurveyWeekChip && uiState.priceSource == HomePriceSource.ANP) onWeekChanged else null,
            )
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .verticalScroll(scrollState)
                .padding(horizontal = AbastevoSpacing.screenGutter, vertical = AbastevoSpacing.md),
            verticalArrangement = Arrangement.spacedBy(AbastevoSpacing.md),
        ) {
            // A verified STALE week stays quiet: the user already synced and
            // ANP has nothing newer; the transient notice explained it once.
            if (uiState.priceSource == HomePriceSource.ANP && !uiState.isStaleVerified) {
                SyncStatusBanner(readiness = uiState.readiness)
            }

            if (uiState.isOffline && (uiState.hasCachedData || uiState.priceSource == HomePriceSource.COMMUNITY)) {
                OfflineBanner()
            }

            if (uiState.priceSource == HomePriceSource.ANP && !uiState.isRefreshing) {
                uiState.syncError?.let { syncError ->
                    Text(
                        text = stringResource(AppErrorMapper.toStringRes(syncError)),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                        textAlign = TextAlign.Center,
                        modifier = Modifier
                            .fillMaxWidth()
                            .testTag("home_sync_error")
                            .liveRegionSemantics(),
                    )
                }
            }

            if (uiState.priceSource == HomePriceSource.ANP && !uiState.isStaleVerified &&
                (uiState.readiness == DataReadinessState.STALE || uiState.readiness == DataReadinessState.ERROR)
            ) {
                when {
                    uiState.isRefreshing -> {
                        TextButton(
                            onClick = onRefresh,
                            enabled = false,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(text = stringResource(R.string.banner_syncing))
                        }
                    }
                    else -> {
                        TextButton(
                            onClick = onRefresh,
                            enabled = true,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(text = stringResource(R.string.onboarding_action_sync_now))
                        }
                    }
                }
            }

            if (uiState.tankFillCostEstimates.isNotEmpty()) {
                VehicleCarousel(
                    estimates = uiState.tankFillCostEstimates,
                    onNavigate = onNavigate,
                    onRouteToStation = onRouteToStation,
                )
            } else if (uiState.isLoading && uiState.hasVehicle) {
                VehicleCarouselSkeleton()
            } else {
                HomeCommunityHero(onContribute = { onNavigate(Routes.CAPTURE) })
            }
            if (uiState.showBenefits) {
                HomeBenefits(onDismiss = onDismissBenefits)
            }

            SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth().testTag("home_price_source")) {
                HomePriceSource.entries.forEachIndexed { index, source ->
                    SegmentedButton(
                        selected = uiState.priceSource == source,
                        onClick = { onPriceSourceChanged(source) },
                        modifier = Modifier.heightIn(min = AbastevoSpacing.controlMinHeight),
                        shape = SegmentedButtonDefaults.itemShape(index = index, count = HomePriceSource.entries.size),
                        colors = AbastevoSelectionDefaults.segmentedButtonColors(),
                        label = {
                            Text(stringResource(if (source == HomePriceSource.ANP) R.string.home_source_anp else R.string.home_source_community))
                        },
                    )
                }
            }
            when {
                uiState.priceSource==HomePriceSource.COMMUNITY -> {
                    Text(stringResource(R.string.home_community_prices_title),style=MaterialTheme.typography.titleLarge,fontWeight=FontWeight.Bold)
                    HomePriceFilterButton(uiState.priceSort,onOpenFilters={ filterOpen=true })
                    when {
                        uiState.communityFailed && uiState.communityPrices.isEmpty() -> ErrorState(stringResource(R.string.home_community_unavailable),onRetry=onRetry)
                        !uiState.hasLocation && !uiState.communityLoading -> { EmptyState(stringResource(R.string.home_no_location_message),hint=stringResource(R.string.home_no_location_hint));RowActions(onNavigate) }
                        uiState.communityLoading && uiState.communityPrices.isEmpty() -> HomeLoadingSkeleton()
                        uiState.communityPrices.isEmpty() -> EmptyState(
                            stringResource(R.string.home_community_empty),
                            hint = stringResource(R.string.home_community_empty_hint),
                            icon = Icons.Outlined.Groups,
                            actionLabel = stringResource(R.string.community_action_contribute),
                            onAction = { onNavigate(Routes.CAPTURE) },
                        )
                        else -> {
                            if(uiState.communityLoading) androidx.compose.material3.LinearProgressIndicator(Modifier.fillMaxWidth())
                            uiState.communityUpdatedAt?.let { at -> Text(stringResource(R.string.home_community_updated,
                                com.anpfuel.app.mapper.LastSyncFormatter.format(at, LocalConfiguration.current.locales[0]).orEmpty()),style=MaterialTheme.typography.bodySmall) }
                            if (uiState.communityFailed || uiState.communityStale) {
                                Text(stringResource(R.string.feed_cached_hint),style=MaterialTheme.typography.bodySmall,
                                    color=MaterialTheme.colorScheme.onSurfaceVariant)
                                if (uiState.communityFailed) {
                                    TextButton(onClick=onRetry) { Text(stringResource(R.string.feed_retry)) }
                                }
                            }
                            uiState.communityPrices.forEach { price -> HomeReferencePriceCard(price,darkTheme,
                                onClick={onNavigate(Routes.COMMUNITY)},priceSort=uiState.priceSort,community=true) }
                        }
                    }
                }
                uiState.isLoading -> HomeLoadingSkeleton()

                uiState.error != null -> {
                    ErrorState(
                        message = stringResource(AppErrorMapper.toStringRes(uiState.error)),
                        modifier = Modifier.fillMaxWidth(),
                        onRetry = onRetry,
                    )
                }

                !uiState.hasCachedData -> {
                    EmptyState(
                        message = stringResource(R.string.home_empty_message),
                        hint = stringResource(R.string.home_empty_hint),
                        modifier = Modifier.fillMaxWidth(),
                    )
                }

                !uiState.hasLocation -> {
                    EmptyState(
                        message = stringResource(R.string.home_no_location_message),
                        hint = stringResource(R.string.home_no_location_hint),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    RowActions(onNavigate = onNavigate)
                }

                uiState.isEmptyMunicipality -> {
                    Br010EmptyState(
                        dataAvailability = uiState.dataAvailability ?: DataAvailability.NO_DATA_THIS_WEEK,
                        operationalNote = uiState.operationalNote,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    RowActions(onNavigate = onNavigate)
                }

                else -> {
                    Text(
                        stringResource(R.string.home_reference_prices_title),
                        modifier = Modifier.fillMaxWidth(),
                        style = MaterialTheme.typography.titleLarge,
                        fontWeight = FontWeight.Bold,
                    )
                    PriceMetadata(uiState = uiState)
                    HomePriceFilterButton(
                        current = uiState.priceSort,
                        onOpenFilters = { filterOpen = true },
                    )
                    uiState.prices.forEach { price ->
                        HomeReferencePriceCard(
                            price = price,
                            darkTheme = darkTheme,
                            onClick = { onNavigate(Routes.stations(price.fuelProduct)) },
                            priceSort = uiState.priceSort,
                        )
                    }
                    Button(
                        onClick = { onNavigate(Routes.MAP) },
                        modifier = Modifier.fillMaxWidth().defaultMinSize(minHeight = 56.dp),
                    ) {
                        Icon(Icons.Default.Map, contentDescription = null, modifier = Modifier.size(20.dp))
                        Text(
                            text = stringResource(R.string.home_open_map),
                            modifier = Modifier.weight(1f).padding(start = 8.dp),
                        )
                        Icon(Icons.AutoMirrored.Filled.ArrowForward, contentDescription = null, modifier = Modifier.size(20.dp))
                    }
                    RowActions(onNavigate = onNavigate)
                }
            }
        }
    }
}

/**
 * Quick price filter entry: current order stays visible, options open in
 * a single-choice dialog and apply instantly without reloading.
 */
@Composable
private fun HomePriceFilterButton(
    current: HomePriceSortOrder,
    onOpenFilters: () -> Unit,
    modifier: Modifier = Modifier,
) {
    // Same container as the fuel cards: white fill, shared shape and outline.
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        onClick = onOpenFilters,
        modifier = modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Icon(
                imageVector = Icons.Default.FilterList,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = stringResource(R.string.home_filter_button),
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.weight(1f),
            )
            Text(
                text = stringResource(current.labelRes),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Icon(
                imageVector = Icons.Default.ChevronRight,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.outline,
            )
        }
    }
}

@Composable
private fun HomePriceSortDialog(
    current: HomePriceSortOrder,
    onSelect: (HomePriceSortOrder) -> Unit,
    onDismiss: () -> Unit,
) {
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false),
    ) {
        Surface(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 24.dp),
            shape = MaterialTheme.shapes.extraLarge,
            tonalElevation = 6.dp,
        ) {
            Column(
                modifier = Modifier.padding(horizontal = 8.dp, vertical = 16.dp),
                verticalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                Text(
                    text = stringResource(R.string.home_filter_title),
                    style = MaterialTheme.typography.titleLarge,
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                )
                HomePriceSortOrder.entries.forEach { order ->
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(12.dp))
                            .clickable(
                                role = Role.RadioButton,
                                onClick = { onSelect(order) },
                            )
                            .padding(horizontal = 16.dp, vertical = 14.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                    ) {
                        RadioButton(
                            selected = order == current,
                            onClick = null,
                        )
                        Text(
                            text = stringResource(order.labelRes),
                            style = MaterialTheme.typography.bodyLarge,
                        )
                    }
                }
            }
        }
    }
}

/**
 * Placeholder mirroring a tank-fill card while loading for drivers with
 * a registered vehicle, so the hero banner never flashes first.
 */
@Composable
private fun VehicleCarouselSkeleton(modifier: Modifier = Modifier) {
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = modifier
            .fillMaxWidth()
            .defaultMinSize(minHeight = 112.dp),
        colors = CardDefaults.cardColors(
            containerColor = containerColor,
        ),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            SkeletonLine(width = 150.dp, height = 14.dp)
            SkeletonLine(width = 190.dp, height = 32.dp)
            SkeletonLine(width = 170.dp, height = 20.dp)
            SkeletonLine(modifier = Modifier.padding(top = 4.dp), height = 44.dp)
        }
    }
}

@Composable
private fun HomeLoadingSkeleton() {
    SkeletonGroup(modifier = Modifier.fillMaxWidth()) {
        SkeletonLine(width = 200.dp, height = 24.dp)
        SkeletonLine(width = 260.dp)
        SkeletonCard(height = 104.dp)
        SkeletonCard(height = 104.dp)
        SkeletonButton()
    }
}

@Composable
private fun VehicleCarousel(
    estimates: List<TankFillCostEstimateUiModel>,
    onNavigate: (String) -> Unit,
    onRouteToStation: (String, String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val screenWidth = LocalConfiguration.current.screenWidthDp.dp
    // A single vehicle lines up with the cards below; several peek the next one.
    val cardWidth = if (estimates.size == 1) {
        screenWidth - AbastevoSpacing.screenGutter * 2
    } else {
        screenWidth * 0.85f
    }

    LazyRow(
        modifier = modifier.fillMaxWidth().testTag("home_vehicle_carousel"),
        horizontalArrangement = Arrangement.spacedBy(AbastevoSpacing.md),
    ) {
        items(estimates, key = { it.vehicleId.value }) { estimate ->
            TankFillCostCard(
                estimate = estimate,
                onClick = { onNavigate(Routes.VEHICLES) },
                onGoToStation = estimate.stationNavigationQuery?.let { query ->
                    { onRouteToStation(query, estimate.stationDisplayName.orEmpty()) }
                },
                modifier = Modifier.width(cardWidth),
            )
        }
    }
}

@Composable
private fun PriceMetadata(uiState: HomeUiState) {
    uiState.surveyWeek?.let { week ->
        val locale = LocalConfiguration.current.locales[0]
        Text(
            text = stringResource(
                R.string.home_reference_period,
                SurveyWeekFormatter.formatRange(week, locale),
            ),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun syncNoticeText(notice: HomeSyncNotice, uiState: HomeUiState): String = when (notice) {
    HomeSyncNotice.UPDATED -> stringResource(R.string.sync_completed)
    HomeSyncNotice.VERIFIED_NO_NEW_WEEK -> {
        val locale = LocalConfiguration.current.locales[0]
        val weekLabel = uiState.surveyWeek?.let { SurveyWeekFormatter.formatRange(it, locale) }.orEmpty()
        val timeLabel = LastSyncFormatter.format(
            java.time.Instant.ofEpochMilli(uiState.lastVerifiedAtMillis ?: System.currentTimeMillis()),
            locale,
        ).orEmpty()
        stringResource(R.string.home_stale_verified, timeLabel, weekLabel)
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun RowActions(onNavigate: (String) -> Unit) {
    FlowRow(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        HomeShortcutChip(R.string.nav_location, Icons.Outlined.LocationOn) { onNavigate(Routes.LOCATION) }
        HomeShortcutChip(R.string.nav_history, Icons.Outlined.History) { onNavigate(Routes.HISTORY) }
        HomeShortcutChip(R.string.nav_stations, Icons.Outlined.LocalGasStation) { onNavigate(Routes.STATIONS) }
        HomeShortcutChip(R.string.nav_vehicles, Icons.Outlined.DirectionsCar) { onNavigate(Routes.VEHICLES) }
        HomeShortcutChip(R.string.nav_settings, Icons.Outlined.Settings) { onNavigate(Routes.SETTINGS) }
    }
}

@Composable
private fun HomeShortcutChip(labelRes: Int, icon: ImageVector, onClick: () -> Unit) {
    AssistChip(
        onClick = onClick,
        label = { Text(text = stringResource(labelRes)) },
        leadingIcon = {
            Icon(
                imageVector = icon,
                contentDescription = null,
                modifier = Modifier.size(AssistChipDefaults.IconSize),
            )
        },
    )
}

@Preview(showBackground = true, name = "Home with prices")
@Composable
private fun HomeWithPricesPreview() {
    AnpFuelTheme {
        HomeContent(
            uiState = HomeUiState(
                isLoading = false,
                readiness = DataReadinessState.READY,
                hasCachedData = true,
                hasLocation = true,
                municipality = "Curitiba",
                state = BrazilianState.PARANA,
                surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"),
                prices = listOf(
                    AveragePriceUiModel(
                        fuelProduct = FuelProduct.ETHANOL,
                        averageFormatted = "R$ 3,42",
                        minimumFormatted = "R$ 3,10",
                        maximumFormatted = "R$ 3,80",
                        stationCount = 42,
                    ),
                ),
            ),
            darkTheme = false,
            onNavigate = {},
            onRefresh = {},
            onRetry = {},
            onWeekChanged = {},
        )
    }
}
