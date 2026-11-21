package com.anpfuel.app.ui.enterprise.expense

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.enterprise.attachment.AttachmentRows
import com.anpfuel.app.ui.enterprise.attachment.ReceiptCaptureCard
import com.anpfuel.app.ui.enterprise.attachment.ReceiptCaptureUi
import com.anpfuel.app.ui.enterprise.attachment.localFailureText
import com.anpfuel.app.ui.enterprise.fleet.EnterpriseAssetPicker
import com.anpfuel.app.ui.enterprise.reasonText
import com.anpfuel.app.ui.enterprise.statusText
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.enterprise.attachment.ReceiptDraft
import com.anpfuel.application.enterprise.expense.ExpenseForm
import com.anpfuel.application.enterprise.expense.ExpensePayer
import com.anpfuel.application.enterprise.expense.ExpenseViewState
import com.anpfuel.application.enterprise.fleet.MyAssetsViewState
import com.anpfuel.domain.enterprise.attachment.AttachmentView
import com.anpfuel.domain.enterprise.expense.ExpenseOperations
import com.anpfuel.domain.enterprise.expense.ExpenseReviewLine
import com.anpfuel.domain.enterprise.expense.ExpenseView
import com.anpfuel.domain.enterprise.fleet.AssetKind
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import java.math.BigDecimal
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/*
 * SCR-EXP-01 (EMP-EXP-004, B-BR-EMP-517..521): an expense is saved on the device first; its sync state comes only from
 * its own receipt and its receipt photo shows its own progress. No identifier, code or protocol word is shown, and no
 * approval or decision is ever made or implied here.
 */

/** A chosen asset from the protected FLEET-013 cache. */
internal class ExpenseTarget(val choice: EnterpriseAssetChoice, val label: String)

private sealed interface ExpenseMode {
    data object Home : ExpenseMode
    data object Picker : ExpenseMode
    class Form(val target: ExpenseTarget) : ExpenseMode
    class Receipt(val expenseId: String) : ExpenseMode
}

