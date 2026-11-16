package com.anpfuel.app.ui.enterprise.trip

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.enterprise.reasonText
import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.fleet.MyAssetsViewState
import com.anpfuel.domain.enterprise.trip.MissionAction
import com.anpfuel.domain.enterprise.trip.MissionAnswerState
import com.anpfuel.domain.enterprise.trip.MissionInboxView
import com.anpfuel.domain.enterprise.trip.MissionLocalFailure
import com.anpfuel.domain.enterprise.trip.MissionView
import com.anpfuel.domain.enterprise.trip.TripCommand
import com.anpfuel.domain.enterprise.trip.TripMissionLoad
import com.anpfuel.domain.enterprise.trip.TripMissionState

/*
 * EMP-TRIP-021 (B-BR-EMP-527..531): the missions inbox inside the trip panel. The copy always states its age; the mission
 * state comes from the copy, an answer's state only from its own receipt; refreshing is the only online action and says so
 * without a connection. No identifier, code or protocol word is shown.
 */

internal fun missionRefusalText(failure: MissionLocalFailure): Int = when (failure) {
    MissionLocalFailure.UNKNOWN_MISSION -> R.string.enterprise_missions_refusal_unknown
    MissionLocalFailure.NOT_ANSWERABLE -> R.string.enterprise_missions_refusal_not_answerable
    MissionLocalFailure.ALREADY_ANSWERED -> R.string.enterprise_missions_refusal_answered
    MissionLocalFailure.NOT_CONFIRMED -> R.string.enterprise_missions_start_not_confirmed
    MissionLocalFailure.ALREADY_STARTED -> R.string.enterprise_missions_start_already
    MissionLocalFailure.WRONG_VEHICLE -> R.string.enterprise_missions_refusal_wrong_vehicle
    MissionLocalFailure.FOREIGN -> R.string.enterprise_missions_refusal_foreign
    MissionLocalFailure.FULL -> R.string.enterprise_missions_refusal_full
    MissionLocalFailure.DUPLICATE -> R.string.enterprise_trips_save_failed
}

private fun stateText(state: TripMissionState?): Int? = when (state) {
    null -> null
    TripMissionState.ASSIGNED -> R.string.enterprise_missions_state_assigned
    TripMissionState.ACCEPTED -> R.string.enterprise_missions_state_accepted
    TripMissionState.DECLINED -> R.string.enterprise_missions_state_declined
    TripMissionState.IN_PROGRESS -> R.string.enterprise_missions_state_in_progress
    TripMissionState.COMPLETED -> R.string.enterprise_missions_state_completed
    TripMissionState.CANCELLED -> R.string.enterprise_missions_state_cancelled
    TripMissionState.DRAFT, TripMissionState.PLANNED -> R.string.enterprise_missions_state_planning
}

private fun answerText(answer: MissionAnswerState): Int? = when (answer) {
    MissionAnswerState.NONE -> null
    MissionAnswerState.SAVED -> R.string.enterprise_missions_answer_saved
    MissionAnswerState.ACCEPTED -> R.string.enterprise_missions_answer_accepted
    MissionAnswerState.DECLINE_RECORDED -> R.string.enterprise_missions_answer_declined
    MissionAnswerState.REFUSED -> R.string.enterprise_missions_answer_refused
    MissionAnswerState.NOT_CONFIRMED_CHANGED -> R.string.enterprise_missions_answer_changed
    MissionAnswerState.NOT_CONFIRMED_REMOVED -> R.string.enterprise_missions_answer_removed
}

/** "Atualizadas há …" from the device receipt clock; never without the age (BR-OFFL-016). */
@Composable
private fun ageText(view: MissionInboxView): String {
    val age = view.ageMillis ?: return stringResource(R.string.enterprise_missions_age_unknown)
    val minutes = age / 60_000
    return when {
        minutes < 1 -> stringResource(R.string.enterprise_missions_age_now)
        minutes < 60 -> stringResource(R.string.enterprise_missions_age, stringResource(R.string.enterprise_missions_age_minutes, minutes.toInt()))
        minutes < 48 * 60 -> stringResource(R.string.enterprise_missions_age, stringResource(R.string.enterprise_missions_age_hours, (minutes / 60).toInt()))
        else -> stringResource(R.string.enterprise_missions_age, stringResource(R.string.enterprise_missions_age_days, (minutes / 1440).toInt()))
    }
}

