package com.anpfuel.app.ui.account

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material.icons.filled.Logout
import androidx.compose.material.icons.filled.PersonOutline
import androidx.compose.material.icons.outlined.Block
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
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
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.BuildConfig
import com.anpfuel.app.R
import com.anpfuel.app.ui.auth.AccountAuthForm
import com.anpfuel.app.ui.auth.AccountKeyCard
import com.anpfuel.app.ui.auth.AuthError
import com.anpfuel.app.ui.auth.AuthStep
import com.anpfuel.app.ui.auth.AuthUiState
import com.anpfuel.app.ui.auth.AuthViewModel
import com.anpfuel.app.ui.auth.ConfirmAccountDeletion
import com.anpfuel.app.ui.auth.SecureAccountWindow
import com.anpfuel.app.ui.auth.editingProfile
import com.anpfuel.app.ui.auth.formStep
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.ProfileActionRow
import com.anpfuel.app.ui.components.SkeletonCard
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.premium.PremiumEntryCard
import com.anpfuel.app.ui.theme.AnpContainerDefaults

/**
 * Dedicated account management page: status, key backup (export) and
 * login/logout/deletion. Reuses the shared session store through
 * [AuthViewModel] plus the reusable [AccountKeyCard]; Perfil links
 * here instead of detouring through the login screen. The only Premium
 * element here is a compact shortcut to the dedicated Premium area.
 *
 * Layout follows the Perfil "tools" pattern: section headings plus
 * rounded cards with icon rows, so the page reads as part of the app.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AccountScreen(
    onNavigateBack: () -> Unit,
    onNavigateToAuth: () -> Unit,
    onOpenPremium: () -> Unit,
    modifier: Modifier = Modifier,
    onNavigateToLedgerSync: () -> Unit = {},
    onOpenBlockedUsers: () -> Unit = {},
    authViewModel: AuthViewModel = hiltViewModel(),
) {
    val authState by authViewModel.uiState.collectAsStateWithLifecycle()
    val signedIn = authState.formStep in setOf(AuthStep.AUTHENTICATED, AuthStep.OFFLINE_ACCOUNT)
    val editing = authState.editingProfile
    val canCancelEdit = editing && authState.formStep != AuthStep.CONFIRM_NAME
    val onBack = if (canCancelEdit) authViewModel::onCancelProfileEdit else onNavigateBack
    BackHandler(enabled = canCancelEdit, onBack = onBack)
    SecureAccountWindow()
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { authViewModel.refreshAccount() }
    LaunchedEffect(authState.step, authState.profile) {
        authViewModel.resumePublicNameConfirmation()
    }
    var showDeleteConfirm by remember { mutableStateOf(false) }
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(text = stringResource(R.string.account_screen_title)) },
                onNavigateUp = onBack,
            )
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .imePadding()
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
            if (editing) {
                AccountAuthForm(authState, authViewModel, onBack)
                return@Column
            }
            if (!signedIn) {
                val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
                Card(
                    modifier = Modifier.fillMaxWidth(),
                    shape = AnpContainerDefaults.containerShape(),
                    colors = CardDefaults.cardColors(
                        containerColor = containerColor,
                    ),
                    border = AnpContainerDefaults.borderStroke(containerColor),
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
                AuthError(authState, authViewModel::onDismissError)
                return@Column
            }
            PremiumEntryCard(
                onClick = onOpenPremium,
                compact = true,
            )
            val ledgerSyncContainer = MaterialTheme.colorScheme.surfaceContainerHighest
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(containerColor = ledgerSyncContainer),
                border = AnpContainerDefaults.borderStroke(ledgerSyncContainer),
            ) {
                ProfileActionRow(icon = Icons.Default.PersonOutline,
                    title = stringResource(R.string.ledger_sync_title),
                    subtitle = stringResource(R.string.ledger_sync_explanation),
                    onClick = onNavigateToLedgerSync)
            }
            // BUC-SAFE-04: the account manages its own blocks.
            val blockedUsersContainer = MaterialTheme.colorScheme.surface
            Card(
                modifier = Modifier.fillMaxWidth(),
                shape = AnpContainerDefaults.containerShape(),
                colors = CardDefaults.cardColors(containerColor = blockedUsersContainer),
                border = AnpContainerDefaults.borderStroke(blockedUsersContainer),
            ) {
                ProfileActionRow(
                    icon = Icons.Outlined.Block,
                    title = stringResource(R.string.blocked_users_title),
                    subtitle = stringResource(R.string.blocked_users_entry_subtitle),
                    onClick = onOpenBlockedUsers,
                )
            }
            SignedInAccountContent(
                state = authState,
                viewModel = authViewModel,
                onDelete = { showDeleteConfirm = true },
            )
        }
    }
    if (showDeleteConfirm) {
        ConfirmAccountDeletion(
            onDismiss = { showDeleteConfirm = false },
            onConfirm = {
                showDeleteConfirm = false
                authViewModel.onDeleteAccount()
            },
        )
    }
}

/** Shared by account details and login completion; one source for their UI. */
@Composable
internal fun SignedInAccountContent(state: AuthUiState, viewModel: AuthViewModel, onDelete: () -> Unit) {
    Text(
        text = stringResource(R.string.profile_account_section),
        style = MaterialTheme.typography.titleSmall,
        color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.semantics { heading() },
    )
    AccountStatusCard(stringResource(R.string.profile_signed_in_title), stringResource(R.string.profile_signed_in_subtitle))
    state.profile?.let { profile ->
        if (state.showProtectionPrompt) {
            ProfileFieldsCard {
                Text(stringResource(R.string.auth_protect_title), style = MaterialTheme.typography.titleMedium)
                Text(stringResource(R.string.auth_protect_copy))
                if (profile.name == null) {
                    TextButton(viewModel::beginPublicNameEdit) { Text(stringResource(R.string.auth_edit_name)) }
                }
                if (profile.recoveryEmailHint == null) {
                    TextButton(viewModel::beginRecoveryEmailChange) { Text(stringResource(R.string.auth_add_recovery_email)) }
                }
                TextButton(viewModel::dismissProtectionPrompt) { Text(stringResource(R.string.auth_protect_dismiss)) }
            }
        }
        ProfileFieldsCard {
            Text(stringResource(R.string.auth_public_name), style = MaterialTheme.typography.titleMedium)
            Text(profile.publicName)
            TextButton(viewModel::beginPublicNameEdit) { Text(stringResource(R.string.auth_edit_name)) }
            HorizontalDivider()
            Text(stringResource(R.string.auth_recovery_email_title), style = MaterialTheme.typography.titleMedium)
            profile.recoveryEmailHint?.let { Text(it) }
            TextButton(viewModel::beginRecoveryEmailChange) {
                Text(stringResource(if (profile.recoveryEmailHint == null) R.string.auth_add_recovery_email else R.string.auth_change_email))
            }
            val linked = "google" in profile.providers
            if (linked || BuildConfig.GOOGLE_SERVER_CLIENT_ID.isNotBlank()) {
                HorizontalDivider()
                if (linked) Text(stringResource(R.string.auth_google_linked))
                else TextButton(viewModel::beginGoogleLink) { Text(stringResource(R.string.auth_google)) }
            }
        }
    }
    AuthError(state, viewModel::onDismissError)
    state.keyBackup?.let { backup ->
        AccountKeyCard(backup = backup)
    }
    val actionContainer = MaterialTheme.colorScheme.surface
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = actionContainer,
        ),
        border = AnpContainerDefaults.borderStroke(actionContainer),
    ) {
        Column {
            ProfileActionRow(
                icon = Icons.Default.Logout,
                title = stringResource(R.string.auth_logout),
                onClick = viewModel::onLogout,
            )
            HorizontalDivider()
            ProfileActionRow(
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
private fun ProfileFieldsCard(content: @Composable () -> Unit) {
    val fill = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = fill),
        border = AnpContainerDefaults.borderStroke(fill),
    ) {
        Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            content()
        }
    }
}

@Composable
internal fun AccountStatusCard(title: String, subtitle: String) {
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(
            containerColor = containerColor,
        ),
        border = AnpContainerDefaults.borderStroke(containerColor),
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
