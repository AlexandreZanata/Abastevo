package com.anpfuel.app.ui.charging

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.OfflineBanner
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.domain.charging.*
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import com.anpfuel.domain.charging.ChargingCondition
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingReportReason
import com.anpfuel.domain.charging.ChargingVote
import com.anpfuel.domain.charging.CommunityChargingObservation
import com.anpfuel.domain.charging.OwnedChargingObservation
import com.anpfuel.domain.charging.PendingChargingOperation

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ChargingCommunityScreen(
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: ChargingCommunityViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { viewModel.refresh() }
    var discard by remember { mutableStateOf<String?>(null) }
    var reportTarget by remember { mutableStateOf<CommunityChargingObservation?>(null) }
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(text = stringResource(R.string.charging_community_title)) },
                onNavigateUp = onNavigateBack,
            )
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(padding).padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            if (state.offline) item { OfflineBanner() }
            state.notice?.let { notice ->
                item {
                    Card(
                        Modifier.fillMaxWidth(),
                        border = AnpContainerDefaults.borderStroke(MaterialTheme.colorScheme.surfaceContainerHighest),
                    ) {
                        Row(Modifier.fillMaxWidth().padding(12.dp), Arrangement.SpaceBetween) {
                            Text(
                                text = stringResource(
                                    when (notice) {
                                        "saved" -> R.string.charging_community_saved
                                        "withdrawn" -> R.string.charging_community_withdrawn
                                        "conflict" -> R.string.charging_community_conflict
                                        "refused" -> R.string.charging_community_refused
                                        "queue-full" -> R.string.map21_full
                                        "voted" -> R.string.charging_community_voted
                                        "reported" -> R.string.charging_community_reported
                                        "self" -> R.string.charging_community_self
                                        "sign-in" -> R.string.charging_community_sign_in
                                        else -> R.string.charging_community_offline
                                    },
                                ),
                                style = MaterialTheme.typography.bodySmall,
                            )
                            TextButton(onClick = viewModel::dismissNotice) {
                                Text(text = stringResource(R.string.action_back))
                            }
                        }
                    }
                }
            }
            item {
                Text(
                    text = stringResource(R.string.charging_community_share_title),
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.semantics { heading() },
                )
                Text(
                    text = stringResource(R.string.charging_community_share_copy),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            item { CommunityForm(state, viewModel) }
            if (state.pending.isNotEmpty()) {
                item {
                    Text(
                        text = stringResource(R.string.charging_community_pending, state.pending.size),
                        style = MaterialTheme.typography.titleSmall,
                        modifier = Modifier.semantics { heading() },
                    )
                }
                items(state.pending, key = { it.operationId }) { pending -> PendingRow(pending)
                    Row { TextButton(onClick = viewModel::retry) { Text(stringResource(R.string.map21_retry)) }; TextButton(onClick = { discard = pending.operationId }) { Text(stringResource(R.string.map21_discard)) } }
                }
            }
            if (state.mine.isNotEmpty()) {
                item {
                    Text(
                        text = stringResource(R.string.charging_community_mine),
                        style = MaterialTheme.typography.titleSmall,
                        modifier = Modifier.semantics { heading() },
                    )
                }
                items(state.mine, key = { "mine:${it.id}" }) { owned ->
                    OwnedRow(
                        owned = owned,
                        busy = state.saving,
                        onEdit = { viewModel.edit(owned) },
                        onWithdraw = { viewModel.withdraw(owned) },
                    )
                }
            }
            item {
                Text(
                    text = stringResource(R.string.charging_community_reports),
                    style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.semantics { heading() },
                )
            }
            if (state.loading) {
                item { Text(text = stringResource(R.string.charging_community_loading)) }
            } else if (state.history.isEmpty()) {
                item { Text(text = stringResource(R.string.charging_community_empty)) }
            }
            items(state.history, key = { "history:${it.id}" }) { observation ->
                HistoryRow(
                    observation = observation,
                    signedOut = state.signedOut,
                    onVote = { choice -> viewModel.vote(observation, choice) },
                    onReport = { reportTarget = observation },
                )
            }
        }
    }
    discard?.let { id -> AlertDialog(onDismissRequest = { discard = null },
        title = { Text(stringResource(R.string.map21_discard)) }, text = { Text(stringResource(R.string.map21_discard_warning)) },
        confirmButton = { TextButton(onClick = { viewModel.discard(id); discard = null }) { Text(stringResource(R.string.map21_discard)) } },
        dismissButton = { TextButton(onClick = { discard = null }) { Text(stringResource(R.string.action_back)) } }) }
    reportTarget?.let { target ->
        AlertDialog(
            onDismissRequest = { reportTarget = null },
            title = { Text(text = stringResource(R.string.charging_community_report_title)) },
            text = {
                Column {
                    ChargingReportReason.entries.forEach { reason ->
                        TextButton(
                            onClick = {
                                reportTarget = null
                                viewModel.report(target, reason)
                            },
                        ) {
                            Text(
                                text = stringResource(
                                    when (reason) {
                                        ChargingReportReason.INACCURATE -> R.string.charging_community_reason_inaccurate
                                        ChargingReportReason.SPAM -> R.string.charging_community_reason_spam
                                        ChargingReportReason.PRIVATE_DATA -> R.string.charging_community_reason_private
                                        ChargingReportReason.OTHER -> R.string.charging_community_reason_other
                                    },
                                ),
                            )
                        }
                    }
                }
            },
            confirmButton = {
                TextButton(onClick = { reportTarget = null }) {
                    Text(text = stringResource(R.string.action_back))
                }
            },
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
private fun CommunityForm(state: ChargingCommunityUiState, viewModel: ChargingCommunityViewModel) {
    Card(
        Modifier.fillMaxWidth(),
        border = AnpContainerDefaults.borderStroke(MaterialTheme.colorScheme.surfaceContainerHighest),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Text(
                text = stringResource(
                    if (state.form.editingId == null) R.string.charging_community_new else R.string.charging_community_edit_report,
                ),
                style = MaterialTheme.typography.titleSmall,
                modifier = Modifier.semantics { heading() },
            )
            Text(
                text = stringResource(R.string.charging_community_connector),
                style = MaterialTheme.typography.labelLarge,
            )
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                ChargingConnector.entries.forEach { connector ->
                    FilterChip(
                        selected = state.form.connector == connector,
                        onClick = { viewModel.setConnector(connector) },
                        label = { Text(text = connector.name) },
                    )
                }
            }
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                ChargingCondition.entries.forEach { condition ->
                    FilterChip(
                        selected = state.form.condition == condition,
                        onClick = { viewModel.setCondition(condition) },
                        label = {
                            Text(
                                text = stringResource(
                                    when (condition) {
                                        ChargingCondition.STANDARD -> R.string.charging_community_condition_standard
                                        ChargingCondition.OFF_PEAK -> R.string.charging_community_condition_off_peak
                                        ChargingCondition.PEAK -> R.string.charging_community_condition_peak
                                        ChargingCondition.MEMBER -> R.string.charging_community_condition_member
                                        ChargingCondition.UNKNOWN -> R.string.charging_community_condition_unknown
                                    },
                                ),
                            )
                        },
                    )
                }
            }
            Text(stringResource(R.string.map21_dated))
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(selected = state.form.event == null, onClick = { viewModel.setEvent(null) }, label = { Text(stringResource(R.string.map21_no_event)) })
                ChargingOperationalStatus.entries.forEach { event -> FilterChip(selected = state.form.event == event,
                    onClick = { viewModel.setEvent(event) }, label = { Text(eventLabel(event)) }) }
            }
            if (state.form.event != null) {
                OutlinedTextField(value = state.form.eventTime, onValueChange = viewModel::setEventTime,
                    label = { Text(stringResource(R.string.map21_time)) }, modifier = Modifier.fillMaxWidth())
                FlowRow {
                    ChargingTargetScope.entries.forEach { scope -> FilterChip(selected = state.form.scope == scope,
                        onClick = { viewModel.setScope(scope) }, label = { Text(scopeLabel(scope)) }) }
                }
                if (state.form.scope != ChargingTargetScope.SITE) {
                    Text(stringResource(R.string.map21_reference))
                    val eligible = state.groups.filter { state.form.scope != ChargingTargetScope.EQUIPMENT || it.reportedQuantity == 1L }
                    if (eligible.isEmpty()) Text(stringResource(R.string.map21_no_target))
                    eligible.forEachIndexed { index, group -> FilterChip(selected = state.form.groupId == group.id,
                        onClick = { viewModel.setGroup(group) }, label = { Text("${index + 1} · ${group.connector.name} · ${group.externalId}") }) }
                }
            }
            OutlinedTextField(
                value = state.form.price,
                onValueChange = viewModel::setPrice,
                label = { Text(text = stringResource(R.string.charging_community_price)) },
                supportingText = { Text(text = stringResource(R.string.charging_community_price_hint)) },
                singleLine = true,
                isError = state.formError == "price",
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = state.form.day,
                onValueChange = viewModel::setDay,
                label = { Text(text = stringResource(R.string.charging_community_day)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = state.form.note,
                onValueChange = viewModel::setNote,
                label = { Text(text = stringResource(R.string.charging_community_note)) },
                modifier = Modifier.fillMaxWidth(),
            )
            state.formError?.let {
                Text(
                    text = stringResource(R.string.charging_community_form_error),
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
                )
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = viewModel::save, enabled = !state.saving && !state.signedOut) {
                    Text(
                        text = stringResource(
                            if (state.form.editingId == null) R.string.charging_community_save else R.string.charging_community_update,
                        ),
                    )
                }
                if (state.form.editingId != null) {
                    OutlinedButton(onClick = viewModel::cancelEdit) {
                        Text(text = stringResource(R.string.action_back))
                    }
                }
            }
            if (state.signedOut) {
                Text(
                    text = stringResource(R.string.charging_community_sign_in),
                    style = MaterialTheme.typography.bodySmall,
                )
            }
        }
    }
}

