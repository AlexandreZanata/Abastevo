package com.anpfuel.app.ui.enterprise.tracking

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.LocationOn
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
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
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.enterprise.tracking.MyTrackingViewState
import com.anpfuel.application.enterprise.tracking.TrackingActivity
import com.anpfuel.application.enterprise.tracking.TrackingHistoryEntry
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.tracking.LocationPermission
import com.anpfuel.domain.enterprise.tracking.TrackingGapReason
import com.anpfuel.domain.enterprise.tracking.TrackingPolicy
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/*
 * EMP-GPS-015 (B-BR-EMP-548..551, SCR-GPS-02/04): the indicator drawn once in the enterprise header and "Meu rastreamento".
 * Text and icon, never colour alone; counts, causes and instants only, never a coordinate.
 */

@Composable
private fun formatted(instant: Instant): String {
    val locale = LocalConfiguration.current.locales[0]
    val zone = remember { ZoneId.systemDefault() }
    val formatter = remember(locale, zone) { DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM).withLocale(locale).withZone(zone) }
    return formatter.format(instant)
}

/** One line per open gap reason in the indicator, the same words as the ongoing notification (EMP-GPS-010). */
private fun gapNotice(reason: TrackingGapReason): Int = when (reason) {
    TrackingGapReason.PROVIDER_DISABLED -> R.string.enterprise_tracking_notification_gap_provider
    TrackingGapReason.SIGNAL_LOST -> R.string.enterprise_tracking_notification_gap_signal
    TrackingGapReason.MOCK_LOCATION -> R.string.enterprise_tracking_notification_gap_mock
}

/**
 * The organization tracking indicator of every enterprise screen (B-BR-EMP-548): shown only while this process's organization
 * collector listens; names the organization and the phone; a tap opens "Meu rastreamento".
 */
@Composable
internal fun EnterpriseTrackingIndicator(activity: TrackingActivity?, organizationName: String?, onOpen: () -> Unit, modifier: Modifier = Modifier) {
    val current = activity ?: return
    val name = current.organizationName ?: organizationName ?: ""
    val title = stringResource(R.string.enterprise_tracking_indicator_active, name)
    val open = stringResource(R.string.enterprise_tracking_indicator_open)
    Row(modifier.fillMaxWidth().heightIn(min = 48.dp).clickable(onClickLabel = open, role = Role.Button, onClick = onOpen)
        .padding(horizontal = 16.dp, vertical = 8.dp).testTag("enterprise-tracking-indicator")
        .semantics(mergeDescendants = true) { liveRegion = LiveRegionMode.Polite },
        horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
        Icon(Icons.Filled.LocationOn, contentDescription = null)
        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(title, style = MaterialTheme.typography.titleSmall)
            Text(stringResource(R.string.enterprise_tracking_indicator_detail, formatted(current.since)), style = MaterialTheme.typography.bodySmall)
            Text(stringResource(if (current.precision == LocationPermission.APPROXIMATE) R.string.enterprise_tracking_indicator_approximate
                else R.string.enterprise_tracking_indicator_precise), style = MaterialTheme.typography.bodySmall)
            current.gap?.let { Text(stringResource(gapNotice(it)), style = MaterialTheme.typography.bodySmall, modifier = Modifier.testTag("enterprise-tracking-indicator-gap")) }
        }
    }
}

@Composable
private fun TransparencyCard(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    val fill = MaterialTheme.colorScheme.surfaceContainerLow
    Card(modifier.fillMaxWidth(), shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = fill), border = AnpContainerDefaults.borderStroke(fill)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) { content() }
    }
}

/** Home entry card of "Meu rastreamento"; it reads the phone's sealed records, never the network. */
@Composable
internal fun MyTrackingHomeCard(onOpen: () -> Unit, modifier: Modifier = Modifier) {
    TransparencyCard(modifier.testTag("enterprise-my-tracking-home")) {
        Text(stringResource(R.string.enterprise_tracking_mine_title), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.enterprise_tracking_mine_home))
        Button(onClick = onOpen, modifier = Modifier.fillMaxWidth().testTag("enterprise-my-tracking-open")) {
            Text(stringResource(R.string.enterprise_tracking_indicator_open))
        }
    }
}

@Composable
private fun Section(title: Int, text: Int) {
    Text(stringResource(title), style = MaterialTheme.typography.titleSmall, modifier = Modifier.semantics { heading() })
    Text(stringResource(text))
}

