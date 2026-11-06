package com.anpfuel.app.ui.forum

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.OpenInNew
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.SuggestionChipDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.SkeletonCard
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.domain.forum.ForumComment
import com.anpfuel.domain.forum.ForumReportReason

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
) {
    val context = LocalContext.current
    var selectedReportReason by remember { mutableStateOf(ForumReportReason.SPAM) }
    var reportDetailText by remember { mutableStateOf("") }

    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(stringResource(R.string.forum_detail_title)) },
                onNavigateUp = onBack,
                actions = {
                    if (state.post?.kind == com.anpfuel.domain.forum.ForumPostKind.TOPIC && state.isAuthenticated) {
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
                },
            )
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
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
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(8.dp),
                            ) {
                                SuggestionChip(
                                    onClick = {},
                                    label = { Text(stringResource(forumKindLabel(post.kind))) },
                                    colors = SuggestionChipDefaults.suggestionChipColors(
                                        containerColor = MaterialTheme.colorScheme.secondaryContainer,
                                        labelColor = MaterialTheme.colorScheme.onSecondaryContainer,
                                    ),
                                    border = null,
                                )
                                Text(
                                    text = formatForumDate(post.firstPublishedAt),
                                    style = MaterialTheme.typography.labelSmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                                if (post.updatedAt != post.firstPublishedAt) {
                                    Text(
                                        text = "· ${stringResource(R.string.forum_updated_label)}",
                                        style = MaterialTheme.typography.labelSmall,
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
                            if (source != null) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(4.dp),
                                ) {
                                    Text(
                                        text = source.name,
                                        style = MaterialTheme.typography.labelMedium,
                                        fontWeight = FontWeight.SemiBold,
                                        color = MaterialTheme.colorScheme.primary,
                                    )
                                    val alias = post.publicAlias
                                    if (alias != null) {
                                        Text(
                                            text = "· $alias",
                                            style = MaterialTheme.typography.labelSmall,
                                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                                        )
                                    }
                                }
                            }

                            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.5f))

                            if (post.excerpt.isNotBlank()) {
                                Card(
                                    modifier = Modifier.fillMaxWidth(),
                                    shape = AnpContainerDefaults.containerShape(),
                                    colors = CardDefaults.cardColors(
                                        containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
                                    ),
                                ) {
                                    Text(
                                        text = post.excerpt,
                                        style = MaterialTheme.typography.bodyLarge,
                                        fontStyle = FontStyle.Italic,
                                        color = MaterialTheme.colorScheme.onSurface,
                                        modifier = Modifier.padding(16.dp),
                                        lineHeight = 24.sp,
                                    )
                                }
                            }

                            val body = post.bodyText
                            if (!body.isNullOrBlank()) {
                                Text(
                                    text = body,
                                    style = MaterialTheme.typography.bodyLarge,
                                    color = MaterialTheme.colorScheme.onSurface,
                                    lineHeight = 26.sp,
                                )
                            }

                            val sourceUrl = source?.url
                            if (!sourceUrl.isNullOrBlank()) {
                                val host = try {
                                    java.net.URI(sourceUrl).host ?: sourceUrl
                                } catch (_: Exception) {
                                    sourceUrl
                                }
                                Button(
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
                        }
                    }

                    // Discussion Section
                    item {
                        HorizontalDivider(
                            modifier = Modifier.padding(vertical = 8.dp),
                            color = MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.5f),
                        )
                        Text(
                            text = stringResource(R.string.forum_comments_title),
                            style = MaterialTheme.typography.titleMedium,
                            fontWeight = FontWeight.Bold,
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
                                                val authorName = state.replyingTo.publicAlias ?: "Motorista"
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
                                    onToggleReplies = { onToggleReplies(comment.id) },
                                    onReply = { onStartReply(comment) },
                                    onReport = { onOpenReport("FORUM_COMMENT", comment.id) },
                                    onEdit = { onStartEdit(comment) },
                                    onDelete = { onConfirmRemove(comment) },
                                )
                            }
                        }
                    }
                }
            }
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
}

@Composable
private fun CommentItemView(
    comment: ForumComment,
    isExpanded: Boolean,
    replies: List<ForumComment>,
    repliesLoading: Boolean,
    onToggleReplies: () -> Unit,
    onReply: () -> Unit,
    onReport: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        border = AnpContainerDefaults.borderStroke(MaterialTheme.colorScheme.surface),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = comment.publicAlias ?: "Motorista",
                    style = MaterialTheme.typography.labelMedium,
                    fontWeight = FontWeight.Bold,
                    color = MaterialTheme.colorScheme.primary,
                )
                Text(
                    text = formatForumDate(comment.createdAt),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            Text(
                text = comment.bodyText,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurface,
            )

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(4.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (comment.replyCount > 0) {
                    TextButton(onClick = onToggleReplies) {
                        Text(
                            if (isExpanded) stringResource(R.string.forum_hide_replies)
                            else stringResource(R.string.forum_replies_count, comment.replyCount)
                        )
                    }
                }
                TextButton(onClick = onReply) {
                    Text(stringResource(R.string.forum_reply_button))
                }
                TextButton(onClick = onReport) {
                    Text(stringResource(R.string.forum_report_button))
                }
                Spacer(modifier = Modifier.weight(1f))
                TextButton(onClick = onEdit) {
                    Text(stringResource(R.string.forum_edit_comment))
                }
                TextButton(onClick = onDelete) {
                    Text(
                        text = stringResource(R.string.forum_delete_comment),
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }

            if (isExpanded) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(start = 16.dp, top = 8.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    if (repliesLoading && replies.isEmpty()) {
                        CircularProgressIndicator(modifier = Modifier.size(24.dp), strokeWidth = 2.dp)
                    } else {
                        replies.forEach { reply ->
                            ReplyItemView(
                                reply = reply,
                                onReport = onReport,
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ReplyItemView(
    reply: ForumComment,
    onReport: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(10.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = reply.publicAlias ?: "Motorista",
                    style = MaterialTheme.typography.labelSmall,
                    fontWeight = FontWeight.Bold,
                    color = MaterialTheme.colorScheme.primary,
                )
                Text(
                    text = formatForumDate(reply.createdAt),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            Text(
                text = reply.bodyText,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurface,
            )

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.End,
            ) {
                TextButton(onClick = onReport) {
                    Text(
                        text = stringResource(R.string.forum_report_button),
                        style = MaterialTheme.typography.labelSmall,
                    )
                }
            }
        }
    }
}

@Composable
private fun reportReasonText(reason: ForumReportReason): String {
    return when (reason) {
        ForumReportReason.SPAM -> stringResource(R.string.forum_report_reason_spam)
        ForumReportReason.OFFENSIVE -> stringResource(R.string.forum_report_reason_offensive)
        ForumReportReason.MISINFORMATION -> stringResource(R.string.forum_report_reason_misinformation)
        ForumReportReason.COMMERCIAL -> stringResource(R.string.forum_report_reason_commercial)
        ForumReportReason.LEGAL_NOTICE -> stringResource(R.string.forum_report_reason_legal_notice)
        ForumReportReason.OTHER -> stringResource(R.string.forum_report_reason_other)
    }
}
