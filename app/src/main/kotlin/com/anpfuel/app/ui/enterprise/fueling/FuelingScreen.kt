package com.anpfuel.app.ui.enterprise.fueling

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
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
import androidx.compose.runtime.mutableIntStateOf
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
import com.anpfuel.application.enterprise.fleet.MyAssetsViewState
import com.anpfuel.application.enterprise.fueling.FuelingForm
import com.anpfuel.application.enterprise.fueling.FuelingViewState
import com.anpfuel.application.enterprise.fueling.FuelingWarnings
import com.anpfuel.application.enterprise.trip.TripCapture
import com.anpfuel.application.enterprise.trip.TripMeterChoice
import com.anpfuel.domain.enterprise.fleet.AssetKind
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import com.anpfuel.domain.enterprise.fueling.FillLevel
import com.anpfuel.domain.enterprise.fueling.FuelProduct
import com.anpfuel.domain.enterprise.fueling.FuelProducts
import com.anpfuel.domain.enterprise.fueling.FuelUnit
import com.anpfuel.domain.enterprise.fueling.FuelingEvidence
import com.anpfuel.domain.enterprise.fueling.FuelingReviewLine
import com.anpfuel.domain.enterprise.fueling.FuelingView
import com.anpfuel.domain.enterprise.fueling.Payer
import com.anpfuel.domain.enterprise.fueling.PaymentMethod
import com.anpfuel.domain.enterprise.meter.MeterEntryForm
import com.anpfuel.domain.enterprise.meter.MeterKind
import com.anpfuel.domain.enterprise.meter.ReadingFlag
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import java.math.BigDecimal
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/*
 * SCR-FUEL-01/02 (EMP-FUEL-011, B-BR-EMP-455..460): a fueling is saved on the device first; its sync state comes only
 * from its own receipt and its receipt photo shows its own progress. No identifier, code or protocol word is shown.
 */

/** A chosen asset with the products and meters the protected FLEET-013 cache offers for it. */
internal class FuelingTarget(val choice: EnterpriseAssetChoice, val label: String, val products: List<FuelProduct>, val meters: List<TripMeterChoice>)

private sealed interface FuelingMode {
    data object Home : FuelingMode
    data object Picker : FuelingMode
    class Form(val target: FuelingTarget) : FuelingMode
    class Receipt(val fuelingId: String) : FuelingMode
}

/** The payment methods offered offline (EDR-296): an agreement voucher is online-only and internal stock belongs to F6. */
internal val OFFLINE_PAYMENTS = listOf(PaymentMethod.FUEL_CARD, PaymentMethod.CREDIT_CARD, PaymentMethod.DEBIT_CARD, PaymentMethod.PIX,
    PaymentMethod.CASH, PaymentMethod.SUPPLIER_ACCOUNT, PaymentMethod.OTHER)

/** The products of the asset's cached tanks; every catalog product when the cache lists none (BR-FUEL-016 warns on the server). */
internal fun offeredProducts(codes: List<String>): List<FuelProduct> =
    codes.mapNotNull { code -> FuelProduct.entries.firstOrNull { it.name == code } }.ifEmpty { FuelProducts.all() }

