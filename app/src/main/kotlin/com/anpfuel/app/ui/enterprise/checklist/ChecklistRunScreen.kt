package com.anpfuel.app.ui.enterprise.checklist

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Block
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.enterprise.fleet.EnterpriseAssetPicker
import com.anpfuel.app.ui.enterprise.reasonText
import com.anpfuel.app.ui.enterprise.statusText
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.enterprise.checklist.ChecklistViewState
import com.anpfuel.application.enterprise.fleet.MyAssetsViewState
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistAnswer
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistContext
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistDefinition
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistLocalFailure
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistOutcome
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistPresentation
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistRunView
import com.anpfuel.domain.enterprise.maintenance.checklist.PolicyMode
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/*
 * The checklist execution screens (EMP-CHECK-002, B-BR-EMP-443): one item per row with large targets, critical marks,
 * the three answers and the note, progress and the cached template's confirmation age; the local outcome with the
 * items that block, and after delivery the company's result with "A empresa recalculou o resultado" when it differs.
 * Everything shown comes from the protected book; no identifier, code or protocol word is shown.
 */

/** The gestures of the run screen; the caller binds them to the rendered epoch and generation. */
internal class ChecklistActions(
    val onRefresh: () -> Unit,
    val onRetry: () -> Unit,
    val onStart: () -> Unit,
    val onAnswer: (runId: String, position: Int, answer: String, note: String?) -> Unit,
    val onComplete: (runId: String, acknowledged: Boolean) -> Unit,
    val onSend: () -> Unit,
)

internal fun outcomeText(outcome: ChecklistOutcome): Int = when (outcome) {
    ChecklistOutcome.COMPLETED_OK -> R.string.enterprise_checklists_outcome_ok
    ChecklistOutcome.COMPLETED_WITH_ISSUES -> R.string.enterprise_checklists_outcome_issues
    ChecklistOutcome.BLOCKING -> R.string.enterprise_checklists_outcome_blocking
}

private fun contextText(context: ChecklistContext): Int = when (context) {
    ChecklistContext.DEPARTURE -> R.string.enterprise_checklists_context_departure
    ChecklistContext.RETURN -> R.string.enterprise_checklists_context_return
    ChecklistContext.AD_HOC -> R.string.enterprise_checklists_context_ad_hoc
}

private fun refusalText(failure: ChecklistLocalFailure): Int = when (failure) {
    ChecklistLocalFailure.INCOMPLETE -> R.string.enterprise_checklists_refusal_incomplete
    ChecklistLocalFailure.ANSWER_NOT_ALLOWED -> R.string.enterprise_checklists_refusal_not_allowed
    ChecklistLocalFailure.PHOTO_REQUIRED -> R.string.enterprise_checklists_refusal_photo
    ChecklistLocalFailure.ACKNOWLEDGEMENT_REQUIRED -> R.string.enterprise_checklists_refusal_ack
    ChecklistLocalFailure.RUN_IN_PROGRESS -> R.string.enterprise_checklists_refusal_in_progress
    ChecklistLocalFailure.NO_CHECKLIST -> R.string.enterprise_checklists_refusal_no_checklist
    ChecklistLocalFailure.CHECKLIST_EXPIRED -> R.string.enterprise_checklists_expired
    ChecklistLocalFailure.BEFORE_START -> R.string.enterprise_checklists_refusal_before_start
    ChecklistLocalFailure.INVALID_ANSWER -> R.string.enterprise_checklists_refusal_invalid
    ChecklistLocalFailure.FULL -> R.string.enterprise_checklists_refusal_full
    ChecklistLocalFailure.DUPLICATE -> R.string.enterprise_checklists_save_failed
    ChecklistLocalFailure.UNKNOWN_RUN, ChecklistLocalFailure.RUN_NOT_OPEN, ChecklistLocalFailure.FOREIGN -> R.string.enterprise_checklists_refusal_other
}