@Composable
internal fun ExpensePanel(
    state: ExpenseViewState,
    connected: Boolean,
    assets: MyAssetsViewState,
    onBack: () -> Unit,
    onRetry: () -> Unit,
    onOpen: (String?) -> Unit,
    onPick: () -> Unit,
    onChoose: (assetId: String, warningAccepted: Boolean) -> EnterpriseAssetChoice?,
    onSave: (EnterpriseAssetChoice, ExpenseForm, ReceiptDraft?) -> Unit,
    onSend: () -> Unit,
    modifier: Modifier = Modifier,
    capture: ReceiptCaptureUi? = null,
    onAttach: (String, ReceiptDraft) -> Unit = { _, _ -> },
) {
    var mode by remember { mutableStateOf<ExpenseMode>(ExpenseMode.Home) }
    // A durable save closes the form and drops the unsaved photo; a refusal or failure keeps both.
    LaunchedEffect(state.lastSaved) { if (state.lastSaved != null && mode is ExpenseMode.Form) { capture?.reset?.invoke(); mode = ExpenseMode.Home } }
    LaunchedEffect(state.lastAttached) { if (state.lastAttached != null && mode is ExpenseMode.Receipt) { capture?.reset?.invoke(); mode = ExpenseMode.Home } }
    LaunchedEffect(state.context?.partition, state.epoch) { capture?.reset?.invoke(); mode = ExpenseMode.Home }
    when (val current = mode) {
        ExpenseMode.Picker -> EnterpriseAssetPicker(
            state = assets, connected = connected,
            onChoose = { id, warning ->
                val choice = onChoose(id, warning)
                val asset = assets.snapshot?.assets?.firstOrNull { it.asset.id == id }?.asset
                if (choice != null && asset != null) {
                    capture?.reset?.invoke()
                    mode = ExpenseMode.Form(ExpenseTarget(choice, listOf(asset.name, asset.fleetNumber, asset.plate).filter { it.isNotBlank() }.joinToString(" · ")))
                }
            },
            onCancel = { mode = ExpenseMode.Home },
            modifier = modifier.testTag("enterprise-expense-picker"),
            accepts = { it.kind != AssetKind.IMPLEMENT },
        )
        else -> LazyColumn(modifier.testTag("enterprise-expense-panel").padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            val open = state.openExpense
            item {
                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Text(stringResource(R.string.enterprise_expenses_title), style = MaterialTheme.typography.headlineSmall,
                        modifier = Modifier.semantics { heading() })
                    TextButton(onClick = {
                        when {
                            current !is ExpenseMode.Home -> { capture?.reset?.invoke(); mode = ExpenseMode.Home }
                            open != null -> onOpen(null)
                            else -> onBack()
                        }
                    }, modifier = Modifier.fillMaxWidth().testTag("enterprise-expense-back")) {
                        Text(stringResource(if (open == null && current is ExpenseMode.Home) R.string.enterprise_expenses_back_home else R.string.enterprise_expenses_back_list))
                    }
                    Text(stringResource(R.string.enterprise_expenses_synthetic_notice), style = MaterialTheme.typography.bodySmall)
                    Feedback(state, connected)
                }
            }
            if (state.error != null || state.unlockRequired || state.unrecoverable) item {
                ExpenseCard(Modifier.testTag("enterprise-expense-error")) {
                    Notice(stringResource(R.string.enterprise_expenses_unavailable))
                    Button(onClick = onRetry, enabled = !state.busy, modifier = Modifier.fillMaxWidth().testTag("enterprise-expense-retry")) {
                        Text(stringResource(R.string.enterprise_expenses_retry))
                    }
                }
            } else if (!state.loaded) item {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    CircularProgressIndicator(Modifier.testTag("enterprise-expense-loading"))
                    Notice(stringResource(R.string.enterprise_expenses_loading))
                }
            } else when (current) {
                is ExpenseMode.Form -> item {
                    ExpenseFormCard(current.target, state.busy, capture,
                        onChange = { onPick(); mode = ExpenseMode.Picker },
                        onCancel = { capture?.reset?.invoke(); mode = ExpenseMode.Home }) { form, draft -> onSave(current.target.choice, form, draft) }
                }
                is ExpenseMode.Receipt -> if (capture != null) item {
                    ReceiptCaptureCard(stringResource(R.string.enterprise_expenses_receipt_title), capture, state.busy,
                        onSave = { onAttach(current.expenseId, it) }, onCancel = { capture.reset(); mode = ExpenseMode.Home })
                }
                else -> if (open != null) item {
                    Detail(open, state.attachments[open.fact.operationId].orEmpty(), connected, state.busy, capture != null,
                        onReceipt = { capture?.reset?.invoke(); mode = ExpenseMode.Receipt(open.fact.expenseId) }, onSend = onSend)
                } else {
                    item {
                        Button(onClick = { onPick(); mode = ExpenseMode.Picker }, enabled = !state.busy,
                            modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-expense-new")) {
                            Text(stringResource(R.string.enterprise_expenses_new))
                        }
                    }
                    if (state.expenses.isEmpty()) item {
                        ExpenseCard(Modifier.testTag("enterprise-expense-empty")) { Text(stringResource(R.string.enterprise_expenses_empty)) }
                    } else items(state.expenses, key = { it.fact.expenseId }) { view ->
                        Row(view, state.attachments[view.fact.operationId].orEmpty(), !state.busy) { onOpen(view.fact.expenseId) }
                    }
                }
            }
        }
    }
}

@Composable
private fun Feedback(state: ExpenseViewState, connected: Boolean) {
    if (!connected) Notice(stringResource(R.string.enterprise_expenses_send_offline), Modifier.testTag("enterprise-expense-offline"))
    if (state.busy) CircularProgressIndicator()
    val refusal = state.attachmentRefusal
    val message = when {
        state.saveFailed -> R.string.enterprise_expenses_save_failed
        state.captureInvalid -> R.string.enterprise_expenses_capture_invalid
        state.sendFailed -> R.string.enterprise_expenses_send_failed
        state.lastSaved != null && state.receiptNotSaved -> R.string.enterprise_expenses_saved_receipt_not_saved
        refusal != null -> localFailureText(refusal)
        state.lastSaved != null && state.lastAttached != null -> R.string.enterprise_expenses_saved_with_receipt
        state.lastSaved != null -> R.string.enterprise_expenses_saved
        state.lastAttached != null -> R.string.enterprise_receipts_saved
        else -> null
    }
    message?.let { Notice(stringResource(it), Modifier.testTag(if (state.lastSaved != null && !state.saveFailed) "enterprise-expense-saved" else "enterprise-expense-feedback")) }
    if (state.lastSaved != null && state.receiptNotSaved && refusal != null) Notice(stringResource(localFailureText(refusal)))
    if (state.attachmentsUnavailable) Notice(stringResource(R.string.enterprise_receipts_unavailable), Modifier.testTag("enterprise-expense-receipts-unavailable"))
}

/** The EMP-EXP-002 v1 direct categories (EDR-337); FUEL is a fueling and is never offered here. */
internal fun categoryText(category: String): Int = when (category) {
    "TOLL" -> R.string.enterprise_expenses_category_toll
    "PARKING" -> R.string.enterprise_expenses_category_parking
    "MAINTENANCE" -> R.string.enterprise_expenses_category_maintenance
    "LEASE" -> R.string.enterprise_expenses_category_lease
    "INSURANCE" -> R.string.enterprise_expenses_category_insurance
    "FINE" -> R.string.enterprise_expenses_category_fine
    else -> R.string.enterprise_expenses_category_other
}

