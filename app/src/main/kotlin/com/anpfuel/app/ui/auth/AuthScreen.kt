package com.anpfuel.app.ui.auth

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Key
import androidx.compose.material.icons.filled.PersonOutline
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AbastevoBrandBanner
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.theme.AnpFuelTheme

/**
 * FREE-account screen: anonymous key accounts (P13 key extension).
 *
 * A username creates the account, signs in automatically and opens the
 * protected key-backup step. The saved key also supports later login
 * (no email or provider ceremony). Session tokens never render:
 * only the account key can be deliberately revealed or exported.
 * Provider completion for legacy accounts arrives via the
 * `anpfuel://auth/callback` deep link; the native SDK minting real id
 * tokens is device-gated (release horizon).
 */
@Composable
fun AuthRoute(
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: AuthViewModel = hiltViewModel(),
    providerArg: String = "",
    idTokenArg: String = "",
    nonceArg: String = "",
    stateArg: String = "",
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    LaunchedEffect(providerArg, idTokenArg, nonceArg, stateArg) {
        if (providerArg.isNotEmpty()) {
            viewModel.onProviderCallback(providerArg, idTokenArg, nonceArg, stateArg)
        }
    }
    LaunchedEffect(viewModel) {
        viewModel.navigation.collect { onNavigateBack() }
    }
    androidx.lifecycle.compose.LifecycleEventEffect(androidx.lifecycle.Lifecycle.Event.ON_RESUME) {
        viewModel.refreshAccount()
    }
    AuthScreen(
        state = uiState,
        onEmailChange = viewModel::onEmailChange,
        onCodeChange = viewModel::onCodeChange,
        onRequestCode = viewModel::onRequestCode,
        onConsumeCode = viewModel::onConsumeCode,
        onBackToEmail = viewModel::onBackToEmail,
        onUsernameChange = viewModel::onUsernameChange,
        onKeyChange = viewModel::onKeyChange,
        onCreateAccount = viewModel::onCreateAccount,
        onLoginWithKey = viewModel::onLoginWithKey,
        onGoToKeyEntry = viewModel::onGoToKeyEntry,
        onBackToUsername = viewModel::onBackToUsername,
        onProviderClick = viewModel::onProviderClick,
        onCancelProviderLink = viewModel::onCancelProviderLink,
        onLogout = viewModel::onLogout,
        onDeleteAccount = viewModel::onDeleteAccount,
        onDismissError = viewModel::onDismissError,
        onNavigateBack = onNavigateBack,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AuthScreen(
    state: AuthUiState,
    onEmailChange: (String) -> Unit,
    onCodeChange: (String) -> Unit,
    onRequestCode: () -> Unit,
    onConsumeCode: () -> Unit,
    onBackToEmail: () -> Unit,
    onUsernameChange: (String) -> Unit,
    onKeyChange: (String) -> Unit,
    onCreateAccount: () -> Unit,
    onLoginWithKey: () -> Unit,
    onGoToKeyEntry: () -> Unit,
    onBackToUsername: () -> Unit,
    onProviderClick: (String) -> Unit,
    onCancelProviderLink: () -> Unit,
    onLogout: () -> Unit,
    onDeleteAccount: () -> Unit,
    onDismissError: () -> Unit,
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var showDeleteConfirm by remember { mutableStateOf(false) }
    val focusManager = LocalFocusManager.current
    val canCreate = state.username.length in 3..20 && state.username.firstOrNull() in 'a'..'z'
    LaunchedEffect(state.step) {
        if (state.step !in listOf(AuthStep.USERNAME_ENTRY, AuthStep.KEY_ENTRY, AuthStep.EMAIL_ENTRY, AuthStep.CODE_SENT)) {
            focusManager.clearFocus()
        }
    }
    SecureAccountWindow()
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            AnpTopAppBar(title = { Text(stringResource(R.string.auth_title)) }, onNavigateUp = onNavigateBack)
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = if (state.step in listOf(AuthStep.KEY_ISSUED, AuthStep.AUTHENTICATED, AuthStep.OFFLINE_ACCOUNT)) 16.dp else 24.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            if (state.step == AuthStep.KEY_ISSUED) {
                Text(stringResource(R.string.profile_account_section), style = MaterialTheme.typography.titleSmall,
                    color = MaterialTheme.colorScheme.primary, modifier = Modifier.fillMaxWidth())
                com.anpfuel.app.ui.account.AccountStatusCard(
                    stringResource(R.string.auth_key_issued_title), stringResource(R.string.auth_key_issued_warning))
                AccountKeyCard(com.anpfuel.application.portable.AuthFlow.KeyBackup(state.issuedUsername, state.issuedKey))
                Button(onClick = onLoginWithKey, modifier = Modifier.fillMaxWidth()) {
                    Text(stringResource(R.string.auth_saved_enter))
                }
                state.error?.let { error -> TextButton(onClick = onDismissError) { Text(errorText(error), color = MaterialTheme.colorScheme.error) } }
                return@Column
            }
            if (state.step in listOf(AuthStep.AUTHENTICATED, AuthStep.OFFLINE_ACCOUNT)) {
                com.anpfuel.app.ui.account.SignedInAccountContent(state, onLogout, { showDeleteConfirm = true })
                Button(onClick = onNavigateBack, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.auth_continue)) }
                state.error?.let { error -> TextButton(onClick = onDismissError) { Text(errorText(error), color = MaterialTheme.colorScheme.error) } }
                return@Column
            }
            AbastevoBrandBanner()
            Card(
                modifier = Modifier.widthIn(max = 520.dp).fillMaxWidth(),
                shape = RoundedCornerShape(28.dp),
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow),
            ) {
                Column(Modifier.padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                    when (state.step) {
                        AuthStep.CHECKING, AuthStep.BUSY -> {
                            CircularProgressIndicator()
                        }
                        AuthStep.USERNAME_ENTRY -> {
                            Text(stringResource(R.string.auth_signup_heading), style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
                            Text(text = stringResource(R.string.auth_key_subtitle), color = MaterialTheme.colorScheme.onSurfaceVariant)
                            val usernameTaken = state.error == AuthUiError.UsernameTaken
                            OutlinedTextField(
                                value = state.username,
                                onValueChange = onUsernameChange,
                                label = { Text(text = stringResource(R.string.auth_username_label)) },
                                leadingIcon = { Icon(Icons.Default.PersonOutline, contentDescription = null) },
                                supportingText = {
                                    Text(
                                        stringResource(
                                            if (usernameTaken) {
                                                R.string.auth_error_username_taken
                                            } else {
                                                R.string.auth_username_hint
                                            },
                                        ),
                                    )
                                },
                                isError = usernameTaken,
                                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Ascii, autoCorrectEnabled = false, imeAction = ImeAction.Done),
                                keyboardActions = KeyboardActions(onDone = { if (canCreate) onCreateAccount() }),
                                shape = RoundedCornerShape(16.dp),
                                singleLine = true,
                                modifier = Modifier.fillMaxWidth(),
                            )
                            Button(
                                onClick = onCreateAccount,
                                enabled = canCreate,
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                Text(text = stringResource(R.string.auth_create_account))
                            }
                            TextButton(onClick = onGoToKeyEntry) {
                                Text(text = stringResource(R.string.auth_have_key))
                            }
                        }
                        AuthStep.KEY_ENTRY -> {
                            Text(stringResource(R.string.auth_login_heading), style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
                            Text(text = stringResource(R.string.auth_key_login_copy), color = MaterialTheme.colorScheme.onSurfaceVariant)
                            OutlinedTextField(
                                value = state.accountKey,
                                onValueChange = onKeyChange,
                                label = { Text(text = stringResource(R.string.auth_key_label)) },
                                leadingIcon = { Icon(Icons.Default.Key, contentDescription = null) },
                                visualTransformation = PasswordVisualTransformation(),
                                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, autoCorrectEnabled = false, imeAction = ImeAction.Done),
                                keyboardActions = KeyboardActions(onDone = { if (state.accountKey.isNotBlank()) onLoginWithKey() }),
                                shape = RoundedCornerShape(16.dp),
                                singleLine = true,
                                modifier = Modifier.fillMaxWidth(),
                            )
                            Button(
                                onClick = onLoginWithKey,
                                enabled = state.accountKey.isNotBlank(),
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                Text(text = stringResource(R.string.auth_key_login))
                            }
                            TextButton(onClick = onBackToUsername) {
                                Text(text = stringResource(R.string.action_back))
                            }
                        }
                        AuthStep.EMAIL_ENTRY, AuthStep.CODE_SENT -> {
                            OutlinedTextField(
                                value = state.email,
                                onValueChange = onEmailChange,
                                label = { Text(text = stringResource(R.string.auth_email_label)) },
                                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email, imeAction = ImeAction.Done),
                                keyboardActions = KeyboardActions(onDone = { onRequestCode() }),
                                singleLine = true,
                                enabled = state.step == AuthStep.EMAIL_ENTRY,
                                modifier = Modifier.fillMaxWidth(),
                            )
                            if (state.step == AuthStep.CODE_SENT) {
                                OutlinedTextField(
                                    value = state.code,
                                    onValueChange = onCodeChange,
                                    label = { Text(text = stringResource(R.string.auth_code_label)) },
                                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number, imeAction = ImeAction.Done),
                                    keyboardActions = KeyboardActions(onDone = { onConsumeCode() }),
                                    singleLine = true,
                                    modifier = Modifier.fillMaxWidth(),
                                )
                                Button(
                                    onClick = onConsumeCode,
                                    modifier = Modifier.fillMaxWidth(),
                                ) {
                                    Text(text = stringResource(R.string.auth_confirm))
                                }
                                TextButton(onClick = onBackToEmail) {
                                    Text(text = stringResource(R.string.action_back))
                                }
                            } else {
                                Button(
                                    onClick = onRequestCode,
                                    modifier = Modifier.fillMaxWidth(),
                                ) {
                                    Text(text = stringResource(R.string.auth_continue))
                                }
                            }
                        }
                        AuthStep.PROVIDER_PENDING -> {
                            CircularProgressIndicator()
                            Text(text = stringResource(R.string.auth_pending_provider, state.pendingProvider))
                            TextButton(onClick = onCancelProviderLink) {
                                Text(text = stringResource(R.string.action_cancel))
                            }
                        }
                        AuthStep.KEY_ISSUED, AuthStep.AUTHENTICATED, AuthStep.OFFLINE_ACCOUNT -> Unit
                    }
                }
            }
            state.error?.let { error ->
                // UsernameTaken already renders inside the username field.
                if (error != AuthUiError.UsernameTaken) {
                    TextButton(onClick = onDismissError) {
                        Text(
                            text = errorText(error),
                            color = MaterialTheme.colorScheme.error,
                        )
                    }
                }
            }
            Spacer(modifier = Modifier.height(8.dp))
            Text(stringResource(R.string.auth_security_footer), style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant)
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
                        onDeleteAccount()
                    },
                ) {
                    Text(text = stringResource(R.string.auth_delete_confirm))
                }
            },
            dismissButton = {
                TextButton(onClick = { showDeleteConfirm = false }) {
                    Text(text = stringResource(R.string.action_cancel))
                }
            },
        )
    }
}

