package com.anpfuel.app.ui.enterprise.trip

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
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
import com.anpfuel.app.ui.enterprise.fleet.EnterpriseAssetPicker
import com.anpfuel.app.ui.enterprise.reasonText
import com.anpfuel.app.ui.enterprise.statusText
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.enterprise.fleet.MyAssetsViewState
import com.anpfuel.application.enterprise.trip.TripCapture
import com.anpfuel.application.enterprise.trip.TripMeterChoice
import com.anpfuel.application.enterprise.trip.TripViewState
import com.anpfuel.domain.enterprise.fleet.AssetKind
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import com.anpfuel.domain.enterprise.meter.MeterEntryForm
import com.anpfuel.domain.enterprise.meter.MeterKind
import com.anpfuel.domain.enterprise.meter.ReadingFlag
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.trip.TripAssetLabel
import com.anpfuel.domain.enterprise.trip.TripEvidence
import com.anpfuel.domain.enterprise.trip.TripLocalFailure
import com.anpfuel.domain.enterprise.trip.TripLocalState
import com.anpfuel.domain.enterprise.trip.TripLogbookType
import com.anpfuel.domain.enterprise.trip.TripOperationFact
import com.anpfuel.domain.enterprise.trip.TripRecordView
import com.anpfuel.domain.enterprise.trip.TripReviewLine
import com.anpfuel.domain.enterprise.trip.TripView
import java.math.BigDecimal
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/*
 * SCR-TRIP-01 (EMP-TRIP-005): start, typed logbook events, finish and "ready to send", all saved on the device first.
 * Each record shows only its own sync state; the operational line is local and the review line is what a receipt proves.
 * No identifier, code or protocol word is shown.
 */

private sealed interface TripMode {
    data object Home : TripMode
    data object Picker : TripMode
    class Start(val choice: EnterpriseAssetChoice, val label: TripAssetLabel) : TripMode
    class Event(val tripId: String) : TripMode
    class Finish(val tripId: String) : TripMode
}

/** The meters the driver may read for a choice: the picker's in-service meters, by kind. */
internal fun meterChoices(choice: EnterpriseAssetChoice): List<TripMeterChoice> =
    choice.meters.map { TripMeterChoice(it.id, it.segmentId, MeterKind.parse(it.kind.name), it.modulus) }