/** The run screen of one asset and moment, from the cached template to the company's result. */
@Composable
internal fun ChecklistRunPanel(
    state: ChecklistViewState,
    connected: Boolean,
    vehicle: String,
    actions: ChecklistActions,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    LazyColumn(modifier.testTag("enterprise-checklist-panel").padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        item(key = "header") {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                val moment = state.checklistContext ?: ChecklistContext.DEPARTURE
                Text(if (moment == ChecklistContext.DEPARTURE) stringResource(R.string.enterprise_checklists_departure_title)
                    else stringResource(R.string.enterprise_checklists_title) + " · " + stringResource(contextText(moment)),
                    style = MaterialTheme.typography.headlineSmall, modifier = Modifier.semantics { heading() })
                if (vehicle.isNotBlank()) Text(stringResource(R.string.enterprise_checklists_vehicle, vehicle), modifier = Modifier.testTag("enterprise-checklist-vehicle"))
                TextButton(onClick = onBack, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-checklist-back")) {
                    Text(stringResource(R.string.enterprise_checklists_back))
                }
                Feedback(state, connected)
            }
        }
        when {
            state.error != null || state.unlockRequired || state.unrecoverable -> item(key = "error") {
                ChecklistCard(Modifier.testTag("enterprise-checklist-error")) {
                    Notice(stringResource(R.string.enterprise_checklists_unavailable))
                    Button(onClick = actions.onRetry, enabled = !state.busy, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
                        Text(stringResource(R.string.enterprise_checklists_retry))
                    }
                }
            }
            !state.loaded -> item(key = "loading") {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    CircularProgressIndicator(Modifier.testTag("enterprise-checklist-loading"))
                    Notice(stringResource(R.string.enterprise_checklists_loading))
                }
            }
            else -> runContent(state, connected, actions)
        }
    }
}

private fun LazyListScope.runContent(state: ChecklistViewState, connected: Boolean, actions: ChecklistActions) {
    val applicable = state.applicable
    val open = state.open
    item(key = "template") {
        ChecklistCard(Modifier.testTag("enterprise-checklist-template")) {
            when {
                applicable == null -> Notice(stringResource(R.string.enterprise_checklists_not_downloaded), Modifier.testTag("enterprise-checklist-not-downloaded"))
                !applicable.required -> Text(stringResource(R.string.enterprise_checklists_none_required), style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.testTag("enterprise-checklist-none"))
                else -> {
                    Text(applicable.template!!.name, style = MaterialTheme.typography.titleMedium)
                    Text(stringResource(R.string.enterprise_checklists_confirmed_at, formatted(applicable.evaluatedAt)),
                        modifier = Modifier.testTag("enterprise-checklist-confirmed"))
                }
            }
            OutlinedButton(onClick = actions.onRefresh, enabled = connected && !state.busy,
                modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-checklist-refresh")) {
                Text(stringResource(R.string.enterprise_checklists_refresh))
            }
        }
    }
    if (open != null) {
        openRun(open, state.busy, actions)
    } else {
        state.latest?.let { latest -> item(key = "outcome-" + latest.run.runId) { Outcome(latest, connected, state.busy, actions.onSend) } }
        if (applicable?.required == true) item(key = "start") {
            Button(onClick = actions.onStart, enabled = !state.busy, modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-checklist-start")) {
                Text(stringResource(R.string.enterprise_checklists_start))
            }
        }
    }
}

