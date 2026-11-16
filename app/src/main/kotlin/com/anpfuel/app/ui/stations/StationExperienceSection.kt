package com.anpfuel.app.ui.stations

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.outlined.StarBorder
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.community.CommunitySafetyError
import com.anpfuel.app.community.CommunitySafetyNotice
import com.anpfuel.app.community.CommunitySafetyViewModel
import com.anpfuel.app.community.FeedbackDisplay
import com.anpfuel.app.community.FeedbackUiState
import com.anpfuel.app.community.FeedbackViewModel
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.designsystem.FilterChip
import com.anpfuel.app.ui.designsystem.OutlinedButton
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.safety.BlockUserDialog
import com.anpfuel.app.ui.safety.ReportUserDialog
import com.anpfuel.application.usecase.feedback.SubmitFeedbackUseCase
import com.anpfuel.domain.portable.PortableText
import com.anpfuel.domain.repository.FeedbackCommentView
import com.anpfuel.domain.safety.SafetyContentKind
import com.anpfuel.domain.safety.SafetyContentRef

/** Separate retained VMs for ratings and discussion: loading totals never erases comments. */
@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun StationExperienceSection(
    stationId: String,
    fuelWire: String,
    accountId: String,
    onSignIn: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val scopeKey = "$stationId:$fuelWire:$accountId"
    val ratings: FeedbackViewModel = hiltViewModel(key = "rating:$scopeKey")
    val comments: FeedbackViewModel = hiltViewModel(key = "comments:$scopeKey")
    // Report-user, block-user and hidden-comment filtering (BUC-SAFE-02/03, B-BR-SAFE-06).
    val safety: CommunitySafetyViewModel = hiltViewModel(key = "safety:$scopeKey")
    val ratingState by ratings.state.collectAsStateWithLifecycle()
    val commentState by comments.state.collectAsStateWithLifecycle()
    val safetyState by safety.state.collectAsStateWithLifecycle()
    var stars by remember(scopeKey) { mutableIntStateOf(0) }
    var draft by remember(scopeKey) { mutableStateOf("") }
    var ratingAttempt by remember(scopeKey) { mutableStateOf(false) }
    var commentAttempt by remember(scopeKey) { mutableStateOf(false) }
    var notice by remember(scopeKey) { mutableStateOf<Int?>(null) }
    var reportTarget by remember(scopeKey) { mutableStateOf<FeedbackCommentView?>(null) }
    var reason by remember(scopeKey) { mutableStateOf("spam") }
    val signedIn = accountId.isNotBlank()
    val busy = commentState is FeedbackUiState.Submitting || ratingState is FeedbackUiState.Submitting
    LaunchedEffect(scopeKey) {
        ratings.prepare(stationId, fuelWire, accountId); ratings.loadStats()
        comments.prepare(stationId, fuelWire, accountId); comments.loadFirstPage()
    }
    LaunchedEffect(commentState) {
        when (commentState) {
            is FeedbackUiState.Saved -> { commentAttempt = false; draft = ""; notice = R.string.station_page_write_received; comments.loadFirstPage() }
            is FeedbackUiState.Reported -> { commentAttempt = false; notice = R.string.station_page_report_received; comments.loadFirstPage() }
            is FeedbackUiState.Voted -> { commentAttempt = false; notice = R.string.station_page_write_received; comments.loadFirstPage() }
            else -> Unit
        }
    }
    LaunchedEffect(commentState) {
        (commentState as? FeedbackUiState.Loaded)?.let { loaded -> safety.show(loaded.items.map { it.safetyRef() }) }
    }
    LaunchedEffect(safetyState.requireSignIn) {
        if (safetyState.requireSignIn) {
            safety.consumeSignIn()
            onSignIn()
        }
    }
    LaunchedEffect(ratingState) {
        if (ratingState is FeedbackUiState.RatingDeleted) {
            ratingAttempt = false; stars = 0; notice = R.string.station_page_write_received
            ratings.loadStats()
        }
    }
    if (ratingState is FeedbackUiState.Disabled && commentState is FeedbackUiState.Disabled) {
        StationSectionCard(stringResource(R.string.station_page_experience_title), modifier) {
            Text(stringResource(R.string.station_page_participation_pending))
        }
        return
    }
    StationSectionCard(stringResource(R.string.station_page_experience_title), modifier) {
        Text(stringResource(R.string.station_page_experience_note), style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant)
        val stats = when (val state = ratingState) {
            is FeedbackUiState.StatsLoaded -> state.count to state.sum
            is FeedbackUiState.Rated -> state.count to state.sum
            else -> null
        }
        if (stats != null) {
            if (stats.first == 0L) Text(stringResource(R.string.station_page_rating_empty))
            else {
                val tenth = 10L * stats.second / stats.first
                val mean = "${tenth / 10L}.${tenth % 10L}"
                Text(stringResource(R.string.station_page_rating_summary, mean, stats.first),
                    style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
            }
        }
        if (ratingState is FeedbackUiState.Submitting) LinearProgressIndicator(Modifier.fillMaxWidth())
        if (ratingState is FeedbackUiState.Rejected || ratingState is FeedbackUiState.Queued) {
            Text(stringResource(if (ratingState is FeedbackUiState.Queued) R.string.station_page_feedback_queued else R.string.station_page_feedback_failed))
            TextButton(onClick = { if (ratingAttempt) ratings.retry() else ratings.loadStats() }, enabled = !busy) { Text(stringResource(R.string.action_retry)) }
        }
        if (ratingState is FeedbackUiState.SignInRequired) {
            OutlinedButton(onClick = onSignIn) { Text(stringResource(R.string.auth_title)) }
        }
        if (signedIn && ratingState !is FeedbackUiState.SignInRequired && ratingState !is FeedbackUiState.Disabled) {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceEvenly) {
                for (value in 1..5) {
                    IconButton(onClick = { stars = value }, enabled = !busy) {
                        Icon(if (value <= stars) Icons.Filled.Star else Icons.Outlined.StarBorder,
                            contentDescription = "$value ★", tint = MaterialTheme.colorScheme.primary)
                    }
                }
            }
            Button(onClick = { ratingAttempt = true; ratings.rate(stars) }, enabled = stars in 1..5 && !busy,
                modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.station_page_rating_save)) }
            if (ratingState is FeedbackUiState.Rated) {
                Text(stringResource(R.string.station_page_write_received), style = MaterialTheme.typography.bodySmall)
                TextButton(onClick = { ratingAttempt = true; ratings.deleteRating() }, enabled = !busy) {
                    Text(stringResource(R.string.station_feedback_delete_rating))
                }
            }
        } else {
            Text(stringResource(R.string.station_feedback_guest))
            OutlinedButton(onClick = onSignIn, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.auth_title)) }
        }
        HorizontalDivider()
        notice?.let { Text(stringResource(it), color = MaterialTheme.colorScheme.primary) }
        safetyState.notice?.let { Text(stringResource(safetyNoticeText(it)), color = MaterialTheme.colorScheme.primary) }
        safetyState.error?.let { error ->
            val text = when (error) {
                is CommunitySafetyError.RateLimited ->
                    stringResource(R.string.safety_error_rate_limited, (error.retryAfterSeconds ?: 30L).toInt())
                else -> stringResource(safetyErrorText(error))
            }
            Text(text, color = MaterialTheme.colorScheme.error)
        }
        when (val state = commentState) {
            is FeedbackUiState.Loaded -> {
                // Comments by blocked authors never show; the rest show once checked (B-BR-SAFE-06).
                val listed = state.items.filter { it.stationId == stationId && it.product == fuelWire && it.parentId.isEmpty() }
                val visible = listed.filter { safetyState.isVisible(it.safetyRef()) }
                val checking = listed.any { it.safetyRef() !in safetyState.checked }
                if (state.items.isEmpty() || (listed.isNotEmpty() && visible.isEmpty() && !checking)) {
                    Text(stringResource(R.string.station_page_feedback_empty))
                }
                visible.forEach { comment ->
                    var menu by remember(comment.id) { mutableStateOf(false) }
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                            Text(comment.alias, style = MaterialTheme.typography.labelLarge, modifier = Modifier.weight(1f))
                            Box {
                                IconButton(onClick = { menu = true }, enabled = !busy, modifier = Modifier.size(48.dp)) {
                                    Icon(Icons.Default.MoreVert, contentDescription = stringResource(R.string.safety_more_actions))
                                }
                                DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                                    DropdownMenuItem(text = { Text(stringResource(R.string.station_page_report_title)) }, onClick = {
                                        menu = false
                                        if (signedIn) { reason = "spam"; reportTarget = comment } else onSignIn()
                                    })
                                    // Ownership is not known here: the server refuses own
                                    // comments with safety.self-target (B-BR-SAFE-02).
                                    DropdownMenuItem(text = { Text(stringResource(R.string.safety_report_user)) }, onClick = {
                                        menu = false
                                        if (signedIn) safety.openReportUser(comment.safetyRef()) else onSignIn()
                                    })
                                    DropdownMenuItem(text = { Text(stringResource(R.string.safety_block_user)) }, onClick = {
                                        menu = false
                                        if (signedIn) safety.openBlockUser(comment.safetyRef()) else onSignIn()
                                    })
                                }
                            }
                        }
                        Text(comment.text, style = MaterialTheme.typography.bodyMedium)
                        if (signedIn) FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            TextButton(onClick = { commentAttempt = true; comments.submitVote(comment.id, SubmitFeedbackUseCase.VOTE_VALID) }, enabled = !busy) {
                                Text(stringResource(R.string.station_feedback_vote_valid))
                            }
                            TextButton(onClick = { commentAttempt = true; comments.submitVote(comment.id, SubmitFeedbackUseCase.VOTE_INVALID) }, enabled = !busy) {
                                Text(stringResource(R.string.station_feedback_vote_invalid))
                            }
                        }
                        HorizontalDivider()
                    }
                }
                if (state.stale) Text(stringResource(R.string.server_station_cached), color = MaterialTheme.colorScheme.error)
            }
            is FeedbackUiState.Submitting -> { LinearProgressIndicator(Modifier.fillMaxWidth()); Text(stringResource(R.string.station_page_feedback_loading)) }
            is FeedbackUiState.Queued, is FeedbackUiState.Rejected -> {
                Text(stringResource(if (state is FeedbackUiState.Queued) R.string.station_page_feedback_queued else R.string.station_page_feedback_failed))
                TextButton(onClick = {
                    if (commentAttempt) comments.retry() else comments.loadFirstPage()
                }) { Text(stringResource(R.string.action_retry)) }
            }
            is FeedbackUiState.SignInRequired -> OutlinedButton(onClick = onSignIn) { Text(stringResource(R.string.auth_title)) }
            is FeedbackUiState.Disabled -> Text(stringResource(R.string.station_page_feedback_failed))
            else -> Unit
        }
        if (signedIn && commentState !is FeedbackUiState.SignInRequired && commentState !is FeedbackUiState.Disabled) {
            OutlinedTextField(value = draft, onValueChange = { draft = it }, modifier = Modifier.fillMaxWidth(),
                label = { Text(stringResource(R.string.station_feedback_comment_hint)) },
                supportingText = { Text("${FeedbackDisplay.charsRemaining(draft)} / 280") },
                isError = FeedbackDisplay.charsRemaining(draft) < 0, enabled = !busy, minLines = 2, maxLines = 5)
            Button(onClick = { notice = null; commentAttempt = true; comments.submitComment(draft) },
                enabled = !busy && PortableText.normalize(draft).isNotEmpty() && FeedbackDisplay.charsRemaining(draft) >= 0,
                modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.station_feedback_submit)) }
        }
    }
    reportTarget?.let { comment ->
        AlertDialog(onDismissRequest = { reportTarget = null }, title = { Text(stringResource(R.string.station_page_report_title)) },
            text = {
                Column(Modifier.verticalScroll(androidx.compose.foundation.rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Text(comment.text, maxLines = 3, overflow = androidx.compose.ui.text.style.TextOverflow.Ellipsis)
                    Text(stringResource(R.string.station_page_report_copy))
                    for ((code, label) in listOf("spam" to R.string.station_page_report_spam,
                        "offensive" to R.string.station_page_report_abuse,
                        "misleading" to R.string.station_page_report_misleading,
                        "inappropriate" to R.string.station_page_report_other)) {
                        FilterChip(selected = reason == code, onClick = { reason = code }, label = { Text(stringResource(label)) })
                    }
                }
            }, confirmButton = {
                TextButton(onClick = { reportTarget = null; notice = null; commentAttempt = true; comments.report(comment.id, reason) }, enabled = !busy) {
                    Text(stringResource(R.string.station_page_report_send))
                }
            }, dismissButton = { TextButton(onClick = { reportTarget = null }) { Text(stringResource(R.string.action_cancel)) } })
    }
    if (safetyState.userReportTarget != null) {
        ReportUserDialog(
            submitting = safetyState.submittingReport,
            onDismiss = safety::cancelReportUser,
            onSubmit = safety::submitReportUser,
        )
    }
    if (safetyState.blockTarget != null) {
        BlockUserDialog(
            blocking = safetyState.blocking,
            onDismiss = safety::cancelBlockUser,
            onConfirm = safety::confirmBlockUser,
        )
    }
}

private fun FeedbackCommentView.safetyRef(): SafetyContentRef = SafetyContentRef(SafetyContentKind.STATION_COMMENT, id)

private fun safetyNoticeText(notice: CommunitySafetyNotice): Int = when (notice) {
    CommunitySafetyNotice.REPORTED -> R.string.safety_report_user_received
    CommunitySafetyNotice.REPORT_REPLAYED -> R.string.safety_report_user_replayed
    CommunitySafetyNotice.BLOCKED -> R.string.safety_block_done
}

private fun safetyErrorText(error: CommunitySafetyError): Int = when (error) {
    CommunitySafetyError.SelfTarget -> R.string.safety_error_self_target
    CommunitySafetyError.BlockLimit -> R.string.safety_error_block_limit
    CommunitySafetyError.NotFound -> R.string.safety_error_not_found
    CommunitySafetyError.Offline -> R.string.safety_error_offline
    is CommunitySafetyError.RateLimited, CommunitySafetyError.Generic -> R.string.safety_error_generic
}
