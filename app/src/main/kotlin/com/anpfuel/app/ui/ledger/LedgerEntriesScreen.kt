package com.anpfuel.app.ui.ledger

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.DateRange
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.PriceFormatter
import com.anpfuel.app.ui.components.ActionCardButton
import com.anpfuel.app.ui.components.ActionCardStyle
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.EmptyState
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.application.usecase.ledger.LedgerEntryRow
import com.anpfuel.application.usecase.ledger.LedgerEntryState
import com.anpfuel.application.usecase.ledger.LedgerEntrySummary
import com.anpfuel.domain.premium.ExpenseCategory
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerFill
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.PriceAmount
import java.math.BigDecimal
import java.util.Locale

@Composable
fun LedgerEntriesScreen(
    vehicleId: String,
    onNavigateBack: () -> Unit,
    onOpenMonthly: (String) -> Unit = {},
    modifier: Modifier = Modifier,
    viewModel: LedgerEntriesViewModel = hiltViewModel(),
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val locale = LocalConfiguration.current.locales[0]

    LaunchedEffect(vehicleId) {
        viewModel.load(DomainId.from(vehicleId), locale)
    }

    LedgerEntriesContent(
        uiState = uiState,
        locale = locale,
        vehicleIsElectric = uiState.vehicleIsElectric,
        simulatedTotal = if (uiState.form.dataset == LedgerDataset.CHARGE) {
            viewModel.simulatedChargeTotal()
        } else {
            null
        },
        onNavigateBack = onNavigateBack,
        onAddRefuel = { viewModel.startAdd(LedgerDataset.REFUEL) },
        onAddExpense = { viewModel.startAdd(LedgerDataset.EXPENSE) },
        onEdit = viewModel::startEdit,
        onQueue = viewModel::queueEntry,
        onRequestDelete = viewModel::requestDelete,
        onConfirmDelete = viewModel::confirmDelete,
        onDismissDelete = viewModel::dismissDelete,
        onOpenGap = viewModel::openGapDialog,
        onDismissGap = viewModel::dismissGapDialog,
        onGapBeforeChanged = viewModel::onGapBeforeChanged,
        onGapNoteChanged = viewModel::onGapNoteChanged,
        onConfirmGap = viewModel::declareGap,
        onRequestReset = viewModel::requestReset,
        onConfirmReset = viewModel::confirmReset,
        onDismissReset = viewModel::dismissReset,
        onDismissForm = viewModel::dismissForm,
        onDatasetChanged = viewModel::onDatasetChanged,
        onDateChanged = viewModel::onDateChanged,
        onTimeChanged = viewModel::onTimeChanged,
        onQuantityChanged = viewModel::onQuantityChanged,
        onTotalChanged = viewModel::onTotalChanged,
        onUnitPriceChanged = viewModel::onUnitPriceChanged,
        onRateChanged = viewModel::onRateChanged,
        onSocStartChanged = viewModel::onSocStartChanged,
        onSocEndChanged = viewModel::onSocEndChanged,
        onTariffIdChanged = viewModel::onTariffIdChanged,
        onTariffRevisionChanged = viewModel::onTariffRevisionChanged,
        onContextChanged = viewModel::onContextChanged,
        onFillChanged = viewModel::onFillChanged,
        onCategoryChanged = viewModel::onCategoryChanged,
        onOdometerChanged = viewModel::onOdometerChanged,
        onStationChanged = viewModel::onStationChanged,
        onNoteChanged = viewModel::onNoteChanged,
        onSaveDraft = viewModel::saveDraft,
        onEstimateChanged = viewModel::onEstimateChanged,
        onSaveEstimate = viewModel::saveEstimate,
        onClearEstimate = viewModel::clearEstimate,
        onDismissError = viewModel::dismissError,
        onOpenMonthly = { onOpenMonthly(vehicleId) },
        modifier = modifier,
    )
}

