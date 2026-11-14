package com.anpfuel.app.ui.enterprise

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.sync.EnterpriseSyncViewState
import com.anpfuel.domain.enterprise.sync.EnterpriseAttachmentEvidence
import com.anpfuel.domain.enterprise.sync.EnterpriseOperationIdentity
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncDetail
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncHistoryKind
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncItem
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncMode
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncProjection
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncTotals
import com.anpfuel.domain.enterprise.sync.SyncResult
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/** Sanitized presentation only. Gesture closures retain their caller's rendered owner. */
@Composable
internal fun EnterpriseSyncPanel(
    state: EnterpriseSyncViewState,
    connected: Boolean,
    onBack: () -> Unit,
    onRefresh: () -> Unit,
    onMore: () -> Unit,
    onMode: (EnterpriseSyncMode) -> Unit,
    onDetail: (EnterpriseOperationIdentity) -> Unit,
    onManualCheck: () -> Unit,
    onDismiss: (EnterpriseOperationIdentity, String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val unavailable = state.error != null || state.unlockRequired || state.unrecoverable
    val detail = state.detail
    // Actual SENDING is only a transient hint within a durable saved group.
    // It must not reorder records while a request starts or finishes.
    val groups = remember(state.items) { state.items.groupBy { it.status } }
    if (detail != null && !unavailable) {
        Column(modifier.testTag("enterprise-sync-panel").verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)) {
            SyncControls(state, connected, false, onBack, onRefresh, onManualCheck)
            SyncFeedback(state)
            SyncDetail(detail, detail.item.identity in state.active, !state.busy, state.generation, onDismiss)
        }
    } else LazyColumn(modifier.testTag("enterprise-sync-panel").padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp)) {
        item {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                SyncControls(state, connected, unavailable, onBack, onRefresh, onManualCheck)
                if (unavailable) Notice(stringResource(when {
                    state.unrecoverable -> R.string.enterprise_sync_unrecoverable
                    state.unlockRequired -> R.string.enterprise_sync_unlock
                    state.error == EnterpriseAccessError.ACCESS_DENIED -> R.string.enterprise_sync_denied
                    state.error in setOf(EnterpriseAccessError.SIGN_IN_REQUIRED, EnterpriseAccessError.FRESH_AUTH_REQUIRED) -> R.string.enterprise_sync_sign_in
                    state.error == EnterpriseAccessError.RATE_LIMITED -> R.string.enterprise_sync_limited
                    state.error == EnterpriseAccessError.STALE -> R.string.enterprise_sync_stale
                    else -> R.string.enterprise_sync_unavailable
                })) else {
                    SyncFeedback(state)
                    TextButton(onClick = { onMode(EnterpriseSyncMode.PENDING) }, enabled = !state.busy && state.mode != EnterpriseSyncMode.PENDING,
                        modifier = Modifier.fillMaxWidth().testTag("enterprise-sync-mode-pending")) { Text(stringResource(R.string.enterprise_sync_pending)) }
                    TextButton(onClick = { onMode(EnterpriseSyncMode.HISTORY) }, enabled = !state.busy && state.mode != EnterpriseSyncMode.HISTORY,
                        modifier = Modifier.fillMaxWidth().testTag("enterprise-sync-mode-history")) { Text(stringResource(R.string.enterprise_sync_history)) }
                    state.totals?.let { Text(stringResource(R.string.enterprise_sync_counts, it.savedCount, it.reviewCount,
                        if (state.mode == EnterpriseSyncMode.PENDING) it.visibleRejectedCount else it.rejectedCount, it.acceptedCount)) }
                    if (!state.busy && state.items.isEmpty()) Text(stringResource(
                        if (state.mode == EnterpriseSyncMode.PENDING) R.string.enterprise_sync_empty_pending else R.string.enterprise_sync_empty_history))
                }
            }
        }
        if (!unavailable) {
            for (status in listOf(EnterpriseSyncStatus.SAVED_ON_DEVICE, EnterpriseSyncStatus.NEEDS_REVIEW,
                EnterpriseSyncStatus.REJECTED, EnterpriseSyncStatus.ACCEPTED)) {
                val group = groups[status].orEmpty()
                if (group.isNotEmpty()) {
                    item(key = "enterprise-sync-group-${status.name}") {
                        Text(stringResource(statusText(status)), style = MaterialTheme.typography.titleLarge,
                            modifier = Modifier.fillMaxWidth().testTag("enterprise-sync-group-${status.name}").semantics { heading() })
                    }
                    items(group, key = { it.identity.operationId }) { item ->
                        val label = stringResource(statusText(EnterpriseSyncProjection.status(item, item.identity in state.active)))
                        Card(onClick = { onDetail(item.identity) }, enabled = !state.busy,
                            modifier = Modifier.fillMaxWidth().testTag("enterprise-sync-item-${item.identity.operationId}")
                                .semantics(mergeDescendants = true) { stateDescription = label }) {
                            Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                                Text(stringResource(R.string.enterprise_sync_record), style = MaterialTheme.typography.titleMedium)
                                ItemFacts(item, item.identity in state.active)
                            }
                        }
                    }
                }
            }
            if (state.hasMore) item {
                TextButton(onClick = onMore, enabled = !state.busy,
                    modifier = Modifier.fillMaxWidth().testTag("enterprise-sync-more")) { Text(stringResource(R.string.enterprise_sync_more)) }
            }
        }
    }
}

