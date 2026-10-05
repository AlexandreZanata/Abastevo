package com.anpfuel.app.ui.stations

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
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
import com.anpfuel.app.community.FeedbackDisplay
import com.anpfuel.app.community.FeedbackViewModel
import com.anpfuel.app.community.FeedbackUiState
import com.anpfuel.domain.feedback.FeedbackTarget

/**
 * P36-T02 station discussion bound to a canonical UUID target.
 *
 * Ratings (1–5 personal stars), 280-scalar comments/one-level replies and
 * revision-bound valid/invalid votes address `stationId + fuelProductWire`
 * resolved by [FeedbackTarget] — never a legacy CNPJ row or a manual ID
 * textbox. Vote agreement renders independently of price confidence; no
 * trust bonus or moderator power is granted. Guests read; writes need a
 * signed account (`SignInRequired`, enforced again server-side). An
 * invalid target renders an honest note, never an invented thread.
 */
@Composable
fun StationFeedbackSection(
    stationId: String,
    fuelProductWire: String,
    accountId: String,
    modifier: Modifier = Modifier,
    viewModel: FeedbackViewModel = hiltViewModel(),
) {
    val target = remember(stationId, fuelProductWire, accountId) {
        runCatching { FeedbackTarget.create(stationId, fuelProductWire, accountId) }.getOrNull()
    }
    if (target == null) {
        Text(
            text = stringResource(R.string.server_stations_unavailable),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = modifier,
        )
        return
    }
    val state by viewModel.state.collectAsStateWithLifecycle()
    LaunchedEffect(target) {
        viewModel.prepare(target.stationId, target.fuelProductWire, target.accountId)
        viewModel.loadFirstPage()
        viewModel.loadStats()
    }

    StationFeedbackContent(
        state = state,
        isSignedIn = target.isSignedIn,
        onRate = viewModel::rate,
        onDeleteRating = viewModel::deleteRating,
        onComment = viewModel::submitComment,
        onVote = viewModel::submitVote,
        onReport = viewModel::report,
        onRetry = viewModel::retry,
        modifier = modifier,
    )
}

@Composable
internal fun StationFeedbackContent(
    state: FeedbackUiState,
    isSignedIn: Boolean,
    onRate: (Int) -> Unit,
    onDeleteRating: () -> Unit,
    onComment: (String) -> Unit,
    onVote: (String, String) -> Unit,
    onReport: (String, String) -> Unit,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var draft by remember { mutableStateOf("") }
    Card(
        modifier = modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
        ),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(
                text = stringResource(R.string.station_feedback_title),
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.semantics { heading() },
            )
            when (state) {
                is FeedbackUiState.Disabled -> Unit
                is FeedbackUiState.Idle, is FeedbackUiState.Submitting -> {
                    Text(
                        text = stringResource(R.string.community_pending_p04),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                }
                is FeedbackUiState.SignInRequired -> {
                    Text(
                        text = stringResource(R.string.station_feedback_guest),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                }
                is FeedbackUiState.StatsLoaded -> {
                    Text(
                        text = FeedbackDisplay.ratingLine(state.count, state.sum),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                }
                is FeedbackUiState.Loaded -> {
                    if (state.items.isEmpty()) {
                        Text(
                            text = stringResource(R.string.community_pending_p04),
                            style = MaterialTheme.typography.bodyMedium,
                        )
                    } else {
                        state.items.forEach { comment ->
                            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                                Text(
                                    text = comment.text,
                                    style = MaterialTheme.typography.bodyMedium,
                                )
                                if (isSignedIn) {
                                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                        TextButton(onClick = {
                                            onVote(
                                                comment.id,
                                                com.anpfuel.application.usecase.feedback.SubmitFeedbackUseCase.VOTE_VALID,
                                            )
                                        }) {
                                            Text(text = stringResource(R.string.station_feedback_vote_valid))
                                        }
                                        TextButton(onClick = {
                                            onVote(
                                                comment.id,
                                                com.anpfuel.application.usecase.feedback.SubmitFeedbackUseCase.VOTE_INVALID,
                                            )
                                        }) {
                                            Text(text = stringResource(R.string.station_feedback_vote_invalid))
                                        }
                                        TextButton(onClick = { onReport(comment.id, "inappropriate") }) {
                                            Text(text = stringResource(R.string.station_feedback_report))
                                        }
                                    }
                                }
                            }
                        }
                    }
                    if (state.stale) {
                        Text(
                            text = stringResource(R.string.server_station_cached),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.error,
                        )
                    }
                }
                is FeedbackUiState.Rated -> {
                    Text(
                        text = FeedbackDisplay.ratingLine(state.count, state.sum),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                    if (isSignedIn) {
                        TextButton(onClick = onDeleteRating) {
                            Text(text = stringResource(R.string.station_feedback_delete_rating))
                        }
                    }
                }
                is FeedbackUiState.Saved -> {
                    Text(
                        text = stringResource(R.string.community_vote_confirmed),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                }
                is FeedbackUiState.Voted -> {
                    Text(
                        text = stringResource(
                            R.string.station_feedback_votes,
                            state.valid,
                            state.invalid,
                        ),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                }
                is FeedbackUiState.Queued -> {
                    Text(
                        text = stringResource(R.string.station_feedback_queued),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                    TextButton(onClick = onRetry) {
                        Text(text = stringResource(R.string.station_feedback_retry))
                    }
                }
                is FeedbackUiState.Rejected -> {
                    Text(
                        text = "${state.kindLabel}: ${state.message}",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                    TextButton(onClick = onRetry) {
                        Text(text = stringResource(R.string.station_feedback_retry))
                    }
                }
                else -> Unit
            }
            if (isSignedIn) {
                Text(
                    text = stringResource(R.string.station_feedback_rate),
                    style = MaterialTheme.typography.labelLarge,
                )
                Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                    for (stars in 1..5) {
                        TextButton(onClick = { onRate(stars) }) {
                            Text(text = "$stars ★")
                        }
                    }
                }
                OutlinedTextField(
                    value = draft,
                    onValueChange = { draft = it },
                    modifier = Modifier.fillMaxWidth(),
                    placeholder = { Text(text = stringResource(R.string.station_feedback_comment_hint)) },
                    supportingText = {
                        Text(text = "${FeedbackDisplay.charsRemaining(draft)}")
                    },
                    singleLine = false,
                    maxLines = 4,
                )
                OutlinedButton(
                    onClick = {
                        onComment(draft)
                        draft = ""
                    },
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Text(text = stringResource(R.string.station_feedback_submit))
                }
            } else {
                Text(
                    text = stringResource(R.string.station_feedback_guest),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Text(
                text = stringResource(R.string.community_confidence_note),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}