@OptIn(ExperimentalLayoutApi::class, androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
internal fun LedgerEntriesContent(
    uiState: LedgerEntriesUiState,
    locale: Locale,
    vehicleIsElectric: Boolean,
    simulatedTotal: String?,
    onNavigateBack: () -> Unit,
    onAddRefuel: () -> Unit,
    onAddExpense: () -> Unit,
    onEdit: (LedgerEntryRow) -> Unit,
    onQueue: (LedgerEntryRow) -> Unit,
    onRequestDelete: (LedgerEntryRow) -> Unit,
    onConfirmDelete: () -> Unit,
    onDismissDelete: () -> Unit,
    onOpenGap: (LedgerEntryRow) -> Unit,
    onDismissGap: () -> Unit,
    onGapBeforeChanged: (String) -> Unit,
    onGapNoteChanged: (String) -> Unit,
    onConfirmGap: () -> Unit,
    onRequestReset: () -> Unit,
    onConfirmReset: () -> Unit,
    onDismissReset: () -> Unit,
    onDismissForm: () -> Unit,
    onDatasetChanged: (LedgerDataset) -> Unit,
    onDateChanged: (String) -> Unit,
    onTimeChanged: (String) -> Unit,
    onQuantityChanged: (String) -> Unit,
    onTotalChanged: (String) -> Unit,
    onUnitPriceChanged: (String) -> Unit,
    onRateChanged: (String) -> Unit,
    onSocStartChanged: (String) -> Unit,
    onSocEndChanged: (String) -> Unit,
    onTariffIdChanged: (String) -> Unit,
    onTariffRevisionChanged: (String) -> Unit,
    onContextChanged: (com.anpfuel.domain.premium.LedgerSessionContext?) -> Unit,
    onFillChanged: (LedgerFill) -> Unit,
    onCategoryChanged: (ExpenseCategory) -> Unit,
    onOdometerChanged: (String) -> Unit,
    onStationChanged: (String) -> Unit,
    onNoteChanged: (String) -> Unit,
    onSaveDraft: () -> Unit,
    onEstimateChanged: (String) -> Unit,
    onSaveEstimate: () -> Unit,
    onClearEstimate: () -> Unit,
    onDismissError: () -> Unit,
    onOpenMonthly: () -> Unit,
    modifier: Modifier = Modifier,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            AnpTopAppBar(
                title = {
                    Text(
                        text = if (uiState.vehicleName.isBlank()) {
                            stringResource(R.string.ledger_entries_title)
                        } else {
                            uiState.vehicleName
                        },
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
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            EstimateSection(
                estimateInput = uiState.estimateInput,
                estimateSaved = uiState.estimateSaved,
                estimateError = uiState.estimateError,
                onEstimateChanged = onEstimateChanged,
                onSaveEstimate = onSaveEstimate,
                onClearEstimate = onClearEstimate,
            )
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                if (uiState.vehicleHasFuel) {
                    ActionCardButton(
                        label = stringResource(R.string.ledger_entry_add_refuel),
                        icon = Icons.Default.Add,
                        onClick = onAddRefuel,
                        style = ActionCardStyle.Primary,
                        modifier = Modifier.weight(1f),
                    )
                }
                ActionCardButton(
                    label = stringResource(R.string.ledger_entry_add_expense),
                    icon = Icons.Default.Add,
                    onClick = onAddExpense,
                    style = ActionCardStyle.Primary,
                    modifier = Modifier.weight(1f),
                )
            }
                ActionCardButton(
                    label = stringResource(R.string.ledger_entry_reset),
                    icon = Icons.Default.Add,
                    onClick = onRequestReset,
                    style = ActionCardStyle.Destructive,
                    modifier = Modifier.fillMaxWidth(),
                )
                ActionCardButton(
                    label = stringResource(R.string.monthly_open),
                    icon = Icons.Default.DateRange,
                    onClick = onOpenMonthly,
                    style = ActionCardStyle.Primary,
                    modifier = Modifier.fillMaxWidth(),
                )
            if (uiState.entries.isEmpty()) {
                EmptyState(
                    message = stringResource(R.string.ledger_entries_empty),
                    modifier = Modifier.fillMaxWidth(),
                )
            } else {
                uiState.entries.forEach { row ->
                    LedgerEntryCard(
                        row = row,
                        locale = locale,
                        onEdit = { onEdit(row) },
                        onQueue = { onQueue(row) },
                        onDelete = { onRequestDelete(row) },
                        onGap = { onOpenGap(row) },
                    )
                }
            }
            uiState.errorMessage?.let { message ->
                Text(
                    text = message,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                )
            }
        }
    }

    if (uiState.showForm) {
        LedgerEntryFormDialog(
            form = uiState.form,
            isSaving = uiState.isSaving,
            locale = locale,
            simulatedTotal = simulatedTotal,
            vehicleHasFuel = uiState.vehicleHasFuel,
            vehicleIsElectric = vehicleIsElectric,
            onDismiss = onDismissForm,
            onDatasetChanged = onDatasetChanged,
            onDateChanged = onDateChanged,
            onTimeChanged = onTimeChanged,
            onQuantityChanged = onQuantityChanged,
            onTotalChanged = onTotalChanged,
                        onUnitPriceChanged = onUnitPriceChanged,
                        onRateChanged = onRateChanged,
                        onSocStartChanged = onSocStartChanged,
                        onSocEndChanged = onSocEndChanged,
                        onTariffIdChanged = onTariffIdChanged,
                        onTariffRevisionChanged = onTariffRevisionChanged,
                        onContextChanged = onContextChanged,
                        onFillChanged = onFillChanged,
            onCategoryChanged = onCategoryChanged,
            onOdometerChanged = onOdometerChanged,
            onStationChanged = onStationChanged,
            onNoteChanged = onNoteChanged,
            onSave = onSaveDraft,
        )
    }

    if (uiState.showDeleteConfirm) {
        AlertDialog(
            onDismissRequest = onDismissDelete,
            title = { Text(text = stringResource(R.string.ledger_entry_delete_confirm)) },
            text = { Text(text = stringResource(R.string.ledger_entry_delete_message)) },
            confirmButton = {
                TextButton(onClick = onConfirmDelete) {
                    Text(text = stringResource(R.string.vehicle_delete))
                }
            },
            dismissButton = {
                TextButton(onClick = onDismissDelete) {
                    Text(text = stringResource(R.string.action_cancel))
                }
            },
        )
    }

    if (uiState.showGapDialog) {
        var before by rememberSaveable { mutableStateOf(uiState.gapBeforeInput) }
        var note by rememberSaveable { mutableStateOf(uiState.gapNoteInput) }
        AlertDialog(
            onDismissRequest = onDismissGap,
            title = { Text(text = stringResource(R.string.ledger_entry_gap_title)) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(
                        value = before,
                        onValueChange = {
                            before = it
                            onGapBeforeChanged(it)
                        },
                        label = { Text(text = stringResource(R.string.ledger_entry_gap_before)) },
                        singleLine = true,
                    )
                    OutlinedTextField(
                        value = note,
                        onValueChange = {
                            note = it
                            onGapNoteChanged(it)
                        },
                        label = { Text(text = stringResource(R.string.ledger_entry_gap_note)) },
                        singleLine = true,
                    )
                }
            },
            confirmButton = {
                TextButton(onClick = onConfirmGap) {
                    Text(text = stringResource(R.string.vehicle_save))
                }
            },
            dismissButton = {
                TextButton(onClick = onDismissGap) {
                    Text(text = stringResource(R.string.action_cancel))
                }
            },
        )
    }

    if (uiState.showResetConfirm) {
        AlertDialog(
            onDismissRequest = onDismissReset,
            title = { Text(text = stringResource(R.string.ledger_entry_reset_title)) },
            text = { Text(text = stringResource(R.string.ledger_entry_reset_message)) },
            confirmButton = {
                TextButton(onClick = onConfirmReset) {
                    Text(text = stringResource(R.string.vehicle_save))
                }
            },
            dismissButton = {
                TextButton(onClick = onDismissReset) {
                    Text(text = stringResource(R.string.action_cancel))
                }
            },
        )
    }
}

