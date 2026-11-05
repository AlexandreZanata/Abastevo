package com.anpfuel.app.ui.forum

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.OpenInNew
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.SuggestionChipDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
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

@Composable
fun ForumDetailScreen(
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: ForumDetailViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    ForumDetailContent(
        state = state,
        onBack = onBack,
        onRefresh = viewModel::refresh,
        onExternalLinkClick = viewModel::onExternalLinkClick,
        onDismissLinkDialog = viewModel::dismissLinkDialog,
        modifier = modifier,
    )
}

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
internal fun ForumDetailContent(
    state: ForumDetailUiState,
    onBack: () -> Unit,
    onRefresh: () -> Unit,
    onExternalLinkClick: (String) -> Unit,
    onDismissLinkDialog: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current

    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(stringResource(R.string.forum_detail_title)) },
                onNavigateUp = onBack,
                actions = {
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
                }
            }
        }
    }

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
}
