package com.anpfuel.app.ui.profile

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.AssignmentTurnedIn
import androidx.compose.material.icons.filled.CloudUpload
import androidx.compose.material.icons.filled.History
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.FuelProductIcon
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.components.SkeletonLine
import com.anpfuel.application.usecase.contribution.OwnedContributionStatus
import com.anpfuel.domain.contribution.ContributionState

/**
 * Dedicated "Minhas contribuições" page.
 *
 * Professional private owner ledger: header summary (total / queued /
 * cancelled), then one card per retained outbox command with its frozen
 * presentation state. Cancel removes a command from dispatch; failed
 * commands retry automatically with bounded backoff. Nothing here is
 * public and no precise location ever leaves the device in this view.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MyContributionsScreen(
    onNavigateBack: () -> Unit,
    onContribute: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: ContributionStatusViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    LaunchedEffect(viewModel) { viewModel.load() }
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(stringResource(R.string.profile_contributions_section)) },
                onNavigateUp = onNavigateBack,
            )
        },
    ) { padding ->
        val uiState = state
        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(padding),
            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            item(key = "hero") { ContributionsHero(uiState, onContribute, Modifier.fillMaxWidth()) }
            when (uiState) {
                ContributionStatusUiState.Loading -> item(key = "loading") {
                    SkeletonGroup(Modifier.fillMaxWidth()) { SkeletonLine(); SkeletonLine(width = 160.dp) }
                }
                ContributionStatusUiState.Empty -> item(key = "empty") {
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Text(stringResource(R.string.profile_contributions_empty))
                        OutlinedButton(onClick = onContribute, modifier = Modifier.fillMaxWidth()) {
                            Text(stringResource(R.string.community_action_contribute))
                        }
                    }
                }
                ContributionStatusUiState.Error -> item(key = "error") {
                    Text(stringResource(R.string.profile_contributions_error))
                    Button(onClick = viewModel::load, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.action_retry)) }
                }
                is ContributionStatusUiState.Content -> {
                    items(uiState.items, key = { "contribution:${it.commandId}" }) { item ->
                        ContributionLedgerRow(item, onCancel = { viewModel.onCancel(item.commandId) }, modifier = Modifier.fillMaxWidth())
                    }
                    if (uiState.hasMore) item(key = "more") {
                        OutlinedButton(onClick = viewModel::loadMore, enabled = !uiState.loadingMore, modifier = Modifier.fillMaxWidth()) {
                            Text(stringResource(if (uiState.moreFailed) R.string.action_retry else R.string.feed_load_more))
                        }
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun ContributionsHero(
    state: ContributionStatusUiState,
    onContribute: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Card(
        modifier,
        shape = RoundedCornerShape(24.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.primaryContainer),
    ) {
        Column(Modifier.fillMaxWidth().padding(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Icon(Icons.Default.AssignmentTurnedIn, null,
                    tint = MaterialTheme.colorScheme.onPrimaryContainer, modifier = Modifier.size(32.dp))
                Column(Modifier.weight(1f)) {
                    Text(stringResource(R.string.profile_contributions_section),
                        style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold,
                        color = MaterialTheme.colorScheme.onPrimaryContainer,
                        modifier = Modifier.semantics { heading() })
                    Text(stringResource(R.string.contributions_screen_subtitle),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onPrimaryContainer)
                }
            }
            if (state is ContributionStatusUiState.Content) {
                val queued = state.items.count { it.state is ContributionState.Queued }
                val cancelled = state.items.count { it.state == null }
                FlowRow(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    AssistChip(onClick = {}, label = {
                        Text(
                            stringResource(R.string.contributions_summary_total, state.items.size),
                            maxLines = 1,
                            softWrap = false,
                        )
                    }, leadingIcon = { Icon(Icons.Default.History, null, Modifier.size(16.dp)) })
                    AssistChip(onClick = {}, label = {
                        Text(
                            stringResource(R.string.contributions_summary_queued, queued),
                            maxLines = 1,
                            softWrap = false,
                        )
                    }, leadingIcon = { Icon(Icons.Default.CloudUpload, null, Modifier.size(16.dp)) })
                    if (cancelled > 0) AssistChip(onClick = {}, label = {
                        Text(
                            stringResource(R.string.contributions_summary_cancelled, cancelled),
                            maxLines = 1,
                            softWrap = false,
                        )
                    })
                }
            }
            Button(onClick = onContribute, modifier = Modifier.fillMaxWidth()) {
                Icon(Icons.Default.Add, null, Modifier.size(20.dp))
                Spacer(Modifier.size(8.dp))
                Text(stringResource(R.string.community_action_contribute))
            }
        }
    }
}

@Composable
private fun ContributionLedgerRow(
    item: OwnedContributionStatus,
    onCancel: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Card(
        modifier,
        shape = RoundedCornerShape(20.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                val fuel = item.fuelProduct
                if (fuel != null) {
                    FuelProductIcon(
                        product = fuel,
                        size = 40.dp,
                        contentDescription = null,
                    )
                }
                Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Text(
                        if (fuel != null) {
                            stringResource(FuelProductI18n.toStringRes(fuel))
                        } else {
                            shortCommandId(item.commandId)
                        },
                        style = MaterialTheme.typography.titleSmall,
                        fontWeight = FontWeight.Bold,
                    )
                    Text(stringResource(R.string.profile_contributions_revision_attempts, item.revision, item.attempts),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                AssistChip(onClick = {}, label = { Text(statusLabel(item)) })
            }
            val reason = item.reason
            if (reason != null) {
                Text(reason, style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            if (item.state != null) {
                TextButton(onClick = onCancel) {
                    Text(stringResource(R.string.profile_contributions_cancel))
                }
            }
        }
    }
}

private fun shortCommandId(commandId: String): String =
    if (commandId.length <= 12) commandId else "…${commandId.takeLast(8)}"

@Composable
private fun statusLabel(item: OwnedContributionStatus): String =
    when (val status = item.state) {
        null -> stringResource(R.string.profile_contributions_state_cancelled)
        is ContributionState.Queued -> if (status.retryable) {
            stringResource(R.string.profile_contributions_state_retrying)
        } else {
            stringResource(R.string.profile_contributions_state_queued)
        }
        ContributionState.Pending,
        ContributionState.Accepted,
        ContributionState.Disputed,
        ContributionState.Rejected,
        -> status.javaClass.simpleName
    }