@Composable
private fun EstimateSection(
    estimateInput: String,
    estimateSaved: String?,
    estimateError: Boolean,
    onEstimateChanged: (String) -> Unit,
    onSaveEstimate: () -> Unit,
    onClearEstimate: () -> Unit,
) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(
                text = stringResource(R.string.ledger_entry_estimate_label),
                style = MaterialTheme.typography.titleSmall,
            )
            if (estimateSaved != null) {
                Text(
                    text = stringResource(R.string.ledger_entry_estimate_saved, estimateSaved),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
            OutlinedTextField(
                value = estimateInput,
                onValueChange = onEstimateChanged,
                modifier = Modifier.fillMaxWidth(),
                label = { Text(text = stringResource(R.string.ledger_entry_estimate_hint)) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                isError = estimateError,
                supportingText = if (estimateError) {
                    { Text(text = stringResource(R.string.ledger_entry_estimate_invalid)) }
                } else {
                    null
                },
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                ActionCardButton(
                    label = stringResource(R.string.vehicle_save),
                    icon = Icons.Default.Check,
                    onClick = onSaveEstimate,
                    style = ActionCardStyle.Primary,
                    modifier = Modifier.weight(1f),
                )
                if (estimateSaved != null) {
                    ActionCardButton(
                        label = stringResource(R.string.ledger_entry_estimate_clear),
                        icon = Icons.Default.Close,
                        onClick = onClearEstimate,
                        style = ActionCardStyle.Destructive,
                        modifier = Modifier.weight(1f),
                    )
                }
            }
        }
    }
}

