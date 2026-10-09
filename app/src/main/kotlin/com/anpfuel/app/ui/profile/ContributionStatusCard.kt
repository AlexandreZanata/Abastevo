package com.anpfuel.app.ui.profile

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.components.SkeletonLine
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.application.usecase.contribution.OwnedContributionStatus
import com.anpfuel.domain.contribution.ContributionState

/**
 * P21-T03 private owner status card.
 *
 * Lists only this device's retained commands with the frozen P21-T01
 * states. Cancelled commands (no status by contract) render an explicit
 * cancelled label. Cancel removes a command from dispatch; failed
 * commands otherwise retry automatically with bounded backoff.
 */
@Composable
fun ContributionStatusCard(
    state: ContributionStatusUiState,
    onRetry: () -> Unit,
    onCancel: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    Card(
        modifier = modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceVariant,
        ),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            when (state) {
                ContributionStatusUiState.Loading -> {
                    SkeletonGroup(modifier = Modifier.fillMaxWidth()) {
                        SkeletonLine()
                        SkeletonLine(width = 160.dp)
                    }
                }
                ContributionStatusUiState.Empty -> {
                    Text(
                        text = stringResource(R.string.profile_contributions_empty),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                ContributionStatusUiState.Error -> {
                    Text(
                        text = stringResource(R.string.profile_contributions_error),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                    Button(onClick = onRetry, modifier = Modifier.fillMaxWidth()) {
                        Text(text = stringResource(R.string.action_retry))
                    }
                }
                is ContributionStatusUiState.Content -> {
                    state.items.forEach { item ->
                        ContributionStatusRow(
                            item = item,
                            onCancel = { onCancel(item.commandId) },
                            modifier = Modifier.fillMaxWidth(),
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun ContributionStatusRow(
    item: OwnedContributionStatus,
    onCancel: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier = modifier,
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = item.commandId,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurface,
            )
            Text(
                text = stringResource(
                    R.string.profile_contributions_revision_attempts,
                    item.revision,
                    item.attempts,
                ),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = when (val status = item.state) {
                    null -> stringResource(R.string.profile_contributions_state_cancelled)
                    is ContributionState.Queued ->
                        if (status.retryable) {
                            stringResource(R.string.profile_contributions_state_retrying)
                        } else {
                            stringResource(R.string.profile_contributions_state_queued)
                        }
                    // Unreachable until the review feed exists; defensive
                    // fallback keeps the row visible instead of crashing.
                    ContributionState.Pending,
                    ContributionState.Accepted,
                    ContributionState.Disputed,
                    ContributionState.Rejected,
                    -> status.javaClass.simpleName
                },
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        if (item.state != null) {
            TextButton(onClick = onCancel) {
                Text(text = stringResource(R.string.profile_contributions_cancel))
            }
        }
    }
}

@Preview(showBackground = true)
@Composable
private fun ContributionStatusCardPreview() {
    AnpFuelTheme {
        ContributionStatusCard(
            state = ContributionStatusUiState.Content(
                items = listOf(
                    OwnedContributionStatus(
                        commandId = "cmd-1",
                        revision = 2,
                        attempts = 1,
                        state = ContributionState.Queued(retryable = true),
                    ),
                    OwnedContributionStatus(
                        commandId = "cmd-2",
                        revision = 1,
                        attempts = 0,
                        state = null,
                    ),
                ),
            ),
            onRetry = {},
            onCancel = {},
        )
    }
}
