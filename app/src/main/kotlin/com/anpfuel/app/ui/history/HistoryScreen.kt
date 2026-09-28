package com.anpfuel.app.ui.history

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.mapper.SurveyWeekFormatter
import com.anpfuel.app.ui.components.AnpAttributionFooter
import com.anpfuel.app.ui.components.EmptyState
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.components.FuelProductIcon
import com.anpfuel.app.ui.components.FuelProductLabel
import com.anpfuel.app.ui.components.LoadingState
import com.anpfuel.app.ui.components.OfflineBanner
import com.anpfuel.app.ui.components.PriceHistoryEntryRow
import com.anpfuel.app.ui.components.PriceHistoryTrendChart
import com.anpfuel.app.ui.weekpicker.SurveyWeekChipAction
import com.anpfuel.app.ui.model.HistoryEntryUiModel
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import java.math.BigDecimal

@Composable
fun HistoryScreen(
    onNavigateBack: (() -> Unit)? = null,
    modifier: Modifier = Modifier,
    viewModel: HistoryViewModel = hiltViewModel(),
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val locale = LocalConfiguration.current.locales[0]

    LaunchedEffect(locale) {
        viewModel.load(locale)
    }

    HistoryContent(
        uiState = uiState,
        onNavigateBack = onNavigateBack,
        onFuelProductSelected = { fuelProduct -> viewModel.onFuelProductSelected(fuelProduct, locale) },
        onRetry = { viewModel.load(locale) },
        onWeekChanged = { viewModel.load(locale) },
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
private fun HistoryContent(
    uiState: HistoryUiState,
    onNavigateBack: (() -> Unit)? = null,
    onFuelProductSelected: (FuelProduct) -> Unit,
    onRetry: () -> Unit,
    onWeekChanged: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val locale = LocalConfiguration.current.locales[0]

    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            AnpTopAppBar(
                onNavigateUp = onNavigateBack,
                title = {
                    Text(
                        text = stringResource(R.string.history_title),
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
                    )
                }
            }

            when {
                uiState.isLoading -> LoadingState(modifier = Modifier.fillMaxWidth())

                uiState.errorMessage != null -> {
                    ErrorState(
                        message = uiState.errorMessage,
                        modifier = Modifier.fillMaxWidth(),
                        onRetry = onRetry,
                    )
                }

                uiState.showHistoryDisabled -> {
                    EmptyState(
                        message = stringResource(R.string.history_disabled),
                        modifier = Modifier.fillMaxWidth(),
                    )
                }

                uiState.showInsufficientData -> {
                    EmptyState(
                        message = stringResource(R.string.history_insufficient_data),
                        hint = stringResource(R.string.history_insufficient_data_hint),
                        modifier = Modifier.fillMaxWidth(),
                    )
                }

                uiState.entries.isNotEmpty() -> {
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

                    PriceHistoryTrendChart(entries = uiState.entries)

                    uiState.entries.forEach { entry ->
                        PriceHistoryEntryRow(
                            weekLabel = stringResource(
                                R.string.history_week_label,
                                SurveyWeekFormatter.formatRangeCompact(entry.surveyWeek, locale),
                            ),
                            averageFormatted = entry.averageFormatted
                                ?: stringResource(R.string.prices_not_available),
                        )
                    }
                }
            }
        }
    }
}

@Preview(showBackground = true)
@Composable
private fun HistoryScreenPreview() {
    AnpFuelTheme {
        HistoryContent(
            uiState = HistoryUiState(
                isLoading = false,
                selectedFuelProduct = FuelProduct.ETHANOL,
                municipality = "Curitiba",
                state = BrazilianState.PARANA,
                entries = listOf(
                    HistoryEntryUiModel(
                        surveyWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06"),
                        averageFormatted = "R$ 3,40",
                        averageValue = BigDecimal("3.40"),
                    ),
                    HistoryEntryUiModel(
                        surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"),
                        averageFormatted = "R$ 3,52",
                        averageValue = BigDecimal("3.52"),
                    ),
                ),
            ),
            onNavigateBack = {},
            onFuelProductSelected = {},
            onRetry = {},
            onWeekChanged = {},
        )
    }
}
