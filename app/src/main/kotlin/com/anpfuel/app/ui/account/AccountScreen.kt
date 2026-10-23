package com.anpfuel.app.ui.account

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material.icons.filled.Logout
import androidx.compose.material.icons.filled.OpenInNew
import androidx.compose.material.icons.filled.PersonOutline
import androidx.compose.material.icons.filled.Restore
import androidx.compose.material.icons.filled.Star
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.auth.AccountKeyCard
import com.anpfuel.app.ui.auth.AuthStep
import com.anpfuel.app.ui.auth.AuthViewModel
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.SkeletonCard
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.domain.premium.PremiumState

/**
 * Dedicated account management page: status, key backup (export) and
 * login/logout/deletion. Reuses the shared session store through
 * [AuthViewModel] plus the reusable [AccountKeyCard]; Perfil links
 * here instead of detouring through the login screen.
 *
 * Layout follows the Perfil "tools" pattern: section headings plus
 * rounded cards with icon rows, so the page reads as part of the app.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AccountScreen(
    onNavigateBack: () -> Unit,
    onNavigateToAuth: () -> Unit,
    onNavigateToLedgerSync: () -> Unit = {},
    modifier: Modifier = Modifier,
    authViewModel: AuthViewModel = hiltViewModel(),
    premiumViewModel: PremiumViewModel = hiltViewModel(),
) {
    val authState by authViewModel.uiState.collectAsStateWithLifecycle()
    val premiumState by premiumViewModel.uiState.collectAsStateWithLifecycle()
    val signedIn = authState.step == AuthStep.AUTHENTICATED || authState.step == AuthStep.OFFLINE_ACCOUNT
    LaunchedEffect(authViewModel, signedIn) {
        authViewModel.refreshAccount()
        if (signedIn) authViewModel.loadBackup()
    }
    LaunchedEffect(premiumViewModel, signedIn) {
        if (signedIn) premiumViewModel.refresh()
    }
    var showDeleteConfirm by remember { mutableStateOf(false) }
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(text = stringResource(R.string.account_screen_title)) },
                onNavigateUp = onNavigateBack,
            )
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            if (authState.step == AuthStep.CHECKING) {
                SkeletonGroup(modifier = Modifier.fillMaxWidth()) {
                    SkeletonCard(height = 96.dp)
                    SkeletonCard(height = 220.dp)
                }
                return@Column
            }
            if (!signedIn) {
                Card(
                    modifier = Modifier.fillMaxWidth(),
                    shape = RoundedCornerShape(24.dp),
                    colors = CardDefaults.cardColors(
                        containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
                    ),
                ) {
                    Column(
                        modifier = Modifier.padding(24.dp),
                        verticalArrangement = Arrangement.spacedBy(16.dp),
                    ) {
                        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                            Icon(
                                Icons.Default.PersonOutline,
                                contentDescription = null,
                                tint = MaterialTheme.colorScheme.primary,
                            )
                            Text(
                                text = stringResource(R.string.account_guest_title),
                                style = MaterialTheme.typography.titleLarge,
                                modifier = Modifier.semantics { heading() },
                            )
                        }
                        Text(
                            text = stringResource(R.string.account_guest_subtitle),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        Button(onClick = onNavigateToAuth, modifier = Modifier.fillMaxWidth()) {
                            Text(text = stringResource(R.string.profile_action_sign_in))
                        }
                    }
                }
                return@Column
            }
            PremiumSection(
                state = premiumState,
                onRefresh = premiumViewModel::refresh,
                onRestoreUnavailable = premiumViewModel::restoreUnavailable,
                onClearNotice = premiumViewModel::clearNotice,
            )
            Card(modifier = Modifier.fillMaxWidth()) {
                AccountActionRow(icon = Icons.Default.PersonOutline,
                    title = stringResource(R.string.ledger_sync_title),
                    subtitle = stringResource(R.string.ledger_sync_explanation),
                    onClick = onNavigateToLedgerSync)
            }
            SignedInAccountContent(
                state = authState,
                onLogout = { authViewModel.onLogout(); onNavigateBack() },
                onDelete = { showDeleteConfirm = true },
            )
        }
    }
    if (showDeleteConfirm) {
        AlertDialog(
            onDismissRequest = { showDeleteConfirm = false },
            title = { Text(text = stringResource(R.string.auth_delete_title)) },
            text = { Text(text = stringResource(R.string.auth_delete_copy)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        showDeleteConfirm = false
                        authViewModel.onDeleteAccount()
                        onNavigateBack()
                    },
                ) {
                    Text(text = stringResource(R.string.auth_delete_confirm))
                }
            },
            dismissButton = {
                TextButton(onClick = { showDeleteConfirm = false }) {
                    Text(text = stringResource(R.string.action_back))
                }
            },
        )
    }
}

@Composable
private fun PremiumSection(
    state: PremiumUiState,
    onRefresh: () -> Unit,
    onRestoreUnavailable: () -> Unit,
    onClearNotice: () -> Unit,
) {
    val uriHandler = LocalUriHandler.current
    val status = state.status
    val label = when (status?.state) {
        PremiumState.ACTIVE -> stringResource(R.string.premium_status_active)
        PremiumState.PENDING -> stringResource(R.string.premium_status_pending)
        PremiumState.CANCELED -> stringResource(R.string.premium_status_canceled)
        PremiumState.GRACE -> stringResource(R.string.premium_status_grace)
        PremiumState.EXPIRED -> stringResource(R.string.premium_status_expired)
        PremiumState.REVOKED -> stringResource(R.string.premium_status_revoked)
        PremiumState.UNKNOWN -> stringResource(R.string.premium_status_unknown)
        else -> stringResource(R.string.premium_status_inactive)
    }
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(
            text = stringResource(R.string.premium_section_title),
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.primary,
            modifier = Modifier.semantics { heading() },
        )
        Card(modifier = Modifier.fillMaxWidth()) {
            Column {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable(onClick = onRefresh)
                        .padding(horizontal = 16.dp, vertical = 12.dp),
                    horizontalArrangement = Arrangement.spacedBy(12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Icon(
                        imageVector = Icons.Default.Star,
                        contentDescription = null,
                        tint = MaterialTheme.colorScheme.primary,
                    )
                    Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        Text(text = label, style = MaterialTheme.typography.bodyLarge)
                        Text(
                            text = status?.validUntil?.take(10)
                                ?.let { stringResource(R.string.premium_valid_until, it) }
                                ?: stringResource(R.string.premium_retained),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
                state.notice?.let { notice ->
                    Text(
                        text = when (notice) {
                            PremiumNotice.OFFLINE -> stringResource(R.string.premium_offline)
                            PremiumNotice.DENIED -> stringResource(R.string.premium_denied)
                            PremiumNotice.ERROR -> stringResource(R.string.premium_error)
                            PremiumNotice.RESTORE_UNAVAILABLE -> stringResource(R.string.premium_restore_unavailable)
                        },
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable(onClick = onClearNotice)
                            .padding(horizontal = 16.dp, vertical = 8.dp),
                    )
                }
                HorizontalDivider()
                AccountActionRow(
                    icon = Icons.Default.OpenInNew,
                    title = stringResource(R.string.premium_manage),
                    onClick = {
                        uriHandler.openUri(status?.managementUrl ?: com.anpfuel.domain.premium.PremiumStatus.MANAGEMENT_URL)
                    },
                )
                HorizontalDivider()
                AccountActionRow(
                    icon = Icons.Default.Restore,
                    title = stringResource(R.string.premium_restore),
                    subtitle = stringResource(R.string.premium_restore_note),
                    onClick = onRestoreUnavailable,
                )
            }
        }
    }
}

/** Shared by account details and login completion; one source for their UI. */
@Composable
internal fun SignedInAccountContent(state: com.anpfuel.app.ui.auth.AuthUiState, onLogout: () -> Unit, onDelete: () -> Unit) {
    Text(
        text = stringResource(R.string.profile_account_section),
        style = MaterialTheme.typography.titleSmall,
        color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.semantics { heading() },
    )
    AccountStatusCard(stringResource(R.string.profile_signed_in_title), stringResource(R.string.profile_signed_in_subtitle))
    state.keyBackup?.let { backup ->
        AccountKeyCard(backup = backup)
    }
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surface,
        ),
    ) {
        Column {
            AccountActionRow(
                icon = Icons.Default.Logout,
                title = stringResource(R.string.auth_logout),
                onClick = onLogout,
            )
            HorizontalDivider()
            AccountActionRow(
                icon = Icons.Default.DeleteOutline,
                title = stringResource(R.string.auth_delete_account),
                subtitle = stringResource(R.string.auth_delete_copy),
                destructive = true,
                onClick = onDelete,
            )
        }
    }
}

@Composable
internal fun AccountStatusCard(title: String, subtitle: String) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(24.dp),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
        ),
    ) {
        Column(
            modifier = Modifier.padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Icon(
                    Icons.Default.PersonOutline,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.primary,
                )
                Text(
                    text = title,
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.semantics { heading() },
                )
            }
            Text(
                text = subtitle,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }

}

@Composable
private fun AccountActionRow(
    icon: ImageVector,
    title: String,
    onClick: () -> Unit,
    subtitle: String? = null,
    destructive: Boolean = false,
) {
    val contentColor = if (destructive) {
        MaterialTheme.colorScheme.error
    } else {
        MaterialTheme.colorScheme.onSurface
    }
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Icon(
            imageVector = icon,
            contentDescription = null,
            tint = if (destructive) {
                MaterialTheme.colorScheme.error
            } else {
                MaterialTheme.colorScheme.onSurfaceVariant
            },
        )
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(2.dp),
        ) {
            Text(
                text = title,
                style = MaterialTheme.typography.bodyLarge,
                color = contentColor,
            )
            subtitle?.let {
                Text(
                    text = it,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        Icon(
            imageVector = Icons.Default.ChevronRight,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.outline,
        )
    }
}