@Composable
private fun PendingRow(pending: PendingChargingOperation) {
    Card(
        Modifier.fillMaxWidth(),
        border = AnpContainerDefaults.borderStroke(MaterialTheme.colorScheme.surfaceContainerHighest),
    ) {
        Text(
            text = stringResource(
                when (pending.kind.name) {
                    "WITHDRAW" -> R.string.charging_community_pending_withdraw
                    "CORRECT" -> R.string.charging_community_pending_correct
                    else -> R.string.charging_community_pending_submit
                },
            ) + (pending.lastError?.let { " · $it" } ?: ""),
            modifier = Modifier.padding(12.dp),
            style = MaterialTheme.typography.bodySmall,
        )
    }
}

@Composable
private fun OwnedRow(
    owned: OwnedChargingObservation,
    busy: Boolean,
    onEdit: () -> Unit,
    onWithdraw: () -> Unit,
) {
    Card(
        Modifier.fillMaxWidth(),
        border = AnpContainerDefaults.borderStroke(MaterialTheme.colorScheme.surfaceContainerHighest),
    ) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(text = "${owned.connector.name} · ${owned.condition.name} · ${owned.observedDay}")
            owned.priceMilliBrlPerKwh?.let {
                Text(text = stringResource(R.string.charging_community_price_value, java.math.BigDecimal(it).movePointLeft(3)))
            }
            owned.operational?.let { OperationalLabel(it) }
            if (owned.note.isNotBlank()) Text(text = owned.note)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                TextButton(onClick = onEdit, enabled = !busy) {
                    Text(text = stringResource(R.string.charging_community_edit))
                }
                TextButton(onClick = onWithdraw, enabled = !busy) {
                    Text(text = stringResource(R.string.charging_community_withdraw))
                }
            }
        }
    }
}