@Composable
internal fun TripPanel(
    state: TripViewState,
    connected: Boolean,
    assets: MyAssetsViewState,
    onBack: () -> Unit,
    onRetry: () -> Unit,
    onOpenTrip: (String?) -> Unit,
    onPick: () -> Unit,
    onChoose: (assetId: String, warningAccepted: Boolean) -> EnterpriseAssetChoice?,
    onStart: (EnterpriseAssetChoice, TripAssetLabel, TripCapture) -> Unit,
    onLog: (String, TripLogbookType, String?) -> Unit,
    onFinish: (String, TripCapture) -> Unit,
    finishWarnings: (String, TripCapture) -> List<ReadingFlag>,
    onReady: (String) -> Unit,
    onSend: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var mode by remember { mutableStateOf<TripMode>(TripMode.Home) }
    // A durable save closes the form it came from; a refusal or failure keeps the entered values.
    LaunchedEffect(state.lastSaved) { if (state.lastSaved != null && mode !is TripMode.Picker) mode = TripMode.Home }
    LaunchedEffect(state.context?.partition, state.epoch) { mode = TripMode.Home }
    when (val current = mode) {
        TripMode.Picker -> EnterpriseAssetPicker(
            state = assets, connected = connected,
            onChoose = { id, warning ->
                val choice = onChoose(id, warning)
                val asset = assets.snapshot?.assets?.firstOrNull { it.asset.id == id }?.asset
                if (choice != null && asset != null) mode = TripMode.Start(choice, TripAssetLabel(asset.name, asset.fleetNumber, asset.plate))
            },
            onCancel = { mode = TripMode.Home },
            modifier = modifier.testTag("enterprise-trip-picker"),
            accepts = { it.kind != AssetKind.IMPLEMENT },
        )
        else -> LazyColumn(modifier.testTag("enterprise-trip-panel").padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            val open = state.openTrip
            item {
                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Text(stringResource(R.string.enterprise_trips_title), style = MaterialTheme.typography.headlineSmall,
                        modifier = Modifier.semantics { heading() })
                    TextButton(onClick = {
                        when {
                            current !is TripMode.Home -> mode = TripMode.Home
                            open != null -> onOpenTrip(null)
                            else -> onBack()
                        }
                    }, modifier = Modifier.fillMaxWidth().testTag("enterprise-trip-back")) {
                        Text(stringResource(if (open == null && current is TripMode.Home) R.string.enterprise_trips_back_home else R.string.enterprise_trips_back_list))
                    }
                    Text(stringResource(R.string.enterprise_trips_logbook_notice), style = MaterialTheme.typography.bodySmall)
                    Feedback(state, connected)
                }
            }
            if (state.error != null || state.unlockRequired || state.unrecoverable) item {
                TripCard(Modifier.testTag("enterprise-trip-error")) {
                    Notice(stringResource(R.string.enterprise_trips_unavailable))
                    Button(onClick = onRetry, enabled = !state.busy, modifier = Modifier.fillMaxWidth().testTag("enterprise-trip-retry")) {
                        Text(stringResource(R.string.enterprise_trips_retry))
                    }
                }
            } else if (!state.loaded) item {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    CircularProgressIndicator(Modifier.testTag("enterprise-trip-loading"))
                    Notice(stringResource(R.string.enterprise_trips_loading))
                }
            } else when (current) {
                is TripMode.Start -> item { StartForm(current, state.busy, onStart, onChange = { onPick(); mode = TripMode.Picker },
                    onCancel = { mode = TripMode.Home }) }
                is TripMode.Event -> item { EventForm(state.busy, onCancel = { mode = TripMode.Home }) { type, text -> onLog(current.tripId, type, text) } }
                is TripMode.Finish -> state.trips.firstOrNull { it.trip.tripId == current.tripId }?.let { trip ->
                    item { FinishForm(trip, assets, state.busy, finishWarnings, onCancel = { mode = TripMode.Home }) { onFinish(current.tripId, it) } }
                }
                else -> if (open != null) {
                    tripDetail(open, connected, state.busy, onEvent = { mode = TripMode.Event(open.trip.tripId) },
                        onFinish = { mode = TripMode.Finish(open.trip.tripId) }, onReady = { onReady(open.trip.tripId) }, onSend = onSend)
                } else {
                    item {
                        Button(onClick = { onPick(); mode = TripMode.Picker }, enabled = !state.busy,
                            modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-trip-start")) {
                            Text(stringResource(R.string.enterprise_trips_start))
                        }
                    }
                    if (state.trips.isEmpty()) item {
                        TripCard(Modifier.testTag("enterprise-trip-empty")) { Text(stringResource(R.string.enterprise_trips_empty)) }
                    } else items(state.trips, key = { it.trip.tripId }) { trip ->
                        TripRow(trip, !state.busy) { onOpenTrip(trip.trip.tripId) }
                    }
                }
            }
        }
    }
}

@Composable
private fun Feedback(state: TripViewState, connected: Boolean) {
    if (!connected) Notice(stringResource(R.string.enterprise_trips_send_offline), Modifier.testTag("enterprise-trip-offline"))
    if (state.busy) CircularProgressIndicator()
    val refusal = state.refusal
    val message = when {
        state.saveFailed -> R.string.enterprise_trips_save_failed
        state.captureInvalid -> R.string.enterprise_trips_capture_invalid
        state.driverUnknown -> R.string.enterprise_trips_driver_unknown
        state.sendFailed -> R.string.enterprise_trips_send_failed
        refusal != null -> refusalText(refusal)
        state.lastSaved != null -> R.string.enterprise_trips_saved
        else -> null
    }
    message?.let { Notice(stringResource(it), Modifier.testTag(if (it == R.string.enterprise_trips_saved) "enterprise-trip-saved" else "enterprise-trip-feedback")) }
}