/** The offered order: the most frequent field costs first; every v1 direct category exactly once. */
internal val OFFERED_CATEGORIES = listOf("TOLL", "PARKING", "MAINTENANCE", "LEASE", "INSURANCE", "FINE", "OTHER")
    .also { check(it.sorted() == ExpenseOperations.DIRECT_CATEGORIES.sorted()) }

private fun payerText(payer: String): Int =
    if (payer == ExpensePayer.EMPLOYEE.name) R.string.enterprise_expenses_payer_employee else R.string.enterprise_expenses_payer_company

private fun reviewText(line: ExpenseReviewLine): Int = when (line) {
    ExpenseReviewLine.NOT_SENT -> R.string.enterprise_expenses_review_not_sent
    ExpenseReviewLine.AWAITING_APPROVAL -> R.string.enterprise_expenses_review_awaiting
    ExpenseReviewLine.NEEDS_REVIEW -> R.string.enterprise_expenses_review_needs
    ExpenseReviewLine.REFUSED -> R.string.enterprise_expenses_review_refused
}

/** Exact centavos as reais with a decimal comma, never rounded. */
internal fun reais(centavos: Long): String = BigDecimal.valueOf(centavos, 2).toPlainString().replace('.', ',')

@Composable
private fun Row(view: ExpenseView, receipts: List<AttachmentView>, enabled: Boolean, onOpen: () -> Unit) {
    val fact = view.fact
    ExpenseCard(Modifier.testTag("enterprise-expense-row-${fact.expenseId}")) {
        Text(stringResource(categoryText(fact.category)), style = MaterialTheme.typography.titleMedium)
        Text(formatted(fact.declared.occurredAt))
        Text(stringResource(R.string.enterprise_expenses_amounts, reais(fact.paidCentavos), reais(fact.companyCentavos)))
        StatusChip(view, "enterprise-expense-row-sync-${fact.expenseId}")
        AttachmentRows(receipts, "enterprise-expense-row-receipt-${fact.expenseId}", null)
        OutlinedButton(onClick = onOpen, enabled = enabled, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)
            .testTag("enterprise-expense-open-${fact.expenseId}")) { Text(stringResource(R.string.enterprise_expenses_open)) }
    }
}

@Composable
private fun StatusChip(view: ExpenseView, tag: String) {
    val status = stringResource(statusText(view.sync))
    val reason = view.code?.takeIf { view.sync == EnterpriseSyncStatus.NEEDS_REVIEW || view.sync == EnterpriseSyncStatus.REJECTED }
        ?.let { stringResource(reasonText(it)) }
    Column(Modifier.testTag(tag).semantics(mergeDescendants = true) { stateDescription = listOfNotNull(status, reason).joinToString(". ") }) {
        Text(stringResource(R.string.enterprise_expenses_record_status, status), style = MaterialTheme.typography.labelLarge)
        reason?.let { Text(stringResource(R.string.enterprise_expenses_reason, it)) }
    }
}