/** SCR-GPS-04 "Meu rastreamento" (B-BR-EMP-549..551): what is collected, the state now and the own sessions of this phone. */
@Composable
internal fun MyTrackingPanel(state: MyTrackingViewState, activity: TrackingActivity?, onBack: () -> Unit, onRetry: () -> Unit,
    modifier: Modifier = Modifier) {
    LazyColumn(modifier.testTag("enterprise-my-tracking-panel").padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        item {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(stringResource(R.string.enterprise_tracking_mine_title), style = MaterialTheme.typography.headlineSmall,
                    modifier = Modifier.semantics { heading() })
                TextButton(onClick = onBack, modifier = Modifier.fillMaxWidth().testTag("enterprise-my-tracking-back")) {
                    Text(stringResource(R.string.enterprise_tracking_mine_back))
                }
                Text(stringResource(if (activity != null && activity.partition == state.context?.partition) R.string.enterprise_tracking_mine_now_active
                    else R.string.enterprise_tracking_mine_now_inactive), style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.testTag("enterprise-my-tracking-now").semantics { liveRegion = LiveRegionMode.Polite })
            }
        }
        item {
            TransparencyCard(Modifier.testTag("enterprise-my-tracking-notice")) {
                Section(R.string.enterprise_tracking_mine_collected_title, R.string.enterprise_tracking_mine_collected)
                Section(R.string.enterprise_tracking_mine_not_collected_title, R.string.enterprise_tracking_mine_not_collected)
                Section(R.string.enterprise_tracking_mine_who_title, R.string.enterprise_tracking_mine_who)
                Section(R.string.enterprise_tracking_mine_retention_title, R.string.enterprise_tracking_mine_retention)
                Section(R.string.enterprise_tracking_mine_device_title, R.string.enterprise_tracking_mine_device)
                val policy = state.history?.policy
                Text(if (policy == null) stringResource(R.string.enterprise_tracking_mine_policy_unknown)
                    else stringResource(R.string.enterprise_tracking_mine_policy, stringResource(when (policy.policy) {
                        TrackingPolicy.EXPECTED -> R.string.enterprise_tracking_mine_policy_expected
                        TrackingPolicy.OPTIONAL -> R.string.enterprise_tracking_mine_policy_optional
                        TrackingPolicy.OFF -> R.string.enterprise_tracking_mine_policy_off
                    }), policy.noticeVersion ?: "-"), style = MaterialTheme.typography.bodySmall, modifier = Modifier.testTag("enterprise-my-tracking-policy"))
            }
        }
        if (state.error != null || state.unlockRequired || state.unrecoverable) item {
            TransparencyCard(Modifier.testTag("enterprise-my-tracking-error")) {
                Text(stringResource(R.string.enterprise_tracking_mine_unavailable), modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite })
                Button(onClick = onRetry, enabled = !state.busy, modifier = Modifier.fillMaxWidth().testTag("enterprise-my-tracking-retry")) {
                    Text(stringResource(R.string.enterprise_tracking_mine_retry))
                }
            }
        }
        val history = state.history
        if (history != null) {
            item { Text(stringResource(R.string.enterprise_tracking_mine_sessions_title), style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.semantics { heading() }) }
            if (history.entries.isEmpty()) item {
                Text(stringResource(R.string.enterprise_tracking_mine_empty), modifier = Modifier.testTag("enterprise-my-tracking-empty"))
            }
            items(history.entries, key = { it.session.sessionId }) { entry -> SessionCard(entry) }
        }
    }
}

@Composable
private fun SessionCard(entry: TrackingHistoryEntry) {
    val session = entry.session
    TransparencyCard(Modifier.testTag("enterprise-my-tracking-session")) {
        entry.vehicle?.let { label ->
            Text(stringResource(R.string.enterprise_tracking_mine_session_vehicle, listOf(label.name, label.fleetNumber, label.plate)
                .filter { it.isNotBlank() }.joinToString(" · ")), style = MaterialTheme.typography.titleSmall)
        }
        Text(stringResource(trackingStateText(session.state)))
        val started = entry.startedAt
        Text(if (started == null) stringResource(R.string.enterprise_tracking_mine_session_never)
            else stringResource(R.string.enterprise_tracking_mine_session_started, formatted(started)))
        val ended = entry.endedAt
        if (started != null) Text(if (ended == null) stringResource(R.string.enterprise_tracking_mine_session_ongoing)
            else stringResource(R.string.enterprise_tracking_mine_session_ended, formatted(ended)))
        for (interruption in session.interruptions) {
            val label = stringResource(trackingStateText(interruption.state))
            val cause = stringResource(trackingCauseText(interruption.cause) ?: R.string.enterprise_tracking_cause_other)
            val end = interruption.end
            Text(if (end == null) stringResource(R.string.enterprise_tracking_interruption_open, label, formatted(interruption.start), cause)
                else stringResource(R.string.enterprise_tracking_interruption_closed, label, formatted(interruption.start), formatted(end), cause),
                style = MaterialTheme.typography.bodySmall)
        }
        for (gap in session.gaps) {
            val end = gap.end
            if (end == null) Text(stringResource(when (gap.reason) {
                TrackingGapReason.SIGNAL_LOST -> R.string.enterprise_tracking_gap_signal_lost
                TrackingGapReason.PROVIDER_DISABLED -> R.string.enterprise_tracking_gap_provider_disabled
                TrackingGapReason.MOCK_LOCATION -> R.string.enterprise_tracking_gap_mock_location
            }, formatted(gap.start)), style = MaterialTheme.typography.bodySmall)
            else Text(stringResource(when (gap.reason) {
                TrackingGapReason.SIGNAL_LOST -> R.string.enterprise_tracking_mine_gap_signal
                TrackingGapReason.PROVIDER_DISABLED -> R.string.enterprise_tracking_mine_gap_provider
                TrackingGapReason.MOCK_LOCATION -> R.string.enterprise_tracking_mine_gap_mock
            }, formatted(gap.start), formatted(end)), style = MaterialTheme.typography.bodySmall)
        }
        if (session.coarse) Text(stringResource(R.string.enterprise_tracking_coarse_parts), style = MaterialTheme.typography.bodySmall)
        if (session.pendingFixes > 0 || session.batches.isNotEmpty()) {
            Text(stringResource(R.string.enterprise_tracking_pending, session.pendingFixes), style = MaterialTheme.typography.bodySmall)
            Text(stringResource(R.string.enterprise_tracking_batches, session.batches.count { it.sync == EnterpriseSyncStatus.ACCEPTED },
                session.batches.count { it.sync != EnterpriseSyncStatus.ACCEPTED }), style = MaterialTheme.typography.bodySmall)
        }
    }
}