private fun refusalText(failure: TripLocalFailure): Int = when (failure) {
    TripLocalFailure.ASSET_HAS_UNFINISHED_TRIP -> R.string.enterprise_trips_refusal_asset
    TripLocalFailure.UNKNOWN_TRIP -> R.string.enterprise_trips_refusal_unknown
    TripLocalFailure.TRIP_NOT_OPEN -> R.string.enterprise_trips_refusal_not_open
    TripLocalFailure.NOT_FINISHED -> R.string.enterprise_trips_refusal_not_finished
    TripLocalFailure.ALREADY_SUBMITTED -> R.string.enterprise_trips_refusal_submitted
    TripLocalFailure.BEFORE_START -> R.string.enterprise_trips_refusal_before_start
    TripLocalFailure.DUPLICATE -> R.string.enterprise_trips_save_failed
    TripLocalFailure.FULL -> R.string.enterprise_trips_refusal_full
    TripLocalFailure.FOREIGN -> R.string.enterprise_trips_refusal_foreign
}

private fun stateText(state: TripLocalState): Int = when (state) {
    TripLocalState.IN_PROGRESS -> R.string.enterprise_trips_in_progress
    TripLocalState.FINISHED -> R.string.enterprise_trips_finished_local
    TripLocalState.START_REFUSED -> R.string.enterprise_trips_start_refused_local
}

private fun reviewText(line: TripReviewLine): Int = when (line) {
    TripReviewLine.NOT_READY -> R.string.enterprise_trips_review_not_ready
    TripReviewLine.READY_TO_SEND -> R.string.enterprise_trips_review_ready
    TripReviewLine.AWAITING_APPROVAL -> R.string.enterprise_trips_review_awaiting
    TripReviewLine.SUBMISSION_NEEDS_REVIEW -> R.string.enterprise_trips_review_needs
    TripReviewLine.SUBMISSION_REFUSED -> R.string.enterprise_trips_review_refused
}

private fun eventText(type: TripLogbookType): Int = when (type) {
    TripLogbookType.STOP -> R.string.enterprise_trips_event_stop
    TripLogbookType.ARRIVAL -> R.string.enterprise_trips_event_arrival
    TripLogbookType.DEPARTURE -> R.string.enterprise_trips_event_departure
    else -> R.string.enterprise_trips_event_note
}

private fun label(label: TripAssetLabel): String = listOf(label.name, label.fleetNumber, label.plate).filter { it.isNotBlank() }.joinToString(" · ")

@Composable
private fun TripRow(view: TripView, enabled: Boolean, onOpen: () -> Unit) {
    TripCard(Modifier.testTag("enterprise-trip-row-${view.trip.tripId}")) {
        Text(label(view.trip.label), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(R.string.enterprise_trips_started_at, formatted(view.trip.startedAt)))
        view.trip.finishedAt?.let { Text(stringResource(R.string.enterprise_trips_finished_at, formatted(it))) }
        Text(stringResource(stateText(view.trip.state)), modifier = Modifier.testTag("enterprise-trip-state-${view.trip.tripId}"))
        StatusChip(view.start, "enterprise-trip-row-sync-${view.trip.tripId}")
        Text(stringResource(reviewText(view.review)))
        OutlinedButton(onClick = onOpen, enabled = enabled, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)
            .testTag("enterprise-trip-open-${view.trip.tripId}")) { Text(stringResource(R.string.enterprise_trips_open_trip)) }
    }
}

@Composable
private fun StatusChip(record: TripRecordView, tag: String) {
    val status = stringResource(statusText(record.sync))
    val reason = record.code?.takeIf { record.sync == EnterpriseSyncStatus.NEEDS_REVIEW || record.sync == EnterpriseSyncStatus.REJECTED }
        ?.let { stringResource(reasonText(it)) }
    Column(Modifier.testTag(tag).semantics(mergeDescendants = true) { stateDescription = listOfNotNull(status, reason).joinToString(". ") }) {
        Text(stringResource(R.string.enterprise_trips_record_status, status), style = MaterialTheme.typography.labelLarge)
        reason?.let { Text(stringResource(R.string.enterprise_trips_reason, it)) }
    }
}