/** The inbox header and its rows, as separate list items. */
internal fun LazyListScope.missionsSection(
    view: MissionInboxView?,
    unavailable: Boolean,
    connected: Boolean,
    busy: Boolean,
    refreshed: Boolean,
    refreshFailed: EnterpriseAccessError?,
    onRefresh: () -> Unit,
    onOpen: (String) -> Unit,
) {
    item(key = "missions-header") {
        TripCard(Modifier.testTag("enterprise-missions")) {
            Text(stringResource(R.string.enterprise_missions_section_title), style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.semantics { heading() })
            when {
                unavailable -> Notice(stringResource(R.string.enterprise_missions_unavailable), Modifier.testTag("enterprise-missions-unavailable"))
                view == null || !view.refreshed -> Notice(stringResource(R.string.enterprise_missions_never), Modifier.testTag("enterprise-missions-never"))
                else -> {
                    Text(ageText(view), modifier = Modifier.testTag("enterprise-missions-age"))
                    if (view.stale) Notice(stringResource(R.string.enterprise_missions_stale), Modifier.testTag("enterprise-missions-stale"))
                    if (view.truncated) Text(stringResource(R.string.enterprise_missions_truncated))
                }
            }
            if (refreshed) Notice(stringResource(R.string.enterprise_missions_refreshed), Modifier.testTag("enterprise-missions-refreshed"))
            if (refreshFailed != null) Notice(stringResource(R.string.enterprise_missions_refresh_failed), Modifier.testTag("enterprise-missions-refresh-failed"))
            // Online only (BR-OFFL-017): disabled with its reason, never queued as if it had happened.
            OutlinedButton(onClick = onRefresh, enabled = connected && !busy, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)
                .testTag("enterprise-missions-refresh")) { Text(stringResource(R.string.enterprise_missions_refresh)) }
            if (!connected) Text(stringResource(R.string.enterprise_missions_refresh_offline), modifier = Modifier.testTag("enterprise-missions-refresh-offline"))
        }
    }
    val missions = view?.missions.orEmpty()
    if (view != null && view.refreshed && missions.isEmpty()) item(key = "missions-empty") {
        TripCard(Modifier.testTag("enterprise-missions-empty")) { Text(stringResource(R.string.enterprise_missions_empty)) }
    }
    items(missions, key = { "mission-" + it.missionId }) { mission ->
        TripCard(Modifier.testTag("enterprise-mission-row-${mission.missionId}")) {
            MissionSummary(mission)
            OutlinedButton(onClick = { onOpen(mission.missionId) }, enabled = !busy,
                modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-mission-open-${mission.missionId}")) {
                Text(stringResource(R.string.enterprise_missions_open))
            }
        }
    }
}

@Composable
private fun MissionSummary(mission: MissionView) {
    val item = mission.item
    if (item != null) {
        Text(item.title, style = MaterialTheme.typography.titleSmall)
        Text(stringResource(R.string.enterprise_missions_window, formatted(item.startsAt), formatted(item.endsAt)))
    }
    val state = stateText(mission.state)?.let { stringResource(it) }
    val answer = answerText(mission.answer)?.let { stringResource(it) }
    val code = mission.answerCode?.takeIf { mission.answer == MissionAnswerState.REFUSED }?.let { stringResource(reasonText(it)) }
    Column(Modifier.testTag("enterprise-mission-state-${mission.missionId}").semantics(mergeDescendants = true) {
        stateDescription = listOfNotNull(state, answer, code).joinToString(". ")
    }) {
        state?.let { Text(it, style = MaterialTheme.typography.labelLarge) }
        answer?.let { Text(it) }
        code?.let { Text(stringResource(R.string.enterprise_trips_reason, it)) }
    }
}

