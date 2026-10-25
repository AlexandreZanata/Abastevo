package com.anpfuel.app.ui.account

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material.icons.filled.Logout
import androidx.compose.material.icons.filled.PersonOutline
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
import com.anpfuel.app.ui.components.ProfileActionRow
import com.anpfuel.app.ui.components.SkeletonCard
import com.anpfuel.app.ui.components.SkeletonGroup
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
    authViewModel: AuthViewModel = hiltViewModel(),
) {
    val authState by authViewModel.uiState.collectAsStateWithLifecycle()
    val signedIn = authState.step == AuthStep.AUTHENTICATED || authState.step == AuthStep.OFFLINE_ACCOUNT
    LaunchedEffect(authViewModel, signedIn) {
        authViewModel.refreshAccount()
        if (signedIn) authViewModel.loadBackup()
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
                onClick = onLogout,
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