/** The open trip as separate list items, so a long timeline scrolls record by record. */
private fun LazyListScope.tripDetail(view: TripView, connected: Boolean, busy: Boolean, onEvent: () -> Unit, onFinish: () -> Unit,
    onReady: () -> Unit, onSend: () -> Unit) {
    item(key = "detail-header") {
        TripCard(Modifier.testTag("enterprise-trip-detail")) {
            Text(label(view.trip.label), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
            if (view.trip.reviewWarningAccepted) Text(stringResource(R.string.enterprise_trips_asset_warning_recorded))
            Text(stringResource(stateText(view.trip.state)), modifier = Modifier.testTag("enterprise-trip-operational"))
            Text(stringResource(reviewText(view.review)), modifier = Modifier.testTag("enterprise-trip-review"))
            if (view.review == TripReviewLine.AWAITING_APPROVAL) Text(stringResource(R.string.enterprise_trips_review_not_received))
            if (view.waitingForStartReview) Notice(stringResource(R.string.enterprise_trips_waiting_start_review), Modifier.testTag("enterprise-trip-waiting"))
            if (view.trip.state == TripLocalState.START_REFUSED) Notice(stringResource(R.string.enterprise_trips_start_refused_notice))
        }
    }
    item(key = "detail-timeline") {
        Text(stringResource(R.string.enterprise_trips_timeline), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
    }
    item(key = "record-start") { Record(view.start, stringResource(R.string.enterprise_trips_record_start), "start") }
    items(view.events, key = { "record-" + it.fact.operationId }) { event ->
        val fact = event.fact as TripOperationFact.Event
        Record(event, listOfNotNull(stringResource(eventText(fact.type)), fact.text).joinToString(": "), "event-${fact.eventId}")
    }
    view.finish?.let { item(key = "record-finish") { Record(it, stringResource(R.string.enterprise_trips_record_finish), "finish") } }
    view.submission?.let { item(key = "record-submission") { Record(it, stringResource(R.string.enterprise_trips_record_submission), "submission") } }
    item(key = "detail-actions") {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            if (view.trip.state == TripLocalState.IN_PROGRESS) {
                Button(onClick = onEvent, enabled = !busy, modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-trip-add-event")) {
                    Text(stringResource(R.string.enterprise_trips_add_event))
                }
                OutlinedButton(onClick = onFinish, enabled = !busy, modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-trip-finish")) {
                    Text(stringResource(R.string.enterprise_trips_finish))
                }
            }
            if (view.trip.state == TripLocalState.FINISHED && view.submission == null) Button(onClick = onReady, enabled = !busy,
                modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-trip-ready")) { Text(stringResource(R.string.enterprise_trips_mark_ready)) }
            OutlinedButton(onClick = onSend, enabled = connected && !busy, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-trip-send")) {
                Text(stringResource(R.string.enterprise_trips_send_now))
            }
        }
    }
}

@Composable
private fun Record(record: TripRecordView, title: String, tag: String) {
    TripCard(Modifier.testTag("enterprise-trip-record-$tag")) {
        Text(title, style = MaterialTheme.typography.titleSmall)
        Text(formatted(record.fact.declared.occurredAt))
        when (val fact = record.fact) {
            is TripOperationFact.Start -> Text(evidenceText(fact.evidence))
            is TripOperationFact.Finish -> Text(evidenceText(fact.evidence))
            else -> Unit
        }
        StatusChip(record, "enterprise-trip-sync-$tag")
    }
}

@Composable
private fun evidenceText(evidence: TripEvidence): String = when (evidence) {
    is TripEvidence.Absent -> stringResource(R.string.enterprise_trips_reading_absent_reason, evidence.reason)
    is TripEvidence.Reading -> if (evidence.value.kind == MeterKind.ODOMETER)
        stringResource(R.string.enterprise_trips_reading_km, BigDecimal.valueOf(evidence.value.units, 3).stripTrailingZeros().toPlainString())
    else stringResource(R.string.enterprise_trips_reading_hours, BigDecimal.valueOf(evidence.value.units).divide(BigDecimal.valueOf(3600), 2,
        java.math.RoundingMode.DOWN).stripTrailingZeros().toPlainString())
}

/** The meter and value, or the stated absence; null while incomplete. */
@Composable
private fun ReadingEntry(meters: List<TripMeterChoice>, title: Int, onChange: (TripCapture?) -> Unit) {
    var selected by remember(meters) { mutableIntStateOf(if (meters.isEmpty()) -1 else 0) }
    var value by remember { mutableStateOf("") }
    var reason by remember { mutableStateOf("") }
    fun publish() = onChange(if (selected < 0) reason.takeIf { it.isNotBlank() }?.let { TripCapture.Absent(it) }
        else value.trim().replace(',', '.').takeIf { it.isNotEmpty() }?.let { raw ->
            val meter = meters[selected]
            TripCapture.Reading(meter, if (meter.kind == MeterKind.ODOMETER) MeterEntryForm.KM else MeterEntryForm.HOURS, raw)
        })
    Text(stringResource(title), style = MaterialTheme.typography.titleSmall)
    if (meters.isEmpty()) Text(stringResource(R.string.enterprise_trips_no_meter))
    meters.forEachIndexed { index, meter ->
        Choice(stringResource(if (meter.kind == MeterKind.ODOMETER) R.string.enterprise_trips_meter_odometer else R.string.enterprise_trips_meter_hours),
            selected == index, "enterprise-trip-meter-$index") { selected = index; publish() }
    }
    Choice(stringResource(R.string.enterprise_trips_reading_absent), selected < 0, "enterprise-trip-meter-absent") { selected = -1; publish() }
    if (selected >= 0) OutlinedTextField(value = value, onValueChange = { value = it; publish() }, label = { Text(stringResource(title)) },
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal), singleLine = true,
        modifier = Modifier.fillMaxWidth().testTag("enterprise-trip-reading-value"))
    else OutlinedTextField(value = reason, onValueChange = { reason = it; publish() }, label = { Text(stringResource(R.string.enterprise_trips_absence_reason)) },
        modifier = Modifier.fillMaxWidth().testTag("enterprise-trip-reading-reason"))
}

@Composable
private fun Choice(text: String, selected: Boolean, tag: String, onSelect: () -> Unit) {
    Row(Modifier.fillMaxWidth().heightIn(min = 48.dp).selectable(selected, onClick = onSelect, role = Role.RadioButton).testTag(tag),
        verticalAlignment = Alignment.CenterVertically) {
        RadioButton(selected = selected, onClick = null)
        Text(text, modifier = Modifier.padding(start = 8.dp))
    }
}

@Composable
private fun StartForm(mode: TripMode.Start, busy: Boolean, onStart: (EnterpriseAssetChoice, TripAssetLabel, TripCapture) -> Unit, onChange: () -> Unit,
    onCancel: () -> Unit) {
    var capture by remember(mode) { mutableStateOf<TripCapture?>(null) }
    TripCard(Modifier.testTag("enterprise-trip-start-form")) {
        Text(stringResource(R.string.enterprise_trips_start_title), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.enterprise_trips_asset_chosen, label(mode.label)), modifier = Modifier.testTag("enterprise-trip-chosen"))
        if (mode.choice.reviewWarningAccepted) Text(stringResource(R.string.enterprise_trips_asset_warning_recorded))
        TextButton(onClick = onChange, enabled = !busy, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.enterprise_trips_change_asset)) }
        ReadingEntry(meterChoices(mode.choice), R.string.enterprise_trips_departure_value) { capture = it }
        Button(onClick = { capture?.let { onStart(mode.choice, mode.label, it) } }, enabled = !busy && capture != null,
            modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-trip-save-start")) { Text(stringResource(R.string.enterprise_trips_save_start)) }
        CancelButton(onCancel)
    }
}

