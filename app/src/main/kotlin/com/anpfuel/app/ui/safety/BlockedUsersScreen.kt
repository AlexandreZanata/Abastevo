package com.anpfuel.app.ui.safety

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Block
import androidx.compose.material.icons.outlined.PersonOff
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.community.CommunitySafetyError
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.EmptyState
import com.anpfuel.app.ui.components.SkeletonCard
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.designsystem.OutlinedButton
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.domain.safety.BlockedUser
import com.anpfuel.domain.safety.SafetyContentKind
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/** Account > Blocked users (BUC-SAFE-04). */
@Composable
fun BlockedUsersScreen(
    onNavigateBack: () -> Unit,
    onSignIn: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: BlockedUsersViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    LaunchedEffect(state.requireSignIn) {
        if (state.requireSignIn) {
            viewModel.consumeSignIn()
            onSignIn()
        }
    }
    BlockedUsersContent(
        state = state,
        onNavigateBack = onNavigateBack,
        onSignIn = onSignIn,
        onRetry = viewModel::load,
        onAskUnblock = viewModel::askUnblock,
        onCancelUnblock = viewModel::cancelUnblock,
        onConfirmUnblock = viewModel::confirmUnblock,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun BlockedUsersContent(
    state: BlockedUsersUiState,
    onNavigateBack: () -> Unit,
    onSignIn: () -> Unit,
    onRetry: () -> Unit,
    onAskUnblock: (BlockedUser) -> Unit,
    onCancelUnblock: () -> Unit,
    onConfirmUnblock: () -> Unit,
    modifier: Modifier = Modifier,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(stringResource(R.string.blocked_users_title)) },
                onNavigateUp = onNavigateBack,
            )
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            item {
                Text(
                    text = stringResource(R.string.blocked_users_intro),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (state.unblockedNotice) {
                item {
                    Text(stringResource(R.string.blocked_users_unblocked), color = MaterialTheme.colorScheme.primary)
                }
            }
            state.error?.let { error ->
                item {
                    val text = when (error) {
                        is CommunitySafetyError.RateLimited ->
                            stringResource(R.string.safety_error_rate_limited, (error.retryAfterSeconds ?: 30L).toInt())
                        CommunitySafetyError.Offline -> stringResource(R.string.safety_error_offline)
                        else -> stringResource(R.string.safety_error_generic)
                    }
                    Text(text, color = MaterialTheme.colorScheme.error)
                }
            }
            when {
                !state.signedIn -> item {
                    EmptyState(
                        message = stringResource(R.string.blocked_users_sign_in),
                        icon = Icons.Outlined.PersonOff,
                        actionLabel = stringResource(R.string.profile_action_sign_in),
                        onAction = onSignIn,
                    )
                }
                state.loading && state.items.isEmpty() -> item {
                    SkeletonGroup(modifier = Modifier.fillMaxWidth()) {
                        SkeletonCard(height = 72.dp)
                        SkeletonCard(height = 72.dp)
                    }
                }
                state.loadFailed && state.items.isEmpty() -> item {
                    EmptyState(
                        message = stringResource(R.string.blocked_users_load_error),
                        actionLabel = stringResource(R.string.action_retry),
                        onAction = onRetry,
                    )
                }
                state.isEmpty -> item {
                    EmptyState(
                        message = stringResource(R.string.blocked_users_empty_title),
                        hint = stringResource(R.string.blocked_users_empty_message),
                        icon = Icons.Outlined.Block,
                    )
                }
                else -> item {
                    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
                    Card(
                        modifier = Modifier.fillMaxWidth(),
                        shape = AnpContainerDefaults.containerShape(),
                        colors = CardDefaults.cardColors(containerColor = containerColor),
                        border = AnpContainerDefaults.borderStroke(containerColor),
                    ) {
                        Column {
                            state.items.forEachIndexed { index, user ->
                                if (index > 0) HorizontalDivider()
                                BlockedUserRow(user = user, enabled = !state.unblocking, onUnblock = { onAskUnblock(user) })
                            }
                        }
                    }
                }
            }
        }
    }

    state.pendingUnblock?.let { user ->
        val label = blockedUserLabel(user)
        AlertDialog(
            onDismissRequest = { if (!state.unblocking) onCancelUnblock() },
            title = { Text(stringResource(R.string.blocked_users_unblock_title, label)) },
            text = { Text(stringResource(R.string.blocked_users_unblock_message)) },
            confirmButton = {
                Button(onClick = onConfirmUnblock, enabled = !state.unblocking) {
                    if (state.unblocking) {
                        CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp)
                    } else {
                        Text(stringResource(R.string.blocked_users_unblock))
                    }
                }
            },
            dismissButton = {
                TextButton(onClick = onCancelUnblock, enabled = !state.unblocking) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }
}

@Composable
private fun BlockedUserRow(user: BlockedUser, enabled: Boolean, onUnblock: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .heightIn(min = 64.dp)
            .padding(horizontal = 16.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Icon(
            Icons.Outlined.PersonOff,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(
                text = blockedUserLabel(user),
                style = MaterialTheme.typography.titleSmall,
                modifier = Modifier.semantics { heading() },
            )
            Text(
                text = stringResource(R.string.blocked_users_since, formatBlockDate(user.createdAt)),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        OutlinedButton(onClick = onUnblock, enabled = enabled) {
            Text(stringResource(R.string.blocked_users_unblock))
        }
    }
}

/** The alias seen when blocking, or a neutral name by where the block was made (B-BR-SAFE-07). */
@Composable
internal fun blockedUserLabel(user: BlockedUser): String {
    val label = user.label.trim()
    if (label.isNotEmpty()) return label
    return stringResource(
        when (user.sourceKind) {
            SafetyContentKind.FORUM_POST, SafetyContentKind.FORUM_COMMENT -> R.string.blocked_users_fallback_forum
            SafetyContentKind.STATION_COMMENT -> R.string.blocked_users_fallback_station
            null -> R.string.blocked_users_fallback_generic
        },
    )
}

/** Local medium date of an RFC 3339 timestamp; the raw date part when it cannot be read. */
internal fun formatBlockDate(rfc3339: String, zone: ZoneId = ZoneId.systemDefault()): String = try {
    OffsetDateTime.parse(rfc3339)
        .atZoneSameInstant(zone)
        .format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM))
} catch (_: Exception) {
    rfc3339.take(10)
}