@Composable
private fun LedgerEntryCard(
    row: LedgerEntryRow,
    locale: Locale,
    onEdit: () -> Unit,
    onQueue: () -> Unit,
    onDelete: () -> Unit,
    onGap: () -> Unit,
) {
    val stateRes = when (row.state) {
        LedgerEntryState.LOCAL -> R.string.ledger_entry_state_local
        LedgerEntryState.PENDING -> R.string.ledger_entry_state_pending
        LedgerEntryState.CONFLICT -> R.string.ledger_entry_state_conflict
    }
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Text(text = entrySummaryText(row, locale), style = MaterialTheme.typography.titleMedium)
            Text(
                text = stringResource(stateRes),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                if (row.state == LedgerEntryState.LOCAL || row.state == LedgerEntryState.CONFLICT) {
                    TextButton(onClick = onQueue) {
                        Text(text = stringResource(R.string.ledger_entry_queue))
                    }
                }
                IconButton(onClick = onEdit) {
                    Icon(imageVector = Icons.Default.Edit, contentDescription = stringResource(R.string.ledger_entry_edit))
                }
                IconButton(onClick = onDelete) {
                    Icon(imageVector = Icons.Default.Delete, contentDescription = stringResource(R.string.vehicle_delete))
                }
                TextButton(onClick = onGap) {
                    Text(text = stringResource(R.string.ledger_entry_gap))
                }
            }
        }
    }
}

@Composable
private fun entrySummaryText(row: LedgerEntryRow, locale: Locale): String {
    val amount = when (val summary = row.summary) {
        is LedgerEntrySummary.Refuel -> money(summary.totalCentavos, locale) +
            " • " + milliToUnits(summary.quantityMilli, locale) + " L"
        is LedgerEntrySummary.Expense -> money(summary.amountCentavos, locale)
        is LedgerEntrySummary.Charge -> (summary.totalCentavos?.let { money(it, locale) } ?: "?") +
            " • " + (summary.energyWh?.let { whToKwh(it, locale) + " kWh" } ?: "?")
    }
    return amount
}

private fun money(centavos: Long, locale: Locale): String =
    PriceFormatter.formatAmount(
        PriceAmount.of(BigDecimal.valueOf(centavos, 2)),
        locale,
    ).orEmpty()

private fun milliToUnits(milli: Long, locale: Locale): String =
    java.text.NumberFormat.getNumberInstance(locale).format(milli / 1000.0)

private fun whToKwh(wh: Long, locale: Locale): String =
    java.text.NumberFormat.getNumberInstance(locale).format(wh / 1000.0)