@Composable
internal fun FuelingPanel(
    state: FuelingViewState,
    connected: Boolean,
    assets: MyAssetsViewState,
    onBack: () -> Unit,
    onRetry: () -> Unit,
    onOpen: (String?) -> Unit,
    onPick: () -> Unit,
    onChoose: (assetId: String, warningAccepted: Boolean) -> EnterpriseAssetChoice?,
    onSave: (EnterpriseAssetChoice, FuelingForm, ReceiptDraft?) -> Unit,
    warnings: (FuelingForm) -> FuelingWarnings,
    onSend: () -> Unit,
    modifier: Modifier = Modifier,
    capture: ReceiptCaptureUi? = null,
    onAttach: (String, ReceiptDraft) -> Unit = { _, _ -> },
) {
    var mode by remember { mutableStateOf<FuelingMode>(FuelingMode.Home) }
    // A durable save closes the form and drops the unsaved photo; a refusal or failure keeps both.
    LaunchedEffect(state.lastSaved) { if (state.lastSaved != null && mode is FuelingMode.Form) { capture?.reset?.invoke(); mode = FuelingMode.Home } }
    LaunchedEffect(state.lastAttached) { if (state.lastAttached != null && mode is FuelingMode.Receipt) { capture?.reset?.invoke(); mode = FuelingMode.Home } }
    LaunchedEffect(state.context?.partition, state.epoch) { capture?.reset?.invoke(); mode = FuelingMode.Home }
    when (val current = mode) {
        FuelingMode.Picker -> EnterpriseAssetPicker(
            state = assets, connected = connected,
            onChoose = { id, warning ->
                val choice = onChoose(id, warning)
                val asset = assets.snapshot?.assets?.firstOrNull { it.asset.id == id }?.asset
                if (choice != null && asset != null) {
                    capture?.reset?.invoke()
                    mode = FuelingMode.Form(FuelingTarget(choice, listOf(asset.name, asset.fleetNumber, asset.plate).filter { it.isNotBlank() }.joinToString(" · "),
                        offeredProducts(asset.products), choice.meters.map { TripMeterChoice(it.id, it.segmentId, MeterKind.parse(it.kind.name), it.modulus) }))
                }
            },
            onCancel = { mode = FuelingMode.Home },
            modifier = modifier.testTag("enterprise-fueling-picker"),
            accepts = { it.kind != AssetKind.IMPLEMENT },
        )
        else -> LazyColumn(modifier.testTag("enterprise-fueling-panel").padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            val open = state.openFueling
            item {
                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Text(stringResource(R.string.enterprise_fuelings_title), style = MaterialTheme.typography.headlineSmall,
                        modifier = Modifier.semantics { heading() })
                    TextButton(onClick = {
                        when {
                            current !is FuelingMode.Home -> { capture?.reset?.invoke(); mode = FuelingMode.Home }
                            open != null -> onOpen(null)
                            else -> onBack()
                        }
                    }, modifier = Modifier.fillMaxWidth().testTag("enterprise-fueling-back")) {
                        Text(stringResource(if (open == null && current is FuelingMode.Home) R.string.enterprise_fuelings_back_home else R.string.enterprise_fuelings_back_list))
                    }
                    Text(stringResource(R.string.enterprise_fuelings_synthetic_notice), style = MaterialTheme.typography.bodySmall)
                    Feedback(state, connected)
                }
            }
            if (state.error != null || state.unlockRequired || state.unrecoverable) item {
                FuelingCard(Modifier.testTag("enterprise-fueling-error")) {
                    Notice(stringResource(R.string.enterprise_fuelings_unavailable))
                    Button(onClick = onRetry, enabled = !state.busy, modifier = Modifier.fillMaxWidth().testTag("enterprise-fueling-retry")) {
                        Text(stringResource(R.string.enterprise_fuelings_retry))
                    }
                }
            } else if (!state.loaded) item {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    CircularProgressIndicator(Modifier.testTag("enterprise-fueling-loading"))
                    Notice(stringResource(R.string.enterprise_fuelings_loading))
                }
            } else when (current) {
                is FuelingMode.Form -> item {
                    FuelingFormCard(current.target, state.busy, capture, warnings,
                        onChange = { onPick(); mode = FuelingMode.Picker },
                        onCancel = { capture?.reset?.invoke(); mode = FuelingMode.Home }) { form, draft -> onSave(current.target.choice, form, draft) }
                }
                is FuelingMode.Receipt -> if (capture != null) item {
                    ReceiptCaptureCard(stringResource(R.string.enterprise_fuelings_receipt_title), capture, state.busy,
                        onSave = { onAttach(current.fuelingId, it) }, onCancel = { capture.reset(); mode = FuelingMode.Home })
                }
                else -> if (open != null) item {
                    Detail(open, state.attachments[open.fact.operationId].orEmpty(), connected, state.busy, capture != null,
                        onReceipt = { capture?.reset?.invoke(); mode = FuelingMode.Receipt(open.fact.fuelingId) }, onSend = onSend)
                } else {
                    item {
                        Button(onClick = { onPick(); mode = FuelingMode.Picker }, enabled = !state.busy,
                            modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-fueling-new")) {
                            Text(stringResource(R.string.enterprise_fuelings_new))
                        }
                    }
                    if (state.fuelings.isEmpty()) item {
                        FuelingCard(Modifier.testTag("enterprise-fueling-empty")) { Text(stringResource(R.string.enterprise_fuelings_empty)) }
                    } else items(state.fuelings, key = { it.fact.fuelingId }) { view ->
                        Row(view, state.attachments[view.fact.operationId].orEmpty(), !state.busy) { onOpen(view.fact.fuelingId) }
                    }
                }
            }
        }
    }
}