@Composable
private fun SyncControls(state: EnterpriseSyncViewState, connected: Boolean, unavailable: Boolean,
    onBack: () -> Unit, onRefresh: () -> Unit, onManualCheck: () -> Unit) {
    Text(stringResource(R.string.enterprise_sync_title), style = MaterialTheme.typography.headlineSmall,
        modifier = Modifier.semantics { heading() })
    TextButton(onClick = onBack, modifier = Modifier.fillMaxWidth()) {
        Text(stringResource(if (state.detail == null) R.string.enterprise_sync_back_home else R.string.enterprise_sync_back_list))
    }
    Text(stringResource(R.string.enterprise_sync_local_scope))
    if (!connected) Notice(stringResource(R.string.enterprise_sync_offline))
    Button(onClick = onManualCheck, enabled = connected && !state.busy && !unavailable,
        modifier = Modifier.fillMaxWidth().testTag("enterprise-sync-check")) { Text(stringResource(R.string.enterprise_sync_check)) }
    TextButton(onClick = onRefresh, enabled = !state.busy, modifier = Modifier.fillMaxWidth()) {
        Text(stringResource(R.string.enterprise_sync_refresh))
    }
    if (state.busy) {
        CircularProgressIndicator()
        Notice(stringResource(R.string.enterprise_sync_loading))
    }
}

@Composable
private fun SyncFeedback(state: EnterpriseSyncViewState) {
    if (state.manualAttention > 0) Notice(stringResource(R.string.enterprise_sync_manual_attention, state.manualAttention),
        Modifier.testTag("enterprise-sync-manual-attention"))
    state.manualWaitingMillis?.takeIf { it > 0 }?.let {
        val seconds = it / 1000 + if (it % 1000 == 0L) 0 else 1
        Notice(stringResource(R.string.enterprise_sync_manual_wait, seconds), Modifier.testTag("enterprise-sync-manual-wait"))
    }
    state.totals?.let { StorageNotices(it) }
}

/** Full selected-partition totals; never turns local emptiness into global delivery. */
@Composable
internal fun EnterpriseSyncHomeStatus(totals: EnterpriseSyncTotals?, onOpen: () -> Unit, modifier: Modifier = Modifier) {
    Card(modifier.testTag("enterprise-sync-home-status")) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.enterprise_sync_title), style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.semantics { heading() })
            Text(stringResource(R.string.enterprise_sync_local_scope))
            if (totals != null) {
                Text(stringResource(R.string.enterprise_sync_pending_count, totals.pendingCount))
                StorageNotices(totals)
                totals.latestConfirmedReceiptAt?.let { Text(stringResource(R.string.enterprise_sync_latest_receipt, formattedTime(it))) }
                totals.oldestPendingDeclaredAt?.let { Text(stringResource(R.string.enterprise_sync_oldest_declared, formattedTime(it))) }
            }
            Button(onClick = onOpen, modifier = Modifier.fillMaxWidth().testTag("enterprise-sync-open")) {
                Text(stringResource(R.string.enterprise_sync_open))
            }
        }
    }
}