@Composable
@OptIn(ExperimentalLayoutApi::class)
private fun LedgerEntryFormDialog(
    form: LedgerEntryFormState,
    isSaving: Boolean,
    locale: Locale,
    simulatedTotal: String?,
    vehicleHasFuel: Boolean,
    vehicleIsElectric: Boolean,
    onDismiss: () -> Unit,
    onDatasetChanged: (LedgerDataset) -> Unit,
    onDateChanged: (String) -> Unit,
    onTimeChanged: (String) -> Unit,
    onQuantityChanged: (String) -> Unit,
    onTotalChanged: (String) -> Unit,
    onUnitPriceChanged: (String) -> Unit,
    onRateChanged: (String) -> Unit,
    onSocStartChanged: (String) -> Unit,
    onSocEndChanged: (String) -> Unit,
    onTariffIdChanged: (String) -> Unit,
    onTariffRevisionChanged: (String) -> Unit,
    onContextChanged: (com.anpfuel.domain.premium.LedgerSessionContext?) -> Unit,
    onFillChanged: (LedgerFill) -> Unit,
    onCategoryChanged: (ExpenseCategory) -> Unit,
    onOdometerChanged: (String) -> Unit,
    onStationChanged: (String) -> Unit,
    onNoteChanged: (String) -> Unit,
    onSave: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = {
            Text(
                text = stringResource(
                    if (form.entryId == null) {
                        R.string.ledger_entry_add_title
                    } else {
                        R.string.ledger_entry_edit_title
                    },
                ),
            )
        },
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (vehicleHasFuel) {
                        FilterChip(
                            selected = form.dataset == LedgerDataset.REFUEL,
                            onClick = { onDatasetChanged(LedgerDataset.REFUEL) },
                            label = { Text(text = stringResource(R.string.ledger_entry_add_refuel)) },
                        )
                    }
                    FilterChip(
                        selected = form.dataset == LedgerDataset.EXPENSE,
                        onClick = { onDatasetChanged(LedgerDataset.EXPENSE) },
                        label = { Text(text = stringResource(R.string.ledger_entry_add_expense)) },
                    )
                    if (vehicleIsElectric) {
                        FilterChip(
                            selected = form.dataset == LedgerDataset.CHARGE,
                            onClick = { onDatasetChanged(LedgerDataset.CHARGE) },
                            label = { Text(text = stringResource(R.string.ledger_entry_add_charge)) },
                        )
                    }
                }
                OutlinedTextField(
                    value = form.dateInput,
                    onValueChange = onDateChanged,
                    label = { Text(text = stringResource(R.string.ledger_entry_date)) },
                    singleLine = true,
                    isError = form.validationErrorKey == LedgerEntryValidationError.DATE_INVALID,
                    supportingText = errorText(LedgerEntryValidationError.DATE_INVALID, form),
                )
                OutlinedTextField(
                    value = form.timeInput,
                    onValueChange = onTimeChanged,
                    label = { Text(text = stringResource(R.string.ledger_entry_time)) },
                    singleLine = true,
                    isError = form.validationErrorKey == LedgerEntryValidationError.DATE_INVALID,
                    supportingText = errorText(LedgerEntryValidationError.DATE_INVALID, form),
                )
                if (form.dataset == LedgerDataset.REFUEL) {
                    OutlinedTextField(
                        value = form.quantityInput,
                        onValueChange = onQuantityChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_quantity)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                        isError = form.validationErrorKey == LedgerEntryValidationError.QUANTITY_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.QUANTITY_INVALID, form),
                    )
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        LedgerFill.entries.forEach { fill ->
                            FilterChip(
                                selected = form.fill == fill,
                                onClick = { onFillChanged(fill) },
                                label = { Text(text = fillLabel(fill)) },
                            )
                        }
                    }
                }
                if (form.dataset == LedgerDataset.CHARGE) {
                    OutlinedTextField(
                        value = form.quantityInput,
                        onValueChange = onQuantityChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_charge_energy)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                        isError = form.validationErrorKey == LedgerEntryValidationError.QUANTITY_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.QUANTITY_INVALID, form),
                    )
                    OutlinedTextField(
                        value = form.rateInput,
                        onValueChange = onRateChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_charge_rate)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                        isError = form.validationErrorKey == LedgerEntryValidationError.RATE_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.RATE_INVALID, form),
                    )
                    simulatedTotal?.let { simulated ->
                        Text(
                            text = stringResource(R.string.ledger_entry_charge_simulated, simulated),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    OutlinedTextField(
                        value = form.socStartInput,
                        onValueChange = onSocStartChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_charge_soc_start)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                        isError = form.validationErrorKey == LedgerEntryValidationError.SOC_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.SOC_INVALID, form),
                    )
                    OutlinedTextField(
                        value = form.socEndInput,
                        onValueChange = onSocEndChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_charge_soc_end)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                        isError = form.validationErrorKey == LedgerEntryValidationError.SOC_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.SOC_INVALID, form),
                    )
                    OutlinedTextField(
                        value = form.tariffIdInput,
                        onValueChange = onTariffIdChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_charge_tariff)) },
                        singleLine = true,
                        isError = form.validationErrorKey == LedgerEntryValidationError.TARIFF_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.TARIFF_INVALID, form),
                    )
                    OutlinedTextField(
                        value = form.tariffRevisionInput,
                        onValueChange = onTariffRevisionChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_charge_tariff_revision)) },
                        singleLine = true,
                        isError = form.validationErrorKey == LedgerEntryValidationError.TARIFF_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.TARIFF_INVALID, form),
                    )
                    Text(
                        text = stringResource(R.string.ledger_entry_charge_context),
                        style = MaterialTheme.typography.titleSmall,
                    )
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        com.anpfuel.domain.premium.LedgerSessionContext.entries.forEach { context ->
                            FilterChip(
                                selected = form.context == context,
                                onClick = { onContextChanged(if (form.context == context) null else context) },
                                label = { Text(text = contextLabel(context)) },
                            )
                        }
                    }
                }
                if (form.dataset == LedgerDataset.EXPENSE) {
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        ExpenseCategory.entries.forEach { category ->
                            FilterChip(
                                selected = form.category == category,
                                onClick = { onCategoryChanged(category) },
                                label = { Text(text = categoryLabel(category)) },
                            )
                        }
                    }
                }
                OutlinedTextField(
                    value = form.totalInput,
                    onValueChange = onTotalChanged,
                    label = { Text(text = stringResource(R.string.ledger_entry_total)) },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                    isError = form.validationErrorKey == LedgerEntryValidationError.TOTAL_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.TOTAL_INVALID, form),
                )
                if (form.dataset == LedgerDataset.REFUEL) {
                    OutlinedTextField(
                        value = form.unitPriceInput,
                        onValueChange = onUnitPriceChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_unit_price)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                        isError = form.validationErrorKey == LedgerEntryValidationError.UNIT_PRICE_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.UNIT_PRICE_INVALID, form),
                    )
                }
                OutlinedTextField(
                    value = form.odometerInput,
                    onValueChange = onOdometerChanged,
                    label = { Text(text = stringResource(R.string.ledger_entry_odometer)) },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                    isError = form.validationErrorKey == LedgerEntryValidationError.ODOMETER_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.ODOMETER_INVALID, form),
                )
                if (form.dataset == LedgerDataset.REFUEL) {
                    OutlinedTextField(
                        value = form.stationInput,
                        onValueChange = onStationChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_station)) },
                        singleLine = true,
                        isError = form.validationErrorKey == LedgerEntryValidationError.STATION_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.STATION_INVALID, form),
                    )
                }
                OutlinedTextField(
                    value = form.noteInput,
                    onValueChange = onNoteChanged,
                    label = { Text(text = stringResource(R.string.ledger_entry_note)) },
                    singleLine = true,
                )
                if (form.validationErrorKey == LedgerEntryValidationError.FUEL_UNAVAILABLE) {
                    Text(
                        text = stringResource(R.string.ledger_entry_fuel_unavailable),
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
        },
        confirmButton = {
            TextButton(onClick = onSave, enabled = !isSaving) {
                Text(text = stringResource(R.string.vehicle_save))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text(text = stringResource(R.string.action_cancel))
            }
        },
    )
}