@Composable
private fun Feedback(state: FuelingViewState, connected: Boolean) {
    if (!connected) Notice(stringResource(R.string.enterprise_fuelings_send_offline), Modifier.testTag("enterprise-fueling-offline"))
    if (state.busy) CircularProgressIndicator()
    val refusal = state.attachmentRefusal
    val message = when {
        state.saveFailed -> R.string.enterprise_fuelings_save_failed
        state.captureInvalid -> R.string.enterprise_fuelings_capture_invalid
        state.responsibleUnknown -> R.string.enterprise_fuelings_responsible_unknown
        state.sendFailed -> R.string.enterprise_fuelings_send_failed
        state.lastSaved != null && state.receiptNotSaved -> R.string.enterprise_fuelings_saved_receipt_not_saved
        refusal != null -> localFailureText(refusal)
        state.lastSaved != null && state.lastAttached != null -> R.string.enterprise_fuelings_saved_with_receipt
        state.lastSaved != null -> R.string.enterprise_fuelings_saved
        state.lastAttached != null -> R.string.enterprise_receipts_saved
        else -> null
    }
    message?.let { Notice(stringResource(it), Modifier.testTag(if (state.lastSaved != null && !state.saveFailed) "enterprise-fueling-saved" else "enterprise-fueling-feedback")) }
    if (state.lastSaved != null && state.receiptNotSaved && refusal != null) Notice(stringResource(localFailureText(refusal)))
    if (state.attachmentsUnavailable) Notice(stringResource(R.string.enterprise_receipts_unavailable), Modifier.testTag("enterprise-fueling-receipts-unavailable"))
}

private fun productText(product: FuelProduct): Int = when (product) {
    FuelProduct.ETHANOL -> R.string.enterprise_fuelings_product_ethanol
    FuelProduct.GASOLINE_REGULAR -> R.string.enterprise_fuelings_product_gasoline_regular
    FuelProduct.GASOLINE_ADDITIVED -> R.string.enterprise_fuelings_product_gasoline_additived
    FuelProduct.GASOLINE_PREMIUM_GRADE -> R.string.enterprise_fuelings_product_gasoline_premium
    FuelProduct.DIESEL_S10 -> R.string.enterprise_fuelings_product_diesel_s10
    FuelProduct.DIESEL_S500 -> R.string.enterprise_fuelings_product_diesel_s500
    FuelProduct.CNG -> R.string.enterprise_fuelings_product_cng
    FuelProduct.ELECTRICITY -> R.string.enterprise_fuelings_product_electricity
    FuelProduct.ARLA_32 -> R.string.enterprise_fuelings_product_arla
}

private fun unitText(unit: FuelUnit): Int = when (unit) {
    FuelUnit.L -> R.string.enterprise_fuelings_unit_l
    FuelUnit.M3 -> R.string.enterprise_fuelings_unit_m3
    FuelUnit.KWH -> R.string.enterprise_fuelings_unit_kwh
}

private fun fillText(fill: FillLevel): Int = when (fill) {
    FillLevel.FULL -> R.string.enterprise_fuelings_fill_full
    FillLevel.PARTIAL -> R.string.enterprise_fuelings_fill_partial
    FillLevel.UNKNOWN -> R.string.enterprise_fuelings_fill_unknown
}

private fun payerText(payer: Payer): Int = when (payer) {
    Payer.COMPANY -> R.string.enterprise_fuelings_payer_company
    Payer.EMPLOYEE -> R.string.enterprise_fuelings_payer_employee
}

private fun paymentText(method: PaymentMethod): Int = when (method) {
    PaymentMethod.CASH -> R.string.enterprise_fuelings_payment_cash
    PaymentMethod.PIX -> R.string.enterprise_fuelings_payment_pix
    PaymentMethod.DEBIT_CARD -> R.string.enterprise_fuelings_payment_debit
    PaymentMethod.CREDIT_CARD -> R.string.enterprise_fuelings_payment_credit
    PaymentMethod.FUEL_CARD -> R.string.enterprise_fuelings_payment_fuel_card
    PaymentMethod.SUPPLIER_ACCOUNT -> R.string.enterprise_fuelings_payment_supplier
    PaymentMethod.AGREEMENT_VOUCHER, PaymentMethod.INTERNAL_STOCK, PaymentMethod.OTHER -> R.string.enterprise_fuelings_payment_other
}

