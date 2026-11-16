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
import com.anpfuel.app.ui.enterprise.attachment.AttachmentRows
import com.anpfuel.app.ui.enterprise.attachment.ReceiptCaptureCard
import com.anpfuel.app.ui.enterprise.attachment.ReceiptCaptureUi
import com.anpfuel.app.ui.enterprise.attachment.localFailureText
import com.anpfuel.app.ui.enterprise.checklist.DepartureChecklistLine
import com.anpfuel.app.ui.enterprise.checklist.departureBlocks
import com.anpfuel.app.ui.enterprise.fleet.EnterpriseAssetPicker
import com.anpfuel.app.ui.enterprise.reasonText
import com.anpfuel.app.ui.enterprise.statusText
import com.anpfuel.app.ui.enterprise.tracking.TrackingCard
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.enterprise.attachment.ReceiptDraft
import com.anpfuel.application.enterprise.checklist.ChecklistViewState
import com.anpfuel.application.enterprise.fleet.MyAssetsViewState
import com.anpfuel.application.enterprise.trip.TripCapture
import com.anpfuel.application.enterprise.trip.TripMeterChoice
import com.anpfuel.application.enterprise.trip.TripViewState
import com.anpfuel.domain.enterprise.attachment.AttachmentTarget
import com.anpfuel.domain.enterprise.attachment.AttachmentView
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.fleet.AssetKind
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import com.anpfuel.domain.enterprise.meter.MeterEntryForm
import com.anpfuel.domain.enterprise.meter.MeterKind
import com.anpfuel.domain.enterprise.meter.ReadingFlag
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.trip.MissionView
import com.anpfuel.domain.enterprise.trip.ClosingEvidenceState
import com.anpfuel.domain.enterprise.trip.ClosingItem
import com.anpfuel.domain.enterprise.trip.ClosingItemKind
import com.anpfuel.domain.enterprise.trip.ClosingRecordState
import com.anpfuel.domain.enterprise.trip.TripClosing
import com.anpfuel.domain.enterprise.tracking.TrackingDevice
import com.anpfuel.domain.enterprise.trip.TripAssetLabel
import com.anpfuel.domain.enterprise.trip.TripCommand
import com.anpfuel.domain.enterprise.trip.TripMissionResponse
import com.anpfuel.domain.enterprise.trip.TripNavigation
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
    /** [missionId] and [assetId] narrow the picker to a planned trip's vehicle (EMP-TRIP-021, B-BR-EMP-529). */
    class Picker(val missionId: String? = null, val assetId: String? = null) : TripMode
    class Start(val choice: EnterpriseAssetChoice, val label: TripAssetLabel, val missionId: String? = null) : TripMode
    /** EMP-TRIP-021: one mission of the inbox, its decline reason and a trip's lifecycle reason. */
    class Mission(val missionId: String) : TripMode
    class Decline(val missionId: String) : TripMode
    class Lifecycle(val tripId: String, val command: TripCommand) : TripMode
    class Event(val tripId: String) : TripMode
    class Finish(val tripId: String) : TripMode
    /** EMP-DOC-004: a photo of the reading carried by one record (owner operation and its METER_READING target). */
    class Photo(val ownerOperationId: String, val target: AttachmentTarget) : TripMode
    /** EMP-CHECK-002: the chosen vehicle's departure checklist, returning to the same start form. */
    class Checklist(val start: Start) : TripMode
}