@Composable
private fun errorText(key: LedgerEntryValidationError, form: LedgerEntryFormState): @Composable (() -> Unit)? {
    if (form.validationErrorKey != key) return null
    val res = when (key) {
        LedgerEntryValidationError.DATE_INVALID -> R.string.ledger_entry_validation_date
        LedgerEntryValidationError.QUANTITY_INVALID -> R.string.ledger_entry_validation_quantity
        LedgerEntryValidationError.TOTAL_INVALID -> R.string.ledger_entry_validation_total
        LedgerEntryValidationError.UNIT_PRICE_INVALID -> R.string.ledger_entry_validation_unit_price
        LedgerEntryValidationError.ODOMETER_INVALID -> R.string.ledger_entry_validation_odometer
        LedgerEntryValidationError.STATION_INVALID -> R.string.ledger_entry_validation_station
        LedgerEntryValidationError.FUEL_UNAVAILABLE -> R.string.ledger_entry_fuel_unavailable
        LedgerEntryValidationError.CHARGE_UNAVAILABLE -> R.string.ledger_entry_charge_unavailable
        LedgerEntryValidationError.TARIFF_INVALID -> R.string.ledger_entry_validation_tariff
        LedgerEntryValidationError.RATE_INVALID -> R.string.ledger_entry_validation_rate
        LedgerEntryValidationError.SOC_INVALID -> R.string.ledger_entry_validation_soc
    }
    return { Text(text = stringResource(res)) }
}