@Composable
private fun errorText(error: AuthUiError): String = stringResource(
    when (error) {
        AuthUiError.InvalidInput -> R.string.auth_error_input
        AuthUiError.WrongCode -> R.string.auth_error_code
        AuthUiError.WrongKey -> R.string.auth_error_key
        AuthUiError.UsernameTaken -> R.string.auth_error_username_taken
        AuthUiError.ExpiredCode -> R.string.auth_error_expired
        AuthUiError.LockedCode -> R.string.auth_error_locked
        AuthUiError.SessionExpired -> R.string.auth_error_session
        AuthUiError.Suspended -> R.string.auth_error_suspended
        AuthUiError.Deleted -> R.string.auth_error_deleted
        AuthUiError.Offline -> R.string.auth_error_offline
        AuthUiError.ProviderDenied -> R.string.auth_error_provider
        AuthUiError.LoginFirst -> R.string.auth_error_login_first
        AuthUiError.SecureStorage -> R.string.auth_error_secure_storage
        AuthUiError.Unknown -> R.string.auth_error_unknown
    },
)

@Preview(showBackground = true)
@Composable
private fun AuthScreenEmailPreview() {
    AnpFuelTheme {
        AuthScreen(
            state = AuthUiState(step = AuthStep.USERNAME_ENTRY, username = "ana123"),
            onEmailChange = {},
            onCodeChange = {},
            onRequestCode = {},
            onConsumeCode = {},
            onBackToEmail = {},
            onUsernameChange = {},
            onKeyChange = {},
            onCreateAccount = {},
            onLoginWithKey = {},
            onGoToKeyEntry = {},
            onBackToUsername = {},
            onProviderClick = {},
            onCancelProviderLink = {},
            onLogout = {},
            onDeleteAccount = {},
            onDismissError = {},
            onNavigateBack = {},
        )
    }
}

@Preview(showBackground = true)
@Composable
private fun AuthScreenAuthenticatedPreview() {
    AnpFuelTheme {
        AuthScreen(
            state = AuthUiState(step = AuthStep.AUTHENTICATED),
            onEmailChange = {},
            onCodeChange = {},
            onRequestCode = {},
            onConsumeCode = {},
            onBackToEmail = {},
            onUsernameChange = {},
            onKeyChange = {},
            onCreateAccount = {},
            onLoginWithKey = {},
            onGoToKeyEntry = {},
            onBackToUsername = {},
            onProviderClick = {},
            onCancelProviderLink = {},
            onLogout = {},
            onDeleteAccount = {},
            onDismissError = {},
            onNavigateBack = {},
        )
    }
}
