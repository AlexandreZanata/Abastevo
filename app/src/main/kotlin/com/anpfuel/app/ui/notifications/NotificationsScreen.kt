package com.anpfuel.app.ui.notifications

import android.text.format.DateUtils
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.DoneAll
import androidx.compose.material.icons.outlined.CalendarMonth
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.Forum
import androidx.compose.material.icons.outlined.HighlightOff
import androidx.compose.material.icons.outlined.NotificationsNone
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material.icons.outlined.TrendingDown
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.EmptyState
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.components.SkeletonText
import com.anpfuel.app.ui.components.skeletonShimmer
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.usecase.notification.AccountFeedState
import com.anpfuel.domain.notification.AppNotification
import com.anpfuel.domain.notification.AppNotificationContent

/**
 * B-BR-NOTIF-01 — Notifications: account notifications and local alerts in one
 * list, newest first, with an unread dot, mark-all-read and a settings action.
 */
@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun NotificationsScreen(
    onNavigateBack: () -> Unit,
    onOpenTarget: (NotificationTarget) -> Unit,
    onOpenSettings: () -> Unit,
    onSignIn: () -> Unit,
    viewModel: NotificationsViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { viewModel.refresh() }
    AnpScaffold(
        modifier = Modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(stringResource(R.string.notifications_screen_title)) },
                onNavigateUp = onNavigateBack,
                actions = {
                    if (state.unread > 0) {
                        IconButton(onClick = viewModel::markAllRead) {
                            Icon(Icons.Default.DoneAll, contentDescription = stringResource(R.string.notifications_mark_all_read))
                        }
                    }
                    IconButton(onClick = onOpenSettings) {
                        Icon(Icons.Outlined.Settings, contentDescription = stringResource(R.string.notifications_settings))
                    }
                },
            )
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(padding),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            when (state.accountState) {
                AccountFeedState.SIGNED_OUT -> item(key = "sign-in") {
                    NoticeRow(stringResource(R.string.notifications_sign_in_hint), stringResource(R.string.notifications_sign_in_action), onSignIn)
                }
                AccountFeedState.NOT_REFRESHED -> item(key = "stale") {
                    NoticeRow(stringResource(R.string.notifications_not_refreshed), stringResource(R.string.action_retry), viewModel::refresh)
                }
                AccountFeedState.FRESH -> Unit
            }
            if (state.markFailed) {
                item(key = "mark-failed") {
                    Text(
                        stringResource(R.string.notifications_mark_failed),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
            when {
                state.loading -> items(4) { NotificationRowSkeleton() }
                state.items.isEmpty() -> item(key = "empty") {
                    EmptyState(
                        message = stringResource(R.string.notifications_empty_title),
                        hint = stringResource(R.string.notifications_empty_hint),
                        icon = Icons.Outlined.NotificationsNone,
                        actionLabel = stringResource(R.string.notifications_settings),
                        onAction = onOpenSettings,
                    )
                }
                else -> items(state.items, key = { "${it.source}:${it.id}" }) { item ->
                    NotificationRow(item) { onOpenTarget(viewModel.open(item)) }
                }
            }
        }
    }
}

@Composable
private fun NotificationRow(item: AppNotification, onClick: () -> Unit) {
    val context = LocalContext.current
    val locale = LocalConfiguration.current.locales[0]
    val text = NotificationTextFormatter.format(context, item.content, locale)
    val time = DateUtils.getRelativeTimeSpanString(item.createdAtMillis, System.currentTimeMillis(), DateUtils.MINUTE_IN_MILLIS).toString()
    val unreadLabel = stringResource(R.string.notifications_unread)
    val scheme = MaterialTheme.colorScheme
    val container = scheme.surfaceContainerLow
    Surface(
        shape = AnpContainerDefaults.containerShape(),
        color = container,
        border = AnpContainerDefaults.borderStroke(container),
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .semantics { if (!item.read) contentDescription = "$unreadLabel. ${text.title}. ${text.body}. $time" },
    ) {
        Row(Modifier.padding(16.dp), horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.Top) {
            Surface(
                shape = CircleShape,
                color = if (item.read) Color.Transparent else scheme.primary,
                contentColor = if (item.read) scheme.onSurface else scheme.onPrimary,
                border = if (item.read) BorderStroke(1.dp, scheme.outlineVariant) else null,
                modifier = Modifier.size(40.dp),
            ) {
                Box(contentAlignment = Alignment.Center) { Icon(iconOf(item.content), null, Modifier.size(20.dp)) }
            }
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(
                        text.title,
                        style = MaterialTheme.typography.titleSmall,
                        fontWeight = if (item.read) FontWeight.SemiBold else FontWeight.Bold,
                        color = scheme.onSurface,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f, fill = false),
                    )
                    if (!item.read) Box(Modifier.size(8.dp).background(scheme.secondary, CircleShape))
                }
                Text(text.body, style = MaterialTheme.typography.bodyMedium, color = scheme.onSurfaceVariant, maxLines = 3, overflow = TextOverflow.Ellipsis)
                Text(time, style = MaterialTheme.typography.labelMedium, color = scheme.onSurfaceVariant)
            }
        }
    }
}

@Composable
private fun NoticeRow(message: String, action: String, onAction: () -> Unit) {
    val container = MaterialTheme.colorScheme.surfaceContainerLow
    Surface(
        shape = AnpContainerDefaults.containerShape(),
        color = container,
        border = AnpContainerDefaults.borderStroke(container),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Row(
            Modifier.padding(start = 16.dp, end = 8.dp, top = 8.dp, bottom = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(message, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
            TextButton(onClick = onAction) { Text(action) }
        }
    }
}

/** Mirrors [NotificationRow]: badge, title, body and time lines with the same paddings. */
@Composable
private fun NotificationRowSkeleton() {
    val container = MaterialTheme.colorScheme.surfaceContainerLow
    SkeletonGroup {
        Surface(
            shape = AnpContainerDefaults.containerShape(),
            color = container,
            border = AnpContainerDefaults.borderStroke(container),
            modifier = Modifier.fillMaxWidth(),
        ) {
            Row(Modifier.padding(16.dp), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Box(Modifier.size(40.dp).then(skeletonShimmer(cornerRadius = 20.dp)))
                Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    SkeletonText(MaterialTheme.typography.titleSmall, width = 180.dp)
                    SkeletonText(MaterialTheme.typography.bodyMedium, width = 240.dp)
                    SkeletonText(MaterialTheme.typography.labelMedium, width = 72.dp)
                }
            }
        }
    }
}

private fun iconOf(content: AppNotificationContent): ImageVector = when (content) {
    is AppNotificationContent.ForumReply -> Icons.Outlined.Forum
    is AppNotificationContent.ContributionDecided -> if (content.validated) Icons.Outlined.CheckCircle else Icons.Outlined.HighlightOff
    is AppNotificationContent.CityAverageDrop, is AppNotificationContent.VehiclePriceDrop -> Icons.Outlined.TrendingDown
    is AppNotificationContent.NewSurveyWeek -> Icons.Outlined.CalendarMonth
}
