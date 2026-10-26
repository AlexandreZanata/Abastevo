package com.anpfuel.app.ui.monthly

import android.content.Intent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
import com.anpfuel.app.mapper.PriceFormatter
import com.anpfuel.app.mapper.LedgerValueFormatter
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.EmptyState
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.application.usecase.ledger.MonthlyVehicleIndicators
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.domain.valueobject.PriceAmount
import java.math.BigDecimal
import java.time.Month
import java.time.ZoneId
import java.time.format.TextStyle
import java.util.Locale

@Composable
fun MonthlyIndicatorsScreen(
    vehicleId: String,
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: MonthlyIndicatorsViewModel = hiltViewModel(),
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val locale = LocalConfiguration.current.locales[0]
    val context = LocalContext.current

    LaunchedEffect(vehicleId) {
        val now = java.time.LocalDate.now()
        viewModel.load(DomainId.from(vehicleId), now.year, now.monthValue, ZoneId.systemDefault())
    }
    LaunchedEffect(uiState.exportCsv) {
        uiState.exportCsv?.let { csv ->
            val intent = Intent(Intent.ACTION_SEND).apply {
                type = "text/plain"
                putExtra(Intent.EXTRA_SUBJECT, context.getString(R.string.monthly_export_subject))
                putExtra(Intent.EXTRA_TEXT, csv)
            }
            context.startActivity(Intent.createChooser(intent, context.getString(R.string.monthly_export_title)))
            viewModel.onExportConsumed()
        }
    }

    MonthlyIndicatorsContent(
        uiState = uiState,
        locale = locale,
        onNavigateBack = onNavigateBack,
        onPreviousMonth = viewModel::previousMonth,
        onNextMonth = viewModel::nextMonth,
        onRetry = viewModel::reload,
        onExport = viewModel::prepareExport,
        onDismissError = viewModel::dismissError,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun MonthlyIndicatorsContent(
    uiState: MonthlyIndicatorsUiState,
    locale: Locale,
    onNavigateBack: () -> Unit,
    onPreviousMonth: () -> Unit,
    onNextMonth: () -> Unit,
    onRetry: () -> Unit,
    onExport: () -> Unit,
    onDismissError: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val monthLabel = try {
        Month.of(uiState.month).getDisplayName(TextStyle.FULL, locale) + " " + uiState.year
    } catch (_: Exception) {
        "${uiState.month}/${uiState.year}"
    }

    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            AnpTopAppBar(
                title = {
                    Text(
                        text = uiState.vehicleName.ifBlank { stringResource(R.string.monthly_title) },
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
                .padding(horizontal = 16.dp, vertical = 8.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                IconButton(onClick = onPreviousMonth) {
                    Icon(
                        imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                        contentDescription = stringResource(R.string.monthly_previous),
                    )
                }
                Text(text = monthLabel, style = MaterialTheme.typography.titleMedium)
                IconButton(onClick = onNextMonth) {
                    Icon(
                        imageVector = Icons.AutoMirrored.Filled.ArrowForward,
                        contentDescription = stringResource(R.string.monthly_next),
                    )
                }
            }
            if (uiState.isLoading) {
                CircularProgressIndicator(modifier = Modifier.align(Alignment.CenterHorizontally))
            } else if (uiState.rows.isEmpty()) {
                EmptyState(
                    message = stringResource(R.string.monthly_empty),
                    modifier = Modifier.fillMaxWidth(),
                )
            } else {
                uiState.rows.forEach { row ->
                    MonthlyIndicatorCard(row = row, locale = locale)
                }
                TextButton(onClick = onExport, modifier = Modifier.fillMaxWidth()) {
                    Text(text = stringResource(R.string.monthly_export))
                }
            }
            uiState.errorMessage?.let { message ->
                ErrorState(message = message, modifier = Modifier.fillMaxWidth(), onRetry = onDismissError)
            }
        }
    }
}

@Composable
private fun MonthlyIndicatorCard(row: MonthlyVehicleIndicators, locale: Locale) {
    fun money(centavos: Long): String =
        PriceFormatter.formatAmount(PriceAmount.of(BigDecimal.valueOf(centavos, 2)), locale).orEmpty()

    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(text = row.vehicleName, style = MaterialTheme.typography.titleMedium)
            Text(
                text = stringResource(R.string.monthly_purchase, money(row.purchaseCentavos)),
                style = MaterialTheme.typography.bodyMedium,
            )
            Text(
                text = stringResource(R.string.monthly_expense, money(row.expenseCentavos)),
                style = MaterialTheme.typography.bodyMedium,
            )
            if (row.chargeCostCentavos > 0 || row.chargeEnergyWh > 0) {
                Text(
                    text = stringResource(R.string.monthly_charge, money(row.chargeCostCentavos)),
                    style = MaterialTheme.typography.bodyMedium,
                )
                Text(
                    text = stringResource(
                        R.string.monthly_energy_purchased,
                        java.text.NumberFormat.getIntegerInstance(locale).format(row.chargeEnergyWh),
                    ),
                    style = MaterialTheme.typography.bodyMedium,
                )
                if (row.energyMeasured.unknown.isEmpty()) {
                    Text(
                        text = stringResource(R.string.monthly_energy_measured, row.energyMeasured.whPerKm),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                } else {
                    Text(
                        text = stringResource(R.string.monthly_energy_unavailable, row.energyMeasured.unknown),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                }
                if (row.energyReference.unknown.isEmpty()) {
                    Text(
                        text = stringResource(R.string.monthly_energy_reference, row.energyReference.whPerKm),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                } else {
                    Text(
                        text = stringResource(R.string.monthly_energy_reference_unavailable, row.energyReference.unknown),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                if (row.energyComparison.unknown.isEmpty()) {
                    Text(
                        text = stringResource(R.string.monthly_energy_comparison, row.energyComparison.changePercent),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                } else {
                    Text(
                        text = stringResource(R.string.monthly_energy_comparison_unknown, row.energyComparison.unknown),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            if (row.measured.unknown.isEmpty()) {
                Text(
                    text = stringResource(
                        R.string.monthly_measured,
                        LedgerValueFormatter.efficiency(row.measured.kmPerUnit, locale).orEmpty(),
                        money(row.measured.eligibleCostCentavos),
                    ),
                    style = MaterialTheme.typography.bodyMedium,
                )
            } else {
                val estimated = row.estimatedKmPerL
                if (estimated != null) {
                    Text(
                        text = stringResource(R.string.monthly_estimated, estimated),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                } else {
                    Text(
                        text = stringResource(R.string.monthly_window_unknown, row.measured.unknown),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                }
            }
            val costPerKm = LedgerValueFormatter.costPerKilometre(row.costPerKm, locale)
            if (costPerKm != null) {
                Text(
                    text = stringResource(R.string.monthly_cost_per_km, costPerKm),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
            val combinedCost = LedgerValueFormatter.costPerKilometre(row.combinedCentavosPerKm, locale)
            if (row.combinedUnknown.isEmpty() && combinedCost != null) {
                Text(
                    text = stringResource(R.string.monthly_combined, combinedCost),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
            if (row.comparisonUnknown.isEmpty()) {
                Text(
                    text = stringResource(R.string.monthly_comparison, row.comparisonChangePercent),
                    style = MaterialTheme.typography.bodySmall,
                )
            } else {
                Text(
                    text = stringResource(R.string.monthly_comparison_unknown, row.comparisonUnknown),
                    style = MaterialTheme.typography.bodySmall,
                )
            }
            Text(
                text = stringResource(
                    R.string.monthly_window,
                    row.windowNote.recordCount,
                    row.windowNote.anchorFills,
                    row.windowNote.distanceMetres,
                ),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            row.byCategory.entries.sortedBy { it.key.name }.forEach { (category, total) ->
                Text(
                    text = "${categoryLabel(category)}: ${money(total)}",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun categoryLabel(category: com.anpfuel.domain.premium.ExpenseCategory): String = stringResource(
    when (category) {
        com.anpfuel.domain.premium.ExpenseCategory.MAINTENANCE -> R.string.ledger_entry_category_maintenance
        com.anpfuel.domain.premium.ExpenseCategory.INSURANCE -> R.string.ledger_entry_category_insurance
        com.anpfuel.domain.premium.ExpenseCategory.TAXES -> R.string.ledger_entry_category_taxes
        com.anpfuel.domain.premium.ExpenseCategory.PARKING -> R.string.ledger_entry_category_parking
        com.anpfuel.domain.premium.ExpenseCategory.TOLLS -> R.string.ledger_entry_category_tolls
        com.anpfuel.domain.premium.ExpenseCategory.CLEANING -> R.string.ledger_entry_category_cleaning
        com.anpfuel.domain.premium.ExpenseCategory.OTHER -> R.string.ledger_entry_category_other
    },
)

@Preview(showBackground = true, name = "Monthly indicators")
@Composable
private fun MonthlyIndicatorsPreview() {
    AnpFuelTheme {
        MonthlyIndicatorsContent(
            uiState = MonthlyIndicatorsUiState(isLoading = false, vehicleName = "Gol", year = 2026, month = 3),
            locale = Locale.US,
            onNavigateBack = {},
            onPreviousMonth = {},
            onNextMonth = {},
            onRetry = {},
            onExport = {},
            onDismissError = {},
        )
    }
}
