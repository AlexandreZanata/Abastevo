package com.anpfuel.app.community

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpContainerDefaults

/**
 * CS08B — Helpful/unhelpful panel (B-BR-CS01, BUC-CS02).
 *
 * Counts render from exact tallies with the owner's selected choice
 * and the frozen review notice; reporting reuses the host dispute flow
 * through [onReport] (this panel never reports by itself). Flag OFF or
 * no prepared target renders nothing (caller keeps surrounding panels
 * intact). Guests see the free-login explanation with [onSignIn].
 * Single-flight submitting shows progress with no second tap target.
 */
@Composable
fun CommunityHelpfulnessPanel(    state: CommunityHelpfulnessUiState,
    onVote: (choiceWire: String) -> Unit,
    onRemove: () -> Unit,
    onRetry: () -> Unit,
    onReport: () -> Unit,
    onSignIn: () -> Unit,
    modifier: Modifier = Modifier,
) {
    when (state) {
        is CommunityHelpfulnessUiState.Disabled -> Unit
        is CommunityHelpfulnessUiState.Idle -> Unit
        is CommunityHelpfulnessUiState.Loading -> {
            CircularProgressIndicator(modifier = modifier.padding(16.dp))
        }
        is CommunityHelpfulnessUiState.Guest -> {
            Card(
                modifier = modifier.fillMaxWidth(),
                border = AnpContainerDefaults.borderStroke(MaterialTheme.colorScheme.surfaceContainerHighest),
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text = stringResource(R.string.community_helpful_guest),
                        style = MaterialTheme.typography.titleMedium,
                        modifier = Modifier.semantics { heading() },
                    )
                    Text(
                        text = stringResource(R.string.community_helpful_guest_body),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Button(onClick = onSignIn, modifier = Modifier.fillMaxWidth()) {
                        Text(text = stringResource(R.string.station_claim_login))
                    }
                }
            }
        }
        is CommunityHelpfulnessUiState.Content -> {
            Card(
                modifier = modifier.fillMaxWidth(),
                border = AnpContainerDefaults.borderStroke(MaterialTheme.colorScheme.surfaceContainerHighest),
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text = stringResource(R.string.community_helpful_title),
                        style = MaterialTheme.typography.titleMedium,
                        modifier = Modifier.semantics { heading() },
                    )
                    val counts = stringResource(
                        R.string.community_helpful_counts,
                        state.helpful,
                        state.notHelpful,
                    )
                    Text(
                        text = counts,
                        style = MaterialTheme.typography.bodyLarge,
                        modifier = Modifier.semantics {
                            contentDescription = helpfulnessTalkBackDescription(
                                counts,
                                state.myChoice,
                                state.underReview,
                            )
                        },
                    )
                    if (state.myChoice != null) {
                        Text(
                            text = stringResource(R.string.community_helpful_choice, state.myChoice),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    if (state.underReview) {
                        Text(
                            text = stringResource(R.string.community_helpful_under_review),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        Button(
                            onClick = { onVote("HELPFUL") },
                            modifier = Modifier.weight(1f),
                        ) {
                            Text(text = stringResource(R.string.community_helpful_helpful))
                        }
                        OutlinedButton(
                            onClick = { onVote("NOT_HELPFUL") },
                            modifier = Modifier.weight(1f),
                        ) {
                            Text(text = stringResource(R.string.community_helpful_not_helpful))
                        }
                    }
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        if (state.myChoice != null) {
                            TextButton(onClick = onRemove) {
                                Text(text = stringResource(R.string.community_helpful_remove))
                            }
                        }
                        TextButton(onClick = onReport) {
                            Text(text = stringResource(R.string.community_helpful_report))
                        }
                    }
                }
            }
        }
        is CommunityHelpfulnessUiState.Submitting -> {
            CircularProgressIndicator(modifier = modifier.padding(16.dp))
        }
        is CommunityHelpfulnessUiState.Rejected -> {
            Card(
                modifier = modifier.fillMaxWidth(),
                border = AnpContainerDefaults.borderStroke(MaterialTheme.colorScheme.surfaceContainerHighest),
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text = stringResource(R.string.community_helpful_rejected, state.kind.name),
                        style = MaterialTheme.typography.titleMedium,
                        modifier = Modifier.semantics { heading() },
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

/**
 * CS08B TalkBack description: counts plus the owner's choice and the
 * frozen review notice, never bare numbers alone. Pure for unit tests;
 * large text scales through MaterialTheme typography with no fixed
 * heights in the panel above.
 */
internal fun helpfulnessTalkBackDescription(
    counts: String,
    myChoice: String?,
    underReview: Boolean,
): String {
    val parts = mutableListOf(counts)
    if (!myChoice.isNullOrBlank()) {
        parts += "Your choice: $myChoice"
    }
    if (underReview) {
        parts += "Counts under community review"
    }
    return parts.joinToString(", ")
}