/** EMP-GPS-002 (B-BR-EMP-496): the tracking gestures of the open trip; null hides the card (rollback flag). */
internal class TripTrackingActions(val onStart: (tripId: String, TrackingDevice) -> Unit, val onStop: (tripId: String) -> Unit,
    val onRefresh: () -> Unit,
    /** EMP-GPS-015 (B-BR-EMP-548): the session this phone collects now; null when the screen does not know the collector. */
    val collecting: (() -> String?)? = null)

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
    /** EMP-DOC-004 capture; null when it is switched off (B-BR-EMP-450). */
    capture: ReceiptCaptureUi? = null,
    onAttach: (String, AttachmentTarget, ReceiptDraft) -> Unit = { _, _, _ -> },
    /** EMP-CHECK-002 (B-BR-EMP-443): the departure checklist owner's state; null hides the departure line (rollback flag). */
    checklist: ChecklistViewState? = null,
    onDepartureCheck: (assetId: String) -> Unit = {},
    departureContent: (@Composable (assetId: String, label: String, onDone: () -> Unit) -> Unit)? = null,
    tracking: TripTrackingActions? = null,
    /** EMP-TRIP-021: the missions inbox and the pause/resume/cancel actions; false hides them (`ENTERPRISE_MISSION_UI_ENABLED`). */
    missionsEnabled: Boolean = false,
    onRefreshMissions: () -> Unit = {},
    onRespond: (missionId: String, TripMissionResponse, reason: String?) -> Unit = { _, _, _ -> },
    onStartMission: (missionId: String, EnterpriseAssetChoice, TripAssetLabel, TripCapture) -> Unit = { _, _, _, _ -> },
    onLifecycle: (tripId: String, TripCommand, reason: String?) -> Unit = { _, _, _ -> },
    /** Extra mission actions owned by other tasks; none by default. */
    missionExtra: @Composable (MissionView) -> Unit = {},
    tripExtra: @Composable (TripView) -> Unit = {},
    /** EMP-ROUTE-004: hands an approved destination to the app's own route preview; null offers no navigation. */
    onNavigate: ((String) -> Unit)? = null,
) {
    var mode by remember { mutableStateOf<TripMode>(TripMode.Home) }
    // A durable save closes the form it came from; a refusal or failure keeps the entered values.
    LaunchedEffect(state.lastSaved) {
        // An answer returns to its mission, where its own state is shown; any other save returns to the list.
        if (state.lastSaved != null && mode !is TripMode.Picker) mode = when (val saved = mode) {
            is TripMode.Decline -> TripMode.Mission(saved.missionId)
            is TripMode.Mission -> saved
            else -> TripMode.Home
        }
    }
    // A sealed photo closes its sheet and drops the unsaved copy; a refusal or failure keeps it for another attempt.
    LaunchedEffect(state.lastAttached) { if (state.lastAttached != null && mode is TripMode.Photo) { capture?.reset?.invoke(); mode = TripMode.Home } }
    LaunchedEffect(state.context?.partition, state.epoch) { capture?.reset?.invoke(); mode = TripMode.Home }
    val photo: (String, AttachmentTarget) -> Unit = { owner, target -> capture?.reset?.invoke(); mode = TripMode.Photo(owner, target) }
    when (val current = mode) {
        is TripMode.Picker -> EnterpriseAssetPicker(
            state = assets, connected = connected,
            onChoose = { id, warning ->
                val choice = onChoose(id, warning)
                val asset = assets.snapshot?.assets?.firstOrNull { it.asset.id == id }?.asset
                if (choice != null && asset != null) mode = TripMode.Start(choice, TripAssetLabel(asset.name, asset.fleetNumber, asset.plate), current.missionId)
            },
            onCancel = { mode = current.missionId?.let { TripMode.Mission(it) } ?: TripMode.Home },
            modifier = modifier.testTag("enterprise-trip-picker"),
            accepts = { it.kind != AssetKind.IMPLEMENT && (current.assetId == null || it.id == current.assetId) },
        )
        is TripMode.Checklist -> if (departureContent != null) departureContent(current.start.choice.assetId, label(current.start.label)) { mode = current.start }
            else LaunchedEffect(current) { mode = current.start }
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
                is TripMode.Start -> item { StartForm(current, state.busy, { choice, label, capture ->
                        current.missionId?.let { onStartMission(it, choice, label, capture) } ?: onStart(choice, label, capture) },
                    onChange = { onPick(); mode = TripMode.Picker(current.missionId, state.missions?.missions?.firstOrNull { it.missionId == current.missionId }?.item?.assetId) },
                    onCancel = { mode = TripMode.Home }, checklist = checklist.takeIf { departureContent != null },
                    onDepartureCheck = onDepartureCheck, onDepartureOpen = { mode = TripMode.Checklist(current) }) }
                is TripMode.Event -> item { EventForm(state.busy, onCancel = { mode = TripMode.Home }) { type, text -> onLog(current.tripId, type, text) } }
                is TripMode.Finish -> state.trips.firstOrNull { it.trip.tripId == current.tripId }?.let { trip ->
                    item { FinishForm(trip, assets, state.busy, finishWarnings, onCancel = { mode = TripMode.Home }) { onFinish(current.tripId, it) } }
                }
                is TripMode.Mission -> state.missions?.missions?.firstOrNull { it.missionId == current.missionId }?.let { mission ->
                    item(key = "mission-detail") {
                        MissionDetail(mission, assets, state.busy,
                            onAccept = { onRespond(mission.missionId, TripMissionResponse.ACCEPT, null) },
                            onDecline = { mode = TripMode.Decline(mission.missionId) },
                            onStart = { onPick(); mode = TripMode.Picker(mission.missionId, mission.item?.assetId) },
                            onOpenTrip = { mode = TripMode.Home; onOpenTrip(it) },
                            extra = {
                                missionExtra(mission)
                                // EMP-ROUTE-004: only this mission's approved text destination, to the app's own route preview.
                                onNavigate?.let { TripNavigationAction(TripNavigation.forMission(state.missions, mission.missionId), state.busy, it) }
                            })
                    }
                }
                is TripMode.Decline -> item {
                    ReasonForm(R.string.enterprise_missions_decline, R.string.enterprise_missions_decline_reason, R.string.enterprise_missions_save_decline,
                        required = true, busy = state.busy, tag = "enterprise-mission-decline-form", onCancel = { mode = TripMode.Mission(current.missionId) }) {
                        onRespond(current.missionId, TripMissionResponse.DECLINE, it)
                    }
                }
                is TripMode.Lifecycle -> item {
                    val (title, label, save) = lifecycleForm(current.command)
                    ReasonForm(title, label, save, required = current.command == TripCommand.CANCEL, busy = state.busy,
                        tag = "enterprise-trip-lifecycle-form", onCancel = { mode = TripMode.Home }) { onLifecycle(current.tripId, current.command, it) }
                }
                is TripMode.Photo -> if (capture != null) item {
                    ReceiptCaptureCard(stringResource(R.string.enterprise_receipts_title_reading), capture, state.busy,
                        onSave = { onAttach(current.ownerOperationId, current.target, it) },
                        onCancel = { capture.reset(); mode = TripMode.Home })
                }
                else -> if (open != null) {
                    tripDetail(open, connected, state.busy, onEvent = { mode = TripMode.Event(open.trip.tripId) },
                        onFinish = { mode = TripMode.Finish(open.trip.tripId) }, onReady = { onReady(open.trip.tripId) }, onSend = onSend,
                        photos = state.attachments, onPhoto = photo.takeIf { capture != null }, closing = state.closing(open.trip.tripId),
                        state = state, tracking = tracking,
                        onLifecycle = if (missionsEnabled) { command -> mode = TripMode.Lifecycle(open.trip.tripId, command) } else null,
                        missionTitle = open.missionId?.let { id -> state.missions?.missions?.firstOrNull { it.missionId == id }?.item?.title },
                        extra = {
                            tripExtra(open)
                            // EMP-ROUTE-004: the destination of the mission this trip's own start names, never another trip's.
                            onNavigate?.let { TripNavigationAction(TripNavigation.forTrip(state.trips, state.missions, open.trip.tripId), state.busy, it) }
                        })
                } else {
                    if (missionsEnabled) missionsSection(state.missions, state.missionsUnavailable, connected, state.busy, state.refreshed,
                        state.refreshFailed, onRefresh = onRefreshMissions, onOpen = { mode = TripMode.Mission(it) })
                    item {
                        Button(onClick = { onPick(); mode = TripMode.Picker() }, enabled = !state.busy,
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
    val photoRefusal = state.attachmentRefusal
    val missionRefusal = state.missionRefusal
    val message = when {
        state.saveFailed -> R.string.enterprise_trips_save_failed
        state.captureInvalid -> R.string.enterprise_trips_capture_invalid
        state.driverUnknown -> R.string.enterprise_trips_driver_unknown
        state.sendFailed -> R.string.enterprise_trips_send_failed
        refusal != null -> refusalText(refusal)
        missionRefusal != null -> missionRefusalText(missionRefusal)
        photoRefusal != null -> localFailureText(photoRefusal)
        state.lastAttached != null -> R.string.enterprise_receipts_saved
        state.lastSaved != null -> R.string.enterprise_trips_saved
        else -> null
    }
    message?.let { Notice(stringResource(it), Modifier.testTag(when (it) {
        R.string.enterprise_trips_saved -> "enterprise-trip-saved"
        R.string.enterprise_receipts_saved -> "enterprise-trip-photo-saved"
        else -> "enterprise-trip-feedback"
    })) }
    if (state.attachmentsUnavailable) Notice(stringResource(R.string.enterprise_receipts_unavailable), Modifier.testTag("enterprise-trip-photos-unavailable"))
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
    TripLocalFailure.NOT_RUNNING -> R.string.enterprise_missions_refusal_not_running
    TripLocalFailure.NOT_PAUSED -> R.string.enterprise_missions_refusal_not_paused
    TripLocalFailure.MISSION_ALREADY_STARTED -> R.string.enterprise_missions_start_already
}

private fun stateText(state: TripLocalState): Int = when (state) {
    TripLocalState.IN_PROGRESS -> R.string.enterprise_trips_in_progress
    TripLocalState.PAUSED -> R.string.enterprise_missions_paused
    TripLocalState.CANCELLED -> R.string.enterprise_missions_cancelled_local
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
    onReady: () -> Unit, onSend: () -> Unit, state: TripViewState, tracking: TripTrackingActions?,
    photos: Map<String, List<AttachmentView>> = emptyMap(), onPhoto: ((String, AttachmentTarget) -> Unit)? = null,
    closing: TripClosing? = null, onLifecycle: ((TripCommand) -> Unit)? = null, missionTitle: String? = null,
    extra: @Composable () -> Unit = {}) {
    item(key = "detail-header") {
        TripCard(Modifier.testTag("enterprise-trip-detail")) {
            Text(label(view.trip.label), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
            if (view.trip.reviewWarningAccepted) Text(stringResource(R.string.enterprise_trips_asset_warning_recorded))
            Text(stringResource(stateText(view.trip.state)), modifier = Modifier.testTag("enterprise-trip-operational"))
            Text(stringResource(reviewText(view.review)), modifier = Modifier.testTag("enterprise-trip-review"))
            if (view.review == TripReviewLine.AWAITING_APPROVAL) Text(stringResource(R.string.enterprise_trips_review_not_received))
            if (view.waitingForStartReview) Notice(stringResource(R.string.enterprise_trips_waiting_start_review), Modifier.testTag("enterprise-trip-waiting"))
            if (view.trip.state == TripLocalState.START_REFUSED) Notice(stringResource(R.string.enterprise_trips_start_refused_notice))
            missionTitle?.let { Text(stringResource(R.string.enterprise_missions_trip_mission, it), modifier = Modifier.testTag("enterprise-trip-mission")) }
        }
    }
    // EMP-GPS-002: shown for an organization trip in progress or one that already has a session; never for personal trips.
    if (tracking != null && (view.trip.state == TripLocalState.IN_PROGRESS || state.tracking?.session(view.trip.tripId) != null)) item(key = "detail-tracking") {
        val tripId = view.trip.tripId
        TrackingCard(tripId, view.trip.state == TripLocalState.IN_PROGRESS, state.tracking, state.trackingRefusal, state.trackingPolicyFailed,
            connected, busy, { formatted(it) }, onStart = { tracking.onStart(tripId, it) }, onStop = { tracking.onStop(tripId) },
            onRefresh = tracking.onRefresh, collecting = tracking.collecting)
    }
    item(key = "detail-timeline") {
        Text(stringResource(R.string.enterprise_trips_timeline), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
    }
    item(key = "record-start") { Record(view.start, stringResource(R.string.enterprise_trips_record_start), "start", busy, photos, onPhoto) }
    // Events and the EMP-TRIP-011 lifecycle commands in occurrence order, each with its own sync state.
    val timeline = (view.events + view.lifecycle).sortedWith(compareBy({ it.fact.declared.occurredAt }, { it.fact.operationId }))
    items(timeline, key = { "record-" + it.fact.operationId }) { record ->
        when (val fact = record.fact) {
            is TripOperationFact.Event -> Record(record, listOfNotNull(stringResource(eventText(fact.type)), fact.text).joinToString(": "), "event-${fact.eventId}")
            is TripOperationFact.Lifecycle -> Record(record, listOfNotNull(stringResource(when (fact.command) {
                TripCommand.PAUSE -> R.string.enterprise_missions_record_pause
                TripCommand.RESUME -> R.string.enterprise_missions_record_resume
                else -> R.string.enterprise_missions_record_cancel
            }), fact.reason).joinToString(": "), "lifecycle-${fact.operationId}")
            else -> Unit
        }
    }
    view.finish?.let { item(key = "record-finish") { Record(it, stringResource(R.string.enterprise_trips_record_finish), "finish", busy, photos, onPhoto) } }
    view.submission?.let { item(key = "record-submission") { Record(it, stringResource(R.string.enterprise_trips_record_submission), "submission") } }
    // EMP-TRIP-020: the closing conference of a finished trip, before and after it is marked ready (B-BR-EMP-522..526).
    if (closing != null && view.trip.state == TripLocalState.FINISHED) item(key = "detail-closing") { ClosingConference(closing) }
    item(key = "detail-actions") {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            if (view.trip.open) {
                Button(onClick = onEvent, enabled = !busy, modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-trip-add-event")) {
                    Text(stringResource(R.string.enterprise_trips_add_event))
                }
                if (onLifecycle != null) {
                    val command = if (view.trip.state == TripLocalState.PAUSED) TripCommand.RESUME else TripCommand.PAUSE
                    OutlinedButton(onClick = { onLifecycle(command) }, enabled = !busy, modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp)
                        .testTag(if (command == TripCommand.PAUSE) "enterprise-trip-pause" else "enterprise-trip-resume")) {
                        Text(stringResource(if (command == TripCommand.PAUSE) R.string.enterprise_missions_pause else R.string.enterprise_missions_resume))
                    }
                }
                OutlinedButton(onClick = onFinish, enabled = !busy, modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-trip-finish")) {
                    Text(stringResource(R.string.enterprise_trips_finish))
                }
                if (onLifecycle != null) OutlinedButton(onClick = { onLifecycle(TripCommand.CANCEL) }, enabled = !busy,
                    modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-trip-cancel-trip")) {
                    Text(stringResource(R.string.enterprise_missions_cancel_trip))
                }
            }
            extra()
            if (view.trip.state == TripLocalState.FINISHED && view.submission == null) Button(onClick = onReady, enabled = !busy,
                modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-trip-ready")) { Text(stringResource(R.string.enterprise_trips_mark_ready)) }
            OutlinedButton(onClick = onSend, enabled = connected && !busy, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-trip-send")) {
                Text(stringResource(R.string.enterprise_trips_send_now))
            }
        }
    }
}

@Composable
private fun Record(record: TripRecordView, title: String, tag: String, busy: Boolean = false, photos: Map<String, List<AttachmentView>> = emptyMap(),
    onPhoto: ((String, AttachmentTarget) -> Unit)? = null) {
    TripCard(Modifier.testTag("enterprise-trip-record-$tag")) {
        Text(title, style = MaterialTheme.typography.titleSmall)
        Text(formatted(record.fact.declared.occurredAt))
        val evidence = when (val fact = record.fact) {
            is TripOperationFact.Start -> fact.evidence
            is TripOperationFact.Finish -> fact.evidence
            else -> null
        }
        evidence?.let { Text(evidenceText(it)) }
        StatusChip(record, "enterprise-trip-sync-$tag")
        // EMP-DOC-004: a recorded reading may carry photos, each with its own state (B-BR-EMP-454).
        if (evidence is TripEvidence.Reading) {
            val owner = record.fact.operationId
            val target = AttachmentTarget(DocumentTargetKind.METER_READING, evidence.readingId)
            AttachmentRows(photos[owner].orEmpty(), "enterprise-trip-photo-$tag", onPhoto?.let { { it(owner, target) } })
            if (onPhoto != null) OutlinedButton(onClick = { onPhoto(owner, target) }, enabled = !busy,
                modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-trip-add-photo-$tag")) {
                Text(stringResource(R.string.enterprise_receipts_add_reading_photo))
            }
        }
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
    onCancel: () -> Unit, checklist: ChecklistViewState? = null, onDepartureCheck: (String) -> Unit = {}, onDepartureOpen: () -> Unit = {}) {
    var capture by remember(mode) { mutableStateOf<TripCapture?>(null) }
    // EMP-CHECK-002: the chosen vehicle's latest valid departure run; a BLOCKING one keeps the start disabled here.
    if (checklist != null) LaunchedEffect(mode.choice.assetId) { onDepartureCheck(mode.choice.assetId) }
    val blocked = departureBlocks(checklist, mode.choice.assetId)
    TripCard(Modifier.testTag("enterprise-trip-start-form")) {
        Text(stringResource(R.string.enterprise_trips_start_title), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.enterprise_trips_asset_chosen, label(mode.label)), modifier = Modifier.testTag("enterprise-trip-chosen"))
        if (mode.choice.reviewWarningAccepted) Text(stringResource(R.string.enterprise_trips_asset_warning_recorded))
        TextButton(onClick = onChange, enabled = !busy, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.enterprise_trips_change_asset)) }
        if (checklist != null) DepartureChecklistLine(checklist, mode.choice.assetId, onDepartureOpen)
        ReadingEntry(meterChoices(mode.choice), R.string.enterprise_trips_departure_value) { capture = it }
        Button(onClick = { capture?.takeIf { !blocked }?.let { onStart(mode.choice, mode.label, it) } }, enabled = !busy && capture != null && !blocked,
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

private fun closingRecordText(state: ClosingRecordState): Int = when (state) {
    ClosingRecordState.SAVED_ON_DEVICE -> R.string.enterprise_trip_closing_record_saved
    ClosingRecordState.SENDING -> R.string.enterprise_trip_closing_record_sending
    ClosingRecordState.ACCEPTED -> R.string.enterprise_trip_closing_record_accepted
    ClosingRecordState.NEEDS_REVIEW -> R.string.enterprise_trip_closing_record_needs_review
    ClosingRecordState.REFUSED -> R.string.enterprise_trip_closing_record_refused
}

private fun closingEvidenceText(state: ClosingEvidenceState): Int = when (state) {
    ClosingEvidenceState.MISSING -> R.string.enterprise_trip_closing_evidence_missing
    ClosingEvidenceState.PENDING -> R.string.enterprise_trip_closing_evidence_pending
    ClosingEvidenceState.AVAILABLE -> R.string.enterprise_trip_closing_evidence_available
    ClosingEvidenceState.UNREADABLE -> R.string.enterprise_trip_closing_evidence_unreadable
}

/** Exact centavos as reais with a decimal comma, never rounded and never added to another item. */
private fun closingReais(centavos: Long): String = java.math.BigDecimal.valueOf(centavos, 2).toPlainString().replace('.', ',')

/** SCR-TRIP-01 closing conference (EMP-TRIP-020): each item with its own record and evidence state; no total, no decision. */
@Composable
private fun ClosingConference(closing: TripClosing) {
    TripCard(Modifier.testTag("enterprise-trip-closing")) {
        Text(stringResource(R.string.enterprise_trip_closing_title), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.enterprise_trip_closing_intro), style = MaterialTheme.typography.bodySmall)
        if (closing.relatedUnavailable) Notice(stringResource(R.string.enterprise_trip_closing_related_unavailable), Modifier.testTag("enterprise-trip-closing-unavailable"))
        if (!closing.relatedUnavailable && closing.items.none { it.kind == ClosingItemKind.FUELING || it.kind == ClosingItemKind.EXPENSE })
            Text(stringResource(R.string.enterprise_trip_closing_none), modifier = Modifier.testTag("enterprise-trip-closing-none"))
        closing.items.forEach { ClosingRow(it) }
        Text(stringResource(R.string.enterprise_trip_closing_no_decision), style = MaterialTheme.typography.bodySmall)
    }
}

@Composable
private fun ClosingRow(item: ClosingItem) {
    val title = when (item.kind) {
        ClosingItemKind.DEPARTURE_READING -> stringResource(R.string.enterprise_trip_closing_departure)
        ClosingItemKind.FINAL_READING -> stringResource(R.string.enterprise_trip_closing_final)
        ClosingItemKind.FUELING -> stringResource(R.string.enterprise_trip_closing_fueling, closingReais(item.paidCentavos ?: 0))
        ClosingItemKind.EXPENSE -> stringResource(R.string.enterprise_trip_closing_expense, closingReais(item.paidCentavos ?: 0))
    }
    val record = stringResource(closingRecordText(item.record))
    val evidence = stringResource(closingEvidenceText(item.evidence))
    val reason = item.code?.takeIf { item.record == ClosingRecordState.NEEDS_REVIEW || item.record == ClosingRecordState.REFUSED }
        ?.let { stringResource(reasonText(it)) }
    Column(Modifier.fillMaxWidth().testTag("enterprise-trip-closing-${item.kind.name.lowercase()}-${item.recordId}")
        .semantics(mergeDescendants = true) { stateDescription = listOfNotNull(record, reason, evidence).joinToString(". ") }) {
        Text(title, style = MaterialTheme.typography.labelLarge)
        Text(formatted(item.occurredAt), style = MaterialTheme.typography.bodySmall)
        Text(record)
        reason?.let { Text(stringResource(R.string.enterprise_trips_reason, it)) }
        Text(evidence)
        if (item.photos > 0) Text(stringResource(R.string.enterprise_trip_closing_photos, item.photos), style = MaterialTheme.typography.bodySmall)
    }
}

@Composable
internal fun TripCard(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    val fill = MaterialTheme.colorScheme.surfaceContainerLow
    Card(modifier.fillMaxWidth(), shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = fill), border = AnpContainerDefaults.borderStroke(fill)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) { content() }
    }
}

@Composable
internal fun Notice(message: String, modifier: Modifier = Modifier) {
    Text(message, modifier = modifier.semantics { liveRegion = LiveRegionMode.Polite })
}

@Composable
internal fun formatted(instant: Instant): String {
    val locale = LocalConfiguration.current.locales[0]
    val zone = remember { ZoneId.systemDefault() }
    val formatter = remember(locale, zone) { DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM).withLocale(locale).withZone(zone) }
    return formatter.format(instant)
}
