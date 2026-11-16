package com.anpfuel.app.ui.forum

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.OpenInNew
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.outlined.ChatBubbleOutline
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.Share
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.SuggestionChipDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.LifecycleStartEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.SkeletonCard
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.designsystem.OutlinedButton
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.safety.BlockUserDialog
import com.anpfuel.app.ui.safety.ContentMenuAction
import com.anpfuel.app.ui.safety.ContentOverflowMenu
import com.anpfuel.app.ui.safety.ReportUserDialog
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.domain.forum.ForumComment
import com.anpfuel.domain.forum.ForumReaction
import com.anpfuel.domain.forum.ForumReplySort
import com.anpfuel.domain.forum.ForumReportReason
import com.anpfuel.domain.safety.SafetyContentRef
import com.anpfuel.domain.safety.UserReportReason
import kotlinx.coroutines.launch

@Composable
fun ForumDetailScreen(
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    onSignIn: () -> Unit = {},
    onEditTopic: (String) -> Unit = {},
    viewModel: ForumDetailViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    if (state.requireAuthPrompt) {
        viewModel.dismissAuthPrompt()
        onSignIn()
    }

    // Live updates only while the conversation is on screen (B-BR-FOR-18);
    // returning from sign-in also picks up the new session here.
    LifecycleStartEffect(viewModel) {
        viewModel.checkSessionConsistency()
        viewModel.startLive()
        onStopOrDispose { viewModel.stopLive() }
    }

    ForumDetailContent(
        state = state,
        onBack = onBack,
        onRefresh = viewModel::refresh,
        onSignIn = onSignIn,
        onEditTopic = onEditTopic,
        onExternalLinkClick = viewModel::onExternalLinkClick,
        onDismissLinkDialog = viewModel::dismissLinkDialog,
        onToggleReplies = viewModel::toggleReplies,
        onStartReply = viewModel::startReply,
        onCancelReply = viewModel::cancelReply,
        onUpdateComposerText = viewModel::updateComposerText,
        onSubmitComment = viewModel::submitComment,
        onStartEdit = viewModel::startEdit,
        onUpdateEditDraftText = viewModel::updateEditDraftText,
        onCancelEdit = viewModel::cancelEdit,
        onSaveEdit = viewModel::saveEdit,
        onConfirmRemove = viewModel::confirmRemove,
        onCancelRemove = viewModel::cancelRemove,
        onExecuteRemove = viewModel::executeRemove,
        onOpenReport = viewModel::openReport,
        onCancelReport = viewModel::cancelReport,
        onSubmitReport = viewModel::submitReport,
        onDismissReportReceipt = viewModel::dismissReportReceipt,
        onDismissSubmissionReceipt = viewModel::dismissSubmissionReceipt,
        onDismissActionError = viewModel::dismissActionError,
        onReactToPost = viewModel::reactToPost,
        onReactToComment = viewModel::reactToComment,
        onSortChange = viewModel::setReplySort,
        onLoadMoreComments = viewModel::loadMoreComments,
        onShowNewReplies = viewModel::showNewReplies,
        onOpenReportUser = viewModel::openReportUser,
        onCancelReportUser = viewModel::cancelReportUser,
        onSubmitReportUser = viewModel::submitReportUser,
        onDismissUserReportReceipt = viewModel::dismissUserReportReceipt,
        onOpenBlockUser = viewModel::openBlockUser,
        onCancelBlockUser = viewModel::cancelBlockUser,
        onConfirmBlockUser = viewModel::confirmBlockUser,
        onDismissBlockedNotice = viewModel::dismissBlockedNotice,
        modifier = modifier,
    )
}

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
internal fun ForumDetailContent(
    state: ForumDetailUiState,
    onBack: () -> Unit,
    onRefresh: () -> Unit,
    onSignIn: () -> Unit,
    onExternalLinkClick: (String) -> Unit,
    onDismissLinkDialog: () -> Unit,
    onToggleReplies: (String) -> Unit,
    onStartReply: (ForumComment) -> Unit,
    onCancelReply: () -> Unit,
    onUpdateComposerText: (String) -> Unit,
    onSubmitComment: () -> Unit,
    onStartEdit: (ForumComment) -> Unit,
    onUpdateEditDraftText: (String) -> Unit,
    onCancelEdit: () -> Unit,
    onSaveEdit: () -> Unit,
    onConfirmRemove: (ForumComment) -> Unit,
    onCancelRemove: () -> Unit,
    onExecuteRemove: () -> Unit,
    onOpenReport: (String, String) -> Unit,
    onCancelReport: () -> Unit,
    onSubmitReport: (ForumReportReason, String?) -> Unit,
    onDismissReportReceipt: () -> Unit,
    onDismissSubmissionReceipt: () -> Unit,
    onDismissActionError: () -> Unit,
    modifier: Modifier = Modifier,
    onEditTopic: (String) -> Unit = {},
    onReactToPost: (ForumReaction) -> Unit = {},
    onReactToComment: (ForumComment, ForumReaction) -> Unit = { _, _ -> },
    onSortChange: (ForumReplySort) -> Unit = {},
    onLoadMoreComments: () -> Unit = {},
    onShowNewReplies: () -> Unit = {},
    onOpenReportUser: (SafetyContentRef) -> Unit = {},
    onCancelReportUser: () -> Unit = {},
    onSubmitReportUser: (UserReportReason, String?) -> Unit = { _, _ -> },
    onDismissUserReportReceipt: () -> Unit = {},
    onOpenBlockUser: (SafetyContentRef) -> Unit = {},
    onCancelBlockUser: () -> Unit = {},
    onConfirmBlockUser: () -> Unit = {},
    onDismissBlockedNotice: () -> Unit = {},
) {
    val context = LocalContext.current
    var selectedReportReason by remember { mutableStateOf(ForumReportReason.SPAM) }
    var reportDetailText by remember { mutableStateOf("") }

    val share = rememberForumShare()
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.surface,
        topBar = {
            AnpTopAppBar(
                title = { Text(stringResource(R.string.forum_detail_title)) },
                onNavigateUp = onBack,
                actions = {
                    if (state.post?.kind == com.anpfuel.domain.forum.ForumPostKind.TOPIC && state.isTopicOwner) {
                        IconButton(
                            onClick = { onEditTopic(state.postId) },
                        ) {
                            Icon(
                                Icons.Default.Edit,
                                contentDescription = stringResource(R.string.forum_edit_topic),
                            )
                        }
                    }
                    IconButton(
                        onClick = onRefresh,
                        enabled = !state.loading && !state.refreshing,
                    ) {
                        Icon(
                            Icons.Default.Refresh,
                            contentDescription = stringResource(R.string.forum_retry),
                        )
                    }
                    // BUC-SAFE-01..03: a topic the viewer did not write can be
                    // reported and its author reported or blocked.
                    val topicRef = state.post?.safetyRef()
                    if (state.showsTopicSafetyActions && topicRef != null) {
                        ContentOverflowMenu(
                            actions = listOf(
                                ContentMenuAction(stringResource(R.string.safety_report_topic)) {
                                    onOpenReport("FORUM_POST", topicRef.id)
                                },
                                ContentMenuAction(stringResource(R.string.safety_report_user)) { onOpenReportUser(topicRef) },
                                ContentMenuAction(stringResource(R.string.safety_block_user)) { onOpenBlockUser(topicRef) },
                            ),
                            contentDescription = stringResource(R.string.safety_more_actions),
                            tint = LocalContentColor.current,
                        )
                    }
                },
            )
        },
    ) { padding ->
        val listState = rememberLazyListState()
        val scope = rememberCoroutineScope()
        Box(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
        ) {
        LazyColumn(
            state = listState,
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(16.dp, 16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            if (state.isOffline) {
                item {
                    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
                    Card(
                        modifier = Modifier.fillMaxWidth(),
                        shape = AnpContainerDefaults.containerShape(),
                        colors = CardDefaults.cardColors(containerColor = containerColor),
                        border = AnpContainerDefaults.borderStroke(containerColor),
                    ) {
                        Text(
                            text = stringResource(R.string.forum_offline_notice),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(12.dp),
                        )
                    }
                }
            }

            // Action Error Notice
            val currentActionError = state.actionError
            if (currentActionError != null) {
                item {
                    val errorMsg = when (currentActionError) {
                        is ForumActionError.Conflict -> stringResource(R.string.forum_conflict_error)
                        is ForumActionError.RateLimited -> stringResource(
                            R.string.forum_rate_limited_error,
                            currentActionError.retryAfterSeconds ?: 30L,
                        )
                        is ForumActionError.TargetMissing -> stringResource(R.string.forum_post_not_found_hint)
                        is ForumActionError.Offline -> stringResource(R.string.forum_offline_notice)
                        is ForumActionError.Generic -> currentActionError.message ?: stringResource(R.string.forum_unavailable_hint)
                        is ForumActionError.SelfTarget -> stringResource(R.string.safety_error_self_target)
                        is ForumActionError.BlockLimit -> stringResource(R.string.safety_error_block_limit)
                    }
                    Card(
                        modifier = Modifier.fillMaxWidth(),
                        shape = AnpContainerDefaults.containerShape(),
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.errorContainer),
                    ) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(12.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween,
                        ) {
                            Text(
                                text = errorMsg,
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onErrorContainer,
                                modifier = Modifier.weight(1f),
                            )
                            IconButton(onClick = onDismissActionError, modifier = Modifier.size(48.dp)) {
                                Icon(
                                    Icons.Default.Close,
                                    contentDescription = stringResource(R.string.action_cancel),
                                    tint = MaterialTheme.colorScheme.onErrorContainer,
                                )
                            }
                        }
                    }
                }
            }

            // Submission Review Receipt
            if (state.submissionReceipt != null) {
                item {
                    Card(
                        modifier = Modifier.fillMaxWidth(),
                        shape = AnpContainerDefaults.containerShape(),
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.primaryContainer),
                    ) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(12.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween,
                        ) {
                            Text(
                                text = stringResource(R.string.forum_comment_pending_receipt),
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onPrimaryContainer,
                                modifier = Modifier.weight(1f),
                            )
                            IconButton(onClick = onDismissSubmissionReceipt, modifier = Modifier.size(48.dp)) {
                                Icon(
                                    Icons.Default.Close,
                                    contentDescription = stringResource(R.string.action_cancel),
                                    tint = MaterialTheme.colorScheme.onPrimaryContainer,
                                )
                            }
                        }
                    }
                }
            }

            // Report Receipt
            if (state.reportReceipt != null) {
                item {
                    Card(
                        modifier = Modifier.fillMaxWidth(),
                        shape = AnpContainerDefaults.containerShape(),
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.secondaryContainer),
                    ) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(12.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween,
                        ) {
                            Text(
                                text = stringResource(R.string.forum_report_success),
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSecondaryContainer,
                                modifier = Modifier.weight(1f),
                            )
                            IconButton(onClick = onDismissReportReceipt, modifier = Modifier.size(48.dp)) {
                                Icon(
                                    Icons.Default.Close,
                                    contentDescription = stringResource(R.string.action_cancel),
                                    tint = MaterialTheme.colorScheme.onSecondaryContainer,
                                )
                            }
                        }
                    }
                }
            }

            if (state.userReportReceipt != null) {
                item {
                    ForumNoticeCard(
                        text = stringResource(
                            if (state.userReportReceipt.replayed) {
                                R.string.safety_report_user_replayed
                            } else {
                                R.string.safety_report_user_received
                            },
                        ),
                        onDismiss = onDismissUserReportReceipt,
                    )
                }
            }

            if (state.blockedNotice) {
                item {
                    ForumNoticeCard(text = stringResource(R.string.safety_block_done), onDismiss = onDismissBlockedNotice)
                }
            }

            when {
                state.loading && state.post == null -> {
                    item {
                        SkeletonGroup(modifier = Modifier.fillMaxWidth()) {
                            SkeletonCard(height = 240.dp)
                        }
                    }
                }
                state.error == ForumDetailError.NOT_FOUND -> {
                    item {
                        ForumMessageCard(
                            title = stringResource(R.string.forum_post_not_found_title),
                            description = stringResource(R.string.forum_post_not_found_hint),
                            actionLabel = stringResource(R.string.action_back),
                            onAction = onBack,
                        )
                    }
                }
                state.post != null && state.topicHidden -> {
                    // B-BR-SAFE-06: a topic by a blocked author shows a neutral notice instead of its text.
                    item {
                        ForumMessageCard(
                            title = stringResource(R.string.safety_topic_hidden_title),
                            description = stringResource(R.string.safety_topic_hidden_message),
                            actionLabel = stringResource(R.string.action_back),
                            onAction = onBack,
                        )
                    }
                }
                state.error != null && state.post == null -> {
                    item {
                        ForumMessageCard(
                            title = stringResource(R.string.forum_unavailable_title),
                            description = stringResource(R.string.forum_unavailable_hint),
                            actionLabel = stringResource(R.string.forum_retry),
                            onAction = onRefresh,
                        )
                    }
                }
                state.post != null -> {
                    val post = state.post
                    item {
                        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                            // Author header, like the opened post of a social feed.
                            val author = forumPostAuthor(post.kind, post.publicAlias)
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(12.dp),
                            ) {
                                ForumAvatar(author, size = 48.dp)
                                Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                                    ForumAuthorLine(author = author, timeIso = post.firstPublishedAt)
                                    Text(
                                        text = listOfNotNull(
                                            stringResource(forumKindLabel(post.kind)),
                                            stringResource(R.string.forum_updated_label)
                                                .takeIf { post.updatedAt != post.firstPublishedAt },
                                        ).joinToString(" · "),
                                        style = MaterialTheme.typography.labelMedium,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    )
                                }
                            }

                            Text(
                                text = post.title,
                                style = MaterialTheme.typography.headlineSmall,
                                fontWeight = FontWeight.Bold,
                            )

                            val source = post.source
                            if (post.excerpt.isNotBlank()) {
                                Text(
                                    text = post.excerpt,
                                    style = MaterialTheme.typography.bodyLarge,
                                    fontWeight = FontWeight.SemiBold,
                                    color = MaterialTheme.colorScheme.onSurface,
                                    lineHeight = 24.sp,
                                )
                            }

                            val body = post.bodyText
                            if (!body.isNullOrBlank()) {
                                ForumBodyText(body = body)
                            }

                            val bulletin = post.bulletin
                            if (bulletin != null) {
                                Card(
                                    modifier = Modifier.fillMaxWidth(),
                                    shape = AnpContainerDefaults.containerShape(),
                                    colors = CardDefaults.cardColors(
                                        containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
                                    ),
                                ) {
                                    Column(
                                        modifier = Modifier.padding(16.dp),
                                        verticalArrangement = Arrangement.spacedBy(6.dp),
                                    ) {
                                        Text(
                                            text = stringResource(R.string.forum_bulletin_provenance_title),
                                            style = MaterialTheme.typography.titleSmall,
                                            fontWeight = FontWeight.Bold,
                                        )
                                        if (post.supersedesPostId != null) {
                                            Text(
                                                text = stringResource(R.string.forum_bulletin_correction_notice),
                                                style = MaterialTheme.typography.labelMedium,
                                                color = MaterialTheme.colorScheme.primary,
                                            )
                                        }
                                        Text(
                                            text = stringResource(
                                                R.string.forum_bulletin_methodology,
                                                bulletin.methodologyVersion,
                                                bulletin.templateVersion,
                                            ),
                                            style = MaterialTheme.typography.bodySmall,
                                        )
                                        Text(
                                            text = stringResource(
                                                R.string.forum_bulletin_fuel_unit,
                                                bulletin.fuel,
                                                bulletin.unit,
                                            ),
                                            style = MaterialTheme.typography.bodySmall,
                                        )
                                        Text(
                                            text = stringResource(
                                                R.string.forum_bulletin_current_period,
                                                bulletin.current.surveyStart,
                                                bulletin.current.surveyEnd,
                                            ),
                                            style = MaterialTheme.typography.bodySmall,
                                        )
                                        Text(
                                            text = stringResource(
                                                R.string.forum_bulletin_previous_period,
                                                bulletin.previous.surveyStart,
                                                bulletin.previous.surveyEnd,
                                            ),
                                            style = MaterialTheme.typography.bodySmall,
                                        )
                                        Text(
                                            text = stringResource(
                                                R.string.forum_bulletin_current_sample,
                                                bulletin.current.observationCount,
                                                bulletin.current.sampledStations,
                                                bulletin.current.sourceStations,
                                            ),
                                            style = MaterialTheme.typography.bodySmall,
                                        )
                                        Text(
                                            text = stringResource(
                                                R.string.forum_bulletin_previous_sample,
                                                bulletin.previous.observationCount,
                                                bulletin.previous.sampledStations,
                                                bulletin.previous.sourceStations,
                                            ),
                                            style = MaterialTheme.typography.bodySmall,
                                        )
                                    }
                                }
                            }

                            val sourceUrl = source?.url
                            if (!sourceUrl.isNullOrBlank()) {
                                val host = try {
                                    java.net.URI(sourceUrl).host ?: sourceUrl
                                } catch (_: Exception) {
                                    sourceUrl
                                }
                                OutlinedButton(
                                    onClick = { onExternalLinkClick(sourceUrl) },
                                    modifier = Modifier.fillMaxWidth(),
                                ) {
                                    Icon(
                                        Icons.AutoMirrored.Filled.OpenInNew,
                                        contentDescription = null,
                                        modifier = Modifier.padding(end = 8.dp),
                                    )
                                    Text(stringResource(R.string.forum_read_source, host))
                                }
                            }

                            if (state.linkError) {
                                Text(
                                    text = stringResource(R.string.forum_external_link_invalid),
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.error,
                                )
                            }

                            ForumDivider()
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceAround,
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                val on = stringResource(R.string.forum_reaction_on)
                                val off = stringResource(R.string.forum_reaction_off)
                                ForumIconAction(
                                    icon = Icons.Outlined.ChatBubbleOutline,
                                    contentDescription = stringResource(R.string.forum_action_comment),
                                    onClick = { if (!state.isAuthenticated) onSignIn() },
                                    count = state.comments.size,
                                )
                                ForumLikeToggle(
                                    liked = state.postReaction == ForumReaction.LIKE,
                                    count = post.reactions.likes,
                                    label = stringResource(R.string.forum_like),
                                    stateLabel = if (state.postReaction == ForumReaction.LIKE) on else off,
                                    onToggle = { onReactToPost(ForumReaction.LIKE) },
                                )
                                ForumDislikeToggle(
                                    disliked = state.postReaction == ForumReaction.DISLIKE,
                                    count = post.reactions.dislikes,
                                    label = stringResource(R.string.forum_dislike),
                                    stateLabel = if (state.postReaction == ForumReaction.DISLIKE) on else off,
                                    onToggle = { onReactToPost(ForumReaction.DISLIKE) },
                                )
                                ForumIconAction(
                                    icon = Icons.Outlined.Share,
                                    contentDescription = stringResource(R.string.forum_action_share),
                                    onClick = { share(post.title, post.excerpt) },
                                )
                            }
                            ForumDivider()
                        }
                    }

                    // Discussion Section
                    item {
                        ForumRepliesHeader(
                            sort = state.replySort,
                            live = state.liveConnected,
                            onSortChange = onSortChange,
                        )
                    }

                    if (!state.isSocialEnabled) {
                        item {
                            Card(
                                modifier = Modifier.fillMaxWidth(),
                                shape = AnpContainerDefaults.containerShape(),
                                colors = CardDefaults.cardColors(
                                    containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
                                ),
                            ) {
                                Text(
                                    text = stringResource(R.string.forum_social_disabled_notice),
                                    style = MaterialTheme.typography.bodyMedium,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    modifier = Modifier.padding(16.dp),
                                )
                            }
                        }
                    } else if (!post.commentsEnabled) {
                        item {
                            Card(
                                modifier = Modifier.fillMaxWidth(),
                                shape = AnpContainerDefaults.containerShape(),
                                colors = CardDefaults.cardColors(
                                    containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
                                ),
                            ) {
                                Text(
                                    text = stringResource(R.string.forum_comments_disabled),
                                    style = MaterialTheme.typography.bodyMedium,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    modifier = Modifier.padding(16.dp),
                                )
                            }
                        }
                    } else {
                        // Composer or Sign-in Prompt
                        item {
                            if (!state.isAuthenticated) {
                                Card(
                                    modifier = Modifier.fillMaxWidth(),
                                    shape = AnpContainerDefaults.containerShape(),
                                    colors = CardDefaults.cardColors(
                                        containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
                                    ),
                                ) {
                                    Column(
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .padding(16.dp),
                                        horizontalAlignment = Alignment.CenterHorizontally,
                                        verticalArrangement = Arrangement.spacedBy(8.dp),
                                    ) {
                                        Button(
                                            onClick = onSignIn,
                                            modifier = Modifier.fillMaxWidth(),
                                        ) {
                                            Text(stringResource(R.string.forum_sign_in_to_participate))
                                        }
                                    }
                                }
                            } else {
                                Card(
                                    modifier = Modifier.fillMaxWidth(),
                                    shape = AnpContainerDefaults.containerShape(),
                                    colors = CardDefaults.cardColors(
                                        containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
                                    ),
                                ) {
                                    Column(
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .padding(16.dp),
                                        verticalArrangement = Arrangement.spacedBy(8.dp),
                                    ) {
                                        if (state.replyingTo != null) {
                                            Row(
                                                modifier = Modifier.fillMaxWidth(),
                                                verticalAlignment = Alignment.CenterVertically,
                                                horizontalArrangement = Arrangement.SpaceBetween,
                                            ) {
                                                val authorName = forumCommentAuthor(state.replyingTo.publicAlias).name
                                                Text(
                                                    text = stringResource(R.string.forum_replying_to, authorName),
                                                    style = MaterialTheme.typography.labelMedium,
                                                    color = MaterialTheme.colorScheme.primary,
                                                )
                                                IconButton(onClick = onCancelReply, modifier = Modifier.size(48.dp)) {
                                                    Icon(
                                                        Icons.Default.Close,
                                                        contentDescription = stringResource(R.string.forum_cancel_reply),
                                                        modifier = Modifier.size(16.dp),
                                                    )
                                                }
                                            }
                                        }

                                        OutlinedTextField(
                                            value = state.composerText,
                                            onValueChange = onUpdateComposerText,
                                            placeholder = { Text(stringResource(R.string.forum_comment_composer_hint)) },
                                            modifier = Modifier.fillMaxWidth(),
                                            minLines = 2,
                                            maxLines = 5,
                                        )

                                        Row(
                                            modifier = Modifier.fillMaxWidth(),
                                            horizontalArrangement = Arrangement.SpaceBetween,
                                            verticalAlignment = Alignment.CenterVertically,
                                        ) {
                                            Text(
                                                text = stringResource(R.string.forum_comment_char_counter, state.composerText.length),
                                                style = MaterialTheme.typography.labelSmall,
                                                color = if (state.composerText.length >= 280) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                                            )
                                            Button(
                                                onClick = onSubmitComment,
                                                enabled = state.composerText.isNotBlank() && !state.isSubmittingComment,
                                            ) {
                                                if (state.isSubmittingComment) {
                                                    CircularProgressIndicator(
                                                        modifier = Modifier.size(16.dp),
                                                        strokeWidth = 2.dp,
                                                    )
                                                } else {
                                                    Text(stringResource(R.string.forum_comment_publish))
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }

                        // Comments List
                        if (state.commentsLoading && state.comments.isEmpty()) {
                            item {
                                SkeletonGroup(modifier = Modifier.fillMaxWidth()) {
                                    SkeletonCard(height = 80.dp)
                                    Spacer(modifier = Modifier.height(8.dp))
                                    SkeletonCard(height = 80.dp)
                                }
                            }
                        } else if (state.comments.isEmpty()) {
                            item {
                                Text(
                                    text = stringResource(R.string.forum_comments_empty),
                                    style = MaterialTheme.typography.bodyMedium,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    modifier = Modifier.padding(vertical = 12.dp),
                                )
                            }
                        } else {
                            items(state.comments, key = { it.id }) { comment ->
                                CommentItemView(
                                    comment = comment,
                                    isExpanded = state.expandedReplies.contains(comment.id),
                                    replies = state.repliesMap[comment.id].orEmpty(),
                                    repliesLoading = state.repliesLoading.contains(comment.id),
                                    reactions = state.commentReactions,
                                    onReact = onReactToComment,
                                    onToggleReplies = { onToggleReplies(comment.id) },
                                    onReply = { onStartReply(comment) },
                                    onReport = { target -> onOpenReport("FORUM_COMMENT", target.id) },
                                    onReportUser = { target -> onOpenReportUser(target.safetyRef()) },
                                    onBlockUser = { target -> onOpenBlockUser(target.safetyRef()) },
                                    ownCommentIds = state.ownCommentIds,
                                    isOwn = state.ownCommentIds.contains(comment.id),
                                    onEdit = { onStartEdit(comment) },
                                    onDelete = { onConfirmRemove(comment) },
                                )
                                ForumDivider(modifier = Modifier.padding(top = 8.dp))
                            }
                            if (state.commentsCursor != null) {
                                item(key = "load-more-replies") {
                                    Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                                        if (state.loadingMoreComments) {
                                            CircularProgressIndicator(modifier = Modifier.size(24.dp), strokeWidth = 2.dp)
                                        } else {
                                            TextButton(onClick = onLoadMoreComments) {
                                                Text(stringResource(R.string.forum_show_more_replies))
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
        // New replies wait behind this pill instead of moving the list (B-BR-FOR-18).
        val pendingCount = state.pendingLiveComments.size
        ForumNewRepliesPill(
            visible = pendingCount > 0,
            label = pluralStringResource(R.plurals.forum_new_replies, pendingCount, pendingCount),
            onClick = {
                val trailing = if (state.commentsCursor != null) 1 else 0
                onShowNewReplies()
                scope.launch {
                    withFrameNanos { }
                    val total = listState.layoutInfo.totalItemsCount
                    val firstReply = total - state.comments.size - pendingCount - trailing
                    listState.animateScrollToItem(firstReply.coerceAtLeast(0))
                }
            },
            modifier = Modifier
                .align(Alignment.TopCenter)
                .padding(top = 12.dp),
        )
        }
    }

    // External link dialog
    if (state.linkConfirmationTarget != null) {
        val targetUrl = state.linkConfirmationTarget
        val host = state.linkConfirmationHost.orEmpty()
        AlertDialog(
            onDismissRequest = onDismissLinkDialog,
            title = { Text(stringResource(R.string.forum_external_link_dialog_title)) },
            text = { Text(stringResource(R.string.forum_external_link_dialog_message, host)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        onDismissLinkDialog()
                        try {
                            val intent = Intent(Intent.ACTION_VIEW, Uri.parse(targetUrl))
                            context.startActivity(intent)
                        } catch (_: Exception) {
                        }
                    },
                ) {
                    Text(stringResource(R.string.forum_external_link_dialog_confirm))
                }
            },
            dismissButton = {
                TextButton(onClick = onDismissLinkDialog) {
                    Text(stringResource(R.string.forum_external_link_dialog_cancel))
                }
            },
        )
    }

    // Edit comment dialog
    if (state.editingComment != null) {
        AlertDialog(
            onDismissRequest = onCancelEdit,
            title = { Text(stringResource(R.string.forum_edit_dialog_title)) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(
                        value = state.editDraftText,
                        onValueChange = onUpdateEditDraftText,
                        modifier = Modifier.fillMaxWidth(),
                        minLines = 3,
                        maxLines = 6,
                    )
                    Text(
                        text = stringResource(R.string.forum_comment_char_counter, state.editDraftText.length),
                        style = MaterialTheme.typography.labelSmall,
                        color = if (state.editDraftText.length >= 280) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            },
            confirmButton = {
                Button(
                    onClick = onSaveEdit,
                    enabled = state.editDraftText.isNotBlank() && !state.isSavingEdit,
                ) {
                    if (state.isSavingEdit) {
                        CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp)
                    } else {
                        Text(stringResource(R.string.forum_edit_save))
                    }
                }
            },
            dismissButton = {
                TextButton(onClick = onCancelEdit, enabled = !state.isSavingEdit) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    // Delete comment confirmation dialog
    if (state.commentPendingRemoval != null) {
        AlertDialog(
            onDismissRequest = onCancelRemove,
            title = { Text(stringResource(R.string.forum_delete_dialog_title)) },
            text = { Text(stringResource(R.string.forum_delete_dialog_message)) },
            confirmButton = {
                Button(
                    onClick = onExecuteRemove,
                    enabled = !state.isRemovingComment,
                ) {
                    if (state.isRemovingComment) {
                        CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp)
                    } else {
                        Text(stringResource(R.string.forum_delete_confirm))
                    }
                }
            },
            dismissButton = {
                TextButton(onClick = onCancelRemove, enabled = !state.isRemovingComment) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    // Report target dialog
    if (state.reportingTarget != null) {
        AlertDialog(
            onDismissRequest = onCancelReport,
            title = { Text(stringResource(R.string.forum_report_dialog_title)) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    Text(
                        text = stringResource(R.string.forum_report_dialog_reason_prompt),
                        style = MaterialTheme.typography.labelMedium,
                    )
                    ForumReportReason.entries.forEach { reason ->
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable { selectedReportReason = reason }
                                .padding(vertical = 4.dp),
                        ) {
                            RadioButton(
                                selected = selectedReportReason == reason,
                                onClick = { selectedReportReason = reason },
                            )
                            Text(
                                text = reportReasonText(reason),
                                style = MaterialTheme.typography.bodyMedium,
                                modifier = Modifier.padding(start = 8.dp),
                            )
                        }
                    }
                    OutlinedTextField(
                        value = reportDetailText,
                        onValueChange = { if (it.length <= 500) reportDetailText = it },
                        placeholder = { Text(stringResource(R.string.forum_report_detail_hint)) },
                        modifier = Modifier.fillMaxWidth(),
                        maxLines = 3,
                    )
                }
            },
            confirmButton = {
                Button(
                    onClick = {
                        onSubmitReport(selectedReportReason, reportDetailText.takeIf { it.isNotBlank() })
                        reportDetailText = ""
                    },
                    enabled = !state.isSubmittingReport,
                ) {
                    if (state.isSubmittingReport) {
                        CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp)
                    } else {
                        Text(stringResource(R.string.forum_report_submit))
                    }
                }
            },
            dismissButton = {
                TextButton(onClick = onCancelReport, enabled = !state.isSubmittingReport) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    if (state.userReportTarget != null) {
        ReportUserDialog(
            submitting = state.isSubmittingUserReport,
            onDismiss = onCancelReportUser,
            onSubmit = onSubmitReportUser,
        )
    }

    if (state.blockTarget != null) {
        BlockUserDialog(
            blocking = state.isBlocking,
            onDismiss = onCancelBlockUser,
            onConfirm = onConfirmBlockUser,
        )
    }
}

/** Dismissible confirmation in the conversation list, like the report receipt. */
@Composable
private fun ForumNoticeCard(text: String, onDismiss: () -> Unit) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.secondaryContainer),
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween,
        ) {
            Text(
                text = text,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSecondaryContainer,
                modifier = Modifier.weight(1f),
            )
            IconButton(onClick = onDismiss, modifier = Modifier.size(48.dp)) {
                Icon(
                    Icons.Default.Close,
                    contentDescription = stringResource(R.string.action_cancel),
                    tint = MaterialTheme.colorScheme.onSecondaryContainer,
                )
            }
        }
    }
}

/** "Comentários" with the live dot and the reply order menu, as on X. */
@Composable
private fun ForumRepliesHeader(
    sort: ForumReplySort,
    live: Boolean,
    onSortChange: (ForumReplySort) -> Unit,
) {
    var menuOpen by remember { mutableStateOf(false) }
    val labels = mapOf(
        ForumReplySort.RELEVANT to stringResource(R.string.forum_sort_relevant),
        ForumReplySort.RECENT to stringResource(R.string.forum_sort_recent),
        ForumReplySort.LIKED to stringResource(R.string.forum_sort_liked),
    )
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(
            text = stringResource(R.string.forum_comments_title),
            style = MaterialTheme.typography.titleMedium,
            fontWeight = FontWeight.Bold,
        )
        if (live) {
            Box(Modifier.size(8.dp).background(forumLikeColor(), CircleShape))
            Text(
                text = stringResource(R.string.forum_live),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Spacer(Modifier.weight(1f))
        Box {
            TextButton(
                onClick = { menuOpen = true },
                modifier = Modifier.semantics { contentDescription = labels.getValue(sort) },
            ) {
                Text(labels.getValue(sort), color = MaterialTheme.colorScheme.onSurface)
                Icon(
                    Icons.Filled.ArrowDropDown,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.onSurface,
                )
            }
            DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                ForumReplySort.entries.forEach { option ->
                    DropdownMenuItem(
                        text = {
                            Text(
                                labels.getValue(option),
                                fontWeight = if (option == sort) FontWeight.Bold else FontWeight.Normal,
                            )
                        },
                        onClick = {
                            menuOpen = false
                            onSortChange(option)
                        },
                    )
                }
            }
        }
    }
}

/**
 * One reply of the conversation, as on X: avatar with the thread line down
 * to its best answer preview, author line, text and icon actions with like
 * and dislike. The full thread opens on demand under the same line.
 */
@Composable
private fun CommentItemView(
    comment: ForumComment,
    isExpanded: Boolean,
    replies: List<ForumComment>,
    repliesLoading: Boolean,
    reactions: Map<String, ForumReaction>,
    onReact: (ForumComment, ForumReaction) -> Unit,
    onToggleReplies: () -> Unit,
    onReply: () -> Unit,
    onReport: (ForumComment) -> Unit,
    onReportUser: (ForumComment) -> Unit,
    onBlockUser: (ForumComment) -> Unit,
    ownCommentIds: Set<String>,
    isOwn: Boolean,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val author = forumCommentAuthor(comment.publicAlias)
    fun safetyFor(target: ForumComment) = ForumCommentSafety(
        isOwn = target.id in ownCommentIds,
        onReport = { onReport(target) },
        onReportUser = { onReportUser(target) },
        onBlockUser = { onBlockUser(target) },
    )
    val preview = comment.topReply?.takeIf { !isExpanded }
    val threadBelow = preview != null || (isExpanded && (replies.isNotEmpty() || repliesLoading))
    Column(modifier = modifier.fillMaxWidth()) {
        ForumThreadRow(
            avatar = { ForumAvatar(author, size = 40.dp) },
            connectBelow = threadBelow,
        ) {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                ForumAuthorLine(author = author, timeIso = comment.createdAt)
                Text(
                    text = comment.bodyText,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurface,
                )
                ForumCommentActions(
                    comment = comment,
                    reaction = reactions[comment.id] ?: ForumReaction.NONE,
                    onReact = onReact,
                    onReply = onReply,
                    safety = safetyFor(comment),
                    isOwn = isOwn,
                    onEdit = onEdit,
                    onDelete = onDelete,
                )
            }
        }
        if (preview != null) {
            ReplyItemView(
                reply = preview,
                reaction = reactions[preview.id] ?: ForumReaction.NONE,
                onReact = onReact,
                safety = safetyFor(preview),
                connectBelow = false,
            )
        }
        val hiddenReplies = comment.replyCount - (if (preview != null) 1 else 0)
        if (!isExpanded && hiddenReplies > 0) {
            TextButton(onClick = onToggleReplies, modifier = Modifier.padding(start = 44.dp)) {
                Text(stringResource(R.string.forum_replies_count, comment.replyCount))
            }
        }
        if (isExpanded) {
            if (repliesLoading && replies.isEmpty()) {
                CircularProgressIndicator(
                    modifier = Modifier.padding(start = 52.dp, top = 8.dp).size(20.dp),
                    strokeWidth = 2.dp,
                )
            }
            replies.forEachIndexed { index, reply ->
                ReplyItemView(
                    reply = reply,
                    reaction = reactions[reply.id] ?: ForumReaction.NONE,
                    onReact = onReact,
                    safety = safetyFor(reply),
                    connectBelow = index < replies.lastIndex,
                )
            }
            TextButton(onClick = onToggleReplies, modifier = Modifier.padding(start = 44.dp)) {
                Text(stringResource(R.string.forum_hide_replies))
            }
        }
    }
}

/** Report and block entry points of one comment or reply (BUC-SAFE-02, BUC-SAFE-03). */
private class ForumCommentSafety(
    val isOwn: Boolean,
    val onReport: () -> Unit,
    val onReportUser: () -> Unit,
    val onBlockUser: () -> Unit,
)

/**
 * Reply, like, dislike, the owner's edit/remove and a "more" menu with
 * report and block, in one row of icons. Own content shows no report or
 * block action (BUC-SAFE-02).
 */
@Composable
private fun ForumCommentActions(
    comment: ForumComment,
    reaction: ForumReaction,
    onReact: (ForumComment, ForumReaction) -> Unit,
    onReply: (() -> Unit)?,
    safety: ForumCommentSafety,
    isOwn: Boolean = false,
    onEdit: () -> Unit = {},
    onDelete: () -> Unit = {},
) {
    val on = stringResource(R.string.forum_reaction_on)
    val off = stringResource(R.string.forum_reaction_off)
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (onReply != null) {
            ForumIconAction(
                icon = Icons.Outlined.ChatBubbleOutline,
                contentDescription = stringResource(R.string.forum_reply_button),
                onClick = onReply,
                count = comment.replyCount,
            )
        }
        ForumLikeToggle(
            liked = reaction == ForumReaction.LIKE,
            count = comment.reactions.likes,
            label = stringResource(R.string.forum_like),
            stateLabel = if (reaction == ForumReaction.LIKE) on else off,
            onToggle = { onReact(comment, ForumReaction.LIKE) },
        )
        ForumDislikeToggle(
            disliked = reaction == ForumReaction.DISLIKE,
            count = comment.reactions.dislikes,
            label = stringResource(R.string.forum_dislike),
            stateLabel = if (reaction == ForumReaction.DISLIKE) on else off,
            onToggle = { onReact(comment, ForumReaction.DISLIKE) },
        )
        Spacer(modifier = Modifier.weight(1f))
        if (isOwn) {
            ForumIconAction(
                icon = Icons.Outlined.Edit,
                contentDescription = stringResource(R.string.forum_edit_comment),
                onClick = onEdit,
            )
            ForumIconAction(
                icon = Icons.Outlined.DeleteOutline,
                contentDescription = stringResource(R.string.forum_delete_comment),
                onClick = onDelete,
            )
        }
        if (!safety.isOwn) {
            ContentOverflowMenu(
                actions = listOf(
                    ContentMenuAction(stringResource(R.string.safety_report_comment), safety.onReport),
                    ContentMenuAction(stringResource(R.string.safety_report_user), safety.onReportUser),
                    ContentMenuAction(stringResource(R.string.safety_block_user), safety.onBlockUser),
                ),
                contentDescription = stringResource(R.string.safety_more_actions),
            )
        }
    }
}

/** Answer inside a thread: smaller avatar on the same line, same actions. */
@Composable
private fun ReplyItemView(
    reply: ForumComment,
    reaction: ForumReaction,
    onReact: (ForumComment, ForumReaction) -> Unit,
    safety: ForumCommentSafety,
    connectBelow: Boolean,
    modifier: Modifier = Modifier,
) {
    val author = forumCommentAuthor(reply.publicAlias)
    ForumThreadRow(
        avatar = { ForumAvatar(author, size = 32.dp) },
        connectBelow = connectBelow,
        connectAbove = true,
        avatarSize = 40.dp,
        modifier = modifier.padding(top = 4.dp),
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
            ForumAuthorLine(author = author, timeIso = reply.createdAt)
            Text(
                text = reply.bodyText,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurface,
            )
            ForumCommentActions(
                comment = reply,
                reaction = reaction,
                onReact = onReact,
                onReply = null,
                safety = safety,
            )
        }
    }
}

@Composable
private fun reportReasonText(reason: ForumReportReason): String {
    return when (reason) {
        ForumReportReason.SPAM -> stringResource(R.string.forum_report_reason_spam)
        ForumReportReason.HARASSMENT -> stringResource(R.string.forum_report_reason_harassment)
        ForumReportReason.DEFAMATION -> stringResource(R.string.forum_report_reason_defamation)
        ForumReportReason.OFF_TOPIC -> stringResource(R.string.forum_report_reason_off_topic)
        ForumReportReason.ILLEGAL -> stringResource(R.string.forum_report_reason_illegal)
        ForumReportReason.OTHER -> stringResource(R.string.forum_report_reason_other)
    }
}
