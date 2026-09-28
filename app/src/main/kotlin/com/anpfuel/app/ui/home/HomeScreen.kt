package com.anpfuel.app.ui.home

import android.widget.Toast
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.ScrollState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.DarkMode
import androidx.compose.material.icons.filled.LightMode
import androidx.compose.material3.AssistChip
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.repeatOnLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.AppErrorMapper
import com.anpfuel.app.mapper.SurveyWeekFormatter
import com.anpfuel.app.navigation.MapAppChooser
import com.anpfuel.app.navigation.MapNavigationResult
import com.anpfuel.app.navigation.Routes
import com.anpfuel.app.ui.components.AnpAttributionFooter
import com.anpfuel.app.ui.components.Br010EmptyState
import com.anpfuel.app.ui.components.EmptyState
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.components.FuelPriceCard
import com.anpfuel.app.ui.components.LoadingState
import com.anpfuel.app.ui.components.OfflineBanner
import com.anpfuel.app.ui.components.SyncStatusBanner
import com.anpfuel.app.ui.components.TankFillCostCard
import com.anpfuel.app.ui.components.TankFillCostPlaceholderCard
import com.anpfuel.app.ui.weekpicker.SurveyWeekChipAction
import com.anpfuel.app.ui.model.AveragePriceUiModel
import com.anpfuel.app.ui.model.TankFillCostEstimateUiModel
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
    onToggleTheme: () -> Unit,
    onNavigate: (String) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: HomeViewModel = hiltViewModel(),
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val locale = LocalConfiguration.current.locales[0]
    val lifecycleOwner = LocalLifecycleOwner.current

    LaunchedEffect(lifecycleOwner, locale) {
        lifecycleOwner.lifecycle.repeatOnLifecycle(Lifecycle.State.RESUMED) {
            viewModel.load(locale)
        }
    }

    HomeContent(
        uiState = uiState,
        darkTheme = darkTheme,
        onToggleTheme = onToggleTheme,
        onNavigate = onNavigate,
        onRefresh = { viewModel.refresh(locale) },
        onRetry = { viewModel.load(locale, showLoadingIndicator = true) },
        onWeekChanged = { viewModel.load(locale, showLoadingIndicator = true) },
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
internal fun HomeContent(
    uiState: HomeUiState,
    darkTheme: Boolean,
    onToggleTheme: () -> Unit,
    onNavigate: (String) -> Unit,
    onRefresh: () -> Unit,
    onRetry: () -> Unit,
    onWeekChanged: () -> Unit,
    modifier: Modifier = Modifier,
    includeSurveyWeekChip: Boolean = true,
) {
    val context = LocalContext.current
    val scrollState = rememberSaveable(saver = ScrollState.Saver) {
        ScrollState(0)
    }

    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            AnpTopAppBar(
                title = { Text(text = stringResource(R.string.home_title)) },
                actions = {
                    if (includeSurveyWeekChip) {
                        SurveyWeekChipAction(
                            onWeekChanged = onWeekChanged,
                            hasTrailingAction = true,
                        )
                    }
                    IconButton(onClick = onToggleTheme) {
                        Icon(
                            imageVector = if (darkTheme) Icons.Default.LightMode else Icons.Default.DarkMode,
                            contentDescription = stringResource(
                                if (darkTheme) R.string.action_switch_light_theme else R.string.action_switch_dark_theme,
                            ),
                        )
                    }
                },
            )
        },
        bottomBar = { AnpAttributionFooter() },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .verticalScroll(scrollState)
                .padding(horizontal = 16.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            SyncStatusBanner(readiness = uiState.readiness)

            if (uiState.isOffline && uiState.hasCachedData) {
                OfflineBanner()
            }

            if (uiState.readiness == DataReadinessState.STALE ||
                uiState.readiness == DataReadinessState.ERROR
            ) {
                TextButton(
                    onClick = onRefresh,
                    enabled = !uiState.isRefreshing,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Text(
                        text = stringResource(
                            if (uiState.isRefreshing) {
                                R.string.banner_syncing
                            } else {
                                R.string.onboarding_action_sync_now
                            },
                        ),
                    )
                }
            }

            when {
                uiState.isLoading -> LoadingState(modifier = Modifier.fillMaxWidth())

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
                    LocationHeader(uiState = uiState)
                    Br010EmptyState(
                        dataAvailability = uiState.dataAvailability ?: DataAvailability.NO_DATA_THIS_WEEK,
                        operationalNote = uiState.operationalNote,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    RowActions(onNavigate = onNavigate)
                }

                else -> {
                    LocationHeader(uiState = uiState)
                    PriceMetadata(uiState = uiState)
                    if (uiState.tankFillCostEstimates.isEmpty()) {
                        TankFillCostPlaceholderCard(
                            onClick = { onNavigate(Routes.VEHICLES) },
                        )
                    } else {
                        VehicleCarousel(
                            estimates = uiState.tankFillCostEstimates,
                            onNavigate = onNavigate,
                        )
                    }
                    uiState.prices.forEach { price ->
                        FuelPriceCard(
                            price = price,
                            onClick = { onNavigate(Routes.stations(price.fuelProduct)) },
                        )
                    }
                    TextButton(
                        onClick = { onNavigate(Routes.PRICES) },
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = stringResource(R.string.home_view_price_details))
                    }
                    RowActions(onNavigate = onNavigate)
                }
            }
        }
    }
}