private fun reviewText(line: FuelingReviewLine): Int = when (line) {
    FuelingReviewLine.NOT_SENT -> R.string.enterprise_fuelings_review_not_sent
    FuelingReviewLine.AWAITING_APPROVAL -> R.string.enterprise_fuelings_review_awaiting
    FuelingReviewLine.NEEDS_REVIEW -> R.string.enterprise_fuelings_review_needs
    FuelingReviewLine.REFUSED -> R.string.enterprise_fuelings_review_refused
}

private fun decimal(units: Long, scale: Int): String = BigDecimal.valueOf(units, scale).stripTrailingZeros().toPlainString().replace('.', ',')

@Composable
private fun Row(view: FuelingView, receipts: List<com.anpfuel.domain.enterprise.attachment.AttachmentView>, enabled: Boolean, onOpen: () -> Unit) {
    val fact = view.fact
    FuelingCard(Modifier.testTag("enterprise-fueling-row-${fact.fuelingId}")) {
        Text(stringResource(productText(fact.quantity.product)), style = MaterialTheme.typography.titleMedium)
        Text(formatted(fact.declared.occurredAt))
        Text(stringResource(R.string.enterprise_fuelings_quantity_paid, decimal(fact.quantity.amount, 3), stringResource(unitText(fact.quantity.unit)),
            decimal(fact.amounts.paid, 2)))
        StatusChip(view, "enterprise-fueling-row-sync-${fact.fuelingId}")
        AttachmentRows(receipts, "enterprise-fueling-row-receipt-${fact.fuelingId}", null)
        OutlinedButton(onClick = onOpen, enabled = enabled, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)
            .testTag("enterprise-fueling-open-${fact.fuelingId}")) { Text(stringResource(R.string.enterprise_fuelings_open)) }
    }
}

@Composable
private fun StatusChip(view: FuelingView, tag: String) {
    val status = stringResource(statusText(view.sync))
    val reason = view.code?.takeIf { view.sync == EnterpriseSyncStatus.NEEDS_REVIEW || view.sync == EnterpriseSyncStatus.REJECTED }
        ?.let { stringResource(reasonText(it)) }
    Column(Modifier.testTag(tag).semantics(mergeDescendants = true) { stateDescription = listOfNotNull(status, reason).joinToString(". ") }) {
        Text(stringResource(R.string.enterprise_fuelings_record_status, status), style = MaterialTheme.typography.labelLarge)
        reason?.let { Text(stringResource(R.string.enterprise_fuelings_reason, it)) }
    }
}