private fun LazyListScope.openRun(view: ChecklistRunView, busy: Boolean, actions: ChecklistActions) {
    val run = view.run
    item(key = "progress-" + run.runId) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(stringResource(R.string.enterprise_checklists_progress, run.answered, run.answers.size), style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.testTag("enterprise-checklist-progress").semantics { liveRegion = LiveRegionMode.Polite })
            if (view.expired) ChecklistCard(Modifier.testTag("enterprise-checklist-expired")) {
                Notice(stringResource(R.string.enterprise_checklists_expired))
                Button(onClick = actions.onStart, enabled = !busy, modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-checklist-start-again")) {
                    Text(stringResource(R.string.enterprise_checklists_start_again))
                }
            }
        }
    }
    run.template.items.forEachIndexed { index, item ->
        val position = index + 1
        item(key = run.runId + "-" + position) {
            val saved = run.answers[index]
            var note by remember(run.runId, position, saved.note) { mutableStateOf(saved.note.orEmpty()) }
            ChecklistCard(Modifier.testTag("enterprise-checklist-item-$position")) {
                Text(stringResource(R.string.enterprise_checklists_item, position, item.label), style = MaterialTheme.typography.titleMedium)
                if (item.critical) Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Filled.Warning, contentDescription = null, tint = MaterialTheme.colorScheme.error, modifier = Modifier.size(20.dp))
                    Text(stringResource(R.string.enterprise_checklists_critical), color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.padding(start = 8.dp).testTag("enterprise-checklist-critical-$position"))
                }
                if (item.optional) Text(stringResource(R.string.enterprise_checklists_optional))
                val answers = listOfNotNull(ChecklistAnswer.OK, ChecklistAnswer.NOT_OK, ChecklistAnswer.NOT_APPLICABLE.takeIf { item.optional })
                val current = saved.answer?.let { stringResource(answerText(it)) } ?: stringResource(R.string.enterprise_checklists_unanswered)
                Column(Modifier.semantics { stateDescription = current }, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    for (answer in answers) {
                        val selected = saved.answer == answer
                        val label = stringResource(answerText(answer))
                        val onClick = { actions.onAnswer(run.runId, position, answer, note.takeIf { it.isNotBlank() }) }
                        val target = Modifier.fillMaxWidth().heightIn(min = 56.dp).semantics { this.selected = selected }
                            .testTag("enterprise-checklist-answer-$position-$answer")
                        if (selected) Button(onClick = onClick, enabled = !busy && !view.expired, modifier = target) { Text(label) }
                        else OutlinedButton(onClick = onClick, enabled = !busy && !view.expired, modifier = target) { Text(label) }
                    }
                }
                if (item.photoOnNotOk && saved.answer == ChecklistAnswer.NOT_OK) Notice(stringResource(R.string.enterprise_checklists_photo_required),
                    Modifier.testTag("enterprise-checklist-photo-$position"))
                val chosen = saved.answer
                if (chosen != null) {
                    OutlinedTextField(value = note, onValueChange = { if (it.codePointCount(0, it.length) <= ChecklistDefinition.MAX_NOTE) note = it },
                        label = { Text(stringResource(R.string.enterprise_checklists_note)) },
                        modifier = Modifier.fillMaxWidth().testTag("enterprise-checklist-note-$position"))
                    if (note != saved.note.orEmpty()) TextButton(onClick = { actions.onAnswer(run.runId, position, chosen, note.takeIf { it.isNotBlank() }) },
                        enabled = !busy && !view.expired, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-checklist-save-note-$position")) {
                        Text(stringResource(R.string.enterprise_checklists_save_note))
                    }
                }
            }
        }
    }
    item(key = "complete-" + run.runId) {
        // WARN with a critical issue asks for "Estou ciente das pendências" before completion (B-BR-EMP-438).
        val warned = run.policy.mode == PolicyMode.WARN && run.template.items.withIndex().any { (i, item) ->
            item.critical && run.answers[i].answer == ChecklistAnswer.NOT_OK }
        var aware by remember(run.runId) { mutableStateOf(false) }
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            if (warned) {
                Notice(stringResource(R.string.enterprise_checklists_acknowledge_copy))
                Row(Modifier.fillMaxWidth().heightIn(min = 56.dp).toggleable(aware, role = Role.Checkbox, onValueChange = { aware = it })
                    .testTag("enterprise-checklist-acknowledge"), verticalAlignment = Alignment.CenterVertically) {
                    Checkbox(checked = aware, onCheckedChange = null)
                    Text(stringResource(R.string.enterprise_checklists_acknowledge), modifier = Modifier.padding(start = 8.dp))
                }
            }
            Button(onClick = { actions.onComplete(run.runId, warned && aware) },
                enabled = !busy && !view.expired && run.answered == run.answers.size && (!warned || aware),
                modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-checklist-complete")) {
                Text(stringResource(R.string.enterprise_checklists_complete))
            }
        }
    }
}

private fun answerText(answer: String): Int = when (answer) {
    ChecklistAnswer.OK -> R.string.enterprise_checklists_answer_ok
    ChecklistAnswer.NOT_OK -> R.string.enterprise_checklists_answer_not_ok
    else -> R.string.enterprise_checklists_answer_na
}

