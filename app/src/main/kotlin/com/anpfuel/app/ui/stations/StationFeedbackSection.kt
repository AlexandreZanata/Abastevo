package com.anpfuel.app.ui.stations

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.wrapContentWidth
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.community.FeedbackDisplay
import com.anpfuel.app.community.FeedbackUiState

/**
 * P36-T02 station discussion bound to a canonical UUID target.
 *
 * Ratings (1–5 personal stars), 280-scalar comments/one-level replies and
 * revision-bound valid/invalid votes address `stationId + fuelProductWire`
 * resolved by [com.anpfuel.domain.feedback.FeedbackTarget] — never a legacy
 * CNPJ row or a manual ID textbox. Vote agreement renders independently of
 * price confidence; no trust bonus or moderator power is granted. Guests
 * read; writes need a signed account (`SignInRequired`, enforced again
 * server-side). An invalid target renders an honest note, never an
 * invented thread.
 *
 * Lazy host: comments render as keyed items in the caller's virtualized
 * list with stable `feedback-comment-<id>` identity, so continuations
 * never rebuild or reorder visible rows. Continuation failures keep
 * every displayed comment with a same-cursor retry.
 */
internal object FeedbackListKeys {
    const val TITLE = "feedback-title"
    const val STATE = "feedback-state"
    const val STALE_NOTE = "feedback-stale"
    const val MORE = "feedback-more"
    const val MORE_LOADING = "feedback-more-loading"
    const val MORE_ERROR = "feedback-more-error"
    const val COMPOSER = "feedback-composer"
    const val NOTE = "feedback-note"

    fun comment(id: String) = "feedback-comment-$id"
}

fun LazyListScope.stationFeedbackItems(
    state: FeedbackUiState,
    isSignedIn: Boolean,
    onRate: (Int) -> Unit,
    onDeleteRating: () -> Unit,
    onComment: (String) -> Unit,
    onVote: (String, String) -> Unit,
    onReport: (String, String) -> Unit,
    onRetry: () -> Unit,
    onLoadMore: () -> Unit,
) {
    item(key = FeedbackListKeys.TITLE) {
        Text(
            text = stringResource(R.string.station_feedback_title),
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.semantics { heading() },
        )
    }
    when (state) {
        is FeedbackUiState.Disabled -> Unit
        is FeedbackUiState.Idle, is FeedbackUiState.Submitting -> {
            item(key = FeedbackListKeys.STATE) {
                Text(
                    text = stringResource(R.string.community_pending_p04),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }
        is FeedbackUiState.SignInRequired -> {
            item(key = FeedbackListKeys.STATE) {
                Text(
                    text = stringResource(R.string.station_feedback_guest),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }
        is FeedbackUiState.StatsLoaded -> {
            item(key = FeedbackListKeys.STATE) {
                Text(
                    text = FeedbackDisplay.ratingLine(state.count, state.sum),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }
        is FeedbackUiState.Loaded -> {
            if (state.items.isEmpty()) {
                item(key = FeedbackListKeys.STATE) {
                    Text(
                        text = stringResource(R.string.community_pending_p04),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                }
            } else {
                items(state.items, key = { FeedbackListKeys.comment(it.id) }) { comment ->
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
            if (state.isLoadingMore) {
                item(key = FeedbackListKeys.MORE_LOADING) {
                    CircularProgressIndicator(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(vertical = 4.dp)
                            .wrapContentWidth(Alignment.CenterHorizontally)
                            .size(24.dp),
                        strokeWidth = 2.dp,
                    )
                }
            } else if (state.moreError != null) {
                item(key = FeedbackListKeys.MORE_ERROR) {
                    Text(
                        text = stringResource(R.string.station_feedback_show_more_failed),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
                item(key = "${FeedbackListKeys.MORE}-retry") {
                    TextButton(onClick = onLoadMore) {
                        Text(text = stringResource(R.string.station_feedback_retry))
                    }
                }
            } else if (state.nextCursor != null) {
                item(key = FeedbackListKeys.MORE) {
                    OutlinedButton(
                        onClick = onLoadMore,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = stringResource(R.string.station_feedback_show_more))
                    }
                }
            }
            if (state.stale) {
                item(key = FeedbackListKeys.STALE_NOTE) {
                    Text(
                        text = stringResource(R.string.server_station_cached),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
        }
        is FeedbackUiState.Rated -> {
            item(key = FeedbackListKeys.STATE) {
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
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
            }
        }
        is FeedbackUiState.Saved -> {
            item(key = FeedbackListKeys.STATE) {
                Text(
                    text = stringResource(R.string.community_vote_confirmed),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }
        is FeedbackUiState.Voted -> {
            item(key = FeedbackListKeys.STATE) {
                Text(
                    text = stringResource(
                        R.string.station_feedback_votes,
                        state.valid,
                        state.invalid,
                    ),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }
        is FeedbackUiState.Queued -> {
            item(key = FeedbackListKeys.STATE) {
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text(
                        text = stringResource(R.string.station_feedback_queued),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                    TextButton(onClick = onRetry) {
                        Text(text = stringResource(R.string.station_feedback_retry))
                    }
                }
            }
        }
        is FeedbackUiState.Rejected -> {
            item(key = FeedbackListKeys.STATE) {
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text(
                        text = "${state.kindLabel}: ${state.message}",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                    TextButton(onClick = onRetry) {
                        Text(text = stringResource(R.string.station_feedback_retry))
                    }
                }
            }
        }
        else -> Unit
    }
    if (isSignedIn) {
        item(key = FeedbackListKeys.COMPOSER) {
            var draft by remember { mutableStateOf("") }
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
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
            }
        }
    } else {
        item(key = FeedbackListKeys.COMPOSER) {
            Text(
                text = stringResource(R.string.station_feedback_guest),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
    item(key = FeedbackListKeys.NOTE) {
        Text(
            text = stringResource(R.string.community_confidence_note),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}