@Composable
private fun HistoryRow(
    observation: CommunityChargingObservation,
    signedOut: Boolean,
    onVote: (ChargingVote) -> Unit,
    onReport: () -> Unit,
) {
    Card(
        Modifier.fillMaxWidth(),
        border = AnpContainerDefaults.borderStroke(MaterialTheme.colorScheme.surfaceContainerHighest),
    ) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(text = "${observation.connector.name} · ${observation.condition.name} · ${observation.observedDay}")
            observation.priceMilliBrlPerKwh?.let {
                Text(text = stringResource(R.string.charging_community_price_value, java.math.BigDecimal(it).movePointLeft(3)))
            }
            observation.operational?.let { OperationalLabel(it) }
            if (observation.note.isNotBlank()) Text(text = observation.note)
            Text(
                text = stringResource(R.string.charging_community_tally, observation.helpful, observation.notHelpful),
                style = MaterialTheme.typography.bodySmall,
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                TextButton(onClick = { onVote(ChargingVote.HELPFUL) }, enabled = !signedOut) {
                    Text(text = stringResource(R.string.charging_community_helpful))
                }
                TextButton(onClick = { onVote(ChargingVote.NOT_HELPFUL) }, enabled = !signedOut) {
                    Text(text = stringResource(R.string.charging_community_not_helpful))
                }
                TextButton(onClick = onReport, enabled = !signedOut) {
                    Text(text = stringResource(R.string.charging_community_report))
                }
            }
            HorizontalDivider()
        }
    }
}

@Composable private fun eventLabel(event: ChargingOperationalStatus): String = stringResource(when(event) {
    ChargingOperationalStatus.WORKED -> R.string.map21_worked
    ChargingOperationalStatus.DID_NOT_WORK -> R.string.map21_did_not_work
    ChargingOperationalStatus.IN_USE -> R.string.map21_in_use
    ChargingOperationalStatus.ACCESS_RESTRICTED -> R.string.map21_restricted
})
@Composable private fun scopeLabel(scope: ChargingTargetScope): String = stringResource(when(scope) {
    ChargingTargetScope.SITE -> R.string.map21_site
    ChargingTargetScope.EQUIPMENT -> R.string.map21_equipment
    ChargingTargetScope.CONNECTOR -> R.string.map21_connector
})
@Composable private fun OperationalLabel(event: ChargingOperationalEvent) {
    Text("${eventLabel(event.event)} · ${scopeLabel(event.targetScope)} · ${event.observedAt}")
    Text(stringResource(R.string.map21_dated), style = MaterialTheme.typography.bodySmall)
}