@Composable
private fun Detail(view: ExpenseView, receipts: List<AttachmentView>, connected: Boolean, busy: Boolean, captureEnabled: Boolean,
    onReceipt: () -> Unit, onSend: () -> Unit) {
    val fact = view.fact
    ExpenseCard(Modifier.testTag("enterprise-expense-detail")) {
        Text(stringResource(categoryText(fact.category)), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        Text(formatted(fact.declared.occurredAt))
        Text(stringResource(R.string.enterprise_expenses_paid_value, reais(fact.paidCentavos)), modifier = Modifier.testTag("enterprise-expense-detail-paid"))
        Text(stringResource(R.string.enterprise_expenses_company_value, reais(fact.companyCentavos)), modifier = Modifier.testTag("enterprise-expense-detail-company"))
        Text(stringResource(payerText(fact.payer)))
        StatusChip(view, "enterprise-expense-sync")
        Text(stringResource(reviewText(view.review)), modifier = Modifier.testTag("enterprise-expense-review"))
        Text(stringResource(R.string.enterprise_expenses_decided_by_company), style = MaterialTheme.typography.bodySmall)
        AttachmentRows(receipts, "enterprise-expense-receipt", if (captureEnabled) onReceipt else null)
        if (captureEnabled) OutlinedButton(onClick = onReceipt, enabled = !busy,
            modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-expense-add-receipt")) {
            Text(stringResource(R.string.enterprise_expenses_add_receipt))
        }
        OutlinedButton(onClick = onSend, enabled = connected && !busy, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-expense-send")) {
            Text(stringResource(R.string.enterprise_expenses_send_now))
        }
    }
}

@Composable
private fun Choice(text: String, selected: Boolean, tag: String, onSelect: () -> Unit) {
    androidx.compose.foundation.layout.Row(Modifier.fillMaxWidth().heightIn(min = 48.dp).selectable(selected, onClick = onSelect, role = Role.RadioButton).testTag(tag),
        verticalAlignment = Alignment.CenterVertically) {
        RadioButton(selected = selected, onClick = null)
        Text(text, modifier = Modifier.padding(start = 8.dp))
    }
}

@Composable
private fun ExpenseFormCard(target: ExpenseTarget, busy: Boolean, capture: ReceiptCaptureUi?, onChange: () -> Unit, onCancel: () -> Unit,
    onSave: (ExpenseForm, ReceiptDraft?) -> Unit) {
    var category by remember(target) { mutableStateOf<String?>(null) }
    var paid by remember(target) { mutableStateOf("") }
    var company by remember(target) { mutableStateOf("") }
    var payer by remember(target) { mutableStateOf<ExpensePayer?>(null) }
    // Every field is explicit (B-BR-EMP-519): no category, payer or company share is ever assumed.
    val form = category?.let { c -> payer?.takeIf { paid.isNotBlank() && company.isNotBlank() }?.let { p -> ExpenseForm(c, paid, company, p) } }
    ExpenseCard(Modifier.testTag("enterprise-expense-form")) {
        Text(stringResource(R.string.enterprise_expenses_form_title), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.enterprise_expenses_asset_chosen, target.label), modifier = Modifier.testTag("enterprise-expense-chosen"))
        TextButton(onClick = onChange, enabled = !busy, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.enterprise_expenses_change_asset)) }
        Text(stringResource(R.string.enterprise_expenses_category), style = MaterialTheme.typography.titleSmall)
        OFFERED_CATEGORIES.forEach { c -> Choice(stringResource(categoryText(c)), category == c, "enterprise-expense-category-$c") { category = c } }
        OutlinedTextField(value = paid, onValueChange = { paid = it }, label = { Text(stringResource(R.string.enterprise_expenses_paid)) },
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal), singleLine = true, modifier = Modifier.fillMaxWidth().testTag("enterprise-expense-paid"))
        OutlinedTextField(value = company, onValueChange = { company = it }, label = { Text(stringResource(R.string.enterprise_expenses_company)) },
            supportingText = { Text(stringResource(R.string.enterprise_expenses_company_hint)) },
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal), singleLine = true, modifier = Modifier.fillMaxWidth().testTag("enterprise-expense-company"))
        Text(stringResource(R.string.enterprise_expenses_payer), style = MaterialTheme.typography.titleSmall)
        ExpensePayer.entries.forEach { p -> Choice(stringResource(payerText(p.name)), payer == p, "enterprise-expense-payer-${p.name}") { payer = p } }
        if (capture != null) ReceiptCaptureCard(stringResource(R.string.enterprise_expenses_receipt_optional), capture, busy,
            onSave = null, onCancel = null, onUsePaid = { paid = it })
        Button(onClick = { form?.let { onSave(it, capture?.state?.draft) } }, enabled = !busy && form != null,
            modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-expense-save")) { Text(stringResource(R.string.enterprise_expenses_save)) }
        TextButton(onClick = onCancel, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-expense-cancel")) {
            Text(stringResource(R.string.enterprise_expenses_cancel))
        }
    }
}

/** Home entry card; it opens the protected expenses, never a server read by itself. */
@Composable
internal fun ExpensesHomeCard(onOpen: () -> Unit, modifier: Modifier = Modifier) {
    ExpenseCard(modifier.testTag("enterprise-expenses-home")) {
        Text(stringResource(R.string.enterprise_expenses_title), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.enterprise_expenses_home_copy))
        Button(onClick = onOpen, modifier = Modifier.fillMaxWidth().testTag("enterprise-expenses-open")) { Text(stringResource(R.string.enterprise_expenses_open_list)) }
    }
}

@Composable
private fun ExpenseCard(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    val fill = MaterialTheme.colorScheme.surfaceContainerLow
    Card(modifier.fillMaxWidth(), shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = fill), border = AnpContainerDefaults.borderStroke(fill)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) { content() }
    }
}

@Composable
private fun Notice(message: String, modifier: Modifier = Modifier) {
    Text(message, modifier = modifier.semantics { liveRegion = LiveRegionMode.Polite })
}

@Composable
private fun formatted(instant: Instant): String {
    val locale = LocalConfiguration.current.locales[0]
    val zone = remember { ZoneId.systemDefault() }
    val formatter = remember(locale, zone) { DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM).withLocale(locale).withZone(zone) }
    return formatter.format(instant)
}