/** SCR-TRIP-02 style detail: plan, vehicle and the eligible actions of the offline matrix. */
@Composable
internal fun MissionDetail(
    mission: MissionView,
    assets: MyAssetsViewState,
    busy: Boolean,
    onAccept: () -> Unit,
    onDecline: () -> Unit,
    onStart: () -> Unit,
    onOpenTrip: (String) -> Unit,
    extra: @Composable () -> Unit = {},
) {
    TripCard(Modifier.testTag("enterprise-mission-detail")) {
        MissionSummary(mission)
        val item = mission.item
        if (item != null) {
            val asset = item.assetId?.let { id -> assets.snapshot?.assets?.firstOrNull { it.asset.id == id }?.asset }
            Text(when {
                item.assetId == null -> stringResource(R.string.enterprise_missions_vehicle_any)
                asset == null -> stringResource(R.string.enterprise_missions_vehicle_unknown)
                else -> stringResource(R.string.enterprise_missions_vehicle,
                    listOf(asset.name, asset.fleetNumber, asset.plate).filter { it.isNotBlank() }.joinToString(" · "))
            })
            val plan = item.plan
            if (plan == null) Text(stringResource(R.string.enterprise_missions_no_plan)) else {
                Text(plan.destinationText?.let { stringResource(R.string.enterprise_missions_destination, it) }
                    ?: stringResource(R.string.enterprise_missions_destination_unit), modifier = Modifier.testTag("enterprise-mission-destination"))
                Text(stringResource(R.string.enterprise_missions_reason, plan.reason))
                Text(when (plan.load) {
                    TripMissionLoad.NONE -> stringResource(R.string.enterprise_missions_load_none)
                    TripMissionLoad.CARGO -> stringResource(R.string.enterprise_missions_load_cargo, plan.cargoDescription.orEmpty())
                    TripMissionLoad.PASSENGERS -> stringResource(R.string.enterprise_missions_load_passengers, plan.passengerCount ?: 0)
                })
            }
        }
        mission.startBlocked?.let { Notice(stringResource(missionRefusalText(it)), Modifier.testTag("enterprise-mission-start-blocked")) }
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            if (MissionAction.ACCEPT in mission.actions) Button(onClick = onAccept, enabled = !busy,
                modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-mission-accept")) { Text(stringResource(R.string.enterprise_missions_accept)) }
            if (MissionAction.DECLINE in mission.actions) OutlinedButton(onClick = onDecline, enabled = !busy,
                modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-mission-decline")) { Text(stringResource(R.string.enterprise_missions_decline)) }
            if (MissionAction.START_TRIP in mission.actions) Button(onClick = onStart, enabled = !busy,
                modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-mission-start")) { Text(stringResource(R.string.enterprise_missions_start)) }
            val trip = mission.tripId
            if (MissionAction.OPEN_TRIP in mission.actions && trip != null) OutlinedButton(onClick = { onOpenTrip(trip) }, enabled = !busy,
                modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-mission-open-trip")) { Text(stringResource(R.string.enterprise_missions_open_trip)) }
            extra()
        }
    }
}

/** A decline needs a reason; a pause or resume may carry one; a cancellation needs one. */
@Composable
internal fun ReasonForm(title: Int, label: Int, save: Int, required: Boolean, busy: Boolean, tag: String, onCancel: () -> Unit, onSave: (String?) -> Unit) {
    var text by remember { mutableStateOf("") }
    TripCard(Modifier.testTag(tag)) {
        Text(stringResource(title), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        OutlinedTextField(value = text, onValueChange = { text = it }, label = { Text(stringResource(label)) },
            modifier = Modifier.fillMaxWidth().testTag("$tag-text"))
        Button(onClick = { onSave(text.takeIf { it.isNotBlank() }) }, enabled = !busy && (!required || text.isNotBlank()),
            modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("$tag-save")) { Text(stringResource(save)) }
        OutlinedButton(onClick = onCancel, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(stringResource(R.string.enterprise_trips_cancel)) }
    }
}

internal fun lifecycleForm(command: TripCommand): Triple<Int, Int, Int> = when (command) {
    TripCommand.PAUSE -> Triple(R.string.enterprise_missions_pause, R.string.enterprise_missions_reason_optional, R.string.enterprise_missions_save_pause)
    TripCommand.RESUME -> Triple(R.string.enterprise_missions_resume, R.string.enterprise_missions_reason_optional, R.string.enterprise_missions_save_resume)
    else -> Triple(R.string.enterprise_missions_cancel_trip, R.string.enterprise_missions_reason_cancel, R.string.enterprise_missions_save_cancel)
}
