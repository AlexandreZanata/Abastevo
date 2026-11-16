package com.anpfuel.app.ui.enterprise.tracking

import android.Manifest
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.LocationOff
import androidx.compose.material.icons.filled.LocationOn
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.enterprise.statusText
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.enterprise.tracking.TrackingLocalFailure
import com.anpfuel.application.enterprise.tracking.TrackingScreen
import com.anpfuel.application.enterprise.tracking.TrackingSessionView
import com.anpfuel.domain.enterprise.tracking.LocationPermission
import com.anpfuel.domain.enterprise.tracking.TrackingCause
import com.anpfuel.domain.enterprise.tracking.TrackingDevice
import com.anpfuel.domain.enterprise.tracking.TrackingGapReason
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.tracking.TrackingPolicy
import com.anpfuel.domain.enterprise.tracking.TrackingSessionState
import java.time.Instant

/*
 * SCR-GPS-01/02/03 on the trip screen (EMP-GPS-002, B-BR-EMP-495..500): what the policy offers, the notice at each
 * start, the state in text and icon, the stop and the explicit resume. No coordinate is ever shown.
 */

internal fun trackingStateText(state: TrackingSessionState): Int = when (state) {
    TrackingSessionState.NOT_STARTED -> R.string.enterprise_tracking_state_not_started
    TrackingSessionState.RECORDING -> R.string.enterprise_tracking_state_recording
    TrackingSessionState.SUSPENDED -> R.string.enterprise_tracking_state_suspended
    TrackingSessionState.INTERRUPTED -> R.string.enterprise_tracking_state_interrupted
    TrackingSessionState.ENDED -> R.string.enterprise_tracking_state_ended
}

internal fun trackingCauseText(cause: TrackingCause): Int? = when (cause) {
    TrackingCause.PERMISSION_DENIED -> R.string.enterprise_tracking_cause_permission_denied
    TrackingCause.NOTIFICATION_BLOCKED -> R.string.enterprise_tracking_cause_notification_blocked
    TrackingCause.PROCESS_STOPPED -> R.string.enterprise_tracking_cause_process_stopped
    TrackingCause.DEVICE_RESTARTED -> R.string.enterprise_tracking_cause_device_restarted
    TrackingCause.STOPPED_BY_DRIVER -> R.string.enterprise_tracking_cause_stopped_by_driver
    TrackingCause.OWNER_CHANGED -> R.string.enterprise_tracking_cause_owner_changed
    TrackingCause.PERMISSION_REVOKED -> R.string.enterprise_tracking_cause_permission_revoked
    TrackingCause.PRECISION_REDUCED -> R.string.enterprise_tracking_approximate
    TrackingCause.NOT_STARTED -> null
    else -> R.string.enterprise_tracking_cause_other
}

private fun refusalText(failure: TrackingLocalFailure): Int = when (failure) {
    TrackingLocalFailure.POLICY_OFF -> R.string.enterprise_tracking_refusal_off
    TrackingLocalFailure.POLICY_UNKNOWN -> R.string.enterprise_tracking_policy_unknown
    TrackingLocalFailure.BUSY -> R.string.enterprise_tracking_refusal_busy
    else -> R.string.enterprise_tracking_refusal_other
}

/**
 * The tracking card of an open trip. [inProgress] is the trip's own local state; [onStart] receives the permission and
 * notification visibility observed right after the system dialog, which the start records as they are.
 */