@Composable
private fun EventForm(busy: Boolean, onCancel: () -> Unit, onSave: (TripLogbookType, String?) -> Unit) {
    var type by remember { mutableStateOf(TripLogbookType.STOP) }
    var text by remember { mutableStateOf("") }
    TripCard(Modifier.testTag("enterprise-trip-event-form")) {
        Text(stringResource(R.string.enterprise_trips_add_event), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        for (kind in listOf(TripLogbookType.STOP, TripLogbookType.ARRIVAL, TripLogbookType.DEPARTURE, TripLogbookType.NOTE)) {
            Choice(stringResource(eventText(kind)), type == kind, "enterprise-trip-event-${kind.name}") { type = kind }
        }
        OutlinedTextField(value = text, onValueChange = { text = it },
            label = { Text(stringResource(if (type == TripLogbookType.NOTE) R.string.enterprise_trips_event_text_note else R.string.enterprise_trips_event_text_optional)) },
            modifier = Modifier.fillMaxWidth().testTag("enterprise-trip-event-text"))
        Button(onClick = { onSave(type, text.takeIf { it.isNotBlank() }) }, enabled = !busy && (type != TripLogbookType.NOTE || text.isNotBlank()),
            modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-trip-save-event")) { Text(stringResource(R.string.enterprise_trips_save_event)) }
        CancelButton(onCancel)
    }
}

@Composable
private fun FinishForm(view: TripView, assets: MyAssetsViewState, busy: Boolean, warnings: (String, TripCapture) -> List<ReadingFlag>,
    onCancel: () -> Unit, onSave: (TripCapture) -> Unit) {
    // The departure meter first, then the asset's cached in-service meters (FLEET-013), once each.
    val meters = remember(view, assets.snapshot) {
        val departure = ((view.start.fact as TripOperationFact.Start).evidence as? TripEvidence.Reading)?.let {
            TripMeterChoice(it.meterId, it.segmentId, it.value.kind, null)
        }
        val cached = assets.snapshot?.assets?.firstOrNull { it.asset.id == view.trip.assetId }?.asset?.meters.orEmpty()
            .map { TripMeterChoice(it.id, it.segmentId, MeterKind.parse(it.kind.name), it.modulus) }
        (listOfNotNull(departure) + cached).distinctBy { it.meterId to it.segmentId }
    }
    var capture by remember(view.trip.tripId) { mutableStateOf<TripCapture?>(null) }
    TripCard(Modifier.testTag("enterprise-trip-finish-form")) {
        Text(stringResource(R.string.enterprise_trips_finish), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        ReadingEntry(meters, R.string.enterprise_trips_final_value) { capture = it }
        val flags = capture?.let { warnings(view.trip.tripId, it) }.orEmpty()
        if (ReadingFlag.READING_BELOW_PREVIOUS in flags) Notice(stringResource(R.string.enterprise_trips_warning_below), Modifier.testTag("enterprise-trip-warning"))
        else if (ReadingFlag.IMPLAUSIBLE_RATE in flags) Notice(stringResource(R.string.enterprise_trips_warning_rate), Modifier.testTag("enterprise-trip-warning"))
        Button(onClick = { capture?.let(onSave) }, enabled = !busy && capture != null,
            modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-trip-save-finish")) { Text(stringResource(R.string.enterprise_trips_save_finish)) }
        CancelButton(onCancel)
    }
}

@Composable
private fun CancelButton(onCancel: () -> Unit) {
    TextButton(onClick = onCancel, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-trip-cancel")) {
        Text(stringResource(R.string.enterprise_trips_cancel))
    }
}

/** Home entry card; it opens the protected trip book, never a server read by itself. */
@Composable
internal fun TripsHomeCard(onOpen: () -> Unit, modifier: Modifier = Modifier) {
    TripCard(modifier.testTag("enterprise-trips-home")) {
        Text(stringResource(R.string.enterprise_trips_title), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.enterprise_trips_home_copy))
        Button(onClick = onOpen, modifier = Modifier.fillMaxWidth().testTag("enterprise-trips-open")) { Text(stringResource(R.string.enterprise_trips_open)) }
    }
}

@Composable
private fun TripCard(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
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