@Composable
private fun VehicleCarousel(
    estimates: List<TankFillCostEstimateUiModel>,
    onNavigate: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    val screenWidth = LocalConfiguration.current.screenWidthDp.dp
    val cardWidth = screenWidth * 0.85f

    LazyRow(
        modifier = modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        contentPadding = PaddingValues(horizontal = 4.dp),
    ) {
        items(estimates, key = { it.vehicleId.value }) { estimate ->
            TankFillCostCard(
                estimate = estimate,
                onClick = { onNavigate(Routes.VEHICLES) },
                onGoToStation = estimate.stationNavigationQuery?.let { query ->
                    {
                        when (MapAppChooser.openNavigation(context, query)) {
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
                },
                modifier = Modifier.width(cardWidth),
            )
        }
    }
}

@Composable
private fun LocationHeader(uiState: HomeUiState) {
    val municipality = uiState.municipality ?: return
    val state = uiState.state ?: return
    Text(
        text = stringResource(R.string.home_location_format, municipality, state.abbreviation),
        style = MaterialTheme.typography.headlineSmall,
        color = MaterialTheme.colorScheme.onBackground,
    )
}

@Composable
private fun PriceMetadata(uiState: HomeUiState) {
    uiState.surveyWeek?.let { week ->
        val locale = LocalConfiguration.current.locales[0]
        Text(
            text = stringResource(
                R.string.prices_survey_week_label,
                SurveyWeekFormatter.formatRange(week, locale),
            ),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        val currentDate = java.time.LocalDate.now()
        if (week.endDate.isBefore(currentDate)) {
            val formatter = java.time.format.DateTimeFormatter
                .ofLocalizedDate(java.time.format.FormatStyle.SHORT)
                .withLocale(locale)
            val formattedDate = week.endDate.format(formatter)
            Text(
                text = stringResource(
                    R.string.home_stale_price_table_message,
                    formattedDate,
                ),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.error,
                modifier = Modifier.padding(top = 4.dp),
            )
        }
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
        AssistChip(
            onClick = { onNavigate(Routes.LOCATION) },
            label = { Text(text = stringResource(R.string.nav_location)) },
        )
        AssistChip(
            onClick = { onNavigate(Routes.HISTORY) },
            label = { Text(text = stringResource(R.string.nav_history)) },
        )
        AssistChip(
            onClick = { onNavigate(Routes.STATIONS) },
            label = { Text(text = stringResource(R.string.nav_stations)) },
        )
        AssistChip(
            onClick = { onNavigate(Routes.VEHICLES) },
            label = { Text(text = stringResource(R.string.nav_vehicles)) },
        )
        AssistChip(
            onClick = { onNavigate(Routes.SETTINGS) },
            label = { Text(text = stringResource(R.string.nav_settings)) },
        )
    }
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
            onToggleTheme = {},
            onNavigate = {},
            onRefresh = {},
            onRetry = {},
            onWeekChanged = {},
        )
    }
}