/** The outcome shown in text and icon, the items that block, and the company's result once read. */
@Composable
private fun Outcome(view: ChecklistRunView, connected: Boolean, busy: Boolean, onSend: () -> Unit) {
    val run = view.run
    val device = run.deviceOutcome ?: return
    ChecklistCard(Modifier.testTag("enterprise-checklist-outcome")) {
        Text(stringResource(R.string.enterprise_checklists_outcome_title), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        OutcomeLine(device, "enterprise-checklist-device-outcome")
        val blocking = view.blocking.mapNotNull { run.template.items.getOrNull(it - 1)?.label }
        if (blocking.isNotEmpty()) Text(stringResource(R.string.enterprise_checklists_blocking_items, blocking.joinToString(", ")),
            modifier = Modifier.testTag("enterprise-checklist-blocking-items"))
        Text(stringResource(R.string.enterprise_checklists_company_rechecks))
        view.sync?.let { sync ->
            val status = stringResource(statusText(sync))
            val reason = view.code?.takeIf { sync == EnterpriseSyncStatus.NEEDS_REVIEW || sync == EnterpriseSyncStatus.REJECTED }?.let { stringResource(reasonText(it)) }
            Text(listOfNotNull(stringResource(R.string.enterprise_checklists_sync, status), reason).joinToString(". "), style = MaterialTheme.typography.labelLarge,
                modifier = Modifier.testTag("enterprise-checklist-sync").semantics { stateDescription = status })
        }
        val server = run.server
        if (server == null) Text(stringResource(R.string.enterprise_checklists_server_waiting))
        else {
            Text(stringResource(R.string.enterprise_checklists_server_title), style = MaterialTheme.typography.titleSmall)
            OutcomeLine(server.outcome, "enterprise-checklist-server-outcome")
            if (server.outcomeDiffers) Column(Modifier.testTag("enterprise-checklist-recalculated").semantics(mergeDescendants = true) { }) {
                Text(stringResource(R.string.enterprise_checklists_recalculated), style = MaterialTheme.typography.titleSmall)
                Text(stringResource(R.string.enterprise_checklists_both_outcomes, stringResource(outcomeText(device)), stringResource(outcomeText(server.recheckOutcome))))
            }
            if (server.releasedAt != null) Text(stringResource(R.string.enterprise_checklists_released), modifier = Modifier.testTag("enterprise-checklist-released"))
        }
        if (view.sync != EnterpriseSyncStatus.ACCEPTED) OutlinedButton(onClick = onSend, enabled = connected && !busy,
            modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-checklist-send")) {
            Text(stringResource(R.string.enterprise_checklists_send_now))
        }
    }
}

/** An outcome in text and icon; BLOCKING is never shown by colour alone. */
@Composable
internal fun OutcomeLine(outcome: ChecklistOutcome, tag: String) {
    val text = stringResource(outcomeText(outcome))
    val (icon, tint) = when (outcome) {
        ChecklistOutcome.BLOCKING -> Icons.Filled.Block to MaterialTheme.colorScheme.error
        ChecklistOutcome.COMPLETED_WITH_ISSUES -> Icons.Filled.Warning to MaterialTheme.colorScheme.onSurface
        ChecklistOutcome.COMPLETED_OK -> Icons.Filled.CheckCircle to MaterialTheme.colorScheme.onSurface
    }
    Row(Modifier.testTag(tag).semantics(mergeDescendants = true) { stateDescription = text }, verticalAlignment = Alignment.CenterVertically) {
        Icon(icon, contentDescription = text, tint = tint, modifier = Modifier.size(28.dp))
        Text(text, style = MaterialTheme.typography.titleLarge, modifier = Modifier.padding(start = 8.dp))
    }
}

@Composable
private fun Feedback(state: ChecklistViewState, connected: Boolean) {
    if (!connected) Notice(stringResource(R.string.enterprise_checklists_send_offline), Modifier.testTag("enterprise-checklist-offline"))
    if (state.busy) CircularProgressIndicator()
    val refusal = state.refusal
    val message = when {
        state.saveFailed -> R.string.enterprise_checklists_save_failed
        refusal != null -> refusalText(refusal)
        state.sendFailed -> R.string.enterprise_checklists_send_failed
        state.refreshFailed -> R.string.enterprise_checklists_refresh_failed
        state.lastSaved != null -> R.string.enterprise_checklists_saved
        else -> null
    }
    message?.let { Notice(stringResource(it), Modifier.testTag(if (it == R.string.enterprise_checklists_saved) "enterprise-checklist-saved" else "enterprise-checklist-feedback")) }
}

/** The trip start form's departure line (B-BR-EMP-443): the latest valid departure run and whether it blocks the start. */
@Composable
internal fun DepartureChecklistLine(state: ChecklistViewState?, assetId: String, onOpen: () -> Unit) {
    val matches = state != null && state.loaded && state.assetId == assetId && state.checklistContext == ChecklistContext.DEPARTURE
    val departure = if (matches) state?.departure else null
    ChecklistCard(Modifier.testTag("enterprise-trip-departure-checklist")) {
        when {
            departure != null -> {
                Text(stringResource(R.string.enterprise_checklists_trip_last, stringResource(outcomeText(departure.outcome!!))))
                OutcomeLine(departure.outcome!!, "enterprise-trip-departure-outcome")
                if (ChecklistPresentation.startBlocked(departure)) Notice(stringResource(R.string.enterprise_checklists_trip_blocked),
                    Modifier.testTag("enterprise-trip-departure-blocked"))
            }
            matches && state?.applicable?.required == false -> Text(stringResource(R.string.enterprise_checklists_none_required))
            else -> Text(stringResource(R.string.enterprise_checklists_trip_none))
        }
        OutlinedButton(onClick = onOpen, modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-trip-departure-open")) {
            Text(stringResource(R.string.enterprise_checklists_departure_title))
        }
    }
}

/** The device effect of BR-CHECK-006 on the trip start form: the start waits while the chosen vehicle's departure run blocks. */
internal fun departureBlocks(state: ChecklistViewState?, assetId: String): Boolean =
    state != null && state.assetId == assetId && state.checklistContext == ChecklistContext.DEPARTURE && ChecklistPresentation.startBlocked(state.departure)

private sealed interface HomeMode {
    data object Picker : HomeMode
    class Moment(val assetId: String, val label: String) : HomeMode
    class Run(val assetId: String, val label: String) : HomeMode
}

/** The enterprise "Checklists" entry: choose an asset with the FLEET-013 picker, then the moment, then the run screen. */
@Composable
internal fun ChecklistHomePanel(
    assets: MyAssetsViewState,
    state: ChecklistViewState,
    connected: Boolean,
    actions: ChecklistActions,
    onOpenRun: (assetId: String, checklistContext: ChecklistContext) -> Unit,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var mode by remember { mutableStateOf<HomeMode>(HomeMode.Picker) }
    LaunchedEffect(assets.context?.partition, assets.epoch) { mode = HomeMode.Picker }
    when (val current = mode) {
        HomeMode.Picker -> EnterpriseAssetPicker(state = assets, connected = connected,
            onChoose = { id, _ ->
                val asset = assets.snapshot?.assets?.firstOrNull { it.asset.id == id }?.asset
                if (asset != null) mode = HomeMode.Moment(id, listOf(asset.name, asset.fleetNumber, asset.plate).filter { it.isNotBlank() }.joinToString(" · "))
            },
            onCancel = onBack, modifier = modifier.testTag("enterprise-checklist-picker"))
        is HomeMode.Moment -> Column(modifier.testTag("enterprise-checklist-moment").padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.enterprise_checklists_choose_context), style = MaterialTheme.typography.headlineSmall, modifier = Modifier.semantics { heading() })
            Text(stringResource(R.string.enterprise_checklists_vehicle, current.label))
            for (moment in ChecklistContext.entries) {
                Button(onClick = { onOpenRun(current.assetId, moment); mode = HomeMode.Run(current.assetId, current.label) },
                    modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-checklist-moment-${moment.name}")) {
                    Text(stringResource(contextText(moment)))
                }
            }
            TextButton(onClick = { mode = HomeMode.Picker }, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
                Text(stringResource(R.string.enterprise_checklists_change_vehicle))
            }
        }
        is HomeMode.Run -> ChecklistRunPanel(state, connected, current.label, actions, onBack = { mode = HomeMode.Moment(current.assetId, current.label) }, modifier)
    }
}

/** Home entry card; it opens the protected book, never a server read by itself. */
@Composable
internal fun ChecklistsHomeCard(onOpen: () -> Unit, modifier: Modifier = Modifier) {
    ChecklistCard(modifier.testTag("enterprise-checklists-home")) {
        Text(stringResource(R.string.enterprise_checklists_title), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.enterprise_checklists_home_copy))
        Button(onClick = onOpen, modifier = Modifier.fillMaxWidth().testTag("enterprise-checklists-open")) { Text(stringResource(R.string.enterprise_checklists_open)) }
    }
}

@Composable
private fun ChecklistCard(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
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