@Composable
private fun contextLabel(context: com.anpfuel.domain.premium.LedgerSessionContext): String = stringResource(
    when (context) {
        com.anpfuel.domain.premium.LedgerSessionContext.HOME -> R.string.ledger_entry_context_home
        com.anpfuel.domain.premium.LedgerSessionContext.WORK -> R.string.ledger_entry_context_work
        com.anpfuel.domain.premium.LedgerSessionContext.PUBLIC -> R.string.ledger_entry_context_public
        com.anpfuel.domain.premium.LedgerSessionContext.OTHER -> R.string.ledger_entry_context_other
    },
)

@Composable
private fun fillLabel(fill: LedgerFill): String = stringResource(
    when (fill) {
        LedgerFill.FULL -> R.string.ledger_entry_fill_full
        LedgerFill.PARTIAL -> R.string.ledger_entry_fill_partial
        LedgerFill.UNKNOWN -> R.string.ledger_entry_fill_unknown
    },
)

@Composable
private fun categoryLabel(category: ExpenseCategory): String = stringResource(
    when (category) {
        ExpenseCategory.MAINTENANCE -> R.string.ledger_entry_category_maintenance
        ExpenseCategory.INSURANCE -> R.string.ledger_entry_category_insurance
        ExpenseCategory.TAXES -> R.string.ledger_entry_category_taxes
        ExpenseCategory.PARKING -> R.string.ledger_entry_category_parking
        ExpenseCategory.TOLLS -> R.string.ledger_entry_category_tolls
        ExpenseCategory.CLEANING -> R.string.ledger_entry_category_cleaning
        ExpenseCategory.OTHER -> R.string.ledger_entry_category_other
    },
)

@Preview(showBackground = true, name = "Ledger entries")
@Composable
private fun LedgerEntriesPreview() {
    AnpFuelTheme {
        LedgerEntriesContent(
            uiState = LedgerEntriesUiState(isLoading = false, vehicleName = "Gol"),
            locale = Locale.US,
            vehicleIsElectric = false,
            simulatedTotal = null,
            onNavigateBack = {},
            onAddRefuel = {},
            onAddExpense = {},
            onEdit = {},
            onQueue = {},
            onRequestDelete = {},
            onConfirmDelete = {},
            onDismissDelete = {},
            onOpenGap = {},
            onDismissGap = {},
            onGapBeforeChanged = {},
            onGapNoteChanged = {},
            onConfirmGap = {},
            onRequestReset = {},
            onConfirmReset = {},
            onDismissReset = {},
            onDismissForm = {},
            onDatasetChanged = {},
            onDateChanged = {},
            onTimeChanged = {},
            onQuantityChanged = {},
            onTotalChanged = {},
            onUnitPriceChanged = {},
            onRateChanged = {},
            onSocStartChanged = {},
            onSocEndChanged = {},
            onTariffIdChanged = {},
            onTariffRevisionChanged = {},
            onContextChanged = {},
            onFillChanged = {},
            onCategoryChanged = {},
            onOdometerChanged = {},
            onStationChanged = {},
            onNoteChanged = {},
            onSaveDraft = {},
            onEstimateChanged = {},
            onSaveEstimate = {},
                        onClearEstimate = {},
                        onDismissError = {},
                        onOpenMonthly = {},
                    )
                }
            }