@Composable
private fun Detail(view: FuelingView, receipts: List<com.anpfuel.domain.enterprise.attachment.AttachmentView>, connected: Boolean, busy: Boolean,
    captureEnabled: Boolean, onReceipt: () -> Unit, onSend: () -> Unit) {
    val fact = view.fact
    FuelingCard(Modifier.testTag("enterprise-fueling-detail")) {
        Text(stringResource(productText(fact.quantity.product)), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        Text(formatted(fact.declared.occurredAt))
        Text(stringResource(R.string.enterprise_fuelings_quantity_value, decimal(fact.quantity.amount, 3), stringResource(unitText(fact.quantity.unit))))
        Text(stringResource(R.string.enterprise_fuelings_paid_value, decimal(fact.amounts.paid, 2)), modifier = Modifier.testTag("enterprise-fueling-detail-paid"))
        fact.amounts.discount?.let { Text(stringResource(R.string.enterprise_fuelings_discount_value, decimal(it, 2))) }
        fact.amounts.unitPrice?.let { Text(stringResource(R.string.enterprise_fuelings_price_value, decimal(it, 3), stringResource(unitText(fact.quantity.unit)))) }
        Text(stringResource(fillText(fact.fill)))
        Text(stringResource(payerText(fact.payer)) + " · " + stringResource(paymentText(fact.payment)))
        when (val evidence = fact.evidence) {
            is FuelingEvidence.Absent -> Text(stringResource(R.string.enterprise_fuelings_reading_absent_reason, evidence.reason))
            is FuelingEvidence.Reading -> Text(if (evidence.value.kind == MeterKind.ODOMETER)
                stringResource(R.string.enterprise_fuelings_reading_km, decimal(evidence.value.units, 3))
                else stringResource(R.string.enterprise_fuelings_reading_hours, BigDecimal.valueOf(evidence.value.units).divide(BigDecimal.valueOf(3600), 2,
                    java.math.RoundingMode.DOWN).stripTrailingZeros().toPlainString().replace('.', ',')))
        }
        fact.placeName?.let { Text(stringResource(R.string.enterprise_fuelings_place_value, it)) }
        StatusChip(view, "enterprise-fueling-sync")
        Text(stringResource(reviewText(view.review)), modifier = Modifier.testTag("enterprise-fueling-review"))
        AttachmentRows(receipts, "enterprise-fueling-receipt", if (captureEnabled) onReceipt else null)
        if (captureEnabled) OutlinedButton(onClick = onReceipt, enabled = !busy,
            modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-fueling-add-receipt")) {
            Text(stringResource(R.string.enterprise_fuelings_add_receipt))
        }
        OutlinedButton(onClick = onSend, enabled = connected && !busy, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-fueling-send")) {
            Text(stringResource(R.string.enterprise_fuelings_send_now))
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
private fun FuelingFormCard(target: FuelingTarget, busy: Boolean, capture: ReceiptCaptureUi?, warnings: (FuelingForm) -> FuelingWarnings, onChange: () -> Unit,
    onCancel: () -> Unit, onSave: (FuelingForm, ReceiptDraft?) -> Unit) {
    var product by remember(target) { mutableStateOf(target.products.first()) }
    var quantity by remember(target) { mutableStateOf("") }
    var paid by remember(target) { mutableStateOf("") }
    var discount by remember(target) { mutableStateOf("") }
    var price by remember(target) { mutableStateOf("") }
    var fill by remember(target) { mutableStateOf(FillLevel.FULL) }
    var payer by remember(target) { mutableStateOf(Payer.COMPANY) }
    var payment by remember(target) { mutableStateOf(PaymentMethod.FUEL_CARD) }
    var meter by remember(target) { mutableIntStateOf(if (target.meters.isEmpty()) -1 else 0) }
    var reading by remember(target) { mutableStateOf("") }
    var reason by remember(target) { mutableStateOf("") }
    var place by remember(target) { mutableStateOf("") }
    val capture1 = if (meter < 0) reason.takeIf { it.isNotBlank() }?.let { TripCapture.Absent(it) }
        else reading.trim().takeIf { it.isNotEmpty() }?.let { raw ->
            val m = target.meters[meter]
            TripCapture.Reading(m, if (m.kind == MeterKind.ODOMETER) MeterEntryForm.KM else MeterEntryForm.HOURS, raw.replace(',', '.'))
        }
    val form = capture1?.takeIf { quantity.isNotBlank() && paid.isNotBlank() }?.let {
        FuelingForm(product, quantity, paid, discount.takeIf { d -> d.isNotBlank() }, price.takeIf { p -> p.isNotBlank() }, fill, payer, payment, it,
            place.takeIf { p -> p.isNotBlank() })
    }
    FuelingCard(Modifier.testTag("enterprise-fueling-form")) {
        Text(stringResource(R.string.enterprise_fuelings_form_title), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.enterprise_fuelings_asset_chosen, target.label), modifier = Modifier.testTag("enterprise-fueling-chosen"))
        TextButton(onClick = onChange, enabled = !busy, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.enterprise_fuelings_change_asset)) }
        Text(stringResource(R.string.enterprise_fuelings_product), style = MaterialTheme.typography.titleSmall)
        target.products.forEach { p -> Choice(stringResource(productText(p)), product == p, "enterprise-fueling-product-${p.name}") { product = p } }
        val unit = stringResource(unitText(product.unit))
        OutlinedTextField(value = quantity, onValueChange = { quantity = it }, label = { Text(stringResource(R.string.enterprise_fuelings_quantity, unit)) },
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal), singleLine = true, modifier = Modifier.fillMaxWidth().testTag("enterprise-fueling-quantity"))
        OutlinedTextField(value = paid, onValueChange = { paid = it }, label = { Text(stringResource(R.string.enterprise_fuelings_paid)) },
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal), singleLine = true, modifier = Modifier.fillMaxWidth().testTag("enterprise-fueling-paid"))
        OutlinedTextField(value = price, onValueChange = { price = it }, label = { Text(stringResource(R.string.enterprise_fuelings_unit_price, unit)) },
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal), singleLine = true, modifier = Modifier.fillMaxWidth().testTag("enterprise-fueling-price"))
        OutlinedTextField(value = discount, onValueChange = { discount = it }, label = { Text(stringResource(R.string.enterprise_fuelings_discount)) },
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal), singleLine = true, modifier = Modifier.fillMaxWidth().testTag("enterprise-fueling-discount"))
        Text(stringResource(R.string.enterprise_fuelings_fill), style = MaterialTheme.typography.titleSmall)
        FillLevel.entries.forEach { f -> Choice(stringResource(fillText(f)), fill == f, "enterprise-fueling-fill-${f.name}") { fill = f } }
        Text(stringResource(R.string.enterprise_fuelings_payer), style = MaterialTheme.typography.titleSmall)
        Payer.entries.forEach { p -> Choice(stringResource(payerText(p)), payer == p, "enterprise-fueling-payer-${p.name}") { payer = p } }
        Text(stringResource(R.string.enterprise_fuelings_payment), style = MaterialTheme.typography.titleSmall)
        OFFLINE_PAYMENTS.forEach { m -> Choice(stringResource(paymentText(m)), payment == m, "enterprise-fueling-payment-${m.name}") { payment = m } }
        Text(stringResource(R.string.enterprise_fuelings_reading), style = MaterialTheme.typography.titleSmall)
        if (target.meters.isEmpty()) Text(stringResource(R.string.enterprise_fuelings_no_meter))
        target.meters.forEachIndexed { index, m ->
            Choice(stringResource(if (m.kind == MeterKind.ODOMETER) R.string.enterprise_fuelings_meter_odometer else R.string.enterprise_fuelings_meter_hours),
                meter == index, "enterprise-fueling-meter-$index") { meter = index }
        }
        Choice(stringResource(R.string.enterprise_fuelings_reading_absent), meter < 0, "enterprise-fueling-meter-absent") { meter = -1 }
        if (meter >= 0) OutlinedTextField(value = reading, onValueChange = { reading = it }, label = { Text(stringResource(R.string.enterprise_fuelings_reading)) },
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal), singleLine = true, modifier = Modifier.fillMaxWidth().testTag("enterprise-fueling-reading-value"))
        else OutlinedTextField(value = reason, onValueChange = { reason = it }, label = { Text(stringResource(R.string.enterprise_fuelings_absence_reason)) },
            modifier = Modifier.fillMaxWidth().testTag("enterprise-fueling-reading-reason"))
        OutlinedTextField(value = place, onValueChange = { place = it }, label = { Text(stringResource(R.string.enterprise_fuelings_place)) },
            singleLine = true, modifier = Modifier.fillMaxWidth().testTag("enterprise-fueling-place"))
        // Warnings inform, people decide (BR-FUEL-017): never a block.
        val flags = form?.let(warnings)
        if (flags?.arithmeticMismatch == true) Notice(stringResource(R.string.enterprise_fuelings_warning_arithmetic), Modifier.testTag("enterprise-fueling-warning-arithmetic"))
        if (flags != null && ReadingFlag.READING_BELOW_PREVIOUS in flags.reading) Notice(stringResource(R.string.enterprise_fuelings_warning_below), Modifier.testTag("enterprise-fueling-warning-reading"))
        else if (flags != null && ReadingFlag.IMPLAUSIBLE_RATE in flags.reading) Notice(stringResource(R.string.enterprise_fuelings_warning_rate), Modifier.testTag("enterprise-fueling-warning-reading"))
        if (capture != null) ReceiptCaptureCard(stringResource(R.string.enterprise_fuelings_receipt_optional), capture, busy, onSave = null, onCancel = null)
        Button(onClick = { form?.let { onSave(it, capture?.state?.draft) } }, enabled = !busy && form != null,
            modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-fueling-save")) { Text(stringResource(R.string.enterprise_fuelings_save)) }
        TextButton(onClick = onCancel, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-fueling-cancel")) {
            Text(stringResource(R.string.enterprise_fuelings_cancel))
        }
    }
}

/** Home entry card; it opens the protected fuelings, never a server read by itself. */
@Composable
internal fun FuelingsHomeCard(onOpen: () -> Unit, modifier: Modifier = Modifier) {
    FuelingCard(modifier.testTag("enterprise-fuelings-home")) {
        Text(stringResource(R.string.enterprise_fuelings_title), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.enterprise_fuelings_home_copy))
        Button(onClick = onOpen, modifier = Modifier.fillMaxWidth().testTag("enterprise-fuelings-open")) { Text(stringResource(R.string.enterprise_fuelings_open_list)) }
    }
}

@Composable
private fun FuelingCard(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
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