@Composable
private fun StorageNotices(totals: EnterpriseSyncTotals) {
    if (totals.storage.pressure) Notice(stringResource(R.string.enterprise_sync_storage_pressure), Modifier.testTag("enterprise-sync-storage-pressure"))
    if (totals.storage.legacyCount > 0) Notice(stringResource(R.string.enterprise_sync_legacy, totals.storage.legacyCount), Modifier.testTag("enterprise-sync-legacy"))
    if (totals.unconfirmedAttachmentCount > 0) Text(stringResource(R.string.enterprise_sync_attachments_pending, totals.unconfirmedAttachmentCount))
}

@Composable
private fun ItemFacts(item: EnterpriseSyncItem, sending: Boolean) {
    Text(stringResource(statusText(EnterpriseSyncProjection.status(item, sending))), style = MaterialTheme.typography.titleMedium)
    item.receipt?.code?.let { Text(stringResource(reasonText(it))) }
    if (item.dismissedRejected) Text(stringResource(R.string.enterprise_sync_hidden))
    Text(stringResource(R.string.enterprise_sync_declared_at, formattedTime(item.declared.occurredAt, item.declared.entryZone)))
    if (item.attachmentCount > 0) Text(stringResource(R.string.enterprise_sync_attachments, item.attachmentCount))
}

@Composable
private fun SyncDetail(detail: EnterpriseSyncDetail, sending: Boolean, enabled: Boolean, generation: Long,
    onDismiss: (EnterpriseOperationIdentity, String) -> Unit) {
    val item = detail.item
    var confirm by remember(generation, item.identity) { mutableStateOf(false) }
    ItemFacts(item, sending)
    if (item.receipt != null) Text(stringResource(R.string.enterprise_sync_acceptance_separate))
    for (event in detail.history) when (event.kind) {
        EnterpriseSyncHistoryKind.DURABLE_SAVE -> Text(stringResource(R.string.enterprise_sync_save_unknown))
        EnterpriseSyncHistoryKind.RETRY_OBSERVATION -> {
            val time = event.time
            Text(if (time == null) stringResource(R.string.enterprise_sync_retry_unknown)
                else stringResource(R.string.enterprise_sync_retry_observation, formattedTime(time)))
        }
        EnterpriseSyncHistoryKind.SERVER_RECEIPT -> event.time?.let { Text(stringResource(R.string.enterprise_sync_receipt_at, formattedTime(it))) }
    }
    if (item.receipt?.warnings?.contains("CLOCK_SKEW") == true) Notice(stringResource(R.string.enterprise_sync_clock_warning))
    detail.attachments.forEachIndexed { index, attachment ->
        Card(Modifier.fillMaxWidth()) {
            Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(stringResource(R.string.enterprise_sync_attachment, index + 1), style = MaterialTheme.typography.titleMedium)
                Text(stringResource(R.string.enterprise_sync_upload_unconfirmed))
                Text(stringResource(when (attachment.localEvidence) {
                    EnterpriseAttachmentEvidence.NOT_INSPECTED -> R.string.enterprise_sync_file_not_inspected
                    EnterpriseAttachmentEvidence.MISSING -> R.string.enterprise_sync_file_missing
                    EnterpriseAttachmentEvidence.MATCHING -> R.string.enterprise_sync_file_matching
                    EnterpriseAttachmentEvidence.MISMATCH -> R.string.enterprise_sync_file_mismatch
                    EnterpriseAttachmentEvidence.UNAVAILABLE -> R.string.enterprise_sync_file_unavailable
                }))
            }
        }
    }
    val receipt = item.receipt
    if (receipt?.result == SyncResult.REJECTED && !item.dismissedRejected) {
        TextButton(onClick = { confirm = true }, enabled = enabled,
            modifier = Modifier.fillMaxWidth().testTag("enterprise-sync-dismiss")) { Text(stringResource(R.string.enterprise_sync_hide)) }
        if (confirm) AlertDialog(onDismissRequest = { confirm = false },
            title = { Text(stringResource(R.string.enterprise_sync_hide_title)) },
            text = { Text(stringResource(R.string.enterprise_sync_hide_copy)) },
            confirmButton = { TextButton(onClick = { confirm = false; onDismiss(item.identity, receipt.receiptDigest) }, enabled = enabled,
                modifier = Modifier.testTag("enterprise-sync-dismiss-confirm")) { Text(stringResource(R.string.enterprise_sync_hide_confirm)) } },
            dismissButton = { TextButton(onClick = { confirm = false }) { Text(stringResource(R.string.enterprise_sync_cancel)) } })
    }
}

