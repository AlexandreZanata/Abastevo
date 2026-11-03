package com.anpfuel.app.ui.ledger

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.material3.Surface
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.FilledIconButton
import androidx.compose.material.icons.filled.Speed
import androidx.compose.material.icons.filled.RestartAlt
import androidx.compose.material.icons.filled.LocalGasStation
import androidx.compose.material.icons.filled.EvStation
import androidx.compose.material.icons.automirrored.filled.ReceiptLong
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.clickable
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
import androidx.compose.ui.window.DialogProperties
import androidx.compose.material3.Button
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
import com.anpfuel.app.ui.theme.AnpContainerDefaults
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
    initialDataset: LedgerDataset? = null,
    modifier: Modifier = Modifier,
    viewModel: LedgerEntriesViewModel = hiltViewModel(),
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val locale = LocalConfiguration.current.locales[0]

    LaunchedEffect(vehicleId) {
        viewModel.load(DomainId.from(vehicleId), locale)
    }
    var initialStarted by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(initialDataset, uiState.isLoading) {
        if (!initialStarted && initialDataset != null && !uiState.isLoading) {
            initialStarted = true
            viewModel.startAdd(initialDataset)
        }
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
        onFeesChanged = viewModel::onFeesChanged,
        onSocStartChanged = viewModel::onSocStartChanged,
        onSocEndChanged = viewModel::onSocEndChanged,
        onTariffIdChanged = viewModel::onTariffIdChanged,
        onTariffRevisionChanged = viewModel::onTariffRevisionChanged,
        onContextChanged = viewModel::onContextChanged,
        onFillChanged = viewModel::onFillChanged,
        onCategoryChanged = viewModel::onCategoryChanged,
        onOdometerChanged = viewModel::onOdometerChanged,
        onReviewOdometer = viewModel::reviewOdometer,
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
    onFeesChanged: (String) -> Unit,
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
    onReviewOdometer: (com.anpfuel.domain.premium.OdometerReviewChoice) -> Unit = {},
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
            // Primary actions first: what a driver does most, side by side.
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                if (uiState.vehicleHasFuel) {
                    LedgerQuickAction(
                        label = stringResource(R.string.ledger_entry_add_refuel),
                        icon = Icons.Default.LocalGasStation,
                        onClick = onAddRefuel,
                        modifier = Modifier.weight(1f),
                    )
                }
                LedgerQuickAction(
                    label = stringResource(R.string.ledger_entry_add_expense),
                    icon = Icons.AutoMirrored.Filled.ReceiptLong,
                    onClick = onAddExpense,
                    modifier = Modifier.weight(1f),
                )
            }
            EstimateSection(
                estimateInput = uiState.estimateInput,
                estimateSaved = uiState.estimateSaved,
                estimateError = uiState.estimateError,
                onEstimateChanged = onEstimateChanged,
                onSaveEstimate = onSaveEstimate,
                onClearEstimate = onClearEstimate,
            )
            LedgerToolsCard(onOpenMonthly = onOpenMonthly, onRequestReset = onRequestReset)
            Text(
                text = stringResource(R.string.ledger_entries_title),
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.padding(top = 8.dp),
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
                        onFeesChanged = onFeesChanged,
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
            onReviewOdometer = onReviewOdometer,
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
    LedgerCard {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            LedgerIconBadge(Icons.Default.Speed)
            Column(Modifier.weight(1f)) {
                Text(
                    text = stringResource(R.string.ledger_entry_estimate_label),
                    style = MaterialTheme.typography.titleSmall,
                )
                if (estimateSaved != null) {
                    Text(
                        text = stringResource(R.string.ledger_entry_estimate_saved, estimateSaved),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            if (estimateSaved != null) {
                TextButton(onClick = onClearEstimate) {
                    Text(text = stringResource(R.string.ledger_entry_estimate_clear))
                }
            }
        }
        OutlinedTextField(
            value = estimateInput,
            onValueChange = onEstimateChanged,
            modifier = Modifier.fillMaxWidth(),
            placeholder = { Text(text = stringResource(R.string.ledger_entry_estimate_hint)) },
            suffix = { Text("km/L") },
            singleLine = true,
            shape = RoundedCornerShape(16.dp),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
            isError = estimateError,
            supportingText = if (estimateError) {
                { Text(text = stringResource(R.string.ledger_entry_estimate_invalid)) }
            } else {
                null
            },
            trailingIcon = {
                FilledIconButton(onClick = onSaveEstimate) {
                    Icon(imageVector = Icons.Default.Check, contentDescription = stringResource(R.string.vehicle_save))
                }
            },
        )
    }
}

/** Large tonal action with the icon above a single-line label (never wraps mid-word). */
@Composable
private fun LedgerQuickAction(label: String, icon: ImageVector, onClick: () -> Unit, modifier: Modifier = Modifier) {
    Surface(
        onClick = onClick,
        shape = RoundedCornerShape(20.dp),
        color = MaterialTheme.colorScheme.primaryContainer,
        contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
        modifier = modifier.heightIn(min = 96.dp),
    ) {
        Column(
            Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Icon(icon, contentDescription = null)
                Spacer(Modifier.weight(1f))
                Icon(Icons.Default.Add, contentDescription = null, modifier = Modifier.size(20.dp))
            }
            Text(label, style = MaterialTheme.typography.titleSmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
    }
}

/** Secondary tools grouped as a list: monthly report and the destructive reset. */
@Composable
private fun LedgerToolsCard(onOpenMonthly: () -> Unit, onRequestReset: () -> Unit) {
    Surface(
        shape = RoundedCornerShape(20.dp),
        color = MaterialTheme.colorScheme.surfaceContainer,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column {
            LedgerToolRow(
                label = stringResource(R.string.monthly_open),
                icon = Icons.Default.DateRange,
                tint = MaterialTheme.colorScheme.primary,
                onClick = onOpenMonthly,
            )
            HorizontalDivider(Modifier.padding(horizontal = 16.dp), color = MaterialTheme.colorScheme.outlineVariant)
            LedgerToolRow(
                label = stringResource(R.string.ledger_entry_reset),
                icon = Icons.Default.RestartAlt,
                tint = MaterialTheme.colorScheme.error,
                onClick = onRequestReset,
            )
        }
    }
}

@Composable
private fun LedgerToolRow(label: String, icon: ImageVector, tint: androidx.compose.ui.graphics.Color, onClick: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).heightIn(min = 56.dp)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Icon(icon, contentDescription = null, tint = tint)
        Text(label, style = MaterialTheme.typography.bodyLarge, color = if (tint == MaterialTheme.colorScheme.error) tint else MaterialTheme.colorScheme.onSurface, modifier = Modifier.weight(1f))
        Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, contentDescription = null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun LedgerIconBadge(icon: ImageVector) {
    Surface(shape = CircleShape, color = MaterialTheme.colorScheme.secondaryContainer, modifier = Modifier.size(40.dp)) {
        Box(contentAlignment = Alignment.Center) {
            Icon(icon, contentDescription = null, tint = MaterialTheme.colorScheme.onSecondaryContainer, modifier = Modifier.size(22.dp))
        }
    }
}

@Composable
private fun LedgerCard(content: @Composable ColumnScope.() -> Unit) {
    Surface(
        shape = RoundedCornerShape(20.dp),
        color = MaterialTheme.colorScheme.surfaceContainer,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp), content = content)
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
    LedgerCard {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            LedgerIconBadge(
                when (row.summary) {
                    is LedgerEntrySummary.Refuel -> Icons.Default.LocalGasStation
                    is LedgerEntrySummary.Expense -> Icons.AutoMirrored.Filled.ReceiptLong
                    is LedgerEntrySummary.Charge -> Icons.Default.EvStation
                },
            )
            Text(text = entrySummaryText(row, locale), style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
        }
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            if (row.odometerNeedsReview) Text(stringResource(R.string.ledger_review_entry), style = MaterialTheme.typography.bodySmall)
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
    onFeesChanged: (String) -> Unit,
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
    onReviewOdometer: (com.anpfuel.domain.premium.OdometerReviewChoice) -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        // Data entry needs room: near full width with a 16 dp gutter.
        properties = DialogProperties(usePlatformDefaultWidth = false),
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp),
        shape = RoundedCornerShape(28.dp),
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
                verticalArrangement = Arrangement.spacedBy(12.dp),
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
                Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    OutlinedTextField(
                        value = form.dateInput,
                        onValueChange = onDateChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_date)) },
                        leadingIcon = { Icon(Icons.Default.DateRange, contentDescription = null) },
                        singleLine = true,
                        shape = RoundedCornerShape(16.dp),
                        isError = form.validationErrorKey == LedgerEntryValidationError.DATE_INVALID,
                        modifier = Modifier.weight(1.4f),
                    )
                    OutlinedTextField(
                        value = form.timeInput,
                        onValueChange = onTimeChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_time)) },
                        singleLine = true,
                        shape = RoundedCornerShape(16.dp),
                        isError = form.validationErrorKey == LedgerEntryValidationError.DATE_INVALID,
                        modifier = Modifier.weight(1f),
                    )
                }
                errorText(LedgerEntryValidationError.DATE_INVALID, form)?.let { message ->
                    androidx.compose.runtime.CompositionLocalProvider(
                        androidx.compose.material3.LocalContentColor provides MaterialTheme.colorScheme.error,
                    ) { message() }
                }
                if (form.dataset == LedgerDataset.REFUEL) {
                    OutlinedTextField(
                        modifier = Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(16.dp),
                        value = form.quantityInput,
                        onValueChange = onQuantityChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_quantity)) },
                        suffix = { Text("L") },
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
                        modifier = Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(16.dp),
                        value = form.quantityInput,
                        onValueChange = onQuantityChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_charge_energy)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                        isError = form.validationErrorKey == LedgerEntryValidationError.QUANTITY_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.QUANTITY_INVALID, form),
                    )
                    OutlinedTextField(
                        modifier = Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(16.dp),
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
                        modifier = Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(16.dp),
                        value = form.feesInput,
                        onValueChange = onFeesChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_charge_fees)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                        isError = form.validationErrorKey == LedgerEntryValidationError.FEES_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.FEES_INVALID, form),
                    )
                    OutlinedTextField(
                        modifier = Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(16.dp),
                        value = form.socStartInput,
                        onValueChange = onSocStartChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_charge_soc_start)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                        isError = form.validationErrorKey == LedgerEntryValidationError.SOC_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.SOC_INVALID, form),
                    )
                    OutlinedTextField(
                        modifier = Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(16.dp),
                        value = form.socEndInput,
                        onValueChange = onSocEndChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_charge_soc_end)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                        isError = form.validationErrorKey == LedgerEntryValidationError.SOC_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.SOC_INVALID, form),
                    )
                    OutlinedTextField(
                        modifier = Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(16.dp),
                        value = form.tariffIdInput,
                        onValueChange = onTariffIdChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_charge_tariff)) },
                        singleLine = true,
                        isError = form.validationErrorKey == LedgerEntryValidationError.TARIFF_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.TARIFF_INVALID, form),
                    )
                    OutlinedTextField(
                        modifier = Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(16.dp),
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
                    modifier = Modifier.fillMaxWidth(),
                    shape = RoundedCornerShape(16.dp),
                    value = form.totalInput,
                    onValueChange = onTotalChanged,
                    label = { Text(text = stringResource(R.string.ledger_entry_total)) },
                    prefix = { Text("R$ ") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                    isError = form.validationErrorKey == LedgerEntryValidationError.TOTAL_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.TOTAL_INVALID, form),
                )
                if (form.dataset == LedgerDataset.REFUEL) {
                    OutlinedTextField(
                        modifier = Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(16.dp),
                        value = form.unitPriceInput,
                        onValueChange = onUnitPriceChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_unit_price)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                        isError = form.validationErrorKey == LedgerEntryValidationError.UNIT_PRICE_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.UNIT_PRICE_INVALID, form),
                    )
                }
                if (form.odometerNeedsReview) {
                    Text(stringResource(R.string.ledger_review_required))
                    form.originalOdometerValue?.let { Text(stringResource(R.string.ledger_review_saved, it)) }
                    form.metresReviewKm?.let { km ->
                        TextButton(onClick = { onReviewOdometer(com.anpfuel.domain.premium.OdometerReviewChoice.AS_METRES) }, enabled = !isSaving) {
                            Text(stringResource(R.string.ledger_review_metres, km))
                        }
                    }
                    form.kilometresReviewKm?.let { km ->
                        TextButton(onClick = { onReviewOdometer(com.anpfuel.domain.premium.OdometerReviewChoice.AS_KILOMETRES) }, enabled = !isSaving) {
                            Text(stringResource(R.string.ledger_review_kilometres, km))
                        }
                    }
                    errorText(LedgerEntryValidationError.ODOMETER_REVIEW_FAILED,form)?.invoke()
                    errorText(LedgerEntryValidationError.ODOMETER_REVIEW_REQUIRED,form)?.invoke()
                }
                OutlinedTextField(
                    modifier = Modifier.fillMaxWidth(),
                    shape = RoundedCornerShape(16.dp),
                    value = form.odometerInput,
                    enabled = !form.odometerNeedsReview && !isSaving,
                    onValueChange = onOdometerChanged,
                    label = { Text(text = stringResource(R.string.ledger_entry_odometer)) },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                    isError = form.validationErrorKey == LedgerEntryValidationError.ODOMETER_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.ODOMETER_INVALID, form),
                )
                if (form.dataset == LedgerDataset.REFUEL) {
                    OutlinedTextField(
                        modifier = Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(16.dp),
                        value = form.stationInput,
                        onValueChange = onStationChanged,
                        label = { Text(text = stringResource(R.string.ledger_entry_station)) },
                        singleLine = true,
                        isError = form.validationErrorKey == LedgerEntryValidationError.STATION_INVALID,
                        supportingText = errorText(LedgerEntryValidationError.STATION_INVALID, form),
                    )
                }
                OutlinedTextField(
                    modifier = Modifier.fillMaxWidth(),
                    shape = RoundedCornerShape(16.dp),
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
            Button(onClick = onSave, enabled = !isSaving) {
                Icon(Icons.Default.Check, contentDescription = null, modifier = Modifier.padding(end = 6.dp).size(18.dp))
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
        LedgerEntryValidationError.ODOMETER_REVIEW_REQUIRED -> R.string.ledger_review_required
        LedgerEntryValidationError.ODOMETER_REVIEW_FAILED -> R.string.ledger_review_failed
        LedgerEntryValidationError.STATION_INVALID -> R.string.ledger_entry_validation_station
        LedgerEntryValidationError.FUEL_UNAVAILABLE -> R.string.ledger_entry_fuel_unavailable
        LedgerEntryValidationError.CHARGE_UNAVAILABLE -> R.string.ledger_entry_charge_unavailable
        LedgerEntryValidationError.TARIFF_INVALID -> R.string.ledger_entry_validation_tariff
        LedgerEntryValidationError.FEES_INVALID -> R.string.ledger_entry_validation_fees
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
            onFeesChanged = {},
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