@Composable
internal fun TrackingCard(
    tripId: String,
    inProgress: Boolean,
    tracking: TrackingScreen?,
    refusal: TrackingLocalFailure?,
    policyFailed: Boolean,
    connected: Boolean,
    busy: Boolean,
    formatted: @Composable (Instant) -> String,
    onStart: (TrackingDevice) -> Unit,
    onStop: () -> Unit,
    onRefresh: () -> Unit,
    /** EMP-GPS-015: the session this phone collects now; null when not known (nothing is said then). */
    collecting: (() -> String?)? = null,
) {
    val context = LocalContext.current
    val session: TrackingSessionView? = tracking?.session(tripId)
    val policy = tracking?.policy
    var declined by rememberSaveable(tripId) { mutableStateOf(false) }
    val request = rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) {
        val platform = AndroidTrackingPlatform(context)
        onStart(TrackingDevice(platform.permission(), platform.notificationVisible()))
    }
    fun ask() {
        val wanted = buildList {
            add(Manifest.permission.ACCESS_FINE_LOCATION); add(Manifest.permission.ACCESS_COARSE_LOCATION)
            if (Build.VERSION.SDK_INT >= 33) add(Manifest.permission.POST_NOTIFICATIONS)
        }
        request.launch(wanted.toTypedArray())
    }
    val fill = MaterialTheme.colorScheme.surfaceContainerLow
    Card(Modifier.fillMaxWidth().testTag("enterprise-tracking-card"), shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = fill), border = AnpContainerDefaults.borderStroke(fill)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(stringResource(R.string.enterprise_tracking_title), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
            if (session != null) {
                val stateLabel = stringResource(trackingStateText(session.state))
                Column(Modifier.testTag("enterprise-tracking-state").semantics(mergeDescendants = true) {
                    stateDescription = stateLabel; liveRegion = LiveRegionMode.Polite
                }) {
                    Icon(if (session.state == TrackingSessionState.RECORDING) Icons.Filled.LocationOn else Icons.Filled.LocationOff, contentDescription = null)
                    Text(stateLabel, style = MaterialTheme.typography.titleSmall)
                    Text(stringResource(R.string.enterprise_tracking_since, formatted(session.since)))
                    session.cause?.let(::trackingCauseText)?.let { Text(stringResource(it), modifier = Modifier.testTag("enterprise-tracking-cause")) }
                    if (session.precision == LocationPermission.APPROXIMATE && session.cause != TrackingCause.PRECISION_REDUCED)
                        Text(stringResource(R.string.enterprise_tracking_approximate))
                    if (session.refused) Text(stringResource(R.string.enterprise_tracking_refused, session.refusalCode ?: "-"),
                        modifier = Modifier.testTag("enterprise-tracking-refused"))
                    // B-BR-EMP-548: a stored recording session is not shown as collected when this phone does not listen for it.
                    if (session.state == TrackingSessionState.RECORDING && collecting != null && collecting() != session.sessionId)
                        Text(stringResource(R.string.enterprise_tracking_not_collecting_here), modifier = Modifier.testTag("enterprise-tracking-not-collecting"))
                }
                // EMP-GPS-003 (B-BR-EMP-503..505): counts and gaps only, never a position.
                session.openGap?.let { gap ->
                    Text(stringResource(when (gap.reason) {
                        TrackingGapReason.SIGNAL_LOST -> R.string.enterprise_tracking_gap_signal_lost
                        TrackingGapReason.PROVIDER_DISABLED -> R.string.enterprise_tracking_gap_provider_disabled
                        TrackingGapReason.MOCK_LOCATION -> R.string.enterprise_tracking_gap_mock_location
                    }, formatted(gap.start)), modifier = Modifier.testTag("enterprise-tracking-gap"))
                }
                if (session.gaps.isNotEmpty()) Text(stringResource(R.string.enterprise_tracking_gaps_count, session.gaps.size), style = MaterialTheme.typography.bodySmall)
                // EMP-GPS-010 (B-BR-EMP-544/546): every interruption stays listed with its cause and bounds; approximate parts are said.
                for (interruption in session.interruptions) {
                    val label = stringResource(trackingStateText(interruption.state))
                    val cause = stringResource(trackingCauseText(interruption.cause) ?: R.string.enterprise_tracking_cause_other)
                    val end = interruption.end
                    Text(if (end == null) stringResource(R.string.enterprise_tracking_interruption_open, label, formatted(interruption.start), cause)
                        else stringResource(R.string.enterprise_tracking_interruption_closed, label, formatted(interruption.start), formatted(end), cause),
                        style = MaterialTheme.typography.bodySmall, modifier = Modifier.testTag("enterprise-tracking-interruption"))
                }
                if (session.coarse && session.precision != LocationPermission.APPROXIMATE) Text(stringResource(R.string.enterprise_tracking_coarse_parts),
                    modifier = Modifier.testTag("enterprise-tracking-coarse"))
                if (session.pendingFixes > 0 || session.batches.isNotEmpty()) {
                    Text(stringResource(R.string.enterprise_tracking_pending, session.pendingFixes), modifier = Modifier.testTag("enterprise-tracking-pending"))
                    Text(stringResource(R.string.enterprise_tracking_batches, session.batches.count { it.sync == EnterpriseSyncStatus.ACCEPTED },
                        session.batches.count { it.sync != EnterpriseSyncStatus.ACCEPTED }), modifier = Modifier.testTag("enterprise-tracking-batches"))
                }
                for (record in session.records) Text(stringResource(R.string.enterprise_tracking_record,
                    stringResource(trackingStateText(stateAfter(record.type))), stringResource(statusText(record.sync))), style = MaterialTheme.typography.bodySmall)
            }
            refusal?.let { Text(stringResource(refusalText(it)), modifier = Modifier.testTag("enterprise-tracking-refusal")) }
            if (policyFailed) Text(stringResource(R.string.enterprise_tracking_policy_failed))
            val canAct = inProgress && session?.refused != true && session?.state != TrackingSessionState.ENDED
            when {
                !canAct -> Unit
                session?.state == TrackingSessionState.RECORDING -> OutlinedButton(onClick = onStop, enabled = !busy,
                    modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-tracking-stop")) {
                    Text(stringResource(R.string.enterprise_tracking_stop))
                }
                session == null && (policy == null || policy.policy == TrackingPolicy.OFF) -> {
                    // OFF hides every control; a policy never read says so (B-BR-EMP-495).
                    if (policy == null) Text(stringResource(R.string.enterprise_tracking_policy_unknown), modifier = Modifier.testTag("enterprise-tracking-unknown"))
                }
                declined && session == null -> TextButton(onClick = { declined = false }, modifier = Modifier.testTag("enterprise-tracking-reconsider")) {
                    Text(stringResource(R.string.enterprise_tracking_start))
                }
                else -> {
                    val sessionPolicy = session?.policy ?: policy?.policy
                    if (session == null) Text(stringResource(if (sessionPolicy == TrackingPolicy.EXPECTED) R.string.enterprise_tracking_policy_expected
                        else R.string.enterprise_tracking_policy_optional), modifier = Modifier.testTag("enterprise-tracking-policy"))
                    if (session != null && session.state != TrackingSessionState.NOT_STARTED) Text(stringResource(R.string.enterprise_tracking_resume_question),
                        style = MaterialTheme.typography.titleSmall)
                    // The notice is shown at each start and resume (BR-GPS-034).
                    Text(stringResource(R.string.enterprise_tracking_notice, policy?.noticeVersion ?: "-"), style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.testTag("enterprise-tracking-notice"))
                    Button(onClick = ::ask, enabled = !busy, modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-tracking-start")) {
                        Text(stringResource(if (session == null || session.state == TrackingSessionState.NOT_STARTED) R.string.enterprise_tracking_start
                            else R.string.enterprise_tracking_resume))
                    }
                    if (session == null) TextButton(onClick = { declined = true }, modifier = Modifier.fillMaxWidth().testTag("enterprise-tracking-without")) {
                        Text(stringResource(R.string.enterprise_tracking_without))
                    }
                }
            }
            if (inProgress && connected) TextButton(onClick = onRefresh, enabled = !busy, modifier = Modifier.testTag("enterprise-tracking-refresh")) {
                Text(stringResource(R.string.enterprise_tracking_policy_refresh))
            }
        }
    }
}

/** The state a recorded operation leads to, for its line in the record list. */
private fun stateAfter(type: String): TrackingSessionState = when (type) {
    com.anpfuel.domain.enterprise.tracking.TrackingOperations.START, com.anpfuel.domain.enterprise.tracking.TrackingOperations.RESUME,
    com.anpfuel.domain.enterprise.tracking.TrackingOperations.REDUCE -> TrackingSessionState.RECORDING
    com.anpfuel.domain.enterprise.tracking.TrackingOperations.SUSPEND -> TrackingSessionState.SUSPENDED
    com.anpfuel.domain.enterprise.tracking.TrackingOperations.INTERRUPT -> TrackingSessionState.INTERRUPTED
    else -> TrackingSessionState.ENDED
}