@Composable
private fun Notice(message: String, modifier: Modifier = Modifier) {
    Text(message, modifier = modifier.semantics { liveRegion = LiveRegionMode.Polite })
}

@Composable
private fun formattedTime(instant: Instant, zone: String = "UTC"): String {
    val locale = LocalConfiguration.current.locales[0]
    val formatter = remember(locale, zone) { DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM).withLocale(locale).withZone(ZoneId.of(zone)) }
    return formatter.format(instant) + " (" + zone + ")"
}

private fun statusText(status: EnterpriseSyncStatus): Int = when (status) {
    EnterpriseSyncStatus.SAVED_ON_DEVICE -> R.string.enterprise_sync_saved
    EnterpriseSyncStatus.SENDING -> R.string.enterprise_sync_sending
    EnterpriseSyncStatus.SENT -> R.string.enterprise_sync_sent
    EnterpriseSyncStatus.ACCEPTED -> R.string.enterprise_sync_accepted
    EnterpriseSyncStatus.NEEDS_REVIEW -> R.string.enterprise_sync_review
    EnterpriseSyncStatus.REJECTED -> R.string.enterprise_sync_rejected
}

private fun reasonText(code: String): Int = when (code) {
    "ORPHAN_REFERENCE" -> R.string.enterprise_sync_reason_orphan
    "ACCESS_ENDED_BEFORE_RECEIPT" -> R.string.enterprise_sync_reason_access_ended
    "PERMISSION_CHANGED" -> R.string.enterprise_sync_reason_permission_changed
    "ASSET_UNAVAILABLE" -> R.string.enterprise_sync_reason_asset_unavailable
    "ASSET_BUSY" -> R.string.enterprise_sync_reason_asset_busy
    "TRIP_ALREADY_CLOSED" -> R.string.enterprise_sync_reason_trip_closed
    "PERIOD_LOCKED" -> R.string.enterprise_sync_reason_period_locked
    "STALE_REVISION" -> R.string.enterprise_sync_reason_revision
    "ORG_SUSPENDED_GRACE" -> R.string.enterprise_sync_reason_suspended_review
    "PLAN_CHANGED" -> R.string.enterprise_sync_reason_plan
    "OPERATION_ID_REUSED" -> R.string.enterprise_sync_reason_reused
    "MALFORMED" -> R.string.enterprise_sync_reason_malformed
    "DEVICE_REVOKED" -> R.string.enterprise_sync_reason_device
    "MEMBERSHIP_ENDED" -> R.string.enterprise_sync_reason_membership
    "INTAKE_WINDOW_CLOSED" -> R.string.enterprise_sync_reason_window
    "FORBIDDEN" -> R.string.enterprise_sync_reason_forbidden
    "ORG_SUSPENDED" -> R.string.enterprise_sync_reason_suspended
    else -> R.string.enterprise_sync_unavailable
}
