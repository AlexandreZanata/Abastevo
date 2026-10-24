package com.anpfuel.app.ui.sync

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.contentDescription
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.application.usecase.ledger.LedgerConflictChoice
import com.anpfuel.domain.premium.LedgerDataset
import java.text.DateFormat
import java.util.Date

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LedgerSyncScreen(onNavigateBack: () -> Unit, viewModel: LedgerSyncViewModel = hiltViewModel()) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    DisposableEffect(lifecycle, viewModel) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) viewModel.refresh()
        }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer) }
    }
    LaunchedEffect(viewModel) { viewModel.refresh() }
    AnpScaffold(topBar = {
        AnpTopAppBar(title = { Text(stringResource(R.string.ledger_sync_title)) }, onNavigateUp = onNavigateBack)
    }) { padding ->
        LazyColumn(modifier = Modifier.fillMaxSize().padding(padding), contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)) {
            item { Text(stringResource(R.string.ledger_sync_explanation)) }
            if (state.busy) item { LinearProgressIndicator(modifier = Modifier.fillMaxWidth()) }
            state.notice?.let { notice -> item {
                Text(stringResource(when (notice) {
                    SyncNotice.OFFLINE -> R.string.ledger_sync_offline
                    SyncNotice.ERROR -> R.string.ledger_sync_error
                    SyncNotice.STALE -> R.string.ledger_sync_stale
                    SyncNotice.PARTIAL -> R.string.ledger_sync_partial
                    SyncNotice.RESTORED -> R.string.ledger_sync_restored
                    SyncNotice.RESOLVED -> R.string.ledger_sync_resolved
                }), color = MaterialTheme.colorScheme.primary)
            } }
            val status = state.status
            if (status != null && status.accountId == null) item { Text(stringResource(R.string.ledger_sync_sign_in)) }
            status?.let {
                item { SyncToggle(stringResource(R.string.ledger_sync_master), it.masterEnabled, !state.busy && (it.accountId != null || it.masterEnabled), viewModel::setMasterEnabled) }
                item { SyncToggle(stringResource(R.string.ledger_sync_device), it.deviceEnabled, !state.busy, viewModel::setDeviceEnabled) }
                items(it.datasets, key = { row -> row.dataset.wire }) { row ->
                    Card(modifier = Modifier.fillMaxWidth()) {
                        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            SyncToggle(datasetName(row.dataset), row.enabled, !state.busy) { value -> viewModel.setDatasetEnabled(row.dataset, value) }
                            Text(stringResource(R.string.ledger_sync_counts, row.pending, row.conflicts))
                            Text(row.lastAckAtMillis?.let { at ->
                                stringResource(R.string.ledger_sync_last_ack, row.lastAckRevision ?: 0,
                                    DateFormat.getDateTimeInstance().format(Date(at)))
                            } ?: stringResource(R.string.ledger_sync_no_ack))
                        }
                    }
                }
                if (it.conflictRecords.isEmpty()) item { Text(stringResource(R.string.ledger_sync_no_conflicts)) }
                items(it.conflictRecords, key = { row -> row.dataset.wire + row.recordId }) { record ->
                    Card(modifier = Modifier.fillMaxWidth()) {
                        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text(stringResource(R.string.ledger_sync_conflict, datasetName(record.dataset), record.recordId))
                            val enabled = !state.busy && it.masterEnabled && it.deviceEnabled && it.datasets.any { row -> row.dataset == record.dataset && row.enabled }
                            Button(onClick = { viewModel.resolveConflict(record, LedgerConflictChoice.KEEP_MINE) }, enabled = enabled,
                                modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.ledger_sync_keep_mine)) }
                            OutlinedButton(onClick = { viewModel.resolveConflict(record, LedgerConflictChoice.TAKE_SERVER) }, enabled = enabled,
                                modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.ledger_sync_take_server)) }
                            TextButton(onClick = { viewModel.resolveConflict(record, LedgerConflictChoice.CANCEL) }, enabled = !state.busy) { Text(stringResource(R.string.action_cancel)) }
                        }
                    }
                }
                item {
                    OutlinedButton(onClick = viewModel::previewRestore, enabled = !state.busy && it.accountId != null && it.masterEnabled && it.deviceEnabled,
                        modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.ledger_sync_preview)) }
                }
            }
            state.staged?.let { diff ->
                item {
                    Card(modifier = Modifier.fillMaxWidth()) {
                        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text(stringResource(R.string.ledger_sync_diff, diff.toPull.size, diff.localOnlyUnsynced, diff.skippedErased, diff.skippedVoided))
                            if (diff.truncated) Text(stringResource(R.string.ledger_sync_truncated))
                            if (diff.toPull.isEmpty()) Text(stringResource(R.string.ledger_sync_empty))
                        }
                    }
                }
                items(diff.toPull, key = { row -> "restore|${row.dataset.wire}|${row.recordId}" }) { row ->
                    Text(stringResource(R.string.ledger_sync_pull_item, datasetName(row.dataset), row.recordId, row.serverRevision))
                }
                item {
                    Column {
                        Button(onClick = viewModel::confirmRestore, enabled = !state.busy && !diff.truncated && diff.toPull.isNotEmpty(),
                            modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.ledger_sync_confirm)) }
                        TextButton(onClick = viewModel::cancelRestore, enabled = !state.busy) { Text(stringResource(R.string.action_cancel)) }
                    }
                }
            }
            item { TextButton(onClick = viewModel::refresh, enabled = !state.busy) { Text(stringResource(R.string.action_retry)) } }
        }
    }
}

@Composable
private fun SyncToggle(label: String, checked: Boolean, enabled: Boolean, onChanged: (Boolean) -> Unit) {
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(label, modifier = Modifier.weight(1f))
        Switch(checked = checked, onCheckedChange = onChanged, enabled = enabled,
            modifier = Modifier.semantics { contentDescription = label })
    }
}

@Composable
private fun datasetName(dataset: LedgerDataset): String = stringResource(when (dataset) {
    LedgerDataset.REFUEL -> R.string.ledger_sync_refuel
    LedgerDataset.EXPENSE -> R.string.ledger_sync_expense
    LedgerDataset.CHARGE -> R.string.ledger_sync_charge
})
