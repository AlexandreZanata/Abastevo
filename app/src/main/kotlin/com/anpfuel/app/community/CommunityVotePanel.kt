package com.anpfuel.app.community

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R

/**
 * P10-T07 — Confirm/dispute panel (BUC-005, B-BR-006/011).
 *
 * The shown summary (exact amount/product/unit/full condition) stays
 * visible above both actions; confirming affirms exactly that, and a
 * changed price asks for its replacement observation id. Dispute
 * detail is private copy only. Flag OFF or no prepared target renders
 * nothing (caller keeps the ANP/community panels intact).
 */
@Composable
fun CommunityVotePanel(
    state: CommunityVoteUiState,
    onConfirm: () -> Unit,
    onDispute: (reasonWire: String, detail: String?, replacementObservationId: String?) -> Unit,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    when (state) {
        is CommunityVoteUiState.Disabled -> Unit
        is CommunityVoteUiState.Idle -> Unit
        is CommunityVoteUiState.Ready -> {
            VoteForm(
                summary = state.summary,
                onConfirm = onConfirm,
                onDispute = onDispute,
                modifier = modifier,
            )
        }
        is CommunityVoteUiState.Submitting -> {
            CircularProgressIndicator(modifier = modifier.padding(16.dp))
        }
        is CommunityVoteUiState.Confirmed -> {
            Card(modifier = modifier.fillMaxWidth()) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text = stringResource(R.string.community_vote_confirmed),
                        style = MaterialTheme.typography.titleMedium,
                    )
                    if (state.replayed) {
                        Text(
                            text = stringResource(R.string.community_vote_replayed),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
        }
        is CommunityVoteUiState.Disputed -> {
            Card(modifier = modifier.fillMaxWidth()) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text = stringResource(R.string.community_vote_disputed, state.state),
                        style = MaterialTheme.typography.titleMedium,
                    )
                }
            }
        }
        is CommunityVoteUiState.Rejected -> {
            Card(modifier = modifier.fillMaxWidth()) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text = stringResource(R.string.community_vote_rejected, state.kindLabel),
                        style = MaterialTheme.typography.titleMedium,
                    )
                    Text(
                        text = state.message,
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Button(onClick = onRetry, modifier = Modifier.fillMaxWidth()) {
                        Text(text = stringResource(R.string.action_retry))
                    }
                }
            }
        }
    }
}

@Composable
private fun VoteForm(
    summary: String,
    onConfirm: () -> Unit,
    onDispute: (reasonWire: String, detail: String?, replacementObservationId: String?) -> Unit,
    modifier: Modifier = Modifier,
) {
    var reasonWire by remember { mutableStateOf<String?>(null) }
    var detail by remember { mutableStateOf("") }
    var replacement by remember { mutableStateOf("") }
    Card(modifier = modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(
                text = stringResource(R.string.community_vote_title),
                style = MaterialTheme.typography.titleMedium,
            )
            Text(
                text = summary,
                style = MaterialTheme.typography.bodyMedium,
            )
            Button(onClick = onConfirm, modifier = Modifier.fillMaxWidth()) {
                Text(text = stringResource(R.string.community_vote_confirm))
            }
            Text(
                text = stringResource(R.string.community_vote_dispute_hint),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            CommunityVoteDisplay.disputeReasons().forEach { reason ->
                val selected = reason == reasonWire
                if (selected) {
                    Button(
                        onClick = { reasonWire = reason },
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = CommunityVoteDisplay.disputeReasonLabel(reason))
                    }
                } else {
                    OutlinedButton(
                        onClick = { reasonWire = reason },
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = CommunityVoteDisplay.disputeReasonLabel(reason))
                    }
                }
            }
            TextField(
                value = detail,
                onValueChange = { next ->
                    if (next.length <= CommunityVoteDisplay.MAX_DETAIL_CHARS) {
                        detail = next
                    }
                },
                label = { Text(text = stringResource(R.string.community_vote_detail_label)) },
                supportingText = { Text(text = stringResource(R.string.community_vote_private_note)) },
                modifier = Modifier.fillMaxWidth(),
            )
            val selectedReason = reasonWire
            if (selectedReason != null && CommunityVoteDisplay.needsReplacement(selectedReason)) {
                TextField(
                    value = replacement,
                    onValueChange = { next -> replacement = next },
                    label = { Text(text = stringResource(R.string.community_vote_replacement_label)) },
                    supportingText = { Text(text = stringResource(R.string.community_vote_replacement_hint)) },
                    modifier = Modifier.fillMaxWidth(),
                )
            }
            Button(
                onClick = {
                    val reason = selectedReason ?: return@Button
                    onDispute(
                        reason,
                        detail.takeIf { it.isNotBlank() },
                        replacement.takeIf {
                            it.isNotBlank() &&
                                CommunityVoteDisplay.needsReplacement(reason)
                        },
                    )
                },
                enabled = selectedReason != null,
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(text = stringResource(R.string.community_vote_dispute))
            }
        }
    }
}
